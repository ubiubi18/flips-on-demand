package main

// What a marketplace call costs, before anything is signed (`SPEC.md` 4.3 step
// 5 and the definition of done in 11.2).
//
// What the pinned node charges for a contract call is
// `feePerGas * size(tx) * 10 + feePerGas * gasUsed` (`blockchain/fee/fee_calc.go`
// `CalculateFee`, `CalculateGas`, plus the receipt's `GasCost` added in
// `blockchain/blockchain.go`; the size carries 67 bytes when the transaction is
// unsigned). The second term dominates for a proof-carrying call.
//
// The `MaxFee` a caller signs is a budget, not the payment: the node derives the
// gas allowance as `(MaxFee - sizeFee) / feePerGas` (`blockchain.go` `getGasLimit`),
// validation refuses a max fee below the charged fee or above the block's gas
// cap (`blockchain/validation`, `SOURCES.md` F-15), and the runtime charges 100
// wasm gas per user gas (`GasToWasmGas`, `SOURCES.md` F-8). So a call that
// consumes W wasm gas needs a max fee of at least
// `sizeFee + ceil(W / 100) * feePerGas`, which is also what it can cost.
//
// The gas numbers and transaction sizes below are measured, not modelled: each
// is the highest value a harness, a CLI run, or a live chain receipt recorded
// against the escrow, and `SOURCES.md` F-76 names the runs. The registration
// figure was re-measured when the O-12 change replaced the guest program id and
// the sealed receipt, because the call that carries a proof is metered partly
// by the proof it carries; the other operations were unchanged by that. The
// same figures hold under the runtime mainnet executes (`SOURCES.md` F-90), so
// they inform the estimate rather than merely describing the local pin.
//
// The harness under-measures the short pool calls. On the isolated play-money
// chain the same calls cost 5.6x to 16.3x the harness figure - buy 1,313,000
// wasm against 163,757, deliver 1,083,800 against 110,861, confirm 2,035,700
// against 124,748, beacon 630,600 against 111,751, donateUnit 630,100 against
// 111,746, addReserve 672,600 against 105,827 - so the entries the lifecycle
// exercises carry the live receipts' numbers. (The buy and deliver figures
// were re-measured on the module that carries the retrieval-fee list and
// confirm on the envelope that carries the buyer's ciphertext; the earlier
// module recorded 1,101,300 / 1,035,100 and the digest-form confirm
// 1,474,700.) The refund, reveal, withdrawal and fulfilment operations carry
// live receipts too: 1,357,900, 1,268,100, 371,966 and 197,276,900 wasm.
// `sellUnit` stays the
// harness figure plus the largest observed gap (1,400,000 wasm) because the
// payout path is disabled. An underpriced max fee is not a
// harmless warning: the proposal runs out of gas, and the pinned node adds the
// transaction to its persistent blacklist, after which no proposal will ever
// include that hash again (`docs/PROGRESS.md`, 2026-09-29).

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// maxUserGasUpgrade11 is the largest user-gas budget one transaction may
	// ask for once upgrade 11 is active (`SOURCES.md` F-15: MaxBlockGasUpgrade11
	// = 5000 KiB, and validation rejects a derived gas above it).
	maxUserGasUpgrade11 = 5_120_000
	// wasmGasPerUserGas is the runtime's scaling factor (`SOURCES.md` F-8).
	wasmGasPerUserGas = 100
	// operationDeployModule is the one-off deploy of the escrow module itself.
	// It is not a contract method, so it is named here rather than beside a
	// prepared action request.
	operationDeployModule = "deployModule"
)

// operationGas is one escrow operation's measured wasm-gas requirement.
type operationGas struct {
	Operation string
	WasmGas   uint64
	TxBytes   uint64
	Runs      int
	Note      string
}

