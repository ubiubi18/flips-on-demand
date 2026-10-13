package main

// The seller's first command as the pieces it puts in order: a capture becomes a
// decision, the decision carries the witness the prover reads, and the assembly
// refuses what the prover would refuse rather than proving an input it cannot.

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/types"
)

// TestRegistrationInputCarriesTheProofAnchor checks the anchor bundle a seller
// attaches: the shape `fod state-proof -proof-bundle` writes, the round trip
// through a file, and the refusal of a root that is not a digest.
func TestRegistrationInputCarriesTheProofAnchor(t *testing.T) {
	bundle := &ProofBundle{
		Tree:    "identity",
		Height:  10860138,
		RootHex: strings.Repeat("ab", 32),
		Proofs: []ProofBundleEntry{{
			KeyHex:       "021a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d",
			ValueHex:     "0102",
			ValueHashHex: strings.Repeat("cd", 32),
			Version:      10860138,
			Path: []ProofPathItem{{
				Height:   1,
				Size:     2,
				Version:  10860138,
				RightHex: strings.Repeat("ef", 32),
			}},
		}},
	}
	if err := validateProofAnchor(bundle); err != nil {
		t.Fatalf("a well-formed anchor was refused: %v", err)
	}

	path := filepath.Join(t.TempDir(), "bundle.json")
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	loaded, err := readProofBundleFile(path)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	if err := validateProofAnchor(loaded); err != nil {
		t.Fatalf("the file round trip broke the anchor: %v", err)
	}
	if loaded.Height != bundle.Height || len(loaded.Proofs) != 1 || loaded.Tree != "identity" {
		t.Fatalf("the file round trip changed the bundle: %+v", loaded)
	}

	bad := *bundle
	bad.RootHex = "abcd"
	if err := validateProofAnchor(&bad); err == nil {
		t.Fatal("a root that is not a 32-byte digest was accepted")
	}
	bad = *bundle
	bad.Proofs = nil
	if err := validateProofAnchor(&bad); err == nil {
		t.Fatal("an anchor without proofs was accepted")
	}

	anchor := strings.Repeat("ab", 32)
	if err := proofAnchorMatchesAnchor(bundle, anchor); err != nil {
		t.Fatalf("a bundle that proves the manifest's anchor was refused: %v", err)
	}
	if err := proofAnchorMatchesAnchor(bundle, strings.Repeat("ab", 31)+"cd"); err == nil {
		t.Fatal("a bundle that proves some other root was accepted")
	}
	if err := proofAnchorMatchesAnchor(nil, anchor); err != nil {
		t.Fatalf("no bundle is allowed when the input carries none: %v", err)
	}
}

// TestReadAddressList covers both shapes a caller can hand the tool: the JSON
// array an operator may generate from a capture, and a plain one-per-line list.
func TestReadAddressList(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "addresses.json")
	if err := os.WriteFile(jsonPath, []byte(`["0x0102030405060708090a0b0c0d0e0f1011121314","0x1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d"]`), 0o600); err != nil {
		t.Fatalf("write json list: %v", err)
	}
	addresses, err := readAddressList(jsonPath)
	if err != nil {
		t.Fatalf("json list: %v", err)
	}
	if len(addresses) != 2 {
		t.Fatalf("json list gave %d addresses, want 2", len(addresses))
	}

	linesPath := filepath.Join(dir, "addresses.txt")
	if err := os.WriteFile(linesPath, []byte("# the shard's candidates\n0x0102030405060708090a0b0c0d0e0f1011121314\n\n0x1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d\n"), 0o600); err != nil {
		t.Fatalf("write line list: %v", err)
	}
	addresses, err = readAddressList(linesPath)
	if err != nil {
		t.Fatalf("line list: %v", err)
	}
	if len(addresses) != 2 || addresses[0] != "0x0102030405060708090a0b0c0d0e0f1011121314" {
		t.Fatalf("line list gave %v", addresses)
	}
}

