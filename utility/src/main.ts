// Starts the reference utility.
//
//   node src/main.ts <policy.json> [port] [--host <address>] [--trusted-proxy <address>]... [--grants <file>]

import { loadPolicy } from "./policy.ts";
import { FileGrants } from "./access.ts";
import { createUtilityServer } from "./server.ts";

const policyPath = process.argv[2];
if (policyPath === undefined) {
  console.error("usage: node src/main.ts <policy.json> [port] [--host <address>] [--trusted-proxy <address>]... [--grants <file>]");
  process.exit(2);
}
const port = Number.parseInt(process.argv[3] ?? "8788", 10);
const trustedProxies: string[] = [];
let grantsPath = "";
// Loopback by default: serving the LAN is an explicit choice (`AH-9`).
let host = "127.0.0.1";
for (let index = 4; index < process.argv.length; index += 1) {
  const argument = process.argv[index];
  const value = process.argv[index + 1];
  if (argument === "--host" && value !== undefined) {
    host = value;
    index += 1;
    continue;
  }
  if (argument === "--trusted-proxy" && value !== undefined) {
    trustedProxies.push(value);
    index += 1;
    continue;
  }
  if (argument === "--grants" && value !== undefined) {
    grantsPath = value;
    index += 1;
    continue;
  }
  {
    console.error("usage: node src/main.ts <policy.json> [port] [--host <address>] [--trusted-proxy <address>]... [--grants <file>]");
    process.exit(2);
  }
}
const policy = await loadPolicy(policyPath);
const grants = grantsPath === "" ? undefined : await FileGrants.open(grantsPath);
const server = createUtilityServer({ policy, trustedProxies, ...(grants === undefined ? {} : { grants }) });
server.listen(port, host, () => {
  console.log(`utility ${policy.utilityId} listening on ${host}:${port}`);
  console.log(`accepts sessions: ${policy.sessions.join(", ")}`);
  console.log(`scope: ${policy.scope.statement}`);
  if (trustedProxies.length > 0) {
    console.log(`trusted proxies for rate limits: ${trustedProxies.join(", ")}`);
  }
  if (grantsPath !== "") {
    console.log(`grants persist in ${grantsPath} (0600)`);
  }
});
