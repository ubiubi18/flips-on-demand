//! BN254 arithmetic inside the real Idena contract runtime.
//!
//! Purpose: measure what arkworks BN254 operations cost in the runtime's gas
//! model, before attempting a full Groth16 verification. The dominant cost of a
//! Groth16 verification is pairing work, so `pairing(n)` is the number that
//! matters for the Package 1 gate.
//!
//! Scope limits, stated plainly:
//! - This is not a verifier. It uses curve generators, not a real proof, and
//!   there is no verification key, no public input check, and no proof encoding.
//! - The host environment is stubbed (see spikes/harness); the runtime,
//!   gatekeeper, and gas metering are real.
//!
//! ABI: see spikes/contract-abi and docs/SOURCES.md F-12.

#![no_std]
#![allow(static_mut_refs)]

extern crate alloc;

use alloc::vec::Vec;
use core::alloc::{GlobalAlloc, Layout};
use core::panic::PanicInfo;
use core::sync::atomic::{AtomicUsize, Ordering};

use ark_bn254::{Bn254, Fr, G1Affine, G1Projective, G2Affine};
use ark_ec::pairing::Pairing;
use ark_ec::{AffineRepr, CurveGroup, PrimeGroup};
use ark_groth16::{prepare_verifying_key, Groth16, Proof, VerifyingKey};
use ark_serialize::{CanonicalDeserialize, CanonicalSerialize};

mod vector;

#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    core::arch::wasm32::unreachable()
}

// ---------------------------------------------------------------------------
// Allocator: arkworks needs `alloc`, and the runtime caps guest memory at
// 100 wasm pages (6.25 MiB) total.
// ---------------------------------------------------------------------------

const HEAP_SIZE: usize = 2 * 1024 * 1024;

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
// Contract ABI: Region-based arguments and returns.
// ---------------------------------------------------------------------------

#[repr(C)]
#[derive(Clone, Copy)]
pub struct Region {
    offset: u32,
    capacity: u32,
    length: u32,
}

const ARG_BUF_SIZE: usize = 8 * 1024;
const VAL_BUF_SIZE: usize = 32 * 1024;

static mut ARG_BUF: [u8; ARG_BUF_SIZE] = [0; ARG_BUF_SIZE];
static mut ARG_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};
static mut KEY_BUF: [u8; 64] = [0; 64];
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

fn push_hex_u64(buf: &mut [u8], len: &mut usize, v: u64) {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut started = false;
    let mut shift = 60i32;
    while shift >= 0 {
        let nibble = ((v >> shift) & 0xf) as usize;
        if nibble != 0 || started || shift == 0 {
            started = true;
            if *len < buf.len() {
                buf[*len] = HEX[nibble];
                *len += 1;
            }
        }
        shift -= 4;
    }
}

/// FNV-1a over serialized bytes: a cheap, allocation-free fingerprint of a
/// result so the harness can tell that real values came out of the computation.
fn fingerprint(bytes: &[u8]) -> u64 {
    let mut h: u64 = 0xcbf2_9ce4_8422_2325;
    for b in bytes {
        h ^= *b as u64;
        h = h.wrapping_mul(0x0000_0100_0000_01b3);
    }
    h
}

