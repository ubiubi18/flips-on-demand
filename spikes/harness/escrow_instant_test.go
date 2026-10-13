package harness

// The approved instant-purchase rules, through the real runtime: a three-block
// delivery window with no grace, a retrieval fee escrowed with the purchase and
// capped by the authenticated covered count, a permissionless refund that pays
// the buyer directly in one transaction, and a reveal settlement that pays the
// author without the buyer's confirmation.

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

type instantFixture struct {
	env        *MinimalEnv
	api        *lib.GoAPI
	code       []byte
	contract   lib.Address
	commitment []byte
	author     lib.Address
	buyer      lib.Address
	host       lib.Address
	key        *ecdsa.PrivateKey
	quote      *big.Int
}

// newInstantFixture seeds one bundle committed to `secrets`: two units at
// `reserve`, so the buy quote is `reserve` itself.
func newInstantFixture(t *testing.T, secrets []byte, reserve *big.Int, covered uint32) *instantFixture {
	t.Helper()
	return newInstantFixtureWithUnits(t, secrets, reserve, covered, 2)
}

// newInstantFixtureWithUnits is the same fixture with a chosen sale-unit count,
// so a test can keep two real buys open on one bundle: the pool only quotes a
// buy while at least two units remain, and the quote is `ceil(r / (q-1))`.
func newInstantFixtureWithUnits(t *testing.T, secrets []byte, reserve *big.Int, covered uint32, units uint64) *instantFixture {
	t.Helper()
	if units < 2 {
		t.Fatalf("a buyable bundle needs at least two units, not %d", units)
	}
	code := readEscrowModule(t)
	env, api := d0Env(t, code)
	digest := sha256.Sum256(secrets)
	commitment := make([]byte, 32)
	copy(commitment, digest[:])
	author := addressFromHex(t, "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1")
	buyer := addressFromHex(t, "b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2")
	host := addressFromHex(t, "c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3")

	env.SetBlockNumber(1000)
	env.SetCaller(author)
	seed := join(commitment, u128ArgValue(reserve), u64Arg(units), u64Arg(1000))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", seed); !strings.Contains(out, "true") {
		t.Fatalf("seed the bundle: %s", out)
	}
	env.CreditContractBalance(reserve)
	if covered > 0 {
		count := []byte{0, 0, byte(covered >> 8), byte(covered)}
		if _, out := escrowCallBytes(t, env, api, code, "seedPoolCovered", join(commitment, count)); !strings.Contains(out, "true") {
			t.Fatalf("seed the covered count: %s", out)
		}
	}
	divisor := new(big.Int).Sub(new(big.Int).SetUint64(units), big.NewInt(1))
	// ceil(r / (q-1)): the extra unit, minus one, is what makes the division
	// round up rather than truncate.
	quote := new(big.Int).Add(reserve, divisor)
	quote.Sub(quote, big.NewInt(1))
	quote.Div(quote, divisor)
	return &instantFixture{
		env:        env,
		api:        api,
		code:       code,
		contract:   env.ContractAddressValue(),
		commitment: commitment,
		author:     author,
		buyer:      buyer,
		host:       host,
		key:        NewDeliveryKey(t),
		quote:      quote,
	}
}

// buy opens order 1 for the fixture's quote plus `fee`, naming the fixture host
// when the fee is positive.
func (f *instantFixture) buy(t *testing.T, fee *big.Int) {
	t.Helper()
	signature := SignPoolBuy(t, f.contract, f.commitment, 0, f.buyer, f.key)
	due := new(big.Int).Add(f.quote, fee)
	f.env.SetCaller(f.buyer)
	f.env.SetPayAmount(due)
	f.env.CreditContractBalance(due)
	arg := PoolBuyArgs(f.commitment, DeliveryKeyBytes(f.key), signature)
	if fee.Sign() > 0 {
		arg = PoolBuyArgsWithFees(f.commitment, DeliveryKeyBytes(f.key), signature,
			[][20]byte{f.host}, []*big.Int{fee})
	}
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "buy", arg); !strings.Contains(out, "true") {
		t.Fatalf("the buy: %s", out)
	}
}

