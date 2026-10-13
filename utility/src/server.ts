// The utility's endpoint: one POST with a provider result, one decision.
//
// Like the gateway, it has an explicit surface, a body cap, and a rate limit,
// and it never sets a cookie. It holds no provider key beyond the public ones in
// its policy, and it cannot issue results.

import { createServer } from "node:http";
import type { IncomingMessage, Server, ServerResponse } from "node:http";
import { randomUUID } from "node:crypto";
import { decideAccess, FileGrants, InMemoryGrants, type GrantStore } from "./access.ts";
import { ClaimChallenges, isClaimAddress } from "./claims.ts";
import { AnswerCommitments } from "./commitments.ts";
import type { UtilityPolicy } from "./policy.ts";

export interface UtilityServerOptions {
  policy: UtilityPolicy;
  bodyLimitBytes?: number;
  rateLimit?: { windowSeconds: number; maxRequests: number };
  now?: () => number;
  grants?: GrantStore;
  /** True when `grants` is a FileGrants store, so `/policy` can say so. */
  grantsDurable?: boolean;
  challenges?: ClaimChallenges;
  /** The answers this utility witnessed during a session's window. */
  commitments?: AnswerCommitments;
  /**
   * Peer addresses whose `X-Forwarded-For` may be believed, most commonly the
   * loopback address of the TLS proxy in front of this utility. With none
   * configured the header is ignored, so a direct caller cannot rotate it to
   * mint fresh rate-limit buckets (`AH-4`).
   */
  trustedProxies?: string[];
}

