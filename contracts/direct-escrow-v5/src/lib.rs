//! Ownerless direct sales of original Idena flip keys, version 5 candidate.
//!
//! A listing commits to one author's epoch keys and complete CID inventory.
//! The buyer checks the original blobs locally before funding, then may confirm
//! a privately delivered key envelope for immediate payment. If the buyer does
//! not confirm, the author stakes a proof-backed fallback and publishes the
//! committed keys before the deadline.
//! The buyer's refundable bond covers the fallback's measured gas budget;
//! any amount above the proof reimbursement returns to the buyer.
//! A hash preimage alone never releases escrow. An unproved order refunds.

#![no_std]
#![allow(static_mut_refs)]

extern crate alloc;

use core::panic::PanicInfo;
use fod_fast_content_core::{Journal, JOURNAL_BYTES};
use fod_fast_session_core::{verify_with_roots, MAX_WIRE_BYTES};

mod state;
#[path = "../../escrow/src/verifier.rs"]
mod verifier;

use state::{Offer, Order, FALLBACK_PENDING, FUNDED, REFUNDED, SETTLED};

const ADDRESS_BYTES: usize = 20;
const DIGEST_BYTES: usize = 32;
const AMOUNT_BYTES: usize = 16;
const DELIVERY_KEY_BYTES: usize = 65;
const SECRETS_BYTES: usize = 106;
const HEADER_READ_BYTES: usize = 16 * 1024;
const OFFER_ARG_BYTES: usize = 2 + 4 + 32 + 32 + 16;
const BUY_ARG_BYTES: usize = 32 + DELIVERY_KEY_BYTES;
const CONFIRM_ARG_BYTES: usize = 8 + 32;
const START_HEADER_BYTES: usize = 8 + JOURNAL_BYTES + verifier::SEAL_BYTES + 2;
const MAX_SESSION_WIRE_BYTES: usize = u16::MAX as usize;
const REVEAL_ARG_BYTES: usize = 8 + SECRETS_BYTES;
/// At the observed roughly 20-second mainnet cadence this is about 48 hours.
/// The exact on-chain boundary is a block height, shown to buyers and sellers.
const PROOF_WINDOW_BLOCKS: u64 = 8_640;
const BUYER_CONFIRM_GRACE_BLOCKS: u64 = 30;
const PROOF_DEADLINE_MARGIN_BLOCKS: u64 = 120;
const MAX_FLIPS: u32 = 32;
/// Reproduced v5 guest image. The deployed v4 fast escrow retains its own pin.
pub const FAST_CONTENT_V5_IMAGE_ID: [u8; 32] = [
    0x4f, 0xd1, 0x24, 0xc8, 0xcb, 0x91, 0x37, 0x0d, 0xab, 0x60, 0xd3, 0xe1, 0xd7, 0xc4, 0xe6, 0x66,
    0xe7, 0x94, 0x85, 0xe2, 0x10, 0xb5, 0xf2, 0x0e, 0xd3, 0x6c, 0xc2, 0x10, 0x48, 0x83, 0x72, 0x6c,
];
const SECRETS_DOMAIN: &[u8] = b"fod:epoch-secrets:v1";
const OFFER_DOMAIN: &[u8] = b"fod:direct-offer:v2";
const OFFER_PREFIX: &[u8] = b"direct/offer/";
const ORDER_PREFIX: &[u8] = b"direct/order/";
const REVEAL_PREFIX: &[u8] = b"direct/reveal/";
const STAKE_PREFIX: &[u8] = b"direct/stake/";
const PROOF_RATE_PREFIX: &[u8] = b"direct/proof-rate/";
const NEXT_ORDER_KEY: &[u8] = b"direct/next-order";
const PENDING_KEY: &[u8] = b"direct/pending";
/// The buyer locks a reserve for 2.5 million gas at twice the fee rate when
/// the buy lands. A later fee increase above that margin remains an economic risk.
const BOND_GAS_BUDGET: u128 = 2_500_000;
const BOND_FEE_MARGIN: u128 = 2;
/// The release gate must measure the complete fallback transaction below this
/// allowance. The rest of the buyer's fee-rise reserve is never seller revenue.
const FALLBACK_REIMBURSE_GAS_BUDGET: u128 = 2_500_000;
const SELLER_STAKE_GAS_BUDGET: u128 = 100_000;
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
const VAL_BUF_SIZE: usize = 256;
const READ_BUF_SIZE: usize = 1024;
const HEAP_SIZE: usize = 512;
static mut ALLOC_ARENA: [u8; ALLOC_ARENA_SIZE] = [0; ALLOC_ARENA_SIZE];
static mut ALLOC_NEXT: usize = 0;
static mut KEY_BUF: [u8; KEY_BUF_SIZE] = [0; KEY_BUF_SIZE];
static mut KEY_REGION: Region = Region { offset: 0, capacity: 0, length: 0 };
static mut VAL_BUF: [u8; VAL_BUF_SIZE] = [0; VAL_BUF_SIZE];
static mut VAL_REGION: Region = Region { offset: 0, capacity: 0, length: 0 };
static mut READ_BUF: [u8; READ_BUF_SIZE] = [0; READ_BUF_SIZE];
static mut REPLY_BUF: [u8; 16] = [0; 16];
static mut REPLY_REGION: Region = Region { offset: 0, capacity: 0, length: 0 };
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
        region as u32
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
    Some(core::slice::from_raw_parts((*region).offset as *const u8, (*region).length as usize))
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
    point_region(core::ptr::addr_of_mut!(KEY_REGION), core::ptr::addr_of_mut!(KEY_BUF) as *mut u8, KEY_BUF_SIZE as u32, len as u32)
}

