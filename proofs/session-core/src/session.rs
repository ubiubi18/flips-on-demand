//! The two-anchor session evidence of `SOURCES.md` F-97.
//!
//! A bundle proves two things about one author's epoch, and nothing else:
//!
//! - the *inventory* leaf (the state tree at a height inside the flip phase,
//!   whose global leaf reads `Epoch == N`) lists every flip the author submitted
//!   for that epoch;
//! - the *verdict* leaf (the same author at the epoch's `ValidationFinished`
//!   block, whose global leaf reads `Epoch == N+1`) carries the chain's
//!   validation status flags after the flips were cleared.
//!
//! Both leaves fold to their own header's state root, which the contract
//! authenticates against the node's canonical header (`SOURCES.md` F-96). The
//! chain stores no per-flip verdict, so the provable claim is the bundle label -
//! clean, contains reported, weak - and the raw flags.

use crate::iavl::{verify_value_with, Hash256, PathItem, Sha2Hash};

/// `StateDbKeys.IdentityKey(addr)` is `0x02 || addr` (`core/state/keys.go`).
pub const IDENTITY_KEY_PREFIX: u8 = 0x02;
/// `StateDbKeys.GlobalKey()` (`core/state/keys.go`).
pub const GLOBAL_KEY: &[u8] = &[0x03];
pub const IDENTITY_KEY_BYTES: usize = 21;
pub const AUTHOR_BYTES: usize = 20;

/// Protobuf field numbers from `protobuf/models.pb.go`.
const GLOBAL_EPOCH_FIELD: u32 = 1;
/// `ProtoStateGlobal.validationPeriod` (`protobuf/models.proto:417`).
const GLOBAL_VALIDATION_PERIOD_FIELD: u32 = 3;
const IDENTITY_FLIPS_FIELD: u32 = 9;
const IDENTITY_VALIDATION_STATUS_FIELD: u32 = 16;
const FLIP_CID_FIELD: u32 = 1;

/// `state.FlipLotteryPeriod` (`core/state/state_object.go:148`).
///
/// From this period on, `validateSubmitFlipTx` refuses a flip submission as
/// `LateTx` (`blockchain/validation/validation.go:349`), and within one epoch
/// the period only advances - `ValidationFinished` is the single transition
/// that returns it to `None`, and that block increments the epoch
/// (`blockchain/blockchain.go:1211-1231`). An inventory anchor that reports
/// this period or a later one therefore holds the epoch's final flip set.
pub const FLIP_LOTTERY_PERIOD: u32 = 1;

/// `state.ValidationStatusFlag` (`core/state/state_object.go:436-440`).
pub const FLAG_AT_LEAST_ONE_REPORTED: u32 = 1 << 0;
pub const FLAG_AT_LEAST_ONE_NOT_QUALIFIED: u32 = 1 << 1;
pub const FLAG_ALL_NOT_QUALIFIED: u32 = 1 << 2;

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Label {
    Clean,
    ContainsReported,
    Weak,
}

impl Label {
    pub fn as_str(self) -> &'static str {
        match self {
            Label::Clean => "clean",
            Label::ContainsReported => "contains-reported",
            Label::Weak => "weak",
        }
    }

    /// The label the escrow and the interface derive from the raw flags.
    pub fn for_flags(flags: u32) -> Label {
        if flags & (FLAG_AT_LEAST_ONE_NOT_QUALIFIED | FLAG_ALL_NOT_QUALIFIED) != 0 {
            Label::Weak
        } else if flags & FLAG_AT_LEAST_ONE_REPORTED != 0 {
            Label::ContainsReported
        } else {
            Label::Clean
        }
    }
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Refusal {
    /// The leaf's key is not the author's identity key in either anchor.
    IdentityKey,
    /// A global leaf is not under `StateDbKeys.GlobalKey()`.
    GlobalKey,
    /// A leaf does not fold to the root its anchor names.
    RootMismatch,
    /// The inventory leaf's epoch is not the bundle's epoch.
    EpochMismatch,
    /// The verdict leaf's epoch is not the inventory's plus one.
    EpochGap,
    /// The inventory anchor predates the flip lottery, so submissions could
    /// still arrive after it and the set it shows is not the epoch's final one.
    InventoryStillOpen,
    /// The two anchors are not in inventory-then-verdict order.
    HeightOrder,
    /// The bundle claims no flips.
    EmptyManifest,
    /// The record's flips and the manifest disagree.
    ManifestMismatch,
    /// The verdict leaf still carries flips, so it is not the post-validation state.
    VerdictNotCleared,
    /// A record's protobuf could not be read.
    Malformed,
}

