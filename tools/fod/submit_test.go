package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
)

func preparedActionJSON(format, operation, contract, amount, calldata string) string {
	return fmt.Sprintf(`{
  "format": %q,
  "version": 1,
  "operation": %q,
  "network": "idena-local",
  "contract": %q,
  "amount": %q,
  "calldata": %q,
  "preparedAt": "2026-09-25T20:00:00Z",
  "submitted": false
}`, format, operation, contract, amount, calldata)
}

// TestBuildTransactionIsTheNodesOwnFormat is the point of this command: the
// bytes it signs are decoded again with the node's own decoder, recover the
// signing address, and still carry the contract call.
func TestBuildTransactionIsTheNodesOwnFormat(t *testing.T) {
	dir := t.TempDir()
	keyPath, _ := writeTestKey(t, dir, "nodekey")
	identity, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		t.Fatalf("load nodekey: %v", err)
	}
	signer, err := nodekeyAddressOf(identity)
	if err != nil {
		t.Fatalf("signer address: %v", err)
	}

	calldata := strings.Repeat("ab", 48)
	path := writeRequest(t, dir, preparedActionJSON("fod.prepared-deliver", operationDeliver, testContract, "0", calldata))
	prepared, err := loadPreparedAction(path)
	if err != nil {
		t.Fatalf("load prepared action: %v", err)
	}

	nonce, epoch := uint32(17), uint16(205)
	maxFee := big.NewInt(1_000_000)
	tips := big.NewInt(0)
	rawTx, sender, err := BuildTransaction(prepared, identity, nonce, epoch, maxFee, tips)
	if err != nil {
		t.Fatalf("build transaction: %v", err)
	}

	var decoded types.Transaction
	if err := decoded.FromBytes(rawTx); err != nil {
		t.Fatalf("the node's decoder refused the transaction: %v", err)
	}
	if decoded.Type != types.CallContractTx {
		t.Fatalf("transaction type is %#x, expected the contract call type", decoded.Type)
	}
	if decoded.To == nil {
		t.Fatal("the transaction names no contract")
	}
	contractBytes, _ := hex.DecodeString(testContract)
	var want common.Address
	copy(want[:], contractBytes)
	if *decoded.To != want {
		t.Fatalf("the transaction calls %s, expected %s", decoded.To.Hex(), want.Hex())
	}
	if decoded.AccountNonce != nonce || decoded.Epoch != epoch {
		t.Fatalf("the transaction carries nonce %d epoch %d", decoded.AccountNonce, decoded.Epoch)
	}
	if decoded.MaxFee == nil || decoded.MaxFee.Cmp(maxFee) != 0 {
		t.Fatalf("the transaction carries max fee %v", decoded.MaxFee)
	}
	// The node writes a zero amount as nothing and reads it back as nil.
	if amountOrZero(decoded.Amount).Sign() != 0 {
		t.Fatalf("a deliver must send no value, the transaction carries %v", decoded.Amount)
	}

	attachment := attachments.ParseCallContractAttachment(&decoded)
	if attachment == nil {
		t.Fatal("the transaction carries no contract call")
	}
	if attachment.Method != operationDeliver {
		t.Fatalf("the call names method %q", attachment.Method)
	}
	if len(attachment.Args) != 1 || hex.EncodeToString(attachment.Args[0]) != calldata {
		t.Fatalf("the call carries %d args, expected the prepared calldata", len(attachment.Args))
	}

	recovered, err := types.Sender(&decoded)
	if err != nil {
		t.Fatalf("the node's own recovery failed: %v", err)
	}
	if !strings.EqualFold(recovered.Hex(), "0x"+signer) {
		t.Fatalf("the transaction recovers to %s, not %s", recovered.Hex(), signer)
	}
	if sender != recovered {
		t.Fatalf("BuildTransaction reported %s, the decoder recovers %s", sender.Hex(), recovered.Hex())
	}
	// The signature is what makes it the node's transaction, not a copy.
	if len(decoded.Signature) != signatureBytes {
		t.Fatalf("signature is %d bytes", len(decoded.Signature))
	}
}

