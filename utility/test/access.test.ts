// The utility's rules under test.
//
// The results here are signed with the gateway's own signing helper and checked
// with the utility's independent verifier, so the two implementations have to
// agree on the canonical form - a utility that only trusts a result because the
// issuer's helper said so would prove nothing.

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import { generateKeyPairSync } from "node:crypto";
import { mkdtemp, stat, writeFile } from "node:fs/promises";
import type { AddressInfo } from "node:net";
import type { Server } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { privateKeyToAccount } from "viem/accounts";
import { canonicalJson as gatewayCanonical, signResult } from "../../gateway/src/keys.ts";
import { canonicalJson } from "../../app/src/provider.ts";
import { answerCommitmentText, answersDigest, canonicalAnswer, sha256Hex } from "../../app/src/score.ts";
import { AnswerCommitments } from "../src/commitments.ts";
import { decideAccess, FileGrants, InMemoryGrants } from "../src/access.ts";
import { loadPolicy, type UtilityPolicy } from "../src/policy.ts";
import { createUtilityServer } from "../src/server.ts";

const PROVIDER = "example-provider";
const UTILITY = "utility-example";
const SESSION = "session-example-1";
// The public Anvil test key: a fixed test wallet, not a secret.
const testWallet = privateKeyToAccount("0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d");

const { privateKey, publicKey } = generateKeyPairSync("ed25519");
const publicKeyHex = publicKey.export({ format: "der", type: "spki" }).toString("hex");

function policy(overrides: Partial<UtilityPolicy> = {}): UtilityPolicy {
  return {
    format: "fod.utility-policy",
    version: 1,
    utilityId: UTILITY,
    sessions: [SESSION],
    requireOutcome: "passed",
    grantsPerParticipant: 1,
    grantTtlSeconds: 3600,
    providers: [{ providerId: PROVIDER, publicKeyHex }],
    scope: {
      kind: "provider-participant",
      statement: "scoped to a provider-issued participant id, not to a person",
    },
    ...overrides,
  };
}

function signedResult(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const now = Date.now();
  const unsigned = {
    format: "fod.session-result",
    version: 1,
    providerId: PROVIDER,
    sessionId: SESSION,
    utilityId: UTILITY,
    // What the session was, as the provider's own policy published it.
    contentDigest: "71".repeat(32),
    participantId: "participant-1",
    issuedAt: new Date(now).toISOString(),
    expiresAt: new Date(now + 600_000).toISOString(),
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
    issuedBy: { kind: "provider", providerId: PROVIDER, note: "issued, not necessarily honest" },
    algorithm: "ed25519",
    ...overrides,
  };
  return { ...unsigned, signature: signResult(privateKey, gatewayCanonical(unsigned)) };
}

async function verifiedFixture(resultOverrides: Record<string, unknown> = {}) {
  const now = Date.now();
  const window = { opensAt: new Date(now - 120_000).toISOString(), closesAt: new Date(now - 60_000).toISOString() };
  const reveal = {
    sessionId: SESSION, utilityId: UTILITY, rules: { passThreshold: 1, ...window },
    answerSalt: "ab".repeat(32),
    challenges: [{ challengeId: "c1", cid: "cid-1", answerDigest: await sha256Hex("first answer"), contentSha256: null }],
  };
  const contentDigest = await sha256Hex(canonicalJson(reveal));
  const address = String(resultOverrides.participantAddress ?? testWallet.address);
  return {
    request: {
      result: signedResult({ contentDigest, participantAddress: address, ...resultOverrides }),
      scoreReveal: reveal, claim: { address }, commitment: { address, answers: [{ challengeId: "c1", answer: "first answer" }] },
    },
    policy: policy({ requireCommittedAnswers: true, sessionDigests: { [SESSION]: contentDigest }, sessionWindows: { [SESSION]: window } }),
  };
}

let grantCounter = 0;
const newGrantId = () => `grant-${(grantCounter += 1)}`;

