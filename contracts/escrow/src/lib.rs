//! Flips on Demand pool escrow: registration, reserve, orders, credits.
//!
//! It implements the ownerless pool of `SPEC.md` 5.1 and 7 over the Idena
//! contract ABI that Package 1 validated (docs/SOURCES.md F-12):
//!
//! - required exports: `memory`, `allocate`, `deploy`, plus one export per method
//! - arguments arrive as pointers to Regions; returns are pointers to Regions
//! - host functions are imported from module `env`
//! - the runtime rejects bulk-memory and float operators, so modules are lowered
//!   with `wasm-opt --llvm-memory-copy-fill-lowering` before deployment
//!
//! Both proof verifications are here: `registerBundle` checks a sealed
//! registration receipt against the claim its pinned guest program produces,
//! and `fulfill` checks a delivery receipt the same way (`verifier.rs`). A
//! bundle enters the pool through `registerBundle` alone - the `test-hooks`
//! build adds `seedPoolBundle`/`seedPoolOrder` so the accounting can be
//! exercised without a proof, and the release build carries no such export.
//!
//! Deliberately absent, and rejected rather than faked: an owner, an upgrade
//! hook, a terminate path, and any way to move the reserve that is not a
//! market rule: a buy, a sell quote, a refund, or a withdrawal of credit.

#![no_std]
#![allow(static_mut_refs)]

extern crate alloc;

use core::panic::PanicInfo;

mod pool;
mod verifier;
#[cfg(feature = "fast-candidate")]
mod fast;

/// `fulfill` argument header: order id, journal, seal; the ciphertext follows.
const FULFILL_HEADER_BYTES: usize = 8 + verifier::JOURNAL_BYTES + verifier::SEAL_BYTES;
/// SHA-256 commitment to the canonical epoch secrets.
const COMMITMENT_BYTES: usize = 32;
/// iDNA amounts are stored as 128-bit big-endian integers. `u64` holds only
/// about 18.4 iDNA once the 18 decimals are counted, which is far below any
/// realistic price, so amounts that size silently saturated (see `O-11` in
/// `SPEC.md`).
const AMOUNT_BYTES: usize = 16;
/// A canonical header is a few hundred bytes; anything larger is not one.
const HEADER_READ_BYTES: usize = 16 * 1024;

/// `registerBundle` argument: the registration journal (`SOURCES.md` F-69),
/// then the Groth16 seal.
const REGISTRATION_ARG_BYTES: usize = verifier::REGISTRATION_JOURNAL_BYTES + verifier::SEAL_BYTES;
/// Offsets inside the version 2 registration journal, matching
/// `proofs/session-core/src/statement.rs` field for field.
///
/// The version 2 statement binds two anchored state versions - the flips and the
/// verdict of one session (`SOURCES.md` F-97) - so the log is longer than the
/// version 1 one and every field after the covered flips moved.
const REGISTRATION_INVENTORY_HEIGHT_OFFSET: usize = 50;
const REGISTRATION_INVENTORY_ROOT_OFFSET: usize = 58;
const REGISTRATION_VERDICT_HEIGHT_OFFSET: usize = 90;
const REGISTRATION_VERDICT_ROOT_OFFSET: usize = 98;
const REGISTRATION_VALIDATION_FLAGS_OFFSET: usize = 130;
const REGISTRATION_KEY_COMMITMENT_OFFSET: usize = 166;
/// Version 3 fields: the commitment over the bundle's CIDs, which the pool
/// record republishes so a buyer can check a candidate set against chain state,
/// and the digest of the canonical network name.
const REGISTRATION_INVENTORY_COMMITMENT_OFFSET: usize = 198;
const REGISTRATION_POLICY_DIGEST_OFFSET: usize = 230;
const REGISTRATION_CONSENSUS_DIGEST_OFFSET: usize = 262;
const REGISTRATION_NETWORK_DIGEST_OFFSET: usize = 294;
/// Offset of the manifest commitment inside the registration journal.
const REGISTRATION_MANIFEST_OFFSET: usize = 134;
/// Offset of the covered flip count inside the registration journal.
const REGISTRATION_COVERED_FLIPS_OFFSET: usize = 46;
/// The label digests the registry accepts, each the SHA-256 of the exact string:
/// `fod:eligibility:v1` and the consensus version this release pins.
const ELIGIBILITY_POLICY_DIGEST: [u8; 32] = [
    0x06, 0xb3, 0x1d, 0x89, 0x13, 0x36, 0xe0, 0xb2, 0x0f, 0xce, 0xc9, 0x98, 0x3c, 0xfb, 0x03, 0x03,
    0x18, 0x6b, 0x04, 0xbc, 0x0d, 0xa9, 0x06, 0xc9, 0x26, 0xd5, 0x7e, 0x25, 0xa7, 0xf3, 0x39, 0x9f,
];
const CONSENSUS_VERSION_DIGEST: [u8; 32] = [
    0xca, 0xc2, 0xb1, 0x7b, 0x70, 0xe1, 0x12, 0x49, 0x91, 0x23, 0xc4, 0x96, 0x92, 0x8e, 0x2f, 0x8e,
    0xb9, 0xba, 0x1b, 0x78, 0x56, 0x05, 0x18, 0x51, 0x9c, 0x34, 0x5f, 0xfb, 0xc1, 0xd5, 0xef, 0xdf,
];
/// The network this module was built for, as the SHA-256 of the canonical name
/// `idena` - the same string `proofs/registration-v2/host/src/main.rs` proves
/// and `tools/fod` names in its action requests. A statement proved for another
/// network carries another digest and is refused, so a receipt cannot move
/// between deployments that call their chain differently. A build for a
/// different network replaces this constant with that network's digest.
const NETWORK_DIGEST: [u8; 32] = [
    0xf4, 0xb1, 0x51, 0x2d, 0x62, 0x54, 0x23, 0x6f, 0x6c, 0x27, 0xa5, 0x38, 0x77, 0x0e, 0x79, 0x02,
    0x24, 0x07, 0x6e, 0xf8, 0x8b, 0x24, 0xd5, 0xbc, 0xe8, 0xda, 0x73, 0x20, 0x09, 0x34, 0x76, 0xd2,
];
/// An uncompressed secp256k1 public key, which is what the runtime's
/// `ecrecover` returns.
const DELIVERY_KEY_BYTES: usize = 65;
/// A `[R || S || V]` secp256k1 signature, the format the node signs and recovers.
const SIGNATURE_BYTES: usize = 65;
/// Domain separator for the delivery-key possession proof.
const DELIVERY_KEY_DOMAIN: &[u8] = b"fod:delivery-key:v1";
/// The delivery deadline, in blocks after the buy is included: the approved
/// hard local-lab maximum. Delivery, confirmation and settlement may run while
/// block is strictly before funded+3; repayment starts at funded+3. The
/// buyer may explicitly confirm later until the order has refunded.
const DEADLINE_BLOCKS: u64 = 3;
/// The retrieval-fee cap per authenticated eligible item (play money): 1 iDNA.
const FEE_PER_ITEM: u128 = 1_000_000_000_000_000_000;
/// At most this many retrieval hosts per purchase, and at most this many bytes
/// of committed secrets in a reveal settlement.
const MAX_FEE_ENTRIES: usize = 16;
const MAX_SECRETS_BYTES: usize = 512;
/// A public reveal exposes one bundle's keys to every buyer. Bound the number
/// of open orders it must settle atomically, including their fee credits.
const MAX_OPEN_POOL_ORDERS: usize = 8;

/// Reserve-funded supply is switched off in this build. A registration receipt
/// is public transaction data: it proves that a proof exists, not that whoever
/// presents it may collect a payout, and it cannot make copyable keys scarce
/// (`REVIEW.md` A01, A05; `SPEC.md` 5.1). Paying units out of a reserve stays
/// disabled until a finite, authenticated issuance rule exists (`SPEC.md`
/// 10.2, package D2). This is a build constant, not an administrative switch.
const SUPPLY_PAYOUTS_ENABLED: bool = false;

/// The runtime has no unwinding: a panic becomes a trap. Only the wasm artefact
/// defines it, so host-side unit tests can still build against std.
#[cfg(target_arch = "wasm32")]
#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    core::arch::wasm32::unreachable()
}

/// Bump allocator for the verifier's internal vectors. The runtime caps guest
/// linear memory, so this is a fixed 1 MiB region that is never freed: a
/// verification allocates a bounded amount and the instance ends with the call.
#[cfg(target_arch = "wasm32")]
mod bump {
    use core::alloc::{GlobalAlloc, Layout};
    use core::sync::atomic::{AtomicUsize, Ordering};

    const HEAP_SIZE: usize = 1024 * 1024;
    static mut HEAP: [u8; HEAP_SIZE] = [0; HEAP_SIZE];
    static HEAP_NEXT: AtomicUsize = AtomicUsize::new(0);

    pub struct Bump;

    unsafe impl GlobalAlloc for Bump {
        unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
            let align = layout.align().max(8);
            let start = HEAP_NEXT.load(Ordering::SeqCst);
            let aligned = (start + align - 1) & !(align - 1);
            let end = aligned.saturating_add(layout.size());
            if end > HEAP_SIZE {
                return core::ptr::null_mut();
            }
            HEAP_NEXT.store(end, Ordering::SeqCst);
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

/// Arena for regions the runtime asks the contract to provide: arguments,
/// storage reads, and host results. Every request gets its own region.
#[cfg(not(feature = "fast-candidate"))]
const ALLOC_ARENA_SIZE: usize = 64 * 1024;
#[cfg(feature = "fast-candidate")]
const ALLOC_ARENA_SIZE: usize = 128 * 1024;
const VAL_BUF_SIZE: usize = 4 * 1024;
const KEY_BUF_SIZE: usize = 128;
const OUT_BUF_SIZE: usize = 1024;
/// Scratch space for storage values read back from the host. The host writes
/// into memory through `allocate`, so read buffers come from the same heap.
const READ_BUF_SIZE: usize = 8 * 1024;
const HEAP_SIZE: usize = 4 * 1024;

static mut ALLOC_ARENA: [u8; ALLOC_ARENA_SIZE] = [0; ALLOC_ARENA_SIZE];
static mut ALLOC_NEXT: usize = 0;
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
static mut OUT_BUF: [u8; OUT_BUF_SIZE] = [0; OUT_BUF_SIZE];
static mut OUT_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};
static mut READ_BUF: [u8; READ_BUF_SIZE] = [0; READ_BUF_SIZE];
static mut HEAP: [u8; HEAP_SIZE] = [0; HEAP_SIZE];
static mut HEAP_USED: usize = 0;

/// Bump-allocates a Region (header plus bytes) from the static heap.
///
/// Host imports that take several Region arguments at once need each argument in
/// its own memory, because the host reads them in sequence during the call.
unsafe fn heap_region(bytes: &[u8]) -> u32 {
    let align = 4usize;
    let start = (HEAP_USED + align - 1) & !(align - 1);
    let data_len = bytes.len();
    let total = core::mem::size_of::<Region>() + data_len;
    if start + total > HEAP_SIZE {
        return 0;
    }
    let base = core::ptr::addr_of_mut!(HEAP) as *mut u8;
    let region = base.add(start) as *mut Region;
    let data = base.add(start + core::mem::size_of::<Region>());
    core::ptr::copy_nonoverlapping(bytes.as_ptr(), data, data_len);
    (*region).offset = data as u32;
    (*region).capacity = data_len as u32;
    (*region).length = data_len as u32;
    HEAP_USED = start + total;
    region as u32
}

#[link(wasm_import_module = "env")]
extern "C" {
    fn set_storage(key: u32, value: u32);
    fn get_storage(key: u32) -> u32;
    fn block_number() -> u64;
    fn caller() -> u32;
    fn pay_amount() -> u32;
    fn balance() -> u32;
    fn create_transfer_promise(addr: u32, amount: u32);
    fn own_addr() -> u32;
    fn keccak256(data: u32) -> u32;
    fn ecrecover(data: u32, signature: u32) -> u32;
    fn block_header(height: u64) -> u32;
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
        // One region per request. A single shared buffer would be overwritten by
        // the next host write - reading a stored record while still parsing the
        // call argument, for example - and the contract would silently work on
        // destroyed input.
        let align = 4usize;
        let header = core::mem::size_of::<Region>();
        let start = (ALLOC_NEXT + align - 1) & !(align - 1);
        let data = start + header;
        let end = data + size as usize;
        if end > ALLOC_ARENA_SIZE {
            return 0;
        }
        ALLOC_NEXT = end;
        let base = core::ptr::addr_of_mut!(ALLOC_ARENA) as *mut u8;
        let region = base.add(start) as *mut Region;
        (*region).offset = base.add(data) as u32;
        (*region).capacity = size;
        (*region).length = 0;
        region as u32
    }
}

/// Reads a Region the runtime wrote: its `length` field holds the number of
/// bytes, while `capacity` stays at the size the contract allocated
/// (`idena-wasm` `src/memory.rs`, `write_region`).
unsafe fn read_arg(ptr: u32) -> (*const u8, usize) {
    if ptr == 0 {
        return (core::ptr::null(), 0);
    }
    let region = ptr as *const Region;
    if (*region).offset == 0 {
        return (core::ptr::null(), 0);
    }
    ((*region).offset as *const u8, (*region).length as usize)
}

unsafe fn parse_u64(ptr: u32) -> u64 {
    let (bytes, len) = read_arg(ptr);
    if bytes.is_null() {
        return 0;
    }
    let mut i = 0;
    let mut value = 0u64;
    while i < len && i < 24 {
        let c = *bytes.add(i);
        if !c.is_ascii_digit() {
            break;
        }
        value = value.saturating_mul(10).saturating_add((c - b'0') as u64);
        i += 1;
    }
    value
}

