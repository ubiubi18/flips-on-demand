extern crate std;

use super::*;
use serde_json::Value;
use std::vec::Vec;

fn hex_bytes(value: &str) -> Vec<u8> {
    assert_eq!(value.len() % 2, 0);
    value
        .as_bytes()
        .chunks_exact(2)
        .map(|pair| u8::from_str_radix(std::str::from_utf8(pair).unwrap(), 16).unwrap())
        .collect()
}

fn write_leaf(out: &mut Vec<u8>, leaf: &Value) {
    let key = hex_bytes(leaf["keyHex"].as_str().unwrap());
    let value = hex_bytes(leaf["valueHex"].as_str().unwrap());
    out.push(key.len() as u8);
    out.extend_from_slice(&key);
    out.extend_from_slice(&(value.len() as u16).to_be_bytes());
    out.extend_from_slice(&value);
    out.extend_from_slice(&leaf["version"].as_i64().unwrap().to_be_bytes());
    let path = leaf["path"].as_array().unwrap();
    out.push(path.len() as u8);
    for item in path {
        for name in ["height", "size", "version"] {
            out.extend_from_slice(&item[name].as_i64().unwrap().to_be_bytes());
        }
        for name in ["leftHex", "rightHex"] {
            let hash = hex_bytes(item[name].as_str().unwrap());
            out.push(hash.len() as u8);
            out.extend_from_slice(&hash);
        }
    }
}

fn write_anchor(out: &mut Vec<u8>, anchor: &Value) {
    out.extend_from_slice(&anchor["height"].as_u64().unwrap().to_be_bytes());
    out.extend_from_slice(&hex_bytes(anchor["rootHex"].as_str().unwrap()));
    write_leaf(out, &anchor["global"]);
    write_leaf(out, &anchor["identity"]);
}

struct Fixture {
    journal: [u8; JOURNAL_BYTES],
    seal: [u8; verifier::SEAL_BYTES],
    wire: Vec<u8>,
    offer: Offer,
    contract: [u8; 20],
    inventory_height: u64,
    inventory_root: [u8; 32],
    verdict_height: u64,
    verdict_root: [u8; 32],
}

