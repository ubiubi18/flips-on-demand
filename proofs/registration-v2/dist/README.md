`registration-v2.json` is the historical sealed proof for the original escrow
image. `direct-v2-synthetic.json` is the replacement direct-sale fixture for
the current guest build. Both use the default inputs of
`proofs/registration-v2/host` (`--session ../../session-core/fixtures/session-bundle-vector.json`,
`--blob ../../registration/vectors/blob-flip-v1.json`) name the placeholder
contract `0x1111111111111111111111111111111111111111` inside the journal. The
tests use them as separate frozen statement, image-id and encoding records.

Guest builds use the immutable Docker builder and source-only `/src` layout
in `proofs/guest-build.rs`. SDK 3.0.6 and the guest lockfile are retained.
Moving the checkout must reproduce the current direct-sale image; its release
gate compares two clean builds and verifies the genuine sealed artifact.
The historical fixture is verified against the original escrow's immutable
image pin. Recompiling to a different image requires a coordinated
receipt/contract/CLI tuple change; existing deployments cannot acquire a new
pin through an administrator.

A receipt for a deployed escrow has to name that escrow, so it is produced with
the same guest over the live inputs:

```sh
fod session-bundle -inventory-anchor <saved-inventory.json> \
  -verdict-anchor <saved-verdict.json> -author <author> \
  -nodekey <author-nodekey> -blob-dir <CID-named-encrypted-blobs> \
  -out <session-bundle.json> -blob-out <private-witness.json>
cargo run --release -- --prove \
  --contract <escrow address> \
  --session <session-bundle.json> \
  --blob <private-witness.json> \
  --out path/to/chain-bound.json
```

The CLI verifies every manifest CID against the encrypted bytes and checks
that the nodekey belongs to the author. The private witness contains the
author's epoch secrets; keep it on the key-holding host with mode 0600. The
host passes all manifest blobs to the unchanged guest in manifest order.

`fod prepare-registration` and `fod prepare-list` refuse a receipt whose
contract field does not match the `-contract` they are given, which is how the
difference shows up.
