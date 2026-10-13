//! Produces one genuine RISC Zero Groth16 receipt for the delivery statement and
//! exports everything a verifier needs.
//!
//! With no arguments it proves the built-in synthetic vector: the run recorded
//! in `dist/delivery-v2.json` and replayed by the compatibility tests. With
//! `--order <path>` - or `--order -` to read stdin - it proves the order in that
//! file instead, so a seller can deliver against a funded order whose terms were
//! never baked into this program.
//!
//! An order file carries the prover's witness. Treat it like a key file: keep it
//! out of the repository, prefer `--order -` so nothing is written to disk, and
//! never reuse the checked-in example's values as real keys.
//!
//! `--check` validates an order file - hex lengths, the buyer key against the
//! buyer secret when both are given, and the derived ciphertext - and prints the
//! result without proving. It answers "is this order well formed" in seconds; it
//! is not evidence that a delivery statement holds, and it writes no artefact.
//!
//! `--no-witness` leaves the secret half of the order out of the artefact: the
//! epochs' private scalars, the canonical secret bytes, the ephemeral scalars,
//! and the buyer's secret. What remains is public - the terms, the journal, the
//! seal, the ciphertext, and the measurements - so a seller's proof can be kept
//! and shared without also writing the witness to disk.
//!
//! `--verify <path>` checks a stored receipt without proving anything: it
//! rebuilds the receipt from the artefact's journal and seal against the guest
//! id this build pins and verifies the Groth16 seal natively. That is the check
//! a buyer can run on what the registry published, so a delivery does not have
//! to be taken on the seller's or the chain's word.

use fod_delivery_core as delivery;
use methods::{DELIVERY_GUEST_ELF, DELIVERY_GUEST_ID};
use risc0_zkvm::sha::Digestible;
use risc0_zkvm::{
    default_prover, ExecutorEnv, Groth16Receipt, InnerReceipt, ProverOpts, Receipt, ReceiptClaim,
    VerifierContext,
};
use sha2::{Digest, Sha256};
use std::fs;
use std::io::Read;
use std::time::Instant;

mod journal;
use journal::{Journal, JOURNAL_BYTES, JOURNAL_VERSION};

const SYNTHETIC_NOTE: &str = "synthetic values for compatibility testing, not real keys";

const USAGE: &str = "\
usage: delivery-host [--order PATH|-] [--out PATH] [--check] [--no-witness]

  --order PATH   prove the order in PATH; - reads it from stdin.
                 Without this flag the built-in synthetic vector is proved.
  --out PATH     write the artefact here (default dist/delivery-v2.json).
  --check        validate the order and stop; no seal, no artefact.
  --no-witness   leave the order's secret fields out of the artefact.
  --verify PATH  verify the receipt in an artefact (default dist/delivery-v2.json)
                 without proving; exits non-zero when the seal does not hold.
  -h, --help     print this.
";

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

fn address(fill: u8) -> Vec<u8> {
    vec![fill; 20]
}

fn scalar(fill: u8) -> [u8; 32] {
    let mut out = [fill; 32];
    // Keep the value well below the secp256k1 group order.
    out[0] = 0x00;
    out[1] = 0x11;
    out
}

fn decode(name: &str, value: &str) -> Vec<u8> {
    assert!(
        value.len() % 2 == 0,
        "{name}: hex has an odd number of digits"
    );
    let mut out = Vec::with_capacity(value.len() / 2);
    for i in (0..value.len()).step_by(2) {
        let byte = u8::from_str_radix(&value[i..i + 2], 16)
            .unwrap_or_else(|_| panic!("{name}: not hexadecimal"));
        out.push(byte);
    }
    out
}

fn decode_len(name: &str, value: &str, len: usize) -> Vec<u8> {
    let raw = decode(name, value);
    assert_eq!(raw.len(), len, "{name}: expected {len} bytes");
    raw
}

fn decode_scalar(name: &str, value: &str) -> [u8; 32] {
    let raw = decode_len(name, value, 32);
    let mut out = [0u8; 32];
    out.copy_from_slice(&raw);
    out
}