/// One membership proof: the key, the value, the leaf's version, and the path.
pub struct Leaf<'a> {
    pub key: &'a [u8],
    pub value: &'a [u8],
    pub version: i64,
    pub path: &'a [PathItem<'a>],
}

/// One anchored state version: the height, the state root, and two leaves.
pub struct Anchor<'a> {
    pub height: u64,
    pub root: &'a [u8],
    pub identity: Leaf<'a>,
    pub global: Leaf<'a>,
}

/// What a seller publishes for one author and epoch.
pub struct Bundle<'a> {
    pub author: &'a [u8],
    pub epoch: u32,
    /// The flip CIDs in the record's own order.
    pub manifest: &'a [&'a [u8]],
    pub inventory: Anchor<'a>,
    pub verdict: Anchor<'a>,
}

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub struct SessionVerdict {
    pub label: Label,
    pub flags: u32,
    pub flips: u32,
}

/// Folds one leaf and checks it against the anchor's root.
fn anchored<H: Hash256>(root: &[u8], leaf: &Leaf) -> Result<(), Refusal> {
    if verify_value_with::<H>(root, leaf.version, leaf.key, leaf.value, leaf.path) {
        Ok(())
    } else {
        Err(Refusal::RootMismatch)
    }
}

fn identity_key_for(author: &[u8]) -> [u8; IDENTITY_KEY_BYTES] {
    let mut key = [0u8; IDENTITY_KEY_BYTES];
    key[0] = IDENTITY_KEY_PREFIX;
    let mut index = 0;
    while index < AUTHOR_BYTES {
        key[1 + index] = author[index];
        index += 1;
    }
    key
}

fn is_global_key(key: &[u8]) -> bool {
    key == GLOBAL_KEY
}

// ---------------------------------------------------------------------------
// Minimal protobuf reader.
// ---------------------------------------------------------------------------

enum Value<'a> {
    Varint(u64),
    Bytes(&'a [u8]),
    Skipped,
}

struct Field<'a> {
    number: u32,
    value: Value<'a>,
}

struct Fields<'a> {
    bytes: &'a [u8],
    at: usize,
}

impl<'a> Fields<'a> {
    fn new(bytes: &'a [u8]) -> Self {
        Fields { bytes, at: 0 }
    }

    fn varint(&mut self) -> Option<u64> {
        let mut value: u64 = 0;
        let mut shift = 0u32;
        loop {
            let byte = *self.bytes.get(self.at)?;
            self.at += 1;
            if shift >= 64 {
                return None;
            }
            value |= u64::from(byte & 0x7f) << shift;
            if byte & 0x80 == 0 {
                return Some(value);
            }
            shift += 7;
        }
    }

    fn bytes_field(&mut self) -> Option<&'a [u8]> {
        let len = self.varint()? as usize;
        let end = self.at.checked_add(len)?;
        let out = self.bytes.get(self.at..end)?;
        self.at = end;
        Some(out)
    }

    fn next(&mut self) -> Option<Field<'a>> {
        if self.at >= self.bytes.len() {
            return None;
        }
        let key = self.varint()?;
        let number = (key >> 3) as u32;
        let wire = key & 7;
        if number == 0 {
            return None;
        }
        let value = match wire {
            0 => Value::Varint(self.varint()?),
            1 => {
                self.at = self.at.checked_add(8)?;
                Value::Skipped
            }
            2 => Value::Bytes(self.bytes_field()?),
            5 => {
                self.at = self.at.checked_add(4)?;
                Value::Skipped
            }
            _ => return None,
        };
        if self.at > self.bytes.len() {
            return None;
        }
        Some(Field { number, value })
    }
}

