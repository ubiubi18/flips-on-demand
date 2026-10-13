# Pinned Sources and Verified Facts

This file records the exact inputs this repository was derived from and which
statements in `SPEC.md` were verified against source in this session.

"Verified" means: read directly from the pinned local checkout listed below, at
the revision shown, in this session. It does not mean "tested on a live network".

## 1. Pinned inputs

| Input | Location | Revision / identity | Note |
|---|---|---|---|
| Implementation plan | pasted plan, supplied locally | sha256 `1fab9cdf246faa93c226ccbb46d21405cb984c41937a47b51ef5b6dc08add4ae`, 803 lines, 38572 bytes | Source of `SPEC.md`. The hash is the durable pin; the file itself is not part of this repository. |
| `idena-go` reference checkout | local checkout, wired through `go.work` | `9bc57040337fd19f45ba65323939cac8789b9efd` (2026-07-10) `Harden transport and persisted state handling` | Read-only reference. Worktree is **dirty** (`.github/workflows/*`, `.go-version`, `.tool-versions`, `Dockerfile`, `README.md`, `go.mod`, `go.sum`, `scripts/run-go-toolchain.sh`). Do not modify or clean. |
| `idena-wasm-binding` | local checkout, wired through `go.work` | `f7c71ce32d86c8ed26e74935265c95768b0230d0` (2026-07-10) `Refresh verified wasm artifacts and scan binaries` | Contract runtime binding. Matches the `go.mod` replace pin (see fact F-5). |
| `idena-wasm` (contract runtime source) | local checkout of the runtime source | `7e1138959f9f96f59d20efa11f2c27b134067541` | Revision recorded in `idena-wasm-binding/lib/ARTIFACTS_SOURCE` as the source of the shipped archives. Read through `git show` only; checkout left untouched. |
| Wasmer (runtime engine fork) | recorded in `idena-wasm-binding/lib/ARTIFACTS_SOURCE` | `1637dea03c0110f7dd800f2d9781193caf820074` | Pinned engine revision used to build the shipped archives. |
| Idena contract SDK (`idena-sdk-as`) | `IdenaAI/vendor/idena.social-contract/node_modules/idena-sdk-as` | `0.0.29` (with `idena-sdk-bindgen` / `idena-sdk-core` `0.0.29`) | Canonical contract SDK: AssemblyScript. Layout of the `Region` type read from its source. |
| AssemblyScript compiler | `IdenaAI/vendor/idena.social-contract/package.json` | `assemblyscript@0.19.23` | Compiler version the canonical contracts are built with. |
| Local wasm tooling installed for this work | Homebrew | `wabt` (wasm2wat, wasm-objdump), `binaryen` (wasm-opt) | Used to inspect modules and to lower bulk-memory operators. |
| IdenaAI | local checkout | `f2e0dc9d7aee38eac4615b010447b10189f43173` (2026-09-24) `Shuffle and submit scheduled flips automatically` | Reuse source for UI and transaction-serialization patterns. Do not modify. |
| `idena.social` contract vendor copy | `IdenaAI/vendor/idena.social-contract` | `f2e0dc9d7aee38eac4615b010447b10189f43173` | Same revision as IdenaAI HEAD. |
| `idena.social` UI vendor copy | `IdenaAI/vendor/idena.social-ui` | `f2e0dc9d7aee38eac4615b010447b10189f43173` | Same revision as IdenaAI HEAD. |

Upstream URLs observed in the reference checkouts: `idena-go` origin is
`https://github.com/ubiubi18/idena-go.git`. The canonical upstream project is
`https://github.com/idena-network/idena-go`.

## 2. Toolchain measured on this machine (2026-09-25)

| Tool | Version | Status |
|---|---|---|
| Go | `go1.26.1 darwin/arm64` | present |
| Rust / Cargo | `1.96.1` (`31fca3adb 2026-06-26`) | present |
| Rust targets installed | `aarch64-apple-darwin` only | no wasm target installed yet |
| Node | `v24.18.0` | present |
| npm | `11.16.0` | present |
| Python | `3.13.15` | present |
| Docker | `29.2.1` | present |
| `rzup` (RISC Zero installer) | not installed | required for Package 1 |
| `cargo-risczero` | not installed | required for Package 1 |

Runtime build pin recorded in `idena-wasm-binding/lib/ARTIFACTS_SOURCE`:
`idena-wasm` revision `7e113895...`, Wasmer revision `1637dea0...`, built with
`rust_toolchain=1.97.0`, workflow run `29082016126`.

Consequence for Package 1: the RISC Zero toolchain must be installed before any
verifier work starts, and the exact installed version must be recorded here when
it is.

## 3. Verified protocol facts

### F-1 Flip encryption keys are derived per epoch from the identity key

Source: `idena-go/core/flip/flipper.go` lines 271-287
(`generateFlipEncryptionKey`).

```text
public  seed = "flip-key-for-epoch-<epoch>"
private seed = "flip-private-key-for-epoch-<epoch>"
hash        = common.Hash(crypto.Hash(seed))
signature   = secStore.Sign(hash.Bytes())
seed bytes  = first 40 bytes of signature
k           = big-endian(seed bytes) mod (curve order - 1) + 1
key         = ECDSA(k) -> ecies.ImportECDSA
```

Implications recorded in `SPEC.md`:

- Keys are a deterministic function of the identity key and the epoch number,
  which is what makes historical reconstruction possible without the original
  ceremony state.
- Public and private encryption keys are **two different** derived keys, not a
  single pair.
- The CLI derivation in Package 3 must reproduce exactly this construction,
  including the hash input and the signing step, and must be checked against
  independent vectors.

### F-2 Flip payload, CID, and submission binding

Source: `idena-go/core/flip/flipper.go` lines 57-71 (`IpfsFlip` protobuf
`PubKey`, `PublicPart`, `PrivatePart`), lines 118-147, lines 151-160.

- Stored blob = protobuf `ProtoIpfsFlip{PubKey, PublicPart, PrivatePart}`.
- CID = IPFS CID over the marshalled blob bytes.
- The `flipSubmit` transaction attachment carries a CID; the node rejects the
  submission when `tx cid != computed cid`.
- Blob size is checked against `common.MaxFlipSize`.

Implication: the spec's requirement that "original blob bytes match their
authenticated CIDs" is checkable locally, because the CID is a content hash of
the exact bytes the seller must deliver.

### F-3 ECIES is used for public and private parts separately

Source: `idena-go/core/flip/flipper.go` lines 194-215 (`PrepareFlip`).

- `PublicPart` is ECIES-encrypted to the epoch **public** encryption key.
- `PrivatePart` is ECIES-encrypted to the epoch **private** encryption key.
- The library is the repository-local `crypto/ecies` package, not a generic
  third-party ECIES.

Implication: the delivery proof must pin this exact construction and verify it
against compatibility vectors. Libraries named "ECIES" are not interchangeable.

### F-4 `flip_getKeys` is ceremony-state dependent

Source: `idena-go/core/ceremony/ceremony.go` lines 1736-1763.

- `GetFlipKeys` returns an error unless the ceremony lottery has finished.
- The private key returned is a recipient-encrypted key package from the keys
  pool, indexed by the caller's identity and the flip author, not a plaintext
  historical owner key.

Implication: the plan's statement that this endpoint is not a generic
historical owner-key exporter is confirmed. Package 3 must derive keys locally
via F-1 instead.

### F-5 The contract runtime is an external Rust binding, not a generic Go WASM engine

Source: `idena-go/go.mod` line 45 and line 273; `idena-go/vm/wasm/wasm_env.go`
line 11.

```text
require github.com/idena-network/idena-wasm-binding v0.0.0-20260703133323-47b198d9b194
replace github.com/idena-network/idena-wasm-binding => github.com/ubiubi18/idena-wasm-binding v0.0.0-20260710092321-f7c71ce32d86
```

The replace pin resolves to the locally checked-out binding revision
`f7c71ce32d86...`, which is the revision recorded in section 1.

Implication for Package 1: verifier feasibility is a question about what this
binding's runtime (memory, instructions, gas metering, host functions) allows,
not about a stock Go WASM engine. The spike must run inside the binding's own
test surface.

### F-6 The minimum deployment stake applies only to embedded contracts

Source: `idena-go/blockchain/validation/validation.go` lines 726-752
(`validateDeployContractTx`).

```text
if codeHash is in embedded.AvailableContracts:
    require tx.Amount >= FeePerGas * 3_000_000   else InvalidDeployAmount
if len(attachment.Code) > 0 and not EnableUpgrade11:
    return InvalidPayload
if codeHash not embedded and len(attachment.Code) == 0:
    return InvalidPayload
```

Implications recorded in `SPEC.md` section 7.4:

- The 3,000,000-gas minimum stake is **not** a generic custom-contract
  requirement; it is gated on the embedded code-hash table.
- Custom contract code requires the upgrade-11 flag to be active.
- Termination of non-embedded contracts is separately gated
  (`validation.go` line 763, `EnableUpgrade11`).

This supports the plan's instruction not to invent a recurring stake fee, and
it also records the real deployment prerequisite that Package 5 must satisfy.

### F-7 Gas and storage limits exist and must be measured, not assumed

Source: `idena-go/vm/costs/costs.go` line 17 (`DeployContractGas = 200`);
`idena-go/vm/wasm/wasm_env.go` lines 197-240 (`NewWasmEnv`, `SetStorage`).

- `SetStorage` enforces `common.MaxWasmContractStoreKeyLength` and meters gas
  through `lib.GasMeter`.
- Numeric runtime limits (memory pages, instruction budget, code-size caps) were
  **not** found in the files inspected here; they live in the binding and must
  be measured in Package 1.

### F-8 Contract runtime limits and gas model (measured from the pinned runtime source)

Source: `idena-wasm@7e1138959f9f96f59d20efa11f2c27b134067541`, read via
`git show` from the checkout in section 1, together with the Go side in
`idena-go`.

| Property | Value | Source |
|---|---|---|
| Guest linear memory limit | 100 Wasm pages = 6,553,600 bytes (6.25 MiB); a module requesting more is rejected, and a missing maximum is set to this limit | `idena-wasm` `src/runner.rs:123`, `src/limiting_tunables.rs:16-35,44-63` |
| Gas metering | Per-operator metering middleware is active: each executed operator is charged `cost_function(operator)` | `src/runner.rs:116-118` |
| Per-operator cost | Flat `1`, with the detailed cost table commented out | `src/costs.rs` (`cost_function`) |
| Base deploy cost | `3,000,000` wasm gas | `src/costs.rs` `BASE_DEPLOY_COST` |
| Base call cost | `100,000` wasm gas | `src/costs.rs` `BASE_CALL_COST` |
| Gas scaling | `wasmGas = userGas * 100` | `idena-go/vm/costs/costs.go` (`n = 100`, `GasToWasmGas`) |
| Call gas budget | Derived from the transaction fee headroom: `(MaxFee - txFee) / gasCostPerGas`, not from a small fixed protocol cap | `idena-go/blockchain/blockchain.go:1797-1807` (`getGasLimit`) |

Consequence recorded for Package 1: paying `N` user gas buys up to `100 * N`
executed operators. A verifier that executes tens of millions of operators is
therefore an economic and wall-clock question, not an immediate hard rejection.

Scope caveat (important): these numbers come from the runtime revision recorded
in the local binding checkout, which is built from the `ubiubi18` fork of
`idena-wasm` and pinned through a `replace` directive in the local `idena-go`
fork. Whether mainnet currently runs that same runtime revision was **not**
verified here. The limits must be re-confirmed against the runtime revision that
mainnet actually executes before any conclusion is treated as mainnet-valid.

### F-9 Host-function surface contains no big-integer or pairing precompile

Source: `idena-wasm@7e113895...`, `src/imports.rs`.

Host functions available to a contract include: `write_to_contract`,
`set_storage`, `get_storage`, `remove_storage`, `block_timestamp`,
`block_number`, `block_seed`, `min_fee_per_gas`, `balance`, `network_size`,
`event`, `epoch`, `pay_amount`, `caller`, `original_caller`, `debug`, `abort`,
`panic`, `promise_result`, `create_call_function_promise`,
`create_deploy_contract_promise`, `promise_then`,
`create_transfer_promise`, `own_addr`, `contract_addr`,
`contract_addr_by_hash`, `own_code`, `code_hash`,
`create_read_contract_data_promise`, `create_get_identity_promise`,
`bytes_to_hex`, `block_header`, `keccak256`, `global_state`, `gas_limit`,
`gas_left`, `burn`, `ecrecover`.

There is **no** big-integer, elliptic-curve, or pairing precompile. The only
cryptographic primitive exposed is `keccak256`, plus `ecrecover` for secp256k1
signature recovery.

Consequence: a Groth16 verifier must implement field and curve arithmetic in
WASM instructions. This is the central Package 1 cost driver and must be
measured, not assumed.

### F-10 The gatekeeper disables bulk memory operations

Source: `idena-wasm@7e113895...`, `src/gatekeeper.rs:20,62`
(`allow_feature_bulk_memory_operations: false`).

Consequence: contract modules must avoid bulk-memory operators
(`memory.copy`, `memory.fill`, and related). Toolchain flags and post-build
validation must be chosen accordingly; a module that emits them will be
rejected rather than merely be slow.

### F-11 Licenses of the reuse sources, and what the pinned references do not grant

Verified by reading the license files in the reference checkouts:

| Source | License file | Copyright line |
|---|---|---|
| IdenaAI | `LICENSE` | `MIT License`, `Copyright (c) 2020 Idena` |
| `idena.social-contract` (vendored) | `LICENCE` | `MIT License`, `Copyright (c) 2025 N3CR0M4NC3R` |
| `idena.social-ui` (vendored) | `LICENCE` | `MIT License`, `Copyright (c) 2025 N3CR0M4NC3R` |
| `idena.social-contract` package metadata | `package.json` | `"license": "ISC"` (package metadata differs from the `LICENCE` file; resolve per file when reusing) |

The reuse sources above are permissive, so this repository can be licensed MIT
as a whole provided their notices are carried into reused files. The difference
between the `LICENCE` file (MIT) and `package.json` (`ISC`) in the vendored
contract means per-file notice checks are required when actual reuse begins.
Recorded in `THIRD_PARTY.md`.

The pinned reference checkouts are a different case. Read in this session
(`idena-go` and `idena-wasm-binding` from their local checkouts; `idena-wasm`
from the host's API, because this machine holds no checkout of it at that
revision):

| Source | Licence file | Content |
|---|---|---|
| `idena-go@9bc57040` | `LICENSE` | GNU LGPL `Version 3, 29 June 2007`. One file, no `COPYING`/`NOTICE`, no per-file `SPDX-License-Identifier` header anywhere in the checkout, no copyright-holder line, and no licence section in `README.md` |
| `idena-wasm-binding@f7c71ce3` | `LICENSE` | GNU LGPL `Version 3, 29 June 2007`, same text |
| `idena-wasm@7e113895` | none | No `LICENSE`, `COPYING`, or `NOTICE` file at that revision and no `license` key in `Cargo.toml` (`publish = false`), so that revision carries no licence grant |

Consequence: the pinned references are **not** reuse sources. Nothing is copied
from them into this repository, they are not vendored, and no binary that links
them is conveyed: `release/` holds the interface HTML and the escrow module,
both this repository's own. LGPL-3.0 defines a Combined Work with
corresponding-source and relink obligations for conveyance, and a revision with
no licence grant permits no conveyance at all. They stay read-only inputs from
which behavioural facts are recorded in section 3. If reuse ever becomes
necessary, it goes into its own component with the upstream licence text and
notice rather than into this MIT tree.

### F-12 Contract ABI: exports, Region semantics, and argument passing

Source: `idena-wasm@7e113895...` `src/runner.rs:514,529-531,622`,
`src/imports.rs:30-60`, `src/memory.rs:22-33,45-53,160-186`; confirmed
empirically by `spikes/contract-abi` (see `PROGRESS.md`).

| Property | Value |
|---|---|
| Required exports | `allocate`, `deploy`, `memory` (`runner.rs:514`), plus one export per method |
| Call dispatch | the host calls the export whose name equals the method name; `deploy` cannot be called directly, `_`-prefixed methods are callback-only |
| Argument passing | the host calls the guest's `allocate(len)`, then writes that argument's bytes into the returned Region; the method receives one i32 pointer per argument, and that pointer addresses a **Region**, not raw bytes |
| Region layout | 12 bytes `{ offset: u32, capacity: u32, length: u32 }` in host source; the AssemblyScript SDK names the same layout `{ offset, len, capacity }` and keeps both values equal |
| Region validity | host rejects `offset == 0`, `length > capacity`, and `capacity > u32::MAX - offset` |
| Return value | an i32 pointer to a Region; the host copies `length` bytes from `offset` |
| Return size cap | `MAX_RETURN_VALUE_SIZE = 64 * 1024` bytes (`imports.rs:20`) |
| Host module | all host functions are imported from module `env` |

### F-13 The canonical contract toolchain is AssemblyScript, but the ABI is reproducible from Rust

Source: `IdenaAI/vendor/idena.social-contract/package.json` (scripts call `asc`;
`assemblyscript@0.19.23`), its `asconfig.json` (extends
`idena-sdk-as/asconfig.json`, targets `build/release.wasm`), and
`node_modules/idena-sdk-as/asconfig.json` (`runtime: "stub"`, bindgen
transforms, `lib: ./assembly/json.lib.ts`).

Demonstrated in this repository: a `no_std` Rust `cdylib` built for
`wasm32-unknown-unknown` that exports `memory`, `allocate`, `deploy`, and method
exports, reads arguments as Regions, returns a Region, and calls the `env`
`set_storage` host function. The real runtime accepted and executed it; see
`spikes/contract-abi` and the measurements in `PROGRESS.md`.

Consequence: the proof verifier does not have to be written in AssemblyScript,
which matters because a pairing implementation would be impractical there. The
adapter that enables this is small - the spike module is under 3 KB.

### F-14 The gatekeeper rejects bulk-memory operators and floats; Rust needs a lowering step

Source: `idena-wasm@7e113895...` `src/gatekeeper.rs:20,62,72-80` (all optional
features disabled: floats, bulk memory, reference types, SIMD, exception
handling, threads). Confirmed empirically: a plain
`cargo build --release --target wasm32-unknown-unknown` module was rejected by
the live runtime with
`Gatekeeper: "Bulk memory operation detected: MemoryFill { mem: 0 }"`.

`-C target-feature=-bulk-memory` on the crate does not remove those operators,
because the precompiled Rust `core`/`compiler_builtins` linked into the module
carry the feature and LTO merges target features. Working recipe, verified here:
lower the module with
`wasm-opt --llvm-memory-copy-fill-lowering -O3 <in> -o <out>`, then check with
`wasm2wat` that no `memory.fill|memory.copy|memory.init|data.drop` and no
`f32.`/`f64.` operator remains.

### F-15 The per-call gas ceiling is block-sized, so the verifier budget is bounded but large

Source: `idena-go/blockchain/types/types.go:47,49,78-83`
(`MaxBlockGas = 3000 * 1024`, `MaxBlockGasUpgrade11 = 5000 * 1024`, and
`MaxBlockSize(enableUpgrade11)` returning the same values), plus
`idena-go/blockchain/validation/validation.go:236-250`, where a transaction's
derived gas `MaxFee / minFeePerGas` is rejected with `TooHighMaxFee` when it
exceeds that block constant.

| Quantity | Value |
|---|---|
| Max user gas per transaction | 3,072,000 (before upgrade 11) / 5,120,000 (with upgrade 11) |
| Max wasm gas per call | 100x user gas, so 307,200,000 / 512,000,000 |
| Implication | one maximum-size call can execute roughly 5.1e8 metered wasm operators, so a heavy verifier can consume an entire block's gas budget in a single transaction |

Whether upgrade 11 is active on mainnet, and what `FeePerGas` costs there, is
not verified here (see O-5, O-10).

### F-16 Measured cost of the real runtime executing a Rust contract

Produced by `spikes/harness` against the real runtime (gatekeeper and gas
metering active) with a stubbed host environment. Module:
`spikes/contract-abi/dist/fod_abi_spike.mvp.wasm`, 2,969 bytes, sha256
`57bf6d90807c879088e092acdbc86a7911449180ff48ce6422be709d46a56f8e`.

| Call | Wasm gas | Marginal cost |
|---|---|---|
| `deploy` | 3,000,034 | base deploy cost 3,000,000 plus 34 |
| `ping` (baseline call, tiny payload) | 100,034 | base call cost 100,000 plus 34 |
| `compute(0)` | 103,218 | includes one storage write and JSON build |
| `compute(1,000)` | 115,098 | 11,880 |
| `compute(10,000)` | 216,541 | 101,443 |
| `compute(100,000)` | 1,229,234 | 1,012,693 |

Derived: a 64-bit multiply/add/xor/shift loop body costs about **10.1 wasm gas
per iteration**, consistent with the flat one-gas-per-operator model. Results
were read back from contract storage, so this measures execution of the
contract's own arithmetic rather than trusting printed output.

Interpretation for Package 1: within the 512,000,000 wasm gas ceiling, about
5.0e7 iterations of that loop body fit. A BN254 pairing-based Groth16
verification involves far more work per verification than one such iteration,
so whether it fits remains an open measurement - it has to be built and run,
not estimated.

### F-17 Measured cost of arkworks BN254 work inside the contract runtime

Produced by `spikes/harness` (`TestGroth16SpikeInRuntime`) against the real
runtime with a stubbed host environment. Module:
`spikes/groth16-verify/dist/fod_groth16_spike.mvp.wasm`, 303,769 bytes,
sha256 `2704c0fe3b73d94b1b0071a66a0e8dfbf6dcb54956c517cf8b5deb513965eb13`,
declared memory 49 pages (3.1 MiB) of the 100-page cap, no bulk-memory and no
float operators after lowering. Curve library: `arkworks` 0.5.0
(`ark-bn254 0.5.0`, `ark-ec 0.5.0`, `ark-ff 0.5.0`).

| Call | Wasm gas | User gas (gas / 100) |
|---|---|---|
| `deploy` | 3,000,046 | 30,000 |
| `field(1,000)` - BN254 `Fr` squares | 1,411,803 | 14,118 |
| `field(10,000)` | 13,156,690 | 131,567 |
| `field(100,000)` | 130,592,703 | 1,305,927 |
| `mul(1)` - G1 scalar multiplication | 365,508 | 3,655 |
| `mul(100)` | 17,445,310 | 174,453 |
| `pairing(1)` - full BN254 pairing on generators | 40,299,766 | 402,998 |
| `pairing(2)` | 80,610,638 | 806,106 |
| `verify(0)` - genuine Groth16 proof, correct input | 152,194,393 | 1,521,944 |
| `verify(1)` - same proof, tampered public input | 152,214,933 | 1,522,149 |

Derived unit costs:

- One `Fr` multiplication: about **1,306 wasm gas**.
- One G1 scalar multiplication: about **174,000 wasm gas**.
- One BN254 pairing: about **40,300,000 wasm gas**.

Comparison with the per-call ceiling (F-15): a three-pairing Groth16 check would
be roughly 121,000,000 wasm gas, and a two-pairing batched check roughly
81,000,000 wasm gas, before public-input and deserialization work. Against the
512,000,000 wasm gas ceiling that leaves roughly a 4x margin; against the
307,200,000 ceiling that applies before upgrade 11, the margin is roughly 2.5x.

