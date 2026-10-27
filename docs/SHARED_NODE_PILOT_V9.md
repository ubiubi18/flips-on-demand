# Shared node pilot for v9 sellers

An Idena author can sell a validated flip set without running a separate full node for Flips on Demand. The shared service watches the author's **public** chain records, retains the **original encrypted** flip files, and supplies restricted market RPC over HTTPS. The author still signs locally and keeps their identity key, epoch keys, vault password, and delivery witness. A shared node cannot reconstruct those secrets for an author.

This is an opt-in pilot for enrolled authors. It is not a public Idena RPC endpoint, a hosted wallet, or a promise that a captured set qualifies for sale. The v9 contract checks the final validation result and exact flip count when the author lists.

## What the operator runs

1. Run two synchronized native Idena nodes that independently agree on canonical block headers. Keep each node's keyed RPC on loopback and never expose its API key. Retain state through the validation boundary. The capture process needs access to a node's local state files for its read-only snapshot.
2. Enroll each consenting author before their validation session with a unique `LABEL:ADDRESS`. Run the existing `tools/fod/session_indexer.py` with `--author` for each enrolled author, an absolute owner-only archive, one or two `--node NAME:PORT:DATADIR` entries, and the executable `fod` CLI. If one configured node is temporarily unavailable, the other can retain the boundary anchors; two agreeing nodes are still required before a final bundle becomes ready. Use a dedicated archive for this public-author mode; the existing owner-only watcher and its witnesses stay separate. Keep the process running before, during, and after validation so it can prefetch encrypted CIDs and capture both state boundaries before pruning. A missing CID is retried and never marked ready.
3. Run `fod buyer-relay` on loopback with the same node's keyed loopback RPC, the independently checked v9 contract address and code hash, and `-shared-archive-dir` pointing to that archive. Put a TLS reverse proxy in front of its `/rpc` and `/evidence/v1/` paths, with request and rate limits. The relay accepts a narrow allowlist of public reads and **already signed** calls to the pinned contract. It refuses node-side signing and transactions to other contracts. It serves a verified archive copy for enrolled captures. For a CID absent from the archive, it queries the node only when the pinned contract includes that CID in an active trade, and returns it only after checking the CID and flip format.
4. Monitor the capture status. A `ready.json` record means the bundle's public state proofs and every encrypted blob were saved with CID and digest checks, with both header roots checked against the two nodes. It does **not** assert that the validation verdict qualifies or that the author has working opening keys. An incomplete archive is not served as ready.

The operator should publish the HTTPS relay origin, the pinned contract and code hash, and the epoch for which evidence was captured. Do not publish node API keys, data directories, identity keys, vaults, opening keys, or private witnesses. Keep the public relay behind an HTTPS proxy; `buyer-relay` itself binds only to loopback.

### Public IP without a domain

A domain is optional. The client accepts `https://<public-ip>/rpc` when the proxy presents a publicly trusted certificate whose IP address subject alternative name matches that public IP. Plain HTTP and an untrusted self-signed certificate do not work for community clients.

