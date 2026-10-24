use crate::{curve::{self, IDNA}, market::*, records::*, proof, verifier};
use std::collections::BTreeMap;

#[derive(Clone, Default)]
struct Memory(BTreeMap<Vec<u8>, Vec<u8>>);
impl Store for Memory {
    fn get(&self, k: &[u8]) -> Option<Vec<u8>> { self.0.get(k).cloned() }
    fn put(&mut self, k: &[u8], v: &[u8]) { self.0.insert(k.to_vec(), v.to_vec()); }
}
struct Fixture { store: Memory, held: u128, height: u64 }
const SELLER: Address = [2;20];
const BUYER: Address = [3;20];
const CAP: u128 = 1_000 * IDNA;
impl Fixture {
    fn new() -> Self {
        let mut f = Self { store: Memory::default(), held: 0, height: 1000 };
        f.call(SELLER, 0, |m|m.deploy()).unwrap(); f
    }
    fn call<F>(&mut self, who: Address, amount: u128, action: F) -> Result<Outcome>
    where F: FnOnce(&mut Market<Memory>) -> Result<Outcome> {
        let before = self.store.clone(); let held = self.held;
        self.held += amount;
        let mut m = Market { store: &mut self.store, caller: who, height: self.height };
        match action(&mut m) {
            Ok(out) => {
                for (_, amount) in &out.transfers { self.held = self.held.checked_sub(*amount).expect("solvent transfers"); }
                assert_eq!(m.liabilities().unwrap(), self.held, "no liability can spend someone else's money");
                // Independently sum stored individual stake obligations.
                let mut balances = 0;
                for (k, v) in &self.store.0 {
                    if k.starts_with(b"v9/account/") {
                        let a = Account::decode(v).unwrap(); assert!(a.reserved <= a.balance); balances += a.balance;
                    }
                }
                assert_eq!(balances, Totals::decode(self.store.0.get(TOTALS).unwrap()).unwrap().stakes);
                Ok(out)
            }
            Err(err) => { self.store = before; self.held = held; Err(err) }
        }
    }
    fn inspect<T>(&mut self, f: impl FnOnce(&Market<Memory>) -> T) -> T {
        f(&Market { store: &mut self.store, caller: SELLER, height: self.height })
    }
    fn stake(&mut self, who: Address, amount: u128) { self.call(who, amount, |m|m.deposit(amount, amount)).unwrap(); }
    fn seller(&mut self, who: Address, epoch: u16) -> Digest {
        let id = bundle_id(&who, epoch);
        self.call(who, 0, |m|m.register(Bundle {author:who, epoch, count:5, inventory:[4;32],
            keys:[5;32], sale:0, used:0}, b"synthetic validated inventory")).unwrap(); id
    }
    fn setup(&mut self, pool: bool) -> u64 {
        self.stake(SELLER, CAP); self.stake(BUYER, CAP);
        self.call(SELLER, 1000*IDNA, |m|m.fund(1000*IDNA)).unwrap();
        let id = self.seller(SELLER, 232);
        self.call(SELLER, 0, |m|m.queue_sell(id, 1, CAP, 4000)).unwrap();
        if pool { self.call(BUYER, 0, |m|m.acquire()).unwrap(); }
        self.call(BUYER, CAP, |m|m.queue_buy([4;65], CAP, 4000, CAP)).unwrap();
        self.call(BUYER, 0, |m|m.match_next(IDNA/100_000_000)).unwrap().id
    }
    fn delivered(&mut self, pool: bool) -> (u64, Digest) {
        let id = self.setup(pool);
        self.call(BUYER, 0, |m|m.ready(id, 999, [8;32])).unwrap();
        let cipher = [9; CIPHER_BYTES];
        self.call(SELLER, 0, |m|m.deliver(id, &cipher)).unwrap();
        (id, verifier::sha256(&cipher))
    }
}

