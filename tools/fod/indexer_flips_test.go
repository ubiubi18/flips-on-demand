package main

// The Plan B cross-checks: an explorer's per-identity qualification record and an
// explorer's epoch root, each compared with something this repository can check
// for itself. The explorer is a source here, never an authority, so every test
// below is about what the tool does when the two disagree.

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoveredFlipsAreComparedWithTheExplorersRecords(t *testing.T) {
	covered := []CoveredFlip{
		{Index: 0, Cid: "bafkqaaa", Status: string(FlipQualified), Answer: "left"},
		{Index: 1, Cid: "bafkqbbb", Status: string(FlipQualified), Answer: "right"},
		{Index: 2, Cid: "bafkqccc", Status: string(FlipNotQualified), Answer: "none"},
		{Index: 3, Cid: "bafkqddd", Status: string(FlipQualified), Answer: "left"},
		{Index: 4, Cid: "bafkqeee", Status: string(FlipQualified), Answer: "left"},
	}

	// The explorer's list is two pages, and it carries a flip of somebody else
	// and a flip of another epoch, neither of which is this author's business.
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		switch {
		case r.URL.Query().Get("continuationToken") == "":
			io.WriteString(w, `{"result":[
				{"cid":"bafkqaaa","author":"0xaa","epoch":7,"status":"Qualified","answer":"Left"},
				{"cid":"bafkqbbb","author":"0xaa","epoch":7,"status":"Qualified","answer":"Left"},
				{"cid":"bafkqccc","author":"0xaa","epoch":7,"status":"NotQualified","answer":"None"}
			],"continuationToken":"more"}`)
		default:
			io.WriteString(w, `{"result":[
				{"cid":"bafkqddd","author":"0xaa","epoch":7,"status":"","answer":""},
				{"cid":"bafkqfff","author":"0xbb","epoch":7,"status":"Qualified","answer":"Left"},
				{"cid":"bafkqggg","author":"0xaa","epoch":6,"status":"Qualified","answer":"Left"}
			]}`)
		}
	}))
	defer server.Close()

	check, err := crossCheckCoveredFlips(newHTTPClient(), server.URL, 7, "0xaa", covered)
	if err != nil {
		t.Fatalf("cross-check: %v", err)
	}
	if check.Matched != 2 || len(check.NoResult) != 1 {
		t.Fatalf("the check read %+v, expected 2 matches and 1 record with no published result", check)
	}
	if len(check.Mismatched) != 1 || !strings.Contains(check.Mismatched[0], "bafkqbbb") {
		t.Fatalf("the answer that disagrees was not named: %+v", check.Mismatched)
	}
	if len(check.Missing) != 1 || !strings.Contains(check.Missing[0], "bafkqeee") {
		t.Fatalf("the flip the explorer does not list was not named: %+v", check.Missing)
	}
	if !strings.Contains(check.Note, "5 covered flips") {
		t.Fatalf("the note does not count the covered flips: %q", check.Note)
	}
	if len(paths) != 2 || !strings.Contains(paths[0], "/Epoch/7/Identity/0xaa/Flips") || !strings.Contains(paths[0], "limit=100") {
		t.Fatalf("the client did not walk the author's flips as expected: %v", paths)
	}
}

