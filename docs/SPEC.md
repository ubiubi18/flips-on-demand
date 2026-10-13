# Flips on Demand - Authoritative Specification

Status: repair specification after the 2026-09-26 chief developer review.
The immediate target is an isolated play-money MVP; mainnet is a later phase.
Derived from the consolidated implementation plan supplied 2026-09-25
(source hash recorded in `SOURCES.md`).

Language: this document, code, comments, commit messages, and technical
documentation are English. User-facing German text is out of scope for the
repository documents.

Normative terms: **must** = required for the first usable release, **must not** =
prohibited, **should** = default unless a recorded reason overrides it.

Review precedence: `REVIEW.md` records defects at `a91e475`; `PROGRESS.md`
records the revised objective and current decisions. Section 10.2 is the active
implementation sequence. Descriptions of existing interfaces elsewhere are not
evidence that the behavior is safe or complete; disputed pool semantics must be
resolved before paid supply is enabled.

---

## 1. Purpose

Build **Flips on Demand**, a decentralized marketplace for selling the original
decryption keys of eligible historical Idena flips.

Distribution model:

- The application is a self-contained HTML file that opens directly through
  `file://`.
- A local CLI (`fod`) handles secrets, proof generation, node access, and
  marketplace transactions.
- A native Idena escrow contract releases payment only after verifying correct,
  buyer-bound encrypted delivery.
- Providers may operate shared Idena nodes and session services and distribute
  local client files to customers for gated utility sessions.

Implementation model: DeepSeek V4.1 Flash.

## 2. Product decisions

| Topic | Requirement |
|---|---|
| Application | Self-contained `flips-on-demand.html`, directly openable through `file://` |
| Hosting | No required hosted website, central backend, CDN, or hosted indexer |
| Distribution | File copying, release archives, optional IPFS |
| Local companion | CLI named `fod` |
| Provider deployment | Providers may run shared Idena nodes and session services |
| Customer experience | Participants receive a local client and configuration; no own node, no marketplace CLI |
| Settlement | Native Idena smart contract using iDNA |
| First usable release | Real mainnet payments alongside development networks |
| Sale unit | Original flip key pair for one author and one completed epoch |
| Eligibility | All covered mainnet-submitted flips have strong consensus and no final `Reported` classification |
| Verification | Cryptographic verification without a trusted marketplace operator or verifier quorum |
| Sales | The canonical market is one ownerless pool: it holds an iDNA reserve and paid-for sale units for each listed bundle, quotes a price from their ratio, and settles a purchase by a buyer-confirmed delivery (5.1, 7.2) |
| Privacy | Keys remain private; addresses, epoch, CIDs, quotes, and sale status are public. The contract holds no keys |
| Marketplace commission | Zero |
| Additional contribution | Only a verified requirement to fund necessary contract stake; otherwise zero |
| Network fees | Ordinary Idena transaction fees, displayed before signing |
| Documentation | Offline FAQ covering purchases, provider deployment, gated utility, AI, costs, security limits |

### 2.1 Economic premise

The service assumes that flips with authenticated Idena consensus can have
economic value. AI can generate and solve flips; generation, solving, quality
control, validation, and consensus participation consume resources, and those
costs can be substantial at scale.

Requirements:

- The product must not claim that automation is impossible.
- The product must not claim that every automated solution has a fixed minimum
  cost.
- Actual costs depend on models, hardware, accuracy requirements, reuse,
  caching, infrastructure, and operational efficiency, and must be described as
  measurable rather than assumed.
- The market determines value: the pool's reserve against the sale units it has
  bought sets the quote, and that quote falls as sellers bring keys and rises as
  buyers take them. Buyers decide what content, provenance, consensus evidence,
  and intended utility are worth. No author sets a price, no operator earns a
  share, and no contract-enforced floor exists (5.1).

### 2.2 First-release scope

Included:

- Key marketplace as an ownerless pool, with escrow and settlement.
- Local key/proof CLI.
- Locally opened HTML interface.
- Shared-node connectivity and provider configuration.
- Offline FAQ and session-integration guide.
- Standardized content export and participant integration format.
- Small reference provider integration: challenge delivery, answer submission,
  provider-signed result.

Excluded (separate integrations): asset-specific distribution contracts,
complex campaign builders, independently verifiable session-scoring protocols.

The reference integration demonstrates how providers connect components. It is
not a new decentralized consensus protocol and not a general-purpose
utility-management platform.

## 3. Architecture and deployment modes

### 3.1 Components

| Component | Responsibility |
|---|---|
| Local HTML interface | Browse listings, inspect public evidence, prepare actions, display trades, read FAQ |
| Go CLI (`fod`) | Keys, vaults, node communication, action validation, proof execution, transaction submission |
| Local evidence builder | Reconstruct complete historical submissions and validation results |
| Rust proof programs | Prove eligibility, original-key correctness, buyer-bound encryption |
| Rust WASM contract | Register bundles, hold the reserve, verify delivery, settle sales, refund |
| Local public index/cache | Chain records and encrypted flip blobs |
| Provider gateway | Restricted shared-node and session operations for customer clients |
| Reference session adapter | Participant enrollment, challenge assignment, answer collection, signed results |

Repository: this standalone repository (`flips-on-demand`).

Constraint: the existing IdenaAI repository and its nested node checkout are
reference sources only and must not be modified by this implementation.

### 3.2 Reuse from idena.social

Reuse where suitable: React/TypeScript UI patterns, Idena transaction
serialization, receipt and event handling, IPFS integration patterns, relevant
encryption interoperability knowledge. Preserve required attribution and
licenses.

Build a separate escrow contract. The social tipping contract does not provide
escrow, and the vendored source differs from the deployed contract version its
UI references.

### 3.3 Mode A - independent marketplace operation

```text
Local HTML <-> public files <-> local CLI <-> own Idena node
```

The CLI synchronizes listings and receipts, exports public state, and validates
every action against current chain state before submitting it. This mode must
work without a hosted marketplace service.

### 3.4 Mode B - provider-operated shared infrastructure

| Provider operates | Customers receive |
|---|---|
| Shared Idena node | Local HTML client |
| Restricted HTTPS gateway | Connection and session configuration |
| Session coordination and scoring | Challenge interface and participation rules |
| Utility integration | Instructions for using or claiming the utility |

Rules:

- Session participants do not need the marketplace CLI.
- The provider uses the CLI to acquire and prepare flips.
- Customers use their own wallet when an operation requires their signature.
- The provider must not sign customer transactions with the node operator
  wallet.

### 3.5 Hosting and decentralization

- The HTML application must remain distributable independently of any provider.
- A provider gateway supplies connectivity and application services; it must not
  become an authority that can approve invalid marketplace proofs or redirect
  escrow.
- Providers must be replaceable or independently operable.
- Public data must be reconstructable from the chain.
- Provider availability and provider-controlled session scoring are explicit
  trust assumptions for that provider's utility.

## 4. Local HTML, shared-node access, and public interfaces

### 4.1 Self-contained HTML

Built with React and TypeScript, bundled into one HTML file containing scripts,
styles, icons, and FAQ.

Requirements:

- Opens by double-clicking the file.
- Hash-based navigation.
- Loads no remote JavaScript, styles, or fonts.
- Requires no local HTTP server for the independent file-based workflow.
- FAQ is available offline.
- Displays source, timestamp, and block height of imported state.
- Treats imported files, public text, and gateway responses as untrusted input.
- Never executes HTML embedded in public data.

Prohibited: analytics, remote error reporting, automatic code loading, hosted
login service.

### 4.2 Minimal views

| View | Content |
|---|---|
| Market | Listings: what is inside, the pool's quote, the last availability check, the capped retrieval fee, and the delivery window |
| My trades | This viewer's orders, deliveries, refunds and withdrawals, plus the bundles it can sell or give, the delivery desk state, and earnings |
| Use flips | The customer-facing gate: try the built-in example, import a connection file, enrol, answer the pool's challenges, verify the provider-signed result, and expand "Set up a gate for your use case" for the faucet/dapp operator journey. The pool it serves is built by the local companion from the user's own keys (`fod session-pool`) |
| FAQ | Offline usage and economic explanations |

Technical evidence uses expandable details. Primary actions stay visible. The
interface stays visually calm. A browser-local session with no provider process
is not implemented: the Use flips view always talks to a gateway the participant
starts locally or was given, and that gateway is replaceable and never holds
marketplace authority.

### 4.3 File-based marketplace workflow

1. CLI synchronizes public marketplace data.
2. User imports exported state into the HTML.
3. HTML produces a typed action-request file.
4. CLI rechecks current chain state and validates the request.
5. CLI displays exact operation, contract, amount, and estimated fee.
6. After confirmation, CLI submits through the local node.
7. CLI exports updated state and receipts.

An imported snapshot is for browsing and must never be sufficient
authorization for a financial action.

Action request v1, the file the interface hands to the CLI:

| Field | Meaning |
|---|---|
| `format` / `version` | `fod.action-request` / `1` |
| `network` | the network the snapshot came from |
| `contract` | marketplace contract address, 20 bytes hex |
| `operation` | supported operation from the interface: `buy`, `donateUnit`, `confirm`. The contract's `sellUnit` entry point stays refused under the no-subsidy rule and the interface does not offer it. The CLI also prepares the delivery desk's `deliver` and the proof fallback `fulfill` without an interface request |
| `arguments` | `buy`: `bundleId`, `expectedQuote`, `buyer`. `donateUnit`: `commitment`. `confirm`: `bundleId`, `orderId`, `ciphertextSha256` and `ciphertextHex` - the digest and the exact bytes the buyer checked; the CLI checks the pair, and the escrow records those bytes as the delivery (7.1, `AH-10`) |
| `amount` | explicit amount in iDNA base units, decimal string |
| `createdAt` | when the interface wrote the file |

The request names what the user intends and carries no key material: the buyer's
delivery key stays with the CLI, which builds the possession proof and the exact
call arguments the contract expects, checks what can be checked locally, shows
the operation, contract, and amount, and only then asks the node to submit. A
request that does not match local keys or the contract's current state is
refused rather than adjusted.

A `fulfill` request is the seller's mirror of the same workflow: the CLI derives
the bundle's epoch secrets from the seller's nodekey, hands them to the delivery
prover, and writes the call arguments the escrow settles with. The witness stays
on the seller's machine; the request itself carries no key material either.

`donateUnit` carries no key material of its own: a donation is accepted only
from the bundle's registered author, because a free unit is one more claim on
the same reserve and an outsider's would move the quote without paying for it.
The contract's `sellUnit` entry point stays refused under the no-subsidy rule,
so the interface does not offer it. The `buy`
request's expected quote is a guard, not a price: the CLI refuses to sign if the
pool's current quote differs.

### 4.4 Shared-node gateway

A small TypeScript gateway with narrowly defined endpoints, not unrestricted
RPC forwarding.

May expose: public marketplace and session state, participant enrollment,
assigned challenges, answer submission, session results, submission of
supported already-signed transactions where required.

Must keep inaccessible: node administration, wallet export, unrestricted
signing, operator credentials.

For local HTML access:

- Use HTTPS.
- Configure browser cross-origin handling explicitly.
- Do not treat a `file://` origin as proof of authorization.
- Use scoped, short-lived participant credentials where required.
- Avoid ambient cookie authentication.
- Apply request-size and rate limits.
- Bind identity-sensitive requests to the configured authentication mechanism.

Connection files contain public endpoint and protocol information. A node's
unrestricted API key must never appear in a distributable client or shared
configuration file.

### 4.5 Public data types

Versioned types are required for:

| Type | Core fields |
|---|---|
| `BundleManifest` | Network, author, epoch, complete sorted CIDs, validation anchor, consensus version, eligibility-policy version |
| `BundleRecord` | Bundle ID (the key commitment), manifest commitment, seller, registration proof, the pool's reserve and unit count for this bundle, beacon state, strike count (see 5.1, 7.2) |
| `Listing` | Bundle ID, quote for one unit, units left, age of the delivery desk's beacon |
| `DeliveryDesk` | Bundle ID, beacon state, orders served, strikes, earnings of the key holder |
| `PurchaseRequest` | Bundle ID, intended buyer, order nonce, expected quote. The delivery key and its possession proof are built by the CLI when the request is prepared and never travel in the request (see 4.3) |
| `Order` | Bundle ID, buyer, delivery key, nonce, paid amount, deadlines, delivery and confirmation state |
| `Delivery` | Order ID, ciphertext, proof when a seller settles without the buyer, proof-program version |
| `MarketplaceState` | Network, contract/code hash, observed block, listings, per-bundle reserve, pending buyer escrow and unit counts, orders, credits, and `stateDigest`: a SHA-256 over the snapshot in the schema's own key order with `observedAt` and the digest itself removed, so two operators of the same chain compare one line instead of diffing JSON |
| `ActionRequest` | Version, network, contract, supported operation, typed arguments, explicit amount (see 4.3) |
| `ProviderConfig` | Provider endpoint, network, supported protocol, session identifier, authentication requirements |
| `SessionPolicy` | Utility identifier, timing, scoring rules, participation restrictions, result issuer, claim destination, and `contentDigest`: the SHA-256 over the rules, the stated provenance and every challenge with the CID, answer digest and content it scores against |
| `SessionResult` | Utility/session identifiers, that same `contentDigest`, participant binding, the per-challenge `transcript` of the answers the provider scored, outcome, expiry, issuer signature |

Encoding rules:

- Public JSON files are transport wrappers.
- One canonical encoding for commitments and proof inputs, frozen and verified
  with cross-language fixtures.
- iDNA amounts are integer base units, decimal strings in JSON, display
  conversion at the edge. No floating-point arithmetic for accounting.

### 4.6 The three journeys

Selling keys, buying keys, and using bought keys in a private flip session must
each work without blockchain knowledge and without reading a manual. In each
journey the client shows one primary action per screen, plain-language labels,
the amount, and the fee before anything is signed.

| Journey | What the user does | What the client must never require |
|---|---|---|
| Sell or give keys | The local tool finds the bundle; one screen shows what is inside, the pool's quote, and the choices Sell and Give; Confirm. One switch, Keep serving, then runs the delivery desk and shows live or stale, orders served, and earnings | Setting a price, editing files by hand, computing a fee, knowing what a nonce, gas, or a contract call is, starting the desk again for every order |
| Buy keys | One list of listings with contents, price, the capped retrieval-fee estimate and the last availability check; Buy; save the offered recovery file; My trades then shows the order as keys ready, and the delivery is confirmed in the same screen | Pasting keys, choosing amounts, reading raw JSON, understanding gas, working a terminal |
| Use the keys | My trades hands the bought keys to the companion, which builds a session pool (`fod session-pool`); the Use flips view then shows the flips, collects answers, lets the participant verify the provider-signed result the local utility decides on, and links from a keys-ready order; the same view carries the example gate and the operator setup journey | Buying anything else first, creating or renewing an identity, any claim about personhood |

Requirements:

- The buy journey states the maximum cost and the expected delivery window
  before anything is signed: the quote, the optional retrieval fee (capped at
  one iDNA per authenticated eligible flip and paid only to the hosts the buyer
  names), and the delivery window in wall-clock terms derived from the node's
  observed block interval when it reports one. It distinguishes waiting for the
  buy transaction to be included from the order's three-block delivery
  deadline, and says that the local companion claims the refund automatically
  when the window closes without settlement. A chain that stalls can take
  longer in wall-clock time; zero latency is never promised.
- All three journeys run in the browser walk (`app/verify.mjs`) against the
  built file, with the local companion as the only helper the user starts.
- Every screen states what to do next. A missing next step is a defect, not a
  documentation gap.
- A journey that needs a decision the user cannot make from the screen - a
  contract address, a price, a nonce, or a proof file - is a defect.
- The offline FAQ answers what the journey screens assume (section 9).

## 5. Eligible bundles and cryptographic verification

### 5.1 Sale unit and the pool

Idena uses shared flip encryption secrets across an author's flips within an
epoch. Original-key sales therefore cover an author-epoch bundle, not a selected
individual flip. Validation participants may already know those keys; neither
exclusive knowledge nor prevention of copying is promised.

The target remains an ownerless liquidity market, bootstrapped voluntarily with
flips and play-iDNA during testing. No administrator, privileged market maker,
commission recipient, LP revenue share, upgrade key or trusted inventory issuer
may be introduced. The pool stores public commitments and accounting, never
plaintext decryption keys.

The earlier per-bundle product-pool implementation is **not an approved safe
economic model**. REVIEW.md A01/A03/A05 demonstrate that publicly reusable
receipts, unlimited copy issuance and spendable refund money invalidate its
safety claims. Its quote formulas, `ceil(r/(q-1))` for buys and
`floor(r/(q+1))` for sells, are properties of the current prototype, not proof
that arbitrary trade sequences are safe. A monotonically increasing product
does not prove authorization, bounded subsidy, useful new supply or solvency.

The replacement model must meet these constraints before paid supply is enabled:

- Registration binds a canonical author, epoch and complete eligible inventory
  to authenticated legacy-chain session evidence (5.5). A public registration
  receipt alone grants no right to collect money.
- The exact meaning and maximum issuance of a unit must be explicit, derived
  from authenticated inventory, and enforced with consumed entitlements or
  nullifiers. Repeated copies, accounts, proofs or donations cannot mint new
  reserve-funded eligibility. A caller-bound knowledge proof does not by itself
  make copyable keys scarce.
