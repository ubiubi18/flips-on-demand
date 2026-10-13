package main

// The Plan B route for one author-epoch listing: the submission list comes from
// the explorer's per-identity flip records, each flip's chain anchor comes from
// the explorer's own flip record, and every one of those claims is either
// confirmed by a node the caller named or recorded as unconfirmed. `SPEC.md` 5.3
// permits an indexer as a source under exactly that condition.
//
// What this route does not do: it never calls the listing complete. Completeness
// rests on a committed-body sweep, which an explorer's list cannot replace, and
// the ceremony's assignment - which the manifest's proof needs - is state no
// explorer publishes at all. `prepare-offer` therefore refuses an inventory this
// route produced.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/idena-network/idena-go/blockchain/attachments"
)

// RootCheck is the epoch's state root as an indexer published it, together with
// what a node was able to confirm about it. The root is the one field of an
// explorer answer anybody with a node can read for themselves, from the header
// at the height the root belongs to (`SOURCES.md` F-82).
type RootCheck struct {
	Root         string `json:"root,omitempty"`
	Height       uint64 `json:"height,omitempty"`
	HeightSource string `json:"heightSource,omitempty"`
	NodeVerified bool   `json:"nodeVerified"`
	Note         string `json:"note,omitempty"`
}

// indexerFlipAnchor is the explorer's own record for one flip CID
// (`/Flip/{cid}`). Unlike the per-identity list, it names the transaction that
// carried the submission, the block that transaction landed in, and the height
// of that block - the three fields that let someone with a node check the claim
// rather than believe it.
type indexerFlipAnchor struct {
	CID         string `json:"-"`
	Author      string `json:"author"`
	Epoch       uint16 `json:"epoch"`
	Size        int    `json:"size"`
	Status      string `json:"status"`
	Answer      string `json:"answer"`
	TxHash      string `json:"txHash"`
	BlockHash   string `json:"blockHash"`
	BlockHeight uint64 `json:"blockHeight"`
}

type indexerFlipAnchorPage struct {
	Result indexerFlipAnchor `json:"result"`
}

// fetchIndexerFlipAnchor reads the explorer's record for one CID.
func fetchIndexerFlipAnchor(c *httpClient, base, cid string) (*indexerFlipAnchor, error) {
	var answer indexerFlipAnchorPage
	endpoint := fmt.Sprintf("%s/Flip/%s", strings.TrimRight(base, "/"), cid)
	if err := c.get(endpoint, &answer); err != nil {
		return nil, err
	}
	anchor := answer.Result
	anchor.CID = cid
	return &anchor, nil
}

