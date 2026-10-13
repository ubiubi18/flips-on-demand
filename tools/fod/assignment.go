package main

// The step between a capture and the answer-to-flip join: the epoch store's
// candidate list turned into the lottery's assignment.
//
// The node keeps that assignment in memory only (F-79). What survives is the
// seed and the candidate list, in the order the ceremony indexed them, and the
// assignment is what `AssignFlips` derives from those two. This is the command
// that turns a captured epoch into the file the rest of the pipeline reads.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/idena-network/idena-go/common"
)

// CapturedIdentity is one candidate as the epoch store holds it: the address, the
// shard the ceremony sorted it into, and the CIDs of the flips it submitted.
type CapturedIdentity struct {
	Address  string   `json:"address"`
	Shard    uint8    `json:"shard"`
	FlipCids []string `json:"flipCids"`
}

// ShardAssignment is one shard's derived lottery.
type ShardAssignment struct {
	Shard      uint8    `json:"shard"`
	Addresses  []string `json:"addresses"`
	FlipCounts []int    `json:"flipCounts"`

	AuthorsPerCandidate    map[int][]int `json:"authorsPerCandidate"`
	CandidatesPerAuthor    map[int][]int `json:"candidatesPerAuthor"`
	ShortFlipsPerCandidate [][]int       `json:"shortFlipsPerCandidate"`
	LongFlipsPerCandidate  [][]int       `json:"longFlipsPerCandidate"`
}

// EpochAssignment is what a capture produces once the lottery is derived.
type EpochAssignment struct {
	Statement       string            `json:"statement"`
	Epoch           uint16            `json:"epoch"`
	SeedHex         string            `json:"seedHex"`
	ShortFlipsCount int               `json:"shortFlipsCount"`
	Candidates      int               `json:"candidates"`
	Shards          []ShardAssignment `json:"shards"`
	Indexer         *IndexerCheck     `json:"indexer,omitempty"`
}

// IndexerCheck is what a public indexer said about the same epoch, and how much
// of it the derived assignment reproduced.
//
// An indexer is a Plan B source, never an authority: `SPEC.md` 5.3 rules an
// explorer response insufficient on its own. What makes it usable is that the
// per-identity answer counts are a consequence of the assignment, so an
// assignment derived from a captured candidate list has to reproduce them for
// every candidate. A mismatch is a reason to stop, not to prefer either side.
type IndexerCheck struct {
	Source          string     `json:"source"`
	EpochStateRoot  string     `json:"epochStateRoot,omitempty"`
	EpochOpenHeight uint64     `json:"epochOpenHeight,omitempty"`
	EpochOpenSource string     `json:"epochOpenHeightSource,omitempty"`
	NodeVerified    bool       `json:"nodeVerified"`
	NodeNote        string     `json:"nodeNote,omitempty"`
	Records         int        `json:"records"`
	Matched         int        `json:"matched"`
	Mismatched      int        `json:"mismatched"`
	Missing         int        `json:"missing"`
	Flips           *FlipCheck `json:"coveredFlips,omitempty"`
	Note            string     `json:"note,omitempty"`
}

