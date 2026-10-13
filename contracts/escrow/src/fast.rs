//! Experimental fast path. The normal release omits this entire module.
//! Registration remains closed until a reproducible content guest image is
//! pinned in `verifier.rs` and its real receipt is checked.

use super::*;
use fod_fast_content_core::{Journal, JOURNAL_BYTES, MAX_FLIPS};
use fod_fast_delivery_core::{
    compressed_public_key, order_context, valid_point, verify, Delivery, PointBytes, DELIVERY_BYTES,
};
use fod_fast_session_core::{verify_with_roots, MAX_WIRE_BYTES};
use sha2::{Digest, Sha256};

const FAST_BUNDLE_PREFIX: &[u8] = b"pfast/";
const FAST_AUTHOR_EPOCH_PREFIX: &[u8] = b"pfae/";
const FAST_DELIVERY_PREFIX: &[u8] = b"pfdelivery/";
const FAST_CERTIFICATE_PREFIX: &[u8] = b"pfcert/";
const FAST_RECORD_BYTES: usize = 2 + 2 + 4 + 32 + 33 + 32;
const FAST_REGISTER_HEADER: usize = JOURNAL_BYTES + verifier::SEAL_BYTES + 2;

#[derive(Clone, Copy)]
struct FastBundle {
    epoch: u16,
    count: u16,
    flags: u32,
    copies: [u8; 32],
    point: PointBytes,
    original_keys: [u8; 32],
}

impl FastBundle {
    fn encode(&self) -> [u8; FAST_RECORD_BYTES] {
        let mut out = [0u8; FAST_RECORD_BYTES];
        out[0..2].copy_from_slice(&self.epoch.to_be_bytes());
        out[2..4].copy_from_slice(&self.count.to_be_bytes());
        out[4..8].copy_from_slice(&self.flags.to_be_bytes());
        out[8..40].copy_from_slice(&self.copies);
        out[40..73].copy_from_slice(&self.point);
        out[73..105].copy_from_slice(&self.original_keys);
        out
    }

    fn decode(bytes: &[u8]) -> Option<Self> {
        if bytes.len() != FAST_RECORD_BYTES {
            return None;
        }
        let point: PointBytes = bytes[40..73].try_into().ok()?;
        if !valid_point(&point) {
            return None;
        }
        Some(Self {
            epoch: u16::from_be_bytes(bytes[0..2].try_into().ok()?),
            count: u16::from_be_bytes(bytes[2..4].try_into().ok()?),
            flags: u32::from_be_bytes(bytes[4..8].try_into().ok()?),
            copies: bytes[8..40].try_into().ok()?,
            point,
            original_keys: bytes[73..105].try_into().ok()?,
        })
    }
}

unsafe fn read_fast_bundle(bundle: &[u8; 32]) -> Option<FastBundle> {
    read_keyed(key_for_commitment(FAST_BUNDLE_PREFIX, bundle)).and_then(FastBundle::decode)
}

unsafe fn write_fast_bundle(bundle: &[u8; 32], fast: &FastBundle) {
    write_keyed(
        key_for_commitment(FAST_BUNDLE_PREFIX, bundle),
        &fast.encode(),
    );
}

/// ASCII suffix permits an exact `contract_readData` lookup from any synced
/// node, without a transaction archive or an indexer.
fn fast_certificate_key(bundle: &[u8; 32]) -> [u8; 71] {
    let mut key = [0u8; 71];
    key[..FAST_CERTIFICATE_PREFIX.len()].copy_from_slice(FAST_CERTIFICATE_PREFIX);
    let mut at = FAST_CERTIFICATE_PREFIX.len();
    push_hex(&mut key, &mut at, bundle);
    key
}

/// Keep the entire verified public certificate in current contract state.
/// `write_keyed` is limited to 4 KiB, so this points the value Region at the
/// already bounded call argument instead of silently truncating it.
unsafe fn write_fast_certificate(bundle: &[u8; 32], certificate: &[u8]) {
    let key = fast_certificate_key(bundle);
    let key_region = key_raw(&key);
    let value_region = point_region(
        core::ptr::addr_of_mut!(VAL_REGION),
        certificate.as_ptr() as *mut u8,
        certificate.len() as u32,
        certificate.len() as u32,
    );
    set_storage(key_region, value_region);
}