/// One delivery order: the public terms the escrow re-checks against its own
/// funded order, plus the witness that proves knowledge of the committed
/// secrets.
///
/// `buyer_key` is the public key the order registered at funding time, and is
/// what a real seller supplies. `buyer_secret` is optional and only lets a test
/// decrypt the delivery afterwards; give it instead of `buyer_key` to have the
/// key derived. Exactly one of the two is required. `buyer_key`, `secrets` and
/// `ciphertext` are all filled in on the way out.
struct Order {
    note: String,
    contract: String,
    network: String,
    funded_at: u64,
    funding_root: String,
    offer_id: u64,
    order_nonce: u64,
    seller: String,
    buyer: String,
    buyer_key: Option<String>,
    epoch: u16,
    author: String,
    public_part_secret: String,
    private_part_secret: String,
    ephemeral: String,
    buyer_secret: Option<String>,
}

impl Order {
    fn from_json(value: &serde_json::Value) -> Order {
        fn text(value: &serde_json::Value, key: &str) -> String {
            value
                .get(key)
                .and_then(|v| v.as_str())
                .unwrap_or_else(|| panic!("order needs a string \"{key}\""))
                .to_string()
        }
        fn number(value: &serde_json::Value, key: &str) -> u64 {
            value
                .get(key)
                .and_then(|v| v.as_u64())
                .unwrap_or_else(|| panic!("order needs an integer \"{key}\""))
        }
        fn optional(value: &serde_json::Value, key: &str) -> Option<String> {
            value
                .get(key)
                .and_then(|v| v.as_str())
                .map(|v| v.to_string())
        }
        Order {
            note: text(value, "note"),
            contract: text(value, "contract"),
            network: text(value, "network"),
            funded_at: number(value, "funded_at"),
            funding_root: text(value, "funding_root"),
            offer_id: number(value, "offer_id"),
            order_nonce: number(value, "order_nonce"),
            seller: text(value, "seller"),
            buyer: text(value, "buyer"),
            buyer_key: optional(value, "buyer_key"),
            epoch: number(value, "epoch") as u16,
            author: text(value, "author"),
            public_part_secret: text(value, "public_part_secret"),
            private_part_secret: text(value, "private_part_secret"),
            ephemeral: text(value, "ephemeral"),
            buyer_secret: optional(value, "buyer_secret"),
        }
    }
}

/// The reproducible vector that `dist/delivery-v2.json` records. Fixed scalars, so
/// the artefact is deterministic and the compatibility test has something to
/// decrypt; none of it is a real key.
fn synthetic_order() -> Order {
    Order {
        note: SYNTHETIC_NOTE.to_string(),
        contract: hex(&address(0x11)),
        network: "idena".to_string(),
        funded_at: 1000,
        funding_root: hex(&[0x88; 32]),
        offer_id: 7,
        order_nonce: 1,
        seller: hex(&address(0x22)),
        buyer: hex(&address(0x33)),
        buyer_key: Some(hex(
            &delivery::public_key_of(&scalar(0x51)).expect("buyer key")
        )),
        epoch: 204,
        author: hex(&[0x44u8; 20]),
        public_part_secret: hex(&scalar(0x61)),
        private_part_secret: hex(&scalar(0x71)),
        ephemeral: hex(&scalar(0x81)),
        buyer_secret: Some(hex(&scalar(0x51))),
    }
}