unsafe fn publish_and_return(body: &[u8]) -> u32 {
    let key = b"spike.result";
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

/// `field(n)`: n BN254 scalar-field multiplications.
#[no_mangle]
pub extern "C" fn field(arg: u32) -> u32 {
    let n = unsafe {
        let (bytes, len) = read_arg(arg);
        parse_u64(bytes, len)
    };
    let mut x = Fr::from(3u64);
    let mut i: u64 = 0;
    while i < n {
        x = x * x;
        i += 1;
    }
    let mut ser: Vec<u8> = Vec::new();
    let _ = x.serialize_compressed(&mut ser);
    let h = fingerprint(&ser);

    let mut buf = [0u8; 128];
    let mut len = 0usize;
    push_bytes(&mut buf, &mut len, b"{\"field\":");
    push_u64(&mut buf, &mut len, n);
    push_bytes(&mut buf, &mut len, b",\"fp\":\"");
    push_hex_u64(&mut buf, &mut len, h);
    push_bytes(&mut buf, &mut len, b"\"}");
    unsafe { publish_and_return(&buf[..len]) }
}

/// `pairing(n)`: n full BN254 pairings e(G1, G2) on the curve generators.
#[no_mangle]
pub extern "C" fn pairing(arg: u32) -> u32 {
    let n = unsafe {
        let (bytes, len) = read_arg(arg);
        parse_u64(bytes, len)
    };
    let g1: G1Affine = G1Affine::generator();
    let g2: G2Affine = G2Affine::generator();
    let mut acc = Bn254::pairing(g1, g2);
    let mut i: u64 = 1;
    while i < n {
        acc = acc + Bn254::pairing(g1, g2);
        i += 1;
    }
    let mut ser: Vec<u8> = Vec::new();
    let _ = acc.serialize_compressed(&mut ser);
    let h = fingerprint(&ser);

    let mut buf = [0u8; 128];
    let mut len = 0usize;
    push_bytes(&mut buf, &mut len, b"{\"pairings\":");
    push_u64(&mut buf, &mut len, n);
    push_bytes(&mut buf, &mut len, b",\"fp\":\"");
    push_hex_u64(&mut buf, &mut len, h);
    push_bytes(&mut buf, &mut len, b"\"}");
    unsafe { publish_and_return(&buf[..len]) }
}

/// `mul(n)`: n scalar multiplications in G1 on the generator.
#[no_mangle]
pub extern "C" fn mul(arg: u32) -> u32 {
    let n = unsafe {
        let (bytes, len) = read_arg(arg);
        parse_u64(bytes, len)
    };
    let mut acc = G1Projective::generator();
    let s = Fr::from(7u64);
    let mut i: u64 = 0;
    while i < n {
        acc = acc + G1Projective::generator() * s;
        i += 1;
    }
    let affine = acc.into_affine();
    let mut ser: Vec<u8> = Vec::new();
    let _ = affine.serialize_compressed(&mut ser);
    let h = fingerprint(&ser);

    let mut buf = [0u8; 128];
    let mut len = 0usize;
    push_bytes(&mut buf, &mut len, b"{\"muls\":");
    push_u64(&mut buf, &mut len, n);
    push_bytes(&mut buf, &mut len, b",\"fp\":\"");
    push_hex_u64(&mut buf, &mut len, h);
    push_bytes(&mut buf, &mut len, b"\"}");
    unsafe { publish_and_return(&buf[..len]) }
}

/// `verify(mode)`: verifies a genuine BN254 Groth16 proof generated by
/// `spikes/proofgen` for the circuit `c = a * b`.
///
/// - `mode` 0: the untampered proof and public input (expect `true`)
/// - `mode` 1: the public input incremented by one (expect `false`)
///
/// This is a real proof over a real (trivial) circuit. It is not a RISC Zero
/// receipt: there is no ImageID binding and no seal decoding here.
#[no_mangle]
pub extern "C" fn verify(arg: u32) -> u32 {
    let mode = unsafe {
        let (bytes, len) = read_arg(arg);
        parse_u64(bytes, len)
    };

    let mut buf = [0u8; 160];
    let mut len = 0usize;

    let vk = match VerifyingKey::<Bn254>::deserialize_compressed(vector::VK) {
        Ok(v) => v,
        Err(_) => {
            push_bytes(&mut buf, &mut len, b"{\"error\":\"vk\"}");
            return unsafe { publish_and_return(&buf[..len]) };
        }
    };
    let proof = match Proof::<Bn254>::deserialize_compressed(vector::PROOF) {
        Ok(p) => p,
        Err(_) => {
            push_bytes(&mut buf, &mut len, b"{\"error\":\"proof\"}");
            return unsafe { publish_and_return(&buf[..len]) };
        }
    };
    let mut input = match Fr::deserialize_compressed(&vector::PUBLIC_INPUT[..]) {
        Ok(i) => i,
        Err(_) => {
            push_bytes(&mut buf, &mut len, b"{\"error\":\"input\"}");
            return unsafe { publish_and_return(&buf[..len]) };
        }
    };

    if mode == 1 {
        input = input + Fr::from(1u64);
    }

    let pvk = prepare_verifying_key(&vk);
    let verified = Groth16::<Bn254>::verify_proof(&pvk, &proof, &[input]).unwrap_or(false);

    push_bytes(&mut buf, &mut len, b"{\"mode\":");
    push_u64(&mut buf, &mut len, mode);
    push_bytes(&mut buf, &mut len, b",\"verified\":");
    push_bytes(&mut buf, &mut len, if verified { b"true" } else { b"false" });
    push_bytes(&mut buf, &mut len, b"}");
    unsafe { publish_and_return(&buf[..len]) }
}
