package harness

// D0 containment regressions for the chief developer review (`REVIEW.md`):
//
//	A01  a public registration receipt paid whoever presented it
//	A03  refund money could be spent by supply payouts first
//	A04  a rejected payable call reported success and kept the value
//	A06  the first publisher of a payload owned the only delivery slot
//
// Each test drives the released surface in the real runtime with synthetic
// inputs. The harness runs the test-hooks module, whose seeds stand in for a
// registration proof and a buyer's signature, so those boundaries are reachable
// without a prover or a key ceremony; the release build carries no hook and
// `escrow_no_admin_test.go` pins that. A future re-enable of reserve-funded
// supply (D2) has to keep these tests passing.

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	gethtcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/idena-network/idena-wasm-binding/lib"
)

// d0Env deploys the escrow for one regression and returns the runtime handle.
func d0Env(t *testing.T, code []byte) (*MinimalEnv, *lib.GoAPI) {
	t.Helper()
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, "1111111111111111111111111111111111111111"))
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	return env, api
}

// A04: a buy whose payment is not the quote must abort the call and keep
// nothing, instead of reporting `ok:false` while the amount stays behind.
func TestD0RejectedBuyKeepsNothing(t *testing.T) {
	code := readEscrowModule(t)
	env, api := d0Env(t, code)
	commitment := bytes.Repeat([]byte{0x21}, 32)
	buyer := addressFromHex(t, strings.Repeat("22", 20))
	env.SetBlockNumber(1000)
	env.SetCaller(buyer)
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle",
		join(commitment, u128Arg(50_000), u64Arg(3), u64Arg(1000))); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool bundle: %s", out)
	}

	key, err := gethtcrypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the delivery key: %v", err)
	}
	signature := SignPoolBuy(t, env.ContractAddressValue(), commitment, 0, buyer, key)
	calldata := PoolBuyArgs(commitment, DeliveryKeyBytes(key), signature)

	// The quote is 25,000; the call carries 24,999.
	escrowRejectPayable(t, env, api, code, "buy", calldata, big.NewInt(24_999))

	view := readPoolView(t, env, api, code, commitment)
	if view.Reserve != "50000" || view.Units != 3 {
		t.Fatalf("a rejected buy moved the pool: reserve %s, units %d", view.Reserve, view.Units)
	}
	if balance := env.ContractBalance().String(); balance != "0" {
		t.Fatalf("the rejected buy left %s in the contract", balance)
	}
	// No order was opened either.
	escrowRejectCall(t, env, api, code, "orderPoolView", "1")
}

// A04, against the pinned runtime: a zero-value call cannot be asked whether it
// carried a value - the runtime hands the host a nil `pay_amount` and the
// binding's C shim traps on it before the guest runs (`contracts/escrow/src/lib.rs`,
// `pay_amount_value`) - so a non-payable method accepts the value rather than
// refusing it. What has to hold instead is that the value buys nothing: it
// becomes unattributed surplus, and every tracked total stays what the
// legitimate calls made it.
func TestD0ValueOnANonPayableCallBecomesSurplus(t *testing.T) {
	code := readEscrowModule(t)
	env, api := d0Env(t, code)
	commitment := bytes.Repeat([]byte{0x23}, 32)
	env.SetBlockNumber(1000)
	env.SetCaller(addressFromHex(t, strings.Repeat("24", 20)))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle",
		join(commitment, u128Arg(50_000), u64Arg(3), u64Arg(1000))); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool bundle: %s", out)
	}
	before := readPoolView(t, env, api, code, commitment)

	// The host moves the value into the contract before the call, exactly as the
	// node does for a value-carrying transaction.
	env.SetPayAmount(big.NewInt(1_000))
	env.CreditContractBalance(big.NewInt(1_000))
	defer env.ResetPayAmount()
	if _, out := escrowCallBytes(t, env, api, code, "poolView", commitment); !strings.Contains(out, "true") {
		t.Fatalf("the view call with a value: %s", out)
	}

	if balance := env.ContractBalance().String(); balance != "1000" {
		t.Fatalf("the contract holds %s, not the surplus the call carried", balance)
	}
	after := readPoolView(t, env, api, code, commitment)
	if after.Reserve != before.Reserve || after.Units != before.Units || after.BeaconBlock != before.BeaconBlock {
		t.Fatalf("the surplus moved the pool: %+v then %+v", before, after)
	}
	var accounting accountingJSON
	if _, out := escrowCall(t, env, api, code, "accounting", "0"); json.Unmarshal([]byte(out), &accounting) != nil {
		t.Fatalf("reading accounting: %s", out)
	}
	if accounting.ReserveTotal != 50_000 || accounting.CreditTotal != 0 || accounting.PendingTotal != 0 {
		t.Fatalf("the surplus entered a tracked total: %+v", accounting)
	}
}

