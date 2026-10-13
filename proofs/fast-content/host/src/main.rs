//! Proves one explicitly supplied pre-session content witness against copies
//! already prepared for the same author, epoch, contract, and CID set.

use fast_content_methods::{FAST_CONTENT_GUEST_ELF, FAST_CONTENT_GUEST_ID};
use fod_fast_content_core::{self as content, Input, Record};
use fod_registration_core::blob;
use risc0_zkvm::sha::Digestible;
use risc0_zkvm::{
    default_executor, default_prover, ExecutorEnv, ExitCode, Groth16Receipt,
    Groth16ReceiptVerifierParameters, InnerReceipt, MaybePruned, ProverOpts, Receipt, ReceiptClaim,
};
use serde::Serialize;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::fs::{self, File, OpenOptions};
use std::io::{Read, Write};
use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};

const NETWORK: &str = "idena";
const CONSENSUS: &str = "idena-go@9bc57040337fd19f45ba65323939cac8789b9efd";
const POLICY: &str = "fod:eligibility:v1";
const MAX_WITNESS_JSON: usize = 64 * 1024 * 1024;
const MAX_MARKER_JSON: usize = 32 * 1024;
const MAX_PRIVATE_JSON: usize = 4 * 1024;
const MAX_PROOF_JSON: usize = 64 * 1024;
const MAX_COPY_BYTES: usize = blob::MAX_FLIP_BYTES + 45;
const SEAL_BYTES: usize = 256;

struct Original {
    author: [u8; 20],
    epoch: u16,
    public_secret: [u8; 32],
    private_secret: [u8; 32],
    blobs: Vec<(Vec<u8>, Vec<u8>)>,
}

struct Prepared {
    message: [u8; 33],
    blind: [u8; 32],
    copies: Vec<Vec<u8>>,
    items: Vec<Value>,
    journal: [u8; content::JOURNAL_BYTES],
}

fn read_bounded(
    path: &Path,
    maximum: usize,
    label: &str,
    private: bool,
) -> Result<Vec<u8>, String> {
    let metadata = fs::symlink_metadata(path).map_err(|_| format!("cannot inspect {label}"))?;
    if !metadata.is_file() || metadata.file_type().is_symlink() || metadata.len() > maximum as u64 {
        return Err(format!("{label} is not a bounded regular file"));
    }
    if private && metadata.permissions().mode() & 0o077 != 0 {
        return Err(format!("{label} must be owner-only"));
    }
    let mut bytes = Vec::new();
    let file = File::open(path).map_err(|_| format!("cannot open {label}"))?;
    let opened = file
        .metadata()
        .map_err(|_| format!("cannot inspect opened {label}"))?;
    if !opened.is_file() || (private && opened.permissions().mode() & 0o077 != 0) {
        return Err(format!("opened {label} is not an owner-only regular file"));
    }
    file.take((maximum + 1) as u64)
        .read_to_end(&mut bytes)
        .map_err(|_| format!("cannot read {label}"))?;
    if bytes.len() > maximum {
        return Err(format!("{label} exceeds size limit"));
    }
    Ok(bytes)
}

fn read_json(path: &Path, maximum: usize, label: &str, private: bool) -> Result<Value, String> {
    serde_json::from_slice(&read_bounded(path, maximum, label, private)?)
        .map_err(|_| format!("invalid {label} JSON"))
}

fn field<'a>(value: &'a Value, name: &str) -> Result<&'a str, String> {
    value[name]
        .as_str()
        .ok_or_else(|| format!("missing {name}"))
}

fn unhex(value: &str, maximum: usize, name: &str) -> Result<Vec<u8>, String> {
    let text = value.strip_prefix("0x").unwrap_or(value);
    if text.len() > maximum * 2
        || text.len() % 2 != 0
        || !text.bytes().all(|byte| byte.is_ascii_hexdigit())
    {
        return Err(format!("invalid {name}"));
    }
    text.as_bytes()
        .chunks_exact(2)
        .map(|pair| {
            u8::from_str_radix(std::str::from_utf8(pair).expect("ASCII hex"), 16)
                .map_err(|_| format!("invalid {name}"))
        })
        .collect()
}

fn fixed<const N: usize>(value: &Value, name: &str) -> Result<[u8; N], String> {
    unhex(field(value, name)?, N, name)?
        .try_into()
        .map_err(|_| format!("wrong {name} length"))
}

fn compiled_image_id() -> [u8; 32] {
    std::array::from_fn(|at| FAST_CONTENT_GUEST_ID[at / 4].to_le_bytes()[at % 4])
}