// measuredOperationGas lists operations in the order the workflow meets them.
// The transaction sizes are the signed transactions the CLI produced for each
// prepared action, which is the size the node charges for.
//
// The deploy is in the table because it is the first thing the workflow spends
// money on and its size fee dominates the cost: 365,269 bytes is 3,652,690 gas
// before the deploy executes anything.
var measuredOperationGas = []operationGas{
	{operationDeployModule, 3_000_051, 365_269, 1, "deploy the release module; the CLI signed this for the release artefact on 2026-09-26 after the offer-slice removal, and `spikes/harness` checks its gas against the deploy headroom"},
	{operationWithdrawal, 371_966, 117, 1, "withdraw a credit; live receipt 0x4fa5df79… at height 1684 on 2026-09-29 (gasUsed 3,719 user gas / 371,966 wasm, gasCost 37.19), paying the caller's whole credit; the live signed envelope was 122 bytes at the 200 iDNA budget, and the submit guard prices the size it actually signed"},
	{operationReserveWithdrawal, 422_600, 129, 1, "withdraw a donor's whole reserve credit, leaving buyer escrow and host credits untouched; measured live on 2026-09-30: tx 0x7b91a991… in block 0x0462cc56… at epoch 28 (4,226 user gas / 422,600 wasm), which returned the v4 lab's 200 iDNA donation and left the contract balance at 0 with every escrow entry and credit already settled. The live signed envelope was 129 bytes at the 200 iDNA budget; the previous row carried the withdraw call's 371,966 wasm and 117 bytes as a placeholder"},
	{operationBuy, 1_313_000, 292, 1, "buy one pool unit at the quote, with the delivery-key possession proof; live receipt 0xa0dbe60b… on 2026-09-29 (`gasUsed` 13,130 user gas / 1,313,000 wasm, gasCost 131.30, txFee 33.1) on the current module, which carried the retrieval-fee list and signed 331 bytes - the reference is the canonical 292-byte envelope (`TestServeCanonicalSignedSizes`) and the submit guard prices the size it actually signed"},
	{operationDeliver, 1_083_800, 350, 1, "publish the buyer-bound ciphertext for a funded pool order; live receipts 0xf5e36a06… and 0x1ac30518… on 2026-09-29 (`gasUsed` 10,838 user gas / 1,083,800 wasm, gasCost 108.38) on the current module, the size carries a 219-byte ciphertext"},
	{operationBeacon, 630_600, 151, 1, "refresh a bundle's delivery beacon as its author; live receipts at heights 514 and 598 on 2026-09-29 (`gasUsed` 6,306), with the author check"},
	{operationFulfill, 197_276_900, 887, 1, "settle a pool order with the delivery proof, without the buyer; live receipt 0xa082ce99… on 2026-09-29 (`gasUsed` 1,972,769 user gas / 197,276,900 wasm, gasCost 19,727.69, txFee 81.7) from the desk's prover run, with delivery image id 2a4f2f7b… and a 692-byte fulfilment argument; the historical v1 signed size was 815 bytes; the delivery-v2 reference envelope is 887 bytes (measured by local serialization on 2026-10-01), while the gas measurement above remains historical v1 evidence and does not qualify the new module; the submit guard prices the size it actually signed"},
	{operationRegisterBundle, 195_741_200, 712, 1, "register a bundle into the pool from a real sealed receipt, with the version 3 journal and its two anchored state versions; live receipt at height 472 on 2026-09-29 (`gasUsed` 1,957,412) and the signed envelope"},
	{operationSellUnit, 196_995_892, 738, 1, "sell one pool unit at the pool's own quote, presenting the sealed registration receipt; reserve-funded supply is disabled until a finite issuance rule exists (`REVIEW.md` A01/A05), so the figure is the harness measurement plus the largest observed live gap until a sale is measured; the signed size carries the version 3 journal"},
	{operationDonateUnit, 630_100, 155, 1, "give one pool unit, paying nothing, as the bundle's author; live receipts at heights 478 and 481 on 2026-09-29 (`gasUsed` 6,301), with the author check"},
	{operationAddReserve, 672_600, 182, 1, "add iDNA to a pool bundle's reserve; live receipt at height 476 on 2026-09-29 (`gasUsed` 6,726), the transaction sends exactly the argument amount"},
	{operationConfirm, 2_035_700, 350, 1, "confirm a pool order by bringing the ciphertext the buyer checked: the call carries the order id and the 219-byte reference payload (`AH-10`), and settlement pays the bundle's registered author the purchase amount and credits the named hosts with the fee; measured on the current module on 2026-09-30: the node's `contract_estimateCall` recorded 20,357 user gas / 2,035,700 wasm for the 227-byte argument, and the same call then settled order 2 live in tx 0xdd130fd1… (host credit 1 iDNA, order `settled`) once the signed max fee carried that budget. The reference envelope is the canonical 350-byte one and the v4 run signed 351, which the submit guard prices as it actually signs. The superseded row carried 1,474,700, the pre-`AH-10` digest form with a 160-byte envelope; it underpriced the call by about 21 iDNA, so the proposal ran out of gas and the node blacklisted the hash (`AH-18`, `PROGRESS.md` section 3)"},
	{operationRefund, 1_357_900, 120, 1, "repay an order that missed its three-block window, out of that order's own escrow, and count the strike: one permissionless transaction pays the original buyer the purchase amount plus the retrieval fee directly, with no credit step; live receipt 0xb338bfe2… on 2026-09-29 (`gasUsed` 13,579 user gas / 1,357,900 wasm, gasCost 135.79, txFee 12.1) under the current module, and 0xc44bac31… at height 1683 (13,008 user gas) under the previous one; the higher figure is carried, the reference size is the smallest signed envelope and the live one was 121 bytes, and the submit guard prices the size it actually signed"},
	{operationReveal, 2_500_000, 235, 1, "conservative signing budget for retained public reveal: historical baseline 1,268,100 wasm (receipt 0x6fe5a420… on 2026-09-29, old deadline rules), plus the pinned node's 20 user gas per stored byte for at most 512 secret bytes and a 28-byte preveal key, plus instruction headroom. Local runtime regressions check retention; this is not a new-chain cost measurement. The 235-byte envelope is the unchanged canonical secret payload; actual signed size is priced separately"},
}

