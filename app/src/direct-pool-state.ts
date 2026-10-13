// Public v6 pool snapshots are browsing data. Keep private delivery material out
// of the accepted schema, and never use an imported file to authorize a trade.

export interface DirectPoolOffer {
  offerId: string;
  author: string;
  epoch: number;
  coveredFlips: number;
  originalCids: string[];
  inventoryCommitment: string;
  keysCommitment: string;
  payout: string;
  sellerBond: string;
  fraudCollateral: string;
  nextNonce: string;
  openOrderId: string;
  state: "qualified" | "queued-primary" | "queued-secondary" | "active" | "sold-private" | "cancelled" | "retired";
  everSold: boolean;
  contentProven: boolean;
  publicSeed: boolean;
  queue?: "primary" | "secondary";
  queuePosition: number;
  sequence: string;
  listedAtBlock: string;
}

export interface DirectPoolOrder {
  orderId: string;
  offerId: string;
  buyer: string;
  paid: string;
  bond: string;
  sellerPayout: string;
  fundedAtBlock: string;
  refundAtBlock: string;
  nonce: string;
  state: "funded" | "fallback-pending" | "settled" | "refunded";
  challenged: boolean;
  refundAvailable: boolean;
}

export interface DirectPoolTotals {
  reserve: string;
  committed: string;
  buyerPayments: string;
  insurance: string;
  pending: string;
  virtualReserve: string;
  primaryUnits: number;
  secondaryUnits: number;
  poolUnits: number;
  reservedUnits: number;
  seedCount: number;
  seedOfferIds: [string, string];
  nextOrderId: string;
  nextSequence: string;
  nextOfferId?: string;
  buyQuote?: string;
  minimumBuyerBond?: string;
  minimumBuyerTotal?: string;
  nextSellerPayout?: string;
  minimumProvedSellerBond?: string;
  minimumUnprovedSellerFunds?: string;
  bondTopUpRequired?: string;
  buyPausedForBond: boolean;
}

export interface DirectPoolState {
  format: "fod.direct-pool-state";
  version: 6;
  scope: "live";
  network: "idena";
  contract: string;
  codeHash: string;
  blockHeight: number;
  observedAt: string;
  source: "node";
  pool: DirectPoolTotals;
  offers: DirectPoolOffer[];
  orders: DirectPoolOrder[];
}

type ParseResult =
  | { ok: true; state: DirectPoolState; warnings: string[] }
  | { ok: false; error: string };

// Five thousand live offers can each contain up to 32 public CID references.
const MAX_BYTES = 20 * 1024 * 1024;
const MAX_ITEMS = 5000;
const HEX_20 = /^[0-9a-f]{40}$/;
const HEX_32 = /^[0-9a-f]{64}$/;
const DECIMAL = /^(0|[1-9][0-9]*)$/;
// CIDv1/raw or CIDv1/dag-pb, sha2-256, lower-case unpadded base32. The last
// character has two zero pad bits, so alternate spellings cannot make links.
const FLIP_CID = /^b(?:afkrei|afybei)[a-z2-7]{51}[aeimquy4]$/;
const UINT64_MAX = (1n << 64n) - 1n;
const AMOUNT_MAX = (1n << 128n) - 1n;
export const DIRECT_POOL_PRICE_CAP = 1_000n * 1_000_000_000_000_000_000n;

function cappedQuote(value: bigint): bigint {
  return value < DIRECT_POOL_PRICE_CAP ? value : DIRECT_POOL_PRICE_CAP;
}

function object(value: unknown, where: string, required: readonly string[], optional: readonly string[] = []): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(`${where} must be an object`);
  }
  const item = value as Record<string, unknown>;
  for (const key of required) {
    if (!Object.prototype.hasOwnProperty.call(item, key)) throw new Error(`${where}.${key} is missing`);
  }
  for (const key of Object.keys(item)) {
    if (!required.includes(key) && !optional.includes(key)) {
      throw new Error(`${where} contains an unexpected field`);
    }
  }
  return item;
}

function shaped(value: unknown, where: string, pattern: RegExp): string {
  if (typeof value !== "string" || !pattern.test(value)) {
    throw new Error(`${where} has an unexpected form`);
  }
  return value;
}

