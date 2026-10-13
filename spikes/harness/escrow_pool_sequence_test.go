package harness

// The pool's call sequence through the real runtime: two buyers take units at
// the pool's own quotes, a wrong amount and a replayed proof are refused, the
// delivery, confirmation, deadline, refund, and strike rules hold, and the
// accounting invariant is checked after every move.
//
// The bundle is seeded through the test hook, so this exercises the call and
// accounting paths rather than the registration proof; the sealed-receipt path
// is covered by TestPoolSupplyFromASealedReceipt.

import (
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

// assertPoolInvariant checks the contract's own rule: the liquid balance is at
// least the bundle's reserve plus all withdrawal credits.
func assertPoolInvariant(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte, commitment []byte) {
	t.Helper()
	view := readPoolView(t, env, api, code, commitment)
	_, out := escrowCall(t, env, api, code, "accounting", "0")
	var accounting accountingJSON
	if err := json.Unmarshal([]byte(out), &accounting); err != nil {
		t.Fatalf("parse accounting %q: %v", out, err)
	}
	// The invariant `SPEC.md` 5.1 states, read from the contract's own totals:
	// everything the pool holds covers the reserve sellers are paid from, the
	// buyer escrow it is still holding, and the withdrawal credits it owes.
	liabilities := new(big.Int).SetUint64(accounting.ReserveTotal)
	liabilities.Add(liabilities, new(big.Int).SetUint64(accounting.PendingTotal))
	liabilities.Add(liabilities, new(big.Int).SetUint64(accounting.CreditTotal))
	if new(big.Int).SetUint64(accounting.Liquid).Cmp(liabilities) < 0 {
		t.Fatalf("liquid %d is below reserve %d plus pending %d plus credits %d (bundle %s holds reserve %s, pending %s)",
			accounting.Liquid, accounting.ReserveTotal, accounting.PendingTotal, accounting.CreditTotal,
			"this one", view.Reserve, view.Pending)
	}
}

func TestPoolCallSequenceAndInvariants(t *testing.T) {
	code, err := os.ReadFile(escrowWasmPath)
	if err != nil {
		t.Fatalf("read escrow module: %v", err)
	}
	env := NewMinimalEnv()
	contract := addressFromHex(t, "1111111111111111111111111111111111111111")
	env.SetContractAddress(contract)
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	env.SetBlockNumber(1000)

	commitment := make([]byte, 32)
	for i := range commitment {
		commitment[i] = byte(0xa0 + i)
	}
	author := addressFromHex(t, "2222222222222222222222222222222222222222")
	buyer1 := addressFromHex(t, "3333333333333333333333333333333333333333")
	buyer2 := addressFromHex(t, "4444444444444444444444444444444444444444")
	key1 := NewDeliveryKey(t)
	key2 := NewDeliveryKey(t)
	key1Bytes := DeliveryKeyBytes(key1)
	key2Bytes := DeliveryKeyBytes(key2)

	// 50,000 against three units: the first buyer pays ceil(50,000 / 2).
	env.CreditContractBalance(big.NewInt(50_000))
	env.SetCaller(author)
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", join(commitment, u128Arg(50_000), u64Arg(3), u64Arg(1000))); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool bundle was refused: %s", out)
	}
	view := readPoolView(t, env, api, code, commitment)
	if view.Reserve != "50000" || view.Units != 3 || view.BuyQuote == nil || *view.BuyQuote != "25000" {
		t.Fatalf("the seed reads reserve %s, units %d, buy quote %v", view.Reserve, view.Units, view.BuyQuote)
	}
	assertPoolInvariant(t, env, api, code, commitment)

	// A wrong amount is refused before it can move anything, and the same proof
	// still works for the right amount because the failed call did not consume
	// the buy nonce.
	sig1 := SignPoolBuy(t, contract, commitment, 0, buyer1, key1)
	env.SetCaller(buyer1)
	env.SetPayAmount(big.NewInt(24_999))
	escrowReject(t, env, api, code, "buy", PoolBuyArgs(commitment, key1Bytes, sig1))
	env.SetPayAmount(big.NewInt(25_000))
	env.CreditContractBalance(big.NewInt(25_000))
	if _, out := escrowCallBytes(t, env, api, code, "buy", PoolBuyArgs(commitment, key1Bytes, sig1)); !strings.Contains(out, "true") {
		t.Fatalf("the first pool buy was refused: %s", out)
	}
	view = readPoolView(t, env, api, code, commitment)
	// The payment is escrow, not reserve: the reserve is still what the seller
	// funded, and the next quote follows the unit count that is left.
	if view.Reserve != "50000" || view.Pending != "25000" || view.Units != 2 ||
		view.BuyQuote == nil || *view.BuyQuote != "50000" {
		t.Fatalf("after the first buy: reserve %s, pending %s, units %d, buy quote %v",
			view.Reserve, view.Pending, view.Units, view.BuyQuote)
	}
	assertPoolInvariant(t, env, api, code, commitment)

	// The same proof cannot be replayed once the nonce has moved.
	env.SetPayAmount(big.NewInt(50_000))
	escrowReject(t, env, api, code, "buy", PoolBuyArgs(commitment, key1Bytes, sig1))

	// The second buyer takes the last unit at the new quote.
	sig2 := SignPoolBuy(t, contract, commitment, 1, buyer2, key2)
	env.SetCaller(buyer2)
	env.SetPayAmount(big.NewInt(50_000))
	env.CreditContractBalance(big.NewInt(50_000))
	if _, out := escrowCallBytes(t, env, api, code, "buy", PoolBuyArgs(commitment, key2Bytes, sig2)); !strings.Contains(out, "true") {
		t.Fatalf("the second pool buy was refused: %s", out)
	}
	view = readPoolView(t, env, api, code, commitment)
	if view.Reserve != "50000" || view.Pending != "75000" || view.Units != 1 || view.BuyQuote != nil {
		t.Fatalf("after the second buy: reserve %s, pending %s, units %d, buy quote %v",
			view.Reserve, view.Pending, view.Units, view.BuyQuote)
	}
	assertPoolInvariant(t, env, api, code, commitment)

	// One unit left means no buy quote; the proof is valid for the next nonce,
	// so the refusal is the arithmetic, not the signature.
	sig3 := SignPoolBuy(t, contract, commitment, 2, buyer1, key1)
	env.SetCaller(buyer1)
	env.SetPayAmount(big.NewInt(150_000))
	escrowReject(t, env, api, code, "buy", PoolBuyArgs(commitment, key1Bytes, sig3))

	// Deliver and settle the first order. A second delivery replaces the pending
	// candidate (`REVIEW.md` A06); a second confirmation and a refund after
	// settlement are refused.
	env.SetCaller(author)
	if _, out := escrowCallBytes(t, env, api, code, "deliver", join(u64Arg(1), []byte{0xde, 0xad})); !strings.Contains(out, "true") {
		t.Fatalf("the first delivery was refused: %s", out)
	}
	if _, out := escrowCallBytes(t, env, api, code, "deliver", join(u64Arg(1), []byte{0xbe, 0xef})); !strings.Contains(out, "true") {
		t.Fatalf("a replacement delivery was refused: %s", out)
	}
	env.SetCaller(buyer1)
	// The buyer confirms by bringing the bytes they checked themselves
	// (`AH-10`), whatever the order's pending slot holds.
	if _, out := escrowCallBytes(t, env, api, code, "confirm", join(u64Arg(1), []byte{0xbe, 0xef})); !strings.Contains(out, "true") {
		t.Fatalf("the buyer's confirmation was refused: %s", out)
	}
	escrowReject(t, env, api, code, "confirm", join(u64Arg(1), []byte{0xbe, 0xef}))
	escrowRejectCall(t, env, api, code, "refund", "1")

	// The second order waits out its deadline. The window is funded+1..funded+3:
	// delivery is refused from the next block, and the refund opens in that same
	// block, so the two boundary rules never overlap.
	env.SetBlockNumber(1000 + poolDeadlineBlocks + 1)
	env.SetCaller(author)
	escrowReject(t, env, api, code, "deliver", join(u64Arg(2), []byte{0xbe, 0xef}))
	env.SetCaller(buyer2)
	if _, out := escrowCall(t, env, api, code, "refund", "2"); !strings.Contains(out, "true") {
		t.Fatalf("the mature refund was refused: %s", out)
	}
	view = readPoolView(t, env, api, code, commitment)
	if view.Strikes != 1 || view.BeaconBlock != 0 || view.Units != 1 || view.Reserve != "50000" {
		t.Fatalf("after the first refund: strikes %d, beacon %d, units %d, reserve %s",
			view.Strikes, view.BeaconBlock, view.Units, view.Reserve)
	}
	assertPoolInvariant(t, env, api, code, commitment)
	escrowReject(t, env, api, code, "confirm", join(u64Arg(2), []byte{0x01}))

	// A stale beacon blocks new funding even before the unit count does.
	env.SetCaller(buyer1)
	env.SetPayAmount(big.NewInt(75_000))
	escrowReject(t, env, api, code, "buy", PoolBuyArgs(commitment, key1Bytes, sig3))

	// A gift adds a unit, the desk refreshes the beacon, and the third order
	// takes the next quote. Missing it is the second strike, and the listing is
	// cleared.
	env.SetCaller(author)
	if _, out := escrowCallBytes(t, env, api, code, "donateUnit", commitment); !strings.Contains(out, "true") {
		t.Fatalf("the gift was refused: %s", out)
	}
	if _, out := escrowCallBytes(t, env, api, code, "beacon", commitment); !strings.Contains(out, "true") {
		t.Fatalf("the beacon refresh was refused: %s", out)
	}
	view = readPoolView(t, env, api, code, commitment)
	if view.Units != 2 || view.BuyQuote == nil || *view.BuyQuote != "50000" {
		t.Fatalf("after the gift: units %d, buy quote %v", view.Units, view.BuyQuote)
	}
	env.SetCaller(buyer1)
	env.SetPayAmount(big.NewInt(50_000))
	env.CreditContractBalance(big.NewInt(50_000))
	if _, out := escrowCallBytes(t, env, api, code, "buy", PoolBuyArgs(commitment, key1Bytes, sig3)); !strings.Contains(out, "true") {
		t.Fatalf("the third pool buy was refused: %s", out)
	}
	// The third order was funded at block 1004, so its first refundable block
	// is 1004 + DEADLINE + 1.
	env.SetBlockNumber(1004 + poolDeadlineBlocks + 1)
	env.SetCaller(buyer1)
	if _, out := escrowCall(t, env, api, code, "refund", "3"); !strings.Contains(out, "true") {
		t.Fatalf("the second refund was refused: %s", out)
	}
	view = readPoolView(t, env, api, code, commitment)
	if view.Strikes != 2 || view.Units != 0 {
		t.Fatalf("after the second strike: strikes %d, units %d", view.Strikes, view.Units)
	}
	assertPoolInvariant(t, env, api, code, commitment)
	env.SetCaller(buyer2)
	escrowReject(t, env, api, code, "buy", PoolBuyArgs(commitment, key2Bytes, sig2))
}
