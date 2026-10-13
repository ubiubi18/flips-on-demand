// What the utility will accept.
//
// The utility is a separate party from the provider: it decides which providers,
// sessions, and outcomes it honours, and it says plainly what a grant is scoped
// to. SPEC.md 8.3 requires that scope to be honest - a provider-issued
// participant id is not a person, and a wallet-only quota must never be
// described as one-person-one-allocation.

import { readFile } from "node:fs/promises";

export interface ProviderTrust {
  providerId: string;
  publicKeyHex: string;
  gatewayUrl?: string;
}

export interface UtilityPolicy {
  format: "fod.utility-policy";
  version: 1;
  utilityId: string;
  sessions: string[];
  requireOutcome: "passed";
  grantsPerParticipant: number;
  grantTtlSeconds: number;
  providers: ProviderTrust[];
  scope: { kind: "provider-participant"; statement: string };
  /**
   * Required for grants: a session id mapped to the digest of the session
   * content the operator examined (the provider's own `contentDigest`). When a
   * result names a session this pins, a different digest is refused, so a
   * provider cannot rerun the same session id over easier content. A session
   * with no entry cannot authorize a grant.
   */
  sessionDigests?: Record<string, string>;
  /**
   * Browser origins allowed to call the endpoint. `["null"]` is the offline
   * `file://` page and `["*"]` is a public testbed; a production host lists its
   * exact dapp origins. Omitting the field allows no browser origin - the
   * endpoint sends no CORS header - so a host has to pin its origins rather
   * than inherit a wildcard. Non-browser callers are unaffected by CORS.
   */
  origins?: string[];
  /** Public origin used to bind wallet signatures behind a TLS proxy. */
  publicOrigin?: string;
  /**
   * When true, a grant needs a score this utility recomputed itself: the
   * claimant sends the closed policy's `contentReveal` and the result has to
   * carry the transcript the provider scored. `score_unverifiable` refuses a
   * result without it, and `score_mismatch` refuses one whose arithmetic does
   * not reproduce the signed score.
   */
  requireScoreProof?: boolean;
  /**
   * The window each session ran in. A utility that scores a session itself
   * needs its own close time: an answer commitment is only accepted while the
   * window is open, which is what stops answers being tailored to a key that
   * was published afterwards.
   */
  sessionWindows?: Record<string, { opensAt: string; closesAt: string }>;
  /**
   * When true, the grant is decided by this utility's own arithmetic over the
   * answers the participant committed while the window was open, not by the
   * provider's signed score. The provider's result is still required, but only
   * as the session and participant binding; a missing commitment is
   * `answers_not_committed` and a commitment that does not pass is
   * `score_not_passed`.
   */
  requireCommittedAnswers?: boolean;
  /**
   * Whether this utility wants identity-based eligibility. There is no field
   * through which a caller or a server can assert that eligibility: while this
   * is `required`, every request is refused, because the check this adapter
   * would need - chain-committed epoch state plus control of that identity -
   * does not exist here yet. `note` is the operator's own explanation.
   */
  identityVerification?: { required: boolean; note: string };
}

