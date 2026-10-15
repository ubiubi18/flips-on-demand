import { useEffect, useRef, useState } from "react";
import { DIRECT_POOL_PRICE_CAP } from "../direct-pool-state";
import type { DirectPoolOffer, DirectPoolState } from "../direct-pool-state";
import { formatAmount, shortHex } from "../format";

const PAGE_SIZE = 50;
const INITIAL_OFFER_COUNT = 10;
const MAX_PREVIEW_BYTES = 20 * 1024 * 1024;
const MAX_PICTURE_BYTES = 1024 * 1024;
const PREVIEW_TIMEOUT_MS = 12_000;

async function publicFlipPictures(cid: string, signal: AbortSignal): Promise<Blob[]> {
  const response = await fetch(`https://api.idena.io/api/flip/${encodeURIComponent(cid)}/content`, {
    signal, credentials: "omit", redirect: "error", referrerPolicy: "no-referrer",
  });
  if (!response.ok || !response.body) throw new Error("public preview is unavailable");
  const lengthHeader = response.headers.get("content-length");
  if (lengthHeader !== null && Number(lengthHeader) > MAX_PREVIEW_BYTES) {
    await response.body.cancel();
    throw new Error("public preview is too large");
  }
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > MAX_PREVIEW_BYTES) {
      await reader.cancel();
      throw new Error("public preview is too large");
    }
    chunks.push(value);
  }
  const body = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  const data: unknown = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(body));
  if (typeof data !== "object" || data === null || !("result" in data)) throw new Error("invalid public preview");
  if ("error" in data && data.error) throw new Error("public preview is unavailable");
  const result = data.result;
  if (result === null) return [];
  if (typeof result !== "object" || !("pics" in result) || !Array.isArray(result.pics) || result.pics.length > 8) {
    throw new Error("invalid public preview");
  }
  return result.pics.map((picture: unknown) => {
    if (typeof picture !== "string" || !picture.startsWith("0x") || picture.length < 18 ||
        (picture.length - 2) % 2 !== 0 || (picture.length - 2) / 2 > MAX_PICTURE_BYTES ||
        !/^[0-9a-f]+$/i.test(picture.slice(2))) throw new Error("invalid public preview image");
    const bytes = new Uint8Array((picture.length - 2) / 2);
    for (let index = 0; index < bytes.length; index++) {
      bytes[index] = Number.parseInt(picture.slice(2 + index * 2, 4 + index * 2), 16);
    }
    if (bytes[0] !== 0xff || bytes[1] !== 0xd8 || bytes[2] !== 0xff ||
        bytes[bytes.length - 2] !== 0xff || bytes[bytes.length - 1] !== 0xd9) {
      throw new Error("invalid public preview image");
    }
    return new Blob([bytes], { type: "image/jpeg" });
  });
}

type Preview = { kind: "idle" | "loading" | "empty" | "error" } | { kind: "ready"; urls: string[] };

function PublicFlipPreview({ cid, autoLoad = false, compact = false }: { cid: string; autoLoad?: boolean; compact?: boolean }) {
  const [preview, setPreview] = useState<Preview>({ kind: "idle" });
  const controller = useRef<AbortController | null>(null);
  const urls = useRef<string[]>([]);
  useEffect(() => () => {
    controller.current?.abort();
    controller.current = null;
    urls.current.forEach(URL.revokeObjectURL);
  }, []);

  async function load() {
    controller.current?.abort();
    const request = new AbortController();
    controller.current = request;
    let timedOut = false;
    const timeout = window.setTimeout(() => {
      timedOut = true;
      request.abort();
    }, PREVIEW_TIMEOUT_MS);
    setPreview({ kind: "loading" });
    try {
      const pictures = await publicFlipPictures(cid, request.signal);
      if (request.signal.aborted || controller.current !== request) return;
      if (pictures.length === 0) {
        setPreview({ kind: "empty" });
        return;
      }
      urls.current = pictures.map((picture) => URL.createObjectURL(picture));
      setPreview({ kind: "ready", urls: urls.current });
    } catch {
      if (controller.current === request && (timedOut || !request.signal.aborted)) setPreview({ kind: "error" });
    } finally {
      window.clearTimeout(timeout);
      if (controller.current === request) controller.current = null;
    }
  }

  useEffect(() => {
    if (autoLoad) void load();
  }, [autoLoad, cid]);

  function imageFailed() {
    urls.current.forEach(URL.revokeObjectURL);
    urls.current = [];
    setPreview({ kind: "error" });
  }

  return <div className={compact ? "pool-preview pool-preview-compact" : "pool-preview"}>
    {preview.kind !== "ready" && <button className="pool-more" type="button" disabled={preview.kind === "loading"} onClick={() => void load()}>
      {preview.kind === "loading" ? "Loading…" : preview.kind === "idle" ? compact ? "Preview" : "Show public images" : compact ? "Retry" : "Retry public images"}
    </button>}
    {preview.kind === "empty" && <p className="muted">{compact ? "Unavailable" : "No public preview is available."}</p>}
    {preview.kind === "error" && <p className="muted">{compact ? "Unavailable" : "Public preview could not be loaded."}</p>}
    {preview.kind === "ready" && <>
      {!compact && <p className="muted">Public images from Idena explorer; the complete flip may remain encrypted.</p>}
      <div className="pool-preview-images">
        {(compact ? preview.urls.slice(0, 2) : preview.urls).map((url, index) => <img key={url} src={url} alt={`Public flip image ${index + 1}`} onError={imageFailed} />)}
      </div>
    </>}
  </div>;
}

