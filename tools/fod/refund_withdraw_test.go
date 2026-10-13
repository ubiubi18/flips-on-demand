package main

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/crypto"
)

func refundRequestJSON(contract string, orderID uint64) string {
	return fmt.Sprintf(`{
  "format": "fod.action-request",
  "version": 1,
  "network": "idena-local",
  "contract": %q,
  "operation": "refund",
  "arguments": {"orderId": %d},
  "amount": "0",
  "createdAt": "2026-09-25T21:00:00Z"
}`, contract, orderID)
}

func withdrawRequestJSON(contract, arguments string) string {
	return fmt.Sprintf(`{
  "format": "fod.action-request",
  "version": 1,
  "network": "idena-local",
  "contract": %q,
  "operation": "withdraw",
  "arguments": %s,
  "amount": "0",
  "createdAt": "2026-09-25T21:00:00Z"
}`, contract, arguments)
}

func TestPrepareWithdrawalCarriesNoArguments(t *testing.T) {
	dir := t.TempDir()
	path := writeRequest(t, dir, withdrawRequestJSON(testContract, "{}"))
	request, err := loadWithdrawalRequest(path)
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	prepared, err := PrepareWithdrawal(request)
	if err != nil {
		t.Fatalf("prepare withdrawal: %v", err)
	}
	if prepared.Format != preparedWithdrawalFormat || prepared.Operation != operationWithdrawal {
		t.Fatalf("unexpected artefact: %s / %s", prepared.Format, prepared.Operation)
	}
	if prepared.Calldata != "" {
		t.Fatalf("a withdrawal takes no argument, the artefact carries %q", prepared.Calldata)
	}
}

// The reserve's donor-owned exit has its own operation and format (`AH-15`): a
// `withdrawReserve` request prepares the escrow's `withdrawReserve` call, which
// takes no argument because the credit is the caller's own.
func TestPrepareReserveWithdrawalCarriesNoArguments(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(withdrawRequestJSON(testContract, "{}"), `"operation": "withdraw"`, `"operation": "withdrawReserve"`, 1)
	path := writeRequest(t, dir, body)
	request, err := loadReserveWithdrawalRequest(path)
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	prepared, err := PrepareReserveWithdrawal(request)
	if err != nil {
		t.Fatalf("prepare reserve withdrawal: %v", err)
	}
	if prepared.Format != preparedReserveWithdrawalFormat || prepared.Operation != operationReserveWithdrawal {
		t.Fatalf("unexpected artefact: %s / %s", prepared.Format, prepared.Operation)
	}
	if prepared.Calldata != "" {
		t.Fatalf("a reserve withdrawal takes no argument, the artefact carries %q", prepared.Calldata)
	}
	// The prepared file survives the submit reader, and the two withdrawal
	// readers cannot be confused for one another.
	preparedPath := writeRequest(t, dir, preparedClaimJSON(prepared))
	loaded, err := loadPreparedAction(preparedPath)
	if err != nil {
		t.Fatalf("submit refused the reserve withdrawal: %v", err)
	}
	if loaded.Operation != operationReserveWithdrawal {
		t.Fatalf("submit read the reserve withdrawal as %q", loaded.Operation)
	}
	if _, err := loadWithdrawalRequest(path); err == nil {
		t.Fatal("a reserve withdrawal was accepted as a credit withdrawal")
	}
}

// TestPreparePoolRefundOrderWritesThePoolEncoding: the refund names an order id
// in decimal text, exactly the encoding the escrow's `parse_u64` reads, and the
// prepared file is one `fod submit` accepts.
func TestPreparePoolRefundOrderWritesThePoolEncoding(t *testing.T) {
	body := fmt.Sprintf(`{
  "format": "fod.action-request",
  "version": 1,
  "network": "idena-local",
  "contract": %q,
  "operation": "refund",
  "arguments": {"orderId": 9},
  "amount": "0",
  "createdAt": "2026-09-26T12:00:00Z"
}`, testContract)
	requestPath := writeRequest(t, t.TempDir(), body)
	request, arguments, err := loadRefundRequest(requestPath)
	if err != nil {
		t.Fatalf("load the pool refund: %v", err)
	}
	if request.Operation != operationRefund || arguments.OrderID != 9 {
		t.Fatalf("the pool refund reads as %s / %d", request.Operation, arguments.OrderID)
	}
	prepared := PreparePoolRefund(request, arguments)
	if prepared.Format != preparedPoolRefundFormat || prepared.Operation != operationRefund {
		t.Fatalf("the pool refund is %s / %s", prepared.Format, prepared.Operation)
	}
	calldata, err := hex.DecodeString(prepared.Calldata)
	if err != nil || string(calldata) != "9" {
		t.Fatalf("the pool refund calldata is %q (%v)", calldata, err)
	}
	out := filepath.Join(t.TempDir(), "refund-order.json")
	if err := writeJSON(out, prepared); err != nil {
		t.Fatalf("write the pool refund: %v", err)
	}
	loaded, err := loadPreparedAction(out)
	if err != nil {
		t.Fatalf("load the pool refund through the submit reader: %v", err)
	}
	if loaded.Operation != operationRefund || loaded.OfferID != 9 {
		t.Fatalf("the loaded pool refund is %+v", loaded)
	}

	badPath := writeRequest(t, t.TempDir(), strings.Replace(body, `"orderId": 9`, `"orderId": 0`, 1))
	if _, _, err := loadRefundRequest(badPath); err == nil {
		t.Fatal("a pool refund without an order id was accepted")
	}
}

