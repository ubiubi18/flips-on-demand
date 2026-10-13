package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cosmos/iavl"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	"github.com/syndtr/goleveldb/leveldb/opt"
	dbm "github.com/tendermint/tm-db"
)

// readProofBundleFile loads a bundle this tool wrote, so a seller can carry it
// into a registration input.
func readProofBundleFile(path string) (*ProofBundle, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read proof bundle: %w", err)
	}
	var bundle ProofBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, fmt.Errorf("parse proof bundle %s: %w", path, err)
	}
	return &bundle, nil
}

// validateProofAnchor refuses a bundle that could not be verified, before a
// fifteen-minute proof is started. It mirrors what the guest will require.
func validateProofAnchor(bundle *ProofBundle) error {
	if bundle == nil {
		return nil
	}
	root, err := hex.DecodeString(bundle.RootHex)
	if err != nil || len(root) != 32 {
		return fmt.Errorf("the proof anchor's root is not a 32-byte digest")
	}
	if bundle.Tree != "identity" && bundle.Tree != "state" {
		return fmt.Errorf("the proof anchor's tree is %q, not identity or state", bundle.Tree)
	}
	if bundle.Height <= 0 || len(bundle.Proofs) == 0 {
		return fmt.Errorf("the proof anchor needs a height and at least one proof")
	}
	for index, proof := range bundle.Proofs {
		key, err := hex.DecodeString(proof.KeyHex)
		if err != nil || len(key) != 21 {
			return fmt.Errorf("proof anchor entry %d has no 21-byte identity key", index)
		}
		if _, err := hex.DecodeString(proof.ValueHex); err != nil {
			return fmt.Errorf("proof anchor entry %d has no readable value", index)
		}
		if len(proof.Path) == 0 {
			return fmt.Errorf("proof anchor entry %d carries no path", index)
		}
	}
	return nil
}

// readAddressList reads the addresses a bundle has to cover: a JSON array of hex
// strings, or one address per line with `#` comments.
func readAddressList(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read address list: %w", err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var addresses []string
		if err := json.Unmarshal([]byte(trimmed), &addresses); err != nil {
			return nil, fmt.Errorf("parse address list %s: %w", path, err)
		}
		return addresses, nil
	}
	var addresses []string
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		addresses = append(addresses, line)
	}
	return addresses, nil
}

// StateProofSummary is one identity record with its membership proof against the
// state root: the artifact an anchor bundle has to carry so that a verifier
// without historical state can check that a shard input is real.
//
// The state tree is IAVL, so the proof is a range proof whose leaf carries the
// key and the value hash and whose path carries the sibling hashes. ProofBytes
// counts those fields as they would sit in a bundle; it is the natural size, not
// a frozen wire format.
type StateProofSummary struct {
	DataDir       string `json:"dataDir"`
	Tree          string `json:"tree"`
	Height        int64  `json:"height"`
	Versions      []int  `json:"availableVersions,omitempty"`
	Address       string `json:"address,omitempty"`
	KeyHex        string `json:"keyHex,omitempty"`
	IdentityBytes int    `json:"identityBytes"`
	IdentityKeys  int    `json:"identityKeys"`
	ProofBytes    int    `json:"proofBytes"`
	PathItems     int    `json:"pathItems"`
	RootHex       string `json:"rootHex,omitempty"`
	Verified      bool   `json:"verified"`
	VerifyError   string `json:"verifyError,omitempty"`
}

// ProofBundle is what a verifier without the tree needs: one version's root and,
// per record, the value with the leaf and path that place it in the tree. The
// spike under spikes/iavl-proof consumes this shape.
type ProofBundle struct {
	Tree    string             `json:"tree"`
	Height  int64              `json:"height"`
	RootHex string             `json:"rootHex"`
	Proofs  []ProofBundleEntry `json:"proofs"`
}

type ProofBundleEntry struct {
	KeyHex       string          `json:"keyHex"`
	ValueHex     string          `json:"valueHex"`
	ValueHashHex string          `json:"valueHashHex"`
	Version      int64           `json:"version"`
	Path         []ProofPathItem `json:"path"`
}

type ProofPathItem struct {
	Height   int64  `json:"height"`
	Size     int64  `json:"size"`
	Version  int64  `json:"version"`
	LeftHex  string `json:"leftHex"`
	RightHex string `json:"rightHex"`
}

