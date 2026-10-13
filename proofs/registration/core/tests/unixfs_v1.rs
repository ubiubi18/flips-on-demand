//! The importer's own rule, checked against the node that runs it.
//!
//! The CIDs below are what a live mainnet node answered for these exact bytes
//! through `ipfs_cid`, recorded in `SOURCES.md` F-88; the bytes are a
//! deterministic pattern rather than a fixture, because a CID depends on them
//! exactly. They are the same vectors `tools/fod/unixfs_test.go` reproduces, so
//! the Go CLI and the guest cannot drift apart without one of them failing.

use fod_registration_core::blob;

/// The deterministic byte pattern the node was asked about.
fn pattern(len: usize) -> Vec<u8> {
    (0..len).map(|i| ((i * 31 + 7) % 251) as u8).collect()
}

#[test]
fn the_importer_layout_matches_the_nodes_own_answers() {
    let cases: [(usize, &str); 5] = [
        (
            262_144,
            "bafkreieiuj5mzeuqor24l4lmo2yumm6665yv4gq7ozzp2nrfmqrjyboosy",
        ),
        (
            262_145,
            "bafybeich6cqd6yabw3cku6edwmz4rvwe275nx2l6ootvc2bpbt5bdvarua",
        ),
        (
            524_288,
            "bafybeid5sthfzhlnifk5fvivasemgw2rzzfdp54rapknx7eaz5j4yoae3i",
        ),
        (
            524_289,
            "bafybeiag5snf6cdu2gfoobd6elsfvq3tsujp3gztgpvulhyzxo3ax6e2vy",
        ),
        (
            614_400,
            "bafybeihr2fddcxgyaodxyumttzp2zioqpjezbsynz7v3rwsrh3ogrhnliy",
        ),
    ];
    for (len, cid) in cases {
        let computed = blob::blob_cid_multibase(&pattern(len)).expect("a flip-sized blob");
        assert_eq!(computed, cid, "{len} bytes");
    }
}

#[test]
fn the_boundary_is_the_chunk_and_nothing_larger_is_claimed() {
    // One byte more than a flip may be, and one byte less than the cap: the
    // first has no CID this construction can produce, the second does.
    assert!(matches!(
        blob::blob_cid_multibase(&pattern(blob::MAX_FLIP_BYTES + 1)),
        Err(blob::BlobError::TooLarge)
    ));
    assert!(blob::blob_cid_multibase(&pattern(blob::MAX_FLIP_BYTES)).is_ok());
    assert!(matches!(
        blob::blob_cid_multibase(&[]),
        Err(blob::BlobError::Empty)
    ));
}
