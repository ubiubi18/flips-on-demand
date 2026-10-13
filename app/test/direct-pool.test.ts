import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { build } from "esbuild";
import { chromium } from "playwright";
import { fileURLToPath } from "node:url";
import { parseDirectPoolState } from "../src/direct-pool-state.ts";

const ids = ["a".repeat(64), "b".repeat(64), "c".repeat(64)];
const tinyJpeg = Buffer.from(
  "/9j/4AAQSkZJRgABAgAAAQABAAD//gAQTGF2YzYyLjExLjEwMAD/2wBDAAgEBAQEBAUFBQUFBQYGBgYGBgYGBgYHBwcICAgHBwcGBgcHCAgICAkJCQgICAgJCQoKCgwMCwsODg4RERT/xABLAAEBAAAAAAAAAAAAAAAAAAAABwEBAAAAAAAAAAAAAAAAAAAAABABAAAAAAAAAAAAAAAAAAAAABEBAAAAAAAAAAAAAAAAAAAAAP/AABEIAAIAAgMBIgACEQADEQD/2gAMAwEAAhEDEQA/AL+AD//Z",
  "base64",
).toString("hex");

function cid(seed: string, codec = 0x55): string {
  const bytes = Buffer.concat([Buffer.from([1, codec, 0x12, 0x20]), createHash("sha256").update(seed).digest()]);
  const alphabet = "abcdefghijklmnopqrstuvwxyz234567";
  let value = 0;
  let bits = 0;
  let encoded = "b";
  for (const byte of bytes) {
    value = (value << 8) | byte;
    bits += 8;
    while (bits >= 5) {
      bits -= 5;
      encoded += alphabet[(value >>> bits) & 31];
    }
    value &= (1 << bits) - 1;
  }
  if (bits > 0) encoded += alphabet[(value << (5 - bits)) & 31];
  return encoded;
}

function offer(offerId: string, originalCid: string) {
  return {
    offerId, author: "1".repeat(40), epoch: 230, coveredFlips: 1, originalCids: [originalCid],
    inventoryCommitment: "d".repeat(64), keysCommitment: "e".repeat(64),
    payout: "1000000000000000000", sellerBond: "500000000000000000", fraudCollateral: "0",
    nextNonce: "2", openOrderId: "0", state: "queued-primary", everSold: false,
    contentProven: false, publicSeed: false, queue: "primary", queuePosition: 1,
    sequence: "1", listedAtBlock: "5000000",
  };
}

function snapshot() {
  const first = offer(ids[0], cid("first"));
  const second = { ...offer(ids[1], cid("second", 0x70)), state: "queued-secondary", queue: "secondary", queuePosition: 2, sequence: "2" };
  const active = { ...offer(ids[2], cid("active")), state: "active", openOrderId: "1", sequence: "3" };
  delete (active as any).queue;
  delete (active as any).queuePosition;
  return {
    format: "fod.direct-pool-state", version: 6, scope: "live", network: "idena",
    contract: "f".repeat(40), codeHash: "9".repeat(64), blockHeight: 5_000_100,
    observedAt: new Date().toISOString().replace(/\.\d{3}Z$/, "Z"), source: "node",
    pool: {
      reserve: "10000000000000000000", committed: "3000000000000000000",
      buyerPayments: "2000000000000000000", insurance: "0", pending: "0",
      virtualReserve: "9000000000000000000", primaryUnits: 1, secondaryUnits: 1,
      poolUnits: 3, reservedUnits: 1, seedCount: 2, seedOfferIds: [ids[1], ids[2]],
      nextOrderId: "2", nextSequence: "4", nextOfferId: ids[0],
      buyQuote: "9000000000000000000", minimumBuyerBond: "500000000000000000",
      minimumBuyerTotal: "9500000000000000000", nextSellerPayout: "3000000000000000000",
      buyPausedForBond: false,
    },
    offers: [second, active, first],
    orders: [{
      orderId: "1", offerId: ids[2], buyer: "2".repeat(40), paid: "2000000000000000000",
      bond: "500000000000000000", sellerPayout: "1000000000000000000",
      fundedAtBlock: "5000000", refundAtBlock: "5012960", nonce: "1",
      state: "funded", challenged: true, refundAvailable: false,
    }],
  };
}

test("v6 live snapshot accepts public fields and exact CID links", () => {
  const input = snapshot();
  const result = parseDirectPoolState(JSON.stringify(input));
  assert.equal(result.ok, true);
  if (!result.ok) return;
  assert.equal(result.state.scope, "live");
  assert.equal(result.state.pool.buyQuote, "9000000000000000000");
  assert.deepEqual(result.state.offers.map((item) => item.originalCids[0]), input.offers.map((item) => item.originalCids[0]));
  assert.equal(result.state.orders[0].challenged, true);
});

