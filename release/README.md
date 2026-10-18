# Historical release experiments

**Current status (2026-10-18):** The separate direct-v7 one-seed candidate
completed one owner-operated, funded two-identity mainnet pilot, including a
private sale, buyer challenge, seller fallback, and public proof and reveal.
Its candidate artifact is recorded in `DIRECT_V7_MANIFEST.json`. This does not
qualify other contracts or a general release. The authorization and readiness
statements below describe earlier prototypes, not the current v7 pilot.

This document records earlier contract candidates and a separate experimental
v6 build recipe. It does not authorize a deployment. Some recipes and fee
observations refer to older code and may no longer work as written. Start with
the repository README and verify the exact
current source, artifact, chain state, and fees before any funded experiment.

The manifest and artifacts record an earlier dirty-source snapshot. Current
source and interface files have changed since that snapshot; rebuild and recheck
the selected contract before deployment. One separately approved, unfunded
mainnet deployment and a read-only contract view succeeded; that deployment is
not qualified for funded use. Historical 2026-09-29 lab
receipts cover an earlier module and interface: registration, fee-carrying
purchase, verified delivery and confirmation/reveal/proof/refund settlement.
The two lab nodes were configured independently on one host; their agreement
is not independent-operator availability evidence. See `docs/PROGRESS.md` for
source-specific measurements and remaining blockers, including pre-inclusion
key exposure, canonical encrypted-content availability and timely private
proof delivery.

One owner-approved mainnet deployment occurred without a deposit. Further
mainnet actions and funding require separate authorization. `docs/SPEC.md` 10.3
supersedes 10.2, and the bounded deploy smoke-test allowance recorded in
`docs/PROGRESS.md` section 0 remains suspended. The recipes below build and
validate artifacts; they are not approval to execute an on-chain action.

## Experimental v6 candidate build

`node release/build-direct-v6-candidate.mjs` builds lowered v6 WASM and
`release/DIRECT_V6_MANIFEST.json` offline. It checks the reproduced fast-content
image, sealed synthetic fixture, exact exports, and deploy, activation, and
fallback gas in the pinned Idena WASM runtime. The manifest records source and
code hashes. This is candidate evidence only; a real seller proof, deployment,
and funded trade remain unqualified.

The bootstrap sequence is a zero-value deploy with the two seed offer IDs.
`fod prepare-deploy` requires `-v6-seed-ids` with those two 32-byte IDs
concatenated as hex and refuses a nonzero deployment amount.
The node omits a zero amount from the signed transaction. V6's zero-value
methods therefore do not call its `pay_amount` import, which fails on an absent
amount. Send exactly zero to them: extra value is not added to the tracked
pool reserve and may be unrecoverable.
Each original author qualifies and proves their own bundle, then makes its
verified keys public. Only after both seeds are public can `activate` accept
the same 64-byte seed ID argument and exactly 10,000 iDNA. Trading remains
closed until activation completes.

The current escrow retains settled public reveal keys for recovery and keeps
the exact funded+3 refund boundary. Those changes do not retroactively modify
existing deployments or qualify historical receipts for this artifact.
Reserve-funded supply remains off by the accepted policy ("no subsidy at all").

## Experimental direct-sale contract

The current source targets a proof-backed direct-v4 candidate. The deployed
direct-v2 and direct-v3 contracts are immutable; their saved artifacts,
manifests, and trade records remain historical evidence.
The included direct-v4 deployment record identifies the exact module included
on mainnet and independently observed by both test nodes. Deployment alone
does not verify a funded v4 trade or a real seller fallback.

`node release/build-direct-candidate.mjs` rebuilds
`release/artifacts/fod_direct_escrow_v4.wasm` and
`release/DIRECT_V4_MANIFEST.json`
from the separate `contracts/direct-escrow` crate. It checks the export surface,
forbidden WASM operators, a fresh registration-v2 guest build against its
sealed fixture, settlement,
refund ordering, and separate proof-start, key-reveal, and deploy gas in the
pinned Idena WASM runtime.
`fod prepare-deploy` requires the exact manifest hash and measured deploy gas
for this module. This candidate requires manifest version 4 and a combined
fallback cost within the seller reimbursement cap, with each transaction below
the block gas limit. The deployed direct-v2 and direct-v3 artifacts keep their
own names. The older pool and fast candidate remain separate contracts.

A seller uses `fod direct-list` with a canonical two-anchor session bundle and
an encrypted vault of the **original epoch keys**. The listing commits to the
complete CID inventory and keys, and sets a price. It does not claim the seller
has already generated a slow proof. The buyer uses `fod direct-buy` to verify
both canonical headers, fetch and retain every original encrypted blob, and
lock the price plus the refundable proof bond quoted at the current chain fee
rate. `fod direct-dm` writes an order-bound, encrypted delivery
text file for the seller to send through a private channel such as idena.social.
No CLI command sends or reads social messages. `fod direct-receive` imports that
text, rechecks the order's chain state, decrypts the original blobs, verifies the
supported flip structure, and saves an encrypted vault and signed local receipt.
For a cooperatively settled order, it additionally matches the ciphertext to
the immutable on-chain delivery hash and only permits a local save. Its per-order
`-auto-confirm -broadcast` option submits buyer confirmation only after those
checks; otherwise `fod direct-confirm` prepares confirmation separately. Buyer
confirmation pays the price to the seller and returns the bond to the buyer.
For a verification preview, `fod direct-check` performs the same funded-order
and original-flip checks and writes a display-only result. It neither saves a
vault nor issues a receipt that `direct-confirm` can use; run `direct-receive`
before confirming payment so the buyer retains the keys. `direct-check` can
also inspect a cooperatively settled order when the message ciphertext matches
the delivery hash recorded on-chain.
The buyer can pass decrypted idena.social message text on standard input with
`-message -`; no message channel is accessed by the CLI.

