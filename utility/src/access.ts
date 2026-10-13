// The access decision, and the local grant bookkeeping that goes with it.
//
// Order matters and is deliberate: the signature is checked before anything in
// the result is believed, then the policy, then expiry, and only then quota.
// Every refusal names a reason the caller can act on.
//
// Two limits are deliberate. No request field can claim an identity: a grant is
// scoped to a provider-issued participant id, and a policy that requires
// identity eligibility is refused rather than satisfied by a caller's word.
// A result that names an EVM/Idena address is only honoured when the presenter
// proved control of that address with a one-time utility challenge (`claims.ts`,
// `AH-6`), and such a grant is keyed by the address rather than the
// provider-issued participant id (`AH-7`). And the grant store is local
// bookkeeping - namespaced by utility, provider and either participant or
// address - so it cannot decide a limit that is advertised as global. Such a
// limit has to be enforced by canonical protocol state.

import { closeSync, fsyncSync, mkdtempSync, openSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import type { UtilityPolicy } from "./policy.ts";
import { policyEvidenceError } from "./policy.ts";
import { parseProviderResult, verifyResultSignature, type ProviderResult } from "./results.ts";
import { checkProviderScore } from "./score.ts";
import { scoreCommittedAnswers } from "../../app/src/score.ts";

export type AccessDecision =
  | {
      granted: true;
      reason: "granted";
      scope: UtilityPolicy["scope"];
      grantId: string;
      expiresAt: string;
      /**
       * This adapter performs no independent identity verification, so every
       * grant it issues says so. `provider-participant` is the whole scope.
       */
      identity: "not-verified";
      /** Present when the provider bound an EVM/Idena address to the result. */
      participantAddress?: string;
      /**
       * Present when this utility scored the session itself from the answers
       * the participant committed during the window, instead of accepting the
       * provider's signed score.
       */
      scoring?: { by: "utility"; assigned: number; answered: number; correct: number; passThreshold: number };
    }
  | {
      granted: false;
      reason:
        | "malformed_result"
        | "untrusted_provider"
        | "bad_signature"
        | "wrong_utility"
        | "unknown_session"
        | "wrong_session_content"
        | "outcome_not_accepted"
        | "result_expired"
        | "identity_required"
        | "claim_control_required"
        | "score_mismatch"
        | "score_unverifiable"
        | "answers_not_committed"
        | "score_not_passed"
        | "session_not_closed"
        | "quota_exhausted";
      detail?: string;
    };

/**
 * What one local grant is counted against. The kind is explicit because the
 * two scopes are different promises: a result that names an address is keyed
 * by that address (so re-enrolling under a new participant id cannot reset the
 * quota), and a result without one is keyed by the provider-issued participant
 * id, which is not a person.
 */
export interface GrantScopeKey {
  utilityId: string;
  providerId: string;
  kind: "provider-participant" | "wallet";
  participantId?: string;
  address?: string;
}

export interface GrantStore {
  /** Grants already issued for this utility, provider and participant. */
  count(key: GrantScopeKey, now: number): number;
  record(key: GrantScopeKey, grantId: string, expiresAt: string, now: number): void;
}

/** The bound on remembered keys. A reference adapter, not a durable store. */
const MAX_TRACKED_KEYS = 10_000;

export class InMemoryGrants implements GrantStore {
  private readonly grants = new Map<string, { grantId: string; expiresAt: number }[]>();

  count(key: GrantScopeKey, now: number): number {
    const id = scopeKey(key);
    const list = this.live(id, now);
    if (list === null) {
      return 0;
    }
    this.grants.set(id, list);
    return list.length;
  }

  record(key: GrantScopeKey, grantId: string, expiresAt: string, now: number): void {
    const id = scopeKey(key);
    const list = this.live(id, now) ?? [];
    if (!this.grants.has(id) && this.grants.size >= MAX_TRACKED_KEYS) {
      // Capacity never erases a still-live consumed allowance. Expiry may free
      // space; otherwise this request fails without changing the old grants.
      for (const existing of this.grants.keys()) this.live(existing, now);
      if (this.grants.size >= MAX_TRACKED_KEYS) throw new Error("local grant store is full; no live grants were evicted");
    }
    list.push({ grantId, expiresAt: Date.parse(expiresAt) });
    this.grants.set(id, list);
  }

  /** The unexpired entries under one key, or null when there are none. */
  private live(id: string, now: number): { grantId: string; expiresAt: number }[] | null {
    const list = this.grants.get(id);
    if (list === undefined) {
      return null;
    }
    const unexpired = list.filter((entry) => entry.expiresAt > now);
    if (unexpired.length === 0) {
      this.grants.delete(id);
      return null;
    }
    return unexpired;
  }
}

/**
 * A grant store that survives a restart by keeping its live entries in a file.
 *
 * The file is the operator's own local bookkeeping, not a global rule: it is
 * written 0600 and rewritten atomically, and it still cannot decide a limit
 * that an operator advertises as global (`SPEC.md` 10.3 clause 4). It exists so
 * that an unexpired local grant allowance survives restarting the utility.
 * Expired entries may be issued again; this is not a lifetime spent ledger.
 */
export class FileGrants implements GrantStore {
  static readonly format = "fod.utility-grants";
  static readonly version = 1;

  private readonly path: string;
  private readonly grants: Map<string, { grantId: string; expiresAt: number }[]>;

  private constructor(path: string, loaded: Map<string, { grantId: string; expiresAt: number }[]>) {
    this.path = path;
    this.grants = loaded;
  }

  static async open(path: string): Promise<FileGrants> {
    let loaded = new Map<string, { grantId: string; expiresAt: number }[]>();
    let raw: string;
    try {
      raw = await readFile(path, "utf8");
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "ENOENT") {
        return new FileGrants(path, loaded);
      }
      throw error;
    }
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) {
      throw new Error("the grant file is not an object");
    }
    const record = parsed as Record<string, unknown>;
    if (record.format !== FileGrants.format || record.version !== FileGrants.version) {
      throw new Error("unsupported grant file format");
    }
    const entries = record.grants;
    if (typeof entries !== "object" || entries === null) {
      throw new Error("the grant file names no grants");
    }
    loaded = new Map();
    for (const [scope, value] of Object.entries(entries as Record<string, unknown>)) {
      if (!Array.isArray(value)) {
        throw new Error("the grant file carries a malformed entry");
      }
      const list = value.map((entry) => {
        if (
          typeof entry !== "object" ||
          entry === null ||
          typeof (entry as Record<string, unknown>).grantId !== "string" ||
          !Number.isFinite((entry as Record<string, unknown>).expiresAt)
        ) {
          throw new Error("the grant file carries a malformed grant");
        }
        return {
          grantId: String((entry as Record<string, unknown>).grantId),
          expiresAt: Number((entry as Record<string, unknown>).expiresAt),
        };
      });
      loaded.set(scope, list);
    }
    return new FileGrants(path, loaded);
  }

  count(key: GrantScopeKey, now: number): number {
    const id = scopeKey(key);
    const list = this.live(id, now);
    if (list === null) {
      return 0;
    }
    this.grants.set(id, list);
    return list.length;
  }

  record(key: GrantScopeKey, grantId: string, expiresAt: string, now: number): void {
    const id = scopeKey(key);
    const list = this.live(id, now) ?? [];
    if (!this.grants.has(id) && this.grants.size >= MAX_TRACKED_KEYS) {
      // Capacity never erases a still-live consumed allowance. Expiry may free
      // space; otherwise this request fails without changing the old grants.
      for (const existing of this.grants.keys()) this.live(existing, now);
      if (this.grants.size >= MAX_TRACKED_KEYS) throw new Error("local grant store is full; no live grants were evicted");
    }
    list.push({ grantId, expiresAt: Date.parse(expiresAt) });
    this.grants.set(id, list);
    // The write happens after the in-memory update and synchronously, so a
    // failed write surfaces as the endpoint's error instead of a grant that was
    // recorded but never persisted.
    const body = {
      format: FileGrants.format,
      version: FileGrants.version,
      grants: Object.fromEntries(this.grants),
    };
    this.persist(JSON.stringify(body));
  }

  /** The unexpired entries under one key, or null when there are none. */
  private live(id: string, now: number): { grantId: string; expiresAt: number }[] | null {
    const list = this.grants.get(id);
    if (list === undefined) {
      return null;
    }
    const unexpired = list.filter((entry) => entry.expiresAt > now);
    if (unexpired.length === 0) {
      this.grants.delete(id);
      return null;
    }
    return unexpired;
  }

  private persist(body: string): void {
    // A fixed .tmp path could be an attacker-planted symlink. Create the
    // candidate exclusively inside a new private directory on the same
    // filesystem, then publish it atomically after flushing its contents.
    const directory = mkdtempSync(join(dirname(this.path), ".fod-grants-"));
    const temporary = join(directory, "grants.json");
    try {
      const file = openSync(temporary, "wx", 0o600);
      try {
        writeFileSync(file, body);
        fsyncSync(file);
      } finally {
        closeSync(file);
      }
      renameSync(temporary, this.path);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  }
}