func TestPoolRevealRetainsKeysOnlyWithSuccessfulSettlement(t *testing.T) {
	// Runtime fixture: registration is seeded, not historical provenance.
	secrets := append([]byte("fod:epoch-secrets:v1"), bytes.Repeat([]byte{1}, 86)...)
	fee := big.NewInt(1_000_000_000_000_000_000)
	f := newInstantFixture(t, secrets, big.NewInt(100), 1)
	f.buy(t, fee)
	f.env.SetBlockNumber(1002)
	f.env.SetCaller(f.host) // Permissionless caller receives no author payment.
	gas, out := escrowCallBytes(t, f.env, f.api, f.code, "reveal", join(u64Arg(1), secrets))
	if !strings.Contains(out, "true") {
		t.Fatal("reveal settlement refused")
	}
	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("preveal/1")); !bytes.Equal(got, secrets) {
		t.Fatal("settled keys were not retained in contract state")
	}
	if got := f.env.BalanceOf(f.author); got.Cmp(f.quote) != 0 {
		t.Fatal("author payment did not accompany key retention")
	}
	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("pdeliv/1")); len(got) != 0 {
		t.Fatal("fixture unexpectedly depends on a previous delivery")
	}
	escrowReject(t, f.env, f.api, f.code, "reveal", join(u64Arg(1), secrets))
	f.env.SetBlockNumber(1003)
	escrowRejectCall(t, f.env, f.api, f.code, "refund", "1")
	t.Logf("retained reveal: %d public bytes, %d WASM gas (runtime fixture, not a chain receipt)", len(secrets), gas)

	for _, height := range []uint64{1002, 1003} {
		other := newInstantFixture(t, secrets, big.NewInt(100), 1)
		other.buy(t, fee)
		other.env.SetBlockNumber(height)
		arg := secrets
		if height == 1002 {
			arg = []byte("not the committed keys")
		}
		escrowReject(t, other.env, other.api, other.code, "reveal", join(u64Arg(1), arg))
		if got := other.env.GetStorage(&lib.GasMeter{}, []byte("preveal/1")); len(got) != 0 {
			t.Fatal("refused reveal retained keys")
		}
		other.env.SetBlockNumber(1003)
		other.env.SetCaller(other.host)
		if _, out := escrowCall(t, other.env, other.api, other.code, "refund", "1"); !strings.Contains(out, "true") {
			t.Fatal("refund refused")
		}
		want := new(big.Int).Add(other.quote, fee)
		if got := other.env.BalanceOf(other.buyer); got.Cmp(want) != 0 {
			t.Fatal("refund did not return payment and fee to original buyer")
		}
		escrowReject(t, other.env, other.api, other.code, "reveal", join(u64Arg(1), secrets))
	}
}

func TestPoolRevealSettlesWithoutTheBuyer(t *testing.T) {
	secrets := []byte("the committed epoch secrets this bundle sells")
	fee := big.NewInt(1_000_000_000_000_000_000)
	f := newInstantFixture(t, secrets, big.NewInt(100), 1)
	f.buy(t, fee)

	// The buyer never confirms. Inside the delivery window the author publishes
	// the committed secrets: one hash check settles the order, pays the author
	// and credits the retrieval host, so withholding cannot convert a received
	// delivery into a refund. After the window closes a late reveal is refused
	// and repayment is the only exit (`AH-13`).
	f.env.SetBlockNumber(1002)
	f.env.SetCaller(f.author)
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "reveal", join(u64Arg(1), secrets)); !strings.Contains(out, "true") {
		t.Fatalf("the reveal settlement was refused: %s", out)
	}
	if got := f.env.BalanceOf(f.author); got.Cmp(f.quote) != 0 {
		t.Fatalf("the author received %s, expected the purchase amount %s", got, f.quote)
	}
	f.env.SetCaller(f.host)
	if _, out := escrowCall(t, f.env, f.api, f.code, "accounting", "0"); !strings.Contains(out, `"callerCredit":`+fee.String()) {
		t.Fatalf("the retrieval host was not credited: %s", out)
	}
	f.env.SetCaller(f.buyer)
	escrowRejectCall(t, f.env, f.api, f.code, "refund", "1")

	// A preimage that is not the committed one cannot settle, and the buyer can
	// still take the refund instead.
	other := newInstantFixture(t, []byte("another committed bundle"), big.NewInt(100), 1)
	other.buy(t, big.NewInt(0))
	other.env.SetBlockNumber(1002)
	other.env.SetCaller(other.author)
	escrowReject(t, other.env, other.api, other.code, "reveal", join(u64Arg(1), []byte("not the committed secrets")))
	other.env.SetBlockNumber(1004)
	other.env.SetCaller(other.buyer)
	if _, out := escrowCall(t, other.env, other.api, other.code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatalf("the refund after a failed reveal: %s", out)
	}
	// The refund is final in the other direction too: once it is paid, even the
	// committed preimage cannot settle the order afterwards.
	other.env.SetCaller(other.author)
	escrowReject(t, other.env, other.api, other.code, "reveal", join(u64Arg(1), []byte("another committed bundle")))
}