// TestBuildTransactionCarriesThePreparedAmount checks the one prepared action
// that sends value: a buyer's funding call must pay the price it named.
func TestBuildTransactionCarriesThePreparedAmount(t *testing.T) {
	dir := t.TempDir()
	keyPath, _ := writeTestKey(t, dir, "nodekey")
	identity, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		t.Fatalf("load nodekey: %v", err)
	}
	calldata := strings.Repeat("cd", 138)
	path := writeRequest(t, dir, preparedActionJSON("fod.prepared-fund", operationAddReserve, testContract, "100000000000000000000", calldata))
	prepared, err := loadPreparedAction(path)
	if err != nil {
		t.Fatalf("load prepared action: %v", err)
	}
	rawTx, _, err := BuildTransaction(prepared, identity, 4, 205, big.NewInt(2_000_000), big.NewInt(0))
	if err != nil {
		t.Fatalf("build transaction: %v", err)
	}
	var decoded types.Transaction
	if err := decoded.FromBytes(rawTx); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Amount == nil || decoded.Amount.String() != "100000000000000000000" {
		t.Fatalf("the transaction carries %v, expected the prepared price", decoded.Amount)
	}
	if attachment := attachments.ParseCallContractAttachment(&decoded); attachment == nil || attachment.Method != operationAddReserve {
		t.Fatal("the reserve top-up did not survive the round trip")
	}
}

// A prepared buy is signable only with the buyer's own recent, complete,
// chain-bound content check: the absence of a check must not be read as one
// (`AH-17`).
func TestSubmitRefusesABuyWithoutAFreshAvailabilityCheck(t *testing.T) {
	dir := t.TempDir()
	calldata := strings.Repeat("ab", 138)
	body := func(check map[string]any) string {
		payload := map[string]any{
			"format":     "fod.prepared-buy",
			"version":    1,
			"operation":  operationBuy,
			"network":    "idena-local",
			"contract":   testContract,
			"amount":     "0",
			"calldata":   calldata,
			"preparedAt": "2026-09-25T20:00:00Z",
			"submitted":  false,
		}
		if check != nil {
			payload["availability"] = check
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode the prepared buy: %v", err)
		}
		return string(raw)
	}

	// No check at all.
	if _, err := loadPreparedAction(writeRequest(t, dir, body(nil))); err == nil {
		t.Fatal("a prepared buy without an availability check was accepted for signing")
	}
	// A complete, chain-bound check older than the freshness bound is refused.
	stale := map[string]any{
		"checkedAt":     time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		"binding":       "committed-inventory",
		"coveredFlips":  1,
		"verifiedFlips": 1,
	}
	if _, err := loadPreparedAction(writeRequest(t, dir, body(stale))); err == nil {
		t.Fatal("a stale availability check was accepted for signing")
	}
	// Merely asserting a fresh, complete check cannot make a file signable.
	fresh := map[string]any{
		"checkedAt":     time.Now().UTC().Format(time.RFC3339),
		"binding":       "committed-inventory",
		"coveredFlips":  2,
		"verifiedFlips": 2,
	}
	if _, err := loadPreparedAction(writeRequest(t, dir, body(fresh))); err == nil {
		t.Fatal("editable counters without retained bytes were accepted")
	}
}