describe("utility access decisions", () => {
  it("refuses signature-only grants, including the permissive default policy", async () => {
    for (const configured of [policy(), policy({ requireScoreProof: false }), policy({ requireCommittedAnswers: false })]) {
      const store = new InMemoryGrants();
      const decision = await decideAccess({ result: signedResult() }, configured, store, Date.now(), newGrantId);
      assert.equal(decision.granted ? "granted" : decision.reason, "score_unverifiable");
      assert.equal(store.count({ utilityId: UTILITY, providerId: PROVIDER, kind: "provider-participant", participantId: "participant-1" }, Date.now()), 0);
    }
  });

  it("refuses a tampered payload", async () => {
    const result = signedResult();
    result.outcome = "not-passed";
    const decision = await decideAccess({ result }, policy(), new InMemoryGrants(), Date.now(), newGrantId);
    assert.deepEqual(decision.granted ? {} : decision.reason, "bad_signature");
  });

  it("refuses a provider it does not trust, or one it has no key for", async () => {
    const unknown = await decideAccess(
      { result: signedResult({ providerId: "someone-else" }) },
      policy(),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(unknown.granted ? "" : unknown.reason, "untrusted_provider");

    const keyless = await decideAccess(
      { result: signedResult() },
      policy({ providers: [{ providerId: PROVIDER, publicKeyHex: "" }] }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(keyless.granted ? "" : keyless.reason, "untrusted_provider");
  });

  it("refuses the wrong utility, an unknown session, a failed outcome, and an expired result", async () => {
    const cases: [Record<string, unknown>, string][] = [
      [{ utilityId: "another-utility" }, "wrong_utility"],
      [{ sessionId: "session-elsewhere" }, "unknown_session"],
      [{ outcome: "not-passed" }, "outcome_not_accepted"],
      [{ expiresAt: new Date(Date.now() - 1000).toISOString() }, "result_expired"],
    ];
    for (const [override, expected] of cases) {
      const result = signedResult(override);
      const decision = await decideAccess({ result }, policy(), new InMemoryGrants(), Date.now(), newGrantId);
      assert.equal(decision.granted ? "granted" : decision.reason, expected);
    }
  });

  it("refuses a malformed result without guessing", async () => {
    for (const value of [null, 42, {}, { format: "fod.session-result" }, "not a result"]) {
      const decision = await decideAccess({ result: value }, policy(), new InMemoryGrants(), Date.now(), newGrantId);
      assert.equal(decision.granted ? "granted" : decision.reason, "malformed_result");
    }
  });

  it("does not grant against a provider-issued participant id without a wallet", async () => {
    const fixture = await verifiedFixture();
    const request = { ...fixture.request, result: signedResult({ contentDigest: fixture.request.result.contentDigest }) };
    const decision = await decideAccess(request, fixture.policy, new InMemoryGrants(), Date.now(), newGrantId);
    assert.equal(decision.granted ? "granted" : decision.reason, "answers_not_committed");
  });

  it("refuses a result that names a wallet without a verified claim", async () => {
    const address = testWallet.address;
    const missing = await decideAccess(
      { result: signedResult({ participantAddress: address }) },
      policy(),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(missing.granted ? "granted" : missing.reason, "claim_control_required");

    // A claim for another address does not stand in for the named one.
    const wrong = await decideAccess(
      {
        result: signedResult({ participantAddress: address }),
        claim: { address: `0x${"11".repeat(20)}` },
      },
      policy(),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(wrong.granted ? "granted" : wrong.reason, "claim_control_required");
  });

  it("keys independently scored grants by wallet so re-enrollment cannot reset the quota", async () => {
    const store = new InMemoryGrants();
    for (const [participantId, address, expected] of [
      ["participant-1", testWallet.address, "granted"],
      ["participant-2", testWallet.address, "quota_exhausted"],
      ["participant-3", privateKeyToAccount(`0x${"02".padStart(64, "0")}`).address, "granted"],
    ]) {
      const fixture = await verifiedFixture({ participantId, participantAddress: address });
      const decision = await decideAccess(fixture.request, fixture.policy, store, Date.now(), newGrantId);
      assert.equal(decision.granted ? "granted" : decision.reason, expected);
      if (decision.granted) assert.equal(decision.identity, "not-verified");
    }
  });

  it("persists a spent wallet grant across a reconstruction", async () => {
    const path = join(await mkdtemp(join(tmpdir(), "fod-grants-")), "grants.json");
    const fixture = await verifiedFixture();
    const store = await FileGrants.open(path);
    assert.equal((await decideAccess(fixture.request, fixture.policy, store, Date.now(), newGrantId)).granted, true);
    const reopened = await FileGrants.open(path);
    assert.equal(reopened.count({ utilityId: UTILITY, providerId: PROVIDER, kind: "wallet", address: testWallet.address }, Date.now()), 1);
    const again = await decideAccess(fixture.request, fixture.policy, reopened, Date.now(), newGrantId);
    assert.equal(again.granted ? "granted" : again.reason, "quota_exhausted");
    assert.equal((await stat(path)).mode & 0o777, 0o600);
  });

  it("refuses an identity-dependent policy instead of trusting an assertion", async () => {
    const strict = policy({
      identityVerification: {
        required: true,
        note: "one grant per identity, once chain-committed eligibility can be checked",
      },
    });

    // A valid, honestly signed result is still refused while the policy wants
    // an identity check this adapter cannot perform.
    const honest = await decideAccess({ result: signedResult() }, strict, new InMemoryGrants(), Date.now(), newGrantId);
    assert.equal(honest.granted ? "granted" : honest.reason, "identity_required");

    // And nothing a caller can put in the request turns that into a grant: the
    // boolean the adapter used to honour is not read at all any more.
    for (const assertion of [
      { result: signedResult(), identityVerified: true },
      { result: signedResult(), identityVerified: "true" },
      { result: signedResult(), identity: { verified: true } },
    ]) {
      const decision = await decideAccess(assertion, strict, new InMemoryGrants(), Date.now(), newGrantId);
      assert.equal(decision.granted ? "granted" : decision.reason, "identity_required");
    }
  });

  it("counts local grants per utility and provider and forgets expired entries", async () => {
    const store = new InMemoryGrants();
    const fixture = await verifiedFixture();
    const now = Date.now();
    assert.equal((await decideAccess(fixture.request, fixture.policy, store, now, newGrantId)).granted, true);
    const key = { utilityId: UTILITY, providerId: PROVIDER, kind: "wallet" as const, address: testWallet.address };
    assert.equal(store.count(key, now), 1);
    assert.equal(store.count({ ...key, providerId: "other-provider" }, now), 0);
    assert.equal(store.count({ ...key, utilityId: "other-utility" }, now), 0);
    assert.equal(store.count(key, now + 3_600_001), 0);
  });

  it("refuses a session id the policy pinned to different content", async () => {
    const pinned = policy({ sessionDigests: { [SESSION]: "71".repeat(32) } });
    const matching = await decideAccess({ result: signedResult() }, pinned, new InMemoryGrants(), Date.now(), newGrantId);
    assert.equal(matching.granted ? "granted" : matching.reason, "score_unverifiable");

    const otherContent = await decideAccess(
      { result: signedResult({ contentDigest: "72".repeat(32) }) },
      pinned,
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(otherContent.granted ? "granted" : otherContent.reason, "wrong_session_content");

    // An unpinned session cannot authorize a grant.
    const unpinned = policy();
    const accepted = await decideAccess(
      { result: signedResult({ contentDigest: "72".repeat(32) }) },
      unpinned,
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(accepted.granted ? "granted" : accepted.reason, "score_unverifiable");
  });

  it("refuses a result that names no session content", async () => {
    const result = signedResult();
    delete result.contentDigest;
    const decision = await decideAccess({ result }, policy(), new InMemoryGrants(), Date.now(), newGrantId);
    assert.equal(decision.granted ? "granted" : decision.reason, "malformed_result");
  });
});

describe("utility endpoint", () => {
  let server: Server;
  let base: string;

  before(async () => {
    server = createUtilityServer({
      policy: policy(),
      rateLimit: { windowSeconds: 60, maxRequests: 5 },
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address() as AddressInfo;
    base = `http://127.0.0.1:${address.port}`;
  });

  after(async () => {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });

  it("publishes its policy and decides on a posted result", async () => {
    const policyResponse = await fetch(`${base}/policy`);
    assert.equal(policyResponse.status, 200);
    const published = (await policyResponse.json()) as {
      scope: { statement: string };
      quota: { scope: string; durable: boolean };
      identityVerification: null;
      sessionDigests: null | Record<string, string>;
    };
    assert.ok(published.scope.statement.includes("not to a person"));
    // The published limit says what it is: this process, not the world.
    assert.equal(published.quota.scope, "local-process");
    assert.equal(published.quota.durable, false);
    assert.equal(published.identityVerification, null);
    // No session content is pinned in this policy, and the endpoint says so.
    assert.equal(published.sessionDigests, null);

    const granted = await fetch(`${base}/access`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ result: signedResult() }),
    });
    assert.equal(granted.status, 403);
    const decision = (await granted.json()) as { granted: boolean; reason: string };
    assert.equal(decision.reason, "score_unverifiable");

    const refused = await fetch(`${base}/access`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ result: signedResult({ sessionId: "session-elsewhere" }) }),
    });
    assert.equal(refused.status, 403);
    assert.equal(((await refused.json()) as { reason: string }).reason, "unknown_session");
  });

  it("has no other surface", async () => {
    assert.equal((await fetch(`${base}/admin`)).status, 404);
    assert.equal((await fetch(`${base}/access`)).status, 405);
    assert.equal((await fetch(`${base}/access`, { method: "POST", headers: { "content-type": "text/plain" }, body: "{}" })).status, 415);
  });
});

// `SPEC.md` 8.3's CORS rule: the reference utility pins its browser origins, and
// a policy that names none gets no CORS header at all, so a hosted utility
// cannot inherit a wildcard by leaving the field out.
describe("utility origin pin", () => {
  const withServer = async (overrides: Partial<UtilityPolicy>, run: (base: string) => Promise<void>) => {
    const server = createUtilityServer({ policy: policy(overrides) });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address() as AddressInfo;
    try {
      await run(`http://127.0.0.1:${address.port}`);
    } finally {
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  };

  it("echoes a pinned origin and sends nothing to another one", async () => {
    await withServer({ origins: ["null"] }, async (base) => {
      const pinned = await fetch(`${base}/policy`, { headers: { origin: "null" } });
      assert.equal(pinned.headers.get("access-control-allow-origin"), "null");
      assert.equal(pinned.headers.get("vary"), "Origin");
      const elsewhere = await fetch(`${base}/policy`, { headers: { origin: "https://dapp.example" } });
      assert.equal(elsewhere.headers.get("access-control-allow-origin"), null);
      // Non-browser callers are unaffected: no Origin header, no CORS header.
      const direct = await fetch(`${base}/policy`);
      assert.equal(direct.headers.get("access-control-allow-origin"), null);
      assert.equal(direct.status, 200);
    });
  });

  it("sends no CORS header when the policy names no origin", async () => {
    await withServer({}, async (base) => {
      const response = await fetch(`${base}/policy`, { headers: { origin: "null" } });
      assert.equal(response.headers.get("access-control-allow-origin"), null);
      assert.equal(response.status, 200);
    });
  });

  it("keeps an explicit public testbed wildcard", async () => {
    await withServer({ origins: ["*"] }, async (base) => {
      const response = await fetch(`${base}/policy`, { headers: { origin: "https://dapp.example" } });
      assert.equal(response.headers.get("access-control-allow-origin"), "*");
    });
  });

  it("publishes the pin it is enforcing", async () => {
    await withServer({ origins: ["null", "https://dapp.example"] }, async (base) => {
      const published = (await (await fetch(`${base}/policy`)).json()) as { origins: string[] | null };
      assert.deepEqual(published.origins, ["null", "https://dapp.example"]);
    });
    await withServer({}, async (base) => {
      const published = (await (await fetch(`${base}/policy`)).json()) as { origins: string[] | null };
      assert.equal(published.origins, null);
    });
  });
});

describe("utility claim challenges", () => {
  let server: Server;
  let base: string;

  before(async () => {
    server = createUtilityServer({ policy: policy(), rateLimit: { windowSeconds: 60, maxRequests: 50 } });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  });

  after(async () => {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });

  it("consumes a control nonce once without treating it as evidence of solving", async () => {
    const address = testWallet.address;
    const result = signedResult({ participantAddress: address });
    const post = (body: unknown) =>
      fetch(`${base}/access`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(body),
      });

    const bare = await post({ result });
    assert.equal(bare.status, 403);
    assert.equal(((await bare.json()) as { reason: string }).reason, "claim_control_required");

    const challengeResponse = await fetch(`${base}/access-challenge`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ address }),
    });
    assert.equal(challengeResponse.status, 201);
    const issued = (await challengeResponse.json()) as { challenge: string; expiresAt: string };
    const signature = await testWallet.signMessage({ message: issued.challenge });
    const proof = { address, challenge: issued.challenge, signature };

    const granted = await post({ result, ...proof });
    assert.equal(granted.status, 403);
    assert.equal(((await granted.json()) as { reason: string }).reason, "score_unverifiable");

    // The same challenge is worth exactly one claim.
    const replay = await post({
      result: signedResult({ participantId: "participant-9", participantAddress: address }),
      ...proof,
    });
    assert.equal(replay.status, 403);
    assert.equal(((await replay.json()) as { reason: string }).reason, "unknown_nonce");
  });
});

