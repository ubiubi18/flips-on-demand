package harness

// A beacon is the author's hint, not a capability.
//
// A fresh beacon is what makes a bundle buyable, so a stranger's beacon could
// lure a purchase into a refund strike against the author. Only the address the
// registration proof named may set it, and even for the author it grants
// nothing: no unit, no credit, no claim on an order, and no ability to settle
// one. Delivery itself stays open to any desk - a pending delivery is
// replaceable, which is the fix `REVIEW.md` A06 asked for and
// `escrow_d0_regressions_test.go` exercises.

import (
	"bytes"
	"math/big"
	"strings"
	"testing"
)

func TestOnlyTheAuthorMayBeaconAndItGrantsNothing(t *testing.T) {
	code := readEscrowModule(t)
	env, api := d0Env(t, code)

	commitment := bytes.Repeat([]byte{0x71}, 32)
	author := addressFromHex(t, strings.Repeat("72", 20))
	stranger := addressFromHex(t, strings.Repeat("73", 20))
	env.SetBlockNumber(1000)
	env.SetCaller(author)
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle",
		join(commitment, u128Arg(30_000), u64Arg(2), u64Arg(1000))); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool bundle: %s", out)
	}
	reserve := new(big.Int).SetUint64(30_000)
	env.CreditContractBalance(reserve)

	env.SetBlockNumber(1200)
	// A stranger's beacon is refused, and it changes nothing on the record: the
	// bundle is not made buyable by someone who cannot deliver for it.
	env.SetCaller(stranger)
	escrowReject(t, env, api, code, "beacon", commitment)
	if view := readPoolView(t, env, api, code, commitment); view.BeaconBlock != 1000 {
		t.Fatalf("the stranger's refused beacon moved the record to block %d, past the seed's 1000", view.BeaconBlock)
	}

	env.SetCaller(author)
	if _, out := escrowCallBytes(t, env, api, code, "beacon", commitment); !strings.Contains(out, "true") {
		t.Fatalf("the author's beacon was refused: %s", out)
	}

	// Freshness moved; nothing else did.
	view := readPoolView(t, env, api, code, commitment)
	if view.BeaconBlock != 1200 {
		t.Fatalf("the beacon block reads %d", view.BeaconBlock)
	}
	if view.Reserve != "30000" || view.Units != 2 || view.Strikes != 0 {
		t.Fatalf("the beacon moved the pool: reserve %s, %d units, %d strikes",
			view.Reserve, view.Units, view.Strikes)
	}
	if got := env.BalanceOf(author); got.Sign() != 0 {
		t.Fatalf("the author holds %s after beaconing", got)
	}
	if got := env.ContractBalance(); got.Cmp(reserve) != 0 {
		t.Fatalf("the contract holds %s after the beacon", got)
	}
	_, accounting := escrowCall(t, env, api, code, "accounting", "0")
	if !strings.Contains(accounting, `"callerCredit":0`) || !strings.Contains(accounting, `"creditTotal":0`) {
		t.Fatalf("a beacon created a liability: %s", accounting)
	}
	// And it cannot settle anything: there is no order to confirm.
	escrowReject(t, env, api, code, "confirm", join(u64Arg(1), []byte{0x01}))
}
