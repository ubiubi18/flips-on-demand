# Chief Developer Review - 2026-09-26

Baseline: `a91e475`, clean working tree, 204 commits ahead of the locally recorded
remote branch. This is one source and targeted-verification pass, not an
exhaustive security certification. No production code, transactions, remote
services, or reference checkouts were changed. DeepSeek V4.1 Flash implements
the follow-up; the reviewer supplies findings, requirements, and exit tests.

**Verdict: not ready to deploy or fund.** The project contains useful working
components, but its current market does not establish authenticated session
provenance, protected refund liabilities, or a complete user journey. The old
claim that only a capture and approvals remain is withdrawn.

## Findings

All locations below refer to the baseline. P1 blocks any funded release; P2
blocks the relevant advertised feature or release claim. Source-proven defects
are distinguished from executed reproductions. Proposed tests are work for
DeepSeek, not tests added by this review.

### A01 - P1: A public receipt authorizes payouts to arbitrary callers

- Evidence: `contracts/escrow/src/lib.rs:1003` (`sellUnit`) checks the original
  registration journal and seal, then credits `caller_address()` at line 1050.
  The journal is bound to the original author, not this caller or a sale nonce.
- A registration receipt is public transaction data. Reusing it proves that a
  proof was produced, not that its present submitter knows the keys. Any address
  can reuse the same bytes for repeated payouts. The supply harness tests an
  honest author only. Confidence: high, source-proven; no exploit broadcast.
- Repair: separate registration evidence from authorization to supply. Bind any
  possession proof to caller, network, contract, operation, bundle and a consumed
  nonce/nullifier. For author-only initial issuance, authenticate the author and
  enforce the agreed finite entitlement. Apply equivalent authorization to
  donations and beacons; their current calls accept only a public commitment.
- Exit tests: copy a real registration receipt to another caller; replay the
  same supply authorization; replay across actions/contracts/networks; attempt
  unowned donation and beacon refresh. Each must fail without payment or state
  changes. A valid authorized supply must still work.

### A02 - P1: The registration proof is not a proof of a real session

- Evidence: `proofs/registration/methods/guest/src/main.rs:100` reads the anchor,
  inventory, assignments, answers and grades from the witness. Lines 131-135
  require only one or more paths that fold to that supplied anchor.
  `proofs/anchor-core/src/lib.rs:142` carries key/value hashes, not decoded
  authenticated candidate records. No relation to the candidate array is checked.
- Qualification at guest lines 186-225 uses the separate supplied arrays. There
  is no authenticated header, submission/answer inclusion, candidate completeness,
  seed-to-assignment derivation, or record-value-to-witness binding. Contract
  registration checks neither an authenticated anchor nor an explicit network
  identifier. Even a genuine unrelated tree proof does not authenticate invented
  answers or blobs. Confidence: high, source-proven; a forged Groth16 receipt was
  not generated during this pass.
- Repair: first map the legacy runtime's authenticated header access and the
  exact state/identity roots. Bind the public journal to chain identity, height,
  finalized header/root and rule version, and have the contract authenticate
  that anchor. Inside the guest, derive complete inventory, candidate records,
  lottery assignment and session verdicts from committed evidence. Inclusion of
  a selected subset is not completeness. A seller's root, an explorer response,
  or an operator checkpoint cannot substitute for this requirement.
- Exit tests: reject fabricated roots, a real unrelated root, omitted candidates
  or submissions, replaced answers/grades, altered epoch/network and arbitrary
  assignments. Accept a portable real closed-session package on a default
  fast-sync verifier. If legacy data cannot establish this, report infeasibility;
  a future capture by itself is not the repair.

### A03 - P1: Refund money can be spent on supply payouts

- Evidence: `buy` at `contracts/escrow/src/lib.rs:1386` adds the buyer's payment
  to the spendable reserve. `sellUnit` spends that reserve immediately. `refund`
  at line 1576 must subtract the full original payment from what remains.
- Executed integer-model counterexample using the code's exact quote formulas:
  reserve 50,000 / 6 units, buy for 10,000 -> reserve 60,000 / 5 units; 26 sales
  leave reserve 9,685, seller credits 50,315, and a 10,000 refund liability.
  `refund` must return `reserve_too_small`. This is an arithmetic reproduction,
  not a newly executed runtime attack. Confidence: high.
