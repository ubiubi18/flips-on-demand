// The participant's score check under test: the provider's signed score has to
// follow from the answers this browser submitted and the answer key the
// provider pinned before the session opened, or the check refuses it.

import { test } from "node:test";
import assert from "node:assert/strict";
import { canonicalJson } from "../src/provider.ts";
import { canonicalAnswer, sha256Hex, verifyScore, verifyTranscript } from "../src/score.ts";

const sessionId = "session-1";

async function fixture() {
  const reveal = {
    sessionId,
    utilityId: "utility-1",
    rules: { passThreshold: 2, opensAt: "2026-10-01T00:00:00Z", closesAt: "2026-10-01T00:10:00Z" },
    provenance: { epoch: 228, author: "0x" + "11".repeat(20), source: "epoch 228", blockRange: "1-2" },
    answerSalt: "ab".repeat(32),
    challenges: [
      { challengeId: "c1", cid: "cid-1", answerDigest: await sha256Hex(canonicalAnswer("left")), contentSha256: null },
      {
        challengeId: "c2",
        cid: "cid-2",
        answerDigest: await sha256Hex(canonicalAnswer("second answer")),
        contentSha256: null,
      },
    ],
  };
  return { reveal, pinned: await sha256Hex(canonicalJson(reveal)) };
}

async function entry(challengeId: string, answer: string | null) {
  return {
    challengeId,
    answerDigest: answer === null ? null : await sha256Hex(canonicalAnswer(answer)),
  };
}

test("a passing score recomputes from the participant's own answers and the pinned reveal", async () => {
  const { reveal, pinned } = await fixture();
  const checked = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "left"), await entry("c2", "second answer")] },
    answers: { c1: "left", c2: "second answer" },
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
  });
  assert.deepEqual(checked, { ok: true });
});

test("case folding and extra whitespace do not change what was submitted", async () => {
  const { reveal, pinned } = await fixture();
  const checked = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "left"), await entry("c2", "second answer")] },
    answers: { c1: "  Left ", c2: "Second   Answer" },
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
  });
  assert.deepEqual(checked, { ok: true });
});

test("a validly signed result that claims a pass the answers do not support is refused", async () => {
  const { reveal, pinned } = await fixture();
  // The browser answered "right" to c1; the provider's transcript agrees, but
  // its signed score counts two correct and passes. The signature can be
  // perfect and the claim still does not follow from the answers.
  const checked = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "right"), await entry("c2", "second answer")] },
    answers: { c1: "right", c2: "second answer" },
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
  });
  assert.equal(checked.ok, false);
  assert.match((checked as { reason: string }).reason, /recomputes 1/);
});

test("a provider that records an answer the participant never submitted is refused", async () => {
  const { reveal, pinned } = await fixture();
  const checked = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "right"), await entry("c2", "second answer")] },
    answers: { c1: "left", c2: "second answer" },
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
  });
  assert.equal(checked.ok, false);
  assert.match((checked as { reason: string }).reason, /not the answer this browser submitted/);
});

test("a provider that scored an answer the participant never sent is refused", async () => {
  const { reveal, pinned } = await fixture();
  const checked = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "left"), await entry("c2", "second answer")] },
    answers: { c1: "left" },
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
  });
  assert.equal(checked.ok, false);
  assert.match((checked as { reason: string }).reason, /never submitted/);
});

test("an answer key served after the fact does not hash to the pin and is refused", async () => {
  const { reveal, pinned } = await fixture();
  const tampered = { ...reveal, challenges: [{ ...reveal.challenges[0] }, { ...reveal.challenges[1] }] };
  tampered.challenges[0].answerDigest = await sha256Hex(canonicalAnswer("right"));
  const checked = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal: tampered,
    transcript: { entries: [await entry("c1", "right"), await entry("c2", "second answer")] },
    answers: { c1: "right", c2: "second answer" },
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
  });
  assert.equal(checked.ok, false);
  assert.match((checked as { reason: string }).reason, /does not hash to the digest this session is pinned by/);
});

