package harness

// Credits belong to the buyer the order named, are keyed by the whole address,
// and survive a node restart.
//
// The bug this pins, found when the offer slice was current: credits were keyed
// by the first eight bytes of the buyer's address, so two buyers sharing that
// prefix shared one credit record and either could withdraw the sum
// (`docs/PROGRESS.md`, Package 5). The offer slice is gone; this is the same
// property in the pool model, with the restart the earlier test used to model.

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

func TestPoolRefundsSurviveARestartAndStaySeparatePerBuyer(t *testing.T) {
	code := readEscrowModule(t)
	env, api := d0Env(t, code)

	commitment := bytes.Repeat([]byte{0x51}, 32)
	// Two buyers whose addresses share their first eight bytes, and neither is
	// the registry's own address.
	buyerA := addressFromHex(t, "aaaaaaaaaaaaaaaa111111111111111111111111")
	buyerB := addressFromHex(t, "aaaaaaaaaaaaaaaa222222222222222222222222")
	if buyerA == buyerB {
		t.Fatal("the fixture buyers are the same address")
	}

	env.SetBlockNumber(1000)
	env.SetCaller(addressFromHex(t, strings.Repeat("52", 20)))
	reserve := new(big.Int).Lsh(testPriceValue, 1)
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle",
		join(commitment, u128ArgValue(reserve), u64Arg(2), u64Arg(1000))); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool bundle: %s", out)
	}
	env.CreditContractBalance(reserve)

	for index, buyer := range []lib.Address{buyerA, buyerB} {
		order := join(u64Arg(uint64(index+1)), commitment, buyer[:], make([]byte, 65),
			u128ArgValue(testPriceValue), u64Arg(1000), u64Arg(0))
		env.CreditContractBalance(testPriceValue)
		if _, out := escrowCallBytes(t, env, api, code, "seedPoolOrder", order); !strings.Contains(out, "true") {
			t.Fatalf("seeding buyer %d's order: %s", index+1, out)
		}
	}

	// Both orders mature, and anyone may refund: the repayment lands with the
	// buyer the order named, not with whoever sent the call. The first
	// refundable block is funded plus the delivery window plus one.
	env.SetBlockNumber(1000 + poolDeadlineBlocks + 1)
	env.SetCaller(addressFromHex(t, strings.Repeat("53", 20)))
	for index := 0; index < 2; index++ {
		if _, out := escrowCall(t, env, api, code, "refund", string(rune('1'+index))); !strings.Contains(out, "true") {
			t.Fatalf("the refund of order %d: %s", index+1, out)
		}
	}

	for index, buyer := range []lib.Address{buyerA, buyerB} {
		if got := env.BalanceOf(buyer); got.Cmp(testPriceValue) != 0 {
			t.Fatalf("buyer %d holds %s after the refund", index+1, got)
		}
	}
	_, out := escrowCall(t, env, api, code, "accounting", "0")
	if !strings.Contains(out, `"creditTotal":0`) || !strings.Contains(out, `"pendingTotal":0`) {
		t.Fatalf("the pool after both refunds reads %s", out)
	}

	// A restart reloads storage and balances. The closed orders have to survive:
	// a second refund must still be refused, and no escrow may reappear.
	restarted := env.Restart()
	restartedAPI := lib.NewGoAPI(restarted, &lib.GasMeter{})

	for index := 1; index <= 2; index++ {
		restarted.SetCaller(addressFromHex(t, strings.Repeat("53", 20)))
		escrowRejectCall(t, restarted, restartedAPI, code, "refund", string(rune('0'+index)))
	}
	if got := restarted.ContractBalance(); got.Cmp(reserve) != 0 {
		t.Fatalf("the restarted contract holds %s, expected the reserve %s", got, reserve)
	}
	_, settled := escrowCall(t, restarted, restartedAPI, code, "accounting", "0")
	if !strings.Contains(settled, `"pendingTotal":0`) || !strings.Contains(settled, `"creditTotal":0`) {
		t.Fatalf("after the restart the books read %s", settled)
	}
}
