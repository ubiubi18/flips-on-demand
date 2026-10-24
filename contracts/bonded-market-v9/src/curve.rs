//! Integer constant-product quotes. Virtual balances price, never fund, a sale.
pub const IDNA: u128 = 1_000_000_000_000_000_000;
// With no funded reserve or inventory, one bundle costs exactly 500 iDNA:
// 4,500 * 100 / (1,000 - 100). Virtual balances never fund a purchase.
pub const VIRTUAL_RESERVE: u128 = 4_500 * IDNA;
pub const VIRTUAL_UNITS: u128 = 1_000;
pub const STANDARD_WEIGHT: u128 = 100;

/// Seller's gross proceeds for adding one bundle to the priced inventory.
pub fn sell(reserve: u128, inventory: u128) -> Option<u128> {
    let w = STANDARD_WEIGHT;
    reserve.checked_add(VIRTUAL_RESERVE)?.checked_mul(w)?
        .checked_div(inventory.checked_add(VIRTUAL_UNITS)?.checked_add(w)?)
        .filter(|p| *p != 0)
}

/// Buyer's price for removing a bundle. Round upwards to preserve the reserve.
pub fn buy(reserve: u128, inventory: u128) -> Option<u128> {
    let w = STANDARD_WEIGHT;
    let n = reserve.checked_add(VIRTUAL_RESERVE)?.checked_mul(w)?;
    let d = inventory.checked_add(VIRTUAL_UNITS)?.checked_sub(w)?;
    if d == 0 { return None; }
    (n / d).checked_add(u128::from(n % d != 0)).filter(|p| *p != 0)
}
