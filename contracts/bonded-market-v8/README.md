# Bonded curve market v8 — implementation candidate

Status: deployed once on 23 October 2026 UTC (24 October locally), with the receipt, canonical block and code pin independently verified through both native nodes. Recorded network cost: **499.413725490325926 iDNA**, below the approved 550-iDNA ceiling. **Zero initial pool reserve, inventory and trading stakes. No funded v8 trade has been performed.** The existing Direct v7 contract and pilot remain separate. No star premium or star priority is included.

## Trading rules

- Buyers and sellers maintain reusable security stakes. Each queued order reserves its full permitted trade exposure; buyer payment is additional escrow. Reserved funds cannot secure another order or be withdrawn.
- A dispute deduction pauses new trading until the recorded stake target is restored. Existing orders can still be cancelled or resolved. A target cannot be lowered to evade a deduction. Withdrawal is an explicit full exit only, after all queued orders, trades and disputes release their reservations. Partial withdrawals are rejected. A participant can withdraw the remaining deposit after a settled penalty without refilling just to exit; the penalty flag and original target survive, so re-entry still requires replenishment.
- Buyers queue a maximum spend; sellers queue minimum earnings and a covered execution limit. The earliest compatible buyer is paired with the earliest compatible seller. Assigned orders retain their bounded queue slots. Unready buyers can be refunded without losing the seller's original position.
- A funded pool can reserve earnings for a bundle before a buyer arrives. These are pending earnings, paid after private delivery is accepted or proven. Pool acquisition never spends buyer escrow or security stakes. At zero free reserve, pool acquisitions stop; a buyer-funded match can still execute.
- There is at most one reserve acquisition entitlement per author and epoch. Original epoch keys cover the canonical bundle together. Private delivery does not guarantee exclusive knowledge or prevent copying.

## Pricing and accounting

Every bundle has weight 100. With free reserve R (base units) and acquired inventory Q:

- Buyer quote: `ceil((R + 100 iDNA) * 100 / (Q + 1000 - 100))`.
- Seller quote: `floor((R + 100 iDNA) * 100 / (Q + 1000 + 100))`.

Quotes change with acquired supply and settled demand. Cancelable queue entries do not move prices. Virtual balances set the curve; they cannot fund payouts. The initial zero-reserve quotes are approximately 11.1111 iDNA to buy and 9.0909 iDNA to sell. A queued order's limits are checked at execution.

Free reserve, buyer escrow, total stakes, and reserved seller earnings are separate liabilities. In a direct match the spread enters the reserve on settlement. For an acquired bundle the buyer's payment enters reserve and the previously reserved seller earnings are paid. A later valid sale below acquisition cost reduces reserve: collateral does not insure price movements. Reserve contributions are permanent and issue no redeemable shares.

## Private delivery and disputes

1. The contract assigns a buyer, seller, bundle, price, encryption public key and deadlines.
2. Before declaring readiness, the buyer's local CLI retains every original encrypted flip and verifies its CID. Readiness binds the prior canonical header state root.
3. The seller posts one immutable, buyer-encrypted payload on chain. The original encryption witness stays on the seller's machine.
4. The buyer's local CLI decrypts that exact payload, checks its committed author and epoch keys, and opens every retained flip. It persists an encrypted local vault before preparing acceptance.
5. Acceptance settles once. A complaint or missed acceptance deadline permits a proof of the **original** ciphertext to the assigned buyer. The pinned Groth16 delivery verifier resolves this without publishing plaintext keys or using an arbitrator.
6. Missing delivery or an unproved delivery is refunded after the deadline. No corrected ciphertext, substituted recipient, late proof or replay can settle the original trade.

Candidate compensation schedule (must be reviewed before funded use): seller default refunds payment and pays 10% of the execution price from seller stake. A successful proof after an explicit buyer challenge charges 5% plus bounded proof cost to the buyer. Silence alone charges only proof cost. Proof cost is `min(feePerGas * 2,500,000, price / 20)`. Full collateral coverage does not mean the entire stake is automatically confiscated.

Deadlines in blocks: queued order at most 4,320; buyer readiness 180; seller delivery 720; acceptance 180; proof window 12,960. Each queue and the active trade index are bounded at 64. Expiry settlement is permissionless; someone must submit a transaction.

## Local client and native app

The CLI adds `fod v8` status, typed preparation, retention, delivery and receive commands. Submission reuses the existing pinned binary, independently checked contract code, active identity, nonce journal, simulation and uncertain-broadcast safeguards. There is no hosted matching or delivery server.