func measuredGasFor(operation string) (operationGas, bool) {
	for _, gas := range measuredOperationGas {
		if gas.Operation == operation {
			return gas, true
		}
	}
	return operationGas{}, false
}

// userGasForWasmGas is the smallest user-gas budget that can run the measured
// wasm gas.
func userGasForWasmGas(wasmGas uint64) uint64 {
	return (wasmGas + wasmGasPerUserGas - 1) / wasmGasPerUserGas
}

// sizeFeeFor is what the node charges for the transaction's size: ten gas per
// byte, at the chain's rate.
func sizeFeeFor(txBytes uint64, feePerGas *big.Int) *big.Int {
	gas := new(big.Int).SetUint64(txBytes * 10)
	return gas.Mul(gas, feePerGas)
}

// requiredMaxFeeFor is the smallest max fee that lets the call run and covers
// what it can cost: the size fee plus the gas budget. The node divides the
// remaining budget by the rate, so rounding up matters.
func requiredMaxFeeFor(wasmGas, txBytes uint64, feePerGas *big.Int) *big.Int {
	gas := new(big.Int).SetUint64(userGasForWasmGas(wasmGas))
	gas.Mul(gas, feePerGas)
	return gas.Add(gas, sizeFeeFor(txBytes, feePerGas))
}

// maxMaxFeeFor is the other end of the range: validation divides the *whole*
// max fee by the rate and refuses the transaction when that exceeds the block's
// gas limit (`blockchain/validation/validation.go`, `TooHighMaxFee`). A deploy
// of the release module hits this first, because its size fee already uses most
// of the allowance.
func maxMaxFeeFor(feePerGas *big.Int) *big.Int {
	cap := new(big.Int).SetUint64(maxUserGasUpgrade11)
	return cap.Mul(cap, feePerGas)
}

// estimateForOperation returns the measured requirement and the max fee it
// implies at the given rate.
func estimateForOperation(operation string, feePerGas *big.Int) (operationGas, *big.Int, bool) {
	gas, ok := measuredGasFor(operation)
	if !ok || feePerGas == nil || feePerGas.Sign() <= 0 {
		return operationGas{}, nil, false
	}
	return gas, requiredMaxFeeFor(gas.WasmGas, gas.TxBytes, feePerGas), true
}

// iDNAText renders base units as iDNA with two decimals, for display only.
func iDNAText(base *big.Int) string {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	value := new(big.Rat).SetFrac(base, scale)
	return value.FloatString(2)
}

// parseFeePerGas reads what `bcn_feePerGas` answers: an integer, or a decimal or
// hex string, whichever the node revision sends.
func parseFeePerGas(raw json.RawMessage) (*big.Int, error) {
	text := strings.TrimSpace(string(raw))
	text = strings.Trim(text, `"`)
	if text == "" || text == "null" {
		return nil, fmt.Errorf("the node answered no fee per gas")
	}
	value := new(big.Int)
	if _, ok := value.SetString(strings.TrimPrefix(text, "0x"), feeBase(text)); !ok || value.Sign() <= 0 {
		return nil, fmt.Errorf("the node's fee per gas %q is not a positive integer", text)
	}
	return value, nil
}

func feeBase(text string) int {
	if strings.HasPrefix(text, "0x") {
		return 16
	}
	return 10
}

// readFeePerGas asks the node for the chain's current rate.
func readFeePerGas(c *client) (*big.Int, error) {
	var raw json.RawMessage
	if err := c.call("bcn_feePerGas", []any{}, &raw); err != nil {
		return nil, err
	}
	return parseFeePerGas(raw)
}

// indexerBlock is the part of the public indexer's block answer this needs. Its
// `feeRate` is iDNA per gas as a decimal string; the chain holds an integer.
type indexerBlock struct {
	Result struct {
		Height  uint64 `json:"height"`
		Epoch   uint16 `json:"epoch"`
		FeeRate string `json:"feeRate"`
	} `json:"result"`
}

