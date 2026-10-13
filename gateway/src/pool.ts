// The exported flip pool a provider scores against.
//
// The adapter trusts this file, and says so: the provider builds it from the
// bundles it acquired, whose provenance lives in the marketplace and the CLI's
// inventory export. A utility that needs independent proof of that provenance
// checks the marketplace, not this gateway. What the adapter does enforce is
// that the pool names where it came from, so an unsigned blob cannot pass as a
// session pool.

import { readFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import { readFlip, variantsOf } from "./flip.ts";
import { canonicalJson } from "./keys.ts";

export interface Challenge {
  challengeId: string;
  cid: string;
  /** SHA-256 of the canonical answer text, as the provider scored it. */
  answerDigest: string;
  /**
   * The two sequences a participant chooses between, each four data URLs in
   * display order. Present when the pool carries the flip's decrypted content;
   * a challenge without it can only be answered from what the participant
   * already has.
   */
  variants?: string[][];
}

export interface FlipPool {
  format: "fod.flip-pool";
  version: 1;
  sessionId: string;
  utilityId: string;
  provenance: {
    epoch: number;
    author: string;
    source: string;
    blockRange: string;
    /**
     * What the provider's answers rest on, as the pool carries it. It is the
     * provider's own statement, shown to participants as such.
     */
    answers?: string;
  };
  /** Timing and scoring rules, published to participants as they are. */
  rules: {
    passThreshold: number;
    opensAt: string;
    closesAt: string;
  };
  /**
   * A per-session secret the content digest is salted with (`AH-2`). The pool
   * file is the provider's own copy and is never served; the public policy
   * carries only the digest. Because the salt is inside the digest preimage
   * and is revealed only after the session closes, a published digest cannot
   * be enumerated offline against a small answer pool before claims close.
   */
  answerSalt: string;
  challenges: Challenge[];
  /**
   * The digest of what this session is: the session and utility ids, the rules,
   * the provenance the provider states, and every challenge with the CID and
   * answer digest it scores against (and the pictures it shows, as their own
   * digest). It travels in the signed result, so a utility that pins it grants
   * only for the session it examined rather than for whatever the provider
   * later calls by the same name.
   */
  contentDigest: string;
}

const HEX_64 = /^[0-9a-f]{64}$/;
const HEX_40 = /^[0-9a-f]{40}$/;
// Session ids travel inside the enrollment challenge text, which is
// colon-delimited, so the characters a colon could split are refused here.
const SESSION_ID = /^[A-Za-z0-9._-]{1,64}$/;

/** The exact shape a session digest commits to (`describeContent` below). */
export interface ContentDescription {
  sessionId: string;
  utilityId: string;
  rules: FlipPool["rules"];
  provenance: FlipPool["provenance"];
  answerSalt: string;
  challenges: {
    challengeId: string;
    cid: string;
    answerDigest: string;
    contentSha256: string | null;
  }[];
}

/**
 * The exact content the session digest commits to. It is security-sensitive:
 * it carries the answer digests the provider scores against and the salt that
 * keeps them unenumerable while the session is open, so it is published only
 * once the session is closed.
 */
export function describeContent(pool: {
  sessionId: string;
  utilityId: string;
  rules: FlipPool["rules"];
  provenance: FlipPool["provenance"];
  answerSalt: string;
  challenges: Challenge[];
}): ContentDescription {
  return {
    sessionId: pool.sessionId,
    utilityId: pool.utilityId,
    rules: pool.rules,
    provenance: pool.provenance,
    answerSalt: pool.answerSalt,
    challenges: pool.challenges.map((challenge) => ({
      challengeId: challenge.challengeId,
      cid: challenge.cid,
      answerDigest: challenge.answerDigest,
      // The pictures are part of what a participant answered, but hashing them
      // separately keeps the pinned form small.
      contentSha256:
        challenge.variants === undefined
          ? null
          : createHash("sha256").update(canonicalJson(challenge.variants), "utf8").digest("hex"),
    })),
  };
}

/** The digest of an already-described session content (the reveal, or a pool). */
export function digestOfContent(described: ContentDescription): string {
  return createHash("sha256").update(canonicalJson(described), "utf8").digest("hex");
}

/** The digest a session's content is pinned by (`contentDigest` above). */
export function sessionContentDigest(pool: {
  sessionId: string;
  utilityId: string;
  rules: FlipPool["rules"];
  provenance: FlipPool["provenance"];
  answerSalt: string;
  challenges: Challenge[];
}): string {
  return digestOfContent(describeContent(pool));
}

export async function loadPool(path: string): Promise<FlipPool> {
  const parsed: unknown = JSON.parse(await readFile(path, "utf8"));
  if (typeof parsed !== "object" || parsed === null) {
    throw new Error("pool must be an object");
  }
  const pool = parsed as Record<string, unknown>;
  if (pool.format !== "fod.flip-pool" || pool.version !== 1) {
    throw new Error("unsupported pool format");
  }
  const provenance = pool.provenance;
  if (typeof provenance !== "object" || provenance === null) {
    throw new Error("pool.provenance is required");
  }
  const meta = provenance as Record<string, unknown>;
  if (!Number.isInteger(meta.epoch) || typeof meta.blockRange !== "string" || meta.blockRange === "") {
    throw new Error("pool.provenance must name the epoch and block range");
  }
  if (typeof meta.author !== "string" || !HEX_40.test(meta.author)) {
    throw new Error("pool.provenance.author must be an address");
  }
  if (typeof meta.source !== "string" || meta.source === "") {
    throw new Error("pool.provenance.source is required");
  }
  if (!Array.isArray(pool.challenges) || pool.challenges.length === 0) {
    throw new Error("pool.challenges must not be empty");
  }
  const rules = pool.rules;
  if (typeof rules !== "object" || rules === null) {
    throw new Error("pool.rules is required");
  }
  const ruleRecord = rules as Record<string, unknown>;
  if (
    !Number.isInteger(ruleRecord.passThreshold) ||
    (ruleRecord.passThreshold as number) <= 0 ||
    (ruleRecord.passThreshold as number) > pool.challenges.length
  ) {
    throw new Error("pool.rules.passThreshold must be between 1 and the number of challenges");
  }
  for (const field of ["opensAt", "closesAt"] as const) {
    if (typeof ruleRecord[field] !== "string" || Number.isNaN(Date.parse(ruleRecord[field] as string))) {
      throw new Error(`pool.rules.${field} must be a timestamp`);
    }
  }
  // A window that closes before it opens would leave the session in no state at
  // all, so it is refused here rather than half-enforced at request time.
  if (Date.parse(ruleRecord.closesAt as string) <= Date.parse(ruleRecord.opensAt as string)) {
    throw new Error("pool.rules.closesAt must be after pool.rules.opensAt");
  }
  const challenges: Challenge[] = pool.challenges.map((entry, index) => {
    if (typeof entry !== "object" || entry === null) {
      throw new Error(`pool.challenges[${index}] must be an object`);
    }
    const challenge = entry as Record<string, unknown>;
    if (typeof challenge.challengeId !== "string" || challenge.challengeId === "") {
      throw new Error(`pool.challenges[${index}].challengeId is required`);
    }
    if (typeof challenge.answerDigest !== "string" || !HEX_64.test(challenge.answerDigest)) {
      throw new Error(`pool.challenges[${index}].answerDigest must be a SHA-256 digest`);
    }
    // The content is optional, but never trusted blindly: a pool that carries
    // it has to carry a flip the renderer can show (`SOURCES.md` F-98).
    let variants: string[][] | undefined;
    if (challenge.content !== undefined) {
      if (typeof challenge.content !== "object" || challenge.content === null) {
        throw new Error(`pool.challenges[${index}].content must be an object`);
      }
      const content = challenge.content as Record<string, unknown>;
      try {
        variants = variantsOf(readFlip(content.publicPart, content.privatePart));
      } catch (problem) {
        throw new Error(
          `pool.challenges[${index}].content is not a flip: ${
            problem instanceof Error ? problem.message : String(problem)
          }`
        );
      }
      if (variants.some((variant) => variant.length === 0)) {
        throw new Error(`pool.challenges[${index}].content has an empty sequence`);
      }
    }
    return {
      challengeId: challenge.challengeId,
      cid: typeof challenge.cid === "string" ? challenge.cid : "",
      answerDigest: challenge.answerDigest,
      ...(variants === undefined ? {} : { variants }),
    };
  });
  const answerSalt = stringField(pool.answerSalt, "pool.answerSalt").toLowerCase();
  if (!HEX_64.test(answerSalt)) {
    throw new Error("pool.answerSalt must be a 32-byte hex value");
  }
  const sessionId = stringField(pool.sessionId, "pool.sessionId");
  if (!SESSION_ID.test(sessionId)) {
    throw new Error("pool.sessionId must use letters, digits, dot, underscore or dash");
  }
  const loaded: Omit<FlipPool, "contentDigest"> = {
    format: "fod.flip-pool",
    version: 1,
    sessionId,
    utilityId: stringField(pool.utilityId, "pool.utilityId"),
    provenance: {
      epoch: meta.epoch as number,
      author: meta.author,
      source: meta.source,
      blockRange: meta.blockRange,
      ...(meta.answers === undefined ? {} : { answers: stringField(meta.answers, "pool.provenance.answers") }),
    },
    rules: {
      passThreshold: ruleRecord.passThreshold as number,
      opensAt: ruleRecord.opensAt as string,
      closesAt: ruleRecord.closesAt as string,
    },
    answerSalt,
    challenges,
  };
  return {
    ...loaded,
    contentDigest: sessionContentDigest(loaded),
  };
}

function stringField(value: unknown, field: string): string {
  if (typeof value !== "string" || value === "") {
    throw new Error(`${field} is required`);
  }
  return value;
}
