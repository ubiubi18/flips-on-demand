package main

// The delivery prover's client half, shared by the desk's `serve` fallback: the
// desk derives the buyer-bound ciphertext witness, hands the order to the
// prover on stdin so the witness never reaches the filesystem, and assembles
// the exact `fulfill` argument the escrow checks: order id (8) || journal (281)
// || seal (256) || ciphertext.

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

const (
	operationFulfill       = "fulfill"
	preparedDeliveryFormat = "fod.prepared-delivery"
	deliveryStatement      = "fod:delivery:v2"
	deliveryNetworkName    = "idena"

	// The escrow parses the fulfill argument in this layout, so the sizes are
	// pinned here as well as in the contract and the prover.
	fulfillJournalBytes  = 281
	fulfillSealBytes     = 256
	ephemeralScalarBytes = 32
)

// fulfillArguments is the typed argument set of a `fulfill` action request.
type fulfillArguments struct {
	FundingHeight uint64 `json:"fundingHeight"`
	FundingRoot   string `json:"fundingRoot"`
	OfferID       uint64 `json:"offerId"`
	OrderNonce    uint64 `json:"orderNonce"`
	Seller        string `json:"seller"`
	Buyer         string `json:"buyer"`
	DeliveryKey   string `json:"deliveryKey"`
	Epoch         uint16 `json:"epoch"`
	Author        string `json:"author"`
}

// PreparedDelivery is what the seller's wallet submits: the terms to recheck
// against the chain, plus the exact argument bytes for `fulfill`.
type PreparedDelivery struct {
	Format      string `json:"format"`
	Version     int    `json:"version"`
	Operation   string `json:"operation"`
	Network     string `json:"network"`
	Contract    string `json:"contract"`
	OfferID     uint64 `json:"offerId"`
	OrderNonce  uint64 `json:"orderNonce"`
	Seller      string `json:"seller"`
	Buyer       string `json:"buyer"`
	DeliveryKey string `json:"deliveryKey"`
	Amount      string `json:"amount"`
	Calldata    string `json:"calldata"`
	Proof       string `json:"proof"`
	PreparedAt  string `json:"preparedAt"`
	Submitted   bool   `json:"submitted"`
}

// deliveryOrderInput is the prover's view of one delivery: the public terms the
// escrow re-checks against its own funded order, plus the witness. It is
// marshalled to the prover's stdin and never written by this CLI.
type deliveryOrderInput struct {
	Network           string `json:"network"`
	FundedAt          uint64 `json:"funded_at"`
	FundingRoot       string `json:"funding_root"`
	Note              string `json:"note"`
	Contract          string `json:"contract"`
	OfferID           uint64 `json:"offer_id"`
	OrderNonce        uint64 `json:"order_nonce"`
	Seller            string `json:"seller"`
	Buyer             string `json:"buyer"`
	BuyerKey          string `json:"buyer_key"`
	Epoch             uint16 `json:"epoch"`
	Author            string `json:"author"`
	PublicPartSecret  string `json:"public_part_secret"`
	PrivatePartSecret string `json:"private_part_secret"`
	Ephemeral         string `json:"ephemeral"`
}

// proverArtefact is what the delivery prover writes. The witness is absent
// because the CLI asks for it to be left out.
type proverArtefact struct {
	ImageID              string `json:"image_id"`
	ClaimDigest          string `json:"claim_digest"`
	Statement            string `json:"statement"`
	Journal              string `json:"journal"`
	Seal                 string `json:"seal"`
	SealBytes            int    `json:"seal_bytes"`
	Ciphertext           string `json:"ciphertext"`
	CiphertextBytes      int    `json:"ciphertext_bytes"`
	FulfillArgument      string `json:"fulfill_argument"`
	FulfillArgumentBytes int    `json:"fulfill_argument_bytes"`
	Order                struct {
		Contract    string `json:"contract"`
		Network     string `json:"network"`
		FundedAt    uint64 `json:"funded_at"`
		FundingRoot string `json:"funding_root"`
		BuyerKey    string `json:"buyer_key"`
		OfferID     uint64 `json:"offer_id"`
		OrderNonce  uint64 `json:"order_nonce"`
		Seller      string `json:"seller"`
		Buyer       string `json:"buyer"`
	} `json:"order"`
}

// newEphemeralScalar draws the scalar the delivery ECIES encryption uses. It is
// fresh per delivery: a reused ephemeral across orders would leak the plaintext.
func newEphemeralScalar() ([]byte, error) {
	order := crypto.S256().Params().N
	for attempt := 0; attempt < 16; attempt++ {
		raw := make([]byte, ephemeralScalarBytes)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		value := new(big.Int).SetBytes(raw)
		if value.Sign() > 0 && value.Cmp(order) < 0 {
			return raw, nil
		}
	}
	return nil, errors.New("could not draw a scalar below the group order")
}

