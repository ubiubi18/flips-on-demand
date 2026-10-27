# Optimistic bonded market v9

For a practical seller walkthrough, including package preparation and encrypted key delivery, see [Sell a validated flip set](SELLER_GUIDE_V9.md).

25 October 2026. **Deployed and verified on all three native nodes.** The two original five-flip offers were purchased and settled. Three other buy orders remained stored as queued, but a later read on 25 October found that all three had passed their block expiry and were no longer matchable. These were common-operator pilot trades, not evidence of independent demand. See the [settlement evidence](evidence/v9-two-purchases-completed.json) and [purchased-preview verification](evidence/v9-native-purchased-previews.json). The earlier [deployment and queue evidence](evidence/v9-deployment-and-queue-preparation.json) and candidate manifest preserve their historical setup snapshot.

## Normal trade

**Starting price:** one complete bundle costs exactly **500 iDNA** to buy when the market has zero funded reserve and zero acquired inventory. The existing constant-product curve sets subsequent quotes. Its initial seller quote is **409.090909090909090909 iDNA**, retaining the existing buy/sell spread. This price is per author/epoch package, not per individual flip; network fees are additional.

The v9 virtual reserve is 4,500 iDNA, with 1,000 virtual units and 100 units per package. Added acquired inventory lowers quotes; settled purchases raise them. Merely joining or cancelling an unfilled queue does not move the curve. Funded reserve also affects prices. Virtual reserve sets prices and cannot pay for a purchase. Existing v8 parameters remain unchanged.

1. Seller imports the canonical validation session and unlocks the existing encrypted epoch-key vault. The client checks every original encrypted flip against its CID, author, keys and decoded content.
2. Seller deposits reusable collateral. `queueSell` records the package commitments, verifies the canonical inventory and clean validation verdict, reserves collateral and joins the queue in one transaction. There is no `registerBundle` operation and no upfront content seal.
3. Buyer queues an order with escrow and separate collateral. Matching preserves the existing curve and earliest-compatible-order policy. Pool acquisition is possible only when actual free reserve is available.
4. Buyer retains and checks every original encrypted CID before declaring readiness. Seller delivers the epoch keys encrypted to the assigned buyer and keeps the original delivery witness privately.
5. Buyer decrypts the original delivered ciphertext, checks the key commitment, opens every original flip, verifies its author/content, and saves an encrypted vault before accepting. Acceptance settles the trade without a cryptographic content or delivery proof.

The on-chain package commitment and canonical state membership checks are still necessary: they identify what was sold and bind later disputes to that exact package. They do not require an upfront zero-knowledge content proof. A buyer may explicitly consent to acceptance on chain; automatic acceptance in both supported clients requires the complete local content check.

## Dispute or missing acceptance

Only a challenged delivery or a delivery whose acceptance period has elapsed can use `proveDelivery`. Settlement requires **both** proofs in the same transaction:

- The pinned original-content proof establishes valid decryption and content for the listed author, epoch, original CID inventory and key commitment under this contract.
- The pinned delivery proof establishes encryption of those same keys to the assigned buyer, for the original trade, original ciphertext hash and canonical funding anchor.

A delivery proof alone cannot establish that the supplied keys open valid original flips. Removing the upfront registration therefore requires adding the original-content check to disputed settlement.

The original ciphertext cannot be corrected after delivery. Altered keys, author, contract, inventory, trade, buyer or funding anchor cannot replace the agreed values. Successful settlement is single-use. Without acceptance or successful proof before expiry, the buyer is refunded and the seller is charged under the existing timeout rules. A buyer who has not completed readiness does not create a seller delivery obligation.

Existing v8 deposit rules, reservation accounting, refill restrictions, full exit, curve and queues are retained. The proof period remains 12,960 blocks after the acceptance deadline. Proof production is a separate local operation; neither UI starts a prover automatically.

**Cost limitation:** the inherited proof reimbursement is capped at the smaller of `feePerGas × 2,500,000` and 5% of the trade price. An unsuccessful explicit buyer challenge adds a further 5% charge. The combined v9 proof can cost substantially more than this reimbursement, especially for low-priced packages. A deposit enables adjudication and penalties; it does not guarantee economical disputes or full reimbursement of network fees. Actual production proving time and a fresh native fee estimate must be assessed before offering packages.

## Versioned client support