fn push_bytes(buf: &mut [u8], len: &mut usize, bytes: &[u8]) {
    for b in bytes {
        if *len < buf.len() {
            buf[*len] = *b;
            *len += 1;
        }
    }
}

fn push_u64(buf: &mut [u8], len: &mut usize, mut value: u64) {
    let mut tmp = [0u8; 20];
    let mut n = 0;
    if value == 0 {
        tmp[0] = b'0';
        n = 1;
    }
    while value > 0 {
        tmp[n] = b'0' + (value % 10) as u8;
        value /= 10;
        n += 1;
    }
    while n > 0 {
        n -= 1;
        if *len < buf.len() {
            buf[*len] = tmp[n];
            *len += 1;
        }
    }
}

/// Pushes an amount as decimal text. Amounts are 128-bit, so they cannot go
/// through the u64 printer.
fn push_u128(buf: &mut [u8], len: &mut usize, mut value: u128) {
    let mut tmp = [0u8; 40];
    let mut n = 0usize;
    if value == 0 {
        tmp[0] = b'0';
        n = 1;
    }
    while value > 0 {
        tmp[n] = b'0' + (value % 10) as u8;
        value /= 10;
        n += 1;
    }
    while n > 0 {
        n -= 1;
        if *len < buf.len() {
            buf[*len] = tmp[n];
            *len += 1;
        }
    }
}

/// Pushes a u64 in big-endian bytes, for values that are hashed rather than
/// displayed.
fn push_u64_be(buf: &mut [u8], len: &mut usize, value: u64) {
    for b in value.to_be_bytes() {
        if *len < buf.len() {
            buf[*len] = b;
            *len += 1;
        }
    }
}

fn push_hex(buf: &mut [u8], len: &mut usize, bytes: &[u8]) {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    for b in bytes {
        if *len + 2 > buf.len() {
            return;
        }
        buf[*len] = HEX[(b >> 4) as usize];
        buf[*len + 1] = HEX[(b & 0x0f) as usize];
        *len += 2;
    }
}

unsafe fn reply(body: &[u8]) -> u32 {
    let buf = core::ptr::addr_of_mut!(OUT_BUF) as *mut u8;
    let n = body.len().min(OUT_BUF_SIZE);
    core::ptr::copy_nonoverlapping(body.as_ptr(), buf, n);
    // The runtime does not surface a top-level call's returned payload to the
    // host API, so the harness needs the result somewhere readable. Under the
    // test-hooks feature only, mirror it into storage; the release artefact
    // writes nothing extra.
    #[cfg(feature = "test-hooks")]
    {
        let key = key_raw(b"spike-last");
        let val = point_region(
            core::ptr::addr_of_mut!(VAL_REGION),
            buf,
            OUT_BUF_SIZE as u32,
            n as u32,
        );
        set_storage(key, val);
    }
    point_region(
        core::ptr::addr_of_mut!(OUT_REGION),
        buf,
        OUT_BUF_SIZE as u32,
        n as u32,
    )
}

fn ok_reply() -> u32 {
    unsafe { reply(b"{\"ok\":true}") }
}

/// Refuses the call by aborting it.
///
/// A refusal must be a failed call: the node discards the storage and the value
/// of a call that fails (`idena-go` `vm/wasm/vm.go` commits only when
/// `err == nil`), while a JSON `{"ok":false}` reply is a *successful* call that
/// keeps whatever the caller sent (`REVIEW.md` A04, `SPEC.md` 7.2). The reason
/// stays in the argument for readers of the source; a wasm trap carries no
/// payload to the node.
fn error_reply(code: &[u8]) -> u32 {
    let _ = code;
    panic!("fod:refused")
}

// ---------------------------------------------------------------------------
// Storage helpers.
// ---------------------------------------------------------------------------

/// Builds a storage key such as `offer/12` and points KEY_REGION at it.
unsafe fn key_for(prefix: &[u8], id: u64) -> u32 {
    let buf = core::ptr::addr_of_mut!(KEY_BUF) as *mut u8;
    let mut len = 0usize;
    for b in prefix {
        if len < KEY_BUF_SIZE {
            *buf.add(len) = *b;
            len += 1;
        }
    }
    let mut tmp = [0u8; 20];
    let mut n = 0usize;
    let mut value = id;
    if value == 0 {
        tmp[0] = b'0';
        n = 1;
    }
    while value > 0 {
        tmp[n] = b'0' + (value % 10) as u8;
        value /= 10;
        n += 1;
    }
    while n > 0 {
        n -= 1;
        if len < KEY_BUF_SIZE {
            *buf.add(len) = tmp[n];
            len += 1;
        }
    }
    point_region(
        core::ptr::addr_of_mut!(KEY_REGION),
        buf,
        KEY_BUF_SIZE as u32,
        len as u32,
    )
}

/// Builds a storage key from a prefix and a full address. Addresses must never
/// be truncated into a numeric id: two buyers would then share one record.
unsafe fn key_for_address(prefix: &[u8], addr: &[u8; 20]) -> u32 {
    let buf = core::ptr::addr_of_mut!(KEY_BUF) as *mut u8;
    let mut len = 0usize;
    for b in prefix {
        if len < KEY_BUF_SIZE {
            *buf.add(len) = *b;
            len += 1;
        }
    }
    for b in addr {
        if len < KEY_BUF_SIZE {
            *buf.add(len) = *b;
            len += 1;
        }
    }
    point_region(
        core::ptr::addr_of_mut!(KEY_REGION),
        buf,
        KEY_BUF_SIZE as u32,
        len as u32,
    )
}

/// Builds a storage key from a prefix and a full key commitment. Bundles are
/// keyed by the commitment registration binds, so a record cannot be moved to
/// another bundle by renumbering anything.
unsafe fn key_for_commitment(prefix: &[u8], commitment: &[u8; COMMITMENT_BYTES]) -> u32 {
    let buf = core::ptr::addr_of_mut!(KEY_BUF) as *mut u8;
    let mut len = 0usize;
    for b in prefix {
        if len < KEY_BUF_SIZE {
            *buf.add(len) = *b;
            len += 1;
        }
    }
    for b in commitment {
        if len < KEY_BUF_SIZE {
            *buf.add(len) = *b;
            len += 1;
        }
    }
    point_region(
        core::ptr::addr_of_mut!(KEY_REGION),
        buf,
        KEY_BUF_SIZE as u32,
        len as u32,
    )
}

/// Reads a storage value into a slice of the static read buffer.
/// Builds a storage key with no id suffix, for singleton entries such as the
/// test-hook reply mirror.
unsafe fn key_raw(name: &[u8]) -> u32 {
    let buf = core::ptr::addr_of_mut!(KEY_BUF) as *mut u8;
    let len = name.len().min(KEY_BUF_SIZE);
    core::ptr::copy_nonoverlapping(name.as_ptr(), buf, len);
    point_region(
        core::ptr::addr_of_mut!(KEY_REGION),
        buf,
        KEY_BUF_SIZE as u32,
        len as u32,
    )
}

unsafe fn read_keyed(key: u32) -> Option<&'static [u8]> {
    let value_ptr = get_storage(key);
    if value_ptr == 0 {
        return None;
    }
    let region = value_ptr as *const Region;
    let len = (*region).length as usize;
    let src = (*region).offset as *const u8;
    if src.is_null() || len == 0 {
        return None;
    }
    let dst = core::ptr::addr_of_mut!(READ_BUF) as *mut u8;
    let n = len.min(READ_BUF_SIZE);
    core::ptr::copy_nonoverlapping(src, dst, n);
    Some(core::slice::from_raw_parts(dst, n))
}

unsafe fn read_value(prefix: &[u8], id: u64) -> Option<&'static [u8]> {
    read_keyed(key_for(prefix, id))
}

unsafe fn write_keyed(key: u32, value: &[u8]) {
    let buf = core::ptr::addr_of_mut!(VAL_BUF) as *mut u8;
    let n = value.len().min(VAL_BUF_SIZE);
    core::ptr::copy_nonoverlapping(value.as_ptr(), buf, n);
    let val = point_region(
        core::ptr::addr_of_mut!(VAL_REGION),
        buf,
        VAL_BUF_SIZE as u32,
        n as u32,
    );
    set_storage(key, val);
}

unsafe fn write_value(prefix: &[u8], id: u64, value: &[u8]) {
    write_keyed(key_for(prefix, id), value)
}

const CREDIT_PREFIX: &[u8] = b"credit/";
const CREDIT_TOTAL_PREFIX: &[u8] = b"credits";
/// Per-address ownership of what each donor put into the reserve. The reserve
/// total is one number, but a contribution is not a gift to the world: its
/// donor can withdraw what is still there (`AH-15`).
const RESERVE_CREDIT_PREFIX: &[u8] = b"reserve-credit/";
/// The two other liabilities, tracked the same way as `credits` so the
/// conservation invariant can be checked without iterating storage:
/// `liquid >= reserve0 + pending0 + credits0`.
const RESERVE_TOTAL_PREFIX: &[u8] = b"reserve";
const PENDING_TOTAL_PREFIX: &[u8] = b"pending";
const UNITS_TOTAL_PREFIX: &[u8] = b"units";

unsafe fn caller_address() -> [u8; 20] {
    let mut addr = [0u8; 20];
    let ptr = caller();
    if ptr == 0 {
        return addr;
    }
    let region = ptr as *const Region;
    let src = (*region).offset as *const u8;
    let len = (*region).length as usize;
    if src.is_null() || len == 0 {
        return addr;
    }
    let n = len.min(20);
    core::ptr::copy_nonoverlapping(src, addr.as_mut_ptr(), n);
    addr
}

/// The contract's own address, as the delivery statement has to name it.
unsafe fn own_address() -> [u8; 20] {
    let mut addr = [0u8; 20];
    let ptr = own_addr();
    if ptr == 0 {
        return addr;
    }
    let region = ptr as *const Region;
    let src = (*region).offset as *const u8;
    let len = (*region).length as usize;
    if src.is_null() || len == 0 {
        return addr;
    }
    let n = len.min(20);
    core::ptr::copy_nonoverlapping(src, addr.as_mut_ptr(), n);
    addr
}

/// Reads a big-endian iDNA amount from a region the host wrote. Amounts are
/// 128-bit. Anything larger saturates at `u128::MAX` rather than wrapping, so a
/// comparison against a stored price can only fail: an unrepresentable amount is
/// refused, never quietly underpaid.
unsafe fn read_amount(ptr: u32) -> u128 {
    if ptr == 0 {
        return 0;
    }
    let region = ptr as *const Region;
    let src = (*region).offset as *const u8;
    let len = (*region).length as usize;
    if src.is_null() {
        return 0;
    }
    let mut value: u128 = 0;
    for i in 0..len.min(32) {
        value = value
            .saturating_mul(256)
            .saturating_add(*src.add(i) as u128);
    }
    value
}

/// The value an executing call carries (`pay_amount`).
///
/// The pinned runtime sets the call context's amount from `tx.Amount`, which the
/// transaction decoder leaves nil when no value was sent, and the binding's C
/// shim dereferences whatever the host returns: **calling this on a zero-value
/// call traps inside the host before the guest sees anything**. Methods that
/// must work with no value therefore do not read it and accept that an
/// unexpected value becomes unattributed surplus; `buy` and `addReserve` do read
/// it, because those are the calls that carry a value.
unsafe fn pay_amount_value() -> u128 {
    read_amount(pay_amount())
}

