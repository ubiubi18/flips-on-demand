package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"

	"github.com/idena-network/idena-go/crypto"
)

// The mobile backup is hex(12-byte GCM nonce || 32-byte key || 16-byte tag).
// Leave room for a trailing newline, but never read an unbounded input file.
const maxIdenaBackupBytes = 128
const idenaBackupCipherBytes = 12 + 32 + 16

func runImportIdenaBackup(args []string) error {
	f := flag.NewFlagSet("import-idena-backup", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	backup := f.String("backup", "", "encrypted Idena mobile backup file")
	out := f.String("out", "", "new owner-only nodekey file")
	if err := f.Parse(args); err != nil {
		return errors.New("import-idena-backup needs -backup ABS_FILE -out ABS_FILE")
	}
	if f.NArg() != 0 || !filepath.IsAbs(*backup) || !filepath.IsAbs(*out) {
		return errors.New("import-idena-backup needs -backup ABS_FILE -out ABS_FILE")
	}

	dir, err := os.Lstat(filepath.Dir(*out))
	if err != nil || !dir.IsDir() || dir.Mode().Perm() != 0o700 {
		return errors.New("output directory must be an ordinary 0700 directory")
	}
	info, err := os.Lstat(*backup)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxIdenaBackupBytes {
		return errors.New("backup must be a regular file within the size limit")
	}
	file, err := os.Open(*backup)
	if err != nil {
		return errors.New("cannot open backup file")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Size() > maxIdenaBackupBytes {
		return errors.New("backup changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxIdenaBackupBytes+1))
	if err != nil || len(raw) > maxIdenaBackupBytes {
		return errors.New("cannot read bounded backup file")
	}
	defer clear(raw)
	encoded := bytes.TrimSpace(raw)
	ciphertext := make([]byte, hex.DecodedLen(len(encoded)))
	defer clear(ciphertext)
	n, err := hex.Decode(ciphertext, encoded)
	if err != nil {
		return errors.New("invalid Idena backup encoding")
	}
	ciphertext = ciphertext[:n]
	// crypto.Decrypt slices the nonce before checking ciphertext length.
	if len(ciphertext) < 12+16 {
		return errors.New("invalid Idena backup ciphertext")
	}
	if len(ciphertext) != idenaBackupCipherBytes {
		return errors.New("invalid Idena backup length")
	}
	passphrase, err := readPassphrase("Idena backup passphrase: ")
	if err != nil {
		return err
	}
	keyBytes, err := crypto.Decrypt(ciphertext, passphrase)
	if err != nil {
		return errors.New("Idena backup decryption failed")
	}
	defer clear(keyBytes)
	if len(keyBytes) != 32 {
		return errors.New("Idena backup contains an invalid key")
	}
	if _, err := crypto.ToECDSA(keyBytes); err != nil {
		return errors.New("Idena backup contains an invalid key")
	}
	nodekey := make([]byte, hex.EncodedLen(len(keyBytes)))
	defer clear(nodekey)
	hex.Encode(nodekey, keyBytes)
	if err := writeNewPoolV7PrivateFile(*out, nodekey); err != nil {
		return errors.New("cannot create owner-only nodekey file")
	}
	return nil
}
