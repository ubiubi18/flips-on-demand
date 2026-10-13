package main

// Rewrites the session fixture the version 2 guest binds, over the CID of the
// re-sealed flip blob (`proofs/registration/vectors/blob-flip-v1.json`).
//
// The fixture is a recorded artefact, not a test output, so it is regenerated
// only when its input changes and only on request:
//
//	FOD_REGENERATE_SESSION_VECTOR=1 go test -modfile=go.local.mod -run TestRegenerateSessionBundleVector .
//
// The author is carried over from the fixture being replaced, so regenerating
// changes the CID and the leaf values and nothing else.

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/common"
)

const (
	sessionVectorPath   = "../../proofs/session-core/fixtures/session-bundle-vector.json"
	flipBlobVectorPath  = "../../proofs/registration/vectors/blob-flip-v1.json"
	sessionVectorSource = "Generated over the CID of the re-sealed flip blob in proofs/registration/vectors/blob-flip-v1.json (a real legacy flip content, sealed with the epoch keys of blob-v1.json), so the version 2 guest checks a record against a blob whose plaintext is a flip. Regenerate with FOD_REGENERATE_SESSION_VECTOR=1. See SOURCES.md F-97, F-98."
)

// The fixture's shape: one session per label, each with the two anchored leaves.
type sessionVectorSession struct {
	Label         string        `json:"label"`
	Epoch         uint16        `json:"epoch"`
	ExpectedLabel string        `json:"expectedLabel"`
	Inventory     SessionAnchor `json:"inventory"`
	Verdict       SessionAnchor `json:"verdict"`
}

type sessionVectorFixture struct {
	Author   string                 `json:"author"`
	Manifest []string               `json:"manifest"`
	Sessions []sessionVectorSession `json:"sessions"`
	Source   string                 `json:"source"`
}

func TestRegenerateSessionBundleVector(t *testing.T) {
	if os.Getenv("FOD_REGENERATE_SESSION_VECTOR") == "" {
		t.Skip("set FOD_REGENERATE_SESSION_VECTOR=1 to rewrite the session fixture")
	}

	raw, err := os.ReadFile(flipBlobVectorPath)
	if err != nil {
		t.Fatalf("read the flip blob vector: %v", err)
	}
	var blobVector struct {
		BlobCid string `json:"blobCid"`
	}
	if err := json.Unmarshal(raw, &blobVector); err != nil {
		t.Fatalf("parse the flip blob vector: %v", err)
	}
	if !strings.HasPrefix(blobVector.BlobCid, "bafkrei") {
		t.Fatalf("the flip blob's CID is %q", blobVector.BlobCid)
	}
	// A submitted flip keeps the CIDv1 value as its raw bytes; the bundle names
	// the same value in the multibase text a node's API renders, and the guest
	// compares that text with the CID it recomputes from the blob.
	rawCID, err := cidBytesFromString(blobVector.BlobCid)
	if err != nil {
		t.Fatalf("the flip blob's CID is %q: %v", blobVector.BlobCid, err)
	}

	current, err := os.ReadFile(sessionVectorPath)
	if err != nil {
		t.Fatalf("read the existing fixture: %v", err)
	}
	var existing sessionVectorFixture
	if err := json.Unmarshal(current, &existing); err != nil {
		t.Fatalf("parse the existing fixture: %v", err)
	}
	var author common.Address
	authorRaw, err := hex.DecodeString(strings.TrimPrefix(existing.Author, "0x"))
	if err != nil || len(authorRaw) != len(author) {
		t.Fatalf("the existing fixture's author is %q", existing.Author)
	}
	copy(author[:], authorRaw)

	dir := writeSessionDatadir(t, author, [][]byte{rawCID}, 0, true, fixtureEpoch+1)
	bundle, err := readSessionBundle(dir, sessionInventoryHeight, sessionVerdictHeight, existing.Author)
	if err != nil {
		t.Fatalf("read the session bundle: %v", err)
	}
	// The record carries the CID bytes; the fixture names them in the multibase
	// form a manifest uses, which is what the guest compares.
	if len(bundle.Manifest) != 1 || bundle.Manifest[0] != blobVector.BlobCid {
		t.Fatalf("the regenerated bundle names %v", bundle.Manifest)
	}
	fixture := sessionVectorFixture{
		Author:   bundle.Author,
		Manifest: []string{blobVector.BlobCid},
		Source:   sessionVectorSource,
		Sessions: []sessionVectorSession{{
			Label:         "clean",
			Epoch:         bundle.Epoch,
			ExpectedLabel: "clean",
			Inventory:     bundle.Inventory,
			Verdict:       bundle.Verdict,
		}},
	}
	encoded, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("render the fixture: %v", err)
	}
	if err := os.WriteFile(sessionVectorPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}
	t.Logf("wrote %s over %s", sessionVectorPath, bundle.Manifest[0])
}
