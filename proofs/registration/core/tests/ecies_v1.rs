//! The registration side of the shared construction vector: this crate has to
//! recover the fixture's plaintext, refuse a tampered or wrongly keyed message,
//! and compute the CID the manifest binds.

use fod_registration_core as core;
use serde_json::Value;

fn vector() -> Value {
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../vectors/ecies-v1.json");
    let raw = std::fs::read_to_string(&path).expect("read the shared vector");
    serde_json::from_str(&raw).expect("parse the shared vector")
}

fn field<'a>(vector: &'a Value, section: &str, name: &str) -> &'a str {
    vector[section][name]
        .as_str()
        .unwrap_or_else(|| panic!("the vector has no {section}.{name}"))
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

#[test]
fn the_registration_side_recovers_the_vector() {
    let vector = vector();
    let secret = decode(field(&vector, "ecies", "secret"));
    let ephemeral = decode(field(&vector, "ecies", "ephemeral"));
    let plaintext = decode(field(&vector, "ecies", "plaintext"));
    let ciphertext = decode(field(&vector, "ecies", "ciphertext"));

    let recovered = core::decrypt_from(&secret, &ciphertext).expect("the vector decrypts");
    assert_eq!(hex(&recovered), field(&vector, "ecies", "plaintext"));
    assert_eq!(recovered, plaintext);
    assert_eq!(ephemeral.len(), core::EPHEMERAL_BYTES);
    assert_eq!(
        ciphertext.len(),
        plaintext.len() + core::CIPHERTEXT_OVERHEAD
    );
}

#[test]
fn a_tampered_or_wrongly_keyed_message_is_refused() {
    let vector = vector();
    let secret = decode(field(&vector, "ecies", "secret"));
    let ciphertext = decode(field(&vector, "ecies", "ciphertext"));

    // One bit in the body, and one in the tag.
    let mut body = ciphertext.clone();
    let at = core::PUBLIC_KEY_BYTES + 16;
    body[at] ^= 0x01;
    assert!(
        core::decrypt_from(&secret, &body).is_none(),
        "a changed body decrypted"
    );

    let mut tag = ciphertext.clone();
    let last = tag.len() - 1;
    tag[last] ^= 0x01;
    assert!(
        core::decrypt_from(&secret, &tag).is_none(),
        "a changed tag decrypted"
    );

    // A different secret scalar, and a truncated message.
    let mut other = [0u8; 32];
    other[31] = 9;
    assert!(
        core::decrypt_from(&other, &ciphertext).is_none(),
        "the wrong secret decrypted"
    );
    assert!(
        core::decrypt_from(&secret, &ciphertext[..100]).is_none(),
        "a truncated message decrypted"
    );
}

#[test]
fn the_registration_side_secrets_and_cid_match_the_vector() {
    let vector = vector();
    let mut author = [0u8; 20];
    author.copy_from_slice(&decode(field(&vector, "secrets", "author")));
    let mut public_part = [0u8; 32];
    public_part.copy_from_slice(&decode(field(&vector, "secrets", "publicPart")));
    let mut private_part = [0u8; 32];
    private_part.copy_from_slice(&decode(field(&vector, "secrets", "privatePart")));
    let epoch = vector["secrets"]["epoch"].as_u64().expect("epoch") as u16;

    let canonical = core::secrets_bytes(epoch, &author, &public_part, &private_part);
    assert_eq!(hex(&canonical), field(&vector, "secrets", "canonical"));
    assert_eq!(
        hex(&core::secrets_commitment(&canonical)),
        field(&vector, "secrets", "commitment")
    );
    assert_eq!(
        hex(&core::cid_v1_raw_sha256(
            field(&vector, "cid", "data").as_bytes()
        )),
        field(&vector, "cid", "cidV1RawSha256"),
        "the CID this crate computes is not the vector's"
    );
}