/// One length-delimited protobuf field's payload, or `None` when the field is
/// absent or the message is malformed. Unknown fields are skipped, and the last
/// occurrence of a wanted field wins, which is protobuf's own rule.
fn protobuf_message_field(message: &[u8], wanted: u32) -> Option<&[u8]> {
    let mut at = 0usize;
    let mut found = None;
    while at < message.len() {
        let key = protobuf_varint(message, &mut at)?;
        let field = (key >> 3) as u32;
        let wire = key & 7;
        if field == 0 {
            return None;
        }
        match wire {
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

fn protobuf_varint(message: &[u8], at: &mut usize) -> Option<u64> {
    let mut value: u64 = 0;
    let mut shift = 0u32;
    loop {
        let byte = *message.get(*at)?;
        *at += 1;
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

/// The state root the executing node's own chain carries for one height.
///
/// The runtime answers `block_header(height)` with the node's canonical header
/// (`SOURCES.md` F-96), protobuf-encoded: the proposed shape carries the state
/// root in field 6 and the skip-block shape in field 3. A height this node does
/// not hold, or a header that does not parse, is `None` - and a registration
/// that cannot be checked against the chain is refused rather than assumed.
unsafe fn header_state_root(height: u64) -> Option<[u8; COMMITMENT_BYTES]> {
    let ptr = block_header(height);
    if ptr == 0 {
        return None;
    }
    let region = ptr as *const Region;
    if (*region).offset == 0 {
        return None;
    }
    let len = (*region).length as usize;
    if len == 0 || len > HEADER_READ_BYTES {
        return None;
    }
    let header = core::slice::from_raw_parts((*region).offset as *const u8, len);
    let (shape, sub) = match protobuf_message_field(header, 1) {
        Some(sub) => (1u32, sub),
        None => (2u32, protobuf_message_field(header, 2)?),
    };
    let root_field = if shape == 1 { 6 } else { 3 };
    let root = protobuf_message_field(sub, root_field)?;
    if root.len() != COMMITMENT_BYTES {
        return None;
    }
    let mut out = [0u8; COMMITMENT_BYTES];
    out.copy_from_slice(root);
    Some(out)
}

/// The contract's own balance, as the node defines it
/// (`wasm_env.go Balance` reads the contract address).
unsafe fn contract_balance() -> u128 {
    read_amount(balance())
}

unsafe fn credit_of(addr: &[u8; 20]) -> u128 {
    match read_keyed(key_for_address(CREDIT_PREFIX, addr)) {
        Some(bytes) if bytes.len() >= AMOUNT_BYTES => {
            let mut raw = [0u8; AMOUNT_BYTES];
            raw.copy_from_slice(&bytes[..AMOUNT_BYTES]);
            u128::from_be_bytes(raw)
        }
        _ => 0,
    }
}

unsafe fn set_credit(addr: &[u8; 20], amount: u128) {
    write_keyed(key_for_address(CREDIT_PREFIX, addr), &amount.to_be_bytes());
}

unsafe fn reserve_credit_of(addr: &[u8; 20]) -> u128 {
    match read_keyed(key_for_address(RESERVE_CREDIT_PREFIX, addr)) {
        Some(bytes) if bytes.len() >= AMOUNT_BYTES => {
            let mut raw = [0u8; AMOUNT_BYTES];
            raw.copy_from_slice(&bytes[..AMOUNT_BYTES]);
            u128::from_be_bytes(raw)
        }
        _ => 0,
    }
}

unsafe fn set_reserve_credit(addr: &[u8; 20], amount: u128) {
    write_keyed(
        key_for_address(RESERVE_CREDIT_PREFIX, addr),
        &amount.to_be_bytes(),
    );
}

/// Sum of all outstanding withdrawal credits. Tracked so the accounting
/// invariant `liquid >= reserve + credits` can be checked without iterating
/// storage, which a contract cannot do.
unsafe fn credit_total() -> u128 {
    match read_value(CREDIT_TOTAL_PREFIX, 0) {
        Some(bytes) if bytes.len() >= AMOUNT_BYTES => {
            let mut raw = [0u8; AMOUNT_BYTES];
            raw.copy_from_slice(&bytes[..AMOUNT_BYTES]);
            u128::from_be_bytes(raw)
        }
        _ => 0,
    }
}

unsafe fn set_credit_total(total: u128) {
    write_value(CREDIT_TOTAL_PREFIX, 0, &total.to_be_bytes());
}

unsafe fn running_total(prefix: &[u8]) -> u128 {
    match read_value(prefix, 0) {
        Some(bytes) if bytes.len() >= AMOUNT_BYTES => {
            let mut raw = [0u8; AMOUNT_BYTES];
            raw.copy_from_slice(&bytes[..AMOUNT_BYTES]);
            u128::from_be_bytes(raw)
        }
        _ => 0,
    }
}

unsafe fn set_running_total(prefix: &[u8], total: u128) {
    write_value(prefix, 0, &total.to_be_bytes());
}

unsafe fn reserve_total() -> u128 {
    running_total(RESERVE_TOTAL_PREFIX)
}

unsafe fn pending_total() -> u128 {
    running_total(PENDING_TOTAL_PREFIX)
}

unsafe fn units_total() -> u128 {
    running_total(UNITS_TOTAL_PREFIX)
}

/// The one pool's own state. The owner answered the liquidity-scope decision on
/// 2026-09-29 with "one global pool": a single reserve and sale-unit count stand
/// for the whole market, while each bundle keeps its own inventory, covered
/// count, strikes, beacon and nonce. The three numbers live in running totals
/// so the conservation check can read them without iterating storage.
unsafe fn global_pool() -> pool::Pool {
    pool::Pool {
        reserve: reserve_total(),
        units: units_total().min(u128::from(u64::MAX)) as u64,
        pending: pending_total(),
    }
}

unsafe fn set_global_pool(state: pool::Pool) {
    set_running_total(RESERVE_TOTAL_PREFIX, state.reserve);
    set_running_total(PENDING_TOTAL_PREFIX, state.pending);
    set_running_total(UNITS_TOTAL_PREFIX, u128::from(state.units));
}

// ---------------------------------------------------------------------------
// Contract methods.
// ---------------------------------------------------------------------------

#[no_mangle]
pub extern "C" fn deploy(_arg: u32) -> u32 {
    ok_reply()
}

fn journal_u32(journal: &[u8], offset: usize) -> u32 {
    let mut raw = [0u8; 4];
    raw.copy_from_slice(&journal[offset..offset + 4]);
    u32::from_be_bytes(raw)
}

fn journal_u64(journal: &[u8], offset: usize) -> u64 {
    let mut raw = [0u8; 8];
    raw.copy_from_slice(&journal[offset..offset + 8]);
    u64::from_be_bytes(raw)
}

/// Withdrawal: consume the caller's credit and hand the value over as a
/// transfer.
///
/// The pinned runtime debits the contract balance inside
/// `create_transfer_promise` (`idena-wasm` `src/imports.rs`), so a credit the
/// contract cannot cover aborts this call, and the node discards the state of a
/// failed call (`idena-go` `vm/wasm/vm.go` commits only when `err == nil`). That
/// makes the payout atomic, which is also why no promise callback is used here:
/// in `idena-wasm` revision 7e113895 the `Action::Transfer` arm runs none.
#[no_mangle]
pub extern "C" fn withdraw(arg: u32) -> u32 {
    let _ = arg;
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let caller = caller_address();
        let credit = credit_of(&caller);
        if credit == 0 {
            return error_reply(b"no_credit");
        }
        // Effects first: this call either pays the whole credit or leaves no
        // trace at all, so a second withdrawal finds nothing to take.
        set_credit(&caller, 0);
        set_credit_total(credit_total().saturating_sub(credit));

        // Each argument needs its own region: the host reads them in sequence
        // during the call, so a shared buffer would corrupt the second read.
        let addr_region = heap_region(&caller);
        let amount_region = heap_region(&credit.to_be_bytes());
        create_transfer_promise(addr_region, amount_region);
    }
    ok_reply()
}

/// Read-only accounting view, used by tests and by the CLI before submitting.
#[no_mangle]
pub extern "C" fn accounting(arg: u32) -> u32 {
    let _ = arg;
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let credit = credit_of(&caller_address());
        let mut buf = [0u8; 512];
        let mut len = 0usize;
        push_bytes(&mut buf, &mut len, b"{\"callerCredit\":");
        push_u128(&mut buf, &mut len, credit);
        push_bytes(&mut buf, &mut len, b",\"callerReserve\":");
        push_u128(&mut buf, &mut len, reserve_credit_of(&caller_address()));
        push_bytes(&mut buf, &mut len, b",\"creditTotal\":");
        push_u128(&mut buf, &mut len, credit_total());
        push_bytes(&mut buf, &mut len, b",\"reserveTotal\":");
        push_u128(&mut buf, &mut len, reserve_total());
        push_bytes(&mut buf, &mut len, b",\"pendingTotal\":");
        push_u128(&mut buf, &mut len, pending_total());
        push_bytes(&mut buf, &mut len, b",\"unitsTotal\":");
        push_u128(&mut buf, &mut len, units_total());
        push_bytes(&mut buf, &mut len, b",\"liquid\":");
        push_u128(&mut buf, &mut len, contract_balance());
        push_bytes(&mut buf, &mut len, b"}");
        reply(&buf[..len])
    }
}

/// Test-only pool seed: commitment (32) || reserve (16) || units (8) || beacon
/// block (8), with the caller as the author. Stands in for the registration
/// proof and the funding a test cannot sign.
#[cfg(feature = "test-hooks")]
#[no_mangle]
pub extern "C" fn seedPoolBundle(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len != COMMITMENT_BYTES + AMOUNT_BYTES + 8 + 8 {
            return error_reply(b"bad_args");
        }
        let mut commitment = [0u8; COMMITMENT_BYTES];
        core::ptr::copy_nonoverlapping(bytes, commitment.as_mut_ptr(), COMMITMENT_BYTES);
        let mut reserve = [0u8; AMOUNT_BYTES];
        core::ptr::copy_nonoverlapping(
            bytes.add(COMMITMENT_BYTES),
            reserve.as_mut_ptr(),
            AMOUNT_BYTES,
        );
        let mut units = [0u8; 8];
        core::ptr::copy_nonoverlapping(
            bytes.add(COMMITMENT_BYTES + AMOUNT_BYTES),
            units.as_mut_ptr(),
            8,
        );
        let mut beacon = [0u8; 8];
        core::ptr::copy_nonoverlapping(
            bytes.add(COMMITMENT_BYTES + AMOUNT_BYTES + 8),
            beacon.as_mut_ptr(),
            8,
        );
        // The seed hook carries no receipt, so it names no covered count.
        let mut bundle = PoolBundle::new(
            caller_address(),
            [0u8; COMMITMENT_BYTES],
            [0u8; COMMITMENT_BYTES],
            0,
        );
        bundle.units = u64::from_be_bytes(units);
        bundle.beacon_block = u64::from_be_bytes(beacon);
        // The seed stands in for a real funding call, so the global totals have
        // to see it: the conservation check reads the totals, not the bundles.
        let seeded = u128::from_be_bytes(reserve);
        set_running_total(RESERVE_TOTAL_PREFIX, reserve_total().saturating_add(seeded));
        set_running_total(
            UNITS_TOTAL_PREFIX,
            units_total().saturating_add(u128::from(bundle.units)),
        );
        write_pool_bundle(&commitment, &bundle);
        ok_reply()
    }
}

/// Test-only order seed: id (8) || commitment (32) || buyer (20) || delivery
/// Test-only covered-count seed: commitment (32) || covered (4). The seed hook
/// above carries no receipt, so it names no covered count; a fee test needs one
/// because the retrieval-fee cap is `covered * FEE_PER_ITEM`.
#[cfg(feature = "test-hooks")]
#[no_mangle]
pub extern "C" fn seedPoolCovered(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len != COMMITMENT_BYTES + 4 {
            return error_reply(b"bad_args");
        }
        let mut commitment = [0u8; COMMITMENT_BYTES];
        core::ptr::copy_nonoverlapping(bytes, commitment.as_mut_ptr(), COMMITMENT_BYTES);
        let mut covered = [0u8; 4];
        core::ptr::copy_nonoverlapping(bytes.add(COMMITMENT_BYTES), covered.as_mut_ptr(), 4);
        let mut bundle = match registered_pool_bundle(&commitment) {
            Ok(bundle) => bundle,
            Err(reply) => return reply,
        };
        bundle.covered = u32::from_be_bytes(covered);
        write_pool_bundle(&commitment, &bundle);
        ok_reply()
    }
}

/// key (65) || paid (16) || funded at (8) || nonce (8), opened in the funded
/// state.
#[cfg(feature = "test-hooks")]
#[no_mangle]
pub extern "C" fn seedPoolOrder(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null()
            || len != 8 + COMMITMENT_BYTES + 20 + DELIVERY_KEY_BYTES + AMOUNT_BYTES + 8 + 8
        {
            return error_reply(b"bad_args");
        }
        let mut id_bytes = [0u8; 8];
        core::ptr::copy_nonoverlapping(bytes, id_bytes.as_mut_ptr(), 8);
        let id = u64::from_be_bytes(id_bytes);
        let mut at = 8;
        let mut order = PoolOrder {
            bundle: [0u8; COMMITMENT_BYTES],
            buyer: [0u8; 20],
            delivery_key: [0u8; DELIVERY_KEY_BYTES],
            paid: 0,
            fee: 0,
            funded_at: 0,
            nonce: 0,
            state: POOL_ORDER_FUNDED,
        };
        core::ptr::copy_nonoverlapping(bytes.add(at), order.bundle.as_mut_ptr(), COMMITMENT_BYTES);
        at += COMMITMENT_BYTES;
        core::ptr::copy_nonoverlapping(bytes.add(at), order.buyer.as_mut_ptr(), 20);
        at += 20;
        core::ptr::copy_nonoverlapping(
            bytes.add(at),
            order.delivery_key.as_mut_ptr(),
            DELIVERY_KEY_BYTES,
        );
        at += DELIVERY_KEY_BYTES;
        let mut paid = [0u8; AMOUNT_BYTES];
        core::ptr::copy_nonoverlapping(bytes.add(at), paid.as_mut_ptr(), AMOUNT_BYTES);
        order.paid = u128::from_be_bytes(paid);
        at += AMOUNT_BYTES;
        let mut funded = [0u8; 8];
        core::ptr::copy_nonoverlapping(bytes.add(at), funded.as_mut_ptr(), 8);
        order.funded_at = u64::from_be_bytes(funded);
        at += 8;
        let mut nonce = [0u8; 8];
        core::ptr::copy_nonoverlapping(bytes.add(at), nonce.as_mut_ptr(), 8);
        order.nonce = u64::from_be_bytes(nonce);
        // A seeded order stands in for a separate buyer payment. The harness
        // must fund that pending escrow in addition to any seeded reserve.
        if registered_pool_bundle(&order.bundle).is_err() {
            return error_reply(b"bundle_not_found");
        }
        let mut open = match read_open_pool_orders(&order.bundle) {
            Some(open) => open,
            None => return error_reply(b"invalid_open_orders"),
        };
        if read_pool_order(id).is_some() || !open.push(id) {
            return error_reply(b"too_many_open_orders");
        }
        // A seeded order stands in for a purchase the buyer already paid: the
        // harness credits the contract balance for the payment, and this hook
        // opens the order with it in the pool's pending escrow.
        set_running_total(
            PENDING_TOTAL_PREFIX,
            match pending_total().checked_add(order.paid) {
                Some(total) => total,
                None => return error_reply(b"overflow"),
            },
        );
        write_pool_order(id, &order);
        write_open_pool_orders(&order.bundle, &open);
        ok_reply()
    }
}

// ---------------------------------------------------------------------------
// The pool (SPEC.md 5.1, 7.1): the supply side (registration, the reserve, the
// sale units, the delivery beacon) and the demand side (buy, deliver, confirm,
// fulfill, refund, and the strike).
// ---------------------------------------------------------------------------

