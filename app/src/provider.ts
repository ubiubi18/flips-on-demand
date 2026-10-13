// The provider connection a customer is handed, and the client that talks to
// the gateway it names (SPEC.md 4.5 `ProviderConfig`, 8.1 step 5).
//
// Two things matter here and nowhere else in the interface:
//
// - The participant token lives in memory for the length of the tab. It is
//   never written to storage, because a file:// page has no origin to scope it.
// - The signed result is checked in the browser with WebCrypto before it is
//   shown, so "verified" means the signature verified, not that the page says
//   so.

export interface ProviderConfig {
  format: "fod.provider-config";
  version: number;
  providerId: string;
  network: string;
  gatewayUrl: string;
  protocol: string;
  /** The utility the provider's result is claimed against, when it is published. */
  utilityUrl?: string;
  /**
   * The application key this provider signed with when the file was written, if
   * the file records one. A result is verified against it, so a participant who
   * keeps the file does not have to take whatever key the host serves later.
   */
  publicKeyHex?: string;
  note?: string;
}

export type ProviderParse =
  | { ok: true; config: ProviderConfig }
  | { ok: false; error: string };

function isLoopbackURL(url: URL): boolean {
  return url.hostname === "localhost" || url.hostname === "[::1]" || /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(url.hostname);
}

export function parseProviderConfig(text: string): ProviderParse {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return { ok: false, error: "the connection file is not valid JSON" };
  }
  if (typeof raw !== "object" || raw === null) {
    return { ok: false, error: "the connection file must contain an object" };
  }
  const record = raw as Record<string, unknown>;
  if (record.format !== "fod.provider-config") {
    return { ok: false, error: "this is not a provider connection file" };
  }
  if (record.version !== 1) {
    return { ok: false, error: "unsupported connection file version" };
  }
  for (const field of ["providerId", "network", "gatewayUrl", "protocol"] as const) {
    if (typeof record[field] !== "string" || record[field] === "") {
      return { ok: false, error: `the connection file is missing ${field}` };
    }
  }
  let gateway: URL;
  try {
    gateway = new URL(String(record.gatewayUrl));
  } catch {
    return { ok: false, error: "the connection file names an unreadable gateway" };
  }
  if (gateway.protocol !== "http:" && gateway.protocol !== "https:") {
    return { ok: false, error: "the gateway must be http or https" };
  }
  if (gateway.protocol === "http:" && !isLoopbackURL(gateway)) {
    return { ok: false, error: "the gateway must use https outside loopback" };
  }
  // Resolve the localhost alias before both fetching and signing. Accepting a
  // 127.0.0.1 challenge while still fetching localhost could let a second
  // loopback listener on the same port relay the wallet signature.
  if (gateway.protocol === "http:" && gateway.hostname === "localhost") gateway.hostname = "127.0.0.1";
  let utility: URL | undefined;
  if (record.utilityUrl !== undefined) {
    if (typeof record.utilityUrl !== "string" || record.utilityUrl === "") {
      return { ok: false, error: "the connection file's utilityUrl is not a URL" };
    }
    try {
      utility = new URL(String(record.utilityUrl));
    } catch {
      return { ok: false, error: "the connection file names an unreadable utility" };
    }
    if (utility.protocol !== "http:" && utility.protocol !== "https:") {
      return { ok: false, error: "the utility must be http or https" };
    }
    if (utility.protocol === "http:" && !isLoopbackURL(utility)) {
      return { ok: false, error: "the utility must use https outside loopback" };
    }
    if (utility.protocol === "http:" && utility.hostname === "localhost") utility.hostname = "127.0.0.1";
  }
  const pinned = record.publicKeyHex;
  // One agreed encoding (`AH-8`): the 44-byte SPKI DER key the gateway
  // publishes at `GET /provider/key`, as 88 hex characters. The browser imports
  // exactly that form, and the served key is compared against it byte for byte.
  if (pinned !== undefined && (typeof pinned !== "string" || !/^[0-9a-f]{88}$/i.test(pinned))) {
    return {
      ok: false,
      error: "the connection file's publicKeyHex is not the gateway's SPKI key (88 hex characters from GET /provider/key)",
    };
  }
  return {
    ok: true,
    config: {
      format: "fod.provider-config",
      version: 1,
      providerId: String(record.providerId),
      network: String(record.network),
      gatewayUrl: gateway.origin,
      protocol: String(record.protocol),
      utilityUrl: utility?.origin,
      publicKeyHex: pinned === undefined ? undefined : String(pinned).toLowerCase(),
      note: typeof record.note === "string" ? record.note : undefined,
    },
  };
}