// nodeFlipAnchorCheck asks a node what it holds for one indexed flip: the header
// at the height the explorer names, and the transaction it says carried the
// submission. A contradiction is refused; a node that does not answer leaves the
// anchor unchecked, which the caller records and `-require-node` turns into a
// refusal.
func nodeFlipAnchorCheck(node *client, epoch uint16, anchor *indexerFlipAnchor) (bool, string, error) {
	if node == nil {
		return false, "not checked: no node was named to compare with", nil
	}
	if anchor.TxHash == "" || anchor.BlockHash == "" || anchor.BlockHeight == 0 {
		return false, "", fmt.Errorf(
			"the explorer's record for flip %s names no transaction, block, or height: there is nothing for a node to check",
			anchor.CID)
	}
	var header rpcBlock
	if err := node.call("bcn_blockAt", []any{anchor.BlockHeight}, &header); err != nil {
		return false, fmt.Sprintf("not checked: the node did not answer for block %d: %v", anchor.BlockHeight, err), nil
	}
	if !strings.EqualFold(header.Hash, anchor.BlockHash) {
		return false, "", fmt.Errorf(
			"the explorer places flip %s in block %d (%s) and the node's header for that height is %s: the listing is refused",
			anchor.CID, anchor.BlockHeight, anchor.BlockHash, header.Hash)
	}
	if len(header.Transactions) > 0 && !hasFold(header.Transactions, anchor.TxHash) {
		return false, "", fmt.Errorf(
			"the explorer places flip %s in the transaction %s, which the node's block %d does not list: the listing is refused",
			anchor.CID, anchor.TxHash, anchor.BlockHeight)
	}
	var tx rpcTx
	if err := node.call("bcn_transaction", []any{anchor.TxHash}, &tx); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return false, "", fmt.Errorf(
				"the explorer names transaction %s for flip %s and the node does not have it: the listing is refused",
				anchor.TxHash, anchor.CID)
		}
		return false, fmt.Sprintf("not checked: the node did not answer for transaction %s: %v", anchor.TxHash, err), nil
	}
	if tx.Type != "submitFlip" {
		return false, "", fmt.Errorf(
			"the explorer names transaction %s for flip %s and the node has it as a %q transaction: the listing is refused",
			anchor.TxHash, anchor.CID, tx.Type)
	}
	if !sameAddress(tx.From, anchor.Author) || tx.Epoch != epoch {
		return false, "", fmt.Errorf(
			"the explorer names transaction %s for flip %s by %s in epoch %d, and the node has it by %s in epoch %d: the listing is refused",
			anchor.TxHash, anchor.CID, anchor.Author, epoch, tx.From, tx.Epoch)
	}
	var attachment attachments.FlipSubmitAttachment
	if err := attachment.FromBytes([]byte(tx.Payload)); err != nil {
		return false, "", fmt.Errorf(
			"the node's transaction %s does not carry a flip submission payload: %v", anchor.TxHash, err)
	}
	if cidString(attachment.Cid) != anchor.CID {
		return false, "", fmt.Errorf(
			"the explorer names transaction %s for flip %s and the node's copy of that transaction carries %s: the listing is refused",
			anchor.TxHash, anchor.CID, cidString(attachment.Cid))
	}
	return true, fmt.Sprintf(
		"checked: the node's header for block %d is the one the explorer names, and its copy of transaction %s is this author's flip submission in this epoch",
		anchor.BlockHeight, anchor.TxHash), nil
}