The measured end-to-end verification is higher than the three-pairing estimate:
**152,194,393 wasm gas** to deserialize a verifying key (296 bytes), a proof
(128 bytes), and a public input (32 bytes), then verify. That is about 30% of
the 512,000,000 wasm gas ceiling, or about 50% of the 307,200,000 ceiling that
applies before upgrade 11. The same proof with a public input incremented by one
returns `false`, so the rejection path is measured too.

What this does **not** establish:

- The proof is a genuine BN254 Groth16 proof over a trivial circuit (`c = a * b`),
  generated natively by `spikes/proofgen`. It is **not** a RISC Zero receipt:
  there is no ImageID binding, no seal decoding, and no dev-mode rejection test.
- The verifying key and proof are compile-time constants in the module. The
  deserialization cost is included in the measurement, but the path that carries
  them in as transaction arguments is not yet exercised.
- Wall-clock behavior under real block production is unmeasured; local execution
  of all of the above completed in well under a second.
- The ceiling assumes upgrade 11 semantics and a transaction that pays for the
  corresponding gas, which is a real cost per call at mainnet `FeePerGas`.

### F-18 Memory headroom for a verifier, measured rather than assumed

The arkworks module declares 49 wasm pages (3,142,144 bytes) of the 100-page
(6,553,600-byte) cap while raising the heap to 2 MiB, and it instantiated
successfully. A real verifier therefore has roughly 3.4 MiB of undeclared
headroom for proof, key, and input data - a real constraint, but not an
immediate blocker.

### F-19 The RISC Zero verifier crate builds for the contract target and runs there

Crate: `risc0-groth16 5.0.0-rc.1` (Apache-2.0 OR MIT), used with
`default-features = false`. The crate declares
`#![cfg_attr(not(feature = "std"), no_std)]` plus `extern crate alloc`, and it
has no separate verifier feature: verification is available in the default
(non-`std`) configuration, while `std`, `prove`, `cuda`, and `unstable` are the
only features listed.

Verifier API, read from `risc0-groth16-5.0.0-rc.1/src/verifier.rs`:
`Verifier::new(seal, control_root, claim_digest, bn254_control_id,
verifying_key)` decodes the seal, forms five public inputs (two halves of the
control root, two halves of the claim digest, and the BN254 control id), prepares
the verifying key and the inputs, and `Verifier::verify()` runs
`Groth16::<Bn254>::verify_proof_with_prepared_inputs`. The embedded ceremony
constants (alpha, beta, gamma, delta, IC0..IC5) come from
`risc0-ethereum/contracts/src/groth16/Groth16Verifier.sol`.

Measured in this repository (`spikes/r0-verifier-probe`, module
`dist/fod_r0_verifier_probe.mvp.wasm`, 70,765 bytes, 33 declared memory pages,
sha256 `6c5e910508433bc1fb6a6b9f6988c2b6bf0a744eb79ad093a75f5fd94b8660ce`):

| Property | Result |
|---|---|
| Builds for `wasm32-unknown-unknown` with no_std | yes |
| Survives the runtime gatekeeper after lowering | yes (no bulk-memory, no float operators) |
| Executes in the real runtime | yes - an empty seal is rejected, storing `{"seal_rejected":true,"code":3}` |
| Gas for `verifying_key()` plus a rejected `Verifier::new` | 25,121,494 wasm gas |

The 25.1M figure is dominated by constructing the verifying key from the
embedded ceremony constants on every call. A real contract should build the
prepared key once or embed it, so this is an upper bound for that step rather
than an unavoidable per-call cost.

What this does **not** establish: no real RISC Zero receipt has been verified.
Generating one needs the prover toolchain (`rzup`, `cargo-risczero`, `r0vm`),
which is absent from this machine. Dev-mode receipts are not Groth16 seals and
must be rejected; that rejection is also untested here.

### F-20 Toolchain pins for real RISC Zero Groth16 proving

Installed with `rzup 0.5.0` (official installer `https://risczero.com/install`,
Apache-2.0, verified by reading the script before running it):

| Component | Version |
|---|---|
| `rzup` | 0.5.0 |
| `cargo-risczero` | 3.0.6 |
| `r0vm` | 3.0.6 |
| RISC Zero Rust toolchain | 1.97.0 |
| `cpp` prover component | 2024.1.5 |
| Groth16 prover image | `risczero/risc0-groth16-prover:v2025-04-03.1` |
| `risc0-zkvm` (host) | 3.0.6 |
| `risc0-circuit-recursion` | 4.0.5 |
| `risc0-groth16` (verifier) | 3.0.5 (no 3.0.6 exists on crates.io; 3.0.5 is the version matched to the v3 ceremony) |
| `risc0-zkp` | 3.0.5 |

Three environment constraints were hit and resolved, all local and reversible:

1. Apple's Metal compiler is unavailable on this machine, so
   `risc0-build-kernel` cannot build GPU kernels. `RISC0_SKIP_BUILD_KERNELS=1`
   builds the CPU path instead.
2. Groth16 proving runs in Docker (`risc0-groth16-3.0.5/src/prove/docker.rs`
   invokes the prover image). Docker Desktop was installed but its daemon was
   not running; it was started.
3. The default 8.3 GB Docker VM memory was not enough - the prover container was
   killed with exit code 137. Raising `MemoryMiB` to 16384 in Docker Desktop's
   `settings-store.json` (16.7 GB VM) let the same proving run succeed.

### F-21 A real Groth16 receipt verifies inside the contract runtime

Produced by `spikes/r0-receipt` with `RISC0_DEV_MODE=0` (dev mode explicitly
off, so this is a genuine Groth16 seal, not a fake receipt):

| Field | Value |
|---|---|
| Seal | 256 bytes |
| Image ID | `466470aacb42f484a387f39069961904e49a56f3da9e1946b9472475c70b21b8` |
| Claim digest | `22c29908d2fcde95a452cb8058f52233e04aea9d5b7ce828b1cb54311446ed81` |
| Control root | `a54dc85ac99f851c92d7c96d7318af41dbe7c0194edfcc37eb4d422a998c1f56` |
| BN254 control id | `c07a65145c3cb48b6101962ea607a4dd93c753bb26975cb47feb00d3666e4404` |
| Journal | `2a000000` (4 bytes; the guest's committed `u32` value 42) |

Verified inside the real contract runtime by `spikes/r0-receipt-verify`
(module `dist/fod_r0_receipt_verify.mvp.wasm`, 316,756 bytes, 33 declared memory
pages, sha256 `83ccc623555bb2b17828a842d2c3ceccefe5f477e42409ef8d5d1767293d81b1`):

| Call | Wasm gas | Stored result |
|---|---|---|
| `verify(0)` - the real receipt | 195,325,117 | `{"mode":0,"verified":true,"detail":"ok"}` |
| `verify(1)` - claim digest with one byte flipped | 195,352,445 | `{"mode":1,"verified":false,"detail":"invalid_proof"}` |

195.3M wasm gas is 38% of the 512M per-call ceiling (F-15), and the tampered case
shows the check rejects rather than accepting by construction.

Design consequence for the product, recorded rather than glossed over: this
spike supplies the claim digest from the prover. A contract that accepts
receipts must instead recompute it from the expected ImageID and the journal
digest inside the contract (SHA-256 over the RISC Zero tagged struct), and must
reject receipts whose claim does not match the expected program. That binding is
not implemented yet.

Dev-mode receipts are structurally excluded from this path - they carry no
Groth16 seal (`InnerReceipt::Fake`) - and the rejection is demonstrated where a
receipt enters the CLI: `AssembleRegistration` and `AssembleDelivery` refuse an
artefact whose seal is absent or not 256 bytes, so a development-mode receipt
cannot become a contract call (`prepare_registration_test.go`,
`prepare_delivery_test.go`).

### F-22 Historical evidence model: what must be reconstructed, and from where

Source: `idena-go` blockchain attachment and ceremony sources.

| Fact | Value | Source |
|---|---|---|
| Flip submission tx type | `submitFlip`, payload `FlipSubmitAttachment { Cid []byte, Pair uint8 }` | `blockchain/attachments/attachments.go:122-132`, `:154` |
| Flip deletion | `deleteFlip`, parsed by `ParseDeleteFlipAttachment` | `blockchain/attachments/attachments.go:307` |
| Long-session answers | on-chain: `LongAnswerAttachment { Answers, Proof, Key, Salt []byte }` | `blockchain/attachments/attachments.go:65-70`, `:107` |
| Short-session answers | on-chain commitment: `ShortAnswerAttachment` | `blockchain/attachments/attachments.go:50-54` |
| Flip status values | `NotQualified=0`, `Qualified=1`, `WeaklyQualified=2`, `QualifiedByNone=3` | `core/ceremony/types.go:33-36` |
| Qualification process | ingests short and long answers per sender and persists them per epoch | `core/ceremony/qualification.go:40-93` |

Consequence for Package 2: both halves of the evidence are reconstructible from
chain data - submissions from `submitFlip` transactions, and the left/right
consensus inputs from the on-chain long answers. The qualification rules must be
reproduced per `core/ceremony/qualification.go` and `reporters.go` rather than
invented.

Block retrieval detail: `bcn_blockAt` returns transaction **hashes**, not
transactions; bodies come from `bcn_transaction`. The paged `bcn_transactions`
method is per-address (`ReadTxs(address, count, token)`), so a complete epoch
sweep has to walk blocks.

### F-23 A frozen local node is an authoritative historical source, verified end to end

Source of the data: a locally installed Idena node (arm64 binary, `idena-go
version 1.1.2`) with its application-support datadir holding a 3.1 GB chain DB
and a 600 MB IPFS repo, last written June 2026. The datadir path is a local
setup detail and is not recorded here.

Procedure used here, chosen so that the user's node data is never written:

1. Copy-on-write clone of only `idenachain.db` and `ipfs` into a scratch datadir
   (`cp -c -R`, completing in about 0.2 s). The clone deliberately excludes
   `keystore` and `api.key`, so no key material was duplicated.
2. Start the node offline against the clone:
   `--datadir <clone> --rpcaddr 127.0.0.1 --rpcport 9019 --ipfsport 40415
   --ipfsportstatic --nodiscovery --apikey <scratch key>`.

Frozen state observed:

| Field | Value |
|---|---|
| Last block height | 10,860,138 |
| Last block hash | `0x93172fa5dcfc406fe45c7d617231cb03f2f76613e8bf5fc24bc8c088b0a52ca1` |
| Epoch at that height | 205 |
| Epoch 205 start block | 10,847,140 |
| Next validation (as of that epoch) | 2026-06-06 |
| Peers | none (`--nodiscovery`; the node reports itself synchronized) |

End-to-end path verified against that node - submission, payload decode, and blob
retrieval:

| Step | Result |
|---|---|
| `bcn_blockAt 10847251` | 2 transactions |
| `bcn_transaction` of the second tx | `submitFlip`, epoch 205, author `<author-address>`, 82-byte payload |
| Payload decode | `pair = 6`, CID = 36 bytes (CIDv1, sha2-256) = `bafkreihmf6xrvfh26yymneankpe6ffuyqj7nvakxnlc6vguodzoqe2jiai` |
| `ipfs_get` for that CID | 35,878 bytes of flip blob returned by the node |

Also observed: `flip_get` returns null on this offline node. The flip API is
ceremony-scoped, so historical blobs must be fetched by CID from IPFS rather than
through it.

Open question carried into the next step of Package 2: mapping answers back to
flips requires each epoch's lottery assignment (which participant was assigned
which flip). That assignment is derived from the epoch's candidate identities
and the lottery seed; whether it can be fully reconstructed from chain data plus
the node's own state snapshots is not yet established.

### F-24 Local node history is not complete: measured tx-body availability

Two local datadirs were tested with the same arm64 node binary (`idena-go
1.1.2`), each through a copy-on-write clone and the tool in `tools/fod-evidence`
(range 10,847,140 to 10,860,138, epoch 205, 12,999 blocks).

| Source | Frozen height | Blocks served | Transactions referenced | Bodies available | Bodies missing |
|---|---|---|---|---|---|
| IdenaAI datadir (3.1 GB chain DB) | 10,860,138 | 12,999 | 349 | 177 | 172 |
| `idena-local-node` datadir (8.8 GB chain DB) | 10,998,672 | 12,999 queried | 0 | 0 | returns `null` for old blocks, e.g. `bcn_blockAt 10847251` |

Availability on the IdenaAI datadir is not uniform across the epoch:

| Height range | Bodies available |
|---|---|
| 10,846,000 - 10,847,999 | 131 of 131 (100%) |
| 10,848,000 - 10,849,999 | 19 of 56 (34%) |
| 10,850,000 - 10,857,999 | 0 of 121 (0%) |
| 10,858,000 - 10,859,999 | 25 of 39 (64%) |
| 10,860,000 - 10,861,999 | 1 of 1 (100%) |

The gap is behavioural, not a parsing artifact: for a missing hash the node's
`bcn_transaction` returns a null result with no error, and the block still lists
the hash. `tools/fod-evidence` therefore records missing bodies explicitly
(`missingTxBodies`, `missingTxHashes`, and `<missing-body>` entries in `txs`)
instead of treating them as empty transactions.

Peer-based retrieval of missing bodies is **untested**: the scratch clone was
started without the original app's peer hints and reports zero peers, so a
missing body stayed missing across retries.

Consequence for Package 2, recorded rather than assumed away: a complete
author-epoch inventory cannot be assumed to be reconstructible from any
available node. The CLI has to treat "body unavailable" as its own state, and an
offer must not claim inventory completeness it cannot substantiate. This is
directly load-bearing for gate G-2.

### F-25 Block headers commit to the transaction set and to the body, so inventory completeness is verifiable

Source: `idena-go/blockchain/types/types.go:114-131` (`ProposedHeader`),
`:1036-1076` (`Body.ToBytes`/`FromBytes`),
`blockchain/blockchain_test.go:2494-2495` (`ipfs.Add(body.ToBytes(), false)`).

| Header field | Meaning |
|---|---|
| `TxHash` | hash of the block's transaction hashes - commits to the transaction set |
| `IpfsHash` | IPFS CID of the block body (`Body.ToBytes()`) |
| `TxReceiptsCid` | IPFS CID of the transaction receipts |

Measured behaviour on the frozen datadir:

| Check | Result |
|---|---|
| Every non-empty sampled block carries an `ipfsCid` | yes (empty blocks have null) |
| `ipfs_get` of a block's body CID returns the body | yes - block 10,848,563 body, 78 bytes |
| Recomputing CIDv1/sha2-256 over the returned bytes reproduces the CID | yes |
| Peer-based retrieval of missing tx bodies | not testable here - the app's peer hints are unreachable (TCP connect failed to all three tested), `net_addPeer` accepted them, and the node stayed at 0 peers |

Consequence, and this resolves the F-24 blocker by design rather than by trust:
the complete transaction set of an epoch can be reconstructed from chain-
committed data alone. Walk the headers, fetch each body by its committed
`IpfsHash`, verify the bytes against that CID, decode with the reference
`Body.FromBytes`, and check the resulting set against the header's `TxHash`. A
node that fails to serve a transaction body then costs availability, not
correctness: the body is obtainable by CID from IPFS instead.

### F-26 Committed bodies recover submissions the node's transaction store omits

Measured with `tools/fod-evidence -body-source ipfs` over epoch 205 (blocks
10,847,140-10,860,138, 12,999 blocks), against the frozen IdenaAI datadir:

| Metric | Node transaction store (`bcn_transaction`) | Committed block bodies |
|---|---|---|
| Transactions enumerated | 177 of 349 referenced had bodies | 349 |
| Flip submissions found | 24 | 75 |
| Distinct authors | 10 | 24 |

Body verification for the same run:

| Check | Result |
|---|---|
| Non-empty blocks | 306 |
| Bodies fetched by committed CID | 306 |
| Bodies whose bytes hash to that CID | 306 (100%) |
| Blocks whose decoded transaction hashes equal the node's listed hashes | 306 (100%, zero mismatches) |

Interpretation: enumerating submissions from the node's transaction API
under-reports them by more than 3x on this datadir, silently. Enumerating from
the CID-committed bodies does not, and every body's bytes are bound to the CID by
hashing.