impl Fixture {
    fn new() -> Self {
        // Both inputs are committed public synthetic fixtures. No private
        // content witness or live author evidence is read by these tests.
        let proof: Value = serde_json::from_str(include_str!(
            "../../../proofs/fast-content/dist/synthetic-v5-one-flip.json"
        ))
        .unwrap();
        let session: Value = serde_json::from_str(include_str!(
            "../../../proofs/fast-content/dist/synthetic-session.json"
        ))
        .unwrap();
        let image: [u8; 32] = hex_bytes(proof["image_id"].as_str().unwrap()).try_into().unwrap();
        assert_eq!(image, FAST_CONTENT_V5_IMAGE_ID);
        let journal: [u8; JOURNAL_BYTES] = hex_bytes(proof["journal"].as_str().unwrap()).try_into().unwrap();
        let seal: [u8; verifier::SEAL_BYTES] = hex_bytes(proof["seal"].as_str().unwrap()).try_into().unwrap();
        let claim = Journal::decode(&journal).unwrap();
        let public_session = &session["sessions"][0];
        let author: [u8; 20] = hex_bytes(session["author"].as_str().unwrap()).try_into().unwrap();
        assert_eq!(claim.author, author);
        assert_eq!(claim.epoch, public_session["epoch"].as_u64().unwrap() as u16);
        let cids = session["manifest"].as_array().unwrap();
        assert_eq!(usize::from(claim.count), cids.len());
        let mut wire = Vec::new();
        wire.push(1); // Public session wire version.
        wire.extend_from_slice(&author);
        wire.extend_from_slice(&claim.epoch.to_be_bytes());
        wire.push(cids.len() as u8);
        for cid in cids {
            let cid = cid.as_str().unwrap().as_bytes();
            wire.push(cid.len() as u8);
            wire.extend_from_slice(cid);
        }
        write_anchor(&mut wire, &public_session["inventory"]);
        write_anchor(&mut wire, &public_session["verdict"]);
        assert!(wire.len() <= MAX_SESSION_WIRE_BYTES);
        Self {
            journal,
            seal,
            wire,
            offer: Offer {
                author,
                epoch: claim.epoch,
                covered: u32::from(claim.count),
                inventory: claim.original_inventory,
                keys: claim.original_keys,
                price: 1,
                next_nonce: 1,
                open_order: 1,
                retired: false,
            },
            contract: claim.contract,
            inventory_height: public_session["inventory"]["height"].as_u64().unwrap(),
            inventory_root: hex_bytes(public_session["inventory"]["rootHex"].as_str().unwrap()).try_into().unwrap(),
            verdict_height: public_session["verdict"]["height"].as_u64().unwrap(),
            verdict_root: hex_bytes(public_session["verdict"]["rootHex"].as_str().unwrap()).try_into().unwrap(),
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

    fn check(&self, journal: &[u8], seal: &[u8], wire: &[u8], offer: &Offer, contract: &[u8; 20]) -> bool {
        fallback_claim_matches(journal, seal, wire, offer, contract, |height| self.root_at(height))
    }
}

#[test]
fn sealed_content_and_full_canonical_session_are_both_required() {
    let f = Fixture::new();
    assert!(f.check(&f.journal, &f.seal, &f.wire, &f.offer, &f.contract));

    let mut changed_offer = f.offer;
    changed_offer.inventory[0] ^= 1;
    assert!(!f.check(&f.journal, &f.seal, &f.wire, &changed_offer, &f.contract));
    changed_offer = f.offer;
    changed_offer.keys[0] ^= 1;
    assert!(!f.check(&f.journal, &f.seal, &f.wire, &changed_offer, &f.contract));
    changed_offer = f.offer;
    changed_offer.covered += 1;
    assert!(!f.check(&f.journal, &f.seal, &f.wire, &changed_offer, &f.contract));

    let mut omitted = f.wire.clone();
    let first_cid_end = 25 + usize::from(omitted[24]);
    omitted.drain(24..first_cid_end);
    omitted[23] = 0;
    assert!(!f.check(&f.journal, &f.seal, &omitted, &f.offer, &f.contract));
    let mut changed_wire = f.wire.clone();
    changed_wire[25] ^= 1;
    assert!(!f.check(&f.journal, &f.seal, &changed_wire, &f.offer, &f.contract));
    assert!(!fallback_claim_matches(&f.journal, &f.seal, &f.wire, &f.offer, &f.contract, |_| None));
    assert!(!fallback_claim_matches(&f.journal, &f.seal, &f.wire, &f.offer, &f.contract, |_| Some([0; 32])));

    let mut changed_seal = f.seal;
    changed_seal[0] ^= 1;
    assert!(!f.check(&f.journal, &changed_seal, &f.wire, &f.offer, &f.contract));
    let mut changed_journal = f.journal;
    changed_journal[241] ^= 1;
    let mut matching_offer = f.offer;
    matching_offer.keys[0] ^= 1;
    assert!(!f.check(&changed_journal, &f.seal, &f.wire, &matching_offer, &f.contract));
}

#[test]
fn credential_cannot_replay_across_contract_author_or_epoch() {
    let f = Fixture::new();
    let mut other_contract = f.contract;
    other_contract[0] ^= 1;
    assert!(!f.check(&f.journal, &f.seal, &f.wire, &f.offer, &other_contract));
    let mut other_offer = f.offer;
    other_offer.author[0] ^= 1;
    assert!(!f.check(&f.journal, &f.seal, &f.wire, &other_offer, &f.contract));
    other_offer = f.offer;
    other_offer.epoch += 1;
    assert!(!f.check(&f.journal, &f.seal, &f.wire, &other_offer, &f.contract));
    let mut other_session = f.wire.clone();
    other_session[22] ^= 1;
    assert!(!f.check(&f.journal, &f.seal, &other_session, &f.offer, &f.contract));
}

#[test]
fn fallback_argument_requires_exact_bounded_session_wire() {
    let f = Fixture::new();
    let mut arg = Vec::new();
    arg.extend_from_slice(&7u64.to_be_bytes());
    arg.extend_from_slice(&f.journal);
    arg.extend_from_slice(&f.seal);
    arg.extend_from_slice(&(f.wire.len() as u16).to_be_bytes());
    arg.extend_from_slice(&f.wire);
    let (id, journal, seal, wire) = parse_start_arg(&arg).unwrap();
    assert_eq!(id, 7);
    assert_eq!(journal, f.journal);
    assert_eq!(seal, f.seal);
    assert_eq!(wire, f.wire);
    assert!(parse_start_arg(&arg[..arg.len() - 1]).is_none());
    let mut trailing = arg.clone();
    trailing.push(0);
    assert!(parse_start_arg(&trailing).is_none());
    let mut zero_length = arg.clone();
    zero_length[START_HEADER_BYTES - 2..START_HEADER_BYTES].copy_from_slice(&0u16.to_be_bytes());
    assert!(parse_start_arg(&zero_length).is_none());
    assert!(parse_start_arg(&arg[..START_HEADER_BYTES - 1]).is_none());
    assert!(parse_start_arg(&std::vec![0; START_HEADER_BYTES + MAX_SESSION_WIRE_BYTES + 1]).is_none());
}

#[test]
fn production_fallback_grace_margin_and_refund_boundaries() {
    let order = Order {
        offer: [0; 32], buyer: [0; 20], delivery_key: [0; 65],
        paid: 1, bond: 1, funded_at: 100, nonce: 1, state: FUNDED,
        delivery_hash: [0; 32],
    };
    assert!(!order.can_start_fallback(129, PROOF_WINDOW_BLOCKS, BUYER_CONFIRM_GRACE_BLOCKS, PROOF_DEADLINE_MARGIN_BLOCKS));
    assert!(order.can_start_fallback(130, PROOF_WINDOW_BLOCKS, BUYER_CONFIRM_GRACE_BLOCKS, PROOF_DEADLINE_MARGIN_BLOCKS));
    assert!(order.can_start_fallback(8619, PROOF_WINDOW_BLOCKS, BUYER_CONFIRM_GRACE_BLOCKS, PROOF_DEADLINE_MARGIN_BLOCKS));
    assert!(!order.can_start_fallback(8620, PROOF_WINDOW_BLOCKS, BUYER_CONFIRM_GRACE_BLOCKS, PROOF_DEADLINE_MARGIN_BLOCKS));
    assert!(!order.can_refund(8739, PROOF_WINDOW_BLOCKS));
    assert!(order.can_refund(8740, PROOF_WINDOW_BLOCKS));
    let pending = Order { state: FALLBACK_PENDING, ..order };
    assert!(pending.seller_can_reveal(8739, PROOF_WINDOW_BLOCKS));
    assert!(!pending.seller_can_reveal(8740, PROOF_WINDOW_BLOCKS));
    assert!(pending.can_refund(8740, PROOF_WINDOW_BLOCKS));
}
