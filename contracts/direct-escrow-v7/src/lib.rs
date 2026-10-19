//! Ownerless queued sales of original Idena flip keys, version 7 candidate.
//!
//! The seed is funded only after one pinned public bundle is proved and queued,
//! and cannot be withdrawn. A private first sale pays its locked buyer price;
//! no payout is due until confirmation or valid public proof and reveal.
//! Buyer funds remain isolated pending liabilities until settlement or refund.
//! A buyer cannot select around the oldest first-sale or resale queue head.
//! Prices use one capped pool curve. Listings never reserve a future payout;
//! repeat sales pay the author half the locked buyer price and retain the rest.

#![no_std]
#![allow(static_mut_refs)]

extern crate alloc;

use alloc::vec::Vec;
use core::panic::PanicInfo;
use fod_fast_content_core::{Journal, JOURNAL_BYTES};
use fod_fast_session_core::{verify_with_roots, verify_with_roots_and_cids, MAX_WIRE_BYTES};

mod curve;
mod state;
#[path = "../../escrow/src/verifier.rs"]
mod verifier;

use state::{
    Offer, Order, ACTIVE, CANCELLED, FALLBACK_PENDING, FUNDED, QUALIFIED, QUEUED_PRIMARY,
    QUEUED_SECONDARY, REFUNDED, RETIRED, SETTLED, SOLD_PRIVATE,
};

const ADDRESS_BYTES: usize = 20;
const DIGEST_BYTES: usize = 32;
const AMOUNT_BYTES: usize = 16;
const DELIVERY_KEY_BYTES: usize = 65;
const SECRETS_BYTES: usize = 106;
const HEADER_READ_BYTES: usize = 16 * 1024;
const OFFER_METADATA_BYTES: usize = 2 + 4 + 32 + 32;
const OFFER_ARG_BYTES: usize = OFFER_METADATA_BYTES + 16;
const RELIST_ARG_BYTES: usize = 32 + 16;
const BUY_ARG_BYTES: usize = 32 + DELIVERY_KEY_BYTES;
const CONFIRM_ARG_BYTES: usize = 8 + 32;
const START_HEADER_BYTES: usize = 8 + JOURNAL_BYTES + verifier::SEAL_BYTES + 2;
const PROVE_HEADER_BYTES: usize = 32 + JOURNAL_BYTES + verifier::SEAL_BYTES + 2;
const QUALIFY_HEADER_BYTES: usize = OFFER_METADATA_BYTES + 2;
const MAX_SESSION_WIRE_BYTES: usize = u16::MAX as usize;
const REVEAL_ARG_BYTES: usize = 8 + SECRETS_BYTES;
/// At the observed roughly 20-second mainnet cadence this is about 72 hours.
/// The exact on-chain boundary is a block height, shown to buyers and sellers.
const PROOF_WINDOW_BLOCKS: u64 = 12_960;
/// At roughly 20 seconds per block, an unsold head expires after seven days.
const QUEUE_HEAD_TIMEOUT_BLOCKS: u64 = 30_240;
const BUYER_CONFIRM_GRACE_BLOCKS: u64 = 30;
const PROOF_DEADLINE_MARGIN_BLOCKS: u64 = 120;
const MAX_FLIPS: u32 = 32;
const PUBLIC_CID_BYTES: usize = 59;
const IDNA_BASE: u128 = 1_000_000_000_000_000_000;
const PILOT_SEED: u128 = 10_000 * IDNA_BASE;
/// Reproduced content guest image. New receipts bind to this contract address.
pub const FAST_CONTENT_V7_IMAGE_ID: [u8; 32] = [
    0x4f, 0xd1, 0x24, 0xc8, 0xcb, 0x91, 0x37, 0x0d, 0xab, 0x60, 0xd3, 0xe1, 0xd7, 0xc4, 0xe6, 0x66,
    0xe7, 0x94, 0x85, 0xe2, 0x10, 0xb5, 0xf2, 0x0e, 0xd3, 0x6c, 0xc2, 0x10, 0x48, 0x83, 0x72, 0x6c,
];
/// The final direct-v6 deployment used the identical content guest. A sealed
/// epoch-230 receipt from it may prove original content before a v7 sale, but
/// cannot serve as a v7 encrypted-delivery or funded-fallback receipt.
const LEGACY_V6_CONTENT_CONTRACT: [u8; 20] = [
    0x92, 0xf6, 0x64, 0xdb, 0x85, 0x80, 0x5d, 0x70, 0x70, 0x16, 0xfe, 0xbb, 0xfa, 0x55, 0xbd, 0x2b,
    0xea, 0xaf, 0x80, 0x8b,
];
const LEGACY_V6_CONTENT_EPOCH: u16 = 230;
const SECRETS_DOMAIN: &[u8] = b"fod:epoch-secrets:v1";
const OFFER_DOMAIN: &[u8] = b"fod:pool-offer:v7";
const OFFER_PREFIX: &[u8] = b"pool/offer/";
const INVENTORY_PREFIX: &[u8] = b"pool/inventory/";
const ORDER_PREFIX: &[u8] = b"pool/order/";
const REVEAL_PREFIX: &[u8] = b"pool/reveal/";
const PROOF_RATE_PREFIX: &[u8] = b"pool/proof-rate/";
const NEXT_ORDER_KEY: &[u8] = b"pool/next-order";
const VALUE_POLICY_KEY: &[u8] = b"pool/value-policy";
const NEXT_SEQUENCE_KEY: &[u8] = b"pool/next-sequence";
const PENDING_KEY: &[u8] = b"pool/pending";
const RESERVE_KEY: &[u8] = b"pool/reserve";
const ACTIVATED_AT_KEY: &[u8] = b"pool/activated-at";
const PRIMARY_UNITS_KEY: &[u8] = b"pool/primary-units";
const SECONDARY_UNITS_KEY: &[u8] = b"pool/secondary-units";
const INSURANCE_KEY: &[u8] = b"pool/insurance";
const POOL_UNITS_KEY: &[u8] = b"pool/units";
const SEED_COUNT_KEY: &[u8] = b"pool/seed-count";
const SEED_ID_KEY: &[u8] = b"pool/seed-id";
const RESERVED_UNITS_KEY: &[u8] = b"pool/reserved-units";
const PUBLIC_SEED_PREFIX: &[u8] = b"pool/public-seed/";
const SELLER_LISTING_PREFIX: &[u8] = b"pool/seller/";
const SELLER_BAN_PREFIX: &[u8] = b"pool/ban/";
const PRIMARY_HEAD_KEY: &[u8] = b"pool/primary-head";
const PRIMARY_TAIL_KEY: &[u8] = b"pool/primary-tail";
const SECONDARY_HEAD_KEY: &[u8] = b"pool/secondary-head";
const SECONDARY_TAIL_KEY: &[u8] = b"pool/secondary-tail";
const HEAD_SINCE_KEY: &[u8] = b"pool/head-since";
/// The buyer locks a reserve for 2.5 million gas at twice the fee rate when
/// the buy lands. A later fee increase above that margin remains an economic risk.
const BOND_GAS_BUDGET: u128 = 2_500_000;
const BOND_FEE_MARGIN: u128 = 2;
/// The release gate must measure the complete fallback transaction below this
/// allowance. The rest of the buyer's fee-rise reserve is never seller revenue.
const FALLBACK_REIMBURSE_GAS_BUDGET: u128 = 2_500_000;
const SELLER_PROOF_GAS_BUDGET: u128 = 2_500_000;
const SELLER_BOND_FEE_MARGIN: u128 = 2;
/// Unproved original content locks the largest possible buyer price until
/// independent proof or forfeiture. Pre-proving avoids this collateral.
const UNPROVED_FRAUD_COLLATERAL: u128 = curve::PRICE_CAP;
const FIRST_DEFAULT_BAN_EPOCHS: u32 = 10;
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

#[cfg(target_arch = "wasm32")]
#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    core::arch::wasm32::unreachable()
}

#[cfg(target_arch = "wasm32")]
mod bump {
    use core::alloc::{GlobalAlloc, Layout};
    use core::sync::atomic::{AtomicUsize, Ordering};

    const HEAP_SIZE: usize = 1024 * 1024;
    static mut HEAP: [u8; HEAP_SIZE] = [0; HEAP_SIZE];
    static NEXT: AtomicUsize = AtomicUsize::new(0);
    pub struct Bump;

    unsafe impl GlobalAlloc for Bump {
        unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
            let align = layout.align().max(8);
            let start = NEXT.load(Ordering::SeqCst);
            let aligned = (start + align - 1) & !(align - 1);
            let end = aligned.saturating_add(layout.size());
            if end > HEAP_SIZE {
                return core::ptr::null_mut();
            }
            NEXT.store(end, Ordering::SeqCst);
            (core::ptr::addr_of_mut!(HEAP) as *mut u8).add(aligned)
        }
        unsafe fn dealloc(&self, _ptr: *mut u8, _layout: Layout) {}
    }

    #[global_allocator]
    pub static ALLOCATOR: Bump = Bump;
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct Region {
    offset: u32,
    capacity: u32,
    length: u32,
}

// A full bounded session wire follows the fixed proof payload.
const ALLOC_ARENA_SIZE: usize = 128 * 1024;
const KEY_BUF_SIZE: usize = 96;
const VAL_BUF_SIZE: usize = 1 + MAX_FLIPS as usize * (1 + PUBLIC_CID_BYTES);
const READ_BUF_SIZE: usize = 1024;
const HEAP_SIZE: usize = 512;
static mut ALLOC_ARENA: [u8; ALLOC_ARENA_SIZE] = [0; ALLOC_ARENA_SIZE];
static mut ALLOC_NEXT: usize = 0;
static mut LAST_ALLOC_REGION: u32 = 0;
static mut KEY_BUF: [u8; KEY_BUF_SIZE] = [0; KEY_BUF_SIZE];
static mut KEY_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};
static mut VAL_BUF: [u8; VAL_BUF_SIZE] = [0; VAL_BUF_SIZE];
static mut VAL_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};
static mut READ_BUF: [u8; READ_BUF_SIZE] = [0; READ_BUF_SIZE];
static mut REPLY_BUF: [u8; 16] = [0; 16];
static mut REPLY_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};
static mut HEAP: [u8; HEAP_SIZE] = [0; HEAP_SIZE];
static mut HEAP_NEXT: usize = 0;

