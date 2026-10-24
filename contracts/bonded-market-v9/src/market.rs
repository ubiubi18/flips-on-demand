//! Settlement engine. Store writes and transfers belong to one atomic VM call.
//! A host must roll back the whole call on error; no intermediate result is valid.
use alloc::{vec, vec::Vec};
use crate::{curve, records::*, verifier};

pub const MAX_QUEUE: usize = 64;
pub const MAX_QUEUE_BLOCKS: u64 = 4_320;
pub const READY_BLOCKS: u64 = 180;
pub const DELIVERY_BLOCKS: u64 = 720;
pub const ACCEPT_BLOCKS: u64 = 180;
pub const PROOF_BLOCKS: u64 = 12_960;
pub const CIPHER_BYTES: usize = 219;
pub const TOTALS: &[u8] = b"v9/totals";
pub const SELL_QUEUE: &[u8] = b"v9/sells";
pub const BUY_QUEUE: &[u8] = b"v9/buys";
pub const ACTIVE_TRADES: &[u8] = b"v9/active";

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Error { Missing, Malformed, Overflow, Unauthorized, State, Expired,
    Stake, Topup, Limit, NoMatch, NoLiquidity, Proof, Duplicate, Full }
pub type Result<T> = core::result::Result<T, Error>;
pub trait Store { fn get(&self, key: &[u8]) -> Option<Vec<u8>>; fn put(&mut self, key: &[u8], value: &[u8]); }
#[derive(Default, Debug)]
pub struct Outcome { pub id: u64, pub transfers: Vec<(Address, u128)> }
impl Outcome {
    fn pay(&mut self, who: Address, amount: u128) { if amount != 0 { self.transfers.push((who, amount)); } }
}
pub struct Market<'a, S: Store> { pub store: &'a mut S, pub caller: Address, pub height: u64 }
pub fn key(prefix: &[u8], suffix: &[u8]) -> Vec<u8> { [prefix, suffix].concat() }
pub fn bundle_id(author: &Address, epoch: u16) -> Digest {
    verifier::sha256(&[b"fod:bundle:v9".as_slice(), author.as_slice(), &epoch.to_be_bytes()].concat())
}
fn add(a: u128, b: u128) -> Result<u128> { a.checked_add(b).ok_or(Error::Overflow) }
fn sub(a: u128, b: u128) -> Result<u128> { a.checked_sub(b).ok_or(Error::State) }
fn later(a: u64, b: u64) -> Result<u64> { a.checked_add(b).ok_or(Error::Overflow) }

