// Validate a marketplace state file with the interface's own parser.
//
//   npx esbuild check-state.ts --bundle --platform=node --format=esm --outfile=/tmp/fod-check-state.mjs
//   node /tmp/fod-check-state.mjs path/to/state.json
//
// This uses the same parsers the page runs on imported snapshots.

import { readFileSync } from "node:fs";
import { parseMarketplaceState } from "./src/state";
import { parseDirectMarketState } from "./src/direct-state";

const path = process.argv[2];
if (!path) {
  console.error("usage: node check-state.mjs <state.json>");
  process.exit(2);
}

const text = readFileSync(path, "utf8");
let format: unknown;
try {
  format = JSON.parse(text)?.format;
} catch {
  format = undefined;
}
const result = format === "fod.direct-market-state"
  ? parseDirectMarketState(text)
  : parseMarketplaceState(text);
if (!result.ok) {
  console.error(`refused: ${result.error}`);
  process.exit(1);
}

if (result.state.format === "fod.direct-market-state") {
  console.log(`accepted: ${result.state.offers.length} direct offers, ${result.state.orders.length} direct orders`);
} else {
  console.log(
    `accepted: ${result.state.pool.length} pool listings, ` +
      `${result.state.poolOrders.length} pool orders, ${result.state.credits.length} credits` +
      (result.warnings.length ? `, ${result.warnings.length} warnings` : ""),
  );
}
for (const warning of result.warnings) {
  console.log(`warning: ${warning}`);
}
