//! Prepare public authenticated copies and a private seller key before the
//! validation session. Output is explicitly unsealed until a guest proof exists.

use fod_fast_content_core::{self as core, Input, Record};
use fod_fast_delivery_core::content::{seal_copy, CopyContext};
use fod_fast_delivery_core::SellerKey;
use fod_registration_core::blob;
use serde_json::{json, Value};
use std::fs::{DirBuilder, File, OpenOptions};
use std::io::{Read, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};

const CONSENSUS: &str = "idena-go@9bc57040337fd19f45ba65323939cac8789b9efd";
const POLICY: &str = "fod:eligibility:v1";
const MAX_WITNESS_JSON_BYTES: u64 = 64 * 1024 * 1024;

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}

fn unhex(text: &str) -> Result<Vec<u8>, String> {
    let value = text.strip_prefix("0x").unwrap_or(text);
    if value.len() % 2 != 0 || !value.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return Err("invalid hex field".into());
    }
    value
        .as_bytes()
        .chunks_exact(2)
        .map(|pair| {
            u8::from_str_radix(std::str::from_utf8(pair).expect("ASCII hex"), 16)
                .map_err(|_| "invalid hex field".to_string())
        })
        .collect()
}

fn fixed<const N: usize>(value: &Value, field: &str) -> Result<[u8; N], String> {
    unhex(
        value[field]
            .as_str()
            .ok_or_else(|| format!("missing {field}"))?,
    )?
    .try_into()
    .map_err(|_| format!("wrong {field} length"))
}

fn flag(args: &[String], name: &str) -> Result<String, String> {
    let mut found = None;
    for pair in args.chunks_exact(2) {
        if pair[0] == name {
            if found.replace(pair[1].clone()).is_some() {
                return Err(format!("repeated {name}"));
            }
        }
    }
    found.ok_or_else(|| format!("missing {name}"))
}

fn input_blobs(value: &Value) -> Result<Vec<(Vec<u8>, Vec<u8>)>, String> {
    let entries: Vec<&Value> = match value.get("blobs") {
        Some(Value::Array(items)) => items.iter().collect(),
        Some(_) => return Err("blobs must be an array".into()),
        None => vec![value],
    };
    if entries.is_empty() || entries.len() > core::MAX_FLIPS {
        return Err("flip count outside bounds".into());
    }
    let mut blobs = Vec::with_capacity(entries.len());
    for entry in entries {
        let cid = entry["blobCid"]
            .as_str()
            .ok_or("missing blob CID")?
            .as_bytes()
            .to_vec();
        let body = unhex(entry["blob"].as_str().ok_or("missing blob bytes")?)?;
        blobs.push((cid, body));
    }
    blobs.sort_by(|left, right| left.0.cmp(&right.0));
    if blobs.windows(2).any(|pair| pair[0].0 == pair[1].0) {
        return Err("duplicate blob CID".into());
    }
    Ok(blobs)
}

fn create_private_file(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let mut file = OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .open(path)
        .map_err(|err| format!("create private output: {err}"))?;
    file.write_all(bytes)
        .map_err(|err| format!("write private output: {err}"))?;
    file.sync_all()
        .map_err(|err| format!("sync private output: {err}"))
}

