//go:build darwin || linux || freebsd

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStandaloneLockSurvivesStaleFileAndRefusesConcurrentApp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.lock")
	if err := os.WriteFile(path, []byte("pid=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	release, err := acquireStandaloneLock(path)
	if err != nil {
		t.Fatalf("stale lock file blocked restart: %v", err)
	}
	if other, err := acquireStandaloneLock(path); err == nil {
		other()
		t.Fatal("second app acquired an active data directory")
	}
	release()
	release, err = acquireStandaloneLock(path)
	if err != nil {
		t.Fatalf("released lock blocked restart: %v", err)
	}
	release()

	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if release, err := acquireStandaloneLock(path); err == nil {
		release()
		t.Fatal("symlink lock accepted")
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != "preserve" {
		t.Fatal("unsafe lock changed the symlink target")
	}
}
