// The utility's score recomputation.
//
// `app/src/score.ts` holds the single implementation of the rule - canonical
// answer form, the pinned reveal, and the transcript arithmetic. The utility is
// a separate party from the gateway and from the participant, so it runs that
// rule itself on the bytes it was handed instead of believing a stated score.
// The reveal is claimant-supplied and checked against the configured digest.
// Recomputing a provider transcript establishes arithmetic consistency with
// that pinned answer key. It does not establish historical answer correctness,
// honest disclosure timing, independent solving or operator-free utility
// release. The supported grant policy additionally requires answers witnessed
// by the utility during its configured window.

import { verifyTranscript } from "../../app/src/score.ts";
import type { ProviderResult } from "./results.ts";

export interface ScoreRefusal {
  reason: "score_mismatch" | "score_unverifiable";
  detail: string;
}

/**
 * Runs the recomputation when it can and the policy asks for it. Returns null
 * when the grant may proceed.
 */
export async function checkProviderScore(input: {
  result: ProviderResult;
  /** The closed policy's `contentReveal`, as the caller sent it. Untrusted. */
  reveal: unknown;
  /** The digest the reveal has to hash to: this utility's pin. */
  expectedDigest: string;
  /** The policy's `requireScoreProof`: no grant without a recomputation. */
  required: boolean;
}): Promise<ScoreRefusal | null> {
  if (input.reveal === undefined || input.result.transcript === undefined) {
    if (!input.required) {
      // Documented behaviour for a policy that does not require the proof: the
      // result is still signature- and pin-checked, and the decision says the
      // score was not recomputed.
      return null;
    }
    return {
      reason: "score_unverifiable",
      detail:
        "this policy grants only on a recomputable score: send the closed policy's contentReveal " +
        "with a result that carries the transcript it scored",
    };
  }
  const checked = await verifyTranscript({
    sessionId: input.result.sessionId,
    expectedDigest: input.expectedDigest,
    reveal: input.reveal,
    transcript: input.result.transcript,
    score: input.result.score,
    outcome: input.result.outcome,
  });
  if (checked.ok) {
    return null;
  }
  return { reason: "score_mismatch", detail: checked.reason };
}
