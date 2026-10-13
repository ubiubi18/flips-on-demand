package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestVaultReusePreservesOldKeysAndInterruptedWritesDoNotPublish(t *testing.T) {
	path := filepath.Join(t.TempDir(), "purchase.age")
	if err := WriteVault(path, "right", []byte("first purchased keys")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := WriteVault(path, "right", []byte("second purchased keys")); err == nil {
		t.Fatal("replaced the first purchase")
	}
	if err := WriteVault(path, "wrong", []byte("first purchased keys")); err == nil {
		t.Fatal("replaced a vault with another passphrase")
	}
	if err := WriteVault(path, "right", []byte("first purchased keys")); err != nil {
		t.Fatal("existing-key recovery:", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("existing vault bytes changed")
	}
	interrupted := filepath.Join(filepath.Dir(path), "interrupted.age")
	err := writePrivateFile(interrupted, false, func(f *os.File) error {
		f.Write([]byte("incomplete encrypted data"))
		return errors.New("interrupted writer")
	})
	if err == nil {
		t.Fatal("interruption ignored")
	}
	if _, err := os.Stat(interrupted); !os.IsNotExist(err) {
		t.Fatal("partial vault published")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("failed write left staging files")
	}
	jsonPath := filepath.Join(filepath.Dir(path), "author.age")
	indented := []byte("{\n  \"kind\": \"synthetic-keys\",\n  \"privateKey\": \"test-only-key\"\n}")
	if err := WriteVault(jsonPath, "right", indented); err != nil {
		t.Fatal(err)
	}
	jsonBefore, _ := os.ReadFile(jsonPath)
	if err := WriteVault(jsonPath, "right", []byte(`{"privateKey":"test-only-key","kind":"synthetic-keys"}`)); err != nil {
		t.Fatal("formatted existing-key recovery:", err)
	}
	jsonAfter, _ := os.ReadFile(jsonPath)
	if !bytes.Equal(jsonBefore, jsonAfter) {
		t.Fatal("reused JSON vault was rewritten")
	}
}

func TestPrivateWriteDoesNotFollowDestinationSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "keep")
	os.WriteFile(outside, []byte("unchanged"), 0o600)
	link := filepath.Join(dir, "part.private")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateBytes(link, []byte("new private part")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(outside)
	if string(got) != "unchanged" {
		t.Fatal("write followed a symlink outside its directory")
	}
}

// TestVaultRoundTrip covers the property the seller flow needs: secrets survive
// a round trip through the vault, the file is owner-only, and the plaintext is
// not present in the file.
func TestVaultRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.age")
	passphrase := "correct horse battery staple"
	payload := []byte("private epoch key material for a synthetic bundle")

	if err := WriteVault(path, passphrase, payload); err != nil {
		t.Fatalf("write vault: %v", err)
	}

	secure, err := VaultPermissionsSecure(path)
	if err != nil {
		t.Fatalf("stat vault: %v", err)
	}
	if !secure {
		info, _ := os.Stat(path)
		t.Fatalf("vault permissions are not owner-only: %v", info.Mode().Perm())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vault bytes: %v", err)
	}
	if bytes.Contains(raw, payload) {
		t.Fatal("vault file contains the plaintext payload")
	}

	got, err := ReadVault(path, passphrase)
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("vault round trip changed the payload")
	}
}

// TestVaultRejectsWrongPassphrase checks the failure mode, not just the happy
// path: a wrong passphrase must not yield data.
func TestVaultRejectsWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.age")
	if err := WriteVault(path, "right", []byte("payload")); err != nil {
		t.Fatalf("write vault: %v", err)
	}
	if _, err := ReadVault(path, "wrong"); err == nil {
		t.Fatal("wrong passphrase decrypted the vault")
	}
}

// TestVaultRejectsEmptyPassphrase checks that nothing is written with an empty
// passphrase, which would be an unencrypted-in-practice vault.
func TestVaultRejectsEmptyPassphrase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.age")
	if err := WriteVault(path, "", []byte("payload")); err == nil {
		t.Fatal("empty passphrase was accepted")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a file was created despite the rejected passphrase")
	}
}
