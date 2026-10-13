//! Verifies a real RISC Zero Groth16 receipt inside the Idena contract runtime.
//!
//! The receipt material in `receipt_vector.rs` is produced by
//! `spikes/r0-receipt` (real proving run, dev mode off) and describes the
//! trivial guest that commits a u32.
//!
//! Verification recipe mirrors `risc0-zkvm 3.0.6`'s Groth16 receipt path:
//! `Verifier::new(seal, ALLOWED_CONTROL_ROOT, claim_digest,
//! BN254_IDENTITY_CONTROL_ID, &risc0_groth16::verifying_key())`.
//!
//! Scope limits, stated plainly:
//! - The receipt material is embedded rather than passed as call arguments.
//! - The claim digest is taken from the prover instead of being recomputed from
//!   the image id and journal inside the contract, so this spike does not yet
//!   bind the claim to an expected program. Binding is required before any
//!   product use.
//! - The host environment is the stub from `spikes/harness`; the runtime,
//!   gatekeeper, and metering are the real ones.

#![no_std]
#![allow(static_mut_refs)]

extern crate alloc;

use alloc::alloc::{GlobalAlloc, Layout};
use core::panic::PanicInfo;
use core::sync::atomic::{AtomicUsize, Ordering};

use risc0_groth16::{verifying_key, Verifier};
use risc0_zkp::core::digest::Digest;

mod receipt_vector;

#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    core::arch::wasm32::unreachable()
}

// ---------------------------------------------------------------------------
// Allocator for the verifier's internal vectors.
// ---------------------------------------------------------------------------

const HEAP_SIZE: usize = 1024 * 1024;
static mut HEAP: [u8; HEAP_SIZE] = [0; HEAP_SIZE];
static HEAP_NEXT: AtomicUsize = AtomicUsize::new(0);

struct Bump;

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
static ALLOCATOR: Bump = Bump;

// ---------------------------------------------------------------------------
// Contract ABI.
// ---------------------------------------------------------------------------

#[repr(C)]
#[derive(Clone, Copy)]
pub struct Region {
    offset: u32,
    capacity: u32,
    length: u32,
}

const ARG_BUF_SIZE: usize = 4096;
const VAL_BUF_SIZE: usize = 1024;

static mut ARG_BUF: [u8; ARG_BUF_SIZE] = [0; ARG_BUF_SIZE];
static mut ARG_REGION: Region = Region { offset: 0, capacity: 0, length: 0 };
static mut KEY_BUF: [u8; 64] = [0; 64];
static mut KEY_REGION: Region = Region { offset: 0, capacity: 0, length: 0 };
static mut VAL_BUF: [u8; VAL_BUF_SIZE] = [0; VAL_BUF_SIZE];
static mut VAL_REGION: Region = Region { offset: 0, capacity: 0, length: 0 };

#[link(wasm_import_module = "env")]
extern "C" {
    fn set_storage(key: u32, value: u32);
}

unsafe fn point_region(region: *mut Region, data: *mut u8, capacity: u32, length: u32) -> u32 {
    (*region).offset = data as u32;
    (*region).capacity = capacity;
    (*region).length = length;
    region as u32
}

unsafe fn read_arg(ptr: u32) -> (*const u8, usize) {
    if ptr == 0 {
        return (core::ptr::null(), 0);
    }
    let region = ptr as *const Region;
    if (*region).offset == 0 {
        return (core::ptr::null(), 0);
    }
    ((*region).offset as *const u8, (*region).capacity as usize)
}

unsafe fn parse_u64(bytes: *const u8, len: usize) -> u64 {
    if bytes.is_null() {
        return 0;
    }
    let mut i = 0usize;
    let mut v: u64 = 0;
    while i < len && i < 32 {
        let c = *bytes.add(i);
        if c.is_ascii_digit() {
            v = v.saturating_mul(10).saturating_add((c - b'0') as u64);
            i += 1;
        } else {
            break;
        }
    }
    v
}

fn push_bytes(buf: &mut [u8], len: &mut usize, bytes: &[u8]) {
    for b in bytes {
        if *len < buf.len() {
            buf[*len] = *b;
            *len += 1;
        }
    }
}