// readProofBundle collects up to limit identity proofs from one version of the
// named tree, with the root every proof has to fold to.
// readProofBundle collects one identity proof per requested address, or the
// first limit identity proofs when no addresses are given, from one version of
// the named tree. expectRoot, when set, has to equal that version's root: the
// bundle is only useful if it proves the root the manifest commits.
func readProofBundle(dataDir string, height int64, limit int, treeName string, addresses []string, expectRoot string) (*ProofBundle, error) {
	if height == 0 || (limit <= 0 && len(addresses) == 0) {
		return nil, fmt.Errorf("a proof bundle needs -state-height N and either -proof-limit N or -addresses FILE")
	}
	opened, err := openTree(dataDir, treeName)
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	if _, err := opened.tree.Load(); err != nil {
		return nil, fmt.Errorf("no %s versions in %s: %w", opened.name, dataDir, err)
	}
	if _, err := opened.tree.LoadVersion(height); err != nil {
		return nil, fmt.Errorf("load %s at height %d: %w", opened.name, height, err)
	}
	imm, err := opened.tree.GetImmutable(height)
	if err != nil {
		return nil, fmt.Errorf("%s at height %d: %w", opened.name, height, err)
	}

	rootHex := hex.EncodeToString(imm.Hash())
	if expectRoot != "" {
		want := strings.TrimPrefix(strings.ToLower(expectRoot), "0x")
		if !strings.EqualFold(rootHex, want) {
			return nil, fmt.Errorf("the %s tree at height %d has root %s, not the expected %s", opened.name, height, rootHex, want)
		}
	}

	var keys [][]byte
	if len(addresses) > 0 {
		for _, address := range addresses {
			addr, err := parseIdentityAddress(address)
			if err != nil {
				return nil, err
			}
			keys = append(keys, state.StateDbKeys.IdentityKey(addr))
		}
	} else {
		iterateIdentityRange(imm, func(key, value []byte) bool {
			if len(keys) >= limit {
				return true
			}
			keys = append(keys, append([]byte{}, key...))
			return len(keys) >= limit
		})
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no identity keys in %s at height %d", opened.name, height)
	}

	bundle := &ProofBundle{
		Tree:    opened.name,
		Height:  height,
		RootHex: rootHex,
		Proofs:  make([]ProofBundleEntry, 0, len(keys)),
	}
	for _, key := range keys {
		value, proof, err := imm.GetWithProof(key)
		if err != nil {
			return nil, fmt.Errorf("no identity record for %s at height %d: %w", addressString(key), height, err)
		}
		if value == nil || len(proof.Leaves) != 1 {
			return nil, fmt.Errorf("no identity record for %s at height %d", addressString(key), height)
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
		bundle.Proofs = append(bundle.Proofs, entry)
	}
	return bundle, nil
}

// readStateProof opens the chain database read-only, loads one version of the
// identity tree or the ledger-side state tree, and returns one identity record
// with its proof and the version's root hash. The identity tree is the one the
// header commits as IdentityRoot and the one the candidate check reads; the
// state tree carries the ledger-side record, flips included. A height of zero
// lists the versions the datadir still holds instead.
func readStateProof(dataDir string, height int64, address string, treeName string) (*StateProofSummary, error) {
	opened, err := openTree(dataDir, treeName)
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	tree, treeName := opened.tree, opened.name

	summary := &StateProofSummary{DataDir: dataDir, Tree: treeName, Height: height}
	// A fresh handle knows nothing until it loads the roots the datadir holds.
	if _, err := tree.Load(); err != nil {
		if height == 0 {
			return summary, nil
		}
		return nil, fmt.Errorf("no %s versions in %s: %w", treeName, dataDir, err)
	}
	summary.Versions = tree.AvailableVersions()
	if height == 0 {
		return summary, nil
	}
	if _, err := tree.LoadVersion(height); err != nil {
		return nil, fmt.Errorf("load state at height %d: %w", height, err)
	}
	imm, err := tree.GetImmutable(height)
	if err != nil {
		return nil, fmt.Errorf("state at height %d: %w", height, err)
	}

	key, err := findIdentityKey(imm, address)
	if err != nil {
		return nil, err
	}
	summary.IdentityKeys = countIdentityKeys(imm)
	value, proof, err := imm.GetWithProof(key)
	if err != nil {
		return nil, fmt.Errorf("state proof for %s: %w", hex.EncodeToString(key), err)
	}
	if value == nil || len(proof.Leaves) != 1 {
		return nil, fmt.Errorf("state proof for %s: no single leaf", hex.EncodeToString(key))
	}

	root := imm.Hash()
	summary.Address = addressString(key)
	summary.KeyHex = hex.EncodeToString(proof.Leaves[0].Key)
	summary.IdentityBytes = len(value)
	summary.ProofBytes, summary.PathItems = stateProofSize(proof)
	summary.RootHex = hex.EncodeToString(root)
	if err := proof.Verify(root); err != nil {
		summary.VerifyError = err.Error()
	} else {
		summary.Verified = true
	}
	return summary, nil
}

// openedTree is one loaded IAVL tree with the database it lives in.
type openedTree struct {
	name string
	db   dbm.DB
	tree *iavl.MutableTree
}

func (o *openedTree) Close() {
	_ = o.db.Close()
}

// openTree opens the chain database read-only and returns the IAVL tree behind
// the named tree's prefix: the identity tree the header commits as IdentityRoot,
// or the ledger-side state tree.
func openTree(dataDir string, treeName string) (*openedTree, error) {
	db, err := dbm.NewGoLevelDBWithOpts("idenachain", dataDir, &opt.Options{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("open chain db read-only: %w", err)
	}
	if treeName == "" {
		treeName = "identity"
	}
	var prefix []byte
	switch treeName {
	case "identity":
		prefix, err = state.IdentityStateDbKeys.LoadDbPrefix(db, false)
	case "state":
		prefix, err = state.StateDbKeys.LoadDbPrefix(db)
	default:
		_ = db.Close()
		return nil, fmt.Errorf("unknown tree %q: use identity or state", treeName)
	}
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load %s db prefix: %w", treeName, err)
	}
	tree, err := iavl.NewMutableTree(dbm.NewPrefixDB(db, prefix), 1024)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open %s tree: %w", treeName, err)
	}
	return &openedTree{name: treeName, db: db, tree: tree}, nil
}