function scopeKey(key: GrantScopeKey): string {
  return key.kind === "wallet"
    ? JSON.stringify([key.utilityId, key.providerId, "wallet", key.address?.toLowerCase() ?? ""])
    : JSON.stringify([key.utilityId, key.providerId, "provider-participant", key.participantId ?? ""]);
}

export interface AccessRequest {
  result: unknown;
  /**
   * Present only when the server verified the one-time challenge this utility
   * issued and the claimant's signature over it. The HTTP layer constructs it;
   * a request body cannot produce it directly.
   */
  claim?: { address: string };
  /**
   * The closed policy's `contentReveal`, sent by the claimant so this utility
   * can recompute the score itself. It is untrusted input: it is checked
   * against the digest this utility pinned before anything in it is believed.
   */
  scoreReveal?: unknown;
  /**
   * The answers this utility witnessed during the window, signed by the
   * claimant's wallet. The HTTP layer takes it from the commitment store; a
   * request body cannot produce it directly.
   */
  commitment?: { address: string; answers: { challengeId: string; answer: string }[] };
}

export async function decideAccess(
  request: AccessRequest,
  policy: UtilityPolicy,
  grants: GrantStore,
  now: number,
  newGrantId: () => string,
): Promise<AccessDecision> {
  const parsed = parseProviderResult(request.result);
  if (!parsed.ok) {
    return { granted: false, reason: "malformed_result" };
  }
  const result: ProviderResult = parsed.result;

  const provider = policy.providers.find((entry) => entry.providerId === result.providerId);
  if (provider === undefined) {
    return { granted: false, reason: "untrusted_provider", detail: result.providerId };
  }
  if (provider.publicKeyHex === "") {
    return {
      granted: false,
      reason: "untrusted_provider",
      detail: "policy holds no key for this provider, so nothing can be verified",
    };
  }
  if (!verifyResultSignature(provider.publicKeyHex, result)) {
    return { granted: false, reason: "bad_signature" };
  }
  if (result.utilityId !== policy.utilityId) {
    return { granted: false, reason: "wrong_utility", detail: result.utilityId };
  }
  if (!policy.sessions.includes(result.sessionId)) {
    return { granted: false, reason: "unknown_session", detail: result.sessionId };
  }
  // A session id is a name the provider chooses. When this utility pinned the
  // content it examined for that name, a result over other content is refused:
  // the same name must not mean an easier session.
  const pinned = policy.sessionDigests?.[result.sessionId];
  if (pinned !== undefined && pinned.toLowerCase() !== result.contentDigest.toLowerCase()) {
    return {
      granted: false,
      reason: "wrong_session_content",
      detail: `the result binds session content ${result.contentDigest}, and this utility accepts ${pinned}`,
    };
  }
  if (result.outcome !== policy.requireOutcome) {
    // A utility that scores the session itself does not let the provider's own
    // verdict decide: it derives the outcome from the committed answers below.
    if (policy.requireCommittedAnswers !== true) {
      return { granted: false, reason: "outcome_not_accepted", detail: result.outcome };
    }
  }
  const expiresAt = Date.parse(result.expiresAt);
  if (Number.isNaN(expiresAt) || now >= expiresAt) {
    return { granted: false, reason: "result_expired", detail: result.expiresAt };
  }
  // Two ways to stop believing a provider's score. A utility that witnessed the
  // answers itself scores the session with its own arithmetic over them and the
  // answer key the pinned content reveals; otherwise it recomputes the score the
  // provider signed from that provider's own transcript (`SPEC.md` 8.3).
  let utilityScore: { assigned: number; answered: number; correct: number; passThreshold: number } | undefined;
  if (policy.requireCommittedAnswers === true) {
    if (pinned === undefined) {
      return {
        granted: false,
        reason: "score_unverifiable",
        detail:
          "this policy scores sessions itself, so it has to pin the session content " +
          "(policy.sessionDigests) before the answers it witnessed can be scored",
      };
    }
    const window = policy.sessionWindows?.[result.sessionId];
    if (policyEvidenceError(policy) !== undefined || window === undefined) {
      return { granted: false, reason: "score_unverifiable", detail: policyEvidenceError(policy) };
    }
    if (now < Date.parse(window.closesAt)) {
      return { granted: false, reason: "session_not_closed" };
    }
    const reveal = request.scoreReveal as { utilityId?: unknown; rules?: { opensAt?: unknown; closesAt?: unknown } } | undefined;
    if (reveal === null || reveal?.utilityId !== policy.utilityId ||
        reveal.rules?.opensAt !== window.opensAt || reveal.rules?.closesAt !== window.closesAt) {
      return { granted: false, reason: "score_mismatch", detail: "the reveal does not bind this utility and its configured session window" };
    }
    // The commitment is only usable for the address the *signed* result names,
    // so one participant's answers cannot stand in for another's.
    const committed =
      result.participantAddress !== undefined &&
      request.commitment !== undefined &&
      request.commitment.address.toLowerCase() === result.participantAddress.toLowerCase()
        ? request.commitment
        : undefined;
    if (committed === undefined) {
      return {
        granted: false,
        reason: "answers_not_committed",
        detail:
          "this policy grants on the answers it witnessed while the session was open; " +
          "no signed answer commitment from this address reached it",
      };
    }
    const scored = await scoreCommittedAnswers({
      sessionId: result.sessionId,
      expectedDigest: pinned,
      reveal: request.scoreReveal,
      answers: committed.answers,
    });
    if (!scored.ok) {
      return { granted: false, reason: "score_mismatch", detail: scored.reason };
    }
    if (scored.correct < scored.passThreshold) {
      return {
        granted: false,
        reason: "score_not_passed",
        detail: `the committed answers score ${scored.correct} of ${scored.assigned} against the pinned content, and ${scored.passThreshold} are needed`,
      };
    }
    utilityScore = {
      assigned: scored.assigned,
      answered: scored.answered,
      correct: scored.correct,
      passThreshold: scored.passThreshold,
    };
  } else {
    const score = await checkProviderScore({
      result,
      reveal: request.scoreReveal,
      expectedDigest: pinned ?? result.contentDigest,
      required: policy.requireScoreProof === true,
    });
    if (score !== null) {
      return { granted: false, reason: score.reason, detail: score.detail };
    }
  }
  // A policy may ask for identity eligibility, but this adapter cannot check
  // it and a caller's assertion is not evidence. Until eligibility is proved
  // from chain-committed epoch state together with control of that identity,
  // every identity-dependent request is refused.
  if (policy.identityVerification?.required === true) {
    return {
      granted: false,
      reason: "identity_required",
      detail:
        "this adapter verifies no identity: eligibility must come from chain-committed " +
        "epoch state together with proof of control of that identity, and no such check exists yet",
    };
  }
  const address = result.participantAddress?.toLowerCase();
  // A result that names an address is honoured only when the presenter proved
  // control of it: the provider's signature says what the provider stated, not
  // who is asking (`AH-6`).
  if (address !== undefined && request.claim?.address.toLowerCase() !== address) {
    return {
      granted: false,
      reason: "claim_control_required",
      detail: `the result names ${address}, and the claim did not prove control of it`,
    };
  }
  const unsupported = policyEvidenceError(policy);
  if (unsupported !== undefined || address === undefined) {
    return { granted: false, reason: "score_unverifiable", detail: unsupported ?? "a grant requires a wallet-bound answer commitment and control proof" };
  }
  const key: GrantScopeKey =
    address === undefined
      ? {
          utilityId: policy.utilityId,
          providerId: result.providerId,
          kind: "provider-participant",
          participantId: result.participantId,
        }
      : {
          utilityId: policy.utilityId,
          providerId: result.providerId,
          kind: "wallet",
          address,
        };
  if (grants.count(key, now) >= policy.grantsPerParticipant) {
    return { granted: false, reason: "quota_exhausted", detail: result.participantId };
  }

  const grantExpiry = new Date(
    Math.min(now + policy.grantTtlSeconds * 1000, expiresAt),
  );
  const grantId = newGrantId();
  grants.record(key, grantId, grantExpiry.toISOString(), now);
  return {
    granted: true,
    reason: "granted",
    scope: policy.scope,
    grantId,
    expiresAt: grantExpiry.toISOString(),
    identity: "not-verified",
    ...(address === undefined ? {} : { participantAddress: address }),
    ...(utilityScore === undefined ? {} : { scoring: { by: "utility" as const, ...utilityScore } }),
  };
}
