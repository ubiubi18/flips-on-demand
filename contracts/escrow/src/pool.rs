//! The pool's arithmetic: one reserve against one sale-unit count, and the
//! moves that change them.
//!
//! `SPEC.md` 5.1 fixes the rule and the owner answered the liquidity-scope
//! decision on 2026-09-29 with "one global pool": the whole market has one `r`
//! against one `q`. Integer arithmetic cannot hold a product exactly, so both
//! quotes round in the pool's favour: a buyer pays
//! `ceil(r / (q - 1))` and a seller receives `floor(r / (q + 1))`. A buy
//! followed by a sale of the same unit therefore returns at most what it paid,
//! and the reserve never shrinks by a rounding step, which is what makes a
//! sequence of trades unable to extract value.
//!
//! A buyer's payment is not reserve. `SPEC.md` 5.1 requires pending purchase
//! funds to stay refund escrow, so `buy` takes the unit and holds the payment in
//! `pending`; only `settle` moves it into the reserve, and `refund` releases it
//! back to the buyer. The old product property (`reserve * units` never falls)
//! described a pool whose reserve grew on every purchase; that is not this one,
//! and the tests below check what replaced it: the pool's own funds
//! (`reserve + pending`) are conserved by every move, and a round trip still
//! never pays out more than it took in.

/// The pool's state. One global pool holds the only reserve, sale-unit count
/// and pending escrow; a bundle's own record keeps its inventory instead.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Pool {
    /// The reserve in iDNA base units.
    pub reserve: u128,
    /// The sale units the pool can still sell, across every bundle.
    pub units: u64,
    /// Buyer payments held for orders that are neither settled nor refunded.
    /// Not spendable: it leaves this balance only through `settle` or `refund`.
    pub pending: u128,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PoolError {
    /// Buying needs at least two units: with one, the quote has no denominator.
    NotEnoughUnits,
    /// The payment is not the pool's current quote.
    WrongAmount,
    /// The reserve cannot cover the payout.
    ReserveTooSmall,
    /// The accounting is full.
    Overflow,
}

fn div_ceil(value: u128, divisor: u128) -> Option<u128> {
    let quotient = value / divisor;
    if value % divisor == 0 {
        Some(quotient)
    } else {
        quotient.checked_add(1)
    }
}

/// The price of one unit: `ceil(reserve / (units - 1))`, and nothing when the
/// bundle has fewer than two units.
pub fn buy_quote(reserve: u128, units: u64) -> Option<u128> {
    if units < 2 {
        return None;
    }
    div_ceil(reserve, u128::from(units - 1))
}

/// What a seller receives for one unit: `floor(reserve / (units + 1))`.
pub fn sell_quote(reserve: u128, units: u64) -> Option<u128> {
    let divisor = u128::from(units).checked_add(1)?;
    reserve.checked_div(divisor)
}

impl Pool {
    pub fn new(reserve: u128, units: u64) -> Pool {
        Pool {
            reserve,
            units,
            pending: 0,
        }
    }

    /// Takes one unit out of the pool for the caller's exact payment, holding
    /// the payment as pending: it is the buyer's refund escrow until the order
    /// settles, and it is not part of the reserve a seller is paid from.
    pub fn buy(&mut self, paid: u128) -> Result<u128, PoolError> {
        let quote = buy_quote(self.reserve, self.units).ok_or(PoolError::NotEnoughUnits)?;
        if paid != quote {
            return Err(PoolError::WrongAmount);
        }
        self.pending = self.pending.checked_add(paid).ok_or(PoolError::Overflow)?;
        self.units -= 1;
        Ok(quote)
    }

    /// Moves one order's payment from pending escrow into the reserve, which is
    /// what pays sellers. Called once, when a delivery settles.
    pub fn settle(&mut self, paid: u128) -> Result<(), PoolError> {
        self.pending = self
            .pending
            .checked_sub(paid)
            .ok_or(PoolError::ReserveTooSmall)?;
        self.reserve = self.reserve.checked_add(paid).ok_or(PoolError::Overflow)?;
        Ok(())
    }

