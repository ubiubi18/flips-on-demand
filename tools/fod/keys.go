package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
	models "github.com/idena-network/idena-go/protobuf"
	"github.com/idena-network/idena-go/secstore"
	"google.golang.org/protobuf/encoding/protowire"
)

// EpochKeys holds the two key pairs an author's epoch flips are encrypted to.
//
// Idena derives both from the identity key and the epoch number:
//
//	public seed  = "flip-key-for-epoch-<epoch>"
//	private seed = "flip-private-key-for-epoch-<epoch>"
//	hash         = common.Hash(crypto.Hash(seed))
//	signature    = identityKey.Sign(hash)
//	key pair     = the legacy Go 1.17 seed mapping of the signature,
//	               imported as an ECIES key
//
// Reference: idena-go/core/flip/flipper.go generateFlipEncryptionKey.
//
// The private halves are secrets. Only public material and fingerprints are
// returned from this type unless a caller explicitly asks for the secret bytes.
type EpochKeys struct {
	Epoch           uint16 `json:"epoch"`
	Address         string `json:"address"`
	PublicPartKey   string `json:"publicPartEncryptionPublicKey"`
	PrivatePartKey  string `json:"privatePartEncryptionPublicKey"`
	PublicPartFP    string `json:"publicPartKeyFingerprint"`
	PrivatePartFP   string `json:"privatePartKeyFingerprint"`
	PublicSeedText  string `json:"publicPartKeySeedText"`
	PrivateSeedText string `json:"privatePartKeySeedText"`

	publicPair  *ecies.PrivateKey
	privatePair *ecies.PrivateKey
}

// DeriveEpochKeys reproduces the node's per-epoch flip key derivation from an
// identity private key.
func DeriveEpochKeys(identityKey *ecdsa.PrivateKey, epoch uint16) (*EpochKeys, error) {
	sec := secstore.NewSecStore()
	sec.AddKey(crypto.FromECDSA(identityKey))
	defer sec.Destroy()

	publicSeed := fmt.Sprintf("flip-key-for-epoch-%v", epoch)
	privateSeed := fmt.Sprintf("flip-private-key-for-epoch-%v", epoch)

	publicPair, err := deriveECIESKey(sec, publicSeed)
	if err != nil {
		return nil, fmt.Errorf("derive public epoch key: %w", err)
	}
	privatePair, err := deriveECIESKey(sec, privateSeed)
	if err != nil {
		return nil, fmt.Errorf("derive private epoch key: %w", err)
	}

	return &EpochKeys{
		Epoch:           epoch,
		Address:         sec.GetAddress().String(),
		PublicPartKey:   fmt.Sprintf("%x", crypto.CompressPubkey(publicPair.ExportECDSA().Public().(*ecdsa.PublicKey))),
		PrivatePartKey:  fmt.Sprintf("%x", crypto.CompressPubkey(privatePair.ExportECDSA().Public().(*ecdsa.PublicKey))),
		PublicPartFP:    fingerprintBytes(crypto.FromECDSAPub(publicPair.ExportECDSA().Public().(*ecdsa.PublicKey))),
		PrivatePartFP:   fingerprintBytes(crypto.FromECDSAPub(privatePair.ExportECDSA().Public().(*ecdsa.PublicKey))),
		PublicSeedText:  publicSeed,
		PrivateSeedText: privateSeed,
		publicPair:      publicPair,
		privatePair:     privatePair,
	}, nil
}

// deriveECIESKey follows the node exactly: hash the seed string, sign the hash
// with the identity key, then turn that signature into an ECDSA key.
func deriveECIESKey(sec *secstore.SecStore, seed string) (*ecies.PrivateKey, error) {
	hash := common.Hash(crypto.Hash([]byte(seed)))
	signature := sec.Sign(hash.Bytes())
	key, err := generateProtocolKeyFromSeed(bytes.NewReader(signature))
	if err != nil {
		return nil, err
	}
	return ecies.ImportECDSA(key), nil
}

