//! Session-header anchor spike, written in Rust with no_std.
//!
//! Purpose: answer `REVIEW.md` A02's first question inside the real contract
//! runtime - can a contract authenticate a historical session header itself,
//! without an oracle, a checkpoint publisher or a consensus change?
//!
//! The runtime already imports `env.block_header(height)` and the node answers
//! it from its canonical chain (`idena-go` `vm/wasm/wasm_env.go` `BlockHeader`
//! reads `GetBlockHeaderByHeight`, charging `costs.ReadBlockGas`). The bytes are
//! the header's protobuf encoding (`blockchain/types` `Header.ToBytes` is
//! `proto.Marshal(ToProto())`), so the contract can read the very fields a
//! session proof has to be bound to: height, state root, identity root, block
//! seed and the ceremony flags.
//!
//! This spike takes `height(8) || block_hash(32) || state_root(32) ||
//! identity_root(32) || seed(32)` and refuses unless the chain's own header at
//! that height carries exactly those values, is a proposed (non-skip) block,
//! and carries the `FlipLotteryStarted` flag. Refusals abort the call, as the
//! escrow does after D0 (`REVIEW.md` A04).
//!
//! Field numbers come from `idena-go` `protobuf/models.pb.go`:
//! `ProtoBlockHeader` carries `proposedHeader = 1`, `emptyHeader = 2`;
//! the proposed message carries `height = 2`, `root = 6`, `identityRoot = 7`,
//! `flags = 8`, `blockSeed = 12`; the empty message carries `height = 2`,
//! `root = 3`, `identityRoot = 4`, `timestamp = 5`, `blockSeed = 6`, `flags = 7`.

#![no_std]
#![allow(static_mut_refs)]

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

const ARG_BUF_SIZE: usize = 1024;
const OUT_BUF_SIZE: usize = 512;
const HEADER_BUF_SIZE: usize = 16 * 1024;
const ARG_BYTES: usize = 8 + 32 + 32 + 32 + 32;
const HASH_BYTES: usize = 32;
/// `FlipLotteryStarted = 1 << 1` in `blockchain/types/types.go`.
const FLAG_FLIP_LOTTERY_STARTED: u32 = 1 << 1;

static mut ARG_BUF: [u8; ARG_BUF_SIZE] = [0; ARG_BUF_SIZE];
static mut ARG_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};
static mut HEADER_BUF: [u8; HEADER_BUF_SIZE] = [0; HEADER_BUF_SIZE];
static mut HASH_REGION: Region = Region {
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
static mut VAL_BUF: [u8; OUT_BUF_SIZE] = [0; OUT_BUF_SIZE];
static mut VAL_REGION: Region = Region {
    offset: 0,
    capacity: 0,
    length: 0,
};

#[link(wasm_import_module = "env")]
extern "C" {
    fn set_storage(key: u32, value: u32);
    fn block_header(height: u64) -> u32;
    fn keccak256(data: u32) -> u32;
}

/// Refuses the call by aborting it, so the node keeps no part of it.
fn refuse() -> u32 {
    panic!("fod:refused")
}

unsafe fn point_region(region: *mut Region, data: *mut u8, capacity: u32, length: u32) -> u32 {
    (*region).offset = data as u32;
    (*region).capacity = capacity;
    (*region).length = length;
    region as u32
}

#[no_mangle]
pub extern "C" fn allocate(size: u32) -> u32 {
    if size as usize > ARG_BUF_SIZE {
        return 0;
    }
    unsafe {
        let buf = core::ptr::addr_of_mut!(ARG_BUF) as *mut u8;
        let region = core::ptr::addr_of_mut!(ARG_REGION);
        (*region).offset = buf as u32;
        (*region).capacity = ARG_BUF_SIZE as u32;
        (*region).length = 0;
        region as u32
    }
}

/// Reads an argument the host wrote into a Region.
unsafe fn read_arg(ptr: u32) -> &'static [u8] {
    if ptr == 0 {
        return &[];
    }
    let region = ptr as *const Region;
    if (*region).offset == 0 {
        return &[];
    }
    let src = (*region).offset as *const u8;
    let len = (*region).length as usize;
    core::slice::from_raw_parts(src, len)
}