function decimal(value: unknown, where: string, max: bigint): string {
  const result = shaped(value, where, DECIMAL);
  if (result.length > max.toString().length || BigInt(result) > max) {
    throw new Error(`${where} is outside its range`);
  }
  return result;
}

function integer(value: unknown, where: string, max: number): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0 || value > max) {
    throw new Error(`${where} must be a non-negative integer`);
  }
  return value;
}

function flag(value: unknown, where: string): boolean {
  if (typeof value !== "boolean") throw new Error(`${where} must be a boolean`);
  return value;
}

function optionalDecimal(item: Record<string, unknown>, key: string, where: string): string | undefined {
  return key in item ? decimal(item[key], `${where}.${key}`, AMOUNT_MAX) : undefined;
}

function parsePool(value: unknown): DirectPoolTotals {
  const required = ["reserve", "committed", "buyerPayments", "insurance", "pending", "virtualReserve",
    "primaryUnits", "secondaryUnits", "poolUnits", "reservedUnits", "seedCount", "seedOfferIds",
    "nextOrderId", "nextSequence", "buyPausedForBond"];
  const optional = ["nextOfferId", "buyQuote", "minimumBuyerBond", "minimumBuyerTotal", "nextSellerPayout",
    "minimumProvedSellerBond", "minimumUnprovedSellerFunds", "bondTopUpRequired"];
  const item = object(value, "pool", required, optional);
  if (!Array.isArray(item.seedOfferIds) || item.seedOfferIds.length !== 2) {
    throw new Error("pool.seedOfferIds must contain two offer IDs");
  }
  const minimumBuyerBond = optionalDecimal(item, "minimumBuyerBond", "pool");
  const minimumBuyerTotal = optionalDecimal(item, "minimumBuyerTotal", "pool");
  const nextSellerPayout = optionalDecimal(item, "nextSellerPayout", "pool");
  const minimumProvedSellerBond = optionalDecimal(item, "minimumProvedSellerBond", "pool");
  const minimumUnprovedSellerFunds = optionalDecimal(item, "minimumUnprovedSellerFunds", "pool");
  const bondTopUpRequired = optionalDecimal(item, "bondTopUpRequired", "pool");
  return {
    reserve: decimal(item.reserve, "pool.reserve", AMOUNT_MAX),
    committed: decimal(item.committed, "pool.committed", AMOUNT_MAX),
    buyerPayments: decimal(item.buyerPayments, "pool.buyerPayments", AMOUNT_MAX),
    insurance: decimal(item.insurance, "pool.insurance", AMOUNT_MAX),
    pending: decimal(item.pending, "pool.pending", AMOUNT_MAX),
    virtualReserve: decimal(item.virtualReserve, "pool.virtualReserve", AMOUNT_MAX),
    primaryUnits: integer(item.primaryUnits, "pool.primaryUnits", MAX_ITEMS),
    secondaryUnits: integer(item.secondaryUnits, "pool.secondaryUnits", MAX_ITEMS),
    poolUnits: integer(item.poolUnits, "pool.poolUnits", MAX_ITEMS),
    reservedUnits: integer(item.reservedUnits, "pool.reservedUnits", MAX_ITEMS),
    seedCount: integer(item.seedCount, "pool.seedCount", 2),
    seedOfferIds: [shaped(item.seedOfferIds[0], "pool.seedOfferIds[0]", HEX_32),
      shaped(item.seedOfferIds[1], "pool.seedOfferIds[1]", HEX_32)],
    nextOrderId: decimal(item.nextOrderId, "pool.nextOrderId", UINT64_MAX),
    nextSequence: decimal(item.nextSequence, "pool.nextSequence", UINT64_MAX),
    ...(item.nextOfferId === undefined ? {} : { nextOfferId: shaped(item.nextOfferId, "pool.nextOfferId", HEX_32) }),
    ...(item.buyQuote === undefined ? {} : { buyQuote: decimal(item.buyQuote, "pool.buyQuote", AMOUNT_MAX) }),
    ...(minimumBuyerBond === undefined ? {} : { minimumBuyerBond }),
    ...(minimumBuyerTotal === undefined ? {} : { minimumBuyerTotal }),
    ...(nextSellerPayout === undefined ? {} : { nextSellerPayout }),
    ...(minimumProvedSellerBond === undefined ? {} : { minimumProvedSellerBond }),
    ...(minimumUnprovedSellerFunds === undefined ? {} : { minimumUnprovedSellerFunds }),
    ...(bondTopUpRequired === undefined ? {} : { bondTopUpRequired }),
    buyPausedForBond: flag(item.buyPausedForBond, "pool.buyPausedForBond"),
  };
}

