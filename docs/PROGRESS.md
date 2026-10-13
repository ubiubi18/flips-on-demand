# Progress and Evidence Record

Compact record of what is actually done, what was measured, and what is blocked.
`SPEC.md` holds the requirements; this file holds the state.

Rule: an entry exists here only when there is an artifact or a command output to
point at. Intentions, plans, and generated-looking output are not evidence.

## 0. Active goal

Experimental fast-trade redesign, 2026-10-10: `SPEC.md` 7.7 now has an
implemented pre-session content statement, authenticated copy format, public
four-leaf session verifier, and buyer-bound point delivery. The content core
checks original CIDs, decrypts every blob, checks supported flip structure,
checks each embedded identity key against the claimed author, rebuilds every
copy, and commits the original keys, sorted original and copy
inventories, count and hiding point. It rejects duplicate protobuf fields
because the original Rust blob reader selects the first occurrence while the
node selects the last. The guest built from a source-only context using the
pinned builder, and the proving host compiled with locked dependencies. A
synthetic one-flip guest preflight matched the prepared journal; a changed
copy was refused. Two clean source builds produced the same guest image and
ELF hash. A genuine synthetic one-flip Groth16 seal was generated, checked by
the host, and accepted by the pinned Idena WASM verifier at 195,558,635 WASM
gas; changed journal and seal bytes were refused. A local preparation command accepts an
explicit witness and candidate contract, writes authenticated copy bytes and
seller key material into a new private directory, and marks the result
`prepared-unsealed`. Its output is not a credential or a listing.
The preparation and proving commands now refuse original witnesses and seller
key files with group or world access, and the proving host requires owner-only
prepared directories.

The core buyer prefetch check rejects a wrong contract, bundle id or changed
copy bytes. Its synthetic recovery vector verifies the delivery proof and opens
the exact original flip parts. The buyer CLI now reads the fast bundle and full
registration certificate from canonical contract state, compares the published
original/copy CID pairs with those records, fetches and retains every original
and copy before preparing a payment, and checks the retained bytes again before
signing. Its fast receive path exports the included order and delivery proof
for an owner-only local recovery tool. A separate offline verifier replays the
content seal and four session leaves against roots supplied from the verifier's
own synced node. The ordinary buyer path still refuses fast bundles.
The host requires explicit original witness, prepared directory, contract and
new output file; it can preflight without proving and only produces a sealed
artifact with `--prove`. The public synthetic artifact is retained under
`proofs/fast-content/dist/`; its placeholder contract and flip vector cannot
register a live author.

`contracts/escrow` now has a `fast-candidate` feature: it removes the old
registration and delivery/settlement exports, checks the public session
against canonical headers, enforces one fast registration per author and epoch,
and settles a funded order atomically with its 196-byte point proof. The
candidate's fast registration pins the reproduced guest image and verifies its
real Groth16 seal. The dedicated test-hooks WASM
accepted a synthetic funded order, retained the full included proof, and paid
the author at 21,739,039 WASM gas (217,390 contract gas after the node's
100:1 conversion, excluding transaction overhead). A changed proof, replay,
and late delivery failed; refund remained available at block N+3. The isolated
four-leaf fixture cost 315,716 WASM gas; the point verifier cost 21,268,853
WASM gas. These are local measurements, not a deployed or real-proof
registration test. The full bounded registration certificate is now retained
in contract state; the pinned runtime stored both a 5 KiB value and the
66,066-byte maximum without truncation. The seller CLI exports canonical
funded-order context and builds a small buyer-bound point proof from its private
pre-session witness; signing rechecks the funded order, author and deadline.
The owner accepted public mempool censorship risk only for an experimental
fast path.
A separate release-mode candidate build exposed neither test hooks nor legacy
settlement exports; it was inspected locally and not deployed.
There is no qualification for paid mainnet use, live-author fast credential,
or measured combined real-session registration gas yet. The fast candidate has
not been deployed, so no live fast registrations or purchases exist.

Owner-authorized epoch-230 test preparation, 2026-10-10: the live watcher
captured root-checked inventory and verdict anchors for `idenaai2` and
`idenaai3`, plus all eight manifest blobs (5/5 and 3/3) with local CID checks.
Their private witnesses are retained in separate mode-0700 identity directories
on the key-holding host. Regenerating each witness with the installed helper
reproduced its retained hash, and crossing the witnesses was refused in both
directions. The watcher service is active with zero restarts; its deployed
script matches the repaired source, including the cross-filesystem snapshot
fallback, and its helper carries the corrected key derivation and proof pin.
The next validation reported by both synced nodes is 2026-10-14 17:00
Europe/Berlin. A separately authorized 50 iDNA transfer from `idenaai3` funded
`idenaai2`; the receipt remains in ignored private evidence.

The live witnesses exposed two compatibility defects that synthetic fixtures
had hidden: epoch keys require the original Go 1.17 40-byte seed mapping, and
the node encrypts the renderer's raw nested RLP rather than ASCII hex around a
flat list. Both are repaired and the corrected guest executed all five and all
three real flips. Its image differs from the immutable image in the deployed
escrow, so that deployment cannot register these captures. A matching ownerless
contract candidate and genuine 256-byte Groth16 fixture seal are built and
verified locally; no replacement deployment or registration transaction has
been signed or broadcast.

Optional node-integrated evidence-cache candidate, 2026-10-06: an isolated,
uncommitted idena-go worktree now captures
root-verifiable public identity ranges at lottery and validation boundaries,
exports per-author anchors, and retains one completed epoch. Synthetic
restart, pruning, root-mismatch, symlink and local-key-exclusion checks pass;
the existing `fod session-bundle` consumer accepted generated anchors. This
candidate has not been installed on IdenaAI 2/3 or measured over a real
ceremony. It does not retain encrypted IPFS content, establish live seller
eligibility, or qualify funded use. The short delivery-or-refund goal remains
blocked on its existing conditions.

Owner-authorized mainnet test, 2026-10-03: the current ownerless escrow WASM was
deployed once from the designated test identity with no deposit. A read-only
accounting view executed. The transaction locator and sender are kept in ignored
local evidence, not tracked documentation. This confirms deployment and ABI
execution only; the short delivery-or-refund and fair-exchange requirements
below remain open. The contract is permanent and must not be funded or offered
as a qualified marketplace on this evidence.

Active implementation follow-up, eighteenth revision, 2026-10-01. Retain the
short delivery-or-refund objective: for a purchase included at block N, usable,
authenticated content and safe settlement must complete before N+3, or one
permissionless transaction at N+3 must return the purchase payment and retrieval
fee to the original buyer. No hour-long window or extra grace period is accepted
as a remedy. Chain stalls do not imply a fixed wall-clock promise. Pursue a
faster settlement design that preserves both sides of fair exchange, canonical
availability and operator-free recovery; qualify it with adversarial boundary
tests, measured post-inclusion latency and transaction costs on isolated
play-money infrastructure before offering it as the ordinary purchase path.
The current order-specific delivery-v2 proof took 946.459854 seconds after its
funding anchor became available and remains unqualified for this deadline.
Its reproducible image and valid receipt establish proof compatibility, not
timely delivery or safe disclosure. Keep the unfinished MVP blocked until the
full acceptance criteria are demonstrated; preserve all historical evidence.

Local security follow-up, 2026-10-03: the owner explicitly resumed a bounded
audit/repair window (maximum six hours). Revalidate the dirty worktree, repair
observable local vulnerabilities in coherent packages, and record SA-01..SA-16
with adversarial regressions. The objective and deadline above remain in force;
local repair closure does not complete S8/S11 or the MVP. No authorization for
existing credentials, remote hosts or chain actions is inferred.

Owner follow-up, 2026-10-03: "dont ask, decide and do it" authorizes rotating
the exposed local gateway application key and updating its local public pins.
The replacement was published and reloaded; old-key browser verification and
utility authorization refuse. This narrowly supersedes the credential-rotation
blocker, without authorizing wallet keys, remote hosts or chain actions.

Owner-authorized passive capture follow-up, 2026-10-03: arm IdenaAI 2 and
IdenaAI 3 for epoch-229 session bundles and retain their encrypted flip bytes
privately. This permits read-only access to those two existing nodes for this
capture only. It does not authorize identity-key access, transactions,
deployment, funding, or treating the mainnet escrow as qualified. The
2026-10-04 boundary attempt failed: the capture selected the block before
epoch 230's `startBlock` as its verdict anchor, but that opening block is the
required post-validation anchor. Both live nodes have since pruned the
necessary state versions. Eight encrypted blobs were retained and locally
CID-checked, but no epoch-229 session bundle exists. The failed service is
disabled; its script was corrected and checked against the prior real bundle,
but was not rearmed. The evidence and recovery limit are recorded below.

Owner clarification, 2026-10-04: historical capture should be a replaceable
shared-node service, not a technical task imposed on every seller. A buyer may
also opt into `api.idena.io` as an explicitly unverified chain-data source and
accept its service risk, but that choice cannot bypass local CID checks, the
contract's authorization, signed price ceiling or refund right. Waiting for
the next ceremony is a no-payment preparation route, not a longer funded
settlement window. The optional indexer is now armed for epoch 230 as recorded
below; API-only funded buying and independently verified lightweight buying
are still unimplemented, and the three-block fair-settlement goal is open.

Local S11 containment, 2026-10-01: a runtime regression exposed two funded
orders for the same shared-key bundle, where revealing for one paid only that
order and left the other buyer able to refund after learning the keys. The
current candidate closes all open same-bundle orders atomically, caps that set
at eight, refuses public reveal after any deadline or prior refund on the
bundle, and rechecks the set before local signing. The desk's plan now shares
one disclosure bound with that local signing guard: it delivers only while a
later pass can still include the committed secrets before the refund boundary,
settles a funded order with the reveal at that last head, and leaves later
orders to the buyer's repayment. These are local containment results, not a
faster qualified delivery design or a new chain receipt.

Active implementation follow-up, seventeenth revision, 2026-10-01. The owner
requested repair of H-10: a newly generated proof for a relocated image cannot
validate the old contract pin. Pursue S10 in SPEC 10.6: normalize and pin the
guest build, compare two clean source locations, then update and verify the
complete registration/delivery/contract/CLI tuple together if images change.
Preserve the blocked unfinished MVP and historical artifacts/evidence. No
deployment, transaction, remote access or real keys/funds are authorized.
Acceptance requires reproducible images and genuine matching receipts; it
does not establish fast private delivery, operator availability or fresh chain
execution.

S10 is now locally verified by the dated evidence below: both clean guest
programs and genuine receipts agree with the updated contract/CLI pins. The
unfinished MVP remains blocked on the other stated guarantees; existing
deployments were not changed.

Active verification follow-up, sixteenth revision, 2026-10-01. The owner
requested proof of independent-operator availability, authenticated lightweight
purchasing, full CLI/proof-production environment reproduction and fresh chain
receipts. Verify the current candidate against each criterion, without
substituting fixtures or historical receipts. Clean CLI builds and focused
tests now pass with the immutable node source export. Relocated clean builds
of both proof guests fail the shipped receipts; this is the new H-10 release
blocker. Independent operators, a light-client purchase implementation and an
authorized current-module chain exercise remain unavailable. Preserve the
unfinished blocked goal and all previous evidence; no production repair,
deployment, broadcast or remote access was performed by this verification.

Active follow-up, fifteenth revision, 2026-10-01. The owner requested repair
of safe disclosure timing and canonical availability. Pursue S9 in SPEC 10.6:
canonical recovery of settled public keys and local signing/broadcast timing
containment, with focused adversarial evidence. Preserve the three-block
boundary and the unfinished MVP goal. Full pre-inclusion fair exchange,
canonical encrypted-content availability, fast private delivery and independent
lightweight authentication remain technical blockers. The goal API still
reports blocked; do not replace or complete it from these local repairs.

Active follow-up, fourteenth revision, 2026-10-01. The owner requested repair
of the remaining limitations. Pursue S5-S8 in SPEC 10.6: one refund-only
boundary at funded+3; delivery-v2 network/funding-anchor binding with a genuine
local receipt and coordinated consumers; exact dirty-source release provenance
and clean rebuild comparison; evidence-backed fair-exchange/utility feasibility.
Preserve all earlier work and measurements. No deployments, broadcasts,
remote hosts, real keys or funds are authorized. The goal API remains blocked;
this local continuation does not replace or complete the unfinished MVP.

Active security work, thirteenth revision, 2026-09-30. The owner resumed local
implementation against the completed scan `edd7e9ae-97d8-4ea5-8e09-4ab85dd1ca02`
(seven findings: three high, four medium). Pursue the existing MVP objective
by closing the four repair packages in `SPEC.md` 10.6: purchase integrity,
utility authorization, recovery/file safety, and browser/network boundaries.
Completion of this repair slice requires an adversarial regression for each
finding, passing affected suites, and accurate limits for the remaining chain,
utility, operator-availability and release-provenance guarantees. The current
model and reasoning setting are the owner's selection; no selector change is
claimed. The desktop goal still reports blocked and its API cannot edit or
resume it; this request authorizes useful local work without replacing the
unfinished MVP or falsely completing it. Existing historical measurements
below remain evidence for their recorded source, not proof for new changes.

Objective, revised on 2026-09-29 (third revision), extending the second
revision to the three-sided on-demand model in `SPEC.md` section 10.4:

> Deliver a verifiable on-demand flip marketplace on an isolated play-money
> chain: Idena flip authors can sell through a self-hosted or provider-hosted
> shared node without operating the Idena stack themselves; a provider can buy
> the flips and host instant on-demand sessions; an EVM-compatible customer can
> log in with a wallet address, buy or request the flips, solve them
> simultaneously with others, and receive a utility grant (testnet faucet, dapp
> access). The flips' provenance comes from real Idena epochs; the on-demand
> session adds the fresh participation. Ship a buyer-testable testbed with
> example flips and a test utility. Keep the trust boundary explicit: the Idena
> consensus proves the flip set, not the provider's scoring; an EVM address is
> the same secp256k1/keccak address as an Idena address, and the customer's lack
> of an Idena identity is a personhood limit, not a key limit; cross-chain
> payment and custody stay a separate decision. Keep paid supply disabled until
> its economic, authorization, proof-completeness and accounting prerequisites
> are satisfied.

The third revision keeps the second-revision purchase flow and its economic
policy as the first acceptance slice. `SPEC.md` 10.4 is the new package G plan;
`SPEC.md` 10.3 (packages A-F) and section 3 remain the store of record for what
that first slice changed.

Fourth revision, 2026-09-30. The read-only audit of the third revision
(section 6, audit 3) found sixteen open hardening items in the session gate,
the utility claim path, the reference service defaults, the settlement state
and the verification environment. They are the active extension of this goal
and are specified as Package H in `SPEC.md` 10.5, with their rows in the
matrix below. The four genuine external blockers in the final acceptance
report are unchanged, and H-3 (participant-verifiable scoring) stays the
handover note for the part no local patch can settle.

Fifth revision, 2026-09-30. The Codex Security standard scan of this working
tree (scan `aab316d1-e176-47df-a708-5a3b9b3e6c90`, report under
`<internal-state-path>`) completed with
11 findings (4 high, 6 medium, 1 low, all confidence high). Ten map onto
Package H rows: AH-13, AH-11, AH-1/AH-2, AH-3, AH-8, AH-12, AH-10, AH-9,
AH-6/AH-7 and AH-14, so the scan independently confirms those items. One is
new: the ordinary `prepare-buy` artefact can be signed while the
content-availability check is left unattested (`tools/fod/pool_buy.go:286-337`),
so a seller can settle a purchase whose bytes the buyer cannot obtain. It is
folded in as AH-17 in `SPEC.md` 10.5 with its matrix row below. Closing
AH-1..AH-17 is the active local work; the audit cadence and the four external
blockers in the final acceptance report are unchanged.

Sixth revision, 2026-09-30. The final acceptance exercise for this revision
ran on a fresh v4 lab deployment (escrow `0x8a225b8e…`, network 33411, the two
reference nodes). It found one closed item incomplete and one new defect, so
the active work is no longer "close AH-1..AH-17". The ordinary buy check the
AH-17 row claims committed the CID set over the hex of the *decoded* bytes,
while the registration guest hashes the canonical multibase text, so every
honest `prepare-buy` against a real registered bundle was refused; it is fixed
and pinned by a live-chain vector
(`TestTheInventoryCommitmentCommitsTheCanonicalCIDText`), and the same lab's
`prepare-buy` then fetched and rebuilt the real bundle's content. The new
defect is AH-18: on this node the buyer's `confirm` transaction is accepted
into the mempool and never included - the proposer silently drops a
transaction whose execution fails (`filterTxs` -> `applyTxOnState`) while buy,
deliver, receive and the permissionless refund all execute on the same
contract - so the buyer-confirmation leg of the acceptance exercise stalled
with the exact evidence in section 3. It was resolved the same day: the call
was not refused by the contract but
underpriced - the fee table carried the pre-`AH-10` digest-form figure while
the confirmation that carries the buyer's ciphertext needs 20,357 user gas -
so the row now carries the live measurement, the submit guard refuses a short
budget before broadcasting, and order 2 settled from the buyer's own checked
bytes (tx `0xdd130fd1…`); see the resolution entry in section 3. AH-1..AH-18
are closed for this revision.

Seventh revision, 2026-09-30. The second Codex Security standard scan of this
working tree (scan `e87b560e-75bd-43ac-9f26-22043435ff6a`, completed
2026-09-30T11:05Z, snapshot digest
`codex-security-snapshot/v1:sha256:073f89a9...`) confirmed that the items the
first scan demanded are repaired, and reported three findings of its own: one
medium (a delivered order whose buyer never confirms can still be refunded in
full because the reference desk plans no in-window settlement without the
buyer) and two low (three writers of decrypted flip material and session
answers do not re-tighten a pre-existing wider file; the tree carries live
Idena identities in documents and fixtures and an unignored local market
snapshot). Audit 5 (section 6) folds them in as AH-19..AH-21 in `SPEC.md` 10.5
with their matrix rows, and adds AH-22 for the release check's truncated
walk-failure evidence and AH-23 for the test-hooks artefact that shares the
`dist` deployment path. Closing AH-19..AH-23 is the active local work; the four
external blockers and the H-3/H-5 handovers are unchanged.

Eighth revision, 2026-09-30. AH-19..AH-23 are closed by the same-day fixes
recorded in section 3: the desk settles an unconfirmed delivered order with the
committed secrets inside the window, the secret-bearing writers re-tighten a
pre-existing file, the tree's full identity addresses are redacted (with the
pinned IAVL proof fixture as the one stated exception) and the CLI outputs are
ignored, a failing release walk keeps its evidence, and the test-hooks artefact
no longer shares the release path. The package plan's local work is empty
again; the four external blockers and the H-3/H-5 handovers are unchanged.

Ninth revision, 2026-09-30. The objective's "without trusted scoring" half got
its first implemented layer (Package I, section 3). The signed session result
now carries the answers the provider scored, and the participant's client
recomputes the score from its own submitted answers and the answer key the
closed policy reveals, after checking that reveal against the digest it pinned
before answering; a result that does not reproduce is shown as not verifiable
and cannot claim the utility grant. The check protects the participant from the
provider. The utility still decides on the provider's signed result, and a
provider cooperating with a participant can attest answers neither had to
solve, so H-3 stays the handover for the enforcement half; the four external
blockers are unchanged.

Tenth revision, 2026-09-30. The enforcement half of "without trusted scoring"
landed (Package J, section 3): the reference utility no longer takes the
provider's arithmetic on its word. A policy that sets `requireScoreProof` grants
only on a score the utility recomputed itself from the provider's signed
transcript and the answer key the pinned content carries, and the release
integration proves it with a result the gateway's own key signed whose numbers
do not add up - the utility refuses it with `score_mismatch`. What remains is
the residue the transcript cannot settle: it is still the provider's attestation
of what a participant submitted, so H-3 stays the handover. The four external
blockers are unchanged.

Eleventh revision, 2026-09-30. The hosted shared node's onboarding landed
(Package K, section 3): `fod shared-node` performs the node's export and builds
the session pool from the author's epoch vault, then writes the
`fod.shared-node` connection file that names the node, the author the escrow
pays, the pool the gateway loads and the custody boundary. The Use flips view
names that command instead of four hand-run steps, and the walk pins the
journey text. The listing/donation stays the author's own prepared action and
the delivery desk is the separate `fod serve`, so G3 stays "mostly met" rather
than one command doing everything; the four external blockers and the
H-3/H-5 handovers are unchanged.

Twelfth revision, 2026-09-30. The objective's "without trusted scoring" half
is now closed for the reference flow (Package L, section 3). A utility can
score the session itself: participants commit their answers to it, wallet-signed
and only while the pinned window is open, and a policy that sets
`requireCommittedAnswers` grants on the utility's own arithmetic over those
answers and the answer key the pinned content reveals - the provider's signed
score and even its `outcome` are not consulted. The release gate proves it with
a result the gateway's own key signed whose verdict says `not-passed`: the
utility still grants, with `scoring: { by: "utility" }`, while the browser half
keeps the participant's own recomputation. H-3 now records the resolution and
what remains (the utility's own clock and store, answer sharing and guessing,
and the operator's pin); the four external blockers are unchanged.

History, the second revision (still the first acceptance slice of the third
revision). The objective active from 2026-09-29 was:

> Provide a fully permissionless purchase flow: the buyer receives usable,
> authenticated content effectively immediately, with a hard delivery deadline
> of only a few blocks. Otherwise, the purchase payment and retrieval fee must
> be repayable without a trusted operator or an unnecessary manual process.
> Easy access and full decentralisation are acceptance criteria, not optional
> future improvements.

The approving message also fixed the policy: no subsidy (no reserve-funded
acquisition and no author acquisition rewards), one global liquidity pool with
exact per-order and per-account liabilities, author-only inventory donations,
a buyer-funded retrieval fee collected in the buy call and capped by the
authenticated eligible count, and immediate delivery or repayment with a
provisional local-lab deadline of three blocks. The five economic decisions are
answered; they are no longer open blockers. Genuine technical blockers stay in
section 1 and are investigated, not hidden.

History, superseded rather than completed. The objective active from
2026-09-28 was:

> Deliver a verifiable experimental Flips on Demand MVP on an isolated
> play-money chain, with no mandatory central application operator. Complete
> the independently verifiable marketplace lifecycle, protect inventory and
> funds through adversarial sequences, preserve local keys and recovery, and
> demonstrate the supported session-to-utility journey without trusted scoring
> or discretionary operator approval. Finish with a coherent reproducible
> artifact set, real execution receipts, and evidence that independent node
> operators can verify the same outcomes. Keep paid supply disabled until its
> economic, authorization, proof-completeness and accounting prerequisites
> are satisfied.

The plan that serves it is `SPEC.md` 10.3: packages A-F, worked one coherent
package at a time, under the rule that this application adds no mandatory
trusted operator beyond the pinned chain and the cryptography it names. Section
3 is the store of record for what each package changed.

The standing audit cadence in section 5 is part of this goal: while no blocker
is active, the three-hour `goal audit` heartbeat runs the read-only repository
audit, folds each actionable finding into the matrix or package plan with a
proposed fix, and writes anything beyond this session's capability into the
handover notes in section 7.

History, superseded rather than completed. The objective active from
2026-09-26 was:

> Deliver the experimental, ownerless Flips on Demand MVP on an isolated
> play-money chain, following SPEC.md section 10.2 and closing REVIEW.md findings
> A01-A10. The fail-closed containment and its adversarial regressions (D0) and
> the statement half of the chain-authenticated session provenance (D1) are
> landed; from here the objective runs through the live session capture and the
> content check, then the finite issuance and refund accounting (D2), reliable
> buyer-bound delivery and the fresh-CID retrieval fee (D3), the real sell, buy
> and private-session journeys (D4), and one assembly and verification pass on
> the final artefacts (D5). Prevent public-receipt replay and unlimited
> reserve-funded copies; segregate refundable buyer escrow. Keep the contract
> ownerless, keys local, the explorer optional, and fast-sync verification
> portable, without changing Idena consensus. Use one coherent package and one
> focused verification batch at a time. Preserve source pins and privacy; record
> exact evidence in PROGRESS.md. Resolve the explicit economic and settlement
> decisions before implementing their dependent rules. Finish only when the
> final release artifacts and real receipts pass the integrated play-money flow
> and recovery tests, with no open P1 finding. If a required chain commitment or
> decentralized settlement mechanism is impossible on the legacy chain, report
> the bounded feasibility result and exact blocker; never replace it with
> trusted operators or claim completion from fixtures. Mainnet funding/deployment
> and publication are separate, per-action-approved work. Do not poll unchanged
> external blockers or restart already proven spikes.

The desktop goal carried that 2026-09-26 wording; the goal API has no edit
operation, and the replaced text is kept here rather than deleted. Its D0-D5
sequence still describes the same body of work as packages A-F, re-ordered and
extended; where the two disagree, `SPEC.md` 10.3 governs.

On 2026-09-27 the goal was marked **blocked**: every remaining item of the
objective needs the owner or a live node. D2 and D3 wait on the accepted
economic rule and the fee's transport; D1's real-session package needs a synced
node inside a validation window; D5's integrated play-money flow needs the
chain it should run on and the artefacts frozen after D2. The decision sheet
above and the closed/open table below are the exact list a resumption starts
from; no local work in these papers is pending.

### Where the objective stands (2026-09-27)

| Closed | Evidence |
|---|---|
| A04 refused mutations | `error_reply` aborts the call, so the node rolls storage and value back; the D0 regressions and `escrow_no_admin_test.go` pin it |
| A01/A03/A05 containment | reserve-funded supply is disabled (`supply_disabled`) while the issuance rule is open, and the D0 sequence keeps the buyer's refund backed |
| A06 delivery slot | a pending delivery is replaceable; the honest payload settles and the order closes |
| A07 prepared-buy handoff | the review's reproduction now reaches the signer and fails only on the missing key file |
| A02 anchor half | the version 2 statement binds two session state versions and the contract compares each folded root against the header its own chain holds for that height; a receipt that can register exists and the CLI freeze test is restored |
| SPEC 5.5 item 6 | the version 2 guest accepts only the legacy flip content the client's renderer reads - hex parts, RLP lists, four pictures, two differing orders - and the recorded node-built blob whose plaintext is placeholder JSON is the refused regression (`SOURCES.md` F-98) |

| Open, in order | What it needs |
|---|---|
| A02 live half | a capture over a real closed session (`fod session-bundle` against a datadir), then the same receipt path end to end; the frozen evidence is a node-encoded fixture |
| A01/A03/A05 model | the finite, consensus-bound issuance rule and the segregated refund accounting, with the maximum subsidy stated (D2) |
| A02 sellUnit authorization | a supply authorization bound to caller, contract, bundle and a consumed entitlement; a receipt alone is public transaction data (D2) |
| A08 | the buyer's own fetch-and-verify before a buy, and the per-flip check times, are in (`tools/fod/availability_check.go`); what remains is the buyer-funded host credit capped by the authenticated eligible count, which needs the retrieval-fee decision |
| A09 | closed in code: the session shows each flip's two sequences and takes the choice, the provider writes the pool from its export (`fod session-pool`), the answers' provenance travels with it and is shown as the provider's statement, and the sell/give, buy and session-setup paths are documented in the FAQ, the runbook and the CLI. What remains is D2's paid-sell path, which is disabled until the issuance rule exists |
| A10/D5 | `release/` still describes the D0 revision; regenerate the artefacts, guest ids, seals, HTML and the matching CLI constants once, then run the integrated play-money flow |

### Bounded deploy smoke-test allowance (2026-09-26)

Not in force under the 2026-09-28 plan: `SPEC.md` 10.3 authorizes local
changes, tests, builds and disposable isolated local environments only, and
states that this allowance is suspended. The record below is kept as history.

The owner placed the residual balance of a terminated identity (about 37,000
iDNA, explorer-read) at the project's disposal for deployment experiments. That
establishes one narrow, standing allowance:

- A mainnet deploy smoke test may spend from the owner-designated experiment
  address, capped at **1,000 iDNA in total** for the whole series. The measured
  deploy needs about 30,000 user gas (base cost 3,000,000 wasm gas, no deployment
  stake for a custom module: `SOURCES.md` F-12, F-92), roughly 3 iDNA at the
  2026-09-25 rate, so the cap sits far above the expected spend.
- Its purpose is to validate `fod prepare-deploy` -> submit -> node execution and
  read-back with a small, disposable module, for example the ABI spike artefact
  already built under `spikes/contract-abi/dist/`. It is not permission to deploy
  or fund the reviewed escrow, to move reserve money, to open a market, or to
  send anything D0-D2 would invalidate. The current escrow artefacts stay
  undeployed while a P1 finding in `REVIEW.md` is open.
- Deployment is public forever: no private key, vault or node key, flip secret,
  passphrase or token may enter a module, constructor argument, calldata, log or
  committed file. Treat every deployed byte and argument as publishable.
- The signing key stays in the node's own keystore and is never printed or stored
  by the CLI. Each send records network, contract address, transaction hash,
  measured fee and the read-back before the next one is prepared.
- Stop and report before spending if one deploy would exceed 100 iDNA, if the
  measured cost is unexplained, or if a deploy fails: never retry by resending.
- The experiment address is supplied out-of-band and is not recorded here.

Pool funding, the full mainnet exercise of `SPEC.md` 7.5, and publication remain
separate, per-action-approved work.

## 1. Current state

**Not release-ready.** `REVIEW.md` is the bounded review of the clean baseline
`a91e475`. It identifies locally actionable security and workflow defects, so
the previous "packages 0-8 done; only external blockers remain" assessment is
withdrawn. The reviewer made no implementation changes; the D0 and D1 slices
that followed did.

Reusable evidence remains: real Groth16 verification in the runtime, crypto/CID
vectors, local key tooling, ownerless release exports, and provider signatures.
Since the review, a refused paid call aborts instead of reporting success, value
on a non-payable call is refused, reserve-funded supply is disabled while the
issuance rule is open, a pending delivery is replaceable, the prepared-buy file
reaches the signer, and the version 2 registration statement ties a bundle to
two anchored session state versions that the contract checks against its own
chain. What still stands between the tree and a funded release: the live
capture (the frozen session evidence is a node-encoded fixture), a content check
that a customer can solve a bought flip, the finite issuance and refund
accounting, the retrieval-fee path, and the session journey. Continue with the
D2, D3 and D4 packages in that order, then D5.

## 2. Package status

| # | Package | Status | Next action |
|---|---|---|---|
| 0 | Baseline | reusable pins and protocol research | preserve; resolve provenance/notices before distribution |
| 1 | Proof feasibility | real-receipt verifier feasibility established for recorded versions | do not restart the pairing spike; remeasure a changed final statement |
| 2 | Historical evidence | the two-anchor bundle is verified by the contract against its own chain, over a node-encoded fixture (A02 anchor half closed) | D1: live capture over a real session, then the same receipt end to end |
| 3 | Local key tool | crypto/CID/vault components tested; integrated journey incomplete | D4: actual command handoff and bought-content recovery |
| 4 | Proof programs | the version 2 registration statement is proved and the escrow, CLI and harness take it (image id `f8ef4e01…`, claim digest `70111077396fdb…`) | keep the artefact/constant tuple in sync; regenerate only with a guest change, in D5 |
| 5 | Escrow and pool | D0 containment plus the version 2 receipt: refusals abort the call, non-payable value is refused, supply payouts stay disabled, a pending delivery is replaceable, and both anchors are checked against the node's own headers; the economics stay open (A01/A03/A05) | D2 accounting/issuance; D3 delivery guarantee |
| 6 | Local HTML | implemented file interface; user journeys incomplete (A07/A08/A09) | D4 guided flow with actual visible flips |
| 7 | Provider mode | tested scoring/signature example, not bought-flip integration | D4 content adapter, rendering and session setup |
| 8 | FAQ and exports | exports exist; claims and integration need revision | D3/D4; state actual trust and settlement boundaries |
| 9 | Release | blocked by code defects, not by missing mainnet funds; `release/` currently describes the D0 revision | D5 regenerate and run the integrated play-money evidence; mainnet is later |

### Owner decisions

The former 14-row ledger conflated engineering work, already stated constraints,
optional scope and genuinely undecided economics. It no longer blocks all work.
What follows is what the owner has said, quoted, and how each line was resolved.
On 2026-09-29 the owner answered the decision sheet that stood open here (and in
`SPEC.md` 10.3); those answers are quoted verbatim under the table, and they, not
the recommendations in these papers, are the consent. A quotation is consent
only for what it says.

| # | Stated by the owner | Resolution (2026-09-29 answers below) | Blocks |
|---|---|---|---|
| 1 | "i want the flip key market to be handled via a liquidity pool of which noone is in charge and which i ll fill at the start with a few flips and a few idna, like all my flips and 50k idna"; "in such a pool NOONE should be marketmaker and in charge, it has to work 100% DECENTRALISED"; "i dont want anyone to receive extra payments as share from market trades, except the smart contract itself, if needed for gas" | answered: **no subsidy at all**; the pool never funds an acquisition, `sellUnit` stays refused, and only the authenticated author's donation adds a unit. The bootstrap reserve stays a quote anchor and is never spent | closed |
| 2 | "it is important to allow instant settlement, otherwise it becomes to complicated for potential customers who want to use flips to secure something with them" | answered: a **stronger guarantee** than the bare fast path - a hard three-block delivery deadline with no grace period, one permissionless refund transaction that pays the buyer directly, and the reveal settlement so an author who delivered can be paid without the buyer's confirmation | closed |
| 3 | "it should not be about size, but only about eligible flip count"; "there should be a small fee that is payed to the person where the pinned flip was downloaded from"; "pay nobody and leave it off-protocol" | answered: the fee is **collected in the buy call**, capped by the authenticated covered flip count at 1 iDNA per eligible flip, escrowed with the purchase payment, credited only to the hosts the buyer named on settlement, and returned with the payment on timeout or invalid delivery | closed |
| 4 | "people could offer also reported flips but never expect anyone to buy them" | none. Recorded as accepted: reported and weak bundles are listable, the label comes from the chain's own flags, and reserve-funded purchases keep the clean rule | - |

On 2026-09-29 the five decisions of `SPEC.md` 10.3 were put to the owner as
one bundled sheet, each with a recommendation and one alternative: the finite
subsidized acquisition unit and maximum total subsidy; global versus
per-bundle liquidity; donation authorization and quantity; the retrieval fee's
transport, amount per eligible flip and payer; and the measurable meaning of
"instant" delivery or settlement. They refine the open rows above and add
donation authorization. No dependent rule is implemented while they are open;
the answers recorded here end the ask.

Answered by the owner on 2026-09-29, verbatim:

1. Finite subsidized acquisition unit: **no subsidy at all**. The pool never
   funds an acquisition; `sellUnit` stays refused (`supply_disabled`), the
   maximum subsidy is zero, and the only way a unit enters the pool is an
   author-only donation.
2. Liquidity scope: **one global pool**. One reserve and one sale-unit count
   stand for the whole market; per-bundle state keeps inventory, the covered
   count, strikes, the beacon and the nonce.
3. Donation authorization: **author only** (the rule the contract already
   enforces).
4. Retrieval fee: **the buyer pays it in the buy call**, capped by the
   covered flip count and never by bytes; it is escrowed with the purchase
   payment, paid to the hosts the buyer named only on settlement, and returned
   with the payment on timeout or invalid delivery.
5. Instant delivery or settlement: **a stronger guarantee than the bare
   live-desk fast path** - a hard three-block delivery deadline with no grace
   period, one permissionless refund transaction that pays the buyer directly,
   and a reveal settlement that lets the author publish the committed secrets
   and be paid without the buyer's confirmation.

These answers close the ask; dependent rules are implemented from here.

Already established: testing uses play money; no marketplace owner/admin or
commission; no explorer-only authenticity; legacy-chain compatibility; free
donations; retrieval fees based on eligible count, never bytes; English technical
papers; private repository; no publication and no mainnet action beyond the
section 0 deploy smoke-test allowance without explicit per-action approval. The
designated test instance is instance 3; do not broaden remote access to other
identities or instances. End-user UI language can stay English during repair. A
display-language preference or a future mainnet budget does not block local
engineering. Avoid new license reuse while provenance is being checked. Capture
timing is an external input to D1, not a reason to stop D0. The two datadirs on
this machine are months stale and a node keeps only about a hundred state
versions, so a real session capture needs a node that is synced now and stays up
through a `ValidationFinished` block; that node and its window are owner inputs,
and the capture command itself is read-only (`fod session-bundle`).

### Definition-of-done audit (2026-09-26)

The earlier completion audit is superseded by `REVIEW.md`. The release table is
now a requirement map, not permission to use funds. D5 requires a real end-to-end
play-money flow; the existing fixture flow and recorded component proofs are
insufficient. In particular, refund solvency, session binding, supply authority,
and usable participant content are open implementation work.

### Chief developer review (2026-09-26)

Reviewed clean baseline `a91e475` once. Fresh results: CLI suite passes;
11 selected contract/runtime tests pass; 15 registration-core and 1 anchor-core
tests pass; gateway 16 and utility 11 tests pass; app typecheck and the nine-check
example integration pass. Actual prepared-buy -> submit fails with unknown
`bundle`. An integer trace demonstrates insufficient reserve for a pending
refund after repeated sells. Source tracing identifies the remaining A01-A10
findings. Commands, limitations, privacy-scan triage and exit tests are in
`REVIEW.md`. Only working papers were changed; DeepSeek owns implementation.

Owner follow-up after the review (2026-09-26): the desktop goal was replaced with
the section 0 objective, the bounded deploy smoke-test allowance was recorded,
and remaining host and service identifiers in the committed documents were
replaced with role placeholders. No implementation code changed.

## Requirement-to-evidence matrix (living)

One row per decentralisation clause (`SPEC.md` 10.3) and per objective
requirement, with the evidence that currently proves it and its exact limit.
This is the acceptance report in progress; the pending rows name what still
has to happen.

| Requirement | Evidence | Status |
|---|---|---|
| 1. No application authority | only the documented exports ship (`spikes/harness/escrow_no_admin_test.go` walks the module's export section, `reveal` included); refused mutating calls abort with storage and value rollback (`escrow_d0_regressions_test.go`); `sellUnit` stays refused (`supply_disabled`); no upgrade, admin or keeper entry point exists in the escrow | met for escrow only; operator-controlled utility release remains open |
| 2. Evidence proves the claim | the registration statement binds both anchor heights, their state roots, the network, the manifest and key commitments; the confirmation names the digest of the ciphertext the buyer's own tool decrypted (`prepare_confirm.go`, contract `confirm`); `reveal` settles only when `sha256(secrets)` is the bundle key; `SPEC.md` 7.x states what each artifact proves and what it does not | partial: local content checks improved; delivery network/fork binding and canonical availability remain open |
| 3. Production is independently possible | the whole lab journey ran with the CLI and a node RPC only: the author's own `fod serve` desk, the buyer's own `fod receive`/`prepare-*`/`submit`, no project service; the interface is offline (`release/MANIFEST.json` reports 0 remote tags) and the manifest publishes the identifiers a third party checks | historical lab evidence only; no new independent production run in this repair |
| 4. Global rules live in canonical state | the one global pool (`reserve`/`units`/`pending`) and the per-order escrow live in contract storage; `buy` takes the unit atomically; quotes, refunds, strikes and fee credits are canonical transitions pinned by the sequence, two-bundle and restart tests; `accounting` publishes the totals every check compares | met for escrow accounting; utility consumption is local, not canonical |
| 5. Availability is a dependency, not a guarantee | the desk can vanish: order 3 was never delivered and the buyer was repaid in one permissionless transaction; the epoch-228 capture watched the node's own IPFS drop the very flips it served ten minutes earlier, and a public gateway verified 0 of 201 blobs | partial: retained-byte normal CLI path tested; arbitrary contract callers and independent operator availability remain unproved |
| 6. The utility is part of the claim | `release/check-integration.mjs` (no chain) runs the session -> grant -> replay-refused -> quota-once loop with a utility that scores the session itself: both customers commit their answers inside the pinned window (`/answer-commitment`, wallet-signed), the utility recomputes the score from those answers and the answer key the pinned content reveals (never from the provider's score or `outcome`), the browser independently recomputes the same score from its own answers, and the provider's signature still has to bind the session and participant; the gate forges a result the gateway's own key signed whose verdict says `not-passed` and the utility grants anyway with `scoring: { by: "utility" }`, while a signature-tampered result is refused (`bad_signature`) | not met for the full-product goal: reference adapter recomputes answers, but its clock/store/pin and actual benefit release remain operator-controlled |
| 7. Bootstrap and rule changes are independent | the historical CLI path uses release artifacts and a node RPC; no project account, website, indexer or update server is mandatory in that path; an optional shared-node watcher can preserve portable anchors and lottery observations without seller key access; format versions travel in the artifacts and the contract has no unilateral upgrade or allowlist | historical own-node lab path; independent lightweight buying and the explicit API-only risk route remain unimplemented |
| Objective: optional historical capture for low-tech sellers | `session_indexer.py` is active on the test host and archived a five-CID epoch-228 bundle in a replay after its source snapshot was released; failure retains a boundary snapshot, and the private live archive is armed for epoch 230 | partial: no epoch-230 lottery or boundary evidence yet; the two nodes share one host, lottery records are node observations, and no ciphertext serving or funded purchase is qualified |
| Objective: lifecycle with real receipts | the served bundle's live registration from a sealed receipt (`0x5366f8ea...`, `gasUsed` 1,958,525, image `7ee79d90...`), then order 1 confirmation (settled 2409), order 2 reveal (settled by 2503 after the buyer verified the delivery), order 3 refund (`0xb338bfe2...`, `gasUsed` 13,579, `gasCost` 135.79, `txFee` 12.1) and order 4 fulfilment by the delivery proof (`0xa082ce99...`, `gasUsed` 1,972,769, `gasCost` 19,727.69), with the buyer never confirming or refunding; the confirm and the two withdrawals were re-read read-only from both lab nodes on 2026-09-30 and each receipt is byte-identical across them (section 3) | historical pre-repair receipts retained; no new chain execution receipts generated by this authorized local repair |
| Objective: adversarial sequences | junk-first delivery, duplicate settlement, cross-order settlement, unbacked withdrawal, fee above the cap, delivery and refund boundary blocks, a stranger's donation, buy-proof replay, refunds backed while supply is disabled, and two real orders open on one bundle with separate liabilities | met (harness) |
| Objective: local keys and recovery | `fod receive` writes an owner-only vault; restart recovery and per-buyer separation are pinned by `escrow_restart_test.go` and the pool-receive tests; the buyer's own check runs before any confirmation | met locally after S3: atomic vault publication, existing-key recovery and different-purchase refusal; synthetic regressions only |
| Objective: independent nodes verify the same outcomes | two lab nodes report the same head, the same market and accounting payloads, and byte-identical receipts for the refund and the earlier refund/withdraw pair; re-read on 2026-09-30 with the current CLI: both nodes return the same `fod market` payload for escrow `0x8a225b8e...` (code hash `54caa028...`, 1 listing, 3 orders, 1 credit) and now the same printed `state digest 11968c6e...` at block 7254, plus the same `bcn_txReceipt` for the confirm (`gasUsed` 20,357, digest `dc4923b4...`) and the two withdrawals (`3,724` / `4,227`, digests `8a84a847...` / `4faeb717...`) | configuration-level historical agreement only; independent-operator availability is not established by one host |
| Objective: coherent reproducible artifacts | `release/build-manifest.mjs` at revision `1462782b8f15` (dirty worktree): interface 302,236 bytes `e3922fcf...`, module 380,347 bytes `0769bbf0...`, 0 forbidden operators, 0 remote tags; the module and the interface both rebuilt byte for byte from the same sources in this pass; `release/check-integration.mjs` ALL CHECKS PASSED | local build/check evidence only; dirty source is not reconstructable from the HEAD revision alone |
| Objective: documented boundaries match the code | the user-facing FAQ (`app/src/faq.ts`) and `SPEC.md` 7.2/11.1 carry the implemented three-block window, the direct permissionless refund, the author-only donation rule, the reveal settlement and the current measured fees | corrected for strict utility grants and contributor-owned reserve; deadline wording discrepancy remains explicit in SPEC 10.6 |
| Objective: key material stays owner-only | vaults, key archives, session pools and `receive` outputs were already 0600; audit 1 found three key-bearing writers at 0644 - the prepared reveal, the session-bundle blob witness and the `prepare-offer` registration input - and all three now go through `writePrivateJSON` (0600 on create and on rewrite), pinned by `TestRevealActionFileIsOwnerOnly` and `TestRegistrationInputIsOwnerOnly`; audit 5 then found the rewrite gap in `fod receive`/`fod export`/`fod session-pool`, closed by `writePrivateBytes` and the three rewrite regressions (AH-20) | met |
| Objective: the tree carries no operator identities or unignored local state | audit 5 found the live Idena identities in `docs/PROGRESS.md` and test fixtures, and `tools/fod/sweep.json` untracked and not ignored (scan `e87b560e` finding 3); the full addresses are now redacted to the prefix form the documents already use, tests and fixtures use synthetic addresses, and the CLI outputs are ignored (`git check-ignore tools/fod/sweep.json` succeeds) | met with the pinned IAVL proof fixture as the one stated exception (AH-21) |
| Objective: paid supply stays disabled | the payout path is compiled off, `sellUnit` refuses, and the test that keeps refunds backed while it is disabled passes | met |
| Package G1 EVM customer identity | the Use flips view connects an EIP-1193 wallet, asks the gateway for a one-time `fod-enroll:v2` challenge and signs it, and the gateway binds the address into the token, the signed result and the utility grant; the gateway tests cover a good binding, a bad signature, a stale challenge, a consumed nonce and a cross-provider challenge, and the browser walk checks the address in the result and the grant | met for the reference example; the address is a key, not a person, and the utility scope kind stays `provider-participant` for now |
| Package G2 on-demand session | the reference gateway publishes `GET /session/lobby` (window state, participant count, challenge count); the gateway test proves two participants in one window get the same challenge set, and the results carry the G1 address binding; the real epoch-228 pool still needs the seller's `fod export-keys` vault because the capture holds no epoch secrets | partially met; the real-pool build is blocked on the seller's vault (owner input) |
| Package G3 shared-node supply | the gateway publishes `GET /shared-node/status` (node, session, utility, provenance, operations, custody modes); the Use flips view imports a `fod.shared-node` connection, shows the node status plus the self/hosted custody boundary, names the node command and still generates the file by hand; `fod shared-node` performs the node's export and the pool from the author's vault and writes the connection file (the author the escrow pays, the pool the gateway loads, the custody boundary), with tests for the hosted and self paths and for the refusals (`TestSharedNode*`), and the walk pins the journey text | mostly met: the node's export, pool and vault integration are implemented and tested; the listing/donation stays the author's own prepared action and the delivery desk is the separate long-running `fod serve`, so the two are not stitched into one command |
| Package G4 buyer testbed | the Use flips example gate, example flips and the reference test utility complete the browser walk: solve, verify the provider result, claim the grant; audit 2 F1-F3 are fixed: the claim requires a browser-verified result, the connection note names both URLs, and the utility policy carries an explicit origins list; the production utility's provider trust remains H-3 | met for the reference example |
| Package G5 payment and custody | the play-money lab remains the only purchase path; the sponsored/relayed Idena buy is the recommended first customer path and the EVM-side escrow is deferred; the owner decision is pending | open - owner decision |
| Pending | the unfunded mainnet deploy cost is measured in section 3; funded-call costs and wall-clock behavior remain unmeasured (`SPEC.md` O-9). The epoch-228 ceremony capture is recorded below, with its empty epoch-228 key census and the second identity's zero flips stated there | open |


### Package H rows (audit hardening, 2026-09-30; AH-1..AH-18 closed 2026-09-30, AH-14 by recorded acceptance and AH-18 by the fee-table correction; AH-19..AH-23 opened and closed 2026-09-30 by audit 5 and the same-day fixes)

| Requirement (`SPEC.md` 10.5) | Evidence that closes it | Status |
|---|---|---|
| AH-1 no per-answer oracle | gateway regressions: answers carry no correctness, the result is refused until close, and a second live enrollment for the same address is refused | closed |
| AH-2 no offline answer enumeration | the digest preimage carries a per-session salt; the open policy serves neither salt nor answer digest, and the closed policy's reveal recomputes to the pinned digest | closed |
| AH-3 one-time enrollment nonce | replay gives `unknown_nonce`, a challenge naming another provider gives `bad_challenge`, and a challenge older than five minutes gives `stale_challenge` | closed |
| AH-4 rate limit that survives a proxy | two clients behind a configured trusted proxy have separate buckets; an untrusted forwarded header cannot mint one (gateway and utility) | closed |
| AH-5 no eviction of live participants | at capacity a newcomer is refused with `session_full`, a lobby read keeps the live participant, and its token still answers | closed |
| AH-6 claimant control at the utility | a wallet-named result without the signed one-time challenge is refused (`claim_control_required`); with it, granted once, and a replay is `unknown_nonce` | closed |
| AH-7 explicit grant scope and reset behavior | grants carry `kind: "wallet"` or `"provider-participant"` and key accordingly; one wallet under two participant ids shares its quota, and `--grants FILE` keeps a spent grant across a reconstruction (0600 file) | closed |
| AH-8 provider pin round trip | one agreed encoding (88-hex SPKI from `GET /provider/key`) in the parser and the browser import; the gate-setup form writes the pasted key into a generated connection (walk check), and a substituted key shows "not verified" (walk half two) | closed |
| AH-9 loopback default | the gateway `host` defaults to `127.0.0.1` and the utility binds loopback unless `--host` names another interface (gateway regression "binds loopback unless the config names another host") | closed |
| AH-10 delivery replacement cannot strand the checked bytes | `confirm` carries the buyer's checked ciphertext and settles on it whatever the pending slot holds; junk-first, replacement and honest-desk sequences pinned (`TestD0JunkFirstDeliveryDoesNotOwnTheSlot`, the CLI's checked-bytes record). The live chain settles the same call once the fee budget is right: order 2 confirmed in tx `0xdd130fd1…` (`settled`, host credit 1 iDNA, `pending` 0) | closed |
| AH-11 no donation to a cleared listing | a cleared bundle's donation is refused and the global units stay unchanged (`TestPoolClearedListingRefusesDonations`) | closed |
| AH-12 a revealed bundle stops selling | later buy and donation are refused; every open same-bundle order closes atomically (`TestPoolRevealStopsTheListing`, `TestPoolRevealSettlesEveryOpenOrderOnBundle`) | closed locally; current-module chain receipt absent |
| AH-13 refund and late settlement pinned | S5 supersedes the previous +3 settlement/+4 refund boundary: unilateral paths refuse at +3, permissionless repayment opens at +3; both orderings preserve the original buyer's payment/fee (`TestPoolRefundOnlyBoundaryBothOrderings`) | closed locally for the new candidate; previous live receipts qualify the previous module |
| AH-14 delivery chain domain | S6 replaces recorded acceptance with delivery-v2: canonical network digest, funding height/root checked against the executing node; genuine seal verifies through the contract, wrong/missing anchors and legacy statements refuse | closed locally for the new tuple; shared-anchor descendant forks retain the chain assumption; no deployment |
| AH-15 reserve contribution rule | each deposit is recorded against the address that paid it, and `withdrawReserve` returns it while the reserve covers it and buyer escrow and host credits stay covered (`TestPoolReserveDonationIsWithdrawable`, the `withdraw-reserve` CLI path); the v4 lab's owner then withdrew the whole 200 iDNA donation live (tx `0x7b91a991…`, epoch 28) after taking the 1 iDNA host credit (`0xd24d10e4…`), leaving reserve 0, credit 0 and contract balance 0 with nothing outstanding | closed |
| AH-16 verification environment | `playwright` is declared in `app/package.json`, and `release/check-integration.mjs` runs the walk first and fails loudly when it cannot run (13 checks) | closed |
| AH-17 availability is a settlement precondition | `prepare-buy` refuses without an advisory, checks the CID set against the chain's inventory commitment over the canonical CID text the registration guest hashes (the decoded-bytes form of the first cut was refused live on 2026-09-30; `TestTheInventoryCommitmentCommitsTheCanonicalCIDText` pins the chain vector, and the v4 lab's real bundle printed `checked 1/1 … the advisory's CIDs match the bundle's committed inventory`) and rebuilds every byte; `submit` refuses a buy check that is missing, incomplete or stale (`TestPreparePoolBuyFromTheInterfaceRequest`, `TestPreparePoolBuyRepeatsTheAvailabilityCheck`, `TestSubmitRefusesABuyWithoutAFreshAvailabilityCheck`) | closed |
| AH-18 the buyer's `confirm` executes on the live node | closed: the call was underpriced, not refused - the node derives the gas budget from the signed max fee, the 219-byte confirm needs 20,357 user gas, and the fee table carried the superseded 14,747 so the proposal ran out of gas and blacklisted the hash. The row now carries the live `contract_estimateCall` measurement, `TestTheConfirmRowCarriesTheAH10Envelope` pins it, the guard refuses the 200 iDNA envelope before broadcasting (`needs a max fee of 238.67…`), and order 2 settled live in tx `0xdd130fd1…`. Evidence: the 2026-09-30 resolution entry | closed |
| AH-19 a delivered order cannot be refunded while the buyer keeps the content | closed: `fod serve` plans a `reveal` for a delivered order this desk published once it has `revealMargin` blocks or fewer left (default 2, `-reveal-margin`) and a configured prover's `fulfill` no longer fits; a funded order and a foreign delivery slot are never revealed. The prepared reveal carries the committed secrets and is written owner-only. Tests: `TestServePlanRevealsAnUnconfirmedDeliveryAtTheBoundary`, `TestServeWritesTheRevealActionOwnerOnly`; the escrow boundary is pinned by `TestPoolRevealSettlesWithoutTheBuyer` and `TestPoolUnilateralSettlementsCloseAtTheDeadline` | closed (2026-09-30) |
| AH-20 sensitive outputs tighten a pre-existing file | closed: `writePrivateBytes` (0600 on create and re-applied on rewrite) backs `fod receive`, `fod export` and `fod session-pool`, and `writeExportPart` re-applies its mode; three tests pre-create a 0644 destination and assert 0600 (`TestReceiveFlipEndToEnd`, `TestExportTightensAPreExistingOutput`, `TestSessionPoolCarriesContentAndAnswers`) | closed (2026-09-30) |
| AH-21 the publishable tree carries no operator identities or local lab state | closed with one stated exception: the full addresses are redacted to the prefix form the documents already use, the test constants and fixtures use synthetic addresses, and `.gitignore` covers `sweep.json`, `epoch-keys*.json` and `*.private` (`git check-ignore tools/fod/sweep.json` succeeds). The one remaining real key is `spikes/iavl-proof/core/fixtures/bundle.json`, a pinned real Merkle proof that cannot be edited without invalidating it; a synthetic replacement is the pre-publication task if that fixture must be removed | closed with the fixture exception (2026-09-30) |
| AH-22 a failing release walk leaves diagnosable evidence | closed: `release/check-integration.mjs` keeps the last 40 non-empty lines in the failure detail and writes the full walk output to `$TMPDIR/fod-walk-failure.log` before failing; the same trace showed the walk's own 3.5 s session window could close before the session opened under load, so both walk gateways now use an 8 s window (`app/verify.mjs`) | closed (2026-09-30) |
| AH-23 the test-hooks artefact cannot be deployed by mistake | closed: the harness artefact is `contracts/escrow/dist/fod_escrow.testhooks.wasm` (`spikes/harness` reads it), the release build no longer copies into `dist/`, and `release/build-manifest.mjs` refuses to promote a staged module whose export section is not the published 19-name surface (no `seed*`; `checks.releaseExportSurface`); the manifest regenerated for that closure pinned the interface at 293,805 bytes `b5b3d5aa…` and the module at 380,347 bytes `0769bbf0…` (later revisions re-pin the interface; the module is unchanged) | closed (2026-09-30) |

`REVIEW.md`'s findings, each with the evidence that closes it on the supported
path:

| Finding | Closure |
|---|---|
| A01 a public receipt authorizes payouts to arbitrary callers | `sellUnit` is refused by the accepted policy and the pool's only supply path is the registered author's donation; `TestD0RegistrationReceiptDoesNotPayItsPresenter` pins that presenting a receipt pays nothing |
| A02 the registration proof is not a proof of a real session | the statement binds two anchored session state versions that the contract checks against its own headers, and a live bundle was registered from a sealed receipt on the current module (tx `0x5366f8ea…`) |
| A03 refund money can be spent on supply payouts | there is no subsidy; each order's payment stays its own pending liability and a refund pays it out directly (`TestPoolFeeCapAndRefundReturnTheFee`, `TestD0RefundsStayBackedWhileSupplyIsDisabled`, `TestPoolConcurrentOrdersOnOneBundleStaySeparate`) |
| A04 rejection returns success instead of reverting | `error_reply` aborts the call and the runtime rolls storage and value back; the D0 regressions pin it |
| A05 unlimited copies are treated as finite inventory | inventory is finite state, units enter only by the registered author's donation, and the reserve never funds an acquisition |
| A06 an outsider can occupy the delivery slot | a pending delivery is replaceable until settlement and the buyer's own checked digest decides which bytes settle (`TestD0JunkFirstDeliveryDoesNotOwnTheSlot`, `TestPoolDeliveryDeadlineBoundaries`) |
| A07 the normal buy file cannot reach the signer | four orders were prepared, signed and broadcast through the CLI on 2026-09-29, each settling or refunding on chain |
| A08 availability is a badge and the retrieval-fee path is absent | the buyer's own CID-checked availability check runs before signing and the fee is escrowed inside the buy call, capped by the authenticated covered count and returned on timeout |
| A09 the private-session journey never displays a flip | the session view shows each flip's two sequences and takes the choice, and the interface walk passes (`cd app && npm run verify`) |
| A10 status and release evidence overstate completion | this matrix and the corrected `release/README.md` state the verified scope and each remaining limit, while the historical entries stay in the log |

## Final acceptance report (living)

**Delivered result.** A verifiable play-money marketplace runs on an isolated
two-node chain (network 33411, escrow `0x2cfbbafa229f53a9c2009551a832019b3fc94244`,
nodes on RPC 19095 and 19097). A bundle registers from a sealed receipt whose
two session anchors the contract checks against its own headers; the registered
author adds inventory only as a donation; a buyer pays the pool's quote plus a
capped retrieval fee in the same call; the desk publishes buyer-bound
ciphertext inside a three-block window; the buyer's own tool verifies it before
settlement; settlement is by confirmation, by reveal, or by the delivery proof;
a missed window is repaid to the buyer in one permissionless transaction; a
host's retrieval-fee credit is withdrawn with `withdraw`; and the buyer's
purchased keys open the seller's actual flip content, not only its commitment.
No owner, admin, keeper, allowlist or required hosted service sits on any path,
and paid supply is compiled off.

**Real receipts on that deployment.**

| Step | Receipt |
|---|---|
| bundle registration | `registerBundle` tx `0x5366f8ea…`, `gasUsed` 1,958,525, `gasCost` 19,585.25, registration image `7ee79d90…` |
| purchase and delivery | order 1 funded 2398, delivered inside the window, confirmed `0xa653075…` (`gasUsed` 14,689); orders 2-4 prepared, signed and broadcast through the same CLI |
| measured delivery latency | order 2 funded in block 2496 and read back `delivered` at 2497: the desk published within one block, with a 5 s pass interval against the lab's observed 20 s per block |
| reveal settlement | `0x6fe5a420…`, `gasUsed` 12,681, `gasCost` 126.81, after the deadline with no confirmation |
| timeout refund | `0xb338bfe2…`, `gasUsed` 13,579, `gasCost` 135.79, submitted by a third party's key, buyer repaid exactly `+201` iDNA in one transaction |
| delivery proof | `0xa082ce99…`, `gasUsed` 1,972,769, `gasCost` 19,727.69, after a ~16-minute `r0vm` proof |

Both nodes report the same head and the same market and accounting payloads, the
refund and the fulfilment receipts read identically from both, and the
registration, confirmation and reveal receipts were read back from the chain on
the node that broadcast them.

**Reproducible verification.** The "Verify this release" section of
`release/README.md` lists the exact local commands. On this revision they give:
interface 302,236 bytes `e3922fcf…`, module 380,347 bytes `0769bbf0…`, 0
forbidden operators, 0 remote tags; two consecutive interface builds produced
the same `e3922fcf…` hash and the escrow module's earlier clean-checkout
byte-for-byte result stands; `release/check-integration.mjs` 18 of 18;
`cd app && npm run verify` 60 checks; `tools/fod` (with the usage test) and
`spikes/harness` suites green; `contracts/escrow` 15 of 15; `proofs/session-core`
19 of 19; the sealed registration artefact 1 of 1 against the guest this tree
builds.

**Current-revision sweep (2026-09-30).** The receipts above are the
2026-09-29 deployment. On the current revision a fresh v4 deployment
(`0x8a225b8e…`) re-ran the same journey: registration, donations, reserve,
two buys, two deliveries, two buyer-side verified receives, the permissionless
refund, the buyer's own confirm and both exits - the host credit withdrawal
`0xd24d10e4…` and the donor's reserve withdrawal `0x7b91a991…` - all landed
(section 3, sixth revision).
The two confirms that stalled were underpriced: the fee table carried the
pre-`AH-10` figure, the call ran out of gas and the node blacklisted the hash
(`AH-18`). With the measured budget the same call settled order 2 in tx
`0xdd130fd1…` (`settled`, host credit 1 iDNA, `pending` 0), so the live
buyer-side settlement claim is current again.

**Requirement mapping.** The decentralisation clauses 1-7 of `SPEC.md` 10.3 and
the objective's requirements each have a row in the matrix above, and
`REVIEW.md`'s A01-A10 each have their closing evidence in the table above it.

**Precise remaining blockers.** (1) Mainnet wall-clock costs remain unmeasured
(`SPEC.md` O-9); measuring them needs an explicit mainnet authorization that
this task does not carry. (2) Operator independence is configuration-level: a
second operator needs a second host, outside this task's authorized scope.
(3) A utility no longer has to trust the provider's signed result for its own
grant: with `requireCommittedAnswers` it scores the session itself from the
answers participants commit to it while the window is open (Package L), and the
release gate proves a grant that the provider's own signed verdict denied.
What remains is the utility itself - its availability, its clock and its store
are the gate's own dependencies, and `SPEC.md` 8.3 states them rather than
hiding them. The epoch-228 ceremony capture is collected and
recorded in the evidence log; the empty key census is not a blocker because
`fod export-keys` re-derives the epoch-228 secrets from the identity key
(pinned by `TestTheCliOpensTheNodeBuiltBlob`), so the remaining limit is the
second identity's lack of seller-side epoch-228 inventory; it can still act as
the buyer.

Package H (`SPEC.md` 10.5, section 6 audits 3 and 4) holds the 2026-09-30
hardening items. AH-1 to AH-5 (the session-gate oracle, the salted digest,
one-time enrollment nonces, proxy-aware limits and no eviction of live
participants) are closed by the P1 entry in section 3, and AH-11 to AH-13 (the
cleared-listing donation, the revealed-bundle state and the hard repayment
boundary) by the P2a entry, AH-17 (the mandatory availability check) by P2b,
AH-6/AH-7 (claim control and explicit grant scope) by P3, and AH-9/AH-15/AH-16
(loopback defaults, the donor-owned reserve exit and the release walk) by P4.
AH-8 (the provider-pin round trip) is closed by P5a, and AH-10 with AH-14 by
P5b: the buyer's checked ciphertext settles the order under the harness, and
the delivery journal's missing chain domain is a recorded acceptance with the
concrete fix in H-5. AH-18 was resolved the same day it opened (the fee table
carried the pre-`AH-10` confirm figure; see the resolution entry in section
3), so every AH-1..AH-18 item is closed for this revision; audit 5 then opened
AH-19..AH-23 (the unconfirmed-delivery settlement gap, the key-writer rewrite
gap, the tree's identity and unignored-output exposure, the truncated
walk-failure evidence and the test-hooks artefact path), and all five are
closed by the same-day fixes recorded in section 3. The remaining external
limits are the four blockers and the H-3/H-5 handovers, all stated in section 7.

### Security repair rows (scan edd7e9ae, current verdict 2026-10-01)

Numbers below are the completed report's original finding numbers.

| Finding | Fresh evidence | Status |
|---|---|---|
| SEC-1 / report 1, CID attestation fallback | `TestCIDVerificationNeverTrustsTheServingPeersAttestation`, local raw/UnixFS reconstruction and dishonest callback refusal | closed locally |
| SEC-2 / report 2, transient availability and editable prepared summary | retained encrypted bytes, signing-time canonical bundle/nonce/quote/fee/context recheck, buyer-delivery-key signed content receipt and canonical confirmation recheck, vanished-peer recovery and edited-file refusal | mitigated on the normal own-node CLI path; canonical byte availability and two-sided off-chain fair exchange remain open |
| SEC-3 / report 3, signature-only utility grant | unsafe policy files and programmatic policies refuse; wallet-signed in-window answers, independently configured pin/window, closed-window scoring and once-consumed control nonce required | adapter bypass closed; fully permissionless scoring/utility release remains blocked |
| SEC-4 / report 4, vault truncation | atomic encrypted publication, identical-key reuse, second-purchase refusal with unchanged first-vault bytes, interrupted-write refusal | closed locally |
| SEC-5 / report 5, unbounded remote buffers | 16 MiB declared/streaming cap on RPC, explorer and IPFS reads; oversize refusal is not retried; normal/boundary responses pass | closed locally |
| SEC-6 / report 6, imported export paths | canonical CID filenames and directory-rooted I/O; traversal/input symlink/output symlink regressions and valid normal export | closed locally |
| SEC-7 / report 7, pinless verification | `verifyPinnedProviderResult` refuses absent/substituted pins and provider mismatch; valid distributed pin passes; browser walk refuses grant after substitution | closed locally; distributing the initial pin is a stated trust/bootstrapping requirement |
| SEC-8 / additional release gate finding, ignored disassembler failure | nonzero `wasm-objdump` shim refuses the candidate, preserves released WASM/manifest hashes and removes staging | closed locally |
| SEC-9 / additional local quota finding, capacity evicts consumed grants | 10,000-entry memory/durable stores refuse newcomers without erasing a live allowance; expiry frees capacity and restart preserves the replacement | closed locally (40 utility tests/typecheck passed) |

### Remaining-limit repairs (current candidate, 2026-10-01)

| Requirement / handover | Evidence | Current status |
|---|---|---|
| S5 / H-8 exact three-block boundary | Runtime rejects deliver/fulfill/reveal at funded+3 in both refund orderings; CLI deadline and first refundable block agree; direct refund returns price plus fee to the original buyer | closed locally; new chain receipts absent |
| S6 / H-5 delivery domain | Genuine delivery-v2 Groth16 receipt, native verification, runtime domain/root/replay refusals, coordinated guest/contract/CLI/manifest identifiers | closed locally; same-funding-anchor forks still use canonical-chain assumptions |
| S7 / H-9 dirty source identity | Source inventory and private export; source drift/symlink/existing-destination refusals; clean HTML/WASM rebuild hashes match; immutable-reference CLI builds reproduce; S10 reproduces both active guest programs and verifies their new genuine receipts | local source/build evidence established; no cold-cache cross-platform qualification or fresh chain receipts |
| S8 / H-7 two-sided fair exchange | Registration decrypts/checks CID-bound blobs; retained purchase bytes survive serving loss; negative control learns unincluded keys and subsequently refunds | open; current construction does not protect both parties against unincluded disclosure |
| S9 / H-7 canonical settled-key recovery | `preveal/<id>` written only with successful settlement; CLI verifies committed keys and retained content without desk/IPFS, supports legacy settled ciphertext, refuses invalid state/keys and duplicate confirmation | locally verified for the explicit public-reveal path; not canonical encrypted-content availability |
| S9 / H-7 disclosure timing containment | CLI and desk recheck canonical order and completed head before signing/broadcast; next inclusion must be strictly before N+3 | known-late disclosure contained locally; pending-transaction exposure and inclusion liveness remain open |
| S10 / H-10 portable proof tuple | Source-only fixed `/src` builds with immutable Rust 1.91.1 builder and SDK 3.0.6; two fresh source/target locations produce identical complete programs; genuine seals verify natively and in WASM; contract/CLI/manifest pins agree; retired seal and build overrides refuse | closed locally for the active tuple; existing deployments retain old pins; same-host build reproduction is not independent-operator evidence |
| S8 / H-3 utility release | Witnessed local answers and score checks pass the reference integration, but answer authority, time/store and actual benefit remain operator-controlled | open; canonical answer/window/consumption/release mechanism and concrete utility target needed |
| S11 timely safe settlement | Delivery-v2 proving took 946.459854 seconds after the funding anchor. Runtime/CLI containment now closes up to eight same-bundle orders in one reveal, refuses a ninth, an expired sibling and a prior refund, and retains all settled recovery records. The desk plan and the signing guard share one disclosure bound (`lastDisclosureHead`), so a delivery is withheld once no later reveal can be included and a funded order at that head is settled by the reveal itself. The eight-order/128-fee-entry runtime fixture used 1,483,236 WASM gas; it is not a chain cost. S8's negative control still learns unincluded keys and subsequently refunds. | open; the proof remains unqualified for three blocks. Pending-transaction disclosure, private-delivery fairness, canonical byte availability, actual inclusion/cost and independently authenticated lightweight operation remain unproved. Require a faster independently verifiable design without extending the deadline. |
| Lightweight authentication / H-1 independent operators | No current authenticated light-client purchase route or second-operator host evidence | open; optional public RPC/mirrors do not authenticate state by assertion |

### SA security audit repairs — 2026-10-03

These rows close local findings against the starting dirty worktree, not the
unfinished marketplace's architectural guarantees. Native scan `31766947`
retains the finding details, baseline evidence and verification logs.

| Finding | Severity | Boundary and regression evidence | Status |
|---|---|---|---|
| SA-01 | medium | Claim nonce can be consumed twice during concurrent signature checks; Concurrent signature checks return exactly one success and one unknown_nonce. (`utility/test/security.test.ts`) | closed locally |
| SA-02 | medium | Answer replacements bypass the utility's stored-byte budget; Oversized and multibyte replacements refuse and preserve the preceding valid commitment. (`utility/test/security.test.ts`) | closed locally |
| SA-03 | low | Predictable grant staging follows a planted symlink; A preplanted temporary symlink cannot alter the victim; persisted grants reopen correctly. (`utility/test/security.test.ts`) | closed locally |
| SA-04 | medium | Export can label substituted decryptable blobs with an unrelated author and CID; Substituted blob, mismatched vault/inventory author, and re-CIDed wrong-author blob all refuse; normal export passes. (`tools/fod/export_test.go`) | closed locally |
| SA-05 | medium | Concurrent gateway starts can publish different application identities; Twelve concurrent starts use one persisted identity; aliases, another provider and mismatched public keys refuse. (`gateway/test/keys.test.ts`) | closed locally |
| SA-06 | medium | Gateway challenge images can cause unintended third-party or local requests; Remote/local/file URLs, SVG and SVG disguised as PNG refuse; a supported raster fixture and real browser flow pass. (`app/test/provider.test.ts`) | closed locally |
| SA-07 | medium | Browser adapters buffer unbounded endpoint bodies; Declared and lengthless oversize bodies cancel; malformed shared-node status is refused without crashing; ordinary responses pass. (`app/test/provider.test.ts; app/verify.mjs`) | closed locally |
| SA-08 | medium | Untrusted endpoint challenge text reaches the wallet signer; Arbitrary, foreign-provider/session, wrong-wallet and stale prompts refuse; legitimate enrollment and claim prompts pass. (`app/test/provider.test.ts`) | closed locally |
| SA-09 | low | A score transcript can omit pinned challenges and still be labelled verified; An omitted pinned challenge and an unknown unanswered challenge refuse; complete supported transcripts pass. (`app/test/score.test.ts`) | closed locally |
| SA-10 | medium | Delivery-key first creation can overwrite a concurrent buyer's key; Twelve concurrent attempts publish one key; a dangling symlink cannot create a key outside the requested path. (`tools/fod/local_safety_test.go`) | closed locally |
| SA-11 | low | Inventory-cache writes follow destination symlinks; A planted CID symlink leaves the outside file unchanged and retains the authenticated bytes; CLI suite passes. (`tools/fod/local_safety_test.go`) | closed locally |
| SA-12 | medium | RPC redirects forward credential-bearing and signed request bodies; A two-server redirect receives no forwarded request; HTTP 403 with result refuses after one call; ordinary RPCs pass. (`tools/fod/rpc_limits_test.go`) | closed locally |
| SA-13 | medium | Unbroadcast reveal transactions can expose their secrets in logs or public files; Reveal without -out refuses before key loading; valid offline reveal preserves its transaction in a 0600 file without printing secrets or following a destination symlink. (`tools/fod/reveal_test.go; tools/fod/submit_test.go`) | closed locally |
| SA-14 | low | Rate-limit capacity eviction forgets live clients' consumed allowances; At capacity a newcomer and an exhausted original key both refuse; after expiry a new key passes in each adapter. (`gateway/test/gateway.test.ts; utility/test/security.test.ts`) | closed locally |
| SA-15 | medium | Source snapshots can export runtime credentials under custom filenames; Application/epoch/delivery key payloads with lower-case, upper-case and absent suffixes and private PEM refuse; ordinary dirty-source export passes. (`release/source-snapshot.test.mjs`) | closed locally |
| SA-16 | low | Verification can contact an existing service when its own child cannot bind; An occupied-port fixture receives zero requests from verification, stays alive, and the verifier fails; the normal integrated journey passes. (`release/service-isolation.test.mjs`) | closed locally |
## 3. Evidence log

### 2026-10-06 — optional idena-go ceremony cache built locally

- Source: isolated idena-go worktree based on
  `9bc57040337fd19f45ba65323939cac8789b9efd`, uncommitted. Main new
  files: `core/state/ceremony_evidence.go` (SHA-256
  `40e2155d5bbcefffa76420145a4db052c80eba48679be9a480e48390b7003c01`)
  and `core/ceremony/evidence_cache.go` (SHA-256
  `e021e0debd0a31873da093c94a428d403e6f1bc5fee5e606430aad04ddc5fd3b`).
- Focused verification in that worktree: `go test ./core/state ./core/ceremony
  ./config` passed; `go build -o /tmp/idena-go-fod-evidence-cache-test .`
  passed; `git diff --check` passed. Synthetic tests cover complete identity
  ranges, wrong observed lottery candidate sets, canonical-root refusal,
  missing lottery, seed-height/canonical refusal, restart, one-epoch pruning,
  private file modes, symlink refusal and
  exclusion of an injected local node-key marker.
- Consumer check: generated synthetic author anchors were accepted unchanged
  by `fod session-bundle -inventory-anchor ... -verdict-anchor ... -author ...
  -out ...` with the FOD CLI temporarily pointed at the isolated node worktree;
  it produced an epoch-31, one-flip bundle. This is schema/consumer evidence,
  not a real ceremony or a chain receipt.
- Limitation: the feature is off by default and was not deployed, enabled or
  measured on live nodes. A full-ceremony size/timing measurement, ciphertext
  retention, independent-operator serving and the purchase/settlement guarantees
  remain open. No remote node, transaction, wallet key or contract was touched.

### 2026-10-04 — optional two-node session-anchor archive armed for epoch 230

- Code: `tools/fod/sessionanchor.go`, `sessionanchor_test.go`,
  `sessionbundle.go`, `commands.go`, `main.go`, `session_indexer.py` and
  `session_indexer_test.py` add portable public IAVL anchor extraction and a
  read-only watcher. During validation it archives each node's lottery seed
  and candidate order if both agree, labelling them local observations; at the
  boundary it checks both state roots against the nodes' headers. Failed
  extraction keeps a private state snapshot; successful extraction removes it.
  The watcher reads a loopback RPC credential in memory, never reads an identity
  node key, and publishes neither credential nor epoch secrets.
- Local checks: `python3 -m py_compile tools/fod/session_indexer.py` and
  `python3 -m unittest session_indexer_test.py` (4 tests covering not-ready,
  agreement, disagreement/failure recovery and lagging heads) passed. The live
  epoch-230 store still reports `seed_bytes=0 candidates=0`, so no lottery-ready
  claim exists. Also,
  `GOFLAGS=-modfile=go.local.mod go test -count=1 -run
  'Test(SavedSessionAnchors|SessionBundle)' ./...` and the full
  `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod` passed
  (the latter in 40.052 s). A
  Linux/amd64 CLI built with the locally cached Go 1.26.8 image and offline
  module cache is
  SHA-256 `2f7a09d501a0bfb81af884d4517f66da4e1341051ffd03502229a02ec48ceacc`;
  the installed watcher is SHA-256
  `803057d8ac0ad230bd8425a595d3e52a01656bcfe843d2c1bf00f4f8320e5ee5`.
- Historical replay: the watcher used the retained epoch-228 datadir to save
  inventory height 11,372,565 and verdict height 11,372,566 separately, then
  rebuilt a session bundle after releasing the datadir snapshot. Its author,
  epoch, five CIDs and both anchors exactly matched the earlier real bundle.
  Deliberately refusing extraction with a nonexistent author left the private
  boundary snapshot available; retry with the actual author succeeded and
  removed it.
- Live status: `/etc/systemd/system/fod-session-indexer.service` is enabled
  and active on the existing test host, watching IdenaAI 2 and 3 loopback RPCs
  without restarting either node. A one-shot live check armed epoch 230;
  `watch.json` is mode 0600, its directory mode 0700, and the live archive was
  12 KiB with no retained full snapshot. It has no epoch-230 lottery or boundary
  capture yet. Both nodes remain on one host, and an RPC
  root comparison is not independent light-client authentication. The watcher
  compares their hash and root at their highest common height, so ordinary
  head lag does not stop it; a one-shot live check passed after this change.
  The service's `InaccessiblePaths` denies both node keystore directories;
  a check from its mount namespace reported both inaccessible while the
  service remained active. It has only `CAP_DAC_READ_SEARCH` to read the two
  owner-only databases and loopback API credentials; no write or mount
  capability is granted. The credentials stay in memory.
  No identity keys, contract calls, transactions, funding or mainnet purchases
  were used.
- Limitation: this prevents the previous pruning loss if the next capture
  succeeds, but does not recover epoch 229, guarantee content serving, provide
  an API-only funded buyer route, or qualify three-block fair settlement.

### 2026-10-04 — epoch-229 capture failure and bounded verification

The one-time service first attempted extraction at `15:34:48Z`, after both
nodes reported epoch 230. `capture.py` calculated the verdict as
`dna_epoch.startBlock - 1` (`11,394,352`) and tried earlier inventory versions.
The post-validation anchor is the opening block itself (`11,394,353`): the
prior real epoch-228 bundle likewise uses adjacent inventory and opening-block
verdict heights `11,372,565` and `11,372,566`. Every attempt refused with
`one-or-more-authors-have-no-valid-bundle`; there is no `done.json` or
epoch-229 session bundle. The service was disabled and stopped after diagnosis,
without restarting either node.

The disabled capture script now uses `startBlock` as the verdict height and
retains its first boundary state snapshot if extraction fails, instead of
deleting the only recoverable versions on each retry. Its installed and local
SHA-256 is
`50db85c398ca982db5dcb589050a4684b8003aa3659c0c773c676f1d5436c409`.
Against the retained real epoch-228 state database, the corrected adjacent
inventory/verdict pair produced the five-CID bundle; the former one-block-early
pair refused. A disposable check confirmed that a failed attempt's boundary
snapshot is retained and reused. These repairs cannot recreate epoch-229
proofs after the live nodes have pruned them; the service remains disabled.

Read-only snapshots of both live state databases listed 100 retained versions,
oldest `11,394,824`, newest `11,394,923` when checked. Neither the necessary
inventory version nor the verdict version remains; both nodes agreed on the
opening block's header hash and root. A node snapshot at `11,394,095` predates
the ceremony and does not directly supply the final inventory or verdict
proofs. Historical replay or another authenticated archival source is required
before claiming an epoch-229 bundle; the cached ciphertext cannot substitute
for a chain-authenticated inventory manifest.

The existing private prefetch was copied into ignored, mode-0700
`output/capture/epoch-229/encrypted-blobs/`, with only the mode-0600
`encrypted-blob-status.json` metadata; no node key, API key, epoch secret or
author address file was copied. Metadata SHA-256:
`13ae6f226e01cb144bfd2e7d475fffd702533e99cf81813182c5c1bb9c18c150`.
Local hash and deterministic CID checks passed for five IdenaAI 2 ciphertexts
(1,700,036 bytes) and three IdenaAI 3 ciphertexts (966,277 bytes); one-byte
mutations of every blob failed CID verification. Focused
`GOFLAGS=-modfile=go.local.mod go test -count=1 -run
'Test(SessionBundle|UnixFSCID|CIDVerification)' ./...` in `tools/fod` passed,
and `cargo test --locked --offline` in `proofs/session-core` passed 19 tests.
These checks establish byte integrity and the fixture verifier behavior only.
They do not establish an authenticated epoch-229 inventory or qualify
registration, sale, purchase or funded mainnet testing for these two authors.
The three-block fair-settlement goal remains open.

### 2026-10-03 — epoch-229 two-author passive capture armed

At the checked head, both local node RPCs reported epoch 229, the same next
validation time (`2026-10-04T15:00:00Z`), and `syncing=false`. `dna_identity`
reported five flips for IdenaAI 2 (Human) and three for IdenaAI 3 (Newbie).
The author addresses are discovered from each node at runtime and retained
only in a mode-0600 host file. The two nodes are on one host; this is not
independent-operator availability evidence.

The one-time `fod-session-229-capture.service` is enabled and active with zero
restarts. It polls for epoch 230, briefly hard-links immutable tables and
copies LevelDB metadata into a private temporary directory, invokes
`fod session-bundle` for each author without `-blob-out`, removes the snapshot,
then checks both anchor roots and header hashes through both nodes. The
installed script and unit match ignored local copies under
`output/capture/epoch-229/`: SHA-256
`e2b7a79dfdf12bf25cbaf585d26e890789890f13bcc80e7d11a496b958a72e50`
and `137d0345c2499e05369f33ccb4d3d2f946634f9e69af28df5c27cfcf9ada8f54`.
The script, address file, status and output directory are mode 0600/0700.
The service reads each local RPC credential in memory, permits only loopback
network access, logs no addresses or RPC/CLI responses, and neither reads the
node key nor derives epoch secrets. No transaction or node restart was made.

Focused checks: `python3 capture.py --preflight` opened a short-lived snapshot
with 1,840 linked tables and three copied metadata files; the same preflight
passed under the installed service restrictions. A retained epoch-228 bundle
was re-extracted by the capture code from its saved state database; its five
CIDs matched and both nodes confirmed its header roots. The local CID importer
matched five
pinned node vectors and refused one-byte mutations. Under the service
restrictions, `python3 capture.py --prefetch-encrypted` retained all five
encrypted blobs for IdenaAI 2 (1,700,036 bytes) and all three for IdenaAI 3
(966,277 bytes); every blob reproduced its recorded raw/UnixFS CID locally.
All eight files are mode 0600 inside mode-0700 directories. These are opaque
encrypted bytes, not decrypted flips or keys. At the epoch boundary, the
captured manifests must still match this inventory; a missing or changed CID
will be reported, not assumed valid. The session bundles and their final
two-node anchor checks have not happened yet.

### 2026-10-03 — unfunded mainnet module deployment, owner authorization

The release WASM was rebuilt byte-for-byte from the then-current 376-file dirty
source snapshot (`7a07d50a7ac6fb3b6c3c28d4c760aee75a3c7c51e923286a93dc6815e1c915a0`),
and the release export and escrow test checks passed. Its SHA-256 is
`ac97c56ce63628687c4a93e2da5337cea5067b60da1155340e2c17642fbb3e14`.
An owner-designated, synchronized mainnet node estimated and then signed one
zero-deposit deployment. The included receipt succeeded, used 30,000 gas and
charged 350.602678571372475 iDNA total. A read-only `accounting` estimate on
the resulting contract succeeded and reported zero reserve, pending payment,
credit, inventory and liquid balance. The API key and wallet key remained on
the node host; no key or flip content was supplied as a deployment argument.
Targeted scans found no known private key, API key, local path or identity label
in the published WASM. Chain observers can see the deployment code and sender.

The receipt and exact pre-documentation source export are retained under
ignored `output/mainnet/` with private local permissions. Transaction, sender,
contract and host identifiers are omitted from tracked files to avoid linking
the test identity to this repository. No registration, offer, purchase, pool
funding or seller transaction was submitted. This evidence does not qualify
timely delivery, fair settlement, content availability or the utility; paid
supply remains disabled. The contract cannot be terminated.

Same-day follow-up: two synchronized mainnet nodes on one host independently
reported the same deployed code, inclusion block and empty accounting state.
Read-only simulations from their two distinct identities succeeded for
`accounting` and refused empty/unregistered `buy`, `donateUnit`, `poolView`,
`orderPoolView`, `withdraw`, `withdrawReserve` and disabled `sellUnit` calls.
Both nodes refused a simulated termination request. The already-public
deployer then sent one zero-value `accounting` transaction with empty arguments;
both nodes agreed on its included successful receipt and unchanged zero
liabilities. Its receipt records 1,120 gas and a combined base fee and gas cost
of 0.208928571428538 iDNA. The second identity broadcast nothing, avoiding a
new public link to this contract. Exact transaction and node evidence is in the
private, ignored `output/mainnet/two-identity-mainnet-check.json`. These probes
do not test valid registration, funded purchase, delivery or fair settlement;
two nodes on one host do not prove independent-operator availability.

Same-day buying/selling prerequisite check (read-only): both node-local RPCs
reported epoch 229, synchronized head 11,389,948 and next validation
2026-10-04T15:00:00Z. Each identity currently reports authored flips (five
for IdenaAI 2, three for IdenaAI 3), but these are not a completed-epoch
eligibility verdict. `contract_readData` returned zero global reserve, pending
escrow, units and credits; `contract_iterateMap` returned no `pool/` bundles or
`porder/` orders on either node. The sole epoch-228 captured session bundle's
author matches neither identity; a later read-only comparison against the
live IdenaAI 1 node also refused a match, while `dna_identity` reports the
captured author as a Human. The capture host notes call it the exercise seller.
The checked sealed registration receipts do not bind the deployed contract.
No valid inventory can therefore be registered from the current evidence. With
`sellUnit` disabled by the approved no-subsidy policy, the supported author
route is registration followed by
author-only `donateUnit`; a positive buy quote also needs voluntarily supplied
global reserve and at least two total units. No registration, reserve deposit,
sale, purchase or key disclosure was broadcast in this check. The identities
and key material remained on the node host; a mainnet exchange and its
privacy linkage remain untested.

Same-day short-window feasibility check: a focused harness run of
`TestPoolRevealRetainsKeysOnlyWithSuccessfulSettlement`,
`TestPoolRevealSettlesWithoutTheBuyer` and
`TestUnincludedDeliveryExposureCanPrecedeRefund` passed against the current
module. A read-only sample of 20 consecutive mainnet block intervals ending at
11,390,007 had a 20-second median; the sampled three-block spans were 56-60
seconds. These are observations, not a wall-clock or transaction-inclusion
guarantee. The cheap `reveal` requires no post-buy proof, but its prepared
calldata exposes the keys before the transaction is included; a dropped call
followed by the N+3 refund is therefore still an adversarial gap. No secrets,
transaction or payment were submitted for this measurement.

### 2026-10-03 — exposed application key rotated and old local trust retired

Owner authorization: "dont ask, decide and do it", following the specific
application-key exposure and proposed rotation. Rechecked the gateway config,
key loader, browser pin verifier and utility signature/access paths. No
repository gateway/utility process was running; ports 8787/8788 had no listener.
No non-ignored source file referred to the former public pin. Generic example
configs remain portable; no historical reference snapshot was changed.

Published a complete fresh Ed25519 application key at
`gateway/provider-application-key.json`, mode 0600, after privately preserving
the exposed file and staging the replacement. The parent directory was synced;
the published key reloads and matches its new pin. The retired key is quarantined
with mode 0600 in an ignored directory with mode 0700, never a trusted rollback.
Private values were read only in-process and were not printed or copied to
source, logs or public evidence.

Local outputs under `output/security/application-key-rotation-9zs2HT/`:
- `connection.json`: browser connection pinned to the replacement key.
- `utility-policy.json`: utility provider pin and actual configured historical
  session digest/window; no new session or access promise was invented.
- `rotation-evidence.json`: public pins, paths, permissions and measured checks.
- `retired-provider-application-key.key`: private quarantine; do not distribute.

Verification executed in a one-shot local Node module using
`loadOrCreateApplicationKey`, `loadConfig`, `loadPool`, `loadPolicy`,
`parseProviderConfig`, `verifyResultSignature`, `verifyPinnedProviderResult`
and `decideAccess`. The locally signed test result is explicitly synthetic and
cannot grant a benefit. Results: new key verifies in browser and utility; old
signatures refuse in both; an old served pin refuses under the new connection;
the utility returns `bad_signature` for the old-key access attempt. Key and
quarantine-file modes are 0600; quarantine-directory mode is 0700.

Limits: external copies of the old pin cannot be rewritten from this checkout;
their holders must import the new connection before reuse. The current example
session is historical; its unchanged window does not authorize a fresh live
gate. Old conversation/tool output cannot be erased by key rotation. No wallet
key, existing node, remote host, deployment or transaction was used. The MVP
remains unfinished. After this entry, rebuild the manifest; its source inventory
and the private rotation verification log record the final result.

### 2026-10-03 — SA-01..SA-16 — bounded local security audit and repairs

Revision: `1462782b8f15b6c8451a71e010b9a31143168793`, dirty on entry and
preserved. Authorized window began 2026-10-02 23:33:28 UTC, maximum end
2026-10-03 05:33:28 UTC. Direct audit and implementation with the selected
GPT-6.1 Sol model; no additional agents, threads or automations.

Files and findings:
- Utility: `src/claims.ts`, `commitments.ts`, `access.ts`, `server.ts`,
  `score.ts` and `test/security.test.ts`: SA-01/02/03/14. Nonces survive
  simultaneous signature checks only once; replacement commitments obey their
  UTF-8 byte budget; private atomic grant persistence defeats a planted staging
  symlink; live limiter windows are retained at capacity. Corrected comments
  that overstated protection against a cooperating provider.
- Gateway: `src/keys.ts`, `http.ts`, `test/keys.test.ts` and
  `test/gateway.test.ts`: SA-05/14. Concurrent starts publish one complete
  private key; loading refuses symlinks, wrong providers and inconsistent
  key metadata; limiter capacity cannot reset an exhausted live allowance.
- Browser: `src/provider.ts`, `score.ts`, `views/Session.tsx`,
  `views/SharedNode.tsx`, their focused tests and `verify.mjs`:
  SA-06/07/08/09/16. Challenge images accept supported inline raster bytes only;
  replaceable JSON responses are bounded before buffering, with no credentials
  or redirects. Wallet signing checks the exact expected context and validity.
  Transcripts cover every pinned challenge. Malformed shared-node status is
  refused without crashing.
- CLI: `export.go`, `action_request.go`, `inventory.go`,
  `indexer_inventory.go`, `rpc.go`, `submit.go` and export/local-safety/
  RPC/reveal/submit regression tests: SA-04/10/11/12/13. Export rehashes CIDs,
  binds blob/vault/inventory authors and bounds blob reads; delivery keys publish
  exclusively; cache writes cannot follow destination symlinks. RPC redirects
  and non-success HTTP results refuse. Reveal requires private output before
  loading a key, and signed outputs publish atomically with mode 0600.
  Synthetic export/shared-node/receive fixtures now use their real computed CID.
- Release: `source-snapshot.mjs`, `source-snapshot.test.mjs`,
  `check-integration.mjs`, `service-isolation.test.mjs`: SA-15/16.
  Known runtime key payloads and private PEM are refused irrespective of
  filename/suffix. Verification requires readiness from its own spawned child
  and refuses before contacting an occupied endpoint. It does not terminate the
  existing process. This is a known-payload guard, not a universal secret scanner.

Focused commands/results, after the coherent changes:
```sh
(cd utility && npm test && npm run typecheck)
(cd gateway && npm test && npm run typecheck)
(cd app && npm test && npm run typecheck && npm run build)
(cd tools/fod && GOFLAGS=-modfile=go.local.mod GOPROXY=off FOD_LIVE_NODE= FOD_LIVE_INDEXER= FOD_REGENERATE_SESSION_VECTOR= go test -count=1 ./...)
(cd spikes/harness && GOFLAGS=-modfile=go.local.mod GOPROXY=off go test -count=1 ./...)
(cd contracts/escrow && cargo test --locked --offline)
(cd proofs/session-core && cargo test --locked --offline)
node --test release/source-snapshot.test.mjs release/service-isolation.test.mjs
node release/check-integration.mjs
git diff --check
```
All passed: utility 44 tests; gateway 40; browser 19; release regressions 3;
CLI suite 38.527 seconds; harness 11.811 seconds; escrow 16 and session-core
19 Rust tests. The integration script includes the offline browser walk and
gateway/utility journey. Its final run exited 0 with ALL CHECKS PASSED.
The separate skeptical bypass pass found custom uppercase/extensionless key
filenames and occupied-port verification; both have negative regressions.
Unchanged Rust/harness checks were not repeatedly rerun. Later utility comment
edits do not change executable behavior.

Evidence is persisted outside source in the native scan directory:
`1462782b8f15b6c8451a71e010b9a31143168793_20261002T233402Z_tm15q6wq`
under `<internal-state-path>`.
It holds baseline sources, final verification logs and the finding details.
No value from the inadvertent credential exposure described in the handover
below was copied to those artifacts.

Limitations: synthetic keys/local fixtures only; no fresh chain receipts,
measured delivery latency/cost or independent-operator availability. Existing
S8/S11 fair-exchange, canonical availability, authenticated lightweight
purchasing and H-3 actual utility enforcement remain open. The reported CID
fallback remains locally closed; it was not reintroduced or counted as a new
finding. Paid supply is still disabled. No remote access, real-wallet use,
deployment, funding, transaction broadcast, commit or push occurred.

Next: refresh the candidate via `node release/build-manifest.mjs` after this
entry. At this entry's timestamp that gate has not yet run; the final manifest
and native scan's release log will record success, or its exact refusal will be
retained. Do not infer qualification from a manifest rebuild. Existing MVP goal
is still blocked and was not falsely completed.



### 2026-10-02 - S11 lab step 2: the listing is live; the lab mempool wedges on out-of-order nonces

- What landed on the isolated lab (`127.0.0.1:19095`, network 33411) after
  restarting the wedged proposer: `registerBundle` of the epoch-74 witness
  (pool key `c77e6862…`, manifest `8496f081…`, 1 flip) is on chain - the
  contract reads `1 listings` from block 13,239; the pending `addReserve`
  control call (1 iDNA) also landed; two author-only `donateUnit` transactions
  took the listing to `units 2`, `sellQuote 333333333333333333` (0.333 iDNA),
  `strikes 0`, `0 orders` at block 13,318. The buyer's own node fetched and
  rebuilt the flip through its RPC (`checked 1/1 flips fetched and rebuilt by
  this node ... the advisory's CIDs match the bundle's committed inventory`)
  and produced a valid 200-byte `buy` calldata.
- Blocker (same class, mechanism now identified): every non-ceremonial
  transaction must carry the epoch's exact next account nonce. A transaction
  with a skipped or already-used nonce is accepted into the mempool and then
  never included, and the pool keeps it: `dna_getBalance` for the god account
  reports state `nonce 1` and `mempoolNonce 7` while the proposer keeps
  proposing `txs=0`. Evidence: the first `donateUnit` (nonce 3) was skipped
  because the 1 iDNA funding send had already taken nonce 3; the retry with
  nonce 4 landed; in epoch 76 the pool again holds stale entries (a donate
  with nonce 5, the 1,000 iDNA funding `0xe9e1a406…`) and neither they nor a
  fresh tx land. Transactions are also refused outright during the ceremony
  periods (`tx can't be accepted due to validation ceremony`).
- Working procedure proven so far: stop `fod-v3-cer`, clear
  `datadir/mempool-txs/txs.json`, restart the node **in a foreground session**
  (a detached `nohup` process does not survive the shell), then send one
  transaction at a time with the exact next nonce (`state nonce + 1` for the
  epoch) and wait for inclusion before the next. Detached processes aside,
  the restart cleared the pool once; the second wedge shows it is not a
  complete repair.
- Files: `output/lab/witness-epoch74-*.json` (witness, receipt, prepared
  action, advisory), `/tmp/fod-capture-v7/market-final.json` (the current
  market snapshot), `/tmp/fod-capture-v7/prepared-buy.json` (a buy prepared at
  an older quote; re-prepare it at the current quote before submitting). No
  source changes; no mainnet; no real funds.
- Next task: with the pool cleared and one tx at a time - fund the buyer with
  the epoch's first nonce, then submit a freshly prepared buy, let the desk
  deliver inside the window, verify the buyer's `receive`, and confirm; then
  run the no-confirm variant so the desk settles by reveal. Record inclusion
  latency and gas for deliver, confirm and reveal. Paid supply stays
  disabled; the delivery-v2 proof remains unqualified for the three-block
  window.

### 2026-10-02 - S11 lab step: epoch-74 witness and proof, and the lab proposer that stopped including transactions

- Scope: build a fresh, contract-bound registration witness on the isolated
  play-money lab (`127.0.0.1:19095`, network 33411) and register it on the
  module deployed on 2026-10-01 (`0xd53b3032…`). Local lab only; no mainnet,
  real keys, real funds or remote hosts.
- Valid flip. The first two lab flips used synthetic 64-byte parts and the
  registration guest refused them (`a part has an unsupported shape:
  Malformed`), because the frozen statement requires the decrypted parts to be
  hex text that `flip_core` can read. A third flip for epoch 74 was submitted
  with the plaintext parts of the committed vector
  (`proofs/registration/vectors/blob-flip-v1.json`): tx
  `0x187a5dc2411362470682548c5c745b03b9c64877dd966874d5a9b8333fee4b03`,
  CID `bafkreih2qukzk3pnwhynpvai7yquzthznvmvappqt6idpntt6ppa2qp6x4`,
  included in block `0x99f6647e…` at epoch 74.
- Witness and proof. The epoch-74 flip lottery opened at head 13,088
  (02:25:43) and the epoch-75 boundary (ValidationFinished) at head 13,107
  (02:32:14). A read-only datadir copy captured
  the session bundle and the blob witness (epoch 74, 1 flip, inventory 13,088
  -> verdict 13,107); the durable copies are
  `output/lab/witness-epoch74-session.json` and
  `output/lab/witness-epoch74-blob.json` (0600, the blob witness carries the
  epoch secrets). The registration host verified the
  witness natively for the new contract (`journal 326 bytes, epoch 74, 1
  flips`) and then produced a genuine Groth16 receipt: 8,388,608 cycles in 8
  segments, image id
  `07a72c7bfa698540eaeeabc9294b47d2f827becc9acd2266d43b364f0ad35631`,
  claim digest
  `ed72db3b2b0e694d71540c747b269c4b8ed577999fb1f58327625bffd37934e8`,
  artefact `output/lab/witness-epoch74-registration.json`. `fod
  prepare-registration` turned it into the pool key
  `c77e6862f05d14ffef9e77caf1f5ee7000feacf83dae0b930273876614de30f4`
  (manifest `8496f081…`, calldata 582 bytes). The node's own
  `contract_estimateCall` for that call answers `success true`, `gasUsed`
  1,961,616 - the contract accepts the proof and the arguments.
- Blocker (opened 2026-10-02, first occurrence): the lab stops including
  transactions. The signed `registerBundle` transaction stays pending on both
  nodes at every fee tried (`0xc5a628ac…` at 19,645.52 iDNA, `0x2ba6cd9b…` at
  40,000, `0x11d72529…` at 50,000; `bcn_transaction` reports a zero block
  hash, `dna_getBalance` reports nonce 0 / mempoolNonce 1, `bcn_sendRawTx`
  answers `tx with same hash already exists`). The sole proposer
  (`fod-v3-cer`, the god account) logs `Proposed block … txs=0` for every
  block since at least 02:50, and a deliberately tiny control call
  (`addReserve`, 182-byte transaction `0x6a5983c7…`) also stayed pending
  across about ten blocks. The chain therefore produces only empty blocks
  while valid pending transactions exist in both mempools; this is the
  `H-6`-class silent proposer behaviour, not a contract, proof or fee
  refusal, and the node logs no drop reason.
- Next task: restart the lab proposer node
  (`/tmp/idena-go-pinned --config …/fod-v3-cer/config.json --datadir
  …/fod-v3-cer/datadir`; the pending transactions persist in
  `datadir/mempool-txs/txs.json`) or repair its mempool/proposer state, then
  confirm `registerBundle` lands and continue the lifecycle
  (donateUnit, addReserve, buy, desk deliver, buyer receive/confirm, and the
  no-confirm reveal) with inclusion latency and gas recorded for each call.
  The witness and the proof above are already valid for that contract and do
  not need regeneration. Paid supply stays disabled; the delivery-v2 proof
  remains unqualified for the three-block window.

### 2026-10-01 - S11 lab step: the promoted module is deployed on the isolated chain

- Scope: deploy the promoted release module to the isolated play-money lab so
  the faster settlement design can be qualified on chain. Local lab only
  (node `127.0.0.1:19097`, network 33411, generated keys, play money); no
  mainnet, real keys, real funds or remote hosts.
- Commands/results: `fod prepare-deploy -code
  release/artifacts/fod_escrow.wasm -nodekey <lab god key> -nonce 1 -tx-epoch
  72 -max-fee 42000000000000000000000 -rpc http://127.0.0.1:19097 -network
  33411 -broadcast` derived and sent contract
  `0xd53b3032139c33ab87bd9bac7f0871658805c60f` from the promoted module
  (388,019 bytes, sha256 `418334c4…`; derived code hash
  `0371a877a4f9ec3f3a810c5f7807b097e8c2b3e22f5148e75a0f449b295d5434`, raw tx
  388,184 bytes, sender `19e7e376…`, nonce 1, epoch 72, amount 0).
  `bcn_txReceipt 0xff188dd425f11209ccb799d61c94adf67089515fca269295a90d9e933483fc65`
  reports `deploy`, `success true`, `gasUsed 30000`, `gasCost 300`,
  `txFee 38818.4`. The transaction sits in block `0xeda62dba…` at epoch 72
  (`usedFee 38818.4`, `maxFee 42000`, `tips 0`), and the fresh contract's own
  state read (`fod market`, head 12,625) returns the same code hash with
  `0 listings (0 orders), 0 credits`, state digest
  `dfe0ac1d5dedc2963c2d726bf2d44f0bdc2eae3f1817fba2c0ef85d9d324b67b`. The
  receipt and the prepared deploy are saved under `output/lab/`
  (`current-module-deploy-receipt.json`, `current-module-deploy.json`).
- Limit: this proves the promoted module deploys, runs and answers on the
  isolated chain; it is not a marketplace receipt. The fresh contract has no
  registered listing, and the epoch-1 witness cannot be rebuilt: read-only
  copies of both lab datadirs answer `session bundle failed: load the state at
  height 198: wanted to load target 198 but only found up to 0`
  (`/tmp/fod-witness-cer2/datadir`, `/tmp/fod-witness-lab3/datadir`; the
  running nodes hold their DB and the state behind the old receipt is pruned).
  Only the receipt `/tmp/fod-v4/registration-v4.json` (journal heights 198 and
  222, manifest `881b07ea…`) and `prepared-register.json` survive, and both are
  bound to the old contract `0x8a225b8e…`. A listing on `0xd53b3032…`
  therefore needs a fresh capture on this chain: submit flips in an epoch's
  FlipLottery window, let that epoch's validation ceremony finish, copy the
  datadir before pruning, then `fod session-bundle … -blob-out …` and a
  contract-bound registration proof (about 17 minutes on this host). At
  2026-10-01T23:57+02:00 the lab was at epoch 72, period `None`, next validation
  2026-10-02T00:26:12+02:00, so the next flip-submission window opens when epoch
  73 starts. The earlier v4 deployment (`0x8a225b8e…`, code hash `54caa028…`)
  is untouched and its listing has no units. Paid supply stays disabled and the
  delivery-v2 proof remains unqualified for the three-block window.
- Next task: run that capture pipeline in the next epoch window, prove and
  register the bundle against `0xd53b3032…`, then run the deliver to confirm and
  the no-confirm to reveal boundary with the current desk and record inclusion
  latency and gas for each call.

### 2026-10-01 - S7/S11 release coherence: both artefacts rebuild to the recorded bytes

- Scope: after the S10 proof-tuple and S11 escrow/desk changes, re-establish
  that the promoted release artefacts are the current source's bytes. Local
  build and verification only; no deployment, transaction, commit or push.
- Commands/results: `node release/build-manifest.mjs` rebuilt the escrow module
  (`cargo build --locked --offline --release --target wasm32-unknown-unknown`
  plus the pinned `wasm-opt --llvm-memory-copy-fill-lowering -O3` lowering) and
  the interface from `app/`, validated, then promoted.
  `release/artifacts/fod_escrow.wasm` is 388,019 bytes, sha256
  `418334c42473c8e6b0442b48ad5982cd9f634b844696f9ac792384e603f8ca32` -
  byte-identical to the artefact already in the tree, so the current contract
  source reproduces the promoted module. `app/dist/flips-on-demand.html` is
  302,139 bytes, sha256
  `a67c7903dbd44bd9b57f6f7a50e6ae1f600d9aeee6d7802b4eaf5177d198618c`, the same
  hash the S9 and security-follow-up snapshots recorded. Checks: 19 published
  escrow exports, no test hooks, 0 gatekeeper-forbidden operators, 0 remote
  interface tags. `node release/check-integration.mjs` passed every check - the
  offline interface walk and the session-to-utility journey (answers accepted
  without a correctness signal while the session is open, the utility scoring
  the committed answers itself, a tampered result refused, the quota spent
  once).
- Files: `release/MANIFEST.json` regenerated for this revision (374 source
  paths) and this record. No contract, guest, proof or interface input changed
  in this package.
- Limitation: the manifest still identifies a dirty worktree at revision
  `1462782b8f15b6c8451a71e010b9a31143168793`; a HEAD clone cannot reproduce it
  without the source snapshot. No cold-cache or cross-platform qualification
  and no fresh chain receipt were produced. The delivery-v2 proof remains
  unqualified for the three-block window; paid supply stays disabled.
- Next task: deploy this promoted module on the isolated lab and produce the
  contract-bound registration receipt plus fresh deliver/confirm/reveal
  receipts with their measured inclusion latency and gas.

### 2026-10-01 - S11: the desk must not hand out keys it can no longer settle

- Finding: the desk's plan could publish a buyer-bound delivery at a head where
  the committed secrets could no longer be broadcast and included before the
  refund boundary (`reveal.go` refuses once the head reaches `deadline-1`).
  That is the documented counterexample reached through the desk's own
  planning: the buyer decrypts the delivery and still refunds at the boundary.
  The same plan proposed reveals that its own signing guard refuses, because the
  plan and the guard each carried their own copy of the bound.
- Change: `tools/fod/serve.go` derives one bound, `lastDisclosureHead(deadline)
  = deadline-2`, and applies it to every key-disclosure action. A funded order
  is delivered only while a later pass can still reveal (`head < last`), is
  settled by the reveal itself at `head == last`, and is left to the buyer's
  repayment past it. A delivered order reveals inside the margin only at
  `head <= last`. A stranger's delivery is replaced while there is room and
  settled by the reveal afterwards. The proof fallback is planned only while a
  settlement can still land before the boundary. `tools/fod/reveal.go` now
  calls the same helper before signing, so plan and guard cannot drift apart.
  No contract, guest, proof or wire format changed.
- Files: `tools/fod/serve.go`, `tools/fod/serve_test.go`, `tools/fod/reveal.go`,
  this record and `docs/SPEC.md`; dirty HEAD
  `1462782b8f15b6c8451a71e010b9a31143168793`, all existing changes preserved.
- Commands/results: `cd tools/fod && GOFLAGS=-modfile=go.local.mod GOPROXY=off
  go test -count=1 ./...` passed (28.866 s) after every edit; the focused
  `-run 'TestDisclosure|TestDeskRechecks|TestSettledReveal|TestRevealBudget|TestServe'`
  batch passed (6.212 s). `TestServePlanDisclosesKeysOnlyWhileSettlementStillFits`
  pins the boundary table at head 10,000 with `serveDefaultReveal = 2`: funded
  deadlines `head+3`, `head+2`, `head+1` plan deliver, reveal, nothing;
  delivered orders stay quiet at `head+4`, reveal at `head+2`, do nothing at
  `head+1`; a stranger's delivery is replaced at `head+3`, revealed at
  `head+2`, and left alone at `head+1`. `gofmt -l` on the three files and
  `git diff --check` are clean.
- Limitation: this is desk policy, not a chain guarantee. A reveal transaction
  that stalls or is dropped, a dishonest RPC and pending-data visibility remain
  open, so a buyer who reads a pending reveal can still race the refund at the
  boundary; the contract cannot see the mempool. The delivery-v2 proof remains
  unqualified for the three-block window and no new chain receipt was produced.
  Paid supply stays disabled.
- Next task: qualify the same boundary on the isolated lab with the current
  module and a contract-bound registration receipt, and measure post-inclusion
  latency and gas for deliver, confirm and reveal. Independent-operator
  availability and an authenticated lightweight state route remain open.

### 2026-10-01 - S11: shared-key reveal must close every exposed order

- Revision `1462782b8f15b6c8451a71e010b9a31143168793`, dirty/uncommitted.
  Files in this containment: `contracts/escrow/src/lib.rs`,
  `contracts/escrow/README.md`, `spikes/harness/escrow_instant_test.go`, the
  existing D0 harness fixture, `tools/fod/{registry,pool_buy,reveal}.go` and
  focused CLI regressions, plus this record and `docs/SPEC.md`.
- Defect reproduced in the pinned runtime: two funded orders on one bundle,
  followed by one successful public reveal, previously paid only the named
  order; the second buyer knew the same keys and could claim a full refund.
  The contract now tracks at most eight open orders per bundle and atomically
  settles all of them, preserving `preveal/<id>` for each. It preflights
  protected reserve, pending escrow and host-credit solvency before transfers.
  Any expired sibling or earlier refund refuses public reveal before key
  publication. The CLI checks all sibling deadlines and the refund history
  before signing, but an untrusted RPC cannot replace canonical enforcement.
- `cd contracts/escrow && cargo test --locked --offline`: 16 passed. The
  test-hook WASM built with `cargo build --locked --offline --release --target
  wasm32-unknown-unknown --features test-hooks` and the pinned `wasm-opt
  --llvm-memory-copy-fill-lowering -O3` lowering. `cd spikes/harness &&
  GOFLAGS=-modfile=go.local.mod GOPROXY=off go test -count=1 ./...`: pass.
  `cd tools/fod && GOFLAGS=-modfile=go.local.mod GOPROXY=off go test -count=1
  ./...`: pass. `git diff --check`: pass.
- The runtime regressions cover two concurrent buyers, a ninth-order refusal,
  128 host-fee entries, an expired sibling, a refunded sibling, failed payout
  rollback, retained keys for each settled order and refused later refunds.
  The eight-order reveal measured 1,483,236 WASM gas in the local fixture,
  which is neither chain transaction cost nor delivery latency. The
  unincluded-disclosure/refund negative control still passes: two-sided fair
  exchange, canonical ciphertext availability and the three-block user
  journey remain unqualified. Paid supply stays disabled; no deployment,
  transaction, remote node, real key, commit or push occurred.

### 2026-10-01 - S10 / H-10: reproducible guest build and coordinated proof tuple

- Revision `1462782b8f15b6c8451a71e010b9a31143168793`, dirty/uncommitted.
  Baseline source inventory `86b3e735...14727`; checked code snapshot
  `6fc28e69...869c5`. All 354 source files outside this repair are byte-for-byte
  preserved (16 existing files changed, two build/verification files added).
  `output/verification/reproducible-proof-build-2026-10-01/preservation.json`
  carries the exact list. Final record-only changes refresh manifest provenance.
- Files: `proofs/guest-build.rs`, both active methods' build scripts/manifests,
  both proof JSON artifacts, the registration sealed-artifact test,
  `contracts/escrow/src/verifier.rs`, CLI claim pins/tests, release build/gate,
  affected existing READMEs and SPEC 10.6. No guest statement source changed.
  The rebuilt local CLI also carries both new pins and runs `-h` successfully.
- Both guests compile at `/src` using
  `risczero/risc0-guest-builder:r0.1.91.1@sha256:fafb377a44e1cfca415577c48d2f7012bda99ed36f2fae27f9a663b9fe6048f0`,
  compiler `rustc 1.91.1-dev (30e2186d0 2025-11-25)`, SDK exactly 3.0.6 and
  unchanged guest lockfiles. Docker receives only manifests, locks and Rust
  compile inputs (8 delivery files, 19 registration files), with no captures,
  order witnesses, host fixtures or vaults. Symlinks refuse. Nonempty skip,
  debug and builder overrides refuse; skip/tag refusals were exercised (101).
- Concrete development refusals: the SDK-default 1.88 builder cannot compile
  locked `enum-ordinalize` 4.4.2 (MSRV 1.89) or `ruint` 1.20.1 (MSRV 1.90).
  The source-only staging workspace needed an explicit root to avoid discovering
  the caller's workspace above Cargo's output directory. Both were corrected;
  no dependency downgrade or proof-verification bypass was used. The fresh
  delivery host rejects the previous image's receipt (exit 1, invalid proof).
- Program reproduction PASS: original/relocated builds, then two additional
  fresh source and target locations in the release gate, yield identical full
  programs, not just matching image labels. Shared immutable Docker/dependency
  caches were used; this is one-machine source-location evidence.

| Program | Image ID | Program SHA-256 | Program bytes |
|---|---|---|---|
| Delivery v2 | `6bcb725eff9008b2baba7aec227bdf0342dd11e7e03a34b26a49f0a72cabd443` | `a46fd3dbb875530b5f1eb2fb9ed7edf31a935a656381615417fcdea7322753f7` | 467,544 |
| Registration v3 | `07a72c7bfa698540eaeeabc9294b47d2f827becc9acd2266d43b364f0ad35631` | `bd050a10e195a91ca61969fba0bc5f9cc18129c7127427a03f1f3c5bda44aa27` | 528,232 |

- One genuine Groth16 receipt per changed image was generated locally with
  `RISC0_PROVER=ipc RISC0_DEV_MODE=0`, with Bonsai variables removed.
  Delivery used `cargo run --locked --offline --release --bin delivery-host --
  --no-witness --out ../../output/verification/reproducible-proof-build-2026-10-01/delivery-v2.json`
  from `proofs/delivery`. Registration used `cargo run --locked --offline
  --release -p registration-v2-host -- --prove --out
  ../../output/verification/reproducible-proof-build-2026-10-01/registration-v2.json`
  from `proofs/registration-v2`; its host-relative output was moved from
  `proofs/output/verification/` to the evidence directory after generation.
  Use an absolute `--out` when reproducing to avoid that path ambiguity.
  Both new journals equal their previous journals; statements remain v2/v3.
  Delivery: 7,602,176 cycles / 8 segments / 1,839.966 s. Registration:
  8,388,608 cycles / 8 segments / 1,911.412 s. These runs were concurrent on
  one machine and are not an isolated latency comparison or a delivery bound.
- Focused commands/results PASS:
  `cargo run --locked --offline --release -p registration-v2-host -- --negative`
  refuses all five malformed witness cases;
  `cargo test --locked --offline` in `contracts/escrow` passes 16 tests,
  including a genuine previous-image seal refusing the new pin;
  `GOFLAGS=-modfile=go.local.mod go test -count=1 -run
  'Test(ClaimDigest|AssembleDelivery|AssembleRegistration|RegistrationLabelDigests)' ./...`
  in `tools/fod` passes (0.688 s).
- Runtime artifact built in an isolated `CARGO_TARGET_DIR` with
  `cargo build --locked --offline --release --target wasm32-unknown-unknown
  --features test-hooks`, then lowered with `wasm-opt
  --llvm-memory-copy-fill-lowering -O3 ... -o
  contracts/escrow/dist/fod_escrow.testhooks.wasm`. This keeps the hook variant
  apart from the release target. Harness `GOFLAGS=-modfile=go.local.mod go
  test -count=1 -v -run
  '^Test(PoolFulfillDeliveryV2DomainAndReplay|PoolSupplyFromASealedReceipt|D0RegistrationReceiptDoesNotPayItsPresenter)$' .`
  passes all three (1.028 s). Registration: 195,664,723 WASM gas; delivery:
  195,738,019 WASM gas. Wrong headers/domain/context, replay and supply payout
  refuse. These are runtime fixture measurements, not chain receipts or fees.
- `node release/build-manifest.mjs` PASS: invokes
  `node release/check-proof-builds.mjs`, compares both clean programs, verifies
  both genuine seals natively and validates all contract/CLI/artifact pins
  before promotion. Release module: 19 expected exports, no test hooks,
  zero forbidden operators. `node release/check-integration.mjs` PASS,
  including the bundled browser walk. HTML remains byte-identical (302,139
  bytes, `a67c7903...98618c`); updated release WASM is 380,630 bytes,
  SHA-256 `368d19dfa7d7ff9b122700f5a35105f4aff5b99878fd7fd1b53c7d1583da097b`.
- Exact artifact and log hashes, program comparisons, proof statistics,
  runtime results and preservation evidence:
  `output/verification/reproducible-proof-build-2026-10-01/EVIDENCE.json`,
  SHA-256 `5227734a60a361621f9a34e221765b5fc14edb56772aa79e67ea29e72712f4c1`.
  Delivery artifact SHA-256 `689d051d...0e37e`; registration `4342d263...e4f6d`.
- Limit/next task: H-10 is locally repaired for this candidate. Existing
  immutable deployments keep old pins and cannot accept the replacement seals
  through relabelling or an administrator. No deployment/broadcast, remote
  host, real wallet, funding, commit or push occurred; paid supply remains off.
  Fresh chain qualification requires separate authority. Fast private delivery,
  safe pre-inclusion disclosure, canonical content availability, participant-
  verifiable scoring/utility release and independent-operator/lightweight
  purchase evidence remain open. The desktop MVP goal remains unfinished.

### 2026-10-01 - requested proof of the four remaining acceptance gaps

- Scope/revision: verification of dirty HEAD
  `1462782b8f15b6c8451a71e010b9a31143168793`. The starting snapshot is 370
  inputs with source digest
  `1fa276de83b6f4f7be2ffb3fca79c497c7c149821f8bf5cf90167e9c4ffd441c`.
  Evidence, environment identities, binaries and the private source export
  are retained in `output/verification/acceptance-proof-2026-10-01/`.
  Neither a new proving receipt nor a chain receipt was generated. Only this
  progress record and the manifest's source inventory are updated afterward;
  application source, proof artifacts and deployed systems are unchanged.
- CLI reproduction: `node release/source-snapshot.mjs output/verification/acceptance-proof-2026-10-01/source`
  passed. Exported the immutable `idena-go` revision
  `9bc57040337fd19f45ba65323939cac8789b9efd` with `git archive`, leaving its
  dirty reference checkout untouched. Archive SHA-256 is
  `b1d1d3e55569ecbddb9461c9253c9618c2ab8f9859ffda3128f2e634ece2b83b`.
  In the exported CLI, copied `go.mod`/`go.sum` to the local modfiles and
  added `replace github.com/idena-network/idena-go => ../../../idena-go`.
  With the cached Go 1.26.8 executable, `GOTOOLCHAIN=local
  GOFLAGS=-modfile=go.local.mod GOPROXY=off GOSUMDB=off`, and separate
  `GOCACHE` directories, `go build -mod=readonly -trimpath -buildvcs=false
  -o ../../../fod-reproducible-a .` and the equivalent `b` build produced
  identical 87,813,090-byte binaries, SHA-256
  `8b53b2c933e19807354949bbfe58c26b487dbc0ecf779aa52494ee5647ce2dd4`.
  `fod-reproducible-a -h` exits 0. This proves native macOS/arm64 reproduction
  on this host with the recorded compiler/native-library prerequisites,
  not portability to a second operator's environment.
  Initial verification commands were corrected rather than counted as passes:
  disabling the checksum database during automatic toolchain selection failed;
  directly using the cached executable resolved that offline prerequisite.
  Suppressing Go's build ID caused 48 UUID/signature bytes to differ. A
  `-no_uuid` control matched but aborted in dyld, so it was rejected. Normal
  Go build-ID handling gives both identical bytes and a runnable binary.
- Focused behavior: in the exported CLI, the same offline environment and
  `go test -mod=readonly -count=1 -run 'Test(PreparePoolBuy|TheInventoryCommitment|PrepareConfirm|PoolReceive|SettledReveal)' ./...`
  passed in 6.006 s. The checks use disposable RPC fixtures; they do not
  authenticate a public RPC or establish independent-operator availability.
- Proof reproduction FAIL / H-10: fresh target directory, host Rust 1.98.1,
  guest Rust `1.97.0-dev (e638c6cfe 2026-07-15)`, SDK/r0vm 3.0.6, locked
  offline builds and local proving selected. In the exported delivery
  workspace, `cargo build --locked --offline --release --bin delivery-host`
  succeeded (2m 04s), but the rebuilt host's
  `--verify source/proofs/delivery/dist/delivery-v2.json` exited 1 with
  `verification indicates proof is invalid`. Its guest ID is
  `ce65383db9fce8c7c221ac14e72171ba7c14671dd5737bcca93d58164bc9ae92`,
  whereas the shipped receipt/contract pin is
  `60ec600b94660ae6d91ec4332eb0d0ece19e37396e72ef299678547bc95dd4ff`.
  In the exported registration workspace,
  `cargo test --locked --offline --release -p registration-v2-host --test sealed_artefact`
  built (1m 26s) but failed the sole cryptographic verification. Its clean
  guest ID is `37a6bfbd066a86efbb21db8d3506364beb79a23867b5a5e26e6342a6954d2223`,
  while the pin is `7ee79d90a7c5a3c99bb76b6eab24ae532e9a4ff1f9fc23eecb1806eb5f9d86d9`.
  No proof was regenerated for these incompatible IDs.
  A control build with a new target directory at the original delivery source
  location (1m 54s), then the same receipt verification, passed and reproduced
  the pinned image. Both delivery builds used the same guest compiler and
  flags; the local core package's path fingerprint differs. This establishes
  a build-location dependency; the exact code-generation cause needs further
  diagnosis. Existing proofs are not shown cryptographically invalid.
- Independent operators: historical section-3 evidence explicitly places both
  nodes on one host. No authorized second operator/test host is available;
  no remote access was attempted. That evidence cannot establish independent
  failure domains or project-host removal.
- Lightweight purchase: `pool_buy.go` calls `registryPoolBundle`, which reads
  `contract_iterateMap` and counters from RPC. `registryHead` reads RPC block
  responses; no purchase-time finality/header/state-membership verifier
  authenticates these replies. Historical IAVL session proofs authenticate
  different claims and are not a light-client purchase route. Criterion unmet;
  a portable pin or a second agreeing RPC does not supply the missing proof.
- Fresh chain receipts: historical v4 receipts describe their old module.
  Current source and settlement/recovery changes have not been deployed by
  this task. The explicit deployment/broadcast/remote-host restrictions remain
  in force. A local proving receipt or MinimalEnv call is not a chain receipt.
- Next task: close H-10 with a pinned, reproducible guest build; verify identical
  images from two source locations, then replace the entire proof tuple once
  and add a clean-build release gate. Implement the authenticated lightweight
  purchase route and obtain a designated independent operator plus explicit
  authority for an isolated current-module chain exercise. Paid supply remains
  disabled. The MVP is not complete.

### 2026-10-01 - S9 settled-key recovery and disclosure containment

- Findings: H-7 / SEC-2 remain open at protocol level. Fixed a concrete
  recovery defect: reveal could settle without retaining a readable key
  preimage, while the buyer only tried `pdeliv`. The contract now writes
  `preveal/<id>` atomically with settlement. CLI receive verifies the commitment,
  restores the vault and opens retained content without a desk, IPFS server or
  transaction-history indexer. Junk/missing candidate ciphertext cannot prevent
  public recovery. A public reveal remains explicitly public and retires the
  listing; it is not the proposed private capsule scheme.
- Files/revision: dirty HEAD `1462782b8f15b6c8451a71e010b9a31143168793`;
  `contracts/escrow/{src/lib.rs,README.md}`;
  `tools/fod/{delivery_secrets,pool_receive,prepare_confirm,reveal,submit,serve,
  main,fees}.go`, corresponding receive/reveal/submit/serve/fee tests;
  `spikes/harness/escrow_instant_test.go`, `docs/{SPEC,PROGRESS}.md`,
  `release/{README.md,MANIFEST.json,artifacts/fod_escrow.wasm}` and the rebuilt
  test-hooks module. Other source inputs retain their previous manifest hashes;
  preservation evidence is `output/design/disclosure-availability/` against
  baseline source digest `7f14074e0536521b8e1a71e9e41d9774150ae99ec3412924e5d62e2051d3f01a`.
- Commands/results:
  - `cd tools/fod && GOFLAGS=-modfile=go.local.mod GOPROXY=off go test -count=1 ./...`
    passed (29.816 s). Expanded checks initially exposed an outdated desk test
    double returning a one-byte counter instead of the requested order; that
    fixture was corrected. Typed gas-test constants were corrected before the
    final passing run; failed checks were not counted as passes.
  - `cd tools/fod && GOFLAGS=-modfile=go.local.mod GOPROXY=off go test -count=1 -run 'Test(Disclosure|DeskRechecks|SettledReveal|PoolReceive|RevealBudget)' -v ./...`
    passed (5.966 s): six public-recovery cases, legacy settled private delivery,
    27 state/head combinations across all three disclosure calls, mismatched
    context/keys, and the desk refusing a deadline crossed during signing with
    zero broadcasts. Recovered public content needs no new confirmation.
  - `cd contracts/escrow && cargo test --locked --offline` passed 15 tests;
    existing unused-code/import warnings remain. `cargo build --locked --offline
    --release --target wasm32-unknown-unknown --features test-hooks` and
    `wasm-opt --llvm-memory-copy-fill-lowering -O3 target/wasm32-unknown-unknown/release/fod_escrow.wasm -o dist/fod_escrow.testhooks.wasm`
    produced the module exercised below.
  - `cd spikes/harness && GOFLAGS=-modfile=go.local.mod GOPROXY=off go test -count=1 -run 'Test(Pool|Unincluded)' -v ./...`
    passed 19 tests (4.507 s), including real v2 receipt verification, deadline
    orderings, exact refund and rollback. Successful reveal retained 106 public
    bytes and paid the author; invalid, duplicate and late calls did not retain
    keys. The minimal host reported 239,378 WASM gas; it does not meter every
    node storage callback, so this is not a chain cost measurement. The CLI
    uses a conservative 2,500,000 signing budget with explicit provenance.
  - `node release/check-integration.mjs` passed the offline browser walk and
    local gateway/utility journey. This is existing adapter regression evidence,
    not canonical utility-release evidence.
  - `node release/build-manifest.mjs` passed: 19 published exports, zero test
    hooks/forbidden operators and zero remote interface tags. The current v2
    receipt was verified without generating another proof. Fresh release WASM
    is 380,630 bytes, SHA-256
    `40704aebe3c3d208fa819f1d828d5c89941fe47c22443589a55d057d0afe6f2f`.
    `node release/source-snapshot.mjs output/design/disclosure-availability/source`,
    then `cargo build --locked --offline --release --target wasm32-unknown-unknown`
    in that snapshot's `contracts/escrow`, and
    `wasm-opt --llvm-memory-copy-fill-lowering -O3 output/design/disclosure-availability/source/contracts/escrow/target/wasm32-unknown-unknown/release/fod_escrow.wasm -o output/design/disclosure-availability/rebuilt-escrow.wasm`
    reproduced the same release hash. The snapshot build used a new target
    directory and existing offline dependency caches. Only this evidence entry
    changes afterward; contract source is unchanged. Final manifest/source
    verification follows that documentation update. Of 370 prior source inputs,
    351 retain their baseline hashes and 19 are the scoped files listed above;
    no input was added or removed. This proves local WASM reproducibility, not
    production qualification, timely chain delivery or independent availability.
- Limitation: current `TestUnincludedDeliveryExposureCanPrecedeRefund` still
  passes as a counterexample: keys learned before inclusion survive a refund
  at N+3. New signing and broadcast guards reject known-late calls once the
  completed head is N+2; they cannot guarantee inclusion, hide pending data or
  authenticate a dishonest public RPC. Historical deployments have no new
  `preveal` records; no migration, deployment or transaction was performed.
- Feasibility: read-only sizes plus pinned fee/storage constants give a
  47,070,900 user-gas lower bound for the naive one-call archive of five captured
  blobs (1,569,030 bytes), against a 5,120,000 block cap. Even the smallest
  blob's bound is 8,740,980. Exact inputs/limitations are in
  `output/design/disclosure-availability/availability.json`. Chunked authenticated
  pre-publication is not ruled out, but is not implemented or qualified.
- Next task: establish a finality-dependent release construction or a precise
  justified inclusion assumption before promoting private capsules, and design
  authenticated bounded content preparation plus lightweight state verification.
  Do not extend deadlines, substitute a trusted keeper or close H-7. Paid supply
  stays disabled; the unfinished desktop MVP goal remains blocked.

### 2026-10-01 - S5 exact repayment boundary

- Findings: H-8 / AH-13 closed locally for this candidate. Unilateral delivery,
  fulfilment and reveal require `height < funded+3`; permissionless repayment
  starts at `funded+3`. Buyer-authorized confirmation can still settle an open
  order later. No grace period or deployment.
- Files/revision: `contracts/escrow/src/lib.rs`,
  `tools/fod/{market,serve}.go` and market/watch tests,
  `spikes/harness/escrow_instant_test.go`, `app/src/{faq,views/Market}.tsx/ts`; dirty HEAD
  `1462782b8f15b6c8451a71e010b9a31143168793`, earlier changes preserved.
- Commands/results: the old module failed the new
  `GOFLAGS=-modfile=go.local.mod go test -count=1 -run '^TestPoolRefundOnlyBoundaryBothOrderings$' ./...`
  in `spikes/harness` (deliver incorrectly succeeded at +3). After rebuilding
  the test-hooks WASM, the focused runtime batch passed in 1.458 s;
  `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 -run 'Test.*(DecodePoolOrder|Serve|Watch|SettlementBudget)' ./...`
  passed in 5.471 s after correcting obsolete +4 fixture expectations.
  The final complete CLI suite passed in 24.891 s and complete harness in
  6.728 s (S6/S7 below).
  Final purchase/FAQ wording inspection found stale next-block repayment and
  unconditional disclosure-safety claims; these now state the exact +3 rule,
  included settlement requirement and running-watcher execution dependency.
- Adversarial evidence: both boundary orderings reject deliver/fulfill/reveal,
  a third-party refund returns price plus retrieval fee only to the original
  buyer, duplicate refund refuses, failed calls preserve state/value, +2
  remains settleable. The normal recovery watcher treats +3 as refundable.
- Limitation/next task: runtime fixtures are not chain receipts. Historical
  +3/+4 receipts remain evidence for the previous module. Private delivery
  proving time and off-chain disclosure remain S6/S8 gaps.

### 2026-10-01 - S6 genuine delivery-v2 domain tuple

- Findings: H-5 / AH-14 repaired locally. The 281-byte v2 journal adds SHA-256
  of canonical network `idena`, funding height and canonical funding state
  root. The executing node checks its own header. Missing/mismatched headers,
  wrong domain/context and v1 statements refuse. No deployed contract changes.
- Files/revision: delivery guest/host/journal and README,
  `contracts/escrow/src/{lib,verifier}.rs`,
  `tools/fod/{prepare_delivery,receipt_claim,serve,fees}.go` and tests,
  `spikes/harness/escrow_pool_test.go`, active
  `proofs/delivery/dist/delivery-v2.json`, release builder/pins; same dirty HEAD.
- Commands/results: small witness check
  `cd proofs/delivery && cargo run --locked --offline --release --bin delivery-host -- --check --no-witness`
  passed before proving. `RISC0_PROVER=local` refused without the optional prove
  feature before any expensive work; the installed IPC prover was then used:
  `env -u BONSAI_API_KEY -u BONSAI_API_URL RISC0_PROVER=ipc RISC0_DEV_MODE=0 target/release/delivery-host --no-witness --out dist/delivery-v2.json`.
  Genuine Groth16 proof: 946.459853667 seconds, 7,405,568 cycles, 8 segments,
  281-byte journal, 256-byte seal, 219-byte ciphertext, no witness fields.
  Image ID `60ec600b94660ae6d91ec4332eb0d0ece19e37396e72ef299678547bc95dd4ff`;
  claim `de2956a89a9a2e49c100ab3bcd1420c2c4403290664318727347862b25ce5b66`.
  `cargo test --locked --offline` in `proofs/delivery` passed 3 tests and in
  `contracts/escrow` passed 15 tests. Test-hooks build:
  `cargo build --locked --offline --release --target wasm32-unknown-unknown --features test-hooks`
  then `wasm-opt --llvm-memory-copy-fill-lowering -O3 target/wasm32-unknown-unknown/release/fod_escrow.wasm -o dist/fod_escrow.testhooks.wasm`.
  `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...`
  passed 24.891 s after updating the obsolete signed-size expectation (815 to
  887 bytes). `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 -run 'Test(Pool|Unincluded)' -v ./...`
  passed 18 tests in 4.428 s after using the harness's existing ECIES dependency.
- Adversarial evidence: actual sealed v2 fulfilment verifies inside the WASM
  runtime (195,440,811 WASM gas), rejects absent/different funding headers,
  network/height mutations, duplicate/cross-context settlement and refund after
  settlement. CLI refuses changed public context or inconsistent embedded
  calldata. Native checks refuse tampered seal/journal. The 887-byte signed
  size is measured locally; retained fee-table gas is labelled historical v1,
  not a new v2 chain fee measurement.
- Limitation/next task: same-funding-anchor descendant forks retain the
  blockchain canonical-view assumption. The funding root is available only
  after inclusion: full order-specific precomputation is impossible for this
  statement. Sixteen minutes of local proving has not met the three-block
  private-delivery path. No deadline extension, trusted signer or paid prover
  substitutes for that missing performance guarantee. New chain receipts and
  actual transaction fees remain unmeasured.

### 2026-10-01 - S7 source snapshot and staged release

- Findings: H-9 HTML/WASM source provenance repaired locally; stale HTML
  promotion additionally contained. `MANIFEST.json.source` records the actual
  uncommitted input bytes, file set and executable flags. Private export refuses
  drift, symlinks, private runtime key filenames and an existing destination.
  The builder rebuilds HTML in staging, verifies the current delivery seal,
  checks pins/operators/exports, then promotes. The manifest publishes last;
  interrupted multi-file promotion is detected by artifact hash disagreement,
  not claimed to be one atomic directory transaction.
- Files/revision: `release/{source-snapshot.mjs,source-snapshot.test.mjs,build-manifest.mjs,README.md,MANIFEST.json}`,
  `app/build.mjs`, `spikes/harness/escrow_cli_deploy_test.go`, HTML/WASM; same
  dirty HEAD, no commit or publication. The old ignored prepared transaction
  is untouched. The harness now creates its current deploy preparation offline
  with a generated temporary key and refuses downloads/broadcasts.
- Commands/results: `node --test release/source-snapshot.test.mjs` passed 1
  test (drift, symlink, existing destination and ignored-key cases).
  Private clean source copy under
  `<local-temporary-path>`:
  `cd app && npm ci --offline --ignore-scripts && npm run build` passed;
  `cd contracts/escrow && CARGO_BUILD_JOBS=2 cargo build --locked --offline --release --target wasm32-unknown-unknown && wasm-opt --llvm-memory-copy-fill-lowering -O3 target/wasm32-unknown-unknown/release/fod_escrow.wasm -o ../../fod_escrow.rebuilt.wasm`
  passed (cold Cargo build 30.43 s). HTML SHA-256
  `c6e056bd93ad25df6b657a8ef00b3e13738142d8097930b4fc7c90cf8476d733`,
  302,483 bytes; WASM SHA-256
  `2689ab5e5856640c78f510a7f164bc74dae8575c4dcf0ea7f4b78263b819cff4`,
  380,582 bytes. Both rebuilt files match the candidate byte-for-byte.
  After correcting the final customer wording, HTML is 302,139 bytes, SHA-256
  `a67c7903dbd44bd9b57f6f7a50e6ae1f600d9aeee6d7802b4eaf5177d198618c`;
  `cd app && npm run typecheck && npm run build` passed. The final private
  snapshot is `output/release/security-followup-2026-10-01/source/`; its source
  inventory is copied from the final manifest. Rebuilds use the commands above.
  `node release/build-manifest.mjs` passed: 19 exports, no test hooks, zero
  forbidden operators/remote tags. A temporary `npm` shim emitting a remote
  script tag (`PATH=<temporary-shim>:$PATH node release/build-manifest.mjs`)
  refused promotion; Node hash assertions confirmed unchanged HTML, WASM and
  manifest, and all staging files were removed.
  `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...`
  first found the historical 380,347-byte deploy fixture; after fresh offline
  preparation, toolchain/command-invocation corrections were required, then the
  complete suite passed in 6.728 s. No failed check was ignored or skipped.
  Toolchain corrections used cached Go 1.26.8 for the CLI rather than the
  harness's 1.26.5, without dependency upgrades; the current positional
  `prepare-deploy` command replaces the obsolete `-mode` invocation.
  `cd app && npm test && npm run typecheck && npm run build` passed 14 tests;
  `node release/check-integration.mjs` passed the required browser walk and
  loopback gateway/utility scenarios.
  `cd proofs/delivery && target/release/delivery-host --order orders/example-order.json --check --no-witness`
  passed for the documented v2 order example with synthetic funding context;
  no second expensive proof was generated.
- Limitation/next task: immutable reference dependencies and cached toolchains
  remain prerequisites. HTML/WASM clean rebuild is established; full proof
  regeneration and CLI clean-environment reproduction were not repeated.
  The final inventory is in the manifest, rather than self-hashed into this
  input document. Final documentation/test-fixture edits change source identity
  but not the verified artifact build inputs. H-1/H-2/H-3/H-7 remain open.

### 2026-10-01 - S8 fair-exchange and utility feasibility

- Findings: H-7 and H-3 remain open, with a new concrete counterexample rather
  than an unsupported closure claim. Registration-v3 already validates the
  CID-bound ciphertexts, committed decryption keys and supported flip structure.
  Delivery-v2 does not add evidence of the buyer's retained bytes.
- Files/revision: `spikes/harness/escrow_instant_test.go`, existing specification,
  matrix and handovers; same dirty HEAD. Read the registration guest, session
  statement and utility scoring/control paths; no new operator authority added.
- Commands/results: `TestUnincludedDeliveryExposureCanPrecedeRefund` passed in
  the focused and complete harness batches above. It constructs valid ECIES
  ciphertext for the buyer, decrypts it without including any delivery or
  settlement, then repays the buyer permissionlessly at +3. The learned keys
  remain usable. This uses synthetic fixtures, not a mainnet transaction or
  proof of historical provenance. The reference integration recomputes witnessed
  answers and refuses tampering, but is still local service evidence.
- Limitation/next task: establish canonical availability/possession and a
  disclosure construction protected against dropped/censored settlement;
  signatures alone do not prove possession. For utility, specify authenticated
  answer authority, canonical wallet-bound answer timing/global consumption
  and the actual enforced benefit. The owner was asked once for the concrete
  lab utility/asset/chain (native isolated Idena faucet recommended, EVM target
  alternative); no answer yet. The five economic choices remain approved.
  Independent-operator availability and an authenticated lightweight state
  route remain unproved. No remote host, real key/fund, deployment, broadcast,
  commit or push was used. Paid supply remains disabled and MVP completion is
  not claimed.

### 2026-10-01 - S1 purchase integrity (local repair)

- Findings: SEC-1 closed, SEC-2 mitigated on the ordinary own-node CLI path;
  full canonical availability/fair-exchange guarantee remains open.
- Files/revision: `tools/fod/{cid,availability_check,pool_buy,submit,pool_receive,prepare_confirm,main,commands}.go`
  and focused tests; HEAD `1462782b8f15b6c8451a71e010b9a31143168793`, dirty
  checkout preserved (114 tracked modifications and 19 untracked entries at
  reconciliation; no commit). Retained bytes are privately and durably written
  in the prepared buy. Signing reconstructs CIDs and rechecks current bundle
  terms; recovery verifies/decrypts retained bytes before normal confirmation.
  A delivery-key signature protects the usable-content record; confirmation
  signing rechecks its exact bytes and current order/buyer/nonce/key/author/count.
  This is buyer consent, not a signature substituting for canonical storage.
- Commands/results: `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...`
  passed (19.123 s after correcting two obsolete fixtures; final signing-receipt
  revision passed the complete CLI suite in 25.657 s after fixing two compile
  errors in the new test/import);
  `cd contracts/escrow && cargo test --locked --offline` passed, 15 tests;
  `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 -run 'Test.*(Deadline|Boundary|Refund|Reveal|Settlement|Restart|Replay)' ./...`
  passed (2.386 s).
- Adversarial evidence: a lying CID calculator cannot authorize mismatched
  bytes; omitted/substituted bytes, commitment, bundle, nonce, amount and stale
  signing records refuse. A storage-only node stub serves no flip bytes, yet
  retained ciphertext decrypts after restart and signing freshness expiry.
  Keys-only and unsigned confirmation records refuse. Edited ciphertext/count,
  wrong order and an attacker-signed delivery key refuse; failed revalidation
  clears the signing guard. Valid receipts sign after the own-node order
  recheck. Restart reuses the first purchase's exact vault ciphertext and a
  second purchase at the same vault path refuses. Tests are synthetic/local, not new
  historical provenance, genuine session evidence or live receipts.
- Limitation/next task: the contract proves key release, not durable byte
  storage. An arbitrary RPC cannot authenticate canonical state merely by being
  queried again. A custom buyer may bypass the CLI. Secret disclosure off-chain
  before timely settlement may still leave the buyer able to refund. Preserve
  the block boundary and investigate stronger canonical evidence (handover H-7).

### 2026-10-01 - S2 utility authorization (local repair)

- Findings: SEC-3 adapter bypass closed; clause 10.3(6) remains unfulfilled.
- Files/revision: `utility/src/{policy,access}.ts`, `utility/test/access.test.ts`,
  `utility/example-policy.json`, `utility/README.md`, `docs/SPEC.md`; same dirty
  HEAD. Policies need wallet-signed answers, content pins and configured
  windows. Provider-only pass/transcript claims cannot grant, even with proof
  flags omitted/false. Closed-window scoring binds the utility and window.
- Commands/results: `cd utility && npm test` passed 40/40;
  `cd utility && npm run typecheck` passed.
- Adversarial evidence: unsafe policy files refuse; correct provider transcript
  without witnessed wallet answers refuses; caller-asserted control refuses;
  witnessed answers + closed window + fresh signed control nonce grants once,
  and nonce replay refuses. Wrong reveal, wrong window/utility and early scoring
  refuse. At 10,000 entries both memory and durable stores refuse newcomers,
  preserve live consumed allowances, then free expired capacity; reopening the
  durable store retains the new allowance. The example policy's empty key/dummy
  pin is a disabled template. This is an expiring local quota, not global or
  lifetime consumption.
- Limitation/next task: local clock/store, answer-key selection and benefit
  release are still operator-controlled. No canonical answer provenance,
  session consumption or actual faucet/dapp release was implemented. Correct
  H-3 rather than claiming the full-product scoring blocker is resolved.

### 2026-10-01 - S3 recovery and local file safety (local repair)

- Findings: SEC-4 and SEC-6 closed locally.
- Files/revision: `tools/fod/{vault,main,export,session_pool}.go`, vault/export
  and purchased-content tests; same dirty HEAD. Fully encrypted vaults publish
  atomically without overwriting an existing vault. Same-key reuse is permitted;
  different keys/passphrases require a fresh path. Private replacements are
  atomic and directory-rooted export paths cannot escape through traversal or
  external symlinks.
- Commands/results: `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...`
  passed (23.727 s);
  the focused vault/export/pool follow-up passed (16.138 s), including
  semantically identical existing JSON key payloads
  with different formatting/order, preserving the original encrypted vault.
  The final complete CLI suite including signed receipts passed in 25.657 s.
- Adversarial evidence: interrupted writes publish no partial vault and clean
  staging files; old ciphertext remains unchanged after attempted replacement;
  private-output symlinks do not overwrite their external targets; crafted CID
  traversal and input/output symlinks refuse; normal export/pool ingestion passes.
- Limitation/next task: these protect local recovery files, not compromised
  operating systems or lost passphrases. S4 browser/network checks are complete;
  remaining protocol and release qualification is recorded in section 7.

### 2026-10-01 - S4 browser/network and final release checks

- Findings: SEC-5 and SEC-7 closed locally; SEC-8 release failure check closed;
  SEC-9 quota follow-up verification is recorded in S2 above.
- Files/revision: `app/src/{provider,views/Session,views/GateSetup,views/Trades}.tsx/ts`,
  `app/test/provider.test.ts`, `app/verify.mjs`, CLI RPC/body readers and tests,
  `release/build-manifest.mjs`, HTML/manifest after final build; same dirty HEAD.
- Commands/results: `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...`
  passed (24.084 s); `cd app && npm test && npm run typecheck && npm run build`
  passed, 14/14 tests; `cd app && npm run verify` passed all checks.
  `node release/check-integration.mjs` passed the required browser walk and
  loopback gateway/utility scenarios; `node release/build-manifest.mjs` passed
  with 19 published exports, no test hooks, zero forbidden WASM operators and
  zero remote HTML tags. A temporary failing `wasm-objdump` shim exited 1; the
  builder refused it, retained the prior WASM/manifest SHA-256 values and removed
  staging. The final CLI suite also passed (25.657 s).
- Adversarial evidence: missing/substituted pins and wrong provider identity
  cannot verify; valid pins pass. The browser refuses a grant for substituted
  keys and leaves no browser-storage residue. Declared oversized and lengthless
  RPC/explorer/IPFS bodies refuse with bounded reads; exact-boundary and normal
  responses pass. Private-key material is absent from these retained fixtures.
- Artifacts: `app/dist/flips-on-demand.html`, 302421 bytes, SHA-256
  `b4936fafc115148cd1779c004e69ab46c0cda5d9035550a0b9544c32718db4a5`;
  `release/artifacts/fod_escrow.wasm`, 380347 bytes, SHA-256
  `0769bbf075f3da81423dbdec959b4480c7d6c0e5fb497cc9bde64e8d21209d48`.
  Both match `release/MANIFEST.json`; it correctly states `worktreeDirty:true`
  and does not qualify this changed candidate with historical receipts.
- Limitation/next task: local gates do not establish fresh-chain execution,
  independent operators or dirty-source clean-clone reproducibility. The
  supported local repairs are complete; H-3/H-5/H-7/H-8/H-9 remain open. Keep
  the MVP unfinished and preserve paid-supply containment.


### 2026-09-30 - the market snapshot carries its own digest, so two operators compare one line

Package: the "independent node operators verify the same outcomes" row, whose
evidence was a hand-diff of two `fod market` payloads that differ in the
observation timestamp by construction.

What changed:

- `tools/fod/market.go`: `marketState` gains `stateDigest`, a SHA-256 over the
  state as the tool writes it - the schema's own key order, no whitespace - with
  `observedAt` and the digest itself removed; `fod market` prints
  `state digest <hex>` beside the registry line and carries the field in the
  JSON it writes (`SPEC.md` 4.5). `tools/fod/market_test.go` pins that the
  digest is stable, ignores only the observation time and the stored digest
  itself, changes with the block, code hash and contract, and is reproducible by
  a reader that only has the written file.

Evidence on this revision:

- `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok,
  17.3 s; `gofmt -l market.go market_test.go` clean.
- Both lab nodes, read-only, with the CLI built from this tree:
  `fod market -contract 0x8a225b8e… -network 33411 -rpc http://127.0.0.1:19095
  -key … -out …` and the same against 19097 both report block 7254, code hash
  `54caa028…`, 1 listing (units 0, nonce 3, strikes 2), 3 orders, 1 credit and
  the same `state digest 11968c6e96866991a4c0ebee7f56973a7892501c6e5ee4fcff83742a5991deed`;
  the written payloads are equal once `observedAt` is removed.

Remaining limitation: the digest covers what this tool reads and writes, not the
chain's own consensus - it makes two reads comparable, it is not a proof about
either node. Both nodes still run on one host (`H-1`).

### 2026-09-30 - read-only re-verification of the live receipts with the CLI this tree builds

Scope: the objective's "real execution receipts" and "independent node operators
verify the same outcomes" rows, re-checked against the running v4 lab chain
(network 33411, escrow `0x8a225b8ec06e84601ecf90ded4f7b3f361654d5a`, code hash
`54caa02860166c1d9caad70f7417f4c99714506e4b525a4b7ad386cf48048936`) with the
CLI built from this tree (`cd tools/fod && go build -o /tmp/fod-current .`).
Nothing was broadcast: this task's authority excludes transaction broadcasts, so
the exercise is read-only by construction.

Commands and results:

- `fod market -contract 0x8a225b8e… -network 33411 -rpc http://127.0.0.1:19095
  -key … -out /tmp/market-19095.json`, then the same against 19097 with its own
  key: both report block 7234, code hash `54caa028…`, 1 listing (units 0,
  nonce 3, strikes 2, reserve 0), 3 orders and 1 credit, and the two payloads
  are byte-identical once the observation timestamp is removed.
- `bcn_txReceipt` against both nodes for the three recorded transactions whose
  full hashes are in this record: the buyer's confirm
  `0xdd130fd1537ee0c725cf452a2431a71f208b12e30193d4e17366bad3fdb8ea41`
  (`method` `confirm`, `success` true, `gasUsed` 20,357, `gasCost` 203.57,
  `txFee` 35.1, output `{"ok":true}`), the host credit withdrawal
  `0xd24d10e4d17d99d06d64896eeb5e488324d04cdc195b0eba9db76c3788379eb8`
  (`withdraw`, 3,724 / 37.24) and the donor reserve withdrawal
  `0x7b91a99183b17509e1f33514edd0e882bef79706b1abdd5e15727e83ae39c779`
  (`withdrawReserve`, 4,227 / 42.27). Each pair is identical across the two
  nodes, with canonical `sha256` digests `dc4923b4…`, `8a84a847…` and
  `4faeb717…` over the receipt payloads.

Limits stated rather than implied: the confirm's 20,357 user gas is the figure
the AH-18 row records, and the module (`0769bbf0…`) has not changed since that
deployment, so the recorded receipts still describe the shipped artefact; the
register, refund and delivery-proof receipts are recorded with truncated hashes,
so only these three could be re-read this way. Because broadcasts are outside
this task's authority, no fresh transaction was produced, which is also why
H-5's re-proving stays deferred: changing the delivery guest changes the module
hash and would retire these live receipts without the ability to mint new ones
here.

### 2026-09-30 - Package L - the utility witnesses the answers in-window and scores the session itself

Package: the objective's "without trusted scoring" half and the H-3 residue -
the provider's transcript was still its own attestation of what a participant
submitted.

What changed:

- `app/src/score.ts`: the reveal rule is factored into `revealedAnswerKey` (the
  reveal has to hash to the digest the session is pinned by, and to carry a
  usable threshold and answer digests) and reused by `verifyTranscript`
  (provider-side arithmetic), `verifyScore` (the participant's own check, which
  now also refuses a transcript that omits an answer the browser submitted) and
  the new `scoreCommittedAnswers` (a utility's arithmetic over the answers it
  witnessed). `canonicalAnswers`, `answersDigest` and `answerCommitmentText`
  define the commitment a wallet signs.
- `utility/src/commitments.ts` (new): `AnswerCommitments` verifies that
  commitment - the answer list, a fresh `issuedAt`, the window from the policy,
  and the wallet signature over `fod-answers:v1:<utility>:<session>:<address>:<issuedAt>:<digest>` -
  keeps the latest per utility, session and address, is bounded, and expires an
  hour after the close.
- `utility/src/policy.ts`, `utility/src/server.ts`, `utility/src/access.ts`:
  `sessionWindows` and `requireCommittedAnswers` are validated policy fields and
  `/policy` publishes them; `POST /answer-commitment` records a commitment (404
  `unknown_session`, 403 `outside_window`/`stale_commitment`, 400
  `bad_address`/`bad_answers`/`bad_signature`, 503 `busy`); with
  `requireCommittedAnswers` the decision scores the session from the commitment
  and the pinned reveal, refuses `answers_not_committed`, `score_not_passed` or
  `score_mismatch`, does not consult the provider's score or `outcome`, and
  returns `scoring: { by: "utility", ... }`. The commitment is only usable for
  the address the signed result names.
- `app/src/views/Session.tsx`: every accepted answer is committed to the utility
  while the window is open, signed with the wallet that will claim the grant;
  the page says whether the utility accepted it and why not; the walk's utility
  policy now requires the commitment.
- `app/verify.mjs`, `release/check-integration.mjs`: both halves of the walk and
  both integration customers commit their answers; the integration forges a
  result the gateway's own key signed whose verdict says `not-passed` and the
  utility grants anyway. The walk's console-error evidence now names the failing
  URL, which is how the commitment endpoint's window was diagnosed.

Evidence on this revision:

- `cd utility && npm test` - 35 of 35 (four cases for a utility-scored policy:
  grant on its own arithmetic despite the provider's verdict, refusal of a pass
  the answers do not earn, no commitment or one for another address, and a
  tampered or unpinned reveal; plus the endpoint cases and the store's
  latest-commitment rule); `cd utility && npm run typecheck` clean.
- `cd app && npm test` - 13 of 13; typecheck clean.
- `cd app && npm run verify` - 60 checks, ALL CHECKS PASSED, including "the
  browser committed its answers to the utility while the window was open".
- `node release/check-integration.mjs` - ALL CHECKS PASSED: both customers'
  commitments recorded inside the window, the grant scored by the utility
  (`{"by":"utility","assigned":2,"answered":2,"correct":2,"passThreshold":2}`),
  the forged `not-passed` verdict still granted, the tampered result refused,
  the re-enrolment scope and the quota-once rules unchanged.
- `cd app && npm run build` - interface 302,236 bytes
  `e3922fcf4d51733b...`, identical on rebuild; `node release/build-manifest.mjs`
  regenerated the manifest (module unchanged 380,347 bytes `0769bbf0...`, 0
  forbidden operators, 0 remote tags).

Remaining limitation: the utility's own clock and store witness the window, the
operator's threshold has to be chosen against guessing, and the answer key is
pinned by the operator's examination of the pool. An independently timestamped
commitment would need a chain transaction from the participant and an owner
decision; that is recorded in H-3, not implemented.

### 2026-09-30 - Package K - the hosted shared node builds the author's export, pool and connection file

Package: `SPEC.md` 10.4 G3's remaining work - the hosted node's export and vault
integration, and the connection file the author imports.

What changed:

- `tools/fod/sharednode.go` (new): `BuildSharedNode` takes the author's vault,
  the captured inventory and the saved blobs, performs
  `ExportSessionContent`, builds the session pool a gateway loads, and writes
  `fod-shared-node.json` naming the node, the author the escrow pays (normalized
  to the `0x` form the interface parses), the pool and the custody boundary.
  The refusal cases are explicit: unknown custody, a URL the interface cannot
  read, and a missing vault or inventory are rejected before anything is built.
- `tools/fod/main.go`, `tools/fod/commands.go`: the `shared-node` mode with
  `-vault -inventory -blob-dir -out -node-url -session-id -utility-id
  -threshold -closes-at [-opens-at] [-node-id] [-custody hosted|self]`, its
  usage line, and the command registration.
- `app/src/views/SharedNode.tsx`: the author journey leads with the one node
  command and keeps the listing and the delivery desk as separate, named steps.
- `app/verify.mjs`, `docs/SPEC.md` (6.x and 10.4 G3) state the new path and its
  limits.

Evidence on this revision:

- `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok
  (17.2 s), including the two new cases: the export, pool and connection built
  from a synthetic vault (the decoded flip on disk, the pool 0600 with the
  threshold and one challenge, the connection file 0644 naming node, author,
  custody and pool), and the self-custody plus refusal paths.
- `gofmt -l sharednode.go sharednode_test.go main.go commands.go` - no output.
- `cd app && npm run typecheck` clean; `npm run build` - interface 300,129 bytes
  `66a8c044c25f43de...`; `npm run verify` - ALL CHECKS PASSED, including "the
  shared-node journey names the node command that builds the author's export and
  pool".
- `node release/check-integration.mjs` - ALL CHECKS PASSED;
  `node release/build-manifest.mjs` regenerated the manifest.

Remaining limitation: the command performs the export and the pool; the listing
and donation stay the author's own prepared action, and the delivery desk is the
separate long-running `fod serve`. Those two are named in the journey instead of
being stitched into one command, which is why the G3 row stays "mostly met".

### 2026-09-30 - Package J - the utility recomputes the score instead of taking the provider's word

Package: the enforcement half of the objective's "without trusted scoring"
clause (`SPEC.md` 8.3, matrix row 6, H-3), on top of Package I.

What changed:

- `app/src/score.ts`: the shared rule is split into `verifyTranscript` (no
  participant input: the reveal must hash to the digest the session is pinned
  by, and `assigned`/`answered`/`correct`/outcome must follow from the transcript
  against that revealed answer key) and `verifyScore` (the participant's check,
  which first refuses a transcript that misnames, substitutes or omits an answer
  this browser submitted).
- `utility/src/score.ts` (new): `checkProviderScore` runs `verifyTranscript`
  with the digest this utility pinned - or the result's own `contentDigest`
  when the policy pins none - and reports `score_mismatch` or, when the policy
  requires the proof and the material is missing, `score_unverifiable`.
- `utility/src/policy.ts`, `utility/src/access.ts`, `utility/src/server.ts`:
  `requireScoreProof` is a policy field, `/policy` publishes it, `/access`
  accepts the claimant's `scoreReveal` as untrusted input, and `decideAccess`
  (now async) recomputes before the identity, control and quota steps.
- `utility/src/results.ts` validates the transcript it will recompute from.
- `app/src/views/Session.tsx` sends the reveal it already fetches when it
  recomputes its own score; `app/src/faq.ts`, `utility/README.md` and `SPEC.md`
  8.3 state the rule and the new refusal reasons.

Evidence on this revision:

- `cd app && npm test` - 13 of 13 (four new cases: an omitted answer, the
  utility-side arithmetic on its own, a utility-side mismatch, and a reveal that
  does not hash to the pin).
- `cd utility && npm test` - 28 of 28, including five new cases under "a policy
  that requires a recomputed score": `score_unverifiable` without the material,
  a grant on a recomputed score, `score_mismatch` for a signed score the
  transcript does not support, `score_mismatch` for a reveal that does not hash
  to the pin even when the proof is optional, and the unchanged behaviour of a
  policy that does not require the proof.
- `cd utility && npm run typecheck`, `cd app && npm run typecheck`,
  `cd gateway && npm run typecheck` - clean.
- `cd app && npm run build` - interface 299,889 bytes `f7f34ac6878db881...`;
  `npm run verify` - ALL CHECKS PASSED with the walk's utility policy requiring
  the proof, so the browser really sends the reveal.
- `node release/check-integration.mjs` - ALL CHECKS PASSED, including the new
  "the utility refuses a validly signed result its own arithmetic does not
  reproduce": the integration reads the gateway's application key file, flips
  one transcript digest, re-signs, and the utility answers `score_mismatch`
  before quota is even considered.
- `node release/build-manifest.mjs` - regenerated: interface 299,889 bytes
  `f7f34ac6...`, module unchanged 380,347 bytes `0769bbf0...`, 0 forbidden
  operators, 0 interface remote tags.

Remaining limitation: the transcript is the provider's own attestation of what a
participant submitted. Both sides now recompute the arithmetic, and a
participant can refute a substituted or omitted answer, but a provider that
cooperates with a participant can attest answers neither had to solve; closing
that needs a commitment the participant makes while the session is open, which
stays H-3.

### 2026-09-30 - Package I - the participant recomputes its own score instead of trusting the provider's statement

Package: the objective's "without trusted scoring" half (`SPEC.md` 10.4
"provenance versus scoring", matrix row 6, H-3), first implemented layer.

What changed:

- `gateway/src/session.ts`, `gateway/src/routes.ts`: the score summary carries a
  `transcript` - one entry per assigned challenge with the digest of the answer
  the provider recorded, `null` when it recorded none - and the signed result
  publishes it as `transcript: { rule, entries }`, the rule naming the canonical
  answer form the digest uses.
- `app/src/score.ts` (new): `verifyScore`, a pure check with no network or DOM.
  It refuses when no digest was pinned before the session, when the result
  states no transcript, when the closed policy published no reveal, when the
  reveal does not hash to the pin, when the reveal is about another session or
  carries a different pass threshold than the signed score, when the transcript
  repeats or misnames a challenge or omits one the score counts, when the
  provider records an answer this browser never sent, and when the recorded
  answer is not the one this browser submitted; then it recomputes `answered`,
  `correct` and the outcome and refuses any disagreement with the signed score.
- `app/src/views/Session.tsx`, `app/src/provider.ts`: the client keeps the
  `contentDigest` from the first policy read and the answer it submitted per
  challenge, fetches the closed policy's `contentReveal` with the result, shows
  "recomputed from your answers" or the refusal reason, and only a result that
  reproduces the signed score is claimable.
- `app/src/faq.ts` and `docs/SPEC.md` 4.5/8.3/10.4/11 state the boundary as
  implemented, including that the check protects the participant and not the
  utility.

Evidence on this revision:

- `cd app && npm test` (new `node --test` suite, `app/test/score.test.ts`) - 9 of
  9: an honest pass, canonical spelling and whitespace, an inflated score, a
  substituted answer, an answer the browser never sent, a reveal that does not
  hash to the pin, a signed threshold that is not the pinned one, a missing
  transcript, and an unanswered challenge counted as answered.
- `cd gateway && npm test` - 36 of 36, including the new "publishes the answers
  it scored" case (transcript entries, `null` for the unanswered challenge, and
  the provider signature covering the transcript).
- `cd app && npm run typecheck` and `cd gateway && npm run typecheck` - clean.
- `cd app && npm run build` - interface 298,928 bytes
  `383c59b7719c43ef...`; `npm run verify` - ALL CHECKS PASSED, including "the
  browser recomputed the score from the answers it submitted".
- `node release/check-integration.mjs` - ALL CHECKS PASSED, with the score
  suite as its first check.
- `node release/build-manifest.mjs` - regenerated: interface 298,928 bytes
  `383c59b7...`, module unchanged 380,347 bytes `0769bbf0...`, 0 forbidden
  operators, 0 interface remote tags.

Remaining limitation: this is the participant's refutation. A utility that
grants on the provider's signed result still trusts it, and a provider
cooperating with a participant can attest answers neither had to solve. Both
need a utility-side recomputation from a participant-signed transcript plus a
commitment the participant makes while the session is open, which stays H-3.

### 2026-09-30 - AH-19..AH-23 closed - the desk settles a missing confirmation, the writers tighten, the tree is redacted, and the artefact paths separate

Package: the five audit-5 findings (section 6) and the second Codex Security
scan `e87b560e-75bd-43ac-9f26-22043435ff6a`.

What changed:

- `tools/fod/serve.go`: the pass plans a `reveal` for a delivered order this
  desk published when it has `revealMargin` blocks or fewer left (default 2,
  `-reveal-margin`) and a configured prover's `fulfill` no longer fits; a
  funded order and a foreign delivery slot are never revealed, and the
  prepared reveal (its calldata is the committed secrets) is written through
  the private writer (`AH-19`).
- `tools/fod/main.go`, `receive.go`, `export.go`, `session_pool.go`:
  `writePrivateBytes` (0600 on create, re-applied on rewrite) backs the
  receive, export and session-pool outputs, and `writeExportPart` re-applies
  its mode (`AH-20`).
- `.gitignore`: ignores `sweep.json`, `epoch-keys*.json` and `*.private`, so
  `tools/fod/sweep.json` is no longer an untracked dirty file (`AH-21`).
- `tools/fod/indexer_inventory_test.go`, `prepare_offer_test.go`,
  `stateproof_test.go`, `docs/PROGRESS.md`: the test constants and fixtures use
  synthetic addresses, and the operator's full addresses are redacted to the
  prefix form the documents already use; the pinned real IAVL proof fixture
  `spikes/iavl-proof/core/fixtures/bundle.json` stays as the one stated
  exception (`AH-21`).
- `release/check-integration.mjs`: a failing walk keeps the last 40 non-empty
  lines in the failure detail and writes the full output to
  `$TMPDIR/fod-walk-failure.log` (`AH-22`).
- `contracts/escrow/dist/fod_escrow.testhooks.wasm`,
  `spikes/harness/helpers_test.go`, `release/build-manifest.mjs`,
  `release/README.md`, `contracts/escrow/README.md`: the harness artefact is
  renamed out of the release path, the release build no longer copies into
  `dist/`, and the manifest stage refuses to promote a module whose export
  section is not the published 19-name surface (`AH-23`).
- `app/src/faq.ts`, `app/verify.mjs` and the rebuilt interface: the delivery
  FAQ names both non-cooperative settlement paths (reveal inside the window,
  proof with a prover); the walk's own session window is 8 s instead of 3.5 s
  after the improved trace showed it could close before the session opened
  under load.

Verification on this revision (dirty worktree, `1462782`):

- `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok,
  15.9 s; the new regressions are
  `TestServePlanRevealsAnUnconfirmedDeliveryAtTheBoundary`,
  `TestServeWritesTheRevealActionOwnerOnly` and
  `TestExportTightensAPreExistingOutput`, with the receive and session-pool
  rewrite cases added to their existing tests.
- `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` -
  ok, 6.1 s, against `dist/fod_escrow.testhooks.wasm`.
- `node release/build-manifest.mjs` - the new gate reports `release exports: 19
  published names, no test hooks`; the manifest records interface 293,805 bytes
  `b5b3d5aa…` (two consecutive `npm run build` runs produced the same hash; the
  previous 293,244 `03aa5573…` predates the FAQ text) and module 380,347 bytes
  `0769bbf0…`, unchanged.
- `node release/check-integration.mjs` - ALL CHECKS PASSED twice: once alone
  (24.9 s) and once in parallel with both Go suites (25.0 s), the walk green in
  both runs with the 8 s windows.
- The export reader was checked against both artefacts: the release module has
  19 exports with no `seed*`; the test-hooks module has 22 with
  `seedPoolBundle`/`seedPoolCovered`/`seedPoolOrder`.
- `git check-ignore -v tools/fod/sweep.json` matches the new rule; a grep for
  the three full addresses finds only the IAVL fixture key.

Limits: the reveal retires the bundle's remaining units, so the desk trades
unsold inventory for settling a non-confirming order at the boundary; the
margin is configurable (`-reveal-margin`, default 2 blocks). The IAVL fixture
exception above is the only remaining real-address occurrence in tracked
content; regenerating it as a synthetic proof stays a pre-publication task.

### 2026-09-30 - AH-18 resolved - the buyer's `confirm` was underpriced, and the guard now refuses that

The v4 sweep's open leg (both confirms accepted into the mempool, never
included) was not a contract defect and not a host difference. The node derives
a contract call's gas budget from the signed max fee, `(MaxFee - sizeFee) /
feePerGas` (`blockchain/blockchain.go` `getGasLimit`); the confirmation that
carries the buyer's 219-byte ciphertext needs 20,357 user gas; and
`tools/fod/fees.go` still carried the pre-`AH-10` digest-form figure (14,747)
as a placeholder. With a 200 iDNA budget the call could draw about 19,649 user
gas, so the proposal ran it out of gas and the pinned node added the hash to
its persistent blacklist - silently, exactly the symptom the sweep recorded.

How it was pinned, all read-only on the live node:

- the node's RPC exposes `contract_estimateCall`, which runs a call against a
  check state and returns its receipt: the failing `confirm` came back
  `success=false`, `RuntimeError: unreachable` after 2,718 gas - and the
  contract's `error_reply` is `panic!("fod:refused")` by design, so a refusal
  and a trap look alike there;
- the same call on order 3 (a funded order whose buyer is the signable key)
  executed: `success=true`, `gasUsed 20,357` for the 227-byte argument
  (2,035,700 wasm); a stranger's call refused at the `not_buyer` check (2,718
  gas), which shows the estimator resolves a signed caller and not an unsigned
  `from`;
- a `refund` (14,190 gas) landed as a transaction from the same key and epoch,
  so the builder included other calls; `deliver` executed and landed too, so
  zero-value calls are not the issue.

Resolution, verified:

- the confirm row now carries the measured 2,035,700 wasm gas over the
  350-byte reference envelope (the v4 run signed 351; the guard prices the
  signed size), and `TestTheConfirmRowCarriesTheAH10Envelope` pins the
  figure, the 238.57 / 238.67 iDNA requirements and the 182.57 the superseded
  row estimated;
- the corrected guard refuses the exact input that had been blacklisted:
  `refused before broadcasting: this signed transaction needs a max fee of
  238670000000000000000, and it carries 200000000000000000000`;
- the same call then settled order 2 live with a 260 iDNA budget: tx
  `0xdd130fd1537ee0c725cf452a2431a71f208b12e30193d4e17366bad3fdb8ea41`
  (block `0x45d973a5…`, `usedFee` 35.1, timestamp 1790728742). The market then
  read order 2 `settled`, pool `pending` 0, and the retrieval host credited
  `1000000000000000000`. The confirm path carries no deadline gate by design
  (the deadline rule gates the seller's unilateral paths), so the buyer's own
  checked bytes settled the order after its window had passed.

Files: `tools/fod/fees.go`, `tools/fod/fees_test.go`, `app/src/faq.ts`,
`docs/SPEC.md` (7.4 fee row), `app/dist/flips-on-demand.html`,
`release/MANIFEST.json`.

Commands and results: `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test
-count=1 ./...` passed (14.1 s); `cd app && npm run build` wrote 293,244 bytes
`03aa5573…` (two consecutive builds identical) and `npm run verify` passed all
checks; `node release/build-manifest.mjs` recorded interface 293,244 bytes
`03aa5573…` and module 380,347 bytes `0769bbf0…`, 0 forbidden operators, 0
remote tags; `node release/check-integration.mjs` ALL CHECKS PASSED (13).
Both lab nodes then read the same payload at head 4514 (`fod market` through
19095 and 19097, canonical digest `fabccee386138899…`, orders 1 refunded /
2 settled / 3 refunded, `pending` 0, host credit `1000000000000000000`), and
`fod fees` prints the corrected row: `confirm 2035700 20357 350
238570000000000000000 ... 238.57`.

### 2026-09-30 - AH-17 correction - the buy check committed the decoded bytes, and the live chain refused every honest bundle

The AH-17 commit check recomputed the chain's inventory commitment over the
*hex of the decoded* CID bytes (`tools/fod/pool_buy.go` after the P2b entry),
while the registration guest hashes each CID as the canonical multibase text
the node writes (`proofs/registration-v2/methods/guest/src/main.rs`
`manifest_cids` -> sort -> `statement::inventory_commitment`). The unit tests
passed because their fixtures built the commitment with the same wrong helper.
The final acceptance exercise caught it: with the v4 lab's registered bundle
the chain held `3f56c06e381d9bb9f215433fed03260a1c8b72dab9680dfdc2fd4a5798a0f237`
(the text form) and `prepare-buy` computed
`d949a69b57ea2871f9e50935bd07dca5ab6741dc5d267302eba5de239dbc28c1` (the
decoded form) and refused (`the advisory … does not describe bundle …`).

Fixed: the check validates each CID with `cidBytesFromString` and commits the
canonical text (`cidString(decoded)`); the fixtures build chain-shaped
commitments and `TestTheInventoryCommitmentCommitsTheCanonicalCIDText` pins
the live value independently of the helper.

Files: `tools/fod/pool_buy.go`, `tools/fod/availability_check.go`,
`tools/fod/pool_buy_test.go`.

Commands and results: `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test
-count=1 ./...` passed (14.8 s); the live `prepare-buy` on the v4 lab then
printed `checked 1/1 flips fetched and rebuilt by this node at
2026-09-30T00:03:30Z, and the advisory's CIDs match the bundle's committed
inventory`.

### 2026-09-30 - final acceptance sweep v4 (partial) - four legs land, the buyer's confirm never does

Fresh deployment of the current module on the lab chain (network 33411, escrow
`0x8a225b8ec06e84601ecf90ded4f7b3f361654d5a`, code hash `54caa028…`, two
reference nodes on RPC 19095/19097). Receipts that landed:

- `registerBundle` of the v4 session bundle (prepared
  `/tmp/fod-v4/prepared-register.json`, receipt `/tmp/fod-v4/registration-v4.json`).
- three `donateUnit` calls by the registered author (`donate-1..3.json`),
  three units, then `addReserve` of 200 iDNA (`submitted-fund.json`).
- order 1: `buy` `0xf04d9eadefc601c3c33490ea1056c6009874c3215477c315c265da872178665e`
  (included in a block the node logged with `txs=1`), desk `deliver` inside the
  window, buyer `receive` verified the ciphertext against the bundle's key
  commitment (`/tmp/fod-v4/buyer.vault`), then the permissionless `refund`
  `0x50e8b74bcb0a583d669076249445f4095435de47bb6fa9aa59319fdba03f2fa9`
  paid the buyer `+101` iDNA (paid + retrieval fee) in one transaction and
  counted the strike; the market read `state refunded`.
- order 2 (buyer2 `0xcd0d46e3…`): `buy`
  `0x35a388dff9f1bdc7f2941382b52011af5330d81f86d91e9abeb667a5891db6bf`
  (funded at block 4420), `deliver` inside the window, `receive` verified
  (`/tmp/fod-v4/buyer2.vault`).
- `prepare-buy` for order 2 printed the chain-bound check `checked 1/1 …
  the advisory's CIDs match the bundle's committed inventory`.

The open leg is the buyer's own settlement. `confirm` was prepared from the
buyer's checked bytes and broadcast twice: order 1
`0x74971550cc53d67f079e505edbd2317061d59ee87330119fd446c9b3c6a6e13c`
(nonce 2) and order 2
`0xbcf2da3c6b93676e0a4f803f2093a1990ef3f3a376b12c779a033bd04dac9712`
(nonce 2, about eight seconds after funding). Both sit in the mempool on both
nodes (`mempoolNonce` 2) and are never included: the proposer's `filterTxs`
(`blockchain/blockchain.go`) silently skips a transaction whose
`applyTxOnState` fails, and all later blocks are empty for those senders with
no `invalid block roots`, blacklist entry or execution error in
`datadir/logs/output.log`. The same node and module include `buy`, `deliver`
and `refund`, and the same wasm's `confirm` passes in the harness
(`escrow_pool_test.go:161`, `escrow_d0_regressions_test.go:279`,
`escrow_withdraw_test.go:78`), so the failure is specific to `confirm` on the
real host. Filed as **AH-18** with the handover note H-6: identifying the
failing host call needs a lab node build that logs the `filterTxs` error (or
an equivalent host-level replay against the live pool state).
Resolved the same day - the failure was an underpriced `confirm`, not a host
gap; see the AH-18 resolution entry above.

Files: none changed by the sweep; artefacts under `/tmp/fod-v4/`
(`deploy-receipt.json`, `registration-v4.json`, `market-now.json`,
`prepared-*.json`, `submitted-*.json`, `buyer*.vault`).

Receipts and measured latency, read back from the chain: order 1's buy
`0xf04d9ead…` is in block 4386 and the desk's deliver `0x0075a68a…` in block
4388 (two blocks, 40 s); order 2's buy `0x35a388df…` is in block 4420 and the
desk's deliver `0x155eb604…` in block 4421 (one block, 20 s). Order 3 (buy
`0xe1d56c43…`, funded in epoch 26) fell into the validation ceremony: the
desk retried `deliver` fifteen times at five-second intervals and the node
refused each at submission (`tx can't be accepted due to validation
ceremony`), the window then closed, and the order was repaid by the
permissionless refund `0x3e2eb74e…` submitted from a different key - the
bounded exit working on the live chain.

The two remaining v4 legs then landed. They were prepared while the funder's
blacklisted out-of-gas confirm still occupied its epoch-27 nonce slot (the
pool keeps an unexpired tx until the next ceremony drops epoch-27 entries,
`core/mempool` `removeInvalidTxs`), and both were re-signed and submitted the
moment epoch 28 opened:

- `withdraw` (the host's 1 iDNA credit): tx
  `0xd24d10e4d17d99d06d64896eeb5e488324d04cdc195b0eba9db76c3788379eb8`,
  nonce 1, epoch 28, block `0x96a2acba…`, `usedFee` 12.2;
- `withdrawReserve` (the funder's 200 iDNA donation, `AH-15`): tx
  `0x7b91a99183b17509e1f33514edd0e882bef79706b1abdd5e15727e83ae39c779`,
  nonce 2, epoch 28, block `0x0462cc56…`, `usedFee` 12.9.

`contract_estimateCall` had pre-flighted both against the live state
(`withdraw` 3,724 user gas; `withdrawReserve` 4,226), and the market after the
second receipt reads `reserve` 0, `pending` 0, credit 0 and contract balance
0 - every liability settled and the donor exit drained by its owner with no
operator involvement. The reserve row in `fod fees` now carries the measured
422,600 wasm / 129-byte envelope instead of the placeholder borrowed from
`withdraw`.

### 2026-09-30 - H/P5b - the buyer's own ciphertext settles, and the fork domain is a stated limitation

Package H, P5b (AH-10, AH-14), which closes the audit-hardening package.

`confirm` no longer compares a claimed digest with the order's single mutable
delivery slot. The calldata is the order id followed by the exact bytes the
buyer's own tool checked; the escrow records those bytes as the delivery and
settles, so a payload an outsider replaced after the check cannot strand the
payment (`AH-10`). The buyer's record `fod receive -out` now carries
`ciphertextHex` next to the digest, the CLI checks the pair and puts the bytes
in the call, and the interface's confirm request carries both so the CLI can
make the same check. `TestD0JunkFirstDeliveryDoesNotOwnTheSlot` now drives the
attack - junk, honest, replacement, then the buyer's own confirmation - and
asserts the settled payload is the buyer's checked copy.

The fee table's `confirm` row records the new reference envelope (order id plus
the lab's 219-byte payload = 350 signed bytes) and carries the higher of the two
live digest-form gas receipts with a note saying so until a live confirm under
the current ABI is measured.

`AH-14` is closed by acceptance, as `SPEC.md` 10.5 allows and H-5 in section 7
now states: the delivery journal still has no network or fork domain, so a
clone with a matching contract address and order state could accept a delivery
proof. The fix is a `NETWORK_DIGEST` field checked by `fulfill`, which changes
the guest program, its pinned image id in `contracts/escrow/src/verifier.rs`
and `tools/fod/receipt_claim.go`, and the delivery-proof record in the release
manifest; it is scheduled with the mainnet-preparation pass because regenerating
the proof and the pinned identifiers belongs with that release exercise.

Files: `contracts/escrow/src/lib.rs`, `contracts/escrow/README.md`,
`spikes/harness/{escrow_d0_regressions,escrow_pool,escrow_pool_sequence,escrow_withdraw,escrow_beacon}_test.go`,
`tools/fod/{pool_receive,prepare_confirm,fees}.go` and their tests,
`app/src/views/Trades.tsx`, `app/verify.mjs`, `docs/SPEC.md` 4.3/7.1/7.2 and
10.5, `output/cli/prepared-deploy.json` (regenerated),
`release/MANIFEST.json`.

Commands and results: `contracts/escrow` 15 passed; the test-hooks module was
rebuilt and `spikes/harness` passed (17.6 s) with the new confirmation
regression; `tools/fod` passed (15.4 s); `cd app` typecheck clean, `npm run
build` wrote 293,244 bytes `88110076…`, `npm run verify` ALL CHECKS PASSED;
`node release/check-integration.mjs` ALL CHECKS PASSED (13 checks); `node
release/build-manifest.mjs` recorded interface 293,244 bytes `88110076…` and
module 380,347 bytes `0769bbf0…`, 0 forbidden operators, 0 remote tags.

### 2026-09-30 - H/P5a - one key-pin encoding, from the form to the browser check

Package H, AH-8. The connection file's `publicKeyHex` is now the gateway's own
published form: the 44-byte SPKI DER Ed25519 key as 88 hex characters, exactly
what `GET /provider/key` serves and what the browser imports with
`crypto.subtle.importKey("spki", …)`. The old parser accepted 64 hex characters
that the browser could never import, so a genuine pin was refused while an
unpinned connection silently trusted the served key. The gate-setup form gained
a "Gateway key" input: whatever the operator pastes is written into the
generated connection, and the walk now fetches the live key, fills the field,
downloads the connection and asserts the pin matches. The walk's substituted-key
half pins a wrong 88-hex key and still reads "not verified".

Files: `app/src/provider.ts`, `app/src/views/GateSetup.tsx`, `app/verify.mjs`,
`docs/SPEC.md` (connection-file table), `release/MANIFEST.json`.

Commands and results: `cd app` typecheck clean, `npm run build` wrote 293,002
bytes `f4611700…`, `npm run verify` ALL CHECKS PASSED including the new pin
check; `node release/build-manifest.mjs` recorded interface 293,002 bytes
`f4611700…` and module unchanged 380,293 bytes `07de1235…`.

Remaining limitation: a connection with no pin still verifies against the key
the host serves at that moment, which the file's own note and the walk's first
half make explicit; AH-10 and AH-14 remain open.

### 2026-09-30 - H/P4 - loopback by default, a donor-owned reserve exit, and the release check runs the walk

Package H, P4 slice (AH-9, AH-15, AH-16). The gateway config's `host` defaults
to `127.0.0.1` (a network-facing bind is now an explicit config field), the
utility binds loopback unless `--host` names another interface, and both still
serve plain HTTP only where the README says TLS is required.

`addReserve` deposits are no longer anonymous and irreversible: each is recorded
against the address that paid it, and the new `withdrawReserve` returns that
donor's own contribution - the reserve's only outflow. It refuses unless the
reserve still covers the credit and the contract's balance still covers every
buyer escrow and host credit, so a withdrawal cannot reach another party's
money; `accounting` now publishes `callerReserve`. The CLI gained a
`withdraw-reserve` mode (request file, `fod.prepared-withdraw-reserve`), and the
export-surface test lists the new entry point.

`release/check-integration.mjs` now runs the offline interface walk first and
reports it as a check, so a missing build or a missing browser dependency fails
the release check instead of being skipped; the command list in
`release/README.md` says the walk runs there (13 checks).

Files: `gateway/src/config.ts`, `gateway/src/main.ts`, `gateway/README.md`,
`gateway/test/gateway.test.ts`, `utility/src/main.ts`, `utility/README.md`,
`contracts/escrow/src/lib.rs`, `contracts/escrow/README.md`,
`spikes/harness/escrow_reserve_test.go` (new),
`spikes/harness/escrow_no_admin_test.go`, `tools/fod/refund_withdraw.go`,
`tools/fod/main.go`, `tools/fod/commands.go`, `tools/fod/submit.go`,
`tools/fod/fees.go`, `tools/fod/fees_test.go`,
`tools/fod/refund_withdraw_test.go`, `release/check-integration.mjs`,
`release/README.md`, `output/cli/prepared-deploy.json` (regenerated against the
new module), `docs/SPEC.md` 7.1/7.4, `release/MANIFEST.json`.

Commands and results: `gateway` typecheck clean and `npm test` 35 passed;
`utility` typecheck clean; `contracts/escrow` 15 passed; the test-hooks module
was rebuilt and `spikes/harness` passed (5.7 s) with the new reserve
regression; `tools/fod` passed (15.3 s); `node release/check-integration.mjs`
ALL CHECKS PASSED (13 checks, walk included); `node release/build-manifest.mjs`
recorded interface 292,636 bytes `2f7a66cb…` and module 380,293 bytes
`07de1235…`, 0 forbidden operators, 0 remote tags.

Remaining limitation: the reserve-withdrawal fee figure is carried from the
measured `withdraw` call until a live reserve withdrawal is measured, and the
interface has no reserve-donor screen yet (the CLI mode and a hand-written
`withdrawReserve` action request are the current path). AH-8, AH-10 and AH-14
remain open.

### 2026-09-30 - H/P3 - the utility claim proves who is asking, and its quota knows what it counts

Package H, P3 (AH-6, AH-7). The utility's `/access` no longer treats a
provider-signed result as a bearer credential when the result names an address:
`POST /access-challenge` issues
`fod-claim:v1:<utility>:<address>:<nonce>:<issuedAt>`, the claimant signs it with
`personal_sign`, and the server verifies the signature with `viem` and consumes
the nonce once before `decideAccess` sees a verified claim. The challenge is
bound to the utility id as well as the address and nonce, so it cannot be moved
between utilities. A result that names an address without a verified claim is
refused as `claim_control_required`; a consumed or expired challenge is
`unknown_nonce` or `stale_challenge`.

The local quota now has an explicit kind: a wallet-named result is keyed by
`(utility, provider, wallet)` so re-enrolling under a new participant id cannot
reset it, and a result without an address stays keyed by the provider-issued
participant id. `GET /policy` publishes `quota.keyedBy`. A new optional
`--grants FILE` store writes the live entries 0600 and atomically and reloads
them at startup, so a one-time benefit survives a reconstruction; without the
flag the store stays in-memory and says `durable: false`.

Files: `utility/src/claims.ts` (new), `utility/src/access.ts`,
`utility/src/server.ts`, `utility/src/main.ts`, `utility/package.json` (the
`viem` dependency the repo already uses elsewhere), `utility/README.md`,
`utility/test/access.test.ts`, `app/src/views/Session.tsx` (the claim step),
`app/src/faq.ts`, `release/check-integration.mjs`, `docs/SPEC.md` 8.3,
`release/MANIFEST.json`.

Commands and results: `cd utility` typecheck clean and `npm test` 19 passed,
including the endpoint challenge/replay regression, the wallet-scope
regression and the reconstruction regression; `cd app` typecheck clean,
`npm run build` wrote 292,636 bytes `2f7a66cb…`, `npm run verify` ALL CHECKS
PASSED (the browser signs the utility challenge with its wallet); `node
release/check-integration.mjs` ALL CHECKS PASSED (12 checks, each customer
claiming through a fresh challenge); `node release/build-manifest.mjs` recorded
the interface at 292,636 bytes `2f7a66cb…` and the module unchanged at 377,217
bytes `7ea3983c…`, 0 forbidden operators, 0 remote tags.

Remaining limitation: the challenge proves control of the address at claim
time; the provider's scoring behind the result is still a provider statement
(H-3), and a local grant file still cannot decide a limit advertised as global.
AH-8, AH-9, AH-10, AH-14, AH-15 and AH-16 remain open.

### 2026-09-30 - H/P2b - the ordinary buy path fetches and checks the content before it is signable

Package H, P2b (AH-17). `prepare-buy` no longer has an unchecked mode: without
`-availability FILE` it refuses, and with one it still requires the recomputed
CID commitment to equal the inventory commitment the registration statement
published and every CID to fetch from the buyer's own node and rebuild itself.
The signing side closes the other half: `loadPreparedAction` refuses a
`fod.prepared-buy` whose availability check is missing, not a complete
chain-bound check, unreadable, or older than fifteen minutes, and the CLI's
prepare-buy usage now says the advisory is required. The fifteen-minute bound
is an observation window, not a delivery guarantee: the protocol cannot promise
that a peer keeps serving, which is why the on-chain outcome stays "usable
content settles inside the delivery window, otherwise the buyer is repaid".

Files: `tools/fod/pool_buy.go`, `tools/fod/submit.go`,
`tools/fod/availability_check.go`, `tools/fod/main.go`,
`tools/fod/pool_buy_test.go`, `tools/fod/submit_test.go`,
`tools/fod/availability_check_test.go`, `app/src/faq.ts`, `docs/SPEC.md` 5.8
and the command table, `release/README.md`, `release/MANIFEST.json`.

Commands and results: `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test
-count=1 ./...` ok (15.1 s), including the new regressions
`TestSubmitRefusesABuyWithoutAFreshAvailabilityCheck` and the request-driven
refusal; `cd app` typecheck clean, `npm run build` wrote 292,051 bytes
`5e8ac2f4…`, `npm run verify` ALL CHECKS PASSED; `node release/build-manifest.mjs`
recorded the interface at 292,051 bytes `5e8ac2f4…` and the module unchanged at
377,217 bytes `7ea3983c…`, 0 forbidden operators, 0 remote tags.

Remaining limitation: a check made at signing time cannot prove that the bytes
will still be served during the delivery window; the hard deadline and the
permissionless refund remain the enforcement for that gap. AH-10 and AH-14 are
the remaining P2 items.

### 2026-09-30 - H/P2a - the repayment boundary closes every seller path, and a revealed bundle stops selling

Package H, P2 slice: AH-11, AH-12 and AH-13 are implemented; AH-10, AH-14 and
AH-17 remain open.

`fulfill` and `reveal` now carry the same hard boundary as `deliver`: once
`block_number() > funded_at + DEADLINE_BLOCKS`, they abort with
`deadline_passed` before the proof or the secrets are parsed, so the block that
opens the refund is refund-only and no seller path can race the buyer's
guaranteed repayment. `TestPoolUnilateralSettlementsCloseAtTheDeadline` pins a
settling reveal at `funded+3` and aborting reveal/fulfill plus a successful
refund at `funded+4`; `TestPoolRevealSettlesWithoutTheBuyer` now reveals inside
the window and records why.

`donateUnit` refuses a listing with `strikes >= 2`, so a cleared author cannot
dilute the one global quote with unbuyable inventory;
`TestPoolClearedListingRefusesDonations` drives two refunds to two strikes and
asserts the refused donation moves neither the bundle nor `unitsTotal`.

`reveal` now records the published bundle under `pool-revealed/<commitment>`,
refuses later buys and donations (`bundle_revealed`) and removes the bundle's
remaining units from the one global count, while orders already funded keep
their settlement and refund paths. `TestPoolRevealStopsTheListing` pins the
zeroed counts, the refused buy and donation, and a second pre-funded order that
still closes; `TestPoolConcurrentOrdersOnOneBundleStaySeparate` now ends with
`unitsTotal` 0 because the reveal retired the last unsold unit.

Files: `contracts/escrow/src/lib.rs`, `contracts/escrow/README.md`,
`spikes/harness/escrow_instant_test.go`, `release/MANIFEST.json` (regenerated).

Commands and results: `cd contracts/escrow && cargo test --locked --offline`
15 passed; the test-hooks module was rebuilt (`dist/fod_escrow.wasm`, 383,379
bytes) and `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test
-count=1 ./...` passed (6.2 s); `cd tools/fod` the same way passed (15.2 s);
`node release/check-integration.mjs` ALL CHECKS PASSED (12 checks); `node
release/build-manifest.mjs` recorded the release module at 377,217 bytes
`7ea3983c…` with 0 forbidden operators and 0 remote tags, and the interface
unchanged at 291,916 bytes `b080d848…`.

Remaining limitation: a seller who delivers but reveals after `funded+3` cannot
settle and the buyer is repaid. That is the accepted hard-deadline policy,
stated in `contracts/escrow/README.md`; AH-17 still leaves the buyer able to
sign a purchase without an authenticated availability check, and AH-10/AH-14
stay open.

### 2026-09-30 - H/P1 - the session gate closes its answer oracle and binds the proof of control

Package H, P1 (AH-1 to AH-5). The gateway no longer returns a per-answer
correctness signal; `/session/result` signs only after `closesAt`, because the
score line was the same oracle through a second door. Enrollment now requires
a wallet binding and a one-time server nonce (`fod-enroll:v2:<provider>:<session>:<address>:<nonce>:<issuedAt>`),
is refused for an address with a live enrollment (`address_enrolled`, 409) and
for a full table (`session_full`, 503), and a public lobby read only expires
records instead of evicting a live participant. The session digest commits to
a per-session `answerSalt`; the open policy serves neither the salt nor any
answer digest, and the closed policy publishes the preimage as `contentReveal`,
which recomputes to the pinned `contentDigest`. Rate limits charge the nearest
untrusted hop only when the socket peer is a configured `trustedProxies` entry
(gateway `config.trustedProxies`, utility `--trusted-proxy`); otherwise the
forwarded header is ignored. `fod session-pool` draws the salt (32 bytes,
`crypto/rand`) or accepts a pinned one for deterministic fixtures;
`gateway/fixtures/cli-pool.json` and `gateway/example-pool.json` carry theirs.

Files: `gateway/src/{config,http,pool,routes,session,wallet}.ts`,
`gateway/test/gateway.test.ts`, `gateway/README.md`, `utility/src/{main,server}.ts`,
`utility/test/access.test.ts`, `utility/README.md`,
`app/src/provider.ts`, `app/src/views/Session.tsx`, `app/verify.mjs`,
`tools/fod/session_pool.go`, `tools/fod/session_pool_test.go`,
`release/check-integration.mjs`, `app/package.json` (the `playwright`
verification dependency, AH-16's first half).

Commands and results: `gateway` typecheck clean, `npm test` 34 passed;
`utility` typecheck clean, `npm test` 15 passed; `tools/fod`
`GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` ok (15.2 s);
`spikes/harness` ok (5.5 s); `app` typecheck clean, `npm run build` wrote
291,916 bytes `b080d848…`, a second build reproduced the hash, `npm run verify`
ALL CHECKS PASSED (the walk runs its own short window, checks the submitted
chips carry no correctness, collects the result only after close, and runs the
wrong-key half on a second gateway); `node release/check-integration.mjs` ALL
CHECKS PASSED, including "the closed session reveals a preimage that recomputes
to the pinned digest"; `node release/build-manifest.mjs` (dirty worktree)
recorded interface 291,916 bytes `b080d848…` and module unchanged 373,223 bytes
`a6764bc7…`, 0 forbidden operators, 0 remote tags.

Remaining limitation: the salt reveal is served by the same gateway that
scored; the utility's pin binds the digest, and an operator can recompute the
reveal, but the adapter does not fetch it. AH-6/AH-7 still leave the claim
path without a claimant-control proof. AH-14's domain-free delivery journal and
the rest of package H stay open.

### 2026-09-29 - G3 partial - the shared-node connection and custody boundary

The reference gateway now publishes `GET /shared-node/status` with the node id,
session id, utility id, pool provenance, the operations it serves (`session`,
`pool`) and the custody modes it supports (`self`, `hosted`). The Use flips view
imports a `fod.shared-node` v1 connection, validates the node URL, author
address and custody choice, fetches the status, and shows the self/hosted
custody boundary plus the author's export/prepare-offer/prepare-list/serve
steps. It also generates a `fod.shared-node` file from the node id, URL, author
address and custody choice.

Evidence: gateway `npm test` 30 of 30 adds "publishes the shared-node status
and its custody modes"; app typecheck clean; `npm run build` wrote the
interface at 291,609 bytes `4e37c406…`; `npm run verify` ALL CHECKS PASSED,
including "the shared-node journey states the node status and the custody
boundary" and "the shared-node file names the node, author and custody"; a
second build produced the same `4e37c406…` hash; `node
release/build-manifest.mjs` regenerated the manifest (interface 291,609 bytes
`4e37c406…`, module unchanged 373,223 bytes `a6764bc7…`, 0 forbidden
operators, 0 remote tags); `node release/check-integration.mjs` ALL CHECKS
PASSED.

Remaining G3 work: the hosted node's actual export/session-pool/listing/
delivery operations and the author's vault integration are not built. The
custody boundary is explicit: hosted means the node holds the author's epoch
vault and can deliver; self-hosted keeps the keys with the author. In both cases
the escrow pays the registered author, so the node cannot redirect the
proceeds. That hosted-operation work is the next G3 slice.

### 2026-09-29 - G2 partial - the shared on-demand lobby

The reference gateway now publishes the shared window a provider opens on
demand: `GET /session/lobby` answers with the window state, the participant
count and the challenge count. The Use flips view polls it while a policy is
loaded and shows "Session window", "Participants" and "Challenges"; the Start
session button stays disabled until the window is open.

Evidence: `gateway/test/gateway.test.ts` adds "publishes the shared lobby and
gives two participants the same challenges" - the lobby reads `window: open`
and two challenges, two enrolled participants receive byte-identical challenge
assignments, and the participant count rises by two. Gateway typecheck clean,
`npm test` 29 of 29. `cd app && npm run typecheck` clean; `npm run build` wrote
the interface at 285,634 bytes `7d0f9ffa…`; `npm run verify` ALL CHECKS PASSED,
including "the on-demand lobby shows the shared window and participants"; a
second build produced the same `7d0f9ffa…` hash; `node
release/build-manifest.mjs` regenerated the manifest (interface 285,634 bytes
`7d0f9ffa…`, module unchanged 373,223 bytes `a6764bc7…`, 0 forbidden
operators, 0 remote tags); `node release/check-integration.mjs` ALL CHECKS
PASSED.

Blocked part: the real epoch-228 pool is not built. The capture holds the
session bundle and the five encrypted blobs, but no epoch secrets - the
epoch-228 key census is empty by design. The captured
`inventory-ai-228.json` is complete (`complete: true`, 5 submissions, 5 unique
CIDs, 5/5 blobs verified), so the only missing input is the seller's derived
vault. On the seller's own machine (identity `0xace93bd0…`), the exact sequence
is:

```bash
# 1. Derive the epoch secrets from the seller's nodekey; the key never leaves
#    this machine.
fod export-keys -nodekey <seller nodekey> -inventory-epoch 228 \
  -vault seller-228.age

# 2. Turn the captured inventory and blobs into the session-content export.
fod export \
  -inventory output/capture/epoch-228/inventory-ai-228.json \
  -vault seller-228.age \
  -blob-dir output/capture/epoch-228/blobs-ai \
  -out /tmp/fod-export-228

# 3. Build the on-demand pool with the opens/closes window the provider wants.
fod session-pool -export-dir /tmp/fod-export-228 \
  -out pool-228.json \
  -session-id fod-228-<timestamp> \
  -utility-id <utility id> \
  -threshold <answers needed to pass> \
  -opens-at <RFC3339 now> -closes-at <RFC3339 close>
```

Then point the gateway's `poolPath` at `pool-228.json`, start the gateway and
the utility, and share the connection file; the lobby and the two-participant
assignment are already implemented. That is an owner-provided input; the
read-only capture cannot substitute for it. The two-participant part is not
blocked: participants solve the same pool without owning the flips.

### 2026-09-29 - G1 - the EVM address is bound to the session and the grant

The Use flips view connects an EIP-1193 wallet, signs a session-bound EIP-191
challenge, and sends the address, challenge and signature with the enrollment.
The gateway verifies the signature with `viem`, refuses a stale challenge or a
mismatched address, stores the address on the participant, and includes
`participantAddress` in the signed result. The utility accepts the field,
includes the address in the local quota key and the grant, and keeps the
grant's `identity` at `not-verified` because an address is a key, not a person.

| Surface | Change |
|---|---|
| `gateway/src/wallet.ts` | new EIP-191 challenge check: `fod-enroll:<sessionId>:<address>:<unix-ms>`, a five-minute freshness bound, and `verifyMessage` from `viem` |
| `gateway/src/session.ts`, `gateway/src/routes.ts` | the enrollment binds the checked address; the signed result carries `participantAddress` |
| `utility/src/results.ts`, `utility/src/access.ts` | the optional address is parsed, added to the local quota key and shown in the grant |
| `app/src/provider.ts`, `app/src/views/Session.tsx` | the wallet connect/sign flow and the bound-address display in the result and the grant |
| `gateway/test/gateway.test.ts`, `app/verify.mjs` | gateway tests for a good binding, a bad signature and a stale challenge; the browser walk uses a mock EIP-1193 wallet and checks the address in the result and the grant |
| `THIRD_PARTY.md`, `gateway/README.md` | the unvendored `viem` MIT dependency and the wallet-binding behavior are recorded |

Evidence: gateway `npm run typecheck` clean and `npm test` 28 of 28; utility
`npm run typecheck` clean and `npm test` 14 of 14; app `npm run typecheck`
clean; `npm run build` wrote the interface at 284,854 bytes `35623c6b…`;
`npm run verify` ALL CHECKS PASSED, including "the signed result names the
bound EVM address" and "the utility grant names the bound EVM address";
`node release/build-manifest.mjs` regenerated the manifest (interface 284,854
bytes `35623c6b…`, module unchanged 373,223 bytes `a6764bc7…`, 0 forbidden
operators, 0 remote tags); `node release/check-integration.mjs` ALL CHECKS
PASSED.

Limits: the address is a key, not a person; the utility's scope kind stays
`provider-participant`, with the address added to the local quota key, so a
wallet-only quota is not one-person-one-allocation. The production utility's
provider-trusted scoring remains H-3. G2 (shared on-demand window), G3
(shared-node supply) and G5 (payment/custody) stay open.

### 2026-09-29 - G4 - the testbed buyer claims the utility grant in the browser

The Use flips example gate now completes the buyer testbed: the reference
utility runs beside the gateway, the browser posts the verified provider result
to the utility's `/access`, and the grant is shown in the page.

| Surface | Change |
|---|---|
| `app/src/provider.ts`, `app/example-provider.json` | the connection file carries an optional `utilityUrl`; the example points at the reference utility on 8788 |
| `app/src/views/Session.tsx`, `app/src/views/GateSetup.tsx` | the verified passing result offers "Claim test access"; the page shows the utility's granted/refused decision and the scope; the setup generator writes the same `utilityUrl` |
| `utility/src/server.ts` | the public endpoint sends the CORS headers and answers `OPTIONS`, because the decision is a signature check and the testbed browser calls it directly |
| `app/verify.mjs` | starts the reference utility with a policy that pins the gateway key and session digest, clicks "Claim test access", and checks the grant; the remote-request check allows the utility origin |

Evidence: `cd app && npm run typecheck` clean; `npm run build` wrote the
interface at 283,304 bytes `382e639c…`; `npm run verify` ALL CHECKS PASSED,
including "the test utility grants access for the passing result"; `node
release/build-manifest.mjs` regenerated the manifest (interface 283,304 bytes
`382e639c…`, module unchanged 373,223 bytes `a6764bc7…`, 0 forbidden
operators, 0 remote tags); `node release/check-integration.mjs` ALL CHECKS
PASSED.

Limits: this is the reference testbed utility, not a production utility; the
grant is scoped to the provider-issued participant id, which is not a person,
and the provider-trusted scoring of the result remains H-3. The EVM
address-bound enrollment and the shared-node author path are Package G1/G3 and
stay open.

### 2026-09-29 - E - the customer-facing Use flips gate journey

The Session view is now the customer-facing **Use flips** gate and carries the
operator setup journey:

| Surface | Change |
|---|---|
| `app/src/views/GateSetup.tsx` | new: the faucet and dapp recipes, the five-step provider path, the exact `fod session-pool`/gateway/utility commands, the provider/utility/threshold inputs, and the connection/gateway-config downloads |
| `app/src/views/Session.tsx` | the built-in "Try the example gate" needs no connection file, the heading and example note read "Use flips", and the setup section is always available |
| `app/src/main.tsx` | the nav label is "Use flips" (the `#/session` route is unchanged) |
| `app/src/views/Trades.tsx` | a keys-ready order links to the flips journey with "Use these flips" |
| `app/src/faq.ts`, `docs/SPEC.md` | the buy and gated-access answers and the view/journey tables point at the in-app example gate and the faucet/dapp setup journey |
| screenshots | `output/playwright/use-flips-example.png`, `gate-setup.png`, `live-trades.png` |

Evidence: `cd app && npm run typecheck` clean; `npm run build` wrote the
interface at 281,712 bytes `44278fca…`; `npm run verify` ALL CHECKS PASSED,
including the example gate without a connection file, the setup journey naming
the faucet/dapp recipes and the pool command, and the keys-ready order linking
to the flips journey; `node release/build-manifest.mjs` regenerated the manifest
(interface 281,712 bytes `44278fca…`, module unchanged 373,223 bytes
`a6764bc7…`, 0 forbidden operators, 0 remote tags); `node
release/check-integration.mjs` ALL CHECKS PASSED.

Limits: the built-in example gate needs the reference gateway at
127.0.0.1:8787; the keys-ready link opens the gate view and leaves the actual
vault/pool build to `fod session-pool`, so no browser holds the buyer's keys.
The utility's provider-trusted scoring is still the H-3 handover.

### 2026-09-29 - mainnet epoch-228 ceremony capture collected and verified (owner request)

The host script reached `capture done` at 15:36:16Z. The read-only copy and the
post-ceremony censuses are now in `output/capture/epoch-228/`. No contract was
deployed and no transaction was sent; the designated test host was only read.

| Artefact | Value |
|---|---|
| session bundle | `session-bundle-ace93bd0-11372565-11372566.json`, sha256 `dda8d6fc…`: author `ace93bd0…`, epoch 228, five manifest CIDs, inventory height 11372565 root `0ef688d1…`, verdict height 11372566 root `7b9c2814…`. The five CIDs equal the five fetched in `inventory-ai-228.json`, and the accepted pair passed the script's anchor rule; the refused candidates whose verdict record still listed flips are in `capture.log` (sha256 `9c1dbbc9…`). |
| ceremony sweep | `sweep-ceremony-228.json`, sha256 `e9aa8a6e…`: 591 blocks (11371975-11372565), 545 transactions, 108 `submitFlip`, 117 `submitShortAnswers`, 117 `submitLongAnswers`, 0 deletions, 0 missing tx bodies or hashes, 0 body-tx-set mismatches. No IPFS body source was used in this sweep, so `bodiesCidVerified` is 0 and the CID-verification claim stays with the earlier epoch sweep. |
| censuses | `epoch-db-228.json` and `epoch-db-228-post.json`, sha256 `25faed28…`; `epoch-db-229.json`, sha256 `33cf518b…`; `epoch-keys.json` and `epoch-keys-post.json`, sha256 `84409250…`; all read empty (`seed_bytes=0 candidates=0 assigned_flips=0 states=map[]`, `totalEpochKeys 0`). `versions.json`, sha256 `5a25149d…`, lists the retained versions 11372470-11372569 and reports the height-0 probe `verified=false`. |
| commands | copy: `scp <test host>:/root/fod-session-228/{session-bundle-…,epoch-db-228.json,epoch-keys.json,sweep-ceremony-228.json,versions.json,listing.txt,sb.out,capture.log} output/capture/epoch-228/`; censuses: `./fod epoch-db -datadir /root/fod-session-228/datadir -inventory-epoch 229 -out …/epoch-db-229.json`, the same for epoch 228, and `./fod epoch-keys -datadir … -out …/epoch-keys-post.json` |

Limitations, stated plainly. The second identity `1af48202…` submitted no flips
in epoch 228, so no valid anchor pair exists and no bundle was written for it.
That identity can still take the buyer role; a session bundle is only the
seller-side artifact.
The copy was taken 45 seconds after epoch 229 started, so the boundary had
already cleared the epoch-228 lottery and key store, and epoch 229's lottery
store was not populated yet: the censuses are empty. This capture proves the
flip membership, the manifest, the verdict and the ceremony transactions for
`ace93bd0…`. The empty key census does not block a later sale:
`DeriveEpochKeys` reproduces the node's derivation from the identity private key
and the epoch number alone (`tools/fod/keys.go:47`), pinned against the
node-built blob vector by `TestTheCliOpensTheNodeBuiltBlob`; `fod export-keys
-nodekey … -inventory-epoch 228 -vault …` writes those secrets into an
encrypted vault, and `fod prepare-offer` derives the same pair, so the
epoch-228 secrets are recoverable locally by the identity holder without the
node's epoch store.

Blocker resolved: the "mainnet epoch-228 ceremony capture" entry in the final
acceptance report is now collected and recorded. The other three blockers
remain: mainnet wall-clock costs, second-host operator independence, and the
utility's provider-trusted scoring.

### 2026-09-29 - E - the refund watch has a stubbed CLI regression

The entry `E - the buyer's cost and time display, and the automatic refund
watch` named one limitation: the watch had only the live lab run behind it, with
no deterministic CLI regression. This closes that limitation.

| Surface | Change |
|---|---|
| `tools/fod/watch.go` | `runWatch` and `submitWatchRefund` return an error instead of calling `os.Exit` inside the loop, so the real RPC path is callable from a test; `tools/fod/main.go` keeps the exit-2 flag check and reports a runtime failure with exit 1 |
| `tools/fod/watch_test.go` | a stubbed node answers `bcn_lastBlock`, `contract_readData`, `dna_getBalance`, `dna_epoch`, `bcn_feePerGas`, `bcn_sendRawTx` and `bcn_txReceipt`; one test reaches the refund block with a delivered order and asserts exactly one transaction is sent, and a second leaves a settled order alone |

Evidence: `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...`
- ok (14.8 s), including the two new regressions; `go vet ./...` clean. The CLI
is not a hashed release artefact, so the manifest, HTML and module hashes stand.

### 2026-09-29 - D2/D3 - the escrow's own README matches the shipped rules

`contracts/escrow/README.md` still described the old model: a 720 + 120 block
refund that *credits* the buyer, a buy that moves the quote *into the reserve*,
and reserve-funded supply as temporarily disabled. A reader of the contract's
own document would have taken away the wrong deadline, the wrong payout path and
the wrong supply rule. It now states the shipped surface: `reveal` in the demand
list, one global pool with `PoolBundle` keeping only inventory, commitments,
beacon, strikes and the buy nonce, no subsidy (`sellUnit` compiled off,
author-only `donateUnit`), the buy putting the quote plus the capped retrieval
fee into pending escrow and never the reserve, settlement paying the registered
author and the named hosts, the three-block window with the one-transaction
permissionless refund at `funded+4`, and the `accounting` invariant
`liquid == reserve + pending + credits`. Documentation only: no artefact
changed, so the manifest, hashes and clean rebuild stand.

### 2026-09-29 - E - the view list and journeys match the implemented interface

| Surface | Change |
|---|---|
| `SPEC.md` 4.2 | the seven-view design table is replaced by the four implemented views (Market, My trades, Session, FAQ), with the merged orders/bundles spelled out, the Session view described as the provider integration whose pool the companion builds (`fod session-pool`), and the limit stated plainly: a browser-local session with no provider process is not implemented |
| `SPEC.md` 4.6 | the buy journey ends at My trades' keys-ready state, and the use-the-keys journey names the companion's `fod session-pool` and the provider-signed result the local utility decides on, replacing the "My keys" screen that never shipped and the "scores them locally" wording that misdescribed the division of labour |
| `app/verify.mjs` | the browser walk now asserts that the delivered order shows `keys ready` (it already rendered it; the walk pins it), so the buy journey's ready state is covered by the walk the spec requires |
| evidence | `cd app && npm run verify` ALL CHECKS PASSED including the new check; the interface artefact is untouched, so its `b9d50c4f…` hash and the manifest stand |

### 2026-09-29 - verification - a refunded order refuses the committed preimage too

The reveal/refund race is pinned in both directions now:
`TestPoolRevealSettlesWithoutTheBuyer` already showed a reveal settling and the
buyer's refund then refused; it now also refunds an order and afterwards presents
the *correct* committed preimage, which the escrow refuses because the order is
closed. The refund is therefore final whichever side pays first. The full
`spikes/harness` suite passes (4.9 s).

### 2026-09-29 - verification - the live books re-read at the current head

Both lab nodes answer the same market snapshot at the same head: pool `units 1`,
`pending 0`, `strikes 1`; orders 1, 2 and 4 `settled` and order 3 `refunded`;
one host credit of 3 iDNA. The contract's own `accounting` view, read through
`contract_estimateCall` and decoded from `actionResult.outputData`, is identical
on both: `reserveTotal 200e18`, `pendingTotal 0`, `creditTotal 3e18`,
`unitsTotal 1`, `liquid 203e18` - so `liquid == reserve + pending + credits`
exactly, on the live chain, with every order closed. The `callerCredit` field
differs between the two reads only because each node signs the probe with its
own key and the pinned RPC ignores the `from` field, which the fee-table notes
already record.

### 2026-09-29 - E - the buyer's purchased keys open the seller's actual flip

| Step | Evidence |
|---|---|
| buyer vault | `fod receive -pool-order 2 …` had already checked the recorded ciphertext against the bundle's key commitment and written the buyer's owner-only vault |
| content | `fod receive -vault /tmp/fod-new/buyer-order2.vault -blob /tmp/fod-cer/flip-blob-epoch1.bin` decrypted the real epoch-1 flip blob with `0x19e7e376…`: a 68-byte public part and a 90-byte private part, the PNG-prefixed images the renderer reads |
| meaning | the keys the buyer paid for open the seller's actual content, not only a commitment match, so "correct original keys are delivered to the funded buyer" and "the buyer can use the content" have a live run; the private part stayed owner-only on disk and out of this record |

### 2026-09-29 - D3 - the current module's buy, deliver and confirm costs, and the delivered latency

| Item | Value |
|---|---|
| delivery latency | order 2 was funded in block 2496 and read back `delivered` at 2497, so the desk published the buyer-bound ciphertext within one block of funding; the lab's observed cadence was 20 s per block and the desk's pass interval 5 s, so the latency was bounded by the desk's next pass plus one block |
| buy | live receipt `0xa0dbe60b…` on the current module, carrying the retrieval-fee list: `gasUsed` 13,130 user gas / 1,313,000 wasm, `gasCost` 131.30, `txFee` 33.1, signed envelope 331 bytes. The fee table keeps the canonical 292-byte reference (a purchase naming no host) and its guard prices the size actually signed |
| deliver | live receipts `0xf5e36a06…` and `0x1ac30518…` on the current module: `gasUsed` 10,838 user gas / 1,083,800 wasm, `gasCost` 108.38 each |
| confirm | the current module's receipt `0xa6530752…` reads `gasUsed` 14,689, below the 14,747 the table already carried from the previous module, so the higher figure stands |
| re-priced | `fod fees -fee-per-gas 96153846153800` now prices a buy at 1.54 iDNA and a delivery at 1.38 at the recorded mainnet rate; `SPEC.md` 7.2 and `app/src/faq.ts` carry the same figures |
| artefacts | the FAQ edit re-built the interface: 276,502 bytes `b9d50c4f…` with the module unchanged at 373,223 bytes `a6764bc7…`; `release/build-manifest.mjs` regenerated the manifest, `release/check-integration.mjs` and `npm run verify` pass, and a fresh clone rebuilt the interface to `b9d50c4f…` byte for byte |
| tests | `tools/fod` suite ok (14.6 s) |

### 2026-09-29 - F1/F2 - the release README matches the verified state

| Surface | Change |
|---|---|
| `release/README.md` | the status block names the verified play-money target and the receipts in this log instead of the 2026-09-26 review verdict; it states that `SPEC.md` 10.3 supersedes 10.2 and that the deploy smoke-test allowance is suspended, not in force; the stale 20-row definition-of-done table is replaced by a pointer to the living matrix here, so no second table can drift; "what is missing" now lists only mainnet costs (`O-9`), configuration-level node independence, the utility's provider trust and the running capture; the recovery row states the one-transaction refund; and the mainnet-cost paragraph carries the current measured fees and the corrected price flow (escrow to the registered author and the named hosts, returned to the buyer on timeout) |
| effect | documentation only: no artifact changed, so the manifest, hashes and the clean rebuild stand |
| verification recipe | a new "Verify this release" section lists the exact local commands a third party runs - manifest rebuild, artefact hashes, the integration loop, the offline interface walk, the CLI, escrow, harness and sealed-artefact suites, including the test-hooked module build the harness needs - and names the byte-for-byte clean-checkout comparison as the reproducibility check |

### 2026-09-29 - B2/D1 - the served bundle's registration is live on the current module

| Item | Value |
|---|---|
| artefact | the CLI-prepared registration the desk's bundle came from names the current module `0x2cfbbafa…`: key commitment `b08e178b…`, manifest commitment `881b07ea…`, inventory commitment `3f56c06e…`, covered flips 1, author `19e7e376…`, epoch 1, registration image id `7ee79d90…`, claim `eee7aa3f…`, 582-byte journal |
| receipt | `registerBundle` tx `0x5366f8ea…` reads `success true gasUsed 1,958,525 gasCost 19,585.25 txFee 71.4`, so the contract accepted the sealed receipt - its two session anchors checked against the chain's own headers - and wrote the bundle the market shows and every order above settled against |
| tests | on this revision: `contracts/escrow` `cargo test --locked --offline` 15 of 15, `proofs/session-core` 19 of 19, and `proofs/registration-v2/host --test sealed_artefact` 1 of 1 - the sealed artefact verifies against the guest image id this tree builds, so the guest-receipt-contract-CLI tuple holds |

### 2026-09-29 - E - the user-facing FAQ and the spec's fee table match the implemented rules

| Surface | Change |
|---|---|
| `app/src/faq.ts` | the seller-failure answer no longer promises a grace period or a refund credit: delivery is valid for three blocks, the next block opens one permissionless refund that pays the payment and the retrieval fee directly, the first strike clears the beacon and the second clears the listing; the supply answers say only a bundle's registered author contributes a unit, as a gift, and that pool payouts stay off; the costs answer carries the current measured figures (about 1.3-1.6 iDNA for the everyday calls, about 190 for a proof, about 354 for the deploy) |
| `docs/SPEC.md` | 7.2's fee table carries the current measured gas, sizes and mainnet-rate maximum fees, `reveal` and `withdraw` included, and 11.1's deadline row now states the no-grace rule instead of listing a grace period as a scenario |
| evidence | `cd app && npm run build` wrote 276,502 bytes; `npm run verify` ALL CHECKS PASSED; `release/build-manifest.mjs` re-ran (interface `b9d50c4f…`, module `a6764bc7…`, 0 forbidden operators, 0 remote tags); `release/check-integration.mjs` ALL CHECKS PASSED |
| clean rebuild | a disposable `git clone --local` plus `git diff --binary HEAD \| git apply` of the worktree, an empty escrow target and a copied `app/node_modules` rebuilt both artefacts byte for byte at this revision: HTML `b9d50c4f…` (276,502 bytes) and module `a6764bc7…` (373,223 bytes) |

### 2026-09-29 - C2/D3 - the delivery proof settled a live order without the buyer

| Item | Value |
|---|---|
| order | order 4, funded at 2567 (`deadline` 2570), never confirmed and never refunded; the refund window was already open when the settlement landed |
| desk | restarted with `-prover proofs/delivery/target/release/delivery-host` and a 40,000 iDNA budget: it refreshed the strike-zeroed beacon (tx `0xa2f0547d…`, `gasUsed` 5,300), delivered the buyer-bound ciphertext (nonce 7, tx `0x1ac30518…`), and the planner started the fallback as soon as the deadline came within its 280-block margin |
| proof | `proof-4.json` (4,151 bytes) after about sixteen minutes of `r0vm` proving at multi-core load; delivery image id `2a4f2f7b…`, claim digest `17e5db9b…`, 209-byte journal, 692-byte fulfilment argument |
| settlement | `fulfill` tx `0xa082ce99…` reads `success true gasUsed 1,972,769 gasCost 19,727.69 txFee 81.7` identically on both nodes; order 4 is `settled`, the pool reads `pending 0`, and the named host's credit grows to 3 iDNA, one per settled order |
| measured cost | `tools/fod/fees.go` carries the live figure: `fod fees` prices a fulfilment at 19,809.19 iDNA, 38.53% of the node's cap |
| tests | `tools/fod` suite ok (15.8 s) |

Limitation: the proof took about sixteen minutes here, so the desk's proof
margin has to exceed the local proving time, and a buyer who refunds first still
wins the race - that is the contract's rule, not a defect.

### 2026-09-29 - verification - two real orders on one bundle keep separate liabilities

The retrieval-fee scenario list of `SPEC.md` 11.1 now has its repeated-host
case: `TestPoolFeeListRepeatsAHostUnderTheSameCap` names one host twice at half
the covered-count cap each, refuses a list one base unit over the cap, accepts
the capped list, and shows the single host's credit as the sum of both entries
with the reserve still at 100. The full harness suite passes (4.8 s).

The concurrent-order half of the policy is now pinned without any test hook:
`TestPoolConcurrentOrdersOnOneBundleStaySeparate` seeds a three-unit bundle and
makes two real `buy` calls (quotes 50, then 100, each plus the 1 iDNA capped
fee). The pool's `pending` reads exactly both liabilities; the first order
settles by `reveal` with no buyer cooperation (author paid the purchase amount,
named host credited the fee) and leaves only the second order's liability; a
third party refunds the second after its own window and the second buyer is
repaid `quote + fee` exactly. The closing books read `reserve 100`, `pending 0`,
`units 1`, `creditTotal` one fee and `liquid = reserve + credit`, so neither the
reserve nor the settled order paid for the other. The fixture helper gained a
unit-count parameter and computes `ceil(r/(q-1))` for any count; the full
`spikes/harness` suite passes (5.0 s).

### 2026-09-29 - E/D3 - the approved purchase rules ran on the lab chain: fee, reveal, and the refund watch

| Step | Evidence |
|---|---|
| author donation | `donateUnit` from the bundle's registered author lands (tx `0xdfc71b4e…`, `gasUsed` 5,902); `units`/`poolUnits` move 1 -> 2 and the buy quote returns 200 iDNA |
| buyer purchase | order 2's `buy` carries the capped fee list (quote 200 + fee 1 iDNA) and mines at height 2496 (tx `0xa0dbe60b…`); pool `pending 201`, `units` 1 |
| delivery inside the window | the desk published the buyer-bound ciphertext in the same window: order 2 records `funded 2496`, `deadline 2499`, and reads `delivered` at 2497 |
| buyer check | `fod receive -pool-order 2 …`: the recorded ciphertext decrypted to 106 bytes, they hash to the bundle's key commitment `b08e178b…`, and the ciphertext hash `189c2d10…` is what a confirmation names |
| no confirmation | the buyer never confirmed; on the first block after the deadline the author published the committed secrets: `reveal` (tx `0x6fe5a420…`, `gasUsed` 12,681, `gasCost` 126.81, `txFee` 23.5) settles order 2 at 2502-2503 with `strikes 0`; the named host's credit reads 2 iDNA, one per settled order |
| vanished desk | the desk process was stopped; order 3 is funded at 2508 (`deadline` 2511, `refund` 2512) and nothing delivers it |
| automatic refund | `fod watch -order 3` submitted the permissionless refund from a third party's key (the author's, not the buyer's) at head 2512. The automining node kept proposing empty blocks over its own blacklisted first attempt, so - lab-only - both disposable nodes were restarted with their persisted mempools moved aside to `/tmp/fod-cer/mempool-backup-125034/`, and the re-broadcast refund landed: receipt `0xb338bfe2…` reads `refund success true gasUsed 13,579 gasCost 135.79 txFee 12.1` identically on both nodes, order 3 reads `refunded`, pool `pending 0`, `strikes 1`, and the buyer is repaid exactly once, `1020.67 -> 1221.67` (the 200 payment plus the 1 iDNA fee, no credit and no second step) |
| builder diagnosis | `contract_estimateCall` of the same refund against the live state succeeds on both nodes (`success true`, `gasUsed 13,579`, `gasCost 135.79`), so the skip was the lab node's mempool/blacklist behaviour, not a contract refusal |
| defects the live run found | `fod watch` was not registered as a command and passed the key's address to `dna_getBalance` without the `0x` prefix (both fixed in `commands.go`/`watch.go`); it also computed the refund's max fee from the fee table's reference size and refused its own one-byte-larger signed envelope - it now asks for the calldata's own bytes plus headroom, and `fod submit`'s signed-size check stays the authority |
| new command | `fod reveal -contract … -pool-order N -vault FILE -prepared FILE` re-encodes the vault's committed secrets, refuses unless `sha256(secrets)` is the order's bundle, and writes the call; `reveal_test.go` pins the calldata and the mismatch refusal |
| measured costs | the live receipts are folded into `tools/fod/fees.go`: `refund` now carries the higher 1,357,900 wasm gas (13,579 user gas, 135.79 iDNA, 12.1 fee) and the new `reveal` entry carries 1,268,100 wasm (12,681 user gas, 126.81 iDNA, 23.5 fee); `fod fees` prices both at the chain's rate |
| tests | `tools/fod` `go test -count=1 ./...` ok (14.9 s); `spikes/harness` full suite ok (4.8 s) after `TestPoolDonationStaysWithTheBundleAuthor` was added (a stranger's donation is refused; the author's adds one unit to the bundle and the pool) and `reveal` was added to the published export surface; the git-ignored `output/cli/prepared-deploy.json` was regenerated against the release module (`codeBytes` 373,223), which the deploy test had caught as stale |

Limitation: the lab validator needed a lab-only mempool reset before it would
mine the refund it had already blacklisted; the contract itself executed the
call at the first simulation, and the refund's exact amounts and boundary blocks
stay pinned by `TestPoolFeeCapAndRefundReturnTheFee` and
`TestPoolDeliveryDeadlineBoundaries`.

### 2026-09-29 - mainnet epoch-228 session data collected read-only (owner request)

The owner asked for the live session's flip data to be collected passively, for
a later sell/buy test between two mainchain identities; no contract is deployed
and no transaction is sent. Read-only access runs over SSH to the designated
test host's node (loopback RPC, epoch 228, `startBlock` 11350781,
`nextValidation` 2026-09-29T15:00:00Z) and a tunnel on this Mac
(127.0.0.1:19130).

| Item | Value |
|---|---|
| sweep | `fod sweep -from 11350781 -to 11371974 -epoch 228`: 21,194 blocks, 468 txs, 199 `submitFlip`, 1 `deleteFlip`, 5 `storeToIpfs` -> `output/capture/epoch-228/sweep-epoch228.json` (sha256 `c669eacd…`) |
| submitters | 57 authors; `0xace93bd0…` submitted 5 flips in epoch 228 and `0x1af48202…` none |
| inventory and blobs | `fod inventory -author 0xace9… -sweep … -inventory-epoch 228 -blobs-out …`: 5 of 5 blobs fetched through the node's own IPFS and CID-verified, `complete=true` -> `output/capture/epoch-228/blobs-ai/` |
| availability decay | a re-fetch about ten minutes later failed for the same five CIDs (`blobs_ok 0`), and the public-gateway fallback (`-indexer https://api.idena.io/api -gateway https://ipfs.io/ipfs/`) verified 0 of 201: the epoch-wide blob set is not retrievable, the two identities' own material is |
| session capture | a read-only script on the test host waits for the node to report epoch 229, copies `idenachain.db` (the running node holds the LevelDB lock, so the CLI cannot open it in place), then writes the two-anchor session bundles for both identities, the `epoch-db` and `epoch-keys` censuses, and a sweep of the ceremony blocks |
| host binary | the current revision built on the host for linux/amd64 (`/root/fod-session-228/fod`, 106,391,112 bytes) |

Limitation: passive reading only. The session bundles, the censuses and the
ceremony sweep land after the live validation finishes, which is why the
follow-up automation watches the host; epoch-229 submissions are a later sweep.
Pre-flight at 14:09Z before the ceremony: the host has 10 GB free against the
3.5 GB `idenachain.db` copy the script takes, and the node the script reads
(the ai node on loopback RPC 9129) answers epoch 228 at head 11,372,305 with
`currentPeriod: None` and `nextValidation 15:00:00Z`. The capture assets were
checked too: the host script's SHA-256 matches the reviewed copy
(`018ace94…`) and the host's CLI binary runs. The copy-open path was verified
without touching the live node: `fod state-proof -datadir D -state-height 0`
resolves `<D>/idenachain.db`, exactly the layout the script's copy creates, and
it fails with a clear error when the directory is wrong. The host's older
Sep-4 chain clones are empty, so the ceremony copy itself is the first real
rehearsal of that step.
For the later mainchain sell test the seller identity's own nodekey (or its
keystore passphrase) is still a data dependency: the capture is public material,
and `fod derive-keys`/`export-keys` need that key to produce the epoch secrets a
bundle sells. Nothing in the capture substitutes for it.

### 2026-09-29 - E - the buyer's cost and time display, and the automatic refund watch

The buyer side of the approved policy: the price, the capped retrieval fee and
the three-block window are visible before signing, and the local companion
claims the refund without a browser or an operator.

| Surface | Change |
|---|---|
| `tools/fod/market.go` | the market snapshot records the node's observed block interval (`blockSeconds`) and computes the order deadlines as `funded+3` delivery and `funded+4` first refundable block |
| `app/src/state.ts`, `app/src/views/Market.tsx` | the listing shows the price, the retrieval-fee cap (covered flips x 1 iDNA, paid only to the hosts the buyer names), the delivery estimate from the observed cadence, and the distinction between inclusion and the order's window; it states that the refund opens in the next block and the local client claims it automatically |
| `tools/fod/watch.go`, `tools/fod/main.go` | new `fod watch -order N -contract ADDRESS -nodekey FILE`: it follows the order and submits the permissionless refund as soon as the chain passes the refund block. The refund pays the order's buyer regardless of the caller, so the key may be the buyer's or a funded relayer's; `refund` + `submit` stays the manual fallback |

Evidence, 2026-09-29: `cd app && npm run build && npm run verify` - ALL CHECKS
PASSED on the 275,940-byte HTML; `tools/fod` suite - ok (14.8 s); `spikes/harness`
full suite - ok (4.6 s); `node release/build-manifest.mjs` - new module 373,223
bytes `a6764bc7…`, HTML `8e5051c1…`, 0 forbidden operators, 0 remote tags,
pinned identifiers agree.

Limitations. The watch loop has no stubbed CLI regression yet; the lab journey
on a deployment of the new module is what will exercise it and produce the
latency and cost measurements. The reveal settlement still publishes the
committed keys, which is the author's choice at the boundary.

Next task: deploy the new module on the isolated lab chain, run the buyer
journey (buy with a retrieval fee, delivery, confirmation, and the watch's
automatic refund on a second order), and measure delivery latency and the
actual transaction costs.

### 2026-09-29 - D3/E - the approved purchase rules: three-block window, escrowed fee, one-transaction refund, reveal settlement

The owner's approved policy is implemented in the escrow, its hooks and the
CLI, and the focused tests the policy requires are in place.

| Surface | Change |
|---|---|
| `contracts/escrow/src/lib.rs` | `DEADLINE_BLOCKS = 3` with no grace: delivery is valid in `funded+1 ..= funded+3` and the refund opens at `funded+4` (strict `>` check); `buy` carries the retrieval-fee list after the delivery-key proof, caps it at `covered * FEE_PER_ITEM` (1 iDNA per authenticated item), requires the payment `quote + fee` and escrows both (order `fee` field plus the `pfee/<id>` list); settlement (`confirm`, `fulfill`) pays the bundle's registered author the purchase amount directly and credits the named hosts with the fee, so the reserve is never spent; `refund` is permissionless and pays the original buyer `paid + fee` directly in one transfer, then strikes the bundle; the new `reveal` settlement publishes the committed secrets (`sha256(secrets)` is the registered bundle key) and settles without the buyer's confirmation, so withholding cannot convert a received delivery into a refund |
| `proofs/*`, `fod_escrow` hooks | the seed-order hook now models a buyer's payment as escrow the harness credits (no reserve move); `seedPoolCovered` names the authenticated covered count so the fee cap is exercisable; refusal diagnostics stay off the state path |
| `tools/fod/pool_buy.go` | `prepare-buy` builds the fee list, checks the covered-count cap, and signs `quote + fee`; the committed `output/cli/prepared-buy.json` is regenerated in the new format |
| `spikes/harness/escrow_instant_test.go` | new focused tests: reveal settlement pays the author and the named host while a refund is refused (and a wrong preimage cannot settle, leaving the refund open); a fee above the cap is refused and a timeout returns the payment plus the fee; the deadline boundaries accept delivery at `funded+3`, refuse the refund there, refuse delivery at `funded+4` and open the refund in that block |

Evidence, 2026-09-29: `contracts/escrow` `cargo test --locked --offline` - 15
pass; test-hooked module rebuilt; `spikes/harness` full suite `go test
-count=1 ./...` - ok (4.5 s), including the three new tests; `tools/fod` suite -
ok (14.8 s).

Limitations. The buyer client still needs the delivery-estimate and cost
display plus the automatic refund action; the release artifact and
`release/MANIFEST.json` are stale until the next rebuild; and the live lab
journey has not been re-run on a deployment of this module, so no latency or
cost measurement exists yet. The reveal settlement publishes the committed
keys, which is the author's choice at the boundary and is documented in
`SPEC.md` 7.2.

Next task: update the buyer client (estimate, total cost, automatic refund),
rebuild the release artifact and manifest, then deploy on the lab chain and
measure delivery latency and the refund path end to end.

### 2026-09-29 - D2 - one global pool, and the supply side is refused for good

The owner's liquidity-scope answer ("one global pool") and subsidy answer ("no
subsidy at all") are implemented in the escrow and its tests.

| Surface | Change |
|---|---|
| `contracts/escrow/src/lib.rs` | the reserve, sale-unit count and pending escrow are one market-wide state (`reserve`, `units`, `pending` running totals with `global_pool()`/`set_global_pool`); a `PoolBundle` keeps only its author, the two commitments, its inventory, beacon, strikes, buy nonce and covered count; `buy` quotes from the global pool and decrements the bundle's inventory; `confirm`/`refund` move the pool's pending escrow only; `addReserve` funds the single reserve; `donateUnit` adds to the global unit count; `sellUnit` stays refused (`supply_disabled`), and `accounting` publishes `unitsTotal` |
| `contracts/escrow/src/pool.rs` | the module documents and keeps the one-pool arithmetic (`ceil(r/(q-1))` buy, `floor(r/(q+1))` sell), unchanged in form, now standing for the whole market |
| `spikes/harness/escrow_two_bundles_test.go` | the old per-bundle isolation test is restated as the global property: a refund consumes only its own order's pending escrow, the one reserve is untouched, the other bundle's inventory, beacon and strikes stay, and the books balance (`reserveTotal 300, pendingTotal 0, unitsTotal 4, creditTotal 100, liquid 400`) |
| `spikes/harness/escrow_pool_test.go` | the accounting JSON carries `unitsTotal` |

Evidence, 2026-09-29: `contracts/escrow` `cargo test --locked --offline` - 15
pass; the test-hooked module rebuilt (`dist/fod_escrow.wasm`,
sha256 `4789220a…`); `spikes/harness` full suite `go test -count=1 ./...` - ok
(4.1 s), one expectation updated, no refusal regressed.

Limitation: this is the contract half only. The release artifact, the CLI's new
`buy` arguments (host fees) and the bond-backed delivery window are the next
steps, and the live journey must be re-run on a fresh deployment of the new
module before any of it is presented as release evidence.

Next task: implement the buyer-paid retrieval fee in `buy`, then the short
delivery window with the author bond.

### 2026-09-29 - F3/E - the refund and withdrawal leg settled on the chain, and the fee table carries its live gas

The last open acceptance row ran on the isolated play-money chain. Order 2 was
delivered but never confirmed, so after its deadline plus grace the buyer
refunded it and withdrew the credit; both nodes agree on every receipt and on
the accounting.

| Step | Evidence |
|---|---|
| window | order 2 funded at 832, `refundAfterBlock 1672`; the earlier attempt at 876 was refused on-chain, so the deadline was already enforced |
| refund | `0xc44bac31…` mines at height 1683 (nonce 1, epoch 10): `method refund, success true, gasUsed 13,008, gasCost 130.08, txFee 12.1`, identical on both nodes |
| refund state | `fod market` from both nodes at 1687: order 2 `refunded`, pool `pending 0`, `reserve 2e21`, `strikes 1`, the buyer's credit listed |
| withdraw | `0x4fa5df79…` mines at height 1684 (nonce 2): `method withdraw, success true, gasUsed 3,719, gasCost 37.19, txFee 12.2`, identical on both nodes; the buyer's balance moves 325.14 -> 2275.75 |
| after | both nodes' market files are byte-identical at 1687 (`credits` back to 0); `contract_estimateCall(accounting)` with `from` the buyer returns the same payload from both: `callerCredit 0, creditTotal 0, reserveTotal 2e21, pendingTotal 0, liquid 2e21`; the contract balance is 2000 iDNA on both, so `balance == reserve + pending + credits` |
| fees | both receipts are folded into `tools/fod/fees.go` (1,300,836 wasm refund, 371,966 withdraw); `fod fees` prices a refund at 142.09 iDNA and a withdrawal at 48.90 iDNA at the chain's 1e16 per gas |
| tests | `tools/fod` `go test -count=1 ./...` ok (15.0 s); `spikes/harness` `-run 'Test(Escrow|Pool|Release)'` ok (2.6 s) |

Closing batch, same state: both nodes' `bcn_txReceipt` payloads for
`0xc44bac31…` (refund, 13,008 gas) and `0x4fa5df79…` (withdraw, 3,719 gas) are
byte-identical; `node release/check-integration.mjs` - ALL CHECKS PASSED (11 of
11); the full `spikes/harness` suite passes in 3.9 s; `release/MANIFEST.json`
is regenerated against the worktree (HTML 275,082 `42e48dfe…`, module 369,709
`be6d9de6…`).

Operational notes. The first refund signature was refused before broadcasting:
the signed envelope is one byte larger than the fee table's reference size, so
163.95 iDNA was required and the carried 163.85 was 0.1 short. The wider
200 iDNA budget went through, and nothing was sent by the refused attempt. The
estimate path takes `from`; a `caller` field is ignored, so the node signs the
probe with its own key and a node without funds answers `insufficient funds`.

Limitation: one refund/withdrawal pair on the isolated lab chain, with both
nodes on this host sharing keys; the receipts are real, the operator
independence is configuration-level.

Next task: none for this leg. Regenerate the manifest and assemble the final
acceptance report with the decentralisation clause matrix.

### 2026-09-29 - verification - the Package A containment re-checked in the working tree

Source re-check of the adapter findings, against the current files:

| Finding | State in the worktree |
|---|---|
| caller-asserted identity | `identityVerified` no longer appears anywhere under `utility/src/`; the grant store and policy name only provider-issued participant ids, and the README says a request carrying the field is inert |
| rate-limit bypass | `gateway/src/http.ts:96` keys the anonymous limit by `addr:<remote address>` and `:136`-`:142` keys an authenticated limit by the participant, with the comment stating the presented token never names a bucket |
| session timing | `gateway/src/session.ts:74`-`:79` refuse before `opensAt` and at/after `closesAt`; `gateway/src/routes.ts:111`, `:146`, `:167` refuse enrolment and answers with `the session opens at …` |
| local store scope | `utility/src/access.ts:8`-`11` states the grant store is in-memory, namespaced by utility, provider and participant, forgotten on restart, and cannot decide a global limit; the key is built from the three ids at `:108` |

Limitation: inspection only (the package's regression suites already pass on
this tree, and the sources above are the ones those suites exercise).

Next task: at block >= 1672 run the buyer's `refund` and `withdraw`, fold their
live gas into `tools/fod/fees.go`, then regenerate the manifest and finish the
acceptance exercise.

### 2026-09-29 - verification - the five contract findings re-checked in the working tree

Source re-check of the five findings the contract/proof audit raised at
`1462782b8f15`, against the current worktree. This is inspection, not a new
run; the runtime evidence for these paths is the receipt sequence 472-979
recorded below.

| Finding | State in the worktree |
|---|---|
| anyone could create units and advertise a live desk (`donateUnit`, `beacon`) | both are author-only now: `donateUnit` refuses a caller that is not the registration-named author (`not_author`, `lib.rs:1349`), `beacon` likewise (`lib.rs:1467`); the wider third-party donation rule stays an owner decision, recorded in the code comment at `lib.rs:1322` |
| `confirm` settled by order id without the bytes the buyer checked | `confirm` takes `(id, claimed)` and refuses `ciphertext_mismatch` unless `sha256(stored delivery) == claimed` (`lib.rs:1946`-`1989`); replacement is still allowed while the order is pending, and a settled order cannot be delivered into (`order_not_delivered`) |
| buyer escrow sat in the spendable reserve | `buy` moves the payment into `bundle.pending` and the global `pending_total` (`lib.rs:933`-`958`); `settle_pending`/`release_pending` move it to the reserve or to a caller credit, and `accounting` publishes `pendingTotal` (`lib.rs:812`-`832`); the live read showed `balance == reserve + pending + credits` exactly |
| inventory root could predate the epoch's final submission | closed against the pinned chain rules: the inventory anchor must be past the flip-submission window (`InventoryStillOpen`, `proofs/session-core/src/session.rs`; `SOURCES.md` F-99), and the verdict anchor is the epoch's `ValidationFinished` record; epoch 0 stays unprovable (proto3 omits a zero epoch), recorded in the 2026-09-29 B1/B4 entry |
| no network binding in the statements | the registration journal folds a network digest at offset 294 and the escrow compares it with its pinned `NETWORK_DIGEST` (`lib.rs:67`, `:90`, `:1179`-`1188`, `proofs/session-core/src/statement.rs:143`); the delivery statement relies on the two anchored header roots plus the order, recipient and ciphertext binding, stated in `SPEC.md` 5.5 rather than carried in that journal |

Limitation: the two owner-gated economic halves (finite subsidized issuance,
retrieval fee) are unchanged and stay open. The delivery statement's
chain binding is the header-root comparison, not a journal field; `SPEC.md`
5.5 is the place that says so.

Next task: at block >= 1672 run the buyer's `refund` and `withdraw`, fold their
live gas into `tools/fod/fees.go`, then regenerate the manifest and finish the
acceptance exercise.

### 2026-09-29 - F1/F2 - failed checks now stop the build, and a clean clone rebuilds both artefacts byte for byte

F1's remaining half was the release tooling: it recorded validation results
instead of failing on them, and it replaced `release/artifacts/fod_escrow.wasm`
before validating the candidate.

| Surface | Change |
|---|---|
| `release/build-manifest.mjs` | the candidate module is built to `fod_escrow.wasm.staged` and only renamed into place after every check passes; a missing `wasm-objdump`, a non-zero forbidden-operator count, non-zero remote interface tags, or a pinned-identifier drift now exits 1, prints the reason, and removes the staged file |

Evidence, 2026-09-29:

- `node release/build-manifest.mjs` in the worktree passes and records the same
  tuple the previous run did: HTML 275,082 bytes `42e48dfe…`, module 369,709
  bytes `be6d9de6…`, 0 forbidden operators, 0 remote tags.
- `node release/check-integration.mjs` - ALL CHECKS PASSED (11 of 11,
  including the grant scope, replay refusal and quota spend).
- `cd proofs/registration-v2/host && cargo test --locked --offline --test
  sealed_artefact` - 1 passed: the committed `fod:registration:v3` artefact
  verifies against the guest image id this tree builds, with the recorded
  claim digest, so the guest-receipt-contract-CLI tuple holds on the current
  sources.
- Clean-clone rebuild comparison: a disposable clone (`git clone --local` of the
  worktree, `git diff --binary HEAD | git apply`, copied `app/node_modules`,
  empty `contracts/escrow/target`) ran `cd app && npm run build` and
  `node release/build-manifest.mjs` with the patched script and reproduced
  **both artefacts byte for byte** - HTML 275,082 `42e48dfe…`, module 369,709
  `be6d9de6…`. The clone is `/tmp/fod-rebuild-DbkNC9` (`dirty` status 70 files,
  matching the worktree).
- Failure paths, each in the clone with a `SENTINEL` written to the released
  module: an injected remote `<script src>`, `PATH` without `wasm-objdump`, and a
  stub `wasm-objdump` reporting an `f32.const` operator each exited 1 with the
  named reason; in all three the sentinel survived and no `.staged` file was
  left behind.

Limitation: the manifest records the base commit `1462782b8f15` with
`worktreeDirty: true`; commits are not authorized for this task, so the
"delivered revision" remains the worktree. The identifier-drift branch is
unchanged from the earlier run and was not re-simulated. Re-run
`node release/build-manifest.mjs` once more after the last source change.

Next task: at block >= 1672 run the buyer's `refund` and `withdraw`, fold their
live gas into `tools/fod/fees.go`, then regenerate the manifest and finish the
acceptance exercise.

### 2026-09-29 - verification - the full harness suite, and the stale local deploy artifact

Re-ran the remaining batches in one pass:

- `contracts/escrow` `cargo test --locked --offline`: **15 tests pass**.
- `proofs/session-core` `cargo test --locked --offline`: **19 tests pass** (the
  unit binary; an earlier `tail` had caught the doc-test section, which reports
  zero).
- `spikes/harness` **full** `go test -count=1 ./...` failed on the first run:
  `escrow_cli_deploy_test.go:62: the artefact names 361960 module bytes, the
  release module is 369709`. The artifact is `output/cli/prepared-deploy.json`,
  which is git-ignored local state dated 2026-09-26: it had been signed against
  the module from before the raw-CID guest change. The plan's focused batch
  (`-run 'Test(Escrow|Pool|Release|Registration|Session)'`) does not include that
  test, which is why the drift was invisible there. Regenerating it against the
  current module
  (`cargo`-built `release/artifacts/fod_escrow.wasm`, keccak `0cb87e7d…`) with
  `fod prepare-deploy … -signed output/cli/prepared-deploy.json` gives
  `codeBytes 369709`, `broadcast false`, and the **full suite then passes in
  3.8 s**.
- Tool note recorded from the same run: `prepare-deploy` writes through
  `-signed` (or an explicit `-out`); `-prepared` is accepted and silently writes
  nothing, which is the trap the CLI's own comment names.

Next task: at block >= 1672 run the buyer's `refund` and `withdraw`, fold their
live gas into `tools/fod/fees.go`, then the final acceptance exercise.

### 2026-09-29 - acceptance - definition-of-done matrix (SPEC 11.2)

One line per requirement, with the evidence that exists today. One item is
open: the retrieval fee, whose transport, amount and payer are an owner
decision; the refund/withdrawal leg closed at heights 1683-1684.

| # | Requirement | Evidence | Status |
|---|---|---|---|
| 1 | obtain and locally open the HTML | `app/dist/flips-on-demand.html` 275,082 bytes, sha256 `42e48dfe…` in `release/MANIFEST.json`; `app/verify.mjs` opens it from `file://` and asserts no remote requests beyond a configured gateway | met |
| 2 | independent marketplace operation through the user's own node | the whole journey (472 registration to 979 concurrency test) ran through node RPC and `tools/fod`; no gateway or utility process was running | met |
| 3 | providers offer shared-node access without admin credentials | `gateway/src/routes.ts` is a fixed table with no RPC passthrough; gateway's 25 tests and typecheck pass (`SOURCES.md` O-7) | met |
| 4 | customers use the participant integration without a full node or CLI | `release/check-integration.mjs` runs session, provider-signed result, grant, replay refusal and quota spend; `app/verify.mjs` walks the same in the browser | met |
| 5 | sellers inspect historical flips and prepare eligible bundles | `fod inventory`, `eligibility`, `session-bundle`; the captured epoch-1 bundle registered on-chain at height 472 | met |
| 6 | incomplete or ineligible bundles cannot register | the v3 guest refuses unsupported flip structure (entry of 2026-09-27 and the sealed-artefact tests) | met |
| 7 | buyers deposit play-iDNA on the isolated chain | funding sends at 645, 671, 825, 827 and the two buys at 782 and 832 | met |
| 8 | correct original keys delivered encrypted to the funded buyer | deliveries at 785 and 942; the buyer's `receive` decrypts 106 bytes whose hash is the bundle commitment | met |
| 9 | invalid, substituted or replayed delivery evidence cannot settle | junk delivery refused by the buyer; duplicate confirm refused at 878; deliver-into-settled refused at 880 | met |
| 10 | sold at the quote the reserve and units imply, delivery bound to the funded buyer's key | quotes 1e21 and 2e21 equal `ceil(reserve/(units-1))` at the two buys; confirm at 794 names the ciphertext digest the buyer checked | met |
| 11 | buyers recover deliveries after restart | `buyer.vault`, `buyer2.vault`, `buyer3.vault` and the delivery records; every CLI run is a fresh process that re-verifies from the chain | met |
| 12 | unfulfilled orders can be refunded and credits withdrawn | order 2 refunded at 1683 (`0xc44bac31…`, success, gasUsed 13,008) and the credit withdrawn at 1684 (`0x4fa5df79…`, success, gasUsed 3,719); the buyer's balance moved 325.14 -> 2275.75; the too-early attempt was refused on-chain at 876 | met |
| 13 | commission is zero and no person receives a share | `escrow_no_admin_test.go`; the live accounting read shows `creditTotal 0` and no fee recipient exists in the escrow | met |
| 14 | FAQ explains gated utility, hosting, AI, costs, limitations | `app/src/faq.ts` states the page needs no hosted service, that a provider is trusted for the utility it hosts and cannot move escrow, and that a provider-signed result is trusted for its scoring; the walk checks it renders offline | met |
| 15 | release evidence names source, artifacts, contract, proofs and outcomes | `release/MANIFEST.json` (revision, HTML sha256, module sha256, 0 forbidden operators); `keccak256(release/artifacts/fod_escrow.wasm)` equals the chain's code hash `0cb87e7d…`; image id `7ee79d90…` pinned in artefact, escrow and CLI; receipts in this log | met |
| 16 | a gifted listing sells without paying its donor; seller lists, keeps a desk and withdraws without an operator | the two donated units sold (orders 1 and 2), the author took no credit, the desk published and replaced deliveries, and the buyer's withdrawal at 1684 paid the credit out on the buyer's own signature | met |
| 17 | each of the three journeys completes in the browser walk | `app/verify.mjs` walks market to buy request, trades to confirm/refund/sell/give/withdraw requests with their CLI commands, and the session to a verified provider result, plus FAQ, phone-width and offline checks - ALL CHECKS PASSED | met |
| 18 | reserve, unit and quote invariants under concurrent calls, refunds and strikes | after the refund both nodes return the same accounting payload (`pendingTotal 0`, `creditTotal 0`, `reserveTotal 2e21`, `liquid 2e21`) and the contract balance 2000 equals reserve + pending + credits; the strike count reads 1; the same-nonce conflict resolved to exactly one transaction at 979 | met |
| 19 | no unit enters the pool without a verified session binding | registration at 472 carries the sealed v3 receipt bound to two anchored session state versions; donations are author-only and reserve-funded supply is disabled | met |
| 20 | a retrieval fee reaches only the hosts the buyer names, from the buyer's payment, within the cap | no fee path exists yet; the plan records transport, amount and payer as an owner decision, and the reserve pays nothing for retention today | **open - owner decision** |

Review findings A01-A10 are contained or closed as the per-finding entries
record; A05's finite-issuance and A08's retrieval-fee halves remain the two
owner-gated economic decisions.

### 2026-09-29 - F3 - conflicting transactions through two nodes, and the pool's promotion path

The acceptance list's "concurrent consumption through different nodes" is now
exercised with real receipts.

| Step | Result |
|---|---|
| two conflicting transactions | the same desk delivery for order 2 signed twice with account nonce 5 and different max fees, so two distinct hashes: `0x227babda…` (200 iDNA budget) sent to the miner, `0x191e69dd…` (300 iDNA) sent to the follower; both nodes accept theirs |
| outcome | **exactly one lands**: `0x227babda…` mines at height 979 (`method deliver, success true, gasUsed 10351`); `0x191e69dd…` never gets a receipt and both mempools drain to empty |
| no churn | the on-chain ciphertext for order 2 still hashes to `77dd0d253e77d3b7057268dc3b6eb94da56f7828b13e6f5b74927a33e9758ba1`, the digest the desk recorded, so the losing transaction left nothing behind |

Promotion path observed on the same chain: the desk's earlier delivery signed at
the fee floor (`0xcafe6eb1…`, nonce 3) never entered a block and its pool entry
occupied the slot; after the pool was cleared and a fresh nonce-3 delivery
(`0x86ee6d44…`) mined at height 942, the still-queued successor
(`0x3c6bd703…`, nonce 4) was promoted and mined at height 957 - the account's
epoch-6 history is nonces 1 and 2 (the two refusals), then 3 and 4 (the two
deliveries).

Contract-level regressions re-run the same day: `spikes/harness`
`go test -count=1 -run 'Test(Escrow|Pool|Release|Registration|Session)' .`
passes 12 tests.

Next task: at block >= 1672 run the buyer's `refund` and `withdraw`, fold their
live gas into `tools/fod/fees.go`, then the final acceptance exercise.

### 2026-09-29 - A06 - the desk replaces a delivery slot it does not own, and it signs with headroom

The contract already let any caller replace a pending ciphertext; the desk now
uses that instead of leaving an occupied slot to the attacker until the refund
deadline.

| Surface | Change |
|---|---|
| `tools/fod/serve.go` | a `deliver` records the ciphertext digest it published (`delivered-<id>.json` in the desk's outdir); `foreignDeliveries` compares that record with the value the escrow holds under `pdeliv/<id>` and marks a delivered order for replacement when they differ, or when the desk has no record at all - so a desk that was offline or an order delivered before the record existed is repaired on the next pass |
| `tools/fod/serve.go` | the planner plans `deliver` for a marked order and only falls back to `fulfill` when the recorded ciphertext is the desk's own; the replacement is one-shot because the record then matches the chain |
| `tools/fod/serve.go` | the desk's automatic fee is the measured floor plus `deskFeeHeadroomPercent` (25%); the node adds a transaction that runs out of gas inside a proposal to its persistent blacklist, so an unattended desk must not sign the exact floor |
| `tools/fod/serve_test.go` | an occupied slot plans `deliver`, never `fulfill`; the delivery record round-trips, ignores other orders' files and distrusts a corrupt one; the desk fee is above the measured floor and below the node's cap |

Live demonstration on order 2 (its slot held a ciphertext the desk had not
published): the desk's pass prepared `deliver-2.json` plus `delivered-2.json`
(before this change the same pass prepared only the beacon). The first
`-broadcast` attempt was signed at the fee floor, never entered a block, and its
pool entry occupied the account's nonce slot; after clearing that entry and
submitting the prepared delivery with a fresh nonce and a generous budget it
mined at **block 942** (`method deliver, success true, gasUsed 10351`). The next
desk pass prepared only the beacon - no repeated delivery - and the buyer's
`receive -pool-order 2` verified the new ciphertext
`77dd0d253e77d3b7057268dc3b6eb94da56f7828b13e6f5b74927a33e9758ba1`, decrypting
106 bytes that hash to the bundle commitment.

Verification: `go build ./...` and the full `tools/fod` suite pass (14 s,
including the three new tests).

Limitations: the first desk broadcast's exact failure mechanism was not pinned
(the fee floor and the node's silent blacklist are the consistent explanation,
and the headroom removes that case); the desk still aborts a pass when
`waitForNonce` does not see the nonce move, so the beacon behind a delivery waits
for the next pass.

Follower repair, 2026-09-29: the follower lost its peer when the miner restarted
and then could not fetch blocks (`fail to retrieve block …
timeout while reading data from ipfs`), so its state fell forty blocks behind.
After the miner's restart it was restored by copying the miner's `idenachain.db`
into its datadir (the stale one kept as `idenachain.db.stale-0605`); both nodes
now report height 971 and the same normalised market digest
`20378fc07c629619a79f98ea2f1e0f7a` (`units 1, pending 2e21, reserve 2e21, nonce
2, strikes 0`, order 1 settled, order 2 delivered). The bitswap path between two
kubo instances on this host remains unreliable, which is why the copy is the
repair of record.

Next task: at block >= 1672 run the buyer's `refund` and `withdraw`, fold their
live gas into `tools/fod/fees.go`, then the final acceptance exercise.

### 2026-09-29 - F3 - two nodes reproduce every recorded receipt and the live accounting totals

The acceptance claim "independent node operators can verify the same outcomes"
was tested directly against the recorded evidence instead of restated.

- Eighteen transactions across seventeen heights (472 registration, 476
  addReserve, 478/481 donateUnit, 514/598 beacon, 782 buy, 785 deliver, 794
  confirm, 825 fund send + donateUnit, 832 buy, 835 junk deliver, 838
  replacement deliver, 876 refund refusal, 878 duplicate-confirm refusal, 880
  deliver-into-settled refusal, 890 sellUnit refusal) were read through both
  nodes. **17 of 18 receipts are byte-identical on both** (`method`, `success`,
  `gasUsed`).
- The eighteenth is the plain `send` funding transaction. `bcn_txReceipt`
  answers nothing for a plain send on **either** node - the converter panics on
  the nil receipt and the RPC recovers per request - so the sends are evidenced
  by their blocks (the tx lists match on both nodes) and by the balance deltas
  they produce, not by a receipt.
- Live accounting, read with `contract_estimateCall` (`method accounting`,
  no arguments) returns the same payload from both nodes:
  `{"callerCredit":0,"creditTotal":0,"reserveTotal":2000000000000000000000,"pendingTotal":2000000000000000000000,"liquid":4000000000000000000000}`.
  The contract's own balance is `4000` iDNA on both nodes, so
  `balance == reserveTotal + pendingTotal + creditTotal` holds exactly with no
  unattributed surplus.
- No gateway or utility process was running (and none of their ports were
  listening) while the on-chain journey above was produced; the node RPC and
  `tools/fod` carried registration, funding, donations, beacon, buy, delivery,
  the buyer's verification, confirm, every refusal and the accounting read.
- Artifact-set pins, checked together: the guest image id is `7ee79d90…` in the
  sealed artefact, in `contracts/escrow/src/verifier.rs`
  (`REGISTRATION_IMAGE_ID`) and in `tools/fod/receipt_claim.go`;
  `keccak256(release/artifacts/fod_escrow.wasm)` is the chain's code hash
  `0cb87e7d…`, so the release module is the deployed code; and
  `sha256(app/dist/flips-on-demand.html)` is the manifest's `42e48dfe…`. The one
  deliberate exception stays the committed receipt's placeholder contract,
  recorded in `proofs/registration-v2/dist/README.md`.

Limitations: both nodes share this host and the account keys, so this is
independent configuration and state, not independent operators; and the refund
leg still waits for order 2 to pass block 1672.

Next task: at block >= 1672 run the buyer's `refund` and `withdraw`, fold their
live gas into `tools/fod/fees.go`, then the final acceptance exercise.

### 2026-09-29 - A01/A05/F1 - paid supply is refused on-chain, both nodes agree, and the in-tree receipt names a placeholder contract

The reserve-funded sale is disabled in the deployed escrow, and that refusal is
now on the chain with both nodes agreeing on the verdict.

| Step | Result |
|---|---|
| prepare | `/tmp/fod-cer/prepared-sell.json` (614 bytes: commitment 32 + v3 journal 326 + seal 256), built from the sealed artefact `/tmp/fod-cer/registration-lab-2.json` because that is the one bound to the deployed contract |
| both nodes estimate the same signed call | port 19095 and port 19097 each return `success false, method sellUnit, gasUsed 1000, RuntimeError: unreachable` - two independently configured nodes agree on the authorization outcome before anything is mined |
| broadcast | `0x0510aaed…` mines at height 890 as `method sellUnit, success false, gasUsed 1000`; the market read after it is unchanged (`units 1, pending 2e21, reserve 2e21, nonce 2, strikes 0`, order 1 settled, order 2 delivered, no credits) |
| fee guard | the CLI refused the first, cheaper signature with `this signed transaction needs a max fee of 19773490000000000000000, and it carries 1000000000000000000000` - the corrected `tools/fod/fees.go` now blocks exactly the underpricing that used to run out of gas and get the transaction blacklisted |

**Finding, medium - the in-tree sealed receipt is placeholder-bound.** `proofs/registration-v2/dist/registration-v2.json` (`statement fod:registration:v3`) names contract `1111111111111111111111111111111111111111`, so `prepare-list`/`prepare-registration` refuse it against the deployed escrow; the receipt the chain accepted at height 472 was built from the lab artefact bound to `f8bab10cd96962d140f6fa7ba822974d48d3e0d5`, which lives outside the repository. Fixed as documentation rather than as an artefact swap: `proofs/registration-v2/dist/README.md` now records that the committed proof is fixture-bound, why its journal names the placeholder, and the exact `cargo run --release -- --prove --contract … --session … --blob …` command that produces a chain-bound receipt. `node release/check-integration.mjs` and the host's `the_sealed_v2_artefact_verifies_against_the_guest` test still pass with the note in place.

Next task: at block >= 1672 run the buyer's `refund` and `withdraw` for order 2
and fold their live gas into `tools/fod/fees.go`; decide the F1 artefact
question above; then the final acceptance exercise.

### 2026-09-29 - C2/D3/F2 - refusal paths and duplicate settlement are contained, and the toolchain declarations match

Three adversarial calls landed on the chain and all three trapped without
changing the marketplace state; the market read after them is identical to the
read before (`units 1, pending 2e21, reserve 2e21, nonce 2, strikes 0,
credits []`, order 1 settled, order 2 delivered).

| Attempt | Receipt |
|---|---|
| refund before the deadline + grace (`0xd3e4596e…`) | height 876, `method refund`, `success false`, `gasUsed 2562` - the escrow refuses `refund_not_available` and credits nothing |
| confirm order 1 a second time (`0x8f5ad03e…`) | height 878, `method confirm`, `success false`, `gasUsed 2544` - a settled order cannot settle twice, and no second escrow movement happens |
| deliver into settled order 1 (`0x83d8832f…`) | height 880, `method deliver`, `success false`, `gasUsed 2542` - a closed order rejects further ciphertext |

The generic `RuntimeError: unreachable` in the receipts is the guest's
`error_reply` (it ignores the code on purpose), so the refusals are identified by
the call and the unchanged state, not by the receipt text.

F2: `.tool-versions` now declares `go 1.26.8` (the module's own requirement in
`tools/fod/go.mod`, previously declared as 1.26.1) and `rust 1.98.1` (the
toolchain the proofs were built and tested with here; the repos' own
`rust-toolchain.toml` files pin `stable`). Node 24.18.0 and Python 3.13.15
already matched. The README's `go.local.mod` recipe stays path-free.

Limitations: the refusal codes are not observable in a receipt, so each refusal
is evidence of "this call at this order state is refused", not of which internal
guard fired; and the withdrawal leg still waits for order 2 to pass block 1672.

Verification run the same day, on the working tree: `utility` 14 tests pass and
`tsc --noEmit` is clean; `gateway` 25 tests pass and `tsc --noEmit` is clean;
`app` builds `app/dist/flips-on-demand.html` at 275,082 bytes and
`npm run verify` reports ALL CHECKS PASSED (signed provider result verified in
the browser, no remote requests beyond the configured gateway, nothing kept in
browser storage, a policy from a future version refused by name, an unreachable
gateway reported instead of swallowed, no console errors beyond that refusal).

Next task: at block >= 1672 run the buyer's `refund` and then the `withdraw`,
measure both live and fold the numbers into `tools/fod/fees.go`, then run the
release-artifact pass and the final acceptance exercise.

### 2026-09-29 - E/C2 - junk-first delivery refused, replaced, recovered, and the refund clock started

Order 2 carries the delivery-failure sequence the review named, with the refund
window now open on the chain.

| Step | Receipt |
|---|---|
| buy (`0x52663696…`) | height 832, `method buy`, `success true`, `gasUsed 11013`; order 2 is funded with `paid 2000000000000000000000`, `units 1`, `pending 2e21` |
| junk delivery (`0x94f3dac2…`) | height 835, `method deliver`, `success true`, `gasUsed 9968`; a third party published 200 random bytes for order 2 and the contract accepts any candidate |
| buyer refuses junk | `fod receive -pool-order 2` -> `receive failed: the ciphertext recorded for pool order 2 does not decrypt with this delivery key: ecies: invalid public key` |
| valid replacement (`0x4c1109ea…`) | height 838, `method deliver`, `success true`, `gasUsed 10351`; the buyer-bound ciphertext replaces the junk while the order is still pending (`REVIEW.md` A06) |
| buyer recovers | `fod receive -pool-order 2` verifies the delivered key, decrypts 106 bytes, matches the commitment `b08e178b…` and the ciphertext hash `1e34ba4e…`, and writes `/tmp/fod-cer/buyer2.vault` |

**Finding, medium - the desk never replaces a delivery it did not publish.**
`tools/fod/serve.go` `planServePass` plans `deliver` only for orders in the
`funded` state and `fulfill` for `delivered` orders (and only with a prover), so
after a junk candidate lands the honest desk plans nothing and the slot stays
occupied until the deadline. The contract allows the replacement - the
replacement above was published with the desk's own buyer-bound ciphertext - so
the buyer's only bounded exit is the refund (`refund` after
`funded_at + DEADLINE_BLOCKS + REFUND_GRACE_BLOCKS` = block 832 + 840 = 1672).
Recorded as a tooling gap, not an escrow defect.

Fee table refined: the buy entry now carries the higher of the two live
receipts (11,013 user gas = 1,101,300 wasm, heights 782 and 832). Verified with
`go build ./...` and the focused fee tests in `tools/fod`.

Next task: at block >= 1672 run the buyer's `refund` and then `withdraw` on
order 2 (the first live measurements for those two operations, which currently
carry the harness figure plus the observed gap), confirm the credit and the
strike, and fold the numbers back into `tools/fod/fees.go`.

### 2026-09-29 - F3/E - the pool lifecycle ran end to end, and the fee table is why it kept failing

The isolated play-money chain now carries the whole buyer leg with real
receipts, and the reason the earlier attempts never settled is a defect in the
CLI's fee table plus the pinned node's blacklist behaviour, not the contract.

| Step | Receipt |
|---|---|
| buy (`0x4c19f52f…`) | height 782, `method buy`, `success true`, `gasUsed 10766`; pool turns into `units 1, pending 1000000000000000000000, nonce 1` |
| deliver (`0xe392e324…`) | height 785, `method deliver`, `success true`, `gasUsed 10351` |
| receive (offline) | the buyer's own node repeated the delivery key, decrypted 106 bytes of secrets, matched them to the bundle key commitment `b08e178b…` and named the ciphertext hash `1e34ba4e…` that the confirmation carries; the keys went into `/tmp/fod-cer/buyer.vault` |
| confirm (`0x296d101b…`) | height 794, `method confirm`, `success true`, `gasUsed 14747`; order 1 settles, `pending 0`, `reserve 2000000000000000000000`, `units 1`, `nonce 1`, `strikes 0` |
| bought content | `fod receive -vault buyer.vault -blob flip-blob-epoch1.bin` decrypts the delivered content: public part 68 bytes, private part 90 bytes, both PNG data (`89 50 4e 47`), written to `/tmp/fod-cer/buyer-flip-content.json{,.private}` |
| two nodes | miner (19095) and the follower with its own datadir, IPFS repo and peer id (19097) both report height 815, head `0xf101d8b3…`, and the same normalised market digest `8a3486ab97b6e207264173b4de43cfbe` (`units 1, pending 0, reserve 2e21, order 1 settled`) |

**Fix landed - `tools/fod/fees.go` carries the live numbers.** The short pool
calls now use the receipts' figures (buy 1,076,600, deliver 1,035,100, confirm
1,474,700, beacon 630,600, donateUnit 630,100, addReserve 672,600,
registerBundle 195,741,200 wasm), the operations without a live measurement yet
carry the harness figure plus the largest observed gap (1,400,000 wasm), and the
header records why an underpriced max fee is not a harmless warning. Verified
with `go build ./...` and the focused fee tests
(`TestEveryOperationHasAMeasuredNumber`, `TestUserGasRoundsUp`,
`TestRequiredMaxFeeMultipliesExactly`, `TestMaxMaxFeeIsTheBlockGasCapTimesTheRate`,
`TestIDNAText`, `TestSizeFeeChargesTenGasPerByte`) in `tools/fod`.

**Finding 1, high - the fee table underprices every pool call by 6-10x.**
`tools/fod/fees.go` lists `buy` at 163,757 wasm and `deliver`/`confirm` at
110,861/124,748; the same calls used 1,076,600 / ~1,035,100 / ~1,474,700 wasm on
the chain (the receipts' `gasUsed` is user gas: 10,766 / 10,351 / 14,747). A max
fee computed from the table is below the live requirement, so the call runs
`Out of gas` inside the proposal. The working submission used
`-max-fee 200000000000000000000` (200 iDNA) for the buy and 1000 iDNA for the
deliver, not the table's number.

**Finding 2, high - the pinned node blacklists such a transaction permanently.**
`blockchain/filterTxs` drops a transaction whose proposal-time execution ends in
`Out of gas` **and calls `repo.AddToBlackList`**; the blacklist lives in the
datadir. The transaction stays in the mempool, keeps the sender's nonce slot in
`executableTxs`, and is then silently skipped by every later proposal, so the
same sender can never get a replacement with that nonce mined. The instrumented
run recorded both halves:
`LABPOOL tryExecute … err="Error calling the VM: RuntimeError: Out of gas"` and,
after a restart, `LABPOOL filter blacklist hash=…` for the same hash. Only
clearing the persisted pool (or using new hashes without the poisoned entry in
the slot) recovers; restarting the node alone does not.

**Finding 3, medium - `prepare-buy` must carry the bundle's buy nonce.** The
prepared artifact used `-pool-nonce 1` while the bundle was at `nonce 0`, so
`pool_delivery_key_is_bound` refused the possession proof. `bcn_estimateRawTx`
separates the two cases cleanly: old artifact -> `success false`, corrected
artifact -> `success true` with output `{"ok":true,"order":1}`.

Evidence: receipts and market views under `/tmp/fod-cer/`
(`market-order.json`, `market-settled.json`, `buyer-delivery-record.json`); the
run logs `/tmp/fod-cer/lab-diagnose.log`, `/tmp/fod-cer/lab-guards.log`,
`/tmp/fod-cer/lab-fresh.log`; preserved pools
`mempool-txs.stuck-20260929`, `…stuck-epoch4`, `…blacklisted-0448`,
`…poisoned-0456`, `…oog-0502`.

Lab repair, recorded for honesty: the miner's state store had diverged (its own
proposals failed root verification and no transaction was admitted); its
`idenachain.db` was rebuilt from the independent follower's block replay
(kept as `idenachain.db.diverged-0415`). Blocks 671 onward were produced after
that repair by the pinned `/tmp/idena-go-pinned`. Two nodes agreed at height 626
(head `0xf944a012…`, identical code hash and market digest). A log-only build
(`/tmp/idena-go-lab-bin`) was used for diagnosis only; its only changes are
`log.Warn` lines.

Limitations: both nodes run on this one host and share the account keys, so this
is independent configuration and state, not independent operators; the fee
numbers for `withdraw`, `refund`, `fulfil` and `sellUnit` are still the harness
figure plus the observed gap rather than live receipts; the refund leg needs an
order that outlives `DEADLINE_BLOCKS` (720), so it waits for the next window.

Next task: measure the withdrawal, refund, fulfilment and sale legs on the chain
and replace the scaled figures; run the refund/withdrawal journey on an order
that has passed its 720-block deadline; finish the session-to-utility journey
(the bought content decrypts - what remains is the client journey and the
utility grant); then the release-artifact pass (F1/F2) and the final acceptance
exercise.

### 2026-09-29 - F3 partial - the buy artifact proved the wrong pool nonce, and the lab validator then stopped including the corrected call

The buyer's possession proof is bound to the bundle's *current buy nonce*, and
the prepared artifact carried `nonce 1` while the chain's bundle was at
`nonce 0`. Nothing else was wrong with the buy.

| Surface | Result |
|---|---|
| stuck beacon queue | two same-nonce (8) beacon txs sat in the ceremony node's persisted pool (`mempool-txs.stuck-20260929/txs.json`) with no inclusion; after the file was moved aside and the node restarted, a fresh beacon over the same calldata (`nonce 8`, `epoch 3`) was mined at height 514, and the pool view reads `beaconBlock 514, units 2, reserve 1000000000000000000000` |
| buy artifact | `/tmp/fod-cer/prepared-buy.json` recovered `0x297174ca…` over pool nonce 1; mined as `0x3bc192dc2d5bc75604d64be815a02710da02373fa96836b1fc3e4f9e10ca0ded` at height 522 the call trapped (`RuntimeError: unreachable`) |
| corrected artifact | `/tmp/fod-cer/prepared-buy-n0.json` (pool nonce 0) is accepted by the node's own simulator: `bcn_estimateRawTx` -> `success true`, output `{"ok":true,"order":1}`; the old artifact under the same account nonce, epoch and fee -> `success false`, `RuntimeError: unreachable` |
| blocker | from the epoch-4 window (`startBlock 555`) the validator keeps proposing empty blocks (`Proposed block ... txs=0`) while its pool holds the valid buys (`0x955a6db078bdb6554022a10e603bb2940dd43b92fc4244b7027603b3af7e280c`, `0xd9beb9350490aaf5740b33e849184e53d9893d1294481bdd72d8af9a23b0a580`, `0x02bb485327b90bdc2384ae5cd1ec0e9e3b31cb9dc9c18c4f9d05d240ec1fd936`); several proposals are then dropped (`Proposed block is not found ... invalid block roots`) |
| sender asymmetry | the coinbase's epoch-4 beacon (`0x3658786946a9e0148c3594d2d703a1aab5e3b9bd171b2b51f89dd34f63c28903`) *was* included at height 598 in the same process, so the builder is not globally skipping contract calls: the external buyer account's calls are the ones that never enter a block |
| fresh-state control | an instrumented follower on the same chain state (`/tmp/idena-go-lab-bin`, RPC 19097, peer `12D3KooWQd5SNYtvHbzeu1zrNzzRxfaR6nHbLmFZFHvi2F9FJE4e`) logs `LABPOOL put decision ... txEpoch=4 txNonce=1 globalEpoch=4 rawNonce=2 accountEpoch=3 executable=true`, so the tx and the pool's admission rule are not the cause |
| two-node agreement | the miner (RPC 19095) and a follower with its own datadir, IPFS store and peer id (RPC 19097) both report height 626 and head `0xf944a012ee6d46eafb…`; `fod market` against each returns the same code hash `0cb87e7d…` and the same pool state digest `d84f184dc4efb6b374a1841defd0f968` (`beaconBlock 598, units 2, pending 0, nonce 0, strikes 0`) |

Evidence: the ceremony chain at height 616; the preserved pool files
`mempool-txs.stuck-20260929` and `mempool-txs.stuck-epoch4`; the market views
`/tmp/fod-cer/market-after-beacon3.json` and `/tmp/fod-cer/market-buy-ok.json`
(`units 2, pending 0, nonce 0`); the `bcn_estimateRawTx` outputs above.

Limitation: the lab validator's post-ceremony divergence is an environment
blocker, not an application defect; the earlier follower (`fod-v3-cer-2`) was
banned as `fork is smaller ... worse seed` and is stopped, and the new follower
agrees with the miner on the head. Paid supply stays disabled.

Next task: stand up one clean ceremony chain (fresh genesis, no restarts or
forked followers) or pin the validator's builder divergence for external
accounts, then run buy -> deliver -> receive -> confirm with the corrected
pool-nonce artifact.

### 2026-09-29 - A04/F3 - the pinned runtime traps on `pay_amount` for a zero-value call

The first on-chain registration was broadcast, included in block 398, and
**aborted inside the host**: `success false`, gasUsed 1005, error `backend error
in env method: pay_amount`. The trace:

- `vm/wasm/context.go` builds the contract context with `payAmount = tx.Amount`;
- a zero-value transaction carries no protobuf amount, so
  `tx.Amount` is nil (`common.BigIntOrNil`, `blockchain/types/types.go:904`);
- the binding's `cpay_amount` (`lib/callbacks.go`) calls `amount.Bytes()` on what
  the host returns, which panics on nil and the C shim reports the error above.

So on this pinned node **any call to a wasm contract that reads `pay_amount`
traps when the transaction carries no value**. Every escrow method whose rule
requires a zero value - `registerBundle`, `deliver`, `confirm`, `fulfill`,
`refund`, `withdraw`, `donateUnit`, `sellUnit`, `beacon`, `accounting`,
`poolView`, `orderPoolView` - could therefore never execute on chain; only the
value-carrying calls (`buy`, `addReserve`) could. The harness never saw it
because its `MinimalEnv` starts with a non-nil zero (`spikes/harness/env.go:88`).

The reference checkout stays read-only, and an app that only runs on a patched
node would not be independently runnable, so the fix is in the contract:

| Surface | Change |
|---|---|
| `contracts/escrow/src/lib.rs` | the twelve zero-value methods no longer read `pay_amount`; the doc at `pay_amount_value` records why, and states the replacement guarantee: value sent to such a call stays in the contract's balance as unattributed surplus - it creates no unit, order, credit or reserve entry, and no tracked total includes it. `buy` and `addReserve` still compare the value they receive |
| `spikes/harness/escrow_d0_regressions_test.go` | `TestD0ValueOnANonPayableCallIsRefused` becomes `TestD0ValueOnANonPayableCallBecomesSurplus`: the call succeeds, the contract holds the surplus, the pool view and `accounting`'s totals are unchanged |

Evidence, 2026-09-29:

- the failed registration receipt above (tx
  `0x50fc60b916a1b7530fbb4d1d6bed3afb169bfc3b3e61677d7cbc7aef016bba3e`);
- `cargo test --locked --offline` in `contracts/escrow` - 15 pass;
- `go test -count=1 -run 'Test(Escrow|Pool|Release|Registration|Session)' .` in
  `spikes/harness` - ok, including the updated regression;
- the module rebuilt (release 374,141 bytes `be6d9de6…`, hooks `b4dfafc9…`) and
  redeployed on the ceremony chain at `0xf8bab10cd96962d140f6fa7ba822974d48d3e0d5`
  (tx `0xddddf977…`, receipt `success true`), because the contract address is
  part of the statement the receipt proves.

Limitation: the D0 review's "a non-payable call refuses value" property is
unattainable on this runtime, since the refusal itself needs the call that
traps. The guarantee the release keeps is the one that matters for solvency:
such value can never buy a unit, open an order, or be withdrawn by anyone.

### 2026-09-29 - B1/B4 - the record carries raw CIDs, and the statement now reads them

The epoch-1 capture exposed an encoding mismatch the fixtures had hidden. A flip
submission keeps the CIDv1 value as its **raw bytes** (`cid.Bytes()` in the
submission attachment), so the identity record carries
`01 55 12 20 <digest>`; every fixture written so far put the *multibase text*
(`bafkrei…`) into the record instead
(`proofs/session-core/fixtures/session-bundle-vector.json` before this entry,
`tools/fod/session_bundle_regenerate_test.go`'s synthetic datadir). The guest
compares the record's bytes with the bundle's manifest entry
(`session::identity_matches_manifest`) *and* the blob's own CID with that same
entry (`blob_cid_multibase`), so no manifest could satisfy both for a real
record: a real-chain registration was never provable. Only a live capture could
show this; the fixtures are self-consistent by construction.

| Surface | Change |
|---|---|
| `proofs/session-core/src/session.rs` | `cid_matches` accepts a record CID as the manifest entry itself (the fixture shape) or as the multibase base32 text of the CIDv1 value (the chain shape); `multibase_base32` renders exactly the CIDv1/raw/sha2-256 form. `global_epoch` reads an absent field as epoch 0 (proto3 omits zero; a chain's first epoch is 0). Two tests added |
| `tools/fod/sessionbundle.go` | the manifest is the multibase text (`cidString`), refusing a CID this build cannot render; the blob witness names the same text |
| `tools/fod/session_bundle_test.go`, `session_bundle_regenerate_test.go` | the synthetic datadirs carry real CIDv1 bytes; the bundle and the fixture name the text |
| `proofs/registration-v2/host/src/main.rs` | both bundle shapes carry the manifest as text, so the host converts nothing (the earlier hex decoding is gone) |
| `contracts/escrow/src/verifier.rs`, `tools/fod/receipt_claim.go` | the pinned registration image id is the regenerated guest `7ee79d90a7c5a3c99bb76b6eab24ae532e9a4ff1f9fc23eecb1806eb5f9d86d9`; the escrow module is rebuilt from it |

Evidence, 2026-09-29:

- `cargo test --locked --offline` in `proofs/session-core` - 19 pass, including
  `a_raw_record_cid_matches_the_multibase_manifest` and
  `an_absent_epoch_reads_as_zero`.
- `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod` - ok.
- the real epoch-1 bundle now passes the guest: `journal 326 bytes, epoch 1, 1
  flips, inventory 198 -> verdict 222` (captured from the ceremony chain's
  second finished epoch).
- `cargo test --locked --offline` in `contracts/escrow` - 15 pass; the rebuilt
  module is 374,141 bytes `e3bbc16d…` (release) and 381,570 bytes (test hooks).

Limitation: the recorded fixture artefact and the harness's sealed-receipt test
belong to the previous guest, so they are regenerated in this pass
(`cargo run --release -- --prove`); until that lands, the escalation harness
fails on the stale seal, which is expected rather than a second defect. The
epoch-0 bundle captured before the manifest fix would need a re-capture, and its
state versions may already be pruned.

### 2026-09-29 - F3 - the second node agrees with the registry and the ledger

The ceremony chain now has an independently configured follower: its own datadir
and IPFS store, same genesis and swarm key, no automine
(`~/Library/Application Support/FlipsOnDemand/fod-v3-cer-2`, RPC 19096, IPFS
22513, peer `QmdBNSv952GdCj3z3vZKgqwFmATqFuMUTq7P7Taw2ter6o`). It found the
miner through mDNS (`IpfsConf.Profile: ""`) and synced the whole chain,
including the non-empty blocks, with `Internal.Bitswap.BroadcastControl.Enable`
set to false in its repo config - the same lab setting the earlier two-node
exercise needed.

Evidence, 2026-09-29:

- both nodes reported height 65 with the same head
  `0x92e49da27ca8859999906cdf272f68ffb77cba28e6f9a110d04f50f5f7725ea7`, and the
  follower logged `Node is synchronized` with one peer in its own shard.
- `contract_getStake 0x11ae19f5…` returned the same code hash
  `0x3b398ab0…` on both nodes.
- `dna_identity 0x19e7…` returned the same record on both: `Verified`,
  `madeFlips 1`, flips `[bafkreie7fhwchauzjipbivbybng3mgcpfiiqhl74pvwpa627jhkt2sajmm]`.
- `fod market -contract 11ae19f5… -network idena` against each node wrote
  byte-identical state files (sha256
  `af69190f8b7fcf540a17d524b1f88d559762e8e31f646db1472dbc3d764604ba`): block 88,
  0 listings, 0 orders, 0 credits.

Limitation: both nodes run on this one host and share the account key, so this
is independent *configuration and state reading*, not independence of machine or
operator. The registry is deployed but no bundle is registered yet.

### 2026-09-29 - B1/B4/F3 - the isolated chain ran a real ceremony, and epoch zero is unprovable

The two-anchor evidence no longer has to come from an external node: a fresh
lab genesis ran a real validation ceremony and `fod session-bundle` captured
the result. Two code paths that only fixtures exercised are now usable with
live data, and one refusal turned out to be a statement gap.

| Surface | Change |
|---|---|
| `proofs/registration-v2/host/src/main.rs` | `--session FILE`, `--blob FILE` and `--contract ADDRESS` (defaults: the recorded fixtures, so default behaviour is unchanged). A bundle captured by `fod` is recognised by its `format` field: its manifest carries the record's CID bytes hex-encoded, so the host decodes them back; a live bundle carries its anchors at the top level instead of a `sessions` array |
| `tools/fod/main.go`, `sessionbundle.go` | `session-bundle -blob-out FILE` writes the host's blob witness: the flip blob the identity record's CID names, plus the two epoch secrets derived from the node key. The blob comes from `-blob FILE` (fetched while the node ran) or `-rpc`. The capture cannot read the state database while the node holds it, so the two steps are separate |

The ceremony lab (`~/Library/Application Support/FlipsOnDemand/fod-v3-cer`):
fresh genesis, `FirstCeremonyTime` = start + 420 s, `ValidationInterval` 3600 s,
lottery 60 s, short session 120 s, long session 60 s, RPC 19095, IPFS 22512,
network 33411 with its own swarm key. One flip was submitted through the node's
own `flip_submit` (tx `0xae1b6567…`, CID `bafkreif5y7g27…`).

Evidence, 2026-09-29:

- `22:25:29` block 19 carries `FlipLotteryStarted`; `22:26:12` short session
  started; `22:28:29` long session; `22:29:29` after-long; `22:31:29` block 37
  finished the epoch (`validation failed, nobody is validated, identities
  remains the same`, which is what a one-identity ceremony can produce). The
  node then reports `startBlock 37`, the next validation at `01:26:12`.
- `fod session-bundle -datadir … -inventory-height 19 -verdict-height 37
  -author 0x19e7… -blob flip-blob.bin -blob-out blob.json` wrote both files:
  `author=19e7… epoch=0 flips=1 inventory=19 verdict=37`.
- the escrow was deployed on this chain from the release module
  (`code hash 0x3b398ab0…`); its address is taken from the receipt
  (`bcn_txReceipt`: `contract 0x11ae19f5…`, `success true`). `prepare-deploy`'s
  printed address is only a prediction: `-deploy-nonce` defaults to a fresh
  random 32-byte value, so a second prepare run predicts a different address.
  The receipt is authoritative.
- `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok
  (14.3 s); `go vet` clean. `cd proofs/registration-v2/host &&
  cargo test --locked --offline` - the sealed v2 artefact still verifies; `cargo
  run --quiet -- --negative` - every mutated witness refused.

**Finding (statement gap).** The captured bundle is refused by the guest as
`Malformed` (`the session bundle does not prove this author's epoch`). Cause:
the first epoch of a chain is 0, proto3 omits a zero value, so the inventory
global record carries no epoch field (`protobuf/models.pb.go`,
`ProtoStateGlobal.epoch`), and `session::global_epoch` returns `Malformed` when
the field is absent (`proofs/session-core/src/session.rs`). The node's own
decoder reads that record as epoch 0, so an absent epoch means 0, the same way
`global_period` and `identity_flags` already treat absent as the zero value.
The one-line fix belongs to a single regeneration pass: `fod-session-core` is
linked into the registration guest, so changing it moves the guest image id and
the escrow's `REGISTRATION_IMAGE_ID` and the CLI pin with it.

Workaround taken instead, so the released tuple stays untouched: the flips for
the *second* epoch were submitted on the same chain (tx `0x77df1c2b…`, CID
`bafkreie7fhw…`, mined, blob saved beside the first), and the anchors will be
captured at that epoch's ceremony (`01:26:12`), whose inventory record carries
epoch 1 explicitly.

Limitations. The registration proof over the epoch-1 bundle, the registration
call and the marketplace lifecycle have not run yet; the epoch-0 bundle is
recorded but provably refused. The lab node keeps running so the ceremony is
not missed; no remote host, credential or mainnet action is involved.

### 2026-09-29 - F3 partial - two isolated nodes agree on the same chain view

The final acceptance needs a play-money chain that two independently configured
nodes follow. The pinned reference binary runs one, and the version 3 escrow is
live on it.

| Item | Value |
|---|---|
| Binary | `/tmp/idena-go-pinned`, built from the read-only `idena-go` reference checkout; no consensus change |
| Chain | custom genesis, `Network` 33410, private `SwarmKey`, `Validation.FirstCeremonyTime` 1990000000 |
| Node 1 | `~/Library/Application Support/FlipsOnDemand/fod-v3-lab`, RPC 127.0.0.1:19093, `--automine --autoonline`, IPFS 22509, peer `QmVbMGqjNCpmeLrq1eVe88QkcNC1eoUeMJFUr7tugyCGWE` |
| Node 2 | `.../fod-v3-lab-2`, RPC 127.0.0.1:19094, follower without `--automine`, IPFS 22510, peer `QmNcMc7e82UNgNqFqgYjQTxsxZmGutqX9cmG5M9BXucUiB` |
| Contract | `ff6caf89a3e1fe097582a079f6588313a30b3209`, code hash `0x3b398ab0…`, deploy tx `0x61c462f0…` |

Evidence, 2026-09-29 - both RPC endpoints answer the same chain:

- `bcn_lastBlock` at height 48: hash
  `0x018f0429f9812904eea979bebe0fba70128f4c18f800ba35311c4bc059282964` on both
  nodes.
- `contract_getStake 0xff6caf89…` returns Hash `0x3b398ab0…` on both nodes.
- after `net_addPeer /ip4/127.0.0.1/tcp/22509/p2p/QmVb…` on node 2, its
  downloader logged `Full sync will be used`, applied the block batch, and
  reports `Node is synchronized`.

Two lab settings were needed for local peering; both are configuration, not
source changes:

1. kubo's `server` IPFS profile adds `127.0.0.0/8` to `Swarm.AddrFilters`, so
   the connection gater refused the loopback dial (`failed to dial … gater
   disallows connection to peer`). Both labs run with `IpfsConf.Profile: ""`
   and `Swarm.AddrFilters: null`.
2. kubo v0.42 defaults `Internal.Bitswap.BroadcastControl.Enable` to true. With
   that default no bitswap want reached the connected peer in either direction:
   `net_peers` showed 1 peer, blocks timed out after 30 s, and no bitswap log
   line appeared even with `GOLOG_LOG_LEVEL=bitswap=debug`. Setting
   `Internal.Bitswap.BroadcastControl.Enable: false` restored transfer: a block
   added on the other node was fetched in 13 ms.

Limitation and next task: this chain has no validation ceremony, so the two
anchors the version 3 statement and `fod session-bundle` need cannot be
produced on it yet, and the external live capture stays out of scope. The next
F3 step is to decide whether a fresh lab genesis with a near `FirstCeremonyTime`
and short intervals can run a real submission plus `ValidationFinished` for
those anchors, or to record the precise blocker. Paid supply stays disabled.

### 2026-09-29 - F1 - the release tuple checks out at the current revision

- `node release/check-integration.mjs` - ALL CHECKS PASSED, 11 of 11, including
  the session-content digest pin, the grant scope and the re-enrolment case.
- `node release/build-manifest.mjs` - writes for `1462782b8f15` (dirty
  worktree): HTML 275,082 bytes `42e48dfe…`, module 374,141 bytes `2fa9f8ab…`,
  0 forbidden operators, 0 remote tags.

Limitation: this is the working-tree tuple, not a release revision; the
manifest records the base commit with `worktreeDirty: true`, so regeneration
against a delivered revision remains F1's closing step.

### 2026-09-28 - B3 - the utility can pin what a session was

A session id is a name the provider chooses. A utility that pinned only the name
accepted whatever content the provider later gave that name, so "I accept
session X" was a claim about spelling rather than about the session
(`SPEC.md` 8.3). The gateway now publishes and signs a digest of the session's
content, and a utility can pin it.

| Surface | Change |
|---|---|
| `gateway/src/pool.ts` | `sessionContentDigest`: SHA-256 over the session and utility ids, the rules, the stated provenance, and every challenge with its CID, its answer digest and a digest of the pictures it shows |
| `gateway/src/routes.ts` | `/session/policy` publishes it, and the signed result carries it |
| `utility/src/results.ts` | a result without a 64-hex `contentDigest` is `malformed_result` |
| `utility/src/policy.ts`, `access.ts`, `server.ts` | optional `sessionDigests` (session id to digest); a result whose digest differs is refused as `wrong_session_content`; `/policy` publishes the pin |
| `release/check-integration.mjs` | builds the utility's policy with the digest the gateway publishes, checks the signed result names it, and grants with the pin in force |

Evidence, 2026-09-28: gateway 25 tests pass - the digest is stable for one file
and moves with an answer digest, the pass threshold or the stated provenance,
and the signed result and the policy name the same value; utility 14 tests pass
- a pinned session with matching content grants, another digest is refused, an
unpinned session still grants on its name, and a result with no digest is
malformed; both typechecks are clean; `release/check-integration.mjs` passes 11
checks including the two new ones; the app's own walk still verifies the signed
result in the browser.

Limitations. The digest pins what the provider says the session is. It does not
prove the answers are correct, that the pictures are meaningful, or that a pass
was earned rather than issued, which is the boundary `SPEC.md` 8.3 draws; a
utility that wants chain-backed evidence checks the marketplace instead (5.5).

The same day, the second half of that boundary: the connection file
(`fod.provider-config` v1) may now record `publicKeyHex`, and the interface
verifies a result against it. A host that serves a different key now reads as
`not verified` rather than verified, so a participant who keeps the file no
longer depends on the host to serve its own key (`app/src/provider.ts`,
`app/src/views/Session.tsx`, `SPEC.md` 8.2). The walk covers it: a connection
file that pinned another key cannot show a verified result. The built interface
is 275,082 bytes.

### 2026-09-28 - D3 - buyer escrow is not spendable reserve

`buy` used to add the caller's payment to the bundle's reserve and `refund`
subtracted it from the same number, so what a seller could be paid from and what
a buyer was owed were one balance (`REVIEW.md` A03). They are separate
liabilities now.

| Move | Where the money goes |
|---|---|
| `addReserve` | the funder's amount joins `reserve` and `reserveTotal` |
| `buy` | the exact quote joins `pending` and `pendingTotal`; the reserve is untouched, and the quote stays a function of `reserve` and `units` alone |
| `confirm`, `fulfill` | that order's payment leaves `pending` for `reserve`: the delivery settled |
| `refund` | that order's payment leaves `pending` for the buyer's credit, with the strike in the same write |
| `withdraw` | consumes the caller's credit and the contract's balance together, as before |

`accounting` now publishes `reserveTotal`, `pendingTotal`, `creditTotal` and
`liquid`, so `liquid >= reserveTotal + pendingTotal + creditTotal` is checkable
without iterating storage, which a contract cannot do. The harness asserts that
relation after every move of its pool sequence.

| Surface | Change |
|---|---|
| `contracts/escrow/src/pool.rs` | `Pool` carries `pending`; `buy` holds the payment there, `settle` and `refund` release it. The old `reserve * units` product property described a pool whose reserve grew on every purchase; it is replaced by what this pool promises: the pool's own funds fall only by a payout, and a payout comes out of the reserve |
| `contracts/escrow/src/lib.rs` | `PoolBundle.pending` at the end of the record, the `reserveTotal`/`pendingTotal` running totals, the settlement and refund transitions, and the widened `accounting` and `poolView` |
| `tools/fod/market.go` | reads the 148-byte record and publishes `pending` on each listing |
| `spikes/harness/*` | the sequence test asserts the four-number invariant after every move; the A03 containment test finds the buyer's 10,000 in escrow while the reserve stays what the seller funded; the CLI-calldata test expects the payment in escrow |

Evidence, 2026-09-28: `contracts/escrow` - 15 pass, including the new cases (a
pending payment becomes reserve only on settlement, a refund releases escrow and
leaves the reserve alone, and a pending refund survives any number of supply
payouts); `spikes/harness` - every test passes except the stale-file check
recorded above; `tools/fod` - `ok`, 14.4 s; both module variants rebuilt and
lowered, the manifest recording 374,141 bytes `2fa9f8ab…`, 0 forbidden operators,
0 remote tags.

Limitations. Supply payouts stay disabled (`SUPPLY_PAYOUTS_ENABLED`), so the
separation is enforced and tested but no live payout exercises it yet; the model
that decides the subsidy and the issuance rule is still the owner's decision 1.
The quote's meaning changed with the separation: a purchase no longer raises the
next buyer's quote until its delivery settles, which is the honest reading of
money that is not earned until delivery, and it is recorded here rather than
left implicit.

### 2026-09-28 - D1 - the author owns the beacon and the gift

Two moves let an outsider change a bundle they do not own. Both are now bound
to the address the registration proof named, which is a fact of the bundle
rather than an operator's choice.

| Move | What it did | What it does now |
|---|---|---|
| `donateUnit` | added a free sale unit to any registered bundle, so a buyer could donate first and then pay a smaller quote: one more claim on the same reserve, moved without paying for it (`REVIEW.md` A05) | only the bundle's registered author may add a unit |
| `beacon` | anyone could refresh any bundle's beacon, and a fresh beacon is what `buy` requires and what the interface's Buy action keys on, so an outsider could make a bundle with no desk look live, lure a purchase, and leave the author a strike when it refunded | only the bundle's registered author may set it |

Both rules are narrower than what the owner has not yet decided - who may donate
and in what quantity, and who may advertise availability - and they are recorded
as containment rather than as the accepted model. Delivery stays permissionless:
any desk may publish the buyer-bound ciphertext, and the proof fallback is
unchanged.

| Surface | Change |
|---|---|
| `contracts/escrow/src/lib.rs` | `donateUnit` and `beacon` require `caller_address() == bundle.author`, with the reasoning at each call site |
| `tools/fod/serve.go` | a broadcasting desk leaves another author's beacon alone instead of writing a call the node would refuse; a prepare-only desk still writes the file and the refusal surfaces at the node |
| `spikes/harness/escrow_beacon_test.go` | the stranger's beacon is refused and moves nothing; the author's is accepted and still creates no unit, credit, or liability |
| `tools/fod/serve_test.go` | the plan skips a bundle whose author is not the desk, and the broadcast fixture serves its own bundle |

Evidence, 2026-09-28: `spikes/harness` - every test passes except the stale-file
check recorded above; `tools/fod` - `ok`, 14.6 s; `contracts/escrow` -
14 pass; both module variants rebuilt and lowered, `release/MANIFEST.json`
regenerated with the module at 365,821 bytes `e43c0d16…`, 0 forbidden operators
and 0 remote tags.

Limitation: the beacon is still a claim rather than a promise. The author can
beacon a bundle with no working desk, and a buyer who pays then waits out the
refund window. What this rule removes is an outsider doing that to somebody
else's bundle; presenting, pricing or bonding the beacon as a service level
belongs to the owner's availability decision.

### 2026-09-28 - C2 - confirmation names the payload the buyer checked

`deliver` lets anyone replace a pending payload (A06's repair), and `confirm`
took only an order id, so a replacement published between the buyer's check and
their confirmation settled the order on bytes the buyer never saw. The
confirmation now carries the digest of the bytes that passed the buyer's own
check, and the escrow hashes what it holds before it settles.

| Surface | Change |
|---|---|
| `contracts/escrow/src/lib.rs` | `confirm` takes the order id and the SHA-256 of the recorded delivery, refuses anything else as `ciphertext_mismatch`, and leaves the delivery readable after settlement. It is the same construction `fulfill` binds a proof to, so both settlement paths agree on what "the ciphertext" is |
| `tools/fod/pool_receive.go` | the record `receive -out` writes names the digest of the ciphertext that run checked |
| `tools/fod/prepare_confirm.go` | the call is the order id (8 bytes, big-endian) then that digest; the flag form needs `-delivery FILE` and refuses a record for another order or contract, the request form needs `ciphertextSha256`, and neither defaults to what the chain holds at signing time |
| `tools/fod/pool_receive_test.go`, `spikes/harness/*` | the confirm call sites pass the digest, and A06's regression now replaces the payload after the buyer's check |
| `app/src/views/Trades.tsx`, `app/verify.mjs` | the client's confirm action now reads the record `fod receive -out` wrote instead of naming whatever the snapshot holds, and the walk hands it one; the built client is 274,751 bytes `6afeb683…` |
| `tools/fod/fees.go`, `docs/SPEC.md`, `release/README.md` | the call's signed size is 160 bytes (the argument grew; the gas is still the runtime measurement of the smaller call) |

Evidence, 2026-09-28:

- `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod` - `ok`,
  14.3 s, including the digest binding, the refusal of a call without one, and
  the request envelope that has to carry it
- the same command in `spikes/harness` - every test passes except the stale-file
  check recorded above, including `TestD0JunkFirstDeliveryDoesNotOwnTheSlot`
  (junk first, honest replacement, attacker replacement, stale confirmation
  refused, honest payload re-published, that confirmation settles, and a further
  delivery refused), `TestPoolCallSequenceAndInvariants` and
  `TestPoolSupplyRefundAndStrikes`
- `cargo test --locked --offline` in `contracts/escrow` - 14 pass
- `cd app && npm run typecheck && npm run build && npm run verify` - the built
  client's own walk passes every check, including the one that reads the opened
  record and requires the request to carry its digest
- both module variants rebuilt and lowered; `node release/build-manifest.mjs`
  records the module at 365,723 bytes `3e0dacb0…` and the client at 274,751
  bytes `6afeb683…`, with 0 forbidden operators and 0 remote tags
- `node release/check-integration.mjs` - all 9 checks pass

Limitation: the escrow checks that the digest names the payload it holds. It
does not check that the buyer opened that payload, so a buyer who confirms a
digest of bytes they never decrypted still settles - their money, their choice.
What the binding removes is an outsider settling the buyer on a payload the
buyer never named.

### 2026-09-28 - B2/C1 - the statement publishes the inventory commitment and the network

The two open gaps had one cause: the values a reader needed sat inside a fold
nobody could recompute. Version 3 publishes them, the escrow checks the network,
the pool record carries the inventory commitment, and the buyer's tool checks a
seller's advisory against it. One receipt was re-proved for the whole change.

| Surface | Change |
|---|---|
| `proofs/session-core/src/statement.rs` | layout 262 to 326 bytes, version 3: `inventory commitment` at 198 and `network digest` at 294, a `network_digest()` helper, and layout/digest tests |
| `proofs/registration-v2/methods/guest/src/main.rs` | writes both fields; the version it checks comes from the statement constant |
| `proofs/registration-v2/host/src/main.rs` | sends `statement::STATEMENT_VERSION` rather than a literal and writes the artefact's `statement` label from it; the host asserts the two published fields against the fixture |
| `contracts/escrow/src/verifier.rs` | `REGISTRATION_JOURNAL_BYTES` 326, `REGISTRATION_JOURNAL_VERSION` 3, and the regenerated `REGISTRATION_IMAGE_ID` |
| `contracts/escrow/src/lib.rs` | refuses `foreign_network`, stores the inventory commitment in the pool bundle, publishes it in the pool record, and names the offsets the checks read |
| `tools/fod/*` | the CLI pins the same network name and image id, decodes the version 3 journal, reads the 132-byte pool record, and refuses a purchase whose advisory does not recompute to the chain's commitment; the fee table's two sizes grew by the journal's 64 bytes |
| `spikes/harness/registration_v2_test.go` | the receipt fixture's offsets follow the version 3 layout |

Evidence, 2026-09-28:

- the guest, executed: `cargo run --release` in `proofs/registration-v2/host` -
  326-byte journal, version 3, the inventory commitment recomputed over the
  fixture's manifest and the network digest equal to `sha256("idena")`
- the receipt: `cargo run --release -- --prove` - 1,001.7 s, 7,864,320 cycles in
  8 segments, 256-byte seal, image id `29139ba0…`, claim `5b567330…`;
  `cargo test --locked --offline` in that host re-verifies the artefact against
  the guest built from this tree
- the modules: both variants lowered with `wasm-opt`, each with zero
  bulk-memory and zero float operators; test-hooks 368,817 bytes, release
  365,660 bytes
- `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod` - `ok`,
  15.0 s, including the frozen-journal round trip and every mutation case
- `cargo test --locked --offline` in `proofs/session-core` (17 tests) and
  `contracts/escrow` (14 tests) - pass
- `node release/check-integration.mjs` - all 9 checks pass
- `node release/build-manifest.mjs` - writes the manifest for `1462782` (dirty
  worktree): HTML 273,509 bytes `82e8d113…`, module 365,660 bytes `2069f224…`,
  forbidden operators 0, remote tags 0, pinned identifiers agree with the proved
  artefacts, registration statement `fod:registration:v3`

Limitations. The live capture over a real closed session is still the external
input B1 and B4 need. `spikes/harness`'s
`TestCliPreparedDeployDerivesTheNodesAddress` fails against
`output/cli/prepared-deploy.json`, an untracked file generated for the suspended
mainnet exercise against the previous module: a fresh clone skips that test, and
the file has to be regenerated when the deploy exercise resumes. The fee table's
two registration sizes are the previous measurement plus the journal's 64 bytes
rather than a fresh gas measurement, and their notes say so.

### 2026-09-28 - reconciliation and Package A - the optional adapters fail closed

Reconciled at HEAD `1462782`, clean tree, 231 commits ahead of `origin/main`,
no repository `AGENTS.md`: the superseded objective, the new one and the
package order are in section 0 and `SPEC.md` 10.3. A1-A5 were re-checked in
source before any edit. The A02 anchor half and the D0 containment were
re-confirmed as closed, so the earlier "arbitrary anchor" and "immediately
drainable supply" findings are not re-reported as live exploits; the open
items are the ones named in section 0's table.

| Item | Change | Files |
|---|---|---|
| A1 | no request field carries an identity assertion; a policy that requires eligibility is refused, and `/policy` says `identityVerification.supported: false` | `utility/src/access.ts`, `utility/src/server.ts`, `utility/src/policy.ts` |
| A2 | the request URL is parsed inside the guard, and no rejected promise can escape either server callback | `gateway/src/http.ts`, `utility/src/server.ts` |
| A3 | the limiter is keyed by authenticated participant or client address, never by a presented token, and both limiter tables are bounded and expiring | `gateway/src/http.ts`, `gateway/src/session.ts`, `utility/src/server.ts` |
| A4 | `opensAt`, `closesAt` and token expiry are enforced on enrolment, answers and statement issuance, and the pool loader refuses a window that closes before it opens | `gateway/src/session.ts`, `gateway/src/routes.ts`, `gateway/src/pool.ts` |
| A5 | the grant store is namespaced by utility, provider and participant, expires as it is read, and is published as local rather than durable | `utility/src/access.ts`, `utility/src/server.ts` |

Commands and results, 2026-09-28 against the working tree above:

- `cd gateway && npm test` - 24 pass, 0 fail, 254 ms. The new regressions are a
  raw `Host: [` request answered `400` with the next ordinary request still
  served in the same process; four rotating invalid tokens reaching `429` on
  the fourth; an authenticated participant keeping its own allowance while the
  address bucket is exhausted; and the window boundaries, where `opensAt` is
  open, `closesAt` is closed, a statement is refused before opening and a
  participant's unexpired token can still collect one after closing.
- `cd utility && npm test` - 12 pass, 0 fail, 153 ms. A policy that requires
  identity refuses a valid signed result and stays refused when the body
  carries `identityVerified` or an equivalent object; a grant reports
  `identity: "not-verified"`; the quota key separates providers and utilities;
  an expired grant stops counting; and `/policy` publishes the local scope.
- `npm run typecheck` in `gateway/` and `utility/` - clean.

Limitation: these are process-level regressions against the reference adapters,
which stay optional. They establish nothing about the chain-backed guarantees
in packages B-F, and A1's refusal is a fail-closed placeholder rather than an
eligibility check: the path it waits for is the chain-committed evidence B1 has
to establish first. Next: Package B's temporal inventory question, and the
section 2 owner decisions that gate their dependent rules.

### 2026-09-28 - C1 finding - the CID set is not checkable from public data

Read while scoping C1, against `1462782`. The pool record the buyer reads
publishes `author`, `manifestCommitment`, `coveredFlips`, the reserve and the
unit counters (`contracts/escrow/src/lib.rs:1304-1322`). The buyer's tool
matches the advisory's author to that record, re-fetches every CID, and then
copies the chain's commitment into its own result
(`tools/fod/pool_buy.go:220-230`), so the CID set and the commitment are named
together while no relation between them is checked.

What would close that, and why public data cannot close it today:

- The commitment over the CIDs is
  `sha256("fod.bundle-manifest:v1" | author | epoch | 0x00 | cid ...)` over the
  sorted list (`tools/fod/export.go:81-93`, `proofs/session-core/src/statement.rs:154-161`).
  A buyer can compute it from a candidate CID set.
- The public journal does not carry it. The journal carries the folded
  `manifest_commitment`, `Sha256(policy)` and `Sha256(consensus)`, and no
  network field (`proofs/session-core/src/statement.rs:28-46`).
- The fold is one-way:
  `sha256("fod:manifest:claim:v2" | inventory | inventory height | inventory root
  | verdict height | verdict root | len||consensus | len||policy | len||network)`
  over the raw strings (`proofs/registration-v2/methods/guest/src/main.rs:218-227`).
  Only digests of two of those strings are published, so the inputs cannot be
  recovered from the journal.

No buyer and no independent node can therefore recompute the commitment from a
candidate CID set with what the chain publishes. Closing C1 needs a statement
change - publish the inventory commitment, or commit a content-addressed
manifest whose body the buyer can fetch - which means guest, escrow offsets,
identifiers and a proof regeneration; otherwise the ordinary purchase path has
to say plainly that its content check observes the seller's advisory and is not
a proof about the committed inventory.

Established versus impossible: the chain commitment is genuine and authenticated;
what is missing is a published component of its preimage. That is a
statement-design gap, not a chain limitation, and it cannot be repaired by a
trusted fetch service or a project-hosted indexer. This is the first input
Package B needs; C1 waits on it, and the section 3 A-package work is unaffected.

### 2026-09-28 - B1 - the inventory anchor has to be past the submission window

The temporal half of the inventory claim, closed against the pinned chain rules
(`SOURCES.md` F-99). A flip submission is valid only while the chain reports
`NonePeriod`: `validateSubmitFlipTx` returns `LateTx` from `FlipLotteryPeriod`
on (`blockchain/validation/validation.go:349`), and inside one epoch the period
only advances - `ValidationFinished` is the single transition that returns it
to `None`, and that block increments the epoch. So a state version carrying
`Epoch == N` and `validationPeriod >= FlipLotteryPeriod` holds epoch N's final
flip set, and the earlier anchor the review objected to does not.

Change: the two-anchor verifier now requires that period on the inventory
anchor, and the capture refuses to build a bundle without it.

| File | Change |
|---|---|
| `proofs/session-core/src/session.rs` | `global_period` reads `ProtoStateGlobal.validationPeriod`; `verify_with` refuses an anchor below the lottery with the new `Refusal::InventoryStillOpen`; three tests, including a fully valid proof rebuilt over an open anchor |
| `tools/fod/sessionbundle.go` | `readSessionBundle` refuses an inventory record whose period still permits a submission, naming the height and period |
| `tools/fod/session_bundle_test.go` | the fixture builder takes the inventory period; a new test reads both an open and a frozen record |

Commands and results, 2026-09-28:

- `cargo test --locked --offline` in `proofs/session-core` - 16 pass, 0 fail.
  The new cases are the period decoder (absent field reads as `NonePeriod`, a
  wrong wire type is `Malformed`), a valid proof rebuilt over an open anchor
  refused as `InventoryStillOpen`, and the last ceremony period still
  verifying.
- `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod` -
  `ok`, 14.9 s; `go vet` clean.
- `GOFLAGS=-modfile=go.local.mod go test -count=1 -run 'Test(Registration|Escrow|Pool|Release|Session)' .`
  in `spikes/harness` - `ok`, 2.4 s.

Limitation: the guest's own image id follows its sources, so the sealed version 2
receipt no longer matches the guest this tree builds, and
`proofs/registration-v2/host/tests/sealed_artefact.rs` is expected to fail until
the artefact is regenerated. That test was not run, and the regeneration is
deliberately left to Package F's single pass, because the journal layout is
unchanged and Package B has a second guest change (the network binding) still to
come. No escrow offset, CLI constant or manifest entry is invalidated by this
change. The live capture over a real closed session remains the external input
B1 still needs, and it is not authorized on the designated host under the
current plan.

### 2026-09-28 - C1 interim - the buyer's check says what it established

The C1 finding above says the commitment over the advisory's CID set cannot be
recomputed from what the chain publishes, so an advisory-only purchase cannot
claim the committed inventory. Until the statement carries that commitment, the
prepared purchase stops implying that it did.

| File | Change |
|---|---|
| `tools/fod/availability_check.go` | the record carries `binding` (`advisory` or `unchecked`); the chain's `manifestCommitment` is no longer copied into a record that proved no relation to it; `Complete()` is false for an unchecked purchase, and `uncheckedAvailability()` writes the reason |
| `tools/fod/pool_buy.go` | a purchase prepared without an advisory records that record instead of carrying none; the printed result says what the advisory check does and does not establish |
| `tools/fod/availability_check_test.go`, `tools/fod/pool_buy_test.go` | the advisory path asserts its binding, and a new case prepares without an advisory and requires `binding: "unchecked"` with `Complete() == false` |

Commands and results: `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in
`tools/fod` - `ok`, 14.4 s; `go vet` clean.

Limitation: this is honesty about a gap, not its repair. C1 closes when the
statement publishes the inventory commitment, or commits a manifest whose body
a buyer can fetch, and that is a guest and escrow change to be batched with B2
before the single regeneration. The prepared-buy file changed shape and `fod
submit` decodes strictly, so a file prepared by an earlier build is refused with
the offending field named and has to be re-prepared; nothing was deployed, so no
live artefact carries the old shape.

### 2026-09-28 - F1 pre-check - the integration check was failing, and the manifest refuses

Found while checking that this turn's changes had not disturbed the release
tooling. Both findings were confirmed pre-existing by running the same scripts
against a pristine copy of HEAD in a temporary directory
(`git archive HEAD | tar -x -C <tmp>`), so neither belongs to the package
changes above.

| Check | Result |
|---|---|
| `node release/check-integration.mjs` at HEAD | 5 of 9 checks failed: the customer scored 1 of 2 correct, so no passing result was signed and the utility refused with `outcome_not_accepted` |
| the same script after the repair below | `ALL CHECKS PASSED`, 9 of 9 |
| `node release/build-manifest.mjs` at HEAD and now | refuses to write: the contract and the CLI pin `b4a32e7f…` while `proofs/registration/dist/registration.json` carries the version 1 artefact `5f0a0c0d…` |

Cause of the integration failure: `gateway/example-pool.json` carries real flip
content for `c1`, so that challenge is answered by choosing a sequence and its
recorded digest is the digest of `right`; the script still sent the
pre-content answer `first answer`, which scored wrong. `release/check-integration.mjs`
now answers `right` in both sessions, with a comment recording why.

Limitation: that script is a local fixture journey, not an independent-node
one, and it says nothing about the chain-backed packages. `release/build-manifest.mjs`
still needs F1's own repair - read the version 2 artefact, and fail on the drift
it currently refuses to write through - and its refusal to write is the correct
fail-closed behaviour rather than a second defect.

### 2026-09-26 - D1 - the contract authenticates the session header itself

The first D1 question was whether a session anchor has to be a checkpoint, an
explorer answer or an operator attestation. It does not: the runtime hands the
contract the header the executing node holds for a height, and the contract can
verify the session fields against it.

| Item | Evidence |
|---|---|
| host surface | `env.block_header(height)` exists in the runtime binding (`idena-wasm-binding/lib/callbacks.go`), and the node answers it from its canonical chain (`idena-go/vm/wasm/wasm_env.go`, `blockchain/blockchain.go:2623`), charging 500 wasm gas (`ReadBlockGas = 5`) |
| encoding | the header is protobuf (`blockchain/types/types.go:504`, `Header.ToBytes`), so the parser is a bounds-checked wire walker, not RLP; field numbers recorded in `SOURCES.md` F-96 |
| spike | `spikes/header-anchor` (4,122 bytes, sha256 `1ee33567...`) verifies height, block hash, state root, identity root, seed and the `FlipLotteryStarted` flag; refusals abort the call, as after D0 |
| measurement | a 730-byte proposed header is accepted at 147,537 wasm gas, 0.03% of the 512,000,000 call ceiling |
| refusals | `TestHeaderAnchorRefusesAnythingElse`: wrong block hash, state root, identity root or seed, a height with no header, a skip block, a proposed block without the lottery flag, a truncated message, an over-long field and both shapes at once |
| fixture limit | the vectors are encoded by the pinned node's own protobuf package but carry synthetic field values; a real captured header replaces them at capture time |
| next for D1 | superseded as the *plan* by the two-anchor bundle below, which the same source reading made possible; the fold measurement stays useful (each leaf costs about 12,453 cycles) |

### 2026-09-26 - D1 - the two-anchor bundle replaces the assignment replay

While checking what a session proof still needs, the pinned source showed that
the chain already commits both halves of a session in the author's own identity
record - the flips during the epoch, the verdict flags at its end. That removes
the candidate list, the lottery assignment and the answers from the proof
entirely (`SOURCES.md` F-97).

| Item | What it means |
|---|---|
| inventory | the author's state-tree leaf at a height in the flip phase carries `Flips` (every CID the author submitted for that epoch) and the global leaf at the same height says `Epoch == N`; the manifest's CIDs must equal that list |
| verdict | the same author's leaf at the `ValidationFinished` block carries `LastValidationStatus` (at least one reported, at least one not qualified, all not qualified) and the global leaf says `Epoch == N+1` |
| label, not per-flip | the chain stores verdicts per author and epoch, not per flip, so the provable claim is a bundle label: clean, contains reported, or weak. That matches the owner's rule that session-level verifiability is enough and reported flips may be offered but not expected to sell |
| cost | four single-leaf folds (two identity, two global) at 12,453 cycles each, plus two header checks at about 147,537 wasm gas each - two orders of magnitude below the proof-carrying call budget |
| capture | a state version is readable for about 100 blocks (`MaxSavedStatesCount`, one version per height), so a bundle's two leaves must be captured at their heights by a live node or an immediate datadir copy. Producing them is an off-protocol service; verifying them is not |
| retention limit | a node that fast-synced after an anchor height has no header for it, and `block_header` then returns nothing, so the check fails closed. Anchors must sit inside the retained history of the nodes that execute and validate the registration; the CLI must say so when a bundle is older than the local node's floor |
| what it replaces | the D1 plan to replay candidates, seed, assignment and answers inside the guest. That lane is not deleted from the specification's fallback if the two-anchor bundle fails its live test, but no further work goes into it while this one is measured to be feasible |
| status | design recorded from source; the CLI bundle producer, the guest fold and decode, the contract's anchor call in `registerBundle`, and the adversarial tests are the next package |

### 2026-09-26 - D1 - the session-bundle evidence core decodes the node's own encodings

| Item | Evidence |
|---|---|
| fixture | `spikes/session-bundle/core/fixtures/session-bundle.json`: four sessions (clean, contains-reported, weak, and a verdict that still carries flips) over two state versions each, encoded with the node's own `state.Identity`/`state.Global` protobuf, written into a tree built by the pinned IAVL fork and proved with `GetWithProof`, with the generator recorded in the file's `source` note |
| core | `spikes/session-bundle/core` (`no_std`, allocation-free): folds the four leaves with the existing `fod-iavl-proof-core`, decodes the records itself (flips in record order against the manifest, validation status flags, global epoch), and derives the label |
| policy | weak when any not-qualified flag is set, contains-reported when the reported flag is set, clean otherwise; the raw flags and the flip count come back with the label, and proto3's omitted zero is treated as the clean case |
| tests | `cargo test --offline` in that crate: 8 passed. The four sessions reach their expected outcome, and a changed CID, a short or extended manifest, swapped roots, another author, a wrong epoch, and an inventory leaf reused as the verdict are all refused with their own codes |
| limit | these synthetic proofs have one path item each; deep real paths stay covered by the IAVL spike's recorded proofs |
| next | the CLI capture that emits this bundle from a datadir, the guest that wraps the core, and the contract's anchor check for both heights |

### 2026-09-27 - D1 - a real Groth16 receipt for the version 2 statement

| Item | Evidence |
|---|---|
| proof | `RISC0_SKIP_BUILD_KERNELS=1 RISC0_DEV_MODE=0 cargo run --release -p registration-v2-host -- --prove` over the fixture witness: 1,041.8 s, 7,864,320 cycles in 8 segments, 262-byte journal, 256-byte seal |
| image id | `f8ef4e01d05c0ac5c01866706207a64507e8ea685596952509bf522b00161579` |
| claim digest | `733da5fed0afd3cb02e6ce9e7aa09c7241461033a0c18f5bcf5601b256964039` |
| artefact | `proofs/registration-v2/dist/registration-v2.json`: statement `fod:registration:v2`, the journal, the seal, both digests, the cycle count and the proving time |
| verifies again | `cargo test --locked --offline -p registration-v2-host` rebuilds the claim as `ReceiptClaim::ok(image_id, journal)` and verifies the seal with one pairing check, and the recorded image id and claim digest have to match what it verified |
| refusals | `cargo run --release -p registration-v2-host -- --negative` now runs four mutations: a tampered blob (its CID no longer matches), a wrong secret (the blob does not open), an extra manifest CID (no matching blob) and a wrong epoch (`EpochMismatch`). Every one is refused |
| next | the escrow's version 2 offsets and its two `block_header` checks, the CLI witness builder, then the coordinated swap of the escrow's constants, the release artefact and the manifest |

### 2026-09-27 - D1 - the version 2 registration guest executes over real-encoder fixtures

| Item | Evidence |
|---|---|
| program | `proofs/registration-v2` (own workspace, own image id, so the pinned version 1 guest and its sealed receipt stay valid while this one is built) |
| guest | reads the two-anchor session bundle, the blobs and the two secrets; folds all four leaves with the zkVM's accelerated SHA-256; requires the inventory record's flips to equal the manifest in record order, the verdict record to have no flips and to name the next epoch; checks every blob against the CID its record carries and opens it with the committed secrets; commits the 262-byte version 2 journal |
| fixtures | the witness comes from `proofs/session-core/fixtures/session-bundle-vector.json` (sessions built over the CID of the node-built blob in `proofs/registration/vectors/blob-v1.json`) plus that blob vector, so every input is produced by the node's own encoders |
| execution | `cargo run --release -p registration-v2-host`: journal 262 bytes, version 2, covered 1 flip, flags 0, with the author, both heights and both roots read back from the fixture |
| refusal | `cargo run --release -p registration-v2-host -- --negative`: one blob byte flipped, and the guest refused with `a blob does not hash to the CID its record carries` |
| next | the escrow's offsets and its two `block_header` checks, the CLI witness builder for a capture, then `--prove` and the coordinated image-id, seal, constant and manifest swap |

### 2026-09-27 - D1 - the evidence layer and the version 2 statement move into the proof tree

| Item | Evidence |
|---|---|
| new crate | `proofs/session-core`: `iavl` (the fork's single-leaf fold), `session` (the two anchored leaves, the record decode, the bundle label) and `statement` (the version 2 journal and the manifest commitment) |
| statement v2 | a 262-byte journal: version 2, contract, author, epoch, covered flips, inventory height + state root, verdict height + state root, validation flags, manifest commitment, key commitment, policy digest, consensus digest. The layout table sits in the module, and the escrow's offsets will be checked against it |
| manifest commitment | domain `fod:manifest:claim:v2`, binding both anchors and their heights; the inventory commitment keeps the version 1 bytes the CLI already writes |
| tests | `cargo test --locked --offline` in `proofs/session-core`: 13 passed (8 evidence, 5 statement) |
| the trap this confirmed | adding three unused modules to `fod-registration-core` moved the pinned guest's image id, and `cargo test -p registration-host` (the sealed-artefact test) failed exactly as that crate's own comment warns. The modules were reverted; the version 2 statement lives in its own crate, and the same test passes again, so the shipped seal is untouched |
| retired | the `spikes/session-bundle` prototype is removed and its fixture moved to `proofs/session-core/fixtures/` |
| next | wire the version 2 guest and host onto this crate, then the escrow's offsets and its two `block_header` checks |

### 2026-09-27 - D1 - the CLI captures a two-anchor session bundle

| Item | Evidence |
|---|---|
| command | `fod session-bundle -datadir PATH -inventory-height N -verdict-height N -author ADDRESS -out bundle.json`, read-only, wired into the command map and the help text |
| producer | `tools/fod/sessionbundle.go`: proves the global leaf (`0x03`) and the author's identity leaf (`0x02 || addr`) at both heights, checks every proof against the version's own root before writing it, decodes the records with the node's own `state.Identity`/`state.Global`, takes the manifest from the inventory record's flips in record order, and refuses an inventory height without flips, a verdict height that still lists flips, or epochs that do not differ by one |
| datadir layout | the test writes a real datadir the way the node lays it out - a LevelDB named `idenachain`, the stored state prefix, the tree under that prefix, one version per height (`writeSessionDatadir`) |
| tests | four session tests in `tools/fod`: both anchors read back with roots, keys, paths, manifest and flags; inverted or identical heights and an unknown author are refused; an uncleared verdict and a mismatched epoch are refused; and the Rust core's fixture decodes into the CLI's own types, so the Go writer and the Rust reader cannot drift |
| next | the guest wrapper and the contract's two-height anchor check in `registerBundle`; then the live capture |

### 2026-09-27 - D3 - the pool record carries the authenticated flip count

| Item | Evidence |
|---|---|
| the gap | `SPEC.md` 5.5 item 7 says the statement binds the authenticated eligible count, and the review's A08 needs the pool record to carry it. The escrow read the journal's count only to refuse an empty bundle and then discarded it, so the market's `flipCount` came from a registration artefact the caller named |
| the record | `PoolBundle` gains `covered` (4 bytes, appended so every existing offset is unchanged, `POOL_VALUE_BYTES` 100) and `registerBundle` writes it from the journal's `covered flips` field; `poolView` publishes `coveredFlips`. The pool record round-trip test carries it |
| the reader | `tools/fod/market.go` decodes it at 96..100 and the listing's `FlipCount` now comes from the record; a registration artefact only fills it in when the record has none (`withPoolProvenance`), and the record's own number wins |
| end to end | `TestPoolSupplyFromASealedReceipt` registers the sealed receipt and reads `coveredFlips: 1` back from the contract - the receipt's own count, not one a caller supplied |
| verification | `contracts/escrow` `cargo test` 14 passed; `tools/fod` `go test ./...` pass (14.3 s); `spikes/harness` full package pass (3.5 s); `contracts/escrow/dist/fod_escrow.wasm` rebuilt |
| note | the count also unblocks the fee cap the review asks for, whichever transport the owner chooses; the fee itself still waits on that decision |

### 2026-09-27 - D4 - the answers' provenance travels with the pool

| Item | Evidence |
|---|---|
| the gap | `REVIEW.md` A09 asks the session to connect "verified purchased content and authenticated answer provenance". The content arrived last turn; the export's `answersEvidence` - what the provider says its answers rest on - was dropped by `fod session-pool`, so a participant saw the rules and never the basis |
| the pool carries it | `sessionPoolProvenance.Answers` copies the export's `answersEvidence`; the regenerated `gateway/fixtures/cli-pool.json` carries `"answers": "fixture answers file"`, and `TestSessionPoolCarriesContentAndAnswers` requires it back |
| the gateway serves it | `gateway/src/pool.ts` validates the optional `provenance.answers` and `/session/policy` returns it unchanged; the gateway test asserts the policy carries it |
| the session shows it | the policy block reads "Answers from <the provider's statement>", or "not stated" when the pool names none, labelled as the provider's own statement rather than a chain fact (`app/src/views/Session.tsx`) |
| guided path | the FAQ's session-setup answer now names the three steps - `fod export`, `fod session-pool`, and the gateway's `poolPath` - and says the session shows each flip's two sequences and where the answers come from |
| verification | tools/fod `go test ./...` pass; gateway `npm test` 20 passed and typecheck clean; `npm run verify` passes with a new check, "the policy says what the answers rest on"; `app/dist/flips-on-demand.html` rebuilt (273,509 bytes, including the FAQ sentence) |

### 2026-09-27 - D3 - two bundles, and the restart with per-buyer credits

| Item | Evidence |
|---|---|
| two bundles | `TestRefundingOneBundleLeavesAnotherUntouched` (A03's exit list): one bundle's refund moves only its own reserve and quote and counts its own strike, the other keeps its reserve, units, beacon and quote, and the books balance exactly - the two reserves plus the credit are the liquid balance, before and after the withdrawal |
| how it was checked | the quotes follow `contracts/escrow/src/pool.rs`: buy `ceil(reserve / (units - 1))`, sell `floor(reserve / (units + 1))`, so the assertions name the arithmetic rather than a magic number |

| Item | Evidence |
|---|---|
| the claim pointed at a deleted file | the Package 5 entry cited `spikes/harness/escrow_refund_test.go` and `env.Restart` for the two-buyer refund across a restart. The offer-slice removal deleted that file, and nothing used `Restart` afterwards, so the property was claimed but untested |
| restored | `TestPoolRefundsSurviveARestartAndStaySeparatePerBuyer`: one bundle, two buyers whose addresses share their first eight bytes, both orders refunded, both credits 100 each with the books at 200, then a restart reloads storage and balances, each buyer reads their own credit, withdraws it, and the contract ends at 0 with the credit total 0 |
| the bug it found | `MinimalEnv.Restart` copied storage, balances and code but not the contract's own address or the node's fee, so a test whose module lives at its own address looked for the balance at the default one and a withdrawal failed with "insufficient funds". Both are configuration a restart reloads, and the harness carries them now |
| A06's exit list | junk-first and the "competing desk" case are the same contract behaviour - the chain cannot tell a desk's payload from junk, so the last pending one wins and the buyer's confirmation decides which bytes were right (`TestD0JunkFirstDeliveryDoesNotOwnTheSlot`); the absent buyer is covered by the fulfilment test and the desk restart by this one. A false beacon now has its own test: `TestAStrangersBeaconGrantsNothingButFreshness` shows a stranger's beacon moves only the beacon block and grants no unit, credit or claim |
| verification | `spikes/harness` full package pass (3.3 s) |

### 2026-09-27 - D5/D3 - artefacts fail closed, and the desk can see its budget

| Item | Evidence |
|---|---|
| missing artefacts fail | `REVIEW.md` A10: the sealed-artefact tests returned quietly when the file was absent. The committed artefacts are `proofs/registration/dist/registration.json`, `proofs/registration-v2/dist/registration-v2.json`, `proofs/delivery/dist/delivery.json` and `release/artifacts/fod_escrow.wasm`, so both Rust hosts (v1, v2), the CLI's artefact tests and the harness tests now fail with the file named and the command that regenerates it |
| demonstrated | with `proofs/registration-v2/dist/registration-v2.json` moved aside, `TestRegistrationJournalIsFrozen` and the v2 host's sealed-artefact test both failed; after restoring it both passed again. Files produced by the CLI (prepared actions, `output/`), live indexer tests and the guarded fixture generator still skip, which is what they are for |
| the desk's budget | `registryHead` read the node's own block interval and nothing used it. `fod serve` now reads the head and interval in one place and, before a proof fallback, prints "720 blocks left, about 3h00 at this node's 15s per block"; a node that reports no interval says so instead of implying one (`TestSettlementBudgetUsesTheNodesOwnBlockInterval`) |
| who pays | `SPEC.md` 7.2 now tabulates the measured gas, signed size and fee for `buy`, `deliver`, `confirm`, `refund` and `fulfill` at the recorded mainnet rate: the fast path costs the buyer about 0.67 iDNA and the desk about 0.44, while settling without the buyer costs about 189 - which is why the fallback is a last resort and the desk has to be online near the deadline |
| verification | `tools/fod` `go test ./...` pass (14.5 s, including the new budget test and the serve suites); `spikes/harness` pass (3.4 s); `cargo test -p registration-v2-host` and `-p registration-host` verify their sealed artefacts |
| next | the same treatment for the last silent skip in `tools/fod` (`submit -broadcast` receipt tests read generated output, which is correct to skip), then D2 once the owner's economics are settled |

### 2026-09-27 - D4 - the session shows the flip and takes the choice

| Item | Evidence |
|---|---|
| the gateway reads the format | `gateway/src/flip.ts` decodes the two hex parts with the client's rules (RLP, four pictures, two orders of in-range indices, the order zero written as RLP writes it) and renders each sequence as data URLs; `gateway/test/flip.test.ts` reads the same frozen vector as the guest, so the two languages cannot drift |
| the pool carries content | `gateway/src/pool.ts` accepts an optional `content` per challenge and refuses a pool whose content is not a flip; `gateway/example-pool.json` c1 carries a real flip over four 8x8 PNGs, encoded with the client's `rlp` package |
| the participant's view | `GET /session/challenges` returns `variants`: two sequences of four pictures, in the order each shows them, and never answer material (`gateway/test/gateway.test.ts`) |
| the interface | `app/src/views/Session.tsx` renders both sequences and answers by choosing one, sending `left` or `right`; a challenge without content keeps the text field, and the Submit button is not offered twice |
| verified in a browser | `npm run verify` drives the built page against the example gateway: "the flip shows both sequences as eight loaded pictures" (eight `data:image/png` URLs with a decoded width), the choice scores correct, c2 is answered by text, the score is 2 of 2 and the provider result still verifies in the browser. Screenshot: `output/playwright/session.png` |
| verification | `gateway` `npm test` 19 passed and typecheck clean; `app` typecheck, build and verify (22 checks) pass |
| the provider's side | `fod session-pool -export-dir DIR -out FILE -session-id ID -utility-id ID -threshold N -closes-at T [-opens-at T]` turns a session-content export into the pool the gateway loads: one challenge per flip with its two parts as content and the answer digest the export evidences, refusing a flip without an answer, an edited part file (SHA-256 against the export's record), a threshold above the challenge count and a close before the open; the pool is mode 0600 |
| the pair is pinned | `gateway/fixtures/cli-pool.json` is what this command writes over `gateway/fixtures/session-export`; `TestSessionPoolMatchesTheCommittedFixture` requires the bytes back, and the gateway's own test loads that same file, so the CLI's output and the gateway's loader cannot drift. `gateway` `npm test` 20 passed |
| next | the guided sell/give journey: the same export path with a runbook a first-time provider can follow, and the app's next-step text tied to it |

### 2026-09-27 - D1 - the guest accepts only real flip content

| Item | Evidence |
|---|---|
| the format | `SOURCES.md` F-98 pins it from the client's own files: `renderer/screens/flips/utils.js:241-258` writes the public part as `RLP([image, image])` and the private part as `RLP([image, image, [order, order]])`, both as `0x` hex; `renderer/screens/validation/machine.js:1729-1765` reads them back; an order entry is an RLP integer, so the index 0 arrives as the empty string and the renderer's decoder defaults it to 0 |
| the checker | `proofs/flip-core` (`no_std`, allocation-free): hex parts, RLP lists, exactly four non-empty pictures, two orders of four entries each, every index in range, and the two orders differ - so a customer can see the pictures and has something to choose between. Eight unit tests, including the frozen vector |
| the frozen vector | `proofs/flip/vectors/flip-v1.json`, encoded with the same `rlp` package the client uses (`IdenaAI/package.json`: `"rlp": "^2.2.2"`), so the wire form is the client's rather than ours |
| the fixture blob | `proofs/registration/vectors/blob-flip-v1.json`, written by `proofs/flip-fixture` with fixed ephemeral scalars: the frozen flip sealed with the epoch keys of `blob-v1.json` under the node's own ECIES construction. Its test opens the blob, re-reads the plaintext as a flip and checks the CID; the recorded node-built `blob-v1.json`, whose plaintext is placeholder JSON, stays as the refused case |
| the session fixture | regenerated over the new CID by `tools/fod/session_bundle_regenerate_test.go` (`FOD_REGENERATE_SESSION_VECTOR=1`); a submitted flip carries its CID as the multibase text a node stores, which the record and the manifest have to share |
| the guest | `proofs/registration-v2/methods/guest/src/main.rs` `check_structure` now requires that format instead of nonempty UTF-8. `cargo run --release -- --negative` refused all five witnesses, including "placeholder content, not a flip" |
| the receipt | re-proved with the content check in place: 1,003.0 s, 7,864,320 cycles in 8 segments, image id `b4a32e7fcc45bd38…`, claim digest `c7ba51154c936d9a…`; `contracts/escrow/src/verifier.rs` `REGISTRATION_IMAGE_ID` and `tools/fod/receipt_claim.go` `registrationImageIDHex` moved with it, and `contracts/escrow/dist/fod_escrow.wasm` was rebuilt |
| verification | `proofs/flip-core` 8 passed; `proofs/flip-fixture` 1 passed; `contracts/escrow` `cargo test` 14 passed; `tools/fod` `go test ./...` pass (14.5 s); `spikes/harness` full package pass (3.5 s); `cargo test -p registration-v2-host` verifies the sealed artefact against the built guest |
| next | the same format in the session renderer (A09): show the four pictures and the two orders to a participant and take the choice |

### 2026-09-27 - D3 - the availability observation is real, dated, and repeated by the buyer

| Item | Evidence |
|---|---|
| the lie | `tools/fod/availability.go` stamped every advisory with `time.Now()`, so an advisory built from stored inventory flags claimed a check that had not happened (`REVIEW.md` A08). The fetch loops recorded no time at all |
| dated at the fetch | `BundleEntry.BlobCheckedAt` is written where the blob is fetched and hashed (`inventory.go`, `indexer_inventory.go`), and `availabilityFromInventory` carries it per flip and sets the advisory's `checkedAt` to the **oldest** check. A flip with no recorded time leaves the field empty rather than borrowing the clock (`TestAvailabilityAdvisoryNeverRestamps`) |
| off-chain label | the advisory carries a fixed `note` saying the bytes were fetched and hashed by the reporting tool, that no chain rule confirms availability, and that anyone may replace it |
| the buyer's own check | `fod prepare-buy -request ... -availability FILE` now fetches every CID the advisory names from the buyer's own node, rebuilds each CID from the bytes, and refuses to build the call unless all of them verify (`tools/fod/availability_check.go`). The refusal names the failures, and the author named by the advisory has to be the bundle's on-chain author |
| recorded | the prepared file and `fod submit`'s strict union carry the check: its own `checkedAt`, the advisory file, the author and epoch, the pool record's manifest commitment, the counts, and any failures - so what the buyer saw before signing is auditable afterwards |
| shown | the market row now shows the advisory's time ("at 2026-09-26 12:00 UTC", or "at an unknown time" when the advisory recorded none) next to the verified count (`app/src/views/Market.tsx`); `app/dist/flips-on-demand.html` was rebuilt (271,895 bytes, including the FAQ sentence) and `npm run verify` passes its 19 browser checks |
| tests | `TestRecheckAvailabilityVerdictsEveryFlip` (right bytes, wrong bytes, unserved CID, repeated CID), `TestRecheckAvailabilityRefusesAnEmptyAdvisory`, `TestPreparePoolBuyRepeatsTheAvailabilityCheck` (a served bundle prepares and records the check; an unserved CID and another author's advisory are refused), `TestAvailabilityFromInventory` and `TestAvailabilityAdvisoryNeverRestamps` for the times |
| verification | `tools/fod`: `go test -modfile=go.local.mod -count=1 ./...` pass (14.6 s) |
| runbook | the buyer's step in `release/README.md` names `-availability` and what it refuses, and the FAQ says the same in one sentence |
| still open in A08 | the contract half: buyer-funded host credits capped by the authenticated eligible flip count (`SPEC.md` 5.1, review D3). It needs the retrieval-fee decision (owner decision 3) and the covered-count field the pool record does not carry yet |

### 2026-09-27 - D1 - the escrow and the CLI take the version 2 statement

| Item | Evidence |
|---|---|
| escrow | `contracts/escrow/src/lib.rs`: the 262-byte journal's offsets, the three flag bits, and `header_state_root(height)`, which calls the runtime's own `block_header` and takes the state root out of the proposed or empty shape the node's header union carries (`SOURCES.md` F-96). A height this node does not hold, or a header that does not parse, is refused rather than assumed |
| two statements, two versions | the swap had bumped the one shared `JOURNAL_VERSION` to 2, which refused every delivery proof: `TestPoolFulfilSettlesWithTheCliPreparedDelivery` failed at `fulfill` (`RuntimeError: unreachable`). `verifier.rs` now carries `DELIVERY_JOURNAL_VERSION` (1) and `REGISTRATION_JOURNAL_VERSION` (2) separately, and the whole harness package passes |
| the receipt can register | the version 2 host carried a placeholder consensus label, so nothing it proved could pass the escrow's pinned digest. The label is the pinned node revision now, the statement was re-proved (`cargo run --release -- --prove` in `proofs/registration-v2/host`: 1,005.6 s, 7,864,320 cycles in 8 segments, image id unchanged `f8ef4e01…`, claim digest `70111077396fdb…`), and the CLI assembles it into the call the escrow accepts |
| CLI | the anchor and flag offsets (`tools/fod/prepare_registration.go`), the version 2 artefact as the sealed receipt the tests assemble, and the restored cross-language freeze: `TestRegistrationJournalIsFrozen` decodes the frozen journal at the CLI's offsets and requires the claim model to re-encode it byte for byte, and `TestRegistrationClaimBindsEveryField` requires every field to move the bytes |
| fees re-measured | `registerBundle` 195,689,565 wasm gas and 648 signed bytes, `donateUnit` 102,779 (`tools/fod/fees.go`); `sellUnit`'s signed size is 674 at the version 2 journal and its budget stays the successful-sale measurement because reserve-funded supply is disabled (`REVIEW.md` A01/A05) |
| stale consumers fixed | `tools/fod/session_bundle_test.go` pointed at the retired `spikes/session-bundle` path; the D0 regressions and the supply test pinned the version 1 offsets. They take the receipt through `spikes/harness/registration_v2_test.go` now, which binds both anchors to the headers the env's chain answers |
| papers corrected | `SPEC.md` 5.5 and `O-15` claimed the contract compares height, block hash, state root, identity root and seed. It compares the state root at each anchor height, which is what the evidence folds to; the fuller check is the `spikes/header-anchor` measurement, and the rows say so now |
| header shapes | `header_state_root` reads the state root from whichever shape the node's own header union carries (proposed or skip). The release contract does not re-check the block hash or the `FlipLotteryStarted` flag; `spikes/header-anchor` measured that it could |
| rebuilt | `contracts/escrow/dist/fod_escrow.wasm` (the harness's test-hooks build, 366,886 bytes). `release/` still describes the D0 revision and is regenerated once in D5 |
| drift outside this slice | `gofmt -l tools/fod spikes/harness` lists `answers.go`, `body.go`, `delivery_test.go`, `stateat.go`, `r0_test.go` and `r0_receipt_test.go`, none of them touched here; and the no-feature wasm build warns that `key_raw` is unused, which is the test-hook helper the release build compiles out |
| verification | `tools/fod`: `go test -modfile=go.local.mod -count=1 ./...` pass (14.7 s). `contracts/escrow`: `cargo test --locked --offline` 14 passed. `spikes/harness`: full package pass (3.6 s). `proofs/session-core`: 13 passed. `proofs/registration-v2`: the sealed v2 artefact verifies against the built guest |
| next | the live capture over a real session (`fod session-bundle` against a datadir, then the same receipt path end to end), the content check (`SPEC.md` 5.5 item 6), then D2 |

### 2026-09-26 - D0 - fail closed and capture the A01/A03/A04/A06/A07 regressions

DeepSeek implemented the first containment package. The adversarial tests were
written first and run against the pre-fix build, then against the fixed one, so
each finding has a reproduction and a regression.

| Item | Evidence |
|---|---|
| A01/A05 reproduced | `TestD0RegistrationReceiptDoesNotPayItsPresenter` failed against the baseline test-hooks build (sha256 `e129d065...`, built from `e685d96` in a scratch worktree): `sellUnit did not fail`. The real sealed registration receipt paid its presenter |
| A03 reproduced | `TestD0RefundsStayBackedWhileSupplyIsDisabled` failed the same way: the review's 26-sale sequence was reachable. `pool.rs::supply_payouts_outrun_a_pending_refund` pins the traced numbers (50,315 paid out, 9,685 left against a 10,000 refund) |
| A04 reproduced | `TestD0RejectedBuyKeepsNothing` failed: `buy accepted a value it must refuse`; `TestD0ValueOnANonPayableCallIsRefused` failed on `beacon` |
| A06 reproduced | `TestD0JunkFirstDeliveryDoesNotOwnTheSlot` failed: `the honest delivery was refused after junk: {"ok":false,"error":"order_not_funded"}` |
| A07 reproduced | the new CLI round trip failed in the scratch worktree: `read prepared action: json: unknown field "bundle"`. Repair: the strict reader now names the producer's `bundle`, `nonce` and `note`, and `TestPreparePoolBuyRoundTripsThroughSubmit` round-trips the real artefact |
| Containment | `error_reply` aborts the call; `SUPPLY_PAYOUTS_ENABLED = false` refuses `sellUnit`; every non-payable export refuses attached value; `deliver` replaces a pending candidate |
| Focused batch | `go test -modfile=go.local.mod -count=1 .` in `spikes/harness`: ok (the five D0 tests included); `go test -modfile=go.local.mod -count=1 ./...` in `tools/fod`: ok, 14.5 s; `cargo test --locked --offline` in `contracts/escrow`: 14 passed |
| Test hooks | `TestReleaseModuleExportsNoAdministrativeEntryPoint` passes against the rebuilt module; the release artefact still exports no hook |
| Interface honesty | the offline FAQ's sell entry now states that gifting is the only supply path this build exposes, so the shipped HTML does not promise the disabled payout |
| Packaging defect found and fixed | `.gitignore`'s `vault*` rule swallowed `tools/fod/vault.go` and `vault_test.go`, so a fresh clone could not build `fod` (`undefined: WriteVault` in the scratch worktree). The rule now negates those two files |
| Remaining | A01/A03/A05 need the D2 model (finite issuance, segregated refund escrow); A06's settlement guarantee and authenticated delivery capability stay with D3; A02/D1 and the rest of the A07 journey stay open |
| Release record | `release/MANIFEST.json` rebuilt at the D0 source revision `fbb24cc`, `worktreeDirty: false`; `release/artifacts/fod_escrow.wasm` 361,960 bytes, sha256 `a1a031c3...`; `app/dist/flips-on-demand.html` 271,566 bytes, sha256 `b156d672...` |
| Local CLI artifact refreshed | the D0 rebuild made the git-ignored `output/cli/prepared-deploy.json` stale, and `TestCliPreparedDeployDerivesTheNodesAddress` caught it; regenerated from the new module: code hash `0eb6ba69...`, address `dd84df91...`, raw transaction 362,119 bytes, not broadcast |

### 2026-09-25 - Package 0 - baseline

Created repository skeleton and the authoritative specification.

| Item | Evidence |
|---|---|
| Repository created empty, no prior commits | `git status` reported "No commits yet on master" with no tracked or untracked files before this package |
| Specification derived from the supplied plan | `SPEC.md`; source pinned by sha256 `1fab9cdf...` in `SOURCES.md` section 1 |
| Reference sources pinned without modification | `SOURCES.md` section 1; `idena-go` worktree dirty state recorded, not touched |
| Toolchain measured | `SOURCES.md` section 2; RISC Zero toolchain absent (`rzup`, `cargo-risczero` not found on `PATH`) |
| Epoch key derivation identified for Package 3 | `SOURCES.md` F-1, from `idena-go/core/flip/flipper.go:271-287` |
| Blob/CID binding identified | `SOURCES.md` F-2, from `idena-go/core/flip/flipper.go:57-71,118-160` |
| ECIES construction pinned to the repository-local library | `SOURCES.md` F-3, from `idena-go/core/flip/flipper.go:194-215` |
| `flip_getKeys` ruled out as historical exporter | `SOURCES.md` F-4, from `idena-go/core/ceremony/ceremony.go:1736-1763` |
| Contract runtime identified as external Rust binding | `SOURCES.md` F-5, from `idena-go/go.mod:45,273` and `vm/wasm/wasm_env.go:11` |
| Deployment-stake check scope established | `SOURCES.md` F-6, from `idena-go/blockchain/validation/validation.go:726-752` |
| Contract runtime limits and gas model measured | `SOURCES.md` F-8, from `idena-wasm@7e113895 src/runner.rs:116-123`, `src/costs.rs`, `idena-go/vm/costs/costs.go`, `idena-go/blockchain/blockchain.go:1797-1807` |
| Host-function surface inspected: no big-integer or pairing precompile | `SOURCES.md` F-9, from `idena-wasm@7e113895 src/imports.rs` |
| Bulk-memory operators rejected by the gatekeeper | `SOURCES.md` F-10, from `idena-wasm@7e113895 src/gatekeeper.rs:20,62` |
| Contract runtime source pin identified and located locally | `idena-wasm-binding/lib/ARTIFACTS_SOURCE`; revision `7e113895...` read via `git show` from the upstream checkout, which was left untouched |

Not done in Package 0, and deliberately out of scope:

- No build, compilation, or test of any component.
- No network access to Idena mainnet or testnet.
- No modification of the reference checkouts.
- No wallet, key, or credential handling.

### 2026-09-25 - Package 0 - project framing and publication

| Item | Evidence |
|---|---|
| Repository published as private, owned by `ubiubi18` | `gh repo view` returned `"visibility":"PRIVATE"`, `"name":"flips-on-demand"`, default branch `main` |
| License recognized as MIT by the host | `gh repo view` returned `"licenseInfo":{"key":"mit","name":"MIT License"}` |
| Framing recorded in the repository itself | `README.md` carries the experimental-MVP framing, the use-at-your-own-risk disclaimer, and the contributing section; `LICENSE` is MIT; `THIRD_PARTY.md` records reuse notices |
| Topics applied | `experimental-mvp`, `idena`, `learning-project`, `marketplace` |
| Reference source licenses checked | `SOURCES.md` F-11: IdenaAI MIT (c) 2020 Idena; both vendored `idena.social` repositories MIT (c) 2025 N3CR0M4NC3R; vendored contract `package.json` says ISC, so per-file checks are required at reuse time |

Publishing the repository does not verify any technical claim in `SPEC.md`. The
disclaimer and framing text reduce the chance of someone misusing an unverified
experiment; they do not make it safe.

## 4. Gates and blockers

| ID | Gate | Consequence if it fails |
|---|---|---|
| G-1 | Groth16 verification must fit the contract runtime limits measured in Package 1 | Cleared: a real RISC Zero Groth16 receipt verifies in the runtime at 195,325,117 wasm gas (38.1% of the 512,000,000 ceiling) and a tampered claim digest is rejected (F-17 to F-21). Residual items (claim binding, dev-mode rejection, block-production wall-clock) are assigned to later packages |
| G-2 | Historical reconstruction must reproduce eligibility from authenticated inputs only | Open: A02. Capture is only an input; complete records, transaction inclusion and their binding to the qualification witness are missing. The anchor half is measured: the contract reads the executing node's own header for the height and compares block hash, roots and seed (`SOURCES.md` F-96). Follow D1; no arbitrary root or explorer assertion can authorize supply |
| G-3 | Delivery proof must bind buyer, order, nonce, ciphertext, and contract context | Cleared against the pinned runtime: the contract recomputes the claim digest from the journal and refuses a delivery whose journal names another contract, order, order nonce, seller, buyer, delivery key, or commitment, or whose ciphertext does not hash to the journal value (F-45). Parity with the runtime mainnet executes is measured rather than assumed: the same suite passes under the released binding (`O-10` closed, F-90) |
| G-4 | Pending refunds, available reserves and withdrawal credits must remain fully backed under every transition | Reopened: A01/A03/A04/A05. Individual withdrawal rollback passes, but that does not establish pool solvency or supply authority. D0 (2026-09-26) makes every refusal a failed call and disables reserve-funded supply, which closes A04 and contains A01/A03/A05; the accounting model itself stays open until D2 |

G-3 remains evidence for the recorded proof-based delivery statement only. The
new fast path, its availability and fallback guarantees remain open under A06.
All older entries below are historical observations, not overrides of this
review or evidence that a finding is repaired.

### Package 1 pre-check result (read-only, 2026-09-25)

The feasibility gate is now quantified instead of unknown:

- Measured: flat `1` wasm gas per executed operator, `wasmGas = userGas * 100`,
  base deploy cost `3,000,000` wasm gas, base call cost `100,000` wasm gas,
  6.25 MiB guest memory cap, no big-integer or pairing precompile, bulk-memory
  operators rejected.
- Not yet measured: operators executed by a Groth16 verification, module size,
  wall-clock per verification, and the resulting fee for a real order.
- Blocking decision: if a single verification cannot fit a call within block
  production limits at an acceptable fee, Package 1 fails and the proof-system
  choice in `SPEC.md` section 5.4 must be revisited before any contract work.
- Provenance limit: the measured numbers describe the runtime revision pinned by
  the local fork checkout. Mainnet's executed runtime revision is unverified
  (open item O-10), so these numbers are not yet mainnet evidence.

No component was built, compiled, or executed in this pre-check.

### Package 1 spike progress (2026-09-25)

The contract ABI is no longer an unknown. A `no_std` Rust module that implements
it was built, lowered, and executed by the real runtime.

Artifacts:

| Artifact | Purpose |
|---|---|
| `spikes/contract-abi/` | Rust module implementing the ABI: `memory`, `allocate`, `deploy`, `ping`, `compute`; imports `env.set_storage` |
| `spikes/contract-abi/dist/fod_abi_spike.mvp.wasm` | Build output actually tested: 2,969 bytes, sha256 `57bf6d90807c879088e092acdbc86a7911449180ff48ce6422be709d46a56f8e` |
| `spikes/harness/` | Go module that runs the artifact in the real Rust-binding runtime with a stubbed host env |

Measured by `GOTOOLCHAIN=go1.26.5 go test -v ./...` in `spikes/harness`:

| Call | Wasm gas | Marginal |
|---|---|---|
| `deploy` | 3,000,034 | base 3,000,000 plus 34 |
| `ping` | 100,034 | base 100,000 plus 34 |
| `compute(0)` | 103,218 | includes one storage write and JSON build |
| `compute(1,000)` | 115,098 | 11,880 |
| `compute(10,000)` | 216,541 | 101,443 |
| `compute(100,000)` | 1,229,234 | 1,012,693 |

Result values were read back from contract storage, so the arithmetic really ran
and produced the expected checksum chain.

Two concrete blockers were found and solved, both recorded in `SOURCES.md`:

1. A plain Rust `wasm32-unknown-unknown` module is rejected by the runtime
   gatekeeper for bulk-memory operators; lowering with
   `wasm-opt --llvm-memory-copy-fill-lowering` makes it acceptable (F-14).
2. Method arguments arrive as Region pointers, not raw byte pointers, and
   `allocate` must return a Region the host can write into (F-12).

What this does **not** show: any Groth16 verification. No proof system is
integrated, the RISC Zero toolchain is still absent, the host environment is
stubbed, and the 512,000,000 wasm gas ceiling has not been tested against real
verification work. Package 1 remains open.

### Package 1 - BN254 cost measurement (2026-09-25)

The central unknown of this package was whether pairing-based verification can
fit the runtime's gas ceiling. That is now measured, not estimated.

Artifacts:

| Artifact | Purpose |
|---|---|
| `spikes/groth16-verify/` | `no_std` arkworks 0.5 BN254 module with a bump allocator, exposing `field`, `mul`, and `pairing` benches through the contract ABI |
| `spikes/groth16-verify/dist/fod_groth16_spike.mvp.wasm` | Build output tested: 303,769 bytes, 49 declared memory pages, sha256 `2704c0fe3b73d94b1b0071a66a0e8dfbf6dcb54956c517cf8b5deb513965eb13` |
| `spikes/harness/groth16_test.go` | Runs those benches in the real runtime at the full per-call gas ceiling |

Measured (`GOTOOLCHAIN=go1.26.5 go test -v -run TestGroth16SpikeInRuntime`):

| Call | Wasm gas | User gas |
|---|---|---|
| `deploy` | 3,000,046 | 30,000 |
| `field(1,000)` | 1,411,803 | 14,118 |
| `field(10,000)` | 13,156,690 | 131,567 |
| `field(100,000)` | 130,592,703 | 1,305,927 |
| `mul(1)` G1 scalar mul | 365,508 | 3,655 |
| `mul(100)` | 17,445,310 | 174,453 |
| `pairing(1)` | 40,299,766 | 402,998 |
| `pairing(2)` | 80,610,638 | 806,106 |

Unit costs: about 1,306 wasm gas per `Fr` multiplication, 174,000 per G1 scalar
multiplication, and 40,300,000 per full BN254 pairing.

Why this matters for gate G-1: a Groth16 check is dominated by two to three
pairings, so roughly 81,000,000 to 121,000,000 wasm gas before public-input and
deserialization work, against a 512,000,000 wasm gas ceiling. That is a real
margin, and the module also instantiated with 3.1 MiB of its 6.25 MiB memory
budget declared.

What is still missing before Package 1 can be called done:

1. A real receipt: verification key, seal encoding, and the digest binding that
   RISC Zero receipts carry. Pairing cost is the dominant term but not the whole
   verification.
2. The RISC Zero toolchain pin (`rzup`/`cargo-risczero` are still absent).
3. Wall-clock behavior under real block production, and the money cost of a
   ~800,000-user-gas call at mainnet `FeePerGas`.

Gate status update: G-1 moved from "unknown" to "partially cleared - gas and
memory headroom measured; real receipt verification outstanding".

### Package 1 - genuine Groth16 verification in the runtime (2026-09-25)

The gate question "can a real Groth16 proof be verified inside this runtime,
within its limits" is now answered, with a passing and a failing case.

Artifacts:

| Artifact | Purpose |
|---|---|
| `spikes/proofgen/` | Native generator: real BN254 Groth16 setup and proving run for the circuit `c = a * b`, with serialization round-trip checks, writing the verification material as constants |
| `spikes/groth16-verify/src/vector.rs` | Generated material: vk 296 bytes, proof 128 bytes, public input 32 bytes, sha256 `6aa9a68c1e860b0430ef44afa0d011278909bedabef724fa5732702592c2c168` |
| `spikes/groth16-verify/dist/fod_groth16_spike.mvp.wasm` | Built module, 365,172 bytes, 49 declared memory pages, sha256 `96dc864d9da901744c8053a3603a69c49925fa3ab3133ea2588b8d08c28b45f4` |

Measured in the real runtime:

| Call | Wasm gas | Share of the 512,000,000 ceiling | Stored result |
|---|---|---|---|
| `verify(0)` - untampered proof and input | 152,194,393 | 29.7% | `{"mode":0,"verified":true}` |
| `verify(1)` - public input incremented by one | 152,214,933 | 29.7% | `{"mode":1,"verified":false}` |

The verification path deserializes the verifying key, proof, and public input at
runtime, so deserialization cost is included. The passing case shows the
arithmetic path works in this runtime; the failing case shows the check is a real
verification rather than a constant.

Honest scope: the proof is genuine, but it is not a RISC Zero receipt - no
ImageID binding, no seal decoding, and no dev-mode rejection case. The
verification material is embedded rather than passed as call arguments. Package
1 therefore stays open on the RISC Zero-format items, which need the prover
toolchain that is still absent from this machine.

Gate status update: G-1 "partial - a genuine proof verifies at 30% of the wasm
gas ceiling, with the rejection path demonstrated".

### Package 1 - RISC Zero verifier crate probe (2026-09-25)

Question answered: does the actual RISC Zero Groth16 verifier (not just
arkworks) build for the contract target and execute in the runtime?

| Artifact | Purpose |
|---|---|
| `spikes/r0-verifier-probe/` | no_std module using `risc0-groth16 5.0.0-rc.1` with `default-features = false`; exposes the contract ABI and a `probe` method |
| `spikes/r0-verifier-probe/dist/fod_r0_verifier_probe.mvp.wasm` | Built module: 70,765 bytes, 33 declared memory pages, sha256 `6c5e910508433bc1fb6a6b9f6988c2b6bf0a744eb79ad093a75f5fd94b8660ce` |
| `spikes/harness/r0_test.go` | Runs the probe in the real runtime |

Measured: deploy accepted, `probe` executed and stored
`{"seal_rejected":true,"code":3}` at 25,121,494 wasm gas. That number covers
building the verifying key from the embedded ceremony constants and rejecting an
empty seal; it is an upper bound for verifier setup, not the verification cost
itself.

Design note recorded for Package 5: the verifying key should be constructed once
or embedded in prepared form, since rebuilding it per call costs about 25M wasm
gas on its own.

### Package 1 - real RISC Zero receipt verified in the runtime (2026-09-25)

Package 1 exit condition met. The RISC Zero prover toolchain was installed
(pins in `SOURCES.md` F-20), a genuine Groth16 receipt was produced with dev mode
off, and that receipt verifies inside the contract runtime.

Artifacts:

| Artifact | Purpose |
|---|---|
| `spikes/r0-receipt/` | RISC Zero workspace: trivial guest plus a host that proves with `ProverOpts::groth16()` and exports the material to `dist/receipt.json` |
| `spikes/r0-receipt/dist/receipt.json` | The receipt: 256-byte seal, claim digest, control root, BN254 control id, image id, journal (sha256 `7ee360c76f0e29807117a3435a556c66c704017332378dc12470b5358e4eab49`) |
| `spikes/r0-receipt-verify/` | Contract-side module: `risc0-groth16 3.0.5` in no_std with the contract ABI, embedding the receipt material via `generate_vector.py` |
| `spikes/r0-receipt-verify/dist/fod_r0_receipt_verify.mvp.wasm` | Built module: 316,756 bytes, 33 memory pages, sha256 `83ccc623555bb2b17828a842d2c3ceccefe5f477e42409ef8d5d1767293d81b1` |
| `spikes/harness/r0_receipt_test.go` | Runs both cases in the real runtime |

Measured:

| Call | Wasm gas | Share of ceiling | Stored result |
|---|---|---|---|
| `verify(0)` real receipt | 195,325,117 | 38.1% | `{"mode":0,"verified":true,"detail":"ok"}` |
| `verify(1)` claim digest tampered | 195,352,445 | 38.2% | `{"mode":1,"verified":false,"detail":"invalid_proof"}` |

Proving cost for the trivial guest: execution 4.5 ms, Groth16 compression through
the Docker prover taking tens of seconds, and more memory than the 8.3 GB Docker
default (it succeeded at 16.7 GB). The 60-minute registration target is not
threatened by this; the memory target needs a real measurement for real
workloads.

Carried forward, not quietly dropped:

1. Claim binding: the contract must recompute the claim digest from the expected
   ImageID and journal digest (SHA-256 over the RISC Zero tagged struct) instead
   of trusting a supplied digest. Belongs to Package 4.
2. Dev-mode rejection: dev receipts carry no Groth16 seal, so they cannot reach
   this path, but end-to-end rejection belongs to the CLI that consumes receipts.
3. Wall-clock under real block production, and the mainnet fee cost of a
   ~1.95M user gas call.

Gate status update: G-1 cleared for the Package 1 question - real proof, measured
limits, and both the valid and the invalid case demonstrated in the runtime.

### Package 2 - historical evidence baseline (2026-09-25)

A frozen local node was identified and used as the authoritative historical
source, and the reconstruction path was verified end to end against real chain
data. Details and file references are in `SOURCES.md` F-22 and F-23.

What was established:

1. The evidence paths are on-chain: `submitFlip` carries `{Cid, Pair}`,
   `deleteFlip` marks removals, long-session answers are submitted on-chain with
   `{Answers, Proof, Key, Salt}`, and short answers provide the commitment.
2. The qualification rules to reproduce live in
   `core/ceremony/qualification.go` and `reporters.go`, with flip status values
   `NotQualified`/`Qualified`/`WeaklyQualified`/`QualifiedByNone`.
3. A real observation: block 10,847,251 in epoch 205 contains a `submitFlip` by
   `0xafa56a9f...` with `pair = 6` and CID
   `bafkreihmf6xrvfh26yymneankpe6ffuyqj7nvakxnlc6vguodzoqe2jiai`, and the node
   served that CID's blob from IPFS at 35,878 bytes.

Method note for anyone repeating this: the node's datadir was never written to.
Only `idenachain.db` and `ipfs` were copied (copy-on-write) into a scratch
datadir, the node was started against that clone with `--nodiscovery`, and
`keystore`/`api.key` were excluded from the copy.

Next steps in this package, in order:

1. Sweep one complete epoch: walk the block range, collect transaction hashes,
   fetch bodies, and classify by type. This produces the raw material for any
   author's inventory.
2. Reconstruct one author's inventory for that epoch, including deletions, and
   check that a deliberately omitted submission is detectable.
3. Reproduce qualification. This is the open risk: mapping answers back to flips
   needs the epoch's lottery assignment (candidate identities plus the lottery
   seed). Whether that is fully recoverable from chain data and the node's state
   needs to be established before the eligible/ineligible cases can be produced.

No eligibility claim is recorded yet: the positive and negative cases do not
exist until step 3 succeeds.

### Package 2 - epoch sweep and tx-body availability (2026-09-25)

The first reconstruction tool exists and has been run against real chain data.

| Artifact | Purpose |
|---|---|
| `tools/fod-evidence/` | Go tool: sweeps a block range over the node RPC, decodes `submitFlip`/`deleteFlip` payloads with the pinned idena-go attachment types, and writes a JSON sweep |

Sweep of epoch 205 (blocks 10,847,140-10,860,138, 12,999 blocks, 349 referenced
transactions) against the frozen IdenaAI datadir, in 2.3 seconds:

- 24 `submitFlip` submissions from 10 distinct authors, five of which submitted 3
  flips each; 0 `deleteFlip` in this window.
- Both a hand decode and the tool agree on the known example: author
  `0xafa56a9f...`, `pair = 6`, CID
  `bafkreihmf6xrvfh26yymneankpe6ffuyqj7nvakxnlc6vguodzoqe2jiai`.

The blocking finding is data availability, measured rather than assumed
(`SOURCES.md` F-24):

| Source | Bodies available in the range |
|---|---|
| IdenaAI datadir (3.1 GB) | 177 of 349; 172 missing |
| `idena-local-node` datadir (8.8 GB) | 0 - returns null for old blocks |

For a missing hash the node returns a null result with no error while the block
still lists the hash, so an inventory built naively would silently drop
submissions. The tool now records that state explicitly instead.

Why this matters: an offer must not claim completeness it cannot prove. Until
full body availability is demonstrated, the eligible/ineligible cases cannot be
produced honestly, and gate G-2 stays open.

Next actions, in order:

1. Test peer-based body retrieval: run the clone with the original app's peer
   hints (or add peers) and re-request a known-missing hash. If missing bodies
   are fetched on demand, the constraint is "use a synced node with peers".
2. If they are not retrievable, define the evidence format so the seller
   publishes the raw bodies it can obtain, and specify exactly what a buyer can
   verify against on-chain commitments.
3. Only then continue with inventory completeness and the two cases.

### Package 2 - completeness is verifiable by design (2026-09-25)

The tx-body gap from the previous step is no longer the blocker. Block headers
commit to both the transaction set (`TxHash`) and the block body on IPFS
(`IpfsHash`), and the body is exactly `Body.ToBytes()` as published by the node
(`blockchain_test.go:2494-2495`).

Measured:

| Check | Result |
|---|---|
| Non-empty blocks carry an `ipfsCid` | yes |
| Body fetched from IPFS by that CID | 78 bytes for block 10,848,563 |
| CID recomputed over the fetched bytes matches | yes |
| Peer-based retrieval of missing tx bodies | not testable here: the app's June peer hints are all TCP-unreachable, `net_addPeer` accepted them, and the node stayed at 0 peers |

So the reconstruction becomes: walk headers, fetch each body by its committed
CID, verify the bytes against the CID, decode with the reference codec, and check
the set against `TxHash`. A node that cannot serve a transaction body costs
availability, not correctness - which is what gate G-2 needs.

Concrete next step: implement that body-based sweep in `tools/fod-evidence`
(`-body-source ipfs`), re-run epoch 205, and confirm the reconstructed
transaction set matches the header `TxHash` for every non-empty block. Then, and
only then, resume inventory completeness and the eligible/ineligible cases.

### Package 2 - committed-body sweep implemented and run (2026-09-25)

`tools/fod-evidence -body-source ipfs` now reconstructs transactions from the
block bodies the headers commit to, verifies each body against its CID, and
cross-checks the decoded transaction hashes against the node's own listing.

Epoch 205 result (12,999 blocks, 2 seconds):

| Metric | Node tx store | Committed bodies |
|---|---|---|
| Transactions enumerated | 177 of 349 referenced | 349 |
| Flip submissions | 24 | **75** |
| Distinct authors | 10 | 24 |

Verification: 306 non-empty blocks, 306 bodies fetched by their committed CID,
306 bodies whose bytes hash to that CID, and 306 blocks whose decoded
transaction hashes match the node's listing exactly (zero mismatches).

This is the omission case the plan cares about, measured rather than argued: the
node's transaction store silently under-reports flip submissions by more than 3x
on this datadir, while the committed-body path does not, and every body it
returns is bound to its CID by content hashing.

Scope limit recorded with it: the CIDs come from the node's header view, and the
RPC does not expose the header's `TxHash` or signature, so the header itself is
not yet authenticated. Two independent local paths agree on every block; making
the argument independent of the node is still open (`SOURCES.md` F-26).

Next: build one author's inventory from the committed-body sweep (dedup by CID,
carry the `Pair` index, include deletions - epoch 205 has none), then attack the
lottery-assignment question that gates qualification reproduction.

### Package 2 - author inventory built, closed-epoch data missing (2026-09-25)

`tools/fod-evidence -mode inventory` exists and was run against the epoch sweep.

Result for author `<author-address>`, epoch 205:
5 submissions, 5 unique CIDs, no duplicates, no deletions, all 5 blobs fetched
and each one hashing back to its CID (34-36 KB each), `complete=true`.

Recorded with it: `Pair` values are 0, 1, 3, 4, 6 - not contiguous. Because the
snapshot ends mid-epoch, a deletion could still happen after it, so a bundle is
only complete relative to a closed epoch. That is now a hard requirement of the
format rather than a footnote.

Qualification progress: flip assignment is deterministic over public inputs
(`core/ceremony/lottery.go`: `GetAuthorsDistribution`, `GetFlipsDistribution`
take candidates, the epoch's flips, and a seed), so it is not gated on private
keys. What it needs is a closed epoch's candidate set, seed, and answers.

Blocker, measured (`SOURCES.md` F-27): no locally available source covers a
closed epoch.

| Source | Head | Bodies | Closed epoch |
|---|---|---|---|
| IdenaAI datadir | 10,860,138, 2026-06-04 14:46 UTC | yes (via committed CID) | no - epoch 205 validated 2026-06-06 15:00 UTC |
| `idena-local-node` datadir | 10,998,672 | headers only, no transactions, no CIDs | no |

So the snapshot stops about two days before the validation that would close
epoch 205. Submissions can be reconstructed and verified; qualification,
eligibility, and the eligible/ineligible cases cannot be produced from this
machine's current data.

Unblocking options, in order of preference:

1. Forward-sync a node past an epoch boundary so a closed epoch exists locally.
   One default bootstrap address (`135.181.40.10:40405`) is reachable; the app's
   June peer hints are not. Whether blockchain peers can be established from this
   environment is untested.
2. Import an archive covering a closed epoch.

No eligibility claim is recorded. Gate G-2 stays open.

### Package 2 - closed-epoch data found (2026-09-25)

The previous entry's blocker was wrong, and the correction matters: the sweep had
only ever looked at epoch 205, which the snapshot truncates. Widening the search
showed that epoch 204 is complete, including its whole validation ceremony.

Epoch 204 over blocks 10,820,000-10,847,139 (`-body-source ipfs`):

| Metric | Value |
|---|---|
| Non-empty blocks | 1,285, all bodies CID-verified, all transaction sets matching the node's listing |
| Transactions | 2,395 |
| Flip submissions | 437 from 126 authors |
| Flip deletions | 1 (height 10,846,707) |
| `submitAnswersHash` | 147 transactions |
| `submitShortAnswers` | 147 transactions |
| `submitLongAnswers` | 145 transactions, heights 10,847,046 - 10,847,127 |

So the answers, the answer hash, and the submission window for a closed epoch are
all present and CID-verified. The eligibility half of Package 2 is no longer
blocked on data availability.

Also re-tested and ruled out: the `idena-local-node` datadir remains unusable for
evidence even with a build of the pinned checkout, so this is not a binary
version artefact. Its block store genuinely lacks bodies.

Next: reconstruct one author's epoch-204 inventory including the deleted flip,
then reproduce the qualification rules for that epoch and produce one eligible
and one ineligible case.

### Package 2 - deleted-flip inventory and answer decoding (2026-09-25)

Two more pieces of the reconstruction are working against real epoch-204 data.

Inventory for author `0xA40B9099...`, epoch 204: 3 submissions, 3 unique CIDs,
1 deleted, all 3 blobs fetched and CID-verified, `complete=true`. Details in
`SOURCES.md` F-28, including the case that matters for the bundle format:

| Pair | Block | Deleted |
|---|---|---|
| 0 | 10,846,619 | no |
| 2 | 10,846,667 | yes |
| 2 | 10,846,739 | no |

So the pair index is reused after a deletion, and the deleted flip's ciphertext
remains retrievable. A bundle must key on the CID, keep the deleted entry, and
treat the pair index as metadata.

Long answers decode from chain data with the reference decoder (`F-29`): one real
payload from epoch 204 decodes to 12 left / 7 right / 18 D / 1 A over an inferred
19 flips. The flip count itself is not in the payload - it comes from the
lottery assignment - so the inferred count is provisional until that step is
implemented.

Remaining for Package 2:

1. Reconstruct the lottery assignment (candidates, the epoch's flips, the seed)
   so the flip count per candidate is exact and votes can be attributed to
   specific flips.
2. Reproduce the qualification rules over those attributed votes.
3. Produce one eligible and one ineligible author-epoch case.

### Package 2 - lottery inputs traced; one lives in state (2026-09-25)

`fod-evidence -mode epoch-db` reads a node's epoch store read-only. Measured on
the frozen clone:

| Observation | Value |
|---|---|
| Chain database keys | 12,602,776 |
| Keys under any `epoch` prefix | 18 |
| Epochs with keys | 205 only (`epoch 205` + `cid` + CID bytes) |
| Lottery identities / seed for epoch 204 or 205 | none |

Tracing the source settles what the qualification step needs
(`SOURCES.md` F-30):

1. The lottery seed is the block seed at `flipLotteryStartHeight -
   LotterySeedLag`: public, from a block header.
2. The epoch's flips come from `submitFlip` transactions: public, already
   reconstructed and CID-verified.
3. The candidate list - identity, shard, state, and that identity's flips - comes
   from the chain state (`State.IterateIdentities`), not from transactions. The
   epoch store only holds it so a node can restore its own ceremony, which is why
   this datadir has none: its identity did not take part.

So the remaining work is obtaining state data at an epoch's lottery height and
authenticating it against the block header's state root - the "state snapshots
and proofs" the spec already lists as required input. That is the next
implementation piece, and it is the last thing standing between Package 2 and
its exit condition.

Gate G-2 status: still open. Submissions, blobs, deletions, and answer decoding
are verified; the candidate/assignment half needs state proofs.

### Package 2 - historical state ruled out; assignment must be chain-derived (2026-09-25)

`fod-evidence -mode state-at` reads the identity state at a past height. Measured
against the frozen clone at epoch 204's lottery height (10,847,021): the node
refuses with `version does not exist`.

That is structural, not a bug in the reader: `core/state/statedb.go:39` sets
`MaxSavedStatesCount = 100` and `CommitTree` deletes older versions. It is a
compile-time constant, so no node retains the identity state at a past epoch's
lottery height. Combined with the epoch store only recording lottery identities
when a node processed that block live, both "read it from a node" routes are
closed (`SOURCES.md` F-31).

Reference points captured while tracing:

| Item | Value |
|---|---|
| Epoch 204 `FlipLotteryStarted` block | 10,847,021 |
| Seed block (lottery start - 100) | 10,846,921 |

Therefore the candidate list and author ordering have to be rebuilt from
chain-derived data: the identities that submitted answer transactions, their
shard ids, their submitted flips, and the public seed. That reconstruction has a
built-in self-check worth exploiting next: an answer payload encodes `5n` bits
for `n` assigned flips, so a correct assignment must make each participant's
payload length match their assigned flip count. If that holds for all 147
participants of epoch 204, the reconstruction is corroborated; if it fails, the
ordering rule is wrong and the package has a precise error to report.

Gate G-2 status: open, with the remaining work narrowed to one computation and a
defined acceptance check.

### Package 2 - loop closed on the seed; candidate-list question settled (2026-09-25)

The lottery seed is now recovered offline from the frozen database
(`-mode seed-at`), for epoch 204: seed height 10,846,921, block
`0x83b9bbbc...`, seed
`81e5dfd53f82eee1933357eda9d3e2d85ce46a042110ea5bb158ba663ddb93c0`. That closes
the public half of the lottery inputs: seed from the header, flips from
committed bodies (`SOURCES.md` F-32).

The candidate half is settled as a design question rather than an open puzzle:

- Retroactive reconstruction from a fast-synced datadir is impossible; state
  versions are pruned to 100 and the epoch store only records lottery identities
  for a node that processed the block live (F-30, F-31).
- For the product this is fine. A seller offering an author-epoch bundle had to
  be online during that epoch, so their own node holds the lottery identities.
  The data is available to the party with standing to make the offer, which is
  what the plan's local-CLI model assumes.
- For the spike, an inferred candidate set is worth one attempt because it is
  falsifiable: the answer payload length is `5n` bits for `n` assigned flips, so
  a wrong candidate set shows up as mismatched lengths.

Two ways forward for the spike's eligibility cases, both honest:

1. Port the lottery algorithm (`core/ceremony/lottery.go`) and run it against an
   inferred candidate set, accepting the result only if every participant's
   payload length matches their assigned flip count.
2. Or obtain a datadir from a node that was live during a closed epoch, which
   would make the candidate list directly available without inference.

Gate G-2 status: open. Everything except the candidate list and the qualification
arithmetic is reconstructed and verified from chain-committed data.

### Package 2 - eligibility half blocked on external data (2026-09-25)

`fod-evidence -mode epoch-keys` enumerates the `epoch`-prefixed keys of a chain
database. Run over all seven Idena datadirs on this machine: every one that has
epoch keys has only `cid` entries (the node's own flips); none has the lottery
records `li`/`ls`. Details in `SOURCES.md` F-34.

That closes the last local avenue. The candidate list is unreachable here by any
of the three routes tried:

1. Epoch store: absent - the record is written only when a node processes a
   `FlipLotteryStarted` block live.
2. State tree: pruned to 100 versions (`MaxSavedStatesCount`).
3. Inference from chain data: shard membership is state, and shard relocation is
   a randomized process over prior state, not a function of an address.

Package 2 split verdict:

- Reconstructed and verified from chain-committed data: submissions (437 in
  epoch 204, 75 in 205), blob/CID integrity, deletions with pair reuse, answer
  decoding, and the lottery seed.
- Blocked: the candidate list and therefore qualification, which need either a
  datadir from a node that was live during a closed epoch, or a chain replay to
  rebuild historical state. Neither is an implementation gap in this repository.

Re-tried 2026-09-25 against the test host node meant for the live exercise: it is
not reachable from this machine. The host answers on SSH but presents a
different host key than the one recorded here, and both of the keys available on
this machine are refused; the login this Mac's SSH configuration names points at
a Windows key path, so the working key lives on another machine. The node's RPC
port is closed from outside, and the tailnet this machine belongs to does not
contain the host. The blocker therefore stands unchanged: the candidate list
comes from the machine holding a key to a node that was live during a closed
epoch. `SPEC.md` 5.3 was re-read while checking this: eligibility must reproduce
candidate assignment, so no amount of on-chain data substitutes for it.

One route stays open, and it does not need an old datadir. The lottery records
are written by a node that is live when a `FlipLotteryStarted` block is
produced, so a node kept synced from now on captures the candidate list of the
*next* epoch, and eligibility for that epoch becomes reconstructible once its
validation completes. The timing is measured from the pinned data (`SOURCES.md`
F-32): epoch 205 spans 12,998 blocks, and the two recorded datadir heads give
about 20 seconds per block, so an epoch is about three days. A machine that
stays online for a week therefore produces the missing list, which is what that
host was kept for.

Decision: stop pressing on this sub-item and move to Package 3 (local key tool),
which does not depend on historical state: epoch key derivation, vault handling,
and blob verification can all be built and tested against synthetic vectors
without network access or funds. Gate G-2 stays open and unclaimed.

### Package 3 - epoch key derivation and flip decryption (2026-09-25)

The local key tool now derives an author's per-epoch keys from a nodekey file and
decrypts flip blobs, using the pinned reference primitives rather than a
reimplementation (`SOURCES.md` F-35).

Delivered:

| Piece | State |
|---|---|
| `-mode derive-keys -nodekey PATH -inventory-epoch N` | works; prints only public keys, fingerprints, and seed texts |
| Epoch key derivation | `secstore.Sign` + `crypto.GenerateKeyFromSeed` + `ecies.ImportECDSA`, exactly as `generateFlipEncryptionKey` |
| Flip blob parse/build (`ProtoIpfsFlip`) | implemented |
| Blob decryption with derived keys | public part via the public-part pair, private part via the private-part pair |

Tests (`go test ./...` in `tools/fod-evidence`): derivation is deterministic and
epoch-specific, both ECIES round trips hold, the two pairs are not
interchangeable, and a blob in the node's wire format parses back and decrypts.

A mistake worth recording: the first version printed the derived private key to
stdout, which violates the spec's rule against logging secrets. That is fixed -
secrets are now reachable only through an explicit `SecretBytes()` method aimed
at the vault path - and the test suite plus a field check on the output file
confirm the summary carries no private material.

Still open in this package:

1. The age-encrypted vault for private keys and recovery archives, with
   interactive passphrase handling and restrictive file permissions. Until that
   exists, secrets are not persisted at all rather than persisted in plaintext.
2. Independent verification against a real flip blob, which requires a seller's
   own nodekey. The tests here are self-consistency against reference
   primitives, not network vectors, and are recorded as such.

### Package 3 - encrypted vault and CLI key commands (2026-09-25)

The vault is in, so secrets now have an encrypted home instead of not being
persisted at all.

Delivered:

| Piece | State |
|---|---|
| `vault.go` | `age` (scrypt recipient) encrypt/decrypt, `0600` enforced, empty passphrase refused |
| `passphrase.go` | `FOD_PASSPHRASE` when set, otherwise an echo-disabled prompt; never a CLI flag |
| `-mode export-keys -nodekey PATH -inventory-epoch N -vault FILE` | derives keys and writes them to the vault |
| `-mode import-keys -vault FILE` | decrypts, validates the payload shape, prints only public material |

Verified end to end with a synthetic nodekey: a 763-byte age file at mode
`-rw-------`, with a valid age header, containing neither the JSON field names
nor any derived key hex; import round-trips; a wrong passphrase fails with
`identity did not match any of the recipients`. Test coverage in
`vault_test.go` and `keys_test.go` (`SOURCES.md` F-36).

Package 3 status: the mechanism is complete and tested. What remains before the
package can be called done is not mechanism but integration:

1. Wire these into the `fod` CLI surface named in the spec (`prepare-offer`,
   `export-keys`, `receive`) rather than the spike tool's `-mode` flags.
2. Independent verification against a real flip blob, which needs a seller's own
   nodekey. Until that happens the derivation is verified against reference
   primitives only, and the record says so.

### Package 3 - CLI surface wired (2026-09-25)

The tool is now `tools/fod`, invoked as `fod <command> [flags]`, with the spec's
command names (`sync`, `inspect`, `receive`) alongside the technical ones. Flags
read naturally (`-epoch`, `-height`) and map onto the internal names
(`SOURCES.md` F-37).

New in this step:

| Piece | State |
|---|---|
| `commands.go` | subcommand dispatch, `fod help`, unknown-command handling with exit 2 |
| `receive.go` | buyer path: vault -> epoch keys -> decrypt both blob parts -> write outputs at 0600 |
| `receive_test.go` | end-to-end derive/encrypt/vault/decrypt, plus a wrong-vault negative case |

Verified by building and running `fod derive-keys`, `fod export-keys`,
`fod import-keys`, and `fod epoch-keys` against a synthetic nodekey and the
frozen datadir, and by the full test suite (`go test .` in `tools/fod`).

Package 3 is complete as far as mechanism and integration go. The one thing it
cannot yet claim is independent verification against a real flip blob, which
needs a seller's own nodekey. That is a data dependency, not missing code, and it
stays recorded rather than implied.

### Package 5 - escrow first slice (2026-09-25)

Package 2's eligibility half is blocked on data this machine does not hold
(F-34), but the escrow does not depend on it, so work moved to Package 5 rather
than idling on the blocker.

`contracts/escrow` implements the state machine over the ABI Package 1 validated:
registry records in storage, funding with exact-amount checks, owner-only
cancellation, deadline-gated refunds, and an accounting view. The module lowers
to 9,803 bytes with zero bulk-memory or float operators and deploys in the real
runtime.

`spikes/harness/escrow_test.go` walks the lifecycle there: seed (test hook) ->
fund -> second-fund rejection -> early-refund rejection -> refund -> second-refund
rejection -> accounting, ending at
`{"offer":1,"escrowed":0,"callerCredit":100}`. Gas for the calls sits around
103,000 wasm gas each, well inside the 512,000,000 ceiling.

Two deliberate refusals rather than fakes:

1. `registerOffer`, `fulfill`, and `withdraw` return explicit
   `*_not_implemented` errors. Nothing accepts placeholder evidence, and
   `withdraw` will not clear a credit before the promise-based transfer exists.
2. The `seedOffer` hook exists only under `--features test-hooks`. The harness
   verifies the release build has no such export.

Honest scope: the accounting evidence comes from a seeded offer. It shows the
state machine and deadline logic behave, not that a real purchase settles. That
claim waits for Package 4's proofs.

Next in this package: promise-based transfers for `withdraw`, then the refund
path exercised across a restart, then wiring the real registration and delivery
verifiers once Package 4 exists.

### Package 5 - credit totals and the liquidity invariant (2026-09-25)

The escrow now tracks total outstanding credits and reads its own balance, which
makes the invariant gate G-4 names checkable in one call - a contract cannot
iterate its storage, so an aggregate is the only way to compare total liabilities
against the balance.

Measured in the runtime (`SOURCES.md` F-39):

| Check | Result |
|---|---|
| `accounting` after fund and refund | `{"offer":1,"escrowed":0,"callerCredit":100,"creditTotal":100,"liquid":100}` |
| Invariant `liquid >= escrowed + creditTotal` | holds |
| `cancelOffer` unfunded, then fund | refused after cancellation |
| `cancelOffer` twice | refused |

I also confirmed from the node that `env.balance()` returns the *contract's* own
balance (`wasm_env.go Balance` reads `ctx.ContractAddr()`), so the `liquid` figure
in the accounting view is the right quantity and not a caller balance misread.

The promise ABI for `withdraw` is now mapped rather than guessed: transfers are
created with `create_transfer_promise` (which returns nothing, deducts the
contract balance, and takes the next promise index), callbacks are attached with
`promise_then` and must be named with a leading `_`, and the result is read with
`promise_result` using statuses 0/1/2. The correct shape follows: zero the credit
before creating the transfer, restore it from the callback on failure. That is
the next slice, and the harness must be able to drive promise completion before
the path can be claimed as tested - until then `withdraw` keeps refusing.

Open items O-1 to O-7 and assumptions A-1 to A-2 are listed in `SPEC.md`
section 12.

### Package 5 - withdrawal reservation and the release export check (2026-09-25)

`withdraw` now reserves the caller's credit before creating the transfer promise
and settles the reservation from the callback path, instead of paying out of a
live credit balance.

Measured in the runtime with `spikes/harness/escrow_withdraw_test.go`, starting
from a caller credit of 100:

| Step | `accounting` result |
|---|---|
| `withdraw` | `{"offer":1,"escrowed":0,"callerCredit":0,"creditTotal":100,"pendingTotal":100,"liquid":100}` - reserved, not spendable |
| second `withdraw` while the first is pending | refused: `no_credit` |
| failed outcome | `{"callerCredit":100,"creditTotal":100,"pendingTotal":0,"liquid":100}` - credit restored |
| successful outcome | `{"callerCredit":0,"creditTotal":0,"pendingTotal":0,"liquid":100}` - reservation and credit cleared |

The test hook calls the same function as the real callback, so the state machine
under test is the state machine that ships. One real bug was found on the way
and fixed: the first implementation reused one buffer region for both promise
arguments, and the amount write overwrote the address bytes before the import
read them. Each argument now gets its own heap region, and the exposed-buffer
helpers that encouraged sharing are gone.

Release artefact check extended to the new hook. `wasm-objdump -x` on the
no-feature build lists `_withdrawResult`, `accounting`, `allocate`,
`cancelOffer`, `deploy`, `fulfill`, `fundOffer`, `memory`, `refund`,
`registerOffer`, and `withdraw`; neither `seedOffer` nor
`simulateWithdrawResult` appears. The build is warning-free with
`--features test-hooks` and warns only about test-hook-only helpers without it.
After lowering, the harness artefact is 16,502 bytes and the no-feature build
is 14,666 bytes, both with zero bulk-memory and zero float operators.

Superseded later the same day: the block below shows that a transfer promise
carries no callback in this runtime, so this reservation state machine was
replaced by an atomic payout. The measurements above stand; the design they
describe does not.

### Package 5 - the payout is atomic: no callback exists to wait for (2026-09-25)

The reservation design assumed a promise callback reports the transfer outcome.
Reading the pinned runtime shows the opposite: a transfer promise carries no
callback, and the value movement is already settled inside the call.

The reading, with sources:

- `create_transfer_promise` debits the contract's own balance before it queues
  the transfer, and returns an error when the contract cannot cover the amount
  (`idena-wasm` revision 7e113895, `src/imports.rs`).
- Promises execute at the end of the same top-level call, and the
  `Action::Transfer` arm credits the receiver there; it never runs a callback
  (same revision, `src/runner.rs`, where that arm carries `//todo : handle
  result`).
- A failed call leaves no trace: the node commits only when the call succeeded
  (`idena-go` `vm/wasm/vm.go`).

So `withdraw` consumes the credit and creates the transfer in one call: either
the whole credit leaves the contract and reaches the caller, or the call aborts
and the credit is still there. The reservation record, the aggregate that
tracked it, the `_withdrawResult` export, and the hook that simulated it are
gone.

The harness models per-address balances and the node's commit rule now
(`spikes/harness/env.go`), which makes a payout observable instead of invisible.
Measured in `spikes/harness/escrow_withdraw_test.go`:

| Check | Result |
|---|---|
| payout of a credit of 100 | credit 0, `creditTotal` 0, `liquid` 0, contract 0, caller 100 |
| nested calls made by `withdraw` | none |
| second `withdraw` | refused: `no_credit` |
| payout while the contract holds only 50 of the 100 it owes | aborted: `backend error: insufficient funds` |
| state after that abort, model applied | credit 100, `creditTotal` 100, caller unpaid |
| the same credit after the contract is topped up | paid in full |

Two claims recorded earlier in this file are corrected here: the withdrawal does
not need a callback, and the harness does move balances when the module moves
them. Gate G-4 is cleared against the pinned runtime on that basis; parity with
the runtime revision mainnet actually executes stays open as `O-10`.

Artefacts: harness module 12,922 bytes, release build 11,797 bytes, both with
zero bulk-memory and zero float operators. Release exports: `accounting`,
`allocate`, `cancelOffer`, `deploy`, `fulfill`, `fundOffer`, `memory`, `refund`,
`registerOffer`, `withdraw`.

Next in this package: the refund path exercised across a restart, then the real
verifiers once Package 4 lands. Package 4 opens with the buyer-bound delivery
program, which needs no historical state; registration stays behind Package 2's
candidate list.

### Package 5 - the order binds the buyer's delivery key (2026-09-25)

`fundOffer` now binds the paying caller and a delivery key, not just the payer,
which is what `SPEC.md` 7.1 and 6.6 require before a delivery proof can exist.

The argument and the proof are canonical, and `SPEC.md` 6.6 records the
definition:

| Item | Definition |
|---|---|
| `fundOffer` argument | offer id (8, big-endian) \|\| delivery key (65) \|\| possession signature (65) |
| Delivery key | uncompressed secp256k1 public key, the form the runtime's `ecrecover` returns |
| Signed digest | Keccak-256 over `fod:delivery-key:v1` \|\| contract address (20) \|\| offer id (8) \|\| order nonce (8) \|\| caller (20) \|\| delivery key (65) |

The offer record gained the delivery key and an order nonce, and the nonce
advances on every funding, so a proof made for one order cannot settle an offer
that was refunded and funded again.

Measured in the runtime. The harness now implements the node's `keccak256` and
`ecrecover` callbacks with real cryptography (geth secp256k1, the library family
the node itself uses) instead of stubs:

| Check | Result |
|---|---|
| fund with the buyer's own key and proof | succeeds, gas 158,575 (103,991 before the proof) |
| `orderView` after funding | `{"state":"funded","buyer":"02...","orderNonce":1,"deliveryKey":"04..."}` |
| proof signed by a different key | refused: `key_not_bound` |
| proof bound to another payer | refused: `key_not_bound` |
| proof bound to another offer id | refused: `key_not_bound` |
| empty key and signature | refused: `key_not_bound` |
| short argument | refused: `bad_args` |
| second funding while funded | refused: `offer_not_open` |
| refund, then the stale nonce-1 proof | refused: `key_not_bound` |
| refund, then a fresh key with a nonce-2 proof | succeeds, `orderNonce` 2, new key stored |

One real bug surfaced on the way, and it was in this repository's own code: the
argument reader took a Region's `capacity` as its byte count, but the runtime
sets only `length` when it writes a region (`idena-wasm` `src/memory.rs`). Until
now every argument read had been scanning the whole 4 KiB buffer and stopping at
the first byte that was not a digit, which is why the decimal parsers never
noticed. `read_arg` returns `length` now.

Artefacts: harness module 18,843 bytes, release build 18,156 bytes, both with
zero bulk-memory and zero float operators. Release exports: `accounting`,
`allocate`, `cancelOffer`, `deploy`, `fulfill`, `fundOffer`, `memory`,
`orderView`, `refund`, `registerOffer`, `withdraw`.

Fidelity note: the harness charges the node's host gas for `keccak256` (10,000
wasm gas) and `ecrecover` (30,000 wasm gas) but charges nothing for the older
callbacks, so absolute gas figures are a floor, not the node's exact number.

Next in this package: the refund path exercised across a restart, then the real
registration and delivery verifiers once Package 4 lands.

### Package 5 - credits keyed by the full address, and two buyers through a restart (2026-09-25)

The last open local item in this package was the refund path across a restart.
Writing that test found a real bug in the contract.

Credits were keyed by `credit/` plus the first eight bytes of the buyer's
address. Two buyers whose addresses share that prefix therefore shared one
credit record: the second refund added to the first buyer's balance, and either
of them could withdraw the sum. The multi-buyer test caught it immediately -
after two refunds of 100 the caller saw `"callerCredit":200`. An attacker can
grind an identity key for a matching prefix, so this was not only a correctness
bug but a way to interfere with another buyer's credit.

Credits are now keyed by the full 20-byte address (`key_for_address`), and the
storage helpers take either a numeric id or a raw key. Nothing else in the
contract truncated an address.

Measured in the runtime with two buyers, two offers, and a modelled node
restart (`spikes/harness/escrow_refund_test.go`, `env.Restart`, which copies the
modelled state into a fresh environment so only storage and balances carry
over):

| Step | Result |
|---|---|
| buyer A and buyer B each fund one offer | `escrowed` 200 |
| both orders expire, both are refunded | `creditTotal` 200, and each buyer's own `callerCredit` is 100 |
| after a restart, buyer A's accounting | still `callerCredit` 100 |
| buyer A withdraws | A holds 100, the contract holds 100, buyer B's credit is still 100 |
| buyer B withdraws | B holds 100, the contract holds 0, `creditTotal` 0 |

Artefacts after the change: harness module 20,437 bytes, release build 19,736
bytes, both with zero bulk-memory and zero float operators. Release exports are
unchanged: `accounting`, `allocate`, `cancelOffer`, `deploy`, `fulfill`,
`fundOffer`, `memory`, `orderView`, `refund`, `registerOffer`, `withdraw`.

Package 5 now has no open local item: what remains in it is the registration and
delivery verifier wiring, which waits on Package 4.

### Package 4 - the delivery statement proves (2026-09-25)

Package 4's buyer-bound half does not depend on the historical data that blocks
registration, so it opened first. `proofs/delivery` now implements the delivery
statement of `SPEC.md` 5.6 and produces a real Groth16 receipt for it.

The statement, recorded in `SPEC.md` 5.6 as delivery v1:

- Secret material: the canonical `fod:epoch-secrets:v1` bytes - domain, epoch,
  author address, and the epoch's two private scalars - 106 bytes.
- Commitment: SHA-256 over those bytes, the value registration will commit to as
  well.
- Encryption: the pinned node's repository-local ECIES (`ECIES_AES128_SHA256` on
  secp256k1) with the CTR IV taken from the second KDF block instead of a random
  draw, so the ciphertext is deterministic and therefore provable.
- Journal: 209 bytes naming version, contract, offer, order nonce, seller,
  buyer, buyer delivery key, secrets commitment, and the ciphertext hash.

Measured on this machine (`proofs/delivery/dist/delivery.json`):

| Quantity | Value |
|---|---|
| Guest execution | 7,405,568 cycles in 8 segments |
| Proving wall clock | 957.8 s |
| Peak resident memory | about 8.9 GB |
| Seal / journal / ciphertext | 256 / 209 / 219 bytes |

The receipt is verified on the host before anything is written, and the journal
is parsed and checked field by field afterwards.

The check that matters most here is cross-language: `tools/fod`'s
`TestDeliveryCiphertextDecryptsWithNodeLibrary` decrypts the ciphertext the Rust
guest produced with the node's own ECIES library and recovers the exact 106
secret bytes, matches them against the journal commitment, matches the published
ciphertext against the journal hash, and confirms an unrelated key fails. A
guest and a host sharing one implementation can agree while both being wrong;
this test is what rules that out.

Against `SPEC.md` 5.7's usability targets, delivery proving took 16 minutes of a
30-minute allowance and 8.9 GB of a 16 GiB allowance. Both fit, with the memory
figure the tighter of the two, and neither has been re-measured on other
hardware.

Next in this package: embed the verifier in the contract and wire `fulfill` -
check the journal against the funded order, the commitment against the
registered commitment, and the ciphertext against its hash - then measure the
verification gas in the real runtime.

### Package 4/5 - the escrow verifies a real delivery proof and settles (2026-09-25)

The delivery verifier is inside the contract now, and `fulfill` settles a funded
order against the receipt the delivery program actually produced.

The piece Package 1's probe left out was the claim binding: that probe verified a
seal against a digest handed to it by the prover, so it proved nothing about
which program ran or what it committed to. The host does not work that way -
`Receipt::verify` rebuilds `ReceiptClaim::ok(image_id, Pruned(sha256(journal)))`
and compares digests - and neither does the contract now. The escrow recomputes
that digest from the journal bytes and the delivery image id, using its own
SHA-256 and the `tagged_struct` construction, and the recomputation reproduces
the proven run's digest exactly (escrow unit test).

Measured in the Idena runtime:

| Check | Result |
|---|---|
| `fulfill` with the real receipt | accepted, gas 195,630,810 of the 512,000,000 ceiling (38.2%) |
| Order afterwards | `sold`, `escrowed` 0, seller credited 100, `liquid` 100 |
| `deliveryView` | returns exactly the proven ciphertext |
| Altered seal | refused: `invalid_proof` |
| Altered ciphertext | refused: `ciphertext_mismatch` |
| Journal naming another offer | refused: `wrong_offer` |
| Journal naming another delivery key | refused: `wrong_delivery_key` |
| Second settlement | refused: `offer_not_funded` |
| Escrow module | 349,279 bytes with test hooks, 348,459 bytes in the release build, zero bulk-memory and zero float operators |

The claim binding cost is small: 195.6M gas against the 195.3M the Package 1
probe measured for verification alone, so the journal checks and the digest
recomputation together add about 0.3M gas.

Two things worth recording beyond the feature:

1. A real bug in this repository's contract: `allocate` returned one shared
   buffer for every host request, so loading the offer record overwrote the
   argument the contract was still parsing. The symptom was
   `stale_statement` on a proof that was perfectly valid. `allocate` now hands
   out a fresh region from a 64 KiB arena, which is what the ABI expects.
2. A harness hazard: `go test` caches results by Go inputs, so a rebuilt
   `.wasm` does not invalidate a cached pass. Every escrow test run must use
   `-count=1`; a cached result can otherwise report a contract that no longer
   exists.

What is left in these two packages is registration. `registerOffer` still
refuses by design, and the commitment `fulfill` checks against is set only by
the test hook, so an offer cannot be created with a commitment in the release
build yet. That needs Package 2's candidate list, which this machine does not
hold.

### Package 6 - the interface runs from a file (2026-09-25)

`app/` builds the interface into one self-contained HTML file: React and
TypeScript bundled by esbuild with the stylesheet inlined, so there is nothing
to fetch at runtime. `npm run verify` drives the built file through Chrome from
a `file://` URL and checks the workflow end to end:

| Check | Result |
|---|---|
| Opens from `file://` | yes, `file:///.../app/dist/flips-on-demand.html` |
| Default view | Market, "No state loaded" until something is imported |
| State import | provenance bar shows network, contract, block height, timestamp, and source |
| Market | Open filter shows 1 offer, All shows 2 |
| Offer | price and epoch shown, eligibility evidence expands, purchase disabled on a funded offer |
| My trades | the imported order is listed with its status |
| FAQ | renders offline |
| Console errors | none |
| Remote requests | none - the page loads nothing beyond itself |
| A file with an unsupported version | refused with a message, no partial render |
| Markup in an imported field | rendered as text; no element created, no script ran |

Artefact: `app/dist/flips-on-demand.html`, 247,947 bytes, committed so it can be
opened without a build step.

Deliberate limits of this slice: the provider Session view is not built (it
belongs with Package 7), and the purchase action is visible but disabled -
turning it into an action request that the CLI validates and submits is the next
step of the file-based workflow in `SPEC.md` 4.3.

Also worth knowing for anyone re-running the checks: the playwright CLI refuses
`file:` URLs, which is why `app/verify.mjs` drives the browser through the
Playwright library instead. The screenshots it writes land in
`output/playwright/`.

### Package 6 - the action request reaches the contract (2026-09-25)

The interface writes a typed action request from an open offer the viewer does
not own; the CLI turns it into the exact call the escrow expects. `SPEC.md` 4.3
now fixes the request format, and `fod prepare-purchase` validates it, checks the
buyer it names against the supplied nodekey, builds the delivery-key possession
proof, and prints the operation, contract, offer, amount, and calldata.

Verified in two places:

| Check | Result |
|---|---|
| Interface, in Chrome through `file://` | Purchase downloads a request naming the operation, offer, order nonce, buyer, amount, and contract |
| CLI unit tests | calldata is 138 bytes with the offer id, delivery key, and signature; the proof recovers to the delivered key over a hand-assembled message; ten malformed or foreign-request cases are refused |
| Runtime cross-check | the escrow accepts the CLI's calldata in the Idena runtime and funds the order with the CLI's delivery key and order nonce (gas 160,516) |

The cross-check works because the CLI and the contract build the possession
message independently. That is also what caught the next item.

### Package 5 - found: amounts above 18.4 iDNA saturate (2026-09-25)

The cross-check prepared a 100 iDNA order and the order view came back with
`"price":18446744073709551615` - `u64::MAX`, not 1e20. Prices, escrowed totals,
and credits are `u64`, and the amount parser saturates, so:

- any price above about 18.4 iDNA is stored wrong,
- two different over-range prices compare equal, so a buyer could pay less than
  the seller asked,
- credits and totals saturate instead of adding.

Every escrow test until today used amounts that fit in `u64` (the accounting
examples used 100 *base units*, not 100 iDNA), which is why this survived the
deadline, payout, and settlement tests. Recorded as `O-11` in `SPEC.md`; the fix
is to widen the contract's amount fields and re-run the suite at a realistic
scale, and it is the next piece of work in this package. No release claim
depends on the current code.

### Package 5 - O-11 closed: amounts are 128-bit and exact (2026-09-25)

The overflow found by the CLI-to-contract cross-check is fixed, and the escrow
is now exercised at a realistic scale instead of with toy amounts.

Prices, escrowed totals, per-buyer credits, and the credit total were `u64`.
With 18 decimals that holds only about 18.4 iDNA, so a 100 iDNA order saturated
to `u64::MAX`. They are now 16-byte big-endian `u128` values, the payable amount
is read as `u128`, and the decimal printer handles 128 bits. The offer record
grew from 130 to 170 bytes. The CLI refuses an amount above `u128::MAX` rather
than letting it saturate, and says so.

Every escrow test now prices at 100 iDNA (50 for the cancellation case), and the
numbers come back exact:

| Step | Result |
|---|---|
| `orderView` after the CLI's prepared purchase | `"price":100000000000000000000` |
| Two refunded offers | `escrowed` 0, `callerCredit` 1e20, `creditTotal` 2e20, `liquid` 2e20 |
| After one buyer withdrew | `callerCredit` 1e20, `creditTotal` 1e20, `liquid` 1e20 |
| Invariant `liquid >= escrowed + creditTotal` | holds at that scale |
| Gas | seed 109,242; fund 161,165; refund 110,479; fulfill 195,631,562 (38.2% of the ceiling) |

Artefacts: harness module 352,934 bytes, release build 351,926 bytes, zero
bulk-memory and zero float operators. All ten escrow tests pass, and `O-11` is
closed in `SPEC.md` with the amount range stated in 7.3.

### Package 7 - the provider gateway runs (2026-09-25)

`gateway/` is the shared-node gateway of `SPEC.md` 4.4 and the reference session
integration of 8.2: customers enroll, receive challenges, submit answers, and
collect a provider-signed result, without a node or the marketplace CLI. It is
plain Node with no framework, one pool, one provider identity, and one Ed25519
application key.

Verified two ways. First a real start from `example-config.json`: the policy
endpoint answers, `/rpc` is 404, and a protected endpoint without a token is 401.
Then `npm test`, 13 checks against a loopback server:

| Property | Result |
|---|---|
| No RPC passthrough | `/rpc`, `/bcn_blockAt`, `/`, and a traversal-shaped path are 404 |
| Explicit route table | a known path with the wrong method is 405 with `Allow` |
| Bearer-only authorisation | no token, `?token=`, `Basic`, and an unknown token are 401 |
| No ambient session | no `Set-Cookie` is ever sent |
| Assignments | contain no answer digest |
| Scoring, replay, expiry | correct answer scores; replay is 409; an expired token is 410 |
| Result signature | verifies with the published key, fails after one byte of tampering, labelled provider-issued |
| Transaction forwarding | refused 501 rather than relayed |
| Limits | 10 KB answer is 413 against a 4 KiB cap; non-JSON is 415; the fourth request in a 3-request window is 429 |
| CORS | configured origins only, never `*`; a `file://` origin gets no authorisation and still needs a token |

Two boundaries are deliberate and written into the service. The gateway trusts
the pool's provenance claim, because it is the provider's own acquisition; a
utility that needs independent proof checks the marketplace. And submission of
already-signed transactions is refused with 501 until that path is designed,
because forwarding caller-supplied transactions would make the gateway a relay.

Not built in this slice: the interface's Session view (the customer half of
8.1's "distributes HTML client, connection file, and rules") and the connection
file it would consume.

### Package 7 - the customer half runs end to end (2026-09-25)

The interface now consumes the connection file a provider hands out
(`fod.provider-config` v1, `SPEC.md` 8.2): import it, see the published policy,
enroll, answer challenges, and collect a provider result that the browser checks
itself with the provider's published Ed25519 key.

Verified in Chrome from a `file://` page against the running gateway
(`app/verify.mjs`, now 22 checks):

| Check | Result |
|---|---|
| Connection file | policy renders: session id, utility, pass threshold, pool epoch |
| Enrolment | challenges listed, with no answer material |
| Answers | both score `correct` through the real gateway |
| Result | `passed`, 2 of 2, chip reads `verified in browser` |
| Network | only the configured gateway origin is contacted |
| Console | no errors |

Wiring this found a real gap in the gateway: it answered no CORS preflight, so a
cross-origin JSON POST from a `file://` page was blocked before reaching any
handler. Preflights now return 204 with CORS headers for allowed origins only,
and the gateway suite grew to 14 checks covering both the allowed and disallowed
cases. That makes twice that a browser-driven check found something the unit
tests could not - the shared `allocate` buffer earlier, this now.

The participant token stays in memory for the tab's lifetime and is never
persisted, because a `file://` page has no origin to scope it. The result screen
says what the signature means: the provider issued the statement, not that the
scoring was honest.

Next in this package: the utility side that accepts a result, and a decision on
whether the gateway should ever submit already-signed transactions (still 501).

### Package 7 closed, Package 8 opened (2026-09-25)

Package 7's exit condition - "customers participate without their own node or
marketplace CLI" - is met and verified in a browser: connection file, policy,
enrolment, challenges, answers, and a result the client verifies itself. One
thing remains deliberately unimplemented: submitting already-signed transactions
through the gateway is refused with 501, because relaying caller-supplied
transactions is the thing a shared-node gateway must not quietly become.

Package 8 started with the session-content export (`SPEC.md` 6.7), implemented as
`fod export`:

| Check | Result |
|---|---|
| Decoded parts | written beside the manifest, byte-identical to what was encrypted |
| Deleted flips | excluded from the export and from the manifest commitment |
| Manifest commitment | reproducible from author, epoch, and sorted live CIDs |
| Epoch keys | absent: the export carries decoded content and says key pairs are excluded |
| Answers | carried only with evidence; otherwise labelled `none`, never guessed |
| Failure modes | wrong vault passphrase and a missing blob both refuse instead of writing a partial bundle |

`fod inspect` gained `-blobs-out`, so the blobs it already verified against their
CIDs are kept for the export rather than fetched twice.

The answers rule is worth stating plainly: mapping historical answers to flips
needs the lottery assignment, the same external data that blocks Package 2's
eligibility half. An export from this machine therefore carries no answers, and
says so, instead of shipping a guess that a session would score against.

Next in this package: the gated-utility example that consumes a signed result
(8.3), then the FAQ reviewed against actual behaviour.

### Package 8 - the utility example, and what a grant is scoped to (2026-09-25)

`utility/` is the other side of `SPEC.md` 8.3: it accepts a provider-signed
session result and decides whether it earns access. It is a separate service
from the gateway, with an explicit surface (`GET /policy`, `POST /access`), and
it implements the canonical payload and the Ed25519 check itself instead of
importing the issuer's helper - the tests sign with the gateway's helper and
verify with the utility's, so the two independent implementations have to agree.

| Check | Result |
|---|---|
| Valid, freshly signed result | granted, with grant id, expiry, and the scope statement |
| Tampered payload | `bad_signature` |
| Unknown provider or no key in policy | `untrusted_provider` |
| Wrong utility, unknown session, failed outcome, expired | each refused with its own reason |
| Malformed result | `malformed_result` for null, numbers, empty objects, strings |
| Quota | second grant for one participant id is `quota_exhausted`; a re-enrolled id is granted again, and the decision still says `provider-participant` |
| Identity requirement | refused `identity_required` without a separate identity check, granted with one |
| Endpoint | `/policy` publishes the scope; unknown path 404; wrong method 405; non-JSON 415 |

The honest part is the scope. A provider-issued participant id is not a person:
re-enrolling produces a new one, so the reference quota cannot be
one-person-one-allocation, and both the response and the policy say so. A
program that needs per-person limits sets `identityVerification.required` and
verifies Idena identity eligibility itself; that is the documented integration
point rather than something this adapter pretends to do.

Package 8's remaining item is the FAQ: it exists in the interface and covers the
same ground, but it has not been swept against what the code now actually does.

### Package 8 closed - the FAQ now matches what the code does (2026-09-25)

The FAQ existed before the code did. Reviewing it against the running system
found three claims that had drifted and one that was missing:

| Claim | Correction |
|---|---|
| "you receive the keys plus the encrypted blobs" | the blobs are public; what a buyer receives is the epoch keys that make them readable |
| "the interface loads nothing from the network" | true for browsing and buying; a provider session contacts the gateway named in the imported connection file, and nothing else |
| Offers can be browsed as if they could be created | added: this build cannot create offers, because registration needs data the project does not hold |
| A provider result | added: the signature proves the provider issued it, not that scoring was honest, and a grant is scoped to a participant id rather than a person |

Re-verified in the browser (`npm run verify`, all checks pass, FAQ section
included).

Package 8 is closed with one gap recorded rather than papered over: the
commitment that `fulfill` checks is still only ever set by the test hook, because
`registerOffer` cannot exist until Package 2's candidate-list data does.

### Package 9 - the release is pinned, not shipped (2026-09-25)

The artefacts exist and are reproducible; nothing is deployed. `release/
build-manifest.mjs` rebuilds the release escrow module without test hooks,
hashes both artefacts, checks two properties, and writes `release/MANIFEST.json`
with the revision, the build commands, and the delivery proof program's
identifiers.

| Item | Value |
|---|---|
| Interface | 257,892 bytes, sha256 `0b38d9d9...` |
| Release escrow module | 351,926 bytes, sha256 `a6207153...` |
| Gatekeeper-forbidden operators | 0 |
| Remote tags in the interface | 0 |
| Delivered proof program | image id `2a4f2f7b...`, 209-byte journal, 256-byte seal, 7.4M cycles |
| Deployed network | none - null in the manifest by construction |

`release/README.md` walks all fifteen items of `SPEC.md` 11.2 and says where each
one stands: seven verified, six partial, two not implemented or not exercised.
It also records the recovery paths (vault, delivery key, refund and withdrawal,
rebuilding the revision) and the seven-step mainnet exercise, none of which has
been run.

Two items cannot be produced from inside this repository:

1. **The candidate-list data** that Package 2's eligibility half and therefore
   registration need. Three reconstruction routes were tried and closed with
   evidence in `SOURCES.md` F-34; a datadir from a node live during a closed
   epoch, or a chain replay, would unblock it.
2. **Approval for the mainnet exercise** in `SPEC.md` 7.5, which spends real
   iDNA and publishes a contract.

Until both exist, the honest state of this project is: everything except the
registration triad (Package 2's eligibility, Package 4's registration proof,
Package 5's `registerOffer`) is implemented and measured, and the release is
pinned but not shipped.

### Package 9 - the services run together (2026-09-25)

`release/check-integration.mjs` starts the gateway and the utility and walks the
loop `SPEC.md` 8.1 describes: enroll, answer the example pool, collect the
provider's signed result, and let the utility decide. The utility's policy is
built from the key the provider publishes, so the signature has to survive an
independent verifier for a grant to happen.

Nine checks pass: both services start, both answers score correct, the provider
signs a passing result, the utility grants access, the grant carries its
`provider-participant` scope, a tampered result is refused `bad_signature`, a
re-enrolled customer is treated as a new scope rather than a person, and one
participant id cannot spend its quota twice.

With that, every part of the system that does not need chain data or mainnet
approval has been run end to end at least once: the interface in a browser, the
CLI against the contract in the runtime, the proofs against the node's own
cryptography, and the provider services against each other.

### Sale rule changed: bundles are retraded, with a 10% ceiling (2026-09-25)

The user asked whether flip keys can change hands more than once. `SPEC.md` now
says they can, under a decay ceiling. The old rule - at most one successful sale
per bundle, with settlement permanently consuming the bundle - is gone.

The rule as written: the author prices the first sale freely; whoever holds the
bundle may list it again at any price up to 90% of what they paid, so each trade
lowers the next ceiling by 10%; only the holder may list; at most one offer per
bundle is live; only a settled sale lowers the ceiling.

Nothing is implemented. `registerOffer` refuses everything, so no bundle record
exists yet - there is no holder, no sale count, and no last price for a ceiling
to be computed from. `fulfill` also has to mutate the bundle, not only the
offer. The delivery circuit is unaffected: the guest reads `seller` from the
environment and only length-checks it, so a buyer who relists proves against
their own address rather than the original author's, with the same proving key.

Definition-of-done item 10 drops from partial to not implemented in
`release/README.md`, because the ceiling is a new requirement with no code
behind it. Off-platform trades stay outside what any of this constrains (5.1,
7.6).

### Package 4 - the delivery prover takes an order, not a built-in vector (2026-09-25)

The delivery host proved exactly one thing: the vector compiled into its own
source, contract address, seller, buyer, offer id and nonce included. Nothing
could deliver against a funded order, which is step 10 of the seller workflow in
`SPEC.md` 6.4. The host takes the order as input now.

```bash
cd proofs/delivery
cargo run --release --bin delivery-host -- --order orders/example-order.json \
  --out dist/order-example.json
```

`--out` defaults to `dist/delivery.json`, and `--order -` reads stdin so a
seller never writes the witness to disk. `--check` validates an order file in
seconds and writes nothing. The artefact gained `fulfill_argument`: the exact
bytes `fulfill` parses, offer id (8) || journal (209) || seal (256) ||
ciphertext (219).

`dist/delivery.json` was not re-proved. Its `test_vector` key became `order` and
`fulfill_argument` was assembled from the bytes already in the file; a script
check confirms the journal, seal, ciphertext, and every measurement are
unchanged and only those keys differ.

Measured on the second artefact, whose terms differ from the built-in vector in
every field - contract, seller, buyer, epoch, author, all four scalars, offer 3,
nonce 2:

| Check | Result |
|---|---|
| Proving run | 7,405,568 cycles in 8 segments, 904.0 s wall clock |
| Journal | 209 bytes, naming offer 3 and nonce 2 |
| `fulfill` in the Idena runtime | accepted, gas 195,605,927 |
| Order afterwards | `sold`, escrowed 0, seller credited 100 iDNA |
| Go compatibility | the pinned node's own ECIES library decrypts the ciphertext to the 106 secret bytes the journal commits to |
| Built-in vector | still settles, gas 195,631,562 |

The nonce is the interesting part. The escrow derives `order_nonce` as the
stored value plus one, so a proof made for nonce 2 cannot fund a fresh offer;
the harness funds, refunds, and funds again, which is the escrow's own route to
a re-opened order. The settled order is therefore the `SPEC.md` 7.2 re-open
path, carrying a delivery proof bound to the post-reopen nonce.

Two operational facts worth keeping:

1. Groth16 proving runs the `stark2snark` step in Docker. With the daemon down
   the run fails after the STARK phase with `docker returned failure exit code:
   Some(1)`, wasting the pass - check `docker info` first.
2. The harness needs `-count=1`; `go test` caches by Go inputs and will not
   notice a rebuilt wasm.
3. `proofs/delivery/Cargo.lock` was stale: `cargo metadata --locked` refused it
   with the committed manifests, so any build rewrote it. The refresh removes
   39 packages no workspace member reaches - `addchain`, `risc0-sys`,
   `risc0-build-kernel`, `rayon`, `zip` and their dependencies - with no version
   changes and nothing added. Local and Groth16 proving both work afterwards.

Still missing is the CLI half: `prepare-delivery` in `SPEC.md` 6.1 does not
exist, so a seller assembles the order file by hand today. That, and the same
wiring for `prepare-offer` and `submit`, is what turns this into the workflow
the spec describes.

### Package 6/5 - the CLI prepares the seller's delivery (2026-09-25)

`prepare-delivery` did not exist, so the delivery proof could only be produced
by hand: a seller had to assemble the prover's order file themselves. The CLI
now covers both halves of the file-based workflow in `SPEC.md` 4.3.

```bash
cd tools/fod
./fod prepare-delivery -request delivery-request.json -nodekey seller-nodekey \
  -prover ../../proofs/delivery/target/release/delivery-host \
  -prepared output/cli/prepared-delivery.json
```

It validates the `fulfill` action request, checks the seller it names against
the nodekey, derives the epoch secrets, hands the order to the prover on
**stdin** so the witness never reaches the filesystem, and writes the prepared
action with the prover's fulfill argument. The prover grew `--no-witness` for
this: the artefact keeps the journal, seal, ciphertext, and measurements and
drops the four secret fields, which the run below confirms by inspection.

Measured end to end, with both calls produced by the CLI and neither half
knowing the other's keys:

| Check | Result |
|---|---|
| `fod prepare-delivery` | offer 7 nonce 1, 927.6 s proving, 692-byte calldata |
| Artefact | no `public_part_secret`, `private_part_secret`, `ephemeral`, `secrets`, or `buyer_secret` |
| Buyer's prepared call funds the order | the escrow accepts `fod.prepared-order`'s calldata |
| Seller's prepared call settles it | the escrow accepts `fod.prepared-delivery`'s calldata, gas 195,582,418 |
| Order afterwards | `sold`, escrowed 0, seller credited 100 iDNA |

The cross-check (`spikes/harness/escrow_cli_delivery_test.go`) reads both
artefacts, refuses them if they name different contracts, offers, or delivery
keys, and then drives the escrow with each side's own bytes. That is the loop
the spec describes: the interface writes two action requests, the CLI turns each
into a call, and the contract funds with one and settles with the other.

`PrepareDeliveryOrder` refuses a request that names another seller or another
author rather than deriving the wrong secrets. Delivering a bundle someone else
authored - the retrade case in 5.1 - needs the secrets that bundle sold, which
this command does not read yet; that is the next gap, along with `prepare-offer`
and `submit`.

### Package 5 - the retrade ceiling is enforced, with two real proofs (2026-09-25)

`SPEC.md` 5.1 has said since the flip-key question that a bundle trades more than
once and loses 10% per trade. The contract allowed one sale: an offer was
consumed on settlement and nothing recorded who held the bundle.

The registry now keeps a bundle record, keyed by the commitment registration
binds, holding the current holder, the settled sale count, the price that holder
paid, and the live offer. `listBundle` is the listing path: only the holder may
call it, at most one offer per bundle may be live, the first sale has no ceiling,
and every later sale is at most `floor(9p/10)` of what the holder paid. A settled
sale moves the bundle to the buyer and records the price; a cancelled listing
frees the bundle again; a settlement whose commitment has no bundle is refused
before anything moves.

Measured in the Idena runtime with two real Groth16 receipts - sale one is
`dist/delivery.json`, sale two is `dist/retrade-order.json`, a new proof whose
seller is the first sale's buyer and whose commitment is the same bundle:

| Check | Result |
|---|---|
| Proving the retrade | 7,405,568 cycles in 8 segments, 944.2 s |
| First sale | 100 iDNA, bundle moves to the buyer, ceiling 90 |
| Non-holder listing | refused `not_holder` |
| One base unit above the ceiling | refused `above_ceiling` |
| Exactly the ceiling | accepted, offer 8 |
| A second live offer | refused `offer_live` |
| The retrade | 90 iDNA, bundle moves again, last price 90, next ceiling 81 |
| Accounting afterwards | escrowed 0, retrade seller credited 90 iDNA, `liquid >= escrowed + credits` |
| Cancel frees the bundle | a cancelled listing leaves no live offer, and the bundle relists under a new id |

Unit tests cover the ceiling arithmetic itself: 90% rounded down, compounding
(100 -> 90 -> 81), and the `2^128 - 1` case where the naive `9 * price` would
overflow.

What is still hook-driven is the bundle's *creation*: `registerOffer` remains
refused, so the harness seeds the record, and no claim here covers how a bundle
comes to exist. Every other step is the contract's own code.

### Package 6/5 - the listing path runs from the interface to the contract (2026-09-25)

The retrade rule was enforced by the contract but out of reach: the interface
could not produce a listing, `fod` had no command to turn one into a call, and
the FAQ still told buyers that a settled bundle "cannot be sold again" - true
when it was written, false since the ceiling landed.

`fod prepare-list` reads a `listBundle` action request and writes the call the
escrow expects: the key commitment, then the price as a 128-bit amount. A
listing needs no signature of its own - the holder proves who they are by
sending the transaction, and the registry checks the holder and the ceiling
against its own state - so the CLI checks only what the snapshot can tell it (a
price above the snapshot's ceiling is refused before anything is signed) and
leaves the binding check where it belongs.

The interface follows: marketplace state can carry bundle records, the trades
view lists what the viewer holds with its ceiling, and a listing button writes
the request.

Verified in three places, each with its own toolchain:

| Check | Result |
|---|---|
| Browser, from `file://` | `app/verify.mjs` lists the held bundle with its 90 iDNA ceiling, and the downloaded request names `listBundle`, the commitment, the price, and the ceiling |
| CLI unit tests | the calldata is 48 bytes, commitment then price; one base unit over the snapshot's ceiling is refused, as is a price larger than the contract can hold |
| Runtime cross-check | the escrow accepts the CLI's own listing calldata (gas 106,935), and the offer it creates carries that price and commitment |
| FAQ | the resale claim is replaced by the ceiling rule, including what the registry cannot constrain |

What the listing path still cannot do is start from nothing: a bundle record
exists only after registration, which remains blocked on the epoch candidate
list, and `submit` still stops at "not submitted".

### Package 6/5 - the CLI signs the transaction, and stops at the network (2026-09-25)

`SPEC.md` 4.3 ended at "the CLI submits through the local node", and nothing
did: the three prepared actions carried calldata that no command turned into a
transaction. `fod submit` does now.

It builds the call with the pinned node's own packages -
`attachments.CreateCallContractAttachment`, `types.SignTx`, `Transaction.ToBytes`
- rather than a second implementation of the encoding, and then checks its own
work the way a node would: the bytes are decoded again, the sender is recovered,
and the attachment is parsed back. Any disagreement between the transaction and
what was intended is a refusal, not a signed file.

Where the values come from is the honest limitation: `-nonce`, `-tx-epoch`,
`-max-fee`, and `-tips` are the caller's, and nothing checks them against the
chain, because this machine runs no node. A stale nonce or epoch makes the node
reject the transaction rather than apply it, which is why the command prints
that note before it writes anything. `-broadcast` sends the raw transaction with
`bcn_sendRawTx`; without it the command signs, reports, and stops.

Measured on the three prepared actions the CLI had already produced, signed with
one synthetic nodekey:

| Prepared action | Call | Transaction |
|---|---|---|
| `prepared-listing.json` | `listBundle`, 48 bytes | 167 bytes, recovers `0xf4083033...` |
| `prepared-order.json` | `fundOffer`, 138 bytes, 100 iDNA | 270 bytes, recovers the same address |
| `prepared-delivery.json` | `fulfill`, 692 bytes | 811 bytes, recovers the same address |

The receipts are read back in `TestSignedReceiptsDecode`, which decodes each
`rawTx` with the node's own types, checks the type, the recipient, the amount,
the method, and the arguments, and recovers the sender - so the evidence is the
written file, not the run that wrote it. Nothing was broadcast: `broadcast` is
false in every receipt.

Two things this does not do. It does not recheck the action against current chain
state (step 4 of 4.3), which needs a node; and it has never sent a transaction,
so `bcn_sendRawTx` is implemented and unexercised. Both are in the readiness
record rather than in a claim.

### Package 6/5 - the money comes back out (2026-09-25)

`SPEC.md` 6.1 lists `refund` and `withdraw`; neither existed, so a buyer whose
order was never delivered and a seller holding credit had no way to reach the
contract's own payouts from the CLI. Both exist now, and the interface writes
their requests.

The two do not share an argument encoding, which is the detail worth recording:
`refund` reads the offer id as decimal text (`37`, the ASCII "7", for offer 7),
while `withdraw` takes no argument at all, because the credit it pays is the
caller's own. Both write a prepared action for `fod submit`.

Verified in three toolchains again:

| Check | Result |
|---|---|
| Browser, from `file://` | the trades view shows the credit with a Withdraw button and a refundable order with a Refund button; the downloaded requests carry `withdraw` with no arguments, and `refund` with the offer id |
| CLI unit tests | the refund calldata is the decimal offer id, the withdrawal calldata is empty, and each command refuses the other's operation and its own malformed requests |
| Signed transactions | 116 bytes for the refund, 117 for the withdrawal; both decode with the node's types and recover the synthetic nodekey |
| Runtime cross-check | the escrow accepts the CLI's own refund calldata (gas 110,493: buyer credited, nothing paid out) and its withdrawal calldata (gas 201,987: the whole credit paid, a second call returns `no_credit`) |

Two details fell out of this. An empty argument survives the transaction
encoding as one empty argument, which is what a call without arguments looks
like. And the possession proof binds the contract address, so the harness helper
that builds a credit now takes that address as a parameter: a proof signed for
another address is refused `key_not_bound`, which is the escrow working as
intended rather than a harness accident.

### Package 2 - a live node was reached, and it holds no lottery records yet (2026-09-25)

Everything here is read-only: no node was restarted, no datadir written, no
transaction sent. Host, credentials, and addresses stay out of this record on
purpose.

The reading half of DoD item 2, driven over an SSH tunnel against a synced
mainnet node:

| Check | Result |
|---|---|
| Node RPC | answers; `bcn_lastBlock` reported height 11,355,363 |
| `fod sync` over 1,364 live blocks | 26 transactions, 0 txset mismatches, no errors |
| Classified live | 11 `submitFlip`, 7 `online`, 3 `send`, 2 `storeToIpfs`, 1 `callContract`, 1 `activation`, 1 `replenishStake` |
| Submissions decoded | 11 |

That closes the "no live node was driven end to end" gap for reading. The
marketplace-state export stays open, because the metadata it needs comes from
registration.

Every chain database on that host was then read for the epoch store's lottery
records. The reader was written for this check because the host has no Go
toolchain, and it was validated first against the local datadir whose contents
F-34 records: it reproduced that result exactly (one epoch, `cid` only, 32 keys)
before being pointed anywhere else. Since the running nodes hold the LevelDB
lock, each read used a hard-linked view of the database on the same filesystem.

| Node | Epoch keys found |
|---|---|
| instance 2 | epoch 228, `cid` only, 33 |
| instance 3 | epoch 228, `cid` only, 31 |
| three running lab nodes | epoch 228, `cid` only, 34 and 34; one with no keys |
| one lab node built from a 2021 snapshot | epoch 91, `cid` only, 2 |
| stopped lab nodes | no epoch keys at all |

No `li` or `ls` anywhere. None of these nodes was live when epoch 228's
`FlipLotteryStarted` block was produced: they have been up between 10 and 39
hours and the block is older. That is the negative result F-33 predicts, and it
comes with the remedy, because a node that keeps running through the next lottery
writes exactly those records. The console reports the next validation at
2026-09-29, which is the epoch boundary where the next lottery fires, so a node
left running across it captures epoch 229's candidate list, and eligibility for
that epoch follows once its validation completes about three days later.

One correction and one scope finding from the same session. The lab node showing
epoch 91 is not a devnet: it is a mainnet node whose datadir descends from a 2021
snapshot and was fast-synced forward, so the old epoch keys stayed while the chain
caught up. Every node on that host is mainnet. And the node's API exposes no
historical per-flip qualification: `FlipApi` serves the ceremony in progress
(hashes, flips, keys) and `DnaApi.Identity` reports per-identity state, so the
qualification outcome of a closed epoch has to be reconstructed from the answers
and the assignment, which is what `SPEC.md` 5.3 asks for anyway.

### Package 2 - the pinned references' licences, and the reconstruction rule (2026-09-25)

The licence question came before the code, because Package 2's remaining half
derives from the pinned node's own qualification rules. Read in this session;
nothing about the checkouts was modified. The licence line in the Package 0
block covered the reuse sources only.

| Source | Licence at the pinned revision |
|---|---|
| `idena-go@9bc57040` | LGPL-3.0: one `LICENSE` file, no `COPYING`/`NOTICE`, no per-file `SPDX-License-Identifier` header anywhere in the checkout, no holder line, no licence section in `README.md` |
| `idena-wasm-binding@f7c71ce3` | LGPL-3.0, same text |
| `idena-wasm@7e113895` | no `LICENSE`, `COPYING`, or `NOTICE` file at that revision and no `license` key in `Cargo.toml`: no licence grant attached. Read through the host's API, because this machine holds no checkout of that repo |

Recorded as `SOURCES.md` F-11 and F-60, and the `THIRD_PARTY.md` rows that said
"see upstream" now name the licence. What follows from it:

- The MIT framing still holds for this repository's own code, and the reuse
  sources stay MIT. The pinned references are not reuse sources: no code is
  copied from them, and nothing derived from their files is relicensed as MIT.
- `tools/fod` consumes `idena-go` as an unvendored build dependency, which is
  use, not copying, and no binary that links it is conveyed: `release/` holds
  the interface HTML and the escrow module, both this repository's own.
- Qualification is reconstructed from recorded behavioural facts, and that is
  the only route: the rules sit behind an unexported type, so the node cannot be
  asked for a closed epoch's per-flip qualification either (F-60).

In the same session the console tunnel was re-established and the console
checked: the forwarding port serves the page, the websocket path upgrades
(HTTP 101) and speaks RFB, and the desktop renders with the app connected. Host,
credentials, and addresses stay out of this record on purpose.

The first slice of that reconstruction is now written down as facts rather than
code: `SOURCES.md` F-61 records how one flip qualifies - the reported and graded
gates, the 75%/66% answer thresholds and the order they are checked in, the
grade and grade-score rules before and after upgrades 10 and 11, and the
exclusions that shrink a committee - and it was checked against the node's own
vectors in `qualification_test.go:55-125`. Committee sizes and the upgrade flags
come from the epoch's consensus configuration, so `SPEC.md` O-2 stays open until
their activation epochs are pinned. The candidate-level point score and identity
state are the next slice; no qualification code has been written yet.

### Package 9 - the release module can be deployed, and the address is checked (2026-09-25)

Nothing could be deployed before this: the repository had a module and no way to
build the transaction that puts it on a chain. `fod prepare-deploy` closes that
gap. It signs a `DeployContractTx` carrying the module, prints the address the
node will derive, and stops at the network unless `-broadcast` is passed.

Measured on the release artefact, with the local test nodekey in `output/cli`
(not an operational identity, and it holds nothing):

```bash
cd tools/fod
GOFLAGS=-modfile=go.local.mod go build -o fod .
./fod prepare-deploy -code ../../release/artifacts/fod_escrow.wasm \
  -nodekey ../../output/cli/nodekey -nonce 1 -tx-epoch 204 -max-fee 1000000 \
  -network idena-local -signed ../../output/cli/prepared-deploy.json
```

| Quantity | Value |
|---|---|
| Module | `release/artifacts/fod_escrow.wasm`, 359,500 bytes |
| Code hash | `17493988996349c0e2adace0a933eb5e5bffd25f59946c9b19212bfc2e224ef1` |
| Address the deploy lands on | `1541d02fe15d164f390fe54625ebf4986bf2c82c` |
| Sender recovered from the signed bytes | `19e7e376e7c213b7e7e7e46cc70a5dd086daff2a` |
| Signed transaction | 359,659 bytes, `broadcast` false |

The signed bytes are checked twice. The CLI decodes them again with the node's
own decoder, recovers the sender with the node's own recovery, and parses the
attachment back before writing anything (`tools/fod/prepare_deploy_test.go`).
Then the harness derives the address from the module a second time - keccak256
over the code hash, the binding's own `PackArguments`, and the artefact's
deployment nonce - and requires the CLI's address, the module bytes inside the
signed transaction, and the deployment nonce to agree
(`spikes/harness/escrow_cli_deploy_test.go`, `GOFLAGS=-modfile=go.local.mod
GOTOOLCHAIN=go1.26.5 go test -count=1 -run TestCliPreparedDeploy ./...`).

What this does not do: no transaction was broadcast, no chain was touched, and
the module it names is the one from before the next contract change. A deploy of
a custom module needs the chain's upgrade-11 flag, which `prepare-deploy` prints
as a note because validation refuses the transaction without it (`SOURCES.md`
F-62).

Those numbers are the module as it stood then. The single-sale change below
rebuilt it, and the regenerated artefact is recorded there.

### Package 2 - the flip verdict is code now, checked against the node's vectors (2026-09-25)

Package 2's remaining half started as facts (`SOURCES.md` F-61) and is now code.
`tools/fod/qualification.go` implements the verdict for one flip: the reported
and graded gates with their small-committee counts, the grade and grade-score
rules of both upgrade paths, the answer thresholds in the order they are checked,
and the 34% report share that drops a participant's reports. It is this
repository's own implementation, written from the recorded behaviour under the
rule F-60 sets for the LGPL reference checkouts.

The check is the node's own vector set, all fifteen cases reproduced:

```bash
cd tools/fod
GOFLAGS=-modfile=go.local.mod go test -count=1 -run 'QualifyFlip|Gates|AnswerCounts' ./...
```

| Vector | Result |
|---|---|
| 6 left, 1 right, 1 none, 4 reports of 8, graded 18 of 8 | qualified left, score `18/8` |
| 5 reports of 8 | qualified left, grade reported, score zero |
| 15 left, 3 right, 4 none | weakly qualified left |
| 30 left, 66 right, 4 none | weakly qualified right |
| 4 left, 4 right, 4 none | not qualified |
| 1 left, 2 right, 10 none | qualified by none |

One correction the run produced: the threshold-only vectors leave every
committee input at zero, and that takes the node's default branch, which writes
grade D's score of 2 rather than zero. The first expectation in the test said
zero and was wrong; `blockchain/types/types.go:1327-1340` is what settled it, and
the score map is recorded in `SOURCES.md` F-63 as a fact.

The assembly landed in the same slice. `BuildFlipCommittees` turns an epoch's
answers and assignment into those committees, and reading `reporters.go` for it
produced three rules worth naming, all recorded in `SOURCES.md` F-64:

| Path | What it counts |
|---|---|
| Before upgrade 11 | the score sum is over the grade enum values, `GradeNone` counts for nothing, and the graded gate divides by the number of answers |
| From upgrade 11 | every grade counts towards the committee and the score, so `GradeNone` adds one, and the reported gate's committee is approves plus reports |
| Both | a participant that reported on 34% or more of its flips loses its reports; from upgrade 11 it also loses its grades when it approved nothing at all or approved more than one grade above D |

Two of the first fixtures written for that slice were wrong and the rules
corrected them: a candidate with two grades above D is dropped, and a candidate
that only ever answered `GradeNone` is dropped with its grades. Both are now
asserted by their own tests, so the next reader does not have to rediscover them.

The candidate-level scoring followed in the same slice. `ScoreCandidate` turns a
participant's answers into its point score, its qualified count, and the set of
flips that counted, and `CandidateFlipStatus` carries the short session's status
override. It is checked against three of the node's own tests, all reproduced:

| Vector | Result |
|---|---|
| `Test_qualifyCandidate`, short session | 2.5 points over 3 counted flips |
| `Test_qualifyCandidate`, long session | 4.5 points over 6 counted flips |
| `Test_qualifyCandidate`, answers that fail the short-answer binding | 0 points, 9 counted |
| `Test_qualifyCandidateWithFewFlips` | 1.5 points over 2 |
| `Test_getFlipStatusForCandidate` | all nine assertions |

One inversion was caught by running the node's own test in the pinned checkout
(read-only; nothing was written to it) rather than by reading alone. The
reported-flip rule skips the flip in the *short* session and scores it normally
in the long one, which is the opposite of how the condition reads at a glance.
The direction is now recorded in `SOURCES.md` F-65 with the vectors that pin it.
Two more short-session rules came out of the same slice: a flip past position 6
counts only if the participant bought an extra flip with a none answer on one of
the first six, at most two of them, and a qualified flip counts whatever the
participant answered even though only a matching answer pays.

The eligibility policy itself is now code as well. `CheckBundleEligibility`
decides whether a bundle's covered flips satisfy `SPEC.md` 5.1 and gives a
reason for each flip that does not: `not_strong_consensus`, `reported`, or
`no_verdict`. It carries the version string the manifest commits to
(`fod:eligibility:v1`), refuses an unsupported version by name, refuses a bundle
that covers no flips or repeats one, and returns the covered flips sorted so a
manifest can commit to the decision. One reading is stated rather than implied,
in `SOURCES.md` F-66: "strong consensus" is taken as the node's `qualified`
status, so a weakly qualified flip fails the policy, and changing that reading
means a new policy version.

That closes the reconstruction as far as the spec's inputs allow. What is left
in Package 2 is the identity-state half (what the scored participant becomes in
the epoch's state) and the wiring that feeds these functions from
chain-committed answers. The wiring needs the candidate list, and the epoch
range needs O-2 answered, so the section 4 row that waits on that list stays
open; nothing here claims eligibility has been reconstructed for a real epoch.

### Package 5/6/9 - a bundle is sold once, at the author's price (2026-09-25)

The user took resale back: it is not a priority, and the rule that avoids
problems is the simple one. `SPEC.md` 5.1 now says a bundle is sold once, the
author prices it freely, and settlement consumes it. Nothing about it is a
ceiling, a counter, or a hand-off, so all three are gone:

- the contract loses `retrade_ceiling`, the `sales` and `last_price` record
  fields, the ceiling check in `listBundle`, and the ownership hand-off in
  `fulfill`, which removes the bundle record instead (`remove_storage`);
- the interface loses the ceiling column, the pre-filled capped price, and the
  FAQ claim that a bundle can be listed again;
- the CLI's `prepare-list` no longer takes a ceiling, and a request that still
  carries one is refused with a message saying the price is free;
- the evidence loses the retrade proof artefacts, and the harness test that
  drove two sales is replaced by one that drives the single sale.

| Check | Result |
|---|---|
| Contract unit tests | 4 passed: the bundle record round-trips, a truncated record is refused, and the two verifier tests still pass |
| Release module | 356,374 bytes, sha256 `591c7832606ffa46...`, 0 gatekeeper-forbidden operators |
| Interface | 260,865 bytes, 0 remote tags, and `node verify.mjs` reports all checks passed |
| The one sale, in the runtime, with the real receipt from `dist/delivery.json` | the author lists at ten times the reference price and cancels, lists at three times and settles: the escrow accepts both listings, credits the author 300 iDNA, and leaves `escrowed` 0 |
| After the sale | `bundleView` and `listBundle` both answer `bundle_not_found`, for the author and for the buyer |
| Deploy artefact, regenerated for this module | code hash `34eab3eec9b76efe...`, address `be7a9a296975da85af41f88206b4b5dc6d6b6f53`, 356,374 bytes, not broadcast |
| Harness suite | `go test -count=1 ./...` in `spikes/harness` passes, including the CLI-artefact cross-checks |
| CLI suite | `go test -count=1 ./...` in `tools/fod` passes |

The first rebuild shipped a malformed `bundleView` reply - a missing quote the
edit introduced - and the harness caught it at once. It is fixed, and the tests
that parse the view are what caught it.

Recorded as a constraint rather than a design, for whatever comes next: the user
does not want a market maker, an operator, or anyone in charge of a flip market,
and will not carry responsibility for losses a trusted party could cause. A pool
that custodies keys cannot meet that, because a delivery is proved by whoever
holds the keys off chain; the escrow's model - the buyer funds, the seller
proves, the contract settles or refunds - is the part that has no one in charge.
No pool design is accepted or implemented.

### Package 4 - the manifest is frozen, with a cross-language vector (2026-09-25)

Registration's first artifact is the one `SPEC.md` 5.5 item 1 needs: the
manifest of the complete author-epoch inventory. `tools/fod/manifest.go` builds
and validates it, and `BundleManifest` now carries the fields `SPEC.md` 4.5
lists, with the commitment an offer will carry.

The layout is recorded in `SOURCES.md` F-67 and frozen as a vector any
implementation has to reproduce, which is what the spec's cross-language fixture
rule asks for:

| Quantity | Value in the vector |
|---|---|
| Manifest | network `idena-local`, author `3333...3333`, epoch 204, two CIDs, a 32-byte anchor, consensus `idena-go@9bc57040...`, policy `fod:eligibility:v1` |
| Inventory commitment | `dddda520b96d48a74a88c0c93fdcfecda8bb885d146d736f4c54e2e6efe4d2ba` |
| Manifest commitment | `d886a180dd8d1b1c2d857440688390d1607bd2e2400e2c9539a5b806e6bc3cb1` |
| Vector file | `proofs/registration/vectors/manifest-v1.json`: the manifest, both commitments, and the canonical bytes in hex |

```bash
cd tools/fod
GOFLAGS=-modfile=go.local.mod go test -count=1 -run Manifest -v ./...
```

Four tests pass: the vector is reproduced byte for byte, every field changes the
commitment, malformed manifests are refused (empty inventory, repeated CID, a
CIDv0 value, a short anchor, missing labels), and CID order does not change the
commitment because the builder sorts. Reusing the content export's inventory
commitment instead of writing a second construction is deliberate: `export.go`
now names it `inventoryCommitment` and the manifest layers the anchors on top.

Not built yet, and recorded rather than implied: the validation anchor's
canonical source, the registration claim and its guest program, and the contract
side that verifies the receipt and creates the bundle record. Nothing here makes
registration work.

### Package 4 - what the registration guest costs, measured (2026-09-25)

Before writing a registration circuit, the question is what its primitives cost
in the zkVM. `spikes/registration-cycles` answers it: a guest that runs one
primitive family at a time, times itself with its own cycle counter, and commits
the deltas, driven by a host that executes it through the RISC Zero executor. No
proof is produced, so a measurement takes seconds.

```bash
cd spikes/registration-cycles
cargo build --release
./target/release/cycles-host flip 4     # modes: read, raw, sha256, aes, ecdh, flip
```

| Work | Cycles | Per byte |
|---|---|---|
| Ingest 4 KiB as `Vec<u8>` (serde) | 629,725 | ~154 |
| Ingest 4 KiB with `read_slice` | 15,760 | ~3.8 |
| SHA-256 over 4 KiB | 261,225 | ~64 |
| AES-128-CTR plus HMAC-SHA256 over 4 KiB | 564,895 | ~138 |
| One secp256k1 ECDH | ~3,360,000 | - |
| The per-flip mix at 4 KiB | 7,947,606 | - |

Three consequences, recorded in `SOURCES.md` F-68:

- A blob must be read as a slice. Serde's element-by-element `Vec<u8>` costs
  about 154 cycles per byte against 3.8 for `read_slice`; at the 600 KiB blob cap
  that is 92M cycles against 2.3M.
- One receipt per bundle is the only workable shape, because one Groth16
  verification costs 38% of the per-call gas ceiling and at most two fit in a
  call. Proving then follows the delivery proof's ratio (about 8,200 cycles per
  second): roughly 2.8 hours for ten 4 KiB flips and 5.4 hours for twenty.
- What stays open is whether the guest must produce plaintext or only verify the
  pinned construction's tag. The spec says the secrets "authenticate and
  decrypt"; checking the tag alone would save about 138 cycles per byte and is a
  policy decision to make before the circuit is written.

Nothing about registration works yet: no claim, no guest, no contract-side
verification. This block is the measurement the next design step needs.

### Package 4 - the registration claim is frozen (2026-09-25)

The second artifact registration needs is the public input set a receipt commits
to. `tools/fod/registration.go` derives it from a manifest and the key
commitment, and `JournalBytes` produces the 178-byte layout recorded in
`SOURCES.md` F-69: statement version, contract, author, epoch, covered flips,
and the four digests (manifest, key, eligibility policy, consensus version).

```bash
cd tools/fod
GOFLAGS=-modfile=go.local.mod go test -count=1 -run RegistrationClaim -v ./...
```

Three tests pass:

| Test | What it establishes |
|---|---|
| the vector is frozen | the derived claim and its journal match `proofs/registration/vectors/claim-v1.json` byte for byte, and the contract, author, and consensus fields sit where the escrow will read them |
| every field binds | changing any of the eight fields changes the journal |
| refusals | an unsupported version, a short contract or author, an empty bundle, a short digest, and a non-hex digest produce no journal |

The two label digests are the digests of the exact label strings, and the vector
reproduced them with `shasum -a 256` outside this implementation, so the binding
is not just self-consistent. The layout deliberately carries no per-flip data:
the verdicts, the CIDs, the blobs, and the secrets stay in the guest's private
input, which is what the commitments bind.

What is left before a registration can happen: the guest that commits these
bytes, the escrow's checks (the two label digests as constants and the caller
against the author field), and the wiring that turns real epoch data into a
manifest. Nothing yet registers anything; `registerOffer` still refuses every
call.

One question came up before writing that guest: where its decryption should
live. The answer is in `SOURCES.md` F-70 and it is a measured one. A fresh build
of the delivery crate reproduces the pinned image id `2a4f2f7b...524e4`; adding a
single unused public function to `fod-delivery-core` moves that id; removing it
moves it back. So the delivery crate is frozen in practice - extending it would
invalidate the contract's pinned constant, the manifest's recorded id, and every
stored receipt - and the registration guest gets its own implementation of the
construction, with a shared vector keeping the two honest. The source of the
delivery crate is unchanged (`git diff` on it is empty).

The construction itself is now in place on the registration side.
`proofs/registration/core` (crate `fod-registration-core`) carries the pinned
ECIES construction in the decrypt direction, the canonical secrets encoding and
its commitment, and the CIDv1 raw sha2-256 helper the manifest's CIDs are checked
against. It is a second implementation rather than a shared crate, for the reason
F-70 measures, and `proofs/vectors/ecies-v1.json` is what keeps the two honest.

```bash
cd proofs/registration && cargo test          # 3 tests
cd proofs/delivery && cargo test -p delivery-host   # 2 tests
cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 -run VectorsCID ./...
```

| Side | What it proves against the shared vector |
|---|---|
| delivery core | derives the vector's public key and reproduces its ciphertext |
| registration core | recovers the vector's plaintext, refuses a changed body, a changed tag, a wrong secret, and a truncated message, and computes the vector's CID and commitment |
| the CLI | its multibase CID string decodes to the vector's CID bytes |

The key commitment and the CID in the vector were both reproduced outside these
implementations (`shasum -a 256` over the canonical bytes, and the CID's own
digest), so the three are agreeing on a value that does not come from any of
them. The delivery guest's image id was re-read after adding the host-side test
and the vector: unchanged, so the tests do not disturb the pinned crate.

The qualification rules followed, in the language the guest will use.
`proofs/registration/core/src/qualification.rs` carries the flip verdict, the
gates with their boundaries, the 34% report share, and the eligibility policy;
`proofs/registration/vectors/qualification-v1.json` is the single file both it
and the CLI check against, so the two implementations cannot drift apart
quietly.

```bash
cd proofs/registration && cargo test      # 4 tests
cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 -run QualificationVector ./...
```

| Vector part | Cases | Source |
|---|---|---|
| flip verdicts | 15 | the pinned node's own `Test_qualifyOneFlip` |
| eligibility decisions | 6 | this project's policy |

Both sides reproduce every case, including the boundary cases that are easy to
get wrong: the small-committee counts, exactly half a committee not counting as
reported, and the zero-committee vectors that carry grade D's score rather than
zero. The Rust rules had to be written for `no_std`, so the pre-upgrade-10 mean
is rounded by integer arithmetic instead of `f64::round`, and that is recorded in
`SOURCES.md` F-72.

The epoch assembly followed in the same slice. `build_flip_committees` in the
registration core turns the assignment and every participant's answers into the
per-flip committees, including the three upgrade-11 drops and the tracker's
accounting, and six epoch fixtures were added to the shared vector so both
implementations answer with the same numbers:

| Fixture | What it pins |
|---|---|
| three participants, three flips | the tracker's count and score, including `GradeNone` scoring 1 |
| a report-heavy participant | its reports leave; from upgrade 11 its grades leave too |
| more than one grade above D | dropped from upgrade 11 only |
| a participant that never approved | dropped from upgrade 11 only |
| a moderate reporter | stays, with its grades |
| a void participant | contributes nothing in either path |

The provenance of that section is labelled differently in `SOURCES.md` F-72 on
purpose: the verdict cases are the pinned node's own test literals, while the
epoch numbers are this repository's reading of the tracker's rules, so a reader
knows which claims rest on the node and which rest on the record.

The blob path closed the last unknown before a guest can be written: whether this
Rust code can open something the node produced. It can, and it is now pinned by a
vector built with the node's own primitives.

`proofs/registration/vectors/blob-v1.json` carries a blob made through the CLI
from a throwaway identity key: the node's `crypto/ecies` encrypted the two parts,
the node's protobuf message wrapped them, and the vector records the blob, its
CID, both plaintexts, both secrets, the canonical secrets, and the commitment.

```bash
cd proofs/registration && cargo test --test blob_v1     # 4 tests
cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 -run NodeBuiltBlob ./...
```

| Side | Result |
|---|---|
| registration core | parses the protobuf, opens both parts with the secrets, renders the CID in the manifest's multibase form, reproduces the canonical secrets and the commitment, and refuses a tampered, wrongly keyed, truncated, or non-protobuf blob |
| CLI | the vector's nodekey derives the vector's secrets and author, the node's own library decrypts both parts to the recorded plaintexts, and the CID and commitment match |

The detail that made it work is recorded in `SOURCES.md` F-73: the node's ECIES
wire layout has no version byte and carries its IV, which is the one difference
from the delivery construction, so a decrypt that reads the IV from the wire
opens a node-produced blob. The CLI also gained the canonical-secrets helper, so
the journal's key commitment is now computable on all three sides that need it.

The registration guest exists. `proofs/registration/methods/guest` reads the
epoch's assignment and answers, the author's blobs, and the secrets; establishes
the four things a proof can establish (the policy over the covered flips, the
blobs against their CIDs, the secrets against the blobs, and the journal's
binding); and commits the 178-byte claim. `proofs/registration/host` builds the
input from the frozen vectors and checks the result.

```bash
cd proofs/registration && cargo build --release
./target/release/registration-host --check      # executes the guest, no proof
./target/release/registration-host --negative   # four mutated inputs, all refused
```

| Check | Result |
|---|---|
| the journal | exactly the bytes of `claim-v1.json`, so the Rust guest and the Go claim builder agree end to end |
| a tampered blob | refused: the CID no longer matches |
| a manifest CID the inventory lacks | refused: the manifest and the inventory disagree |
| a covered flip the committee reported | refused: the eligibility policy |
| a wrong secret | refused: the blob does not open |

Two things are recorded rather than implied, both in `SOURCES.md` F-74. First,
the proof cannot establish that the inventory is complete with respect to the
chain: it checks that the manifest and the inventory name the same CIDs, which is
consistency, and completeness is checkable by anyone from the chain's submissions
for that author and epoch. Second, the structure check is deliberately weak -
non-empty, within the node's blob cap, and valid UTF-8 - so nothing stronger is
claimed about the flip's shape.

One of the negative fixtures taught something worth writing down: with every
participant reporting the covered flip, the guest accepted the bundle, and it was
right to. Each participant had reported all of its own flips, so all of them were
dropped as spammers under the 34% rule and the flip ended up unreported. The
fixture now gives each participant two other flips so the reporters survive.

Not built yet: the sealed receipt. `--prove` exists and has not been run; the
escrow's `registerOffer` still refuses every call, and nothing registers.

### Package 5 - the registration receipt verifies in the runtime (2026-09-25)

`registerOffer` no longer refuses everything. The blocker was a byte-order bug in
the contract's registration image id, and it was found by comparing conventions,
not by re-proving.

`methods.rs` carries a guest id as eight `u32` words, and the canonical byte
string is each word's little-endian bytes. The delivery host already wrote the id
that way (`w.to_le_bytes()`), and its output matches both the byte array the
contract pins and the proved delivery run, so `DELIVERY_IMAGE_ID` is right. The
registration constant had been written from the big-endian packing of the same
words, which byte-swaps every 4-byte group: the contract computed a claim digest
no receipt can commit, so every registration seal was refused as
`invalid_proof`.

| Step | Result |
|---|---|
| the constant | `REGISTRATION_IMAGE_ID` is now `a52541d0…`, the little-endian packing; the old `d04125a5…` is that string with every word reversed |
| the host | `proofs/registration/host/src/main.rs` now runs `receipt.verify(REGISTRATION_GUEST_ID)` before writing the artefact and records `claim_digest` and `image_id`, so a pin that drifts from the proof is visible in the artefact |
| the runtime | `TestEscrowRegistersABundleFromASealedReceipt` registers the bundle from the sealed 905.5 s receipt: `registration accepted, gas=195311616`; the same receipt with a tampered journal is still refused as `invalid_proof` |
| the suite | `GOFLAGS=-modfile=go.local.mod GOTOOLCHAIN=go1.26.5 go test -count=1 ./...` in `spikes/harness`: `ok`, 3.985 s |

The fix does not widen what registration establishes: the manifest's completeness
is still not proved (`SOURCES.md` F-74), and the eligibility half of Package 2
still waits for a candidate list. What is closed is the contract path - a real
Groth16 registration receipt is verified inside the Idena contract runtime, so a
bundle can be registered without the `seedBundle` hook.

### Package 4/5 - the CLI prepares and signs the registration call (2026-09-25)

The seller path gained its registration half: `fod prepare-registration` turns a
sealed registration receipt into the `registerOffer` argument, and `fod submit`
signs it. Producing the receipt is still the prover's job, and that is where
Package 2's blocked half sits - the private input needs the epoch's assignment.

| Step | Result |
|---|---|
| the command | `./fod prepare-registration -proof ../../proofs/registration/dist/registration.json -network idena-local -prepared ../../output/cli/prepared-registration.json`: a 434-byte argument (178-byte journal + 256-byte seal), contract `1111…`, author `45869fd2…`, epoch 204, one covered flip |
| the refusals | another statement, a short seal, a seal size that disagrees, another statement version, an empty bundle, another eligibility policy, another consensus revision, and a receipt for another registry are each refused before a transaction exists (`prepare_registration_test.go`) |
| the labels | `labelDigest(EligibilityPolicyV1)` and `labelDigest(ConsensusVersionPinned)` are asserted against the two literals `contracts/escrow/src/lib.rs` pins, so a drift fails in Go instead of only as `invalid_proof` at the node |
| the runtime | `TestEscrowAcceptsCliPreparedRegistration` takes the CLI-prepared argument: the bundle registers at 195,311,616 wasm gas, the record's holder is the author, and a stranger's call is refused as `not_author` |
| signing | `./fod submit -prepared ../../output/cli/prepared-registration.json -nodekey ../../output/cli/nodekey -nonce 1 -tx-epoch 204 -max-fee 1000000 -signed ../../output/cli/signed-registration.json` writes a 559-byte raw transaction and stops there; nothing is broadcast |
| the artefact, offline | `cargo test -p registration-host` rebuilds the receipt from the artefact, verifies it against the guest this tree builds, and checks the recorded digests: one pairing check, 0.01 s, no proving. The escrow's check is the same one with the same constants, so an artefact that fails here could never register |

Two bounds stay recorded. The artefact of 20:29 predates the host recording
`image_id` and `claim_digest`, so those two fields are absent from the current
prepared file and appear from the next proving run. And a real bundle still needs
the assignment data of Package 2's eligibility half.

Rebuilding the release module for the earlier registration fix also made the local
`output/cli/prepared-deploy.json` stale: it named 356,374 module bytes while the
repository ships 357,695. The harness caught that
(`TestCliPreparedDeployDerivesTheNodesAddress`), and the prepared deploy was
regenerated from `release/artifacts/fod_escrow.wasm` - code hash
`be2614062a074bc915c076a098aabca845dc19072fce98ad30e131b480b7ec90`, address
`2c01eb709155cf086d9ba7b42fdbbe07619d7c3f` - and signed again without broadcast.

### Package 4/5 - a fresh registration receipt, and the node that must stay up (2026-09-25)

The registration program was re-proved to exercise the two host-side checks that
had never run and to let the artefact record its own identity.

| Step | Result |
|---|---|
| proving | `cargo run --release -p registration-host -- --prove`: 896.5 s, 7,602,176 cycles in 8 segments, 178-byte journal, 256-byte seal. The host's new `receipt.verify(REGISTRATION_GUEST_ID)` passed before the artefact was written, so the check is exercised, not just present |
| the artefact | the journal is identical to the frozen claim vector; `image_id` is `a52541d0…`, the value `contracts/escrow/src/verifier.rs` pins, and `claim_digest` is `4d0f10a4…` - both recorded for the first time |
| the runtime | the fresh seal registers the bundle at 195,304,165 wasm gas (`TestEscrowRegistersABundleFromASealedReceipt` and `TestEscrowAcceptsCliPreparedRegistration`), and the regenerated CLI-prepared file now carries the image id and claim digest too |
| offline | `cargo test -p registration-host`: the artefact rebuilds to a receipt that verifies against this tree's guest in 0.01 s, and the recorded digests match what was verified |

Package 2's eligibility half still needs one thing this machine does not hold: the
lottery candidate list for a closed epoch. The node that has to be live when that
lottery fires is in place - read-only checks only, nothing on the host was
changed:

| Observation | Value |
|---|---|
| unit | the designated test instance's console service on its host, `active` |
| supervision | `Restart=always`, `RestartUSec=5s`, started 2026-09-25 06:45:37 UTC |
| host | up 36 days, load average 2.9 |
| next lottery | the console reading recorded earlier puts the next validation at 2026-09-29, which is the epoch boundary where epoch 229's candidate list is written |

Correction to the Package 2 blocker entry: that host is reachable from this Mac
after all. An explicit identity file plus a fresh
known-hosts file both connect; what failed earlier was the recorded host key and
the identity path this Mac's SSH configuration names, not the key material. The
live-node route is therefore open, and the epoch store can be read over SSH once
the lottery has fired. Only read-only commands were run on the host.

### Package 6/9 - the CLI prices a call before it is signed (2026-09-25)

`SPEC.md` 4.3 asks the CLI to display the operation, contract, amount, and
estimated fee before signing. The estimate was the missing part.

| Step | Result |
|---|---|
| the model | the pinned node derives the gas budget from the fee, and the runtime charges 100 wasm gas per user gas, so an operation's requirement is `ceil(wasmGas / 100)` user gas and a max fee of that times the chain rate (`SOURCES.md` F-76) |
| the numbers | the highest wasm gas each escrow operation consumed in the harness, pinned with provenance: registration 195,304,165; listing 106,204; funding 161,165; settlement 195,632,417; refund 110,479; withdrawal 201,987 - 13 measured runs in total |
| the command | `fod fees -fee-per-gas 1000` prints user gas, the implied max fee, and the share of the 5,120,000 user-gas cap; without `-fee-per-gas` it asks the node (`bcn_feePerGas`) |
| at signing | `fod submit -fee-per-gas RATE` prints the estimate beside the max fee and warns when the max fee cannot buy the measured gas: with `-max-fee 1000 -fee-per-gas 1000` the registration call reports `estimated 1953042000 ... warning ... out of gas`. The transaction is still only signed, not broadcast |
| the gaps | an operation with no measured number produces no estimate (`cancelOffer` today), and `fees_test.go` fails if a prepared operation lacks one; the mainnet rate and the block wall-clock stay unmeasured, so the estimate is only as good as the pin |

Verification: `fees_test.go` (rounding at the boundary, the exact product, the
node's answer forms, coverage of every prepared operation), the full `tools/fod`
suite, and the harness suite - all green.

### Package 9 / research - the mainnet fee, measured, and the model corrected (2026-09-25)

Read-only, from public chain data; no node login and nothing spent.

| Step | Result |
|---|---|
| the rate | `fod fees -indexer https://api.idena.io` reads the public indexer's last block: `feeRate` 0.0000961538461538 iDNA per gas at block 11,355,812, epoch 228 |
| the cost | at that rate a registration costs 188.33 iDNA and a settlement 188.89; listing 0.26, funding 0.41, refunding 0.22, withdrawal 0.31. A bundle price therefore has to cover roughly 377 iDNA of fees on top of the value being transferred |
| the model, corrected | the node charges `feePerGas * size(tx) * 10 + feePerGas * gasUsed`, and `MaxFee` is the budget the gas allowance is derived from, not the payment (`SOURCES.md` F-76, which now records the correction). The first version of that entry said the max fee was the cost; the numbers happen to be close because the compute term dominates, but the reasoning was wrong |
| upgrade 11 | activated by consensus version 11 (`idena-go/config/consensus.go`, window 2023-03-01..03-07), and a deploy carrying code is refused without it (`blockchain/validation/validation.go:744`). `O-5` is closed; `O-10`(the runtime revision mainnet executes) stays open |
| the interface | the FAQ's cost answer now carries the measured magnitudes and the real proving times, and the stale "registration does not exist yet" sentence is gone. The single-file artefact was rebuilt and both sentences are in it (261,101 bytes) |

Verification: `fod fees -indexer https://api.idena.io` (live), `fees_test.go`
including the recorded indexer answer, the full `tools/fod` suite, and the
rebuilt interface inspected for the new text. The Playwright interface check was
not re-run: the change is FAQ text, and the check's claims are about the sections
rendering, which a rebuild does not change.

### Package 9 / lab - a local chain runs the release deploy path (2026-09-25)

Rows 2 and 15 of the release record say the deploy path is exercised only as far
as signing, because no chain exists to broadcast to. A throwaway single-node
chain built from the *pinned* node revision removes that excuse locally, and the
first live broadcast happened on it.

Recipe (nothing outside `~/Library/Application Support/FlipsOnDemand/`, no
reference checkout modified, no real funds):

| Step | Detail |
|---|---|
| the node | `rsync -a --exclude .git` the pinned `idena-go` checkout to a scratch directory and `go build -o /tmp/idena-go-pinned .`; the local IdenaAI binary (1.1.2) panics under automine with a single validator, the pinned revision does not |
| the chain | a genesis with one alloc entry: the repository's local test key (`0x19e7e376…`, the key in `output/cli`), `State: 3`, `Stake: 25e18`, `Balance: 1e27`, `GodAddress` and first ceremony set to that address, network 33404 |
| the node config | `Consensus: {Automine: true, EnableUpgrade10: true, EnableUpgrade11: true}`, RPC on 127.0.0.1:19093 with an API key generated into the 0600 config, `--automine --autoonline` |
| the key | `<datadir>/keystore/nodekey`, which the node reads itself (`idena-go/config/config.go`); the node logs `Failed to decode keystore key` and continues, because the file is a raw hex key |

What it proved, and what it did not:

| Observation | Evidence |
|---|---|
| the chain runs | blocks finalise every ~10 s, `Reached FINAL` with `txs=0/1`, head advancing; `bcn_feePerGas` answers `10000000000000000` (0.01 iDNA per gas for a one-validator network) |
| the deploy is signed and sent | `fod prepare-deploy -broadcast` built a 357,858-byte transaction for `release/artifacts/fod_escrow.wasm` and the node logged `Sending new tx ... Sent own tx push` for `0x01abf8c1…` |
| the node validates fees precisely | two earlier attempts were refused with `invalid max fee` (max fee below the size fee) and `too high max fee` (whole max fee above the block gas cap); both are now understood and the CLI warns about the second (`fod fees`, `fod submit -fee-per-gas`) |
| the deploy is pending, not landed | `bcn_mempool` still lists `0x01abf8c1…` after several blocks, and `contract_getStake` on the derived address `0x40d18800…` returns `Hash: null`; nothing has been deployed yet |
| an unrelated node bug | `bcn_txReceipt` panics the RPC handler in `api.convertReceipt` when the receipt is missing; the node recovers per request |

Next step for the lab: why the block builder leaves the deploy in the mempool.
The likely candidates are the deploy's execution cost against the 5,120,000 user
gas cap at this fee rate (the size fee already uses 3.58e6) and the epoch-1
set-up of a chain whose validator was never part of a ceremony.

### Package 9 / lab - the release module is deployed on a local chain (2026-09-25)

The deploy that was pending in the previous entry landed. Two of the reasons it
had been stuck were mistakes on this side, and both are worth carrying forward:

| Step | Result |
|---|---|
| nonces are 1-based | the pool only makes a transaction executable when `tx.AccountNonce == state nonce + 1` (`core/mempool/txpool.go`, `put`); a transaction with nonce 0 is queued rather than proposed, and the RPC still lists it under `bcn_mempool` |
| the epoch must match the chain's | `createBuildingContext` drops any transaction whose `tx.Epoch != appState.State.Epoch()`; the lab chain reports epoch 0 through `dna_epoch`, not the `InitialEpoch` the genesis carries |
| the transaction | the node's own `online` transaction had taken nonce 1, so the deploy went out with nonce 2, epoch 0, max fee 3.7e22: `broadcast 0xdf7fc0d7…`, included in block 32 |
| the result | `contract_getStake` at `0x40d18800…` returns `Hash: 0xbe2614062a074bc915c076a098aabca845dc19072fce98ad30e131b480b7ec90`, the release module's code hash, so the chain accepted the module and ran its `deploy` export |
| what that costs | `spikes/harness/escrow_deploy_cost_test.go` deploys the same artefact against the real runtime: 3,000,051 wasm gas (30,000 user gas) of the 1,541,420 user gas a 357,858-byte transaction leaves under the cap, so the size fee is not what blocks a release deploy |

A limitation found while reading the contract back: the pinned revision's
`contract_readonlyCall` runs through `VmImpl.Read` → `createContract`, which only
instantiates **embedded** contracts, so a custom module answers `unknown contract`.
Custom contracts execute only through transactions (`VmImpl.Run` →
`wasm.NewWasmVM`). Consequence for the interface's state: on this revision it
cannot read the marketplace through a read-only call and has to read the
contract's storage directly (`contract_readData`, `contract_iterateMap`) or index
transactions and events, which is a Package 6 decision rather than an
assumption to leave implicit.

### Package 4/5 - the delivery carries keys, never flip content (2026-09-25)

Stated by the owner while the lab work was running: the marketplace must deliver
decryption keys for flips that live elsewhere, not the flips themselves. That is
already what the implementation does, and the constraint is now pinned in
`SPEC.md` 5.6 so it cannot drift.

| Point | Where it holds |
|---|---|
| the plaintext is keys | the delivery host encrypts exactly `delivery::secrets_bytes(...)`, 106 bytes: domain, epoch, author, and the two epoch scalars (`proofs/delivery/host/src/main.rs`, `proofs/delivery/core/src/lib.rs`) |
| the envelope is small | `ephemeral key (65) + IV (16) + ciphertext (106) + tag (32) = 219` bytes, which is what the frozen artefact records (`proofs/delivery/dist/delivery.json`, `ciphertext_bytes`) |
| a buyer checks it | `tools/fod/delivery_test.go` decrypts the real delivery with the node's own ECIES library, asserts the plaintext equals the secrets and that its SHA-256 matches the journal commitment; an unrelated key is refused |
| content is not carried | `fulfill`'s argument is offer id, journal, seal, and the envelope; the escrow stores that envelope under `delivery/`, and no path carries flip bytes |
| content stays public | the manifest names the CIDs; the FAQ says it plainly ("The encrypted flip blobs themselves are public: anyone can fetch them, and these keys are what make them readable"), and `fod receive -vault FILE -blob FILE` opens a blob the buyer fetched itself |
| no messaging channel | recovery reads contract state (`SPEC.md` 7.3), so neither an operator mailbox nor direct buyer-seller contact is on the path |

### Package 5/9 - ownership: there is none to hand over (2026-09-25)

The owner asked whether a deployed registry should be made unownable by routing
the deploy through a multisig and then removing their own key. The answer is that
there is no owner to begin with, and the chain enforces it:

| Check | Result |
|---|---|
| the module | the release artefact exports exactly the published surface; `escrow_no_admin_test.go` reads the module's export section and compares every exported name against that list rather than against a deny-list, so an admin hook added under a new name fails too - it also deploys the artefact, shows `accounting` executes, and requires `terminate`, `seedBundle`, `seedOffer`, and `setOfferCommitment` to be refused |
| the chain | `validateTerminateContractTx` returns `InvalidRecipient` for any contract outside the embedded set while upgrade 11 is active (`blockchain/validation/validation.go`), which it has been since March 2023 - so nobody can terminate the registry, and no stake or balance behind it can be recovered that way |
| the address | a contract address is fixed by code hash, argument format byte, and deploy nonce, so the code at an address cannot be replaced: there is no upgrade to seize |
| what remains | only per-offer, per-holder actions exist - the owner field of an offer record, nothing global |

So no multisig and no key removal step is needed, and neither would have a place
to act. Recorded as `SOURCES.md` F-78, pinned in `SPEC.md` 5.1, and guarded by
the harness test above.

### Package 4 / lab - the registration proof can be bound to a lab registry (2026-09-25)

The registration host now takes `--contract`, `--author`, and `--out`. The first
two are what a local chain needs: a deploy lands at an address its code hash and
nonce decide, never at the vector's placeholder, and the caller of `registerOffer`
has to be the journal's author, whose key nobody holds for the vector. `--out`
keeps a lab artefact from overwriting the tracked one.

Verified without proving: `--contract 40d18800… --author 19e7e376… --check`
executes the guest and compares the journal field by field - the header
(version, contract, author, epoch, covered flips) and the two label digests match
the frozen claim, while the manifest and key commitments are recomputed because
the claim domain covers the contract. The mismatch in the middle is expected and
printed, not silently accepted. Also fixed while checking this: the guest reads
those two fields as byte vectors, so the host pushes vectors rather than
fixed-size arrays, which the zkVM's serde encodes differently.

### Package 2/6 - a real bundle, an open offer, and the state the interface imports (2026-09-25)

The lab chain from the previous entries now carries a registry that has been
driven through the seller path with real proofs, and `fod market` produces the
snapshot the interface imports out of the registry's own storage.

| Step | Result |
|---|---|
| a proof bound to the lab | `registration-host --contract 40d18800… --author 19e7e376… --out … --prove`: 957.4 s, 7,602,176 cycles, artefact kept outside the repository |
| registration | `fod prepare-registration` + `fod submit -broadcast`: included in block 118; the registry stored the bundle `fa7127e3…` held by `19e7e376…` and `next0` stayed unset |
| a protocol rule learned | the first listing was refused with `LateTx` ("tx can't be accepted due to validation ceremony"): non-ceremonial transactions are rejected during `FlipLottery` and `ShortSession` (`blockchain/validation/validation.go:187-189`). The lab's ceremony windows were shortened to 60 s so the wait is minutes instead of half an hour |
| the listing | re-sent after the ceremony: included, offer id 1, state `open`, price 1e18, and `next0` became 2; the storage key is the ASCII digit `1`, which is how `key_for` writes ids |
| the state | `fod market -contract … -proofs <registration artefact> -viewer …`: 1 bundle, 1 offer, 0 orders, 0 credits, read through `contract_iterateMap` and `contract_readData` (a read-only *call* cannot see a custom contract on this revision) |
| the interface's verdict | `app/check-state.ts` (the page's own `parseMarketplaceState`) accepts the snapshot: `accepted: 1 offers, 0 orders, 1 bundles, 0 credits` |

Two defects the interface's parser caught, both fixed: the node already answers
storage keys as hex, so re-encoding them produced double-hex bundle commitments;
and the snapshot carried an eligibility object with an empty anchor. Eligibility
evidence is now emitted only when a manifest supplies the anchor, and the offer's
`bundleId` follows the interface's own convention (`<epoch>-<author>`) when the
provenance is known.

### Package 5/6 / lab - a two-party lab chain, and where this lab stops (2026-09-25)

The registry from the previous entry had a wedged sender: a transaction sent with
the *previous* epoch was holding the next account nonce, and because the epoch is
not part of a transaction's hash, no corrected version could replace it - the pool
keeps such transactions across restarts. The chain was rebuilt rather than nursed.

| Step | Result |
|---|---|
| two funded accounts | the fresh genesis funds the repository's test key as the validator (`State 3`, stake 25 iDNA) and the CLI's second key `f40830…` with 1,000,000 iDNA, with a one-hour validation interval and 60-second ceremony windows |
| the same contract address | deploying the release module with the *same deployment nonce* lands on `40d18800…` again, because the address is a function of code hash, argument format byte, and that nonce. The registration proof from the previous chain stayed valid and did not have to be re-proved |
| the flow | bundle `fa7127e3…` registered, offer 1 listed, and the buyer's `fundOffer` signed and accepted into the pool |
| where it stopped | the funding transaction stayed pending while the node began failing its own proposals: `Proposed block is not found err="invalid block roots…"`. A one-validator lab has a fee floor of 0.01 iDNA per gas, and the node's *own* online-status transaction is refused as `invalid max fee` under it, so the validator drops out of good standing and keeps proposing blocks its own validation rejects. There is no live-chain evidence of a funded order yet |

What this lab is good for is now clear: deploying, calling, and reading state, and
checking the interface's parser against real storage. It is not a consensus
testbed. The next attempt should either keep the validator's internal
transactions affordable - a genesis with enough identities to push the fee floor
down, which is a hypothesis, not a measurement - or accept the harness as the
place where funded-order behaviour is exercised.

### Package 2 - the lottery assignment is a computation, not missing data (2026-09-25)

The eligibility half was recorded as blocked on a candidate list this machine does
not hold. Reading the pinned node moved the blocker one step back: the list is not
the only input, and the assignment that maps answers to flips is derived, not
stored. Recorded as `SOURCES.md` F-79.

| Input | Where it comes from |
|---|---|
| lottery seed | the `BlockSeed` of the header 100 blocks before the `FlipLotteryStarted` block (`LotterySeedLag`), clamped to genesis+1; the RPC block DTO has no seed field, so a capture reads it from the chain database |
| candidate list | the identity state at that block, filtered by `IsCeremonyCandidateData` and grouped by shifted shard; no node keeps it (`MaxSavedStatesCount = 100`, F-31) |
| answers | `DbAnswer{Addr, Ans []byte}` in the epoch store: positional, with no flip identifiers |
| assignment | stored nowhere: two distribution passes over the seed and the list, `CandidatesPerAuthor` 13 above seven authors, queue seed `seed64*21-77`, permutation seeds `seed64*77+55` and `seed64*12+3` |

`tools/fod/lottery.go` implements those rules for one shard and reports, per
candidate, which authors its flips came from and which flip indexes it answered in
each session. It is written here rather than translated from the pinned file
(LGPL-3.0, F-11) and checked against
`proofs/registration/vectors/lottery-v1.json`: six cases captured by running the
pinned functions unchanged in a scratch module outside the checkout.

| Check | Command | Result |
|---|---|---|
| the vector | `GOFLAGS=-modfile=go.local.mod go test -count=1 -run LotteryVector ./...` in `tools/fod` | `ok`, 0.57 s |
| the vector is not vacuous | the same test with the author-queue seed changed from `seed64*21-77` to `seed64*21-78` | fails on every case with field-level differences; reverted |
| static | `go vet .` in `tools/fod` | clean |

What this does not do is produce an assignment for a closed epoch: the seed is
recoverable, the candidate list is not. The capture planned for 2026-09-29 is
still the gate, and the work behind it is now a pipeline rather than an open
question: read the seed and the identities from the live node, rebuild the
shard's candidate list, run this port, map each candidate's answer bytes onto the
flips that candidate answered, and only then run the qualification verdict
`tools/fod/qualification.go` already implements.

### Package 2 - the answers land on the flips the lottery gave them (2026-09-25)

The assignment was the missing half of the join. `tools/fod/epochjoin.go` now
turns one shard's long answers into a verdict per flip: the payload's flip count
comes from the candidate's long-session assignment, and each position is looked
up in that assignment instead of being inferred from the payload length, which is
what `answers.go` had to do while the assignment did not exist. Recorded as
`SOURCES.md` F-80.

| Check | Result |
|---|---|
| the placement | `TestEpochJoinPlacesLongAnswersOnAssignedFlips` builds payloads with the node's own encoder against the frozen lottery case, and reads back `qualified/left` for the flip whose graders all answer left and approve, `qualifiedByNone` for the flip whose graders answer nothing, and the reported grade for a flip with four or more graders that all report it |
| the width rule | `TestEpochJoinReadsShortPayloadsAndRefusesWideOnes`: a payload whose zero tail drops off is read, because the encoder writes a big-endian integer; a twelve-flip payload against a six-flip assignment is refused |
| the negative control | the same test with the join moved onto `ShortFlipsPerCandidate` fails: flip 0 comes out `qualifiedByNone/0` instead of `qualified/left`, so the placement assertions do not pass by accident; reverted |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.0 s; `go vet .` clean |

The eligibility half is now three checked pieces in a row - `AssignFlips` per
shard, `ReconstructFlipVerdicts`, then `CheckBundleEligibility` - and what it
still lacks is the input they all take: an epoch's candidate list and answers,
which no node keeps past the epoch (F-31, F-79). That is what the capture at the
next `FlipLotteryStarted` block has to bring back.

### Package 9 / capture - what the live capture needs, and where this host stands (2026-09-25)

Read-only checks on the host that has to be live at the next lottery, run this
session. Nothing on it was changed.

| Observation | Value |
|---|---|
| the console service | the designated instance's console unit: `active` |
| running nodes | four mainnet nodes, one with a direct public RPC listener |
| chain databases | 909 MB in one node's datadir, 3.5 GB in another; the host's disk is 82% full with 14 GB free |
| toolchain | no Go on the host; gcc 15.2 is present |
| cross-building the CLI | `GOOS=linux GOARCH=amd64 go build` in `tools/fod` fails: `crypto/secp256k1` is cgo-only, so the CLI cannot be built for the host from this Mac |

So the capture cannot be "run the CLI on the host" or "ship the CLI to the host"
as things stand. Three ways remain, with what each costs:

1. Copy that datadir's chain database here and read it with the local CLI. The
   reader opens the database read-only, and goleveldb's read-only path still
   replays the journal (`recoverJournalRO`, `leveldb/db.go`), so records still in
   the write-ahead log are visible. A copy taken while the node writes can still
   be torn, which is the residual risk. Cost: about 1 GB over the network and
   the disk to hold it.
2. Install a Go toolchain on the host and read the records there. Cost: software
   installed on a machine this project has otherwise only read from.
3. Run a mainnet node locally for the days around the lottery, so the datadir and
   the reader are on one machine and the node can be stopped before the database
   is read. Cost: a local node syncing and running, and its disk.

That decision is the project owner's, and it is recorded with the measurements
rather than taken silently. What is already true is that everything downstream of
the capture exists and is checked, so the capture is the only thing standing
between Package 2 and its exit condition.

### Package 6/5 - the buyer's request is rechecked against the registry (2026-09-25)

`SPEC.md` 4.3 step 4 requires the CLI to recheck an action request against current
chain state before it prepares anything, and `purchase.go` recorded that it did
not, because no node was reachable. It does now: the offer's own record is read
out of the registry through the node, and a request the chain disagrees with is
refused rather than adjusted.

| Refusal | Why the chain disagrees |
|---|---|
| the offer is not in the registry | the request names an offer the contract at that address does not hold |
| `funded`, `sold`, or `cancelled` | the escrow funds only an open or refunded offer (`contracts/escrow/src/lib.rs`, `fundOffer`) |
| the price differs | the escrow refuses the payment as `wrong_amount` |
| the order nonce differs | the escrow sets the nonce to the offer's stored nonce plus one and the possession proof is bound to it, so a request built from a stale snapshot would be signed against a nonce the chain will never use |

The read is `contract_iterateMap` over the `offer/` prefix, keyed by the id in
decimal ASCII (`contracts/escrow/src/lib.rs`, `key_for`), which is the same rule
the marketplace reader already relies on. Reading a whole prefix to take one
record is what the node's map interface allows: it walks by a string prefix, and
an id's suffix is not expressible as one.

| Check | Result |
|---|---|
| unit | `purchase_chain_test.go` covers eight cases over the escrow's own record layout: open with the matching price and nonce, a stale nonce, a moved price, funded, sold, cancelled, and a refunded offer funded again with the nonce after its last order |
| the key form | a stub node answers `contract_iterateMap` with the record keyed by the hex of the ASCII digits and the reader finds it; an id that is not there is refused |
| the negative control | ignoring the nonce comparison fails exactly the two stale-nonce cases; reverted |
| live, against the lab chain | `./fod prepare-purchase -rpc http://127.0.0.1:19093` printed `chain       offer 1 is open at 1000000000000000000 base units, next order nonce 1` and prepared the same calldata the earlier run did; the same request with its nonce at 2 was refused with `the chain's next order nonce for offer 1 is 1, the request names 2` and wrote no prepared file |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.2 s; `go vet .` clean |

The lab node was started for the live check and stopped afterwards; it wrote to
its own datadir as a node does, and nothing else was changed.

### Package 6/7 - the interface speaks to someone who has never held a chain key (2026-09-25)

The owner asked for the two journeys the release is for - buying flip keys and
selling a flip set - to work for people who know nothing about blockchains. Both
ended at a downloaded file with nothing on screen to say what it was for, and
both were described in the registry's own vocabulary.

Every download now reports the file and the command that consumes it
(`app/src/views/NextStep.tsx`), and the FAQ carries a walkthrough for buying and
one for selling, offline (`app/src/faq.ts`, "Using this page").

| View | Before | Now |
|---|---|---|
| Market | `open` / `all`, `Author`, `State`, `open` | `For sale` / `All`, `Flip author`, `Availability`, `for sale` |
| Offer | `Purchase`, `order nonce 1` beside the button, `Commitments` | `Buy these keys`, the nonce under `Technical details` |
| My trades | `Withdrawal credit`, `Escrowed`, `Funded`, `Bundles you hold`, `List` | `Money you can withdraw`, `Paid`, `Paid at block`, `Flip sets you can sell`, `Offer for sale` |
| Session | `Enroll`, `Utility`, `Pass threshold`, `Challenge` | `Start session`, `Reward`, `Answers needed to pass`, `Flip` |
| Provenance | `contract 1111…` | `marketplace 1111…`, with the full address in the tooltip |

Two behaviour fixes came out of walking the flow against the contract:

- A returned offer can be funded again: `fundOffer` accepts `Open` or
  `Refunded`, with the nonce after the last order
  (`contracts/escrow/src/lib.rs`). The interface had treated `refunded` as
  unbuyable, so it hid a live offer; it is now listed under `For sale` and its
  button is enabled.
- The refund button keyed off the rendered label text
  (`status.label === "refundable"`), and renaming the label broke it - the
  harness caught that as a timeout. It now keys off a `refundable` field.

| Check | Result |
|---|---|
| browser harness | `node app/verify.mjs`: every check passes, including two new ones for a returned offer (listed, buyable, next order nonce) and four for the file notices; screenshots in `output/playwright/` |
| self-contained | the harness still sees no request beyond the configured gateway, and the regenerated manifest records `interfaceRemoteTags: 0` |
| narrow viewport | at 360 px the document does not overflow (`scrollWidth` 360 against a 360 px viewport); the dense tables scroll inside `.table-wrap` |
| types and bundle | `npx tsc --noEmit` clean; the single file builds to 263,899 bytes |
| artefacts | `node release/build-manifest.mjs` rebuilt the escrow to the same `0a518406…` hash, so the app change moved no contract identity, and the manifest now records the app's `f87c74dd…` |

The language is still the interface's own; nothing here changes what an action
request contains, and the CLI remains the only thing that signs or sends.

### Package 6/5 - the seller and the listing are rechecked too (2026-09-25)

The buyer's path got its chain recheck first; the other two have it now, so
`SPEC.md` 4.3 step 4 holds for every operation the interface can request.

| Path | What is read | What it refuses |
|---|---|---|
| `prepare-list` | the bundle record, by its key commitment | a flip set the supplied nodekey does not hold, and one that already has a live offer - the same two refusals `listBundle` makes |
| `prepare-delivery` | the offer, its order, the head block, and the last block's interval | an order that is not funded, a nonce, buyer, delivery key, or seller that differs from the chain's, a bundle the registry no longer holds, a deadline that has passed, and a deadline with less wall-clock left than the proof takes |

`prepare-list` now takes `-nodekey`, because the holder check is the point of the
recheck and nothing else can say which address will send the listing.

The delivery margin is measured rather than assumed: the proof takes 927.6 s
(recorded above) and the node reports how long its last block took, so the CLI
refuses when `blocks left x seconds per block` is below that. A chain that reports
no interval - an automined lab, say - falls back to the deadline alone.

| Check | Result |
|---|---|
| unit | `registry_test.go`: three listing cases (held, foreign holder, already listed) and thirteen delivery cases (matching order, open, sold, refunded, four field mismatches, missing bundle, passed deadline, too little margin, enough margin, and no observable interval) |
| the key forms | a stub node answers `contract_iterateMap` with the bundle keyed by its commitment and the reader finds it; a commitment the registry does not hold is refused as `not in the registry`, which is distinct from a node that could not be asked |
| the negative controls | dropping the holder comparison fails the foreign-holder case; setting the proving time to zero fails the margin case; both reverted |
| live, against the lab chain | `prepare-list` with the holder's key: `this flip set already has offer 1 live`; with a foreign key: `held by 19e7e376..., and the supplied nodekey controls 5f8ae476...`; `prepare-delivery` for that open offer: `offer 1 is open on the chain, and only a funded order can be delivered` - refused before a proof was started |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 8.9 s; `go vet .` clean |

The lab node was started for the live checks and stopped afterwards, and nothing
else on the machine was changed. One case has no live evidence: the lab's only
bundle already has a live offer and the CLI has no command that cancels one, so
the accepted listing is covered by unit tests alone.

### Package 6 - the CLI records what a submission produced (2026-09-25)

`SPEC.md` 4.3 step 7 asks the CLI to export receipts, which it did not: a signed
transaction said `broadcast` and nothing about what the chain did with it. `fod
submit -broadcast` now asks the node for the receipt, once and then once more
after a block's worth of time, and writes what it said into the same record:

| Field | Meaning |
|---|---|
| `included` | the node had a receipt: separate from `success`, because a transaction that is not mined yet has none and saying so is not a failure |
| `success`, `error` | the contract call's own result, so a refused call is recorded as a mined transaction that returned an error rather than as a lost transaction |
| `gasUsed`, `txFee` | what the call cost, as the node reports it; `txFee` is kept as the node writes it |
| `note` | the node's own words when there is no receipt: not mined yet, or the lookup itself failed |

The first real broadcast found two rules the CLI had been leaving to the node,
both now checked before it sends, with the value that would work named in the
refusal:

- A nonce equal to the account's is invalid: a transaction has to be *higher*
  (`blockchain/validation/validation.go`, `nonce >= tx.AccountNonce`). The node's
  own message is `invalid nonce` with both numbers; the CLI now says
  `the account nonce on the chain is 4, and a new transaction has to be higher:
  use 5`.
- A transaction may not name an epoch below the chain's. The lab chain moved from
  epoch 0 to 1 between two attempts, and the pre-flight caught it:
  `the chain is at epoch 1, so a transaction cannot name epoch 0`.

| Check | Result |
|---|---|
| unit, the record | `submit_outcome_test.go`: a mined call with its gas and fee, a call the contract refused, `null` while unmined, the node's errored lookup, and a transaction that is mined on the second look |
| unit, the pre-flight | five cases over `dna_getBalance` and `dna_epoch`: equal nonce, lower nonce, higher nonce, an epoch behind the chain, and an unreadable account |
| the negative controls | treating a `null` receipt as included fails the unmined case; both reverted |
| live | the first transaction the CLI has ever broadcast: `broadcast 0x64d18f1567c7e8a038f4252df98538a09b90941f63d92da44bb7c167cfcbd133`, accepted into the lab node's mempool; the two pre-flight refusals above were produced live, and the max fee cap (5.12e22 base units at the lab's 0.01 iDNA per gas) was found by the node refusing 1e23 as `too high max fee` |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.0 s; `go vet .` clean |

Step 7 has one gap this build cannot close: `bcn_txReceipt` answers an empty body
on the lab node, because `api.convertReceipt` panics when the node holds a
transaction but no receipt for it yet and the handler recovers per request. The
record therefore says `included: false` with that symptom, which is honest but is
not a positive receipt. The lab node was stopped afterwards, and the transaction
was left in its mempool.

### Package 6 - the interface and CLI walked against `SPEC.md` 4.1 to 4.5 (2026-09-25)

Section 4 requirement by requirement, with what backs each one. Package 6 is the
local HTML, so 4.4 belongs to the gateway and is Package 7's.

| Requirement | Evidence |
|---|---|
| 4.1 opens by double-clicking, hash navigation, no remote script or stylesheet | `app/verify.mjs` loads the built file over `file://` and walks the hash routes; `release/MANIFEST.json` records `interfaceRemoteTags: 0`; the built file contains no `script src=` or stylesheet link |
| 4.1 FAQ offline, provenance shown, untrusted input | the FAQ renders with no network; the header shows network, marketplace, block height, timestamp, and source; the harness feeds markup through a public field and requires no element and no execution |
| 4.1 prohibited surface | the only `fetch` in the app is the gateway client, used after a connection file is imported; no analytics, telemetry, error reporting, `eval`, `new Function`, or `innerHTML` anywhere in `app/src` |
| 4.2 minimal views | Market, Offer, My trades, Session, and FAQ in `app/src/views/`, technical evidence behind `details`, screenshots in `output/playwright/` |
| 4.3 steps 1 to 7 | `fod sync` and `fod market` export state; the interface writes typed action requests; all three prepare commands recheck the registry; the CLI prints operation, contract, amount, and now the estimated fee; `fod submit -broadcast` sends through the node and records the outcome |
| 4.5 public data types | the manifest, offer, order, delivery, marketplace state, action request, provider config, session policy, and session result are versioned types in the CLI, the app, and the gateway; commitments and proof inputs share frozen cross-language vectors (qualification, lottery, journal) |

One requirement was not met and is now: step 5 asks for the estimated fee before
submission, and the CLI showed it only when the caller passed `-fee-per-gas` by
hand. A broadcast now reads the chain's own rate, so the estimate appears with the
cap warnings before anything is sent. Measured live:

```text
estimated   43120000000000000000 base units at 10000000000000000 per gas
            (1612 user gas for 161165 measured wasm gas, 2 runs)
```

Two rows of 4.5 could not be satisfied as written, because they describe a model
the specification itself moved away from, so the specification was corrected
rather than the code bent to match it:

- `BundleRecord` asked for sale count and last sale price. A bundle is sold once
  and settlement removes it (5.1), so the registry's record holds the bundle id,
  the holder, and the live offer, and there is no sale history to keep.
- `PurchaseRequest` asked for the delivery public key and its possession proof.
  Since 4.3, the request carries no key material and the CLI builds the proof
  when it prepares the call, which is the safer of the two.

4.4's HTTPS bullet had no recorded position. The gateway README now says what the
code does: the bundled example binds loopback and speaks plain HTTP, and a
gateway that serves anyone else has to be reached over HTTPS, with the token and
the configured origin list as its only authorisation.

The package closes as done. Its one recorded caveat stays open in Package 9: the
positive half of step 7 cannot be shown on the lab node, where `bcn_txReceipt`
answers an empty body, and the two transactions it accepted were still in its
mempool with the offer record unchanged when the node was stopped.

### Package 3 - a live node's own flip opens with this CLI's derivation (2026-09-26)

Package 3's exit condition is "independent vectors match; secrets stay local".
`SOURCES.md` F-35 recorded what was missing: the key tests were self-consistency
against the reference primitives, and the strongest check - opening a real
author-epoch blob - needs the author's own nodekey, which this repository must
never obtain.

The lab chain removes that obstacle without weakening it: there, this project
holds the author's key legitimately, and the node can be asked to encrypt a flip
with its own composition. `flip_submit` does exactly that - it builds the epoch
keys through the node's own `Flipper`, encrypts both parts, adds the blob to the
node's IPFS, and returns the CID.

| Step | Result |
|---|---|
| the node's own encryption | `flip_submit` at epoch 1 on the lab node, two 64-byte parts (all `0x11` and all `0x22`), returned CID `bafkreidoyaebqvlpzmaxoba6vm2v6b2miae5g67ky3lfta2vec4bkyrbd4` |
| the blob | fetched back through the node's IPFS API (`ipfs_get`), 427 bytes |
| this CLI's derivation | `fod derive-keys -nodekey output/cli/nodekey -epoch 1`: public key `02eadfc7…`, fingerprint `2a7ee9f76847f0ad` |
| the decryption | `export-keys` into a vault, then `receive`: `public part 64 bytes, private part 64 bytes`, the public part uniformly `0x11` and the private part uniformly `0x22` - the bytes the node was handed |

Nothing in that chain came from this repository's derivation: the node ran its
own composition and the CLI only had to produce keys that open what it made. The
vector is committed as `proofs/registration/vectors/node-flip-v1.json` with the
blob, its CID, the author public key, the epoch, and both plaintexts, and without
the nodekey: the test reads the local lab key from `output/cli/nodekey` and skips
where it is absent, so no private key enters the repository.

| Check | Result |
|---|---|
| the vector | `GOFLAGS=-modfile=go.local.mod go test -count=1 -run NodeMadeFlip ./...` in `tools/fod`: the blob parses, the author key matches, and both parts open to the submitted bytes |
| the negative control | deriving for epoch 2 instead fails with `ecies: invalid message`, so the test binds the derivation rather than the file alone; reverted |
| secrets stay local | the derivation summary carries public keys, fingerprints, and seed texts only (F-35); the vault is age-encrypted, mode 0600; no step wrote a secret into this record |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.0 s; `go vet .` clean |

The lab node was started for the run and stopped afterwards; the flip it published
stayed in its own datadir.

### Package 4 - the registration prover takes a bundle the CLI assembled (2026-09-26)

The host built its input from the frozen vectors alone, so the prover could only
ever prove the example bundle; the values a real seller would prove had no way
in. They now travel as a file: `--input FILE` carries the contract, network,
author, epoch, validation anchor, the consensus and policy labels, the manifest's
CIDs, the inventory (flip, CID, blob), the two part secrets, every candidate's
assignment with its answers and grades, and the covered flips - the same values
the guest reads, in the same order. `--dump-input FILE` writes the vector-built
input in that shape, which is both a template for the CLI and the fixture for the
round trip below.

| Check | Result |
|---|---|
| the round trip | `--dump-input` then `--input` on the written file ends with `check         the guest commits exactly the frozen claim vector`, so the file path is encoding-identical to the vector path |
| the refusals still hold | `--negative --input` refuses all four mutations, including the tampered blob and the wrong secret |
| a malformed input stops early | lengths and shapes are checked before anything runs: addresses, the anchor, both secrets, a non-empty manifest, inventory, and covered list, and parallel per-candidate lengths |
| the recorded artefact | `cargo test -p registration-host`: the sealed receipt still rebuilds and verifies in 0.01 s |

What a real bundle still needs is the values themselves, which is Package 2's
eligibility half, not plumbing: the CLI can now write the input it assembled and
the host will prove exactly that, or refuse it before a fifteen-minute run.

### Package 2 - the capture route rehearsed, and a Plan B that has to be checkable (2026-09-26)

The lab chain ran a lottery while its node was live, which is the situation the
mainnet capture waits for, so the route was rehearsed end to end on it.

| Step | Result |
|---|---|
| the lottery | the lab node entered `FlipLottery` at 00:39:27 and wrote its epoch store for epoch 1 |
| reading a live datadir | refused: `open chain db read-only: resource temporarily unavailable` - the database keeps its lock while the node runs, so the earlier plan of reading the host's node in place cannot work |
| copying instead | `cp -R` of the 1.1 MB `idenachain.db` while the node ran, then reading the copy |
| the capture | `fod epoch-db -datadir <copy> -inventory-epoch 1`: `seed_bytes=32 candidates=2` - the first time this reader has seen real records rather than an empty store |
| the assignment | `fod assignment` on the same copy: `candidates=2 shards=1 authors=0`, `shortFlipsCount=8` |

That last number is a correction. The node derives the short session as
`ShortSessionFlipsCount() + ShortSessionExtraFlipsCount()`
(`core/ceremony/ceremony.go`), which is eight, not the six the first constant
alone suggests; the assignment command now uses the sum, and the public
explorer's per-identity `shortAnswersCount` of 8 agrees.

### Package 2/9 - the indexer route, and what it is not allowed to be

Some users will not have a node to read, so the public explorer is a permitted
Plan B source - with the rule that its answers must be checkable by someone who
does run one, and that an indexer-only result is never recorded as verified.
Recorded in `SPEC.md` 5.3 and as `SOURCES.md` F-81, whose endpoints are:

| Endpoint | What it is good for |
|---|---|
| `/Epoch/{n}` | the epoch's `stateRoot`, comparable with a node's header |
| `/Epoch/{n}/Identities` | per identity: state, answer counts, `approved` |
| `/Epoch/{n}/Flips` | per flip: cid, author, status, answer, grade |

`fod assignment -indexer URL` now cross-checks a derived assignment against the
indexer's per-identity answer counts: a candidate who answered must have exactly
as many answers as the assignment gives that candidate flips, a candidate who did
not answer is not evidence either way, and a mismatch or an address the indexer
does not list refuses the assignment rather than preferring either side.

| Check | Result |
|---|---|
| unit | a stub indexer with two pages: 3 records, 1 match, 1 mismatch, 1 missing, and the pagination and URL shape asserted |
| unit | an epoch the indexer does not list is refused (`lists no identities`), which is also what the lab epoch produces: `the indexer lists no identities for epoch 1` |
| live client | `FOD_LIVE_INDEXER=https://api.idena.io/api go test -run IndexerLive`: epoch 227 reads 158 identities, 107 with answers, and the epoch's state root `0x40a2a96a...`, the same value `/Epoch/227` returns |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, and `go vet .` clean |

What the explorer cannot supply is the assignment itself or the ceremony's
candidate order; those exist only in a node that was live at the epoch's
`FlipLotteryStarted` block. The indexer route therefore widens who can *check* a
bundle, and does not remove the capture.

### Package 2 - the eligibility decision as one command (2026-09-26)

The pieces existed and were checked one by one, but nothing ran them in order on
a captured epoch. `fod eligibility -datadir <capture> -inventory-epoch N
-author ADDRESS -out FILE` does: it reads the capture (seed, candidate list in
the ceremony's order, and the answers the node collected), derives the lottery per
shard, finds the author's shard and their own flips, decodes every candidate's
long answers against their assignment, builds the per-flip committees, decides
the author's flips, and applies `EligibilityPolicyV1` to them.

The shard's flip list is the candidates' flips concatenated in the captured
order, so the author's own indices follow from the capture rather than from a
guess; the report carries them with each flip's CID, status, answer, and grade,
which is exactly the covered list the registration input needs.

| Check | Result |
|---|---|
| the positive path | a synthetic epoch whose candidates all answer left and approve: the author's flip comes out `qualified` and the bundle is eligible with no exclusions |
| a split committee | one candidate answers the other way: the flip is not qualified and the bundle is refused as `not_strong_consensus` |
| the refusals | an identity the capture does not hold, an identity that submitted no flips, and a policy this release does not prove are each refused with their own message |
| against the real capture | the lab capture from the rehearsal: `the capture holds 19e7e376... in epoch 1, but records no flips for it`, which is true - that identity submitted nothing before the lottery fired |
| malformed captures | a capture address the CLI cannot read stops the reconstruction instead of keying answers under the zero address, which the test suite found while the fake addresses it started with were too short |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.0 s; `go vet .` clean |

The report also names the consensus flags its verdicts were computed with
(`upgrade10`, `upgrade11`), because those decide the thresholds and an unstated
assumption there would be invisible in the output.

### Package 6/2 - the seller's first command, end to end (2026-09-26)

`SPEC.md` 6.1 lists `prepare-offer` and the CLI did not have it. It does now:
`fod prepare-offer -datadir <capture> -inventory-epoch N -author ADDRESS
-nodekey FILE -bundle <inventory.json> -blob-dir DIR -contract ADDRESS -network
LABEL` runs the pieces in the order the seller needs them - the capture becomes a
decision, the decision's covered list is checked against the inventory, the
anchor is resolved (explicitly, or from the explorer and then the node's header
when a node answers), the epoch keys are derived from the nodekey, and the
registration input is assembled, validated, and written. With `-prover` it runs
the registration prover on that input and with `-proof` writes the receipt.

What was missing in code was the committee evidence: the report carried the
decision but not the shard's assignments, answers, and grades, and the input
`proofs/registration/host` reads needs exactly those. The report now carries them
for the run that consumes it, without writing them into the report file - the
report stays the seller's decision document. A candidate whose long answers were
absent stays empty, which is how both the node and the proof program treat it.

| Check | Result |
|---|---|
| unit | a synthetic epoch: the assembly carries the shard's witness, the blob it read, the pinned consensus label, and passes the same checks the prover's host applies before it starts |
| unit | a manifest CID that is not one of the author's captured flips is refused, and a report built without a capture refuses for want of a witness |
| the CLI | `fod help` lists the command; a bare `fod prepare-offer` exits 2, naming the flags it needs |
| against the lab capture | the rehearsal's copy of the lab chain still reads the way it did: `epoch-db` reports `seed_bytes=32 candidates=2`, `assignment` derives `candidates=2 shards=1 authors=0`, and `eligibility` refuses the identity that submitted nothing |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.2 s |

Not run: the prover itself (`-prover`) on a real capture, because that needs an
epoch whose capture exists - the same live mainnet capture the exercise is
waiting for.

### Package 2/9 - the Plan B route, with a node behind it (2026-09-26)

`SPEC.md` 5.3 now pins the third rule the earlier Plan B wording implied: an
indexer-only route may not be the only one, so everything written from an
explorer carries the source, the epoch's state root with the height it belongs
to, and whether a node confirmed that root against its own header. The
confirmation is the one any ordinary node can give, because headers survive
state pruning, so the root stays checkable after the epoch closed.

Two cross-checks run in `fod eligibility -indexer URL`, on top of the existing
per-candidate answer-count check:

| Check | Result |
|---|---|
| the epoch root | read the explorer's `stateRoot`, then read the node's header at the height the root belongs to: agreement records `nodeVerified: true` with the height, disagreement refuses the epoch, and a node that does not answer records `not checked` rather than agreement. The first version of this used `validationFirstBlockHeight` as that height, which the entry below shows is the wrong block and corrects |
| the verdicts | `GET /Epoch/{n}/Identity/{address}/Flips` returns the node's own qualification result per flip, so every covered flip's status and answer is compared with the reconstruction's; a flip the explorer does not list, or lists differently, refuses the report and names the flip, and a record with no published status is counted as unpublished |
| `-require-node` | turns "no node confirmed the epoch root" from a recorded note into a refusal, for callers who will not take an explorer's word for an epoch |
| unit | a stub explorer whose records disagree in exactly one status and miss exactly one flip produces 2 matches, 1 mismatch, 1 missing, 1 unpublished, named individually; a stub node confirms the root, contradicts it (refused), or is absent (`not checked`) |
| live | `FOD_LIVE_INDEXER=https://api.idena.io/api go test -run IndexerLive -v`: epoch 227 reads 158 identities and the state root `0x40a2a96a...`, and `0x012eFb6c...` has 3 flips in it, all three with a published status |
| live refusal | the lab capture asked for a cross-check against the public explorer: `the indexer cross-check failed: the indexer lists no identities for epoch 1`, because the explorer does not have the lab chain - a refusal, not a silent pass |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.2 s |

Not run at the time: the node half of the root check against a real node, because
no node RPC answered on this machine while the change was made (`bcn_blockAt`
against `127.0.0.1:9019` returned nothing). It was run later the same day against
a mainnet node, which is what showed the height to be wrong; see the entry below.

What the explorer still cannot supply, and the tooling still refuses to guess:
the lottery assignment and the ceremony's candidate order, both of which live in
a node that processed the epoch's `FlipLotteryStarted` block. A user without a
node obtains those from a capture or not at all, which is why the capture remains
step 0 of the mainnet exercise.

### Package 2/9 - the epoch root check pointed at the wrong block (2026-09-26)

The check added earlier today compared the explorer's epoch `stateRoot` with the
node's header at `validationFirstBlockHeight`. Run against a real mainnet node, it
refuses a correct epoch: the two values are different fields of the record.

| Epoch | explorer `stateRoot` | root of the header at `validationFirstBlockHeight` (11350664 / -) | root of the header at the epoch's opening block |
|---|---|---|---|
| 227 | `0x40a2a96a...` | `0x03db419e...` - differs | 11328931: matches |
| 228 | `0x82ae4945...` | validation not due | 11350781: matches |

What the explorer publishes as an epoch's state root is the state the epoch
opened from, so it belongs to the block the node itself calls the epoch's start
(`dna_epoch.startBlock`, `State.EpochBlock()`), which is the newest block the
explorer lists for the epoch before it. Recorded as `SOURCES.md` F-82, and
`SPEC.md` 5.3's wording is corrected there rather than dropped.

The check now resolves that height, compares the node's own header at it, records
where the height came from (`epochOpenHeightSource`) and refuses on a
disagreement. A caller who will not take the explorer's word for the height can
name it: `-epoch-block N` is used as given, and if the explorer publishes no
height and the caller names none, the root is recorded as not checked instead of
verified.

| Check | Result |
|---|---|
| live, corrected rule | `FOD_LIVE_INDEXER=https://api.idena.io/api FOD_LIVE_NODE=<mainnet node> go test -run LiveNode -v` in `tools/fod`: `epoch 228: the explorer's state root 0x82ae4945... is the root of block 11350781, which is the block the node names as its start` |
| the falsification | the same node at 11350664 returns `0x03db419e...`, which is not the root the explorer publishes for epoch 227, so the earlier check would have refused that epoch |
| unit | a stub explorer that publishes no blocks for the previous epoch leaves the root `not checked`, with the reason recorded; a caller-named height is used as given |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.2 s; `go vet` clean |

The node was reached over an SSH tunnel to a loopback port on the test host and
authenticated with that node's own RPC key; the key was read into the
environment, never printed, and no state was changed on the host. What is still
not verified: the same comparison for an epoch whose opening block a node has
already pruned, which is what the `-epoch-block` escape hatch and a published
capture are for.

### Package 6/9 - the interface's own check, and one label it got wrong (2026-09-26)

Running the interface's browser check turned up two things, one in the check and
one in the screen.

The check `node app/verify.mjs` failed its own case "a returned offer is still
listed for sale". The screen was right and the check was early: it counted the
market table before the re-imported snapshot had rendered. It now waits for the
row, and prints what the table holds when it fails, which is what showed the row
was there.

The same screen showed a refunded offer as `money returned` while it sat in the
for-sale list - the one reading a buyer must not take from it. `SPEC.md` 7.2 lets
a refunded offer reopen with a new order nonce, so the offer chip now says `for
sale again`. Order rows in Trades keep their own wording; only the offer label
changed.

| Check | Result |
|---|---|
| the interface | `cd app && npm run build && npm run verify`: all checks passed, including "a returned offer can be bought again" and "buying a returned offer takes the next order nonce" |
| types | `cd app && npm run typecheck`: clean |
| the gateway | `cd gateway && npm test`: 14 tests, 14 pass (unauthorised access, no RPC passthrough, transaction forwarding refused, body caps, rate limits, session expiry, CORS) |
| the joined services | `node release/check-integration.mjs`: 9 checks passed (gateway session, signed provider result, tampered result refused, quota spent once) |

The release manifest was regenerated at the revision that carries these changes,
with a clean worktree: `app/dist/flips-on-demand.html` 263899 bytes
`21eeb574...`, `release/artifacts/fod_escrow.wasm` 357695 bytes `0a518406...` -
the escrow artefact rebuilt to the same bytes as before, so the lowering step is
reproducible here - `gatekeeperForbiddenOperators: 0` and `interfaceRemoteTags: 0`.
Deployment is still recorded as not done, for the reasons the exercise lists.

### Package 2/6/8 - an epoch that has not ended cannot produce a bundle (2026-09-26)

`SPEC.md` 5.2 requires a successfully completed validation for every covered
flip, and nothing enforced it. A capture taken while an epoch is still being
validated holds partial answers, and a committee built from them is a snapshot of
an unfinished ceremony - it can come out qualified and be proved, because the
proof checks the arithmetic it is handed, not that the epoch was over.

Two rules close that:

- `fod eligibility` and `fod prepare-offer` ask the node which epoch it is in.
  An epoch the node is still in, or has not reached, has not ended, and both
  commands refuse it. The answer is recorded in the report as `epochEnded`, and
  `-require-node` refuses when no node answered at all.
- A covered flip the explorer lists but publishes no qualification result for now
  refuses the report. It used to be counted as "unpublished" and then ignored,
  which left the one hole where a bundle could pass on absent evidence.

| Check | Result |
|---|---|
| live, past epoch | against the mainnet node: `epoch   checked: the node is in epoch 228, so epoch 1 has ended`, then the capture's own refusal |
| live, current epoch | `fod eligibility -inventory-epoch 228` against the same node: `eligibility refused: epoch 228 has not ended: the node is in epoch 228, so its validation has not finished and no bundle from it can be eligible` |
| unit | a node in epoch 7 refuses epoch 7 and epoch 8, accepts epoch 6, a node that names epoch 0 is not agreement, and no node is recorded as `not checked` |
| unit | the refusal rule: a clean cross-check passes, a missing flip refuses, a count mismatch refuses, and a flip with no published result refuses with the reason named |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.2 s; `go vet` clean |

Package 8's exit condition is that the documentation matches behaviour, so the
FAQ was checked claim by claim against the record. Its numbers hold: delivery
proving 16 minutes and 8.9 GB (recorded against a 30-minute and 16 GiB
allowance), registration 896.5 s and 7,602,176 cycles, the proof-carrying calls
at 188.33 and 188.89 iDNA with listing, funding, refunding and withdrawing under
1 iDNA, and the commission at zero. Two wordings were wrong rather than stale:
"cannot create offers yet" gave the wrong reason (the lottery records come from a
node that was live during the epoch, and the ciphertext from IPFS), and "no
recovery path" read as "no refunds" when the escrow does return an undelivered
payment. Both are corrected, and the interface's browser check still passes.

### Package 2/9 - step 0 rehearsed against the live mainnet node (2026-09-26)

The capture has only ever been rehearsed on a 1.1 MB lab database. The live
mainnet node on the test host is the machine step 0 will actually run on, so the
procedure was rehearsed there while its epoch is still open - a copy taken and
read, then discarded - and the numbers are what step 0 will cost.

| Step | Measured |
|---|---|
| the copy | `cp -a` of `idenachain.db`: 3.2 GB in 1767 files, **12 seconds**, on the node's own disk |
| the round trip | the copy pulled to this machine: 3,417,982,390 bytes in 12 min 12 s (about 4.7 MB/s over the usual path) - only needed if the reading happens off-host |
| the epoch read | `fod epoch-db -datadir <copy> -inventory-epoch 228`: the reader opens a 3.2 GB mainnet database and answers in **0.2 s** - `seed_bytes=0 candidates=0` |
| the key scan | `fod epoch-keys -datadir <copy>`: full keyspace scan in **6.2 s**, 33 epoch keys, all of them `epoch 228: cid:33` |
| what that means | the store holds the node's own 33 flip CIDs for the epoch it is in, and no `li`/`ls` records: the lottery has not fired, so the capture window is still closed. That is the same answer the node's RPC gives (`currentPeriod: None`), from the other side |

Two things this closes: the reader is settled for a mainnet database at real size
and not only for the lab's, and step 0 is cheap enough to run inside the window
without racing it - a 12-second copy, not a 3.2 GB drain. Both copies were deleted
afterwards (the host's disk is back to 14 GB free, the local one is gone), and the
reading printed counts and sub-key names only: no key material was read out.

What is still open is the same thing it was: the window opens when the node leaves
its candidate phase and processes epoch 228's `FlipLotteryStarted` block, before
validation on 2026-09-29T15:00Z. Every piece that consumes the capture -
assignment, eligibility, the seller's registration input, and the prover - is
already exercised on captured data.

### Package 2 - the last chain-only route to a closed epoch is closed too (2026-09-26)

Before waiting on the window, the one route that would have removed the wait was
re-examined: rebuilding a closed epoch's candidate list from chain data. F-31
listed "their shard ids" among the inputs a sweep would supply, so the question
was whether the shard is on chain. It is not, and now with a mechanism rather
than a conclusion (`SOURCES.md` F-83):

- the node's RPC transaction DTO has no shard field (`api/blockchain_api.go`);
- `Transaction.shardId` is an unexported `atomic.Value` cache, so it is not
  RLP-encoded and is absent from the committed block bodies too;
- it is filled from the state at submission (`txpool.go:372`), which is exactly
  the state version the chain prunes;
- the key packages are no help either: `PublicFlipKey.ToBytes()` serialises key,
  epoch, and signature only.

Two further gaps sit beside it, both read from the ported lottery: the reviewers
are walked over the *whole* candidate list (`firstDistribution`, `extraCandidates`
in `tools/fod/lottery.go`), so the committee layout depends on candidates a sweep
cannot see at all - the ones that submitted neither flips nor answers - and the
order inside a shard is the state iteration order, not an address order the chain
records.

So the capture is not a convenience, it is the only source, and the step above
measures what it costs to take one on the machine that will take it. G-2 stays
open for exactly one reason: the window.

### Package 2/9 - no node on the test host holds a closed epoch's records (2026-09-26)

One question was worth settling before submitting to the wait: whether a closed
epoch's lottery records already exist on a machine that is reachable. Epoch 227's
`FlipLotteryStarted` block was produced around 2026-09-24T14:20Z and the epoch
closed at 15:33Z, so any node live across that afternoon would hold them.

| Node on the test host | Running since | Head | Epoch keys in its database |
|---|---|---|---|
| designated instance | 2026-09-25 06:45 | mainnet head | `epoch 228: cid` only |
| second designated instance | 2026-09-25 06:45 | mainnet head | not read - started after the lottery block |
| compatibility candidate/active-peer node | 2026-09-24 01:31 | 11,356,613, the mainnet head | `epoch 228: cid:34` only - full scan of a 3.5 GB database, 7.0 s |
| compatibility legacy node | 2026-09-24 02:12 | mainnet head | not read - same host and sync history |
| compatibility direct-public node (old snapshot) | - | 4,871,247, epoch 91 | old snapshot, not applicable |
| compatibility rc12 snapshot node (old snapshot) | - | 5,211,152, epoch 95 | old snapshot, not applicable |

The two that had been up long enough still hold nothing for 227: they were
fast-syncing while the lottery block was produced, which is the same reason a
freshly started node cannot help. The wait for epoch 228's own lottery block is
therefore the only route left, and the two compat nodes above will be the ones to
capture from when it fires.

An operational finding that matters for step 0, since the capture has to happen
inside a window: **a hardlink copy of a live database is not a usable capture.**
`cp -al` copies directory entries, but LevelDB appends to its MANIFEST in place,
so the copy's MANIFEST keeps growing and ends up naming tables the copy never had:
the reader answered `open /tmp/…/idenachain.db/106631.sst: no such file or
directory`. The recorded recipe (`cp -R`) is the correct one - a real copy of the
same 3.5 GB database took 13 s and opened - and a stale copy can be brought up to
date cheaply, which is what makes a retry affordable if a copy does catch a
compaction: `rsync` against the existing copy moved 202 MB in 30 s. Reading
`fod epoch-keys -datadir <copy>` before the capture is what tells an operator
whether the records they need (`li`/`ls` under the epoch) are actually in hand.

Both copies, host and local, were deleted after the check.

One clarification this makes worth stating: the capture is not a race against the
clock. The lottery records persist in the node's epoch store once written -
datadirs from April still carry their own epoch's `cid` keys months later - so what
the exercise needs is a node that was live at the block and a datadir that
survives, not a copy taken within minutes of it. Epoch 228's records will still be
there days or weeks after its lottery block, as long as that node is not
re-synced from scratch.

### Package 2 - the covered epoch range is pinned and enforced, O-2 closed (2026-09-26)

`SPEC.md` 5.2 requires unsupported epochs to fail with a clear explanation, and
nothing checked which epochs the release's rules actually cover. The question O-2
asks - which range, and how strong qualification is encoded - is now answered and
the answer is enforced.

How the range was derived (`SOURCES.md` F-84):

| Step | Finding |
|---|---|
| which switches decide a flip | exactly two: `EnableUpgrade11` at six sites in `core/ceremony/qualification.go` (167, 173, 203, 398, 427, 434) and `EnableUpgrade10` at line 403. The other epoch-conditional flags in the ceremony feed identity-state and reward logic, not flips |
| when they turn on | consensus V11 (`config/consensus.go:152-157`), activation window 2023-03-01T08:00Z to 2023-03-07T00:00Z, carrying `KeyWordsV3Epoch = 104` |
| which epoch that is | epoch 103 spans block 5,664,108 (2023-02-28T14:04Z) to 5,720,711 (2023-03-13T14:03Z) and validated 2023-03-13T13:30Z, so V11's window closed inside it; epoch 104 validated 2023-03-25T13:30Z. The boundary is 104, which the version's own keyword epoch names independently |

`ConsensusEpochFloor = 104` is now part of `tools/fod`, and `requireSupportedEpoch`
is called by the eligibility report builder, its inner reconstruction, and the
assignment builder:

```
eligibility failed: epoch 103 predates this release's rules: consensus V11
activated upgrade 10 and upgrade 11 at epoch 104, so flips up to epoch 103 were
decided by arithmetic this release does not implement and it refuses them rather
than applying the wrong rules
```

| Check | Result |
|---|---|
| unit | the floor epoch and a current epoch pass; the epoch below fails with all four phrases asserted (predates, consensus V11, both upgrades, does not implement), and both entry points fail rather than the helper alone |
| the CLI | `fod eligibility -inventory-epoch 103` refuses with the message above before reading the capture, and `-inventory-epoch 104` passes the floor and fails later on the lab capture, which holds no epoch-104 records |
| the CLI | `fod assignment -inventory-epoch 103` refuses the same way |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.3 s; `go vet` clean |

`SPEC.md` §12's O-2 is marked closed for this, and 5.2 now names the range. What
stays open next to it is `O-10`: which runtime revision mainnet actually executes,
which is a mainnet claim and cannot be settled from here.

### Packages 2/3/4 - the encoding and ECIES items were already closed in substance (2026-09-26)

While auditing the open-item table against the tree, O-3 (canonical encodings with
cross-language fixtures) and O-4 (ECIES construction with compatibility vectors)
turned out to be answered by work that landed in earlier packages but was never
reflected in `SPEC.md` §12. The table is now corrected rather than the work redone.

| Item | Where it is pinned |
|---|---|
| manifest commitment | `proofs/registration/vectors/manifest-v1.json`, read by `tools/fod/manifest_test.go` |
| registration journal | `proofs/registration/vectors/claim-v1.json`, read by `tools/fod/registration_test.go` |
| blob CID, canonical secrets, secrets commitment | `proofs/registration/vectors/blob-v1.json`, read by `tools/fod/blob_vector_test.go` |
| lottery distribution | `proofs/registration/vectors/lottery-v1.json`, from `core/ceremony/lottery.go` run unchanged, read by `tools/fod/lottery_vector_test.go` and `assignment_test.go` |
| flip verdict and eligibility | `proofs/registration/vectors/qualification-v1.json`, read by `tools/fod/qualification_vector_test.go` |
| delivery journal layout | `proofs/delivery/dist/delivery.json` and `order-example.json`, produced by the Rust host, re-parsed by `tools/fod/delivery_test.go` |
| ECIES construction and a real node's blob | `proofs/vectors/ecies-v1.json` (`construction_test.go`) and `proofs/registration/vectors/node-flip-v1.json`, a blob a live lab node encrypted through its own `flip_submit` (`node_flip_test.go`) |

Fresh evidence for the strongest of them: `cargo run --release -p
registration-host` (check mode, no proving) re-ran today and printed `check the
guest commits exactly the frozen claim vector`, so the Go claim builder and the
Rust guest still agree byte for byte on the 178-byte journal.

Statuses updated in `SPEC.md` §12: O-3 and O-4 closed with these pointers, and
O-7 with the one next to them - the reference session adapter needs no on-chain
component, which is what the gateway's fixed route table and the chain-free
integration loop show. What remains open is O-9's wall-clock half (how long a
proof-carrying call takes on mainnet) and O-10 (which runtime revision mainnet
executes): both are mainnet claims and neither can be settled from here.

### Package 6 - the privacy row's browser half is checked now (2026-09-26)

`SPEC.md` 11.1's privacy row asks that no secret reaches public files, browser
storage, transactions, logs, or proof outputs. Two halves were already covered:
`tools/fod/export_test.go` asserts the content export carries no epoch key
material ("the export leaked the private-part private key" is a failure), and the
gateway's suite asserts it never sets a cookie. The page's own storage was
covered only by construction - the source touches no `localStorage`,
`sessionStorage`, cookie, or `indexedDB` API - and now it is checked as well.

`app/verify.mjs` asserts it at the end of the browser run, after an imported
state, a purchase request, a gateway session, and a signed result have all gone
through the page:

```
ok   the page keeps nothing in browser storage - localStorage 0, sessionStorage 0, cookie ""
```

The whole browser run passes with it (39 checks), and no artefact changed, so the
manifest still describes the same bytes.

### Package 9 - the artefacts reproduce from a clean clone (2026-09-26)

`SPEC.md` 11.1's distribution row asks for checksums, pinned identifiers and
reproducible artefacts. The escrow module had only ever been rebuilt twice inside
the same worktree, which cannot tell a reproducible build from a cached one.

The repository was cloned at HEAD into a fresh directory and the manifest's own
commands were run there:

| Artefact | Clean clone | `release/MANIFEST.json` | Result |
|---|---|---|---|
| `release/artifacts/fod_escrow.wasm` | 357695 bytes, `0a518406a6518854…` | 357695 bytes, `0a518406a6518854…` | identical |
| `app/dist/flips-on-demand.html` | 264085 bytes, `8c82564dad86cc21…` | 264085 bytes, `8c82564dad86cc21…` | identical |

The commands are the ones the manifest records: `cargo build --release --target
wasm32-unknown-unknown` in `contracts/escrow`, `wasm-opt
--llvm-memory-copy-fill-lowering -O3 … -o release/artifacts/fod_escrow.wasm` at
the root, and `cd app && npm install && npm run build`. The clone was at a
revision several documentation commits after the one the manifest names, so the
bytes are stable across those revisions as well, and nothing in either build
depends on worktree state. The clone was deleted afterwards.

### Package 9 - the pinned identifiers are checked, not only recorded (2026-09-26)

`SOURCES.md` F-70 and F-75 record which image ids the escrow pins and why the byte
order matters, and earlier entries carry the numbers from the lab runs - but
nothing compared the contract's constants with the artefacts a prover writes, so
a rebuilt guest would have surfaced only as a receipt that fails to verify, which
is the worst place to find it.

`release/build-manifest.mjs` now parses `DELIVERY_IMAGE_ID` and
`REGISTRATION_IMAGE_ID` out of `contracts/escrow/src/verifier.rs`, compares each
with the `image_id` its proved artefact carries, refuses to write the manifest on
a mismatch, and records the result. The manifest also carries the registration
program now - image id, claim digest, 178-byte journal, 256-byte seal, 7,602,176
cycles in 8 segments, 896.5 s - where before only the delivery one was listed.

| Check | Result |
|---|---|
| today's values | delivery `2a4f2f7b…` and registration `a52541d0…` agree with `proofs/delivery/dist/delivery.json` and `proofs/registration/dist/registration.json`, and `checks.pinnedIdentifiers` says so in the manifest |
| the guard has teeth | the same parse and comparison, run against the constant with one byte changed, reports a mismatch - which is the condition that makes the script exit before writing |
| the manifest | regenerated from a clean worktree, so its revision field names the sources of the bytes it lists |

### Package 2 - the inventory and body checks have tests now (2026-09-26)

`SPEC.md` 11.1 asks for coverage of the inventory's failure modes and of source
authentication. Both had been demonstrated by live sweeps weeks earlier (epoch
204: 437 submissions, 1,285 bodies CID-verified, one deletion) but nothing in the
suite would have noticed a regression, and the sweep is the step that decides
which CIDs a bundle may commit to.

`tools/fod/body_test.go` and `tools/fod/inventory_test.go` pin them, building
their inputs with the node's own encoders rather than with fixtures:

| Scenario | What the test builds | What it requires |
|---|---|---|
| honest body | a two-transaction body encoded by `types.Body.ToBytes`, its CID from `cidRawSha256`, and the node's own transaction listing | CID matches, the body decodes, the transaction set matches, and a derived transaction hash is recorded |
| substituted body | the same CID, one flipped byte | the check must not report a CID match |
| wrong transaction set | the honest body, a listing that names another transaction | the sets must not match, and the RPC count is carried |
| missing body, header without a CID | a fetch that fails, and an empty CID | each is reported in `Error` rather than silently passing |
| repeated CID | two submissions of one CID with different pairs | one entry, `duplicatesSeen` 1, earliest submission kept |
| deleted submission | a deletion for the author and epoch, and one by somebody else | the author's entry is marked deleted and counted; the stranger's is ignored |
| another author or epoch | one submission each | neither reaches the inventory |
| CID the blob does not hash to, and a submission naming no CID | a fetch returning other bytes, and an empty CID | `complete` false with each entry naming its own failure |

The suite grew by three tests and stays green: `go test -count=1 ./...` in
`tools/fod` is `ok` in 9.0 s, `go vet` clean.

What these tests cannot watch is what the tool cannot see either: the header
fields are still the node's word, because the node's RPC does not expose the
header's transaction hash (`tools/fod/body.go` says so where the checks live). The
strongest statement the sweep makes is that a body matches the CID the node
reported and that its transactions match the node's listing for that block - and a
node holder falsifies it by running the same sweep against their own node, which
is what `SPEC.md` 5.3 asks of a source.

### Package 3 - a malformed flip blob is refused where it is read (2026-09-26)

`SPEC.md` 11.1's keys row asks for wrong secrets, corrupted ciphertext, invalid
authentication tags and malformed flip structure. The wrong-secret half was
already covered (`tools/fod/receive_test.go` refuses a vault holding the wrong
epoch keys, `keys_test.go` refuses a public-part ciphertext in the private pair);
the malformed half was not, and the parser was the reason it could not be: an
empty payload parses cleanly as an empty protobuf, and a blob missing one of its
three parts did too, so the failure surfaced later as a decryption error that
reads like a key mismatch.

`ParseFlipBlob` now refuses a blob that is missing any of the three parts the node
writes, and says which: `the flip blob is missing a part: 0 bytes of public key,
0 of public part, 0 of private part`. `keys_test.go` covers the row's scenarios:

| Input | Required outcome |
|---|---|
| a well-formed blob | parses, and the round trip still decrypts |
| bytes that are not a blob | refused at parse |
| an empty payload | refused at parse, naming the missing parts |
| a byte flipped inside the public ciphertext | decryption refuses it - the ECIES authentication tag is what catches corruption |
| a truncated ciphertext | decryption refuses it |

The node-built vectors (`node-flip-v1`, `blob-v1`) still parse, so the stricter
parser does not reject what a real node writes. Suite: `go test -count=1 ./...` in
`tools/fod` `ok` in 9.1 s, `go vet` clean.

### Package 4 - a development-mode receipt stops at the CLI (2026-09-26)

Package 1 carried a residual forward in as many words: "Dev-mode rejection: dev
receipts carry no Groth16 seal, so they cannot reach this path, but end-to-end
rejection belongs to the CLI that consumes receipts." That CLI is
`fod prepare-registration` and `fod prepare-delivery`, and neither had a case
named for it.

| Where a receipt enters | What it requires | Test |
|---|---|---|
| `AssembleRegistration` | a seal of exactly 256 bytes; one with none is refused as `artefact seal is 0 bytes, expected 256` | `TestAssembleRegistrationRefusesBadReceipts`, case `a development-mode receipt, which has no Groth16 seal` |
| `AssembleDelivery` | the same, because the escrow decodes the seal out of the fulfill argument | `TestAssembleDeliveryAcceptsAProvenOrder`, its last case |

So a development-mode receipt - `InnerReceipt::Fake`, no Groth16 seal - cannot
become a contract call from either path, which is what `SOURCES.md` F-21 said had
not been demonstrated. Of that residual only the block-production wall-clock
(`O-9`) is left.

### Package 6 - a restart and an unreachable gateway are checked now (2026-09-26)

`SPEC.md` 11.1's recovery row lists restarts, a restored buyer key, an index
rebuild and an unavailable gateway. The key half is covered (`receive_test.go`
opens a blob with the buyer's own delivery key and refuses a vault holding the
wrong epoch keys, `vault_test.go` refuses a wrong passphrase), but the page's side
of a restart and of a gateway that is not there had no check at all.

The browser run now ends with both, and the order is what makes them mean
anything - the reload comes after a complete session with the live gateway, and
the dead gateway after that:

| Step | What it shows |
|---|---|
| `page.reload()` | the page asks for a connection file again: a restart keeps nothing, which is the same fact the storage check reports from the other side |
| importing a connection file that names `http://127.0.0.1:8799`, where nothing listens | the page answers `the gateway could not be reached` in its alert banner and stays usable |
| the console-error check, moved after both | the one browser-generated error for the refused connection is named rather than console errors being ignored in general; nothing else appears |

```
ok   a reload forgets the connection it had
ok   an unreachable gateway is reported, not swallowed - Session example-provider http://127.0.0.1:8799 the gateway could not be reached Start session
ok   the page survives it
ok   no console errors beyond the refused connection
```

The whole browser run passes (42 checks). What the recovery row still owes is its
mainnet half - a buyer recovering a real delivery after a restart is step 5 of the
exercise, and the lab equivalent is `receive_test.go` end to end.

### Handoff - the definition-of-done table, and what the approval would cost (2026-09-26)

`release/README.md` is the handoff the spec asks for, and four of its rows had gone
stale in the direction of understating the work: rows 4, 5, 6 and 15 now carry the
checks added since they were written - the page's restart and refused-gateway
behaviour, the seller's `prepare-offer` command with the epoch-floor and
epoch-ended guards, the development-mode receipt refusal at both CLI receipt
boundaries, and the manifest's identifier check and clean-clone reproduction.

The exercise section also says what the approval is being asked for, in money
terms, because that is the one number a reader deciding on it needs: the two
proof-carrying calls measured 188.33 and 188.89 iDNA at the rate mainnet charged
on 2026-09-25, everything else in the exercise stayed under 1 iDNA, so about 378
iDNA of fees - and the escrowed price is not a cost, since it moves to the seller
on settlement or back to the buyer on a refund.

State at the time of writing: epoch 228 is still in its candidate phase
(`currentPeriod: None` at 2026-09-26T00:18Z), so the capture window has not
opened; G-2 stays open until it does, and the mainnet exercise needs explicit
per-action approval.

### Package 8 - the FAQ answered half of the questions section 9 asks (2026-09-26)

`SPEC.md` 9 lists eighteen questions the offline FAQ must answer accurately. The
earlier audit checked whether the answers that existed were true, not whether they
were all there - and counted against the list, eight had no answer at all: why
bundles are sold whole, whether a website or a node is needed, what a shared node
provides, whether bought content can be used in a later session, how to organise
gated access, whether a session proves a unique person, whether somebody can
participate again with another wallet, whether participants may change the page,
whether participants should receive the original keys, whether flips can be
reused, and what a complete utility platform includes.

| Section 9's questions | Where they are answered |
|---|---|
| what do I buy; what if key delivery fails; what fees apply | the existing buyer, delivery and cost entries |
| why consensus-qualified flips have value; can AI generate and solve them; is automated solving free | the existing value entry, written to 2.1's premise: costs are real, measurable, and the market sets value |
| does a session prove a unique person; can somebody join twice with another wallet | new entries, using 8.4's framing: repeated participation is a cost, stronger duplicate-person rules need their own identity controls, and a scheduled session is not Idena's coordinated validation |
| can participants change the page to score themselves; should they get the original keys; can flips be reused | new entries: frontend scoring cannot authorise anything, keys stay with the utility, and reuse makes answers easier to memorise |
| why epoch bundles; is a website or node needed; what a shared node provides; can a session be scheduled later; how to organise gated access; what a complete platform includes | new entries, with the marketplace, provider and utility roles separated the way 8.3 describes them |

The FAQ gains a "Sessions and gated access" section and one bundle question in
"Buying"; all eighteen questions now map to an entry, and nothing in the new text
contradicts 2.1, 5, 7 or 8.

| Check | Result |
|---|---|
| the interface | `npm run typecheck` clean; the built file grew from 264085 to 268293 bytes |
| the browser | `node verify.mjs` passes, including the offline FAQ check |
| the manifest | regenerated for the new artefact, so the release record pins the bytes the new text is in |

### Package 7 - the published session policy is a versioned type now (2026-09-26)

Reading `SPEC.md` 4.5's list of versioned public types against the tree, every one
of them carried a `format` and a `version` - the pool file, the provider
connection file, the session result, the utility policy, the action request, the
marketplace state, the manifest - except the one a customer's client reads first:
the gateway's `/session/policy` answer published its fields bare.

That is now `format: "fod.session-policy"` and `version: 1`, and the version is
used rather than only published: `GatewayClient.policy()` refuses anything else,
with the refusal phrased for the reader because the session view only shows
messages from its own `GatewayError` type.

| Check | Result |
|---|---|
| the gateway publishes it | 14 gateway tests pass; the policy test asserts the format and version by name |
| the client refuses what it does not read | the browser run feeds the page a stub provider answering `version: 2`: `this is not a session policy this build reads (fod.session-policy v2)` |
| the rest of the loop | `release/check-integration.mjs` passes, and the whole browser run passes with the new check in it |
| the record | the manifest is regenerated for the rebuilt interface |

The stub needed one thing a real provider configures: `access-control-allow-origin`
for the `file://` page's null origin, without which the browser blocks the answer
before the client can judge it - which is what the first attempt of this check
showed.

### Handoff - a requirement-by-requirement sweep of the sections not yet counted (2026-09-26)

The last two turns found two requirements that were *accurate but absent* - the FAQ
questions section 9 lists and the versioning section 4.5 requires. That suggests
counting the rest the same way, so the sections whose requirement lists had not
been ticked off one by one were swept. This entry is a negative result: nothing
was missing, and here is where each list is answered.

| Section | Requirement list | Where it is answered |
|---|---|---|
| 2.2 | what the first release includes | escrow and marketplace (`contracts/escrow`, `docs/PROGRESS.md`), CLI (`tools/fod`), HTML (`app/dist`), shared-node connectivity and provider config (`gateway/`, `app/src/provider.ts`), offline FAQ and the integration guide (`app/src/faq.ts`, `gateway/README.md`), export and participant format (`export-session`, `utility/`), reference provider integration (`release/check-integration.mjs`) |
| 4.1 | how the HTML must behave | React and TypeScript bundled by `app/build.mjs`; the browser run opens it from `file://`, walks hash routes, asserts no remote requests and no remote tags (`checks.interfaceRemoteTags`), covers the offline FAQ, checks that markup-bearing input is neutralised, and shows source, timestamp and block height in the header; nothing in the page loads code, reports errors, or logs anyone in |
| 4.3 | the seven workflow steps and action-request v1 | `sync`/`inspect`/`market` produce and re-read state; the interface's request carries `format`/`version`/`network`/`contract`/`operation`/`arguments`/`amount`/`createdAt` (asserted in the browser run); every request is rechecked against the registry before preparation; `submit.go` cites section 4.3 step 5 where it reads `bcn_feePerGas` and prints the estimate before sending |
| 4.4 | what the gateway may expose and must keep back | fixed route table with no RPC passthrough, no wallet material (14 gateway tests); CORS only for configured origins and `file://` is not authorisation; bearer-only scoped tokens with expiry; no cookies; body caps and rate limits; no node API key anywhere in the gateway or the client; HTTPS is the deployment's job and the gateway README says to terminate TLS in front of it |
| 6.2 | original-key access rules | the CLI takes an explicit `-nodekey` and never scans for one; derivation is checked against a vector a real node produced (`proofs/registration/vectors/node-flip-v1.json`); identity keys, epoch secrets and delivery keys have separate flags and types |
| 6.3 | vault and recovery rules | `filippo.io/age` with an interactive passphrase (`readPassphrase`, `tools/fod/vault_test.go`), 0600 files in the tests, public caches kept apart from the vault, the prover runs locally, and the delivery artefact carries no witness field |

Together with the two gaps those turns closed and the section 11.1 rows closed
before them, every requirement list in the spec now either has evidence or an
explicitly recorded limit - the two remaining limits being that a body check
cannot authenticate header fields the node's RPC does not expose, and that the
mainnet rows need the exercise.

### Handoff - the escrow harness re-run, and the figures it corrected (2026-09-26)

The escrow's behaviour was last measured when its own entries were written, so the
26 tests in `spikes/harness` - the ones that run the real module inside the pinned
runtime rather than a model of it - were re-run against the current tree:
`GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `spikes/harness`, `ok` in
4.5 s. Everything they cover still holds: deploy, registration from a sealed
receipt, listing, funding with a bound delivery key and its refusal, fulfil with
altered evidence refused, cancellation, refunds across a restart, a refunded offer
reopening with a new nonce, a single sale at the asking price, payout and its
refusal when unbacked, lifecycle accounting, and no administrative entry point.

The strongest of them reproduced exactly: `verify(0): gas=195325117` with
`verified: true` for a real RISC Zero Groth16 receipt, and `verify(1)` refused as
`invalid_proof` at 195,352,445 - the gate G-1 figure, unchanged.

Three call figures had drifted from what the record says, because the older
numbers were measured with a different input rather than because the module
changed (it is byte-identical):

| Call | Recorded then | Measured now |
|---|---|---|
| `registerOffer` | 195,311,616 | 195,304,165 |
| `fulfill` from the CLI's delivery | 195,582,418 | 195,583,273 |
| `refund` | 110,493 | 110,479 |
| `withdraw` | 201,987 | 201,987 |

The current-state rows of this record and of `release/README.md` now cite the probe
that measures each figure, and today's value, so the next re-run is a command
rather than a search. The dated entries keep their own numbers, as the record's
rules ask. Nothing a decision rests on moved: the fee figures in the FAQ and in
the exercise's price are the measured gas times the chain's rate, and 7,451 wasm
gas is 74.5 user gas, about 0.007 iDNA at the rate mainnet charged - below the
precision those documents print.

### Handoff - assumption A-1 is verified before the exercise, not after it (2026-09-26)

`SPEC.md` §12 carries an assumption that had to be settled before any mainnet
action: "the pinned reference node revision used for protocol derivation is
compatible with the mainnet consensus version pinned in Package 2." It is now
verified from the chain's own record rather than argued from the fork's shape.

The test host runs a mainnet node whose journal shows what it did while replaying
the chain: `Consensus config transformed to ver=10`, then `ver=11`, then `ver=12`,
with nothing after. The pinned checkout's table defines exactly 10, 11 and 12 and
no later version, so the revision this repository derives its protocol facts from
carries the rules mainnet is executing - which is what the qualification port
depends on (`SOURCES.md` F-85). The second half was already true from earlier
work: the pinned parsers have read real mainnet transactions, attachments and a
live node's epoch store.

With that, the two things the exercise needed in advance are in place: the
approval's price (about 378 iDNA of fees, measured) and the protocol revision it
would run against (consensus 12, observed). G-2 remains the only gate waiting on
anything other than the approval.

### Handoff - the guarantee boundaries the FAQ was not stating yet (2026-09-26)

`SPEC.md` 7.6 lists boundaries the product must not paper over, and two of them
were absent from the offline FAQ rather than contradicted by it: settlement
depends on the chain including the transaction, so a payment that is only waiting
is not settled; and the ciphertext a delivery carries can be visible before the
transaction that settles it is included. Deadlines reduce those races but cannot
remove them.

The FAQ's "What is not protected?" entry now states both, in the same words the
spec uses. It had already said the other two - keys are copies rather than
property, and the registry is not a price control on trades made elsewhere.

`SPEC.md` 12's assumption A-2 is also settled in the same pass: the reasons the
single-file bundle is acceptable for review are the ones the checks assert - no
remote script, style or font, no remote request of its own, markup in imported
data neutralised, nothing kept in browser storage, an unreadable gateway refused
rather than rendered - with the note that whether a reviewer accepts the design is
their judgement and not something this record can settle.

| Check | Result |
|---|---|
| the FAQ | typecheck clean; the built file grew from 268471 to 268790 bytes; the browser run passes |
| the spec | A-2's line now names the evidence instead of asking for a review that has not happened |
| the record | the manifest is regenerated for the rebuilt interface |

### Handoff - the Plan B route, and the CID check only a node can make (2026-09-26)

`SPEC.md` 5.3 permits an indexer as a *source* for a caller who cannot read a
node's database, on the condition that what it supplies stays checkable by
someone who does run a node, and that an indexer-only result is never recorded as
verified. Both halves now exist as commands rather than as a paragraph.

The Plan B route is `fod inspect -indexer URL -inventory-epoch N -author ADDR`.
It builds the listing from the explorer's per-identity flip records, and for each
CID it reads the explorer's own flip record, which names the transaction that
carried the submission, the block, and the height. Every one of those anchors is
then checked against the node the caller named: the block hash against the
node's header at that height, the transaction against the node's own copy of it
(type, sender, epoch, and the CID in its payload), and the epoch's state root
against the header at the block the root belongs to. A contradiction refuses the
listing; a node that does not answer leaves it unconfirmed, and `-require-node`
turns that into a refusal. The artifact carries the source, the root with its
height, and whether a node confirmed each anchor.

The blob check needed the second half of a discovery this session made: a flip
over one IPFS chunk is submitted with a UnixFS/dag-pb CID, which no local sha256
can reproduce. `fod` now asks the node's own IPFS for a blob's CID (`ipfs_cid`)
and decides raw CIDs locally; a dag-pb CID that nobody imported is recorded
unverified rather than accepted. The two parts of the release that still assume
the raw form - `isFlipCID` in the manifest path and the registration guest's
`blob_cid_multibase` - are recorded as `SPEC.md` O-12 with the affected
population, and nothing was worked around to hide them.

| Check | Result |
|---|---|
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.0 s; `go vet .` clean |
| the split | epoch 228's flip list: 34 flips, 26 at or under 262,144 bytes with raw CIDs, 8 larger with dag-pb CIDs, 0 contradicting the boundary |
| the node's own answer | `ipfs_cid` over the 307,089 bytes a live node serves for `bafybeid2…` returns that CID; the sha256 of the same bytes is not its multihash |
| the Plan B route, with a node | `fod inspect -indexer https://api.idena.io/api -inventory-epoch 228 -author 0xace93b… -rpc <node> -key <key>`: 5 submissions, 5 blobs verified, 0 failed, `complete=false node_verified=true`, "a node confirmed all 5 anchors and the epoch's state root", 14.3 s |
| the Plan B route, without one | the same command with no node: 0 blobs verified, 5 failed, `node_verified=false`, and the note names the block the node did not answer for |
| a listing as a manifest | `prepare-offer` refuses an indexer-sourced inventory, naming either the missing node confirmation or the missing committed-body sweep |

### Handoff - a flip's dag-pb cid becomes checkable without a node, and the capture window is now known (2026-09-26)

Package 2's missing input is still the epoch-228 lottery seed and candidate list,
but two things around it moved.

First, the blob check no longer needs a node. `SOURCES.md` F-86 left a flip over
one chunk undecidable by a caller who has none; the layout is now reproduced in
`tools/fod/unixfs.go` from the pinned importer's own path: a file up to 262,144
bytes is one raw block, a larger one is a dag-pb root over raw leaves carrying
the UnixFS file data, whose links come before the data and carry a present but
empty name. That is checked against the node, not against a second copy of the
same code: a live node's `ipfs_cid` returns the same CID for blobs of five sizes
(262,144; 262,145; 524,288; 524,289; 614,400 - the last is `MaxFlipSize`) and all
five reproduce in `unixfs_test.go`; all five real epoch-228 dag-pb flips
reproduce their recorded CIDs from their fetched bytes. `verifyBlobCID` uses the
rebuild first and asks the node only when it does not match, which is the case
where another client imported the file.

Second, the capture needs a window rather than a moment, and the window is the
epoch itself: `completeEpoch()` clears the previous epoch's store when the node
processes the epoch switch (`core/ceremony/ceremony.go`, `edb.Clear()`), so
epoch 228's records are gone from a live node once it crosses into 229. Within
228 they are stable, and the node that will hold them is identified and
verified:

| Check | Result |
|---|---|
| the node | the console service unit: enabled, `Restart=always` (5 s), active since 2026-09-25T06:45:37Z with 0 restarts, and its command line carries no `--fast`, so it processes each block rather than fast-syncing |
| it is following the chain | head 11,356,834 at 00:55:43Z, 11,356,836 39 s later (2 blocks, ~19.5 s/block); `bcn_syncing` reports `syncing: false`, `currentBlock == highestBlock` |
| when its lottery block fires | `dna_epoch` gives `nextValidation 2026-09-29T15:00:00Z`, and `config/validation.go` sets `FlipLotteryDuration = 5 * time.Minute`, so the flag lands on the first block after 2026-09-29T14:55:00Z - around height 11,372,700 at the measured rate |
| the capture is affordable | `idenachain.db` is 3.2 GB, `/` has 14 GB free, and the recorded recipe is a real `cp -R` (a hardlink copy is not readable, F-83's note); `/tmp` is a 3.8 GB tmpfs, so the copy belongs on `/` |
| the answers may not need the store at all | a long answer travels in the clear in its transaction: `SubmitLongAnswers` builds `attachments.CreateLongAnswerAttachment(answers.Bytes(), proof, salt, key)` and the ceremony reads it back with `qualification.addAnswers(false, sender, tx.Payload)`, so a sweep of the epoch's validation blocks carries the answers an eligibility reconstruction needs - the store is then needed only for the seed and the candidate list, which is what no other route supplies |

Next action for Package 2 is unchanged in substance and now dated: capture the
epoch-228 store from that node after its lottery block and before it crosses into
229, run the assignment and the join against the capture, and take the answers
from the epoch's own transactions where the sweep has them.

| Check | Result |
|---|---|
| the layout | `GOFLAGS=-modfile=go.local.mod go test -count=1 -run UnixFS ./...` in `tools/fod`: `ok` |
| the real data | a temporary check over the five saved epoch-228 blobs: "reproduced 5 real dag-pb flip cids from their bytes" |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.1 s; `go vet .` clean |

The answers now have their second route in code. `fod sync` records every
long-answer transaction it sweeps, payload included (`SweepResult.Answers`), and
`fod eligibility -sweep FILE` merges those with whatever the capture holds:
agreement is recorded, a disagreement refuses, and either source alone is enough
to reconstruct - which is what makes a capture taken after the epoch switch
usable. `fod prepare-offer` takes the same flag for the same reason, because its
own precondition is that the epoch has ended.

| Check | Result |
|---|---|
| a live sweep carries them | `fod sync -from 10847046 -to 10847070 -epoch 204 -body-source ipfs`: 25 blocks, 23 bodies CID-verified, 127 transactions, 89 long answers recorded with author, height and transaction hash |
| they parse back | a temporary check read `/tmp/fod-sweep-longanswers.json`: "read 89 real long answers from the epoch's transactions" |
| the merge | `answers_source_test.go`: both sources are taken, the chain's answers alone are enough, and a disagreement is refused |

### Handoff - the proof program takes either CID a flip can carry (2026-09-26)

O-12 asked which side moves when a flip is larger than one IPFS chunk: the
commitment, or the product. The proof program moved, and the reason is concrete
rather than aesthetic - of epoch 228's ten flip authors, two carry a dag-pb CID,
one of them is the only identity this project controls, and its whole bundle is
dag-pb. Refusing would have made that bundle, and the exercise that needs it,
impossible.

What changed is the rule the guest applies: `blob_cid_multibase` now returns the
CID the importer would have produced - the raw leaf while a blob fits in a chunk,
the dag-pb root over raw leaves once it does not - and refuses a blob larger than
a flip can be. The rule is the same one `tools/fod/unixfs.go` applies, and both
are checked against the node's own answers rather than against each other:
`proofs/registration/core/tests/unixfs_v1.rs` reproduces the five recorded sizes.

Because the guest changed, its image id changed with it, and so did the constant
the escrow pins. That is why the artefact was re-proved rather than re-used, and
why this was the cheapest moment to settle the question: nothing is deployed yet.

| Check | Result |
|---|---|
| the core | `cargo test -p fod-registration-core`: 8 tests pass, including the five node-verified sizes |
| the receipt | `cargo run --release --bin registration-host -- --prove`: 933.8 s, 7,602,176 cycles in 8 segments, the same cycle count as before the change |
| the identifiers | registration image id `046c8ff8…`, claim digest `e46cefab…`; `contracts/escrow/src/verifier.rs` pins the first, and `release/build-manifest.mjs` refuses to write a manifest when the two drift |
| the contract | `cargo test --release` in `contracts/escrow`: 4 tests pass; the module rebuilds to 357,695 bytes with zero gatekeeper-forbidden operators |
| the runtime | `spikes/harness` `go test ./...`: `ok`; `TestEscrowRegistersABundleFromASealedReceipt` accepts the new receipt and `TestEscrowAcceptsCliPreparedRegistration` measures 195,304,165 wasm gas, unchanged from the previous program |
| the CLI guard | `assembleRegistrationInput` now checks each blob against its CID with the importer's rule before a proof starts, so a bundle that would fail the guest fails in a second instead of after fifteen minutes |

The local prepared artefacts under `output/cli` were regenerated against the new
module and receipt (`prepare-deploy`, `prepare-registration`); they are not part
of the repository.

### Handoff - O-10 closed: the module runs on the runtime mainnet executes (2026-09-26)

The last measurement the mainnet claim was missing is this one: whether the
runtime this project measured is the runtime the network runs. It is not the same
artifact, and now the difference is quantified.

What mainnet installs is the released `idena-go` v1.1.2, and its binary records
the binding it links: `idena-wasm-binding v0.0.0-20230503080211-4227b9778d3d`,
whose static libraries were built from upstream `idena-wasm` at `9fc402e` - the
repository's last commit, 2023-05-03. This project measured a fork of that
runtime, `ubiubi18/idena-wasm` `7e113895`, 29 commits further on (Wasmer
hardening, CI, dependency locks, and changes in the export, import, memory, and
environment layers).

Nothing that decides cost or admissibility moved: `src/costs.rs` is byte-equal
apart from whitespace, and outside its own tests `src/gatekeeper.rs` differs only
in an import order and comment indentation. The runtime is what executes the
module, so the check that matters is behavioural, and it was run: the released
binding is fetchable from the module proxy, so the harness was pointed at it in a
`linux/amd64` container and the whole suite passes.

| Check | Result |
|---|---|
| the released runtime | `docker run --platform linux/amd64 -e 'GOFLAGS=-modfile=go.official.mod -mod=mod' golang:1.26.5 go test -count=1 ./...` in `spikes/harness`: `ok`, 9.0 s |
| the cost there | exactly the same as under the pin: the registration call is metered at 195,478,684 wasm gas under both runtimes, 38.18% of the 512,000,000 ceiling |
| the figure that moved | 195,304,165 was the *previous* module and receipt, not the pin: the O-12 change replaced the pinned image id and re-proved the seal together, and that artefact change accounts for the +174,519 gas, which is why the two runtimes were measured on the same pair |
| the code | `cost_function` still returns 1 per operator and the base costs are 3,000,000 deploy, 100,000 call, 10,000 bytes-to-hex; the gatekeeper's rules are unchanged outside its tests |
| the consequence | the module's behaviour and its metered cost are the same on the runtime mainnet will execute, so the recorded fee figures need no correction from this item; the 188.33 iDNA registration estimate stands as measured, and `fod fees` recomputes it from the chain's rate at run time |

`spikes/harness/go.official.mod` and its sum file are committed beside
`go.local.mod`, so the claim can be re-run rather than believed; the local
development modfile keeps the fork pin.

### Handoff - the seller's CLI refuses a receipt it could not register (2026-09-26)

Package 1's follow-up was to carry claim binding into the tool that consumes
receipts. Until now `fod prepare-registration` checked the journal's labels and
shapes and left the program identity to the contract: a receipt from another
guest program, or one whose journal had been altered since it was proved, became
a signed transaction that a node would answer with `invalid_proof` - after a fee
was spent to learn it.

The construction the escrow verifies is now reproduced in the CLI
(`tools/fod/receipt_claim.go`, RISC Zero's `ReceiptClaim` for a successful run,
the same `tagged_struct` chain `contracts/escrow/src/verifier.rs` computes), and
the assembler checks three things before it writes the call: the artefact's image
id is the program this release pins, the artefact records a claim digest at all,
and that digest is the one this journal and the pinned program produce.

The reproduction is pinned from both ends rather than by reading it twice: the
Go test recomputes the digest of the escrow's own frozen delivery fixture
(`verifier.rs`, including the tampered-journal case), and of the sealed
registration artefact this release ships. `release/build-manifest.mjs` now reads
the CLI's constant as a third pinned copy, so the contract, the CLI, and the
proved artefact cannot drift apart without the manifest refusing to build.

| Check | Result |
|---|---|
| the digest | `go test -count=1 -run ClaimDigest ./...` in `tools/fod`: both fixtures reproduce, and a tampered journal moves the digest |
| the refusals | `TestAssembleRegistrationRefusesBadReceipts` gained a foreign program id, a journal altered after the proof, and a receipt with no image id - all refused before assembly |
| the manifest | `node release/build-manifest.mjs` reports `pinned agree with the proved artefacts` with the CLI constant included |
| the suites | `tools/fod` `ok`, 9.1 s, `go vet` clean; `spikes/harness` `ok`, 4.3 s - the prepared call the harness submits is byte-identical, because the new checks refuse rather than rewrite |

### Handoff - the sweep's own epoch filter was reachable again (2026-09-26)

Found while writing the exercise's sweep step into the release README: the
friendly `-epoch` alias, which the inventory and eligibility modes want, was
applied to *every* command before the flag parser saw it. `fod sync` has an
epoch filter of its own under that name (`-epoch`, `-1` keeps all), so the alias
silently reshaped it into `-inventory-epoch`, a flag `sync` ignores - the filter
could not be set from the command line at all, and a sweep of one epoch kept
every epoch's answers with it.

The alias is now mode-aware: the modes that work on one epoch keep it, and
`sync` keeps its own flag.

| Check | Result |
|---|---|
| before | `fod sync -from 10847046 -to 10847046 -epoch 1 ...` against a block from epoch 204: `txs 3`, `answers 1` in the artefact - the filter did nothing |
| after | the same command: `txs []`, `answers []` |
| the aliases | `commands_test.go` pins both directions, including the `-epoch=204` form |
| the suite | `tools/fod` `ok`, 9.0 s, `go vet .` clean |

### Handoff - the fee table describes the module that ships (2026-09-26)

The O-12 change replaced the guest program id and re-proved the sealed receipt,
and the fee table in the CLI did not follow it. Walking the release's fee story
for this audit showed why it had to: a proof-carrying call's gas follows the
module and the proof it carries - the three delivery settlements on one module
cost three different amounts - and the new registration pair costs 195,478,684
wasm gas where the previous one cost 195,304,165, +174,519 (+0.089%), moving the
registration estimate from 188.33 to 188.50 iDNA at the 2026-09-25 rate.

The other five operations were unchanged by that artefact change, which is why
the audit had to be per operation rather than a single multiplier: listing,
funding, settlement, refund and withdrawal measure exactly what they measured
before.

| Check | Result |
|---|---|
| the refreshed figures | `spikes/harness -v`: registerOffer 195,478,684; listBundle 106,204; fundOffer 161,165; fulfill 195,632,417; refund 110,479; withdraw 201,987 |
| the tool | `fod fees -fee-per-gas 96153846153800` prints the same six operations, 15 runs recorded, and 188.50 iDNA for the registration |
| the table | `tools/fod/fees.go` and its test carry the new number; `SOURCES.md` F-76 carries a correction and F-77's cost table the new fee |
| the docs | the release README's exercise cost is now about 379 iDNA of fees, and the O-12 entry points at the corrected sibling figure instead of claiming the cost was unchanged |

### Handoff - the shipped FAQ quotes the measurements it was built from (2026-09-26)

The same audit found the interface's Costs entry still quoting the old numbers:
"about 188 iDNA each" for the two proof-carrying calls, whose measured values
are 188.50 and 188.89 after the fee table was refreshed, and "896.5 seconds" for
the registration proof, where the re-proved receipt took 933.8 seconds. Both are
user-facing money-and-time figures, so they were corrected in
`app/src/faq.ts`, the interface was rebuilt (268,798 bytes, from 268,790), and
the browser run was repeated.

| Check | Result |
|---|---|
| the typecheck | `npm run typecheck` clean |
| the build | `npm run build`: 268,798 bytes, sha256 `f6108b8286743fa2b7cf…` |
| the browser run | `npm run verify`: ALL CHECKS PASSED, including "faq renders offline content" and the no-remote-request, no-browser-storage, and stale-policy checks |
| the record | `release/MANIFEST.json` regenerated for the new interface artefact |

### Handoff - two more documents caught up with the code (2026-09-26)

The same sweep checked the prose that introduces the packages rather than the
release table: `contracts/escrow/README.md` still said `registerOffer` returned
`registration_proof_not_implemented` and that the registration proof "arrives
with Package 4", months of work after it was implemented and verified in the
runtime; the root README still quoted 896.5 s for the registration proof, which
the re-proved receipt measures at 933.8 s (the delivery figure, about 16 minutes
and 8.9 GB, is unchanged).

Both now state what the release does, and name the evidence (`SOURCES.md` F-74,
F-89) instead of a package that has landed.

Two headers in the code itself said the same kind of thing:
`contracts/escrow/src/lib.rs` still described `registerOffer` verification as
"not implemented in this slice" and the delivery commitment as test-hook-only,
and `tools/fod/main.go` still introduced itself as a Package 2 spike whose
"qualification reproduction is not implemented yet" - which is the half of the
tool that decides eligibility. Both were rewritten to describe what the code
does; the escrow change is a comment, and the module rebuilds byte for byte
(`release/artifacts/fod_escrow.wasm` `9cb353997f…`, `dist/fod_escrow.wasm`
`cb426bb1…`, both unchanged), which is also a small reproducibility check.

One more correction came out of the same sweep, in the release README's "what
is missing": it still said the epoch records "persist in that node's epoch store
once written" and placed the window "before validation on 2026-09-29T15:00Z".
Both are now stated the way the code behaves - the lottery block is the first
block after `2026-09-29T14:55Z`, and the capture has to happen inside the epoch
because `completeEpoch` clears the store at the boundary - and the two copy
measurements in that file were unified on the node the capture uses (3.2 GB,
about 12 s).

The sweep also produced `SOURCES.md` F-91, from an experiment that was not
meant to be one: a doc-comment edit in the registration core made
`cargo test -p registration-host --test sealed_artefact` fail, and restoring the
exact text made it pass. The guest's image id follows its sources down to
comments and line positions, so the guest sources, the shipped receipt, the
contract's constant, the manifest, and the CLI's copy are one set - any edit in
that crate means a re-prove and a re-pin. The comment was reverted rather than
paying for a 16-minute proof, and the fact is recorded so the next person pays
it deliberately.

### Handoff - the seller's bundle inventory, built the way the exercise will build it (2026-09-26)

The `-bundle` input the exercise's `fod prepare-offer` needs is an inventory, and
the node route for it had only been exercised through the indexer and through
fixtures. It now runs against the epoch-228 sweep and the live node:

| Check | Result |
|---|---|
| the command | `fod inspect -sweep <epoch-228 sweep> -author <seller> -inventory-epoch 228 -rpc <node> -key <key> -blobs-out <dir> -out <inventory>` |
| the result | 5 submissions, 5 unique CIDs, 0 duplicates, 0 deletions, 5 blobs verified, 0 failed, `complete=true node_verified=true` |
| what it establishes | every flip the committed-body sweep found for that author is in the bundle, and every blob's CID was confirmed by the node's own IPFS - including the dag-pb form all five carry (F-86, F-88) |
| what it does not | the seller can still submit flips until the lottery block, so the exercised run rebuilds the inventory after that block; this entry pins the command and its evidence, not a final bundle |

### Handoff - the CLI runs where the nodekey lives (2026-09-26)

The exercise's seller steps need three things that only exist together on the
node's host: the datadir, the running node, and the seller's nodekey. The nodekey
is the one that must not move, so the CLI has to run there rather than the
artefacts coming here. It can:

| Check | Result |
|---|---|
| the build | `docker run --platform linux/amd64` with the pinned `idena-go` and `idena-wasm-binding` checkouts mounted and path-replaced in a container modfile: `CGO_ENABLED=1 go build` produces a 106,884,176-byte linux/amd64 `fod` (the module needs Go >= 1.26.8, so the `golang:1.26` image, not the older patch tag) |
| it runs on the host | `./fod-linux-amd64 help` prints the command list |
| it reads that node | `./fod-linux-amd64 fees -operation registerOffer -rpc http://127.0.0.1:9129 -key <key>` prints 195,478,684 wasm gas, 1,954,787 user gas, 559 bytes, 188.50 iDNA at the node's own rate |

The binary sits at `/root/fod-linux-amd64` on that host, which is a test machine;
it can be deleted at any time and rebuilt with the command above. With this, the
seller's nodekey never leaves the host: the capture, the inventory, the
eligibility run and `prepare-offer` can all run there, and only the registration
input (which carries the epoch's derived secrets, shared with the ceremony's
participants by design) has to reach the prover.

### Handoff - the deploy's price is in the fee table the CLI prints (2026-09-26)

The exercise's first spend is the module deploy, and the deploy is its largest
transaction by an order of magnitude: 357,858 signed bytes, so the size fee
alone is 3,578,580 gas before the deploy executes anything. The harness that
measures it against the release module already existed
(`spikes/harness/escrow_deploy_cost_test.go`: `deploy used 3000051 wasm gas
(30000 user gas) of 1541420 user gas of headroom`), but the table `fod fees`
prints listed only the contract methods, so a caller preparing the deploy had no
number for the step they pay for first. `operationDeployModule` is now the
table's first row, and the test that fails on an operation without a measured
number, provenance, a measured transaction size, or a budget under the
per-transaction cap covers it like any other.

Two numbers in this entry differ by one on purpose, and the row uses the larger
one: the harness prints `wasmGas / 100 = 30,000` as a description of the
requirement, while the max-fee budget is `ceil(W / 100) = 30,001`, because the
node derives the gas allowance by dividing the remaining budget and rounds down
(`tools/fod/fees.go`). The extra user gas costs one time the chain's rate.

| Check | Result |
|---|---|
| the row, at the 2026-09-26 rate | `./fod fees -fee-per-gas 96153846153800`: `deployModule 3000051 30001 357858 346978942307525757800 0.59% 346.98` - about 347 iDNA, 0.59% of the per-transaction cap |
| why it does not hit the cap | the cap binds the whole max fee, and this one is dominated by the byte fee: 357,858 bytes * 10 gas * the rate |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 -run 'EveryOperation\|Fees\|UserGas' ./...` in `tools/fod`: `ok` |

### Handoff - the Plan B route says what it can and cannot verify (2026-09-26)

The rule this entry ends on: a user who has no node may read a public indexer
such as `api.idena.io`, and nothing the tooling writes from one is accepted on
its word. The route already existed (`SPEC.md` 5.3,
`tools/fod/indexer_inventory.go`); this pass checked it in the code and against
the live service, corrected two notes the route prints that had gone stale
behind F-88 and F-89, and gave the route a way to be found from the interface.

| Check | Result |
|---|---|
| the guard, where it stands | an indexer listing is never `complete` (completeness rests on a committed-body sweep), carries its `source`, the epoch's root with the height it belongs to, and a per-anchor `nodeChecked`; a named node that contradicts an anchor refuses the listing, and `-require-node` turns "not checked" into the same refusal |
| it cannot become a registration input | `prepare-offer` refuses an indexer-sourced inventory by name, whether or not a node confirmed it: only `-sweep`, the committed-body route, may become a manifest (`inventoryCidsFor`) |
| the tests | `go test -run 'IndexerInventory\|AListingFromAnIndexer'` in `tools/fod`: all six pass, including a contradiction being refused, a caller who requires a node getting a refusal without one, and a dag-pb blob being decided locally by the rebuild |
| the live run, no node | `./fod inspect -indexer https://api.idena.io/api -inventory-epoch 228 -author 0xace93b… -gateway https://ipfs.io/ipfs/`: 5 submissions, 5 unique, `complete=false node_verified=false`, and the note names the block no node answered for; the five blobs did not fetch, because no node's IPFS was named and the public gateway did not serve them |
| what it is worth to a node owner | every field it writes is re-checkable: the block hash against the header at that height, the transaction against the node's own copy (type, sender, epoch, and the CID in its payload), the state root against the header at the epoch's opening block |

The two stale notes were the route's own. It still said that a dag-pb CID cannot
be decided without a node, and that such flips "cannot enter a registration
proof yet"; F-88 reproduced the importer's layout locally and F-89 moved the
guest onto either form, so the first claim is false and the second is
superseded. The notes now say what actually decides a dag-pb CID - the local
rebuild, with the node's own importer as the fallback when the rebuild does not
reproduce it - and what a missing blob really leaves unverified, and the test
that asserts the note moved with it. `SOURCES.md` F-86 keeps its dated
observation and now carries a line naming F-88 and F-89 as what supersedes its
consequence.

The route was also invisible in the interface: the FAQ's "Do I need a website,
or my own Idena node?" answered with the node route and the provider route only,
so a reader without a node could not learn that the CLI reads a public indexer
at all. It now says in one sentence that such a listing is named as
indexer-sourced, stays marked unverified, never counts as complete, and can be
checked afterwards by anyone who runs a node - and that a bundle offered for
sale still needs the node's own sweep.

| Check | Result |
|---|---|
| the interface | `npm run typecheck` clean; `npm run build` wrote 269,152 bytes (from 268,790); `node verify.mjs`: all checks pass, including the offline FAQ render |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.2 s |

### Handoff - the capture route is rehearsed on the node that has to survive it (2026-09-26)

Gate G-2's capture is the one step of the exercise the chain time-boxes, so its
procedure was rehearsed against the live node instead of assumed. Nothing usable
was captured: the epoch's lottery records do not exist yet, because the block
that writes them is the first after `2026-09-29T14:55Z`. What the run produced
is the procedure, its timings, and the negative signature the checks must not
accept as a capture.

| Check | Result |
|---|---|
| the node is still the right one | `dna_epoch`: `epoch 228` with `nextValidation 2026-09-29T15:00:00Z`; `bcn_syncing`: `syncing=false`, current == highest at 11,357,151; the store is 3.2 GB with 14 GB free on `/` |
| a live database cannot be read in place | `fod epoch-keys -datadir <live datadir>`: `open chain db read-only: resource temporarily unavailable` - the node holds the lock |
| the copy route works | `cp -R idenachain.db <scratch>/idenachain.db`: 3.2 GB, 1766 files, `real 12.582s` on the node's own disk |
| the copy is readable | `fod epoch-keys -datadir <copy>`: `epoch keys total: 33`, `epoch 228: map[cid:33]` |
| the reader's negative control | `fod epoch-db -datadir <copy> -inventory-epoch 228`: `seed_bytes=0 candidates=0 assigned_flips=0 states=map[]`, 0.206 s - the pre-lottery state a capture taken too early has |
| the assignment reader refuses rather than approximates | `fod assignment -datadir <copy> -inventory-epoch 228`: `epoch 228 has no lottery identities in <copy>: a node records them only when it is live at that epoch's FlipLotteryStarted block` |
| cleanup | the copy was deleted; `/` is back to 14 GB free |

What this buys the capture day: both readers are now known to run against a
copy of this node's database, and their pre-lottery output is recorded here, so a
capture taken after the block can be told apart from one taken before it by the
same two commands. The positive case stays unverified until then - `seed_bytes`
and `candidates` non-zero and an assignment produced, from a copy taken after
the lottery block and before the node crosses into epoch 229.

### Package 9 - the current bytes reproduce from a clean clone, and the suites are green (2026-09-26)

The clean-clone reproducibility check was last run before the O-12 change
replaced the module and before the FAQ change rebuilt the interface, so it
proved reproducibility for bytes that no longer exist. It was re-run from a
fresh clone at `edf20b5`; the clone differs from the revision the manifest names
only by `MANIFEST.json` itself.

| Artefact | Clean clone | `release/MANIFEST.json` | Result |
|---|---|---|---|
| `release/artifacts/fod_escrow.wasm` | 357695 bytes, `9cb353997f6fe394…` | 357695 bytes, `9cb353997f6fe394…` | identical |
| `app/dist/flips-on-demand.html` | 269152 bytes, `c034bb01c97d1e87…` | 269152 bytes, `c034bb01c97d1e87…` | identical |

The commands are the ones the manifest records: `cargo build --release --target
wasm32-unknown-unknown` in `contracts/escrow` (16.2 s in the clone, with the
crate cache warm), `wasm-opt --llvm-memory-copy-fill-lowering -O3` at the root
(0.5 s), then `cd app && npm install && npm run build` (0.8 s + 0.2 s). The
clone sits under the system temp directory and was not deleted, because this
session's command guard rejects removals; it holds nothing but the repository.

The same revision also carries a green regression pass over the parts today's
changes could have touched:

| Check | Result |
|---|---|
| the escrow harness | `GOFLAGS=-modfile=go.local.mod go test -count=1 -v ./...` in `spikes/harness`: 26 tests, `ok`, 4.5 s |
| the integration loop | `node release/check-integration.mjs`: all checks pass - session, provider-signed result, utility grant, replay refused, quota spent once |
| the CLI | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.2 s (recorded above) |

### Handoff - the exercise's second input was rehearsed on the node's host (2026-09-26)

The epoch's transactions - and, after validation, its long answers - are the one
input the exercise takes from the chain rather than from the capture, and the
command that collects them had only ever run from this machine. It now runs on
the host that will carry the exercise, with the CLI rebuilt there first.

| Check | Result |
|---|---|
| the staged CLI was rebuilt | the binary at `/root/fod-linux-amd64` was older than this session's changes; it was rebuilt from `e576823` for linux/amd64 in the pinned container (`golang:1.26`, the read-only `idena-go` checkout mounted, path rewritten in a container modfile, `CGO_ENABLED=1`): 106,883,808 bytes |
| it runs there | `./fod-linux-amd64 help` prints the command list, and `fees -operation deployModule -rpc http://127.0.0.1:9129` prints 3,000,051 wasm gas, 30,001 user gas, 357,858 bytes, 346.98 iDNA at the node's own rate - the row this session added, on the platform the exercise uses |
| the sweep runs there, epoch 228 | `sync -from 11351740 -to 11351760 -epoch 228 -body-source ipfs`: 21 blocks, 6 non-empty bodies all CID-verified, 7 `submitFlip` transactions reconstructed, 0 transaction-set mismatches, 0.14 s |
| the sweep runs there, epoch 204 | the same command over 10847046-10847070: 25 blocks, 23 bodies CID-verified, 127 transactions, 0 mismatches, and 89 long answers recorded with their payloads - the input `eligibility -sweep` merges - in 0.26 s |
| the indexer is reachable from there | `https://api.idena.io/api/Block/Last` answers 200 in 0.21 s, which the eligibility cross-check needs |

The rehearsal JSONs stay on the test host (`/root/fod-sweep-rehearsal3.json`,
`/root/fod-sweep-204.json`), beside the rebuilt binary. With this, every input of
the exercise except the lottery records themselves has been produced once from
the host, and the lottery records are what the 2026-09-29 window is for.

### Package 9 - pre-flight: what the exercise costs and what the account can pay (2026-09-26)

The approval the mainnet exercise waits for needs two numbers together: what the
exercise's calls cost, and whether the account that would send them can pay. Both
were read today rather than carried over.

| Check | Result |
|---|---|
| the rate | `bcn_feePerGas` on the node that will run the exercise: `96153846153800` base units per gas (0.0000961538461538 iDNA), the same rate the 2026-09-25 measurements used |
| the per-call prices | `fod fees -fee-per-gas 96153846153800`: deploy 346.98 iDNA, registerOffer 188.50, listBundle 0.26, fundOffer 0.41, fulfill 188.89, refund 0.22, withdraw 0.31 |
| the exercise's nine calls | deploy, register, list, fund, fulfill, withdraw, fund, refund, withdraw = about 726 iDNA of fees |
| what the record claimed | `release/README.md` said "about 379 iDNA of fees for the whole exercise" and called the two proof-carrying calls the only significant cost; the deploy row was priced the same day and the sentence had not caught up. Corrected in the same commit as this entry |
| the paying account | `dna_getBalance` for the exercise identity: balance 29.78 iDNA, stake 86,233 iDNA, nonce 16, mempool nonce 16 |
| what that means | fees come from the balance, not the stake, so the exercise cannot start until that account holds roughly 726 iDNA or has stake unlocked; the README now states it beside the approval text |
| no deploy stake is needed | `validateDeployContractTx` applies the 3,000,000-gas minimum only to the node's `embedded.AvailableContracts`, and a custom module is not one, so `tx.Amount` may stay zero (`SOURCES.md` F-92) |
| the nonce to expect | 16, with the mempool at 16: the deploy's account nonce is read from the chain, and the CLI refuses a mismatch rather than guessing |

The FAQ's cost answer quoted only the two proof-carrying calls; it now names the
one-off deploy and the balance rule, and the interface is rebuilt with it.

| Check | Result |
|---|---|
| the interface | `npm run typecheck` clean, `npm run build`, `node verify.mjs`: all checks pass |
| the record | `release/README.md`'s money paragraph, `docs/SOURCES.md` F-92 |

### Package 9 - the seller's key signs as the seller, and a broadcast that cannot run refuses (2026-09-26)

Step 1 was rehearsed on the host without broadcasting, and the rehearsal caught
a money bug in the table it was rehearsing against.

| Check | Result |
|---|---|
| the host's node is the seller | `dna_getCoinbaseAddr`: `0xace93bd0…`, the exercise identity, so the nodekey at `<datadir>/keystore/nodekey` signs as the seller and the seller steps can stay on that host |
| the CLI reads that nodekey | `fod prepare-deploy` on the host against the release artefact: `sender ace93bd0…`, an address derived, `raw tx 357860 bytes`, nothing broadcast |
| the table's size was stale | the deploy row said 357,858 bytes, the size of the *previous* module's transaction; the O-12 change replaced the artefact and the row had not followed. It reads 357,860 now |
| why two bytes matter | the requirement is `sizeFee + ceil(W/100) * rate`, so a two-byte-short figure leaves 20 gas (about 0.002 iDNA) of the call unbudgeted, and the chain charges for a call that runs out of gas: the deploy would have cost its whole fee and deployed nothing |
| the guard | `fod submit` computes the requirement from the transaction it actually signed - RLP encodes the nonce, epoch, amount and the max fee, so the signed size is the one that counts - prints it, and refuses to broadcast when the signed max fee cannot cover it rather than sending a transaction that cannot run. `TestAMaxFeeForTheMeasuredSizeIsShortOfTheSignedOne` pins the two-byte case at the mainnet rate |
| the tests | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.2 s; `TestReleaseModuleFitsTheDeployGasHeadroom` passes with the corrected size, headroom 1,541,400 user gas (twenty less, the two extra bytes) |

The linux/amd64 CLI on the test host was then rebuilt from this revision, so the
day's commands carry the guard rather than the table they were rehearsed
against, and the corrected figure was checked against the transaction it signs:

| Check | Result |
|---|---|
| the build | 106,884,168 bytes, the same container recipe (`golang:1.26`, the read-only `idena-go` mount, a path-rewritten modfile, `CGO_ENABLED=1`) |
| the table there | `fod fees -operation deployModule -fee-per-gas 96153846153800`: 3,000,051 wasm gas, 30,001 user gas, 357,860 bytes, 346.98 iDNA - the corrected row |
| the signed transaction matches | `prepare-deploy` with the printed max fee `346980865384448833800`: `raw tx 357860 bytes`, `sender ace93bd0…`, an address derived, nothing broadcast |
| why that is the point | the requirement is computed from the signed size and the signed size is the one the row names, so the figure `fod fees` prints covers the transaction the CLI signs; the two-byte gap that would have cost the deploy's whole fee is closed |
| what stays unexercised | the broadcast itself. The guard's refusal path is covered by its test; its acceptance path needs a real send, which stays behind the approval |

### Package 2 - the store reader's positive control, and where a lab chain stops (2026-09-26)

The capture rehearsal earlier today proved the readers refuse a capture that
lacks the lottery records. The lab chains still hold records of their own, so the
other half was checked against three of them (read-only, no node running):

| Store | `epoch-db` | `assignment` |
|---|---|---|
| `lab-chain-testkey`, epoch 1 | `seed_bytes=32 candidates=1 assigned_flips=0 states=map[Invite:1]` | refused: `epoch 1 predates this release's rules ...` |
| `lab-chain-two-party`, epoch 1 | `seed_bytes=32 candidates=2 assigned_flips=0 states=map[Invite:1 state(0):1]` | refused, the same message |
| `lab-chain-probe`, epoch 0 | a key census of the store, no epoch record for 0 | refused, the same message |

So a real 32-byte lottery seed and a real candidate list do come out of a node's
epoch store - the reader's positive control - and the assignment reader refuses
these chains by design, because their epochs sit below the 104 boundary this
release pins (`2d1a3e4`). That amends an earlier entry today: its claim that
`assignment` derived `candidates=2 shards=1 authors=0` from the lab capture
predates the boundary check. Today the same store is refused, and the
derivation's controls are the frozen lottery vector
(`TestAssignmentFromACapturedCandidateList`) and the first in-range capture,
which is the mainnet one. The practical consequence for the window: run
`epoch-db` first and read `seed_bytes` and `candidates` before anything is built
on the capture, which is what `release/README.md` step 0 says.

A lab chain that could have closed the gap by reaching the range was tried and
does not work. A fresh chain (network 33406, 30-second validation interval,
5-second ceremony windows, one validator) plays its first ceremony through
`FlipLottery` and `LongSession`, then stops with `ERROR short session answers
are missing` and never advances the epoch, so 104 validations are not a route on
a lab. The attempt left a directory, `lab-chain-range` beside the other lab
chains, which can be deleted.

### Package 9 - the deploy broadcast gets the same guard as the calls (2026-09-26)

`fod submit` refuses to broadcast a prepared call whose max fee cannot run it. The
deploy had no such check, and the deploy is the transaction the exercise pays
most for (346.98 iDNA) and the one the lab refused twice on fee grounds. It now
reads the chain's rate the way the call path does (unless `-fee-per-gas` is
given), prints the requirement computed from the bytes it signed, refuses to
broadcast below it, warns above the node's cap, and runs the same account check
the call path runs - a wrong nonce or a stale epoch is refused before the send
rather than wedging the account, which is what the lab chain's wedged sender came
from.

The path was exercised against a stub node on `127.0.0.1` that answers
`bcn_feePerGas`, `dna_getBalance`, and `dna_epoch` and records any
`bcn_sendRawTx` instead of reaching a chain, so both outcomes could be seen
without a mainnet send:

| Run | Result |
|---|---|
| the measured-size figure, `346978942307525757800` | `estimated 346980865384448833800 base units at 96153846153800 per gas for the 357860 bytes signed`, then `refused before broadcasting: this signed deploy needs a max fee of ... and it carries ...`; the stub recorded no send |
| the corrected figure, `346980865384448833800` | the same estimate, no refusal, and the stub's own refusal of `bcn_sendRawTx` - the only thing that stopped the send was the stub |

The guarded binary is staged on the test host: 106,885,824 bytes, SHA-256
`6cf452adb65e7ead51384c1c592aee2a5e5d37fc6b25c1371165aa60f41250ec`, the same
digest as this machine's build, and a dry run there still signs exactly 357,860
bytes from the exercise identity. The flag help text was corrected after that
build, so the host was given one more build from the corrected revision:
106,885,848 bytes, SHA-256
`6a7f94fd360e04884b3be059d48d34c0d9c860812b3d54b917bbc4ebf06ecc21`, which is
what the host carries now and what this machine's build produces.

### Package 9 - the exercise's steps are commands, and three files that were not written (2026-09-26)

The release record's exercise section was prose from step 1 on: a reader could see
what had to happen and not how to run it, and the listing step was missing from
the sequence altogether although nothing can be funded before it. Writing the
steps out against the CLI's own required-flag lists also caught three places
where a flag named a file the tool never wrote.

| Fix | Detail |
|---|---|
| the runbook | steps 1-8 carry the commands: deploy (`prepare-deploy -signed deploy.json -broadcast`), register (`prepare-registration -prepared ...` then `submit`), list, fund, deliver, withdraw, recover, refund - each with the nonce, the epoch, and the figure `fod fees` prints for that operation. Listing now precedes funding |
| `prepare-deploy` honoured no `-out` | it takes `-signed` only, so a dry run that named `-out` wrote nothing at all; the three files that were never written on the test host are the evidence. An explicit `-out` is honoured now when `-signed` is absent |
| the prepare modes named a file they had not written | `prepare-list`, `prepare-purchase`, `prepare-registration`, `refund`, and `withdraw` printed "pass this file to `fod submit`" whether or not `-prepared` was given. They now name the flag that writes it when it was not |
| `epoch-keys` wrote `sweep.json` | step 0's command passes no `-out`, so a read-only key listing landed in a file named like a sweep - the shape `-sweep` reads. That mode defaults to `epoch-keys.json` now |

| Check | Result |
|---|---|
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 9.1 s |
| the three files | `refund` without `-prepared` prints `not submitted: pass -prepared FILE to write this call, then pass that file to fod submit`; `epoch-keys` wrote `epoch-keys.json` (202 bytes) and no `sweep.json`; `prepare-deploy -out /tmp/deploy-out-alias.json` wrote the 716,341-byte signed deploy |

The host then carried the build with these fixes: 106,884,960 bytes, SHA-256
`30afbdde08d94309836396f4eb0b8cb7fd63dcf3b87966ca7f54874a1d06664e`, the same
digest as this machine's build.

### Package 9 - the current bytes reproduce, and a blank answer gets one retry (2026-09-26)

The interface changed after the last reproducibility run, so the checks that
depend on the current revision were redone rather than carried over, and the
capture node was asked how it is.

| Check | Result |
|---|---|
| the clean clone | a fresh clone at `16c2c88` (the manifest names `61f37f7`, which differs by `MANIFEST.json` alone) rebuilds both artefacts byte for byte: `release/artifacts/fod_escrow.wasm` 357,695 bytes `9cb353997f6fe394…`, `app/dist/flips-on-demand.html` 269,410 bytes `32c85e84db7d60f5…` |
| the suites | `spikes/harness` `ok` (7.5 s), `release/check-integration.mjs` all checks pass, `tools/fod` `ok` (9.2 s) |
| the capture node | epoch 228, `nextValidation 2026-09-29T15:00:00Z`, `syncing=false` at block 11,357,309; the host has been up 36 days |
| the transient worth handling | one `dna_epoch` request returned an empty body and the next answered; the node shares its host with the compatibility lab, so its handlers recover per request under load (`SOURCES.md`) |
| the retry | `tools/fod/rpc.go` now retries a failed RPC once when the failure is not an answer the node gave, and never retries `bcn_sendRawTx` or a `dna_send*`. For a sweep of thousands of blocks, one blank answer used to end the run. `TestABlankAnswerIsRetriedOnceAndASendIsNot` pins both halves against a stub node |

The test host carries the build with the retry: 106,894,864 bytes, SHA-256
`dbaaf3f1d0f15d9a20c9e5a9d33f97031a66d22754fb46aff5db316199a99ec6`, the same
digest as this machine's build, and it reads the live node's rate there.

### Package 9 - the staged CLI sweeps, and the epoch's sweep is half a minute (2026-09-26)

The staged build was exercised where the day's work happens rather than only
asked for a rate, and the one step whose scale was unknown was measured.

| Check | Result |
|---|---|
| the staged binary sweeps | `/root/fod-linux-amd64 sync` over epoch 228's 11351740-11351760: 21 blocks, 6 bodies CID-verified, 7 `submitFlip`, 0 mismatches; over epoch 204's 10847046-10847070: 25 blocks, 23 bodies, 127 transactions, and the 89 long answers recorded with their payloads |
| the scale | a 2,000-block window of epoch 228 (11350781-11352780) swept in **2.667 s**: 102 bodies CID-verified, 113 transactions, 0 mismatches, a 754,070-byte sweep file |
| what that means for the day | epoch 228 is about 21,900 blocks, so its full sweep is roughly half a minute on the capture host - the answers step is not a bottleneck after validation |
| the range | `dna_epoch` answers the epoch's `startBlock`; the next epoch's `startBlock` minus one is the last block to sweep. `release/README.md` step 0 now says so |

### Package 3/5 - the buyer can open a delivery without the author's key (2026-09-26)

`SPEC.md` 6.5 ends with "retrieve the recorded delivery" and "verify its proof and
decrypt locally", and 6.1 describes `receive` as "verify, decrypt, display
purchased flips locally" - but nothing user-facing did the first half. The escrow
publishes the ciphertext inside `fulfill` (`delivery/<id>`), and the only
decryption of a delivery lived in `delivery_test.go`. A buyer who is not also the
author held the delivery key and the ciphertext and had no way to turn them into
the epoch keys `fod receive` needs, because a vault could be written only from
the author's nodekey (`export-keys`). The gap was hidden by the exercise's one
identity playing both sides.

`fod receive -offer <id> -contract <registry> -delivery-key <key> -vault <file>`
is the buyer's half now:

| Step | What it does |
|---|---|
| the order | reads `offer/<id>`, refuses anything not settled, and refuses a delivery key other than the one the order recorded |
| the ciphertext | reads `delivery/<id>` and decrypts it with the node's own ECIES library - the one the Rust guest's encryption is checked against (`proofs/vectors/ecies-v1.json`) |
| the secrets | rebuilds the two epoch key pairs from the delivered 106 bytes and refuses a length or domain that is not the guest's |
| the commitment | hashes the recovered secrets and refuses unless they match the key commitment the registered bundle carries |
| the vault | writes the keys as the same age-encrypted vault `export-keys` writes, so the blob path opens with them |
| its own boundary | says that the Groth16 seal behind the ciphertext was verified by the registry inside `fulfill` and that this CLI carries no independent verifier yet (`release/README.md`, "what is known to be missing") |

| Check | Result |
|---|---|
| the round trip | `TestOpenDeliveryRebuildsTheKeysAndOpensABlob`: a delivery built by the real constructions (the guest's secrets layout, ECIES to the buyer's key, the escrow's offer layout) opens, and the vault it writes opens a blob the author encrypted - no author key involved |
| the real artefact | `TestOpenDeliveryParsesTheRealArtefact`: the Rust-produced `proofs/delivery/dist/delivery.json`, with its journal's offer rebuilt, opens to the epoch and author its secrets carry and hashes to the journal's commitment |
| the refusals | `TestOpenDeliveryRefusesWhatItCannotTieToTheBundle`: another key's delivery is refused as `not for this key`, a mismatched commitment as `refusing to keep them`, and neither writes a vault |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 10.4 s; `go vet .` clean |

The other half of the same gap was the key itself: `SPEC.md` 6.1 gives
`prepare-purchase` the job of creating the buyer's dedicated delivery key, and
nothing created one - the file had to be produced by hand. A missing
`-delivery-key` file is a new owner-only key now, an existing one is never
overwritten, and the command prints the public half it will bind
(`TestCreateDeliveryKeyWritesOnceAndNeverOverwrites`). One piece of `SPEC.md` 6.5
is still missing and is listed in `release/README.md`: step 5's encrypted
recovery archive for that key, because the vault kinds this CLI writes hold
epoch keys rather than delivery keys.

One precondition for the exercise's buyer role was open until now: whether the
buyer needs identity status. It does not. `validateCallContractTx` checks only
the recipient and the payload, and the shared path checks the sender's nonce,
epoch, and balance (`blockchain/validation/validation.go`), so a fresh address
funded with the price and its fees can fund an order - which lets the buyer side
run on the buyer's own machine and keeps the delivery key away from the shared
host, as `SPEC.md` 6.6 requires.

The same pass closed the last piece of `SPEC.md` 6.5: step 5's encrypted
recovery archive for the delivery key. `prepare-purchase
-delivery-key-archive FILE` writes an age vault of the same key under a
passphrase of the buyer's own, and every `-delivery-key` argument accepts that
archive instead of the plain hex file - the loader asks for the passphrase only
when the file is not hex, so the plain path stays silent for scripted use, and a
vault holding anything but a delivery key is refused by name.

| Check | Result |
|---|---|
| the round trip | `TestDeliveryKeyArchiveRoundTrip`: the archive is 0600, holds the same public key, opens under its passphrase, refuses a wrong one, loads through the same `loadDeliveryKey` the buyer commands use, and an epoch-key vault is refused as `not a delivery key` |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 13.0 s; `go vet .` clean |

### Package 9 - the release candidate reproduces at the current revision (2026-09-26)

The buyer-side work changed the CLI and the interface again, so the checks that
depend on the current revision were redone.

| Check | Result |
|---|---|
| the clean clone | a fresh clone at `3e049fb` rebuilds both artefacts byte for byte against `release/MANIFEST.json`: the module 357,695 bytes `9cb353997f6fe394…`, the interface 269,747 bytes `fad4bd123582dbd0…` |
| the suites | `spikes/harness` `ok` (8.5 s), `release/check-integration.mjs` all checks pass, `tools/fod` `ok` (13.0 s) |
| the day's positive signal | `release/README.md` step 0 now names what a good capture looks like - `li`/`ls` in the key census and `seed_bytes=32` with one candidate per lottery identity - next to the too-early signature this record measured, so the operator has both shapes to compare |

### Package 4/6 - the buyer can verify a delivery's seal without the registry's word (2026-09-26)

`SPEC.md` 6.5 ends at "verify its proof and decrypt locally", and until now the
first half rested on the registry: `fod receive -offer` checked the decryption,
the key binding, the settled order, and the commitment, while the Groth16 seal
behind the ciphertext was whatever the escrow's `fulfill` accepted. The
verification itself is small when the receipt is at hand, so it is no longer
missing.

| Piece | What it does |
|---|---|
| `proofs/delivery/host --verify [PATH]` | reads an artefact, rebuilds the receipt from its journal and seal against the guest id this build pins, verifies the Groth16 seal natively, and prints the image id, the journal's SHA-256, and the claim digest. It refuses a journal or a seal that was touched |
| `fod receive -offer ... -receipt FILE` | checks a published receipt's journal against the registry's own record: contract, offer, order nonce, seller, buyer, delivery key, and the key commitment the bundle carries, plus that its ciphertext hash is the hash of the ciphertext the registry published, and that its ciphertext field, when present, is that ciphertext |
| `fod receive -offer ... -verifier PATH` | runs the verifier on that receipt, refuses the run when it refuses the seal, and records its first line among the checks |
| what the tool says it did not do | without `-receipt` it says the seal is still the registry's verification; with `-receipt` but no `-verifier` it says the journal was checked and the seal was not |

| Check | Result |
|---|---|
| the native verification | `delivery-host --verify dist/delivery.json`: `verified 256 bytes of seal over 209 bytes of journal` in 0.2 s, image id `2a4f2f7b…e4`, journal SHA-256 `a6feac36…`; the shipped receipt verifies from stored bytes, which is what a buyer has |
| the refusals | a journal with one byte flipped: `the seal does not verify: verification indicates proof is invalid`; a seal with one byte flipped: `invalid receipt format`; both exit non-zero (`tests::verifies_the_shipped_receipt_and_refuses_tampering`) |
| the CLI's checks | `TestOpenDeliveryChecksAPublishedReceipt`: a matching receipt passes and is listed among the checks, a verifier's answer is recorded, a refusing verifier fails the run, a receipt for another commitment is refused, and a receipt carrying another ciphertext is refused |
| the suites | `cargo test --workspace --release` in `proofs/delivery`: all pass; `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 14.3 s; `go vet .` clean |

What remains on that path is fetching the receipt: the registry does not store
the settlement transaction's hash, so a buyer who does not have the seller's
artefact has to find the `fulfill` call in the epoch's blocks themselves. That is
recorded in `release/README.md` as the open piece, not worked around.

### Package 6 - the buyer fetches the receipt from the chain itself (2026-09-26)

The piece the entry above left open is closed: `fod receive -offer ... -receipt
chain` finds the settlement call without the seller's help. The order record
already names the window it happened in - the funding block and the block after
which a refund becomes available - so the tool walks it (blocks in parallel,
then the transactions they name), keeps a call to the registry whose argument
starts with the offer id and is long enough to be a sealed delivery, and refuses
two such calls or none rather than guessing.

| Check | Result |
|---|---|
| the walk | `TestFindSettlementCallWalksTheOrderWindow`: among a plain send, a fulfil for another offer, and the call itself, the right one is found with its block and calldata; an empty window is refused as `no fulfil call for offer 7 in blocks 12..13` |
| the whole path | `TestOpenDeliveryTakesTheReceiptFromTheChain`: the registry's record, the published ciphertext, and the settlement call from the window together list `the receipt came from transaction 0xcc in block 1000` and the ciphertext check |
| the seal | with `-verifier` the call's journal and seal are handed to the verifier as the artefact it expects, so the same native check runs on a receipt nobody had to hand over |
| the suite | `GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` in `tools/fod`: `ok`, 14.5 s; `go vet .` clean |

With this, `release/README.md`'s list of what is missing is down to the eligible
bundle, the mainnet exercise, and the block-production wall-clock - all three on
the exercise's side of the line.

### Package 6 - the interface fits a phone as well as a laptop (2026-09-26)

A preview pass over the built page caught a layout that only worked at desktop
width: the market table had a 620px floor, so a phone clipped the price and the
availability chip, and the two trade tables put their actions past the right
edge. Nothing about the data changed; the narrow layout did.

| Screen | What a phone shows now |
|---|---|
| market | the offer, its epoch, the price, and whether it is for sale; the author and the flip count are one tap away on the offer itself |
| my trades | each order as a short card - offer, paid, paid at, deliver by, refund from, status, and the Refund action - with every value labelled |
| flip sets you can sell | the bundle, the price field under its own label, and the Offer for sale button |

The desktop tables are unchanged, and the browser walk (`node app/verify.mjs`)
still passes end to end.

### Change of plan - the market becomes a pool (owner, 2026-09-26)

The owner asked for the flip-key market to be handled by a liquidity pool, funded
at the start with their flips and 50,000 iDNA, with nobody taking a share of the
trades. That reverses the free author price, the sold-once rule, and `SPEC.md`
5.1's "no operator, market maker, or liquidity pool in this release".

What the change can and cannot mean is recorded here before any code moves, so
the next step is a decision rather than a guess:

| Fact | Evidence and consequence |
|---|---|
| chain state is public | `fod market` reads a custom contract's storage with `contract_readData`/`contract_iterateMap`, and on this revision a custom contract cannot even be called read-only. A pool can hold iDNA and bundle commitments; it cannot hold decryption keys without publishing them |
| the contract cannot encrypt to a buyer it has not met | the delivery guest proves a ciphertext bound to the buyer's key at order time, so every sale still needs a delivery by someone who holds the keys, with the existing deadline and refund as the buyer's protection |
| the pool's rules can be admin-free | the module has no owner and cannot be terminated (`SOURCES.md` F-78); a fixed curve plus refunds needs no operator |
| the buy side can be automatic | a holder selling keys back proves a delivery bound to the depositor's published key, and the pool pays the curve price from its reserve |
| the sell side needs a live key holder | initially the depositor's machine - a delivery daemon - or any holder re-selling their own keys; if that stops, sales stop and buyers refund |
| 50,000 iDNA has to be balance | the identity holds 29.78 iDNA of balance against 86,233 iDNA of stake, and this node version has no partial unstake: only replenishing stake, and delegate/undelegate for validation pools (`blockchain/validation/validation.go`) |
| the ratio is the price | 50,000 iDNA against N flips is 50,000/N iDNA per flip at the start, and a two-way pool pays that price to anyone selling keys into it |

Not started: the `SPEC.md` revision, the pool contract, the CLI and interface
changes, and the re-scoped exercise. The delivery proof, the refund and withdraw
path, the fee guard, and the ownerless deploy stay as they are.

### Change of plan - instant settlement is required (owner, 2026-09-26)

The owner requires that a customer who buys keys can use them immediately;
waiting on a slow delivery is what makes the marketplace complicated for someone
who wants flips to secure something. The design that satisfies it separates what
was one step:

| Piece | Cost | Who can certify it |
|---|---|---|
| delivery: the buyer-bound ciphertext | milliseconds of ECIES in memory | nobody has to; the buyer checks it themselves |
| settlement: the money moving | one small call | the buyer, or the seller's Groth16 proof |

The 16-minute figure is the *proof*, not the delivery. A key holder's daemon can
publish the buyer-bound ciphertext in the next block, and the buyer's tool can
check it without any proof: the decrypted secrets must hash to the key
commitment the registry carries, which `fod receive -offer` already checks. So
the fast path is fund -> deliver -> verify -> confirm, about two blocks, with no
human in the loop and no proof at all.

What stays: the proof path as the automatic fallback when a buyer never
confirms, so a seller is never at their buyer's mercy; the refund path as the
buyer's fallback when no valid delivery arrives; and the deadline that separates
them. The escrow gains a delivered/confirmed pair of states for this, and the
pool reuses the same path in both directions - buyers confirming purchases, the
depositor confirming buy-backs from the reserve.

| Check | Result |
|---|---|
| why the buyer can check without the proof | the delivered secrets hash to the offer's key commitment, which is on chain; `receive -offer` already performs exactly that comparison before it writes a vault |
| who can settle | the buyer in one call, or the seller with the existing proof - neither needs the other's cooperation |
| what it adds in fees | two small transactions (deliver and confirm, each well under 1 iDNA at the 2026-09-26 rate) |

### Change of plan - donations and availability as a gate (owner, 2026-09-26)

Two more requirements: any Idena user may give their flip keys to the pool free
of charge so it can sell them later, and only bundles whose flips are actually
retrievable may be sold, with availability checkable in real time.

| Requirement | What is implementable, and why |
|---|---|
| a donation cannot hand the contract the keys | contract storage is public, so keys held by the contract are keys given to every reader. A donation is a *listing* whose proceeds go to the pool, with the donor (or their tool) as the deliverer - the same deliver, confirm, or prove path as any sale |
| a donor has nothing to gain, so credibility needs a mechanism | a heartbeat: the donor's tool posts a cheap liveness beacon while it is willing to deliver, and the pool treats a stale beacon as "not for sale". A deterministic on-chain rule, no bond and no arbiter: a bundle whose beacon is stale cannot be funded, so an absent donor costs a buyer nothing |
| a failed order should cost the donor something | an order that ends in a refund - the buyer's claim that nothing usable arrived, and no proof from the donor - counts a strike against the bundle: one epoch delisted, and the listing is cleared if it happens again. An honest donor defends a lazy buyer with the existing proof path, which settles the order and makes the refund impossible |
| availability is not chain data | whether some node serves a CID is network state; a contract reading it would differ between validators and break consensus. No contract can check IPFS |
| availability is checkable in real time, off chain | the CLI fetches a CID, rebuilds the node importer's DAG, and compares (`inspect`, `SOURCES.md` F-88) - the same check that the pool's own tooling runs at deposit and again before a buyer signs a funding transaction. The interface shows the last check per bundle, and anyone may refresh a "checked at" advisory: permissionless, advisory, never authoritative |
| what the pool enforces on chain | the beacon's freshness, the deadline, the refund, the strike, and the curve price. What binds a buyer: the content check their own tool runs before the funding transaction is signed |

One command serves both the bootstrap deposit and later donations: a tool that
watches the pool for funded orders on the bundles whose keys it holds, delivers
the buyer-bound ciphertext, and keeps the heartbeat current while it runs.

### Change of plan - the pool's first quote, and funding deferred (owner, 2026-09-26)

The owner settled how the pool should start and when it is funded:

| Fact | Consequence for the work |
|---|---|
| the ratio is the incentive | one sale unit against the reserve sets the first quote, and a quote set high enough draws sellers into the app for iDNA from the reserve |
| the decline is intended | every sale and every gift lowers the next quote; the floor is the price at which sellers stop bringing keys, not a value anyone sets, and supply that arrives at any price makes keys cheaper for later real buyers |
| funding happens later | the 50,000 iDNA and the bundle set are a funding profile chosen at funding time; nothing on mainnet is funded or spent now, and every rehearsal uses play money |
| nothing may leak | a buy followed by a sale of the same unit returns exactly the amount paid before any recorded spread, so no sequence of calls extracts reserve value, and the owner's iDNA stays out until they fund it deliberately |
| the three journeys must be easy | selling keys, buying keys, and starting a private session on bought keys are acceptance criteria with named screens and forbidden steps, not documentation (`SPEC.md` 4.6, 11.2 items 16 to 18) |

`docs/SPEC.md` now carries the pool model: the market rules of 5.1 (reserve,
sale units, the quote table, the beacon and strike rules), the availability
rules of 5.8, the pool interface, state machine, and accounting rules of 7.1 to
7.3, the funding-profile note of 7.5, and the new open items O-13 (bootstrap
numbers) and O-14 (harness evidence that no call sequence extracts reserve
value).

Not started: the pool contract, the CLI and interface changes, the delivery
desk command, and the re-scoped exercise. The delivery proof, the refund and
withdraw path, the fee guard, and the ownerless deploy stay as they are.

### Open proposal - list reported flips, priced by the market (owner, 2026-09-26)

The owner asked whether the proof relates a bundle to its session's consensus in
a verifiable way, and proposed that bundles containing reported flips should be
listable rather than refused, because buyers would not buy them if nobody can
publish invented flips.

| Question | Answer from the record |
|---|---|
| is the session's outcome related to the proof? | yes, the arithmetic is: the guest recomputes each covered flip's verdict from the session's assignment, answers, committee sizes, and the upgrade rules, and the journal pins the policy and consensus digests (`SOURCES.md` F-69, F-74) |
| does the seal prove the flips were really submitted? | not today. The guest's inputs are evidence, not chain data, and completeness against the chain is explicitly not established by the proof (F-74). "No fake flips" currently comes from the node-anchored checks of `SPEC.md` 5.3, which any node-running party can repeat |
| what would make a status label trustless? | chain authentication inside the guest: headers, the state root at the epoch-opening block, submission inclusion, and the assignment (O-15). The cost is unmeasured, and registration already runs 7.6 million cycles |
| what changes if that is done? | the journal can carry a bound status label (clean, contains reported, weak) instead of a binary policy refusal, and the market can price the label |
| what must not change meanwhile | the pool pays from the reserve, so a reported unit sold or given into it would buy junk or dilute the clean quote. The pool lane stays clean-only until a labelled lane has its own accounting |

Not decided: the eligibility rule is unchanged (`SPEC.md` 5.1, 5.2, 5.5). The
next step, if the owner wants it, is to pin the labelled lane, its status
binding, and its accounting.

### Hard requirement - no unit without a session binding (owner, 2026-09-26)

The owner ruled out relying on a publisher's honesty: a bundle may be sold only
if it is cryptographically bound to the flips that received consensus in a real
session. Re-publishing the same bytes stays fine, because the binding is to the
bytes' CID; inventing bytes cannot claim a CID.

Feasibility, checked against the pinned checkout the same day (`SOURCES.md`
F-93):

| Link | Verdict |
|---|---|
| the flip that was submitted | committed - the submission transaction carries the blob CID as its attachment, and the header names the block body by CID, so verifying the body against that CID is the inclusion check |
| the shard's candidates | provable - the state tree returns an identity record with a proof against the state root (`GetIdentityWithProof`) |
| the lottery seed | committed and provable - the header carries the seed and its VRF proof, which the chain re-derives |
| the answers of the session | committed - short and long answers are transaction types, and the long-answer attachment carries the answers bytes with the VRF proof |
| the per-flip verdict | already proven arithmetic in the registration guest (`qualification.rs`) |

Why this is a prerequisite rather than a hardening option: the reserve pays the
seller when a unit is added, so a bundle that is only internally consistent
would mint units from the reserve with no buyer and no delivery. No `sellUnit`
or `donateUnit` call may pay or count a unit until the registration path
verifies those anchors (`SPEC.md` 5.1, 5.5, `O-15`, definition of done 19).

Not measured yet: the identity proof's wire format outside the node, the guest's
verification cost for a shard's worth of records, and whether the anchor is the
verifier's own header or a checkpoint.

### Constraint - default nodes, retention, and the legacy chain (owner, 2026-09-26)

The owner requires the binding to work for people who run default nodes and do
not keep old flips, and to stay compatible with the legacy chain. Checked
against the pinned checkout (`SOURCES.md` F-94):

| Fact | Consequence |
|---|---|
| fast sync is the default (`config/config.go:280`) | verification must not assume a node processed the epoch |
| blocks and flips are pinned only up to a quantile (0.3 and 0.5, `config/ipfs.go:33`), with `Pin` available to keep more | "keep more flips" is a supported choice, not the default, so retention needs an incentive |
| the state tree keeps only the last 100 versions, by a compile-time constant (`core/state/statedb.go:39`) | `fast sync: false` does not retain an old epoch's state either; the shard input must come from a live capture or a replay, never from a running node |
| headers outlive the state they commit to | a proof bundle can be verified later by a default node: headers plus the bundle, no historical state |

Design rule recorded as `SPEC.md` 5.9: verification works on a default node, the
scarcity is in producing anchor bundles (live capture or replay) and in pinning
blobs, and those are off-chain service roles. No consensus change, no new
transaction type, no host function: the overlay reads only what the legacy chain
already commits. Who pays for retention is open as `O-16`.

### Decision - retention is demand-funded by a retrieval fee (owner, 2026-09-26)

The owner settled `O-16`: the reserve pays nothing for pinning or for producing
anchor bundles, the demand side pays, and every purchase carries a small fee to
the host the flip data was downloaded from.

| Rule | Consequence in the design |
|---|---|
| the buyer pays, not the reserve | a fee list rides in `buy`; it is credited to the named hosts from the buyer's own payment, so no buyer-host pair can draw on the reserve |
| the client names the hosts | the client knows which endpoint it fetched each CID from; the availability advisory carries the payment address a host advertises, and the fee is shown before signing |
| capped and optional | the cap is the recorded per-flip fee times the bundle's covered flip count, never blob size: size is not consensus-bound and would be a lever to pull. A purchase with no named host pays nothing, and a list over the cap is refused |
| attribution is reported, not proven | there is no proof of bandwidth; that is acceptable because the money is the buyer's own, and it is the buyer's client that reports the fetch |

The same rule covers the other retention role: producing an anchor bundle is
paid off protocol by whoever wants it, and never from the reserve.

### Measurement - a real identity proof, and what it costs (2026-09-26)

`O-15`'s first unknown is closed with a real datadir rather than a fixture. The
tool gained a read-only mode, `fod state-proof`: `-state-height 0` lists the
versions a datadir still holds, and a height prints one record with its
membership proof, the proof's natural size, the tree's identity count, and the
result of the pinned fork's own `RangeProof.Verify` against that version's root.
`-tree` chooses the identity tree (the header's `IdentityRoot`, which the
candidate check reads) or the ledger-side state tree.

| Step | Result |
|---|---|
| the retained window | a local June mainnet copy holds exactly 100 state versions, 10860039 to 10860138, which is `MaxSavedStatesCount` (F-31) seen on real data |
| the state tree | `tree=state height=10860138 identity_keys=211 identity_bytes=131 proof_bytes=796 path_items=16 verified=true`, root `264fbaa6...` |
| the identity tree | at the same height, the first key in the range: `identity_keys=144 identity_bytes=2 proof_bytes=404 path_items=8 verified=true` |
| a second version | the state-tree proof at 10860039 verifies against a different root `9be075a2...`, so a proof is bound to its own version |
| the tool's test | `TestStateProofVerifiesAgainstItsRoot`: a synthetic tree verifies against its own root and refuses a tampered one; `go vet .` and the whole tool suite are clean |

Two things fall out of the table. The mechanism is real and small: a proof is
hundreds of bytes and a few hashes, not a state snapshot, so a shard's worth is
kilobytes. And the sizes depend on tree depth, while these app datadirs hold only
a couple of hundred identity keys, so the representative mainnet size has to come
from a full node's copy, which means the capture host or a replay.

The count discrepancy from the first pass is resolved rather than queued:
`fod state-at` returned `true` from its walk callback for every key, and in this
tree that means *stop*, so the tool reported exactly one identity for every
datadir and height. The walk now takes the same direct route as the proof mode,
and `TestCollectStateIdentitiesWalksEveryKey` fails if a three-key walk ever
reports one. Re-measured with the fix: the first datadir has 211 identities and
80 candidates at 10860138 (Zombie 106, Newbie 42, Invite 19, Suspended 16, Human
13, Candidate 6, Undefined 3, unreadable 6), and the second has 196 identities
and 92 candidates at 10998672 with a 321-byte record and an 845-byte, 17-item
proof that verifies. No earlier recorded number depended on the broken count;
the runs quoted here are the ones that did.

### Measurement - the guest folds a shard's worth of identity proofs (2026-09-26)

`spikes/iavl-proof` answers the second half of `O-15`: can a guest verify the
identity proofs an anchor bundle carries, and what does it cost? The spike ports
the pinned fork's single-leaf fold (`ProofLeafNode.Hash`, `ProofInnerNode.Hash`,
zigzag varints, sha256) into a `no_std` core, checks it against real proofs with
a native test, and runs the same core in a RISC Zero guest over a fixture the
tool emitted.

| Step | Cycles | Per proof | Segments |
|---|---|---|---|
| one `env::read` per field, `sha2` crate | 539,236 for 4 proofs | 134,809 | 1 |
| packed input through serde, `sha2` crate | 363,622 for 4 proofs | 90,906 | 1 |
| packed input, zkVM SHA accelerator, serde | 2,840,971 for 32 | 88,780 | 3 |
| packed input through `read_slice`, accelerator | 402,342 for 32 | 12,573 | 1 |
| a shard's worth, 256 proofs | 3,187,850 | 12,453 | 4 |

Two negative results are the useful part: the first measurements were dominated
by the input path (a serde read per field, then a serpentine read of the blob),
and the `sha2` crate does not use the zkVM's SHA-256 accelerator. Once the input
arrives through `read_slice` and the fold hashes through `risc0_zkvm::sha`, the
cost per proof falls by an order of magnitude.

What that means for the release: a 256-proof shard adds about 3.2 million cycles
and four segments to a registration that already runs 7.6 million cycles and
eight segments, so folding the anchor check into registration is affordable.

Evidence: `cargo test -p fod-iavl-proof-core` verifies every proof in
`spikes/iavl-proof/core/fixtures/bundle.json` against its version's root and
refuses a changed value or path; `cargo run --release -p iavl-proof-host --
core/fixtures/bundle.json N` prints the cycles and exits non-zero if the guest's
verdict differs from the native one. The fixture is four real identity proofs at
height 10860138, emitted by `fod state-proof -proof-bundle`.

Still open in `O-15`: the shard size on a real mainnet capture, and whether the
anchor is the verifier's own header or a checkpoint.

### Decision - the anchor check folds into registration (2026-09-26)

The session binding could be a second proof the escrow verifies beside
registration, or part of the registration guest. The measurement above decides
it for folding:

| Reason | Number |
|---|---|
| one verification instead of two | a proof-carrying call is about 195 million wasm gas of the 512-million ceiling, so two receipts would roughly double the gas and the fee of every registration (about 188 iDNA per call at the 2026-09-25 rate) |
| the extra guest work is affordable | 3,187,850 cycles for 256 proofs, four segments on top of registration's 7.6 million cycles and eight segments |
| one artifact to pin | the escrow already pins one guest image id and one claim digest; a second receipt would add a second set of both to the module and to every deployment check |

The anchor choice stays a verifier policy rather than a bundle change: the same
bundle folds to the root of whatever version it was taken from, so a buyer whose
own node agrees on that header, and a buyer checking a published checkpoint,
verify the same bytes. `O-15` keeps that question open without blocking the
format.

What changes when the pool package lands: the registration input gains
`proofAnchor` (the bundle `fod state-proof -proof-bundle` writes, which
`prepare-offer -proof-anchor` already carries and validates), the guest folds it
and binds the anchor root, height, tree, and proof count in the claim, and the
escrow's registration path checks nothing new beyond the claim it already
parses. The guest edit invalidates the shipped registration receipt, so the pool
package re-proves it and re-pins the image id in the escrow, the CLI, and the
release manifest.

### Seller-side piece - the bundle covers the shard and pins the anchor (2026-09-26)

`fod state-proof` gained the two flags the registration input needs:
`-addresses FILE` emits one proof per identity address, refusing any address with
no record at that height, and `-expect-root DIGEST` refuses a version whose root
is not the digest the caller expects. `prepare-offer` refuses a `-proof-anchor`
whose root is not the input's validation anchor, so a bundle about some other
state cannot ride along.

| Check, real datadir at height 10860138 | Result |
|---|---|
| three candidate addresses from a capture | `tree=state proofs=3 root=264fbaa6...`, the anchor those candidates live under |
| a wrong expected root | refused: `has root 264fbaa6..., not the expected 00` |
| an address with no record | refused: `no identity record for 0x0123... at height 10860138` |
| the guest over that bundle | native and guest fold 3/3 proofs to the root, 76,688 cycles, 25,563 per proof |

The state tree's proofs are larger than the identity tree's (16 to 17 path items
against 7 to 8), so a shard's worth costs more here than the 12,453-cycle figure
above; both are the same fold and both are measured.

What is still missing before the pool package's first half is done: the guest
must fold the bundle and bind the anchor root, height, tree, and proof count in
the registration claim, and the escrow constants, the CLI constants, and the
release manifest follow with the re-prove.

### Anchor module placed, and what the guest wiring has to ship with (2026-09-26)

The fold now lives as its own product crate, `proofs/anchor-core`, with the real
candidate bundle as its vector (`anchor-v1.json`, three state-tree proofs under
`264fbaa6...`, 16 to 17 path items each). Its test folds every record to the
anchor, refuses a tampered value hash, an empty bundle, and another root, and the
registration crate is untouched.

Two findings decide how the guest wiring has to land:

| Finding | Consequence |
|---|---|
| any edit under `proofs/registration/` invalidates the sealed receipt | adding the module and one `pub mod anchor;` line made `sealed_artefact` fail; reverting the two edits restored it. The guest edit, the re-prove, and the re-pin of the image id are one package, not two |
| the frozen manifest vector's anchor `18c73701...` has no retained source | checked the probe node (one version), the devnet node's versions 30 to 36, and both 100-version datadirs: none carries that root, so no bundle can be produced for it. When the bundle becomes mandatory, the vector's anchor moves to a root with a retained bundle - the devnet node's state root `b1c962c7...` at height 36 is the obvious candidate - and the manifest commitment and claim vector are regenerated in the same change |

Next: the guest reads a packed bundle and requires it to fold to the anchor of
the manifest commitment; the host packs the `proofAnchor` the tool already
carries; the vectors are regenerated; the receipt is re-proved (about sixteen
minutes) and the image id re-pinned in the escrow, the CLI, and the manifest.

### The registration proof now requires the session binding (2026-09-26)

The anchor check is folded into the registration guest, so one receipt carries
both the qualification and the proof that the shard's identity records sit under
the anchor the claim commits. The claim layout is unchanged at 178 bytes: the
anchor was already inside the manifest commitment, and the bundle now makes it a
root the records were proven against instead of a number the seller typed.

| Step | Result |
|---|---|
| the vector's anchor | moved from the lab root `18c73701...`, which no retained datadir can prove, to the devnet node's state root `b1c962c7...` at height 36, with a real three-record bundle beside it |
| the guest | reads a packed bundle, folds every record to the anchor, and refuses the input otherwise; an empty bundle is refused |
| the negative runs | five mutations are refused, the new one being `tampered anchor`: `the proof anchor does not fold to the anchor the manifest commits` |
| cross-language agreement | the Rust guest commits exactly the Go-built claim vector: `check the guest commits exactly the frozen claim vector` |
| the new receipt | proved in 1082.4 s, 8,388,608 cycles in 8 segments, image id `5f0a0c0d...`, claim digest `bdf15a3f...` |
| the pin | the escrow constant, the CLI constant, and the release manifest carry the new ids, `sealed_artefact` passes, the escrow's tests pass, and the tool suite passes |

The seller's tool refuses to prepare an offer without the bundle
(`prepare-offer -proof-anchor`), so a registration that could not be sold into
the pool cannot be built by this tooling either.

### The pool's arithmetic, and the rounding rule it needs (2026-09-26)

The pool contract starts with the part that carries the invariants: `r` iDNA
against `q` sale units per bundle, and the three moves over them, in
`contracts/escrow/src/pool.rs`. Integer arithmetic cannot hold the product
exactly, so the spec's claim of an exactly neutral round trip was wrong; both
quotes now round in the pool's favour, a buyer paying `ceil(r / (q - 1))` and a
seller receiving `floor(r / (q + 1))`.

| Check | Result |
|---|---|
| the quotes | 50,000 against six units gives a 10,000 buy and a 7,142 sell; one unit has no buy quote |
| a round trip | for every reserve in a set spanning one base unit to `u64::MAX` and every unit count from 2 to 64, the seller gets back at most what the buyer paid and the reserve never shrinks |
| a walk | 7,200 steps of buys, sells, and gifts over thirty-six starting states: the product never falls below its starting value |
| direction | a buy raises the next quote and sells lower it |
| exactness | when the reserve divides by `q - 1`, a buy leaves `r * q` unchanged |
| refusals | a wrong amount, a one-unit bundle, and a full `u128` accounting are each refused without moving the state |

Ten escrow tests pass. The contract call path that uses this arithmetic, and the
refund and strike paths, are the next piece; `O-14` stays open until they are
harnessed.

### The pool's supply side is in the escrow (2026-09-26)

`contracts/escrow` now carries the pool's supply side beside the earlier offer
slice, per `SPEC.md` 5.1 and 7.1:

| Piece | What it does |
|---|---|
| the `PoolBundle` record | author, manifest commitment, reserve, units, beacon block, and strikes, in a fixed 88-byte encoding with a round-trip test and a short-record refusal |
| `registerBundle` | the session-bound registration proof: the same journal checks as the offer slice, and the bundle record the pool reads |
| `addReserve` | anyone may add the amount the call carries to the reserve; the ratio is what sets the quote |
| `donateUnit` | adds a unit and pays nothing |
| `sellUnit` | presents the registration receipt, so the seal proves possession while the caller may be any holder, then pays `floor(r / (q + 1))` into the caller's credit |
| `beacon` | refreshes the delivery beacon at the current block |
| `poolView` | reserve, units, beacon, strikes, and both quotes as JSON |

Thirteen escrow tests pass, the wasm target builds with the new exports, and the
offer slice is untouched so its harness coverage keeps working while the pool's
call surface is built. The demand side (`buy`, `deliver`, `confirm`, `refund`,
and the strike) and a Go harness test over these calls are the next piece; the
`pool.rs` warnings for the unused `buy` are that work showing up.

### The pool's demand side is in the escrow (2026-09-26)

The rest of the pool's call surface is written: `buy` (the pool's quote, the
delivery-key possession proof bound to the bundle and a per-bundle nonce, and the
order the caller gets back), `deliver` (stores the buyer-bound ciphertext before
the deadline), `confirm` (the buyer's one call), `fulfillOrder` (the delivery
proof path that settles without the buyer), and `refundOrder` (credits the buyer
out of the reserve after the deadline and the grace period, and records the
strike). A strike takes the beacon down, so the desk has to prove it is alive
before new orders; the second clears the listing. The beacon's freshness window
is 288 blocks.

Naming: the pool's proof path and refund are `fulfillOrder` and `refundOrder`
while the offer slice's `fulfill` and `refund` still exist. They take the
specification's names when the offer slice goes.

Verified so far: the wasm target builds with the eleven pool exports, and
fifteen escrow tests pass, including the pool record with its buy nonce, the
order record's round trip and state check, and the strike rule. The call surface
itself still needs the Go harness, which needs pool seed hooks under the
`test-hooks` feature; that is the next piece, and it is what closes `O-14` for
the calls rather than only the arithmetic.

### The pool's call surface has harness coverage (2026-09-26)

`spikes/harness` now walks the pool through the real runtime, with two seed hooks
(`seedPoolBundle`, `seedPoolOrder`) standing in for the registration proof and
the buyer's signature:

| Check | Result |
|---|---|
| the supply side | a seed of 50,000 against two units quotes 50,000 to buy and 16,666 to sell; a gift makes it 25,000 and 12,500; adding 10,000 to the reserve makes the buy quote 30,000 |
| delivery and confirmation | `deliver` publishes the ciphertext once, a second call is refused `order_not_funded`, and the buyer's `confirm` settles |
| the refund window | a refund before the deadline is refused `refund_not_available`; after it, the buyer's credit is exactly the paid amount and the reserve falls by it |
| conservation | with the seeded money in place, the accounting view reads `liquid 70,000` against a 30,000 reserve and 30,000 of credit |
| the strikes | the first refund takes the beacon down; the second leaves the bundle with zero units, so the listing is cleared |
| the proof path | the CLI's real Groth16 delivery receipt settles a pool order through `fulfillOrder` at 195,585,115 wasm gas, and the settled order refuses a later refund |
| the surface | the no-admin check pins the eleven pool exports and the two seed hooks' absence from the release module |

The delivery statement binds the order id and nonce rather than an offer record,
which is why the same seal that settles an offer settles a pool order whose
fields it names: no new proof program was needed for the pool's proof path.

Not covered yet: `buy`'s possession signature, which the seed stands in for, and
the CLI-side preparation of the pool calls. Two local artefacts had to be
regenerated after the earlier re-prove and the pool's growth - the prepared
registration and the prepared deploy - which belongs in the exercise runbook.

### The buyer's side of a pool purchase is prepared and checked (2026-09-26)

`fod prepare-buy` builds the call the pool's `buy` takes: the bundle, the
delivery key, and the possession proof over the pool's own context - the domain,
the contract, the bundle, the buy nonce, the paying caller, and the key - which
is a different construction from the offer flow's, because it binds the bundle
and its nonce rather than an offer id (§`contracts/escrow/src/lib.rs`,
`pool_delivery_key_message`).

| Check | Result |
|---|---|
| the tool | `prepare-buy` writes a 162-byte calldata and refuses a short bundle, a bad amount, a short contract, and a missing delivery key |
| the proof | the test recovers the signer from the calldata and gets the delivery key back, while another nonce, caller, or bundle each breaks it |
| the runtime | the CLI's calldata opens an order for exactly the pool's quote (50,000 against three units quotes 25,000) at 164,547 wasm gas, and the order view names the prepared buyer and amount |
| the replay | the same calldata is refused afterwards with `delivery_key_not_bound`, because the buy nonce moved |

Both full suites pass: the tool's tests and the harness (`go test ./...` in
`tools/fod` and `spikes/harness`). What is left in the pool package is the
interface and CLI pass (the pool views the client reads, the delivery desk
command), then the offer slice goes and the pool's two transitional names take
the specification's.

### The CLI reads the pool for the interface (2026-09-26)

`fod market` now also walks the contract's `pool/` prefix and writes a `pool`
section into the marketplace state the client imports: bundle, author, manifest
commitment, reserve, units, beacon, strikes, and both quotes. The quotes are
computed with the contract's own rounding, a buy rounded up and a sell rounded
down, and the reader refuses a short record, so a listing the client shows is the
one the contract would quote.

Evidence: a decode test mirrors the escrow's 96-byte `PoolBundle` encoding - it
reads 50,000 against six units as 10,000 and 7,142, the 10-against-three rounding
as 5 and 2, and a single-unit bundle as no buy quote with a 25,000 sell quote -
and the tool suite passes.

The client ignores the section for now: its reader requires `offers`, `orders`,
and `credits` and treats `bundles` as optional, so rendering the pool is the UI
pass's work rather than this one's. What remains in the pool package: the
delivery desk command (`serve`), the UI pass over this state, then removing the
offer slice and renaming `fulfillOrder` and `refundOrder` to the specification's
names.

### The CLI reads the pool's orders too (2026-09-26)

`fod market` now walks `porder/` beside `pool/`, so the marketplace state
carries the pool's orders as well: id, bundle, buyer, paid amount, funded block,
both deadlines, and the state. The decode mirrors the escrow's 150-byte
`PoolOrder`, computes the deadlines from the funded block, and refuses a short
record or an unknown state byte. The delivery key is deliberately left out of
the state file: the buyer keeps their own copy, and the delivery desk reads the
order from the chain.

Evidence: the decode test reads a crafted record as 25,000 paid from block 1,000
with a 1,720 delivery deadline and a 1,840 refund block in the delivered state,
then refuses an unknown state and a short record; the tool suite passes.

This is the desk's read half. `serve` still has to loop over these orders, build
the buyer-bound ciphertext with the existing delivery prover, publish it with
`deliver`, keep the beacon fresh, and fall back to the proof when a buyer does
not confirm.

### The delivery desk's fast path (2026-09-26)

`fod serve` now runs the desk of `SPEC.md` 6.4 step 9. It reads one or more
encrypted epoch-key vaults, derives each bundle's key commitment from the keys
alone, watches `pool/` and `porder/`, and for every funded order on a bundle it
holds publishes the buyer-bound ciphertext with `deliver`. It refreshes a
served bundle's beacon when the beacon is older than half of the contract's
288-block window. It keeps one transaction in flight at a time: it reads the
account nonce and the chain epoch, signs the next call from the measured fee
table, re-signs when the signed size needs a higher max fee, and waits for the
account nonce to move before the next call.

The fast path needs no proof. `deliveryCiphertextFor` encrypts the canonical
secrets to the delivery key the order recorded with the pinned node's own
ECIES, and the buyer's tool is what checks the plaintext against the bundle
commitment before confirming. The proof fallback is wired for a delivered order
in the last 280 blocks before its deadline, and it only runs when the caller
names `-prover`: it builds the prover order from the vault, runs the delivery
prover, and submits the same `fulfillOrder` calldata the harness already
accepts.

Evidence:

- `TestDeliveryCiphertextOpensToTheBundleCommitment` decrypts the desk's
  ciphertext with the buyer's key and checks SHA-256 of the plaintext against
  the pool commitment; an unrelated key fails.
- `TestServePlanDeliversFundedOrdersAndRefreshesStaleBeacons` picks the funded
  served order and the stale beacon, and leaves alone an expired order, a
  settled order, an unserved bundle, and a fresh beacon; without a prover the
  delivered order is not proved.
- `TestServeOnceWritesTheDeliverAndBeaconActions` drives the real runner over a
  node-shaped JSON-RPC endpoint with the contract's own 96-byte and 150-byte
  records, then opens the written `deliver-1.json` with the buyer key.
- `TestServeBroadcastSignsTheDeskCalls` signs both calls and hands them to the
  endpoint; decoding them with the node's own parser recovers a `deliver` and a
  `beacon` call, and the deliver ciphertext opens to the commitment.
- The harness measured the calls the desk sends: `beacon` 102,415 wasm gas,
  `deliver` 111,201, `confirm` 111,338, `refundOrder` 113,964, `buy` 164,547,
  and `fulfillOrder` 195,585,115. The fee table now carries the desk operations
  with the canonical signed sizes (`beacon` 151, `deliver` 350, `fulfillOrder`
  820, `buy` 292 bytes at a 100 iDNA quote), and `TestServeCanonicalSignedSizes`
  pins them.
- `go test -count=1 ./...` in `tools/fod` passes; the pool tests and the
  no-admin check pass in `spikes/harness`.

Not covered, and not claimed: the desk has not run against a live node or a
deployed registry, and the fallback's sixteen-minute prove-and-submit cycle has
not been run end to end. The buyer's receive and confirm side for the pool is
not implemented yet; `fod receive` still reads the offer slice, so a real
purchase cannot finish through this CLI until that package lands. The release
artefact is unchanged; only the CLI and the harness test moved.

### The buyer's pool receive and confirm path (2026-09-26)

`fod receive -pool-order N` opens the delivery the pool recorded. It reads the
order and the ciphertext from `porder/<id>` and `pdeliv/<id>`, refuses a funded
order with no delivery and a refunded order, checks that the delivery key this
machine holds is the one the order recorded, decrypts with the pinned node's
ECIES, and checks SHA-256 of the plaintext against the bundle commitment before
writing the epoch keys as an `age` vault. The checks list says what was verified
and what was not: the publisher of the ciphertext is not authenticated, and a
delivered order still needs the buyer's confirmation to settle.

`fod prepare-confirm -contract ADDRESS -pool-order N -prepared FILE` writes the
buyer's confirmation call. The escrow parses the order id as a decimal string,
so the calldata is the decimal text (`7` for order 7), not the 8-byte
big-endian form the delivery calls use. `fod submit` signs it and the buyer's
nodekey sends it.

The marketplace state now carries the pool bundle's buy nonce, so the client can
drive `prepare-buy` from the state it imports instead of asking the user for a
chain value.

Evidence:

- `TestPoolReceiveOpensTheDeskDelivery` opens a desk-built delivery with the
  buyer's key, checks the commitment, reads the written vault back, and refuses
  an unrelated delivery key, a ciphertext for another bundle, and a funded
  order with no delivery.
- `TestPrepareConfirmBuildsTheDecimalCall` checks the decimal calldata for a
  small and a large order id and loads the written action through the same
  `fod submit` decoder.
- `TestDecodePoolRecordReadsTheReserveAndQuotes` checks the nonce the client now
  receives.
- `go test -count=1 ./...` in `tools/fod` passes; the harness's `confirm` call
  (decimal `1`) still settles a delivered order.

Not covered, and not claimed: no live node or deployed registry, and the
buyer's confirm call has not been broadcast on a chain. The interface does not
render the pool section or an order list yet; that is the UI pass. The offer
slice remains until its readers are replaced.

### The pool's supply side (2026-09-26)

`fod prepare-registration -pool` now writes the pool's `registerBundle` call
from the same sealed receipt the offer slice uses. The escrow parses journal
(178) || seal (256) identically for both operations, and the CLI checks the
same policy, consensus, image id, and claim digests before writing it; a test
pins the two variants' calldata to the same bytes.

`fod prepare-list -pool` reads a `sellUnit` or `donateUnit` action request and
writes the pool call. A gift is the 32-byte commitment. A sale is the
commitment followed by the receipt's journal and seal (466 bytes); the CLI
reads the pool record (keyed by the raw commitment, so the read walks the
`pool/` prefix), checks the receipt's bundle and author against it, and refuses
a sale without the receipt or a gift that carries one.

The harness now registers a real sealed receipt through `registerBundle`,
refuses a non-author and a duplicate, gives a unit, adds 50,000 to the reserve,
and sells a unit at the pool's own quote: the seller's credit is 25,000, the
reserve falls to 25,000 against two units, and the next buy quote is 25,000.

Evidence:

- `TestPoolSupplyFromASealedReceipt` measures `registerBundle` 195,587,992 wasm
  gas, `sellUnit` 195,592,599, and `donateUnit` 102,480, and checks the pool
  record, the credit, the reserve, and both quotes after each move.
- `TestAssemblePoolRegistrationUsesTheSameReceipt` pins the pool registration
  to the offer registration's argument bytes.
- `TestPreparePoolSellUnitBuildsTheReceiptArgument` checks the 466-byte
  commitment || journal || seal calldata and the refusals for another author,
  another bundle, a missing receipt, and a receipt on a gift.
- `TestPreparePoolDonateUnitBuildsTheCommitment` checks the 32-byte gift and
  loads it through the `fod submit` decoder.
- `TestRegistryPoolBundleReadsTheRawKeyLayout` checks the prefix-walk read the
  pool's raw-key records need.
- `TestPoolSupplyCanonicalSignedSizes` pins the fee table's signed sizes:
  `registerBundle` 564, `sellUnit` 590, and `donateUnit` 155 bytes.
- `go test -count=1 ./...` in `tools/fod` passes; the pool harness tests pass.

Not covered, and not claimed: no live node or deployed registry, and no mainnet
call. The interface does not write the pool's sell or give requests yet; that
is part of the UI pass. The offer slice's `listBundle` path remains until its
readers are replaced.

### The client's pool market (2026-09-26)

The single-file client now reads the pool state and renders the canonical
market. `state.ts` validates the optional `pool` and `poolOrders` sections, so
an older offer-only snapshot still parses and falls back to the offer table.
The market view shows each listing's author, epoch and flip count when a
registration artefact supplied them, the pool's quote, the unit count, and the
delivery beacon's freshness at the snapshot's block. A buy button writes the
`buy` action request the CLI consumes; a listing with fewer than two units, a
stale beacon, or a snapshot without the viewer's address disables it.

`fod prepare-buy` now takes `-request FILE` as well as the flag form. It checks
the request's buyer against the nodekey, walks the pool's raw-keyed records for
the current nonce and quote, refuses a quote that moved, and creates the
buyer's dedicated delivery key when the file does not exist yet; the
`-delivery-key-archive` flag still writes the recovery archive.

Evidence:

- `npm run typecheck`, `npm run build`, and `npm run verify` in `app/` pass.
  The browser walk's new checks are "the pool market renders the listing, its
  quote, and the live desk", "the pool buy request names the bundle, the
  quote, and the buyer", and "the pool buyer is told what to do with the file".
- `app/check-state.ts` accepts the new `example-pool-state.json` as one pool
  listing and one pool order, and still accepts the offer-only example.
- `TestPreparePoolBuyFromTheInterfaceRequest` builds the buy from an interface
  request against a pool record, and refuses a stale quote and another buyer.
- `TestLoadBuyRequestChecksTheEnvelope` checks the request envelope, including
  the amount and expected-quote agreement.
- `TestPoolListingCarriesRegistrationProvenance` checks the epoch and flip
  count the client shows.
- The full `tools/fod` suite and the pool harness tests pass. The app bundle
  changed from 272,144 to 277,311 bytes, and the release manifest is refreshed.

Not covered, and not claimed: the trades view still writes the offer slice's
`listBundle` and `refund` requests; the pool's sell, give, confirm, and
`refundOrder` views are the next slice. The availability advisory is still off
chain and not in the snapshot, so the market shows the on-chain beacon rather
than a live fetch check. No live node or deployment is involved; the browser
walk uses the example fixtures and the local gateway.

### The client's pool trades (2026-09-26)

The trades view now renders the pool when a snapshot carries it. The buyer's
orders show the paid amount, both deadlines, and the state; a delivered order
offers Confirm (the `confirm` request) and Receive (the `fod receive` command),
and an order past its refund block offers Refund (`refundOrder`). The seller's
listings show the quote, units, and beacon; the author can Sell one unit, and
the author or a holder can Give one unit. The offer slice's trades remain as
the fallback for snapshots without pool data.

`fod prepare-confirm` now reads the interface's confirm request (bundle and
order id) as well as the flag form, and the `refund` command accepts the pool's
`refundOrder` request, writing the decimal order id the escrow parses. The
FAQ's market entries now describe the pool's quote, the one-sale rule, the
delivery desk, and the strike instead of the offer slice's author-set price.

Evidence:

- The browser walk's new checks: the pool trades list the delivered and the
  refundable order; the confirm request names the bundle and the order; the
  pool refund request names the order; the sell and give requests name the
  commitment; the notices name `fod prepare-confirm`, `fod refund`, and
  `fod prepare-list -pool -request`.
- `TestLoadConfirmRequestNamesTheOrder` and
  `TestPreparePoolRefundOrderWritesThePoolEncoding` check the new CLI request
  paths and their decimal encodings; the full `tools/fod` suite passes.
- `npm run typecheck`, `npm run build`, and the full `npm run verify` pass; the
  app bundle is 284,959 bytes and the manifest is refreshed.

Not covered, and not claimed: the availability advisory is still off chain and
not in the snapshot. The offer slice's trades remain as a fallback and go with
the slice. No live node or deployment is involved; the browser walk uses the
example fixtures and the local gateway.

### The owner's funding call (2026-09-26)

`fod prepare-fund -contract ADDRESS -pool-bundle COMMITMENT -amount BASE_UNITS
-prepared FILE` builds the pool's `addReserve` call: the commitment (32 bytes)
and the amount as a 128-bit big-endian argument, with the same amount carried
as the transaction's value, because the escrow checks that the value equals the
argument. The prepared file is one `fod submit` signs. This closes the last
contract operation the CLI could not prepare: the pool can now be registered,
funded, sold into, gifted into, bought, delivered, confirmed, proved, refunded,
and withdrawn from the CLI.

The harness's supply test now logs the `addReserve` measurement: 102,892 wasm
gas. The fee table carries it at 182 canonical signed bytes.

Evidence:

- `TestPreparePoolFundWritesTheAmountAndCommitment` checks the 48-byte calldata,
  the prepared amount, and the file's acceptance by the `fod submit` decoder.
- `TestPreparePoolFundRefusesBadInputs` refuses a zero amount, a non-integer,
  an amount above `u128`, a short bundle, and a short contract.
- `TestPoolFundCanonicalSignedSize` pins the fee table's signed size.
- `TestPoolSupplyFromASealedReceipt` measures the call and checks the reserve
  and quote after the funding; the full `tools/fod` suite passes.

Not covered, and not claimed: the release README's mainnet exercise still
describes the offer slice's commands; updating it to the pool and this funding
call is the next documentation package. No live node or deployment is involved.

### The release map and the last fee entries (2026-09-26)

`fod fees` now carries the pool's `confirm` (111,338 wasm gas, 121 canonical
signed bytes) and `refundOrder` (113,964 wasm, 125 bytes) entries, so every pool
operation the CLI can prepare has a measured gas figure and a signed size.
`TestPoolConfirmAndRefundSignedSizes` pins both.

`release/README.md` now maps the definition of done to the pool: the
proof-carrying calls (`registerBundle`, `sellUnit`, `fulfillOrder`), the small
calls, the gifted-listing path, the three journeys, the reserve invariants, the
session-binding gate, and the retrieval fee (marked not implemented). Its
mainnet exercise now uses the pool's commands: `prepare-registration -pool`,
`prepare-fund`, `prepare-list -pool`, `serve`, `prepare-buy -request`,
`receive -pool-order`, `prepare-confirm -request`, `refundOrder`, and
`withdraw`. The two lab-chain broadcasts recorded on 2026-09-25 are described
as the earlier offer slice, not the pool.

Evidence: the full `tools/fod` suite passes with the new fee entries; the
browser walk and the harness pool tests are unchanged and passing from the
previous packages; the release README was reviewed as a document, and no
executable behavior changed there.

Not covered, and not claimed: the availability advisory and the retrieval fee
remain open, and the offer slice's code remains as the transitional path. No
live node or mainnet action is involved.

### O-14 closed: the pool's call sequence (2026-09-26)

`TestPoolCallSequenceAndInvariants` drives the pool through the real runtime
with two buyers and a seeded bundle. The first buyer's wrong amount is refused
without moving the nonce, and the same proof then buys at the quote; the proof
cannot be replayed after the nonce moves; the second buyer takes the last unit
at the next quote; a third buy is refused for the unit count; delivery and
confirmation settle the first order and duplicates are refused; the second
order's delivery is too late after the deadline and its refund is refused until
the grace period ends; the mature refund credits the buyer and strikes the
bundle, taking the beacon down; a stale beacon blocks new funding; a gift adds
a unit, the desk refreshes the beacon, and the third order's missed delivery is
the second strike, which clears the listing. `assertPoolInvariant` checks
`liquid >= reserve + creditTotal` after every move.

The real sealed-receipt path (`registerBundle`, `donateUnit`, `addReserve`,
`sellUnit`) remains covered by `TestPoolSupplyFromASealedReceipt`. `O-14` is
closed in the spec, and the release map's row 18 is verified.

Evidence: the full harness suite and the full `tools/fod` suite pass.

### The availability advisory (2026-09-26)

`fod inspect -availability-out FILE` writes the off-chain availability advisory
of `SPEC.md` 5.8 from the inventory's blob checks: author, epoch, the check
time, the source, the covered and verified flip counts, the completeness flag,
and each flip's CID, bytes, fetch source, and CID verdict. It is advisory and
never contract state. `fod market -availability FILE[,FILE...]` reads one or
more advisories, matches them to the pool listings by author and epoch (the
provenance the registration artefacts supply), and writes the summary into the
market state. The client's pool market renders the result: an "available" or
"partial" chip and the verified/covered flip count beside the delivery-desk
state, or "not checked" when no advisory was supplied.

Evidence:

- `TestAvailabilityFromInventory` checks the counts and the per-flip record,
  including a failed CID.
- `TestAvailabilityRoundTripAndMarketJoin` checks the envelope, the author/epoch
  index, the join, the no-epoch refusal, and a bad format.
- The full `tools/fod` suite passes.
- The browser walk's pool-market check now asserts "available" and "6/6 flips
  checked"; the full `npm run verify` passes. The app bundle is 285,724 bytes
  and the manifest is refreshed.

Not covered, and not claimed: the advisory is not authoritative, it carries no
proof of bandwidth, and the retrieval fee itself (the host payment address and
the per-flip value, `O-16`) remains unimplemented pending the owner's decision.
A permissionless refresher can run `fod inspect` and publish the result.

### The transitional offer slice is gone, and the pool owns the spec's names (2026-09-26)

The contract now implements one marketplace, the ownerless pool, under the
names `SPEC.md` publishes. The offer record, its bundle registry, and the entry
points `registerOffer`, `listBundle`, `fundOffer`, `cancelOffer`, the offer
`refund` and `fulfill`, the order and bundle views, and the `seedBundle`,
`seedOffer`, and `setOfferCommitment` hooks are removed. The pool's
`fulfillOrder` and `refundOrder` are renamed to `fulfill` and `refund`, and the
release module's export surface is exactly the pool's: `allocate`, `deploy`,
`registerBundle`, `addReserve`, `donateUnit`, `sellUnit`, `beacon`, `poolView`,
`buy`, `deliver`, `confirm`, `fulfill`, `refund`, `orderPoolView`, `withdraw`,
`accounting`, and `memory` (`TestReleaseModuleExportsNoAdministrativeEntryPoint`
compares the whole surface, not a deny-list; the test-hooks build adds only
`seedPoolBundle` and `seedPoolOrder`).

The client and the CLI lose every offer-lane surface with it. `fod market`
writes a pool-only snapshot (the `offers`, `orders`, and `bundles` arrays are
gone; `credits`, `pool`, and `poolOrders` stay), the CLI commands
`prepare-purchase` and `prepare-delivery` are removed, `prepare-list` and
`prepare-registration` are pool-only, `receive -offer` is removed, and the
interface's offer fallback, relist flow, and offer state reader are gone. The
pool refund request is the spec-named `refund` with an `orderId`; its calldata
is still the order id as decimal text.

Re-measured on the smaller module, the pool's gas maxima (test-hooks build,
which is the conservative bound) and the signed sizes the fee table now carries:

| Operation | wasm gas | signed bytes |
|---|---|---|
| `registerBundle` | 195,587,992 | 564 |
| `sellUnit` | 195,595,892 | 590 |
| `fulfill` | 195,589,579 | 815 |
| `buy` | 167,771 | 292 |
| `deliver` | 112,341 | 350 |
| `beacon` | 102,691 | 151 |
| `donateUnit` | 102,725 | 155 |
| `addReserve` | 106,380 | 182 |
| `confirm` | 114,331 | 121 |
| `refund` | 116,889 | 120 |
| `withdraw` | 201,987 | 117 |

The signed deploy grew with the rebuilt module: 365,269 raw transaction bytes,
which is 354.10 iDNA at the 2026-09-25 rate, and the deploy still measures
3,000,051 wasm gas inside the 1,541,400 user-gas headroom
(`TestReleaseModuleFitsTheDeployGasHeadroom`). The gatekeeper check reports zero
forbidden operators on the release artefact.

Evidence:

- `cargo test` in `contracts/escrow`: 13 passed (the pool arithmetic, the pool
  and order records, the strike rule, and the verifier bindings).
- `GOFLAGS=-modfile=go.local.mod go test ./...` in `spikes/harness`: the pool
  supply, sequence, buy, delivery, and refund paths pass against the real
  runtime; the CLI-prepared pool refund (order 7, decimal calldata) credits the
  buyer 100 iDNA at 116,889 wasm gas; that cross-check runs after `fod refund`
  has prepared `output/cli/prepared-pool-refund.json` from an order-7 request
  and skips when the file is absent, like the other CLI cross-checks; the
  release module exports nothing the spec does not publish.
- `GOFLAGS=-modfile=go.local.mod go test ./...` in `tools/fod`: the full suite
  passes with the pool-only market state and the renamed fee entries.
- `npm run typecheck && npm run build && npm run verify` in `app`: the browser
  walk passes pool market, buy, confirm, refund, sell, give, and withdrawal
  requests from the pool-only fixture; the bundle is 271,204 bytes.

Not covered, and not claimed: no live node or deployment is involved, and the
pool path has still not been broadcast (the two lab-chain broadcasts of
2026-09-25 were offer-slice calls and remain historical). The `O-15` anchor
gate is unchanged, and the retrieval fee (`O-16`) is still open.

### Package 9 pre-flight re-priced on the pool-only contract (2026-09-26)

The earlier pre-flight priced the exercise's nine calls with the offer-slice
operations, which the contract no longer has. Priced again with
`fod fees -fee-per-gas 96153846153800` against the pool-only module, the same
nine steps - deploy, `registerBundle`, `sellUnit`, `addReserve`, `fulfill`,
`withdraw`, `addReserve`, `refund`, `withdraw` - carry 921.61 iDNA in required
max fees. The deploy is 354.10 and the three proof-carrying calls are 188.61,
188.64, and 188.85; the other five calls are below 0.31 each. The exercise
account holds 29.78 iDNA in balance against 86,233 iDNA staked, so the funding
requirement is roughly 922 iDNA at the measured maxima, up from the
pre-removal 726 because the pool's deploy and proof calls are larger and the
small offer-lane calls are gone. No transaction was sent; this is a fee-table
read. Funding status changed on 2026-09-26: the owner designated a terminated
identity's residual balance for bounded deployment experiments (section 0),
which covers the deploy smoke test only, not this full-exercise fee table.

### The unpushed history passes the owner's privacy audit (2026-09-26)

Before any push (`Q12`), the history was scanned for the private-data classes the
owner called out: absolute home paths, worktree paths, host names and addresses,
`root@` SSH strings, and credential or key material. The scan covers the added
lines of every commit in `origin/main..HEAD`, the added lines of the pushed
history, the full text of every tracked file, every added file name, and the
strings of every tracked wasm artefact. A pickaxe search over all refs checks
whether any commit ever introduced a home path or a worktree path, even if a
later commit removed it.

| Scan | Result |
|---|---|
| `git log -p origin/main..HEAD` added lines, private path/host patterns | 0 |
| the same over `origin/main` | 0 |
| added-line key material (private keys, provider tokens) | 0 |
| tracked-file contents, the same key material | 0 |
| tracked wasm strings, private path/host and private-key markers | 0 |
| added file names matching nodekey/keystore/.key/.env/passphrase | 1: `tools/fod/delivery_secrets.go`, source code with no secret |
| `git log --all -S` for a home path or a worktree path | no commit |

The rewrite question in `Q12` is therefore about presentation only: there is no
leak the history rewrite has to remove.

### Package 9 - the pool-only bytes reproduce from a clean clone (2026-09-26)

A fresh `git clone --no-hardlinks` at `a4b2041` rebuilt both release artefacts
with the manifest's own commands - `cargo build --release --target
wasm32-unknown-unknown` in `contracts/escrow`, `wasm-opt
--llvm-memory-copy-fill-lowering -O3`, and `npm install && npm run build` in
`app` - and every byte and digest matches `release/MANIFEST.json`. The clone is
two documentation commits after the revision the manifest names (`bf023ff`),
which is why the bytes can be expected to match; that is exactly what was
checked rather than assumed.

| Artefact | Clean clone | `release/MANIFEST.json` |
|---|---|---|
| `release/artifacts/fod_escrow.wasm` | 365,110 bytes, `ca87237931f7dab9…` | 365,110 bytes, `ca87237931f7dab9…` |
| `app/dist/flips-on-demand.html` | 271,204 bytes, `4a9841da4242349d…` | 271,204 bytes, `4a9841da4242349d…` |

The clone sits under the system temp directory and was left there; it holds
nothing but the repository.

The provider half was re-run on the same revision as well:
`node release/check-integration.mjs` passes the gateway, the signed result, the
scoped grant, the tampered-result refusal, the re-enrolment scope, and the
one-spend quota rule.

### The phone pass: the market becomes cards on a small screen (2026-09-26)

The desktop browser walk passed while the phone did not show the buy action: at
390×844 the pool table still scrolled sideways, with the delivery-desk column
clipped and `Buy keys` off screen. The market table now uses the same
stacked-card layout as the trades table on screens up to 560px, the buy button
is full width, and adjacent action links get a 12px gap - the desktop trades
view had rendered "ConfirmReceive" and "Sell one unitGive one unit".

Evidence:

- `npm run verify` walks the pool snapshot in a second 390×844 viewport: the
  document does not overflow (`overflow=0` on the market and trades views), the
  card carries Bundle, Epoch, Price, Units, the delivery desk and a full-width
  buy button, no page errors occur, and `pool-market-mobile.png` and
  `pool-trades-mobile.png` are written beside the desktop shots.
- The desktop walk still passes every check, and its trades screenshot shows
  the spaced actions.
- The app bundle is 271,528 bytes; the manifest is regenerated in the same
  package.
- A fresh clone at `6d0ec21` rebuilds both artefacts byte for byte: the module
  365,110 bytes `ca87237931f7dab9…` and the interface 271,528 bytes
  `ee8c3b04b2d94768…`, which is what the refreshed manifest records.

### The Plan B route and the capture clock, checked live (2026-09-26)

Two read-only checks against the live chain, on the current revision:

| Check | Command | Result |
|---|---|---|
| the explorer client still reads the public service | `FOD_LIVE_INDEXER=https://api.idena.io/api GOFLAGS=-modfile=go.local.mod go test -count=1 -run IndexerLive -v ./...` in `tools/fod` | pass: epoch 227 lists 158 identities, 107 of them with answers, state root `0x40a2a96a…`; identity `0x012eFb6c…` has 3 flips, 3 with a published status |
| the epoch-228 capture window is still the planned one | `curl -s https://api.idena.io/api/Epoch/Last` | epoch 228, `validationTime 2026-09-29T15:00:00Z`, `candidateCount 5`, state root `0x82ae4945…` |

The first says the Plan B envelope and pagination still match what this client
reads; it does not vouch for an explorer's answers, which the assignment
cross-check and the node-root comparison are for. The second confirms the
time-boxed input: the node that has to survive the lottery block must still be
live through the first block after `2026-09-29T14:55Z`. No node was queried in
this check; the live node-root comparison remains the one recorded earlier.

### Both capture candidates are live and synced, three days before the window (2026-09-26)

Read-only checks on the designated test host, each node asked through its own
loopback RPC with the key from its own datadir; the key was read into a shell
variable and never printed or written to the repository.

| Candidate | `dna_epoch` | `bcn_syncing` |
|---|---|---|
| IdenaAI 2, RPC `127.0.0.1:9129` | epoch 228, `startBlock 11350781`, `nextValidation 2026-09-29T15:00:00Z`, `currentPeriod None` | `syncing=false`, current == highest == 11,359,753, `wrongTime=false` |
| IdenaAI 3, RPC `127.0.0.1:9130` | the same | the same |

The two are independent datadirs on the same host and agree on the head, so
either can serve as the capture node; keeping both up through the lottery block
removes the single-process risk the earlier plan carried. The copy budget still
fits: the two chain directories measure 3.3 GB and 3.2 GB against 13 GB free on
the host's `/`, and the rehearsal's copy-and-delete procedure returns that
space. The positive capture signature (`seed_bytes=32`, a non-zero candidate
list, `li`/`ls` in the key census) remains unverifiable until the lottery block
itself; this check only establishes that the nodes are alive and in the right
epoch now.

### The CLI's help lists the money-out commands (2026-09-26)

The release runbook tells the operator to run `fod submit`, `fod refund`, and
`fod withdraw`, but `fod help` omitted all three: the commands exist in the
dispatch map and are tested, while the help text stopped at `prepare-deploy`.
The help now lists them, and the check is mechanical rather than eyeballed -
every command name the dispatcher accepts appears in the help text, and every
command the release runbook invokes exists.

Evidence: `./fod help` lists `submit`, `refund`, and `withdraw` with one-line
descriptions; `go vet` and the full `tools/fod` suite pass. The same inventory
now covers the runbook's flags: every whitespace-preceded `-flag` in
`release/README.md` resolves to a flag the CLI defines; the only tokens the
check leaves unresolved are the `cp -a`/`cp -al` flags in the capture
instructions.

### The spec's CLI table names the pool commands (2026-09-26)

The final stale-reference sweep found the last two offer-era entries in
`SPEC.md` 6.1: `prepare-purchase` and `prepare-delivery`. They are replaced by
`prepare-buy` - create the buyer's delivery key and prepare the pool purchase,
rechecking the quote and nonce - and the `serve` row now names the
delivery-proof fallback, which is where the removed `prepare-delivery` work
lives. The table also lists `prepare-fund` and `prepare-deploy`, the commands
the funding and deploy steps use. The sweep over `SPEC.md` and
`release/README.md` now finds no removed operation name; the remaining mentions
of the offer slice are dated history in the release record.

### The component docs name the pool surface (2026-09-26)

A repo-wide sweep past the dated logs found the last current-document holdouts
of the offer lane: `contracts/escrow/README.md` still advertised `registerOffer`,
`listBundle`, `fundOffer`, `cancelOffer`, and the offer views as implemented,
and named the pool's proof path `fulfillOrder`/`refundOrder`; the delivery
README pointed at the removed `prepare-delivery`; and a buy comment and a submit
test fixture still carried the old names. The escrow README now describes the
pool's supply and demand surface, the build's two seed hooks, and the rules the
contract encodes; the delivery README points at `fod serve`; the stale comments
and fixture strings are gone. The only remaining hit is intentional:
`tools/fod/fees_test.go` asks for an estimate for `listBundle` as its example of
an operation with no measured number.

Evidence: the sweep command returns only that negative test; `go vet` and the
full `tools/fod` suite pass.

### The exercise bundle's blobs are still fetchable (2026-09-26)

The seller's five epoch-228 flips were re-read through the live node, because
IPFS availability is the one mutable precondition the capture does not fix. The
staged CLI on the host (built before the offer-slice removal; the inventory path
is unchanged) was run against the seller's own node:

```
author=0xace93bd0… epoch=228 submissions=5
unique=5 duplicates=0 deleted=0 blobs_ok=5 blobs_failed=0 complete=false
node_verified=true
node    checked: a node confirmed all 5 anchors and the epoch's state root
```

All five CIDs are UnixFS/dag-pb, the importer layout the registration proof
already handles (`SOURCES.md` F-86, F-88). The completeness caveat is unchanged:
this route lists the explorer's record, and the committed-body sweep plus the
epoch's candidate list remain the capture's work. What this check adds is that
the flips have not become unavailable while the release waited.

## 5. Reporting rules for this record

- Add one dated block per package of work, not one per edit.
- Record commands or artifacts that a reviewer can re-run.
- Keep measured numbers, not adjectives: sizes in bytes, gas in units, time in
  minutes, memory in GiB.
- Record negative results explicitly. A failed feasibility check is a result.
- Do not record a capability as working because it was demonstrated with mocks,
  fixtures, or development-mode receipts.
- Paths and commands are recorded as they were when the entry was written. The
  evidence tool moved from `tools/fod-evidence` to `tools/fod` in `cd4c98d`,
  and its modes became subcommands: `fod sweep`, `fod inventory`, `fod epoch-db`,
  `fod epoch-keys`, `fod state-at`.

### Audit cadence (standing rule)

While no blocker is active, run a read-only repository audit every three hours
(the `goal audit` heartbeat, `FREQ=HOURLY;INTERVAL=3`). Each audit:

1. runs the cheap gates - `node release/check-integration.mjs`,
   `cd app && npm run verify`, the `tools/fod` and `spikes/harness` suites, and
   the cargo tests for anything changed since the last audit - and records the
   results;
2. checks documentation against implementation for drift: stale rules, stale
   hashes, claims the code no longer matches;
3. re-examines the weak or pending rows of the requirement-to-evidence matrix
   and any module changed since the last audit for new defects;
4. appends exactly one dated entry to the audit log below, naming what was
   checked even when nothing new was found, and, for each finding, its severity
   and a proposed fix.

Actionable findings are folded into the matrix and the package plan with their
proposal. Cases that need more capability than this session has - a different
model, deeper proof work, an authority this task does not hold - are written as
handover notes instead of guesses. If a blocker is active, the audit is skipped
and the turn stays quiet.

Recurring risk checklist, distilled from audit 1's findings, applied to every
module changed since the last audit:

- key material written to disk goes through the private writer (0600, and a
  rewrite tightens a file that already existed wider) and never to a predictable
  path in a shared directory;
- anything a key signs is built in-process or written by the process itself into
  a private directory, so no other local user can substitute it;
- public artefacts - market snapshots, prepared actions, censuses, logs, proof
  outputs - carry no private scalars;
- new or changed CLI commands are registered in `commands.go`, appear in the
  usage text and match the spec and the walk that exercises them;
- changed gas or fee figures come from live receipts, and the table keeps the
  highest figure recorded while the submit guard prices the size it signed.

## 6. Audit log

### 2026-10-03 — owner-requested bounded audit — SA-01..SA-16

Directly authorized audit despite the standing scheduled-audit blocker. Checked
the six gateway/utility/browser/CLI/release/escrow surfaces, focused adversarial
regressions, the existing Rust/Go suites and the integrated offline walk.
Found and patched eleven medium and five low local issues; the SA matrix and
section 3 record every finding, proposed control and closing regression.
The source and interface privacy boundaries were reviewed again after the fixes.
Partial assurance remains explicit: upstream dependency/cryptography review,
remote availability, canonical fair exchange and real-chain performance were
not qualified. Existing credential rotation is an authority handover below.
No new administrator, trusted scorer, hosted prover or operator dependency was
introduced.



### 2026-10-02 - finding triage - the CID fallback is already fail-closed; its positive case is now pinned too

Reported (security scan, pasted verbatim): an untrusted RPC can certify
substituted flip bytes as the committed CID, because `verifyBlobCID` was said to
accept the node's `ipfs_cid` answer when local reconstruction fails, with the
bytes and the assertion coming from the same endpoint before a buy is prepared.

Disposition on the current tree: **already closed, and the production code did
not need a change.**

- `tools/fod/cid.go` - `verifyBlobCID(cid, blob, _ cidComputer)` ignores the
  peer callback entirely (`_`), accepts only locally reconstructed profiles
  (`cidRawSha256` for CIDv1/raw, `unixfsCID` for the pinned CIDv1/dag-pb
  UnixFS importer), and otherwise returns the fail-closed note
  `unverified: these bytes do not reproduce the CID locally under a supported
  importer profile`. `nodeCIDComputer` is documented as diagnostic information
  and its answer is never consumed for authorization; `rg '"ipfs_cid"'` finds
  exactly one non-test use, inside that diagnostic helper.
- The attack path is pinned negatively by
  `TestCIDVerificationNeverTrustsTheServingPeersAttestation`
  (`tools/fod/availability_check_test.go`): for a dag-pb CID and an unsupported
  CID it hands `verifyBlobCID` a callback that would return the requested CID
  and requires both a refusal **and** `calls == 0`, i.e. the serving peer is
  never asked to authorize its own bytes.
- Added in this pass, because the report also requires that real vectors keep
  verifying: `TestUnixFSCIDMatchesTheNodesOwnImporter`
  (`tools/fod/unixfs_test.go`) now also runs every one of the five recorded
  node-answered vectors (`SOURCES.md` F-88: one raw, four dag-pb, 262,144 to
  614,400 bytes) through `verifyBlobCID` and requires acceptance there while the
  same lying callback stays unused. That closes the gap the report's test list
  pointed at - the positive path was only covered through `unixfsCID` directly,
  not through the check a buyer actually runs.

Checks: `cd tools/fod && GOFLAGS=-modfile=go.local.mod GOPROXY=off go test
-count=1 ./...` - ok, 35.7 s; `gofmt -l` clean on the changed test. No
production code, keys, fees or prepared actions were touched.

### 2026-10-02 - lab run - the buyer's confirm was refused by the contract's own solvency guard; refund is the working exit

Scope: the isolated play-money lab (`fod-v3-cer`, RPC `127.0.0.1:19095`,
`fod-v3-lab3`, RPC `127.0.0.1:19097`), contract
`0xd53b3032139c33ab87bd9bac7f0871658805c60f`, and the harness replay of the
exact on-chain storage. Read-only against the chain; one refund and one
0.01 iDNA `addReserve` were broadcast on the lab, both from play money.

What the run established:

1. **The deployed module is the current source, byte for byte.** The chain's
   `codeHash` is `0371a877a4f9ec3f3a810c5f7807b097e8c2b3e22f5148e75a0f449b295d5434`.
   The code read out of a datadir snapshot (`state.StateDbKeys.ContractCodeKey`)
   is 388,019 bytes with exactly that hash, and it is byte-identical (`cmp -s`)
   both to `release/artifacts/fod_escrow.wasm` and to a fresh
   `cargo build --release --target wasm32-unknown-unknown` lowered with
   `wasm-opt --llvm-memory-copy-fill-lowering -O3`. There is no stale artifact
   in this deployment; an *unlowered* build is what the chain refuses, and the
   deploy attempt in this run failed as `Bulk memory operation detected:
   MemoryCopy` before any code landed.
2. **`RuntimeError: unreachable` is a refusal, not a crash.** `error_reply`
   panics on purpose (`contracts/escrow/src/lib.rs`), so every refusal is a
   failed call with the code discarded. Every existing method refuses at the
   ~1,000-gas floor when given junk arguments; methods that do not exist report
   `method is not found`, which is how the two are told apart.
3. **The buyer's confirm was refused by `not_enough_balance`.** Seeding the
   harness with the exact on-chain records (`pool/`, `porder/1`, `popen/`,
   `pfee/1`, `pdeliv/1`, `reserve0`, `pending0`, `units0`,
   `reserve-credit/<author>`) and the artifact above, then calling `confirm(1,
   ciphertext)` gives `RuntimeError: unreachable` at a 2 iDNA contract balance
   and success at 3 iDNA - nothing else changes. The chain's own numbers were
   `reserve0 = 1e18` credited to the author, `pending0 = 2e18`, contract balance
   `2e18`: the guard `contract_balance() < reserve + pending + credits` in
   `settle_order` refuses, so the order was stuck in `delivered` with the buyer's
   escrow still inside.
4. **The operator-free exit works.** `fod refund -request … -prepared …` +
   `fod submit` from the buyer's own nodekey broadcast
   `0xf9fd11aa0d39e49e0ed02f9ced47cceac1c9b0e92eab74c28bcbcd2d0afaf17b`
   (`bcn_txReceipt`: `method refund`, `success true`, `gasUsed 14473`,
   `txFee 12.1`). State-tree reads of two datadir snapshots: contract
   `2e18 -> 0`, buyer `812.88 -> 658.05 iDNA` on the same read that shows the
   buyer's nonce `3 -> 4` - the 2 iDNA escrow came back and the 156.83 iDNA
   difference is the transaction's own cost.

Findings:

- **F1 high - a successful `addReserve` left the reserve unbacked.** Block 13239
  carried `addReserve` tx `0x6a5983c7aafc…` with `amount 1` iDNA and a
  `success true` receipt, and the contract's storage did record the reserve and
  the donor credit; but the earliest readable state version (height 13339) shows
  the contract's balance at 0, and the balance only ever became 2e18 when the
  buyer's `buy` landed at 13355. A fresh `addReserve` of 0.01 iDNA from the same
  account today (`0x3d432c845f9d…`, `success true`) *did* raise the contract
  balance to 0.01, so contract-call value transfers are not broken in general.
  The 1 iDNA of block 13239 is unexplained: nothing in the contract can spend
  without `create_transfer_promise` and no such call ran before height 13339.
  Proposed next step: on a fresh lab instance, read the contract's balance per
  height around its first `addReserve` and compare with the sender's balance
  delta. If the value transfer is dropped by the node, that is a node-level
  finding and belongs in section 7, not in a contract patch.
- **F2 high - a contract whose balance does not cover its tracked totals cannot
  settle, and the guard is a trap.** The refusal is correct (it protects other
  liabilities) but the *cause* - an unbacked reserve credit - is invisible to
  both the buyer and the desk. Proposed: `addReserve` should self-check after
  the deposit, e.g. refuse unless `contract_balance() >= reserve_total +
  pending_total + credit_total`, so an unfunded credit cannot be written at all
  and the reserves stay provably backed.
- **F3 low - refusal reasons cost a full replay to diagnose.** `error_reply`
  discards its code by design. Keep the trap, but consider a debug-only event
  carrying the code so a wedged order can be diagnosed from the chain alone.

Disposition: the refund exit is proven on chain; the fresh deployment of the
same, now reproducible artifact is prepared (its lowered build is byte-identical
to `release/artifacts/fod_escrow.wasm`); F1 and F2 are folded into the package
plan. Nothing here changes the claim set: paid supply stays disabled, and this
run used only local play money.

Follow-up in the same session - the guard, and two fresh deployments:

- **F2 is fixed.** `addReserve` now refuses when the contract's own balance does
  not cover the deposit plus every tracked liability
  (`reserve_total + amount + pending_total + credit_total`), which is sound
  because the node credits the contract before the guest runs and rolls the
  transfer back when the call fails (`blockchain/blockchain.go`,
  `AddBalance(contractAddr, amount)` before `vm.Run`). The new code is
  `unfunded_deposit` in `contracts/escrow/src/lib.rs`.
- **The regression pins it.** `spikes/harness/escrow_reserve_test.go`,
  `TestPoolReserveDepositMustBeBacked`: the runtime reports the amount but the
  balance is not credited - the call must abort and write nothing - and the same
  deposit *with* the balance credited is still accepted. It fails on the old
  code and passes now.
- **Checks run once, on the delivered state:** `cd spikes/harness &&
  GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok (7.1 s, the new
  case included); `cd contracts/escrow && cargo test --locked --offline` - 16
  passed. The rebuilt artefacts (production 389,510 bytes; test-hooks 397,099
  bytes; both through `wasm-opt --llvm-memory-copy-fill-lowering -O3`) deploy
  cleanly: production contract `0xc4032046e752eb9871b62ee7b8e20803cd93d889`
  (code hash `c4677464162f852734f60f1bab1580322424833709e213bb7789b374c1072bbe`,
  tx `0xfd56c057…`, `success true`, `txFee 38967.5`) and the test-hooks module
  `0x450d2d5535f3c1b3ebfe00aa44f3ed872a4a4a55` (code hash
  `418566df3f043747a3301d30529bb247090438520da868ba568cd038c950a940`, tx
  `0x8a94ddf1…`, receipt `success true`).
- **Known limit of the next step.** `registerBundle` proves a journal that
  carries the contract address (`/tmp` artefact `register.json`: journal
  `00000003d53b3032…`), so the fresh contract needs its own registration proof -
  the measured cost of that proof in this lab is ~989 s (16.5 min) from
  `registration.json`. That is pre-delivery work by design; the three-block
  window only has to cover `deliver` and `confirm`. Until that proof is run, the
  seeded test-hooks contract above is the way to exercise settlement on the
  current code without mistaking a seeded bundle for a proven one.
- **Coherence note for the release package:** with the guard in the source,
  `release/artifacts/fod_escrow.wasm` (03:45) no longer matches the source; the
  release artefact and `release/MANIFEST.json` have to be regenerated with the
  hash above before any claim that binaries and manifest form one set.

Lifecycle on the guarded contract (`0xc4032046…`, code hash `c4677464…`) - the
first end-to-end round trip that settles instead of wedging. Every step below is
a real transaction on the lab chain; the desk ran the current `fod serve`
against the same address:

| step | tx | receipt |
|---|---|---|
| deploy, lowered build | `0xfd56c057…` | `success`, `txFee 38967.5` |
| `registerBundle`, sealed proof bound to this address | `0xbb4a2de1…` | `success`, `gasUsed 1960342` |
| `donateUnit` (author) x2 | `0x0b32e06c…`, `0xfaed1912…` | `success`, `txFee 15.6` each |
| `addReserve` 2 iDNA, the guard's happy path | `0xab16cd38…` | `success`; `reserve0 = 2e18`, `reserve-credit = 2e18`, contract balance `2 iDNA` |
| `buy` - 2 iDNA quote + 1 iDNA fee, after the buyer's own re-check | `0x13c0d557…` | `success`; order 1, `fundedAtBlock 13539`, `deliveryDeadlineBlock 13542` |
| `deliver` (desk) | included at height 13540 | inside the window; ciphertext 219 bytes |
| buyer `receive` | - | ciphertext `sha256 6e5ac2d7…`, inventory matches the bundle, 1 flip decrypted |
| `confirm` | `0x0d356634…` | **`success`**, `gasUsed 21586` |
| `withdraw` (named host takes the fee) | `0x8251508c…` | `success`, `txFee 12.2` |

Settlement left `pending0 = 0`, the fee list cleared, `credit/<host> = 1e18`,
and the contract at 3 iDNA against exactly `reserve 2 + credit 1`; the
withdrawal then took the credit out and left the contract at 2 iDNA, again
exactly the reserve. The accounting invariant holds at every step on a live
chain, and the reserve stays backed - the failure mode of the 2026-10-02 wedge
is not reproducible on this artefact.

Measured on this run, and worth keeping honest: transaction **inclusion** on
this lab took ~60 s (the buy was broadcast at head 13536 and included at 13539),
so the funded-to-deadline window of three blocks is about three block
intervals, not three seconds. The desk's delivery landed at 13540 (one block
after it saw the order), and the buyer's own check plus confirm were run
afterwards at 13541-13547 - the buyer's path from delivery to confirm took
~2 s of tool time, so the ladder is dominated by inclusion latency.

Both lab nodes read that same state: at block 13549 the marketplace digest is
`336d01288c7bed79e9572efbef4631d01dfa636513c20ff14a5419e86e8d72e7` from the
`fod-v3-cer` node (19095) and from the `fod-v3-lab3` node (19097). That is two
independently configured node processes on one host, so it is node agreement,
not independent-operator evidence - the second host stays a section 7 item.

Refund branch on the same guarded contract, with the desk deliberately stopped
so the order could stay undelivered: a third `donateUnit` (`0x64c65236…`) put
the listing back to two units, the buyer bought order 2 (`0xd4ac938b…`, funded
at 13556, deadline 13559 - the buyer's own pre-payment re-check ran first), no
delivery followed, and at head 13559 the buyer's own `fod refund` was broadcast
as `0xd216de52…`: `success`, `gasUsed 14473`, `txFee 12.1`. Afterwards
`pending0 = 0`, `reserve0 = 2e18` unchanged, the contract at exactly 2 iDNA (the
reserve), and the buyer up by the whole 3 iDNA escrow minus the transaction's
own 156.83 iDNA. The exit needs no operator, no approval, and no cooperation
from the desk.

Cost note from the same run, for the lab's economics: contract calls on this
chain cost the caller ~148 iDNA of gas headroom each (the refund's max fee
requirement was `147890000000000000000`), and the buyer's account ran out at
57 iDNA after one purchase and one confirmation. It was topped up with a plain
transfer from the author's own account (`0xba81afff…`, 2000 iDNA) - the CLI has
no transfer mode, so that needed a throwaway helper that was deleted again.
Any repeat of this ladder must budget per-call gas, not just the purchase
amount.

Release coherence, rebuilt against the guarded source: `node
release/build-manifest.mjs` re-ran the whole promotion path - cargo build, the
`wasm-opt` lowering, the gatekeeper check (`forbidden operators: 0`), the export
surface check (`19 published names, no test hooks`), the proof-tuple
verification in `release/check-proof-builds.mjs`, and the self-contained
interface check (`remote tags: 0`) - and only then replaced the artefacts.
`release/MANIFEST.json` now records revision `1462782b8f15` with
`worktreeDirty: true` (the tree is uncommitted), the escrow module at 389,510
bytes `sha256 ac97c56ce6362868…` and the interface at 302,139 bytes
`a67c7903dbd44bd9…`; the recomputed digest of the promoted module matches the
manifest, and that module is **byte-identical** to the one deployed on the lab
as `0xc4032046…`. `node release/check-integration.mjs` then reported
`ALL CHECKS PASSED` (74.3 s wall). The earlier coherence gap - a release
artefact older than the source - is closed for this revision.

Session-to-utility journey, for the record, on this revision: the browser walk
inside `release/check-integration.mjs` starts **both services as real
processes** - `gateway/src/main.ts` against a temporary pool and config the walk
writes (its own short window and its own application key), and
`utility/src/main.ts` on `:8788` against a temporary policy that pins the
gateway's key, the session digest and that window. The browser then enrolls with
a wallet binding (`personal_sign` over the gateway's challenge), answers both
challenges (the example flip with the sequence choice and the typed answer),
commits every answer to the utility **while the window is open**, waits for the
close, reads the provider-signed result, has the utility recompute the score
from the committed answers rather than from the provider's verdict, and reads
the grant; the same walk checks that a tampered result is refused and that a
re-enrolled participant is a new scope with its own quota. Fixture answers for
the example pool are c1 = `right` (second sequence) and c2 = `second answer`
(the shipped pool scores exactly those two digests).

What that does **not** prove, and must not be read into it: the utility is still
an operator that decides grants and can refuse service (H-3), the session
content is the example pool rather than chain-derived inventory, and the
answers rest on the pool's committed digests rather than on canonical chain
state. Two services on one host are also not independent-operator evidence
(H-1). Treat the walk as proof that the supported journey runs and that the
scoring is recomputed independently of the provider, not as proof that the
service side has become permissionless.

### 2026-09-30 - audit 8 - the market digest re-read; two findings, both fixed

Scope: what changed since audit 7 - `tools/fod/market.go`,
`tools/fod/market_test.go`, the `SPEC.md` 4.5 row for `MarketplaceState` and the
two rows that cite the digest - plus the matrix's weak rows and section 5's
recurring risk checklist.

Gates on this revision:

- `time node release/check-integration.mjs` - ALL CHECKS PASSED, 18 checks,
  74.4 s wall (this also re-ran the browser walk, which is the app's suite).
- `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok,
  17.6 s; `gofmt -l` clean on the changed files.
- `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` -
  ok, 6.6 s.
- `cd contracts/escrow && cargo test --locked --offline` - 15 passed.
- `cd proofs/session-core && cargo test --locked --offline` - 19 passed.
- Unchanged since audit 7 and re-run there: `app` 13 unit cases and `utility`
  36; both typechecks were clean then and neither package changed here.

Findings, both in the digest code added after audit 7, both fixed:

- **F1 low (silent failure in a verification aid).** `marketStateDigest` returned
  an empty string if the encoder ever failed, and `runMarket` would print an
  empty `state digest` - two operators could then "agree" on nothing, which is
  the opposite of what the field is for. Fixed: `runMarket` refuses with a named
  error (`the state could not be encoded for its digest`) instead of printing
  one.
- **F2 low (test did not cover what the digest is for).** The digest's cases
  mutated only the block height, the code hash and the contract, so a digest
  that ignored listings, orders or credits would still have passed - exactly the
  state two operators compare. Fixed: the test now also mutates a credit, a
  listing and an order, and keeps the file round-trip case.

Risk checklist on the changed module: `fod market` is read-only, holds no keys,
writes only the `-out` snapshot it already wrote (public state, 0644), and no
fee or gas figure is touched. Drift check: `SPEC.md` 4.5 documents the digest
and its canonical form; the interface's parser deliberately ignores the extra
field (it is an operator comparison, not a client input); the section-3 entry
and the matrix row carry the measured digest (`11968c6e...` at block 7254) and
the stated limit that the digest is a comparison, not a proof about a node. No
other document cites the market schema.

Disposition: F1 and F2 are fixed and pinned; no new matrix row was needed. With
this, every local row the audits and the package plan can close is closed: what
remains in the acceptance report and section 7 needs inputs this task does not
hold - a second operator host (`H-1`), mainnet authority (`H-2`), broadcast
authority for a delivery-guest change and its fresh receipts (`H-5`), the
seller's own epoch vault (`G2`), the payment/custody decision (`G5`) and the
executive decision on an independently timestamped commitment that would close
`H-3`'s residue.

### 2026-09-30 - audit 7 - the five new packages re-read; four findings, all fixed

Scope: the modules changed since audit 6 - `app/src/{score.ts,provider.ts,faq.ts}`,
`app/src/views/Session.tsx`, `app/test/score.test.ts`, `app/verify.mjs`,
`utility/src/{score,commitments,policy,access,results,server}.ts`,
`utility/test/access.test.ts`, `gateway/src/{session,routes}.ts`,
`gateway/test/gateway.test.ts`, `tools/fod/{sharednode,sharednode_test,main,commands}.go`
and `release/check-integration.mjs` - together with the matrix's weak rows and
section 5's recurring risk checklist. Read-only except for the four fixes below.

Gates on this revision:

- `time node release/check-integration.mjs` - ALL CHECKS PASSED, 18 checks,
  74 s wall (the walk's first window is 30 s and the second 60 s, which is what
  the commitments inside them need).
- `cd app && npm run verify` - 60 checks, ALL CHECKS PASSED; `cd app && npm test`
  - 13 of 13.
- `cd utility && npm test` - 36 of 36 (up from 35 with the store-bound case);
  `cd utility && npm run typecheck` clean.
- `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok,
  17.4 s, including the new usage test; `gofmt -l` clean on the changed files.
- `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` -
  ok, 5.9 s.
- `cd contracts/escrow && cargo test --locked --offline` - 15 passed.
- `cd proofs/session-core && cargo test --locked --offline` - 19 passed.
- `cd app && npm run build` - interface 302,236 bytes `e3922fcf...`, identical
  on rebuild; `release/MANIFEST.json` regenerated for it.

Findings, all fixed in this pass:

- **F1 low (CLI usage drift).** `commandModes` carries 43 spellings; the usage
  text named 34 and silently omitted `sweep`, `inventory` and `export-session`,
  so `fod inventory` worked but `fod help` never mentioned it. Fixed: the usage
  names every registered spelling (`sync / sweep`, `inspect / inventory`,
  `export / export-session`), `printUsage` now prints a `usageText()` a test can
  read, and `TestUsageListsEveryRegisteredCommand` fails when a command is
  registered but unnamed - or when the text names something unregistered.
- **F2 medium (availability).** `AnswerCommitments` bounded the entry count
  (10,000) but not the answer text; with the 32 KiB body cap that is up to
  ~320 MiB resident from a flood of freshly generated wallets. Fixed: the store
  takes `{maxEntries, maxBytes}` (default 8 MiB), counts each entry's answer
  text, refuses `busy` when the budget is spent, subtracts the bytes again on
  expiry or replacement, and still lets a wallet that already committed replace
  its own entry.
- **F3 low (stale current-revision claims).** The living acceptance report still
  cited interface 293,805 bytes `b5b3d5aa...` and "13 of 13", and the AH-23 row
  read as if `MANIFEST.json` carried that hash now. Fixed: the report carries the
  current numbers (302,236 bytes `e3922fcf...`, 18 gate checks, 60 walk checks)
  and the AH-23 row states the closure revision's measurement instead of a
  current one.
- **F4 low (correctness).** `AnswerCommitments.get` did not check retention, so
  an entry that outlived `closesAt + 1h` could still reach a decision whenever
  no later `record` swept it. Fixed: `get` takes the clock, forgets an expired
  entry and returns null; the store test pins it.

Risk checklist on the changed modules: the new writers are the `fod shared-node`
outputs - the export and pool keep the 0600/0700 private path and the connection
file is public by design (node URL, author address, custody, pool name, no
scalars); the new signing paths build their payloads in-process from data both
sides recompute (the commitment text is over the canonical answer digest); the
public artefacts (`/policy`, the commitment response, the signed transcript)
carry only digests, timings and public identifiers; `shared-node` is registered,
appears in the usage text, matches `SPEC.md` and is named by the walk; no fee or
gas figure changed. Documentation against implementation found no further drift:
the utility README's decision order, refusal reasons and test count match the
code, `SPEC.md` 8.3 describes the commitment path the utility implements, and
the G3, H-3 and clause-6 rows match what the tree does.

Disposition: F1-F4 are fixed and pinned here; no new matrix row was needed. The
handover notes are unchanged in kind: H-1 (second host), H-2 (mainnet
measurement), H-4 (chain guarantees) and H-5 (delivery journal domain, whose fix
regenerates the pinned proof), with H-3 now recording its resolution and the
residue (the utility's own clock and store, answer sharing or guessing below the
chosen threshold, and the operator's pin).

### 2026-09-30 - audit 6 - two of the three audit-2 follow-ups were not in the code, and the CORS wildcard default is closed

Scope: a read-only pass over `app/src/views/*`, `app/verify.mjs`, `utility/src/*`
and `utility/test/*` at revision `1462782` (125 dirty entries), re-checking the
audit-2 findings F1-F3 against the source instead of against the entry that
closed them, together with the audit-4 note that the utility's CORS answer still
defaults to `*` when a policy omits `origins`.

Findings, all low, all closed by this pass:

- **F2 was recorded as fixed, but the code did not match.** The 2026-09-29
  audit-2 entry says "the generated connection note now names both the gateway
  and utility URLs"; at this revision `app/src/views/GateSetup.tsx` wrote
  `replace the gateway and utility URLs with your own hosts`, which names the two
  roles and neither URL. Fixed: the generator hoists `gatewayUrl`/`utilityUrl`
  and builds the note from them, and `app/verify.mjs` now asserts that the
  downloaded connection's note contains both URLs.
- **F3's wildcard default was still open.** The CORS answer was policy-driven,
  but `options.policy.origins ?? ["*"]` let any policy that omitted the field
  silently allow every browser origin. Fixed: an omitted list now sends no CORS
  header at all (the methods and headers fields go only with an allowed origin),
  the reference testbed keeps its explicit `["null"]` / `["*"]` forms, and
  `GET /policy` publishes `origins`, so a host can confirm the pin the running
  endpoint enforces rather than the file it believes it passed.
- **F1's guard was presentational.** The audit-2 fix disabled the button; the
  `claim()` call path itself did not re-check. Fixed: one `claimable` predicate
  (browser-verified signature and `outcome: passed`) now gates both the button
  and the call, so the two cannot drift apart.

Evidence on this revision:

- `cd utility && npm test` - 23 of 23 (four new cases pin the origin rule: a
  listed origin is echoed with `Vary`, another origin gets nothing, an omitted
  list gets nothing, an explicit `["*"]` is kept, and the published pin matches
  the enforced one).
- `cd utility && npm run typecheck` - clean.
- `cd app && npm run build` - interface 293,875 bytes
  `d33fc6a80fbcbbc70798f208b5b181262fb1b27353b621ca06afbebe433767a3`.
- `cd app && npm run verify` - ALL CHECKS PASSED, including "an unverified result
  cannot claim the test utility" and the new connection-note assertion.
- `node release/check-integration.mjs` - ALL CHECKS PASSED.
- `node release/build-manifest.mjs` - regenerated: interface 293,875 bytes
  `d33fc6a8...`, module unchanged 380,347 bytes `0769bbf0...`, 0 forbidden
  operators, 0 interface remote tags, revision `1462782` dirty.

Disposition: no new Package H row. These are the audit-2 F1-F3 items plus the
audit-4 CORS note, all low, now closed against the source and pinned by tests;
`docs/SPEC.md` 8.3 states the origin rule. The `origins` pin governs browser
reads only - the endpoint stays signature-gated for every caller - and the
provider-trusted scoring boundary stays H-3. No new handover note.

### 2026-09-30 - audit 5 - the second security scan's three findings, folded into Package H

Scope: the sixth-revision goal (section 0), the audit cadence (section 5) with
its recurring risk checklist, the whole working tree at `1462782` (118 dirty
entries, unchanged), the modules changed since audit 4 (P5a/P5b, the AH-17
correction and the AH-18 resolution), and the second Codex Security standard
scan of this tree: scan `e87b560e-75bd-43ac-9f26-22043435ff6a`, started
2026-09-30T10:45Z, completed 2026-09-30T11:05Z, snapshot digest
`codex-security-snapshot/v1:sha256:073f89a9...`, report under
`<internal-state-path>`.
That scan reports 3 findings (1 medium, 2 low, all confidence high, coverage
partial) and confirms the repairs the first scan demanded.

Gates on this revision:

- `node release/check-integration.mjs` - first run FAILED at "the offline
  interface walk passes": the spawned walk exited non-zero with a bare
  `Node.js v24.18.0` tail while the registration-v2 cargo test compiled in
  parallel. Re-run alone: ALL CHECKS PASSED (13 checks). `cd app && npm run
  verify` alone: ALL CHECKS PASSED. The gate keeps only the walk's last two
  output lines, so the failure's cause is not diagnosable from the gate
  itself; AH-22.
- `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok,
  14.5 s.
- `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` -
  ok, 6.1 s.
- `cd contracts/escrow && cargo test --locked --offline` - 15 passed (three
  pre-existing warnings: an unused `PanicInfo` import, a needless `mut` in
  `sellUnit`, and the unused `key_raw` helper).
- `cd proofs/session-core && cargo test --locked --offline` - 19 passed.
- `cd proofs/registration-v2/host && cargo test --locked --offline --test
  sealed_artefact` - 1 passed.

Documentation/hash drift: `release/MANIFEST.json` (revision `1462782b8f15`,
dirty, generated 2026-09-30T00:45Z) matches the tree byte for byte - interface
293,244 bytes `03aa5573...`, module 380,347 bytes `0769bbf0...` - and the
matrix and final report carry the same figures; `release/README.md`'s "13
checks" matches the check-integration run; `tools/fod/fees.go` carries the live
AH-18 confirm measurement (2,035,700 wasm / 20,357 user gas); the new commands
(`session-pool`, `prepare-buy`, `prepare-confirm`, `reveal`, `watch`) are
registered in `commands.go` and named in the usage text; neither release
artefact contains `BEGIN PRIVATE KEY` or `privateKeyPem`. No stale current
claim found.

Risk checklist on the modules changed since audit 4: the key-material item
found AH-20 below; the signing path (the watch's 0700 temp directory and
`writePrivateJSON`), the public-artefact check (no private scalars in the built
HTML or module) and the fee item (the table carries the live receipt) are
clean.

Findings, each folded into `SPEC.md` 10.5 and the matrix rows above:

- **AH-19 medium (scan finding 1).** `refund` accepts `POOL_ORDER_DELIVERED`
  after `funded+3` while `fulfill` and `reveal` refuse at the same boundary,
  and `fod serve` plans only deliver/beacon/fulfill, so a buyer who decrypted
  the delivery and never confirms keeps the content and takes the payment and
  fee back; the seller takes a strike. Proposed fix: give the desk an
  in-window, buyer-independent settlement (the prepared `reveal` submitted as
  the deadline approaches, or an equivalent), keeping the refund for
  undelivered orders.
- **AH-20 low (scan finding 2).** `fod receive`, `fod export` and
  `fod session-pool` write decrypted flip material and the session answer salt
  through `os.WriteFile(..., 0o600)`, which does not tighten a pre-existing
  wider file; `writePrivateJSON` already carries the fix for the audit-1
  writers. Proposed fix: route the three through it and pin the rewrite case.
- **AH-21 low (scan finding 3).** The tree carries the operator's live Idena
  identities in `docs/PROGRESS.md` and fixtures, and leaves
  `tools/fod/sweep.json` untracked and not ignored. Proposed fix: synthetic
  addresses in tracked content before any publication or share, and
  `.gitignore` entries for the CLI's default outputs.
- **AH-22 low (this audit).** `release/check-integration.mjs` keeps only the
  last two lines of a failing walk; proposed fix: capture the last ~40 lines
  (or the full output) while still failing the gate.
- **AH-23 low (this audit).** The harness build and the manifest's release
  copy both target `contracts/escrow/dist/fod_escrow.wasm`; the committed file
  already carries `seedPoolBundle`/`seedPoolOrder`, the worktree rebuild adds
  `seedPoolCovered`, and a seeded order can settle against the contract's real
  balance, while the release artefact exports only the published surface
  (`spikes/harness/escrow_no_admin_test.go`, green in this audit). Proposed
  fix: separate the harness artefact's name from the release path (or stop the
  release copy into `dist/`) and name the verified artefact in the release
  check.

Disposition: AH-19..AH-23 are written into `SPEC.md` 10.5 and the matrix above
with their proposals; no fix is applied in this read-only audit, and the
working tree stays at its 118 dirty entries. The scan's remaining observations
are notes, not rows: the accepted AH-14 delivery-domain limitation stands as
recorded, and the utility's CORS still defaults to `*` only when a policy omits
`origins` (pin exact origins before a production host). No new handover note:
H-1 to H-5 are unchanged, H-3 (participant-verifiable scoring) stays the
hard-case handover, and AH-19..AH-23 are local, bounded work.

### 2026-09-30 - audit 4 - the Codex Security scan of the working tree, folded into Package H

Scope: the repository-wide Codex Security standard scan of this working tree
(scan `aab316d1-e176-47df-a708-5a3b9b3e6c90`, started 2026-09-29T20:51Z,
completed 2026-09-29T21:14Z, base revision `1462782`, snapshot digest
`codex-security-snapshot/v1:sha256:a7646f2b...`). Report:
`<internal-state-path>`.

Result: 11 findings, all confidence high - 4 high, 6 medium, 1 low. Coverage
partial (360-file inventory, 8 worklist rows closed); artifacts reviewed were
`contracts/escrow/src`, `proofs/session-core`, `proofs/delivery`, `tools/fod`,
`gateway/src`, `utility/src`, `app/src`, the release tooling and the focused
harness and service tests. No live-chain, mainnet, remote-host or
proof-generation action was taken, and the dirty app distribution was verified
but not rebuilt, so source-to-dist equality was not established.

Mapping into the package plan: finding 1 -> AH-13; 2 -> new AH-17; 3 -> AH-11;
4 -> AH-1/AH-2; 5 -> AH-3; 6 -> AH-8; 7 -> AH-12; 8 -> AH-10; 9 -> AH-9;
10 -> AH-6/AH-7; 11 -> AH-14. Ten of the eleven severities agree with the
audit 3 rows. The two disagreements: the scan rates the deadline race (AH-13)
and the cleared-listing donation dilution (AH-11) as high fund-loss issues on
this snapshot; both rows keep their audit 3 wording until a fix is proposed.

Disposition: AH-17 is written into `SPEC.md` 10.5 with its matrix row above;
the other ten stay in their existing rows with their stated closing evidence,
now confirmed by an independent scan. No new external blocker: H-1 to H-4 in
section 7 are unchanged, and no fix from this scan has been applied.

### 2026-09-30 - audit 3 - the session gate leaks its answer key; claim, binding and default gaps

Scope: the third-revision working tree (dirty, uncommitted; the last commit is
`1462782`), `SPEC.md` 10.3/10.4, the audit cadence in section 5, and the
changed gateway, utility, app, escrow, proof and release surfaces.

Gates on this revision (2026-09-30):

- `node release/check-integration.mjs` - ALL CHECKS PASSED (11 checks).
- `cd app && npm run verify` - **did not run.** `import("playwright")` fails
  with `Cannot find module 'playwright-core'`: `app/node_modules/playwright` is
  a symlink into `/tmp/fod-pw`, and `playwright` is not declared in
  `app/package.json`. Finding AH-16. Audit 2 ran this walk on a machine where
  the /tmp link resolved; the gate is not reproducible from the declared
  environment.
- `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok,
  15.4 s.
- `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` -
  ok, 5.9 s.
- `cd contracts/escrow && cargo test --locked --offline` - 15 passed.
- `cd proofs/session-core && cargo test --locked --offline` - 19 passed.
- `cd proofs/registration-v2/host && cargo test --locked --offline --test
  sealed_artefact` - 1 passed.

Checked and already closed in this tree, so not re-reported from the baseline
reports: caller-asserted identity is refused and a policy that requires
identity is refused as unsupported (`utility/src/access.ts:150-163`); donation
and the beacon are author-only (`contracts/escrow/src/lib.rs:1349,1474`); a
buyer's payment is pending escrow and not spendable reserve
(`contracts/escrow/src/pool.rs:47-115`); confirmation binds the digest of the
stored ciphertext (`lib.rs:2055`); the version 3 statement publishes the
inventory commitment and the network digest
(`proofs/session-core/src/statement.rs:29-46,198,294`) and the buyer recomputes
the CID commitment against chain state (`tools/fod/pool_buy.go:315-330`);
audit 2's F1-F3 are in the working tree (the claim button requires
`verification === "verified"`, the connection note names both URLs, and the
utility CORS follows the policy origins).

Findings, each folded into `SPEC.md` 10.5 as package H and into the matrix rows
above:

- **AH-1 high.** `gateway/src/routes.ts:213` returns `{correct}` per answer
  while `gateway/src/session.ts:107-145` enrolls a fresh `participantId` on
  every request (address optional, no per-address rule). A caller learns every
  answer from a wrong submission, re-enrolls, and obtains a passing signed
  result without solving. Fix: no correctness before close; a gate session
  requires the address binding and one live enrollment per address.
- **AH-2 high.** `gateway/src/pool.ts:sessionContentDigest` commits to each
  challenge's `answerDigest`, and the pool publishes the challenge options, so
  a small binary pool is offline-enumerable from the published value. Fix: a
  per-session secret salt revealed only after claims close.
- **AH-3 medium.** `gateway/src/wallet.ts:31-40` signs
  `fod-enroll:<sessionId>:<address>:<ms>`: no server nonce, no provider or
  origin binding, five-minute window. A captured signature replays for fresh
  enrollments and at another gateway with the same session id. Fix: a one-time
  nonce consumed at enrollment, with provider context in the signed text.
- **AH-4 medium.** Rate limits key on `req.socket.remoteAddress`
  (`gateway/src/http.ts:174`, `utility/src/server.ts:91`), so every client
  behind the README's recommended TLS proxy shares one bucket. Fix: a
  trusted-proxy configuration or proxy-side limits, documented.
- **AH-5 medium.** `gateway/src/session.ts:196-218` evicts the oldest live
  participant once the table is at `MAX_PARTICIPANTS`, and `participantCount`
  (the public lobby read) calls it. A flood can log out honest participants.
  Fix: expire-only on reads; refuse new enrollment at capacity.
- **AH-6 high for the faucet/dapp recipe.** `utility/src/server.ts:95-102`
  passes only `{result}` into `decideAccess`; nothing proves the claimant
  controls `result.participantAddress`, so a copied result yields a grant
  bearing the victim's address (and, for a wallet-scoped benefit, a usable
  copied entitlement). Fix: a one-time utility nonce signed by the bound
  address before a wallet-scoped grant.
- **AH-7 medium.** `utility/src/access.ts:scopeKey` serializes only utility,
  provider and participant id while `GrantScopeKey` carries the address, and
  `InMemoryGrants` forgets everything on restart. The comment "re-enrolling
  with the same address cannot reset the grant" is not true of the code. Fix:
  an explicit scope kind, key by address where wallet-scoped, and either
  persistence or an honest statement of the reset.
- **AH-8 medium.** `app/src/provider.ts:79-81` accepts only a 64-hex pin;
  `gateway/src/keys.ts:44` publishes the 88-hex Ed25519 SPKI DER. A genuine
  pinned connection is refused as "not an Ed25519 key", no generated
  connection file writes a pin (`app/src/views/GateSetup.tsx`), and an
  unpinned connection verifies against the key the same gateway served. Fix:
  one agreed encoding, a generated pin, and a round-trip test.
- **AH-9 medium.** `gateway/src/main.ts:37` and `utility/src/main.ts:16` call
  `listen(port)` without a host, binding every interface, while
  `gateway/README.md:62` describes the example as loopback. Fix: bind
  `127.0.0.1` unless a host is configured.
- **AH-10 medium.** `deliver` (`lib.rs:2006`) lets any caller replace the
  pending ciphertext and `confirm` (`:2055`) matches only the current stored
  value, so a third party can replace the bytes the buyer checked and make
  confirmation fail until the deadline. Fix: keep permissionless delivery and
  junk-first recovery while the checked digest stays settleable (bounded
  candidate set or an equivalent rule), with all sequences tested.
- **AH-11 low.** `donateUnit` (`:1349`) does not refuse a listing cleared by
  two strikes, so its units keep moving the one global quote while `buy`
  refuses the bundle (`listing_cleared`). Fix: refuse the donation or remove
  its units from the count.
- **AH-12 medium.** `reveal` (`:2193`) settles one order but writes no
  revealed state and leaves the remaining units sellable, so later buyers can
  pay for a secret that is already public. Fix: record revealed state, remove
  the units, and refuse new buys and donations; keep funded orders closable.
- **AH-13 medium.** After `funded + DEADLINE`, `refund` (`:2240`) and
  `fulfill` (`:2105`) / `reveal` are all live; the comment at `:2288-2293`
  says the boundary rules never overlap, but they overlap in ordering. Fix:
  state that repayment is available whenever the seller has not settled and
  pin both orderings with boundary tests.
- **AH-14 low.** The 209-byte delivery journal
  (`proofs/delivery/methods/guest/src/main.rs:29-56`) has no network or fork
  domain, while the registration statement carries the network digest. Fix:
  add the canonical network digest and check it in `fulfill`, or record the
  fork limitation as accepted.
- **AH-15 medium.** `addReserve` (`:1297`) takes unattributed, irreversible
  deposits and the reserve has no supported outflow while
  `SUPPLY_PAYOUTS_ENABLED` is false. Fix: refuse the deposit in that state, or
  define ownership and a bounded exit that cannot touch pending escrow or
  credits.
- **AH-16 medium.** The app walk cannot run from the declared environment:
  `playwright` is absent from `app/package.json` and the `node_modules` link
  points into `/tmp`. Fix: declare the verification dependency and make the
  release check fail loudly when a required walk cannot run.

The deepest question is unchanged: H-3 (participant-verifiable scoring) stays
a handover note. AH-1 and AH-2 remove the trivial bypass of the reference
gate; they do not make the provider's score independently verifiable.

### 2026-09-29 - audit 2 - gates green; three low-severity follow-ups

Scope: the third-revision goal (section 0) and the audit cadence rule (section
5), the changed app/utility/release files, and the pending/weak matrix rows.

Gates, all green on this revision:

- `node release/check-integration.mjs` - ALL CHECKS PASSED.
- `cd app && npm run verify` - ALL CHECKS PASSED, including the example gate and
  the test utility grant.
- `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` - ok
  (14.8 s).
- `cd spikes/harness && GOFLAGS=-modfile=go.local.mod go test -count=1 ./...` -
  ok (5.3 s).
- `cd contracts/escrow && cargo test --locked --offline` - 15 passed.
- `cd proofs/session-core && cargo test --locked --offline` - 19 passed.
- `cd proofs/registration-v2/host && cargo test --locked --offline --test
  sealed_artefact` - 1 passed.

Documentation/hash drift: the living matrix and final report carry the current
interface 283,304 bytes `382e639c…`; `release/MANIFEST.json` matches the built
HTML; the older hashes remain only in the dated evidence entries. The SPEC 10.4
Package G rows match the matrix: G4 met for the reference testbed, G1/G2/G3/G5
open. The utility README's test count was corrected from 12 to 14 in this pass;
no other stale current claim found.

Risk checklist (files, keys, signing, fees) on the changed modules: the app
changes do not write files or handle keys beyond downloading public connection
and gateway configs; the utility change is the public `/access` endpoint; the
escrow and `tools/fod` suites above re-pin the earlier key/write/fee fixes. No
new high or medium finding.

Findings, all three fixed as goal work after the read-only audit:

- **F1 (low, trust UX).** The "Claim test access" button was enabled whenever
  the provider result said `passed`, even when the browser's signature check
  said "not verified". Fixed: the button requires `verification ===
  "verified"` (`app/src/views/Session.tsx`), and the app walk asserts that an
  unverified result cannot claim the test utility.
- **F2 (low, documentation).** Fixed: the generated connection note now names
  both the gateway and utility URLs (`app/src/views/GateSetup.tsx`).
- **F3 (low, hosting hardening).** Fixed: the reference utility's CORS answer
  is driven by the policy's `origins` list (`utility/src/policy.ts`,
  `utility/src/server.ts`): `["null"]` for the offline testbed (the example
  policy), `["*"]` for a public testbed, or the exact dapp origins for a
  production host; `utility/README.md` documents it.

Verification of the fixes: `cd utility && npm run typecheck && npm test` - 14 of
14; `cd app && npm run typecheck` clean; `npm run build` wrote the interface at
283,334 bytes `ec5843d9…`; `npm run verify` ALL CHECKS PASSED, including "an
unverified result cannot claim the test utility"; a second build produced the
same `ec5843d9…` hash; `node release/build-manifest.mjs` regenerated the
manifest (interface 283,334 bytes `ec5843d9…`, module unchanged 373,223 bytes
`a6764bc7…`, 0 forbidden operators, 0 remote tags); `node
release/check-integration.mjs` ALL CHECKS PASSED.

No new handover note: H-3 (provider-trusted scoring) remains the only hard-case
handover, and the findings above are local, low-severity follow-ups that are now
closed.

### 2026-09-29 - UI review follow-up - the author view still offers the refused sell path

Finding (severity: low, user-facing claim drift): `My trades` showed a "Sell
one unit" button for the bundle author (`app/src/views/Trades.tsx:325`) that
downloaded a `sellUnit` request (`prepareSell`, `:126`), while the accepted
policy refuses `sellUnit` (`supply_disabled`) and the FAQ already states that
"Give one unit" is the only supply path.

Fix, applied: the button and the `prepareSell` handler are removed, so the
author view offers only "Give one unit". `app/verify.mjs` now asserts the
refused button is absent; `docs/SPEC.md`'s interface-operation list and 7.1
`sellUnit` row and `release/README.md`'s sell/give step now state that the
interface offers donation only and the contract's `sellUnit` stays refused.

Verification: `cd app && npm run typecheck` clean; `npm run build` wrote the
interface at 276,102 bytes `0bbc9800…`; `npm run verify` ALL CHECKS PASSED
including the new absence check; a second build produced the same `0bbc9800…`
hash; `node release/build-manifest.mjs` regenerated the manifest (interface
276,102 bytes `0bbc9800…`, module unchanged 373,223 bytes `a6764bc7…`, 0
forbidden operators, 0 remote tags); `node release/check-integration.mjs` ALL
CHECKS PASSED.

### 2026-09-29 - audit 1 - finding and fix: the refund watch signed from a shared temp path

Finding (severity: medium on a multi-user host, low on a single-user one):
`fod watch` wrote its prepared refund action to a predictable path in the
world-writable temp directory (`/tmp/fod-watch-refund-<id>.json`, mode 0644) and
then handed that path to `fod submit` to sign. Any other local user could
pre-create that path or replace its content between the write and the read, and
the watch would sign and broadcast *their* prepared call with the watcher's
nodekey - a refund, a reveal, or anything else the CLI can prepare.

Fix, applied in this audit and folded into the goal: the watch creates its own
0700 work directory with `os.MkdirTemp`, writes a uniquely named action through
`writePrivateJSON` (0600) and removes the directory when it returns; no shared
path is involved, and `TestWatchActionLivesInAPrivateDirectory` pins the 0700
directory, the 0600 action, the round trip and the cleanup. `tools/fod` builds,
`go vet` is clean and the suite passes (14.6 s). The CLI is not part of the
manifest's hashed artefacts, so the manifest, hashes and clean rebuild stand.

### 2026-09-29 - audit 1 - finding and fix: key-bearing files were world-readable

While reviewing the code added this session, the audit found that `writeJSON`
writes with mode 0644 and three *key-bearing* artefacts used it:

- `fod reveal`'s prepared action carries the committed epoch secrets in its
  calldata; the file written on this machine was observed at 0644
  (`/tmp/fod-new/prepared-reveal-order2.json`, 750 bytes).
- `fod session-bundle -blob-out`'s registration blob witness carries both epoch
  private scalars, and its own comment says the file must not be published.
- `fod prepare-offer -input`'s registration input carries the same two epoch
  secrets for the registration host to consume - the most exposed of the three,
  because that file is handed to another process by design.

Severity: medium. Any local reader, sync client or backup process could copy the
keys before a reveal was ever submitted, silently destroying the bundle's
exclusivity while the seller believed the secrets were still private.

Fix, applied in this audit and folded into the goal: `writePrivateJSON` writes
0600 on create and resets the mode on an existing file, because `os.WriteFile`
keeps whatever mode a file already has; `writeRevealAction`,
`writeRegistrationBlobInput` and `writeRegistrationInput` now use it, and two
tests write over a pre-existing 0644 file and assert 0600
(`TestRevealActionFileIsOwnerOnly`, `TestRegistrationInputIsOwnerOnly`).
`tools/fod` suite green (15.2 s). The lab's pre-fix file stays 0644 in `/tmp` on
purpose: its secrets are the epoch-1 lab keys the on-chain reveal already
published, so there is nothing left to protect there.
The other half of the privacy row was already covered: the browser walk asserts
that nothing the page handled is left in `localStorage`, `sessionStorage` or a
cookie (`app/verify.mjs`, "the page keeps nothing in browser storage"), so the
CLI's key-bearing files were the only uncovered surface this audit found.
Coverage extended the same pass to the desk's submit path: one transaction in
flight at a time (`nextTransaction` reads the mined nonce plus one and
`waitForNonce` blocks until it moves), a 25% fee headroom over the measured
requirement, a rebuild with the signed-size requirement when the first signature
falls short, and `checkAccountAgainstChain` before the broadcast. Nothing new
was found there, and the desk's outdir is created 0700, so its prepared actions
are not exposed even though they are written 0644.

### 2026-09-29 - audit 1 - documentation drift sweep, all in scope

Scope: every README in the repository (`release`, `contracts/escrow`, `gateway`,
`utility`, `proofs/delivery`, `spikes`, `spikes/r0-receipt`), `SPEC.md` 4.x and
7.x, `app/src/faq.ts`, and the requirement-to-evidence matrix rows that were
written in this session.

Findings and disposition: the escrow README and the two spec sections were the
stale documents; both were corrected this session (entries above) and a re-scan
for the old phrases (`720`, "credit the buyer", "into the reserve", "grace
period", "per-bundle reserve", an enabled `sellUnit`) now returns nothing.
Nothing else was found in this pass. No new gate failure: the last runs of
Gates re-run in this audit on this revision: `tools/fod` ok (15.3 s),
`spikes/harness` ok (5.5 s), `cd app && npm run verify` ALL CHECKS PASSED,
`node release/check-integration.mjs` ALL CHECKS PASSED. The cargo suites were
last run after their sources last changed (`contracts/escrow` 15/15,
`proofs/session-core` 19/19, the sealed registration artefact 1/1), and the
audit automation re-runs the whole set each cycle.

## 7. Handover notes (hardest cases)

These are the questions this session cannot settle, with the evidence gathered
and what a stronger model or a differently authorized operator should decide.

- **SA privacy incident — local application-key rotation completed 2026-10-03.**
  The initial broad search unexpectedly included the ignored
  `gateway/provider-application-key.json` and emitted its private signing
  value in tool output. Treat that application key as exposed. The value was
  not repeated or copied into audit artifacts; subsequent searches used
  explicit source paths and excluded runtime data. Required next action:
  rotate that application signing key, distribute its replacement public pin
  to affected connection/policy files, and invalidate acceptance of the old
  pin before reusing the gateway. At the original audit, existing credential
  rotation and any restart were outside authority and were not performed.
  The owner's follow-up authorized this local application-key rotation: it is
  now complete, with old-key browser/utility refusals and replacement-pin
  verification recorded in section 3. No service was running to restart.
  External old-pin copies remain outside this checkout and must be replaced
  before reuse. This is an audit handling incident, not evidence that a wallet
  key was exposed.


- **H-1 Independent-operator evidence across hosts.** Historical independently
  configured nodes agreed on head, market, accounting and receipts for the
  previous module, but both ran on one host. They do not qualify the S5/S6
  candidate. What is needed: a second host running a compatible lab node (or a
  real second operator). Concretely: copy `release/artifacts/fod_escrow.wasm`
  and the CLI to that host, run the pinned reference node with a compatible
  config, deploy the module, repeat `donateUnit` / `buy` / `deliver` /
  `confirm` / `refund`, then diff the two hosts' `fod market`, `accounting` and
  `bcn_txReceipt` payloads. The expected hashes are in `release/MANIFEST.json`.
  Blocked on authority, not on code: remote hosts are outside this task's scope.
- **H-2 Mainnet wall-clock and cost measurement (`SPEC.md` O-9).** Historical lab
  receipts measure gas and fees for their recorded module; mainnet wall-clock per call, real block cadence
  under load and the fee at mainnet's rate for the current module are unmeasured.
  What is needed: an authorized mainnet exercise with real funds and a recorded
  receipt per call. Concretely: deploy the release module, run one order through
  buy / deliver / confirm and another through a timeout refund, and record each
  receipt's `gasUsed`, `txFee` and the wall-clock between submission and
  inclusion; compare against `fod fees` at the chain's then-current rate.
  Evidence ready: the measured gas table, the rate recorded in `SOURCES.md`
  F-77, and the deploy/proof cost bounds. The deploy smoke-test allowance is
  suspended, so this needs a fresh per-action approval.
- **H-3 Honest scoring and actual utility release - open for the full-product
  goal.** Packages I, J and L established local recomputation. S2 now requires
  witnessed wallet-signed answers, an independently configured content digest
  and window, recomputed closed-window scoring and a fresh wallet-control nonce.
  The optional `requireScoreProof` transcript branch is diagnostic only: it
  cannot grant without this evidence. The reference integration rejects a
  copied or tampered claim and can grant from witnessed correct answers despite
  a provider's contrary verdict. These are local HTTP/browser measurements,
  not canonical timing/provenance/consumption guarantees.
  The utility's clock and store witness the window; its operator selects the
  answer key and controls the actual benefit. Matching a commitment establishes
  consistency, not historical correctness. Expiring durable quota storage
  survives restart and refuses capacity eviction, but remains a local allowance,
  not global or lifetime consumption. The full-product goal needs independently
  authenticated answer authority and timing, canonical consumption and benefit
  release without discretionary refusal. A chain commitment may be part of a
  design, but does not by itself establish all of those properties. Do not
  substitute a trusted scorer, hosted keeper or allowlist.
  S8 rechecked the current registration/session statement: author-level
  qualification and CID/decryption evidence do not carry a per-flip correct
  answer or a fresh customer's solve. No general impossibility of deriving
  additional historical evidence is claimed. A concrete utility/asset/chain
  target was requested once; the recommendation is an isolated native
  play-money faucet. Defining that target does not itself resolve canonical
  answer authority, timing, consumption or release.
- **H-4 What the chain itself guarantees.** The lab chain is a private
  single-validator automine; the mainnet fast-sync route verifies roots against
  headers but keeps the chain's own trust assumptions. The project claims only
  what its own application adds beyond the pinned chain and its cryptography.
  Concretely, the assumptions to analyse are the pinned chain's consensus and
  the light-client route recorded in `docs/SOURCES.md` F-77 and F-90; a stronger
  claim needs that analysis, which this task does not attempt.
- **H-5 The delivery journal's chain domain - repaired locally by S6.** The
  genuine delivery-v2 tuple binds network, funding height/root, contract, order,
  nonce, author, buyer, delivery key and commitments. Native and runtime
  verification pass; missing/wrong anchors, legacy statements and replay refuse.
  A shared-funding-anchor fork retains the chain's canonical-view assumption.
  Deployment and fresh chain receipts remain outside this task's authorization.
  Historical finding/acceptance (2026-09-30; superseded for the local candidate):
  `fulfill`'s journal carried no
  network or fork domain while the registration statement does (`AH-14`),
  accepted as a stated limitation on 2026-09-30. The fix is a `NETWORK_DIGEST`
  field checked against the module's own constant, which changes the guest
  program, its pinned image id in `contracts/escrow/src/verifier.rs` and
  `tools/fod/receipt_claim.go`, and the delivery-proof record in
  `release/MANIFEST.json`. A network identifier alone does not distinguish
  same-network forks; specify the accepted chain/genesis domain and replay
  model before changing the journal. The complete versioned proof tuple needs
  regeneration and verification. Local guest/tuple changes and local proof generation are authorized;
  deployment of a changed tuple and replacement live-chain execution receipts
  are not. Existing receipts stay valid for the existing module: changing local
  source does not retire them or imply authorization to deploy. No such local
  tuple change or new proof was made in S1-S4; S6 now supplies the new local tuple.
- **H-6 (opened and resolved 2026-09-30).** The lab node's silent skip of the
  buyer's `confirm` was not a host gap: the node derives a contract call's gas
  budget from the signed max fee, the confirmation that carries the buyer's
  219-byte ciphertext needs 20,357 user gas, and the fee table carried the
  superseded pre-`AH-10` 14,747, so the proposal ran the call out of gas and
  blacklisted the hash. The row now carries the live `contract_estimateCall`
  measurement, the guard refuses a short budget before broadcasting, and order
  2 settled in tx `0xdd130fd1…` (`PROGRESS.md` section 3, 2026-09-30
  resolution entry). Kept here so the trail is not re-chased; nothing about it
  needs a different capability or authority.

- **H-7 Canonical availability and two-sided fair exchange (2026-10-01).**
  Registration-v3 proves that committed secrets open the exact CID-bound blobs
  into supported structures. S8's negative runtime control demonstrates a
  distinct gap: the buyer decrypts a valid unincluded envelope, then gets the
  payment back at +3 without losing the keys. This refutes the current
  construction's unconditional fair-exchange claim, not all possible designs.
  `fulfill` authenticates encryption of the committed secrets, and `reveal`
  authenticates their hash, not retained content bytes. S1 closes the serving
  disappearance attack on the ordinary buyer path by keeping authenticated
  ciphertext before signing, but the contract cannot enforce that local storage
  for custom callers. A dishonest public RPC also cannot authenticate chain
  state by assertion. Establish a canonical buyer content-possession acceptance
  and independently authenticated state route, or an equivalent fair-exchange
  construction, before making a protocol-wide usable-content claim. Do not
  insert a trusted availability signer. Off-chain secrets disclosed before a
  seller misses the settlement deadline cannot be revoked; preserve the honest
  provider's protection without pretending a refund undoes that disclosure.
  The order-specific v2 proof measured 946.459854 seconds after inclusion, so it
  has not demonstrated timely private delivery. Investigate independently
  producible proofs with a justified precomputation/context design and canonical
  availability/inclusion-dependent release; do not extend the deadline or add
  a trusted availability signer. An authenticated lightweight state route is
  also missing: replaceable RPCs remain untrusted until their state is verified.
  S9 now preserves settled public keys in contract state and refuses known-late
  disclosure before signing/broadcast. That improves recovery and containment,
  not finality-dependent secrecy. The captured inventory's naive single-call
  archive has a 47,070,900 user-gas lower bound against a 5,120,000 block cap;
  authenticated, bounded pre-publication remains an unimplemented candidate.
  The small capsule experiment is not promoted: preparation proof, WASM cost,
  canonical content availability and safe disclosure are unestablished. A
  protocol design must address the exposed-pending-transaction counterexample
  under the stated chain assumptions; no private keeper or deadline extension
  is authorized as a replacement.
  The cheap public `reveal` has the same pre-inclusion exposure: its calldata
  contains the epoch secrets, which a buyer may learn from a pending transaction
  even if it never settles. Instant finality after inclusion does not make the
  mempool confidential or guarantee inclusion before `funded+3`. A bounded
  cooperative mainnet exercise can verify actual inclusion, payout and recovery,
  but cannot close this adversarial guarantee. One candidate would replace
  selling secret keys with a canonical utility entitlement released atomically
  with payment; the actual utility would have to enforce that entitlement, and
  authenticated content would have to be available before purchase. This is a
  material product change. A secret-sale candidate instead needs an
  independently verifiable finality-dependent release primitive; none has
  been established on the pinned chain. These candidates do not constitute a
  general impossibility theorem or a qualified implementation.
- **H-8 Deadline wording discrepancy - repaired locally by S5 (2026-10-01).**
  All unilateral paths now refuse at funded+3, and direct permissionless refund
  opens at that same block; both orderings and original-buyer repayment pass.
  Buyer-authorized confirmation may still settle an open order later. New
  chain receipts remain absent. Historical discrepancy: previous source/harness
  allowed unilateral settlement at `funded+3` and refund at `funded+4`, whereas the
  earlier AH-13 brief requested refund-only at `funded+3`. S1 preserved the
  current rule, not a silently shortened/extended window. Reconcile this with
  the intended three-block acceptance before final-chain qualification; any
  boundary change must update all settle/refund predicates, CLI watch/quotes,
  browser estimates, tests and candidate module together.
- **H-9 Dirty-source release provenance - HTML/WASM repaired locally by S7
  (2026-10-01).** The manifest now identifies complete non-ignored input bytes
  and a private export verifies drift, symlinks and file-set identity. A clean
  source snapshot rebuilt HTML and lowered WASM byte-for-byte. The builder
  stages freshly built HTML/WASM and verifies the active delivery seal before
  promotion. The 2026-10-01 acceptance verification now also reproduces the
  native CLI from immutable reference source on this host. Portable guest
  reproduction fails H-10; full proof-production environment portability and
  current chain qualification remain unproved. Historical gap: the HEAD is unchanged
  and substantial tracked/untracked changes are preserved. Manifest hashes
  identify generated bytes and flag the dirty worktree, but do not identify a
  complete redistributable source snapshot. A clean HEAD clone cannot reproduce
  this release. Preserve/build from an approved source snapshot (with necessary
  generated proof tuple and dependency pins), compare clean builds and collect
  fresh local-chain receipts before calling the candidate reproducible.
- **H-10 Clean relocated proof builds did not reproduce the previous tuple
  (2026-10-01, initial high release blocker confirmed; locally repaired by S10).** The same snapshotted source
  and installed guest compiler produce a delivery image `ce65383d...bc9ae92`
  in the relocated build instead of the pin `60ec600b...95dd4ff`; the freshly
  built host rejects the shipped seal. Registration similarly changes from
  `7ee79d90...f9d86d9` to `37a6bfbd...54d2223` and fails `sealed_artefact`.
  A fresh delivery build at the original source location matches the pin and
  verifies the receipt. The local core dependency's path fingerprint differs;
  the exact compiler/linking cause is not yet established. Thus the receipt
  is not shown invalid, but default independent source builds cannot reproduce
  the accepted program. Evidence: the dated acceptance entry in section 3 and
  `output/verification/acceptance-proof-2026-10-01/`.
  Proposed repair: pin the complete guest toolchain and reproducible build
  environment/source layout, including local path dependencies; demonstrate
  identical images from two clean source locations, then regenerate and verify
  the registration/delivery/contract/CLI tuple together once. RISC Zero provides
  a [containerized deterministic guest build](https://docs.rs/cargo-risczero/3.0.5/cargo_risczero/)
  as a candidate; verify the actual installed SDK/pinned builder before relying
  on it. Add a clean relocated-build receipt gate to release validation. Do not
  weaken verification, retain old labels over new guests, or regenerate an
  incompatible seal merely to make one check pass. This verification did not
  change production code, proof artifacts or deployments.
  Follow-up S10, same date: source-only digest-pinned container builds produce
  identical programs at separate clean locations. New genuine delivery and
  registration receipts verify natively and in the executing WASM runtime;
  contract, CLI and manifest pins now agree. The release gate checks both
  programs and both seals. See the S10 evidence block above and its hash-bound
  `EVIDENCE.json`. This closes the source-location/image mismatch locally,
  not cold-cache platform qualification, timely private delivery or fresh
  chain deployment evidence. Previous immutable deployments and their pins
  remain unchanged; paid supply stays disabled.
- **H-11 A successful `addReserve` whose value never reached the contract
  (2026-10-02, open).** On the previous lab instance
  (`0xd53b3032…`) block 13239 carried `addReserve` tx `0x6a5983c7aafc…` with
  `amount 1` iDNA and a `success true` receipt, and the contract wrote the
  reserve and the donor credit (`reserve0 = 1e18`,
  `reserve-credit/<author> = 1e18`). The contract's balance never carried it:
  the earliest readable state version (height 13339) reads 0, and the balance
  only became `2e18` when the buyer's `buy` landed at 13355. Nothing in the
  contract can pay out without `create_transfer_promise`, and no such call ran
  before 13339, so the money did not leave by contract logic. A fresh
  `addReserve` of 0.01 iDNA from the same account on the same contract did raise
  the balance, so value transfers are not broken in general. Consequence on
  that instance: `settle_order`'s solvency guard refused the buyer's `confirm`
  (`not_enough_balance`), and the order could only be refunded. The guard added
  the same day (`unfunded_deposit`) would refuse the write whenever the balance
  does not carry the deposit, so a repeat cannot wedge a contract this way even
  if the cause stays unexplained. What is needed: a fresh lab instance with
  per-height reads of the contract account (`StateDbKeys.AddressKey`) around its
  first `addReserve`, the sender's balance delta across that block, and the
  node's own path for that tx (`blockchain.go`: `shouldAddPayAmount`,
  `AddBalance(contractAddr, amount)` before `vm.Run`). If the value is dropped
  by the node, that is a node-level defect and needs an upstream report; the
  lab datadirs from this run are `~/Library/Application
  Support/FlipsOnDemand/fod-v3-cer/datadir` and its `datadir-snap-*` copies in
  `/tmp/fod-capture-v7`. This is not reproducible on the current artefact, and
  no claim should be made either way until the per-height ladder is run.
  Update, same day, from the node's own log
  (`…/fod-v3-cer/datadir/logs/output.log`, 32 MB, retained): the node was
  **rejecting its own proposals for an invalid state root** throughout that
  window - 15 occurrences between `10-02|01:04:28` and `10-02|01:13:08`,
  `Proposed block is not found err="invalid block roots. Expected=… & e3b2529d…,
  actual=… & e3b2529d…"` (the identity root agrees on both sides, the state root
  does not), each followed by `Reached consensus on empty block`. The
  `addReserve` tx was sent at `01:04:14`, i.e. into exactly that episode, and the
  same log holds 13 more such lines from 09-29 but **none after 01:13:08 today**
  - today's successful lifecycle (~02:55-03:10) ran on the clean side of it.
  That makes a node-level state-integrity episode the most likely cause of the
  missing value transfer rather than a contract fault, which is why the note
  stays open for a stronger model: it needs the per-height ladder above plus the
  node's own root computation, not another contract patch.
- **H-12 `bcn_txReceipt` panics instead of returning null for a tx that is not
  in a block yet (2026-10-02, open).** The same log shows, immediately after
  each broadcast, a recovered panic whose stack ends in
  `api.convertReceipt` (`api/contract_api.go:393`) reached from
  `BlockchainApi.TxReceipt` (`api/blockchain_api.go:208`) over HTTP. The RPC
  server recovers it per request, so the node survives, but the client sees an
  empty body - which is the `the node reported no receipt: decode RPC response:
  unexpected end of JSON input` every `fod submit -broadcast` printed in this
  session. Every one of those transactions later had a normal receipt, so this
  is a robustness defect in the node's RPC surface, not a consensus issue.
  Evidence: the stack traces in `output.log` at `10-02|01:04:14.280`,
  `01:04:16.284`, `01:04:19.288` and at each later broadcast; the pinned
  checkout has `convertReceipt` dereferencing a receipt that is nil for a
  pending hash. Needs an upstream report against the pinned revision, not a
  local patch of the reference checkout.

Client-side mitigation for H-12, same day (the node stays unpatched, as it
must): `tools/fod/rpc.go` now recognises a response that arrives with **no body
at all** and returns a named error saying the node recovered from a handler
panic, instead of letting it surface as `decode RPC response: …`;
`tools/fod/submit.go` turns that case into
`the node has no receipt yet: its converter panicked on a missing receipt and the
request came back empty`, keeps the ordinary single retry, and leaves a real
node answer (including its error replies) reported as itself. Two cases pin it
in `tools/fod/submit_outcome_test.go`: an empty answer is recorded as "not
included yet" and must not blame a decode failure, and a transaction that is
mined on the second look after such an answer is still reported with its gas and
fee. Verification: `cd tools/fod && GOFLAGS=-modfile=go.local.mod go test
-count=1 ./...` - ok, 35.0 s; `gofmt -l` clean on the three files. The node-level
defect is unchanged and still belongs upstream.

Coherence after that change: the manifest's source digest has to describe the
tree, so `node release/build-manifest.mjs` was run again - same artefact hashes
(escrow `ac97c56ce6362868…`, interface `a67c7903dbd44bd9…`, `forbidden operators:
0`, `19 published names, no test hooks`, `remote tags: 0`) with the source
inventory now covering the CLI fix - and `node release/check-integration.mjs`
reported `ALL CHECKS PASSED` again (74.3 s wall, the browser walk included).

Quote check on the same run, because the numbers looked odd at first: with
`reserve = 2e18` and `units = 2` the buyer's `prepare-buy` quoted
`2000000000000000000` while `fod market` published `sellQuote 666666666666666666`.
Both match `contracts/escrow/src/pool.rs` exactly - `buy_quote` is
`ceil(reserve / (units - 1))` (2e18 / 1 = 2e18) and `sell_quote` is
`floor(reserve / (units + 1))` (2e18 / 3) - so the chain quoted what the
contract's own formula says, including the "two units left" case where one unit
costs the whole reserve. Not a drift; the formula and the on-chain numbers agree.

Measured chain stall, and what it does to the delivery window (2026-10-02, same
session, after the successful ladder): the lab node entered its validation
ceremony and then refused **every** non-ceremony transaction - the desk's
`beacon` broadcast three times, then an ordinary `donateUnit` submitted with a
correct nonce - each with
`rpc bcn_sendRawTx: tx can't be accepted due to validation ceremony (-32000)`.
The chain did not stop: the head advanced from 13569 to 13622 (empty blocks
kept being produced) while no contract call could be included. Consequence for
the product, and the reason this is recorded rather than waited out: the
delivery deadline is **counted in blocks** (`fundedAt + 3`), so a buy that lands
just before such a window can lose its entire delivery window while the chain
still produces blocks and refuses the desk's `deliver`; the buyer then refunds
and the author is never paid. Nothing here contradicts the recorded refund
path - funds stay refundable - but it is the measured counterexample to any
"instant" or fixed wall-clock promise, and it belongs in the interface's
delivery estimate rather than in a footnote. The ladder paused here; the
delivered-then-refunded case (the buyer keeping content while the deadline
expires) still has no on-chain receipt for this artefact, and waiting out the
ceremony is the only way to get one.

That consequence is now in the interface rather than only in this record:
`app/src/views/Market.tsx` keeps the block-cadence estimate ("delivery within 3
blocks (about Ns)") and its second line now reads "the window starts when your
buy is included and counts blocks, not seconds - a chain stall (a validation
ceremony, for instance) can use it up before the desk can deliver; repayment
opens at the third block either way", which is the measured behaviour above.
Checks: `cd app && npm run typecheck` clean, `npm run build` wrote a 302,257-byte
interface, `npm run verify` reported `ALL CHECKS PASSED` (the browser walk,
including its session-to-utility half), and `node release/build-manifest.mjs`
re-recorded the artefact set - interface `aa52797dbf2cee7f…`, escrow unchanged
at `ac97c56ce6362868…`, `forbidden operators: 0`, `remote tags: 0`.

Ceremony window closed, same day: at head 13635 the node **accepted** a
`donateUnit` again (`0xc1c6887a73dad067…`, nonce 11, epoch 77) instead of
refusing it for the validation ceremony, so the stall above lasted from about
head 13569 to 13634 (roughly 65 blocks of refused, non-ceremony transactions,
the head still advancing throughout). At head 13636 the transaction was still in
the mempool (`bcn_mempool` holds exactly it) and the listing still read
`units 1`, i.e. inclusion had not happened yet - worth noting because that
pending gap is the same window the delivery deadline is measured in. The
delivered-then-refunded ladder is unblocked and can resume from here; it needs
the donation to be mined, a fresh beacon from the desk, and a buy whose delivery
is allowed to run past the confirmation.

Correction to the line above, from the node's own log: that donation never made
it in, and the reason is not slow inclusion. The log holds
`WARN [10-02|03:26:23.431] Tx is not valid
hash=0xc1c6887a73dad067… err="tx can't be acce…"` for it, the CLI's
resubmission at `03:30:42` (`Sending new tx … Sent own tx push`), and then
`Reached consensus on empty block` / `Proposed block` lines for every block
afterwards - the producer is finalising blocks while declining this
transaction, even though `bcn_mempool` still lists it. The truncated error is
the ceremony family, so the producer's own view of the ceremony boundary has
not lifted even though the RPC endpoint accepts the submission. That is a
node-side state I cannot clear from the RPC surface: the ladder's first step is
blocked by the producer, not by the contract, the client or the money.
Next action for whoever picks this up, and the reason this is not marked as a
dead end: the lab node is ours and disposable - restarting it (same binary,
same datadir) is within this task's authority, unlike a remote host, and is the
cheap way to re-derive the ceremony boundary and let the pending ladder run.
Everything else in this entry stands; only the "unblocked" claim above is
superseded.

That restart was done, and it was the right lever: the local `fod-v3-cer` node
(pid 53965, up ~1h45m at that point) was stopped and started again from the same
`/tmp/idena-go-pinned` binary, config and datadir, while the second node
(`fod-v3-lab3`) stayed up. On the first poll afterwards the node answered at
head **13645**, was not syncing, its mempool was **empty**, the previously stuck
donation had a receipt (`bcn_txReceipt` for `0xc1c6887a73dad067…`), and the
listing read **`units 2`**. So the producer's stale ceremony view was purely
in-process state, not chain state, and the pending ladder is unblocked for real
this time. Next steps in order: start the desk against `0xc4032046…` for a fresh
beacon, buy, let the desk deliver inside the window, withhold confirmation, wait
for `fundedAt + 3`, refund.

The ladder then ran one step further and produced a result worth its own line.
After the restart the desk published a fresh beacon (`0xd8b288a2…`), the buyer
bought order 3 (`0xe7c4ed7f…`, nonce 1, epoch 78 - and the CLI's rebuilt binary
reported the receipt honestly as "the node has no receipt yet: its converter
panicked on a missing receipt", which is H-12's mitigation working in a real
run), and the desk delivered inside the window (`deliver-3`, tx `0x169ac51f…`).
`pdeliv/3` is on chain at 219 bytes and `pending0` holds the 3 iDNA escrow. Then
the desk's own boundary logic fired:
`boundary order 3: no confirmation, publishing the committed secrets (2 blocks
left, about 40s at this node's 20s per block)` followed by
`serve: bundle has a prior refund: refusing public key disclosure`.

That is the measured answer to the case this ladder was built for, and it is
more nuanced than "the buyer keeps the content and refunds": on this build the
desk **withholds the epoch keys when the bundle already carries a refund**, so a
buyer who withholds confirmation on such a bundle can refund the escrow but
cannot decrypt what was delivered. The pure fair-exchange case - usable content
*and* the escrow back - therefore needs a bundle with no prior refund (or a desk
with a real prover path), which is a fresh-bundle run, not another order on this
one. Order 3's refund itself can still be exercised to finish the money side;
what it will not show is the content-plus-refund loss.

Order 3's refund was then run, so the money half of the case is on chain:
`fod refund` for order 3 from the buyer's own key broadcast `0xdaccfec4…` and the
receipt reads `method refund`, `success true`, `gasUsed 15079`, `txFee 12.1`.
Afterwards `pending0 = 0`, the buyer's escrow came back (balance `1903.42` before
the buy, `1566.55` after buy plus refund, i.e. the purchase price returned and
only the two calls' own costs - roughly 300 iDNA of gas plus fees at this lab's
scale - kept), and `pdeliv/3` is **still on chain at 219 bytes**: the delivered
ciphertext outlives the refund that returned its price. The keys did not, which
is the desk's own prior-refund policy from the paragraph above. So the recorded
outcome for this artefact is: delivered but unconfirmed, price refunded,
ciphertext retained, decryption impossible because the desk withheld the reveal
for a bundle with refund history.
