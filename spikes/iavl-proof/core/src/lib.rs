//! IAVL single-leaf range-proof verification, ported from the pinned fork.
//!
//! The node pins `github.com/idena-network/iavl` v0.12.3 (`SOURCES.md` F-95),
//! whose `ProofLeafNode.Hash` and `ProofInnerNode.Hash` are what the Go verifier
//! folds. This module reproduces that fold so a guest can check an identity
//! proof against the root a header commits.
//!
//! Preimage rules, from the fork's `proof.go` and `encoding.go`:
//!
//! - signed fields use Go's `binary.PutVarint`, which is zigzag,
//! - byte strings are `uvarint(len)` followed by the bytes,
//! - a leaf hashes `varint(0)`, `varint(1)`, `varint(version)`, key, value hash,
//! - an inner node hashes `varint(height)`, `varint(size)`, `varint(version)`,
//!   then the left and right child hashes, and the child given on the path
//!   replaces whichever sibling hash the node does not carry.

#![no_std]

use sha2::{Digest, Sha256};

/// One inner node on the path from the root to the leaf, as `ProofInnerNode`
/// carries it: the sibling hash and the node's own bookkeeping fields.
pub struct PathItem<'a> {
    pub height: i64,
    pub size: i64,
    pub version: i64,
    pub left: &'a [u8],
    pub right: &'a [u8],
}

/// Three varints, two length prefixes, and two hashes stay far below this bound.
const BUF_BYTES: usize = 512;

struct Writer<'a> {
    buf: &'a mut [u8],
    len: usize,
}

impl<'a> Writer<'a> {
    fn new(buf: &'a mut [u8]) -> Self {
        Self { buf, len: 0 }
    }

    fn uvarint(&mut self, mut v: u64) {
        loop {
            let byte = (v & 0x7f) as u8;
            v >>= 7;
            if v == 0 {
                self.push(byte);
                return;
            }
            self.push(byte | 0x80);
        }
    }

    fn varint(&mut self, v: i64) {
        self.uvarint(((v << 1) ^ (v >> 63)) as u64)
    }

    fn bytes(&mut self, b: &[u8]) {
        self.uvarint(b.len() as u64);
        self.push_slice(b);
    }

    fn push(&mut self, byte: u8) {
        self.buf[self.len] = byte;
        self.len += 1;
    }

    fn push_slice(&mut self, b: &[u8]) {
        self.buf[self.len..self.len + b.len()].copy_from_slice(b);
        self.len += b.len();
    }
}

pub fn sha256(bytes: &[u8]) -> [u8; 32] {
    let digest = Sha256::digest(bytes);
    let mut out = [0u8; 32];
    out.copy_from_slice(&digest);
    out
}

/// The hash the fold uses. The native build takes the `sha2` crate; a guest
/// passes the zkVM's accelerated implementation instead, because software
/// SHA-256 dominated the first cycle measurement.
pub trait Hash256 {
    fn hash(bytes: &[u8]) -> [u8; 32];
}

pub struct Sha2Hash;

impl Hash256 for Sha2Hash {
    fn hash(bytes: &[u8]) -> [u8; 32] {
        sha256(bytes)
    }
}

pub fn leaf_hash_with<H: Hash256>(version: i64, key: &[u8], value_hash: &[u8]) -> [u8; 32] {
    let mut buf = [0u8; BUF_BYTES];
    let len = {
        let mut writer = Writer::new(&mut buf);
        writer.varint(0);
        writer.varint(1);
        writer.varint(version);
        writer.bytes(key);
        writer.bytes(value_hash);
        writer.len
    };
    H::hash(&buf[..len])
}

pub fn inner_hash_with<H: Hash256>(item: &PathItem, child: &[u8; 32]) -> [u8; 32] {
    let mut buf = [0u8; BUF_BYTES];
    let len = {
        let mut writer = Writer::new(&mut buf);
        writer.varint(item.height);
        writer.varint(item.size);
        writer.varint(item.version);
        if item.left.is_empty() {
            writer.bytes(child);
            writer.bytes(item.right);
        } else {
            writer.bytes(item.left);
            writer.bytes(child);
        }
        writer.len
    };
    H::hash(&buf[..len])
}

/// Folds a single-leaf proof to its root. The path is ordered with the node
/// closest to the root last, exactly as the fork's `PathToLeaf` documents.
pub fn fold_with<H: Hash256>(version: i64, key: &[u8], value_hash: &[u8], path: &[PathItem]) -> [u8; 32] {
    let mut hash = leaf_hash_with::<H>(version, key, value_hash);
    for item in path.iter().rev() {
        hash = inner_hash_with::<H>(item, &hash);
    }
    hash
}

