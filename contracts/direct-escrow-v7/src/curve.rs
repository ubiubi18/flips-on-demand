//! Checked, deterministic one-bundle buyer quotes for the capped v7 pool.

pub const PRICE_CAP: u128 = 1_000 * 1_000_000_000_000_000_000;
// Ten virtual units calibrate the 10,000 iDNA deposit to a 1,000 iDNA
// quote with one actual, proved public bundle in the queue.
const VIRTUAL_FLIP_UNITS: u64 = 10;

/// Quote the iDNA charged for one genuine bundle removed from the pool.
///
/// The caller holds the price as an order liability; only resale revenue
/// enters the reserve after a successful delivery.
pub fn buy_quote(x: u64, y: u128) -> Option<u128> {
    if x == 0 || y == 0 {
        return None;
    }

    let denominator = u128::from(x.checked_add(VIRTUAL_FLIP_UNITS)?.checked_sub(1)?);
    let price = (y / denominator)
        .checked_add(u128::from(y % denominator != 0))?
        .min(PRICE_CAP);
    if price == 0 {
        return None;
    }

    Some(price)
}

#[cfg(test)]
mod tests {
    use super::*;

    const IDNA: u128 = 1_000_000_000_000_000_000;
    const PILOT_RESERVE: u128 = 10_000 * IDNA;

    #[test]
    fn one_permanent_seed_can_be_bought_at_the_cap() {
        assert_eq!(buy_quote(1, PILOT_RESERVE), Some(PRICE_CAP));
    }

    #[test]
    fn more_queued_offers_lower_the_price_below_the_cap() {
        assert!(buy_quote(2, PILOT_RESERVE).unwrap() < PRICE_CAP);
        assert!(buy_quote(11, PILOT_RESERVE).unwrap() < buy_quote(2, PILOT_RESERVE).unwrap());
        assert!(buy_quote(12, PILOT_RESERVE).unwrap() < buy_quote(11, PILOT_RESERVE).unwrap());
    }

    #[test]
    fn quoted_price_is_positive_and_bounded() {
        for units in 1..=20 {
            for reserve in [500 * IDNA, 1_000 * IDNA, PILOT_RESERVE] {
                let price = buy_quote(units, reserve).unwrap();
                assert!(price > 0 && price <= PRICE_CAP);
            }
        }
    }

    #[test]
    fn zero_quotes_and_empty_reserves_are_rejected() {
        assert_eq!(buy_quote(1, 0), None);
        assert_eq!(buy_quote(0, PILOT_RESERVE), None);
    }
}
