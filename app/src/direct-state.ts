// Public direct-sale records exported from a node. Imported state is advisory:
// the local CLI rechecks the contract, canonical session and blobs before buying.

export interface DirectOfferView {
  offerId: string;
  author: string;
  epoch: number;
  coveredFlips: number;
  inventoryCommitment: string;
  price: string;
  openOrderId: string;
  retired: boolean;
}

export interface DirectOrderView {
  orderId: string;
  offerId: string;
  buyer: string;
  paid: string;
  bond: string;
  sellerStake?: string;
  fundedAtBlock: string;
  refundAtBlock: string;
  state: "funded" | "fallback-intent" | "fallback-pending" | "settled" | "refunded";
  keysPublic: boolean;
}

export interface DirectMarketState {
  format: "fod.direct-market-state";
  version: 2 | 3;
  network: "idena";
  contract: string;
  codeHash: string;
  blockHeight: number;
  observedAt: string;
  source: "node";
  offers: DirectOfferView[];
  orders: DirectOrderView[];
}

type ParseResult =
  | { ok: true; state: DirectMarketState; warnings: string[] }
  | { ok: false; error: string };

const MAX_BYTES = 5 * 1024 * 1024;
const MAX_ITEMS = 5000;
const HEX_20 = /^[0-9a-f]{40}$/;
const HEX_32 = /^[0-9a-f]{64}$/;
const DECIMAL = /^(0|[1-9][0-9]{0,38})$/;

function record(value: unknown, where: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`${where} must be an object`);
  }
  return value as Record<string, unknown>;
}

function string(value: unknown, where: string, pattern: RegExp): string {
  if (typeof value !== "string" || !pattern.test(value)) {
    throw new Error(`${where} has an unexpected form`);
  }
  return value;
}

function decimal(value: unknown, where: string): string {
  return string(value, where, DECIMAL);
}

function number(value: unknown, where: string, max: number): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0 || value > max) {
    throw new Error(`${where} must be a non-negative integer`);
  }
  return value;
}

function boolean(value: unknown, where: string): boolean {
  if (typeof value !== "boolean") {
    throw new Error(`${where} must be a boolean`);
  }
  return value;
}

function parseOffer(value: unknown, index: number): DirectOfferView {
  const where = `offers[${index}]`;
  const item = record(value, where);
  return {
    offerId: string(item.offerId, `${where}.offerId`, HEX_32),
    author: string(item.author, `${where}.author`, HEX_20),
    epoch: number(item.epoch, `${where}.epoch`, 65535),
    coveredFlips: number(item.coveredFlips, `${where}.coveredFlips`, 32),
    inventoryCommitment: string(item.inventoryCommitment, `${where}.inventoryCommitment`, HEX_32),
    price: decimal(item.price, `${where}.price`),
    openOrderId: decimal(item.openOrderId, `${where}.openOrderId`),
    retired: boolean(item.retired, `${where}.retired`),
  };
}

function parseOrder(value: unknown, index: number, version: DirectMarketState["version"]): DirectOrderView {
  const where = `orders[${index}]`;
  const item = record(value, where);
  const state = string(item.state, `${where}.state`, version === 3
    ? /^(funded|fallback-pending|settled|refunded)$/
    : /^(funded|fallback-intent|fallback-pending|settled|refunded)$/);
  const keysPublic = boolean(item.keysPublic, `${where}.keysPublic`);
  if (keysPublic && state !== "settled") {
    throw new Error(`${where}.keysPublic requires a settled order`);
  }
  const fundedAtBlock = decimal(item.fundedAtBlock, `${where}.fundedAtBlock`);
  const refundAtBlock = decimal(item.refundAtBlock, `${where}.refundAtBlock`);
  if (BigInt(refundAtBlock) < BigInt(fundedAtBlock)) {
    throw new Error(`${where}.refundAtBlock is before funding`);
  }
  const paid = decimal(item.paid, `${where}.paid`);
  let sellerStake: string | undefined;
  if (version === 3 && state === "fallback-pending") {
    sellerStake = decimal(item.sellerStake, `${where}.sellerStake`);
    if (BigInt(sellerStake) < BigInt(paid)) {
      throw new Error(`${where}.sellerStake is below the price`);
    }
  } else if ("sellerStake" in item) {
    throw new Error(`${where}.sellerStake is unexpected for this order`);
  }
  return {
    orderId: decimal(item.orderId, `${where}.orderId`),
    offerId: string(item.offerId, `${where}.offerId`, HEX_32),
    buyer: string(item.buyer, `${where}.buyer`, HEX_20),
    paid,
    bond: decimal(item.bond, `${where}.bond`),
    ...(sellerStake === undefined ? {} : { sellerStake }),
    fundedAtBlock,
    refundAtBlock,
    state: state as DirectOrderView["state"],
    keysPublic,
  };
}

export function parseDirectMarketState(text: string): ParseResult {
  if (text.length > MAX_BYTES) {
    return { ok: false, error: "the file is larger than the interface accepts" };
  }
  try {
    const raw = record(JSON.parse(text), "the file");
    const version = raw.version;
    if (raw.format !== "fod.direct-market-state" || (version !== 2 && version !== 3)) {
      throw new Error("this is not a supported direct-sale state file");
    }
    if (raw.network !== "idena" || raw.source !== "node") {
      throw new Error("direct-sale state has an unexpected network or source");
    }
    if (!Array.isArray(raw.offers) || !Array.isArray(raw.orders) ||
        raw.offers.length > MAX_ITEMS || raw.orders.length > MAX_ITEMS) {
      throw new Error("direct-sale state has invalid or excessive entries");
    }
    const state: DirectMarketState = {
      format: "fod.direct-market-state", version, network: "idena",
      contract: string(raw.contract, "contract", HEX_20),
      codeHash: string(raw.codeHash, "codeHash", HEX_32),
      blockHeight: number(raw.blockHeight, "blockHeight", Number.MAX_SAFE_INTEGER),
      observedAt: string(raw.observedAt, "observedAt", /^\d{4}-\d\d-\d\dT/),
      source: "node",
      offers: raw.offers.map(parseOffer),
      orders: raw.orders.map((order, index) => parseOrder(order, index, version)),
    };
    if (Number.isNaN(Date.parse(state.observedAt))) {
      throw new Error("observedAt is not a readable date");
    }
    const ids = new Set<string>();
    for (const offer of state.offers) {
      if (ids.has(offer.offerId) || offer.coveredFlips === 0 || offer.price === "0") {
        throw new Error("direct-sale state has a duplicate or invalid offer");
      }
      ids.add(offer.offerId);
    }
    const orderIds = new Set<string>();
    for (const order of state.orders) {
      if (orderIds.has(order.orderId) || order.orderId === "0" || order.paid === "0" ||
          order.bond === "0" || !ids.has(order.offerId)) {
        throw new Error("direct-sale state has a duplicate or orphan order");
      }
      orderIds.add(order.orderId);
    }
    return { ok: true, state, warnings: [] };
  } catch (error) {
    return { ok: false, error: error instanceof SyntaxError ? "the file is not valid JSON" :
      error instanceof Error ? error.message : "the file could not be read" };
  }
}
