//! The registration side of the blob vector: a blob built with the pinned
//! node's own ECIES and protobuf has to open with these secrets, and its CID has
//! to come back in the form a manifest carries.

use fod_registration_core as core;
use fod_registration_core::blob;
use serde_json::Value;

fn vector() -> Value {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../vectors/blob-v1.json");
    let raw = std::fs::read_to_string(&path).expect("read the blob vector");
    serde_json::from_str(&raw).expect("parse the blob vector")
}

fn field<'a>(vector: &'a Value, name: &str) -> &'a str {
    vector[name]
        .as_str()
        .unwrap_or_else(|| panic!("the vector has no {name}"))
}

fn decode(value: &str) -> Vec<u8> {
    (0..value.len())
        .step_by(2)
        .map(|at| u8::from_str_radix(&value[at..at + 2], 16).expect("hex"))
        .collect()
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

fn encoded_field(number: u8, value: &[u8]) -> Vec<u8> {
    let mut out = vec![(number << 3) | 2];
    let mut length = value.len();
    while length >= 0x80 {
        out.push((length as u8) | 0x80);
        length >>= 7;
    }
    out.push(length as u8);
    out.extend_from_slice(value);
    out
}

#[test]
fn the_registration_refuses_duplicate_singular_blob_fields() {
    let vector = vector();
    let original = decode(field(&vector, "blob"));
    let public_secret = decode(field(&vector, "publicPartSecret"));
    let private_secret = decode(field(&vector, "privatePartSecret"));
    let parsed = blob::parse_flip_blob(&original).expect("the node's blob parses");

    for (number, original_value) in [
        (1, parsed.identity_key.as_slice()),
        (2, parsed.public_part.as_slice()),
        (3, parsed.private_part.as_slice()),
    ] {
        // A conflicting trailing field previously left the proof reading the
        // original value while the node's decoder selected the trailing value.
        // Empty and identical duplicates must also fail, in either order.
        for value in [&[0x42][..], &[][..], original_value] {
            let duplicate = encoded_field(number, value);
            for prepend in [false, true] {
                let bytes = if prepend {
                    [duplicate.as_slice(), original.as_slice()].concat()
                } else {
                    [original.as_slice(), duplicate.as_slice()].concat()
                };
                let expected = blob::BlobError::DuplicateField(u64::from(number));
                assert_eq!(blob::parse_flip_blob(&bytes), Err(expected));
                assert_eq!(
                    blob::open_flip_blob(&bytes, &public_secret, &private_secret),
                    Err(expected)
                );
            }
        }
    }
}

#[test]
fn the_registration_preserves_unknown_fields_and_field_order() {
    let vector = vector();
    let original = decode(field(&vector, "blob"));
    let parsed = blob::parse_flip_blob(&original).expect("the node's blob parses");
    let public_secret = decode(field(&vector, "publicPartSecret"));
    let private_secret = decode(field(&vector, "privatePartSecret"));
    let mut bytes = encoded_field(3, &parsed.private_part);
    bytes.extend(encoded_field(4, b"extension"));
    bytes.extend(encoded_field(1, &parsed.identity_key));
    bytes.extend(encoded_field(4, b"another extension"));
    // Unknown varint, fixed64 and fixed32 fields remain supported.
    bytes.extend_from_slice(&[0x28, 7, 0x31, 0, 0, 0, 0, 0, 0, 0, 0, 0x3d, 0, 0, 0, 0]);
    bytes.extend(encoded_field(2, &parsed.public_part));

    assert_eq!(blob::parse_flip_blob(&bytes), Ok(parsed));
    let (public, private) = blob::open_flip_blob(&bytes, &public_secret, &private_secret)
        .expect("the extended blob opens");
    assert_eq!(hex(&public), field(&vector, "publicPartPlaintext"));
    assert_eq!(hex(&private), field(&vector, "privatePartPlaintext"));
}

#[test]
fn the_registration_refuses_ambiguous_or_node_invalid_fields() {
    let vector = vector();
    let original = decode(field(&vector, "blob"));
    // The node treats an unexpected wire type on fields 1-3 as unknown data;
    // refusing it here keeps proof input unambiguous and may be conservative.
    for malformed in [
        vec![0x08, 0],
        vec![0x11, 0, 0, 0, 0, 0, 0, 0, 0],
        vec![0x1d, 0, 0, 0, 0],
        vec![0],
        [vec![0x80; 9], vec![2]].concat(),
        vec![0x80, 0x80, 0x80, 0x80, 0x10, 0],
    ] {
        let bytes = [original.as_slice(), malformed.as_slice()].concat();
        assert_eq!(
            blob::parse_flip_blob(&bytes),
            Err(blob::BlobError::Malformed)
        );
    }
}

#[test]
fn the_registration_opens_the_node_built_blob() {
    let vector = vector();
    let blob = decode(field(&vector, "blob"));
    let public_secret = decode(field(&vector, "publicPartSecret"));
    let private_secret = decode(field(&vector, "privatePartSecret"));

    let (public, private) =
        blob::open_flip_blob(&blob, &public_secret, &private_secret).expect("the blob opens");
    assert_eq!(hex(&public), field(&vector, "publicPartPlaintext"));
    assert_eq!(hex(&private), field(&vector, "privatePartPlaintext"));

    // The structure the node writes: the identity key, then the two parts, each
    // a length-delimited protobuf field.
    let parsed = blob::parse_flip_blob(&blob).expect("the blob parses");
    assert_eq!(
        parsed.identity_key.len(),
        65,
        "the identity key is uncompressed"
    );
    assert!(!parsed.public_part.is_empty(), "the public part is missing");
    assert!(
        !parsed.private_part.is_empty(),
        "the private part is missing"
    );
}

#[test]
fn the_registration_computes_the_blobs_cid() {
    let vector = vector();
    let blob = decode(field(&vector, "blob"));
    assert_eq!(
        blob::blob_cid_multibase(&blob).expect("the vector's blob has a cid"),
        field(&vector, "blobCid")
    );
}

#[test]
fn the_registration_computes_the_vectors_secrets_commitment() {
    let vector = vector();
    let mut author = [0u8; 20];
    author.copy_from_slice(&decode(field(&vector, "author")));
    let mut public_part = [0u8; 32];
    public_part.copy_from_slice(&decode(field(&vector, "publicPartSecret")));
    let mut private_part = [0u8; 32];
    private_part.copy_from_slice(&decode(field(&vector, "privatePartSecret")));
    let epoch = vector["epoch"].as_u64().expect("epoch") as u16;

    let canonical = core::secrets_bytes(epoch, &author, &public_part, &private_part);
    assert_eq!(hex(&canonical), field(&vector, "secretsCanonical"));
    assert_eq!(
        hex(&core::secrets_commitment(&canonical)),
        field(&vector, "secretsCommitment")
    );
}

#[test]
fn the_registration_refuses_a_tampered_or_malformed_blob() {
    let vector = vector();
    let blob = decode(field(&vector, "blob"));
    let public_secret = decode(field(&vector, "publicPartSecret"));
    let private_secret = decode(field(&vector, "privatePartSecret"));

    // One flipped bit in the public part's ciphertext: the tag has to catch it.
    let mut tampered = blob.clone();
    let at = tampered.len() - 40;
    tampered[at] ^= 0x01;
    assert!(matches!(
        blob::open_flip_blob(&tampered, &public_secret, &private_secret),
        Err(blob::BlobError::Decryption) | Err(blob::BlobError::Malformed)
    ));

    // A wrong secret.
    let mut other = [0u8; 32];
    other[31] = 7;
    assert!(matches!(
        blob::open_flip_blob(&blob, &other, &private_secret),
        Err(blob::BlobError::Decryption)
    ));

    // Truncated, and not a protobuf message at all.
    assert!(matches!(
        blob::parse_flip_blob(&blob[..10]),
        Err(blob::BlobError::Malformed) | Err(blob::BlobError::MissingField(_))
    ));
    assert!(matches!(
        blob::parse_flip_blob(&[0xff, 0xff, 0xff]),
        Err(blob::BlobError::Malformed)
    ));
}