/// The global object's epoch (`ProtoStateGlobal.epoch`).
///
/// Proto3 omits a zero value, and zero is the first epoch of a chain, so an
/// absent field means epoch 0 rather than malformed - the same reading the node
/// itself gives the record. The value is committed in the state tree, so the
/// choice of anchor, not this reader, decides which epoch a proof is about.
pub fn global_epoch(value: &[u8]) -> Result<u32, Refusal> {
    let mut fields = Fields::new(value);
    let mut epoch = 0u32;
    while let Some(field) = fields.next() {
        if field.number == GLOBAL_EPOCH_FIELD {
            match field.value {
                Value::Varint(value) => epoch = value as u32,
                _ => return Err(Refusal::Malformed),
            }
        }
    }
    Ok(epoch)
}

/// A CIDv1 value is 36 bytes: version, raw codec, sha2-256, length, digest.
const CIDV1_RAW_BYTES: usize = 36;
/// Its multibase text is `b` plus 58 lower-case base32 characters.
const CIDV1_RAW_TEXT: usize = 59;

/// `b` plus lower-case base32 without padding: the multibase spelling a bundle,
/// the availability advisory and the blob's own CID use.
///
/// Only CIDv1 values are rendered; a CIDv0 or anything else returns `None`, and
/// the comparison then falls back to the bytes as they are.
fn multibase_base32(cid: &[u8]) -> Option<[u8; CIDV1_RAW_TEXT]> {
    if cid.len() != CIDV1_RAW_BYTES || cid[0] != 0x01 {
        return None;
    }
    const ALPHABET: &[u8; 32] = b"abcdefghijklmnopqrstuvwxyz234567";
    let mut out = [0u8; CIDV1_RAW_TEXT];
    out[0] = b'b';
    let mut written = 1usize;
    let mut acc: u32 = 0;
    let mut have: u32 = 0;
    for &byte in cid {
        acc = (acc << 8) | u32::from(byte);
        have += 8;
        while have >= 5 {
            have -= 5;
            out[written] = ALPHABET[((acc >> have) & 0x1f) as usize];
            written += 1;
        }
        acc &= if have == 0 { 0 } else { (1u32 << have) - 1 };
    }
    if have > 0 {
        out[written] = ALPHABET[((acc << (5 - have)) & 0x1f) as usize];
        written += 1;
    }
    if written != CIDV1_RAW_TEXT {
        return None;
    }
    Some(out)
}

/// Whether a record's flip CID names the manifest's entry.
///
/// The chain stores a CIDv1 value as its raw bytes (`cid.Bytes()` in the flip
/// submission attachment), while a bundle and the guest's own blob check use
/// the multibase text. Both spellings of the same CID are accepted; nothing
/// else is, so a substituted entry still fails.
fn cid_matches(record: &[u8], manifest: &[u8]) -> bool {
    if record == manifest {
        return true;
    }
    match multibase_base32(record) {
        Some(text) => text.as_slice() == manifest,
        None => false,
    }
}

/// The global object's validation period (`ProtoStateGlobal.validationPeriod`).
///
/// Proto3 omits a zero value, and zero is `NonePeriod` - the period in which a
/// flip submission is still valid - so an absent field means "open" rather than
/// "malformed". That is the fail-closed direction: a record that does not say
/// the lottery started is treated as if it could still change.
pub fn global_period(value: &[u8]) -> Result<u32, Refusal> {
    let mut fields = Fields::new(value);
    let mut period = 0u32;
    while let Some(field) = fields.next() {
        if field.number == GLOBAL_VALIDATION_PERIOD_FIELD {
            match field.value {
                Value::Varint(value) => period = value as u32,
                _ => return Err(Refusal::Malformed),
            }
        }
    }
    Ok(period)
}

/// The record's validation status flags (`ProtoStateIdentity.validationStatus`).
///
/// Proto3 omits a zero value, and zero is exactly the clean case, so an absent
/// field means "no flags" rather than a malformed record.
pub fn identity_flags(value: &[u8]) -> Result<u32, Refusal> {
    let mut fields = Fields::new(value);
    let mut flags = 0u32;
    while let Some(field) = fields.next() {
        if field.number == IDENTITY_VALIDATION_STATUS_FIELD {
            match field.value {
                Value::Varint(value) => flags = value as u32,
                _ => return Err(Refusal::Malformed),
            }
        }
    }
    Ok(flags)
}

