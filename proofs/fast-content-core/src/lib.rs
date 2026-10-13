//! Private-content check that can execute before a validation session.
//!
//! The caller still has to prove execution under a pinned guest image. This
//! library only defines the statement and checks one supplied witness.

#![no_std]

extern crate alloc;

use alloc::vec;
use alloc::vec::Vec;
use fod_fast_delivery_core::content::{open_copy, seal_copy, CopyContext};
use fod_fast_delivery_core::{
    buyer_content_key, commitment_from_witness, content_key_from_message, order_context,
    valid_point, Delivery, PointBytes,
};
use fod_registration_core::blob;
use fod_session_core::statement;
use k256::elliptic_curve::sec1::ToEncodedPoint;
use k256::PublicKey;
use sha2::{Digest, Sha256};
use sha3::Keccak256;

pub const STATEMENT_VERSION: u32 = 1;
pub const JOURNAL_BYTES: usize = 273;
pub const MAX_FLIPS: usize = 32;
const MAX_CID_BYTES: usize = 128;
const MAX_COPY_BYTES: usize = blob::MAX_FLIP_BYTES + 45;
const COPY_COMMITMENT_DOMAIN: &[u8] = b"fod:fast-content:copies:v1";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Refusal {
    Empty,
    TooManyFlips,
    BadCid,
    DuplicateCid,
    UnsortedCid,
    BadBlob,
    BadFlip,
    BadPoint,
    BadCopy,
    WrongBundle,
    WrongContract,
    WrongNetwork,
    WrongCount,
    WrongInventory,
    WrongAuthorKey,
    BadDelivery,
}

#[derive(Clone, Copy)]
pub struct Record<'a> {
    pub original_cid: &'a [u8],
    pub original_blob: &'a [u8],
    pub encrypted_copy: &'a [u8],
}

/// Bytes the buyer has downloaded before paying. The caller must obtain the
/// expected bundle id and contract from canonical chain state; a seller's
/// manifest alone cannot authenticate either one.
#[derive(Clone, Copy)]
pub struct PrefetchedRecord<'a> {
    pub original_cid: &'a [u8],
    pub copy_cid: &'a [u8],
    pub encrypted_copy: &'a [u8],
}

pub struct Input<'a> {
    pub network: &'a str,
    pub contract: &'a [u8; 20],
    pub author: &'a [u8; 20],
    pub epoch: u16,
    pub consensus: &'a str,
    pub policy: &'a str,
    pub public_secret: &'a [u8; 32],
    pub private_secret: &'a [u8; 32],
    pub message: &'a PointBytes,
    pub blind: &'a [u8; 32],
    pub records: &'a [Record<'a>],
}

/// The sealed proof commits to these fixed-width fields. The contract must
/// compare the policy and network digests to its pinned values, the author to
/// the caller, the original CID commitment to the public session leaves, and
/// the point/copy commitments to every order and displayed copy inventory.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Journal {
    pub network: [u8; 32],
    pub contract: [u8; 20],
    pub author: [u8; 20],
    pub epoch: u16,
    pub count: u16,
    pub consensus: [u8; 32],
    pub policy: [u8; 32],
    pub original_inventory: [u8; 32],
    pub copies: [u8; 32],
    pub point_commitment: PointBytes,
    pub original_keys: [u8; 32],
}

impl Journal {
    pub fn encode(&self) -> [u8; JOURNAL_BYTES] {
        let mut out = [0u8; JOURNAL_BYTES];
        let mut at = 0usize;
        for field in [
            &STATEMENT_VERSION.to_be_bytes()[..],
            &self.network,
            &self.contract,
            &self.author,
            &self.epoch.to_be_bytes(),
            &self.count.to_be_bytes(),
            &self.consensus,
            &self.policy,
            &self.original_inventory,
            &self.copies,
            &self.point_commitment,
            &self.original_keys,
        ] {
            out[at..at + field.len()].copy_from_slice(field);
            at += field.len();
        }
        out
    }

    pub fn decode(bytes: &[u8]) -> Option<Self> {
        if bytes.len() != JOURNAL_BYTES || bytes[..4] != STATEMENT_VERSION.to_be_bytes() {
            return None;
        }
        Some(Self {
            network: bytes[4..36].try_into().ok()?,
            contract: bytes[36..56].try_into().ok()?,
            author: bytes[56..76].try_into().ok()?,
            epoch: u16::from_be_bytes(bytes[76..78].try_into().ok()?),
            count: u16::from_be_bytes(bytes[78..80].try_into().ok()?),
            consensus: bytes[80..112].try_into().ok()?,
            policy: bytes[112..144].try_into().ok()?,
            original_inventory: bytes[144..176].try_into().ok()?,
            copies: bytes[176..208].try_into().ok()?,
            point_commitment: bytes[208..241].try_into().ok()?,
            original_keys: bytes[241..273].try_into().ok()?,
        })
    }
}

