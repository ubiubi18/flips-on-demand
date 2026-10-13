// The score rules, shared by every verifier.
//
// A provider's signature proves it issued a statement, not that the score in it
// follows from the answers anyone submitted (`SPEC.md` 8.3). Everything here
// rests on two facts: the digest a verifier read from the public policy *before*
// the session ran, and the digest preimage (`contentReveal`) the gateway
// publishes once it has closed. From those and the answers:
//
// - `verifyTranscript` checks the arithmetic the provider signed;
// - `verifyScore` also checks that the provider's record is what this browser
//   submitted, which is the participant's refutation;
// - `scoreCommittedAnswers` scores the answers a utility itself witnessed while
//   the window was open, so its gate does not consult the provider at all.
//
// This is deliberately a pure module: no network, no DOM, so the same rules run
// in a test, in the browser, and inside the utility.

// The `.ts` extension keeps this module runnable by `node --test` as well as
// bundlable by esbuild (`app/tsconfig.json` sets `allowImportingTsExtensions`).
import { canonicalJson } from "./provider.ts";

export type ScoreCheck = { ok: true } | { ok: false; reason: string };

const HEX_64 = /^[0-9a-f]{64}$/;

export interface ScoreInput {
  /** The session the result names, which the reveal has to be about. */
  sessionId: string;
  /** The digest the reveal has to hash to: the pin, or the result's own claim. */
  expectedDigest: string;
  /** The `contentReveal` the closed policy publishes; unknown until then. */
  reveal: unknown;
  /** What the provider says it scored, as its signed result carries it. */
  transcript: { entries: { challengeId: string; answerDigest: string | null }[] } | undefined;
  score: { assigned: number; answered: number; correct: number; passThreshold: number };
  outcome: string;
}

/**
 * The gateway's canonical answer text (`gateway/src/session.ts`, `answerDigest`):
 * trimmed, inner whitespace collapsed, case-folded. Anything else is a
 * different answer, and both sides must agree on that byte for byte.
 */
export function canonicalAnswer(answer: string): string {
  return answer.trim().replace(/\s+/g, " ").toLowerCase();
}

export async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

/**
 * The answer key a closed session reveals, checked against the digest that
 * session is pinned by. Every verifier needs exactly this much before it can
 * say anything about a score: the reveal has to be the preimage of the pin, and
 * the pin has to be the one that was published before the session ran.
 */
export type RevealedKey =
  | { ok: true; key: Map<string, string>; passThreshold: number }
  | { ok: false; reason: string };

export async function revealedAnswerKey(input: {
  sessionId: string;
  expectedDigest: string;
  reveal: unknown;
}): Promise<RevealedKey> {
  if (!HEX_64.test(input.expectedDigest.toLowerCase())) {
    return {
      ok: false,
      reason: "there is no content digest to tie this score to the flip set it was scored against",
    };
  }
  if (typeof input.reveal !== "object" || input.reveal === null) {
    return { ok: false, reason: "the closed session published no answer key, so the score cannot be recomputed" };
  }
  const reveal = input.reveal as {
    sessionId?: unknown;
    answerSalt?: unknown;
    rules?: { passThreshold?: unknown };
    challenges?: { challengeId?: unknown; answerDigest?: unknown }[];
  };
  if (typeof reveal.sessionId !== "string" || reveal.sessionId !== input.sessionId) {
    return { ok: false, reason: "the revealed content is about another session" };
  }
  if (typeof reveal.answerSalt !== "string" || !HEX_64.test(reveal.answerSalt.toLowerCase())) {
    return { ok: false, reason: "the revealed content carries no usable answer salt" };
  }
  if (!Array.isArray(reveal.challenges) || reveal.challenges.length === 0) {
    return { ok: false, reason: "the revealed content lists no challenges" };
  }
  const passThreshold = reveal.rules?.passThreshold;
  if (!Number.isInteger(passThreshold) || (passThreshold as number) < 1) {
    return { ok: false, reason: "the revealed content carries no usable pass threshold" };
  }
  const recomputedDigest = await sha256Hex(canonicalJson(input.reveal));
  if (recomputedDigest !== input.expectedDigest.toLowerCase()) {
    return { ok: false, reason: "the revealed content does not hash to the digest this session is pinned by" };
  }
  const key = new Map<string, string>();
  for (const challenge of reveal.challenges) {
    if (typeof challenge.challengeId !== "string" || typeof challenge.answerDigest !== "string") {
      return { ok: false, reason: "the revealed content has a challenge without an answer digest" };
    }
    if (!HEX_64.test(challenge.answerDigest.toLowerCase())) {
      return { ok: false, reason: `the revealed answer digest for ${challenge.challengeId} is malformed` };
    }
    if (key.has(challenge.challengeId)) {
      return { ok: false, reason: "the revealed content lists a challenge twice" };
    }
    key.set(challenge.challengeId, challenge.answerDigest.toLowerCase());
  }
  return { ok: true, key, passThreshold: passThreshold as number };
}