func TestPoolFeeCapAndRefundReturnTheFee(t *testing.T) {
	f := newInstantFixture(t, []byte("fee cap bundle"), big.NewInt(100), 1)
	quote := f.quote

	// One covered item caps the retrieval fee at one iDNA.
	over := big.NewInt(2_000_000_000_000_000_000)
	signature := SignPoolBuy(t, f.contract, f.commitment, 0, f.buyer, f.key)
	due := new(big.Int).Add(quote, over)
	f.env.SetCaller(f.buyer)
	f.env.SetPayAmount(due)
	f.env.CreditContractBalance(due)
	escrowReject(t, f.env, f.api, f.code, "buy",
		PoolBuyArgsWithFees(f.commitment, DeliveryKeyBytes(f.key), signature, [][20]byte{f.host}, []*big.Int{over}))

	// The capped fee is escrow, not a charge: the timeout returns the purchase
	// amount and the fee together, and the host receives nothing.
	fee := big.NewInt(1_000_000_000_000_000_000)
	f.buy(t, fee)
	f.env.SetBlockNumber(1004)
	f.env.SetCaller(f.buyer)
	if _, out := escrowCall(t, f.env, f.api, f.code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatalf("the timeout refund: %s", out)
	}
	if got := f.env.BalanceOf(f.buyer); got.Cmp(new(big.Int).Add(quote, fee)) != 0 {
		t.Fatalf("the buyer received %s, expected the payment plus the fee", got)
	}
	if _, out := escrowCall(t, f.env, f.api, f.code, "accounting", "0"); !strings.Contains(out, `"creditTotal":0`) || !strings.Contains(out, `"pendingTotal":0`) {
		t.Fatalf("the refund left escrow behind: %s", out)
	}
}

// A buyer whose client fetched two objects from one peer may name that host
// twice: the entries add up under the same covered-count cap, the host's final
// credit is their sum, and the reserve still pays nothing (`SPEC.md` 7.3, 11.1).
func TestPoolFeeListRepeatsAHostUnderTheSameCap(t *testing.T) {
	secrets := []byte("repeated host fee list")
	f := newInstantFixture(t, secrets, big.NewInt(100), 1)
	half := big.NewInt(500_000_000_000_000_000)
	capped := new(big.Int).Mul(half, big.NewInt(2))

	// One base unit over the cap is refused, so the cap counts the whole list.
	signature := SignPoolBuy(t, f.contract, f.commitment, 0, f.buyer, f.key)
	over := new(big.Int).Add(capped, big.NewInt(1))
	f.env.SetCaller(f.buyer)
	f.env.SetPayAmount(new(big.Int).Add(f.quote, over))
	f.env.CreditContractBalance(new(big.Int).Add(f.quote, over))
	escrowReject(t, f.env, f.api, f.code, "buy", PoolBuyArgsWithFees(f.commitment, DeliveryKeyBytes(f.key), signature,
		[][20]byte{f.host, f.host}, []*big.Int{half, half, big.NewInt(1)}))

	// The capped list itself is accepted, and settlement adds both entries to
	// that one host's credit.
	due := new(big.Int).Add(f.quote, capped)
	f.env.SetPayAmount(due)
	f.env.CreditContractBalance(due)
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "buy", PoolBuyArgsWithFees(f.commitment, DeliveryKeyBytes(f.key), signature,
		[][20]byte{f.host, f.host}, []*big.Int{half, half})); !strings.Contains(out, "true") {
		t.Fatalf("the repeated-host buy: %s", out)
	}
	f.env.SetCaller(f.author)
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "reveal", join(u64Arg(1), secrets)); !strings.Contains(out, "true") {
		t.Fatalf("the reveal settlement: %s", out)
	}
	f.env.SetCaller(f.host)
	_, out := escrowCall(t, f.env, f.api, f.code, "accounting", "0")
	if !strings.Contains(out, `"callerCredit":`+capped.String()) {
		t.Fatalf("the repeated host was not credited the list's sum: %s", out)
	}
	if !strings.Contains(out, `"reserveTotal":100`) {
		t.Fatalf("the retrieval fee touched the reserve: %s", out)
	}
}

func TestPoolDeliveryDeadlineBoundaries(t *testing.T) {
	// funded+2 is the last delivery block, and the refund is not available yet.
	last := newInstantFixture(t, []byte("boundary last block"), big.NewInt(100), 0)
	last.buy(t, big.NewInt(0))
	last.env.SetBlockNumber(1002)
	last.env.SetCaller(last.author)
	if _, out := escrowCallBytes(t, last.env, last.api, last.code, "deliver", join(u64Arg(1), []byte{0xaa})); !strings.Contains(out, "true") {
		t.Fatalf("delivery at funded+2 was refused: %s", out)
	}
	last.env.SetCaller(last.buyer)
	escrowRejectCall(t, last.env, last.api, last.code, "refund", "1")
	last.env.SetBlockNumber(1003)
	if _, out := escrowCall(t, last.env, last.api, last.code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatalf("the refund at funded+3: %s", out)
	}

	// funded+3 refuses delivery and opens the refund in the same block.
	late := newInstantFixture(t, []byte("boundary late block"), big.NewInt(100), 0)
	late.buy(t, big.NewInt(0))
	late.env.SetBlockNumber(1003)
	late.env.SetCaller(late.author)
	escrowReject(t, late.env, late.api, late.code, "deliver", join(u64Arg(1), []byte{0xbb}))
	late.env.SetCaller(late.buyer)
	if _, out := escrowCall(t, late.env, late.api, late.code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatalf("the refund at funded+3 was refused: %s", out)
	}
}

