import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { privateKeyToAccount } from "viem/accounts";
import { AnswerCommitments } from "../src/commitments.ts";
import { answerCommitmentText, answersDigest, type CommittedAnswer } from "../../app/src/score.ts";

// Published disposable test key.
const wallet = privateKeyToAccount(`0x${"11".repeat(32)}`);
const answers = [{ challengeId: "c1", answer: "first" }, { challengeId: "c2", answer: "second" }];

function fixture() {
  const store = new AnswerCommitments();
  const now = Date.now();
  const window = { opensAt: new Date(now - 1000).toISOString(), closesAt: new Date(now + 1000).toISOString() };
  const record = async (set: CommittedAnswer[], issuedAt: number, receivedAt = now) => {
    const message = answerCommitmentText({ utilityId: "u", sessionId: "s", address: wallet.address, issuedAt, digest: await answersDigest(set) });
    return store.record({ utilityId: "u", window, now: receivedAt, proof: { address: wallet.address, sessionId: "s", answers: set, issuedAt, signature: await wallet.signMessage({ message }) } });
  };
  return { store, now, record, stored: () => store.get("u", "s", wallet.address, now)?.answers };
}

describe("answer commitment ordering", () => {
  it("rejects an older signed snapshot after the complete set arrives", async () => {
    const { now, record, stored } = fixture();
    assert.equal((await record(answers, now)).ok, true);
    assert.deepEqual(await record(answers.slice(0, 1), now - 1), { ok: false, error: "stale_commitment" });
    assert.deepEqual(stored(), answers);
  });

  it("accepts equal-time extensions and retries, but rejects rollback and ambiguous correction", async () => {
    const { now, record, stored } = fixture();
    assert.equal((await record(answers.slice(0, 1), now)).ok, true);
    assert.equal((await record(answers, now)).ok, true);
    assert.equal((await record([...answers].reverse(), now)).ok, true);
    assert.deepEqual(await record(answers.slice(0, 1), now), { ok: false, error: "bad_answers" });
    assert.deepEqual(await record([{ ...answers[0], answer: "changed" }, answers[1]], now), { ok: false, error: "stale_commitment" });
    assert.deepEqual(stored(), [...answers].reverse());
  });

  it("requires newer updates to retain every challenge, while allowing answer corrections", async () => {
    const { now, record, stored } = fixture();
    assert.equal((await record(answers, now - 2)).ok, true);
    assert.deepEqual(await record(answers.slice(1), now - 1), { ok: false, error: "bad_answers" });
    const corrected = [{ ...answers[0], answer: "corrected" }, answers[1]];
    assert.equal((await record(corrected, now)).ok, true);
    assert.deepEqual(stored(), corrected);
  });

  it("refuses duplicate challenges and updates outside the pinned window", async () => {
    const { now, record, stored } = fixture();
    assert.equal((await record(answers, now)).ok, true);
    assert.deepEqual(await record([...answers, answers[0]], now + 1), { ok: false, error: "bad_answers" });
    assert.deepEqual(await record(answers, now + 1000, now + 1000), { ok: false, error: "outside_window" });
    assert.deepEqual(stored(), answers);
  });
});
