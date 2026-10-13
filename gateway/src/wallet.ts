// The EVM wallet binding: a participant proves control of an address by
// signing a short, session-bound challenge. Idena derives addresses exactly as
// Ethereum does (`keccak256(pubkey[1:])[12:]`), so the same address is also an
// Idena address; it is a key, not a person.
//
// The challenge is v3: it also binds the public origin the wallet contacted.
import { verifyMessage } from "viem";

export interface WalletBinding {
  address: string;
  challenge: string;
  signature: string;
}

export type WalletCheck =
  | { ok: true; address: string; nonce: string }
  | { ok: false; error: "bad_address" | "bad_challenge" | "stale_challenge" | "bad_signature" };

const ADDRESS = /^0x[0-9a-fA-F]{40}$/;
const SIGNATURE = /^0x[0-9a-fA-F]{130}$/;
const NONCE = /^[0-9a-f]{32}$/;
const PREFIX = "fod-enroll:v3:";
const MAX_AGE_MS = 5 * 60 * 1000;

export function isWalletAddress(value: string): boolean {
  return ADDRESS.test(value);
}

/** The exact text a participant signs, built by the gateway that will check it. */
export function enrollmentChallenge(input: {
  providerId: string;
  sessionId: string;
  origin: string;
  address: string;
  nonce: string;
  issuedAt: number;
}): string {
  return `${PREFIX}${input.providerId}:${input.sessionId}:${input.address.toLowerCase()}:${encodeURIComponent(input.origin)}:${input.nonce}:${input.issuedAt}`;
}

export async function checkWalletBinding(
  context: { providerId: string; sessionId: string; origin: string },
  binding: WalletBinding,
  now: number,
): Promise<WalletCheck> {
  if (!ADDRESS.test(binding.address)) {
    return { ok: false, error: "bad_address" };
  }
  const address = binding.address.toLowerCase();
  if (!binding.challenge.startsWith(PREFIX)) {
    return { ok: false, error: "bad_challenge" };
  }
  const parts = binding.challenge.slice(PREFIX.length).split(":");
  if (parts.length !== 6) {
    return { ok: false, error: "bad_challenge" };
  }
  const [providerId, sessionId, boundAddress, audience, nonce, stampText] = parts;
  if (providerId !== context.providerId || sessionId !== context.sessionId || audience !== encodeURIComponent(context.origin)) {
    return { ok: false, error: "bad_challenge" };
  }
  if (boundAddress !== address || !NONCE.test(nonce)) {
    return { ok: false, error: "bad_challenge" };
  }
  const stamp = Number.parseInt(stampText, 10);
  if (!Number.isFinite(stamp) || Math.abs(now - stamp) > MAX_AGE_MS) {
    return { ok: false, error: "stale_challenge" };
  }
  if (!SIGNATURE.test(binding.signature)) {
    return { ok: false, error: "bad_signature" };
  }
  try {
    const valid = await verifyMessage({
      address: binding.address as `0x${string}`,
      message: binding.challenge,
      signature: binding.signature as `0x${string}`,
    });
    if (!valid) {
      return { ok: false, error: "bad_signature" };
    }
  } catch {
    return { ok: false, error: "bad_signature" };
  }
  return { ok: true, address, nonce };
}
