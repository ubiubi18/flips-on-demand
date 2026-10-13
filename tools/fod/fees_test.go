package main

// The estimator's arithmetic and its table. The gas numbers themselves are
// measured elsewhere (`SOURCES.md` F-76); these checks keep the conversion, the
// rounding, and the coverage honest.

import (
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The recorded answer of the public indexer's last block at height 11,355,800
// (epoch 228), reduced to the fields the estimator reads.
const indexerLastBlock = `{"result":{"epoch":228,"height":11355800,"feeRate":"0.0000961538461538"}}`

func TestBaseUnitsPerGasFromTheIndexerDecimal(t *testing.T) {
	got, err := baseUnitsPerGasFromRate("0.0000961538461538")
	if err != nil {
		t.Fatalf("convert the rate: %v", err)
	}
	if want := "96153846153800"; got.String() != want {
		t.Fatalf("the rate is %s base units per gas, expected %s", got, want)
	}
	for _, rate := range []string{"", "0", "-1", "abc", "null"} {
		if _, err := baseUnitsPerGasFromRate(rate); err == nil {
			t.Errorf("the rate %q was accepted", rate)
		}
	}
}

func TestReadIndexerFeePerGas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/Block/Last" {
			t.Errorf("the estimator asked for %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, indexerLastBlock)
	}))
	defer server.Close()

	rate, height, err := readIndexerFeePerGas(server.URL)
	if err != nil {
		t.Fatalf("read the indexer: %v", err)
	}
	if height != 11_355_800 {
		t.Errorf("the block height is %d", height)
	}
	if want := "96153846153800"; rate.String() != want {
		t.Errorf("the rate is %s, expected %s", rate, want)
	}
}

func TestUserGasRoundsUp(t *testing.T) {
	cases := []struct {
		wasmGas uint64
		want    uint64
	}{
		{100, 1},
		{101, 2},
		{0, 0},
		{195_478_684, 1_954_787},
		{195_632_417, 1_956_325},
	}
	for _, c := range cases {
		if got := userGasForWasmGas(c.wasmGas); got != c.want {
			t.Errorf("%d wasm gas is %d user gas, expected %d", c.wasmGas, got, c.want)
		}
	}
}

func TestRequiredMaxFeeMultipliesExactly(t *testing.T) {
	rate := big.NewInt(1000)
	// 1,954,787 user gas plus 5,590 gas for a 559-byte transaction.
	got := requiredMaxFeeFor(195_478_684, 559, rate)
	if want := new(big.Int).SetUint64((1_954_787 + 5_590) * 1000); got.Cmp(want) != 0 {
		t.Fatalf("the required max fee is %s, expected %s", got, want)
	}
}

func TestSizeFeeChargesTenGasPerByte(t *testing.T) {
	got := sizeFeeFor(559, big.NewInt(10))
	if want := new(big.Int).SetUint64(55_900); got.Cmp(want) != 0 {
		t.Fatalf("the size fee is %s, expected %s", got, want)
	}
}

func TestMaxMaxFeeIsTheBlockGasCapTimesTheRate(t *testing.T) {
	if got, want := maxMaxFeeFor(big.NewInt(1_000)), new(big.Int).SetUint64(5_120_000_000); got.Cmp(want) != 0 {
		t.Fatalf("the cap is %s, expected %s", got, want)
	}
	// The release module's deploy transaction is 357,860 bytes, so its size fee
	// alone is 3.57860e6 gas; on a chain whose rate is 1e16 that size fee has to
	// fit under the same cap the whole max fee is measured against.
	rate := big.NewInt(10_000_000_000_000_000)
	if sizeFee := sizeFeeFor(357_860, rate); sizeFee.Cmp(maxMaxFeeFor(rate)) >= 0 {
		t.Fatal("the release deploy's size fee does not fit under the cap")
	}
}

func TestIDNAText(t *testing.T) {
	base, _ := new(big.Int).SetString("188497788461447982600", 10)
	if got := iDNAText(base); got != "188.50" {
		t.Fatalf("the amount renders as %s iDNA, expected 188.50", got)
	}
}

// Every prepared operation has to have a measured number, or `submit` cannot
// tell the user what the call costs. A new operation without a measurement
// fails here instead of printing a made-up fee.
func TestRevealBudgetCoversRetainedStateAndRefusesTheOldBudget(t *testing.T) {
	gas, ok := measuredGasFor(operationReveal)
	if !ok {
		t.Fatal("no reveal signing budget")
	}
	// Pinned node host cost: 20 user gas per key/value byte, multiplied by
	// 100 for WASM gas. Include the contract's maximum 512-byte preimage and
	// the longest decimal uint64 order suffix, independently of the table.
	const historicalWasm = 1_268_100
	const extraStorageWasm = uint64((512 + len("preveal/") + 20) * 20 * 100)
	if gas.WasmGas < historicalWasm+extraStorageWasm {
		t.Fatal("reveal budget omits canonical key storage")
	}
	rate := big.NewInt(1000)
	oldBudget := requiredMaxFeeFor(historicalWasm, 235, rate)
	if maxFeeShortfall(operationReveal, 235, rate, oldBudget) == nil {
		t.Fatal("the old underfunded reveal budget was accepted")
	}
	newBudget := requiredMaxFeeFor(gas.WasmGas, 235, rate)
	if maxFeeShortfall(operationReveal, 235, rate, newBudget) != nil {
		t.Fatal("the updated reveal budget was refused")
	}
}

