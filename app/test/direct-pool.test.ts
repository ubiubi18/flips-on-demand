import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
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

test("v6 snapshot accepts two proved seeds before liquidity and offers no price", () => {
  const input = snapshot();
  const first = input.offers[2];
  const second = input.offers[0];
  first.state = "queued-secondary";
  first.queue = "secondary";
  first.contentProven = true;
  first.publicSeed = true;
  first.everSold = false;
  first.payout = "0";
  first.sellerBond = "0";
  second.contentProven = true;
  second.publicSeed = true;
  second.everSold = false;
  second.payout = "0";
  second.sellerBond = "0";
  input.offers = [first, second];
  input.orders = [];
  Object.assign(input.pool, {
    reserve: "0", committed: "0", buyerPayments: "0", insurance: "0", pending: "0",
    virtualReserve: "0", primaryUnits: 0, secondaryUnits: 2, poolUnits: 2,
    reservedUnits: 0, seedCount: 2, seedOfferIds: [ids[0], ids[1]],
    nextOrderId: "1", nextSequence: "3", nextOfferId: ids[0], buyPausedForBond: false,
  });
  delete (input.pool as any).buyQuote;
  delete (input.pool as any).minimumBuyerBond;
  delete (input.pool as any).minimumBuyerTotal;
  delete (input.pool as any).nextSellerPayout;
  const result = parseDirectPoolState(JSON.stringify(input));
  assert.equal(result.ok, true);
  input.pool.buyQuote = "1";
  assert.equal(parseDirectPoolState(JSON.stringify(input)).ok, false);
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

test("v6 market foregrounds the next offer, keeps explorer loading opt-in, and shows disputes", async () => {
  const bundle = await build({
    absWorkingDir: fileURLToPath(new URL("..", import.meta.url)),
    entryPoints: ["src/main.tsx"], bundle: true, write: false, format: "iife", jsx: "automatic",
  });
  const browser = await chromium.launch({ headless: true, channel: "chrome" });
  try {
    const page = await browser.newPage();
    const styles = await readFile(fileURLToPath(new URL("../src/styles.css", import.meta.url)), "utf8");
    await page.route("**/*", (route) => route.fulfill({ contentType: "text/html", body: '<style>' + styles + '</style><div id="root"></div>' }));
    await page.goto("https://pool-test.invalid/#/trades");
    await page.addScriptTag({ content: bundle.outputFiles[0].text });
    const input = snapshot();
    let previewRequests = 0;
    await page.route("https://api.idena.io/api/flip/*/content", (route) => {
      previewRequests++;
      const url = route.request().url();
      const pics = url.includes(input.offers[2].originalCids[0]) ? Array(2).fill("0x" + tinyJpeg) :
        url.includes(input.offers[0].originalCids[0]) ? [] : ["0x00112233"];
      return route.fulfill({
        headers: { "access-control-allow-origin": "*", "content-type": "application/json" },
        body: JSON.stringify({ result: { pics } }),
      });
    });
    await page.locator('input[type="file"]').setInputFiles({
      name: "pool.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(input)),
    });
    await page.getByRole("heading", { name: "Explore original flips." }).waitFor();
    assert.match(await page.locator(".pool-hero").innerText(), /Next bundle at snapshot\s*9 iDNA/);
    assert.match(await page.locator(".pool-featured").innerText(), /1 original flip/);
    assert.match(await page.locator(".pool-economics").innerText(), /iDNA reserve\s*10/);
    assert.match(await page.locator(".pool-economics").innerText(), /Author payout\s*1 iDNA/);
    assert.match(await page.locator(".pool-economics").innerText(), /New primary listing quote\s*3 iDNA/);
    const curve = await page.locator(".pool-curve").innerText();
    assert.match(curve, /2 units · snapshot\s*9 iDNA/);
    assert.match(curve, /3 units\s*3 iDNA/);
    assert.match(curve, /4 units\s*1\.5 iDNA/);
    assert.match(curve, /oldest seller's locked payout/);
    assert.equal(await page.locator(".pool-curve-chart polyline").count(), 1);
    assert.equal(await page.locator(".pool-queue .pool-offer-card").count(), 2);
    assert.match(await page.locator(".pool-queue .pool-offer-card").first().innerText(), /Queue 1/);
    assert.match(await page.locator(".pool-queue .pool-offer-card").last().innerText(), /Resale/);
    assert.match(await page.locator("#pool-orders").innerText(), /Buyer challenged/);
    assert.equal((await page.locator("#pool-orders tbody tr").first().locator("td").nth(3).innerText()).trim(), "2 iDNA");
    assert.match(await page.locator(".pool-queue-rule").innerText(), /Never-sold bundles go first, oldest listing first/);
    assert.match(await page.locator(".pool-aside-note").innerText(), /Repeat-sale payments split 50%/);
    assert.equal(previewRequests, 0);
    assert.equal(await page.locator("img").count(), 0);
    await page.getByRole("button", { name: "Show public preview" }).click();
    await page.waitForFunction(() => document.querySelector<HTMLImageElement>(".pool-featured img")?.naturalWidth === 2);
    assert.equal(previewRequests, 1);
    assert.equal(await page.locator(".pool-featured img").count(), 2);

    await page.locator(".pool-featured .pool-flips summary").click();
    const featureLink = page.locator(".pool-featured a[href^='https://scan.idena.io/flip/']");
    await featureLink.waitFor();
    assert.equal(await featureLink.count(), 1);
    assert.equal(await featureLink.getAttribute("href"), "https://scan.idena.io/flip/" + input.offers[2].originalCids[0]);
    await page.locator(".pool-featured .pool-cids .pool-preview button").click();
    await page.waitForFunction(() => document.querySelectorAll(".pool-featured .pool-cids img").length === 2);
    assert.match(await page.locator(".pool-featured .pool-cids .pool-preview").innerText(), /complete flip may remain encrypted/);

    await page.locator(".pool-queue .pool-offer-card").last().getByRole("button", { name: "Preview" }).click();
    await page.locator(".pool-queue .pool-offer-card").last().getByText("Unavailable").waitFor();
    await page.locator(".pool-other-offers summary").first().click();
    await page.locator(".pool-other-offers .pool-offer-card").first().getByRole("button", { name: "Preview" }).click();
    await page.locator(".pool-other-offers .pool-offer-card").first().getByText("Unavailable").waitFor();
    assert.equal(previewRequests, 4);
    assert.equal(await page.getByRole("button", { name: /buy|sign|purchase|confirm/i }).count(), 0);

    await page.setViewportSize({ width: 390, height: 800 });
    await page.getByRole("searchbox", { name: "Find offer" }).fill(ids[1]);
    assert.equal(await page.locator(".pool-queue .pool-offer-card").count(), 1);
    assert.match(await page.locator(".pool-queue .pool-offer-card").first().innerText(), /Resale/);
    assert.equal(await page.locator("body").evaluate((body) => body.scrollWidth <= window.innerWidth), true);

    const paused = snapshot();
    paused.pool.buyPausedForBond = true;
    (paused.pool as any).bondTopUpRequired = "750000000000000000";
    await page.locator('input[type="file"]').setInputFiles({
      name: "paused-pool.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(paused)),
    });
    await page.getByText(/seller bond for queue head.*needs 0\.75 iDNA more/).waitFor();

    paused.observedAt = "2020-01-01T00:00:00Z";
    await page.locator('input[type="file"]').setInputFiles({
      name: "stale-pool.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(paused)),
    });
    await page.getByText(/Buying was paused at this snapshot's block/).waitFor();
    assert.match(await page.locator(".pool-hero").innerText(), /Unavailable \(stale snapshot\)/);
    assert.match(await page.locator(".pool-economics").innerText(), /Buyer bond\s*Unavailable \(stale snapshot\)/);
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
    await page.getByRole("heading", { name: "Explore original flips." }).waitFor();
    assert.equal(await page.locator(".pool-queue .pool-offer-card").count(), 10);
    assert.equal(await page.locator('a[href^="https://scan.idena.io/flip/"]').count(), 0);
    await page.getByRole("button", { name: "Show 50 more" }).click();
    assert.equal(await page.locator(".pool-queue .pool-offer-card").count(), 52);
  } finally {
    await browser.close();
  }
});