// iterateIdentityRange walks every identity record at one loaded version. The
// callback returns true to stop, so a caller that wants the whole range must
// return false; an earlier revision of the state reader returned true for every
// key and reported a single identity for every datadir.
func iterateIdentityRange(imm *iavl.ImmutableTree, fn func(key, value []byte) bool) {
	base, end := identityKeyRange()
	imm.IterateRange(base, end, true, fn)
}

// findIdentityKey returns the key to prove: the caller's address when given,
// otherwise the first identity at that state version.
func findIdentityKey(imm *iavl.ImmutableTree, address string) ([]byte, error) {
	if address != "" {
		addr, err := parseIdentityAddress(address)
		if err != nil {
			return nil, err
		}
		return state.StateDbKeys.IdentityKey(addr), nil
	}

	var found []byte
	iterateIdentityRange(imm, func(key, value []byte) bool {
		found = append([]byte{}, key...)
		return true
	})
	if found == nil {
		return nil, fmt.Errorf("no identity key at this state version")
	}
	return found, nil
}

// identityKeyRange is the half-open key range every identity record sits in: the
// identity prefix plus the address, from the zero address to its successor.
func identityKeyRange() ([]byte, []byte) {
	base := state.StateDbKeys.IdentityKey(common.Address{})
	base = base[:len(base)-common.AddressLength]
	end := append([]byte{}, base...)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] != 0xff {
			end[i]++
			end = end[:i+1]
			break
		}
	}
	return base, end
}

// countIdentityKeys reports how many identity records the loaded version holds,
// which is what tells a full datadir apart from a lite one.
func countIdentityKeys(imm *iavl.ImmutableTree) int {
	count := 0
	iterateIdentityRange(imm, func(key, value []byte) bool {
		count++
		return false
	})
	return count
}

func parseIdentityAddress(address string) (common.Address, error) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(address, "0x"), "0X")
	raw, err := hex.DecodeString(trimmed)
	if err != nil || len(raw) != common.AddressLength {
		return common.Address{}, fmt.Errorf("identity address %q is not 20 bytes of hex", address)
	}
	return common.BytesToAddress(raw), nil
}

// addressString renders an identity key as an address: the identity prefix is
// one byte and the address is the twenty that follow it.
func addressString(key []byte) string {
	if len(key) < common.AddressLength {
		return ""
	}
	return "0x" + hex.EncodeToString(key[len(key)-common.AddressLength:])
}

// stateProofSize counts what a bundle would carry for one identity proof: the
// leaf's key, value hash, and version, and every inner node's height, size,
// version, and sibling hashes.
func stateProofSize(proof *iavl.RangeProof) (int, int) {
	size, items := 0, 0
	for _, leaf := range proof.Leaves {
		size += len(leaf.Key) + len(leaf.ValueHash) + 8
		items++
	}
	count := func(nodes []iavl.ProofInnerNode) {
		for _, node := range nodes {
			size += 1 + 8 + 8 + len(node.Left) + len(node.Right)
			items++
		}
	}
	count(proof.LeftPath)
	for _, path := range proof.InnerNodes {
		count(path)
	}
	return size, items
}
