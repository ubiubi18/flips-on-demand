import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  GatewayClient,
  GatewayError,
  parseProviderConfig,
  checkedWalletChallenge,
  requestJSON,
  verifyPinnedProviderResult,
  type ChallengeSummary,
  type Enrolment,
  type Lobby,
  type ProviderConfig,
  type SessionPolicy,
  type SessionResult,
} from "../provider";
import { shortHex } from "../format";
import { answerCommitmentText, answersDigest, verifyScore } from "../score";
import { GateSetup } from "./GateSetup";
import { SharedNode } from "./SharedNode";

type Verification = "unknown" | "verified" | "failed";
type ScoreCheckState = { state: "unknown" | "verified" } | { state: "failed"; reason: string };

interface UtilityGrant {
  granted: boolean;
  reason?: string;
  scope?: { statement?: string };
  participantAddress?: string;
}

const EXAMPLE_PROVIDER: ProviderConfig = {
  format: "fod.provider-config",
  version: 1,
  providerId: "example-provider",
  network: "idena-local",
  gatewayUrl: "http://127.0.0.1:8787",
  utilityUrl: "http://127.0.0.1:8788",
  protocol: "fod.session.v1",
  note: "example connection file for the reference integration",
};

export function Session() {
  const [config, setConfig] = useState<ProviderConfig | null>(null);
  const [configError, setConfigError] = useState<string | null>(null);
  const [policy, setPolicy] = useState<SessionPolicy | null>(null);
  const [lobby, setLobby] = useState<Lobby | null>(null);
  const [enrolment, setEnrolment] = useState<Enrolment | null>(null);
  const [challenges, setChallenges] = useState<ChallengeSummary[]>([]);
  const [answers, setAnswers] = useState<Record<string, string>>({});
  // A submission is accepted or pending; the gateway deliberately gives no
  // correctness signal while the session is open (`AH-1`).
  const [submitted, setSubmitted] = useState<Record<string, "submitted" | "pending">>({});
  const [result, setResult] = useState<SessionResult | null>(null);
  const [verification, setVerification] = useState<Verification>("unknown");
  // The digest read before answering, the answers this browser submitted, and
  // the recomputation of the provider's score from the two (`../score.ts`).
  const [pinnedDigest, setPinnedDigest] = useState<string | null>(null);
  const [submittedAnswers, setSubmittedAnswers] = useState<Record<string, string>>({});
  // Accepted gateway responses share this snapshot even before React renders.
  // Each connection gets its own queue, so old responses cannot alter it.
  const answerProgress = useRef({ answers: {} as Record<string, string>, queue: Promise.resolve() });
  const [scoreCheck, setScoreCheck] = useState<ScoreCheckState>({ state: "unknown" });
  // The digest preimage the closed policy published: the utility recomputes the
  // score from it too, so the grant does not rest on the provider's summary.
  const [scoreReveal, setScoreReveal] = useState<unknown>(undefined);
  // Whether the utility accepted the answers this browser committed while the
  // window is open; the utility scores the session from that, not from the
  // provider's statement (`SPEC.md` 8.3).
  const [committed, setCommitted] = useState<boolean | null>(null);
  // The utility's own word for a refused commitment (`outside_window`,
  // `bad_signature`, ...), so a participant can see why and retry.
  const [commitError, setCommitError] = useState<string | null>(null);
  const [grant, setGrant] = useState<UtilityGrant | null>(null);
  const [grantError, setGrantError] = useState<string | null>(null);
  const [walletAddress, setWalletAddress] = useState<string | null>(null);
  const [walletError, setWalletError] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const client = useMemo(() => (config ? new GatewayClient(config.gatewayUrl) : null), [config]);

  const describe = (thrown: unknown): string =>
    thrown instanceof GatewayError ? thrown.message.replace(/_/g, " ") : "something went wrong";

  const importConnection = useCallback(async (file: File) => {
    if (file.size > 32 * 1024) {
      setConfigError("the connection file is too large");
      return;
    }
    const parsed = parseProviderConfig(await file.text());
    if (!parsed.ok) {
      answerProgress.current = { answers: {}, queue: Promise.resolve() };
      setConfigError(parsed.error);
      setConfig(null);
      setPolicy(null);
      setLobby(null);
      setEnrolment(null);
      return;
    }
    setConfigError(null);
    setConfig(parsed.config);
    setPolicy(null);
    setLobby(null);
    setEnrolment(null);
    setChallenges([]);
    setAnswers({});
    setSubmitted({});
    setResult(null);
    setVerification("unknown");
    setPinnedDigest(null);
    setSubmittedAnswers({});
    answerProgress.current = { answers: {}, queue: Promise.resolve() };
    setScoreCheck({ state: "unknown" });
    setScoreReveal(undefined);
    setCommitted(null);
    setCommitError(null);
    setGrant(null);
    setGrantError(null);
  }, []);

  // The built-in example needs no connection file, so a first-time visitor can
  // see the whole journey before running the reference gateway.
  const useExample = () => {
    setConfigError(null);
    setPolicy(null);
    setLobby(null);
    setEnrolment(null);
    setChallenges([]);
    setAnswers({});
    setSubmitted({});
    setResult(null);
    setVerification("unknown");
    setPinnedDigest(null);
    setSubmittedAnswers({});
    answerProgress.current = { answers: {}, queue: Promise.resolve() };
    setScoreCheck({ state: "unknown" });
    setScoreReveal(undefined);
    setCommitted(null);
    setCommitError(null);
    setGrant(null);
    setGrantError(null);
    setConfig(EXAMPLE_PROVIDER);
  };

  // The policy is public, so it loads as soon as a connection is imported.
  useEffect(() => {
    if (!client) {
      return;
    }
    let cancelled = false;
    client
      .policy()
      .then((loaded) => {
        if (!cancelled) {
          if (loaded.providerId !== config?.providerId) throw new GatewayError(0, "the session belongs to another provider");
          setPolicy(loaded);
          // The first policy read is the pin: it is what the participant saw
          // before answering, and a reveal served later has to hash to it.
          if (typeof loaded.contentDigest === "string") {
            setPinnedDigest((current) => current ?? loaded.contentDigest);
          }
          setError(null);
        }
      })
      .catch((thrown) => {
        if (!cancelled) {
          setError(describe(thrown));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [client]);

  // The lobby is public: it says whether the shared window is open and how
  // many participants have joined, so a buyer can see the on-demand session
  // before enrolling.
  useEffect(() => {
    if (!client || !policy) {
      return;
    }
    let cancelled = false;
    const load = () => {
      client
        .lobby()
        .then((loaded) => {
          if (!cancelled) {
            setLobby(loaded);
          }
        })
        .catch(() => {
          // The policy error, if any, stays the one the reader sees.
        });
    };
    load();
    const timer = setInterval(load, 5000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [client, policy]);

  const injectedWallet = (): { request: (args: { method: string; params?: unknown[] }) => Promise<unknown> } | null => {
    const injected = (
      window as unknown as {
        ethereum?: { request: (args: { method: string; params?: unknown[] }) => Promise<unknown> };
      }
    ).ethereum;
    return injected ?? null;
  };

  const connectWallet = async () => {
    const injected = injectedWallet();
    if (!injected) {
      setWalletError("No EVM wallet was found in this browser.");
      return;
    }
    try {
      const accounts = (await injected.request({ method: "eth_requestAccounts" })) as string[];
      const address = accounts?.[0] ?? "";
      if (!/^0x[0-9a-fA-F]{40}$/.test(address)) {
        setWalletError("The wallet returned no usable address.");
        return;
      }
      setWalletAddress(address.toLowerCase());
      setWalletError(null);
    } catch {
      setWalletError("The wallet request was refused.");
    }
  };

  const enroll = async () => {
    if (!client || !policy) {
      return;
    }
    if (walletAddress === null) {
      setError("Connect an EVM wallet first: the gate session binds the address that solves it.");
      return;
    }
    setBusy(true);
    try {
      const injected = injectedWallet();
      if (!injected) {
        throw new Error("no wallet");
      }
      // The gateway issues the one-time challenge; the wallet signs exactly
      // that text, and the gateway consumes it once (`AH-3`).
      const issued = await client.enrollChallenge(walletAddress);
      const challenge = checkedWalletChallenge(issued, {
        kind: "enroll", providerId: config!.providerId, sessionId: policy.sessionId, address: walletAddress,
        endpointOrigin: config!.gatewayUrl,
      });
      const signature = (await injected.request({
        method: "personal_sign",
        params: [challenge, walletAddress],
      })) as string;
      const enrolled = await client.enroll({
        address: walletAddress,
        challenge,
        signature,
      });
      setEnrolment(enrolled);
      const assigned = await client.challenges(enrolled.token);
      setChallenges(assigned.challenges);
      setError(null);
    } catch (thrown) {
      if (thrown instanceof GatewayError) {
        setError(describe(thrown));
      } else if (thrown instanceof Error && thrown.message === "no wallet") {
        setError("Connect an EVM wallet first.");
      } else {
        setError("The wallet did not sign the enrollment.");
      }
    } finally {
      setBusy(false);
    }
  };

  // A typed answer and a chosen sequence are the same thing to the gateway: the
  // canonical text it scores. The choice buttons hand it "left" or "right".
  //
  // Every accepted answer is also committed to the utility, signed by the same
  // wallet that will claim the grant: that commitment has to arrive while the
  // window is open, which is what lets the utility score the session itself
  // instead of believing the provider (`SPEC.md` 8.3).
  const commitAnswers = async (answers: Record<string, string>, progress: typeof answerProgress.current) => {
    const utilityBase = config?.utilityUrl?.replace(/\/$/, "");
    const injected = injectedWallet();
    if (utilityBase === undefined || !policy || walletAddress === null || !injected) {
      return;
    }
    const list = Object.entries(answers).map(([challengeId, answer]) => ({ challengeId, answer }));
    if (list.length === 0) {
      return;
    }
    try {
      const issuedAt = Date.now();
      const digest = await answersDigest(list);
      const text = answerCommitmentText({
        utilityId: policy.utilityId,
        sessionId: policy.sessionId,
        address: walletAddress,
        issuedAt,
        digest,
      });
      const signature = (await injected.request({
        method: "personal_sign",
        params: [text, walletAddress],
      })) as string;
      const response = await requestJSON<{ committed: boolean; error?: string }>(`${utilityBase}/answer-commitment`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          address: walletAddress,
          sessionId: policy.sessionId,
          answers: list,
          issuedAt,
          signature,
        }),
      });
      if (answerProgress.current === progress && progress.answers === answers) {
        setCommitted(response?.committed === true);
        setCommitError(response?.committed === true ? null : "the utility refused the commitment");
      }
    } catch (thrown) {
      // The next accepted answer sends the whole set again; the claim is only
      // granted when a commitment arrived inside the window.
      if (answerProgress.current === progress && progress.answers === answers) {
        setCommitted(false);
        setCommitError(describe(thrown));
      }
    }
  };

  const submit = async (challengeId: string, chosen?: string) => {
    if (!client || !enrolment) {
      return;
    }
    const answer = chosen ?? answers[challengeId] ?? "";
    const progress = answerProgress.current;
    setSubmitted((current) => ({ ...current, [challengeId]: "pending" }));
    try {
      const response = await client.submitAnswer(enrolment.token, challengeId, answer);
      if (answerProgress.current !== progress) {
        return;
      }
      if (response.accepted) {
        setSubmitted((current) => ({ ...current, [challengeId]: "submitted" }));
        // What this browser actually submitted is the ground truth the score
        // check later compares the provider's transcript against.
        const next = { ...progress.answers, [challengeId]: answer };
        progress.answers = next;
        setSubmittedAnswers(next);
        setCommitted(null);
        setCommitError(null);
        // Serialize wallet prompts and POSTs. A queued snapshot superseded
        // before signing can be skipped; the next one contains all answers.
        progress.queue = progress.queue.then(async () => {
          if (answerProgress.current === progress && progress.answers === next) {
            await commitAnswers(next, progress);
          }
        });
      }
      setError(null);
    } catch (thrown) {
      if (answerProgress.current !== progress) {
        return;
      }
      setSubmitted((current) => {
        const next = { ...current };
        delete next[challengeId];
        return next;
      });
      setError(describe(thrown));
    }
  };

  const fetchResult = async () => {
    if (!client || !enrolment) {
      return;
    }
    setBusy(true);
    setVerification("unknown");
    setScoreCheck({ state: "unknown" });
    try {
      const [loaded, key] = await Promise.all([client.result(enrolment.token), client.providerKey()]);
      if (loaded.sessionId !== policy?.sessionId || loaded.utilityId !== policy.utilityId ||
          loaded.participantId !== enrolment.participantId || loaded.participantAddress?.toLowerCase() !== enrolment.address?.toLowerCase()) {
        throw new GatewayError(0, "the result belongs to another session or participant");
      }
      setResult(loaded);
      setGrant(null);
      setGrantError(null);
      // The key the connection file recorded comes first: a host that serves a
      // different key now cannot turn a result into a verified one, which is
      // what "verified in browser" is supposed to mean (`SPEC.md` 8.2).
      const verified = await verifyPinnedProviderResult(config, String(key.publicKeyHex), loaded);
      setVerification(verified ? "verified" : "failed");
      if (verified) {
        // The signature proves who spoke. This proves what they said about the
        // score: the closed policy carries the digest preimage, and the claim is
        // only offered when the provider's transcript, this browser's own
        // answers and the pinned flip set reproduce the signed score (`10.4`).
        const closed = await client.policy();
        setScoreReveal(closed.contentReveal);
        const checked = await verifyScore({
          sessionId: loaded.sessionId,
          expectedDigest: pinnedDigest ?? "",
          reveal: closed.contentReveal,
          transcript: loaded.transcript,
          answers: submittedAnswers,
          score: loaded.score,
          outcome: loaded.outcome,
        });
        setScoreCheck(checked.ok ? { state: "verified" } : { state: "failed", reason: checked.reason });
      } else {
        setScoreCheck({ state: "unknown" });
      }
      setError(null);
    } catch (thrown) {
      if (thrown instanceof GatewayError && thrown.status === 403 && thrown.message.includes("session_open")) {
        setError("The result is signed after the session closes; answers are kept for it.");
      } else {
        setError(describe(thrown));
      }
    } finally {
      setBusy(false);
    }
  };

  // The provider's result is only a claim until the utility checks it. The
  // testbed utility is another public endpoint; it verifies the provider
  // signature itself and answers with its own grant decision.
  //
  // A claim is only offered for a result this browser verified itself, never for
  // one that merely reads `outcome: "passed"`, and only when this browser
  // recomputed that score from its own answers and the revealed answer key. The
  // same predicate gates the button and the call, so the two cannot drift apart.
  const claimable =
    result !== null &&
    result.outcome === "passed" &&
    verification === "verified" &&
    scoreCheck.state === "verified";
  const textOnlyChallenges = challenges.filter((challenge) => !Array.isArray(challenge.variants) || challenge.variants.length !== 2).length;
  const visualAnswerRequired = policy !== null && challenges.length > 0 && textOnlyChallenges < policy.rules.passThreshold;
  const claim = async () => {
    if (!config?.utilityUrl || !result || !claimable) {
      return;
    }
    setBusy(true);
    try {
      const utilityBase = config.utilityUrl.replace(/\/$/, "");
      // A result that names an address is only honoured by the utility when
      // the claimant proves control of it with a one-time challenge (`AH-6`).
      let proof: { address: string; challenge: string; signature: string } | undefined;
      if (result.participantAddress !== undefined) {
        const injected = injectedWallet();
        if (!injected) {
          throw new Error("no wallet");
        }
        const issued = await requestJSON<unknown>(`${utilityBase}/access-challenge`, {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ address: result.participantAddress }),
        });
        const challenge = checkedWalletChallenge(issued, {
          kind: "claim", utilityId: result.utilityId, address: result.participantAddress,
          endpointOrigin: utilityBase,
        });
        const signature = (await injected.request({
          method: "personal_sign",
          params: [challenge, result.participantAddress],
        })) as string;
        proof = { address: result.participantAddress, challenge, signature };
      }
      const response = await requestJSON<UtilityGrant>(`${utilityBase}/access`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          result,
          ...(proof ?? {}),
          ...(scoreReveal === undefined ? {} : { scoreReveal }),
        }),
      });
      if (typeof response?.granted !== "boolean" ||
          (response.reason !== undefined && typeof response.reason !== "string") ||
          (response.scope?.statement !== undefined && typeof response.scope.statement !== "string") ||
          (response.participantAddress !== undefined && typeof response.participantAddress !== "string")) {
        throw new GatewayError(0, "the utility returned an invalid decision");
      }
      setGrant(response);
      setGrantError(null);
    } catch (thrown) {
      setGrantError(
        thrown instanceof Error && thrown.message === "no wallet"
          ? "connect your wallet to claim the grant"
          : describe(thrown),
      );
    } finally {
      setBusy(false);
    }
  };

  if (!config) {
    return (
      <section className="empty">
        <h2>Use flips</h2>
        <p>Import the connection file a provider gave you, or try the example gate.</p>
        <div className="actions">
          <button type="button" className="primary" onClick={useExample}>
            Try the example gate
          </button>
          <label className="import">
            <input
              type="file"
              accept="application/json,.json"
              aria-label="Import connection"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) {
                  void importConnection(file);
                }
                event.target.value = "";
              }}
            />
            <span>Import connection</span>
          </label>
        </div>
        <p className="muted">
          The example gate uses the reference provider at http://127.0.0.1:8787.
          Start it with <code>node gateway/src/main.ts gateway/example-config.json</code>.
        </p>
        {configError !== null && (
          <p className="banner" role="alert">
            {configError}
          </p>
        )}
        <GateSetup />
        <SharedNode />
      </section>
    );
  }

  return (
    <section>
      <header className="section-head">
        <h2>Use flips</h2>
        <span className="muted">{config.providerId}</span>
        <span className="muted">{config.gatewayUrl}</span>
      </header>

      {config.providerId === "example-provider" && (
        <p className="notice">
          The example gate has one real flip with two sequences and one typed
          challenge. The provider signs the result; the utility decides what it
          unlocks. This is the testbed: example flips, a test utility, no real
          funds.
        </p>
      )}

      {lobby && (
        <dl className="facts">
          <div>
            <dt>Session window</dt>
            <dd>
              {lobby.window === "open"
                ? "open"
                : lobby.window === "not-open"
                  ? `opens ${lobby.opensAt}`
                  : "closed"}
            </dd>
          </div>
          <div>
            <dt>Participants</dt>
            <dd>{lobby.participants}</dd>
          </div>
          <div>
            <dt>Challenges</dt>
            <dd>{lobby.challenges}</dd>
          </div>
        </dl>
      )}

      {error !== null && (
        <p className="banner" role="alert">
          {error}
        </p>
      )}

      {policy && (
        <dl className="facts">
          <div>
            <dt>Session</dt>
            <dd>{policy.sessionId}</dd>
          </div>
          <div>
            <dt>Reward</dt>
            <dd>{policy.utilityId}</dd>
          </div>
          <div>
            <dt>Answers needed to pass</dt>
            <dd>{policy.rules.passThreshold}</dd>
          </div>
          <div>
            <dt>Closes</dt>
            <dd>{policy.rules.closesAt}</dd>
          </div>
          <div>
            <dt>Flip pool epoch</dt>
            <dd>{policy.provenance.epoch}</dd>
          </div>
          <div>
            <dt>Answers from</dt>
            {/* The provider's own statement, not a chain fact. */}
            <dd>{policy.provenance.answers ?? "not stated"}</dd>
          </div>
        </dl>
      )}

      <div className="actions">
        <button type="button" className="linklike" onClick={connectWallet} disabled={busy}>
          {walletAddress === null ? "Connect EVM wallet" : `Wallet ${shortHex(walletAddress, 6)}`}
        </button>
        {walletAddress !== null && (
          <span className="muted">the address is bound to the enrollment, the result and the grant</span>
        )}
      </div>
      {walletError !== null && (
        <p className="banner" role="alert">
          {walletError}
        </p>
      )}
      {!enrolment && (
        <div className="actions">
          <button
            type="button"
            className="primary"
            onClick={enroll}
            disabled={busy || !policy || walletAddress === null || (lobby !== null && lobby.window !== "open")}
          >
            Start session
          </button>
          {walletAddress === null && <span className="muted">connect a wallet to enroll</span>}
        </div>
      )}

      {enrolment && (
        <>
          {visualAnswerRequired && (
            <p className="notice" role="status">
              This session requires at least one visual flip answer to pass. This version has no equivalent nonvisual challenge.
            </p>
          )}
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Flip</th>
                  <th>Your answer</th>
                  <th>State</th>
                </tr>
              </thead>
              <tbody>
                {challenges.map((challenge) => {
                  const state = submitted[challenge.challengeId];
                  const variants = Array.isArray(challenge.variants)
                    ? challenge.variants.filter((sequence) => Array.isArray(sequence) && sequence.length > 0)
                    : [];
                  const answered = state === "submitted";
                  return (
                    <tr key={challenge.challengeId}>
                      {variants.length === 2 ? (
                        <td>
                          <div className="flip-variants">
                            {variants.map((sequence, variant) => (
                              <figure key={`${challenge.challengeId}-${variant}`} className="flip-variant">
                                <div className="flip-sequence">
                                  {sequence.map((image, at) => (
                                    <img
                                      key={`${challenge.challengeId}-${variant}-${at}`}
                                      src={image}
                                      alt={`Flip ${challenge.challengeId}, sequence ${variant + 1}, picture ${at + 1}`}
                                    />
                                  ))}
                                </div>
                                {!answered && (
                                  <button
                                    type="button"
                                    onClick={() => void submit(challenge.challengeId, variant === 0 ? "left" : "right")}
                                  >
                                    {variant === 0 ? "Left" : "Right"}
                                  </button>
                                )}
                              </figure>
                            ))}
                          </div>
                        </td>
                      ) : (
                        <td className="mono" title={challenge.cid}>
                          {challenge.challengeId}
                        </td>
                      )}
                      <td>
                        {variants.length === 2 ? (
                          <span className="muted">choose the sequence</span>
                        ) : (
                          <input
                            type="text"
                            value={answers[challenge.challengeId] ?? ""}
                            disabled={answered}
                            onChange={(event) =>
                              setAnswers((current) => ({ ...current, [challenge.challengeId]: event.target.value }))
                            }
                            aria-label={`Answer for ${challenge.challengeId}`}
                          />
                        )}
                      </td>
                      <td>
                        {state === undefined && variants.length !== 2 && (
                          <button
                            type="button"
                            className="linklike"
                            onClick={() => void submit(challenge.challengeId)}
                          >
                            Submit
                          </button>
                        )}
                        {state === "pending" && <span className="muted">sending</span>}
                        {state === "submitted" && <span className="chip chip-sold">submitted</span>}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          {Object.keys(submittedAnswers).length > 0 && (
            <p className="muted">
              {committed === true
                ? "answers committed to the utility while the window is open, so it can score the session itself"
                : committed === false
                  ? `the utility has not accepted the commitment yet${commitError === null ? "" : ` (${commitError})`}; it is sent again with the next answer`
                  : "committing the answers to the utility"}
            </p>
          )}
          <div className="actions">
            <button type="button" className="primary" onClick={fetchResult} disabled={busy}>
              Get result
            </button>
            <span className="muted">
              session ends {new Date(enrolment.expiresAt).toISOString().slice(11, 16)} UTC
            </span>
          </div>
        </>
      )}

      {result && (
        <div className="result">
          <h3>Provider result</h3>
          <dl className="facts">
            <div>
              <dt>Result</dt>
              <dd>{result.outcome}</dd>
            </div>
            <div>
              <dt>Score</dt>
              <dd>
                {result.score.correct} of {result.score.assigned} (threshold {result.score.passThreshold})
              </dd>
            </div>
            <div>
              <dt>Issued by</dt>
              <dd>{result.issuedBy.providerId}</dd>
            </div>
            {result.participantAddress && (
              <div>
                <dt>Bound address</dt>
                <dd className="mono">{result.participantAddress}</dd>
              </div>
            )}
            <div>
              <dt>Signature</dt>
              <dd>
                {verification === "verified" && <span className="chip chip-sold">verified in browser</span>}
                {verification === "failed" && <span className="chip chip-cancelled">not verified</span>}
                {verification === "unknown" && <span className="muted">not checked</span>}
              </dd>
            </div>
            <div>
              <dt>Score check</dt>
              <dd>
                {scoreCheck.state === "verified" && (
                  <span className="chip chip-sold">recomputed from your answers</span>
                )}
                {scoreCheck.state === "failed" && <span className="chip chip-cancelled">not verifiable</span>}
                {scoreCheck.state === "unknown" && <span className="muted">not checked</span>}
              </dd>
            </div>
          </dl>
          {scoreCheck.state === "failed" && (
            <p className="banner" role="alert">
              {scoreCheck.reason}
            </p>
          )}
          {config.utilityUrl && (
            <div className="actions">
              <button
                type="button"
                className="primary"
                onClick={claim}
                disabled={busy || !claimable}
              >
                Claim test access
              </button>
              <span className="muted">utility {config.utilityUrl}</span>
            </div>
          )}
          {grantError !== null && (
            <p className="banner" role="alert">
              {grantError}
            </p>
          )}
          {grant && (
            <dl className="facts">
              <div>
                <dt>Grant</dt>
                <dd>{grant.granted ? "granted" : grant.reason ?? "refused"}</dd>
              </div>
              {grant.scope?.statement && (
                <div>
                  <dt>Scope</dt>
                  <dd>{grant.scope.statement}</dd>
                </div>
              )}
              {grant.participantAddress && (
                <div>
                  <dt>Address</dt>
                  <dd className="mono">{grant.participantAddress}</dd>
                </div>
              )}
            </dl>
          )}
          <details>
            <summary>Signed statement</summary>
            <p className="muted">{result.issuedBy.note}</p>
            <p className="mono">{result.signature}</p>
          </details>
        </div>
      )}

      <GateSetup />
      <SharedNode />
    </section>
  );
}
