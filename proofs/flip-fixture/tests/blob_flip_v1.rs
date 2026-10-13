//! The re-sealed blob has to be a flip this tree can read: it opens with the
//! recorded epoch secrets, its plaintext is the frozen flip vector byte for
//! byte, the version 2 content check accepts it, and its CID is the one the
//! session fixture's record names.

use serde_json::Value;

fn fixture(relative: &str) -> Value {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .join(relative);
    let raw = std::fs::read_to_string(&path).expect("read the fixture");
    serde_json::from_str(&raw).expect("parse the fixture")
}

fn hex(value: &str) -> Vec<u8> {
    let digits = value.strip_prefix("0x").unwrap_or(value);
    (0..digits.len())
        .step_by(2)
        .map(|at| u8::from_str_radix(&digits[at..at + 2], 16).expect("hex"))
        .collect()
}

#[test]
fn the_sealed_blob_opens_and_reads_as_a_flip() {
    let blob_vector = fixture("proofs/registration/vectors/blob-flip-v1.json");
    let flip_vector = fixture("proofs/flip/vectors/flip-v1.json");
    let text = |vector: &Value, name: &str| -> String {
        vector[name]
            .as_str()
            .unwrap_or_else(|| panic!("the vector has no {name}"))
            .to_string()
    };

    let blob = hex(&text(&blob_vector, "blob"));
    let (public, private) = fod_registration_core::blob::open_flip_blob(
        &blob,
        &hex(&text(&blob_vector, "publicPartSecret")),
        &hex(&text(&blob_vector, "privatePartSecret")),
    )
    .expect("the sealed blob opens with the recorded secrets");

    assert_eq!(public, text(&flip_vector, "publicPart").into_bytes());
    assert_eq!(private, text(&flip_vector, "privatePart").into_bytes());

    // The version 2 content check reads the parts the way the client's renderer
    // does: hex, RLP, four pictures and two orders.
    let mut public_bytes = std::vec![0u8; fod_flip_core::hex_bytes_len(&public).expect("hex")];
    fod_flip_core::decode_hex(&public, &mut public_bytes).expect("decode the public part");
    let mut private_bytes = std::vec![0u8; fod_flip_core::hex_bytes_len(&private).expect("hex")];
    fod_flip_core::decode_hex(&private, &mut private_bytes).expect("decode the private part");
    let flip = fod_flip_core::read(&public_bytes, &private_bytes).expect("a flip");
    assert_eq!(flip.images[0].len(), 15);
    assert_eq!(flip.orders[1].index(0), Some(2));

    // The CID the manifest and the session fixture's record carry.
    let cid = fod_registration_core::blob::cid_multibase(
        &fod_registration_core::cid_v1_raw_sha256(&blob),
    );
    assert_eq!(cid, text(&blob_vector, "blobCid"));
    let session = fixture("proofs/session-core/fixtures/session-bundle-vector.json");
    assert!(
        session["manifest"]
            .as_array()
            .expect("manifest")
            .iter()
            .any(|entry| entry.as_str() == Some(cid.as_str())),
        "the session fixture does not record this blob's CID"
    );
}
