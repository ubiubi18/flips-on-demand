//! Exact proof bindings; caller assertions and replacement ciphertexts cannot settle.
use crate::{market::{Error, Result}, records::*, verifier};
use fod_fast_content_core::{Journal, JOURNAL_BYTES};
use fod_fast_session_core::verify_with_roots_and_cids;
use alloc::vec::Vec;
const POLICY_DIGEST: [u8; 32] = [
    0x06, 0xb3, 0x1d, 0x89, 0x13, 0x36, 0xe0, 0xb2, 0x0f, 0xce, 0xc9, 0x98, 0x3c, 0xfb, 0x03, 0x03,
    0x18, 0x6b, 0x04, 0xbc, 0x0d, 0xa9, 0x06, 0xc9, 0x26, 0xd5, 0x7e, 0x25, 0xa7, 0xf3, 0x39, 0x9f,
];
const CONSENSUS_DIGEST: [u8; 32] = [
    0xca, 0xc2, 0xb1, 0x7b, 0x70, 0xe1, 0x12, 0x49, 0x91, 0x23, 0xc4, 0x96, 0x92, 0x8e, 0x2f, 0x8e,
    0xb9, 0xba, 0x1b, 0x78, 0x56, 0x05, 0x18, 0x51, 0x9c, 0x34, 0x5f, 0xfb, 0xc1, 0xd5, 0xef, 0xdf,
];
const NETWORK_DIGEST: [u8; 32] = [
    0xf4, 0xb1, 0x51, 0x2d, 0x62, 0x54, 0x23, 0x6f, 0x6c, 0x27, 0xa5, 0x38, 0x77, 0x0e, 0x79, 0x02,
    0x24, 0x07, 0x6e, 0xf8, 0x8b, 0x24, 0xd5, 0xbc, 0xe8, 0xda, 0x73, 0x20, 0x09, 0x34, 0x76, 0xd2,
];

pub const CONTENT_IMAGE_ID: [u8; 32] = [
    0x4f, 0xd1, 0x24, 0xc8, 0xcb, 0x91, 0x37, 0x0d, 0xab, 0x60, 0xd3, 0xe1, 0xd7, 0xc4, 0xe6, 0x66,
    0xe7, 0x94, 0x85, 0xe2, 0x10, 0xb5, 0xf2, 0x0e, 0xd3, 0x6c, 0xc2, 0x10, 0x48, 0x83, 0x72, 0x6c,
];

pub fn valid_delivery_key(key: &[u8;65]) -> bool {
    let mut one = [0;32]; one[31] = 1;
    key[0] == 4 && fod_delivery_core::encrypt_to(key, b"", &one).is_some()
}

/// Listing carries public commitments and canonical state leaves, never a content seal.
/// Possession of working keys is checked by the buyer, or by `content` in a dispute.
pub fn listing<F>(raw: &[u8], caller: Address, contract: Address, root_at: F) -> Result<(Bundle, Vec<u8>)>
where F: FnMut(u64) -> Option<Digest> {
    const HEADER: usize = JOURNAL_BYTES + 2;
    if raw.len() < HEADER { return Err(Error::Malformed); }
    let len = u16::from_be_bytes(raw[HEADER-2..HEADER].try_into().map_err(|_|Error::Malformed)?) as usize;
    if len == 0 || raw.len() != HEADER+len { return Err(Error::Malformed); }
    let journal = &raw[..JOURNAL_BYTES];
    let j = Journal::decode(journal).ok_or(Error::Proof)?;
    if j.author != caller || j.contract != contract || j.network != NETWORK_DIGEST ||
        j.policy != POLICY_DIGEST || j.consensus != CONSENSUS_DIGEST || j.count == 0 || j.count > 32 || j.original_keys == [0;32] {
        return Err(Error::Proof);
    }
    let (verdict, mut cids) = verify_with_roots_and_cids(&raw[HEADER..], &j.author, j.epoch,
        j.count, &j.original_inventory, root_at).map_err(|_|Error::Proof)?;
    if verdict.flags != 0 || verdict.flips != u32::from(j.count) {
        return Err(Error::Proof);
    }
    cids.sort(); let mut inventory = alloc::vec![cids.len() as u8];
    for cid in cids {
        if cid.len() != 59 || cid[0] != b'b' || !cid[1..].iter().all(|c|c.is_ascii_lowercase() || (b'2'..=b'7').contains(c)) {
            return Err(Error::Proof);
        }
        inventory.push(cid.len() as u8); inventory.extend_from_slice(cid);
    }
    Ok((Bundle { author: caller, epoch: j.epoch, count: j.count, inventory: j.original_inventory,
        keys: j.original_keys, sale: 0, used: 0 }, inventory))
}

pub fn content_journal_matches(raw: &[u8], b: &Bundle, contract: &Address) -> bool {
    let Some(j) = Journal::decode(raw) else { return false; };
    j.contract == *contract && j.author == b.author && j.epoch == b.epoch &&
        j.count == b.count && j.original_inventory == b.inventory && j.original_keys == b.keys &&
        j.network == NETWORK_DIGEST && j.policy == POLICY_DIGEST && j.consensus == CONSENSUS_DIGEST
}

pub fn content(j: &[u8], seal: &[u8], b: &Bundle, contract: &Address) -> bool {
    content_journal_matches(j, b, contract) &&
        verifier::verify_fast_content_image(j, seal, &CONTENT_IMAGE_ID)
}

pub fn dispute(cj: &[u8], cs: &[u8], dj: &[u8], ds: &[u8], t: &Trade, b: &Bundle,
    id: u64, contract: &Address) -> bool {
    b.author == t.seller && content(cj, cs, b, contract) && delivery(dj, ds, t, b, id, contract)
}

pub fn delivery_journal_matches(j: &[u8], t: &Trade, b: &Bundle, id: u64, contract: &Address) -> bool {
    j.len() == verifier::JOURNAL_BYTES && j[..4] == 2u32.to_be_bytes() &&
    j[4..24] == contract[..] && j[24..32] == id.to_be_bytes() &&
    j[32..40] == 1u64.to_be_bytes() && j[40..60] == t.seller && j[60..80] == t.buyer &&
    j[80..145] == t.delivery_key && j[145..177] == b.keys && j[177..209] == t.ciphertext_hash &&
    j[209..241] == NETWORK_DIGEST && j[241..249] == t.funded_at.to_be_bytes() &&
    j[249..281] == t.funding_root
}

pub fn delivery(j: &[u8], seal: &[u8], t: &Trade, b: &Bundle, id: u64, contract: &Address) -> bool {
    delivery_journal_matches(j, t, b, id, contract) && verifier::verify_delivery(j, seal)
}
