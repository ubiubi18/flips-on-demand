//! Open copies retained before payment using only an included on-chain point
//! delivery and the buyer's local key. The input is exported by `fod receive`.

use fod_fast_content_core::{
    recover_prefetched, Journal, PrefetchedRecord, JOURNAL_BYTES, MAX_FLIPS,
};
use fod_fast_delivery_core::{Delivery, PointBytes, DELIVERY_BYTES};
use fod_registration_core::blob;
use k256::elliptic_curve::sec1::ToEncodedPoint;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::fs::{self, DirBuilder, File, OpenOptions};
use std::io::{Read, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt, PermissionsExt};
use std::path::Path;

const MAX_TRANSCRIPT_BYTES: usize = 48 * 1024 * 1024;
const MAX_COPY_BYTES: usize = blob::MAX_FLIP_BYTES + 45;

#[derive(Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
struct RetainedCopy {
    original_cid: String,
    copy_cid: String,
    hex: String,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
struct RecoveryInput {
    format: String,
    version: u32,
    contract: String,
    bundle: String,
    journal: String,
    order_id: u64,
    nonce: u64,
    buyer: String,
    delivery_key: String,
    delivery: String,
    copies: Vec<RetainedCopy>,
}

fn read_bounded(path: &Path, maximum: usize, owner_only: bool) -> Result<Vec<u8>, String> {
    let meta = fs::symlink_metadata(path).map_err(|_| "cannot inspect input file")?;
    if !meta.is_file()
        || meta.file_type().is_symlink()
        || meta.len() > maximum as u64
        || (owner_only && meta.permissions().mode() & 0o077 != 0)
    {
        return Err("input must be a bounded owner-only regular file".into());
    }
    let file = File::open(path).map_err(|_| "cannot open input file")?;
    let opened = file.metadata().map_err(|_| "cannot inspect opened input")?;
    if !opened.is_file() || (owner_only && opened.permissions().mode() & 0o077 != 0) {
        return Err("opened input must be owner-only and regular".into());
    }
    let mut bytes = Vec::new();
    file.take((maximum + 1) as u64)
        .read_to_end(&mut bytes)
        .map_err(|_| "cannot read input")?;
    if bytes.len() > maximum {
        return Err("input exceeds size limit".into());
    }
    Ok(bytes)
}

fn unhex(text: &str, maximum: usize) -> Result<Vec<u8>, String> {
    let text = text.strip_prefix("0x").unwrap_or(text);
    if text.len() > maximum * 2
        || text.len() % 2 != 0
        || !text.bytes().all(|byte| byte.is_ascii_hexdigit())
    {
        return Err("invalid hex field".into());
    }
    text.as_bytes()
        .chunks_exact(2)
        .map(|pair| {
            u8::from_str_radix(std::str::from_utf8(pair).expect("ASCII hex"), 16)
                .map_err(|_| "invalid hex field".to_owned())
        })
        .collect()
}

fn fixed<const N: usize>(text: &str) -> Result<[u8; N], String> {
    unhex(text, N)?
        .try_into()
        .map_err(|_| "wrong hex field length".into())
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}

fn cid(text: &str) -> Result<&[u8], String> {
    if text.is_empty()
        || text.len() > 128
        || !text
            .bytes()
            .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit())
    {
        return Err("invalid CID spelling".into());
    }
    Ok(text.as_bytes())
}