fn author_epoch_id(author: &[u8; 20], epoch: u16) -> [u8; 32] {
    let mut hash = Sha256::new();
    hash.update(b"fod:fast:author-epoch:v1");
    hash.update(author);
    hash.update(epoch.to_be_bytes());
    hash.finalize().into()
}

/// Argument: fixed content journal || Groth16 seal || session length (u16) ||
/// bounded public four-leaf session witness. The author calls after validation.
#[no_mangle]
pub extern "C" fn registerFast(arg: u32) -> u32 {
    unsafe {
        if verifier::FAST_CONTENT_IMAGE_ID == [0u8; 32] {
            return error_reply(b"image_unpinned");
        }
        let (bytes, len) = read_arg(arg);
        if bytes.is_null()
            || len < FAST_REGISTER_HEADER
            || len > FAST_REGISTER_HEADER + MAX_WIRE_BYTES
        {
            return error_reply(b"bad_args");
        }
        let journal_bytes = core::slice::from_raw_parts(bytes, JOURNAL_BYTES);
        let journal = match Journal::decode(journal_bytes) {
            Some(journal) => journal,
            None => return error_reply(b"bad_journal"),
        };
        if journal.network != NETWORK_DIGEST
            || journal.contract != own_address()
            || journal.author != caller_address()
            || journal.consensus != CONSENSUS_VERSION_DIGEST
            || journal.policy != ELIGIBILITY_POLICY_DIGEST
            || journal.count == 0
            || usize::from(journal.count) > MAX_FLIPS
            || !valid_point(&journal.point_commitment)
        {
            return error_reply(b"wrong_claim");
        }
        let unique = author_epoch_id(&journal.author, journal.epoch);
        if read_keyed(key_for_commitment(FAST_AUTHOR_EPOCH_PREFIX, &unique)).is_some() {
            return error_reply(b"epoch_registered");
        }
        let bundle_id = verifier::sha256(journal_bytes);
        if read_pool_bundle(&bundle_id).is_some() {
            return error_reply(b"already_registered");
        }
        let wire_len = u16::from_be_bytes([
            *bytes.add(JOURNAL_BYTES + verifier::SEAL_BYTES),
            *bytes.add(JOURNAL_BYTES + verifier::SEAL_BYTES + 1),
        ]) as usize;
        if wire_len == 0 || wire_len > MAX_WIRE_BYTES || len != FAST_REGISTER_HEADER + wire_len {
            return error_reply(b"bad_session_size");
        }
        let wire = core::slice::from_raw_parts(bytes.add(FAST_REGISTER_HEADER), wire_len);
        let verdict = match verify_with_roots(
            wire,
            &journal.author,
            journal.epoch,
            journal.count,
            &journal.original_inventory,
            |height| header_state_root(height),
        ) {
            Ok(verdict) => verdict,
            Err(_) => return error_reply(b"invalid_session"),
        };
        if verdict.flips != u32::from(journal.count) {
            return error_reply(b"wrong_count");
        }
        let seal = core::slice::from_raw_parts(bytes.add(JOURNAL_BYTES), verifier::SEAL_BYTES);
        if !verifier::verify_fast_content(journal_bytes, seal) {
            return error_reply(b"invalid_proof");
        }
        let mut manifest_hash = Sha256::new();
        manifest_hash.update(b"fod:fast:manifest:v1");
        manifest_hash.update(journal_bytes);
        manifest_hash.update(wire);
        let manifest: [u8; 32] = manifest_hash.finalize().into();
        write_fast_certificate(&bundle_id, core::slice::from_raw_parts(bytes, len));
        write_pool_bundle(
            &bundle_id,
            &PoolBundle::new(
                journal.author,
                manifest,
                journal.original_inventory,
                u32::from(journal.count),
            ),
        );
        write_fast_bundle(
            &bundle_id,
            &FastBundle {
                epoch: journal.epoch,
                count: journal.count,
                flags: verdict.flags,
                copies: journal.copies,
                point: journal.point_commitment,
                original_keys: journal.original_keys,
            },
        );
        write_keyed(key_for_commitment(FAST_AUTHOR_EPOCH_PREFIX, &unique), &[1]);
        let mut out = [0u8; 100];
        let mut at = 0;
        push_bytes(&mut out, &mut at, b"{\"ok\":true,\"bundle\":\"");
        push_hex(&mut out, &mut at, &bundle_id);
        push_bytes(&mut out, &mut at, b"\"}");
        reply(&out[..at])
    }
}