/// One bundle's pool state, keyed by the commitment registration binds.
struct PoolBundle {
    /// The author the registration proof named. Only an address the claim names
    /// may register; any holder may sell a unit by presenting the receipt.
    author: [u8; 20],
    /// The manifest commitment the claim carries, so a reader can compare it
    /// with what the chain holds.
    manifest_commitment: [u8; COMMITMENT_BYTES],
    /// The commitment over the bundle's CIDs, from the same statement: what
    /// lets a buyer check a candidate set against chain state instead of
    /// trusting the seller's advisory (`PROGRESS.md`, the C1 finding).
    inventory_commitment: [u8; COMMITMENT_BYTES],
    units: u64,
    /// The block of the last beacon, and how many orders the bundle missed.
    beacon_block: u64,
    strikes: u32,
    /// Advances on every buy, so a delivery-key proof cannot be replayed.
    nonce: u64,
    /// The authenticated covered flip count the registration statement bound
    /// (`SPEC.md` 5.5 item 7): the receipt's own count, not one a caller names.
    /// A listing can therefore say how many flips a bundle carries, and a fee
    /// rule bounded by eligible flips has the number to bound it with.
    covered: u32,
}

/// One bundle's own record. The pool's reserve, sale units and pending escrow
/// are global (`global_pool`), so this record keeps only what belongs to the
/// bundle: its author, the two commitments, its remaining inventory, the
/// beacon, its strikes, the buy nonce and the authenticated covered count.
const POOL_VALUE_BYTES: usize = 20 + COMMITMENT_BYTES + COMMITMENT_BYTES + 8 + 8 + 4 + 8 + 4;
const POOL_MANIFEST_OFFSET: usize = 20;
const POOL_INVENTORY_OFFSET: usize = POOL_MANIFEST_OFFSET + COMMITMENT_BYTES;
const POOL_UNITS_OFFSET: usize = POOL_INVENTORY_OFFSET + COMMITMENT_BYTES;
const POOL_BEACON_OFFSET: usize = POOL_UNITS_OFFSET + 8;
const POOL_STRIKES_OFFSET: usize = POOL_BEACON_OFFSET + 8;
const POOL_NONCE_OFFSET: usize = POOL_STRIKES_OFFSET + 4;
const POOL_COVERED_OFFSET: usize = POOL_NONCE_OFFSET + 8;

impl PoolBundle {
    fn new(
        author: [u8; 20],
        manifest_commitment: [u8; COMMITMENT_BYTES],
        inventory_commitment: [u8; COMMITMENT_BYTES],
        covered: u32,
    ) -> PoolBundle {
        PoolBundle {
            author,
            manifest_commitment,
            inventory_commitment,
            units: 0,
            beacon_block: 0,
            strikes: 0,
            nonce: 0,
            covered,
        }
    }

    fn encode(&self) -> [u8; POOL_VALUE_BYTES] {
        let mut out = [0u8; POOL_VALUE_BYTES];
        out[..20].copy_from_slice(&self.author);
        out[POOL_MANIFEST_OFFSET..POOL_INVENTORY_OFFSET].copy_from_slice(&self.manifest_commitment);
        out[POOL_INVENTORY_OFFSET..POOL_UNITS_OFFSET].copy_from_slice(&self.inventory_commitment);
        out[POOL_UNITS_OFFSET..POOL_BEACON_OFFSET].copy_from_slice(&self.units.to_be_bytes());
        out[POOL_BEACON_OFFSET..POOL_STRIKES_OFFSET]
            .copy_from_slice(&self.beacon_block.to_be_bytes());
        out[POOL_STRIKES_OFFSET..POOL_NONCE_OFFSET].copy_from_slice(&self.strikes.to_be_bytes());
        out[POOL_NONCE_OFFSET..POOL_COVERED_OFFSET].copy_from_slice(&self.nonce.to_be_bytes());
        out[POOL_COVERED_OFFSET..POOL_VALUE_BYTES].copy_from_slice(&self.covered.to_be_bytes());
        out
    }

    fn decode(bytes: &[u8]) -> Option<PoolBundle> {
        if bytes.len() < POOL_VALUE_BYTES {
            return None;
        }
        let mut author = [0u8; 20];
        let mut manifest = [0u8; COMMITMENT_BYTES];
        let mut inventory = [0u8; COMMITMENT_BYTES];
        let mut units = [0u8; 8];
        let mut beacon = [0u8; 8];
        let mut strikes = [0u8; 4];
        let mut nonce = [0u8; 8];
        let mut covered = [0u8; 4];
        author.copy_from_slice(&bytes[..20]);
        manifest.copy_from_slice(&bytes[POOL_MANIFEST_OFFSET..POOL_INVENTORY_OFFSET]);
        inventory.copy_from_slice(&bytes[POOL_INVENTORY_OFFSET..POOL_UNITS_OFFSET]);
        units.copy_from_slice(&bytes[POOL_UNITS_OFFSET..POOL_BEACON_OFFSET]);
        beacon.copy_from_slice(&bytes[POOL_BEACON_OFFSET..POOL_STRIKES_OFFSET]);
        strikes.copy_from_slice(&bytes[POOL_STRIKES_OFFSET..POOL_NONCE_OFFSET]);
        nonce.copy_from_slice(&bytes[POOL_NONCE_OFFSET..POOL_COVERED_OFFSET]);
        covered.copy_from_slice(&bytes[POOL_COVERED_OFFSET..POOL_VALUE_BYTES]);
        Some(PoolBundle {
            author,
            manifest_commitment: manifest,
            inventory_commitment: inventory,
            units: u64::from_be_bytes(units),
            beacon_block: u64::from_be_bytes(beacon),
            strikes: u32::from_be_bytes(strikes),
            nonce: u64::from_be_bytes(nonce),
            covered: u32::from_be_bytes(covered),
        })
    }
}

const POOL_PREFIX: &[u8] = b"pool/";
/// Set once a bundle's secrets are published: the listing is spent and every
/// still-open order on it has settled in the same call.
const POOL_REVEALED_PREFIX: &[u8] = b"pool-revealed/";
/// Public keys released by settlement, retained per order for offline recovery.
const POOL_REVEAL_PREFIX: &[u8] = b"preveal/";
const POOL_COMMIT_ARG_BYTES: usize = COMMITMENT_BYTES;
const POOL_FUND_ARG_BYTES: usize = COMMITMENT_BYTES + AMOUNT_BYTES;

unsafe fn read_pool_bundle(commitment: &[u8; COMMITMENT_BYTES]) -> Option<PoolBundle> {
    read_keyed(key_for_commitment(POOL_PREFIX, commitment)).and_then(PoolBundle::decode)
}

unsafe fn write_pool_bundle(commitment: &[u8; COMMITMENT_BYTES], bundle: &PoolBundle) {
    write_keyed(
        key_for_commitment(POOL_PREFIX, commitment),
        &bundle.encode(),
    );
}

unsafe fn pool_commitment_arg(bytes: *const u8, len: usize) -> Result<[u8; COMMITMENT_BYTES], u32> {
    if bytes.is_null() || len < POOL_COMMIT_ARG_BYTES {
        return Err(error_reply(b"bad_args"));
    }
    let mut commitment = [0u8; COMMITMENT_BYTES];
    core::ptr::copy_nonoverlapping(bytes, commitment.as_mut_ptr(), COMMITMENT_BYTES);
    Ok(commitment)
}

unsafe fn registered_pool_bundle(commitment: &[u8; COMMITMENT_BYTES]) -> Result<PoolBundle, u32> {
    match read_pool_bundle(commitment) {
        Some(bundle) => Ok(bundle),
        None => Err(error_reply(b"bundle_not_found")),
    }
}

unsafe fn pool_revealed(commitment: &[u8; COMMITMENT_BYTES]) -> bool {
    read_keyed(key_for_commitment(POOL_REVEALED_PREFIX, commitment)).is_some()
}

/// Marks a bundle revealed and removes its remaining units from the one global
/// count, so the shared quote prices only inventory that is still saleable.
unsafe fn reveal_registered_bundle(commitment: &[u8; COMMITMENT_BYTES]) {
    if pool_revealed(commitment) {
        return;
    }
    write_keyed(key_for_commitment(POOL_REVEALED_PREFIX, commitment), &[1u8]);
    if let Some(mut bundle) = read_pool_bundle(commitment) {
        if bundle.units > 0 {
            let remaining = u128::from(bundle.units);
            set_running_total(UNITS_TOTAL_PREFIX, units_total().saturating_sub(remaining));
            bundle.units = 0;
            write_pool_bundle(commitment, &bundle);
        }
    }
}

/// Registers a bundle into the pool. The proof has to be the session-bound one
/// (`proofs/registration` folds the anchor bundle and refuses an unbound input),
/// and the caller has to be the author the claim names. Later holders sell units
/// by presenting the same receipt, which `sellUnit` checks differently.
#[cfg(not(feature = "fast-candidate"))]
#[no_mangle]
pub extern "C" fn registerBundle(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len != REGISTRATION_ARG_BYTES {
            return error_reply(b"bad_args");
        }
        let journal = core::slice::from_raw_parts(bytes, verifier::REGISTRATION_JOURNAL_BYTES);
        let seal = core::slice::from_raw_parts(
            bytes.add(verifier::REGISTRATION_JOURNAL_BYTES),
            verifier::SEAL_BYTES,
        );

        // The journal is not trusted until the seal verifies; the checks that
        // cost nothing run first, the receipt last.
        if journal_u32(journal, 0) != verifier::REGISTRATION_JOURNAL_VERSION {
            return error_reply(b"stale_statement");
        }
        if journal[4..24] != own_address() {
            return error_reply(b"wrong_contract");
        }
        if journal[24..44] != caller_address() {
            return error_reply(b"not_author");
        }
        if journal[REGISTRATION_POLICY_DIGEST_OFFSET..REGISTRATION_CONSENSUS_DIGEST_OFFSET]
            != ELIGIBILITY_POLICY_DIGEST
        {
            return error_reply(b"unsupported_policy");
        }
        if journal[REGISTRATION_CONSENSUS_DIGEST_OFFSET..REGISTRATION_NETWORK_DIGEST_OFFSET]
            != CONSENSUS_VERSION_DIGEST
        {
            return error_reply(b"unsupported_consensus");
        }
        // The statement names the network its evidence came from; this module
        // serves exactly one, so a receipt proved elsewhere stops here.
        if journal[REGISTRATION_NETWORK_DIGEST_OFFSET
            ..REGISTRATION_NETWORK_DIGEST_OFFSET + COMMITMENT_BYTES]
            != NETWORK_DIGEST
        {
            return error_reply(b"foreign_network");
        }
        if journal_u32(journal, REGISTRATION_COVERED_FLIPS_OFFSET as usize) == 0 {
            return error_reply(b"empty_bundle");
        }
        // The three status flags are the only ones the record can carry; a
        // journal with anything else has no session behind it.
        if journal_u32(journal, REGISTRATION_VALIDATION_FLAGS_OFFSET) & !0b111 != 0 {
            return error_reply(b"bad_flags");
        }

        // The session's two state versions have to be the ones this node's own
        // chain carries (`SPEC.md` 5.5): a receipt that folds evidence under a
        // root the chain never committed cannot register, whatever it proves
        // internally. Both reads are cheap host calls, so they run before the
        // pairing-heavy receipt check.
        let inventory_height = journal_u64(journal, REGISTRATION_INVENTORY_HEIGHT_OFFSET);
        let inventory_root = match header_state_root(inventory_height) {
            Some(root) => root,
            None => return error_reply(b"no_inventory_header"),
        };
        let mut claimed_root = [0u8; COMMITMENT_BYTES];
        core::ptr::copy_nonoverlapping(
            journal.as_ptr().add(REGISTRATION_INVENTORY_ROOT_OFFSET),
            claimed_root.as_mut_ptr(),
            COMMITMENT_BYTES,
        );
        if inventory_root != claimed_root {
            return error_reply(b"wrong_inventory_root");
        }
        let verdict_height = journal_u64(journal, REGISTRATION_VERDICT_HEIGHT_OFFSET);
        let verdict_root = match header_state_root(verdict_height) {
            Some(root) => root,
            None => return error_reply(b"no_verdict_header"),
        };
        core::ptr::copy_nonoverlapping(
            journal.as_ptr().add(REGISTRATION_VERDICT_ROOT_OFFSET),
            claimed_root.as_mut_ptr(),
            COMMITMENT_BYTES,
        );
        if verdict_root != claimed_root {
            return error_reply(b"wrong_verdict_root");
        }

        let mut key_commitment = [0u8; COMMITMENT_BYTES];
        core::ptr::copy_nonoverlapping(
            journal.as_ptr().add(REGISTRATION_KEY_COMMITMENT_OFFSET),
            key_commitment.as_mut_ptr(),
            COMMITMENT_BYTES,
        );
        if read_pool_bundle(&key_commitment).is_some() {
            return error_reply(b"already_registered");
        }
        if !verifier::verify_registration(journal, seal) {
            return error_reply(b"invalid_proof");
        }

        let mut manifest = [0u8; COMMITMENT_BYTES];
        core::ptr::copy_nonoverlapping(
            journal.as_ptr().add(REGISTRATION_MANIFEST_OFFSET),
            manifest.as_mut_ptr(),
            COMMITMENT_BYTES,
        );
        let mut inventory_commitment = [0u8; COMMITMENT_BYTES];
        core::ptr::copy_nonoverlapping(
            journal
                .as_ptr()
                .add(REGISTRATION_INVENTORY_COMMITMENT_OFFSET),
            inventory_commitment.as_mut_ptr(),
            COMMITMENT_BYTES,
        );
        write_pool_bundle(
            &key_commitment,
            &PoolBundle::new(
                caller_address(),
                manifest,
                inventory_commitment,
                journal_u32(journal, REGISTRATION_COVERED_FLIPS_OFFSET as usize),
            ),
        );
        ok_reply()
    }
}

