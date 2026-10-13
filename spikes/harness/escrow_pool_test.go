package harness

// The pool's call surface, exercised through the test-hooks module: the seed
// hooks stand in for the registration proof and for the buyer's signature, so
// the reserve, the units, the refund, the credit, and the strike can be checked
// end to end before the CLI can prepare those calls.

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

const (
	// The approved hard local-lab deadline: three blocks after inclusion, with
	// repayment available in the first block after the window.
	poolDeadlineBlocks = 3
	poolGraceBlocks    = 0
)

func u128Arg(value uint64) []byte {
	out := make([]byte, 16)
	binary.BigEndian.PutUint64(out[8:], value)
	return out
}

func u64Arg(value uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, value)
	return out
}

func join(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

type poolViewJSON struct {
	Author             string `json:"author"`
	ManifestCommitment string `json:"manifestCommitment"`
	Reserve            string `json:"reserve"`
	// Pending is the buyer escrow the bundle holds for orders that have neither
	// settled nor refunded (`SPEC.md` 5.1).
	Pending      string  `json:"pending"`
	Units        uint64  `json:"units"`
	BeaconBlock  uint64  `json:"beaconBlock"`
	Strikes      uint64  `json:"strikes"`
	CoveredFlips uint32  `json:"coveredFlips"`
	BuyQuote     *string `json:"buyQuote"`
	SellQuote    *string `json:"sellQuote"`
}

func readPoolView(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte, commitment []byte) poolViewJSON {
	t.Helper()
	_, out := escrowCallBytes(t, env, api, code, "poolView", commitment)
	var view poolViewJSON
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("parse the pool view %q: %v", out, err)
	}
	return view
}

type accountingJSON struct {
	CallerCredit uint64 `json:"callerCredit"`
	CreditTotal  uint64 `json:"creditTotal"`
	ReserveTotal uint64 `json:"reserveTotal"`
	PendingTotal uint64 `json:"pendingTotal"`
	UnitsTotal   uint64 `json:"unitsTotal"`
	Liquid       uint64 `json:"liquid"`
}

