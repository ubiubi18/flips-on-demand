package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/idena-network/idena-go/common/hexutil"
)

// BundleEntry is one flip in an author-epoch bundle.
type BundleEntry struct {
	CID         string `json:"cid"`
	CIDBytesHex string `json:"cidBytesHex"`
	Pair        uint8  `json:"pair"`
	TxHash      string `json:"txHash"`
	BlockHash   string `json:"blockHash,omitempty"`
	BlockHeight uint64 `json:"blockHeight"`
	Deleted     bool   `json:"deleted"`
	// BlobSource names who supplied the ciphertext (the node's IPFS, a gateway,
	// or nothing when no source was named), and NodeChecked says whether a node
	// confirmed this entry's anchor - the transaction that carried the flip and
	// the block it sits in - in this run.
	BlobSource  string `json:"blobSource,omitempty"`
	NodeChecked bool   `json:"nodeChecked,omitempty"`
	BlobBytes   int    `json:"blobBytes"`
	BlobCIDOk   bool   `json:"blobCidVerified"`
	// BlobCheckedAt is when this entry's bytes were fetched and hashed back to
	// the CID in this run, whether or not the check passed. An entry that was
	// never attempted carries no time, so no later reader can date it.
	BlobCheckedAt string `json:"blobCheckedAt,omitempty"`
	// BlobCIDNote names who decided the CID check, or what still has to be
	// asked, so a reader can tell a checked blob from an unchecked one.
	BlobCIDNote string `json:"blobCidNote,omitempty"`
	Error       string `json:"error,omitempty"`
}

// BundleInventory is the reconstruction result for one author and epoch.
//
// Completeness here means: every submission the committed-body sweep found for
// that author and epoch is present, each CID appears once, deletions are
// carried, and every blob was fetched and hashed back to its CID. It does not
// yet mean the sweep itself is authenticated (see SOURCES.md F-26).
type BundleInventory struct {
	Author      string        `json:"author"`
	Epoch       uint16        `json:"epoch"`
	Submissions int           `json:"submissionsSeen"`
	UniqueCIDs  int           `json:"uniqueCids"`
	Duplicates  int           `json:"duplicateCids"`
	Deleted     int           `json:"deletedCids"`
	BlobsOk     int           `json:"blobsVerified"`
	BlobsFailed int           `json:"blobsFailed"`
	Complete    bool          `json:"complete"`
	Notes       []string      `json:"notes"`
	Entries     []BundleEntry `json:"entries"`

	// Source names where the submission list came from: the node route reads a
	// sweep the caller's own node produced, the Plan B route reads an indexer
	// (`SPEC.md` 5.3).
	Source string `json:"source,omitempty"`
	// Root is the epoch's state root and what a node confirmed about it; only
	// the indexer route fills it, because only there can the root be a claim
	// rather than something the node said itself.
	Root *RootCheck `json:"epochRoot,omitempty"`
	// NodeVerified says every entry's anchor was confirmed by a node in this
	// run. An indexer-only listing is never verified, and `prepare-offer`
	// refuses one rather than turning it into a manifest.
	NodeVerified bool   `json:"nodeVerified"`
	NodeNote     string `json:"nodeNote,omitempty"`
}