// A01/A05: a registration receipt is public transaction data. Presenting it
// must not pay the presenter, and it must not mint reserve-funded copies for the
// author either while the issuance rule is open. The receipt is the real sealed
// artefact the registration program produced.
func TestD0RegistrationReceiptDoesNotPayItsPresenter(t *testing.T) {
	receipt := loadRegistrationV2(t)
	code := readEscrowModule(t)
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, hexOf(receipt.journal[4:24])))
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	env.SetBlockNumber(1000)
	receipt.bindSessionAnchors(t, env)

	author := receipt.author()
	commitment := receipt.keyCommitment()
	calldata := receipt.args()

	env.SetCaller(addressFromHex(t, hexOf(author)))
	if _, out := escrowCallBytes(t, env, api, code, "registerBundle", calldata); !strings.Contains(out, "true") {
		t.Fatalf("the pool refused the sealed receipt: %s", out)
	}
	// Fund the reserve the old code would have paid out of.
	env.SetPayAmount(big.NewInt(50_000))
	env.CreditContractBalance(big.NewInt(50_000))
	if _, out := escrowCallBytes(t, env, api, code, "addReserve", join(commitment, u128Arg(50_000))); !strings.Contains(out, "true") {
		t.Fatalf("adding to the reserve was refused: %s", out)
	}

	stranger := addressFromHex(t, strings.Repeat("77", 20))
	env.SetCaller(stranger)
	escrowReject(t, env, api, code, "sellUnit", join(commitment, calldata))
	env.SetCaller(addressFromHex(t, hexOf(author)))
	escrowReject(t, env, api, code, "sellUnit", join(commitment, calldata))

	view := readPoolView(t, env, api, code, commitment)
	if view.Reserve != "50000" || view.Units != 0 {
		t.Fatalf("a refused sale moved the pool: reserve %s, units %d", view.Reserve, view.Units)
	}
	_, out := escrowCall(t, env, api, code, "accounting", "0")
	var accounting accountingJSON
	if err := json.Unmarshal([]byte(out), &accounting); err != nil {
		t.Fatalf("parse accounting %q: %v", out, err)
	}
	if accounting.CallerCredit != 0 || accounting.CreditTotal != 0 {
		t.Fatalf("a refused sale credited %d of %d", accounting.CallerCredit, accounting.CreditTotal)
	}
}

// A03: the review's 26-sale sequence drained the reserve below the buyer's
// pending refund. Supply payouts are disabled, so every sale must abort, the
// reserve must keep the buyer's money, and the mature refund must be backed.
func TestD0RefundsStayBackedWhileSupplyIsDisabled(t *testing.T) {
	code := readEscrowModule(t)
	env, api := d0Env(t, code)
	commitment := bytes.Repeat([]byte{0x31}, 32)
	author := addressFromHex(t, strings.Repeat("32", 20))
	buyer := addressFromHex(t, strings.Repeat("33", 20))
	env.SetBlockNumber(1000)
	env.SetCaller(author)
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle",
		join(commitment, u128Arg(50_000), u64Arg(6), u64Arg(1000))); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool bundle: %s", out)
	}
	// The seeded reserve is real money as far as the accounting is concerned.
	env.CreditContractBalance(big.NewInt(50_000))

	key, err := gethtcrypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the delivery key: %v", err)
	}
	signature := SignPoolBuy(t, env.ContractAddressValue(), commitment, 0, buyer, key)
	env.SetCaller(buyer)
	env.SetPayAmount(big.NewInt(10_000))
	env.CreditContractBalance(big.NewInt(10_000))
	if _, out := escrowCallBytes(t, env, api, code, "buy",
		PoolBuyArgs(commitment, DeliveryKeyBytes(key), signature)); !strings.Contains(out, "true") {
		t.Fatalf("the 10,000 buy was refused: %s", out)
	}

	// 26 sales would have paid 50,315 out of the reserve and left the buyer's
	// 10,000 refund unpayable (`escrow/src/pool.rs`, the traced counterexample).
	for i := 0; i < 26; i++ {
		escrowReject(t, env, api, code, "sellUnit", join(commitment, make([]byte, registrationV2JournalBytes+registrationV2SealBytes)))
	}
	view := readPoolView(t, env, api, code, commitment)
	// The 10,000 is the buyer's escrow, so the reserve a seller is paid from is
	// untouched and the refund has its own money behind it.
	if view.Reserve != "50000" || view.Pending != "10000" || view.Units != 5 {
		t.Fatalf("after the refused sales: reserve %s, pending %s, units %d", view.Reserve, view.Pending, view.Units)
	}

	env.SetBlockNumber(1000 + poolDeadlineBlocks + 1)
	env.SetCaller(buyer)
	if _, out := escrowCall(t, env, api, code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatalf("the mature refund was refused: %s", out)
	}
	view = readPoolView(t, env, api, code, commitment)
	if view.Reserve != "50000" || view.Pending != "0" {
		t.Fatalf("after the refund the reserve is %s and the pending escrow is %s", view.Reserve, view.Pending)
	}
	_, out := escrowCall(t, env, api, code, "accounting", "0")
	var accounting accountingJSON
	if err := json.Unmarshal([]byte(out), &accounting); err != nil {
		t.Fatalf("parse accounting %q: %v", out, err)
	}
	if accounting.CallerCredit != 0 || accounting.CreditTotal != 0 || accounting.Liquid != 50_000 {
		t.Fatalf("the refund left escrow behind: credit %d of %d against liquid %d",
			accounting.CallerCredit, accounting.CreditTotal, accounting.Liquid)
	}
	if got := env.BalanceOf(buyer); got.Cmp(big.NewInt(10_000)) != 0 {
		t.Fatalf("the refunded buyer holds %s", got)
	}
}

