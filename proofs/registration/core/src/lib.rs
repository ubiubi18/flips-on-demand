//! Registration core: the pinned construction in the direction the registration
//! proof needs, plus the secrets encoding and the blob CID the manifest binds.
//!
//! This is deliberately a second implementation rather than a shared crate. The
//! delivery proof's image id pins `fod-delivery-core` (`SOURCES.md` F-70): adding
//! a single unused function to it moves the id, which would invalidate the
//! contract's pinned constant, the release manifest's recorded id, and every
//! stored receipt. So the construction lives in both places, and
//! `proofs/vectors/ecies-v1.json` is what keeps them honest: the delivery side
//! proves it reproduces the vector's ciphertext, and this side proves it recovers
//! the vector's plaintext.
//!
//! The parameters are the repository-local ECIES of the pinned node
//! (`idena-go/crypto/ecies`, `ECIES_AES128_SHA256` on secp256k1), with the CTR
//! initialisation vector taken from the second KDF block so the ciphertext is a
//! deterministic function of the ephemeral scalar, the plaintext, and the
//! recipient key.

#![no_std]

extern crate alloc;

pub mod blob;
pub mod qualification;

use aes::Aes128;
use alloc::vec::Vec;
use ctr::cipher::{KeyIvInit, StreamCipher};
#[cfg(not(feature = "risc0-sha"))]
use hmac::Hmac;
use hmac::Mac;
#[cfg(feature = "risc0-sha")]
use hmac::SimpleHmac;
use k256::elliptic_curve::group::prime::PrimeCurveAffine;
use k256::elliptic_curve::point::AffineCoordinates;
use k256::elliptic_curve::sec1::{FromEncodedPoint, ToEncodedPoint};
use k256::elliptic_curve::PrimeField;
use k256::{AffinePoint, EncodedPoint, ProjectivePoint, Scalar};
#[cfg(feature = "risc0-sha")]
use risc0_zkvm::sha::rust_crypto::{Digest, Sha256};
#[cfg(not(feature = "risc0-sha"))]
use sha2::{Digest, Sha256};

#[cfg(feature = "risc0-sha")]
type HmacSha256 = SimpleHmac<Sha256>;
#[cfg(not(feature = "risc0-sha"))]
type HmacSha256 = Hmac<Sha256>;
type Aes128Ctr = ctr::Ctr128BE<Aes128>;

/// Domain separator of the canonical secrets encoding, the same as the delivery
/// side's: the key commitment in a registration journal has to be the commitment
/// the sale settles.
pub const SECRETS_DOMAIN: &[u8] = b"fod:epoch-secrets:v1";
pub const EPHEMERAL_BYTES: usize = 32;
pub const PUBLIC_KEY_BYTES: usize = 65;
/// Ephemeral public key, CTR IV, and HMAC tag around the plaintext.
pub const CIPHERTEXT_OVERHEAD: usize = PUBLIC_KEY_BYTES + 16 + 32;
/// A CIDv1 raw sha2-256 value: the multicodec prefix and a 32-byte digest.
pub const CID_V1_RAW_SHA256_BYTES: usize = 36;

/// Canonical bytes of the secrets a bundle sells: the two per-epoch secp256k1
/// private scalars, bound to the author and epoch they belong to.
pub fn secrets_bytes(
    epoch: u16,
    author: &[u8; 20],
    public_part: &[u8; 32],
    private_part: &[u8; 32],
) -> Vec<u8> {
    let mut out = Vec::with_capacity(SECRETS_DOMAIN.len() + 2 + 20 + 64);
    out.extend_from_slice(SECRETS_DOMAIN);
    out.extend_from_slice(&epoch.to_be_bytes());
    out.extend_from_slice(author);
    out.extend_from_slice(public_part);
    out.extend_from_slice(private_part);
    out
}

