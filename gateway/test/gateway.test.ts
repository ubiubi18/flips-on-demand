// Security and flow tests for the provider gateway.
//
// These check the properties SPEC.md 4.4 and 8.2 name, against a real HTTP
// server on a loopback port: no RPC passthrough, bearer-only authorisation,
// size and rate limits, no cookies, CORS limited to configured origins, no
// answer material in challenge assignments, replay and expiry rejection, and a
// result signature that verifies independently.

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import type { AddressInfo } from "node:net";
import { connect } from "node:net";
import type { Server } from "node:http";
import { loadPool } from "../src/pool.ts";
import { answerDigest, SessionState } from "../src/session.ts";
import { loadOrCreateApplicationKey, verifyResult } from "../src/keys.ts";
import { createGatewayServer } from "../src/http.ts";
import { RateLimiter } from "../src/http.ts";
import { loadConfig, type GatewayConfig } from "../src/config.ts";
import { privateKeyToAccount } from "viem/accounts";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const PROVIDER = "test-provider";

it("refuses new rate-limit keys at capacity without forgetting a live client", () => {
  const limiter = new RateLimiter({ windowSeconds: 60, maxRequests: 1 });
  for (let i = 0; i < 10_000; i++) assert.equal(limiter.allow(`client:${i}`, 0), true);
  assert.equal(limiter.allow("new-client", 1), false);
  assert.equal(limiter.allow("client:0", 2), false);
  assert.equal(limiter.allow("new-client", 60_000), true);
});

// The frozen flip the pool fixture carries: the same vector the guest and the
// gateway decoder are checked against (`SOURCES.md` F-98).
const flipVector = JSON.parse(
  await readFile(join(import.meta.dirname, "..", "..", "proofs", "flip", "vectors", "flip-v1.json"), "utf8")
) as { publicPart: string; privatePart: string };

async function startGateway(options: {
  rateLimit?: { windowSeconds: number; maxRequests: number };
  origins?: string[];
  ttlSeconds?: number;
  now?: () => number;
  trustedProxies?: string[];
  maxParticipants?: number;
}): Promise<{ server: Server; base: string; keyPath: string; pool: Awaited<ReturnType<typeof loadPool>> }> {
  const dir = await mkdtemp(join(tmpdir(), "fod-gateway-"));
  const poolPath = join(dir, "pool.json");
  await writeFile(
    poolPath,
    JSON.stringify({
      format: "fod.flip-pool",
      version: 1,
      sessionId: "session-test",
      utilityId: "utility-test",
      provenance: {
        epoch: 204,
        author: "4444444444444444444444444444444444444444",
        source: "test pool",
        blockRange: "1-2",
        answers: "the provider's answers file",
      },
      rules: {
        passThreshold: 2,
        opensAt: "2026-01-01T00:00:00Z",
        closesAt: "2027-01-01T00:00:00Z",
      },
      // Fixture-only salt: it keeps the same digest across test runs.
      answerSalt: "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a",
      challenges: [
        {
          challengeId: "c1",
          cid: "cid-1",
          answerDigest: answerDigest("first answer"),
          content: { publicPart: flipVector.publicPart, privatePart: flipVector.privatePart },
        },
        { challengeId: "c2", cid: "cid-2", answerDigest: answerDigest("second answer") },
      ],
    }),
  );
  const pool = await loadPool(poolPath);
  const keyPath = join(dir, "provider-application-key.json");
  const applicationKey = await loadOrCreateApplicationKey(keyPath, PROVIDER);
  const config: GatewayConfig = {
    port: 0,
    host: "127.0.0.1",
    providerId: PROVIDER,
    origins: options.origins ?? ["null"],
    sessionTtlSeconds: options.ttlSeconds ?? 1800,
    bodyLimitBytes: 4096,
    rateLimit: options.rateLimit ?? { windowSeconds: 60, maxRequests: 1000 },
    poolPath,
    applicationKeyPath: keyPath,
    trustedProxies: options.trustedProxies ?? [],
    baseDir: dir,
  };
  const session = new SessionState(pool, config.sessionTtlSeconds, options.maxParticipants);
  const server = createGatewayServer({
    config,
    pool,
    session,
    applicationKey,
    marketplaceState: () => ({ offers: [] }),
    now: options.now ?? (() => Date.now()),
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address() as AddressInfo;
  return { server, base: `http://127.0.0.1:${address.port}`, keyPath, pool };
}

function stop(server: Server): Promise<void> {
  return new Promise((resolve) => server.close(() => resolve()));
}

/** Writes a raw request and returns whatever the server answers. */
function rawExchange(port: number, request: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const socket = connect(port, "127.0.0.1");
    let received = "";
    socket.setEncoding("utf8");
    socket.on("connect", () => socket.write(request));
    socket.on("data", (chunk: string) => {
      received += chunk;
    });
    socket.on("end", () => resolve(received));
    socket.on("error", reject);
    socket.setTimeout(5_000, () => {
      socket.destroy();
      resolve(received);
    });
  });
}