#[test]
fn curve_moves_with_acquired_supply_and_settled_demand() {
    assert_eq!(curve::buy(0, 0), Some(500*IDNA));
    assert_eq!(curve::sell(0, 0), Some(409_090_909_090_909_090_909));
    let r = 1000*IDNA;
    assert_eq!(curve::sell(r, 0), Some(500*IDNA));
    let after_sell = curve::sell(500*IDNA, 100).unwrap();
    assert!(after_sell < 500*IDNA);
    assert_eq!(curve::buy(500*IDNA, 100), Some(500*IDNA));
    assert!(curve::buy(r, 0).unwrap() > curve::buy(500*IDNA, 100).unwrap());
    assert!(curve::sell(u128::MAX, 0).is_none());
}

#[test]
fn private_cooperative_delivery_settles_once_in_both_paths() {
    for pool in [false,true] {
        let mut f = Fixture::new(); let (id, hash) = f.delivered(pool);
        let before = f.inspect(|m|m.trade(id).unwrap());
        let out = f.call(BUYER,0,|m|m.accept(id, hash)).unwrap();
        assert!(out.transfers.contains(&(SELLER,before.proceeds)));
        assert_eq!(f.inspect(|m|m.account(&SELLER).unwrap().reserved),0);
        assert_eq!(f.inspect(|m|m.account(&BUYER).unwrap().reserved),0);
        assert_eq!(f.inspect(|m|m.totals().unwrap().entitlements),0);
        assert!(f.call(BUYER,0,|m|m.accept(id, hash)).is_err());
        assert!(f.call(BUYER,0,|m|m.expire_trade(id)).is_err());
        assert!(f.call(SELLER,0,|m|m.deliver(id,&[9;219])).is_err());
    }
}

#[test]
fn pool_cannot_spend_buyer_payments_or_either_stake() {
    let mut f = Fixture::new(); f.stake(SELLER,CAP); f.stake(BUYER,CAP);
    let id = f.seller(SELLER,232);
    f.call(SELLER,0,|m|m.queue_sell(id,1,CAP,4000)).unwrap();
    f.call(BUYER,CAP,|m|m.queue_buy([4;65],CAP,4000,CAP)).unwrap();
    assert_eq!(f.call(SELLER,0,|m|m.acquire()).unwrap_err(),Error::NoLiquidity);
    // A funded buyer can still match, even with zero reserve.
    f.call(BUYER,0,|m|m.match_next(1)).unwrap();
    assert_eq!(f.inspect(|m|m.totals().unwrap().reserve),0);
}

#[test]
fn simultaneous_roles_cannot_reuse_the_same_collateral() {
    let mut f = Fixture::new(); f.stake(SELLER,CAP);
    let id = f.seller(SELLER,232);
    f.call(SELLER,0,|m|m.queue_sell(id,1,CAP,4000)).unwrap();
    assert_eq!(f.call(SELLER,CAP,|m|m.queue_buy([4;65],CAP,4000,CAP)).unwrap_err(),Error::Stake);
    assert!(f.call(SELLER,0,|m|m.withdraw(CAP,true)).is_err());
    f.call(SELLER,0,|m|m.cancel_sell(1)).unwrap();
    f.call(SELLER,0,|m|m.withdraw(CAP,true)).unwrap();
}

#[test]
fn missing_delivery_refunds_escrow_and_requires_seller_topup() {
    let mut f = Fixture::new(); let id=f.setup(true);
    f.call(BUYER,0,|m|m.ready(id,999,[8;32])).unwrap();
    let t=f.inspect(|m|m.trade(id).unwrap()); f.height=t.deliver_by;
    let out=f.call(BUYER,0,|m|m.expire_trade(id)).unwrap();
    assert_eq!(out.transfers.iter().filter(|(to,_)|*to==BUYER).map(|(_,v)|*v).sum::<u128>(),t.price+t.price/10);
    let a=f.inspect(|m|m.account(&SELLER).unwrap()); assert_eq!(a.needs_topup,1);
    let other=f.seller(SELLER,233);
    assert_eq!(f.call(SELLER,0,|m|m.queue_sell(other,1,1,4000)).unwrap_err(),Error::Topup);
    assert!(f.call(SELLER,1,|m|m.deposit(1,1)).is_err());
    f.call(SELLER,t.price/10,|m|m.deposit(t.price/10,CAP)).unwrap();
    f.call(SELLER,0,|m|m.queue_sell(other,1,CAP,4000)).unwrap();
    assert_eq!(f.inspect(|m|m.totals().unwrap().reserve),1000*IDNA);
}