/// Argument: funded order id (u64 big-endian) || 196-byte point proof.
/// Proof and payout execute in one transaction; the included proof is retained
/// for buyer verification and recovery. The accepted censorship risk is
/// documented in SPEC 7.7.
#[no_mangle]
pub extern "C" fn fulfillFast(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len != 8 + DELIVERY_BYTES {
            return error_reply(b"bad_args");
        }
        let id = u64::from_be_bytes(core::slice::from_raw_parts(bytes, 8).try_into().unwrap());
        let delivery =
            match Delivery::decode(core::slice::from_raw_parts(bytes.add(8), DELIVERY_BYTES)) {
                Some(delivery) => delivery,
                None => return error_reply(b"bad_delivery"),
            };
        let mut order = match read_pool_order(id) {
            Some(order) => order,
            None => return error_reply(b"order_not_found"),
        };
        if order.state != POOL_ORDER_FUNDED {
            return error_reply(b"order_closed");
        }
        if block_number() >= order.funded_at.saturating_add(DEADLINE_BLOCKS) {
            return error_reply(b"delivery_deadline");
        }
        let bundle = match read_pool_bundle(&order.bundle) {
            Some(bundle) => bundle,
            None => return error_reply(b"bundle_not_found"),
        };
        let fast = match read_fast_bundle(&order.bundle) {
            Some(fast) => fast,
            None => return error_reply(b"not_fast_bundle"),
        };
        let buyer_point = match compressed_public_key(&order.delivery_key) {
            Some(point) => point,
            None => return error_reply(b"bad_buyer_key"),
        };
        let context = order_context(
            &NETWORK_DIGEST,
            &own_address(),
            &order.bundle,
            id,
            order.nonce,
            &order.buyer,
            &buyer_point,
        );
        if !verify(&fast.point, &buyer_point, &context, &delivery) {
            return error_reply(b"invalid_delivery");
        }
        if let Some(code) = settle_order(id, &mut order, &bundle) {
            return error_reply(code);
        }
        write_value(FAST_DELIVERY_PREFIX, id, &delivery.encode());
        ok_reply()
    }
}

/// Fixed-width candidate record, returned as JSON for wallet clients.
#[no_mangle]
pub extern "C" fn fastBundleView(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len != 32 {
            return error_reply(b"bad_args");
        }
        let bundle: [u8; 32] = core::slice::from_raw_parts(bytes, 32).try_into().unwrap();
        let fast = match read_fast_bundle(&bundle) {
            Some(fast) => fast,
            None => return error_reply(b"fast_bundle_not_found"),
        };
        let mut out = [0u8; 380];
        let mut at = 0usize;
        push_bytes(&mut out, &mut at, b"{\"ok\":true,\"epoch\":");
        push_u64(&mut out, &mut at, u64::from(fast.epoch));
        push_bytes(&mut out, &mut at, b",\"count\":");
        push_u64(&mut out, &mut at, u64::from(fast.count));
        push_bytes(&mut out, &mut at, b",\"flags\":");
        push_u64(&mut out, &mut at, u64::from(fast.flags));
        push_bytes(&mut out, &mut at, b",\"copies\":\"");
        push_hex(&mut out, &mut at, &fast.copies);
        push_bytes(&mut out, &mut at, b"\",\"point\":\"");
        push_hex(&mut out, &mut at, &fast.point);
        push_bytes(&mut out, &mut at, b"\",\"originalKeys\":\"");
        push_hex(&mut out, &mut at, &fast.original_keys);
        push_bytes(&mut out, &mut at, b"\"}");
        reply(&out[..at])
    }
}