/// Adds the amount the call carries to a bundle's reserve. Anyone may fund a
/// bundle: the ratio between the reserve and the units is the quote, and no one
/// sets a price.
#[no_mangle]
pub extern "C" fn addReserve(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len != POOL_FUND_ARG_BYTES {
            return error_reply(b"bad_args");
        }
        let commitment = match pool_commitment_arg(bytes, len) {
            Ok(commitment) => commitment,
            Err(reply) => return reply,
        };
        let _bundle = match registered_pool_bundle(&commitment) {
            Ok(bundle) => bundle,
            Err(reply) => return reply,
        };
        let mut amount_bytes = [0u8; AMOUNT_BYTES];
        core::ptr::copy_nonoverlapping(
            bytes.add(COMMITMENT_BYTES),
            amount_bytes.as_mut_ptr(),
            AMOUNT_BYTES,
        );
        let amount = u128::from_be_bytes(amount_bytes);
        if amount == 0 {
            return error_reply(b"zero_amount");
        }
        if pay_amount_value() != amount {
            return error_reply(b"wrong_amount");
        }
        // A deposit only becomes a reserve claim once the contract's own
        // balance carries it. The node credits the value before the guest runs
        // (`blockchain/blockchain.go`, `AddBalance(contractAddr, amount)`), so a
        // reserve the balance does not cover is refused here instead of being
        // written and then stranding every later settlement behind
        // `settle_order`'s solvency guard (the lab wedge of 2026-10-02:
        // `docs/PROGRESS.md`, audit log).
        let needed = match reserve_total()
            .checked_add(amount)
            .and_then(|total| total.checked_add(pending_total()))
            .and_then(|total| total.checked_add(credit_total()))
        {
            Some(total) => total,
            None => return error_reply(b"overflow"),
        };
        if contract_balance() < needed {
            return error_reply(b"unfunded_deposit");
        }
        // One global pool: the deposit lands in the single reserve, whether or
        // not the listing that named it still carries its own units.
        set_running_total(
            RESERVE_TOTAL_PREFIX,
            match reserve_total().checked_add(amount) {
                Some(total) => total,
                None => return error_reply(b"overflow"),
            },
        );
        // The donation is not a gift to nobody: it is recorded against the
        // address that paid it, and `withdrawReserve` can hand it back while it
        // is still in the reserve (`AH-15`).
        let caller = caller_address();
        set_reserve_credit(
            &caller,
            match reserve_credit_of(&caller).checked_add(amount) {
                Some(owned) => owned,
                None => return error_reply(b"overflow"),
            },
        );
        ok_reply()
    }
}

/// Withdraws the caller's own reserve contribution.
///
/// This is the reserve's only outflow: the reserve is donated liquidity and
/// nothing else spends it. The call refuses unless the reserve still covers the
/// caller's credit and the contract's balance still covers every buyer escrow
/// and host credit after the payout, so a withdrawal can never reach another
/// party's money, and effects are applied before the transfer promise exactly
/// as `withdraw` does (`AH-15`).
#[no_mangle]
pub extern "C" fn withdrawReserve(arg: u32) -> u32 {
    let _ = arg;
    unsafe {
        let caller = caller_address();
        let owned = reserve_credit_of(&caller);
        if owned == 0 {
            return error_reply(b"no_reserve_credit");
        }
        let remaining = match reserve_total().checked_sub(owned) {
            Some(remaining) => remaining,
            None => return error_reply(b"reserve_too_small"),
        };
        let protected = match pending_total().checked_add(credit_total()) {
            Some(protected) => protected,
            None => return error_reply(b"overflow"),
        };
        let needed = match remaining.checked_add(protected) {
            Some(needed) => needed,
            None => return error_reply(b"overflow"),
        };
        if contract_balance() < needed {
            return error_reply(b"not_enough_balance");
        }
        // Effects first: the whole credit is paid or nothing moves, so a second
        // withdrawal finds nothing to take.
        set_reserve_credit(&caller, 0);
        set_running_total(RESERVE_TOTAL_PREFIX, remaining);
        let addr_region = heap_region(&caller);
        let amount_region = heap_region(&owned.to_be_bytes());
        create_transfer_promise(addr_region, amount_region);
    }
    ok_reply()
}

/// Adds one sale unit and pays nothing. The unit is offered at the pool's own
/// quote, and its proceeds go to this bundle's registered author when the order
/// settles (never to the reserve).
///
/// Only the bundle's registered author may donate. A free unit is one more
/// claim on the same reserve, so an outsider's donation dilutes the quote every
/// other buyer pays and moves the price without paying for it (`REVIEW.md`
/// A05); the author can only do that to their own bundle. The address is the
/// one the registration proof named, not an operator's choice; the owner
/// answered on 2026-09-29 that donations stay author-only, so this is the
/// accepted rule.
#[no_mangle]
pub extern "C" fn donateUnit(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let (bytes, len) = read_arg(arg);
        let commitment = match pool_commitment_arg(bytes, len) {
            Ok(commitment) => commitment,
            Err(reply) => return reply,
        };
        let mut bundle = match registered_pool_bundle(&commitment) {
            Ok(bundle) => bundle,
            Err(reply) => return reply,
        };
        if caller_address() != bundle.author {
            return error_reply(b"not_author");
        }
        // A listing cleared by two missed orders is unbuyable, so new units
        // there would only dilute the one global quote with inventory no buyer
        // can reach (`AH-11`).
        if bundle.strikes >= 2 {
            return error_reply(b"listing_cleared");
        }
        // A revealed bundle is spent: new units would price a secret every
        // buyer can already read (`AH-12`).
        if pool_revealed(&commitment) {
            return error_reply(b"bundle_revealed");
        }
        bundle.units = match bundle.units.checked_add(1) {
            Some(units) => units,
            None => return error_reply(b"overflow"),
        };
        set_running_total(
            UNITS_TOTAL_PREFIX,
            match units_total().checked_add(1) {
                Some(total) => total,
                None => return error_reply(b"overflow"),
            },
        );
        write_pool_bundle(&commitment, &bundle);
        ok_reply()
    }
}

/// Sells one unit into the pool at the pool's own quote. The caller presents the
/// registration receipt of the bundle: the seal proves possession of the
/// secrets, the author field ties the receipt to the registered bundle, and the
/// caller may be any holder rather than only the author.
#[no_mangle]
pub extern "C" fn sellUnit(arg: u32) -> u32 {
    unsafe {
        // The owner answered the finite-subsidy decision on 2026-09-29 with
        // "no subsidy at all": the pool never funds an acquisition, so this
        // payout path stays refused. The arithmetic below remains only as the
        // disabled shape of the path.
        if !SUPPLY_PAYOUTS_ENABLED {
            return error_reply(b"supply_disabled");
        }
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len != POOL_COMMIT_ARG_BYTES + REGISTRATION_ARG_BYTES {
            return error_reply(b"bad_args");
        }
        let commitment = match pool_commitment_arg(bytes, len) {
            Ok(commitment) => commitment,
            Err(reply) => return reply,
        };
        let mut bundle = match registered_pool_bundle(&commitment) {
            Ok(bundle) => bundle,
            Err(reply) => return reply,
        };
        let journal = core::slice::from_raw_parts(
            bytes.add(POOL_COMMIT_ARG_BYTES),
            verifier::REGISTRATION_JOURNAL_BYTES,
        );
        let seal = core::slice::from_raw_parts(
            bytes.add(POOL_COMMIT_ARG_BYTES + verifier::REGISTRATION_JOURNAL_BYTES),
            verifier::SEAL_BYTES,
        );
        if journal_u32(journal, 0) != verifier::REGISTRATION_JOURNAL_VERSION {
            return error_reply(b"stale_statement");
        }
        if journal[4..24] != own_address() {
            return error_reply(b"wrong_contract");
        }
        if journal[24..44] != bundle.author {
            return error_reply(b"wrong_author");
        }
        if journal[REGISTRATION_KEY_COMMITMENT_OFFSET
            ..REGISTRATION_KEY_COMMITMENT_OFFSET + COMMITMENT_BYTES]
            != commitment
        {
            return error_reply(b"wrong_bundle");
        }
        if !verifier::verify_registration(journal, seal) {
            return error_reply(b"invalid_proof");
        }

        let mut state = global_pool();
        let payout = match state.sell() {
            Ok(payout) => payout,
            Err(pool::PoolError::ReserveTooSmall) => return error_reply(b"reserve_too_small"),
            Err(_) => return error_reply(b"overflow"),
        };
        set_global_pool(state);
        let caller = caller_address();
        let credit = match credit_of(&caller).checked_add(payout) {
            Some(credit) => credit,
            None => return error_reply(b"overflow"),
        };
        set_credit(&caller, credit);
        match credit_total().checked_add(payout) {
            Some(total) => set_credit_total(total),
            None => return error_reply(b"overflow"),
        }
        write_pool_bundle(&commitment, &bundle);

        let mut buf = [0u8; 96];
        let mut out_len = 0usize;
        push_bytes(&mut buf, &mut out_len, b"{\"ok\":true,\"paid\":\"");
        push_u128(&mut buf, &mut out_len, payout);
        push_bytes(&mut buf, &mut out_len, b"\"}");
        reply(&buf[..out_len])
    }
}

/// Refreshes the bundle's delivery beacon. Anyone who will serve the bundle's
/// orders may keep it fresh; a stale beacon blocks new funding.
#[no_mangle]
pub extern "C" fn beacon(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let (bytes, len) = read_arg(arg);
        let commitment = match pool_commitment_arg(bytes, len) {
            Ok(commitment) => commitment,
            Err(reply) => return reply,
        };
        let mut bundle = match registered_pool_bundle(&commitment) {
            Ok(bundle) => bundle,
            Err(reply) => return reply,
        };
        // A fresh beacon is what makes a bundle buyable (`buy` refuses a stale
        // one), so an outsider could beacon a bundle whose desk is absent and
        // lure a purchase into a refund strike against its author. Only the
        // address the registration proof named may say the bundle is reachable;
        // delivery itself stays open to any desk. Who may advertise
        // availability is an owner decision (`PROGRESS.md`, section 2) and this
        // is the narrower rule until it lands.
        if caller_address() != bundle.author {
            return error_reply(b"not_author");
        }
        bundle.beacon_block = block_number();
        write_pool_bundle(&commitment, &bundle);
        ok_reply()
    }
}

/// One bundle's pool state, as the interface reads it.
#[no_mangle]
pub extern "C" fn poolView(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let (bytes, len) = read_arg(arg);
        let commitment = match pool_commitment_arg(bytes, len) {
            Ok(commitment) => commitment,
            Err(reply) => return reply,
        };
        let bundle = match registered_pool_bundle(&commitment) {
            Ok(bundle) => bundle,
            Err(reply) => return reply,
        };
        // Room for every field, including the two 32-byte commitments the
        // record publishes: the push helpers drop what does not fit, so the
        // buffer has to be larger than the longest possible record.
        let mut buf = [0u8; 768];
        let mut len = 0usize;
        push_bytes(&mut buf, &mut len, b"{\"ok\":true,\"author\":\"");
        push_hex(&mut buf, &mut len, &bundle.author);
        push_bytes(&mut buf, &mut len, b"\",\"manifestCommitment\":\"");
        push_hex(&mut buf, &mut len, &bundle.manifest_commitment);
        push_bytes(&mut buf, &mut len, b"\",\"inventoryCommitment\":\"");
        push_hex(&mut buf, &mut len, &bundle.inventory_commitment);
        push_bytes(&mut buf, &mut len, b"\",\"reserve\":\"");
        push_u128(&mut buf, &mut len, reserve_total());
        push_bytes(&mut buf, &mut len, b"\",\"pending\":\"");
        push_u128(&mut buf, &mut len, pending_total());
        push_bytes(&mut buf, &mut len, b"\",\"units\":");
        push_u64(&mut buf, &mut len, bundle.units);
        push_bytes(&mut buf, &mut len, b",\"beaconBlock\":");
        push_u64(&mut buf, &mut len, bundle.beacon_block);
        push_bytes(&mut buf, &mut len, b",\"strikes\":");
        push_u64(&mut buf, &mut len, u64::from(bundle.strikes));
        push_bytes(&mut buf, &mut len, b",\"coveredFlips\":");
        push_u64(&mut buf, &mut len, u64::from(bundle.covered));
        match pool::buy_quote(reserve_total(), global_pool().units) {
            Some(quote) => {
                push_bytes(&mut buf, &mut len, b",\"buyQuote\":\"");
                push_u128(&mut buf, &mut len, quote);
                push_bytes(&mut buf, &mut len, b"\"");
            }
            None => {
                push_bytes(&mut buf, &mut len, b",\"buyQuote\":null");
            }
        }
        match pool::sell_quote(reserve_total(), global_pool().units) {
            Some(quote) => {
                push_bytes(&mut buf, &mut len, b",\"sellQuote\":\"");
                push_u128(&mut buf, &mut len, quote);
                push_bytes(&mut buf, &mut len, b"\"}");
            }
            None => {
                push_bytes(&mut buf, &mut len, b",\"sellQuote\":null}");
            }
        }
        reply(&buf[..len])
    }
}

// The demand side of the pool: what a buyer pays for a unit, the delivery the
// desk publishes, the buyer's confirmation, the proof path that settles without
// the buyer, and the refund with the strike.

/// The beacon's freshness window in blocks: about an hour at the chain's block
/// time, and the value `SPEC.md` 7.2 records.
const BEACON_BLOCKS: u64 = 288;

const POOL_ORDER_PREFIX: &[u8] = b"porder/";
const POOL_DELIVERY_PREFIX: &[u8] = b"pdeliv/";
const POOL_OPEN_ORDERS_PREFIX: &[u8] = b"popen/";
/// The confirmation argument: the order id, then the ciphertext's digest.
const POOL_NEXT_ORDER_PREFIX: &[u8] = b"pnext";
const POOL_BUY_ARG_BYTES: usize = COMMITMENT_BYTES + DELIVERY_KEY_BYTES + SIGNATURE_BYTES;

