package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func TestDeliveryKeyCreationIsExclusiveAndDoesNotFollowSymlinks(t *testing.T) {
	for _, mode := range []string{"concurrent", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "delivery.key")
			if mode == "symlink" {
				outside := filepath.Join(t.TempDir(), "must-not-create")
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
				if _, err := CreateDeliveryKey(path); err == nil {
					t.Fatal("followed a dangling delivery-key symlink")
				}
				if _, err := os.Stat(outside); !os.IsNotExist(err) {
					t.Fatal("created a key outside its requested path", err)
				}
				return
			}
			start := make(chan struct{})
			results := make(chan string, 12)
			var workers sync.WaitGroup
			for range 12 {
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					pub, err := CreateDeliveryKey(path)
					if err == nil {
						results <- pub
					}
				}()
			}
			close(start)
			workers.Wait()
			close(results)
			var successes []string
			for pub := range results {
				successes = append(successes, pub)
			}
			if len(successes) != 1 {
				t.Fatalf("created %d different keys at one path", len(successes))
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			key, err := parseDeliveryKey(string(raw))
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey)) != successes[0] {
				t.Fatal("saved key is not the key returned to the buyer")
			}
			if secure, err := VaultPermissionsSecure(path); err != nil || !secure {
				t.Fatal("delivery key is not private", err)
			}
		})
	}
}

func TestInventoryCacheCannotOverwriteAnOutsideSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	data := []byte("authenticated encrypted fixture bytes")
	cid := cidRawSha256(data)
	author := "0x" + strings.Repeat("aa", 20)
	sweep := filepath.Join(dir, "sweep.json")
	if err := writeJSON(sweep, &SweepResult{Submissions: []Submission{{Author: author, Epoch: 204, CID: cid}}}); err != nil {
		t.Fatal(err)
	}
	blobs := filepath.Join(dir, "blobs")
	if err := os.Mkdir(blobs, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "keep")
	if err := os.WriteFile(outside, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(blobs, cid)); err != nil {
		t.Fatal(err)
	}
	_, err := buildInventory(sweep, author, 204, oneSource(func(string) ([]byte, error) { return data, nil }, "fixture"), blobs, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "unchanged" {
		t.Fatal("inventory cache overwrote an outside file")
	}
	got, err = os.ReadFile(filepath.Join(blobs, cid))
	if err != nil || string(got) != string(data) {
		t.Fatal("authenticated bytes were not saved", err)
	}
}
