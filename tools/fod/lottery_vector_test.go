package main

// The lottery port against vectors captured from the pinned node.
//
// `proofs/registration/vectors/lottery-v1.json` holds inputs and the assignment
// the pinned implementation computes for them, captured by running that code
// unchanged. Every field has to match: a port that agrees on the short session
// but not on the long one would still map answers to the wrong flips.

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type lotteryVectorFile struct {
	Statement string          `json:"statement"`
	Cases     []lotteryVector `json:"cases"`
}

type lotteryVector struct {
	Name            string               `json:"name"`
	Seed            string               `json:"seed"`
	ShortFlipsCount int                  `json:"shortFlipsCount"`
	Shards          []lotteryVectorShard `json:"shards"`
}

type lotteryVectorShard struct {
	ID                     uint32              `json:"id"`
	Candidates             []lotteryVectorFlip `json:"candidates"`
	AuthorsPerCandidate    map[int][]int       `json:"authorsPerCandidate"`
	CandidatesPerAuthor    map[int][]int       `json:"candidatesPerAuthor"`
	ShortFlipsPerCandidate [][]int             `json:"shortFlipsPerCandidate"`
	LongFlipsPerCandidate  [][]int             `json:"longFlipsPerCandidate"`
}

type lotteryVectorFlip struct {
	IsAuthor bool     `json:"isAuthor"`
	Flips    []string `json:"flips"`
}

func TestLotteryVector(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/registration/vectors/lottery-v1.json")
	if err != nil {
		t.Fatalf("read the vector: %v", err)
	}
	var file lotteryVectorFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse the vector: %v", err)
	}
	if file.Statement != "fod:lottery:v1" {
		t.Fatalf("the vector is %q, not the lottery vector", file.Statement)
	}
	if len(file.Cases) == 0 {
		t.Fatal("the vector has no cases")
	}
	assigned := 0
	for _, vector := range file.Cases {
		seed, err := hex.DecodeString(vector.Seed)
		if err != nil {
			t.Fatalf("%s: seed: %v", vector.Name, err)
		}
		for _, shard := range vector.Shards {
			input := make([]LotteryCandidate, 0, len(shard.Candidates))
			for _, candidate := range shard.Candidates {
				entry := LotteryCandidate{IsAuthor: candidate.IsAuthor}
				for _, flip := range candidate.Flips {
					entry.Flips = append(entry.Flips, []byte(flip))
				}
				input = append(input, entry)
			}
			got, err := AssignFlips(input, seed, vector.ShortFlipsCount)
			if err != nil {
				t.Fatalf("%s shard %d: %v", vector.Name, shard.ID, err)
			}
			assigned += len(got.AuthorsPerCandidate)
			at := func(field string) string { return field + " of " + vector.Name }
			if !sameIntMap(got.AuthorsPerCandidate, shard.AuthorsPerCandidate) {
				t.Errorf("%s: authorsPerCandidate is %v, the vector says %v",
					at("authorsPerCandidate"), got.AuthorsPerCandidate, shard.AuthorsPerCandidate)
			}
			if !sameIntMap(got.CandidatesPerAuthor, shard.CandidatesPerAuthor) {
				t.Errorf("%s: candidatesPerAuthor is %v, the vector says %v",
					at("candidatesPerAuthor"), got.CandidatesPerAuthor, shard.CandidatesPerAuthor)
			}
			if !sameIntLists(got.ShortFlipsPerCandidate, shard.ShortFlipsPerCandidate) {
				t.Errorf("%s: the short session is %v, the vector says %v",
					at("the short session"), got.ShortFlipsPerCandidate, shard.ShortFlipsPerCandidate)
			}
			if !sameIntLists(got.LongFlipsPerCandidate, shard.LongFlipsPerCandidate) {
				t.Errorf("%s: the long session is %v, the vector says %v",
					at("the long session"), got.LongFlipsPerCandidate, shard.LongFlipsPerCandidate)
			}
		}
	}
	// A vector that assigned nobody would pass every comparison above while
	// testing nothing.
	if assigned == 0 {
		t.Fatal("no case in the vector assigned a flip")
	}
}

// sameIntMap compares two assignments. An absent key and an empty list mean the
// same thing to a caller, and the reference produces both.
func sameIntMap(got, want map[int][]int) bool {
	if len(got) != len(want) {
		return false
	}
	for key, wantList := range want {
		if !reflect.DeepEqual(emptyIfNil(got[key]), emptyIfNil(wantList)) {
			return false
		}
	}
	return true
}

func sameIntLists(got, want [][]int) bool {
	if len(got) != len(want) {
		return false
	}
	for idx := range want {
		if !reflect.DeepEqual(emptyIfNil(got[idx]), emptyIfNil(want[idx])) {
			return false
		}
	}
	return true
}

func emptyIfNil(values []int) []int {
	if values == nil {
		return []int{}
	}
	return values
}