const POOL_ORDER_FUNDED: u8 = 1;
const POOL_ORDER_DELIVERED: u8 = 2;
const POOL_ORDER_SETTLED: u8 = 3;
const POOL_ORDER_REFUNDED: u8 = 4;

/// One pool order: the unit a buyer took, and what settles it.
struct PoolOrder {
    bundle: [u8; COMMITMENT_BYTES],
    buyer: [u8; 20],
    delivery_key: [u8; DELIVERY_KEY_BYTES],
    paid: u128,
    /// The buyer-funded retrieval fee held with the payment until settlement;
    /// it is returned with the payment when the order times out.
    fee: u128,
    funded_at: u64,
    /// The buy nonce the delivery key was proven against.
    nonce: u64,
    state: u8,
}

const POOL_ORDER_BUNDLE_OFFSET: usize = 0;
const POOL_ORDER_BUYER_OFFSET: usize = COMMITMENT_BYTES;
const POOL_ORDER_KEY_OFFSET: usize = POOL_ORDER_BUYER_OFFSET + 20;
const POOL_ORDER_PAID_OFFSET: usize = POOL_ORDER_KEY_OFFSET + DELIVERY_KEY_BYTES;
const POOL_ORDER_FUNDED_OFFSET: usize = POOL_ORDER_PAID_OFFSET + AMOUNT_BYTES;
const POOL_ORDER_NONCE_OFFSET: usize = POOL_ORDER_FUNDED_OFFSET + 8;
const POOL_ORDER_STATE_OFFSET: usize = POOL_ORDER_NONCE_OFFSET + 8;
const POOL_ORDER_FEE_OFFSET: usize = POOL_ORDER_STATE_OFFSET + 1;
const POOL_ORDER_VALUE_BYTES: usize = POOL_ORDER_FEE_OFFSET + AMOUNT_BYTES;

impl PoolOrder {
    fn encode(&self) -> [u8; POOL_ORDER_VALUE_BYTES] {
        let mut out = [0u8; POOL_ORDER_VALUE_BYTES];
        out[POOL_ORDER_BUNDLE_OFFSET..POOL_ORDER_BUYER_OFFSET].copy_from_slice(&self.bundle);
        out[POOL_ORDER_BUYER_OFFSET..POOL_ORDER_KEY_OFFSET].copy_from_slice(&self.buyer);
        out[POOL_ORDER_KEY_OFFSET..POOL_ORDER_PAID_OFFSET].copy_from_slice(&self.delivery_key);
        out[POOL_ORDER_PAID_OFFSET..POOL_ORDER_FUNDED_OFFSET]
            .copy_from_slice(&self.paid.to_be_bytes());
        out[POOL_ORDER_FUNDED_OFFSET..POOL_ORDER_NONCE_OFFSET]
            .copy_from_slice(&self.funded_at.to_be_bytes());
        out[POOL_ORDER_NONCE_OFFSET..POOL_ORDER_STATE_OFFSET]
            .copy_from_slice(&self.nonce.to_be_bytes());
        out[POOL_ORDER_STATE_OFFSET] = self.state;
        out[POOL_ORDER_FEE_OFFSET..POOL_ORDER_VALUE_BYTES].copy_from_slice(&self.fee.to_be_bytes());
        out
    }

    fn decode(bytes: &[u8]) -> Option<PoolOrder> {
        if bytes.len() < POOL_ORDER_VALUE_BYTES {
            return None;
        }
        let mut bundle = [0u8; COMMITMENT_BYTES];
        let mut buyer = [0u8; 20];
        let mut delivery_key = [0u8; DELIVERY_KEY_BYTES];
        let mut paid = [0u8; AMOUNT_BYTES];
        let mut funded_at = [0u8; 8];
        let mut nonce = [0u8; 8];
        let mut fee = [0u8; AMOUNT_BYTES];
        bundle.copy_from_slice(&bytes[POOL_ORDER_BUNDLE_OFFSET..POOL_ORDER_BUYER_OFFSET]);
        buyer.copy_from_slice(&bytes[POOL_ORDER_BUYER_OFFSET..POOL_ORDER_KEY_OFFSET]);
        delivery_key.copy_from_slice(&bytes[POOL_ORDER_KEY_OFFSET..POOL_ORDER_PAID_OFFSET]);
        paid.copy_from_slice(&bytes[POOL_ORDER_PAID_OFFSET..POOL_ORDER_FUNDED_OFFSET]);
        funded_at.copy_from_slice(&bytes[POOL_ORDER_FUNDED_OFFSET..POOL_ORDER_NONCE_OFFSET]);
        nonce.copy_from_slice(&bytes[POOL_ORDER_NONCE_OFFSET..POOL_ORDER_STATE_OFFSET]);
        let state = bytes[POOL_ORDER_STATE_OFFSET];
        fee.copy_from_slice(&bytes[POOL_ORDER_FEE_OFFSET..POOL_ORDER_VALUE_BYTES]);
        if !(POOL_ORDER_FUNDED..=POOL_ORDER_REFUNDED).contains(&state) {
            return None;
        }
        Some(PoolOrder {
            bundle,
            buyer,
            delivery_key,
            paid: u128::from_be_bytes(paid),
            fee: u128::from_be_bytes(fee),
            funded_at: u64::from_be_bytes(funded_at),
            nonce: u64::from_be_bytes(nonce),
            state,
        })
    }
}

unsafe fn read_pool_order(id: u64) -> Option<PoolOrder> {
    read_value(POOL_ORDER_PREFIX, id).and_then(PoolOrder::decode)
}

unsafe fn write_pool_order(id: u64, order: &PoolOrder) {
    write_value(POOL_ORDER_PREFIX, id, &order.encode());
}

/// The bounded set of unsettled orders whose buyers would all learn a
/// bundle's secrets from one public reveal. It is changed by every buy,
/// settlement and refund, so a reveal can close the complete set atomically.
struct OpenPoolOrders {
    ids: [u64; MAX_OPEN_POOL_ORDERS],
    len: usize,
}

impl OpenPoolOrders {
    fn decode(bytes: Option<&[u8]>) -> Option<Self> {
        let mut out = Self {
            ids: [0; MAX_OPEN_POOL_ORDERS],
            len: 0,
        };
        let bytes = match bytes {
            Some(bytes) => bytes,
            None => return Some(out),
        };
        if bytes.len() % 8 != 0 || bytes.len() > MAX_OPEN_POOL_ORDERS * 8 {
            return None;
        }
        for chunk in bytes.chunks_exact(8) {
            let mut raw = [0u8; 8];
            raw.copy_from_slice(chunk);
            let id = u64::from_be_bytes(raw);
            if id == 0 || out.contains(id) {
                return None;
            }
            out.ids[out.len] = id;
            out.len += 1;
        }
        Some(out)
    }

    fn contains(&self, id: u64) -> bool {
        self.ids[..self.len].contains(&id)
    }

    fn push(&mut self, id: u64) -> bool {
        if id == 0 || self.len == MAX_OPEN_POOL_ORDERS || self.contains(id) {
            return false;
        }
        self.ids[self.len] = id;
        self.len += 1;
        true
    }

    fn remove(&mut self, id: u64) -> bool {
        let Some(index) = self.ids[..self.len].iter().position(|entry| *entry == id) else {
            return false;
        };
        for at in index..self.len - 1 {
            self.ids[at] = self.ids[at + 1];
        }
        self.len -= 1;
        self.ids[self.len] = 0;
        true
    }
}

unsafe fn read_open_pool_orders(bundle: &[u8; COMMITMENT_BYTES]) -> Option<OpenPoolOrders> {
    OpenPoolOrders::decode(read_keyed(key_for_commitment(
        POOL_OPEN_ORDERS_PREFIX,
        bundle,
    )))
}

unsafe fn write_open_pool_orders(bundle: &[u8; COMMITMENT_BYTES], open: &OpenPoolOrders) {
    let mut bytes = [0u8; MAX_OPEN_POOL_ORDERS * 8];
    for (index, id) in open.ids[..open.len].iter().enumerate() {
        bytes[index * 8..(index + 1) * 8].copy_from_slice(&id.to_be_bytes());
    }
    write_keyed(
        key_for_commitment(POOL_OPEN_ORDERS_PREFIX, bundle),
        &bytes[..open.len * 8],
    );
}

/// The retrieval-fee list the buyer attached to an order: one 20-byte host
/// address plus a 16-byte amount per entry. It is read at settlement, when the
/// fee stops being escrow, and at a refund, when the buyer takes it back.
const POOL_FEE_ENTRY_BYTES: usize = 20 + AMOUNT_BYTES;
const POOL_FEE_PREFIX: &[u8] = b"pfee/";

unsafe fn read_pool_fee_list(id: u64) -> Option<&'static [u8]> {
    read_value(POOL_FEE_PREFIX, id)
}

unsafe fn write_pool_fee_list(id: u64, bytes: &[u8]) {
    write_value(POOL_FEE_PREFIX, id, bytes);
}

unsafe fn next_pool_order_id() -> u64 {
    match read_value(POOL_NEXT_ORDER_PREFIX, 0) {
        Some(bytes) if bytes.len() >= 8 => {
            let mut raw = [0u8; 8];
            raw.copy_from_slice(&bytes[..8]);
            u64::from_be_bytes(raw)
        }
        _ => 1,
    }
}

unsafe fn set_next_pool_order_id(id: u64) {
    write_value(POOL_NEXT_ORDER_PREFIX, 0, &id.to_be_bytes());
}

/// Hashes what a pool buyer's delivery key proves possession of: the domain,
/// this contract, the bundle, the buy nonce, the paying caller, and the key.
unsafe fn pool_delivery_key_message(
    bundle: &[u8; COMMITMENT_BYTES],
    nonce: u64,
    caller: &[u8; 20],
    delivery_key: &[u8; DELIVERY_KEY_BYTES],
) -> [u8; 32] {
    let mut buf = [0u8; 224];
    let mut len = 0usize;
    push_bytes(&mut buf, &mut len, DELIVERY_KEY_DOMAIN);
    let (contract, contract_len) = read_arg(own_addr());
    if !contract.is_null() {
        let n = contract_len.min(20);
        let mut address = [0u8; 20];
        core::ptr::copy_nonoverlapping(contract, address.as_mut_ptr(), n);
        push_bytes(&mut buf, &mut len, &address);
    }
    push_bytes(&mut buf, &mut len, bundle);
    push_u64_be(&mut buf, &mut len, nonce);
    push_bytes(&mut buf, &mut len, caller);
    push_bytes(&mut buf, &mut len, delivery_key);

    let mut hash = [0u8; 32];
    let hash_ptr = keccak256(heap_region(&buf[..len]));
    if hash_ptr == 0 {
        return hash;
    }
    let region = hash_ptr as *const Region;
    let src = (*region).offset as *const u8;
    let n = (*region).length as usize;
    if src.is_null() || n != 32 {
        return hash;
    }
    core::ptr::copy_nonoverlapping(src, hash.as_mut_ptr(), 32);
    hash
}

/// True when `signature` recovers to `delivery_key` over the pool context.
unsafe fn pool_delivery_key_is_bound(
    bundle: &[u8; COMMITMENT_BYTES],
    nonce: u64,
    caller: &[u8; 20],
    delivery_key: &[u8; DELIVERY_KEY_BYTES],
    signature: &[u8; SIGNATURE_BYTES],
) -> bool {
    let hash = pool_delivery_key_message(bundle, nonce, caller, delivery_key);
    let recovered = ecrecover(heap_region(&hash), heap_region(signature));
    if recovered == 0 {
        return false;
    }
    let region = recovered as *const Region;
    let src = (*region).offset as *const u8;
    let n = (*region).length as usize;
    if src.is_null() || n != DELIVERY_KEY_BYTES {
        return false;
    }
    for i in 0..DELIVERY_KEY_BYTES {
        if *src.add(i) != delivery_key[i] {
            return false;
        }
    }
    true
}

/// Records one missed order: the first strike takes the beacon down, so the
/// desk has to prove it is alive before new orders; the second clears the
/// listing.
/// Counts a failed order against its bundle. The first strike takes the beacon
/// down; the second clears the inventory. Returns the units the strike removed
/// so the global pool can take them out of its own count.
fn strike(bundle: &mut PoolBundle) -> u64 {
    bundle.strikes = bundle.strikes.saturating_add(1);
    if bundle.strikes >= 2 {
        let removed = bundle.units;
        bundle.units = 0;
        removed
    } else {
        bundle.beacon_block = 0;
        0
    }
}

unsafe fn beacon_is_fresh(bundle: &PoolBundle) -> bool {
    bundle.beacon_block != 0 && block_number() <= bundle.beacon_block.saturating_add(BEACON_BLOCKS)
}

