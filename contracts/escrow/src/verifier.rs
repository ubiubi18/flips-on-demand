//! Contract-side RISC Zero verification for the delivery statement.
//!
//! The Package 1 probe verified a seal against a claim digest handed to it by
//! the prover, which proves nothing about which program ran or what it said.
//! The host's own check does not work that way: `Receipt::verify` rebuilds the
//! canonical claim with
//!
//! ```text
//! ReceiptClaim::ok(image_id, Pruned(sha256(journal)))
//! ```
//!
//! and compares its digest with the receipt's claim (`risc0-zkvm 3.0.6`
//! `src/receipt.rs`, `verify_with_context`). This module does the same thing
//! inside the contract, so the seal can only settle an order when it proves
//! exactly this journal for exactly this program.
//!
//! The digest construction follows `risc0-binfmt 3.0.5` `src/hash.rs`
//! (`tagged_struct`) and the field digests in `risc0-zkvm 3.0.6`
//! `src/claim/receipt.rs` (`ReceiptClaim::ok`, `Output`) and
//! `risc0-binfmt 3.0.5` `src/sys_state.rs` (`SystemState`).

use risc0_groth16::{verifying_key, Verifier};
use risc0_zkp::core::digest::Digest;
use sha2::{Digest as _, Sha256};

/// Journal layout of the delivery statement (see `SPEC.md` 5.6).
pub const JOURNAL_BYTES: usize = 281;
/// Journal layout of the registration statement (`SOURCES.md` F-69).
/// Version 2 binds the two anchored state versions of a session
/// (`SOURCES.md` F-97, `proofs/session-core/src/statement.rs`).
/// Version 3 publishes the inventory commitment and the network digest, so a
/// buyer can check a CID set against chain state and the contract can refuse a
/// statement made for another network.
pub const REGISTRATION_JOURNAL_BYTES: usize = 326;
pub const SEAL_BYTES: usize = 256;
/// Statement versions. The delivery statement (`proofs/delivery`) and the
/// registration statement version independently, so one constant cannot serve
/// both: bumping it for the registration statement would refuse every delivery
/// proof, and the other way round.
pub const DELIVERY_JOURNAL_VERSION: u32 = 2;
pub const REGISTRATION_JOURNAL_VERSION: u32 = 3;

/// Image id of the delivery guest in `proofs/delivery`, from the proven run
/// recorded in `proofs/delivery/dist/delivery-v2.json`.
pub const DELIVERY_IMAGE_ID: [u8; 32] = [
    0x6b, 0xcb, 0x72, 0x5e, 0xff, 0x90, 0x08, 0xb2, 0xba, 0xba, 0x7a, 0xec, 0x22, 0x7b, 0xdf, 0x03,
    0x42, 0xdd, 0x11, 0xe7, 0xe0, 0x3a, 0x34, 0xb2, 0x6a, 0x49, 0xf0, 0xa7, 0x2c, 0xab, 0xd4, 0x43,
];

/// Image id of the registration guest in `proofs/registration-v2`, from the
/// proven run recorded in `proofs/registration-v2/dist/registration-v2.json`.
/// The version 1 guest in `proofs/registration` keeps its own id and artefact;
/// the escrow accepts one statement at a time, so the swap happens here.
///
/// `methods.rs` carries the id as eight `u32` words and the canonical byte
/// string is each word's little-endian bytes, which is how the host writes the
/// id into the artefact (`SOURCES.md` F-75). Packing the words big-endian
/// produces the byte-swapped id, which no receipt can ever satisfy.
pub const REGISTRATION_IMAGE_ID: [u8; 32] = [
    0xbd, 0x4b, 0xea, 0x61, 0xef, 0x9c, 0x37, 0x0c, 0x5e, 0x4c, 0x5a, 0x0c, 0x51, 0xe3, 0xaf, 0x81,
    0x46, 0xd2, 0xfe, 0xa5, 0x7f, 0x5b, 0x3c, 0x83, 0x10, 0x58, 0x96, 0xc8, 0x4d, 0x68, 0xcc, 0x4d,
];

/// Fast content image reproduced in two clean source builds and checked by
/// `proofs/fast-content/dist/synthetic-one-flip.json`. The host verifies the
/// genuine Groth16 receipt before writing that public fixture.
#[cfg(feature = "fast-candidate")]
pub const FAST_CONTENT_IMAGE_ID: [u8; 32] = [
    0xb8, 0x58, 0x6d, 0x40, 0xd7, 0xd5, 0xb9, 0x12, 0xa1, 0xce, 0x38, 0x63, 0x00, 0x9c, 0x9c, 0xcb,
    0x88, 0xeb, 0x83, 0x6d, 0x2b, 0x44, 0x1a, 0x3a, 0x08, 0xce, 0x32, 0xec, 0xc8, 0x5b, 0xd7, 0xd2,
];

