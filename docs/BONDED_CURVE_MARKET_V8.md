# Bonded curve market: proposed settlement rules

Status: design proposal, 23 October 2026. This records the requested buyer and
seller security stakes and automatic acceptance. The implemented candidate and
verified limits are now documented in [the v8 implementation guide](../contracts/bonded-market-v8/README.md). It is not deployed. Current Direct v7 remains a different contract. This proposal replaces
the earlier fixed-price publication proposal for the private curve-market flow;
that earlier document is retained as historical evidence.

## Product behavior

Buyers and sellers trade at the curve price when their order executes. A queued
buy specifies a maximum spend, and a queued sell specifies minimum proceeds;
these are execution limits, not fixed market prices. Either side can wait when
inventory, liquidity, or a compatible counterparty is unavailable.

Both participants provide a separate security stake before a trade can execute.
Matching locks the price, the eligible author-epoch bundle, the buyer delivery
key, the exact obligations, and the deadlines. The seller immediately sees
pending proceeds. Those proceeds become withdrawable only after authenticated
buyer acceptance or a successful contract-verifiable delivery proof. Thus the
normal path can complete promptly, but a disputed trade never presents pending
earnings as spendable funds.

The unit is the canonical author-epoch bundle: original epoch keys cover that
author's eligible flips together. Enforce at most one reserve-funded acquisition
entitlement per author and epoch. Private delivery does not guarantee exclusive
knowledge or prevent copying outside the market.

## Stake requirements and trading capacity

For an order with an executed price P, reserve at least P from each participant's
stake, independently of the buyer's escrowed purchase price. Reserve an additional
bounded verification-cost allowance wherever the agreed maximum charge can
exceed P. A 100 iDNA purchase therefore needs 100 iDNA payment plus at least
100 iDNA buyer stake; the seller needs at least 100 iDNA seller stake. Network
fees and any additional cost allowance are shown separately.

An account may maintain one reusable stake balance, but every live obligation
reserves its own share. The same funds cannot cover two simultaneous orders or
both roles in separate orders. For each account:

- Stake balance must cover the sum of all active reserved liabilities.
- Each new order must fit within free, unreserved stake and the configured trade
  limit. A standing queue order reserves enough for its permitted execution or
  is explicitly ineligible until the required capacity is available.
- The required maintenance target covers the account's agreed epoch trade
  capacity, with buyer and seller roles combined. Epoch rollover cannot release
  collateral reserved for unfinished orders or disputes.
- An adverse dispute deduction marks the account as requiring a top-up. The
  contract refuses new trading commitments until the stake reaches its recorded
  maintenance target again. The account cannot lower that target to erase the
  deduction or an existing obligation.
- Top-up, evidence submission, acceptance, settlement, refunds, cancellation of
  unmatched orders, and recovery remain available while new trading is paused.
  A queued order must pass the stake check again at execution; it cannot execute
  while the participant is underbonded.
- Only genuinely unreserved excess can be withdrawn. Any reduction in future
  trading capacity needs a declared exit rule and cannot evade an outstanding
  dispute or top-up requirement.

Full collateral coverage is separate from the penalty amount. A losing dispute
does not automatically transfer the entire stake to the other party. The
contract applies a published compensation and cost schedule, bounded by the
stake reserved for the order, and releases the remainder. Fees claimed by a
participant cannot be arbitrary invoices.

## Normal path and automatic acceptance

1. The buyer funds the price and both sides reserve their collateral. The
   contract validates the buyer's encryption public key and binds it to the
   order. A seller cannot replace that key, the price, or the bundle afterward.
2. The seller submits the buyer-specific encrypted key payload through a
   delivery record that provides canonical availability and an immutable hash.
   The delivery record is bound to the contract, order, nonce, buyer key, author,
   epoch, canonical inventory, and key commitment.
3. The buyer's local client retrieves the exact ciphertext, decrypts the keys,
   checks the commitment, verifies every promised original blob against its CID,
   decrypts every promised flip, and checks the authenticated validation and
   content evidence. A reachable file or matching ciphertext hash alone is not
   sufficient. The recovered material is saved locally before acceptance.
4. If all checks pass, the client automatically submits an order-specific signed
   acceptance. The contract settles the order once, makes proceeds withdrawable,
   and releases the reserved stake shares.

Automatic acceptance is a narrowly scoped authorization selected with the buy
order: only that order and verified payload can be accepted, with a bounded fee,
expiry, and replay protection. It grants no authority to place another trade,
withdraw funds, change a delivery key, or access the buyer's identity secrets.
The app displays "Verified and completed"; a manual button, when needed, is
"Accept verified delivery", rather than "Undisputed".

The confirmation window starts from a complete, timely, canonically available
delivery, not from an arbitrary seller assertion or the buyer entering a queue.
There is also a separate fixed delivery deadline after matching, so a seller
cannot keep funds locked by never delivering. Generic queued buyers need an
availability/assignment step for the eventual bundle before their verification
obligation starts; their client cannot pre-check an unknown inventory.

## Missing acceptance and disputes

An explicit buyer challenge enters dispute immediately. If the buyer has not
accepted by the confirmation deadline, the order becomes eligible for mandatory
verification. Anyone may invoke the deterministic transition; a smart contract
does not wake itself on a timer. Both cases keep proceeds and the order's stakes
locked. Silence is not itself proof that the buyer or seller acted dishonestly.

