//! Replays a public fast registration certificate without seller keys or a
//! marketplace service. Header roots must come from the verifier's own node.

use fast_content_methods::FAST_CONTENT_GUEST_ID;
use fod_fast_content_core::{Journal, JOURNAL_BYTES, MAX_FLIPS};
use fod_fast_session_core::{verify_with_roots, MAX_WIRE_BYTES};
use risc0_zkvm::sha::Digestible;
use risc0_zkvm::{
    Groth16Receipt, Groth16ReceiptVerifierParameters, InnerReceipt, MaybePruned, Receipt,
    ReceiptClaim,
};
use serde_json::Value;
use sha2::{Digest, Sha256};
use std::fs;
use std::path::Path;

const SEAL_BYTES: usize = 256;
const HEADER_BYTES: usize = JOURNAL_BYTES + SEAL_BYTES + 2;
const MAX_CERTIFICATE_BYTES: usize = HEADER_BYTES + MAX_WIRE_BYTES - 1;
const CONSENSUS: &str = "idena-go@9bc57040337fd19f45ba65323939cac8789b9efd";
const POLICY: &str = "fod:eligibility:v1";

#[derive(Clone, Copy)]
struct Root {
    height: u64,
    state: [u8; 32],
}

fn read_bounded(path: &Path, max: usize) -> Result<Vec<u8>, String> {
    let meta = fs::symlink_metadata(path).map_err(|_| "cannot inspect input file")?;
    if !meta.is_file() || meta.file_type().is_symlink() || meta.len() > max as u64 {
        return Err("input must be a bounded regular file".into());
    }
    let bytes = fs::read(path).map_err(|_| "cannot read input file")?;
    if bytes.len() > max {
        return Err("input exceeds size limit".into());
    }
    Ok(bytes)
}

fn decode_hex<const N: usize>(text: &str) -> Result<[u8; N], String> {
    let text = text.strip_prefix("0x").unwrap_or(text);
    if text.len() != N * 2 || !text.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return Err("invalid hex field".into());
    }
    let mut out = [0u8; N];
    for (index, pair) in text.as_bytes().chunks_exact(2).enumerate() {
        out[index] = u8::from_str_radix(std::str::from_utf8(pair).unwrap(), 16)
            .map_err(|_| "invalid hex field")?;
    }
    Ok(out)
}

fn parse_root(value: &Value) -> Result<Root, String> {
    Ok(Root {
        height: value["height"].as_u64().ok_or("missing header height")?,
        state: decode_hex(
            value["stateRoot"]
                .as_str()
                .ok_or("missing header state root")?,
        )?,
    })
}

fn roots_from_file(path: &Path) -> Result<[Root; 2], String> {
    let bytes = read_bounded(path, 4096)?;
    let value: Value = serde_json::from_slice(&bytes).map_err(|_| "invalid header JSON")?;
    let inventory = parse_root(&value["inventory"])?;
    let verdict = parse_root(&value["verdict"])?;
    if inventory.height == verdict.height {
        return Err("header heights must differ".into());
    }
    Ok([inventory, verdict])
}