fn prepare(input_path: &Path, contract: [u8; 20], out_dir: &Path) -> Result<(), String> {
    let input_meta =
        std::fs::symlink_metadata(input_path).map_err(|_| "cannot inspect original witness")?;
    if !input_meta.is_file()
        || input_meta.file_type().is_symlink()
        || input_meta.permissions().mode() & 0o077 != 0
    {
        return Err("original witness must be an owner-only regular file".into());
    }
    let file = File::open(input_path).map_err(|err| format!("read input: {err}"))?;
    let opened_meta = file
        .metadata()
        .map_err(|_| "cannot inspect opened witness")?;
    if !opened_meta.is_file() || opened_meta.permissions().mode() & 0o077 != 0 {
        return Err("opened witness must be an owner-only regular file".into());
    }
    let mut bounded = file.take(MAX_WITNESS_JSON_BYTES + 1);
    let mut raw = Vec::new();
    bounded
        .read_to_end(&mut raw)
        .map_err(|err| format!("read input: {err}"))?;
    if raw.len() as u64 > MAX_WITNESS_JSON_BYTES {
        return Err("witness too large".into());
    }
    let input: Value = serde_json::from_slice(&raw).map_err(|err| format!("parse input: {err}"))?;
    let author: [u8; 20] = fixed(&input, "author")?;
    let epoch = input["epoch"]
        .as_u64()
        .ok_or("missing epoch")
        .and_then(|value| u16::try_from(value).map_err(|_| "epoch out of range"))?;
    let public_secret: [u8; 32] = fixed(&input, "publicPartSecret")?;
    let private_secret: [u8; 32] = fixed(&input, "privatePartSecret")?;
    let blobs = input_blobs(&input)?;
    let seller = SellerKey::generate();
    let (message, blind) = seller.witness_material();
    let network = fod_session_core::statement::network_digest("idena");
    let key = seller.content_key();
    let mut copies = Vec::with_capacity(blobs.len());
    for (index, (cid, body)) in blobs.iter().enumerate() {
        if blob::blob_cid_multibase(body)
            .map_err(|_| "invalid original blob")?
            .as_bytes()
            != cid
        {
            return Err("original CID mismatch".into());
        }
        let (public, private) = blob::open_flip_blob(body, &public_secret, &private_secret)
            .map_err(|_| "original blob does not decrypt")?;
        let encrypted = seal_copy(
            &key,
            &CopyContext {
                network: &network,
                contract: &contract,
                author: &author,
                epoch,
                index: index as u16,
                original_cid: cid,
            },
            &public,
            &private,
        )
        .ok_or("copy encryption refused")?;
        copies.push(encrypted);
    }
    let records: Vec<Record<'_>> = blobs
        .iter()
        .zip(&copies)
        .map(|((cid, body), copy)| Record {
            original_cid: cid,
            original_blob: body,
            encrypted_copy: copy,
        })
        .collect();
    let journal = core::verify(&Input {
        network: "idena",
        contract: &contract,
        author: &author,
        epoch,
        consensus: CONSENSUS,
        policy: POLICY,
        public_secret: &public_secret,
        private_secret: &private_secret,
        message: &message,
        blind: &blind,
        records: &records,
    })
    .map_err(|reason| format!("content refused: {reason:?}"))?;

    let mut directory = DirBuilder::new();
    directory.mode(0o700);
    directory
        .create(out_dir)
        .map_err(|err| format!("create new output directory: {err}"))?;
    let copy_dir = out_dir.join("copies");
    directory
        .create(&copy_dir)
        .map_err(|err| format!("create copies directory: {err}"))?;
    let mut pairs = Vec::with_capacity(blobs.len());
    for ((cid, _), encrypted) in blobs.iter().zip(&copies) {
        let copy_cid = blob::blob_cid_multibase(encrypted).map_err(|_| "copy CID failed")?;
        create_private_file(&copy_dir.join(&copy_cid), encrypted)?;
        pairs.push(json!({"originalCid": String::from_utf8_lossy(cid), "copyCid": copy_cid}));
    }
    let private = json!({
        "format": "fod:fast-content-seller-private:v1",
        "message": hex(&message),
        "blind": hex(&blind),
        "author": hex(&author),
        "epoch": epoch,
        "contract": hex(&contract),
        "journal": hex(&journal.encode()),
    });
    create_private_file(
        &out_dir.join("seller-private.json"),
        serde_json::to_string_pretty(&private)
            .map_err(|err| err.to_string())?
            .as_bytes(),
    )?;
    // Write the public marker last. A partial preparation has no artifact.
    let public = json!({
        "format": "fod:fast-content-prepared-unsealed:v1",
        "journal": hex(&journal.encode()),
        "copiesCommitment": hex(&journal.copies),
        "items": pairs,
    });
    create_private_file(
        &out_dir.join("prepared-unsealed.json"),
        serde_json::to_string_pretty(&public)
            .map_err(|err| err.to_string())?
            .as_bytes(),
    )
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.len() != 6
        || args
            .chunks_exact(2)
            .any(|pair| !["--input", "--contract", "--out-dir"].contains(&pair[0].as_str()))
    {
        eprintln!("usage: fast-content-prepare --input witness.json --contract 0x... --out-dir NEW_DIRECTORY");
        std::process::exit(2);
    }
    let result = (|| {
        let input = PathBuf::from(flag(&args, "--input")?);
        let contract_bytes = unhex(&flag(&args, "--contract")?)?;
        let contract: [u8; 20] = contract_bytes.try_into().map_err(|_| "contract length")?;
        let out = PathBuf::from(flag(&args, "--out-dir")?);
        prepare(&input, contract, &out)
    })();
    if let Err(problem) = result {
        eprintln!("preparation failed: {problem}");
        std::process::exit(1);
    }
    println!("prepared unsealed content locally; no proof or transaction was created");
}
