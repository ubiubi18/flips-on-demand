import { StrictMode, useCallback, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { parseMarketplaceState, snapshotIsRecent, type MarketplaceState } from "./state";
import { parseDirectMarketState, type DirectMarketState } from "./direct-state";
import { parseDirectPoolState, type DirectPoolState } from "./direct-pool-state";
import { formatTimestamp, shortHex } from "./format";
import { Market } from "./views/Market";
import { Trades } from "./views/Trades";
import { Faq } from "./views/Faq";
import { Session } from "./views/Session";
import { DirectGuide, DirectMarket, DirectTrades } from "./views/DirectMarket";
import { DirectPoolMarket } from "./views/DirectPool";

type ImportedState = MarketplaceState | DirectMarketState | DirectPoolState;
const MAX_IMPORT_BYTES = 20 * 1024 * 1024;

type Route =
  | { view: "market" }
  | { view: "trades" }
  | { view: "session" }
  | { view: "faq" };

function parseRoute(hash: string): Route {
  const path = hash.replace(/^#\/?/, "");
  const [head] = path.split("/");
  switch (head) {
    case "trades":
      return { view: "trades" };
    case "session":
      return { view: "session" };
    case "faq":
      return { view: "faq" };
    default:
      return { view: "market" };
  }
}

function App() {
  const [route, setRoute] = useState<Route>(() => parseRoute(window.location.hash));
  const [state, setState] = useState<ImportedState | null>(null);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [fileName, setFileName] = useState<string | null>(null);
  const [importRevision, setImportRevision] = useState(0);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const onHashChange = () => setRoute(parseRoute(window.location.hash));
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(timer);
  }, []);

  const importFile = useCallback(async (file: File) => {
    if (file.size > MAX_IMPORT_BYTES) {
      setState(null);
      setWarnings([]);
      setError("the file is larger than the interface accepts");
      setFileName(file.name);
      return;
    }
    const text = await file.text();
    let format: unknown;
    try {
      format = JSON.parse(text)?.format;
    } catch {
      format = undefined;
    }
    const result = format === "fod.direct-market-state"
      ? parseDirectMarketState(text)
      : format === "fod.direct-pool-state" ? parseDirectPoolState(text) : parseMarketplaceState(text);
    if (result.ok) {
      setState(result.state);
      setWarnings(result.warnings);
      setError(null);
      setFileName(file.name);
      setImportRevision((current) => current + 1);
      setNow(Date.now());
    } else {
      setState(null);
      setWarnings([]);
      setError(result.error);
      setFileName(file.name);
    }
  }, []);

  const canPrepare = state !== null && snapshotIsRecent(state.observedAt, now);
  const poolLoaded = state?.format === "fod.direct-pool-state";

  return (
    <>
      <header className="masthead">
        <div className="masthead-row">
          <h1>Flips on Demand</h1>
          <label className="import">
            <input
              type="file"
              accept="application/json,.json"
              aria-label="Import state"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) {
                  void importFile(file);
                }
                event.target.value = "";
              }}
            />
            <span>Import state</span>
          </label>
        </div>
        <nav className="tabs">
          <a href="#/market" aria-current={poolLoaded || route.view === "market" ? "page" : undefined}>
            Market
          </a>
          {!poolLoaded && <a href="#/trades" aria-current={route.view === "trades" ? "page" : undefined}>
            {state?.format === "fod.direct-market-state" ? "Trades" : "My trades"}
          </a>}
          {!poolLoaded && <a href="#/session" aria-current={route.view === "session" ? "page" : undefined}>
            Use flips
          </a>}
          {!poolLoaded && <a href="#/faq" aria-current={route.view === "faq" ? "page" : undefined}>
            {state?.format === "fod.direct-market-state" ? "How it works" : "FAQ"}
          </a>}
        </nav>
        <Provenance state={state} fileName={fileName} warnings={warnings} />
      </header>
      <main>
        {error !== null && (
          <p className="banner" role="alert">
            {error}
          </p>
        )}
        {state !== null && !canPrepare && (
          <p className="notice" role="status">
            {poolLoaded
              ? "This imported snapshot is older than 15 minutes or its timestamp is invalid. Current quotes are hidden; verify chain state before trading."
              : "This snapshot is older than 15 minutes or its timestamp is invalid. Import fresh state before relying on it."}
          </p>
        )}
        {poolLoaded && <DirectPoolMarket key={importRevision} state={state} snapshotFresh={canPrepare} />}
        {!poolLoaded && route.view === "market" && (state?.format === "fod.direct-market-state"
          ? <DirectMarket key={importRevision} state={state} />
          : <Market key={importRevision} state={state} canPrepare={canPrepare} />)}
        {!poolLoaded && route.view === "trades" && (state?.format === "fod.direct-market-state"
          ? <DirectTrades key={importRevision} state={state} />
          : <Trades key={importRevision} state={state} />)}
        {!poolLoaded && route.view === "session" && <Session />}
        {!poolLoaded && route.view === "faq" && (state?.format === "fod.direct-market-state" ? <DirectGuide version={state.version} /> : <Faq />)}
      </main>
    </>
  );
}

function Provenance({
  state,
  fileName,
  warnings,
}: {
  state: ImportedState | null;
  fileName: string | null;
  warnings: string[];
}) {
  if (!state) {
    return (
      <div className="provenance" role="status" aria-live="polite" aria-atomic="true">
        <span className="muted">No state loaded</span>
        {fileName !== null && <span className="muted">{fileName}</span>}
      </div>
    );
  }
  return (
    <div className="provenance" role="status" aria-live="polite" aria-atomic="true">
      <span>{state.network}</span>
      <span title={`contract ${state.contract}`}>marketplace {shortHex(state.contract)}</span>
      <span>block {state.blockHeight.toLocaleString()}</span>
      <span>{formatTimestamp(state.observedAt)}</span>
      <span title={state.format === "fod.direct-pool-state" ? "The app has not verified this file against the chain" : state.source}>
        {state.format === "fod.direct-pool-state" ? "Imported file · unverified" : state.source}
      </span>
      {warnings.map((warning) => (
        <span key={warning} className="warning">
          {warning}
        </span>
      ))}
    </div>
  );
}

const container = document.getElementById("root");
if (!container) {
  throw new Error("missing root element");
}
createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