#[link(wasm_import_module = "env")]
extern "C" {
    fn set_storage(key: u32, value: u32);
    fn get_storage(key: u32) -> u32;
    fn block_number() -> u64;
    fn caller() -> u32;
    fn own_addr() -> u32;
    fn pay_amount() -> u32;
    fn balance() -> u32;
    fn create_transfer_promise(addr: u32, amount: u32);
    fn block_header(height: u64) -> u32;
    fn min_fee_per_gas() -> u32;
    fn epoch() -> u32;
}

unsafe fn point_region(region: *mut Region, data: *mut u8, capacity: u32, length: u32) -> u32 {
    (*region).offset = data as u32;
    (*region).capacity = capacity;
    (*region).length = length;
    region as u32
}

#[no_mangle]
pub extern "C" fn allocate(size: u32) -> u32 {
    unsafe {
        // The pinned runtime passes the region returned by its preparatory
        // allocation as the argument to a directly called export. Reject a
        // transaction that targets this required allocator itself.
        if (size == 0 && LAST_ALLOC_REGION == 0) || size == LAST_ALLOC_REGION {
            refused();
        }
        let start = (ALLOC_NEXT + 3) & !3;
        let data = start + core::mem::size_of::<Region>();
        let end = match data.checked_add(size as usize) {
            Some(end) if end <= ALLOC_ARENA_SIZE => end,
            _ => return 0,
        };
        ALLOC_NEXT = end;
        let base = core::ptr::addr_of_mut!(ALLOC_ARENA) as *mut u8;
        let region = base.add(start) as *mut Region;
        (*region).offset = base.add(data) as u32;
        (*region).capacity = size;
        (*region).length = 0;
        LAST_ALLOC_REGION = region as u32;
        LAST_ALLOC_REGION
    }
}

unsafe fn read_region(ptr: u32) -> Option<&'static [u8]> {
    if ptr == 0 {
        return None;
    }
    let region = ptr as *const Region;
    if (*region).offset == 0 {
        return None;
    }
    Some(core::slice::from_raw_parts(
        (*region).offset as *const u8,
        (*region).length as usize,
    ))
}

unsafe fn heap_region(bytes: &[u8]) -> u32 {
    let start = (HEAP_NEXT + 3) & !3;
    let data = start + core::mem::size_of::<Region>();
    let end = data + bytes.len();
    if end > HEAP_SIZE {
        return 0;
    }
    HEAP_NEXT = end;
    let base = core::ptr::addr_of_mut!(HEAP) as *mut u8;
    let region = base.add(start) as *mut Region;
    core::ptr::copy_nonoverlapping(bytes.as_ptr(), base.add(data), bytes.len());
    (*region).offset = base.add(data) as u32;
    (*region).capacity = bytes.len() as u32;
    (*region).length = bytes.len() as u32;
    region as u32
}

unsafe fn address(ptr: u32) -> Option<[u8; ADDRESS_BYTES]> {
    let value = read_region(ptr)?;
    value.try_into().ok()
}

unsafe fn amount(ptr: u32) -> Option<u128> {
    let value = read_region(ptr)?;
    if value.len() > AMOUNT_BYTES {
        return None;
    }
    let mut out = [0u8; AMOUNT_BYTES];
    out[AMOUNT_BYTES - value.len()..].copy_from_slice(value);
    Some(u128::from_be_bytes(out))
}

unsafe fn key(prefix: &[u8], suffix: &[u8]) -> u32 {
    let len = prefix.len() + suffix.len();
    if len > KEY_BUF_SIZE {
        return 0;
    }
    KEY_BUF[..prefix.len()].copy_from_slice(prefix);
    KEY_BUF[prefix.len()..len].copy_from_slice(suffix);
    point_region(
        core::ptr::addr_of_mut!(KEY_REGION),
        core::ptr::addr_of_mut!(KEY_BUF) as *mut u8,
        KEY_BUF_SIZE as u32,
        len as u32,
    )
}

unsafe fn order_id_key(prefix: &[u8], id: u64) -> u32 {
    let mut digits = [0u8; 20];
    let mut n = 0;
    let mut value = id;
    loop {
        digits[n] = b'0' + (value % 10) as u8;
        n += 1;
        value /= 10;
        if value == 0 {
            break;
        }
    }
    digits[..n].reverse();
    key(prefix, &digits[..n])
}

unsafe fn order_key(id: u64) -> u32 {
    order_id_key(ORDER_PREFIX, id)
}

unsafe fn reveal_key(id: u64) -> u32 {
    order_id_key(REVEAL_PREFIX, id)
}

unsafe fn public_seed_key(id: &[u8; DIGEST_BYTES]) -> u32 {
    key(PUBLIC_SEED_PREFIX, id)
}

unsafe fn proof_rate_key(id: u64) -> u32 {
    order_id_key(PROOF_RATE_PREFIX, id)
}

unsafe fn read_keyed(storage_key: u32) -> Option<&'static [u8]> {
    if storage_key == 0 {
        return None;
    }
    let src = read_region(get_storage(storage_key))?;
    if src.is_empty() || src.len() > READ_BUF_SIZE {
        return None;
    }
    READ_BUF[..src.len()].copy_from_slice(src);
    Some(&READ_BUF[..src.len()])
}

unsafe fn write_keyed(storage_key: u32, value: &[u8]) {
    if storage_key == 0 || value.len() > VAL_BUF_SIZE {
        refused();
    }
    VAL_BUF[..value.len()].copy_from_slice(value);
    let region = point_region(
        core::ptr::addr_of_mut!(VAL_REGION),
        core::ptr::addr_of_mut!(VAL_BUF) as *mut u8,
        VAL_BUF_SIZE as u32,
        value.len() as u32,
    );
    set_storage(storage_key, region);
}

unsafe fn read_offer(id: &[u8; DIGEST_BYTES]) -> Option<Offer> {
    Offer::decode(read_keyed(key(OFFER_PREFIX, id))?)
}

unsafe fn write_offer(id: &[u8; DIGEST_BYTES], offer: &Offer) {
    write_keyed(key(OFFER_PREFIX, id), &offer.encode());
}

unsafe fn seller_listing(author: &[u8; ADDRESS_BYTES]) -> [u8; 32] {
    read_keyed(key(SELLER_LISTING_PREFIX, author))
        .and_then(|raw| raw.try_into().ok())
        .unwrap_or([0; 32])
}

unsafe fn close_seller_listing(author: &[u8; ADDRESS_BYTES], id: &[u8; 32]) {
    if seller_listing(author) != *id {
        refused();
    }
    write_keyed(key(SELLER_LISTING_PREFIX, author), &[0; 32]);
}

unsafe fn seller_banned(author: &[u8; ADDRESS_BYTES]) -> bool {
    let Some(raw) = read_keyed(key(SELLER_BAN_PREFIX, author)) else {
        return false;
    };
    if raw.len() != 5 || raw[0] > 2 {
        refused();
    }
    raw[0] == 2 || (raw[0] == 1 && epoch() < u32::from_be_bytes(raw[1..5].try_into().unwrap()))
}

unsafe fn note_seller_default(author: &[u8; ADDRESS_BYTES]) {
    let prior = read_keyed(key(SELLER_BAN_PREFIX, author))
        .map(|raw| {
            if raw.len() != 5 || raw[0] > 2 {
                refused();
            }
            raw[0]
        })
        .unwrap_or(0);
    let strikes = if prior == 0 { 1 } else { 2 };
    let until = if strikes == 1 {
        epoch()
            .checked_add(FIRST_DEFAULT_BAN_EPOCHS)
            .unwrap_or_else(|| refused())
    } else {
        0
    };
    let mut record = [0u8; 5];
    record[0] = strikes;
    record[1..5].copy_from_slice(&until.to_be_bytes());
    write_keyed(key(SELLER_BAN_PREFIX, author), &record);
}

unsafe fn read_order(id: u64) -> Option<Order> {
    Order::decode(read_keyed(order_key(id))?)
}

unsafe fn write_order(id: u64, order: &Order) {
    write_keyed(order_key(id), &order.encode());
}

unsafe fn read_proof_rate(id: u64) -> Option<u128> {
    let raw = read_keyed(proof_rate_key(id))?;
    let rate = u128::from_be_bytes(raw.try_into().ok()?);
    if rate == 0 {
        return None;
    }
    Some(rate)
}

unsafe fn read_u128(key_name: &[u8]) -> u128 {
    read_keyed(key(key_name, b""))
        .and_then(|raw| raw.try_into().ok())
        .map(u128::from_be_bytes)
        .unwrap_or(0)
}

unsafe fn write_u128(key_name: &[u8], value: u128) {
    write_keyed(key(key_name, b""), &value.to_be_bytes());
}

unsafe fn read_u64(key_name: &[u8]) -> u64 {
    read_keyed(key(key_name, b""))
        .and_then(|raw| raw.try_into().ok())
        .map(u64::from_be_bytes)
        .unwrap_or(0)
}

unsafe fn write_u64(key_name: &[u8], value: u64) {
    write_keyed(key(key_name, b""), &value.to_be_bytes());
}

unsafe fn read_id(key_name: &[u8]) -> [u8; 32] {
    read_keyed(key(key_name, b""))
        .and_then(|raw| raw.try_into().ok())
        .unwrap_or([0; 32])
}

unsafe fn write_id(key_name: &[u8], value: &[u8; 32]) {
    write_keyed(key(key_name, b""), value);
}

unsafe fn require_solvency(reserve: u128, pending: u128) {
    let liabilities = reserve.checked_add(pending).unwrap_or_else(|| refused());
    if amount(balance()).unwrap_or(0) < liabilities {
        refused();
    }
}

