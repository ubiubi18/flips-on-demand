//! Checked, deterministic one-bundle quotes for the capped v6 pool.

pub const PRICE_CAP: u128 = 1_000 * 1_000_000_000_000_000_000;

/// Quote the iDNA paid for one genuine bundle added to the pool.
///
/// Returns `(payout, next_units, next_reserve)`. The capped payout keeps
/// a positive reserve and declines as queued supply grows.
pub fn sell_quote(x: u64, y: u128) -> Option<(u128, u64, u128)> {
    if x == 0 || y == 0 {
        return None;
    }

    let next_x = x.checked_add(1)?;
    let payout = (y / u128::from(next_x)).min(PRICE_CAP);
    if payout == 0 {
        return None;
    }

    let next_y = y.checked_sub(payout)?;
    if next_y == 0 {
        return None;
    }
    Some((payout, next_x, next_y))
}

/// Quote the iDNA charged for one genuine bundle removed from the pool.
///
/// Returns `(price, next_units, next_reserve)`. Both sides use the same
/// ceiling; this is a bounded curve, not an exact constant-product swap.
pub fn buy_quote(x: u64, y: u128) -> Option<(u128, u64, u128)> {
    if x <= 1 || y == 0 {
        return None;
    }

    let next_x = x - 1;
    let denominator = u128::from(next_x);
    let price = (y / denominator)
        .checked_add(u128::from(y % denominator != 0))?
        .min(PRICE_CAP);
    if price == 0 {
        return None;
    }

    let next_y = y.checked_add(price)?;
    Some((price, next_x, next_y))
}

#[cfg(test)]
mod tests {
    use super::*;

    const IDNA: u128 = 1_000_000_000_000_000_000;
    const PILOT_RESERVE: u128 = 10_000 * IDNA;

    #[test]
    fn selling_then_buying_one_bundle_returns_near_the_starting_reserve() {
        let (payout, x, y) = sell_quote(2, PILOT_RESERVE).unwrap();
        assert_eq!(payout, PRICE_CAP);
        assert_eq!((x, y), (3, 9_000 * IDNA));

        let (price, x, y) = buy_quote(x, y).unwrap();
        assert_eq!(price, PRICE_CAP);
        assert_eq!((x, y), (2, PILOT_RESERVE));
    }

    #[test]
    fn buying_from_two_units_respects_the_cap_and_keeps_one_unit() {
        assert_eq!(
            buy_quote(2, PILOT_RESERVE),
            Some((PRICE_CAP, 1, PILOT_RESERVE + PRICE_CAP))
        );
        assert_eq!(buy_quote(1, PILOT_RESERVE), None);
    }

    #[test]
    fn more_listings_eventually_lower_the_price_below_the_cap() {
        let (mut units, mut reserve) = (2, PILOT_RESERVE);
        for _ in 0..5 {
            (_, units, reserve) = sell_quote(units, reserve).unwrap();
        }
        assert!(buy_quote(units, reserve).unwrap().0 < PRICE_CAP);
    }

    #[test]
    fn adding_then_removing_one_unit_cannot_extract_the_reserve() {
        for units in 2..=20 {
            for reserve in [500 * IDNA, 1_000 * IDNA, PILOT_RESERVE] {
                let Some((payout, next_units, next_reserve)) = sell_quote(units, reserve) else {
                    continue;
                };
                let (price, back_units, back_reserve) =
                    buy_quote(next_units, next_reserve).unwrap();
                assert!(payout <= PRICE_CAP && price <= PRICE_CAP);
                assert_eq!(back_units, units);
                assert!(price >= payout && back_reserve >= reserve);
            }
        }
    }

    #[test]
    fn zero_quotes_empty_reserves_and_overflow_are_rejected() {
        assert_eq!(sell_quote(0, PILOT_RESERVE), None);
        assert_eq!(sell_quote(2, 0), None);
        assert_eq!(sell_quote(2, 2), None);
        assert_eq!(sell_quote(u64::MAX, PILOT_RESERVE), None);
        assert_eq!(buy_quote(2, 0), None);
        assert_eq!(buy_quote(2, u128::MAX), None);
    }
}
