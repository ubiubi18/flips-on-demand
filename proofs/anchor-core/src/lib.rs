//! The identity half of the session binding: fold a bundle of tree proofs to the
//! anchor root the claim commits.
//!
//! A seller attaches the records of the shard's candidates with the membership
//! proofs that place each of them under the epoch's state root (`SOURCES.md`
//! F-93, F-95; the fold is the pinned fork's, ported in
//! `spikes/iavl-proof/core`). The guest folds every proof to the anchor it was
//! given and refuses the input when any of them misses, so the anchor is a root
//! the records were proven against rather than a number the seller typed.
//!
//! Preimage rules, from the fork's `proof.go` and `encoding.go`: signed fields
//! are Go's zigzag varints, byte strings are `uvarint(len)` then the bytes, a
//! leaf hashes `varint(0)`, `varint(1)`, `varint(version)`, key, and value hash,
//! and an inner node hashes its height, size, version, and two child hashes with
//! the proven child replacing the sibling the node does not carry.

#![no_std]

extern crate alloc;

use alloc::vec::Vec;
use sha2::{Digest, Sha256};

/// One inner node on the path from a leaf to the root.
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

fn sha256(bytes: &[u8]) -> [u8; 32] {
    let digest = Sha256::digest(bytes);
    let mut out = [0u8; 32];
    out.copy_from_slice(&digest);
    out
}

pub fn leaf_hash(version: i64, key: &[u8], value_hash: &[u8]) -> [u8; 32] {
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
    sha256(&buf[..len])
}

pub fn inner_hash(item: &PathItem, child: &[u8; 32]) -> [u8; 32] {
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
    sha256(&buf[..len])
}

/// Folds one record under `key` to its root. `value_hash` is the hash the leaf
/// carries; the caller checks it against the record it uses.
pub fn fold(version: i64, key: &[u8], value_hash: &[u8], path: &[PathItem]) -> [u8; 32] {
    let mut hash = leaf_hash(version, key, value_hash);
    for item in path.iter().rev() {
        hash = inner_hash(item, &hash);
    }
    hash
}

/// One record and the path that places it in the tree.
pub struct Record<'a> {
    pub version: i64,
    pub key: &'a [u8],
    pub value_hash: &'a [u8],
    pub path: Vec<PathItem<'a>>,
}

/// Verifies every record against the anchor and returns the count, or `None`
/// when any record misses the anchor.
pub fn verify_records(anchor: &[u8], records: &[Record]) -> Option<u32> {
    for record in records {
        if fold(record.version, record.key, record.value_hash, &record.path).as_slice() != anchor {
            return None;
        }
    }
    Some(records.len() as u32)
}

/// Where the packed bundle's fields sit: the anchor root, a count, then per
/// record the version, key, value hash, and path items. Little-endian fields,
/// `u32` lengths, the order `tools/fod` writes and the host packs.
pub struct Cursor<'a> {
    data: &'a [u8],
    at: usize,
}

impl<'a> Cursor<'a> {
    pub fn new(data: &'a [u8]) -> Self {
        Self { data, at: 0 }
    }

    pub fn take(&mut self, n: usize) -> Option<&'a [u8]> {
        let end = self.at.checked_add(n)?;
        let slice = self.data.get(self.at..end)?;
        self.at = end;
        Some(slice)
    }

    pub fn u32(&mut self) -> Option<u32> {
        Some(u32::from_le_bytes(self.take(4)?.try_into().ok()?))
    }

    pub fn i64(&mut self) -> Option<i64> {
        Some(i64::from_le_bytes(self.take(8)?.try_into().ok()?))
    }

    pub fn bytes(&mut self) -> Option<&'a [u8]> {
        let n = self.u32()? as usize;
        self.take(n)
    }
}

/// Parses one packed record.
pub fn read_record<'a>(cursor: &mut Cursor<'a>) -> Option<Record<'a>> {
    let version = cursor.i64()?;
    let key = cursor.bytes()?;
    let value_hash = cursor.bytes()?;
    let path_len = cursor.u32()?;
    let mut path = Vec::with_capacity(path_len as usize);
    for _ in 0..path_len {
        let height = cursor.i64()?;
        let size = cursor.i64()?;
        let item_version = cursor.i64()?;
        let left = cursor.bytes()?;
        let right = cursor.bytes()?;
        path.push(PathItem {
            height,
            size,
            version: item_version,
            left,
            right,
        });
    }
    Some(Record {
        version,
        key,
        value_hash,
        path,
    })
}

