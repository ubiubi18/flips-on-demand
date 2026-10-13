package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectCommandProtectsTradeInputsFromOutputAliases(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "nodekey")
	if err := os.WriteFile(key, []byte("retained key"), 0o600); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(dir, "alias")
	if err := os.Symlink(dir, aliasDir); err != nil {
		t.Fatal(err)
	}
	hardlink := filepath.Join(dir, "key-link")
	if err := os.Link(key, hardlink); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts directCommandOptions
	}{
		{"delivery would replace purchase", directCommandOptions{Mode: "direct-receive", PurchasePath: key, VaultPath: filepath.Join(dir, "./nodekey")}},
		{"message would replace vault through a linked directory", directCommandOptions{Mode: "direct-dm", VaultPath: key, OutPath: filepath.Join(aliasDir, "nodekey")}},
		{"buy would replace node key", directCommandOptions{Mode: "direct-buy", NodeKeyPath: key, PreparedPath: hardlink}},
		{"automatic confirmation would replace node key", directCommandOptions{Mode: "direct-receive", MessagePath: "-", NodeKeyPath: key, SignedPath: key, AutoConfirm: true}},
		{"recovery would replace purchase", directCommandOptions{Mode: "direct-recover", PurchasePath: key, OutPath: key}},
		{"check would replace delivery key", directCommandOptions{Mode: "direct-check", DeliveryKeyPath: key, OutPath: key}},
		{"fallback start would replace proof", directCommandOptions{Mode: "direct-start-fallback", ProofPath: key, PreparedPath: key}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkDirectPaths(tc.opts); err == nil || !strings.Contains(err.Error(), "must be distinct") {
				t.Fatalf("expected a path alias refusal, got %v", err)
			}
		})
	}
	if err := directCommand(directCommandOptions{Mode: "direct-dm", VaultPath: key, OutPath: key}); err == nil || !strings.Contains(err.Error(), "must be distinct") {
		t.Fatalf("expected refusal before contract or RPC access, got %v", err)
	}
	if raw, err := os.ReadFile(key); err != nil || string(raw) != "retained key" {
		t.Fatalf("protected input changed: %v", err)
	}
}

func TestDirectCommandAcceptsDistinctPathsAndStdinMessage(t *testing.T) {
	dir := t.TempDir()
	if err := checkDirectPaths(directCommandOptions{
		Mode: "direct-receive", MessagePath: "-", PurchasePath: filepath.Join(dir, "purchase"),
		DeliveryKeyPath: filepath.Join(dir, "key"), VaultPath: filepath.Join(dir, "vault"),
		OutPath: filepath.Join(dir, "receipt"),
	}); err != nil {
		t.Fatal(err)
	}
}