The native app first detects current flips, distinguishes registered sellable bundles, shows live curve quotes, and explains the reusable deposit and full-exit rule. Its suggested deposit is rounded up to whole iDNA. Deposit review retains an exact native fee and signing confirmation. After setup, the app presents Sell now, Sell to pool, Buy now, or the appropriate queue action based on current compatible demand and actual free reserve. It remembers the verified v8 profile; the original v7 profile is preserved.

Automatic handling requires an explicit native authorization: eight hours, a 25-iDNA aggregate network-fee ceiling, and matching, pool reservation, retention/readiness, delivery and verified acceptance while the app is open. Users can additionally authorize one-click orders with a per-trade cap, aggregate purchase budget, and 5% quote protection. Only an explicit user click creates an order; the background worker never originates orders. The main process refreshes identity, contract pin, eligibility, collateral, price, nonce and unresolved-attempt status before preparation and signing. The shared signing boundary consumes fee and purchase allowances conservatively before signing; uncertain attempts are not automatically refunded. Deposits, withdrawals, reserve contributions and disputes still require separate confirmation. Queued orders persist after local permission expires. Seller automation uses the selected owner-only epoch vault, whose contents are independently checked by the CLI; buyer delivery keys and vaults stay local.

Dispute proof generation is **not automated** in this candidate UI. The seller uses the existing local delivery prover with the saved original witness, then selects the resulting proof for submission. CPU, memory and completion time must fit the proof deadline. Closing the app stops local automation.

## Verification and remaining release gate

- 14 Rust tests cover curve movement, both settlement paths, stake and escrow isolation, concurrent liabilities, refunds, penalties/top-up, readiness/FIFO, deadlines and genuine proof binding.
- Two fresh native WASM tests execute canonical registration and cooperative settlement, plus a genuine original-ciphertext delivery proof and dispute settlement. Measured user gas: registration 1,968,203; delivery proof 2,002,085.
- Five new CLI tests exercise proof-compatible encryption, native decryption/tamper rejection, typed signing, retained inventory identity, file permissions, exclusive creation and action-specific fees including transaction-size fees. Relevant existing retention/private-delivery tests also pass.
- 44 targeted native-service/UI tests pass (37 service/signing, 7 UI), including the existing signing and nonce safeguards. Targeted lint and renderer builds pass.
- The final 505,785-byte artifact passes the unsigned native constructor simulation and the native WASM tests. Estimated total gas with an empty deployment nonce was 5,092,643 against the 5,120,000 cap. The actual deployment used a fresh 32-byte nonce, a maximum fee of 501.960784313856 iDNA (the chain cap at the sampled rate), and one base unit for the required strict-value ABI. This is not a reserve contribution.
- The deployment succeeded at block 11,478,465. Both nodes agreed on the successful receipt, transaction, canonical block roots, code hash, and zero reserve/inventory/stake totals. The account nonce guard was released only after verification. Hash-only public evidence is saved under `docs/evidence/v8-guided-deployment-verification.json`; the complete deployment record remains owner-only on the host.
- The guided app is installed on IdenaAI 2 and uses its verified v8 profile. Existing v7 configuration is preserved. The native node recovered after the desktop service restart; both nodes resynchronized, both original witnesses verified unchanged, and all ten encrypted CIDs verified. Later renderer-only polish does not restart either node.

These checks verify deployment identity and tested behavior. They do not demonstrate a funded production trade or economic profitability. Contract-bound bundle registration and explicitly funded buyer/seller security deposits are still required before trading. Pool acquisitions additionally require free reserve; at zero reserve, buyer-funded matching and queues remain available. The current five epoch-233 flips are not automatically sellable before validation and v8 proof registration. Do not silently reuse a v7-bound proof. No stakes, pool contributions, proof registrations or trades were submitted as part of this deployment.


## Build

From the repository root, using the existing pinned toolchain:

```sh
cargo test --offline --manifest-path contracts/bonded-market-v8/Cargo.toml --lib
cargo build --locked --offline --release --target wasm32-unknown-unknown --manifest-path contracts/bonded-market-v8/Cargo.toml
wasm-opt --llvm-memory-copy-fill-lowering -Oz contracts/bonded-market-v8/target/wasm32-unknown-unknown/release/fod_bonded_market_v8.wasm -o contracts/bonded-market-v8/target/bonded-v8.wasm
```

All exported entry points use the strict one-byte-array ABI. `deploy`, `addReserve`, `acquire` and `matchNext` take the one-byte version marker `08`. State-only calls attach and refund the one-base-unit sentinel; deposits and buyer orders require their exact positive value. See the typed CLI encoder for layouts.
