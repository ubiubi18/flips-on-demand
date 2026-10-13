// Drives the built HTML through a real browser from a file:// URL.
//
// Why not the playwright CLI: it refuses file: URLs, and "opens by
// double-clicking the file" is exactly the property that has to be checked.
// This is a scripted browser run, not a test suite; it needs the `playwright`
// package (`npm i -D playwright`) and uses the installed Chrome, so no browser
// download is implied.
//
// Run: node verify.mjs    (from app/, after npm run build)

import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

let chromium;
try {
  ({ chromium } = await import("playwright"));
} catch {
  console.error("this check needs the playwright package: npm i -D playwright");
  process.exit(2);
}

const here = dirname(fileURLToPath(import.meta.url));
const app = join(here, "dist", "flips-on-demand.html");
const stateFixture = join(here, "example-pool-state.json");
const state = join(await mkdtemp(join(tmpdir(), "fod-market-state-")), "state.json");
const freshState = JSON.parse(await readFile(stateFixture, "utf8"));
freshState.observedAt = new Date().toISOString();
for (const listing of freshState.pool ?? []) {
  if (listing.availability) listing.availability.checkedAt = freshState.observedAt;
}
await writeFile(state, JSON.stringify(freshState));
const connection = join(await mkdtemp(join(tmpdir(), "fod-provider-pin-")), "connection.json");
const shots = join(here, "..", "output", "playwright");
const gatewayDir = join(here, "..", "gateway");
const gatewayRequire = createRequire(join(gatewayDir, "package.json"));
const { privateKeyToAccount } = gatewayRequire("viem/accounts");
// The public Anvil test key: a fixed test wallet, not a secret.
const TEST_WALLET_KEY = "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d";
const testWallet = privateKeyToAccount(TEST_WALLET_KEY);

const failures = [];
const ownedChildren = [];
process.once("exit", () => { for (const child of ownedChildren) child.kill(); });
const check = (name, ok, detail = "") => {
  console.log(`${ok ? "ok  " : "FAIL"} ${name}${detail ? " - " + detail : ""}`);
  if (!ok) failures.push(name);
};

await mkdir(shots, { recursive: true });

// The provider half runs for real: the gateway the connection file names is
// started here, so the session checks below talk to it over HTTP. The walk
// gives the session a short window of its own: answers go in while it is open
// and the signed result is collected only after it closes (`AH-1`), which the
// long-lived example pool could not exercise. The window has to outlast the
// checks that run before the session view imports the connection plus the
// answers themselves; 3.5 s proved too tight under load (`AH-22`).
const examplePool = JSON.parse(await readFile(join(gatewayDir, "example-pool.json"), "utf8"));
const exampleConfig = JSON.parse(await readFile(join(gatewayDir, "example-config.json"), "utf8"));
const gatewayRunDir = await mkdtemp(join(tmpdir(), "fod-app-gateway-"));
const walkPoolPath = join(gatewayRunDir, "pool.json");
const walkConfigPath = join(gatewayRunDir, "config.json");
// One window for both halves of the walk: the browser answers and commits each
// answer to the utility while it is open, and still waits for the close before
// it collects a result.
const walkWindow = {
  opensAt: new Date(Date.now() - 60_000).toISOString(),
  closesAt: new Date(Date.now() + 30_000).toISOString(),
};
// The wrong-key half is a *different* session, so it gets its own id and its
// own window; the utility pins both because both commit answers to it.
const secondWindow = {
  opensAt: walkWindow.opensAt,
  closesAt: new Date(Date.now() + 60_000).toISOString(),
};
examplePool.rules.opensAt = walkWindow.opensAt;
examplePool.rules.closesAt = walkWindow.closesAt;
await writeFile(walkPoolPath, JSON.stringify(examplePool));
await writeFile(
  walkConfigPath,
  JSON.stringify({
    ...exampleConfig,
    poolPath: walkPoolPath,
    applicationKeyPath: join(gatewayRunDir, "provider-application-key.json"),
    trustedProxies: ["127.0.0.1"],
  }),
);
const gateway = spawn("node", ["src/main.ts", walkConfigPath], {
  cwd: gatewayDir,
  stdio: ["ignore", "pipe", "pipe"],
});
ownedChildren.push(gateway);
let gatewayOutput = "";
gateway.stdout.on("data", (chunk) => {
  gatewayOutput += String(chunk);
});
gateway.stderr.on("data", (chunk) => {
  gatewayOutput += String(chunk);
});
const gatewayReady = await waitForGateway("http://127.0.0.1:8787/session/policy", gateway);
check("the example gateway starts", gatewayReady, gatewayOutput.trim().slice(0, 80));
if (!gatewayReady) process.exit(1);