fn parse_args() -> (Option<String>, String, bool, bool, Option<String>) {
    let mut order_path: Option<String> = None;
    let mut out_path = String::from("dist/delivery-v2.json");
    let mut check_only = false;
    let mut no_witness = false;
    let mut verify_path: Option<String> = None;
    let args: Vec<String> = std::env::args().skip(1).collect();
    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "-h" | "--help" => {
                print!("{USAGE}");
                std::process::exit(0);
            }
            "--order" => {
                i += 1;
                order_path = Some(
                    args.get(i)
                        .expect("--order needs a path, or - for stdin")
                        .clone(),
                );
            }
            "--out" => {
                i += 1;
                out_path = args.get(i).expect("--out needs a path").clone();
            }
            "--check" => check_only = true,
            "--no-witness" => no_witness = true,
            "--verify" => {
                // The path is optional so `--verify` alone checks the default
                // artefact, which is what a reader of `dist/delivery-v2.json` does.
                let next = args.get(i + 1).cloned();
                match next {
                    Some(path) if !path.starts_with("--") => {
                        i += 1;
                        verify_path = Some(path);
                    }
                    _ => verify_path = Some(String::from("dist/delivery-v2.json")),
                }
            }
            other => panic!("unknown argument {other}: run with --help for the flags"),
        }
        i += 1;
    }
    (order_path, out_path, check_only, no_witness, verify_path)
}

// A tiny hex reader for the artefact fields, so verifying needs no dependency
// the prover side does not already carry.
fn unhex(text: &str) -> Result<Vec<u8>, String> {
    let text = text.trim();
    if text.len() % 2 != 0 {
        return Err(format!("hex length {} is odd", text.len()));
    }
    (0..text.len())
        .step_by(2)
        .map(|index| {
            u8::from_str_radix(&text[index..index + 2], 16)
                .map_err(|err| format!("hex at {index}: {err}"))
        })
        .collect()
}

/// Check a stored receipt without proving: rebuild it from the public parts of
/// an artefact and verify the seal against the guest id this build pins. The
/// journal is what makes the rebuilt claim; the seal is what attests to it, so a
/// buyer who has both can check a delivery without the prover's help.
fn verify_artefact(path: &str) -> Result<(), String> {
    let raw = fs::read(path).map_err(|err| format!("read {path}: {err}"))?;
    let artefact: serde_json::Value =
        serde_json::from_slice(&raw).map_err(|err| format!("parse {path}: {err}"))?;
    let field = |name: &str| -> Result<Vec<u8>, String> {
        let text = artefact
            .get(name)
            .and_then(|value| value.as_str())
            .ok_or_else(|| format!("{path} carries no {name}"))?;
        unhex(text).map_err(|err| format!("{name}: {err}"))
    };
    let journal = field("journal")?;
    let seal = field("seal")?;
    if artefact.get("statement").and_then(|v| v.as_str()) != Some("fod:delivery:v2")
        || Journal::parse(&journal).is_none()
    {
        return Err("unsupported delivery statement".to_string());
    }

    let context = VerifierContext::default();
    let parameters = context
        .groth16_verifier_parameters
        .ok_or("this build carries no Groth16 verifier parameters")?;
    let claim = ReceiptClaim::ok(DELIVERY_GUEST_ID, journal.clone());
    let inner = InnerReceipt::Groth16(Groth16Receipt::new(
        seal.clone(),
        claim.into(),
        parameters.digest(),
    ));
    let receipt = Receipt::new(inner, journal.clone());
    receipt
        .verify(DELIVERY_GUEST_ID)
        .map_err(|err| format!("the seal does not verify: {err}"))?;
    let claim_digest = receipt
        .claim()
        .map_err(|err| format!("claim: {err}"))?
        .digest();

    let image_id: Vec<u8> = DELIVERY_GUEST_ID
        .iter()
        .flat_map(|word| word.to_le_bytes())
        .collect();
    println!(
        "verified  {} bytes of seal over {} bytes of journal",
        seal.len(),
        journal.len()
    );
    println!("image id  {}", hex(&image_id));
    println!("journal   sha256 {}", hex(&Sha256::digest(&journal)));
    println!("claim     {}", hex(claim_digest.as_bytes()));
    if let Some(stated) = artefact
        .get("claim_digest")
        .and_then(|value| value.as_str())
    {
        let expected = unhex(stated).map_err(|err| format!("claim_digest: {err}"))?;
        if expected != claim_digest.as_bytes() {
            return Err(String::from(
                "the artefact's claim_digest is not the one the seal attests",
            ));
        }
    }
    Ok(())
}