function FullId({ value }: { value: string }) {
  return <details className="pool-id">
    <summary className="mono">{shortHex(value)}</summary>
    <code>{value}</code>
  </details>;
}

function currentAmount(value: string | undefined, snapshotFresh: boolean): string {
  if (!snapshotFresh) return "Unavailable (stale snapshot)";
  return value === undefined ? "Unavailable" : `${formatAmount(value)} iDNA`;
}

type CurveState = { units: bigint; reserve: bigint };
type CurvePoint = { units: bigint; price: bigint; current: boolean };
const MAX_AMOUNT = (1n << 128n) - 1n;

function buyFrom({ units, reserve }: CurveState): (CurveState & { price: bigint }) | null {
  if (units <= 1n || reserve === 0n) return null;
  const divisor = units - 1n;
  const rawPrice = (reserve + divisor - 1n) / divisor;
  const price = rawPrice < DIRECT_POOL_PRICE_CAP ? rawPrice : DIRECT_POOL_PRICE_CAP;
  if (price === 0n || reserve + price > MAX_AMOUNT) return null;
  return { units: units - 1n, reserve: reserve + price, price };
}

function sellInto({ units, reserve }: CurveState): CurveState | null {
  if (units === 0n || reserve === 0n) return null;
  const rawPayout = reserve / (units + 1n);
  const payout = rawPayout < DIRECT_POOL_PRICE_CAP ? rawPayout : DIRECT_POOL_PRICE_CAP;
  if (payout === 0n || reserve === payout) return null;
  return { units: units + 1n, reserve: reserve - payout };
}

function nearbyCurve(state: DirectPoolState): CurvePoint[] {
  const center = { units: BigInt(state.pool.primaryUnits + state.pool.secondaryUnits), reserve: BigInt(state.pool.virtualReserve) };
  const states: CurveState[] = [center];
  let lower = center;
  for (let step = 0; step < 2; step++) {
    const next = buyFrom(lower);
    if (next === null) break;
    lower = next;
    states.unshift(lower);
  }
  let upper = center;
  for (let step = 0; step < 2; step++) {
    const next = sellInto(upper);
    if (next === null) break;
    upper = next;
    states.push(upper);
  }
  return states.flatMap((pool) => {
    const next = buyFrom(pool);
    return next === null ? [] : [{ units: pool.units, price: next.price, current: pool.units === center.units }];
  });
}

function PriceCurve({ state }: { state: DirectPoolState }) {
  const points = nearbyCurve(state);
  const prices = points.map((point) => point.price);
  const low = prices.reduce((min, value) => value < min ? value : min, prices[0] ?? 0n);
  const high = prices.reduce((max, value) => value > max ? value : max, prices[0] ?? 0n);
  const coordinates = points.map((point, index) => ({
    x: points.length === 1 ? 160 : 16 + index * 288 / (points.length - 1),
    y: high === low ? 48 : 76 - Number(point.price - low) / Number(high - low) * 56,
  }));
  return <section className="pool-curve" aria-labelledby="pool-curve-title">
    <h3 id="pool-curve-title">Nearby price curve</h3>
    <p className="muted">Illustrative curve prices. The next actual quote can be higher to cover the oldest seller's locked payout.</p>
    {points.length === 0 ? <p className="muted">No nearby buy price is available.</p> : <>
      {points.length > 1 && <svg className="pool-curve-chart" viewBox="0 0 320 92" aria-hidden="true">
        <polyline points={coordinates.map(({ x, y }) => `${x},${y}`).join(" ")} fill="none" stroke="currentColor" strokeWidth="2" />
        {coordinates.map(({ x, y }, index) => <circle key={points[index].units.toString()} cx={x} cy={y} r={points[index].current ? 5 : 3} fill="currentColor" />)}
      </svg>}
      <dl className="pool-curve-values">
        {points.map((point) => <div key={point.units.toString()}>
          <dt>{point.units.toString()} units{point.current ? " · snapshot" : ""}</dt>
          <dd>{formatAmount(point.price.toString())} iDNA</dd>
        </div>)}
      </dl>
    </>}
  </section>;
}