// The testbed utility runs for real too: it pins the gateway key and the
// session content digest, then decides on the result the browser claims.
const gatewayKey = await (await fetch("http://127.0.0.1:8787/provider/key")).json();
await writeFile(connection, JSON.stringify({ ...JSON.parse(await readFile(join(here, "example-provider.json"), "utf8")), publicKeyHex: gatewayKey.publicKeyHex }));
const gatewayPolicy = await (await fetch("http://127.0.0.1:8787/session/policy")).json();
const utilityDir = await mkdtemp(join(tmpdir(), "fod-app-utility-"));
const utilityPolicyPath = join(utilityDir, "policy.json");
await writeFile(
  utilityPolicyPath,
  JSON.stringify({
    format: "fod.utility-policy",
    version: 1,
    utilityId: "utility-example",
    sessions: ["session-example-1", "session-wrongkey-1"],
    origins: ["null"],
    sessionDigests: {
      "session-example-1": gatewayPolicy.contentDigest,
      // The wrong-key half serves the same flip pool from a second gateway.
      "session-wrongkey-1": gatewayPolicy.contentDigest,
    },
    requireOutcome: "passed",
    // The testbed utility recomputes the score itself, so the walk proves the
    // browser sends the reveal and the utility refuses without it.
    requireScoreProof: true,
    // And it goes one step further: it scores the session from the answers the
    // browser committed while the window was open, so the walk proves the
    // commitment path (`SPEC.md` 8.3).
    requireCommittedAnswers: true,
    sessionWindows: {
      "session-example-1": { opensAt: gatewayPolicy.rules.opensAt, closesAt: gatewayPolicy.rules.closesAt },
      "session-wrongkey-1": secondWindow,
    },
    grantsPerParticipant: 1,
    grantTtlSeconds: 3600,
    providers: [
      {
        providerId: gatewayKey.providerId,
        publicKeyHex: gatewayKey.publicKeyHex,
        gatewayUrl: "http://127.0.0.1:8787",
      },
    ],
    scope: {
      kind: "provider-participant",
      statement: "testbed grant to the provider-issued participant id",
    },
  }),
);
const utility = spawn("node", ["src/main.ts", utilityPolicyPath, "8788"], {
  cwd: join(here, "..", "utility"),
  stdio: ["ignore", "pipe", "pipe"],
});
ownedChildren.push(utility);
let utilityOutput = "";
utility.stdout.on("data", (chunk) => {
  utilityOutput += String(chunk);
});
utility.stderr.on("data", (chunk) => {
  utilityOutput += String(chunk);
});
const utilityReady = await waitForGateway("http://127.0.0.1:8788/policy", utility);
check("the test utility starts", utilityReady, utilityOutput.trim().slice(0, 80));
if (!utilityReady) process.exit(1);

const browser = await chromium.launch({ channel: "chrome" });
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });

// A minimal EIP-1193 wallet for the browser walk: it exposes the test address
// and signs the enrollment challenge with a public test key.
await page.exposeFunction("__fodWalletSign", (message) => testWallet.signMessage({ message }));
await page.addInitScript((address) => {
  window.ethereum = {
    request: async ({ method, params }) => {
      if (method === "eth_requestAccounts") {
        return [address];
      }
      if (method === "personal_sign") {
        return await window.__fodWalletSign(params[0]);
      }
      throw new Error(`unsupported wallet method ${method}`);
    },
  };
}, testWallet.address);

const consoleErrors = [];
const remoteRequests = [];
page.on("console", (message) => {
  if (message.type() === "error") {
    // Keep the URL: "403 (Forbidden)" alone does not say which call failed.
    const location = message.location();
    const where = typeof location?.url === "string" && location.url !== "" ? ` at ${location.url}` : "";
    consoleErrors.push(`${message.text()}${where}`);
  }
});
page.on("pageerror", (error) => consoleErrors.push(String(error)));
page.on("request", (request) => {
  if (!request.url().startsWith("file:")) remoteRequests.push(request.url());
});

await page.goto(`file://${app}`);
check("opens from file://", page.url().startsWith("file:"), page.url());
check("market is the default view", await page.getByRole("heading", { name: "Nothing loaded yet" }).isVisible());

// A file that only looks like a snapshot must be refused, not partially rendered.
const badPath = join(tmpdir(), "fod-invalid-state.json");
await writeFile(
  badPath,
  JSON.stringify({ format: "fod.marketplace-state", version: 2, network: "n", contract: "x", blockHeight: 1, observedAt: "x", source: "x", credits: [] }),
);
await page.getByLabel("Import state").setInputFiles(badPath);
check("a wrong state version is refused", (await page.locator(".banner").innerText()).includes("unsupported state version"));
check("a refused file renders nothing", (await page.getByRole("heading", { name: "Nothing loaded yet" }).count()) === 1);

await page.getByLabel("Import state").setInputFiles(stateFixture);
check("an old snapshot is visible but cannot prepare a buy", (await page.getByRole("button", { name: /^Save buy request for bundle/ }).isDisabled()) &&
  (await page.getByRole("status").allInnerTexts()).some((message) => message.includes("older than 15 minutes")));