// FlipCheck is the second half of an indexer cross-check: what the explorer
// publishes for the flips the bundle would cover, compared with the
// reconstruction's own verdicts.
//
// The explorer derives these from the node's own qualification result, so a
// status or an answer that disagrees with the reconstruction means one of the two
// is wrong. The lists name what disagreed rather than only counting it, because
// a refusal has to say which flip it is about.
type FlipCheck struct {
	Source     string   `json:"source"`
	Read       int      `json:"read"`
	Matched    int      `json:"matched"`
	Mismatched []string `json:"mismatched,omitempty"`
	Missing    []string `json:"missing,omitempty"`
	// NoResult names covered flips the explorer lists but publishes no
	// qualification result for. That is not agreement: without a result there is
	// nothing that says the flip completed validation.
	NoResult []string `json:"noPublishedResult,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// EpochStateCheck records whether the chain has finished an epoch, which is the
// precondition `SPEC.md` 5.2 puts on every covered flip: a successfully
// completed validation. An epoch still being validated cannot satisfy it - the
// answers are still being collected - so a capture taken now would be a snapshot
// of an unfinished ceremony.
type EpochStateCheck struct {
	Source    string `json:"source,omitempty"`
	NodeEpoch uint16 `json:"nodeEpoch,omitempty"`
	Verified  bool   `json:"verified"`
	Note      string `json:"note,omitempty"`
}

// epochCompletionCheck asks the node which epoch it is in. An epoch the node has
// already left has ended; the epoch it is in has not. A node that does not answer
// leaves the question open, which is recorded as such rather than assumed away.
func epochCompletionCheck(node *client, epoch uint16) (EpochStateCheck, error) {
	check := EpochStateCheck{}
	if node == nil {
		check.Note = "not checked: no node was named, so nothing here says the epoch has ended"
		return check, nil
	}
	var answer struct {
		Epoch uint16 `json:"epoch"`
	}
	if err := node.call("dna_epoch", []any{}, &answer); err != nil {
		check.Note = fmt.Sprintf("not checked: the node at %s did not answer: %v", node.url, err)
		return check, nil
	}
	check.Source = node.url
	check.NodeEpoch = answer.Epoch
	if answer.Epoch == 0 {
		check.Note = "not checked: the node named no epoch"
		return check, nil
	}
	if epoch >= answer.Epoch {
		return check, fmt.Errorf(
			"epoch %d has not ended: the node is in epoch %d, so its validation has not finished and no bundle from it can be eligible",
			epoch, answer.Epoch)
	}
	check.Verified = true
	check.Note = fmt.Sprintf("checked: the node is in epoch %d, so epoch %d has ended", answer.Epoch, epoch)
	return check, nil
}

// nodeRootCheck compares an explorer's epoch state root with the header a node
// holds at the height that root belongs to.
//
// This is the check `SPEC.md` 5.3 asks of a Plan B source: the root is the one
// field of an explorer answer anybody who runs a node can read for themselves,
// and a node keeps headers even when it prunes state, so the comparison still
// works long after the epoch closed. A disagreement refuses; a node that is not
// there leaves the root unverified, which is recorded as such rather than being
// glossed over as agreement.
//
// The height has to be the epoch's opening block, not its first validation block
// - measured on mainnet, see `SOURCES.md` F-82. An explorer that publishes a
// root is publishing the state the epoch began from.
func nodeRootCheck(node *client, root string, height uint64) (bool, string, error) {
	if node == nil {
		return false, "not checked: no node was named to compare with", nil
	}
	if root == "" || height == 0 {
		return false, "not checked: the state root and the height it belongs to are not both known", nil
	}
	var header rpcBlock
	if err := node.call("bcn_blockAt", []any{height}, &header); err != nil {
		return false, fmt.Sprintf("not checked: the node at %s did not answer: %v", node.url, err), nil
	}
	if !strings.EqualFold(header.Root, root) {
		return false, "", fmt.Errorf(
			"the explorer's state root %s is not the root of block %d (%s): the epoch is refused",
			root, height, header.Root)
	}
	return true, fmt.Sprintf("checked: block %d is the header the node itself holds, and its root matches the explorer's", height), nil
}

// assignmentFor derives the assignment for every shard of one epoch.
//
// Candidates are grouped by their shifted shard, and the order inside a shard is
// the order they appear in the store, which is the order the ceremony used.
func assignmentFor(epoch uint16, seed []byte, identities []CapturedIdentity, shortFlipsCount int) (*EpochAssignment, error) {
	if len(identities) == 0 {
		return nil, fmt.Errorf("epoch %d has no candidates to assign", epoch)
	}
	if len(seed) < 8 {
		return nil, fmt.Errorf("epoch %d has a %d-byte lottery seed", epoch, len(seed))
	}

	order := []uint8{}
	byShard := map[uint8][]CapturedIdentity{}
	for _, identity := range identities {
		if _, seen := byShard[identity.Shard]; !seen {
			order = append(order, identity.Shard)
		}
		byShard[identity.Shard] = append(byShard[identity.Shard], identity)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	out := &EpochAssignment{
		Statement:       "fod:epoch-assignment:v1",
		Epoch:           epoch,
		SeedHex:         hex.EncodeToString(seed),
		ShortFlipsCount: shortFlipsCount,
		Candidates:      len(identities),
		Shards:          []ShardAssignment{},
	}
	for _, shard := range order {
		group := byShard[shard]
		candidates := make([]LotteryCandidate, 0, len(group))
		assignment := ShardAssignment{
			Shard:      shard,
			Addresses:  make([]string, 0, len(group)),
			FlipCounts: make([]int, 0, len(group)),
		}
		for _, identity := range group {
			flips := make([][]byte, 0, len(identity.FlipCids))
			for _, cid := range identity.FlipCids {
				flips = append(flips, []byte(cid))
			}
			candidates = append(candidates, LotteryCandidate{
				IsAuthor: len(flips) > 0,
				Flips:    flips,
			})
			assignment.Addresses = append(assignment.Addresses, identity.Address)
			assignment.FlipCounts = append(assignment.FlipCounts, len(flips))
		}
		derived, err := AssignFlips(candidates, seed, shortFlipsCount)
		if err != nil {
			return nil, fmt.Errorf("shard %d: %w", shard, err)
		}
		assignment.AuthorsPerCandidate = derived.AuthorsPerCandidate
		assignment.CandidatesPerAuthor = derived.CandidatesPerAuthor
		assignment.ShortFlipsPerCandidate = derived.ShortFlipsPerCandidate
		assignment.LongFlipsPerCandidate = derived.LongFlipsPerCandidate
		out.Shards = append(out.Shards, assignment)
	}
	return out, nil
}

// buildEpochAssignment reads one epoch's capture from a node datadir and derives
// the assignment. The datadir is opened read-only; a datadir a node currently
// holds cannot be opened at all, because the database keeps its lock, so a capture
// is either taken from a stopped node or from a copy of its database.
func buildEpochAssignment(dataDir string, epoch uint16) (*EpochAssignment, error) {
	if err := requireSupportedEpoch(epoch); err != nil {
		return nil, err
	}
	seed, identities, err := readLotteryRecords(dataDir, epoch)
	if err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		return nil, fmt.Errorf(
			"epoch %d has no lottery identities in %s: a node records them only when it is live at that epoch's FlipLotteryStarted block",
			epoch, dataDir)
	}
	captured := make([]CapturedIdentity, 0, len(identities))
	for _, identity := range identities {
		cids := make([]string, 0, len(identity.FlipCids))
		for _, cid := range identity.FlipCids {
			cids = append(cids, cidString(cid))
		}
		captured = append(captured, CapturedIdentity{
			Address:  identity.Address.String(),
			Shard:    uint8(identity.ShiftedShardId),
			FlipCids: cids,
		})
	}
	return assignmentFor(epoch, seed, captured, defaultShortFlipsCount())
}

// defaultShortFlipsCount is the short session the node derives an assignment
// with. It is the sum of two constants, not the first one alone
// (`core/ceremony/ceremony.go`, `shortFlipsCount := ShortSessionFlipsCount() +
// ShortSessionExtraFlipsCount()`), which is eight for the epochs this release
// supports. The public explorer's per-identity `shortAnswersCount` agrees.
func defaultShortFlipsCount() int {
	return int(common.ShortSessionFlipsCount() + common.ShortSessionExtraFlipsCount())
}

// indexerIdentity is one record of the public explorer's `/Epoch/{n}/Identities`,
// the Plan B source for an epoch whose capture is unavailable. The answer counts
// are a consequence of the assignment, which is what makes them checkable.
type indexerIdentity struct {
	Address           string `json:"address"`
	ShortAnswersCount uint32 `json:"shortAnswersCount"`
	LongAnswersCount  uint32 `json:"longAnswersCount"`
	Approved          bool   `json:"approved"`
	Missed            bool   `json:"missed"`
}

type indexerIdentityPage struct {
	Result            []indexerIdentity `json:"result"`
	ContinuationToken *string           `json:"continuationToken"`
}

const (
	indexerPageLimit = 100
	indexerMaxPages  = 200
)

// fetchIndexerIdentities walks the explorer's paginated identity list for one
// epoch. The list is sorted by address, not by the ceremony's order, so it can
// check an assignment but cannot replace the capture that produces one.
func fetchIndexerIdentities(c *httpClient, base string, epoch uint16) ([]indexerIdentity, error) {
	base = strings.TrimRight(base, "/")
	all := []indexerIdentity{}
	token := ""
	for page := 0; page < indexerMaxPages; page++ {
		endpoint := fmt.Sprintf("%s/Epoch/%d/Identities?limit=%d", base, epoch, indexerPageLimit)
		if token != "" {
			endpoint += "&continuationToken=" + url.QueryEscape(token)
		}
		var answer indexerIdentityPage
		if err := c.get(endpoint, &answer); err != nil {
			return nil, err
		}
		all = append(all, answer.Result...)
		if answer.ContinuationToken == nil || *answer.ContinuationToken == "" {
			return all, nil
		}
		token = *answer.ContinuationToken
	}
	return nil, fmt.Errorf("the indexer kept paginating past %d pages", indexerMaxPages)
}

// fetchIndexerStateRoot reads the epoch's committed state root, which is the one
// field of an explorer answer this repository treats as a commitment: a node can
// compare it with its own header at the epoch's opening block.
func fetchIndexerStateRoot(c *httpClient, base string, epoch uint16) (string, error) {
	var answer struct {
		Result struct {
			StateRoot string `json:"stateRoot"`
		} `json:"result"`
	}
	if err := c.get(fmt.Sprintf("%s/Epoch/%d", strings.TrimRight(base, "/"), epoch), &answer); err != nil {
		return "", err
	}
	return answer.Result.StateRoot, nil
}

// fetchIndexerEpochOpeningBlock asks the explorer which block an epoch's state
// root belongs to.
//
// The answer is the explorer's own block list for the epoch *before* it: that
// list is ordered newest first, so its first entry is the block the earlier
// epoch ended on, and the state after that block is the state the later epoch
// began from. The explorer labels the epoch's blocks from the validation
// onwards, so the opening block is filed under the previous epoch - which is
// why this reads it that way rather than from the epoch's own list
// (`SOURCES.md` F-82).
func fetchIndexerEpochOpeningBlock(c *httpClient, base string, epoch uint16) (uint64, error) {
	if epoch == 0 {
		return 0, errors.New("epoch 0 has no opening block to look up")
	}
	var answer struct {
		Result []struct {
			Height uint64 `json:"height"`
		} `json:"result"`
	}
	endpoint := fmt.Sprintf("%s/Epoch/%d/Blocks?limit=1", strings.TrimRight(base, "/"), epoch-1)
	if err := c.get(endpoint, &answer); err != nil {
		return 0, err
	}
	if len(answer.Result) == 0 || answer.Result[0].Height == 0 {
		return 0, fmt.Errorf("the indexer lists no blocks for epoch %d, which is where epoch %d's state root belongs", epoch-1, epoch)
	}
	return answer.Result[0].Height, nil
}

// crossCheckAssignment compares a derived assignment with what the explorer
// publishes for the same epoch.
//
// The comparison is one-sided on purpose: a candidate who submitted answers must
// have exactly as many of them as the assignment gives that candidate flips, so a
// count that disagrees means the assignment (or the record) is wrong. A candidate
// with no answers is not evidence either way, so it is counted as unanswered
// rather than as a match. The result is recorded, never assumed.
func crossCheckAssignment(c *httpClient, base string, assignment *EpochAssignment, node *client, openingBlock uint64) (*IndexerCheck, error) {
	records, err := fetchIndexerIdentities(c, base, assignment.Epoch)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("the indexer lists no identities for epoch %d", assignment.Epoch)
	}
	byAddress := make(map[string]indexerIdentity, len(records))
	for _, record := range records {
		byAddress[strings.ToLower(strings.TrimPrefix(record.Address, "0x"))] = record
	}
	check := &IndexerCheck{Source: base, Records: len(records)}
	if root, err := fetchIndexerStateRoot(c, base, assignment.Epoch); err == nil {
		check.EpochStateRoot = root
		height := openingBlock
		if height != 0 {
			check.EpochOpenSource = "the height the caller named"
		} else if height, err = fetchIndexerEpochOpeningBlock(c, base, assignment.Epoch); err != nil {
			check.EpochOpenSource = fmt.Sprintf("not resolved: %v", err)
		} else {
			check.EpochOpenSource = fmt.Sprintf("the first block the explorer lists for epoch %d", assignment.Epoch-1)
		}
		check.EpochOpenHeight = height
		verified, note, err := nodeRootCheck(node, root, height)
		if err != nil {
			return nil, err
		}
		check.NodeVerified = verified
		check.NodeNote = note
	}
	unanswered := 0
	for _, shard := range assignment.Shards {
		for index, address := range shard.Addresses {
			record, ok := byAddress[strings.ToLower(strings.TrimPrefix(address, "0x"))]
			if !ok {
				check.Missing++
				continue
			}
			short := len(shard.ShortFlipsPerCandidate[index])
			long := len(shard.LongFlipsPerCandidate[index])
			if record.ShortAnswersCount == 0 && record.LongAnswersCount == 0 {
				unanswered++
				continue
			}
			if int(record.ShortAnswersCount) == short && int(record.LongAnswersCount) == long {
				check.Matched++
			} else {
				check.Mismatched++
			}
		}
	}
	check.Note = fmt.Sprintf(
		"candidates that answered must match the assignment exactly; %d answered and did, %d answered and did not, %d did not answer, %d are not in the indexer",
		check.Matched, check.Mismatched, unanswered, check.Missing)
	return check, nil
}

