//! Registration proof guest, version 2.
//!
//! Private inputs: the two-anchor session bundle of `SOURCES.md` F-97, the
//! author's blobs, and the epoch's two secrets. Public output: the 262-byte
//! version 2 journal of `proofs/session-core/src/statement.rs`.
//!
//! What the guest establishes: the inventory leaf at its height lists exactly
//! the manifest's CIDs and the global leaf there names the bundle's epoch; the
//! verdict leaf at the `ValidationFinished` height carries the chain's status
//! flags, has no flips left and names the next epoch; each blob hashes to the
//! CID its record carries and opens with the committed secrets; and the journal
//! binds both anchors, the content commitment and the key commitment.
//!
//! What it does not claim: a per-flip verdict (the chain stores none), or that
//! the blobs stay fetchable (that is the availability advisory's job).

use fod_registration_core::blob::{self, BlobError};
use fod_flip_core as flip_core;
use fod_session_core::iavl::{Hash256, PathItem};
use fod_session_core::session::{self, Anchor, Bundle, Leaf};
use fod_session_core::statement::{self, Statement};
use risc0_zkvm::guest::env;
use risc0_zkvm::sha::Sha256 as _;
use sha2::{Digest, Sha256};

const MAX_FLIP_BYTES: usize = 1024 * 600;
const ADDRESS_BYTES: usize = 20;
const DIGEST_BYTES: usize = 32;
const SCALAR_BYTES: usize = 32;

/// The zkVM's accelerated SHA-256: the fold is hash-bound.
struct AcceleratedHash;

impl Hash256 for AcceleratedHash {
    fn hash(bytes: &[u8]) -> [u8; 32] {
        let digest = risc0_zkvm::sha::Impl::hash_bytes(bytes);
        let mut out = [0u8; 32];
        out.copy_from_slice(digest.as_bytes());
        out
    }
}

type PathItemIn = (i64, i64, i64, Vec<u8>, Vec<u8>);
type LeafIn = (Vec<u8>, Vec<u8>, i64, Vec<PathItemIn>);

fn path_of(input: &LeafIn) -> Vec<PathItem<'_>> {
    input
        .3
        .iter()
        .map(|(height, size, version, left, right)| PathItem {
            height: *height,
            size: *size,
            version: *version,
            left,
            right,
        })
        .collect()
}

/// The supported shape of a flip's two parts: non-empty and together within the
/// node's blob cap. A stricter word-pair check is not claimed.
/// The content check of `SPEC.md` 5.5 item 6: the decrypted parts have to be a
/// flip the client's own renderer shows: raw RLP lists, four pictures, and two
/// orders a validator chooses between (`SOURCES.md` F-98). The old fixture used
/// ASCII hex around the RLP, so that wrapper remains a fallback. Anything else
/// is refused, so a bundle cannot sell a blob that is not a flip.
fn check_structure(public_part: &[u8], private_part: &[u8]) -> Result<(), BlobError> {
    if public_part.is_empty() || private_part.is_empty() {
        return Err(BlobError::Malformed);
    }
    if public_part.len() + private_part.len() > MAX_FLIP_BYTES {
        return Err(BlobError::Malformed);
    }
    if flip_core::read(public_part, private_part).is_ok() {
        return Ok(());
    }
    let public_bytes = flip_core::hex_bytes_len(public_part).map_err(|_| BlobError::Malformed)?;
    let private_bytes = flip_core::hex_bytes_len(private_part).map_err(|_| BlobError::Malformed)?;
    let mut public = vec![0u8; public_bytes];
    flip_core::decode_hex(public_part, &mut public).map_err(|_| BlobError::Malformed)?;
    let mut private = vec![0u8; private_bytes];
    flip_core::decode_hex(private_part, &mut private).map_err(|_| BlobError::Malformed)?;
    flip_core::read(&public, &private)
        .map(|_| ())
        .map_err(|_| BlobError::Malformed)
}

