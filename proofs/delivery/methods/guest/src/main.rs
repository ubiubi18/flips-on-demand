//! Delivery proof guest for Flips on Demand.
//!
//! Private inputs: the epoch secrets a bundle sells and the ephemeral scalar
//! the seller used. Public inputs: the trade context the contract will re-check.
//!
//! The guest re-encrypts the secrets to the buyer's delivery key under the
//! pinned construction (delivery ECIES v1), then commits a journal holding the
//! trade context, the commitment to the secrets, and the hash of the ciphertext
//! it just produced. Nothing secret reaches the journal.

use fod_delivery_core as delivery;
use risc0_zkvm::guest::env;
use sha2::{Digest, Sha256};

/// Journal version, so a future statement can be told apart from this one.
const JOURNAL_VERSION: u32 = 2;
const ADDRESS_BYTES: usize = 20;

fn main() {
    let version: u32 = env::read();
    let contract: Vec<u8> = env::read();
    let offer_id: u64 = env::read();
    let order_nonce: u64 = env::read();
    let seller: Vec<u8> = env::read();
    let buyer: Vec<u8> = env::read();
    let buyer_key: Vec<u8> = env::read();
    let secrets: Vec<u8> = env::read();
    let ephemeral: Vec<u8> = env::read();
    let network: Vec<u8> = env::read();
    let funded_at: u64 = env::read();
    let funding_root: Vec<u8> = env::read();
    assert_eq!(network.len(), 32, "network digest size");
    assert_eq!(funding_root.len(), 32, "funding state root size");
    assert!(funded_at > 0, "funding height");

    assert_eq!(version, JOURNAL_VERSION, "unsupported statement version");
    assert_eq!(contract.len(), ADDRESS_BYTES, "contract address size");
    assert_eq!(seller.len(), ADDRESS_BYTES, "seller address size");
    assert_eq!(buyer.len(), ADDRESS_BYTES, "buyer address size");
    assert_eq!(
        buyer_key.len(),
        delivery::PUBLIC_KEY_BYTES,
        "delivery key size"
    );
    assert_eq!(
        ephemeral.len(),
        delivery::EPHEMERAL_BYTES,
        "ephemeral scalar size"
    );

    // The ciphertext is the whole point: re-encrypt the secrets to the buyer.
    let ciphertext = delivery::encrypt_to(&buyer_key, &secrets, &ephemeral)
        .expect("delivery encryption refused the inputs");

    let commitment = delivery::secrets_commitment(&secrets);
    let ciphertext_hash = Sha256::digest(&ciphertext);

    let mut journal = Vec::with_capacity(281);
    journal.extend_from_slice(&version.to_be_bytes());
    journal.extend_from_slice(&contract);
    journal.extend_from_slice(&offer_id.to_be_bytes());
    journal.extend_from_slice(&order_nonce.to_be_bytes());
    journal.extend_from_slice(&seller);
    journal.extend_from_slice(&buyer);
    journal.extend_from_slice(&buyer_key);
    journal.extend_from_slice(&commitment);
    journal.extend_from_slice(&ciphertext_hash);
    journal.extend_from_slice(&network);
    journal.extend_from_slice(&funded_at.to_be_bytes());
    journal.extend_from_slice(&funding_root);
    assert_eq!(journal.len(), 281);

    env::commit_slice(&journal);
}
