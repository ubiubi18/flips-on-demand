package main

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/secstore"
)

// The CLI's fixed mapping must agree with the exact node source used to build
// it. A node revision with a different mapping is not a compatible build.
func TestDeriveEpochKeysMatchesPinnedNode(t *testing.T) {
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}
	keys, err := DeriveEpochKeys(identity, 204)
	if err != nil {
		t.Fatalf("derive epoch keys: %v", err)
	}
	store := secstore.NewSecStore()
	store.AddKey(crypto.FromECDSA(identity))
	defer store.Destroy()
	publicSecret, privateSecret := keys.SecretBytes()
	for _, check := range []struct {
		seed   string
		actual []byte
	}{
		{"flip-key-for-epoch-204", publicSecret},
		{"flip-private-key-for-epoch-204", privateSecret},
	} {
		hash := common.Hash(crypto.Hash([]byte(check.seed)))
		reference, err := crypto.GenerateKeyFromSeed(bytes.NewReader(store.Sign(hash.Bytes())))
		if err != nil {
			t.Fatalf("reference node key for %s: %v", check.seed, err)
		}
		if !bytes.Equal(check.actual, crypto.FromECDSA(reference)) {
			t.Fatalf("the CLI key mapping differs from the node source for %s", check.seed)
		}
	}
}

func TestProtocolKeyFromSeedMatchesLegacyNodeMapping(t *testing.T) {
	seed := make([]byte, 65)
	for i := range seed {
		seed[i] = byte(i)
	}
	reader := bytes.NewReader(seed)

	key, err := generateProtocolKeyFromSeed(reader)
	if err != nil {
		t.Fatalf("derive protocol key: %v", err)
	}
	const want = "08090a0b0c0d0e0f101259f2ae83a986b98f00bf4a7447eac8c31193e6e1dd68"
	if got := hex.EncodeToString(crypto.FromECDSA(key)); got != want {
		t.Fatalf("protocol seed mapping = %s, want %s", got, want)
	}
	if reader.Len() != 25 {
		t.Fatalf("protocol seed mapping consumed %d bytes, want 40", len(seed)-reader.Len())
	}
}

// TestFlipBlobRoundTrip builds a flip blob the way the node stores one and
// checks that it parses back and decrypts with the derived epoch keys.
//
// Synthetic contents: this verifies the construction and the derive/decrypt
// path, not any claim about a real flip.
func TestFlipBlobRoundTrip(t *testing.T) {
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}
	keys, err := DeriveEpochKeys(identity, 204)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	publicPlaintext := []byte("public part of a synthetic flip")
	privatePlaintext := []byte("private part of a synthetic flip")
	publicCipher, err := keys.EncryptPublicPart(publicPlaintext)
	if err != nil {
		t.Fatalf("encrypt public part: %v", err)
	}
	privateCipher, err := keys.EncryptPrivatePart(privatePlaintext)
	if err != nil {
		t.Fatalf("encrypt private part: %v", err)
	}

	blob, err := BuildFlipBlob(crypto.FromECDSAPub(&identity.PublicKey), publicCipher, privateCipher)
	if err != nil {
		t.Fatalf("build blob: %v", err)
	}
	blobPubKey, blobPublic, blobPrivate, err := ParseFlipBlob(blob)
	if err != nil {
		t.Fatalf("parse blob: %v", err)
	}
	if !bytes.Equal(blobPubKey, crypto.FromECDSAPub(&identity.PublicKey)) {
		t.Fatal("blob identity public key did not survive the round trip")
	}

	gotPublic, err := keys.DecryptPublicPart(blobPublic)
	if err != nil {
		t.Fatalf("decrypt public part of blob: %v", err)
	}
	if !bytes.Equal(gotPublic, publicPlaintext) {
		t.Fatal("public part of the blob did not decrypt")
	}
	gotPrivate, err := keys.DecryptPrivatePart(blobPrivate)
	if err != nil {
		t.Fatalf("decrypt private part of blob: %v", err)
	}
	if !bytes.Equal(gotPrivate, privatePlaintext) {
		t.Fatal("private part of the blob did not decrypt")
	}
}

// TestDeriveEpochKeysIsDeterministic pins the property the seller flow depends
// on: the same identity key and epoch always produce the same two epoch keys,
// and different epochs produce different keys.
func TestDeriveEpochKeysIsDeterministic(t *testing.T) {
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}

	first, err := DeriveEpochKeys(identity, 204)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	second, err := DeriveEpochKeys(identity, 204)
	if err != nil {
		t.Fatalf("derive again: %v", err)
	}
	if first.PublicPartKey != second.PublicPartKey || first.PrivatePartKey != second.PrivatePartKey {
		t.Fatal("same identity key and epoch produced different keys")
	}

	other, err := DeriveEpochKeys(identity, 205)
	if err != nil {
		t.Fatalf("derive for another epoch: %v", err)
	}
	if other.PublicPartKey == first.PublicPartKey {
		t.Fatal("different epochs produced the same public epoch key")
	}
	if first.PublicPartKey == first.PrivatePartKey {
		t.Fatal("the two epoch keys must not be equal")
	}
}

