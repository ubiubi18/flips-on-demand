package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const maxRPCKeyFileBytes = 4096

func resolveRPCKey(keyFile string, keyFileSet bool) (string, error) {
	if !keyFileSet {
		return "", nil
	}
	if keyFile == "" {
		return "", fmt.Errorf("-key-file needs a path")
	}

	info, err := os.Lstat(keyFile)
	if err != nil {
		return "", fmt.Errorf("inspect -key-file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("-key-file must be an owner-only regular file")
	}
	if info.Size() > maxRPCKeyFileBytes {
		return "", fmt.Errorf("-key-file is too large")
	}
	file, err := os.Open(keyFile)
	if err != nil {
		return "", fmt.Errorf("open -key-file: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("inspect open -key-file: %w", err)
	}
	if !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("-key-file changed or is not an owner-only regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxRPCKeyFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("read -key-file: %w", err)
	}
	if len(raw) > maxRPCKeyFileBytes {
		return "", fmt.Errorf("-key-file is too large")
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		return "", fmt.Errorf("-key-file contains an empty key")
	}
	return key, nil
}
