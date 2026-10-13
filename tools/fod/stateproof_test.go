package main

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/cosmos/iavl"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	dbm "github.com/tendermint/tm-db"
)

// TestCollectStateIdentitiesWalksEveryKey guards the regression that made every
// datadir report one identity: the walk's callback returned stop for each key,
// so the iterator ended after the first one.
func TestCollectStateIdentitiesWalksEveryKey(t *testing.T) {
	tree, err := iavl.NewMutableTree(dbm.NewMemDB(), 1024)
	if err != nil {
		t.Fatalf("new tree: %v", err)
	}
	for i := 0; i < 3; i++ {
		addr := common.BigToAddress(big.NewInt(int64(i + 1)))
		tree.Set(state.StateDbKeys.IdentityKey(addr), []byte{0x01})
	}
	if _, _, err := tree.SaveVersion(); err != nil {
		t.Fatalf("save version: %v", err)
	}
	imm, err := tree.GetImmutable(tree.Version())
	if err != nil {
		t.Fatalf("immutable version: %v", err)
	}
	summary, err := collectStateIdentities(imm, 5)
	if err != nil {
		t.Fatalf("collect identities: %v", err)
	}
	if summary.Identities != 3 {
		t.Fatalf("walked %d identities, want 3; a walk that stops after the first key is the bug this guards", summary.Identities)
	}
}

// TestStateProofVerifiesAgainstItsRoot exercises the measurement path without a
// datadir: a small IAVL tree with one identity key, a proof from the saved
// version, the pinned fork's verifier, and a tampered root that must fail.
func TestStateProofVerifiesAgainstItsRoot(t *testing.T) {
	tree, err := iavl.NewMutableTree(dbm.NewMemDB(), 1024)
	if err != nil {
		t.Fatalf("new tree: %v", err)
	}
	for i := 0; i < 64; i++ {
		tree.Set([]byte{0x02, byte(i)}, []byte("filler"))
	}
	address := common.HexToAddress("0x1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d")
	identity := []byte("identity record bytes")
	tree.Set(state.StateDbKeys.IdentityKey(address), identity)
	if _, _, err := tree.SaveVersion(); err != nil {
		t.Fatalf("save version: %v", err)
	}

	imm, err := tree.GetImmutable(tree.Version())
	if err != nil {
		t.Fatalf("immutable version: %v", err)
	}
	value, proof, err := imm.GetWithProof(state.StateDbKeys.IdentityKey(address))
	if err != nil {
		t.Fatalf("state proof: %v", err)
	}
	if !bytes.Equal(value, identity) {
		t.Fatalf("proof carries %q, want %q", value, identity)
	}
	if len(proof.Leaves) != 1 {
		t.Fatalf("proof has %d leaves, want 1", len(proof.Leaves))
	}
	if err := proof.Verify(imm.Hash()); err != nil {
		t.Fatalf("proof does not verify against its own root: %v", err)
	}
	size, items := stateProofSize(proof)
	if size <= 0 || items <= 0 {
		t.Fatalf("proof size %d and path items %d must both be positive", size, items)
	}

	tampered := append([]byte{}, imm.Hash()...)
	tampered[0] ^= 0x01
	if err := proof.Verify(tampered); err == nil {
		t.Fatal("a proof verified against a tampered root")
	}
}