/// Verifier parameters of the pinned RISC Zero version, taken from
/// `risc0-circuit-recursion 4.0.5` `src/control_id.rs` (the same values the
/// Package 1 probe embedded and verified against).
pub const ALLOWED_CONTROL_ROOT: [u8; 32] = [
    0xa5, 0x4d, 0xc8, 0x5a, 0xc9, 0x9f, 0x85, 0x1c, 0x92, 0xd7, 0xc9, 0x6d, 0x73, 0x18, 0xaf, 0x41,
    0xdb, 0xe7, 0xc0, 0x19, 0x4e, 0xdf, 0xcc, 0x37, 0xeb, 0x4d, 0x42, 0x2a, 0x99, 0x8c, 0x1f, 0x56,
];

pub const BN254_IDENTITY_CONTROL_ID: [u8; 32] = [
    0xc0, 0x7a, 0x65, 0x14, 0x5c, 0x3c, 0xb4, 0x8b, 0x61, 0x01, 0x96, 0x2e, 0xa6, 0x07, 0xa4, 0xdd,
    0x93, 0xc7, 0x53, 0xbb, 0x26, 0x97, 0x5c, 0xb4, 0x7f, 0xeb, 0x00, 0xd3, 0x66, 0x6e, 0x44, 0x04,
];

/// Standard SHA-256, which is also what RISC Zero's `hash_bytes` computes
/// (`risc0-zkp` `src/core/hash/sha/cpu.rs`, `sha2::Sha256::digest`), and whose
/// bytes are the canonical form of a `Digest`.
pub fn sha256(data: &[u8]) -> [u8; 32] {
    let mut out = [0u8; 32];
    out.copy_from_slice(&Sha256::digest(data));
    out
}

/// `risc0_binfmt::hash::tagged_struct`: the tag's digest, the field digests, the
/// data words as little-endian u32s, and the field count as a little-endian u16.
fn tagged_struct(tag: &str, down: &[[u8; 32]], data: &[u32]) -> [u8; 32] {
    let mut buf = alloc::vec::Vec::with_capacity(64 + 32 * (down.len() + 1) + 4 * data.len() + 2);
    buf.extend_from_slice(&sha256(tag.as_bytes()));
    for field in down {
        buf.extend_from_slice(field);
    }
    for word in data {
        buf.extend_from_slice(&word.to_le_bytes());
    }
    buf.extend_from_slice(&(down.len() as u16).to_le_bytes());
    sha256(&buf)
}

/// Digest of the canonical successful-execution claim for a journal.
pub fn claim_digest(image_id: &[u8; 32], journal: &[u8]) -> [u8; 32] {
    // Output { journal: Pruned(sha256(journal)), assumptions: Pruned(ZERO) }
    let output = tagged_struct("risc0.Output", &[sha256(journal), [0u8; 32]], &[]);
    // input: MaybePruned::Value(None) -> Option::None digest is ZERO.
    let input = [0u8; 32];
    // pre: Pruned(image_id).
    let pre = *image_id;
    // post: SystemState { pc: 0, merkle_root: ZERO }.
    let post = tagged_struct("risc0.SystemState", &[[0u8; 32]], &[0]);
    // exit_code: Halted(0) -> (system_exit, user_exit) = (0, 0).
    tagged_struct("risc0.ReceiptClaim", &[input, pre, post, output], &[0, 0])
}

/// Verifies a delivery seal against the journal it must be proving.
pub fn verify_delivery(journal: &[u8], seal: &[u8]) -> bool {
    if journal.len() != JOURNAL_BYTES || seal.len() != SEAL_BYTES {
        return false;
    }
    let verifier = match Verifier::new(
        seal,
        Digest::from_bytes(ALLOWED_CONTROL_ROOT),
        Digest::from_bytes(claim_digest(&DELIVERY_IMAGE_ID, journal)),
        Digest::from_bytes(BN254_IDENTITY_CONTROL_ID),
        &verifying_key(),
    ) {
        Ok(verifier) => verifier,
        Err(_) => return false,
    };
    verifier.verify().is_ok()
}

/// Verifies a registration seal against the journal it must be proving.
pub fn verify_registration(journal: &[u8], seal: &[u8]) -> bool {
    verify_registration_image(journal, seal, &REGISTRATION_IMAGE_ID)
}