func TestClaimCommandsRefuseRequestsTheyCannotPrepare(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		refusedBy string
	}{
		{"withdraw is not a refund", withdrawRequestJSON(testContract, "{}"), "refund"},
		{"refund is not a withdrawal", refundRequestJSON(testContract, 7), "withdraw"},
		{"refund names no order", refundRequestJSON(testContract, 0), "both"},
		{"refund with an unknown argument", strings.Replace(
			refundRequestJSON(testContract, 7), `"orderId": 7`, `"orderId": 7, "buyer": "x"`, 1), "both"},
		{"withdraw with an argument", withdrawRequestJSON(testContract, `{"orderId": 7}`), "both"},
		{"refund without an amount", strings.Replace(
			refundRequestJSON(testContract, 7), `"amount": "0",`, "", 1), "both"},
	}
	for _, tc := range cases {
		path := writeRequest(t, t.TempDir(), tc.body)
		// Neither command may prepare the other's operation, and neither may
		// prepare a request it cannot honour.
		_, _, refundErr := loadRefundRequest(path)
		_, withdrawErr := loadWithdrawalRequest(path)
		if tc.refusedBy != "withdraw" && refundErr == nil {
			t.Errorf("%s: the refund loader accepted the request", tc.name)
		}
		if tc.refusedBy != "refund" && withdrawErr == nil {
			t.Errorf("%s: the withdrawal loader accepted the request", tc.name)
		}
	}
}

// TestWithdrawalSurvivesTheTransactionRoundTrip covers the empty-argument call:
// a withdrawal names no arguments, and the transaction has to come back through
// the node's decoder still naming the method.
func TestWithdrawalSurvivesTheTransactionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	keyPath, _ := writeTestKey(t, dir, "nodekey")
	identity, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		t.Fatalf("load nodekey: %v", err)
	}
	path := writeRequest(t, dir, withdrawRequestJSON(testContract, "{}"))
	request, err := loadWithdrawalRequest(path)
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	prepared, err := PrepareWithdrawal(request)
	if err != nil {
		t.Fatalf("prepare withdrawal: %v", err)
	}
	action := &preparedAction{
		Format:    prepared.Format,
		Version:   prepared.Version,
		Operation: prepared.Operation,
		Network:   prepared.Network,
		Contract:  prepared.Contract,
		Amount:    prepared.Amount,
		Calldata:  prepared.Calldata,
	}
	rawTx, _, err := BuildTransaction(action, identity, 3, 206, big.NewInt(1_000_000), big.NewInt(0))
	if err != nil {
		t.Fatalf("build transaction: %v", err)
	}
	var decoded types.Transaction
	if err := decoded.FromBytes(rawTx); err != nil {
		t.Fatalf("decode: %v", err)
	}
	attachment := attachments.ParseCallContractAttachment(&decoded)
	if attachment == nil || attachment.Method != operationWithdrawal {
		t.Fatalf("the withdrawal call did not survive: %+v", attachment)
	}
	if amountOrZero(decoded.Amount).Sign() != 0 {
		t.Fatalf("a withdrawal sends no value, the transaction carries %v", decoded.Amount)
	}
	t.Logf("withdrawal transaction: %d bytes, %d args", len(rawTx), len(attachment.Args))

	// The same prepared file has to survive `fod submit` reading it back.
	preparedPath := writeRequest(t, dir, preparedClaimJSON(prepared))
	loaded, err := loadPreparedAction(preparedPath)
	if err != nil {
		t.Fatalf("submit refused the withdrawal: %v", err)
	}
	if loaded.Operation != operationWithdrawal || loaded.Calldata != "" {
		t.Fatalf("submit read the withdrawal as %+v", loaded)
	}
}

func preparedClaimJSON(claim *PreparedClaim) string {
	return fmt.Sprintf(`{
  "format": %q,
  "version": %d,
  "operation": %q,
  "network": %q,
  "contract": %q,
  "amount": %q,
  "calldata": %q,
  "preparedAt": %q,
  "submitted": false
}`, claim.Format, claim.Version, claim.Operation, claim.Network, claim.Contract, claim.Amount, claim.Calldata, claim.PreparedAt)
}
