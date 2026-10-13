//! Delivery ECIES v1: the construction the delivery proof commits to.
//!
//! The primitives are the repository-local ECIES of the pinned node
//! (`idena-go/crypto/ecies`, parameters `ECIES_AES128_SHA256` on secp256k1):
//! ECDH over secp256k1, the NIST SP 800-56A concatenation KDF with SHA-256,
//! AES-128-CTR, and an HMAC-SHA256 tag over the encrypted message.
//!
//! One detail differs from a plain `ecies.Encrypt` call: the CTR initialisation
//! vector comes from the second KDF block instead of a random draw. The wire
//! format is unchanged, so the repository-local library decrypts the result
//! without modification, while the ciphertext becomes a deterministic function
//! of the ephemeral scalar, the plaintext, and the recipient key. That
//! determinism is what lets a zkVM prove the encryption happened.

#![no_std]

extern crate alloc;

use alloc::vec::Vec;
use aes::Aes128;
use ctr::cipher::{KeyIvInit, StreamCipher};
use hmac::{Hmac, Mac};
use k256::elliptic_curve::group::prime::PrimeCurveAffine;
use k256::elliptic_curve::point::AffineCoordinates;
use k256::elliptic_curve::sec1::{FromEncodedPoint, ToEncodedPoint};
use k256::elliptic_curve::PrimeField;
use k256::{AffinePoint, EncodedPoint, ProjectivePoint, Scalar};
use sha2::{Digest, Sha256};

type HmacSha256 = Hmac<Sha256>;
type Aes128Ctr = ctr::Ctr128BE<Aes128>;

/// Domain separator of the canonical secrets encoding.
pub const SECRETS_DOMAIN: &[u8] = b"fod:epoch-secrets:v1";
pub const EPHEMERAL_BYTES: usize = 32;
pub const PUBLIC_KEY_BYTES: usize = 65;
/// Ephemeral public key, CTR IV, and HMAC tag around the plaintext.
pub const CIPHERTEXT_OVERHEAD: usize = PUBLIC_KEY_BYTES + 16 + 32;

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

/// The commitment the registration proof and the delivery proof share.
pub fn secrets_commitment(secrets: &[u8]) -> [u8; 32] {
    let digest = Sha256::digest(secrets);
    let mut out = [0u8; 32];
    out.copy_from_slice(&digest);
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

/// Encrypts `plaintext` to `buyer_key` under the pinned construction.
///
/// Returns `None` for a malformed recipient key or an unusable ephemeral scalar.
pub fn encrypt_to(buyer_key: &[u8], plaintext: &[u8], ephemeral: &[u8]) -> Option<Vec<u8>> {
    if buyer_key.len() != PUBLIC_KEY_BYTES || ephemeral.len() != EPHEMERAL_BYTES {
        return None;
    }
    let encoded = EncodedPoint::from_bytes(buyer_key).ok()?;
    let buyer = Option::<AffinePoint>::from(AffinePoint::from_encoded_point(&encoded))?;
    if bool::from(buyer.is_identity()) {
        return None;
    }

    let mut scalar_bytes = [0u8; 32];
    scalar_bytes.copy_from_slice(ephemeral);
    let scalar = Option::<Scalar>::from(Scalar::from_repr(scalar_bytes.into()))?;
    if bool::from(scalar.is_zero()) {
        return None;
    }

    let ephemeral_point = AffinePoint::from(ProjectivePoint::GENERATOR * scalar);
    let shared = AffinePoint::from(ProjectivePoint::from(buyer) * scalar);
    let z = shared.x();

    // Blocks 1 and 2 of the concatenation KDF. Block 1 supplies the same key
    // material the repository-local library derives; block 2 supplies the IV.
    let k1 = kdf_block(&z, 1);
    let k2 = kdf_block(&z, 2);
    let key = &k1[..16];
    let mac_key = Sha256::digest(&k1[16..32]);
    let iv = &k2[..16];

    let mut body = plaintext.to_vec();
    let mut cipher = Aes128Ctr::new(key.into(), iv.into());
    cipher.apply_keystream(&mut body);

    let mut encrypted_message = Vec::with_capacity(16 + body.len());
    encrypted_message.extend_from_slice(iv);
    encrypted_message.extend_from_slice(&body);

    let mut mac = HmacSha256::new_from_slice(&mac_key).ok()?;
    mac.update(&encrypted_message);
    let tag = mac.finalize().into_bytes();

    let ephemeral_bytes = ephemeral_point.to_encoded_point(false);
    let mut out = Vec::with_capacity(CIPHERTEXT_OVERHEAD + plaintext.len());
    out.extend_from_slice(ephemeral_bytes.as_bytes());
    out.extend_from_slice(&encrypted_message);
    out.extend_from_slice(&tag);
    Some(out)
}

/// Parses a 32-byte scalar the way the encryption path does, for callers that
/// need to validate a key or an ephemeral value before using it.
pub fn scalar_from_bytes(bytes: &[u8]) -> Option<Scalar> {
    if bytes.len() != 32 {
        return None;
    }
    let mut raw = [0u8; 32];
    raw.copy_from_slice(bytes);
    Option::<Scalar>::from(Scalar::from_repr(raw.into()))
}

/// Computes the public key matching a secret scalar.
pub fn public_key_of(secret: &[u8]) -> Option<Vec<u8>> {
    let scalar = scalar_from_bytes(secret)?;
    if bool::from(scalar.is_zero()) {
        return None;
    }
    let point = AffinePoint::from(ProjectivePoint::GENERATOR * scalar);
    Some(point.to_encoded_point(false).as_bytes().to_vec())
}