test("v6 snapshot enforces the 1,000 iDNA ceiling and repeat-sale dividend", () => {
  const capped = snapshot();
  capped.pool.reserve = "10000000000000000000000";
  capped.pool.committed = "1000000000000000000";
  capped.pool.virtualReserve = "10001000000000000000000";
  capped.pool.buyQuote = "1000000000000000000000";
  capped.pool.minimumBuyerTotal = "1000500000000000000000";
  capped.pool.nextSellerPayout = "1000000000000000000000";
  assert.equal(parseDirectPoolState(JSON.stringify(capped)).ok, true);
  capped.pool.buyQuote = "1000000000000000000001";
  assert.equal(parseDirectPoolState(JSON.stringify(capped)).ok, false);

  const repeat = snapshot();
  repeat.offers[1].everSold = true;
  repeat.offers[1].payout = "0";
  repeat.pool.committed = "2000000000000000000";
  repeat.pool.buyerPayments = "1000000000000000000";
  assert.equal(parseDirectPoolState(JSON.stringify(repeat)).ok, true);
  repeat.orders[0].sellerPayout = "1000000000000000001";
  assert.equal(parseDirectPoolState(JSON.stringify(repeat)).ok, false);
});

test("v6 snapshot requires the next price to cover a locked primary payout", () => {
  const floored = snapshot();
  floored.offers[0].payout = "0";
  floored.offers[0].everSold = true;
  floored.offers[0].contentProven = true;
  floored.offers[2].payout = "10000000000000000000";
  floored.pool.reserve = "18000000000000000000";
  floored.pool.committed = "11000000000000000000";
  floored.pool.buyQuote = "10000000000000000000";
  floored.pool.minimumBuyerTotal = "10500000000000000000";
  assert.equal(parseDirectPoolState(JSON.stringify(floored)).ok, true);
  floored.pool.buyQuote = "9000000000000000000";
  floored.pool.minimumBuyerTotal = "9500000000000000000";
  assert.equal(parseDirectPoolState(JSON.stringify(floored)).ok, false);
});

test("v6 imports contract quotes with an active order outside the queued units", () => {
  const input = snapshot();
  const result = parseDirectPoolState(JSON.stringify(input));
  assert.equal(result.ok, true);
  if (!result.ok) return;
  assert.equal(result.state.pool.primaryUnits + result.state.pool.secondaryUnits, 2);
  assert.equal(result.state.pool.poolUnits, 3);
  assert.equal(result.state.orders[0].state, "funded");
  assert.equal(result.state.pool.buyQuote, "9000000000000000000");
  assert.equal(result.state.pool.nextSellerPayout, "3000000000000000000");

  input.pool.buyQuote = "4500000000000000000";
  input.pool.minimumBuyerTotal = "5000000000000000000";
  assert.equal(parseDirectPoolState(JSON.stringify(input)).ok, false);
});

test("v6 parser rejects secrets, malformed CIDs, invalid states, and broken references", () => {
  const base = snapshot();
  const changed = (edit: (value: any) => void) => {
    const input = structuredClone(base);
    edit(input);
    return parseDirectPoolState(JSON.stringify(input));
  };
  for (const edit of [
    (value: any) => { value.deliveryKey = "secret"; },
    (value: any) => { value.pool.deliveryHash = "secret"; },
    (value: any) => { value.offers[0].privateKey = "secret"; },
    (value: any) => { value.orders[0].ciphertext = "secret"; },
    (value: any) => { value.offers[0].originalCids = ["javascript:alert(1)"]; },
    (value: any) => { value.offers[0].originalCids = [cid("second").slice(0, -1) + "b"]; },
    (value: any) => { value.offers[0].state = "available"; },
    (value: any) => { value.orders[0].state = "fallback-intent"; },
    (value: any) => { value.orders[0].offerId = "0".repeat(64); },
    (value: any) => { value.pool.nextOfferId = "0".repeat(64); },
    (value: any) => { value.pool.buyQuote = "4000000000000000000"; },
    (value: any) => { value.pool.virtualReserve = "8000000000000000000"; },
    (value: any) => { value.offers[0].queuePosition = 1; },
    (value: any) => { value.scope = "history"; },
  ]) assert.equal(changed(edit).ok, false);
});

test("v6 parser caps snapshot bytes and total entries", () => {
  const input = snapshot();
  input.offers = Array(5001).fill(input.offers[0]);
  assert.equal(parseDirectPoolState(JSON.stringify(input)).ok, false);
  assert.equal(parseDirectPoolState(" ".repeat(20 * 1024 * 1024 + 1)).ok, false);
});

