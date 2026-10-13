import { useState } from "react";
import { downloadActionRequest } from "../request";
import { requestJSON } from "../provider";

interface SharedNodeConfig {
  format: "fod.shared-node";
  version: 1;
  nodeId: string;
  url: string;
  author: string;
  custody: "self" | "hosted";
  note?: string;
}

interface SharedNodeStatus {
  format: string;
  version: number;
  nodeId: string;
  sessionId: string;
  utilityId: string;
  provenance: { author: string; epoch: number; blockRange: string };
  operations: string[];
  custody: string[];
  note: string;
}

type SharedNodeParse =
  | { ok: true; config: SharedNodeConfig }
  | { ok: false; error: string };

function parseSharedNode(text: string): SharedNodeParse {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return { ok: false, error: "the shared-node file is not valid JSON" };
  }
  if (typeof raw !== "object" || raw === null) {
    return { ok: false, error: "the shared-node file must contain an object" };
  }
  const record = raw as Record<string, unknown>;
  if (record.format !== "fod.shared-node") {
    return { ok: false, error: "this is not a shared-node connection file" };
  }
  if (record.version !== 1) {
    return { ok: false, error: "unsupported shared-node file version" };
  }
  for (const field of ["nodeId", "url", "author"] as const) {
    if (typeof record[field] !== "string" || record[field] === "") {
      return { ok: false, error: `the shared-node file is missing ${field}` };
    }
  }
  if (record.custody !== "self" && record.custody !== "hosted") {
    return { ok: false, error: "the shared-node file must name custody self or hosted" };
  }
  if (!/^0x[0-9a-fA-F]{40}$/.test(record.author as string)) {
    return { ok: false, error: "the shared-node author is not an address" };
  }
  let url: URL;
  try {
    url = new URL(String(record.url));
  } catch {
    return { ok: false, error: "the shared-node URL is unreadable" };
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    return { ok: false, error: "the shared-node URL must be http or https" };
  }
  return {
    ok: true,
    config: {
      format: "fod.shared-node",
      version: 1,
      nodeId: String(record.nodeId),
      url: url.origin,
      author: String(record.author).toLowerCase(),
      custody: record.custody,
      note: typeof record.note === "string" ? record.note : undefined,
    },
  };
}

/**
 * The author's shared-node journey: connect to a node that runs the Idena and
 * CLI work, and state which side holds the epoch vault. The reference node
 * publishes the session/pool status; listing and delivery still need the
 * author's nodekey or vault, which is exactly the custody boundary shown here.
 */