- Repair: track pending buyer escrow separately from spendable reserves. Release
  it only on verified settlement; refunds consume that order's escrow. Maintain
  `balance >= available reserves + pending refunds + withdrawal credits + host
  fee liabilities`, without counting the same funds twice. Compute all effects
  before writes and enforce the invariant across bundles and withdrawals.
- Exit tests: buy -> many sells -> failed delivery -> full refund -> withdraw;
  multiple concurrent buyers; two bundles; reordered settlement/refund; duplicate
  refunds; failed transfers. No authorized sale may consume another liability.

### A04 - P1: Rejection returns success to the runtime instead of reverting

- Evidence: `contracts/escrow/src/lib.rs:349` returns JSON from `error_reply`.
  It does not trap. The harness call helpers (`helpers_test.go:68`) require the
  runtime error to be nil even when the JSON reports refusal. `buy:1381` rejects
  changed quotes this way; `addReserve:962` does the same for wrong payments.
- A payable rejected call can retain the transferred amount with no order,
  reserve allocation, or refund credit. The release does not expose the test
  `spike-last` record, and `fod submit` reports the node receipt's success flag.
  Some overflow checks also follow storage writes. Confidence: high on return
  semantics; exact node-level value rollback still needs the devnet regression.
- Repair: use the runtime-supported failure/trap mechanism for rejected mutations
  so the node rolls back value and storage, or explicitly account for returned
  funds. Validate lengths and amounts before mutations; never silently truncate
  ciphertext or saturate an out-of-range amount into an accepted value.
- Exit tests through the release module and a node: stale quote, wrong payment,
  stale nonce, nonpayable value, oversized inputs and overflow. Failure must be
  observable and balances unchanged except the network fee.

### A05 - P1: Unlimited copies are treated as finite pool inventory

- Evidence: `sellUnit` always increments units, and `donateUnit:977` accepts an
  arbitrary caller without consuming a unique entitlement. `pool.rs:76` cannot
  distinguish a new authenticated bundle from another copy of the same keys.
- Caller-bound proofs alone cannot repair this: a genuine key holder can still
  sell unlimited copies. Starting with zero units, the first sale can claim the
  entire reserve. After two strikes units become zero, yet supply payouts are
  still permitted. The round-trip product tests do not establish bounded issuance
  or refund solvency. Confidence: high, source-proven.
- The product also promises bootstrap liquidity attracting other authors, while
  `PoolBundle` holds a separate reserve per commitment: funding one author's
  bundle gives a new author's bundle no liquidity.
- Repair proposal needing product acceptance: finite, consensus-bound issuance
  with at most one reserve-funded acquisition per canonical author/epoch bundle,
  an authenticated eligible-flip count, and a defined supply cap. Price delivery
  copies separately from subsidized inventory. Decide global versus per-bundle
  liquidity explicitly. Never implement a human issuer or administrator to patch
  this. Keep reserve-funded supply disabled until the model is specified and tested.
- Exit tests: unlimited repeated proofs, new addresses holding the same keys,
  duplicate donations, zero-unit bootstrap, cleared listings and new authors.
  Demonstrate the maximum total subsidy and quote/refund invariants analytically
  and in adversarial sequences, not just honest buy/sell round trips.

### A06 - P1: An outsider can occupy the fast delivery slot

- Evidence: `deliver` at `contracts/escrow/src/lib.rs:1425` accepts any caller
  and any nonempty payload, marks the order delivered, and refuses another fast
  delivery. `beacon:1074` likewise proves no delivery capability.
- An outsider can publish junk first, forcing an honest desk into the expensive
  proof fallback. Without that fallback the order times out and strikes penalize
  the bundle. The current protocol requires buy, delivery and confirmation or
  proof; same-transaction settlement is not implemented. Confidence: high.
- Repair: bind the fast path to an authenticated, permissionlessly selectable
  delivery capability, or keep unverified candidates replaceable without blocking
  correct delivery. Preserve proof-based settlement against an uncooperative
  buyer. Identify who pays fallback gas and measure the deadline budget.