/// Verifies that `value` sits under `key` at `version` in the tree whose root is
/// `root`, binding the value itself through the leaf hash.
pub fn verify_value_with<H: Hash256>(
    root: &[u8],
    version: i64,
    key: &[u8],
    value: &[u8],
    path: &[PathItem],
) -> bool {
    fold_with::<H>(version, key, &H::hash(value), path).as_slice() == root
}

/// The native entry point: the same fold over the `sha2` crate.
pub fn verify_value(root: &[u8], version: i64, key: &[u8], value: &[u8], path: &[PathItem]) -> bool {
    verify_value_with::<Sha2Hash>(root, version, key, value, path)
}

#[cfg(test)]
mod tests {
    extern crate std;

    use super::*;
    use std::vec::Vec;

    struct FixturePath {
        height: i64,
        size: i64,
        version: i64,
        left: Vec<u8>,
        right: Vec<u8>,
    }

    struct FixtureProof {
        key: Vec<u8>,
        value: Vec<u8>,
        value_hash: Vec<u8>,
        version: i64,
        path: Vec<FixturePath>,
    }

    fn parse_fixture(raw: &str) -> (Vec<u8>, Vec<FixtureProof>) {
        let json: serde_json::Value = serde_json::from_str(raw).expect("fixture json");
        let root = hex::decode(json["rootHex"].as_str().expect("root")).expect("root hex");
        let proofs = json["proofs"]
            .as_array()
            .expect("proofs")
            .iter()
            .map(|proof| FixtureProof {
                key: hex::decode(proof["keyHex"].as_str().expect("key")).expect("key hex"),
                value: hex::decode(proof["valueHex"].as_str().expect("value")).expect("value hex"),
                value_hash: hex::decode(proof["valueHashHex"].as_str().expect("value hash"))
                    .expect("value hash hex"),
                version: proof["version"].as_i64().expect("version"),
                path: proof["path"]
                    .as_array()
                    .expect("path")
                    .iter()
                    .map(|item| FixturePath {
                        height: item["height"].as_i64().expect("height"),
                        size: item["size"].as_i64().expect("size"),
                        version: item["version"].as_i64().expect("version"),
                        left: hex::decode(item["leftHex"].as_str().expect("left")).expect("left hex"),
                        right: hex::decode(item["rightHex"].as_str().expect("right"))
                            .expect("right hex"),
                    })
                    .collect(),
            })
            .collect();
        (root, proofs)
    }

    fn fixture() -> (Vec<u8>, Vec<FixtureProof>) {
        parse_fixture(include_str!("../fixtures/bundle.json"))
    }

    #[test]
    fn every_real_proof_folds_to_the_root() {
        let (root, proofs) = fixture();
        assert!(!proofs.is_empty());
        for proof in &proofs {
            assert_eq!(
                sha256(&proof.value).as_slice(),
                proof.value_hash.as_slice(),
                "the leaf's value hash is the hash of the value"
            );
            let path: Vec<PathItem> = proof
                .path
                .iter()
                .map(|item| PathItem {
                    height: item.height,
                    size: item.size,
                    version: item.version,
                    left: &item.left,
                    right: &item.right,
                })
                .collect();
            assert!(
                verify_value(&root, proof.version, &proof.key, &proof.value, &path),
                "a real proof did not fold to its version's root"
            );
        }
    }

    #[test]
    fn a_changed_value_or_path_is_refused() {
        let (root, mut proofs) = fixture();
        let proof = &mut proofs[0];
        let path: Vec<PathItem> = proof
            .path
            .iter()
            .map(|item| PathItem {
                height: item.height,
                size: item.size,
                version: item.version,
                left: &item.left,
                right: &item.right,
            })
            .collect();
        let mut changed = proof.value.clone();
        changed[0] ^= 0x01;
        assert!(!verify_value(&root, proof.version, &proof.key, &changed, &path));

        let mut broken = proof.path[0].right.clone();
        if broken.is_empty() {
            broken = proof.path[0].left.clone();
        }
        broken[0] ^= 0x01;
        let mut items: Vec<PathItem> = proof
            .path
            .iter()
            .map(|item| PathItem {
                height: item.height,
                size: item.size,
                version: item.version,
                left: &item.left,
                right: &item.right,
            })
            .collect();
        if items[0].right.is_empty() {
            items[0].left = &broken;
        } else {
            items[0].right = &broken;
        }
        assert!(!verify_value(&root, proof.version, &proof.key, &proof.value, &items));
    }
}