// AssembleDelivery checks the prover's artefact against the request and builds
// the prepared action. Every field the escrow re-checks is read back out of the
// argument, so a prover that named another order is refused here rather than on
// chain.
func AssembleDelivery(request *actionRequest, arguments *fulfillArguments, artefact *proverArtefact) (*PreparedDelivery, error) {
	if artefact.Statement != deliveryStatement {
		return nil, fmt.Errorf("the artefact is not a delivery proof: %q", artefact.Statement)
	}
	journal, err := hex.DecodeString(artefact.Journal)
	if err != nil {
		return nil, fmt.Errorf("artefact journal: %w", err)
	}
	claim, err := claimDigestFromHex(deliveryImageIDHex, journal)
	if err != nil || artefact.ImageID != deliveryImageIDHex || artefact.ClaimDigest != hex.EncodeToString(claim[:]) {
		return nil, fmt.Errorf("delivery receipt does not bind the pinned guest and journal")
	}
	seal, err := hex.DecodeString(artefact.Seal)
	if err != nil {
		return nil, fmt.Errorf("artefact seal: %w", err)
	}
	ciphertext, err := hex.DecodeString(artefact.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("artefact ciphertext: %w", err)
	}
	calldata, err := hex.DecodeString(artefact.FulfillArgument)
	if err != nil {
		return nil, fmt.Errorf("artefact fulfill argument: %w", err)
	}

	if len(journal) != fulfillJournalBytes {
		return nil, fmt.Errorf("artefact journal is %d bytes, expected %d", len(journal), fulfillJournalBytes)
	}
	if len(seal) != fulfillSealBytes {
		return nil, fmt.Errorf("artefact seal is %d bytes, expected %d", len(seal), fulfillSealBytes)
	}
	if binary.BigEndian.Uint32(journal[:4]) != 2 || hex.EncodeToString(journal[4:24]) != strings.TrimPrefix(strings.ToLower(request.Contract), "0x") {
		return nil, fmt.Errorf("wrong delivery version or contract")
	}
	network := sha256.Sum256([]byte(deliveryNetworkName))
	ciphertextHash := sha256.Sum256(ciphertext)
	if !bytes.Equal(journal[209:241], network[:]) || !bytes.Equal(journal[177:209], ciphertextHash[:]) ||
		binary.BigEndian.Uint64(journal[241:249]) != arguments.FundingHeight || arguments.FundingHeight == 0 ||
		hex.EncodeToString(journal[249:281]) != strings.ToLower(arguments.FundingRoot) ||
		hex.EncodeToString(journal[80:145]) != strings.TrimPrefix(strings.ToLower(arguments.DeliveryKey), "0x") {
		return nil, fmt.Errorf("delivery network, funding anchor, ciphertext or buyer key differs")
	}
	expected := 8 + len(journal) + len(seal) + len(ciphertext)
	if len(calldata) != expected {
		return nil, fmt.Errorf("fulfill argument is %d bytes, expected %d", len(calldata), expected)
	}
	if !bytes.Equal(calldata[8:8+len(journal)], journal) || !bytes.Equal(calldata[8+len(journal):8+len(journal)+len(seal)], seal) || !bytes.Equal(calldata[8+len(journal)+len(seal):], ciphertext) {
		return nil, fmt.Errorf("fulfill calldata differs from the checked receipt")
	}
	if binary.BigEndian.Uint64(calldata[:8]) != arguments.OfferID {
		return nil, fmt.Errorf("the fulfill argument names another order")
	}

	// The journal repeats the trade context; the escrow checks it field by field,
	// so a disagreement with the request is a refusal here instead of a revert.
	if binary.BigEndian.Uint64(journal[24:32]) != arguments.OfferID {
		return nil, fmt.Errorf("the proof's journal names another order")
	}
	if binary.BigEndian.Uint64(journal[32:40]) != arguments.OrderNonce {
		return nil, fmt.Errorf("the proof's journal names another order nonce")
	}
	if !strings.EqualFold(hex.EncodeToString(journal[40:60]), arguments.Seller) {
		return nil, fmt.Errorf("the proof's journal names another seller")
	}
	if !strings.EqualFold(hex.EncodeToString(journal[60:80]), arguments.Buyer) {
		return nil, fmt.Errorf("the proof's journal names another buyer")
	}
	if artefact.Order.OfferID != arguments.OfferID || artefact.Order.OrderNonce != arguments.OrderNonce {
		return nil, fmt.Errorf("the artefact was proved for another order")
	}

	return &PreparedDelivery{
		Format:      preparedDeliveryFormat,
		Version:     actionRequestVersion,
		Operation:   operationFulfill,
		Network:     request.Network,
		Contract:    strings.ToLower(request.Contract),
		OfferID:     arguments.OfferID,
		OrderNonce:  arguments.OrderNonce,
		Seller:      strings.ToLower(arguments.Seller),
		Buyer:       strings.ToLower(arguments.Buyer),
		DeliveryKey: strings.ToLower(arguments.DeliveryKey),
		Amount:      "0",
		Calldata:    hex.EncodeToString(calldata),
		Proof:       "",
		PreparedAt:  time.Now().UTC().Format(time.RFC3339),
		Submitted:   false,
	}, nil
}

// runDeliveryProver proves one order. The witness travels on stdin and the
// prover is told to leave it out of the artefact it writes.
func runDeliveryProver(proverPath, proofPath string, order []byte) (string, error) {
	cmd := exec.Command(proverPath, "--order", "-", "--out", proofPath, "--no-witness")
	cmd.Stdin = bytes.NewReader(order)
	var log bytes.Buffer
	cmd.Stdout = &log
	cmd.Stderr = &log
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("the prover failed: %w\n%s", err, tailLines(log.String(), 8))
	}
	return log.String(), nil
}

func tailLines(text string, count int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}

func readProverArtefact(path string) (*proverArtefact, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the prover's artefact: %w", err)
	}
	var artefact proverArtefact
	if err := json.Unmarshal(raw, &artefact); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &artefact, nil
}

// defaultProofPath keeps the artefact beside the prepared action unless the
// caller names a path.
func defaultProofPath(preparedPath, proofPath string) string {
	if proofPath != "" {
		return proofPath
	}
	return strings.TrimSuffix(preparedPath, ".json") + ".proof.json"
}