// The node omits a zero Amount from serialized transactions, so pay_amount
// traps when absent. State-only actions require one base unit and return it
// to the immediate caller. Check that liabilities remain covered after refund.
unsafe fn refund_action_sentinel() {
    if amount(pay_amount()) != Some(1) {
        refused();
    }
    let liabilities = read_u128(RESERVE_KEY)
        .checked_add(read_u128(PENDING_KEY))
        .and_then(|value| value.checked_add(1))
        .unwrap_or_else(|| refused());
    if amount(balance()).unwrap_or(0) < liabilities {
        refused();
    }
    let sender = address(caller()).unwrap_or_else(|| refused());
    transfer(&sender, 1);
}

unsafe fn virtual_pool() -> (u64, u128) {
    let units = read_u64(PRIMARY_UNITS_KEY)
        .checked_add(read_u64(SECONDARY_UNITS_KEY))
        .unwrap_or_else(|| refused());
    let reserve = read_u128(RESERVE_KEY);
    let insurance = read_u128(INSURANCE_KEY);
    // Held buyer prices and seller collateral never enter the pricing reserve.
    // Slashed fraud collateral is solvent insurance, not curve liquidity.
    let virtual_reserve = reserve.checked_sub(insurance).unwrap_or_else(|| refused());
    if virtual_reserve == 0 {
        refused();
    }
    (units, virtual_reserve)
}

unsafe fn next_order() -> u64 {
    read_u64(NEXT_ORDER_KEY)
}

fn queue_keys(status: u8) -> (&'static [u8], &'static [u8], &'static [u8]) {
    match status {
        QUEUED_PRIMARY => (PRIMARY_HEAD_KEY, PRIMARY_TAIL_KEY, PRIMARY_UNITS_KEY),
        QUEUED_SECONDARY => (SECONDARY_HEAD_KEY, SECONDARY_TAIL_KEY, SECONDARY_UNITS_KEY),
        _ => refused(),
    }
}

unsafe fn enqueue(id: &[u8; 32], offer: &mut Offer) {
    let old_head = oldest_offer_id();
    let (head_key, tail_key, units_key) = queue_keys(offer.status);
    let tail = read_id(tail_key);
    offer.previous = tail;
    offer.next = [0; 32];
    if tail == [0; 32] {
        if read_id(head_key) != [0; 32] {
            refused();
        }
        write_id(head_key, id);
    } else {
        let mut last = read_offer(&tail).unwrap_or_else(|| refused());
        if last.status != offer.status || last.next != [0; 32] {
            refused();
        }
        last.next = *id;
        write_offer(&tail, &last);
    }
    let sequence = read_u64(NEXT_SEQUENCE_KEY);
    offer.sequence = sequence;
    offer.listed_at = block_number();
    write_u64(
        NEXT_SEQUENCE_KEY,
        sequence.checked_add(1).unwrap_or_else(|| refused()),
    );
    write_id(tail_key, id);
    write_u64(
        units_key,
        read_u64(units_key)
            .checked_add(1)
            .unwrap_or_else(|| refused()),
    );
    note_head_change(old_head);
}

unsafe fn dequeue(id: &[u8; 32], offer: &mut Offer) {
    if !offer.queued() {
        refused();
    }
    let old_head = oldest_offer_id();
    let (head_key, tail_key, units_key) = queue_keys(offer.status);
    if offer.previous == [0; 32] {
        if read_id(head_key) != *id {
            refused();
        }
        write_id(head_key, &offer.next);
    } else {
        let mut previous = read_offer(&offer.previous).unwrap_or_else(|| refused());
        if previous.next != *id || previous.status != offer.status {
            refused();
        }
        previous.next = offer.next;
        write_offer(&offer.previous, &previous);
    }
    if offer.next == [0; 32] {
        if read_id(tail_key) != *id {
            refused();
        }
        write_id(tail_key, &offer.previous);
    } else {
        let mut next = read_offer(&offer.next).unwrap_or_else(|| refused());
        if next.previous != *id || next.status != offer.status {
            refused();
        }
        next.previous = offer.previous;
        write_offer(&offer.next, &next);
    }
    offer.previous = [0; 32];
    offer.next = [0; 32];
    write_u64(
        units_key,
        read_u64(units_key)
            .checked_sub(1)
            .unwrap_or_else(|| refused()),
    );
    note_head_change(old_head);
}

fn priority_head(primary: [u8; 32], secondary: [u8; 32]) -> [u8; 32] {
    if primary != [0; 32] {
        primary
    } else {
        secondary
    }
}

unsafe fn oldest_offer_id() -> [u8; 32] {
    priority_head(read_id(PRIMARY_HEAD_KEY), read_id(SECONDARY_HEAD_KEY))
}

unsafe fn note_head_change(previous: [u8; 32]) {
    if oldest_offer_id() != previous {
        write_u64(HEAD_SINCE_KEY, block_number());
    }
}

fn offer_id(author: &[u8; ADDRESS_BYTES], epoch: u16) -> [u8; DIGEST_BYTES] {
    let mut data = [0u8; OFFER_DOMAIN.len() + ADDRESS_BYTES + 2];
    data[..OFFER_DOMAIN.len()].copy_from_slice(OFFER_DOMAIN);
    data[OFFER_DOMAIN.len()..OFFER_DOMAIN.len() + ADDRESS_BYTES].copy_from_slice(author);
    data[OFFER_DOMAIN.len() + ADDRESS_BYTES..].copy_from_slice(&epoch.to_be_bytes());
    verifier::sha256(&data)
}

unsafe fn required_bond() -> u128 {
    let rate = amount(min_fee_per_gas()).unwrap_or_else(|| refused());
    if rate == 0 {
        refused();
    }
    rate.checked_mul(BOND_GAS_BUDGET)
        .and_then(|value| value.checked_mul(BOND_FEE_MARGIN))
        .unwrap_or_else(|| refused())
}

unsafe fn required_seller_bond() -> u128 {
    let rate = amount(min_fee_per_gas()).unwrap_or_else(|| refused());
    if rate == 0 {
        refused();
    }
    seller_bond_for_rate(rate).unwrap_or_else(|| refused())
}

fn seller_bond_for_rate(rate: u128) -> Option<u128> {
    rate.checked_mul(SELLER_PROOF_GAS_BUDGET)?
        .checked_mul(SELLER_BOND_FEE_MARGIN)
}

fn fallback_reimbursement(bond: u128, rate: u128) -> u128 {
    match rate.checked_mul(FALLBACK_REIMBURSE_GAS_BUDGET) {
        Some(cost) => bond.min(cost),
        None => bond,
    }
}

fn funded_content_proof_allowed(offer: &Offer, id: &[u8; 32], order: &Order, height: u64) -> bool {
    let valid_state = if offer.status == SOLD_PRIVATE {
        order.state == SETTLED
    } else {
        offer.status == ACTIVE && matches!(order.state, FUNDED | FALLBACK_PENDING)
    };
    offer.open_order != 0
        && order.offer == *id
        && valid_state
        && order
            .funded_at
            .checked_add(PROOF_WINDOW_BLOCKS)
            .is_some_and(|deadline| height < deadline)
}

fn u32_at(bytes: &[u8], at: usize) -> u32 {
    u32::from_be_bytes(bytes[at..at + 4].try_into().unwrap())
}

fn u64_at(bytes: &[u8], at: usize) -> u64 {
    u64::from_be_bytes(bytes[at..at + 8].try_into().unwrap())
}

fn statement_matches_offer(journal: &Journal, offer: &Offer, contract: &[u8; 20]) -> bool {
    journal.contract == *contract
        && journal.author == offer.author
        && journal.epoch == offer.epoch
        && u32::from(journal.count) == offer.covered
        && offer.covered > 0
        && offer.covered <= MAX_FLIPS
        && journal.original_keys == offer.keys
        && journal.original_inventory == offer.inventory
        && journal.policy == POLICY_DIGEST
        && journal.consensus == CONSENSUS_DIGEST
        && journal.network == NETWORK_DIGEST
}

fn can_use_legacy_content_proof(offer: &Offer) -> bool {
    offer.epoch == LEGACY_V6_CONTENT_EPOCH
        && offer.status == QUALIFIED
        && !offer.content_proven
        && !offer.public_seed
        && !offer.ever_sold
        && offer.open_order == 0
}

/// A valid content seal does not attest that validation later accepted every
/// original CID. The public witness supplies that independent session check.
fn fallback_claim_matches<F>(
    journal_bytes: &[u8],
    seal: &[u8],
    wire: &[u8],
    offer: &Offer,
    contract: &[u8; 20],
    require_clean: bool,
    root_at: F,
) -> bool
where
    F: FnMut(u64) -> Option<[u8; 32]>,
{
    let Some(journal) = Journal::decode(journal_bytes) else {
        return false;
    };
    if !statement_matches_offer(&journal, offer, contract) {
        return false;
    }
    let Ok(verdict) = verify_with_roots(
        wire,
        &journal.author,
        journal.epoch,
        journal.count,
        &journal.original_inventory,
        root_at,
    ) else {
        return false;
    };
    verdict.flips == offer.covered
        && (!require_clean || verdict.flags == 0)
        && verifier::verify_fast_content_image(journal_bytes, seal, &FAST_CONTENT_V7_IMAGE_ID)
}

fn parse_start_arg(raw: &[u8]) -> Option<(u64, &[u8], &[u8], &[u8])> {
    if raw.len() < START_HEADER_BYTES || raw.len() > START_HEADER_BYTES + MAX_SESSION_WIRE_BYTES {
        return None;
    }
    let wire_len = u16::from_be_bytes(
        raw[START_HEADER_BYTES - 2..START_HEADER_BYTES]
            .try_into()
            .ok()?,
    ) as usize;
    if wire_len == 0 || wire_len > MAX_WIRE_BYTES || raw.len() != START_HEADER_BYTES + wire_len {
        return None;
    }
    Some((
        u64_at(raw, 0),
        &raw[8..8 + JOURNAL_BYTES],
        &raw[8 + JOURNAL_BYTES..START_HEADER_BYTES - 2],
        &raw[START_HEADER_BYTES..],
    ))
}

