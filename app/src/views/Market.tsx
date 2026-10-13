import { useMemo, useState } from "react";
import { snapshotIsRecent, type MarketplaceState, type PoolListing } from "../state";
import { formatAmount, shortHex } from "../format";
import { downloadActionRequest } from "../request";
import { NextStep } from "./NextStep";

// The advisory records when its oldest check was made. An advisory that
// recorded no time says so instead of showing the page's own clock.
function availabilityCheckedAt(value: string): string {
  const at = new Date(value);
  if (value.trim() === "" || Number.isNaN(at.getTime())) {
    return "at an unknown time";
  }
  return `at ${at.toISOString().replace("T", " ").slice(0, 16)} UTC`;
}

// Repayment opens at the third block after inclusion; unilateral settlement
// must precede it. The chain's observed block interval
// turns that into an estimate; without one, the time depends on the chain.
const DELIVERY_BLOCKS = 3;
const FEE_PER_ITEM = 10n ** 18n;

function deliveryEstimate(state: MarketplaceState): string {
  if (state.blockSeconds === undefined || state.blockSeconds <= 0) {
    return "delivery within 3 blocks (the time depends on the chain)";
  }
  const seconds = state.blockSeconds * DELIVERY_BLOCKS;
  if (seconds < 90) {
    return `delivery within 3 blocks (about ${seconds}s)`;
  }
  return `delivery within 3 blocks (about ${Math.round(seconds / 60)} min)`;
}

function retrievalFeeCap(listing: PoolListing): bigint | undefined {
  if (listing.flipCount === undefined) {
    return undefined;
  }
  return BigInt(listing.flipCount) * FEE_PER_ITEM;
}

export function Market({ state, canPrepare }: { state: MarketplaceState | null; canPrepare: boolean }) {
  if (!state) {
    return (
      <section className="empty">
        <h2>Nothing loaded yet</h2>
        <p>Import the file your flips tool wrote, and what is for sale appears here.</p>
      </section>
    );
  }
  return <PoolMarket state={state} canPrepare={canPrepare} />;
}

/**
 * The ownerless pool is the canonical market: one reserve against the sale
 * units sets the quote, and the buy request names the chain's own nonce so the
 * CLI can recheck it before signing.
 */
