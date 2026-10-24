use alloc::vec::Vec;
pub type Address = [u8; 20];
pub type Digest = [u8; 32];

/// Fixed big-endian records; no floats, JSON parser, or unchecked truncation on chain.
pub trait Codec: Sized {
    fn put(&self, out: &mut Vec<u8>);
    fn take(input: &mut &[u8]) -> Option<Self>;
    fn encode(&self) -> Vec<u8> { let mut b = Vec::new(); self.put(&mut b); b }
    fn decode(mut input: &[u8]) -> Option<Self> {
        let v = Self::take(&mut input)?;
        if input.is_empty() { Some(v) } else { None }
    }
}
macro_rules! integer {
    ($($t:ty),*) => {$ (impl Codec for $t {
        fn put(&self, out: &mut Vec<u8>) { out.extend_from_slice(&self.to_be_bytes()) }
        fn take(input: &mut &[u8]) -> Option<Self> {
            let n = core::mem::size_of::<Self>();
            let (v, rest) = (input.get(..n)?, input.get(n..)?);
            let v = Self::from_be_bytes(v.try_into().ok()?); *input = rest; Some(v)
        }
    })*};
}
integer!(u8, u16, u32, u64, u128);
impl<const N: usize> Codec for [u8; N] {
    fn put(&self, out: &mut Vec<u8>) { out.extend_from_slice(self) }
    fn take(input: &mut &[u8]) -> Option<Self> {
        let v = input.get(..N)?.try_into().ok()?; *input = input.get(N..)?; Some(v)
    }
}
macro_rules! record {
    ($name:ident { $($field:ident: $type:ty),* $(,)? }) => {
        #[derive(Clone, Debug, PartialEq, Eq)]
        pub struct $name { $(pub $field: $type),* }
        impl Codec for $name {
            fn put(&self, out: &mut Vec<u8>) { $(self.$field.put(out);)* }
            fn take(input: &mut &[u8]) -> Option<Self> { Some(Self { $($field: <$type>::take(input)?),* }) }
        }
    };
}
record!(Totals {
    reserve: u128, escrow: u128, stakes: u128, entitlements: u128,
    inventory: u128, next_sell: u64, next_buy: u64, next_trade: u64,
});
impl Default for Totals {
    fn default() -> Self { Self { reserve: 0, escrow: 0, stakes: 0, entitlements: 0,
        inventory: 0, next_sell: 1, next_buy: 1, next_trade: 1 } }
}
record!(Account { balance: u128, reserved: u128, target: u128, needs_topup: u8 });
impl Default for Account {
    fn default() -> Self { Self { balance: 0, reserved: 0, target: 0, needs_topup: 0 } }
}
record!(Bundle {
    author: Address, epoch: u16, count: u16, inventory: Digest, keys: Digest,
    sale: u64, used: u8,
});
record!(Sell {
    bundle: Digest, seller: Address, minimum: u128, capacity: u128,
    expires: u64, queued_at: u64, proceeds: u128, state: u8,
});
record!(Buy {
    buyer: Address, delivery_key: [u8; 65], maximum: u128, expires: u64,
    state: u8,
});
record!(Trade {
    sell: u64, buy: u64, bundle: Digest, seller: Address, buyer: Address,
    delivery_key: [u8; 65], price: u128, proceeds: u128, pool_funded: u8,
    ready_by: u64, funded_at: u64, funding_root: Digest, deliver_by: u64,
    accept_by: u64, prove_by: u64, ciphertext_hash: Digest,
    proof_cost: u128, state: u8, explicit_challenge: u8,
});

pub const QUEUED: u8 = 1;
pub const ACQUIRED: u8 = 2;
pub const ASSIGNED: u8 = 3;
pub const CLOSED: u8 = 4;
pub const CANCELLED: u8 = 5;
pub const AWAITING_READY: u8 = 1;
pub const AWAITING_DELIVERY: u8 = 2;
pub const DELIVERED: u8 = 3;
pub const DISPUTED: u8 = 4;
pub const SETTLED: u8 = 5;
pub const REFUNDED: u8 = 6;