If the buyer does not confirm, the seller can generate a proof and use the v4
`fod direct-start-fallback` action after a 30-block confirmation grace period.
This first seller transaction carries the sealed proof and a refundable stake
at least the greater of the price and 100,000 gas at the inclusion fee rate.
Buyer confirmation and proof start race for inclusion; if confirmation wins,
the seller bears any proof-generation cost. The proof transaction puts a
public statement on-chain without disclosing the keys. The
contract verifies the proof against the funded offer, complete CID inventory,
canonical roots, pinned guest image, and its own address before it marks
fallback proof-verified. The local registration-v2 host runs with
`-registration-host`, without the vault passphrase or RPC credentials in its
environment. Proof starts are refused in the last 120 blocks of the window.

The seller must verify canonical proof-verified state before preparing or
broadcasting the key-bearing `fod direct-fallback` transaction. That
transaction publishes only the committed original epoch keys and settles
before `fundedAt + 8640` blocks. The seller receives the price, the return of
its stake, and capped proof-cost reimbursement calculated from the fee rate
at proof start; unused buyer bond returns to the buyer. If the seller never
proves or reveals in time, `fod direct-refund` returns the price and full buyer
bond to the buyer, plus the seller stake if proof start was included. The full
window is approximately 48 hours at a 20-second block cadence; exact heights
are shown by
`fod direct-status`. Key reveal requires a loopback RPC for pre-broadcast
simulation. Its public mempool calldata exposes keys before inclusion; if
censored until the deadline, the buyer may obtain them and still refund. This
experimental route accepts that risk. A synthetic sealed fixture qualifies
only the local candidate checks, not a real seller claim.
After an included seller fallback, `fod direct-recover` reads the public keys
from contract storage and checks them against the buyer's retained original
blobs and canonical session before writing a local vault. A direct-market
snapshot marks these orders as having public keys; it never exports the keys.

The first direct-v4 mainnet trade is recorded in `DIRECT_V4_TRADE.json`.
The seller listed three canonical epoch-230 originals; the buyer funded a
1 iDNA purchase and a refundable proof bond, verified all three originals
from an owner-only encrypted record, saved a vault and signed receipt, and
confirmed. Both identity nodes observed successful inclusion and the same
settled order, returned buyer bond, reusable listing, private keys, and zero
contract balance. The exact encrypted record was later sent once through
idena.social, and the recipient identity decrypted a copy with the same SHA-256
hash. The deployed recipient app then verified all three original flips using
the retained purchase and delivery key. Because the message was sent after
settlement, this trade does not test message-triggered confirmation or a live
seller fallback.

On 2026-10-12, companion IdenaAI commit `513dab05` was pushed and installed
on both identity consoles with separate builds that retained their existing
social UI differences. After a lost confirmation RPC response, the buyer app
now recognizes only an exact pending confirmation transaction. Otherwise it
shows an unverified state and keeps the no-retry marker. The 18 focused tests
and both social UI builds passed. An initial staging-permission error was
repaired; the final source and asset hashes, Electron load, node synchronization,
and scheduled target were verified. No new funded order, message-triggered
confirmation, or real seller fallback was tested by this client update.

On 2026-10-12, companion IdenaAI commit `7b2dd10d` and the `fod direct-buy`
CLI from commit `855071a` were installed on both identity consoles. The Linux
CLI SHA-256 is
`8606243ff2528026cacc9cb6bbe75907657cc99cbe903f2a3d17c87a6888e776`;
the installed source files matched that companion commit, and both restarted
consoles returned HTTP 200 with synchronized, distinct identity nodes in their
own service cgroups. A live-node preparation against the seller's canonical
three-flip bundle quoted the 1 iDNA price and the contract-required refundable
bond. The buyer commit path reached its native confirmation, which was canceled
for this check; no purchase transaction was submitted. The app stores a
no-retry marker before any approved purchase call and resumes saved purchases
only for the active identity. A funded purchase through this new UI, delivery
before confirmation, and the real on-chain seller fallback remain unverified.

Companion IdenaAI commit `e9d51ada` was pushed and installed on both consoles
on 2026-10-12. Purchase recovery now binds the saved transaction to its
canonical funding block before identifying a funded or settled order. If two
orders still match, it reports the inclusion without guessing an order ID or
resubmitting the purchase. The renderer displays a recovered settled order.
The focused service tests passed (30/30), and the changed renderer parsed.
Both consoles restarted with the expected source and CLI hashes, separate node
processes and service users, synchronized chain heads, and the pinned contract
code. The first node needed a second restart after a transient IPFS port
collision; both nodes passed the final checks. This is a recovery and release
check, not a new funded purchase or seller fallback test.

Companion IdenaAI commit `3dd71462` was pushed and installed on both consoles
on 2026-10-12. The desktop client can cycle through all saved direct purchases
for its active identity after a restart, including an older pending attempt.
Delayed buy results are ignored after the active identity changes. A transient
temporary-file read failure occurred on the first test run; the isolated case
and complete focused suite passed on rerun (30/30). The renderer parsed. Both
consoles restarted from the exact source hashes, with separate synchronized
nodes, the pinned CLI and contract
code, and unchanged on-chain offer and order state. This did not submit a new
purchase or exercise the seller fallback.

