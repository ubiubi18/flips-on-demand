//! Fixed-width direct-sale records. The contract stores one immutable offer per
//! author and epoch, and permits only one funded order on it at a time.

pub const OFFER_BYTES: usize = 20 + 2 + 4 + 32 + 32 + 16 + 8 + 8 + 1;
pub const ORDER_BYTES: usize = 32 + 20 + 65 + 16 + 16 + 8 + 8 + 1 + 32;
pub const FUNDED: u8 = 1;
pub const SETTLED: u8 = 2;
pub const REFUNDED: u8 = 3;
pub const FALLBACK_PENDING: u8 = 4;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Offer {
    pub author: [u8; 20],
    pub epoch: u16,
    pub covered: u32,
    pub inventory: [u8; 32],
    pub keys: [u8; 32],
    pub price: u128,
    pub next_nonce: u64,
    pub open_order: u64,
    pub retired: bool,
}

impl Offer {
    pub fn encode(&self) -> [u8; OFFER_BYTES] {
        let mut out = [0u8; OFFER_BYTES];
        out[0..20].copy_from_slice(&self.author);
        out[20..22].copy_from_slice(&self.epoch.to_be_bytes());
        out[22..26].copy_from_slice(&self.covered.to_be_bytes());
        out[26..58].copy_from_slice(&self.inventory);
        out[58..90].copy_from_slice(&self.keys);
        out[90..106].copy_from_slice(&self.price.to_be_bytes());
        out[106..114].copy_from_slice(&self.next_nonce.to_be_bytes());
        out[114..122].copy_from_slice(&self.open_order.to_be_bytes());
        out[122] = u8::from(self.retired);
        out
    }

    pub fn decode(raw: &[u8]) -> Option<Self> {
        if raw.len() != OFFER_BYTES || raw[122] > 1 {
            return None;
        }
        Some(Self {
            author: raw[0..20].try_into().ok()?,
            epoch: u16::from_be_bytes(raw[20..22].try_into().ok()?),
            covered: u32::from_be_bytes(raw[22..26].try_into().ok()?),
            inventory: raw[26..58].try_into().ok()?,
            keys: raw[58..90].try_into().ok()?,
            price: u128::from_be_bytes(raw[90..106].try_into().ok()?),
            next_nonce: u64::from_be_bytes(raw[106..114].try_into().ok()?),
            open_order: u64::from_be_bytes(raw[114..122].try_into().ok()?),
            retired: raw[122] == 1,
        })
    }

    pub fn can_buy(&self) -> bool {
        !self.retired && self.open_order == 0 && self.price > 0
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Order {
    pub offer: [u8; 32],
    pub buyer: [u8; 20],
    pub delivery_key: [u8; 65],
    pub paid: u128,
    pub bond: u128,
    pub funded_at: u64,
    pub nonce: u64,
    pub state: u8,
    pub delivery_hash: [u8; 32],
}

impl Order {
    pub fn encode(&self) -> [u8; ORDER_BYTES] {
        let mut out = [0u8; ORDER_BYTES];
        out[0..32].copy_from_slice(&self.offer);
        out[32..52].copy_from_slice(&self.buyer);
        out[52..117].copy_from_slice(&self.delivery_key);
        out[117..133].copy_from_slice(&self.paid.to_be_bytes());
        out[133..149].copy_from_slice(&self.bond.to_be_bytes());
        out[149..157].copy_from_slice(&self.funded_at.to_be_bytes());
        out[157..165].copy_from_slice(&self.nonce.to_be_bytes());
        out[165] = self.state;
        out[166..198].copy_from_slice(&self.delivery_hash);
        out
    }

    pub fn decode(raw: &[u8]) -> Option<Self> {
        if raw.len() != ORDER_BYTES || !(FUNDED..=FALLBACK_PENDING).contains(&raw[165]) {
            return None;
        }
        Some(Self {
            offer: raw[0..32].try_into().ok()?,
            buyer: raw[32..52].try_into().ok()?,
            delivery_key: raw[52..117].try_into().ok()?,
            paid: u128::from_be_bytes(raw[117..133].try_into().ok()?),
            bond: u128::from_be_bytes(raw[133..149].try_into().ok()?),
            funded_at: u64::from_be_bytes(raw[149..157].try_into().ok()?),
            nonce: u64::from_be_bytes(raw[157..165].try_into().ok()?),
            state: raw[165],
            delivery_hash: raw[166..198].try_into().ok()?,
        })
    }

    pub fn can_start_fallback(&self, height: u64, window: u64, grace: u64, margin: u64) -> bool {
        self.state == FUNDED
            && self.funded_at.checked_add(grace).is_some_and(|start| height >= start)
            && self.funded_at.checked_add(window).and_then(|end| end.checked_sub(margin)).is_some_and(|end| height < end)
    }

    pub fn seller_can_reveal(&self, height: u64, window: u64) -> bool {
        self.state == FALLBACK_PENDING && self.funded_at.checked_add(window).is_some_and(|end| height < end)
    }

    pub fn can_refund(&self, height: u64, window: u64) -> bool {
        matches!(self.state, FUNDED | FALLBACK_PENDING)
            && self.funded_at.checked_add(window).is_some_and(|end| height >= end)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn records_round_trip_without_role_or_amount_loss() {
        let offer = Offer { author: [1; 20], epoch: 230, covered: 5, inventory: [2; 32], keys: [3; 32], price: 12_345_678_901_234_567_890, next_nonce: 2, open_order: 9, retired: false };
        assert_eq!(Offer::decode(&offer.encode()), Some(offer));
        let order = Order { offer: [4; 32], buyer: [5; 20], delivery_key: [6; 65], paid: offer.price, bond: 99, funded_at: 100, nonce: 2, state: FUNDED, delivery_hash: [7; 32] };
        assert_eq!(Order::decode(&order.encode()), Some(order));
        assert!(Offer::decode(&offer.encode()[..OFFER_BYTES - 1]).is_none());
        assert!(Order::decode(&order.encode()[..ORDER_BYTES - 1]).is_none());
        let mut retired_state = order.encode();
        retired_state[165] = 5;
        assert!(Order::decode(&retired_state).is_none());
    }

    #[test]
    fn fallback_and_refund_windows_do_not_overlap() {
        let order = Order { offer: [0; 32], buyer: [0; 20], delivery_key: [0; 65], paid: 1, bond: 10, funded_at: 100, nonce: 0, state: FUNDED, delivery_hash: [0; 32] };
        assert!(!order.can_start_fallback(129, 100, 30, 10));
        assert!(order.can_start_fallback(130, 100, 30, 10));
        assert!(order.can_start_fallback(189, 100, 30, 10));
        assert!(!order.can_start_fallback(190, 100, 30, 10));
        assert!(!order.can_refund(199, 100));
        assert!(order.can_refund(200, 100));
        let pending = Order { state: FALLBACK_PENDING, ..order };
        assert!(!pending.can_start_fallback(130, 100, 30, 10));
        assert!(pending.seller_can_reveal(199, 100));
        assert!(!pending.seller_can_reveal(200, 100));
        assert!(pending.can_refund(200, 100));
        let closed = Order { state: SETTLED, ..order };
        assert!(!closed.seller_can_reveal(199, 100));
        assert!(!closed.can_refund(200, 100));
    }
}
