package main

// The version 2 registration journal is produced in Rust
// (`proofs/registration-v2`) and read by the escrow and by this CLI. No Go code
// builds it any more, so the layout is pinned against the one frozen artefact
// the prover wrote: decode it at the CLI's own offsets, re-encode it with the
// claim model, and require the exact bytes back. A drift on either side fails
// here rather than on a chain, where a mis-read field is only a refusal.

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func readFrozenRegistrationArtefact(t *testing.T) *registrationArtefact {
	t.Helper()
	raw, err := os.ReadFile(sealedRegistrationArtefact)
	if err != nil {
		t.Fatalf("read the committed version 2 artefact: %v", err)
	}
	var artefact registrationArtefact
	if err := json.Unmarshal(raw, &artefact); err != nil {
		t.Fatalf("parse the frozen artefact: %v", err)
	}
	if artefact.Statement != registrationStatement {
		t.Fatalf("the artefact is statement %q, expected %q", artefact.Statement, registrationStatement)
	}
	return &artefact
}

func frozenRegistrationJournal(t *testing.T) []byte {
	t.Helper()
	artefact := readFrozenRegistrationArtefact(t)
	journal, err := hex.DecodeString(artefact.Journal)
	if err != nil {
		t.Fatalf("decode the frozen journal: %v", err)
	}
	if len(journal) != registrationJournalBytes {
		t.Fatalf("the frozen journal is %d bytes, expected %d", len(journal), registrationJournalBytes)
	}
	return journal
}

// claimOfJournal reads a journal at the offsets the CLI assembles calls with,
// so the decode and the encode cannot quietly disagree about a field.
func claimOfJournal(t *testing.T, journal []byte) *RegistrationClaim {
	t.Helper()
	digest := func(offset int) string {
		return hex.EncodeToString(journal[offset : offset+32])
	}
	return &RegistrationClaim{
		Version:             binary.BigEndian.Uint32(journal[0:4]),
		Contract:            hex.EncodeToString(journal[registrationContractOffset : registrationContractOffset+addressBytes]),
		Author:              hex.EncodeToString(journal[registrationAuthorOffset : registrationAuthorOffset+addressBytes]),
		Epoch:               binary.BigEndian.Uint16(journal[registrationEpochOffset : registrationEpochOffset+2]),
		CoveredFlips:        binary.BigEndian.Uint32(journal[registrationCoveredOffset : registrationCoveredOffset+4]),
		InventoryHeight:     binary.BigEndian.Uint64(journal[registrationInventoryHeightOffset : registrationInventoryHeightOffset+8]),
		InventoryRoot:       digest(registrationInventoryRootOffset),
		VerdictHeight:       binary.BigEndian.Uint64(journal[registrationVerdictHeightOffset : registrationVerdictHeightOffset+8]),
		VerdictRoot:         digest(registrationVerdictRootOffset),
		ValidationFlags:     binary.BigEndian.Uint32(journal[registrationFlagsOffset : registrationFlagsOffset+4]),
		ManifestCommitment:  digest(registrationManifestOffset),
		KeyCommitment:       digest(registrationCommitmentOffset),
		InventoryCommitment: digest(registrationInventoryOffset),
		PolicyDigest:        digest(registrationPolicyOffset),
		ConsensusDigest:     digest(registrationConsensusOffset),
		NetworkDigest:       digest(registrationNetworkOffset),
	}
}