On 2026-10-11, one direct-v1 three-flip listing, funded purchase, and buyer
confirmation were included on mainnet. Both identity nodes reported the same
settled order, zero remaining escrow, and the seller's exact 1 iDNA payment.
The encrypted delivery text was transferred as an owner-only file; an actual
idena.social message was not sent or received in this test. A real seller
fallback proof and a live `proveAndReveal` call remain unverified.

The deployed direct module has no buyer dispute bond. At the fee rate observed
for this test, its conservative fallback gas budget would cost about 164 iDNA
to recover the 1 iDNA purchase price. A buyer who withholds confirmation can
therefore force an expensive proof or wait for a refund; refund permanently
retires this author-and-epoch listing on this module. Public fallback also
reveals the shared epoch keys. Treat this deployment as an experimental
cooperative path, not as an economically safe small-trade market.

The bonded direct-v2 contract requires the buyer to lock a
bond of at least 2.5 million gas at twice the fee rate when `buy` executes.
The CLI quotes 25% above that minimum to absorb a moderate fee rise between
preparation and inclusion; the contract stores the actual funded bond. Signing
rechecks that the amount is within the current minimum and quote. The release
gate measured the fallback below the 2 million gas seller
reimbursement cap. The bond is returned on buyer confirmation or refund. After
a valid public content proof, the seller receives only the price plus at most
that cap at the then-current fee rate; the buyer receives the unused bond. The
locked bond can be much larger than the listed price. A later fee rise above
the margin, a censored fallback, or unavailable proof inputs can still leave
the seller unpaid; the bond does not guarantee settlement. The contract
cannot prove whether the seller sent the earlier private message. A seller can
choose public fallback and impose its proof cost on the buyer, even after
withholding private delivery; the cap prevents collecting the unused margin.
This contract does not impose a proof preflight before listing. For larger
inventories, proof generation may outlast the 48-hour block window, leaving only the
buyer's refund path. The seller bears that timing risk. Its order layout and
offer ID domain differ from direct-v1. The mainnet v2 contract, code hash, and
public transaction references are recorded in `release/DIRECT_DEPLOYMENT.json`.
Clients must check that code hash on their own node before spending funds.

On 2026-10-11, a v2 seller listed three canonical epoch-230 flips and a buyer
funded the 1 iDNA price plus a 520.833333333125 iDNA refundable bond. The
buyer imported an encrypted owner-only file, verified all three original flips,
and confirmed. Both identity nodes reported successful inclusion, the same
settled order, and zero remaining contract balance. The buyer's delivery key
remained private. The seller's balance reconciled to the exact 1 iDNA payment
after deployment and listing gas costs. The listing remained available for
another buyer. A real seller
fallback proof and on-chain claim remain unverified; the sealed fallback and
deadline were verified in the pinned local runtime only.
The companion social UI bridge was installed on both identity consoles from
commit `0daf04ba`, and both restarted consoles passed their node and UI health
checks. It imports the delivery file into a single-buyer private message draft.
The sender console received the direct-message fee-cap update from social
bridge commit `bab947c9` and passed its restart and UI health checks. The
seller then sent the approved encrypted delivery record once for the already
settled order. Its `sendMessage` transaction succeeded on mainnet, and the
buyer console displayed the decrypted delivery JSON in the matching private
conversation. The reviewed maximum fee was 10.8388 iDNA, below the new
20 iDNA direct-message cap. The buyer console then received the local
`Check flip keys` bridge from companion commit `8af22637` and ran the
`f7938ff` direct-check CLI against the decrypted message in that conversation.
The check matched the settled order's on-chain delivery hash, the retained
purchase and canonical session, and all three original flip blobs; the UI
reported three original flips verified locally. This was a display-only check:
it did not create a confirmable receipt, save a key vault, submit a new
purchase, or exercise the seller fallback.
Companion commit `abb6572c` retries social node initialization after startup
synchronization. In a buyer-console restart check, direct messages became ready
and the same conversation appeared without a manual reload.
The buyer-side `direct-receive` CLI then processed that exact record against its
live node, verified the same three original flips, and wrote an encrypted vault
and signed receipt to temporary owner-only storage. The deployed app service
passed the same live-node check after companion commit `6904f7fe` fixed parsing
of Go's indented receipt JSON. Both temporary vaults were removed. No persistent
buyer vault was saved through the UI, and this post-settlement check did not
submit another transaction or exercise fallback.

After a seller generates a proof for the deployed direct-v1 module, the registration host's
`--verify ARTEFACT --contract ADDRESS` mode checks its Groth16 seal against
that module's immutable image ID and the expected address without
rerunning the multi-hour prover. This check does not submit a fallback or
disclose keys. A fresh build of the current registration guest produced a
different image ID from that deployment's pin; the source and sealed fixture
must be reconciled before another contract release can claim reproducibility.

Every direct command requires `-network idena`, the independently
checked `-contract` and `-code-hash`, and the intended identity's own node.
The buyer's delivery key must differ from its signing identity. `fod help`
lists the commands; `fod submit` still requires explicit nonce, epoch, fee,
signer, output file, and `-broadcast` for an on-chain action. Estimate a call
from the signing identity's node: the pinned node version simulates a foreign
sender without that sender's signature, so its WASM `caller` is zero.

The separate Linux x86-64 operator CLI from commit `e0114dc` now rejects
nonce gaps and insufficient maximum transaction cost before a direct broadcast.
It saves the signed transaction and its locally derived hash before sending,
so an unanswered node response can be reconciled without signing or sending
again. The server copy is root-owned, remains separate from the buyer app's
pinned verification binary, and has SHA-256
`beb7b16aaeb96f13baaa3562abad8ccd5021ddd4265e399059a6a654192d7f4d`.
Its build used Go 1.26.8 and a local Idena node checkout at `9bc5704` with
uncommitted toolchain metadata; the installed binary hash is the deployment
pin, not a source-only reproducibility claim. Its startup and focused
submission tests passed. No real v4 fallback transaction has used it yet.