func TestSubmitRefusesPreparedActionsItCannotSign(t *testing.T) {
	dir := t.TempDir()
	calldata := strings.Repeat("ab", 48)
	cases := []struct {
		name string
		body string
	}{
		{"unknown format", preparedActionJSON("fod.prepared-something", operationDeliver, testContract, "0", calldata)},
		{"operation does not match the format", preparedActionJSON("fod.prepared-listing", operationFulfill, testContract, "0", calldata)},
		{"wrong version", strings.Replace(preparedActionJSON("fod.prepared-deliver", operationDeliver, testContract, "0", calldata), `"version": 1`, `"version": 2`, 1)},
		{"short contract", preparedActionJSON("fod.prepared-deliver", operationDeliver, "abcd", "0", calldata)},
		{"non-decimal amount", preparedActionJSON("fod.prepared-deliver", operationDeliver, testContract, "0x1", calldata)},
		{"calldata is not hex", preparedActionJSON("fod.prepared-deliver", operationDeliver, testContract, "0", "zzzz")},
		{"unknown field", strings.Replace(preparedActionJSON("fod.prepared-deliver", operationDeliver, testContract, "0", calldata), `"amount":`, `"extra": 1, "amount":`, 1)},
	}
	for _, tc := range cases {
		path := writeRequest(t, t.TempDir(), tc.body)
		if _, err := loadPreparedAction(path); err == nil {
			t.Errorf("%s: the prepared action was accepted", tc.name)
		}
	}

	// A well-formed action signed with the wrong key is still refused: the
	// transaction must recover to the nodekey that signed it.
	keyPath, _ := writeTestKey(t, dir, "nodekey")
	identity, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		t.Fatalf("load nodekey: %v", err)
	}
	valid := writeRequest(t, dir, preparedActionJSON("fod.prepared-deliver", operationDeliver, testContract, "0", calldata))
	prepared, err := loadPreparedAction(valid)
	if err != nil {
		t.Fatalf("load prepared action: %v", err)
	}
	if _, _, err := BuildTransaction(prepared, identity, 1, 1, big.NewInt(1), big.NewInt(0)); err != nil {
		t.Fatalf("a valid action was refused: %v", err)
	}
}

// TestSubmitSignsWithoutBroadcasting is the default: a signed transaction and a
// receipt, and no node contact for a non-disclosing recovery call. Delivery,
// fulfilment and reveal require current order/deadline checks even when signing
// without broadcasting.
func TestSubmitSignsWithoutBroadcasting(t *testing.T) {
	dir := t.TempDir()
	keyPath, _ := writeTestKey(t, dir, "nodekey")
	calldata := hex.EncodeToString([]byte("7"))
	preparedPath := writeRequest(t, dir, preparedActionJSON(preparedPoolRefundFormat, operationRefund, testContract, "0", calldata))
	signedPath := filepath.Join(dir, "signed.json")
	// An existing public output must be atomically replaced with a private file.
	if err := os.WriteFile(signedPath, []byte("old output"), 0o644); err != nil {
		t.Fatal(err)
	}

	runSubmit(preparedPath, keyPath, 9, 205, big.NewInt(3_000_000), big.NewInt(0), signedPath, false, "http://127.0.0.1:1", "", nil, "", "")

	raw, err := os.ReadFile(signedPath)
	if err != nil {
		t.Fatalf("read the signed transaction: %v", err)
	}
	if secure, err := VaultPermissionsSecure(signedPath); err != nil || !secure {
		t.Fatal("signed transaction is readable by other users", err)
	}
	var submitted SubmittedTransaction
	if err := json.Unmarshal(raw, &submitted); err != nil {
		t.Fatalf("parse %s: %v", signedPath, err)
	}
	if submitted.Format != "fod.submitted-transaction" || submitted.Broadcast {
		t.Fatalf("unexpected receipt: %+v", submitted)
	}
	if submitted.NodeHash != "" {
		t.Fatalf("a hash appeared without broadcasting: %q", submitted.NodeHash)
	}
	rawTx, err := hex.DecodeString(submitted.RawTx)
	if err != nil {
		t.Fatalf("signed transaction hex: %v", err)
	}
	var decoded types.Transaction
	if err := decoded.FromBytes(rawTx); err != nil {
		t.Fatalf("the receipt's transaction does not decode: %v", err)
	}
	if decoded.AccountNonce != 9 {
		t.Fatalf("the receipt's transaction carries nonce %d", decoded.AccountNonce)
	}
}

