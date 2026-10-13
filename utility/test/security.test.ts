import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { privateKeyToAccount } from "viem/accounts";
import { ClaimChallenges } from "../src/claims.ts";
import { AnswerCommitments } from "../src/commitments.ts";
import { FileGrants } from "../src/access.ts";
import { allowRateLimit } from "../src/server.ts";
import { answerCommitmentText, answersDigest } from "../../app/src/score.ts";

// Published test key, used only for disposable signatures.
const wallet = privateKeyToAccount(`0x${"11".repeat(32)}`);

describe("security boundary regressions", () => {
  it("retains live rate-limit allowances at capacity and admits new keys after expiry", () => {
    const windows = new Map<string, { start: number; count: number }>();
    const config = { windowSeconds: 60, maxRequests: 1 };
    for (let i = 0; i < 10_000; i++) assert.equal(allowRateLimit(windows, config, `client:${i}`, 0), true);
    assert.equal(allowRateLimit(windows, config, "new-client", 1), false);
    assert.equal(allowRateLimit(windows, config, "client:0", 2), false);
    assert.equal(allowRateLimit(windows, config, "new-client", 60_000), true);
  });
  it("consumes a claim nonce only once under concurrent signature checks", async () => {
    const store = new ClaimChallenges();
    const now = Date.now();
    const origin = "https://utility.example";
    const issued = store.issue("test-utility", wallet.address, origin, now)!;
    const proof = {
      address: wallet.address,
      challenge: issued.challenge,
      signature: await wallet.signMessage({ message: issued.challenge }),
    };
    const results = await Promise.all([
      store.check("test-utility", origin, proof, now),
      store.check("test-utility", origin, proof, now),
    ]);
    assert.equal(results.filter((result) => result.ok).length, 1);
    assert.equal(results.filter((result) => !result.ok && result.error === "unknown_nonce").length, 1);
  });

  it("rejects a valid signature for another utility origin", async () => {
    const store = new ClaimChallenges();
    const now = Date.now();
    const issued = store.issue("test-utility", wallet.address, "https://utility.example", now)!;
    const proof = { address: wallet.address, challenge: issued.challenge,
      signature: await wallet.signMessage({ message: issued.challenge }) };
    assert.deepEqual(await store.check("test-utility", "https://relay.example", proof, now),
      { ok: false, error: "bad_challenge" });
    assert.equal((await store.check("test-utility", "https://utility.example", proof, now)).ok, true);
  });

  it("applies the answer byte budget to replacements and counts UTF-8 bytes", async () => {
    const store = new AnswerCommitments({ maxEntries: 1, maxBytes: 16 });
    const now = Date.now();
    const window = { opensAt: new Date(now - 1000).toISOString(), closesAt: new Date(now + 1000).toISOString() };
    let issuedAt = now - 10;
    const record = async (answer: string) => {
      issuedAt += 1;
      const answers = [{ challengeId: "c", answer }];
      const message = answerCommitmentText({ utilityId: "u", sessionId: "s", address: wallet.address, issuedAt, digest: await answersDigest(answers) });
      return store.record({ utilityId: "u", window, now, proof: { address: wallet.address, sessionId: "s", answers, issuedAt, signature: await wallet.signMessage({ message }) } });
    };
    assert.equal((await record("a")).ok, true);
    assert.equal((await record("1234567890123456")).ok, false);
    assert.equal((await record("\u{1f600}".repeat(4))).ok, false);
    assert.equal(store.get("u", "s", wallet.address, now)?.answers[0].answer, "a");
    assert.equal((await record("short")).ok, true);
    assert.equal(store.get("u", "s", wallet.address, now)?.answers[0].answer, "short");
  });

  it("cannot truncate another file through a preplanted grant temporary symlink", async () => {
    const dir = await mkdtemp(join(tmpdir(), "fod-grants-test-"));
    try {
      const path = join(dir, "grants.json");
      const victim = join(dir, "unrelated.txt");
      await writeFile(victim, "preserve this file", { mode: 0o600 });
      await symlink(victim, `${path}.tmp`);
      const store = await FileGrants.open(path);
      const key = { utilityId: "u", providerId: "p", kind: "wallet" as const, address: wallet.address };
      const now = Date.now();
      store.record(key, "grant", new Date(now + 60_000).toISOString(), now);
      assert.equal(await readFile(victim, "utf8"), "preserve this file");
      assert.equal((await FileGrants.open(path)).count(key, now), 1);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
});