export function createUtilityServer(options: UtilityServerOptions): Server {
  const bodyLimit = options.bodyLimitBytes ?? 32 * 1024;
  const rateLimit = options.rateLimit ?? { windowSeconds: 60, maxRequests: 120 };
  const now = options.now ?? (() => Date.now());
  const grants = options.grants ?? new InMemoryGrants();
  const challenges = options.challenges ?? new ClaimChallenges();
  const commitments = options.commitments ?? new AnswerCommitments();
  const trustedProxies = (options.trustedProxies ?? []).map(normalizeIp);
  const windows = new Map<string, { start: number; count: number }>();

  const handle = async (req: IncomingMessage, res: ServerResponse): Promise<void> => {
    res.setHeader("X-Content-Type-Options", "nosniff");
    res.setHeader("Cache-Control", "no-store");
    // The decision is a signature check, not a cookie or an origin capability.
    // The policy decides which browser origins may read the answer. An explicit
    // `["*"]` is the public testbed; a policy that names origins gets exactly
    // those, echoed so the browser can read the answer; a policy that names no
    // origin gets no CORS header at all, so a hosted utility pins its own
    // origins instead of inheriting a wildcard.
    const allowedOrigins = options.policy.origins ?? [];
    const requestOrigin = String(req.headers.origin ?? "");
    const allowOrigin = allowedOrigins.includes("*")
      ? "*"
      : requestOrigin !== "" && allowedOrigins.includes(requestOrigin)
        ? requestOrigin
        : "";
    if (allowOrigin !== "") {
      res.setHeader("Access-Control-Allow-Origin", allowOrigin);
      res.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
      res.setHeader("Access-Control-Allow-Headers", "content-type");
      if (allowOrigin !== "*") {
        res.setHeader("Vary", "Origin");
      }
    }
    try {
      // A Host header is caller-supplied, so it is parsed inside the guard: an
      // unparseable one is this request's 400, never the process's exit.
      const url = requestUrl(req);
      if (req.method === "OPTIONS") {
        res.statusCode = 204;
        res.end();
        return;
      }
      if (req.method === "GET" && url.pathname === "/policy") {
        return respond(res, 200, {
          utilityId: options.policy.utilityId,
          sessions: options.policy.sessions,
          requireOutcome: options.policy.requireOutcome,
          grantsPerParticipant: options.policy.grantsPerParticipant,
          grantTtlSeconds: options.policy.grantTtlSeconds,
          scope: options.policy.scope,
          sessionDigests: options.policy.sessionDigests ?? null,
          // The pin the server is enforcing, so a host can confirm it from the
          // running endpoint instead of from the file it believes it passed.
          origins: options.policy.origins ?? null,
          publicOrigin: options.policy.publicOrigin ?? null,
          // Whether a grant carries a score this utility recomputed itself
          // instead of trusting the provider's summary (`SPEC.md` 8.3).
          requireScoreProof: options.policy.requireScoreProof === true,
          // Whether a grant is decided by this utility's own arithmetic over
          // the answers it witnessed, and the window it enforced for that.
          requireCommittedAnswers: options.policy.requireCommittedAnswers === true,
          sessionWindows: options.policy.sessionWindows ?? null,
          identityVerification:
            options.policy.identityVerification === undefined
              ? null
              : {
                  required: options.policy.identityVerification.required,
                  // This adapter holds no identity check, so a policy that
                  // requires one makes every request refused rather than
                  // satisfied by a caller's or a server's assertion.
                  supported: false,
                  note: options.policy.identityVerification.note,
                },
          quota: {
            scope: "local-process",
            durable: options.grantsDurable ?? options.grants instanceof FileGrants,
            keyedBy:
              "wallet when the result names an address (proved with a one-time challenge), " +
              "otherwise the provider-issued participant id",
            note:
              "grants are counted in this process only, per utility, provider and participant; " +
              "a limit that is claimed to hold globally needs canonical protocol state",
          },
        });
      }
      const isAccess = req.method === "POST" && url.pathname === "/access";
      const isChallenge = req.method === "POST" && url.pathname === "/access-challenge";
      const isCommitment = req.method === "POST" && url.pathname === "/answer-commitment";
      if (!isAccess && !isChallenge && !isCommitment) {
        const known =
          url.pathname === "/access" ||
          url.pathname === "/access-challenge" ||
          url.pathname === "/answer-commitment" ||
          url.pathname === "/policy";
        return respond(res, known ? 405 : 404, { error: known ? "method_not_allowed" : "not_found" });
      }
      const key = clientAddress(req, trustedProxies);
      if (!allowRateLimit(windows, rateLimit, key, now())) {
        return respond(res, 429, { error: "rate_limited" });
      }
      const body = await readJson(req, bodyLimit);
      if (isChallenge) {
        const address = typeof body?.address === "string" ? body.address : "";
        if (!isClaimAddress(address)) {
          return respond(res, 400, { error: "bad_address" });
        }
        const issued = challenges.issue(options.policy.utilityId, address, walletOrigin(req, options.policy), now());
        if (issued === null) {
          return respond(res, 503, { error: "busy" });
        }
        return respond(res, 201, {
          challenge: issued.challenge,
          expiresAt: new Date(issued.expiresAt).toISOString(),
        });
      }
      if (isCommitment) {
        // The answers a participant commits while the window is open. This
        // utility is the witness: it enforces the window with its own clock and
        // scores the session itself, so the grant does not rest on the
        // provider's transcript (`SPEC.md` 8.3).
        const address = typeof body?.address === "string" ? body.address : "";
        const sessionId = typeof body?.sessionId === "string" ? body.sessionId : "";
        const window = options.policy.sessionWindows?.[sessionId];
        if (window === undefined) {
          return respond(res, 404, { committed: false, error: "unknown_session" });
        }
        if (!isClaimAddress(address)) {
          return respond(res, 400, { committed: false, error: "bad_address" });
        }
        if (!Array.isArray(body?.answers)) {
          return respond(res, 400, { committed: false, error: "bad_answers" });
        }
        const recorded = await commitments.record({
          utilityId: options.policy.utilityId,
          window,
          proof: {
            address,
            sessionId,
            answers: body.answers as { challengeId: string; answer: string }[],
            issuedAt: typeof body?.issuedAt === "number" ? body.issuedAt : Number.NaN,
            signature: typeof body?.signature === "string" ? body.signature : "",
          },
          now: now(),
        });
        if (!recorded.ok) {
          const status =
            recorded.error === "busy" ? 503 : recorded.error === "outside_window" || recorded.error === "stale_commitment" ? 403 : 400;
          return respond(res, status, { committed: false, error: recorded.error });
        }
        return respond(res, 202, {
          committed: true,
          digest: recorded.commitment.digest,
          closesAt: window.closesAt,
        });
      }
      // A result that names an address is only honoured with a one-time proof
      // that the presenter controls it (`AH-6`). The proof is verified here;
      // `decideAccess` receives the verified fact, never the raw fields.
      let claim: { address: string } | undefined;
      const claimFields = [body?.address, body?.challenge, body?.signature];
      if (claimFields.some((value) => value !== undefined)) {
        if (claimFields.some((value) => typeof value !== "string")) {
          return respond(res, 400, { error: "bad_claim" });
        }
        const checked = await challenges.check(
          options.policy.utilityId,
          walletOrigin(req, options.policy),
          {
            address: body.address as string,
            challenge: body.challenge as string,
            signature: body.signature as string,
          },
          now(),
        );
        if (!checked.ok) {
          return respond(res, 403, { granted: false, reason: checked.error });
        }
        claim = { address: checked.address };
      }
      // The answers this utility witnessed for the address the *signed* result
      // names; `decideAccess` checks that address against the result it verified
      // before it scores anything.
      let commitment: { address: string; answers: { challengeId: string; answer: string }[] } | undefined;
      const claimed = body?.result as { sessionId?: unknown; participantAddress?: unknown } | undefined;
      if (typeof claimed?.sessionId === "string" && typeof claimed?.participantAddress === "string") {
        const stored = commitments.get(
          options.policy.utilityId,
          claimed.sessionId,
          claimed.participantAddress,
          now(),
        );
        if (stored !== null) {
          commitment = { address: stored.address, answers: stored.answers };
        }
      }
      const decision = await decideAccess(
        {
          result: body?.result,
          ...(claim === undefined ? {} : { claim }),
          // Untrusted input: the decision checks it against the digest this
          // utility pinned before it believes anything in it.
          ...(body?.scoreReveal === undefined ? {} : { scoreReveal: body.scoreReveal }),
          // The answers this utility witnessed, looked up by the address the
          // signed result names; the decision re-checks that address against
          // the result it verified before scoring anything.
          ...(commitment === undefined ? {} : { commitment }),
        },
        options.policy,
        grants,
        now(),
        () => randomUUID(),
      );
      return respond(res, decision.granted ? 200 : 403, decision);
    } catch (error) {
      if (error instanceof HttpFailure) {
        return respond(res, error.status, { error: error.code });
      }
      return respond(res, 500, { error: "internal_error" });
    }
  };

  return createServer((req, res) => {
    // Nothing may escape this promise: an unhandled rejection would end the
    // process and take every other caller with it.
    void handle(req, res).catch(() => respond(res, 500, { error: "internal_error" }));
  });
}