describe("utility rate limits behind a proxy", () => {
  it("charges the forwarded client only when the proxy is trusted", async () => {
    const post = (base: string, address: string, participantId: string) =>
      fetch(`${base}/access`, {
        method: "POST",
        headers: { "content-type": "application/json", "x-forwarded-for": address },
        body: JSON.stringify({ result: signedResult({ participantId }) }),
      });

    const trusted = createUtilityServer({
      policy: policy(),
      rateLimit: { windowSeconds: 60, maxRequests: 1 },
      trustedProxies: ["127.0.0.1"],
    });
    await new Promise<void>((resolve) => trusted.listen(0, "127.0.0.1", resolve));
    const trustedBase = `http://127.0.0.1:${(trusted.address() as AddressInfo).port}`;
    try {
      assert.equal((await post(trustedBase, "198.51.100.7", "participant-a")).status, 403);
      assert.equal((await post(trustedBase, "198.51.100.7", "participant-b")).status, 429);
      assert.equal(
        (await post(trustedBase, "198.51.100.8", "participant-c")).status,
        403,
        "another client behind the proxy has its own bucket",
      );
    } finally {
      await new Promise<void>((resolve) => trusted.close(() => resolve()));
    }

    const direct = createUtilityServer({
      policy: policy(),
      rateLimit: { windowSeconds: 60, maxRequests: 1 },
    });
    await new Promise<void>((resolve) => direct.listen(0, "127.0.0.1", resolve));
    const directBase = `http://127.0.0.1:${(direct.address() as AddressInfo).port}`;
    try {
      assert.equal((await post(directBase, "198.51.100.7", "participant-d")).status, 403);
      assert.equal(
        (await post(directBase, "198.51.100.8", "participant-e")).status,
        429,
        "an untrusted forwarded header must not mint fresh buckets",
      );
    } finally {
      await new Promise<void>((resolve) => direct.close(() => resolve()));
    }
  });
});

