package main

// The epoch-store shape turned into the lottery, checked against the frozen
// case: the same candidates, the same seed, and the assignment the pinned node
// computes for them.

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/common"
)

func TestIndexerCrossCheckComparesAnswerCounts(t *testing.T) {
	assignment := &EpochAssignment{
		Epoch: 7,
		Shards: []ShardAssignment{{
			Shard:                  1,
			Addresses:              []string{"0xaa", "0xbb", "0xcc", "0xdd"},
			ShortFlipsPerCandidate: [][]int{{0, 1, 2, 3, 4, 5, 6, 7}, {0}, {0}, {0}},
			LongFlipsPerCandidate:  [][]int{{0, 1, 2}, {0}, {0}, {0}},
		}},
	}

	// Two pages, so the continuation token is exercised too.
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		switch {
		case r.URL.Path == "/Epoch/7/Identities" && r.URL.Query().Get("continuationToken") == "":
			io.WriteString(w, `{"result":[
				{"address":"aa","shortAnswersCount":8,"longAnswersCount":3,"approved":true},
				{"address":"bb","shortAnswersCount":7,"longAnswersCount":3,"approved":false}
			],"continuationToken":"more"}`)
		case r.URL.Path == "/Epoch/7/Identities":
			io.WriteString(w, `{"result":[
				{"address":"cc","shortAnswersCount":0,"longAnswersCount":0,"missed":true}
			],"continuationToken":""}`)
		case r.URL.Path == "/Epoch/7":
			io.WriteString(w, `{"result":{"epoch":7,"stateRoot":"0xabc"}}`)
		case r.URL.Path == "/Epoch/6/Blocks":
			io.WriteString(w, `{"result":[{"height":99,"timestamp":"2026-09-25T23:09:04Z"}],"continuationToken":98}`)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	check, err := crossCheckAssignment(newHTTPClient(), server.URL, assignment, nil, 0)
	if err != nil {
		t.Fatalf("cross-check: %v", err)
	}
	if check.Records != 3 || check.Matched != 1 || check.Mismatched != 1 || check.Missing != 1 {
		t.Fatalf("the check read %+v, expected 3 records, 1 match, 1 mismatch, 1 missing", check)
	}
	if check.EpochStateRoot != "0xabc" {
		t.Fatalf("the state root is %q", check.EpochStateRoot)
	}
	if !strings.Contains(check.Note, "did not answer") {
		t.Fatalf("the note does not account for the candidate who did not answer: %q", check.Note)
	}
	if len(paths) != 4 || !strings.Contains(paths[0], "limit=100") || !strings.Contains(paths[1], "continuationToken=more") {
		t.Fatalf("the client did not walk the pages as expected: %v", paths)
	}
	if !strings.Contains(paths[3], "/Epoch/6/Blocks") || check.EpochOpenHeight != 99 {
		t.Fatalf("the epoch's opening block was not looked up: %v, %+v", paths, check)
	}
}

func TestIndexerCrossCheckRefusesAnEpochTheIndexerDoesNotList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":[],"continuationToken":""}`)
	}))
	defer server.Close()

	_, err := crossCheckAssignment(newHTTPClient(), server.URL, &EpochAssignment{Epoch: 1}, nil, 0)
	if err == nil || !strings.Contains(err.Error(), "lists no identities") {
		t.Fatalf("an epoch the indexer does not list was accepted, or the refusal is unclear: %v", err)
	}
}

func TestAssignmentFromACapturedCandidateList(t *testing.T) {
	vector := readLotteryCase(t, "seven-candidates-three-authors")
	shard := vector.Shards[0]
	seed, err := hex.DecodeString(vector.Seed)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The store carries addresses where the lottery vector carries positions.
	captured := make([]CapturedIdentity, 0, len(shard.Candidates))
	for index, candidate := range shard.Candidates {
		address := common.Address{}
		address[19] = byte(index + 1)
		entry := CapturedIdentity{Address: address.String(), Shard: uint8(shard.ID)}
		if candidate.IsAuthor {
			entry.FlipCids = append([]string{}, candidate.Flips...)
		}
		captured = append(captured, entry)
	}

	assignment, err := assignmentFor(204, seed, captured, vector.ShortFlipsCount)
	if err != nil {
		t.Fatalf("assignment: %v", err)
	}
	if assignment.Statement != "fod:epoch-assignment:v1" || assignment.Candidates != 7 {
		t.Fatalf("the assignment describes %+v", assignment)
	}
	if len(assignment.Shards) != 1 {
		t.Fatalf("the assignment carries %d shards, expected 1", len(assignment.Shards))
	}
	derived := assignment.Shards[0]
	if !sameIntMap(derived.AuthorsPerCandidate, shard.AuthorsPerCandidate) {
		t.Errorf("authorsPerCandidate is %v, the vector says %v", derived.AuthorsPerCandidate, shard.AuthorsPerCandidate)
	}
	if !sameIntMap(derived.CandidatesPerAuthor, shard.CandidatesPerAuthor) {
		t.Errorf("candidatesPerAuthor is %v, the vector says %v", derived.CandidatesPerAuthor, shard.CandidatesPerAuthor)
	}
	if !sameIntLists(derived.ShortFlipsPerCandidate, shard.ShortFlipsPerCandidate) {
		t.Errorf("the short session is %v, the vector says %v", derived.ShortFlipsPerCandidate, shard.ShortFlipsPerCandidate)
	}
	if !sameIntLists(derived.LongFlipsPerCandidate, shard.LongFlipsPerCandidate) {
		t.Errorf("the long session is %v, the vector says %v", derived.LongFlipsPerCandidate, shard.LongFlipsPerCandidate)
	}
	if len(derived.Addresses) != 7 || derived.FlipCounts[0] != 2 {
		t.Errorf("the captured list is not carried through: %v %v", derived.Addresses, derived.FlipCounts)
	}
}

func TestAssignmentRefusesAnEmptyCapture(t *testing.T) {
	seed := make([]byte, 32)
	if _, err := assignmentFor(204, seed, nil, 6); err == nil {
		t.Fatal("an empty capture was assigned")
	}
	if _, err := assignmentFor(204, seed[:4], []CapturedIdentity{{Address: "x", Shard: 1}}, 6); err == nil {
		t.Fatal("a short seed was accepted")
	}
}

func TestAssignmentFileCarriesWhatThePipelineNeeds(t *testing.T) {
	// The written shape is a contract with whatever reads it next: the seed, the
	// short-session size, and every shard's candidate order and four assignments.
	raw, err := os.ReadFile("../../proofs/registration/vectors/lottery-v1.json")
	if err != nil {
		t.Fatalf("read the lottery vector: %v", err)
	}
	var file lotteryVectorFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse the lottery vector: %v", err)
	}
	seed, err := hex.DecodeString(file.Cases[0].Seed)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	captured := []CapturedIdentity{{Address: "a", Shard: 1, FlipCids: []string{"one", "two"}}}
	assignment, err := assignmentFor(7, seed, captured, 6)
	if err != nil {
		t.Fatalf("assignment: %v", err)
	}
	encoded, err := json.Marshal(assignment)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, key := range []string{"seedHex", "shortFlipsCount", "shards", "addresses", "flipCounts",
		"authorsPerCandidate", "candidatesPerAuthor", "shortFlipsPerCandidate", "longFlipsPerCandidate"} {
		if !strings.Contains(string(encoded), key) {
			t.Errorf("the assignment file has no %q", key)
		}
	}
}
