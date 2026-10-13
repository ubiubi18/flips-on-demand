import { test } from "node:test";
import assert from "node:assert/strict";
import { generateKeyPairSync, sign } from "node:crypto";
import { GatewayClient, checkedWalletChallenge, parseProviderConfig, requestJSON, canonicalJson, verifyPinnedProviderResult, type ProviderConfig, type SessionResult } from "../src/provider.ts";

test("imported provider URLs require HTTPS outside loopback", () => {
  const connection = (gatewayUrl: string, utilityUrl?: string) => JSON.stringify({
    format: "fod.provider-config", version: 1, providerId: "test-provider", network: "lab",
    gatewayUrl, utilityUrl, protocol: "fod.session.v1",
  });
  for (const gateway of ["http://node.example", "http://127.evil.invalid", "http://127.0.0.1.evil.invalid"]) {
    const parsed = parseProviderConfig(connection(gateway));
    assert.equal(parsed.ok, false);
    if (!parsed.ok) assert.match(parsed.error, /gateway must use https/);
  }
  const utility = parseProviderConfig(connection("https://node.example", "http://utility.example"));
  assert.equal(utility.ok, false);
  if (!utility.ok) assert.match(utility.error, /utility must use https/);
  for (const gateway of ["http://127.0.0.1:8080", "http://127.0.0.2:8080", "http://[::1]:8080", "http://localhost:8080", "https://node.example"]) {
    assert.equal(parseProviderConfig(connection(gateway)).ok, true, gateway);
  }
  assert.equal(parseProviderConfig(connection("https://node.example", "https://utility.example")).ok, true);
  assert.equal(parseProviderConfig(connection("http://127.0.0.1", "http://localhost:8081")).ok, true);
  const localAlias = parseProviderConfig(connection("http://localhost:8080", "http://localhost:8081"));
  assert.equal(localAlias.ok, true);
  if (localAlias.ok) {
    assert.equal(localAlias.config.gatewayUrl, "http://127.0.0.1:8080");
    assert.equal(localAlias.config.utilityUrl, "http://127.0.0.1:8081");
  }
});

test("a gateway's own key cannot verify a pinless connection or substituted identity", async () => {
  const { privateKey, publicKey } = generateKeyPairSync("ed25519");
  const key = publicKey.export({ type: "spki", format: "der" }).toString("hex");
  const config: ProviderConfig = { format: "fod.provider-config", version: 1, providerId: "test-provider", network: "lab", gatewayUrl: "http://127.0.0.1", protocol: "fod.session.v1" };
  const unsigned = { format: "fod.session-result", version: 1, providerId: config.providerId, sessionId: "test-session", utilityId: "test-utility", contentDigest: "11".repeat(32), participantId: "test-participant", issuedAt: "2026-10-01T00:00:00Z", expiresAt: "2026-10-01T01:00:00Z", score: { assigned: 1, answered: 1, correct: 1, passThreshold: 1 }, outcome: "passed" as const, issuedBy: { kind: "provider", providerId: config.providerId, note: "test" }, algorithm: "ed25519" };
  const result: SessionResult = { ...unsigned, signature: sign(null, Buffer.from(canonicalJson(unsigned)), privateKey).toString("hex") };
  assert.equal(await verifyPinnedProviderResult(config, key, result), false);
  assert.equal(await verifyPinnedProviderResult({ ...config, publicKeyHex: "ab".repeat(44) }, key, result), false);
  assert.equal(await verifyPinnedProviderResult({ ...config, providerId: "another-provider", publicKeyHex: key }, key, result), false);
  assert.equal(await verifyPinnedProviderResult({ ...config, publicKeyHex: key }, key, result), true);
});

