//! The version 2 registration statement: the journal the escrow reads and the
//! manifest commitment it binds.
//!
//! Version 1 bound one anchor (the `IdentityRoot` a candidate-list proof folded
//! to). Version 2 binds the two state versions a session actually needs - the
//! inventory height with the flips and the `ValidationFinished` height with the
//! verdict flags (`SOURCES.md` F-97) - and carries those flags to the escrow, so
//! the bundle's label is a function of the chain's own record.
//!
//! Layout, in the order the escrow reads it (`contracts/escrow/src/verifier.rs`
//! holds the matching offsets):
//!
//! ```text
//!   0    4   version (u32 BE, 2)
//!   4    20  contract
//!   24   20  author
//!   44   2   epoch (u16 BE)
//!   46   4   covered flips (u32 BE)
//!   50   8   inventory height (u64 BE)
//!   58   32  inventory state root
//!   90   8   verdict height (u64 BE)
//!   98   32  verdict state root
//!   130  4   validation flags (u32 BE)
//!   134  32  manifest commitment
//!   166  32  key commitment
//!   198  32  inventory commitment
//!   230  32  policy digest
//!   262  32  consensus digest
//!   294  32  network digest
//! ```
//!
//! Version 3 publishes two values version 2 only folded into the manifest
//! commitment, where no reader could see them:
//!
//! - the *inventory commitment* over the bundle's CIDs, so a buyer holding a
//!   candidate set can compute the same value and compare it with what the
//!   chain stores (`tools/fod/pool_buy.go`, the C1 finding in `PROGRESS.md`);
//! - the *network digest*, so the escrow can refuse a statement made for
//!   another network by comparing it with the constant it was built for, the
//!   way it already refuses a foreign eligibility policy or consensus version.

use sha2::{Digest, Sha256};

pub const STATEMENT_VERSION: u32 = 3;
pub const JOURNAL_BYTES: usize = 326;

pub const OFFSET_VERSION: usize = 0;
pub const OFFSET_CONTRACT: usize = 4;
pub const OFFSET_AUTHOR: usize = 24;
pub const OFFSET_EPOCH: usize = 44;
pub const OFFSET_COVERED_FLIPS: usize = 46;
pub const OFFSET_INVENTORY_HEIGHT: usize = 50;
pub const OFFSET_INVENTORY_ROOT: usize = 58;
pub const OFFSET_VERDICT_HEIGHT: usize = 90;
pub const OFFSET_VERDICT_ROOT: usize = 98;
pub const OFFSET_VALIDATION_FLAGS: usize = 130;
pub const OFFSET_MANIFEST_COMMITMENT: usize = 134;
pub const OFFSET_KEY_COMMITMENT: usize = 166;
pub const OFFSET_INVENTORY_COMMITMENT: usize = 198;
pub const OFFSET_POLICY_DIGEST: usize = 230;
pub const OFFSET_CONSENSUS_DIGEST: usize = 262;
pub const OFFSET_NETWORK_DIGEST: usize = 294;

pub const ADDRESS_BYTES: usize = 20;
pub const DIGEST_BYTES: usize = 32;

/// The inventory commitment keeps the version 1 format: the CLI writes the same
/// bytes, so a change here would orphan every manifest it produced.
const INVENTORY_DOMAIN: &[u8] = b"fod.bundle-manifest:v1";
/// The manifest commitment binds the anchors, and its format changed with them.
pub const MANIFEST_DOMAIN: &[u8] = b"fod:manifest:claim:v2";

/// The public statement one registration proof carries.
pub struct Statement<'a> {
    pub contract: &'a [u8],
    pub author: &'a [u8],
    pub epoch: u16,
    pub covered_flips: u32,
    pub inventory_height: u64,
    pub inventory_root: &'a [u8],
    pub verdict_height: u64,
    pub verdict_root: &'a [u8],
    pub validation_flags: u32,
    pub manifest_commitment: &'a [u8],
    pub key_commitment: &'a [u8],
    pub inventory_commitment: &'a [u8],
    pub policy_digest: &'a [u8],
    pub consensus_digest: &'a [u8],
    pub network_digest: &'a [u8],
}