// Every unilateral settlement path obeys the same hard boundary: inside the
// window a seller may settle without the buyer; once the refund is available,
// only the buyer's own confirm or refund can move the order (`AH-13`).
func TestPoolUnilateralSettlementsCloseAtTheDeadline(t *testing.T) {
	last := newInstantFixture(t, []byte("last reveal block"), big.NewInt(100), 1)
	last.buy(t, big.NewInt(0))
	last.env.SetBlockNumber(1002)
	last.env.SetCaller(last.author)
	if _, out := escrowCallBytes(t, last.env, last.api, last.code, "reveal",
		join(u64Arg(1), []byte("last reveal block"))); !strings.Contains(out, "true") {
		t.Fatalf("the reveal at funded+2 was refused: %s", out)
	}
	if got := last.env.BalanceOf(last.author); got.Cmp(last.quote) != 0 {
		t.Fatalf("the author received %s at funded+2, expected %s", got, last.quote)
	}

	late := newInstantFixture(t, []byte("late reveal block"), big.NewInt(100), 0)
	late.buy(t, big.NewInt(0))
	late.env.SetBlockNumber(1003)
	late.env.SetCaller(late.author)
	escrowReject(t, late.env, late.api, late.code, "reveal", join(u64Arg(1), []byte("late reveal block")))
	// The deadline is checked before the proof is parsed, so a late prover
	// cannot even reach verification.
	escrowReject(t, late.env, late.api, late.code, "fulfill", make([]byte, 1_000))
	late.env.SetCaller(late.buyer)
	if _, out := escrowCall(t, late.env, late.api, late.code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatalf("the refund at funded+3 was refused: %s", out)
	}
}

// A listing cleared by two strikes is unbuyable, so its author can no longer
// add units: new inventory there would dilute the one global quote without
// ever being purchasable (`AH-11`).
func TestPoolClearedListingRefusesDonations(t *testing.T) {
	f := newInstantFixtureWithUnits(t, []byte("cleared listing bundle"), big.NewInt(100), 1, 4)
	seed := func(id uint64) {
		order := join(u64Arg(id), f.commitment, f.buyer[:], make([]byte, 65), u128Arg(10), u64Arg(1000), u64Arg(id-1))
		f.env.CreditContractBalance(big.NewInt(10))
		if _, out := escrowCallBytes(t, f.env, f.api, f.code, "seedPoolOrder", order); !strings.Contains(out, "true") {
			t.Fatalf("seed order %d: %s", id, out)
		}
	}
	seed(1)
	f.env.SetBlockNumber(1004)
	f.env.SetCaller(f.buyer)
	if _, out := escrowCall(t, f.env, f.api, f.code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatalf("the first refund: %s", out)
	}
	seed(2)
	f.env.SetBlockNumber(1008)
	f.env.SetCaller(f.buyer)
	if _, out := escrowCall(t, f.env, f.api, f.code, "refund", "2"); !strings.Contains(out, "true") {
		t.Fatalf("the second refund: %s", out)
	}
	before := readPoolView(t, f.env, f.api, f.code, f.commitment)
	if before.Strikes != 2 {
		t.Fatalf("the two refunds left %d strikes", before.Strikes)
	}

	// The listing is cleared: the author's donation is refused and neither the
	// bundle's units nor the shared count moves.
	f.env.SetCaller(f.author)
	escrowReject(t, f.env, f.api, f.code, "donateUnit", f.commitment)
	after := readPoolView(t, f.env, f.api, f.code, f.commitment)
	if after.Units != before.Units || after.Strikes != before.Strikes {
		t.Fatalf("the refused donation moved the bundle: units %d -> %d, strikes %d -> %d",
			before.Units, after.Units, before.Strikes, after.Strikes)
	}
	if _, out := escrowCall(t, f.env, f.api, f.code, "accounting", "0"); !strings.Contains(out, `"unitsTotal":0`) {
		t.Fatalf("the refused donation moved the pool's units: %s", out)
	}
}

