//! Host for the registration proof.
//!
//! With no flags it executes the guest and compares the journal it commits with
//! the frozen claim vector, which is the byte-for-byte agreement between this
//! guest and the CLI's claim builder. `--negative` mutates the input one way at a
//! time and requires the guest to refuse it. With `--prove` it produces a Groth16
//! receipt over the same input and writes the artefact the seller would submit.
//!
//! The input comes from `--input FILE`, the JSON the CLI writes for a bundle it
//! assembled; `--dump-input FILE` writes the vector-built input in that same
//! shape. Without either, the frozen vectors are used, so the whole program still
//! runs on a bundle the CLI and the cores already agree about.
//!
//! `--contract` and `--author` override the two fields of the frozen bundle that
//! a local chain cannot match: the registry address (a deploy lands where its
//! code hash and nonce put it, never on the vector's placeholder) and the author
//! (the caller has to be the journal's author, and nobody holds the vector's
//! key). Everything else stays the frozen construction. `--out` keeps a lab
//! artefact from overwriting the tracked one.
//!
//! The vectors carry one blob built by the node's own primitives and an epoch
//! whose seven participants graded its flip.

use risc0_zkvm::sha::Digestible;
use risc0_zkvm::{default_executor, default_prover, ExecutorEnv, ProverOpts};
use serde_json::Value;
use std::path::PathBuf;

const FLIP_INDEX: u32 = 7;
const CANDIDATES: usize = 7;
/// Each participant also answers two flips nobody covers, so a participant that
/// reports the covered flip is not dropped as a spammer for reporting all of its
/// own flips.
const OTHER_FLIPS: u32 = 8;

fn vector(name: &str) -> Value {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join(format!("../vectors/{name}"));
    let raw = std::fs::read_to_string(&path).unwrap_or_else(|err| panic!("read {path:?}: {err}"));
    serde_json::from_str(&raw).expect("parse the vector")
}

/// Registry and author a lab run binds instead of the frozen ones.
#[derive(Default, Clone)]
struct Lab {
    contract: Option<String>,
    author: Option<String>,
}

fn argument(args: &[String], name: &str) -> Option<String> {
    let at = args.iter().position(|arg| arg == name)?;
    args.get(at + 1)
        .map(|value| value.trim_start_matches("0x").to_lowercase())
}

