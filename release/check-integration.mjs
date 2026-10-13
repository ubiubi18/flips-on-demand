// Runs the three services together and checks the loop that ties them:
// gateway session -> provider-signed result -> utility decision.
//
//   node release/check-integration.mjs
//
// No browser and no chain: the gateway and the utility are started here on
// loopback ports, the customer is played by this script, and every step is a
// real HTTP request against the real services. The utility's policy is built
// from the gateway's published key, so the signature has to survive the
// independent verifier for a grant to happen.

import { spawn, spawnSync } from "node:child_process";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { createRequire } from "node:module";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..");
const GATEWAY_PORT = 8787;
const UTILITY_PORT = 8788;
const gatewayRequire = createRequire(join(root, "gateway", "package.json"));
const { privateKeyToAccount } = gatewayRequire("viem/accounts");

const failures = [];
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "ok  " : "FAIL"} ${name}${detail ? " - " + detail : ""}`);
  if (!ok) failures.push(name);
};

const children = [];
function start(command, args, cwd) {
  const child = spawn(command, args, { cwd, stdio: ["ignore", "pipe", "pipe"] });
  children.push(child);
  return child;
}

async function waitFor(url, child, attempts = 50) {
  let listening = false;
  let output = "";
  child.stdout.on("data", (chunk) => {
    output += String(chunk);
    listening ||= output.includes(`listening on ${new URL(url).host}`);
  });
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    if (child.exitCode !== null || child.signalCode !== null) return false;
    if (!listening) {
      await new Promise((resolve) => setTimeout(resolve, 100));
      continue;
    }
    try {
      const response = await fetch(url);
      if (response.ok) {
        return true;
      }
    } catch {
      // not listening yet
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  return false;
}

// A distinct deterministic wallet per customer: a gate session admits one live
// enrollment per address (`AH-1`), and a second customer needs a second key.
let walletCounter = 1;
const wallets = new Map();
async function enrollCustomer() {
  const account = privateKeyToAccount(`0x${walletCounter.toString(16).padStart(64, "0")}`);
  walletCounter += 1;
  wallets.set(account.address.toLowerCase(), account);
  const challengeResponse = await fetch(`http://127.0.0.1:${GATEWAY_PORT}/session/enroll-challenge`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ address: account.address }),
  });
  const { challenge } = await challengeResponse.json();
  const signature = await account.signMessage({ message: challenge });
  return (
    await fetch(`http://127.0.0.1:${GATEWAY_PORT}/session/enroll`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ address: account.address, challenge, signature }),
    })
  ).json();
}

// The utility only honours a result that names an address when the presenter
// proves control of it with a one-time challenge (`AH-6`).
// The answers a customer committed while the window was open, signed by the
// wallet that will claim the grant. The utility scores the session from these
// (`SPEC.md` 8.3), so a claim is only possible if one arrived in time.
async function commitAnswers(address, answers) {
  const account = wallets.get(String(address).toLowerCase());
  if (account === undefined) {
    return { committed: false, error: "test-missing-wallet" };
  }
  const { answerCommitmentText, answersDigest } = await import(
    pathToFileURL(join(root, "app", "src", "score.ts")).href
  );
  const issuedAt = Date.now();
  const text = answerCommitmentText({
    utilityId: "utility-example",
    sessionId: "session-example-1",
    address,
    issuedAt,
    digest: await answersDigest(answers),
  });
  const signature = await account.signMessage({ message: text });
  return (
    await fetch(`http://127.0.0.1:${UTILITY_PORT}/answer-commitment`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        address,
        sessionId: "session-example-1",
        answers,
        issuedAt,
        signature,
      }),
    })
  ).json();
}

async function claimAccess(result, scoreReveal) {
  const body = { result };
  if (scoreReveal !== undefined) {
    // The utility recomputes the score from this and the result's transcript.
    body.scoreReveal = scoreReveal;
  }
  if (typeof result.participantAddress === "string") {
    const account = wallets.get(result.participantAddress.toLowerCase());
    if (account === undefined) {
      return { granted: false, reason: "test-missing-wallet" };
    }
    const issued = await (
      await fetch(`http://127.0.0.1:${UTILITY_PORT}/access-challenge`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ address: result.participantAddress }),
      })
    ).json();
    body.address = result.participantAddress;
    body.challenge = issued.challenge;
    body.signature = await account.signMessage({ message: issued.challenge });
  }
  return (
    await fetch(`http://127.0.0.1:${UTILITY_PORT}/access`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    })
  ).json();
}

