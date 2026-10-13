//! Probe only: can the RISC Zero Groth16 verifier crate build for the contract
//! target (`wasm32-unknown-unknown`, no_std) at all?
//!
//! This module is not deployed and does not verify anything. It exists to fail
//! fast if the verifier cannot be compiled into the contract toolchain, before
//! any prover toolchain is installed.

#![no_std]
#![allow(static_mut_refs)]

extern crate alloc;

use core::panic::PanicInfo;

use alloc::alloc::{GlobalAlloc, Layout};
use core::sync::atomic::{AtomicUsize, Ordering};
use risc0_groth16::{Verifier, VerifyingKey, verifying_key};

static mut HEAP: [u8; 1024 * 1024] = [0; 1024 * 1024];
static HEAP_NEXT: AtomicUsize = AtomicUsize::new(0);

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

unsafe fn publish_and_return(body: &[u8]) -> u32 {
    let key = b"probe.result";
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

struct Bump;

unsafe impl GlobalAlloc for Bump {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        let align = layout.align().max(8);
        let start = HEAP_NEXT.load(Ordering::SeqCst);
        let aligned = (start + align - 1) & !(align - 1);
        let end = aligned.saturating_add(layout.size());
        if end > HEAP.len() {
            return core::ptr::null_mut();
        }
        HEAP_NEXT.store(end, Ordering::SeqCst);
        (core::ptr::addr_of_mut!(HEAP) as *mut u8).add(aligned)
    }

    unsafe fn dealloc(&self, _ptr: *mut u8, _layout: Layout) {}
}

#[global_allocator]
static ALLOCATOR: Bump = Bump;

#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    core::arch::wasm32::unreachable()
}

#[no_mangle]
pub extern "C" fn deploy(_arg: u32) -> u32 {
    unsafe { publish_and_return(b"{\"ok\":true}") }
}

#[no_mangle]
pub extern "C" fn probe(_arg: u32) -> u32 {
    // Forces the verifier path to be compiled and linked. An empty seal must be
    // rejected, so a successful build plus a non-zero return here shows the
    // verifier code is present and running in the module.
    let vk: VerifyingKey = verifying_key();
    let digest = Default::default();
    let code = match Verifier::new(&[], digest, digest, digest, &vk) {
        Ok(v) => match v.verify() {
            Ok(()) => 1,
            Err(_) => 2,
        },
        Err(_) => 3,
    };
    // 3 = the empty seal was rejected while constructing the verifier, which
    // means the RISC Zero verifier code path ran inside the contract runtime.
    let body: &[u8] = if code == 3 {
        b"{\"seal_rejected\":true,\"code\":3}"
    } else if code == 2 {
        b"{\"verify_failed\":true,\"code\":2}"
    } else {
        b"{\"verified\":true,\"code\":1}"
    };
    unsafe { publish_and_return(body) }
}
