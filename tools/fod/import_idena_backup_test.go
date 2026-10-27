package main

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func TestImportIdenaBackup(t *testing.T) {
	keyBytes := make([]byte, 32)
	keyBytes[31] = 1 // Synthetic test key; never use it for an identity.
	const passphrase = "test-only-mobile-passphrase"
	sealed, err := crypto.Encrypt(keyBytes, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	backupHex := hex.EncodeToString(sealed)
	writeBackup := func(t *testing.T, contents string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "mobile-backup")
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	newOut := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(dir, "nodekey")
	}
	assertNoSecrets := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("import unexpectedly succeeded")
		}
		for _, secret := range []string{passphrase, backupHex, hex.EncodeToString(keyBytes)} {
			if strings.Contains(err.Error(), secret) {
				t.Fatal("import error disclosed secret material")
			}
		}
	}

	t.Run("valid mobile backup becomes owner-only nodekey", func(t *testing.T) {
		t.Setenv(PassphraseEnvVar, passphrase)
		backup := writeBackup(t, backupHex+"\n")
		out := newOut(t)
		if err := runImportIdenaBackup([]string{"-backup", backup, "-out", out}); err != nil {
			t.Fatal(err)
		}
		contents, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(contents, []byte(hex.EncodeToString(keyBytes))) {
			t.Fatal("imported nodekey differs from the decrypted private key")
		}
		info, err := os.Lstat(out)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatal("imported nodekey is not an owner-only regular file")
		}
		key, err := crypto.ToECDSA(keyBytes)
		if err != nil {
			t.Fatal(err)
		}
		want, err := nodekeyAddressOf(key)
		if err != nil {
			t.Fatal(err)
		}
		got, err := sellerIdentityAddress(out)
		if err != nil || got != want {
			t.Fatal("seller-client cannot load the imported identity")
		}
	})

	t.Run("wrong password never creates nodekey", func(t *testing.T) {
		t.Setenv(PassphraseEnvVar, "wrong-password")
		out := newOut(t)
		assertNoSecrets(t, runImportIdenaBackup([]string{"-backup", writeBackup(t, backupHex), "-out", out}))
		if _, err := os.Lstat(out); !os.IsNotExist(err) {
			t.Fatal("nodekey appeared after failed decryption")
		}
	})

	t.Run("truncated and tampered ciphertext never create nodekey", func(t *testing.T) {
		t.Setenv(PassphraseEnvVar, passphrase)
		altered := append([]byte(nil), sealed...)
		altered[len(altered)-1] ^= 1
		for _, contents := range []string{hex.EncodeToString(sealed[:1]), hex.EncodeToString(altered)} {
			out := newOut(t)
			assertNoSecrets(t, runImportIdenaBackup([]string{"-backup", writeBackup(t, contents), "-out", out}))
			if _, err := os.Lstat(out); !os.IsNotExist(err) {
				t.Fatal("nodekey appeared from invalid ciphertext")
			}
		}
	})

	t.Run("existing nodekey is never overwritten", func(t *testing.T) {
		t.Setenv(PassphraseEnvVar, passphrase)
		out := newOut(t)
		original := []byte("existing-nodekey")
		if err := os.WriteFile(out, original, 0o600); err != nil {
			t.Fatal(err)
		}
		assertNoSecrets(t, runImportIdenaBackup([]string{"-backup", writeBackup(t, backupHex), "-out", out}))
		contents, err := os.ReadFile(out)
		if err != nil || !bytes.Equal(contents, original) {
			t.Fatal("existing nodekey changed")
		}
	})

	t.Run("rejects unsafe paths and permissions", func(t *testing.T) {
		t.Setenv(PassphraseEnvVar, passphrase)
		backup := writeBackup(t, backupHex)
		publicDir := filepath.Join(t.TempDir(), "public")
		if err := os.Mkdir(publicDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(publicDir, 0o755); err != nil {
			t.Fatal(err)
		}
		assertNoSecrets(t, runImportIdenaBackup([]string{"-backup", backup, "-out", filepath.Join(publicDir, "nodekey")}))
		backupLink := filepath.Join(t.TempDir(), "backup-link")
		if err := os.Symlink(backup, backupLink); err != nil {
			t.Fatal(err)
		}
		assertNoSecrets(t, runImportIdenaBackup([]string{"-backup", backupLink, "-out", newOut(t)}))
		privateDir := t.TempDir()
		linkedDir := filepath.Join(t.TempDir(), "private-link")
		if err := os.Symlink(privateDir, linkedDir); err != nil {
			t.Fatal(err)
		}
		assertNoSecrets(t, runImportIdenaBackup([]string{"-backup", backup, "-out", filepath.Join(linkedDir, "nodekey")}))
		assertNoSecrets(t, runImportIdenaBackup([]string{"-backup", backup, "-out", "relative-nodekey"}))
		assertNoSecrets(t, runImportIdenaBackup([]string{"-backup", "relative-backup", "-out", newOut(t)}))
		assertNoSecrets(t, runImportIdenaBackup([]string{"-backup", writeBackup(t, strings.Repeat("0", maxIdenaBackupBytes+1)), "-out", newOut(t)}))
	})
}
