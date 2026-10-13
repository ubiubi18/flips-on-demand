//! The delivery statement's journal layout (version 2).
//!
//! Fixed-size fields, so the contract can read them without a serialiser:
//! version (4) || contract (20) || offer id (8) || order nonce (8) ||
//! seller (20) || buyer (20) || buyer delivery key (65) ||
//! secrets commitment (32) || ciphertext hash (32) || network digest (32) || funding height (8) ||
//! canonical funding state root (32).

pub const JOURNAL_BYTES: usize = 4 + 20 + 8 + 8 + 20 + 20 + 65 + 32 + 32 + 32 + 8 + 32;
pub const JOURNAL_VERSION: u32 = 2;

pub struct Journal<'a> {
    pub version: u32,
    pub contract: &'a [u8],
    pub offer_id: u64,
    pub order_nonce: u64,
    pub seller: &'a [u8],
    pub buyer: &'a [u8],
    pub buyer_key: &'a [u8],
    pub commitment: &'a [u8],
    pub ciphertext_hash: &'a [u8],
    pub network: &'a [u8],
    pub funded_at: u64,
    pub funding_root: &'a [u8],
}

impl<'a> Journal<'a> {
    pub fn parse(bytes: &'a [u8]) -> Option<Journal<'a>> {
        if bytes.len() != JOURNAL_BYTES {
            return None;
        }
        let mut cursor = 0usize;
        let mut take = |len: usize| {
            let slice = &bytes[cursor..cursor + len];
            cursor += len;
            slice
        };
        let version = u32::from_be_bytes(take(4).try_into().ok()?);
        if version != JOURNAL_VERSION {
            return None;
        }
        let contract = take(20);
        let offer_id = u64::from_be_bytes(take(8).try_into().ok()?);
        let order_nonce = u64::from_be_bytes(take(8).try_into().ok()?);
        let seller = take(20);
        let buyer = take(20);
        let buyer_key = take(65);
        let commitment = take(32);
        let ciphertext_hash = take(32);
        let network = take(32);
        let funded_at = u64::from_be_bytes(take(8).try_into().ok()?);
        let funding_root = take(32);
        Some(Journal {
            version,
            contract,
            offer_id,
            order_nonce,
            seller,
            buyer,
            buyer_key,
            commitment,
            ciphertext_hash,
            network,
            funded_at,
            funding_root,
        })
    }
}