function walletOrigin(req: IncomingMessage, policy: UtilityPolicy): string {
  if (policy.publicOrigin !== undefined) return policy.publicOrigin;
  const address = req.socket.localAddress?.replace(/^::ffff:/, "");
  const port = req.socket.localPort;
  if ((address === undefined || (!/^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(address) && address !== "::1")) || port === undefined) {
    throw new HttpFailure(503, "wallet_origin_unconfigured");
  }
  return new URL(`http://${address === "::1" ? "[::1]" : address}:${port}`).origin;
}

function requestUrl(req: IncomingMessage): URL {
  try {
    return new URL(req.url ?? "/", `http://${req.headers.host ?? "localhost"}`);
  } catch {
    throw new HttpFailure(400, "bad_request");
  }
}

/** Strips brackets and IPv4-mapped prefixes so two spellings compare equal. */
function normalizeIp(value: string): string {
  const trimmed = value.trim().replace(/^\[|\]$/g, "");
  return trimmed.toLowerCase().startsWith("::ffff:") ? trimmed.slice(7) : trimmed;
}

/** The rate-limit identity; a forwarded header counts only behind a trusted proxy. */
function clientAddress(req: IncomingMessage, trustedProxies: string[]): string {
  const remote = normalizeIp(req.socket.remoteAddress ?? "unknown");
  if (trustedProxies.length === 0 || !trustedProxies.includes(remote)) {
    return remote;
  }
  const forwarded = req.headers["x-forwarded-for"];
  if (typeof forwarded !== "string" || forwarded.trim() === "") {
    return remote;
  }
  const hops = forwarded
    .split(",")
    .map((hop) => normalizeIp(hop))
    .filter((hop) => hop !== "");
  for (let index = hops.length - 1; index >= 0; index -= 1) {
    if (!trustedProxies.includes(hops[index])) {
      return hops[index];
    }
  }
  return remote;
}