/// Verify an already sealed, public credential without loading any witness or
/// re-running the prover. The contract still independently checks the seal and
/// canonical session leaves before it accepts a fallback.
fn verify_saved_artefact(artefact: &Value, contract: &[u8; 20]) -> Result<(), String> {
    if artefact["format"].as_str() != Some("fod:fast-content-sealed:v1")
        || artefact["statement"].as_str() != Some("fod:fast-content:v1")
        || artefact["sealBytes"].as_u64() != Some(SEAL_BYTES as u64)
    {
        return Err("unexpected content proof format".into());
    }
    if fixed::<32>(artefact, "image_id")? != compiled_image_id() {
        return Err("content proof uses another guest image".into());
    }
    let journal = unhex(
        field(artefact, "journal")?,
        content::JOURNAL_BYTES,
        "journal",
    )?;
    let statement = content::Journal::decode(&journal).ok_or("invalid content journal")?;
    let expected_network: [u8; 32] = Sha256::digest(NETWORK.as_bytes()).into();
    let expected_consensus: [u8; 32] = Sha256::digest(CONSENSUS.as_bytes()).into();
    let expected_policy: [u8; 32] = Sha256::digest(POLICY.as_bytes()).into();
    if statement.contract != *contract
        || statement.network != expected_network
        || statement.consensus != expected_consensus
        || statement.policy != expected_policy
        || statement.count == 0
        || usize::from(statement.count) > content::MAX_FLIPS
    {
        return Err("content proof has another contract or policy".into());
    }
    let seal = unhex(field(artefact, "seal")?, SEAL_BYTES, "seal")?;
    if seal.len() != SEAL_BYTES {
        return Err("wrong content seal length".into());
    }
    let recorded_claim = fixed::<32>(artefact, "claim_digest")?;
    let params = Groth16ReceiptVerifierParameters::default();
    let claim = ReceiptClaim::ok(FAST_CONTENT_GUEST_ID, journal.clone());
    let inner = Groth16Receipt::new(seal, MaybePruned::Value(claim), params.digest());
    let receipt = Receipt::new(InnerReceipt::Groth16(inner), journal);
    receipt
        .verify(FAST_CONTENT_GUEST_ID)
        .map_err(|_| "Groth16 content seal does not verify".to_string())?;
    if receipt
        .claim()
        .map_err(|_| "missing content receipt claim".to_string())?
        .digest()
        .as_bytes()
        != &recorded_claim
    {
        return Err("recorded content claim differs from the seal".into());
    }
    Ok(())
}

fn cid(value: &Value, name: &str) -> Result<Vec<u8>, String> {
    let text = value.as_str().ok_or_else(|| format!("missing {name}"))?;
    if text.is_empty()
        || text.len() > 128
        || !text
            .bytes()
            .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit())
    {
        return Err(format!("invalid {name}"));
    }
    Ok(text.as_bytes().to_vec())
}

fn original(value: &Value) -> Result<Original, String> {
    let author = fixed(value, "author")?;
    let epoch = value["epoch"]
        .as_u64()
        .and_then(|number| u16::try_from(number).ok())
        .ok_or("invalid epoch")?;
    let public_secret = fixed(value, "publicPartSecret")?;
    let private_secret = fixed(value, "privatePartSecret")?;
    let entries: Vec<&Value> = match value.get("blobs") {
        Some(Value::Array(items))
            if value.get("blob").is_none() && value.get("blobCid").is_none() =>
        {
            items.iter().collect()
        }
        Some(_) => return Err("invalid blobs array".into()),
        None => vec![value],
    };
    if entries.is_empty() || entries.len() > content::MAX_FLIPS {
        return Err("flip count outside bounds".into());
    }
    let mut blobs = Vec::with_capacity(entries.len());
    for entry in entries {
        let original_cid = cid(&entry["blobCid"], "blobCid")?;
        let body = unhex(field(entry, "blob")?, blob::MAX_FLIP_BYTES, "blob")?;
        blobs.push((original_cid, body));
    }
    blobs.sort_by(|left, right| left.0.cmp(&right.0));
    if blobs.windows(2).any(|pair| pair[0].0 == pair[1].0) {
        return Err("duplicate original CID".into());
    }
    Ok(Original {
        author,
        epoch,
        public_secret,
        private_secret,
        blobs,
    })
}

