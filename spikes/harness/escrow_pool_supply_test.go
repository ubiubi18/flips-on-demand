package harness

// The pool's supply side: a sealed registration receipt registers a bundle into
// the ownerless pool, a gift adds a unit, and a sale adds one at the pool's own
// quote. The receipt is the real sealed artefact; the test skips when it has not
// been generated, like the offer slice's registration test.

import (
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

func TestPoolSupplyFromASealedReceipt(t *testing.T) {
	receipt := loadRegistrationV2(t)
	code, err := os.ReadFile(escrowWasmPath)
	if err != nil {
		t.Fatalf("read escrow module: %v", err)
	}
	env := NewMinimalEnv()
	contract := addressFromHex(t, hexOf(receipt.journal[4:24]))
	env.SetContractAddress(contract)
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	env.SetBlockNumber(1000)
	receipt.bindSessionAnchors(t, env)

	author := receipt.author()
	manifest := receipt.manifestCommitment()
	commitment := receipt.keyCommitment()
	calldata := receipt.args()

	// Only the author the receipt names may register the bundle, and a refusal
	// is a failed call rather than a JSON reply (`REVIEW.md` A04).
	env.SetCaller(addressFromHex(t, "9999999999999999999999999999999999999999"))
	escrowReject(t, env, api, code, "registerBundle", calldata)
	env.SetCaller(addressFromHex(t, hexOf(author)))
	registerGas, out := escrowCallBytes(t, env, api, code, "registerBundle", calldata)
	if !strings.Contains(out, "true") {
		t.Fatalf("the pool refused the sealed receipt: %s", out)
	}
	t.Logf("pool registerBundle gas=%d", registerGas)

	view := readPoolView(t, env, api, code, commitment)
	if !strings.EqualFold(view.Author, hexOf(author)) || !strings.EqualFold(view.ManifestCommitment, hexOf(manifest)) {
		t.Fatalf("the pool record names author %s manifest %s", view.Author, view.ManifestCommitment)
	}
	if view.Reserve != "0" || view.Units != 0 || view.BeaconBlock != 0 {
		t.Fatalf("the fresh pool record reads reserve %s, %d units, beacon %d", view.Reserve, view.Units, view.BeaconBlock)
	}
	// The count is the sealed receipt's own: the statement bound one flip, so
	// the record carries one, not a number a caller supplied (`SPEC.md` 5.5).
	if view.CoveredFlips != 1 {
		t.Fatalf("the record carries %d covered flips", view.CoveredFlips)
	}
	escrowReject(t, env, api, code, "registerBundle", calldata)

	// A gift adds one unit and pays nothing.
	donateGas, out := escrowCallBytes(t, env, api, code, "donateUnit", commitment)
	if !strings.Contains(out, "true") {
		t.Fatalf("the gift was refused: %s", out)
	}
	t.Logf("pool donateUnit gas=%d", donateGas)
	view = readPoolView(t, env, api, code, commitment)
	if view.Units != 1 || view.Reserve != "0" {
		t.Fatalf("after the gift: reserve %s against %d units", view.Reserve, view.Units)
	}

	// The reserve is what makes the sale quote: 50,000 against one unit pays
	// floor(50,000 / 2) for the next unit.
	env.SetPayAmount(big.NewInt(50_000))
	env.CreditContractBalance(big.NewInt(50_000))
	addGas, out := escrowCallBytes(t, env, api, code, "addReserve", join(commitment, u128Arg(50_000)))
	if !strings.Contains(out, "true") {
		t.Fatalf("adding to the reserve was refused: %s", out)
	}
	t.Logf("pool addReserve gas=%d", addGas)
	view = readPoolView(t, env, api, code, commitment)
	if view.Reserve != "50000" || view.SellQuote == nil || *view.SellQuote != "25000" {
		t.Fatalf("the sale quote is %v against reserve %s", view.SellQuote, view.Reserve)
	}

	// A sealed receipt is public transaction data, not a right to collect:
	// reserve-funded supply stays disabled until a finite issuance rule exists
	// (`REVIEW.md` A01/A05, `SPEC.md` 5.1), so the sale has to fail and the
	// reserve has to hold.
	sellGas := escrowReject(t, env, api, code, "sellUnit", join(commitment, calldata))
	t.Logf("refused pool sellUnit gas=%d", sellGas)
	view = readPoolView(t, env, api, code, commitment)
	if view.Reserve != "50000" || view.Units != 1 {
		t.Fatalf("after the refused sale: reserve %s, %d units", view.Reserve, view.Units)
	}
	if view.SellQuote == nil || *view.SellQuote != "25000" {
		t.Fatalf("the sale quote moved to %v", view.SellQuote)
	}
	_, accountOut := escrowCall(t, env, api, code, "accounting", "0")
	var accounting accountingJSON
	if err := json.Unmarshal([]byte(accountOut), &accounting); err != nil {
		t.Fatalf("parse accounting %q: %v", accountOut, err)
	}
	if accounting.CallerCredit != 0 || accounting.CreditTotal != 0 {
		t.Fatalf("a refused sale credited %d of %d", accounting.CallerCredit, accounting.CreditTotal)
	}
}