// A distinct deterministic key per call, so one test's live enrollment never
// collides with another address (`AH-1` wants at most one per address).
let walletCounter = 1;
function nextTestKey(): `0x${string}` {
  const hex = walletCounter.toString(16).padStart(64, "0");
  walletCounter += 1;
  return `0x${hex}` as `0x${string}`;
}

async function enrollWith(
  base: string,
  key: `0x${string}`,
): Promise<{ token: string; address: string }> {
  const account = privateKeyToAccount(key);
  const challengeResponse = await fetch(`${base}/session/enroll-challenge`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ address: account.address }),
  });
  assert.equal(challengeResponse.status, 201, "the gateway issues an enrollment challenge");
  const { challenge } = (await challengeResponse.json()) as { challenge: string };
  const signature = await account.signMessage({ message: challenge });
  const response = await fetch(`${base}/session/enroll`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ address: account.address, challenge, signature }),
  });
  assert.equal(response.status, 201, "the bound wallet enrolls");
  const body = (await response.json()) as { token: string; address?: string };
  return { token: body.token, address: body.address ?? account.address.toLowerCase() };
}

async function enroll(base: string): Promise<string> {
  return (await enrollWith(base, nextTestKey())).token;
}

describe("provider gateway", () => {
  let gateway: Awaited<ReturnType<typeof startGateway>>;
  // The suite runs on a controllable clock inside the pool's window, so a test
  // can close the session and collect a result without waiting for wall time.
  const openClock = Date.parse("2026-06-01T00:00:00Z");
  const closedClock = Date.parse("2027-01-02T00:00:00Z");
  let clock = openClock;

  before(async () => {
    // A long TTL: the clock jumps to the closed window to collect a result, and
    // the token that answered inside the window must still authenticate.
    gateway = await startGateway({ ttlSeconds: 40_000_000, now: () => clock });
  });
  after(async () => {
    await stop(gateway.server);
    clock = openClock;
  });

  it("serves the published policy without authentication, and no answer material while open", async () => {
    const response = await fetch(`${gateway.base}/session/policy`);
    assert.equal(response.status, 200);
    const policy = (await response.json()) as {
      format: string;
      version: number;
      rules: { passThreshold: number };
      sessionId: string;
      provenance: { answers?: string };
      contentDigest: string;
    };
    // `SPEC.md` 4.5: a customer's client has to be able to tell what it is
    // reading before it trusts a field in it.
    assert.equal(policy.format, "fod.session-policy");
    assert.equal(policy.version, 1);
    assert.equal(policy.sessionId, "session-test");
    // What the answers rest on travels with the pool and reaches the
    // participant as the provider's own statement (`REVIEW.md` A09).
    assert.equal(policy.provenance.answers, "the provider's answers file");
    assert.equal(policy.rules.passThreshold, 2);
    assert.match(policy.contentDigest, /^[0-9a-f]{64}$/);
    // `AH-2`: while the session is open the digest is opaque. Neither the salt
    // nor any answer digest may appear in what the public policy serves, so the
    // digest cannot be enumerated offline against a small answer pool.
    const serialized = JSON.stringify(policy);
    assert.equal(serialized.includes("answerSalt"), false, "the open policy must not reveal the salt");
    assert.equal(serialized.includes("answerDigest"), false, "the open policy must not reveal answer digests");
    assert.equal("contentReveal" in policy, false, "the preimage is revealed only after close");
  });

  it("has no RPC passthrough", async () => {
    for (const path of ["/rpc", "/bcn_blockAt", "/", "/session/../../rpc"]) {
      const response = await fetch(`${gateway.base}${path}`);
      assert.equal(response.status, 404, `${path} should not exist`);
    }
  });

  it("answers 405 for a known path with the wrong method", async () => {
    const response = await fetch(`${gateway.base}/session/policy`, { method: "POST" });
    assert.equal(response.status, 405);
    assert.equal(response.headers.get("allow"), "GET");
  });

  it("requires a bearer token on protected endpoints", async () => {
    const anonymous = await fetch(`${gateway.base}/session/challenges`);
    assert.equal(anonymous.status, 401);
    // A query parameter is not a token: tokens only travel in the header.
    const inQuery = await fetch(`${gateway.base}/session/challenges?token=deadbeef`);
    assert.equal(inQuery.status, 401);
    const wrongScheme = await fetch(`${gateway.base}/session/challenges`, {
      headers: { authorization: "Basic deadbeef" },
    });
    assert.equal(wrongScheme.status, 401);
    const unknown = await fetch(`${gateway.base}/session/challenges`, {
      headers: { authorization: "Bearer 00ff00ff" },
    });
    assert.equal(unknown.status, 401);
  });

  it("never sets a cookie", async () => {
    const response = await fetch(`${gateway.base}/session/enroll`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: "{}",
    });
    assert.equal(response.headers.get("set-cookie"), null);
  });

  it("assigns challenges with their content and without answer material", async () => {
    const token = await enroll(gateway.base);
    const response = await fetch(`${gateway.base}/session/challenges`, {
      headers: { authorization: `Bearer ${token}` },
    });
    assert.equal(response.status, 200);
    const body = (await response.json()) as {
      challenges: { challengeId: string; cid: string; variants?: string[][] }[];
    };
    assert.equal(body.challenges.length, 2);
    // The challenge whose pool entry carries content arrives as two sequences of
    // four pictures, in the order each sequence shows them; the other one has
    // nothing to show and says so by carrying no variants.
    const withContent = body.challenges.find((entry) => entry.challengeId === "c1");
    assert.equal(withContent?.variants?.length, 2);
    for (const sequence of withContent?.variants ?? []) {
      assert.equal(sequence.length, 4);
      assert.ok(sequence.every((image) => image.startsWith("data:image/")));
    }
    assert.notDeepEqual(withContent?.variants?.[0], withContent?.variants?.[1]);
    assert.equal(body.challenges.find((entry) => entry.challengeId === "c2")?.variants, undefined);
    const serialized = JSON.stringify(body);
    assert.ok(!serialized.includes("answerDigest"), "assignments must not leak answer digests");
  });

  it("accepts answers without a correctness signal, and signs a result only after close", async () => {
    const token = await enroll(gateway.base);
    const auth = { authorization: `Bearer ${token}`, "content-type": "application/json" };

    const first = await fetch(`${gateway.base}/session/answers`, {
      method: "POST",
      headers: auth,
      body: JSON.stringify({ challengeId: "c1", answer: "  First   Answer " }),
    });
    assert.equal(first.status, 202);
    // `AH-1`: a submission is accepted; nothing about its correctness leaves
    // the gateway while the session is open.
    assert.deepEqual(await first.json(), { accepted: true });

    const replay = await fetch(`${gateway.base}/session/answers`, {
      method: "POST",
      headers: auth,
      body: JSON.stringify({ challengeId: "c1", answer: "first answer" }),
    });
    assert.equal(replay.status, 409);
    assert.deepEqual(await replay.json(), { error: "already_answered" });

    const unknown = await fetch(`${gateway.base}/session/answers`, {
      method: "POST",
      headers: auth,
      body: JSON.stringify({ challengeId: "nope", answer: "x" }),
    });
    assert.equal(unknown.status, 400);

    const second = await fetch(`${gateway.base}/session/answers`, {
      method: "POST",
      headers: auth,
      body: JSON.stringify({ challengeId: "c2", answer: "second answer" }),
    });
    assert.equal(second.status, 202);

    // While the window is open the signed statement is refused: its score
    // would be the same answer oracle the answers endpoint no longer gives.
    const early = await fetch(`${gateway.base}/session/result`, { headers: auth });
    assert.equal(early.status, 403);
    assert.equal(((await early.json()) as { error: string }).error, "session_open");

    clock = closedClock;
    try {
      const resultResponse = await fetch(`${gateway.base}/session/result`, { headers: auth });
      assert.equal(resultResponse.status, 200);
      const result = (await resultResponse.json()) as Record<string, unknown> & { signature: string };
      assert.equal(result.outcome, "passed");
      assert.equal((result.score as { correct: number }).correct, 2);
      assert.equal((result.issuedBy as { kind: string }).kind, "provider");

      // The signed result names what the session was, and the published policy
      // names the same digest: that is what a utility pins to accept only the
      // session it examined (`SPEC.md` 8.3).
      const policy = (await (await fetch(`${gateway.base}/session/policy`)).json()) as {
        contentDigest: string;
        contentReveal: Record<string, unknown>;
      };
      assert.match(policy.contentDigest, /^[0-9a-f]{64}$/);
      assert.equal(result.contentDigest, policy.contentDigest);
      // `AH-2`: after close the preimage is published and recomputes to the
      // digest the utility already pinned.
      const { digestOfContent } = await import("../src/pool.ts");
      assert.equal(
        digestOfContent(policy.contentReveal as never),
        policy.contentDigest,
        "the revealed content must recompute to the pinned digest",
      );

      const keyResponse = await fetch(`${gateway.base}/provider/key`);
      const key = (await keyResponse.json()) as { publicKeyHex: string };
      const { signature, ...unsigned } = result;
      const { canonicalJson } = await import("../src/keys.ts");
      assert.ok(
        verifyResult(key.publicKeyHex, canonicalJson(unsigned), signature),
        "the result signature must verify against the published key",
      );
      // A tampered result must not verify.
      const tampered = { ...unsigned, outcome: "passed", score: { assigned: 2, answered: 2, correct: 2, passThreshold: 1 } };
      assert.equal(verifyResult(key.publicKeyHex, canonicalJson(tampered), signature), false);
    } finally {
      clock = openClock;
    }
  });

  it("binds a signed EVM address to the enrollment and the result", async () => {
    const { token, address } = await enrollWith(gateway.base, nextTestKey());
    const auth = { authorization: `Bearer ${token}`, "content-type": "application/json" };
    for (const [challengeId, answer] of [
      ["c1", "first answer"],
      ["c2", "second answer"],
    ] as const) {
      const answered = await fetch(`${gateway.base}/session/answers`, {
        method: "POST",
        headers: auth,
        body: JSON.stringify({ challengeId, answer }),
      });
      assert.equal(answered.status, 202);
    }
    clock = closedClock;
    try {
      const resultResponse = await fetch(`${gateway.base}/session/result`, { headers: auth });
      assert.equal(resultResponse.status, 200);
      const result = (await resultResponse.json()) as { participantAddress?: string; signature: string };
      assert.equal(result.participantAddress, address);

      const key = (await (await fetch(`${gateway.base}/provider/key`)).json()) as { publicKeyHex: string };
      const { signature: providerSignature, ...unsigned } = result;
      const { canonicalJson } = await import("../src/keys.ts");
      assert.ok(
        verifyResult(key.publicKeyHex, canonicalJson(unsigned), providerSignature),
        "the signed result has to cover the bound address",
      );
    } finally {
      clock = openClock;
    }
  });

  it("publishes the answers it scored, so the participant can recompute the score", async () => {
    const { token } = await enrollWith(gateway.base, nextTestKey());
    const auth = { authorization: `Bearer ${token}`, "content-type": "application/json" };
    // One answer, in a spelling the canonical form folds onto c1's digest, and
    // one challenge left unanswered.
    const answered = await fetch(`${gateway.base}/session/answers`, {
      method: "POST",
      headers: auth,
      body: JSON.stringify({ challengeId: "c1", answer: "  First   Answer " }),
    });
    assert.equal(answered.status, 202);
    clock = closedClock;
    try {
      const response = await fetch(`${gateway.base}/session/result`, { headers: auth });
      assert.equal(response.status, 200);
      const result = (await response.json()) as {
        score: { assigned: number; answered: number; correct: number; passThreshold: number };
        transcript?: { entries: { challengeId: string; answerDigest: string | null }[] };
        signature: string;
      };
      assert.deepEqual(result.transcript?.entries, [
        { challengeId: "c1", answerDigest: answerDigest("first answer") },
        { challengeId: "c2", answerDigest: null },
      ]);
      assert.equal(result.score.assigned, 2);
      assert.equal(result.score.answered, 1);
      assert.equal(result.score.correct, 1);
      // The transcript travels inside the signed statement, not beside it.
      const key = (await (await fetch(`${gateway.base}/provider/key`)).json()) as { publicKeyHex: string };
      const { signature, ...unsigned } = result;
      const { canonicalJson } = await import("../src/keys.ts");
      assert.ok(verifyResult(key.publicKeyHex, canonicalJson(unsigned), signature));
    } finally {
      clock = openClock;
    }
  });

  it("refuses an EVM binding whose signature does not match the address", async () => {
    const account = privateKeyToAccount(nextTestKey());
    const other = privateKeyToAccount(nextTestKey());
    const challengeResponse = await fetch(`${gateway.base}/session/enroll-challenge`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ address: account.address }),
    });
    const { challenge } = (await challengeResponse.json()) as { challenge: string };
    const signature = await other.signMessage({ message: challenge });
    const response = await fetch(`${gateway.base}/session/enroll`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ address: account.address, challenge, signature }),
    });
    assert.equal(response.status, 403);
    const body = (await response.json()) as { error?: string };
    assert.equal(body.error, "bad_signature");
  });

  it("refuses a stale EVM binding challenge", async () => {
    const account = privateKeyToAccount(nextTestKey());
    const challengeResponse = await fetch(`${gateway.base}/session/enroll-challenge`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ address: account.address }),
    });
    const { challenge } = (await challengeResponse.json()) as { challenge: string };
    const signature = await account.signMessage({ message: challenge });
    clock = openClock + 10 * 60 * 1000;
    try {
      const response = await fetch(`${gateway.base}/session/enroll`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ address: account.address, challenge, signature }),
      });
      assert.equal(response.status, 403);
      const body = (await response.json()) as { error?: string };
      assert.equal(body.error, "stale_challenge");
    } finally {
      clock = openClock;
    }
  });

  it("refuses a replayed or cross-provider enrollment challenge", async () => {
    const account = privateKeyToAccount(nextTestKey());
    const issuedResponse = await fetch(`${gateway.base}/session/enroll-challenge`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ address: account.address }),
    });
    assert.equal(issuedResponse.status, 201);
    const { challenge } = (await issuedResponse.json()) as { challenge: string };
    const signature = await account.signMessage({ message: challenge });
    const binding = { address: account.address, challenge, signature };
    const first = await fetch(`${gateway.base}/session/enroll`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(binding),
    });
    assert.equal(first.status, 201);
    // The same signature again is worth nothing: the nonce is consumed once.
    const replay = await fetch(`${gateway.base}/session/enroll`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(binding),
    });
    assert.equal(replay.status, 403);
    assert.equal(((await replay.json()) as { error: string }).error, "unknown_nonce");

    // A challenge naming another provider is refused even when it is signed
    // correctly: the gateway checks the context it issued, not just the key.
    const forged = `fod-enroll:v3:other-provider:session-test:${account.address.toLowerCase()}:${encodeURIComponent(gateway.base)}:${"11".repeat(16)}:${clock}`;
    const forgedSignature = await account.signMessage({ message: forged });
    const cross = await fetch(`${gateway.base}/session/enroll`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ address: account.address, challenge: forged, signature: forgedSignature }),
    });
    assert.equal(cross.status, 403);
    assert.equal(((await cross.json()) as { error: string }).error, "bad_challenge");
  });

  it("rejects a valid signature for another gateway origin even with a matching nonce", async () => {
    const account = privateKeyToAccount(nextTestKey());
    const issued = await fetch(`${gateway.base}/session/enroll-challenge`, {
      method: "POST", headers: { "content-type": "application/json" },
      body: JSON.stringify({ address: account.address }),
    });
    const { challenge } = (await issued.json()) as { challenge: string };
    const changed = challenge.replace(encodeURIComponent(gateway.base), encodeURIComponent("https://relay.example"));
    const response = await fetch(`${gateway.base}/session/enroll`, {
      method: "POST", headers: { "content-type": "application/json", host: "relay.example" },
      body: JSON.stringify({ address: account.address, challenge: changed,
        signature: await account.signMessage({ message: changed }) }),
    });
    assert.equal(response.status, 403);
    assert.equal(((await response.json()) as { error: string }).error, "bad_challenge");
  });

  it("refuses a second live enrollment for the same address", async () => {
    const key = nextTestKey();
    const first = await enrollWith(gateway.base, key);
    const account = privateKeyToAccount(key);
    const issuedResponse = await fetch(`${gateway.base}/session/enroll-challenge`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ address: account.address }),
    });
    assert.equal(issuedResponse.status, 201);
    const { challenge } = (await issuedResponse.json()) as { challenge: string };
    const signature = await account.signMessage({ message: challenge });
    const second = await fetch(`${gateway.base}/session/enroll`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ address: account.address, challenge, signature }),
    });
    assert.equal(second.status, 409);
    assert.equal(((await second.json()) as { error: string }).error, "address_enrolled");
    // The first enrollment is untouched: its token still reads challenges.
    const stillThere = await fetch(`${gateway.base}/session/challenges`, {
      headers: { authorization: `Bearer ${first.token}` },
    });
    assert.equal(stillThere.status, 200);
  });

  it("publishes the shared lobby and gives two participants the same challenges", async () => {
    const lobby = (await (await fetch(`${gateway.base}/session/lobby`)).json()) as {
      window: string;
      participants: number;
      challenges: number;
    };
    assert.equal(lobby.window, "open");
    assert.equal(lobby.challenges, 2);

    const first = await enroll(gateway.base);
    const second = await enroll(gateway.base);
    const challengesFor = async (token: string) =>
      (await (
        await fetch(`${gateway.base}/session/challenges`, {
          headers: { authorization: `Bearer ${token}` },
        })
      ).json()) as { challenges: unknown };
    assert.deepEqual(await challengesFor(first), await challengesFor(second));

    const after = (await (await fetch(`${gateway.base}/session/lobby`)).json()) as {
      participants: number;
    };
    assert.ok(after.participants >= lobby.participants + 2);
  });

  it("publishes the shared-node status and its custody modes", async () => {
    const response = await fetch(`${gateway.base}/shared-node/status`);
    assert.equal(response.status, 200);
    const status = (await response.json()) as {
      format: string;
      nodeId: string;
      operations: string[];
      custody: string[];
    };
    assert.equal(status.format, "fod.shared-node-status");
    assert.equal(status.nodeId, PROVIDER);
    assert.deepEqual(status.operations, ["session", "pool"]);
    assert.deepEqual(status.custody, ["self", "hosted"]);
  });

  it("refuses transaction forwarding", async () => {
    const token = await enroll(gateway.base);
    const response = await fetch(`${gateway.base}/transactions`, {
      method: "POST",
      headers: { authorization: `Bearer ${token}`, "content-type": "application/json" },
      body: JSON.stringify({ signed: "0xdeadbeef" }),
    });
    assert.equal(response.status, 501);
    assert.deepEqual(await response.json(), {
      error: "not_implemented",
      message: "submitting signed transactions through the gateway is not implemented",
    });
  });
});