func TestEpochMustHaveEndedBeforeABundleCanBeDecided(t *testing.T) {
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"result":{"epoch":7,"startBlock":11350781},"error":null}`)
	}))
	defer node.Close()
	behind := &client{url: node.URL, http: newHTTPClient()}

	// The epoch the node is in has not finished, so nothing from it can be
	// eligible however complete its capture looks.
	if _, err := epochCompletionCheck(behind, 7); err == nil || !strings.Contains(err.Error(), "has not ended") {
		t.Fatalf("an epoch the node is still in was treated as ended: %v", err)
	}
	// So has an epoch the chain has not reached.
	if _, err := epochCompletionCheck(behind, 8); err == nil || !strings.Contains(err.Error(), "has not ended") {
		t.Fatalf("an epoch the node has not reached was treated as ended: %v", err)
	}

	check, err := epochCompletionCheck(behind, 6)
	if err != nil {
		t.Fatalf("an epoch the node has left was refused: %v", err)
	}
	if !check.Verified || check.NodeEpoch != 7 || !strings.Contains(check.Note, "has ended") {
		t.Fatalf("the check did not record the node's epoch: %+v", check)
	}

	// No node is not a failure, it is an unchecked question - and -require-node
	// is what turns that into a refusal.
	check, err = epochCompletionCheck(nil, 6)
	if err != nil || check.Verified || !strings.Contains(check.Note, "not checked") {
		t.Fatalf("an unchecked epoch was recorded as ended: %+v (%v)", check, err)
	}

	// A node that answers something else is also not an agreement.
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"result":{"epoch":0},"error":null}`)
	}))
	defer empty.Close()
	if check, err = epochCompletionCheck(&client{url: empty.URL, http: newHTTPClient()}, 6); err != nil || check.Verified {
		t.Fatalf("a node that named no epoch was read as agreement: %+v (%v)", check, err)
	}
}

func TestAnIndexerCheckRefusesAFlipWithNoPublishedResult(t *testing.T) {
	check := &IndexerCheck{Note: "one candidate answered and did", Records: 1, Matched: 1}

	if reason := eligibilityRefusal(nil, nil); reason != "" {
		t.Fatalf("an absent cross-check refused a bundle: %q", reason)
	}
	if reason := eligibilityRefusal(check, &FlipCheck{Matched: 3, Note: "all agree"}); reason != "" {
		t.Fatalf("a clean cross-check refused a bundle: %q", reason)
	}
	if reason := eligibilityRefusal(check, &FlipCheck{Missing: []string{"flip 1 bafy"}, Note: "one is not listed"}); reason == "" {
		t.Fatal("a flip the explorer does not list was accepted")
	}
	if reason := eligibilityRefusal(
		&IndexerCheck{Mismatched: 1, Note: "one disagreed"},
		&FlipCheck{Matched: 2, Note: "two agree"},
	); reason == "" {
		t.Fatal("an answer count that disagreed was accepted")
	}
	// The one this rule closed: a covered flip the explorer lists but has no
	// result for is not evidence that it completed validation.
	reason := eligibilityRefusal(check, &FlipCheck{NoResult: []string{"flip 4 bafy"}, Note: "4 covered flips, 3 agree, 1 carries no published result"})
	if reason == "" || !strings.Contains(reason, "completed validation") {
		t.Fatalf("a flip with no published result was accepted: %q", reason)
	}
}