await page.getByLabel("Import state").setInputFiles(state);
await page.waitForSelector(".provenance");
const provenance = await page.locator(".provenance").innerText();
check(
  "provenance shows the snapshot source",
  provenance.includes("idena-local") && provenance.includes("841"),
  provenance.replace(/\s+/g, " ").slice(0, 90),
);

// Markup arriving in a public field is text, never part of the document.
const markupPath = join(tmpdir(), "fod-markup-state.json");
const withMarkup = JSON.parse(await (await import("node:fs/promises")).readFile(state, "utf8"));
withMarkup.source = '<img src=x onerror="window.__pwned = true">';
await writeFile(markupPath, JSON.stringify(withMarkup));
await page.getByLabel("Import state").setInputFiles(markupPath);
await page.waitForSelector(".provenance");
check("markup in an imported field stays text", (await page.locator("img").count()) === 0);
check("markup did not execute", (await page.evaluate(() => window.__pwned === undefined)) === true);

await page.getByLabel("Import state").setInputFiles(state);
await page.waitForTimeout(50);

// The pool market: a snapshot with the pool section renders the live quote and
// turns the buy button into the request the CLI rechecks against the chain.
await page.getByRole("button", { name: /^Save buy request for bundle/ }).waitFor({ timeout: 5000 });
const poolRow = page.locator("tbody tr").first();
const poolRowText = (await poolRow.innerText()).replace(/\s+/g, " ");
const poolEpoch = (await poolRow.locator("td").nth(2).innerText()).replace(/\s+/g, " ");
const poolFlips = (await poolRow.locator("td").nth(3).innerText()).replace(/\s+/g, " ");
check(
  "the pool market renders the listing, its quote, and the live desk",
  (await page.locator("tbody tr").count()) === 1 &&
    poolRowText.includes("25000") &&
    poolEpoch.includes("204") &&
    poolFlips.includes("6") &&
    poolRowText.includes("live") &&
    poolRowText.includes("available") &&
    poolRowText.includes("6/6 flips checked"),
  poolRowText,
);
const [poolBuyDownload] = await Promise.all([
  page.waitForEvent("download"),
  page.getByRole("button", { name: /^Save buy request for bundle/ }).click(),
]);
const poolBuyRequest = JSON.parse(
  await (await import("node:fs/promises")).readFile(await poolBuyDownload.path(), "utf8"),
);
check(
  "the pool buy request names the bundle, the quote, and the buyer",
  poolBuyRequest.operation === "buy" &&
    poolBuyRequest.arguments.bundleId ===
      "49d4c4921367105c023c4eaf21329e28925793e6950fee60fe106790547a2fa9" &&
    poolBuyRequest.arguments.expectedQuote === "25000000000000000000000" &&
    poolBuyRequest.arguments.buyer === "3333333333333333333333333333333333333333" &&
    poolBuyRequest.amount === "25000000000000000000000",
  JSON.stringify(poolBuyRequest.arguments),
);
const poolBuyNotice = await page.locator(".notice").innerText();
check(
  "the pool buyer is told what to do with the file",
  poolBuyNotice.includes("fod-action-buy-") && poolBuyNotice.includes("fod prepare-buy -request"),
  poolBuyNotice.replace(/\s+/g, " "),
);
await page.screenshot({ path: `${shots}/pool-market.png`, fullPage: true });

await page.getByLabel("Import state").setInputFiles(badPath);
check("a failed replacement import clears the old buy action", (await page.getByRole("heading", { name: "Nothing loaded yet" }).count()) === 1 &&
  (await page.getByRole("button", { name: /^Save buy request for bundle/ }).count()) === 0);
await page.getByLabel("Import state").setInputFiles(state);

