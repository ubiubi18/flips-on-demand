// Verifying a provider result from the utility's side.
//
// This is a second, independent implementation of the canonical form and the
// Ed25519 check: the utility is a different party from the gateway, and
// "it verified" should mean the utility checked the bytes itself, not that it
// reused the issuer's helper.

import { createPublicKey, verify } from "node:crypto";

export interface ProviderResult {
  format: string;
  version: number;
  providerId: string;
  sessionId: string;
  utilityId: string;
  /**
   * What the session was: the digest the provider's own policy published over
   * the rules, the stated provenance and every challenge with the answer it
   * scores against. A utility that pins it in its policy refuses a result that
   * names the same session id over different content.
   */
  contentDigest: string;
  participantId: string;
  /** The EVM/Idena address the enrollment bound, when the provider recorded one. */
  participantAddress?: string;
  issuedAt: string;
  expiresAt: string;
  score: { assigned: number; answered: number; correct: number; passThreshold: number };
  /**
   * The answers the provider says it scored: one entry per assigned challenge
   * with the digest of the recorded answer, or `null` where it recorded none.
   * With the closed policy's content reveal this is what lets a second party
   * recompute the score instead of believing the summary.
   */
  transcript?: { rule?: string; entries: { challengeId: string; answerDigest: string | null }[] };
  outcome: string;
  issuedBy: { kind: string; providerId: string; note: string };
  algorithm: string;
  signature: string;
}

export type ParseOutcome =
  | { ok: true; result: ProviderResult }
  | { ok: false; reason: "malformed_result" };

export function parseProviderResult(value: unknown): ParseOutcome {
  if (typeof value !== "object" || value === null) {
    return { ok: false, reason: "malformed_result" };
  }
  const record = value as Record<string, unknown>;
  const strings = ["format", "providerId", "sessionId", "utilityId", "participantId", "issuedAt", "expiresAt", "outcome", "algorithm", "signature"] as const;
  for (const field of strings) {
    if (typeof record[field] !== "string" || record[field] === "") {
      return { ok: false, reason: "malformed_result" };
    }
  }
  if (record.format !== "fod.session-result" || record.version !== 1 || record.algorithm !== "ed25519") {
    return { ok: false, reason: "malformed_result" };
  }
  if (typeof record.contentDigest !== "string" || !/^[0-9a-f]{64}$/i.test(record.contentDigest)) {
    return { ok: false, reason: "malformed_result" };
  }
  const score = record.score;
  if (typeof score !== "object" || score === null) {
    return { ok: false, reason: "malformed_result" };
  }
  const scoreRecord = score as Record<string, unknown>;
  for (const field of ["assigned", "answered", "correct", "passThreshold"] as const) {
    if (!Number.isInteger(scoreRecord[field]) || (scoreRecord[field] as number) < 0) {
      return { ok: false, reason: "malformed_result" };
    }
  }
  if (Number.isNaN(Date.parse(record.issuedAt as string)) || Number.isNaN(Date.parse(record.expiresAt as string))) {
    return { ok: false, reason: "malformed_result" };
  }
  if (!/^[0-9a-f]+$/i.test(record.signature as string)) {
    return { ok: false, reason: "malformed_result" };
  }
  const transcript = record.transcript;
  if (transcript !== undefined) {
    if (typeof transcript !== "object" || transcript === null) {
      return { ok: false, reason: "malformed_result" };
    }
    const entries = (transcript as { entries?: unknown }).entries;
    if (!Array.isArray(entries) || entries.length === 0) {
      return { ok: false, reason: "malformed_result" };
    }
    for (const entry of entries) {
      if (typeof entry !== "object" || entry === null) {
        return { ok: false, reason: "malformed_result" };
      }
      const { challengeId, answerDigest } = entry as { challengeId?: unknown; answerDigest?: unknown };
      if (typeof challengeId !== "string" || challengeId === "") {
        return { ok: false, reason: "malformed_result" };
      }
      if (answerDigest === null) {
        continue;
      }
      if (typeof answerDigest !== "string" || !/^[0-9a-f]{64}$/i.test(answerDigest)) {
        return { ok: false, reason: "malformed_result" };
      }
    }
  }
  if (
    record.participantAddress !== undefined &&
    (typeof record.participantAddress !== "string" || !/^0x[0-9a-f]{40}$/i.test(record.participantAddress))
  ) {
    return { ok: false, reason: "malformed_result" };
  }
  return { ok: true, result: value as ProviderResult };
}

/** Canonical JSON: object keys sorted, no insignificant whitespace. */
export function canonicalJson(value: unknown): string {
  if (value === null || typeof value !== "object") {
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return `[${value.map(canonicalJson).join(",")}]`;
  }
  const entries = Object.entries(value as Record<string, unknown>)
    .filter(([, entry]) => entry !== undefined)
    .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0));
  return `{${entries.map(([key, entry]) => `${JSON.stringify(key)}:${canonicalJson(entry)}`).join(",")}}`;
}

export function verifyResultSignature(publicKeyHex: string, result: ProviderResult): boolean {
  const { signature, ...unsigned } = result;
  try {
    const publicKey = createPublicKey({
      key: Buffer.from(publicKeyHex, "hex"),
      format: "der",
      type: "spki",
    });
    return verify(
      null,
      Buffer.from(canonicalJson(unsigned), "utf8"),
      publicKey,
      Buffer.from(signature, "hex"),
    );
  } catch {
    return false;
  }
}
