// The HTTP surface: routing, size limits, rate limits, bearer auth, and CORS.
//
// Every request goes through this one path, so the properties SPEC.md 4.4 asks
// for are here rather than spread across handlers:
//
// - only listed routes exist, so there is no RPC passthrough,
// - bodies are capped and must be JSON,
// - requests are rate limited per authenticated participant, or per client
//   address when the caller is anonymous or its token does not authenticate,
// - authorisation is a bearer token; a file:// origin is never authorisation,
// - no cookies are ever set, so there is no ambient session to steal,
// - CORS headers are only sent for configured origins, and never as a wildcard.

import { createServer } from "node:http";
import type { IncomingMessage, Server, ServerResponse } from "node:http";
import { buildRoutes, HttpError } from "./routes.ts";
import type { GatewayDeps, Route } from "./routes.ts";
import type { Participant } from "./session.ts";
import { normalizeIp } from "./config.ts";

const BEARER = "Bearer ";

/** The bound on remembered rate-limit windows. */
const MAX_LIMIT_KEYS = 10_000;

export class RateLimiter {
  private readonly windows = new Map<string, { start: number; count: number }>();
  private readonly config: { windowSeconds: number; maxRequests: number };

  constructor(config: { windowSeconds: number; maxRequests: number }) {
    this.config = config;
  }

  allow(key: string, now: number): boolean {
    const windowMs = this.config.windowSeconds * 1000;
    const window = this.windows.get(key);
    if (window === undefined || now - window.start >= windowMs) {
      if (this.windows.size >= MAX_LIMIT_KEYS) {
        this.evict(now);
      }
      if (window === undefined && this.windows.size >= MAX_LIMIT_KEYS) return false;
      this.windows.set(key, { start: now, count: 1 });
      return true;
    }
    window.count += 1;
    return window.count <= this.config.maxRequests;
  }

  private evict(now: number): void {
    const windowMs = this.config.windowSeconds * 1000;
    for (const [key, window] of this.windows) {
      if (now - window.start >= windowMs) {
        this.windows.delete(key);
      }
    }
    // Live windows preserve their consumed allowance. Capacity refuses new
    // identities until expiry rather than making a rotating client unlimited.
  }
}

export function createGatewayServer(deps: GatewayDeps): Server {
  const routes = buildRoutes();
  const limiter = new RateLimiter(deps.config.rateLimit);
  return createServer((req, res) => {
    // Nothing may escape this promise: an unhandled rejection would end the
    // process and take every other caller with it.
    void handle(req, res, routes, deps, limiter).catch(() => {
      respond(res, 500, { error: "internal_error" });
    });
  });
}

async function handle(
  req: IncomingMessage,
  res: ServerResponse,
  routes: Route[],
  deps: GatewayDeps,
  limiter: RateLimiter,
): Promise<void> {
  try {
    applyCors(req, res, deps.config.origins);
    res.setHeader("X-Content-Type-Options", "nosniff");
    res.setHeader("Cache-Control", "no-store");

    // A browser sending a JSON body cross-origin asks first, and a file:// page
    // sends `Origin: null`. Answering the preflight is what makes the client
    // usable at all; it grants nothing, because every protected request still
    // needs its bearer token. It is charged against the address like any other
    // anonymous request, so a preflight flood is not free.
    if (req.method === "OPTIONS") {
      if (!limiter.allow(`addr:${clientAddress(req, deps.config.trustedProxies)}`, deps.now())) {
        throw new HttpError(429, "rate_limited", "too many requests");
      }
      res.statusCode = 204;
      res.end();
      return;
    }

    // A Host header is caller-supplied, so it is parsed inside the guard: an
    // unparseable one is this request's 400, never the process's exit.
    const url = requestUrl(req);
    const route = routes.find((entry) => entry.method === req.method && entry.path === url.pathname);
    if (route === undefined) {
      const otherMethod = routes.find((entry) => entry.path === url.pathname);
      if (otherMethod !== undefined) {
        res.setHeader("Allow", otherMethod.method);
        throw new HttpError(405, "method_not_allowed");
      }
      throw new HttpError(404, "not_found", "no such endpoint");
    }

    const token = bearerToken(req);

    let participant: Participant | undefined;
    let refusal: HttpError | null = null;
    if (route.authenticated) {
      if (token === null) {
        refusal = new HttpError(401, "missing_token", "a bearer token is required");
      } else {
        const authenticated = deps.session.authenticate(token, deps.now());
        if (authenticated === null) {
          refusal = new HttpError(401, "invalid_token");
        } else if (authenticated === "expired") {
          refusal = new HttpError(410, "session_expired");
        } else {
          participant = authenticated;
        }
      }
    }

    // One bucket per caller: an authenticated participant, or the client
    // address. The presented token never names a bucket, so rotating invalid
    // tokens cannot mint fresh budget, and a shared address does not spend an
    // authenticated participant's allowance.
    const limitKey =
      participant === undefined
        ? `addr:${clientAddress(req, deps.config.trustedProxies)}`
        : `participant:${participant.participantId}`;
    if (!limiter.allow(limitKey, deps.now())) {
      throw new HttpError(429, "rate_limited", "too many requests");
    }
    if (refusal !== null) {
      throw refusal;
    }

    const body = route.body === "json" ? await readJsonBody(req, deps.config.bodyLimitBytes) : undefined;
    const result = await route.handler({ req, res, url, body, participant, deps });
    respond(res, result.status ?? 200, result.body);
  } catch (error) {
    if (error instanceof HttpError) {
      const payload: Record<string, unknown> = { error: error.code };
      if (error.message !== error.code) {
        payload.message = error.message;
      }
      respond(res, error.status, payload);
      return;
    }
    // Nothing about internals leaves the process.
    respond(res, 500, { error: "internal_error" });
  }
}

