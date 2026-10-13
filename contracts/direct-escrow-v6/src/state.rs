//! Fixed-width records for one author and epoch bundle per offer.

pub const OFFER_BYTES: usize = 238;
pub const ORDER_BYTES: usize = 215;
pub const QUALIFIED: u8 = 0;
pub const QUEUED_PRIMARY: u8 = 1;
pub const QUEUED_SECONDARY: u8 = 2;
pub const ACTIVE: u8 = 3;
pub const SOLD_PRIVATE: u8 = 4;
pub const CANCELLED: u8 = 5;
pub const RETIRED: u8 = 6;
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
    pub payout: u128,
    pub seller_bond: u128,
    pub next_nonce: u64,
    pub open_order: u64,
    pub status: u8,
    pub ever_sold: bool,
    pub previous: [u8; 32],
    pub next: [u8; 32],
    pub sequence: u64,
    pub listed_at: u64,
    pub content_proven: bool,
    pub fraud_collateral: u128,
    pub public_seed: bool,
}

impl Offer {
    pub fn encode(&self) -> [u8; OFFER_BYTES] {
        let mut out = [0u8; OFFER_BYTES];
        out[0..20].copy_from_slice(&self.author);
        out[20..22].copy_from_slice(&self.epoch.to_be_bytes());
        out[22..26].copy_from_slice(&self.covered.to_be_bytes());
        out[26..58].copy_from_slice(&self.inventory);
        out[58..90].copy_from_slice(&self.keys);
        out[90..106].copy_from_slice(&self.payout.to_be_bytes());
        out[106..122].copy_from_slice(&self.seller_bond.to_be_bytes());
        out[122..130].copy_from_slice(&self.next_nonce.to_be_bytes());
        out[130..138].copy_from_slice(&self.open_order.to_be_bytes());
        out[138] = self.status;
        out[139] = u8::from(self.ever_sold);
        out[140..172].copy_from_slice(&self.previous);
        out[172..204].copy_from_slice(&self.next);
        out[204..212].copy_from_slice(&self.sequence.to_be_bytes());
        out[212..220].copy_from_slice(&self.listed_at.to_be_bytes());
        out[220] = u8::from(self.content_proven);
        out[221..237].copy_from_slice(&self.fraud_collateral.to_be_bytes());
        out[237] = u8::from(self.public_seed);
        out
    }

    pub fn decode(raw: &[u8]) -> Option<Self> {
        if raw.len() != OFFER_BYTES
            || raw[138] > RETIRED
            || raw[139] > 1
            || raw[220] > 1
            || raw[237] > 1
        {
            return None;
        }
        Some(Self {
            author: raw[0..20].try_into().ok()?,
            epoch: u16::from_be_bytes(raw[20..22].try_into().ok()?),
            covered: u32::from_be_bytes(raw[22..26].try_into().ok()?),
            inventory: raw[26..58].try_into().ok()?,
            keys: raw[58..90].try_into().ok()?,
            payout: u128::from_be_bytes(raw[90..106].try_into().ok()?),
            seller_bond: u128::from_be_bytes(raw[106..122].try_into().ok()?),
            next_nonce: u64::from_be_bytes(raw[122..130].try_into().ok()?),
            open_order: u64::from_be_bytes(raw[130..138].try_into().ok()?),
            status: raw[138],
            ever_sold: raw[139] == 1,
            previous: raw[140..172].try_into().ok()?,
            next: raw[172..204].try_into().ok()?,
            sequence: u64::from_be_bytes(raw[204..212].try_into().ok()?),
            listed_at: u64::from_be_bytes(raw[212..220].try_into().ok()?),
            content_proven: raw[220] == 1,
            fraud_collateral: u128::from_be_bytes(raw[221..237].try_into().ok()?),
            public_seed: raw[237] == 1,
        })
    }

    pub fn queued(&self) -> bool {
        matches!(self.status, QUEUED_PRIMARY | QUEUED_SECONDARY)
            && self.open_order == 0
            && (self.public_seed
                || (self.seller_bond > 0
                    && ((self.status == QUEUED_PRIMARY && !self.ever_sold && self.payout > 0)
                        || (self.status == QUEUED_SECONDARY
                            && self.ever_sold
                            && self.content_proven
                            && self.payout == 0))))
    }

