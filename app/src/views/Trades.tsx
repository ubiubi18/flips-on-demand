import type { MarketplaceState, PoolListing, PoolOrder } from "../state";
import { formatAmount, shortHex } from "../format";
import { downloadActionRequest } from "../request";
import { NextStep } from "./NextStep";
import { useState } from "react";

export function Trades({ state }: { state: MarketplaceState | null }) {
  if (!state) {
    return (
      <section className="empty">
        <h2>Nothing loaded yet</h2>
        <p>Import the file your flips tool wrote to see your orders and your money here.</p>
      </section>
    );
  }
  const viewer = state.viewer;
  if (!viewer) {
    return (
      <section className="empty">
        <h2>No address in this file</h2>
        <p>This file does not say which address is yours, so there is nothing to show yet.</p>
      </section>
    );
  }
  return <PoolTrades state={state} viewer={viewer} />;
}

/**
 * The viewer's pool orders and the listings the viewer can serve. Every action
 * writes a typed request; the CLI rechecks the chain and does the signing, so
 * the snapshot never authorises a payment.
 */
function PoolTrades({
  state,
  viewer,
}: {
  state: MarketplaceState;
  viewer: { address: string; credits: string };
}) {
  const [saved, setSaved] = useState<{ file: string; command: string } | null>(null);
  const [hint, setHint] = useState<string | null>(null);
  const hasCredit = BigInt(viewer.credits) > 0n;
  const orders = state.poolOrders
    .filter((order) => order.buyer === viewer.address)
    .sort((a, b) => b.fundedAtBlock - a.fundedAtBlock);
  const listings = state.pool.filter((listing) => listing.author === viewer.address);
  const openOrders = orders.filter((order) => order.state === "funded" || order.state === "delivered").length;

  const save = (file: string, command: string) => {
    setHint(null);
    setSaved({ file, command });
  };
  const prepareWithdrawal = () => {
    const fileName = "fod-action-withdraw.json";
    downloadActionRequest(
      {
        format: "fod.action-request",
        version: 1,
        network: state.network,
        contract: state.contract,
        operation: "withdraw",
        arguments: {},
        amount: "0",
        createdAt: new Date().toISOString(),
      },
      fileName,
    );
    save(fileName, "fod withdraw");
  };
  /**
   * The confirmation has to name the ciphertext the buyer's own run checked
   * (`SPEC.md` 7.1), so this action reads the record `fod receive -out` wrote.
   * Naming whatever the snapshot holds would settle a payload the buyer never
   * saw, which is exactly what the escrow's digest check exists to stop.
   */
  const prepareConfirm = async (order: PoolOrder, file: File) => {
    const parsed = readOpenedDelivery(await file.text());
    if (!parsed.ok) {
      setHint(parsed.error);
      return;
    }
    if (
      parsed.record.orderId !== order.orderId ||
      parsed.record.contract.toLowerCase() !== state.contract.toLowerCase()
    ) {
      setHint("That record is for another order or another marketplace.");
      return;
    }
    const fileName = `fod-action-confirm-${order.orderId}.json`;
    downloadActionRequest(
      {
        format: "fod.action-request",
        version: 1,
        network: state.network,
        contract: state.contract,
        operation: "confirm",
        arguments: {
          bundleId: order.bundle,
          orderId: order.orderId,
          ciphertextSha256: parsed.record.ciphertextSha256,
          ciphertextHex: parsed.record.ciphertextHex,
        },
        amount: "0",
        createdAt: new Date().toISOString(),
      },
      fileName,
    );
    save(fileName, "fod prepare-confirm");
  };
  const prepareRefund = (order: PoolOrder) => {
    const fileName = `fod-action-refund-${order.orderId}.json`;
    downloadActionRequest(
      {
        format: "fod.action-request",
        version: 1,
        network: state.network,
        contract: state.contract,
        operation: "refund",
        arguments: { orderId: order.orderId },
        amount: "0",
        createdAt: new Date().toISOString(),
      },
      fileName,
    );
    save(fileName, "fod refund");
  };
  const prepareGive = (listing: PoolListing) => {
    const fileName = `fod-action-give-${listing.bundle.slice(0, 16)}.json`;
    downloadActionRequest(
      {
        format: "fod.action-request",
        version: 1,
        network: state.network,
        contract: state.contract,
        operation: "donateUnit",
        arguments: { commitment: listing.bundle },
        amount: "0",
        createdAt: new Date().toISOString(),
      },
      fileName,
    );
    save(fileName, "fod prepare-list -request");
  };
  const showReceive = (order: PoolOrder) => {
    setSaved(null);
    if (state.pool.find((listing) => listing.bundle === order.bundle)?.fast) {
      setHint(`Run fod receive -pool-order ${order.orderId} -contract ${state.contract} -delivery-key <your delivery key> -prepared <your buy file> -out recovery.json, then recover_fast --input recovery.json --delivery-key <your delivery key> --out-dir <new directory>. The included delivery has already settled this order.`);
      return;
    }
    setHint(
      `Run fod receive -pool-order ${order.orderId} -contract ${state.contract} -delivery-key <your delivery key> -vault <your vault> -prepared <your buy file> -out delivery.json to verify retained content. Then use -delivery delivery.json with fod prepare-confirm.`,
    );
  };

  return (
    <section>
      <header className="section-head">
        <h2>My trades</h2>
        <span className="mono muted" title={viewer.address}>
          {shortHex(viewer.address, 8)}
        </span>
      </header>

      <dl className="facts">
        <div>
          <dt>Money you can withdraw</dt>
          <dd>
            {formatAmount(viewer.credits)} iDNA{" "}
            <button type="button" className="linklike" disabled={!hasCredit} onClick={prepareWithdrawal}>
              Withdraw
            </button>
          </dd>
        </div>
        <div>
          <dt>Pool orders in progress</dt>
          <dd>{openOrders}</dd>
        </div>
      </dl>
      <NextStep fileName={saved?.file ?? null} command={saved?.command ?? ""} />
      {hint !== null && (
        <p className="notice" role="status">
          {hint}
        </p>
      )}

      <div className="table-wrap">
        <table className="stack">
          <thead>
            <tr>
              <th>Order</th>
              <th>Bundle</th>
              <th className="numeric">Paid</th>
              <th className="numeric">Paid at block</th>
              <th className="numeric">Deliver by block</th>
              <th className="numeric">Refund from block</th>
              <th>Status</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {orders.map((order) => (
              <tr key={order.orderId}>
                <td>
                  <span className="field-label">Order</span>#{order.orderId}
                </td>
                <td className="mono" title={order.bundle}>
                  <span className="field-label">Bundle</span>
                  {shortHex(order.bundle, 8)}
                </td>
                <td className="numeric">
                  <span className="field-label">Paid</span>
                  {formatAmount(order.paid)}
                </td>
                <td className="numeric">
                  <span className="field-label">Paid at block</span>
                  {order.fundedAtBlock.toLocaleString()}
                </td>
                <td className="numeric">
                  <span className="field-label">Deliver by block</span>
                  {order.deliveryDeadlineBlock.toLocaleString()}
                </td>
                <td className="numeric">
                  <span className="field-label">Refund from block</span>
                  {order.refundAfterBlock.toLocaleString()}
                </td>
                <td>
                  <span className="field-label">Status</span>
                  <span className={`chip chip-${poolOrderTone(order, state.blockHeight)}`}>
                    {poolOrderLabel(order, state.blockHeight)}
                  </span>
                </td>
                <td>
                  {order.state === "delivered" && (
                    <label className="import">
                      <input
                        type="file"
                        accept="application/json,.json"
                        aria-label={`Confirm order ${order.orderId}`}
                        onChange={(event) => {
                          const file = event.target.files?.[0];
                          if (file) {
                            void prepareConfirm(order, file);
                          }
                          event.target.value = "";
                        }}
                      />
                      <span>Confirm</span>
                    </label>
                  )}
                  {(order.state === "delivered" || order.state === "settled") && (
                    <button type="button" className="linklike" onClick={() => showReceive(order)}>
                      Receive
                    </button>
                  )}
                  {(order.state === "delivered" || order.state === "settled") &&
                    !state.pool.find((listing) => listing.bundle === order.bundle)?.fast && (
                    <a className="linklike" href="#/session">
                      Use these flips
                    </a>
                  )}
                  {poolRefundable(order, state.blockHeight) && (
                    <button type="button" className="linklike" onClick={() => prepareRefund(order)}>
                      Refund
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {orders.length === 0 && (
              <tr>
                <td colSpan={8} className="muted">
                  No pool orders for this address in the snapshot.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <h3>Flip sets you can serve</h3>
      {listings.length === 0 ? (
        <p className="muted">This snapshot holds no pool listing of yours yet.</p>
      ) : (
        <div className="table-wrap">
          <table className="stack">
            <thead>
              <tr>
                <th>Bundle</th>
                <th className="numeric">Quote</th>
                <th className="numeric">Units</th>
                <th>Delivery desk</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {listings.map((listing) => {
                const fresh = listing.beaconBlock !== 0 && state.blockHeight <= listing.beaconBlock + 288;
                return (
                  <tr key={listing.bundle}>
                    <td className="mono" title={listing.bundle}>
                      <span className="field-label">Bundle</span>
                      {shortHex(listing.bundle, 8)}
                    </td>
                    <td className="numeric">
                      <span className="field-label">Quote</span>
                      {listing.buyQuote === undefined ? "no quote" : formatAmount(listing.buyQuote)}
                    </td>
                    <td className="numeric">
                      <span className="field-label">Units</span>
                      {listing.units}
                    </td>
                    <td>
                      <span className="field-label">Delivery desk</span>
                      <span className={`chip chip-${fresh ? "open" : "refunded"}`}>
                        {fresh ? `reported live at block ${state.blockHeight}` : "stale in snapshot"}
                      </span>
                    </td>
                    <td>
                      <button type="button" className="linklike" onClick={() => prepareGive(listing)}>
                        Give one unit
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function poolRefundable(order: PoolOrder, blockHeight: number): boolean {
  return (order.state === "funded" || order.state === "delivered") && blockHeight >= order.refundAfterBlock;
}

const HEX_64 = /^[0-9a-f]{64}$/i;
const HEX_40 = /^[0-9a-f]{40}$/i;

/**
 * The record `fod receive -out` writes: what the buyer's run read, opened and
 * checked. The confirmation names its digest rather than the snapshot's bytes.
 */
function readOpenedDelivery(
  text: string,
): {
  ok: true;
  record: { orderId: number; contract: string; ciphertextSha256: string; ciphertextHex: string };
} | { ok: false; error: string } {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return { ok: false, error: "That file is not JSON." };
  }
  if (typeof parsed !== "object" || parsed === null) {
    return { ok: false, error: "That file is not a record from `fod receive -out`." };
  }
  const record = parsed as Record<string, unknown>;
  if (record.format !== "fod.opened-pool-delivery") {
    return { ok: false, error: "That file is not a record from `fod receive -out`." };
  }
  const orderId = record.orderId;
  const contract = record.contract;
  const digest = record.ciphertextSha256;
  const ciphertextHex = record.ciphertextHex;
  if (typeof orderId !== "number" || !Number.isInteger(orderId) || orderId <= 0) {
    return { ok: false, error: "That record names no pool order." };
  }
  if (typeof contract !== "string" || !HEX_40.test(contract)) {
    return { ok: false, error: "That record names no marketplace." };
  }
  if (typeof digest !== "string" || !HEX_64.test(digest)) {
    return {
      ok: false,
      error: "That record names no ciphertext digest: re-run `fod receive -out` with this build.",
    };
  }
  // The confirmation carries the checked bytes themselves, so the record has
  // to carry them too (`AH-10`).
  if (typeof ciphertextHex !== "string" || ciphertextHex === "" || !/^([0-9a-f]{2})+$/i.test(ciphertextHex)) {
    return {
      ok: false,
      error: "That record carries no ciphertext bytes: re-run `fod receive -out` with this build.",
    };
  }
  return {
    ok: true,
    record: {
      orderId,
      contract,
      ciphertextSha256: digest.toLowerCase(),
      ciphertextHex: ciphertextHex.toLowerCase(),
    },
  };
}

function poolOrderLabel(order: PoolOrder, blockHeight: number): string {
  if (poolRefundable(order, blockHeight)) {
    return "refund available";
  }
  switch (order.state) {
    case "funded":
      return "paid, desk delivers";
    case "delivered":
      return "keys ready";
    case "settled":
      return "settled";
    case "refunded":
      return "refunded";
    default:
      return order.state;
  }
}

function poolOrderTone(order: PoolOrder, blockHeight: number): string {
  if (poolRefundable(order, blockHeight)) {
    return "refunded";
  }
  switch (order.state) {
    case "funded":
      return "funded";
    case "delivered":
      return "open";
    case "settled":
      return "sold";
    case "refunded":
      return "refunded";
    default:
      return "open";
  }
}