## Experimental fast contract candidate

`node release/build-fast-candidate.mjs` builds the separate
`release/artifacts/fod_fast_escrow.wasm` and `release/FAST_MANIFEST.json`.
It requires two clean builds of the fast-content guest, the matching image pin
and synthetic sealed receipt, the production export surface, and successful
synthetic registration and deploy-gas checks in the pinned Idena WASM runtime.
The older `fod_escrow.wasm` and `MANIFEST.json` remain separate. Rebuild the
fast candidate after any source or guest change; an image change also requires
a new sealed receipt and matching contract pin. The synthetic receipt proves
the test vector only. A seller still needs its own contract-bound content
credential and canonical post-session state before a real registration or
funded trade can be qualified.

### Prepare a real content proof before validation

Keep the read-only `session_indexer.py` running beside two synced nodes; its
normal loop prefetches the encrypted blobs (`--prefetch-blobs` is a one-shot
check). On that node host,
`pre_session_snapshot.py snapshot` checks both nodes' current epoch, author
addresses, submitted CID set and retained blob hashes. It waits for the same
inventory to remain unchanged for ten minutes. It then invokes the separate
`fod pre-session-witness` command with only the selected author's node
key and CID-named blobs. The resulting witness stays owner-only; the watcher
never opens a key. A changed inventory gets a new digest and cannot reuse an
older proof.

Run `tools/fod/pre_session_prover.py --config PRIVATE.json` periodically on a
machine with enough memory and disk for proving. The private JSON config names
`remoteHost`, `remoteScript`, `remoteCli`, `remoteArchive`, `remoteOutput`, two
`nodes` in `NAME:PORT:DATADIR` form, `identities` as separate `{name,author}`
pairs, the deployed `contract`, `manifest` (`release/FAST_MANIFEST.json`),
`contractCodeHash`, `contractSha256`, `prepareBinary`, `proveBinary`, `output`,
and `minEpoch`. `holdStatus` may name an
already-running private proof status file so the jobs do not compete. The
runner copies only the selected, hash-checked witness over strict-host-key SSH,
prepares its encrypted copies, preflights the guest and proves the sealed
credential once for that author, epoch, contract and pinned guest image. It
does not sign, register, fund or broadcast anything. If validation begins
while proving, keep the sealed proof for the later session check; the public
inventory and verdict leaves still have to match before `registerFast` can
accept it. A machine that sleeps before the job starts cannot guarantee a proof
will be ready immediately after validation.

`fod prepare-deploy` checks this candidate's module bytes against its manifest
and uses the candidate's measured deploy gas for its fee guard; keep the
generated `release` directory layout intact when preparing the transaction.
Broadcasting a module without a matching measured fast-candidate manifest is
refused; the historical deploy-fee row is only an estimate for its older module.

Both proof guests use SDK 3.0.6 and the digest-pinned RISC Zero Rust 1.91.1
builder in `proofs/guest-build.rs`. Docker must be available. Only Cargo
manifests/locks and Rust compile inputs enter its `/src` context; local vaults,
captures and order witnesses are excluded. The first build may download the
public builder and locked crates. Thereafter those immutable inputs can be
cached or supplied by any mirror. No hosted prover is required.

`node release/check-proof-builds.mjs` builds both guests in two fresh source
and target locations, compares the complete programs and image IDs, then
cryptographically verifies both active receipts. The manifest builder requires
this check before replacing release outputs. Build-skipping, debug and builder
overrides are refused. A compiler/layout change that changes an image requires
new genuine receipts and matching contract/CLI pins together. Existing
ownerless deployments retain their old pins; this build does not upgrade them.
The replacement direct candidate uses
`node release/check-proof-builds.mjs --direct-only` to reproduce and verify its
separate sealed fixture; its builder requires that check before publication.

```bash
node release/build-manifest.mjs      # rebuilds the artefacts and their hashes
shasum -a 256 release/artifacts/fod_escrow.wasm   # compare with MANIFEST.json
node release/check-integration.mjs   # gateway + utility, session to decision
cd tools/fod && ./fod prepare-deploy -code ../../release/artifacts/fod_escrow.wasm \
  -nodekey <nodekey> -nonce <account nonce> -tx-epoch <epoch> -max-fee <base units>
```

`prepare-deploy` builds and signs the deploy of the escrow module, prints the
address the node will derive for it, and stops at the network: the transaction is
written, not broadcast.

## Verify this release

Everything here runs locally and touches no chain. The expected outcomes are the
ones `release/MANIFEST.json` records and the suites print. Run the lines in
order; each starts where the previous one left the shell. The interface checks
need `cd app && npm install` once.

```bash
node release/build-manifest.mjs            # rebuilds both artefacts; 0 forbidden operators, 0 remote tags
shasum -a 256 release/artifacts/fod_escrow.wasm app/dist/flips-on-demand.html   # compare with MANIFEST.json
node release/check-integration.mjs         # 13 checks: the offline interface walk, then session, closed-session result, reveal recompute, grant, tampered result refused, quota once
cd app && npm run verify                   # the same walk on its own, when you want the detail
cd ../tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...
cd ../../contracts/escrow && cargo test --locked --offline
cd ../../contracts/escrow && cargo build --release --target wasm32-unknown-unknown --features test-hooks && \
  wasm-opt --llvm-memory-copy-fill-lowering -O3 target/wasm32-unknown-unknown/release/fod_escrow.wasm -o dist/fod_escrow.testhooks.wasm
cd ../../spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...
cd ../../proofs/registration-v2/host && cargo test --locked --offline --test sealed_artefact
```

