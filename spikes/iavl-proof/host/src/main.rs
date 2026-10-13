//! Runs the IAVL proof guest over a proof bundle and reports the cost.
//!
//! Usage: `cargo run --release -p iavl-proof-host -- <bundle.json> [repeat]`
//!
//! The host first verifies every proof natively with the same core the guest
//! uses, so a cycle number is never reported for a guest that disagrees with the
//! Go verifier. The optional repeat multiplies the bundle's entries, which is how
//! a shard's worth of proofs is priced.

use fod_iavl_proof_core::{verify_value, PathItem};
use risc0_zkvm::{default_executor, ExecutorEnv};
use std::vec::Vec;

struct PathHex {
    height: i64,
    size: i64,
    version: i64,
    left: Vec<u8>,
    right: Vec<u8>,
}

struct ProofHex {
    key: Vec<u8>,
    value: Vec<u8>,
    version: i64,
    path: Vec<PathHex>,
}

fn decode(bytes: &serde_json::Value, field: &str) -> Vec<u8> {
    hex::decode(bytes[field].as_str().unwrap_or("")).expect(field)
}

fn load(path: &str) -> (Vec<u8>, Vec<ProofHex>) {
    let raw = std::fs::read_to_string(path).expect("read bundle");
    let json: serde_json::Value = serde_json::from_str(&raw).expect("parse bundle");
    let root = hex::decode(json["rootHex"].as_str().expect("root")).expect("root hex");
    let proofs = json["proofs"]
        .as_array()
        .expect("proofs")
        .iter()
        .map(|proof| ProofHex {
            key: decode(proof, "keyHex"),
            value: decode(proof, "valueHex"),
            version: proof["version"].as_i64().expect("version"),
            path: proof["path"]
                .as_array()
                .expect("path")
                .iter()
                .map(|item| PathHex {
                    height: item["height"].as_i64().expect("height"),
                    size: item["size"].as_i64().expect("size"),
                    version: item["version"].as_i64().expect("version"),
                    left: decode(item, "leftHex"),
                    right: decode(item, "rightHex"),
                })
                .collect(),
        })
        .collect();
    (root, proofs)
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let bundle_path = args
        .get(1)
        .cloned()
        .unwrap_or_else(|| "core/fixtures/bundle.json".to_string());
    let repeat: usize = args.get(2).and_then(|value| value.parse().ok()).unwrap_or(1);
    let (root, proofs) = load(&bundle_path);
    let entries = proofs.len() * repeat;

    let mut native_ok = 0usize;
    for _ in 0..repeat {
        for proof in &proofs {
            let path: Vec<PathItem> = proof
                .path
                .iter()
                .map(|item| PathItem {
                    height: item.height,
                    size: item.size,
                    version: item.version,
                    left: &item.left,
                    right: &item.right,
                })
                .collect();
            if verify_value(&root, proof.version, &proof.key, &proof.value, &path) {
                native_ok += 1;
            }
        }
    }
    println!(
        "bundle       {} proofs={} repeat={} entries={} root={}",
        bundle_path,
        proofs.len(),
        repeat,
        entries,
        hex::encode(&root)
    );
    println!("native       {native_ok}/{entries} proofs folded to the root");
    if native_ok != entries {
        eprintln!("the native verifier disagrees with the fixture; refusing to measure the guest");
        std::process::exit(1);
    }

    let packed = pack(&root, &proofs, repeat, entries);
    let mut builder = ExecutorEnv::builder();
    builder.write(&(packed.len() as u32)).expect("packed length");
    builder.write_slice(&packed);
    let env = builder.build().expect("env");

    let started = std::time::Instant::now();
    let session = default_executor()
        .execute(env, methods::IAVL_PROOF_GUEST_ELF)
        .expect("execute");
    let seconds = started.elapsed().as_secs_f64();
    let cycles = session.cycles();
    let segments = session.segments.len();
    let journal = session.journal.bytes;
    if journal.len() != 40 || journal[..32] != root[..] {
        eprintln!("the guest journal does not carry the root it was given");
        std::process::exit(1);
    }
    let count = u32::from_be_bytes(journal[32..36].try_into().unwrap());
    let verified = u32::from_be_bytes(journal[36..40].try_into().unwrap());
    println!("guest        {verified}/{count} proofs folded to the root");
    println!(
        "cycles       {} in {} segments, {:.1} s, {:.0} cycles per proof",
        cycles,
        segments,
        seconds,
        cycles as f64 / entries as f64
    );
    if verified != count {
        std::process::exit(1);
    }
}

fn push_bytes(out: &mut Vec<u8>, bytes: &[u8]) {
    out.extend_from_slice(&(bytes.len() as u32).to_le_bytes());
    out.extend_from_slice(bytes);
}

/// The guest's input: root, count, then per proof the version, key, value, and
/// each path item. Little-endian fields and `u32` lengths, no padding.
fn pack(root: &[u8], proofs: &[ProofHex], repeat: usize, entries: usize) -> Vec<u8> {
    let mut out = Vec::new();
    out.extend_from_slice(root);
    out.extend_from_slice(&(entries as u32).to_le_bytes());
    for _ in 0..repeat {
        for proof in proofs {
            out.extend_from_slice(&proof.version.to_le_bytes());
            push_bytes(&mut out, &proof.key);
            push_bytes(&mut out, &proof.value);
            out.extend_from_slice(&(proof.path.len() as u32).to_le_bytes());
            for item in &proof.path {
                out.extend_from_slice(&item.height.to_le_bytes());
                out.extend_from_slice(&item.size.to_le_bytes());
                out.extend_from_slice(&item.version.to_le_bytes());
                push_bytes(&mut out, &item.left);
                push_bytes(&mut out, &item.right);
            }
        }
    }
    out
}