fn push_u64(buf: &mut [u8], len: &mut usize, mut v: u64) {
    let mut tmp = [0u8; 20];
    let mut n = 0usize;
    if v == 0 {
        tmp[0] = b'0';
        n = 1;
    }
    while v > 0 {
        tmp[n] = b'0' + (v % 10) as u8;
        v /= 10;
        n += 1;
    }
    let mut i = n;
    while i > 0 {
        i -= 1;
        if *len < buf.len() {
            buf[*len] = tmp[i];
            *len += 1;
        }
    }
}

unsafe fn publish_and_return(body: &[u8]) -> u32 {
    let key = b"receipt.result";
    let key_buf = core::ptr::addr_of_mut!(KEY_BUF) as *mut u8;
    core::ptr::copy_nonoverlapping(key.as_ptr(), key_buf, key.len());
    let key_ptr = point_region(
        core::ptr::addr_of_mut!(KEY_REGION),
        key_buf,
        KEY_BUF.len() as u32,
        key.len() as u32,
    );
    let val_buf = core::ptr::addr_of_mut!(VAL_BUF) as *mut u8;
    let n = body.len().min(VAL_BUF_SIZE);
    core::ptr::copy_nonoverlapping(body.as_ptr(), val_buf, n);
    let val_ptr = point_region(
        core::ptr::addr_of_mut!(VAL_REGION),
        val_buf,
        VAL_BUF_SIZE as u32,
        n as u32,
    );
    set_storage(key_ptr, val_ptr);
    val_ptr
}

fn report(mode: u64, verified: bool, detail: &[u8]) -> u32 {
    let mut buf = [0u8; 256];
    let mut len = 0usize;
    push_bytes(&mut buf, &mut len, b"{\"mode\":");
    push_u64(&mut buf, &mut len, mode);
    push_bytes(&mut buf, &mut len, b",\"verified\":");
    push_bytes(&mut buf, &mut len, if verified { b"true" } else { b"false" });
    push_bytes(&mut buf, &mut len, b",\"detail\":\"");
    push_bytes(&mut buf, &mut len, detail);
    push_bytes(&mut buf, &mut len, b"\"}");
    unsafe { publish_and_return(&buf[..len]) }
}

#[no_mangle]
pub extern "C" fn allocate(size: u32) -> u32 {
    if size as usize > ARG_BUF_SIZE {
        return 0;
    }
    let buf = core::ptr::addr_of_mut!(ARG_BUF) as *mut u8;
    let region = core::ptr::addr_of_mut!(ARG_REGION);
    unsafe {
        (*region).offset = buf as u32;
        (*region).capacity = ARG_BUF_SIZE as u32;
        (*region).length = 0;
    }
    region as u32
}

#[no_mangle]
pub extern "C" fn deploy(_arg: u32) -> u32 {
    unsafe { publish_and_return(b"{\"ok\":true}") }
}

/// `verify(mode)`:
/// - 0: the real receipt as produced (expect verified)
/// - 1: the claim digest with one byte flipped (expect rejected)
#[no_mangle]
pub extern "C" fn verify(arg: u32) -> u32 {
    let mode = unsafe {
        let (bytes, len) = read_arg(arg);
        parse_u64(bytes, len)
    };

    let control_root = Digest::from_bytes(receipt_vector::CONTROL_ROOT);
    let bn254_control_id = Digest::from_bytes(receipt_vector::BN254_CONTROL_ID);
    let mut claim_bytes = receipt_vector::CLAIM_DIGEST;
    if mode == 1 {
        claim_bytes[0] ^= 0x01;
    }
    let claim_digest = Digest::from_bytes(claim_bytes);

    let vk = verifying_key();
    let verifier = match Verifier::new(
        receipt_vector::SEAL,
        control_root,
        claim_digest,
        bn254_control_id,
        &vk,
    ) {
        Ok(v) => v,
        Err(_) => return report(mode, false, b"verifier_construction_failed"),
    };

    match verifier.verify() {
        Ok(()) => report(mode, true, b"ok"),
        Err(_) => report(mode, false, b"invalid_proof"),
    }
}
