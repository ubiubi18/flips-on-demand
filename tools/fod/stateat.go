package main

import (
	"fmt"

	"github.com/cosmos/iavl"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/database"
	"github.com/syndtr/goleveldb/leveldb/opt"
	dbm "github.com/tendermint/tm-db"
)

// StateAtSummary is the historical identity state at one height: who existed,
// who was a ceremony candidate, and which flips they held.
//
// This mirrors what the reference ceremony does when it builds its candidate
// list, but at a chosen past height instead of the node's head.
type StateAtSummary struct {
	DataDir      string             `json:"dataDir"`
	Height       int64              `json:"height"`
	Identities   int                `json:"identities"`
	Candidates   int                `json:"candidates"`
	TotalFlips   int                `json:"candidateFlips"`
	StateCounts  map[string]int     `json:"stateCounts"`
	ShardCounts  map[string]int     `json:"shardCounts"`
	CandidatesL  []StateCandidate   `json:"candidateSample"`
	IdentityRoot string             `json:"identityRootNotIncluded"`
}

// StateCandidate is one eligible identity at that height.
type StateCandidate struct {
	Address   string   `json:"address"`
	State     string   `json:"state"`
	Shard     uint8    `json:"shard"`
	Flips     int      `json:"flips"`
	FlipCIDs  []string `json:"flipCids"`
	DoneFlips bool     `json:"hasDoneAllRequiredFlips"`
}

// readStateAt opens the chain database read-only and reads the identity state at
// a historical height. The state tree is versioned, so past heights are
// readable as long as the node retained them.
func readStateAt(dataDir string, height int64, sampleLimit int) (*StateAtSummary, error) {
	opened, err := openTree(dataDir, "state")
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	// A fresh handle knows nothing until it loads the roots the datadir holds.
	if _, err := opened.tree.Load(); err != nil {
		return nil, fmt.Errorf("no state versions in %s: %w", dataDir, err)
	}
	if _, err := opened.tree.LoadVersion(height); err != nil {
		return nil, fmt.Errorf("load state at height %d: %w", height, err)
	}
	imm, err := opened.tree.GetImmutable(height)
	if err != nil {
		return nil, fmt.Errorf("state at height %d: %w", height, err)
	}
	summary, err := collectStateIdentities(imm, sampleLimit)
	if err != nil {
		return nil, err
	}
	summary.DataDir = dataDir
	summary.Height = height
	return summary, nil
}

// collectStateIdentities walks every identity record in one loaded state
// version. The walk continues for every key: an earlier revision returned stop
// for each key, so every datadir reported exactly one identity.
func collectStateIdentities(imm *iavl.ImmutableTree, sampleLimit int) (*StateAtSummary, error) {
	summary := &StateAtSummary{
		StateCounts: map[string]int{},
		ShardCounts: map[string]int{},
		CandidatesL: []StateCandidate{},
	}

	iterateIdentityRange(imm, func(key []byte, value []byte) bool {
		if key == nil {
			return false
		}
		summary.Identities++
		addr := state.StateDbKeys.IdentityKeyToAddress(key)
		var identity state.Identity
		if err := identity.FromBytes(value); err != nil {
			return false
		}
		summary.StateCounts[stateName(uint8(identity.State))]++
		if !state.IsCeremonyCandidateData(identity.State, identity.HasDoneAllRequiredFlips()) {
			return false
		}
		shard := uint8(identity.ShiftedShardId())
		candidate := StateCandidate{
			Address:   addr.String(),
			State:     stateName(uint8(identity.State)),
			Shard:     shard,
			Flips:     len(identity.Flips),
			DoneFlips: identity.HasDoneAllRequiredFlips(),
			FlipCIDs:  []string{},
		}
		for _, flip := range identity.Flips {
			candidate.FlipCIDs = append(candidate.FlipCIDs, cidString(flip.Cid))
		}
		summary.Candidates++
		summary.TotalFlips += candidate.Flips
		summary.ShardCounts[fmt.Sprintf("%d", shard)]++
		if len(summary.CandidatesL) < sampleLimit {
			summary.CandidatesL = append(summary.CandidatesL, candidate)
		}
		return false
	})
	return summary, nil
}

// BlockSeedAt is the lottery seed source for one height.
//
// The ceremony uses the block seed at `flipLotteryStartHeight - LotterySeedLag`,
// so the seed is public chain data; this reads it straight from the block header
// in the node's database.
type BlockSeedAt struct {
	DataDir    string `json:"dataDir"`
	Height     uint64 `json:"height"`
	BlockHash  string `json:"blockHash"`
	SeedHex    string `json:"seedHex"`
	SeedBytes  int    `json:"seedBytes"`
}

// readBlockSeedAt reads the canonical block header at a height and returns its
// seed. The database is opened read-only.
func readBlockSeedAt(dataDir string, height uint64) (*BlockSeedAt, error) {
	db, err := dbm.NewGoLevelDBWithOpts("idenachain", dataDir, &opt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open chain db read-only: %w", err)
	}
	defer db.Close()

	repo := database.NewRepo(db)
	hash := repo.ReadCanonicalHash(height)
	if hash == (common.Hash{}) {
		return nil, fmt.Errorf("no canonical block at height %d in this database", height)
	}
	header := repo.ReadBlockHeader(hash)
	if header == nil {
		return nil, fmt.Errorf("block header %s (height %d) not found", hash.String(), height)
	}
	seed := header.Seed().Bytes()
	return &BlockSeedAt{
		DataDir:   dataDir,
		Height:    height,
		BlockHash: hash.String(),
		SeedHex:   fmt.Sprintf("%x", seed),
		SeedBytes: len(seed),
	}, nil
}