describe("a policy that requires identity eligibility", () => {
  let server: Server;
  let base: string;

  before(async () => {
    server = createUtilityServer({
      policy: policy({
        identityVerification: {
          required: true,
          note: "one grant per identity, once chain-committed eligibility can be checked",
        },
      }),
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address() as AddressInfo;
    base = `http://127.0.0.1:${address.port}`;
  });

  after(async () => {
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });

  it("publishes that it holds no identity check instead of implying one", async () => {
    const published = (await (await fetch(`${base}/policy`)).json()) as {
      identityVerification: { required: boolean; supported: boolean; note: string };
    };
    assert.equal(published.identityVerification.required, true);
    assert.equal(published.identityVerification.supported, false);
    assert.ok(published.identityVerification.note.length > 0);
  });

  it("refuses a signed result, and request data cannot turn that into a grant", async () => {
    const bodies = [
      { result: signedResult() },
      { result: signedResult(), identityVerified: true },
      { result: signedResult(), identityVerified: "true", participant: { verified: true } },
    ];
    for (const body of bodies) {
      const response = await fetch(`${base}/access`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(body),
      });
      assert.equal(response.status, 403, JSON.stringify(body).slice(0, 60));
      assert.equal(((await response.json()) as { reason: string }).reason, "identity_required");
    }
  });
});

// The utility recomputes the score from the provider's own signed transcript
// and the answer key the pinned content carries (`SPEC.md` 8.3). It never takes
// the provider's arithmetic on its word, and a policy can require that a grant
// always carries that recomputation.
describe("a policy that requires a recomputed score", () => {
  async function scoreFixture() {
    const digestOf = async (answer: string) => sha256Hex(canonicalAnswer(answer));
    const reveal = {
      sessionId: SESSION,
      utilityId: UTILITY,
      rules: { passThreshold: 2, opensAt: "2026-10-01T00:00:00Z", closesAt: "2026-10-01T00:10:00Z" },
      provenance: { epoch: 228, author: `0x${"11".repeat(20)}`, source: "epoch 228", blockRange: "1-2" },
      answerSalt: "ab".repeat(32),
      challenges: [
        { challengeId: "c1", cid: "cid-1", answerDigest: await digestOf("first answer"), contentSha256: null },
        { challengeId: "c2", cid: "cid-2", answerDigest: await digestOf("second answer"), contentSha256: null },
      ],
    };
    return {
      reveal,
      contentDigest: await sha256Hex(canonicalJson(reveal)),
      honest: [
        { challengeId: "c1", answerDigest: await digestOf("first answer") },
        { challengeId: "c2", answerDigest: await digestOf("second answer") },
      ],
      // The provider says it scored a second answer c2 never matched: the
      // transcript exists, the signature is valid, and the arithmetic is not.
      inflated: [
        { challengeId: "c1", answerDigest: await digestOf("first answer") },
        { challengeId: "c2", answerDigest: "ab".repeat(32) },
      ],
    };
  }

  it("refuses a grant that carries no recomputable score", async () => {
    const { contentDigest } = await scoreFixture();
    const decision = await decideAccess(
      { result: signedResult({ contentDigest }) },
      policy({ sessionDigests: { [SESSION]: contentDigest }, requireScoreProof: true }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(decision.granted, false);
    assert.equal(decision.granted ? "" : decision.reason, "score_unverifiable");
  });

  it("refuses a provider-invented transcript even when its arithmetic is correct", async () => {
    const { reveal, contentDigest, honest } = await scoreFixture();
    const decision = await decideAccess(
      {
        result: signedResult({ contentDigest, transcript: { rule: "sha256 of the canonical answer text", entries: honest } }),
        scoreReveal: reveal,
      },
      policy({ sessionDigests: { [SESSION]: contentDigest }, requireScoreProof: true }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(decision.granted ? "granted" : decision.reason, "score_unverifiable");
  });

  it("refuses a signed score its arithmetic does not reproduce", async () => {
    const { reveal, contentDigest, inflated } = await scoreFixture();
    const decision = await decideAccess(
      {
        result: signedResult({
          contentDigest,
          transcript: { rule: "sha256 of the canonical answer text", entries: inflated },
        }),
        scoreReveal: reveal,
      },
      policy({ sessionDigests: { [SESSION]: contentDigest }, requireScoreProof: true }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(decision.granted, false);
    assert.equal(decision.granted ? "" : decision.reason, "score_mismatch");
    assert.match(decision.granted ? "" : (decision.detail ?? ""), /recomputes 1/);
  });

  it("refuses a reveal that does not hash to the pin, even when the proof is optional", async () => {
    const { reveal, contentDigest, honest } = await scoreFixture();
    const decision = await decideAccess(
      {
        result: signedResult({ contentDigest, transcript: { entries: honest } }),
        scoreReveal: { ...reveal, answerSalt: "cd".repeat(32) },
      },
      policy({ sessionDigests: { [SESSION]: contentDigest } }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(decision.granted, false);
    assert.equal(decision.granted ? "" : decision.reason, "score_mismatch");
    assert.match(decision.granted ? "" : (decision.detail ?? ""), /does not hash to the digest/);
  });

  it("refuses the old signature-only policy", async () => {
    const decision = await decideAccess(
      { result: signedResult() },
      policy(),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(decision.granted ? "granted" : decision.reason, "score_unverifiable");
  });
});

// A utility that witnessed the answers itself does not need the provider's
// transcript or its verdict: it scores the session with its own arithmetic over
// what the participant committed while the window was open and the answer key
// the pinned content reveals (`SPEC.md` 8.3).
describe("a policy that scores the session itself", () => {
  async function answersFixture() {
    const reveal = {
      sessionId: SESSION,
      utilityId: UTILITY,
      rules: { passThreshold: 2, opensAt: new Date(Date.now() - 120_000).toISOString(), closesAt: new Date(Date.now() - 60_000).toISOString() },
      provenance: { epoch: 228, author: `0x${"11".repeat(20)}`, source: "epoch 228", blockRange: "1-2" },
      answerSalt: "ab".repeat(32),
      challenges: [
        { challengeId: "c1", cid: "cid-1", answerDigest: await sha256Hex(canonicalAnswer("first answer")), contentSha256: null },
        { challengeId: "c2", cid: "cid-2", answerDigest: await sha256Hex(canonicalAnswer("second answer")), contentSha256: null },
      ],
    };
    return {
      reveal,
      contentDigest: await sha256Hex(canonicalJson(reveal)),
      passing: [
        { challengeId: "c1", answer: "first answer" },
        { challengeId: "c2", answer: "second answer" },
      ],
      failing: [
        { challengeId: "c1", answer: "first answer" },
        { challengeId: "c2", answer: "not the second answer" },
      ],
    };
  }

  it("grants on its own arithmetic even when the provider's verdict says otherwise", async () => {
    const { reveal, contentDigest, passing } = await answersFixture();
    const result = signedResult({
      contentDigest,
      participantAddress: testWallet.address,
      outcome: "not-passed",
      score: { assigned: 2, answered: 2, correct: 0, passThreshold: 2 },
    });
    const decision = await decideAccess(
      {
        result,
        claim: { address: testWallet.address },
        scoreReveal: reveal,
        commitment: { address: testWallet.address, answers: passing },
      },
      policy({ sessionDigests: { [SESSION]: contentDigest }, requireCommittedAnswers: true, sessionWindows: { [SESSION]: reveal.rules } }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(decision.granted, true);
    assert.deepEqual(decision.granted ? decision.scoring : null, {
      by: "utility",
      assigned: 2,
      answered: 2,
      correct: 2,
      passThreshold: 2,
    });
  });

  it("refuses a pass the committed answers do not earn", async () => {
    const { reveal, contentDigest, failing } = await answersFixture();
    const decision = await decideAccess(
      {
        result: signedResult({ contentDigest, participantAddress: testWallet.address }),
        claim: { address: testWallet.address },
        scoreReveal: reveal,
        commitment: { address: testWallet.address, answers: failing },
      },
      policy({ sessionDigests: { [SESSION]: contentDigest }, requireCommittedAnswers: true, sessionWindows: { [SESSION]: reveal.rules } }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(decision.granted, false);
    assert.equal(decision.granted ? "" : decision.reason, "score_not_passed");
    assert.match(decision.granted ? "" : (decision.detail ?? ""), /score 1 of 2/);
  });

  it("refuses a grant with no commitment, or one made for another address", async () => {
    const { reveal, contentDigest, passing } = await answersFixture();
    const request = {
      result: signedResult({ contentDigest, participantAddress: testWallet.address }),
      claim: { address: testWallet.address },
      scoreReveal: reveal,
    };
    const missing = await decideAccess(
      request,
      policy({ sessionDigests: { [SESSION]: contentDigest }, requireCommittedAnswers: true, sessionWindows: { [SESSION]: reveal.rules } }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(missing.granted ? "" : missing.reason, "answers_not_committed");

    const swapped = await decideAccess(
      {
        ...request,
        commitment: { address: `0x${"22".repeat(20)}`, answers: passing },
      },
      policy({ sessionDigests: { [SESSION]: contentDigest }, requireCommittedAnswers: true, sessionWindows: { [SESSION]: reveal.rules } }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(swapped.granted ? "" : swapped.reason, "answers_not_committed");
  });

  it("refuses a reveal that does not hash to the pin, and an unpinned session", async () => {
    const { reveal, contentDigest, passing } = await answersFixture();
    const tampered = await decideAccess(
      {
        result: signedResult({ contentDigest, participantAddress: testWallet.address }),
        claim: { address: testWallet.address },
        scoreReveal: { ...reveal, answerSalt: "cd".repeat(32) },
        commitment: { address: testWallet.address, answers: passing },
      },
      policy({ sessionDigests: { [SESSION]: contentDigest }, requireCommittedAnswers: true, sessionWindows: { [SESSION]: reveal.rules } }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(tampered.granted ? "" : tampered.reason, "score_mismatch");

    const unpinned = await decideAccess(
      {
        result: signedResult({ contentDigest, participantAddress: testWallet.address }),
        claim: { address: testWallet.address },
        scoreReveal: reveal,
        commitment: { address: testWallet.address, answers: passing },
      },
      policy({ requireCommittedAnswers: true, sessionWindows: { [SESSION]: reveal.rules } }),
      new InMemoryGrants(),
      Date.now(),
      newGrantId,
    );
    assert.equal(unpinned.granted ? "" : unpinned.reason, "score_unverifiable");
    assert.match(unpinned.granted ? "" : (unpinned.detail ?? ""), /pin the session content/);
  });
});

// The endpoint that takes those commitments: only while the pinned window is
// open, only with a signature over the exact answers, and never into a session
// this utility does not know.
describe("the answer commitment endpoint", () => {
  async function startServer(window: { opensAt: string; closesAt: string }, now: () => number = Date.now) {
    const reveal = {
      sessionId: SESSION,
      utilityId: UTILITY,
      rules: { passThreshold: 2, ...window },
      provenance: { epoch: 228, author: `0x${"11".repeat(20)}`, source: "epoch 228", blockRange: "1-2" },
      answerSalt: "ab".repeat(32),
      challenges: [
        {
          challengeId: "c1",
          cid: "cid-1",
          answerDigest: await sha256Hex(canonicalAnswer("first answer")),
          contentSha256: null,
        },
        {
          challengeId: "c2",
          cid: "cid-2",
          answerDigest: await sha256Hex(canonicalAnswer("second answer")),
          contentSha256: null,
        },
      ],
    };
    const contentDigest = await sha256Hex(canonicalJson(reveal));
    const server = createUtilityServer({
      now,
      policy: policy({
        sessionDigests: { [SESSION]: contentDigest },
        sessionWindows: { [SESSION]: window },
        requireCommittedAnswers: true,
      }),
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address() as AddressInfo;
    return { server, base: `http://127.0.0.1:${address.port}`, reveal, contentDigest };
  }

  const answers = [
    { challengeId: "c1", answer: "first answer" },
    { challengeId: "c2", answer: "second answer" },
  ];

  it("grants only after witnessed answers, close, and fresh wallet control; replay cannot grant twice", async () => {
    let now = Date.now();
    const started = await startServer({ opensAt: new Date(now - 1000).toISOString(), closesAt: new Date(now + 1000).toISOString() }, () => now);
    const post = (path: string, body: unknown) => fetch(`${started.base}/${path}`, {
      method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body),
    });
    try {
      const signature = await testWallet.signMessage({ message: answerCommitmentText({
        utilityId: UTILITY, sessionId: SESSION, address: testWallet.address, issuedAt: now, digest: await answersDigest(answers),
      }) });
      assert.equal((await post("answer-commitment", { address: testWallet.address, sessionId: SESSION, answers, issuedAt: now, signature })).status, 202);
      const older = answers.slice(0, 1);
      const olderSignature = await testWallet.signMessage({ message: answerCommitmentText({
        utilityId: UTILITY, sessionId: SESSION, address: testWallet.address, issuedAt: now - 1, digest: await answersDigest(older),
      }) });
      const rollback = await post("answer-commitment", { address: testWallet.address, sessionId: SESSION, answers: older, issuedAt: now - 1, signature: olderSignature });
      assert.equal(rollback.status, 403);
      assert.equal(((await rollback.json()) as { error: string }).error, "stale_commitment");
      now += 1000;
      const result = signedResult({ contentDigest: started.contentDigest, participantAddress: testWallet.address });
      const bare = await post("access", { result, scoreReveal: started.reveal, claim: { address: testWallet.address } });
      assert.equal(bare.status, 403, "caller-asserted control cannot authorize a copied result");
      const challenge = await (await post("access-challenge", { address: testWallet.address })).json() as { challenge: string };
      const body = { result, scoreReveal: started.reveal, address: testWallet.address, challenge: challenge.challenge,
        signature: await testWallet.signMessage({ message: challenge.challenge }) };
      const granted = await post("access", body);
      assert.equal(granted.status, 200);
      assert.equal(((await granted.json()) as { granted: boolean }).granted, true);
      const replay = await post("access", body);
      assert.equal(replay.status, 403);
      assert.equal(((await replay.json()) as { reason: string }).reason, "unknown_nonce");
    } finally {
      await new Promise<void>((resolve) => started.server.close(() => resolve()));
    }
  });

  it("keeps the latest commitment from one wallet, per utility and session", async () => {
    const store = new AnswerCommitments();
    const now = Date.now();
    const window = {
      opensAt: new Date(now - 60_000).toISOString(),
      closesAt: new Date(now + 10 * 60_000).toISOString(),
    };
    for (const set of [[{ challengeId: "c1", answer: "first answer" }], answers]) {
      const issuedAt = Date.now();
      const signature = await testWallet.signMessage({
        message: answerCommitmentText({
          utilityId: UTILITY,
          sessionId: SESSION,
          address: testWallet.address,
          issuedAt,
          digest: await answersDigest(set),
        }),
      });
      const recorded = await store.record({
        utilityId: UTILITY,
        window,
        proof: { address: testWallet.address, sessionId: SESSION, answers: set, issuedAt, signature },
        now: Date.now(),
      });
      assert.equal(recorded.ok, true);
    }
    assert.equal(store.get(UTILITY, SESSION, testWallet.address, Date.now())?.answers.length, 2);
    // A commitment is bound to the utility it names.
    assert.equal(store.get("utility-other", SESSION, testWallet.address, Date.now()), null);
    // And one that outlived its retention is forgotten, not handed out.
    assert.equal(store.get(UTILITY, SESSION, testWallet.address, now + 80 * 60 * 1000), null);
  });

  it("bounds what a flood of committing wallets can store", async () => {
    const store = new AnswerCommitments({ maxEntries: 50, maxBytes: 2_048 });
    const now = Date.now();
    const window = {
      opensAt: new Date(now - 60_000).toISOString(),
      closesAt: new Date(now + 600_000).toISOString(),
    };
    let refusal: string | null = null;
    let firstAccount: ReturnType<typeof privateKeyToAccount> | null = null;
    let firstAddress = "";
    for (let attempt = 0; attempt < 20 && refusal === null; attempt += 1) {
      const account = privateKeyToAccount(`0x${(0x1000 + attempt).toString(16).padStart(64, "0")}`);
      if (firstAccount === null) {
        firstAccount = account;
        firstAddress = account.address;
      }
      const answers = [{ challengeId: "c1", answer: `${attempt}`.padEnd(400, "y") }];
      const issuedAt = Date.now();
      const signature = await account.signMessage({
        message: answerCommitmentText({
          utilityId: UTILITY,
          sessionId: SESSION,
          address: account.address,
          issuedAt,
          digest: await answersDigest(answers),
        }),
      });
      const recorded = await store.record({
        utilityId: UTILITY,
        window,
        proof: { address: account.address, sessionId: SESSION, answers, issuedAt, signature },
        now: Date.now(),
      });
      if (!recorded.ok) {
        refusal = recorded.error;
      }
    }
    assert.equal(refusal, "busy");

    // A wallet that already committed may still replace its own commitment: the
    // budget bounds a flood, not a participant correcting its answers.
    const replacement = [{ challengeId: "c1", answer: "corrected".padEnd(400, "z") }];
    const issuedAt = Date.now();
    const signature = await firstAccount!.signMessage({
      message: answerCommitmentText({
        utilityId: UTILITY,
        sessionId: SESSION,
        address: firstAddress,
        issuedAt,
        digest: await answersDigest(replacement),
      }),
    });
    const recorded = await store.record({
      utilityId: UTILITY,
      window,
      proof: { address: firstAddress, sessionId: SESSION, answers: replacement, issuedAt, signature },
      now: Date.now(),
    });
    assert.equal(recorded.ok, true);
  });

  it("records a commitment signed in the window and refuses one that is not", async () => {
    const now = Date.now();
    const open = await startServer({
      opensAt: new Date(now - 60_000).toISOString(),
      closesAt: new Date(now + 10 * 60_000).toISOString(),
    });
    try {
      const issuedAt = Date.now();
      const signature = await testWallet.signMessage({
        message: answerCommitmentText({
          utilityId: UTILITY,
          sessionId: SESSION,
          address: testWallet.address,
          issuedAt,
          digest: await answersDigest(answers),
        }),
      });
      const accepted = await fetch(`${open.base}/answer-commitment`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ address: testWallet.address, sessionId: SESSION, answers, issuedAt, signature }),
      });
      assert.equal(accepted.status, 202);
      const recorded = (await accepted.json()) as { committed: boolean; digest: string };
      assert.equal(recorded.committed, true);
      assert.equal(recorded.digest, await answersDigest(answers));

      // The same body against a window that already closed is refused: the
      // commitment has to be witnessed while the session is open.
      const closed = await startServer({
        opensAt: new Date(now - 20 * 60_000).toISOString(),
        closesAt: new Date(now - 60_000).toISOString(),
      });
      try {
        const late = await fetch(`${closed.base}/answer-commitment`, {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ address: testWallet.address, sessionId: SESSION, answers, issuedAt, signature }),
        });
        assert.equal(late.status, 403);
        assert.equal(((await late.json()) as { error: string }).error, "outside_window");
      } finally {
        await new Promise<void>((resolve) => closed.server.close(() => resolve()));
      }
    } finally {
      await new Promise<void>((resolve) => open.server.close(() => resolve()));
    }
  });

  it("refuses answers whose signature does not cover them, and an unknown session", async () => {
    const now = Date.now();
    const started = await startServer({
      opensAt: new Date(now - 60_000).toISOString(),
      closesAt: new Date(now + 10 * 60_000).toISOString(),
    });
    try {
      const issuedAt = Date.now();
      // A signature over a different answer set proves nothing about these.
      const signature = await testWallet.signMessage({
        message: answerCommitmentText({
          utilityId: UTILITY,
          sessionId: SESSION,
          address: testWallet.address,
          issuedAt,
          digest: await answersDigest([{ challengeId: "c1", answer: "something else" }]),
        }),
      });
      const mismatched = await fetch(`${started.base}/answer-commitment`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ address: testWallet.address, sessionId: SESSION, answers, issuedAt, signature }),
      });
      assert.equal(mismatched.status, 400);
      assert.equal(((await mismatched.json()) as { error: string }).error, "bad_signature");

      const unknown = await fetch(`${started.base}/answer-commitment`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          address: testWallet.address,
          sessionId: "session-elsewhere",
          answers,
          issuedAt,
          signature,
        }),
      });
      assert.equal(unknown.status, 404);
      assert.equal(((await unknown.json()) as { error: string }).error, "unknown_session");

      // The policy publishes what it enforces, so an operator can see it.
      const published = (await (await fetch(`${started.base}/policy`)).json()) as {
        requireCommittedAnswers: boolean;
        sessionWindows: Record<string, { closesAt: string }> | null;
      };
      assert.equal(published.requireCommittedAnswers, true);
      assert.equal(published.sessionWindows?.[SESSION]?.closesAt, started.reveal.rules.closesAt);
    } finally {
      await new Promise<void>((resolve) => started.server.close(() => resolve()));
    }
  });
});


describe("utility policy safety", () => {
  it("refuses policy files without committed answers, pins, or windows", async () => {
    const fixture = await verifiedFixture();
    const path = join(await mkdtemp(join(tmpdir(), "fod-policy-")), "policy.json");
    for (const change of [{ requireCommittedAnswers: undefined }, { requireCommittedAnswers: false }, { sessionDigests: undefined }, { sessionWindows: undefined }]) {
      await writeFile(path, JSON.stringify({ ...fixture.policy, ...change }));
      await assert.rejects(loadPolicy(path), /grants require/);
    }
    await writeFile(path, JSON.stringify(fixture.policy));
    assert.equal((await loadPolicy(path)).requireCommittedAnswers, true);
  });
  it("refuses scoring before close and a mismatched utility/window reveal", async () => {
    const fixture = await verifiedFixture();
    const early = await decideAccess(fixture.request, fixture.policy, new InMemoryGrants(), Date.parse(fixture.policy.sessionWindows![SESSION].opensAt), newGrantId);
    assert.equal(early.granted ? "granted" : early.reason, "session_not_closed");
    const mismatched = await decideAccess({ ...fixture.request, scoreReveal: { ...fixture.request.scoreReveal, utilityId: "other" } }, fixture.policy, new InMemoryGrants(), Date.now(), newGrantId);
    assert.equal(mismatched.granted ? "granted" : mismatched.reason, "score_mismatch");
  });
});


describe("local grant store capacity", () => {
  it("refuses new keys without forgetting consumed grants, in memory and after restart", async () => {
    const now = Date.now();
    const expiresAt = new Date(now + 60_000).toISOString();
    const memory = new InMemoryGrants();
    const entries: Record<string, unknown> = {};
    for (let i = 0; i < 10_000; i++) {
      const key = { utilityId: UTILITY, providerId: PROVIDER, kind: "wallet" as const, address: `wallet-${i}` };
      memory.record(key, `grant-${i}`, expiresAt, now);
      entries[JSON.stringify([UTILITY, PROVIDER, "wallet", key.address])] = [{ grantId: `grant-${i}`, expiresAt: now + 60_000 }];
    }
    const path = join(await mkdtemp(join(tmpdir(), "fod-capacity-")), "grants.json");
    await writeFile(path, JSON.stringify({ format: FileGrants.format, version: FileGrants.version, grants: entries }), { mode: 0o600 });
    const durable = await FileGrants.open(path);
    const old = { utilityId: UTILITY, providerId: PROVIDER, kind: "wallet" as const, address: "wallet-0" };
    const fresh = { ...old, address: "wallet-new" };
    for (const store of [memory, durable]) {
      assert.throws(() => store.record(fresh, "new", expiresAt, now), /no live grants were evicted/);
      assert.equal(store.count(old, now), 1);
      assert.equal(store.count(fresh, now), 0);
      store.record(fresh, "new", new Date(now + 120_000).toISOString(), now + 60_000);
      assert.equal(store.count(old, now + 60_000), 0);
      assert.equal(store.count(fresh, now + 60_000), 1);
    }
    assert.equal((await FileGrants.open(path)).count(fresh, now + 60_000), 1);
  });
});