function checkPoolMath(pool: DirectPoolTotals, nextPayout: bigint): void {
  if (pool.primaryUnits + pool.secondaryUnits + pool.reservedUnits !== pool.poolUnits) {
    throw new Error("pool unit counts disagree");
  }
  const reserve = BigInt(pool.reserve);
  const committed = BigInt(pool.committed);
  const virtualReserve = reserve - committed + BigInt(pool.buyerPayments) - BigInt(pool.insurance);
  if (reserve < committed || virtualReserve <= 0n || virtualReserve !== BigInt(pool.virtualReserve)) {
    throw new Error("pool virtual reserve disagrees with its balances");
  }
  const units = BigInt(pool.primaryUnits + pool.secondaryUnits);
  const curveQuote = units > 1n ? cappedQuote((virtualReserve + units - 2n) / (units - 1n)) : 0n;
  const expectedQuote = curveQuote > nextPayout ? curveQuote : nextPayout;
  if (pool.buyQuote !== undefined &&
      (units <= 1n || BigInt(pool.buyQuote) !== expectedQuote)) {
    throw new Error("pool buy quote disagrees with its curve and locked payout");
  }
  if (pool.nextSellerPayout !== undefined &&
      (units === 0n || BigInt(pool.nextSellerPayout) !== cappedQuote(virtualReserve / (units + 1n)))) {
    throw new Error("pool listing payout disagrees with its curve");
  }
  if (pool.minimumBuyerTotal !== undefined &&
      (pool.buyQuote === undefined || pool.minimumBuyerBond === undefined ||
       BigInt(pool.minimumBuyerTotal) !== BigInt(pool.buyQuote) + BigInt(pool.minimumBuyerBond))) {
    throw new Error("pool buyer total disagrees with its quote and bond");
  }
  if (pool.buyPausedForBond !== (pool.bondTopUpRequired !== undefined) ||
      (pool.bondTopUpRequired !== undefined && (pool.nextOfferId === undefined || BigInt(pool.bondTopUpRequired) === 0n))) {
    throw new Error("pool bond pause disagrees with its shortfall");
  }
}

