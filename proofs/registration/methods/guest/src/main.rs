//! Registration proof guest for Flips on Demand.
//!
//! Private inputs: the epoch's assignment and every participant's answers, the
//! author's blobs, and the secrets a bundle sells. Public output: the 178-byte
//! registration claim of `SOURCES.md` F-69, which the escrow re-checks.
//!
//! What the guest establishes, per `SPEC.md` 5.5: the manifest it was given holds
//! exactly the CIDs of the blobs it checked, every covered flip satisfies the
//! eligibility policy of `SOURCES.md` F-66, each blob hashes to the CID its
//! manifest names, the secrets open each blob, and the parts have the supported
//! shape. What it cannot establish is that the inventory is complete with
//! respect to the chain: that is a property of the inputs, which is why the claim
//! binds the manifest commitment and anyone can compare that manifest with the
//! chain's submissions for the author and epoch.
//!
//! The identity private key never enters here: the guest receives the epoch's
//! derived scalars, not the key that derived them.

use fod_registration_core::blob::{self, BlobError};
use fod_registration_core::qualification as q;
use risc0_zkvm::guest::env;
use sha2::{Digest, Sha256};

/// The statement version, the same value the claim's first field carries.
const STATEMENT_VERSION: u32 = 1;
const ADDRESS_BYTES: usize = 20;
const DIGEST_BYTES: usize = 32;
const JOURNAL_BYTES: usize = 178;
const SCALAR_BYTES: usize = 32;
/// The node's cap on a flip blob (`common.MaxFlipSize`).
const MAX_FLIP_BYTES: usize = 1024 * 600;

const INVENTORY_DOMAIN: &[u8] = b"fod.bundle-manifest:v1";
const CLAIM_DOMAIN: &[u8] = b"fod:manifest:claim:v1";

fn hex_lower(bytes: &[u8]) -> Vec<u8> {
    const DIGITS: &[u8; 16] = b"0123456789abcdef";
    let mut out = Vec::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(DIGITS[(byte >> 4) as usize]);
        out.push(DIGITS[(byte & 0x0f) as usize]);
    }
    out
}

/// The commitment the CLI's content export produces: the author, the epoch, and
/// the sorted CIDs, under their own domain.
fn inventory_commitment(author: &[u8], epoch: u16, cids: &[Vec<u8>]) -> [u8; 32] {
    let mut hasher = Sha256::new();
    hasher.update(INVENTORY_DOMAIN);
    hasher.update(hex_lower(author));
    hasher.update(epoch.to_be_bytes());
    for cid in cids {
        hasher.update([0u8]);
        hasher.update(cid);
    }
    let digest = hasher.finalize();
    let mut out = [0u8; 32];
    out.copy_from_slice(&digest);
    out
}

/// The manifest's canonical bytes, as `SOURCES.md` F-67 records them.
fn manifest_commitment(
    network: &str,
    consensus: &str,
    policy: &str,
    inventory: &[u8; 32],
    anchor: &[u8],
) -> [u8; 32] {
    let mut hasher = Sha256::new();
    hasher.update(CLAIM_DOMAIN);
    hasher.update(inventory);
    hasher.update(anchor);
    for field in [consensus, policy, network] {
        hasher.update((field.len() as u16).to_be_bytes());
        hasher.update(field.as_bytes());
    }
    let digest = hasher.finalize();
    let mut out = [0u8; 32];
    out.copy_from_slice(&digest);
    out
}

/// The supported shape of a flip's two parts: non-empty, together within the
/// node's blob cap, and text. A stricter word-pair check is not claimed.
fn check_structure(public_part: &[u8], private_part: &[u8]) -> Result<(), BlobError> {
    if public_part.is_empty() || private_part.is_empty() {
        return Err(BlobError::Malformed);
    }
    if public_part.len() + private_part.len() > MAX_FLIP_BYTES {
        return Err(BlobError::Malformed);
    }
    if core::str::from_utf8(public_part).is_err() || core::str::from_utf8(private_part).is_err() {
        return Err(BlobError::Malformed);
    }
    Ok(())
}