fn parse_prove_arg(raw: &[u8]) -> Option<([u8; 32], &[u8], &[u8], &[u8])> {
    if raw.len() < PROVE_HEADER_BYTES || raw.len() > PROVE_HEADER_BYTES + MAX_SESSION_WIRE_BYTES {
        return None;
    }
    let wire_len = u16::from_be_bytes(
        raw[PROVE_HEADER_BYTES - 2..PROVE_HEADER_BYTES]
            .try_into()
            .ok()?,
    ) as usize;
    if wire_len == 0 || wire_len > MAX_WIRE_BYTES || raw.len() != PROVE_HEADER_BYTES + wire_len {
        return None;
    }
    Some((
        raw[..32].try_into().ok()?,
        &raw[32..32 + JOURNAL_BYTES],
        &raw[32 + JOURNAL_BYTES..PROVE_HEADER_BYTES - 2],
        &raw[PROVE_HEADER_BYTES..],
    ))
}

fn parse_qualification_arg(raw: &[u8]) -> Option<(&[u8], &[u8])> {
    if raw.len() < QUALIFY_HEADER_BYTES || raw.len() > QUALIFY_HEADER_BYTES + MAX_SESSION_WIRE_BYTES
    {
        return None;
    }
    let wire_len = u16::from_be_bytes(
        raw[QUALIFY_HEADER_BYTES - 2..QUALIFY_HEADER_BYTES]
            .try_into()
            .ok()?,
    ) as usize;
    if wire_len == 0 || wire_len > MAX_WIRE_BYTES || raw.len() != QUALIFY_HEADER_BYTES + wire_len {
        return None;
    }
    Some((&raw[..OFFER_METADATA_BYTES], &raw[QUALIFY_HEADER_BYTES..]))
}

fn offer_metadata(raw: &[u8]) -> Option<(u16, u32, [u8; 32], [u8; 32])> {
    if raw.len() < OFFER_METADATA_BYTES {
        return None;
    }
    Some((
        u16::from_be_bytes(raw[0..2].try_into().ok()?),
        u32_at(raw, 2),
        raw[6..38].try_into().ok()?,
        raw[38..70].try_into().ok()?,
    ))
}

fn secrets_match_offer(secrets: &[u8], offer: &Offer) -> bool {
    secrets.len() == SECRETS_BYTES
        && secrets[..SECRETS_DOMAIN.len()] == *SECRETS_DOMAIN
        && secrets[SECRETS_DOMAIN.len()..SECRETS_DOMAIN.len() + 2] == offer.epoch.to_be_bytes()
        && secrets[SECRETS_DOMAIN.len() + 2..SECRETS_DOMAIN.len() + 22] == offer.author
        && verifier::sha256(secrets) == offer.keys
}

fn protobuf_varint(message: &[u8], at: &mut usize) -> Option<u64> {
    let mut value = 0u64;
    let mut shift = 0;
    loop {
        let b = *message.get(*at)?;
        *at += 1;
        if shift >= 64 {
            return None;
        }
        value |= u64::from(b & 0x7f) << shift;
        if b & 0x80 == 0 {
            return Some(value);
        }
        shift += 7;
    }
}

fn protobuf_field(message: &[u8], wanted: u32) -> Option<&[u8]> {
    let mut at = 0;
    let mut found = None;
    while at < message.len() {
        let tag = protobuf_varint(message, &mut at)?;
        let field = (tag >> 3) as u32;
        if field == 0 {
            return None;
        }
        match tag & 7 {
            0 => {
                protobuf_varint(message, &mut at)?;
            }
            1 => at = at.checked_add(8)?,
            2 => {
                let len = protobuf_varint(message, &mut at)? as usize;
                let end = at.checked_add(len)?;
                let value = message.get(at..end)?;
                at = end;
                if field == wanted {
                    found = Some(value);
                }
            }
            5 => at = at.checked_add(4)?,
            _ => return None,
        }
        if at > message.len() {
            return None;
        }
    }
    found
}

unsafe fn header_state_root(height: u64) -> Option<[u8; 32]> {
    let header = read_region(block_header(height))?;
    if header.is_empty() || header.len() > HEADER_READ_BYTES {
        return None;
    }
    let (shape, sub) = match protobuf_field(header, 1) {
        Some(sub) => (1, sub),
        None => (2, protobuf_field(header, 2)?),
    };
    let root = protobuf_field(sub, if shape == 1 { 6 } else { 3 })?;
    root.try_into().ok()
}

unsafe fn transfer(to: &[u8; 20], value: u128) {
    let addr = heap_region(to);
    let amount = heap_region(&value.to_be_bytes());
    if addr == 0 || amount == 0 {
        refused();
    }
    create_transfer_promise(addr, amount);
}

fn sale_payout(offer: &Offer, paid: u128) -> u128 {
    if offer.ever_sold {
        paid / 2
    } else {
        paid
    }
}

fn sale_reserve_credit(offer: &Offer, paid: u128) -> u128 {
    paid - sale_payout(offer, paid)
}

fn fraud_collateral_required(content_proven: bool) -> u128 {
    if content_proven {
        0
    } else {
        UNPROVED_FRAUD_COLLATERAL
    }
}

fn full_default_refund(order: &Order, seller_bond: u128) -> Option<u128> {
    order.paid.checked_add(order.bond)?.checked_add(seller_bond)
}

unsafe fn settle(id: u64, order: &mut Order, offer: &mut Offer, retired: bool) {
    let expected_state = if retired { FALLBACK_PENDING } else { FUNDED };
    if offer.status != ACTIVE
        || offer.open_order != id
        || order.state != expected_state
        || order.offer != offer_id(&offer.author, offer.epoch)
        || order.seller_payout != sale_payout(offer, order.paid)
        || offer.seller_bond == 0
        || offer.public_seed
    {
        refused();
    }
    let released = order
        .paid
        .checked_add(order.bond)
        .and_then(|sum| sum.checked_add(offer.seller_bond))
        .and_then(|sum| sum.checked_add(if retired { offer.fraud_collateral } else { 0 }))
        .unwrap_or_else(|| refused());
    let total = order
        .paid
        .checked_add(order.bond)
        .and_then(|sum| sum.checked_add(offer.seller_bond))
        .and_then(|sum| sum.checked_add(offer.fraud_collateral))
        .unwrap_or_else(|| refused());
    let pending = read_u128(PENDING_KEY);
    let reserve = read_u128(RESERVE_KEY);
    if order.bond == 0 || pending < total {
        refused();
    }
    require_solvency(reserve, pending);
    let next_reserve = reserve
        .checked_add(sale_reserve_credit(offer, order.paid))
        .unwrap_or_else(|| refused());
    if next_reserve == 0 {
        refused();
    }
    let reimbursement = if retired && order.challenged {
        read_proof_rate(id)
            .map(|rate| fallback_reimbursement(order.bond, rate))
            .unwrap_or(0)
    } else {
        0
    };
    let returned_bond = order.bond - reimbursement;
    let seller_payout = order
        .seller_payout
        .checked_add(offer.seller_bond)
        .and_then(|sum| sum.checked_add(if retired { offer.fraud_collateral } else { 0 }))
        .and_then(|sum| sum.checked_add(reimbursement))
        .unwrap_or_else(|| refused());
    write_u128(PENDING_KEY, pending - released);
    write_u128(RESERVE_KEY, next_reserve);
    write_u64(
        POOL_UNITS_KEY,
        read_u64(POOL_UNITS_KEY)
            .checked_sub(1)
            .unwrap_or_else(|| refused()),
    );
    write_u64(
        RESERVED_UNITS_KEY,
        read_u64(RESERVED_UNITS_KEY)
            .checked_sub(1)
            .unwrap_or_else(|| refused()),
    );
    order.state = SETTLED;
    offer.open_order = 0;
    offer.seller_bond = 0;
    if retired {
        offer.content_proven = true;
        offer.fraud_collateral = 0;
    }
    offer.ever_sold = true;
    offer.status = if retired { RETIRED } else { SOLD_PRIVATE };
    // An unproved private settlement keeps its order id until content is
    // proved or slashed, so the proof deadline cannot be bypassed.
    if !retired && !offer.content_proven {
        offer.open_order = id;
    }
    write_order(id, order);
    write_offer(&order.offer, offer);
    if retired || offer.content_proven {
        close_seller_listing(&offer.author, &order.offer);
    }
    transfer(&offer.author, seller_payout);
    if returned_bond > 0 {
        transfer(&order.buyer, returned_bond);
    }
}

unsafe fn queue_offer(id: &[u8; 32], offer: &mut Offer, minimum_payout: u128) {
    let reserve = read_u128(RESERVE_KEY);
    let pending = read_u128(PENDING_KEY);
    if read_u64(SEED_COUNT_KEY) != 1
        || !matches!(offer.status, QUEUED_PRIMARY | QUEUED_SECONDARY)
        || offer.public_seed
        || seller_banned(&offer.author)
        || seller_listing(&offer.author) != [0; 32]
    {
        refused();
    }
    let (units, virtual_reserve) = virtual_pool();
    if offer.ever_sold && !offer.content_proven {
        refused();
    }
    let next_units = units.checked_add(1).unwrap_or_else(|| refused());
    let first_buyer_quote =
        curve::buy_quote(next_units, virtual_reserve).unwrap_or_else(|| refused());
    let current_seller_quote = if offer.ever_sold {
        first_buyer_quote / 2
    } else {
        first_buyer_quote
    };
    if current_seller_quote < minimum_payout {
        refused();
    }
    let posted = amount(pay_amount()).unwrap_or_else(|| refused());
    let fraud_collateral = fraud_collateral_required(offer.content_proven);
    let delivery_bond = posted
        .checked_sub(fraud_collateral)
        .unwrap_or_else(|| refused());
    if delivery_bond < required_seller_bond() {
        refused();
    }
    let next_pending = pending.checked_add(posted).unwrap_or_else(|| refused());
    require_solvency(reserve, next_pending);
    offer.seller_bond = delivery_bond;
    offer.fraud_collateral = fraud_collateral;
    enqueue(id, offer);
    write_offer(id, offer);
    write_keyed(key(SELLER_LISTING_PREFIX, &offer.author), id);
    write_u128(PENDING_KEY, next_pending);
    write_u64(
        POOL_UNITS_KEY,
        read_u64(POOL_UNITS_KEY)
            .checked_add(1)
            .unwrap_or_else(|| refused()),
    );
}