// buildIndexerInventory turns the explorer's records for one author-epoch into a
// listing, checking every anchor and every blob against whatever the caller
// offers to check them with.
func buildIndexerInventory(ex *httpClient, base string, epoch uint16, author string, fetch bodyFetcher, saveBlobsDir string, compute cidComputer, node *client, requireNode bool) (*BundleInventory, error) {
	records, err := fetchIndexerIdentityFlips(ex, base, epoch, author)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("the indexer lists no flips for %s in epoch %d", author, epoch)
	}

	inventory := &BundleInventory{
		Author:  author,
		Epoch:   epoch,
		Source:  strings.TrimRight(base, "/"),
		Notes:   []string{},
		Entries: []BundleEntry{},
	}
	seen := map[string]bool{}
	anchors := []*indexerFlipAnchor{}
	for _, record := range records {
		if !sameAddress(record.Author, author) || record.Epoch != epoch {
			continue
		}
		if seen[record.CID] {
			inventory.Duplicates++
			continue
		}
		seen[record.CID] = true
		anchor, err := fetchIndexerFlipAnchor(ex, base, record.CID)
		if err != nil {
			return nil, fmt.Errorf("read the explorer's record for flip %s: %w", record.CID, err)
		}
		if !sameAddress(anchor.Author, author) || anchor.Epoch != epoch {
			return nil, fmt.Errorf(
				"the explorer's record for flip %s names %s in epoch %d, not %s in epoch %d: the listing is refused",
				record.CID, anchor.Author, anchor.Epoch, author, epoch)
		}
		inventory.Submissions++
		anchors = append(anchors, anchor)
	}
	if inventory.Submissions == 0 {
		return nil, fmt.Errorf("the indexer lists no flips for %s in epoch %d", author, epoch)
	}
	inventory.UniqueCIDs = len(anchors)

	confirmed := 0
	unconfirmed := ""
	for _, anchor := range anchors {
		entry := BundleEntry{
			CID:         anchor.CID,
			TxHash:      anchor.TxHash,
			BlockHash:   anchor.BlockHash,
			BlockHeight: anchor.BlockHeight,
		}
		checked, note, err := nodeFlipAnchorCheck(node, epoch, anchor)
		if err != nil {
			return nil, err
		}
		entry.NodeChecked = checked
		if checked {
			confirmed++
		} else if unconfirmed == "" {
			unconfirmed = note
		}
		blob, source, err := fetch(entry.CID)
		entry.BlobCheckedAt = checkedAtNow()
		if err != nil {
			entry.Error = fmt.Sprintf("fetch: %v", err)
			inventory.BlobsFailed++
			inventory.Entries = append(inventory.Entries, entry)
			continue
		}
		entry.BlobSource = source
		if len(blob) == 0 {
			entry.Error = "blob unavailable"
			inventory.BlobsFailed++
			inventory.Entries = append(inventory.Entries, entry)
			continue
		}
		entry.BlobBytes = len(blob)
		entry.BlobCIDOk, entry.BlobCIDNote = verifyBlobCID(entry.CID, blob, compute)
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
		inventory.Entries = append(inventory.Entries, entry)
	}

	root, err := fetchIndexerStateRoot(ex, base, epoch)
	if err != nil {
		return nil, fmt.Errorf("read the epoch's state root: %w", err)
	}
	check := &RootCheck{Root: strings.TrimPrefix(strings.ToLower(root), "0x")}
	if height, err := fetchIndexerEpochOpeningBlock(ex, base, epoch); err != nil {
		check.HeightSource = fmt.Sprintf("not resolved: %v", err)
	} else {
		check.Height = height
		check.HeightSource = fmt.Sprintf("the first block the explorer lists for epoch %d", epoch-1)
	}
	verified, note, err := nodeRootCheck(node, root, check.Height)
	if err != nil {
		return nil, err
	}
	check.NodeVerified = verified
	check.Note = note
	inventory.Root = check

	inventory.NodeVerified = confirmed == len(anchors) && verified
	switch {
	case confirmed == len(anchors) && verified:
		inventory.NodeNote = fmt.Sprintf(
			"checked: a node confirmed all %d anchors and the epoch's state root", len(anchors))
	case confirmed == 0:
		inventory.NodeNote = fmt.Sprintf(
			"not checked: no node confirmed an anchor here (%s), and the epoch's state root is %s", unconfirmed, check.Note)
	default:
		inventory.NodeNote = fmt.Sprintf(
			"partly checked: a node confirmed %d of %d anchors, and the epoch's state root is %s", confirmed, len(anchors), check.Note)
	}
	if requireNode && !inventory.NodeVerified {
		return nil, fmt.Errorf(
			"a node did not confirm this listing: %d of %d anchors are checked, and the epoch's state root is %s",
			confirmed, len(anchors), check.Note)
	}

	inventory.Complete = false
	inventory.Notes = append(inventory.Notes,
		"the listing is the explorer's own record of this author and epoch; completeness rests on a committed-body sweep, which this route does not perform")
	if inventory.Duplicates > 0 {
		inventory.Notes = append(inventory.Notes, fmt.Sprintf("%d repeated cid records collapsed", inventory.Duplicates))
	}
	inventory.Notes = append(inventory.Notes,
		"deletions are not part of the explorer's per-identity flip list, so a deleted submission this route cannot see is not in the listing")
	if large := countUnixFSCIDs(anchors); large > 0 {
		inventory.Notes = append(inventory.Notes, fmt.Sprintf(
			"%d of %d cids are UnixFS/dag-pb (a flip over one chunk): the importer's layout is rebuilt locally to check one, and the registration proof carries whichever form a blob's size implies (`SOURCES.md` F-86, F-88, F-89)",
			large, len(anchors)))
	}
	if inventory.BlobsFailed > 0 {
		inventory.Notes = append(inventory.Notes, fmt.Sprintf(
			"%d blobs were not fetched, so their cids stay unverified: the bytes come from the node's own IPFS or a gateway the caller names, and the cid is decided from those bytes",
			inventory.BlobsFailed))
	}
	return inventory, nil
}

// sameAddress compares two addresses in the forms the explorer and the node use:
// one may be checksummed, the other lower case, and either may lack the prefix.
func sameAddress(a, b string) bool {
	return strings.EqualFold(strings.TrimPrefix(a, "0x"), strings.TrimPrefix(b, "0x"))
}

func hasFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

// countUnixFSCIDs counts the cids that are not the raw sha256 form: the
// UnixFS/dag-pb roots a flip over one chunk carries (`SOURCES.md` F-86).
func countUnixFSCIDs(anchors []*indexerFlipAnchor) int {
	count := 0
	for _, anchor := range anchors {
		if !isRawCID(anchor.CID) {
			count++
		}
	}
	return count
}
