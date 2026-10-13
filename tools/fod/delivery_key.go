package main

// The buyer's delivery key and its encrypted recovery archive (`SPEC.md` 6.5
// steps 4-5): the key is bound into the funding call, it is what opens the
// delivery the registry publishes, and losing it is not a refund condition, so
// the buyer can keep it as an owner-only file and as an age-encrypted archive
// under a passphrase of their own.

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/idena-network/idena-go/crypto"
)

// DeliveryKeyPayload is what an encrypted delivery-key archive holds.
type DeliveryKeyPayload struct {
	Kind       string `json:"kind"`
	Version    int    `json:"version"`
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
}

// WriteDeliveryKeyArchive saves the key under a passphrase, so a lost machine
// does not lose what the buyer paid for.
func WriteDeliveryKeyArchive(path, passphrase string, key *ecdsa.PrivateKey) error {
	payload, err := json.Marshal(DeliveryKeyPayload{
		Kind:       "fod.delivery-key",
		Version:    1,
		PublicKey:  hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey)),
		PrivateKey: hex.EncodeToString(crypto.FromECDSA(key)),
	})
	if err != nil {
		return fmt.Errorf("encode delivery-key archive: %w", err)
	}
	return WriteVault(path, passphrase, payload)
}

// ReadDeliveryKeyArchive opens one and rebuilds the key.
func ReadDeliveryKeyArchive(path, passphrase string) (*ecdsa.PrivateKey, error) {
	raw, err := ReadVault(path, passphrase)
	if err != nil {
		return nil, err
	}
	var payload DeliveryKeyPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse the archive: %w", err)
	}
	if payload.Kind != "fod.delivery-key" {
		return nil, fmt.Errorf("this vault holds %q, not a delivery key", payload.Kind)
	}
	return parseDeliveryKey(payload.PrivateKey)
}

// parseDeliveryKey reads the 32-byte hex secret of a delivery key.
func parseDeliveryKey(text string) (*ecdsa.PrivateKey, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(text), "0x"))
	if err != nil {
		return nil, fmt.Errorf("delivery key is not hex: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("delivery key must be 32 bytes, got %d", len(raw))
	}
	key, err := crypto.ToECDSA(raw)
	if err != nil {
		return nil, fmt.Errorf("delivery key is not a valid secp256k1 key: %w", err)
	}
	return key, nil
}

// loadDeliveryKey accepts either the owner-only hex file this CLI writes or an
// encrypted archive of the same key. The passphrase is asked for only when the
// file is not the hex form, so scripted use with a plain key stays quiet.
func loadDeliveryKey(path string) (*ecdsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(raw))
	if _, hexErr := hex.DecodeString(strings.TrimPrefix(text, "0x")); hexErr == nil && len(text) > 0 {
		return parseDeliveryKey(text)
	}
	passphrase, err := readPassphrase(fmt.Sprintf("Passphrase for %s: ", path))
	if err != nil {
		return nil, err
	}
	return ReadDeliveryKeyArchive(path, passphrase)
}
