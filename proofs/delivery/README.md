# Delivery proof program

The buyer-bound delivery statement of `SPEC.md` 5.6: the guest re-encrypts the
epoch secrets to the buyer's registered delivery key under delivery ECIES v1 and
commits a journal holding the trade context, the secrets commitment, and the
hash of the ciphertext it produced.

## The built-in vector

```bash
cd proofs/delivery
cargo run --release --bin delivery-host
```

This proves the fixed vector and writes `dist/delivery-v2.json`: the journal, the
ciphertext, the receipt material a contract-side verifier needs, and the
measurements of the run. Expect minutes, not seconds - the guest executes about
7.4 million cycles, which measured 16 minutes of wall-clock proving time on this
machine with a peak of roughly 8.9 GB resident memory. The vector is synthetic,
so the artefact is reproducible and the compatibility test has something to
decrypt.

The Groth16 backend runs its `stark2snark` step in Docker. With the daemon down
the run fails after the STARK phase with `docker returned failure exit code:
Some(1)`, which costs the whole pass - check `docker info` before starting one.

## A real order

```bash
RISC0_PROVER=ipc RISC0_DEV_MODE=0 cargo run --locked --offline --release --bin delivery-host -- \
  --order orders/example-order.json --no-witness --out dist/order-example-v2.json
```

`--order` proves the order in that file instead of the built-in vector, so a
seller can deliver against a funded order whose terms were never compiled into
this program. `--out` defaults to `dist/delivery-v2.json`, and `--order -` reads
the order from stdin.

`--check` validates an order file - hex lengths, the buyer key against the
buyer's secret when both are given, the derived ciphertext and its hash - and
prints the result without proving. It answers "is this file well formed" in
seconds. It is not evidence that a delivery statement holds.

`--no-witness` leaves the order's secret fields - the two private scalars, the
canonical secret bytes, the ephemeral, and the buyer's secret - out of the
artefact, so a seller can keep the journal, seal, ciphertext, and measurements
without also writing the witness to disk. The delivery desk (`fod serve`) passes
it and feeds the order on stdin.

### The order file

| Field | Meaning |
|---|---|
| `note` | Free text. Keep the synthetic marker on test files. |
| `contract` | The marketplace contract the journal must name (20 bytes, hex). |
| `network` | Canonical chain name (`idena`); the journal carries its SHA-256, not an action-file alias. |
| `funded_at`, `funding_root` | Included buy height and its authenticated header state root (32-byte hex). The checked-in example uses a synthetic anchor. |
| `offer_id`, `order_nonce` | The funded order's identity. |
| `seller` | The address that opened the offer (20 bytes, hex). |
| `buyer` | The funded buyer's address (20 bytes, hex). |
| `buyer_key` | The delivery key the order registered at funding (65 bytes, hex). |
| `buyer_secret` | Alternative to `buyer_key`: the key is derived from it. Test-only. |
| `epoch`, `author` | Which bundle's secrets these are. |
| `public_part_secret`, `private_part_secret` | The epoch's two private scalars (32 bytes each). |
| `ephemeral` | The scalar the seller uses for this delivery (32 bytes). |

Exactly one of `buyer_key` and `buyer_secret` is required. A real sale has only
the public key, because it is the buyer's secret; `buyer_secret` exists so a
test can decrypt the delivery afterwards.

An order file carries the prover's witness. Treat it like a key file: keep it
out of the repository, prefer `--order -` so nothing is written to disk, and
never reuse the example's values as real keys. `orders/example-order.json` is
synthetic and exists to be re-run.

### What the artefact holds

`journal` is the 281-byte statement the escrow re-checks; `seal` and
`claim_digest` are what its verifier is built against; `ciphertext` is the
delivery the buyer decrypts. `fulfill_argument` is what the seller submits:
offer id (8, big-endian) || journal (281) || Groth16 seal (256) || ciphertext.

The receipt is verified on the host with `receipt.verify(DELIVERY_GUEST_ID)`
before the artefact is written, and the journal fields are checked against the
order afterwards.

## Checks that use these artefacts

```bash
cd tools/fod && go test -count=1 -run TestDeliveryCiphertextDecryptsWithNodeLibrary .
```

decrypts both artefacts' ciphertexts with the repository-local ECIES library of
the pinned node - code the guest implementation never touches - checks the
secrets against the journal commitment, checks the published ciphertext against
the journal hash, and confirms an unrelated key cannot open the delivery. It
skips an artefact that has not been built.

```bash
cd spikes/harness && go test -count=1 -run TestEscrowFulfill .
```

deploys the escrow module in the Idena runtime, funds an order from each
artefact's terms, and settles it with that artefact's `fulfill_argument`, so a
settlement does not depend on terms compiled into the host. The `-count=1` is
not optional: `go test` caches by Go inputs and does not notice a rebuilt wasm.

The active v2 journal appends SHA-256 of the canonical network name `idena`,
the order's funding height and its canonical header state root. The executing
contract checks those fields against its network pin and its own node's
header. Missing headers and forks with different funding state roots fail closed. A later fork
sharing the funding anchor still relies on a common canonical chain view.
The action file's human-readable network alias does not replace this proof
binding. Historical v1 artifacts remain archived but are refused by the new
candidate; existing deployed contracts are not upgraded.

For a local, witness-free receipt, explicitly select the installed IPC prover:

The guest is built through the source-only, digest-pinned Docker builder in
`../guest-build.rs`; the native host and prover run locally. Docker is required
even when Cargo's host dependencies are cached. The builder uses Rust 1.91.1
and locked guest dependencies at `/src`, so moving the checkout cannot select
a different image. `../../release/check-proof-builds.mjs` checks two clean
source locations and both active receipts before release promotion.

```sh
RISC0_PROVER=ipc RISC0_DEV_MODE=0 cargo run --locked --offline --release \
  --bin delivery-host -- --no-witness --out dist/delivery-v2.json
```

The funding anchor only exists after inclusion, so this order-specific proof
cannot be precomputed in full before buying. Its minutes-long proving time is
not an immediate private-delivery guarantee; the existing cheap reveal path
has different exposure/availability assumptions. No proof verification is
removed to meet the deadline.
