// The gateway's whole surface, written out. There is no generic proxy and no
// RPC passthrough: a provider's node stays behind this table, and a path that is
// not listed is a 404 (SPEC.md 4.4).

import type { IncomingMessage, ServerResponse } from "node:http";
import type { GatewayConfig } from "./config.ts";
import type { ApplicationKey } from "./keys.ts";
import { canonicalJson, signResult } from "./keys.ts";
import { describeContent, type FlipPool } from "./pool.ts";
import type { Participant, SessionState } from "./session.ts";
import { checkWalletBinding, isWalletAddress } from "./wallet.ts";

export interface GatewayDeps {
  config: GatewayConfig;
  pool: FlipPool;
  session: SessionState;
  applicationKey: ApplicationKey;
  marketplaceState: () => unknown;
  now: () => number;
}

export interface GatewayError {
  error: string;
  message?: string;
}

export class HttpError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message?: string) {
    super(message ?? code);
    this.status = status;
    this.code = code;
  }
}

/**
 * A handler runs after the request passed size, rate, and (where required)
 * authentication checks. Throwing HttpError produces a JSON error with that
 * status; anything else becomes a 500 with no internals leaked.
 */
export type Handler = (request: RouteRequest) => Promise<{ status?: number; body: unknown }>;

export interface RouteRequest {
  req: IncomingMessage;
  res: ServerResponse;
  url: URL;
  body: unknown;
  participant?: Participant;
  deps: GatewayDeps;
}

export interface Route {
  method: "GET" | "POST";
  path: string;
  authenticated: boolean;
  /** Whether a JSON body is parsed and how large it may be. */
  body: "none" | "json";
  handler: Handler;
}

function walletOrigin(req: IncomingMessage, config: GatewayConfig): string {
  if (config.publicOrigin !== undefined) return config.publicOrigin;
  const address = req.socket.localAddress?.replace(/^::ffff:/, "");
  const port = req.socket.localPort;
  if ((address === undefined || (!/^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(address) && address !== "::1")) || port === undefined) {
    throw new HttpError(503, "wallet_origin_unconfigured", "configure publicOrigin for wallet enrollment");
  }
  return new URL(`http://${address === "::1" ? "[::1]" : address}:${port}`).origin;
}