// baseUnitsPerGasFromRate converts an iDNA-per-gas decimal into base units per
// gas, rounding down. The indexer publishes thirteen significant digits, so the
// result is an estimate: it is exact to about one part in 1e11, which cannot
// change what a call costs by a base unit's worth of gas.
func baseUnitsPerGasFromRate(rate string) (*big.Int, error) {
	value, ok := new(big.Rat).SetString(strings.TrimSpace(rate))
	if !ok || value.Sign() <= 0 {
		return nil, fmt.Errorf("the indexer's fee rate %q is not a positive decimal", rate)
	}
	value.Mul(value, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)))
	return new(big.Int).Quo(value.Num(), value.Denom()), nil
}

// readIndexerFeePerGas asks the public indexer's last block for the chain's rate.
func readIndexerFeePerGas(baseURL string) (*big.Int, uint64, error) {
	url := strings.TrimSuffix(baseURL, "/") + "/api/Block/Last"
	response, err := (&http.Client{Timeout: 30 * time.Second}).Get(url)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", url, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", url, err)
	}
	var block indexerBlock
	if err := json.Unmarshal(body, &block); err != nil {
		return nil, 0, fmt.Errorf("parse %s: %w", url, err)
	}
	rate, err := baseUnitsPerGasFromRate(block.Result.FeeRate)
	if err != nil {
		return nil, 0, err
	}
	return rate, block.Result.Height, nil
}

// runFees prints the measured gas and the fee it implies at the current rate.
func runFees(operation, feePerGasFlag, indexerURL, rpcURL, apiKey string) {
	var rate *big.Int
	switch {
	case feePerGasFlag != "":
		parsed, ok := new(big.Int).SetString(strings.TrimPrefix(feePerGasFlag, "0x"), feeBase(feePerGasFlag))
		if !ok || parsed.Sign() <= 0 {
			fmt.Fprintf(os.Stderr, "the fee per gas %q is not a positive integer\n", feePerGasFlag)
			os.Exit(2)
		}
		rate = parsed
		fmt.Printf("fee/gas     %s base units (given)\n", rate)
	case indexerURL != "":
		read, height, err := readIndexerFeePerGas(indexerURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		rate = read
		fmt.Printf("indexer     %s at block %d\n", indexerURL, height)
		fmt.Printf("fee/gas     %s base units (from the indexer's decimal, rounded down)\n", rate)
	default:
		node := &client{url: rpcURL, key: apiKey, http: newHTTPClient()}
		read, err := readFeePerGas(node)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read the fee per gas (or pass -fee-per-gas, or -indexer URL): %v\n", err)
			os.Exit(1)
		}
		rate = read
		fmt.Printf("node        %s\n", rpcURL)
		fmt.Printf("fee/gas     %s base units\n", rate)
	}
	fmt.Printf("cap         %d user gas per transaction with upgrade 11, so a max fee above\n", maxUserGasUpgrade11)
	fmt.Printf("            %s base units is refused as too high\n\n", maxMaxFeeFor(rate))

	operations := measuredOperationGas
	if operation != "" {
		one, ok := measuredGasFor(operation)
		if !ok {
			fmt.Fprintf(os.Stderr, "no measured gas for operation %q\n", operation)
			os.Exit(1)
		}
		operations = []operationGas{one}
	}
	fmt.Printf("%-18s %12s %11s %9s %22s %8s %12s\n", "operation", "wasm gas", "user gas", "tx bytes", "max fee (base units)", "of cap", "in iDNA")

	failed := false
	for _, gas := range operations {
		userGas := userGasForWasmGas(gas.WasmGas)
		fee := requiredMaxFeeFor(gas.WasmGas, gas.TxBytes, rate)
		share := float64(userGas) / float64(maxUserGasUpgrade11) * 100
		fmt.Printf("%-18s %12d %11d %9d %22s %7.2f%% %12s\n",
			gas.Operation, gas.WasmGas, userGas, gas.TxBytes, fee, share, iDNAText(fee))
		if userGas > maxUserGasUpgrade11 {
			failed = true
			fmt.Printf("%-18s exceeds the per-transaction gas cap; this operation cannot be sent as one call\n", "")
		}
	}
	fmt.Printf("\nthe node charges the size fee plus the gas used, so the max fee has to cover both;\n")
	fmt.Printf("gas and sizes are measured against the pinned runtime and this escrow (%d runs), and the pool calls carry their live play-money-chain receipts; see docs/SOURCES.md F-76 and docs/PROGRESS.md 2026-09-29\n", totalRuns())
	if failed {
		os.Exit(1)
	}
}

func totalRuns() int {
	total := 0
	for _, gas := range measuredOperationGas {
		total += gas.Runs
	}
	return total
}
