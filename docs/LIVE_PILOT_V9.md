# Live Flips on Demand pilot

Flips on Demand is an experimental market where an Idena author can sell the keys to a validated set of their own flips. A buyer pays through the v9 contract, receives keys encrypted for that buyer, and opens the original flips locally.

**This is the real network, with real funds and no example offers or prices.** As checked on 27 October 2026, the shared HTTPS relay serves live chain reads and forwards only locally signed calls to the pinned v9 contract. IdenaAI 3 is enrolled for epoch 234 capture; its encrypted flip files are being retained, but a final sale package cannot be ready until validation finishes. Both native nodes reported the next validation for 29 October 2026 at 15:00 UTC. The live market can therefore be empty.

| Connection item | Pilot value |
| --- | --- |
| Relay API | `https://62.238.124.168/rpc` |
| v9 contract | `0xc114db894aa8Bb112CbbFd86FE0788067Fc88702` |
| Deployed code hash | `4c151e3e6f2c4a54597783b7ea2cae7b0c0078ba99ff30eeb03ba5be8a764319` |
| Public contract record | [Idena Scan](https://scan.idena.io/contract/0xc114db894aa8Bb112CbbFd86FE0788067Fc88702) |

The relay is an API, **not a website or wallet**. The desktop client opens its own local browser window and keeps signing keys on the user's device. The node RPC API key stays on the operator's server. The shared relay serves captured flip files from verified enrolled archives, or flip files tied to an active trade on the pinned contract. Never send a wallet key, identity key, backup, vault password, or node API key by idena.social message.

## Buy without IdenaAI or a node

For this technical pilot, [build `fod` from source](../README.md#build-the-cli); a packaged installer is not available yet. A buyer needs a funded Idena wallet but does not need a validated identity. From the built `tools/fod` directory:

```bash
install -d -m 700 "$HOME/.local/share/fod-buyer"
./fod buyer-wallet-create -out "$HOME/.local/share/fod-buyer/wallet.key"
./fod app-v9 -buyer-only \
  -relay https://62.238.124.168/rpc \
  -wallet-key "$HOME/.local/share/fod-buyer/wallet.key" \
  -contract 0xc114db894aa8Bb112CbbFd86FE0788067Fc88702 \
  -code-hash 4c151e3e6f2c4a54597783b7ea2cae7b0c0078ba99ff30eeb03ba5be8a764319 \
  -profile "$HOME/.local/share/fod-buyer/connection.json"
```

Create the wallet only once; the command refuses to overwrite its key file. Fund the printed public wallet address before placing an order. If you already have an owner-only wallet key file, use it instead and skip wallet creation. Enter the pairing code printed locally, review the live quote, payment escrow, network fee and reusable security deposit, then decide whether to join the buy queue. Keep the app open through any matching and delivery deadlines. [Buyer details](BUYER_ONLY_V9.md).

## Sell your own validated set

The operator must enroll your **public author address before your validation session** so the shared nodes can retain the original encrypted files and canonical session evidence. Only you keep the identity key and derive the opening keys locally. The shared service cannot recover a missed validation boundary, create keys for you, or list another author's flips. After validation, the seller client checks the capture and clean verdict before a deposit and listing can be submitted. [Shared-node seller steps](SHARED_NODE_PILOT_V9.md#what-the-seller-does) and [full seller guide](SELLER_GUIDE_V9.md).

## Short FAQ

**Do I need an API key or a personal node?** No. The relay URL and public contract pins are sufficient to connect; the operator's node key is never shared. A buyer needs a local wallet key. A seller needs their own local Idena identity key.

**Can I try this in a phone app?** The current live trading client runs on desktop. Felix's mobile identity backup can be imported offline on the seller's own desktop; phone-only selling is not implemented.

**What do I pay or lock?** The app shows the current curve quote. A buyer locks purchase escrow and separate reusable collateral; a seller locks collateral. Network fees are additional. An order may remain queued, match, be cancelled, or expire.

**Can I close the app after queuing?** The order stays on chain, but deadline-bound matching, delivery, checking and acceptance need the relevant local client running with bounded trading permission. The relay does not act as a custodian or autonomous trader for you.

**Where are purchased flips?** After verified delivery and acceptance, the original picture sequences appear under **My purchases** in the buyer's local client. Keep its key file, app data and saved-set password backed up.

**What if no set appears?** A pre-session capture is not a validated sale package. The final session evidence and contract eligibility check are required; a missing or adverse result is not silently listed. The market may also have no current matching offer.

This pilot is experimental and has not had an independent security audit. Start with an amount you are comfortable testing; inspect the contract and code pin yourself before signing. The [protocol guide](OPTIMISTIC_BONDED_MARKET_V9.md) describes settlement and disputes.