// buildInventory turns a sweep into a per-author bundle and verifies the blobs.
// buildInventory turns a sweep into a per-author bundle and verifies the blobs.
//
// When saveBlobsDir is not empty, every blob that verifies against its CID is
// written there as <cid>, so a later export does not have to fetch it again.
//
// The CID a blob must hash to depends on the size of the blob: the pinned node
// imports a file as one raw leaf while it fits in a chunk and as a UnixFS root
// over raw leaves once it does not, so a large flip's CID is a dag-pb CID that
// recomputing a raw CID can never produce (`SOURCES.md` F-86). `cids` is how the
// caller answers that question with the same IPFS the node uses; when it is nil,
// only raw CIDs can be verified, which is what the fixtures need.
func buildInventory(sweepPath, author string, epoch uint16, fetch bodyFetcher, saveBlobsDir string, cids cidComputer) (*BundleInventory, error) {
	raw, err := os.ReadFile(sweepPath)
	if err != nil {
		return nil, err
	}
	var sweep SweepResult
	if err := json.Unmarshal(raw, &sweep); err != nil {
		return nil, fmt.Errorf("parse sweep: %w", err)
	}

	inventory := &BundleInventory{Author: author, Epoch: epoch, Notes: []string{}, Entries: []BundleEntry{}}
	seen := map[string]int{}

	deletedCIDs := map[string]bool{}
	for _, d := range sweep.Deletions {
		if !strings.EqualFold(d.Author, author) || d.Epoch != epoch {
			continue
		}
		deletedCIDs[d.CID] = true
	}

	for _, s := range sweep.Submissions {
		if !strings.EqualFold(s.Author, author) || s.Epoch != epoch {
			continue
		}
		inventory.Submissions++
		if idx, ok := seen[s.CID]; ok {
			inventory.Duplicates++
			// A repeated CID is one flip; keep the earliest submission.
			if s.BlockHeight < inventory.Entries[idx].BlockHeight {
				inventory.Entries[idx].BlockHeight = s.BlockHeight
				inventory.Entries[idx].TxHash = s.TxHash
				inventory.Entries[idx].Pair = s.Pair
			}
			continue
		}
		seen[s.CID] = len(inventory.Entries)
		inventory.Entries = append(inventory.Entries, BundleEntry{
			CID:         s.CID,
			CIDBytesHex: s.CIDBytes,
			Pair:        s.Pair,
			TxHash:      s.TxHash,
			BlockHeight: s.BlockHeight,
			Deleted:     deletedCIDs[s.CID],
		})
	}
	// Deletions without a matching submission still matter: record them.
	for cid := range deletedCIDs {
		if _, ok := seen[cid]; !ok {
			inventory.Entries = append(inventory.Entries, BundleEntry{CID: cid, Deleted: true})
		}
	}

	sort.Slice(inventory.Entries, func(i, j int) bool {
		return inventory.Entries[i].BlockHeight < inventory.Entries[j].BlockHeight
	})
	inventory.UniqueCIDs = len(inventory.Entries)
	for cid := range deletedCIDs {
		_ = cid
	}
	inventory.Deleted = len(deletedCIDs)

	for i := range inventory.Entries {
		entry := &inventory.Entries[i]
		if entry.CID == "" {
			entry.Error = "cid missing in submission"
			inventory.BlobsFailed++
			continue
		}
		blob, source, err := fetch(entry.CID)
		entry.BlobCheckedAt = checkedAtNow()
		if err != nil {
			entry.Error = fmt.Sprintf("fetch: %v", err)
			inventory.BlobsFailed++
			continue
		}
		entry.BlobSource = source
		if len(blob) == 0 {
			entry.Error = "blob unavailable"
			inventory.BlobsFailed++
			continue
		}
		entry.BlobBytes = len(blob)
		entry.BlobCIDOk, entry.BlobCIDNote = verifyBlobCID(entry.CID, blob, cids)
		if entry.BlobCIDOk {
			inventory.BlobsOk++
			if saveBlobsDir != "" {
				if err := os.MkdirAll(saveBlobsDir, 0o700); err != nil {
					return nil, fmt.Errorf("create blob directory: %w", err)
				}
				if err := writePrivateBytes(filepath.Join(saveBlobsDir, entry.CID), blob); err != nil {
					return nil, fmt.Errorf("save blob %s: %w", entry.CID, err)
				}
			}
		} else {
			entry.Error = entry.BlobCIDNote
			inventory.BlobsFailed++
		}
	}

	inventory.Complete = inventory.BlobsFailed == 0 && inventory.Submissions > 0
	inventory.Source = "node sweep"
	inventory.NodeVerified = true
	inventory.NodeNote = "the listing and every blob came from a node's own RPC, and each body was verified against the CID its header reports"
	if inventory.Submissions == 0 {
		inventory.Notes = append(inventory.Notes, "no submissions found for this author and epoch in the sweep")
	}
	if inventory.Duplicates > 0 {
		inventory.Notes = append(inventory.Notes, fmt.Sprintf("%d repeated cid submissions collapsed", inventory.Duplicates))
	}
	if inventory.Deleted > 0 {
		inventory.Notes = append(inventory.Notes,
			"deleted flips are retained in the bundle; their ciphertext may still be available and shares the same epoch keys")
	}
	inventory.Notes = append(inventory.Notes,
		"completeness rests on the committed-body sweep, not on the node's transaction store")
	return inventory, nil
}

// ipfsFetch builds a fetch function backed by the node's IPFS API.
func ipfsFetch(c *client) bodyFetchFunc {
	return func(cid string) ([]byte, error) {
		var body hexutil.Bytes
		if err := c.call("ipfs_get", []any{cid}, &body); err != nil {
			return nil, err
		}
		return []byte(body), nil
	}
}
