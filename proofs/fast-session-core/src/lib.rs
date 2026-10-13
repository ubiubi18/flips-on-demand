//! Bounded public-session witness for direct Idena contract verification.

#![no_std]

extern crate alloc;

use alloc::vec::Vec;
use fod_session_core::iavl::PathItem;
use fod_session_core::session::{self, Anchor, Bundle, Leaf, SessionVerdict};
use fod_session_core::statement;

pub const WIRE_VERSION: u8 = 1;
pub const MAX_WIRE_BYTES: usize = 64 * 1024;
const MAX_FLIPS: usize = 32;
const MAX_CID_BYTES: usize = 128;
const MAX_VALUE_BYTES: usize = 16 * 1024;
const MAX_PATH_ITEMS: usize = 64;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Refusal {
    Malformed,
    TooLarge,
    WrongAuthor,
    WrongEpoch,
    WrongRoot,
    WrongInventory,
    WrongCount,
    InvalidSession,
    InvalidFlags,
}

struct Cursor<'a> {
    bytes: &'a [u8],
    at: usize,
}

impl<'a> Cursor<'a> {
    fn new(bytes: &'a [u8]) -> Self {
        Self { bytes, at: 0 }
    }

    fn take(&mut self, len: usize) -> Result<&'a [u8], Refusal> {
        let end = self.at.checked_add(len).ok_or(Refusal::Malformed)?;
        let part = self.bytes.get(self.at..end).ok_or(Refusal::Malformed)?;
        self.at = end;
        Ok(part)
    }

    fn u8(&mut self) -> Result<u8, Refusal> {
        Ok(self.take(1)?[0])
    }

    fn u16(&mut self) -> Result<u16, Refusal> {
        Ok(u16::from_be_bytes(
            self.take(2)?.try_into().map_err(|_| Refusal::Malformed)?,
        ))
    }

    fn u64(&mut self) -> Result<u64, Refusal> {
        Ok(u64::from_be_bytes(
            self.take(8)?.try_into().map_err(|_| Refusal::Malformed)?,
        ))
    }

    fn i64(&mut self) -> Result<i64, Refusal> {
        Ok(i64::from_be_bytes(
            self.take(8)?.try_into().map_err(|_| Refusal::Malformed)?,
        ))
    }

    fn leaf(&mut self) -> Result<ParsedLeaf<'a>, Refusal> {
        let key_len = self.u8()? as usize;
        if key_len == 0 || key_len > 21 {
            return Err(Refusal::Malformed);
        }
        let key = self.take(key_len)?;
        let value_len = self.u16()? as usize;
        if value_len > MAX_VALUE_BYTES {
            return Err(Refusal::TooLarge);
        }
        let value = self.take(value_len)?;
        let version = self.i64()?;
        let count = self.u8()? as usize;
        if count > MAX_PATH_ITEMS {
            return Err(Refusal::TooLarge);
        }
        let mut path = Vec::with_capacity(count);
        for _ in 0..count {
            let height = self.i64()?;
            let size = self.i64()?;
            let version = self.i64()?;
            let left_len = self.u8()? as usize;
            if left_len != 0 && left_len != 32 {
                return Err(Refusal::Malformed);
            }
            let left = self.take(left_len)?;
            let right_len = self.u8()? as usize;
            if right_len != 0 && right_len != 32 || (left_len == 0) == (right_len == 0) {
                return Err(Refusal::Malformed);
            }
            let right = self.take(right_len)?;
            path.push(PathItem {
                height,
                size,
                version,
                left,
                right,
            });
        }
        Ok(ParsedLeaf {
            key,
            value,
            version,
            path,
        })
    }

    fn anchor(&mut self) -> Result<ParsedAnchor<'a>, Refusal> {
        Ok(ParsedAnchor {
            height: self.u64()?,
            root: self.take(32)?,
            global: self.leaf()?,
            identity: self.leaf()?,
        })
    }
}

struct ParsedLeaf<'a> {
    key: &'a [u8],
    value: &'a [u8],
    version: i64,
    path: Vec<PathItem<'a>>,
}

impl<'a> ParsedLeaf<'a> {
    fn leaf(&self) -> Leaf<'_> {
        Leaf {
            key: self.key,
            value: self.value,
            version: self.version,
            path: &self.path,
        }
    }
}

struct ParsedAnchor<'a> {
    height: u64,
    root: &'a [u8],
    global: ParsedLeaf<'a>,
    identity: ParsedLeaf<'a>,
}

impl<'a> ParsedAnchor<'a> {
    fn anchor(&self) -> Anchor<'_> {
        Anchor {
            height: self.height,
            root: self.root,
            global: self.global.leaf(),
            identity: self.identity.leaf(),
        }
    }
}

