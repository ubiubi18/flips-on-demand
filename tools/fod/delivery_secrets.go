package main

// The buyer's half of a pool delivery: read the ciphertext the registry
// published, decrypt it with the delivery key the buyer kept, and rebuild the
// epoch key pairs as a vault the blob path opens.

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/idena-network/idena-go/crypto"
)

const (
	// deliverySecretsDomain and the layout behind it are `SECRETS_DOMAIN`,
	// epoch, author, and the two 32-byte scalars in
	// `proofs/delivery/core/src/lib.rs`.
	deliverySecretsDomain = "fod:epoch-secrets:v1"
	deliverySecretsBytes  = 106
)

// readRegistryValue reads one storage value of the registry by its key.
func readRegistryValue(c *client, contract, key string) ([]byte, error) {
	var value string
	if err := c.call("contract_readData", []any{"0x" + contract, key, "hex"}, &value); err != nil {
		// The pinned node reports absent state through this exact RPC error.
		// Network/decoder/other node failures must never trigger a fallback.
		var answered answeredError
		if errors.As(err, &answered) && answered.Error() == "rpc contract_readData: data is nil (-32000)" {
			return nil, fmt.Errorf("read %s: %w", key, errNotInRegistry)
		}
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", key, err)
	}
	return raw, nil
}

// epochKeysFromDeliveredSecrets rebuilds the epoch key pairs from the bytes a
// delivery carries, so a buyer holds usable keys without the author's identity
// key (`EpochKeysFromVaultPayload` does the same from a vault).
func epochKeysFromDeliveredSecrets(plain []byte) (*EpochKeys, VaultPayload, error) {
	if len(plain) != deliverySecretsBytes {
		return nil, VaultPayload{}, fmt.Errorf("the delivered secrets are %d bytes, expected %d", len(plain), deliverySecretsBytes)
	}
	if domain := string(plain[:len(deliverySecretsDomain)]); domain != deliverySecretsDomain {
		return nil, VaultPayload{}, fmt.Errorf("the delivered secrets carry the domain %q, expected %q", domain, deliverySecretsDomain)
	}
	offset := len(deliverySecretsDomain)
	epoch := binary.BigEndian.Uint16(plain[offset : offset+2])
	author := plain[offset+2 : offset+22]
	publicSecret := plain[offset+22 : offset+54]
	privateSecret := plain[offset+54 : offset+86]

	publicKey, err := crypto.ToECDSA(publicSecret)
	if err != nil {
		return nil, VaultPayload{}, fmt.Errorf("public part secret: %w", err)
	}
	privateKey, err := crypto.ToECDSA(privateSecret)
	if err != nil {
		return nil, VaultPayload{}, fmt.Errorf("private part secret: %w", err)
	}
	publicECDSA := &publicKey.PublicKey
	privateECDSA := &privateKey.PublicKey

	payload := VaultPayload{
		Kind:           "fod.epoch-keys",
		Version:        1,
		Epoch:          epoch,
		Address:        "0x" + hex.EncodeToString(author),
		PublicPartKey:  fmt.Sprintf("%x", crypto.CompressPubkey(publicECDSA)),
		PrivatePartKey: fmt.Sprintf("%x", crypto.CompressPubkey(privateECDSA)),
		PublicPartFP:   fingerprintBytes(crypto.FromECDSAPub(publicECDSA)),
		PrivatePartFP:  fingerprintBytes(crypto.FromECDSAPub(privateECDSA)),
		PublicPartSec:  fmt.Sprintf("%x", publicSecret),
		PrivatePartSec: fmt.Sprintf("%x", privateSecret),
	}
	keys, err := EpochKeysFromVaultPayload(payload)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	return keys, payload, nil
}
