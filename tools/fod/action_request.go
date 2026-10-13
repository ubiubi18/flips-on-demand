package main

// The action-request envelope the interface writes and the CLI reads, plus the
// small shared helpers every prepare mode builds on.

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
)

const (
	actionRequestFormat  = "fod.action-request"
	actionRequestVersion = 1

	// Same domain separator and layout the escrow rebuilds and verifies.
	deliveryKeyDomain = "fod:delivery-key:v1"
	deliveryKeyBytes  = 65
	signatureBytes    = 65
	addressBytes      = 20
)

type actionRequest struct {
	Format    string          `json:"format"`
	Version   int             `json:"version"`
	Network   string          `json:"network"`
	Contract  string          `json:"contract"`
	Operation string          `json:"operation"`
	Arguments json.RawMessage `json:"arguments"`
	Amount    string          `json:"amount"`
	CreatedAt string          `json:"createdAt"`
}

func decodeStrict(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func isHex(value string, bytes int) bool {
	if len(value) != bytes*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func isDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// readActionRequest validates the envelope every action request shares, whatever
// operation it names. The operation's own arguments are checked by its caller.
func readActionRequest(path string) (*actionRequest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var request actionRequest
	if err := decodeStrict(raw, &request); err != nil {
		return nil, fmt.Errorf("read action request: %w", err)
	}
	if request.Format != actionRequestFormat {
		return nil, fmt.Errorf("not an action request: %q", request.Format)
	}
	if request.Version != actionRequestVersion {
		return nil, fmt.Errorf("unsupported action request version %d", request.Version)
	}
	if request.Network == "" {
		return nil, errors.New("the request names no network")
	}
	if !isHex(request.Contract, addressBytes) {
		return nil, errors.New("the request names no valid contract address")
	}
	if !isDecimal(request.Amount) {
		return nil, errors.New("the request amount is not an integer in base units")
	}
	// The contract stores amounts as unsigned 128-bit integers; a larger amount
	// would saturate there, so it is refused here instead of being adjusted.
	amount, ok := new(big.Int).SetString(request.Amount, 10)
	if !ok {
		return nil, errors.New("the request amount could not be read")
	}
	if amount.Cmp(maxContractAmount) > 0 {
		return nil, errors.New("the request amount is larger than the contract can hold")
	}
	if len(request.Arguments) == 0 {
		return nil, errors.New("the request carries no arguments")
	}
	return &request, nil
}

var maxContractAmount = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))

// nodekeyAddressOf returns the address a loaded identity key controls, in the
// form the interface writes and the contract hashes: 40 hex characters, no 0x.
func nodekeyAddressOf(identityKey *ecdsa.PrivateKey) (string, error) {
	sec := secstore.NewSecStore()
	sec.AddKey(crypto.FromECDSA(identityKey))
	defer sec.Destroy()
	return strings.ToLower(strings.TrimPrefix(sec.GetAddress().String(), "0x")), nil
}

// CreateDeliveryKey writes a fresh dedicated delivery key where the caller
// asked, owner-only, and refuses to overwrite anything already there. `SPEC.md`
// 6.1 gives the buyer's prepare command this job; the key file is the buyer's
// to keep (`docs/PROGRESS.md`, 2026-09-26).
func CreateDeliveryKey(path string) (string, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return "", fmt.Errorf("generate delivery key: %w", err)
	}
	raw := crypto.FromECDSA(key)
	if err := writePrivateFile(path, false, func(file *os.File) error {
		_, err := file.Write([]byte(hex.EncodeToString(raw)))
		return err
	}); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey)), nil
}

func u64be(value uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, value)
	return out
}