fn verify_certificate(
    certificate: &[u8],
    bundle: &[u8; 32],
    contract: &[u8; 20],
    roots: [Root; 2],
) -> Result<u32, String> {
    if certificate.len() <= HEADER_BYTES || certificate.len() > MAX_CERTIFICATE_BYTES {
        return Err("certificate length outside bounds".into());
    }
    let journal_bytes = &certificate[..JOURNAL_BYTES];
    let seal = &certificate[JOURNAL_BYTES..JOURNAL_BYTES + SEAL_BYTES];
    let wire_len = u16::from_be_bytes([
        certificate[JOURNAL_BYTES + SEAL_BYTES],
        certificate[JOURNAL_BYTES + SEAL_BYTES + 1],
    ]) as usize;
    if wire_len == 0 || certificate.len() != HEADER_BYTES + wire_len {
        return Err("invalid session witness length".into());
    }
    let journal = Journal::decode(journal_bytes).ok_or("invalid content journal")?;
    if journal.contract != *contract
        || journal.count == 0
        || usize::from(journal.count) > MAX_FLIPS
        || journal.network.as_slice() != Sha256::digest(b"idena").as_slice()
        || journal.consensus.as_slice() != Sha256::digest(CONSENSUS).as_slice()
        || journal.policy.as_slice() != Sha256::digest(POLICY).as_slice()
        || Sha256::digest(journal_bytes).as_slice() != bundle
    {
        return Err("journal disagrees with the expected contract or bundle".into());
    }
    let wire = &certificate[HEADER_BYTES..];
    let verdict = verify_with_roots(
        wire,
        &journal.author,
        journal.epoch,
        journal.count,
        &journal.original_inventory,
        |height| {
            roots
                .iter()
                .find(|root| root.height == height)
                .map(|root| root.state)
        },
    )
    .map_err(|reason| format!("invalid public session: {reason:?}"))?;
    if verdict.flips != u32::from(journal.count) {
        return Err("session covered count disagrees with content proof".into());
    }
    let params = Groth16ReceiptVerifierParameters::default();
    let claim = ReceiptClaim::ok(FAST_CONTENT_GUEST_ID, journal_bytes.to_vec());
    let inner = Groth16Receipt::new(seal.to_vec(), MaybePruned::Value(claim), params.digest());
    let receipt = Receipt::new(InnerReceipt::Groth16(inner), journal_bytes.to_vec());
    receipt
        .verify(FAST_CONTENT_GUEST_ID)
        .map_err(|_| "invalid flip-content proof")?;
    Ok(verdict.flags)
}

fn run(args: &[String]) -> Result<(), String> {
    if args.len() != 8 {
        return Err("usage: verify_certificate --certificate FILE --headers FILE --bundle HEX --contract HEX".into());
    }
    let mut certificate = None;
    let mut headers = None;
    let mut bundle = None;
    let mut contract = None;
    for pair in args.chunks_exact(2) {
        match pair[0].as_str() {
            "--certificate" if certificate.is_none() => certificate = Some(pair[1].as_str()),
            "--headers" if headers.is_none() => headers = Some(pair[1].as_str()),
            "--bundle" if bundle.is_none() => bundle = Some(pair[1].as_str()),
            "--contract" if contract.is_none() => contract = Some(pair[1].as_str()),
            _ => return Err("unknown or repeated flag".into()),
        }
    }
    let bytes = read_bounded(
        Path::new(certificate.ok_or("missing certificate")?),
        MAX_CERTIFICATE_BYTES,
    )?;
    let roots = roots_from_file(Path::new(headers.ok_or("missing headers")?))?;
    let bundle = decode_hex(bundle.ok_or("missing bundle")?)?;
    let contract = decode_hex(contract.ok_or("missing contract")?)?;
    let flags = verify_certificate(&bytes, &bundle, &contract, roots)?;
    println!("flip content and canonical-root session witness verified (flags {flags})");
    Ok(())
}

fn main() {
    if let Err(reason) = run(&std::env::args().skip(1).collect::<Vec<_>>()) {
        eprintln!("certificate refused: {reason}");
        std::process::exit(1);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn truncated_and_mismatched_certificates_refuse() {
        let roots = [
            Root {
                height: 1,
                state: [1; 32],
            },
            Root {
                height: 2,
                state: [2; 32],
            },
        ];
        assert!(verify_certificate(&[0; HEADER_BYTES], &[0; 32], &[0; 20], roots).is_err());
        let mut fake = vec![0u8; HEADER_BYTES + 1];
        fake[JOURNAL_BYTES + SEAL_BYTES + 1] = 2;
        assert!(verify_certificate(&fake, &[0; 32], &[0; 20], roots).is_err());
    }
}