function PoolMarket({ state, canPrepare }: { state: MarketplaceState; canPrepare: boolean }) {
  const [query, setQuery] = useState("");
  const [saved, setSaved] = useState<{ file: string; command: string } | null>(null);
  const listings = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return state.pool
      .filter((listing) => {
        if (needle === "") {
          return true;
        }
        return (
          listing.author.toLowerCase().includes(needle) ||
          listing.bundle.toLowerCase().includes(needle) ||
          String(listing.epoch ?? "") === needle
        );
      })
      .slice()
      .sort((a, b) => {
        if (a.buyQuote === undefined && b.buyQuote === undefined) {
          return a.bundle.localeCompare(b.bundle);
        }
        if (a.buyQuote === undefined) {
          return 1;
        }
        if (b.buyQuote === undefined) {
          return -1;
        }
        const left = BigInt(a.buyQuote);
        const right = BigInt(b.buyQuote);
        return left === right ? a.bundle.localeCompare(b.bundle) : left < right ? -1 : 1;
      });
  }, [state, query]);

  return (
    <section>
      <div className="toolbar">
        <input
          type="search"
          placeholder="Search author, epoch or bundle"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          aria-label="Filter pool listings"
        />
        <span className="muted">
          {listings.length} of {state.pool.length} listings
        </span>
        {!state.viewer && <span className="muted">Import a snapshot with your address to buy.</span>}
      </div>
      <NextStep fileName={saved?.file ?? null} command={saved?.command ?? ""} />
      <div className="table-wrap">
        <table className="market stack">
          <thead>
            <tr>
              <th>Bundle</th>
              <th>Flip author</th>
              <th className="numeric">Epoch</th>
              <th className="numeric">Flips</th>
              <th className="numeric">
                Price<span className="unit"> (iDNA)</span>
              </th>
              <th className="numeric">Units</th>
              <th>Delivery desk</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {listings.map((listing) => (
              <PoolRow
                key={listing.bundle}
                listing={listing}
                state={state}
                canPrepare={canPrepare}
                onSaved={(file) => setSaved({ file, command: "fod prepare-buy -request" })}
              />
            ))}
            {listings.length === 0 && (
              <tr>
                <td colSpan={8} className="muted">
                  Nothing matches this filter.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function PoolRow({
  listing,
  state,
  canPrepare,
  onSaved,
}: {
  listing: PoolListing;
  state: MarketplaceState;
  canPrepare: boolean;
  onSaved: (fileName: string) => void;
}) {
  const viewer = state.viewer;
  const beaconFresh = listing.beaconBlock !== 0 && state.blockHeight <= listing.beaconBlock + 288;
  const availabilityFresh = listing.availability === undefined || snapshotIsRecent(listing.availability.checkedAt);
  const buyable = canPrepare && availabilityFresh && listing.buyQuote !== undefined && beaconFresh && viewer !== undefined;

  const prepareBuy = () => {
    if (!viewer || listing.buyQuote === undefined) {
      return;
    }
    const fileName = `fod-action-buy-${listing.bundle.slice(0, 16)}.json`;
    downloadActionRequest(
      {
        format: "fod.action-request",
        version: 1,
        network: state.network,
        contract: state.contract,
        operation: "buy",
        arguments: {
          bundleId: listing.bundle,
          expectedQuote: listing.buyQuote,
          buyer: viewer.address,
        },
        amount: listing.buyQuote,
        createdAt: new Date().toISOString(),
      },
      fileName,
    );
    onSaved(fileName);
  };

  return (
    <tr>
      <td className="mono" title={listing.bundle}>
        <span className="field-label">Bundle</span>
        {shortHex(listing.bundle, 8)}
      </td>
      <td title={listing.author}>
        <span className="field-label">Flip author</span>
        {shortHex(listing.author)}
      </td>
      <td className="numeric">
        <span className="field-label">Epoch</span>
        {listing.epoch ?? "-"}
      </td>
      <td className="numeric">
        <span className="field-label">Flips</span>
        {listing.flipCount ?? "-"}
      </td>
      <td className="numeric">
        <span className="field-label">Price (iDNA)</span>
        {listing.buyQuote === undefined ? (
          "not for sale"
        ) : (
          <>
            {formatAmount(listing.buyQuote)}
            {retrievalFeeCap(listing) !== undefined && (
              <div className="muted">
                + retrieval fee up to {formatAmount(retrievalFeeCap(listing)!.toString())}, paid to the hosts you name
              </div>
            )}
          </>
        )}
      </td>
      <td className="numeric">
        <span className="field-label">Units</span>
        {listing.units}
      </td>
      <td>
        <span className="field-label">Delivery desk</span>
        {listing.fast && <span className="chip chip-open">Fast flips</span>}
        <span className={`chip chip-${beaconFresh ? "open" : "refunded"}`}>
          {beaconFresh ? `reported live at block ${state.blockHeight}` : "stale in snapshot"}
        </span>
        {listing.strikes > 0 && <span className="muted"> {listing.strikes} strike(s)</span>}
        <div className="muted">
          {listing.availability === undefined ? (
            "not checked"
          ) : (
            <>
              <span className={`chip chip-${listing.availability.complete && availabilityFresh ? "sold" : "funded"}`}>
                {!availabilityFresh ? "check expired" : listing.availability.complete ? "available at check" : "partial at check"}
              </span>{" "}
              {listing.availability.verifiedFlips}/{listing.availability.coveredFlips} flips checked{" "}
              {availabilityCheckedAt(listing.availability.checkedAt)}
            </>
          )}
        </div>
        <div className="muted">{deliveryEstimate(state)}</div>
        <div className="muted">
          the window starts when your buy is included and counts blocks, not seconds - a chain stall (a validation
          ceremony, for instance) can use it up before the desk can deliver; repayment opens at the third block either
          way
        </div>
      </td>
      <td>
        <button
          type="button"
          className="primary"
          disabled={!buyable}
          onClick={prepareBuy}
          aria-label={`Save buy request for bundle ${shortHex(listing.bundle, 8)} at ${listing.buyQuote === undefined ? "no quote" : `${formatAmount(listing.buyQuote)} iDNA`}`}
        >
          Save buy request
        </button>
      </td>
    </tr>
  );
}
