package main

// The availability advisory: the inventory's blob checks become an off-chain
// record, and the market joins it to the pool listing by author and epoch.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAvailabilityFromInventory(t *testing.T) {
	inventory := &BundleInventory{
		Author:      "0x" + strings.Repeat("ab", addressBytes),
		Epoch:       228,
		Source:      "node sweep",
		BlobsOk:     1,
		BlobsFailed: 1,
		Entries: []BundleEntry{
			{CID: "cid-ok", BlobBytes: 100, BlobCIDOk: true, BlobSource: "node ipfs", BlobCheckedAt: "2026-09-25T10:00:00Z"},
			{CID: "cid-bad", BlobCIDOk: false, BlobSource: "gateway", BlobCIDNote: "the bytes hash to another CID", BlobCheckedAt: "2026-09-26T09:00:00Z"},
		},
	}
	advisory := availabilityFromInventory(inventory)
	if advisory.Format != availabilityFormat || advisory.Version != availabilityVersion {
		t.Fatalf("the advisory is %s v%d", advisory.Format, advisory.Version)
	}
	if advisory.Author != strings.Repeat("ab", addressBytes) || advisory.Epoch != 228 {
		t.Fatalf("the advisory names %s epoch %d", advisory.Author, advisory.Epoch)
	}
	if advisory.CoveredFlips != 2 || advisory.VerifiedFlips != 1 || advisory.Complete {
		t.Fatalf("the advisory reads %d/%d complete=%v", advisory.VerifiedFlips, advisory.CoveredFlips, advisory.Complete)
	}
	if len(advisory.Flips) != 2 || !advisory.Flips[0].Verified || advisory.Flips[1].Verified {
		t.Fatalf("the flip checks read %+v", advisory.Flips)
	}
	if advisory.Flips[0].Source != "node ipfs" || advisory.Flips[1].Note == "" {
		t.Fatalf("the flip sources read %+v", advisory.Flips)
	}
	// The advisory says when its weakest check was made, not when the file was
	// written, and each flip carries its own time (`REVIEW.md` A08).
	if advisory.CheckedAt != "2026-09-25T10:00:00Z" {
		t.Fatalf("the advisory reads checkedAt %q, expected the oldest check", advisory.CheckedAt)
	}
	if advisory.Flips[0].CheckedAt != "2026-09-25T10:00:00Z" || advisory.Flips[1].CheckedAt != "2026-09-26T09:00:00Z" {
		t.Fatalf("the flip times read %+v", advisory.Flips)
	}
	if advisory.Note == "" {
		t.Fatal("the advisory carries no off-chain label")
	}
}

// TestAvailabilityAdvisoryNeverRestamps: writing the advisory later must not
// make an old check look fresh, and an inventory that recorded no check time
// leaves the advisory without one to claim.
func TestAvailabilityAdvisoryNeverRestamps(t *testing.T) {
	old := &BundleInventory{
		Author: strings.Repeat("ab", addressBytes),
		Epoch:  228,
		Entries: []BundleEntry{
			{CID: "cid", BlobCIDOk: true, BlobCheckedAt: "2020-01-01T00:00:00Z"},
		},
		BlobsOk: 1,
	}
	if got := availabilityFromInventory(old).CheckedAt; got != "2020-01-01T00:00:00Z" {
		t.Fatalf("an old check was restamped as %q", got)
	}

	undated := &BundleInventory{
		Author:  strings.Repeat("ab", addressBytes),
		Epoch:   228,
		Entries: []BundleEntry{{CID: "cid", BlobCIDOk: true}},
		BlobsOk: 1,
	}
	if got := availabilityFromInventory(undated).CheckedAt; got != "" {
		t.Fatalf("an inventory with no check time produced checkedAt %q", got)
	}
}

func TestAvailabilityRoundTripAndMarketJoin(t *testing.T) {
	author := strings.Repeat("ab", addressBytes)
	advisory := &AvailabilityAdvisory{
		Format:        availabilityFormat,
		Version:       availabilityVersion,
		Author:        author,
		Epoch:         228,
		CheckedAt:     "2026-09-26T12:00:00Z",
		CoveredFlips:  2,
		VerifiedFlips: 2,
		Complete:      true,
	}
	path := filepath.Join(t.TempDir(), "advisory.json")
	if err := writeJSON(path, advisory); err != nil {
		t.Fatalf("write the advisory: %v", err)
	}
	if _, err := readAvailability(path); err != nil {
		t.Fatalf("read the advisory: %v", err)
	}
	advisories, err := loadAvailability([]string{path})
	if err != nil {
		t.Fatalf("index the advisory: %v", err)
	}
	listing := &marketPoolListing{Bundle: strings.Repeat("cd", 32), Author: author, Epoch: 228}
	attachAvailability(listing, advisories)
	if listing.Availability == nil || !listing.Availability.Complete || listing.Availability.VerifiedFlips != 2 {
		t.Fatalf("the listing's availability is %+v", listing.Availability)
	}

	// Without the provenance epoch the listing cannot be joined, and it stays
	// unannotated rather than guessing.
	other := &marketPoolListing{Bundle: strings.Repeat("ef", 32), Author: author}
	attachAvailability(other, advisories)
	if other.Availability != nil {
		t.Fatalf("a listing without an epoch gained %+v", other.Availability)
	}

	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"format":"other","version":1,"author":"`+author+`","epoch":228}`), 0o600); err != nil {
		t.Fatalf("write the bad advisory: %v", err)
	}
	if _, err := readAvailability(bad); err == nil {
		t.Fatal("an advisory with another format was accepted")
	}
}
