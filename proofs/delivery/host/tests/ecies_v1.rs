//! The delivery side of the shared construction vector: this crate has to
//! reproduce the fixture's ciphertext, and its secrets encoding has to be the one
//! the fixture's commitment covers.

use fod_delivery_core as core;
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
fn the_delivery_side_reproduces_the_vector() {
    let vector = vector();
    let secret = decode(field(&vector, "ecies", "secret"));
    let public = decode(field(&vector, "ecies", "publicKey"));
    let ephemeral = decode(field(&vector, "ecies", "ephemeral"));
    let plaintext = decode(field(&vector, "ecies", "plaintext"));
    let ciphertext = decode(field(&vector, "ecies", "ciphertext"));

    assert_eq!(
        hex(&core::public_key_of(&secret).expect("public key")),
        field(&vector, "ecies", "publicKey"),
        "the vector's public key is not the one this construction derives"
    );
    assert_eq!(
        hex(&core::encrypt_to(&public, &plaintext, &ephemeral).expect("ciphertext")),
        field(&vector, "ecies", "ciphertext"),
        "this construction no longer reproduces the vector's ciphertext"
    );
}

#[test]
fn the_delivery_side_secrets_encoding_is_the_vectors() {
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
        field(&vector, "secrets", "commitment"),
        "the key commitment this construction derives is not the vector's"
    );
    // The ciphertext in the vector is the canonical bytes encrypted, which is
    // what the delivery proof does with them.
    assert_eq!(field(&vector, "ecies", "plaintext"), field(&vector, "secrets", "canonical"));
}
