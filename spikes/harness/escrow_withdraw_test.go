package harness

// Exercises the withdrawal payout: the credit is consumed by the call that pays
// it, an unbacked payout changes nothing, and a repeated withdrawal cannot pay
// twice.
//
// The transfer goes through the real import path. The runtime debits the
// contract's balance inside create_transfer_promise and queues the transfer,
// which it runs when the call returns (idena-wasm src/imports.rs and
// src/runner.rs, Action::Transfer). A failure aborts the guest call, and the
// node then discards that call's writes (idena-go vm/wasm/vm.go), which the
// refusal case below models with env.Snapshot's restore function.

import (
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

func deployEscrow(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte) {
	t.Helper()
	if _, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false); err != nil {
		t.Fatalf("deploy rejected: %v", err)
	}
}

// creditCaller builds a caller credit of testPriceValue through the retrieval
// fee: the caller is named as the retrieval host of one settled purchase, so
// the fee becomes its withdrawable credit. The purchase amount goes to the
// bundle's author, a distinct address, which keeps the payout assertions about
// the fee alone.
func creditCaller(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte) {
	t.Helper()
	contract := env.ContractAddressValue()
	commitment := make([]byte, 32)
	for i := range commitment {
		commitment[i] = byte(0x40 + i)
	}
	author := addressFromHex(t, "5757575757575757575757575757575757575757")
	host := CallerAddress()
	buyer := addressFromHex(t, "5858585858585858585858585858585858585858")

	env.SetBlockNumber(1)
	env.SetCaller(author)
	// Two units with a reserve of testPriceValue quote testPriceValue.
	reserve := new(big.Int).Set(testPriceValue)
	seed := join(commitment, u128ArgValue(reserve), u64Arg(2), u64Arg(1))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", seed); !strings.Contains(out, "true") {
		t.Fatalf("seed the pool bundle failed: %s", out)
	}
	env.CreditContractBalance(reserve)
	covered := join(commitment, []byte{0, 0, 0, 1})
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolCovered", covered); !strings.Contains(out, "true") {
		t.Fatalf("seed the covered count failed: %s", out)
	}
	if view := readPoolView(t, env, api, code, commitment); view.CoveredFlips != 1 {
		t.Fatalf("the covered count reads %d", view.CoveredFlips)
	}

	key := NewDeliveryKey(t)
	sig := SignPoolBuy(t, contract, commitment, 0, buyer, key)
	due := new(big.Int).Add(reserve, testFeeValue) // quote + retrieval fee
	env.SetCaller(buyer)
	env.SetPayAmount(due)
	env.CreditContractBalance(due)
	hosts := [][20]byte{host}
	if _, out := escrowCallBytes(t, env, api, code, "buy",
		PoolBuyArgsWithFees(commitment, DeliveryKeyBytes(key), sig, hosts, []*big.Int{testFeeValue})); !strings.Contains(out, "true") {
		t.Fatalf("the fee-bearing buy failed: %s", out)
	}
	ciphertext := []byte{0xca, 0xfe}
	if _, out := escrowCallBytes(t, env, api, code, "deliver", join(u64Arg(1), ciphertext)); !strings.Contains(out, "true") {
		t.Fatalf("the delivery failed: %s", out)
	}
	if _, out := escrowCallBytes(t, env, api, code, "confirm", join(u64Arg(1), ciphertext)); !strings.Contains(out, "true") {
		t.Fatalf("the buyer's confirmation failed: %s", out)
	}
	env.SetCaller(host)
}

// u128ArgValue is u128Arg for an amount that does not fit in a uint64.
func u128ArgValue(value *big.Int) []byte {
	out := make([]byte, 16)
	value.FillBytes(out)
	return out
}

