//! The registration host. It proves the session-core statement, so the version
//! it sends is the statement's own constant rather than a copy that can drift.
//!
//! With no flags it builds the witness from the repository's own fixtures - the
//! session bundle recorded over the re-sealed flip blob's CID - executes the
//! guest and checks the 262-byte journal the guest commits. `--negative` runs
//! the same fixture with one mutation at a time, plus the recorded node-built
//! blob whose plaintext is not a flip, and requires the guest to refuse each of
//! them.
//!
//! Proving (`--prove`) and the escrow's constant swap land together: the escrow
//! pins one image id and one claim digest, so they change in the same step as
//! the seal.

use risc0_zkvm::{
    default_executor, default_prover, ExecutorEnv, ExitCode, Groth16Receipt,
    Groth16ReceiptVerifierParameters, InnerReceipt, MaybePruned, ProverOpts, Receipt, ReceiptClaim,
};
use risc0_zkvm::sha::Digestible;
use serde::Serialize;
use std::collections::{HashMap, HashSet};

const ADDRESS_BYTES: usize = 20;
const DIGEST_BYTES: usize = 32;
/// Immutable image ID in the deployed direct-v1 verifier. Verification uses
/// this contract pin even if a later source build produces another guest ID.
const DIRECT_V1_IMAGE_ID: [u8; DIGEST_BYTES] = [
    0xbd, 0x4b, 0xea, 0x61, 0xef, 0x9c, 0x37, 0x0c, 0x5e, 0x4c, 0x5a, 0x0c, 0x51, 0xe3, 0xaf, 0x81,
    0x46, 0xd2, 0xfe, 0xa5, 0x7f, 0x5b, 0x3c, 0x83, 0x10, 0x58, 0x96, 0xc8, 0x4d, 0x68, 0xcc, 0x4d,
];
/// Replacement direct escrow pin. The old `--verify` mode remains bound to
/// DIRECT_V1_IMAGE_ID for receipts made for the deployed contract.
const DIRECT_V2_IMAGE_ID: [u8; DIGEST_BYTES] = [
    0x65, 0x8d, 0xb7, 0x09, 0xb6, 0xde, 0x07, 0x6b, 0x1f, 0x41, 0xcd, 0x14, 0xde, 0x10, 0x6b, 0xe4,
    0xef, 0x67, 0xe3, 0x23, 0x53, 0xb3, 0xe4, 0x6b, 0x41, 0xdb, 0x68, 0x9a, 0x11, 0x97, 0x0b, 0x58,
];
/// A lab registry address for the fixture witness; the CLI substitutes a real
/// one when it builds the witness from a capture.
const CONTRACT: [u8; ADDRESS_BYTES] = [0x11; ADDRESS_BYTES];
const NETWORK: &str = "idena";
/// The node revision whose flip rules this statement proves, byte for byte the
/// string `tools/fod` pins as `ConsensusVersionPinned` and the escrow pins as
/// the digest of (`contracts/escrow/src/lib.rs`, `CONSENSUS_VERSION_DIGEST`).
/// A receipt made under any other label cannot register, so this constant and
/// the two consumers change together.
const CONSENSUS: &str = "idena-go@9bc57040337fd19f45ba65323939cac8789b9efd";
const POLICY: &str = "fod:eligibility:v1";

fn fixture(relative: &str) -> serde_json::Value {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join(relative);
    let raw = std::fs::read_to_string(&path).unwrap_or_else(|err| panic!("read {path:?}: {err}"));
    serde_json::from_str(&raw).expect("parse the fixture")
}

/// A JSON input named on the command line, with the recorded fixture as the
/// default. This is how a bundle captured from a node (`fod session-bundle`)
/// and its blob witness reach the guest without touching the fixtures.
fn input(args: &[String], name: &str, fallback: &str) -> serde_json::Value {
    let path = args
        .iter()
        .position(|arg| arg == name)
        .and_then(|at| args.get(at + 1))
        .map(std::path::PathBuf::from)
        .unwrap_or_else(|| std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join(fallback));
    let raw = std::fs::read_to_string(&path).unwrap_or_else(|err| panic!("read {path:?}: {err}"));
    serde_json::from_str(&raw).expect("parse the input")
}

