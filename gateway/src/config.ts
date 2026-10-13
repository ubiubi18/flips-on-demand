// Gateway configuration. Everything a provider can tune lives in one JSON file;
// nothing here is read from the environment, so a running gateway's behaviour is
// explainable from the file it was started with.

import { readFile } from "node:fs/promises";
import { dirname, isAbsolute, resolve } from "node:path";

export interface RateLimitConfig {
  windowSeconds: number;
  maxRequests: number;
}

export interface GatewayConfig {
  port: number;
  /**
   * The interface the gateway binds. Defaults to loopback: serving other
   * machines is an explicit configuration choice, and the README's bearer
   * tokens over plain HTTP are only safe locally (`AH-9`).
   */
  host: string;
  providerId: string;
  /** Public origin for wallet challenges when this gateway sits behind a proxy. */
  publicOrigin?: string;
  /**
   * Browser origins allowed to read responses. A `file://` page sends `null`,
   * so a provider serving distributed HTML opts in with `"null"`. This list
   * only tells browsers who may read; it never authorises anything.
   */
  origins: string[];
  sessionTtlSeconds: number;
  bodyLimitBytes: number;
  rateLimit: RateLimitConfig;
  poolPath: string;
  applicationKeyPath: string;
  /**
   * Peer addresses whose `X-Forwarded-For` may be believed, most commonly the
   * loopback address of the TLS proxy the README recommends. When the socket's
   * peer is in this list, the client identity for rate limiting is taken from
   * the forwarded chain (walking right to left past trusted hops). With an
   * empty list the header is ignored entirely, so a direct caller cannot mint
   * buckets by inventing it (`AH-4`).
   */
  trustedProxies: string[];
  /** Directory the relative paths above are resolved against. */
  baseDir: string;
}

export async function loadConfig(path: string): Promise<GatewayConfig> {
  const raw = await readFile(path, "utf8");
  const parsed: unknown = JSON.parse(raw);
  if (typeof parsed !== "object" || parsed === null) {
    throw new Error("config must be an object");
  }
  const record = parsed as Record<string, unknown>;
  const baseDir = dirname(resolve(path));
  const originList = record.origins;
  if (!Array.isArray(originList) || originList.some((entry) => typeof entry !== "string")) {
    throw new Error("config.origins must be a list of strings");
  }
  const rateLimit = record.rateLimit;
  if (typeof rateLimit !== "object" || rateLimit === null) {
    throw new Error("config.rateLimit must be an object");
  }
  const limits = rateLimit as Record<string, unknown>;
  const providerId = stringField(record.providerId, "providerId");
  // The provider id travels inside the colon-delimited enrollment challenge,
  // so a value that could split it is refused here.
  if (!/^[A-Za-z0-9._-]{1,64}$/.test(providerId)) {
    throw new Error("config.providerId must use letters, digits, dot, underscore or dash");
  }
  const trustedProxies = readTrustedProxies(record.trustedProxies);
  const publicOrigin = record.publicOrigin;
  if (publicOrigin !== undefined && !isPublicOrigin(publicOrigin)) {
    throw new Error("config.publicOrigin must be an http(s) origin, using https outside loopback");
  }
  const resolvePath = (value: unknown, field: string): string => {
    if (typeof value !== "string" || value === "") {
      throw new Error(`config.${field} must be a path`);
    }
    return isAbsolute(value) ? value : resolve(baseDir, value);
  };
  return {
    port: numberField(record.port, "port"),
    host:
      record.host === undefined
        ? "127.0.0.1"
        : stringField(record.host, "host"),
    providerId,
    ...(publicOrigin === undefined ? {} : { publicOrigin: publicOrigin as string }),
    origins: originList,
    sessionTtlSeconds: numberField(record.sessionTtlSeconds, "sessionTtlSeconds"),
    bodyLimitBytes: numberField(record.bodyLimitBytes, "bodyLimitBytes"),
    rateLimit: {
      windowSeconds: numberField(limits.windowSeconds, "rateLimit.windowSeconds"),
      maxRequests: numberField(limits.maxRequests, "rateLimit.maxRequests"),
    },
    poolPath: resolvePath(record.poolPath, "poolPath"),
    applicationKeyPath: resolvePath(record.applicationKeyPath, "applicationKeyPath"),
    trustedProxies,
    baseDir,
  };
}

function isPublicOrigin(value: unknown): value is string {
  if (typeof value !== "string") return false;
  try {
    const url = new URL(value);
    const loopback = url.hostname === "localhost" || url.hostname === "[::1]" || /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(url.hostname);
    return (url.protocol === "https:" || (url.protocol === "http:" && loopback)) &&
      url.origin === value && url.username === "" && url.password === "";
  } catch {
    return false;
  }
}

/** Strips brackets and IPv4-mapped prefixes so two spellings compare equal. */
export function normalizeIp(value: string): string {
  const trimmed = value.trim().replace(/^\[|\]$/g, "");
  return trimmed.toLowerCase().startsWith("::ffff:") ? trimmed.slice(7) : trimmed;
}

function readTrustedProxies(value: unknown): string[] {
  if (value === undefined) {
    return [];
  }
  if (!Array.isArray(value) || value.some((entry) => typeof entry !== "string" || entry.trim() === "")) {
    throw new Error("config.trustedProxies must be a list of addresses when present");
  }
  return value.map((entry) => normalizeIp(entry as string));
}

function stringField(value: unknown, field: string): string {
  if (typeof value !== "string" || value === "") {
    throw new Error(`config.${field} must be a non-empty string`);
  }
  return value;
}

function numberField(value: unknown, field: string): number {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) {
    throw new Error(`config.${field} must be a positive number`);
  }
  return value;
}
