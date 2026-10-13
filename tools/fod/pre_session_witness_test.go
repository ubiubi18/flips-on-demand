package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPreSessionWitnessBeforeVerdictKeepsIdentitiesSeparate(t *testing.T) {
	dir := t.TempDir()
	key, author := registrationWitnessTestKey(t, dir)
	otherKey, _ := registrationWitnessTestKey(t, t.TempDir())
	blobDir := filepath.Join(dir, "blobs")
	if err := os.Mkdir(blobDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := make([]string, 0, 3)
	for _, body := range [][]byte{[]byte("submitted flip a"), []byte("submitted flip b"), []byte("submitted flip c")} {
		cid := cidRawSha256(body)
		manifest = append(manifest, cid)
		if err := os.WriteFile(filepath.Join(blobDir, cid), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inventoryPath := filepath.Join(dir, "inventory.json")
	raw, err := json.Marshal(preSessionInventory{
		Format: preSessionInventoryFormat, Author: author, Epoch: fixtureEpoch, Manifest: manifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inventoryPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "witness.json")
	if err := writePreSessionWitness(out, inventoryPath, otherKey, blobDir); err == nil {
		t.Fatal("another identity's node key produced a witness")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("cross-identity refusal left a witness")
	}
	if err := writePreSessionWitness(out, inventoryPath, key, blobDir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private witness mode: %v, %v", info, err)
	}
	var witness RegistrationBlobInput
	bytes, err := os.ReadFile(out)
	if err != nil || json.Unmarshal(bytes, &witness) != nil {
		t.Fatal("unreadable witness")
	}
	if witness.Author != author || witness.Epoch != fixtureEpoch || len(witness.Blobs) != 3 {
		t.Fatal("pre-session witness lost author, epoch or a flip")
	}
	for i, cid := range manifest {
		if witness.Blobs[i].BlobCid != cid {
			t.Fatal("pre-session witness changed the submitted CID sequence")
		}
	}
}

func TestPreSessionInventoryRefusesLinkedOrPublicInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(path, []byte(`{"format":"fod.pre-session-inventory:v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPreSessionInventory(path); err == nil {
		t.Fatal("public inventory accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readPreSessionInventory(link); err == nil {
		t.Fatal("linked inventory accepted")
	}
}
