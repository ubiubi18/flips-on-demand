package main

// The two-anchor session bundle of docs/SOURCES.md F-97, produced from a node
// datadir.
//
// The chain keeps an author's epoch in two state versions and never in one: the
// flips while the epoch runs, and the validation status flags at the block that
// finishes it (which also clears the flips). The bundle therefore carries two
// anchored leaves - the author's identity record and the global record (which
// names the epoch) at each of the two heights - and the flip CIDs the inventory
// record lists. `proofs/session-core` folds and decodes exactly this shape; the
// contract authenticates each height against the node's own header.

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/iavl"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
)

const (
	sessionBundleFormat  = "fod.session-bundle"
	sessionBundleVersion = 1
)

// SessionAnchor is one state version: the height, the root every proof folds
// to, and the two records the bundle needs.
type SessionAnchor struct {
	Height   uint64           `json:"height"`
	RootHex  string           `json:"rootHex"`
	Global   ProofBundleEntry `json:"global"`
	Identity ProofBundleEntry `json:"identity"`
}

// SessionBundle is what a seller publishes for one author and epoch.
type SessionBundle struct {
	Format    string        `json:"format"`
	Version   int           `json:"version"`
	Author    string        `json:"author"`
	Epoch     uint16        `json:"epoch"`
	Manifest  []string      `json:"manifest"`
	Inventory SessionAnchor `json:"inventory"`
	Verdict   SessionAnchor `json:"verdict"`
}

// proofEntryAt proves one key at one version and checks the proof before it is
// written into a bundle: a proof that does not fold to the version's own root
// would only waste a verifier's time.
func proofEntryAt(tree *iavl.ImmutableTree, key []byte) (ProofBundleEntry, error) {
	value, proof, err := tree.GetWithProof(key)
	if err != nil {
		return ProofBundleEntry{}, fmt.Errorf("no record for key %x at this version: %w", key, err)
	}
	if value == nil || len(proof.Leaves) != 1 {
		return ProofBundleEntry{}, fmt.Errorf("no record for key %x at this version", key)
	}
	if err := proof.Verify(tree.Hash()); err != nil {
		return ProofBundleEntry{}, fmt.Errorf("the proof for key %x does not fold to the version's root: %w", key, err)
	}
	entry := ProofBundleEntry{
		KeyHex:       hex.EncodeToString(proof.Leaves[0].Key),
		ValueHex:     hex.EncodeToString(value),
		ValueHashHex: hex.EncodeToString(proof.Leaves[0].ValueHash),
		Version:      proof.Leaves[0].Version,
		Path:         make([]ProofPathItem, 0, len(proof.LeftPath)),
	}
	for _, item := range proof.LeftPath {
		entry.Path = append(entry.Path, ProofPathItem{
			Height:   int64(item.Height),
			Size:     item.Size,
			Version:  item.Version,
			LeftHex:  hex.EncodeToString(item.Left),
			RightHex: hex.EncodeToString(item.Right),
		})
	}
	return entry, nil
}

func sessionAnchorAt(opened *openedTree, height int64, addr common.Address) (*SessionAnchor, error) {
	if _, err := opened.tree.LoadVersion(height); err != nil {
		return nil, fmt.Errorf("load the state at height %d: %w", height, err)
	}
	imm, err := opened.tree.GetImmutable(height)
	if err != nil {
		return nil, fmt.Errorf("the state at height %d: %w", height, err)
	}
	global, err := proofEntryAt(imm, state.StateDbKeys.GlobalKey())
	if err != nil {
		return nil, err
	}
	identity, err := proofEntryAt(imm, state.StateDbKeys.IdentityKey(addr))
	if err != nil {
		return nil, err
	}
	return &SessionAnchor{
		Height:   uint64(height),
		RootHex:  hex.EncodeToString(imm.Hash()),
		Global:   global,
		Identity: identity,
	}, nil
}

// readSessionBundle reads both anchors for one author and refuses to write a
// bundle the verifier would reject: no flips at the inventory height, flips
// left at the verdict height, or an epoch that does not line up.
func readSessionBundle(dataDir string, inventoryHeight, verdictHeight int64, author string) (*SessionBundle, error) {
	if inventoryHeight <= 0 || verdictHeight <= 0 {
		return nil, fmt.Errorf("a session bundle needs -inventory-height N and -verdict-height N")
	}
	if verdictHeight <= inventoryHeight {
		return nil, fmt.Errorf("the verdict height %d is not after the inventory height %d", verdictHeight, inventoryHeight)
	}
	if author == "" {
		return nil, fmt.Errorf("a session bundle needs -author ADDRESS")
	}
	addr, err := parseIdentityAddress(author)
	if err != nil {
		return nil, err
	}

	opened, err := openTree(dataDir, "state")
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	if _, err := opened.tree.Load(); err != nil {
		return nil, fmt.Errorf("no state versions in %s: %w", dataDir, err)
	}

	inventory, err := sessionAnchorAt(opened, inventoryHeight, addr)
	if err != nil {
		return nil, err
	}
	verdict, err := sessionAnchorAt(opened, verdictHeight, addr)
	if err != nil {
		return nil, err
	}
	return sessionBundleFromAnchors(author, inventory, verdict)
}