/// Settles an order: the purchase amount leaves escrow for the bundle's
/// registered author, the retrieval fee becomes the named hosts' withdrawable
/// credit, and the order is marked settled. All three settlement paths (the
/// buyer's confirmation, the proven fulfilment and the reveal) run through it,
/// so none of them pays the reserve or leaves escrow behind. `Some(code)` is
/// the refusal to reply with.
unsafe fn settle_order(
    id: u64,
    order: &mut PoolOrder,
    bundle: &PoolBundle,
) -> Option<&'static [u8]> {
    let mut open = match read_open_pool_orders(&order.bundle) {
        Some(open) => open,
        None => return Some(b"invalid_open_orders"),
    };
    if !open.remove(id) {
        return Some(b"order_not_open");
    }
    // A settlement cannot pay an author out of another buyer's escrow, the
    // shared reserve, or a host's withdrawal credit. Check solvency before
    // writing any order state or creating a transfer promise; each successful
    // settlement reduces this protected sum by exactly its author payout.
    let protected = match reserve_total()
        .checked_add(pending_total())
        .and_then(|total| total.checked_add(credit_total()))
    {
        Some(total) => total,
        None => return Some(b"overflow"),
    };
    if contract_balance() < protected {
        return Some(b"not_enough_balance");
    }
    let total = match order.paid.checked_add(order.fee) {
        Some(total) => total,
        None => return Some(b"overflow"),
    };
    let mut state = global_pool();
    state.pending = match state.pending.checked_sub(total) {
        Some(pending) => pending,
        None => return Some(b"pending_too_small"),
    };
    set_global_pool(state);

    // The purchase amount is the author's, not pool liquidity and not a
    // reserve-funded reward (`SPEC.md` 10.3, "no subsidy"). The runtime debits
    // the contract balance when the promise is created, so a contract that
    // cannot cover the payment refuses the call.
    let author_region = heap_region(&bundle.author);
    let amount_region = heap_region(&order.paid.to_be_bytes());
    create_transfer_promise(author_region, amount_region);

    // Escrow stops here for the retrieval fee: each host the buyer named gets a
    // withdrawable credit, and nothing is charged by the byte.
    if order.fee > 0 {
        let fee_bytes: &[u8] = match read_pool_fee_list(id) {
            Some(bytes) => bytes,
            None => &[],
        };
        let mut at = 0usize;
        while at + POOL_FEE_ENTRY_BYTES <= fee_bytes.len() {
            let mut host = [0u8; 20];
            core::ptr::copy_nonoverlapping(fee_bytes.as_ptr().add(at), host.as_mut_ptr(), 20);
            let mut amount_bytes = [0u8; AMOUNT_BYTES];
            core::ptr::copy_nonoverlapping(
                fee_bytes.as_ptr().add(at + 20),
                amount_bytes.as_mut_ptr(),
                AMOUNT_BYTES,
            );
            let amount = u128::from_be_bytes(amount_bytes);
            let credit = match credit_of(&host).checked_add(amount) {
                Some(credit) => credit,
                None => return Some(b"overflow"),
            };
            set_credit(&host, credit);
            let total = match credit_total().checked_add(amount) {
                Some(total) => total,
                None => return Some(b"overflow"),
            };
            set_credit_total(total);
            at += POOL_FEE_ENTRY_BYTES;
        }
        write_pool_fee_list(id, &[]);
    }

    order.state = POOL_ORDER_SETTLED;
    write_pool_order(id, order);
    write_open_pool_orders(&order.bundle, &open);
    None
}

/// Takes one unit out of a bundle for the pool's own quote, binds the caller as
/// buyer with the delivery key they prove, and opens an order.
#[no_mangle]
pub extern "C" fn buy(arg: u32) -> u32 {
    unsafe {
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len < POOL_BUY_ARG_BYTES + 2 {
            return error_reply(b"bad_args");
        }
        let mut bundle_commitment = [0u8; COMMITMENT_BYTES];
        core::ptr::copy_nonoverlapping(bytes, bundle_commitment.as_mut_ptr(), COMMITMENT_BYTES);
        let mut delivery_key = [0u8; DELIVERY_KEY_BYTES];
        core::ptr::copy_nonoverlapping(
            bytes.add(COMMITMENT_BYTES),
            delivery_key.as_mut_ptr(),
            DELIVERY_KEY_BYTES,
        );
        let mut signature = [0u8; SIGNATURE_BYTES];
        core::ptr::copy_nonoverlapping(
            bytes.add(COMMITMENT_BYTES + DELIVERY_KEY_BYTES),
            signature.as_mut_ptr(),
            SIGNATURE_BYTES,
        );
        // The retrieval-fee list follows the delivery-key proof: a two-byte
        // count, then `count` entries of a 20-byte host and a 16-byte amount.
        // It is the buyer's own money, escrowed with the payment until the
        // order settles and returned with it when the order times out.
        let mut count_bytes = [0u8; 2];
        core::ptr::copy_nonoverlapping(bytes.add(POOL_BUY_ARG_BYTES), count_bytes.as_mut_ptr(), 2);
        let fee_count = usize::from(u16::from_be_bytes(count_bytes));
        if fee_count > MAX_FEE_ENTRIES
            || len != POOL_BUY_ARG_BYTES + 2 + fee_count * POOL_FEE_ENTRY_BYTES
        {
            return error_reply(b"bad_fee_list");
        }
        let fee_bytes = if fee_count == 0 {
            &[][..]
        } else {
            core::slice::from_raw_parts(
                bytes.add(POOL_BUY_ARG_BYTES + 2),
                fee_count * POOL_FEE_ENTRY_BYTES,
            )
        };
        let mut fee_total: u128 = 0;
        for index in 0..fee_count {
            let entry = fee_bytes.as_ptr().add(index * POOL_FEE_ENTRY_BYTES);
            let mut amount_bytes = [0u8; AMOUNT_BYTES];
            core::ptr::copy_nonoverlapping(entry.add(20), amount_bytes.as_mut_ptr(), AMOUNT_BYTES);
            fee_total = match fee_total.checked_add(u128::from_be_bytes(amount_bytes)) {
                Some(total) => total,
                None => return error_reply(b"overflow"),
            };
        }

        let mut bundle = match registered_pool_bundle(&bundle_commitment) {
            Ok(bundle) => bundle,
            Err(reply) => return reply,
        };
        if bundle.strikes >= 2 {
            return error_reply(b"listing_cleared");
        }
        if pool_revealed(&bundle_commitment) {
            return error_reply(b"bundle_revealed");
        }
        if !beacon_is_fresh(&bundle) {
            return error_reply(b"beacon_stale");
        }
        let caller = caller_address();
        if !pool_delivery_key_is_bound(
            &bundle_commitment,
            bundle.nonce,
            &caller,
            &delivery_key,
            &signature,
        ) {
            return error_reply(b"delivery_key_not_bound");
        }
        if bundle.units == 0 {
            return error_reply(b"no_units");
        }
        let quote = match pool::buy_quote(reserve_total(), global_pool().units) {
            Some(quote) => quote,
            None => return error_reply(b"not_enough_units"),
        };
        let cap = match u128::from(bundle.covered).checked_mul(FEE_PER_ITEM) {
            Some(cap) => cap,
            None => return error_reply(b"overflow"),
        };
        if fee_total > cap {
            return error_reply(b"fee_above_cap");
        }
        let due = match quote.checked_add(fee_total) {
            Some(due) => due,
            None => return error_reply(b"overflow"),
        };
        if pay_amount_value() != due {
            return error_reply(b"wrong_amount");
        }

        let id = next_pool_order_id();
        let next_id = match id.checked_add(1) {
            Some(next) => next,
            None => return error_reply(b"overflow"),
        };
        let mut open = match read_open_pool_orders(&bundle_commitment) {
            Some(open) => open,
            None => return error_reply(b"invalid_open_orders"),
        };
        if !open.push(id) {
            return error_reply(b"too_many_open_orders");
        }

        let order_nonce = bundle.nonce;
        // One global pool: the quote comes from the whole market, the unit
        // leaves this bundle's inventory, and the payment joins the pool's
        // pending escrow (never its reserve) until the order settles.
        let mut state = global_pool();
        if state.buy(quote).is_err() {
            return error_reply(b"overflow");
        }
        state.pending = match state.pending.checked_add(fee_total) {
            Some(pending) => pending,
            None => return error_reply(b"overflow"),
        };
        set_global_pool(state);
        bundle.units -= 1;
        bundle.nonce = match bundle.nonce.checked_add(1) {
            Some(nonce) => nonce,
            None => return error_reply(b"overflow"),
        };
        write_pool_bundle(&bundle_commitment, &bundle);

        let order = PoolOrder {
            bundle: bundle_commitment,
            buyer: caller,
            delivery_key,
            paid: quote,
            fee: fee_total,
            funded_at: block_number(),
            nonce: order_nonce,
            state: POOL_ORDER_FUNDED,
        };
        write_pool_order(id, &order);
        if fee_count > 0 {
            write_pool_fee_list(id, fee_bytes);
        }
        write_open_pool_orders(&bundle_commitment, &open);
        set_next_pool_order_id(next_id);

        let mut buf = [0u8; 64];
        let mut out_len = 0usize;
        push_bytes(&mut buf, &mut out_len, b"{\"ok\":true,\"order\":");
        push_u64(&mut buf, &mut out_len, id);
        push_bytes(&mut buf, &mut out_len, b"}");
        reply(&buf[..out_len])
    }
}

/// Publishes the buyer-bound ciphertext for a funded order. Anyone may publish
/// it: the buyer checks it against the bundle's key commitment themselves.
#[cfg(not(feature = "fast-candidate"))]
#[no_mangle]
pub extern "C" fn deliver(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len <= 8 {
            return error_reply(b"bad_args");
        }
        let mut id_bytes = [0u8; 8];
        core::ptr::copy_nonoverlapping(bytes, id_bytes.as_mut_ptr(), 8);
        let id = u64::from_be_bytes(id_bytes);
        let ciphertext = core::slice::from_raw_parts(bytes.add(8), len - 8);
        let mut order = match read_pool_order(id) {
            Some(order) => order,
            None => return error_reply(b"order_not_found"),
        };
        // A published payload is a candidate, not an assignment: an outsider who
        // publishes junk first must not occupy the only delivery slot, so a
        // later delivery may replace a pending one until the order settles
        // (`REVIEW.md` A06, `SPEC.md` 7.2). Settlement stays bound to the
        // buyer's own check or to `fulfill`'s proven ciphertext, so a junk
        // payload cannot settle an order.
        if order.state != POOL_ORDER_FUNDED && order.state != POOL_ORDER_DELIVERED {
            return error_reply(b"order_closed");
        }
        if block_number() >= order.funded_at.saturating_add(DEADLINE_BLOCKS) {
            return error_reply(b"delivery_too_late");
        }
        write_value(POOL_DELIVERY_PREFIX, id, ciphertext);
        order.state = POOL_ORDER_DELIVERED;
        write_pool_order(id, &order);
        ok_reply()
    }
}

/// The buyer confirms the ciphertext they checked, bringing it with the call.
///
/// The argument is the order id (8 bytes, big-endian) followed by the exact
/// ciphertext bytes the buyer's own tool decrypted and checked against the
/// bundle commitment. The contract stores those bytes as the delivery record
/// and settles, so a payload someone replaced after the buyer's check cannot
/// strand the payment (`AH-10`): the buyer's own copy settles whatever the
/// pending slot holds. Confirming pays the author, so a junk payload only
/// wastes the buyer's own call. The recorded delivery stays in place after
/// settlement.
#[cfg(not(feature = "fast-candidate"))]
#[no_mangle]
pub extern "C" fn confirm(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len <= 8 {
            return error_reply(b"bad_args");
        }
        let mut id_bytes = [0u8; 8];
        core::ptr::copy_nonoverlapping(bytes, id_bytes.as_mut_ptr(), 8);
        let id = u64::from_be_bytes(id_bytes);
        let ciphertext = core::slice::from_raw_parts(bytes.add(8), len - 8);
        let mut order = match read_pool_order(id) {
            Some(order) => order,
            None => return error_reply(b"order_not_found"),
        };
        // The buyer's own copy settles a funded order even when no `deliver`
        // landed: the desk may hand the ciphertext over directly.
        if order.state != POOL_ORDER_FUNDED && order.state != POOL_ORDER_DELIVERED {
            return error_reply(b"order_closed");
        }
        if caller_address() != order.buyer {
            return error_reply(b"not_buyer");
        }
        write_value(POOL_DELIVERY_PREFIX, id, ciphertext);
        order.state = POOL_ORDER_DELIVERED;
        write_pool_order(id, &order);
        let bundle = match read_pool_bundle(&order.bundle) {
            Some(bundle) => bundle,
            None => return error_reply(b"bundle_not_found"),
        };
        // The payment stops being escrow: the author is paid and the named
        // hosts take the retrieval fee.
        if let Some(code) = settle_order(id, &mut order, &bundle) {
            return error_reply(code);
        }
        ok_reply()
    }
}

/// Settles an order with the seller's delivery proof, without the buyer's
/// cooperation, inside the delivery window. The journal is checked against the
/// order first; the seal last.
#[cfg(not(feature = "fast-candidate"))]
#[no_mangle]
pub extern "C" fn fulfill(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len < FULFILL_HEADER_BYTES + 1 {
            return error_reply(b"bad_args");
        }
        let mut id_bytes = [0u8; 8];
        core::ptr::copy_nonoverlapping(bytes, id_bytes.as_mut_ptr(), 8);
        let id = u64::from_be_bytes(id_bytes);
        let journal = core::slice::from_raw_parts(bytes.add(8), verifier::JOURNAL_BYTES);
        let seal = core::slice::from_raw_parts(
            bytes.add(8 + verifier::JOURNAL_BYTES),
            verifier::SEAL_BYTES,
        );
        let ciphertext = core::slice::from_raw_parts(
            bytes.add(FULFILL_HEADER_BYTES),
            len - FULFILL_HEADER_BYTES,
        );

        let mut order = match read_pool_order(id) {
            Some(order) => order,
            None => return error_reply(b"order_not_found"),
        };
        if order.state != POOL_ORDER_FUNDED && order.state != POOL_ORDER_DELIVERED {
            return error_reply(b"order_closed");
        }
        // The hard repayment boundary applies to every unilateral settlement:
        // once the delivery window has passed, only the buyer's own `confirm`
        // or `refund` can move the order (`AH-13`).
        if block_number() >= order.funded_at.saturating_add(DEADLINE_BLOCKS) {
            return error_reply(b"deadline_passed");
        }
        let bundle = match read_pool_bundle(&order.bundle) {
            Some(bundle) => bundle,
            None => return error_reply(b"bundle_not_found"),
        };
        if journal_u32(journal, 0) != verifier::DELIVERY_JOURNAL_VERSION {
            return error_reply(b"stale_statement");
        }
        // Bind to this node's canonical funding state. Descendant forks with
        // a shared funding anchor still require the chain's canonical view.
        if journal[209..241] != NETWORK_DIGEST {
            return error_reply(b"wrong_network");
        }
        if journal_u64(journal, 241) != order.funded_at {
            return error_reply(b"wrong_funding_height");
        }
        let funding_root = match header_state_root(order.funded_at) {
            Some(root) => root,
            None => return error_reply(b"no_funding_header"),
        };
        if journal[249..281] != funding_root {
            return error_reply(b"wrong_funding_root");
        }
        if journal[4..24] != own_address() {
            return error_reply(b"wrong_contract");
        }
        if journal_u64(journal, 24) != id {
            return error_reply(b"wrong_order");
        }
        if journal_u64(journal, 32) != order.nonce {
            return error_reply(b"wrong_order_nonce");
        }
        if journal[40..60] != bundle.author {
            return error_reply(b"wrong_seller");
        }
        if journal[60..80] != order.buyer {
            return error_reply(b"wrong_buyer");
        }
        if journal[80..145] != order.delivery_key {
            return error_reply(b"wrong_delivery_key");
        }
        if journal[145..177] != order.bundle {
            return error_reply(b"wrong_commitment");
        }
        if verifier::sha256(ciphertext) != journal[177..209] {
            return error_reply(b"ciphertext_mismatch");
        }
        if !verifier::verify_delivery(journal, seal) {
            return error_reply(b"invalid_proof");
        }

        // The proof settles the order, so escrow stops being escrow.
        if let Some(code) = settle_order(id, &mut order, &bundle) {
            return error_reply(code);
        }
        write_value(POOL_DELIVERY_PREFIX, id, ciphertext);
        ok_reply()
    }
}