func TestPoolSupplyRefundAndStrikes(t *testing.T) {
	code, err := os.ReadFile(escrowWasmPath)
	if err != nil {
		t.Fatalf("read escrow module: %v", err)
	}
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, "1111111111111111111111111111111111111111"))
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)

	commitment := make([]byte, 32)
	for i := range commitment {
		commitment[i] = byte(0xa0 + i)
	}
	author := addressFromHex(t, "2222222222222222222222222222222222222222")
	buyer := addressFromHex(t, "3333333333333333333333333333333333333333")
	env.SetCaller(author)
	env.SetBlockNumber(1000)
	// The seeded reserve is real money as far as the accounting is concerned, so
	// the contract has to hold it.
	env.CreditContractBalance(big.NewInt(60_000))

	// Seed: 50,000 against two units, beacon at this block.
	seed := join(commitment, u128Arg(50_000), u64Arg(2), u64Arg(1000))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", seed); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool bundle was refused: %s", out)
	}
	beaconGas, out := escrowCallBytes(t, env, api, code, "beacon", commitment)
	if !strings.Contains(out, "true") {
		t.Fatalf("refreshing the beacon was refused: %s", out)
	}
	t.Logf("pool beacon gas=%d", beaconGas)
	view := readPoolView(t, env, api, code, commitment)
	if view.Reserve != "50000" || view.Units != 2 {
		t.Fatalf("the seed reads back as reserve %s against %d units", view.Reserve, view.Units)
	}
	if view.BuyQuote == nil || *view.BuyQuote != "50000" || view.SellQuote == nil || *view.SellQuote != "16666" {
		t.Fatalf("the seed's quotes are %v and %v", view.BuyQuote, view.SellQuote)
	}

	// A gift adds a unit and lowers both quotes.
	if _, out := escrowCallBytes(t, env, api, code, "donateUnit", commitment); !strings.Contains(out, "true") {
		t.Fatalf("the gift was refused: %s", out)
	}
	view = readPoolView(t, env, api, code, commitment)
	if view.Units != 3 || *view.BuyQuote != "25000" || *view.SellQuote != "12500" {
		t.Fatalf("after the gift: units %d, quotes %v and %v", view.Units, view.BuyQuote, view.SellQuote)
	}

	// Funding the reserve raises the buy quote again.
	env.SetPayAmount(big.NewInt(10_000))
	env.CreditContractBalance(big.NewInt(10_000))
	if _, out := escrowCallBytes(t, env, api, code, "addReserve", join(commitment, u128Arg(10_000))); !strings.Contains(out, "true") {
		t.Fatalf("adding to the reserve was refused: %s", out)
	}
	view = readPoolView(t, env, api, code, commitment)
	if view.Reserve != "60000" || *view.BuyQuote != "30000" {
		t.Fatalf("after funding: reserve %s, buy quote %v", view.Reserve, view.BuyQuote)
	}

	// An order delivered to the buyer and confirmed settles.
	env.SetCaller(author)
	order := join(u64Arg(1), commitment, buyer[:], make([]byte, 65), u128Arg(30_000), u64Arg(1000), u64Arg(0))
	env.CreditContractBalance(big.NewInt(30_000))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolOrder", order); !strings.Contains(out, "true") {
		t.Fatalf("seeding the order was refused: %s", out)
	}
	ciphertext := []byte{0xde, 0xad, 0xbe, 0xef}
	deliverGas, out := escrowCallBytes(t, env, api, code, "deliver", join(u64Arg(1), ciphertext))
	if !strings.Contains(out, "true") {
		t.Fatalf("the delivery was refused: %s", out)
	}
	t.Logf("pool deliver gas=%d", deliverGas)
	// A pending payload is a candidate: publishing again replaces it, so an
	// outsider cannot own the fast path by publishing first (`REVIEW.md` A06).
	if _, out := escrowCallBytes(t, env, api, code, "deliver", join(u64Arg(1), ciphertext)); !strings.Contains(out, "true") {
		t.Fatalf("a repeated delivery was refused: %s", out)
	}
	env.SetCaller(buyer)
	confirmGas, out := escrowCallBytes(t, env, api, code, "confirm", join(u64Arg(1), ciphertext))
	if !strings.Contains(out, "true") {
		t.Fatalf("the buyer's confirmation was refused: %s", out)
	}
	t.Logf("pool confirm gas=%d", confirmGas)

	// An order that is never delivered can only be refunded after the deadline
	// and the grace period, and the refund costs the bundle a strike.
	env.SetCaller(author)
	order = join(u64Arg(2), commitment, buyer[:], make([]byte, 65), u128Arg(30_000), u64Arg(1000), u64Arg(1))
	env.CreditContractBalance(big.NewInt(30_000))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolOrder", order); !strings.Contains(out, "true") {
		t.Fatalf("seeding the second order was refused: %s", out)
	}
	escrowRejectCall(t, env, api, code, "refund", "2")
	env.SetBlockNumber(1000 + poolDeadlineBlocks + poolGraceBlocks + 1)
	refundGas, out := escrowCall(t, env, api, code, "refund", "2")
	if !strings.Contains(out, "true") {
		t.Fatalf("the mature refund was refused: %s", out)
	}
	t.Logf("pool refund gas=%d", refundGas)
	view = readPoolView(t, env, api, code, commitment)
	// The refund repays the buyer straight from escrow: the pool's reserve is
	// untouched (a refund is not reserve spending), and the buyer holds the
	// money instead of a credit that needs a second withdrawal step.
	if view.Reserve != "60000" || view.Strikes != 1 || view.BeaconBlock != 0 {
		t.Fatalf("after the first refund: reserve %s, strikes %d, beacon %d", view.Reserve, view.Strikes, view.BeaconBlock)
	}
	env.SetCaller(buyer)
	_, out = escrowCall(t, env, api, code, "accounting", "0")
	var accounting accountingJSON
	if err := json.Unmarshal([]byte(out), &accounting); err != nil {
		t.Fatalf("parse accounting %q: %v", out, err)
	}
	if accounting.CallerCredit != 0 || accounting.CreditTotal != 0 {
		t.Fatalf("the refund left a credit behind: %d of %d", accounting.CallerCredit, accounting.CreditTotal)
	}
	if accounting.ReserveTotal != 60_000 || accounting.PendingTotal != 0 {
		t.Fatalf("the pool reads reserve %d, pending %d", accounting.ReserveTotal, accounting.PendingTotal)
	}
	if got := env.BalanceOf(buyer); got.Cmp(big.NewInt(30_000)) != 0 {
		t.Fatalf("the refunded buyer holds %s", got)
	}
	// 60,000 of reserve, nothing pending, nothing credited, and the 10,000 the
	// fixture over-credited: the conservation the accounting view shows.
	if accounting.Liquid != 70_000 {
		t.Fatalf("the contract holds %d while the pool reads reserve 60000", accounting.Liquid)
	}

	// A second strike clears the listing.
	env.SetCaller(author)
	order = join(u64Arg(3), commitment, buyer[:], make([]byte, 65), u128Arg(30_000), u64Arg(1841), u64Arg(2))
	env.CreditContractBalance(big.NewInt(30_000))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolOrder", order); !strings.Contains(out, "true") {
		t.Fatalf("seeding the third order was refused: %s", out)
	}
	env.SetBlockNumber(1841 + poolDeadlineBlocks + poolGraceBlocks + 1)
	if _, out := escrowCall(t, env, api, code, "refund", "3"); !strings.Contains(out, "true") {
		t.Fatalf("the second mature refund was refused: %s", out)
	}
	view = readPoolView(t, env, api, code, commitment)
	if view.Strikes != 2 || view.Units != 0 || view.Reserve != "60000" {
		t.Fatalf("after the second refund: reserve %s, units %d, strikes %d", view.Reserve, view.Units, view.Strikes)
	}
}