fn digest(bytes: &[u8]) -> [u8; 32] {
    Sha256::digest(bytes).into()
}

fn author_key_matches(blob_key: &[u8], author: &[u8; 20]) -> bool {
    // Idena stores the uncompressed transaction sender key in ProtoIpfsFlip.
    // Its address uses the last 20 bytes of Keccak-256 over the 64 coordinates.
    if blob_key.len() != 65 || blob_key[0] != 4 {
        return false;
    }
    let Ok(key) = PublicKey::from_sec1_bytes(blob_key) else {
        return false;
    };
    let uncompressed = key.to_encoded_point(false);
    let address = Keccak256::digest(&uncompressed.as_bytes()[1..]);
    address[12..] == author[..]
}

// The node's protobuf decoder uses the last occurrence of a singular field.
// The older blob reader uses the first. Refuse ambiguous blobs so the proved
// copy always contains the same encrypted parts that validators read.
fn unambiguous_flip_fields(bytes: &[u8]) -> bool {
    fn varint(bytes: &[u8], at: &mut usize) -> Option<u64> {
        let mut value = 0u64;
        for shift in (0..=63).step_by(7) {
            let byte = *bytes.get(*at)?;
            *at += 1;
            if shift == 63 && byte > 1 {
                return None;
            }
            value |= u64::from(byte & 0x7f) << shift;
            if byte & 0x80 == 0 {
                return Some(value);
            }
        }
        None
    }
    let mut at = 0usize;
    let mut seen = 0u8;
    while at < bytes.len() {
        let Some(tag) = varint(bytes, &mut at) else {
            return false;
        };
        let field = tag >> 3;
        let wire = tag & 7;
        if field == 0 || field > (1 << 29) - 1 {
            return false;
        }
        if (1..=3).contains(&field) {
            if wire != 2 || seen & (1 << (field - 1)) != 0 {
                return false;
            }
            seen |= 1 << (field - 1);
        }
        let end = match wire {
            0 => {
                if varint(bytes, &mut at).is_none() {
                    return false;
                }
                at
            }
            1 => at.saturating_add(8),
            2 => {
                let Some(len) = varint(bytes, &mut at) else {
                    return false;
                };
                let Ok(len) = usize::try_from(len) else {
                    return false;
                };
                at.saturating_add(len)
            }
            5 => at.saturating_add(4),
            _ => return false,
        };
        if end > bytes.len() {
            return false;
        }
        at = end;
    }
    seen == 0b111
}

fn check_structure(public_part: &[u8], private_part: &[u8]) -> Result<(), Refusal> {
    if public_part.is_empty()
        || private_part.is_empty()
        || public_part.len() + private_part.len() > blob::MAX_FLIP_BYTES
    {
        return Err(Refusal::BadFlip);
    }
    if fod_flip_core::read(public_part, private_part).is_ok() {
        return Ok(());
    }
    let public_len = fod_flip_core::hex_bytes_len(public_part).map_err(|_| Refusal::BadFlip)?;
    let private_len = fod_flip_core::hex_bytes_len(private_part).map_err(|_| Refusal::BadFlip)?;
    let mut public = vec![0u8; public_len];
    let mut private = vec![0u8; private_len];
    fod_flip_core::decode_hex(public_part, &mut public).map_err(|_| Refusal::BadFlip)?;
    fod_flip_core::decode_hex(private_part, &mut private).map_err(|_| Refusal::BadFlip)?;
    fod_flip_core::read(&public, &private)
        .map(|_| ())
        .map_err(|_| Refusal::BadFlip)
}