func TestEveryOperationHasAMeasuredNumber(t *testing.T) {
	operations := []string{
		operationDeployModule,
		operationFulfill,
		operationRefund,
		operationWithdrawal,
		operationReserveWithdrawal,
		operationBuy,
		operationDeliver,
		operationBeacon,
		operationRegisterBundle,
		operationSellUnit,
		operationDonateUnit,
		operationAddReserve,
		operationConfirm,
	}
	for _, operation := range operations {
		gas, ok := measuredGasFor(operation)
		if !ok {
			t.Errorf("%s has no measured gas", operation)
			continue
		}
		if gas.Runs < 1 || gas.Note == "" {
			t.Errorf("%s has no provenance", operation)
		}
		if gas.TxBytes < 100 {
			t.Errorf("%s has no measured transaction size", operation)
		}
		if userGas := userGasForWasmGas(gas.WasmGas); userGas > maxUserGasUpgrade11 {
			t.Errorf("%s needs %d user gas, above the %d cap", operation, userGas, maxUserGasUpgrade11)
		}
	}
}

// TestTheConfirmRowCarriesTheAH10Envelope pins the live measurement the AH-18
// investigation produced. The confirmation that carries the buyer's 219-byte
// ciphertext needs 20,357 user gas, not the 14,747 of the pre-AH-10 digest
// form; the shortfall is what ran the two v4 confirms out of gas and got their
// hashes blacklisted, and the corrected row is what makes the submit guard
// refuse that shortfall before broadcasting.
func TestTheConfirmRowCarriesTheAH10Envelope(t *testing.T) {
	gas, ok := measuredGasFor(operationConfirm)
	if !ok {
		t.Fatal("confirm has no measured gas")
	}
	if gas.WasmGas != 2_035_700 || gas.TxBytes != 350 {
		t.Fatalf("the confirm row measures %d wasm gas over %d bytes, expected the live 2,035,700 over the 350-byte reference envelope",
			gas.WasmGas, gas.TxBytes)
	}
	// 0.01 iDNA per gas: the lab chain's rate at network size one.
	rate := big.NewInt(10_000_000_000_000_000)
	required := requiredMaxFeeFor(gas.WasmGas, gas.TxBytes, rate)
	if want := "238570000000000000000"; required.String() != want {
		t.Fatalf("the confirm requirement is %s base units, expected %s for the reference envelope", required, want)
	}
	// The v4 run signed 351 bytes, and the guard prices the size it signed.
	liveRequired := requiredMaxFeeFor(gas.WasmGas, 351, rate)
	if want := "238670000000000000000"; liveRequired.String() != want {
		t.Fatalf("the signed confirm requirement is %s base units, expected %s", liveRequired, want)
	}
	// The 200 iDNA budget the v4 confirms signed is below this requirement...
	budget, ok := new(big.Int).SetString("200000000000000000000", 10)
	if !ok {
		t.Fatal("the pinned budget does not parse")
	}
	if budget.Cmp(liveRequired) >= 0 {
		t.Fatal("the 200 iDNA budget is not below the corrected requirement")
	}
	// ...while the superseded row would have estimated only 182.57 iDNA for the
	// same signed envelope, which is why the guard let the call through.
	old := requiredMaxFeeFor(1_474_700, 351, rate)
	if want := "182570000000000000000"; old.String() != want {
		t.Fatalf("the superseded row estimates %s base units, expected the 182.57 the v4 run printed", old)
	}
}

func TestParseFeePerGasAcceptsTheNodeForms(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{`1234`, "1234"},
		{`"1234"`, "1234"},
		{`"0x4d2"`, "1234"},
	}
	for _, c := range cases {
		got, err := parseFeePerGas(json.RawMessage(c.raw))
		if err != nil {
			t.Fatalf("%s: %v", c.raw, err)
		}
		if got.String() != c.want {
			t.Fatalf("%s parsed as %s, expected %s", c.raw, got, c.want)
		}
	}
	for _, raw := range []string{``, `null`, `"0"`, `"-5"`, `"abc"`} {
		if _, err := parseFeePerGas(json.RawMessage(raw)); err == nil {
			t.Errorf("%q was accepted as a fee per gas", raw)
		}
	}
}

func TestEstimateRefusesUnknownOperations(t *testing.T) {
	if _, _, ok := estimateForOperation("listBundle", big.NewInt(1000)); ok {
		t.Fatal("an unmeasured operation produced an estimate")
	}
	if _, _, ok := estimateForOperation(operationFulfill, big.NewInt(0)); ok {
		t.Fatal("a zero rate produced an estimate")
	}
}