// A06: a payload anyone published first used to be the only one the fast path
// would ever record, so junk could deny delivery. A pending payload is a
// candidate now: the honest one replaces it, the buyer settles on the stored
// bytes, and settlement still closes the order.
//
// A06's second half: confirmation names the digest of the payload the buyer
// checked, so a replacement published between the buyer's check and the
// confirmation cannot settle the order on bytes the buyer never saw.
func TestD0JunkFirstDeliveryDoesNotOwnTheSlot(t *testing.T) {
	code := readEscrowModule(t)
	env, api := d0Env(t, code)
	commitment := bytes.Repeat([]byte{0x41}, 32)
	author := addressFromHex(t, strings.Repeat("42", 20))
	buyer := addressFromHex(t, strings.Repeat("43", 20))
	outsider := addressFromHex(t, strings.Repeat("44", 20))
	env.SetBlockNumber(1000)
	env.SetCaller(author)
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle",
		join(commitment, u128Arg(30_000), u64Arg(2), u64Arg(1000))); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool bundle: %s", out)
	}
	order := join(u64Arg(1), commitment, buyer[:], make([]byte, 65), u128Arg(30_000), u64Arg(1000), u64Arg(0))
	// The seed hook records both the 30,000 reserve and a separate 30,000
	// buyer payment in pending escrow; both liabilities need real backing.
	env.CreditContractBalance(big.NewInt(60_000))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolOrder", order); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool order: %s", out)
	}

	env.SetCaller(outsider)
	if _, out := escrowCallBytes(t, env, api, code, "deliver", join(u64Arg(1), []byte{0xde, 0xad})); !strings.Contains(out, "true") {
		t.Fatalf("the junk delivery was refused: %s", out)
	}
	honest := []byte{0x01, 0x02, 0x03, 0x04}
	if _, out := escrowCallBytes(t, env, api, code, "deliver", join(u64Arg(1), honest)); !strings.Contains(out, "true") {
		t.Fatalf("the honest delivery was refused after junk: %s", out)
	}
	if stored := env.GetStorage(&lib.GasMeter{}, []byte("pdeliv/1")); !bytes.Equal(stored, honest) {
		t.Fatalf("the pending payload is %x, not the honest one", stored)
	}

	env.SetCaller(buyer)
	// The buyer checked `honest`. An attacker replaces the pending payload
	// before the buyer's confirmation lands: the buyer's own copy settles
	// anyway, so the replacement cannot strand the payment (`AH-10`).
	replacement := []byte{0xfa, 0xce}
	if _, out := escrowCallBytes(t, env, api, code, "deliver", join(u64Arg(1), replacement)); !strings.Contains(out, "true") {
		t.Fatalf("the replacement delivery was refused: %s", out)
	}
	if _, out := escrowCallBytes(t, env, api, code, "confirm", join(u64Arg(1), honest)); !strings.Contains(out, "true") {
		t.Fatalf("the buyer's confirmation with the checked bytes was refused: %s", out)
	}
	// The settled payload is the buyer's own copy, not the attacker's.
	if stored := env.GetStorage(&lib.GasMeter{}, []byte("pdeliv/1")); !bytes.Equal(stored, honest) {
		t.Fatalf("the settled payload is %x, not the buyer's checked one", stored)
	}
	escrowReject(t, env, api, code, "deliver", join(u64Arg(1), []byte{0x99}))
}