unsafe fn order_id_key(prefix: &[u8], id: u64) -> u32 {
    let mut digits = [0u8; 20];
    let mut n = 0;
    let mut value = id;
    loop {
        digits[n] = b'0' + (value % 10) as u8;
        n += 1;
        value /= 10;
        if value == 0 { break; }
    }
    digits[..n].reverse();
    key(prefix, &digits[..n])
}

unsafe fn order_key(id: u64) -> u32 { order_id_key(ORDER_PREFIX, id) }

unsafe fn reveal_key(id: u64) -> u32 {
    order_id_key(REVEAL_PREFIX, id)
}

unsafe fn stake_key(id: u64) -> u32 { order_id_key(STAKE_PREFIX, id) }

unsafe fn proof_rate_key(id: u64) -> u32 { order_id_key(PROOF_RATE_PREFIX, id) }

unsafe fn read_keyed(storage_key: u32) -> Option<&'static [u8]> {
    if storage_key == 0 { return None; }
    let src = read_region(get_storage(storage_key))?;
    if src.is_empty() || src.len() > READ_BUF_SIZE { return None; }
    READ_BUF[..src.len()].copy_from_slice(src);
    Some(&READ_BUF[..src.len()])
}

unsafe fn write_keyed(storage_key: u32, value: &[u8]) {
    if storage_key == 0 || value.len() > VAL_BUF_SIZE { refused(); }
    VAL_BUF[..value.len()].copy_from_slice(value);
    let region = point_region(core::ptr::addr_of_mut!(VAL_REGION), core::ptr::addr_of_mut!(VAL_BUF) as *mut u8, VAL_BUF_SIZE as u32, value.len() as u32);
    set_storage(storage_key, region);
}

unsafe fn read_offer(id: &[u8; DIGEST_BYTES]) -> Option<Offer> {
    Offer::decode(read_keyed(key(OFFER_PREFIX, id))?)
}

unsafe fn write_offer(id: &[u8; DIGEST_BYTES], offer: &Offer) {
    write_keyed(key(OFFER_PREFIX, id), &offer.encode());
}

unsafe fn read_order(id: u64) -> Option<Order> {
    Order::decode(read_keyed(order_key(id))?)
}

unsafe fn write_order(id: u64, order: &Order) {
    write_keyed(order_key(id), &order.encode());
}

