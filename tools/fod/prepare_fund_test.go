package main

// The funding preparation: the commitment and the 128-bit amount, with the
// same figure sent as the transaction's value.

import (
	"encoding/hex"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func TestPreparePoolFundWritesTheAmountAndCommitment(t *testing.T) {
	contract := strings.Repeat("11", addressBytes)
	bundle := strings.Repeat("ab", poolBundleBytes)
	amount := "100000000000000000000" // 100 iDNA
	prepared, err := preparePoolFund(contract, bundle, amount, "idena-local")
	if err != nil {
		t.Fatalf("prepare the fund: %v", err)
	}
	if prepared.Format != preparedFundFormat || prepared.Operation != operationAddReserve {
		t.Fatalf("the fund is %s / %s", prepared.Format, prepared.Operation)
	}
	if prepared.Amount != amount || prepared.Commitment != bundle {
		t.Fatalf("the fund carries amount %s and bundle %s", prepared.Amount, prepared.Commitment)
	}
	calldata, err := hex.DecodeString(prepared.Calldata)
	if err != nil {
		t.Fatalf("decode the fund calldata: %v", err)
	}
	if len(calldata) != poolBundleBytes+16 {
		t.Fatalf("the fund calldata is %d bytes", len(calldata))
	}
	if hex.EncodeToString(calldata[:poolBundleBytes]) != bundle {
		t.Fatalf("the fund calldata starts with %x", calldata[:poolBundleBytes])
	}
	if value := new(big.Int).SetBytes(calldata[poolBundleBytes:]); value.String() != amount {
		t.Fatalf("the fund calldata carries %s", value)
	}
	out := filepath.Join(t.TempDir(), "fund.json")
	if err := writeJSON(out, prepared); err != nil {
		t.Fatalf("write the fund: %v", err)
	}
	loaded, err := loadPreparedAction(out)
	if err != nil {
		t.Fatalf("load the fund through the submit reader: %v", err)
	}
	if loaded.Operation != operationAddReserve || loaded.Amount != amount {
		t.Fatalf("the loaded fund is %+v", loaded)
	}
}

func TestPreparePoolFundRefusesBadInputs(t *testing.T) {
	contract := strings.Repeat("11", addressBytes)
	bundle := strings.Repeat("ab", poolBundleBytes)
	for name, call := range map[string]func() (*preparedAction, error){
		"a zero amount": func() (*preparedAction, error) {
			return preparePoolFund(contract, bundle, "0", "idena-local")
		},
		"a non-integer amount": func() (*preparedAction, error) {
			return preparePoolFund(contract, bundle, "12x", "idena-local")
		},
		"an amount above u128": func() (*preparedAction, error) {
			return preparePoolFund(contract, bundle, new(big.Int).Add(maxContractAmount, big.NewInt(1)).String(), "idena-local")
		},
		"a short bundle": func() (*preparedAction, error) {
			return preparePoolFund(contract, "abcd", "1", "idena-local")
		},
		"a short contract": func() (*preparedAction, error) {
			return preparePoolFund("11", bundle, "1", "idena-local")
		},
	} {
		if _, err := call(); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestPoolFundCanonicalSignedSize(t *testing.T) {
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the signing key: %v", err)
	}
	prepared, err := preparePoolFund(strings.Repeat("aa", addressBytes), strings.Repeat("ab", poolBundleBytes), "100000000000000000000", "testnet")
	if err != nil {
		t.Fatalf("prepare the fund: %v", err)
	}
	raw, _, err := BuildTransaction(prepared, identity, 1, 228, big.NewInt(1_000_000_000_000_000), big.NewInt(0))
	if err != nil {
		t.Fatalf("sign the fund: %v", err)
	}
	gas, ok := measuredGasFor(operationAddReserve)
	if !ok {
		t.Fatal("addReserve has no measured gas")
	}
	if gas.TxBytes != uint64(len(raw)) {
		t.Fatalf("addReserve signs %d bytes, the fee table records %d", len(raw), gas.TxBytes)
	}
}
