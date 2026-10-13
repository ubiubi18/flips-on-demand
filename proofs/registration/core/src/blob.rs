//! The flip blob as the node writes it (`idena-go/core/flip/flipper.go`, the
//! `ProtoIpfsFlip` message): the author's identity public key, the public part
//! encrypted to the epoch public key, and the private part encrypted to the
//! epoch private key.
//!
//! Only the three length-delimited fields are read, and any other field the node
//! may add is skipped rather than rejected, so a blob that carries more than this
//! proof needs still opens.

use alloc::string::String;
use alloc::vec::Vec;

#[cfg(feature = "risc0-sha")]
use risc0_zkvm::sha::rust_crypto::{Digest, Sha256};
#[cfg(not(feature = "risc0-sha"))]
use sha2::{Digest, Sha256};

use crate::{cid_v1_raw_sha256, decrypt_from, CID_V1_RAW_SHA256_BYTES};

const FIELD_IDENTITY_KEY: u64 = 1;
const FIELD_PUBLIC_PART: u64 = 2;
const FIELD_PRIVATE_PART: u64 = 3;

/// go-unixfs's default chunk size, which is also where the node's importer
/// switches from one raw block to a dag-pb root (`SOURCES.md` F-86, F-88).
pub const UNIXFS_CHUNK_BYTES: usize = 262_144;
/// go-unixfs's default fan-out: a balanced root holds this many links before a
/// second level appears.
pub const UNIXFS_MAX_LINKS: usize = 174;
/// `common.MaxFlipSize` in the pinned node: a larger flip is refused before it is
/// stored, so a flip's DAG is one root with at most three leaves.
pub const MAX_FLIP_BYTES: usize = 1_024 * 600;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FlipBlob {
    pub identity_key: Vec<u8>,
    pub public_part: Vec<u8>,
    pub private_part: Vec<u8>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum BlobError {
    /// The bytes are not a protobuf message this reader understands.
    Malformed,
    /// One of the three fields is absent.
    MissingField(u64),
    /// A required singular field occurs more than once, making the blob
    /// ambiguous across protobuf readers.
    DuplicateField(u64),
    /// A part did not decrypt with the secret it was given, or the tag failed.
    Decryption,
    /// The bytes are larger than a flip can be, so no importer's CID describes
    /// them.
    TooLarge,
    /// The blob is empty, which has no CID in this construction.
    Empty,
}

fn read_varint(bytes: &[u8], at: &mut usize) -> Option<u64> {
    let mut value = 0u64;
    for shift in (0..=63).step_by(7) {
        let byte = *bytes.get(*at)?;
        *at += 1;
        if shift == 63 && byte > 1 {
            return None;
        }
        value |= ((byte & 0x7f) as u64) << shift;
        if byte & 0x80 == 0 {
            return Some(value);
        }
    }
    None
}

/// Splits the message into its length-delimited fields.
fn length_delimited_fields(bytes: &[u8]) -> Result<Vec<(u64, Vec<u8>)>, BlobError> {
    let mut fields = Vec::new();
    let mut at = 0usize;
    while at < bytes.len() {
        let tag = read_varint(bytes, &mut at).ok_or(BlobError::Malformed)?;
        let field = tag >> 3;
        let wire = tag & 0x7;
        if field == 0 || field > (1 << 29) - 1 || (1..=3).contains(&field) && wire != 2 {
            return Err(BlobError::Malformed);
        }
        match wire {
            0 => {
                read_varint(bytes, &mut at).ok_or(BlobError::Malformed)?;
            }
            2 => {
                let length =
                    usize::try_from(read_varint(bytes, &mut at).ok_or(BlobError::Malformed)?)
                        .map_err(|_| BlobError::Malformed)?;
                let end = at.checked_add(length).ok_or(BlobError::Malformed)?;
                let value = bytes.get(at..end).ok_or(BlobError::Malformed)?;
                fields.push((field, value.to_vec()));
                at = end;
            }
            // Fixed-width fields the node's message does not use.
            1 => at = at.checked_add(8).ok_or(BlobError::Malformed)?,
            5 => at = at.checked_add(4).ok_or(BlobError::Malformed)?,
            _ => return Err(BlobError::Malformed),
        }
        if at > bytes.len() {
            return Err(BlobError::Malformed);
        }
    }
    Ok(fields)
}

/// Reads the three fields the proof needs, rejecting duplicate singular fields
/// so the proof and the node's protobuf reader cannot select different values.
pub fn parse_flip_blob(bytes: &[u8]) -> Result<FlipBlob, BlobError> {
    let fields = length_delimited_fields(bytes)?;
    let field = |wanted: u64| -> Result<Vec<u8>, BlobError> {
        let mut matches = fields.iter().filter(|(number, _)| *number == wanted);
        let (_, value) = matches.next().ok_or(BlobError::MissingField(wanted))?;
        if matches.next().is_some() {
            return Err(BlobError::DuplicateField(wanted));
        }
        Ok(value.clone())
    };
    Ok(FlipBlob {
        identity_key: field(FIELD_IDENTITY_KEY)?,
        public_part: field(FIELD_PUBLIC_PART)?,
        private_part: field(FIELD_PRIVATE_PART)?,
    })
}

/// Opens a blob with the two epoch secrets: the public part under the epoch
/// public key's scalar, the private part under the epoch private key's scalar.
pub fn open_flip_blob(
    bytes: &[u8],
    public_part_secret: &[u8],
    private_part_secret: &[u8],
) -> Result<(Vec<u8>, Vec<u8>), BlobError> {
    let blob = parse_flip_blob(bytes)?;
    let public =
        decrypt_from(public_part_secret, &blob.public_part).ok_or(BlobError::Decryption)?;
    let private =
        decrypt_from(private_part_secret, &blob.private_part).ok_or(BlobError::Decryption)?;
    Ok((public, private))
}

const MULTIBASE_ALPHABET: &[u8; 32] = b"abcdefghijklmnopqrstuvwxyz234567";

/// The multibase base32 form of a CIDv1 value, which is what a manifest carries:
/// the `b` prefix and 58 lower-case base32 characters, no padding.
pub fn cid_multibase(cid: &[u8; CID_V1_RAW_SHA256_BYTES]) -> String {
    let mut out = String::with_capacity(1 + 58);
    out.push('b');
    let mut buffer = 0u32;
    let mut bits = 0u32;
    for byte in cid {
        buffer = (buffer << 8) | *byte as u32;
        bits += 8;
        while bits >= 5 {
            bits -= 5;
            out.push(MULTIBASE_ALPHABET[((buffer >> bits) & 0x1f) as usize] as char);
        }
    }
    if bits > 0 {
        out.push(MULTIBASE_ALPHABET[((buffer << (5 - bits)) & 0x1f) as usize] as char);
    }
    out
}

/// The CID of a blob, in the form a manifest carries.
///
/// This is the importer's own rule rather than one form of it: a blob that fits
/// in one chunk is a raw leaf, and a larger blob is a dag-pb root over raw
/// leaves. Reproducing only the raw form is what made a bundle containing a flip
/// over one chunk unprovable (`SOURCES.md` F-86, F-88); the vectors in
/// `tests/unixfs_v1.rs` are the node's own answers for five sizes, so this
/// implementation is checked against the importer rather than against itself.
pub fn blob_cid_multibase(blob: &[u8]) -> Result<String, BlobError> {
    Ok(cid_multibase(&blob_cid(blob)?))
}

/// The CID bytes of a blob: raw while it fits in one chunk, otherwise the
/// dag-pb root the balanced importer would build.
fn blob_cid(blob: &[u8]) -> Result<[u8; CID_V1_RAW_SHA256_BYTES], BlobError> {
    if blob.is_empty() {
        return Err(BlobError::Empty);
    }
    if blob.len() <= UNIXFS_CHUNK_BYTES {
        return Ok(cid_v1_raw_sha256(blob));
    }
    let chunks = blob.len().div_ceil(UNIXFS_CHUNK_BYTES);
    if blob.len() > MAX_FLIP_BYTES || chunks > UNIXFS_MAX_LINKS {
        return Err(BlobError::TooLarge);
    }

    // The serialized root: every link first, then the UnixFS file data. A link
    // carries the leaf's CID, a present but empty name, and the leaf's size.
    let mut node: Vec<u8> = Vec::with_capacity(64 + chunks * 48);
    let mut sizes: Vec<usize> = Vec::with_capacity(chunks);
    for chunk in blob.chunks(UNIXFS_CHUNK_BYTES) {
        let leaf = cid_v1_raw_sha256(chunk);
        let link = pb_link(&leaf, chunk.len());
        node.push(0x12);
        push_varint(&mut node, link.len());
        node.extend_from_slice(&link);
        sizes.push(chunk.len());
    }
    let data = unixfs_file_data(blob.len(), &sizes);
    node.push(0x0a);
    push_varint(&mut node, data.len());
    node.extend_from_slice(&data);

    let mut out = [0u8; CID_V1_RAW_SHA256_BYTES];
    out[..4].copy_from_slice(&[0x01, 0x70, 0x12, 0x20]);
    out[4..].copy_from_slice(&Sha256::digest(&node));
    Ok(out)
}

/// One PBLink: the leaf's CID (field 1), its name (field 2), and the size of the
/// subtree it commits to (field 3). The name is empty for a file link, but the
/// field is present all the same - an absent field would serialize to different
/// bytes, and therefore a different CID.
fn pb_link(cid: &[u8; CID_V1_RAW_SHA256_BYTES], tsize: usize) -> Vec<u8> {
    let mut link = Vec::with_capacity(cid.len() + 10);
    link.push(0x0a);
    push_varint(&mut link, cid.len());
    link.extend_from_slice(cid);
    link.push(0x12);
    link.push(0x00);
    link.push(0x18);
    push_varint(&mut link, tsize);
    link
}

/// The UnixFS `Data` message of a file node: the type (field 1), the total size
/// (field 3), and every block's size (field 4).
fn unixfs_file_data(filesize: usize, sizes: &[usize]) -> Vec<u8> {
    let mut data = Vec::with_capacity(8 + sizes.len() * 5);
    data.push(0x08);
    data.push(0x02); // DataType::File
    data.push(0x18);
    push_varint(&mut data, filesize);
    for size in sizes {
        data.push(0x20);
        push_varint(&mut data, *size);
    }
    data
}

/// A protobuf varint, the encoding every dag-pb field length and integer uses.
fn push_varint(out: &mut Vec<u8>, value: usize) {
    let mut value = value as u64;
    while value >= 0x80 {
        out.push((value as u8) | 0x80);
        value >>= 7;
    }
    out.push(value as u8);
}