/// Walks the record's flips and compares them, in order, with the manifest.
fn identity_matches_manifest(value: &[u8], manifest: &[&[u8]]) -> Result<u32, Refusal> {
    if manifest.is_empty() {
        return Err(Refusal::EmptyManifest);
    }
    let mut fields = Fields::new(value);
    let mut count = 0usize;
    while let Some(field) = fields.next() {
        if field.number != IDENTITY_FLIPS_FIELD {
            continue;
        }
        let entry = match field.value {
            Value::Bytes(bytes) => bytes,
            _ => return Err(Refusal::Malformed),
        };
        if count >= manifest.len() {
            return Err(Refusal::ManifestMismatch);
        }
        let mut entry_fields = Fields::new(entry);
        let mut cid = None;
        while let Some(entry_field) = entry_fields.next() {
            if entry_field.number == FLIP_CID_FIELD {
                match entry_field.value {
                    Value::Bytes(bytes) => cid = Some(bytes),
                    _ => return Err(Refusal::Malformed),
                }
            }
        }
        let cid = cid.ok_or(Refusal::Malformed)?;
        if !cid_matches(cid, manifest[count]) {
            return Err(Refusal::ManifestMismatch);
        }
        count += 1;
    }
    if count != manifest.len() {
        return Err(Refusal::ManifestMismatch);
    }
    Ok(count as u32)
}

/// The verdict leaf must be the post-validation record: no flips left.
fn identity_has_no_flips(value: &[u8]) -> Result<(), Refusal> {
    let mut fields = Fields::new(value);
    while let Some(field) = fields.next() {
        if field.number == IDENTITY_FLIPS_FIELD {
            return Err(Refusal::VerdictNotCleared);
        }
    }
    Ok(())
}

pub fn label_for(flags: u32) -> Label {
    Label::for_flags(flags)
}

/// Verifies a bundle against the fold a caller hands it. A guest passes the
/// zkVM's accelerated SHA-256; the native tests pass `Sha2Hash`.
pub fn verify_with<H: Hash256>(bundle: &Bundle) -> Result<SessionVerdict, Refusal> {
    if bundle.author.len() != AUTHOR_BYTES {
        return Err(Refusal::IdentityKey);
    }
    let expected_key = identity_key_for(bundle.author);
    if bundle.inventory.identity.key != expected_key.as_slice()
        || bundle.verdict.identity.key != expected_key.as_slice()
    {
        return Err(Refusal::IdentityKey);
    }
    if !is_global_key(bundle.inventory.global.key) || !is_global_key(bundle.verdict.global.key) {
        return Err(Refusal::GlobalKey);
    }
    if bundle.inventory.height >= bundle.verdict.height {
        return Err(Refusal::HeightOrder);
    }

    anchored::<H>(bundle.inventory.root, &bundle.inventory.identity)?;
    anchored::<H>(bundle.inventory.root, &bundle.inventory.global)?;
    anchored::<H>(bundle.verdict.root, &bundle.verdict.identity)?;
    anchored::<H>(bundle.verdict.root, &bundle.verdict.global)?;

    let inventory_epoch = global_epoch(bundle.inventory.global.value)?;
    let verdict_epoch = global_epoch(bundle.verdict.global.value)?;
    if inventory_epoch != bundle.epoch {
        return Err(Refusal::EpochMismatch);
    }
    if verdict_epoch != bundle.epoch + 1 {
        return Err(Refusal::EpochGap);
    }

    // Membership at the inventory height is not completeness: the same epoch
    // accepts a submission until the chain enters the flip lottery, so an
    // earlier anchor can be followed by a flip the bundle does not list. The
    // anchor has to report a period in which a submission is already refused.
    let inventory_period = global_period(bundle.inventory.global.value)?;
    if inventory_period < FLIP_LOTTERY_PERIOD {
        return Err(Refusal::InventoryStillOpen);
    }

    let flips = identity_matches_manifest(bundle.inventory.identity.value, bundle.manifest)?;
    identity_has_no_flips(bundle.verdict.identity.value)?;
    let flags = identity_flags(bundle.verdict.identity.value)?;

    Ok(SessionVerdict {
        label: label_for(flags),
        flags,
        flips,
    })
}