- The proposed conservative model is one reserve-funded acquisition per canonical
  author/epoch bundle, weighted only by authenticated eligible-flip count.
  Later delivery copies are distinct from new subsidized inventory. This
  proposal and global versus per-bundle liquidity require the product decision
  recorded in PROGRESS.md; do not silently impose either.
- Bootstrap funding must have a documented effect on other authors' ability to
  sell. Funding one isolated bundle must not be presented as funding all sellers.
- A gift creates no donor payment or revenue entitlement. Donation authorization,
  finite quantity and any withdrawal/delisting rule must be explicit.
- Pending purchase funds are refund escrow, unavailable for supply payouts or
  maintenance. Settlement, refund and withdrawal consume their own liabilities
  exactly once (7.3).
- Quotes, rounding, slippage bounds, zero-inventory behavior and listing-clear
  behavior are deterministic and covered by adversarial tests. State a maximum
  subsidy; never assume honest sellers will voluntarily stop exploiting payouts.
- No reserve-funded supply is enabled while authentication, issuance or solvency
  is unresolved. This is a release/build precondition, not an administrator's
  runtime switch.

A key holder delivers buyer-bound ciphertext off chain or through the contract's
public ciphertext record. Permissionless participation must still authenticate
the operation: only possessing public calldata is insufficient. A liveness
beacon is an advisory of a serving participant, never proof that IPFS data or
usable keys will be available. Only the bundle's registered author may set it:
a fresh beacon is what makes a bundle buyable, so a stranger's beacon could lure
a purchase into a refund strike against the author. An outsider must not block valid fast delivery by
publishing junk first. Delivery and final settlement are separate milestones
until the instant-settlement requirement is explicitly resolved (7.2).

The marketplace cannot enforce off-platform copying, redistribution or prices.
Blob availability is observed and verified at the client before payment (5.8);
the contract enforces deterministic state and liabilities only. No chain
consensus change is allowed.

### 5.2 Complete inventory

Bundle identity: `source network + author address + epoch`.

Requirements:

- Reconstruct every accepted flip submission by that author during the epoch.
- Include subsequently deleted submissions: their ciphertext may remain
  available and share the same keys.
- Deduplicate repeated CIDs while retaining submission evidence.
- Do not accept a seller-selected subset containing only successful flips.

Every covered flip requires: authentic mainnet submission by the author, a
successfully completed validation, strong qualification under that epoch's
actual rules, a left/right consensus answer, no final `Reported`
classification, and available original ciphertext matching its CID.

Reject the bundle if any covered flip fails or cannot be verified.

Note: "no final `Reported` classification" does not mean zero individual report
votes.

### 5.3 Authenticated historical evidence

An explorer response, operator signature, or locally generated report is
insufficient. Required historical inputs must be authenticated against
canonical Idena commitments: relevant headers and block bodies, transaction
commitments, state snapshots and proofs, epoch boundaries and validation
completion, and the consensus rules active for the supported epoch.

An indexer such as `api.idena.io` is a permitted Plan B *source* for a user who
cannot read a node's database, under two rules. First, whatever it supplies must
be reproducible or falsifiable from canonical commitments by someone who does run
a node: a state root is comparable with the header at the block the epoch opened
from - which is the block the epoch's record calls its start, and *not* the first
block of its validation (`SOURCES.md` F-82) - a submission is comparable with the
transaction that carried it, and an answer count is a consequence of the epoch's
assignment. Second, an
indexer-only result is never recorded as verified: the tooling names the source
and refuses, rather than accepts, what it cannot cross-check.

A third rule follows from the second: an indexer-only route may not be the only
route that exists. Everything the tooling writes from an indexer carries the
source, the epoch's state root with the height it belongs to, and whether a node
confirmed that root against its own header - a note when none did, and a refusal
where the caller asked for confirmation. The confirmation is the one an ordinary
node can give: headers are kept after state is pruned, so a state root stays
checkable long after the epoch closed.

The two routes are therefore named in the tooling and never merged. The node
route builds a bundle from a committed-body sweep. The Plan B route builds a
listing from an indexer's per-identity flip records and its per-flip records,
which name the transaction that carried each submission and the block and height
it landed in; every one of those anchors is checked against a node when the
caller names one - the block hash against the node's header, the transaction
against the node's own copy of it, including the CID in its payload - and a
contradiction refuses the listing. Each blob is fetched from the node's IPFS or
from a gateway the caller names, and is checked against its CID by locally
rebuilding a supported deterministic raw or UnixFS/dag-pb importer profile
(`SOURCES.md` F-86, F-88). An unknown profile or mismatching bytes are refused;
the same RPC that supplied the bytes cannot certify them with `ipfs_cid`.
A Plan B listing records what confirmed it and is never recorded as complete;
without a node it stays unverified, and a caller who
asked for confirmation gets a refusal rather than a listing.

What an indexer cannot supply at all stays refused rather than approximated. The
epoch's lottery assignment and the ceremony's candidate order are state, not
chain data, and no explorer publishes the shard a candidate was sorted into
(`SOURCES.md` F-79, F-81). A user without a node therefore obtains them from a
capture, from someone who published one, or not at all - the Plan B route widens
who can check a bundle and what they can check, and does not remove the capture.

The long answers are the one historical input with two routes, and both are
used. A capture's epoch store holds the payloads the node saw, and the epoch's
own transactions hold the same payloads, which a sweep reads out of
CID-committed block bodies. The reconstruction takes both when both are present
and refuses when they disagree about a candidate's answers, because what a
candidate answered is not something this tooling may choose between two versions
of. The transaction route is also the one that survives the epoch: a node clears
its store when it crosses into the next one, so a seller whose capture was taken
late still reconstructs from the chain.

An optional shared-node operator can run `tools/fod/session_indexer.py` beside
ordinary nodes. During the ceremony it records the lottery seed and candidate
order from each node's epoch store; at the boundary it saves compact public IAVL
anchors for the author's final inventory and post-validation verdict before
historical state is pruned. A failed extraction retains a private state snapshot
for retry; successful capture discards it. The lottery records are consistent
local node observations, not cryptographic membership proofs. The seller need
not run this process or hand over an identity key merely to make public
evidence available. The saved anchors and CIDs remain untrusted until a buyer
authenticates their roots against canonical block headers and checks the content
bytes locally. Committed-body sweeps, ciphertext availability and historical
qualification remain separate inputs. Two nodes on one host do not establish
independent-operator availability.

`api.idena.io` may be selected as a convenience source for discovery and chain
data. An API-only buyer is explicitly trusting that service for its chain view:
show the source and risk before any signed purchase, never label its statements
independently verified, and never let an API answer bypass contract checks,
content hashing, the signed maximum, or the buyer's refund right. This opt-in
also exposes the buyer's IP address and queried wallet/listings to that API;
private keys, node credentials and decrypted content must remain local. The
API may omit, delay or misstate data, so a purchase prepared from it must show
the exact contract, amount and recipient before local signing. The
funded path is a product requirement, not an implemented or qualified path in
the present CLI. A seller can instead wait for the next captured ceremony
with new flips and without locking buyer money; waiting cannot recreate the
pruned evidence for an old epoch. A validation ceremony is not an escrow
settlement or a substitute for the three-block delivery/refund deadline after
a buy.

Reproduce the relevant qualification process, including candidate assignment,
considered answers, report handling, committee-size behavior, and
protocol-version rules.

Eligibility must not be calculated from a seller-provided subset of votes.

Initially support one explicitly pinned consensus version. Unsupported epochs
must fail with a clear explanation. The covered range is epochs 104 and later,
where consensus V11 activated upgrade 10 and upgrade 11 - the two switches a
flip's verdict depends on (`SOURCES.md` F-84).

### 5.4 Proof-system candidate

One bounded candidate:

- RISC Zero for local execution proofs.
- Groth16 receipts for compact verification.
- Established arkworks components for the Rust verifier.
- Minimal adapter for Idena's existing WASM ABI.

Immutable verification parameters and accepted program identifiers are embedded
in the contract.

This combination must be demonstrated on Idena. Existing RISC Zero and arkworks
implementations do not establish that verification fits Idena's runtime limits.

Measured from the pinned runtime source (see `SOURCES.md` F-8 to F-10): the
contract runtime charges a flat `1` wasm gas per executed operator, scales
`wasmGas = userGas * 100`, derives the call gas budget from transaction fee
headroom rather than a small fixed cap, caps guest linear memory at 6.25 MiB,
exposes no big-integer or pairing precompile, and rejects bulk-memory
operators. Verifier arithmetic therefore has to be implemented in WASM
instructions and measured against those exact limits.

Prohibited: new consensus rule, node host function, EVM bridge, paid proving
service, trusted verification server.

### 5.5 Registration proof

A valid registration proof must establish all of the following. Items 2 and 3
were the assignment-replay route; `SOURCES.md` F-97 replaced it with the
two-anchor bundle the owner accepted, so neither a candidate list nor an answer
replay is required for a bundle label.

1. The public journal identifies the chain, contract, epoch, the two anchored
   state versions it folds its evidence to - inventory height and state root,
   verdict height and state root - the verdict flags, and the consensus rule
   version. The contract authenticates both anchors against the chain the
   executing node itself holds. A prover-supplied root that folds correctly is
   not an authenticated root.

   The journal also publishes what a reader needs to check the content claim
   itself: the commitment over the bundle's CIDs, and the digest of the network
   the evidence came from. The escrow pins the network digest for the chain it
   serves and refuses a statement that names another, and a buyer recomputes the
   inventory commitment over a candidate CID set and compares it with the pool
   record. A value the fold hid cannot be checked by anyone, which is why both
   were published rather than left inside the manifest commitment
   (`SOURCES.md` F-100).
2. The inventory anchor's record lists exactly the flips the manifest commits,
   in the record's own order, and its global entry names the bundle's epoch, so
   the complete eligible inventory is the chain's own record rather than a list
   the seller supplied.
3. The verdict anchor is the same author's record at the epoch's
   `ValidationFinished` block, whose flags the guest reads and turns into the
   bundle's label; the seller cannot name a label.