fn prepared(dir: &Path, source: &Original, contract: &[u8; 20]) -> Result<Prepared, String> {
    let metadata = fs::symlink_metadata(dir).map_err(|_| "cannot inspect prepared directory")?;
    if !metadata.is_dir()
        || metadata.file_type().is_symlink()
        || metadata.permissions().mode() & 0o077 != 0
    {
        return Err("prepared directory must be owner-only".into());
    }
    let copy_dir = dir.join("copies");
    let metadata =
        fs::symlink_metadata(&copy_dir).map_err(|_| "cannot inspect copies directory")?;
    if !metadata.is_dir()
        || metadata.file_type().is_symlink()
        || metadata.permissions().mode() & 0o077 != 0
    {
        return Err("copies directory must be owner-only".into());
    }
    let marker = read_json(
        &dir.join("prepared-unsealed.json"),
        MAX_MARKER_JSON,
        "marker",
        false,
    )?;
    let private = read_json(
        &dir.join("seller-private.json"),
        MAX_PRIVATE_JSON,
        "seller data",
        true,
    )?;
    if marker["format"].as_str() != Some("fod:fast-content-prepared-unsealed:v1")
        || private["format"].as_str() != Some("fod:fast-content-seller-private:v1")
    {
        return Err("wrong prepared format".into());
    }
    if fixed::<20>(&private, "author")? != source.author
        || private["epoch"].as_u64() != Some(u64::from(source.epoch))
        || fixed::<20>(&private, "contract")? != *contract
    {
        return Err("prepared identity or contract differs from witness".into());
    }
    let message = fixed(&private, "message")?;
    let blind = fixed(&private, "blind")?;
    let journal: [u8; content::JOURNAL_BYTES] = unhex(
        field(&marker, "journal")?,
        content::JOURNAL_BYTES,
        "marker journal",
    )?
    .try_into()
    .map_err(|_| "wrong marker journal length")?;
    if unhex(
        field(&private, "journal")?,
        content::JOURNAL_BYTES,
        "seller journal",
    )? != journal
    {
        return Err("prepared journals differ".into());
    }
    let decoded = content::Journal::decode(&journal).ok_or("invalid prepared journal")?;
    if decoded.author != source.author
        || decoded.epoch != source.epoch
        || decoded.contract != *contract
    {
        return Err("prepared journal context differs from witness".into());
    }
    if fixed::<32>(&marker, "copiesCommitment")? != decoded.copies {
        return Err("prepared copies commitment differs from journal".into());
    }
    let items = marker["items"].as_array().ok_or("missing prepared items")?;
    if items.len() != source.blobs.len() {
        return Err("prepared CID set differs from witness".into());
    }
    let mut copies = Vec::with_capacity(items.len());
    for (item, (original_cid, _)) in items.iter().zip(&source.blobs) {
        if cid(&item["originalCid"], "originalCid")? != *original_cid {
            return Err("prepared CID set differs from witness".into());
        }
        let copy_cid = cid(&item["copyCid"], "copyCid")?;
        let filename = std::str::from_utf8(&copy_cid).expect("ASCII CID");
        let copy = read_bounded(
            &copy_dir.join(filename),
            MAX_COPY_BYTES,
            "prepared copy",
            false,
        )?;
        if blob::blob_cid_multibase(&copy)
            .map_err(|_| "invalid prepared copy")?
            .as_bytes()
            != copy_cid
        {
            return Err("prepared copy CID mismatch".into());
        }
        copies.push(copy);
    }
    Ok(Prepared {
        message,
        blind,
        copies,
        items: items.clone(),
        journal,
    })
}

fn native_journal(
    source: &Original,
    prepared: &Prepared,
    contract: &[u8; 20],
) -> Result<[u8; content::JOURNAL_BYTES], String> {
    let records: Vec<Record<'_>> = source
        .blobs
        .iter()
        .zip(&prepared.copies)
        .map(|((cid, body), copy)| Record {
            original_cid: cid,
            original_blob: body,
            encrypted_copy: copy,
        })
        .collect();
    content::verify(&Input {
        network: NETWORK,
        contract,
        author: &source.author,
        epoch: source.epoch,
        consensus: CONSENSUS,
        policy: POLICY,
        public_secret: &source.public_secret,
        private_secret: &source.private_secret,
        message: &prepared.message,
        blind: &prepared.blind,
        records: &records,
    })
    .map(|journal| journal.encode())
    .map_err(|_| "native content verification refused witness".into())
}

fn push<T: Serialize>(words: &mut Vec<u32>, value: &T) -> Result<(), String> {
    words.extend(risc0_zkvm::serde::to_vec(value).map_err(|_| "cannot encode guest input")?);
    Ok(())
}

