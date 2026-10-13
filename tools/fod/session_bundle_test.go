package main

// Builds a synthetic state datadir the way the node lays it out - a LevelDB
// named idenachain, the state tree under the stored prefix, one version per
// height - and reads the two-anchor session bundle back out of it. The records
// are encoded with the node's own state.Identity/state.Global and the tree is
// the pinned fork's, so the CLI's real reader path runs without a live node.

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosmos/iavl"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
	dbm "github.com/tendermint/tm-db"
)

const (
	sessionInventoryHeight = 10_870_000
	sessionVerdictHeight   = 10_880_001
	fixtureEpoch           = 228
)

func sessionAddress(seed byte) common.Address {
	var addr common.Address
	for i := range addr {
		addr[i] = seed + byte(i)
	}
	return addr
}

func sessionCIDs(count int) [][]byte {
	cids := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		// The node stores a CIDv1/raw/sha2-256 value: version, raw codec,
		// sha2-256, digest length, digest.
		cid := []byte{0x01, 0x55, 0x12, 0x20}
		for j := 0; j < 32; j++ {
			cid = append(cid, byte(0x90+i*0x10)+byte(j))
		}
		cids = append(cids, cid)
	}
	return cids
}

// writeSessionDatadir writes the flips at the inventory height and, at the
// verdict height, the validation status flags with the flips cleared - unless
// clearFlips is false, which models reading a height that is not the epoch's
// ValidationFinished block. The inventory version reports the flip lottery
// period, which is where a submission stops being valid.
func writeSessionDatadir(t *testing.T, addr common.Address, cids [][]byte, flags state.ValidationStatusFlag, clearFlips bool, verdictEpoch uint16) string {
	t.Helper()
	return writeSessionDatadirWithPeriod(t, addr, cids, flags, clearFlips, verdictEpoch, state.FlipLotteryPeriod)
}

// writeSessionDatadirWithPeriod is the same fixture with the inventory
// version's validation period under the caller's control, so a record taken
// while submissions were still open can be built.
func writeSessionDatadirWithPeriod(t *testing.T, addr common.Address, cids [][]byte, flags state.ValidationStatusFlag, clearFlips bool, verdictEpoch uint16, inventoryPeriod state.ValidationPeriod) string {
	return writeSessionDatadirAtEpoch(t, addr, cids, flags, clearFlips, fixtureEpoch, verdictEpoch, inventoryPeriod)
}

func writeSessionDatadirAtEpoch(t *testing.T, addr common.Address, cids [][]byte, flags state.ValidationStatusFlag, clearFlips bool, inventoryEpoch, verdictEpoch uint16, inventoryPeriod state.ValidationPeriod) string {
	t.Helper()
	dir := t.TempDir()
	db, err := dbm.NewGoLevelDB("idenachain", dir)
	if err != nil {
		t.Fatalf("create the datadir: %v", err)
	}
	prefix := state.StateDbKeys.BuildDbPrefix(sessionInventoryHeight)
	batch := db.NewBatch()
	state.StateDbKeys.SaveDbPrefix(batch, prefix)
	if err := batch.Write(); err != nil {
		t.Fatalf("write the state prefix: %v", err)
	}
	tree, err := iavl.NewMutableTree(dbm.NewPrefixDB(db, prefix), 1024)
	if err != nil {
		t.Fatalf("open the state tree: %v", err)
	}

	encodeIdentity := func(flips []state.IdentityFlip, status state.ValidationStatusFlag) []byte {
		identity := &state.Identity{
			Stake:                big.NewInt(1000),
			Birthday:             200,
			State:                state.Verified,
			RequiredFlips:        3,
			PubKey:               make([]byte, 65),
			Flips:                flips,
			LastValidationStatus: status,
		}
		value, err := identity.ToBytes()
		if err != nil {
			t.Fatalf("encode the identity: %v", err)
		}
		return value
	}
	encodeGlobal := func(epoch uint16, period state.ValidationPeriod) []byte {
		global := &state.Global{Epoch: epoch, ValidationPeriod: period, ShardsNum: 1}
		value, err := global.ToBytes()
		if err != nil {
			t.Fatalf("encode the global record: %v", err)
		}
		return value
	}

	flips := make([]state.IdentityFlip, 0, len(cids))
	for i, cid := range cids {
		flips = append(flips, state.IdentityFlip{Cid: cid, Pair: uint8(i % 2)})
	}
	tree.Set(state.StateDbKeys.IdentityKey(addr), encodeIdentity(flips, 0))
	tree.Set(state.StateDbKeys.GlobalKey(), encodeGlobal(inventoryEpoch, inventoryPeriod))
	if _, _, err := tree.SaveVersionAt(sessionInventoryHeight); err != nil {
		t.Fatalf("commit the inventory version: %v", err)
	}

	verdictFlips := flips
	if clearFlips {
		verdictFlips = nil
	}
	tree.Set(state.StateDbKeys.IdentityKey(addr), encodeIdentity(verdictFlips, flags))
	tree.Set(state.StateDbKeys.GlobalKey(), encodeGlobal(verdictEpoch, state.FlipLotteryPeriod))
	if _, _, err := tree.SaveVersionAt(sessionVerdictHeight); err != nil {
		t.Fatalf("commit the verdict version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the datadir: %v", err)
	}
	return dir
}