4. The two records belong to consecutive epochs (the inventory's global entry
   says `Epoch == N`, the verdict's says `Epoch == N+1`), which is what makes
   them one session rather than two unrelated records.
5. Original encrypted blob bytes rebuild their authenticated CIDs. The committed
   original epoch secrets authenticate and decrypt those bytes.
6. The decoded content has the real legacy flip format the session renderer
   supports: raw nested RLP parts, four pictures, and two orders over them that
   differ, so a customer can see the pictures and choose between the two
   sequences (`SOURCES.md` F-98). The registration guest checks exactly this
   and refuses anything else; the recorded node-built blob whose plaintext is
   placeholder JSON is the regression that proves it.
7. The manifest, key commitment, authenticated eligible count and verdict policy
   are bound together in the public statement consumed by the contract.

The anchor mechanism of item 1 is measured rather than assumed: the contract
reads the header the executing node holds for each anchor height through the
runtime's own `env.block_header` import and compares the state root the journal
commits against it, for both anchors (`SOURCES.md` F-96, F-97;
`contracts/escrow/src/lib.rs`). A proof from another chain, or from a height the
verifier's own chain does not carry, has nothing to match. The release contract
compares the state root, which is what the evidence folds to; the spike in
`spikes/header-anchor` measured the fuller check - block hash, identity root,
seed and the `FlipLotteryStarted` flag - which is what established that a
contract can authenticate a session header without an oracle.

The evidence a bundle carries is smaller than the original plan assumed, and
this is the design the D1 lane implements (`SOURCES.md` F-97): the author's
state-tree identity leaf at a height in the flip phase proves the complete
submitted set (`Flips`, with the global leaf saying `Epoch == N`), and the same
leaf at the epoch's `ValidationFinished` block proves the chain's verdict flags
(with the global leaf saying `Epoch == N+1`). The assignment, the candidate
list and the answers are then not needed to sell a bundle, and the provable
claim is a bundle label - clean, contains reported, weak - rather than a
per-flip verdict, which the chain does not store. The owner's rule that
session-level verifiability is sufficient, and that reported flips may be
offered but not expected to sell, is what allows the label lane. The
assignment-replay design stays as the fallback only if the two-anchor bundle
fails its live test.

The explorer may distribute data and portable evidence, but explorer-only claims
must never authorize inventory or payouts. Verification must work with default
fast-sync nodes using portable proofs plus authenticated retained/current chain
anchors; proof production may need a historical capture. No appointed checkpoint
publisher or private approval gate may replace the chain trust root.

Use a bounded feasibility experiment before an expensive full proof. Establish
which legacy-chain commitments prove each required fact and measure the guest
and contract cost. If complete session provenance cannot be proven from those
commitments, report the exact unsupported property and stop the paid-market
lane. Do not quietly replace it with an audit, a capture operator or an oracle.

The identity private key must not enter the prover. Registration separately
requires control of the author address. Supply authorization is a different
operation: a public registration proof is neither a bearer credential nor a
proof of its present caller's knowledge (5.1).

### 5.6 Delivery proof

After funding, prove that the same committed original secrets were encrypted to
the order's registered buyer delivery key.

Bind the proof to: source network and marketplace contract; bundle ID, order ID,
order nonce; seller and paying buyer; buyer delivery public key; manifest and
key commitments; exact delivery ciphertext. The pool binds a delivery to the
order that paid for it; the escrow's pinned guest image and this binding move
together, and the release manifest records the pair.

Use a precisely specified, pinned ECIES implementation with compatibility
vectors. Different libraries named "ECIES" must not be assumed interchangeable.

Public proof outputs contain trade context and commitments, never plaintext
keys or secret derivation material.

Delivery statement v2, as implemented in `proofs/delivery` (v1 is historical):

- Secret material: the canonical `fod:epoch-secrets:v1` bytes - domain, epoch
  (2, big-endian), author address (20), the epoch's public-part private scalar
  (32), and its private-part private scalar (32).
- Commitment: SHA-256 over those bytes. The registration proof commits to the
  same value, which is what makes "the same secrets" checkable.
- Delivery encryption: the pinned node's repository-local ECIES with the
  `ECIES_AES128_SHA256` parameters on secp256k1 - ECDH, the NIST SP 800-56A
  concatenation KDF with SHA-256, AES-128-CTR, and an HMAC-SHA256 tag over the
  encrypted message - with the CTR initialisation vector taken from the second
  KDF block instead of a random draw. The wire format is unchanged, so the
  repository-local library decrypts a delivery without modification:
  ephemeral public key (65) || IV (16) || AES-CTR ciphertext || tag (32).
- Journal, 281 bytes: statement version (4) || marketplace contract (20) ||
  order id (8) || order nonce (8) || seller (20) || buyer (20) || buyer delivery
  key (65) || secrets commitment (32) || SHA-256 of the delivery ciphertext (32)
  || network-name digest (32) || funding height (8) || funding state root (32).
- The ciphertext itself is published alongside the proof, not proven in the
  clear: the journal binds it by hash, and the buyer checks the commitment after
  decrypting.

`fulfill` must refuse a receipt whose journal version, contract, order id, order
nonce, seller, buyer, or delivery key does not match the funded order, whose
commitment does not match the bundle's registered commitment, or whose
ciphertext hash does not match the submitted ciphertext. The network digest
must match this build's canonical `idena` pin; funding height must equal the
order's included height and the root must match that header on the executing
node. Missing headers refuse. Forks with different funding state roots are rejected; descendant
forks sharing that anchor still require a common canonical chain view. This
statement does not establish buyer storage or future chain availability.

Delivery medium: the only payload the marketplace carries is the encrypted key
envelope. Its plaintext is the 106-byte canonical secrets encoding above -
two secp256k1 scalars for one author-epoch - and never flip content. The
encrypted blobs stay at the CIDs the manifest names, where anyone can fetch them;
the buyer opens them locally with the delivered keys. The envelope travels
through contract state (7.3), so recovery needs no private message channel, no
operator mailbox, and no direct contact between buyer and seller.

### 5.7 Feasibility evidence

Before developing the complete UI, demonstrate:

| Check | Required result |
|---|---|
| Native verification | Real proof verifies in the actual Idena WASM runtime |
| Rejection | Altered proofs, outputs, program IDs, contexts fail |
| Historical reconstruction | Eligible and ineligible cases reproduce from authenticated inputs |
| Completeness | Omitting a submission cannot produce an accepted proof |
| Key correctness | Wrong secrets or modified ciphertext cannot pass |
| Buyer binding | Substituted keys and replayed orders cannot pass |
| Protocol limits | Deployment and fulfillment fit actual network limits |
| Local proving | Time, memory, output size measured |

Usability targets: registration within 60 minutes, delivery proving within 30
minutes, peak memory within 16 GiB on a documented machine. Deviations are
reported separately from hard protocol failures.

Real network fees must be measured. No arbitrary economic prohibition on
mainnet tests.

Development receipts do not count as verification evidence; release builds must
reject them.

If a required property or native limit cannot be satisfied, stop the dependent
implementation and report the exact blocker. Do not silently weaken the design.

### 5.8 Availability of listed content

A listing is sellable only while the flips it names can actually be fetched from
the CIDs its manifest commits to. Whether a node serves a CID is network state,
not chain state: a contract reading it would differ between validators and break
consensus, so availability is checked off chain, the same way the tooling
already verifies that a blob rebuilds its CID (`SOURCES.md` F-88).

- `fod inventory` fetches every blob of a bundle and rebuilds its CID, and
  `-availability-out` writes what it saw: one entry per flip with the bytes, the
  source, and the time that check was made. The advisory's own time is the
  oldest of those checks, so writing or copying the file later cannot make an
  old check read as a fresh one, and a flip whose check recorded no time leaves
  the field empty rather than borrowing the clock.
- Anyone may refresh that advisory: permissionless, and never authoritative. The
  file says so itself, and it says who fetched the bytes (`note`, `source`).
- `fod prepare-buy -request ... -availability FILE` repeats the check with the
  buyer's own node before the call is built: every CID the advisory names is
  fetched and rebuilt, a CID that cannot be fetched or does not rebuild is a
  refusal that names it, and the set is compared with the inventory commitment
  the registration statement published: a substituted, omitted, duplicated or
  wrong-epoch entry changes the recomputed value and is refused, and an
  advisory naming another author than the pool record's is refused too. The
  prepared action records the check it made - its time, the counts, the
  advisory's author and epoch, and `binding: "committed-inventory"` - so the
  buyer can audit what was seen before signing. The advisory is required:
  without one nothing is fetched, so `prepare-buy` refuses rather than writing
  an unchecked buy, and `fod submit` refuses to sign a buy file whose check is
  missing, incomplete or older than fifteen minutes. Re-running prepare-buy is
  how a buyer refreshes a stale check.
- The contract enforces what chain state can carry: the beacon, the deadline,
  the refund, and the strike (7.2).

An unavailable bundle fails closed at the edge: no quote shown, no funding
transaction, no payment.

### 5.9 Retention and proof portability

Default nodes do not keep history: fast sync is the default, blocks and flips are
pinned only up to a configured quantile, and the state tree keeps a fixed number
of recent versions, so no node retains a past epoch's identity state
(`SOURCES.md` F-94).

- Verification must work on a default node. The registration proof, a portable
  anchor bundle, and the current headers are enough. No check may require
  historical state, and no check may require a node that was live during the
  epoch.
- Production may need history. Producing an anchor bundle needs either a live
  capture at the epoch or a replay of the chain. That is an off-chain service:
  the protocol names no required party, and it does not forbid a market for it.
- Blobs must stay reachable, and retention is demand-funded. The reserve pays
  nothing for pinning or for producing anchor bundles. The buyer pays a small
  retrieval fee to the host that served the flip data: the client names the
  hosts it fetched from, the availability advisory of 5.8 names the payment
  address each host advertises, and the client shows the fee before anything is
  signed. The fee is capped per covered flip, never by blob size, paid from the
  buyer's own payment, and never from the reserve; a purchase that names no
  host pays none.
- No consensus change. None of this adds a transaction type, a host function,
  or a rule the legacy chain has to adopt (5.4). The overlay reads what the
  chain already commits.

## 6. Local key handling and marketplace workflows

### 6.1 CLI commands

| Command | Purpose |
|---|---|
| `sync` | Update local index and export public application state |
| `inspect` | Show historical submissions, eligibility, missing evidence |
| `prepare-offer` | Derive keys, validate content, generate the registration proof that registers a bundle |
| `prepare-buy` | Create the buyer's delivery key and prepare the pool purchase request, rechecking the quote and nonce, and fetching and verifying the bundle's content first against the chain-bound advisory, which is required (5.8); `submit` refuses a buy file whose check is missing, incomplete or older than fifteen minutes |
| `session-pool` | Turn a session-content export into the flip pool a provider's gateway loads (6.7) |
| `prepare-registration` | Turn a sealed registration receipt into the `registerBundle` call |
| `prepare-fund` | Turn the funding profile into the pool's `addReserve` call |
| `prepare-deploy` | Build and sign the release module's deploy transaction |
| `prepare-list` | Turn a holder's sell or give request into the pool call that adds one sale unit |
| `prepare-confirm` | Turn a verified delivery into the buyer's confirmation call |
| `serve` | Watch the pool for orders on the bundles whose keys it holds, publish the buyer-bound ciphertext, fall back to the delivery proof near the deadline, and keep the delivery desk's beacon fresh |
| `fees` | Show the measured gas per escrow operation and the fee it implies at the chain's rate |
| `market` | Read the pool's storage and write the marketplace state the interface imports |
| `submit` | Sign a prepared action into a node transaction, and send it only when asked |
| `receive` | Verify, decrypt, display purchased flips locally |
| `export-keys` | Explicitly export flip secrets into an encrypted archive |
| `export-session` | Export content and provenance for a provider integration |
| `refund` | Prepare and submit an eligible refund |
| `withdraw` | Withdraw available contract credit |
| `withdraw-reserve` | Withdraw the caller's own reserve donation (`withdrawReserve`), the reserve's only outflow |

Imported requests must select from a fixed operation set and must not contain
executable commands or arbitrary privileged RPC calls.

### 6.2 Original-key access

First implementation accepts an explicitly selected local Idena `nodekey` file.
It must not search other profiles or automatically inspect unrelated
credentials. Derive the requested historical epoch's keys using the pinned node
algorithm and independent test vectors.

The existing `flip_getKeys` endpoint is not a generic historical owner-key
exporter; its private-key response is recipient-encrypted and depends on
ceremony state.

Use distinct names and types for identity keys, flip secrets, and delivery keys.

### 6.3 Vaults and recovery

Use passphrase-encrypted `age` files through its Go library for private vaults
and recovery archives. Read passphrases interactively, use restrictive file
permissions, and keep public caches separate from secret storage.

Do not log secrets, derivation signatures, passwords, plaintext archives, or
prover witnesses. Avoid plaintext temporary files. Keep proving local.

### 6.4 Seller workflow

Two ways to fund the pool with the same local material:

1. Select author and historical epoch.
2. Reconstruct complete submission inventory.
3. Review eligibility or exclusion reasons.
4. Select local key file.
5. Derive and verify original secrets.
6. Generate the registration proof that registers the bundle and its key
   commitment; this happens once per bundle.
7. Choose sell or give, and prepare the pool call that adds one sale unit.
8. Submit the call. `sellUnit` is refused under the no-subsidy rule, so a unit
   enters only as this author's donation and pays nothing when it is given.
9. Start the delivery desk and keep it running while willing to deliver: it
   watches the pool for orders on this bundle, publishes the buyer-bound
   ciphertext, keeps the beacon fresh, and settles a missing confirmation inside
   the window - the delivery proof when a prover is configured, the committed
   secrets (`reveal`) in the last blocks (`AH-19`).
10. When an order settles, the escrow pays the purchase amount to the author's
    address directly; the retrieval fee becomes the named hosts' credit, which
    `withdraw` pays out.

Current `Human` status is not required. Control of the original author address
and eligible historical content is required.

### 6.5 Buyer workflow

1. Browse synchronized listings.
2. Inspect author, epoch, flip count, the pool's quote, the evidence, and the
   last availability check.
3. Download and verify the original encrypted blobs before funding.
4. Generate a dedicated delivery key.
5. Save its encrypted recovery archive.
6. Prepare and submit the purchase at the quoted price, which the CLI rechecks
   against the pool immediately before signing.
7. Retrieve the recorded delivery: a live desk publishes it within the
   deadline.
8. Verify it locally. The decrypted secrets must hash to the bundle's key
   commitment, and the flips must match the manifest.
9. Confirm the delivery in one call, or let the seller's proof or the reveal
   settlement close the order without the buyer's cooperation.
10. Use the keys in a private session, or export the content.

The buyer's private delivery key must never go to the seller or shared node.
An order that receives nothing usable can be refunded after the deadline, and
the missed order counts a strike against the bundle (7.2).

### 6.6 Buyer key binding

Use a dedicated secp256k1 delivery key and a context-bound proof of possession.
The contract binds the key to the actual paying transaction caller. Do not rely
on an explorer-supplied public key or require a previous outgoing transaction.

Buyer key binding v1, as implemented in `contracts/escrow`:

- Delivery key: uncompressed secp256k1 public key, 65 bytes, the form the
  runtime's `ecrecover` returns.
- Possession proof: a `[R || S || V]` signature over the Keccak-256 digest of
  `"fod:delivery-key:v1" || contract address (20) || bundle id (8, big endian) ||
  order nonce (8, big endian) || caller address (20) || delivery key (65)`.
- `buy` argument: bundle id (8) || delivery key (65) || signature (65).
- The contract rebuilds the message, recovers the signer with the runtime's
  `ecrecover`, and refuses the funding unless the recovered key matches.
- The order nonce advances on every funding, so a proof cannot be replayed after
  a refund.

Loss of the delivery key is not a seller-failure refund condition.

### 6.7 Session-content export

Export: source CIDs, author, epoch; decoded images and presentation/order data;
authenticated consensus answers; eligibility and provenance references;
export-format version.

Original epoch keys are excluded from the session-content export by default. The
provider retains the keys and distributes only the challenge material needed by
participants.

Session-content export v1, as implemented by `fod export`:

| Field | Meaning |
|---|---|
| `format` / `version` | `fod.session-content` / `1` |
| `bundle` | author, epoch, flip count, manifest commitment, source inventory summary, whether that inventory was complete |
| `flips[]` | CID, pair, submitting transaction and height, and the decoded public and private parts: file name, byte count, SHA-256 |
| `answers[]` | CID, answer digest, and the evidence each answer rests on |
| `answersEvidence` | what the answers came from, or `none` |
| `keys` | always states that epoch key pairs are excluded |

The manifest commitment is `sha256("fod.bundle-manifest:v1" || author || epoch
(2, big-endian) || each sorted CID, NUL-separated)`, so the same bytes can be
committed to by a registration proof. Decoded parts are written beside the
manifest under `flips/<cid>.public` and `flips/<cid>.private`; both are mode 0600
on the exporting side.

Answers are never inferred. Mapping historical answers to flips needs the lottery
assignment, which is the same external data Package 2's eligibility half waits
on, so an export without evidence says `none` instead of guessing.

`fod session-pool` turns that export into the pool file a provider's gateway
loads: one challenge per flip, the answer digest the export evidences, and the
flip's own two parts as its content. It refuses a flip the export carries no
answer for (a challenge the provider cannot score is not a challenge), refuses a
part file that no longer hashes to the digest the export recorded, and writes
the pool mode 0600 because it holds the answer digests. The gateway checks the
content itself when it loads the pool (8.2). The export's `answersEvidence`
travels into the pool's `provenance.answers`, so the session can show a
participant what the provider says its answers rest on. That is a provider
statement, not a chain fact, and the interface shows it as one.

`fod shared-node` is the hosted node's combination of the two steps for one
author: from the author's epoch vault, the captured inventory and the saved
blobs it performs the export and builds the pool, then writes the
`fod.shared-node` connection the author imports, which names the node, the
author the escrow pays, the pool the gateway loads and the custody boundary
(`hosted`: the node holds the vault and can deliver; `self`: the author's own
machine keeps it). Listing and donation stay the author's own prepared action,
and the delivery desk stays the long-running `fod serve` this node runs beside
the gateway.

## 7. Escrow contract, fees, and mainnet release

### 7.1 Interface

| Method | Behavior |
|---|---|
| `registerBundle` | Verify registration evidence and create the bundle record with its key commitment |
| `sellUnit` | Add one sale unit to a bundle and pay the caller the pool's current quote; refused under the no-subsidy rule (10.3), so the interface does not offer it |
| `donateUnit` | Add one sale unit and pay nothing; only the bundle's registered author may, because a free unit dilutes the quote every other buyer pays |
| `beacon` | Refresh the bundle's delivery beacon; only the bundle's registered author may, and it is a hint rather than a promise to serve |
| `buy` | Accept exactly the quoted amount, bind one buyer and delivery key, and take one unit out of the pool |
| `deliver` | Publish the buyer-bound ciphertext for a funded order |
| `confirm` | The buyer brings the ciphertext they checked; the escrow records those bytes as the delivery and settles, so a replaced pending payload cannot strand the payment |
| `fulfill` | Verify a delivery proof and settle the order without buyer cooperation |
| `refund` | Credit the buyer after an unfulfilled order expires, and count the strike |
| `withdraw` | Transfer caller's available credit |
| Reads/events | Expose listings, quotes, reserve and unit counts, beacons, orders, ciphertext, credits, strikes, lifecycle changes |

`fulfill` argument: order id (8, big-endian) || delivery journal (281) ||
Groth16 seal (256) || delivery ciphertext. The contract verifies the seal with
the claim binding described in 5.6, checks every journal field against the
funded order, checks the ciphertext against the journal's hash, and only then
settles. `orderPoolView(order_id)` returns the order's terms; the proved
ciphertext is stored at the `pdeliv/<order id>` key and read through the node's
contract storage.

One immutable WASM artifact containing verifier and registry. No operator may
approve invalid proofs, redirect the reserve, replace verification parameters,
or introduce a commercial commission.

The pool keeps its invariant in one global state: a single reserve `r` and
sale-unit count `q` stand for the whole market, per the owner's "one global
pool" answer of 2026-09-29, while each bundle keeps its own inventory, covered
count, strikes, beacon and nonce. `buy` and `sellUnit` compute their amounts
from that global state (`ceil(r / (q - 1))` and `floor(r / (q + 1))`), and a
call whose amount does not match the computed value is refused rather than
adjusted. The pool never funds an acquisition - the owner's answer to the
finite-subsidy decision was "no subsidy at all", so `sellUnit` stays refused
and only an author-only donation brings a unit in.

`buy` accepts one optional, buyer-funded retrieval fee list: the hosts the
client fetched the blobs from. The total is capped at the documented per-item
fee times the bundle's covered flip count, which the registration journal
already carries. Blob size plays no part: size is not consensus-bound and would
be a lever to pull, while the covered count is part of what registration
proves. The fee is escrowed with the purchase payment; on settlement each named
host receives a withdrawable credit, and on timeout or a pre-settlement refund
the whole fee returns to the buyer with the payment. The reserve neither pays
nor receives it.

### 7.2 State machine

The approved guarantee is a hard three-block delivery window with no grace
period: unilateral delivery/settlement is valid strictly before `funded+3`;
that block is already refund-only for unilateral calls. One permissionless transaction pays the original
buyer the purchase amount and the retrieval fee directly; the reveal settlement
lets the author publish the committed secrets and be paid without the buyer's
confirmation, inside the same window: from `funded+3` every unilateral seller
path abort, so the repayment cannot be raced and only the buyer's own
`confirm` (or the refund) moves the order. Pending buyer money and the fee stay
in escrow until settlement; junk from an unauthenticated submitter must not
occupy the only delivery slot (REVIEW.md A03/A06).

```text
LISTED --buy--> FUNDED --deliver + confirm--> SETTLED
                    |--fulfill (proof)-----> SETTLED
                    |--reveal (secrets)----> SETTLED
                    \--funded+3------------> REFUNDED (strike)
```

Rules:

- A bundle can be funded while it has at least two sale units (`q >= 2`, so the
  quote exists) and at least one fresh beacon. A stale beacon blocks new
  funding; it does not move units, credits, or orders.
- Only one buyer can fund an order.
- Funded terms are immutable.
- Funding pays exactly the quote computed from the current `r` and `q` and
  takes the unit in the same call, so two funding calls cannot take one unit.
- Delivery and confirmation are separate steps. The ciphertext is bound to the
  buyer, the buyer's own tool checks it against the bundle's key commitment, and
  the confirmation carries exactly those bytes with the call. The escrow records
  them as the delivery and settles, so a payload that replaced the pending slot
  after the buyer's check cannot strand the payment (`AH-10`): the buyer's own
  copy settles whatever anyone else published. The delivery proof binds the
  same ciphertext hash, so both settlement paths agree on what "the ciphertext"
  is, and the recorded delivery stays readable after settlement. A seller who
  needs settlement without the buyer's cooperation uses the delivery proof.
- A permissionless refund pays the original buyer the purchase amount and the
  retrieval fee in one transaction and counts a strike against the bundle: one
  epoch delisted, and the listing cleared if it happens again.
- A bundle whose secrets were revealed is spent: its remaining units leave the
  one global count, and new buys and donations are refused. Because the same
  preimage unlocks every order on that bundle, `reveal` settles all its still-open
  orders in one call and retains the preimage under each order id. At most eight
  may be open at once, bounding the atomic call. If any is already at its refund
  boundary, reveal refuses before publishing keys; permissionless refunds remain
  available.
- A settled order cannot settle twice; a refunded order cannot settle after the
  credit exists. Previous delivery proofs cannot settle a new order.
- Failed and duplicate calls cannot create additional liabilities.
- Rejected mutating calls must produce a runtime failure with transaction value
  and storage rolled back. Returning `ok:false` JSON is insufficient. CLI outcome
  reporting must agree with the node's receipt and the actual post-call state.
- Units taken by a sale are consumed: the pool can quote that bundle again only
  from units it later buys or is given.

Delivery deadline: `funded+3`, exclusively. Unilateral completion must precede
that block; refund opens at that block, with no grace period. Beacon freshness
uses a recorded window in blocks. Failed calls do not extend deadlines. The
seller CLI checks current state immediately before submission and avoids
delivery near the cutoff.

The desk reads that budget from the chain rather than assuming a block time:
`fod serve` reads the head, and when the node reports the interval between the
last two blocks it prints how much wall-clock room is left before the proof
fallback has to start ("720 blocks left, about 3h00 at this node's 15s per
block"). A node on a quiet or automined chain reports no interval, and the desk
then says so instead of inventing one.

The reference desk acts on that budget itself: when a delivered order it
published has `revealMargin` blocks or fewer left (default 2, `-reveal-margin`)
and no proof landed, it publishes the committed secrets (`reveal`). That
settles the order without the buyer and retires the bundle's remaining units,
which is the desk's trade at the boundary; a configured prover's `fulfill` is
preferred while it still fits (`AH-19`). The same bound governs delivery: the
desk publishes a buyer-bound ciphertext only while a later pass can still
broadcast and include that reveal before the refund boundary (the last usable
broadcast head is `deadline-2`), settles a funded order with the reveal at that
head, and leaves later orders to the buyer's repayment, so it never hands out
keys it can no longer settle.

Who pays what on each path, at the measured gas (`tools/fod/fees.go`, `fod
fees`) and the mainnet rate recorded on 2026-09-25 (`SOURCES.md` F-77). The
figure is the maximum fee the CLI prices: the gas used plus the transaction's
size fee.

| Call | Who sends it | Wasm gas | Signed bytes | Max fee at that rate |
|---|---|---|---|---|
| `buy` | the buyer | 1,313,000 | 292 | ≈ 1.54 iDNA |
| `deliver` | the desk | 1,083,800 | 350 | ≈ 1.38 iDNA |
| `confirm` | the buyer | 2,035,700 | 350 | ≈ 2.29 iDNA |
| `reveal` | the author; anyone may submit it | 1,268,100 | 235 | ≈ 1.45 iDNA |
| `refund` | the buyer's client; anyone may submit it after the window | 1,357,900 | 120 | ≈ 1.42 iDNA |
| `withdraw` | the host that holds a credit | 371,966 | 117 | ≈ 0.47 iDNA |
| `fulfill` | whoever holds the delivery proof, usually the desk | 197,276,900 | 815 | ≈ 190 iDNA |

`withdrawReserve` is measured live: the v4 lab's donor withdrew the whole
200 iDNA reserve in tx `0x7b91a991…` (epoch 28, block `0x0462cc56…`, 4,226
user gas / 422,600 wasm), which returned the donation and left the contract
balance at 0 with every buyer-escrow entry and host credit already settled -
the bounded donor exit `AH-15` requires.

`confirm` now carries the buyer's checked ciphertext (`AH-10`), so its
reference envelope is the order id plus the 219-byte payload the lab's
deliveries carried (350 signed bytes). The gas figure is the higher of the two
live receipts for the earlier digest form; `fod fees` says so in its note until
a live confirm under the current ABI is measured, and the size fee is priced
from the bytes the CLI actually signs.

So the buyer's fast path (buy and confirm) costs about 3.3 iDNA and the desk's
delivery about 1.3 iDNA, while settling without the buyer's cooperation costs
about 190 iDNA - tens of times the fast path, and minutes of proving. That
asymmetry is why the desk has to be online near the deadline and why the proof
is a last resort. The reveal settlement is the cheap non-cooperative path
(previously measured about 1.45 iDNA for one order) at the cost of publishing
the committed keys. It now settles every open order on that bundle atomically;
its current transaction cost needs a fresh chain receipt. Instant settlement
for the customer therefore means the desk's fast delivery, not an atomic
single call.

### 7.3 Availability and accounting

- Store the published delivery ciphertext in contract state so recovery does
  not depend on a messaging service.
- Buyers retain verified encrypted flip blobs locally before payment.
- The pool keeps one market-wide reserve `r`, sale-unit count `q` and pending
  buyer escrow (the payments held for orders that have neither settled nor
  refunded). A bundle keeps only its own inventory, the covered flip count the
  registration statement bound (5.5 item 7), its beacon, strikes and nonce, so
  a listing can say how many flips a bundle carries without trusting a caller's
  artefact and a fee rule bounded by eligible flips has the authenticated
  number to bound it with. Every trade is an integer computation from the
  reserve and the units, and a call whose amount does not match the computed
  quote is refused rather than truncated.
- Liability conservation: the balance backs the donated reserve, pending buyer
  escrow, withdrawal credits and host-fee liabilities separately, with no
  double counting. Buying does not turn refundable money into spendable
  reserve, and the reserve is never spent on a purchase or a payout. A reserve
  contribution is recorded against the address that paid it and is withdrawn by
  that address alone (`withdrawReserve`), only while the reserve still covers it
  and the balance still covers every buyer escrow and host credit; that is the
  reserve's only outflow. Settlement
  pays the order's purchase amount to the bundle's registered author and turns
  the retrieval fee into the named hosts' credits; refund pays the original
  buyer the same total directly, in one transaction with no credit step. A
  trade, refund or payout that breaks this relation aborts atomically.
- Amounts are unsigned 128-bit integers in base units, which holds any amount
  below 3.4e38 base units exactly. The interface, the CLI, and the contract
  agree on that range; a larger amount is refused rather than truncated.
- Use withdrawal credits. The payout is atomic: the transfer promise debits the
  contract inside the same call, so an unbacked credit aborts the call and the
  credit survives, and a completed payout consumes the credit exactly once
  (`SOURCES.md` F-41).
- Maintain `liquid balance >= available reserves + pending buyer escrow +
  withdrawal credits + unpaid host-fee liabilities` across all bundles. A host
  credit already included in withdrawal credits is not counted twice.
  `accounting` publishes `reserveTotal`, `pendingTotal`, `creditTotal` and
  `liquid` so every reader checks the same relation without iterating storage,
  and the sequence test in `spikes/harness` asserts it after each move.
- Availability advisories stay off chain (5.8) and are never contract state.
- Retrieval fees are pass-through: credited to the named hosts out of the
  buyer's own payment, never debited from the reserve. The cap is the recorded
  per-flip fee times the bundle's covered flip count, and a list that exceeds it
  is refused. Blob size never enters the computation.
- Any verified maintenance allocation is separate from these liabilities.

`buy` holds the caller's payment (`quote + fee`) in the pool's `pending` and
takes the unit; it does not touch `reserve`. `confirm`, `fulfill` and `reveal`
settle it: the purchase amount leaves escrow for the bundle's registered author
and the retrieval fee becomes the named hosts' credits. `refund` returns the
same total to the original buyer directly. The quote keeps being computed from
`reserve` and `units` alone, so a purchase that has not settled cannot change
what the next buyer pays.

### 7.4 Fees

Marketplace commission is exactly zero.

The only value the contract may retain is a spread recorded in the funding
profile that stays in the reserve, exists to cover refunds and the contract's
own network fees, and must keep the round trip of 5.1 non-profitable. No person
receives a share of any trade.

A retrieval fee is not a commission and not a reserve payment: the buyer pays it
to the host that served the blobs, it is capped per covered flip by the recorded
maximum, and a purchase that names no host pays none (5.9, 7.1).

Verified source fact: the minimum deployment-stake check applies only when the
code hash is in the embedded contract set; custom WASM contracts use a separate
path. Do not invent a recurring stake fee for the selected architecture.

Policy:

- Normal network fees are paid by transaction senders.
- Additional contract contribution defaults to zero.
- A contribution is permitted only for an established contract-stake
  requirement, must be disclosed, capped at the actual shortfall, dedicated to
  that requirement, and stop once funded.
- Escrow deposits and withdrawal credits must never finance maintenance.
- No speculative treasury, percentage fee, or operator payout.

Provider-specific utility economics are separate from the marketplace fee
policy.

### 7.5 Mainnet release

This is a later phase. It is not required to finish the immediate play-money
repair goal and must not be used as a reason to wait on funding while local
defects remain. No spend, deployment, release or publication is authorized by
this specification. The reviewed current artifacts must not be deployed/funded.

Mainnet capability belongs to the first usable release. No blanket
mainnet-payment disablement. Local tests precede real transactions.

Mainnet acceptance includes: a real eligible historical bundle; registration
and a bootstrap unit; an actual iDNA purchase at the quoted price; correct
private delivery; buyer confirmation; buyer recovery after restart; an
unfulfilled order, refund, and withdrawal.

The pool's reserve is a funding-profile parameter. The bundle set, the unit
counts, and the iDNA amount are recorded before funding and exercised with play
money first. The mainnet profile is funded only by the owner's later, explicit
action, and the mainnet exercise starts after that (`docs/PROGRESS.md`).

Genuine proofs only. Fixture anchors and synthetic receipts must never satisfy
mainnet verification.

Record source revision, bytecode hash, deployed address, proof-program IDs,
verification parameters, and transaction receipts.

### 7.6 Guarantee boundaries

Successful fulfillment proves that the funded buyer can recover the committed
correct keys. It does not prove the buyer opened them.

Settlement depends on chain inclusion and finality. Pending-transaction
ciphertext may be visible before inclusion. Deadline handling reduces timeout
races but does not guarantee fair exchange under arbitrary censorship.

Do not promise exclusive knowledge or prevention of copying.

The pool binds the quotes and the settlements that pass through it and nothing
else. It is not a price control on trades made anywhere else, and it does not
make the buyer's keys exclusive. Its quote can fall to whatever sellers still
accept: the reserve is finite, and the floor is emergent rather than guaranteed.

A delivery depends on at least one live key holder. The beacon and the strike
make an absent desk visible and costly, and the buyer's refund bounds the loss,
but the pool cannot compel a delivery and cannot recover a payment from a
disappeared holder.

### 7.7 Experimental fast trade candidate

This candidate is not the deployed or release protocol. It separates work that
can finish before validation from work that must wait for the chain's verdict:

1. Before the session, the author proves in private that every original
   encrypted blob rebuilds its CID, opens with the epoch's original secrets,
   contains a supported flip, and embeds the claimed author's identity public
   key. The same proof checks a fresh encrypted copy
   of each decoded flip under `SHA-256("fod:fast-delivery:content-key:v1" ||
   compressed(M))`, where `M` is a random secp256k1 point. Its journal binds
   the network, contract, author, epoch, complete proposed original CID list,
   new encrypted CID list, covered count, original-key commitment, and a hiding
   commitment `C = M + bH` to the content key. `H` must be a separately derived
   secp256k1 generator with unknown discrete-log relation to `G`. The author
   produces this expensive credential once; neither buyers nor the shared
   gateway prove a seller's content on demand. The proof must reject identity
   points and require a distinct nonce for every authenticated encrypted copy.
   Its associated data binds the network, contract, author, epoch, original
   CID and flip index, so neither a copied ciphertext nor a swapped item can
   satisfy the registered journal.
2. After `ValidationFinished`, a registration call carries that sealed
   credential and the four public state leaves: identity and global at the
   closed inventory anchor, then identity and global at the verdict anchor.
   The contract checks each fold against the canonical header's state root,
   compares the inventory's exact CID sequence and count to the credential,
   checks the epoch transition, cleared verdict flips and flags, and activates
   one bundle per network, author and epoch. The author must call; an invalid
   proof changes no state and its submitter pays its bounded verification cost.
   This call verifies the stored private-content work once, while the
   post-session part has no new large proof. A public-data-only verifier must
   have strict limits on calldata, leaf values, path lengths and gas.
3. Each buyer registers a delivery public key in `buy`. The seller computes
   `R = rG`, `T = M + rB` for that buyer's key `B`, with fresh random `r`, and
   supplies a Fiat-Shamir representation proof of knowledge of `r,b` such that
   `R = rG` and `T - C = rB - bH`. Its transcript must bind the network,
   contract, bundle, funded order, nonce, buyer and registered delivery key.
   The contract checks canonical curve encodings and this proof, then retains
   the full proof and settles the order in the same included call. The buyer
   obtains `M = T - xR` with their private delivery scalar `x`, derives the content
   key, and authenticates the prefetched encrypted flip copies. The buyer must
   check the included order, its registered commitment, the delivery proof and
   their own delivery key before trusting that recovered key. The shared
   relation is in `proofs/fast-delivery-core`; the feature-gated escrow checks
   it against the stored funded order. The guest image and a genuine synthetic
   receipt are pinned; live registration still requires a contract-bound proof
   and the public session witness.

The new encrypted copies are a delivery format, not a substitute for original
Idena provenance. A buyer checks their CIDs and fetches them before payment;
`proofs/fast-content-core` exposes a prefetch check that binds every downloaded
copy to the registered journal, contract and bundle id. The same core can
verify a settled delivery proof and recover the flip parts with the buyer's
private key. The experimental CLI reads the retained certificate and funded
order from the buyer's own synced node, retains the originals and copies before
payment, and exports a settled order for local recovery. An offline verifier
can replay the content seal and public session leaves against canonical header
roots supplied by the verifier's own node.
Any buyer who receives `M` can redistribute it, as with the current epoch
secrets. The proposed point encryption and representation proof require an
independent cryptographic review, reproducible credential and client vectors,
and measurement in the actual Idena WASM runtime. Native timing is not a gas
result. No fast path is enabled until malformed points, changed order context,
wrong inventory, duplicate epoch, refund races and concurrent buyers all fail
in adversarial tests, and an included settlement fits the three-block window.

The owner accepts one explicit *experimental* risk: a buyer can decrypt `R,T`
   from a public pending transaction before it is included. A censored or dropped
settlement can therefore leave the buyer holding the content and later taking
the refund. This candidate reduces proving latency and avoids public original
secrets, but does not claim censorship-resistant fair exchange or a secure paid
release under arbitrary block producers. The older contract and its receipt
pins cannot adopt this format without a separately built and verified module.

## 8. Provider-operated gated utility

### 8.1 Intended deployment

1. Provider acquires eligible flip bundles.
2. Keeps secrets in its local environment.
3. Operates a shared Idena node.
4. Runs a restricted gateway and required session services.
5. Distributes HTML client, connection file, and rules.
6. Connects session results to its utility.

Customers open the distributed files and connect. No full node, no marketplace
CLI. A hosted website is optional.

### 8.2 Reference integration

A small working example with configuration files, not an administrative
dashboard. It demonstrates:

- Loading an exported flip pool.
- Publishing session timing and scoring rules.
- Enrolling a participant.
- Assigning challenges.
- Receiving answers.
- Scoring against authenticated historical consensus.
- Issuing a signed result bound to participant and utility.
- Rejecting replayed or expired submissions.

The provider signs results with a dedicated application key, not the shared
node wallet key. Results must be clearly labelled as provider-issued; a valid
signature proves the provider issued a statement, not that scoring was honest.
The signed result therefore carries a `transcript`: one entry per assigned
challenge with the digest of the answer the provider recorded, or `null` when
it recorded none. With the `contentDigest` the participant read before
answering and the `contentReveal` the closed policy publishes, that is enough
for the participant to recompute its own score - `sha256` of the canonical
answer text against each revealed challenge digest, threshold from the pinned
rules - and to refuse a result the recomputation does not reproduce
(`app/src/score.ts`). What this cannot show is that the provider attested the
answers the participant really sent; that residue is stated in 10.4 and carried
as the H-3 handover.

Do not implement an additional live consensus mechanism in this reference
adapter. Document where one could be integrated.

Implemented surface (`gateway/`, one provider, one pool, in-memory sessions):

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/session/policy` | no | session timing, scoring rules, pool provenance |
| GET | `/provider/key` | no | the provider's application public key |
| POST | `/session/enroll-challenge` | no | the one-time EIP-191 text to sign, bound to provider, session and nonce |
| POST | `/session/enroll` | no | consumes that challenge and issues a scoped, short-lived participant token; a bound wallet is required |
| GET | `/session/challenges` | yes | assigned challenges, never answer material |
| POST | `/session/answers` | yes | one answer per challenge; replay and closed sessions refused |
| GET | `/session/result` | yes | provider-signed statement, bound to participant and utility, and issued only after `closesAt` |
| GET | `/marketplace/state` | yes | the provider's cached snapshot |
| POST | `/transactions` | yes | refused until signed-transaction submission is designed |

Unlisted paths are 404: the gateway has no RPC passthrough. Results are signed
with a dedicated Ed25519 application key, never the node wallet key.

Session timing is enforced rather than displayed. `pool.rules.opensAt` and
`closesAt` bound enrolment and answer intake - an instant before `opensAt` is
`session_not_open` (403) and an instant at or after `closesAt` is
`session_closed` (410). A submission is acknowledged without any correctness
signal, and the signed statement is issued only after `closesAt`: before then
the score would be an answer oracle, whether read per answer or from the result.
A participant whose own token has not expired can still collect the statement
for work done inside the window after it closes. The pool loader refuses a
window whose `closesAt` is not after its `opensAt` rather than half-enforcing it
at request time.

Enrolment is wallet-bound and single-use. The client asks
`/session/enroll-challenge` for the text
`fod-enroll:v2:<provider>:<session>:<address>:<nonce>:<issuedAt>`, signs it with
`personal_sign`, and posts it with the address; the gateway verifies the
signature, consumes the server-held nonce once, and refuses a challenge older
than five minutes, a challenge issued for another provider or session, and a
second live enrolment for the same address. The address is a key, not a person.

An assigned challenge carries the flip's two sequences when the pool names the
content (`content.publicPart` / `content.privatePart`, the decrypted parts of
6.7), as `variants`: two lists of four pictures, each list in the order that
sequence shows them, as data URLs. The gateway reads the parts with the client's
own format rules and refuses a pool whose content is not a flip (`SOURCES.md`
F-98), so a provider cannot serve a challenge a participant cannot look at. The
interface answers such a challenge by choosing one sequence and sends `left` or
`right`; a challenge without `variants` is answered from what the participant
already has. Answer material never travels: the digests stay in the pool.

`GET /session/policy` also carries the pool's `provenance.answers`, the
provider's own statement of what its answers rest on, and the interface shows it
next to the session's rules as "Answers from". A participant can therefore see,
before answering anything, that the scoring rests on data the provider names and
not on a chain rule this build can check.

It also carries `contentDigest`, the SHA-256 over the session and utility ids,
the rules, that same provenance, a per-session `answerSalt` from the pool, and
every challenge with its CID, answer digest and a digest of the pictures it
shows. The salt keeps the published digest from being enumerated against a small
answer pool while the session is open: the open policy serves neither the salt
nor any answer digest, and once `closesAt` passes it publishes the preimage as
`contentReveal`, which recomputes to the pinned digest. The signed result
carries the same digest, so a utility can record the content it examined and
refuse a later result that reuses the session's name over different content
(8.3); a participant can also compare the two numbers and see whether the
session they answered is the one the policy described.

Provider connection file v1, the file a customer is handed alongside the
interface:

| Field | Meaning |
|---|---|
| `format` / `version` | `fod.provider-config` / `1` |
| `providerId` | the provider this file belongs to |
| `network` | network the provider serves |
| `gatewayUrl` | http or https origin of the gateway |
| `protocol` | session protocol version, `fod.session.v1` today |
| `publicKeyHex` | optional: the application key the provider signed with when this file was written, in the one agreed encoding - the 44-byte SPKI DER key as 88 hex characters, exactly what `GET /provider/key` publishes. A result is verified against it, so a participant who keeps the file does not depend on the host to serve its own key - and a host that serves a different key cannot produce a "verified" reading |

The interface keeps the participant token in memory for the tab's lifetime and
never writes it to storage, because a `file://` page has no origin to scope one.
A result is verified in the browser with the provider's published key before it
is shown; "verified" means the signature verified, and the result payload says
that it proves issuance, not honest scoring.

### 8.3 Utility enforcement

The utility's application or contract checks the relevant result and
participation policy. Possible utilities: service access, limited allocations,
access passes, invitations, computing resources, restricted asset claims.

If a program uses identity-based restrictions, enforce them across the whole
program including all sessions, retries, and receiving addresses. A wallet-only
quota must not be described as one-person-one-allocation. Where identity
eligibility is required, verify it independently of the provider's scoring
statement.

Reference utility (`utility/`):

- A supported grant requires a provider key, accepted session, content pin,
  independently configured opening/closing window, and
  `requireCommittedAnswers: true`. `loadPolicy` rejects policies without
  these. The access decision also refuses insecure programmatic policies:
  neither a signed `passed` result nor an arithmetically valid provider
  transcript can authorize a grant by itself.
- Each wallet signs its answers to `/answer-commitment` during the configured
  window. After close the adapter checks the reveal against the content pin,
  utility id and window, scores those witnessed answers itself, and requires
  a fresh, once-consumed wallet control challenge. The provider's score and
  outcome do not determine the grant. Every supported grant is wallet-bound;
  provider-issued participant ids cannot create an allowance by themselves.
- The service is still centralized at the utility boundary: its clock,
  commitment store, operator-selected answer pin and actual benefit release
  remain trusted. The pin proves the operator committed to those answers,
  not that historical Idena consensus established them. There is no canonical
  protocol enforcement of new-session scoring, global consumption or actual
  faucet/dapp release. This is an open full-product blocker, not satisfaction
  of clause 10.3(6).
- A granted decision carries `identity: "not-verified"` while this adapter
  holds no identity check, and `/policy` publishes that plainly.
- No request field can assert an identity. A policy that sets
  `identityVerification.required` refuses every request with
  `identity_required` and publishes `identityVerification.supported: false`,
  because the evidence such a policy needs - eligibility committed by the
  chain's epoch state, together with proof that the participant controls that
  identity - does not exist in this adapter. A caller's boolean is not
  evidence, and neither is a trusted server's boolean standing in for it.
- The reference quota is local bookkeeping: namespaced by utility, provider and
  either address or participant, and expired as it is read. `/policy` publishes
  `quota: { scope: "local-process", durable, keyedBy }`. Without `--grants` it
  is in-memory and forgotten on restart (`durable: false`); with
  `--grants FILE` the live entries are written 0600 and atomically, so a
  one-time benefit survives a reconstruction (`durable: true`). Either way, a
  limit claimed to hold globally must be enforced by canonical protocol state;
  a local store may assist operation but cannot decide it.
- The endpoint's browser-origin policy is policy, not a fixed wildcard. A policy
  that lists origins gets exactly those, echoed so the browser can enforce them,
  and `/policy` publishes the pin. The offline `file://` testbed lists
  `["null"]` and a public testbed writes `["*"]` explicitly; a policy that lists
  no origin gets no CORS header at all, so a hosted utility has to name its dapp
  origins rather than inherit a wildcard. Non-browser callers are unaffected:
  the endpoint stays signature-gated for everyone.
- A result whose `sessionId` the policy pinned to another `contentDigest` is
  refused as `wrong_session_content`. A session id is a name the provider
  chooses, so pinning the digest is what makes "I accept this session" a claim
  about what the session was rather than about how it is spelled; a session with
  no pinned digest cannot authorize a grant.

### 8.4 Economic and Sybil-resistance framing

Use consistently:

- AI can generate and solve flips.
- Generation, inference, retries, quality control, infrastructure, and
  consensus participation have costs.
- Historical Idena consensus provides provenance and a qualification signal.
- Those costs and signals may give the content economic value.
- Automated participation is not inherently free; marginal costs can decrease
  through optimization, reuse, and caching.
- The relevant measure is the cost of repeated participation relative to the
  utility obtained.
- Stronger duplicate-person restrictions require additional identity controls.

Do not claim that randomization, deadlines, or historical consensus alone
establish unique-person participation.

## 9. Offline FAQ requirements

The FAQ is bundled inside the HTML and works without network access. It must
answer, accurately and without contradicting sections 2.1, 5, 7, and 8:

| Question | Required content anchor |
|---|---|
| What do I buy? | Original decryption keys for an eligible author-epoch bundle, delivered bound to the buyer's own key, plus verifiable provenance and historical qualification |
| Why epoch bundles? | Keys are shared across the author's flips within an epoch; cannot restrict to one flip |
| Hosted website needed? | No; local HTML plus CLI with own node; providers may distribute client and gateway access |
| Customer node needed? | No; shared node plus client, configuration, participation rules; no marketplace CLI for participants |
| What does a shared node provide? | Blockchain connectivity; challenge assignment, answer collection, scoring, utility enforcement remain application responsibilities |
| Can I schedule a session anytime? | Yes; bought keys can be used in a private session the buyer starts from the client; this does not create or renew an identity |
| How is the price set? | By the pool's reserve against the sale units it has bought: the quote falls when sellers or donors bring keys and rises when buyers take them. No person sets it, and no floor is guaranteed |
| Who runs the pool? | Nobody. It has no owner, no admin entry, no upgrade path, and no commission; its storage is public and holds no keys |
| How to organize gated utility? | Define utility, allocation, eligibility, limits; acquire and verify bundles; register participants and bind destination addresses; apply identity rule if needed; assign challenges and collect answers; verify result plus additional eligibility; grant access or permit claim; enforce program-wide limits. Explain separately what marketplace, provider, participant, and utility contract do |
| Can AI generate and solve flips? | Yes; the system does not depend on impossibility. Costs are real, measurable, and scale with quality and scale |
| Why can consensus-qualified flips have value? | Assumes usable content with authenticated consensus and provenance is valuable; consensus does not create a mandatory price |
| Is automated solving scalable for free? | No; resources are required, no fixed cost floor guaranteed; efficiency, batching, caching reduce marginal cost |
| Does a session prove unique personhood? | No; challenge costs may add economic resistance; stronger uniqueness needs independent identity policy and duplicate controls; Idena's own coordinated validation must not be equated with an independently scheduled session |
| Can someone participate again with another wallet? | A wallet-only rule permits it; identity rules must be enforced against the identity across sessions and addresses |
| Can participants modify their HTML? | They control local files, so frontend scoring cannot authorize payouts or access; the enforcement layer verifies evidence; the reference interface recomputes the provider's score from the answers it submitted and the answer key pinned before the session opened, but a participant who replaces the page skips that check - it protects the participant from the provider, not the utility from the participant |
| Give participants original epoch keys? | Usually no; provide assigned challenge images and retain keys; participants can still copy images they receive |
| Can flips be reused? | Yes, but repeated exposure makes answers easier to memorize, share, or automate; must not be represented as fresh secret material |
| What if key delivery fails? | The buyer confirms a delivery that decrypts to the committed keys, or the seller settles with a proof or the reveal settlement; at the third block after funding anyone may submit one refund that pays the original buyer the payment and the retrieval fee directly, and the bundle counts a strike |
| What fees apply? | Ordinary network fees, zero marketplace commission, no paid operator or market maker, and no share to any person; only a recorded reserve spread if a measured need exists |
| Is a complete utility platform included? | Marketplace, exports, local client, provider configuration, small reference adapter included; asset-specific enforcement and advanced session systems are separate |

## 10. Implementation sequence

| Package | Deliverable | Exit condition |
|---|---|---|
| 0. Baseline | Repository, pinned sources, protocol specification, reproducible setup | Existing work preserved; assumptions recorded |
| 1. Proof feasibility | Real-proof verifier in Idena WASM | Valid/invalid cases verified; native limits measured |
| 2. Historical evidence | Complete inventory and qualification reconstruction | Authenticated positive/negative cases; omission attacks rejected |
| 3. Local key tool | Derivation, vault, recovery, original-blob verification | Independent vectors match; secrets stay local |
| 4. Proof programs | Registration and buyer-bound delivery | Correctness, privacy, replay protection, resource measurements pass |
| 5. Escrow and pool | Registry, reserve, quote rule, sale units, beacons, delivery, deadlines, credits | Reserve and unit invariants, quote arithmetic, strike accounting, and lifecycle pass |
| 6. Local HTML | Single-file interface and public file exchange | Works through `file://` without hosted dependencies |
| 7. Provider mode | Restricted gateway, configuration, participant integration | Customers participate without own node or marketplace CLI |
| 8. FAQ and exports | Offline explanations, gated-utility example, content export | Documentation matches actual behavior |
| 9. Release | Play-money integration, recovery, checksummed artifacts; mainnet later | Exact source, artifacts and exercised chain are evidenced |

### 10.1 Working rules

- Use the model and reasoning effort selected by the owner in the app.
- Work directly without extra implementation agents by default.
- Keep one authoritative specification and one compact evidence/progress
  record (this document and `PROGRESS.md`).
- Complete coherent packages before testing.
- Run the smallest relevant test batch for each package.
- Run the integrated marketplace and provider-flow checks after assembly.
- Preserve source pins and existing user work.
- Do not upgrade unrelated dependencies or refactor unrelated code.
- Do not develop the full UI while fundamental proof feasibility is unresolved.
- Do not silently change chains, trust assumptions, eligibility rules, or fees.
- Report blockers with exact evidence.
- Never treat fake receipts, mocked verification, or a frontend demonstration as
  proof of a working escrow.

Targeted review of delivered proof statements, source authentication, secret
handling, gateway permissions, and contract accounting precedes the mainnet
exercise.

### 10.2 Active repair sequence for DeepSeek V4.1 Flash

This sequence supersedes package-completion claims made before the chief
developer review. Reuse existing components; do not restart the whole project.

| Package | Required work | Exit evidence |
|---|---|---|
| D0 | Fail closed on unsafe supply/payable rejection and capture A01/A03/A04/A06/A07 regressions | Reproduced public-boundary failures with disposable inputs; unsafe release paths disabled without admin hooks; focused regression batch |
| D1 | Prove complete real-session provenance on the unchanged legacy chain (A02) | Authenticated anchor design, full witness bindings and omission/fabrication rejection; one real closed-session package; default-node verification; measured costs or a precise feasibility blocker |
| D2 | Finite authorized supply and segregated accounting (A01/A03/A04/A05) | Accepted economic rule; bounded subsidy; adversarial multi-user/multi-bundle sequences; refunds remain payable after arbitrary authorized trades; rejected calls roll back |
| D3 | Reliable key delivery, real fetch checks and buyer-funded host fees (A06/A08) | Accepted/measured latency guarantee; junk-first and uncooperative-buyer recovery; verified CID downloads; eligible-count cap; no reserve-funded fee |
| D4 | Actual seller, buyer and private-session journeys (A07/A09) | Every prepare output signs unchanged; supported real flips visible and solvable; bought-content adapter; simple session setup; no CLI/wallet requirement for guests |
| D5 | Reproducible integrated play-money candidate (A10) | Real receipts match rebuilt guest IDs and release contract; full sale/donation/buy/delivery/refund/restart/session flow; exact evidence, privacy and notices; no unresolved P1 |

- DeepSeek is the implementer; the chief developer review is not permission to
  replace technical judgment with another long documentation-only continuation.
- Start with D0. D1's capture wait does not block independent local fixes. Product
  decisions gate only their dependent economics/features, not all work.
- Preserve ownerless operation, the legacy chain, local secret handling and
  private repository scope. Use relative paths and placeholders in committed
  documents; never include personal paths, endpoints, credentials or live keys.
- Keep G-1's existing verifier result. Reopen G-2 for full authentication, G-4
  for solvency and failure semantics, and the fast-delivery part of G-3.
- Complete each coherent change, then run the smallest meaningful batch once.
  Tests must exercise producers through consumers and hostile transitions, not
  reconstruct inputs that conveniently match each component.
- Require genuine receipts for final proof verification. Synthetic fixtures are
  valid unit-test inputs but never evidence of real-chain provenance or release
  readiness. Missing required artifacts must fail the final verification.
- Refresh IDs, seals, contract constants, CLI pins, fees and manifests after
  proof/ABI changes. Record evidence against the actual delivered revision.
- Bound feasibility work: map committed inputs, implement one proof path,
  measure, decide. Do not poll a future ceremony or unchanged host status in
  repeated turns. A missing chain commitment needs a decision, not more polling.
- Report only changed evidence and the next concrete task. No publication, and
  no mainnet action beyond the bounded deploy smoke-test allowance in
  `PROGRESS.md` section 0, without the user's explicit per-action approval.

### 10.3 Decentralisation contract and the active package plan

This subsection is normative and supersedes earlier recommendations in this
document for trusted server-side identity verification, provider-authoritative
scoring, a local database enforcing a global quota, and a mandatory designated
capture node. It also supersedes the D0-D5 sequence in 10.2 and the deploy
smoke-test allowance in `PROGRESS.md` section 0: the packages below are the
active plan, and under it only local changes, tests, builds and disposable
isolated local environments are authorized. Mainnet action, deployment,
funding, broadcasts, remote hosts, existing remote nodes, credentials, real
data, paid infrastructure and publication remain separate, per-action-approved
work, and no historical note grants them.

The required application must introduce no mandatory trusted operator beyond
the pinned chain and the cryptography it names. This is an acceptance condition,
not a claim that the current implementation meets it. It does not establish
that the underlying chain itself is fully decentralized.

1. **No application authority.** No administrator, privileged market maker,
   trusted scoring provider, exclusive prover, checkpoint publisher, allowlist
   operator, or required hosted API determines protocol validity or overrides
   an outcome. Ordinary ownership of one's own secrets and keys is not
   administration.
2. **Evidence proves the claim.** A signature proves authorship, a content
   hash proves integrity, a chain commitment proves what was committed. None
   of them prove honest scoring, complete inventory, current availability,
   unique personhood or successful delivery. Every authorization states its
   exact claim, its evidence and authoritative source, the chain commitment
   authenticating it, the independent verification procedure, the required
   identity/control/context bindings, its freshness and replay rule, and who
   can produce or withhold the evidence.
3. **Production is independently possible.** An eligible participant can
   obtain the witness data and produce the required proofs from available
   software and their own secrets. Historical capture may require advance
   preparation, but the capture operator is not an authority and authenticity
   and completeness must be independently verifiable. Missing historical data
   is reported as missing, never reconstructed by assertion.
4. **Global rules live in canonical state.** Replay prevention, consumed
   entitlements, inventory accounting, refunds and allocation limits are
   enforced by specified protocol state transitions, with atomic consumption,
   concurrency, failure, confirmation and reorganization behaviour stated.
   Local stores and caches may assist operation and cannot decide a global
   entitlement.
5. **Availability is a dependency, not a guarantee.** Peers are replaceable
   and their assertions untrusted; a valid hash promises nothing about future
   serving and key possession promises nothing about future cooperation. A
   failed delivery has a bounded refund or exit that needs no operator's
   permission.
6. **The utility is part of the claim.** What the user ultimately receives,
   and what enforces its release, is named. A service that can refuse access at
   its own discretion despite a valid proof is a central dependency and is
   reported as one, not hidden behind the escrow's decentralization.
7. **Bootstrap and rule changes are independent.** A fresh participant needs
   no project account, official website, hosted indexer, privileged discovery
   service or update server to participate, verify or recover. Version and
   migration rules are explicit, and no unilateral upgrade, allowlist or
   emergency administrator is added for convenience.

Package plan. Each package's exit evidence is recorded in `PROGRESS.md`
section 3; the outcomes are:

| Package | Outcome |
|---|---|
| A | the optional adapters cannot be crashed, rate-limit-bypassed, or made to grant on caller-asserted data, and local bookkeeping is not presented as a global rule |
| B | the registration and session statements establish what they claim, or a precise impossibility is recorded: inventory-root temporal completeness, canonical network and context binding, what a fresh session proves, and independent proof production |
| C | a purchase presents authenticated inventory, and confirmation binds the bytes the buyer checked |
| D | finite authenticated issuance and separated liabilities: spendable reserve, pending buyer escrow and withdrawal credits under one conservation invariant |
| E | seller, buyer, refund, withdrawal, export and session journeys run from a fresh checkout with local software only |
| F | one reproducible artifact set with real receipts, independent-node agreement, and remaining limitations stated accurately |

The owner approved the economic policy on 2026-09-29 (verbatim answers in
`PROGRESS.md` section 2) and fixed the goal: a fully permissionless purchase
flow in which the buyer receives usable, authenticated content effectively
immediately, with a hard local-lab delivery deadline of three blocks, or the
payment and retrieval fee are repaid without a trusted operator or an
unnecessary manual step.

- **No subsidy.** The pool never funds an acquisition and no author reward is
  paid from the reserve; `sellUnit` stays refused. Supply enters only as an
  authenticated author's voluntary donation.
- **One global pool.** A single reserve and sale-unit count price the whole
  market. Reserve contributions remain owned by their contributors, tracked by
  canonical per-address credits. `withdrawReserve` provides their bounded exit
  and reduces the shared reserve and ownership totals together; it cannot draw
  pending buyer escrow or host withdrawal credits. The reserve anchors the
  quote and cannot fund acquisition payouts while `sellUnit` is disabled. A buyer's payment is escrow,
  not liquidity. On settlement the purchase amount is paid to the bundle's
  registered author and the retrieval fee to the hosts the buyer named; on
  timeout or invalid delivery both are returned to the buyer. Per-order and
  per-account liabilities stay exact.
- **Author-only inventory donations**, with the chain-verified eligibility and
  author-control binding unchanged.
- **Buyer-funded, capped retrieval fee.** The buy transaction carries the fee;
  the cap is the authenticated covered count times the documented per-item
  cap, never bytes. The fee is escrowed with the payment and returned with it
  on timeout.
- **Immediate delivery or repayment.** The local-lab deadline is three blocks
  after the buy is included, with no grace period; delivery, settlement and
  refund boundaries are explicit and tested at the boundary blocks. A
  permissionless refund pays the original buyer directly in one transaction.
- **Settlement without buyer acknowledgement.** Confirmation and the existing
  proven fulfilment path stay. A reveal settlement lets the author publish the
  committed secrets (`sha256(secrets)` is the registered bundle key) and be
  paid without the buyer's cooperation, inside the delivery window. A validly
  included settlement prevents confirmation withholding from earning a refund.
  Unincluded or off-chain disclosure remains the fair-exchange gap in 10.6. The
  reveal also ends the listing: its remaining units leave the shared count and
  new buys or donations are refused, while orders already funded keep their
  settlement and refund paths.

### 10.4 On-demand customer access and shared-node supply

The third revision adds three roles around the existing marketplace:

| Role | Has | Does not need |
|---|---|---|
| Author | Idena identity, flips, epoch keys | to run the marketplace, the gateway, or a proof pipeline by hand |
| Shared node | the Idena node, CLI, proofs, listing and delivery desk; self-hosted by the author or hosted by the provider | custody of the author's proceeds or the customer's funds beyond what the role states |
| Provider | the purchased flip pool and the on-demand session service | an Idena identity for its customers |
| Customer | an EVM-compatible wallet address | an Idena identity, an Idena node, or terminal work |
| Utility | the faucet/dapp access rule | the marketplace or the provider's signing key |

The evidence indexer belongs to the shared-node operator, who may be the
author or any replaceable host. It is not a prerequisite for an individual
seller's technical setup. A buyer who chooses only the public API accepts an
unverified chain view, but still needs locally authenticated flip bytes and the
contract's normal protections. The interface must separate that explicit risk
choice from a verified purchase; neither route currently qualifies funded
mainnet use while the short fair-settlement guarantee is open.

**Address equivalence.** Idena derives `keccak256(pubkey[1:])[12:]` exactly as
Ethereum does (`idena-go/crypto/crypto.go`), so an EVM wallet address is already
an Idena address. The customer's lack of an Idena identity is a personhood
limit, not a key limit: the same address can hold the purchased keys, sign the
session enrollment, and receive the utility grant.

**Provenance versus scoring.** A real Idena epoch proves the flip set, the
author's submission, the manifest and the verdict. It does not prove the
on-demand provider's scoring. The provider is a coordinator unless the correct
answer is derivable from the flip content the buyer holds and the buyer can
verify its own score; otherwise the provider-trusted scoring of section 8.3
remains and is not hidden behind the chain provenance.

The reference implementation implements the part of that which it can support
today: the provider's signed result carries the answers it scored, the
participant's client keeps the `contentDigest` it read before answering, and
the closed policy reveals the preimage and the per-challenge answer digests.
The client recomputes the score and the outcome from its own answers and that
revealed key, and only a recomputation that reproduces the signed statement is
claimable (`app/src/score.ts`, `app/test/score.test.ts`). A provider that
mislabels, omits or invents an answer the participant did not send is refused;
a provider that cooperates with a participant to attest answers neither of them
had to solve is not distinguishable this way, which is why the check is a
participant-side refutation and the remaining design question stays H-3.

**Package G.**

- **G1 EVM customer identity.** The Use flips view connects an EIP-1193 wallet,
  signs an EIP-712/EIP-191 challenge, binds the address to the session
  enrollment, and shows the address in the signed result and the utility
  grant. The reference gateway implements the EIP-191 path with `viem`,
  refuses a stale challenge, a mismatched address or a tampered signature, and
  carries the checked address in the signed result and the utility grant; the
  address is a key, not a person.
- **G2 On-demand session.** A provider pool is built from real Idena epoch
  flips; the captured epoch-228 bundle is the first pool. A shared window opens
  on demand, participants solve the same challenges simultaneously, and each
  result is bound to its participant's address. Any provider can serve the
  same pool; the connection file is the discovery mechanism. The reference
  gateway publishes `GET /session/lobby` with the window state, participant
  count and challenge count, and the tests prove two participants in one
  window receive the same challenge set. Building the real epoch-228 pool still
  needs the seller's derived epoch vault (`fod export-keys`), because the
  read-only capture deliberately holds no epoch secrets.
- **G3 Shared-node supply.** An author connects a self-hosted or
  provider-hosted shared node, which performs the Idena node work, the
  export/session-pool step, the listing and the delivery desk. The author's
  listing/donation is a minimal UI action. The custody boundary is explicit: a
  hosted node that holds the author's epoch keys can deliver the author's
  flips; a self-hosted node keeps them. The node cannot redirect the author's
  proceeds, which the escrow pays to the registered author. The reference
  gateway publishes `GET /shared-node/status` with the node, session, utility,
  provenance, operations and custody modes, and the Use flips view imports a
  `fod.shared-node` connection and states the custody boundary. The hosted
  node performs the export, the pool and the author's vault integration with
  `fod shared-node`, which also writes the connection file; the delivery desk
  is the long-running `fod serve`, and the listing/donation stays the author's
  own prepared action. Stitching those two into the same command is not
  implemented, so the row stays partly met rather than "one command does
  everything".
- **G4 Buyer testbed.** A labeled example gate with example flips and a test
  utility lets a buyer test the full flow without real funds: connect or use
  the example, solve the flips, verify the provider result, and claim the test
  utility grant (faucet/dapp). The testbed is an example provider, not a claim
  that the production utility is decentralized.
- **G5 Payment and custody.** The customer's EVM-only payment path is a
  separate decision: (a) play-money lab funding; (b) a sponsored/relayed Idena
  buy where the customer's address is the buyer, any relayer may submit the
  signed intent, and refunds go to the buyer; (c) an EVM-side escrow with an
  Idena provenance check, which needs a bridge or light client. Until one is
  accepted, the purchase stays on the play-money lab and the EVM address is the
  session credential. Recommended for the experimental MVP: keep (a) now,
  design (b) as the first customer path because it preserves the Idena escrow
  and provenance while removing the customer's gas and key handling, and defer
  (c) until an EVM-native payment is a product requirement. The owner decision
  is which of these becomes the target.

Package G acceptance: a real on-demand session over the epoch-228 pool with at
least two address-bound participants and the testbed walk; the author
onboarding walk with the custody boundary stated; and the provenance/scoring
boundary above stated in the UI and the spec. Packages A-F and their
verification batches remain the first slice.

### 10.5 Audit hardening package (2026-09-30)

The 2026-09-30 read-only audit of the third revision (`PROGRESS.md` section 6)
found the open items below. They extend the active goal. The same contract
(10.3) and the same rule apply: no fix may add a mandatory trusted operator,
and every refusal names its exact reason. An item that is not repaired must be
stated as an explicit limitation instead of being silently closed.

Session gate of the reference gateway (the provider side the utility consumes):

- **AH-1 (high).** `POST /session/answers` returns `correct` per answer while
  enrollment is free and unlimited, so every answer can be learned and a fresh
  enrollment can replay them into a passing result. Required: no per-answer
  correctness before the session is irrevocably closed; a gate session requires
  the wallet binding and admits at most one live enrollment per address per
  session (a key rule, not a personhood claim). Closing evidence: a regression
  submits a wrong answer, sees no correctness signal, and shows that
  re-enrollment cannot convert learned answers into a pass.
- **AH-2 (high).** The published `contentDigest` commits to per-challenge
  answer digests whose inputs are public, so a small binary pool can be
  enumerated offline. Required: commit to a per-session secret salt that is
  revealed only after claims close, or an equivalent construction that keeps
  the utility's pin binding. Closing evidence: an offline enumerator cannot
  pick the answer vector before close, and the utility accepts the revealed
  salt against the pinned digest afterwards.
- **AH-3 (medium).** The enrollment challenge has no server nonce and no
  provider or origin binding, so a captured signature replays for five minutes
  and across gateways that share a session id. Required: a one-time nonce the
  gateway issues and consumes, with provider and session context in the signed
  text. Closing evidence: replay and cross-provider refusal.
- **AH-4 (medium).** Anonymous rate limits key on `req.socket.remoteAddress`,
  so callers behind the documented TLS proxy share one bucket. Required: a
  trusted-proxy configuration that validates the forwarded client identity, or
  proxy-side limits with the requirement documented. Closing evidence: two
  simulated clients behind one proxy do not exhaust each other's bucket.
- **AH-5 (medium).** The session table evicts the oldest live participant at
  its cap, and a public lobby read triggers that eviction. Required: expire
  expired records only and refuse new enrollment at capacity, so no live
  participant's token or answers are destroyed.

Claim path of the reference utility:

- **AH-6 (high for the faucet/dapp recipe).** `POST /access` takes only
  `{result}`; the provider signature proves the provider's statement, not that
  the claimant controls the bound address. Required: a fresh utility-issued
  nonce signed by `participantAddress` before any wallet-scoped grant,
  consumed once. Closing evidence: a copied result without the control proof
  is refused; the same result with it is granted once.
- **AH-7 (medium).** The local quota key omits the bound address it carries,
  and the in-memory store resets on restart, so a wallet can mint grants by
  re-enrolling. Required: make the scope explicit (`provider-participant` or
  `wallet`) and key accordingly; state the restart behavior, or persist spent
  grants when the benefit is one-time.

Provider connection and service defaults:

- **AH-8 (medium).** The app accepts only a 64-hex `publicKeyHex`, the
  gateway publishes 88-hex SPKI DER, and no generated connection file carries
  a pin, so a genuine pin is refused and "verified in browser" degrades to the
  key the same gateway served. Required: one agreed pin encoding, a generated
  connection that carries it, and a round-trip test.
- **AH-9 (medium).** The gateway and utility mains call `listen(port)` without
  a host, binding every interface while the README describes the example as
  loopback. Required: bind loopback by default and make a network-facing host
  an explicit configuration.

Settlement and inventory state:

- **AH-10 (medium).** The delivery slot stays replaceable by any caller after
  the buyer checked it, so a third party can make the checked digest
  unsettleable until the deadline. Required: keep permissionless delivery and
  junk-first recovery while the bytes the buyer checked stay settleable (for
  example a bounded candidate set); pin replacement, junk-first and
  honest-desk sequences with tests.
- **AH-11 (low).** `donateUnit` accepts the registered author's donation even
  after two strikes cleared the listing, so unbuyable units keep moving the
  global quote. Required: refuse donation to a cleared listing, or remove its
  units from the pool count.
- **AH-12 (medium).** `reveal` settles one order but leaves the bundle's
  remaining units sellable while its secret is public. Required: record the
  revealed state, remove its remaining units from the global count, refuse new
  buys and donations, and let funded orders close.
- **AH-13 (medium).** After `funded + DEADLINE`, `refund` is available while
  `fulfill` and `reveal` stay deadline-free; whichever transaction lands first
  decides. Required: state that repayment is available whenever the seller has
  not settled, and pin both orderings with boundary tests; remove the claim
  that the two rules never overlap.
- **AH-14 (low).** The delivery journal carries no network or fork domain
  while the registration statement does, so a receipt can settle a matching
  order on a clone with the same contract address. Required: add the canonical
  network digest to the delivery statement and check it in the escrow, or
  record the fork limitation as accepted. Accepted on 2026-09-30 for the
  experimental play-money phase, and stated as a limitation rather than
  hidden: the fix is a `NETWORK_DIGEST` field in the delivery journal checked
  by `fulfill`, which changes the guest program, its pinned image id, the CLI's
  identifier pin and the release manifest's delivery-proof record, so it is
  scheduled with the mainnet-preparation pass (`PROGRESS.md` section 7). The
  registration path already refuses a foreign network, and the delivery path's
  exposure needs a matching contract address and order state on a clone or
  fork.
  Superseded locally on 2026-10-01 by S6: delivery-v2 checks the network digest
  and canonical funding height/root. Shared-anchor descendant forks still
  require the blockchain canonical-view assumption; no deployed tuple changed.
- **AH-15 (medium).** `addReserve` accepts unattributed, irreversible
  deposits and the reserve has no supported outflow while supply payouts are
  disabled. Required: refuse contributions in that state, or define ownership
  and a bounded exit that cannot touch pending escrow or credits.
- **AH-17 (high).** The ordinary `prepare-buy` artefact can be signed while
  the content-availability check is left unattested: without `-availability`
  the tool stores an `unchecked` marker, and the delivery proof authenticates
  keys and ciphertext, not that the buyer can obtain the bytes, so a seller
  can settle a purchase whose content the buyer cannot fetch. Required: make
  the authenticated manifest check and byte retrieval a precondition of the
  supported buy path (an unchecked artefact must not be the ordinary verified
  path), and bind the checked inventory and its freshness to the order.
  Closing evidence: prepare refuses absent, incomplete, substituted,
  duplicate, stale and unavailable inventory; with content servers removed, an
  order settles only from locally verified bytes, otherwise the buyer is
  refunded. Source: the 2026-09-30 Codex Security scan (`PROGRESS.md` audit 4).
  Closed 2026-09-30; the final acceptance exercise then found the check
  committed the decoded CID bytes instead of the canonical multibase text the
  registration guest hashes, and the corrected check is pinned by a live-chain
  vector (`PROGRESS.md` section 3).
- **AH-18 (high, new 2026-09-30, closed the same day).** The ordinary buyer
  settlement `confirm` could not be included by the lab chain. The cause was
  the fee budget, not the contract: the node derives a contract call's gas
  budget from the signed max fee, the confirmation that carries the buyer's
  219-byte ciphertext needs 20,357 user gas, and `tools/fod/fees.go` still
  carried the superseded pre-`AH-10` digest-form figure (14,747), so the
  proposal ran the call out of gas and blacklisted the hash. The row now
  carries the live measurement taken with the node's `contract_estimateCall`,
  the submit guard refuses a short budget before broadcasting, and the same
  call settled order 2 live in tx `0xdd130fd1…` (`settled`, host credit
  1 iDNA, `pending` 0). Evidence: `PROGRESS.md` section 3, 2026-09-30
  resolution entry.
- **AH-19 (medium, new 2026-09-30, audit 5).** A delivered order whose buyer
  never confirms can still be refunded in full, and the buyer keeps the
  decrypted content: after `funded+3` the refund accepts an order in
  `POOL_ORDER_DELIVERED` while `fulfill` and `reveal` refuse at the same
  boundary, and `fod serve` plans only deliver, beacon and the proof fallback.
  Required: the supported desk settles a delivered, unconfirmed order inside
  the delivery window without the buyer - automate the prepared `reveal` (its
  calldata needs only the order id and the committed secrets, both known
  before the buy) or an equivalent - while the permissionless refund stays the
  exit for an undelivered order. Closing evidence: a delivered order whose
  buyer never confirms is settled inside `funded+3`, the first refundable
  block then refuses the refund, and an undelivered order still refunds
  exactly once. Source: the 2026-09-30 Codex Security scan
  `e87b560e-75bd-43ac-9f26-22043435ff6a` (`PROGRESS.md` audit 5). Closed
  2026-09-30: the reference desk plans the reveal for a delivered order inside
  the last `revealMargin` blocks (default 2; `-reveal-margin`) when a
  configured prover's `fulfill` no longer fits, and writes the prepared secrets
  owner-only; `tools/fod` pins the plan and the file mode, and the escrow
  boundary stays pinned by the harness. 2026-10-01: the plan withholds a
  delivery once that reveal can no longer be broadcast and included before the
  refund boundary (the last usable broadcast head is `deadline-2`) and settles
  a funded order with the reveal at that head, so the desk never hands out keys
  it can no longer settle.
- **AH-20 (low, new 2026-09-30, audit 5).** Three writers of secret-bearing
  output - `fod receive` (decrypted private parts), `fod export` (the private
  part of a flip) and `fod session-pool` (the answer salt and digests) - pass
  `0o600` to `os.WriteFile`, which only applies to a newly created file, so a
  pre-existing wider file keeps its mode. Required: every sensitive write
  re-applies `chmod 0600` after the write (the `writePrivateJSON` helper the
  audit-1 fix introduced, or an equivalent). Closing evidence: a destination
  created 0644 is 0600 and unreadable by group or others after each of the
  three commands. Source: scan `e87b560e` finding 2. Closed 2026-09-30:
  `writePrivateBytes` backs the receive, export and session-pool outputs,
  `writeExportPart` re-applies its mode, and the tests assert 0600 over a
  pre-existing 0644 file.
- **AH-21 (low, new 2026-09-30, audit 5).** The working tree embeds the
  operator's live Idena identities in `docs/PROGRESS.md` and test fixtures with
  machine-level context, and leaves `tools/fod/sweep.json` (a local marketplace
  snapshot) untracked and uncovered by `.gitignore`. Required: before any
  publication or share, tracked documents, tests and fixtures carry synthetic
  addresses (the real values stay local and ignored), and the CLI's default
  outputs are ignored so local lab state cannot be committed accidentally.
  Closing evidence: no known operator address appears in tracked content, and
  `git check-ignore tools/fod/sweep.json` succeeds. Source: scan `e87b560e`
  finding 3. Closed 2026-09-30 with one stated exception: the full addresses are
  redacted to the prefix form the documents already use, the test constants and
  fixtures use synthetic addresses, and the CLI outputs are ignored; the pinned
  real IAVL proof fixture `spikes/iavl-proof/core/fixtures/bundle.json` stays
  until a synthetic proof replaces it, because editing its key invalidates the
  proof.

Verification environment:

- **AH-16 (medium).** The app walk cannot run from the declared environment:
  `playwright` is not declared in `app/package.json` and is reached through a
  `/tmp` symlink whose `playwright-core` does not resolve. Required: declare
  the verification dependency (or document the exact install) and make the
  release check fail loudly when a required walk cannot run.
- **AH-22 (low, new 2026-09-30, audit 5).** `release/check-integration.mjs`
  keeps only the last two lines of the offline walk's output when it fails, so
  this audit's first failed run (a bare `Node.js v24.18.0` crash tail under
  parallel load; the walk passed alone twice) could not be diagnosed from the
  gate. Required: keep the failure but capture the last ~40 lines of the walk
  output, or write the full output beside the run, so a failing gate names its
  own cause. Closed 2026-09-30: the release check keeps the last 40 non-empty
  lines in the failure detail and writes the full walk output to
  `$TMPDIR/fod-walk-failure.log`; the same trace showed the walk's own 3.5 s
  session window could close before the session opened under load, so both walk
  gateways now use an 8 s window (`app/verify.mjs`).

Release artefacts:

- **AH-23 (low, new 2026-09-30, audit 5).** The harness build writes the
  `--features test-hooks` module to `contracts/escrow/dist/fod_escrow.wasm`
  (the committed blob already exports `seedPoolBundle` and `seedPoolOrder`,
  which can mint units and settle seeded orders against the contract's real
  balance; the worktree rebuild adds `seedPoolCovered`), while the manifest's
  release command also copies the release module to that same path, so the
  name does not say which build a deployer holds. The release artefact itself
  is verified to carry only the published surface
  (`spikes/harness/escrow_no_admin_test.go`). Required: separate the harness
  artefact's name from the release path (or stop the release copy into
  `dist/`), and name the artefact the release check verified. Closing
  evidence: `spikes/harness` reads the renamed test-hooks file, the deployment
  path holds only the release module, and a release check asserts the release
  artefact's export surface. Closed 2026-09-30: the harness artefact is
  `contracts/escrow/dist/fod_escrow.testhooks.wasm`, the release build no longer
  writes into `dist/`, and `release/build-manifest.mjs` refuses to promote a
  staged module whose export section is not the published 19-name surface (no
  `seed*`).