test("a signed threshold that is not the pinned one is refused", async () => {
  const { reveal, pinned } = await fixture();
  const checked = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "right"), await entry("c2", "second answer")] },
    answers: { c1: "right", c2: "second answer" },
    score: { assigned: 2, answered: 2, correct: 1, passThreshold: 1 },
    outcome: "passed",
  });
  assert.equal(checked.ok, false);
  assert.match((checked as { reason: string }).reason, /pass threshold/);
});

test("a result that states no transcript is refused, and an honest partial score still checks", async () => {
  const { reveal, pinned } = await fixture();
  const missing = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: undefined,
    answers: { c1: "left" },
    score: { assigned: 2, answered: 1, correct: 1, passThreshold: 2 },
    outcome: "not-passed",
  });
  assert.equal(missing.ok, false);
  assert.match((missing as { reason: string }).reason, /does not state which answers/);

  const honest = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "left"), await entry("c2", null)] },
    answers: { c1: "left" },
    score: { assigned: 2, answered: 1, correct: 1, passThreshold: 2 },
    outcome: "not-passed",
  });
  assert.deepEqual(honest, { ok: true });
});

test("an unanswered challenge counted as an answer is refused", async () => {
  const { reveal, pinned } = await fixture();
  const checked = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "left"), await entry("c2", null)] },
    answers: { c1: "left" },
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
  });
  assert.equal(checked.ok, false);
  assert.match((checked as { reason: string }).reason, /transcript shows 1/);
});

test("a transcript that omits an answer this browser submitted is refused", async () => {
  const { reveal, pinned } = await fixture();
  // The provider drops c2 from its transcript and shrinks `assigned` with it, so
  // the counts would agree with each other while ignoring a real answer.
  const checked = await verifyScore({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "left")] },
    answers: { c1: "left", c2: "second answer" },
    score: { assigned: 1, answered: 1, correct: 1, passThreshold: 1 },
    outcome: "passed",
  });
  assert.equal(checked.ok, false);
  assert.match((checked as { reason: string }).reason, /omits this browser's answer for c2/);
});

// The utility runs the same rule with no participant input: the score has to
// follow from the transcript the provider signed and the answer key the pinned
// content carries.
test("the arithmetic checks out for a utility that sees only the signed transcript", async () => {
  const { reveal, pinned } = await fixture();
  const checked = await verifyTranscript({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "left"), await entry("c2", "right")] },
    score: { assigned: 2, answered: 2, correct: 1, passThreshold: 2 },
    outcome: "not-passed",
  });
  assert.deepEqual(checked, { ok: true });
});

test("a utility refuses a score its own arithmetic does not reproduce", async () => {
  const { reveal, pinned } = await fixture();
  const checked = await verifyTranscript({
    sessionId,
    expectedDigest: pinned,
    reveal,
    transcript: { entries: [await entry("c1", "left"), await entry("c2", "right")] },
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
  });
  assert.equal(checked.ok, false);
  assert.match((checked as { reason: string }).reason, /recomputes 1/);
});

test("a utility refuses a reveal that does not hash to the digest it pinned", async () => {
  const { reveal, pinned } = await fixture();
  const other = { ...reveal, answerSalt: "cd".repeat(32) };
  const checked = await verifyTranscript({
    sessionId,
    expectedDigest: pinned,
    reveal: other,
    transcript: { entries: [await entry("c1", "left"), await entry("c2", "second answer")] },
    score: { assigned: 2, answered: 2, correct: 2, passThreshold: 2 },
    outcome: "passed",
  });
  assert.equal(checked.ok, false);
  assert.match((checked as { reason: string }).reason, /does not hash to the digest/);
});

test("a transcript must cover the exact pinned challenge set, including unanswered challenges", async () => {
  const { reveal } = await fixture();
  reveal.rules.passThreshold = 1;
  const pinned = await sha256Hex(canonicalJson(reveal));
  for (const entries of [[await entry("c1", "left")], [await entry("c1", "left"), await entry("unassigned", null)]]) {
    const checked = await verifyTranscript({ sessionId, expectedDigest: pinned, reveal, transcript: { entries },
      score: { assigned: entries.length, answered: 1, correct: 1, passThreshold: 1 }, outcome: "passed" });
    assert.equal(checked.ok, false);
  }
});