// Publishing a bundle's secrets stops the listing and atomically closes every
// already funded order whose buyer can use those same secrets.
func TestPoolRevealStopsTheListing(t *testing.T) {
	secrets := []byte("revealed bundle stops selling")
	f := newInstantFixtureWithUnits(t, secrets, big.NewInt(100), 1, 3)
	f.buy(t, big.NewInt(0))
	// A second order is funded before the reveal and must settle with the first.
	order := join(u64Arg(2), f.commitment, f.buyer[:], make([]byte, 65), u128Arg(10), u64Arg(1000), u64Arg(1))
	f.env.CreditContractBalance(big.NewInt(10))
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "seedPoolOrder", order); !strings.Contains(out, "true") {
		t.Fatalf("seed the second order: %s", out)
	}
	if before := readPoolView(t, f.env, f.api, f.code, f.commitment); before.Units != 2 {
		t.Fatalf("after the buy the bundle reads %d units, expected 2", before.Units)
	}

	f.env.SetBlockNumber(1002)
	f.env.SetCaller(f.author)
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "reveal", join(u64Arg(1), secrets)); !strings.Contains(out, "true") {
		t.Fatalf("the reveal: %s", out)
	}
	if view := readPoolView(t, f.env, f.api, f.code, f.commitment); view.Units != 0 {
		t.Fatalf("the revealed bundle still sells %d units", view.Units)
	}
	if _, out := escrowCall(t, f.env, f.api, f.code, "accounting", "0"); !strings.Contains(out, `"unitsTotal":0`) {
		t.Fatalf("the revealed bundle's units still price the pool: %s", out)
	}

	// No new buyer and no donation may pay for a public secret.
	signature := SignPoolBuy(t, f.contract, f.commitment, 1, f.buyer, f.key)
	escrowReject(t, f.env, f.api, f.code, "buy", PoolBuyArgs(f.commitment, DeliveryKeyBytes(f.key), signature))
	f.env.SetCaller(f.author)
	escrowReject(t, f.env, f.api, f.code, "donateUnit", f.commitment)

	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("preveal/2")); !bytes.Equal(got, secrets) {
		t.Fatal("the second buyer has no retained keys")
	}
	f.env.SetBlockNumber(1003)
	escrowRejectCall(t, f.env, f.api, f.code, "refund", "2")
	escrowReject(t, f.env, f.api, f.code, "reveal", join(u64Arg(2), secrets))
}

func TestPoolDonationStaysWithTheBundleAuthor(t *testing.T) {
	f := newInstantFixture(t, []byte("author-only donation bundle"), big.NewInt(100), 1)

	// A stranger's donation would add a claim on the shared quote without
	// paying for it, so it is refused and neither count moves.
	f.env.SetCaller(f.buyer)
	escrowReject(t, f.env, f.api, f.code, "donateUnit", f.commitment)
	if view := readPoolView(t, f.env, f.api, f.code, f.commitment); view.Units != 2 {
		t.Fatalf("the refused donation left the bundle with %d units", view.Units)
	}
	if _, out := escrowCall(t, f.env, f.api, f.code, "accounting", "0"); !strings.Contains(out, `"unitsTotal":2`) {
		t.Fatalf("the refused donation moved the pool's units: %s", out)
	}

	// The registered author's own donation adds exactly one unit to the bundle
	// and to the one global pool.
	f.env.SetCaller(f.author)
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "donateUnit", f.commitment); !strings.Contains(out, "true") {
		t.Fatalf("the author's donation: %s", out)
	}
	if view := readPoolView(t, f.env, f.api, f.code, f.commitment); view.Units != 3 {
		t.Fatalf("the author's donation left the bundle with %d units", view.Units)
	}
	if _, out := escrowCall(t, f.env, f.api, f.code, "accounting", "0"); !strings.Contains(out, `"unitsTotal":3`) {
		t.Fatalf("the author's donation left the pool's units unchanged: %s", out)
	}
}

