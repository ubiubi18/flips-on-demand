# Flips on Demand

**Research software. Use at your own risk. Do not use it with funds or keys you cannot afford to lose.**

This repository explores a market for access to the original decryption keys of historical Idena flips. It contains a local browser interface, a Go command-line tool, proof experiments, a provider gateway, and several *different* escrow contract prototypes. The liquidity-pool contract and dashboard are a development candidate. They are not a deployed or independently audited market.

The code is provided as-is under the [MIT License](LICENSE), without support, warranty, or a promise that a trade will complete. A historical cooperative trade on an earlier contract does not validate the current pool design, dispute path, or any future deployment.

## Try the interface without a wallet

Use Node.js 24 (see [`.tool-versions`](.tool-versions)). From a checkout:

```bash
cd app
npm ci
npm run build
```

Open `app/dist/flips-on-demand.html` in a browser and select **Import state**. Choose `app/example-pool-state.json` to explore the earlier market interface. This bundled file is synthetic and its timestamp is intentionally old, so the interface will mark its quote as stale. Do not use example prices or addresses for a transaction.

The HTML includes its own script and styles. The local market file reader can run without a server. Other features are networked: the optional session view contacts the provider and utility named by an imported connection file, and loading a public flip preview contacts the Idena explorer. Importing a market snapshot does **not** independently verify it against chain state, and the browser interface does not sign a purchase for you.

For source-level experiments, run `npm test` and `npm run typecheck` in `app`. The Go CLI is in [`tools/fod`](tools/fod), the contract prototypes in [`contracts`](contracts), and the proof code in [`proofs`](proofs). They require their own fixtures, toolchains, and chain checks; none is a one-command funded-trade tutorial. The older experiment notes in [`docs`](docs) and [`release`](release) are historical research records, not current release instructions.

## Risks and limits

| Area | What to assume |
| --- | --- |
| Funds and settlement | Contract calls can be irreversible. A bad quote, bond, fee estimate, refund deadline, or proof can strand or lose iDNA. The pool and dispute paths have not passed an independent audit or a complete funded end-to-end test. |
| Delivery | A private message is not itself proof of delivery. Cooperative buyer confirmation is fast, while a dispute may require a costly proof or public key release. Fast delivery can expose usable information before its transaction is included. |
| Proofs | A synthetic receipt or local test does not prove that a real flip belongs to canonical session history. Real proofs can take substantial time and may exceed a dispute window or fee budget. |
| Identity and privacy | On-chain registrations, orders, bonds, and public flip identifiers can link identities and activity. Published test keys and fixtures are public; never fund or reuse them. Keep actual node keys, vaults, flip content, proof witnesses, and delivery records outside this repository. |
| Operators and availability | A shared-node operator running key export can see decrypted flip content on that host. Provider availability, blob retention, private-message delivery, RPC integrity, and third-party preview availability are not guaranteed by a contract. |
| Imported data | A market or provider connection file can be stale, false, or point to a hostile service. Wallet enrollment and claim prompts bind the selected service origin, but importing a hostile service still exposes data sent to it. HTTPS alone does not establish that the service is trusted. Check a contract's address and code independently before any transaction. Do not send a node RPC key to an untrusted endpoint or put it in a shell command. |
| Attached value | The v6 contract rejects direct calls to its required WASM `allocate` export and rejects value on nonpayable methods in the pinned local runtime. This protection depends on that runtime's call convention and has not been qualified on a funded deployment. Do not send value to methods that do not require payment. |

There is no central party that can reverse a contract call or guarantee recovery. Check the code, contract bytes, chain state, and current fees yourself before any experiment involving assets. Research results and historical measurements may not apply to the current source.

## License and attribution

MIT; see [`LICENSE`](LICENSE). Third-party source and license notes are in [`THIRD_PARTY.md`](THIRD_PARTY.md).
