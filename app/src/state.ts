// The public state the interface renders, matching `SPEC.md` 4.5.
//
// Everything here arrives from a file the user picked, so it is treated as
// untrusted: fields are validated, sizes are capped, and nothing is rendered as
// markup. A snapshot is for browsing only - it never authorises a payment.

export interface Credit {
  address: string;
  amount: string;
}

/**
 * One bundle's pool listing: the reserve against its sale units, both quotes,
 * the delivery beacon, and the buy nonce a purchase proof has to name. Epoch
 * and flip count are present when the snapshot's registration artefacts
 * supplied them.
 */
export interface PoolListing {
  bundle: string;
  fast?: boolean;
  author: string;
  manifestCommitment: string;
  epoch?: number;
  flipCount?: number;
  reserve: string;
  units: number;
  beaconBlock: number;
  strikes: number;
  nonce: number;
  buyQuote?: string;
  sellQuote?: string;
  availability?: PoolAvailability;
}

/** The last off-chain blob check someone published for this bundle. */
export interface PoolAvailability {
  checkedAt: string;
  source?: string;
  coveredFlips: number;
  verifiedFlips: number;
  complete: boolean;
}

/** One pool order: a buyer's payment and the delivery deadlines it runs on. */
export interface PoolOrder {
  orderId: number;
  bundle: string;
  buyer: string;
  paid: string;
  fundedAtBlock: number;
  deliveryDeadlineBlock: number;
  refundAfterBlock: number;
  state: "funded" | "delivered" | "settled" | "refunded";
  delivered?: boolean;
}

export interface MarketplaceState {
  format: "fod.marketplace-state";
  version: number;
  network: string;
  contract: string;
  codeHash?: string;
  blockHeight: number;
  /**
   * The node's observed seconds between the last two blocks, when the market
   * snapshot carries one. The interface turns a block deadline into a
   * wall-clock estimate with it and says the time depends on the chain when it
   * is absent.
   */
  blockSeconds?: number;
  observedAt: string;
  source: string;
  viewer?: { address: string; credits: string };
  credits: Credit[];
  pool: PoolListing[];
  poolOrders: PoolOrder[];
}

export type ParseResult =
  | { ok: true; state: MarketplaceState; warnings: string[] }
  | { ok: false; error: string };

/** Imported snapshots are browsing data, so keep them small. */
const MAX_STATE_BYTES = 5 * 1024 * 1024;
const MAX_ITEMS = 5000;
const MAX_SNAPSHOT_AGE_MS = 15 * 60 * 1000;
const MAX_CLOCK_SKEW_MS = 60 * 1000;
const HEX_32 = /^[0-9a-f]{64}$/;
const HEX_20 = /^[0-9a-f]{40}$/;
const DECIMAL = /^[0-9]{1,78}$/;

