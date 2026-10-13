// The claimant's proof of control, issued and checked by this utility (`AH-6`).
//
// A provider-signed result proves what the provider said; it does not prove
// that the party presenting it controls the address the result names. The
// utility therefore issues a one-time challenge bound to its own id and the
// address, the claimant signs it with the wallet, and the server consumes the
// nonce once before any wallet-scoped grant. The challenge text travels in the
// `personal_sign` payload, so the signature covers the utility id as well: a
// challenge issued by one utility cannot be presented at another.

import { randomBytes } from "node:crypto";
import { verifyMessage } from "viem";

const ADDRESS = /^0x[0-9a-fA-F]{40}$/;
const SIGNATURE = /^0x[0-9a-fA-F]{130}$/;
const NONCE = /^[0-9a-f]{32}$/;
const PREFIX = "fod-claim:v2:";
const MAX_AGE_MS = 5 * 60 * 1000;
const MAX_CHALLENGES = 10_000;

export function isClaimAddress(value: string): boolean {
  return ADDRESS.test(value);
}

export function claimChallengeText(input: {
  utilityId: string;
  origin: string;
  address: string;
  nonce: string;
  issuedAt: number;
}): string {
  return `${PREFIX}${input.utilityId}:${input.address.toLowerCase()}:${encodeURIComponent(input.origin)}:${input.nonce}:${input.issuedAt}`;
}

export type ClaimCheck =
  | { ok: true; address: string }
  | { ok: false; error: "bad_address" | "bad_challenge" | "stale_challenge" | "unknown_nonce" | "bad_signature" };

export interface ClaimProof {
  address: string;
  challenge: string;
  signature: string;
}

/**
 * The one-time challenges this process has issued. In-memory and bounded, like
 * the session state: a challenge that is not consumed before its expiry is
 * forgotten, and a challenge that is consumed once cannot be replayed.
 */
export class ClaimChallenges {
  private readonly live = new Map<string, { utilityId: string; address: string; expiresAt: number }>();

  issue(utilityId: string, address: string, origin: string, now: number): { challenge: string; expiresAt: number } | null {
    this.sweep(now);
    if (this.live.size >= MAX_CHALLENGES) {
      return null;
    }
    const nonce = randomBytes(16).toString("hex");
    const expiresAt = now + MAX_AGE_MS;
    this.live.set(nonce, { utilityId, address: address.toLowerCase(), expiresAt });
    return {
      challenge: claimChallengeText({ utilityId, origin, address, nonce, issuedAt: now }),
      expiresAt,
    };
  }

  async check(utilityId: string, origin: string, proof: ClaimProof, now: number): Promise<ClaimCheck> {
    if (!ADDRESS.test(proof.address)) {
      return { ok: false, error: "bad_address" };
    }
    const address = proof.address.toLowerCase();
    if (!proof.challenge.startsWith(PREFIX)) {
      return { ok: false, error: "bad_challenge" };
    }
    const parts = proof.challenge.slice(PREFIX.length).split(":");
    if (parts.length !== 5) {
      return { ok: false, error: "bad_challenge" };
    }
    const [challengeUtility, boundAddress, audience, nonce, stampText] = parts;
    if (challengeUtility !== utilityId || boundAddress !== address || audience !== encodeURIComponent(origin) || !NONCE.test(nonce)) {
      return { ok: false, error: "bad_challenge" };
    }
    const stamp = Number.parseInt(stampText, 10);
    if (!Number.isFinite(stamp) || Math.abs(now - stamp) > MAX_AGE_MS) {
      return { ok: false, error: "stale_challenge" };
    }
    const entry = this.live.get(nonce);
    if (entry === undefined || entry.utilityId !== utilityId || entry.address !== address || now >= entry.expiresAt) {
      return { ok: false, error: "unknown_nonce" };
    }
    if (!SIGNATURE.test(proof.signature)) {
      return { ok: false, error: "bad_signature" };
    }
    try {
      const valid = await verifyMessage({
        address: proof.address as `0x${string}`,
        message: proof.challenge,
        signature: proof.signature as `0x${string}`,
      });
      if (!valid) {
        return { ok: false, error: "bad_signature" };
      }
    } catch {
      return { ok: false, error: "bad_signature" };
    }
    // Verification yields to other requests. Recheck the same entry before
    // consuming it so simultaneous copies of a valid proof cannot both pass.
    if (this.live.get(nonce) !== entry) {
      return { ok: false, error: "unknown_nonce" };
    }
    this.live.delete(nonce);
    return { ok: true, address };
  }

  private sweep(now: number): void {
    for (const [nonce, entry] of this.live) {
      if (now >= entry.expiresAt) {
        this.live.delete(nonce);
      }
    }
  }
}