// TestEpochKeysRoundTrip exercises the actual flip encryption shape: the public
// part goes to the epoch public key, the private part to the epoch private key,
// and each decrypts back with the matching derived pair.
func TestEpochKeysRoundTrip(t *testing.T) {
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}
	keys, err := DeriveEpochKeys(identity, 204)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	publicPlaintext := []byte("synthetic public flip part")
	privatePlaintext := []byte("synthetic private flip part")

	publicCipher, err := keys.EncryptPublicPart(publicPlaintext)
	if err != nil {
		t.Fatalf("encrypt public part: %v", err)
	}
	privateCipher, err := keys.EncryptPrivatePart(privatePlaintext)
	if err != nil {
		t.Fatalf("encrypt private part: %v", err)
	}

	gotPublic, err := keys.DecryptPublicPart(publicCipher)
	if err != nil {
		t.Fatalf("decrypt public part: %v", err)
	}
	if !bytes.Equal(gotPublic, publicPlaintext) {
		t.Fatal("public part did not round-trip")
	}

	gotPrivate, err := keys.DecryptPrivatePart(privateCipher)
	if err != nil {
		t.Fatalf("decrypt private part: %v", err)
	}
	if !bytes.Equal(gotPrivate, privatePlaintext) {
		t.Fatal("private part did not round-trip")
	}

	// The two epoch pairs must not be interchangeable: a public-part ciphertext
	// must not open with the private pair.
	if _, err := keys.DecryptPrivatePart(publicCipher); err == nil {
		t.Fatal("public-part ciphertext decrypted with the private epoch key")
	}
}

// `SPEC.md` 11.1's keys row: a malformed flip structure and a corrupted
// ciphertext have to be refused, and refused in a way that says which it is.
func TestMalformedAndCorruptedFlipBlobsAreRefused(t *testing.T) {
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate identity key: %v", err)
	}
	keys, err := DeriveEpochKeys(identity, 204)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	publicCipher, err := keys.EncryptPublicPart([]byte("the public half of a flip"))
	if err != nil {
		t.Fatalf("encrypt public part: %v", err)
	}
	privateCipher, err := keys.EncryptPrivatePart([]byte("the private half of a flip"))
	if err != nil {
		t.Fatalf("encrypt private part: %v", err)
	}
	blob, err := BuildFlipBlob(crypto.FromECDSAPub(&identity.PublicKey), publicCipher, privateCipher)
	if err != nil {
		t.Fatalf("build blob: %v", err)
	}
	if _, _, _, err := ParseFlipBlob(blob); err != nil {
		t.Fatalf("a well-formed blob was refused: %v", err)
	}
	identityPublic := crypto.FromECDSAPub(&identity.PublicKey)
	duplicateIdentity := append(append(append([]byte{}, blob...), 0x0a, byte(len(identityPublic))), identityPublic...)
	if _, _, _, err := ParseFlipBlob(duplicateIdentity); err == nil {
		t.Fatal("a duplicate identity field was accepted although the proof guest rejects it")
	}

	// Bytes that are not a blob at all.
	if _, _, _, err := ParseFlipBlob([]byte{0xff, 0xff, 0xff, 0xff}); err == nil {
		t.Fatal("garbage parsed as a flip blob")
	}
	// An empty payload: valid protobuf, no parts. This one used to parse.
	if _, _, _, err := ParseFlipBlob(nil); err == nil {
		t.Fatal("an empty blob parsed as a flip blob")
	} else if !strings.Contains(err.Error(), "missing a part") {
		t.Fatalf("the refusal does not name the problem: %v", err)
	}

	// A blob whose parts are there but whose ciphertext was altered: the ECIES
	// authentication tag is what has to catch it.
	corrupted := append([]byte{}, publicCipher...)
	corrupted[len(corrupted)/2] ^= 0x01
	if _, err := keys.DecryptPublicPart(corrupted); err == nil {
		t.Fatal("a corrupted public part decrypted")
	}
	shortened := publicCipher[:len(publicCipher)-1]
	if _, err := keys.DecryptPublicPart(shortened); err == nil {
		t.Fatal("a truncated public part decrypted")
	}
}