test("gateway assignments cannot instruct the browser to fetch third-party images", async () => {
  const original = globalThis.fetch;
  try {
    for (const image of ["https://tracker.invalid/wallet", "http://127.0.0.1/private", "file:///private", "data:image/svg+xml;base64,PHN2Zy8+", "data:image/png;base64,PHN2Zy8+"]) {
      globalThis.fetch = async () => new Response(JSON.stringify({ challenges: [{ challengeId: "c", cid: "fixture", variants: [Array(4).fill(image), Array(4).fill(image)] }] }));
      await assert.rejects(new GatewayClient("http://127.0.0.1").challenges("token"));
    }
    const image = "data:image/png;base64,iVBORw0KGgo=";
    globalThis.fetch = async () => new Response(JSON.stringify({ challenges: [{ challengeId: "c", cid: "fixture", variants: [Array(4).fill(image), Array(4).fill(image)] }] }));
    assert.equal((await new GatewayClient("http://127.0.0.1").challenges("token")).challenges.length, 1);
  } finally { globalThis.fetch = original; }
});

test("chunked gateway responses are bounded before the whole body is buffered", async () => {
  const original = globalThis.fetch;
  let canceled = false;
  try {
    globalThis.fetch = async () => new Response(new ReadableStream({
      pull(controller) { controller.enqueue(new Uint8Array(1024 * 1024).fill(32)); },
      cancel() { canceled = true; },
    }));
    await assert.rejects(new GatewayClient("http://127.0.0.1").challenges("token"), /too large/);
    assert.equal(canceled, true);
  } finally { globalThis.fetch = original; }
});

test("wallet prompts only accept the exact enrollment or claim context", () => {
  const now = Date.now();
  const address = "0x" + "11".repeat(20);
  const nonce = "ab".repeat(16);
  const issued = (challenge: string, stamp = now) => ({ challenge, expiresAt: new Date(stamp + 300_000).toISOString() });
  const origin = "https://gateway.example";
  const enroll = `fod-enroll:v3:p:s:${address}:${encodeURIComponent(origin)}:${nonce}:${now}`;
  const claim = `fod-claim:v2:u:${address}:${encodeURIComponent("https://utility.example")}:${nonce}:${now}`;
  const context = { kind: "enroll" as const, providerId: "p", sessionId: "s", address, endpointOrigin: origin };
  assert.equal(checkedWalletChallenge(issued(enroll), context, now), enroll);
  assert.equal(checkedWalletChallenge(issued(claim), { kind: "claim", utilityId: "u", address, endpointOrigin: "https://utility.example" }, now), claim);
  assert.throws(() => checkedWalletChallenge(issued(enroll), { ...context, endpointOrigin: "https://relay.example" }, now));
  assert.throws(() => checkedWalletChallenge(issued(claim), { kind: "claim", utilityId: "u", address, endpointOrigin: "https://relay.example" }, now));
  for (const challenge of ["Sign in to unrelated app", enroll.replace(":p:", ":other:"), enroll.replace(":s:", ":other:"), enroll.replace(address, "0x" + "22".repeat(20)), enroll + "extra", claim]) {
    assert.throws(() => checkedWalletChallenge(issued(challenge), context, now));
  }
  assert.throws(() => checkedWalletChallenge(issued(enroll, now - 300_000), context, now));
  assert.throws(() => checkedWalletChallenge({ challenge: enroll }, context, now));
  assert.throws(() => checkedWalletChallenge(issued(enroll.replace("v3", "v2")), context, now));
});

test("endpoint requests omit ambient credentials and reject redirects and declared oversized bodies", async () => {
  const original = globalThis.fetch;
  let canceled = false;
  try {
    globalThis.fetch = async (_url, options) => {
      assert.equal(options?.credentials, "omit");
      assert.equal(options?.redirect, "error");
      assert.equal(options?.referrerPolicy, "no-referrer");
      assert.ok(options?.signal);
      return new Response('{"ok":true}');
    };
    assert.deepEqual(await requestJSON("http://127.0.0.1"), { ok: true });
    globalThis.fetch = async () => new Response(new ReadableStream({ cancel() { canceled = true; } }), { headers: { "content-length": String(17 * 1024 * 1024) } });
    await assert.rejects(requestJSON("http://127.0.0.1"), /too large/);
    assert.equal(canceled, true);
  } finally { globalThis.fetch = original; }
});