function parseOffer(value: unknown, index: number): DirectPoolOffer {
  const where = `offers[${index}]`;
  const item = object(value, where,
    ["offerId", "author", "epoch", "coveredFlips", "originalCids", "inventoryCommitment", "keysCommitment",
      "payout", "sellerBond", "fraudCollateral", "nextNonce", "openOrderId", "state", "everSold",
      "contentProven", "publicSeed", "sequence", "listedAtBlock"],
    ["queue", "queuePosition"]);
  const coveredFlips = integer(item.coveredFlips, `${where}.coveredFlips`, 32);
  if (coveredFlips === 0 || !Array.isArray(item.originalCids) || item.originalCids.length !== coveredFlips) {
    throw new Error(`${where}.originalCids must match coveredFlips`);
  }
  const originalCids = item.originalCids.map((cid, offset) => shaped(cid, `${where}.originalCids[${offset}]`, FLIP_CID));
  for (let i = 1; i < originalCids.length; i++) {
    if (originalCids[i - 1] >= originalCids[i]) throw new Error(`${where}.originalCids must be sorted and distinct`);
  }
  const state = shaped(item.state, `${where}.state`, /^(qualified|queued-primary|queued-secondary|active|sold-private|cancelled|retired)$/) as DirectPoolOffer["state"];
  const queue = item.queue === undefined ? undefined : shaped(item.queue, `${where}.queue`, /^(primary|secondary)$/) as DirectPoolOffer["queue"];
  const queuePosition = item.queuePosition === undefined ? 0 : integer(item.queuePosition, `${where}.queuePosition`, MAX_ITEMS);
  if (queue !== (state === "queued-primary" ? "primary" : state === "queued-secondary" ? "secondary" : undefined) ||
      (queue === undefined) !== (queuePosition === 0)) {
    throw new Error(`${where} has inconsistent queue fields`);
  }
  const payout = decimal(item.payout, `${where}.payout`, AMOUNT_MAX);
  if (BigInt(payout) > DIRECT_POOL_PRICE_CAP) throw new Error(`${where}.payout exceeds the price cap`);
  return {
    offerId: shaped(item.offerId, `${where}.offerId`, HEX_32),
    author: shaped(item.author, `${where}.author`, HEX_20),
    epoch: integer(item.epoch, `${where}.epoch`, 65535), coveredFlips, originalCids,
    inventoryCommitment: shaped(item.inventoryCommitment, `${where}.inventoryCommitment`, HEX_32),
    keysCommitment: shaped(item.keysCommitment, `${where}.keysCommitment`, HEX_32),
    payout,
    sellerBond: decimal(item.sellerBond, `${where}.sellerBond`, AMOUNT_MAX),
    fraudCollateral: decimal(item.fraudCollateral, `${where}.fraudCollateral`, AMOUNT_MAX),
    nextNonce: decimal(item.nextNonce, `${where}.nextNonce`, UINT64_MAX),
    openOrderId: decimal(item.openOrderId, `${where}.openOrderId`, UINT64_MAX),
    state, everSold: flag(item.everSold, `${where}.everSold`),
    contentProven: flag(item.contentProven, `${where}.contentProven`),
    publicSeed: flag(item.publicSeed, `${where}.publicSeed`),
    ...(queue === undefined ? {} : { queue }), queuePosition,
    sequence: decimal(item.sequence, `${where}.sequence`, UINT64_MAX),
    listedAtBlock: decimal(item.listedAtBlock, `${where}.listedAtBlock`, UINT64_MAX),
  };
}

function parseOrder(value: unknown, index: number): DirectPoolOrder {
  const where = `orders[${index}]`;
  const item = object(value, where, ["orderId", "offerId", "buyer", "paid", "bond", "sellerPayout",
    "fundedAtBlock", "refundAtBlock", "nonce", "state", "challenged", "refundAvailable"]);
  const fundedAtBlock = decimal(item.fundedAtBlock, `${where}.fundedAtBlock`, UINT64_MAX);
  const refundAtBlock = decimal(item.refundAtBlock, `${where}.refundAtBlock`, UINT64_MAX);
  if (BigInt(refundAtBlock) < BigInt(fundedAtBlock)) throw new Error(`${where}.refundAtBlock is before funding`);
  const orderId = decimal(item.orderId, `${where}.orderId`, UINT64_MAX);
  if (orderId === "0") throw new Error(`${where}.orderId must be positive`);
  return {
    orderId, offerId: shaped(item.offerId, `${where}.offerId`, HEX_32),
    buyer: shaped(item.buyer, `${where}.buyer`, HEX_20),
    paid: decimal(item.paid, `${where}.paid`, AMOUNT_MAX),
    bond: decimal(item.bond, `${where}.bond`, AMOUNT_MAX),
    sellerPayout: decimal(item.sellerPayout, `${where}.sellerPayout`, AMOUNT_MAX),
    fundedAtBlock, refundAtBlock,
    nonce: decimal(item.nonce, `${where}.nonce`, UINT64_MAX),
    state: shaped(item.state, `${where}.state`, /^(funded|fallback-pending|settled|refunded)$/) as DirectPoolOrder["state"],
    challenged: flag(item.challenged, `${where}.challenged`),
    refundAvailable: flag(item.refundAvailable, `${where}.refundAvailable`),
  };
}