fn main() {
    let version: u32 = env::read();
    let contract: Vec<u8> = env::read();
    let network: String = env::read();
    let author: Vec<u8> = env::read();
    let epoch: u16 = env::read();
    let anchor: Vec<u8> = env::read();
    let consensus: String = env::read();
    let policy: String = env::read();
    let cids: Vec<String> = env::read();
    // (flip index, CID, blob bytes) for every flip the manifest covers.
    let inventory: Vec<(u32, String, Vec<u8>)> = env::read();
    let public_secret: Vec<u8> = env::read();
    let private_secret: Vec<u8> = env::read();
    // (assigned flips, answers, grades) per participant, in assignment order.
    let candidates: Vec<(Vec<u32>, Vec<u8>, Vec<u8>)> = env::read();
    let covered: Vec<u32> = env::read();
    // The session binding: the records of the shard's candidates with the
    // membership proofs that place them under the anchor (`SPEC.md` 5.5).
    let anchor_bundle: Vec<u8> = env::read();

    assert_eq!(version, STATEMENT_VERSION, "unsupported statement version");
    assert_eq!(contract.len(), ADDRESS_BYTES, "contract address size");
    assert_eq!(author.len(), ADDRESS_BYTES, "author address size");
    assert_eq!(anchor.len(), DIGEST_BYTES, "anchor size");
    assert_eq!(public_secret.len(), SCALAR_BYTES, "public part secret size");
    assert_eq!(private_secret.len(), SCALAR_BYTES, "private part secret size");
    assert!(!cids.is_empty(), "the manifest covers no CIDs");
    assert!(!inventory.is_empty(), "the inventory is empty");
    assert!(!covered.is_empty(), "the bundle covers no flips");

    // Every record in the bundle has to fold to the anchor the claim commits,
    // or the input is evidence about some other state than the one it names.
    let proven = fod_anchor_core::verify_bundle(&anchor, &anchor_bundle)
        .expect("the proof anchor does not fold to the anchor the manifest commits");
    assert!(proven > 0, "the proof anchor carries no records");

    // 1. The manifest's CIDs and the inventory are the same set.
    let mut manifest_cids = cids.clone();
    manifest_cids.sort();
    let mut inventory_cids: Vec<String> =
        inventory.iter().map(|(_, cid, _)| cid.clone()).collect();
    inventory_cids.sort();
    assert_eq!(
        manifest_cids, inventory_cids,
        "the manifest and the inventory name different CIDs"
    );

    // The covered flips are the inventory's flips, and neither repeats.
    let mut inventory_flips: Vec<u32> = inventory.iter().map(|(flip, _, _)| *flip).collect();
    inventory_flips.sort_unstable();
    for pair in inventory_flips.windows(2) {
        assert_ne!(pair[0], pair[1], "a flip appears twice in the inventory");
    }
    let mut covered_sorted = covered.clone();
    covered_sorted.sort_unstable();
    for pair in covered_sorted.windows(2) {
        assert_ne!(pair[0], pair[1], "a covered flip repeats");
    }
    assert_eq!(
        inventory_flips, covered_sorted,
        "the covered flips are not the inventory's flips"
    );

    // 2. Every blob hashes to the CID its entry names, opens with the secrets,
    // and has the supported shape.
    let secrets = fod_registration_core::secrets_bytes(
        epoch,
        &author.clone().try_into().expect("author size"),
        &public_secret.clone().try_into().expect("secret size"),
        &private_secret.clone().try_into().expect("secret size"),
    );
    for (flip, cid, blob_bytes) in &inventory {
        let computed = blob::blob_cid_multibase(blob_bytes).unwrap_or_else(|problem| {
            panic!("flip {flip}: the blob has no CID this construction produces: {problem:?}")
        });
        assert_eq!(
            &computed, cid,
            "flip {flip}: the blob does not hash to the CID its entry names"
        );
        let (public_part, private_part) =
            blob::open_flip_blob(blob_bytes, &public_secret, &private_secret)
                .expect("a blob did not open with the committed secrets");
        check_structure(&public_part, &private_part).expect("a part has an unsupported shape");
    }

    // 3. The epoch's verdicts, and the policy over the covered flips.
    let total_flips = inventory_flips
        .iter()
        .copied()
        .chain(candidates.iter().flat_map(|(flips, _, _)| flips.iter().copied()))
        .max()
        .map(|max| max as usize + 1)
        .unwrap_or(0);
    let graded: Vec<q::CandidateGrades> = candidates
        .iter()
        .map(|(flips, answers, grades)| q::CandidateGrades {
            flips: flips.clone(),
            answers: answers.clone(),
            grades: grades.clone(),
        })
        .collect();
    let rules = q::Rules {
        upgrade10: true,
        upgrade11: true,
    };
    let committees =
        q::build_flip_committees(total_flips, &graded, rules).expect("the assignment is malformed");
    let verdicts: Vec<q::FlipVerdict> = covered_sorted
        .iter()
        .map(|flip| {
            let verdict = q::qualify_flip(&committees[*flip as usize], rules);
            q::FlipVerdict {
                flip: *flip,
                status: verdict.status,
                answer: verdict.answer,
                grade: verdict.grade,
            }
        })
        .collect();
    let decision = q::check_bundle_eligibility(&policy, &verdicts, &covered_sorted)
        .expect("the policy is unsupported");
    assert!(
        decision.eligible,
        "a covered flip fails the eligibility policy"
    );

    // 4. The claim, in the layout the escrow reads.
    let inventory_commitment = inventory_commitment(&author, epoch, &manifest_cids_bytes(&manifest_cids));
    let manifest_commitment =
        manifest_commitment(&network, &consensus, &policy, &inventory_commitment, &anchor);
    let key_commitment = fod_registration_core::secrets_commitment(&secrets);
    let mut policy_digest = [0u8; 32];
    policy_digest.copy_from_slice(&Sha256::digest(policy.as_bytes()));
    let mut consensus_digest = [0u8; 32];
    consensus_digest.copy_from_slice(&Sha256::digest(consensus.as_bytes()));

    let mut journal = Vec::with_capacity(JOURNAL_BYTES);
    journal.extend_from_slice(&version.to_be_bytes());
    journal.extend_from_slice(&contract);
    journal.extend_from_slice(&author);
    journal.extend_from_slice(&epoch.to_be_bytes());
    journal.extend_from_slice(&(covered_sorted.len() as u32).to_be_bytes());
    journal.extend_from_slice(&manifest_commitment);
    journal.extend_from_slice(&key_commitment);
    journal.extend_from_slice(&policy_digest);
    journal.extend_from_slice(&consensus_digest);
    assert_eq!(journal.len(), JOURNAL_BYTES, "the journal has the wrong size");
    env::commit_slice(&journal);
}

fn manifest_cids_bytes(cids: &[String]) -> Vec<Vec<u8>> {
    cids.iter().map(|cid| cid.as_bytes().to_vec()).collect()
}
