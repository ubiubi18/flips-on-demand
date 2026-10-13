package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cosmos/iavl"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
)

const maxSessionAnchorFile = 1 << 20

// readSessionAnchor writes only the two public state leaves needed for one
// anchor. The source database may discard this version after 100 more blocks;
// the resulting proof remains portable and can be checked against a header.
func readSessionAnchor(dataDir string, height int64, author string) (*SessionAnchor, error) {
	if dataDir == "" || height <= 0 {
		return nil, fmt.Errorf("a session anchor needs a datadir and positive state height")
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
	anchor, err := sessionAnchorAt(opened, height, addr)
	if err != nil {
		return nil, err
	}
	if err := verifySessionAnchor(anchor, addr); err != nil {
		return nil, err
	}
	return anchor, nil
}

func readSessionAnchorFile(path string) (*SessionAnchor, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSessionAnchorFile {
		return nil, fmt.Errorf("session anchor is not a bounded regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var anchor SessionAnchor
	if err := json.Unmarshal(raw, &anchor); err != nil {
		return nil, err
	}
	return &anchor, nil
}

func verifySessionAnchor(anchor *SessionAnchor, addr common.Address) error {
	if anchor == nil || anchor.Height == 0 {
		return fmt.Errorf("missing anchor height")
	}
	root, err := hex.DecodeString(anchor.RootHex)
	if err != nil || len(root) != 32 {
		return fmt.Errorf("invalid 32-byte state root")
	}
	for _, item := range []struct {
		name  string
		key   []byte
		entry ProofBundleEntry
	}{
		{"global", state.StateDbKeys.GlobalKey(), anchor.Global},
		{"identity", state.StateDbKeys.IdentityKey(addr), anchor.Identity},
	} {
		if err := verifySessionEntry(root, item.key, item.entry); err != nil {
			return fmt.Errorf("%s proof: %w", item.name, err)
		}
	}
	return nil
}

func verifySessionEntry(root, expectedKey []byte, entry ProofBundleEntry) error {
	key, err := hex.DecodeString(entry.KeyHex)
	if err != nil || !bytes.Equal(key, expectedKey) {
		return fmt.Errorf("wrong state key")
	}
	value, err := hex.DecodeString(entry.ValueHex)
	if err != nil || len(value) == 0 || len(value) > maxSessionAnchorFile {
		return fmt.Errorf("invalid state value")
	}
	valueHash, err := hex.DecodeString(entry.ValueHashHex)
	if err != nil || len(valueHash) != 32 {
		return fmt.Errorf("invalid value hash")
	}
	if len(entry.Path) == 0 || len(entry.Path) > 128 || entry.Version <= 0 {
		return fmt.Errorf("invalid proof path or version")
	}
	path := make(iavl.PathToLeaf, 0, len(entry.Path))
	for _, item := range entry.Path {
		if item.Height < 0 || item.Height > 127 || item.Size < 0 || item.Version <= 0 {
			return fmt.Errorf("invalid proof node")
		}
		left, err := hex.DecodeString(item.LeftHex)
		if err != nil || len(left) > 32 {
			return fmt.Errorf("invalid left hash")
		}
		right, err := hex.DecodeString(item.RightHex)
		if err != nil || len(right) > 32 {
			return fmt.Errorf("invalid right hash")
		}
		path = append(path, iavl.ProofInnerNode{
			Height: int8(item.Height), Size: item.Size, Version: item.Version,
			Left: left, Right: right,
		})
	}
	proof := &iavl.RangeProof{
		LeftPath: path,
		Leaves:   []iavl.ProofLeafNode{{Key: key, ValueHash: valueHash, Version: entry.Version}},
	}
	if err := proof.Verify(root); err != nil {
		return fmt.Errorf("does not fold to root: %w", err)
	}
	if err := proof.VerifyItem(key, value); err != nil {
		return fmt.Errorf("value does not match leaf: %w", err)
	}
	return nil
}