export function buildRoutes(): Route[] {
  return [
    {
      method: "GET",
      path: "/session/policy",
      authenticated: false,
      body: "none",
      handler: async ({ deps }) => {
        const closed = deps.session.resultsAvailable(deps.now());
        return {
          body: {
            // `SPEC.md` 4.5 asks for versioned public types, and this is the one a
            // customer's client reads before anything else.
            format: "fod.session-policy",
            version: 1,
            providerId: deps.config.providerId,
            sessionId: deps.pool.sessionId,
            utilityId: deps.pool.utilityId,
            // What this session is, so a utility can pin it and a participant can
            // compare it with the result they are handed (SPEC.md 8.2, 8.3).
            contentDigest: deps.pool.contentDigest,
            provenance: deps.pool.provenance,
            rules: deps.pool.rules,
            // The digest preimage (including the per-session answer salt) is
            // revealed only after the session closes, so the published digest
            // cannot be enumerated while answers still count (`AH-2`). Anyone
            // holding the pin can then recompute it with the gateway's own
            // `sessionContentDigest`.
            ...(closed ? { contentReveal: describeContent(deps.pool) } : {}),
          },
        };
      },
    },
    {
      method: "GET",
      path: "/session/lobby",
      authenticated: false,
      body: "none",
      handler: async ({ deps }) => ({
        body: {
          window: deps.session.windowState(deps.now()),
          opensAt: deps.pool.rules.opensAt,
          closesAt: deps.pool.rules.closesAt,
          participants: deps.session.participantCount(deps.now()),
          challenges: deps.session.challenges().length,
        },
      }),
    },
    {
      method: "GET",
      path: "/shared-node/status",
      authenticated: false,
      body: "none",
      handler: async ({ deps }) => ({
        body: {
          format: "fod.shared-node-status",
          version: 1,
          nodeId: deps.config.providerId,
          sessionId: deps.pool.sessionId,
          utilityId: deps.pool.utilityId,
          provenance: {
            author: deps.pool.provenance.author,
            epoch: deps.pool.provenance.epoch,
            blockRange: deps.pool.provenance.blockRange,
          },
          operations: ["session", "pool"],
          custody: ["self", "hosted"],
          note:
            "This reference node serves the session and the pool. Listing and delivery need the author's own nodekey or vault: self-hosted keeps the keys with the author, hosted means the node holds the vault and can deliver.",
        },
      }),
    },
    {
      method: "GET",
      path: "/provider/key",
      authenticated: false,
      body: "none",
      handler: async ({ deps }) => ({
        body: {
          providerId: deps.config.providerId,
          algorithm: "ed25519",
          publicKeyHex: deps.applicationKey.publicKeyHex,
        },
      }),
    },
    {
      method: "POST",
      path: "/session/enroll-challenge",
      authenticated: false,
      body: "json",
      handler: async ({ deps, body, req }) => {
        const address = readString(body, "address");
        if (address === undefined || !isWalletAddress(address)) {
          throw new HttpError(400, "bad_address", "a 20-byte EVM/Idena address is required");
        }
        if (deps.session.windowState(deps.now()) === "closed") {
          throw new HttpError(410, "session_closed", "the session is closed");
        }
        const issued = deps.session.issueEnrollmentNonce(deps.config.providerId, address.toLowerCase(), walletOrigin(req, deps.config), deps.now());
        if (issued === null) {
          throw new HttpError(503, "session_busy", "no enrollment challenge is available right now");
        }
        return {
          status: 201,
          body: {
            challenge: issued.challenge,
            expiresAt: new Date(issued.expiresAt).toISOString(),
          },
        };
      },
    },
    {
      method: "POST",
      path: "/session/enroll",
      authenticated: false,
      body: "json",
      handler: async ({ deps, body, req }) => {
        const claimed = readString(body, "providerId");
        if (claimed !== undefined && claimed !== deps.config.providerId) {
          throw new HttpError(400, "wrong_provider", "this gateway serves another provider");
        }
        const address = readString(body, "address");
        const challenge = readString(body, "challenge");
        const signature = readString(body, "signature");
        if (address === undefined || challenge === undefined || signature === undefined) {
          throw new HttpError(
            400,
            "bad_request",
            "address, challenge and signature are required: a gate session enrolls a bound wallet",
          );
        }
        const checked = await checkWalletBinding(
          { providerId: deps.config.providerId, sessionId: deps.pool.sessionId, origin: walletOrigin(req, deps.config) },
          { address, challenge, signature },
          deps.now(),
        );
        if (!checked.ok) {
          throw new HttpError(403, checked.error, "the wallet binding did not verify");
        }
        if (
          !deps.session.consumeEnrollmentNonce(
            deps.config.providerId,
            checked.address,
            checked.nonce,
            deps.now(),
          )
        ) {
          throw new HttpError(403, "unknown_nonce", "the enrollment challenge was not issued or was already used");
        }
        const enrolled = deps.session.enroll(deps.now(), checked.address);
        if (!enrolled.ok) {
          if (enrolled.error === "session_not_open") {
            throw new HttpError(403, enrolled.error, `the session opens at ${deps.pool.rules.opensAt}`);
          }
          if (enrolled.error === "address_enrolled") {
            throw new HttpError(409, enrolled.error, "this address already has a live enrollment in this session");
          }
          if (enrolled.error === "session_full") {
            throw new HttpError(503, enrolled.error, "this session is at capacity and refuses new enrollments");
          }
          throw new HttpError(410, enrolled.error, "the session is closed");
        }
        return {
          status: 201,
          body: {
            participantId: enrolled.participantId,
            token: enrolled.token,
            expiresAt: new Date(enrolled.expiresAt).toISOString(),
            address: enrolled.address,
          },
        };
      },
    },
    {
      method: "GET",
      path: "/session/challenges",
      authenticated: true,
      body: "none",
      handler: async ({ deps }) => ({ body: { challenges: deps.session.challenges() } }),
    },
    {
      method: "POST",
      path: "/session/answers",
      authenticated: true,
      body: "json",
      handler: async ({ deps, body, participant }) => {
        const challengeId = readString(body, "challengeId");
        const answer = readString(body, "answer");
        if (challengeId === undefined || answer === undefined) {
          throw new HttpError(400, "bad_request", "challengeId and answer are required");
        }
        const submitted = deps.session.submit(participant!.participantId, challengeId, answer, deps.now());
        if (!submitted.ok) {
          if (submitted.error === "session_not_open") {
            throw new HttpError(403, submitted.error, `the session opens at ${deps.pool.rules.opensAt}`);
          }
          if (submitted.error === "session_closed") {
            throw new HttpError(410, submitted.error, "the session is closed");
          }
          const status = submitted.error === "already_answered" ? 409 : 400;
          throw new HttpError(status, submitted.error);
        }
        // Accepted means accepted, nothing about the answer (`AH-1`): the
        // correctness of a submission is only reflected in the signed result
        // after the session closes.
        return { status: 202, body: { accepted: true } };
      },
    },
    {
      method: "GET",
      path: "/session/result",
      authenticated: true,
      body: "none",
      handler: async ({ deps, participant }) => {
        // A result is signed only after the session is irrevocably closed: a
        // participant who answered inside the window collects the signed
        // statement afterwards, as long as their own token has not expired.
        // Before close, the score would be an answer oracle (`AH-1`).
        if (!deps.session.resultsAvailable(deps.now())) {
          throw new HttpError(
            403,
            "session_open",
            `results are issued after the session closes at ${deps.pool.rules.closesAt}`,
          );
        }
        const summary = deps.session.summary(participant!.participantId);
        const issuedAt = deps.now();
        const unsigned = {
          format: "fod.session-result",
          version: 1,
          providerId: deps.config.providerId,
          sessionId: deps.pool.sessionId,
          utilityId: deps.pool.utilityId,
          contentDigest: deps.pool.contentDigest,
          participantId: participant!.participantId,
          ...(participant!.address === undefined ? {} : { participantAddress: participant!.address }),
          issuedAt: new Date(issuedAt).toISOString(),
          expiresAt: new Date(participant!.expiresAt).toISOString(),
          score: {
            assigned: summary.assigned,
            answered: summary.answered,
            correct: summary.correct,
            passThreshold: summary.passThreshold,
          },
          // What the provider scored, challenge by challenge. A signature alone
          // cannot show that the score follows from the participant's answers;
          // this transcript plus the closed policy's `contentReveal` lets the
          // participant's client recompute the score and refuse a mismatch
          // (`SPEC.md` 10.4, "provenance versus scoring").
          transcript: {
            rule: "sha256 of the trimmed, whitespace-collapsed, lower-cased answer text",
            entries: summary.transcript,
          },
          outcome: summary.passed ? "passed" : "not-passed",
          issuedBy: {
            kind: "provider",
            providerId: deps.config.providerId,
            note: "a valid signature proves the provider issued this statement, not that scoring was honest",
          },
          algorithm: "ed25519",
        };
        const signature = signResult(deps.applicationKey.privateKey, canonicalJson(unsigned));
        return { body: { ...unsigned, signature } };
      },
    },
    {
      method: "GET",
      path: "/marketplace/state",
      authenticated: true,
      body: "none",
      handler: async ({ deps }) => ({ body: deps.marketplaceState() }),
    },
    {
      method: "POST",
      path: "/transactions",
      authenticated: true,
      body: "json",
      handler: async () => {
        // Forwarding a caller-supplied transaction would make this gateway an
        // unrestricted relay; the CLI submits through the provider's node.
        throw new HttpError(
          501,
          "not_implemented",
          "submitting signed transactions through the gateway is not implemented",
        );
      },
    },
  ];
}

function readString(body: unknown, field: string): string | undefined {
  if (typeof body !== "object" || body === null) {
    return undefined;
  }
  const value = (body as Record<string, unknown>)[field];
  return typeof value === "string" ? value : undefined;
}
