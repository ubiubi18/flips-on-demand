//! Minimal Idena contract ABI adapter, written in Rust with no_std.
//!
//! Purpose: measure whether a Rust-produced module is accepted and metered by
//! the real Idena contract runtime, and what integer arithmetic costs there.
//! This is a spike, not the contract. It parses raw argument bytes as ASCII
//! digits instead of the SDK's JSON convention, and has no storage, promises,
//! or authorization logic.
//!
//! ABI facts this module implements (verified against the pinned runtime
//! revision, see docs/SOURCES.md):
//! - required exports: `allocate`, `deploy`, `memory`
//! - `allocate(len) -> ptr`: `ptr` must address a Region the host can fill. The
//!   host writes the argument bytes at `region.offset`, requires
//!   `capacity >= len`, and rejects `offset == 0` or `length > capacity`.
//! - method exports receive one i32 pointer per argument, and that pointer
//!   addresses a **Region**, not raw bytes: read `region.offset` for the data.
//! - method exports return an i32 pointer to a Region; the host copies
//!   `length` bytes from `offset`, capped at 64 KiB.
//! - host functions are imported from module `env`.
//!
//! Field naming note: the host source calls the 12-byte struct
//! `{ offset, capacity, length }`, while the AssemblyScript SDK calls the same
//! layout `{ offset, len, capacity }` and always sets both to the same value.
//! This module uses the host's names.

#![no_std]
#![allow(static_mut_refs)]

use core::cell::UnsafeCell;
use core::panic::PanicInfo;

#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    core::arch::wasm32::unreachable()
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct Region {
    offset: u32,
    capacity: u32,
    length: u32,
}

const OUT_SIZE: usize = 1024;
const ARG_BUF_SIZE: usize = 8 * 1024;

/// Argument staging area. The host fills this through `allocate`, so it has to
/// be writable and large enough for the arguments a call carries.
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

/// Holds both the storage value written during a call and the payload returned
/// to the host. The host copies storage bytes before the method returns, so
/// reusing one buffer for both is safe.
static mut VAL_BUF: [u8; VAL_BUF_SIZE] = [0; VAL_BUF_SIZE];
static mut VAL_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};

const VAL_BUF_SIZE: usize = 8 * 1024;

#[link(wasm_import_module = "env")]
extern "C" {
    fn set_storage(key: u32, value: u32);
}

/// Points a Region header at a buffer and records its length.
unsafe fn point_region(region: *mut Region, data: *mut u8, capacity: u32, length: u32) -> u32 {
    (*region).offset = data as u32;
    (*region).capacity = capacity;
    (*region).length = length;
    region as u32
}

#[repr(C)]
struct OutBuf {
    data: [u8; OUT_SIZE],
    region: Region,
}

struct Out(UnsafeCell<OutBuf>);
unsafe impl Sync for Out {}
static OUT: Out = Out(UnsafeCell::new(OutBuf {
    data: [0; OUT_SIZE],
    region: Region {
        offset: 0,
        capacity: 0,
        length: 0,
    },
}));

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

unsafe fn return_bytes(bytes: &[u8]) -> u32 {
    let out = OUT.0.get();
    let data = core::ptr::addr_of_mut!((*out).data) as *mut u8;
    let n = if bytes.len() > OUT_SIZE {
        OUT_SIZE
    } else {
        bytes.len()
    };
    core::ptr::copy_nonoverlapping(bytes.as_ptr(), data, n);
    let region = core::ptr::addr_of_mut!((*out).region);
    (*region).offset = data as u32;
    (*region).capacity = OUT_SIZE as u32;
    (*region).length = n as u32;
    region as u32
}

/// Reads an argument passed by the host. The pointer addresses a Region whose
/// `offset` holds the argument bytes and whose `capacity` equals their length.
unsafe fn read_arg(ptr: u32) -> (*const u8, usize) {
    if ptr == 0 {
        return (core::ptr::null(), 0);
    }
    let region = ptr as *const Region;
    let len = (*region).capacity as usize;
    if (*region).offset == 0 {
        return (core::ptr::null(), 0);
    }
    ((*region).offset as *const u8, len)
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
    let mut n = 0;
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

/// Deployment entry point. Called once through the runtime's deploy path.
#[no_mangle]
pub extern "C" fn deploy(_arg: u32) -> u32 {
    unsafe { return_bytes(b"{\"ok\":true}") }
}

/// Baseline call: returns a fixed payload, so call overhead can be measured.
#[no_mangle]
pub extern "C" fn ping(_arg: u32) -> u32 {
    unsafe { return_bytes(b"{\"pong\":true}") }
}

/// Arithmetic workload: reads an iteration count from the argument bytes and
/// performs that many multiply/xor steps. Used to measure metered cost per
/// step in the real runtime.
#[no_mangle]
pub extern "C" fn compute(arg: u32) -> u32 {
    let iterations = unsafe {
        let (bytes, len) = read_arg(arg);
        parse_u64(bytes, len)
    };
    let mut acc: u64 = 0x9e37_79b9_7f4a_7c15;
    let mut i: u64 = 0;
    while i < iterations {
        acc = acc
            .wrapping_mul(6_364_136_223_846_793_005)
            .wrapping_add(1_442_695_040_888_963_407);
        acc ^= acc >> 33;
        i += 1;
    }

    let mut buf = [0u8; 128];
    let mut len = 0usize;
    push_bytes(&mut buf, &mut len, b"{\"iterations\":");
    push_u64(&mut buf, &mut len, iterations);
    push_bytes(&mut buf, &mut len, b",\"checksum\":\"");
    push_hex_u64(&mut buf, &mut len, acc);
    push_bytes(&mut buf, &mut len, b"\"}");

    unsafe {
        // Publish the result through a host import as well, so the harness can
        // verify what the contract computed without trusting its stdout.
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
        core::ptr::copy_nonoverlapping(buf.as_ptr(), val_buf, len);
        let val_ptr = point_region(
            core::ptr::addr_of_mut!(VAL_REGION),
            val_buf,
            VAL_BUF_SIZE as u32,
            len as u32,
        );
        set_storage(key_ptr, val_ptr);

        val_ptr
    }
}
