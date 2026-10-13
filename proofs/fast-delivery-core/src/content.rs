//! Authenticated copy format shared by the pre-session witness and buyers.

use alloc::vec::Vec;
use chacha20poly1305::aead::{Aead, KeyInit, Payload};
use chacha20poly1305::{Key, XChaCha20Poly1305, XNonce};
use sha2::{Digest, Sha256};

const AAD_DOMAIN: &[u8] = b"fod:fast-content:item:v1";
const NONCE_DOMAIN: &[u8] = b"fod:fast-content:nonce:v1";
const VERSION: u8 = 1;
const NONCE_BYTES: usize = 24;
const TAG_BYTES: usize = 16;
const MAX_CID_BYTES: usize = 128;
const MAX_PARTS_BYTES: usize = 600 * 1024;

/// Every field is either fixed width or length prefixed. The original CID
/// commits to the input blob, while the index prevents nonce reuse among the
/// copies in one bundle.
pub struct CopyContext<'a> {
    pub network: &'a [u8; 32],
    pub contract: &'a [u8; 20],
    pub author: &'a [u8; 20],
    pub epoch: u16,
    pub index: u16,
    pub original_cid: &'a [u8],
}

fn associated_data(context: &CopyContext<'_>) -> Option<Vec<u8>> {
    if context.original_cid.is_empty() || context.original_cid.len() > MAX_CID_BYTES {
        return None;
    }
    let mut out = Vec::with_capacity(
        AAD_DOMAIN.len() + 32 + 20 + 20 + 2 + 2 + 2 + context.original_cid.len(),
    );
    out.extend_from_slice(AAD_DOMAIN);
    out.extend_from_slice(context.network);
    out.extend_from_slice(context.contract);
    out.extend_from_slice(context.author);
    out.extend_from_slice(&context.epoch.to_be_bytes());
    out.extend_from_slice(&context.index.to_be_bytes());
    out.extend_from_slice(&(context.original_cid.len() as u16).to_be_bytes());
    out.extend_from_slice(context.original_cid);
    Some(out)
}

fn nonce(aad: &[u8]) -> [u8; NONCE_BYTES] {
    let mut hasher = Sha256::new();
    hasher.update(NONCE_DOMAIN);
    hasher.update(aad);
    let digest = hasher.finalize();
    let mut out = [0u8; NONCE_BYTES];
    out.copy_from_slice(&digest[..NONCE_BYTES]);
    out
}

/// Version byte, deterministic 24-byte nonce, then XChaCha20-Poly1305
/// ciphertext and tag. The input CID and index make a repeated nonce imply
/// the same original blob and plaintext under the same bundle key.
pub fn seal_copy(
    key: &[u8; 32],
    context: &CopyContext<'_>,
    public_part: &[u8],
    private_part: &[u8],
) -> Option<Vec<u8>> {
    if public_part.is_empty()
        || private_part.is_empty()
        || public_part.len().checked_add(private_part.len())? > MAX_PARTS_BYTES
    {
        return None;
    }
    let aad = associated_data(context)?;
    let iv = nonce(&aad);
    let mut plain = Vec::with_capacity(4 + public_part.len() + private_part.len());
    plain.extend_from_slice(&(public_part.len() as u32).to_be_bytes());
    plain.extend_from_slice(public_part);
    plain.extend_from_slice(private_part);
    let cipher = XChaCha20Poly1305::new(&Key::from(*key));
    let encrypted = cipher
        .encrypt(
            &XNonce::from(iv),
            Payload {
                msg: &plain,
                aad: &aad,
            },
        )
        .ok()?;
    let mut out = Vec::with_capacity(1 + NONCE_BYTES + encrypted.len());
    out.push(VERSION);
    out.extend_from_slice(&iv);
    out.extend_from_slice(&encrypted);
    Some(out)
}

/// Authenticates before parsing the two flip parts. Any changed item context,
/// nonce, ciphertext or tag is refused.
pub fn open_copy(
    key: &[u8; 32],
    context: &CopyContext<'_>,
    envelope: &[u8],
) -> Option<(Vec<u8>, Vec<u8>)> {
    if envelope.len() < 1 + NONCE_BYTES + TAG_BYTES + 6 || envelope[0] != VERSION {
        return None;
    }
    let aad = associated_data(context)?;
    let iv = nonce(&aad);
    if envelope[1..1 + NONCE_BYTES] != iv {
        return None;
    }
    let cipher = XChaCha20Poly1305::new(&Key::from(*key));
    let plain = cipher
        .decrypt(
            &XNonce::from(iv),
            Payload {
                msg: &envelope[1 + NONCE_BYTES..],
                aad: &aad,
            },
        )
        .ok()?;
    if plain.len() < 6 || plain.len() - 4 > MAX_PARTS_BYTES {
        return None;
    }
    let public_len = u32::from_be_bytes(plain[..4].try_into().ok()?) as usize;
    if public_len == 0 || public_len >= plain.len() - 4 {
        return None;
    }
    Some((
        plain[4..4 + public_len].to_vec(),
        plain[4 + public_len..].to_vec(),
    ))
}

#[cfg(test)]
mod tests {
    extern crate std;
    use super::*;

    fn context<'a>(cid: &'a [u8], index: u16) -> CopyContext<'a> {
        CopyContext {
            network: &[1; 32],
            contract: &[2; 20],
            author: &[3; 20],
            epoch: 230,
            index,
            original_cid: cid,
        }
    }

    #[test]
    fn copies_authenticate_the_exact_item_and_parts() {
        let key = [7; 32];
        let original = context(b"synthetic-cid-a", 0);
        let copy = seal_copy(&key, &original, b"public", b"private").unwrap();
        assert_eq!(
            open_copy(&key, &original, &copy),
            Some((b"public".to_vec(), b"private".to_vec()))
        );
        assert_eq!(
            seal_copy(&key, &original, b"public", b"private"),
            Some(copy.clone())
        );
        assert_eq!(
            open_copy(&key, &context(b"synthetic-cid-b", 0), &copy),
            None
        );
        assert_eq!(
            open_copy(&key, &context(b"synthetic-cid-a", 1), &copy),
            None
        );
        assert_ne!(
            seal_copy(&key, &context(b"synthetic-cid-a", 1), b"public", b"private"),
            Some(copy.clone())
        );
        let mut tampered = copy.clone();
        *tampered.last_mut().unwrap() ^= 1;
        assert_eq!(open_copy(&key, &original, &tampered), None);
        tampered = copy;
        tampered[1] ^= 1;
        assert_eq!(open_copy(&key, &original, &tampered), None);
    }

    #[test]
    fn empty_or_oversized_parts_are_refused() {
        let key = [7; 32];
        let context = context(b"synthetic-cid-a", 0);
        assert_eq!(seal_copy(&key, &context, b"", b"private"), None);
        assert_eq!(seal_copy(&key, &context, b"public", b""), None);
        assert_eq!(
            seal_copy(&key, &context, &[1; MAX_PARTS_BYTES], b"private"),
            None
        );
    }
}
