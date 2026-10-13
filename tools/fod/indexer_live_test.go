package main

// The public explorer client, checked against the real service.
//
// It is opt-in because it needs the network and a live third party:
//
//	FOD_LIVE_INDEXER=https://api.idena.io/api \
//	  GOFLAGS=-modfile=go.local.mod go test -count=1 -run IndexerLive -v ./...
//
// What it establishes is small and deliberate: that the endpoint, the envelope,
// and the pagination still look the way this client reads them. It says nothing
// about whether an explorer's answers are true, which is what the assignment
// cross-check is for.

import (
	"os"
	"strings"
	"testing"
)

func TestIndexerLiveReadsAClosedEpoch(t *testing.T) {
	base := os.Getenv("FOD_LIVE_INDEXER")
	if base == "" {
		t.Skip("set FOD_LIVE_INDEXER to run this against a live indexer")
	}
	const epoch = 227

	records, err := fetchIndexerIdentities(newHTTPClient(), base, epoch)
	if err != nil {
		t.Fatalf("read the epoch's identities: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("epoch %d lists no identities", epoch)
	}
	withAnswers := 0
	for _, record := range records {
		if record.ShortAnswersCount > 0 || record.LongAnswersCount > 0 {
			withAnswers++
		}
	}
	t.Logf("epoch %d: %d identities, %d of them answered", epoch, len(records), withAnswers)

	root, err := fetchIndexerStateRoot(newHTTPClient(), base, epoch)
	if err != nil {
		t.Fatalf("read the epoch's state root: %v", err)
	}
	if len(root) != 66 {
		t.Fatalf("the state root is %q, expected a 32-byte hex value", root)
	}
	t.Logf("epoch %d state root %s", epoch, root)
}

// Plan B's second half: the per-identity flip record a covered-flip cross-check
// reads. It is established here only that the route, its envelope, and its
// pagination still look the way the client reads them - whether an explorer's
// status and answer are true is what the reconstruction's own verdicts compare
// against, not what a live test can settle.
func TestIndexerLiveReadsAnIdentitysFlipRecord(t *testing.T) {
	base := os.Getenv("FOD_LIVE_INDEXER")
	if base == "" {
		t.Skip("set FOD_LIVE_INDEXER to run this against a live indexer")
	}
	const epoch = 227

	identities, err := fetchIndexerIdentities(newHTTPClient(), base, epoch)
	if err != nil {
		t.Fatalf("read the epoch's identities: %v", err)
	}
	author := ""
	for _, record := range identities {
		if record.ShortAnswersCount > 0 || record.LongAnswersCount > 0 {
			author = record.Address
			break
		}
	}
	if author == "" {
		t.Skipf("epoch %d lists no identity that answered", epoch)
	}

	flips, err := fetchIndexerIdentityFlips(newHTTPClient(), base, epoch, author)
	if err != nil {
		t.Fatalf("read %s's flips: %v", author, err)
	}
	if len(flips) == 0 {
		t.Fatalf("epoch %d lists no flips for %s, who answered in it", epoch, author)
	}
	withStatus := 0
	for _, flip := range flips {
		if flip.Epoch != epoch {
			t.Fatalf("the record for %s carries epoch %d", flip.CID, flip.Epoch)
		}
		if !strings.EqualFold(flip.Author, author) {
			t.Fatalf("the record for %s carries author %s, expected %s", flip.CID, flip.Author, author)
		}
		if flip.Status != "" {
			withStatus++
		}
	}
	t.Logf("epoch %d: %s has %d flips, %d of them with a published status", epoch, author, len(flips), withStatus)
}