unsafe fn read_stake(id: u64, order: &Order) -> Option<u128> {
    let raw = read_keyed(stake_key(id))?;
    let stake = u128::from_be_bytes(raw.try_into().ok()?);
    if stake < order.paid { return None; }
    Some(stake)
}

unsafe fn read_proof_rate(id: u64) -> Option<u128> {
    let raw = read_keyed(proof_rate_key(id))?;
    let rate = u128::from_be_bytes(raw.try_into().ok()?);
    if rate == 0 { return None; }
    Some(rate)
}

unsafe fn read_u128(key_name: &[u8]) -> u128 {
    read_keyed(key(key_name, b"")).and_then(|raw| raw.try_into().ok()).map(u128::from_be_bytes).unwrap_or(0)
}

unsafe fn write_u128(key_name: &[u8], value: u128) {
    write_keyed(key(key_name, b""), &value.to_be_bytes());
}

unsafe fn next_order() -> u64 {
    read_keyed(key(NEXT_ORDER_KEY, b"")).and_then(|raw| raw.try_into().ok()).map(u64::from_be_bytes).unwrap_or(1)
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
    if rate == 0 { refused(); }
    rate.checked_mul(BOND_GAS_BUDGET)
        .and_then(|value| value.checked_mul(BOND_FEE_MARGIN))
        .unwrap_or_else(|| refused())
}

fn fallback_reimbursement(bond: u128, rate: u128) -> u128 {
    match rate.checked_mul(FALLBACK_REIMBURSE_GAS_BUDGET) {
        Some(cost) => bond.min(cost),
        None => bond,
    }
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

/// A valid content seal does not attest that validation later accepted every
/// original CID. The public witness supplies that independent session check.
fn fallback_claim_matches<F>(
    journal_bytes: &[u8],
    seal: &[u8],
    wire: &[u8],
    offer: &Offer,
    contract: &[u8; 20],
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
    verdict.flips == offer.covered && verifier::verify_fast_content_image(journal_bytes, seal, &FAST_CONTENT_V5_IMAGE_ID)
}

fn parse_start_arg(raw: &[u8]) -> Option<(u64, &[u8], &[u8], &[u8])> {
    if raw.len() < START_HEADER_BYTES
        || raw.len() > START_HEADER_BYTES + MAX_SESSION_WIRE_BYTES
    {
        return None;
    }
    let wire_len = u16::from_be_bytes(raw[START_HEADER_BYTES - 2..START_HEADER_BYTES].try_into().ok()?) as usize;
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
        if shift >= 64 { return None; }
        value |= u64::from(b & 0x7f) << shift;
        if b & 0x80 == 0 { return Some(value); }
        shift += 7;
    }
}

fn protobuf_field(message: &[u8], wanted: u32) -> Option<&[u8]> {
    let mut at = 0;
    let mut found = None;
    while at < message.len() {
        let tag = protobuf_varint(message, &mut at)?;
        let field = (tag >> 3) as u32;
        if field == 0 { return None; }
        match tag & 7 {
            0 => { protobuf_varint(message, &mut at)?; }
            1 => at = at.checked_add(8)?,
            2 => {
                let len = protobuf_varint(message, &mut at)? as usize;
                let end = at.checked_add(len)?;
                let value = message.get(at..end)?;
                at = end;
                if field == wanted { found = Some(value); }
            }
            5 => at = at.checked_add(4)?,
            _ => return None,
        }
        if at > message.len() { return None; }
    }
    found
}

unsafe fn header_state_root(height: u64) -> Option<[u8; 32]> {
    let header = read_region(block_header(height))?;
    if header.is_empty() || header.len() > HEADER_READ_BYTES { return None; }
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
    if addr == 0 || amount == 0 { refused(); }
    create_transfer_promise(addr, amount);
}

