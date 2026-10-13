// The provider's application key.
//
// Results are signed with a dedicated Ed25519 key, never the shared node
// wallet key (SPEC.md 8.2). A valid signature proves the provider issued a
// statement; it says nothing about whether the scoring was honest, and the
// result payload says so.

import { generateKeyPairSync, createPrivateKey, createPublicKey, sign, verify } from "node:crypto";
import type { KeyObject } from "node:crypto";
import { constants } from "node:fs";
import { link, mkdtemp, open, rm } from "node:fs/promises";
import { dirname, join } from "node:path";

export interface ApplicationKey {
  privateKey: KeyObject;
  publicKeyHex: string;
}

export interface StoredKeyPair {
  format: "fod.provider-application-key";
  version: 1;
  providerId: string;
  algorithm: "ed25519";
  privateKeyPem: string;
  publicKeyHex: string;
}

export async function loadOrCreateApplicationKey(
  path: string,
  providerId: string,
): Promise<ApplicationKey> {
  try {
    return await loadApplicationKey(path, providerId);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
      throw error;
    }
  }
  const { privateKey, publicKey } = generateKeyPairSync("ed25519");
  const publicKeyHex = publicKey.export({ format: "der", type: "spki" }).toString("hex");
  const stored: StoredKeyPair = {
    format: "fod.provider-application-key",
    version: 1,
    providerId,
    algorithm: "ed25519",
    privateKeyPem: privateKey.export({ format: "pem", type: "pkcs8" }).toString(),
    publicKeyHex,
  };
  // Publish a complete private file without replacing a key another process
  // has already created. A losing first start loads that same winning key.
  const dir = await mkdtemp(join(dirname(path), ".fod-key-"));
  try {
    const candidate = join(dir, "key.json");
    const file = await open(candidate, "wx", 0o600);
    try {
      await file.writeFile(JSON.stringify(stored, null, 2));
      await file.sync();
    } finally {
      await file.close();
    }
    try {
      await link(candidate, path);
      return { privateKey, publicKeyHex };
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "EEXIST") throw error;
      return await loadApplicationKey(path, providerId);
    }
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

async function loadApplicationKey(path: string, providerId: string): Promise<ApplicationKey> {
  const file = await open(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  try {
    const info = await file.stat();
    if (!info.isFile() || info.size > 16 * 1024) throw new Error("unexpected application key file");
    await file.chmod(0o600);
    const stored = JSON.parse(await file.readFile("utf8")) as StoredKeyPair;
    if (stored.format !== "fod.provider-application-key" || stored.version !== 1 ||
        stored.algorithm !== "ed25519" || stored.providerId !== providerId) {
      throw new Error("unexpected application key file or provider identity");
    }
    const privateKey = createPrivateKey(stored.privateKeyPem);
    const publicKeyHex = createPublicKey(privateKey).export({ format: "der", type: "spki" }).toString("hex");
    if (privateKey.asymmetricKeyType !== "ed25519" || stored.publicKeyHex !== publicKeyHex) {
      throw new Error("the application key pair does not match");
    }
    return { privateKey, publicKeyHex };
  } finally {
    await file.close();
  }
}

export function signResult(privateKey: KeyObject, payload: string): string {
  return sign(null, Buffer.from(payload, "utf8"), privateKey).toString("hex");
}

/** Used by the tests and by a utility that wants to check a result itself. */
export function verifyResult(publicKeyHex: string, payload: string, signatureHex: string): boolean {
  const publicKey = createPublicKey({
    key: Buffer.from(publicKeyHex, "hex"),
    format: "der",
    type: "spki",
  });
  return verify(null, Buffer.from(payload, "utf8"), publicKey, Buffer.from(signatureHex, "hex"));
}

/**
 * Canonical JSON for signing: object keys sorted, no insignificant whitespace,
 * so a verifier can rebuild the exact signed bytes.
 */
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
