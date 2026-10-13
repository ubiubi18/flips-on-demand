// Session state for the reference integration.
//
// Deliberately small and in-memory: enrolment, challenge assignment, answer
// intake, scoring against the provider's pool, and one signed result per
// participant. Expiry and replay protection are part of the state machine, not
// a middleware afterthought, because SPEC.md 8.2 requires both to be rejected.

import { createHash, randomBytes } from "node:crypto";
import type { FlipPool } from "./pool.ts";
import { enrollmentChallenge } from "./wallet.ts";

export interface Participant {
  participantId: string;
  tokenHash: string;
  expiresAt: number;
  /** The EVM/Idena address that signed the enrollment. */
  address: string;
}

export interface Submission {
  challengeId: string;
  answerDigest: string;
  submittedAt: number;
  correct: boolean;
}

export type EnrolResult =
  | { ok: true; participantId: string; token: string; expiresAt: number; address: string }
  | {
      ok: false;
      error: "session_closed" | "session_not_open" | "address_enrolled" | "session_full";
    };

export type SubmitResult =
  | { ok: true }
  | {
      ok: false;
      error: "unknown_challenge" | "already_answered" | "session_closed" | "session_not_open" | "empty_answer";
    };

/** A one-time enrollment challenge the gateway issued and has not consumed. */
export interface IssuedEnrollmentNonce {
  nonce: string;
  challenge: string;
  expiresAt: number;
}

/**
 * Where the request sits relative to the pool's published window. The window
 * bounds work, not identity: `open` is the only state in which a participant
 * may enrol or answer, and `not-open` covers every instant before `opensAt`.
 */
export type WindowState = "not-open" | "open" | "closed";

export interface ScoreSummary {
  assigned: number;
  answered: number;
  correct: number;
  passed: boolean;
  passThreshold: number;
  /**
   * What the provider scored, one entry per assigned challenge in assignment
   * order, with the digest of the answer it recorded or `null` when the
   * participant did not answer (`SPEC.md` 10.4, "provenance versus scoring").
   * Published in the signed result: with the closed policy's `contentReveal` a
   * participant recomputes its own score instead of trusting this summary.
   */
  transcript: { challengeId: string; answerDigest: string | null }[];
}

export function answerDigest(answer: string): string {
  // Canonical form of a short answer: trimmed, inner whitespace collapsed, and
  // case-folded. Anything else counts as a different answer.
  const canonical = answer.trim().replace(/\s+/g, " ").toLowerCase();
  return createHash("sha256").update(canonical, "utf8").digest("hex");
}

export class SessionState {
  private readonly participants = new Map<string, Participant>();
  private readonly byToken = new Map<string, string>();
  private readonly submissions = new Map<string, Map<string, Submission>>();
  private readonly nonces = new Map<string, { providerId: string; address: string; expiresAt: number }>();
  private readonly pool: FlipPool;
  private readonly ttlSeconds: number;
  private readonly maxParticipants: number;

  constructor(pool: FlipPool, ttlSeconds: number, maxParticipants: number = MAX_PARTICIPANTS) {
    this.pool = pool;
    this.ttlSeconds = ttlSeconds;
    this.maxParticipants = maxParticipants;
  }

  get rules() {
    return this.pool.rules;
  }

  windowState(now: number): WindowState {
    const opensAt = Date.parse(this.pool.rules.opensAt);
    const closesAt = Date.parse(this.pool.rules.closesAt);
    if (now < opensAt) {
      return "not-open";
    }
    if (now >= closesAt) {
      return "closed";
    }
    return "open";
  }

  /** Live participants in this window, after forgetting expired ones. */
  participantCount(now: number): number {
    this.sweepExpired(now);
    return this.participants.size;
  }

  /**
   * Results are issued only once the session is irrevocably closed: while the
   * window is open, a participant could otherwise read the correct count after
   * every answer and learn the answers one at a time (`AH-1`).
   */
  resultsAvailable(now: number): boolean {
    return this.windowState(now) === "closed";
  }

  /**
   * Challenges as a participant sees them: the pictures and the two sequences
   * when the pool carries the content, never the answer material.
   */
  challenges(): { challengeId: string; cid: string; variants?: string[][] }[] {
    return this.pool.challenges.map(({ challengeId, cid, variants }) => ({
      challengeId,
      cid,
      ...(variants === undefined ? {} : { variants }),
    }));
  }

  keyMaterial(): string {
    return this.pool.provenance.author;
  }

  enroll(now: number, address: string): EnrolResult {
    const window = this.windowState(now);
    if (window === "not-open") {
      return { ok: false, error: "session_not_open" };
    }
    if (window === "closed") {
      return { ok: false, error: "session_closed" };
    }
    this.sweepExpired(now);
    // One live enrollment per address per session. This is a key rule, not a
    // personhood claim: the same address cannot hold two live attempts, but a
    // second address still can.
    for (const participant of this.participants.values()) {
      if (participant.address === address) {
        return { ok: false, error: "address_enrolled" };
      }
    }
    if (this.participants.size >= this.maxParticipants) {
      return { ok: false, error: "session_full" };
    }
    const token = randomBytes(32).toString("hex");
    const participantId = randomBytes(16).toString("hex");
    const expiresAt = now + this.ttlSeconds * 1000;
    this.participants.set(participantId, {
      participantId,
      tokenHash: hashToken(token),
      expiresAt,
      address,
    });
    this.byToken.set(hashToken(token), participantId);
    this.submissions.set(participantId, new Map());
    return { ok: true, participantId, token, expiresAt, address };
  }

