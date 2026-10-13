package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// These are the public witness bounds in proofs/fast-session-core/src/lib.rs.
const (
	fastRegistrationWireVersion = 1
	// The contract encodes the witness length in u16, so 65,535 is the
	// effective limit even though the Rust parser accepts 64 KiB.
	fastRegistrationMaxWireBytes  = 64*1024 - 1
	fastRegistrationMaxFlips      = 32
	fastRegistrationMaxCIDBytes   = 128
	fastRegistrationMaxValueBytes = 16 * 1024
	fastRegistrationMaxPathItems  = 64
)

type fastRegistrationWireWriter struct {
	data []byte
	err  error
}

func (w *fastRegistrationWireWriter) write(part []byte) {
	if w.err != nil {
		return
	}
	if len(part) > fastRegistrationMaxWireBytes-len(w.data) {
		w.err = fmt.Errorf("fast registration witness exceeds %d bytes", fastRegistrationMaxWireBytes)
		return
	}
	w.data = append(w.data, part...)
}

func (w *fastRegistrationWireWriter) writeByte(value byte) {
	w.write([]byte{value})
}

func (w *fastRegistrationWireWriter) writeU16(value uint16) {
	var encoded [2]byte
	binary.BigEndian.PutUint16(encoded[:], value)
	w.write(encoded[:])
}

func (w *fastRegistrationWireWriter) writeU64(value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	w.write(encoded[:])
}

func (w *fastRegistrationWireWriter) writeI64(value int64) {
	w.writeU64(uint64(value))
}

// fastRegistrationHex bounds the text before decoding it, including when a
// caller has changed an otherwise verified bundle in memory.
func fastRegistrationHex(value string, minBytes, maxBytes int) ([]byte, error) {
	if len(value)%2 != 0 || len(value)/2 < minBytes || len(value)/2 > maxBytes {
		return nil, fmt.Errorf("expected %d to %d hex bytes", minBytes, maxBytes)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}
	return decoded, nil
}

func writeFastRegistrationLeaf(w *fastRegistrationWireWriter, name string, leaf ProofBundleEntry, expectedKey []byte) error {
	key, err := fastRegistrationHex(leaf.KeyHex, 1, 21)
	if err != nil || !bytes.Equal(key, expectedKey) {
		return fmt.Errorf("%s has an invalid state key", name)
	}
	value, err := fastRegistrationHex(leaf.ValueHex, 1, fastRegistrationMaxValueBytes)
	if err != nil {
		return fmt.Errorf("%s has an invalid state value: %w", name, err)
	}
	if leaf.Version <= 0 || len(leaf.Path) == 0 || len(leaf.Path) > fastRegistrationMaxPathItems {
		return fmt.Errorf("%s has an invalid leaf version or path count", name)
	}
	w.writeByte(byte(len(key)))
	w.write(key)
	w.writeU16(uint16(len(value)))
	w.write(value)
	w.writeI64(leaf.Version)
	w.writeByte(byte(len(leaf.Path)))
	for index, item := range leaf.Path {
		if item.Height < 0 || item.Height > 127 || item.Size < 0 || item.Version <= 0 {
			return fmt.Errorf("%s path item %d has invalid IAVL metadata", name, index)
		}
		left, err := fastRegistrationHex(item.LeftHex, 0, 32)
		if err != nil || len(left) != 0 && len(left) != 32 {
			return fmt.Errorf("%s path item %d has an invalid left hash", name, index)
		}
		right, err := fastRegistrationHex(item.RightHex, 0, 32)
		if err != nil || len(right) != 0 && len(right) != 32 || (len(left) == 0) == (len(right) == 0) {
			return fmt.Errorf("%s path item %d has an invalid right hash", name, index)
		}
		w.writeI64(item.Height)
		w.writeI64(item.Size)
		w.writeI64(item.Version)
		w.writeByte(byte(len(left)))
		w.write(left)
		w.writeByte(byte(len(right)))
		w.write(right)
	}
	return nil
}

func writeFastRegistrationAnchor(w *fastRegistrationWireWriter, name string, anchor SessionAnchor, identityKey []byte) error {
	root, err := fastRegistrationHex(anchor.RootHex, 32, 32)
	if err != nil {
		return fmt.Errorf("%s has an invalid state root: %w", name, err)
	}
	w.writeU64(anchor.Height)
	w.write(root)
	if err := writeFastRegistrationLeaf(w, name+" global", anchor.Global, []byte{0x03}); err != nil {
		return err
	}
	return writeFastRegistrationLeaf(w, name+" identity", anchor.Identity, identityKey)
}

// encodeFastRegistrationWire packs an already verified SessionBundle into the
// WIRE_VERSION=1 public four-leaf witness. It preserves the manifest order and
// the exact state bytes; the verifier recomputes each value hash from the value.
// It performs no RPC, signing, or private-witness work.
func encodeFastRegistrationWire(bundle *SessionBundle) ([]byte, error) {
	if bundle == nil || bundle.Format != sessionBundleFormat || bundle.Version != sessionBundleVersion {
		return nil, fmt.Errorf("invalid session bundle format or version")
	}
	author, err := fastRegistrationHex(bundle.Author, 20, 20)
	if err != nil {
		return nil, fmt.Errorf("invalid session author: %w", err)
	}
	if len(bundle.Manifest) == 0 || len(bundle.Manifest) > fastRegistrationMaxFlips {
		return nil, fmt.Errorf("session manifest must contain 1 to %d CIDs", fastRegistrationMaxFlips)
	}
	if bundle.Inventory.Height == 0 || bundle.Verdict.Height <= bundle.Inventory.Height {
		return nil, fmt.Errorf("invalid session anchor heights")
	}
	var identityKey [21]byte
	identityKey[0] = 0x02
	copy(identityKey[1:], author)

	w := fastRegistrationWireWriter{data: make([]byte, 0, 1024)}
	w.writeByte(fastRegistrationWireVersion)
	w.write(author)
	w.writeU16(bundle.Epoch)
	w.writeByte(byte(len(bundle.Manifest)))
	seen := make(map[string]struct{}, len(bundle.Manifest))
	for index, cid := range bundle.Manifest {
		if len(cid) == 0 || len(cid) > fastRegistrationMaxCIDBytes {
			return nil, fmt.Errorf("manifest CID %d has invalid length", index)
		}
		if _, err := cidBytesFromString(cid); err != nil {
			return nil, fmt.Errorf("manifest CID %d is not canonical: %w", index, err)
		}
		if _, exists := seen[cid]; exists {
			return nil, fmt.Errorf("manifest repeats CID %d", index)
		}
		seen[cid] = struct{}{}
		w.writeByte(byte(len(cid)))
		w.write([]byte(cid))
	}
	if err := writeFastRegistrationAnchor(&w, "inventory", bundle.Inventory, identityKey[:]); err != nil {
		return nil, err
	}
	if err := writeFastRegistrationAnchor(&w, "verdict", bundle.Verdict, identityKey[:]); err != nil {
		return nil, err
	}
	if w.err != nil {
		return nil, w.err
	}
	return w.data, nil
}
