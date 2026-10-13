import { useState } from "react";
import { downloadActionRequest } from "../request";

type Recipe = {
  id: string;
  title: string;
  what: string;
  utility: string;
  endpoint: string;
};

const RECIPES: Recipe[] = [
  {
    id: "faucet",
    title: "Testnet faucet",
    what: "A faucet pays testnet funds once per participant. The gate makes the participant solve the flips before the faucet pays out.",
    utility: "testnet-faucet",
    endpoint: "your faucet's claim endpoint",
  },
  {
    id: "dapp",
    title: "Dapp access",
    what: "A dapp opens a session or a role to a wallet. The gate makes the participant solve the flips before the dapp opens it.",
    utility: "dapp-access",
    endpoint: "your dapp's access endpoint",
  },
];

/**
 * The operator's short path from a flip bundle to a guarded use case. The
 * marketplace sells the content, the provider runs the session, and the
 * utility decides what a passing result unlocks; this keeps those three roles
 * visible instead of hiding them behind one "gate" button.
 */
export function GateSetup() {
  const [recipeId, setRecipeId] = useState(RECIPES[0].id);
  const [providerId, setProviderId] = useState("my-gate");
  const [utilityId, setUtilityId] = useState(RECIPES[0].utility);
  const [threshold, setThreshold] = useState(2);
  const [gatewayKey, setGatewayKey] = useState("");
  const recipe = RECIPES.find((entry) => entry.id === recipeId) ?? RECIPES[0];

  const chooseRecipe = (next: Recipe) => {
    setRecipeId(next.id);
    setUtilityId(next.utility);
  };

  // The two hosts the connection file points at. They are named in the note as
  // well, so a customer reading the file sees where the answers go without
  // having to parse the fields.
  const gatewayUrl = "http://127.0.0.1:8787";
  const utilityUrl = "http://127.0.0.1:8788";
  const connection = {
    format: "fod.provider-config",
    version: 1,
    providerId,
    network: "idena-local",
    gatewayUrl,
    utilityUrl,
    protocol: "fod.session.v1",
    // A pin must be distributed through a trusted connection-file channel.
    // Without one, customers may inspect a session but cannot verify or claim.
    ...(gatewayKey === "" ? {} : { publicKeyHex: gatewayKey.trim().toLowerCase() }),
    note: `connection for ${recipe.title}; gateway ${gatewayUrl} runs the session, utility ${utilityUrl} decides the grant - replace both with your own hosts when you publish it`,
  };
  const gatewayConfig = {
    port: 8787,
    providerId,
    publicOrigin: gatewayUrl,
    origins: ["null"],
    sessionTtlSeconds: 1800,
    bodyLimitBytes: 65536,
    rateLimit: { windowSeconds: 60, maxRequests: 120 },
    poolPath: "pool.json",
    applicationKeyPath: "provider-application-key.json",
  };

  return (
    <details className="gate-setup">
      <summary>Set up a gate for your use case</summary>
      <p>
        The marketplace sells the flips, the provider runs the session, and your
        utility decides what a passing result unlocks. This is the shortest
        end-to-end path.
      </p>
      <div className="actions">
        {RECIPES.map((entry) => (
          <button
            key={entry.id}
            type="button"
            className={entry.id === recipe.id ? "primary" : undefined}
            onClick={() => chooseRecipe(entry)}
          >
            {entry.title}
          </button>
        ))}
      </div>
      <p className="muted">{recipe.what}</p>
      <ol className="gate-steps">
        <li>
          Get the flip bundle: buy one in <a href="#/market">Market</a> or author
          your own, then export it with <code>fod export</code>.
        </li>
        <li>
          Build the provider pool:{" "}
          <code>
            fod session-pool -export-dir &lt;export dir&gt; -out pool.json
            -session-id &lt;id&gt; -utility-id {utilityId} -threshold {threshold}
            -closes-at &lt;RFC3339&gt;
          </code>
        </li>
        <li>
          Point the gateway's <code>poolPath</code> at <code>pool.json</code> and
          start it: <code>node gateway/src/main.ts gateway-config.json</code>.
        </li>
        <li>
          Start the utility with its policy:{" "}
          <code>node utility/src/main.ts utility-policy.json &lt;port&gt;</code>,
          and let it grant {recipe.endpoint} on a passing result.
        </li>
        <li>
          Share the connection file with your customers. They import it under{" "}
          <a href="#/session">Use flips</a> and solve the challenges.
        </li>
      </ol>
      <dl className="facts">
        <div>
          <dt>Provider id</dt>
          <dd>
            <input
              value={providerId}
              onChange={(event) => setProviderId(event.target.value)}
              aria-label="Provider id"
            />
          </dd>
        </div>
        <div>
          <dt>Utility id</dt>
          <dd>
            <input
              value={utilityId}
              onChange={(event) => setUtilityId(event.target.value)}
              aria-label="Utility id"
            />
          </dd>
        </div>
        <div>
          <dt>Answers needed to pass</dt>
          <dd>
            <input
              type="number"
              min={1}
              value={threshold}
              onChange={(event) => setThreshold(Math.max(1, Number(event.target.value) || 1))}
              aria-label="Answers needed to pass"
            />
          </dd>
        </div>
        <div>
          <dt>Gateway key</dt>
          <dd>
            <input
              value={gatewayKey}
              onChange={(event) => setGatewayKey(event.target.value)}
              placeholder="88-hex SPKI key from GET /provider/key"
              aria-label="Gateway key"
            />
          </dd>
        </div>
      </dl>
      <div className="actions">
        <button
          type="button"
          className="primary"
          onClick={() => downloadActionRequest(connection, "fod-connection.json")}
        >
          Download connection file
        </button>
        <button
          type="button"
          onClick={() => downloadActionRequest(gatewayConfig, "fod-gateway-config.json")}
        >
          Download gateway config
        </button>
      </div>
      <p className="muted">
        The example gate in this build is the reference provider at
        http://127.0.0.1:8787; start it with{" "}
        <code>node gateway/src/main.ts gateway/example-config.json</code>.
      </p>
    </details>
  );
}