The harness suite runs the test-hooked module the recipe above writes
(`dist/fod_escrow.testhooks.wasm`, a name that cannot be mistaken for the
deployment artefact); the release module is built without the hooks, which is
why the two hashes differ.
A checkout that reproduces the same two artefact hashes is the reproducibility
check - `docs/PROGRESS.md` records one such run at this revision, byte for byte.

## Deployment hands over nothing

The module has no owner, so there is nothing to renounce and no key that would
matter: no ownership hand-over, no multisig, and no follow-up transaction.
Deploying is one transaction, after which the deployer can do exactly what any
other address can do. Two checks stand behind that, and neither is an assumption:

- Module: the release artefact exports the published methods and read views and
  nothing else. `spikes/harness/escrow_no_admin_test.go` compares the whole
  export surface by name, so an administrative hook added later under any name
  fails the check.
- Chain: `validateTerminateContractTx` refuses to terminate a contract outside
  the embedded set while consensus upgrade 11 is active, which it has been since
  March 2023 (`docs/SOURCES.md` F-78). Nobody can terminate or replace a deployed
  registry.

`release/MANIFEST.json` records the source revision, each artefact's SHA-256 and
size, the build commands, the delivery proof program's identifiers, and two
checked properties: the release module contains zero gatekeeper-forbidden
operators, and the interface references no remote script or stylesheet.
Regenerate it after any change that affects an artefact: `revision` names the
commit whose sources produced the artefacts, which is the commit before the one
that stores the updated manifest.

`release/check-integration.mjs` starts the gateway and the utility on loopback
ports and walks the loop that ties them together: enroll with a bound wallet,
answer the example pool while it is open, collect the provider's signed result
after the session closes, and have the utility decide. The utility's policy is
built from the key the gateway publishes, so a grant only happens if the
signature survives the utility's own verifier. It also checks that a tampered
result is refused, that the closed session's content reveal recomputes to the
pinned digest, that re-enrolment is a new scope rather than a person, and that a
participant id cannot spend its quota twice.

## Definition of done (`SPEC.md` 11.2)

The living requirement-to-evidence matrix is the "Requirement-to-evidence
matrix" section of `docs/PROGRESS.md`. It carries one row per decentralisation
clause of `SPEC.md` 10.3 and per objective requirement, each with the evidence
that currently proves it and its exact limit. This file keeps no second copy,
because a duplicate table drifts; `docs/REVIEW.md` remains the bounded review of
the older baseline `a91e475` and its A01-A10 findings are tracked in the matrix
and the evidence log.

## What is known to be missing

- **Mainnet costs and wall-clock:** the unfunded deploy cost is measured;
  funded calls and their wall-clock behavior remain unmeasured (`SPEC.md`
  O-9). No further mainnet action is authorized by this document.
- **Operator independence:** the two agreeing lab nodes share one host, so the
  independence evidence is configuration-level, exactly as the matrix says.
- **Canonical scoring and benefit release:** the reference adapter recomputes
  witnessed wallet-bound answers, but answer authority, timing, consumption
  and the actual utility remain operator-controlled (handover H-3).
- **Historical provenance:** captured epoch-228 evidence is recorded in
  `docs/PROGRESS.md`; it does not establish fresh-session answer authority.
- **Disclosure and content availability:** local timing guards cannot hide a
  pending unlock. Retained reveal keys solve settled public-key recovery;
  encrypted-content retention is still enforced by the normal buyer's tools,
  not by the protocol for every custom caller (handover H-7).

## Recovery

| Asset | Where it lives | How it is recovered |
|---|---|---|
| Epoch keys | the CLI's age-encrypted vault, passphrase-held by its owner | the seller exports them; the buyer writes the same vault with `fod receive -pool-order ...`, which opens the recorded private delivery or a settled public reveal; `fod import-keys` prints the public summary |
| Delivery key | a 32-byte hex file the buyer keeps, and optionally an age-encrypted archive of it (`prepare-buy -delivery-key-archive`) | both forms open the recorded pool delivery (`fod receive -pool-order ...`); its loss is not a seller-failure condition (`SPEC.md` 6.6) |
| Escrowed payment | the contract | at `funded+3`, anyone may submit one `refund`; it pays the original buyer the purchase amount and the retrieval fee directly, with no credit step. A host's retrieval-fee credit is withdrawn with `withdraw` |
| Software | this repository | rebuild the exact source snapshot; the dirty candidate is not a HEAD-only clone, and `MANIFEST.json` holds the commands |
| Evidence | `docs/PROGRESS.md`, `docs/SOURCES.md`, `release/MANIFEST.json` | each claim carries the measurement or source behind it |

## Mainnet exercise, funded steps not yet run

These are historical estimates for earlier artifacts, except for the separately
approved unfunded deployment recorded in `docs/PROGRESS.md`. Each remaining
step needs approval before it happens. What the
approval is for, in money terms at the rate the chain charged on 2026-09-25
(`fod fees -fee-per-gas 96153846153800` prices each call, gas plus size fee):
the deploy is the largest single fee at about 354 iDNA, because its signed
transaction is 365,269 bytes and the chain charges ten gas per byte; the calls
that carry a proof are about 189 iDNA (`registerBundle`) and about 190 iDNA (the
`fulfill` fallback), while the cheap non-cooperative `reveal` is about 1.45 and
funding, donating, buying, delivering, confirming, refunding and withdrawing are
about 0.5-1.6 iDNA each. `fod submit` and `prepare-deploy` both refuse to
broadcast a transaction whose max fee cannot give it the gas it needs, and print
the value to sign again with, because a transaction that runs out of gas is
still charged. The price a buyer pays is not a fee: it is escrow. On settlement
the purchase amount goes to the bundle's registered author and the retrieval fee
to the hosts the buyer named; on a timeout the same total returns to the buyer,
minus only the network fees. The reserve is never spent on either side.

