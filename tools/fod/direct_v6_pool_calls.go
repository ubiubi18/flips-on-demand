package main

// Fixed-width calldata for contracts/direct-escrow-v6. These helpers only
// encode arguments; payable amounts and contract state are checked elsewhere.

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/idena-network/idena-go/crypto"
)

const (
	directV6PoolDeployArgBytes  = 32 + 32
	directV6PoolSeedArgBytes    = 32 + 106
	directV6PoolListArgBytes    = 2 + 4 + 32 + 32 + 16
	directV6PoolBuyArgBytes     = 32 + 65
	directV6PoolConfirmArgBytes = 8 + 32
	directV6PoolRevealArgBytes  = 8 + 106
	directV6PoolSecretsBytes    = 106
	directV6PoolSecretsDomain   = "fod:epoch-secrets:v1"
)

func EncodeDirectV6PoolDeploySeedIDs(first, second [32]byte) ([]byte, error) {
	if first == ([32]byte{}) || second == ([32]byte{}) || first == second {
		return nil, errors.New("v6 pool bootstrap needs two distinct nonzero seed offer IDs")
	}
	out := make([]byte, directV6PoolDeployArgBytes)
	copy(out[:32], first[:])
	copy(out[32:], second[:])
	return out, nil
}

// Activation repeats the pinned deploy IDs and carries the 10,000 iDNA seed
// as transaction value after both public seed bundles have been proved.
func EncodeDirectV6PoolActivateSeedIDs(first, second [32]byte) ([]byte, error) {
	return EncodeDirectV6PoolDeploySeedIDs(first, second)
}

// Seed bundles are intentionally public. Their canonical epoch secrets must
// belong to the pinned offer before they are placed in seed calldata.
func EncodeDirectV6PoolSeedBundle(offerID [32]byte, secrets []byte) ([]byte, error) {
	if offerID == ([32]byte{}) || len(secrets) != directV6PoolSecretsBytes ||
		string(secrets[:len(directV6PoolSecretsDomain)]) != directV6PoolSecretsDomain {
		return nil, errors.New("v6 pool seed needs a nonzero offer ID and canonical epoch secrets")
	}
	var author [20]byte
	copy(author[:], secrets[len(directV6PoolSecretsDomain)+2:len(directV6PoolSecretsDomain)+22])
	epoch := binary.BigEndian.Uint16(secrets[len(directV6PoolSecretsDomain) : len(directV6PoolSecretsDomain)+2])
	if poolV6OfferID(author, epoch) != offerID {
		return nil, errors.New("v6 pool seed secrets belong to another offer")
	}
	out := make([]byte, directV6PoolSeedArgBytes)
	copy(out[:32], offerID[:])
	copy(out[32:], secrets)
	return out, nil
}

func EncodeDirectV6PoolBuyPublicSeed(offerID [32]byte) ([]byte, error) {
	return directV6PoolOfferArg(offerID)
}

// DirectV6PoolListArgs mirrors the v6 list argument. The author and posted
// bond are transaction fields, not calldata fields.
type DirectV6PoolListArgs struct {
	Epoch         uint16
	Covered       uint32
	Inventory     [32]byte
	Keys          [32]byte
	MinimumPayout *big.Int
}

func EncodeDirectV6PoolList(args DirectV6PoolListArgs) ([]byte, error) {
	if args.Covered == 0 || args.Covered > 32 || args.Inventory == ([32]byte{}) ||
		args.Keys == ([32]byte{}) || args.MinimumPayout == nil ||
		args.MinimumPayout.Sign() < 0 || args.MinimumPayout.BitLen() > 128 {
		return nil, errors.New("v6 pool list needs 1 to 32 flips, nonzero commitments, and a u128 minimum payout")
	}
	out := make([]byte, directV6PoolListArgBytes)
	binary.BigEndian.PutUint16(out[:2], args.Epoch)
	binary.BigEndian.PutUint32(out[2:6], args.Covered)
	copy(out[6:38], args.Inventory[:])
	copy(out[38:70], args.Keys[:])
	args.MinimumPayout.FillBytes(out[70:86])
	return out, nil
}

func directV6PoolOfferArg(offerID [32]byte) ([]byte, error) {
	if offerID == ([32]byte{}) {
		return nil, errors.New("v6 pool offer ID must be nonzero")
	}
	out := make([]byte, 32)
	copy(out, offerID[:])
	return out, nil
}

// The amount added to the seller bond is the payable transaction amount.
func EncodeDirectV6PoolTopUpBond(offerID [32]byte) ([]byte, error) {
	return directV6PoolOfferArg(offerID)
}

func EncodeDirectV6PoolBuy(offerID [32]byte, deliveryKey []byte) ([]byte, error) {
	if offerID == ([32]byte{}) || len(deliveryKey) != 65 || deliveryKey[0] != 4 {
		return nil, errors.New("v6 pool buy needs an offer ID and 65-byte uncompressed delivery key")
	}
	if _, err := crypto.UnmarshalPubkey(deliveryKey); err != nil {
		return nil, errors.New("v6 pool delivery key is not a secp256k1 point")
	}
	out := make([]byte, directV6PoolBuyArgBytes)
	copy(out[:32], offerID[:])
	copy(out[32:], deliveryKey)
	return out, nil
}

func directV6PoolOrderArg(orderID uint64) ([]byte, error) {
	if orderID == 0 {
		return nil, errors.New("v6 pool order ID must be positive")
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, orderID)
	return out, nil
}

func EncodeDirectV6PoolChallenge(orderID uint64) ([]byte, error) {
	return directV6PoolOrderArg(orderID)
}

// Confirm hashes the exact ciphertext the buyer checked locally.
func EncodeDirectV6PoolConfirm(orderID uint64, ciphertext []byte) ([]byte, error) {
	if orderID == 0 || len(ciphertext) == 0 {
		return nil, errors.New("v6 pool confirm needs an order ID and ciphertext")
	}
	digest := sha256.Sum256(ciphertext)
	out := make([]byte, directV6PoolConfirmArgBytes)
	binary.BigEndian.PutUint64(out[:8], orderID)
	copy(out[8:], digest[:])
	return out, nil
}

// Only the short form for an already pre-proved offer is encoded here.
func EncodeDirectV6PoolStartFallback(orderID uint64) ([]byte, error) {
	return directV6PoolOrderArg(orderID)
}

func EncodeDirectV6PoolProveAndReveal(orderID uint64, secrets []byte) ([]byte, error) {
	if orderID == 0 || len(secrets) != directV6PoolSecretsBytes ||
		string(secrets[:len(directV6PoolSecretsDomain)]) != directV6PoolSecretsDomain {
		return nil, errors.New("v6 pool reveal needs an order ID and 106-byte canonical epoch secrets")
	}
	out := make([]byte, directV6PoolRevealArgBytes)
	binary.BigEndian.PutUint64(out[:8], orderID)
	copy(out[8:], secrets)
	return out, nil
}

func EncodeDirectV6PoolRefund(orderID uint64) ([]byte, error) {
	return directV6PoolOrderArg(orderID)
}