#[test]
fn deposit_withdrawal_requires_full_exit_and_no_open_obligations() {
    let mut f = Fixture::new(); f.stake(SELLER, CAP);
    assert_eq!(f.call(SELLER,0,|m|m.withdraw(1,false)).unwrap_err(),Error::Stake);
    assert_eq!(f.call(SELLER,0,|m|m.withdraw(1,true)).unwrap_err(),Error::Stake);
    let id = f.seller(SELLER,232);
    f.call(SELLER,0,|m|m.queue_sell(id,1,CAP/2,4000)).unwrap();
    assert_eq!(f.call(SELLER,0,|m|m.withdraw(CAP/2,false)).unwrap_err(),Error::Stake);
    assert_eq!(f.call(SELLER,0,|m|m.withdraw(CAP,true)).unwrap_err(),Error::Stake);
    f.call(SELLER,0,|m|m.cancel_sell(1)).unwrap();
    let out = f.call(SELLER,0,|m|m.withdraw(CAP,true)).unwrap();
    assert_eq!(out.transfers,vec![(SELLER,CAP)]);
    let a=f.inspect(|m|m.account(&SELLER).unwrap());
    assert_eq!(a.balance,0); assert_eq!(a.reserved,0); assert_eq!(a.target,0);
}

#[test]
fn penalized_account_can_exit_but_must_restore_target_before_trading_again() {
    let mut f = Fixture::new(); let id=f.setup(false);
    f.call(BUYER,0,|m|m.ready(id,999,[8;32])).unwrap();
    f.height=f.inspect(|m|m.trade(id).unwrap().deliver_by);
    f.call(BUYER,0,|m|m.expire_trade(id)).unwrap();
    let remaining=f.inspect(|m|m.account(&SELLER).unwrap().balance);
    f.call(SELLER,0,|m|m.withdraw(remaining,true)).unwrap();
    let a=f.inspect(|m|m.account(&SELLER).unwrap());
    assert_eq!(a.balance,0); assert_eq!(a.target,CAP); assert_eq!(a.needs_topup,1);
    assert!(f.call(SELLER,1,|m|m.deposit(1,1)).is_err());
    f.call(SELLER,1,|m|m.deposit(1,CAP)).unwrap();
    let other=f.seller(SELLER,233);
    let expiry=f.height+100;
    assert_eq!(f.call(SELLER,0,|m|m.queue_sell(other,1,1,expiry)).unwrap_err(),Error::Topup);
    f.call(SELLER,CAP-1,|m|m.deposit(CAP-1,CAP)).unwrap();
    f.call(SELLER,0,|m|m.queue_sell(other,1,CAP,expiry)).unwrap();
}

#[test]
fn challenge_and_silence_have_different_bounded_charges() {
    for explicit in [false,true] {
        let mut f=Fixture::new(); let (id,_)=f.delivered(false);
        let t=f.inspect(|m|m.trade(id).unwrap());
        if explicit { f.call(BUYER,0,|m|m.challenge(id)).unwrap(); }
        else { f.height=t.accept_by; f.call([7;20],0,|m|m.challenge(id)).unwrap(); }
        // The private proof is independently exercised by the genuine receipt test below.
        f.call(SELLER,0,|m|m.proven_delivery(id)).unwrap();
        let a=f.inspect(|m|m.account(&BUYER).unwrap());
        let charge=t.proof_cost+if explicit {t.price/20} else {0};
        assert_eq!(a.balance,CAP-charge); assert_eq!(a.needs_topup,1);
        assert!(charge<=t.price/10);
        assert!(f.call(SELLER,0,|m|m.proven_delivery(id)).is_err());
    }
}

