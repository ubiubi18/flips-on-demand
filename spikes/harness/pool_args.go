package harness

// Pool order arguments, kept beside the offer slice's helpers so the tests and
// the contract cannot drift on the encoding.
//
// `buy` argument: bundle commitment (32 bytes) || delivery key (65 bytes,
// uncompressed secp256k1) || possession signature (65 bytes, R || S || V).
//
// Possession proof: the buyer signs the Keccak-256 digest of domain ||
// contract || bundle || buy nonce || caller || delivery key, which is the
// message `pool_delivery_key_message` rebuilds and verifies with `ecrecover`.

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	gethtcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/idena-network/idena-wasm-binding/lib"
)

// poolBuyMessage mirrors the contract's pool purchase context.
func poolBuyMessage(contract lib.Address, bundle []byte, nonce uint64, caller lib.Address, deliveryKey []byte) []byte {
	msg := []byte(deliveryKeyDomain)
	msg = append(msg, contract[:]...)
	msg = append(msg, bundle...)
	msg = append(msg, u64be(nonce)...)
	msg = append(msg, caller[:]...)
	msg = append(msg, deliveryKey...)
	return msg
}

// SignPoolBuy signs the delivery-key possession proof for one pool purchase.
func SignPoolBuy(t *testing.T, contract lib.Address, bundle []byte, nonce uint64, caller lib.Address, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	digest := gethtcrypto.Keccak256(poolBuyMessage(contract, bundle, nonce, caller, DeliveryKeyBytes(key)))
	sig, err := gethtcrypto.Sign(digest, key)
	if err != nil {
		t.Fatalf("sign pool buy proof: %v", err)
	}
	return sig
}

// PoolBuyArgs builds the `buy` argument from the bundle, delivery key, and
// possession signature.
func PoolBuyArgs(bundle, deliveryKey, signature []byte) []byte {
	// The buy call ends with a two-byte retrieval-fee count (zero here) and the
	// fee entries that follow it; the harness's purchases name no hosts.
	arg := make([]byte, 0, len(bundle)+deliveryKeyBytes+signatureBytes+2)
	arg = append(arg, bundle...)
	arg = append(arg, deliveryKey...)
	arg = append(arg, signature...)
	arg = append(arg, 0, 0)
	return arg
}

// PoolBuyArgsWithFees appends a retrieval-fee list to the buy call: one 20-byte
// host address and a big-endian u128 amount per entry, preceded by the count.
func PoolBuyArgsWithFees(bundle, deliveryKey, signature []byte, hosts [][20]byte, amounts []*big.Int) []byte {
	arg := make([]byte, 0, len(bundle)+deliveryKeyBytes+signatureBytes+2+len(hosts)*36)
	arg = append(arg, bundle...)
	arg = append(arg, deliveryKey...)
	arg = append(arg, signature...)
	arg = append(arg, byte(len(hosts)>>8), byte(len(hosts)))
	for index, host := range hosts {
		arg = append(arg, host[:]...)
		amount := make([]byte, 16)
		amounts[index].FillBytes(amount)
		arg = append(arg, amount...)
	}
	return arg
}