/// Reads a packed bundle and verifies it against the anchor. An empty bundle is
/// no proof at all and answers `None`.
pub fn verify_bundle(anchor: &[u8], packed: &[u8]) -> Option<u32> {
    if packed.is_empty() {
        return None;
    }
    let mut cursor = Cursor::new(packed);
    let root = cursor.take(32)?;
    if root != anchor {
        return None;
    }
    let count = cursor.u32()?;
    if count == 0 {
        return None;
    }
    let mut records = Vec::with_capacity(count as usize);
    for _ in 0..count {
        records.push(read_record(&mut cursor)?);
    }
    verify_records(anchor, &records)
}

/// A path item with owned bytes, for a host that builds a bundle to pack.
pub struct OwnedPathItem {
    pub height: i64,
    pub size: i64,
    pub version: i64,
    pub left: Vec<u8>,
    pub right: Vec<u8>,
}

/// A record with owned bytes, for a host that builds a bundle to pack.
pub struct OwnedRecord {
    pub version: i64,
    pub key: Vec<u8>,
    pub value_hash: Vec<u8>,
    pub path: Vec<OwnedPathItem>,
}

fn push_bytes(out: &mut Vec<u8>, bytes: &[u8]) {
    out.extend_from_slice(&(bytes.len() as u32).to_le_bytes());
    out.extend_from_slice(bytes);
}

/// Packs a bundle in the layout `verify_bundle` reads. This is the one packer:
/// the host uses it, and the crate's test proves the pack-verify round trip.
pub fn pack_bundle(anchor: &[u8], records: &[OwnedRecord]) -> Vec<u8> {
    let mut out = Vec::new();
    out.extend_from_slice(anchor);
    out.extend_from_slice(&(records.len() as u32).to_le_bytes());
    for record in records {
        out.extend_from_slice(&record.version.to_le_bytes());
        push_bytes(&mut out, &record.key);
        push_bytes(&mut out, &record.value_hash);
        out.extend_from_slice(&(record.path.len() as u32).to_le_bytes());
        for item in &record.path {
            out.extend_from_slice(&item.height.to_le_bytes());
            out.extend_from_slice(&item.size.to_le_bytes());
            out.extend_from_slice(&item.version.to_le_bytes());
            push_bytes(&mut out, &item.left);
            push_bytes(&mut out, &item.right);
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn unhex(text: &str) -> Vec<u8> {
        let mut out = Vec::with_capacity(text.len() / 2);
        let bytes = text.as_bytes();
        let mut index = 0;
        while index + 1 < bytes.len() {
            let pair = alloc::str::from_utf8(&bytes[index..index + 2]).expect("hex pair");
            out.push(u8::from_str_radix(pair, 16).expect("hex"));
            index += 2;
        }
        out
    }

    /// Turns the vector the way `tools/fod` writes it into owned records and
    /// packs them with the one packer the host also uses.
    fn pack(json: &serde_json::Value) -> (Vec<u8>, Vec<u8>) {
        let root = unhex(json["rootHex"].as_str().expect("root"));
        let records: Vec<OwnedRecord> = json["proofs"]
            .as_array()
            .expect("proofs")
            .iter()
            .map(|proof| OwnedRecord {
                version: proof["version"].as_i64().expect("version"),
                key: unhex(proof["keyHex"].as_str().expect("key")),
                value_hash: unhex(proof["valueHashHex"].as_str().expect("value hash")),
                path: proof["path"]
                    .as_array()
                    .expect("path")
                    .iter()
                    .map(|item| OwnedPathItem {
                        height: item["height"].as_i64().expect("height"),
                        size: item["size"].as_i64().expect("size"),
                        version: item["version"].as_i64().expect("version"),
                        left: unhex(item["leftHex"].as_str().expect("left")),
                        right: unhex(item["rightHex"].as_str().expect("right")),
                    })
                    .collect(),
            })
            .collect();
        let packed = pack_bundle(&root, &records);
        (root, packed)
    }

    /// The bundle `tools/fod state-proof -proof-bundle` wrote from a real
    /// datadir, folded natively here. Every record has to land on the anchor.
    #[test]
    fn a_real_bundle_folds_to_its_anchor() {
        let json: serde_json::Value =
            serde_json::from_str(include_str!("../vectors/anchor-v1.json")).expect("vector");
        let proofs = json["proofs"].as_array().expect("proofs").len() as u32;
        let (root, packed) = pack(&json);
        let verified = verify_bundle(&root, &packed).expect("the real bundle did not fold");
        assert_eq!(verified, proofs);

        // A tampered value hash, an empty bundle, and another root all miss.
        let mut broken = packed.clone();
        let value_hash_at = 32 + 4 + 8 + 4 + 21 + 4;
        broken[value_hash_at] ^= 0x01;
        assert!(verify_bundle(&root, &broken).is_none());
        assert!(verify_bundle(&root, &[]).is_none());
        let mut other = root.clone();
        other[0] ^= 0x01;
        assert!(verify_bundle(&other, &packed).is_none());
    }
}
