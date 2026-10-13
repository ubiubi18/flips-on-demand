//! The historical receipt keeps the deployed image pin. A separate direct-v2
//! receipt has to verify against the guest this tree currently builds.
//!
//! The escrow's check is this same check with the same constants: it rebuilds the
//! claim as `ReceiptClaim::ok(image_id, sha256(journal))` and verifies the seal
//! against it. A receipt that fails here can never register, and running this
//! costs one pairing check instead of a twenty-minute proving run.

use risc0_zkvm::sha::Digestible;
use risc0_zkvm::{
    Groth16Receipt, Groth16ReceiptVerifierParameters, InnerReceipt, MaybePruned, Receipt,
    ReceiptClaim,
};
use serde_json::Value;

fn hex(value: &str) -> Vec<u8> {
    (0..value.len())
        .step_by(2)
        .map(|at| u8::from_str_radix(&value[at..at + 2], 16).expect("hex"))
        .collect()
}

fn hex_of(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

/// The sealed artefact is committed, so a missing one is a broken checkout or a
/// release whose proof was never recorded - not a reason to pass quietly
/// (`REVIEW.md` A10).
fn artefact(name: &str) -> Value {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../dist")
        .join(name);
    let raw = std::fs::read_to_string(&path).unwrap_or_else(|problem| {
        panic!(
            "read {}: {problem}; regenerate the named fixture with `cargo run --release -- --prove --out <fixture path>` in proofs/registration-v2/host",
            path.display()
        )
    });
    serde_json::from_str(&raw).expect("parse the artefact")
}

fn verify_seal(artefact: Value, image_id: [u32; 8]) {
    assert_eq!(
        artefact["statement"].as_str().expect("statement"),
        format!(
            "fod:registration:v{}",
            fod_session_core::statement::STATEMENT_VERSION
        )
    );
    let journal = hex(artefact["journal"].as_str().expect("journal"));
    let seal = hex(artefact["seal"].as_str().expect("seal"));
    assert_eq!(
        journal.len(),
        fod_session_core::statement::JOURNAL_BYTES,
        "the journal has the active statement's fixed size"
    );
    assert_eq!(seal.len(), 256, "the seal is a fixed 256 bytes");

    let params = Groth16ReceiptVerifierParameters::default();
    let claim = ReceiptClaim::ok(image_id, journal.clone());
    let inner = Groth16Receipt::new(seal, MaybePruned::Value(claim), params.digest());
    let receipt = Receipt::new(InnerReceipt::Groth16(inner), journal);

    receipt
        .verify(image_id)
        .expect("the sealed receipt verifies against this guest");

    let digest = receipt.claim().expect("claim").digest();
    assert_eq!(
        artefact["claim_digest"].as_str().expect("claim digest"),
        hex_of(digest.as_bytes()),
        "recorded claim digest"
    );
    let words: Vec<u8> = image_id.iter().flat_map(|w| w.to_le_bytes()).collect();
    assert_eq!(
        artefact["image_id"].as_str().expect("image id"),
        hex_of(&words),
        "recorded image id"
    );
}

#[test]
fn the_historical_seal_verifies_against_its_deployed_image() {
    let old_image: [u8; 32] =
        hex("bd4bea61ef9c370c5e4c5a0c51e3af8146d2fea57f5b3c83105896c84d68cc4d")
            .try_into()
            .expect("historical image id");
    let words = std::array::from_fn(|at| {
        u32::from_le_bytes(old_image[at * 4..at * 4 + 4].try_into().unwrap())
    });
    verify_seal(artefact("registration-v2.json"), words);
}

#[test]
fn the_direct_v2_seal_verifies_against_the_current_guest_and_contract_pin() {
    let expected: [u8; 32] =
        hex("658db709b6de076b1f41cd14de106be4ef67e32353b3e46b41db689a11970b58")
            .try_into()
            .expect("direct image id");
    let guest = v2_methods::REGISTRATION_V2_GUEST_ID;
    let guest_bytes: Vec<u8> = guest.iter().flat_map(|word| word.to_le_bytes()).collect();
    assert_eq!(
        guest_bytes, expected,
        "current guest moved from the direct contract pin"
    );
    let contract = include_str!("../../../../contracts/direct-escrow/src/lib.rs");
    let pin = contract
        .split("const DIRECT_V2_IMAGE_ID: [u8; 32] = [")
        .nth(1)
        .expect("direct image pin")
        .split("];")
        .next()
        .expect("pin end");
    let pin_bytes: Vec<u8> = pin
        .split(',')
        .filter_map(|item| item.trim().strip_prefix("0x"))
        .map(|byte| u8::from_str_radix(byte, 16).expect("pin byte"))
        .collect();
    assert_eq!(
        pin_bytes, expected,
        "direct contract pin moved from the proven guest"
    );
    verify_seal(artefact("direct-v2-synthetic.json"), guest);
}
