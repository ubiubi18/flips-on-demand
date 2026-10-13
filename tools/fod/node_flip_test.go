package main

// Package 3's exit condition asks for independent vectors, and this is the
// strongest one available: a flip a live node encrypted for itself.
//
// `proofs/registration/vectors/node-flip-v1.json` holds a blob a lab node
// produced through its own `flip_submit`, fetched back from that node's IPFS by
// the CID it reported, together with the two plaintext parts that were handed
// to it. Nothing in the vector came from this repository's derivation: the node
// ran its own composition (seed text, hash, signature, seeded key, ECIES) and
// this test asks whether the CLI's derivation opens it.
//
// The nodekey is the local lab key and is deliberately not in the vector, so the
// test skips where it is absent rather than carrying a private key in the
// repository.

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

type nodeFlipVector struct {
	Note                 string `json:"note"`
	Address              string `json:"address"`
	PubKey               string `json:"pubKey"`
	Epoch                uint16 `json:"epoch"`
	BlobCid              string `json:"blobCid"`
	Blob                 string `json:"blob"`
	PublicPartPlaintext  string `json:"publicPartPlaintext"`
	PrivatePartPlaintext string `json:"privatePartPlaintext"`
	KeyFile              string `json:"keyFile"`
}

func TestANodeMadeFlipOpensWithDerivedKeys(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/registration/vectors/node-flip-v1.json")
	if err != nil {
		t.Fatalf("read the vector: %v", err)
	}
	var vector nodeFlipVector
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatalf("parse the vector: %v", err)
	}
	if vector.KeyFile == "" || vector.BlobCid == "" {
		t.Fatal("the vector does not say where the key is or which blob it is")
	}

	keyPath := "../../" + vector.KeyFile
	if _, err := os.Stat(keyPath); err != nil {
		t.Skipf("the lab nodekey is not present at %s (%v)", vector.KeyFile, err)
	}
	identity, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		t.Fatalf("load the lab nodekey: %v", err)
	}
	keys, err := DeriveEpochKeys(identity, vector.Epoch)
	if err != nil {
		t.Fatalf("derive epoch keys: %v", err)
	}
	derivedAddress := strings.ToLower(strings.TrimPrefix(keys.Address, "0x"))
	if want := strings.ToLower(strings.TrimPrefix(vector.Address, "0x")); derivedAddress != want {
		t.Fatalf("the derived address is %s, the vector says %s", derivedAddress, want)
	}

	blob, err := hex.DecodeString(vector.Blob)
	if err != nil {
		t.Fatalf("decode the blob: %v", err)
	}
	authorPubKey, publicCipher, privateCipher, err := ParseFlipBlob(blob)
	if err != nil {
		t.Fatalf("parse the node's blob: %v", err)
	}
	if got := hex.EncodeToString(authorPubKey); got != vector.PubKey {
		t.Fatalf("the blob carries author key %s, the vector says %s", got, vector.PubKey)
	}
	publicPlain, err := keys.DecryptPublicPart(publicCipher)
	if err != nil {
		t.Fatalf("decrypt the public part with the derived keys: %v", err)
	}
	privatePlain, err := keys.DecryptPrivatePart(privateCipher)
	if err != nil {
		t.Fatalf("decrypt the private part with the derived keys: %v", err)
	}
	if got := hex.EncodeToString(publicPlain); got != vector.PublicPartPlaintext {
		t.Fatalf("the public part opened to %s, the node was given %s", got, vector.PublicPartPlaintext)
	}
	if got := hex.EncodeToString(privatePlain); got != vector.PrivatePartPlaintext {
		t.Fatalf("the private part opened to %s, the node was given %s", got, vector.PrivatePartPlaintext)
	}
}