/**
 * One answer a participant committed to while the session was open.
 */
export interface CommittedAnswer {
  challengeId: string;
  answer: string;
}

/** The committed answers in the order both sides hash them. */
export function canonicalAnswers(answers: CommittedAnswer[]): CommittedAnswer[] {
  return [...answers].sort((left, right) => (left.challengeId < right.challengeId ? -1 : 1));
}

/** The digest a commitment names, over the canonical answer list. */
export function answersDigest(answers: CommittedAnswer[]): Promise<string> {
  return sha256Hex(canonicalJson(canonicalAnswers(answers)));
}

/**
 * The text a participant's wallet signs when it commits its answers to a
 * utility during the window (`SPEC.md` 8.3). It names the utility and the
 * session, so a commitment cannot be moved to another gate or another session,
 * and the digest of the answers, so the signature covers exactly what will be
 * scored.
 */
export function answerCommitmentText(input: {
  utilityId: string;
  sessionId: string;
  address: string;
  issuedAt: number;
  digest: string;
}): string {
  return `fod-answers:v1:${input.utilityId}:${input.sessionId}:${input.address.toLowerCase()}:${input.issuedAt}:${input.digest}`;
}

export type CommittedScore =
  | { ok: true; assigned: number; answered: number; correct: number; passThreshold: number }
  | { ok: false; reason: string };

/**
 * The score a utility computes from the answers it witnessed during the window
 * and the answer key the pinned content reveals. No provider statement enters
 * this: it is the gate's own arithmetic over what the participant committed.
 */
export async function scoreCommittedAnswers(input: {
  sessionId: string;
  expectedDigest: string;
  reveal: unknown;
  answers: CommittedAnswer[];
}): Promise<CommittedScore> {
  const revealed = await revealedAnswerKey({
    sessionId: input.sessionId,
    expectedDigest: input.expectedDigest,
    reveal: input.reveal,
  });
  if (!revealed.ok) {
    return revealed;
  }
  const seen = new Set<string>();
  let correct = 0;
  for (const answer of input.answers) {
    if (typeof answer.challengeId !== "string" || typeof answer.answer !== "string") {
      return { ok: false, reason: "a committed answer is not a challenge id and an answer" };
    }
    if (seen.has(answer.challengeId)) {
      return { ok: false, reason: `the commitment answers ${answer.challengeId} twice` };
    }
    seen.add(answer.challengeId);
    const expected = revealed.key.get(answer.challengeId);
    if (expected === undefined) {
      return { ok: false, reason: `the revealed content does not cover ${answer.challengeId}` };
    }
    if ((await sha256Hex(canonicalAnswer(answer.answer))) === expected) {
      correct += 1;
    }
  }
  return {
    ok: true,
    assigned: revealed.key.size,
    answered: input.answers.length,
    correct,
    passThreshold: revealed.passThreshold,
  };
}

/**
 * The part every verifier can run with no participant input: the reveal has to
 * be the preimage of the digest that was pinned, and the signed score has to
 * follow from the transcript against that revealed answer key. A utility uses
 * this to stop taking the provider's arithmetic on its word; the participant's
 * client runs it too, after checking the transcript against its own answers.
 */
