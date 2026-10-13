package main

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

type manifestVector struct {
	Note                string         `json:"note"`
	Manifest            BundleManifest `json:"manifest"`
	InventoryCommitment string         `json:"inventoryCommitment"`
	CanonicalHex        string         `json:"canonicalHex"`
	Commitment          string         `json:"commitment"`
}

func readManifestVector(t *testing.T) manifestVector {
	t.Helper()
	raw, err := os.ReadFile("../../proofs/registration/vectors/manifest-v1.json")
	if err != nil {
		t.Fatalf("read the frozen vector: %v", err)
	}
	var vector manifestVector
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatalf("parse the frozen vector: %v", err)
	}
	return vector
}

// TestManifestVectorIsFrozen is the cross-language check `SPEC.md` 4.5 asks for:
// the layout is recorded in `SOURCES.md` F-67, and any implementation - this one
// or the proof program's guest - has to reproduce these bytes from the manifest.
func TestManifestVectorIsFrozen(t *testing.T) {
	vector := readManifestVector(t)
	manifest := vector.Manifest
	if manifest.Format != manifestFormat || manifest.Version != manifestVersion {
		t.Fatalf("the vector is not a %s v%d manifest", manifestFormat, manifestVersion)
	}
	if !sort.StringsAreSorted(manifest.CIDs) {
		t.Fatal("the vector's CIDs are not sorted, so it does not pin the order")
	}

	built, err := NewBundleManifest(
		manifest.Network, manifest.Author, manifest.Epoch, manifest.CIDs,
		manifest.ValidationAnchor, manifest.ConsensusVersion, manifest.EligibilityPolicy)
	if err != nil {
		t.Fatalf("the vector's manifest is refused by its own implementation: %v", err)
	}
	if built.InventoryCommitment() != vector.InventoryCommitment {
		t.Fatalf("inventory commitment is %s, the vector says %s", built.InventoryCommitment(), vector.InventoryCommitment)
	}
	canonical, err := built.CanonicalBytes()
	if err != nil {
		t.Fatalf("canonical bytes: %v", err)
	}
	if hex.EncodeToString(canonical) != vector.CanonicalHex {
		t.Fatalf("canonical bytes are %s, the vector says %s", hex.EncodeToString(canonical), vector.CanonicalHex)
	}
	commitment, err := built.Commitment()
	if err != nil {
		t.Fatalf("commitment: %v", err)
	}
	if commitment != vector.Commitment {
		t.Fatalf("commitment is %s, the vector says %s", commitment, vector.Commitment)
	}
}