// A public reveal unlocks one bundle for every buyer, so it must settle all
// outstanding orders on that bundle together. Otherwise a second buyer could
// keep the revealed keys and later refund their still-open order.
func TestPoolRevealSettlesEveryOpenOrderOnBundle(t *testing.T) {
	secrets := []byte("concurrent order bundle")
	f := newInstantFixtureWithUnits(t, secrets, big.NewInt(100), 1, 3)
	fee := big.NewInt(1_000_000_000_000_000_000)

	quoteOf := func() *big.Int {
		view := readPoolView(t, f.env, f.api, f.code, f.commitment)
		if view.BuyQuote == nil {
			t.Fatalf("the pool offers no buy: pending %s", view.Pending)
		}
		quote, ok := new(big.Int).SetString(*view.BuyQuote, 10)
		if !ok {
			t.Fatalf("the pool quote %q is not an integer", *view.BuyQuote)
		}
		return quote
	}
	buy := func(buyer lib.Address, key *ecdsa.PrivateKey, nonce uint64, quote *big.Int) {
		signature := SignPoolBuy(t, f.contract, f.commitment, nonce, buyer, key)
		due := new(big.Int).Add(quote, fee)
		f.env.SetCaller(buyer)
		f.env.SetPayAmount(due)
		f.env.CreditContractBalance(due)
		arg := PoolBuyArgsWithFees(f.commitment, DeliveryKeyBytes(key), signature,
			[][20]byte{f.host}, []*big.Int{fee})
		if _, out := escrowCallBytes(t, f.env, f.api, f.code, "buy", arg); !strings.Contains(out, "true") {
			t.Fatalf("the buy by %x: %s", buyer, out)
		}
	}

	first := quoteOf()
	buy(f.buyer, f.key, 0, first)
	secondBuyer := addressFromHex(t, "d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4")
	secondKey := NewDeliveryKey(t)
	second := quoteOf()
	buy(secondBuyer, secondKey, 1, second)

	firstLiability := new(big.Int).Add(first, fee)
	secondLiability := new(big.Int).Add(second, fee)
	outstanding := new(big.Int).Add(firstLiability, secondLiability)
	if view := readPoolView(t, f.env, f.api, f.code, f.commitment); view.Pending != outstanding.String() {
		t.Fatalf("two open orders hold %s in escrow, expected %s", view.Pending, outstanding)
	}
	if got := f.env.BalanceOf(f.buyer); got.Sign() != 0 {
		t.Fatalf("the first buyer was paid before settlement: %s", got)
	}

	// One reveal makes the same keys usable by both buyers. Both payments must
	// leave escrow in this transaction, and both recovery records must remain.
	f.env.SetCaller(f.author)
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "reveal", join(u64Arg(1), secrets)); !strings.Contains(out, "true") {
		t.Fatalf("the reveal settlement: %s", out)
	}
	wantAuthor := new(big.Int).Add(first, second)
	if got := f.env.BalanceOf(f.author); got.Cmp(wantAuthor) != 0 {
		t.Fatalf("the author received %s, expected both purchase amounts %s", got, wantAuthor)
	}
	for _, id := range []string{"1", "2"} {
		if got := f.env.GetStorage(&lib.GasMeter{}, []byte("preveal/"+id)); !bytes.Equal(got, secrets) {
			t.Fatalf("order %s has no retained keys", id)
		}
	}
	if view := readPoolView(t, f.env, f.api, f.code, f.commitment); view.Pending != "0" {
		t.Fatalf("the reveal left %s refundable escrow behind", view.Pending)
	}
	// Neither buyer can keep those usable keys and take a later refund.
	f.env.SetBlockNumber(1000 + poolDeadlineBlocks + 1)
	f.env.SetCaller(addressFromHex(t, "e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5"))
	escrowRejectCall(t, f.env, f.api, f.code, "refund", "1")
	escrowRejectCall(t, f.env, f.api, f.code, "refund", "2")
	escrowRejectCall(t, f.env, f.api, f.code, "reveal", "1")
	if got := f.env.BalanceOf(secondBuyer); got.Sign() != 0 {
		t.Fatalf("the settled second buyer received a refund: %s", got)
	}
	view := readPoolView(t, f.env, f.api, f.code, f.commitment)
	if view.Pending != "0" || view.Strikes != 0 {
		t.Fatalf("after the reveal the pool reads pending %s strikes %d", view.Pending, view.Strikes)
	}
	_, out := escrowCall(t, f.env, f.api, f.code, "accounting", "0")
	var accounting accountingJSON
	if err := json.Unmarshal([]byte(out), &accounting); err != nil {
		t.Fatalf("parse accounting %q: %v", out, err)
	}
	// The reveal also removes its one remaining unsold unit from the global
	// count, while both buyer-funded host fees become credits.
	if accounting.ReserveTotal != 100 || accounting.PendingTotal != 0 ||
		accounting.UnitsTotal != 0 || accounting.CreditTotal != 2*fee.Uint64() {
		t.Fatalf("the books read reserve %d pending %d units %d credits %d",
			accounting.ReserveTotal, accounting.PendingTotal, accounting.UnitsTotal, accounting.CreditTotal)
	}
	if accounting.Liquid != 100+2*fee.Uint64() {
		t.Fatalf("the contract holds %d, expected the reserve plus both host credits", accounting.Liquid)
	}
}

