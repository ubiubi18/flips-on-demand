package main

// The off-chain availability advisory of `SPEC.md` 5.8: a record of the last
// time a bundle's flip blobs were fetched and hashed back to their CIDs. It is
// advisory, never authoritative, and never contract state; anyone may publish
// one, and the interface shows the last check it was given.

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	availabilityFormat  = "fod.availability-advisory"
	availabilityVersion = 1

	// availabilityNote labels what the file is. It is written by the tool that
	// fetched the bytes, not by any chain rule, and anyone may publish another
	// one (`SPEC.md` 5.8).
	availabilityNote = "off-chain observation: the reporting tool fetched these bytes and hashed them back to their CIDs; no chain rule confirms availability, and anyone may refresh or replace this advisory"
)

// AvailabilityFlip is one flip's check: its CID, the bytes fetched, who served
// them, when the bytes were checked, and whether they rebuilt the CID.
type AvailabilityFlip struct {
	CID      string `json:"cid"`
	Bytes    int    `json:"bytes,omitempty"`
	Verified bool   `json:"verified"`
	Source   string `json:"source,omitempty"`
	Note     string `json:"note,omitempty"`
	// CheckedAt is when the bytes were fetched and hashed in the run that
	// produced the inventory. It is never restamped when the advisory is
	// written, so an old check cannot read as a fresh one.
	CheckedAt string `json:"checkedAt,omitempty"`
}

// AvailabilityAdvisory is one author-epoch bundle's last check. The author and
// epoch are the bundle's identity; the market joins on them.
type AvailabilityAdvisory struct {
	Format        string `json:"format"`
	Version       int    `json:"version"`
	Author        string `json:"author"`
	Epoch         uint16 `json:"epoch"`
	CheckedAt     string `json:"checkedAt"`
	Source        string `json:"source,omitempty"`
	CoveredFlips  int    `json:"coveredFlips"`
	VerifiedFlips int    `json:"verifiedFlips"`
	Complete      bool   `json:"complete"`
	NodeVerified  bool   `json:"nodeVerified"`
	// Note labels the file as an off-chain observation.
	Note  string             `json:"note,omitempty"`
	Flips []AvailabilityFlip `json:"flips"`
}

// checkedAtNow is the timestamp every observation in this program carries:
// UTC, RFC 3339, and comparable as text.
func checkedAtNow() string { return time.Now().UTC().Format(time.RFC3339) }

// availabilityOldestCheck is when the advisory's weakest observation was made.
// The value is the oldest check the file records, so writing the advisory later
// cannot make a check look fresher than it was; an entry with no recorded time
// leaves the field empty, because the advisory then holds no time it can claim.
func availabilityOldestCheck(flips []AvailabilityFlip) string {
	oldest := ""
	for _, flip := range flips {
		if flip.CheckedAt == "" {
			return ""
		}
		if oldest == "" || flip.CheckedAt < oldest {
			oldest = flip.CheckedAt
		}
	}
	return oldest
}

// availabilityFromInventory turns an inventory's blob checks into an advisory.
// The advisory is complete only when every entry was fetched and hashed back to
// its CID; a missing blob is recorded as verified=false rather than dropped.
// The times are the inventory's own, and the top-level time is the oldest of
// them rather than the moment this file was written.
func availabilityFromInventory(inventory *BundleInventory) *AvailabilityAdvisory {
	advisory := &AvailabilityAdvisory{
		Format:       availabilityFormat,
		Version:      availabilityVersion,
		Author:       strings.ToLower(strings.TrimPrefix(inventory.Author, "0x")),
		Epoch:        inventory.Epoch,
		Source:       inventory.Source,
		CoveredFlips: len(inventory.Entries),
		NodeVerified: inventory.NodeVerified,
		Note:         availabilityNote,
		Flips:        make([]AvailabilityFlip, 0, len(inventory.Entries)),
	}
	for _, entry := range inventory.Entries {
		flip := AvailabilityFlip{
			CID:       entry.CID,
			Bytes:     entry.BlobBytes,
			Verified:  entry.BlobCIDOk,
			Source:    entry.BlobSource,
			Note:      entry.BlobCIDNote,
			CheckedAt: entry.BlobCheckedAt,
		}
		if entry.BlobCIDOk {
			advisory.VerifiedFlips++
		}
		advisory.Flips = append(advisory.Flips, flip)
	}
	advisory.Complete = len(inventory.Entries) > 0 &&
		inventory.BlobsFailed == 0 &&
		advisory.VerifiedFlips == len(inventory.Entries)
	advisory.CheckedAt = availabilityOldestCheck(advisory.Flips)
	return advisory
}

// readAvailability reads one advisory file and checks its envelope.
func readAvailability(path string) (*AvailabilityAdvisory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var advisory AvailabilityAdvisory
	if err := json.Unmarshal(raw, &advisory); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if advisory.Format != availabilityFormat || advisory.Version != availabilityVersion {
		return nil, fmt.Errorf("%s is not an availability advisory this build reads", path)
	}
	if !isHex(advisory.Author, addressBytes) || advisory.Epoch == 0 {
		return nil, fmt.Errorf("%s names no valid author and epoch", path)
	}
	return &advisory, nil
}

// availabilityKey is the author-epoch join the market uses.
func availabilityKey(author string, epoch uint16) string {
	return strings.ToLower(strings.TrimPrefix(author, "0x")) + "/" + strconv.FormatUint(uint64(epoch), 10)
}