async function main() {
  // The participant's score check has its own suite, including the adversarial
  // cases the walk cannot stage (a provider that signs a score its own
  // transcript does not support). It starts no ports, so it runs first.
  const scoreSuite = spawnSync("npm", ["test"], { cwd: join(root, "app"), encoding: "utf8" });
  const scoreLines = `${scoreSuite.stdout ?? ""}${scoreSuite.stderr ?? ""}`
    .split("\n")
    .map((line) => line.trimEnd())
    .filter((line) => line !== "");
  check(
    "the participant's score check passes its suite",
    scoreSuite.status === 0,
    scoreLines.slice(-4).join(" | ").slice(0, 4000),
  );
  if (scoreSuite.status !== 0) {
    return;
  }

  // The offline interface walk is part of the release check: if it cannot run
  // - a missing browser dependency, a missing build - this fails here rather
  // than being skipped (`AH-16`). It runs first because it starts its own
  // gateway on the same loopback port this script uses below.
  const walk = spawnSync("npm", ["run", "verify"], { cwd: join(root, "app"), encoding: "utf8" });
  const walkLines = `${walk.stdout ?? ""}${walk.stderr ?? ""}`
    .split("\n")
    .map((line) => line.trimEnd())
    .filter((line) => line !== "");
  if (walk.status !== 0) {
    // A failing walk must name its own cause: keep a long tail in the check
    // and leave the full output beside the run (`AH-22`).
    const logPath = join(tmpdir(), "fod-walk-failure.log");
    await writeFile(logPath, `${walkLines.join("\n")}\n`);
    console.log(`     walk log: ${logPath}`);
  }
  check(
    "the offline interface walk passes",
    walk.status === 0,
    (walk.status === 0 ? walkLines.slice(-2) : walkLines.slice(-40)).join(" | ").slice(0, 4000),
  );
  if (walk.status !== 0) {
    return;
  }

  // The walk gives the session its own short window: the customers answer
  // while it is open and collect their signed results just after it closes,
  // which is the only point at which a result exists (`AH-1`).
  const examplePool = JSON.parse(await readFile(join(root, "gateway", "example-pool.json"), "utf8"));
  const exampleConfig = JSON.parse(await readFile(join(root, "gateway", "example-config.json"), "utf8"));
  const runDir = await mkdtemp(join(tmpdir(), "fod-integration-gateway-"));
  const poolPath = join(runDir, "pool.json");
  const configPath = join(runDir, "config.json");
  examplePool.rules.opensAt = new Date(Date.now() - 60_000).toISOString();
  // Long enough for two enrolments, four answers and the two commitments the
  // utility scores the session from, all of which have to land inside it.
  examplePool.rules.closesAt = new Date(Date.now() + 12_000).toISOString();
  await writeFile(poolPath, JSON.stringify(examplePool));
  await writeFile(
    configPath,
    JSON.stringify({
      ...exampleConfig,
      poolPath,
      applicationKeyPath: join(runDir, "provider-application-key.json"),
    }),
  );
  const gateway = start("node", ["src/main.ts", configPath], join(root, "gateway"));
  let gatewayLog = "";
  gateway.stderr.on("data", (chunk) => {
    gatewayLog += String(chunk);
  });
  if (!(await waitFor(`http://127.0.0.1:${GATEWAY_PORT}/session/policy`, gateway))) {
    check("gateway starts", false, gatewayLog.slice(0, 120));
    return;
  }
  check("gateway starts", true);

  // The utility trusts the provider by the key the provider publishes, not by
  // anything the caller says about itself.
  const keyResponse = await fetch(`http://127.0.0.1:${GATEWAY_PORT}/provider/key`);
  const providerKey = await keyResponse.json();
  // And it pins what the session is, not only what it is called: this is the
  // digest of the content the operator examined (`SPEC.md` 8.3).
  const gatewayPolicy = await (await fetch(`http://127.0.0.1:${GATEWAY_PORT}/session/policy`)).json();
  const dir = await mkdtemp(join(tmpdir(), "fod-integration-"));
  const policyPath = join(dir, "policy.json");
  await writeFile(
    policyPath,
    JSON.stringify({
      format: "fod.utility-policy",
      version: 1,
      utilityId: "utility-example",
      sessions: ["session-example-1"],
      sessionDigests: { "session-example-1": gatewayPolicy.contentDigest },
      requireOutcome: "passed",
      // The utility recomputes the score itself from the transcript and the
      // answer key the pinned content carries (`SPEC.md` 8.3).
      requireScoreProof: true,
      // And it scores the session from the answers the customers commit while
      // the window is open, so the provider's verdict is not what decides.
      requireCommittedAnswers: true,
      sessionWindows: {
        "session-example-1": { opensAt: gatewayPolicy.rules.opensAt, closesAt: gatewayPolicy.rules.closesAt },
      },
      grantsPerParticipant: 1,
      grantTtlSeconds: 3600,
      providers: [
        {
          providerId: providerKey.providerId,
          publicKeyHex: providerKey.publicKeyHex,
          gatewayUrl: `http://127.0.0.1:${GATEWAY_PORT}`,
        },
      ],
      scope: {
        kind: "provider-participant",
        statement: "scoped to a provider-issued participant id, not to a person",
      },
    }),
  );
  const utility = start("node", ["src/main.ts", policyPath, String(UTILITY_PORT)], join(root, "utility"));
  let utilityLog = "";
  utility.stderr.on("data", (chunk) => {
    utilityLog += String(chunk);
  });
  if (!(await waitFor(`http://127.0.0.1:${UTILITY_PORT}/policy`, utility))) {
    check("utility starts", false, utilityLog.slice(0, 120));
    return;
  }
  check("utility starts", true);

  // Two customers answer while the session is open: a gate session admits one
  // live enrollment per address (`AH-1`), so the second uses its own key.
  const enrolled = await enrollCustomer();
  const auth = { authorization: `Bearer ${enrolled.token}`, "content-type": "application/json" };
  const first = await fetch(`http://127.0.0.1:${GATEWAY_PORT}/session/answers`, {
    method: "POST",
    headers: auth,
    // c1 carries content, so it is a flip: the answer is the sequence the
    // participant chooses, not a phrase. `gateway/example-pool.json` records
    // the digest of `right` for this challenge.
    body: JSON.stringify({ challengeId: "c1", answer: "right" }),
  });
  const firstBody = await first.json();
  const second = await fetch(`http://127.0.0.1:${GATEWAY_PORT}/session/answers`, {
    method: "POST",
    headers: auth,
    body: JSON.stringify({ challengeId: "c2", answer: "second answer" }),
  });
  const secondBody = await second.json();
  check(
    "answers are accepted without a correctness signal while the session is open",
    first.status === 202 &&
      second.status === 202 &&
      firstBody.accepted === true &&
      firstBody.correct === undefined &&
      secondBody.accepted === true &&
      secondBody.correct === undefined,
    JSON.stringify([firstBody, secondBody]),
  );
  // What the customer answered, committed to the utility inside the window.
  const firstCommitment = await commitAnswers(enrolled.address, [
    { challengeId: "c1", answer: "right" },
    { challengeId: "c2", answer: "second answer" },
  ]);
  check(
    "the first customer's answers are committed to the utility inside the window",
    firstCommitment.committed === true,
    firstCommitment.error ?? "",
  );

  const secondEnrolment = await enrollCustomer();
  const secondAuth = { authorization: `Bearer ${secondEnrolment.token}`, "content-type": "application/json" };
  for (const [challengeId, answer] of [["c1", "right"], ["c2", "second answer"]]) {
    const accepted = await fetch(`http://127.0.0.1:${GATEWAY_PORT}/session/answers`, {
      method: "POST",
      headers: secondAuth,
      body: JSON.stringify({ challengeId, answer }),
    });
    if (accepted.status !== 202) {
      check("the second customer answers while the session is open", false, `${challengeId}: ${accepted.status}`);
    }
  }
  const secondCommitment = await commitAnswers(secondEnrolment.address, [
    { challengeId: "c1", answer: "right" },
    { challengeId: "c2", answer: "second answer" },
  ]);
  check(
    "the second customer's answers are committed to the utility inside the window",
    secondCommitment.committed === true,
    secondCommitment.error ?? "",
  );

  // The signed result exists only after the session is irrevocably closed.
  await new Promise((resolve) =>
    setTimeout(resolve, Math.max(0, Date.parse(examplePool.rules.closesAt) - Date.now() + 500)),
  );
  const result = await (
    await fetch(`http://127.0.0.1:${GATEWAY_PORT}/session/result`, { headers: auth })
  ).json();
  check(
    "the provider signs a passing result after the session closes",
    result.outcome === "passed" && typeof result.signature === "string",
  );
  check(
    "the signed result names the session content the policy publishes",
    result.contentDigest === gatewayPolicy.contentDigest,
    `${result.contentDigest} against ${gatewayPolicy.contentDigest}`,
  );
  // `AH-2`: the closed session publishes the digest preimage, and it recomputes
  // to the digest the utility pinned before the session ran.
  const closedPolicy = await (await fetch(`http://127.0.0.1:${GATEWAY_PORT}/session/policy`)).json();
  const { digestOfContent } = await import(pathToFileURL(join(root, "gateway", "src", "pool.ts")).href);
  check(
    "the closed session reveals a preimage that recomputes to the pinned digest",
    digestOfContent(closedPolicy.contentReveal) === gatewayPolicy.contentDigest,
  );

  // The utility decides, and it verifies the signature itself.
  const utilityPolicy = await (await fetch(`http://127.0.0.1:${UTILITY_PORT}/policy`)).json();
  check(
    "the utility pinned the session content it examined",
    utilityPolicy.sessionDigests?.["session-example-1"] === gatewayPolicy.contentDigest,
    JSON.stringify(utilityPolicy.sessionDigests),
  );
  const decision = await claimAccess(result, closedPolicy.contentReveal);
  check("the utility grants access to the session result", decision.granted === true, decision.reason ?? "");
  check(
    "the grant was scored by the utility from the committed answers, not by the provider",
    decision.scoring?.by === "utility" && decision.scoring?.correct === 2 && decision.scoring?.passThreshold === 2,
    JSON.stringify(decision.scoring ?? null),
  );
  check(
    "the grant states its scope",
    typeof decision.scope?.statement === "string" && decision.scope.statement.includes("not to a person"),
  );

  // A result whose score line was edited must not survive the utility.
  const tampered = {
    ...result,
    score: { ...result.score, correct: Math.max(0, result.score.correct - 1) },
  };
  const tamperedDecision = await claimAccess(tampered, closedPolicy.contentReveal);
  check(
    "a tampered result is refused",
    tamperedDecision.reason === "bad_signature",
    tamperedDecision.reason ?? "",
  );

  // A forged result that carries the provider's own valid signature but a
  // verdict of its own: this one says the participant failed. The utility
  // scores the session itself from the answers the customer committed, so the
  // provider's verdict is not what decides (`SPEC.md` 8.3) - a provider cannot
  // deny a pass it does not like either.
  const { loadOrCreateApplicationKey, signResult, canonicalJson } = await import(
    pathToFileURL(join(root, "gateway", "src", "keys.ts")).href
  );
  const applicationKey = await loadOrCreateApplicationKey(
    join(runDir, "provider-application-key.json"),
    "example-provider",
  );
  // Re-enrolment gets a fresh participant id, so the quota grants again: the
  // utility reports that scope rather than pretending it is a person.
  const secondResult = await (
    await fetch(`http://127.0.0.1:${GATEWAY_PORT}/session/result`, { headers: secondAuth })
  ).json();
  const denial = JSON.parse(JSON.stringify(secondResult));
  denial.outcome = "not-passed";
  denial.score = { assigned: 2, answered: 2, correct: 0, passThreshold: 2 };
  delete denial.signature;
  const forgedDenial = { ...denial, signature: signResult(applicationKey.privateKey, canonicalJson(denial)) };
  const reEnrolled = await claimAccess(forgedDenial, closedPolicy.contentReveal);
  check(
    "the utility grants on its own arithmetic even when the provider's signed verdict denies the pass",
    reEnrolled.granted === true && reEnrolled.scoring?.by === "utility" && reEnrolled.scoring?.correct === 2,
    `${reEnrolled.granted} ${JSON.stringify(reEnrolled.scoring ?? reEnrolled.reason ?? "")}`,
  );
  check("a re-enrolled participant is a new scope, and the utility says so", reEnrolled.granted === true && reEnrolled.scope?.kind === "provider-participant");

  // And the first participant is out of quota.
  const exhausted = await claimAccess(result, closedPolicy.contentReveal);
  check("the same participant id cannot spend its quota twice", exhausted.reason === "quota_exhausted");
}

try {
  await main();
} finally {
  for (const child of children) {
    child.kill();
  }
}

console.log(failures.length === 0 ? "ALL CHECKS PASSED" : `FAILURES: ${failures.join(", ")}`);
process.exit(failures.length === 0 ? 0 : 1);