describe("gateway limits", () => {
  it("answers a malformed Host header and keeps serving", async () => {
    const gateway = await startGateway({});
    try {
      const port = Number(new URL(gateway.base).port);
      const answer = await rawExchange(port, "GET /session/policy HTTP/1.1\r\nHost: [\r\nConnection: close\r\n\r\n");
      assert.match(answer, /^HTTP\/1\.1 400/, `expected a 400, got ${answer.split("\r\n")[0]}`);
      // The process survived: the next ordinary request is served normally.
      const after = await fetch(`${gateway.base}/session/policy`);
      assert.equal(after.status, 200);
      assert.equal(((await after.json()) as { sessionId: string }).sessionId, "session-test");
    } finally {
      await stop(gateway.server);
    }
  });

  it("counts rotating invalid tokens against the client, not the tokens", async () => {
    const gateway = await startGateway({ rateLimit: { windowSeconds: 60, maxRequests: 3 } });
    try {
      for (let attempt = 0; attempt < 3; attempt += 1) {
        const refused = await fetch(`${gateway.base}/session/challenges`, {
          headers: { authorization: `Bearer rotating-token-${attempt}` },
        });
        assert.equal(refused.status, 401, "an unknown token is not authorisation");
      }
      // A fourth fresh token would have been a fourth bucket before; now it is
      // the same anonymous client, and the anonymous limit applies.
      const limited = await fetch(`${gateway.base}/session/challenges`, {
        headers: { authorization: "Bearer rotating-token-3" },
      });
      assert.equal(limited.status, 429);
    } finally {
      await stop(gateway.server);
    }
  });

  it("charges the forwarded client only behind a trusted proxy", async () => {
    const trusted = await startGateway({
      rateLimit: { windowSeconds: 60, maxRequests: 2 },
      trustedProxies: ["127.0.0.1"],
    });
    try {
      const asClient = (address: string) =>
        fetch(`${trusted.base}/session/policy`, { headers: { "x-forwarded-for": address } });
      assert.equal((await asClient("198.51.100.7")).status, 200);
      assert.equal((await asClient("198.51.100.7")).status, 200);
      assert.equal((await asClient("198.51.100.7")).status, 429);
      assert.equal(
        (await asClient("198.51.100.8")).status,
        200,
        "another client behind the proxy has its own bucket",
      );
    } finally {
      await stop(trusted.server);
    }

    const direct = await startGateway({ rateLimit: { windowSeconds: 60, maxRequests: 2 } });
    try {
      const asClient = (address: string) =>
        fetch(`${direct.base}/session/policy`, { headers: { "x-forwarded-for": address } });
      assert.equal((await asClient("198.51.100.7")).status, 200);
      assert.equal((await asClient("198.51.100.8")).status, 200);
      assert.equal(
        (await asClient("198.51.100.9")).status,
        429,
        "an untrusted forwarded header must not mint fresh buckets",
      );
    } finally {
      await stop(direct.server);
    }
  });

  it("refuses enrollment at capacity without evicting a live participant", async () => {
    const gateway = await startGateway({ maxParticipants: 1 });
    try {
      const first = await enrollWith(gateway.base, nextTestKey());
      const account = privateKeyToAccount(nextTestKey());
      const issuedResponse = await fetch(`${gateway.base}/session/enroll-challenge`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ address: account.address }),
      });
      assert.equal(issuedResponse.status, 201);
      const { challenge } = (await issuedResponse.json()) as { challenge: string };
      const signature = await account.signMessage({ message: challenge });
      const second = await fetch(`${gateway.base}/session/enroll`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ address: account.address, challenge, signature }),
      });
      assert.equal(second.status, 503);
      assert.equal(((await second.json()) as { error: string }).error, "session_full");

      // A public lobby read must not destroy the live participant, and the
      // newcomer must not have displaced it.
      const lobby = (await (await fetch(`${gateway.base}/session/lobby`)).json()) as { participants: number };
      assert.equal(lobby.participants, 1);
      const stillThere = await fetch(`${gateway.base}/session/challenges`, {
        headers: { authorization: `Bearer ${first.token}` },
      });
      assert.equal(stillThere.status, 200, "a live participant survives a lobby read at capacity");
    } finally {
      await stop(gateway.server);
    }
  });

  it("binds loopback unless the config names another host", async () => {
    const dir = await mkdtemp(join(tmpdir(), "fod-gateway-config-"));
    const path = join(dir, "config.json");
    const minimal = {
      port: 8787,
      providerId: "example-provider",
      origins: ["null"],
      sessionTtlSeconds: 1800,
      bodyLimitBytes: 4096,
      rateLimit: { windowSeconds: 60, maxRequests: 10 },
      poolPath: "pool.json",
      applicationKeyPath: "key.json",
    };
    await writeFile(path, JSON.stringify(minimal));
    assert.equal((await loadConfig(path)).host, "127.0.0.1", "the default bind is loopback");

    await writeFile(path, JSON.stringify({ ...minimal, host: "0.0.0.0" }));
    assert.equal((await loadConfig(path)).host, "0.0.0.0", "a network-facing bind is explicit");
  });

  it("gives an authenticated participant its own allowance", async () => {
    const gateway = await startGateway({ rateLimit: { windowSeconds: 60, maxRequests: 4 } });
    try {
      const token = await enroll(gateway.base);
      // Spend the anonymous allowance at this address (the enrollment itself
      // already used a few), and stop the moment it is exhausted.
      let limited = false;
      for (let attempt = 0; attempt < 10; attempt += 1) {
        const response = await fetch(`${gateway.base}/session/policy`);
        if (response.status === 429) {
          limited = true;
          break;
        }
        assert.equal(response.status, 200);
      }
      assert.ok(limited, "the anonymous allowance must be exhausted");

      // The participant's own bucket is untouched, so the session stays usable.
      for (let attempt = 0; attempt < 3; attempt += 1) {
        const response = await fetch(`${gateway.base}/session/challenges`, {
          headers: { authorization: `Bearer ${token}` },
        });
        assert.equal(response.status, 200, "an authenticated participant must not be starved by the address bucket");
      }
    } finally {
      await stop(gateway.server);
    }
  });

  it("enforces the published session window on every transition", async () => {
    let now = Date.parse("2025-12-31T23:59:59Z");
    const gateway = await startGateway({ ttlSeconds: 40_000_000, now: () => now });
    try {
      // Before the window a binding can be prepared, but enrollment still
      // refuses: the window gates every transition, not just the answers.
      const earlyAccount = privateKeyToAccount(nextTestKey());
      const earlyChallengeResponse = await fetch(`${gateway.base}/session/enroll-challenge`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ address: earlyAccount.address }),
      });
      assert.equal(earlyChallengeResponse.status, 201);
      const earlyIssued = (await earlyChallengeResponse.json()) as { challenge: string };
      const earlySignature = await earlyAccount.signMessage({ message: earlyIssued.challenge });
      const early = await fetch(`${gateway.base}/session/enroll`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          address: earlyAccount.address,
          challenge: earlyIssued.challenge,
          signature: earlySignature,
        }),
      });
      assert.equal(early.status, 403);
      assert.equal(((await early.json()) as { error: string }).error, "session_not_open");

      // Exactly at the opening instant the window is open.
      now = Date.parse("2026-01-01T00:00:00Z");
      const token = await enroll(gateway.base);
      const auth = { authorization: `Bearer ${token}`, "content-type": "application/json" };
      const first = await fetch(`${gateway.base}/session/answers`, {
        method: "POST",
        headers: auth,
        body: JSON.stringify({ challengeId: "c1", answer: "first answer" }),
      });
      assert.equal(first.status, 202);

      // A clock behind the opening instant must not produce a signed statement
      // or accept work, even with a token issued later.
      now = Date.parse("2025-12-31T23:59:59Z");
      const tooEarly = await fetch(`${gateway.base}/session/result`, { headers: auth });
      assert.equal(tooEarly.status, 403);
      assert.equal(((await tooEarly.json()) as { error: string }).error, "session_open");
      const earlyAnswer = await fetch(`${gateway.base}/session/answers`, {
        method: "POST",
        headers: auth,
        body: JSON.stringify({ challengeId: "c2", answer: "second answer" }),
      });
      assert.equal(earlyAnswer.status, 403);
      assert.equal(((await earlyAnswer.json()) as { error: string }).error, "session_not_open");

      // Exactly at the closing instant the window is closed.
      now = Date.parse("2027-01-01T00:00:00Z");
      const lateChallenge = await fetch(`${gateway.base}/session/enroll-challenge`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ address: privateKeyToAccount(nextTestKey()).address }),
      });
      assert.equal(lateChallenge.status, 410);
      assert.equal(((await lateChallenge.json()) as { error: string }).error, "session_closed");
      const lateAnswer = await fetch(`${gateway.base}/session/answers`, {
        method: "POST",
        headers: auth,
        body: JSON.stringify({ challengeId: "c2", answer: "second answer" }),
      });
      assert.equal(lateAnswer.status, 410);
      assert.equal(((await lateAnswer.json()) as { error: string }).error, "session_closed");

      // After the close, a participant who answered inside the window can still
      // collect the statement their own unexpired token is bound to.
      now = Date.parse("2027-01-02T00:00:00Z");
      const statement = await fetch(`${gateway.base}/session/result`, { headers: auth });
      assert.equal(statement.status, 200);
      assert.equal(((await statement.json()) as { format: string }).format, "fod.session-result");
    } finally {
      await stop(gateway.server);
    }
  });

  it("caps body size", async () => {
    const gateway = await startGateway({});
    try {
      const token = await enroll(gateway.base);
      const response = await fetch(`${gateway.base}/session/answers`, {
        method: "POST",
        headers: { authorization: `Bearer ${token}`, "content-type": "application/json" },
        body: JSON.stringify({ challengeId: "c1", answer: "x".repeat(10_000) }),
      });
      assert.equal(response.status, 413);
    } finally {
      await stop(gateway.server);
    }
  });

  it("requires JSON content type", async () => {
    const gateway = await startGateway({});
    try {
      const response = await fetch(`${gateway.base}/session/enroll`, {
        method: "POST",
        headers: { "content-type": "text/plain" },
        body: "{}",
      });
      assert.equal(response.status, 415);
    } finally {
      await stop(gateway.server);
    }
  });

  it("rate limits a client", async () => {
    const gateway = await startGateway({ rateLimit: { windowSeconds: 60, maxRequests: 3 } });
    try {
      for (let i = 0; i < 3; i += 1) {
        const ok = await fetch(`${gateway.base}/session/policy`);
        assert.equal(ok.status, 200);
      }
      const limited = await fetch(`${gateway.base}/session/policy`);
      assert.equal(limited.status, 429);
    } finally {
      await stop(gateway.server);
    }
  });

  it("expires a session", async () => {
    // Inside the pool's published window, so enrolment succeeds before expiry.
    let now = Date.parse("2026-06-01T00:00:00Z");
    const gateway = await startGateway({ ttlSeconds: 60, now: () => now });
    try {
      const token = await enroll(gateway.base);
      now += 61_000;
      const expired = await fetch(`${gateway.base}/session/challenges`, {
        headers: { authorization: `Bearer ${token}` },
      });
      assert.equal(expired.status, 410);
      assert.deepEqual(await expired.json(), { error: "session_expired" });
    } finally {
      await stop(gateway.server);
    }
  });

  it("sets CORS only for configured origins, and a file:// origin is not authorisation", async () => {
    const gateway = await startGateway({ origins: [] });
    try {
      const response = await fetch(`${gateway.base}/session/challenges`, {
        headers: { origin: "null" },
      });
      assert.equal(response.headers.get("access-control-allow-origin"), null);
      assert.equal(response.status, 401, "the origin must not stand in for a token");
    } finally {
      await stop(gateway.server);
    }

    const allowing = await startGateway({ origins: ["null"] });
    try {
      const response = await fetch(`${allowing.base}/session/challenges`, {
        headers: { origin: "null" },
      });
      assert.equal(response.headers.get("access-control-allow-origin"), "null");
      assert.notEqual(response.headers.get("access-control-allow-origin"), "*");
      assert.equal(response.status, 401, "an allowed origin still needs a token");
    } finally {
      await stop(allowing.server);
    }
  });

  it("answers the preflight a browser sends before a cross-origin POST", async () => {
    const allowing = await startGateway({ origins: ["null"] });
    try {
      const preflight = await fetch(`${allowing.base}/session/answers`, {
        method: "OPTIONS",
        headers: { origin: "null", "access-control-request-method": "POST" },
      });
      assert.equal(preflight.status, 204);
      assert.equal(preflight.headers.get("access-control-allow-origin"), "null");
      assert.ok(
        (preflight.headers.get("access-control-allow-headers") ?? "").includes("authorization"),
        "the preflight must allow the Authorization header",
      );
    } finally {
      await stop(allowing.server);
    }

    const disallowing = await startGateway({ origins: [] });
    try {
      const preflight = await fetch(`${disallowing.base}/session/answers`, {
        method: "OPTIONS",
        headers: { origin: "https://elsewhere.example" },
      });
      assert.equal(preflight.headers.get("access-control-allow-origin"), null);
    } finally {
      await stop(disallowing.server);
    }
  });
});