export async function loadPolicy(path: string): Promise<UtilityPolicy> {
  const parsed: unknown = JSON.parse(await readFile(path, "utf8"));
  if (typeof parsed !== "object" || parsed === null) {
    throw new Error("policy must be an object");
  }
  const record = parsed as Record<string, unknown>;
  if (record.format !== "fod.utility-policy" || record.version !== 1) {
    throw new Error("unsupported policy format");
  }
  if (!Array.isArray(record.sessions) || record.sessions.length === 0) {
    throw new Error("policy.sessions must list the accepted sessions");
  }
  if (record.requireOutcome !== "passed") {
    throw new Error("policy.requireOutcome must be \"passed\"");
  }
  if (!Number.isInteger(record.grantsPerParticipant) || (record.grantsPerParticipant as number) < 1) {
    throw new Error("policy.grantsPerParticipant must be a positive integer");
  }
  if (!Number.isInteger(record.grantTtlSeconds) || (record.grantTtlSeconds as number) < 1) {
    throw new Error("policy.grantTtlSeconds must be a positive integer");
  }
  if (!Array.isArray(record.providers) || record.providers.length === 0) {
    throw new Error("policy.providers must list at least one trusted provider");
  }
  const providers: ProviderTrust[] = record.providers.map((entry, index) => {
    if (typeof entry !== "object" || entry === null) {
      throw new Error(`policy.providers[${index}] must be an object`);
    }
    const provider = entry as Record<string, unknown>;
    if (typeof provider.providerId !== "string" || provider.providerId === "") {
      throw new Error(`policy.providers[${index}].providerId is required`);
    }
    if (typeof provider.publicKeyHex !== "string" || !/^[0-9a-f]*$/i.test(provider.publicKeyHex)) {
      throw new Error(`policy.providers[${index}].publicKeyHex must be hex`);
    }
    return {
      providerId: provider.providerId,
      publicKeyHex: provider.publicKeyHex.toLowerCase(),
      gatewayUrl: typeof provider.gatewayUrl === "string" ? provider.gatewayUrl : undefined,
    };
  });
  const scope = record.scope;
  if (typeof scope !== "object" || scope === null) {
    throw new Error("policy.scope is required: a grant has to say what it is scoped to");
  }
  const scopeRecord = scope as Record<string, unknown>;
  if (scopeRecord.kind !== "provider-participant" || typeof scopeRecord.statement !== "string") {
    throw new Error("policy.scope must name its kind and state its meaning");
  }
  const identity = record.identityVerification;
  const digests = record.sessionDigests;
  if (digests !== undefined && (typeof digests !== "object" || digests === null)) {
    throw new Error("policy.sessionDigests must be an object of session id to digest");
  }
  const sessionDigests: Record<string, string> | undefined =
    digests === undefined
      ? undefined
      : Object.fromEntries(
          Object.entries(digests as Record<string, unknown>).map(([sessionId, digest]) => {
            if (typeof digest !== "string" || !/^[0-9a-f]{64}$/i.test(digest)) {
              throw new Error(`policy.sessionDigests["${sessionId}"] must be a SHA-256 digest`);
            }
            return [sessionId, digest.toLowerCase()];
          }),
        );
  const origins = record.origins;
  const publicOrigin = record.publicOrigin;
  if (publicOrigin !== undefined) {
    if (typeof publicOrigin !== "string") {
      throw new Error("policy.publicOrigin must be an http(s) origin, using https outside loopback");
    }
    let url: URL;
    try { url = new URL(publicOrigin); }
    catch { throw new Error("policy.publicOrigin must be an http(s) origin, using https outside loopback"); }
    const loopback = url.hostname === "localhost" || url.hostname === "[::1]" || /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(url.hostname);
    if ((url.protocol !== "https:" && !(url.protocol === "http:" && loopback)) ||
        url.origin !== publicOrigin || url.username !== "" || url.password !== "") {
      throw new Error("policy.publicOrigin must be an http(s) origin, using https outside loopback");
    }
  }
  const requireScoreProof = record.requireScoreProof;
  if (requireScoreProof !== undefined && typeof requireScoreProof !== "boolean") {
    throw new Error("policy.requireScoreProof must be true or false");
  }
  const requireCommittedAnswers = record.requireCommittedAnswers;
  if (requireCommittedAnswers !== undefined && typeof requireCommittedAnswers !== "boolean") {
    throw new Error("policy.requireCommittedAnswers must be true or false");
  }
  const windows = record.sessionWindows;
  if (windows !== undefined && (typeof windows !== "object" || windows === null)) {
    throw new Error("policy.sessionWindows must be an object of session id to a window");
  }
  const sessionWindows: Record<string, { opensAt: string; closesAt: string }> | undefined =
    windows === undefined
      ? undefined
      : Object.fromEntries(
          Object.entries(windows as Record<string, unknown>).map(([sessionId, value]) => {
            if (typeof value !== "object" || value === null) {
              throw new Error(`policy.sessionWindows["${sessionId}"] must name opensAt and closesAt`);
            }
            const window = value as { opensAt?: unknown; closesAt?: unknown };
            const opensAt = Date.parse(typeof window.opensAt === "string" ? window.opensAt : "");
            const closesAt = Date.parse(typeof window.closesAt === "string" ? window.closesAt : "");
            if (Number.isNaN(opensAt) || Number.isNaN(closesAt) || closesAt <= opensAt) {
              throw new Error(`policy.sessionWindows["${sessionId}"] must carry RFC 3339 times, closesAt after opensAt`);
            }
            return [sessionId, { opensAt: window.opensAt as string, closesAt: window.closesAt as string }];
          }),
        );
  if (origins !== undefined) {
    if (!Array.isArray(origins) || origins.length === 0) {
      throw new Error("policy.origins must be a non-empty array when present");
    }
    for (const [index, origin] of origins.entries()) {
      if (typeof origin !== "string" || origin === "") {
        throw new Error(`policy.origins[${index}] must be a non-empty string`);
      }
      if (origin === "*" || origin === "null") {
        continue;
      }
      let parsed: URL;
      try {
        parsed = new URL(origin);
      } catch {
        throw new Error(`policy.origins[${index}] must be "*", "null", or an http(s) origin`);
      }
      if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || parsed.origin !== origin) {
        throw new Error(`policy.origins[${index}] must be an http(s) origin without a path`);
      }
    }
  }
  const policy: UtilityPolicy = {
    format: "fod.utility-policy",
    version: 1,
    utilityId: stringField(record.utilityId, "policy.utilityId"),
    sessions: record.sessions.map((entry) => stringField(entry, "policy.sessions[]")),
    requireOutcome: "passed",
    grantsPerParticipant: record.grantsPerParticipant as number,
    grantTtlSeconds: record.grantTtlSeconds as number,
    providers,
    scope: { kind: "provider-participant", statement: scopeRecord.statement },
    ...(sessionDigests === undefined ? {} : { sessionDigests }),
    ...(origins === undefined ? {} : { origins: origins as string[] }),
    ...(publicOrigin === undefined ? {} : { publicOrigin }),
    ...(requireScoreProof === undefined ? {} : { requireScoreProof }),
    ...(sessionWindows === undefined ? {} : { sessionWindows }),
    ...(requireCommittedAnswers === undefined ? {} : { requireCommittedAnswers }),
    identityVerification:
      typeof identity === "object" && identity !== null
        ? {
            required: (identity as Record<string, unknown>).required === true,
            note: stringField((identity as Record<string, unknown>).note, "policy.identityVerification.note"),
          }
        : undefined,
  };
  const unsupported = policyEvidenceError(policy);
  if (unsupported !== undefined) throw new Error(unsupported);
  return policy;
}

/** Transcript arithmetic alone cannot establish that a wallet submitted it. */
export function policyEvidenceError(policy: UtilityPolicy): string | undefined {
  if (policy.requireCommittedAnswers !== true) {
    return "grants require wallet-signed answers witnessed during the pinned session window; a provider transcript or passed signature is insufficient";
  }
  for (const session of policy.sessions) {
    const window = policy.sessionWindows?.[session];
    if (!/^[0-9a-f]{64}$/i.test(policy.sessionDigests?.[session] ?? "") || window === undefined ||
        !Number.isFinite(Date.parse(window.opensAt)) || !Number.isFinite(Date.parse(window.closesAt)) ||
        Date.parse(window.closesAt) <= Date.parse(window.opensAt)) {
      return `grants require a content pin and an independently configured window for ${session}`;
    }
  }
  return undefined;
}

function stringField(value: unknown, field: string): string {
  if (typeof value !== "string" || value === "") {
    throw new Error(`${field} is required`);
  }
  return value;
}