impl<S: Store> Market<'_, S> {
    fn read<T: Codec>(&self, k: &[u8]) -> Result<T> {
        T::decode(&self.store.get(k).ok_or(Error::Missing)?).ok_or(Error::Malformed)
    }
    fn write<T: Codec>(&mut self, k: &[u8], v: &T) { self.store.put(k, &v.encode()); }
    pub fn totals(&self) -> Result<Totals> { self.read(TOTALS) }
    pub fn account(&self, who: &Address) -> Result<Account> {
        match self.store.get(&key(b"v9/account/", who)) {
            None => Ok(Account::default()), Some(v) => Account::decode(&v).ok_or(Error::Malformed)
        }
    }
    fn save_account(&mut self, who: &Address, a: &Account) { self.write(&key(b"v9/account/", who), a); }
    pub fn bundle(&self, id: &Digest) -> Result<Bundle> { self.read(&key(b"v9/bundle/", id)) }
    fn save_bundle(&mut self, id: &Digest, b: &Bundle) { self.write(&key(b"v9/bundle/", id), b); }
    pub fn sell(&self, id: u64) -> Result<Sell> { self.read(&key(b"v9/sell/", &id.to_be_bytes())) }
    fn save_sell(&mut self, id: u64, s: &Sell) { self.write(&key(b"v9/sell/", &id.to_be_bytes()), s); }
    pub fn buy(&self, id: u64) -> Result<Buy> { self.read(&key(b"v9/buy/", &id.to_be_bytes())) }
    fn save_buy(&mut self, id: u64, b: &Buy) { self.write(&key(b"v9/buy/", &id.to_be_bytes()), b); }
    pub fn trade(&self, id: u64) -> Result<Trade> { self.read(&key(b"v9/trade/", &id.to_be_bytes())) }
    fn save_trade(&mut self, id: u64, t: &Trade) { self.write(&key(b"v9/trade/", &id.to_be_bytes()), t); }
    pub fn queue(&self, k: &[u8]) -> Result<Vec<u64>> {
        let raw = self.store.get(k).unwrap_or_default();
        if raw.len() % 8 != 0 || raw.len() > MAX_QUEUE * 8 { return Err(Error::Malformed); }
        raw.chunks_exact(8).map(|c| u64::decode(c).ok_or(Error::Malformed)).collect()
    }
    fn save_queue(&mut self, k: &[u8], q: &[u64]) {
        let mut out = Vec::with_capacity(q.len() * 8);
        for id in q { id.put(&mut out); } self.store.put(k, &out);
    }
    fn enqueue(&mut self, k: &[u8], id: u64) -> Result<()> {
        let mut q = self.queue(k)?;
        if q.len() >= MAX_QUEUE || q.contains(&id) { return Err(Error::Full); }
        q.push(id); q.sort_unstable(); self.save_queue(k, &q); Ok(())
    }
    fn dequeue(&mut self, k: &[u8], id: u64) -> Result<()> {
        let mut q = self.queue(k)?;
        let at = q.iter().position(|x| *x == id).ok_or(Error::State)?;
        q.remove(at); self.save_queue(k, &q); Ok(())
    }
    fn eligible(&self, who: &Address) -> Result<bool> {
        let a = self.account(who)?;
        Ok(a.needs_topup == 0 && a.balance >= a.target && a.balance >= a.reserved)
    }
    fn lock(&mut self, who: &Address, capacity: u128) -> Result<()> {
        let mut a = self.account(who)?;
        if a.needs_topup != 0 || a.balance < a.target { return Err(Error::Topup); }
        if capacity == 0 || add(a.reserved, capacity)? > a.balance || add(a.reserved, capacity)? > a.target {
            return Err(Error::Stake);
        }
        a.reserved = add(a.reserved, capacity)?; self.save_account(who, &a); Ok(())
    }
    fn release(&mut self, who: &Address, capacity: u128, charge: u128, totals: &mut Totals) -> Result<()> {
        let mut a = self.account(who)?;
        if charge > capacity { return Err(Error::Stake); }
        a.reserved = sub(a.reserved, capacity)?;
        a.balance = sub(a.balance, charge)?;
        totals.stakes = sub(totals.stakes, charge)?;
        // Any adverse deduction requires a real top-up, even if excess covered it.
        if charge != 0 { a.needs_topup = 1; a.target = a.target.max(add(a.balance, charge)?); }
        self.save_account(who, &a); Ok(())
    }
    fn valid_expiry(&self, expires: u64) -> Result<()> {
        if expires <= self.height || expires > later(self.height, MAX_QUEUE_BLOCKS)? { Err(Error::Expired) } else { Ok(()) }
    }
    pub fn liabilities(&self) -> Result<u128> {
        let t = self.totals()?; add(add(t.reserve, t.escrow)?, add(t.stakes, t.entitlements)?)
    }
    pub fn deploy(&mut self) -> Result<Outcome> {
        if self.store.get(TOTALS).is_some() { return Err(Error::Duplicate); }
        self.write(TOTALS, &Totals::default()); Ok(Outcome::default())
    }
    pub fn deposit(&mut self, amount: u128, target: u128) -> Result<Outcome> {
        let mut totals = self.totals()?;
        let mut a = self.account(&self.caller)?;
        if amount == 0 || target < a.target || target < a.reserved || target == 0 { return Err(Error::Stake); }
        a.balance = add(a.balance, amount)?; a.target = target;
        if a.balance >= a.target { a.needs_topup = 0; }
        totals.stakes = add(totals.stakes, amount)?;
        self.save_account(&{self.caller}, &a); self.write(TOTALS, &totals); Ok(Outcome::default())
    }
    pub fn withdraw(&mut self, amount: u128, exit: bool) -> Result<Outcome> {
        let mut totals = self.totals()?; let mut a = self.account(&self.caller)?;
        // A deposit is reusable until the account explicitly finishes trading.
        // All orders/trades reserve positive collateral, so a zero reservation
        // is required even when withdrawing otherwise unused funds.
        if !exit || a.reserved != 0 || amount == 0 || amount != a.balance { return Err(Error::Stake); }
        // A settled penalty must not trap the remaining funds. Retain the old
        // target/penalty marker so exiting cannot evade the re-entry threshold.
        if a.needs_topup == 0 { a.target = 0; }
        a.balance = sub(a.balance, amount)?; totals.stakes = sub(totals.stakes, amount)?;
        self.save_account(&{self.caller}, &a); self.write(TOTALS, &totals);
        Ok(Outcome { id: 0, transfers: vec![(self.caller, amount)] })
    }
    pub fn fund(&mut self, amount: u128) -> Result<Outcome> {
        if amount == 0 { return Err(Error::Limit); }
        let mut totals = self.totals()?; totals.reserve = add(totals.reserve, amount)?;
        self.write(TOTALS, &totals); Ok(Outcome::default())
    }
    /// Called only after the host authenticates the canonical public session.
    pub(crate) fn register(&mut self, b: Bundle, cids: &[u8]) -> Result<Outcome> {
        self.totals()?;
        if b.author != self.caller || b.count == 0 || b.count > 32 || b.used != 0 || b.sale != 0 {
            return Err(Error::Proof);
        }
        let id = bundle_id(&b.author, b.epoch);
        if self.store.get(&key(b"v9/bundle/", &id)).is_some() { return Err(Error::Duplicate); }
        self.save_bundle(&id, &b); self.store.put(&key(b"v9/cids/", &id), cids);
        Ok(Outcome::default())
    }
    pub fn queue_sell(&mut self, id: Digest, minimum: u128, capacity: u128, expires: u64) -> Result<Outcome> {
        self.valid_expiry(expires)?;
        let mut totals = self.totals()?; let mut b = self.bundle(&id)?;
        if b.author != self.caller { return Err(Error::Unauthorized); }
        if b.used != 0 || b.sale != 0 || minimum == 0 || minimum > capacity { return Err(Error::Limit); }
        self.lock(&{self.caller}, capacity)?;
        let sale = totals.next_sell; totals.next_sell = later(sale, 1)?;
        self.enqueue(SELL_QUEUE, sale)?;
        self.save_sell(sale, &Sell { bundle: id, seller: self.caller, minimum, capacity,
            expires, queued_at: self.height, proceeds: 0, state: QUEUED });
        b.sale = sale; self.save_bundle(&id, &b); self.write(TOTALS, &totals);
        Ok(Outcome { id: sale, ..Outcome::default() })
    }
    /// One atomic user action stores the claim and reserves collateral.
    /// A cancelled listing can be requeued, but its original commitments cannot change.
    pub(crate) fn list(&mut self, b: Bundle, cids: &[u8], minimum: u128, capacity: u128, expires: u64) -> Result<Outcome> {
        if b.author != self.caller { return Err(Error::Unauthorized); }
        let id = bundle_id(&b.author, b.epoch);
        if self.store.get(&key(b"v9/bundle/", &id)).is_some() {
            let saved = self.bundle(&id)?;
            if saved.author != b.author || saved.epoch != b.epoch || saved.count != b.count ||
                saved.inventory != b.inventory || saved.keys != b.keys ||
                self.store.get(&key(b"v9/cids/", &id)).as_deref() != Some(cids) { return Err(Error::Proof); }
        } else { self.register(b, cids)?; }
        self.queue_sell(id, minimum, capacity, expires)
    }
    pub fn queue_buy(&mut self, delivery_key: [u8; 65], maximum: u128, expires: u64, attached: u128) -> Result<Outcome> {
        self.valid_expiry(expires)?;
        // A fully validated curve point is required by the host before this call.
        if maximum == 0 || maximum != attached { return Err(Error::Limit); }
        let mut totals = self.totals()?; self.lock(&{self.caller}, maximum)?;
        let id = totals.next_buy; totals.next_buy = later(id, 1)?;
        totals.escrow = add(totals.escrow, maximum)?;
        self.enqueue(BUY_QUEUE, id)?;
        self.save_buy(id, &Buy { buyer: self.caller, delivery_key, maximum, expires, state: QUEUED });
        self.write(TOTALS, &totals); Ok(Outcome { id, ..Outcome::default() })
    }
    pub fn cancel_sell(&mut self, id: u64) -> Result<Outcome> {
        let mut s = self.sell(id)?; let mut totals = self.totals()?; let mut b = self.bundle(&s.bundle)?;
        if s.seller != self.caller && self.height < s.expires { return Err(Error::Unauthorized); }
        if !matches!(s.state, QUEUED | ACQUIRED) { return Err(Error::State); }
        if s.state == ACQUIRED {
            totals.entitlements = sub(totals.entitlements, s.proceeds)?;
            totals.reserve = add(totals.reserve, s.proceeds)?;
            totals.inventory = sub(totals.inventory, curve::STANDARD_WEIGHT)?;
        }
        self.dequeue(SELL_QUEUE, id)?;
        self.release(&s.seller, s.capacity, 0, &mut totals)?;
        s.state = CANCELLED; b.sale = 0;
        self.save_sell(id, &s); self.save_bundle(&s.bundle, &b); self.write(TOTALS, &totals);
        Ok(Outcome::default())
    }
    pub fn cancel_buy(&mut self, id: u64) -> Result<Outcome> {
        let mut b = self.buy(id)?; let mut totals = self.totals()?;
        if b.buyer != self.caller && self.height < b.expires { return Err(Error::Unauthorized); }
        if b.state != QUEUED { return Err(Error::State); }
        self.dequeue(BUY_QUEUE, id)?;
        self.release(&b.buyer, b.maximum, 0, &mut totals)?;
        totals.escrow = sub(totals.escrow, b.maximum)?; b.state = CANCELLED;
        self.save_buy(id, &b); self.write(TOTALS, &totals);
        Ok(Outcome { id, transfers: vec![(b.buyer, b.maximum)] })
    }
    /// Reserve-backed acquisition. Proceeds remain held until private delivery settles.
    pub fn acquire(&mut self) -> Result<Outcome> {
        let mut totals = self.totals()?;
        let mut selected = None;
        for id in self.queue(SELL_QUEUE)? {
            let s = self.sell(id)?;
            if s.state != QUEUED || s.expires <= self.height || !self.eligible(&s.seller)? { continue; }
            let p = curve::sell(totals.reserve, totals.inventory).ok_or(Error::Overflow)?;
            if p < s.minimum || p > s.capacity || p > totals.reserve { continue; }
            selected = Some(id); break;
        }
        let id = selected.ok_or(Error::NoLiquidity)?;
        let mut s = self.sell(id)?; let mut b = self.bundle(&s.bundle)?;
        let p = curve::sell(totals.reserve, totals.inventory).ok_or(Error::Overflow)?;
        totals.reserve = sub(totals.reserve, p)?; totals.entitlements = add(totals.entitlements, p)?;
        totals.inventory = add(totals.inventory, curve::STANDARD_WEIGHT)?;
        s.proceeds = p; s.state = ACQUIRED; b.used = 1;
        self.save_sell(id, &s); self.save_bundle(&s.bundle, &b); self.write(TOTALS, &totals);
        Ok(Outcome { id, ..Outcome::default() })
    }
    /// Earliest compatible funded buyer and seller, with FIFO within each queue.
    pub fn match_next(&mut self, fee_per_gas: u128) -> Result<Outcome> {
        let mut totals = self.totals()?;
        let price = curve::buy(totals.reserve, totals.inventory).ok_or(Error::Overflow)?;
        let quoted_proceeds = curve::sell(totals.reserve, totals.inventory).ok_or(Error::Overflow)?;
        // Read each storage record once; a full pair of bounded queues must
        // remain matchable within the native block gas limit.
        let mut sellers = Vec::new();
        for sid in self.queue(SELL_QUEUE)? {
            let s = self.sell(sid)?;
            let proceeds = if s.state == ACQUIRED { s.proceeds } else { quoted_proceeds };
            if matches!(s.state, QUEUED | ACQUIRED) && s.expires > self.height &&
                price <= s.capacity && proceeds >= s.minimum && self.eligible(&s.seller)? {
                sellers.push((sid, s.seller));
            }
        }
        let mut chosen = None;
        for bid in self.queue(BUY_QUEUE)? {
            let b = self.buy(bid)?;
            if b.state != QUEUED || b.expires <= self.height || price > b.maximum || !self.eligible(&b.buyer)? { continue; }
            if let Some((sid, _)) = sellers.iter().find(|(_, seller)| *seller != b.buyer) {
                chosen = Some((*sid, bid)); break;
            }
        }
        let (sid, bid) = chosen.ok_or(Error::NoMatch)?;
        let mut s = self.sell(sid)?; let mut b = self.buy(bid)?;
        let price = curve::buy(totals.reserve, totals.inventory).ok_or(Error::Overflow)?;
        let pool_funded = u8::from(s.state == ACQUIRED);
        let proceeds = if pool_funded == 1 { s.proceeds } else {
            curve::sell(totals.reserve, totals.inventory).ok_or(Error::Overflow)? };
        let id = totals.next_trade; totals.next_trade = later(id, 1)?;
        // Fee reimbursement is a disclosed cap; never more than 5% of trade value.
        let proof_cost = fee_per_gas.checked_mul(2_500_000).ok_or(Error::Overflow)?.min(price / 20);
        let t = Trade { sell: sid, buy: bid, bundle: s.bundle, seller: s.seller, buyer: b.buyer,
            delivery_key: b.delivery_key, price, proceeds, pool_funded, ready_by: later(self.height, READY_BLOCKS)?,
            funded_at: 0, funding_root: [0;32], deliver_by: 0, accept_by: 0, prove_by: 0,
            ciphertext_hash: [0;32], proof_cost, state: AWAITING_READY, explicit_challenge: 0 };
        // Assigned entries retain their bounded queue slots until terminal.
        // An unavailable assignment can therefore restore its seller without
        // depending on someone else cancelling a newly filled queue.
        totals.escrow = sub(totals.escrow, sub(b.maximum, price)?)?;
        self.enqueue(ACTIVE_TRADES, id)?;
        s.state = ASSIGNED; b.state = ASSIGNED;
        self.save_sell(sid, &s); self.save_buy(bid, &b); self.save_trade(id, &t); self.write(TOTALS, &totals);
        let mut result = Outcome { id, ..Outcome::default() }; result.pay(b.buyer, b.maximum - price); Ok(result)
    }
    /// Buyer confirms all original CID blobs are already cached locally.
    pub fn ready(&mut self, id: u64, canonical_height: u64, canonical_root: Digest) -> Result<Outcome> {
        let mut t = self.trade(id)?;
        if self.caller != t.buyer { return Err(Error::Unauthorized); }
        if t.state != AWAITING_READY || self.height >= t.ready_by { return Err(Error::Expired); }
        if canonical_height != self.height.checked_sub(1).ok_or(Error::Proof)? || canonical_root == [0;32] { return Err(Error::Proof); }
        t.funded_at = canonical_height; t.funding_root = canonical_root;
        t.deliver_by = later(self.height, DELIVERY_BLOCKS)?; t.state = AWAITING_DELIVERY;
        self.save_trade(id, &t); Ok(Outcome::default())
    }
    pub fn deliver(&mut self, id: u64, ciphertext: &[u8]) -> Result<Outcome> {
        let mut t = self.trade(id)?;
        if self.caller != t.seller { return Err(Error::Unauthorized); }
        if t.state != AWAITING_DELIVERY || self.height >= t.deliver_by { return Err(Error::Expired); }
        if ciphertext.len() != CIPHER_BYTES { return Err(Error::Malformed); }
        t.ciphertext_hash = verifier::sha256(ciphertext);
        t.accept_by = later(self.height, ACCEPT_BLOCKS)?;
        t.prove_by = later(t.accept_by, PROOF_BLOCKS)?; t.state = DELIVERED;
        self.store.put(&key(b"v9/ciphertext/", &id.to_be_bytes()), ciphertext);
        self.save_trade(id, &t); Ok(Outcome::default())
    }
    pub fn challenge(&mut self, id: u64) -> Result<Outcome> {
        let mut t = self.trade(id)?;
        if t.state != DELIVERED || self.height >= t.prove_by { return Err(Error::Expired); }
        if self.caller != t.buyer && self.height < t.accept_by { return Err(Error::Unauthorized); }
        t.explicit_challenge = u8::from(self.caller == t.buyer); t.state = DISPUTED;
        self.save_trade(id, &t); Ok(Outcome::default())
    }
    pub fn accept(&mut self, id: u64, ciphertext_hash: Digest) -> Result<Outcome> {
        let t = self.trade(id)?;
        if self.caller != t.buyer { return Err(Error::Unauthorized); }
        if !matches!(t.state, DELIVERED | DISPUTED) || self.height >= t.prove_by || ciphertext_hash != t.ciphertext_hash {
            return Err(Error::State);
        }
        self.settle(id, 0, [0;20])
    }
    /// Host checks exact original delivery journal and pinned seal before this.
    pub(crate) fn proven_delivery(&mut self, id: u64) -> Result<Outcome> {
        let t = self.trade(id)?;
        if !matches!(t.state, DELIVERED | DISPUTED) || self.height >= t.prove_by ||
            (t.state == DELIVERED && self.height < t.accept_by) { return Err(Error::State); }
        let charge = add(t.proof_cost, if t.explicit_challenge == 1 { t.price / 20 } else { 0 })?;
        self.settle(id, charge, self.caller)
    }
    fn settle(&mut self, id: u64, buyer_charge: u128, proof_submitter: Address) -> Result<Outcome> {
        let mut t = self.trade(id)?; let mut totals = self.totals()?;
        let mut s = self.sell(t.sell)?; let mut b = self.buy(t.buy)?; let mut bundle = self.bundle(&t.bundle)?;
        totals.escrow = sub(totals.escrow, t.price)?;
        if t.pool_funded == 1 {
            totals.entitlements = sub(totals.entitlements, t.proceeds)?;
            totals.inventory = sub(totals.inventory, curve::STANDARD_WEIGHT)?;
            totals.reserve = add(totals.reserve, t.price)?;
        } else { totals.reserve = add(totals.reserve, sub(t.price, t.proceeds)?)?; }
        self.release(&s.seller, s.capacity, 0, &mut totals)?;
        self.release(&b.buyer, b.maximum, buyer_charge, &mut totals)?;
        self.dequeue(SELL_QUEUE, t.sell)?; self.dequeue(BUY_QUEUE, t.buy)?;
        self.dequeue(ACTIVE_TRADES, id)?;
        t.state = SETTLED; s.state = CLOSED; b.state = CLOSED; bundle.used = 1;
        self.save_trade(id, &t); self.save_sell(t.sell, &s); self.save_buy(t.buy, &b);
        self.save_bundle(&t.bundle, &bundle); self.write(TOTALS, &totals);
        let mut result = Outcome { id, ..Outcome::default() }; result.pay(s.seller, t.proceeds);
        result.pay(proof_submitter, t.proof_cost.min(buyer_charge));
        result.pay(s.seller, buyer_charge - t.proof_cost.min(buyer_charge)); Ok(result)
    }
    pub fn expire_trade(&mut self, id: u64) -> Result<Outcome> {
        let mut t = self.trade(id)?; let mut totals = self.totals()?;
        let unready = t.state == AWAITING_READY && self.height >= t.ready_by;
        let absent = t.state == AWAITING_DELIVERY && self.height >= t.deliver_by;
        let unproved = matches!(t.state, DELIVERED | DISPUTED) && self.height >= t.prove_by;
        if !unready && !absent && !unproved { return Err(Error::Expired); }
        let mut s = self.sell(t.sell)?; let mut b = self.buy(t.buy)?; let mut bundle = self.bundle(&t.bundle)?;
        totals.escrow = sub(totals.escrow, t.price)?;
        self.release(&b.buyer, b.maximum, 0, &mut totals)?;
        self.dequeue(BUY_QUEUE, t.buy)?;
        let mut result = Outcome { id, ..Outcome::default() }; result.pay(t.buyer, t.price);
        if unready {
            // No delivery obligation began. Restore seller FIFO position with no blame.
            s.state = if t.pool_funded == 1 { ACQUIRED } else { QUEUED };
        } else {
            let charge = t.price / 10;
            self.release(&s.seller, s.capacity, charge, &mut totals)?; result.pay(t.buyer, charge);
            if t.pool_funded == 1 {
                totals.entitlements = sub(totals.entitlements, t.proceeds)?;
                totals.reserve = add(totals.reserve, t.proceeds)?;
                totals.inventory = sub(totals.inventory, curve::STANDARD_WEIGHT)?;
            }
            s.state = CLOSED; bundle.used = 1;
            self.dequeue(SELL_QUEUE, t.sell)?;
        }
        self.dequeue(ACTIVE_TRADES, id)?;
        t.state = REFUNDED; b.state = CLOSED;
        self.save_trade(id, &t); self.save_sell(t.sell, &s); self.save_buy(t.buy, &b);
        self.save_bundle(&t.bundle, &bundle); self.write(TOTALS, &totals); Ok(result)
    }
}
