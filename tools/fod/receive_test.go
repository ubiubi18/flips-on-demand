package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

// TestReceiveFlipEndToEnd walks the buyer path with synthetic material: derive
// keys, encrypt a blob, store the keys in a vault, then decrypt through the same
// function the CLI uses.
func TestReceiveFlipEndToEnd(t *testing.T) {
	dir := t.TempDir()
	passphrase := "vault-passphrase"

	authorKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate author key: %v", err)
	}
	keys, err := DeriveEpochKeys(authorKey, 204)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	publicPlain := []byte("the public part of the flip")
	privatePlain := []byte("the private part of the flip")
	publicCipher, err := keys.EncryptPublicPart(publicPlain)
	if err != nil {
		t.Fatalf("encrypt public part: %v", err)
	}
	privateCipher, err := keys.EncryptPrivatePart(privatePlain)
	if err != nil {
		t.Fatalf("encrypt private part: %v", err)
	}
	blob, err := BuildFlipBlob(crypto.FromECDSAPub(&authorKey.PublicKey), publicCipher, privateCipher)
	if err != nil {
		t.Fatalf("build blob: %v", err)
	}

	blobPath := filepath.Join(dir, "flip.blob")
	if err := os.WriteFile(blobPath, blob, 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}
	payload, err := json.Marshal(keys.ToVaultPayload())
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	vaultPath := filepath.Join(dir, "bundle.age")
	if err := WriteVault(vaultPath, passphrase, payload); err != nil {
		t.Fatalf("write vault: %v", err)
	}

	outPath := filepath.Join(dir, "decrypted")
	// The outputs may exist wider from an earlier run or another tool; the
	// write has to tighten them (`AH-20`).
	for _, path := range []string{outPath, outPath + ".private"} {
		if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
			t.Fatalf("pre-create %s: %v", path, err)
		}
	}
	result, err := receiveFlip(vaultPath, blobPath, outPath, passphrase)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if result.PublicPartBytes != len(publicPlain) || result.PrivatePartBytes != len(privatePlain) {
		t.Fatalf("unexpected part sizes: %+v", result)
	}

	gotPublic, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read decrypted public part: %v", err)
	}
	if string(gotPublic) != string(publicPlain) {
		t.Fatal("decrypted public part does not match")
	}
	gotPrivate, err := os.ReadFile(outPath + ".private")
	if err != nil {
		t.Fatalf("read decrypted private part: %v", err)
	}
	if string(gotPrivate) != string(privatePlain) {
		t.Fatal("decrypted private part does not match")
	}
	for _, path := range []string{outPath, outPath + ".private"} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s is mode %o after receive, want 0600", path, info.Mode().Perm())
		}
	}
}

// TestReceiveFlipRejectsWrongVault ensures the receive path fails closed when
// the vault does not hold the keys for the blob.
func TestReceiveFlipRejectsWrongVault(t *testing.T) {
	dir := t.TempDir()
	passphrase := "shared-passphrase"

	authorKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate author key: %v", err)
	}
	keys, err := DeriveEpochKeys(authorKey, 204)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	publicCipher, err := keys.EncryptPublicPart([]byte("public"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	privateCipher, err := keys.EncryptPrivatePart([]byte("private"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	blob, err := BuildFlipBlob(crypto.FromECDSAPub(&authorKey.PublicKey), publicCipher, privateCipher)
	if err != nil {
		t.Fatalf("build blob: %v", err)
	}
	blobPath := filepath.Join(dir, "flip.blob")
	if err := os.WriteFile(blobPath, blob, 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}

	otherKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	otherKeys, err := DeriveEpochKeys(otherKey, 204)
	if err != nil {
		t.Fatalf("derive other: %v", err)
	}
	payload, err := json.Marshal(otherKeys.ToVaultPayload())
	if err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	vaultPath := filepath.Join(dir, "other.age")
	if err := WriteVault(vaultPath, passphrase, payload); err != nil {
		t.Fatalf("write vault: %v", err)
	}

	if _, err := receiveFlip(vaultPath, blobPath, "", passphrase); err == nil {
		t.Fatal("a vault with the wrong epoch keys decrypted the blob")
	}
}
