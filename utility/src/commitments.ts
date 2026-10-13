// The answers a participant commits to this utility *while the session is
// open*, signed with the same wallet that will claim the grant (`SPEC.md` 8.3).
//
// This is what lets a utility score a session itself instead of believing a
// provider's transcript: the answers are witnessed here, before the closed
// policy reveals the answer key, so the gate's own arithmetic over them and the
// revealed key is the local scoring input. This checks consistency with the
// committed answers, not historical correctness or honest disclosure: the
// provider can disclose its answer key early, or withhold its signed result.
//
// What is still trusted is stated rather than implied: this utility's own clock
// (it refuses a commitment that arrives outside the pinned window), the pin the
// operator put in the policy, the answer key's authority and disclosure timing,
// and actual utility release. The wallet signature proves control of the
// claiming address; it does not prove independent solving.

import { verifyMessage } from "viem";
import { answerCommitmentText, answersDigest, type CommittedAnswer } from "../../app/src/score.ts";

const ADDRESS = /^0x[0-9a-fA-F]{40}$/;
const SIGNATURE = /^0x[0-9a-fA-F]{130}$/;
const MAX_ANSWERS = 200;
const MAX_ID = 128;
const MAX_ANSWER = 512;
// A commitment may be sent a little before the utility records it; the window
// check below is what keeps it inside the session.
const MAX_AGE_MS = 5 * 60 * 1000;
const KEEP_AFTER_CLOSE_MS = 60 * 60 * 1000;

/**
 * What a flood of committing wallets may cost this process. The request body
 * cap bounds one commitment; these bound the whole store, so neither the entry
 * count nor the answer text it holds can grow without limit.
 */
export interface CommitmentLimits {
  maxEntries: number;
  maxBytes: number;
}

const DEFAULT_LIMITS: CommitmentLimits = { maxEntries: 10_000, maxBytes: 8 * 1024 * 1024 };

export interface SessionWindow {
  opensAt: string;
  closesAt: string;
}

export interface CommitmentProof {
  address: string;
  sessionId: string;
  answers: CommittedAnswer[];
  issuedAt: number;
  signature: string;
}

export interface Commitment {
  address: string;
  sessionId: string;
  answers: CommittedAnswer[];
  digest: string;
  receivedAt: number;
}

export type CommitmentResult =
  | { ok: true; commitment: Commitment }
  | {
      ok: false;
      error:
        | "bad_address"
        | "bad_answers"
        | "outside_window"
        | "stale_commitment"
        | "bad_signature"
        | "busy";
    };

export class AnswerCommitments {
  private readonly live = new Map<string, { commitment: Commitment; issuedAt: number; expiresAt: number; bytes: number }>();
  private readonly limits: CommitmentLimits;
  private storedBytes = 0;

  constructor(limits: Partial<CommitmentLimits> = {}) {
    this.limits = { ...DEFAULT_LIMITS, ...limits };
  }

  /**
   * The commitment this utility holds for one address and session, if any. An
   * entry that outlived its retention is forgotten here rather than handed out:
   * a claim that arrives long after the session must not be scored against
   * answers this utility no longer stands behind.
   */
  get(utilityId: string, sessionId: string, address: string, now: number): Commitment | null {
    const key = commitmentKey(utilityId, sessionId, address);
    const entry = this.live.get(key);
    if (entry === undefined) {
      return null;
    }
    if (now >= entry.expiresAt) {
      this.storedBytes -= entry.bytes;
      this.live.delete(key);
      return null;
    }
    return entry.commitment;
  }