function OriginalCids({ offer }: { offer: DirectPoolOffer }) {
  const [open, setOpen] = useState(false);
  return (
    <details className="pool-flips" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary>View {offer.originalCids.length} {offer.originalCids.length === 1 ? "flip" : "flips"}</summary>
      {open && <ul className="pool-cids">
        {offer.originalCids.map((cid, index) => (
          <li key={cid}>
            <a href={`https://scan.idena.io/flip/${cid}`} target="_blank" rel="noopener noreferrer">Flip {index + 1} on explorer</a>
            <code>{cid}</code>
            <PublicFlipPreview cid={cid} />
          </li>
        ))}
      </ul>}
    </details>
  );
}

function OfferList({ offers, queued, searching }: { offers: DirectPoolOffer[]; queued: boolean; searching: boolean }) {
  const [visibleCount, setVisibleCount] = useState(INITIAL_OFFER_COUNT);
  return <>
    {offers.length === 0 ? <p className="pool-empty">
      {searching ? "No matching offers." : queued ? "No bundles are waiting in the queue." : "No other offers in this snapshot."}
    </p> : <ol className={queued ? "pool-offers pool-queue" : "pool-offers"}>
      {offers.slice(0, visibleCount).map((offer) => (
        <li className="pool-offer-card" key={offer.offerId}>
          <div className="pool-offer-art">
            <span className="pool-offer-art-mark">{queued ? String(offer.queuePosition).padStart(2, "0") : "·"}</span>
            <PublicFlipPreview cid={offer.originalCids[0]} compact />
          </div>
          <div className="pool-offer-body">
            <div className="pool-offer-topline">
              <span className={offer.queue === "secondary" || offer.everSold ? "pool-tag pool-tag-secondary" : "pool-tag"}>
                {offer.publicSeed ? "Public seed" : offer.queue === "secondary" ? "Resale" : "First sale"}
              </span>
              <span className="pool-position">{queued ? "Queue " + offer.queuePosition : offer.state}</span>
            </div>
            <h4>{offer.coveredFlips} {offer.coveredFlips === 1 ? "flip" : "flips"} · epoch {offer.epoch}</h4>
            <div className="pool-offer-byline">Author <FullId value={offer.author} /> · Offer <FullId value={offer.offerId} /></div>
            <div className="pool-offer-foot">
              <span>{offer.contentProven ? "Content proof recorded" : "Content proof pending"}</span>
              <OriginalCids offer={offer} />
            </div>
          </div>
        </li>
      ))}
    </ol>}
    {offers.length > visibleCount && <p className="pool-count">
      Showing {visibleCount} of {offers.length}.{" "}
      <button className="pool-more" type="button" onClick={() => setVisibleCount(Math.min(visibleCount + PAGE_SIZE, offers.length))}>Show 50 more</button>
    </p>}
  </>;
}