func TestPrepareOfferAssemblesAProverInput(t *testing.T) {
	seed := make([]byte, 32)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	captured := captureWith(1, map[string]int{authorA: 1, authorB: 1, authorC: 0})
	// The capture's CIDs become the CIDs of the blobs this test writes, so the
	// assembly's blob check - the rule the prover applies - can pass on them.
	blobOf := map[string][]byte{}
	for index := range captured {
		for flip, cid := range captured[index].FlipCids {
			body := []byte(fmt.Sprintf("captured flip %d of %s", flip, captured[index].Address))
			cid = cidRawSha256(body)
			captured[index].FlipCids[flip] = cid
			blobOf[cid] = body
		}
	}
	answers := answersFor(t, seed, captured, func(string, int) (types.Answer, types.Grade) {
		return types.Left, types.GradeA
	})
	report, err := eligibilityFor(204, seed, captured, answers, authorA, EligibilityPolicyV1)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	witness, err := witnessFor(report)
	if err != nil {
		t.Fatalf("the report carries no witness: %v", err)
	}
	if len(witness) != len(captured) {
		t.Fatalf("the witness covers %d of the shard's %d candidates", len(witness), len(captured))
	}
	for index, entry := range witness {
		if len(entry.Answers) != len(entry.Assignment) || len(entry.Grades) != len(entry.Assignment) {
			t.Fatalf("witness entry %d has %d flips, %d answers, %d grades",
				index, len(entry.Assignment), len(entry.Answers), len(entry.Grades))
		}
	}

	blobs := t.TempDir()
	cids := []string{}
	for _, flip := range report.Covered {
		body := blobOf[flip.Cid]
		if err := os.WriteFile(filepath.Join(blobs, flip.Cid), body, 0o600); err != nil {
			t.Fatalf("write the blob: %v", err)
		}
		cids = append(cids, flip.Cid)
	}
	secrets := make([]byte, 32)
	for index := range secrets {
		secrets[index] = byte(index)
	}
	parts := assembleParts{
		Contract:      "0x" + strings.Repeat("11", 20),
		Network:       "lab",
		Author:        authorA,
		Epoch:         204,
		Anchor:        strings.Repeat("22", 32),
		Consensus:     ConsensusVersionPinned,
		Policy:        EligibilityPolicyV1,
		Cids:          cids,
		Report:        report,
		BlobsDir:      blobs,
		PublicSecret:  secrets,
		PrivateSecret: secrets,
	}

	input, err := assembleRegistrationInput(parts)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := validateRegistrationInput(input); err != nil {
		t.Fatalf("the assembled input does not validate: %v", err)
	}
	if len(input.Candidates) != len(witness) || len(input.Covered) != len(report.Covered) {
		t.Fatalf("the input carries %d candidates and %d covered flips", len(input.Candidates), len(input.Covered))
	}
	if input.ConsensusVersion != ConsensusVersionPinned {
		t.Fatalf("the input names consensus %q, and the escrow pins %q", input.ConsensusVersion, ConsensusVersionPinned)
	}
	if len(input.Inventory) != 1 || input.Inventory[0].Blob != hex.EncodeToString(blobOf[cids[0]]) {
		t.Fatalf("the inventory does not carry the blob it read: %+v", input.Inventory)
	}

	// A report that was not built from a capture has no committee evidence, and
	// the assembly says so instead of proving a bundle with no committee.
	if _, err := witnessFor(&EligibilityReport{}); err == nil || !strings.Contains(err.Error(), "no witness") {
		t.Fatalf("a report without a witness was accepted: %v", err)
	}

	// A CID the manifest claims but the capture does not hold is refused: the
	// proof would otherwise commit to a set nobody can check against the epoch.
	stranger := "bafystranger"
	if err := os.WriteFile(filepath.Join(blobs, stranger), []byte("not the author's"), 0o600); err != nil {
		t.Fatalf("write the blob: %v", err)
	}
	parts.Cids = []string{stranger}
	if _, err := assembleRegistrationInput(parts); err == nil ||
		!strings.Contains(err.Error(), "not one of the author's captured flips") {
		t.Fatalf("a manifest CID outside the capture was accepted: %v", err)
	}
}

// The registration input carries the epoch secrets the guest commits to, so the
// file has to be owner-only - and a file that already existed wider has to be
// tightened, because os.WriteFile keeps the mode a file already has.
func TestRegistrationInputIsOwnerOnly(t *testing.T) {
	input := &registrationInput{
		Statement:         registrationInputStatement,
		PublicPartSecret:  "aa",
		PrivatePartSecret: "bb",
	}
	path := filepath.Join(t.TempDir(), "registration-input.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write the pre-existing file: %v", err)
	}
	if err := writeRegistrationInput(path, input); err != nil {
		t.Fatalf("write the registration input: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the input: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the registration input is %v, expected 0600 - it carries the epoch secrets", info.Mode().Perm())
	}
}
