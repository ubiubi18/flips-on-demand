package main

// The registration claim: the public input set a registration proof commits to
// and the escrow re-checks, the way `fulfill` re-checks the delivery journal.
//
// Version 2 binds the two anchored state versions of one session
// (`SOURCES.md` F-97): the height and state root whose record lists the flips,
// and the height and state root whose record carries the chain's validation
// flags. Everything a receipt cannot answer by itself is here: the registry, the
// author and epoch, those two anchors, the flags the label follows from, the
// manifest commitment that binds the content, the key commitment the bundle
// record is keyed by, and the two labels the escrow pins - the eligibility
// policy and the consensus version, each as the digest of its exact string.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	// registrationStatementVersion tells a future statement apart from this one.
	registrationStatementVersion = 3
	// registrationJournalBytes is the fixed version 3 journal:
	// 4 + 20 + 20 + 2 + 4 + 8 + 32 + 8 + 32 + 4 + six 32-byte digests: the
	// manifest, the key, the inventory commitment, the policy, the consensus
	// version and the network.
	registrationJournalBytes = 326
	// ConsensusVersionPinned is the node revision this release proves against.
	// The escrow pins the digest of this exact string, so a receipt built on
	// another revision cannot register even when its arithmetic is sound.
	ConsensusVersionPinned = "idena-go@9bc57040337fd19f45ba65323939cac8789b9efd"

	// NetworkNamePinned is the network this release's statement binds, and the
	// escrow pins the digest of the same string. A receipt proved for another
	// network cannot register, so it is refused here rather than turned into a
	// call the node would reject.
	NetworkNamePinned = "idena"

	// ConsensusEpochFloor is the first epoch this release's rules cover.
	//
	// A flip's verdict is decided by exactly two switches in the pinned node's
	// qualification path, Upgrade10 and Upgrade11 (`core/ceremony/qualification.go`:
	// lines 167, 173, 203, 398, 403, 427, 434 - the epoch-conditional flags
	// elsewhere belong to identity-state and reward logic, not to flips). Both are
	// true from consensus V11 on, whose activation window ran 2023-03-01T08:00Z to
	// 2023-03-07T00:00Z and whose config carries `KeyWordsV3Epoch = 104`. That
	// window fell inside epoch 103 (block 5,664,108 on 2023-02-28 to block
	// 5,720,711 on 2023-03-13), so V11's rules apply from epoch 104's start, which
	// is the last block of 103. Epoch 104 validated on 2023-03-25.
	//
	// The floor is enforced rather than documented: `SPEC.md` 5.2 requires
	// unsupported epochs to fail with a clear explanation, because applying the
	// V11 arithmetic to an earlier epoch would decide flips the chain decided
	// differently.
	ConsensusEpochFloor uint16 = 104
)

// requireSupportedEpoch refuses an epoch whose qualification ran under rules
// this release does not implement.
func requireSupportedEpoch(epoch uint16) error {
	if epoch < ConsensusEpochFloor {
		return fmt.Errorf(
			"epoch %d predates this release's rules: consensus V11 activated upgrade 10 and upgrade 11 at epoch %d, so flips up to epoch %d were decided by arithmetic this release does not implement and it refuses them rather than applying the wrong rules",
			epoch, ConsensusEpochFloor, ConsensusEpochFloor-1)
	}
	return nil
}

// RegistrationClaim is the journal of a registration receipt.
type RegistrationClaim struct {
	Version            uint32 `json:"version"`
	Contract           string `json:"contract"`
	Author             string `json:"author"`
	Epoch              uint16 `json:"epoch"`
	CoveredFlips       uint32 `json:"coveredFlips"`
	InventoryHeight    uint64 `json:"inventoryHeight"`
	InventoryRoot      string `json:"inventoryRoot"`
	VerdictHeight      uint64 `json:"verdictHeight"`
	VerdictRoot        string `json:"verdictRoot"`
	ValidationFlags    uint32 `json:"validationFlags"`
	ManifestCommitment string `json:"manifestCommitment"`
	KeyCommitment      string `json:"keyCommitment"`
	// InventoryCommitment is the statement's own commitment over the bundle's
	// CIDs. The pool record republishes it so a buyer can check a candidate CID
	// set against chain state.
	InventoryCommitment string `json:"inventoryCommitment"`
	PolicyDigest        string `json:"policyDigest"`
	ConsensusDigest     string `json:"consensusDigest"`
	NetworkDigest       string `json:"networkDigest"`
}

