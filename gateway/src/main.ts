// Starts the provider gateway from a configuration file.
//
//   node src/main.ts <config.json>
//
// The gateway serves one pool, one provider identity, and one signing key. It
// never holds node wallet material and never forwards arbitrary RPC.

import { loadConfig } from "./config.ts";
import { loadOrCreateApplicationKey } from "./keys.ts";
import { loadPool } from "./pool.ts";
import { SessionState } from "./session.ts";
import { createGatewayServer } from "./http.ts";

async function main(): Promise<void> {
  const configPath = process.argv[2];
  if (configPath === undefined) {
    console.error("usage: node src/main.ts <config.json>");
    process.exit(2);
  }
  const config = await loadConfig(configPath);
  const pool = await loadPool(config.poolPath);
  const applicationKey = await loadOrCreateApplicationKey(config.applicationKeyPath, config.providerId);
  const session = new SessionState(pool, config.sessionTtlSeconds);
  const server = createGatewayServer({
    config,
    pool,
    session,
    applicationKey,
    marketplaceState: () => ({
      providerId: config.providerId,
      sessionId: pool.sessionId,
      utilityId: pool.utilityId,
      provenance: pool.provenance,
    }),
    now: () => Date.now(),
  });
  server.listen(config.port, config.host, () => {
    console.log(`gateway for ${config.providerId} listening on ${config.host}:${config.port}`);
    console.log(`session ${pool.sessionId} with ${pool.challenges.length} challenges`);
  });
}

await main();
