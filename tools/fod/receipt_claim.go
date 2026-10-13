package main

// The claim the escrow checks a sealed receipt against, reproduced here so a
// receipt that cannot register is refused before it becomes a transaction
// instead of after the node has answered `invalid_proof`.
//
// The construction is RISC Zero's `ReceiptClaim` for a successful run
// (`risc0_binfmt::hash::tagged_struct`), the same one
// `contracts/escrow/src/verifier.rs` computes in the runtime: the tag's own
// digest, the field digests, the data words as little-endian u32s, and the field
// count as a little-endian u16, hashed with SHA-256. Two fixtures pin it from
// opposite ends: the escrow's own delivery test constants, and the proved
// registration artefact this release ships.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// registrationImageIDHex is the program id the escrow pins for this release
// (`contracts/escrow/src/verifier.rs`, `REGISTRATION_IMAGE_ID`, checked against
// this value by `release/build-manifest.mjs` so the three copies cannot drift). A receipt
// proved by any other guest program can never register.
// The registration guest in `proofs/registration-v2`, from
// `proofs/registration-v2/dist/registration-v2.json`. It moved when the guest
// adopted the version 3 statement, which publishes the inventory commitment and
// the network digest, and it moved before that when the guest began checking
// that a bundle's content is a flip (`SOURCES.md` F-98).
const deliveryImageIDHex = "6bcb725eff9008b2baba7aec227bdf0342dd11e7e03a34b26a49f0a72cabd443"

const registrationImageIDHex = "bd4bea61ef9c370c5e4c5a0c51e3af8146d2fea57f5b3c83105896c84d68cc4d"

// The replacement direct escrow uses a separately reproduced guest image.
// Pool registration retains registrationImageIDHex above.
const directV2ImageIDHex = "658db709b6de076b1f41cd14de106be4ef67e32353b3e46b41db689a11970b58"

// claimDigestFromHex is the claim digest of a journal under a program id named
// in hex, in the form both the host and the escrow record.
func claimDigestFromHex(imageIDHex string, journal []byte) ([32]byte, error) {
	var zero [32]byte
	raw, err := hex.DecodeString(strings.TrimPrefix(strings.TrimPrefix(imageIDHex, "0x"), "0X"))
	if err != nil {
		return zero, err
	}
	if len(raw) != 32 {
		return zero, errors.New("the program id is not a 32-byte value")
	}
	var imageID [32]byte
	copy(imageID[:], raw)
	return claimDigest(imageID, journal), nil
}

// claimDigest is the digest of the canonical successful-execution claim for a
// journal, byte for byte what `verifier.rs`'s `claim_digest` computes.
func claimDigest(imageID [32]byte, journal []byte) [32]byte {
	journalDigest := sha256.Sum256(journal)
	// Output { journal: Pruned(sha256(journal)), assumptions: Pruned(ZERO) }
	output := taggedStruct("risc0.Output", [][32]byte{journalDigest, {}}, nil)
	// input: MaybePruned::Value(None) -> Option::None, the zero digest.
	input := [32]byte{}
	// post: SystemState { pc: 0, merkle_root: ZERO }.
	post := taggedStruct("risc0.SystemState", [][32]byte{{}}, []uint32{0})
	// exit_code: Halted(0) -> (system_exit, user_exit) = (0, 0).
	return taggedStruct("risc0.ReceiptClaim", [][32]byte{input, imageID, post, output}, []uint32{0, 0})
}

// taggedStruct mirrors `risc0_binfmt`'s tagged struct: the tag digest, then the
// field digests, then the data words little-endian, then the field count as a
// little-endian u16.
func taggedStruct(tag string, down [][32]byte, data []uint32) [32]byte {
	buf := make([]byte, 0, 32+32*len(down)+4*len(data)+2)
	tagDigest := sha256.Sum256([]byte(tag))
	buf = append(buf, tagDigest[:]...)
	for _, field := range down {
		buf = append(buf, field[:]...)
	}
	for _, word := range data {
		buf = append(buf, byte(word), byte(word>>8), byte(word>>16), byte(word>>24))
	}
	buf = append(buf, byte(len(down)), byte(len(down)>>8))
	return sha256.Sum256(buf)
}
