# Buy flip keys without a personal Idena node

The v9 buyer client needs a funded Idena wallet, but the wallet does **not** need a validated identity. It runs on your own desktop computer, keeps your signing and delivery keys there, and connects over HTTPS to a restricted relay for market state, original encrypted flip files and broadcasting transactions. It does not require IdenaAI or a node on the buyer's computer.

This connection is usable only when a trusted v9 relay has been configured and independently checked. The [live pilot](LIVE_PILOT_V9.md) gives the current real-network relay and contract pins. The existing `app-v9 -read-only-preview` uses example data and cannot place orders.

## Five steps for a buyer

1. **Create and fund a wallet.** Create a new owner-only key file with `fod buyer-wallet-create -out FILE`, save that file securely, and send iDNA to the address printed by the tool. Existing owner-only Idena wallet key files can also be used. Never upload the key file to a website or relay.
2. **Open the buyer app.** Build `fod` using the [repository instructions](../README.md#build-the-cli). Start it with the HTTPS relay, wallet key and independently checked v9 contract/code pins shown below. Enter the pairing code printed on your computer.
3. **Set up the deposit.** The Market shows the live buy quote, offers and demand. Review and sign a reusable security deposit. The purchase payment and network fees are separate.
4. **Place a buy order.** Set the eight-hour local spending limits and a password for saved flip sets, then choose **Buy**. The order carries its maximum payment in escrow and reserves the same amount from your deposit. An unmatched order stays queued until filled, cancelled or expired.
5. **Keep the app running through delivery.** Renew the eight-hour permission if the order remains open. After matching, the app fetches and CID-checks the original encrypted flips, receives buyer-encrypted keys from the contract, opens every flip and saves a local encrypted vault before accepting. The completed set appears under **My purchases**. You can preview the images or export the encrypted package.

The relay cannot complete deadline-bound buyer actions while this app is closed or asleep. Keep the wallet key, buyer app data and saved-set password backed up securely; losing the private delivery key or local saved data can make a purchase unusable.

### First connection

Use the checked values from the [live pilot](LIVE_PILOT_V9.md), or a different relay and contract you have independently verified. The placeholders below are a template.

```bash
FOD='./fod'
WALLET_KEY_FILE='/absolute/path/to/private/buyer-wallet.key'
RELAY_URL='https://your-verified-relay.example/rpc'
CONTRACT='your-independently-verified-40-hex-v9-contract'
CODE_HASH='your-independently-verified-64-hex-v9-code-hash'
PROFILE='/absolute/path/to/private/buyer-market/connection.json'

"$FOD" buyer-wallet-create -out "$WALLET_KEY_FILE"
"$FOD" app-v9 -buyer-only -relay "$RELAY_URL" \
  -wallet-key "$WALLET_KEY_FILE" -contract "$CONTRACT" \
  -code-hash "$CODE_HASH" -profile "$PROFILE"
```

Create the wallet only once. If you already have the key file, skip that command; never overwrite it. The connection profile and app data must be kept owner-only and backed up with the wallet key. A later launch can use the saved profile.

## What `api.idena.io` can provide

The [official indexer API description](https://api.idena.io/api/swagger/doc.json) exposes contract metadata, blocks, transactions and decoded flip pictures through GET endpoints. It does not expose the v9 contract's current storage, a signed-transaction broadcast endpoint or the original CID-matched encrypted flip bytes. The buyer therefore needs a node-backed relay or another provider for those operations. The [Idena node RPC](https://docs.idena.io/docs/developer/node/node-rpc) defines the underlying methods; Idena's IPFS network is [private to its nodes](https://docs.idena.io/docs/developer/node/general).

The relay sees wallet addresses, requested CIDs, order details and signed transactions. It never receives the wallet key, private delivery key or saved-set password. The client checks the contract/code pins, transaction terms, original file CIDs and decrypted delivery before acceptance. A relay can still misreport current contract state, withhold data or delay broadcast; this client does not independently prove every contract storage value against a canonical header. Choose the relay and amounts accordingly.

## Relay operator setup

The relay runs beside a synced Idena node and holds that node's RPC API key. It binds only to loopback; an operator must provide HTTPS and external request limits at a reverse proxy. Keep the relay's node RPC and key file private, and verify the contract pin before starting it.

```bash
"$FOD" buyer-relay -rpc 'http://127.0.0.1:9019' \
  -key-file '/absolute/path/to/private/node-rpc-key' \
  -contract 'your-independently-verified-40-hex-v9-contract' \
  -code-hash 'your-independently-verified-64-hex-v9-code-hash' \
  -shared-archive-dir '/absolute/path/to/private/shared-archive' \
  -listen '127.0.0.1:9069'
```

The shared archive must already exist; follow the [shared-node setup](SHARED_NODE_PILOT_V9.md) to create and supervise it. The proxy should expose only `POST /rpc` and `GET /evidence/v1/` over HTTPS, enforce its own client rate and body limits, and forward to the loopback listener. The relay allows bounded market reads, verified archive flips, flips included in an active trade on the pinned contract, and already signed calls to that contract. It does not accept a caller-provided node key or expose the node's identity. Review the deployment and data handling before publishing a relay URL.

The seller path remains in the [v9 seller guide](SELLER_GUIDE_V9.md). An author still needs their own validated flip set and session evidence; buyer-only mode does not make a wallet eligible to sell.