The fees are paid from an account's balance, not from its stake: the 2026-09-26
reading of the then-designated exercise identity was 29.78 iDNA of balance
against 86,233 iDNA of stake, so a deploy account has to be funded, or stake
unlocked, before step 1 can succeed, and the escrowed price has to be present in
the funding account while an order is open (one account may play both sides: the
contract binds a delivery key and a buyer address, not a second identity). The
deploy needs no stake of its own: the chain's 3,000,000-gas minimum applies only
to its embedded contracts, and a custom module is not one (`SOURCES.md` F-92).

0. Capture the epoch's lottery records while the node is live. The database
   cannot be read in place - a running node keeps its lock, and the open fails
   with `resource temporarily unavailable` - so copy it and read the copy:

   A shared-node operator can use the optional read-only watcher for the
   time-sensitive lottery record and adjacent session anchors. This is an
   operator task, not a seller requirement:

   ```bash
   python3 tools/fod/session_indexer.py --cli /absolute/path/to/fod \
     --archive /private/absolute/archive \
     --node seller-node:PORT:/absolute/node/datadir
   ```

   It uses loopback RPC, keeps the RPC key out of logs, and discards full
   database snapshots after successful compact extraction. It also retains
   CID-checked encrypted flip bytes for each watched author under
   `epoch-N/encrypted-blobs/<node>/<CID>` and records prefetch and final-manifest
   counts in the private `encrypted-blob-status.json`. `--prefetch-blobs` checks
   and saves the current inventory immediately; `--once` checks configuration
   without running the watcher. The watcher does not access identity keys or
   perform the committed-body sweep below. Multiple independently run watchers
   may publish the same portable anchors; a buyer still checks roots against
   an independently authenticated chain view. The lottery summary is local
   node data, not a cryptographic proof. An API-only buyer route remains an
   explicit unverified opt-in, not the default purchase route.

   The test-host service confines writes to its mode-0700 archive, denies the
   node keystores, allows loopback networking only, and grants only the read
   capability needed for the two owner-only node databases. A different host
   needs its own node paths and access policy; do not copy the test host's RPC
   key or identity key into a connection file.

   ```bash
   # on the machine that runs the node, while the epoch is still open
   cp -R <datadir>/idenachain.db <capture>/idenachain.db
   # here, against the copy
   cd tools/fod
   ./fod epoch-db -datadir <capture> -inventory-epoch <epoch> -out captured.json
   ./fod assignment -datadir <capture> -inventory-epoch <epoch> \
     -out assignment.json -indexer https://api.idena.io/api
   ```

   Measured on the live mainnet node on 2026-09-26 (`docs/PROGRESS.md`): the copy
   is 3.2 GB in 1767 files and takes about 12 seconds on the node's own disk, and
   reading the copy takes 0.2 s for one epoch's records and 6.2 s for a full scan
   of that epoch's keys. Rehearse nothing else here - the reading is what has to
   happen inside the window, and it is cheap.

   Copy it for real: a naive hardlinked copy (`cp -al`) is not usable, because
   LevelDB appends to its MANIFEST in place and the copy ends up naming tables it
   does not have. The watcher hardlinks immutable table files but copies mutable
   metadata. Before going further, `fod epoch-keys -datadir <capture>` says
   whether the capture actually holds the epoch's `li`/`ls` records; if it does not, the node
   was not live at the lottery block and the copy is worthless for this purpose.
   A copy that fails to open or read is not a verdict on the node: LevelDB is
   being written while the copy runs, so a torn copy is retried from the same
   node. The copy takes seconds and the store is not cleared until the node
   crosses into the next epoch, so a retry is cheap while the node is still in
   that epoch; only a readable copy with `seed_bytes=0` and no candidates means
   the node missed the lottery block.
   The shape to look for is the one a lab chain produced when its own lottery
   ran: `epoch <n>: map[li:1 ls:1]` in the key census, and then
   `epoch-db ... : seed_bytes=32 candidates=<one per lottery identity>`; a copy
   taken before the block answers `seed_bytes=0 candidates=0` instead, which is
   the signature `docs/PROGRESS.md` recorded for a too-early capture.
   The copy has to be taken *inside that epoch*: a node that is live at the
   `FlipLotteryStarted` block writes the seed and the candidate list then, and it
   clears its epoch store when it crosses into the next epoch
   (`core/ceremony/ceremony.go`, `completeEpoch` calls `EpochDb.Clear`). Days
   after the block are fine, the next epoch is not. Datadirs that stopped syncing
   before the boundary still carry their epoch's keys months later; a live node
   does not. The historical epoch-228 capture and the failed epoch-229 attempt
   are recorded in `docs/PROGRESS.md`; the optional watcher is now armed for
   epoch 230. None of those records qualifies a funded mainnet purchase.

   The answers are not part of that race: the epoch's own transactions carry them
   in the clear, so a sweep of the epoch's blocks (`fod sync -from <epoch start>
   -to <epoch end> -epoch <epoch> -body-source ipfs -out epoch-sweep.json`)
   supplies them whenever the capture was taken too early to hold them, and
   `fod eligibility -sweep`/`fod prepare-offer -sweep` take both sources when
   both exist and refuse a disagreement.

   `dna_epoch`'s `startBlock` is the epoch's first block and the next epoch's
   `startBlock` minus one is its last. Measured on the capture host on
   2026-09-26: 2,000 of epoch 228's blocks swept in 2.7 s with 102 block bodies
   CID-verified and 0 mismatches, so the epoch's ~22,000 blocks are about half a
   minute of work.

   The copy is the capture: the assignment and the ceremony's candidate order
   exist only in a node that was live at that epoch's `FlipLotteryStarted` block.
   The indexer argument is optional and only cross-checks what it can; it refuses
   an assignment whose answer counts it cannot confirm, rather than vouching for
   one (`SOURCES.md` F-79, F-81). It also compares the explorer's epoch state root
   with the header of the node named by `-rpc` when that node answers, and records
   whether it did: `nodeVerified: true` with the height, or the reason it was not
   checked. That height is the epoch's *opening* block, which the explorer's
   `Epoch/{n-1}` block list names, not `validationFirstBlockHeight`
   (`SOURCES.md` F-82); `-epoch-block N` takes a height the caller knows instead.
   `-require-node` refuses instead of recording.
1. Deploy the release module; record revision, bytecode hash, address, and the
   runtime revision it ran on. On the host that holds the seller's nodekey, which
   is also the deployer:

   ```bash
   ./fod fees -operation deployModule -rpc <node> -key <api key>   # the max fee to sign with
   ./fod prepare-deploy -code release/artifacts/fod_escrow.wasm -nodekey <nodekey> \
     -nonce <account nonce + 1> -tx-epoch <chain epoch> -max-fee <the deploy figure> \
     -network idena-mainnet -signed deploy.json -broadcast
   ```

   The figure is the one the table prints; the broadcast reads the chain's own
   rate, refuses a nonce or epoch the account cannot use, and refuses a max fee
   that cannot run the deploy rather than sending it. The address it prints is
   the `<registry>` every later step names.
2. Register a real eligible bundle into the pool. `fod prepare-offer` takes the
   seller from the capture to the registration input in one command and can run
   the prover on it:

   ```bash
   ./fod eligibility -datadir <capture> -inventory-epoch <epoch> -author <seller> \
     -out eligibility.json -indexer https://api.idena.io/api -require-node \
     -sweep <epoch-sweep.json>
   ./fod prepare-offer -datadir <capture> -inventory-epoch <epoch> -author <seller> \
     -nodekey <nodekey> -bundle <inventory.json> -blob-dir <blobs> \
     -contract <registry> -network idena-mainnet \
     -indexer https://api.idena.io/api -prover <registration-host> \
     -sweep <epoch-sweep.json>
   ./fod prepare-registration -proof <registration.json> -contract <registry> \
     -prepared registration-action.json
   ./fod submit -prepared registration-action.json -nodekey <nodekey> \
     -nonce <next account nonce> -tx-epoch <chain epoch> \
     -max-fee <the registerBundle figure> -broadcast
   ```

   The eligibility report's `indexer` block records what the explorer supplied and
   whether a node confirmed the epoch's state root, and its `epochEnded` block
   records that the node had left the epoch. An epoch the node has not finished,
   and a bundle whose covered flips the explorer lists differently or publishes no
   result for, are refused rather than registered (blocked until step 0 has been
   run against a closed epoch).
3. Fund the pool's reserve. The initial bundle set and the iDNA amount are the
   owner's funding profile (`SPEC.md` 7.5); the numbers are chosen before the
   profile is funded, and every rehearsal uses play money first. `addReserve`
   is permissionless: anyone may contribute to the global reserve and retains a bounded withdrawal
   right to their contribution. Pending buyer escrow and withdrawal credits
   are separate liabilities; the ratio
   between the reserve and the units is what sets the quote.

   ```bash
   ./fod prepare-fund -contract <registry> -pool-bundle <commitment> \
     -amount <iDNA base units> -network idena-mainnet -prepared fund.json
   ./fod submit -prepared fund.json -nodekey <funder nodekey> \
     -nonce <next account nonce> -tx-epoch <chain epoch> \
     -max-fee <the addReserve figure> -broadcast
   ```

4. Give one unit, and keep the delivery desk running. The interface's
   "Give one unit" button writes a `donateUnit` request; `fod prepare-list`
   reads it. Only the bundle's registered author may donate, and the gift
   carries only the commitment. The contract's `sellUnit` entry point stays
   refused under the no-subsidy rule, so this release has no sale-receipt or
   payout step. The desk publishes the buyer-bound ciphertext and keeps the
   beacon fresh while it runs; with `-prover` it can fall back to the
   `fulfill` proof near the deadline.

   ```bash
   # a gift needs no receipt
   ./fod prepare-list -request <donateUnit request> -prepared give.json
   ./fod submit -prepared give.json -nodekey <donor nodekey> \
     -nonce <next account nonce> -tx-epoch <chain epoch> \
     -max-fee <the donateUnit figure> -broadcast
   # the desk: keep serving while you are willing to deliver
   ./fod serve -contract <registry> -vault <epoch vault> \
     -nodekey <desk nodekey> -broadcast -interval 20
   ```

5. Buy a unit from the pool. The interface's buy request names the bundle, the
   expected quote, and the buyer; `fod prepare-buy -request` rechecks the buyer
   against the nodekey, reads the pool's current nonce and quote, and refuses a
   quote that moved. It creates the buyer's dedicated delivery key when the
   file does not exist yet; `-delivery-key-archive <file>` also writes the
   age-encrypted recovery copy. `-availability <advisory>` is required, and
   `fod inventory ... -availability-out` writes one: prepare-buy fetches every
   CID the seller's advisory names from the buyer's own node, rebuilds each CID
   from the bytes, and refuses to build a call whose content it cannot fetch.
   The check it made is recorded in the prepared file with its time, and
   `fod submit` refuses to sign a buy file whose check is missing, incomplete
   or older than fifteen minutes. Both commands also require a contract address
   and Keccak-256 code hash from an independently checked deployment record.
   Do not copy these pins from the imported marketplace snapshot or buy request;
   `submit` checks the node's deployed code hash again before signing and sending.

   ```bash
   ./fod prepare-buy -request <buy request> -contract <approved registry> \
     -code-hash <approved Keccak-256 code hash> -nodekey <buyer nodekey> \
     -delivery-key <buyer delivery key> -availability <advisory> \
     -prepared buy.json
   ./fod submit -prepared buy.json -contract <approved registry> \
     -code-hash <approved Keccak-256 code hash> -nodekey <buyer nodekey> \
     -nonce <next account nonce> -tx-epoch <chain epoch> \
     -max-fee <the buy figure> -broadcast
   ```

   The call holds the purchase payment and retrieval fee as pending escrow and opens the
   order; the desk has until the delivery deadline to publish the keys.

6. Receive and confirm the delivery. The desk's ciphertext is recorded in the
   pool's own storage, so the buyer needs only the delivery key they kept:

   ```bash
   ./fod receive -pool-order <order id> -contract <registry> \
     -delivery-key <buyer delivery key> -vault <buyer vault> \
     -prepared buy.json -out opened.json                     # checks it, writes the keys, records the digest it checked
   ./fod receive -vault <buyer vault> -blob <flip blob>
   # from the record: the confirmation names the ciphertext that was checked
   ./fod prepare-confirm -contract <registry> -pool-order <order id> \
     -delivery opened.json -prepared confirm.json
   # or from an interface request, which carries the same digest
   ./fod prepare-confirm -request <confirm request> -delivery opened.json -prepared confirm.json
   ./fod submit -prepared confirm.json -nodekey <buyer nodekey> \
     -nonce <next account nonce> -tx-epoch <chain epoch> \
     -max-fee <the confirm figure> -broadcast
   ```

   The receive path refuses a delivery that another key owns, one that does not
   decrypt, and one whose secrets do not hash to the commitment the pool order
   carries; the confirmation names the digest of the bytes that passed those
   checks and signs a usable-content receipt. Confirmation includes the checked bytes,
   so an outsider replacing the pending delivery cannot invalidate the buyer's
   retained copy. Signing rechecks the open order and receipt. If the
   buyer never confirms, the desk's `serve -prover` fallback publishes the
   delivery proof and settles with `fulfill`.

7. Leave an order unfulfilled; at `funded+3` anyone may submit a refund.
   The interface writes the `refund` request; it pays the original buyer directly
   (purchase payment plus retrieval fee) and counts a strike, and the second
   strike clears the listing.

   ```bash
   ./fod refund -request <refund request> -prepared refund.json
   ./fod submit -prepared refund.json -nodekey <buyer nodekey> \
     -nonce <next account nonce> -tx-epoch <chain epoch> \
     -max-fee <the refund figure> -broadcast
   ```

8. Run a session for customers with the flips the exercise bought. The buyer's
   export carries the flips and the answers it can evidence; `fod session-pool`
   turns that export into the pool file the provider's gateway loads, so
   participants see the two sequences of each flip and choose between them.

   ```bash
   ./fod export -inventory inventory.json -vault vault.age \
     -blob-dir blobs -answers answers.json -answers-source <where they came from> \
     -out export/
   ./fod session-pool -export-dir export/ -out pool.json \
     -session-id <yours> -utility-id <the utility this session grants> \
     -threshold <answers needed to pass> -closes-at <RFC 3339>
   cd gateway && node src/main.ts <your-config.json>
   ```

   Point the gateway's `poolPath` at that pool file. The gateway refuses content
   that is not a flip, so a session can never show a participant something they
   cannot look at; the pool stays mode 0600 because it holds the answer digests.

9. Record every transaction outcome here and in `docs/PROGRESS.md`. `fod submit`
   writes what the node reported into the signed artefact, and says so plainly
   when there is no receipt yet rather than implying success.

## Exact local source snapshots

A dirty revision alone does not identify the release. `MANIFEST.json.source`
records every non-ignored input's path, size, SHA-256 and executable flag, and
one digest over that inventory. Generated HTML/WASM and the manifest itself
are recorded separately. Ignored runtime keys/data and source symlinks are
excluded or refused. Never publish a snapshot without a separate review.

After building the candidate, verify/export to a new private directory:

```sh
node release/source-snapshot.mjs
node release/source-snapshot.mjs /path/to/new-private-snapshot
```

The exporter refuses changed inputs and an existing destination. In that
snapshot, rebuild the HTML with `cd app && npm ci --offline && npm run build`;
rebuild WASM with the manifest's escrow command using `--locked --offline`.
Compare output hashes to the copied manifest. These checks prove the local
source-to-artifact relationship, not independent-operator availability or new
chain receipts. Cargo/npm caches and the declared toolchain/dependency pins
remain environment prerequisites. The CLI's external node source uses the
immutable dependency setup in the root README, not private machine paths.

The release builder rebuilds the HTML into staging, verifies the delivery-v2
seal against the current guest, and checks the WASM export/operator surface
before promotion. A failed candidate leaves the previous outputs in place.
The new tuple is locally verified; it has not upgraded any deployed module.
