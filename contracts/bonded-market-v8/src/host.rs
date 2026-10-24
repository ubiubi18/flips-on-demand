//! Native Idena ABI. Every operation either commits all accounting and transfers or traps.
use alloc::vec::Vec;
use core::panic::PanicInfo;
use crate::{market::*, records::*, proof};
const ADDRESS_BYTES: usize = 20;
const AMOUNT_BYTES: usize = 16;
const HEADER_READ_BYTES: usize = 16 * 1024;
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
const VAL_BUF_SIZE: usize = 4096;
const HEAP_SIZE: usize = 4096;
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
static mut REPLY_BUF: [u8; 64] = [0; 64];
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


struct NativeStore;
impl Store for NativeStore {
    fn get(&self, k: &[u8]) -> Option<Vec<u8>> {
        unsafe {
            let ptr = key(k, &[]);
            let raw = read_region(get_storage(ptr))?;
            if raw.is_empty() { None } else if raw.len() > 4096 { refused() } else { Some(raw.to_vec()) }
        }
    }
    fn put(&mut self, k: &[u8], v: &[u8]) {
        unsafe {
            if v.len() > VAL_BUF_SIZE { refused(); }
            VAL_BUF[..v.len()].copy_from_slice(v);
            let value = point_region(core::ptr::addr_of_mut!(VAL_REGION),
                core::ptr::addr_of_mut!(VAL_BUF) as *mut u8, VAL_BUF_SIZE as u32, v.len() as u32);
            set_storage(key(k, &[]), value);
        }
    }
}
fn refused() -> ! { panic!("fod bonded market refused") }
fn take<T: Codec>(raw: &mut &[u8]) -> Result<T> { T::take(raw).ok_or(Error::Malformed) }
// Nonempty version marker avoids ambiguous zero-sized native ABI allocation.
fn empty(raw: &mut &[u8]) -> Result<()> {
    if take::<u8>(raw)? != 8 { return Err(Error::Malformed); } Ok(())
}

unsafe fn run<F>(arg: u32, payable: bool, deploying: bool, action: F) -> u32
where F: FnOnce(&mut Market<NativeStore>, &mut &[u8], u128) -> Result<Outcome> {
    let bytes = read_region(arg).unwrap_or_else(|| refused()).to_vec();
    let mut raw = bytes.as_slice();
    let who = address(caller()).unwrap_or_else(|| refused());
    let incoming = amount(pay_amount()).unwrap_or_else(|| refused());
    if incoming == 0 || (!payable && incoming != 1) { refused(); }
    let mut store = NativeStore;
    let mut m = Market { store: &mut store, caller: who, height: block_number() };
    let outcome = action(&mut m, &mut raw, incoming).unwrap_or_else(|_|refused());
    if !raw.is_empty() { refused(); }
    let held = amount(balance()).unwrap_or_else(||refused());
    let mut outgoing = outcome.transfers.iter().try_fold(0u128, |sum, (_, a)|sum.checked_add(*a))
        .unwrap_or_else(||refused());
    let refund_sentinel = !payable && !(deploying && held == 0);
    if refund_sentinel { outgoing = outgoing.checked_add(1).unwrap_or_else(||refused()); }
    let liabilities = m.liabilities().unwrap_or_else(|_|refused());
    if held < liabilities.checked_add(outgoing).unwrap_or_else(||refused()) { refused(); }
    for (to, value) in outcome.transfers { transfer(&to, value); }
    if refund_sentinel { transfer(&who, 1); }
    let reply = alloc::format!("{{\"ok\":true,\"id\":{}}}", outcome.id);
    if reply.len() > REPLY_BUF.len() { refused(); }
    REPLY_BUF[..reply.len()].copy_from_slice(reply.as_bytes());
    point_region(core::ptr::addr_of_mut!(REPLY_REGION), core::ptr::addr_of_mut!(REPLY_BUF) as *mut u8,
        REPLY_BUF.len() as u32, reply.len() as u32)
}