/// A 20-byte address named on the command line, with a default. A live
/// registration carries the escrow's own address; the recorded fixture carries
/// the placeholder the fixture was proved with.
fn address(args: &[String], name: &str, fallback: [u8; ADDRESS_BYTES]) -> [u8; ADDRESS_BYTES] {
    match args.iter().position(|arg| arg == name).and_then(|at| args.get(at + 1)) {
        Some(value) => {
            let text = value.strip_prefix("0x").unwrap_or(value);
            let bytes = hex(text);
            assert_eq!(bytes.len(), ADDRESS_BYTES, "{name} must be a 20-byte address");
            bytes.try_into().expect("address size")
        }
        None => fallback,
    }
}

/// The two anchors: a fixture's `sessions` array, or a bundle captured from a
/// node, which carries them at the top level.
fn chosen_anchor(session: &serde_json::Value) -> &serde_json::Value {
    match session.get("sessions").and_then(|value| value.as_array()) {
        Some(sessions) => sessions
            .iter()
            .find(|entry| entry["label"].as_str() == Some("clean"))
            .expect("the clean session"),
        None => session,
    }
}

/// The manifest CIDs as the multibase text the record, the availability
/// advisory and the blob's own CID agree on. A fixture and a bundle captured by
/// `fod` both carry that text, so no conversion happens here.
fn manifest_cids(session: &serde_json::Value) -> Vec<Vec<u8>> {
    session["manifest"]
        .as_array()
        .expect("manifest")
        .iter()
        .map(|cid| cid.as_str().expect("cid").as_bytes().to_vec())
        .collect()
}

fn hex(value: &str) -> Vec<u8> {
    (0..value.len())
        .step_by(2)
        .map(|at| u8::from_str_radix(&value[at..at + 2], 16).expect("hex"))
        .collect()
}