Package H acceptance: AH-1..AH-18 are closed with the stated regression
evidence and their matrix rows updated, or recorded in `PROGRESS.md` section 7
as a precise handover; AH-18 was opened and closed the same day once the fee
table carried the live confirm measurement. Audit 5 (2026-09-30) then opened
AH-19..AH-23 from the second Codex Security scan
(`e87b560e-75bd-43ac-9f26-22043435ff6a`) and from its own gate review, and all
five were closed the same day with their regression evidence and matrix rows
(`PROGRESS.md` section 3). Closing AH-1 to AH-7 removes what a cheating participant or claimant
can obtain from the reference gate; it does not make scoring
participant-verifiable, so H-3 keeps that limit stated.

### 10.6 Security repair pursuit (2026-09-30)

The owner resumed implementation following scan `edd7e9ae`. Revalidate its
findings against the current dirty checkout and preserve prior changes. Work
directly, without additional agents, threads or automations. This plan extends
the existing MVP goal; completing local repairs does not complete the MVP.

| Package | Outcome and acceptance | Components | Verification and stop condition |
|---|---|---|---|
| S1 purchase integrity | Local CID verification fails closed; prepared buys retain authenticated encrypted bytes, and signing checks those bytes against current canonical bundle terms. Buyer confirmation follows usable-content checks. | CLI CID, availability, buy/sign/receive/confirm; escrow settlement reviewed | CLI regressions and affected escrow checks once. Document any guarantee requiring unsupported canonical evidence; retain three-block/refund rules. |
| S2 utility authorization | Supported grants require pinned content, in-window wallet-controlled answer evidence and recomputed scoring; permissive provider assertions cannot authorize the supported gate. | Utility policy/access and examples | Utility tests/typecheck. Operator-controlled utility release remains a blocker unless protocol enforcement is demonstrated. |
| S3 recovery and files | Existing vaults survive path reuse and failures; imported filenames and symlinks cannot escape export directories. | CLI vault, export and pool ingestion | CLI recovery/traversal regressions once; stop rather than destroy existing recovery data. |
| S4 browser and network | Unpinned keys never yield a verified grant flow; remote responses have enforced byte limits. | Browser provider/session/setup; CLI HTTP readers | App pinning checks/build/walk and bounded-reader regressions once; retain legitimate supported input sizes. |

