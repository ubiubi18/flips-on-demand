package main

// The one Plan B claim that needs a real node on the other side: that an
// explorer's epoch state root is the root of the block the node itself names as
// the epoch's start, so anyone who runs a node can check it without trusting the
// explorer's arithmetic (`SOURCES.md` F-82).

// It is opt-in because it needs both a live explorer and a live node:

//	FOD_LIVE_INDEXER=https://api.idena.io/api \
//	FOD_LIVE_NODE=http://127.0.0.1:9129 FOD_LIVE_NODE_KEY=<api key> \
//	  GOFLAGS=-modfile=go.local.mod go test -count=1 -run LiveNode -v ./...

// The epoch and the height are not hardcoded: the node is asked which epoch it
// is in and which block that epoch started at, so the test keeps working as the
// chain moves.

import (
	"os"
	"testing"
)

func TestLiveNodeConfirmsTheExposersEpochRoot(t *testing.T) {
	base := os.Getenv("FOD_LIVE_INDEXER")
	nodeURL := os.Getenv("FOD_LIVE_NODE")
	if base == "" || nodeURL == "" {
		t.Skip("set FOD_LIVE_INDEXER and FOD_LIVE_NODE to run this against a live explorer and node")
	}
	node := &client{url: nodeURL, key: os.Getenv("FOD_LIVE_NODE_KEY"), http: newHTTPClient()}

	var epoch struct {
		Epoch      uint16 `json:"epoch"`
		StartBlock uint64 `json:"startBlock"`
	}
	if err := node.call("dna_epoch", []any{}, &epoch); err != nil {
		t.Fatalf("read the node's epoch: %v", err)
	}
	if epoch.Epoch == 0 || epoch.StartBlock == 0 {
		t.Fatalf("the node named epoch %d at block %d", epoch.Epoch, epoch.StartBlock)
	}

	root, err := fetchIndexerStateRoot(newHTTPClient(), base, epoch.Epoch)
	if err != nil {
		t.Fatalf("read the explorer's state root: %v", err)
	}
	verified, note, err := nodeRootCheck(node, root, epoch.StartBlock)
	if err != nil {
		t.Fatalf("the node and the explorer disagree about epoch %d: %v", epoch.Epoch, err)
	}
	if !verified {
		t.Fatalf("the node's own start block did not confirm epoch %d: %s", epoch.Epoch, note)
	}
	t.Logf("epoch %d: the explorer's state root %s is the root of block %d, which is the block the node names as its start (header root %s)",
		epoch.Epoch, root, epoch.StartBlock, note)
}