unsafe fn settle(id: u64, order: &mut Order, offer: &mut Offer, retired: bool) {
    let expected_state = if retired { FALLBACK_PENDING } else { FUNDED };
    if offer.open_order != id || order.state != expected_state || order.offer != offer_id(&offer.author, offer.epoch) || order.paid != offer.price {
        refused();
    }
    let stake = if retired { read_stake(id, order).unwrap_or_else(|| refused()) } else { 0 };
    let total = order.paid.checked_add(order.bond).and_then(|sum| sum.checked_add(stake)).unwrap_or_else(|| refused());
    let pending = read_u128(PENDING_KEY);
    if order.bond == 0 || pending < total || amount(balance()).unwrap_or(0) < pending { refused(); }
    let reimbursement = if retired {
        fallback_reimbursement(order.bond, read_proof_rate(id).unwrap_or_else(|| refused()))
    } else { 0 };
    let returned_bond = order.bond - reimbursement;
    let seller_payout = order.paid.checked_add(stake).and_then(|sum| sum.checked_add(reimbursement)).unwrap_or_else(|| refused());
    write_u128(PENDING_KEY, pending - total);
    order.state = SETTLED;
    offer.open_order = 0;
    offer.retired = retired;
    write_order(id, order);
    write_offer(&order.offer, offer);
    transfer(&offer.author, seller_payout);
    if returned_bond > 0 { transfer(&order.buyer, returned_bond); }
}

unsafe fn ok() -> u32 {
    let value = b"{\"ok\":true}";
    REPLY_BUF[..value.len()].copy_from_slice(value);
    point_region(core::ptr::addr_of_mut!(REPLY_REGION), core::ptr::addr_of_mut!(REPLY_BUF) as *mut u8, REPLY_BUF.len() as u32, value.len() as u32)
}

fn refused() -> ! { panic!("fod direct refused") }

#[no_mangle]
pub extern "C" fn deploy(_arg: u32) -> u32 { unsafe { ok() } }

/// `epoch:u16 | covered:u32 | CID-inventory commitment:32 | keys commitment:32 | price:u128`.
/// The author may list without a slow proof. Buyers must independently check
/// canonical session state and fetch every original CID before funding.
#[no_mangle]
pub extern "C" fn list(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != OFFER_ARG_BYTES { refused(); }
        let author = address(caller()).unwrap_or_else(|| refused());
        let epoch = u16::from_be_bytes(raw[0..2].try_into().unwrap());
        let covered = u32_at(raw, 2);
        let inventory: [u8; 32] = raw[6..38].try_into().unwrap();
        let keys: [u8; 32] = raw[38..70].try_into().unwrap();
        let price = u128::from_be_bytes(raw[70..86].try_into().unwrap());
        if covered == 0 || covered > MAX_FLIPS || inventory == [0; 32] || keys == [0; 32] || price == 0 { refused(); }
        let id = offer_id(&author, epoch);
        if read_offer(&id).is_some() { refused(); }
        write_offer(&id, &Offer { author, epoch, covered, inventory, keys, price, next_nonce: 1, open_order: 0, retired: false });
        ok()
    }
}

/// `offer id:32 | buyer's dedicated uncompressed delivery public key:65`.
/// The listed price and a refundable proof-cost bond are locked. One order at a time prevents a public
/// fallback from exposing keys while another order can still refund.
#[no_mangle]
pub extern "C" fn buy(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != BUY_ARG_BYTES { refused(); }
        let id: [u8; 32] = raw[..32].try_into().unwrap();
        let delivery_key: [u8; 65] = raw[32..97].try_into().unwrap();
        if delivery_key[0] != 4 || delivery_key[1..] == [0u8; 64] { refused(); }
        let mut offer = read_offer(&id).unwrap_or_else(|| refused());
        if !offer.can_buy() { refused(); }
        let minimum_bond = required_bond();
        let total = amount(pay_amount()).unwrap_or_else(|| refused());
        let bond = total.checked_sub(offer.price).unwrap_or_else(|| refused());
        if bond < minimum_bond { refused(); }
        let buyer = address(caller()).unwrap_or_else(|| refused());
        let order_id = next_order();
        let next = order_id.checked_add(1).unwrap_or_else(|| refused());
        let pending = read_u128(PENDING_KEY).checked_add(total).unwrap_or_else(|| refused());
        if amount(balance()).unwrap_or(0) < pending { refused(); }
        let nonce = offer.next_nonce;
        offer.next_nonce = nonce.checked_add(1).unwrap_or_else(|| refused());
        offer.open_order = order_id;
        write_offer(&id, &offer);
        write_order(order_id, &Order { offer: id, buyer, delivery_key, paid: offer.price, bond, funded_at: block_number(), nonce, state: FUNDED, delivery_hash: [0; 32] });
        write_u128(PENDING_KEY, pending);
        write_keyed(key(NEXT_ORDER_KEY, b""), &next.to_be_bytes());
        ok()
    }
}