// sessionBundleFromAnchors accepts portable records captured separately before
// their source state versions are pruned. Every leaf is verified against its
// saved root again; a JSON file or its publisher is never the authority.
func sessionBundleFromAnchors(author string, inventory, verdict *SessionAnchor) (*SessionBundle, error) {
	addr, err := parseIdentityAddress(author)
	if err != nil {
		return nil, err
	}
	if inventory == nil || verdict == nil || inventory.Height == 0 || verdict.Height <= inventory.Height {
		return nil, fmt.Errorf("session anchors need an inventory height before the verdict height")
	}
	if err := verifySessionAnchor(inventory, addr); err != nil {
		return nil, fmt.Errorf("inventory anchor: %w", err)
	}
	if err := verifySessionAnchor(verdict, addr); err != nil {
		return nil, fmt.Errorf("verdict anchor: %w", err)
	}

	inventoryValue, err := hex.DecodeString(inventory.Identity.ValueHex)
	if err != nil {
		return nil, fmt.Errorf("decode the inventory record: %w", err)
	}
	var inventoryRecord state.Identity
	if err := inventoryRecord.FromBytes(inventoryValue); err != nil {
		return nil, fmt.Errorf("read the inventory record: %w", err)
	}
	if len(inventoryRecord.Flips) == 0 {
		return nil, fmt.Errorf("the record at height %d lists no flips: that height is not inside the epoch's flip phase", inventory.Height)
	}
	manifest := make([]string, 0, len(inventoryRecord.Flips))
	for index, flip := range inventoryRecord.Flips {
		if len(flip.Cid) == 0 {
			return nil, fmt.Errorf("the flip at index %d carries no CID", index)
		}
		// The record keeps the CIDv1 value as its raw bytes (`cid.Bytes()` in
		// the submission attachment); a bundle, the availability advisory and
		// the blob's own CID all name the multibase text. Anything that is not
		// a CIDv1 this build can render is refused rather than guessed at.
		cidText := cidString(flip.Cid)
		if cidText == "" {
			return nil, fmt.Errorf(
				"the flip at index %d carries %x, which is not a CIDv1 value this build renders",
				index, flip.Cid)
		}
		manifest = append(manifest, cidText)
	}

	verdictValue, err := hex.DecodeString(verdict.Identity.ValueHex)
	if err != nil {
		return nil, fmt.Errorf("decode the verdict record: %w", err)
	}
	var verdictRecord state.Identity
	if err := verdictRecord.FromBytes(verdictValue); err != nil {
		return nil, fmt.Errorf("read the verdict record: %w", err)
	}
	if len(verdictRecord.Flips) != 0 {
		return nil, fmt.Errorf("the record at height %d still lists flips: the verdict anchor has to be the ValidationFinished block", verdict.Height)
	}

	var inventoryGlobal, verdictGlobal state.Global
	inventoryGlobalValue, err := hex.DecodeString(inventory.Global.ValueHex)
	if err != nil {
		return nil, fmt.Errorf("decode the inventory global record: %w", err)
	}
	if err := inventoryGlobal.FromBytes(inventoryGlobalValue); err != nil {
		return nil, fmt.Errorf("read the inventory global record: %w", err)
	}
	verdictGlobalValue, err := hex.DecodeString(verdict.Global.ValueHex)
	if err != nil {
		return nil, fmt.Errorf("decode the verdict global record: %w", err)
	}
	if err := verdictGlobal.FromBytes(verdictGlobalValue); err != nil {
		return nil, fmt.Errorf("read the verdict global record: %w", err)
	}
	if verdictGlobal.Epoch != inventoryGlobal.Epoch+1 {
		return nil, fmt.Errorf("the epochs do not line up: %d at the inventory height and %d at the verdict height",
			inventoryGlobal.Epoch, verdictGlobal.Epoch)
	}
	// Membership at the inventory height is not completeness. The chain accepts
	// a flip submission while the period is `None`, so an anchor taken then can
	// be followed by a flip this bundle does not list; from the lottery on, a
	// submission is refused (`LateTx`), and within one epoch the period only
	// advances. `SOURCES.md` F-97.
	if inventoryGlobal.ValidationPeriod < state.FlipLotteryPeriod {
		return nil, fmt.Errorf(
			"the global record at height %d reports validation period %d, so a flip submission was still valid there: capture the inventory anchor at or after the flip lottery starts",
			inventory.Height, inventoryGlobal.ValidationPeriod)
	}

	return &SessionBundle{
		Format:    sessionBundleFormat,
		Version:   sessionBundleVersion,
		Author:    hex.EncodeToString(addr[:]),
		Epoch:     inventoryGlobal.Epoch,
		Manifest:  manifest,
		Inventory: *inventory,
		Verdict:   *verdict,
	}, nil
}