fn push_bytes(words: &mut Vec<u32>, bytes: &[u8]) -> Result<(), String> {
    push(
        words,
        &u32::try_from(bytes.len()).map_err(|_| "oversized guest item")?,
    )?;
    for chunk in bytes.chunks(4) {
        let mut padded = [0u8; 4];
        padded[..chunk.len()].copy_from_slice(chunk);
        words.push(u32::from_le_bytes(padded));
    }
    Ok(())
}

fn guest_words(
    source: &Original,
    prepared: &Prepared,
    contract: &[u8; 20],
) -> Result<Vec<u32>, String> {
    let mut words = Vec::new();
    push(&mut words, &content::STATEMENT_VERSION)?;
    push(&mut words, &NETWORK.to_string())?;
    push(&mut words, &contract.to_vec())?;
    push(&mut words, &source.author.to_vec())?;
    push(&mut words, &source.epoch)?;
    push(&mut words, &CONSENSUS.to_string())?;
    push(&mut words, &POLICY.to_string())?;
    push(&mut words, &source.public_secret.to_vec())?;
    push(&mut words, &source.private_secret.to_vec())?;
    push(&mut words, &prepared.message.to_vec())?;
    push(&mut words, &prepared.blind.to_vec())?;
    push(&mut words, &(source.blobs.len() as u32))?;
    for ((cid, body), copy) in source.blobs.iter().zip(&prepared.copies) {
        push(&mut words, cid)?;
        push_bytes(&mut words, body)?;
        push_bytes(&mut words, copy)?;
    }
    Ok(words)
}

fn env(words: &[u32]) -> Result<ExecutorEnv<'_>, String> {
    ExecutorEnv::builder()
        .write_slice(words)
        .build()
        .map_err(|_| "cannot build guest environment".into())
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}

fn create_output(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let mut file = OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .open(path)
        .map_err(|_| "cannot create new proof output")?;
    if file.write_all(bytes).and_then(|_| file.sync_all()).is_err() {
        let _ = fs::remove_file(path);
        return Err("cannot write proof output".into());
    }
    Ok(())
}

fn flag(args: &[String], name: &str) -> Result<String, String> {
    let mut found = None;
    for pair in args.chunks_exact(2) {
        if pair[0] == name && found.replace(pair[1].clone()).is_some() {
            return Err(format!("repeated {name}"));
        }
    }
    found.ok_or_else(|| format!("missing {name}"))
}