unsafe fn ok() -> u32 {
    let value = b"{\"ok\":true}";
    REPLY_BUF[..value.len()].copy_from_slice(value);
    point_region(
        core::ptr::addr_of_mut!(REPLY_REGION),
        core::ptr::addr_of_mut!(REPLY_BUF) as *mut u8,
        REPLY_BUF.len() as u32,
        value.len() as u32,
    )
}

fn refused() -> ! {
    panic!("fod direct refused")
}

/// `public seed offer id:32`. The immutable slot binds the pilot's inventory
/// before its iDNA is locked. Any caller may deploy.
#[no_mangle]
pub extern "C" fn deploy(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != 32 {
            refused();
        }
        let seed_id: [u8; 32] = raw.try_into().unwrap();
        if seed_id == [0; 32] {
            refused();
        }
        if read_u64(NEXT_ORDER_KEY) != 0 {
            refused();
        }
        write_u128(RESERVE_KEY, 0);
        write_u64(ACTIVATED_AT_KEY, 0);
        write_u128(PENDING_KEY, 0);
        write_u128(INSURANCE_KEY, 0);
        write_u64(POOL_UNITS_KEY, 0);
        write_u64(SEED_COUNT_KEY, 0);
        write_id(SEED_ID_KEY, &seed_id);
        write_u64(RESERVED_UNITS_KEY, 0);
        write_u64(PRIMARY_UNITS_KEY, 0);
        write_u64(SECONDARY_UNITS_KEY, 0);
        write_u64(NEXT_ORDER_KEY, 1);
        write_u64(VALUE_POLICY_KEY, 1);
        write_u64(NEXT_SEQUENCE_KEY, 1);
        write_u64(HEAD_SINCE_KEY, 0);
        write_id(PRIMARY_HEAD_KEY, &[0; 32]);
        write_id(PRIMARY_TAIL_KEY, &[0; 32]);
        write_id(SECONDARY_HEAD_KEY, &[0; 32]);
        write_id(SECONDARY_TAIL_KEY, &[0; 32]);
        ok()
    }
}

/// `public seed offer id:32`. The 10,000 iDNA deposit is accepted only after
/// the pinned, independently proved public bundle enters the secondary queue.
/// Until this call succeeds, trading is closed.
#[no_mangle]
pub extern "C" fn activate(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != 32 || read_u64(NEXT_ORDER_KEY) != 1 || read_u128(RESERVE_KEY) != 0 {
            refused();
        }
        let seed_id: [u8; 32] = raw.try_into().unwrap();
        if seed_id != read_id(SEED_ID_KEY)
            || read_u64(SEED_COUNT_KEY) != 1
            || read_u64(POOL_UNITS_KEY) != 1
            || read_u64(SECONDARY_UNITS_KEY) != 1
            || read_u64(PRIMARY_UNITS_KEY) != 0
            || read_u64(RESERVED_UNITS_KEY) != 0
            || read_u128(PENDING_KEY) != 0
            || read_u128(INSURANCE_KEY) != 0
            || read_id(PRIMARY_HEAD_KEY) != [0; 32]
            || read_id(PRIMARY_TAIL_KEY) != [0; 32]
            || read_id(SECONDARY_HEAD_KEY) != seed_id
            || read_id(SECONDARY_TAIL_KEY) != seed_id
        {
            refused();
        }
        let offer = read_offer(&seed_id).unwrap_or_else(|| refused());
        if offer.status != QUEUED_SECONDARY
            || !offer.public_seed
            || !offer.content_proven
            || offer.previous != [0; 32]
            || offer.next != [0; 32]
            || seed_id != offer_id(&offer.author, offer.epoch)
            || read_keyed(public_seed_key(&seed_id))
                .is_none_or(|secrets| !secrets_match_offer(secrets, &offer))
        {
            refused();
        }
        let seed = amount(pay_amount()).unwrap_or_else(|| refused());
        if seed != PILOT_SEED {
            refused();
        }
        require_solvency(seed, 0);
        write_u128(RESERVE_KEY, seed);
        write_u64(ACTIVATED_AT_KEY, block_number());
        write_u64(HEAD_SINCE_KEY, block_number());
        ok()
    }
}

/// `epoch:u16 | covered:u32 | inventory:32 | keys:32 | session length:u16 |
/// canonical session witness`. This verifies public flip inventory and clean
/// validation status. The private content/key proof is separately verified.
#[no_mangle]
pub extern "C" fn qualify(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        let (metadata, wire) = parse_qualification_arg(raw).unwrap_or_else(|| refused());
        let author = address(caller()).unwrap_or_else(|| refused());
        let (epoch, covered, inventory, keys) =
            offer_metadata(metadata).unwrap_or_else(|| refused());
        if covered == 0 || covered > MAX_FLIPS || inventory == [0; 32] || keys == [0; 32] {
            refused();
        }
        let id = offer_id(&author, epoch);
        if read_offer(&id).is_some() {
            refused();
        }
        let offer = Offer {
            author,
            epoch,
            covered,
            inventory,
            keys,
            seller_bond: 0,
            next_nonce: 1,
            open_order: 0,
            status: QUALIFIED,
            ever_sold: false,
            previous: [0; 32],
            next: [0; 32],
            sequence: 0,
            listed_at: 0,
            content_proven: false,
            fraud_collateral: 0,
            public_seed: false,
        };
        let (verdict, mut cids) = verify_with_roots_and_cids(
            wire,
            &author,
            epoch,
            covered as u16,
            &inventory,
            |height| header_state_root(height),
        )
        .unwrap_or_else(|_| refused());
        if verdict.flips != covered || verdict.flags != 0 {
            refused();
        }
        if cids.iter().any(|cid| {
            cid.len() != PUBLIC_CID_BYTES
                || cid[0] != b'b'
                || !cid[1..]
                    .iter()
                    .all(|byte| byte.is_ascii_lowercase() || (b'2'..=b'7').contains(byte))
        }) {
            refused();
        }
        cids.sort();
        let mut public_inventory =
            Vec::with_capacity(1 + cids.iter().map(|cid| 1 + cid.len()).sum::<usize>());
        public_inventory.push(cids.len() as u8);
        for cid in cids {
            public_inventory.push(cid.len() as u8);
            public_inventory.extend_from_slice(cid);
        }
        write_keyed(key(INVENTORY_PREFIX, &id), &public_inventory);
        write_offer(&id, &offer);
        ok()
    }
}

/// `offer id:32 | fast-content journal | seal | canonical session witness`.
/// Anyone may present an independent proof. Any fraud collateral is returned
/// to the author only after the original flips and keys verify against it.
/// This independent proof does not draw reimbursement from a buyer's fallback bond.
#[no_mangle]
pub extern "C" fn proveBundle(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        let (id, journal, seal, wire) = parse_prove_arg(raw).unwrap_or_else(|| refused());
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        if offer.public_seed
            || offer.content_proven
            || id != offer_id(&offer.author, offer.epoch)
            || !matches!(
                offer.status,
                QUALIFIED | QUEUED_PRIMARY | QUEUED_SECONDARY | ACTIVE | SOLD_PRIVATE
            )
        {
            refused();
        }
        if matches!(offer.status, ACTIVE | SOLD_PRIVATE) {
            let order = read_order(offer.open_order).unwrap_or_else(|| refused());
            if !funded_content_proof_allowed(&offer, &id, &order, block_number()) {
                refused();
            }
        }
        let contract = address(own_addr()).unwrap_or_else(|| refused());
        let native =
            fallback_claim_matches(journal, seal, wire, &offer, &contract, true, |height| {
                header_state_root(height)
            });
        let legacy = !native
            && can_use_legacy_content_proof(&offer)
            && fallback_claim_matches(
                journal,
                seal,
                wire,
                &offer,
                &LEGACY_V6_CONTENT_CONTRACT,
                true,
                |height| header_state_root(height),
            );
        if !native && !legacy {
            refused();
        }
        let returned = offer.fraud_collateral;
        let pending = read_u128(PENDING_KEY);
        if returned > pending {
            refused();
        }
        require_solvency(read_u128(RESERVE_KEY), pending);
        offer.content_proven = true;
        offer.fraud_collateral = 0;
        if offer.status == SOLD_PRIVATE {
            offer.open_order = 0;
        }
        write_offer(&id, &offer);
        if offer.status == SOLD_PRIVATE {
            close_seller_listing(&offer.author, &id);
        }
        if returned > 0 {
            write_u128(PENDING_KEY, pending - returned);
            transfer(&offer.author, returned);
        }
        ok()
    }
}

/// `settled order id:u64`. An unproved seller has the same three-day window
/// to establish content even when its buyer confirmed privately. Otherwise
/// its held fraud collateral joins the pool reserve and the offer retires.
#[no_mangle]
pub extern "C" fn slashUnproved(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != 8 {
            refused();
        }
        let order_id = u64_at(raw, 0);
        let order = read_order(order_id).unwrap_or_else(|| refused());
        let mut offer = read_offer(&order.offer).unwrap_or_else(|| refused());
        if order.state != SETTLED
            || offer.status != SOLD_PRIVATE
            || offer.content_proven
            || offer.fraud_collateral == 0
            || offer.open_order != order_id
            || offer.next_nonce != order.nonce.checked_add(1).unwrap_or_else(|| refused())
            || !order
                .funded_at
                .checked_add(PROOF_WINDOW_BLOCKS)
                .is_some_and(|deadline| block_number() >= deadline)
        {
            refused();
        }
        let pending = read_u128(PENDING_KEY);
        let reserve = read_u128(RESERVE_KEY);
        let next_reserve = reserve
            .checked_add(offer.fraud_collateral)
            .unwrap_or_else(|| refused());
        let next_insurance = read_u128(INSURANCE_KEY)
            .checked_add(offer.fraud_collateral)
            .unwrap_or_else(|| refused());
        if pending < offer.fraud_collateral {
            refused();
        }
        require_solvency(next_reserve, pending - offer.fraud_collateral);
        write_u128(PENDING_KEY, pending - offer.fraud_collateral);
        write_u128(RESERVE_KEY, next_reserve);
        write_u128(INSURANCE_KEY, next_insurance);
        offer.fraud_collateral = 0;
        offer.open_order = 0;
        offer.status = RETIRED;
        write_offer(&order.offer, &offer);
        close_seller_listing(&offer.author, &order.offer);
        note_seller_default(&offer.author);
        ok()
    }
}