// RegistrationBlobInput is the registration host's private witness: one blob
// for each manifest CID and the author's two epoch secrets. The single-flip
// fields preserve the original host input format.
type RegistrationBlobInput struct {
	Note              string             `json:"note"`
	Author            string             `json:"author"`
	Epoch             uint16             `json:"epoch"`
	BlobCid           string             `json:"blobCid,omitempty"`
	Blob              string             `json:"blob,omitempty"`
	Blobs             []RegistrationBlob `json:"blobs,omitempty"`
	PublicPartSecret  string             `json:"publicPartSecret"`
	PrivatePartSecret string             `json:"privatePartSecret"`
}

type RegistrationBlob struct {
	BlobCid string `json:"blobCid"`
	Blob    string `json:"blob"`
}

// writeRegistrationBlobInput reads every manifest blob from a CID-named
// directory or the node's IPFS, verifies its CID locally, then derives the two
// epoch secrets from the author's identity key. Other directory entries may be
// stale CIDs or in-progress writes and are ignored. -blob FILE remains
// available for a single-flip bundle.
//
// The record, IPFS and witness all name each CID in canonical multibase text.
// The output carries private key material and must remain with the seller.
func writeRegistrationBlobInput(path, dataDir, nodekeyPath, blobFile, blobDir string, bundle *SessionBundle, node *client) error {
	if len(bundle.Manifest) == 0 {
		return fmt.Errorf("the registration bundle lists no flip blobs")
	}
	if blobFile != "" && blobDir != "" {
		return fmt.Errorf("choose either -blob FILE or -blob-dir DIR")
	}
	if blobFile != "" && len(bundle.Manifest) != 1 {
		return fmt.Errorf("-blob FILE supplies one flip blob; this bundle covers %d: use -blob-dir DIR or -rpc", len(bundle.Manifest))
	}
	seen := make(map[string]bool, len(bundle.Manifest))
	for _, cidText := range bundle.Manifest {
		if _, err := cidBytesFromString(cidText); err != nil {
			return fmt.Errorf("the manifest's CID: %w", err)
		}
		if seen[cidText] {
			return fmt.Errorf("the manifest repeats CID %s", cidText)
		}
		seen[cidText] = true
	}
	blobs := make([]RegistrationBlob, 0, len(bundle.Manifest))
	for _, cidText := range bundle.Manifest {
		var body []byte
		if blobDir != "" || blobFile != "" {
			file := blobFile
			if blobDir != "" {
				file = filepath.Join(blobDir, cidText)
				info, err := os.Lstat(file)
				if err != nil {
					return fmt.Errorf("find the manifest's flip blob %s: %w", file, err)
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf("the manifest's flip blob %s is not a regular file", file)
				}
			}
			var err error
			body, err = os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read the flip blob %s: %w", file, err)
			}
		} else {
			if node == nil {
				return fmt.Errorf("the blob witness needs -blob FILE, -blob-dir DIR or -rpc: the capture cannot open the state database while the node holds it")
			}
			var fetched hexutil.Bytes
			if err := node.call("ipfs_get", []any{cidText}, &fetched); err != nil {
				return fmt.Errorf("fetch the flip blob %s from the node: %w", cidText, err)
			}
			body = []byte(fetched)
		}
		if ok, note := verifyBlobCID(cidText, body, nil); !ok {
			return fmt.Errorf("the flip blob %s failed its CID check: %s", cidText, note)
		}
		blobs = append(blobs, RegistrationBlob{BlobCid: cidText, Blob: hex.EncodeToString(body)})
	}
	keyPath := nodekeyPath
	if keyPath == "" {
		keyPath = filepath.Join(dataDir, "keystore", "nodekey")
	}
	identity, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		return fmt.Errorf("load the identity key %s: %w", keyPath, err)
	}
	keyAuthor, err := nodekeyAddressOf(identity)
	if err != nil {
		return fmt.Errorf("read the identity key's author: %w", err)
	}
	bundleAuthor, err := parseIdentityAddress(bundle.Author)
	if err != nil {
		return err
	}
	if keyAuthor != hex.EncodeToString(bundleAuthor[:]) {
		return fmt.Errorf("the nodekey does not belong to the bundle's author")
	}
	keys, err := DeriveEpochKeys(identity, bundle.Epoch)
	if err != nil {
		return fmt.Errorf("derive the epoch keys for epoch %d: %w", bundle.Epoch, err)
	}
	publicSecret, privateSecret := keys.SecretBytes()
	witness := RegistrationBlobInput{
		Note:              "Each flip blob matches its manifest CID; the epoch secrets are derived from the node key. Keep this private witness with the seller's keys.",
		Author:            bundle.Author,
		Epoch:             bundle.Epoch,
		PublicPartSecret:  hex.EncodeToString(publicSecret),
		PrivatePartSecret: hex.EncodeToString(privateSecret),
	}
	if len(blobs) == 1 {
		witness.BlobCid, witness.Blob = blobs[0].BlobCid, blobs[0].Blob
	} else {
		witness.Blobs = blobs
	}
	if err := writePrivateJSON(path, witness); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
