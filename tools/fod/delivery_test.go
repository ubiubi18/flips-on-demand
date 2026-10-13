package main

// Compatibility check for delivery ECIES v1, from the Go side.
//
// The delivery proof guest (proofs/delivery) encrypts the epoch secrets to the
// buyer's key in Rust. This test takes the ciphertext that run published and
// decrypts it with the repository-local ECIES library of the pinned node, which
// the guest implementation never touches. If the two disagree on the wire
// format, key derivation, IV, or tag, this fails.
//
// The artefact is produced by `cargo run --release --bin delivery-host` in
// proofs/delivery, which takes minutes because it makes a real Groth16 receipt,
// so a missing artefact skips rather than fails.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
)

type deliveryArtifact struct {
	Statement  string `json:"statement"`
	Journal    string `json:"journal"`
	Ciphertext string `json:"ciphertext"`
	Order struct {
		Note              string `json:"note"`
		Buyer             string `json:"buyer"`
		BuyerSecret       string `json:"buyer_secret"`
		BuyerKey          string `json:"buyer_key"`
		PublicPartSecret  string `json:"public_part_secret"`
		PrivatePartSecret string `json:"private_part_secret"`
		Secrets           string `json:"secrets"`
	} `json:"order"`
}

// journalLayoutV1: version (4) || contract (20) || offer id (8) || order nonce
// (8) || seller (20) || buyer (20) || buyer key (65) || commitment (32) ||
// ciphertext hash (32).
const (
	journalBuyerKeyOffset   = 4 + 20 + 8 + 8 + 20 + 20
	journalCommitmentOffset = journalBuyerKeyOffset + 65
	journalBytesV1          = journalCommitmentOffset + 32 + 32
)

func TestDeliveryCiphertextDecryptsWithNodeLibrary(t *testing.T) {
	// Two artefacts: the built-in vector, and one proved from an order file
	// whose terms were never compiled into the prover.
	for _, name := range []string{"delivery.json", "order-example.json"} {
		t.Run(name, func(t *testing.T) {
			checkDeliveryArtefact(t, name)
		})
	}
}

func checkDeliveryArtefact(t *testing.T, name string) {
	t.Helper()
	path := filepath.Join("..", "..", "proofs", "delivery", "dist", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the committed delivery artefact: %v", err)
	}
	var artifact deliveryArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if artifact.Statement != "fod:delivery:v1" {
		t.Fatalf("unexpected statement: %s", artifact.Statement)
	}

	ciphertext, err := hex.DecodeString(artifact.Ciphertext)
	if err != nil {
		t.Fatalf("ciphertext hex: %v", err)
	}
	secrets, err := hex.DecodeString(artifact.Order.Secrets)
	if err != nil {
		t.Fatalf("secrets hex: %v", err)
	}
	journal, err := hex.DecodeString(artifact.Journal)
	if err != nil {
		t.Fatalf("journal hex: %v", err)
	}
	if len(journal) != journalBytesV1 {
		t.Fatalf("journal is %d bytes, expected %d", len(journal), journalBytesV1)
	}

	// The buyer's delivery key in the journal must be the one the host used.
	journalKey := hex.EncodeToString(journal[journalBuyerKeyOffset : journalBuyerKeyOffset+65])
	if journalKey != artifact.Order.BuyerKey {
		t.Fatalf("journal buyer key %s != order key %s", journalKey, artifact.Order.BuyerKey)
	}

	// Decrypt with the node's own library, exactly as a buyer would.
	identity, err := hexToECDSA(artifact.Order.BuyerSecret)
	if err != nil {
		t.Fatalf("buyer secret: %v", err)
	}
	plaintext, err := ecies.ImportECDSA(identity).Decrypt(ciphertext, nil, nil)
	if err != nil {
		t.Fatalf("the node library refused the delivered ciphertext: %v", err)
	}
	if string(plaintext) != string(secrets) {
		t.Fatalf("decrypted %d bytes, expected the delivered secrets (%d bytes)",
			len(plaintext), len(secrets))
	}

	// The buyer can check what they received against the proof's commitment.
	digest := sha256.Sum256(plaintext)
	commitment := journal[journalCommitmentOffset : journalCommitmentOffset+32]
	if fmt.Sprintf("%x", digest[:]) != fmt.Sprintf("%x", commitment) {
		t.Fatalf("decrypted secrets do not match the journal commitment")
	}

	// And the ciphertext hash in the journal must be the published ciphertext.
	hash := sha256.Sum256(ciphertext)
	ciphertextHash := journal[journalCommitmentOffset+32:]
	if fmt.Sprintf("%x", hash[:]) != fmt.Sprintf("%x", ciphertextHash) {
		t.Fatalf("published ciphertext does not match the journal hash")
	}

	// The wrong key must not open the delivery.
	unrelated, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate unrelated key: %v", err)
	}
	if _, err := ecies.ImportECDSA(unrelated).Decrypt(ciphertext, nil, nil); err == nil {
		t.Fatalf("an unrelated key decrypted the delivery")
	}

	t.Logf("delivery decrypted with the repository-local library: %d bytes plaintext, %d bytes ciphertext",
		len(plaintext), len(ciphertext))
}
