package main

// `BundleManifest` from `SPEC.md` 4.5: the complete inventory of one author's
// flips in one epoch, plus the anchors that say what it was checked against.
// `SPEC.md` 5.5 item 1 is what a registration proof establishes with it: that the
// manifest really holds the complete inventory.
//
// The encoding below is frozen and written down in `SOURCES.md` F-67, because the
// proof program and this command have to commit to the same bytes. The CIDs are
// bound through the inventory commitment the content export already produces, so
// the two stay the same construction rather than two lookalikes.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	manifestFormat  = "fod.bundle-manifest"
	manifestVersion = 1

	// claimDomain separates the manifest's commitment from every other use of
	// these bytes in this project.
	claimDomain = "fod:manifest:claim:v1"

	// cidV1MultibaseLength is the multibase form of a CIDv1/sha2-256 value:
	// "b" plus 58 base32 characters. Both forms a flip can carry - raw while it
	// fits in a chunk, dag-pb over raw leaves once it does not - are this long
	// (`tools/fod/cid.go`, `SOURCES.md` F-86, F-88).
	cidV1MultibaseLength = 59
)

// BundleManifest is one author-epoch inventory with its anchors. The fields are
// the ones `SPEC.md` 4.5 lists, in the same order.
type BundleManifest struct {
	Format            string   `json:"format"`
	Version           int      `json:"version"`
	Network           string   `json:"network"`
	Author            string   `json:"author"`
	Epoch             uint16   `json:"epoch"`
	CIDs              []string `json:"cids"`
	ValidationAnchor  string   `json:"validationAnchor"`
	ConsensusVersion  string   `json:"consensusVersion"`
	EligibilityPolicy string   `json:"eligibilityPolicy"`
}

// NewBundleManifest validates the manifest's inputs and sorts its CIDs. The
// network, the consensus version, and the eligibility policy are labels the
// manifest carries; what they must name is settled where they are produced, not
// here.
func NewBundleManifest(network, author string, epoch uint16, cids []string, validationAnchor, consensusVersion, eligibilityPolicy string) (*BundleManifest, error) {
	if strings.TrimSpace(network) == "" || len(network) > 255 {
		return nil, errors.New("the manifest names no network")
	}
	if !isHex(author, addressBytes) {
		return nil, errors.New("the manifest names no valid author address")
	}
	if !isHex(validationAnchor, 32) {
		return nil, errors.New("the validation anchor is not a 32-byte hex digest")
	}
	if strings.TrimSpace(consensusVersion) == "" || len(consensusVersion) > 255 {
		return nil, errors.New("the manifest names no consensus version")
	}
	if strings.TrimSpace(eligibilityPolicy) == "" || len(eligibilityPolicy) > 255 {
		return nil, errors.New("the manifest names no eligibility policy")
	}
	if len(cids) == 0 {
		return nil, errors.New("the manifest covers no CIDs")
	}

	sorted := append([]string(nil), cids...)
	sort.Strings(sorted)
	for index, cid := range sorted {
		if !isFlipCID(cid) {
			return nil, fmt.Errorf("cid %d is not a CIDv1 sha2-256 value in the multibase base32 form a flip carries: %q", index, cid)
		}
		if index > 0 && sorted[index-1] == cid {
			return nil, fmt.Errorf("cid %s appears twice", cid)
		}
	}

	return &BundleManifest{
		Format:            manifestFormat,
		Version:           manifestVersion,
		Network:           network,
		Author:            strings.ToLower(strings.TrimPrefix(author, "0x")),
		Epoch:             epoch,
		CIDs:              sorted,
		ValidationAnchor:  strings.ToLower(strings.TrimPrefix(validationAnchor, "0x")),
		ConsensusVersion:  consensusVersion,
		EligibilityPolicy: eligibilityPolicy,
	}, nil
}

// isFlipCID accepts the multibase base32 form of a CIDv1/sha2-256 value and
// nothing else, so a manifest cannot quietly commit to a CID this project would
// not be able to check. Both forms the node produces are this shape; which one a
// blob has is decided by the blob's size when it is checked (`tools/fod/unixfs.go`).
func isFlipCID(cid string) bool {
	if len(cid) != cidV1MultibaseLength || !strings.HasPrefix(cid, "b") {
		return false
	}
	for _, char := range cid[1:] {
		if (char < 'a' || char > 'z') && (char < '2' || char > '7') {
			return false
		}
	}
	return true
}

// InventoryCommitment is the commitment the content export already produces: the
// author, the epoch, and the sorted CIDs, under their own domain.
func (m *BundleManifest) InventoryCommitment() string {
	return inventoryCommitment(m.Author, m.Epoch, m.CIDs)
}

// CanonicalBytes is the frozen encoding: the claim domain, the inventory
// commitment, the validation anchor, and the three labelled fields, each text
// field length-prefixed.
func (m *BundleManifest) CanonicalBytes() ([]byte, error) {
	inventory, err := hex.DecodeString(m.InventoryCommitment())
	if err != nil {
		return nil, fmt.Errorf("inventory commitment: %w", err)
	}
	anchor, err := hex.DecodeString(m.ValidationAnchor)
	if err != nil {
		return nil, fmt.Errorf("validation anchor: %w", err)
	}

	out := make([]byte, 0, len(claimDomain)+32+32+2*(len(m.ConsensusVersion)+len(m.EligibilityPolicy)+len(m.Network)))
	out = append(out, claimDomain...)
	out = append(out, inventory...)
	out = append(out, anchor...)
	for _, field := range []string{m.ConsensusVersion, m.EligibilityPolicy, m.Network} {
		if len(field) > 255 {
			return nil, fmt.Errorf("manifest field %q is longer than 255 bytes", field)
		}
		out = binary.BigEndian.AppendUint16(out, uint16(len(field)))
		out = append(out, field...)
	}
	return out, nil
}

// Commitment is what a registration proof binds and an offer carries.
func (m *BundleManifest) Commitment() (string, error) {
	canonical, err := m.CanonicalBytes()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}