export function parseDirectPoolState(text: string): ParseResult {
  if (text.length > MAX_BYTES || new TextEncoder().encode(text).length > MAX_BYTES) {
    return { ok: false, error: "the file is larger than the interface accepts" };
  }
  try {
    const raw = object(JSON.parse(text), "the file", ["format", "version", "scope", "network", "contract",
      "codeHash", "blockHeight", "observedAt", "source", "pool", "offers", "orders"]);
    if (raw.format !== "fod.direct-pool-state" || raw.version !== 6 || raw.scope !== "live") {
      throw new Error("this is not a supported live pool state file");
    }
    if (raw.network !== "idena" || raw.source !== "node") {
      throw new Error("pool state has an unexpected network or source");
    }
    if (!Array.isArray(raw.offers) || !Array.isArray(raw.orders) ||
        raw.offers.length + raw.orders.length > MAX_ITEMS) {
      throw new Error("pool state has invalid or excessive entries");
    }
    const observedAt = shaped(raw.observedAt, "observedAt", /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$/);
    if (Number.isNaN(Date.parse(observedAt))) throw new Error("observedAt is not a readable date");
    const state: DirectPoolState = {
      format: "fod.direct-pool-state", version: 6, scope: "live", network: "idena",
      contract: shaped(raw.contract, "contract", HEX_20),
      codeHash: shaped(raw.codeHash, "codeHash", HEX_32),
      blockHeight: integer(raw.blockHeight, "blockHeight", Number.MAX_SAFE_INTEGER),
      observedAt, source: "node", pool: parsePool(raw.pool),
      offers: raw.offers.map(parseOffer), orders: raw.orders.map(parseOrder),
    };
    const offerIds = new Set<string>();
    const offersById = new Map<string, DirectPoolOffer>();
    const positions = new Set<number>();
    for (const offer of state.offers) {
      if (offerIds.has(offer.offerId)) throw new Error("pool state has duplicate offers");
      offerIds.add(offer.offerId);
      offersById.set(offer.offerId, offer);
      if (offer.queuePosition > 0) {
        if (positions.has(offer.queuePosition)) throw new Error("pool state has duplicate queue positions");
        positions.add(offer.queuePosition);
      }
    }
    const queueLength = state.pool.primaryUnits + state.pool.secondaryUnits;
    if (positions.size !== queueLength || queueLength > MAX_ITEMS) {
      throw new Error("pool state queue length disagrees with its unit counts");
    }
    for (const offer of state.offers) {
      if (offer.queuePosition > 0 &&
          (offer.queuePosition > queueLength || offer.queue !== (offer.queuePosition <= state.pool.primaryUnits ? "primary" : "secondary"))) {
        throw new Error("pool state queue position disagrees with its lane");
      }
    }
    if (queueLength > 0 && (state.pool.nextOfferId === undefined || offersById.get(state.pool.nextOfferId)?.queuePosition !== 1) ||
        queueLength === 0 && state.pool.nextOfferId !== undefined) {
      throw new Error("pool.nextOfferId disagrees with the current queue");
    }
    checkPoolMath(state.pool, state.pool.nextOfferId === undefined ? 0n : BigInt(offersById.get(state.pool.nextOfferId)!.payout));
    const orderIds = new Set<string>();
    const ordersById = new Map<string, DirectPoolOrder>();
    for (const order of state.orders) {
      if (orderIds.has(order.orderId) || !offerIds.has(order.offerId)) {
        throw new Error("pool state has a duplicate or orphan order");
      }
      orderIds.add(order.orderId);
      ordersById.set(order.orderId, order);
    }
    for (const offer of state.offers) {
      if (offer.state === "active") {
        const order = ordersById.get(offer.openOrderId);
        if (!order || order.offerId !== offer.offerId ||
            (order.state !== "funded" && order.state !== "fallback-pending")) {
          throw new Error("pool state active offer has no current order");
        }
        const expectedPayout = offer.everSold ? BigInt(order.paid) / 2n : BigInt(offer.payout);
        if (BigInt(order.paid) > DIRECT_POOL_PRICE_CAP ||
            (!offer.everSold && BigInt(order.paid) < BigInt(offer.payout)) ||
            BigInt(order.sellerPayout) !== expectedPayout) {
          throw new Error("pool state active order has an invalid price or author payout");
        }
      }
    }
    for (const order of state.orders) {
      if ((order.state === "funded" || order.state === "fallback-pending") &&
          (offersById.get(order.offerId)?.state !== "active" || offersById.get(order.offerId)?.openOrderId !== order.orderId)) {
        throw new Error("pool state current order disagrees with its offer");
      }
    }
    return { ok: true, state, warnings: [] };
  } catch (error) {
    return { ok: false, error: error instanceof SyntaxError ? "the file is not valid JSON" :
      error instanceof Error ? error.message : "the file could not be read" };
  }
}
