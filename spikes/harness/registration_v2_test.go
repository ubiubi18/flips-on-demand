package harness

// The version 2 registration receipt is the escrow's only door: the journal
// binds two anchored session state versions, the receipt's seal is verified
// against the pinned guest program, and the contract checks each anchor against
// the header its own chain carries for that height.
//
// The tests that register take the receipt through here, so the offsets, the
// artefact path and the two anchors are bound in one place instead of in each
// test's own copy of the layout.

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"testing"
)

// The artefact `proofs/registration-v2/host` writes with `--prove`. Its name is
// the proof program's label; the statement it carries is the version 3 layout.
const registrationV2Artefact = "../../proofs/registration-v2/dist/registration-v2.json"

// Journal offsets and sizes, matching `contracts/escrow/src/lib.rs` and
// `proofs/session-core/src/statement.rs`.
const (
	registrationV2JournalBytes    = 326
	registrationV2SealBytes       = 256
	registrationV2ManifestOffset  = 134
	registrationV2KeyOffset       = 166
	registrationV2InventoryOffset = 198
	registrationV2InventoryHeight = 50
	registrationV2InventoryRoot   = 58
	registrationV2VerdictHeight   = 90
	registrationV2VerdictRoot     = 98
	registrationV2NetworkOffset   = 294
)

type sealedRegistrationV2 struct {
	journal []byte
	seal    []byte
}

// loadRegistrationV2 reads the sealed artefact, or skips when it has not been
// generated. A fixture is evidence of the offsets; it is not chain data.
func loadRegistrationV2(t *testing.T) sealedRegistrationV2 {
	return loadRegistrationV2File(t, registrationV2Artefact)
}

func loadRegistrationV2File(t *testing.T, path string) sealedRegistrationV2 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the committed version 2 receipt: %v", err)
	}
	var artefact registrationArtifact
	if err := json.Unmarshal(raw, &artefact); err != nil {
		t.Fatalf("parse the registration artefact: %v", err)
	}
	journal := mustHex(t, artefact.Journal)
	seal := mustHex(t, artefact.Seal)
	if len(journal) != registrationV2JournalBytes || len(seal) != registrationV2SealBytes {
		t.Fatalf("the receipt is journal %d / seal %d bytes", len(journal), len(seal))
	}
	return sealedRegistrationV2{journal: journal, seal: seal}
}

// args is the `registerBundle` argument: the journal, then the seal.
func (r sealedRegistrationV2) args() []byte { return join(r.journal, r.seal) }

// author is the address the journal names, which is the only caller the escrow
// accepts a registration from.
func (r sealedRegistrationV2) author() []byte { return r.journal[24:44] }

// keyCommitment is the bundle key the escrow stores the record under.
func (r sealedRegistrationV2) keyCommitment() []byte {
	return r.journal[registrationV2KeyOffset : registrationV2KeyOffset+32]
}

// manifestCommitment is what the pool record carries as the bundle's manifest.
func (r sealedRegistrationV2) manifestCommitment() []byte {
	return r.journal[registrationV2ManifestOffset : registrationV2ManifestOffset+32]
}

// inventoryCommitment is the version 3 field over the bundle's CIDs, which the
// pool record republishes so a buyer can check a candidate set against it.
func (r sealedRegistrationV2) inventoryCommitment() []byte {
	return r.journal[registrationV2InventoryOffset : registrationV2InventoryOffset+32]
}

// bindSessionAnchors answers `block_header(height)` with the node's own header
// shape for both heights the journal commits, so the contract's anchor check
// sees the state root the receipt folded its evidence to. Without this the
// escrow refuses the receipt - which is the check's whole point.
func (r sealedRegistrationV2) bindSessionAnchors(t *testing.T, env *MinimalEnv) {
	t.Helper()
	for _, anchor := range []struct {
		heightOffset int
		rootOffset   int
		name         string
	}{
		{registrationV2InventoryHeight, registrationV2InventoryRoot, "inventory"},
		{registrationV2VerdictHeight, registrationV2VerdictRoot, "verdict"},
	} {
		height := binary.BigEndian.Uint64(r.journal[anchor.heightOffset : anchor.heightOffset+8])
		root := r.journal[anchor.rootOffset : anchor.rootOffset+32]
		if height == 0 {
			t.Fatalf("the receipt's %s anchor names height 0", anchor.name)
		}
		env.SetBlockHeader(height, proposedHeaderWire(t, height, root))
	}
}

// proposedHeaderWire builds the node's own `BlockHeader` message in its
// proposed shape: field 1 carries the proposed header, whose field 2 is the
// height and whose field 6 is the state root (`SOURCES.md` F-96). `idena-go`
// answers `block_header(height)` with exactly this encoding
// (`blockchain/types/types.go` `Header.ToBytes`).
func proposedHeaderWire(t *testing.T, height uint64, root []byte) []byte {
	t.Helper()
	if len(root) != 32 {
		t.Fatalf("a state root is %d bytes, expected 32", len(root))
	}
	inner := appendUvarint([]byte{0x10}, height) // field 2: height
	inner = append(inner, 0x32, 32)              // field 6: state root, length-delimited
	inner = append(inner, root...)
	out := appendUvarint([]byte{0x0a}, uint64(len(inner))) // field 1: proposed header
	return append(out, inner...)
}

// appendUvarint appends one protobuf varint field value.
func appendUvarint(out []byte, value uint64) []byte {
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}