func TestSessionBundleReadsBothAnchors(t *testing.T) {
	addr := sessionAddress(0x31)
	cids := sessionCIDs(3)
	dir := writeSessionDatadir(t, addr, cids, state.AtLeastOneFlipReported, true, fixtureEpoch+1)

	bundle, err := readSessionBundle(dir, sessionInventoryHeight, sessionVerdictHeight, hex.EncodeToString(addr[:]))
	if err != nil {
		t.Fatalf("read the session bundle: %v", err)
	}
	if bundle.Format != sessionBundleFormat || bundle.Version != sessionBundleVersion {
		t.Fatalf("the bundle is %s/%d", bundle.Format, bundle.Version)
	}
	if bundle.Author != hex.EncodeToString(addr[:]) || bundle.Epoch != fixtureEpoch {
		t.Fatalf("the bundle names author %s at epoch %d", bundle.Author, bundle.Epoch)
	}
	if len(bundle.Manifest) != len(cids) {
		t.Fatalf("the manifest carries %d of %d flips", len(bundle.Manifest), len(cids))
	}
	for index, cid := range cids {
		if bundle.Manifest[index] != cidString(cid) {
			t.Fatalf("manifest entry %d is %s", index, bundle.Manifest[index])
		}
	}
	if bundle.Inventory.Height != sessionInventoryHeight || bundle.Verdict.Height != sessionVerdictHeight {
		t.Fatalf("the anchors are %d and %d", bundle.Inventory.Height, bundle.Verdict.Height)
	}
	if len(bundle.Inventory.Identity.Path) == 0 || len(bundle.Verdict.Global.Path) == 0 {
		t.Fatal("an anchor carries no path")
	}

	// The verdict record is the post-validation one: the flips are gone and the
	// chain's flags stand.
	value, err := hex.DecodeString(bundle.Verdict.Identity.ValueHex)
	if err != nil {
		t.Fatalf("decode the verdict value: %v", err)
	}
	var record state.Identity
	if err := record.FromBytes(value); err != nil {
		t.Fatalf("read the verdict record: %v", err)
	}
	if len(record.Flips) != 0 {
		t.Fatal("the verdict record still lists flips")
	}
	if record.LastValidationStatus != state.AtLeastOneFlipReported {
		t.Fatalf("the verdict flags are %d", record.LastValidationStatus)
	}
}

func TestSessionBundleRefusesTheWrongHeightsOrAuthor(t *testing.T) {
	addr := sessionAddress(0x41)
	dir := writeSessionDatadir(t, addr, sessionCIDs(2), 0, true, fixtureEpoch+1)
	author := hex.EncodeToString(addr[:])

	if _, err := readSessionBundle(dir, sessionVerdictHeight, sessionInventoryHeight, author); err == nil {
		t.Fatal("an inverted pair of heights was accepted")
	}
	if _, err := readSessionBundle(dir, sessionInventoryHeight, sessionInventoryHeight, author); err == nil {
		t.Fatal("the same height twice was accepted")
	}
	other := sessionAddress(0x42)
	if _, err := readSessionBundle(dir, sessionInventoryHeight, sessionVerdictHeight, hex.EncodeToString(other[:])); err == nil {
		t.Fatal("an author with no record was accepted")
	}
}