The seller has a bounded proof period, long enough for measured generation and
inclusion costs, to establish the original delivery obligation. No administrator
judges a story. The contract must verify the exact objective statement:

- The original, timely ciphertext encrypts the committed authentic epoch keys
  to this buyer's valid public key for this order.
- Those keys open the original authenticated CID inventory and satisfy the
  agreed content and validation predicate.
- The ciphertext was available through the agreed canonical mechanism by the
  delivery deadline. A hash alone is not availability evidence.

An original-content proof, a screenshot, a new valid key, or a replacement
ciphertext created after the complaint does not prove that the original delivery
was correct. Any permitted correction is a distinct recovery event and cannot
justify a penalty against a buyer whose original complaint was valid.

| Outcome | Purchase payment | Stakes |
| --- | --- | --- |
| Buyer accepts a locally verified delivery | Pay the recorded seller proceeds; account for any declared pool share once | Release both reserved shares without a dispute charge |
| Seller proves the original delivery was valid and timely | Settle the locked trade | Release seller share; charge buyer only the predefined bounded verification cost and, for a rejected explicit challenge, any separately disclosed penalty; release buyer remainder |
| Original delivery is invalid, absent, or unproved by the final deadline | Refund buyer from the purchase escrow; seller receives no proceeds | Release buyer share; deduct the declared seller default charge and compensate the eligible parties; release seller remainder |

Automatic escalation caused by an offline buyer can incur a disclosed necessary
verification charge if the seller proves valid delivery. It does not by itself
justify a dishonesty penalty. Explicit challenges and timeout verification must
remain distinguishable in state and in the UI.

Settlement is terminal and mutually exclusive with refund or slash. A failed
transfer must not leave half a settlement recorded. No funds can be recovered
twice, no additional dispute can extend a deadline indefinitely, and proof,
acceptance, cancellation, refund, and timeout races have deterministic precedence.

## Curve and pool accounting

Keep spendable reserve, buyer payments, pending seller entitlements, buyer stake,
seller stake, insurance, and other liabilities separately accounted. Escrow and
stakes never become liquidity for another trade. Once a price executes it is
fixed for that order; later curve movement changes future quotes, not collateral
or compensation already agreed for that order.

The intended price response remains: acquired supply increases available bundle
inventory and consumes reserve, lowering subsequent quotes; completed purchases
consume inventory and replenish reserve, raising subsequent quotes. Cancelable
queue counts alone do not create priced supply or demand. Reservations must
reduce executable capacity without counting refundable money as free reserve.
Their exact effect on quotes and cancellation must be fixed and tested with the
curve formula. A quote backed by virtual reserves cannot authorize an unfunded
payout. When free reserve is exhausted, reserve-funded purchases stop and sellers
wait or match an already funded buyer.

For a seller selling to the pool before any buyer exists, there is no buyer
acceptance or buyer bond yet. The UI must distinguish a queued offer or pending
pool entitlement from a completed sale. An independently withdrawable pool
advance remains a separate feature requiring coverage of the advance plus all
remaining delivery liabilities. The same loss must not be charged twice against
the seller's stake, pending proceeds, and a buyer's escrow. Public key release
is a separately chosen trade mode; it is not the private dispute default.

## Decentralization and verification boundary

Each party's local client and node perform checking, encryption, signing, and
watching. Contract state and permissions determine settlement. Anyone can submit
a valid proof or trigger an expired state transition, with compensation bounded
by the terms. Operation must not depend on a named relayer, hosted market API,
central key custodian, or discretionary adjudicator.

A private fallback requires a sound proof about the buyer-bound encryption and
the authentic keys. Verifiable encryption is an established cryptographic
direction, but the needed circuit, contract bindings, canonical availability,
Idena verifier support, generation time, and gas cost have not been demonstrated
by this proposal. Current v7's content proof and public-reveal fallback do not
provide that private delivery proof.

Before live implementation, freeze the curve, trade limits, maintenance target,
cost schedule, deadlines, and exit rules. Verify full-liability conservation;
concurrent order reservations; epoch rollover; automatic acceptance authorization;
invalid buyer keys; unavailable blobs; fake and late-corrected deliveries; false
challenges; offline clients; timeout/proof races; one-time settlement; and trading
pause/top-up recovery. Also test common-operator and colluding trades: an account
bond is not proof of independent demand, and a curve invariant alone is not proof
that the reserve cannot be drained economically.

## Evidence and references

- Current v7: `contracts/direct-escrow-v7/README.md`, `challenge`, `confirm`, and
  the public fallback in `contracts/direct-escrow-v7/src/lib.rs`. V7 uses a buyer
  proof-cost bond and seller delivery collateral; the symmetric full-volume
  account stakes and private fallback here require new contract behavior.
- Sale unit, issuance, and reserve boundaries: `docs/SPEC.md`, section 5.1.
- [FairSwap: How to fairly exchange digital goods](https://eprint.iacr.org/2018/740.pdf)
  describes smart-contract adjudication of digital-goods exchange; it is prior
  art for objective settlement, not evidence that this private-key protocol is
  implemented or secure.
- [Camenisch and Shoup: Practical Verifiable Encryption and Decryption of Discrete Logarithms](https://eprint.iacr.org/2002/161)
  describes proofs about encryption and decryption. Applying such guarantees to
  the exact flip ciphertext and content relation needs its own implementation
  and security review.
