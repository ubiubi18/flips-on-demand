package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRPCKey(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "rpc.key")
	if err := os.WriteFile(keyFile, []byte("fixture-api-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveRPCKey(keyFile, true); err != nil || got != "fixture-api-key" {
		t.Fatalf("owner-only key file: got %q, %v", got, err)
	}
	if got, err := resolveRPCKey("", false); err != nil || got != "" {
		t.Fatalf("no key file: got %q, %v", got, err)
	}

	if _, err := resolveRPCKey("", true); err == nil {
		t.Fatal("accepted an empty key-file path")
	}

	link := filepath.Join(dir, "link.key")
	if err := os.Symlink(keyFile, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, dir} {
		if _, err := resolveRPCKey(path, true); err == nil {
			t.Fatalf("accepted symlink or directory %q", path)
		}
	}
	if err := os.Chmod(keyFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRPCKey(keyFile, true); err == nil {
		t.Fatal("accepted group-readable key file")
	}
	if err := os.Chmod(keyFile, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(" \n\t"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRPCKey(keyFile, true); err == nil {
		t.Fatal("accepted an empty key")
	}
	if err := os.WriteFile(keyFile, []byte(strings.Repeat("x", maxRPCKeyFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRPCKey(keyFile, true); err == nil {
		t.Fatal("accepted an oversized key file")
	}
}
