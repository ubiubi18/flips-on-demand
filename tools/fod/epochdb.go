package main

import (
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/database"
	"github.com/syndtr/goleveldb/leveldb/opt"
	dbm "github.com/tendermint/tm-db"
)

// EpochDbSummary is the reconstruction input that only the node's epoch store
// holds: the lottery seed and the per-candidate flip assignment.
//
// The database is opened read-only, so a node's datadir is never modified by
// reading it.
type EpochDbSummary struct {
	DataDir     string                `json:"dataDir"`
	Epoch       uint16                `json:"epoch"`
	SeedHex     string                `json:"seedHex"`
	SeedBytes   int                   `json:"seedBytes"`
	Candidates  int                   `json:"candidates"`
	TotalFlips  int                   `json:"totalFlipsAssigned"`
	ShardCounts map[string]int        `json:"shardCounts"`
	StateCounts map[string]int        `json:"stateCounts"`
	Identities  []LotteryIdentityInfo `json:"identities"`
}

// LotteryIdentityInfo is one candidate with their assigned flips.
type LotteryIdentityInfo struct {
	Address      string   `json:"address"`
	Shard        uint8    `json:"shard"`
	State        uint8    `json:"state"`
	Flips        int      `json:"flips"`
	DoneAllFlips bool     `json:"hasDoneAllRequiredFlips"`
	PublicKey    string   `json:"publicKeyHex"`
	FlipCIDs     []string `json:"flipCids"`
}

// readEpochDb opens the chain database read-only and extracts one epoch's
// lottery data.
func readEpochDb(dataDir string, epoch uint16) (*EpochDbSummary, error) {
	seed, identities, err := readLotteryRecords(dataDir, epoch)
	if err != nil {
		return nil, err
	}
	summary := &EpochDbSummary{
		DataDir:     dataDir,
		Epoch:       epoch,
		ShardCounts: map[string]int{},
		StateCounts: map[string]int{},
		Identities:  []LotteryIdentityInfo{},
	}
	summary.SeedHex = fmt.Sprintf("%x", seed)
	summary.SeedBytes = len(seed)

	for _, identity := range identities {
		info := LotteryIdentityInfo{
			Address:      identity.Address.String(),
			Shard:        uint8(identity.ShiftedShardId),
			State:        identity.State,
			Flips:        len(identity.FlipCids),
			DoneAllFlips: identity.HasDoneAllRequiredFlips,
			PublicKey:    fmt.Sprintf("%x", identity.PubKey),
			FlipCIDs:     make([]string, 0, len(identity.FlipCids)),
		}
		for _, cid := range identity.FlipCids {
			info.FlipCIDs = append(info.FlipCIDs, cidString(cid))
		}
		summary.TotalFlips += info.Flips
		summary.ShardCounts[fmt.Sprintf("%d", info.Shard)]++
		summary.StateCounts[stateName(info.State)]++
		summary.Identities = append(summary.Identities, info)
	}
	summary.Candidates = len(summary.Identities)
	return summary, nil
}

// readLotteryRecords opens the chain database read-only and returns one epoch's
// lottery seed and candidate list in the order the node stored them.
//
// The order is part of the data: the lottery and every answer index into it, so
// a reader that reorders the list reconstructs a different assignment.
func readLotteryRecords(dataDir string, epoch uint16) ([]byte, []database.DbLotteryIdentity, error) {
	db, err := dbm.NewGoLevelDBWithOpts("idenachain", dataDir, &opt.Options{ReadOnly: true})
	if err != nil {
		return nil, nil, fmt.Errorf("open chain db read-only: %w", err)
	}
	defer db.Close()
	epochDb := database.NewEpochDb(db, epoch)
	return epochDb.ReadLotterySeed(), epochDb.ReadLotteryIdentities(), nil
}

// readEpochAnswers opens the chain database read-only and returns the epoch's
// stored answers, keyed by the address that sent them.
//
// The node keeps what it saw during the ceremony, so these are as complete as
// that node's view was: they are answers, in the order the assignment gave each
// candidate its flips, and they carry no flip identifiers of their own (F-79).
func readEpochAnswers(dataDir string, epoch uint16) (map[common.Address][]byte, error) {
	db, err := dbm.NewGoLevelDBWithOpts("idenachain", dataDir, &opt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open chain db read-only: %w", err)
	}
	defer db.Close()
	epochDb := database.NewEpochDb(db, epoch)
	_, long := epochDb.ReadAnswers()
	out := make(map[common.Address][]byte, len(long))
	for _, answer := range long {
		if len(answer.Ans) > 0 {
			out[answer.Addr] = answer.Ans
		}
	}
	return out, nil
}

