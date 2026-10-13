//! Cycle probe for the registration guest's primitives.
//!
//! This is a measurement, not a proof program. It reads its input the way one of
//! the two ingestion paths does, times the work with the guest's own cycle
//! counter, and commits the two deltas so the host can read them from the
//! session's journal without proving anything.
//!
//! The modes are the ones a registration proof per flip needs: ingesting a blob
//! either through serde's `Vec<u8>` or through a raw slice, SHA-256 over a blob,
//! the AES-128-CTR and HMAC-SHA256 halves of the pinned ECIES construction, one
//! secp256k1 ECDH, and `flip` - the whole per-flip mix of two ECDH operations,
//! the stream cipher, the tag, and the digest.

use aes::Aes128;
use ctr::cipher::{KeyIvInit, StreamCipher};
use hmac::{Hmac, Mac};
use k256::elliptic_curve::group::prime::PrimeCurveAffine;
use k256::elliptic_curve::point::AffineCoordinates;
use k256::elliptic_curve::sec1::{FromEncodedPoint, ToEncodedPoint};
use k256::elliptic_curve::PrimeField;
use k256::{AffinePoint, EncodedPoint, ProjectivePoint, Scalar};
use risc0_zkvm::guest::env;
use sha2::{Digest, Sha256};

type HmacSha256 = Hmac<Sha256>;

const MODE_BASELINE: u32 = 0;
const MODE_SHA256: u32 = 1;
const MODE_AES: u32 = 2;
const MODE_ECDH: u32 = 3;
const MODE_FLIP: u32 = 4;
const MODE_RAW: u32 = 5;
const MODE_READ: u32 = 6;

fn scalar_from_bytes(bytes: &[u8]) -> Scalar {
    let mut raw = [0u8; 32];
    raw.copy_from_slice(&bytes[..32]);
    Option::<Scalar>::from(Scalar::from_repr(raw.into())).expect("scalar")
}

/// One ECDH exactly as the pinned construction does it: a base-point
/// multiplication by the secret scalar, then the shared x coordinate.
fn ecdh(secret: &[u8], counterpart: &[u8]) -> [u8; 32] {
    let scalar = scalar_from_bytes(secret);
    let encoded = EncodedPoint::from_bytes(counterpart).expect("point");
    let point = Option::<AffinePoint>::from(AffinePoint::from_encoded_point(&encoded)).expect("affine");
    let shared = AffinePoint::from(ProjectivePoint::from(point) * scalar);
    let mut out = [0u8; 32];
    out.copy_from_slice(&shared.x());
    out
}

/// The AES-128-CTR half and the HMAC tag, over `body`, under a fixed key.
fn stream_and_tag(body: &mut [u8]) -> [u8; 32] {
    let key = [0x11u8; 16];
    let iv = [0x22u8; 16];
    let mut cipher = ctr::Ctr128BE::<Aes128>::new((&key).into(), (&iv).into());
    cipher.apply_keystream(body);

    let mac_key = Sha256::digest(key);
    let mut mac = HmacSha256::new_from_slice(&mac_key).expect("hmac key");
    mac.update(body);
    let tag = mac.finalize().into_bytes();
    let mut out = [0u8; 32];
    out.copy_from_slice(&tag);
    out
}

fn main() {
    let mode: u32 = env::read();
    let ingest_start = env::cycle_count();
    let payload: Vec<u8> = if mode == MODE_RAW {
        // The ingestion path a blob wants: one length, then raw bytes. The serde
        // path used by the other modes reads a Vec<u8> element by element.
        let length: u32 = env::read();
        let mut payload = vec![0u8; length as usize];
        env::read_slice(&mut payload);
        payload
    } else {
        env::read()
    };
    let secret: Vec<u8> = env::read();
    let counterpart: Vec<u8> = env::read();
    let ingest_end = env::cycle_count();

    let mut out = [0u8; 32];
    match mode {
        MODE_BASELINE | MODE_READ | MODE_RAW => {
            out.copy_from_slice(&Sha256::digest(&payload));
        }
        MODE_SHA256 => {
            out.copy_from_slice(&Sha256::digest(&payload));
        }
        MODE_AES => {
            let mut body = payload.clone();
            out = stream_and_tag(&mut body);
        }
        MODE_ECDH => {
            out = ecdh(&secret, &counterpart);
        }
        MODE_FLIP => {
            // Two ECDH operations: one per encrypted part of a flip blob.
            let first = ecdh(&secret, &counterpart);
            let second = ecdh(&secret, &counterpart);
            let mut body = payload.clone();
            let tag = stream_and_tag(&mut body);
            let digest = Sha256::digest(&payload);

            let mut hasher = Sha256::new();
            hasher.update(first);
            hasher.update(second);
            hasher.update(tag);
            hasher.update(digest);
            out.copy_from_slice(&hasher.finalize());
        }
        other => panic!("unknown mode {other}"),
    }
    let work_end = env::cycle_count();

    // The digest is committed so the work cannot be optimized away, and the
    // deltas are committed so the host does not have to prove anything.
    let mut journal = [0u8; 32];
    journal[..8].copy_from_slice(&(mode as u64).to_le_bytes());
    journal[8..16].copy_from_slice(&(ingest_end - ingest_start).to_le_bytes());
    journal[16..24].copy_from_slice(&(work_end - ingest_end).to_le_bytes());
    journal[24..32].copy_from_slice(&out[..8]);
    env::commit_slice(&journal);
}