// generateProtocolKeyFromSeed preserves the Go 1.17 ecdsa.GenerateKey mapping
// used by Idena's original releases. Flip encryption keys depend on this exact
// 40-byte mapping. Calling crypto.GenerateKeyFromSeed here would make the
// result depend on the idena-go checkout and Go generation it was built from.
func generateProtocolKeyFromSeed(reader io.Reader) (*ecdsa.PrivateKey, error) {
	params := crypto.S256().Params()
	seed := make([]byte, params.BitSize/8+8)
	if _, err := io.ReadFull(reader, seed); err != nil {
		return nil, err
	}

	k := new(big.Int).SetBytes(seed)
	nMinusOne := new(big.Int).Sub(params.N, big.NewInt(1))
	k.Mod(k, nMinusOne)
	k.Add(k, big.NewInt(1))

	scalar := make([]byte, (params.BitSize+7)/8)
	raw := k.Bytes()
	copy(scalar[len(scalar)-len(raw):], raw)
	return crypto.ToECDSA(scalar)
}

// EncryptPublicPart encrypts a flip's public part to the epoch public key, the
// way the node's PrepareFlip does.
func (k *EpochKeys) EncryptPublicPart(plaintext []byte) ([]byte, error) {
	return ecies.Encrypt(rand.Reader, &k.publicPair.PublicKey, plaintext, nil, nil)
}

// EncryptPrivatePart encrypts a flip's private part to the epoch private key,
// which only assigned validators receive.
func (k *EpochKeys) EncryptPrivatePart(plaintext []byte) ([]byte, error) {
	return ecies.Encrypt(rand.Reader, &k.privatePair.PublicKey, plaintext, nil, nil)
}

// DecryptPublicPart decrypts a flip public part with the epoch public key pair.
func (k *EpochKeys) DecryptPublicPart(ciphertext []byte) ([]byte, error) {
	return k.publicPair.Decrypt(ciphertext, nil, nil)
}

// DecryptPrivatePart decrypts a flip private part with the epoch private key
// pair. This is the part that is only shared with assigned validators.
func (k *EpochKeys) DecryptPrivatePart(ciphertext []byte) ([]byte, error) {
	return k.privatePair.Decrypt(ciphertext, nil, nil)
}

// fingerprintBytes is a short, non-reversible identifier for public material.
func fingerprintBytes(data []byte) string {
	hash := crypto.Hash(data)
	return fmt.Sprintf("%x", hash[:8])
}

// SecretBytes returns the private halves of both epoch keys.
//
// This is the material a bundle actually sells, so callers must route it to
// encrypted storage. It exists as an explicit method so that nothing writes
// secrets by accident while serialising the public summary.
func (k *EpochKeys) SecretBytes() (publicPartKey, privatePartKey []byte) {
	return crypto.FromECDSA(k.publicPair.ExportECDSA()), crypto.FromECDSA(k.privatePair.ExportECDSA())
}

// VaultPayload is the JSON structure stored inside an encrypted vault. It is
// the only place private key material is persisted.
type VaultPayload struct {
	Kind           string `json:"kind"`
	Version        int    `json:"version"`
	Epoch          uint16 `json:"epoch"`
	Address        string `json:"address"`
	PublicPartKey  string `json:"publicPartEncryptionPublicKey"`
	PrivatePartKey string `json:"privatePartEncryptionPublicKey"`
	PublicPartFP   string `json:"publicPartKeyFingerprint"`
	PrivatePartFP  string `json:"privatePartKeyFingerprint"`
	PublicPartSec  string `json:"publicPartPrivateKey"`
	PrivatePartSec string `json:"privatePartPrivateKey"`
}

// ToVaultPayload packages the derived keys for encrypted storage.
func (k *EpochKeys) ToVaultPayload() VaultPayload {
	publicSecret, privateSecret := k.SecretBytes()
	return VaultPayload{
		Kind:           "fod.epoch-keys",
		Version:        1,
		Epoch:          k.Epoch,
		Address:        k.Address,
		PublicPartKey:  k.PublicPartKey,
		PrivatePartKey: k.PrivatePartKey,
		PublicPartFP:   k.PublicPartFP,
		PrivatePartFP:  k.PrivatePartFP,
		PublicPartSec:  fmt.Sprintf("%x", publicSecret),
		PrivatePartSec: fmt.Sprintf("%x", privateSecret),
	}
}