// TestManifestBindsEveryField checks that no field can be swapped without
// changing the commitment: the commitment is the only thing an offer carries, so
// an unbound field would be a field an offer could not protect.
func TestManifestBindsEveryField(t *testing.T) {
	vector := readManifestVector(t)
	base := vector.Manifest
	baseline, err := NewBundleManifest(
		base.Network, base.Author, base.Epoch, base.CIDs,
		base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	baseCommitment, err := baseline.Commitment()
	if err != nil {
		t.Fatalf("commitment: %v", err)
	}

	changedCIDs := append([]string(nil), base.CIDs...)
	changedCIDs[0] = cidRawSha256([]byte("a different flip"))
	sort.Strings(changedCIDs)

	cases := []struct {
		name  string
		build func() (*BundleManifest, error)
	}{
		{"network", func() (*BundleManifest, error) {
			return NewBundleManifest("idena-mainnet", base.Author, base.Epoch, base.CIDs, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)
		}},
		{"author", func() (*BundleManifest, error) {
			return NewBundleManifest(base.Network, "4444444444444444444444444444444444444444", base.Epoch, base.CIDs, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)
		}},
		{"epoch", func() (*BundleManifest, error) {
			return NewBundleManifest(base.Network, base.Author, base.Epoch+1, base.CIDs, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)
		}},
		{"cids", func() (*BundleManifest, error) {
			return NewBundleManifest(base.Network, base.Author, base.Epoch, changedCIDs, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)
		}},
		{"validation anchor", func() (*BundleManifest, error) {
			return NewBundleManifest(base.Network, base.Author, base.Epoch, base.CIDs, strings.Repeat("ab", 32), base.ConsensusVersion, base.EligibilityPolicy)
		}},
		{"consensus version", func() (*BundleManifest, error) {
			return NewBundleManifest(base.Network, base.Author, base.Epoch, base.CIDs, base.ValidationAnchor, "idena-go@0000000000000000000000000000000000000000", base.EligibilityPolicy)
		}},
		{"eligibility policy", func() (*BundleManifest, error) {
			return NewBundleManifest(base.Network, base.Author, base.Epoch, base.CIDs, base.ValidationAnchor, base.ConsensusVersion, "fod:eligibility:v2")
		}},
	}
	for _, c := range cases {
		changed, err := c.build()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		commitment, err := changed.Commitment()
		if err != nil {
			t.Fatalf("%s: commitment: %v", c.name, err)
		}
		if commitment == baseCommitment {
			t.Errorf("changing the %s did not change the commitment", c.name)
		}
	}
}

// TestManifestRefusals keeps a manifest from committing to something this
// project could not check: a malformed address or anchor, an empty inventory, a
// repeated CID, a CID that is not the form the node uses, and over-long labels.
func TestManifestRefusals(t *testing.T) {
	vector := readManifestVector(t)
	base := vector.Manifest
	build := func(network, author string, epoch uint16, cids []string, anchor, consensus, policy string) error {
		_, err := NewBundleManifest(network, author, epoch, cids, anchor, consensus, policy)
		return err
	}

	cases := []struct {
		name string
		err  error
	}{
		{"no network", build("", base.Author, base.Epoch, base.CIDs, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)},
		{"short author", build(base.Network, "3333", base.Epoch, base.CIDs, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)},
		{"short anchor", build(base.Network, base.Author, base.Epoch, base.CIDs, "ab", base.ConsensusVersion, base.EligibilityPolicy)},
		{"no cids", build(base.Network, base.Author, base.Epoch, nil, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)},
		{"repeated cid", build(base.Network, base.Author, base.Epoch, []string{base.CIDs[0], base.CIDs[0]}, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)},
		{"not a cid", build(base.Network, base.Author, base.Epoch, []string{"9f2a1c"}, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)},
		{"cidv0", build(base.Network, base.Author, base.Epoch, []string{"QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"}, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)},
		{"no consensus version", build(base.Network, base.Author, base.Epoch, base.CIDs, base.ValidationAnchor, " ", base.EligibilityPolicy)},
		{"no policy", build(base.Network, base.Author, base.Epoch, base.CIDs, base.ValidationAnchor, base.ConsensusVersion, "")},
		{"long network", build(strings.Repeat("n", 256), base.Author, base.Epoch, base.CIDs, base.ValidationAnchor, base.ConsensusVersion, base.EligibilityPolicy)},
	}
	for _, c := range cases {
		if c.err == nil {
			t.Errorf("%s: the manifest was accepted", c.name)
		}
	}
}

// TestManifestSortsItsCIDs pins the canonical order: a manifest built from CIDs
// in any order carries them sorted, so two sellers of the same inventory produce
// the same commitment. It builds its own bundle so it does not depend on how
// many CIDs the frozen vector happens to carry.
func TestManifestSortsItsCIDs(t *testing.T) {
	vector := readManifestVector(t)
	manifest := vector.Manifest
	first := cidRawSha256([]byte("sorting vector, flip one"))
	second := cidRawSha256([]byte("sorting vector, flip two"))
	ordered := []string{first, second}
	if ordered[0] > ordered[1] {
		ordered[0], ordered[1] = ordered[1], ordered[0]
	}
	reversed := []string{ordered[1], ordered[0]}

	built, err := NewBundleManifest(manifest.Network, manifest.Author, manifest.Epoch, ordered, manifest.ValidationAnchor, manifest.ConsensusVersion, manifest.EligibilityPolicy)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	rebuilt, err := NewBundleManifest(manifest.Network, manifest.Author, manifest.Epoch, reversed, manifest.ValidationAnchor, manifest.ConsensusVersion, manifest.EligibilityPolicy)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	firstCommitment, _ := built.Commitment()
	secondCommitment, _ := rebuilt.Commitment()
	if firstCommitment != secondCommitment {
		t.Fatal("the CID order changed the commitment")
	}
	if rebuilt.CIDs[0] != ordered[0] || rebuilt.CIDs[1] != ordered[1] {
		t.Fatalf("the CIDs came back as %v, expected them sorted", rebuilt.CIDs)
	}
}