#[test]
fn unready_buyer_cannot_penalize_seller_or_lose_fifo_position() {
    let mut f=Fixture::new(); let id=f.setup(true);
    f.height=f.inspect(|m|m.trade(id).unwrap().ready_by);
    f.call(BUYER,0,|m|m.expire_trade(id)).unwrap();
    assert_eq!(f.inspect(|m|m.queue(SELL_QUEUE).unwrap()),vec![1]);
    assert_eq!(f.inspect(|m|m.sell(1).unwrap().state),ACQUIRED);
    assert_eq!(f.inspect(|m|m.account(&SELLER).unwrap().balance),CAP);
    assert_eq!(f.inspect(|m|m.account(&BUYER).unwrap().balance),CAP);
    f.call(SELLER,0,|m|m.cancel_sell(1)).unwrap();
    assert_eq!(f.inspect(|m|m.totals().unwrap().reserve),1000*IDNA);
}

#[test]
fn no_late_correction_acceptance_or_deadline_extension() {
    let mut f=Fixture::new(); let (id,hash)=f.delivered(false);
    assert!(f.call(SELLER,0,|m|m.deliver(id,&[10;219])).is_err());
    assert!(f.call(BUYER,0,|m|m.accept(id,[1;32])).is_err());
    f.call(BUYER,0,|m|m.challenge(id)).unwrap();
    assert!(f.call(BUYER,0,|m|m.challenge(id)).is_err());
    f.height=f.inspect(|m|m.trade(id).unwrap().prove_by);
    assert!(f.call(BUYER,0,|m|m.accept(id,hash)).is_err());
    assert!(f.call(SELLER,0,|m|m.proven_delivery(id)).is_err());
    f.call(BUYER,0,|m|m.expire_trade(id)).unwrap();
}

fn unhex(s: &str) -> Vec<u8> { (0..s.len()).step_by(2).map(|i|u8::from_str_radix(&s[i..i+2],16).unwrap()).collect() }
#[test]
fn genuine_private_proof_binds_every_order_field_and_original_ciphertext() {
    let fixture: serde_json::Value=serde_json::from_str(include_str!("../../../proofs/delivery/dist/delivery-v2.json")).unwrap();
    let j=unhex(fixture["journal"].as_str().unwrap()); let seal=unhex(fixture["seal"].as_str().unwrap());
    let mut f=Fixture::new(); let id=f.setup(false); let mut t=f.inspect(|m|m.trade(id).unwrap());
    let mut b=f.inspect(|m|m.bundle(&t.bundle).unwrap());
    t.seller=j[40..60].try_into().unwrap(); t.buyer=j[60..80].try_into().unwrap();
    t.delivery_key=j[80..145].try_into().unwrap(); b.keys=j[145..177].try_into().unwrap();
    t.ciphertext_hash=j[177..209].try_into().unwrap(); t.funded_at=u64::decode(&j[241..249]).unwrap();
    t.funding_root=j[249..281].try_into().unwrap();
    assert!(proof::delivery(&j,&seal,&t,&b,7,&[0x11;20]));
    for offset in [0,4,24,32,40,60,80,145,177,209,241,249] {
        let mut changed=j.clone(); changed[offset]^=1;
        assert!(!proof::delivery_journal_matches(&changed,&t,&b,7,&[0x11;20]),"field {offset}");
    }
    let mut changed=seal.clone(); changed[0]^=1;
    assert!(!proof::delivery(&j,&changed,&t,&b,7,&[0x11;20]));
    t.ciphertext_hash=[0x99;32]; assert!(!proof::delivery(&j,&seal,&t,&b,7,&[0x11;20]));
    assert!(proof::valid_delivery_key(&j[80..145].try_into().unwrap()));
    assert!(!proof::valid_delivery_key(&[4;65]));
}

