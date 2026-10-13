//! Isolated on-chain cost check for the public part of fast registration.
//! No proof seal, registration storage, or escrow is present in this spike.

#![no_std]
#![allow(static_mut_refs)]

use core::panic::PanicInfo;
use fod_fast_session_core::{verify_with_roots, MAX_WIRE_BYTES};

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

const ARG_BYTES: usize = 20 + 2 + 2 + 32 + MAX_WIRE_BYTES;
const HEADER_BYTES: usize = 16 * 1024;
const REGION_ARENA_BYTES: usize = 128 * 1024;
static mut REGION_ARENA: [u8; REGION_ARENA_BYTES] = [0; REGION_ARENA_BYTES];
static mut REGION_NEXT: usize = 0;
static mut OUT: [u8; 5] = [0; 5];
static mut OUT_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};

mod bump {
    use core::alloc::{GlobalAlloc, Layout};
    use core::sync::atomic::{AtomicUsize, Ordering};

    const HEAP_BYTES: usize = 1024 * 1024;
    static mut HEAP: [u8; HEAP_BYTES] = [0; HEAP_BYTES];
    static NEXT: AtomicUsize = AtomicUsize::new(0);

    pub struct Bump;

    unsafe impl GlobalAlloc for Bump {
        unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
            let align = layout.align().max(8);
            let start = NEXT.load(Ordering::SeqCst);
            let aligned = (start + align - 1) & !(align - 1);
            let end = aligned.saturating_add(layout.size());
            if end > HEAP_BYTES {
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

#[link(wasm_import_module = "env")]
extern "C" {
    fn block_header(height: u64) -> u32;
}

#[no_mangle]
pub extern "C" fn allocate(size: u32) -> u32 {
    unsafe {
        if size as usize > ARG_BYTES {
            return 0;
        }
        let align = 4usize;
        let start = (REGION_NEXT + align - 1) & !(align - 1);
        let data = start + core::mem::size_of::<Region>();
        let end = data + size as usize;
        if end > REGION_ARENA_BYTES {
            return 0;
        }
        REGION_NEXT = end;
        let base = core::ptr::addr_of_mut!(REGION_ARENA) as *mut u8;
        let region = base.add(start) as *mut Region;
        (*region).offset = base.add(data) as u32;
        (*region).capacity = size;
        (*region).length = 0;
        region as u32
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

fn protobuf_varint(message: &[u8], at: &mut usize) -> Option<u64> {
    let mut value = 0u64;
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

fn protobuf_field(message: &[u8], wanted: u32) -> Option<&[u8]> {
    let mut at = 0usize;
    let mut found = None;
    while at < message.len() {
        let key = protobuf_varint(message, &mut at)?;
        let field = (key >> 3) as u32;
        if field == 0 {
            return None;
        }
        match key & 7 {
            0 => {
                protobuf_varint(message, &mut at)?;
            }
            1 => {
                at = at.checked_add(8)?;
            }
            2 => {
                let len = protobuf_varint(message, &mut at)? as usize;
                let end = at.checked_add(len)?;
                let data = message.get(at..end)?;
                if field == wanted {
                    found = Some(data);
                }
                at = end;
            }
            5 => {
                at = at.checked_add(4)?;
            }
            _ => return None,
        }
        if at > message.len() {
            return None;
        }
    }
    found
}

unsafe fn header_state_root(height: u64) -> Option<[u8; 32]> {
    let ptr = block_header(height);
    if ptr == 0 {
        return None;
    }
    let region = ptr as *const Region;
    if (*region).offset == 0 || (*region).length as usize > HEADER_BYTES {
        return None;
    }
    let header =
        core::slice::from_raw_parts((*region).offset as *const u8, (*region).length as usize);
    let (root_field, sub) = match protobuf_field(header, 1) {
        Some(sub) => (6, sub),
        None => (3, protobuf_field(header, 2)?),
    };
    let root = protobuf_field(sub, root_field)?;
    root.try_into().ok()
}

/// Argument: expected author(20), epoch(2), count(2), content inventory
/// commitment(32), then the bounded public-session witness.
#[no_mangle]
pub extern "C" fn verifySession(arg: u32) -> u32 {
    unsafe {
        if arg == 0 {
            return answer(false);
        }
        let region = arg as *const Region;
        let len = (*region).length as usize;
        if (*region).offset == 0 || len > ARG_BYTES || len > (*region).capacity as usize || len < 56
        {
            return answer(false);
        }
        let bytes = core::slice::from_raw_parts((*region).offset as *const u8, len);
        let author: [u8; 20] = bytes[..20].try_into().expect("checked length");
        let epoch = u16::from_be_bytes(bytes[20..22].try_into().expect("checked length"));
        let count = u16::from_be_bytes(bytes[22..24].try_into().expect("checked length"));
        let original: [u8; 32] = bytes[24..56].try_into().expect("checked length");
        answer(
            verify_with_roots(&bytes[56..], &author, epoch, count, &original, |height| {
                header_state_root(height)
            })
            .is_ok(),
        )
    }
}