  async record(input: {
    utilityId: string;
    window: SessionWindow;
    proof: CommitmentProof;
    now: number;
  }): Promise<CommitmentResult> {
    this.sweep(input.now);
    if (!ADDRESS.test(input.proof.address)) {
      return { ok: false, error: "bad_address" };
    }
    const address = input.proof.address.toLowerCase();
    if (!Array.isArray(input.proof.answers) || input.proof.answers.length === 0 || input.proof.answers.length > MAX_ANSWERS) {
      return { ok: false, error: "bad_answers" };
    }
    const seen = new Set<string>();
    for (const answer of input.proof.answers) {
      if (typeof answer?.challengeId !== "string" || answer.challengeId === "" || answer.challengeId.length > MAX_ID) {
        return { ok: false, error: "bad_answers" };
      }
      if (typeof answer?.answer !== "string" || answer.answer.length > MAX_ANSWER) {
        return { ok: false, error: "bad_answers" };
      }
      if (seen.has(answer.challengeId)) {
        return { ok: false, error: "bad_answers" };
      }
      seen.add(answer.challengeId);
    }
    const opensAt = Date.parse(input.window.opensAt);
    const closesAt = Date.parse(input.window.closesAt);
    if (Number.isNaN(opensAt) || Number.isNaN(closesAt) || closesAt <= opensAt) {
      return { ok: false, error: "outside_window" };
    }
    if (input.now < opensAt || input.now >= closesAt) {
      return { ok: false, error: "outside_window" };
    }
    if (!Number.isFinite(input.proof.issuedAt) || Math.abs(input.now - input.proof.issuedAt) > MAX_AGE_MS) {
      return { ok: false, error: "stale_commitment" };
    }
    if (!SIGNATURE.test(input.proof.signature)) {
      return { ok: false, error: "bad_signature" };
    }
    const digest = await answersDigest(input.proof.answers);
    const text = answerCommitmentText({
      utilityId: input.utilityId,
      sessionId: input.proof.sessionId,
      address,
      issuedAt: input.proof.issuedAt,
      digest,
    });
    try {
      const valid = await verifyMessage({
        address: input.proof.address as `0x${string}`,
        message: text,
        signature: input.proof.signature as `0x${string}`,
      });
      if (!valid) {
        return { ok: false, error: "bad_signature" };
      }
    } catch {
      return { ok: false, error: "bad_signature" };
    }
    const key = commitmentKey(input.utilityId, input.proof.sessionId, address);
    const bytes = answersBytes(input.proof.answers);
    const previous = this.live.get(key);
    // Read the current entry after signature verification, then check and write
    // without awaiting: concurrent requests cannot overwrite a newer snapshot.
    if (previous !== undefined) {
      if (input.proof.issuedAt < previous.issuedAt) {
        return { ok: false, error: "stale_commitment" };
      }
      const nextAnswers = new Map(input.proof.answers.map((answer) => [answer.challengeId, answer.answer]));
      for (const answer of previous.commitment.answers) {
        if (!nextAnswers.has(answer.challengeId)) {
          return { ok: false, error: "bad_answers" };
        }
        // Millisecond ties may add answers or retry the same set, but cannot
        // choose between conflicting values. Corrections need a newer proof.
        if (input.proof.issuedAt === previous.issuedAt && nextAnswers.get(answer.challengeId) !== answer.answer) {
          return { ok: false, error: "stale_commitment" };
        }
      }
    }
    const nextBytes = this.storedBytes - (previous?.bytes ?? 0) + bytes;
    if ((previous === undefined && this.live.size >= this.limits.maxEntries) || nextBytes > this.limits.maxBytes) {
      return { ok: false, error: "busy" };
    }
    const commitment: Commitment = {
      address,
      sessionId: input.proof.sessionId,
      answers: input.proof.answers.map((answer) => ({
        challengeId: answer.challengeId,
        answer: answer.answer,
      })),
      digest,
      receivedAt: input.now,
    };
    // A newer complete snapshot may correct answers while the window is open;
    // every challenge already witnessed by the utility must remain present.
    if (previous !== undefined) {
      this.storedBytes -= previous.bytes;
    }
    this.live.set(key, { commitment, issuedAt: input.proof.issuedAt, expiresAt: closesAt + KEEP_AFTER_CLOSE_MS, bytes });
    this.storedBytes += bytes;
    return { ok: true, commitment };
  }

  private sweep(now: number): void {
    for (const [key, entry] of this.live) {
      if (now >= entry.expiresAt) {
        this.storedBytes -= entry.bytes;
        this.live.delete(key);
      }
    }
  }
}

/** The answer text one commitment holds, as the budget counts it. */
function answersBytes(answers: CommittedAnswer[]): number {
  return answers.reduce((total, answer) => total + Buffer.byteLength(answer.challengeId, "utf8") + Buffer.byteLength(answer.answer, "utf8"), 0);
}

function commitmentKey(utilityId: string, sessionId: string, address: string): string {
  return `${utilityId}|${sessionId}|${address.toLowerCase()}`;
}
