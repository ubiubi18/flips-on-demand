package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyDirectV5FastHost(t *testing.T) {
	const image = fastContentV5ImageIDHex
	const contract = "1111111111111111111111111111111111111111"
	root := t.TempDir()
	proofPath := filepath.Join(root, "proof.json")
	if err := os.WriteFile(proofPath, []byte("dummy proof"), 0o600); err != nil {
		t.Fatal(err)
	}
	hostPath := filepath.Join(root, "fast-content-host")
	argumentsPath := filepath.Join(root, "arguments")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	hostScript := func(imageOutput, verifyBody string) string {
		return "#!/bin/sh\n" +
			"[ \"$RISC0_DEV_MODE\" = 0 ] && [ \"$PATH\" = /usr/bin:/bin ] && " +
			"[ -z \"${FOD_PASSPHRASE+x}\" ] && [ -z \"${FOD_RPC_URL+x}\" ] && " +
			"[ -z \"${FOD_API_KEY+x}\" ] || exit 7\n" +
			"if [ \"$#\" -eq 1 ] && [ \"$1\" = --image-id ]; then\n" +
			"  printf '%s\\n' " + quote(imageOutput) + "\n" +
			"elif [ \"$#\" -eq 4 ] && [ \"$1\" = --verify ] && [ \"$3\" = --contract ]; then\n" +
			verifyBody + "\n" +
			"else\n  exit 8\nfi\n"
	}
	goodVerify := "  printf '%s\\n' \"$2\" \"$4\" > " + quote(argumentsPath)
	writeHost := func(script string) {
		t.Helper()
		if err := os.WriteFile(hostPath, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeHost(hostScript(image, goodVerify))
	t.Setenv("FOD_PASSPHRASE", "private-canary")
	t.Setenv("FOD_RPC_URL", "private-rpc-canary")
	t.Setenv("FOD_API_KEY", "private-api-canary")
	if err := verifyDirectV5FastHost(proofPath, hostPath, contract, image); err != nil {
		t.Fatalf("valid host preflight failed: %v", err)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(arguments) != proofPath+"\n0x"+contract+"\n" {
		t.Fatalf("host received unexpected verification arguments: %q", arguments)
	}

	t.Run("public proof", func(t *testing.T) {
		if err := os.Chmod(proofPath, 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(proofPath, 0o600)
		if err := verifyDirectV5FastHost(proofPath, hostPath, contract, image); err == nil {
			t.Fatal("public proof passed preflight")
		}
	})
	t.Run("oversized proof", func(t *testing.T) {
		if err := os.WriteFile(proofPath, make([]byte, 64*1024+1), 0o600); err != nil {
			t.Fatal(err)
		}
		defer os.WriteFile(proofPath, []byte("dummy proof"), 0o600)
		if err := verifyDirectV5FastHost(proofPath, hostPath, contract, image); err == nil {
			t.Fatal("oversized proof passed preflight")
		}
	})
	t.Run("symlink proof", func(t *testing.T) {
		link := filepath.Join(root, "proof-link.json")
		if err := os.Symlink(proofPath, link); err != nil {
			t.Fatal(err)
		}
		if err := verifyDirectV5FastHost(link, hostPath, contract, image); err == nil {
			t.Fatal("symlink proof passed preflight")
		}
	})
	t.Run("non-executable host", func(t *testing.T) {
		if err := os.Chmod(hostPath, 0o600); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(hostPath, 0o700)
		if err := verifyDirectV5FastHost(proofPath, hostPath, contract, image); err == nil {
			t.Fatal("non-executable host passed preflight")
		}
	})
	t.Run("wrong image", func(t *testing.T) {
		writeHost(hostScript(strings.Repeat("0", 64), goodVerify))
		defer writeHost(hostScript(image, goodVerify))
		if err := verifyDirectV5FastHost(proofPath, hostPath, contract, image); err == nil || !strings.Contains(err.Error(), "image") {
			t.Fatalf("wrong host image passed preflight: %v", err)
		}
	})
	t.Run("host refusal hides output", func(t *testing.T) {
		writeHost(hostScript(image, "  echo private-canary; echo private-canary >&2; exit 9"))
		defer writeHost(hostScript(image, goodVerify))
		err := verifyDirectV5FastHost(proofPath, hostPath, contract, image)
		if err == nil || strings.Contains(err.Error(), "private-canary") || strings.Contains(err.Error(), proofPath) {
			t.Fatalf("host refusal leaked output or passed: %v", err)
		}
	})
}