function requestUrl(req: IncomingMessage): URL {
  try {
    return new URL(req.url ?? "/", `http://${req.headers.host ?? "localhost"}`);
  } catch {
    throw new HttpError(400, "bad_request", "the request target and Host header do not form a URL");
  }
}

/**
 * The identity a rate limit is charged to. The forwarded header is believed
 * only when the socket peer is a configured trusted proxy; otherwise a direct
 * caller could rotate `X-Forwarded-For` values to mint fresh buckets.
 */
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
  // Walk right to left: the nearest hop the trusted proxy itself did not add.
  for (let index = hops.length - 1; index >= 0; index -= 1) {
    if (!trustedProxies.includes(hops[index])) {
      return hops[index];
    }
  }
  return remote;
}

function bearerToken(req: IncomingMessage): string | null {
  const header = req.headers.authorization;
  if (typeof header !== "string" || !header.startsWith(BEARER)) {
    return null;
  }
  const token = header.slice(BEARER.length).trim();
  return token === "" ? null : token;
}

function applyCors(req: IncomingMessage, res: ServerResponse, origins: string[]): void {
  const origin = req.headers.origin;
  if (typeof origin !== "string") {
    return;
  }
  res.setHeader("Vary", "Origin");
  if (origins.includes(origin)) {
    // Echo the specific origin; never "*", and never tied to authorisation.
    res.setHeader("Access-Control-Allow-Origin", origin);
    res.setHeader("Access-Control-Allow-Headers", "authorization, content-type");
    res.setHeader("Access-Control-Allow-Methods", "GET, POST");
    res.setHeader("Access-Control-Max-Age", "60");
  }
}

async function readJsonBody(req: IncomingMessage, limit: number): Promise<unknown> {
  const declared = req.headers["content-length"];
  if (typeof declared === "string") {
    const length = Number.parseInt(declared, 10);
    if (Number.isFinite(length) && length > limit) {
      throw new HttpError(413, "payload_too_large");
    }
  }
  const contentType = req.headers["content-type"] ?? "";
  if (!String(contentType).toLowerCase().startsWith("application/json")) {
    throw new HttpError(415, "unsupported_media_type", "send application/json");
  }
  const chunks: Buffer[] = [];
  let total = 0;
  for await (const chunk of req) {
    const buffer = chunk as Buffer;
    total += buffer.length;
    if (total > limit) {
      throw new HttpError(413, "payload_too_large");
    }
    chunks.push(buffer);
  }
  if (total === 0) {
    return undefined;
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } catch {
    throw new HttpError(400, "invalid_json");
  }
}

function respond(res: ServerResponse, status: number, body: unknown): void {
  if (res.writableEnded === true) {
    return;
  }
  try {
    const payload = JSON.stringify(body ?? {});
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