fn decode20(value: &str, name: &str) -> [u8; 20] {
    let raw = hex(value);
    let mut out = [0u8; 20];
    assert_eq!(raw.len(), 20, "{name} must be a 20-byte address");
    out.copy_from_slice(&raw);
    out
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

/// A mutation the negative checks apply, one at a time.
#[derive(Clone, Copy)]
enum Mutation {
    /// One byte of the blob changes, so it no longer hashes to its CID.
    TamperedBlob,
    /// The manifest names a CID the inventory does not carry.
    ExtraManifestCid,
    /// The committee reported the covered flip, so the policy fails.
    ReportedFlip,
    /// The secrets are not the ones the blob was encrypted to.
    WrongSecret,
    /// One byte of an anchor record's value hash changes, so the bundle no
    /// longer folds to the anchor the manifest commits.
    TamperedAnchor,
}

/// One flip of the manifest and the blob that carries it.
#[derive(Clone)]
struct InventoryEntry {
    flip: u32,
    cid: String,
    blob: String,
}

/// One participant of the epoch: the flips it was assigned, the answers it gave,
/// and the grades it reported, all in assignment order.
#[derive(Clone)]
struct CandidateEntry {
    assignment: Vec<u32>,
    answers: Vec<u8>,
    grades: Vec<u8>,
}

/// The input of one registration, as the CLI writes it and the guest reads it.
///
/// The host used to build this only from the frozen vectors, so the prover could
/// only ever prove the example bundle. The same values now travel as a file, and
/// `--input` is how a bundle the CLI assembled from an epoch reaches the guest.
#[derive(Clone)]
struct RegistrationInput {
    contract: String,
    network: String,
    author: String,
    epoch: u16,
    validation_anchor: String,
    consensus_version: String,
    eligibility_policy: String,
    cids: Vec<String>,
    inventory: Vec<InventoryEntry>,
    public_part_secret: String,
    private_part_secret: String,
    candidates: Vec<CandidateEntry>,
    covered: Vec<u32>,
    proof_anchor: Option<AnchorBundle>,
}

/// One path item of an anchor record, owned so the host can pack it.
#[derive(Clone)]
struct AnchorPathItem {
    height: i64,
    size: i64,
    version: i64,
    left: Vec<u8>,
    right: Vec<u8>,
}

/// One identity record with the proof that places it under the anchor root.
#[derive(Clone)]
struct AnchorRecord {
    key: Vec<u8>,
    value_hash: Vec<u8>,
    version: i64,
    path: Vec<AnchorPathItem>,
}

/// The bundle `tools/fod state-proof -proof-bundle` writes and
/// `prepare-offer -proof-anchor` carries.
#[derive(Clone)]
struct AnchorBundle {
    tree: String,
    height: i64,
    root: Vec<u8>,
    records: Vec<AnchorRecord>,
}

impl AnchorBundle {
    fn from_json(value: &Value) -> Result<AnchorBundle, String> {
        let root = hex(value["rootHex"]
            .as_str()
            .ok_or("the proof anchor names no root")?);
        if root.len() != 32 {
            return Err("the proof anchor's root is not a 32-byte digest".to_string());
        }
        let records = value["proofs"]
            .as_array()
            .ok_or("the proof anchor carries no proofs")?
            .iter()
            .map(|proof| AnchorRecord {
                key: hex(proof["keyHex"].as_str().unwrap_or("")),
                value_hash: hex(proof["valueHashHex"].as_str().unwrap_or("")),
                version: proof["version"].as_i64().unwrap_or(0),
                path: proof["path"]
                    .as_array()
                    .map(|items| {
                        items
                            .iter()
                            .map(|item| AnchorPathItem {
                                height: item["height"].as_i64().unwrap_or(0),
                                size: item["size"].as_i64().unwrap_or(0),
                                version: item["version"].as_i64().unwrap_or(0),
                                left: hex(item["leftHex"].as_str().unwrap_or("")),
                                right: hex(item["rightHex"].as_str().unwrap_or("")),
                            })
                            .collect()
                    })
                    .unwrap_or_default(),
            })
            .collect::<Vec<_>>();
        if records.is_empty() {
            return Err("the proof anchor carries no records".to_string());
        }
        Ok(AnchorBundle {
            tree: value["tree"].as_str().unwrap_or("state").to_string(),
            height: value["height"].as_i64().unwrap_or(0),
            root,
            records,
        })
    }

    fn to_json(&self) -> Value {
        serde_json::json!({
            "tree": self.tree,
            "height": self.height,
            "rootHex": hex_of(&self.root),
            "proofs": self.records.iter().map(|record| serde_json::json!({
                "keyHex": hex_of(&record.key),
                "valueHashHex": hex_of(&record.value_hash),
                "version": record.version,
                "path": record.path.iter().map(|item| serde_json::json!({
                    "height": item.height,
                    "size": item.size,
                    "version": item.version,
                    "leftHex": hex_of(&item.left),
                    "rightHex": hex_of(&item.right),
                })).collect::<Vec<_>>(),
            })).collect::<Vec<_>>(),
        })
    }

    /// The packed bytes the guest reads: the anchor, a count, then the records.
    fn packed(&self) -> Vec<u8> {
        let records: Vec<fod_anchor_core::OwnedRecord> = self
            .records
            .iter()
            .map(|record| fod_anchor_core::OwnedRecord {
                version: record.version,
                key: record.key.clone(),
                value_hash: record.value_hash.clone(),
                path: record
                    .path
                    .iter()
                    .map(|item| fod_anchor_core::OwnedPathItem {
                        height: item.height,
                        size: item.size,
                        version: item.version,
                        left: item.left.clone(),
                        right: item.right.clone(),
                    })
                    .collect(),
            })
            .collect();
        fod_anchor_core::pack_bundle(&self.root, &records)
    }
}

impl RegistrationInput {
    /// The input the frozen vectors describe, with the lab's overrides.
    fn from_vectors(lab: &Lab) -> RegistrationInput {
        let blob = vector("blob-v1.json");
        let manifest = vector("manifest-v1.json")["manifest"].clone();
        let claim = vector("claim-v1.json");
        let contract = lab
            .contract
            .clone()
            .unwrap_or_else(|| claim["contract"].as_str().expect("contract").to_string());
        let author = lab
            .author
            .clone()
            .unwrap_or_else(|| manifest["author"].as_str().expect("author").to_string());
        let cids: Vec<String> = manifest["cids"]
            .as_array()
            .expect("cids")
            .iter()
            .map(|cid| cid.as_str().expect("cid").to_string())
            .collect();
        let assignment: Vec<u32> = vec![FLIP_INDEX, OTHER_FLIPS, OTHER_FLIPS + 1];
        RegistrationInput {
            contract,
            network: manifest["network"].as_str().expect("network").to_string(),
            author,
            epoch: manifest["epoch"].as_u64().expect("epoch") as u16,
            validation_anchor: manifest["validationAnchor"]
                .as_str()
                .expect("anchor")
                .to_string(),
            consensus_version: manifest["consensusVersion"]
                .as_str()
                .expect("consensus")
                .to_string(),
            eligibility_policy: manifest["eligibilityPolicy"]
                .as_str()
                .expect("policy")
                .to_string(),
            inventory: vec![InventoryEntry {
                flip: FLIP_INDEX,
                cid: cids[0].clone(),
                blob: blob["blob"].as_str().expect("blob").to_string(),
            }],
            cids,
            public_part_secret: blob["publicPartSecret"]
                .as_str()
                .expect("public secret")
                .to_string(),
            private_part_secret: blob["privatePartSecret"]
                .as_str()
                .expect("private secret")
                .to_string(),
            candidates: vec![
                CandidateEntry {
                    assignment: assignment.clone(),
                    answers: vec![1u8; assignment.len()],
                    grades: vec![2u8; assignment.len()],
                };
                CANDIDATES
            ],
            covered: vec![FLIP_INDEX],
            proof_anchor: Some(
                AnchorBundle::from_json(&vector("anchor-v1.json")).expect("the anchor vector"),
            ),
        }
    }

    /// Apply one mutation, for the negative runs.
    fn mutate(&mut self, mutation: Mutation) {
        match mutation {
            Mutation::TamperedBlob => {
                let blob = &mut self.inventory[0].blob;
                let mut bytes = hex(blob);
                let at = bytes.len() - 40;
                bytes[at] ^= 0x01;
                *blob = hex_of(&bytes);
            }
            Mutation::ExtraManifestCid => {
                self.cids.push(
                    "bafkreiaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa".to_string(),
                );
            }
            Mutation::ReportedFlip => {
                // Four of seven report the covered flip and grade the others;
                // four of nine assignments is under the drop threshold, so the
                // reports stand.
                for candidate in self.candidates.iter_mut().take(4) {
                    candidate.grades[0] = 1;
                }
            }
            Mutation::WrongSecret => {
                self.public_part_secret = hex_of(&[0x11u8; 32]);
            }
            Mutation::TamperedAnchor => {
                let bundle = self
                    .proof_anchor
                    .as_mut()
                    .expect("checked: the input carries a proof anchor");
                if let Some(byte) = bundle.records[0].value_hash.first_mut() {
                    *byte ^= 0x01;
                }
            }
        }
    }

    fn from_json(value: &Value) -> RegistrationInput {
        let text = |key: &str| -> String {
            value
                .get(key)
                .and_then(|item| item.as_str())
                .unwrap_or_else(|| panic!("the input needs a string \"{key}\""))
                .to_string()
        };
        let number = |key: &str| -> u64 {
            value
                .get(key)
                .and_then(|item| item.as_u64())
                .unwrap_or_else(|| panic!("the input needs an integer \"{key}\""))
        };
        let strings = |key: &str| -> Vec<String> {
            value
                .get(key)
                .and_then(|item| item.as_array())
                .unwrap_or_else(|| panic!("the input needs a list \"{key}\""))
                .iter()
                .map(|item| {
                    item.as_str()
                        .unwrap_or_else(|| panic!("\"{key}\" holds a non-string"))
                        .to_string()
                })
                .collect()
        };
        let numbers = |item: &Value, key: &str| -> Vec<u64> {
            item.get(key)
                .and_then(|entry| entry.as_array())
                .unwrap_or_else(|| panic!("\"{key}\" needs a list of integers"))
                .iter()
                .map(|entry| {
                    entry
                        .as_u64()
                        .unwrap_or_else(|| panic!("\"{key}\" holds a non-integer"))
                })
                .collect()
        };

        let inventory = value["inventory"]
            .as_array()
            .expect("the input needs an \"inventory\" list")
            .iter()
            .map(|entry| InventoryEntry {
                flip: entry["flip"].as_u64().expect("an inventory flip id") as u32,
                cid: entry["cid"].as_str().expect("an inventory cid").to_string(),
                blob: entry["blob"]
                    .as_str()
                    .expect("an inventory blob")
                    .to_string(),
            })
            .collect();
        let candidates = value["candidates"]
            .as_array()
            .expect("the input needs a \"candidates\" list")
            .iter()
            .map(|entry| CandidateEntry {
                assignment: numbers(entry, "assignment")
                    .into_iter()
                    .map(|value| value as u32)
                    .collect(),
                answers: numbers(entry, "answers")
                    .into_iter()
                    .map(|value| value as u8)
                    .collect(),
                grades: numbers(entry, "grades")
                    .into_iter()
                    .map(|value| value as u8)
                    .collect(),
            })
            .collect();

        RegistrationInput {
            contract: text("contract"),
            network: text("network"),
            author: text("author"),
            epoch: number("epoch") as u16,
            validation_anchor: text("validationAnchor"),
            consensus_version: text("consensusVersion"),
            eligibility_policy: text("eligibilityPolicy"),
            cids: strings("cids"),
            inventory,
            public_part_secret: text("publicPartSecret"),
            private_part_secret: text("privatePartSecret"),
            candidates,
            covered: numbers(value, "covered")
                .into_iter()
                .map(|value| value as u32)
                .collect(),
            proof_anchor: value
                .get("proofAnchor")
                .filter(|anchor| !anchor.is_null())
                .map(|anchor| AnchorBundle::from_json(anchor).expect("the proof anchor")),
        }
    }

    fn to_json(&self) -> Value {
        serde_json::json!({
            "statement": "fod:registration-input:v1",
            "note": "the values the guest reads, in the order the host writes them",
            "contract": self.contract,
            "network": self.network,
            "author": self.author,
            "epoch": self.epoch,
            "validationAnchor": self.validation_anchor,
            "consensusVersion": self.consensus_version,
            "eligibilityPolicy": self.eligibility_policy,
            "cids": self.cids,
            "inventory": self.inventory.iter().map(|entry| serde_json::json!({
                "flip": entry.flip,
                "cid": entry.cid,
                "blob": entry.blob,
            })).collect::<Vec<_>>(),
            "publicPartSecret": self.public_part_secret,
            "privatePartSecret": self.private_part_secret,
            "candidates": self.candidates.iter().map(|entry| serde_json::json!({
                "assignment": entry.assignment,
                "answers": entry.answers,
                "grades": entry.grades,
            })).collect::<Vec<_>>(),
            "covered": self.covered,
            "proofAnchor": self.proof_anchor.as_ref().map(|bundle| bundle.to_json()),
        })
    }

    /// Refuse an input that cannot be what the guest expects, before a proof
    /// spends fifteen minutes on it.
    fn check(&self) -> Result<(), String> {
        if decode20(&self.contract, "contract").len() != 20 {
            return Err("the contract is not a 20-byte address".to_string());
        }
        if decode20(&self.author, "author").len() != 20 {
            return Err("the author is not a 20-byte address".to_string());
        }
        if hex(&self.validation_anchor).len() != 32 {
            return Err("the validation anchor is not 32 bytes".to_string());
        }
        if hex(&self.public_part_secret).len() != 32 || hex(&self.private_part_secret).len() != 32 {
            return Err("the part secrets are not 32 bytes each".to_string());
        }
        if self.cids.is_empty() {
            return Err("the manifest covers no CIDs".to_string());
        }
        if self.inventory.is_empty() {
            return Err("the inventory is empty".to_string());
        }
        if self.covered.is_empty() {
            return Err("the bundle covers no flips".to_string());
        }
        for (index, entry) in self.candidates.iter().enumerate() {
            if entry.answers.len() != entry.assignment.len()
                || entry.grades.len() != entry.assignment.len()
            {
                return Err(format!(
                    "candidate {index} has {} assignments, {} answers, and {} grades",
                    entry.assignment.len(),
                    entry.answers.len(),
                    entry.grades.len()
                ));
            }
        }
        let bundle = self.proof_anchor.as_ref().ok_or(
            "the input carries no proof anchor: a bundle is sellable only with its session binding",
        )?;
        if bundle.root != hex(&self.validation_anchor) {
            return Err(
                "the proof anchor's root is not the manifest's validation anchor".to_string(),
            );
        }
        Ok(())
    }

    /// The same values as one word stream, in the order the guest reads them.
    ///
    /// Keep this in step with `methods/guest/src/main.rs`.
    fn words(&self) -> Vec<u32> {
        let mut out = Vec::new();
        push(&mut out, &1u32);
        // The guest reads these as byte vectors, so push a vector rather than a
        // fixed-size array: the zkVM's serde encodes the two differently.
        push(&mut out, &decode20(&self.contract, "contract").to_vec());
        push(&mut out, &self.network);
        push(&mut out, &decode20(&self.author, "author").to_vec());
        push(&mut out, &self.epoch);
        push(&mut out, &hex(&self.validation_anchor));
        push(&mut out, &self.consensus_version);
        push(&mut out, &self.eligibility_policy);
        push(&mut out, &self.cids);
        let inventory: Vec<(u32, String, Vec<u8>)> = self
            .inventory
            .iter()
            .map(|entry| (entry.flip, entry.cid.clone(), hex(&entry.blob)))
            .collect();
        push(&mut out, &inventory);
        push(&mut out, &hex(&self.public_part_secret));
        push(&mut out, &hex(&self.private_part_secret));
        let candidates: Vec<(Vec<u32>, Vec<u8>, Vec<u8>)> = self
            .candidates
            .iter()
            .map(|entry| {
                (
                    entry.assignment.clone(),
                    entry.answers.clone(),
                    entry.grades.clone(),
                )
            })
            .collect();
        push(&mut out, &candidates);
        push(&mut out, &self.covered);
        // The session binding, packed the one way `fod-anchor-core` reads.
        push(
            &mut out,
            &self
                .proof_anchor
                .as_ref()
                .expect("checked: the input carries a proof anchor")
                .packed(),
        );
        out
    }
}

fn push<T: serde::Serialize>(words: &mut Vec<u32>, value: &T) {
    words.extend(risc0_zkvm::serde::to_vec(value).expect("serialize"));
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let prove = args.iter().any(|arg| arg == "--prove");
    let negative = args.iter().any(|arg| arg == "--negative");
    let lab = Lab {
        contract: argument(&args, "--contract"),
        author: argument(&args, "--author"),
    };
    let out_path = argument(&args, "--out")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("../dist/registration.json"));
    let dump_input = argument(&args, "--dump-input").map(PathBuf::from);
    let input_path = argument(&args, "--input");

    let input = match input_path {
        Some(path) => {
            let raw =
                std::fs::read_to_string(&path).unwrap_or_else(|err| panic!("read {path}: {err}"));
            RegistrationInput::from_json(&serde_json::from_str(&raw).expect("parse the input"))
        }
        None => RegistrationInput::from_vectors(&lab),
    };
    if let Err(problem) = input.check() {
        eprintln!("the input is refused: {problem}");
        std::process::exit(1);
    }

    // The input as a file, which is what the CLI writes and what a reader can
    // inspect: the words the guest reads are the host's business.
    if let Some(path) = dump_input {
        std::fs::write(
            &path,
            format!(
                "{}\n",
                serde_json::to_string_pretty(&input.to_json()).expect("encode")
            ),
        )
        .expect("write the input");
        println!(
            "input dump    {} cids, {} inventory entries, {} candidates -> {}",
            input.cids.len(),
            input.inventory.len(),
            input.candidates.len(),
            path.display()
        );
        return;
    }

    if negative {
        let cases = [
            ("tampered blob", Mutation::TamperedBlob),
            ("extra manifest cid", Mutation::ExtraManifestCid),
            ("reported flip", Mutation::ReportedFlip),
            ("wrong secret", Mutation::WrongSecret),
            ("tampered anchor", Mutation::TamperedAnchor),
        ];
        let mut failures = 0;
        for (name, mutation) in cases {
            let mut case = input.clone();
            case.mutate(mutation);
            let env = ExecutorEnv::builder()
                .write_slice(&case.words())
                .build()
                .expect("env");
            match default_executor().execute(env, methods::REGISTRATION_GUEST_ELF) {
                // A guest that refused its input ends on a halt that is not
                // code 0; only that counts as acceptance.
                Ok(session) => match session.exit_code {
                    risc0_zkvm::ExitCode::Halted(0) => {
                        println!(
                            "{name}: the guest accepted it (journal {} bytes)",
                            session.journal.bytes.len()
                        );
                        failures += 1;
                    }
                    other => println!("{name}: refused ({other:?})"),
                },
                Err(err) => println!("{name}: refused ({err})"),
            }
        }
        if failures > 0 {
            std::process::exit(1);
        }
        println!("every mutated input was refused");
        return;
    }

    let words = input.words();
    let env = ExecutorEnv::builder()
        .write_slice(&words)
        .build()
        .expect("env");

    if !prove {
        let session = default_executor()
            .execute(env, methods::REGISTRATION_GUEST_ELF)
            .expect("execute");
        let journal = session.journal.bytes;
        // With overrides, the journal differs from the vector exactly in the
        // contract and author fields, so the expectation is patched the same way.
        let mut expected = hex(vector("claim-v1.json")["journalHex"]
            .as_str()
            .expect("journal"));
        if let Some(contract) = &lab.contract {
            expected[4..24].copy_from_slice(&decode20(contract, "--contract"));
        }
        if let Some(author) = &lab.author {
            expected[24..44].copy_from_slice(&decode20(author, "--author"));
        }
        println!("journal       {} bytes", journal.len());
        println!(
            "journal hex   {}",
            journal
                .iter()
                .map(|b| format!("{b:02x}"))
                .collect::<String>()
        );
        if lab.contract.is_some() || lab.author.is_some() {
            // The claim domain covers the contract, so the two commitments in
            // the middle of the journal are recomputed for the lab registry by
            // design. Everything the overrides do not touch still has to match:
            // header, contract, author, epoch, covered flips, and the two label
            // digests.
            let header_matches = journal[..50] == expected[..50];
            let labels_match = journal[114..] == expected[114..];
            if !header_matches || !labels_match {
                println!("journal does not match the lab claim (header {header_matches}, labels {labels_match})");
                std::process::exit(1);
            }
            println!(
                "check         header and labels match the frozen claim; commitments recomputed (manifest {}, key {})",
                journal[50..82].iter().map(|b| format!("{b:02x}")).collect::<String>(),
                journal[82..114].iter().map(|b| format!("{b:02x}")).collect::<String>(),
            );
        } else if journal != expected {
            println!("journal does not match the frozen claim vector");
            std::process::exit(1);
        } else {
            println!("check         the guest commits exactly the frozen claim vector");
        }
        return;
    }

    let started = std::time::Instant::now();
    let prover = default_prover();
    let prove_info = prover
        .prove_with_opts(env, methods::REGISTRATION_GUEST_ELF, &ProverOpts::groth16())
        .expect("prove");
    let seconds = started.elapsed().as_secs_f64();
    let stats = &prove_info.stats;
    let receipt = prove_info.receipt;

    // Verify the receipt here as well: a receipt nobody checked is not
    // evidence, and this check is what pins the run to the guest id the
    // contract has to pin too.
    receipt
        .verify(methods::REGISTRATION_GUEST_ID)
        .expect("receipt verification");

    let journal = receipt.journal.bytes.clone();
    let claim_digest = receipt.claim().expect("receipt claim").digest();
    let seal = match receipt.inner.groth16() {
        Ok(groth16) => groth16.seal.to_vec(),
        Err(err) => panic!("no groth16 seal: {err}"),
    };

    println!("proving       {seconds:.1} s");
    println!(
        "cycles        {} in {} segments",
        stats.total_cycles, stats.segments
    );
    println!("journal       {} bytes", journal.len());
    println!("seal          {} bytes", seal.len());

    let path = match out_path.is_absolute() {
        true => out_path.clone(),
        false => std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join(out_path.clone()),
    };
    std::fs::create_dir_all(path.parent().expect("parent")).expect("dist directory");
    let artefact = serde_json::json!({
        "statement": "fod:registration:v1",
        "journal": hex_of(&journal),
        "seal": hex_of(&seal),
        "claim_digest": hex_of(claim_digest.as_bytes()),
        "image_id": hex_of(
            &methods::REGISTRATION_GUEST_ID
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
