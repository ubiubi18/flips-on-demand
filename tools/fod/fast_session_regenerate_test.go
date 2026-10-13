package main

// Regenerate a public four-leaf session fixture for the synthetic fast-content
// receipt. The node's state encoder and IAVL tree produce the roots and paths.

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
)

func TestRegenerateFastSessionVector(t *testing.T) {
	if os.Getenv("FOD_REGENERATE_FAST_SESSION_VECTOR") == "" {
		t.Skip("set FOD_REGENERATE_FAST_SESSION_VECTOR=1 to rewrite the synthetic fast session fixture")
	}
	const proofPath = "../../proofs/fast-content/dist/synthetic-one-flip.json"
	const sessionPath = "../../proofs/fast-content/dist/synthetic-session.json"
	raw, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Format  string `json:"format"`
		Journal string `json:"journal"`
		Items   []struct {
			OriginalCID string `json:"originalCid"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &proof); err != nil || proof.Format != "fod:fast-content-sealed:v1" {
		t.Fatal("invalid synthetic content artifact")
	}
	journal, err := hex.DecodeString(proof.Journal)
	if err != nil || len(journal) != 273 || len(proof.Items) != 1 || binary.BigEndian.Uint16(journal[78:80]) != 1 {
		t.Fatal("synthetic content artifact must describe one flip")
	}
	var author common.Address
	copy(author[:], journal[56:76])
	epoch := binary.BigEndian.Uint16(journal[76:78])
	cid, err := cidBytesFromString(proof.Items[0].OriginalCID)
	if err != nil {
		t.Fatal(err)
	}
	dir := writeSessionDatadirAtEpoch(t, author, [][]byte{cid}, 0, true, epoch, epoch+1, state.FlipLotteryPeriod)
	bundle, err := readSessionBundle(dir, sessionInventoryHeight, sessionVerdictHeight, hex.EncodeToString(author[:]))
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Epoch != epoch || len(bundle.Manifest) != 1 || bundle.Manifest[0] != proof.Items[0].OriginalCID {
		t.Fatal("generated session differs from content journal")
	}
	fixture := sessionVectorFixture{
		Author:   bundle.Author,
		Manifest: bundle.Manifest,
		Sessions: []sessionVectorSession{{
			Label: "clean", Epoch: bundle.Epoch, ExpectedLabel: "clean",
			Inventory: bundle.Inventory, Verdict: bundle.Verdict,
		}},
		Source: "Synthetic node state over the original CID in proofs/fast-content/dist/synthetic-one-flip.json; regenerate with FOD_REGENERATE_FAST_SESSION_VECTOR=1.",
	}
	encoded, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