export function DirectPoolMarket({ state, snapshotFresh }: { state: DirectPoolState; snapshotFresh: boolean }) {
  const [visibleOrders, setVisibleOrders] = useState(PAGE_SIZE);
  const [search, setSearch] = useState("");
  const [showFeaturedPreview, setShowFeaturedPreview] = useState(false);
  const needle = search.trim().toLowerCase().replace(/^0x/, "");
  const matchingOffers = needle === "" ? state.offers : state.offers.filter((offer) =>
    offer.offerId.includes(needle) || offer.author.includes(needle) || offer.originalCids.some((cid) => cid.includes(needle)));
  const queued = matchingOffers.filter((offer) => offer.queuePosition > 0)
    .sort((left, right) => left.queuePosition - right.queuePosition);
  const otherOffers = matchingOffers.filter((offer) => offer.queuePosition === 0);
  const nextOffer = state.offers.find((offer) => offer.offerId === state.pool.nextOfferId);
  const nextAuthorPayout = nextOffer === undefined || state.pool.buyQuote === undefined ? undefined :
    nextOffer.queue === "secondary" ? (BigInt(state.pool.buyQuote) / 2n).toString() : nextOffer.payout;
  const orders = state.orders.slice().sort((left, right) => {
    const a = BigInt(left.orderId);
    const b = BigInt(right.orderId);
    return a === b ? 0 : a > b ? -1 : 1;
  });
  const challengedOrders = orders.filter((order) => order.challenged && order.state !== "settled" && order.state !== "refunded").length;
  return (
    <section className="pool-market">
      <header className="pool-hero">
        <div>
          <p className="pool-eyebrow">FLIPS ON DEMAND <span aria-hidden="true">/</span> ON-CHAIN MARKET</p>
          <h2>Explore original flips.</h2>
          <p className="pool-hero-description">Explore the queue, see public flip images, and follow each order on-chain.</p>
        </div>
        <div className="pool-hero-quote">
          <span>Next bundle at snapshot</span>
          <strong>{currentAmount(state.pool.buyQuote, snapshotFresh)}</strong>
          <small>{nextOffer?.publicSeed ? "Public seed · settles on purchase" : "Buyer bond and network fees are additional"}</small>
        </div>
      </header>

      <div className="pool-snapshot-line">
        <span className="pool-live-dot" aria-hidden="true" />
        <span>Imported chain snapshot · block {state.blockHeight.toLocaleString()}</span>
        <span>{state.pool.primaryUnits} first sales · {state.pool.secondaryUnits} resales</span>
        <span>Read only</span>
      </div>

      {state.pool.reserve === "0" && <p className="notice">
        Pool not yet open. {state.pool.seedCount} of 2 original bundles have been independently proved and publicly seeded.
        The 10,000 iDNA starting liquidity can be added only after both are ready.
      </p>}
      {state.pool.buyPausedForBond && <p className="notice">
        {snapshotFresh ? "Buying is paused" : "Buying was paused at this snapshot's block"}: the seller bond for{" "}
        {state.pool.nextOfferId === undefined ? "the queue head" : <>queue head <code className="mono">{state.pool.nextOfferId}</code></>} needs{" "}
        {state.pool.bondTopUpRequired === undefined ? "a top-up" : formatAmount(state.pool.bondTopUpRequired) + " iDNA more"}.
      </p>}

      <div className="pool-layout">
        <div className="pool-catalog">
          <div className="pool-section-heading">
            <div><p className="pool-eyebrow">FIRST IN LINE</p><h3>Next available bundle</h3></div>
            {!showFeaturedPreview && nextOffer && <button className="pool-preview-toggle" type="button" onClick={() => setShowFeaturedPreview(true)}>
              Show public preview
            </button>}
          </div>
          {nextOffer ? <article className="pool-featured">
            <div className="pool-featured-image">
              <span className="pool-featured-number">01 / PUBLIC FLIP IMAGES</span>
              <PublicFlipPreview key={nextOffer.originalCids[0]} cid={nextOffer.originalCids[0]} autoLoad={showFeaturedPreview} compact />
            </div>
            <div className="pool-featured-body">
              <span className={nextOffer.everSold || nextOffer.publicSeed ? "pool-tag pool-tag-secondary" : "pool-tag"}>
                {nextOffer.publicSeed ? "Public seed" : nextOffer.everSold ? "Secondary sale" : "First sale"}
              </span>
              <h4>{nextOffer.coveredFlips} {nextOffer.coveredFlips === 1 ? "original flip" : "original flips"}</h4>
              <p>Epoch {nextOffer.epoch} · Author <FullId value={nextOffer.author} /> · Offer <FullId value={nextOffer.offerId} /></p>
              <div className="pool-featured-amount"><span>Current snapshot quote</span><strong>{currentAmount(state.pool.buyQuote, snapshotFresh)}</strong></div>
              <p className="pool-featured-proof">{nextOffer.contentProven ? "Independent content proof recorded" : "Content proof has not been recorded"}</p>
              <OriginalCids offer={nextOffer} />
            </div>
          </article> : <p className="pool-empty">No bundle is available at this snapshot.</p>}
          <p className="pool-explorer-note">Public previews come from Idena explorer after you request them. They show only the public part of a flip.</p>

          <div className="pool-section-heading pool-browse-heading" id="pool-lookup">
            <div><p className="pool-eyebrow">THE QUEUE</p><h3>Browse bundles</h3></div>
            <span>{queued.length} waiting</span>
          </div>
          <div className="pool-lookup">
            <label htmlFor="pool-offer-search">Find offer</label>
            <input id="pool-offer-search" type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Offer, author, or flip CID" />
          </div>
          <p className="pool-queue-rule">Never-sold bundles go first, oldest listing first. Resales follow in their own queue.</p>
          <OfferList offers={queued} queued searching={needle !== ""} />
          {otherOffers.length > 0 && <details className="pool-other-offers">
            <summary>Other public offers ({otherOffers.length})</summary>
            <OfferList offers={otherOffers} queued={false} searching={needle !== ""} />
          </details>}
        </div>

        <aside className="pool-aside" aria-label="Market details">
          <PriceCurve state={state} />
          <div className="pool-economics">
            <h3>Pool at a glance</h3>
            <dl>
              <div><dt>iDNA reserve</dt><dd>{formatAmount(state.pool.reserve)}</dd></div>
              <div><dt>Buyer bond</dt><dd>{currentAmount(state.pool.minimumBuyerBond, snapshotFresh)}</dd></div>
              <div><dt>Total to lock</dt><dd>{currentAmount(state.pool.minimumBuyerTotal, snapshotFresh)}</dd></div>
              <div><dt>{nextOffer?.queue === "secondary" ? "Author dividend" : "Author payout"}</dt><dd>{currentAmount(nextAuthorPayout, snapshotFresh)}</dd></div>
              <div><dt>New primary listing quote</dt><dd>{currentAmount(state.pool.nextSellerPayout, snapshotFresh)}</dd></div>
            </dl>
          </div>
          <p className="pool-aside-note">This v6 snapshot can differ from current chain state. Prices, fees and bonds must be checked again before signing. Repeat-sale payments split 50% to the author and 50% to the pool.</p>
        </aside>
      </div>

      <section className="pool-order-section" id="pool-orders">
        <div className="pool-section-heading">
          <div><p className="pool-eyebrow">AFTER PURCHASE</p><h3>Orders & disputes</h3></div>
          <span>{challengedOrders > 0 ? challengedOrders + " challenged" : orders.length + " orders"}</span>
        </div>
        <p className="pool-order-intro">Amounts were fixed when each order was funded. Challenge and refund status come from this snapshot.</p>
        <div className="table-wrap pool-orders">
          <table className="market stack">
            <thead><tr><th>Order</th><th>Offer</th><th>Status</th><th className="numeric">Buyer paid</th><th className="numeric">Buyer bond</th><th className="numeric">Author payout</th><th>Proof / dispute</th><th className="numeric">Refund from block</th></tr></thead>
            <tbody>
              {orders.slice(0, visibleOrders).map((order) => (
                <tr key={order.orderId}>
                  <td><span className="field-label">Order</span>{order.orderId}</td>
                  <td><span className="field-label">Offer</span><FullId value={order.offerId} /></td>
                  <td><span className="field-label">Status</span>{order.state === "fallback-pending" ? "Proof accepted; reveal pending" : order.state}</td>
                  <td className="numeric"><span className="field-label">Buyer paid</span>{formatAmount(order.paid)} iDNA</td>
                  <td className="numeric"><span className="field-label">Buyer bond</span>{formatAmount(order.bond)} iDNA</td>
                  <td className="numeric"><span className="field-label">Author payout</span>{formatAmount(order.sellerPayout)} iDNA</td>
                  <td><span className="field-label">Proof / dispute</span>{order.challenged ? "Buyer challenged" : "No challenge recorded"}</td>
                  <td className="numeric"><span className="field-label">Refund from block</span>{order.refundAvailable ? "Available" : order.state === "funded" || order.state === "fallback-pending" ? order.refundAtBlock : "—"}</td>
                </tr>
              ))}
              {orders.length === 0 && <tr><td colSpan={8} className="muted">No current orders in this snapshot.</td></tr>}
            </tbody>
          </table>
        </div>
        {orders.length > visibleOrders && <p className="pool-count">
          Showing {visibleOrders} of {orders.length}.{" "}
          <button className="pool-more" type="button" onClick={() => setVisibleOrders(Math.min(visibleOrders + PAGE_SIZE, orders.length))}>Show 50 more</button>
        </p>}
      </section>
      <p className="pool-experimental">Experimental smart contract market. There is no operator guaranteeing a sale, delivery, price, or recovery.</p>
    </section>
  );
}
