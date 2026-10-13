//! The sealed registration artefact has to verify against the guest this tree
//! builds, without proving it again.
//!
//! The escrow's check is this same check with the same constants: it rebuilds
//! the claim as `ReceiptClaim::ok(image_id, sha256(journal))` and verifies the
//! seal against it. A receipt that fails here can never register, and running
//! this costs one pairing check instead of a fifteen-minute proving run.

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
fn artefact() -> Value {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../dist/registration.json");
    let raw = std::fs::read_to_string(&path).unwrap_or_else(|problem| {
        panic!(
            "read {}: {problem}; the sealed artefact is committed - regenerate it with `cargo run --release -- --prove` in proofs/registration/host",
            path.display()
        )
    });
    serde_json::from_str(&raw).expect("parse the artefact")
}

#[test]
fn the_sealed_registration_artefact_verifies_against_the_guest() {
    let artefact = artefact();
    assert_eq!(
        artefact["statement"].as_str().expect("statement"),
        "fod:registration:v1"
    );
    let journal = hex(artefact["journal"].as_str().expect("journal"));
    let seal = hex(artefact["seal"].as_str().expect("seal"));
    assert_eq!(journal.len(), 178, "the journal is a fixed 178 bytes");
    assert_eq!(seal.len(), 256, "the seal is a fixed 256 bytes");

    let image_id = methods::REGISTRATION_GUEST_ID;
    let params = Groth16ReceiptVerifierParameters::default();
    let claim = ReceiptClaim::ok(image_id, journal.clone());
    let inner = Groth16Receipt::new(seal, MaybePruned::Value(claim), params.digest());
    let receipt = Receipt::new(InnerReceipt::Groth16(inner), journal);

    receipt
        .verify(image_id)
        .expect("the sealed receipt verifies against this guest");

    // When the host recorded these, they must agree with what was verified -
    // the older artefact predates both fields.
    if let Some(recorded) = artefact["claim_digest"].as_str() {
        let digest = receipt.claim().expect("claim").digest();
        assert_eq!(recorded, hex_of(digest.as_bytes()), "recorded claim digest");
    }
    if let Some(recorded) = artefact["image_id"].as_str() {
        let words: Vec<u8> = image_id.iter().flat_map(|w| w.to_le_bytes()).collect();
        assert_eq!(recorded, hex_of(&words), "recorded image id");
    }
}
