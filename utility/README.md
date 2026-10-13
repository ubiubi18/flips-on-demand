# Reference utility

The other side of `SPEC.md` 8.3: an application that decides whether a
provider-signed session result earns access. It is deliberately separate from
the gateway and implements the verification itself, so "it verified" means this
service checked the bytes rather than trusting the issuer's helper. The one rule
it shares with the reference interface is the score recomputation
(`app/src/score.ts`): a single implementation of the canonical answer form and
the transcript arithmetic, run by both sides so the two cannot drift.

```bash
cd utility
npm install
node src/main.ts example-policy.json        # serves :8788
node src/main.ts example-policy.json 8788 --host 0.0.0.0   # serves the LAN (TLS required)
node src/main.ts example-policy.json 8788 --trusted-proxy 127.0.0.1  # behind a TLS proxy
node src/main.ts example-policy.json 8788 --grants grants.json       # grants survive a restart
npm test
npm run typecheck
```

| Method | Path | Purpose |
|---|---|---|
| GET | `/policy` | the policy, including the scope of a grant |
| POST | `/access-challenge` | a one-time challenge an address signs to prove control |
| POST | `/answer-commitment` | the answers a participant committed while the window was open, signed by its wallet |
| POST | `/access` | one provider result, one decision |

The endpoint is public and signature-gated. `origins` pins which browser origins
may read the answer: `["null"]` is the offline `file://` testbed and `["*"]` is a
public testbed, while a production host lists its exact dapp origins. A policy
that names no origin sends no CORS header at all, so a browser outside the list -
or any browser, until origins are pinned - is blocked by its own policy;
non-browser callers are unaffected.

Nothing else exists: unknown paths are 404, the wrong method is 405, bodies must
be JSON and under the cap, requests are rate limited, and no cookie is ever set.
The server binds `127.0.0.1` unless `--host` names another interface; serving
the network with plain HTTP exposes the claim signatures, so terminate TLS in
front of it.

Rate limits are charged to the socket peer. When this service sits behind a
reverse proxy, pass the proxy's own address as `--trusted-proxy` (repeat for
several hops): only then is `X-Forwarded-For` believed, and the nearest
untrusted hop is charged. Without the flag the header is ignored, so a direct
caller cannot rotate it to mint fresh buckets. A proxy that does not forward
the client address must enforce its own per-client limits instead.

The example policy is a disabled template: replace its empty provider key,
placeholder digest and session window using independently obtained session
information before use. No template pin establishes honest consensus answers.

## How a decision is made

1. Parse the result; a malformed one is refused without guessing.
2. Find the provider in the policy. An unknown provider, or one with no key,
   is refused.
3. Verify the Ed25519 signature over the canonical payload, independently of
   the issuer.
4. Check the policy: utility id, accepted sessions, required outcome.
5. Check expiry.
6. Require `requireCommittedAnswers: true`, a content digest and a configured
   window for every accepted session. Policy files without these are refused.
   After close, recompute the score from wallet-signed answers this utility
   received through `/answer-commitment` during that window. The revealed key
   must match the content pin, utility id and configured window. A provider's
   transcript or `passed` signature alone can never authorize a grant, including
   when `requireScoreProof` is omitted or false. Its arithmetic may be checked
   diagnostically, but it is not evidence of the wallet's submitted answers.
7. When the result names an address, require the one-time challenge signed by
   that address; a missing or wrong proof is `claim_control_required`.
8. Refuse a policy that asks for identity eligibility: this adapter holds no
   identity check, so nothing a caller sends can satisfy it.
9. Check the local quota, then record the grant. A full local store refuses
   newcomers rather than evicting still-live consumed allowances; expired
   entries can free space.

Every refusal carries a reason a caller can act on: `bad_signature`,
`untrusted_provider`, `wrong_utility`, `unknown_session`, `outcome_not_accepted`,
`result_expired`, `claim_control_required`, `identity_required`,
`quota_exhausted`, `malformed_result`, `score_mismatch`, `score_unverifiable`,
`answers_not_committed`, `score_not_passed`, `session_not_closed`. The commitment endpoint answers
`unknown_session`, `bad_address`, `bad_answers`, `outside_window`,
`stale_commitment`, `bad_signature` or `busy` instead of recording anything.

## What a grant means, and does not

Every supported grant requires a wallet address, answers witnessed in-window,
and a fresh control proof: `POST /access-challenge` issues a nonce that the wallet
signs, and `/access` verifies and consumes it once. Quotas are keyed by utility,
provider and wallet. This is not unique personhood or a global entitlement.

Set policy `publicOrigin` to the exact HTTPS utility origin in the imported
connection file when using a proxy. Wallet challenges bind that origin; without
it, wallet claims are available only on direct loopback.

This reference service remains an operator-controlled application. Its clock,
commitment store and configured answer pin are not authenticated canonical
chain transitions, and a faucet/dapp operator can refuse the actual benefit.
Checking answers here does not establish their historical consensus correctness
or make the actual utility permissionless. Canonical scoring, consumption and
release remain an architectural blocker to the full-product goal.

There is no field through which a caller, or a server acting for it, can assert
an identity. A request body that carries `identityVerified` is inert: the value
is never read. A policy that sets `identityVerification.required` therefore
refuses every request with `identity_required`, and `/policy` publishes that
with `identityVerification.supported: false`. Eligibility an adapter could
honour has to come from chain-committed epoch state plus proof of control of
that identity, checked by the party that depends on it - not from a trusted
server's boolean, and not from the provider's scoring statement.

The quota is local bookkeeping and says so: `/policy` publishes
`quota: { scope: "local-process", durable, keyedBy }`. Grants are namespaced by
utility, provider and either address or participant, and expire as they are
read. Without `--grants` the store is in-memory and forgotten on restart
(`durable: false`); with `--grants FILE` the live entries are written 0600 and
atomically, so a one-time benefit survives a reconstruction (`durable: true`).
Either way it cannot decide a limit that is claimed to hold globally; such a
limit needs canonical protocol state.

Every granted decision carries `identity: "not-verified"`, which is the honest
scope of a grant this adapter issues.

A valid signature proves the provider issued the statement. It does not prove
the scoring was honest, and nothing here changes that.