func TestSessionBundleRefusesAnUnclearedVerdictAndAMismatchedEpoch(t *testing.T) {
	addr := sessionAddress(0x51)
	uncleared := writeSessionDatadir(t, addr, sessionCIDs(2), state.AtLeastOneFlipNotQualified, false, fixtureEpoch+1)
	if _, err := readSessionBundle(uncleared, sessionInventoryHeight, sessionVerdictHeight, hex.EncodeToString(addr[:])); err == nil {
		t.Fatal("a verdict height that still lists flips was accepted")
	}

	mismatched := writeSessionDatadir(t, addr, sessionCIDs(2), 0, true, fixtureEpoch)
	if _, err := readSessionBundle(mismatched, sessionInventoryHeight, sessionVerdictHeight, hex.EncodeToString(addr[:])); err == nil {
		t.Fatal("an epoch that does not line up was accepted")
	}
}

func TestSessionBundleRefusesAnInventoryAnchorBeforeTheLottery(t *testing.T) {
	addr := sessionAddress(0x61)
	author := hex.EncodeToString(addr[:])

	// While the chain reports NonePeriod, a submission is still valid, so a
	// record captured there is not the epoch's final set: a later flip could
	// arrive and the manifest would not list it.
	open := writeSessionDatadirWithPeriod(t, addr, sessionCIDs(2), 0, true, fixtureEpoch+1, state.NonePeriod)
	if _, err := readSessionBundle(open, sessionInventoryHeight, sessionVerdictHeight, author); err == nil {
		t.Fatal("an inventory anchor taken while submissions were still open was accepted")
	}

	// The same record with the lottery period is the one the CLI must take.
	frozen := writeSessionDatadirWithPeriod(t, addr, sessionCIDs(2), 0, true, fixtureEpoch+1, state.FlipLotteryPeriod)
	bundle, err := readSessionBundle(frozen, sessionInventoryHeight, sessionVerdictHeight, author)
	if err != nil {
		t.Fatalf("a frozen inventory anchor was refused: %v", err)
	}
	if len(bundle.Manifest) != 2 {
		t.Fatalf("the accepted bundle carries %d flips", len(bundle.Manifest))
	}
}

