//! Turn a canonical funded-order export and the seller's private pre-session
//! witness into a small, buyer-bound settlement call. No large proof runs here.

use fod_fast_content_core::{Journal, JOURNAL_BYTES};
use fod_fast_delivery_core::{compressed_public_key, order_context, verify, SellerKey};
use serde::Deserialize;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::fs::{self, File, OpenOptions};
use std::io::{Read, Write};
use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};
use std::path::Path;

#[derive(Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
struct OrderInput {
    format: String,
    version: u32,
    network: String,
    contract: String,
    bundle: String,
    journal: String,
    order_id: u64,
    nonce: u64,
    buyer: String,
    delivery_key: String,
    author: String,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
struct SellerPrivate {
    format: String,
    message: String,
    blind: String,
    author: String,
    epoch: u16,
    contract: String,
    journal: String,
}

fn read_private(path: &Path, maximum: usize) -> Result<Vec<u8>, String> {
    let meta = fs::symlink_metadata(path).map_err(|_| "cannot inspect private input")?;
    if !meta.is_file()
        || meta.file_type().is_symlink()
        || meta.len() > maximum as u64
        || meta.permissions().mode() & 0o077 != 0
    {
        return Err("private input must be a bounded owner-only regular file".into());
    }
    let file = File::open(path).map_err(|_| "cannot open private input")?;
    let opened = file.metadata().map_err(|_| "cannot inspect opened input")?;
    if !opened.is_file() || opened.permissions().mode() & 0o077 != 0 {
        return Err("opened input is not owner-only and regular".into());
    }
    let mut bytes = Vec::new();
    file.take((maximum + 1) as u64)
        .read_to_end(&mut bytes)
        .map_err(|_| "cannot read private input")?;
    if bytes.len() > maximum {
        return Err("private input exceeds size limit".into());
    }
    Ok(bytes)
}

fn fixed<const N: usize>(text: &str) -> Result<[u8; N], String> {
    let text = text.strip_prefix("0x").unwrap_or(text);
    if text.len() != N * 2 || !text.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return Err("invalid fixed-width hex field".into());
    }
    let mut out = [0u8; N];
    for (index, pair) in text.as_bytes().chunks_exact(2).enumerate() {
        out[index] = u8::from_str_radix(std::str::from_utf8(pair).expect("ASCII hex"), 16)
            .map_err(|_| "invalid fixed-width hex field")?;
    }
    Ok(out)
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}

fn prepare(order: &OrderInput, private: &SellerPrivate) -> Result<Value, String> {
    if order.format != "fod.fast-delivery-order"
        || order.version != 1
        || order.network != "idena"
        || order.order_id == 0
        || private.format != "fod:fast-content-seller-private:v1"
    {
        return Err("invalid fast delivery input format".into());
    }
    let contract = fixed::<20>(&order.contract)?;
    let bundle = fixed::<32>(&order.bundle)?;
    let author = fixed::<20>(&order.author)?;
    let buyer = fixed::<20>(&order.buyer)?;
    let journal_bytes = fixed::<JOURNAL_BYTES>(&order.journal)?;
    let journal = Journal::decode(&journal_bytes).ok_or("invalid content journal")?;
    let network: [u8; 32] = Sha256::digest(b"idena").into();
    if journal.network != network
        || journal.contract != contract
        || journal.author != author
        || Sha256::digest(journal_bytes).as_slice() != bundle
        || fixed::<20>(&private.author)? != author
        || fixed::<20>(&private.contract)? != contract
        || private.epoch != journal.epoch
        || fixed::<JOURNAL_BYTES>(&private.journal)? != journal_bytes
    {
        return Err("seller witness differs from registered order and journal".into());
    }
    let seller = SellerKey::from_witness(&fixed(&private.message)?, &fixed(&private.blind)?)
        .ok_or("invalid seller point witness")?;
    if seller.commitment() != journal.point_commitment {
        return Err("seller point witness differs from registered commitment".into());
    }
    let buyer_key = fixed::<65>(&order.delivery_key)?;
    let buyer_point = compressed_public_key(&buyer_key).ok_or("invalid buyer delivery key")?;
    let context = order_context(
        &network,
        &contract,
        &bundle,
        order.order_id,
        order.nonce,
        &buyer,
        &buyer_point,
    );
    let delivery = seller
        .deliver(&buyer_point, &context)
        .ok_or("cannot create fast point delivery")?;
    if !verify(&journal.point_commitment, &buyer_point, &context, &delivery) {
        return Err("local fast delivery verification failed".into());
    }
    let proof = delivery.encode();
    let mut calldata = Vec::with_capacity(8 + proof.len());
    calldata.extend_from_slice(&order.order_id.to_be_bytes());
    calldata.extend_from_slice(&proof);
    Ok(json!({
        "format": "fod.prepared-fulfill-fast", "version": 1,
        "operation": "fulfillFast", "network": "idena",
        "contract": hex(&contract), "amount": "0", "calldata": hex(&calldata),
        "bundle": hex(&bundle), "offerId": order.order_id, "orderNonce": order.nonce,
        "seller": hex(&author), "buyer": hex(&buyer), "deliveryKey": hex(&buyer_key),
        "proof": hex(&proof),
    }))
}