export interface WalletBinding {
  address: string;
  challenge: string;
  signature: string;
}

export interface Enrolment {
  participantId: string;
  token: string;
  expiresAt: string;
  /** The EVM/Idena address the gateway bound to this participant. */
  address?: string;
}

export interface SessionPolicy {
  format: string;
  version: number;
  providerId: string;
  sessionId: string;
  utilityId: string;
  provenance: {
    epoch: number;
    author: string;
    source: string;
    blockRange: string;
    /** What the provider says its answers rest on, when the pool carries it. */
    answers?: string;
  };
  rules: { passThreshold: number; opensAt: string; closesAt: string };
  /**
   * SHA-256 over the session content (rules, provenance, every challenge with
   * its answer digest and content hash), salted by the pool's per-session
   * secret. A participant keeps the digest it read *before* answering; the
   * closed policy then reveals the preimage, so the score can be recomputed
   * against the flip set that was pinned rather than one served afterwards.
   */
  contentDigest: string;
  /**
   * The digest preimage, published by the gateway only after the session closes
   * (`AH-2`). It carries the per-challenge answer digests the provider scored
   * against and the salt the digest was built with, so a participant can check
   * it against the digest it read before answering and recompute its own score.
   */
  contentReveal?: SessionContentReveal;
}

/**
 * Exactly what the session digest commits to, as `gateway/src/pool.ts`'s
 * `describeContent` writes it. Canonical JSON over this object must hash to the
 * policy's `contentDigest`.
 */
export interface SessionContentReveal {
  sessionId: string;
  utilityId: string;
  rules: { passThreshold: number; opensAt: string; closesAt: string };
  provenance: unknown;
  answerSalt: string;
  challenges: { challengeId: string; cid: string; answerDigest: string; contentSha256: string | null }[];
}

/** The shared window a provider publishes while a session is on demand. */
export interface Lobby {
  window: "not-open" | "open" | "closed";
  opensAt: string;
  closesAt: string;
  participants: number;
  challenges: number;
}

export interface ChallengeSummary {
  challengeId: string;
  cid: string;
  /**
   * The two sequences a participant chooses between, each four pictures in the
   * order it shows them, when the provider's pool carried the flip's content
   * (`SOURCES.md` F-98). Without it the challenge can only be answered from
   * what the participant already has.
   */
  variants?: string[][];
}

export interface SessionResult {
  format: string;
  version: number;
  providerId: string;
  sessionId: string;
  utilityId: string;
  participantId: string;
  /** The EVM/Idena address bound at enrollment, when one was provided. */
  participantAddress?: string;
  issuedAt: string;
  expiresAt: string;
  score: { assigned: number; answered: number; correct: number; passThreshold: number };
  /**
   * The answers the provider says it scored: one entry per assigned challenge,
   * `null` when it recorded none. A result without it cannot be recomputed and
   * is therefore not claimable.
   */
  transcript?: { rule?: string; entries: { challengeId: string; answerDigest: string | null }[] };
  outcome: "passed" | "not-passed";
  issuedBy: { kind: string; providerId: string; note: string };
  algorithm: string;
  signature: string;
}

export class GatewayError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

const MAX_RESPONSE_BYTES = 16 * 1024 * 1024;

