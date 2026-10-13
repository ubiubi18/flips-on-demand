//! IAVL proof guest for the spike: fold every identity proof it is handed to the
//! root it is handed, and commit how many of them landed on that root.
//!
//! The journal is root (32) || count (4, big-endian) || verified (4,
//! big-endian), so a reader can see both the claim and the verdict.
//!
//! The whole set arrives as one packed buffer rather than as one `env::read` per
//! field, because the read overhead dominated the first measurement (about
//! 133,000 cycles per proof). Little-endian fields, `u32` lengths, no padding.

use fod_iavl_proof_core::{verify_value_with, Hash256, PathItem};
use risc0_zkvm::guest::env;
use risc0_zkvm::sha::Sha256 as _;

/// The zkVM's accelerated SHA-256. The first measurement showed the fold is
/// hash-bound, and the plain `sha2` crate does not use the accelerator.
struct AcceleratedHash;

impl Hash256 for AcceleratedHash {
    fn hash(bytes: &[u8]) -> [u8; 32] {
        let digest = risc0_zkvm::sha::Impl::hash_bytes(bytes);
        let mut out = [0u8; 32];
        out.copy_from_slice(digest.as_bytes());
        out
    }
}

struct Cursor<'a> {
    data: &'a [u8],
    at: usize,
}

impl<'a> Cursor<'a> {
    fn take(&mut self, n: usize) -> Option<&'a [u8]> {
        let end = self.at.checked_add(n)?;
        let slice = self.data.get(self.at..end)?;
        self.at = end;
        Some(slice)
    }

    fn u32(&mut self) -> Option<u32> {
        Some(u32::from_le_bytes(self.take(4)?.try_into().ok()?))
    }

    fn i64(&mut self) -> Option<i64> {
        Some(i64::from_le_bytes(self.take(8)?.try_into().ok()?))
    }

    fn bytes(&mut self) -> Option<&'a [u8]> {
        let n = self.u32()? as usize;
        self.take(n)
    }
}

fn main() {
    // The length comes through the serde path and the body through `read_slice`,
    // because reading the blob value by value cost about 88,000 cycles per proof.
    let len: u32 = env::read();
    let mut blob = vec![0u8; len as usize];
    env::read_slice(&mut blob);
    let mut cursor = Cursor { data: &blob, at: 0 };
    let root = cursor.take(32).expect("root");
    let count = cursor.u32().expect("count");

    let mut verified: u32 = 0;
    let mut items: Vec<(i64, i64, i64, &[u8], &[u8])> = Vec::new();
    for _ in 0..count {
        let version = cursor.i64().expect("version");
        let key = cursor.bytes().expect("key");
        let value = cursor.bytes().expect("value");
        let path_len = cursor.u32().expect("path length");
        items.clear();
        for _ in 0..path_len {
            let height = cursor.i64().expect("height");
            let size = cursor.i64().expect("size");
            let item_version = cursor.i64().expect("item version");
            let left = cursor.bytes().expect("left");
            let right = cursor.bytes().expect("right");
            items.push((height, size, item_version, left, right));
        }
        let path: Vec<PathItem> = items
            .iter()
            .map(|(height, size, version, left, right)| PathItem {
                height: *height,
                size: *size,
                version: *version,
                left,
                right,
            })
            .collect();
        if verify_value_with::<AcceleratedHash>(&root, version, &key, &value, &path) {
            verified += 1;
        }
    }

    let mut journal = Vec::with_capacity(40);
    journal.extend_from_slice(&root);
    journal.extend_from_slice(&count.to_be_bytes());
    journal.extend_from_slice(&verified.to_be_bytes());
    env::commit_slice(&journal);
}