Follow-ups authorized on 2026-10-01 (S5-S11):

| Package | Outcome and acceptance | Components | Verification / stop condition |
|---|---|---|---|
| S5 exact deadline | Unilateral deliver/fulfill/reveal refuse at funded+3; refund pays the original buyer at that block, irrespective of ordering. Explicit buyer confirmation may still settle an open order later. | Escrow, CLI order deadlines/desk/watch, interface wording | Runtime boundary/order/rollback tests and CLI consumers. No consensus change or deployment. |
| S6 delivery domain | Versioned proof binds canonical network name, funding height and the executing chain's state root at that height. Legacy journals refuse. The complete guest/receipt/contract/CLI tuple agrees. | Delivery guest/host, verifier, CLI, release pins | Small statement checks before one local proof; valid receipt and wrong-domain/funding-fork tests. Same-ancestor descendant forks retain the chain's canonical-view assumption. |
| S7 source provenance | Manifest authenticates complete non-ignored release input bytes; a private disposable snapshot rebuilds HTML/WASM without modifying this checkout or reference dependencies. | Existing release builder/README and manifest | Exact source drift refusal and clean snapshot rebuild comparison. No publication, commit or private runtime data in the snapshot. |
| S8 unsupported guarantees | Trace what chain facts, content possession and fresh-session evidence actually establish. Implement justified local containment, record precise missing canonical mechanisms. | Existing purchase/session/utility specification and evidence | No trusted signer/oracle substitution. Independent-operator availability and actual utility release cannot be called closed from local fixtures. |
| S9 disclosure containment and recovery | Revealed keys remain in canonical order state after settlement; buyers recover without the desk or transaction indexer. CLI/desk refuse disclosure when the next inclusion block is already outside the window. | Escrow reveal, CLI receive/confirm/submit/serve and fee budget | Runtime rollback/deadline tests; retained-byte recovery, legacy ciphertext and late-signing regressions. Do not call pre-inclusion fair exchange or canonical byte availability closed. |
| S10 portable proof tuple (H-10) | Both guests build from source-only contexts at a fixed container path using an immutable builder digest. Separate clean source locations produce identical programs; genuine receipts and the contract/CLI pins agree. Release promotion checks this invariant. | Shared guest build, methods, proof artifacts, contract/CLI pins and release validation | Compare both clean guest builds before proving; generate one genuine receipt per changed image, run native and runtime verification, then release gates. Refuse unavailable builder or drift; no deployment or changed trust assumptions. |
| S11 timely safe settlement | Keep the three-block delivery-or-refund boundary. The ordinary path must make authenticated, usable content available and settle safely before the first refundable block, or repay the original buyer directly at that block. A buyer must not retain usable keys and then obtain a refund, and a seller must not receive payment for unavailable or unusable content. The 946.459854-second order-specific proof is unqualified until a faster design meets these conditions. | Purchase preparation, content availability, disclosure timing, escrow settlement/refund and buyer recovery | Demonstrate an independently verifiable design with adversarial late-inclusion, non-confirmation, vanished-provider, unusable-content and settlement/refund-race tests; measure post-inclusion latency and transaction costs on isolated play-money infrastructure. Do not extend the deadline, add a trusted operator or call a compatible proof receipt delivery evidence. |