Trust-chain limit, stated plainly: the CIDs themselves come from the node's
block header representation, and that RPC does not expose the header's `TxHash`
or the header signature, so this run does not yet authenticate the header. What
it does establish is that two independent local paths (IPFS bodies and the
node's transaction listing) agree on every block, and that the body is bound to
the CID by content hashing. Authenticating the header itself - and thus making
the completeness argument independent of the node - is still open.

### F-27 Closed epochs are present; only the newest epoch is truncated

This corrects an earlier version of this entry, which claimed no local source
covered a closed epoch. Measured on the two local datadirs:

| Source | Head height | Head time | Block bodies | Closed epochs present |
|---|---|---|---|---|
| IdenaAI datadir | 10,860,138 | 2026-06-04 14:46 UTC | available via committed CID (F-26) | yes - epoch 204 and earlier |
| `idena-local-node` datadir | 10,998,672 | July 2026 | headers only: no transactions, no `ipfsCid`; null below ~10,847,251 | not usable |

The snapshot head is two days before epoch 205's validation, so epoch 205 itself
is unclosed and truncated. Every earlier epoch is complete, which is what
Package 2 needs; the earlier claim conflated "the current epoch is open" with
"no closed epoch is available".

Epoch 204, measured with `-body-source ipfs` over blocks 10,820,000-10,847,139:

| Metric | Value |
|---|---|
| Blocks swept | 27,140, of which 1,285 non-empty |
| Bodies fetched and CID-verified | 1,285 of 1,285 (100%) |
| Blocks whose transaction set matched the node's listing | 1,285 of 1,285 |
| Transactions decoded | 2,395 |
| Flip submissions | 437 from 126 authors |
| Flip deletions | 1, at height 10,846,707 |
| Submission window | heights 10,825,292 - 10,847,017 |
| `submitAnswersHash` | 147 transactions, heights 10,847,037 - 10,847,042 |
| `submitShortAnswers` | 147 transactions, height 10,847,044 |
| `submitLongAnswers` | 145 transactions, heights 10,847,046 - 10,847,127 |

The complete validation ceremony of a closed epoch - submission window, answer
hash, short answers, and long answers - is therefore on-chain and inside this
datadir. That removes the data blocker for the eligibility half of Package 2.

### F-28 Deleted flips keep their blob and free their pair slot

Measured with `fod-evidence -mode inventory` for author
`<author-address>`, epoch 204:

| Pair | Block | Deleted | Blob | CID verified |
|---|---|---|---|---|
| 0 | 10,846,619 | no | 35,815 bytes | yes |
| 2 | 10,846,667 | **yes** | 38,861 bytes | yes |
| 2 | 10,846,739 | no | 39,033 bytes | yes |

Two consequences that shape the bundle format:

1. The pair index is reused after a deletion: the same pair value appears twice,
   once deleted and once live. A bundle keyed only by pair index would collide, so
   the CID is the identity and the pair index is metadata.
2. The deleted flip's ciphertext is still retrievable and still hashes to its
   CID, which is exactly the case the spec requires the inventory to keep rather
   than drop.

### F-29 Long answers are bit-packed and decodable from chain data

Source: `idena-go/blockchain/types/types.go:1319-1330` (grades),
`:1359-1420` (`NewAnswersFromBits`, `Answer`, `determineGrade`).

For a candidate assigned `n` flips, the answers byte string is one big-endian
integer holding:

| Bits | Meaning |
|---|---|
| `[0, n)` | left flags |
| `[n, 2n)` | right flags |
| `[2n, 5n)` | three grade bits per flip |

Grades are `None=0`, `Reported=1`, `D=2`, `C=3`, `B=4`, `A=5`; values above 5
decode as `None`.

Verified on a real payload with the reference decoder
(`fod-evidence -mode decode-answers`), transaction
`0x8b5ab610...` in epoch 204, author `0xad0aa968...`:

| Field | Value |
|---|---|
| Answers | 12 bytes |
| Proof / key / salt | 129 / 32 / 32 bytes |
| Inferred flips | 19 |
| Votes | 12 left, 7 right, 0 reported |
| Grades | 18 D, 1 A |

Important limitation: the flip count is not carried in the payload. It comes from
the candidate's lottery assignment, so the count above is inferred from the
payload size (`5n <= 8 * bytes`) and is exact only when `5n` is a multiple of 8.
Qualification reproduction therefore still needs the assignment step, which
`core/ceremony/lottery.go` computes from public inputs (candidates, the epoch's
flips, and a seed).

### F-30 Lottery inputs: two are public, one lives in state

Source: `idena-go/core/ceremony/ceremony.go:434-448` (seed) and `:687-760`
(candidates), `core/ceremony/lottery.go:24,133` (assignment functions).

| Input | Where it comes from | Public? |
|---|---|---|
| Lottery seed | block seed at `flipLotteryStartHeight - LotterySeedLag` | yes, from the block header |
| Epoch flips | `submitFlip` transactions | yes, reconstructed and CID-verified (F-26) |
| Candidates with their flip lists, shard ids, and identity states | `appState.State.IterateIdentities`, filtered by `IsCeremonyCandidateData` | state, not transactions |

The epoch database stores the lottery identities only so a node can *restore* its
own ceremony (`getCandidatesAndFlips(restore)`); it is not a network-wide record.
Measured on the frozen clone with `fod-evidence -mode epoch-db`:

| Observation | Value |
|---|---|
| Keys in the chain database | 12,602,776 |
| Keys under any `epoch` prefix | 18 |
| Epoch keys present | epoch 205 only, e.g. `epoch 205` + `cid` + CID bytes |
| Lottery identities or seed stored | none - both reads returned empty for epochs 204 and 205 |

Consequence for Package 2's qualification step: the seed and the flips are
available from public chain data, but the candidate list - who validated, in
which shard, holding which flips - is state data. Reconstructing it standalone
means obtaining the state at the epoch's lottery height and authenticating it via
state proofs against the block header's state root, which the plan already lists
as required input ("state snapshots and proofs"). A node whose own identity
participated in the lottery also has it locally; this datadir's did not.

### F-31 Historical identity state is pruned, and the epoch store only has it if the node was live

Two independent measurements close the "read the candidate list from a node"
route:

| Observation | Evidence |
|---|---|
| The state tree keeps only the last 100 versions | `core/state/statedb.go:39` `MaxSavedStatesCount = 100`; `CommitTree` deletes older versions |
| A past state version is therefore gone | `fod-evidence -mode state-at -state-height 10847021` returned `version does not exist` on the frozen datadir |
| The epoch store holds lottery identities only if the node processed the lottery block live | the frozen datadir has 18 epoch-prefixed keys and no `li`/`ls` (F-30) |

The pruning constant is a compile-time value, not a configuration switch, so no
node retains the identity state at a past epoch's lottery height. A fast-synced
node also never processes old lottery blocks, which is why this datadir has no
lottery identities either.

Reference points measured for epoch 204 while tracing this:

| Item | Value |
|---|---|
| `FlipLotteryStarted` flag block | height 10,847,021 |
| Seed block (`- LotterySeedLag`, lag = 100) | height 10,846,921 |
| `ShiftedShardId()` | returns the stored `ShardId` (state field), not a value derived from the address |

Consequence: the candidate list and the author ordering must be rebuilt from
chain-derived data - the participants that submitted answer transactions, their
shard ids, their submitted flips, and the public seed - rather than read from a
node. That reconstruction is self-checkable: the decoded answer payloads encode
`5n` bits for `n` assigned flips, so a correct assignment must make the payload
length match the assigned flip count for every participant.

### F-32 The lottery seed is readable from the frozen database

Measured with `fod-evidence -mode seed-at` against the frozen clone, for epoch
204 (seed height = `FlipLotteryStarted` block 10,847,021 minus
`LotterySeedLag` = 100):

| Field | Value |
|---|---|
| Seed height | 10,846,921 |
| Block hash | `0x83b9bbbc112663fae92908c60350fd18eb4632b8cf11f55261baa94aac3a0829` |
| Seed | `81e5dfd53f82eee1933357eda9d3e2d85ce46a042110ea5bb158ba663ddb93c0` (32 bytes) |

Read path: `Repo.ReadCanonicalHash(height)`, `Repo.ReadBlockHeader(hash)`,
`header.Seed()`, with the database opened read-only. The seed is therefore
available offline from a datadir, with no node process and no network.

### F-33 Where the candidate list can come from, and why that is acceptable

The assignment needs an ordered candidate list per shard. F-30 and F-31 showed
that neither the epoch store (written only by a node that processed the lottery
block live) nor the state tree (100 versions retained) provides it
retroactively.

For the product this is not a dead end: a seller offering an author-epoch bundle
must have been online during that epoch to submit flips and take part in
validation, so that node's epoch store holds the lottery identities for that
epoch. The candidate list is available to the party with standing to make the
offer, which matches the plan's model of a local CLI running against the
seller's own node.

For the spike, the available datadir was synced after the fact and holds none.
Inferring the candidate list from chain data alone is incomplete by
construction, because candidates that never answered leave no on-chain trace.
It is still worth attempting, for one reason: the inference is falsifiable.
Assigned flip counts determine the answer payload length (`5n` bits for `n`
flips), so a candidate set that reproduces every participant's payload length is
positive evidence, and one that does not yields a concrete failure instead of a
vague claim.

### F-34 No local datadir contains a lottery candidate list

`fod-evidence -mode epoch-keys` lists the keys under the `epoch` prefix of a
chain database, grouped by epoch and sub-key. Run over every Idena datadir on
this machine:

| Datadir | Date | Epoch keys | Sub-keys present |
|---|---|---|---|
| IdenaAI | June 4 | 18 | `cid` only (epoch 205) |
| `idena-local-node` | July 6 | 32 | `cid` only (epoch 211) |
| Idena.archived | April 12 | 9 | `cid` only (epoch 195) |
| IdenaAI_Benchmarker | April 14 | 0 | - |
| IdenaAI-dev-IdenaAI | April 26 | 0 | - |
| IdenaAI-dev-IdenaAI_Benchmarker | April 26 | 0 | - |
| idena-desktop.archived | April 12 | 0 | - |

Every datadir that has epoch keys has only `cid` entries - the node's own flips.
None has the lottery records (`li`, `ls`), which are written only when a node
processes a `FlipLotteryStarted` block while live (F-30, F-31).

So the candidate list for any past epoch is unavailable on this machine by any
path tried: epoch store (absent), state tree (pruned to 100 versions), or
inference from chain data (shard membership is state, and the shard relocation
in `blockchain.go:998-1030` is a randomized process over prior state, not a
function of an address).

Getting it requires either a datadir from a node that was live during a closed
epoch, or replaying the chain to rebuild historical state. That is a data /
external-state dependency, not an implementation gap in this repository.

### F-35 Epoch key derivation implemented from the reference primitives

`tools/fod/keys.go` derives the two per-epoch key pairs exactly as the live
node does. The original node inherited Go 1.17's 40-byte `ecdsa.GenerateKey`
mapping. A modern `crypto.GenerateKeyFromSeed` call uses a different mapping,
so the protocol conversion is encoded directly rather than inherited from the
Go toolchain used to build this CLI:

| Step | Reference call |
|---|---|
| Identity key from a nodekey file | `crypto.LoadECDSA(path)` (geth-style file, the format `Config.NodeKey` writes to `<datadir>/keystore/nodekey`) |
| Hash the seed text | `common.Hash(crypto.Hash([]byte(seed)))` |
| Sign it with the identity key | `secstore.SecStore.Sign(hash)` |
| Turn the signature into a key pair | read 40 bytes, reduce modulo `N-1`, add one, then `crypto.ToECDSA` and `ecies.ImportECDSA` |

Verified by tests in `tools/fod/keys_test.go`:

| Test | Property |
|---|---|
| `TestProtocolKeyFromSeedMatchesLegacyNodeMapping` | the 40-byte protocol mapping matches an independent fixed scalar |
| `TestDeriveEpochKeysIsDeterministic` | same identity key and epoch always give the same pair; different epochs give different keys; the two epoch keys differ |
| `TestEpochKeysRoundTrip` | ECIES round trip for both parts, and the two epoch pairs are not interchangeable |
| `TestFlipBlobRoundTrip` | a blob in the node's wire format (`ProtoIpfsFlip`) parses back and both parts decrypt with the derived keys |

Also fixed while building this: the first version printed the derived private key
to stdout. That violates the spec's "do not log secrets" rule, so the summary now
carries only public keys, fingerprints, and the seed texts, and the private
halves are reachable only through an explicit `SecretBytes()` method intended for
the encrypted-vault path.

The epoch-230 capture supplies the independent live check: separately derived
keys opened and validated all five retained flips for `idenaai2` and all three
for `idenaai3`; crossing the two private witnesses was refused in both
directions. The nodekeys and derived secrets stayed on their key-holding host.

Closed on 2026-09-26, on the lab chain where this project holds the author's key
legitimately. The node encrypted a flip for itself: `flip_submit` at epoch 1 with
two 64-byte parts built the epoch keys through the node's own `Flipper`, encrypted
both parts, added the blob to its IPFS, and returned
`bafkreidoyaebqvlpzmaxoba6vm2v6b2miae5g67ky3lfta2vec4bkyrbd4`. Fetched back
through `ipfs_get` at 427 bytes, the blob opened with the keys this repository
derives from the same nodekey for the same epoch: `fod derive-keys` gave public
key `02eadfc7…` (fingerprint `2a7ee9f76847f0ad`), and `receive` returned the two
hosted bytes exactly. The vector is
`proofs/registration/vectors/node-flip-v1.json`, and it carries no nodekey: the
test reads the local lab key and skips where it is absent.

What this establishes, and what it does not: the two compositions agree - seed
text, hashed seed, signature, seeded key - between the node's own code and this
repository's, on a live node rather than on a shared helper. It is not a
mainnet-author vector, because no mainnet author's key is available here, and the
node's encryption ran on a lab chain whose only validator is this project's own
key.

### F-36 Encrypted vault for private key material

`tools/fod-evidence/vault.go` stores secrets with the `age` Go library
(`filippo.io/age`) using a scrypt recipient derived from a passphrase, as the
spec requires.

| Property | Value |
|---|---|
| File mode | `0600`, enforced with an explicit `Chmod` after writing |
| Empty passphrase | refused before any file is created |
| Plaintext in file | verified absent by test |
| Wrong passphrase | rejected by `age` with "incorrect passphrase" |

Passphrase handling (`passphrase.go`): read from the `FOD_PASSPHRASE` environment
variable when set, otherwise prompted with echo disabled through
`golang.org/x/term`. A passphrase is never accepted as a command-line flag,
because flags are visible to other processes on the machine.

Measured end to end with a synthetic nodekey:

| Step | Result |
|---|---|
| `-mode export-keys` | wrote a 763-byte age file, mode `-rw-------`, with a valid `age-encryption.org/v1` header |
| Leakage check | file contains neither the JSON field names nor any derived key hex |
| `-mode import-keys` | round-trips; prints only public keys, fingerprints, and secret lengths |
| Wrong passphrase | `identity did not match any of the recipients` |

Tests: `vault_test.go` covers the round trip, the file mode, the absence of
plaintext, rejection of a wrong passphrase, and refusal of an empty one.

### F-37 The CLI surface now matches the spec's command names

The tool lives in `tools/fod` and runs as `fod <command> [flags]`, with the
commands the spec names plus the technical ones used during the investigation:

| Command | Purpose |
|---|---|
| `sync` | walk a block range and write a sweep of public data (alias of `sweep`) |
| `inspect` | build one author-epoch inventory from a sweep (alias of `inventory`) |
| `receive` | decrypt a purchased flip blob using a vault |
| `derive-keys` | derive an author's epoch keys; prints public material only |
| `export-keys` / `import-keys` | write and read the encrypted vault |
| `seed-at`, `epoch-keys`, `state-at`, `epoch-db` | read-only datadir inspection |
| `decode-answers` | decode a long-answer transaction |

User-facing flags are `-epoch` and `-height`; the internal names they map to
remain for compatibility with the earlier flag style. `fod help` prints the
command list, and an unknown command fails with exit code 2.

Verified by building the binary and running `derive-keys`, `export-keys`,
`import-keys`, and `epoch-keys` against a synthetic nodekey and the frozen
datadir. `receive` is covered by tests rather than a shell run: `receive_test.go`
walks derive, encrypt, vault write, and decrypt, and asserts that a vault holding
the wrong epoch keys fails to open the blob.

### F-38 Escrow contract first slice runs in the contract runtime

`contracts/escrow` implements the escrow state machine over the ABI Package 1
validated, and `spikes/harness/escrow_test.go` drives it in the real runtime.

Measured, with the module lowered to 9,803 bytes and zero bulk-memory or float
operators:

| Step | Result |
|---|---|
| `seedOffer(100)` (test hook) | offer 1 created, gas 102,760 |
| `registerOffer` | refused: `registration_proof_not_implemented` |
| `fulfill` | refused: `delivery_proof_not_implemented` |
| `fundOffer` with the wrong amount | refused: `wrong_amount` |
| `fundOffer` with the exact price | funded, gas 103,991 |
| second `fundOffer` | refused: `offer_not_open` (one buyer per offer) |
| `refund` before deadline plus grace | refused: `refund_not_available` |
| `refund` after deadline plus grace | succeeded, gas 103,791 |
| second `refund` | refused: `offer_not_funded` (no double credit) |
| `accounting` | `{"offer":1,"escrowed":0,"callerCredit":100}` |

Two rules of the plan are enforced by construction rather than by convention:

1. The proof-dependent paths refuse. `registerOffer`, `fulfill`, and `withdraw`
   return explicit errors instead of accepting placeholder evidence or clearing a
   credit without paying it.
2. The test hook is absent from the release artefact. Building without
   `--features test-hooks` yields a module in which `seedOffer` does not appear;
   the harness checks the compiled module for that.

The accounting numbers above come from a seeded offer, which is unit-level
evidence about the state machine, not proof that a real purchase settles. The
release path stays unclaimed until Package 4 supplies the proofs.

### F-39 Credit totals, liquidity view, and cancellation rules verified

Extends F-38. The escrow now tracks total outstanding credits and reads its own
balance, so the invariant gate G-4 names is checkable inside the contract.

`env.balance()` semantics confirmed from the node: `vm/wasm/wasm_env.go Balance`
returns `getBalance(w.ctx.ContractAddr())`, i.e. the contract's own balance, not
the caller's.

Measured in the real runtime:

| Check | Result |
|---|---|
| `accounting` after fund and refund | `{"offer":1,"escrowed":0,"callerCredit":100,"creditTotal":100,"liquid":100}` |
| Liquidity invariant `liquid >= escrowed + creditTotal` | holds (100 >= 0 + 100) |
| `cancelOffer` on an unfunded offer | succeeded |
| `fundOffer` after cancellation | refused: `offer_not_open` |
| `cancelOffer` twice | refused: `offer_not_open` |

The credit total is tracked deliberately: a contract cannot iterate its storage,
so an aggregate is the only way to check total liabilities against the balance in
one call.

Also established for the next slice, the promise ABI for `withdraw`:

| Fact | Source |
|---|---|
| `create_transfer_promise(addrRegion, amountRegion)` returns nothing; it deducts the contract balance and occupies the next promise index | `idena-wasm src/imports.rs`, `idena-sdk-core host.ts` |
| `promise_then(promiseIdx, methodRegion, argsRegion, amountRegion, gasLimit)` attaches a callback | `src/imports.rs:334-360` |
| The callback method name must start with `_`; direct calls to such methods are rejected by the runtime | `src/runner.rs:622` |
| Callbacks read the result through `promise_result(statusPtr)`, with statuses 0 failed / 1 empty / 2 value | `idena-sdk-core env.ts:88-94`, `host.ts:6-8` |

The correct withdrawal shape that follows from this: zero the credit before
creating the transfer, then restore it in the callback when the promise failed,
so a failed transfer leaves a recoverable credit and a repeated withdrawal cannot
double-pay. Implementing it is the next slice; the harness must also be able to
drive promise completion before that path can be claimed as tested.

### F-40 Withdrawal reservation state machine, and the extended export check

Superseded later the same day by F-41: the reservation existed because a promise
callback was expected to report the transfer outcome, and in this runtime no
transfer callback is ever delivered. The measurements below were real, but they
describe a state machine that is no longer in the contract.

Extends F-39 with the slice F-39 named as next. `withdraw` reserves the caller's
credit, records it as pending, creates the transfer promise, and settles the
reservation in the shared result handler that the real callback and the test
hook both call.

Measured in the real runtime with `spikes/harness/escrow_withdraw_test.go`,
starting from a caller credit of 100:

| Step | Result |
|---|---|
| `withdraw` | `{"callerCredit":0,"creditTotal":100,"pendingTotal":100,"liquid":100}` |
| second `withdraw` while the first is pending | refused: `no_credit` |
| `simulateWithdrawResult(0)` (failed outcome) | `{"callerCredit":100,"creditTotal":100,"pendingTotal":0,"liquid":100}` |
| `withdraw` again, then `simulateWithdrawResult(2)` (successful outcome) | `{"callerCredit":0,"creditTotal":0,"pendingTotal":0,"liquid":100}` |
| modelled contract balance across all four steps | never negative |

The invariant `liquid >= escrowed + creditTotal` holds at every step, and
`creditTotal` is only decremented on success, so it over-counts liabilities
while a transfer is pending rather than under-counting them.

Promise arguments now get separate heap regions (`heap_region`,
`heap_region_utf16`). The first implementation pointed both arguments at the
shared value buffer, which the host reads in sequence during a single import
call, so the amount write overwrote the address bytes before they were read.
That was a defect in code written this session, found by inspection and fixed
before any test was attempted; the superseded helpers were removed.

Release artefact check extended from F-38's `seedOffer` case to the new hook.
Exports read back with `wasm-objdump -x`:

| Build | Function exports |
|---|---|
| `cargo build --release --target wasm32-unknown-unknown` (no features) | `_withdrawResult`, `accounting`, `allocate`, `cancelOffer`, `deploy`, `fulfill`, `fundOffer`, `memory`, `refund`, `registerOffer`, `withdraw` |
| same command with `--features test-hooks` (the harness artefact) | the same list plus `seedOffer` and `simulateWithdrawResult` |

Lowered sizes: harness artefact 16,502 bytes, no-feature build 14,666 bytes, both
with zero `memory.copy`/`memory.fill` and zero `f32`/`f64` operators, which the
gatekeeper requires. The no-feature build warns about four helpers that only
test-hook code reaches (`key_raw`, `NEXT_ID_PREFIX`, `next_offer_id`,
`set_next_offer_id`); `registerOffer` is still a refusal stub, so `next_offer_id`
has no production caller until Package 4.

Deliberately not claimed, and still in section 4: this harness does not
execute promises. The modelled balance is not debited by
`create_transfer_promise`, the runtime never dispatches `_withdrawResult`, and
`promise_result` is never read. What is verified is the reservation accounting
and the result handler, not the dispatch or the real balance movement.

### F-41 A transfer settles inside the call, so a withdrawal is atomic

F-40 assumed a promise callback would report the transfer result. Reading the
pinned runtime shows that assumption is wrong, and the correction removes the
callback from the payout path entirely.

| Fact | Source |
|---|---|
| `create_transfer_promise(addr, amount)` debits the contract's own balance itself, before the transfer is queued; an amount the contract cannot cover returns an error that aborts the call | `idena-wasm` revision 7e113895, `src/imports.rs` (`create_transfer_promise` calls `deduct_balance`) |
| Promises run at the end of the same top-level call, and the `Action::Transfer` arm credits the receiver there | same revision, `src/runner.rs` (`execute_promises`) |
| That arm never runs a callback, so a transfer callback is not delivered | same revision, `src/runner.rs`; the arm carries `//todo : handle result` |
| A failed call leaves no trace: the node commits only when the call succeeded | `idena-go` `vm/wasm/vm.go` (`if err == nil && vm.commitToState` guards `InternalCommit`) |
| The debited balance is the contract's own, and `add_balance` credits the receiver it is handed | `idena-go` `vm/wasm/wasm_env.go` (`SubBalance`, `AddBalance`) |

So `withdraw` consumes the credit and creates the transfer in one call: either
the whole credit leaves the contract and reaches the caller, or the call aborts
and the credit is still there. The reservation record, its aggregate, the
callback export, and the hook that simulated the callback were all removed
rather than left as dead weight.

Measured with the harness modelling per-address balances and the node's
commit-on-success rule (`spikes/harness/env.go`, `env.Snapshot`), in
`spikes/harness/escrow_withdraw_test.go`:

| Check | Result |
|---|---|
| `withdraw` with a credit of 100 and a contract balance of 100 | credit 0, `creditTotal` 0, accounting `liquid` 0 |
| contract balance after the payout | 0 |
| caller balance after the payout | 100 - the transfer executed inside the call |
| nested calls made by `withdraw` | none |
| second `withdraw` | refused: `no_credit` |
| `withdraw` while the contract holds only 50 of the 100 it owes | aborted: `backend error: insufficient funds` |
| state after the aborted withdrawal, model applied | credit 100, `creditTotal` 100, caller unpaid |
| the same credit after the contract is topped up to 100 | paid out in full |

Artefacts: the harness module lowers to 12,922 bytes and the release build to
11,797 bytes, both with zero `memory.copy`/`memory.fill` and zero `f32`/`f64`
operators. The release build exports `accounting`, `allocate`, `cancelOffer`,
`deploy`, `fulfill`, `fundOffer`, `memory`, `refund`, `registerOffer`,
`withdraw`; the test-hooks build adds only `seedOffer`.

A payout pays the runtime's `MoveBalanceGas` twice, once for the debit inside
`create_transfer_promise` and once for the credit in `add_balance`
(`idena-go` `vm/costs/costs.go`, `MoveBalanceGas = 30` contract gas).

### F-42 Buyer binding: the order stores a proven delivery key

Extends F-41. `fundOffer` binds the paying caller to an uncompressed secp256k1
delivery key by recovering the signer of a context-bound digest, and the offer
record now carries that key and an order nonce.

| Fact | Source |
|---|---|
| `ecrecover(data, signature)` returns the 65-byte uncompressed public key from a `[R || S || V]` signature and a zero region when recovery fails | `idena-wasm` revision 7e113895 `src/imports.rs`; `idena-go` `vm/wasm/wasm_env.go` calls `crypto.Ecrecover` |
| `keccak256(data)` is legacy Keccak-256 and costs `ComputeHashGas = 100`; `ecrecover` costs `EcrecoverGas = 300`; both are scaled by the node's wasm multiplier | `idena-go` `vm/costs/costs.go`, `vm/wasm/wasm_env.go` |
| A region the runtime wrote carries its byte count in `length`; `capacity` stays at the size the contract allocated | `idena-wasm` `src/memory.rs` `write_region`, which sets `length` only |

Measured in `spikes/harness/escrow_order_test.go`: the funded order records
`orderNonce` 1 and the key; proofs made by another key, bound to another payer,
or bound to another offer id are refused with `key_not_bound`; an empty key and
an empty signature are refused the same way; a short argument is refused with
`bad_args`; after a refund the nonce-1 proof is refused and a nonce-2 proof
funds the offer again with the new key stored.

`fundOffer` gas rose from 103,991 to 158,575 wasm gas: the two host charges
(10,000 for the hash, 30,000 for the recovery), the longer record, and the extra
storage write account for it. The harness charges those two host costs but still
charges nothing for the older callbacks, so absolute figures remain a floor.

### F-43 Credits are keyed by the full buyer address

The escrow's credit records were keyed by `credit/` plus the first eight bytes of
the buyer's address. Two buyers sharing that prefix shared one record, so a
second refund increased the first buyer's credit and either buyer could withdraw
the sum. Found by `spikes/harness/escrow_refund_test.go` on its first run: after
two refunds of 100, the caller-facing accounting view reported
`"callerCredit":200`.

Storage keys now use the whole 20-byte address (`key_for_address`), and the
read/write helpers accept either a numeric id or a raw key.

Measured with the fix in place, two buyers and two offers, with a modelled node
restart (`env.Restart`, which copies the modelled state into a fresh
environment) between the refunds and the withdrawals:

| Step | Result |
|---|---|
| both offers funded | `escrowed` 200 |
| both refunded after the deadline | `creditTotal` 200, `escrowed` 0, each buyer's own credit 100 |
| after the restart | buyer A still sees `callerCredit` 100 |
| buyer A withdraws | A 100, contract 100, buyer B's credit still 100 |
| buyer B withdraws | B 100, contract 0, `creditTotal` 0 |

### F-44 The delivery proof runs: delivery ECIES v1 proved with a real Groth16 receipt

`proofs/delivery` implements the delivery statement of `SPEC.md` 5.6 and proves
it. The guest re-encrypts the canonical epoch secrets to the buyer's delivery
key and commits a 209-byte journal (version, contract, offer id, order nonce,
seller, buyer, buyer key, secrets commitment, ciphertext hash).

The construction is the repository-local ECIES of the pinned node - ECDH over
secp256k1, the NIST SP 800-56A concatenation KDF with SHA-256, AES-128-CTR, and
an HMAC-SHA256 tag (`idena-go/crypto/ecies`, `ECIES_AES128_SHA256`; the
parameters map `ethcrypto.S256()` to that entry) - with one pinned difference:
the CTR initialisation vector is the second KDF block rather than a random draw,
which makes the ciphertext a deterministic function of the ephemeral scalar, the
plaintext, and the recipient key. The wire format is untouched, which the
compatibility check below demonstrates rather than assumes.

Measured with `cargo run --release --bin delivery-host` on this machine
(`proofs/delivery/dist/delivery.json`):

| Quantity | Value |
|---|---|
| Statement | `fod:delivery:v1`, journal 209 bytes |
| Plaintext | 106 bytes of canonical secrets |
| Ciphertext | 219 bytes |
| Seal | 256 bytes |
| Guest execution | 7,405,568 cycles in 8 segments |
| Proving wall clock | 957.8 s (about 16 minutes) |
| Peak resident memory | about 8.9 GB |

The receipt is verified on the host with `receipt.verify(DELIVERY_GUEST_ID)`
before the artefact is written, and the journal fields are checked against the
expected vector afterwards.

Cross-language compatibility, which is the part that keeps this from being a
self-consistent fiction: `tools/fod/delivery_test.go` decrypts the published
ciphertext with the node's own `crypto/ecies` library - code the guest
implementation never touches - and recovers exactly the 106 secret bytes, checks
them against the journal's commitment, checks the published ciphertext against
the journal's hash, and confirms an unrelated key cannot open the delivery.

Not yet done: the contract-side verifier and `fulfill` wiring, which is the next
slice of Package 4.

### F-45 The escrow verifies a real delivery proof and settles the order

Extends F-44. The escrow contract now embeds the RISC Zero Groth16 verifier and
`fulfill` settles a funded order against a genuine receipt - inside the Idena
runtime, with the claim binding the Package 1 probe left open.

What the contract must recompute, from the pinned crates:

| Fact | Source |
|---|---|
| The host's `Receipt::verify` compares the receipt's claim with `ReceiptClaim::ok(image_id, Pruned(sha256(journal)))` | `risc0-zkvm 3.0.6` `src/receipt.rs`, `verify_with_context` |
| Struct digests are `sha256(tag_digest \|\| fields \|\| data words (u32 LE) \|\| field count (u16 LE))` | `risc0-binfmt 3.0.5` `src/hash.rs`, `tagged_struct` |
| `hash_bytes` is plain SHA-256, and a digest's canonical bytes are its SHA-256 output | `risc0-zkp 3.0.5` `src/core/hash/sha/cpu.rs`, `sha2::Sha256::digest` |
| The claim fields for a successful run are `input = ZERO`, `pre = image_id`, `post = SystemState { pc: 0, merkle_root: ZERO }`, `exit_code = (0, 0)`, `output = Output { journal, assumptions = ZERO }` | `risc0-zkvm 3.0.6` `src/claim/receipt.rs`, `ReceiptClaim::ok`; `risc0-binfmt 3.0.5` `src/sys_state.rs` |
| `ALLOWED_CONTROL_ROOT` = `a54dc85a...` and `BN254_IDENTITY_CONTROL_ID` = `c07a6514...` | `risc0-circuit-recursion 4.0.5` `src/control_id.rs` (the same values the Package 1 probe embedded) |

Measured:

| Check | Result |
|---|---|
| Recomputed claim digest for the proven delivery run | matches `proofs/delivery/dist/delivery.json` exactly (escrow unit test) |
| `fulfill` with the real receipt in the Idena runtime | accepted, gas 195,630,810 of the 512,000,000 per-call ceiling (38.2%) |
| Order afterwards | `sold`, `escrowed` 0, seller credited 100, `liquid` 100 |
| `deliveryView` | returns exactly the proven ciphertext |
| Altered seal | refused: `invalid_proof` |
| Altered ciphertext | refused: `ciphertext_mismatch` |
| Journal naming another offer | refused: `wrong_offer` |
| Journal naming another delivery key | refused: `wrong_delivery_key` |
| Second settlement of the same order | refused: `offer_not_funded` |
| Escrow module with the verifier embedded | 349,279 bytes (test-hooks) / 348,459 bytes (release), zero bulk-memory and zero float operators |

One real bug was found while wiring this: the contract's `allocate` export
returned a single shared buffer for every host request, so reading the offer
record overwrote the call argument the contract was still parsing. `fulfill`
refused with `stale_statement` for that reason. `allocate` now hands out a
fresh region from a 64 KiB arena.

### F-46 The CLI's prepared call is the call the escrow accepts

Extends F-45 into the file-based workflow of `SPEC.md` 4.3. The interface writes
an action request, the CLI validates it, checks that the buyer it names is the
address of the supplied nodekey, builds the delivery-key possession proof, and
assembles the `fundOffer` argument.

Measured:

| Check | Result |
|---|---|
| CLI unit test: calldata layout | 138 bytes; offer id, 65-byte delivery key, 65-byte signature |
| CLI unit test: possession proof | `crypto.Ecrecover` over the hand-assembled message returns the delivery key |
| CLI unit test: refusals | wrong format, wrong version, bad contract, non-decimal amount, unsupported operation, missing offer, missing nonce, unknown argument, foreign buyer, short delivery key - all refused |
| Runtime cross-check (`spikes/harness/escrow_purchase_test.go`) | the escrow accepts the CLI's calldata and funds the order, recording the CLI's delivery key and order nonce |

That cross-check is the point: the CLI and the contract build the possession
message independently, so a drift in the domain string, field order, address
form, or signature layout shows up as a refusal.

The same run surfaced a defect that no earlier test could have caught, because
every escrow test until now used amounts that fit in `u64`:

| Quantity | Value |
|---|---|
| Amount the CLI prepared (100 iDNA) | 100000000000000000000 base units |
| `u64` maximum | 18446744073709551615 base units (about 18.4 iDNA) |
| Price the contract stored | 18446744073709551615 - saturated, not 1e20 |

Prices, escrowed totals, and credits are `u64`, and `parse_u64` saturates, so any
price above about 18.4 iDNA is wrong: two different over-range prices compare
equal, and credits saturate. Recorded as `O-11` in `SPEC.md`; the fix is a wider
amount representation in the contract and is not done yet.

### F-47 Escrow amounts are 128-bit, and exact at 100 iDNA

Closes `O-11`. Prices, escrowed totals, per-buyer credits, and the credit total
were `u64`; with 18 decimals, `u64` holds only about 18.4 iDNA, so every larger
amount saturated. F-46's cross-check exposed it by preparing a 100 iDNA order
and reading `"price":18446744073709551615` back.

The contract now stores amounts as 16-byte big-endian unsigned 128-bit integers
(`AMOUNT_BYTES`), prints them with a decimal u128 helper, and parses the payable
amount from the host as u128. The offer record grew from 130 to 170 bytes. The
CLI refuses an amount above `u128::MAX` instead of letting it saturate.

Measured, with 100 iDNA (1e20 base units) instead of toy values:

| Check | Result |
|---|---|
| `orderView` after the CLI's prepared purchase | `"price":100000000000000000000` - exactly the prepared amount |
| Two refunded offers | `{"escrowed":0,"callerCredit":100000000000000000000,"creditTotal":200000000000000000000,"liquid":200000000000000000000}` |
| After one buyer withdrew | `{"callerCredit":100000000000000000000,"creditTotal":100000000000000000000,"liquid":100000000000000000000}` |
| Invariant `liquid >= escrowed + creditTotal` | holds at that scale |
| Suite | all ten escrow tests, now priced at 100 iDNA (or 50 for the cancellation case) |
| Gas at that scale | seed 109,242; fund 161,165; refund 110,479; fulfill 195,631,562 (38.2% of the ceiling) |

Artefacts: harness module 352,934 bytes, release build 351,926 bytes, zero
bulk-memory and zero float operators.

### F-48 The provider gateway runs, with its limits pinned by tests

Package 7's reference integration: `gateway/` serves one pool, one provider
identity, and one signing key, with the surface listed in `SPEC.md` 8.2. It is
plain Node with no framework, and it was smoke-started from `example-config.json`
(policy served, unknown path 404, protected path 401 without a token).

Measured with `npm test` (13 checks, node:test against a real loopback server):

| Property | Check |
|---|---|
| No RPC passthrough | `/rpc`, `/bcn_blockAt`, `/` and a traversal-shaped path are all 404 |
| Route table is explicit | a known path with the wrong method is 405 with `Allow` |
| Bearer-only authorisation | no token, `?token=`, `Basic ...`, and an unknown token are all 401 |
| No ambient session | no `Set-Cookie` is ever sent |
| Assignments leak nothing | challenge responses contain no `answerDigest` |
| Scoring and replay | a correct answer scores, a second answer for the same challenge is 409, an unknown challenge is 400 |
| Result signature | verifies with the published Ed25519 key, fails after one byte of tampering, and is labelled provider-issued |
| Transaction forwarding | refused 501 rather than relayed |
| Body limits | a 10 KB answer is 413 against a 4 KiB cap; a non-JSON content type is 415 |
| Rate limit | the fourth request in a 3-request window is 429 |
| Expiry | a token past its TTL is 410 |
| CORS | headers only for configured origins, never `*`; a `null` (file://) origin receives no authorisation and still needs a token |

The pool carries its provenance (epoch, author, source, block range) and the
gateway refuses a pool without it. The gateway trusts that claim; a utility that
needs to verify it checks the marketplace, not this service. The signing key is
written on first start as `provider-application-key.json` (mode 0600, git-ignored)
and is never the node wallet key.

Not built yet: the interface's Session view, and submission of already-signed
transactions (deliberately refused until that path is designed).

### F-49 The customer half of the provider flow, verified in a browser

Extends F-48. The interface now consumes a provider connection file
(`fod.provider-config` v1, `SPEC.md` 8.2), enrolls, answers challenges, and
verifies the provider's result itself.

Measured in Chrome against the running gateway, from a `file://` page
(`app/verify.mjs`, 22 checks):

| Check | Result |
|---|---|
| Connection file import | the policy renders: session id, utility, pass threshold, pool epoch |
| Enrolment | the challenge list appears, without answer material |
| Answers | both answers score `correct` through the real gateway |
| Result | outcome `passed`, score 2 of 2, chip reads `verified in browser` |
| Signature | checked with WebCrypto Ed25519 over the canonical payload the gateway signed |
| Network | the only outbound requests are to the configured gateway origin |
| Console | no errors |

One real gap surfaced while wiring this: the gateway answered no CORS preflight,
so a cross-origin JSON POST from a `file://` page (`Origin: null`) was blocked by
the browser before it reached any handler. Preflights are now answered with 204
and the CORS headers for allowed origins only; the gateway's suite grew to 14
checks covering both the allowed and disallowed cases. This is the second time
the browser-driven check found something the unit tests could not, the first
being the shared `allocate` buffer in F-45's work.

The participant token lives in memory for the tab's lifetime and is never
written to storage: a `file://` page has no origin to scope one.

Not built yet: submission of already-signed transactions through the gateway
(refused 501), and the utility side that accepts a result.

### F-50 Session-content export decodes a bundle and cannot carry keys

Package 8's content export (`SPEC.md` 6.7). `fod export` reads an inventory, the
provider's vault, and saved blobs, decrypts every live flip, and writes
`content-export.json` plus `flips/<cid>.public` and `flips/<cid>.private`.
`fod inspect` gained `-blobs-out` so the blobs it already verified are kept for
the export instead of being fetched twice.

Measured with `go test` (`export_test.go`, synthetic material built with the
node's own primitives):

| Check | Result |
|---|---|
| Decoded parts | byte-identical to what was encrypted, written next to the manifest |
| Deleted entries | carried in the inventory but absent from the export and the manifest commitment |
| Manifest commitment | reproducible from author, epoch, and sorted live CIDs |
| Key material | the export's JSON contains neither private scalar from the vault; `keys` states that key pairs are excluded |
| Answers, none supplied | `answersEvidence` reads `none`, and the list is empty rather than guessed |
| Answers, supplied | carried with their digest and evidence, and refused without a named source or with a malformed digest |
| Wrong passphrase / missing blob | refused instead of exporting a partial bundle |

The answers rule is the honest part: mapping historical answers to flips needs
the lottery assignment, the same external data Package 2's eligibility half
waits on, so the export carries answers only when the provider supplies evidence.

Not built yet: the gated-utility example that consumes a result (8.3), and the
FAQ's review against actual behaviour.

### F-51 The utility verifies a result itself and states its scope

Package 8 continues with the gated-utility example of `SPEC.md` 8.3:
`utility/` accepts a provider result and decides whether it earns access. It is
a separate service from the gateway, an explicit surface (`GET /policy`,
`POST /access`), and it implements the canonical form and the Ed25519 check
itself rather than importing the issuer's helper.

Measured with `npm test` (12 checks; results in the tests are signed with the
gateway's helper and verified by the utility's own code, so the two
implementations have to agree):

| Check | Result |
|---|---|
| Valid result | granted, with a grant id, an expiry, and the scope statement |
| Tampered payload | `bad_signature` |
| Unknown provider, provider with no key | `untrusted_provider` |
| Wrong utility, unknown session, failed outcome, expired result | refused with that reason |
| Malformed result | `malformed_result` for null, numbers, empty objects, and strings |
| Quota | a second grant for the same utility, provider and participant is `quota_exhausted`; another provider's namespace is a different key; an expired grant stops counting; a re-enrolled participant id is granted again and the decision still reports `provider-participant` scope |
| Identity requirement | with `identityVerification.required`, a signed result is refused `identity_required`, and request data carrying `identityVerified` changes nothing |
| Endpoint | `/policy` publishes the scope, `identityVerification.supported: false` and `quota: { scope: "local-process", durable: false }`; unknown path 404; wrong method 405; non-JSON 415 |

The scope rule is the point of the slice: a provider-issued participant id is not
a person, so the reference quota is labelled `provider-participant` and the
policy's own text says one-person-one-allocation needs identity eligibility
verified separately. The adapter does not pretend to provide that check: it
holds none, it reads no assertion of one, and a policy that asks for one refuses
instead of granting on a caller's word.

Correction, 2026-09-28: the quota and identity rows above were re-measured at
`1462782` after A1/A5. The earlier revision counted grants by participant id
alone and granted on a `identityVerified: true` request field; both statements
were true of the revision they were measured on, and neither is true now.

### F-52 Release manifest: the artefacts that exist, and what is missing

Package 9's non-network half. `release/build-manifest.mjs` rebuilds the release
escrow module (no test hooks), hashes both artefacts, checks two properties, and
writes `release/MANIFEST.json`. It deploys nothing and touches no network.

Measured (`MANIFEST.json`, revision `d70fbf5`):

| Item | Value |
|---|---|
| Interface artefact | `app/dist/flips-on-demand.html`, 257,892 bytes, sha256 `0b38d9d9...` |
| Release escrow module | `release/artifacts/fod_escrow.wasm`, 351,926 bytes, sha256 `a6207153...` |
| Gatekeeper-forbidden operators in that module | 0 |
| Remote script or stylesheet tags in the interface | 0 |
| Delivery proof program | image id `2a4f2f7b...`, statement `fod:delivery:v1`, journal 209 bytes, seal 256 bytes, 7,405,568 cycles |
| Deployment | none: network and contract address are null by construction |

`release/README.md` maps all fifteen definition-of-done items to their evidence:
seven verified, six partial, two not implemented or not exercised. The partial
and missing ones are named individually there rather than summarised away, and
the two that need outside input are the candidate-list data for registration and
explicit approval for the mainnet exercise.

### F-53 The three services run together: session to utility decision

Extends F-48, F-49, and F-51. `release/check-integration.mjs` starts the gateway
and the utility on loopback ports and plays the customer: enroll, answer the
example pool's two challenges, collect the provider's signed result, and post it
to the utility for a decision. The utility's policy is built from the key the
gateway publishes at `/provider/key`, so a grant only happens if the signature
survives the utility's own verifier.

Measured (9 checks, all passing):

| Check | Result |
|---|---|
| Gateway and utility start | both serve on their loopback ports |
| Answers | both scored `correct` by the gateway |
| Result | provider signs a passing result with its application key |
| Utility decision | `200`, granted, against a key fetched from the provider |
| Scope | the decision carries `provider-participant` and says it is not a person |
| Tampered result | refused `bad_signature` |
| Re-enrolment | granted again as a new scope, not as the same person |
| Quota | the first participant id cannot spend its quota twice |

This is the loop `SPEC.md` 8.1 describes end to end - provider acquires and
serves, customer participates, utility decides - with every hop a real HTTP
request between the repository's own services.

### F-54 The delivery proof is order-driven, and the escrow settles a re-opened order

Extends F-44 and F-45. `proofs/delivery/host` proved one hard-coded vector; with
`--order <path>`, or `-` for stdin, it proves the order in that file instead,
and the artefact carries `fulfill_argument` - the bytes `fulfill` parses.

Measured on a second proving run whose terms differ from the built-in vector in
every field (contract, seller, buyer, epoch, author, all four scalars, offer id
3, nonce 2):

| Quantity | Value |
|---|---|
| Guest execution | 7,405,568 cycles in 8 segments |
| Proving wall clock | 904.0 s |
| Fulfill argument | 692 bytes: offer id 8 + journal 209 + seal 256 + ciphertext 219 |
| `fulfill` gas in the Idena runtime | 195,605,927 |
| Settlement | `sold`, escrowed 0, seller credited 100 iDNA, `creditTotal` 200 iDNA after the refund |
| Buyer-side decryption | the pinned node's ECIES library recovers 106 bytes matching the journal commitment |

The contract derives the order nonce, so the nonce-2 proof settled an order the
escrow had funded, refunded, and funded again - the re-open path of `SPEC.md`
7.2, with a delivery proof bound to the nonce after reopening.

`--check` validates an order file - hex lengths, the buyer key against the buyer
secret, the derived ciphertext - without proving. It is input validation: it
produces no seal and is not delivery evidence.

`dist/delivery.json` was not re-proved. Its `test_vector` key was renamed to
`order` and `fulfill_argument` assembled from its existing bytes; the journal,
seal, ciphertext, and measurements are unchanged.

### F-55 The CLI prepares and delivers a funded order, without holding the witness on disk

Extends F-46 and F-54. `tools/fod/prepare-delivery` reads a `fulfill` action
request, derives the bundle's epoch secrets from the seller's nodekey, and hands
the order to the delivery prover on stdin; the prover gained `--no-witness`, so
the artefact it writes carries only public fields.

Measured on the full loop, both calls prepared by the CLI:

| Quantity | Value |
|---|---|
| Proving | 7,405,568 cycles in 8 segments, 927.6 s |
| Fulfill argument | 692 bytes: offer id 8 + journal 209 + seal 256 + ciphertext 219 |
| Artefact order keys | author, buyer, buyer_key, ciphertext, contract, epoch, note, offer_id, order_nonce, seller - no witness field |
| Funding | the escrow accepted the calldata from `fod prepare-purchase` |
| Settlement | the escrow accepted the calldata from `fod prepare-delivery`, gas 195,582,418 |
| Order afterwards | `sold`, escrowed 0, seller credited 100 iDNA |

The two halves never share a key: the buyer's delivery key is public in the
funded order, and the seller derives the epoch secrets from its own nodekey.
`PrepareDeliveryOrder` refuses a request naming another seller or another author,
because deriving keys for an address the nodekey does not control would prove the
wrong secrets; delivering a retraded bundle needs the secrets it sold, which no
command reads yet.

Extended on 2026-09-25: `fod prepare-delivery` reads the order back through a node
first and refuses a request the chain disagrees with - not funded, another nonce,
buyer, delivery key, or seller, a bundle the registry no longer holds, a deadline
that has passed, or less wall-clock left than the 927.6 s the proof above takes.
The margin is measured rather than assumed: the node's own last block interval
turns the remaining blocks into seconds, and a chain that reports no interval
falls back to the deadline alone. Measured live against the lab chain for an open
offer: `offer 1 is open on the chain, and only a funded order can be delivered`,
refused before any proving started.

### F-56 The retrade ceiling holds in the runtime, on two sales of one bundle

Superseded on 2026-09-25: a bundle is sold once now, and settlement removes the
record, so the ceiling and the second sale this fact measures no longer exist in
the contract. It is kept because it is the evidence that the retrade rule worked
while it was the rule, and because the sale it describes is still the sale the
runtime settles.

Extends F-45 and F-54. `listBundle` and `bundleView` are new exports; the release
module carries no test hook and no bulk-memory or float operator.

| Quantity | Value |
|---|---|
| Retrade proving | 7,405,568 cycles in 8 segments, 944.2 s |
| Bundle after sale one | holder = the buyer, sales 1, last price 100 iDNA, ceiling 90 iDNA |
| Refusals | `not_holder`, `above_ceiling` for one base unit over the ceiling, and `offer_live` for a second listing |
| Bundle after the retrade | holder = the second buyer, sales 2, last price 90 iDNA, ceiling 81 iDNA |
| Accounting | escrowed 0, `creditTotal` 190 iDNA, `liquid` 190 iDNA |
| Release module | 359,500 bytes; exports `listBundle` and `bundleView`; no `seedBundle`, `seedOffer`, or `setOfferCommitment` |
| Ceiling arithmetic | unit tests: floor(9p/10), compounding 100 -> 90 -> 81, and `u128::MAX` without overflow |

Sale two's seller is sale one's buyer, and both proofs commit to the same
secrets, which is what makes it a retrade rather than a second sale by the author.
The bundle record is created by the `seedBundle` hook: registration is still
unimplemented, so nothing here says how a bundle comes to exist.

### F-57 The listing path: the interface's request, the CLI's call, the escrow's check

Superseded on 2026-09-25 in two places: the request carries no `ceiling` field
now, and a settled bundle cannot be listed again. The part that stands is the
path itself - the interface writes a `listBundle` request, the CLI turns it into
the 48-byte argument, and the escrow accepts it.

Extends F-55 and F-56. `fod prepare-list` turns a `listBundle` action request
into the 48-byte argument the escrow parses: commitment (32), then the price
(16, big-endian). Marketplace state can carry bundle records, so the interface
can show a holder the ceiling and write the request.

| Quantity | Value |
|---|---|
| Calldata | 48 bytes; 90 iDNA is `0000000000000004e1003b28d9280000` |
| Refusals | a price one base unit over the snapshot's ceiling, a price above 2^128-1, a short commitment, a non-decimal price or ceiling, an unknown argument |
| Runtime | the escrow accepted the CLI's calldata and created an open offer carrying that price and commitment, gas 106,935 |
| Browser | `app/verify.mjs` downloads a request naming `listBundle`, the bundle, its 90 iDNA price, the 90 iDNA ceiling, and the contract |
| FAQ | the claim that a settled bundle cannot be sold again was replaced; the old text is in the same commit's diff |

Extended on 2026-09-25: `fod prepare-list` takes `-nodekey` and reads the bundle
record back through a node before it prepares anything, so it now also refuses a
flip set the supplied nodekey does not hold and one that already has a live
offer. Those are the two refusals `listBundle` itself makes, and the calldata is
unchanged. Measured live against the lab chain: `this flip set already has offer
1 live`, and `held by 19e7e376..., and the supplied nodekey controls
5f8ae476...`.

No signature is produced or needed for a listing: the holder's authority is the
transaction that carries the call, and the escrow re-checks the holder and the
ceiling against its own state rather than the snapshot's. The bundle record
still comes from the `seedBundle` hook, because registration does not exist.

### F-58 The signed transaction is the node's own format, and no transaction has been sent

Extends F-46 and F-57. `fod submit` turns any of the three prepared actions into
a transaction built with the pinned node's own packages, then decodes it again,
recovers the sender, and parses the attachment back before writing anything.

| Quantity | Value |
|---|---|
| Transactions built | `listBundle` 48-byte call, 167 bytes; `fundOffer` 138-byte call with 100 iDNA, 270 bytes; `fulfill` 692-byte call, 811 bytes |
| Decoding | `types.Transaction.FromBytes` accepts each; `types.CallContractTx`; the recipient is the snapshot's contract |
| Sender | `types.Sender` recovers `<test-signer-address>`, the nodekey that signed |
| Attachment | `attachments.ParseCallContractAttachment` returns the method and the exact calldata that was prepared |
| Zero amounts | the node writes a zero amount as nothing and reads it back as nil; the check compares through that rather than treating nil as a mismatch |
| Broadcast | none. `broadcast` is false in every receipt; `bcn_sendRawTx` is implemented behind `-broadcast` and has never run |

The nonce, epoch, maximum fee, and tips are the caller's. Nothing here checks
them against the chain, and the command says so: a stale nonce or epoch is
rejected by the node rather than applied, which is the failure mode this
deliberately leaves to the node.

Extended on 2026-09-25, when the first transaction was actually broadcast (epoch
1 on the lab chain, `fundOffer`, hash
`0x64d18f1567c7e8a038f4252df98538a09b90941f63d92da44bb7c167cfcbd133`, accepted
into the mempool). Three rules the CLI now checks before it sends, each read from
the pinned node and each seen live:

- Nonce: a transaction is invalid unless its nonce is **higher** than the
  account's (`blockchain/validation/validation.go`, `nonce >= tx.AccountNonce`),
  which is not the equality a reader expects. The node's own message says only
  `invalid nonce`, and it prints both values, so the check names the value that
  would work instead.
- Epoch: a transaction may not name an epoch below the chain's
  (`globalEpoch > tx.Epoch`), and the lab chain moved from epoch 0 to epoch 1
  while the CLI was being tried, which the pre-flight caught.
- Maximum fee: `maxFee / feePerGas` may not exceed the block gas limit
  (`types.MaxBlockSize`, 5,120,000 with upgrade 11). At the lab's 0.01 iDNA per
  gas that caps the fee at 5.12e22 base units; 1e23 was refused as
  `too high max fee`.

The account nonce is per epoch: the same account read 4 in epoch 0 and 0 in epoch
1.

The receipt is the one part of the release record's outcome-recording step this
build cannot demonstrate positively.
`bcn_txReceipt` answers an empty body on the lab node: `api.convertReceipt`
panics when the node has a transaction but no receipt for it yet, and the handler
recovers per request, which leaves the caller with nothing to parse
(`api.convertReceipt(..., 0x0, ...)` in the node's own log). `fod submit` records
that as `included: false` with the node's symptom rather than as a failure, which
is what the record showed.

### F-59 The refund and withdrawal paths run through the CLI

Extends F-41 and F-58. `SPEC.md` 6.1 lists `refund` and `withdraw`; both now
exist and write prepared actions for `fod submit`. Their encodings differ, which
is the part a second implementation would get wrong: `refund` parses the offer
id as decimal text, and `withdraw` ignores its argument.

| Quantity | Value |
|---|---|
| Refund calldata | `37`, the ASCII "7" for offer 7 |
| Withdrawal calldata | empty; the empty argument survives the transaction encoding as one empty argument |
| Signed transactions | refund 116 bytes, withdrawal 117 bytes, both recovering `<test-signer-address>` |
| Runtime: refund | accepted, gas 110,493; `escrowed` 0, the buyer credited 100 iDNA, the buyer's balance still 0 |
| Runtime: withdrawal | accepted, gas 201,987; the caller paid the whole credit, the contract balance 0, a second call returns `no_credit` |
| Browser | `app/verify.mjs` downloads both requests from the trades view |

The possession proof binds the contract address, so the harness helper that
builds a credit takes the address rather than assuming its own; a proof signed
for another address is refused `key_not_bound`.

### F-60 Qualification is unreachable from outside the node's package, so Package 2 reconstructs it from behaviour

Source: `idena-go@9bc57040`, `core/ceremony/qualification.go`, `reporters.go`,
`types.go`; sizes and entry points read with `wc -l` and a search for top-level
declarations in this session. Nothing was written to the checkout.

| Fact | Value |
|---|---|
| The rules are unexported | `qualification.go:20` `type qualification struct`; the only constructor is `NewQualification(config *config.Config, epochDb *database.EpochDb) *qualification` at line 30 |
| Consequence | No caller outside `core/ceremony` can use them, so the qualification of a closed epoch must be reconstructed rather than imported or queried |
| Scale to mirror | `qualification.go` 495 lines, `reporters.go` 163, `types.go` 44; the node's own vectors are `qualification_test.go` 565 and `reporters_test.go` 192 (1,459 lines total) |
| API route | closed: `FlipApi` serves the ceremony in progress and `DnaApi.Identity` serves per-identity state (same session as the live-node reading in `PROGRESS.md`) |

Decision recorded before any qualification code is written: the reconstruction
is this repository's own implementation, written from the behavioural facts
recorded here (thresholds, statuses, orderings, and verdict rules), under the
same read-only discipline as F-1 to F-12. No line is translated from the pinned
files, nothing from them is vendored, and any rule that cannot be reproduced
from the written facts without translating the source goes into its own
component with the upstream licence text and notice instead of into this MIT
tree. F-11 records why: `idena-go` and `idena-wasm-binding` are LGPL-3.0 and the
pinned `idena-wasm` revision carries no licence grant.

### F-61 How one flip qualifies: the gates, the thresholds, and the committee exclusions

Source: `idena-go@9bc57040`, `core/ceremony/qualification.go` (`qualifyFlips`
lines 115-215, `qualifyOneFlip` 382-495, `getFlipStatusForCandidate` 353-364,
`getAnswersCount` 366-380), checked against the node's own vectors in
`qualification_test.go:55-125`. Recorded as behaviour under F-60: read, not
transcribed. Committee sizes and the upgrade flags come from the node's
consensus configuration for that epoch, not from constants in this file.

Inputs per flip:

| Input | Source |
|---|---|
| Answers | each candidate's long-answer attachment, parsed with `attachments.ParseLongAnswerBytesAttachment`, one 2-bit answer per assigned flip, decoded by `types.NewAnswersFromBits` |
| A candidate's answers are void | in `qualifyCandidate`: unparsable attachment, or the answer hash over short answers and salt not matching `epochDb.GetAnswerHash(candidate)`, or the VRF-derived word not matching the short session's random |
| `approveCnt` | grades `GradeA`-`GradeD` raised on the flip, excluding `GradeNone` and `GradeReported` |
| `reportCnt` | candidates that answered `GradeReported` on the flip |
| `reportCommitteeSize` | before upgrade 11: reported + graded as they accumulate, minus reporters dropped below; from upgrade 11: `approveCnt + reportCnt` |
| `totalGradeScore`, `gradeScoreCommitteeSize` | sum of grade points and their count on the upgrade-11 path |

The gates, applied in this order:

| Gate | Rule |
|---|---|
| reported | committee 0 or 1: never; 2-3: `reportsCount >= committee`; 4: `>= 3`; 5: `>= 4`; otherwise more than half the committee |
| graded | committee size is the grade-score committee from upgrade 11, otherwise the number of answers. With upgrade 10: 0 never, 1 exactly one approve, 2-4 at least two, 5 at least three, otherwise more than a third. Before upgrade 10: always more than a third |
| grade | reported wins; else if graded, up to upgrade 10 the rounded mean `totalGradeScore / approveCnt`, and from upgrade 11 the exact `totalGradeScore / gradeScoreCommitteeSize` as a decimal score with the integer grade left unset; else grade D, or grade D's score from upgrade 11 |

The verdict, checked in this order:

| Share of the flip's answers | Result |
|---|---|
| left >= 75% | Qualified, answer left |
| right >= 75% | Qualified, answer right |
| left >= 66% | WeaklyQualified, answer left |
| right >= 66% | WeaklyQualified, answer right |
| none >= 66% | QualifiedByNone |
| anything else | NotQualified |

Exclusions that shrink a committee:

| Rule | Effect |
|---|---|
| A candidate reported on 34% or more of its own flips | its reports leave every flip's committee |
| From upgrade 11: no approve at all, or more than one grade above D | its grades leave every flip's committee, recorded as `TooManyReports`, `NoApproves`, `TooManyIncreasedApproves` |
| Short session | a status that is already `NotQualified`, or belongs to a flip outside `notApprovedFlips`, or is `QualifiedByNone`, is left alone; otherwise a `None` answer on that flip turns the status into `NotQualified` |

Checked against the node's vectors: 15 left / 3 right / 4 none is
`WeaklyQualified` left, 30/66/4 is `WeaklyQualified` right, 4/4/4 is
`NotQualified`, 1/2/10 is `QualifiedByNone`, and the upgrade-11 score path
returns `totalGradeScore / gradeScoreCommitteeSize` with `GradeNone` left in the
grade field. Because upgrades 10 and 11 change these rules, `SPEC.md` O-2 stays
open until their activation epochs are pinned; the candidate point score and
identity state are the next slice.

### F-62 The deploy transaction: its shape, its address, and the one flag it needs

Source: `idena-go@9bc57040`, read in this session, plus the binding's argument
packing. This is what `tools/fod`'s `prepare-deploy` builds, and nothing in it
is inferred from the plan.

| Fact | Value and source |
|---|---|
| Transaction type | `DeployContractTx = 0xF` (`blockchain/types/types.go:32`) |
| Recipient | must be nil; a deploy with `To` set is `InvalidRecipient` (`blockchain/validation/validation.go:728`) |
| Attachment | `DeployContractAttachment{CodeHash, Code, Args, Nonce}` with `CreateDeployContractAttachment` (`blockchain/attachments/attachment.go:360-378`); the node parses it back with `ParseDeployContractAttachment` |
| Code hash | `keccak256(code)`; the code-hash table of embedded contracts is what the minimum-stake rule is keyed on, and it is not a rule for custom modules (`validation.go:737-742`) |
| Custom modules | `len(Code) > 0` is refused unless the chain's `Consensus.EnableUpgrade11` is set; a non-embedded code hash with no code is refused as well (`validation.go:744-750`) |
| Address | `keccak256(keccak256(code) || packedArgs || nonce)`, last 20 bytes (`vm/wasm/util.go:9-23`, `ComputeContractAddrWithUnpackedArgs`); the node derives it in `ContractAddr` (`vm/vm.go:162-169`) |
| Packed arguments | `lib.PackArguments` prefixes every list with the format byte `0x1` (`idena-wasm-binding/lib/lib.go:31,126-143`), so a deploy with no constructor arguments still packs one byte, and that byte is part of the address |
| Stored on success | `DeployWasmContract(contract, data.Code)` runs from the deploy path (`vm/wasm/wasm_env.go:436`) |
| Code-size cap | none found in the node's Go layer: `common/`, `blockchain/`, and `vm/` carry no module-size limit beyond `MaxWasmContractStoreKeyLength = 128*1024` for storage keys (`common/types.go:42`). A cap in the runtime source was searched for and not found in this session, so the first real deployment is what settles it |

Nothing here is a deployment: no transaction built from these rules has been
broadcast, and the 3,000,000-gas stake rule above applies only to the embedded
code-hash table, so it does not bind this module.

### F-63 The flip verdict is implemented in this repository and checked against the node's vectors

Source of the rules: F-61 (behaviour read from `idena-go@9bc57040`). Source of
the grade scores: `blockchain/types/types.go:1307-1340`, read in this session.
Implementation: `tools/fod/qualification.go`; vectors:
`tools/fod/qualification_test.go`.

| Fact | Value |
|---|---|
| Grade values | `GradeNone 0`, `GradeReported 1`, `GradeD 2`, `GradeC 3`, `GradeB 4`, `GradeA 5` (`types.go:1314-1320`) |
| Grade scores | Reported `0`, None `1`, D `2`, C `4`, B `6`, A `8` (`types.go:1327-1340`) |
| A flip nobody graded | carries grade D and grade D's score, not zero: the node's default branch writes `GradeD.Score()` on the upgrade-11 path. Only a reported flip keeps the zero value |
| Comparisons | the shares are compared as float32, which is what the node does; a float64 implementation disagrees with the chain at some ratios |
| Grade score | the upgrade-11 score is `totalGradeScore / gradeScoreCommitteeSize`; the node keeps it as a decimal with a division precision of 16 digits, this repository keeps the exact rational and leaves rounding to whoever records it |
| First slice built | `AnswerCounts`, `ReportedEnough`, `GradedEnough`, `DroppedReports`, and `QualifyFlip` |
| Checked against | the 15 cases recorded from the node's own `Test_qualifyOneFlip`, all reproduced (`go test -count=1 -run 'QualifyFlip\|Gates\|AnswerCounts' ./...` passes) |
| Not built yet | the committee assembly from an epoch's answers and assignment, which needs `reporters.go`'s tracker semantics, and the candidate-level half: point score, qualification, and identity state |

The implementation is this repository's own, written from the facts above under
the rule F-60 records; no line of the pinned files is translated into it.

### F-64 The committee accounting, and the three upgrade-11 drops

Source: `idena-go@9bc57040`, `core/ceremony/qualification.go` (`qualifyFlips`,
lines 125-215) and `core/ceremony/reporters.go` (the `grades` tracker, lines
100-163), read in this session. Implementation: `BuildFlipCommittees` in
`tools/fod/qualification.go`.

The two paths count the same flip differently, which is why the reconstruction
keeps them apart:

| Quantity | Before upgrade 11 | From upgrade 11 |
|---|---|---|
| Score sum | the grade enum values of the graded answers: D=2, C=3, B=4, A=5 | `Grade.Score()` of every answer: none=1, reported=0, D=2, C=4, B=6, A=8 |
| Committee size | reported plus graded answers, so a `GradeNone` counts for nothing | every grade counts, including `GradeNone` |
| Approves | the graded answers | grades at or above D |
| Reported gate's committee | reported plus graded answers | `approveCnt + reportCnt` |
| Graded gate's divisor | the number of answers on the flip | the grade-score committee size |

The drops, in the order the epoch applies them:

| Condition | Before upgrade 11 | From upgrade 11 |
|---|---|---|
| A participant reported on 34% or more of its own flips | its reports leave every committee; its grades stay | its whole contribution leaves, reports and grades |
| A participant approved nothing at all | no effect | its whole contribution leaves, so even its `GradeNone` answers stop counting |
| A participant approved more than one grade above D | no effect | its whole contribution leaves |

A participant whose long answers are absent or empty contributes nothing at all,
in either path. The two shares that decide a drop are compared as float32, the
same as the thresholds in F-61.

### F-65 How a participant's answers are scored, and where the short session inverts

Source: `idena-go@9bc57040`, `core/ceremony/qualification.go` (`qualifyCandidate`,
lines 220-350) and the session constants in `common/network.go:12-14`
(`ShortSessionFlips = 6`, `ShortSessionExtraFlips = 2`), read in this session.
Implementation: `ScoreCandidate` and `CandidateFlipStatus` in
`tools/fod/qualification.go`.

Per flip, against the epoch's verdict for it:

| Verdict | Matching answer | Answered none | Answered the other way |
|---|---|---|---|
| Qualified | 1 point, counted | counted, no point | counted, no point |
| WeaklyQualified | 1 point, counted | 0 points, counted | 0.5 points, counted |
| QualifiedByNone, NotQualified | nothing | nothing | nothing |

The qualified flip's unconditional count is the part that surprises: a
participant that answered a qualified flip the other way still gets it counted,
and only the point is withheld.

The short session differs in three ways:

| Rule | Effect |
|---|---|
| Reported flips | a flip whose verdict carries `GradeReported` scores nothing and counts for nothing in a short session; in a long session it scores normally |
| Extra flips | a flip past position 6 counts only if the participant bought an extra flip, which it does by answering none on one of its first six assigned flips that the epoch did not approve; at most two |
| Status override | the rule already recorded in F-61, implemented as `CandidateFlipStatus` |

A participant whose long answers fail the short-answer binding returns zero
points, no considered flips, and the assignment's flip count as its qualified
count; `VoidedAnswers` keeps that shape rather than inferring another.

This fact corrects an inversion that was easy to make and hard to see: the
reported-flip rule reads as `!shortSession || grade != GradeReported`, so the
skip applies to the short session, not the long one. The node's own tests were
run in the pinned checkout (read-only, nothing written to it) to settle it, and
the vectors are reproduced by this repository's tests: `Test_qualifyCandidate`
(2.5 points over 3 in its short session, 4.5 over 6 in its long one, 0 over 9 for
the void one), `Test_qualifyCandidateWithFewFlips` (1.5 over 2), and the nine
assertions of `Test_getFlipStatusForCandidate`.

### F-66 The eligibility policy is implemented, and its reading is stated

Requirement: `SPEC.md` 5.1 (`All covered mainnet-submitted flips have strong
consensus and no final Reported classification`), 5.3, and 5.5 item 2, which
requires a registration proof to establish that every covered flip satisfies it.
Implementation: `CheckBundleEligibility` in `tools/fod/eligibility.go`.

| Part | Value |
|---|---|
| Policy version | `fod:eligibility:v1`, the string the manifest's eligibility-policy field carries |
| Strong consensus | read as the node's `qualified` status: `weaklyQualified`, `qualifiedByNone`, and `notQualified` all fail the policy |
| Reported | any covered flip whose verdict carries `GradeReported` fails, whatever its status |
| Missing verdict | a covered flip the epoch has no verdict for fails as `no_verdict` instead of passing by default |
| Reasons recorded | a flip that fails both ways is recorded twice, so an author sees every way the bundle misses |
| Determinism | covered flips come back sorted, so a manifest can commit to the decision |
| Refusals | an unknown policy version (the message names the supported one), a bundle covering no flips, and a repeated flip |

The strong-consensus reading is a decision, not a quotation: the spec says
"strong consensus" and the node distinguishes `qualified` from
`weaklyQualified` by the 75% and 66% answer shares, so this policy takes the
former. Changing that reading changes the policy version rather than the code
silently.

Not settled here: the epoch range and the pinned consensus version that
`SPEC.md` 5.3 requires, which is the open question O-2, and the wiring that feeds
these verdicts from chain-committed answers.

### F-67 The BundleManifest, and the bytes its commitment covers

Requirement: `SPEC.md` 4.5 (`BundleManifest` with network, author, epoch,
complete sorted CIDs, validation anchor, consensus version, eligibility-policy
version; one canonical encoding frozen and verified with cross-language
fixtures) and 5.5 item 1, which requires a registration proof to establish that
the manifest holds the complete inventory. Implementation:
`tools/fod/manifest.go`; frozen vector:
`proofs/registration/vectors/manifest-v1.json`; checked by
`tools/fod/manifest_test.go`.

The commitment is two layers, because the inventory commitment already existed:
it is what the Package 8 content export produces, so the two constructions stay
one rather than two lookalikes.

| Layer | Bytes |
|---|---|
| Inventory commitment | `SHA-256("fod.bundle-manifest:v1" \|\| lowercase author (ASCII) \|\| epoch (u16, big-endian) \|\| for each sorted CID: 0x00 \|\| CID (ASCII))` |
| Manifest claim | `"fod:manifest:claim:v1" \|\| inventory commitment (32, raw) \|\| validation anchor (32, raw) \|\| u16 length \|\| consensus version (UTF-8) \|\| u16 length \|\| eligibility policy (UTF-8) \|\| u16 length \|\| network (UTF-8)` |
| Manifest commitment | `SHA-256` over the manifest claim bytes |

What the builder refuses rather than committing to: an empty or over-long
network, a malformed author address, an anchor that is not a 32-byte digest, an
empty inventory, a repeated CID, a CID that is not the multibase base32 form of
a CIDv1 raw/sha2-256 value (`b` plus 58 base32 characters, the form the node uses
for flip blobs), and empty or over-long consensus and policy labels. Changing any
field changes the commitment, which the tests pin one field at a time.

Not pinned here: what the validation anchor's canonical source is. It is carried
as a 32-byte digest and its derivation belongs to the reconstruction wiring, so
this fact fixes the *form* and leaves the *value* to that step.

### F-68 What the zkVM charges for the registration guest's primitives

Measured on this machine with `spikes/registration-cycles`, run through the
RISC Zero executor so a measurement costs execution time rather than proving
time. The guest times itself with its own cycle counter and commits the deltas,
so these are the zkVM's own numbers, not wall-clock estimates. `risc0-zkvm`
3.0.6, `k256` 0.13, `aes` 0.8, `ctr` 0.9, `hmac` 0.12, `sha2` 0.10.

```bash
cd spikes/registration-cycles && cargo build --release
./target/release/cycles-host <mode> <payload KiB>
```

| Work | Cycles | Per byte |
|---|---|---|
| Ingest 4 KiB through serde's `Vec<u8>` | 629,725 | ~154 |
| Ingest 4 KiB through `read_slice` | 15,760 | ~3.8 |
| SHA-256 over 4 KiB (software `sha2`) | 261,225 | ~64 |
| AES-128-CTR plus HMAC-SHA256 over 4 KiB | 564,895 | ~138 |
| One secp256k1 ECDH | ~3,360,000 | - |
| The whole per-flip mix at 4 KiB (two ECDH, stream cipher, tag, digest) | 7,947,606 | - |

What the design has to do with it:

- **Ingestion is a 40x decision.** A blob read as `Vec<u8>` costs about 154
  cycles per byte because serde reads it element by element; the same blob read
  with a length and `read_slice` costs about 3.8. At the 600 KiB blob cap that is
  the difference between about 92M cycles and about 2.3M. A registration guest
  must read blobs as slices.
- **Per-byte crypto is the second term, the curve is the first.** Hashing a blob
  to check its CID costs about 64 cycles per byte, and the pinned construction's
  stream cipher and tag about 138. Each secp256k1 ECDH costs about 3.4M cycles,
  and a flip needs two.
- **Proving time follows the delivery proof's ratio.** That receipt was
  7,405,568 cycles for 904-944 s of proving, about 8,200 cycles per second, so a
  bundle's registration is roughly 2.1 minutes of proving per million cycles:
  about 2.8 hours for ten 4 KiB flips (79M cycles) and about 5.4 hours for twenty
  (159M cycles). At the 600 KiB cap a single flip is about 130M cycles.
- **Per-flip on-chain verification is out of the question.** One Groth16
  verification costs 195.6M wasm gas, 38% of the per-call ceiling, so at most two
  receipts fit in a call. The bundle's flips have to be aggregated into one
  receipt, which is what the hours of proving buy.

Left open by this measurement: whether the guest must decrypt the plaintext or
only verify the construction's tag. `SPEC.md` 5.5 item 4 says the secrets
"authenticate and decrypt", and the numbers above include the stream cipher; a
weaker claim that checks the tag without producing plaintext would save about
138 cycles per byte and is a policy decision, not a measurement.

### F-69 The registration claim, and the bytes a receipt commits to

Requirement: `SPEC.md` 5.5, which lists what a registration proof establishes
(complete inventory, eligibility policy, blob CIDs, secrets that decrypt,
supported structure, and binding to network, author, epoch, manifest, and
protocol version) and adds that the identity private key must not enter the
prover and that registration separately requires the caller to control the
author address. Implementation: `tools/fod/registration.go`; frozen vector:
`proofs/registration/vectors/claim-v1.json`; checked by
`tools/fod/registration_test.go`.

| Offset | Bytes | Field |
|---|---|---|
| 0 | 4 | statement version, currently `1` |
| 4 | 20 | contract address |
| 24 | 20 | author address |
| 44 | 2 | epoch (u16, big-endian) |
| 46 | 4 | covered flips (u32, big-endian) |
| 50 | 32 | manifest commitment, the construction of F-67 |
| 82 | 32 | key commitment: the secrets commitment the delivery proof also binds |
| 114 | 32 | eligibility-policy digest |
| 146 | 32 | consensus-version digest |

178 bytes in total, read by fixed offsets the way `fulfill` reads the delivery
journal. Both label digests are `SHA-256` over the label's exact string, so a
second implementation cannot bind a label that only looks the same; the vector's
policy digest `06b31d89...` and consensus digest `cac2b17b...` were reproduced
with `shasum -a 256` over the two strings, independently of this implementation.

What the journal deliberately does not carry: the per-flip verdicts, the CIDs,
the blob bytes, and the secrets. They are the guest's private input, and the
commitments above are what binds them; the covered count is public because the
interface and the record show it.

What follows for the two other sides: the escrow has to pin the two label
digests as constants taken from the labels it accepts and check the caller
against the author field, and the guest has to commit exactly these bytes. Neither
is built yet, so nothing here registers anything.

### F-70 The delivery proof's image id pins its crate, so the construction is duplicated on purpose

Measured on this machine while deciding where the registration guest's decryption
should live. The check is one command:

```bash
cd proofs/delivery && cargo build --release
rg DELIVERY_GUEST_ID target/release/build/*/out/methods.rs
```

| State of `proofs/delivery/core/src/lib.rs` | `DELIVERY_GUEST_ID` |
|---|---|
| as committed | `[2066698026, 4249157153, 2249559873, 4198434824, 351460720, 4240891469, 979906412, 3827606954]`, which is the pinned `2a4f2f7b...524e4` |
| with one added public function (`decrypt_from`) | different, so the pinned id no longer describes a build |
| after removing it again | identical to the pinned id, and `git diff` on the file is empty |

Two facts follow. The current source reproduces the pinned delivery image id, so
the receipt evidence in `release/` and in the harness stays anchored to a
buildable revision. And the crate is frozen in practice: an unused public
function is enough to move the id, which would invalidate the contract's pinned
constant, `release/MANIFEST.json`'s `proofPrograms.delivery.imageId`, and every
stored receipt. Extending it is therefore a version bump with a re-proving run,
not a refactor.

The registration proof needs the same construction in the other direction. It
gets its own implementation, and the two are kept honest by a shared frozen
vector rather than by sharing the crate: the delivery side proves it reproduces
the vector's ciphertext, and the registration side proves it recovers the
vector's plaintext.

### F-71 The shared construction vector, and the three implementations that check it

The pinned construction exists twice on purpose (`SOURCES.md` F-70: the delivery
proof's image id freezes its crate), so the two copies are held together by one
frozen vector instead of by a shared dependency.

| Piece | Value |
|---|---|
| Vector file | `proofs/vectors/ecies-v1.json` |
| Secrets | epoch 204, author `3333...`, parts `1111...` and `7777...` |
| Canonical secrets bytes | 106 bytes: the domain, the epoch, the author, and the two scalars |
| Key commitment | `8e1f9781ff3f7b1ba5981e4e16389401a6f47e30b9d7df6204009863db1a6db4`, reproduced with `shasum -a 256` over the canonical bytes outside this implementation |
| ECIES | secret `0102...1f20`, its public key, ephemeral `4041...5e5f`, the canonical bytes as plaintext, and the ciphertext |
| CID | the CIDv1 raw sha2-256 of `flips-on-demand frozen vector, flip one`, `01551220d6657407...7cf90` |

Who checks what:

| Implementation | Test | What it establishes |
|---|---|---|
| `fod-delivery-core` (encrypt direction) | `proofs/delivery/host/tests/ecies_v1.rs`, 2 tests | it derives the vector's public key, reproduces the vector's ciphertext, and its secrets encoding is the one the commitment covers |
| `fod-registration-core` (decrypt direction) | `proofs/registration/core/tests/ecies_v1.rs`, 3 tests | it recovers the vector's plaintext, refuses a changed body, a changed tag, a wrong secret, and a truncated message, and computes the vector's CID and commitment |
| the CLI's CID builder | `tools/fod/construction_test.go` | the multibase string it renders decodes to the vector's CID bytes, so a manifest's CIDs are the value the guest will check |

The delivery guest's image id was re-read after the host-side test and the vector
were added: it is still `2a4f2f7b...524e4`. Tests and vectors outside the guest's
crate do not move the pinned id, which is what makes this approach workable.

### F-72 The qualification rules on the registration side, pinned by the CLI's vector

`proofs/registration/core/src/qualification.rs` carries the flip verdict and the
eligibility policy in the language the guest needs, and
`proofs/registration/vectors/qualification-v1.json` is the file both it and the
CLI check against: the Rust tests read it, and `tools/fod`'s
`qualification_vector_test.go` reads the same one.

| Part of the vector | Cases | Where they come from |
|---|---|---|
| `flipVerdicts` | 15 | the pinned node's own `Test_qualifyOneFlip`, recorded in F-61 |
| `eligibility` | 6 | this project's policy, recorded in F-66 |
| `epochs` | 6 | this project's reading of the committee accounting, recorded in F-64: the tracker's count, the three upgrade-11 drops, and a void participant |

Answers and grades are the node's enum values, statuses are the strings the CLI
uses, and a grade score is written as a rational like `18/8` so both sides
compare exact values rather than decimals.

| Implementation | Tests | What they establish |
|---|---|---|
| `fod-registration-core` | `core/tests/qualification_v1.rs`, 6 tests | every verdict case, the gate boundaries (reported, graded, the 34% share), the six eligibility decisions, the six epoch fixtures on both paths, and the refusals |
| the CLI | `qualification_vector_test.go`, 3 tests | the same verdict cases, the same eligibility decisions, and the same epoch fixtures through `QualifyFlip`, `CheckBundleEligibility`, and `BuildFlipCommittees` |

The epochs section is the one part of the vector that is not the node's own
literals: the node has no test for those numbers, so they are this repository's
reading of F-64, pinned in both implementations. The verdict and eligibility
sections keep their stronger provenance - the node's own vectors and the project's
policy - and the three are labelled apart here so nobody mistakes one for the
other.

The Rust side also had to work without `std`: the pre-upgrade-10 mean is rounded
half away from zero by integer arithmetic rather than `f64::round`, because the
guest cannot link the standard library.

### F-73 A node-built blob opens on both sides, and the blob's wire layout

Requirement: `SPEC.md` 5.5 items 3 to 5, which require a registration proof to
establish that the blob bytes match their authenticated CIDs, that the committed
secrets decrypt them, and that the decoded content has the supported structure.
Vector: `proofs/registration/vectors/blob-v1.json`; implementations:
`tools/fod/commitment.go` and `proofs/registration/core/src/blob.rs`.

The vector was built with the pinned node's own primitives through the CLI: a
throwaway identity key, the epoch keys it derives for epoch 204, the node's
`crypto/ecies` encryption of two short parts, and the node's protobuf message. It
carries the nodekey, the author, the epoch, both secrets, both plaintexts, the
blob, its CID, the canonical secrets, and the commitment.

| Fact | Value and source |
|---|---|
| Blob message | `ProtoIpfsFlip`: the identity public key (65 bytes, uncompressed), the public part encrypted to the epoch public key, the private part encrypted to the epoch private key (`idena-go/core/flip/flipper.go`, F-2) |
| ECIES wire layout | `ephemeral public key (65) \|\| IV (16) \|\| ciphertext \|\| tag (32)`, with no version byte: `crypto/ecies/ecies.go` `symEncrypt` writes the IV ahead of the body and `Encrypt` writes `Rb \|\| em \|\| d` |
| Why that matters | the delivery construction differs only in where the IV comes from (the second KDF block rather than a random draw), so a decrypt that reads the IV from the wire opens a blob the node encrypted. The registration core's `decrypt_from` does exactly that |
| CID | CIDv1 raw sha2-256 of the blob bytes, rendered as `b` plus 58 lower-case base32 characters, which is what a manifest carries |

Who checks what:

| Side | Test | What it establishes |
|---|---|---|
| the CLI | `tools/fod/blob_vector_test.go` | the vector's nodekey derives the vector's secrets and author, the node's own library decrypts both parts to the vector's plaintexts, the CID matches, and the canonical secrets and commitment are the vector's |
| the registration core | `core/tests/blob_v1.rs`, 4 tests | it parses the protobuf, opens both parts with the secrets, renders the CID in the manifest's form, computes the same canonical secrets and commitment, and refuses a tampered, wrongly keyed, truncated, or non-protobuf blob |

The CLI gained `SecretsCanonicalBytes` and `SecretsCommitmentHex` for this: the
journal's key commitment is the same value the delivery proof binds, and until
now only the proof cores could compute it. Three implementations of that encoding
now exist - the two proof cores and the CLI - and all three are pinned by
`proofs/vectors/ecies-v1.json` and this vector rather than by inspection.

### F-74 The registration guest, its input layout, and what it cannot establish

Implementation: `proofs/registration/methods/guest/src/main.rs`, with
`proofs/registration/host/src/main.rs` building the input and checking the
result. The guest commits the 178-byte claim of F-69 and nothing else.

Private input, in the order the host writes it:

| # | Value |
|---|---|
| 1 | statement version (`u32`) |
| 2 | contract address (20) |
| 3 | network (string) |
| 4 | author address (20) |
| 5 | epoch (`u16`) |
| 6 | validation anchor (32) |
| 7 | consensus version (string) |
| 8 | eligibility policy (string) |
| 9 | the manifest's CIDs (strings, sorted) |
| 10 | the inventory: (flip index, CID, blob bytes) per covered flip |
| 11 | public-part secret (32) |
| 12 | private-part secret (32) |
| 13 | the assignment: (assigned flips, answers, grades) per participant |
| 14 | the covered flip indices |

What the guest establishes, and what it does not:

| `SPEC.md` 5.5 | Status |
|---|---|
| 2. every covered flip satisfies the policy | established: it assembles the committees, qualifies the covered flips, and refuses the input when any fails |
| 3. blob bytes match their authenticated CIDs | established: each blob is hashed and its CID compared with the inventory's |
| 4. the secrets decrypt the blobs | established: both parts are opened with the committed scalars, and the tag is verified before anything is decoded |
| 5. the decoded content has the supported structure | established only in the weak sense recorded below |
| 6. binding to network, author, epoch, manifest, protocol version | established: the journal carries the contract, author, epoch, manifest commitment, and the two label digests |
| 1. the manifest holds the complete inventory | **not** established by the proof. The guest checks that the manifest and the inventory name the same CIDs, which is consistency, not completeness against the chain. Completeness is checkable by anyone from the chain's submissions for the author and epoch, and the claim binds the manifest commitment so that comparison has a target |

The structure check is deliberately weak and recorded as such: both parts must be
non-empty, together within the node's 600 KiB blob cap, and valid UTF-8. A
stricter check of the flip's word-pair shape is not implemented, so no claim about
it is made.

Evidence:

| Check | Result |
|---|---|
| the journal | with an input built from the frozen vectors, the guest commits exactly the bytes of `claim-v1.json`'s journal, so the Rust guest and the Go claim builder agree end to end |
| four mutations | a tampered blob, a manifest CID the inventory does not carry, a covered flip the committee reported, and a wrong secret are each refused |

The reported-flip mutation is worth keeping in mind: the first attempt made every
participant report the covered flip, and the guest accepted it, correctly. Under
the 34% rule every participant had reported all of its own flips, so every report
was dropped as spam and the flip ended up unreported. The fixture now gives each
participant two other flips, so the reporters survive and the policy refuses the
bundle. That is the rules interacting, not a loophole in them.

### F-75 A RISC Zero image id is its `methods.rs` words packed little-endian

`methods.rs` carries a guest id as eight `u32` words. The canonical byte string
is each word's little-endian bytes concatenated, not the big-endian reading: the
delivery words pack little-endian to `2a4f2f7b21fe44fd…`, which is exactly the
`image_id` in `proofs/delivery/dist/delivery.json` and the byte array the
contract pins as `DELIVERY_IMAGE_ID`, while the big-endian packing of the same
words is the byte-swapped string `7b2f4f2a…`.

Evidence: the delivery host writes the id with `w.to_le_bytes()`
(`proofs/delivery/host/src/main.rs`), and the artefact it produced in the same
run carries that hex. The registration constant was first written from the
big-endian packing (`d04125a5…`, which is `a52541d0…` word-reversed), so the
contract computed a claim digest no receipt could commit and refused every
registration seal as `invalid_proof`. With the little-endian constant
(`a52541d0…`) the sealed registration receipt verifies in the runtime and the
bundle registers at 195,311,616 wasm gas
(`spikes/harness`, `TestEscrowRegistersABundleFromASealedReceipt`).

Rule for new pins: take the id from a produced artefact, or write it from
`methods.rs` with `to_le_bytes`, and never from a big-endian reading of the
words.

### F-76 What a contract call costs, and the measured gas and size per operation

Correction: the first version of this entry said the fee follows from the gas the
call consumes alone and that the max fee is therefore what the call costs. One
term was missing. The pinned node charges

```text
fee = feePerGas * size(tx) * 10 + feePerGas * gasUsed
```

`blockchain/fee/fee_calc.go` (`CalculateFee`, `CalculateGas`: ten gas per byte of
transaction, plus 67 bytes when the transaction is unsigned) gives the size term,
and `blockchain/blockchain.go` adds the receipt's `GasCost` after the call runs.
The `MaxFee` a caller signs is a budget, not the payment: the node derives the
call's gas allowance as `(MaxFee - sizeFee) / feePerGas` (`blockchain.go`
`getGasLimit`), validation refuses a max fee below the charged fee
(`blockchain/validation/validation.go:182-184` and `252-253`) and above the
block's gas cap (F-15) - and that cap is measured against the **whole** max fee,
not the headroom: `maxTxGas = MaxFee / minFeePerGas` must not exceed
`MaxBlockSize(enableUpgrade11)`, so a transaction whose size fee is large (the
release module's deploy, 357,858 bytes = 3.58e6 of the 5.12e6 allowance at a
rate of 1) has a narrow valid range. A lab node enforced exactly this
(`docs/PROGRESS.md`, "a local chain" entry).

So a call that consumes `W` wasm gas needs a max fee of at least
`sizeFee + ceil(W / 100) * feePerGas`, which is also what it can cost; the
runtime charges 100 wasm gas per user gas (F-8).

Measured per operation against the pinned runtime and this escrow: the highest
wasm gas a run consumed, and the size of the signed transaction the CLI produced
for that operation, with the runs that produced them:

Correction, 2026-09-26: the `registerOffer` figure was 195,304,165 before the
O-12 change replaced the guest program id and re-proved the sealed receipt. A
call that carries a proof is metered partly by the proof it carries - the three
delivery settlements below, all on one module, cost three different amounts - so
the figure moved with the artefact: the current module and receipt measure
195,478,684, under the pinned runtime and under the released one alike
(`SOURCES.md` F-90). The other five operations did not move.

| Operation | Wasm gas (highest) | Tx bytes | Runs | Runs used |
|---|---|---|---|---|
| `registerOffer` | 195,478,684 | 559 | 4 | registration from a real Groth16 receipt, direct and CLI-prepared, under the pin and under the released binding |
| `listBundle` | 106,204 | 167 | 1 | CLI listing calldata |
| `fundOffer` | 161,165 | 270 | 2 | CLI purchase and the direct fund test |
| `fulfill` | 195,632,417 | 811 | 4 | three settle runs with real delivery proofs and the CLI path |
| `refund` | 110,479 | 116 | 2 | direct and CLI-prepared refund |
| `withdraw` | 201,987 | 117 | 2 | direct and CLI-prepared withdrawal |

`fod fees` prints the gas, the size, and what each operation needs at the chain's
rate; `fod submit -fee-per-gas` prints the same estimate beside the max fee it
was given and warns when that max fee cannot cover it. The rate comes from one of
three places: `-fee-per-gas` (an exact integer), `-indexer URL` (the public
indexer's last block, `feeRate` in iDNA per gas), or the node's `bcn_feePerGas`
(`idena-go/api/blockchain_api.go:292`, `BlockchainApi.FeePerGas`).

Worth checking against a live node before it is trusted: the same node exposes
`contract_estimateCall`, whose receipt carries `minGasLimit` and
`minGasLimitCost` (`idena-go/api/contract_api.go:177-190` and `293-301`). Using
it needs the call's argument encoding (`CallArgs.Args`, `jserde`) reconciled with
the raw region bytes this project sends, which is not done here.

What that costs on mainnet, at the rate measured on 2026-09-25, is F-77. The gas
and size numbers are measurements of the local fork pin, so they inform the
estimate; they are not a promise about mainnet.

### F-77 The mainnet fee rate on 2026-09-25, and what each call costs at it

Read from public chain data, without a node login: the public indexer's last
block reported `feeRate` `0.0000961538461538` iDNA per gas at block 11,355,812,
epoch 228. That the rate is in the 1e18-scaled unit and not a smallest-unit
fraction follows from the node's own arithmetic: `FeePerGas` is multiplied by a
gas count and compared against a base-unit `tx.Amount`
(`blockchain/validation/validation.go`, the minimum-deploy-amount row in F-12,
and `fee.CalculateFee`).

At that rate, with F-76's measured gas and transaction sizes:

| Operation | Max fee / cost (base units) | iDNA |
|---|---|---|
| `registerOffer` | 188,497,788,461,447,982,600 | 188.50 |
| `listBundle` | 262,788,461,538,335,400 | 0.26 |
| `fundOffer` | 414,615,384,615,185,600 | 0.41 |
| `fulfill` | 188,887,980,769,140,103,000 | 188.89 |
| `refund` | 217,788,461,538,357,000 | 0.22 |
| `withdraw` | 306,730,769,230,622,000 | 0.31 |

`fod fees -indexer https://api.idena.io` reproduces this from the live rate.

What follows for the marketplace: the two calls that carry a proof dominate the
network cost. An author pays about 188 iDNA to register a bundle, and the seller
pays about 189 iDNA to settle a sale, so a bundle price has to cover roughly
377 iDNA of fees on top of the value being transferred. Listing, funding,
refunding, and withdrawing are all well under 1 iDNA.

Caveats: the indexer publishes thirteen significant digits, so its decimal
estimates the chain's integer rate (the difference is far below one gas unit per
call); the rate follows block usage and is therefore a point measurement, not a
constant; and the wall-clock such a call takes on mainnet is still unmeasured
(`O-9`).

### F-78 The deployed registry has no owner, and the chain gives none one

Two independent halves, both checked on 2026-09-25:

- Module half: the release artefact exports `deploy`, `listBundle`, `fundOffer`,
  `fulfill`, `cancelOffer`, `refund`, `withdraw`, `accounting`, the read views,
  `allocate`, and `memory`, and nothing else - no `terminate`, no `upgrade`, no
  owner setter, and none of the `test-hooks` entries.
  `spikes/harness/escrow_no_admin_test.go` reads the module's export section and
  compares the whole surface by name against that list, so an administrative hook
  added under a name nobody anticipated fails as well. The same test deploys the
  release artefact, shows that `accounting` executes, and requires `terminate`,
  `seedBundle`, `seedOffer`, and `setOfferCommitment` to be refused. Re-run it
  with `GOFLAGS=-modfile=go.local.mod go test -run
  TestReleaseModuleExportsNoAdministrativeEntryPoint ./...` in `spikes/harness`;
  last run 2026-09-25, pass.
- Chain half: `blockchain/validation/validation.go` `validateTerminateContractTx`
  returns `InvalidRecipient` whenever the target's code hash is outside the
  embedded contract set and `EnableUpgrade11` is on. That flag has been on since
  consensus version 11 (`config/consensus.go`, activation window 2023-03-01 to
  2023-03-07), so a custom contract cannot be terminated by anyone and no stake
  or balance can be recovered through termination.
- Address half: a contract address is derived from the code hash, an argument
  format byte, and the deployment nonce, so an address can only ever hold the
  code it was deployed with. There is no upgrade path to take over.
- The only privileged actions in the registry are per-offer and per-holder:
  listing, cancelling, and delivering for the address that owns that offer
  (`contracts/escrow/src/lib.rs`, the `owner` field of the offer record).

Consequence: a multisig or any other ownership hand-over is unnecessary, and
there is no entry point through which it could act. A deployer keeps nothing
beyond the module's public behaviour.

### F-79 The lottery assignment is derivable, and no node keeps it

Read in the pinned checkout on 2026-09-25. Package 2's eligibility half needs the
assignment because answers are positional, and the assignment is a function of
the lottery seed and the candidate list:

- Answers carry no flip identifiers. A short-answer attachment is
  `ShortAnswerAttachment{Answers, Rnd, ClientType}` with the answers as one opaque
  byte slice (`blockchain/attachments/attachment.go`), and the node stores what it
  collects as `DbAnswer{Addr, Ans []byte}` (`database/epoch_db.go`): one byte per
  assigned flip, in assignment order.
- The assignment is not persisted. `EpochDb` has accessors for the lottery seed,
  the lottery identities (address, state, public key, shifted shard, flip CIDs),
  answers, evidence maps, keys, own transactions, and flip CIDs - and none for
  the author-to-candidate or candidate-to-flip mapping, which the ceremony holds
  in memory (`vc.shardLotteries`).
- The seed is a committed header field. `handleFlipLotteryPeriod` takes the
  `FlipLotteryStarted` block height, subtracts `LotterySeedLag` (100), clamps the
  result to genesis+1, and reads `seedBlock.Seed()` (`core/ceremony/ceremony.go`),
  which returns the header's stored `BlockSeed` (`blockchain/types/types.go`).
  The RPC block DTO carries no seed field (`api/blockchain_api.go`), so reading
  one means opening the chain database rather than calling the node.
- The candidate list is the identity state at that block, filtered by
  `state.IsCeremonyCandidateData(state, hasDoneAllRequiredFlips)` and grouped by
  shifted shard (`core/ceremony/ceremony.go`, `core/state/state_object.go`). No
  node retains it: state versions older than `MaxSavedStatesCount = 100` are
  deleted (F-31).
- The distribution is derived in two passes. Authors are handed out round-robin
  to candidates from a queue shuffled with `seed64*21-77`; a shard with more than
  seven authors gets a second pass that tops every author up to
  `CandidatesPerAuthor` (13) from a permutation seeded with `seed64*77+55`; the
  short and long sessions of each candidate come from a permutation seeded with
  `seed64*12+3` (`core/ceremony/lottery.go`). `seed64` is the little-endian first
  eight bytes of the seed, so the whole assignment follows from the seed and the
  candidate list.

Consequence: a node that is live at a `FlipLotteryStarted` block yields the seed,
the candidate list, and the answers, and the assignment is then computed from
them rather than read from anywhere. `tools/fod/lottery.go` is that computation,
written against these rules rather than translated from the pinned file, and
`proofs/registration/vectors/lottery-v1.json` holds six cases captured by
copying `core/ceremony/lottery.go` and the two candidate record types into a
scratch module outside the checkout and running them unchanged; the copy is not
committed, because it is LGPL-3.0 (F-11). The cases reach both distribution
passes, a shard with no authors, an empty shard, and a single-flip shard.

### F-80 Long answers bind to the long-session assignment, and a short payload is not an error

Read in the pinned checkout on 2026-09-25, alongside F-79.

- A flip's verdict comes from the long session. `qualifyFlips` is called once per
  shard with `shard.longFlipsPerCandidate` and `totalFlipsCount = len(shard.flips)`
  (`core/ceremony/ceremony.go`), so the assignment and the flip indexes are both
  local to that shard's flattened flip list.
- The flip count of a payload comes from the assignment, not from the payload.
  For every candidate the node takes `flipsCount := len(flips)` and decodes
  `types.NewAnswersFromBits(flipsCount, attachment.Answers)`
  (`core/ceremony/qualification.go`), so position `j` of the payload answers flip
  `flips[j]`.
- The payload is a big-endian integer, so leading zero bytes drop off and a
  payload can be shorter than the five-bits-per-flip ceiling
  (`blockchain/types/types.go`, `Answers.Bytes`). Missing bits read as zero
  rather than as a malformed payload, and bytes past the bits the node reads are
  ignored.
- Short answers are not part of a flip's verdict. They score the candidate
  (`qualifyCandidate`), and the eligibility policy reads flip verdicts only.

Consequence: `tools/fod/epochjoin.go` reads a payload narrower than the ceiling
and refuses one wider than the candidate's assignment. The node ignores the extra
bits; for a reconstruction they mean the answers belong to a candidate with a
longer assignment, which is exactly the mistake the refusal has to catch.

### F-81 The public explorer's per-epoch endpoints, and which field is a commitment

Read from `https://api.idena.io/api` on 2026-09-26, and read again from this
repository's own client (`tools/fod/indexer_live_test.go`, epoch 227: 158
identities, 107 of them with answers; epoch 200 and 227 for the per-identity
route below).

| Endpoint | What it answers | Use |
|---|---|---|
| `GET /Epoch/{n}` | `validationTime`, `stateRoot`, `validationFirstBlockHeight`, `minScoreForInvite`, `candidateCount`, `discriminationStakeThreshold` | the epoch's state root is the one field comparable with a node's own header, at the epoch's *opening* block and not at `validationFirstBlockHeight` (F-82); `candidateCount` is a hint, not a list |
| `GET /Epoch/{n}/Blocks?limit=&continuationToken=` | the epoch's blocks, newest first: `height`, `hash`, `timestamp`, `txCount`, coin supply, `bodySize`, proposer and VRF fields | ordered newest first, so the first entry of epoch `n-1`'s list is the block epoch `n` opened from - which is how the opening height is resolved without a second source |
| `GET /Epoch/{n}/Identities?limit=&continuationToken=` | per identity: `address`, `prevState`, `state`, `shortAnswers` and `longAnswers` (point, flipsCount), `shortAnswersCount`, `longAnswersCount`, `approved`, `missed`, `requiredFlips`, `madeFlips`, `availableFlips`, `totalValidationReward`, `birthEpoch` | the counts are a consequence of the epoch's assignment, so they check one and cannot replace it; the list is sorted by address, not by the ceremony's order |
| `GET /Epoch/{n}/Flips?limit=` | per flip: `cid`, `author`, `epoch`, `status`, `answer`, `wrongWords`, `grade`, `gradeScore`, `timestamp`, `size`, `withPrivatePart` | the `(cid, author, epoch)` triple is comparable with the chain's own submission; the derived fields are the explorer's reading of the same data this repository reconstructs |
| `GET /Epoch/{n}/Identity/{address}/Flips?limit=&continuationToken=` | the same per-flip record as the epoch-wide route, for one author | the status and the answer are the one explorer field that checks a *reconstruction's verdict* rather than its inputs, which is what `fod eligibility -indexer` compares |

What the endpoints do not provide: the epoch's lottery *assignment*, and the
ceremony's candidate *order*. Those exist only in a node that processed the
`FlipLotteryStarted` block (F-79), which is why an indexer is a Plan B source and
not a replacement. The per-identity record carries status and answer but no
shard, so it cannot be turned into a candidate list either.

Measured while reading them: two paths answer `{"error":{"message":"wrong value
limit="}}` without a `limit`, `/Epoch/{n}/Candidates` is not a route, and an epoch
the explorer does not hold answers `{"result":null}` for identities. Recorded on
2026-09-26 while adding the per-identity route: `/Epoch/{n}/Flips` ignores an
`author=` parameter (the answer carries other authors' flips), `/Epoch/{n}/Answers`
is not a route (404), the per-identity route omits `continuationToken` entirely on
its last page rather than sending it empty as `/Identities` does, and the statuses
observed in both routes are `Qualified`, `WeaklyQualified`, `NotQualified` and the
empty string, with answers `Left`, `Right` and `None`.

### F-82 An explorer's epoch state root belongs to the epoch's opening block

Measured on 2026-09-26, while epoch 228 was open, from `https://api.idena.io/api`
and from a live mainnet node over its own RPC (the node's `dna_epoch`,
`bcn_blockAt`). It corrects a wording this repository had carried as an
assumption: that an epoch's `stateRoot` is comparable with the header at the
epoch's `validationFirstBlockHeight`. It is not.

| Epoch | explorer `stateRoot` | `validationFirstBlockHeight` | root of the header there | root of the header at the epoch's opening block |
|---|---|---|---|---|
| 227 | `0x40a2a96afe397bf012f4775b2219a49bb7c81bca524060fb309add031f2480ec` | 11350664 | `0x03db419e8ca1ac61deae8859610b61f9e643863ed9d7f88f0170d742ce244e79` - differs | 11328931: `0x40a2a96a...` - matches |
| 228 | `0x82ae49458bd20c5c32df52423eba099e672e2a62093c5612e8e747d235ca9044` | 0 (not due yet) | - | 11350781: `0x82ae4945...` - matches |

- The node names the same height for the epoch it is in: `dna_epoch` returns
  `startBlock`, which is `State.EpochBlock()`, and for epoch 228 that is 11350781
  - the block whose root the explorer publishes as epoch 228's state root.
- The explorer's block list gives it too: `/Epoch/226/Blocks?limit=1` answers
  `11328931` and `/Epoch/227/Blocks?limit=1` answers `11350781`, each being the
  newest block the explorer lists for that epoch. The lists are ordered newest
  first, and the block an epoch opened from is filed under the epoch before it.
- The two epochs agree on both sources, so the tooling treats the explorer's
  `Epoch/{n}` state root as the root of the block at `Epoch/{n-1}`'s newest
  listed height, and lets a caller name that height instead
  (`fod ... -epoch-block N`) rather than taking the explorer's word for it.
- `validationFirstBlockHeight` remains what its name says - where the epoch's
  validation began - and is not a commitment to anything this repository checks.

Verified live by `tools/fod/node_root_live_test.go`, which asks the node which
epoch it is in and where that epoch started, reads the explorer's root for the
same epoch, and requires the node's own header at that height to carry it:
`TestLiveNodeConfirmsTheExposersEpochRoot` passes against api.idena.io and a
mainnet node (epoch 228, block 11350781).

### F-83 A transaction's shard is a local cache, not chain data

Read in the pinned checkout on 2026-09-26, while checking whether a closed epoch's
candidate list could be rebuilt from chain data after all - F-31 names "their
shard ids" among the inputs a sweep would supply:

| Item | Evidence |
|---|---|
| the node's RPC transaction DTO has no shard field | `api/blockchain_api.go:106-120` - hash, type, from, to, amount, tips, maxFee, nonce, epoch, payload, blockHash, usedFee, timestamp |
| the field is an unexported cache, so it is not encoded | `blockchain/types/types.go:164-187`: `shardId atomic.Value` sits below `UseRlp` with the other caches; `SetShardId`/`LoadShardId` (924-933) store and read it. An unexported field is not RLP-encoded, so committed block bodies do not carry it either |
| it is filled from the state at submission | `core/mempool/txpool.go:372` `tx.SetShardId(appState.State.ShardId(sender))` |
| the key packages do not carry it either | `PublicFlipKey.ToBytes()` (`blockchain/types/types.go:1207-1219`) serializes `Key`, `Epoch`, and `Signature` only; `shardId` is another cache (1233-1251) |

Consequence: a candidate's shard is state (`Identity.ShardId`, reassigned at epoch
transitions by `blockchain.go:998-1030`) and is mirrored nowhere on chain, so no
sweep - RPC or raw committed bodies - can group a closed epoch's candidates by
shard. That is on top of the members the sweep cannot see at all: a candidate who
submitted neither flips nor answers leaves no trace in the epoch's transactions,
while the lottery still walks it in `firstDistribution` and `extraCandidates`.
F-34's conclusion stands, now with the mechanism named: a capture from a node
that was live during the epoch is the only source for the candidate list.

### F-84 A flip's verdict turns on two switches, and the covered epochs start at 104

Read in the pinned checkout and from the public explorer on 2026-09-26, to answer
`SPEC.md` §12's O-2 - which epoch range the pinned rules cover, and how strong
qualification is encoded per epoch.

| Item | Evidence |
|---|---|
| the qualification path uses exactly two consensus switches | `core/ceremony/qualification.go` references `EnableUpgrade11` at lines 167, 173, 203, 398, 427 and 434, and `EnableUpgrade10` at line 403. Nothing else in that file reads the config or the epoch |
| the epoch-conditional flags elsewhere are not about flips | `core/ceremony/ceremony.go:1149` passes `vc.epoch >= 93`, `EnableUpgrade10` and `EnableUpgrade12` into `determineNewIdentityState`, which decides identity states; `EnableUpgrade12` also appears in block timing and skip-receipt handling (`blockchain/blockchain.go:730`, `741`, `1420`, `1453`, `1636`) |
| both switches come from consensus V11 | `config/consensus.go:152-157`: `ConsensusV11` sets `EnableUpgrade11`, inherits `EnableUpgrade10` from V10, and carries `KeyWordsV3Epoch = 104`. Activation windows: V10 2022-12-23T08:00Z to 2022-12-30T00:00Z, V11 2023-03-01T08:00Z to 2023-03-07T00:00Z, V12 2023-07-10T08:00Z to 2023-07-20T00:00Z |
| the boundary is epoch 104 | measured from `https://api.idena.io/api`: epoch 103 validated 2023-03-13T13:30Z and spans block 5,664,108 (2023-02-28T14:04Z) to 5,720,711 (2023-03-13T14:03Z), so V11's window closed inside 103; epoch 104 validated 2023-03-25T13:30Z. The version's own `KeyWordsV3Epoch = 104` names the same boundary |

Consequence: the arithmetic this repository ports is valid for epoch 104 and
later, and `tools/fod` refuses earlier epochs rather than applying it to them
(`ConsensusEpochFloor`, `requireSupportedEpoch`). Later versions do not change a
flip's verdict: V12's switch is not read by the qualification path, so a bundle's
eligibility is the same arithmetic from 104 onwards.

### F-85 Mainnet runs consensus 12, which the pinned checkout carries

Read on 2026-09-26 from a mainnet node's own journal while it replayed the chain
on the test host, to settle `SPEC.md` §12's assumption A-1 before any mainnet
exercise:

| Observation | Where it comes from |
|---|---|
| the node transforms its consensus config to `ver=10`, then `ver=11`, then `ver=12`, and no further | the compatibility node's journal: `Consensus config transformed to ver=NN` at 2026-09-24T02:02Z, 02:11Z and 02:28Z; the node's head is then the chain's head (11,356,613 against the explorer's 11,356,612), so the range it replayed and stayed valid on covers every activation mainnet has had |
| the pinned checkout defines consensus versions 10, 11 and 12 and nothing later | `config/consensus.go`: `ConsensusV10`, `ConsensusV11`, `ConsensusV12` with their activation windows |
| the pinned parsers read real mainnet data | epoch 204's sweep (437 submissions, 1,285 bodies CID-verified, answer attachments decoded) and a live mainnet node's epoch store read for the capture rehearsal (F-82) |

Consequence: the revision this repository derives protocol facts from carries the
rules mainnet currently executes, so the qualification arithmetic this release
ports (F-84) is the arithmetic mainnet applied to the epochs the release claims
to cover.

### F-86 A flip's CID depends on its size, and only the node's own IPFS can decide it

Read on 2026-09-26 from mainnet's own records and from a live node's IPFS, because
the inventory compares every fetched blob with the CID the chain recorded:

| Observation | Where it comes from |
|---|---|
| the split sits exactly at one chunk: 26 of epoch 228's 34 flips are at most 262,144 bytes and carry a CIDv1/raw/sha2-256 value (`bafkrei…`), the other 8 are larger and carry a CIDv1/dag-pb value (`bafybei…`), with no flip contradicting it | the explorer's `/Epoch/228/Flips?limit=100` answer: `size` against the CID's codec, 34 records, 0 mismatches |
| the pinned node produces both forms from the same call, because it imports through go-unixfs's defaults with `CidVersion(1)` and sets neither a chunker nor raw-leaves: a file that fits in one chunk becomes one raw leaf, a larger one a UnixFS root over raw leaves | `idena-go` `ipfs/ipfs.go`: `api.Unixfs().Add(ctx, file, options.Unixfs.Pin(pin, ""), options.Unixfs.CidVersion(1))` |
| the node's own IPFS is the authority on the question, and it answers: asked for the CID of the 307,089 bytes it serves for `bafybeid2xqbxzmrhfwfejhdzjv4newxeixvl5eljacbye5sxzgzxicdp34`, the live node's `ipfs_cid` returns exactly that CID, while the sha256 of the same bytes is not that CID's multihash | one `ipfs_get` and one `ipfs_cid` call against the node's RPC |

Consequence: `tools/fod/cid.go` decides a blob's CID with the node's `ipfs_cid`
when a node answers, and with a local sha256 only for a raw CID - a dag-pb CID
that nobody imported is recorded unverified rather than accepted
(`verifyBlobCID`). Two parts of the release still assume the raw form and are
therefore blocking for any flip larger than one chunk: `isFlipCID` in
`tools/fod/manifest.go` rejects the dag-pb form when a manifest is built, and the
registration guest compares against `blob_cid_multibase` computed from
`cid_v1_raw_sha256` (`proofs/registration/methods/guest/src/main.rs`). That is
recorded as a package-level blocker in `PROGRESS.md`, not worked around.

Superseded later the same day by F-88 and F-89: the importer's construction is
reproduced in `tools/fod/unixfs.go` and checked against a node's own answers, so
a dag-pb CID is decidable without a node, and the registration guest carries
whichever form a blob's size implies. Neither `isFlipCID` nor the guest refuses
the dag-pb form any more, and `SPEC.md` O-12 is closed.

### F-87 The explorer's flip record carries the transaction anchor its list leaves out

Read on 2026-09-26 while building the Plan B route `SPEC.md` 5.3 permits:

- The per-identity list (`/Epoch/{n}/Identity/{address}/Flips`) names the CID, the
  author, the epoch, the size, the status and the answer, and nothing that ties a
  CID to a transaction: it cannot be checked against the chain on its own.
- The per-flip record (`/Flip/{cid}`) adds `txHash`, `blockHash` and
  `blockHeight` beside the same author, epoch and size. Those three fields are
  what makes the claim checkable by someone who runs a node: the block hash is
  the header the node holds at that height, and the transaction is the node's own
  copy, whose type (`submitFlip`), sender, epoch and payload CID must all agree
  with what the explorer published.
- One live example, epoch 228: `bafybeid2…` is listed with
  `blockHeight 11351745`, a block hash, and the transaction
  `0x54a914e5…`; both were confirmed against the node's own header and
  transaction store.

Consequence: the Plan B listing (`tools/fod/indexer_inventory.go`) records those
anchors per entry, checks each one against a node when the caller names one, and
refuses a contradiction instead of preferring the explorer. The epoch's state
root is checked in the same pass (F-82), and a listing no node confirmed is never
recorded as verified: `prepare-offer` refuses it, because completeness still
rests on a committed-body sweep.

### F-88 The importer's exact construction, reproduced and checked against the node

Read on 2026-09-26 from the pinned checkout's own import path and from a live
mainnet node, because F-86 left a caller with no node unable to decide a flip
over one chunk:

| Observation | Where it comes from |
|---|---|
| the node computes a CID with `coreunix.NewAdder` plus `options.UnixfsAddOptions(options.Unixfs.CidVersion(1))`, so the chunker and raw-leaves settings are that option set's defaults | `idena-go` `ipfs/ipfs.go`, `Cid` (`options.UnixfsAddOptions(options.Unixfs.CidVersion(1))`) |
| a file of at most 262,144 bytes is one raw block; a larger file is a dag-pb root whose links are raw leaves, and the root carries the UnixFS `File` data (type, total size, per-block sizes) | `boxo` `ipld/unixfs/importer/balanced` and `helpers.FillNodeLayer`, which add each leaf with `AddNodeLink("", child)` and commit `n.file.GetBytes()` as the node data |
| the serialized block has links before data, each link carries the leaf's CID, a **present but empty** name, and `Tsize` equal to the leaf's byte count; the root's data is `Type=File(2)`, `filesize`, and one `blocksizes` entry per leaf | `github.com/ipld/go-codec-dagpb` `marshal.go`, and a two-leaf root read out of the pinned importer: `122c 0a24 01551220…dc05ce96 1200 18808010 122a 0a24 01551220…0fb68fde 1200 1801 0a0c 0802 18818010 20808010 2001` |
| the construction is right, not merely plausible: a live node's `ipfs_cid` returns the same CID for blobs the tool built the DAG for | five sizes answered by the node - 262,144 (`bafkreieiuj5mzeuqor24l4lmo2yumm6665yv4gq7ozzp2nrfmqrjyboosy`, raw), 262,145 (`bafybeich6cqd6yabw3cku6edwmz4rvwe275nx2l6ootvc2bpbt5bdvarua`), 524,288 (`bafybeid5sthfzhlnifk5fvivasemgw2rzzfdp54rapknx7eaz5j4yoae3i`), 524,289 (`bafybeiag5snf6cdu2gfoobd6elsfvq3tsujp3gztgpvulhyzxo3ax6e2vy`), 614,400 (`bafybeihr2fddcxgyaodxyumttzp2zioqpjezbsynz7v3rwsrh3ogrhnliy`) - and all five are reproduced by `tools/fod/unixfs.go` (`unixfs_test.go`) |
| it is right for real chain data too: all five dag-pb flips epoch 228's compat node submitted reproduce their recorded CIDs from the fetched bytes | the five blobs saved by the live Plan B run, checked against `unixfsCID` |
| a flip can never need a second DAG level: the node refuses a flip larger than `common.MaxFlipSize` = 614,400 bytes, which is three chunks | `idena-go` `common/network.go`, enforced in `core/flip/flipper.go` |

Consequence: `verifyBlobCID` decides a dag-pb CID by rebuilding the DAG locally
and asks the node's IPFS only when the rebuild does not reproduce the CID - the
case where a client imported with other options - and a CID neither check
confirms is still reported unverified rather than accepted. The remaining
raw-form assumptions are in `isFlipCID` and the registration guest, not in this
check (`SPEC.md` O-12).

### F-89 The registration guest carries the same rule, so either form can register

Done on 2026-09-26, because F-88 left the raw-form assumption in the proof
program and the bundle our own exercise identity holds is one of the affected
ones:

| Observation | Where it comes from |
|---|---|
| the guest's CID rule now matches the importer's: `blob_cid_multibase` is the raw leaf while a blob fits in a chunk and the dag-pb root over raw leaves once it does not, and it refuses a blob over `MAX_FLIP_BYTES` | `proofs/registration/core/src/blob.rs`; the guest reads it in `methods/guest/src/main.rs` |
| the Rust implementation is checked against the same node answers the Go one is, not against itself | `proofs/registration/core/tests/unixfs_v1.rs` reproduces all five recorded sizes (`cargo test -p fod-registration-core`: 2 passed) |
| the shipped receipt was re-proved with the new program | `cargo run --release --bin registration-host -- --prove`: 933.8 s, 7,602,176 cycles in 8 segments, 178-byte journal, 256-byte seal |
| the new identifiers | image id `046c8ff8fa9520b1c6670099ff17254a14d522ffd3789f38fab5d08089c58ee4`, claim digest `e46cefabfc80c1afc079641ae4d94a83c1fa4a5038271b4908217c87f670ca6c`, both pinned by `contracts/escrow/src/verifier.rs` and checked against the proved artefact by `release/build-manifest.mjs` |
| the contract still accepts it, in the runtime | `spikes/harness`: `TestEscrowRegistersABundleFromASealedReceipt` and `TestEscrowAcceptsCliPreparedRegistration` pass; the CLI-prepared call costs 195,478,684 wasm gas for the new module and receipt (the 195,304,165 sibling figure belongs to the pre-change pair; `F-76`'s correction) |

The delivery program is untouched, so its image id and receipt are unchanged.

### F-90 Mainnet's runtime is the upstream archive set, and the module behaves on it

Read on 2026-09-26 to close `SPEC.md` O-10 - whether the runtime revision the
release measured is the one mainnet executes - from the released binary outward:

| Observation | Where it comes from |
|---|---|
| the release the network installs is `idena-go` **v1.1.2** (latest stable, 2025-12-22); its binary records the binding it links | `go version -m` on the published `idena-node-linux-1.1.2` asset: `github.com/idena-network/idena-wasm-binding v0.0.0-20230503080211-4227b9778d3d` |
| that binding vendors static libraries built from upstream `idena-wasm`, whose last commit is `9fc402e` (`v0.0.30`, 2023-05-03) - the runtime source has not moved since | the upstream repository's commit list and tags; the binding at `4227b9778d3d` ships `lib/libidena_wasm_*.a` rather than the archive manifest later revisions carry |
| the revision this project measured is **ahead of that**, in a fork: `ubiubi18/idena-wasm` `7e113895` (2026-07-10), 29 commits on top of `9fc402e` | `lib/ARTIFACTS_SOURCE` of the pinned binding `f7c71ce3`, and the compare list of the two revisions (Wasmer hardening, CI, dependency locks, and changes in `exports.rs`, `imports.rs`, `environment.rs`, `memory.rs`) |
| the parts that decide what a contract is charged and what it may execute are unchanged: `src/costs.rs` differs only in whitespace (`cost_function` returns 1, `BASE_DEPLOY_COST` 3,000,000, `BASE_CALL_COST` 100,000, `BASE_BYTES_TO_HEX_COST` 10,000), and outside its own tests `src/gatekeeper.rs` differs only in an import order and comment indentation | diff of both files between `9fc402e` and `7e113895` |
| it does not merely compile there: the whole escrow harness passes against the released binding, and the registration call is metered **identically** - 195,478,684 wasm gas under the released binding and under the pin, for the same module and receipt | `spikes/harness/go.official.mod` in a `linux/amd64` container (`go list -m` names `v0.0.0-20230503080211-4227b9778d3d`; `go test -count=1 ./...` `ok`, 9.0 s) against the same test run natively with `go.local.mod`; both log 195,478,684 |
| the figure this replaces: 195,304,165 was measured on the module and receipt from before the O-12 change, which replaced the pinned image id and re-proved the seal together - a difference of +174,519 gas belongs to that artefact change, not to the runtime | `git show 16a3703:contracts/escrow/dist/fod_escrow.wasm` and the sealed receipt both differ from the current ones |

Consequence: the runtime mainnet executes is the upstream archive set, and the
release's numbers hold on it exactly - the same module deploys, registers,
settles and refunds, and the metered cost is identical because the fork's
changes are in the engine and its dependencies, not in the operator costs or
the rules that admit a module. The project keeps the fork pin for its own
harness and carries `go.official.mod` beside it so the same suite can be run
against the runtime the network runs, which is what the O-10 claim rests on.

### F-91 The registration guest's image id follows its sources, comments included

Found on 2026-09-26 while correcting a stale comment in
`proofs/registration/core/src/lib.rs`: the wording of the doc comment around
`cid_v1_raw_sha256` no longer described the two-form rule, the edit was made, and
`cargo test -p registration-host --test sealed_artefact` - the test that verifies
the shipped receipt against the guest built from the working tree - failed.
Restoring the exact text made it pass again.

So the id is bound to the guest's sources at the level of text and line
positions, not only to its compiled behaviour, and the consequence is a rule for
this release rather than a curiosity:

- `proofs/registration/dist/registration.json`, the escrow's
  `REGISTRATION_IMAGE_ID`, `release/MANIFEST.json`, and the CLI's
  `registrationImageIDHex` are one set: any edit under `proofs/registration/`,
  a comment or a blank line included, invalidates the shipped receipt and
  requires a re-prove (about 16 minutes) and a re-pin of all three copies.
- That is why the O-12 change, which altered one constant, re-proved the
  artefact instead of reusing it (F-89).
- The correct response to a comment-only improvement in that crate is to leave
  it alone until there is a reason to re-prove. The comment this fact came from
  was reverted to its exact text, and the sealed receipt verifies against the
  working tree again.

### F-92 A custom module needs no deploy stake, only its fee

Read on 2026-09-26 from the pinned node's deploy validation, because the exercise
has to know whether deploying the registry carries a stake that would never come
back (`SPEC.md` 1 asks for exactly this to be verified rather than assumed):

| Observation | Where it comes from |
|---|---|
| the minimum amount is checked only for code hashes in the node's own `embedded.AvailableContracts` set: `if _, ok := embedded.AvailableContracts[attachment.CodeHash]; ok { minStake := FeePerGas * 3_000_000; if tx.AmountOrZero() < minStake { return InvalidDeployAmount } }` | `idena-go` `blockchain/validation/validation.go`, `validateDeployContractTx` |
| the escrow's code hash is not in that set - it is a module the deploy carries, admitted by the `EnableUpgrade11` branch below the check | same function, with `release/artifacts/fod_escrow.wasm` as the artefact |
| nothing else in the deploy path charges an amount: the rest of the function checks the recipient (a deploy has none), the attachment, and the upgrade flag | same function |

Consequence: the deploy's cost is its fee, not a stake. At the rate mainnet
charged on 2026-09-26 (0.0000961538461538 iDNA per gas), the 357,858-byte deploy
transaction costs about 347 iDNA - ten gas per byte for size plus the gas the
deploy runs - and `tx.Amount` may stay zero. The "verified requirement to fund
necessary contract stake; otherwise zero" row of `SPEC.md` 1 is answered in the
"otherwise" direction for this module.

### F-93 A block header commits to a session's evidence, and the node can already prove part of it

Found on 2026-09-26 while answering whether a bundle can be bound to a real
validation session rather than to a publisher's word. The pinned checkout
carries the ingredients of that binding; the registration guest's verdict
arithmetic is the part already proven.

| Ingredient | Where it is committed | Evidence |
|---|---|---|
| the flip submission | the transaction attachment carries the blob CID and its pair, and the transaction sits in the block body, whose IPFS CID is a header field | `blockchain/attachments/attachment.go:145` (`CreateFlipSubmitAttachment(cid, pair)`), `blockchain/types/types.go:123` (`IpfsHash`), `protocol/full.go:191` (`GetBlock` fetches the body by that CID and decodes its transactions) |
| the answers of the session | the short and long answers are transaction types, and the long-answer attachment carries the answers bytes with the VRF proof, salt, and key | `blockchain/types/types.go:22` to `:24`, `core/ceremony/ceremony.go:318` (`CreateLongAnswerAttachment(answers.Bytes(), proof, salt, key)`) |
| the identity record a shard assignment is built from | identities live in their own tree, committed by the header as `IdentityRoot`, keyed by `StateDbKeys.IdentityKey`; the ledger-side record, flips included, sits in the state tree under the same key shape | `core/state/identity_statedb.go:29` (`NewLazyIdentityState`), `core/state/identity_statedb.go:253` (`s.tree.Get(StateDbKeys.IdentityKey(addr))`), `core/state/statedb.go:1557` (`GetIdentityWithProof`) |
| the lottery seed | the header carries the block seed and its VRF proof, which the chain re-derives | `blockchain/types/types.go:129` (`SeedProof`), `blockchain/blockchain.go:2861` (`ProofToHash(seedData, Header.SeedProof)`) |

Consequences for a registration proof that has to survive a dishonest seller:

- Submission inclusion needs no Merkle-path implementation of its own: the
  body's CID is in the header, so checking the body bytes against that CID and
  reading the transaction out of them is the inclusion check, with `TxHash` as
  the second binding.
- The assignment is per shard: `core/ceremony/ceremony.go:261` computes a
  candidate's flips from that shard's candidate list and its flips, so the proof
  input is the shard's candidate identities, each provable against the state
  root, plus the seed.
- Not verified yet: the wire format and verification cost of an identity proof
  outside the node, the guest's cost for a shard's worth of them, and whether
  the anchor should be the verifier's own header or a checkpoint. The guest
  already turns assignment, answers, and committee data into verdicts
  (`proofs/registration/core/src/qualification.rs`), so what is missing is
  authentication, not arithmetic.

### F-94 What a default node keeps, and what the session binding may assume

Read on 2026-09-26 from the pinned checkout while recording the owner's rule that
the binding has to work for people who run default nodes, keep only some flips,
and need the legacy chain untouched.

| Observation | Evidence |
|---|---|
| fast sync is the default | `config/config.go:280` (`FastSync: true`), `config/sync.go:6` |
| blocks and flips are pinned only up to a quantile, and pinning more is possible | `config/ipfs.go:33` to `:34` (`BlockPinThreshold: 0.3`, `FlipPinThreshold: 0.5`), `ipfs/ipfs.go:276` to `:283` (`ShouldPin`), `ipfs/ipfs.go:494` (`Pin`), `ipfs/ipfs.go:195` (`GC`) |
| historical identity state survives on no node | `core/state/statedb.go:39` (`MaxSavedStatesCount = 100`); F-31 records that the pruning constant is compile-time, so `fast sync: false` does not keep an old epoch's state either |
| a kept header outlives the pruned state it commits to | F-31 and `SPEC.md` 5.3: the state root stays checkable against the header long after the state is gone |

Consequences for the registration path:

- Verification may rely only on the header and its committed roots, a portable
  proof bundle carrying the membership and inclusion proofs, and blob bytes
  reachable through a pinned window or a pinner. It must not need historical
  state and must not need a node that was live during the epoch.
- Producing that bundle is the scarce step: it needs a live capture at the epoch
  or a replay of the chain. Once produced, the bundle is small and portable, and
  keeping the blobs needs pinning beyond the default thresholds.
- Both are service roles off the protocol. Whether the reserve pays for them is
  an owner decision; the protocol itself needs no consensus change, no new
  transaction type, and no host function.

### F-95 The identity proof's shape, and the wire format it already has

Read on 2026-09-26 while measuring `O-15`'s identity proof against the pinned
checkout and its IAVL replace pin.

| Fact | Evidence |
|---|---|
| the state tree is IAVL | `core/state/tree.go` imports `github.com/cosmos/iavl` and wraps `iavl.MutableTree` |
| identities live in their own IAVL tree behind `IdentityStateDbKeys` (`LoadDbPrefix(db, false)`), and the ledger-side record sits in the state tree under the same key shape | `core/state/identity_statedb.go:29` to `:35`, `core/state/identity_statedb.go:253`, `core/state/keys.go:128` |
| a membership proof for one identity is an IAVL range proof | `core/state/statedb.go:1557` (`GetIdentityWithProof`), `core/state/tree.go:309` (`GetWithProof`), and the replace pin's `proof_range.go:524` |
| the pinned fork verifies it against the root hash | `github.com/idena-network/iavl` (in `go.mod`), `proof_range.go:179` (`RangeProof.Verify(root)`) |
| the serialized form already carries what verification needs | `core/state/tree.go` `valueWithProof.toBytes`: the value, the leaf's key, value hash and version, then each path item's height, size, version, left and right (`protobuf/tree.models.pb.go`: `ValueWithProof`, `ValueWithProof_Leaf`, `ValueWithProof_Item`) |

Consequence: the anchor bundle needs no new encoding. The node already produces
`ValueWithProof` bytes, and a guest can either decode them back into the fork's
proof types or hash the same fields itself. The measured size on a real datadir
is recorded in `docs/PROGRESS.md`.

### F-96 The contract can read and verify a canonical session header itself

Read on 2026-09-26 from the pinned checkouts and measured in the runtime, to
settle `REVIEW.md` A02's first requirement: the anchor must not be a prover's
root, an explorer response or an operator checkpoint.

| Fact | Evidence |
|---|---|
| the host surface already carries a header lookup | `idena-wasm-binding/lib/callbacks.go:90` (the callback typedef), `:163` (the callback table), `:662-671` (`cblock_header` calls `api.host.BlockHeader`): the guest imports `env.block_header(height)` |
| the node answers from its canonical chain | `idena-go/vm/wasm/wasm_env.go:72-79` (`BlockHeader` calls `headerProvider.GetBlockHeaderByHeight` and charges `GasToWasmGas(costs.ReadBlockGas)`); the provider is the chain itself: `blockchain/blockchain.go:2623-2629` reads `repo.ReadCanonicalHash(height)` then the header |
| the price | `vm/costs/costs.go:8` (`ReadBlockGas = 5`) with `WasmGasMultiplier = 100` (`:23`), so one header read costs 500 wasm gas before parsing |
| the encoding is protobuf, not RLP | `blockchain/types/types.go:504-506` (`Header.ToBytes` is `proto.Marshal(h.ToProto())`); field numbers are in `protobuf/models.pb.go`: `ProtoBlockHeader` carries `proposedHeader = 1` and `emptyHeader = 2`, the proposed message `height = 2`, `root = 6`, `identityRoot = 7`, `flags = 8`, `blockSeed = 12`, the empty message `height = 2`, `root = 3`, `identityRoot = 4`, `blockSeed = 6`, `flags = 7` |
| the block hash is keccak of the sub-message | `types.go:683-686` (`ProposedHeader.Hash` = `crypto.Hash(proto.Marshal(h.ToProto()))`) and `crypto/hash.go:17-21` (`crypto.Hash` is `sha3.NewKeccak256`) |
| the ceremony flag | `blockchain/types/types.go:53-64`: `FlipLotteryStarted` is `1 << 1` |
| measured in the runtime | `spikes/header-anchor` fetches, parses and verifies a 730-byte proposed header in **147,537 wasm gas** (0.03% of the 512,000,000 ceiling); `spikes/harness/header_anchor_test.go` refuses a wrong block hash, state root, identity root or seed, a height with no header, a skip block, a proposed block without the lottery flag, and three malformed encodings (truncated, over-long field, both shapes present) |

Consequence for the registration anchor: the contract can hold the session
binding itself. Because every node that validates the transaction answers
`block_header` from its own canonical chain, a receipt whose journal carries a
height, block hash, state root, identity root and seed either matches the
chain's own header or aborts - no checkpoint publisher, no explorer and no
consensus change. The guest's job becomes folding the evidence to exactly those
roots; the contract compares them. The fixture is encoded by the node's own
protobuf package but carries synthetic field values; a captured real header
replaces it when a capture lands.

### F-97 The state commits an epoch's flips and its verdict, at two neighbouring heights

Read on 2026-09-26 from the pinned checkout while looking for the cheapest
committed evidence that a bundle really belongs to a session. The chain stores
the flips and the verdict in the *same identity record* but never in the *same
state version*, so a bundle needs two anchored heights - and nothing else: no
candidate list, no lottery assignment, no answer replay.

| Fact | Evidence |
|---|---|
| the record carries the epoch's flips | `core/state/state_object.go:429-432` (`IdentityFlip{Cid, Pair}`), `:452-453` (`Flips []IdentityFlip`); a flip-submit transaction writes one through `StateDB.AddFlip` (`core/state/statedb.go:448`, called from `blockchain/blockchain.go:1680`) |
| the record carries the verdict flags | `core/state/state_object.go:434-440` (`AtLeastOneFlipReported`, `AtLeastOneFlipNotQualified`, `AllFlipsNotQualified`), written as `LastValidationStatus` by `SetValidationStatus` (`core/state/statedb.go:556`, `state_object.go:1277`) |
| the flags are computed from the epoch's author results | `blockchain/blockchain.go:750-768` (`calculateNewIdentityStatusFlags` reads `AuthorResults.HasOneReportedFlip`, `HasOneNotQualifiedFlip`, `AllFlipsNotQualified`) |
| the same transition clears the flips | `blockchain/blockchain.go:926` (`ClearFlips` and `ResetValidationTxBits` inside `setNewIdentitiesAttributes`), which only runs on a `ValidationFinished` block (`blockchain.go:707-716`) |
| the epoch counter turns in that same block | `blockchain.go:738` (`appState.State.IncEpoch()`), so the flips sit in versions whose global `Epoch` is N and the flags in versions whose global `Epoch` is N+1 |
| the global object is a committed entry | `core/state/state_object.go:277-295` (`Global.Epoch`), written under `StateDbKeys.GlobalKey()` (`statedb.go:725-731`) |
| the record lives in the state tree, not the identity tree | `statedb.go:1557-1558` (`GetIdentityWithProof` reads `s.tree` at `StateDbKeys.IdentityKey(addr)`); `tools/fod/stateproof.go:313-318` opens `identity` or `state` by prefix |
| a leaf is only readable near its height | `core/appstate/appstate.go:244-252` and `blockchain.go:460` commit one version per block height; `statedb.go:39,1308-1316` deletes everything past `MaxSavedStatesCount = 100`, so a proof has to be produced within about 100 blocks of its height |
| a fast-synced node has no older headers | `protocol/fast.go:113-140` starts consuming at the preliminary head's height + 1, so `block_header` for a height below the sync floor returns nothing; the anchor check fails closed there |

Consequence: a session bundle can be two membership proofs -
the author's identity leaf at a height inside the flip phase (complete `Flips`,
`Epoch == N`) and the same author's leaf at the `ValidationFinished` block
(verdict flags, `Epoch == N+1`) - each folded to its header's state root and
each header authenticated by the contract. The lottery assignment, the candidate
list, the answers and the committee arithmetic are not needed to sell a bundle,
and the per-flip verdict is not on chain at all: the provable label is the
bundle's (no reported / all qualified / weak), which is what `REVIEW.md`'s label
lane describes. The capture at both heights is the scarce, off-protocol service
step; verification needs nothing but the retained headers and the two proofs.

### F-98 The flip content format the client's renderer reads

Read on 2026-09-27 from the pinned IdenaAI checkout - the client whose validation
screen the session UI follows - to close `SPEC.md` 5.5 item 6: the decrypted
parts have to be a flip a customer can actually see and choose between.

| Fact | Evidence |
|---|---|
| the client writes two nested RLP parts | `renderer/screens/flips/utils.js:241-258` (`flipToHex`: the public part is `RLP([[image, image]])` over the first two pictures, the private part is `RLP([[image, image], [order, order]])`, and both are rendered as `0x`-prefixed hex); `main/flip-generation-runtime.js` uses the same encoding for scheduled flips |
| the client reads them the same way | `renderer/screens/validation/machine.js:1729-1765` (`decodeFlip`: `[images] = decode(publicHex)`, `[privateImages, orders] = decode(privateHex)`, then `images.concat(privateImages)`; the single-part `hex` branch is the older form and is not what a client writes today) |
| the RPC wrapper is hex but the encrypted plaintext is raw RLP | `api/flip_api.go:38-41` types `publicHex` and `privateHex` as `hexutil.Bytes`; `common/hexutil/json.go` decodes their `0x` JSON strings to bytes, and `api/flip_api.go:88-97` passes those bytes to `PrepareFlip` |
| four pictures, two orders | `renderer/screens/flips/utils.js:27-28` (`FLIP_LENGTH = 4`, `DEFAULT_FLIP_ORDER = [0,1,2,3]`) and `:215-237` (`shufflePics` builds two permutations); the renderer shows `images[order[i]]` (`renderer/shared/utils/arr.js:29-35`, `reorderList`) |
| an order entry is an RLP integer | `renderer/screens/validation/ai/test-unit-utils.js:108-116` and `machine.js:1751` normalise an entry as `([idx = 0]) => idx`: the byte itself for 1..3, and the empty string RLP writes for 0, which is why the default is 0 |
| the frozen vector | `proofs/flip/vectors/flip-v1.json` retains the earlier flat RLP wrapped in ASCII hex as a compatibility fixture; `proofs/flip-core` also tests the renderer's canonical nested form, and the registration guest tries raw RLP first |
| the fixture blob | `proofs/registration/vectors/blob-flip-v1.json` seals the compatibility vector with the epoch keys of `blob-v1.json` and the node's own ECIES construction (`proofs/delivery/core/src/lib.rs` `encrypt_to`, the mirror of the decrypt the guest runs), generated by `proofs/flip-fixture` with fixed ephemeral scalars. The real epoch-230 five-flip and three-flip witnesses exercise the canonical raw nested form |
| regenerating the session fixture | `tools/fod/session_bundle_regenerate_test.go` (`FOD_REGENERATE_SESSION_VECTOR=1`) rewrites `proofs/session-core/fixtures/session-bundle-vector.json` over the new CID; a submitted flip carries the CID as the multibase text the node stores, which is what the record and the manifest have to share |

Consequence: the content check is a shape check, not a durability check - it
says the four pictures and the two orders are there and in range, so a renderer
can show them and a validator can choose between them. Whether the bytes stay
fetchable is the availability advisory's job (5.8), and whether the pictures are
meaningful is the validator's.

### F-99 A submission stops being valid when the lottery period starts

Read on 2026-09-28 from the pinned checkout, to close the temporal half of the
inventory claim: membership in the state tree at some height is not evidence
that the epoch's flip set was final at that height.

| Fact | Evidence |
|---|---|
| a flip submission is refused from the lottery period on | `blockchain/validation/validation.go:349` (`if appState.State.ValidationPeriod() >= state.FlipLotteryPeriod { return LateTx }`) |
| the period values | `core/state/state_object.go:146-152` (`NonePeriod` 0, `FlipLotteryPeriod` 1, `ShortSessionPeriod` 2, `LongSessionPeriod` 3, `AfterLongSessionPeriod` 4) |
| inside one epoch the period only advances | `blockchain/blockchain.go:1211-1231`: the four ceremony flags set the four periods, and `ValidationFinished` is the only transition that sets `NonePeriod` again |
| that same transition advances the epoch | `blockchain/blockchain.go:738` (`IncEpoch`), which is F-97's `ValidationFinished` block; so no version carries `Epoch == N` with `NonePeriod` after epoch N's ceremony |
| the period is committed state | `core/state/state_object.go:277-284` (`Global.ValidationPeriod`), written by `SetValidationPeriod` and committed under `GlobalKey()` beside the epoch F-97 records |
| the one exception, outside the covered range | `blockchain/blockchain.go:3202-3206` resets the period as a hotfix at one fixed 2022-10-22 block hash; the covered range starts at epoch 104 (F-84), and V11's activation closed inside epoch 103, so supported epochs have the monotone rule |

Consequence, now enforced: the inventory anchor's global record must report
`validationPeriod >= FlipLotteryPeriod`. `proofs/session-core/src/session.rs`
refuses anything else (`Refusal::InventoryStillOpen`), and the CLI refuses the
same capture before a receipt is built (`tools/fod/sessionbundle.go`). An anchor
that could still be followed by a submission can therefore neither be captured
nor registered, and the verdict anchor keeps its own `Epoch == N+1` check,
which only a `ValidationFinished` transition produces. No journal field changed,
so the escrow's offsets, the CLI's pinned identifiers and the manifest are
unaffected; the guest's own image id follows its sources (F-91) and moves at the
next regeneration.

### F-100 The version 3 statement publishes what the fold hid

Read on 2026-09-28 while closing the two gaps the earlier passes named: a buyer
could not check that a seller's advisory is the bundle's committed inventory
(the C1 finding in `PROGRESS.md`), and the contract could not refuse a statement
made for another network.

| Field | Offset | Bytes | What it is for |
|---|---|---|---|
| inventory commitment | 198 | 32 | `Sha256("fod.bundle-manifest:v1" \|\| author \|\| epoch \|\| 0x00 \|\| cid...)` over the sorted manifest CIDs (`proofs/session-core/src/statement.rs:154`, `tools/fod/export.go:81`), so a holder of a candidate set computes the same value |
| network digest | 294 | 32 | `Sha256` over the canonical network name; the escrow holds the digest of `idena` and refuses a statement whose field differs |

Version 2 carried both values only inside other commitments: the inventory
commitment was folded into the manifest commitment, and the network was one of
three strings length-prefixed into that fold
(`proofs/registration-v2/methods/guest/src/main.rs:218-227`). Neither could be
recovered from a journal by any reader, which is what the C1 finding records.

| Reader and check | Where |
|---|---|
| the escrow refuses `journal[294..326] != NETWORK_DIGEST` as `foreign_network`, and stores the journal's inventory commitment in the pool bundle | `contracts/escrow/src/lib.rs` (`registerBundle`, `PoolBundle`) |
| the pool record republishes it as `inventoryCommitment`, so a buyer reads it from chain state | `contracts/escrow/src/lib.rs` (`poolBundle` view), `tools/fod/market.go` |
| the buyer's tool recomputes the commitment over the advisory's CIDs and refuses a set that does not match, before anything is fetched | `tools/fod/pool_buy.go` |
| the seller's CLI checks the same network digest against the string this release pins | `tools/fod/prepare_registration.go` (`NetworkNamePinned`) |

The statement grew from 262 to 326 bytes and its version became 3; the escrow
refuses the earlier version, so the sealed receipt had to be re-proved
(`PROGRESS.md`, the package entry that records the run). A build for another
network replaces `NETWORK_DIGEST` with that network's digest.

What the check does and does not do: a statement proved for a differently named
network cannot register, which is the domain separation the audit asked for. It
does not defeat a byte-identical replica of the same chain, because every value
the contract can read - the anchors' headers, the current epoch, the global
state - is identical on such a replica. That residue belongs to the verifier's
own network choice, not to a check the runtime can make.

## 4. Unverified statements carried over from the plan

These appear in `SPEC.md` as requirements, but were **not** verified in this
session. They must be closed with evidence in the package named in
`PROGRESS.md`.

| Statement | Must be verified by |
|---|---|
| RISC Zero Groth16 verification can run inside the Idena contract runtime within its limits | Package 1 |
| An epoch's flips share one encryption key set, so a single flip cannot be sold in isolation | Package 2 |
| The full qualification and reporting process can be reproduced from authenticated chain inputs for the supported epoch range | Package 2 |
| The deployed mainnet contract runtime settles transfers the way the pinned revision does: debit inside `create_transfer_promise`, execution at the end of the call, no callback for a transfer | Package 5, before the mainnet release (tracked as `O-10` in `SPEC.md`) |
| Whether a transfer promise may fail after the debit, which would need credit recovery that the pinned runtime gives no hook for | Package 5, before the mainnet release |
| iDNA amount handling and fee estimation as displayed to the user | Package 5 |

## 5. Handling rules for these sources

- The two reference worktrees and the IdenaAI repository are read-only inputs.
  Do not commit, clean, reset, or otherwise modify them from this repository's
  work.
- Any new pin (tool version, crate version, commit) discovered during
  implementation is added to section 1 or 2 with the evidence that produced it.
- When a statement moves from section 4 to section 3, record the file and line
  numbers that justify it.
