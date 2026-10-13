package main

// The registration call the CLI prepares is what the escrow takes: these checks
// pin the argument layout, the two label digests the escrow compares as
// literals, and the refusals that keep a malformed or foreign receipt from
// becoming a transaction.

import (
	"encoding/hex"
	"strings"
	"testing"
)

// The version 2 artefact is the only receipt the escrow takes and the only one
// the CLI assembles; the version 1 artefact and its 178-byte journal stay in
// `proofs/registration` as the recorded history of a superseded statement.
const sealedRegistrationArtefact = "../../proofs/registration-v2/dist/registration-v2.json"

func loadSealedRegistration(t *testing.T) *registrationArtefact {
	t.Helper()
	artefact, err := readRegistrationArtefact(sealedRegistrationArtefact)
	if err != nil {
		// Committed: a missing artefact is a broken checkout or a release whose
		// proof was never recorded, not a test to skip (`REVIEW.md` A10).
		t.Fatalf("read the sealed registration artefact: %v", err)
	}
	return artefact
}

// mutateRegistration copies the artefact and applies one change to its journal.
func mutateRegistration(t *testing.T, artefact *registrationArtefact, change func(journal []byte)) *registrationArtefact {
	t.Helper()
	journal, err := hex.DecodeString(artefact.Journal)
	if err != nil {
		t.Fatalf("decode the journal: %v", err)
	}
	if len(journal) != registrationJournalBytes {
		t.Fatalf(
			"the committed receipt carries a %d-byte journal and this release reads %d bytes, so the sealed artefact has to be regenerated for the version 3 statement (`SPEC.md` 10.3, package F)",
			len(journal), registrationJournalBytes)
	}
	change(journal)
	copy := *artefact
	copy.Journal = hex.EncodeToString(journal)
	return &copy
}

func TestAssembleRegistrationFromTheSealedReceipt(t *testing.T) {
	artefact := loadSealedRegistration(t)
	prepared, err := AssemblePoolRegistration("idena-local", "", sealedRegistrationArtefact, artefact)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if prepared.Format != preparedPoolRegistrationFormat || prepared.Operation != operationRegisterBundle {
		t.Fatalf("unexpected artefact: %s / %s", prepared.Format, prepared.Operation)
	}
	if prepared.Submitted {
		t.Fatal("the CLI claims to have submitted something it cannot submit")
	}
	want := 2 * (registrationJournalBytes + registrationSealBytes)
	if len(prepared.Calldata) != want {
		t.Fatalf("calldata is %d hex chars, expected %d", len(prepared.Calldata), want)
	}
	if prepared.CoveredFlips == 0 {
		t.Fatal("the assembled claim covers no flips")
	}
	if !isHex(prepared.Contract, addressBytes) || !isHex(prepared.Author, addressBytes) {
		t.Fatalf("the claim names no addresses: %s / %s", prepared.Contract, prepared.Author)
	}
	if prepared.Epoch == 0 {
		t.Fatal("the assembled claim names epoch 0")
	}
	// The sealed artefact predates the host recording these two fields; when
	// they are present the assembler checks their shape, and the host writes
	// them for every new run.
	if prepared.ImageID != "" {
		t.Logf("image id    %s", prepared.ImageID)
	}
	if prepared.ClaimDigest != "" {
		t.Logf("claim       %s", prepared.ClaimDigest)
	}
	if prepared.Proof != sealedRegistrationArtefact {
		t.Fatalf("the prepared file points at %q", prepared.Proof)
	}
}

// The escrow pins these two digests as literals (`contracts/escrow/src/lib.rs`).
// If a label or the pinned revision drifts, this fails here rather than at the
// node, where a mis-bound receipt is only ever refused as `invalid_proof`.
func TestRegistrationLabelDigestsMatchTheEscrow(t *testing.T) {
	const (
		escrowPolicyDigest    = "06b31d891336e0b20fcec9983cfb0303186b04bc0da906c926d57e25a7f3399f"
		escrowConsensusDigest = "cac2b17b70e112499123c496928e2f8eb9ba1b78560518519c345ffbc1d5efdf"
	)
	if got := labelDigest(EligibilityPolicyV1); got != escrowPolicyDigest {
		t.Fatalf("the policy digest is %s, the escrow pins %s", got, escrowPolicyDigest)
	}
	if got := labelDigest(ConsensusVersionPinned); got != escrowConsensusDigest {
		t.Fatalf("the consensus digest is %s, the escrow pins %s", got, escrowConsensusDigest)
	}
}

