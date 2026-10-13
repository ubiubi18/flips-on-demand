//! Produces one genuine RISC Zero Groth16 receipt and exports the material a
//! contract-side verifier needs.
//!
//! Output: `dist/receipt.json` with the seal, the claim digest, the control
//! root, the BN254 identity control id, the image id, and the journal.
//!
//! Verification recipe (from risc0-zkvm 3.0.6 `src/receipt/groth16.rs`):
//! `risc0_groth16::Verifier::new(seal, control_root, claim_digest,
//! bn254_control_id, &risc0_groth16::verifying_key())`, where `control_root` is
//! `risc0_circuit_recursion::control_id::ALLOWED_CONTROL_ROOT` and
//! `bn254_control_id` is `BN254_IDENTITY_CONTROL_ID`.

use methods::{METHOD_ELF, METHOD_ID};
use risc0_circuit_recursion::control_id::{ALLOWED_CONTROL_ROOT, BN254_IDENTITY_CONTROL_ID};
use risc0_zkvm::sha::Digestible;
use risc0_zkvm::{default_prover, ExecutorEnv, InnerReceipt, ProverOpts};
use std::fs;

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

/// `METHOD_ID` is `[u32; 8]` (little-endian words), not a `Digest`.
fn image_id_bytes() -> Vec<u8> {
    METHOD_ID.iter().flat_map(|w| w.to_le_bytes()).collect()
}

fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::filter::EnvFilter::from_default_env())
        .init();

    // Trivial guest input: the guest commits it back as the journal.
    let input: u32 = 42;
    let env = ExecutorEnv::builder()
        .write(&input)
        .unwrap()
        .build()
        .unwrap();

    let prover = default_prover();
    let opts = ProverOpts::groth16();
    let prove_info = prover.prove_with_opts(env, METHOD_ELF, &opts).unwrap();
    let receipt = prove_info.receipt;

    let seal = match &receipt.inner {
        InnerReceipt::Groth16(inner) => inner.seal.clone(),
        other => panic!("expected a Groth16 receipt, got: {other:?}"),
    };

    let claim = receipt.claim().unwrap();
    let claim_digest = claim.digest();
    let journal = receipt.journal.bytes.clone();

    let out = serde_json::json!({
        "image_id": hex(&image_id_bytes()),
        "seal": hex(&seal),
        "seal_bytes": seal.len(),
        "claim_digest": hex(claim_digest.as_bytes()),
        "control_root": hex(ALLOWED_CONTROL_ROOT.as_bytes()),
        "bn254_control_id": hex(BN254_IDENTITY_CONTROL_ID.as_bytes()),
        "journal": hex(&journal),
        "journal_len": journal.len(),
    });

    fs::create_dir_all("dist").unwrap();
    fs::write("dist/receipt.json", serde_json::to_string_pretty(&out).unwrap()).unwrap();

    println!("seal: {} bytes", seal.len());
    println!("image_id: {}", hex(&image_id_bytes()));
    println!("claim_digest: {}", hex(claim_digest.as_bytes()));
    println!("control_root: {}", hex(ALLOWED_CONTROL_ROOT.as_bytes()));
    println!(
        "bn254_control_id: {}",
        hex(BN254_IDENTITY_CONTROL_ID.as_bytes())
    );
    println!("wrote dist/receipt.json");
}
