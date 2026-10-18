package main

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

type blobVector struct {
	Note                 string `json:"note"`
	Nodekey              string `json:"nodekey"`
	Author               string `json:"author"`
	Epoch                uint16 `json:"epoch"`
	PublicPartSecret     string `json:"publicPartSecret"`
	PrivatePartSecret    string `json:"privatePartSecret"`
	PublicPartPlaintext  string `json:"publicPartPlaintext"`
	PrivatePartPlaintext string `json:"privatePartPlaintext"`
	Blob                 string `json:"blob"`
	BlobCID              string `json:"blobCid"`
	SecretsCanonical     string `json:"secretsCanonical"`
	SecretsCommitment    string `json:"secretsCommitment"`
}

func readBlobVector(t *testing.T) blobVector {
	t.Helper()
	raw, err := os.ReadFile("../../proofs/registration/vectors/blob-v1.json")
	if err != nil {
		t.Fatalf("read the blob vector: %v", err)
	}
	var vector blobVector
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatalf("parse the blob vector: %v", err)
	}
	return vector
}

// This frozen blob was built with a node revision that used a different seed
// mapping. Recorded secrets must still open it, but it is not a derivation
// vector for the node revision used to build the current CLI.
func TestHistoricalNodeBlobOpensWithRecordedKeys(t *testing.T) {
	vector := readBlobVector(t)
	nodekey, err := hex.DecodeString(vector.Nodekey)
	if err != nil {
		t.Fatalf("vector nodekey: %v", err)
	}
	identity, err := crypto.ToECDSA(nodekey)
	if err != nil {
		t.Fatalf("vector nodekey is not a key: %v", err)
	}
	keys, err := EpochKeysFromVaultPayload(VaultPayload{
		Epoch:          vector.Epoch,
		PublicPartSec:  vector.PublicPartSecret,
		PrivatePartSec: vector.PrivatePartSecret,
	})
	if err != nil {
		t.Fatalf("import the recorded epoch keys: %v", err)
	}
	publicSecret, privateSecret := keys.SecretBytes()
	if author := crypto.PubkeyToAddress(identity.PublicKey).Bytes(); hex.EncodeToString(author) != vector.Author {
		t.Fatalf("the derived author is %x, the vector says %s", author, vector.Author)
	}

	blob, err := hex.DecodeString(vector.Blob)
	if err != nil {
		t.Fatalf("vector blob: %v", err)
	}
	_, publicCipher, privateCipher, err := ParseFlipBlob(blob)
	if err != nil {
		t.Fatalf("parse the blob: %v", err)
	}
	publicPlain, err := keys.DecryptPublicPart(publicCipher)
	if err != nil {
		t.Fatalf("decrypt the public part: %v", err)
	}
	privatePlain, err := keys.DecryptPrivatePart(privateCipher)
	if err != nil {
		t.Fatalf("decrypt the private part: %v", err)
	}
	if hex.EncodeToString(publicPlain) != vector.PublicPartPlaintext {
		t.Fatalf("the public part is %x, the vector says %s", publicPlain, vector.PublicPartPlaintext)
	}
	if hex.EncodeToString(privatePlain) != vector.PrivatePartPlaintext {
		t.Fatalf("the private part is %x, the vector says %s", privatePlain, vector.PrivatePartPlaintext)
	}

	// The CID the manifest will carry, and the commitment the journal binds.
	if got := cidRawSha256(blob); got != vector.BlobCID {
		t.Fatalf("the blob's CID is %s, the vector says %s", got, vector.BlobCID)
	}
	authorBytes, err := hex.DecodeString(vector.Author)
	if err != nil {
		t.Fatalf("vector author: %v", err)
	}
	canonical, err := SecretsCanonicalBytes(vector.Epoch, authorBytes, publicSecret, privateSecret)
	if err != nil {
		t.Fatalf("canonical secrets: %v", err)
	}
	if hex.EncodeToString(canonical) != vector.SecretsCanonical {
		t.Fatalf("the canonical secrets are %x, the vector says %s", canonical, vector.SecretsCanonical)
	}
	commitment, err := SecretsCommitmentHex(vector.Epoch, authorBytes, publicSecret, privateSecret)
	if err != nil {
		t.Fatalf("commitment: %v", err)
	}
	if commitment != vector.SecretsCommitment {
		t.Fatalf("the commitment is %s, the vector says %s", commitment, vector.SecretsCommitment)
	}
}