/// `offer id:32 | publicly disclosed epoch secrets:106`. The one pinned,
/// independently proved public bundle initializes permanent inventory. It
/// earns no listing payout; a buyer pays the original author half the sale
/// price without a private-delivery path.
#[no_mangle]
pub extern "C" fn seedBundle(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != 32 + SECRETS_BYTES {
            refused();
        }
        let id: [u8; 32] = raw[..32].try_into().unwrap();
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        let seed_count = read_u64(SEED_COUNT_KEY);
        if id != offer_id(&offer.author, offer.epoch)
            || offer.author != address(caller()).unwrap_or_else(|| refused())
            || offer.status != QUALIFIED
            || !offer.content_proven
            || offer.public_seed
            || seller_banned(&offer.author)
            || seller_listing(&offer.author) != [0; 32]
            || !secrets_match_offer(&raw[32..], &offer)
            || id != read_id(SEED_ID_KEY)
            || seed_count != 0
            || read_u128(RESERVE_KEY) != 0
            || read_u64(POOL_UNITS_KEY) != seed_count
            || read_u64(SECONDARY_UNITS_KEY) != seed_count
            || read_u64(RESERVED_UNITS_KEY) != 0
            || read_u64(PRIMARY_UNITS_KEY) != 0
            || read_u64(NEXT_ORDER_KEY) != 1
        {
            refused();
        }
        offer.public_seed = true;
        offer.status = QUEUED_SECONDARY;
        enqueue(&id, &mut offer);
        write_offer(&id, &offer);
        write_keyed(public_seed_key(&id), &raw[32..]);
        write_u64(SEED_COUNT_KEY, seed_count + 1);
        write_u64(
            POOL_UNITS_KEY,
            read_u64(POOL_UNITS_KEY)
                .checked_add(1)
                .unwrap_or_else(|| refused()),
        );
        ok()
    }
}

/// `epoch:u16 | covered:u32 | inventory:32 | keys:32 | minimum payout:u128`.
/// The minimum checks only the current listing estimate; it reserves no future
/// payout. The author locks a fixed proof/default bond and, without prior
/// content proof, fraud collateral equal to the maximum possible buyer price.
#[no_mangle]
pub extern "C" fn list(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != OFFER_ARG_BYTES {
            refused();
        }
        let author = address(caller()).unwrap_or_else(|| refused());
        let (epoch, covered, inventory, keys) = offer_metadata(raw).unwrap_or_else(|| refused());
        let minimum_payout = u128::from_be_bytes(raw[70..86].try_into().unwrap());
        let id = offer_id(&author, epoch);
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        if offer.status != QUALIFIED
            || offer.author != author
            || offer.epoch != epoch
            || offer.covered != covered
            || offer.inventory != inventory
            || offer.keys != keys
            || offer.ever_sold
            || offer.seller_bond != 0
        {
            refused();
        }
        offer.status = QUEUED_PRIMARY;
        queue_offer(&id, &mut offer, minimum_payout);
        ok()
    }
}

/// `offer id:32 | minimum payout:u128`. A private sale or cancellation can
/// return to the tail. Public seeds rotate automatically and do not occupy
/// their authors' private listing slots.
#[no_mangle]
pub extern "C" fn relist(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != RELIST_ARG_BYTES {
            refused();
        }
        let id: [u8; 32] = raw[..32].try_into().unwrap();
        let minimum_payout = u128::from_be_bytes(raw[32..48].try_into().unwrap());
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        if offer.author != address(caller()).unwrap_or_else(|| refused())
            || !offer.can_relist()
            || offer.public_seed
        {
            refused();
        }
        offer.status = if offer.ever_sold {
            QUEUED_SECONDARY
        } else {
            QUEUED_PRIMARY
        };
        queue_offer(&id, &mut offer, minimum_payout);
        ok()
    }
}

/// `offer id:32`. The author may cancel without paying the buyer's quote.
/// The delivery bond is returned; unproved content forfeits its fraud collateral.
#[no_mangle]
pub extern "C" fn cancel(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        let id: [u8; 32] = raw.try_into().unwrap_or_else(|_| refused());
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        if offer.author != address(caller()).unwrap_or_else(|| refused())
            || !offer.queued()
            || offer.public_seed
        {
            refused();
        }
        remove_queued(&id, &mut offer);
        ok()
    }
}

fn queue_head_expired(listed_at: u64, height: u64) -> bool {
    listed_at
        .checked_add(QUEUE_HEAD_TIMEOUT_BLOCKS)
        .is_some_and(|deadline| height >= deadline)
}

/// `expected oldest offer id:32`. Anyone may clear a private head that has
/// remained selectable for seven days, or rotate a public seed to the tail.
/// A seller who never proves content forfeits only the explicit fraud
/// collateral; the delivery bond is returned.
#[no_mangle]
pub extern "C" fn expireHead(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        let id: [u8; 32] = raw.try_into().unwrap_or_else(|_| refused());
        if id == [0; 32] || oldest_offer_id() != id || read_u128(RESERVE_KEY) == 0 {
            refused();
        }
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        let head_since = read_u64(HEAD_SINCE_KEY).max(read_u64(ACTIVATED_AT_KEY));
        if !offer.queued() || !queue_head_expired(head_since, block_number()) {
            refused();
        }
        if offer.public_seed {
            dequeue(&id, &mut offer);
            enqueue(&id, &mut offer);
            write_offer(&id, &offer);
        } else {
            remove_queued(&id, &mut offer);
        }
        ok()
    }
}

unsafe fn remove_queued(id: &[u8; 32], offer: &mut Offer) {
    let reserve = read_u128(RESERVE_KEY);
    let pending = read_u128(PENDING_KEY);
    let held = offer
        .seller_bond
        .checked_add(offer.fraud_collateral)
        .unwrap_or_else(|| refused());
    if pending < held {
        refused();
    }
    require_solvency(reserve, pending);
    let next_reserve = reserve
        .checked_add(offer.fraud_collateral)
        .unwrap_or_else(|| refused());
    let next_insurance = read_u128(INSURANCE_KEY)
        .checked_add(offer.fraud_collateral)
        .unwrap_or_else(|| refused());
    let returned = offer.seller_bond;
    require_solvency(next_reserve, pending - held);
    dequeue(id, offer);
    offer.status = CANCELLED;
    offer.seller_bond = 0;
    offer.fraud_collateral = 0;
    write_offer(id, offer);
    close_seller_listing(&offer.author, id);
    write_u128(PENDING_KEY, pending - held);
    write_u128(RESERVE_KEY, next_reserve);
    write_u128(INSURANCE_KEY, next_insurance);
    write_u64(
        POOL_UNITS_KEY,
        read_u64(POOL_UNITS_KEY)
            .checked_sub(1)
            .unwrap_or_else(|| refused()),
    );
    if returned > 0 {
        transfer(&offer.author, returned);
    }
}

/// `offer id:32`. The author may add to the fixed delivery bond to cover
/// higher proof/default costs without changing the eventual buyer price.
#[no_mangle]
pub extern "C" fn topUpBond(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        let id: [u8; 32] = raw.try_into().unwrap_or_else(|_| refused());
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        if !offer.queued()
            || offer.public_seed
            || offer.author != address(caller()).unwrap_or_else(|| refused())
        {
            refused();
        }
        let added = amount(pay_amount()).unwrap_or_else(|| refused());
        if added == 0 {
            refused();
        }
        let pending = read_u128(PENDING_KEY)
            .checked_add(added)
            .unwrap_or_else(|| refused());
        require_solvency(read_u128(RESERVE_KEY), pending);
        offer.seller_bond = offer
            .seller_bond
            .checked_add(added)
            .unwrap_or_else(|| refused());
        write_offer(&id, &offer);
        write_u128(PENDING_KEY, pending);
        ok()
    }
}