/// `order id:u64 | sha256(exact private delivery ciphertext):32`. A buyer may
/// confirm a funded order immediately after local verification, including after
/// the seller's proof window, until a refund actually lands.
#[no_mangle]
pub extern "C" fn confirm(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != CONFIRM_ARG_BYTES { refused(); }
        let id = u64_at(raw, 0);
        let mut order = read_order(id).unwrap_or_else(|| refused());
        if order.state != FUNDED || order.buyer != address(caller()).unwrap_or_else(|| refused()) { refused(); }
        let hash: [u8; 32] = raw[8..40].try_into().unwrap();
        if hash == [0; 32] { refused(); }
        order.delivery_hash = hash;
        let mut offer = read_offer(&order.offer).unwrap_or_else(|| refused());
        settle(id, &mut order, &mut offer, false);
        ok()
    }
}

/// `order id:u64 | fast-content journal:273 | Groth16 seal:256 |
/// session length:u16 | bounded public four-leaf session witness`. This
/// secret-free payable transaction verifies both the pre-session content proof
/// and the later canonical session before locking the author's stake.
#[no_mangle]
pub extern "C" fn startFallback(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        let (id, journal, seal, wire) = parse_start_arg(raw).unwrap_or_else(|| refused());
        let mut order = read_order(id).unwrap_or_else(|| refused());
        if !order.can_start_fallback(block_number(), PROOF_WINDOW_BLOCKS, BUYER_CONFIRM_GRACE_BLOCKS, PROOF_DEADLINE_MARGIN_BLOCKS) { refused(); }
        let offer = read_offer(&order.offer).unwrap_or_else(|| refused());
        if offer.retired || offer.open_order != id || order.offer != offer_id(&offer.author, offer.epoch)
            || order.paid != offer.price || offer.author != address(caller()).unwrap_or_else(|| refused())
        { refused(); }
        let rate = amount(min_fee_per_gas()).unwrap_or_else(|| refused());
        if rate == 0 { refused(); }
        let minimum = rate.checked_mul(SELLER_STAKE_GAS_BUDGET).unwrap_or_else(|| refused()).max(offer.price);
        let stake = amount(pay_amount()).unwrap_or_else(|| refused());
        if stake < minimum { refused(); }
        let liability = order.paid.checked_add(order.bond).unwrap_or_else(|| refused());
        let previous_pending = read_u128(PENDING_KEY);
        let pending = previous_pending.checked_add(stake).unwrap_or_else(|| refused());
        if previous_pending < liability || amount(balance()).unwrap_or(0) < pending { refused(); }
        let contract = address(own_addr()).unwrap_or_else(|| refused());
        if !fallback_claim_matches(journal, seal, wire, &offer, &contract, |height| header_state_root(height))
        { refused(); }
        write_keyed(stake_key(id), &stake.to_be_bytes());
        write_keyed(proof_rate_key(id), &rate.to_be_bytes());
        order.state = FALLBACK_PENDING;
        write_order(id, &order);
        write_u128(PENDING_KEY, pending);
        ok()
    }
}

