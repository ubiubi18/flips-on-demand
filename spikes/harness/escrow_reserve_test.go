package harness

// The reserve's donor-owned exit (`AH-15`): a contribution is recorded against
// the address that paid it and can be withdrawn while it is still in the
// reserve, without reaching another party's escrow or credits.

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

func TestPoolReserveDonationIsWithdrawable(t *testing.T) {
	code := readEscrowModule(t)
	env, api := d0Env(t, code)
	commitment := make([]byte, 32)
	for index := range commitment {
		commitment[index] = 0x77
	}
	author := addressFromHex(t, "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1")
	donor := addressFromHex(t, "f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0")
	buyer := addressFromHex(t, "b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2")

	env.SetBlockNumber(1000)
	env.SetCaller(author)
	seed := join(commitment, u128Arg(0), u64Arg(2), u64Arg(1000))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", seed); !strings.Contains(out, "true") {
		t.Fatalf("seed the bundle: %s", out)
	}

	// The donation is recorded against the address that paid it.
	env.SetCaller(donor)
	env.SetPayAmount(big.NewInt(50_000))
	env.CreditContractBalance(big.NewInt(50_000))
	if _, out := escrowCallBytes(t, env, api, code, "addReserve", join(commitment, u128Arg(50_000))); !strings.Contains(out, "true") {
		t.Fatalf("the reserve donation was refused: %s", out)
	}
	if _, out := escrowCall(t, env, api, code, "accounting", "0"); !strings.Contains(out, `"callerReserve":50000`) ||
		!strings.Contains(out, `"reserveTotal":50000`) {
		t.Fatalf("after the donation the books read %s", out)
	}

	// A funded order puts buyer escrow in the contract: the withdrawal below
	// must leave it exactly where it was.
	order := join(u64Arg(1), commitment, buyer[:], make([]byte, 65), u128Arg(30_000), u64Arg(1000), u64Arg(0))
	env.SetCaller(author)
	env.CreditContractBalance(big.NewInt(30_000))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolOrder", order); !strings.Contains(out, "true") {
		t.Fatalf("seed the order: %s", out)
	}

	// The donor takes their contribution back, whole and once.
	env.SetCaller(donor)
	if _, out := escrowCall(t, env, api, code, "withdrawReserve", "0"); !strings.Contains(out, "true") {
		t.Fatalf("the reserve withdrawal was refused: %s", out)
	}
	if got := env.BalanceOf(donor); got.Cmp(big.NewInt(50_000)) != 0 {
		t.Fatalf("the donor received %s, expected the whole 50000", got)
	}
	_, out := escrowCall(t, env, api, code, "accounting", "0")
	var accounting accountingJSON
	if err := json.Unmarshal([]byte(out), &accounting); err != nil {
		t.Fatalf("parse accounting %q: %v", out, err)
	}
	if accounting.ReserveTotal != 0 || accounting.PendingTotal != 30_000 {
		t.Fatalf("after the withdrawal the books read reserve %d pending %d", accounting.ReserveTotal, accounting.PendingTotal)
	}
	// The buyer's escrow is untouched: the order can still settle or refund.
	escrowRejectCall(t, env, api, code, "withdrawReserve", "0")

	// An address that never donated has nothing to withdraw.
	env.SetCaller(author)
	escrowRejectCall(t, env, api, code, "withdrawReserve", "0")
}

// A deposit the runtime reports but the contract's balance does not carry must
// be refused. Otherwise the reserve becomes a claim the balance cannot cover,
// and `settle_order`'s solvency guard then refuses every later settlement -
// the wedge the lab hit on 2026-10-02 (`docs/PROGRESS.md`, audit log): a
// successful `addReserve` left `reserve0 = 1e18` against a contract balance
// that never held it, and the buyer's `confirm` was refused with
// `not_enough_balance` until the order was refunded.
func TestPoolReserveDepositMustBeBacked(t *testing.T) {
	code := readEscrowModule(t)
	env, api := d0Env(t, code)
	commitment := make([]byte, 32)
	for index := range commitment {
		commitment[index] = 0x88
	}
	author := addressFromHex(t, "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1")

	env.SetBlockNumber(1000)
	env.SetCaller(author)
	seed := join(commitment, u128Arg(0), u64Arg(2), u64Arg(1000))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", seed); !strings.Contains(out, "true") {
		t.Fatalf("seed the bundle: %s", out)
	}

	// The runtime reports the amount, but the value never lands in the
	// contract, so the reserve it would write is a claim on money that is not
	// there.
	env.SetPayAmount(big.NewInt(50_000))
	escrowReject(t, env, api, code, "addReserve", join(commitment, u128Arg(50_000)))

	env.SetCaller(author)
	if _, out := escrowCall(t, env, api, code, "accounting", "0"); !strings.Contains(out, `"reserveTotal":0`) ||
		!strings.Contains(out, `"callerReserve":0`) {
		t.Fatalf("an unbacked deposit left a reserve behind: %s", out)
	}

	// The same call with the value actually credited is accepted, so the guard
	// refuses the unbacked case rather than payable deposits in general.
	env.SetPayAmount(big.NewInt(50_000))
	env.CreditContractBalance(big.NewInt(50_000))
	if _, out := escrowCallBytes(t, env, api, code, "addReserve", join(commitment, u128Arg(50_000))); !strings.Contains(out, "true") {
		t.Fatalf("a backed deposit was refused: %s", out)
	}
	if _, out := escrowCall(t, env, api, code, "accounting", "0"); !strings.Contains(out, `"reserveTotal":50000`) ||
		!strings.Contains(out, `"callerReserve":50000`) {
		t.Fatalf("a backed deposit did not reach the books: %s", out)
	}
}