fn load_order(path: &Option<String>) -> Order {
    let text = match path.as_deref() {
        None => return synthetic_order(),
        Some("-") => {
            let mut text = String::new();
            std::io::stdin()
                .read_to_string(&mut text)
                .expect("read order from stdin");
            text
        }
        Some(path) => fs::read_to_string(path).unwrap_or_else(|e| panic!("read {path}: {e}")),
    };
    Order::from_json(&serde_json::from_str(&text).expect("parse order"))
}

fn main() {
    let (order_path, out_path, check_only, no_witness, verify_path) = parse_args();
    if let Some(path) = verify_path {
        if let Err(err) = verify_artefact(&path) {
            eprintln!("verification failed: {err}");
            std::process::exit(1);
        }
        return;
    }
    let order = load_order(&order_path);

    let contract = decode_len("contract", &order.contract, 20);
    assert!(!order.network.is_empty(), "network name");
    assert!(order.funded_at > 0, "funding height");
    let network = Sha256::digest(order.network.as_bytes()).to_vec();
    let funding_root = decode_len("funding_root", &order.funding_root, 32);
    let seller = decode_len("seller", &order.seller, 20);
    let buyer = decode_len("buyer", &order.buyer, 20);
    let author = decode_len("author", &order.author, 20);
    let public_part = decode_scalar("public_part_secret", &order.public_part_secret);
    let private_part = decode_scalar("private_part_secret", &order.private_part_secret);
    let ephemeral = decode_scalar("ephemeral", &order.ephemeral);

    // The order registers the buyer's public key; a test that also holds the
    // buyer's secret gets the key derived, and a stated key is checked against
    // that secret here rather than sixteen minutes into a proving run.
    let buyer_key = match (order.buyer_key.clone(), order.buyer_secret.clone()) {
        (Some(stated), Some(secret_hex)) => {
            let derived = delivery::public_key_of(&decode_scalar("buyer_secret", &secret_hex))
                .expect("buyer key");
            assert_eq!(
                stated,
                hex(&derived),
                "buyer_key does not match buyer_secret"
            );
            decode_len("buyer_key", &stated, 65)
        }
        (Some(stated), None) => decode_len("buyer_key", &stated, 65),
        (None, Some(secret_hex)) => {
            delivery::public_key_of(&decode_scalar("buyer_secret", &secret_hex)).expect("buyer key")
        }
        (None, None) => panic!("order needs buyer_key, or buyer_secret to derive it"),
    };

    let mut author_bytes = [0u8; 20];
    author_bytes.copy_from_slice(&author);
    let secrets = delivery::secrets_bytes(order.epoch, &author_bytes, &public_part, &private_part);
    let ciphertext =
        delivery::encrypt_to(&buyer_key, &secrets, &ephemeral).expect("delivery encryption");
    let expected_commitment = delivery::secrets_commitment(&secrets);
    let expected_ciphertext_hash: [u8; 32] = Sha256::digest(&ciphertext).into();

    if check_only {
        println!(
            "order ok: offer {} nonce {}, buyer {}",
            order.offer_id,
            order.order_nonce,
            hex(&buyer)
        );
        println!("secrets commitment: {}", hex(&expected_commitment));
        println!(
            "ciphertext: {} bytes, sha256 {}",
            ciphertext.len(),
            hex(&expected_ciphertext_hash)
        );
        println!("no seal proved: run without --check to produce the receipt");
        return;
    }

    let env = ExecutorEnv::builder()
        .write(&JOURNAL_VERSION)
        .unwrap()
        .write(&contract)
        .unwrap()
        .write(&order.offer_id)
        .unwrap()
        .write(&order.order_nonce)
        .unwrap()
        .write(&seller)
        .unwrap()
        .write(&buyer)
        .unwrap()
        .write(&buyer_key)
        .unwrap()
        .write(&secrets)
        .unwrap()
        .write(&ephemeral.to_vec())
        .unwrap()
        .write(&network)
        .unwrap()
        .write(&order.funded_at)
        .unwrap()
        .write(&funding_root)
        .unwrap()
        .build()
        .unwrap();

    let started = Instant::now();
    let prover = default_prover();
    let prove_info = prover
        .prove_with_opts(env, DELIVERY_GUEST_ELF, &ProverOpts::groth16())
        .expect("proving failed");
    let prove_seconds = started.elapsed().as_secs_f64();
    let receipt = prove_info.receipt;

    // Verify the receipt here as well: a receipt nobody checked is not evidence.
    receipt
        .verify(DELIVERY_GUEST_ID)
        .expect("receipt verification");

    let journal_bytes = receipt.journal.bytes.clone();

    let seal = match &receipt.inner {
        InnerReceipt::Groth16(inner) => inner.seal.clone(),
        other => panic!("expected a Groth16 receipt, got: {other:?}"),
    };
    let claim_digest = receipt.claim().unwrap().digest();

    let cycles = prove_info.stats.total_cycles;
    let segments = prove_info.stats.segments;

    // What the seller submits to `fulfill`: offer id (8) || journal (281) ||
    // seal (256) || ciphertext.
    let mut fulfill_argument = order.offer_id.to_be_bytes().to_vec();
    fulfill_argument.extend_from_slice(&journal_bytes);
    fulfill_argument.extend_from_slice(&seal);
    fulfill_argument.extend_from_slice(&ciphertext);

    let mut order_json = serde_json::Map::new();
    order_json.insert("note".to_string(), serde_json::json!(order.note));
    order_json.insert("contract".to_string(), serde_json::json!(order.contract));
    order_json.insert("network".to_string(), serde_json::json!(order.network));
    order_json.insert("funded_at".to_string(), serde_json::json!(order.funded_at));
    order_json.insert(
        "funding_root".to_string(),
        serde_json::json!(order.funding_root),
    );
    order_json.insert("offer_id".to_string(), serde_json::json!(order.offer_id));
    order_json.insert(
        "order_nonce".to_string(),
        serde_json::json!(order.order_nonce),
    );
    order_json.insert("seller".to_string(), serde_json::json!(order.seller));
    order_json.insert("buyer".to_string(), serde_json::json!(order.buyer));
    order_json.insert("buyer_key".to_string(), serde_json::json!(hex(&buyer_key)));
    order_json.insert("epoch".to_string(), serde_json::json!(order.epoch));
    order_json.insert("author".to_string(), serde_json::json!(order.author));
    order_json.insert(
        "ciphertext".to_string(),
        serde_json::json!(hex(&ciphertext)),
    );
    if !no_witness {
        order_json.insert(
            "public_part_secret".to_string(),
            serde_json::json!(order.public_part_secret),
        );
        order_json.insert(
            "private_part_secret".to_string(),
            serde_json::json!(order.private_part_secret),
        );
        order_json.insert("ephemeral".to_string(), serde_json::json!(order.ephemeral));
        order_json.insert(
            "buyer_secret".to_string(),
            serde_json::json!(order.buyer_secret),
        );
        order_json.insert("secrets".to_string(), serde_json::json!(hex(&secrets)));
    }

    let out = serde_json::json!({
        "statement": "fod:delivery:v2",
        "journal": hex(&journal_bytes),
        "ciphertext": hex(&ciphertext),
        "ciphertext_bytes": ciphertext.len(),
        "seal": hex(&seal),
        "seal_bytes": seal.len(),
        "claim_digest": hex(claim_digest.as_bytes()),
        "image_id": hex(&DELIVERY_GUEST_ID.iter().flat_map(|w| w.to_le_bytes()).collect::<Vec<_>>()),
        "fulfill_argument": hex(&fulfill_argument),
        "fulfill_argument_bytes": fulfill_argument.len(),
        "prove_seconds": prove_seconds,
        "total_cycles": cycles,
        "segments": segments,
        "order": serde_json::Value::Object(order_json),
    });

    if let Some(parent) = std::path::Path::new(&out_path).parent() {
        fs::create_dir_all(parent).expect("output directory");
    }
    fs::write(&out_path, serde_json::to_vec_pretty(&out).unwrap())
        .unwrap_or_else(|e| panic!("write {out_path}: {e}"));

    // The receipt is on disk now, so a mismatch below costs an inspection, not
    // another proving run.
    assert_eq!(
        journal_bytes.len(),
        JOURNAL_BYTES,
        "unexpected journal size"
    );
    let journal = Journal::parse(&journal_bytes).expect("journal layout");
    assert_eq!(journal.version, JOURNAL_VERSION);
    assert_eq!(journal.network, &network[..]);
    assert_eq!(journal.funded_at, order.funded_at);
    assert_eq!(journal.funding_root, &funding_root[..]);
    assert_eq!(journal.contract, &contract[..], "contract in journal");
    assert_eq!(journal.offer_id, order.offer_id, "offer id in journal");
    assert_eq!(
        journal.order_nonce, order.order_nonce,
        "order nonce in journal"
    );
    assert_eq!(journal.seller, &seller[..], "seller in journal");
    assert_eq!(journal.buyer, &buyer[..], "buyer in journal");
    assert_eq!(journal.buyer_key, &buyer_key[..], "buyer key in journal");
    assert_eq!(
        journal.commitment,
        &expected_commitment[..],
        "secrets commitment in journal"
    );
    assert_eq!(
        journal.ciphertext_hash,
        &expected_ciphertext_hash[..],
        "ciphertext hash in journal"
    );

    println!(
        "delivery proof: offer {} nonce {}, {} bytes ciphertext, {} bytes seal, {} cycles in {} segments, {:.1}s",
        order.offer_id,
        order.order_nonce,
        ciphertext.len(),
        seal.len(),
        cycles,
        segments,
        prove_seconds
    );
    println!("artefact: {out_path}");
    println!("journal: {}", hex(&journal_bytes));
}