/// `order id:u64 | epoch secrets:106`. The prior startFallback transition
/// verified the complete original CID inventory and the two keys against the
/// proof and canonical session roots. Public disclosure is the recovery path;
/// private-message delivery cannot be proven from a sender's assertion.
#[no_mangle]
pub extern "C" fn proveAndReveal(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != REVEAL_ARG_BYTES { refused(); }
        let id = u64_at(raw, 0);
        let mut order = read_order(id).unwrap_or_else(|| refused());
        if !order.seller_can_reveal(block_number(), PROOF_WINDOW_BLOCKS) { refused(); }
        let mut offer = read_offer(&order.offer).unwrap_or_else(|| refused());
        if offer.retired || offer.open_order != id { refused(); }
        if offer.author != address(caller()).unwrap_or_else(|| refused()) { refused(); }
        let secrets = &raw[8..];
        if !secrets_match_offer(secrets, &offer) { refused(); }
        settle(id, &mut order, &mut offer, true);
        // Retain the exact public keys in canonical state so a buyer needs no
        // transaction indexer, seller, or social account for recovery.
        write_keyed(reveal_key(id), secrets);
        ok()
    }
}

/// Anyone may refund at the deadline. A proved fallback without key reveal
/// forfeits the seller's stake to the buyer.
#[no_mangle]
pub extern "C" fn refund(arg: u32) -> u32 {
    unsafe {
        let raw = read_region(arg).unwrap_or_else(|| refused());
        if raw.len() != 8 { refused(); }
        let id = u64_at(raw, 0);
        let mut order = read_order(id).unwrap_or_else(|| refused());
        let stake = if order.state == FALLBACK_PENDING {
            read_stake(id, &order).unwrap_or_else(|| refused())
        } else { 0 };
        if !order.can_refund(block_number(), PROOF_WINDOW_BLOCKS) { refused(); }
        let mut offer = read_offer(&order.offer).unwrap_or_else(|| refused());
        if offer.open_order != id || offer.retired || order.offer != offer_id(&offer.author, offer.epoch) || order.paid != offer.price { refused(); }
        let total = order.paid.checked_add(order.bond).and_then(|sum| sum.checked_add(stake)).unwrap_or_else(|| refused());
        let pending = read_u128(PENDING_KEY);
        if pending < total || amount(balance()).unwrap_or(0) < pending { refused(); }
        write_u128(PENDING_KEY, pending - total);
        order.state = REFUNDED;
        offer.open_order = 0;
        offer.retired = true;
        write_order(id, &order);
        write_offer(&order.offer, &offer);
        transfer(&order.buyer, total);
        ok()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn statement_requires_exact_offer_and_contract_binding() {
        let offer = Offer { author: [1; 20], epoch: 230, covered: 5, inventory: [2; 32], keys: [3; 32], price: 1, next_nonce: 1, open_order: 1, retired: false };
        let mut journal = Journal {
            network: NETWORK_DIGEST, contract: [4; 20], author: offer.author,
            epoch: offer.epoch, count: offer.covered as u16,
            consensus: CONSENSUS_DIGEST, policy: POLICY_DIGEST,
            original_inventory: offer.inventory, copies: [0; 32],
            point_commitment: [0; 33], original_keys: offer.keys,
        };
        assert!(statement_matches_offer(&journal, &offer, &[4; 20]));
        journal.original_inventory[2] ^= 1;
        assert!(!statement_matches_offer(&journal, &offer, &[4; 20]));
        journal.original_inventory[2] ^= 1;
        assert!(!statement_matches_offer(&journal, &offer, &[5; 20]));
    }

    #[test]
    fn plain_hash_preimage_is_not_a_fallback_claim() {
        let mut secrets = [0u8; SECRETS_BYTES];
        secrets[..SECRETS_DOMAIN.len()].copy_from_slice(SECRETS_DOMAIN);
        secrets[SECRETS_DOMAIN.len()..SECRETS_DOMAIN.len() + 2].copy_from_slice(&230u16.to_be_bytes());
        secrets[SECRETS_DOMAIN.len() + 2..SECRETS_DOMAIN.len() + 22].copy_from_slice(&[1; 20]);
        let offer = Offer { author: [1; 20], epoch: 230, covered: 5, inventory: [2; 32], keys: verifier::sha256(&secrets), price: 1, next_nonce: 1, open_order: 1, retired: false };
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

#[cfg(test)]
mod v5_tests;