/// The native entry point: the fold over the `sha2` crate.
pub fn verify(bundle: &Bundle) -> Result<SessionVerdict, Refusal> {
    verify_with::<Sha2Hash>(bundle)
}

#[cfg(test)]
mod tests {
    extern crate std;

    use super::*;
    use crate::iavl::PathItem;
    use std::vec::Vec;

    struct OwnedLeaf {
        key: Vec<u8>,
        value: Vec<u8>,
        version: i64,
        path: Vec<OwnedItem>,
    }

    struct OwnedItem {
        height: i64,
        size: i64,
        version: i64,
        left: Vec<u8>,
        right: Vec<u8>,
    }

    struct OwnedAnchor {
        height: u64,
        root: Vec<u8>,
        identity: OwnedLeaf,
        global: OwnedLeaf,
    }

    struct OwnedBundle {
        author: Vec<u8>,
        epoch: u32,
        manifest: Vec<Vec<u8>>,
        inventory: OwnedAnchor,
        verdict: OwnedAnchor,
    }

    fn bytes(value: &serde_json::Value, field: &str) -> Vec<u8> {
        decode_hex(value[field].as_str().expect(field))
    }

    fn nibble(byte: u8) -> u8 {
        match byte {
            b'0'..=b'9' => byte - b'0',
            b'a'..=b'f' => byte - b'a' + 10,
            b'A'..=b'F' => byte - b'A' + 10,
            _ => panic!("a fixture field is not hex"),
        }
    }

    fn decode_hex(value: &str) -> Vec<u8> {
        let bytes = value.as_bytes();
        assert!(
            bytes.len() % 2 == 0,
            "a fixture field has an odd hex length"
        );
        (0..bytes.len() / 2)
            .map(|index| (nibble(bytes[index * 2]) << 4) | nibble(bytes[index * 2 + 1]))
            .collect()
    }

    fn owned_leaf(value: &serde_json::Value) -> OwnedLeaf {
        OwnedLeaf {
            key: bytes(value, "keyHex"),
            value: bytes(value, "valueHex"),
            version: value["version"].as_i64().expect("version"),
            path: value["path"]
                .as_array()
                .expect("path")
                .iter()
                .map(|item| OwnedItem {
                    height: item["height"].as_i64().expect("height"),
                    size: item["size"].as_i64().expect("size"),
                    version: item["version"].as_i64().expect("version"),
                    left: item["leftHex"].as_str().map(decode_hex).unwrap_or_default(),
                    right: item["rightHex"]
                        .as_str()
                        .map(decode_hex)
                        .unwrap_or_default(),
                })
                .collect(),
        }
    }

    fn owned_anchor(value: &serde_json::Value) -> OwnedAnchor {
        OwnedAnchor {
            height: value["height"].as_u64().expect("height"),
            root: bytes(value, "rootHex"),
            identity: owned_leaf(&value["identity"]),
            global: owned_leaf(&value["global"]),
        }
    }

    fn owned_bundle(doc: &serde_json::Value, session: &serde_json::Value) -> OwnedBundle {
        OwnedBundle {
            author: bytes(doc, "author"),
            epoch: session["epoch"].as_u64().expect("epoch") as u32,
            manifest: doc["manifest"]
                .as_array()
                .expect("manifest")
                .iter()
                .map(|cid| decode_hex(cid.as_str().expect("cid")))
                .collect(),
            inventory: owned_anchor(&session["inventory"]),
            verdict: owned_anchor(&session["verdict"]),
        }
    }