test("v6 import shows the current queue, public CID references, and dispute state only", async () => {
  const bundle = await build({
    absWorkingDir: fileURLToPath(new URL("..", import.meta.url)),
    entryPoints: ["src/main.tsx"], bundle: true, write: false, format: "iife", jsx: "automatic",
  });
  const browser = await chromium.launch({ headless: true, channel: "chrome" });
  try {
    const page = await browser.newPage();
    await page.route("**/*", (route) => route.fulfill({ contentType: "text/html", body: '<div id="root"></div>' }));
    await page.goto("https://pool-test.invalid/#/trades");
    await page.addScriptTag({ content: bundle.outputFiles[0].text });
    const input = snapshot();
    let previewRequests = 0;
    await page.route("https://api.idena.io/api/flip/*/content", (route) => {
      previewRequests++;
      const url = route.request().url();
      const pics = url.includes(input.offers[2].originalCids[0]) ? Array(8).fill(`0x${tinyJpeg}`) :
        url.includes(input.offers[0].originalCids[0]) ? [] : ["0x00112233"];
      return route.fulfill({
        headers: { "access-control-allow-origin": "*", "content-type": "application/json" },
        body: JSON.stringify({ result: { pics } }),
      });
    });
    await page.locator('input[type="file"]').setInputFiles({
      name: "pool.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(input)),
    });
    await page.getByRole("heading", { name: "Liquidity pool" }).waitFor();
    assert.match(await page.locator("main").innerText(), /Reserve at snapshot block\s+10 iDNA/);
    assert.match(await page.locator("main").innerText(), /Snapshot buy quote \(max 1,000 iDNA\)\s+9 iDNA/);
    assert.match(await page.locator("main").innerText(), /Next author payout\s+1 iDNA/);
    assert.match(await page.locator("main").innerText(), /New primary listing payout quote\s+3 iDNA/);
    const curve = await page.locator(".pool-curve").innerText();
    assert.match(curve, /2 units · snapshot\s+9 iDNA/);
    assert.match(curve, /3 units\s+3 iDNA/);
    assert.match(curve, /4 units\s+1\.5 iDNA/);
    assert.match(curve, /oldest seller's locked payout/);
    assert.equal(await page.locator(".pool-curve-chart polyline").count(), 1);
    assert.doesNotMatch(await page.locator("main").innerText(), /Next seller payout/);
    const queueRows = await page.locator(".pool-queue tbody tr").allTextContents();
    assert.match(queueRows[0], /aaaaaaaa\.\.\.aaaaaa/);
    assert.match(queueRows[1], /bbbbbbbb\.\.\.bbbbbb/);
    assert.match(await page.locator("main").innerText(), /Buyer challenged/);
    assert.match(await page.locator("main").innerText(), /Buyer paid\s*2 iDNA/);
    assert.match(await page.locator("main").innerText(), /Author payout\s*1 iDNA/);
    assert.match(await page.locator("main").innerText(), /Repeat-sale payments split 50% to the original author and 50% to the pool/);
    assert.match(await page.locator("main").innerText(), /oldest primary offer first.*oldest secondary offer/);
    assert.match(await page.locator("main").innerText(), /public flip reference may remain encrypted/i);
    assert.match(await page.locator("main").innerText(), /Buyer bond\s+0\.5 iDNA/);
    assert.equal(await page.locator('a[href^="https://scan.idena.io/flip/"]').count(), 0);
    await page.locator(".pool-queue tbody tr").first().locator(".pool-id summary").first().click();
    assert.equal(await page.locator(".pool-queue tbody tr").first().locator(".pool-id code").first().innerText(), ids[0]);
    for (const details of await page.locator(".pool-flips").all()) await details.locator("summary").click();
    const links = page.locator('a[href^="https://scan.idena.io/flip/"]');
    await page.waitForFunction(() => document.querySelectorAll('a[href^="https://scan.idena.io/flip/"]').length === 3);
    assert.equal(await links.count(), 3);
    const actualLinks = await links.evaluateAll((elements) => elements.map((element) => element.getAttribute("href")));
    assert.deepEqual(actualLinks.sort(),
      input.offers.map((item) => `https://scan.idena.io/flip/${item.originalCids[0]}`).sort());
    assert.deepEqual((await page.locator(".pool-cids code").allTextContents()).sort(),
      input.offers.map((item) => item.originalCids[0]).sort());
    assert.equal(previewRequests, 0);
    assert.equal(await page.locator("img").count(), 0);
    await page.locator(".pool-queue tbody tr").first().getByRole("button", { name: "Load preview from Idena explorer" }).click();
    await page.waitForFunction(() => document.querySelector<HTMLImageElement>(".pool-preview img")?.naturalWidth === 2);
    assert.equal(previewRequests, 1);
    assert.equal(await page.locator(".pool-preview img").count(), 8);
    assert.match(await page.locator(".pool-preview").first().innerText(), /images may still be encrypted/);
    await page.locator(".pool-queue tbody tr").nth(1).getByRole("button", { name: "Load preview from Idena explorer" }).click();
    await page.getByText("No public preview is available.").waitFor();
    await page.locator(".pool-preview").last().getByRole("button", { name: "Load preview from Idena explorer" }).click();
    await page.getByText("Public preview could not be loaded.").waitFor();
    assert.equal(previewRequests, 3);
    assert.equal(await page.getByRole("button", { name: /buy|sign|purchase|confirm/i }).count(), 0);
    assert.equal(await page.getByRole("link", { name: "Use flips" }).count(), 0);

    await page.setViewportSize({ width: 390, height: 800 });
    await page.getByRole("button", { name: "Current orders" }).click();
    assert.equal(await page.getByRole("heading", { name: "Current orders" }).isVisible(), true);
    await page.getByRole("button", { name: "Find offer" }).click();
    await page.getByRole("searchbox", { name: "Find offer" }).fill(ids[1]);
    assert.equal(await page.locator(".pool-queue tbody tr").count(), 1);
    assert.match(await page.locator(".pool-queue tbody tr").first().innerText(), /secondary/);

    const paused = snapshot();
    paused.pool.buyPausedForBond = true;
    (paused.pool as any).bondTopUpRequired = "750000000000000000";
    await page.locator('input[type="file"]').setInputFiles({
      name: "paused-pool.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(paused)),
    });
    await page.getByText(/seller bond for queue head.*needs 0\.75 iDNA more/).waitFor();
    assert.match(await page.locator("main").innerText(), /queue head\s+aaaaaaaa/);
    assert.match(await page.locator("main").innerText(), /needs 0\.75 iDNA more/);

    paused.observedAt = "2020-01-01T00:00:00Z";
    await page.locator('input[type="file"]').setInputFiles({
      name: "stale-pool.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(paused)),
    });
    await page.getByText(/Buying was paused at this snapshot's block/).waitFor();
    const staleText = await page.locator("main").innerText();
    assert.match(staleText, /Read-only imported snapshot; verify current chain state before a trade/);
    assert.match(staleText, /Snapshot buy quote \(max 1,000 iDNA\)\s+Unavailable \(stale snapshot\)/);
    assert.match(staleText, /Buyer bond\s+Unavailable \(stale snapshot\)/);
    assert.match(staleText, /Total to lock\s+Unavailable \(stale snapshot\)/);
    assert.match(staleText, /New primary listing payout quote\s+Unavailable \(stale snapshot\)/);
    assert.match(staleText, /Reserve at snapshot block\s+10 iDNA/);
    assert.match(staleText, /Buying was paused at this snapshot's block/);
  } finally {
    await browser.close();
  }
});

test("v6 queue displays large inventories in batches", async () => {
  const bundle = await build({
    absWorkingDir: fileURLToPath(new URL("..", import.meta.url)),
    entryPoints: ["src/main.tsx"], bundle: true, write: false, format: "iife", jsx: "automatic",
  });
  const browser = await chromium.launch({ headless: true, channel: "chrome" });
  try {
    const page = await browser.newPage();
    await page.route("**/*", (route) => route.fulfill({ contentType: "text/html", body: '<div id="root"></div>' }));
    await page.goto("https://pool-test.invalid/#/market");
    await page.addScriptTag({ content: bundle.outputFiles[0].text });
    const input = snapshot();
    const first = input.offers.find((item) => item.offerId === ids[0])!;
    input.offers.find((item) => item.offerId === ids[1])!.queuePosition = 52;
    const extra = Array.from({ length: 50 }, (_, index) => ({
      ...first, offerId: (index + 4).toString(16).padStart(64, "0"),
      originalCids: [cid(`extra-${index}`)], queuePosition: index + 2,
      sequence: String(index + 4),
    }));
    input.offers.push(...extra);
    input.pool.primaryUnits = 51;
    input.pool.poolUnits = 53;
    input.pool.buyQuote = "1000000000000000000";
    input.pool.minimumBuyerTotal = "1500000000000000000";
    input.pool.nextSellerPayout = "169811320754716981";
    assert.equal(parseDirectPoolState(JSON.stringify(input)).ok, true);
    await page.locator('input[type="file"]').setInputFiles({
      name: "pool.json", mimeType: "application/json",
      buffer: Buffer.from(JSON.stringify(input).padEnd(6 * 1024 * 1024, " ")),
    });
    await page.getByRole("heading", { name: "Liquidity pool" }).waitFor();
    assert.equal(await page.locator(".pool-queue tbody tr").count(), 10);
    assert.equal(await page.locator('a[href^="https://scan.idena.io/flip/"]').count(), 0);
    await page.getByRole("button", { name: "Show 50 more" }).click();
    assert.equal(await page.locator(".pool-queue tbody tr").count(), 52);
  } finally {
    await browser.close();
  }
});
