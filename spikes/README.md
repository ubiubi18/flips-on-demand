# Spikes

Throwaway measurement code for the Package 1 feasibility gate. Nothing here is
part of the product; it exists to produce numbers.

## IAVL proof spike

Question (`SPEC.md` `O-15`, measured 2026-09-26): can a RISC Zero guest verify
the identity proofs an anchor bundle carries, and what does a shard's worth
cost?

```bash
cd spikes/iavl-proof
cargo test -p fod-iavl-proof-core        # real proofs, native, against their root
cargo run --release -p iavl-proof-host -- core/fixtures/bundle.json 64
```

The host prints the guest's cycle count; it refuses to measure a guest that
disagrees with the native verifier. The fixture comes from
`fod state-proof -proof-bundle` over a real datadir. Result: 12,453 cycles per
identity proof, 3.19 million cycles and four segments for 256 proofs, after two
negative results that are recorded in `docs/PROGRESS.md` (one input read per
field, and the plain `sha2` crate instead of the zkVM accelerator).

## Contract ABI spike

Question: does the real Idena contract runtime accept, meter, and execute a
Rust-produced module that implements the contract ABI?

Build (three steps; step 2 is required because the runtime gatekeeper rejects
bulk-memory operators, and Rust's precompiled `core` enables them):

```bash
cd spikes/contract-abi

# 1. compile
cargo build --release --target wasm32-unknown-unknown

# 2. lower memory.copy / memory.fill to MVP instructions
mkdir -p dist
wasm-opt --llvm-memory-copy-fill-lowering -O3 \
  target/wasm32-unknown-unknown/release/fod_abi_spike.wasm \
  -o dist/fod_abi_spike.mvp.wasm

# 3. check the two features the gatekeeper rejects
wasm2wat dist/fod_abi_spike.mvp.wasm -o /tmp/spike.wat
rg -n 'memory\.(fill|copy|init)|data\.drop' /tmp/spike.wat   # expect no matches
rg -n '(f32|f64)\.' /tmp/spike.wat                          # expect no matches
```

Tools: `wasm-opt` and `wasm2wat` come from `brew install binaryen wabt`.

Run it inside the real runtime (Rust binding, real gatekeeper and gas metering)
with a stubbed host environment:

```bash
cd spikes/harness
go test -count=1 -v ./...
```

`-count=1` matters: `go test` caches results by Go inputs, and the escalation
tests read the compiled `.wasm` at runtime, so a rebuilt contract would
otherwise be masked by a cached pass.

What the harness proves: the module is accepted or rejected by the runtime's
gatekeeper, the ABI exports are correct, and metered cost is reported.

### The same suite against the runtime mainnet executes

The release the network installs links an older runtime than the fork this
project measures by default (`SOURCES.md` F-90). To run the suite against the
released one - it is on the module proxy, but its static libraries are only
built for Linux - use the modfile beside the local one:

```bash
docker run --rm --platform linux/amd64 -v "$PWD/../..":/work -w /work/spikes/harness \
  -e 'GOFLAGS=-modfile=go.official.mod -mod=mod' golang:1.26.5 go test -count=1 ./...
```

Every test passes under both, and the registration call is metered identically:
195,478,684 wasm gas under mainnet's binding and under the pin. The runtime
difference is in the engine and its dependencies, not in what a contract is
charged.

What it does **not** prove: any product semantics. The host environment models
storage and per-address balances so a payout is observable, but signatures,
cross-contract calls, and proof verification are still stubs, and the modelled
balances are harness bookkeeping rather than node state. See `docs/PROGRESS.md`
for what the numbers do and do not support.

## BN254 / Groth16 cost spike

Question: what do arkworks BN254 operations cost inside the real runtime, and is
there room under the per-call gas ceiling for pairing-based verification?

```bash
cd spikes/groth16-verify
cargo build --release --target wasm32-unknown-unknown
mkdir -p dist
wasm-opt --llvm-memory-copy-fill-lowering -O3 \
  target/wasm32-unknown-unknown/release/fod_groth16_spike.wasm \
  -o dist/fod_groth16_spike.mvp.wasm

cd ../harness
GOTOOLCHAIN=go1.26.5 go test -v -run TestGroth16SpikeInRuntime ./...
```

Measured results and their interpretation are recorded in `docs/PROGRESS.md` and
`docs/SOURCES.md` F-17/F-18. Short version: about 40,300,000 wasm gas per BN254
pairing, against a 512,000,000 wasm gas per-call ceiling.

This is **not** a verifier: it runs pairings and scalar multiplications on curve
generators. There is no verification key, no proof, and no receipt handling.

## Genuine Groth16 verification

Generate real verification material (native, reproducible, fixed seed):

```bash
cd spikes/proofgen
cargo run --release          # writes ../groth16-verify/src/vector.rs
```

Then rebuild and run the verification cases in the runtime:

```bash
cd spikes/groth16-verify
cargo build --release --target wasm32-unknown-unknown
wasm-opt --llvm-memory-copy-fill-lowering -O3 \
  target/wasm32-unknown-unknown/release/fod_groth16_spike.wasm \
  -o dist/fod_groth16_spike.mvp.wasm

cd ../harness
GOTOOLCHAIN=go1.26.5 go test -v -run TestGroth16SpikeInRuntime ./...
```

`verify(0)` verifies the untampered proof (`{"verified":true}`), `verify(1)`
increments the public input and must return `false`. Measured cost:
152,194,393 wasm gas, about 30% of the per-call ceiling.

Scope: the proof is genuine but it is **not** a RISC Zero receipt. There is no
ImageID binding, no seal decoding, and no dev-mode rejection case, and the
verification material is embedded rather than passed as call arguments.

## RISC Zero verifier probe

Question: does the actual `risc0-groth16` verifier build for the contract target
and execute in the runtime?

```bash
cd spikes/r0-verifier-probe
cargo build --release --target wasm32-unknown-unknown
mkdir -p dist
wasm-opt --llvm-memory-copy-fill-lowering -O3 \
  target/wasm32-unknown-unknown/release/fod_r0_verifier_probe.wasm \
  -o dist/fod_r0_verifier_probe.mvp.wasm

cd ../harness
GOTOOLCHAIN=go1.26.5 go test -v -run TestR0VerifierProbeInRuntime ./...
```

Answer: yes. Built at 70,765 bytes and 33 memory pages, it deploys and runs, and
rejects an empty seal at 25,121,494 wasm gas (dominated by constructing the
verifying key from the embedded ceremony constants).

Scope: this does **not** verify a receipt. No real RISC Zero proof exists on this
machine yet, because the prover toolchain (`rzup`, `cargo-risczero`, `r0vm`) is
not installed.

## Real RISC Zero Groth16 receipt

Produce a genuine receipt (dev mode off) and verify it in the runtime:

```bash
export PATH="$HOME/.risc0/bin:$PATH"

# 1. prove (requires Docker; the prover runs in a container)
cd spikes/r0-receipt
RISC0_SKIP_BUILD_KERNELS=1 RISC0_DEV_MODE=0 cargo run --release -p host
#    -> writes dist/receipt.json (256-byte seal, claim digest, control root...)

# 2. embed the receipt material in the contract-side verifier
cd ../r0-receipt-verify
python3 generate_vector.py
cargo build --release --target wasm32-unknown-unknown
wasm-opt --llvm-memory-copy-fill-lowering -O3 \
  target/wasm32-unknown-unknown/release/fod_r0_receipt_verify.wasm \
  -o dist/fod_r0_receipt_verify.mvp.wasm

# 3. verify both cases in the real runtime
cd ../harness
GOTOOLCHAIN=go1.26.5 go test -v -run TestR0ReceiptVerificationInRuntime ./...
```

Expected: `verify(0)` stores `{"verified":true}` at 195,325,117 wasm gas, and
`verify(1)` (one flipped byte in the claim digest) stores `{"verified":false}`.

Environment notes: `RISC0_SKIP_BUILD_KERNELS=1` is needed because the Apple Metal
compiler is absent, and the Docker VM needs more than 8 GB for the Groth16
compression step.

Scope: the receipt material is embedded, and the claim digest comes from the
prover. Binding the claim to an expected ImageID inside the contract is not
implemented.

## Session header anchor spike

Answers `REVIEW.md` D1's anchor question: can a contract authenticate a
historical session header itself, with no oracle and no consensus change? The
runtime already imports `env.block_header(height)` and the node answers it from
its canonical chain, protobuf-encoded, at 500 wasm gas (`docs/SOURCES.md` F-96).
The spike parses that header in `no_std` Rust and refuses unless the height,
block hash, state root, identity root and seed match what the caller claims and
the block carries the `FlipLotteryStarted` flag.

```bash
cd spikes/header-anchor
cargo build --release --target wasm32-unknown-unknown
mkdir -p dist
wasm-opt --llvm-memory-copy-fill-lowering -O3 \
  target/wasm32-unknown-unknown/release/fod_header_anchor.wasm \
  -o dist/fod_header_anchor.mvp.wasm

cd ../harness
go test -modfile=go.local.mod -count=1 -run TestHeaderAnchor -v .
```

Expected: the 730-byte proposed fixture is accepted at 147,537 wasm gas, and a
wrong hash, root or seed, a missing height, a skip block, a block without the
lottery flag and three malformed encodings are all refused. The fixture is
encoded by the pinned node's own protobuf package with synthetic field values
(`spikes/header-anchor/fixtures/headers.json`); a captured real header replaces
it when one lands.

## Session-bundle evidence (moved into the proof tree)

The two-anchor evidence of `docs/SOURCES.md` F-97 was prototyped here and now
lives in `proofs/session-core`, where the version 2 program can use it: `iavl`
(the fork's single-leaf fold), `session` (the two anchored leaves, the record
decode and the bundle label) and `statement` (the version 2 journal and the
manifest commitment). It is a separate crate on purpose: adding modules to
`fod-registration-core` moves the pinned registration guest's image id, which
the host's sealed-artefact test catches.

```bash
cd proofs/session-core
cargo test --locked --offline
```

Expected: 13 tests pass. The fixture
(`proofs/session-core/fixtures/session-bundle.json`) is built with the pinned
node's `state.Identity`/`state.Global` encoders and the pinned IAVL fork, with
synthetic values; four sessions cover clean, contains-reported, weak and a
verdict leaf that still carries flips, and the adversarial tests refuse a
changed CID, a short or extended manifest, swapped roots, another author, a
wrong epoch and a wrong journal size. The recorded proofs are shallow because
the synthetic tree is small; the deep real path is covered by the IAVL spike
above.

The bundle itself comes out of a node datadir with the CLI:

```bash
cd tools/fod
GOFLAGS=-modfile=go.local.mod go build -o fod .
./fod session-bundle -datadir /path/to/node -inventory-height N -verdict-height N \
  -author 0x<20-byte address> -out session-bundle.json
```

The inventory height sits inside the epoch's flip phase (the record still lists
the flips) and the verdict height is that epoch's `ValidationFinished` block
(the flags are written and the flips cleared). Both have to fall inside the
datadir's retained state versions, which is about a hundred blocks.