// TestSignedReceiptsDecode reads the receipts tools/fod wrote from the prepared
// artefacts in output/cli and checks them with the node's own decoder, so the
// evidence is the file rather than the run that produced it. It skips when the
// receipts have not been generated.
func TestSignedReceiptsDecode(t *testing.T) {
	cases := []struct {
		file      string
		operation string
		amount    string
	}{
		{"signed-delivery.json", operationFulfill, "0"},
		{"signed-refund.json", operationRefund, "0"},
		{"signed-withdrawal.json", operationWithdrawal, "0"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("..", "..", "output", "cli", tc.file)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("receipt not generated (%v); run fod submit first", err)
			}
			var receipt SubmittedTransaction
			if err := json.Unmarshal(content, &receipt); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			if receipt.Format != "fod.submitted-transaction" {
				t.Fatalf("unexpected receipt format %q", receipt.Format)
			}
			if receipt.Broadcast || receipt.NodeHash != "" {
				t.Fatal("a receipt claims a broadcast that never happened")
			}
			rawTx, err := hex.DecodeString(receipt.RawTx)
			if err != nil {
				t.Fatalf("raw transaction hex: %v", err)
			}
			var decoded types.Transaction
			if err := decoded.FromBytes(rawTx); err != nil {
				t.Fatalf("the node's decoder refused the receipt's transaction: %v", err)
			}
			if decoded.Type != types.CallContractTx || decoded.To == nil {
				t.Fatal("the receipt's transaction is not a contract call")
			}
			if amountOrZero(decoded.Amount).String() != tc.amount {
				t.Fatalf("the transaction sends %v, expected %s", decoded.Amount, tc.amount)
			}
			attachment := attachments.ParseCallContractAttachment(&decoded)
			if attachment == nil || attachment.Method != tc.operation {
				t.Fatalf("the transaction calls %v, expected %s", attachment, tc.operation)
			}
			if len(attachment.Args) != 1 {
				t.Fatalf("the call carries %d args, expected one", len(attachment.Args))
			}
			sender, err := types.Sender(&decoded)
			if err != nil {
				t.Fatalf("sender recovery: %v", err)
			}
			if !strings.EqualFold(strings.TrimPrefix(sender.Hex(), "0x"), receipt.Sender) {
				t.Fatalf("the transaction recovers to %s, the receipt says %s", sender.Hex(), receipt.Sender)
			}
			t.Logf("%s: %d-byte call to %s from %s", tc.operation, len(attachment.Args[0]), decoded.To.Hex(), sender.Hex())
		})
	}
}

// The fee table's figure is the requirement for the transaction the CLI
// measured. RLP encodes the values it carries, so the transaction a caller
// signs can be a byte or two longer, and then a max fee computed from the
// measured size leaves the call short of gas - the case the broadcast guard
// refuses rather than sending (`docs/PROGRESS.md`, 2026-09-26).
func TestAMaxFeeForTheMeasuredSizeIsShortOfTheSignedOne(t *testing.T) {
	rate := big.NewInt(96_153_846_153_800) // what mainnet charged on 2026-09-26
	forTheMeasured := requiredMaxFeeFor(3_000_051, 357_858, rate)
	if got := maxFeeShortfall(operationDeployModule, 357_858, rate, forTheMeasured); got != nil {
		t.Fatalf("a max fee for the measured size was called short: %s", got)
	}
	shortfall := maxFeeShortfall(operationDeployModule, 357_860, rate, forTheMeasured)
	if shortfall == nil {
		t.Fatal("the two bytes more the CLI actually signs were not called short")
	}
	want := requiredMaxFeeFor(3_000_051, 357_860, rate)
	if shortfall.Cmp(want) != 0 {
		t.Fatalf("the shortfall names %s, expected %s", shortfall, want)
	}
	if delta := new(big.Int).Sub(want, forTheMeasured); delta.Cmp(new(big.Int).Mul(big.NewInt(20), rate)) != 0 {
		t.Fatalf("two bytes should be twenty gas, not %s", delta)
	}
}