func TestAssembleRegistrationRefusesBadReceipts(t *testing.T) {
	artefact := loadSealedRegistration(t)
	otherContract := strings.Repeat("22", addressBytes)

	cases := []struct {
		name     string
		artefact *registrationArtefact
		contract string
		wants    string
	}{
		{
			// The version 1 statement and its 178-byte journal are the recorded
			// history of a superseded proof; the escrow takes only version 2.
			name:     "another statement",
			artefact: &registrationArtefact{Statement: "fod:registration:v1", Journal: artefact.Journal, Seal: artefact.Seal},
			wants:    "not a registration proof",
		},
		{
			name:     "a short seal",
			artefact: &registrationArtefact{Statement: registrationStatement, Journal: artefact.Journal, Seal: artefact.Seal[:len(artefact.Seal)-2]},
			wants:    "artefact seal is",
		},
		{
			// A development-mode receipt carries no Groth16 seal at all
			// (`InnerReceipt::Fake`), so it has to stop here rather than reach a
			// contract that would decode its journal without one.
			name:     "a development-mode receipt, which has no Groth16 seal",
			artefact: &registrationArtefact{Statement: registrationStatement, Journal: artefact.Journal},
			wants:    "artefact seal is 0 bytes",
		},
		{
			name:     "a seal size that disagrees with the seal",
			artefact: &registrationArtefact{Statement: registrationStatement, Journal: artefact.Journal, Seal: artefact.Seal, SealBytes: registrationSealBytes - 1},
			wants:    "the artefact says its seal is",
		},
		{
			name:     "another statement version",
			artefact: mutateRegistration(t, artefact, func(journal []byte) { journal[0] = 2 }),
			wants:    "statement version",
		},
		{
			name:     "an empty bundle",
			artefact: mutateRegistration(t, artefact, func(journal []byte) { journal[46], journal[47], journal[48], journal[49] = 0, 0, 0, 0 }),
			wants:    "covers no flips",
		},
		{
			name:     "another eligibility policy",
			artefact: mutateRegistration(t, artefact, func(journal []byte) { journal[registrationPolicyOffset] ^= 0x01 }),
			wants:    "eligibility policy",
		},
		{
			name:     "another consensus revision",
			artefact: mutateRegistration(t, artefact, func(journal []byte) { journal[registrationConsensusOffset] ^= 0x01 }),
			wants:    "consensus version",
		},
		{
			name:     "a receipt for another registry",
			artefact: artefact,
			contract: otherContract,
			wants:    "not " + otherContract,
		},
		{
			// The escrow verifies the seal against the claim its own pinned guest
			// program produces, so a receipt from another program can never
			// register - and the node would only say `invalid_proof`.
			name: "a receipt from another guest program",
			artefact: func() *registrationArtefact {
				copy := *artefact
				copy.ImageID = strings.Repeat("11", 32)
				return &copy
			}(),
			wants: "this release pins",
		},
		{
			// A journal altered after the proof: the claim digest it carries can
			// no longer be the one this journal produces, which is exactly what the
			// escrow recomputes.
			name:     "a journal altered after it was proved",
			artefact: mutateRegistration(t, artefact, func(journal []byte) { journal[60] ^= 0x01 }),
			wants:    "do not belong together",
		},
		{
			name: "a receipt with no image id",
			artefact: func() *registrationArtefact {
				copy := *artefact
				copy.ImageID = ""
				return &copy
			}(),
			wants: "records no image id",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prepared, err := AssemblePoolRegistration("idena-local", c.contract, sealedRegistrationArtefact, c.artefact)
			if err == nil {
				t.Fatalf("the assembly accepted it: %+v", prepared)
			}
			if !strings.Contains(err.Error(), c.wants) {
				t.Fatalf("refused with %q, expected %q", err, c.wants)
			}
		})
	}
}
