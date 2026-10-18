**Experimental software, without guarantees.** I am testing this out of curiosity while I consider Idena experimental and largely inactive; I currently have no profit intent. I may take this repository offline if the chain becomes mainstream. Published copies may remain available under the [MIT License](LICENSE). Do not use funds or keys you cannot afford to lose.

# Flips on Demand

**Reuse Idena picture puzzles as on-demand challenges for apps and services.**

A *flip* shows the same four pictures in two orders. You choose the order that tells a sensible story. Idena uses flips in its validation sessions; this project explores reusing past puzzles.

## What would you use it for?

An app could ask visitors to solve a few puzzles before unlocking a feature or receiving free test coins. The app operator buys the keys to unlock a collection of existing flips, prepares a session, and decides what passing it unlocks. Visitors solve the puzzles; they do not buy them.

![An author offers puzzles, an app operator buys access, a visitor solves them, and the app decides whether to grant access](docs/visuals/use-case.svg)

**Here are pictures from two real flips.** Each preview shows two of the four pictures, not a complete puzzle.

| IdenaAI 2 — “Avalanche / punch” | IdenaAI 3 — “Balloon / smoke” |
| --- | --- |
| <img src="docs/visuals/idenaai2-public-1.jpg" width="240" alt="A man in a red jacket breaks a cabin window" /> <img src="docs/visuals/idenaai2-public-2.jpg" width="240" alt="The man rests inside the cabin with snow outside" /> | <img src="docs/visuals/idenaai3-public-1.jpg" width="240" alt="A boy beside a smoking barbecue sees his balloon burst" /> <img src="docs/visuals/idenaai3-public-2.jpg" width="240" alt="The boy holds an intact red balloon beside the barbecue" /> |
| [Public flip on Idena explorer](https://scan.idena.io/flip/bafybeicpa2wypblq7ri4522azsjb6y3676nyq4y7se7x4ay7ppy6fcnvj4) | [Public flip on Idena explorer](https://scan.idena.io/flip/bafybeihodzhdrfo47w64d5gad3t5em3tzw5556ya43omq24eajtqigyhtq) |

This use case is still being built. It cannot guarantee one account per person, and AI may solve the puzzles too.

## What does it look like?

The current browser preview shows collections, prices, and trade status. **These are made-up test amounts in a read-only preview**, not a working checkout.

![Current browser preview showing a collection of flips, its sample price, and the waiting list](docs/visuals/viewer-v6-market.png)

[Mobile view and orders screen →](docs/VISUAL_GUIDE.md#current-ui)

## How does a trade work?

The author lists a collection. The buyer pays into **escrow**: a contract holds the money while the seller sends the unlocking keys. Confirmation, public proof or a refund then closes the trade.

![Listing, escrow payment and private delivery lead to confirmation, public proof or refund](docs/visuals/trade-overview.svg)

## What does it cost?

The buyer locks the price **plus a deposit**; the seller also deposits money. Deposits normally return after success, but may cover compensation or proof costs in a dispute. Blockchain fees are extra.

The current price cap is **1,000 iDNA**, Idena's currency, before deposits and fees. Buying keys does not mean exclusive access: they can be copied or sold again.

[Worked cost example and exact dispute rules →](docs/VISUAL_GUIDE.md#costs-and-money-movements)

## How far along is it?

One owner-run pilot tested a paid purchase, buyer challenge and public key release. The complete service is unfinished and has not been independently audited. The screenshots demonstrate the interface, not readiness for general use.

## Technical details

<details>
<summary>Interfaces, contract versions, setup and command reference</summary>

This repository explores sales of access to the original decryption keys of historical Idena flips. It contains several contract prototypes. **Direct escrow v7** is the one-seed contract described below: one permanently public seed bundle anchors a queue of private first sales and resales. One owner-operated, funded two-identity pilot completed a private purchase, challenge, and public seller fallback. That does not establish an independent audit, reliable delivery, or general readiness.

The [v7 contract guide](contracts/direct-escrow-v7/README.md) explains settlement and risks. The [v7 manifest](release/DIRECT_V7_MANIFEST.json) identifies the public candidate WASM and its expected code hash; its deployment field is *not* a directory of trusted live contracts. Independently establish the address and deployed code of any contract you intend to call.

### Choose an interface

| Path | What it supports |
| --- | --- |
| **IdenaAI → Flip market** | The [development source branch](https://github.com/ubiubi18/IdenaAI/tree/vibe/fod-social-bridge) adds **Direct v7** beside the older v4 page. Build that source and a local `fod` CLI; connect a synced node through loopback RPC and pin an independently verified contract, code hash, CLI path, and CLI SHA-256 in the page. It can prepare, review, and submit typed v7 actions and save or verify private delivery files. It does not send ciphertext to another person. Published desktop binaries do not include this source-only page. |
| **Own node → `fod` CLI** | The typed v7 path for qualifying and proving bundles, publishing the seed, activating, listing, private buying, local encrypted delivery and receipt verification, cooperative confirmation, challenging, and public fallback. It checks the selected contract, code hash, and current state before preparing or signing. |
| **Own node → JSON-RPC** | Read v7 state and make contract calls manually if you do not use the CLI. You must check method arguments, attached value, fees, account, and changed chain state yourself. |

The bundled browser page in `app/` is an experimental viewer with synthetic sample state; importing a snapshot does not verify chain state or sign a transaction. Do not mix contract versions, addresses, or prepared actions.

### Before using v7

You need a fully synced Idena node you control, its **local** RPC endpoint and API key file, and the identity that will pay for the call. A seller also needs the original flip inventory, canonical session evidence, a sealed content proof with its verifier host, and an encrypted epoch-key vault. These are private working files, not repository fixtures. A private buyer needs a dedicated delivery key and a way to receive and verify the seller's ciphertext. The contract does not deliver that ciphertext.

Keep RPC and node keys in owner-only files. Use a loopback RPC URL, or a local tunnel to your own node; never expose its key over plain remote HTTP. The examples use `curl`, `jq`, Python 3, and Go (see [`.tool-versions`](.tool-versions)). Replace every placeholder with values checked against your own node. The CLI uses **integer iDNA base units** (`10^18` units per iDNA) for amounts and fees; node JSON-RPC uses **decimal iDNA** for `amount` and `maxFee`.

#### Build the CLI

The public CLI module depends on Idena node source. This local build recipe uses a public node-source revision and leaves tracked module files untouched:

```bash
git clone https://github.com/ubiubi18/flips-on-demand.git
git clone https://github.com/ubiubi18/idena-go.git
git -C idena-go checkout bffafb9cbf94f5f776af341748816e8da86a7e95
NODE_SOURCE="$(pwd)/idena-go"
cd flips-on-demand/tools/fod
cp go.mod go.local.mod
cp go.sum go.local.sum
go mod edit -modfile=go.local.mod "-replace=github.com/idena-network/idena-go=$NODE_SOURCE"
go mod tidy -modfile=go.local.mod
go test -modfile=go.local.mod -run 'TestDeriveEpochKeysMatchesPinnedNode|TestHistoricalNodeBlobOpensWithRecordedKeys' .
go build -modfile=go.local.mod -o fod .
```

Compare the candidate WASM's SHA-256 and deployed code hash with the manifest before trusting a deployment. A matching manifest alone does not prove that a contract is safe.

The test in the build recipe rejects a node-source revision whose epoch-key mapping differs from the CLI. The frozen blob fixture was built with an older mapping and checks opening with its recorded keys; it is not a derivation vector for the pinned node source. Before qualifying a new bundle, verify every intended original flip opens with the derived keys.

#### Read your node and contract

Idena RPC requests put the API key in the JSON body's top-level `key` field. This helper reads it from a file without placing it in the command line. See the [Idena node RPC reference](https://rpc.idena.io/) for the node API:

```bash
RPC_URL='http://127.0.0.1:9019'
RPC_KEY_FILE='/absolute/path/to/your/rpc-key-file'
CONTRACT='your-independently-verified-40-hex-contract-address'

rpc() {
  jq -n --rawfile api_key "$RPC_KEY_FILE" --arg method "$1" --argjson params "$2" \
    '{key:($api_key | rtrimstr("\n")), method:$method, params:$params, id:1}' |
    curl -fsS -H 'Content-Type: application/json' --data-binary @- "$RPC_URL" | jq .
}

rpc bcn_syncing '[]'
rpc dna_epoch '[]'
rpc dna_getCoinbaseAddr '[]'
rpc bcn_feePerGas '[]'
CALLER="$(rpc dna_getCoinbaseAddr '[]' | jq -er '.result')"
rpc dna_getBalance "$(jq -nc --arg a "$CALLER" '[$a]')"
rpc contract_getStake "$(jq -nc --arg c "0x$CONTRACT" '[$c]')"
rpc contract_readData "$(jq -nc --arg c "0x$CONTRACT" '[$c,"pool/seed-id","hex"]')"
rpc contract_readData "$(jq -nc --arg c "0x$CONTRACT" '[$c,"pool/primary-head","hex"]')"
rpc contract_readData "$(jq -nc --arg c "0x$CONTRACT" '[$c,"pool/secondary-head","hex"]')"
rpc contract_iterateMap "$(jq -nc --arg c "0x$CONTRACT" '[$c,"pool/offer/",null,"hex","hex",200]')"
```

Compare `contract_getStake.result.Hash` with the manifest's `artifact.codeHash` **only if this is the intended candidate deployment**. A missing hash, unexpected seed, wrong active identity, syncing node, or stale queue is a stop condition. The primary head has priority; the secondary head is selectable only when the primary queue is empty. Continue `contract_iterateMap` with its returned continuation token until exhausted; its offer values are raw 222-byte records. `contract_readData` also exposes `pool/reserve`, `pool/insurance`, `pool/primary-units`, `pool/secondary-units`, `pool/order/<decimal order ID>`, and, after public fallback, `pool/reveal/<decimal order ID>`. These are raw contract values, not a verified market summary.

#### If you deploy your own instance

An existing v7 address is unnecessary if you deploy the candidate yourself. The constructor pins **one** seed bundle ID; you must be able to qualify, prove, and publicly disclose that real bundle before activation. From `tools/fod`, `prepare-deploy` signs a **zero-value** transaction into a private file but never broadcasts it. Supply a live base-unit fee rate, sufficient maximum fee for the large WASM deployment, your next nonce, and current epoch:

```bash
SEED_ID='your-verified-64-hex-seed-offer-id'
FEE_PER_GAS_BASE='your-current-base-unit-rate'
MAX_FEE_BASE='your-reviewed-base-unit-fee-ceiling'
NEXT_NONCE='your-next-account-nonce'
TX_EPOCH='your-current-chain-epoch'
NODEKEY_FILE='/absolute/path/to/your/nodekey-file'
DEPLOY_SIGNED='/absolute/path/to/private/deploy-signed.json'
./fod prepare-deploy -code ../../release/artifacts/fod_direct_escrow_v7.wasm \
  -v7-seed-id "$SEED_ID" -network idena -nodekey "$NODEKEY_FILE" \
  -nonce "$NEXT_NONCE" -tx-epoch "$TX_EPOCH" \
  -fee-per-gas "$FEE_PER_GAS_BASE" -max-fee "$MAX_FEE_BASE" \
  -signed "$DEPLOY_SIGNED"
```

Review the recorded sender, derived address, seed ID, code hash, zero amount, fee, and signed bytes. After a fresh nonce, balance, sync, and fee check, send `rawTx` with your own node's RPC:

```bash
RAW_TX="$(jq -er '.rawTx' "$DEPLOY_SIGNED")"
rpc bcn_sendRawTx "$(jq -nc --arg tx "0x$RAW_TX" '[$tx]')"
```

Verify the transaction receipt and the derived address's deployed hash and pinned seed. Set `CONTRACT` to that verified derived address before later calls. Do not send another deployment if the first response is uncertain. The [contract guide](contracts/direct-escrow-v7/README.md) describes the required seed proof, publication, and irreversible 10,000 iDNA activation deposit.

### Use the typed v7 CLI

From `tools/fod`, set local paths and independently checked values. `CODE_HASH` is the deployed code hash **without** `0x`, `MAX_FEE_BASE` is a positive base-unit ceiling informed by a fresh fee and gas estimate, and `OFFER_ID` is a 32-byte lowercase hex author-and-epoch bundle ID:

```bash
CODE_HASH='your-verified-64-hex-code-hash'
MAX_FEE_BASE='your-reviewed-base-unit-fee-ceiling'
OFFER_ID='your-verified-64-hex-offer-id'
SESSION_FILE='/absolute/path/to/your/canonical-session.json'
NODEKEY_FILE='/absolute/path/to/your/nodekey-file'
PREPARED='/absolute/path/to/private/prepared.json'
SIGNED='/absolute/path/to/private/signed.json'

./fod direct-v7-prepare -direct-version 7 -network idena \
  -rpc "$RPC_URL" -key-file "$RPC_KEY_FILE" \
  -contract "$CONTRACT" -code-hash "$CODE_HASH" -max-fee "$MAX_FEE_BASE" \
  -operation buy -v7-offer-id "$OFFER_ID" -session "$SESSION_FILE" \
  -delivery-key '/absolute/path/to/private/new-delivery-key' -prepared "$PREPARED"
```

`direct-v7-prepare` reads the live node, checks the oldest offer and canonical session, retains every original encrypted flip from the buyer's node, creates a separate delivery key, estimates the call, and writes a private unsigned action. It does **not** buy anything. Inspect the prepared file and recheck the quote plus required buyer bond, active identity, code hash, queue head, balance, and fee budget. Private `buy` locks **quote plus buyer bond**; it cannot buy a public seed.

To send, provide your nodekey, the next nonce (`dna_getBalance.nonce + 1` when its `mempoolNonce` equals `nonce`) and current `dna_epoch.epoch`, and the same pins. `submit` rechecks the live state before signing and sending. Recheck the node and all terms immediately before this call; omit `-broadcast` if you only want to write a signed transaction:

```bash
NEXT_NONCE='your-next-account-nonce'
TX_EPOCH='your-current-chain-epoch'
./fod submit -prepared "$PREPARED" -nodekey "$NODEKEY_FILE" \
  -nonce "$NEXT_NONCE" -tx-epoch "$TX_EPOCH" -max-fee "$MAX_FEE_BASE" \
  -contract "$CONTRACT" -code-hash "$CODE_HASH" \
  -rpc "$RPC_URL" -key-file "$RPC_KEY_FILE" -signed "$SIGNED" -broadcast
```

The typed CLI also accepts these `-operation` values for `direct-v7-prepare`:

| Operation | Who and required extra input | Attached value |
| --- | --- | --- |
| `qualify` | Bundle author: `-session`, `-proof`, `-registration-host` | Zero |
| `proveBundle` | Anyone: same proof inputs; proves original content before sale, or with a v7-bound proof before a funded order's deadline | Zero |
| `seedBundle` | Seed author: `-vault`; **publishes the seed's keys** | Zero |
| `activate` | Any funder: `-v7-offer-id`, after the one seed is proved and published | Exactly 10,000 iDNA, permanently locked without a redeemable share |
| `list` | Qualified author: `-session`, `-vault`; optional `-minimum-payout` in base units | Current seller delivery bond, plus 1,000 iDNA fraud collateral if content is unproved |
| `buy` | Private buyer: `-v7-offer-id`, canonical `-session`, `-delivery-key` output path | Current quote plus required buyer bond |
| `buyPublicSeed` | Buyer: expected oldest public `-v7-offer-id`; settles immediately with no private order | Exact current quote |
| `confirm` | Funded buyer: `-order`, `-delivery` signed receipt from `direct-v7-receive` | Zero |
| `challenge` | Funded buyer: `-order` | Zero |
| `startFallback` | Seller: `-order`. A pre-proven offer uses the short path. An unproved offer also needs `-session`, `-proof`, and `-registration-host` for a sealed proof bound to this v7 contract | Zero |
| `proveAndReveal` | Seller: `-order`, `-vault`; **publishes the private bundle's keys** | Zero |
| `refund` | Anyone: `-order`, only after the proof deadline on a still funded order; retires the offer | Zero |
| `relist` | Author: sold or cancelled private `-v7-offer-id`; optional `-minimum-payout` | Current seller bond, plus fraud collateral if still unproved |
| `cancel` | Author: queued private `-v7-offer-id`; an unproved listing forfeits fraud collateral | **Zero** |
| `expireHead` | Anyone: expected oldest `-v7-offer-id`, after 30,240 selectable blocks; a public seed rotates | Zero |
| `topUpBond` | Author: queued private `-v7-offer-id` and explicit positive `-amount` in base units | The specified top-up |
| `slashUnproved` | Anyone: settled unproved `-order` after its content-proof deadline; retires the offer | Zero |

Prepare and submit each step separately after the preceding transaction is included and its state is checked. Do not seed or activate a new deployment with sample data. For an unproved `startFallback`, the CLI checks the pinned proof host, v7 contract binding, original CID inventory, key commitment, and canonical session roots before preparing the call; submission checks the live order and proof statement again. The contract verifies the seal and session at execution. `proveBundle` verifies original content; it does not prove that a buyer received a private message. A legacy v6-bound proof is allowed only for an unfunded qualified epoch-230 offer; proving after funding requires a proof bound to the live v7 contract. The fallback deadline is 12,960 blocks from funding; a failed or absent fallback may allow `refund` once that window ends. See the [contract guide](contracts/direct-escrow-v7/README.md) for payouts, bonds, queue order, and deadlines.

#### Verify private delivery before confirming

After the private `buy` is included, the seller can produce one encrypted JSON message from the funded order and their committed key vault. This command writes a local file; it does not send a message:

```bash
ORDER_ID='your-funded-private-order-id'
SELLER_VAULT='/absolute/path/to/private/seller-vault.age'
CIPHERTEXT_FILE='/absolute/path/to/private/new-ciphertext.json'
./fod direct-v7-dm -direct-version 7 -network idena \
  -rpc "$RPC_URL" -key-file "$RPC_KEY_FILE" \
  -contract "$CONTRACT" -code-hash "$CODE_HASH" -order "$ORDER_ID" \
  -vault "$SELLER_VAULT" -out "$CIPHERTEXT_FILE"
```

Transfer that file privately to the buyer using a channel you trust. The buyer uses their own synced node, the retained `buy` file, and its dedicated delivery key to verify the funded order, canonical session, original CIDs, key commitment, and decrypted flip parts. A successful check saves the keys in a new encrypted vault and signs a receipt with the delivery key:

```bash
DELIVERY_KEY_FILE='/absolute/path/to/private/delivery-key'
BUYER_VAULT='/absolute/path/to/private/new-buyer-vault.age'
RECEIPT_FILE='/absolute/path/to/private/new-receipt.json'
CONFIRM_PREPARED='/absolute/path/to/private/new-confirm-prepared.json'
./fod direct-v7-receive -direct-version 7 -network idena \
  -rpc "$RPC_URL" -key-file "$RPC_KEY_FILE" \
  -contract "$CONTRACT" -code-hash "$CODE_HASH" -order "$ORDER_ID" \
  -message "$CIPHERTEXT_FILE" -purchase "$PREPARED" \
  -delivery-key "$DELIVERY_KEY_FILE" -vault "$BUYER_VAULT" -out "$RECEIPT_FILE"

./fod direct-v7-prepare -direct-version 7 -network idena \
  -rpc "$RPC_URL" -key-file "$RPC_KEY_FILE" \
  -contract "$CONTRACT" -code-hash "$CODE_HASH" -max-fee "$MAX_FEE_BASE" \
  -operation confirm -order "$ORDER_ID" -delivery "$RECEIPT_FILE" \
  -prepared "$CONFIRM_PREPARED"
```

Inspect and submit `CONFIRM_PREPARED` with the buyer's nodekey using the `submit` pattern above. Confirmation settles the order and commits the SHA-256 of the exact verified ciphertext on-chain. The CLI checks the receipt signature and live funded order again before signing. Delivery and receipt outputs are create-only; choose new paths for a later attempt. Keep the message, purchase file, delivery key, receipt, and vault private. A ciphertext file or a seller's claim alone is not evidence of a usable delivery.

### Make a manual node RPC call

You can call the same methods without the CLI through your own node's `contract_estimateCall` and then `contract_call`, supplying the exact wire argument and attached value. **The node signs `contract_call` with its active identity.** Read the corresponding export in [`contracts/direct-escrow-v7/src/lib.rs`](contracts/direct-escrow-v7/src/lib.rs) before sending; a successful estimate is not an inclusion guarantee.

For example, `buyPublicSeed` takes the expected oldest public seed ID as one 32-byte `hex` argument and requires the **exact** current quote, with no buyer bond. Read the four pricing fields in `bigint` format, then calculate the quote using integer arithmetic; never round it through floating point. The pricing reserve is `reserve - insurance`, and the denominator is `primary-units + secondary-units + 9`. Cap the rounded-up result at 1,000 iDNA. This purchase settles immediately and creates **no disputable order**:

```bash
read_bigint() {
  rpc contract_readData "$(jq -nc --arg c "0x$CONTRACT" --arg k "$1" '[$c,$k,"bigint"]')" | jq -er '.result'
}

PRIMARY_UNITS="$(read_bigint pool/primary-units)"
SECONDARY_UNITS="$(read_bigint pool/secondary-units)"
RESERVE_BASE="$(read_bigint pool/reserve)"
INSURANCE_BASE="$(read_bigint pool/insurance)"
QUOTE_IDNA="$(python3 - "$PRIMARY_UNITS" "$SECONDARY_UNITS" "$RESERVE_BASE" "$INSURANCE_BASE" <<'PY'
import sys
primary, secondary, reserve, insurance = map(int, sys.argv[1:])
units, pricing_reserve = primary + secondary, reserve - insurance
if units < 1 or pricing_reserve < 1:
    raise SystemExit('no valid quote')
base = 10**18
quote = min(1000 * base, (pricing_reserve + units + 8) // (units + 9))
print(f'{quote // base}.{quote % base:018d}'.rstrip('0').rstrip('.'))
PY
)"

# Re-read both queue heads. Use the currently selectable public seed, without 0x.
OFFER_ID='your-verified-64-hex-public-seed-id'
CALLER="$(rpc dna_getCoinbaseAddr '[]' | jq -er '.result' | sed 's/^0x//')"
MAX_FEE_IDNA='your-reviewed-decimal-idna-fee-ceiling'
CALL="$(jq -nc --arg from "0x$CALLER" --arg contract "0x$CONTRACT" \
  --arg amount "$QUOTE_IDNA" --arg maxFee "$MAX_FEE_IDNA" --arg value "0x$OFFER_ID" \
  '{from:$from,contract:$contract,method:"buyPublicSeed",amount:$amount,maxFee:$maxFee,
    args:[{index:0,format:"hex",value:$value}]}')"
rpc contract_estimateCall "[$CALL]"
# After checking the live code, identity, head, exact quote, balance, fee, and estimate:
# rpc contract_call "[$CALL]"
```

For other calls, follow the same JSON-RPC shape but **do not reuse the public-seed amount or argument**. Each v7 export takes one `hex` argument at index 0. Integer fields are unsigned and big-endian; the lengths below are bytes.

| Argument layout | Methods |
| --- | --- |
| Offer ID (32) | `activate`, `buyPublicSeed`, `cancel`, `expireHead`, `topUpBond` |
| Order ID (8) | `challenge`, `refund`, `slashUnproved`, and `startFallback` for a pre-proven offer |
| Offer ID (32) + uncompressed delivery public key (65) | `buy` |
| Order ID (8) + SHA-256 of the exact verified ciphertext (32) | `confirm` |
| Offer ID (32) + minimum payout (16) | `relist` |
| Offer ID (32) + canonical epoch secrets (106) | `seedBundle` |
| Order ID (8) + canonical epoch secrets (106) | `proveAndReveal` |

`qualify`, `proveBundle`, `list`, and an unproved `startFallback` carry additional session, proof, or commitment fields. Use the typed CLI to construct and validate those calls, or follow their exact encoders in [`tools/fod/direct_v7_pool_calls.go`](tools/fod/direct_v7_pool_calls.go) and the contract exports. Check the attached value in the operation table above. Nonpayable methods such as `cancel` require **zero attached value**; accidental value can be stranded. Simulate the exact call before sending it.

After any broadcast, retain the returned transaction hash and query its receipt and contract state from your own node:

```bash
TX_HASH='your-returned-transaction-hash'
rpc bcn_txReceipt "$(jq -nc --arg h "$TX_HASH" '[$h]')"
```

If the response is missing or uncertain, reconcile the hash, nonce, mempool, receipt, and changed state before trying again; a timeout does not mean the transaction failed. Public fallback or seed publication exposes keys permanently. The contract cannot guarantee content availability, private-message delivery, or recovery of lost local files.

</details>

## License

[MIT](LICENSE). Third-party source and license notes are in [`THIRD_PARTY.md`](THIRD_PARTY.md).