/** A snapshot may be browsed indefinitely, but old state cannot prepare actions. */
export function snapshotIsRecent(observedAt: string, now = Date.now()): boolean {
  const observed = Date.parse(observedAt);
  return Number.isFinite(observed) &&
    observed <= now + MAX_CLOCK_SKEW_MS &&
    now - observed <= MAX_SNAPSHOT_AGE_MS;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function requireString(value: unknown, field: string, pattern?: RegExp): string {
  if (typeof value !== "string") {
    throw new Error(`${field} must be a string`);
  }
  if (pattern && !pattern.test(value)) {
    throw new Error(`${field} has an unexpected form`);
  }
  return value;
}

function requireInteger(value: unknown, field: string, max = Number.MAX_SAFE_INTEGER): number {
  if (typeof value !== "number" || !Number.isInteger(value) || value < 0 || value > max) {
    throw new Error(`${field} must be a non-negative integer`);
  }
  return value;
}

function requireAmount(value: unknown, field: string): string {
  const amount = requireString(value, field);
  if (!DECIMAL.test(amount)) {
    throw new Error(`${field} must be an integer amount in base units`);
  }
  return amount;
}

function parseCredit(value: unknown, index: number): Credit {
  const where = `credits[${index}]`;
  if (!isRecord(value)) {
    throw new Error(`${where} must be an object`);
  }
  return {
    address: requireString(value.address, `${where}.address`, HEX_20),
    amount: requireAmount(value.amount, `${where}.amount`),
  };
}

function parsePoolListing(value: unknown, index: number): PoolListing {
  const where = `pool[${index}]`;
  if (!isRecord(value)) {
    throw new Error(`${where} must be an object`);
  }
  const listing: PoolListing = {
    bundle: requireString(value.bundle, `${where}.bundle`, HEX_32),
    author: requireString(value.author, `${where}.author`, HEX_20),
    manifestCommitment: requireString(value.manifestCommitment, `${where}.manifestCommitment`, HEX_32),
    reserve: requireAmount(value.reserve, `${where}.reserve`),
    units: requireInteger(value.units, `${where}.units`, 1_000_000_000),
    beaconBlock: requireInteger(value.beaconBlock, `${where}.beaconBlock`),
    strikes: requireInteger(value.strikes, `${where}.strikes`, 1_000_000),
    nonce: requireInteger(value.nonce, `${where}.nonce`),
  };
  if (value.fast !== undefined) {
    if (typeof value.fast !== "boolean") {
      throw new Error(`${where}.fast must be a boolean`);
    }
    listing.fast = value.fast;
  }
  if (value.epoch !== undefined) {
    listing.epoch = requireInteger(value.epoch, `${where}.epoch`, 1_000_000);
  }
  if (value.flipCount !== undefined) {
    listing.flipCount = requireInteger(value.flipCount, `${where}.flipCount`, 1_000_000);
  }
  if (value.buyQuote !== undefined) {
    listing.buyQuote = requireAmount(value.buyQuote, `${where}.buyQuote`);
  }
  if (value.sellQuote !== undefined) {
    listing.sellQuote = requireAmount(value.sellQuote, `${where}.sellQuote`);
  }
  if (value.availability !== undefined) {
    const item = value.availability;
    if (!isRecord(item)) {
      throw new Error(`${where}.availability must be an object`);
    }
    listing.availability = {
      checkedAt: requireString(item.checkedAt, `${where}.availability.checkedAt`),
      source: item.source === undefined ? undefined : requireString(item.source, `${where}.availability.source`),
      coveredFlips: requireInteger(item.coveredFlips, `${where}.availability.coveredFlips`, 1_000_000),
      verifiedFlips: requireInteger(item.verifiedFlips, `${where}.availability.verifiedFlips`, 1_000_000),
      complete: item.complete === true,
    };
  }
  return listing;
}

const POOL_ORDER_STATES = ["funded", "delivered", "settled", "refunded"] as const;

function parsePoolOrder(value: unknown, index: number): PoolOrder {
  const where = `poolOrders[${index}]`;
  if (!isRecord(value)) {
    throw new Error(`${where} must be an object`);
  }
  const state = requireString(value.state, `${where}.state`);
  if (!POOL_ORDER_STATES.includes(state as PoolOrder["state"])) {
    throw new Error(`${where}.state is not a known state`);
  }
  return {
    orderId: requireInteger(value.orderId, `${where}.orderId`),
    bundle: requireString(value.bundle, `${where}.bundle`, HEX_32),
    buyer: requireString(value.buyer, `${where}.buyer`, HEX_20),
    paid: requireAmount(value.paid, `${where}.paid`),
    fundedAtBlock: requireInteger(value.fundedAtBlock, `${where}.fundedAtBlock`),
    deliveryDeadlineBlock: requireInteger(value.deliveryDeadlineBlock, `${where}.deliveryDeadlineBlock`),
    refundAfterBlock: requireInteger(value.refundAfterBlock, `${where}.refundAfterBlock`),
    state: state as PoolOrder["state"],
    delivered: value.delivered === true,
  };
}

/**
 * Parses an imported snapshot. Anything unexpected is an error rather than a
 * partially rendered page, and unknown extra fields are reported as warnings.
 */
export function parseMarketplaceState(text: string): ParseResult {
  if (text.length > MAX_STATE_BYTES) {
    return { ok: false, error: "the file is larger than the interface accepts" };
  }
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return { ok: false, error: "the file is not valid JSON" };
  }
  try {
    if (!isRecord(raw)) {
      throw new Error("the file must contain an object");
    }
    if (raw.format !== "fod.marketplace-state") {
      throw new Error("this is not a marketplace state file");
    }
    if (raw.version !== 1) {
      throw new Error(`unsupported state version: ${String(raw.version)}`);
    }
    const credits = raw.credits;
    // A fresh pool has no listings, and the writer omits empty arrays.
    const pool = raw.pool ?? [];
    const poolOrders = raw.poolOrders ?? [];
    if (!Array.isArray(credits)) {
      throw new Error("credits must be an array");
    }
    if (!Array.isArray(pool) || !Array.isArray(poolOrders)) {
      throw new Error("pool and poolOrders must be arrays");
    }
    if (
      credits.length > MAX_ITEMS ||
      pool.length > MAX_ITEMS ||
      poolOrders.length > MAX_ITEMS
    ) {
      throw new Error("the snapshot holds more entries than the interface renders");
    }
    const state: MarketplaceState = {
      format: "fod.marketplace-state",
      version: 1,
      network: requireString(raw.network, "network"),
      contract: requireString(raw.contract, "contract", HEX_20),
      codeHash: raw.codeHash === undefined ? undefined : requireString(raw.codeHash, "codeHash"),
      blockHeight: requireInteger(raw.blockHeight, "blockHeight"),
      blockSeconds:
        raw.blockSeconds === undefined ? undefined : requireInteger(raw.blockSeconds, "blockSeconds", 3600),
      observedAt: requireString(raw.observedAt, "observedAt"),
      source: requireString(raw.source, "source"),
      credits: credits.map(parseCredit),
      pool: pool.map(parsePoolListing),
      poolOrders: poolOrders.map(parsePoolOrder),
    };
    if (raw.viewer !== undefined) {
      if (!isRecord(raw.viewer)) {
        throw new Error("viewer must be an object");
      }
      state.viewer = {
        address: requireString(raw.viewer.address, "viewer.address", HEX_20),
        credits: requireAmount(raw.viewer.credits, "viewer.credits"),
      };
    }
    const warnings: string[] = [];
    if (Number.isNaN(Date.parse(state.observedAt))) {
      warnings.push("the snapshot timestamp is not a readable date");
    }
    return { ok: true, state, warnings };
  } catch (error) {
    return { ok: false, error: error instanceof Error ? error.message : "the file could not be read" };
  }
}
