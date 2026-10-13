# Provider gateway

The shared-node gateway of `SPEC.md` 4.4 and the reference session integration of
`SPEC.md` 8.2: provider customers enroll, receive challenges, submit answers, and
collect a provider-signed result, without running a node or the marketplace CLI.

```bash
cd gateway
npm install
node src/main.ts example-config.json     # serves example-pool.json on :8787
npm test                                 # 34 checks, listed below
npm run typecheck
```

On first start the gateway writes `provider-application-key.json` (mode 0600), a
dedicated Ed25519 key for signing results. It is git-ignored and is never the
shared node wallet key.

## Surface

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/session/policy` | no | published session timing, scoring rules, pool provenance |
| GET | `/session/lobby` | no | the shared window state, participant count and challenge count |
| GET | `/shared-node/status` | no | the node's shared-node status: session, pool, operations and custody modes |
| GET | `/provider/key` | no | the provider's application public key, so a utility can verify results |
| POST | `/session/enroll-challenge` | no | issues the one-time EIP-191 challenge text for a wallet address, bound to the provider, the session and a server nonce |
| POST | `/session/enroll` | no | consumes that challenge and issues a scoped, short-lived participant token; the bound address names the EVM/Idena address in the token, the signed result and the utility grant |
| GET | `/session/challenges` | yes | assigned challenges, never answer material |
| POST | `/session/answers` | yes | one answer per challenge; replays and closed sessions refused |
| GET | `/session/result` | yes | provider-signed statement bound to participant and utility; issued only after the session closes |
| GET | `/marketplace/state` | yes | the provider's own cached snapshot |
| POST | `/transactions` | yes | **refused** (501): forwarding caller-supplied transactions would make this an unrestricted relay |

A gate session requires the wallet binding: the client asks
`/session/enroll-challenge` for the text `fod-enroll:v3:<provider>:<session>:<address>:<encoded-public-origin>:<nonce>:<issuedAt>`,
signs it with `personal_sign`, and the gateway verifies it with `viem`, consumes
the nonce once, refuses a stale challenge or a mismatched address, and includes
the address in the signed result. Idena derives addresses exactly as Ethereum
does, so the same EVM address is also an Idena address; it is a key, not a
person, and the utility grant says so.

## What the tests pin

No RPC passthrough (unlisted paths are 404), 405 for a known path with the wrong
method, bearer-only authorisation (a query parameter is not a token, and neither
is an origin), no cookies, CORS limited to configured origins and never `*`, a
`file://` origin is not authorisation, body size and content-type limits, rate
limiting, session expiry, replay rejection, no answer digests in assignments,
a signed EVM address that binds the enrollment and the result while a bad
signature or a stale challenge is refused,
no per-answer correctness while the session is open, a salted content digest
whose preimage is revealed only after close, a one-time enrollment nonce, and a
result signature that verifies with the published key and fails after a single
byte of tampering.

## What this is not

The gateway is not an authority. It cannot approve marketplace proofs, move
escrow, or sign customer transactions; it holds no wallet material. A valid
result signature means the provider issued a statement, not that the scoring was
honest, and the result payload says so. What the result does carry is the
`transcript`: the digest of every answer this provider scored, `null` where it
scored none. A participant that kept the `contentDigest` it read before
answering can combine that transcript with the `contentReveal` the closed
policy publishes and the answers it submitted, and recompute the score for
itself (`app/src/score.ts`); the pool's provenance is still the provider's
claim: a utility that needs to verify it checks the marketplace rather than this
gateway.

The bundled example binds loopback and speaks plain HTTP, which is what a local
test needs. A gateway that serves anyone but the machine it runs on has to be
reached over HTTPS: the participant token is a bearer credential, and a browser
will not read a `file://` page as authorisation for anything. Terminating TLS in
front of the process is enough; the token and the configured origin list are the
only authorisation the gateway has.

Set `publicOrigin` to the exact HTTPS origin in the imported connection file
when using a proxy. It is bound into wallet challenges and cannot be supplied
by a request header. Without it, only direct loopback wallet enrollment works.

The bind follows from the config: `host` defaults to `127.0.0.1`, so serving
another interface is an explicit choice, and the TLS requirement above applies
the moment that choice is made.

When TLS terminates in a proxy, list the proxy's own address in the config's
`trustedProxies`. Only then is `X-Forwarded-For` believed for rate limiting, and
the nearest untrusted hop is charged, so clients behind one proxy do not share a
bucket. With no `trustedProxies`, the header is ignored and the socket peer is
charged.

Live consensus participation is deliberately absent (`SPEC.md` 8.2). A utility
that needs one would integrate its own contract or service and verify identity
eligibility independently of this scoring statement.