/// `expected oldest offer id:32 | buyer's delivery public key:65`.
/// The payment must cover the current curve quote. Its exact amount fixes the
/// first-sale payout or half-price resale dividend for this order only.
/// The expected ID protects buyers from a changed queue head.
#[no_mangle]
pub extern "C" fn buy(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != BUY_ARG_BYTES {
            refused();
        }
        let id: [u8; 32] = raw[..32].try_into().unwrap();
        let delivery_key: [u8; 65] = raw[32..97].try_into().unwrap();
        if delivery_key[0] != 4 || delivery_key[1..] == [0u8; 64] {
            refused();
        }
        if id == [0; 32] || oldest_offer_id() != id {
            refused();
        }
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        if !offer.queued() || offer.public_seed {
            refused();
        }
        let reserve = read_u128(RESERVE_KEY);
        let (units, virtual_reserve) = virtual_pool();
        let quote = curve::buy_quote(units, virtual_reserve).unwrap_or_else(|| refused());
        if seller_banned(&offer.author) {
            refused();
        }
        if offer.seller_bond < required_seller_bond() {
            refused();
        }
        let minimum_bond = required_bond();
        let total = amount(pay_amount()).unwrap_or_else(|| refused());
        let bond = total.checked_sub(quote).unwrap_or_else(|| refused());
        if bond < minimum_bond {
            refused();
        }
        let buyer = address(caller()).unwrap_or_else(|| refused());
        let order_id = next_order();
        let next = order_id.checked_add(1).unwrap_or_else(|| refused());
        let pending = read_u128(PENDING_KEY)
            .checked_add(total)
            .unwrap_or_else(|| refused());
        require_solvency(reserve, pending);
        let nonce = offer.next_nonce;
        offer.next_nonce = nonce.checked_add(1).unwrap_or_else(|| refused());
        dequeue(&id, &mut offer);
        offer.open_order = order_id;
        offer.status = ACTIVE;
        write_offer(&id, &offer);
        let seller_payout = sale_payout(&offer, quote);
        write_order(
            order_id,
            &Order {
                offer: id,
                buyer,
                delivery_key,
                paid: quote,
                bond,
                seller_payout,
                funded_at: block_number(),
                nonce,
                state: FUNDED,
                delivery_hash: [0; 32],
                challenged: false,
            },
        );
        write_u128(PENDING_KEY, pending);
        write_u64(
            RESERVED_UNITS_KEY,
            read_u64(RESERVED_UNITS_KEY)
                .checked_add(1)
                .unwrap_or_else(|| refused()),
        );
        write_keyed(key(NEXT_ORDER_KEY, b""), &next.to_be_bytes());
        ok()
    }
}

/// `expected oldest public seed id:32`. These keys were already disclosed
/// with an independent content proof, so purchase settles in one transaction.
/// Half the price goes to the original author and half enters the pool. The
/// disclosed seed returns to the secondary tail so real flip inventory remains.
#[no_mangle]
pub extern "C" fn buyPublicSeed(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        let id: [u8; 32] = raw.try_into().unwrap_or_else(|_| refused());
        if id == [0; 32] || oldest_offer_id() != id {
            refused();
        }
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        if !offer.queued()
            || !offer.public_seed
            || !offer.content_proven
            || offer.seller_bond != 0
            || offer.fraud_collateral != 0
            || read_keyed(public_seed_key(&id))
                .is_none_or(|secrets| !secrets_match_offer(secrets, &offer))
        {
            refused();
        }
        let (units, virtual_reserve) = virtual_pool();
        let quote = curve::buy_quote(units, virtual_reserve).unwrap_or_else(|| refused());
        if amount(pay_amount()).unwrap_or_else(|| refused()) != quote {
            refused();
        }
        let reserve = read_u128(RESERVE_KEY);
        let author_dividend = quote / 2;
        let next_reserve = reserve
            .checked_add(quote - author_dividend)
            .unwrap_or_else(|| refused());
        require_solvency(next_reserve, read_u128(PENDING_KEY));
        dequeue(&id, &mut offer);
        offer.ever_sold = true;
        offer.status = QUEUED_SECONDARY;
        enqueue(&id, &mut offer);
        write_offer(&id, &offer);
        write_u128(RESERVE_KEY, next_reserve);
        if author_dividend > 0 {
            transfer(&offer.author, author_dividend);
        }
        ok()
    }
}

/// The buyer may explicitly request a verifiable public fallback. The proof
/// cost was locked as their bond at purchase; it goes to the seller only when
/// the seller proves and reveals valid keys. The deadline is fixed at funding.
#[no_mangle]
pub extern "C" fn challenge(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != 8 {
            refused();
        }
        let id = u64_at(raw, 0);
        let mut order = read_order(id).unwrap_or_else(|| refused());
        if order.state != FUNDED
            || order.challenged
            || order.buyer != address(caller()).unwrap_or_else(|| refused())
            || !order
                .funded_at
                .checked_add(PROOF_WINDOW_BLOCKS)
                .is_some_and(|end| block_number() < end)
        {
            refused();
        }
        order.challenged = true;
        write_order(id, &order);
        ok()
    }
}

/// `order id:u64 | sha256(exact private delivery ciphertext):32`. A buyer may
/// confirm a funded order immediately after local verification, including after
/// the seller's proof window, until a refund actually lands.
#[no_mangle]
pub extern "C" fn confirm(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != CONFIRM_ARG_BYTES {
            refused();
        }
        let id = u64_at(raw, 0);
        let mut order = read_order(id).unwrap_or_else(|| refused());
        if order.state != FUNDED || order.buyer != address(caller()).unwrap_or_else(|| refused()) {
            refused();
        }
        let hash: [u8; 32] = raw[8..40].try_into().unwrap();
        if hash == [0; 32] {
            refused();
        }
        order.delivery_hash = hash;
        let mut offer = read_offer(&order.offer).unwrap_or_else(|| refused());
        settle(id, &mut order, &mut offer, false);
        ok()
    }
}

/// A pre-proved offer needs only `order id:u64`. Otherwise this accepts
/// `order id:u64 | fast-content journal | Groth16 seal | session witness`.
/// The seller's delivery bond was locked at listing. A challenged order can
/// reimburse either proof path from the buyer's bond after valid disclosure.
#[no_mangle]
pub extern "C" fn startFallback(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() < 8 {
            refused();
        }
        let id = u64_at(raw, 0);
        let mut order = read_order(id).unwrap_or_else(|| refused());
        if !order.can_start_fallback(
            block_number(),
            PROOF_WINDOW_BLOCKS,
            BUYER_CONFIRM_GRACE_BLOCKS,
            PROOF_DEADLINE_MARGIN_BLOCKS,
        ) {
            refused();
        }
        let offer = read_offer(&order.offer).unwrap_or_else(|| refused());
        if offer.status != ACTIVE
            || offer.open_order != id
            || order.offer != offer_id(&offer.author, offer.epoch)
            || order.seller_payout != sale_payout(&offer, order.paid)
            || offer.seller_bond == 0
            || offer.author != address(caller()).unwrap_or_else(|| refused())
        {
            refused();
        }
        let rate = amount(min_fee_per_gas()).unwrap_or_else(|| refused());
        if rate == 0 {
            refused();
        }
        let liability = order
            .paid
            .checked_add(order.bond)
            .and_then(|sum| sum.checked_add(offer.seller_bond))
            .and_then(|sum| sum.checked_add(offer.fraud_collateral))
            .unwrap_or_else(|| refused());
        let pending = read_u128(PENDING_KEY);
        if pending < liability {
            refused();
        }
        require_solvency(read_u128(RESERVE_KEY), pending);
        if offer.content_proven {
            if raw.len() != 8 {
                refused();
            }
        } else {
            let (_, journal, seal, wire) = parse_start_arg(raw).unwrap_or_else(|| refused());
            let contract = address(own_addr()).unwrap_or_else(|| refused());
            if !fallback_claim_matches(journal, seal, wire, &offer, &contract, true, |height| {
                header_state_root(height)
            }) {
                refused();
            }
        }
        // Only an explicit buyer proof request can charge the buyer's bond.
        // A seller can still recover payment through public fallback when
        // the buyer remains silent, but cannot earn a proof-request penalty.
        write_keyed(proof_rate_key(id), &rate.to_be_bytes());
        order.state = FALLBACK_PENDING;
        write_order(id, &order);
        ok()
    }
}

/// `order id:u64 | epoch secrets:106`. A completed content proof verified the
/// original CID inventory and the two keys against canonical session roots.
/// Public disclosure is the recovery path; private-message delivery cannot be
/// proven from a sender's assertion.
#[no_mangle]
pub extern "C" fn proveAndReveal(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != REVEAL_ARG_BYTES {
            refused();
        }
        let id = u64_at(raw, 0);
        let mut order = read_order(id).unwrap_or_else(|| refused());
        if !order.seller_can_reveal(block_number(), PROOF_WINDOW_BLOCKS) {
            refused();
        }
        let mut offer = read_offer(&order.offer).unwrap_or_else(|| refused());
        if offer.status != ACTIVE
            || offer.open_order != id
            || offer.author != address(caller()).unwrap_or_else(|| refused())
        {
            refused();
        }
        let secrets = &raw[8..];
        if !secrets_match_offer(secrets, &offer) {
            refused();
        }
        settle(id, &mut order, &mut offer, true);
        // Retain the exact public keys in canonical state so a buyer needs no
        // transaction indexer, seller, or social account for recovery.
        write_keyed(reveal_key(id), secrets);
        ok()
    }
}