- CLI: `fod v9`; standalone browser app: `fod app-v9`.
- Native IdenaAI: separate `social.v9` service and connection profile; the market page opens v9 and retains access to v8 and v7.
- New prepared-action formats, `v9/` state keys and `fod:bundle:v9` bundle identities prevent accidental reuse of v8 records. Both preparation and signing enforce the selected version.
- v9 seller signing reopens the local vault and rechecks the original package; the vault passphrase is not saved in prepared transactions.
- Both interfaces import the two completed dispute proofs together. Existing permission, identity/code pins, nonce protection, budget checks and review controls remain enforced. Deposit and dispute transactions retain their own review.
- Existing canonical captures and pre-session witnesses are read and preserved. v9 never needs to overwrite a witness made for an earlier contract.

### Standalone preview

From the repository root, using the locally built executable:

```sh
./build/fod-standalone-v9 app-v9 -read-only-preview
```

The preview has synthetic examples and no transaction endpoints. For a real connection, use the setup in [Standalone market](STANDALONE_V8.md), substituting `app-v9`, an independent v9 profile/data directory, and the verified v9 deployment/code pins. Use the verified live deployment pins retained in the owner-only rollout record.

### Producing evidence after a dispute

The original delivery command saves a new private delivery witness alongside its output, before a transaction is prepared. Keep that exact witness: a replacement encryption would not prove the original delivery.

For original content, `fod v9 -command contentWitness` accepts an owner-only request with `protocolVersion: 9`, the assigned seller in `caller`, the trade `id`, and the existing `vaultFile`. Use the normal pinned loopback RPC/contract/code options and a **new** `-out` file. It checks the trade, listed keys, author, epoch and CID inventory, then exclusively creates an owner-only content-prover input. It refuses to replace an existing file. Supply the vault passphrase through `FOD_PASSPHRASE` using the existing secure local mechanism.

The existing `fast-content-prepare` and `fast-content-host` tools accept this new input. Preparation makes a new private directory; the host uses `--input`, `--prepared`, `--contract`, `--out` and `--prove`. The original delivery witness is consumed by `delivery-host --order … --no-witness --out …`. Use genuine proving mode, verify the completed receipts, and import both public proof artifacts for transaction review. The stored original pre-session witness remains unchanged, even when it names a previous contract.

## ABI differences from v8

All integers are big-endian. Monetary fields use base units.

| Operation | v9 payload |
| --- | --- |
| `queueSell` | minimum (u128), capacity (u128), expiry (u64), public claim journal (273 bytes), canonical session wire length (u16), session wire |
| `proveDelivery` | trade ID (u64), content journal (273 bytes), content seal (256 bytes), delivery journal (281 bytes), delivery seal (256 bytes) |
| `registerBundle` | Removed |
| Empty-argument operations | Version marker `9` |

The public claim uses the existing content journal layout; the unused encrypted-copy fields carry no listing authority. The contract stores the original inventory and key commitment. Other operation layouts are inherited unchanged.

## Evidence and delivery boundary

The [v9 candidate manifest](../release/BONDED_V9_CANDIDATE_MANIFEST.json) records exact artifact/source hashes, focused test results and measured native gas. Native WASM tests execute the optimized candidate through the Idena engine, including genuine synthetic receipts. They do not establish a funded production trade or a production prover performance benchmark.

The separate v9 deployment and requested queue setup were authorized and completed on 24 October. The deployment, all ten setup-call receipts, canonical blocks, roots and setup state were independently verified on all three nodes. At that setup snapshot, IdenaAI 1 had five active buy orders, 2,750 iDNA in purchase escrow and 2,750 iDNA in separate buyer collateral, with a maximum 550 iDNA per set. IdenaAI 2 and IdenaAI 3 each had one active epoch-233 five-flip offer and 750 iDNA in funded, reserved seller collateral. The user approved a 50-iDNA total buyer setup fee ceiling; actual buyer fees were 43.3125774218051291 iDNA. The earlier 2,200-iDNA funding transfer was sufficient; no additional transfer was made. Both original captures and witness hashes were reverified. No matching, delivery or settlement had yet been executed at that setup snapshot. Two purchases settled on 25 October; the three remaining buy orders subsequently expired without matching. The v9 native app UI was installed and verified on all three identities. Exact confirmed heights and fees are in the live rollout evidence.
