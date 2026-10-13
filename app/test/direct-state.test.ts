import { test } from "node:test";
import assert from "node:assert/strict";
import { parseDirectMarketState } from "../src/direct-state.ts";

const offerId = "a".repeat(64);
const author = "b".repeat(40);
const buyer = "c".repeat(40);

function snapshot() {
  return {
    format: "fod.direct-market-state", version: 2, network: "idena",
    contract: "d".repeat(40), codeHash: "e".repeat(64),
    blockHeight: 5_000_000, observedAt: "2026-10-11T10:00:00Z", source: "node",
    offers: [{
      offerId, author, epoch: 230, coveredFlips: 3,
      inventoryCommitment: "f".repeat(64), price: "1000000000000000000",
      openOrderId: "18446744073709551615", retired: false,
    }],
    orders: [{
      orderId: "18446744073709551615", offerId, buyer, paid: "1000000000000000000", bond: "416666666666500000000",
      fundedAtBlock: "5000000", refundAtBlock: "5008640", state: "funded", keysPublic: false,
    }],
  };
}

test("direct snapshot keeps full-width order IDs and renders public fields only", () => {
  const input = { ...snapshot(), deliveryKey: "private material" };
  const result = parseDirectMarketState(JSON.stringify(input));
  assert.equal(result.ok, true);
  if (!result.ok) return;
  assert.equal(result.state.orders[0].orderId, "18446744073709551615");
  assert.equal(result.state.orders[0].bond, "416666666666500000000");
  assert.equal(result.state.offers[0].price, "1000000000000000000");
  assert.equal("deliveryKey" in result.state, false);
});

test("direct snapshot identifies a settled public key reveal", () => {
  const disclosed = snapshot();
  disclosed.orders[0].state = "settled";
  disclosed.orders[0].keysPublic = true;
  const result = parseDirectMarketState(JSON.stringify(disclosed));
  assert.equal(result.ok, true);
  if (result.ok) assert.equal(result.state.orders[0].keysPublic, true);
});

test("direct snapshot accepts a started fallback without public keys", () => {
  const pending = snapshot();
  pending.orders[0].state = "fallback-pending";
  const result = parseDirectMarketState(JSON.stringify(pending));
  assert.equal(result.ok, true);
  if (result.ok) {
    assert.equal(result.state.orders[0].state, "fallback-pending");
    assert.equal(result.state.orders[0].keysPublic, false);
  }
});

test("direct snapshot accepts a seller intent with an earlier refund boundary", () => {
  const intent = snapshot();
  intent.orders[0].state = "fallback-intent";
  intent.orders[0].refundAtBlock = "5004320";
  const result = parseDirectMarketState(JSON.stringify(intent));
  assert.equal(result.ok, true);
  if (result.ok) assert.equal(result.state.orders[0].refundAtBlock, "5004320");
});

test("v4 direct export loads settled orders and pending seller stake", () => {
  const settled = { ...snapshot(), version: 3 };
  settled.orders[0].state = "settled";
  const settledResult = parseDirectMarketState(JSON.stringify(settled));
  assert.equal(settledResult.ok, true);
  if (settledResult.ok) {
    assert.equal(settledResult.state.version, 3);
    assert.equal(settledResult.state.orders[0].sellerStake, undefined);
  }

  const pending = {
    ...snapshot(), version: 3,
    orders: [{ ...snapshot().orders[0], state: "fallback-pending", sellerStake: "2000000000000000000" }],
  };
  const pendingResult = parseDirectMarketState(JSON.stringify(pending));
  assert.equal(pendingResult.ok, true);
  if (pendingResult.ok) {
    assert.equal(pendingResult.state.orders[0].state, "fallback-pending");
    assert.equal(pendingResult.state.orders[0].sellerStake, "2000000000000000000");
    assert.equal(pendingResult.state.orders[0].keysPublic, false);
  }
});

test("v4 direct export rejects removed states and invalid seller stakes", () => {
  const pending = {
    ...snapshot(), version: 3,
    orders: [{ ...snapshot().orders[0], state: "fallback-pending", sellerStake: "1000000000000000000" }],
  };
  for (const sellerStake of [undefined, "0", "999999999999999999", "not a number"]) {
    const invalid = { ...pending, orders: [{ ...pending.orders[0], sellerStake }] };
    assert.equal(parseDirectMarketState(JSON.stringify(invalid)).ok, false);
  }
  const removedIntent = { ...pending, orders: [{ ...pending.orders[0], state: "fallback-intent" }] };
  assert.equal(parseDirectMarketState(JSON.stringify(removedIntent)).ok, false);
  const unexpectedStake = { ...pending, orders: [{ ...pending.orders[0], state: "funded" }] };
  assert.equal(parseDirectMarketState(JSON.stringify(unexpectedStake)).ok, false);
  const oldVersion = { ...pending, version: 2 };
  assert.equal(parseDirectMarketState(JSON.stringify(oldVersion)).ok, false);
});

test("direct snapshot refuses malformed and orphan chain records", () => {
  const orphan = snapshot();
  orphan.orders[0].offerId = "0".repeat(64);
  assert.equal(parseDirectMarketState(JSON.stringify(orphan)).ok, false);
  const malformed = snapshot();
  malformed.offers[0].author = "0x" + author;
  assert.equal(parseDirectMarketState(JSON.stringify(malformed)).ok, false);
  const past = snapshot();
  past.orders[0].refundAtBlock = "4999999";
  assert.equal(parseDirectMarketState(JSON.stringify(past)).ok, false);
  const duplicate = snapshot();
  duplicate.orders.push({ ...duplicate.orders[0] });
  assert.equal(parseDirectMarketState(JSON.stringify(duplicate)).ok, false);
  const noBond = snapshot();
  noBond.orders[0].bond = "0";
  assert.equal(parseDirectMarketState(JSON.stringify(noBond)).ok, false);
  const prematureDisclosure = snapshot();
  prematureDisclosure.orders[0].keysPublic = true;
  assert.equal(parseDirectMarketState(JSON.stringify(prematureDisclosure)).ok, false);
});
