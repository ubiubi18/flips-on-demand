package harness

// One pool, one escrow: a refund consumes only its own order's payment out of
// the global pending escrow. The global reserve is untouched, the other
// bundle's inventory, beacon and strikes are untouched, and the books balance
// across both (`REVIEW.md` A03's exit list, under the owner's "one global
// pool" answer of 2026-09-29).

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

// quoteOf renders a view's quote for a failure message, or "none".
func quoteOf(quote *string) string {
	if quote == nil {
		return "none"
	}
	return *quote
}

func TestRefundingOneBundleLeavesAnotherUntouched(t *testing.T) {
	code := readEscrowModule(t)
	env, api := d0Env(t, code)

	refunded := bytes.Repeat([]byte{0x61}, 32)
	untouched := bytes.Repeat([]byte{0x62}, 32)
	buyer := addressFromHex(t, "bbbbbbbbbbbbbbbb111111111111111111111111")
	// Base units, as the other accounting tests use them: two units at 100 each.
	unitPrice := big.NewInt(100)
	reserve := big.NewInt(200)

	env.SetBlockNumber(1000)
	env.SetCaller(buyer)
	for index, commitment := range [][]byte{refunded, untouched} {
		if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle",
			join(commitment, u128ArgValue(reserve), u64Arg(2), u64Arg(1000))); !strings.Contains(out, "true") {
			t.Fatalf("seeding bundle %d: %s", index+1, out)
		}
	}
	env.CreditContractBalance(new(big.Int).Lsh(reserve, 1))

	// One order on the first bundle, funded and then refunded on its own.
	order := join(u64Arg(1), refunded, buyer[:], make([]byte, 65),
		u128ArgValue(unitPrice), u64Arg(1000), u64Arg(0))
	env.CreditContractBalance(unitPrice)
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolOrder", order); !strings.Contains(out, "true") {
		t.Fatalf("seeding the order: %s", out)
	}
	env.SetBlockNumber(1000 + poolDeadlineBlocks + 1)
	if _, out := escrowCall(t, env, api, code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatalf("the refund: %s", out)
	}

	// The refund took the payment out of pending escrow, not out of the
	// reserve: the one pool still holds the two seeded reserves (400), with
	// nothing pending, and the buyer holds the money directly.
	first := readPoolView(t, env, api, code, refunded)
	if first.Reserve != "400" || first.Pending != "0" || first.Units != 2 {
		t.Fatalf("the pool reads reserve %s, pending %s against %d units",
			first.Reserve, first.Pending, first.Units)
	}
	if first.Strikes != 1 {
		t.Fatalf("the refunded bundle counts %d strikes", first.Strikes)
	}
	// The one quote comes from the whole pool: `ceil(400 / (4 - 1))`.
	if first.BuyQuote == nil || *first.BuyQuote != "134" {
		t.Fatalf("the pool quotes %v", quoteOf(first.BuyQuote))
	}

	// The other bundle's own record is untouched: same inventory, no strikes,
	// its beacon still fresh, and it reads the same one pool.
	second := readPoolView(t, env, api, code, untouched)
	if second.Reserve != "400" || second.Pending != "0" || second.Units != 2 || second.Strikes != 0 {
		t.Fatalf("the untouched bundle reads reserve %s, pending %s, %d units, %d strikes",
			second.Reserve, second.Pending, second.Units, second.Strikes)
	}
	if second.BuyQuote == nil || *second.BuyQuote != "134" {
		t.Fatalf("the untouched bundle quotes %v", quoteOf(second.BuyQuote))
	}
	if second.BeaconBlock != 1000 {
		t.Fatalf("the untouched bundle's beacon reads %d", second.BeaconBlock)
	}

	// The books balance: the two reserves plus the refund credit are exactly the
	// liquid balance, so no bundle's money pays another's liability.
	_, out := escrowCall(t, env, api, code, "accounting", "0")
	var accounting accountingJSON
	if err := json.Unmarshal([]byte(out), &accounting); err != nil {
		t.Fatalf("parse accounting %q: %v", out, err)
	}
	if accounting.CallerCredit != 0 || accounting.CreditTotal != 0 {
		t.Fatalf("the refund left a credit behind: %d of %d", accounting.CallerCredit, accounting.CreditTotal)
	}
	if accounting.ReserveTotal != 400 || accounting.PendingTotal != 0 || accounting.UnitsTotal != 4 {
		t.Fatalf("the pool reads reserve %d, pending %d, units %d",
			accounting.ReserveTotal, accounting.PendingTotal, accounting.UnitsTotal)
	}
	expected := new(big.Int).Add(reserve, reserve)
	if accounting.Liquid != expected.Uint64() {
		t.Fatalf("the liquid balance reads %d, expected %s", accounting.Liquid, expected)
	}

	// The refund paid the buyer straight away, and the order is closed: no
	// second withdrawal step and no second refund.
	if got := env.BalanceOf(buyer); got.Cmp(unitPrice) != 0 {
		t.Fatalf("the refunded buyer holds %s", got)
	}
	env.SetCaller(buyer)
	escrowRejectCall(t, env, api, code, "refund", "1")
	second = readPoolView(t, env, api, code, untouched)
	if second.Reserve != "400" {
		t.Fatalf("after the withdrawal the pool reads reserve %s", second.Reserve)
	}
	if got := env.ContractBalance(); got.Cmp(new(big.Int).Add(reserve, reserve)) != 0 {
		t.Fatalf("the contract holds %s after the refund", got)
	}

}