// escrowCallFailing runs a call that must fail in the runtime, then restores the
// modelled state because the node keeps a failed call's writes out of state.
func escrowCallFailing(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte, method, arg string) error {
	t.Helper()
	restore := env.Snapshot()
	_, _, err := lib.Execute(api, code, method, [][]byte{[]byte(arg)}, lib.Address{}, maxCallGasWasm, false)
	if err == nil {
		t.Fatalf("%s(%s) unexpectedly succeeded", method, arg)
	}
	restore()
	return err
}

func TestEscrowWithdrawalPayout(t *testing.T) {
	code, err := os.ReadFile(escrowWasmPath)
	if err != nil {
		t.Fatalf("read escrow module: %v", err)
	}
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	creditCaller(t, env, api, code)

	if _, out := escrowCall(t, env, api, code, "accounting", "1"); !strings.Contains(out, `"callerCredit":`+testFeeValue.String()) {
		t.Fatalf("expected the retrieval-fee credit of %s: %s", testFeeValue, out)
	}

	subEnvsBefore := env.SubEnvCount()
	if _, out := escrowCall(t, env, api, code, "withdraw", "0"); !strings.Contains(out, "true") {
		t.Fatalf("withdraw failed: %s", out)
	}

	_, settled := escrowCall(t, env, api, code, "accounting", "1")
	if !strings.Contains(settled, `"callerCredit":0`) || !strings.Contains(settled, `"creditTotal":0`) {
		t.Fatalf("credit was not consumed: %s", settled)
	}
	if got := env.ContractBalance(); got.Sign() != 0 {
		if got.Cmp(testPriceValue) != 0 {
			t.Fatalf("the contract should hold only the pool's reserve: %s", got)
		}
	}
	if got := env.BalanceOf(CallerAddress()); got.Cmp(testFeeValue) != 0 {
		t.Fatalf("caller was not paid: %s", got)
	}
	t.Logf("after payout: %s contract=%s caller=%s",
		settled, env.ContractBalance(), env.BalanceOf(CallerAddress()))

	// A transfer promise carries no callback in this runtime revision, so the
	// payout must not depend on one being dispatched.
	if got := env.SubEnvCount(); got != subEnvsBefore {
		t.Fatalf("withdraw made %d nested calls", got-subEnvsBefore)
	}

	// The credit is spent: the same call again finds nothing to pay, and the
	// refusal is a failed call (`REVIEW.md` A04).
	escrowRejectCall(t, env, api, code, "withdraw", "0")
}

func TestEscrowWithdrawalRefusedWhenUnbacked(t *testing.T) {
	code, err := os.ReadFile(escrowWasmPath)
	if err != nil {
		t.Fatalf("read escrow module: %v", err)
	}
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	creditCaller(t, env, api, code)

	// Model a contract that no longer holds the credit it owes.
	half := new(big.Int).Rsh(testFeeValue, 1)
	env.SetContractBalance(half)
	callErr := escrowCallFailing(t, env, api, code, "withdraw", "0")
	t.Logf("unbacked withdraw refused: %v", callErr)

	_, after := escrowCall(t, env, api, code, "accounting", "1")
	if !strings.Contains(after, `"callerCredit":`+testFeeValue.String()) || !strings.Contains(after, `"creditTotal":`+testFeeValue.String()) {
		t.Fatalf("refused payout did not leave the credit intact: %s", after)
	}
	if got := env.BalanceOf(CallerAddress()); got.Sign() != 0 {
		t.Fatalf("refused payout paid the caller: %s", got)
	}

	// Once the contract can cover it, the same credit pays out.
	env.CreditContractBalance(half)
	if _, out := escrowCall(t, env, api, code, "withdraw", "0"); !strings.Contains(out, "true") {
		t.Fatalf("withdraw after top-up failed: %s", out)
	}
	if got := env.BalanceOf(CallerAddress()); got.Cmp(testFeeValue) != 0 {
		t.Fatalf("caller was not paid after top-up: %s", got)
	}
	if got := env.ContractBalance(); got.Sign() != 0 {
		if got.Cmp(testPriceValue) != 0 {
			t.Fatalf("the contract should hold only the pool's reserve after the top-up: %s", got)
		}
	}
}