/// The commitment a registration journal carries and the delivery proof binds.
pub fn secrets_commitment(secrets: &[u8]) -> [u8; 32] {
    let digest = Sha256::digest(secrets);
    let mut out = [0u8; 32];
    out.copy_from_slice(&digest);
    out
}

/// The CIDv1 raw sha2-256 value of a blob: the form the node uses for flip
/// blobs, so a manifest's CID can be checked against the bytes it names.
pub fn cid_v1_raw_sha256(data: &[u8]) -> [u8; CID_V1_RAW_SHA256_BYTES] {
    let mut out = [0u8; CID_V1_RAW_SHA256_BYTES];
    out[..4].copy_from_slice(&[0x01, 0x55, 0x12, 0x20]);
    out[4..].copy_from_slice(&Sha256::digest(data));
    out
}

fn kdf_block(z: &[u8], counter: u32) -> [u8; 32] {
    let mut hasher = Sha256::new();
    hasher.update(counter.to_be_bytes());
    hasher.update(z);
    let digest = hasher.finalize();
    let mut out = [0u8; 32];
    out.copy_from_slice(&digest);
    out
}

/// Parses a 32-byte scalar the way the construction does.
pub fn scalar_from_bytes(bytes: &[u8]) -> Option<Scalar> {
    if bytes.len() != 32 {
        return None;
    }
    let mut raw = [0u8; 32];
    raw.copy_from_slice(bytes);
    Option::<Scalar>::from(Scalar::from_repr(raw.into()))
}

/// The public key matching a secret scalar.
pub fn public_key_of(secret: &[u8]) -> Option<Vec<u8>> {
    let scalar = scalar_from_bytes(secret)?;
    if bool::from(scalar.is_zero()) {
        return None;
    }
    let point = AffinePoint::from(ProjectivePoint::GENERATOR * scalar);
    Some(point.to_encoded_point(false).as_bytes().to_vec())
}

/// Decrypts a ciphertext under the pinned construction with the recipient's
/// secret scalar: the ephemeral public key is the first 65 bytes, the tag is the
/// last 32, and the IV and body are between them.
///
/// The tag is verified before anything is decrypted, so a tampered message is
/// refused instead of decrypted into noise. `None` means the key, the point, or
/// the tag did not check out.
pub fn decrypt_from(secret: &[u8], ciphertext: &[u8]) -> Option<Vec<u8>> {
    if ciphertext.len() < CIPHERTEXT_OVERHEAD {
        return None;
    }
    let ephemeral_bytes = &ciphertext[..PUBLIC_KEY_BYTES];
    let encrypted_message = &ciphertext[PUBLIC_KEY_BYTES..ciphertext.len() - 32];
    let tag = &ciphertext[ciphertext.len() - 32..];

    let scalar = scalar_from_bytes(secret)?;
    if bool::from(scalar.is_zero()) {
        return None;
    }
    let encoded = EncodedPoint::from_bytes(ephemeral_bytes).ok()?;
    let ephemeral = Option::<AffinePoint>::from(AffinePoint::from_encoded_point(&encoded))?;
    if bool::from(ephemeral.is_identity()) {
        return None;
    }

    let shared = AffinePoint::from(ProjectivePoint::from(ephemeral) * scalar);
    let z = shared.x();
    let k1 = kdf_block(&z, 1);
    // The second KDF block is not computed here: the IV travels with the
    // ciphertext, which is what lets this open a blob the node encrypted.
    let key = &k1[..16];
    let mac_key = Sha256::digest(&k1[16..32]);

    let mut mac = HmacSha256::new_from_slice(&mac_key).ok()?;
    mac.update(encrypted_message);
    mac.verify_slice(tag).ok()?;

    let iv = &encrypted_message[..16];
    let mut out = encrypted_message[16..].to_vec();
    let mut cipher = Aes128Ctr::new(key.into(), iv.into());
    cipher.apply_keystream(&mut out);
    Some(out)
}