impl<'a> Statement<'a> {
    /// The journal bytes the guest commits and the escrow reads. `None` means a
    /// field has the wrong size, which the guest turns into a refusal.
    pub fn encode(&self) -> Option<[u8; JOURNAL_BYTES]> {
        if self.contract.len() != ADDRESS_BYTES
            || self.author.len() != ADDRESS_BYTES
            || self.inventory_root.len() != DIGEST_BYTES
            || self.verdict_root.len() != DIGEST_BYTES
            || self.manifest_commitment.len() != DIGEST_BYTES
            || self.key_commitment.len() != DIGEST_BYTES
            || self.inventory_commitment.len() != DIGEST_BYTES
            || self.policy_digest.len() != DIGEST_BYTES
            || self.consensus_digest.len() != DIGEST_BYTES
            || self.network_digest.len() != DIGEST_BYTES
        {
            return None;
        }
        let mut journal = [0u8; JOURNAL_BYTES];
        journal[OFFSET_VERSION..OFFSET_VERSION + 4]
            .copy_from_slice(&STATEMENT_VERSION.to_be_bytes());
        journal[OFFSET_CONTRACT..OFFSET_CONTRACT + ADDRESS_BYTES].copy_from_slice(self.contract);
        journal[OFFSET_AUTHOR..OFFSET_AUTHOR + ADDRESS_BYTES].copy_from_slice(self.author);
        journal[OFFSET_EPOCH..OFFSET_EPOCH + 2].copy_from_slice(&self.epoch.to_be_bytes());
        journal[OFFSET_COVERED_FLIPS..OFFSET_COVERED_FLIPS + 4]
            .copy_from_slice(&self.covered_flips.to_be_bytes());
        journal[OFFSET_INVENTORY_HEIGHT..OFFSET_INVENTORY_HEIGHT + 8]
            .copy_from_slice(&self.inventory_height.to_be_bytes());
        journal[OFFSET_INVENTORY_ROOT..OFFSET_INVENTORY_ROOT + DIGEST_BYTES]
            .copy_from_slice(self.inventory_root);
        journal[OFFSET_VERDICT_HEIGHT..OFFSET_VERDICT_HEIGHT + 8]
            .copy_from_slice(&self.verdict_height.to_be_bytes());
        journal[OFFSET_VERDICT_ROOT..OFFSET_VERDICT_ROOT + DIGEST_BYTES]
            .copy_from_slice(self.verdict_root);
        journal[OFFSET_VALIDATION_FLAGS..OFFSET_VALIDATION_FLAGS + 4]
            .copy_from_slice(&self.validation_flags.to_be_bytes());
        journal[OFFSET_MANIFEST_COMMITMENT..OFFSET_MANIFEST_COMMITMENT + DIGEST_BYTES]
            .copy_from_slice(self.manifest_commitment);
        journal[OFFSET_KEY_COMMITMENT..OFFSET_KEY_COMMITMENT + DIGEST_BYTES]
            .copy_from_slice(self.key_commitment);
        journal[OFFSET_INVENTORY_COMMITMENT..OFFSET_INVENTORY_COMMITMENT + DIGEST_BYTES]
            .copy_from_slice(self.inventory_commitment);
        journal[OFFSET_POLICY_DIGEST..OFFSET_POLICY_DIGEST + DIGEST_BYTES]
            .copy_from_slice(self.policy_digest);
        journal[OFFSET_CONSENSUS_DIGEST..OFFSET_CONSENSUS_DIGEST + DIGEST_BYTES]
            .copy_from_slice(self.consensus_digest);
        journal[OFFSET_NETWORK_DIGEST..OFFSET_NETWORK_DIGEST + DIGEST_BYTES]
            .copy_from_slice(self.network_digest);
        Some(journal)
    }
}

/// The digest a statement binds a network by: `Sha256` over the canonical
/// network name, the same way the policy and consensus digests are taken over
/// their own strings. The escrow holds the digest of the network it was built
/// for, so a statement proved for another network is refused there.
pub fn network_digest(network: &str) -> [u8; DIGEST_BYTES] {
    let mut out = [0u8; DIGEST_BYTES];
    out.copy_from_slice(&Sha256::digest(network.as_bytes()));
    out
}

fn hex_lower(bytes: &[u8]) -> [u8; 40] {
    const DIGITS: &[u8; 16] = b"0123456789abcdef";
    let mut out = [0u8; 40];
    for (index, byte) in bytes.iter().enumerate() {
        out[index * 2] = DIGITS[(byte >> 4) as usize];
        out[index * 2 + 1] = DIGITS[(byte & 0x0f) as usize];
    }
    out
}

/// The commitment over the bundle's content: the author, the epoch, and the
/// CIDs in canonical (sorted) order, under the version 1 domain the CLI uses.
pub fn inventory_commitment(
    author: &[u8; ADDRESS_BYTES],
    epoch: u16,
    sorted_cids: &[&[u8]],
) -> [u8; DIGEST_BYTES] {
    let mut hasher = Sha256::new();
    hasher.update(INVENTORY_DOMAIN);
    hasher.update(hex_lower(author));
    hasher.update(epoch.to_be_bytes());
    for cid in sorted_cids {
        hasher.update([0u8]);
        hasher.update(cid);
    }
    let digest = hasher.finalize();
    let mut out = [0u8; DIGEST_BYTES];
    out.copy_from_slice(&digest);
    out
}

