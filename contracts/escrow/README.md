# Escrow contract

The ownerless pool of Flips on Demand, written against the Idena contract ABI
that Package 1 validated (`docs/SOURCES.md` F-12): `no_std`,
`memory`/`allocate`/`deploy` exports, Region-based arguments and returns, host
functions imported from module `env`.

## Build

```bash
# Test build: adds the seedPoolBundle/seedPoolOrder hooks so the accounting can
# be exercised without a registration proof or a buyer's signature.
cargo build --release --target wasm32-unknown-unknown --features test-hooks
mkdir -p dist
wasm-opt --llvm-memory-copy-fill-lowering -O3 \
  target/wasm32-unknown-unknown/release/fod_escrow.wasm -o dist/fod_escrow.testhooks.wasm
```

`wasm-opt` is required: the runtime gatekeeper rejects the bulk-memory operators
Rust's precompiled `core` emits (`SOURCES.md` F-14).
The artefact carries `.testhooks` in its name because the seed hooks can mint
units and settle seeded orders; the deployment artefact is built without the
feature and lives in `release/artifacts/fod_escrow.wasm`.

The experimental fast path is a separate `fast-candidate` build. Its
`registerFast` method uses the reproducible `FAST_CONTENT_IMAGE_ID` pin and a
genuine synthetic seal checked in the Idena WASM harness. This fixture is
placeholder-bound; a live author needs a separate contract-bound proof.
This build omits the legacy registration, delivery, confirmation, proof
fulfilment and public reveal exports so they cannot settle a fast order.
An included `fulfillFast` retains its complete public delivery proof for buyer
verification and recovery. A successful `registerFast` also retains its entire
bounded content seal and public session witness at the `pfcert/<bundle hex>`
state key, so an independent client can retrieve and replay the certificate
without relying on a historical transaction body. Registration gas with a real
sealed witness still needs measurement before deployment.
For the synthetic harness only, build with both `fast-candidate,test-hooks`,
lower the WASM into `spikes/fast-escrow-candidate/dist/`, then run
`TestFastEscrow` in `spikes/harness`. Never deploy a test-hooks build.

## What this contract does

Per `SPEC.md` 5.1, 7.1, 7.2 and 10.3:

- Supply: `registerBundle` (a sealed, session-bound registration receipt),
  `addReserve` and its donor-owned exit `withdrawReserve`, `donateUnit`,
  `sellUnit` (refused, see below), `beacon`, and `poolView`, over the one global
  pool and the `PoolBundle` record with the arithmetic in `pool.rs`.
- Demand: `buy` (the pool's quote, the retrieval-fee list, the delivery-key
  possession proof bound to the bundle and the per-bundle nonce, and the order
  it opens), `deliver`, `confirm`, `fulfill` (the delivery-proof path),
  `reveal` (the committed secrets), `refund` (the one-transaction repayment and
  the strike), `orderPoolView`, `withdraw`, and `accounting`.

The beacon's freshness window is 288 blocks. The transitional offer slice was
removed on 2026-09-26; the release module exports only this pool surface plus
`allocate`, `deploy`, and `memory`
(`TestReleaseModuleExportsNoAdministrativeEntryPoint` compares the whole export
set, not a deny-list).

`registerBundle` verifies a sealed registration receipt against the claim its
pinned guest program produces, refuses a caller who is not the author the
receipt names, and creates the pool record the sale units attach to
(`SOURCES.md` F-74, F-89). The `seedPoolBundle`/`seedPoolOrder` test hooks exist
only in the `test-hooks` build, for the accounting paths that do not need a
proof; the release artefact contains no test hook.

## Rules encoded

- One global pool: `reserve`, `units` and `pending` are market-wide state; a
  `PoolBundle` keeps only its inventory, commitments, beacon, strikes and one
  buy nonce.
- The pool's product rule: `r` iDNA stands against `q` sale units and a buyer
  pays `ceil(r/(q-1))`; the sell quote `floor(r/(q+1))` stays refused because
  the payout path is off. Both quotes round in the pool's favour, so no round
  trip returns more than it paid.
- A refused mutation aborts the call instead of returning `{"ok":false}`: the
  node discards a failed call's storage and value, so a rejected payment cannot
  stay behind (`REVIEW.md` A04).
- There is no subsidy: `sellUnit` is compiled off (`SUPPLY_PAYOUTS_ENABLED`),
  and the only way a unit enters is the registered author's own `donateUnit`
  (`SPEC.md` 10.3).
- Value attached to a call whose rule prices nothing is refused rather than
  stranded.
- A published delivery is a candidate, not an assignment: a later `deliver`
  replaces a pending payload until the order settles, so junk published first
  cannot deny the fast path (`REVIEW.md` A06).
- The buyer's `confirm` carries the exact ciphertext they checked and the
  escrow records it as the delivery, so a payload someone replaced after the
  check cannot strand the payment (`AH-10`).
- A buy moves exactly one unit out and puts the exact payment - the quote plus
  the retrieval fee, capped by the authenticated covered count - into the pool's
  pending escrow, never into the reserve; a wrong amount is refused without
  moving the nonce.
- `confirm` and `fulfill` settle one order; `reveal` settles every open order
  on the same bundle atomically before publishing their shared keys. At most
  eight orders may be open on one bundle. The purchase amount is paid
  to the bundle's registered author, the retrieval fee becomes the named hosts'
  withdrawable credit, and the reserve is untouched.
- `addReserve` is permissionless and the transaction must send the amount it
  names; the donated reserve anchors the quote and the pool never spends it.
  Each deposit is recorded against the address that paid it, and
  `withdrawReserve` hands that donor's contribution back while the reserve still
  covers it and the balance still covers every buyer escrow and host credit.
- A beacon older than 288 blocks blocks new buys until it is refreshed.
- One missed delivery takes the beacon down; the second clears the listing, and
  a cleared listing cannot be bought again or receive new donated units.
- Successful reveal settlement also retains its public preimage at
  `preveal/<orderId>`. A settled buyer can recover keys from canonical state
  without the original desk or a transaction-history indexer. This does not
  retain the encrypted flip content; the normal buyer keeps it before payment.
- A reveal publishes the bundle's secrets for good: the listing stops selling,
  its remaining units leave the one global count, and new buys or donations are
  refused. All open orders on that bundle settle in that call, with a retained
  recovery record for each buyer. If any has reached its refund boundary, the
  reveal refuses without exposing keys; each missed order remains refundable.
  A prior refund also permanently bars public reveal of that bundle's shared
  keys, because the refunded buyer retained authenticated encrypted content.
- Unilateral delivery, proof settlement and reveal require a block strictly
  before `funded+3`; permissionless refund starts at `funded+3`. Explicit buyer
  confirmation may settle an open order later. This separates canonical
  settlement/refund eligibility; it cannot prevent pre-inclusion key exposure.
  One permissionless `refund` pays the original buyer the purchase amount plus the retrieval fee
  directly, with no credit step, and counts a strike. The unit a buy consumed
  stays consumed.
- A withdrawal pays the caller's whole credit, and an unbacked credit aborts the
  call with the credit intact; `accounting` publishes `reserveTotal`,
  `pendingTotal`, `creditTotal`, `unitsTotal` and `liquid`, so
  `liquid == reserve + pending + credits` is checkable without iterating
  storage.
- No owner, upgrade hook, terminate path, or commission exists.