/** Read a replaceable endpoint without redirects, cookies or unbounded bodies. */
export async function requestJSON<T>(url: string, options: RequestInit = {}): Promise<T> {
  let response: Response;
  try {
    response = await fetch(url, {
      ...options,
      credentials: "omit",
      redirect: "error",
      referrerPolicy: "no-referrer",
      signal: AbortSignal.timeout(30_000),
    });
  } catch {
    throw new GatewayError(0, "the endpoint could not be reached");
  }
  if (Number(response.headers.get("content-length")) > MAX_RESPONSE_BYTES) {
    await response.body?.cancel();
    throw new GatewayError(0, "the endpoint response is too large");
  }
  const reader = response.body?.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  if (reader !== undefined) {
    try {
      while (true) {
        const next = await reader.read();
        if (next.done) break;
        size += next.value.byteLength;
        if (size > MAX_RESPONSE_BYTES) {
          await reader.cancel();
          throw new GatewayError(0, "the endpoint response is too large");
        }
        chunks.push(next.value);
      }
    } finally { reader.releaseLock(); }
  }
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
  let parsed: unknown;
  try { parsed = JSON.parse(new TextDecoder().decode(bytes)); }
  catch { throw new GatewayError(response.status, "the endpoint returned invalid JSON"); }
  if (!response.ok) {
    const message = isRecord(parsed) && typeof parsed.error === "string"
      ? parsed.error : isRecord(parsed) && typeof parsed.reason === "string"
        ? parsed.reason : `the endpoint answered ${response.status}`;
    throw new GatewayError(response.status, message);
  }
  return parsed as T;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

type WalletChallengeContext =
  | { kind: "enroll"; providerId: string; sessionId: string; address: string; endpointOrigin: string }
  | { kind: "claim"; utilityId: string; address: string; endpointOrigin: string };

/** Never forward arbitrary server text to personal_sign. */
export function checkedWalletChallenge(issued: unknown, context: WalletChallengeContext, now = Date.now()): string {
  if (!isRecord(issued) || typeof issued.challenge !== "string" || typeof issued.expiresAt !== "string" ||
      !/^0x[0-9a-f]{40}$/i.test(context.address)) {
    throw new GatewayError(0, "the endpoint returned an invalid wallet challenge");
  }
  const prefix = context.kind === "enroll" ? "fod-enroll:v3:" : "fod-claim:v2:";
  const fields = issued.challenge.startsWith(prefix) ? issued.challenge.slice(prefix.length).split(":") : [];
  const expected = context.kind === "enroll"
    ? [context.providerId, context.sessionId, context.address.toLowerCase(), encodeURIComponent(context.endpointOrigin)]
    : [context.utilityId, context.address.toLowerCase(), encodeURIComponent(context.endpointOrigin)];
  const nonce = fields[expected.length] ?? "";
  const stampText = fields[expected.length + 1] ?? "";
  const stamp = Number(stampText);
  const expiresAt = Date.parse(issued.expiresAt);
  if (fields.length !== expected.length + 2 || expected.some((value, at) => fields[at] !== value) ||
      !/^[0-9a-f]{32}$/.test(nonce) || !/^[0-9]+$/.test(stampText) || !Number.isSafeInteger(stamp) ||
      stamp > now + 60_000 || expiresAt !== stamp + 5 * 60_000 || now >= expiresAt) {
    throw new GatewayError(0, "the wallet challenge does not match this session, wallet or validity period");
  }
  return issued.challenge;
}

function isRasterDataUrl(value: unknown): value is string {
  if (typeof value !== "string" || value.length > 819_250) return false;
  const match = /^data:image\/(png|jpeg|gif|webp);base64,([A-Za-z0-9+/]+={0,2})$/.exec(value);
  if (match === null || match[2].length % 4 !== 0) return false;
  let bytes: string;
  try { bytes = atob(match[2]); } catch { return false; }
  switch (match[1]) {
    case "png": return bytes.startsWith("\x89PNG\r\n\x1a\n");
    case "jpeg": return bytes.startsWith("\xff\xd8\xff");
    case "gif": return bytes.startsWith("GIF87a") || bytes.startsWith("GIF89a");
    case "webp": return bytes.startsWith("RIFF") && bytes.slice(8, 12) === "WEBP";
  }
  return false;
}

export class GatewayClient {
  private readonly base: string;

  constructor(gatewayUrl: string) {
    this.base = gatewayUrl.replace(/\/$/, "");
  }

  private async request<T>(path: string, options: { method: "GET" | "POST"; token?: string; body?: unknown }): Promise<T> {
    const headers: Record<string, string> = {};
    if (options.token !== undefined) {
      headers.authorization = `Bearer ${options.token}`;
    }
    let payload: string | undefined;
    if (options.body !== undefined) {
      headers["content-type"] = "application/json";
      payload = JSON.stringify(options.body);
    }
    return requestJSON<T>(`${this.base}${path}`, { method: options.method, headers, body: payload });
  }

  policy(): Promise<SessionPolicy> {
    return this.request<SessionPolicy>("/session/policy", { method: "GET" }).then((policy) => {
      // `SPEC.md` 4.5 asks for versioned public types, and reading a policy this
      // client does not understand would mean acting on fields it only guesses
      // at: refuse it with the policy's own name rather than render it. It is a
      // `GatewayError` because that is what the session view is willing to show
      // a reader; anything else would reach them as "something went wrong".
      if (!isRecord(policy) || policy.format !== "fod.session-policy" || policy.version !== 1) {
        throw new GatewayError(
          0,
          "this is not a session policy this build reads",
        );
      }
      if (![policy.providerId, policy.sessionId, policy.utilityId].every((value) => typeof value === "string" && value !== "") ||
          !isRecord(policy.rules) || !Number.isSafeInteger(policy.rules.passThreshold) || policy.rules.passThreshold < 1 ||
          ![policy.rules.opensAt, policy.rules.closesAt].every((value) => typeof value === "string" && Number.isFinite(Date.parse(value))) ||
          Date.parse(policy.rules.opensAt) >= Date.parse(policy.rules.closesAt) ||
          !isRecord(policy.provenance) || !Number.isSafeInteger(policy.provenance.epoch) ||
          (policy.provenance.answers !== undefined && typeof policy.provenance.answers !== "string") ||
          typeof policy.contentDigest !== "string" || !/^[0-9a-f]{64}$/i.test(policy.contentDigest)) {
        throw new GatewayError(0, "the session policy is malformed");
      }
      return policy;
    });
  }

  lobby(): Promise<Lobby> {
    return this.request<Lobby>("/session/lobby", { method: "GET" }).then((value) => {
      if (!isRecord(value) || !["not-open", "open", "closed"].includes(value.window) ||
          ![value.opensAt, value.closesAt].every((stamp) => typeof stamp === "string" && Number.isFinite(Date.parse(stamp))) ||
          ![value.participants, value.challenges].every((count) => Number.isSafeInteger(count) && count >= 0)) {
        throw new GatewayError(0, "the session lobby is malformed");
      }
      return value;
    });
  }

  providerKey(): Promise<{ providerId: string; algorithm: string; publicKeyHex: string }> {
    return this.request("/provider/key", { method: "GET" });
  }

  /** Asks the gateway for the one-time text this wallet should sign (`AH-3`). */
  enrollChallenge(address: string): Promise<{ challenge: string; expiresAt: string }> {
    return this.request("/session/enroll-challenge", { method: "POST", body: { address } });
  }

  enroll(binding: WalletBinding): Promise<Enrolment> {
    return this.request<Enrolment>("/session/enroll", { method: "POST", body: binding }).then((value) => {
      if (!isRecord(value) || typeof value.participantId !== "string" || value.participantId === "" ||
          typeof value.token !== "string" || value.token === "" ||
          typeof value.expiresAt !== "string" || !Number.isFinite(Date.parse(value.expiresAt)) ||
          typeof value.address !== "string" || value.address.toLowerCase() !== binding.address.toLowerCase()) {
        throw new GatewayError(0, "the enrollment does not bind this wallet");
      }
      return value;
    });
  }

  challenges(token: string): Promise<{ challenges: ChallengeSummary[] }> {
    return this.request<{ challenges: ChallengeSummary[] }>("/session/challenges", { method: "GET", token }).then((value) => {
      if (!isRecord(value) || !Array.isArray(value.challenges) || value.challenges.length > 200) {
        throw new GatewayError(0, "the challenge assignment is malformed");
      }
      const seen = new Set<string>();
      for (const challenge of value.challenges) {
        if (!isRecord(challenge) || typeof challenge.challengeId !== "string" || challenge.challengeId === "" ||
            challenge.challengeId.length > 128 || seen.has(challenge.challengeId) || typeof challenge.cid !== "string" ||
            (challenge.variants !== undefined && (!Array.isArray(challenge.variants) || challenge.variants.length !== 2 ||
              !challenge.variants.every((sequence) => Array.isArray(sequence) && sequence.length === 4 && sequence.every(isRasterDataUrl))))) {
          throw new GatewayError(0, "the challenge assignment contains invalid content");
        }
        seen.add(challenge.challengeId);
      }
      return value;
    });
  }

  submitAnswer(token: string, challengeId: string, answer: string): Promise<{ accepted: boolean }> {
    return this.request("/session/answers", { method: "POST", token, body: { challengeId, answer } });
  }

  result(token: string): Promise<SessionResult> {
    return this.request<SessionResult>("/session/result", { method: "GET", token }).then((value) => {
      if (!isRecord(value) || value.format !== "fod.session-result" || value.version !== 1 || value.algorithm !== "ed25519" ||
          ![value.providerId, value.sessionId, value.utilityId, value.participantId].every((field) => typeof field === "string" && field !== "") ||
          ![value.issuedAt, value.expiresAt].every((stamp) => typeof stamp === "string" && Number.isFinite(Date.parse(stamp))) ||
          typeof value.signature !== "string" || !/^[0-9a-f]{128}$/i.test(value.signature) ||
          !["passed", "not-passed"].includes(value.outcome) || !isRecord(value.score) ||
          ![value.score.assigned, value.score.answered, value.score.correct, value.score.passThreshold].every((count) => Number.isSafeInteger(count) && count >= 0) ||
          !isRecord(value.issuedBy) || typeof value.issuedBy.providerId !== "string" || typeof value.issuedBy.note !== "string" ||
          (value.participantAddress !== undefined && (typeof value.participantAddress !== "string" || !/^0x[0-9a-f]{40}$/i.test(value.participantAddress)))) {
        throw new GatewayError(0, "the session result is malformed");
      }
      return value;
    });
  }
}

/** Same canonical form the gateway signs: sorted keys, no extra whitespace. */
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

function fromHex(value: string): ArrayBuffer {
  if (value.length % 2 !== 0 || !/^[0-9a-f]*$/i.test(value)) {
    throw new Error("not hex");
  }
  const buffer = new ArrayBuffer(value.length / 2);
  const bytes = new Uint8Array(buffer);
  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] = Number.parseInt(value.slice(index * 2, index * 2 + 2), 16);
  }
  return buffer;
}

/**
 * Verifies a provider result with the provider's published key.
 *
 * Returns false rather than throwing when the browser cannot do Ed25519, so the
 * interface can say "not verified" instead of implying a check happened.
 */
export async function verifyProviderResult(publicKeyHex: string, result: SessionResult): Promise<boolean> {
  const { signature, ...unsigned } = result;
  try {
    const key = await crypto.subtle.importKey(
      "spki",
      fromHex(publicKeyHex),
      { name: "Ed25519" },
      false,
      ["verify"],
    );
    return await crypto.subtle.verify(
      { name: "Ed25519" },
      key,
      fromHex(signature),
      new TextEncoder().encode(canonicalJson(unsigned)),
    );
  } catch {
    return false;
  }
}

/** The served key cannot authenticate itself; the connection must carry a pin. */
export async function verifyPinnedProviderResult(
  config: ProviderConfig | null,
  servedKey: string,
  result: SessionResult,
): Promise<boolean> {
  const pin = config?.publicKeyHex;
  if (pin === undefined || !/^[0-9a-f]{88}$/i.test(pin) ||
      pin.toLowerCase() !== servedKey.toLowerCase() || result.providerId !== config?.providerId) {
    return false;
  }
  return verifyProviderResult(pin, result);
}