// EpochKeysFromVaultPayload rebuilds the decrypting key pairs from a vault
// payload, so a buyer can open a delivery without the identity key.
func EpochKeysFromVaultPayload(payload VaultPayload) (*EpochKeys, error) {
	publicSecret, err := hexToECDSA(payload.PublicPartSec)
	if err != nil {
		return nil, fmt.Errorf("public-part secret: %w", err)
	}
	privateSecret, err := hexToECDSA(payload.PrivatePartSec)
	if err != nil {
		return nil, fmt.Errorf("private-part secret: %w", err)
	}
	return &EpochKeys{
		Epoch:          payload.Epoch,
		Address:        payload.Address,
		PublicPartKey:  payload.PublicPartKey,
		PrivatePartKey: payload.PrivatePartKey,
		PublicPartFP:   payload.PublicPartFP,
		PrivatePartFP:  payload.PrivatePartFP,
		publicPair:     ecies.ImportECDSA(publicSecret),
		privatePair:    ecies.ImportECDSA(privateSecret),
	}, nil
}

// hexToECDSA converts a hex private key string into an ECDSA key.
func hexToECDSA(hexKey string) (*ecdsa.PrivateKey, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(hexKey, "0x"))
	if err != nil {
		return nil, err
	}
	return crypto.ToECDSA(raw)
}

// ParseFlipBlob splits a stored flip blob into its parts.
//
// Wire format (idena-go/core/flip/flipper.go): a protobuf message with the
// author's identity public key, the ECIES-encrypted public part, and the
// ECIES-encrypted private part.
func ParseFlipBlob(blob []byte) (publicKey, publicPart, privatePart []byte, err error) {
	var seen [4]bool
	for remaining := blob; len(remaining) > 0; {
		field, wire, size := protowire.ConsumeField(remaining)
		if size <= 0 || field > protowire.MaxValidNumber ||
			(wire != protowire.VarintType && wire != protowire.Fixed64Type &&
				wire != protowire.BytesType && wire != protowire.Fixed32Type) {
			return nil, nil, nil, fmt.Errorf("flip blob has a malformed protobuf field")
		}
		if field >= 1 && field <= 3 {
			if wire != protowire.BytesType || seen[field] {
				return nil, nil, nil, fmt.Errorf("flip blob has a repeated or invalid required field %d", field)
			}
			seen[field] = true
		}
		remaining = remaining[size:]
	}
	flip := new(models.ProtoIpfsFlip)
	if err := proto.Unmarshal(blob, flip); err != nil {
		return nil, nil, nil, fmt.Errorf("parse flip blob: %w", err)
	}
	// A protobuf with no fields at all parses cleanly - an empty file does - and
	// every part is required: the node writes the author's public key and both
	// ciphertext parts. Refusing here is what makes a malformed blob a clear
	// error rather than a decryption failure that reads like a key mismatch.
	if len(flip.PubKey) == 0 || len(flip.PublicPart) == 0 || len(flip.PrivatePart) == 0 {
		return nil, nil, nil, fmt.Errorf(
			"the flip blob is missing a part: %d bytes of public key, %d of public part, %d of private part",
			len(flip.PubKey), len(flip.PublicPart), len(flip.PrivatePart))
	}
	return flip.PubKey, flip.PublicPart, flip.PrivatePart, nil
}

// BuildFlipBlob writes the same wire format, used for tests that need a blob
// with known contents.
func BuildFlipBlob(identityPublicKey, publicPart, privatePart []byte) ([]byte, error) {
	flip := &models.ProtoIpfsFlip{
		PubKey:      identityPublicKey,
		PublicPart:  publicPart,
		PrivatePart: privatePart,
	}
	return proto.Marshal(flip)
}