export function SharedNode() {
  const [config, setConfig] = useState<SharedNodeConfig | null>(null);
  const [status, setStatus] = useState<SharedNodeStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [draftNodeId, setDraftNodeId] = useState("my-node");
  const [draftUrl, setDraftUrl] = useState("http://127.0.0.1:8787");
  const [draftAuthor, setDraftAuthor] = useState("");
  const [draftCustody, setDraftCustody] = useState<"self" | "hosted">("hosted");
  const [draftError, setDraftError] = useState<string | null>(null);

  const importFile = async (file: File) => {
    if (file.size > 32 * 1024) {
      setError("the shared-node connection file is too large");
      setConfig(null);
      setStatus(null);
      return;
    }
    const parsed = parseSharedNode(await file.text());
    if (!parsed.ok) {
      setError(parsed.error);
      setConfig(null);
      setStatus(null);
      return;
    }
    setConfig(parsed.config);
    setError(null);
    setStatus(null);
    try {
      const loaded = await requestJSON<SharedNodeStatus>(`${parsed.config.url.replace(/\/$/, "")}/shared-node/status`);
      if (loaded === null || typeof loaded !== "object" ||
          loaded.format !== "fod.shared-node-status" || loaded.version !== 1 ||
          typeof loaded.nodeId !== "string" || !Array.isArray(loaded.operations) ||
          !loaded.operations.every((operation) => typeof operation === "string")) {
        setError("this node does not answer with the shared-node status this build reads");
        return;
      }
      setStatus(loaded);
    } catch {
      setError("the shared node could not be reached");
    }
  };

  const downloadDraft = () => {
    if (draftNodeId.trim() === "" || !/^0x[0-9a-fA-F]{40}$/.test(draftAuthor)) {
      setDraftError("the node id and a 0x author address are required");
      return;
    }
    let url: URL;
    try {
      url = new URL(draftUrl);
    } catch {
      setDraftError("the node URL is unreadable");
      return;
    }
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      setDraftError("the node URL must be http or https");
      return;
    }
    setDraftError(null);
    downloadActionRequest(
      {
        format: "fod.shared-node",
        version: 1,
        nodeId: draftNodeId.trim(),
        url: url.origin,
        author: draftAuthor.toLowerCase(),
        custody: draftCustody,
        note: "connection written by the Flips on Demand interface",
      },
      "fod-shared-node.json",
    );
  };

  return (
    <details className="shared-node">
      <summary>Sell through a shared node</summary>
      <p>
        A shared node runs the Idena node, the flip export and the delivery desk
        for an author. The custody choice decides who holds the epoch vault.
      </p>
      <div className="actions">
        <label className="import">
          <input
            type="file"
            accept="application/json,.json"
            aria-label="Import shared node"
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (file) {
                void importFile(file);
              }
              event.target.value = "";
            }}
          />
          <span>Import shared-node file</span>
        </label>
      </div>
      {error !== null && (
        <p className="banner" role="alert">
          {error}
        </p>
      )}
      {config !== null && (
        <dl className="facts">
          <div>
            <dt>Node</dt>
            <dd>{config.nodeId}</dd>
          </div>
          <div>
            <dt>Author</dt>
            <dd className="mono">{config.author}</dd>
          </div>
          <div>
            <dt>Custody</dt>
            <dd>{config.custody}</dd>
          </div>
          {status !== null && (
            <div>
              <dt>Node status</dt>
              <dd>
                {status.nodeId}, {status.operations.join(" + ")}
              </dd>
            </div>
          )}
        </dl>
      )}
      <p className="muted">
        <strong>hosted</strong>: the node holds the author's epoch vault and can
        deliver the flips; the escrow still pays the registered author, so the
        node cannot redirect the proceeds. <strong>self</strong>: the author's
        own node keeps the keys and the shared node only coordinates.
      </p>
      <ol className="gate-steps">
        <li>
          Run the node for this author in one step:{" "}
          <code>
            fod shared-node -vault FILE -inventory FILE -blob-dir DIR -out DIR -node-url URL
            -session-id ID -utility-id ID -threshold N -closes-at T
          </code>{" "}
          writes the export, the pool the gateway loads, and the connection file your
          customers import - with the custody choice stated in it.
        </li>
        <li>
          Build the registration input with <code>fod prepare-offer</code>, then list or
          donate the bundle with the sealed receipt: <code>fod prepare-list -pool -proof</code>.
        </li>
        <li>
          Keep the delivery desk running while the listing is live:{" "}
          <code>fod serve -contract ADDRESS -vault FILE</code>.
        </li>
      </ol>
      <p className="muted">
        The node never needs the customer's keys, and the author's proceeds
        still go to the registered author address in the escrow.
      </p>
      <h4>Create a shared-node file</h4>
      <dl className="facts">
        <div>
          <dt>Node id</dt>
          <dd>
            <input
              value={draftNodeId}
              onChange={(event) => setDraftNodeId(event.target.value)}
              aria-label="Shared node id"
            />
          </dd>
        </div>
        <div>
          <dt>Node URL</dt>
          <dd>
            <input
              value={draftUrl}
              onChange={(event) => setDraftUrl(event.target.value)}
              aria-label="Shared node URL"
            />
          </dd>
        </div>
        <div>
          <dt>Author address</dt>
          <dd>
            <input
              value={draftAuthor}
              onChange={(event) => setDraftAuthor(event.target.value)}
              aria-label="Shared node author"
              placeholder="0x..."
            />
          </dd>
        </div>
        <div>
          <dt>Custody</dt>
          <dd>
            <select
              value={draftCustody}
              onChange={(event) => setDraftCustody(event.target.value === "self" ? "self" : "hosted")}
              aria-label="Shared node custody"
            >
              <option value="hosted">hosted</option>
              <option value="self">self</option>
            </select>
          </dd>
        </div>
      </dl>
      {draftError !== null && (
        <p className="banner" role="alert">
          {draftError}
        </p>
      )}
      <div className="actions">
        <button type="button" className="primary" onClick={downloadDraft}>
          Download shared-node file
        </button>
      </div>
    </details>
  );
}