/// Included proof for the buyer; the order and bundle views provide the rest
/// of the transcript context used to check it and derive the content key.
#[no_mangle]
pub extern "C" fn fastDeliveryView(arg: u32) -> u32 {
    unsafe {
        let id = parse_u64(arg);
        let published = match read_value(FAST_DELIVERY_PREFIX, id) {
            Some(bytes) if bytes.len() == DELIVERY_BYTES => bytes,
            _ => return error_reply(b"delivery_not_found"),
        };
        let mut out = [0u8; 420];
        let mut at = 0usize;
        push_bytes(&mut out, &mut at, b"{\"ok\":true,\"proof\":\"");
        push_hex(&mut out, &mut at, published);
        push_bytes(&mut out, &mut at, b"\"}");
        reply(&out[..at])
    }
}

#[cfg(feature = "test-hooks")]
#[no_mangle]
pub extern "C" fn verifyFastContentFixture(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len != JOURNAL_BYTES + verifier::SEAL_BYTES {
            return error_reply(b"bad_args");
        }
        let journal = core::slice::from_raw_parts(bytes, JOURNAL_BYTES);
        let seal = core::slice::from_raw_parts(bytes.add(JOURNAL_BYTES), verifier::SEAL_BYTES);
        if !verifier::verify_fast_content(journal, seal) {
            return error_reply(b"invalid_proof");
        }
        ok_reply()
    }
}

#[cfg(feature = "test-hooks")]
#[no_mangle]
pub extern "C" fn seedFastBundle(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len != 32 + FAST_RECORD_BYTES {
            return error_reply(b"bad_args");
        }
        let bundle: [u8; 32] = core::slice::from_raw_parts(bytes, 32).try_into().unwrap();
        let record = core::slice::from_raw_parts(bytes.add(32), FAST_RECORD_BYTES);
        let fast = match FastBundle::decode(record) {
            Some(fast) => fast,
            None => return error_reply(b"bad_record"),
        };
        if registered_pool_bundle(&bundle).is_err() {
            return error_reply(b"bundle_not_found");
        }
        write_fast_bundle(&bundle, &fast);
        ok_reply()
    }
}

#[cfg(feature = "test-hooks")]
#[no_mangle]
pub extern "C" fn seedFastCertificate(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len <= 32 || len > 32 + FAST_REGISTER_HEADER + MAX_WIRE_BYTES {
            return error_reply(b"bad_args");
        }
        let bundle: [u8; 32] = core::slice::from_raw_parts(bytes, 32).try_into().unwrap();
        write_fast_certificate(&bundle, core::slice::from_raw_parts(bytes.add(32), len - 32));
        ok_reply()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fast_record_and_epoch_key_round_trip() {
        let fast = FastBundle {
            epoch: 230,
            count: 5,
            flags: 2,
            copies: [7; 32],
            point: [
                0x02, 0x79, 0xbe, 0x66, 0x7e, 0xf9, 0xdc, 0xbb, 0xac, 0x55, 0xa0, 0x62, 0x95, 0xce,
                0x87, 0x0b, 0x07, 0x02, 0x9b, 0xfc, 0xdb, 0x2d, 0xce, 0x28, 0xd9, 0x59, 0xf2, 0x81,
                0x5b, 0x16, 0xf8, 0x17, 0x98,
            ],
            original_keys: [3; 32],
        };
        assert_eq!(
            FastBundle::decode(&fast.encode()).unwrap().encode(),
            fast.encode()
        );
        assert_ne!(
            author_epoch_id(&[1; 20], 230),
            author_epoch_id(&[1; 20], 231)
        );
    }

    #[test]
    fn fast_certificate_key_is_directly_readable() {
        assert_eq!(
            &fast_certificate_key(&[0xab; 32]),
            b"pfcert/abababababababababababababababababababababababababababababababab"
        );
    }
}