/// `root_at` must read a canonical header from the executing Idena node. It
/// must never return a root supplied by the registration caller.
pub fn verify_with_roots<F>(
    wire: &[u8],
    expected_author: &[u8; 20],
    expected_epoch: u16,
    expected_count: u16,
    expected_inventory: &[u8; 32],
    root_at: F,
) -> Result<SessionVerdict, Refusal>
where
    F: FnMut(u64) -> Option<[u8; 32]>,
{
    verify_with_roots_and_cids(
        wire,
        expected_author,
        expected_epoch,
        expected_count,
        expected_inventory,
        root_at,
    )
    .map(|(verdict, _)| verdict)
}

/// Returns the original public CIDs only after their session roots and
/// inventory commitment have been verified.
pub fn verify_with_roots_and_cids<'a, F>(
    wire: &'a [u8],
    expected_author: &[u8; 20],
    expected_epoch: u16,
    expected_count: u16,
    expected_inventory: &[u8; 32],
    mut root_at: F,
) -> Result<(SessionVerdict, Vec<&'a [u8]>), Refusal>
where
    F: FnMut(u64) -> Option<[u8; 32]>,
{
    if wire.len() > MAX_WIRE_BYTES {
        return Err(Refusal::TooLarge);
    }
    let mut cursor = Cursor::new(wire);
    if cursor.u8()? != WIRE_VERSION {
        return Err(Refusal::Malformed);
    }
    let author = cursor.take(20)?;
    if author != expected_author {
        return Err(Refusal::WrongAuthor);
    }
    let epoch = cursor.u16()?;
    if epoch != expected_epoch {
        return Err(Refusal::WrongEpoch);
    }
    let count = cursor.u8()? as usize;
    if count == 0 || count > MAX_FLIPS {
        return Err(Refusal::Malformed);
    }
    if count != usize::from(expected_count) {
        return Err(Refusal::WrongCount);
    }
    let mut cids = Vec::with_capacity(count);
    for _ in 0..count {
        let len = cursor.u8()? as usize;
        if len == 0 || len > MAX_CID_BYTES {
            return Err(Refusal::Malformed);
        }
        cids.push(cursor.take(len)?);
    }
    let inventory = cursor.anchor()?;
    let verdict = cursor.anchor()?;
    if cursor.at != wire.len() {
        return Err(Refusal::Malformed);
    }
    let inventory_root = root_at(inventory.height).ok_or(Refusal::WrongRoot)?;
    let verdict_root = root_at(verdict.height).ok_or(Refusal::WrongRoot)?;
    if inventory_root.as_slice() != inventory.root || verdict_root.as_slice() != verdict.root {
        return Err(Refusal::WrongRoot);
    }
    let bundle = Bundle {
        author,
        epoch: u32::from(epoch),
        manifest: &cids,
        inventory: inventory.anchor(),
        verdict: verdict.anchor(),
    };
    let result = session::verify(&bundle).map_err(|_| Refusal::InvalidSession)?;
    if result.flags & !0b111 != 0 {
        return Err(Refusal::InvalidFlags);
    }
    let mut sorted = cids.clone();
    sorted.sort();
    if sorted.windows(2).any(|pair| pair[0] == pair[1]) {
        return Err(Refusal::WrongInventory);
    }
    if statement::inventory_commitment(expected_author, epoch, &sorted) != *expected_inventory {
        return Err(Refusal::WrongInventory);
    }
    Ok((result, cids))
}

#[cfg(test)]
mod tests {
    extern crate std;

    use super::*;
    use serde_json::Value;
    use std::vec::Vec;

    fn hex(text: &str) -> Vec<u8> {
        text.as_bytes()
            .chunks_exact(2)
            .map(|part| u8::from_str_radix(std::str::from_utf8(part).unwrap(), 16).unwrap())
            .collect()
    }

    fn leaf(out: &mut Vec<u8>, value: &Value) {
        let key = hex(value["keyHex"].as_str().unwrap());
        let body = hex(value["valueHex"].as_str().unwrap());
        out.push(key.len() as u8);
        out.extend_from_slice(&key);
        out.extend_from_slice(&(body.len() as u16).to_be_bytes());
        out.extend_from_slice(&body);
        out.extend_from_slice(&value["version"].as_i64().unwrap().to_be_bytes());
        let path = value["path"].as_array().unwrap();
        out.push(path.len() as u8);
        for item in path {
            for name in ["height", "size", "version"] {
                out.extend_from_slice(&item[name].as_i64().unwrap().to_be_bytes());
            }
            for name in ["leftHex", "rightHex"] {
                let side = hex(item[name].as_str().unwrap());
                out.push(side.len() as u8);
                out.extend_from_slice(&side);
            }
        }
    }

    fn anchor(out: &mut Vec<u8>, value: &Value) {
        out.extend_from_slice(&value["height"].as_u64().unwrap().to_be_bytes());
        out.extend_from_slice(&hex(value["rootHex"].as_str().unwrap()));
        leaf(out, &value["global"]);
        leaf(out, &value["identity"]);
    }

    struct Fixture {
        wire: Vec<u8>,
        author: [u8; 20],
        original_inventory: [u8; 32],
        inventory_height: u64,
        inventory_root: [u8; 32],
        verdict_height: u64,
        verdict_root: [u8; 32],
    }

    impl Fixture {
        fn new() -> Self {
            let path = concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../session-core/fixtures/session-bundle-vector.json"
            );
            let doc: Value = serde_json::from_str(&std::fs::read_to_string(path).unwrap()).unwrap();
            let session = &doc["sessions"][0];
            let author: [u8; 20] = hex(doc["author"].as_str().unwrap()).try_into().unwrap();
            let epoch = session["epoch"].as_u64().unwrap() as u16;
            let cids: Vec<Vec<u8>> = doc["manifest"]
                .as_array()
                .unwrap()
                .iter()
                .map(|cid| cid.as_str().unwrap().as_bytes().to_vec())
                .collect();
            let mut wire = Vec::new();
            wire.push(WIRE_VERSION);
            wire.extend_from_slice(&author);
            wire.extend_from_slice(&epoch.to_be_bytes());
            wire.push(cids.len() as u8);
            for cid in &cids {
                wire.push(cid.len() as u8);
                wire.extend_from_slice(cid);
            }
            anchor(&mut wire, &session["inventory"]);
            anchor(&mut wire, &session["verdict"]);
            let mut sorted: Vec<&[u8]> = cids.iter().map(Vec::as_slice).collect();
            sorted.sort();
            Self {
                wire,
                author,
                original_inventory: statement::inventory_commitment(&author, epoch, &sorted),
                inventory_height: session["inventory"]["height"].as_u64().unwrap(),
                inventory_root: hex(session["inventory"]["rootHex"].as_str().unwrap())
                    .try_into()
                    .unwrap(),
                verdict_height: session["verdict"]["height"].as_u64().unwrap(),
                verdict_root: hex(session["verdict"]["rootHex"].as_str().unwrap())
                    .try_into()
                    .unwrap(),
            }
        }

        fn root_at(&self, height: u64) -> Option<[u8; 32]> {
            if height == self.inventory_height {
                Some(self.inventory_root)
            } else if height == self.verdict_height {
                Some(self.verdict_root)
            } else {
                None
            }
        }

        fn check(&self, wire: &[u8], inventory: &[u8; 32]) -> Result<SessionVerdict, Refusal> {
            verify_with_roots(wire, &self.author, 228, 1, inventory, |height| {
                self.root_at(height)
            })
        }
    }

    #[test]
    fn exact_public_evidence_and_canonical_headers_are_required() {
        let fixture = Fixture::new();
        let (_, cids) = verify_with_roots_and_cids(
            &fixture.wire,
            &fixture.author,
            228,
            1,
            &fixture.original_inventory,
            |height| fixture.root_at(height),
        )
        .unwrap();
        assert_eq!(cids.len(), 1);
        assert_eq!(cids[0], &fixture.wire[25..25 + fixture.wire[24] as usize]);
        assert_eq!(
            fixture
                .check(&fixture.wire, &fixture.original_inventory)
                .unwrap()
                .flags,
            0
        );
        assert_eq!(
            fixture.check(&fixture.wire, &[0; 32]),
            Err(Refusal::WrongInventory)
        );
        assert_eq!(
            verify_with_roots(
                &fixture.wire,
                &fixture.author,
                228,
                1,
                &fixture.original_inventory,
                |_| Some([0; 32])
            ),
            Err(Refusal::WrongRoot)
        );
        let mut changed = fixture.wire.clone();
        changed[0] ^= 1;
        assert_eq!(
            fixture.check(&changed, &fixture.original_inventory),
            Err(Refusal::Malformed)
        );
        changed = fixture.wire.clone();
        changed[25] ^= 1; // One character of the listed CID.
        assert_eq!(
            fixture.check(&changed, &fixture.original_inventory),
            Err(Refusal::InvalidSession)
        );
        changed = fixture.wire.clone();
        changed.push(0);
        assert_eq!(
            fixture.check(&changed, &fixture.original_inventory),
            Err(Refusal::Malformed)
        );
    }
}
