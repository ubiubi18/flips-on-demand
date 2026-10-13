package harness

// Shared key material for the harness: the delivery-key encoding and the
// per-test helpers the pool tests build on. The pool's own argument encodings
// live in pool_args.go.

import (
	"crypto/ecdsa"
	"encoding/binary"
	"testing"

	gethtcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/idena-network/idena-wasm-binding/lib"
)

const (
	deliveryKeyDomain = "fod:delivery-key:v1"
	deliveryKeyBytes  = 65
	signatureBytes    = 65
)

// NewDeliveryKey generates a secp256k1 key pair for order tests.
func NewDeliveryKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := gethtcrypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate delivery key: %v", err)
	}
	return key
}

// DeliveryKeyBytes is the uncompressed public key the contract stores.
func DeliveryKeyBytes(key *ecdsa.PrivateKey) []byte {
	return gethtcrypto.FromECDSAPub(&key.PublicKey)
}

func u64be(value uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, value)
	return out
}

// ContractAddr is the address the harness deploys the contract at.
func ContractAddr() lib.Address { return contractAddr }

// hexOf renders bytes the way the contract's order view does.
func hexOf(bytes []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(bytes)*2)
	for _, b := range bytes {
		out = append(out, digits[b>>4], digits[b&0x0f])
	}
	return string(out)
}