// TestRegistrationJournalIsFrozen pins the one artefact every consumer reads:
// the CLI's offsets have to land on the fields the Rust statement encodes, and
// the two labels have to be the ones the escrow pins as literals.
func TestRegistrationJournalIsFrozen(t *testing.T) {
	artefact := readFrozenRegistrationArtefact(t)
	journal := frozenRegistrationJournal(t)

	// The claim digest ties the journal to the seal, so it also pins the guest
	// program the CLI records against the one the escrow pins.
	digest, err := claimDigestFromHex(registrationImageIDHex, journal)
	if err != nil {
		t.Fatalf("compute the claim digest: %v", err)
	}
	if got := hex.EncodeToString(digest[:]); got != artefact.ClaimDigest {
		t.Fatalf("the journal's claim digest is %s, the artefact records %s", got, artefact.ClaimDigest)
	}

	claim := claimOfJournal(t, journal)
	if claim.Version != registrationStatementVersion {
		t.Fatalf("the journal commits statement version %d", claim.Version)
	}
	if claim.CoveredFlips == 0 {
		t.Fatal("the frozen journal covers no flips")
	}
	if claim.ValidationFlags&^0b111 != 0 {
		t.Fatalf("the frozen journal binds flags %03b, which the record cannot carry", claim.ValidationFlags)
	}
	if claim.VerdictHeight <= claim.InventoryHeight {
		t.Fatalf("the verdict height %d is not after the inventory height %d", claim.VerdictHeight, claim.InventoryHeight)
	}
	if claim.PolicyDigest != labelDigest(EligibilityPolicyV1) {
		t.Fatalf("the journal binds policy %s, this release pins %s", claim.PolicyDigest, labelDigest(EligibilityPolicyV1))
	}
	if claim.ConsensusDigest != labelDigest(ConsensusVersionPinned) {
		t.Fatalf("the journal binds consensus %s, this release pins %s", claim.ConsensusDigest, labelDigest(ConsensusVersionPinned))
	}
	if claim.NetworkDigest != labelDigest(NetworkNamePinned) {
		t.Fatalf("the journal binds network %s, this release pins %s", claim.NetworkDigest, labelDigest(NetworkNamePinned))
	}

	// The round trip is the actual freeze: the Go model has to reproduce the
	// bytes the Rust guest committed, field for field and in its order.
	encoded, err := claim.JournalBytes()
	if err != nil {
		t.Fatalf("re-encode the frozen journal: %v", err)
	}
	if !equalBytes(encoded, journal) {
		t.Fatalf("the re-encoded journal differs from the frozen bytes:\n got %x\nwant %x", encoded, journal)
	}
}

// TestRegistrationClaimBindsEveryField requires each field to move the bytes: a
// model that dropped one would still round-trip a frozen vector it happens to
// agree with.
func TestRegistrationClaimBindsEveryField(t *testing.T) {
	journal := frozenRegistrationJournal(t)
	frozen := claimOfJournal(t, journal)

	cases := []struct {
		name   string
		change func(claim *RegistrationClaim)
	}{
		{"the covered flip count", func(claim *RegistrationClaim) { claim.CoveredFlips++ }},
		{"the inventory height", func(claim *RegistrationClaim) { claim.InventoryHeight-- }},
		{"the inventory root", func(claim *RegistrationClaim) { claim.InventoryRoot = flipFirstNibble(claim.InventoryRoot) }},
		{"the verdict height", func(claim *RegistrationClaim) { claim.VerdictHeight++ }},
		{"the verdict root", func(claim *RegistrationClaim) { claim.VerdictRoot = flipFirstNibble(claim.VerdictRoot) }},
		{"the validation flags", func(claim *RegistrationClaim) { claim.ValidationFlags ^= 0b001 }},
		{"the manifest commitment", func(claim *RegistrationClaim) { claim.ManifestCommitment = flipFirstNibble(claim.ManifestCommitment) }},
		{"the key commitment", func(claim *RegistrationClaim) { claim.KeyCommitment = flipFirstNibble(claim.KeyCommitment) }},
		{"the inventory commitment", func(claim *RegistrationClaim) { claim.InventoryCommitment = flipFirstNibble(claim.InventoryCommitment) }},
		{"the policy digest", func(claim *RegistrationClaim) { claim.PolicyDigest = flipFirstNibble(claim.PolicyDigest) }},
		{"the consensus digest", func(claim *RegistrationClaim) { claim.ConsensusDigest = flipFirstNibble(claim.ConsensusDigest) }},
		{"the network digest", func(claim *RegistrationClaim) { claim.NetworkDigest = flipFirstNibble(claim.NetworkDigest) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			claim := *frozen
			testCase.change(&claim)
			encoded, err := claim.JournalBytes()
			if err != nil {
				t.Fatalf("encode the changed claim: %v", err)
			}
			if equalBytes(encoded, journal) {
				t.Fatalf("changing %s leaves the journal unchanged", testCase.name)
			}
		})
	}
}

// flipFirstNibble changes a digest without changing its length.
func flipFirstNibble(value string) string {
	first := byte('0')
	if value[0] == '0' {
		first = '1'
	}
	return string(first) + value[1:]
}
