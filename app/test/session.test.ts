import { test } from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";
import { chromium } from "playwright";
import { fileURLToPath } from "node:url";

test("concurrent answers retain the complete set and serialize commitments without stale status", async () => {
  const bundle = await build({
    absWorkingDir: fileURLToPath(new URL("..", import.meta.url)),
    stdin: { contents: 'import React from "react"; import { createRoot } from "react-dom/client"; import { Session } from "./src/views/Session"; createRoot(document.getElementById("root")).render(React.createElement(Session));', resolveDir: fileURLToPath(new URL("..", import.meta.url)), loader: "tsx" },
    bundle: true, write: false, format: "iife", jsx: "automatic",
  });
  const browser = await chromium.launch({ headless: true, channel: "chrome" });
  try {
    const page = await browser.newPage();
    // Every network request is intercepted; no service or real wallet is used.
    await page.route("**/*", (route) => route.fulfill({ contentType: "text/html", body: '<div id="root"></div>' }));
    await page.goto("https://session-test.invalid");
    await page.evaluate(() => {
      const state = window as any;
      const address = `0x${"11".repeat(20)}`;
      const now = Date.now();
      const rules = { passThreshold: 3, opensAt: new Date(now - 1000).toISOString(), closesAt: new Date(now + 600_000).toISOString() };
      state.pendingAnswers = {};
      state.commitments = [];
      state.signatures = [];
      state.ethereum = { request: async ({ method, params }: any) => {
        if (method === "eth_requestAccounts") return [address];
        if (String(params[0]).startsWith("fod-answers:")) {
          state.signatures.push(params[0]);
          return new Promise((resolve) => { state.finishSignature = () => resolve(`0x${"ab".repeat(65)}`); });
        }
        return `0x${"ab".repeat(65)}`;
      } };
      state.fetch = async (url: string, options: any) => {
        const path = new URL(url).pathname;
        let body: unknown;
        switch (path) {
          case "/session/policy": body = { format: "fod.session-policy", version: 1, providerId: "example-provider", sessionId: "s", utilityId: "u", rules, provenance: { epoch: 1 }, contentDigest: "11".repeat(32) }; break;
          case "/session/lobby": body = { ...rules, window: "open", participants: 1, challenges: 3 }; break;
          case "/session/enroll-challenge": body = { challenge: `fod-enroll:v3:example-provider:s:${address}:${encodeURIComponent("http://127.0.0.1:8787")}:${"ab".repeat(16)}:${now}`, expiresAt: new Date(now + 300_000).toISOString() }; break;
          case "/session/enroll": body = { participantId: "p", token: "t", address, expiresAt: rules.closesAt }; break;
          case "/session/challenges": body = { challenges: ["c1", "c2", "c3"].map((challengeId) => ({ challengeId, cid: challengeId })) }; break;
          case "/session/answers": {
            const { challengeId } = JSON.parse(options.body);
            return new Promise((resolve) => { state.pendingAnswers[challengeId] = () => resolve(new Response('{"accepted":true}')); });
          }
          case "/answer-commitment": {
            state.commitments.push(JSON.parse(options.body));
            return new Promise((resolve) => { state.finishCommitment = (committed: boolean) => resolve(new Response(JSON.stringify({ committed }))); });
          }
          default: throw new Error(`unexpected test request: ${path}`);
        }
        return new Response(JSON.stringify(body));
      };
    });
    await page.addScriptTag({ content: bundle.outputFiles[0].text });
    await page.getByRole("button", { name: "Try the example gate" }).click();
    await page.getByRole("button", { name: "Connect EVM wallet" }).click();
    await page.getByRole("button", { name: "Start session", exact: true }).click();
    for (const id of ["c1", "c2", "c3"]) await page.getByRole("textbox", { name: `Answer for ${id}`, exact: true }).fill(id);
    const submit = (id: string) => page.locator("tr").filter({ has: page.getByRole("textbox", { name: `Answer for ${id}`, exact: true }) }).getByRole("button", { name: "Submit", exact: true });
    await submit("c1").click();
    await page.evaluate(() => (window as any).pendingAnswers.c1());
    await page.waitForFunction(() => (window as any).signatures.length === 1);
    // The first signature stays pending while two gateway answers resolve in
    // the same turn. Both must survive even though React has not rerendered.
    await submit("c2").click();
    await submit("c3").click();
    await page.evaluate(() => { (window as any).pendingAnswers.c3(); (window as any).pendingAnswers.c2(); });
    await page.waitForFunction(() => document.querySelectorAll(".chip-sold").length === 3);
    assert.equal(await page.evaluate(() => (window as any).signatures.length), 1);
    await page.evaluate(() => (window as any).finishSignature());
    await page.waitForFunction(() => (window as any).commitments.length === 1);
    await page.evaluate(() => (window as any).finishCommitment(false));
    await page.waitForFunction(() => (window as any).signatures.length === 2);
    assert.match(await page.locator("section").innerText(), /committing the answers to the utility/);
    assert.doesNotMatch(await page.locator("section").innerText(), /has not accepted the commitment/);
    await page.evaluate(() => (window as any).finishSignature());
    await page.waitForFunction(() => (window as any).commitments.length === 2);
    const sets = await page.evaluate(() => (window as any).commitments.map((proof: any) => proof.answers));
    assert.deepEqual(sets, [[{ challengeId: "c1", answer: "c1" }], [{ challengeId: "c1", answer: "c1" }, { challengeId: "c3", answer: "c3" }, { challengeId: "c2", answer: "c2" }]]);
    await page.evaluate(() => (window as any).finishCommitment(true));
    await page.getByText("answers committed to the utility while the window is open, so it can score the session itself", { exact: true }).waitFor();
  } finally {
    await browser.close();
  }
});