- Exit tests: junk-first delivery, competing honest desks, false beacons, absent
  buyer confirmation and desk restart. Measure usable-key latency separately from
  irreversible settlement; do not advertise an unmeasured two-block guarantee.

### A07 - P1: The normal buy file cannot reach the signer

- Executed reproduction: `go run -modfile=go.local.mod . submit -prepared
  ../../output/cli/prepared-buy.json -nodekey /nonexistent-audit-key -nonce 1
  -tx-epoch 1 -max-fee 0` in `tools/fod` exits 1 with
  `json: unknown field "bundle"`. No key was read and nothing was broadcast.
- Evidence: `pool_buy.go:30` writes `bundle`, `nonce` and `note`;
  `submit.go:56` does not declare them, and line 135 decodes strictly. The tests
  construct smaller hand-written envelopes, missing this producer/consumer break.
  Confidence: high, reproduced against current source.
- Repair: use a consistent versioned prepared-action schema, retaining strict
  validation. Round-trip actual command outputs, not reconstructed test JSON.
- Exit tests: every prepare command -> submit -> decoded transaction using only
  disposable test keys. Check the displayed terms against calldata, verify the
  target network/contract, recheck buy preconditions before signing and observe
  transaction inclusion and resulting state before reporting completion.

### A08 - P2: Availability is a badge; the retrieval-fee path is absent

- Evidence: `tools/fod/pool_buy.go:192` rechecks only quote/nonce. `submit.go:384`
  does no content fetch. `app/src/views/Market.tsx:133` enables buys independently
  of the availability advisory. `availability.go:62` timestamps a report made
  from stored inventory flags as if it were checked now.
- `buy` has no host-payment list; `PoolBundle` does not persist the authenticated
  covered-flip count needed to enforce a fee cap. Confidence: high, source-proven.
- Repair: fetch every committed CID and validate its bytes before preparing and
  again as needed before signing; cache the actual fetch time, never relabel old
  flags as a fresh retrieval. Clearly label off-chain observations. Implement
  buyer-funded host credits capped by authenticated eligible count, with no
  reserve deduction or per-byte pricing. A client naming a host is not a
  decentralized proof of which physical server served the bytes.
- Exit tests: stale/forged advisory, one missing CID, bad CID bytes, unavailable
  host, wrong fee count, fee cap, duplicate host allocation, local-cache zero fee.

### A09 - P1: The private-session journey never displays a flip

- Evidence: `gateway/src/session.ts:64` returns challenge ID and CID only.
  `app/src/views/Session.tsx:223` renders that ID and a free-text answer field.
  `tools/fod/export.go:215` writes `fod.session-content`, whereas
  `gateway/src/pool.ts:47` accepts `fod.flip-pool`; no complete content adapter,
  provider setup journey or flip-image rendering connects them.
- Passing fixture answers proves scoring/signatures, not that a customer can
  solve a bought flip. Sellers and buyers still manually move files through
  several CLI commands. Confidence: high, source-proven.
- Repair: connect verified purchased content and authenticated answer provenance
  to the provider session, display the actual image sequences with simple
  choices, and provide the smallest guided sell/give, buy and session-setup
  flows. Keep secrets in the local companion and guests free of wallet/CLI work.
- Exit test: one integrated play-money sale/purchase/restart/session run using
  a supported real flip format. A new participant solves visible flips; the
  utility verifies the result. Include timing (including opensAt), expiry and
  access-policy tests. Provider-issued results remain provider assertions;
  ownerless marketplace operation does not make provider scoring trustless.

### A10 - P2: Status and release evidence overstate completion

- Evidence: the former `PROGRESS.md` summary marked packages 0-8 done and all
  remaining work external. `release/README.md` marked pool invariants verified;
  its registration row described a gate not present in the contract. The root
  README still described the already removed offer slice and O-10 as open.
- These papers are corrected by this review. Historical gas and proof records
  remain measurements of their stated versions; they are not evidence for a new
  proof statement or repaired contract. Missing artifacts must fail release
  verification, not silently skip as `sealed_artefact.rs:30` currently allows.
- Repair: close the findings with evidence for the final source, regenerate guest
  IDs/seals/WASM/HTML and matching CLI constants, then run the targeted final
  integration once. Do not call a fixture receipt proof of chain authenticity.
  Preserve MIT for original code and verify dependency notices/distribution
  obligations before packaging; this pass is not a license opinion.