fn run(args: &[String]) -> Result<(), String> {
    if args.len() == 1 && args[0] == "--image-id" {
        println!("{}", hex(&compiled_image_id()));
        return Ok(());
    }
    if args.first().is_some_and(|arg| arg == "--verify") {
        if args.len() != 4 || args[2] != "--contract" {
            return Err("usage: fast-content-host --verify PROOF.json --contract 0x...".into());
        }
        let contract: [u8; 20] = unhex(&args[3], 20, "contract")?
            .try_into()
            .map_err(|_| "wrong contract length")?;
        let artefact = read_json(Path::new(&args[1]), MAX_PROOF_JSON, "content proof", false)?;
        verify_saved_artefact(&artefact, &contract)?;
        println!("sealed content proof verified for the expected contract");
        return Ok(());
    }
    let prove = args.iter().any(|arg| arg == "--prove");
    let flags: Vec<String> = args
        .iter()
        .filter(|arg| arg.as_str() != "--prove")
        .cloned()
        .collect();
    if args.iter().filter(|arg| arg.as_str() == "--prove").count() > 1
        || flags.len() != 8
        || flags.chunks_exact(2).any(|pair| {
            !["--input", "--prepared", "--contract", "--out"].contains(&pair[0].as_str())
        })
    {
        return Err("usage: fast-content-host --input ORIGINAL.json --prepared DIRECTORY --contract 0x... --out NEW_FILE [--prove]".into());
    }
    let input_path = PathBuf::from(flag(&flags, "--input")?);
    let prepared_dir = PathBuf::from(flag(&flags, "--prepared")?);
    let contract: [u8; 20] = unhex(&flag(&flags, "--contract")?, 20, "contract")?
        .try_into()
        .map_err(|_| "wrong contract length")?;
    let out_path = PathBuf::from(flag(&flags, "--out")?);
    if prove {
        match fs::symlink_metadata(&out_path) {
            Ok(_) => return Err("proof output already exists".into()),
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => {}
            Err(_) => return Err("cannot inspect proof output".into()),
        }
        let parent = out_path
            .parent()
            .filter(|parent| !parent.as_os_str().is_empty())
            .unwrap_or_else(|| Path::new("."));
        if !fs::metadata(parent).is_ok_and(|metadata| metadata.is_dir()) {
            return Err("proof output directory does not exist".into());
        }
    }
    let witness = read_json(&input_path, MAX_WITNESS_JSON, "original witness", true)?;
    if witness.get("contract").is_some() && fixed::<20>(&witness, "contract")? != contract {
        return Err("witness contract differs from requested contract".into());
    }
    let source = original(&witness)?;
    let prepared = prepared(&prepared_dir, &source, &contract)?;
    let expected = native_journal(&source, &prepared, &contract)?;
    if expected != prepared.journal {
        return Err("native journal differs from prepared marker".into());
    }
    let words = guest_words(&source, &prepared, &contract)?;
    let session = default_executor()
        .execute(env(&words)?, FAST_CONTENT_GUEST_ELF)
        .map_err(|_| "guest execution failed")?;
    if session.exit_code != ExitCode::Halted(0) || session.journal.bytes != expected {
        return Err("guest journal differs from prepared marker".into());
    }
    if !prove {
        println!("content preflight passed; no proof created");
        return Ok(());
    }
    let receipt = default_prover()
        .prove_with_opts(env(&words)?, FAST_CONTENT_GUEST_ELF, &ProverOpts::groth16())
        .map_err(|_| "Groth16 proving failed")?
        .receipt;
    receipt
        .verify(FAST_CONTENT_GUEST_ID)
        .map_err(|_| "receipt verification failed")?;
    if receipt.journal.bytes != expected || receipt.journal.bytes.len() != content::JOURNAL_BYTES {
        return Err("receipt journal differs from prepared marker".into());
    }
    let seal = receipt
        .inner
        .groth16()
        .map_err(|_| "receipt is not Groth16")?
        .seal
        .as_slice();
    if seal.len() != SEAL_BYTES {
        return Err("wrong Groth16 seal length".into());
    }
    let claim = receipt
        .claim()
        .map_err(|_| "invalid receipt claim")?
        .digest();
    let image_id = compiled_image_id();
    let artifact = json!({
        "format": "fod:fast-content-sealed:v1",
        "statement": format!("fod:fast-content:v{}", content::STATEMENT_VERSION),
        "journal": hex(&expected),
        "seal": hex(seal),
        "items": prepared.items,
        "claim_digest": hex(claim.as_bytes()),
        "image_id": hex(&image_id),
        "sealBytes": seal.len(),
    });
    let mut bytes =
        serde_json::to_vec_pretty(&artifact).map_err(|_| "cannot encode proof output")?;
    bytes.push(b'\n');
    create_output(&out_path, &bytes)?;
    println!("sealed content proof created");
    Ok(())
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if let Err(problem) = run(&args) {
        eprintln!("content host failed: {problem}");
        std::process::exit(1);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn recorded_seal_verifies_but_stale_guest_image_is_rejected() {
        let artefact: Value =
            serde_json::from_str(include_str!("../../dist/synthetic-one-flip.json"))
                .expect("synthetic sealed fixture");
        let contract = [0x11; 20];
        let image = fixed::<32>(&artefact, "image_id").unwrap();
        let image_words: [u32; 8] = std::array::from_fn(|at| {
            u32::from_le_bytes(image[at * 4..at * 4 + 4].try_into().unwrap())
        });
        let journal = unhex(
            field(&artefact, "journal").unwrap(),
            content::JOURNAL_BYTES,
            "journal",
        )
        .unwrap();
        let seal = unhex(field(&artefact, "seal").unwrap(), SEAL_BYTES, "seal").unwrap();
        let verify_seal = |seal: Vec<u8>| {
            let claim = ReceiptClaim::ok(image_words, journal.clone());
            let inner = Groth16Receipt::new(
                seal,
                MaybePruned::Value(claim),
                Groth16ReceiptVerifierParameters::default().digest(),
            );
            Receipt::new(InnerReceipt::Groth16(inner), journal.clone()).verify(image_words)
        };
        verify_seal(seal.clone()).expect("recorded seal verifies for its recorded image");
        assert!(verify_saved_artefact(&artefact, &contract).is_err());
        assert!(verify_saved_artefact(&artefact, &[0x22; 20]).is_err());
        let mut tampered = seal;
        tampered[0] ^= 1;
        assert!(verify_seal(tampered).is_err());
    }
}
