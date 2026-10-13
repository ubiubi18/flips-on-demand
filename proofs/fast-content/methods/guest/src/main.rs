//! Pre-session guest: original flip bytes and authenticated buyer copies are
//! private inputs; only the fixed-width content credential is committed.

use fod_fast_content_core::{self as content, Input, Record};
use risc0_zkvm::guest::env;

fn bytes(maximum: usize) -> Vec<u8> {
    let len: u32 = env::read();
    assert!(len as usize <= maximum, "oversized witness item");
    let mut words = vec![0u32; (len as usize).div_ceil(4)];
    env::read_slice(&mut words);
    let mut out = Vec::with_capacity(words.len() * 4);
    for word in words {
        out.extend_from_slice(&word.to_le_bytes());
    }
    out.truncate(len as usize);
    out
}

fn main() {
    let version: u32 = env::read();
    assert_eq!(version, content::STATEMENT_VERSION, "statement version");
    let network: String = env::read();
    let contract: Vec<u8> = env::read();
    let author: Vec<u8> = env::read();
    let epoch: u16 = env::read();
    let consensus: String = env::read();
    let policy: String = env::read();
    let public_secret: Vec<u8> = env::read();
    let private_secret: Vec<u8> = env::read();
    let message: Vec<u8> = env::read();
    let blind: Vec<u8> = env::read();
    let count: u32 = env::read();
    assert!(count > 0 && count as usize <= content::MAX_FLIPS, "flip count");
    let mut records = Vec::with_capacity(count as usize);
    for _ in 0..count {
        let cid: Vec<u8> = env::read();
        assert!(!cid.is_empty() && cid.len() <= 128, "CID size");
        let original_blob = bytes(600 * 1024);
        let encrypted_copy = bytes(600 * 1024 + 45);
        records.push((cid, original_blob, encrypted_copy));
    }

    let contract: [u8; 20] = contract.try_into().expect("contract size");
    let author: [u8; 20] = author.try_into().expect("author size");
    let public_secret: [u8; 32] = public_secret.try_into().expect("public secret size");
    let private_secret: [u8; 32] = private_secret.try_into().expect("private secret size");
    let message: [u8; 33] = message.try_into().expect("message point size");
    let blind: [u8; 32] = blind.try_into().expect("blind size");
    let borrowed: Vec<Record<'_>> = records
        .iter()
        .map(|(cid, original_blob, encrypted_copy)| Record {
            original_cid: cid,
            original_blob,
            encrypted_copy,
        })
        .collect();
    let journal = content::verify(&Input {
        network: &network,
        contract: &contract,
        author: &author,
        epoch,
        consensus: &consensus,
        policy: &policy,
        public_secret: &public_secret,
        private_secret: &private_secret,
        message: &message,
        blind: &blind,
        records: &borrowed,
    })
    .expect("content witness refused")
    .encode();
    env::commit_slice(&journal);
}