/// The manifest commitment: the content commitment plus both anchors, so a
/// journal cannot name one height's evidence under another height's root.
pub fn manifest_commitment(
    network: &str,
    consensus: &str,
    policy: &str,
    inventory: &[u8; DIGEST_BYTES],
    inventory_height: u64,
    inventory_root: &[u8; DIGEST_BYTES],
    verdict_height: u64,
    verdict_root: &[u8; DIGEST_BYTES],
) -> [u8; DIGEST_BYTES] {
    let mut hasher = Sha256::new();
    hasher.update(MANIFEST_DOMAIN);
    hasher.update(inventory);
    hasher.update(inventory_height.to_be_bytes());
    hasher.update(inventory_root);
    hasher.update(verdict_height.to_be_bytes());
    hasher.update(verdict_root);
    for field in [consensus, policy, network] {
        hasher.update((field.len() as u16).to_be_bytes());
        hasher.update(field.as_bytes());
    }
    let digest = hasher.finalize();
    let mut out = [0u8; DIGEST_BYTES];
    out.copy_from_slice(&digest);
    out
}

#[cfg(test)]
mod tests {
    extern crate std;

    use super::*;

    fn digest(seed: u8) -> [u8; DIGEST_BYTES] {
        [seed; DIGEST_BYTES]
    }