class HttpFailure extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string) {
    super(code);
    this.status = status;
    this.code = code;
  }
}

async function readJson(
  req: IncomingMessage,
  limit: number,
): Promise<{
  result?: unknown;
  address?: unknown;
  challenge?: unknown;
  signature?: unknown;
  scoreReveal?: unknown;
  sessionId?: unknown;
  answers?: unknown;
  issuedAt?: unknown;
}> {
  const contentType = String(req.headers["content-type"] ?? "").toLowerCase();
  if (!contentType.startsWith("application/json")) {
    throw new HttpFailure(415, "unsupported_media_type");
  }
  const chunks: Buffer[] = [];
  let total = 0;
  for await (const chunk of req) {
    const buffer = chunk as Buffer;
    total += buffer.length;
    if (total > limit) {
      throw new HttpFailure(413, "payload_too_large");
    }
    chunks.push(buffer);
  }
  if (total === 0) {
    throw new HttpFailure(400, "invalid_json");
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8")) as {
      result?: unknown;
      address?: unknown;
      challenge?: unknown;
      signature?: unknown;
      scoreReveal?: unknown;
      sessionId?: unknown;
      answers?: unknown;
      issuedAt?: unknown;
    };
  } catch {
    throw new HttpFailure(400, "invalid_json");
  }
}

/** The bound on remembered rate-limit windows. */
const MAX_WINDOWS = 10_000;

export function allowRateLimit(
  windows: Map<string, { start: number; count: number }>,
  config: { windowSeconds: number; maxRequests: number },
  key: string,
  now: number,
): boolean {
  const windowMs = config.windowSeconds * 1000;
  const window = windows.get(key);
  if (window === undefined || now - window.start >= windowMs) {
    if (windows.size >= MAX_WINDOWS) {
      for (const [existing, entry] of windows) {
        if (now - entry.start >= windowMs) {
          windows.delete(existing);
        }
      }
      // Never forget a live client's consumed allowance to admit a new key.
      if (window === undefined && windows.size >= MAX_WINDOWS) return false;
    }
    windows.set(key, { start: now, count: 1 });
    return true;
  }
  window.count += 1;
  return window.count <= config.maxRequests;
}

function respond(res: ServerResponse, status: number, body: unknown): void {
  if (res.writableEnded === true) {
    return;
  }
  try {
    const payload = JSON.stringify(body);
    if (res.headersSent === false) {
      res.statusCode = status;
      res.setHeader("Content-Type", "application/json; charset=utf-8");
      res.setHeader("Content-Length", Buffer.byteLength(payload));
    }
    res.end(payload);
  } catch {
    // A socket that is already gone is not a reason to end the process.
    res.destroy();
  }
}