// stateName maps identity states to readable names for the summary.
func stateName(state uint8) string {
	switch state {
	case 1:
		return "Undefined"
	case 2:
		return "Candidate"
	case 3:
		return "Invite"
	case 4:
		return "Newbie"
	case 5:
		return "Verified"
	case 6:
		return "Human"
	case 7:
		return "Suspended"
	case 8:
		return "Zombie"
	case 9:
		return "Killed"
	default:
		return fmt.Sprintf("state(%d)", state)
	}
}

// KeyPrefixCount is one prefix bucket from a database layout dump.
type KeyPrefixCount struct {
	PrefixHex string `json:"prefixHex"`
	Count     int    `json:"count"`
	SampleHex string `json:"sampleKeyHex"`
}

// DumpDbLayout reports the key prefixes present in the chain database. It exists
// to verify assumptions about key layout against a real datadir instead of
// guessing: an epoch reader that finds nothing may be reading the wrong shape.
func dumpDbLayout(dataDir string, maxPrefixes int) (int, []KeyPrefixCount, error) {
	db, err := dbm.NewGoLevelDBWithOpts("idenachain", dataDir, &opt.Options{ReadOnly: true})
	if err != nil {
		return 0, nil, fmt.Errorf("open chain db read-only: %w", err)
	}
	defer db.Close()

	it, err := db.Iterator(nil, nil)
	if err != nil {
		return 0, nil, err
	}
	defer it.Close()

	counts := map[string]*KeyPrefixCount{}
	total := 0
	for ; it.Valid(); it.Next() {
		total++
		key := it.Key()
		width := 4
		if len(key) < width {
			width = len(key)
		}
		prefix := hex.EncodeToString(key[:width])
		bucket, ok := counts[prefix]
		if !ok {
			bucket = &KeyPrefixCount{PrefixHex: prefix, SampleHex: hex.EncodeToString(key)}
			counts[prefix] = bucket
		}
		bucket.Count++
	}
	if err := it.Error(); err != nil {
		return total, nil, err
	}

	list := make([]KeyPrefixCount, 0, len(counts))
	for _, bucket := range counts {
		list = append(list, *bucket)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Count > list[j].Count })
	if len(list) > maxPrefixes {
		list = list[:maxPrefixes]
	}
	return total, list, nil
}

// EpochKeySummary groups the keys under the "epoch" prefix by epoch number and
// sub-key name, so a datadir can be checked for the lottery records
// ("li"/"ls") without knowing an epoch number in advance.
type EpochKeySummary struct {
	DataDir string                    `json:"dataDir"`
	Epochs  map[string]map[string]int `json:"epochs"`
	Total   int                       `json:"totalEpochKeys"`
}

// readEpochKeys lists the epoch-prefixed keys in a chain database.
func readEpochKeys(dataDir string) (*EpochKeySummary, error) {
	db, err := dbm.NewGoLevelDBWithOpts("idenachain", dataDir, &opt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open chain db read-only: %w", err)
	}
	defer db.Close()

	summary := &EpochKeySummary{DataDir: dataDir, Epochs: map[string]map[string]int{}}
	prefix := []byte("epoch")
	// Iterate the whole keyspace and filter: a bounded range iterator behaved
	// inconsistently here, and the layout dump proved full iteration works.
	it, err := db.Iterator(nil, nil)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	for ; it.Valid(); it.Next() {
		key := it.Key()
		if len(key) < len(prefix) || string(key[:len(prefix)]) != "epoch" {
			continue
		}
		if len(key) < 7 {
			continue
		}
		epochNumber := int(key[5])<<8 | int(key[6])
		// Sub-keys start with a short ASCII name ("cid", "li", "ls", "hash")
		// followed by binary key material; keep only the name.
		rest := key[7:]
		end := 0
		for end < len(rest) && end < 12 && rest[end] >= 0x20 && rest[end] < 0x7f {
			end++
		}
		name := string(rest[:end])
		if name == "" {
			name = "<binary>"
		}
		bucket, ok := summary.Epochs[fmt.Sprintf("%d", epochNumber)]
		if !ok {
			bucket = map[string]int{}
			summary.Epochs[fmt.Sprintf("%d", epochNumber)] = bucket
		}
		bucket[name]++
		summary.Total++
	}
	if err := it.Error(); err != nil {
		return nil, err
	}
	return summary, nil
}