fn run(args: &[String]) -> Result<(), String> {
    if args.len() != 6 {
        return Err(
            "usage: prepare_fast_delivery --order FILE --seller PRIVATE_FILE --out NEW_FILE".into(),
        );
    }
    let mut order = None;
    let mut seller = None;
    let mut out = None;
    for pair in args.chunks_exact(2) {
        match pair[0].as_str() {
            "--order" if order.is_none() => order = Some(pair[1].as_str()),
            "--seller" if seller.is_none() => seller = Some(pair[1].as_str()),
            "--out" if out.is_none() => out = Some(pair[1].as_str()),
            _ => return Err("unknown or repeated flag".into()),
        }
    }
    let order: OrderInput = serde_json::from_slice(&read_private(
        Path::new(order.ok_or("missing order")?),
        4096,
    )?)
    .map_err(|_| "invalid fast order JSON")?;
    let seller: SellerPrivate = serde_json::from_slice(&read_private(
        Path::new(seller.ok_or("missing seller witness")?),
        4096,
    )?)
    .map_err(|_| "invalid seller witness JSON")?;
    let action = prepare(&order, &seller)?;
    let bytes = serde_json::to_vec_pretty(&action).map_err(|_| "cannot encode fast delivery")?;
    let mut file = OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .open(out.ok_or("missing output")?)
        .map_err(|_| "cannot create new private prepared file")?;
    file.write_all(&bytes)
        .map_err(|_| "cannot write private prepared file")?;
    file.sync_all()
        .map_err(|_| "cannot sync private prepared file")?;
    println!("buyer-bound fast delivery prepared; no transaction sent");
    Ok(())
}

fn main() {
    if let Err(reason) = run(&std::env::args().skip(1).collect::<Vec<_>>()) {
        eprintln!("fast delivery refused: {reason}");
        std::process::exit(1);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use k256::elliptic_curve::sec1::ToEncodedPoint;

    #[test]
    fn seller_point_proof_is_bound_to_the_funded_order() {
        let seller = SellerKey::generate();
        let (message, blind) = seller.witness_material();
        let contract = [0x11; 20];
        let author = [0x33; 20];
        let journal = Journal {
            network: Sha256::digest(b"idena").into(),
            contract,
            author,
            epoch: 230,
            count: 1,
            consensus: [0; 32],
            policy: [0; 32],
            original_inventory: [1; 32],
            copies: [2; 32],
            point_commitment: seller.commitment(),
            original_keys: [3; 32],
        };
        let journal_bytes = journal.encode();
        let buyer_key = k256::SecretKey::from_slice(&[1; 32]).unwrap();
        let input = OrderInput {
            format: "fod.fast-delivery-order".into(),
            version: 1,
            network: "idena".into(),
            contract: hex(&contract),
            bundle: hex(&Sha256::digest(journal_bytes)),
            journal: hex(&journal_bytes),
            order_id: 7,
            nonce: 3,
            buyer: hex(&[0x22; 20]),
            delivery_key: hex(buyer_key.public_key().to_encoded_point(false).as_bytes()),
            author: hex(&author),
        };
        let private = SellerPrivate {
            format: "fod:fast-content-seller-private:v1".into(),
            message: hex(&message),
            blind: hex(&blind),
            author: hex(&author),
            epoch: 230,
            contract: hex(&contract),
            journal: hex(&journal_bytes),
        };
        let action = prepare(&input, &private).unwrap();
        assert_eq!(action["operation"], "fulfillFast");
        assert_eq!(action["calldata"].as_str().unwrap().len(), (8 + 196) * 2);
        let mut moved = input;
        moved.bundle = hex(&[0; 32]);
        assert!(prepare(&moved, &private).is_err());
    }
}
