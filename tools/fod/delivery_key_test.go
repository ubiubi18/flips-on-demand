package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

// `SPEC.md` 6.5 step 5 asks the buyer to save an encrypted recovery archive of
// the delivery key. The archive holds the same key, opens only under its
// passphrase, and the loader every buyer-side command uses accepts either form.
func TestDeliveryKeyArchiveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want := hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey))

	archivePath := filepath.Join(dir, "delivery-key.age")
	if err := WriteDeliveryKeyArchive(archivePath, "archive-passphrase", key); err != nil {
		t.Fatalf("write the archive: %v", err)
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the archive is %v, expected 0600", info.Mode().Perm())
	}

	restored, err := ReadDeliveryKeyArchive(archivePath, "archive-passphrase")
	if err != nil {
		t.Fatalf("read the archive: %v", err)
	}
	if got := hex.EncodeToString(crypto.FromECDSAPub(&restored.PublicKey)); got != want {
		t.Fatalf("the archive holds %s, expected %s", got, want)
	}
	if _, err := ReadDeliveryKeyArchive(archivePath, "not-the-passphrase"); err == nil {
		t.Fatal("a wrong passphrase opened the archive")
	}

	// The commands take either form: an archive prompts, a plain key does not.
	t.Setenv(PassphraseEnvVar, "archive-passphrase")
	loaded, err := loadDeliveryKey(archivePath)
	if err != nil {
		t.Fatalf("load the archive: %v", err)
	}
	if got := hex.EncodeToString(crypto.FromECDSAPub(&loaded.PublicKey)); got != want {
		t.Fatalf("the loader returned %s, expected %s", got, want)
	}
	t.Setenv(PassphraseEnvVar, "")
	plainPath := filepath.Join(dir, "delivery.key")
	if err := os.WriteFile(plainPath, []byte(hex.EncodeToString(crypto.FromECDSA(key))), 0o600); err != nil {
		t.Fatalf("write the plain key: %v", err)
	}
	if _, err := loadDeliveryKey(plainPath); err != nil {
		t.Fatalf("the plain key needed a passphrase: %v", err)
	}

	// An archive that is not a delivery key is refused by name.
	otherPath := filepath.Join(dir, "epoch-keys.age")
	if err := WriteVault(otherPath, "archive-passphrase", []byte(`{"kind":"fod.epoch-keys","version":1}`)); err != nil {
		t.Fatalf("write the other vault: %v", err)
	}
	if _, err := ReadDeliveryKeyArchive(otherPath, "archive-passphrase"); err == nil ||
		!strings.Contains(err.Error(), "not a delivery key") {
		t.Fatalf("an epoch-key vault was accepted: %v", err)
	}
}