Use a stable, publicly reachable IP. Serve the certificate challenge on port 80, terminate HTTPS on port 443, and proxy only `/rpc` and `/evidence/v1/` to the loopback relay with request and rate limits. [Let's Encrypt issues IP address certificates](https://letsencrypt.org/2026/01/15/6day-and-ip-general-availability) with a 160-hour lifetime. [Certbot 5.4 or newer supports requesting them](https://letsencrypt.org/2026/03/11/shorter-certs-certbot) with the `shortlived` profile; configure automatic renewal and a deploy hook that reloads the proxy. Verify the public TLS connection and renewal before inviting sellers. If the public IP changes, issue a new matching certificate and update the published relay URL.

For a single enrolled author, the two processes have this shape after the operator has set the indicated absolute-path and address variables:

```bash
python3 tools/fod/session_indexer.py \
  --cli "$FOD_CLI" --archive "$FOD_ARCHIVE" \
  --node "first:$FIRST_RPC_PORT:$FIRST_NODE_DIR" \
  --node "second:$SECOND_RPC_PORT:$SECOND_NODE_DIR" \
  --author "pilot:$AUTHOR_ADDRESS"

"$FOD_CLI" buyer-relay \
  -rpc "http://127.0.0.1:$FIRST_RPC_PORT" \
  -key-file "$FIRST_RPC_KEY_FILE" \
  -contract "$V9_CONTRACT" -code-hash "$V9_CODE_HASH" \
  -shared-archive-dir "$FOD_ARCHIVE" \
  -listen 127.0.0.1:9069
```

The capture process is a long-running watcher; run the relay as a separate process. Start both **before** validation, supervise them without restarting or replacing an existing owner-only watcher, and expose the relay only through a TLS proxy. The service does not accept a new author by silently changing a running watch file; coordinate enrollment before arming that epoch.

## What the seller does

1. Create and submit flips in the usual Idena app, then retain the identity key locally. Enroll the author address with the shared operator **before** the validation session. The service may be able to fetch the original encrypted CIDs from either node; this is not guaranteed, so keep your own originals when possible.
2. Start the v9 standalone app in `-seller-client` mode with the service's HTTPS `/rpc` URL, the verified contract and code pins, an owner-only local `-nodekey`, and a separate local app-data folder. The app checks that this key belongs to the seller. The node API key is never copied to the seller's device. If you have Felix's password-encrypted mobile backup, import it **offline on your own desktop** with the command below first; the output is a plaintext identity key file protected by owner-only file permissions.
3. After validation, use **My flips → Collect & check**. The client discovers this author's ready captures, including older epochs, and imports one missing session per collection cycle. It verifies the session against chain headers and checks every encrypted blob CID locally. A captured session with an adverse validation result is shown as **Not eligible to sell**. For a clean set, use **Protect package** to derive and password-protect the opening keys on your own device.
4. Review the security deposit and click **Join sell queue** for a ready package. The local client signs `queueSell`; the relay only forwards the signed call. After a match, keep the app available for buyer readiness and buyer-encrypted key delivery. The [seller guide](SELLER_GUIDE_V9.md) covers settlement and disputes.

For a Felix mobile backup, place the encrypted backup on your desktop, choose an existing private `0700` directory for the output, and run:

```bash
"$FOD_CLI" import-idena-backup \
  -backup "$MOBILE_BACKUP_FILE" \
  -out "$LOCAL_IDENTITY_KEY_FILE"
```

The command asks for the backup password without echoing it, refuses to overwrite an existing key, and writes the new key as a `0600` file. Keep that file private; the import does not delete the encrypted backup. After setting the published HTTPS relay and contract pins and the absolute path to the author's local identity key, the seller starts the client with:

```bash
"$FOD_CLI" app-v9 -seller-client \
  -relay "$MARKET_RELAY_HTTPS_URL/rpc" \
  -nodekey "$LOCAL_IDENTITY_KEY_FILE" \
  -contract "$V9_CONTRACT" -code-hash "$V9_CODE_HASH"
```

Use a separate owner-only v9 seller profile. The app remembers the connection after the first run. No node API key or identity-key bytes belong in the relay URL or a support message.

The shared archive can supply ciphertext and public evidence to anyone, but neither is enough to sell: only the author can derive the epoch opening keys and sign the listing. [Felix's Android app](https://github.com/truongfelix/idena-mobile/blob/30ebe8878c6abacbd61a1650750950c9a16cf92b/go/mobile/key.go) can export a password-encrypted identity-key backup. The offline importer follows that format, but it has not yet been tested against an independently exported mobile backup. Test a copy locally before relying on this route; `-nodekey` reads the resulting owner-only local key file. Do not upload the backup, password or decrypted key to the shared operator. A phone-only seller flow still needs local key handling and trading integration. A buyer can use the separate [node-free buyer client](BUYER_ONLY_V9.md) with a funded unvalidated wallet.

## Public evidence API

For an enrolled author, the relay serves:

```text
GET /evidence/v1/authors/{author-address}/epochs
GET /evidence/v1/epochs/{epoch}/authors/{author-address}/session
GET /evidence/v1/epochs/{epoch}/authors/{author-address}/blobs/{cid}
```

The discovery route returns a descending `epochs` array of complete captures for that address. The local enrollment label also works in the session and blob routes. Those routes return `404` while a record is missing, incomplete, mismatched, or ambiguous. The service never serves the local witness, vault or key files. A client must still verify the session's roots against chain headers, its clean validation verdict and each blob against its CID; the HTTPS server and its readiness marker are not the final authority.

## Pilot limits

- Enrollment and capture must start before the relevant validation boundary. The old state versions may be pruned, and missing ciphertext may never become available from the shared nodes.
- A captured session does not by itself prove a clean validation verdict, working opening keys, or market eligibility. The seller client checks the public verdict before showing a package as ready, checks opening keys locally before listing, and the v9 contract independently enforces the validation rules.
- The operator's two nodes can agree yet share an upstream fork or compromise. Sellers should verify the contract code pin independently and use a trusted HTTPS origin.
- Signed transactions, queue matching and encrypted delivery still need the seller's app to be available at the relevant deadlines. The on-chain order persists if the app closes, but the service does not act as the seller's agent.
- This pilot has no anonymous self-enrollment or phone-only trading integration. Desktop import of a mobile backup creates a plaintext local identity key; do not request that users upload identity keys or vault passwords to work around that limit.