export async function verifyTranscript(input: ScoreInput): Promise<ScoreCheck> {
  const revealed = await revealedAnswerKey({
    sessionId: input.sessionId,
    expectedDigest: input.expectedDigest,
    reveal: input.reveal,
  });
  if (!revealed.ok) {
    return revealed;
  }
  if (input.transcript === undefined || !Array.isArray(input.transcript.entries)) {
    return { ok: false, reason: "the provider's result does not state which answers it scored" };
  }
  // The threshold is part of what the digest covers, so a provider cannot sign
  // a score against a rule it published after the answers came in.
  if (revealed.passThreshold !== input.score.passThreshold) {
    return {
      ok: false,
      reason: "the signed score's pass threshold is not the one the pinned session content carries",
    };
  }
  const key = revealed.key;
  if (input.transcript.entries.length !== key.size) {
    return { ok: false, reason: "the transcript does not cover the pinned challenge set" };
  }

  let answered = 0;
  let correct = 0;
  const seen = new Set<string>();
  for (const entry of input.transcript.entries) {
    if (typeof entry?.challengeId !== "string" || seen.has(entry.challengeId) || !key.has(entry.challengeId)) {
      return { ok: false, reason: "the provider's transcript repeats or misnames a challenge" };
    }
    seen.add(entry.challengeId);
    if (entry.answerDigest === null) {
      continue;
    }
    if (typeof entry.answerDigest !== "string" || !HEX_64.test(entry.answerDigest.toLowerCase())) {
      return { ok: false, reason: `the provider's answer digest for ${entry.challengeId} is malformed` };
    }
    const expected = key.get(entry.challengeId);
    if (expected === undefined) {
      return { ok: false, reason: `the revealed content does not cover ${entry.challengeId}` };
    }
    answered += 1;
    if (expected === entry.answerDigest.toLowerCase()) {
      correct += 1;
    }
  }

  if (input.score.assigned !== input.transcript.entries.length) {
    return { ok: false, reason: "the signed score counts a different number of challenges than the transcript" };
  }
  if (input.score.answered !== answered) {
    return { ok: false, reason: `the signed score says ${input.score.answered} answers, the transcript shows ${answered}` };
  }
  if (input.score.correct !== correct) {
    return { ok: false, reason: `the signed score says ${input.score.correct} correct, this browser recomputes ${correct}` };
  }
  const expectedOutcome = correct >= input.score.passThreshold ? "passed" : "not-passed";
  if (input.outcome !== expectedOutcome) {
    return { ok: false, reason: `the signed outcome is ${input.outcome}; the recomputed score is ${expectedOutcome}` };
  }
  return { ok: true };
}

/**
 * The participant's check: what the provider says it scored has to be what this
 * browser submitted, challenge by challenge, and then the shared arithmetic has
 * to reproduce the signed score.
 */
export async function verifyScore(
  input: ScoreInput & {
    /** The answers this participant submitted, keyed by challenge id. */
    answers: Record<string, string>;
  },
): Promise<ScoreCheck> {
  if (input.transcript === undefined || !Array.isArray(input.transcript.entries)) {
    return { ok: false, reason: "the provider's result does not state which answers it scored" };
  }
  const seen = new Set<string>();
  for (const entry of input.transcript.entries) {
    if (typeof entry?.challengeId !== "string") {
      return { ok: false, reason: "the provider's transcript misnames a challenge" };
    }
    seen.add(entry.challengeId);
    const own = input.answers[entry.challengeId];
    if (entry.answerDigest === null) {
      if (own !== undefined) {
        return {
          ok: false,
          reason: `the provider recorded no answer for ${entry.challengeId} while this browser submitted one`,
        };
      }
      continue;
    }
    if (own === undefined) {
      return {
        ok: false,
        reason: `the provider scored an answer for ${entry.challengeId} that this browser never submitted`,
      };
    }
    const ownDigest = await sha256Hex(canonicalAnswer(own));
    if (ownDigest !== entry.answerDigest.toLowerCase()) {
      return {
        ok: false,
        reason: `the provider's record of the answer to ${entry.challengeId} is not the answer this browser submitted`,
      };
    }
  }
  // A transcript that drops a challenge this browser answered would otherwise
  // check out against a smaller `assigned`: the omission has to be refused.
  for (const challengeId of Object.keys(input.answers)) {
    if (!seen.has(challengeId)) {
      return {
        ok: false,
        reason: `the provider's transcript omits this browser's answer for ${challengeId}`,
      };
    }
  }
  return verifyTranscript(input);
}