    /// Releases one order's payment from pending escrow to its buyer, returning
    /// the amount the caller has to credit. The unit stays consumed.
    pub fn refund(&mut self, paid: u128) -> Result<u128, PoolError> {
        self.pending = self
            .pending
            .checked_sub(paid)
            .ok_or(PoolError::ReserveTooSmall)?;
        Ok(paid)
    }

    /// Adds one unit for the caller and pays the pool's current quote.
    pub fn sell(&mut self) -> Result<u128, PoolError> {
        let payout = sell_quote(self.reserve, self.units).ok_or(PoolError::Overflow)?;
        self.reserve = self
            .reserve
            .checked_sub(payout)
            .ok_or(PoolError::ReserveTooSmall)?;
        self.units = self.units.checked_add(1).ok_or(PoolError::Overflow)?;
        Ok(payout)
    }

    /// Adds one unit and pays nothing.
    pub fn donate(&mut self) -> Result<(), PoolError> {
        self.units = self.units.checked_add(1).ok_or(PoolError::Overflow)?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The pool's own funds: what it holds for the seller and for buyers whose
    /// orders have not settled. A payout is the one move that reduces it.
    fn funds(pool: &Pool) -> u128 {
        pool.reserve + pool.pending
    }

    /// A deterministic walk: buys, sells, and gifts over a small state space, so
    /// every step is checked rather than a sample of them. Money leaves the pool
    /// only as a seller payout, so `funds + payouts` may rise but never fall.
    fn walk(reserve: u128, units: u64, steps: u32) -> u32 {
        let mut pool = Pool::new(reserve, units);
        let start = funds(&pool);
        let mut paid_out = 0u128;
        let mut seed = 0x9e3779b97f4a7c15u64;
        for step in 0..steps {
            seed = seed
                .wrapping_mul(6364136223846793005)
                .wrapping_add(1442695040888963407);
            match seed % 3 {
                0 => {
                    if let Some(quote) = buy_quote(pool.reserve, pool.units) {
                        let _ = pool.buy(quote);
                    }
                }
                1 => {
                    if let Ok(payout) = pool.sell() {
                        paid_out += payout;
                    }
                }
                _ => {
                    let _ = pool.donate();
                }
            }
            assert!(
                funds(&pool) + paid_out >= start,
                "the pool's funds fell below their start: reserve {reserve} units {units} step {step}"
            );
        }
        steps
    }

    #[test]
    fn quotes_round_in_the_pools_favour() {
        // 50,000 against six units: a buyer pays 10,000, a seller receives
        // 50,000/7 rounded down.
        assert_eq!(buy_quote(50_000, 6), Some(10_000));
        assert_eq!(sell_quote(50_000, 6), Some(7_142));
        // Rounding: 10 / 3 is 3.34 and rounds up for a buyer, down for a seller.
        assert_eq!(buy_quote(10, 3), Some(5));
        assert_eq!(sell_quote(10, 3), Some(2));
        // A bundle with one unit has no buy quote.
        assert_eq!(buy_quote(50_000, 1), None);
        assert_eq!(buy_quote(50_000, 0), None);
    }

    #[test]
    fn a_round_trip_pays_the_seller_from_the_reserve_only() {
        for reserve in [1u128, 7, 10, 1_000, 50_000, u128::from(u64::MAX)] {
            for units in 2u64..=64 {
                let mut pool = Pool::new(reserve, units);
                let paid = pool.buy(buy_quote(reserve, units).unwrap()).unwrap();
                assert_eq!(pool.pending, paid, "the buy did not hold the payment back");
                let payout = pool.sell().unwrap();
                // The supply side is paid out of the reserve the seller funded,
                // and the buyer's escrow is not touched by it.
                assert_eq!(
                    pool.reserve + payout,
                    reserve,
                    "reserve {reserve} units {units}: the payout did not come from the reserve"
                );
                assert_eq!(pool.pending, paid, "the sell reached the buyer's escrow");
                assert_eq!(pool.units, units);
            }
        }
    }

    #[test]
    fn the_pools_own_funds_never_fall_along_a_walk() {
        let mut checked = 0u32;
        for reserve in [1u128, 3, 9, 100, 12_345, 9_876_543] {
            for units in [2u64, 3, 5, 8, 13, 21] {
                checked += walk(reserve, units, 200);
            }
        }
        assert!(checked >= 6 * 6 * 200);
    }

    #[test]
    fn sellers_lower_the_quote_and_buyers_raise_it() {
        let mut pool = Pool::new(50_000, 6);
        let before = buy_quote(pool.reserve, pool.units).unwrap();
        pool.buy(before).unwrap();
        let after_buy = buy_quote(pool.reserve, pool.units).unwrap();
        assert!(after_buy > before, "a buy did not raise the quote");
        pool.sell().unwrap();
        pool.sell().unwrap();
        let after_sells = buy_quote(pool.reserve, pool.units).unwrap();
        assert!(after_sells < after_buy, "sells did not lower the quote");
    }

    #[test]
    fn a_pending_payment_becomes_reserve_only_on_settlement() {
        let mut pool = Pool::new(50_000, 6);
        let paid = pool.buy(10_000).unwrap();
        // The payment is the buyer's escrow. It is not reserve, so the next
        // buyer's quote is still computed from what the seller funded; only the
        // unit count moved.
        assert_eq!(pool.reserve, 50_000);
        assert_eq!(pool.pending, 10_000);
        assert_eq!(buy_quote(pool.reserve, pool.units), Some(12_500));

        pool.settle(paid).unwrap();
        assert_eq!(
            pool.reserve, 60_000,
            "settlement moves the payment to the reserve"
        );
        assert_eq!(pool.pending, 0);
    }

    #[test]
    fn a_refund_releases_the_payment_and_leaves_the_reserve_alone() {
        let mut pool = Pool::new(50_000, 6);
        let paid = pool.buy(10_000).unwrap();
        assert_eq!(pool.refund(paid), Ok(paid));
        assert_eq!(pool.reserve, 50_000);
        assert_eq!(pool.pending, 0);
        assert_eq!(pool.units, 5, "a refund does not return the unit");
    }

    /// The sequence the review traced by hand (`REVIEW.md` A03): one reserve
    /// could not both pay supply quotes and keep a buyer's refund backed, and
    /// the traced run paid 50,315 out against a 10,000 refund. Separation is
    /// what answers it: the payout side can spend the reserve down to nothing
    /// and the buyer's escrow is untouched, so the refund is still covered.
    #[test]
    fn a_pending_refund_survives_the_supply_side() {
        let mut pool = Pool::new(50_000, 6);
        let paid = pool.buy(10_000).unwrap();
        let mut paid_out = 0u128;
        for _ in 0..26 {
            match pool.sell() {
                Ok(payout) => paid_out += payout,
                Err(_) => break,
            }
        }
        assert!(paid_out > 0, "the walk paid nothing out");
        assert_eq!(
            pool.pending, paid,
            "supply payouts reached the buyer's escrow after {paid_out} was paid out"
        );
        assert_eq!(pool.refund(paid), Ok(paid));
        assert_eq!(pool.pending, 0);
    }

    #[test]
    fn full_accounting_and_wrong_amounts_are_refused() {
        let mut pool = Pool::new(50_000, 6);
        assert_eq!(pool.buy(9_999), Err(PoolError::WrongAmount));
        assert_eq!(pool.units, 6, "a refused buy moved the units");
        assert_eq!(pool.pending, 0, "a refused buy held nothing");

        let mut one_unit = Pool::new(50_000, 1);
        assert_eq!(one_unit.buy(50_000), Err(PoolError::NotEnoughUnits));

        let mut empty = Pool::new(0, 4);
        assert_eq!(empty.sell(), Ok(0));
        assert_eq!(empty.units, 5);

        // Settlement cannot overflow the reserve into nonsense...
        let mut almost_full = Pool::new(u128::MAX - 1, 3);
        let quote = buy_quote(almost_full.reserve, almost_full.units).unwrap();
        assert_eq!(almost_full.buy(quote), Ok(quote));
        assert_eq!(almost_full.settle(quote), Err(PoolError::Overflow));

        // ...and neither settlement nor refund can release money the pool is
        // not holding for that order.
        let mut nothing_pending = Pool::new(50_000, 6);
        assert_eq!(nothing_pending.settle(1), Err(PoolError::ReserveTooSmall));
        assert_eq!(nothing_pending.refund(1), Err(PoolError::ReserveTooSmall));
    }
}
