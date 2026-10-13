//! Minimal Idena ABI wrapper to measure the candidate point-proof verifier.
//! It has no marketplace storage, escrow, or authority to settle an order.

#![no_std]
#![allow(static_mut_refs)]

use core::panic::PanicInfo;
use fod_fast_delivery_core::{verify, Delivery, PointBytes, DELIVERY_BYTES};

#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    core::arch::wasm32::unreachable()
}

#[repr(C)]
struct Region {
    offset: u32,
    capacity: u32,
    length: u32,
}

const ARG_BYTES: usize = 1024;
static mut ARG: [u8; ARG_BYTES] = [0; ARG_BYTES];
static mut ARG_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};
static mut OUT: [u8; 5] = [0; 5];
static mut OUT_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};

#[no_mangle]
pub extern "C" fn allocate(size: u32) -> u32 {
    if size as usize > ARG_BYTES {
        return 0;
    }
    unsafe {
        ARG_REGION.offset = core::ptr::addr_of_mut!(ARG) as u32;
        ARG_REGION.capacity = ARG_BYTES as u32;
        ARG_REGION.length = 0;
        core::ptr::addr_of_mut!(ARG_REGION) as u32
    }
}

unsafe fn answer(ok: bool) -> u32 {
    let value: &[u8] = if ok { b"true" } else { b"false" };
    core::ptr::copy_nonoverlapping(
        value.as_ptr(),
        core::ptr::addr_of_mut!(OUT) as *mut u8,
        value.len(),
    );
    OUT_REGION.offset = core::ptr::addr_of_mut!(OUT) as u32;
    OUT_REGION.capacity = 5;
    OUT_REGION.length = value.len() as u32;
    core::ptr::addr_of_mut!(OUT_REGION) as u32
}

#[no_mangle]
pub extern "C" fn deploy(_arg: u32) -> u32 {
    unsafe { answer(true) }
}

/// Argument: commitment (33) || buyer key (33) || context length (u8) ||
/// context || delivery (196). Context construction remains the escrow's job.
#[no_mangle]
pub extern "C" fn verifyDelivery(arg: u32) -> u32 {
    unsafe {
        if arg == 0 {
            return answer(false);
        }
        let region = arg as *const Region;
        let len = (*region).length as usize;
        let start = (*region).offset as *const u8;
        if start.is_null()
            || len < 67 + DELIVERY_BYTES
            || len > ARG_BYTES
            || len > (*region).capacity as usize
        {
            return answer(false);
        }
        let bytes = core::slice::from_raw_parts(start, len);
        let context_len = bytes[66] as usize;
        if context_len == 0 || len != 67 + context_len + DELIVERY_BYTES {
            return answer(false);
        }
        let commitment: PointBytes = bytes[..33].try_into().expect("checked slice");
        let buyer: PointBytes = bytes[33..66].try_into().expect("checked slice");
        let Some(delivery) = Delivery::decode(&bytes[67 + context_len..]) else {
            return answer(false);
        };
        answer(verify(
            &commitment,
            &buyer,
            &bytes[67..67 + context_len],
            &delivery,
        ))
    }
}