fn main() {
    let version: u32 = env::read();
    let contract: Vec<u8> = env::read();
    let network: String = env::read();
    let author: Vec<u8> = env::read();
    let epoch: u16 = env::read();
    let consensus: String = env::read();
    let policy: String = env::read();

    let inventory_height: u64 = env::read();
    let inventory_root: Vec<u8> = env::read();
    let inventory_global: LeafIn = env::read();
    let inventory_identity: LeafIn = env::read();
    let verdict_height: u64 = env::read();
    let verdict_root: Vec<u8> = env::read();
    let verdict_global: LeafIn = env::read();
    let verdict_identity: LeafIn = env::read();

    let manifest: Vec<Vec<u8>> = env::read();
    let blob_count: u32 = env::read();
    assert_eq!(
        blob_count as usize,
        manifest.len(),
        "the content and the manifest differ in size"
    );
    let mut blobs = Vec::with_capacity(blob_count as usize);
    for _ in 0..blob_count {
        let cid: Vec<u8> = env::read();
        let byte_len: u32 = env::read();
        assert!(
            byte_len as usize <= blob::MAX_FLIP_BYTES,
            "a blob exceeds the node's size cap"
        );
        let mut raw_words = vec![0u32; (byte_len as usize).div_ceil(4)];
        env::read_slice(&mut raw_words);
        let mut bytes = Vec::with_capacity(raw_words.len() * 4);
        for word in raw_words {
            bytes.extend_from_slice(&word.to_le_bytes());
        }
        bytes.truncate(byte_len as usize);
        blobs.push((cid, bytes));
    }
    let public_secret: Vec<u8> = env::read();
    let private_secret: Vec<u8> = env::read();

    assert_eq!(
        version,
        statement::STATEMENT_VERSION,
        "unsupported statement version"
    );
    assert_eq!(contract.len(), ADDRESS_BYTES, "contract address size");
    assert_eq!(author.len(), ADDRESS_BYTES, "author address size");
    assert_eq!(inventory_root.len(), DIGEST_BYTES, "inventory root size");
    assert_eq!(verdict_root.len(), DIGEST_BYTES, "verdict root size");
    assert_eq!(public_secret.len(), SCALAR_BYTES, "public part secret size");
    assert_eq!(private_secret.len(), SCALAR_BYTES, "private part secret size");
    assert!(!manifest.is_empty(), "the manifest covers no CIDs");

    let inventory_identity_path = path_of(&inventory_identity);
    let inventory_global_path = path_of(&inventory_global);
    let verdict_identity_path = path_of(&verdict_identity);
    let verdict_global_path = path_of(&verdict_global);
    let manifest_refs: Vec<&[u8]> = manifest.iter().map(|cid| cid.as_slice()).collect();

    let bundle = Bundle {
        author: &author,
        epoch: u32::from(epoch),
        manifest: &manifest_refs,
        inventory: Anchor {
            height: inventory_height,
            root: &inventory_root,
            identity: Leaf {
                key: &inventory_identity.0,
                value: &inventory_identity.1,
                version: inventory_identity.2,
                path: &inventory_identity_path,
            },
            global: Leaf {
                key: &inventory_global.0,
                value: &inventory_global.1,
                version: inventory_global.2,
                path: &inventory_global_path,
            },
        },
        verdict: Anchor {
            height: verdict_height,
            root: &verdict_root,
            identity: Leaf {
                key: &verdict_identity.0,
                value: &verdict_identity.1,
                version: verdict_identity.2,
                path: &verdict_identity_path,
            },
            global: Leaf {
                key: &verdict_global.0,
                value: &verdict_global.1,
                version: verdict_global.2,
                path: &verdict_global_path,
            },
        },
    };
    let verdict = session::verify_with::<AcceleratedHash>(&bundle)
        .expect("the session bundle does not prove this author's epoch");

    // Every blob hashes to the CID its record carries and opens with the
    // committed secrets, and its parts have the supported shape.
    let mut blob_cids: Vec<Vec<u8>> = Vec::with_capacity(blobs.len());
    for (cid, blob_bytes) in &blobs {
        let computed = blob::blob_cid_multibase(blob_bytes)
            .unwrap_or_else(|problem| panic!("a blob has no CID this construction produces: {problem:?}"));
        assert_eq!(
            computed.as_bytes(),
            cid.as_slice(),
            "a blob does not hash to the CID its record carries"
        );
        let (public_part, private_part) = blob::open_flip_blob(blob_bytes, &public_secret, &private_secret)
            .expect("a blob did not open with the committed secrets");
        check_structure(&public_part, &private_part).expect("a part has an unsupported shape");
        blob_cids.push(cid.clone());
    }
    let mut manifest_sorted = manifest.clone();
    manifest_sorted.sort();
    blob_cids.sort();
    assert_eq!(
        blob_cids, manifest_sorted,
        "the blobs are not the manifest's CIDs"
    );

    let author_address: [u8; ADDRESS_BYTES] = author.clone().try_into().expect("author size");
    let public_scalar: [u8; SCALAR_BYTES] = public_secret
        .clone()
        .try_into()
        .expect("secret size");
    let private_scalar: [u8; SCALAR_BYTES] = private_secret
        .clone()
        .try_into()
        .expect("secret size");
    let secrets = fod_registration_core::secrets_bytes(
        epoch,
        &author_address,
        &public_scalar,
        &private_scalar,
    );

    let sorted_refs: Vec<&[u8]> = manifest_sorted.iter().map(|cid| cid.as_slice()).collect();
    let inventory_commitment =
        statement::inventory_commitment(&author_address, epoch, &sorted_refs);
    let inventory_root_bytes: [u8; DIGEST_BYTES] =
        inventory_root.clone().try_into().expect("root size");
    let verdict_root_bytes: [u8; DIGEST_BYTES] = verdict_root.clone().try_into().expect("root size");
    let manifest_commitment = statement::manifest_commitment(
        &network,
        &consensus,
        &policy,
        &inventory_commitment,
        inventory_height,
        &inventory_root_bytes,
        verdict_height,
        &verdict_root_bytes,
    );
    let key_commitment = fod_registration_core::secrets_commitment(&secrets);
    let policy_digest: [u8; DIGEST_BYTES] = Sha256::digest(policy.as_bytes()).into();
    let consensus_digest: [u8; DIGEST_BYTES] = Sha256::digest(consensus.as_bytes()).into();
    // The escrow holds the digest of the network it was built for and compares
    // this field with it, so a statement proved for another network cannot
    // register there.
    let network_digest = statement::network_digest(&network);

    let journal = Statement {
        contract: &contract,
        author: &author,
        epoch,
        covered_flips: manifest.len() as u32,
        inventory_height,
        inventory_root: &inventory_root,
        verdict_height,
        verdict_root: &verdict_root,
        validation_flags: verdict.flags,
        manifest_commitment: &manifest_commitment,
        key_commitment: &key_commitment,
        // Published so a buyer can compute the same commitment over a candidate
        // CID set and compare it with what the chain stores.
        inventory_commitment: &inventory_commitment,
        policy_digest: &policy_digest,
        consensus_digest: &consensus_digest,
        network_digest: &network_digest,
    }
    .encode()
    .expect("a statement field has the wrong size");
    env::commit_slice(&journal);
}