/// Settles every open order on one bundle by publishing its committed secrets.
///
/// The bundle is keyed by `sha256(secrets)` from the registration statement, so
/// one hash check - no per-order proof - proves that the published bytes are
/// exactly what the registration committed. Those same bytes unlock all orders
/// on this bundle, so paying just one would let another buyer retain the keys
/// and refund. Refuse if any open order is past its deadline; otherwise settle
/// all of them and retain their recovery records atomically. Anyone may submit
/// the transaction; the money goes to the author, never to the caller.
#[cfg(not(feature = "fast-candidate"))]
#[no_mangle]
pub extern "C" fn reveal(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let (bytes, len) = read_arg(arg);
        if bytes.is_null() || len <= 8 || len - 8 > MAX_SECRETS_BYTES {
            return error_reply(b"bad_args");
        }
        let mut id_bytes = [0u8; 8];
        core::ptr::copy_nonoverlapping(bytes, id_bytes.as_mut_ptr(), 8);
        let id = u64::from_be_bytes(id_bytes);
        let secrets = core::slice::from_raw_parts(bytes.add(8), len - 8);
        let order = match read_pool_order(id) {
            Some(order) => order,
            None => return error_reply(b"order_not_found"),
        };
        if order.state != POOL_ORDER_FUNDED && order.state != POOL_ORDER_DELIVERED {
            return error_reply(b"order_closed");
        }
        if verifier::sha256(secrets) != order.bundle {
            return error_reply(b"wrong_secrets");
        }
        let bundle = match read_pool_bundle(&order.bundle) {
            Some(bundle) => bundle,
            None => return error_reply(b"bundle_not_found"),
        };
        // A refunded buyer retained the authenticated encrypted bytes before
        // buying. Publishing this bundle's shared keys later would give that
        // buyer usable content while they keep the refund.
        if bundle.strikes > 0 {
            return error_reply(b"prior_refund");
        }
        let open = match read_open_pool_orders(&order.bundle) {
            Some(open) if open.len > 0 && open.contains(id) => open,
            _ => return error_reply(b"invalid_open_orders"),
        };
        // Validate the complete set before publishing even one key. A missing,
        // foreign or expired order must not be skipped while its buyer can
        // later obtain a refund after seeing the same secrets.
        for open_id in &open.ids[..open.len] {
            let candidate = match read_pool_order(*open_id) {
                Some(candidate) => candidate,
                None => return error_reply(b"order_not_found"),
            };
            if candidate.bundle != order.bundle
                || (candidate.state != POOL_ORDER_FUNDED && candidate.state != POOL_ORDER_DELIVERED)
            {
                return error_reply(b"invalid_open_orders");
            }
            if block_number() >= candidate.funded_at.saturating_add(DEADLINE_BLOCKS) {
                return error_reply(b"deadline_passed");
            }
        }
        for open_id in &open.ids[..open.len] {
            let mut candidate = match read_pool_order(*open_id) {
                Some(candidate) => candidate,
                None => return error_reply(b"order_not_found"),
            };
            if let Some(code) = settle_order(*open_id, &mut candidate, &bundle) {
                return error_reply(code);
            }
            // The transaction body and the original desk are not recovery
            // services. Each settled buyer can find the same preimage by their
            // own order id in canonical state.
            write_value(POOL_REVEAL_PREFIX, *open_id, secrets);
        }
        // The secrets are public from here on. No exposed order remains
        // refundable, and the listing's unsold units leave the shared count.
        reveal_registered_bundle(&order.bundle);
        ok_reply()
    }
}

/// Repays an order that did not settle inside its three-block window.
///
/// Anyone may submit this: the whole escrow - the purchase amount and the
/// retrieval fee - goes straight back to the original buyer, so a third-party
/// caller can never redirect it, and there is no second withdrawal step. The
/// unilateral delivery window is strictly before `funded+DEADLINE`; the first
/// repayment block is `funded+DEADLINE`, so the two boundary rules never
/// overlap. The missed order is recorded against the bundle and the unit a buy
/// consumed stays consumed (`SPEC.md` 5.1).
#[no_mangle]
pub extern "C" fn refund(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let id = parse_u64(arg);
        let mut order = match read_pool_order(id) {
            Some(order) => order,
            None => return error_reply(b"order_not_found"),
        };
        if order.state != POOL_ORDER_FUNDED && order.state != POOL_ORDER_DELIVERED {
            return error_reply(b"order_closed");
        }
        let now = block_number();
        if now < order.funded_at.saturating_add(DEADLINE_BLOCKS) {
            return error_reply(b"refund_not_available");
        }
        let mut open = match read_open_pool_orders(&order.bundle) {
            Some(open) => open,
            None => return error_reply(b"invalid_open_orders"),
        };
        if !open.remove(id) {
            return error_reply(b"order_not_open");
        }
        let mut bundle = match read_pool_bundle(&order.bundle) {
            Some(bundle) => bundle,
            None => return error_reply(b"bundle_not_found"),
        };
        let total = match order.paid.checked_add(order.fee) {
            Some(total) => total,
            None => return error_reply(b"overflow"),
        };
        let mut state = global_pool();
        state.pending = match state.pending.checked_sub(total) {
            Some(pending) => pending,
            None => return error_reply(b"overflow"),
        };
        set_global_pool(state);
        // The runtime debits the contract balance when the promise is created,
        // so a contract that cannot cover the repayment refuses the call.
        let buyer_region = heap_region(&order.buyer);
        let amount_region = heap_region(&total.to_be_bytes());
        create_transfer_promise(buyer_region, amount_region);
        if order.fee > 0 {
            write_pool_fee_list(id, &[]);
        }
        let removed = strike(&mut bundle);
        if removed > 0 {
            set_running_total(
                UNITS_TOTAL_PREFIX,
                units_total().saturating_sub(u128::from(removed)),
            );
        }
        write_pool_bundle(&order.bundle, &bundle);
        order.state = POOL_ORDER_REFUNDED;
        write_pool_order(id, &order);
        write_open_pool_orders(&order.bundle, &open);
        ok_reply()
    }
}

/// One order's public state.
#[no_mangle]
pub extern "C" fn orderPoolView(arg: u32) -> u32 {
    unsafe {
        // The pinned runtime hands a zero-value call a nil `pay_amount`, and the
        // binding's C shim traps on it before the guest runs (`vm/wasm/context.go`
        // sets it from `tx.Amount`, which the transaction decoder leaves nil for
        // a zero amount); see `pay_amount_value`. Value sent anyway stays in the
        // contract's balance as unattributed surplus: it creates no unit, order,
        // credit or reserve entry, and no tracked total includes it.
        let id = parse_u64(arg);
        let order = match read_pool_order(id) {
            Some(order) => order,
            None => return error_reply(b"order_not_found"),
        };
        let mut buf = [0u8; 512];
        let mut len = 0usize;
        push_bytes(&mut buf, &mut len, b"{\"ok\":true,\"bundle\":\"");
        push_hex(&mut buf, &mut len, &order.bundle);
        push_bytes(&mut buf, &mut len, b"\",\"buyer\":\"");
        push_hex(&mut buf, &mut len, &order.buyer);
        push_bytes(&mut buf, &mut len, b"\",\"paid\":\"");
        push_u128(&mut buf, &mut len, order.paid);
        push_bytes(&mut buf, &mut len, b"\",\"fundedAt\":");
        push_u64(&mut buf, &mut len, order.funded_at);
        push_bytes(&mut buf, &mut len, b",\"state\":");
        push_u64(&mut buf, &mut len, u64::from(order.state));
        push_bytes(&mut buf, &mut len, b"}");
        reply(&buf[..len])
    }
}

/// The pool record and the moves over it.
#[cfg(test)]
mod pool_abi_tests {
    use super::*;

    fn record() -> PoolBundle {
        PoolBundle {
            author: [3u8; 20],
            manifest_commitment: [9u8; COMMITMENT_BYTES],
            inventory_commitment: [7u8; COMMITMENT_BYTES],
            units: 5,
            beacon_block: 10_860_138,
            strikes: 2,
            nonce: 11,
            covered: 5,
        }
    }

    #[test]
    fn the_pool_record_round_trips() {
        let bundle = record();
        let encoded = bundle.encode();
        assert_eq!(encoded.len(), POOL_VALUE_BYTES);
        let decoded = PoolBundle::decode(&encoded).expect("decode");
        assert_eq!(decoded.author, bundle.author);
        assert_eq!(decoded.manifest_commitment, bundle.manifest_commitment);
        assert_eq!(decoded.inventory_commitment, bundle.inventory_commitment);
        assert_eq!(decoded.units, bundle.units);
        assert_eq!(decoded.beacon_block, bundle.beacon_block);
        assert_eq!(decoded.strikes, bundle.strikes);
        assert_eq!(decoded.nonce, bundle.nonce);
    }

    #[test]
    fn a_short_pool_record_is_refused() {
        let mut encoded = record().encode();
        encoded[POOL_VALUE_BYTES - 1] = 0;
        assert!(PoolBundle::decode(&encoded[..POOL_VALUE_BYTES - 1]).is_none());
    }

    #[test]
    fn the_pool_moves_and_the_bundle_inventory_are_separate() {
        // The one pool's arithmetic: a gift adds a unit and leaves the reserve
        // alone, a sale pays the pool's own quote out of the reserve, and a
        // deposit raises the quote again.
        let mut pool = pool::Pool::new(50_000, 5);
        pool.donate().expect("gift");
        assert_eq!(pool.units, 6);
        assert_eq!(pool.reserve, 50_000);

        let payout = pool.sell().expect("sale");
        assert_eq!(payout, 50_000 / 7);
        assert_eq!(pool.reserve, 50_000 - payout);
        assert_eq!(pool.units, 7);

        pool.reserve += 1_000;
        assert!(pool::buy_quote(pool.reserve, pool.units).unwrap() > payout);

        // The bundle record keeps only its own inventory, and a strike is what
        // clears it: the first takes the beacon, the second the units, and the
        // returned count is what the global pool subtracts.
        let mut bundle = record();
        bundle.units = 5;
        bundle.strikes = 0;
        bundle.beacon_block = 10_860_138;
        assert_eq!(strike(&mut bundle), 0);
        assert_eq!(bundle.beacon_block, 0);
        assert_eq!(bundle.units, 5);
        assert_eq!(strike(&mut bundle), 5);
        assert_eq!(bundle.units, 0);
    }

    #[test]
    fn the_order_record_round_trips() {
        let order = PoolOrder {
            bundle: [5u8; COMMITMENT_BYTES],
            buyer: [6u8; 20],
            delivery_key: [7u8; DELIVERY_KEY_BYTES],
            paid: 12_345,
            fee: 678,
            funded_at: 10_900_000,
            nonce: 3,
            state: POOL_ORDER_DELIVERED,
        };
        let encoded = order.encode();
        assert_eq!(encoded.len(), POOL_ORDER_VALUE_BYTES);
        let decoded = PoolOrder::decode(&encoded).expect("decode");
        assert_eq!(decoded.bundle, order.bundle);
        assert_eq!(decoded.buyer, order.buyer);
        assert_eq!(decoded.delivery_key, order.delivery_key);
        assert_eq!(decoded.paid, order.paid);
        assert_eq!(decoded.fee, order.fee);
        assert_eq!(decoded.funded_at, order.funded_at);
        assert_eq!(decoded.nonce, order.nonce);
        assert_eq!(decoded.state, order.state);

        assert!(PoolOrder::decode(&encoded[..POOL_ORDER_VALUE_BYTES - 1]).is_none());
        let mut unknown = encoded;
        unknown[POOL_ORDER_STATE_OFFSET] = 9;
        assert!(PoolOrder::decode(&unknown).is_none());
    }

    #[test]
    fn a_strike_takes_the_beacon_then_the_listing() {
        let mut bundle = record();
        bundle.strikes = 0;
        bundle.units = 5;
        strike(&mut bundle);
        assert_eq!(bundle.strikes, 1);
        assert_eq!(
            bundle.beacon_block, 0,
            "the first strike has to take the beacon down"
        );
        assert_eq!(bundle.units, 5, "the first strike keeps the units");

        strike(&mut bundle);
        assert_eq!(bundle.strikes, 2);
        assert_eq!(bundle.units, 0, "the second strike clears the listing");
    }
}