    pub fn can_relist(&self) -> bool {
        (matches!(self.status, SOLD_PRIVATE | CANCELLED)
            || (self.public_seed && self.status == RETIRED && self.ever_sold))
            && self.open_order == 0
            && self.seller_bond == 0
            && self.fraud_collateral == 0
            && (!self.public_seed || self.content_proven)
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Order {
    pub offer: [u8; 32],
    pub buyer: [u8; 20],
    pub delivery_key: [u8; 65],
    pub paid: u128,
    pub bond: u128,
    pub seller_payout: u128,
    pub funded_at: u64,
    pub nonce: u64,
    pub state: u8,
    pub delivery_hash: [u8; 32],
    pub challenged: bool,
}

impl Order {
    pub fn encode(&self) -> [u8; ORDER_BYTES] {
        let mut out = [0u8; ORDER_BYTES];
        out[0..32].copy_from_slice(&self.offer);
        out[32..52].copy_from_slice(&self.buyer);
        out[52..117].copy_from_slice(&self.delivery_key);
        out[117..133].copy_from_slice(&self.paid.to_be_bytes());
        out[133..149].copy_from_slice(&self.bond.to_be_bytes());
        out[149..165].copy_from_slice(&self.seller_payout.to_be_bytes());
        out[165..173].copy_from_slice(&self.funded_at.to_be_bytes());
        out[173..181].copy_from_slice(&self.nonce.to_be_bytes());
        out[181] = self.state;
        out[182..214].copy_from_slice(&self.delivery_hash);
        out[214] = u8::from(self.challenged);
        out
    }

    pub fn decode(raw: &[u8]) -> Option<Self> {
        if raw.len() != ORDER_BYTES
            || !(FUNDED..=FALLBACK_PENDING).contains(&raw[181])
            || raw[214] > 1
        {
            return None;
        }
        Some(Self {
            offer: raw[0..32].try_into().ok()?,
            buyer: raw[32..52].try_into().ok()?,
            delivery_key: raw[52..117].try_into().ok()?,
            paid: u128::from_be_bytes(raw[117..133].try_into().ok()?),
            bond: u128::from_be_bytes(raw[133..149].try_into().ok()?),
            seller_payout: u128::from_be_bytes(raw[149..165].try_into().ok()?),
            funded_at: u64::from_be_bytes(raw[165..173].try_into().ok()?),
            nonce: u64::from_be_bytes(raw[173..181].try_into().ok()?),
            state: raw[181],
            delivery_hash: raw[182..214].try_into().ok()?,
            challenged: raw[214] == 1,
        })
    }

    pub fn can_start_fallback(&self, height: u64, window: u64, grace: u64, margin: u64) -> bool {
        self.state == FUNDED
            && self
                .funded_at
                .checked_add(grace)
                .is_some_and(|start| height >= start)
            && self
                .funded_at
                .checked_add(window)
                .and_then(|end| end.checked_sub(margin))
                .is_some_and(|end| height < end)
    }

    pub fn seller_can_reveal(&self, height: u64, window: u64) -> bool {
        self.state == FALLBACK_PENDING
            && self
                .funded_at
                .checked_add(window)
                .is_some_and(|end| height < end)
    }

    pub fn can_refund(&self, height: u64, window: u64) -> bool {
        matches!(self.state, FUNDED | FALLBACK_PENDING)
            && self
                .funded_at
                .checked_add(window)
                .is_some_and(|end| height >= end)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn records_round_trip_without_role_or_amount_loss() {
        let offer = Offer {
            author: [1; 20],
            epoch: 230,
            covered: 5,
            inventory: [2; 32],
            keys: [3; 32],
            payout: 12_345_678_901_234_567_890,
            seller_bond: 9,
            next_nonce: 2,
            open_order: 0,
            status: QUEUED_PRIMARY,
            ever_sold: false,
            previous: [0; 32],
            next: [4; 32],
            sequence: 12,
            listed_at: 100,
            content_proven: true,
            fraud_collateral: 12_345_678_901_234_567_890,
            public_seed: false,
        };
        assert_eq!(Offer::decode(&offer.encode()), Some(offer));
        assert!(offer.queued());
        let order = Order {
            offer: [4; 32],
            buyer: [5; 20],
            delivery_key: [6; 65],
            paid: offer.payout,
            bond: 99,
            seller_payout: offer.payout,
            funded_at: 100,
            nonce: 2,
            state: FUNDED,
            delivery_hash: [7; 32],
            challenged: true,
        };
        assert_eq!(Order::decode(&order.encode()), Some(order));
        assert!(Offer::decode(&offer.encode()[..OFFER_BYTES - 1]).is_none());
        assert!(Order::decode(&order.encode()[..ORDER_BYTES - 1]).is_none());
        let mut retired_state = order.encode();
        retired_state[181] = 5;
        assert!(Order::decode(&retired_state).is_none());
    }

    #[test]
    fn offer_rejects_malformed_bool_flags() {
        let offer = Offer {
            author: [0; 20],
            epoch: 1,
            covered: 1,
            inventory: [0; 32],
            keys: [0; 32],
            payout: 1,
            seller_bond: 1,
            next_nonce: 1,
            open_order: 0,
            status: QUEUED_PRIMARY,
            ever_sold: false,
            previous: [0; 32],
            next: [0; 32],
            sequence: 0,
            listed_at: 0,
            content_proven: false,
            fraud_collateral: 0,
            public_seed: false,
        };
        for index in [139, 220, 237] {
            let mut raw = offer.encode();
            raw[index] = 2;
            assert!(Offer::decode(&raw).is_none());
        }
    }

    #[test]
    fn public_seed_can_queue_without_payment_and_relist_after_a_sale() {
        let offer = Offer {
            author: [0; 20],
            epoch: 1,
            covered: 1,
            inventory: [0; 32],
            keys: [0; 32],
            payout: 0,
            seller_bond: 0,
            next_nonce: 1,
            open_order: 0,
            status: QUEUED_SECONDARY,
            ever_sold: false,
            previous: [0; 32],
            next: [0; 32],
            sequence: 0,
            listed_at: 0,
            content_proven: false,
            fraud_collateral: 0,
            public_seed: true,
        };
        assert!(offer.queued());
        assert_eq!(Offer::decode(&offer.encode()), Some(offer));
        assert!(!Offer {
            public_seed: false,
            ..offer
        }
        .queued());
        let cancelled = Offer {
            status: CANCELLED,
            ..offer
        };
        assert!(!cancelled.can_relist());
        assert!(Offer {
            status: RETIRED,
            ever_sold: true,
            content_proven: true,
            ..offer
        }
        .can_relist());
        let ordinary = Offer {
            public_seed: false,
            ..cancelled
        };
        assert!(ordinary.can_relist());
        assert!(!Offer {
            fraud_collateral: 1,
            ..ordinary
        }
        .can_relist());
    }

    #[test]
    fn fallback_and_refund_windows_do_not_overlap() {
        let order = Order {
            offer: [0; 32],
            buyer: [0; 20],
            delivery_key: [0; 65],
            paid: 1,
            bond: 10,
            seller_payout: 1,
            funded_at: 100,
            nonce: 0,
            state: FUNDED,
            delivery_hash: [0; 32],
            challenged: false,
        };
        assert!(!order.can_start_fallback(129, 100, 30, 10));
        assert!(order.can_start_fallback(130, 100, 30, 10));
        assert!(order.can_start_fallback(189, 100, 30, 10));
        assert!(!order.can_start_fallback(190, 100, 30, 10));
        assert!(!order.can_refund(199, 100));
        assert!(order.can_refund(200, 100));
        let pending = Order {
            state: FALLBACK_PENDING,
            ..order
        };
        assert!(!pending.can_start_fallback(130, 100, 30, 10));
        assert!(pending.seller_can_reveal(199, 100));
        assert!(!pending.seller_can_reveal(200, 100));
        assert!(pending.can_refund(200, 100));
        let closed = Order {
            state: SETTLED,
            ..order
        };
        assert!(!closed.seller_can_reveal(199, 100));
        assert!(!closed.can_refund(200, 100));
    }
}
