package main

// The sweep's two-path body check as tests: a body is bound to the CID the header
// reported, and its decoded transaction set is compared with the hashes the node
// lists for the same block. `SPEC.md` 11.1's source-authentication row names the
// ways that can fail - altered headers, substituted snapshots, missing bodies,
// incorrect commitments - and this pins the ones the sweep can see.

import (
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/types"
)

// testBody builds a decodable block body with the given number of transactions,
// and returns the transaction hashes the node would list for it.
func testBody(t *testing.T, count int) ([]byte, []string) {
	t.Helper()
	transactions := make([]*types.Transaction, 0, count)
	hashes := make([]string, 0, count)
	for index := 0; index < count; index++ {
		tx := &types.Transaction{
			AccountNonce: uint32(index + 1),
			Epoch:        204,
			Type:         types.SendTx,
			Amount:       big.NewInt(int64(index + 1)),
			MaxFee:       big.NewInt(1),
			Tips:         big.NewInt(0),
			Payload:      []byte{byte(index)},
			Signature:    []byte{0x01, 0x02, 0x03},
		}
		transactions = append(transactions, tx)
		hashes = append(hashes, tx.Hash().String())
	}
	body := &types.Body{Transactions: transactions}
	return body.ToBytes(), hashes
}

func TestBlockBodiesAreBoundToTheirCIDAndTransactionSet(t *testing.T) {
	raw, hashes := testBody(t, 2)
	cid := cidRawSha256(raw)
	serve := func(string) ([]byte, error) { return raw, nil }

	check := checkBlockBody(100, "0xabc", cid, hashes, serve)
	if !check.CIDMatches || !check.Decoded || !check.TxSetMatches {
		t.Fatalf("an honest body was not accepted: %+v", check)
	}
	if check.TxCountBody != 2 || check.TxCountRPC != 2 || check.DerivedTxHash == "" {
		t.Fatalf("the body's counts or derived hash are wrong: %+v", check)
	}

	// A substituted body: the header still names the CID, the bytes are others.
	tampered := append([]byte{}, raw...)
	tampered[len(tampered)-1] ^= 0x01
	check = checkBlockBody(100, "0xabc", cid, hashes, func(string) ([]byte, error) { return tampered, nil })
	if check.CIDMatches {
		t.Fatal("a body that does not hash to the header's cid was accepted")
	}

	// The body is honest but not the one the node lists for that block.
	check = checkBlockBody(100, "0xabc", cid, []string{"0xdead"}, serve)
	if check.TxSetMatches || check.TxCountRPC != 1 {
		t.Fatalf("a body with another transaction set matched: %+v", check)
	}

	// A missing body, and a header that names none.
	check = checkBlockBody(100, "0xabc", cid, hashes, func(string) ([]byte, error) {
		return nil, errors.New("no route to that cid")
	})
	if !strings.Contains(check.Error, "fetch") {
		t.Fatalf("an unavailable body was not reported as such: %+v", check)
	}
	if check = checkBlockBody(100, "0xabc", "", hashes, serve); check.Error != "no body cid in header" {
		t.Fatalf("a header without a body cid produced %+v", check)
	}
}