## Fresh Verification

| Check | Result and boundary |
|---|---|
| CLI `go test -modfile=go.local.mod -count=1 ./...` | pass, 14.565 s; does not cover A07's actual file handoff |
| Harness `go test -modfile=go.local.mod -count=1 -run 'Test(Pool\|Escrow\|Release)' -v .` | 11 pass, 2.184 s, no skips in this selection; test-hook lifecycle and release export/deploy checks |
| Registration core `cargo test --locked --offline -p fod-registration-core` | 15 pass; decryption, qualification and CID vectors, not authenticated session provenance |
| Anchor core `cargo test --locked --offline` | 1 pass; a real path folds to its root, not proof of root authority or witness binding |
| Gateway / utility `npm test -- --test-reporter=dot` | 16 / 11 pass |
| App `npm run typecheck` | pass; no new browser journey certification |
| `node release/check-integration.mjs` | 9 checks pass with the example session; no real flip rendering or marketplace transaction |
| Prepared-buy -> submit | fails as A07 describes, before any key or network access |
| Recorded artifact SHA-256 checks | both match the existing manifest; not a fresh clean-source rebuild |
| Released-runtime native attempt | unavailable here: arm64 linker vs x86_64 archive, then amd64 binary cannot execute; no installation or remote workaround attempted. The Linux recipe remains in `spikes/README.md` |
| Tracked source private-path/identity pattern scan | no matches for the selected private path, host and identity patterns; not a complete history-erasure audit |
| Gitleaks, 204 local commits since recorded remote | 8 generic-api-key matches, all inspected as 21-byte public IAVL `keyHex` fields in three proof fixtures; no credential identified among those matches. No broad allowlist added |

The audit did not regenerate expensive Groth16 proofs, rebuild/redeploy a node,
access wallets or remote hosts, rerun the broad browser suite, or certify the
published Git history. No external publication is authorized by this review.

## DeepSeek Finish Queue

The replacement objective is in `PROGRESS.md` section 0. The requirements and
milestone exits in `SPEC.md` 10.2 govern this queue.

1. **D0: Fail closed and capture regressions.** Prevent unsafe paid supply and
   unsafe payable rejection; reproduce A01/A03/A04/A06/A07 at public boundaries
   with disposable fixtures. Keep test hooks out of release builds. This is
   locally actionable now and does not wait for a ceremony or UI preferences.
2. **D1: Establish the chain trust root.** Close A02 with a concrete legacy-chain
   design and adversarial witness tests. Bound the experiment: identify committed
   inputs, implement one vertical proof, measure runtime and verifier portability.
   If a required commitment is missing, stop that lane with exact evidence and
   a decision-ready explanation. Do not add an operator oracle as a workaround.
3. **D2: Pin the economics and repair accounting.** Present the A05 finite-supply
   proposal and liquidity scope for acceptance. Implement A01/A03/A04 invariants
   for that model. Calculate maximum subsidy and keep buyer refunds segregated.
4. **D3: Finish delivery, availability and host fees.** Close A06/A08, define the
   fast-path guarantee and costs, and prove uncooperative-buyer recovery. Do not
   use a fixed fee value or weaker instant-settlement promise as implicit consent.
5. **D4: Finish the actual journeys.** Close A07/A09 with real command round trips
   and a minimal local companion workflow. Keep the offline HTML; do not expand
   into an unrelated dashboard or centralized marketplace backend.
6. **D5: Assemble and verify once.** Build coherent final artifacts from pinned
   inputs, verify real receipts against the built guest IDs and release module,
   exercise the full play-money flow and recovery, check privacy/notices, and
   record exact revision, commands, results and remaining limits. Mainnet remains
   a separate, explicitly approved phase.

For each package, record only: finding IDs, files/revision, focused commands,
result, remaining limitation, next task. Complete the change before the relevant
test batch; rerun only after a relevant change/failure or a concrete unresolved
risk. Fixture tests can prove regressions but never real-session provenance.
Stop repeated polling or documentation churn when external inputs are unchanged.
Do not rewrite old evidence into a stronger claim.