fn recover(input: &RecoveryInput, secret: &[u8; 32]) -> Result<Vec<(Vec<u8>, Vec<u8>)>, String> {
    if input.format != "fod.fast-recovery-input"
        || input.version != 1
        || input.order_id == 0
        || input.copies.is_empty()
        || input.copies.len() > MAX_FLIPS
    {
        return Err("invalid recovery transcript".into());
    }
    let contract = fixed::<20>(&input.contract)?;
    let bundle = fixed::<32>(&input.bundle)?;
    let buyer = fixed::<20>(&input.buyer)?;
    let journal_bytes = fixed::<JOURNAL_BYTES>(&input.journal)?;
    let journal = Journal::decode(&journal_bytes).ok_or("invalid fast journal")?;
    let network: [u8; 32] = Sha256::digest(b"idena").into();
    if journal.network != network
        || journal.contract != contract
        || Sha256::digest(journal_bytes).as_slice() != bundle
    {
        return Err("journal differs from the expected network, contract, or bundle".into());
    }
    let secret_key = k256::SecretKey::from_slice(secret).map_err(|_| "invalid delivery key")?;
    let public = secret_key.public_key();
    let compressed: PointBytes = public
        .to_encoded_point(true)
        .as_bytes()
        .try_into()
        .map_err(|_| "invalid delivery key")?;
    if public.to_encoded_point(false).as_bytes() != fixed::<65>(&input.delivery_key)? {
        return Err("delivery key differs from the funded order".into());
    }
    let delivery = Delivery::decode(&fixed::<DELIVERY_BYTES>(&input.delivery)?)
        .ok_or("invalid included delivery")?;
    let mut copies = Vec::with_capacity(input.copies.len());
    for item in &input.copies {
        cid(&item.original_cid)?;
        cid(&item.copy_cid)?;
        let body = unhex(&item.hex, MAX_COPY_BYTES)?;
        if body.is_empty() {
            return Err("empty encrypted copy".into());
        }
        copies.push(body);
    }
    let records: Vec<PrefetchedRecord<'_>> = input
        .copies
        .iter()
        .zip(&copies)
        .map(|(item, body)| PrefetchedRecord {
            original_cid: item.original_cid.as_bytes(),
            copy_cid: item.copy_cid.as_bytes(),
            encrypted_copy: body,
        })
        .collect();
    recover_prefetched(
        &journal,
        &network,
        &contract,
        &bundle,
        &records,
        secret,
        &compressed,
        input.order_id,
        input.nonce,
        &buyer,
        &delivery,
    )
    .map_err(|reason| format!("fast delivery or copy refused: {reason:?}"))
}

fn write_parts(
    directory: &Path,
    input: &RecoveryInput,
    parts: &[(Vec<u8>, Vec<u8>)],
) -> Result<(), String> {
    if parts.len() != input.copies.len() {
        return Err("wrong opened copy count".into());
    }
    DirBuilder::new()
        .mode(0o700)
        .create(directory)
        .map_err(|_| "cannot create new private output directory")?;
    for (index, (public, private)) in parts.iter().enumerate() {
        for (name, bytes) in [("public", public), ("private", private)] {
            let path = directory.join(format!(
                "{:02}-{}.{}",
                index, input.copies[index].original_cid, name
            ));
            let mut file = OpenOptions::new()
                .write(true)
                .create_new(true)
                .mode(0o600)
                .open(path)
                .map_err(|_| "cannot create private output file")?;
            file.write_all(bytes)
                .map_err(|_| "cannot write private output file")?;
        }
    }
    Ok(())
}

fn run(args: &[String]) -> Result<(), String> {
    if args.len() != 6 {
        return Err(
            "usage: recover_fast --input FILE --delivery-key FILE --out-dir NEW_DIR".into(),
        );
    }
    let mut input_path = None;
    let mut key_path = None;
    let mut out_dir = None;
    for pair in args.chunks_exact(2) {
        match pair[0].as_str() {
            "--input" if input_path.is_none() => input_path = Some(pair[1].as_str()),
            "--delivery-key" if key_path.is_none() => key_path = Some(pair[1].as_str()),
            "--out-dir" if out_dir.is_none() => out_dir = Some(pair[1].as_str()),
            _ => return Err("unknown or repeated flag".into()),
        }
    }
    let transcript = read_bounded(
        Path::new(input_path.ok_or("missing input")?),
        MAX_TRANSCRIPT_BYTES,
        true,
    )?;
    let input: RecoveryInput =
        serde_json::from_slice(&transcript).map_err(|_| "invalid recovery JSON")?;
    let key_bytes = read_bounded(
        Path::new(key_path.ok_or("missing delivery key")?),
        128,
        true,
    )?;
    let key_text = std::str::from_utf8(&key_bytes)
        .map_err(|_| "invalid delivery key encoding")?
        .trim();
    let secret = fixed::<32>(key_text)?;
    let parts = recover(&input, &secret)?;
    write_parts(
        Path::new(out_dir.ok_or("missing output directory")?),
        &input,
        &parts,
    )?;
    println!(
        "opened {} verified flip copies into a private directory",
        parts.len()
    );
    Ok(())
}