// The pool trades: the viewer's orders and listings produce the confirm,
// refund, sell, give, and withdraw requests the CLI consumes.
await page.getByRole("link", { name: "My trades" }).click();
await page.waitForURL(/#\/trades/);
await page.getByRole("heading", { name: "My trades" }).waitFor();
await page.getByLabel("Confirm order 1").waitFor({ timeout: 5000 });
const poolTradesText = (await page.locator("main section").first().innerText()).replace(/\s+/g, " ");
check(
  "the pool trades list the delivered order and the refundable one",
  poolTradesText.includes("#1") && poolTradesText.includes("#2") && poolTradesText.includes("refund available"),
  poolTradesText.slice(0, 240),
);
check(
  "the delivered order shows its bought keys as ready",
  poolTradesText.includes("keys ready"),
  poolTradesText.slice(0, 240),
);
check(
  "a keys-ready order links to the flips journey",
  (await page.getByRole("link", { name: "Use these flips" }).count()) >= 1,
);
const poolBundle = "49d4c4921367105c023c4eaf21329e28925793e6950fee60fe106790547a2fa9";
// The confirmation names the ciphertext the buyer's own run checked, so the
// walk hands the view the record `fod receive -out` writes.
const openedRecord = join(tmpdir(), `fod-opened-${process.pid}.json`);
const walkedCiphertext = "71".repeat(8);
const walkedDigest = createHash("sha256").update(Buffer.from(walkedCiphertext, "hex")).digest("hex");
await writeFile(
  openedRecord,
  JSON.stringify({
    format: "fod.opened-pool-delivery",
    version: 1,
    contract: "1111111111111111111111111111111111111111",
    orderId: 1,
    ciphertextSha256: walkedDigest,
    ciphertextHex: walkedCiphertext,
  }),
);
const [confirmDownload] = await Promise.all([
  page.waitForEvent("download"),
  page.getByLabel("Confirm order 1").setInputFiles(openedRecord),
]);
const confirmRequest = JSON.parse(
  await (await import("node:fs/promises")).readFile(await confirmDownload.path(), "utf8"),
);
check(
  "the confirm request names the bundle, the order and the checked ciphertext",
  confirmRequest.operation === "confirm" &&
    confirmRequest.arguments.bundleId === poolBundle &&
    confirmRequest.arguments.orderId === 1 &&
    confirmRequest.arguments.ciphertextSha256 === walkedDigest &&
    confirmRequest.arguments.ciphertextHex === walkedCiphertext,
  JSON.stringify(confirmRequest.arguments),
);
check(
  "the confirm page says what to do with the file",
  (await page.locator(".notice").innerText()).includes("fod prepare-confirm"),
);
const [refundDownload] = await Promise.all([
  page.waitForEvent("download"),
  page.getByRole("button", { name: "Refund" }).click(),
]);
const refundRequest = JSON.parse(
  await (await import("node:fs/promises")).readFile(await refundDownload.path(), "utf8"),
);
check(
  "the refund request names the operation and the order",
  refundRequest.operation === "refund" && refundRequest.arguments.orderId === 2,
  JSON.stringify(refundRequest.arguments),
);
check(
  "the refund page says what to do with the file",
  (await page.locator(".notice").innerText()).includes("fod refund"),
);
check(
  "the author view offers only the donation path, not the refused sell path",
  (await page.getByRole("button", { name: "Sell one unit" }).count()) === 0,
);
const [giveDownload] = await Promise.all([
  page.waitForEvent("download"),
  page.getByRole("button", { name: "Give one unit" }).click(),
]);
const giveRequest = JSON.parse(
  await (await import("node:fs/promises")).readFile(await giveDownload.path(), "utf8"),
);
check(
  "the give request names the pool commitment",
  giveRequest.operation === "donateUnit" && giveRequest.arguments.commitment === poolBundle,
  JSON.stringify(giveRequest.arguments),
);
check(
  "the serve page says what to do with the file",
  (await page.locator(".notice").innerText()).includes("fod prepare-list -request"),
);
const [withdrawDownload] = await Promise.all([
  page.waitForEvent("download"),
  page.getByRole("button", { name: "Withdraw" }).click(),
]);
const withdrawRequest = JSON.parse(
  await (await import("node:fs/promises")).readFile(await withdrawDownload.path(), "utf8"),
);
check(
  "the withdrawal request names the operation and no arguments",
  withdrawRequest.operation === "withdraw" &&
    Object.keys(withdrawRequest.arguments).length === 0 &&
    withdrawRequest.amount === "0",
  JSON.stringify(withdrawRequest),
);
check(
  "the withdrawal page says what to do with the file",
  (await page.locator(".notice").innerText()).includes("fod withdraw"),
);
await page.screenshot({ path: `${shots}/pool-trades.png`, fullPage: true });

// A phone viewport: the same snapshot has to render without the document
// overflowing, with the buy and trade actions reachable.
const mobile = await browser.newPage({ viewport: { width: 390, height: 844 } });
const mobileErrors = [];
mobile.on("pageerror", (error) => mobileErrors.push(String(error)));
await mobile.goto(`file://${app}`);
await mobile.getByLabel("Import state").setInputFiles(state);
await mobile.getByRole("button", { name: /^Save buy request for bundle/ }).waitFor({ timeout: 5000 });
const marketOverflow = await mobile.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
check("the pool market fits a phone width", marketOverflow <= 1, `overflow=${marketOverflow}`);
const marketRowOverflow = await mobile.locator(".table-wrap").first().evaluate((element) => element.scrollWidth - element.clientWidth);
check("purchase terms fit inside the phone listing", marketRowOverflow <= 1, `overflow=${marketRowOverflow}`);
await mobile.screenshot({ path: `${shots}/pool-market-mobile.png`, fullPage: true });
await mobile.getByRole("link", { name: "My trades" }).click();
await mobile.waitForURL(/#\/trades/);
await mobile.getByRole("heading", { name: "My trades" }).waitFor();
await mobile.getByRole("button", { name: "Confirm" }).waitFor({ timeout: 5000 });
const tradesOverflow = await mobile.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
check("the pool trades fit a phone width", tradesOverflow <= 1, `overflow=${tradesOverflow}`);
check("no page errors in the phone view", mobileErrors.length === 0, mobileErrors.join(" | ").slice(0, 120));
await mobile.screenshot({ path: `${shots}/pool-trades-mobile.png`, fullPage: true });
await mobile.close();

await page.getByLabel("Import state").setInputFiles(state);
await page.waitForTimeout(50);

await page.getByRole("link", { name: "FAQ" }).click();
await page.waitForURL(/#\/faq/);
await page.getByRole("heading", { name: "FAQ" }).waitFor();
const faqText = (await page.locator("section.faq").innerText()).toLowerCase();
check("faq renders offline content", faqText.includes("what this is") && faqText.includes("security limits"));
await page.screenshot({ path: `${shots}/faq.png`, fullPage: true });

// The customer half of the provider flow: connection file, enrolment,
// challenges, answers, and a result the browser verifies itself.
await page.getByRole("link", { name: "Use flips" }).click();
await page.waitForURL(/#\/session/);
await page.getByRole("heading", { name: "Use flips" }).waitFor();
check(
  "the example gate is offered without a connection file",
  (await page.getByRole("button", { name: "Try the example gate" }).count()) === 1,
);
await page.getByText("Set up a gate for your use case").click();
const setupText = (await page.locator("details.gate-setup").innerText()).replace(/\s+/g, " ");
check(
  "the gate setup journey names the faucet and dapp recipes and the pool command",
  setupText.includes("Testnet faucet") &&
    setupText.includes("Dapp access") &&
    setupText.includes("fod session-pool"),
  setupText.slice(0, 240),
);
// A generated connection can carry the key the operator pasted, so a later
// substitution by the host shows "not verified" (`AH-8`).
const liveKey = (await (await fetch("http://127.0.0.1:8787/provider/key")).json()).publicKeyHex;
await page.getByLabel("Gateway key").fill(liveKey);
const [connectionDownload] = await Promise.all([
  page.waitForEvent("download"),
  page.getByRole("button", { name: "Download connection file" }).click(),
]);
const generatedConnection = JSON.parse(await readFile(await connectionDownload.path(), "utf8"));
check(
  "the generated connection pins the gateway key the operator pasted",
  generatedConnection.publicKeyHex === liveKey.toLowerCase(),
  generatedConnection.publicKeyHex ?? "none",
);
check(
  "the generated connection's note names the gateway and the utility host",
  String(generatedConnection.note ?? "").includes(generatedConnection.gatewayUrl) &&
    String(generatedConnection.note ?? "").includes(generatedConnection.utilityUrl),
  generatedConnection.note ?? "none",
);
await page.getByText("Sell through a shared node").click();
const sharedNodePath = join(tmpdir(), "fod-shared-node.json");
await writeFile(
  sharedNodePath,
  JSON.stringify({
    format: "fod.shared-node",
    version: 1,
    nodeId: "example-provider",
    url: "http://127.0.0.1:8787",
    author: testWallet.address,
    custody: "hosted",
    note: "app walk",
  }),
);
// A malformed optional node must be rejected without crashing the interface.
await page.route("**/shared-node/status", (route) => route.fulfill({
  status: 200,
  contentType: "application/json",
  body: JSON.stringify({ format: "fod.shared-node-status", version: 1, nodeId: "example-provider", operations: null }),
}));
await page.getByLabel("Import shared node").setInputFiles(sharedNodePath);
await page.locator("details.shared-node").getByText("this node does not answer with the shared-node status this build reads").waitFor({ timeout: 5000 });
check("malformed shared-node status is refused without crashing", await page.getByLabel("Import shared node").isVisible());
await page.unroute("**/shared-node/status");
await page.getByLabel("Import shared node").setInputFiles(sharedNodePath);
await page.locator("details.shared-node").getByText("session + pool").waitFor({ timeout: 5000 });
const sharedText = await page.locator("details.shared-node").innerText();
check(
  "the shared-node journey states the node status and the custody boundary",
  sharedText.includes("example-provider") && sharedText.includes("hosted") && sharedText.includes("self"),
);
check(
  "the shared-node journey names the node command that builds the author's export and pool",
  sharedText.includes("fod shared-node") && sharedText.includes("fod serve"),
  sharedText.replace(/\s+/g, " ").slice(0, 160),
);
await page.getByLabel("Shared node author").fill(testWallet.address);
const [sharedDownload] = await Promise.all([
  page.waitForEvent("download"),
  page.getByRole("button", { name: "Download shared-node file" }).click(),
]);
const sharedFile = JSON.parse(
  await (await import("node:fs/promises")).readFile(await sharedDownload.path(), "utf8"),
);
check(
  "the shared-node file names the node, author and custody",
  sharedFile.format === "fod.shared-node" &&
    sharedFile.author === testWallet.address.toLowerCase() &&
    sharedFile.custody === "hosted",
);
await page.getByRole("button", { name: "Try the example gate" }).click();
await page.getByText("session-example-1").waitFor({ timeout: 5000 });
check(
  "the example gate loads its policy without a connection file",
  (await page.locator("main section").innerText()).includes("example-provider"),
);
await page.reload();
await page.getByRole("heading", { name: "Use flips" }).waitFor();
await page.getByLabel("Import connection").setInputFiles(connection);
await page.getByText("session-example-1").first().waitFor();
await page.getByText("Session window").waitFor({ timeout: 5000 });
check(
  "the on-demand lobby shows the shared window and participants",
  (await page.locator("main section").innerText()).includes("Participants") &&
    (await page.locator("main section").innerText()).includes("Challenges"),
);
await page.getByRole("button", { name: "Connect EVM wallet" }).click();
await page.getByRole("button", { name: /Wallet/ }).waitFor();
const policyText = await page.locator("main section").innerText();
check("the connection file loads and the policy shows", policyText.includes("session-example-1") && policyText.includes("example-provider"));
check(
  "the policy says what the answers rest on",
  policyText.includes("Answers from") && policyText.includes("example answers"),
  policyText.replace(/\s+/g, " ").slice(0, 120)
);

await page.getByRole("button", { name: "Start session" }).click();
await page.getByRole("table").waitFor();
check("enrolment lists the challenges", (await page.locator("tbody tr").count()) === 2);

// c1 carries flip content, so it is answered by choosing one of the two
// sequences; c2 has no content and is answered in the text field.
const flipFill = await page.locator("tbody tr").nth(0).locator("img").evaluateAll((images) =>
  images.map((image) => ({ loaded: image.complete && image.naturalWidth > 0, src: image.currentSrc.slice(0, 21) }))
);
check(
  "the flip shows both sequences as eight loaded pictures",
  flipFill.length === 8 && flipFill.every((image) => image.loaded && image.src.startsWith("data:image/png")),
  flipFill.map((image) => `${image.src}:${image.loaded}`).join(" ")
);
await page.getByLabel("Answer for c2").fill("second answer");
await page.locator("tbody tr").nth(0).getByRole("button", { name: "Right" }).click();
await page.getByText("submitted").first().waitFor();
await page.locator("tbody tr").nth(1).getByRole("button", { name: "Submit" }).click();
await page.waitForFunction(() => document.querySelectorAll(".chip-sold").length >= 2);
const answerChips = (await page.locator(".chip-sold").allInnerTexts()).join(" ").toLowerCase();
check(
  "an answer is accepted without a correctness signal while the session is open",
  answerChips.includes("submitted") && !answerChips.includes("correct") && !answerChips.includes("wrong"),
  answerChips,
);

// The utility scores the session itself, so the answers have to reach it while
// the window is open; the browser sends the whole set again with every answer.
let committedToUtility = false;
for (let attempt = 0; attempt < 25 && !committedToUtility; attempt += 1) {
  committedToUtility = (await page.getByText("answers committed to the utility").count()) > 0;
  if (!committedToUtility) {
    await page.waitForTimeout(200);
  }
}
check(
  "the browser committed its answers to the utility while the window was open",
  committedToUtility,
  committedToUtility ? "" : (await page.locator("p.muted").allInnerTexts()).join(" | ").slice(0, 300),
);

// The signed result exists only after the session closes (`AH-1`), so wait for
// the window this walk's own pool published.
const walkPolicy = await (await fetch("http://127.0.0.1:8787/session/policy")).json();
await page.waitForTimeout(Math.max(0, Date.parse(walkPolicy.rules.closesAt) - Date.now() + 700));
await page.getByRole("button", { name: "Get result" }).click();
await page.getByRole("heading", { name: "Provider result" }).waitFor();
const resultText = await page.locator("div.result").innerText();
check("the provider result is signed and verified in the browser", resultText.includes("verified in browser"), resultText.replace(/\s+/g, " ").slice(0, 90));
check("the result reports the score and outcome", resultText.includes("passed") && resultText.includes("2 of 2"));
// The browser does not take the provider's word for the score: it recomputes it
// from its own answers and the answer key the closed policy reveals, and only a
// recomputation that reproduces the signed score is claimable.
check(
  "the browser recomputed the score from the answers it submitted",
  resultText.includes("recomputed from your answers"),
  resultText.replace(/\s+/g, " ").slice(0, 120),
);
check(
  "the signed result names the bound EVM address",
  resultText.toLowerCase().includes(testWallet.address.toLowerCase()),
);
await page.getByRole("button", { name: "Claim test access" }).click();
await page.getByText("testbed grant to the provider-issued participant id").waitFor();
check(
  "the test utility grants access for the passing result",
  (await page.locator("main section").innerText()).includes("granted"),
);
check(
  "the utility grant names the bound EVM address",
  (await page.locator("main section").innerText()).toLowerCase().includes(testWallet.address.toLowerCase()),
);
await page.screenshot({ path: `${shots}/session.png`, fullPage: true });

// The page loads nothing remote on its own. The only outside requests allowed
// are the ones the session makes to the gateway the connection file names.
const allowedOrigins = ["http://127.0.0.1:8787/", "http://127.0.0.1:8788/", "http://127.0.0.1:8790/"];
const unexpectedRequests = remoteRequests.filter((url) => !allowedOrigins.some((origin) => url.startsWith(origin)));
check("no remote requests beyond the configured gateway and test utility", unexpectedRequests.length === 0, unexpectedRequests.join(" | ").slice(0, 120));
check("the session did reach the gateway", remoteRequests.some((url) => url.includes("/session/enroll")));

// A connection file that recorded another key cannot turn a result into a
// verified one: the browser checks against the key the file named, not against
// whatever key the host serves now (`SPEC.md` 8.2).
//
// The first session is closed by now, so this half runs against a second
// gateway with its own short window on port 8790.
const secondRunDir = await mkdtemp(join(tmpdir(), "fod-app-gateway-2-"));
const secondPool = JSON.parse(await readFile(join(gatewayDir, "example-pool.json"), "utf8"));
const secondPoolPath = join(secondRunDir, "pool.json");
const secondConfigPath = join(secondRunDir, "config.json");
secondPool.sessionId = "session-wrongkey-1";
// The same window as the first half: one utility pins one window for this
// session, and its commitments have to fall inside it.
secondPool.rules.opensAt = secondWindow.opensAt;
secondPool.rules.closesAt = secondWindow.closesAt;
await writeFile(secondPoolPath, JSON.stringify(secondPool));
await writeFile(
  secondConfigPath,
  JSON.stringify({
    ...exampleConfig,
    port: 8790,
    publicOrigin: "http://127.0.0.1:8790",
    poolPath: secondPoolPath,
    applicationKeyPath: join(secondRunDir, "provider-application-key.json"),
    trustedProxies: ["127.0.0.1"],
  }),
);
const secondGateway = spawn("node", ["src/main.ts", secondConfigPath], {
  cwd: gatewayDir,
  stdio: ["ignore", "pipe", "pipe"],
});
ownedChildren.push(secondGateway);
let secondGatewayOutput = "";
secondGateway.stdout.on("data", (chunk) => {
  secondGatewayOutput += String(chunk);
});
secondGateway.stderr.on("data", (chunk) => {
  secondGatewayOutput += String(chunk);
});
const secondGatewayReady = await waitForGateway("http://127.0.0.1:8790/session/policy", secondGateway);
check(
  "the second gateway starts",
  secondGatewayReady,
  secondGatewayOutput.trim().slice(0, 80),
);
if (!secondGatewayReady) { await browser.close(); process.exit(1); }

const wrongKeyConfig = JSON.parse(await readFile(connection, "utf8"));
wrongKeyConfig.publicKeyHex = "ab".repeat(44);
wrongKeyConfig.gatewayUrl = "http://127.0.0.1:8790";
const wrongKeyPath = join(tmpdir(), `fod-provider-wrongkey-${process.pid}.json`);
await writeFile(wrongKeyPath, JSON.stringify(wrongKeyConfig));
await page.reload();
await page.getByLabel("Import connection").setInputFiles(wrongKeyPath);
await page.getByText("session-wrongkey-1").first().waitFor();
await page.getByRole("button", { name: "Connect EVM wallet" }).click();
await page.getByRole("button", { name: /Wallet/ }).waitFor();
await page.getByRole("button", { name: "Start session" }).click();
await page.getByRole("table").waitFor();
await page.getByLabel("Answer for c2").fill("second answer");
await page.locator("tbody tr").nth(0).getByRole("button", { name: "Right" }).click();
await page.locator("tbody tr").nth(1).getByRole("button", { name: "Submit" }).click();
await page.waitForFunction(() => document.querySelectorAll(".chip-sold").length >= 2);
const secondPolicy = await (await fetch("http://127.0.0.1:8790/session/policy")).json();
await page.waitForTimeout(Math.max(0, Date.parse(secondPolicy.rules.closesAt) - Date.now() + 700));
await page.getByRole("button", { name: "Get result" }).click();
await page.getByRole("heading", { name: "Provider result" }).waitFor();
const wrongKeyText = await page.locator("div.result").innerText();
check(
  "a connection file that pinned another key cannot show a verified result",
  wrongKeyText.includes("not verified"),
  wrongKeyText.replace(/\s+/g, " ").slice(0, 120),
);
check(
  "an unverified result cannot claim the test utility",
  await page.getByRole("button", { name: "Claim test access" }).isDisabled(),
);

// `SPEC.md` 11.1's privacy row: nothing the page handled may be left in browser
// storage. The flows above went through an imported snapshot, the trade and buy
// requests, a session with the gateway, and a signed result, so this runs at the
// point where a leak would be most likely.
const storage = await page.evaluate(() => ({
  local: window.localStorage.length,
  session: window.sessionStorage.length,
  cookie: document.cookie,
}));
check(
  "the page keeps nothing in browser storage",
  storage.local === 0 && storage.session === 0 && storage.cookie === "",
  `localStorage ${storage.local}, sessionStorage ${storage.session}, cookie ${JSON.stringify(storage.cookie)}`,
);

// `SPEC.md` 11.1's recovery row, both halves. A reload is the restart: the page
// keeps no connection, so it asks for one again, which is what makes importing a
// second file possible. That second file names a gateway nothing listens on, and
// the failure has to surface as a message rather than as a broken page.
await page.reload();
await page.getByRole("heading", { name: "Use flips" }).waitFor();
check(
  "a reload forgets the connection it had",
  (await page.getByLabel("Import connection").count()) === 1,
);
const alert = page.locator('[role="alert"]');

// A provider that publishes a policy this build does not know has to be refused
// by name, not rendered from fields the client only guesses at (`SPEC.md` 4.5).
const { createServer } = await import("node:http");
const futurePolicy = createServer((request, response) => {
  if (request.url?.startsWith("/session/policy")) {
    // A real provider configures CORS for the deployed origin; this stub serves
    // one unauthenticated policy to a file:// page, so it allows the null origin.
    response.writeHead(200, {
      "content-type": "application/json",
      "access-control-allow-origin": "*",
    });
    response.end(
      JSON.stringify({
        format: "fod.session-policy",
        version: 2,
        providerId: "example-provider",
        sessionId: "session-from-the-future",
        utilityId: "utility-example",
        provenance: { epoch: 204, author: "44", source: "test", blockRange: "1-2" },
        rules: { passThreshold: 1, opensAt: "2026-01-01T00:00:00Z", closesAt: "2027-01-01T00:00:00Z" },
      }),
    );
    return;
  }
  response.writeHead(404);
  response.end();
});
await new Promise((resolve) => futurePolicy.listen(0, "127.0.0.1", resolve));
const futurePort = futurePolicy.address().port;
const futurePath = join(tmpdir(), "fod-future-policy.json");
const futureConfig = JSON.parse(await (await import("node:fs/promises")).readFile(connection, "utf8"));
futureConfig.gatewayUrl = `http://127.0.0.1:${futurePort}`;
await writeFile(futurePath, JSON.stringify(futureConfig));
await page.getByLabel("Import connection").setInputFiles(futurePath);
await alert.first().waitFor({ timeout: 5000 }).catch(() => {});
check(
  "a policy from a future version is refused by name",
  (await alert.first().innerText()).includes("not a session policy this build reads"),
  (await page.locator("main section").innerText()).replace(/\s+/g, " ").slice(0, 140),
);
futurePolicy.close();
await page.reload();
await page.getByRole("heading", { name: "Use flips" }).waitFor();

const deadGatewayPath = join(tmpdir(), "fod-dead-gateway.json");
const deadConfig = JSON.parse(await (await import("node:fs/promises")).readFile(connection, "utf8"));
deadConfig.gatewayUrl = "http://127.0.0.1:8799";
await writeFile(deadGatewayPath, JSON.stringify(deadConfig));
await page.getByLabel("Import connection").setInputFiles(deadGatewayPath);
await alert.first().waitFor({ timeout: 5000 }).catch(() => {});
check(
  "an unreachable gateway is reported, not swallowed",
  (await alert.count()) > 0,
  (await page.locator("main section").innerText()).replace(/\s+/g, " ").slice(0, 120),
);
check("the page survives it", (await page.getByRole("heading", { name: "Use flips" }).count()) === 1);

// The refused connection is expected to leave one browser-generated console error;
// nothing else in the whole run may.
const unexpectedConsoleErrors = consoleErrors.filter((line) => !line.includes("ERR_CONNECTION_REFUSED"));
check("no console errors beyond the refused connection", unexpectedConsoleErrors.length === 0, unexpectedConsoleErrors.join(" | ").slice(0, 120));

await browser.close();
utility.kill();
gateway.kill();
secondGateway.kill();
console.log(failures.length === 0 ? "ALL CHECKS PASSED" : `FAILURES: ${failures.join(", ")}`);
process.exit(failures.length === 0 ? 0 : 1);

async function waitForGateway(url, child) {
  let listening = false;
  let output = "";
  child.stdout.on("data", (chunk) => {
    output += String(chunk);
    listening ||= output.includes(`listening on ${new URL(url).host}`);
  });
  for (let attempt = 0; attempt < 40; attempt += 1) {
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