#[cfg(test)]
mod tests {
    use super::*;

    // `--verify` is the buyer's check, so it is exercised the way a buyer
    // exercises it: against the shipped artefact, and against tampering. The
    // artefact is a real Groth16 receipt produced by this host, so the test is
    // skipped only when it has never been proved.
    #[test]
    fn verifies_the_shipped_receipt_and_refuses_tampering() {
        let path =
            std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../dist/delivery-v2.json");
        if !path.exists() {
            eprintln!("no artefact at {}; run the host first", path.display());
            return;
        }
        let raw = fs::read_to_string(&path).expect("read the artefact");
        let artefact: serde_json::Value = serde_json::from_str(&raw).expect("parse the artefact");
        verify_artefact(path.to_str().unwrap()).expect("the shipped receipt verifies");

        let dir = std::env::temp_dir().join(format!("fod-delivery-verify-{}", std::process::id()));
        fs::create_dir_all(&dir).expect("temp dir");

        let mut journal = unhex(artefact["journal"].as_str().expect("journal")).expect("hex");
        journal[0] ^= 0x01;
        let mut tampered_journal = artefact.clone();
        tampered_journal["journal"] = serde_json::Value::String(hex(&journal));
        let journal_path = dir.join("tampered-journal.json");
        fs::write(
            &journal_path,
            serde_json::to_vec(&tampered_journal).unwrap(),
        )
        .expect("write");
        assert!(
            verify_artefact(journal_path.to_str().unwrap()).is_err(),
            "a tampered journal verified"
        );

        let mut seal = unhex(artefact["seal"].as_str().expect("seal")).expect("hex");
        seal[0] ^= 0x01;
        let mut tampered_seal = artefact.clone();
        tampered_seal["seal"] = serde_json::Value::String(hex(&seal));
        let seal_path = dir.join("tampered-seal.json");
        fs::write(&seal_path, serde_json::to_vec(&tampered_seal).unwrap()).expect("write");
        assert!(
            verify_artefact(seal_path.to_str().unwrap()).is_err(),
            "a tampered seal verified"
        );
        let _ = fs::remove_dir_all(&dir);
    }
}