#[no_mangle]
pub extern "C" fn deploy(arg: u32) -> u32 { unsafe { run(arg, false, true, |m,r,_| { empty(r)?; m.deploy() }) } }
#[no_mangle]
pub extern "C" fn depositStake(arg: u32) -> u32 {
    unsafe { run(arg, true, false, |m,r,p|m.deposit(p, take(r)?)) }
}
#[no_mangle]
pub extern "C" fn withdrawStake(arg: u32) -> u32 {
    unsafe { run(arg, false, false, |m,r,_| {
        let amount = take(r)?; let exit: u8 = take(r)?;
        if exit > 1 { return Err(Error::Malformed); } m.withdraw(amount, exit == 1)
    }) }
}
#[no_mangle]
pub extern "C" fn addReserve(arg: u32) -> u32 { unsafe { run(arg, true, false, |m,r,p| { empty(r)?; m.fund(p) }) } }
#[no_mangle]
pub extern "C" fn registerBundle(arg: u32) -> u32 {
    unsafe { run(arg, false, false, |m,r,_| {
        let contract = address(own_addr()).ok_or(Error::Malformed)?;
        let (bundle, cids) = proof::registration(r, m.caller, contract, |h|header_state_root(h))?;
        *r = &[]; m.register(bundle, &cids)
    }) }
}
#[no_mangle]
pub extern "C" fn queueSell(arg: u32) -> u32 {
    unsafe { run(arg, false, false, |m,r,_|m.queue_sell(take(r)?, take(r)?, take(r)?, take(r)?)) }
}
#[no_mangle]
pub extern "C" fn queueBuy(arg: u32) -> u32 {
    unsafe { run(arg, true, false, |m,r,p| {
        let key: [u8;65] = take(r)?;
        if !proof::valid_delivery_key(&key) { return Err(Error::Proof); }
        m.queue_buy(key, take(r)?, take(r)?, p)
    }) }
}
#[no_mangle]
pub extern "C" fn cancelSell(arg: u32) -> u32 { unsafe { run(arg, false, false, |m,r,_|m.cancel_sell(take(r)?)) } }
#[no_mangle]
pub extern "C" fn cancelBuy(arg: u32) -> u32 { unsafe { run(arg, false, false, |m,r,_|m.cancel_buy(take(r)?)) } }
#[no_mangle]
pub extern "C" fn acquire(arg: u32) -> u32 { unsafe { run(arg, false, false, |m,r,_| { empty(r)?; m.acquire() }) } }
#[no_mangle]
pub extern "C" fn matchNext(arg: u32) -> u32 {
    unsafe { run(arg, false, false, |m,r,_| { empty(r)?; m.match_next(amount(min_fee_per_gas()).ok_or(Error::Malformed)?) }) }
}
#[no_mangle]
pub extern "C" fn ready(arg: u32) -> u32 {
    unsafe { run(arg, false, false, |m,r,_| {
        let h = m.height.checked_sub(1).ok_or(Error::Proof)?;
        m.ready(take(r)?, h, header_state_root(h).ok_or(Error::Proof)?)
    }) }
}
#[no_mangle]
pub extern "C" fn deliver(arg: u32) -> u32 {
    unsafe { run(arg, false, false, |m,r,_| {
        let id = take(r)?; let ciphertext: [u8; CIPHER_BYTES] = take(r)?;
        let key: [u8;65] = ciphertext[..65].try_into().map_err(|_|Error::Malformed)?;
        if !proof::valid_delivery_key(&key) { return Err(Error::Proof); }
        m.deliver(id, &ciphertext)
    }) }
}
#[no_mangle]
pub extern "C" fn accept(arg: u32) -> u32 { unsafe { run(arg, false, false, |m,r,_|m.accept(take(r)?, take(r)?)) } }
#[no_mangle]
pub extern "C" fn challenge(arg: u32) -> u32 { unsafe { run(arg, false, false, |m,r,_|m.challenge(take(r)?)) } }
#[no_mangle]
pub extern "C" fn proveDelivery(arg: u32) -> u32 {
    unsafe { run(arg, false, false, |m,r,_| {
        let id = take(r)?;
        let journal: [u8;281] = take(r)?; let seal: [u8;256] = take(r)?;
        let t = m.trade(id)?; let b = m.bundle(&t.bundle)?;
        let contract = address(own_addr()).ok_or(Error::Malformed)?;
        if !proof::delivery(&journal, &seal, &t, &b, id, &contract) { return Err(Error::Proof); }
        m.proven_delivery(id)
    }) }
}
#[no_mangle]
pub extern "C" fn expireTrade(arg: u32) -> u32 { unsafe { run(arg, false, false, |m,r,_|m.expire_trade(take(r)?)) } }
