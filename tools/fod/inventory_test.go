package main

// The inventory's four `SPEC.md` 11.1 scenarios as one synthetic sweep: a
// repeated CID, a deleted submission, an entry for another author or epoch, and a
// submission that names no CID at all - plus the blob check that makes the bundle
// complete or not.

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestInventoryCollapsesRepeatsAndKeepsDeletions(t *testing.T) {
	dir := t.TempDir()
	first := []byte("the first flip's blob")
	second := []byte("the second flip's blob")
	firstCID, secondCID := cidRawSha256(first), cidRawSha256(second)
	author := "0x" + strings.Repeat("aa", 20)
	stranger := "0x" + strings.Repeat("bb", 20)

	sweep := SweepResult{
		Submissions: []Submission{
			{TxHash: "0x1", BlockHeight: 100, Author: author, Epoch: 204, CID: firstCID, Pair: 0},
			// The same CID again, later, with another pair: one flip, earliest wins.
			{TxHash: "0x2", BlockHeight: 101, Author: author, Epoch: 204, CID: firstCID, Pair: 3},
			{TxHash: "0x3", BlockHeight: 102, Author: author, Epoch: 204, CID: secondCID, Pair: 1},
			// Another author's submission, and the same author in another epoch.
			{TxHash: "0x4", BlockHeight: 103, Author: stranger, Epoch: 204, CID: secondCID, Pair: 2},
			{TxHash: "0x5", BlockHeight: 104, Author: author, Epoch: 205, CID: secondCID, Pair: 2},
		},
		Deletions: []Deletion{
			{TxHash: "0x6", BlockHeight: 105, Author: author, Epoch: 204, CID: secondCID},
			// A deletion by somebody else is not this author's business.
			{TxHash: "0x7", BlockHeight: 106, Author: stranger, Epoch: 204, CID: firstCID},
		},
	}
	path := filepath.Join(dir, "sweep.json")
	if err := writeJSON(path, &sweep); err != nil {
		t.Fatalf("write the sweep: %v", err)
	}
	blobs := map[string][]byte{firstCID: first, secondCID: second}
	fetch := func(cid string) ([]byte, error) {
		if body, ok := blobs[cid]; ok {
			return body, nil
		}
		return nil, errors.New("not found")
	}

	inventory, err := buildInventory(path, author, 204, oneSource(fetch, "test source"), "", nil)
	if err != nil {
		t.Fatalf("build the inventory: %v", err)
	}
	if inventory.Submissions != 3 || inventory.UniqueCIDs != 2 || inventory.Duplicates != 1 {
		t.Fatalf("the sweep came out as %d submissions, %d unique, %d duplicates",
			inventory.Submissions, inventory.UniqueCIDs, inventory.Duplicates)
	}
	if inventory.Deleted != 1 || !inventory.Complete || inventory.BlobsOk != 2 || inventory.BlobsFailed != 0 {
		t.Fatalf("the inventory is %+v", inventory)
	}
	if len(inventory.Entries) != 2 {
		t.Fatalf("the inventory holds %d entries", len(inventory.Entries))
	}
	// Earliest submission wins, and the deletion is carried on its entry.
	if inventory.Entries[0].CID != firstCID || inventory.Entries[0].Pair != 0 || inventory.Entries[0].BlockHeight != 100 {
		t.Fatalf("the repeated cid kept the wrong submission: %+v", inventory.Entries[0])
	}
	if inventory.Entries[1].CID != secondCID || !inventory.Entries[1].Deleted {
		t.Fatalf("the deleted flip is not marked: %+v", inventory.Entries[1])
	}
	if !strings.Contains(strings.Join(inventory.Notes, " "), "deleted flips are retained") {
		t.Fatalf("the notes do not explain the deletion: %v", inventory.Notes)
	}
}

func TestInventoryRefusesWhatTheBlobCheckCannotVouchFor(t *testing.T) {
	dir := t.TempDir()
	honest := []byte("the flip's blob")
	cid := cidRawSha256(honest)
	author := "0x" + strings.Repeat("aa", 20)

	sweep := SweepResult{Submissions: []Submission{
		{TxHash: "0x1", BlockHeight: 100, Author: author, Epoch: 204, CID: cid},
		// A submission whose attachment named no CID at all.
		{TxHash: "0x2", BlockHeight: 101, Author: author, Epoch: 204, CID: ""},
	}}
	path := filepath.Join(dir, "sweep.json")
	if err := writeJSON(path, &sweep); err != nil {
		t.Fatalf("write the sweep: %v", err)
	}

	// The node serves bytes that are not the ones the CID commits to.
	inventory, err := buildInventory(path, author, 204, oneSource(func(string) ([]byte, error) {
		return []byte("some other bytes"), nil
	}, "test source"), "", nil)
	if err != nil {
		t.Fatalf("build the inventory: %v", err)
	}
	if inventory.Complete || inventory.BlobsFailed != 2 {
		t.Fatalf("a bundle nobody can vouch for came out complete: %+v", inventory)
	}
	reasons := map[string]bool{}
	for _, entry := range inventory.Entries {
		reasons[entry.Error] = true
	}
	if !reasons["the sha256 of these bytes is not the cid they were fetched for"] || !reasons["cid missing in submission"] {
		t.Fatalf("the entries do not name their failures: %+v", inventory.Entries)
	}
	// The sweep had submissions, so that note must not appear.
	if strings.Contains(strings.Join(inventory.Notes, " "), "no submissions found") {
		t.Fatalf("a sweep with submissions reported none: %v", inventory.Notes)
	}
}

// A flip over one chunk does not have a raw CID: the node imported it as a
// UnixFS/dag-pb root. This fixture's bytes do not rebuild that root locally,
// which is the case where `verifyBlobCID` falls back to the node's own importer
// instead of failing the blob (`SOURCES.md` F-88).
func TestInventoryRefusesANodesFalseCIDAttestation(t *testing.T) {
	dir := t.TempDir()
	blob := []byte("a flip over one chunk")
	cid := planBDagPubCID()
	author := "0x" + strings.Repeat("aa", 20)

	sweep := SweepResult{Submissions: []Submission{
		{TxHash: "0x1", BlockHeight: 100, Author: author, Epoch: 204, CID: cid},
	}}
	path := filepath.Join(dir, "sweep.json")
	if err := writeJSON(path, &sweep); err != nil {
		t.Fatalf("write the sweep: %v", err)
	}
	node := planBNode(t, nil, nil, map[string]string{string(blob): cid})

	inventory, err := buildInventory(path, author, 204,
		oneSource(func(string) ([]byte, error) { return blob, nil }, "node ipfs"), "", nodeCIDComputer(node))
	if err != nil {
		t.Fatalf("build the inventory: %v", err)
	}
	if inventory.Complete || inventory.BlobsOk != 0 || inventory.BlobsFailed != 1 {
		t.Fatalf("the node's false import assertion was accepted: %+v", inventory)
	}
	if !strings.Contains(inventory.Entries[0].BlobCIDNote, "locally") {
		t.Fatalf("the entry does not name local verification: %s", inventory.Entries[0].BlobCIDNote)
	}
	if !inventory.NodeVerified || inventory.Source != "node sweep" {
		t.Fatalf("the node route does not record where its data came from: %+v", inventory)
	}
}