#[test]
fn listing_reserves_deposit_atomically_and_preserves_cancelled_commitments() {
    let mut f=Fixture::new();
    let b=Bundle { author: SELLER, epoch:233, count:5, inventory:[4;32], keys:[5;32], sale:0, used:0 };
    let id=bundle_id(&SELLER,233);
    assert_eq!(f.call(SELLER,0,|m|m.list(b.clone(),b"inventory",1,CAP,4000)).unwrap_err(),Error::Stake);
    assert!(f.inspect(|m|m.bundle(&id)).is_err(),"failed listing leaves no claim behind");
    f.stake(SELLER,CAP);
    f.call(SELLER,0,|m|m.list(b.clone(),b"inventory",1,CAP,4000)).unwrap();
    assert_eq!(f.inspect(|m|m.account(&SELLER).unwrap().reserved),CAP);
    assert!(f.call(SELLER,0,|m|m.list(b.clone(),b"inventory",1,CAP,4000)).is_err());
    f.call(SELLER,0,|m|m.cancel_sell(1)).unwrap();
    let mut changed=b.clone();changed.keys=[6;32];
    assert_eq!(f.call(SELLER,0,|m|m.list(changed,b"inventory",1,CAP,4000)).unwrap_err(),Error::Proof);
    f.call(SELLER,0,|m|m.list(b,b"inventory",1,CAP,4000)).unwrap();
}

#[test]
fn dispute_requires_original_content_even_when_delivery_has_a_genuine_seal() {
    let p:serde_json::Value=serde_json::from_str(include_str!("../../../proofs/fast-content/dist/synthetic-v5-one-flip.json")).unwrap();
    let cj=unhex(p["journal"].as_str().unwrap());let cs=unhex(p["seal"].as_str().unwrap());
    let b=Bundle {author:cj[56..76].try_into().unwrap(),epoch:u16::decode(&cj[76..78]).unwrap(),
        count:u16::decode(&cj[78..80]).unwrap(),inventory:cj[144..176].try_into().unwrap(),
        keys:cj[241..273].try_into().unwrap(),sale:0,used:0};
    assert!(proof::content(&cj,&cs,&b,&[0x11;20]));
    for offset in [4,36,56,76,78,80,112,144,241] {
        let mut changed=cj.clone();changed[offset]^=1;
        assert!(!proof::content_journal_matches(&changed,&b,&[0x11;20]),"field {offset}");
    }
    let mut forged=b.clone();forged.keys=[0x55;32];
    let mut forged_journal=cj.clone();forged_journal[241..273].copy_from_slice(&forged.keys);
    assert!(proof::content_journal_matches(&forged_journal,&forged,&[0x11;20]));
    assert!(!proof::content(&forged_journal,&cs,&forged,&[0x11;20]));
    let d:serde_json::Value=serde_json::from_str(include_str!("../../../proofs/delivery/dist/delivery-v2.json")).unwrap();
    let dj=unhex(d["journal"].as_str().unwrap());let ds=unhex(d["seal"].as_str().unwrap());
    let mut f=Fixture::new();let id=f.setup(false);let mut t=f.inspect(|m|m.trade(id).unwrap());
    t.seller=dj[40..60].try_into().unwrap();t.buyer=dj[60..80].try_into().unwrap();
    t.delivery_key=dj[80..145].try_into().unwrap();t.ciphertext_hash=dj[177..209].try_into().unwrap();
    t.funded_at=u64::decode(&dj[241..249]).unwrap();t.funding_root=dj[249..281].try_into().unwrap();
    forged.author=t.seller;forged.keys=dj[145..177].try_into().unwrap();
    assert!(proof::delivery(&dj,&ds,&t,&forged,7,&[0x11;20]));
    assert!(!proof::dispute(&[],&[],&dj,&ds,&t,&forged,7,&[0x11;20]));
    assert!(!proof::dispute(&cj,&cs,&dj,&ds,&t,&forged,7,&[0x11;20]),"unrelated genuine proofs cannot be combined");
}