/// Anyone may refund at the deadline. The buyer receives the entire locked
/// price and buyer bond, plus the seller's delivery bond as default compensation.
/// Unproved fraud collateral becomes insurance outside the pricing reserve.
#[no_mangle]
pub extern "C" fn refund(arg: u32) -> u32 {
    unsafe {
        refund_action_sentinel();
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != 8 {
            refused();
        }
        let id = u64_at(raw, 0);
        let mut order = read_order(id).unwrap_or_else(|| refused());
        if !order.can_refund(block_number(), PROOF_WINDOW_BLOCKS) {
            refused();
        }
        let mut offer = read_offer(&order.offer).unwrap_or_else(|| refused());
        if offer.status != ACTIVE
            || offer.open_order != id
            || order.offer != offer_id(&offer.author, offer.epoch)
            || order.seller_payout != sale_payout(&offer, order.paid)
            || offer.seller_bond == 0
            || offer.public_seed
        {
            refused();
        }
        let total = order
            .paid
            .checked_add(order.bond)
            .and_then(|sum| sum.checked_add(offer.seller_bond))
            .and_then(|sum| sum.checked_add(offer.fraud_collateral))
            .unwrap_or_else(|| refused());
        let pending = read_u128(PENDING_KEY);
        let reserve = read_u128(RESERVE_KEY);
        let insurance = read_u128(INSURANCE_KEY);
        if pending < total {
            refused();
        }
        require_solvency(reserve, pending);
        let next_reserve = reserve
            .checked_add(offer.fraud_collateral)
            .unwrap_or_else(|| refused());
        let next_insurance = insurance
            .checked_add(offer.fraud_collateral)
            .unwrap_or_else(|| refused());
        let buyer_refund =
            full_default_refund(&order, offer.seller_bond).unwrap_or_else(|| refused());
        require_solvency(next_reserve, pending - total);
        write_u128(PENDING_KEY, pending - total);
        write_u128(RESERVE_KEY, next_reserve);
        write_u128(INSURANCE_KEY, next_insurance);
        write_u64(
            POOL_UNITS_KEY,
            read_u64(POOL_UNITS_KEY)
                .checked_sub(1)
                .unwrap_or_else(|| refused()),
        );
        write_u64(
            RESERVED_UNITS_KEY,
            read_u64(RESERVED_UNITS_KEY)
                .checked_sub(1)
                .unwrap_or_else(|| refused()),
        );
        order.state = REFUNDED;
        offer.open_order = 0;
        offer.seller_bond = 0;
        offer.fraud_collateral = 0;
        offer.status = RETIRED;
        write_order(id, &order);
        write_offer(&order.offer, &offer);
        close_seller_listing(&offer.author, &order.offer);
        note_seller_default(&offer.author);
        transfer(&order.buyer, buyer_refund);
        ok()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn private_offer(ever_sold: bool) -> Offer {
        Offer {
            author: [1; 20],
            epoch: 230,
            covered: 1,
            inventory: [2; 32],
            keys: [3; 32],
            seller_bond: 5,
            next_nonce: 1,
            open_order: 0,
            status: if ever_sold {
                QUEUED_SECONDARY
            } else {
                QUEUED_PRIMARY
            },
            ever_sold,
            previous: [0; 32],
            next: [0; 32],
            sequence: 1,
            listed_at: 10,
            content_proven: true,
            fraud_collateral: 0,
            public_seed: false,
        }
    }

    #[test]
    fn locked_primary_payout_follows_actual_buyer_quote_without_reserve_draw() {
        let offer = private_offer(false);
        let listed_quote = curve::buy_quote(12, PILOT_SEED).unwrap();
        let funded_quote = curve::buy_quote(13, PILOT_SEED).unwrap();
        assert!(funded_quote < listed_quote);
        assert_eq!(sale_payout(&offer, funded_quote), funded_quote);
        assert_eq!(sale_reserve_credit(&offer, funded_quote), 0);
    }

    #[test]
    fn self_trade_cannot_extract_reserve_and_resale_split_rounds_down() {
        let primary = private_offer(false);
        let secondary = private_offer(true);
        assert_eq!(sale_payout(&primary, 5), 5);
        assert_eq!(sale_reserve_credit(&primary, 5), 0);
        assert_eq!(sale_payout(&secondary, 5), 2);
        assert_eq!(sale_reserve_credit(&secondary, 5), 3);
        assert_eq!(sale_payout(&secondary, 6), 3);
        assert_eq!(sale_reserve_credit(&secondary, 6), 3);
    }

    #[test]
    fn default_refund_contains_full_buyer_price_and_bonds() {
        let order = Order {
            offer: [4; 32],
            buyer: [5; 20],
            delivery_key: [6; 65],
            paid: 100,
            bond: 20,
            seller_payout: 100,
            funded_at: 1,
            nonce: 1,
            state: FUNDED,
            delivery_hash: [0; 32],
            challenged: false,
        };
        assert_eq!(full_default_refund(&order, 7), Some(127));
        assert_eq!(
            full_default_refund(
                &Order {
                    paid: u128::MAX,
                    ..order
                },
                7
            ),
            None
        );
    }

    #[test]
    fn primary_queue_precedes_secondary_and_old_head_expires_at_boundary() {
        let primary = [1; 32];
        let secondary = [2; 32];
        assert_eq!(priority_head(primary, secondary), primary);
        assert_eq!(priority_head([0; 32], secondary), secondary);
        assert!(!queue_head_expired(10, 10 + QUEUE_HEAD_TIMEOUT_BLOCKS - 1));
        assert!(queue_head_expired(10, 10 + QUEUE_HEAD_TIMEOUT_BLOCKS));
        assert!(!queue_head_expired(u64::MAX - 1, u64::MAX));
    }

    #[test]
    fn unproved_content_requires_price_cap_collateral_until_proved() {
        assert_eq!(fraud_collateral_required(false), curve::PRICE_CAP);
        assert_eq!(fraud_collateral_required(true), 0);
        assert_eq!(seller_bond_for_rate(1), Some(5_000_000));
        assert_eq!(seller_bond_for_rate(2), Some(10_000_000));
    }

    #[test]
    fn funded_content_proof_cannot_release_collateral_at_or_after_deadline() {
        let id = [9; 32];
        let mut offer = Offer {
            author: [1; 20],
            epoch: 230,
            covered: 1,
            inventory: [2; 32],
            keys: [3; 32],
            seller_bond: 5,
            next_nonce: 2,
            open_order: 7,
            status: ACTIVE,
            ever_sold: false,
            previous: [0; 32],
            next: [0; 32],
            sequence: 1,
            listed_at: 1,
            content_proven: false,
            fraud_collateral: 10,
            public_seed: false,
        };
        let mut order = Order {
            offer: id,
            buyer: [4; 20],
            delivery_key: [5; 65],
            paid: 10,
            bond: 5,
            seller_payout: 10,
            funded_at: 100,
            nonce: 1,
            state: FUNDED,
            delivery_hash: [0; 32],
            challenged: false,
        };
        let deadline = 100 + PROOF_WINDOW_BLOCKS;
        assert!(funded_content_proof_allowed(
            &offer,
            &id,
            &order,
            deadline - 1
        ));
        assert!(!funded_content_proof_allowed(&offer, &id, &order, deadline));
        offer.status = SOLD_PRIVATE;
        order.state = SETTLED;
        assert!(funded_content_proof_allowed(
            &offer,
            &id,
            &order,
            deadline - 1
        ));
        assert!(!funded_content_proof_allowed(&offer, &id, &order, deadline));
        offer.open_order = 0;
        assert!(!funded_content_proof_allowed(
            &offer,
            &id,
            &order,
            deadline - 1
        ));
    }

    #[test]
    fn statement_requires_exact_offer_and_contract_binding() {
        let offer = Offer {
            author: [1; 20],
            epoch: 230,
            covered: 5,
            inventory: [2; 32],
            keys: [3; 32],
            seller_bond: 1,
            next_nonce: 1,
            open_order: 1,
            status: ACTIVE,
            ever_sold: false,
            previous: [0; 32],
            next: [0; 32],
            sequence: 1,
            listed_at: 1,
            content_proven: false,
            fraud_collateral: 1,
            public_seed: false,
        };
        let mut journal = Journal {
            network: NETWORK_DIGEST,
            contract: [4; 20],
            author: offer.author,
            epoch: offer.epoch,
            count: offer.covered as u16,
            consensus: CONSENSUS_DIGEST,
            policy: POLICY_DIGEST,
            original_inventory: offer.inventory,
            copies: [0; 32],
            point_commitment: [0; 33],
            original_keys: offer.keys,
        };
        assert!(statement_matches_offer(&journal, &offer, &[4; 20]));
        journal.original_inventory[2] ^= 1;
        assert!(!statement_matches_offer(&journal, &offer, &[4; 20]));
        journal.original_inventory[2] ^= 1;
        assert!(!statement_matches_offer(&journal, &offer, &[5; 20]));

        // The v6 exception applies only before a sale. The native fallback
        // still expects this contract's address, including after funding.
        let mut qualified = offer;
        qualified.status = QUALIFIED;
        qualified.open_order = 0;
        qualified.seller_bond = 0;
        assert!(can_use_legacy_content_proof(&qualified));
        journal.contract = LEGACY_V6_CONTENT_CONTRACT;
        assert!(statement_matches_offer(
            &journal,
            &qualified,
            &LEGACY_V6_CONTENT_CONTRACT
        ));
        assert!(!statement_matches_offer(&journal, &qualified, &[5; 20]));
        qualified.status = ACTIVE;
        qualified.open_order = 1;
        assert!(!can_use_legacy_content_proof(&qualified));
        qualified.status = QUALIFIED;
        qualified.open_order = 0;
        qualified.epoch += 1;
        assert!(!can_use_legacy_content_proof(&qualified));
        qualified.epoch -= 1;
        qualified.public_seed = true;
        assert!(!can_use_legacy_content_proof(&qualified));
    }

    #[test]
    fn plain_hash_preimage_is_not_a_fallback_claim() {
        let mut secrets = [0u8; SECRETS_BYTES];
        secrets[..SECRETS_DOMAIN.len()].copy_from_slice(SECRETS_DOMAIN);
        secrets[SECRETS_DOMAIN.len()..SECRETS_DOMAIN.len() + 2]
            .copy_from_slice(&230u16.to_be_bytes());
        secrets[SECRETS_DOMAIN.len() + 2..SECRETS_DOMAIN.len() + 22].copy_from_slice(&[1; 20]);
        let offer = Offer {
            author: [1; 20],
            epoch: 230,
            covered: 5,
            inventory: [2; 32],
            keys: verifier::sha256(&secrets),
            seller_bond: 1,
            next_nonce: 1,
            open_order: 1,
            status: ACTIVE,
            ever_sold: false,
            previous: [0; 32],
            next: [0; 32],
            sequence: 1,
            listed_at: 1,
            content_proven: false,
            fraud_collateral: 1,
            public_seed: false,
        };
        assert!(secrets_match_offer(&secrets, &offer));
        let mut wrong = secrets;
        wrong[SECRETS_DOMAIN.len() + 22] ^= 1;
        assert!(!secrets_match_offer(&wrong, &offer));
        // The settlement method separately requires the genuine sealed guest
        // receipt and canonical roots; this check by itself cannot pay.
    }

    #[test]
    fn fallback_returns_bond_above_capped_proof_reimbursement() {
        assert_eq!(fallback_reimbursement(5_000_000, 1), 2_500_000);
        assert_eq!(fallback_reimbursement(1_000_000, 1), 1_000_000);
        assert_eq!(fallback_reimbursement(5_000_000, 0), 0);
        assert_eq!(fallback_reimbursement(5_000_000, u128::MAX), 5_000_000);
    }
}