    fn statement() -> Statement<'static> {
        Statement {
            contract: &[0x11; ADDRESS_BYTES],
            author: &[0x22; ADDRESS_BYTES],
            epoch: 228,
            covered_flips: 5,
            inventory_height: 10_870_000,
            inventory_root: &[0x33; DIGEST_BYTES],
            verdict_height: 10_880_001,
            verdict_root: &[0x44; DIGEST_BYTES],
            validation_flags: 0,
            manifest_commitment: &[0x55; DIGEST_BYTES],
            key_commitment: &[0x66; DIGEST_BYTES],
            inventory_commitment: &[0x99; DIGEST_BYTES],
            policy_digest: &[0x77; DIGEST_BYTES],
            consensus_digest: &[0x88; DIGEST_BYTES],
            network_digest: &[0xaa; DIGEST_BYTES],
        }
    }

    fn u32_at(journal: &[u8], offset: usize) -> u32 {
        u32::from_be_bytes(journal[offset..offset + 4].try_into().expect("u32"))
    }

    fn u64_at(journal: &[u8], offset: usize) -> u64 {
        u64::from_be_bytes(journal[offset..offset + 8].try_into().expect("u64"))
    }

    /// The escrow reads the journal by these offsets; the encoder has to agree
    /// with the table in this module's documentation, byte for byte.
    #[test]
    fn the_journal_layout_is_what_the_escrow_reads() {
        let journal = statement().encode().expect("encode");
        assert_eq!(journal.len(), JOURNAL_BYTES);
        assert_eq!(u32_at(&journal, OFFSET_VERSION), STATEMENT_VERSION);
        assert_eq!(&journal[OFFSET_CONTRACT..OFFSET_CONTRACT + 20], &[0x11; 20]);
        assert_eq!(&journal[OFFSET_AUTHOR..OFFSET_AUTHOR + 20], &[0x22; 20]);
        assert_eq!(
            u32::from(u16::from_be_bytes([
                journal[OFFSET_EPOCH],
                journal[OFFSET_EPOCH + 1]
            ])),
            228
        );
        assert_eq!(u32_at(&journal, OFFSET_COVERED_FLIPS), 5);
        assert_eq!(u64_at(&journal, OFFSET_INVENTORY_HEIGHT), 10_870_000);
        assert_eq!(
            &journal[OFFSET_INVENTORY_ROOT..OFFSET_INVENTORY_ROOT + 32],
            &[0x33; 32]
        );
        assert_eq!(u64_at(&journal, OFFSET_VERDICT_HEIGHT), 10_880_001);
        assert_eq!(
            &journal[OFFSET_VERDICT_ROOT..OFFSET_VERDICT_ROOT + 32],
            &[0x44; 32]
        );
        assert_eq!(u32_at(&journal, OFFSET_VALIDATION_FLAGS), 0);
        assert_eq!(
            &journal[OFFSET_MANIFEST_COMMITMENT..OFFSET_MANIFEST_COMMITMENT + 32],
            &[0x55; 32]
        );
        assert_eq!(
            &journal[OFFSET_KEY_COMMITMENT..OFFSET_KEY_COMMITMENT + 32],
            &[0x66; 32]
        );
        assert_eq!(
            &journal[OFFSET_INVENTORY_COMMITMENT..OFFSET_INVENTORY_COMMITMENT + 32],
            &[0x99; 32]
        );
        assert_eq!(
            &journal[OFFSET_POLICY_DIGEST..OFFSET_POLICY_DIGEST + 32],
            &[0x77; 32]
        );
        assert_eq!(
            &journal[OFFSET_CONSENSUS_DIGEST..OFFSET_CONSENSUS_DIGEST + 32],
            &[0x88; 32]
        );
        assert_eq!(
            &journal[OFFSET_NETWORK_DIGEST..OFFSET_NETWORK_DIGEST + 32],
            &[0xaa; 32]
        );
        // The last field ends exactly at the journal's end.
        assert_eq!(OFFSET_NETWORK_DIGEST + 32, JOURNAL_BYTES);
    }

    #[test]
    fn a_wrongly_sized_field_is_refused_instead_of_truncated() {
        let mut short_author = statement();
        short_author.author = &[0x22; 19];
        assert!(short_author.encode().is_none());

        let mut short_root = statement();
        short_root.verdict_root = &[0x44; 31];
        assert!(short_root.encode().is_none());

        let mut short_inventory = statement();
        short_inventory.inventory_commitment = &[0x99; 31];
        assert!(short_inventory.encode().is_none());

        let mut short_network = statement();
        short_network.network_digest = &[0xaa; 33];
        assert!(short_network.encode().is_none());
    }

    #[test]
    fn the_network_digest_is_taken_over_the_canonical_name() {
        let mut expected = Sha256::new();
        expected.update(b"idena");
        let digest: [u8; DIGEST_BYTES] = expected.finalize().into();
        assert_eq!(network_digest("idena"), digest);
        // Two networks never share a digest, so the escrow's constant separates
        // them.
        assert_ne!(network_digest("idena"), network_digest("idena-local"));
    }

    #[test]
    fn the_flags_travel_in_the_journal() {
        let mut flagged = statement();
        flagged.validation_flags = 0b101;
        let journal = flagged.encode().expect("encode");
        assert_eq!(u32_at(&journal, OFFSET_VALIDATION_FLAGS), 0b101);
    }

    #[test]
    fn the_manifest_commitment_binds_both_anchors() {
        let inventory = inventory_commitment(&[0x22; 20], 228, &[b"cid-a", b"cid-b"]);
        let base = manifest_commitment(
            "idena",
            "consensus-v12",
            "fod:eligibility:v1",
            &inventory,
            10_870_000,
            &digest(0x33),
            10_880_001,
            &digest(0x44),
        );
        let other_height = manifest_commitment(
            "idena",
            "consensus-v12",
            "fod:eligibility:v1",
            &inventory,
            10_870_001,
            &digest(0x33),
            10_880_001,
            &digest(0x44),
        );
        let other_root = manifest_commitment(
            "idena",
            "consensus-v12",
            "fod:eligibility:v1",
            &inventory,
            10_870_000,
            &digest(0x33),
            10_880_001,
            &digest(0x45),
        );
        let other_network = manifest_commitment(
            "idena-local",
            "consensus-v12",
            "fod:eligibility:v1",
            &inventory,
            10_870_000,
            &digest(0x33),
            10_880_001,
            &digest(0x44),
        );
        assert_ne!(base, other_height);
        assert_ne!(base, other_root);
        assert_ne!(base, other_network);
        assert_eq!(
            base,
            manifest_commitment(
                "idena",
                "consensus-v12",
                "fod:eligibility:v1",
                &inventory,
                10_870_000,
                &digest(0x33),
                10_880_001,
                &digest(0x44)
            )
        );
    }

    #[test]
    fn the_inventory_commitment_keeps_the_version_one_format() {
        // The same bytes the version 1 guest hashed: domain, the author as lower
        // hex, the epoch, then each CID behind a zero byte.
        let mut expected = Sha256::new();
        expected.update(b"fod.bundle-manifest:v1");
        expected.update(b"2222222222222222222222222222222222222222");
        expected.update(228u16.to_be_bytes());
        expected.update([0u8]);
        expected.update(b"cid-a");
        expected.update([0u8]);
        expected.update(b"cid-b");
        let digest: [u8; 32] = expected.finalize().into();
        assert_eq!(
            inventory_commitment(&[0x22; 20], 228, &[b"cid-a", b"cid-b"]),
            digest
        );
    }
}