fn hex_of(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

fn artefact_hex(value: &serde_json::Value, field: &str, bytes: usize) -> Result<Vec<u8>, String> {
    let text = value[field]
        .as_str()
        .ok_or_else(|| format!("missing {field}"))?;
    if text.len() != bytes * 2 || !text.is_ascii() {
        return Err(format!("invalid {field} length"));
    }
    (0..text.len())
        .step_by(2)
        .map(|at| {
            u8::from_str_radix(&text[at..at + 2], 16)
                .map_err(|_| format!("invalid {field} hex"))
        })
        .collect()
}

fn verify_saved_artefact_for_image(raw: &str, contract: &[u8; ADDRESS_BYTES], expected_image: [u8; DIGEST_BYTES]) -> Result<(), String> {
    if raw.len() > 64 * 1024 {
        return Err("proof artefact is too large".to_string());
    }
    let artefact: serde_json::Value =
        serde_json::from_str(raw).map_err(|_| "invalid proof artefact JSON".to_string())?;
    let statement = format!("fod:registration:v{}", fod_session_core::statement::STATEMENT_VERSION);
    if artefact["statement"].as_str() != Some(statement.as_str()) {
        return Err("unexpected proof statement".to_string());
    }
    let image_id = std::array::from_fn(|at| {
        u32::from_le_bytes(expected_image[at * 4..at * 4 + 4].try_into().unwrap())
    });
    if artefact_hex(&artefact, "image_id", DIGEST_BYTES)? != expected_image {
        return Err("proof uses another guest image".to_string());
    }
    let journal = artefact_hex(&artefact, "journal", fod_session_core::statement::JOURNAL_BYTES)?;
    let contract_at = fod_session_core::statement::OFFSET_CONTRACT;
    if journal[contract_at..contract_at + ADDRESS_BYTES] != *contract {
        return Err("proof is bound to another contract".to_string());
    }
    let seal = artefact_hex(&artefact, "seal", 256)?;
    let recorded_claim = artefact_hex(&artefact, "claim_digest", DIGEST_BYTES)?;
    let params = Groth16ReceiptVerifierParameters::default();
    let claim = ReceiptClaim::ok(image_id, journal.clone());
    let inner = Groth16Receipt::new(seal, MaybePruned::Value(claim), params.digest());
    let receipt = Receipt::new(InnerReceipt::Groth16(inner), journal);
    receipt
        .verify(image_id)
        .map_err(|_| "Groth16 seal does not verify".to_string())?;
    if receipt.claim().map_err(|_| "missing receipt claim".to_string())?.digest().as_bytes()
        != recorded_claim.as_slice()
    {
        return Err("recorded claim digest differs from the seal".to_string());
    }
    Ok(())
}

#[cfg(test)]
fn verify_saved_artefact(raw: &str, contract: &[u8; ADDRESS_BYTES]) -> Result<(), String> {
    verify_saved_artefact_for_image(raw, contract, DIRECT_V1_IMAGE_ID)
}

type PathItem = (i64, i64, i64, Vec<u8>, Vec<u8>);
type Leaf = (Vec<u8>, Vec<u8>, i64, Vec<PathItem>);

fn leaf_of(value: &serde_json::Value) -> Leaf {
    (
        hex(value["keyHex"].as_str().expect("keyHex")),
        hex(value["valueHex"].as_str().expect("valueHex")),
        value["version"].as_i64().expect("version"),
        value["path"]
            .as_array()
            .expect("path")
            .iter()
            .map(|item| {
                (
                    item["height"].as_i64().expect("height"),
                    item["size"].as_i64().expect("size"),
                    item["version"].as_i64().expect("version"),
                    hex(item["leftHex"].as_str().unwrap_or("")),
                    hex(item["rightHex"].as_str().unwrap_or("")),
                )
            })
            .collect(),
    )
}

fn push<T: Serialize>(words: &mut Vec<u32>, value: &T) {
    words.extend(risc0_zkvm::serde::to_vec(value).expect("serialize"));
}

fn push_blob_bytes(words: &mut Vec<u32>, bytes: &[u8]) {
    push(words, &u32::try_from(bytes.len()).expect("blob length fits u32"));
    for chunk in bytes.chunks(4) {
        let mut padded = [0u8; 4];
        padded[..chunk.len()].copy_from_slice(chunk);
        words.push(u32::from_le_bytes(padded));
    }
}

/// Match the private witness to the manifest before sending it to the guest.
/// The guest then recomputes every blob's CID and opens it with the one epoch
/// secret pair. The original single-blob fixture remains a valid input.
fn ordered_blobs(
    manifest: &[Vec<u8>],
    input: &serde_json::Value,
) -> Result<Vec<(Vec<u8>, Vec<u8>)>, String> {
    let mut expected = HashSet::new();
    for cid in manifest {
        if !expected.insert(cid.as_slice()) {
            return Err("the manifest repeats a CID".to_string());
        }
    }
    let entries: Vec<&serde_json::Value> = match input.get("blobs") {
        Some(serde_json::Value::Array(entries)) => {
            if input.get("blobCid").is_some() || input.get("blob").is_some() {
                return Err("the witness mixes single and multiple blob fields".to_string());
            }
            entries.iter().collect()
        }
        Some(_) => return Err("the witness's blobs field is not an array".to_string()),
        None => vec![input],
    };
    let mut supplied = HashMap::new();
    for entry in entries {
        let cid = entry["blobCid"]
            .as_str()
            .ok_or("a witness blob has no CID")?
            .as_bytes()
            .to_vec();
        if !expected.contains(cid.as_slice()) {
            return Err("the witness contains a CID outside the manifest".to_string());
        }
        let body = entry["blob"].as_str().ok_or("a witness blob has no bytes")?;
        if body.len() % 2 != 0 || !body.bytes().all(|byte| byte.is_ascii_hexdigit()) {
            return Err("a witness blob has invalid hex bytes".to_string());
        }
        if supplied.insert(cid, hex(body)).is_some() {
            return Err("the witness repeats a blob CID".to_string());
        }
    }
    manifest
        .iter()
        .map(|cid| {
            supplied
                .remove(cid)
                .map(|body| (cid.clone(), body))
                .ok_or_else(|| "the witness is missing a manifest blob".to_string())
        })
        .collect()
}

/// The witness the guest reads, in its order.
fn words(session: &serde_json::Value, blob: &serde_json::Value, contract: &[u8; ADDRESS_BYTES]) -> Result<Vec<u32>, String> {
    let chosen = chosen_anchor(session);
    let author = hex(session["author"].as_str().expect("author"));
    let manifest: Vec<Vec<u8>> = manifest_cids(session);
    let blobs = ordered_blobs(&manifest, blob)?;

    let mut out = Vec::new();
    push(&mut out, &fod_session_core::statement::STATEMENT_VERSION);
    push(&mut out, &contract.to_vec());
    push(&mut out, &NETWORK.to_string());
    push(&mut out, &author);
    push(&mut out, &(chosen["epoch"].as_u64().expect("epoch") as u16));
    push(&mut out, &CONSENSUS.to_string());
    push(&mut out, &POLICY.to_string());
    push(&mut out, &chosen["inventory"]["height"].as_u64().expect("height"));
    push(&mut out, &hex(chosen["inventory"]["rootHex"].as_str().expect("root")));
    push(&mut out, &leaf_of(&chosen["inventory"]["global"]));
    push(&mut out, &leaf_of(&chosen["inventory"]["identity"]));
    push(&mut out, &chosen["verdict"]["height"].as_u64().expect("height"));
    push(&mut out, &hex(chosen["verdict"]["rootHex"].as_str().expect("root")));
    push(&mut out, &leaf_of(&chosen["verdict"]["global"]));
    push(&mut out, &leaf_of(&chosen["verdict"]["identity"]));
    push(&mut out, &manifest);
    push(&mut out, &u32::try_from(blobs.len()).expect("blob count fits u32"));
    for (cid, bytes) in &blobs {
        push(&mut out, cid);
        push_blob_bytes(&mut out, bytes);
    }
    push(&mut out, &hex(blob["publicPartSecret"].as_str().expect("publicPartSecret")));
    push(&mut out, &hex(blob["privatePartSecret"].as_str().expect("privatePartSecret")));
    Ok(out)
}

fn execute(
    session: &serde_json::Value,
    blob: &serde_json::Value,
    contract: &[u8; ADDRESS_BYTES],
) -> Result<Vec<u8>, String> {
    let witness = words(session, blob, contract)?;
    let env = ExecutorEnv::builder()
        .write_slice(&witness)
        .build()
        .expect("env");
    match default_executor().execute(env, v2_methods::REGISTRATION_V2_GUEST_ELF) {
        Ok(session) => match session.exit_code {
            ExitCode::Halted(0) => Ok(session.journal.bytes),
            other => Err(format!("the guest refused the witness ({other:?})")),
        },
        Err(err) => Err(format!("the guest refused the witness ({err})")),
    }
}

/// Proves the statement over the fixture witness, verifies the receipt here, and
/// writes the artefact the escrow's constants are taken from.
fn prove_and_write(
    session: &serde_json::Value,
    blob: &serde_json::Value,
    contract: &[u8; ADDRESS_BYTES],
    out: &str,
) {
    let witness = words(session, blob, contract).unwrap_or_else(|problem| panic!("{problem}"));
    let env = ExecutorEnv::builder()
        .write_slice(&witness)
        .build()
        .expect("env");
    let started = std::time::Instant::now();
    let prover = default_prover();
    let prove_info = prover
        .prove_with_opts(
            env,
            v2_methods::REGISTRATION_V2_GUEST_ELF,
            &ProverOpts::groth16(),
        )
        .expect("prove");
    let seconds = started.elapsed().as_secs_f64();
    let stats = &prove_info.stats;
    let receipt = prove_info.receipt;

    // A receipt nobody checked is not evidence: this is the check that pins the
    // run to the guest id the escrow will have to pin too.
    receipt
        .verify(v2_methods::REGISTRATION_V2_GUEST_ID)
        .expect("receipt verification");

    let journal = receipt.journal.bytes.clone();
    let claim_digest = receipt.claim().expect("receipt claim").digest();
    let seal = match receipt.inner.groth16() {
        Ok(groth16) => groth16.seal.to_vec(),
        Err(err) => panic!("no groth16 seal: {err}"),
    };

    println!("proving       {seconds:.1} s");
    println!("cycles        {} in {} segments", stats.total_cycles, stats.segments);
    println!("journal       {} bytes", journal.len());
    println!("seal          {} bytes", seal.len());
    println!(
        "image id      {}",
        hex_of(
            &v2_methods::REGISTRATION_V2_GUEST_ID
                .iter()
                .flat_map(|w| w.to_le_bytes())
                .collect::<Vec<_>>()
        )
    );
    println!("claim digest  {}", hex_of(claim_digest.as_bytes()));

    let path = match std::path::Path::new(out).is_absolute() {
        true => std::path::PathBuf::from(out),
        false => std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join(out),
    };
    std::fs::create_dir_all(path.parent().expect("parent")).expect("dist directory");
    let artefact = serde_json::json!({
        // The label the seller's CLI checks. It is taken from the statement's
        // own version, so a statement revision cannot leave this behind.
        "statement": format!(
            "fod:registration:v{}",
            fod_session_core::statement::STATEMENT_VERSION
        ),
        "journal": hex_of(&journal),
        "seal": hex_of(&seal),
        "claim_digest": hex_of(claim_digest.as_bytes()),
        "image_id": hex_of(
            &v2_methods::REGISTRATION_V2_GUEST_ID
                .iter()
                .flat_map(|w| w.to_le_bytes())
                .collect::<Vec<_>>(),
        ),
        "sealBytes": seal.len(),
        "totalCycles": stats.total_cycles,
        "segments": stats.segments,
        "proveSeconds": seconds,
    });
    std::fs::write(
        &path,
        format!("{}\n", serde_json::to_string_pretty(&artefact).unwrap()),
    )
    .expect("write the artefact");
    println!("artefact      {}", path.display());
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if let Some(at) = args.iter().position(|arg| arg == "--verify" || arg == "--verify-direct-v2") {
        let direct_v2 = args[at] == "--verify-direct-v2";
        let proof = args.get(at + 1).expect("verification requires an artefact file");
        let contract = args
            .iter()
            .position(|arg| arg == "--contract")
            .and_then(|at| args.get(at + 1))
            .expect("--verify requires --contract");
        let contract = hex(contract.strip_prefix("0x").unwrap_or(contract));
        let contract: [u8; ADDRESS_BYTES] = contract.try_into().expect("20-byte contract");
        let raw = std::fs::read_to_string(proof).expect("read proof artefact");
        let image = if direct_v2 { DIRECT_V2_IMAGE_ID } else { DIRECT_V1_IMAGE_ID };
        verify_saved_artefact_for_image(&raw, &contract, image)
            .unwrap_or_else(|problem| panic!("{problem}"));
        println!("sealed registration proof verified for the expected contract");
        return;
    }
    let negative = args.iter().any(|arg| arg == "--negative");
    let prove = args.iter().any(|arg| arg == "--prove");
    let out = args
        .iter()
        .position(|arg| arg == "--out")
        .and_then(|at| args.get(at + 1).cloned())
        .unwrap_or_else(|| "../dist/registration-v2.json".to_string());
    let session = input(&args, "--session", "../../session-core/fixtures/session-bundle-vector.json");
    let blob = input(&args, "--blob", "../../registration/vectors/blob-flip-v1.json");
    let contract = address(&args, "--contract", CONTRACT);

    if negative {
        let flip_first = |value: &serde_json::Value, field: &str| {
            let mut out = value.clone();
            let mut bytes = hex(value[field].as_str().expect(field));
            bytes[0] ^= 0xff;
            out[field] = serde_json::Value::String(hex_of(&bytes));
            out
        };
        // A blob whose bytes changed no longer hashes to the CID its record carries.
        let mut tampered_blob = blob.clone();
        let mut bytes = hex(blob["blob"].as_str().expect("blob"));
        bytes[100] ^= 0xff;
        tampered_blob["blob"] = serde_json::Value::String(hex_of(&bytes));

        // The session fixture carries one recorded CID, so a manifest that names
        // another one has no matching blob.
        let mut extra_cid = session.clone();
        extra_cid["manifest"] = serde_json::Value::Array(vec![
            session["manifest"][0].clone(),
            serde_json::Value::String("bafkreiextramanifestcid".to_string()),
        ]);

        // A witness whose epoch does not match the global leaves is refused by
        // the session check, not by a later comparison.
        let mut wrong_epoch = session.clone();
        wrong_epoch["sessions"][0]["epoch"] = serde_json::Value::Number(229.into());

        // The recorded node-built blob carries placeholder JSON rather than a
        // flip, so the content check of `SPEC.md` 5.5 item 6 refuses it even
        // though its record and its CID line up.
        let placeholder_session = fixture("../../session-core/fixtures/session-bundle-placeholder.json");
        let placeholder_blob = fixture("../../registration/vectors/blob-v1.json");

        let cases: Vec<(&str, serde_json::Value, serde_json::Value)> = vec![
            ("tampered blob", session.clone(), tampered_blob),
            ("wrong secret", session.clone(), flip_first(&blob, "publicPartSecret")),
            ("extra manifest cid", extra_cid, blob.clone()),
            ("wrong epoch", wrong_epoch, blob.clone()),
            ("placeholder content, not a flip", placeholder_session, placeholder_blob),
        ];

        let mut failures = 0;
        for (name, session_case, blob_case) in &cases {
            match execute(session_case, blob_case, &contract) {
                Ok(journal) => {
                    eprintln!(
                        "{name}: the guest accepted it ({} journal bytes)",
                        journal.len()
                    );
                    failures += 1;
                }
                Err(problem) => println!("{name}: refused ({problem})"),
            }
        }
        if failures > 0 {
            std::process::exit(1);
        }
        println!("every mutated witness was refused");
        return;
    }

    if prove {
        prove_and_write(&session, &blob, &contract, &out);
        return;
    }

    let journal = execute(&session, &blob, &contract).unwrap_or_else(|problem| panic!("{problem}"));
    assert_eq!(
        journal.len(),
        fod_session_core::statement::JOURNAL_BYTES,
        "the journal has the wrong size"
    );
    let u32_at = |offset: usize| u32::from_be_bytes(journal[offset..offset + 4].try_into().unwrap());
    let u16_at = |offset: usize| u16::from_be_bytes(journal[offset..offset + 2].try_into().unwrap());
    let u64_at = |offset: usize| u64::from_be_bytes(journal[offset..offset + 8].try_into().unwrap());
    let chosen = chosen_anchor(&session);
    // A captured bundle carries its own epoch, flags and anchors, so the
    // fixture's recorded values are not asserted for it; the important checks
    // are the guest's own, which ran above.
    if session.get("format").and_then(|value| value.as_str()) == Some("fod.session-bundle") {
        println!(
            "journal       {} bytes, epoch {}, {} flips, inventory {} -> verdict {}, contract {}",
            journal.len(),
            u16_at(fod_session_core::statement::OFFSET_EPOCH),
            u32_at(fod_session_core::statement::OFFSET_COVERED_FLIPS),
            u64_at(fod_session_core::statement::OFFSET_INVENTORY_HEIGHT),
            u64_at(fod_session_core::statement::OFFSET_VERDICT_HEIGHT),
            hex_of(&journal[fod_session_core::statement::OFFSET_CONTRACT
                ..fod_session_core::statement::OFFSET_CONTRACT + ADDRESS_BYTES]),
        );
        return;
    }

    assert_eq!(u32_at(fod_session_core::statement::OFFSET_VERSION), 3);
    assert_eq!(
        &journal[fod_session_core::statement::OFFSET_CONTRACT..fod_session_core::statement::OFFSET_CONTRACT + ADDRESS_BYTES],
        &contract
    );
    assert_eq!(
        &journal[fod_session_core::statement::OFFSET_AUTHOR..fod_session_core::statement::OFFSET_AUTHOR + ADDRESS_BYTES],
        hex(session["author"].as_str().expect("author")).as_slice()
    );
    assert_eq!(u16_at(fod_session_core::statement::OFFSET_EPOCH), 228);
    assert_eq!(u32_at(fod_session_core::statement::OFFSET_COVERED_FLIPS), 1);
    assert_eq!(
        u64_at(fod_session_core::statement::OFFSET_INVENTORY_HEIGHT),
        chosen["inventory"]["height"].as_u64().expect("height")
    );
    assert_eq!(
        &journal[fod_session_core::statement::OFFSET_INVENTORY_ROOT..fod_session_core::statement::OFFSET_INVENTORY_ROOT + DIGEST_BYTES],
        hex(chosen["inventory"]["rootHex"].as_str().expect("root")).as_slice()
    );
    assert_eq!(
        u64_at(fod_session_core::statement::OFFSET_VERDICT_HEIGHT),
        chosen["verdict"]["height"].as_u64().expect("height")
    );
    assert_eq!(
        &journal[fod_session_core::statement::OFFSET_VERDICT_ROOT..fod_session_core::statement::OFFSET_VERDICT_ROOT + DIGEST_BYTES],
        hex(chosen["verdict"]["rootHex"].as_str().expect("root")).as_slice()
    );
    assert_eq!(u32_at(fod_session_core::statement::OFFSET_VALIDATION_FLAGS), 0);
    // Version 3 publishes the commitment over the bundle's CIDs, in the form a
    // buyer recomputes from a candidate set (`tools/fod`, the C1 finding), and
    // the digest of the network this statement is for.
    assert_eq!(
        &journal[fod_session_core::statement::OFFSET_INVENTORY_COMMITMENT
            ..fod_session_core::statement::OFFSET_INVENTORY_COMMITMENT + DIGEST_BYTES],
        fod_session_core::statement::inventory_commitment(
            &hex(session["author"].as_str().expect("author"))
                .try_into()
                .expect("author size"),
            228,
            &session["manifest"]
                .as_array()
                .expect("manifest")
                .iter()
                .map(|cid| cid.as_str().expect("cid").as_bytes())
                .collect::<Vec<_>>(),
        )
        .as_slice()
    );
    assert_eq!(
        &journal[fod_session_core::statement::OFFSET_NETWORK_DIGEST
            ..fod_session_core::statement::OFFSET_NETWORK_DIGEST + DIGEST_BYTES],
        fod_session_core::statement::network_digest(NETWORK).as_slice()
    );

    println!("journal       {} bytes", journal.len());
    println!(
        "journal hex   {}",
        journal.iter().map(|b| format!("{b:02x}")).collect::<String>()
    );
    println!(
        "covered       {} flip(s), flags {}, label clean",
        u32_at(fod_session_core::statement::OFFSET_COVERED_FLIPS),
        u32_at(fod_session_core::statement::OFFSET_VALIDATION_FLAGS)
    );
    println!("the guest verified the session bundle, the blob and the statement");
}

#[cfg(test)]
mod tests {
    use super::{ordered_blobs, push_blob_bytes, verify_saved_artefact, DIRECT_V1_IMAGE_ID};
    use serde_json::json;

    #[test]
    fn saved_seal_pin_matches_the_direct_contract_verifier() {
        let verifier = include_str!("../../../../contracts/escrow/src/verifier.rs");
        let section = verifier
            .split("pub const REGISTRATION_IMAGE_ID: [u8; 32] = [")
            .nth(1)
            .expect("registration image pin")
            .split("];")
            .next()
            .expect("pin end");
        let bytes: Vec<u8> = section
            .split(',')
            .filter_map(|word| word.trim().strip_prefix("0x"))
            .map(|hex| u8::from_str_radix(hex, 16).expect("pin byte"))
            .collect();
        assert_eq!(bytes, DIRECT_V1_IMAGE_ID);
    }

    #[test]
    fn saved_seal_verifies_without_reproving_and_rejects_tampering() {
        let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../dist/registration-v2.json");
        let raw = std::fs::read_to_string(path).expect("committed sealed fixture");
        let contract = [0x11; 20];
        verify_saved_artefact(&raw, &contract).expect("pinned seal");
        assert!(verify_saved_artefact(&raw, &[0x22; 20]).is_err());

        let mut changed: serde_json::Value = serde_json::from_str(&raw).expect("fixture JSON");
        let seal = changed["seal"].as_str().expect("seal");
        changed["seal"] = serde_json::Value::String(format!(
            "{}{}",
            if &seal[..1] == "0" { "1" } else { "0" },
            &seal[1..]
        ));
        assert!(verify_saved_artefact(&changed.to_string(), &contract).is_err());
    }

    #[test]
    fn multi_blob_witness_follows_the_manifest_and_rejects_incomplete_sets() {
        let manifest = vec![b"cid-a".to_vec(), b"cid-b".to_vec(), b"cid-c".to_vec()];
        let input = json!({
            "blobs": [
                {"blobCid": "cid-c", "blob": "03"},
                {"blobCid": "cid-a", "blob": "01"},
                {"blobCid": "cid-b", "blob": "02"}
            ],
            "publicPartSecret": "one pair",
            "privatePartSecret": "one pair"
        });
        let ordered = ordered_blobs(&manifest, &input).expect("all manifest blobs");
        assert_eq!(ordered, vec![
            (b"cid-a".to_vec(), vec![1]),
            (b"cid-b".to_vec(), vec![2]),
            (b"cid-c".to_vec(), vec![3]),
        ]);

        let mut missing = input.clone();
        missing["blobs"].as_array_mut().unwrap().pop();
        assert!(ordered_blobs(&manifest, &missing).is_err());

        let mut extra = input.clone();
        extra["blobs"].as_array_mut().unwrap().push(json!({"blobCid": "cid-d", "blob": "04"}));
        assert!(ordered_blobs(&manifest, &extra).is_err());

        let mut duplicate = input.clone();
        duplicate["blobs"][2]["blobCid"] = json!("cid-a");
        assert!(ordered_blobs(&manifest, &duplicate).is_err());

        let mut wrong_cid = input.clone();
        wrong_cid["blobs"][1]["blobCid"] = json!("cid-d");
        assert!(ordered_blobs(&manifest, &wrong_cid).is_err());
    }

    #[test]
    fn original_single_blob_witness_still_loads() {
        let manifest = vec![b"cid-a".to_vec()];
        let input = json!({"blobCid": "cid-a", "blob": "0102"});
        assert_eq!(
            ordered_blobs(&manifest, &input).unwrap(),
            vec![(b"cid-a".to_vec(), vec![1, 2])]
        );
    }

    #[test]
    fn raw_blob_words_preserve_bytes_and_pad_only_the_last_word() {
        let mut words = Vec::new();
        push_blob_bytes(&mut words, &[0, 1, 2, 3, 4]);
        assert_eq!(words, [5, 0x0302_0100, 4]);
    }
}