func TestPoolRevealRefusesAnExpiredSiblingWithoutExposingKeys(t *testing.T) {
	secrets := []byte("one bundle shared by staggered buyers")
	f := newInstantFixtureWithUnits(t, secrets, big.NewInt(100), 1, 3)
	f.buy(t, big.NewInt(0)) // order 1 funded at N
	f.env.SetBlockNumber(1001)
	secondBuyer := addressFromHex(t, "d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4")
	secondKey := NewDeliveryKey(t)
	view := readPoolView(t, f.env, f.api, f.code, f.commitment)
	quote, ok := new(big.Int).SetString(*view.BuyQuote, 10)
	if !ok {
		t.Fatal("the second quote is not an integer")
	}
	f.env.SetCaller(secondBuyer)
	f.env.SetPayAmount(quote)
	f.env.CreditContractBalance(quote)
	signature := SignPoolBuy(t, f.contract, f.commitment, 1, secondBuyer, secondKey)
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "buy",
		PoolBuyArgs(f.commitment, DeliveryKeyBytes(secondKey), signature)); !strings.Contains(out, "true") {
		t.Fatalf("the second buy: %s", out)
	}

	// At N+3 the first buyer has a refund right, even though the second
	// buyer's window remains open. Revealing keys for either would expose the
	// first buyer's content before that refund, so the whole call must roll back.
	f.env.SetBlockNumber(1003)
	f.env.SetCaller(f.author)
	escrowReject(t, f.env, f.api, f.code, "reveal", join(u64Arg(2), secrets))
	for _, id := range []string{"1", "2"} {
		if got := f.env.GetStorage(&lib.GasMeter{}, []byte("preveal/"+id)); len(got) != 0 {
			t.Fatalf("refused reveal exposed keys for order %s", id)
		}
	}
	if got := f.env.BalanceOf(f.author); got.Sign() != 0 {
		t.Fatalf("refused reveal paid the author %s", got)
	}
	if got := readPoolView(t, f.env, f.api, f.code, f.commitment).Pending; got != new(big.Int).Add(f.quote, quote).String() {
		t.Fatalf("refused reveal changed pending escrow to %s", got)
	}
	f.env.SetCaller(f.host)
	if _, out := escrowCall(t, f.env, f.api, f.code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatalf("first buyer's deadline refund: %s", out)
	}
	// The first buyer kept the verified encrypted bytes. Removing their order
	// from the open set must not make a later public reveal safe.
	f.env.SetCaller(f.author)
	escrowReject(t, f.env, f.api, f.code, "reveal", join(u64Arg(2), secrets))
	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("preveal/2")); len(got) != 0 {
		t.Fatal("a later reveal exposed keys after the first buyer's refund")
	}
	f.env.SetBlockNumber(1004)
	f.env.SetCaller(f.host)
	if _, out := escrowCall(t, f.env, f.api, f.code, "refund", "2"); !strings.Contains(out, "true") {
		t.Fatalf("second buyer's deadline refund: %s", out)
	}
	if got := f.env.BalanceOf(secondBuyer); got.Cmp(quote) != 0 {
		t.Fatalf("second buyer was repaid %s, expected %s", got, quote)
	}
}

func TestPoolMultiOrderRevealRollsBackWhenALaterPayoutFails(t *testing.T) {
	secrets := []byte("atomic reveal payout fixture")
	f := newInstantFixtureWithUnits(t, secrets, big.NewInt(100), 1, 3)
	f.buy(t, big.NewInt(0))
	second := join(u64Arg(2), f.commitment, f.buyer[:], make([]byte, 65), u128Arg(10), u64Arg(1000), u64Arg(1))
	f.env.CreditContractBalance(big.NewInt(10))
	if _, out := escrowCallBytes(t, f.env, f.api, f.code, "seedPoolOrder", second); !strings.Contains(out, "true") {
		t.Fatalf("seed second order: %s", out)
	}
	// Artificial insolvency: the first transfer can be promised, but the
	// second cannot. A failed call must roll back the first payout and every
	// order/key write, not leave a partially settled public reveal.
	f.env.SetContractBalance(new(big.Int).Add(f.quote, big.NewInt(5)))
	f.env.SetCaller(f.author)
	escrowReject(t, f.env, f.api, f.code, "reveal", join(u64Arg(1), secrets))
	if got := f.env.BalanceOf(f.author); got.Sign() != 0 {
		t.Fatalf("failed atomic reveal paid the author %s", got)
	}
	for _, id := range []string{"1", "2"} {
		if got := f.env.GetStorage(&lib.GasMeter{}, []byte("preveal/"+id)); len(got) != 0 {
			t.Fatalf("failed atomic reveal published keys for order %s", id)
		}
	}
}