func TestCoveredFlipsRefuseAnAuthorTheExplorerDoesNotList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"result":[],"continuationToken":""}`)
	}))
	defer server.Close()

	_, err := crossCheckCoveredFlips(newHTTPClient(), server.URL, 7, "0xaa", []CoveredFlip{{Index: 0, Cid: "bafkqaaa"}})
	if err == nil || !strings.Contains(err.Error(), "lists no flips") {
		t.Fatalf("an author the explorer does not list was accepted, or the refusal is unclear: %v", err)
	}
}

func TestEpochRootIsCheckedAgainstANodesOwnHeader(t *testing.T) {
	root := "0x" + strings.Repeat("ab", 32)
	assignment := &EpochAssignment{
		Epoch: 7,
		Shards: []ShardAssignment{{
			Shard:                  1,
			Addresses:              []string{"aa"},
			ShortFlipsPerCandidate: [][]int{{0, 1, 2, 3, 4, 5, 6, 7}},
			LongFlipsPerCandidate:  [][]int{{0, 1, 2}},
		}},
	}
	// The epoch's state root belongs to its opening block, which the explorer
	// files as the newest block of the epoch before it (`SOURCES.md` F-82), so
	// the stub serves it the way the real explorer does: from `/Epoch/6/Blocks`.
	indexer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Epoch/7/Identities":
			io.WriteString(w, `{"result":[{"address":"aa","shortAnswersCount":8,"longAnswersCount":3,"approved":true}],"continuationToken":""}`)
		case "/Epoch/7":
			io.WriteString(w, fmt.Sprintf(`{"result":{"epoch":7,"stateRoot":"%s","validationFirstBlockHeight":41}}`, root))
		case "/Epoch/6/Blocks":
			io.WriteString(w, `{"result":[{"height":42,"timestamp":"2026-09-25T23:09:04Z"}],"continuationToken":41}`)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer indexer.Close()

	agreeing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, fmt.Sprintf(`{"result":{"height":42,"hash":"0x01","root":"%s"},"error":null}`, root))
	}))
	defer agreeing.Close()

	check, err := crossCheckAssignment(newHTTPClient(), indexer.URL, assignment, &client{url: agreeing.URL, http: newHTTPClient()}, 0)
	if err != nil {
		t.Fatalf("cross-check with a node: %v", err)
	}
	if !check.NodeVerified || check.EpochOpenHeight != 42 {
		t.Fatalf("the node's agreement was not recorded: %+v", check)
	}
	if !strings.Contains(check.EpochOpenSource, "epoch 6") {
		t.Fatalf("the check does not say where the height came from: %q", check.EpochOpenSource)
	}
	if !strings.Contains(check.NodeNote, "checked") {
		t.Fatalf("the note does not say the root was checked: %q", check.NodeNote)
	}

	disagreeing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"result":{"height":42,"hash":"0x02","root":"0x`+strings.Repeat("cd", 32)+`"},"error":null}`)
	}))
	defer disagreeing.Close()

	_, err = crossCheckAssignment(newHTTPClient(), indexer.URL, assignment, &client{url: disagreeing.URL, http: newHTTPClient()}, 0)
	if err == nil || !strings.Contains(err.Error(), "the epoch is refused") {
		t.Fatalf("a root the node's header disagrees with was accepted: %v", err)
	}

	// No node at all is a recorded note, not an agreement.
	check, err = crossCheckAssignment(newHTTPClient(), indexer.URL, assignment, nil, 0)
	if err != nil {
		t.Fatalf("cross-check without a node: %v", err)
	}
	if check.NodeVerified || !strings.Contains(check.NodeNote, "not checked") {
		t.Fatalf("an unchecked root was recorded as verified: %+v", check)
	}

	// A caller who knows the height does not have to take the explorer's word for
	// it, and the height they name is the one compared.
	check, err = crossCheckAssignment(newHTTPClient(), indexer.URL, assignment, &client{url: agreeing.URL, http: newHTTPClient()}, 42)
	if err != nil {
		t.Fatalf("cross-check with a named height: %v", err)
	}
	if !check.NodeVerified || !strings.Contains(check.EpochOpenSource, "caller") {
		t.Fatalf("the named height was not used or not recorded: %+v", check)
	}
}

func TestEpochRootIsNotCalledVerifiedWhenTheHeightIsUnknown(t *testing.T) {
	assignment := &EpochAssignment{Epoch: 7, Shards: []ShardAssignment{{Shard: 1, Addresses: []string{"aa"}, ShortFlipsPerCandidate: [][]int{{0}}, LongFlipsPerCandidate: [][]int{{0}}}}}
	indexer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Epoch/7/Identities":
			io.WriteString(w, `{"result":[{"address":"aa","shortAnswersCount":1,"longAnswersCount":1,"approved":true}],"continuationToken":""}`)
		case "/Epoch/7":
			io.WriteString(w, `{"result":{"epoch":7,"stateRoot":"0x`+strings.Repeat("ab", 32)+`"}}`)
		case "/Epoch/6/Blocks":
			io.WriteString(w, `{"result":[],"continuationToken":""}`)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer indexer.Close()
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"result":{"height":42,"root":"0x`+strings.Repeat("ab", 32)+`"},"error":null}`)
	}))
	defer node.Close()

	check, err := crossCheckAssignment(newHTTPClient(), indexer.URL, assignment, &client{url: node.URL, http: newHTTPClient()}, 0)
	if err != nil {
		t.Fatalf("cross-check: %v", err)
	}
	if check.NodeVerified {
		t.Fatalf("a root whose height the explorer did not publish was called verified: %+v", check)
	}
	if !strings.Contains(check.NodeNote, "not checked") || !strings.Contains(check.EpochOpenSource, "not resolved") {
		t.Fatalf("the check does not record why the root went unchecked: %+v", check)
	}
}
