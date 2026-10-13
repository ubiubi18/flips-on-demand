package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"filippo.io/age"
)

// VaultFileMode is the permission mode used for vault files: owner read/write
// only. Private key material must never be group or world readable.
const VaultFileMode os.FileMode = 0o600

// WriteVault encrypts payload to path with a passphrase-derived age key.
//
// Published only after encryption completes and fsync succeeds. An existing
// vault is reused only for identical decrypted payload; never overwritten.
func WriteVault(path, passphrase string, payload []byte) error {
	if passphrase == "" {
		return fmt.Errorf("refusing to write a vault with an empty passphrase")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing a non-regular existing vault")
		}
		old, err := ReadVault(path, passphrase)
		if err != nil || !equalVaultPayload(old, payload) {
			return fmt.Errorf("vault already exists with different keys or passphrase; choose a new path (existing vault preserved)")
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		return file.Chmod(VaultFileMode)
	} else if !os.IsNotExist(err) {
		return err
	}
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return fmt.Errorf("build vault recipient: %w", err)
	}

	return writePrivateFile(path, false, func(file *os.File) error {
		writer, err := age.Encrypt(file, recipient)
		if err != nil {
			return err
		}
		if _, err := writer.Write(payload); err != nil {
			return err
		}
		return writer.Close()
	})
}

// Old author vaults may use indented JSON where receive writes compact JSON.
// Exact JSON values can be reused without rewriting the encrypted file.
func equalVaultPayload(old, incoming []byte) bool {
	if bytes.Equal(old, incoming) {
		return true
	}
	var values [2]any
	for i, data := range [][]byte{old, incoming} {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if decoder.Decode(&values[i]) != nil || decoder.Decode(new(any)) != io.EOF {
			return false
		}
	}
	return reflect.DeepEqual(values[0], values[1])
}

// Atomic publication prevents partial writes and never follows a target
// symlink. Root also prevents a rename of the parent redirecting the write.
func writePrivateFile(path string, replace bool, write func(*os.File) error) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	return writeRootPrivateFile(root, filepath.Base(path), replace, write)
}

func writeRootPrivateFile(root *os.Root, name string, replace bool, write func(*os.File) error) error {
	temporary := ".fod-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, VaultFileMode)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if err := write(file); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if replace {
		err = root.Rename(temporary, name)
	} else {
		err = root.Link(temporary, name)
	}
	if err != nil {
		return fmt.Errorf("publish private file: %w", err)
	}
	if !replace {
		if err := root.Remove(temporary); err != nil {
			return err
		}
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// ReadVault decrypts a vault written by WriteVault.
func ReadVault(path, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("empty passphrase")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open vault: %w", err)
	}
	defer file.Close()

	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, fmt.Errorf("build vault identity: %w", err)
	}
	reader, err := age.Decrypt(file, identity)
	if err != nil {
		return nil, fmt.Errorf("decrypt vault: %w", err)
	}
	payload, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read vault payload: %w", err)
	}
	return payload, nil
}

// VaultPermissionsSecure reports whether a vault file is owner-only.
func VaultPermissionsSecure(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().Perm()&0o077 == 0, nil
}