// The real v2 receipt through the executing runtime: every domain/context
// field is checked, missing/wrong canonical headers refuse, and replay refuses.
func TestPoolFulfillDeliveryV2DomainAndReplay(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/delivery/dist/delivery-v2.json")
	if err != nil {
		t.Fatalf("required delivery-v2 receipt: %v", err)
	}
	var receipt struct {
		FulfillArgument string `json:"fulfill_argument"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	calldata := mustHex(t, receipt.FulfillArgument)
	if len(calldata) < 8+281+256 {
		t.Fatal("short v2 receipt")
	}
	journal := calldata[8 : 8+281]
	id := binary.BigEndian.Uint64(journal[24:32])
	nonce := binary.BigEndian.Uint64(journal[32:40])
	funded := binary.BigEndian.Uint64(journal[241:249])
	seller, buyer, key, commitment := journal[40:60], journal[60:80], journal[80:145], journal[145:177]
	code := readEscrowModule(t)
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, hex.EncodeToString(journal[4:24])))
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	env.SetBlockNumber(funded + 1)
	env.SetCaller(addressFromHex(t, hex.EncodeToString(seller)))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", join(commitment, u128Arg(0), u64Arg(2), u64Arg(funded))); !strings.Contains(out, "true") {
		t.Fatal(out)
	}
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolOrder", join(u64Arg(id), commitment, buyer, key, u128Arg(0), u64Arg(funded), u64Arg(nonce))); !strings.Contains(out, "true") {
		t.Fatal(out)
	}
	// No header, then a different funding fork, both refuse without mutation.
	escrowReject(t, env, api, code, "fulfill", calldata)
	wrongRoot := append([]byte(nil), journal[249:281]...)
	wrongRoot[0] ^= 1
	env.SetBlockHeader(funded, proposedHeaderWire(t, funded, wrongRoot))
	escrowReject(t, env, api, code, "fulfill", calldata)
	env.SetBlockHeader(funded, proposedHeaderWire(t, funded, journal[249:281]))
	for _, offset := range []int{3, 209, 241} {
		attack := append([]byte(nil), calldata...)
		attack[8+offset] ^= 1
		escrowReject(t, env, api, code, "fulfill", attack)
	}
	gas, out := escrowCallBytes(t, env, api, code, "fulfill", calldata)
	if !strings.Contains(out, "true") {
		t.Fatal("valid v2 proof refused:", out)
	}
	t.Logf("real delivery-v2 proof accepted, wasm gas=%d", gas)
	escrowReject(t, env, api, code, "fulfill", calldata)
	escrowRejectCall(t, env, api, code, "refund", strconv.FormatUint(id, 10))
}

type preparedBuyArtifact struct {
	Format    string `json:"format"`
	Operation string `json:"operation"`
	Contract  string `json:"contract"`
	Bundle    string `json:"bundle"`
	Nonce     uint64 `json:"nonce"`
	Buyer     string `json:"buyer"`
	Amount    string `json:"amount"`
	Calldata  string `json:"calldata"`
}

type poolOrderViewJSON struct {
	Bundle   string `json:"bundle"`
	Buyer    string `json:"buyer"`
	Paid     string `json:"paid"`
	FundedAt uint64 `json:"fundedAt"`
	State    uint64 `json:"state"`
}

// The possession proof the CLI prepares, through the runtime: the buyer's call
// opens an order for exactly the pool's quote, and the same proof cannot be
// replayed once the buy nonce has moved.
func TestPoolBuyAcceptsCliPreparedCalldata(t *testing.T) {
	raw, err := os.ReadFile("../../output/cli/prepared-buy.json")
	if err != nil {
		t.Skipf("the prepared pool buy is not generated (%v)", err)
	}
	var prepared preparedBuyArtifact
	if err := json.Unmarshal(raw, &prepared); err != nil {
		t.Fatalf("parse the prepared buy: %v", err)
	}
	if prepared.Format != "fod.prepared-buy" || prepared.Operation != "buy" {
		t.Fatalf("unexpected artefact: %s / %s", prepared.Format, prepared.Operation)
	}
	calldata := mustHex(t, prepared.Calldata)
	// The call ends with the two-byte retrieval-fee count.
	if len(calldata) != 32+65+65+2 {
		t.Fatalf("the buy calldata is %d bytes", len(calldata))
	}
	bundle := calldata[:32]

	code, err := os.ReadFile(escrowWasmPath)
	if err != nil {
		t.Fatalf("read escrow module: %v", err)
	}
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, prepared.Contract))
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	env.SetBlockNumber(900)

	// 50,000 against three units quotes exactly the 25,000 the CLI prepared, and
	// the beacon is fresh at this block.
	env.SetCaller(addressFromHex(t, "4444444444444444444444444444444444444444"))
	seed := join(bundle, u128Arg(50_000), u64Arg(3), u64Arg(900))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", seed); !strings.Contains(out, "true") {
		t.Fatalf("seeding the pool bundle was refused: %s", out)
	}

	amount, ok := new(big.Int).SetString(prepared.Amount, 10)
	if !ok {
		t.Fatalf("amount %q is not an integer", prepared.Amount)
	}
	env.SetCaller(addressFromHex(t, prepared.Buyer))
	env.SetPayAmount(amount)
	env.CreditContractBalance(amount)
	gas, out := escrowCallBytes(t, env, api, code, "buy", calldata)
	if !strings.Contains(out, "true") {
		t.Fatalf("the pool refused the CLI's buy calldata: %s", out)
	}
	t.Logf("pool buy accepted, gas=%d", gas)

	view := readPoolView(t, env, api, code, bundle)
	// The payment is the buyer's escrow until the delivery settles, so the
	// reserve still holds what the seller funded.
	if view.Units != 2 || view.Reserve != "50000" || view.Pending != prepared.Amount {
		t.Fatalf("after the buy: reserve %s, pending %s against %d units", view.Reserve, view.Pending, view.Units)
	}
	_, orderOut := escrowCall(t, env, api, code, "orderPoolView", "1")
	var order poolOrderViewJSON
	if err := json.Unmarshal([]byte(orderOut), &order); err != nil {
		t.Fatalf("parse the order view %q: %v", orderOut, err)
	}
	if !strings.EqualFold(order.Buyer, prepared.Buyer) || order.Paid != prepared.Amount || order.State != 1 {
		t.Fatalf("the order reads back as %+v", order)
	}

	// The proof is bound to the nonce, so the same calldata cannot buy again.
	env.SetPayAmount(amount)
	escrowReject(t, env, api, code, "buy", calldata)
}