S11 containment: a public key reveal must close every open order whose buyer
could use that same preimage. Bound concurrent orders to a measured atomic-call
limit and reject a reveal if any exposed order is already refundable or any
buyer on that bundle was previously refunded. The seller's local signing path
checks the complete open set and prior refund state again, but the contract is
the canonical enforcement point. This prevents a successful on-chain reveal
for one buyer from leaving another buyer with both usable keys and a refund.
The desk plan and the signing guard share one disclosure bound
(`tools/fod/serve.go`, `tools/fod/reveal.go`): a delivery is published only
while a later pass can still broadcast and include the committed secrets before
the refund boundary, the last usable broadcast head is `deadline-2`, a funded
order that reaches that head is settled by the reveal itself, and past it the
buyer's repayment is the only exit, so the desk never hands out keys it can no
longer settle. It does not hide pending transaction data, prove the buyer
retained ciphertext, or prevent a buyer who learned keys outside a successful
reveal from obtaining a later refund. A stalled or dropped reveal transaction,
a dishonest RPC and pending-data visibility remain open. These remain S11
qualification gaps.

No new owner decision gates these repairs. Paid supply remains disabled. Local
edits, tests and builds use disposable inputs; no mainnet, real keys, remote
hosts, deployment, funding, broadcast, spend, publication, commit or push is
authorized. Preserve the decentralisation contract. Investigate the delivery
journal's network binding, actual utility release, independent-operator
availability, reproducible source snapshot and reserve-ownership documentation
without replacing missing guarantees with trust. Each package records exact
evidence and its limits in `PROGRESS.md` before a finding is called closed.

