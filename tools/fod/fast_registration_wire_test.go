package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestEncodeFastRegistrationWire(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/session-core/fixtures/session-bundle-vector.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Author   string   `json:"author"`
		Manifest []string `json:"manifest"`
		Sessions []struct {
			Epoch     uint16        `json:"epoch"`
			Inventory SessionAnchor `json:"inventory"`
			Verdict   SessionAnchor `json:"verdict"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Sessions) == 0 {
		t.Fatal("the session fixture is empty")
	}
	session := fixture.Sessions[0]
	bundle, err := sessionBundleFromAnchors(fixture.Author, &session.Inventory, &session.Verdict)
	if err != nil {
		t.Fatalf("verify the fixture bundle: %v", err)
	}
	if bundle.Epoch != session.Epoch || !slices.Equal(bundle.Manifest, fixture.Manifest) {
		t.Fatal("the verified bundle differs from the fixture's epoch or CID sequence")
	}
	wire, err := encodeFastRegistrationWire(bundle)
	if err != nil {
		t.Fatalf("encode the verified bundle: %v", err)
	}
	// The digest covers every byte in the existing harness/Rust fixture layout,
	// including both heights, roots, values, and all four IAVL paths.
	const expectedSHA256 = "c3e327720e38554a1b61ecb436ce51ad2b0f4f4e27fdd1578b9723acb38a9898"
	sum := sha256.Sum256(wire)
	if got := hex.EncodeToString(sum[:]); got != expectedSHA256 {
		t.Fatalf("fixture wire digest = %s, want %s", got, expectedSHA256)
	}
	if len(wire) != 820 {
		t.Fatalf("fixture wire is %d bytes, want 820", len(wire))
	}
	certificate := make([]byte, fastContentJournalBytes+fastContentSealBytes+2)
	binary.BigEndian.PutUint16(certificate[fastContentJournalBytes+fastContentSealBytes:], uint16(len(wire)))
	certificate = append(certificate, wire...)
	headers, err := extractFastCertificateHeaders(certificate)
	if err != nil || headers.Inventory.Height != session.Inventory.Height || headers.Verdict.Height != session.Verdict.Height ||
		headers.Inventory.StateRoot != session.Inventory.RootHex || headers.Verdict.StateRoot != session.Verdict.RootHex {
		t.Fatalf("public certificate header extraction: %v", err)
	}
	if _, err := extractFastCertificateHeaders(certificate[:len(certificate)-1]); err == nil {
		t.Fatal("truncated fast certificate was accepted")
	}

	tests := []struct {
		name   string
		change func(*SessionBundle)
	}{
		{"zero CIDs", func(b *SessionBundle) { b.Manifest = nil }},
		{"too many CIDs", func(b *SessionBundle) { b.Manifest = make([]string, fastRegistrationMaxFlips+1) }},
		{"oversized CID", func(b *SessionBundle) { b.Manifest = []string{strings.Repeat("a", fastRegistrationMaxCIDBytes+1)} }},
		{"oversized value", func(b *SessionBundle) {
			b.Inventory.Global.ValueHex = strings.Repeat("00", fastRegistrationMaxValueBytes+1)
		}},
		{"too many path items", func(b *SessionBundle) {
			b.Inventory.Global.Path = make([]ProofPathItem, fastRegistrationMaxPathItems+1)
		}},
		{"oversized total wire", func(b *SessionBundle) {
			value := strings.Repeat("00", fastRegistrationMaxValueBytes)
			b.Inventory.Global.ValueHex = value
			b.Inventory.Identity.ValueHex = value
			b.Verdict.Global.ValueHex = value
			b.Verdict.Identity.ValueHex = value
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := *bundle
			test.change(&changed)
			if _, err := encodeFastRegistrationWire(&changed); err == nil {
				t.Fatal("malformed or overlimit bundle was encoded")
			}
		})
	}
}