/// A separate contract may pin a later build of the same statement guest
/// without changing the image accepted by the original escrow.
pub fn verify_registration_image(journal: &[u8], seal: &[u8], image_id: &[u8; 32]) -> bool {
    if journal.len() != REGISTRATION_JOURNAL_BYTES || seal.len() != SEAL_BYTES {
        return false;
    }
    let verifier = match Verifier::new(
        seal,
        Digest::from_bytes(ALLOWED_CONTROL_ROOT),
        Digest::from_bytes(claim_digest(image_id, journal)),
        Digest::from_bytes(BN254_IDENTITY_CONTROL_ID),
        &verifying_key(),
    ) {
        Ok(verifier) => verifier,
        Err(_) => return false,
    };
    verifier.verify().is_ok()
}

#[cfg(feature = "fast-candidate")]
pub fn verify_fast_content(journal: &[u8], seal: &[u8]) -> bool {
    verify_fast_content_image(journal, seal, &FAST_CONTENT_IMAGE_ID)
}

/// A later direct contract can pin a new build of the content guest without
/// changing the image accepted by the original fast escrow.
#[cfg(feature = "fast-candidate")]
pub fn verify_fast_content_image(journal: &[u8], seal: &[u8], image_id: &[u8; 32]) -> bool {
    if *image_id == [0u8; 32]
        || journal.len() != fod_fast_content_core::JOURNAL_BYTES
        || seal.len() != SEAL_BYTES
    {
        return false;
    }
    let verifier = match Verifier::new(
        seal,
        Digest::from_bytes(ALLOWED_CONTROL_ROOT),
        Digest::from_bytes(claim_digest(image_id, journal)),
        Digest::from_bytes(BN254_IDENTITY_CONTROL_ID),
        &verifying_key(),
    ) {
        Ok(verifier) => verifier,
        Err(_) => return false,
    };
    verifier.verify().is_ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    // The proven delivery-v2 run: proofs/delivery/dist/delivery-v2.json, produced by
    // `cargo run --release --bin delivery-host` with dev mode off.
    const JOURNAL: &[u8] = &hex_literal::hex!("000000021111111111111111111111111111111111111111000000000000000700000000000000012222222222222222222222222222222222222222333333333333333333333333333333333333333304c7e8ce2d833482ce249b86b7b9f739ac441e33e3b674d28a7de2d5e96d3bb2c4c3a1ea30571f583598451031e4566924435c6de709adcca76b6fdbe787c65e3193c72e262384abc773825769d4b1a0a9b749d4c4921367105c023c4eaf21329e28925793e6950fee60fe106790547a2fa96d145dd31f24215be872341af22f8bf4b1512d6254236f6c27a538770e790224076ef88b24d5bce8da7320093476d200000000000003e88888888888888888888888888888888888888888888888888888888888888888");
    const CLAIM_DIGEST: [u8; 32] =
        hex_literal::hex!("dbeae78499b37b102391404d5d641c3517bf583eb7ef2b91302fe05fc123044d");

    #[test]
    fn claim_digest_matches_the_proven_run() {
        assert_eq!(claim_digest(&DELIVERY_IMAGE_ID, JOURNAL), CLAIM_DIGEST);
    }

    #[test]
    fn tampered_journal_changes_the_claim() {
        let mut journal = JOURNAL.to_vec();
        journal[10] ^= 0x01;
        assert_ne!(claim_digest(&DELIVERY_IMAGE_ID, &journal), CLAIM_DIGEST);
    }

    #[test]
    fn receipt_for_the_previous_image_cannot_be_relabelled() {
        // Genuine seal from the previous 60ec600b... delivery image, over this
        // same journal. Updating an image label cannot migrate a receipt.
        let previous_seal = hex_literal::hex!("09de0315571d843fee378073b5a56c0b8d3c2f5cac9c3ba1b59f9eeb06e41e6408b20e675b9ca5243ef4e1e831dd154c36707233919c8a71d35316caf44fd43d211774115158208019564fba4558a28483e17b9b78a102b9aa3b236f4af4f7bb26330fbf820704d24c6284a445986ac4dbd5138ee45ae60ea8c6a27d7901e19e1946a0375def9587f0a4cd34aaf62d78a54f5c2ff815ad91a02c9f04d48b88d42959b226aab248f8989782f9e869dbae4a61bec41be21a69a027f9e6cbaea8102d8b25252dc7965312b9d72a44215764fbcfdb9779e23b4fd1efb0149594afcc0d91da6e97cf8150913af76ef741f0056d34349e8e0ddde887e3fb02fda3d169");
        assert!(!verify_delivery(JOURNAL, &previous_seal));
    }
}