  /**
   * Issues a one-time nonce bound to the provider, this session and the
   * address whose key will sign it. The nonce is held here, never in the
   * signed text alone, so a captured signature is worth one consumed nonce.
   */
  issueEnrollmentNonce(providerId: string, address: string, origin: string, now: number): IssuedEnrollmentNonce | null {
    this.sweepExpiredNonces(now);
    if (this.nonces.size >= MAX_ENROLLMENT_NONCES) {
      return null;
    }
    const nonce = randomBytes(16).toString("hex");
    const expiresAt = now + ENROLLMENT_NONCE_MS;
    this.nonces.set(nonce, { providerId, address, expiresAt });
    return {
      nonce,
      challenge: enrollmentChallenge({
        providerId,
        sessionId: this.pool.sessionId,
        origin,
        address,
        nonce,
        issuedAt: now,
      }),
      expiresAt,
    };
  }

  /** Consumes a nonce; it is single-use whether or not it checked out. */
  consumeEnrollmentNonce(providerId: string, address: string, nonce: string, now: number): boolean {
    const entry = this.nonces.get(nonce);
    if (entry === undefined) {
      return false;
    }
    this.nonces.delete(nonce);
    if (now >= entry.expiresAt) {
      return false;
    }
    return entry.providerId === providerId && entry.address === address;
  }

  authenticate(token: string, now: number): Participant | "expired" | null {
    const participantId = this.byToken.get(hashToken(token));
    if (participantId === undefined) {
      return null;
    }
    const participant = this.participants.get(participantId);
    if (participant === undefined) {
      return null;
    }
    if (now >= participant.expiresAt) {
      // An expired session is forgotten here rather than kept until restart.
      this.forget(participantId);
      return "expired";
    }
    return participant;
  }

  submit(participantId: string, challengeId: string, answer: string, now: number): SubmitResult {
    const window = this.windowState(now);
    if (window === "not-open") {
      return { ok: false, error: "session_not_open" };
    }
    if (window === "closed") {
      return { ok: false, error: "session_closed" };
    }
    const challenge = this.pool.challenges.find((entry) => entry.challengeId === challengeId);
    if (challenge === undefined) {
      return { ok: false, error: "unknown_challenge" };
    }
    if (answer.trim() === "") {
      return { ok: false, error: "empty_answer" };
    }
    const submissions = this.submissions.get(participantId);
    if (submissions === undefined) {
      return { ok: false, error: "unknown_challenge" };
    }
    if (submissions.has(challengeId)) {
      return { ok: false, error: "already_answered" };
    }
    const digest = answerDigest(answer);
    const correct = digest === challenge.answerDigest;
    submissions.set(challengeId, {
      challengeId,
      answerDigest: digest,
      submittedAt: now,
      correct,
    });
    // The caller gets no per-answer signal: while the session is open, that
    // would be the answer oracle this adapter must not provide (`AH-1`).
    return { ok: true };
  }

  summary(participantId: string): ScoreSummary {
    const submissions = this.submissions.get(participantId) ?? new Map<string, Submission>();
    const correct = [...submissions.values()].filter((entry) => entry.correct).length;
    const passThreshold = this.pool.rules.passThreshold;
    return {
      assigned: this.pool.challenges.length,
      answered: submissions.size,
      correct,
      passThreshold,
      passed: correct >= passThreshold,
      transcript: this.pool.challenges.map((challenge) => ({
        challengeId: challenge.challengeId,
        answerDigest: submissions.get(challenge.challengeId)?.answerDigest ?? null,
      })),
    };
  }

  /**
   * Forgets expired records on reads and on enrolment. A live participant is
   * never evicted: a public lobby read must not destroy someone's token and
   * answers, and a full table refuses the newcomer instead (`AH-5`).
   */
  private sweepExpired(now: number): void {
    for (const [participantId, participant] of this.participants) {
      if (now >= participant.expiresAt) {
        this.forget(participantId);
      }
    }
  }

  private sweepExpiredNonces(now: number): void {
    for (const [nonce, entry] of this.nonces) {
      if (now >= entry.expiresAt) {
        this.nonces.delete(nonce);
      }
    }
  }

  private forget(participantId: string): void {
    const participant = this.participants.get(participantId);
    if (participant !== undefined) {
      this.byToken.delete(participant.tokenHash);
    }
    this.participants.delete(participantId);
    this.submissions.delete(participantId);
  }
}

/** The bound on remembered sessions. */
const MAX_PARTICIPANTS = 10_000;
/** The bound on unspent enrollment nonces. */
const MAX_ENROLLMENT_NONCES = 10_000;
/** How long an issued enrollment challenge stays usable. */
const ENROLLMENT_NONCE_MS = 5 * 60 * 1000;

function hashToken(token: string): string {
  return createHash("sha256").update(token, "utf8").digest("hex");
}
