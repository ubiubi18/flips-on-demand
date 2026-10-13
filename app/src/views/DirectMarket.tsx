import type { DirectMarketState } from "../direct-state";
import { formatAmount, shortHex } from "../format";

export function DirectMarket({ state }: { state: DirectMarketState }) {
  return (
    <section>
      <header className="section-head"><h2>Direct offers</h2></header>
      <p className="muted">Verify an offer with the seller's public session bundle and your local flips tool. Buying also locks a refundable proof bond; the local tool quotes its current amount before signing.</p>
      <div className="table-wrap">
        <table className="market stack">
          <thead><tr>
            <th>Offer</th><th>Flip author</th><th className="numeric">Epoch</th>
            <th className="numeric">Flips</th><th className="numeric">Price (iDNA)</th><th>Status</th>
          </tr></thead>
          <tbody>
            {state.offers.map((offer) => (
              <tr key={offer.offerId}>
                <td className="mono" title={offer.offerId}>{shortHex(offer.offerId)}</td>
                <td className="mono" title={offer.author}>{shortHex(offer.author)}</td>
                <td className="numeric">{offer.epoch}</td>
                <td className="numeric">{offer.coveredFlips}</td>
                <td className="numeric">{formatAmount(offer.price)}</td>
                <td><span className={`chip ${offer.retired ? "chip-refunded" : offer.openOrderId !== "0" ? "chip-funded" : "chip-open"}`}>
                  {offer.retired ? "Closed" : offer.openOrderId !== "0" ? "In order" : "Available"}
                </span></td>
              </tr>
            ))}
            {state.offers.length === 0 && <tr><td colSpan={6} className="muted">No direct offers on this contract.</td></tr>}
          </tbody>
        </table>
      </div>
    </section>
  );
}

export function DirectTrades({ state }: { state: DirectMarketState }) {
  const orders = state.orders.slice().sort((a, b) => {
    const left = BigInt(a.orderId);
    const right = BigInt(b.orderId);
    return left === right ? 0 : left > right ? -1 : 1;
  });
  return (
    <section>
      <header className="section-head"><h2>Direct trades</h2></header>
      <p className="muted">Public orders on this contract. The bond returns on buyer confirmation or refund; a valid seller fallback reimburses capped proof costs and returns the rest to the buyer.</p>
      <div className="table-wrap">
        <table className="market stack">
          <thead><tr>
            <th>Order</th><th>Offer</th><th>Buyer</th><th className="numeric">Price (iDNA)</th><th className="numeric">Bond (iDNA)</th>
            {state.version === 3 && <th className="numeric">Seller stake (iDNA)</th>}
            <th>Status</th><th className="numeric">Refund from block</th>
          </tr></thead>
          <tbody>
            {orders.map((order) => (
              <tr key={order.orderId}>
                <td>{order.orderId}</td>
                <td className="mono" title={order.offerId}>{shortHex(order.offerId)}</td>
                <td className="mono" title={order.buyer}>{shortHex(order.buyer)}</td>
                <td className="numeric">{formatAmount(order.paid)}</td>
                <td className="numeric">{formatAmount(order.bond)}</td>
                {state.version === 3 && <td className="numeric">{order.sellerStake === undefined ? "—" : formatAmount(order.sellerStake)}</td>}
                <td><span className={`chip chip-${order.state}`}>{order.keysPublic ? "Keys public" : order.state === "settled" ? "Buyer confirmed" : order.state === "fallback-intent" ? "Seller intent" : order.state === "fallback-pending" ? "Proof accepted" : order.state}</span></td>
                <td className="numeric">{order.state === "funded" || order.state === "fallback-intent" || order.state === "fallback-pending" ? order.refundAtBlock : "—"}</td>
              </tr>
            ))}
            {orders.length === 0 && <tr><td colSpan={state.version === 3 ? 8 : 7} className="muted">No direct trades on this contract.</td></tr>}
          </tbody>
        </table>
      </div>
    </section>
  );
}

export function DirectGuide({ version }: { version?: DirectMarketState["version"] }) {
  return (
    <section>
      <h2>Direct sales</h2>
      <p>The seller publishes the original flip inventory and offers its epoch keys. The buyer's local tool checks the session and retrieves each original encrypted flip before funding escrow.</p>
      <p>The buyer locks the price plus a proof bond. The seller sends encrypted keys privately. The buyer checks that they open the original flips, then confirms to pay the seller and reclaim the bond. A message alone does not settle an order.</p>
      {version === 3
        ? <p>If the buyer does not confirm, the seller can submit proof of the original flips and lock a stake to start a fallback. Once that proof is accepted, the seller can publish the keys before the deadline. The seller receives the price and capped proof costs; the unused buyer bond and seller stake are returned. If the seller does not finish, the buyer can refund the price and bond and receive the seller stake. The contract cannot check whether a private message was sent.</p>
        : <p>If the buyer does not confirm, the seller can lock a stake to announce a fallback. After that intent is included, the seller submits proof that the original flip keys are valid, then publishes the keys before the deadline. The seller receives the price and capped proof costs; the unused buyer bond and seller stake are returned. If the seller does not finish, the buyer can refund both the price and bond and receive the seller stake. The contract cannot check whether a private message was sent.</p>}
    </section>
  );
}