Current security repair acceptance (2026-10-01):

- S9 retains the public reveal preimage at `preveal/<orderId>` in the same
  successful transaction that settles the order. No buyer private key is
  published. The public fallback already publishes these epoch secrets in
  calldata; this change makes that intentional disclosure durable in contract
  state, not private. `receive` accepts this source only for a settled order,
  checks the purchased commitment, restores the local vault and verifies the
  retained encrypted content. A missing or junk `pdeliv` cannot obstruct this
  recovery. Private settled ciphertext remains supported when the node
  explicitly reports no reveal record. Other RPC errors fail closed. A public
  recovery record is not a new confirmation request. Existing deployments
  without `preveal` cannot gain this record retroactively.
- S9 signing/broadcast containment applies to `deliver`, `fulfill` and `reveal`.
  It reads the current order and completed head before signing and immediately
  before broadcasting. For funding block N, the last unilateral inclusion is
  N+2 and refund starts at N+3, so a newly submitted call is refused once the
  completed head is N+2. Closed orders, missing context and mismatched reveal
  keys also refuse. This only prevents known-invalid/late local disclosure;
  subsequent delay, censorship, dishonest RPC state and pending-data visibility
  are not solved by it. Finality of an included transaction is different from
  assured inclusion of a pending one.
- The retained reveal signing budget is conservatively 2,500,000 WASM gas,
  including the historical 1,268,100 baseline, the pinned 20 user gas per stored
  key/value byte (100 WASM gas per user gas), the maximum 512-byte preimage and
  28-byte order key, plus instruction headroom. This is a budget, not a new
  chain measurement. Fees still use the actual signed transaction size.
  Historical reveal receipts do not qualify the modified module.
- Canonical encrypted-content availability remains open for arbitrary callers.
  The captured five encrypted blobs total 1,569,030 bytes; at the pinned 10
  transaction gas plus 20 state-write gas per byte, one-call publication costs
  at least 47,070,900 user gas, above the 5,120,000 upgrade-11 block limit.
  Even the smallest captured blob has an 8,740,980 lower bound. This rules out
  that naive single-call archive, not pre-publication in bounded chunks. A
  preparation phase must authenticate every chunk and the complete inventory
  before advertising availability; it needs gas/cost and independent retrieval
  evidence. A public manifest, buyer signature or beacon cannot stand in for
  those bytes.
- The small per-buyer capsule prototype is a performance candidate only.
  It has no production preparation statement or canonical availability gate,
  and an unlock visible before inclusion is still usable by its buyer. The
  current counterexample therefore stays open: delay/drop the settlement until
  N+3, then refund without revoking learned keys. No finality-dependent secret
  release primitive was established in the inspected pinned interfaces. Do not
  infer a general impossibility theorem, extend the deadline, require a private
  sequencer, or promote the prototype into the supported purchase route.

- The ordinary `prepare-buy` privately persists the exact CID-verified encrypted
  bytes. `submit` rehashes them and re-reads the bundle commitment, authenticated
  count, saleability, nonce, quote, capped fees and delivery-key context before
  signing. The durable file must remain with the buyer. Recovery through
  `receive -pool-order` requires `-prepared BUY_FILE -out RECORD_FILE`, opens
  the stored bytes with the received keys and persists usable content before
  `prepare-confirm -delivery RECORD_FILE` is allowed. The record is signed by
  the buyer's delivery key; signing confirmation rechecks that signature, the
  exact ciphertext and the currently open canonical order, buyer, delivery key,
  nonce, author and inventory count. Editing a count or timestamp cannot
  authorize payment. Freshness expires for signing, never for recovery of an
  already included purchase. Older unsigned delivery records must be rebuilt
  with `receive` using the retained buy and existing key vault. Missing cached
  bytes cannot be reconstructed merely from such a record.
- This is local buyer-path protection, not canonical proof of retained storage.
  Registration-v3 already verifies each encrypted blob's CID, opens it with the
  committed secrets and checks the supported structure. It is inaccurate to
  describe the registered keys as unbound to the content. What is missing is
  canonical availability/possession at purchase and safe disclosure timing.
  `fulfill` proves encryption of committed secrets to the order's delivery key;
  `reveal` proves the secrets' hash. Neither verifies byte retention, serving
  availability, or off-chain receipt of keys. A custom buyer can bypass the CLI
  cache. Off-chain key disclosure followed by a seller missing the deadline
  also cannot be undone by a refund. Those stronger two-sided fair-exchange
  guarantees remain unproved; do not close the full availability objective.
  `TestUnincludedDeliveryExposureCanPrecedeRefund` demonstrates that the buyer
  can decrypt a valid unincluded envelope and still recover the payment at +3.
  This is a counterexample to the current construction, not a proof that all
  decentralized fair-exchange designs are impossible. A repair needs
  independently verifiable availability and disclosure dependent on canonical
  inclusion, or a precisely justified chain-inclusion assumption. A buyer
  signature alone does not prove byte possession; no trusted signer substitutes.
- S5 reconciles the boundary: unilateral completion must be strictly before
  `funded+3`; permissionless repayment is available at `funded+3`. The buyer's
  explicit confirmation may still settle an open order later. No call extends
  the deadline. Inclusion waiting and chain stalls have no fixed wall-clock
  bound. Historical receipts exercising +3 settlement/+4 refund are for the
  previous module; this change has not upgraded any deployed contract.
- Vaults publish only complete encrypted data and never overwrite different
  keys. Identical-key recovery may reuse an existing vault. Private file
  replacement is atomic; export reads/writes use directory-rooted APIs and
  canonical CID filenames. Supported live browser verification requires an
  independently distributed provider pin. Network buffering is capped at
  16 MiB per RPC, explorer or IPFS response; this is a client support limit,
  not a consensus rule.
