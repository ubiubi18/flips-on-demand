package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/idena-network/idena-go/core/state"
)

func TestSavedSessionAnchorsSurviveSourcePruning(t *testing.T) {
	addr := sessionAddress(0x71)
	author := hex.EncodeToString(addr[:])
	source := writeSessionDatadir(t, addr, sessionCIDs(3), state.AtLeastOneFlipReported, true, fixtureEpoch+1)
	inventory, err := readSessionAnchor(source, sessionInventoryHeight, author)
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := readSessionAnchor(source, sessionVerdictHeight, author)
	if err != nil {
		t.Fatal(err)
	}
	kept := t.TempDir()
	first := filepath.Join(kept, "inventory.json")
	second := filepath.Join(kept, "verdict.json")
	if err := writeJSON(first, inventory); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(second, verdict); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	inventory, err = readSessionAnchorFile(first)
	if err != nil {
		t.Fatal(err)
	}
	verdict, err = readSessionAnchorFile(second)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := sessionBundleFromAnchors(author, inventory, verdict)
	if err != nil {
		t.Fatalf("portable anchors no longer made a bundle after state pruning: %v", err)
	}
	if bundle.Epoch != fixtureEpoch || len(bundle.Manifest) != 3 {
		t.Fatalf("wrong archived bundle: epoch %d, flips %d", bundle.Epoch, len(bundle.Manifest))
	}

	changed := *inventory
	changed.Identity = inventory.Identity
	changed.Identity.ValueHex = hex.EncodeToString([]byte("substituted identity"))
	if _, err := sessionBundleFromAnchors(author, &changed, verdict); err == nil {
		t.Fatal("substituted identity bytes passed the archived proof")
	}
	changed = *inventory
	changed.RootHex = hex.EncodeToString(make([]byte, 32))
	if _, err := sessionBundleFromAnchors(author, &changed, verdict); err == nil {
		t.Fatal("substituted state root passed the archived proof")
	}
	other := sessionAddress(0x72)
	if _, err := sessionBundleFromAnchors(hex.EncodeToString(other[:]), inventory, verdict); err == nil {
		t.Fatal("another author used the archived proof")
	}
	if _, err := sessionBundleFromAnchors(author, verdict, inventory); err == nil {
		t.Fatal("reversed anchors made a bundle")
	}
}