/// Reads a host-returned byte vector into the header buffer. An empty answer
/// (the node has no header at that height) and anything larger than the buffer
/// are refusals, not truncations.
unsafe fn read_header(ptr: u32) -> Option<&'static [u8]> {
    if ptr == 0 {
        return None;
    }
    let region = ptr as *const Region;
    if (*region).offset == 0 {
        return None;
    }
    let src = (*region).offset as *const u8;
    let len = (*region).length as usize;
    if len == 0 || len > HEADER_BUF_SIZE {
        return None;
    }
    let dst = core::ptr::addr_of_mut!(HEADER_BUF) as *mut u8;
    core::ptr::copy_nonoverlapping(src, dst, len);
    Some(core::slice::from_raw_parts(dst, len))
}

/// Keccak-256 of a slice, through the runtime's own hash callback.
unsafe fn keccak(data: &[u8]) -> Option<[u8; HASH_BYTES]> {
    if data.is_empty() {
        return None;
    }
    let region = core::ptr::addr_of_mut!(HASH_REGION);
    point_region(
        region,
        data.as_ptr() as *mut u8,
        data.len() as u32,
        data.len() as u32,
    );
    let ptr = keccak256(region as u32);
    if ptr == 0 {
        return None;
    }
    let out = ptr as *const Region;
    if (*out).offset == 0 || (*out).length as usize != HASH_BYTES {
        return None;
    }
    let mut hash = [0u8; HASH_BYTES];
    core::ptr::copy_nonoverlapping((*out).offset as *const u8, hash.as_mut_ptr(), HASH_BYTES);
    Some(hash)
}

// ---------------------------------------------------------------------------
// Minimal protobuf reader.
//
// The header bytes are protobuf, so the contract walks the wire format itself
// rather than trusting a re-encoding: unknown fields are skipped, the last
// occurrence of a wanted field wins, and every length is bounds-checked.
// ---------------------------------------------------------------------------

enum Value<'a> {
    Varint(u64),
    Bytes(&'a [u8]),
}

struct Reader<'a> {
    bytes: &'a [u8],
    at: usize,
}

impl<'a> Reader<'a> {
    fn varint(&mut self) -> Option<u64> {
        let mut value: u64 = 0;
        let mut shift = 0u32;
        loop {
            let byte = *self.bytes.get(self.at)?;
            self.at += 1;
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

    fn bytes_field(&mut self) -> Option<&'a [u8]> {
        let len = self.varint()? as usize;
        let end = self.at.checked_add(len)?;
        let out = self.bytes.get(self.at..end)?;
        self.at = end;
        Some(out)
    }

    fn skip(&mut self, wire: u64) -> Option<()> {
        match wire {
            0 => {
                self.varint()?;
            }
            1 => self.at = self.at.checked_add(8)?,
            2 => {
                self.bytes_field()?;
            }
            5 => self.at = self.at.checked_add(4)?,
            _ => return None,
        }
        if self.at > self.bytes.len() {
            return None;
        }
        Some(())
    }
}

/// The last occurrence of `wanted` in one protobuf message, or `None`.
fn read_field<'a>(message: &'a [u8], wanted: u32) -> Option<Value<'a>> {
    let mut reader = Reader {
        bytes: message,
        at: 0,
    };
    let mut found = None;
    while reader.at < message.len() {
        let key = reader.varint()?;
        let field = (key >> 3) as u32;
        let wire = key & 7;
        if field == 0 {
            return None;
        }
        if field == wanted {
            match wire {
                0 => found = Some(Value::Varint(reader.varint()?)),
                2 => found = Some(Value::Bytes(reader.bytes_field()?)),
                _ => return None,
            }
        } else {
            reader.skip(wire)?;
        }
    }
    found
}

fn field_u64(message: &[u8], field: u32) -> Option<u64> {
    match read_field(message, field) {
        Some(Value::Varint(value)) => Some(value),
        _ => None,
    }
}

fn field_hash(message: &[u8], field: u32) -> Option<[u8; HASH_BYTES]> {
    match read_field(message, field) {
        Some(Value::Bytes(bytes)) if bytes.len() == HASH_BYTES => {
            let mut out = [0u8; HASH_BYTES];
            out.copy_from_slice(bytes);
            Some(out)
        }
        _ => None,
    }
}

struct Anchor {
    proposed: bool,
    height: u64,
    state_root: [u8; HASH_BYTES],
    identity_root: [u8; HASH_BYTES],
    seed: [u8; HASH_BYTES],
    flags: u32,
    /// The protobuf sub-message the node hashes into the block hash.
    hashed: &'static [u8],
}