func TestPoolRevealBoundedWorstCaseHostCredits(t *testing.T) {
	secrets := []byte("bounded eight-order reveal")
	f := newInstantFixtureWithUnits(t, secrets, big.NewInt(100), 16, 10)
	fee := new(big.Int).Mul(big.NewInt(16), big.NewInt(1_000_000_000_000_000_000))
	hosts := make([][20]byte, 16)
	amounts := make([]*big.Int, 16)
	for i := range hosts {
		hosts[i][19] = byte(i + 1)
		amounts[i] = big.NewInt(1_000_000_000_000_000_000)
	}
	buy := func(nonce uint64, accepted bool) {
		key := NewDeliveryKey(t)
		view := readPoolView(t, f.env, f.api, f.code, f.commitment)
		quote, ok := new(big.Int).SetString(*view.BuyQuote, 10)
		if !ok {
			t.Fatal("the bundle lost its quote before its ninth buy")
		}
		f.env.SetCaller(f.buyer)
		due := new(big.Int).Add(quote, fee)
		f.env.SetPayAmount(due)
		signature := SignPoolBuy(t, f.contract, f.commitment, nonce, f.buyer, key)
		arg := PoolBuyArgsWithFees(f.commitment, DeliveryKeyBytes(key), signature, hosts, amounts)
		if !accepted {
			escrowReject(t, f.env, f.api, f.code, "buy", arg)
			return
		}
		f.env.CreditContractBalance(due)
		if _, out := escrowCallBytes(t, f.env, f.api, f.code, "buy", arg); !strings.Contains(out, "true") {
			t.Fatalf("valid open order %d was refused: %s", nonce+1, out)
		}
	}
	for nonce := uint64(0); nonce < 8; nonce++ {
		buy(nonce, true)
	}
	before := readPoolView(t, f.env, f.api, f.code, f.commitment)
	buy(8, false)
	if after := readPoolView(t, f.env, f.api, f.code, f.commitment); after.Pending != before.Pending || after.Units != before.Units {
		t.Fatal("refused ninth buy changed inventory or pending liabilities")
	}
	f.env.SetCaller(f.author)
	gas, out := escrowCallBytes(t, f.env, f.api, f.code, "reveal", join(u64Arg(1), secrets))
	if !strings.Contains(out, "true") {
		t.Fatalf("bounded reveal could not settle eight buyers and 128 host credits: %s", out)
	}
	if got := readPoolView(t, f.env, f.api, f.code, f.commitment).Pending; got != "0" {
		t.Fatalf("bounded reveal left %s in refundable escrow", got)
	}
	for id := uint64(1); id <= 8; id++ {
		if got := f.env.GetStorage(&lib.GasMeter{}, []byte("preveal/"+strconv.FormatUint(id, 10))); !bytes.Equal(got, secrets) {
			t.Fatalf("order %d cannot recover settled keys", id)
		}
	}
	t.Logf("eight-order public reveal with 128 fee entries: %d WASM gas (runtime fixture)", gas)
}

// At funded+3 repayment is already available. Both transaction orderings must
// protect the buyer: no unilateral call can race a refund at the boundary.
func TestPoolRefundOnlyBoundaryBothOrderings(t *testing.T) {
	for _, refundFirst := range []bool{false, true} {
		f := newInstantFixture(t, []byte("exclusive deadline bundle"), big.NewInt(100), 1)
		fee := big.NewInt(1_000_000_000_000_000_000)
		f.buy(t, fee)
		f.env.SetBlockNumber(1003)
		repay := func() {
			f.env.SetCaller(f.host) // permissionless caller, not the buyer
			if _, out := escrowCall(t, f.env, f.api, f.code, "refund", "1"); !strings.Contains(out, "true") {
				t.Fatalf("boundary refund: %s", out)
			}
		}
		refuse := func() {
			f.env.SetCaller(f.author)
			escrowReject(t, f.env, f.api, f.code, "deliver", join(u64Arg(1), []byte("candidate")))
			escrowReject(t, f.env, f.api, f.code, "reveal", join(u64Arg(1), []byte("exclusive deadline bundle")))
			escrowReject(t, f.env, f.api, f.code, "fulfill", join(u64Arg(1), make([]byte, 992)))
		}
		if refundFirst {
			repay()
			refuse()
		} else {
			refuse()
			repay()
		}
		due := new(big.Int).Add(f.quote, fee)
		if f.env.BalanceOf(f.buyer).Cmp(due) != 0 || f.env.BalanceOf(f.author).Sign() != 0 || f.env.BalanceOf(f.host).Sign() != 0 {
			t.Fatal("refund did not exclusively repay the original buyer")
		}
		escrowRejectCall(t, f.env, f.api, f.code, "refund", "1")
	}
}

// Negative feasibility evidence: possession learned from an unincluded
// envelope cannot be revoked by a later timeout. This is not a closure test.
func TestUnincludedDeliveryExposureCanPrecedeRefund(t *testing.T) {
	secrets := []byte("synthetic committed epoch secrets")
	f := newInstantFixture(t, secrets, big.NewInt(100), 1)
	f.buy(t, big.NewInt(0))
	candidate, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(&f.key.PublicKey), secrets, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	learned, err := ecies.ImportECDSA(f.key).Decrypt(candidate, nil, nil)
	if err != nil || string(learned) != string(secrets) {
		t.Fatal("buyer did not open the unincluded candidate")
	}
	// No delivery/settlement transaction was included. A dropped/censored
	// transaction leaves the order open even though the buyer knows the secrets.
	f.env.SetBlockNumber(1003)
	f.env.SetCaller(f.host)
	if _, out := escrowCall(t, f.env, f.api, f.code, "refund", "1"); !strings.Contains(out, "true") {
		t.Fatal(out)
	}
	if f.env.BalanceOf(f.buyer).Cmp(f.quote) != 0 || string(learned) != string(secrets) {
		t.Fatal("negative fair-exchange control changed")
	}
}