    fn path_items(leaf: &OwnedLeaf) -> Vec<PathItem<'_>> {
        leaf.path
            .iter()
            .map(|item| PathItem {
                height: item.height,
                size: item.size,
                version: item.version,
                left: &item.left,
                right: &item.right,
            })
            .collect()
    }

    fn view<'a>(
        owned: &'a OwnedBundle,
        paths: &'a [Vec<PathItem<'a>>],
        manifest: &'a [&'a [u8]],
    ) -> Bundle<'a> {
        let leaf = |index: usize, own: &'a OwnedLeaf| Leaf {
            key: &own.key,
            value: &own.value,
            version: own.version,
            path: &paths[index],
        };
        Bundle {
            author: &owned.author,
            epoch: owned.epoch,
            manifest,
            inventory: Anchor {
                height: owned.inventory.height,
                root: &owned.inventory.root,
                identity: leaf(0, &owned.inventory.identity),
                global: leaf(1, &owned.inventory.global),
            },
            verdict: Anchor {
                height: owned.verdict.height,
                root: &owned.verdict.root,
                identity: leaf(2, &owned.verdict.identity),
                global: leaf(3, &owned.verdict.global),
            },
        }
    }

    fn verify_owned(owned: &OwnedBundle) -> Result<SessionVerdict, Refusal> {
        let paths = [
            path_items(&owned.inventory.identity),
            path_items(&owned.inventory.global),
            path_items(&owned.verdict.identity),
            path_items(&owned.verdict.global),
        ];
        let manifest: Vec<&[u8]> = owned.manifest.iter().map(|cid| cid.as_slice()).collect();
        let bundle = view(owned, &paths, &manifest);
        verify(&bundle)
    }

    fn varint(mut value: u64) -> Vec<u8> {
        let mut out = Vec::new();
        loop {
            let byte = (value & 0x7f) as u8;
            value >>= 7;
            if value == 0 {
                out.push(byte);
                return out;
            }
            out.push(byte | 0x80);
        }
    }

    /// A `ProtoStateGlobal` carrying the epoch and, when asked, the period.
    fn global_value(epoch: u32, period: u32, with_period: bool) -> Vec<u8> {
        let mut out = Vec::new();
        out.push(0x08);
        out.extend(varint(u64::from(epoch)));
        if with_period {
            out.push(0x18);
            out.extend(varint(u64::from(period)));
        }
        out
    }

    /// Replaces the inventory global record and rebuilds both sides of the
    /// fixture's two-leaf state tree, so the proof stays valid and only the
    /// rule under test can refuse the bundle.
    fn set_inventory_global(owned: &mut OwnedBundle, value: Vec<u8>) {
        use crate::iavl::{inner_hash_with, leaf_hash_with, sha256, Sha2Hash};

        owned.inventory.global.value = value;
        let identity_hash = leaf_hash_with::<Sha2Hash>(
            owned.inventory.identity.version,
            &owned.inventory.identity.key,
            &sha256(&owned.inventory.identity.value),
        );
        let global_hash = leaf_hash_with::<Sha2Hash>(
            owned.inventory.global.version,
            &owned.inventory.global.key,
            &sha256(&owned.inventory.global.value),
        );
        let stored = &owned.inventory.global.path[0];
        let global_is_right = !stored.left.is_empty();
        let (height, size, version) = (stored.height, stored.size, stored.version);
        assert_eq!(
            owned.inventory.identity.path.len(),
            1,
            "the fixture's inventory tree is two leaves"
        );

        let mut global_path = Vec::new();
        global_path.push(OwnedItem {
            height,
            size,
            version,
            left: if global_is_right {
                identity_hash.to_vec()
            } else {
                Vec::new()
            },
            right: if global_is_right {
                Vec::new()
            } else {
                identity_hash.to_vec()
            },
        });
        owned.inventory.global.path = global_path;

        let mut identity_path = Vec::new();
        identity_path.push(OwnedItem {
            height,
            size,
            version,
            left: if global_is_right {
                Vec::new()
            } else {
                global_hash.to_vec()
            },
            right: if global_is_right {
                global_hash.to_vec()
            } else {
                Vec::new()
            },
        });
        owned.inventory.identity.path = identity_path;
        let root = {
            let items = path_items(&owned.inventory.global);
            inner_hash_with::<Sha2Hash>(&items[0], &global_hash)
        };
        owned.inventory.root = root.to_vec();
    }

    fn document() -> serde_json::Value {
        serde_json::from_str(include_str!("../fixtures/session-bundle.json")).expect("fixture")
    }

    fn session<'a>(doc: &'a serde_json::Value, label: &str) -> &'a serde_json::Value {
        doc["sessions"]
            .as_array()
            .expect("sessions")
            .iter()
            .find(|session| session["label"].as_str() == Some(label))
            .expect("session")
    }

    #[test]
    fn every_fixture_session_reaches_its_expected_outcome() {
        let doc = document();
        for entry in doc["sessions"].as_array().expect("sessions") {
            let owned = owned_bundle(&doc, entry);
            let expected = entry["expectedLabel"].as_str().expect("expectedLabel");
            let outcome = verify_owned(&owned);
            if expected == "refusal" {
                assert_eq!(
                    outcome,
                    Err(Refusal::VerdictNotCleared),
                    "session {} should refuse",
                    entry["label"]
                );
                continue;
            }
            let verdict = outcome.expect("session verifies");
            assert_eq!(verdict.label.as_str(), expected);
            assert_eq!(verdict.flips, owned.manifest.len() as u32);
        }
    }

    #[test]
    fn the_reported_and_weak_flags_survive_as_the_raw_value() {
        let doc = document();
        let reported = owned_bundle(&doc, session(&doc, "reported"));
        let verdict = verify_owned(&reported).expect("reported session verifies");
        assert_eq!(verdict.label, Label::ContainsReported);
        assert_eq!(
            verdict.flags & FLAG_AT_LEAST_ONE_REPORTED,
            FLAG_AT_LEAST_ONE_REPORTED
        );

        let weak = owned_bundle(&doc, session(&doc, "weak"));
        let verdict = verify_owned(&weak).expect("weak session verifies");
        assert_eq!(verdict.label, Label::Weak);
        assert_eq!(
            verdict.flags & FLAG_AT_LEAST_ONE_NOT_QUALIFIED,
            FLAG_AT_LEAST_ONE_NOT_QUALIFIED
        );
    }

    #[test]
    fn a_changed_manifest_cid_is_refused() {
        let doc = document();
        let mut owned = owned_bundle(&doc, session(&doc, "clean"));
        owned.manifest[2][0] ^= 0xff;
        assert_eq!(verify_owned(&owned), Err(Refusal::ManifestMismatch));
    }

    /// One length-delimited protobuf field.
    fn field(number: u32, value: &[u8]) -> Vec<u8> {
        let mut out = varint(u64::from((number << 3) | 2));
        out.extend(varint(value.len() as u64));
        out.extend_from_slice(value);
        out
    }

    /// A CIDv1/raw/sha2-256 value whose digest repeats `seed`.
    fn raw_cid(seed: u8) -> Vec<u8> {
        let mut cid = std::vec![0x01, 0x55, 0x12, 0x20];
        cid.extend(std::iter::repeat(seed).take(32));
        cid
    }

    /// An identity record that lists exactly these flips.
    fn identity_value(cids: &[Vec<u8>]) -> Vec<u8> {
        let mut out = Vec::new();
        for cid in cids {
            let entry = field(1, cid);
            out.extend(field(9, &entry));
        }
        out
    }

    #[test]
    fn an_absent_epoch_reads_as_zero() {
        // Proto3 omits a zero value, and a chain's first epoch is 0, so a
        // record that carries only the period still names epoch 0.
        assert_eq!(global_epoch(&[]), Ok(0));
        assert_eq!(global_epoch(&[0x18, 0x01]), Ok(0));
        assert_eq!(global_epoch(&[0x08, 0x05]), Ok(5));
    }

    #[test]
    fn a_raw_record_cid_matches_the_multibase_manifest() {
        let doc = document();
        let mut owned = owned_bundle(&doc, session(&doc, "clean"));
        // The chain stores the raw CIDv1 bytes; the bundle names the text.
        let raw: Vec<Vec<u8>> = (0..owned.manifest.len())
            .map(|index| raw_cid(index as u8 + 1))
            .collect();
        owned.inventory.identity.value = identity_value(&raw);
        let global = owned.inventory.global.value.clone();
        set_inventory_global(&mut owned, global);
        owned.manifest = raw
            .iter()
            .map(|cid| {
                multibase_base32(cid)
                    .expect("a cidv1 value renders")
                    .to_vec()
            })
            .collect();

        let verdict = verify_owned(&owned).expect("a raw record cid names the text manifest");
        assert_eq!(verdict.flips, raw.len() as u32);

        // The same bundle with one text entry altered is still refused.
        let mut altered = owned;
        let last = altered.manifest.len() - 1;
        let mut text = altered.manifest[last].clone();
        text[10] = if text[10] == b'a' { b'b' } else { b'a' };
        altered.manifest[last] = text;
        assert_eq!(verify_owned(&altered), Err(Refusal::ManifestMismatch));
    }

    #[test]
    fn an_incomplete_or_extended_manifest_is_refused() {
        let doc = document();
        let mut short = owned_bundle(&doc, session(&doc, "clean"));
        short.manifest.pop();
        assert_eq!(verify_owned(&short), Err(Refusal::ManifestMismatch));

        let mut long = owned_bundle(&doc, session(&doc, "clean"));
        let extra = long.manifest[0].clone();
        long.manifest.push(extra);
        assert_eq!(verify_owned(&long), Err(Refusal::ManifestMismatch));
    }

    #[test]
    fn a_swapped_root_is_refused() {
        let doc = document();
        let mut swapped = owned_bundle(&doc, session(&doc, "clean"));
        let inventory_root = swapped.inventory.root.clone();
        swapped.inventory.root = swapped.verdict.root.clone();
        swapped.verdict.root = inventory_root;
        assert_eq!(verify_owned(&swapped), Err(Refusal::RootMismatch));
    }

    #[test]
    fn another_author_is_refused() {
        let doc = document();
        let mut owned = owned_bundle(&doc, session(&doc, "clean"));
        owned.author[0] ^= 0x01;
        assert_eq!(verify_owned(&owned), Err(Refusal::IdentityKey));
    }

    #[test]
    fn an_epoch_that_is_not_the_inventory_epoch_is_refused() {
        let doc = document();
        let mut owned = owned_bundle(&doc, session(&doc, "clean"));
        owned.epoch += 1;
        assert_eq!(verify_owned(&owned), Err(Refusal::EpochMismatch));

        let mut tampered = owned_bundle(&doc, session(&doc, "clean"));
        tampered.verdict.global.value = tampered.inventory.global.value.clone();
        assert_eq!(verify_owned(&tampered), Err(Refusal::RootMismatch));
    }

    #[test]
    fn the_inventory_leaf_cannot_stand_in_for_the_verdict() {
        let doc = document();
        let mut owned = owned_bundle(&doc, session(&doc, "clean"));
        owned.verdict.identity = owned_leaf(&session(&doc, "clean")["inventory"]["identity"]);
        owned.verdict.root = bytes(&session(&doc, "clean")["verdict"], "rootHex");
        assert_eq!(verify_owned(&owned), Err(Refusal::RootMismatch));
    }

    #[test]
    fn the_global_period_reads_the_way_the_chain_writes_it() {
        // Proto3 omits a zero period, and zero is the period that still accepts
        // a submission, so an absent field is "open" and never "malformed".
        assert_eq!(global_period(&global_value(228, 0, false)), Ok(0));
        assert_eq!(global_period(&global_value(228, 1, true)), Ok(1));
        assert_eq!(global_period(&global_value(228, 4, true)), Ok(4));
        // Field 3 with a length-delimited wire type is not the chain's shape.
        assert_eq!(
            global_period(&[0x08, 0xe4, 0x01, 0x1a, 0x01, 0x00]),
            Err(Refusal::Malformed)
        );
    }

    #[test]
    fn an_inventory_anchor_before_the_lottery_is_refused() {
        let doc = document();
        let mut owned = owned_bundle(&doc, session(&doc, "clean"));
        let epoch = owned.epoch;
        // The same bundle, with a valid proof, taken while submissions were
        // still accepted: a later flip could be missing from its manifest.
        set_inventory_global(&mut owned, global_value(epoch, 0, false));
        assert_eq!(verify_owned(&owned), Err(Refusal::InventoryStillOpen));
    }

    #[test]
    fn a_later_ceremony_period_is_still_final() {
        let doc = document();
        let mut owned = owned_bundle(&doc, session(&doc, "clean"));
        let epoch = owned.epoch;
        set_inventory_global(&mut owned, global_value(epoch, 4, true));
        let verdict = verify_owned(&owned).expect("an anchor inside the ceremony verifies");
        assert_eq!(verdict.label, Label::Clean);
        assert_eq!(verdict.flips, owned.manifest.len() as u32);
    }
}