/// Reads the fields a session anchor needs out of a `ProtoBlockHeader`.
fn parse_header(bytes: &'static [u8]) -> Option<Anchor> {
    match (read_field(bytes, 1), read_field(bytes, 2)) {
        (Some(Value::Bytes(proposed)), None) => Some(Anchor {
            proposed: true,
            height: field_u64(proposed, 2)?,
            state_root: field_hash(proposed, 6)?,
            identity_root: field_hash(proposed, 7)?,
            seed: field_hash(proposed, 12)?,
            flags: field_u64(proposed, 8).unwrap_or(0) as u32,
            hashed: proposed,
        }),
        (None, Some(Value::Bytes(empty))) => Some(Anchor {
            proposed: false,
            height: field_u64(empty, 2)?,
            state_root: field_hash(empty, 3)?,
            identity_root: field_hash(empty, 4)?,
            seed: field_hash(empty, 6)?,
            flags: field_u64(empty, 7).unwrap_or(0) as u32,
            hashed: empty,
        }),
        _ => None,
    }
}

/// Deployment entry point: nothing to record.
#[no_mangle]
pub extern "C" fn deploy(_arg: u32) -> u32 {
    unsafe { reply(b"{\"ok\":true}") }
}

/// `anchor` takes `height(8 BE) || block_hash(32) || state_root(32) ||
/// identity_root(32) || seed(32)` and refuses unless the chain's own header at
/// that height matches all of them, is proposed, and marks the lottery start.
#[no_mangle]
pub extern "C" fn anchor(arg: u32) -> u32 {
    unsafe {
        let bytes = read_arg(arg);
        if bytes.len() != ARG_BYTES {
            return refuse();
        }
        let mut height_bytes = [0u8; 8];
        height_bytes.copy_from_slice(&bytes[..8]);
        let height = u64::from_be_bytes(height_bytes);
        let mut claimed_hash = [0u8; HASH_BYTES];
        claimed_hash.copy_from_slice(&bytes[8..40]);
        let mut state_root = [0u8; HASH_BYTES];
        state_root.copy_from_slice(&bytes[40..72]);
        let mut identity_root = [0u8; HASH_BYTES];
        identity_root.copy_from_slice(&bytes[72..104]);
        let mut seed = [0u8; HASH_BYTES];
        seed.copy_from_slice(&bytes[104..136]);

        let header_ptr = block_header(height);
        let Some(header) = read_header(header_ptr) else {
            return refuse();
        };
        let Some(anchor) = parse_header(header) else {
            return refuse();
        };
        if !anchor.proposed
            || anchor.height != height
            || anchor.state_root != state_root
            || anchor.identity_root != identity_root
            || anchor.seed != seed
            || anchor.flags & FLAG_FLIP_LOTTERY_STARTED == 0
        {
            return refuse();
        }
        match keccak(anchor.hashed) {
            Some(hash) if hash == claimed_hash => {}
            _ => return refuse(),
        }

        let mut buf = [0u8; OUT_BUF_SIZE];
        let mut len = 0usize;
        push_bytes(&mut buf, &mut len, b"{\"ok\":true,\"shape\":\"proposed\",\"height\":");
        push_u64(&mut buf, &mut len, anchor.height);
        push_bytes(&mut buf, &mut len, b",\"flags\":");
        push_u64(&mut buf, &mut len, u64::from(anchor.flags));
        push_bytes(&mut buf, &mut len, b"}");

        // Mirror the verdict into storage: the host does not surface a call's
        // returned payload, so this is how the harness reads it (as in the ABI
        // spike).
        let key = b"anchor.result";
        let key_buf = core::ptr::addr_of_mut!(ARG_BUF) as *mut u8;
        core::ptr::copy_nonoverlapping(key.as_ptr(), key_buf, key.len());
        let key_ptr = point_region(
            core::ptr::addr_of_mut!(ARG_REGION),
            key_buf,
            ARG_BUF_SIZE as u32,
            key.len() as u32,
        );
        let val_buf = core::ptr::addr_of_mut!(VAL_BUF) as *mut u8;
        core::ptr::copy_nonoverlapping(buf.as_ptr(), val_buf, len);
        let val_ptr = point_region(
            core::ptr::addr_of_mut!(VAL_REGION),
            val_buf,
            OUT_BUF_SIZE as u32,
            len as u32,
        );
        set_storage(key_ptr, val_ptr);

        reply(&buf[..len])
    }
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

unsafe fn reply(body: &[u8]) -> u32 {
    let buf = core::ptr::addr_of_mut!(OUT_BUF) as *mut u8;
    let n = body.len().min(OUT_BUF_SIZE);
    core::ptr::copy_nonoverlapping(body.as_ptr(), buf, n);
    point_region(
        core::ptr::addr_of_mut!(OUT_REGION),
        buf,
        OUT_BUF_SIZE as u32,
        n as u32,
    )
}