func runAssignment(dataDir string, epoch uint16, outPath, indexerURL string, node *client, openingBlock uint64) {
	assignment, err := buildEpochAssignment(dataDir, epoch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "assignment failed: %v\n", err)
		os.Exit(1)
	}
	if indexerURL != "" {
		check, err := crossCheckAssignment(newHTTPClient(), indexerURL, assignment, node, openingBlock)
		if err != nil {
			fmt.Fprintf(os.Stderr, "the indexer cross-check failed: %v\n", err)
			os.Exit(1)
		}
		assignment.Indexer = check
		if check.Mismatched > 0 || check.Missing > 0 {
			fmt.Fprintf(os.Stderr,
				"assignment refused: %s\n", check.Note)
			os.Exit(1)
		}
		fmt.Printf("indexer   %s: %s\n", check.Source, check.Note)
		fmt.Printf("node      %s\n", check.NodeNote)
	}
	if err := writeJSON(outPath, assignment); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", outPath, err)
		os.Exit(1)
	}
	authors := 0
	for _, shard := range assignment.Shards {
		for _, count := range shard.FlipCounts {
			if count > 0 {
				authors++
			}
		}
	}
	fmt.Printf("epoch=%d seed_bytes=%d candidates=%d shards=%d authors=%d -> %s\n",
		assignment.Epoch, len(assignment.SeedHex)/2, assignment.Candidates,
		len(assignment.Shards), authors, outPath)
}