// labelDigest is how a label the escrow pins becomes a journal field: the
// digest of its exact string, so a second implementation cannot bind a label
// that only looks the same.
func labelDigest(label string) string {
	digest := sha256.Sum256([]byte(label))
	return hex.EncodeToString(digest[:])
}

// JournalBytes is the fixed version 3 layout the guest commits and the escrow
// reads: version (u32) || contract (20) || author (20) || epoch (u16) ||
// covered flips (u32) || inventory height (u64) || inventory root (32) ||
// verdict height (u64) || verdict root (32) || validation flags (u32) ||
// manifest commitment (32) || key commitment (32) || inventory commitment (32)
// || policy digest (32) || consensus digest (32) || network digest (32).
func (c *RegistrationClaim) JournalBytes() ([]byte, error) {
	if c.Version != registrationStatementVersion {
		return nil, fmt.Errorf("unsupported registration statement version %d", c.Version)
	}
	if !isHex(c.Contract, addressBytes) {
		return nil, errors.New("the claim names no valid contract address")
	}
	if !isHex(c.Author, addressBytes) {
		return nil, errors.New("the claim names no valid author address")
	}
	if c.CoveredFlips == 0 {
		return nil, errors.New("the claim covers no flips")
	}
	if c.InventoryHeight == 0 || c.VerdictHeight == 0 {
		return nil, errors.New("the claim names no session heights")
	}
	if c.VerdictHeight <= c.InventoryHeight {
		return nil, errors.New("the claim's verdict height is not after its inventory height")
	}
	if c.ValidationFlags&^0b111 != 0 {
		return nil, errors.New("the claim binds flags the chain cannot set")
	}

	out := make([]byte, 0, registrationJournalBytes)
	out = binary.BigEndian.AppendUint32(out, c.Version)
	for _, field := range []struct {
		value string
		bytes int
	}{
		{c.Contract, addressBytes},
		{c.Author, addressBytes},
	} {
		raw, err := hex.DecodeString(field.value)
		if err != nil {
			return nil, fmt.Errorf("journal field: %w", err)
		}
		out = append(out, raw...)
	}
	out = binary.BigEndian.AppendUint16(out, c.Epoch)
	out = binary.BigEndian.AppendUint32(out, c.CoveredFlips)
	digestBytes := func(value, name string) ([]byte, error) {
		raw, err := hex.DecodeString(value)
		if err != nil {
			return nil, fmt.Errorf("journal %s: %w", name, err)
		}
		if len(raw) != 32 {
			return nil, fmt.Errorf("journal %s is %d bytes, expected 32", name, len(raw))
		}
		return raw, nil
	}
	out = binary.BigEndian.AppendUint64(out, c.InventoryHeight)
	root, err := digestBytes(c.InventoryRoot, "inventory root")
	if err != nil {
		return nil, err
	}
	out = append(out, root...)
	out = binary.BigEndian.AppendUint64(out, c.VerdictHeight)
	root, err = digestBytes(c.VerdictRoot, "verdict root")
	if err != nil {
		return nil, err
	}
	out = append(out, root...)
	out = binary.BigEndian.AppendUint32(out, c.ValidationFlags)
	for _, field := range []struct {
		value string
		name  string
	}{
		{c.ManifestCommitment, "manifest commitment"},
		{c.KeyCommitment, "key commitment"},
		{c.InventoryCommitment, "inventory commitment"},
		{c.PolicyDigest, "policy digest"},
		{c.ConsensusDigest, "consensus digest"},
		{c.NetworkDigest, "network digest"},
	} {
		raw, err := digestBytes(field.value, field.name)
		if err != nil {
			return nil, err
		}
		out = append(out, raw...)
	}
	if len(out) != registrationJournalBytes {
		return nil, fmt.Errorf("the journal is %d bytes, expected %d", len(out), registrationJournalBytes)
	}
	return out, nil
}

// JournalHex is the journal as the artefact carries it.
func (c *RegistrationClaim) JournalHex() (string, error) {
	journal, err := c.JournalBytes()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(journal), nil
}
