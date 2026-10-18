package main

// Exact v7 contract arguments. These encoders do not authorize a payment or
// transaction; the caller and current state are checked at preparation/signing.

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/idena-network/idena-go/crypto"
)

const (
	poolV7OfferDomain = "fod:pool-offer:v7"
	poolV7SeedAmount  = "10000000000000000000000" // 10,000 iDNA base units.
	poolV7PriceCap    = "1000000000000000000000"  // 1,000 iDNA base units.
)

func poolV7OfferID(author [20]byte, epoch uint16) [32]byte {
	var epochBE [2]byte
	binary.BigEndian.PutUint16(epochBE[:], epoch)
	preimage := make([]byte, 0, len(poolV7OfferDomain)+len(author)+2)
	preimage = append(preimage, poolV7OfferDomain...)
	preimage = append(preimage, author[:]...)
	preimage = append(preimage, epochBE[:]...)
	return sha256.Sum256(preimage)
}

func encodePoolV7SeedID(id [32]byte) ([]byte, error) {
	if id == ([32]byte{}) {
		return nil, errors.New("v7 seed offer ID must be nonzero")
	}
	return append([]byte(nil), id[:]...), nil
}

func encodePoolV7Relist(id [32]byte, minimum *big.Int) ([]byte, error) {
	if id == ([32]byte{}) || minimum == nil || minimum.Sign() < 0 || minimum.BitLen() > 128 {
		return nil, errors.New("v7 relist needs an offer ID and u128 minimum payout")
	}
	out := make([]byte, 48)
	copy(out[:32], id[:])
	minimum.FillBytes(out[32:])
	return out, nil
}

func encodePoolV7Qualify(epoch uint16, covered uint32, inventory, keys [32]byte, wire []byte) ([]byte, error) {
	if covered == 0 || covered > 32 || inventory == ([32]byte{}) || keys == ([32]byte{}) ||
		len(wire) == 0 || len(wire) > fastRegistrationMaxWireBytes {
		return nil, errors.New("v7 qualification needs valid metadata and bounded session wire")
	}
	out := make([]byte, 72+len(wire))
	binary.BigEndian.PutUint16(out[:2], epoch)
	binary.BigEndian.PutUint32(out[2:6], covered)
	copy(out[6:38], inventory[:])
	copy(out[38:70], keys[:])
	binary.BigEndian.PutUint16(out[70:72], uint16(len(wire)))
	copy(out[72:], wire)
	return out, nil
}

func encodePoolV7ProveBundle(id [32]byte, journal, seal, wire []byte) ([]byte, error) {
	if id == ([32]byte{}) || len(journal) != fastContentJournalBytes ||
		len(seal) != fastContentSealBytes || len(wire) == 0 || len(wire) > fastRegistrationMaxWireBytes {
		return nil, errors.New("v7 proveBundle needs an offer ID, sealed proof, and session wire")
	}
	out := make([]byte, 32+fastContentJournalBytes+fastContentSealBytes+2+len(wire))
	copy(out[:32], id[:])
	copy(out[32:], journal)
	copy(out[32+fastContentJournalBytes:], seal)
	at := 32 + fastContentJournalBytes + fastContentSealBytes
	binary.BigEndian.PutUint16(out[at:at+2], uint16(len(wire)))
	copy(out[at+2:], wire)
	return out, nil
}

func poolV7SecretsOfferID(secrets []byte) ([32]byte, [32]byte, error) {
	var id, digest [32]byte
	if len(secrets) != 106 || string(secrets[:len(directSecretsDomain)]) != directSecretsDomain {
		return id, digest, errors.New("v7 requires 106-byte canonical epoch secrets")
	}
	var author [20]byte
	at := len(directSecretsDomain)
	epoch := binary.BigEndian.Uint16(secrets[at : at+2])
	copy(author[:], secrets[at+2:at+22])
	id = poolV7OfferID(author, epoch)
	digest = sha256.Sum256(secrets)
	return id, digest, nil
}

func encodePoolV7SeedBundle(id [32]byte, secrets []byte) ([]byte, error) {
	derived, _, err := poolV7SecretsOfferID(secrets)
	if err != nil || id == ([32]byte{}) || id != derived {
		return nil, errors.New("v7 seed secrets differ from the offer ID")
	}
	out := make([]byte, 32+106)
	copy(out[:32], id[:])
	copy(out[32:], secrets)
	return out, nil
}

func encodePoolV7List(epoch uint16, covered uint32, inventory, keys [32]byte, minimum *big.Int) ([]byte, error) {
	if covered == 0 || covered > 32 || inventory == ([32]byte{}) || keys == ([32]byte{}) ||
		minimum == nil || minimum.Sign() < 0 || minimum.BitLen() > 128 {
		return nil, errors.New("v7 list needs valid metadata and a u128 minimum payout")
	}
	out := make([]byte, 86)
	binary.BigEndian.PutUint16(out[:2], epoch)
	binary.BigEndian.PutUint32(out[2:6], covered)
	copy(out[6:38], inventory[:])
	copy(out[38:70], keys[:])
	minimum.FillBytes(out[70:86])
	return out, nil
}

func encodePoolV7Buy(id [32]byte, key []byte) ([]byte, error) {
	if id == ([32]byte{}) || len(key) != 65 || key[0] != 4 {
		return nil, errors.New("v7 buy needs an expected offer ID and uncompressed delivery key")
	}
	if _, err := crypto.UnmarshalPubkey(key); err != nil {
		return nil, errors.New("v7 delivery key is not a secp256k1 point")
	}
	out := make([]byte, 97)
	copy(out[:32], id[:])
	copy(out[32:], key)
	return out, nil
}

func encodePoolV7OrderID(id uint64) ([]byte, error) {
	if id == 0 {
		return nil, errors.New("v7 order ID must be positive")
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, id)
	return out, nil
}

func encodePoolV7Confirm(id uint64, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) != directDMCiphertextBytes {
		return nil, errors.New("v7 confirmation needs the exact private delivery ciphertext")
	}
	arg, err := encodePoolV7OrderID(id)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(ciphertext)
	return append(arg, digest[:]...), nil
}

func encodePoolV7ProveAndReveal(id uint64, secrets []byte) ([]byte, error) {
	if _, _, err := poolV7SecretsOfferID(secrets); err != nil {
		return nil, err
	}
	out, err := encodePoolV7OrderID(id)
	if err != nil {
		return nil, err
	}
	return append(out, secrets...), nil
}