/// A fixed-order digest over original-CID to copy-CID pairs. The journal does
/// not hide or authenticate copy availability: a buyer must fetch and hash the
/// advertised bytes before paying.
pub fn copies_commitment(sorted_pairs: &[(&[u8], &[u8])]) -> Option<[u8; 32]> {
    if sorted_pairs.is_empty() || sorted_pairs.len() > MAX_FLIPS {
        return None;
    }
    let mut hash = Sha256::new();
    hash.update(COPY_COMMITMENT_DOMAIN);
    hash.update((sorted_pairs.len() as u16).to_be_bytes());
    let mut previous: Option<&[u8]> = None;
    for &(original, copy) in sorted_pairs {
        if original.is_empty()
            || copy.is_empty()
            || original.len() > MAX_CID_BYTES
            || copy.len() > MAX_CID_BYTES
            || previous.is_some_and(|prior| prior >= original)
        {
            return None;
        }
        hash.update((original.len() as u16).to_be_bytes());
        hash.update(original);
        hash.update((copy.len() as u16).to_be_bytes());
        hash.update(copy);
        previous = Some(original);
    }
    Some(hash.finalize().into())
}

/// Checks every prefetched byte against the registered journal before `buy`.
/// The content remains encrypted until the included delivery is verified.
pub fn verify_prefetched(
    journal: &Journal,
    expected_network: &[u8; 32],
    expected_contract: &[u8; 20],
    expected_bundle: &[u8; 32],
    records: &[PrefetchedRecord<'_>],
) -> Result<(), Refusal> {
    if journal.network != *expected_network {
        return Err(Refusal::WrongNetwork);
    }
    if journal.contract != *expected_contract {
        return Err(Refusal::WrongContract);
    }
    if digest(&journal.encode()) != *expected_bundle {
        return Err(Refusal::WrongBundle);
    }
    if records.is_empty()
        || records.len() > MAX_FLIPS
        || records.len() != usize::from(journal.count)
    {
        return Err(Refusal::WrongCount);
    }
    if !valid_point(&journal.point_commitment) {
        return Err(Refusal::BadPoint);
    }
    let mut pairs = Vec::with_capacity(records.len());
    let mut originals = Vec::with_capacity(records.len());
    for record in records {
        if record.encrypted_copy.len() > MAX_COPY_BYTES {
            return Err(Refusal::BadCopy);
        }
        let cid = blob::blob_cid_multibase(record.encrypted_copy).map_err(|_| Refusal::BadCopy)?;
        if cid.as_bytes() != record.copy_cid {
            return Err(Refusal::BadCopy);
        }
        pairs.push((record.original_cid, record.copy_cid));
        originals.push(record.original_cid);
    }
    if statement::inventory_commitment(&journal.author, journal.epoch, &originals)
        != journal.original_inventory
    {
        return Err(Refusal::WrongInventory);
    }
    if copies_commitment(&pairs) != Some(journal.copies) {
        return Err(Refusal::BadCopy);
    }
    Ok(())
}

/// Opens the prefetched set only after checking the delivery proof against the
/// exact funded-order context supplied from canonical state. The caller must
/// verify that the order is settled and that its bundle id belongs to this
/// contract; those facts cannot be inferred from a seller's files.
pub fn recover_prefetched(
    journal: &Journal,
    expected_network: &[u8; 32],
    expected_contract: &[u8; 20],
    expected_bundle: &[u8; 32],
    records: &[PrefetchedRecord<'_>],
    buyer_secret: &[u8; 32],
    buyer_point: &PointBytes,
    order_id: u64,
    order_nonce: u64,
    buyer_address: &[u8; 20],
    delivery: &Delivery,
) -> Result<Vec<(Vec<u8>, Vec<u8>)>, Refusal> {
    verify_prefetched(
        journal,
        expected_network,
        expected_contract,
        expected_bundle,
        records,
    )?;
    let context = order_context(
        expected_network,
        expected_contract,
        expected_bundle,
        order_id,
        order_nonce,
        buyer_address,
        buyer_point,
    );
    let key = buyer_content_key(
        buyer_secret,
        &journal.point_commitment,
        buyer_point,
        &context,
        delivery,
    )
    .ok_or(Refusal::BadDelivery)?;
    records
        .iter()
        .enumerate()
        .map(|(index, record)| {
            let parts = open_copy(
                &key,
                &CopyContext {
                    network: expected_network,
                    contract: expected_contract,
                    author: &journal.author,
                    epoch: journal.epoch,
                    index: index as u16,
                    original_cid: record.original_cid,
                },
                record.encrypted_copy,
            )
            .ok_or(Refusal::BadCopy)?;
            check_structure(&parts.0, &parts.1)?;
            Ok(parts)
        })
        .collect()
}

pub fn verify(input: &Input<'_>) -> Result<Journal, Refusal> {
    if input.records.is_empty() {
        return Err(Refusal::Empty);
    }
    if input.records.len() > MAX_FLIPS {
        return Err(Refusal::TooManyFlips);
    }
    let point_commitment =
        commitment_from_witness(input.message, input.blind).ok_or(Refusal::BadPoint)?;
    let key = content_key_from_message(input.message).ok_or(Refusal::BadPoint)?;
    let network = statement::network_digest(input.network);

    let mut pairs: Vec<(&[u8], Vec<u8>)> = Vec::with_capacity(input.records.len());
    let mut previous: Option<&[u8]> = None;
    for (index, record) in input.records.iter().enumerate() {
        if record.original_cid.is_empty()
            || record.original_cid.len() > MAX_CID_BYTES
            || record.original_blob.len() > blob::MAX_FLIP_BYTES
            || record.encrypted_copy.len() > MAX_COPY_BYTES
        {
            return Err(Refusal::BadCid);
        }
        if let Some(prior) = previous {
            if prior == record.original_cid {
                return Err(Refusal::DuplicateCid);
            }
            if prior > record.original_cid {
                return Err(Refusal::UnsortedCid);
            }
        }
        previous = Some(record.original_cid);
        let cid = blob::blob_cid_multibase(record.original_blob).map_err(|_| Refusal::BadBlob)?;
        if cid.as_bytes() != record.original_cid {
            return Err(Refusal::BadCid);
        }
        if !unambiguous_flip_fields(record.original_blob) {
            return Err(Refusal::BadBlob);
        }
        let parsed = blob::parse_flip_blob(record.original_blob).map_err(|_| Refusal::BadBlob)?;
        if !author_key_matches(&parsed.identity_key, input.author) {
            return Err(Refusal::WrongAuthorKey);
        }
        let public_part =
            fod_registration_core::decrypt_from(input.public_secret, &parsed.public_part)
                .ok_or(Refusal::BadBlob)?;
        let private_part =
            fod_registration_core::decrypt_from(input.private_secret, &parsed.private_part)
                .ok_or(Refusal::BadBlob)?;
        check_structure(&public_part, &private_part)?;
        let context = CopyContext {
            network: &network,
            contract: input.contract,
            author: input.author,
            epoch: input.epoch,
            index: index as u16,
            original_cid: record.original_cid,
        };
        let expected =
            seal_copy(&key, &context, &public_part, &private_part).ok_or(Refusal::BadCopy)?;
        if expected != record.encrypted_copy {
            return Err(Refusal::BadCopy);
        }
        let copy_cid =
            blob::blob_cid_multibase(record.encrypted_copy).map_err(|_| Refusal::BadCopy)?;
        pairs.push((record.original_cid, copy_cid.into_bytes()));
    }

    let originals: Vec<&[u8]> = pairs.iter().map(|pair| pair.0).collect();
    let copy_pairs: Vec<(&[u8], &[u8])> = pairs
        .iter()
        .map(|(original, copy)| (*original, copy.as_slice()))
        .collect();
    let copies = copies_commitment(&copy_pairs).ok_or(Refusal::BadCopy)?;
    Ok(Journal {
        network,
        contract: *input.contract,
        author: *input.author,
        epoch: input.epoch,
        count: input.records.len() as u16,
        consensus: digest(input.consensus.as_bytes()),
        policy: digest(input.policy.as_bytes()),
        original_inventory: statement::inventory_commitment(input.author, input.epoch, &originals),
        copies,
        point_commitment,
        original_keys: fod_registration_core::secrets_commitment(
            &fod_registration_core::secrets_bytes(
                input.epoch,
                input.author,
                input.public_secret,
                input.private_secret,
            ),
        ),
    })
}

#[cfg(test)]
mod tests {
    extern crate std;

    use super::*;
    use fod_fast_delivery_core::SellerKey;
    use k256::elliptic_curve::sec1::ToEncodedPoint;
    use k256::{ProjectivePoint, Scalar};
    use serde_json::Value;
    use std::string::String;

    fn from_hex(text: &str) -> Vec<u8> {
        text.as_bytes()
            .chunks_exact(2)
            .map(|pair| u8::from_str_radix(std::str::from_utf8(pair).unwrap(), 16).unwrap())
            .collect()
    }

    struct Fixture {
        original_cid: Vec<u8>,
        original_blob: Vec<u8>,
        encrypted_copy: Vec<u8>,
        author: [u8; 20],
        public_secret: [u8; 32],
        private_secret: [u8; 32],
        message: PointBytes,
        blind: [u8; 32],
    }

    impl Fixture {
        fn new() -> Self {
            let path = concat!(
                env!("CARGO_MANIFEST_DIR"),
                "/../registration/vectors/blob-flip-v1.json"
            );
            let fixture: Value =
                serde_json::from_str(&std::fs::read_to_string(path).unwrap()).unwrap();
            let field = |key: &str| -> String { fixture[key].as_str().unwrap().into() };
            let original_cid = field("blobCid").into_bytes();
            let original_blob = from_hex(&field("blob"));
            let author: [u8; 20] = from_hex(&field("author")).try_into().unwrap();
            let public_secret: [u8; 32] = from_hex(&field("publicPartSecret")).try_into().unwrap();
            let private_secret: [u8; 32] =
                from_hex(&field("privatePartSecret")).try_into().unwrap();
            let message: PointBytes = (ProjectivePoint::GENERATOR * Scalar::from(13u64))
                .to_affine()
                .to_encoded_point(true)
                .as_bytes()
                .try_into()
                .unwrap();
            let blind = Scalar::from(23u64).to_bytes().into();
            let key = content_key_from_message(&message).unwrap();
            let (public_part, private_part) =
                blob::open_flip_blob(&original_blob, &public_secret, &private_secret).unwrap();
            let encrypted_copy = seal_copy(
                &key,
                &CopyContext {
                    network: &statement::network_digest("idena"),
                    contract: &[0x11; 20],
                    author: &author,
                    epoch: 230,
                    index: 0,
                    original_cid: &original_cid,
                },
                &public_part,
                &private_part,
            )
            .unwrap();
            Self {
                original_cid,
                original_blob,
                encrypted_copy,
                author,
                public_secret,
                private_secret,
                message,
                blind,
            }
        }
    }

    #[test]
    fn real_flip_vector_binds_authenticated_copy() {
        let fixture = Fixture::new();
        let records = [Record {
            original_cid: &fixture.original_cid,
            original_blob: &fixture.original_blob,
            encrypted_copy: &fixture.encrypted_copy,
        }];
        let input = Input {
            network: "idena",
            contract: &[0x11; 20],
            author: &fixture.author,
            epoch: 230,
            consensus: "synthetic-consensus",
            policy: "fod:eligibility:v1",
            public_secret: &fixture.public_secret,
            private_secret: &fixture.private_secret,
            message: &fixture.message,
            blind: &fixture.blind,
            records: &records,
        };
        let journal = verify(&input).unwrap();
        assert_eq!(journal.count, 1);
        assert_eq!(journal.encode().len(), JOURNAL_BYTES);
        assert_eq!(Journal::decode(&journal.encode()), Some(journal));
        assert_eq!(
            journal.point_commitment,
            commitment_from_witness(&fixture.message, &fixture.blind).unwrap()
        );

        let copy_cid = blob::blob_cid_multibase(&fixture.encrypted_copy).unwrap();
        let prefetched = [PrefetchedRecord {
            original_cid: &fixture.original_cid,
            copy_cid: copy_cid.as_bytes(),
            encrypted_copy: &fixture.encrypted_copy,
        }];
        let bundle = digest(&journal.encode());
        let network = statement::network_digest("idena");
        assert_eq!(
            verify_prefetched(&journal, &network, &[0x11; 20], &bundle, &prefetched),
            Ok(())
        );
        let seller = SellerKey::from_witness(&fixture.message, &fixture.blind).unwrap();
        let buyer_secret: [u8; 32] = Scalar::from(17u64).to_bytes().into();
        let buyer_point: PointBytes = (ProjectivePoint::GENERATOR * Scalar::from(17u64))
            .to_affine()
            .to_encoded_point(true)
            .as_bytes()
            .try_into()
            .unwrap();
        let buyer_address = [0x22; 20];
        let context = order_context(
            &network,
            &[0x11; 20],
            &bundle,
            7,
            3,
            &buyer_address,
            &buyer_point,
        );
        let delivery = seller.deliver(&buyer_point, &context).unwrap();
        let recovered = recover_prefetched(
            &journal,
            &network,
            &[0x11; 20],
            &bundle,
            &prefetched,
            &buyer_secret,
            &buyer_point,
            7,
            3,
            &buyer_address,
            &delivery,
        )
        .unwrap();
        assert_eq!(
            recovered,
            vec![blob::open_flip_blob(
                &fixture.original_blob,
                &fixture.public_secret,
                &fixture.private_secret
            )
            .unwrap()]
        );
        assert_eq!(
            recover_prefetched(
                &journal,
                &network,
                &[0x11; 20],
                &bundle,
                &prefetched,
                &buyer_secret,
                &buyer_point,
                8,
                3,
                &buyer_address,
                &delivery,
            ),
            Err(Refusal::BadDelivery)
        );
        assert_eq!(
            verify_prefetched(&journal, &network, &[0x11; 20], &[0; 32], &prefetched),
            Err(Refusal::WrongBundle)
        );
        assert_eq!(
            verify_prefetched(&journal, &network, &[0x12; 20], &bundle, &prefetched),
            Err(Refusal::WrongContract)
        );

        let mut altered = fixture.encrypted_copy.clone();
        *altered.last_mut().unwrap() ^= 1;
        let tampered_prefetch = [PrefetchedRecord {
            encrypted_copy: &altered,
            ..prefetched[0]
        }];
        assert_eq!(
            verify_prefetched(&journal, &network, &[0x11; 20], &bundle, &tampered_prefetch),
            Err(Refusal::BadCopy)
        );
        let changed = [Record {
            encrypted_copy: &altered,
            ..records[0]
        }];
        assert_eq!(
            verify(&Input {
                records: &changed,
                ..input
            }),
            Err(Refusal::BadCopy)
        );
    }

    #[test]
    fn duplicate_protobuf_parts_cannot_change_what_the_node_reads() {
        let fixture = Fixture::new();
        assert!(unambiguous_flip_fields(&fixture.original_blob));
        for field in [1u8, 2, 3] {
            let mut ambiguous = fixture.original_blob.clone();
            ambiguous.extend_from_slice(&[field << 3 | 2, 1, 0]);
            assert!(!unambiguous_flip_fields(&ambiguous));
            let cid = blob::blob_cid_multibase(&ambiguous).unwrap();
            let records = [Record {
                original_cid: cid.as_bytes(),
                original_blob: &ambiguous,
                encrypted_copy: &fixture.encrypted_copy,
            }];
            assert_eq!(
                verify(&Input {
                    network: "idena",
                    contract: &[0x11; 20],
                    author: &fixture.author,
                    epoch: 230,
                    consensus: "synthetic-consensus",
                    policy: "fod:eligibility:v1",
                    public_secret: &fixture.public_secret,
                    private_secret: &fixture.private_secret,
                    message: &fixture.message,
                    blind: &fixture.blind,
                    records: &records,
                }),
                Err(Refusal::BadBlob)
            );
        }
        let mut out_of_range = fixture.original_blob.clone();
        // Field 2^29 exceeds protobuf's maximum valid field number. The node
        // rejects the whole blob, so a sealed proof must reject it as well.
        out_of_range.extend_from_slice(&[0x80, 0x80, 0x80, 0x80, 0x10, 0]);
        assert!(!unambiguous_flip_fields(&out_of_range));
        let cid = blob::blob_cid_multibase(&out_of_range).unwrap();
        let records = [Record {
            original_cid: cid.as_bytes(),
            original_blob: &out_of_range,
            encrypted_copy: &fixture.encrypted_copy,
        }];
        assert_eq!(
            verify(&Input {
                network: "idena",
                contract: &[0x11; 20],
                author: &fixture.author,
                epoch: 230,
                consensus: "synthetic-consensus",
                policy: "fod:eligibility:v1",
                public_secret: &fixture.public_secret,
                private_secret: &fixture.private_secret,
                message: &fixture.message,
                blind: &fixture.blind,
                records: &records,
            }),
            Err(Refusal::BadBlob)
        );
    }

    #[test]
    fn original_blob_identity_must_match_the_registered_author() {
        let fixture = Fixture::new();
        let parsed = blob::parse_flip_blob(&fixture.original_blob).unwrap();
        assert!(author_key_matches(&parsed.identity_key, &fixture.author));
        assert!(!author_key_matches(&[0u8; 65], &fixture.author));
        let records = [Record {
            original_cid: &fixture.original_cid,
            original_blob: &fixture.original_blob,
            encrypted_copy: &fixture.encrypted_copy,
        }];
        assert_eq!(
            verify(&Input {
                network: "idena",
                contract: &[0x11; 20],
                author: &[0x99; 20],
                epoch: 230,
                consensus: "synthetic-consensus",
                policy: "fod:eligibility:v1",
                public_secret: &fixture.public_secret,
                private_secret: &fixture.private_secret,
                message: &fixture.message,
                blind: &fixture.blind,
                records: &records,
            }),
            Err(Refusal::WrongAuthorKey)
        );
    }
}
