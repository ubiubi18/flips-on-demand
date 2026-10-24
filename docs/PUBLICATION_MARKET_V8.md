# Flips on Demand: publication market proposal

Status: design proposal, 23 October 2026. No v8 contract, funded pool, or live bid flow is implied by this document. The deployed strict-value Direct v7 contract cannot pay an author merely for listing or publishing keys and cannot accept standing buyer bids.

## What is being sold

A flip bundle has two distinct products:

1. **Private delivery:** a funded buyer pays for prompt access through the contract's delivery and settlement process. The contract cannot promise exclusivity outside this market.
2. **Public release:** one or more sponsors pay for a proved bundle's keys to be published. They receive a verifiable funding and provenance receipt, a usable public dataset, and the release they requested. Everyone can then copy the keys. Subsequent payments for that bundle are voluntary support, not access purchases or dependable royalties.

Only an author-epoch bundle whose validation is final and whose original content and key commitment have been independently proved is eligible. Release must not compromise an upcoming validation or an active private order. Proof establishes correspondence to the validated flips; it does not establish ownership of third-party source images or a commercial licence.

## Funded demand first

A buyer can escrow an **exact-price bid** with an expiry, choosing private delivery or public release. A bid can name a specific bundle. A generic bid can instead specify objective eligibility (epoch range, flip count, validation verdict, proof status); the UI must make clear that these criteria do not judge subjective quality. Eligible generic bids match by highest exact price, then earliest bid at that price. An explicit contract call performs matching; the queue is not an autonomous agent. Unmatched bids can be cancelled or expired for a full refund. Bid escrow is a liability and is never counted as spendable reserve.

For public release, several buyers may pledge toward a seller's stated release price. Their escrow pays the author only if the threshold is met before expiry and the proved keys are published; otherwise each pledge is refundable. This is an assurance mechanism for sponsors who want a particular release, not a way to prevent non-contributors from using public keys. A matched single bid is the one-buyer case. Sponsorship receipts confer no exclusive access, financial return, or copyright licence.

For private delivery, the bid also contains a valid on-curve delivery key and the required buyer proof-cost bond. Matching checks the seller delivery bond and moves the deposit into a funded private order with confirmation, public fallback, and refund deadlines. The contract itself must validate the curve point, closing the raw-RPC delivery-key gap in v7. On an original first sale, the author receives the exact bid price on valid settlement, subject to any explicitly shown contract fees; there should be no hidden spread. A resale needs its own clearly displayed split before it can accept a bid.

## Optional instant publication backstop

An author can also accept a **firm publication-pool payout** when a separately funded, visible publication budget has capacity. This is a subsidy or sponsor-funded purchase of a public release, not a redeemable AMM share. It must have an untouchable reserve floor, a fixed per-bundle payout rule, total and per-epoch spending caps, and at most one payout per author-epoch bundle. The amount displayed to the author is the exact amount reserved for that bundle, with network costs shown separately. No pending private payment, buyer bid, seller collateral, or insurance balance can finance it. If no funded budget is available, the action is unavailable and the UI offers a funded bid or listing path.

Public release is irreversible. A pool-cashed-out bundle earns no automatic author royalty from later purchases of the same public keys; any later support to the author must be clearly voluntary and separately funded. The pool must not base its payout on self-trades or claim that public-key resales will replenish the budget. V7's locked activation deposit cannot be withdrawn or migrated to finance this backstop. For the existing pilot, no new liquidity is to be locked.

## Safe settlement sequence

1. The client verifies the contract code pin, both nodes' canonical state, final validation, proof, author and epoch, key commitment, eligible bid or free publication budget, current liabilities, and absence of conflicting private orders.
2. The contract locks one exact public-release payout against the proved offer. A short, bounded reservation prevents an unbounded budget lock. Bid acceptance uses the already escrowed bid. The reservation cannot be repriced or preempted by queue changes.
3. The client checks the original secret bytes against the proved commitment locally, then the author submits a separate reveal. The contract checks the secrets, publishes them, and pays from the locked amount. If the reveal is not completed within the reservation window, the bid is refunded or the publication budget released according to the predeclared rules.

A public transaction's calldata may be seen before inclusion. Even a reserved payout cannot eliminate disclosure risk from an invalid, censored, or late reveal transaction. The UI must show that risk before sending any keys and reconcile uncertain transactions by hash and state before retrying. A one-call reveal-and-sale is not an acceptable default because a changed quote or queue could cause failure after the keys have reached the mempool.

## UI and price language

The seller's first view should show qualified local bundles, proof status, the highest **funded private bid**, the highest **funded public-release bid**, and any **firm pool payout**, each with net author proceeds, bond/fee, timing, and whether keys become public. The buyer's first view should offer **Buy privately** and **Fund public release**, with exact escrow amount, eligible bundle or criteria, expiry, and refund rule. Public bundles may have a **Support author** action, but should not be presented as exclusive content for sale.

Separate a contract-calculated quote from genuine demand: a curve quote is not an external market-clearing price. Show the highest funded bid and a dated history of settled trades when available; exclude known common-operator pilot trades from any market-price summary. Estimates remain conditional until the transaction is funded and settled.

## Acceptance before any live v8 use

- Freeze the publication-pool funding source, reserve floor, payout rule and caps; private/public matching rules; bond and timeout rules; and whether any platform fee exists. Do not silently import v7's seed, liabilities, or proof exception.
- Build a separately pinned v8 contract, proof journal/image, typed CLI, and local UI; test conservation across every bid, reservation, reveal, refund, cancellation, expiry, and failed transfer path.
- Independently review contract security and proof binding. Pilot with bounded amounts and independently funded buyers before calling a displayed quote a market price. Deployment, funding, and signing remain separate decisions.

The public-goods funding problem here is real: once keys are released, use does not consume them and the contract cannot force every beneficiary to pay. Contingent pledges can coordinate willing sponsors; they do not manufacture demand. See [Samuelson's original public-goods model](https://courses.cit.cornell.edu/econ335/out/samuelson_pure.pdf), [Tabarrok's assurance-contract model](https://mason.gmu.edu/~atabarro/PrivateProvision.pdf), and [Idena's flip-encryption description](https://docs.idena.io/docs/wp/technology#flips-encryption).