- S6 coordinates delivery-v2 domain/context checks. Its proof binds encryption
  and commitments to the funding anchor; it does not prove content storage.
  The genuine local receipt took 946.459854 seconds (7,405,568 cycles). The
  anchor is only available after buy inclusion, so the full order-specific
  proof cannot be precomputed before purchase. This private proof path has
  not demonstrated the three-block delivery requirement; increasing the
  deadline is not an accepted remedy. The cheap public reveal has different
  disclosure/availability assumptions and does not close this gap.
  Independent operator availability has not been established by two nodes on
  one host. S7 authenticates an exact source inventory including dirty inputs;
  a HEAD clone still cannot reproduce it without that source snapshot.
  The release builder stages HTML rebuilt from the recorded source and the
  lowered WASM, verifies the current delivery seal, and promotes only after
  their checks pass. The private snapshot's clean HTML/WASM rebuild matched
  both candidates; external CLI dependencies, cached toolchains and genuine
  new-chain execution remain separate prerequisites.
- S8 leaves the utility guarantee open: the current chain-authenticated
  statement carries author-level qualification flags and inventory, not a
  per-flip correct-answer attestation for a fresh session. A canonical design
  needs authenticated answer authority, wallet-bound answers within a defined
  chain window, atomic global consumption and enforcement of the actual
  faucet payment/access right. The target utility/asset/chain is being clarified
  with the owner; the approved five economic decisions remain settled. No new
  trusted scorer, keeper or discretionary grant is authorized as a shortcut.


#### S12 — bounded local security repairs (2026-10-03)

The owner authorized a maximum six-hour local audit and repair pass. SA-01..16
extend the existing repair plan; their closure evidence belongs in PROGRESS.
Acceptance: a nonce is consumed once despite concurrent verification; retained
answers obey a replacement-adjusted UTF-8 budget; private key/cache/grant writes
are exclusive or atomic and cannot follow a planted destination symlink;
exports reproduce supported CIDs and bind the blob, vault and inventory author.
RPC requests refuse redirects and non-success HTTP results. Unbroadcast reveal
transactions require private output and never print their secret-bearing bytes.
The browser signs only the expected fresh wallet challenge, accepts bounded
inline raster challenge images, bounds endpoint bodies before buffering, and
verifies a transcript covering the pinned challenge set. At capacity, limiter
state refuses newcomers instead of forgetting live allowances. Source snapshots
refuse known runtime credential payloads under any filename. Disposable
verification must establish that its own child owns the endpoint before calls.

Components: gateway/utility request and key lifecycles; CLI export, file and RPC
boundaries; browser provider/session/shared-node handling; release source and
service isolation. Verify each coherent group once with its adversarial tests
and smallest relevant suite, then the final integrated walk and staged release
gates. Preserve existing dirty inputs. Stop/report when a remedy needs existing
credentials, remote infrastructure, chain actions or unsupported canonical facts.
These local controls do not establish historical answer authority, compel
utility release, solve pre-inclusion key disclosure, prove independent-operator
availability or qualify the slow proof for the strict three-block deadline.

The owner subsequently authorized replacing the exposed local application
signing key and its local trust pins. Verify replacement-key reload, browser
and utility rejection of old signatures, private quarantine permissions and
refusal of a stale served pin. Keep portable examples and historical snapshots
intact; external copies must adopt the replacement pin. This authorization does
not extend to wallet keys, remote infrastructure or chain actions.

## 11. Verification and completion criteria

### 11.1 Required test coverage

| Area | Scenarios |
|---|---|
| Eligibility | Strong/weak consensus, reported result, missing result, failed validation, active epoch, unsupported version |
| Inventory | Omitted CID, deleted submission, repeated CID, wrong author or epoch |
| Source authentication | Altered headers, substituted snapshots, missing bodies, incorrect commitments |
| Keys | Wrong secrets, corrupted ciphertext, invalid authentication tags, malformed flip structure |
| Proofs | Forgery, changed outputs, wrong program ID, development receipts, cross-context replay |
| Buyer binding | Wrong payer, replaced delivery key, missing possession proof, wrong nonce |
| Pool | Quote computation, buy and sell conservation, round-trip non-profitability, unit exhaustion, donated units, stale beacon refusal, strike accounting |
| Orders | Concurrent buyers, incorrect amounts, delivery after the deadline, duplicate fulfillment, duplicate confirmation |
| Deadlines | Fulfillment and refund boundaries with no grace period, the first refundable block, attempted deadline extension, and a delivery refused after the window |
| Accounting | Double withdrawal, failed transfer, reserve conservation, liability conservation, maintenance separation |
| Privacy | No secrets in public files, browser storage, transactions, logs, or proof outputs |
| Local HTML | Direct opening, offline FAQ, bundled resources, malicious imports, stale-state indicators, the three journeys of 4.6 |
| Availability | Unfetchable blob refused before signing, stale advisory, refreshed advisory, bundle fails closed |
| Session binding | Submission inclusion through the block body CID, a shard's identity proofs, the seed proof, the answer attachments, and refusal of a bundle that proves only internal consistency |
| Retrieval fee | Cap enforcement, repeated hosts, an empty list, a host credit that is never debited from the reserve, and withdrawal of the credit |
| Shared gateway | Unauthorized access, forbidden RPC calls, credential exposure, oversized requests, request limits |
| Sessions | Replayed enrollment/answers/results, expired results, participant binding, scoring-policy mismatch |
| Recovery | Restarts, restored buyer key, index rebuild, unavailable gateway |
| Distribution | Checksums, pinned identifiers, reproducible artifacts |
| Mainnet | Real registration, payment, delivery, settlement, recovery, refund receipts |

Automated tests use synthetic keys. Historical-evidence tests use authenticated
public history. Real keys and designated funds enter only in the explicit
mainnet exercise.

### 11.2 Definition of done

1. Anyone can obtain and locally open the HTML.
2. Independent marketplace operation works through a user's own node.
3. Providers can offer shared-node access without exposing administrative
   credentials.
4. Customers can use the participant integration without a full node or
   marketplace CLI.
5. Sellers can inspect historical flips and prepare eligible original-key
   bundles.
6. Incomplete or ineligible bundles cannot register.
7. Buyers can deposit play-iDNA on the isolated test chain; real mainnet deposits
   are a separate later release criterion, requiring explicit approval.
8. Correct original keys are delivered encrypted to the funded buyer.
9. Invalid, substituted, or replayed delivery evidence cannot settle.
10. A bundle is sold through the pool at the quote its reserve and units imply,
    and the delivery that settles it is bound to the funded buyer's key.
11. Buyers recover deliveries after restart.
12. Unfulfilled orders can be refunded in one permissionless transaction that
    pays the original buyer the purchase amount and the retrieval fee directly;
    a host's credit can be withdrawn.
13. Marketplace commission is zero, and no person receives a share of a trade;
    any retained spread stays in the reserve.
14. The FAQ accurately explains gated utility, provider hosting, AI
    capabilities, costs, and limitations.
15. Release evidence identifies the exact source, artifacts, contract, proofs,
    and transaction outcomes.
16. A gifted listing sells without paying its donor, and a seller can list,
    keep a delivery desk running, and withdraw without an operator.
17. Each of the three journeys in 4.6 completes in the browser walk with no
    more actions than that section records.
18. Reserve, unit, and quote invariants hold under concurrent calls, refunds,
    and strikes.
19. No unit enters the pool without a verified session binding: submission,
    shard inputs, seed, and answers check against committed roots, and an
    unbound bundle can never take a unit or a payout.
20. A retrieval fee reaches only the hosts the buyer names, comes only from the
    buyer's own payment, stays within the recorded cap, and the reserve pays
    nothing for retention.

The handoff must distinguish implemented behavior, measured results, and
remaining blockers. D0-D5 and closure of A01-A10 define the immediate goal;
mainnet capability remains a future phase. The cryptographic guarantees and
the actual user journeys must be demonstrated, not inferred from component tests.

## 12. Open items recorded at baseline

These are unresolved questions that later packages must answer with evidence.
They are not permissions to relax the requirements above.

| ID | Item | Blocks | Status |
|---|---|---|---|
| O-1 | Does a Groth16 verifier (arkworks or equivalent) fit the actual contract runtime limits: code size, memory, gas per call, and host-function surface of the contract runtime? | Package 1 | closed - a real RISC Zero Groth16 receipt verifies in the runtime at 195,325,117 wasm gas of a 512,000,000 ceiling, and a tampered claim digest is rejected (`SOURCES.md` F-17 to F-21) |
| O-2 | Which exact historical epoch range is covered by the pinned consensus version, and how is "strong qualification" encoded per epoch? | Package 2 | closed - a flip's verdict turns on exactly two switches, `Upgrade10` and `Upgrade11`, both set from consensus V11 onwards; the epoch-conditional flags elsewhere govern identity states and rewards, not flips. V11's activation window (2023-03-01T08:00Z to 2023-03-07T00:00Z) closed inside epoch 103 and its config carries `KeyWordsV3Epoch = 104`, so the covered range starts at epoch 104, and the tooling refuses earlier epochs with that explanation instead of applying the arithmetic to them (`SOURCES.md` F-84) |
| O-3 | Exact canonical encoding for commitments and proof inputs, with cross-language fixtures. | Packages 2-4 | closed - every commitment has a frozen fixture both implementations are checked against: `proofs/registration/vectors/` holds the manifest commitment, the blob CID with its canonical secrets and secrets commitment, the lottery distribution and the qualification vector, each read by a `tools/fod` test; the registration journal itself is now the version 2 artefact (`proofs/registration-v2/dist/registration-v2.json`), which `tools/fod/registration_test.go` decodes at the CLI's offsets and requires the claim model to re-encode byte for byte (`claim-v1.json` records the superseded version 1 layout); `proofs/delivery/dist/delivery.json` and `order-example.json` are the Rust-produced delivery artefacts `tools/fod/delivery_test.go` re-parses |
| O-4 | Exact ECIES construction and wire format used by the pinned node version, with compatibility vectors. | Packages 3-4 | closed - `proofs/vectors/ecies-v1.json` pins the construction (`tools/fod/construction_test.go`), and `proofs/registration/vectors/node-flip-v1.json` is a blob a live lab node produced through its own `flip_submit`, which this CLI's derivation opens (`tools/fod/node_flip_test.go`) |
| O-5 | Custom WASM contract deployment path and requirements on current mainnet (code-hash acceptance, upgrade flags, size limits). | Packages 1, 5 | closed for this chain - consensus version 11 sets `EnableUpgrade11` (`idena-go/config/consensus.go`, `ApplyConsensusVersion`) with an activation window of 2023-03-01 to 2023-03-07, and a deploy that carries code is refused while the flag is off (`blockchain/validation/validation.go:744`). Mainnet is far past that window (epoch 228 on 2026-09-25), so the deploy path is open; the runtime revision and limits mainnet actually executes stay `O-10` |
| O-6 | Idena promise/callback semantics for `withdraw` under failed transfers. | Package 5 | closed against the pinned runtime - `create_transfer_promise` debits the contract in the same call, the promise executes when the call returns, and the `Action::Transfer` arm delivers no callback, so the payout is atomic and an unbacked credit aborts it (`SOURCES.md` F-41). The deployed mainnet revision is compared separately under `O-10` |
| O-7 | Whether the reference session adapter needs any on-chain component for the utility it demonstrates. | Package 7 | closed - it needs none. The gateway's routes are a fixed table with no RPC passthrough and it never forwards a transaction (`gateway/src/routes.ts`), and `release/check-integration.mjs` runs the whole loop - session, provider-signed result, utility grant, replay refused, quota spent once - with no chain and no node involved |
| O-8 | Toolchain configuration that compiles the verifier without bulk-memory operators, and the resulting module size. | Package 1 | closed - the release profile plus `wasm-opt --llvm-memory-copy-fill-lowering -O3` produces a module the gatekeeper accepts with **zero** forbidden operators, 357,695 bytes for the release artefact and 359,031 with test hooks (`release/MANIFEST.json` `checks.gatekeeperForbiddenOperators`, recipe in `SOURCES.md` F-14) |
| O-9 | Whether one verification fits a single call within block gas and block production time, and what fee that implies for a real order. | Package 1 | partial - two to three pairings fit the gas ceiling with margin; `fod fees` prices the measured gas and transaction size at the chain's rate, and on 2026-09-25 mainnet's rate was 0.0000961538461538 iDNA per gas, so a call that carries a proof needs about 188 iDNA (`SOURCES.md` F-76, F-77). The wall-clock such a call takes on mainnet is unmeasured |
| O-10 | Which contract runtime revision mainnet actually executes, and whether its limits match the measured local fork pin. | Package 1, before any mainnet claim | closed - mainnet's release links the upstream runtime archive set (binding `4227b9778d3d`, built from `idena-wasm` `9fc402e`, the repository's last commit), while this project measured a fork 29 commits ahead. The files that decide gas and gatekeeping are unchanged between them; the whole escrow harness passes against the released binding, and the registration call is metered identically at 195,478,684 wasm gas under both (`SOURCES.md` F-90) |
| O-11 | Amounts above about 18.4 iDNA overflow the escrow's `u64` accounting, so prices, totals, and credits saturate. | Package 5 | closed - prices, escrowed totals, and credits are 128-bit, and the suite runs at 100 iDNA with exact accounting (`SOURCES.md` F-47). Amounts above `u128::MAX` are refused by the CLI and can only fail comparison in the contract |
| O-12 | A flip larger than one IPFS chunk is submitted with a UnixFS/dag-pb CID rather than the raw CID the manifest commitment and the registration guest both assume (`SOURCES.md` F-86). Which side moves: the commitment gains the UnixFS form, or such flips are refused with an explanation? | Package 2, before any bundle containing one is claimed to register | closed - the proof program moved, because refusing would have left the flips the chain actually accepts unregistrable and our own exercise bundle is one of them. The registration core rebuilds the importer's layout and `blob_cid_multibase` returns whichever form the blob's size implies, checked against a live node's answers for five sizes and against five real epoch-228 flips (F-88, F-89). The escrow pins the new guest image id, a receipt from it registers in the runtime, and the CLI refuses a bundle whose blobs do not build their CIDs before a fifteen-minute proof is started |

| O-13 | Bootstrap parameters for the pool: the reserve amount, the units each bundle starts with, and the spread, if any | Funding profile, before any funding | open - the owner sets the ratio at funding time so the start attracts sellers while quotes fall as supply grows. Every rehearsal uses play money; the numbers are chosen before the mainnet profile is funded (`docs/PROGRESS.md`, 2026-09-26) |
| O-14 | Are supply issuance, reserve payouts, pending refunds and withdrawal liabilities safe under arbitrary hostile sequences? | Any paid supply or funded release | Reopened by REVIEW.md A01/A03/A04/A05. Product-monotonicity and honest lifecycle tests missed public-receipt payout replay, unlimited issuance and refund insolvency. D0/D2 must establish authorization, finite inventory and segregated liabilities; the old round-trip result is insufficient |
| O-15 | Does the contract/guest authenticate the entire session evidence chain, including its root and completeness? | Every accepted paid inventory unit | Anchor and content halves closed in code, live half open. The version 2 statement folds two membership proofs - the author's record at the inventory height and at the epoch's `ValidationFinished` block - and the contract compares each folded root against the header its own chain holds for that height (`SOURCES.md` F-96, F-97; `contracts/escrow/src/lib.rs`). The guest decodes both records itself: the recorded flips must equal the manifest, the global entries must name `Epoch == N` and `Epoch == N+1`, the label comes from the chain's flags rather than the seller, and the content must be a flip the renderer can show (5.5 item 6, `SOURCES.md` F-98). What remains is the live capture over a real session, because the frozen evidence is a node-encoded fixture. The assignment-replay route is superseded, not pending |
| O-16 | Who pays for retention: pinning blobs and producing anchor bundles from a live capture or a chain replay? | Pool package, before any funding | decided 2026-09-26 - the reserve pays nothing; the demand side pays, and the buyer pays a small capped retrieval fee to the host that served the flip data (5.9, 7.1, 7.4). The cap is per covered flip, never per byte. Open sub-questions: whether the fee rides in the purchase call or settles as a direct transfer, and the per-flip fee value |

Assumption A-1: the pinned reference node revision used for protocol derivation
is compatible with the mainnet consensus version pinned in Package 2. **Verified
2026-09-26**: a mainnet node's own log, replaying the chain, transforms its
consensus config to `ver=10`, `ver=11` and finally `ver=12`, and the pinned
checkout's table
(`config/consensus.go`) defines 10, 11 and 12 and nothing later, so mainnet runs
the revision whose rules that checkout carries. The pinned parsers have also read
real mainnet data: epoch 204's transactions and attachments, and a live node's
epoch store (`SOURCES.md` F-85).

Assumption A-2: a self-contained single-file HTML bundle with no remote assets
remains acceptable for browser security review once imports from providers are
considered untrusted. **Upheld by evidence 2026-09-26**: the shipped file carries
no remote script, style or font (`release/MANIFEST.json`
`checks.interfaceRemoteTags` is 0), the browser run asserts that it makes no
remote request of its own, that markup in imported data is neutralised, that
nothing is kept in browser storage, and that a gateway it cannot read is refused
rather than rendered; and the page has no analytics, remote error reporting,
code loading or login. Whether a given reviewer accepts that design is their
judgement; what this record can show is that the claims behind it are checked.