// The Rust core under proofs/session-core reads this shape. Decoding the
// fixture the core is tested against into the CLI's own types keeps the two
// sides from drifting: a renamed field would leave a zero value here.
func TestSessionBundleShapeMatchesTheCoreFixture(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/session-core/fixtures/session-bundle.json")
	if err != nil {
		t.Fatalf("read the core fixture: %v", err)
	}
	var doc struct {
		Author   string   `json:"author"`
		Manifest []string `json:"manifest"`
		Sessions []struct {
			Label     string        `json:"label"`
			Epoch     uint16        `json:"epoch"`
			Inventory SessionAnchor `json:"inventory"`
			Verdict   SessionAnchor `json:"verdict"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse the core fixture: %v", err)
	}
	if len(doc.Author) != 40 || len(doc.Manifest) == 0 || len(doc.Sessions) == 0 {
		t.Fatalf("the fixture is author %q with %d cids and %d sessions", doc.Author, len(doc.Manifest), len(doc.Sessions))
	}
	for _, session := range doc.Sessions {
		if session.Inventory.Height == 0 || session.Verdict.Height == 0 {
			t.Fatalf("session %s has no anchor heights", session.Label)
		}
		if session.Inventory.Height >= session.Verdict.Height {
			t.Fatalf("session %s has the anchors in the wrong order", session.Label)
		}
		for _, anchor := range []SessionAnchor{session.Inventory, session.Verdict} {
			if len(anchor.RootHex) != 64 {
				t.Fatalf("session %s has a %d-character root", session.Label, len(anchor.RootHex))
			}
			if anchor.Global.KeyHex != "03" {
				t.Fatalf("session %s has global key %q", session.Label, anchor.Global.KeyHex)
			}
			if !strings.HasPrefix(anchor.Identity.KeyHex, "02"+doc.Author) {
				t.Fatalf("session %s has identity key %q", session.Label, anchor.Identity.KeyHex)
			}
			if anchor.Identity.ValueHex == "" || len(anchor.Identity.Path) == 0 {
				t.Fatalf("session %s carries an empty identity proof", session.Label)
			}
			if anchor.Identity.ValueHashHex == "" || anchor.Identity.Version == 0 {
				t.Fatalf("session %s carries no leaf hash or version", session.Label)
			}
		}
	}
}

func registrationWitnessTestKey(t *testing.T, dir string) (string, string) {
	t.Helper()
	keyPath, _ := writeTestKey(t, dir, "nodekey")
	identity, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	author, err := nodekeyAddressOf(identity)
	if err != nil {
		t.Fatal(err)
	}
	return keyPath, author
}

func TestRegistrationBlobInputCoversEveryManifestCID(t *testing.T) {
	for _, count := range []int{3, 5} {
		t.Run(fmt.Sprintf("%d flips", count), func(t *testing.T) {
			dir := t.TempDir()
			keyPath, author := registrationWitnessTestKey(t, dir)
			blobDir := filepath.Join(dir, "blobs")
			if err := os.Mkdir(blobDir, 0o700); err != nil {
				t.Fatal(err)
			}
			bundle := &SessionBundle{Author: author, Epoch: fixtureEpoch}
			for i := 0; i < count; i++ {
				body := []byte(fmt.Sprintf("encrypted flip %d", i))
				cid := cidRawSha256(body)
				bundle.Manifest = append(bundle.Manifest, cid)
				if err := os.WriteFile(filepath.Join(blobDir, cid), body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			out := filepath.Join(dir, "private-witness.json")
			if err := writeRegistrationBlobInput(out, "", keyPath, "", blobDir, bundle, nil); err != nil {
				t.Fatalf("write the witness: %v", err)
			}
			info, err := os.Stat(out)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("private witness mode: %v, %v", info, err)
			}
			raw, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if _, old := fields["blobCid"]; old {
				t.Fatal("the multi-flip witness also carries the single-flip fields")
			}
			if _, old := fields["blob"]; old {
				t.Fatal("the multi-flip witness also carries the single-flip fields")
			}
			var witness RegistrationBlobInput
			if err := json.Unmarshal(raw, &witness); err != nil {
				t.Fatal(err)
			}
			if witness.Author != bundle.Author || witness.Epoch != bundle.Epoch ||
				len(witness.Blobs) != count || witness.BlobCid != "" || witness.Blob != "" ||
				len(witness.PublicPartSecret) != 64 || len(witness.PrivatePartSecret) != 64 {
				t.Fatal("the witness does not carry every blob and one author epoch-secret pair")
			}
			for i, cid := range bundle.Manifest {
				if witness.Blobs[i].BlobCid != cid {
					t.Fatalf("blob %d is out of manifest order", i)
				}
				bytes, err := hex.DecodeString(witness.Blobs[i].Blob)
				if err != nil {
					t.Fatal(err)
				}
				if ok, _ := verifyBlobCID(cid, bytes, nil); !ok {
					t.Fatalf("blob %d does not match its CID", i)
				}
			}
		})
	}
}

func TestRegistrationBlobInputKeepsSingleFlipShape(t *testing.T) {
	dir := t.TempDir()
	keyPath, author := registrationWitnessTestKey(t, dir)
	body := []byte("one encrypted flip")
	file := filepath.Join(dir, "flip.blob")
	if err := os.WriteFile(file, body, 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := &SessionBundle{Author: author, Epoch: fixtureEpoch, Manifest: []string{cidRawSha256(body)}}
	out := filepath.Join(dir, "private-witness.json")
	if err := writeRegistrationBlobInput(out, "", keyPath, file, "", bundle, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, multi := fields["blobs"]; multi {
		t.Fatal("the single-flip witness unexpectedly carries a blobs array")
	}
	var witness RegistrationBlobInput
	if err := json.Unmarshal(raw, &witness); err != nil {
		t.Fatal(err)
	}
	if witness.BlobCid != bundle.Manifest[0] || witness.Blob != hex.EncodeToString(body) || len(witness.Blobs) != 0 {
		t.Fatal("the single-flip witness shape changed")
	}
}

func TestRegistrationBlobInputRejectsAnotherAuthorsKey(t *testing.T) {
	dir := t.TempDir()
	_, author := registrationWitnessTestKey(t, t.TempDir())
	otherKey, _ := registrationWitnessTestKey(t, t.TempDir())
	body := []byte("one encrypted flip")
	file := filepath.Join(dir, "flip.blob")
	if err := os.WriteFile(file, body, 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := &SessionBundle{Author: author, Epoch: fixtureEpoch, Manifest: []string{cidRawSha256(body)}}
	out := filepath.Join(dir, "private-witness.json")
	if err := writeRegistrationBlobInput(out, "", otherKey, file, "", bundle, nil); err == nil {
		t.Fatal("another author's key produced this author's witness")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("the wrong key wrote a private witness")
	}
}

func TestRegistrationBlobInputRequiresManifestFilesAndIgnoresOthers(t *testing.T) {
	dir := t.TempDir()
	keyPath, author := registrationWitnessTestKey(t, dir)
	blobDir := filepath.Join(dir, "blobs")
	if err := os.Mkdir(blobDir, 0o700); err != nil {
		t.Fatal(err)
	}
	bundle := &SessionBundle{Author: author, Epoch: fixtureEpoch}
	for i := 0; i < 3; i++ {
		body := []byte(fmt.Sprintf("encrypted flip %d", i))
		cid := cidRawSha256(body)
		bundle.Manifest = append(bundle.Manifest, cid)
		if err := os.WriteFile(filepath.Join(blobDir, cid), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "private-witness.json")
	write := func() error {
		return writeRegistrationBlobInput(out, "", keyPath, "", blobDir, bundle, nil)
	}
	if err := os.Remove(filepath.Join(blobDir, bundle.Manifest[0])); err != nil {
		t.Fatal(err)
	}
	if err := write(); err == nil {
		t.Fatal("a missing blob was accepted")
	}
	if err := os.WriteFile(filepath.Join(blobDir, bundle.Manifest[0]), []byte("substituted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := write(); err == nil {
		t.Fatal("a substituted blob was accepted")
	}
	if err := os.WriteFile(filepath.Join(blobDir, bundle.Manifest[0]), []byte("encrypted flip 0"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldBody := []byte("old encrypted flip")
	oldCID := cidRawSha256(oldBody)
	if err := os.WriteFile(filepath.Join(blobDir, oldCID), oldBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blobDir, ".blob-in-progress"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := write(); err != nil {
		t.Fatalf("stale or temporary blob files blocked the manifest: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var witness RegistrationBlobInput
	if err := json.Unmarshal(raw, &witness); err != nil {
		t.Fatal(err)
	}
	if len(witness.Blobs) != len(bundle.Manifest) {
		t.Fatal("the witness includes an extra or omits a manifest blob")
	}
	for i, cid := range bundle.Manifest {
		if witness.Blobs[i].BlobCid != cid {
			t.Fatalf("witness blob %d is not the manifest CID", i)
		}
	}
	if err := os.Remove(out); err != nil {
		t.Fatal(err)
	}
	manifestFile := filepath.Join(blobDir, bundle.Manifest[1])
	if err := os.Remove(manifestFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(manifestFile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := write(); err == nil {
		t.Fatal("a manifest CID named a directory instead of a regular blob file")
	}
	if err := os.Remove(manifestFile); err != nil {
		t.Fatal(err)
	}
	bundle.Manifest[1] = bundle.Manifest[0]
	if err := write(); err == nil {
		t.Fatal("a duplicate manifest CID was accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("a rejected input wrote a private witness")
	}
}