fn main() {
    if let Err(reason) = run(&std::env::args().skip(1).collect::<Vec<_>>()) {
        eprintln!("fast recovery refused: {reason}");
        std::process::exit(1);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use fod_fast_content_core::{self as content, Input, Record};
    use fod_fast_delivery_core::{
        content::{seal_copy, CopyContext},
        order_context, SellerKey,
    };

    #[test]
    fn recovery_refuses_unbound_or_overlong_inputs() {
        assert!(unhex("0xzz", 32).is_err());
        assert!(cid("../secrets").is_err());
        let input = RecoveryInput {
            format: "wrong".into(),
            version: 1,
            contract: String::new(),
            bundle: String::new(),
            journal: String::new(),
            order_id: 1,
            nonce: 0,
            buyer: String::new(),
            delivery_key: String::new(),
            delivery: String::new(),
            copies: Vec::new(),
        };
        assert!(recover(&input, &[1; 32]).is_err());
    }

    #[test]
    fn included_delivery_opens_every_prefetched_copy() {
        let fixture: serde_json::Value = serde_json::from_str(include_str!(
            "../../../../registration/vectors/blob-flip-v1.json"
        ))
        .unwrap();
        let field = |name: &str| fixture[name].as_str().unwrap();
        let author: [u8; 20] = fixed(field("author")).unwrap();
        let public_secret: [u8; 32] = fixed(field("publicPartSecret")).unwrap();
        let private_secret: [u8; 32] = fixed(field("privatePartSecret")).unwrap();
        let original_cid = field("blobCid").as_bytes();
        let original_blob = unhex(field("blob"), blob::MAX_FLIP_BYTES).unwrap();
        let contract = [0x11; 20];
        let seller = SellerKey::generate();
        let (message, blind) = seller.witness_material();
        let network: [u8; 32] = Sha256::digest(b"idena").into();
        let (public, private) =
            blob::open_flip_blob(&original_blob, &public_secret, &private_secret).unwrap();
        let copy = seal_copy(
            &seller.content_key(),
            &CopyContext {
                network: &network,
                contract: &contract,
                author: &author,
                epoch: 204,
                index: 0,
                original_cid,
            },
            &public,
            &private,
        )
        .unwrap();
        let records = [Record {
            original_cid,
            original_blob: &original_blob,
            encrypted_copy: &copy,
        }];
        let journal = content::verify(&Input {
            network: "idena",
            contract: &contract,
            author: &author,
            epoch: 204,
            consensus: "idena-go@9bc57040337fd19f45ba65323939cac8789b9efd",
            policy: "fod:eligibility:v1",
            public_secret: &public_secret,
            private_secret: &private_secret,
            message: &message,
            blind: &blind,
            records: &records,
        })
        .unwrap();
        let journal_bytes = journal.encode();
        let bundle: [u8; 32] = Sha256::digest(journal_bytes).into();
        let buyer_secret = [1u8; 32];
        let buyer_key = k256::SecretKey::from_slice(&buyer_secret).unwrap();
        let buyer_point: PointBytes = buyer_key
            .public_key()
            .to_encoded_point(true)
            .as_bytes()
            .try_into()
            .unwrap();
        let buyer = [0x22; 20];
        let context = order_context(&network, &contract, &bundle, 7, 3, &buyer, &buyer_point);
        let delivery = seller.deliver(&buyer_point, &context).unwrap();
        let transcript = RecoveryInput {
            format: "fod.fast-recovery-input".into(),
            version: 1,
            contract: hex(&contract),
            bundle: hex(&bundle),
            journal: hex(&journal_bytes),
            order_id: 7,
            nonce: 3,
            buyer: hex(&buyer),
            delivery_key: hex(buyer_key.public_key().to_encoded_point(false).as_bytes()),
            delivery: hex(&delivery.encode()),
            copies: vec![RetainedCopy {
                original_cid: field("blobCid").into(),
                copy_cid: blob::blob_cid_multibase(&copy).unwrap(),
                hex: hex(&copy),
            }],
        };
        assert_eq!(
            recover(&transcript, &buyer_secret).unwrap(),
            vec![(public, private)]
        );
        let mut wrong = transcript;
        wrong.nonce += 1;
        assert!(recover(&wrong, &buyer_secret).is_err());
    }
}
