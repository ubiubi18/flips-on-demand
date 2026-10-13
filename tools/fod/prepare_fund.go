package main

// The owner's funding step of `SPEC.md` 7.5: the pool's reserve is funded by
// whoever wants to add to it. `addReserve` takes the bundle commitment and the
// amount as a 128-bit argument, and the transaction has to send exactly that
// amount, so the prepared action carries the same figure twice.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

const (
	operationAddReserve = "addReserve"
	preparedFundFormat  = "fod.prepared-fund"
)

// preparePoolFund builds the `addReserve` call: commitment (32) || amount (16,
// big-endian). The prepared amount is the same figure, so `fod submit` sends
// exactly what the contract checks.
func preparePoolFund(contractHex, bundleHex, amount, network string) (*preparedAction, error) {
	contract := strings.ToLower(strings.TrimPrefix(contractHex, "0x"))
	if !isHex(contract, addressBytes) {
		return nil, errors.New("the contract address is not 20 bytes of hex")
	}
	bundle := strings.ToLower(strings.TrimPrefix(bundleHex, "0x"))
	if !isHex(bundle, poolBundleBytes) {
		return nil, errors.New("the bundle commitment is not 32 bytes of hex")
	}
	if !isDecimal(amount) {
		return nil, errors.New("the amount is not an integer in base units")
	}
	value, ok := new(big.Int).SetString(amount, 10)
	if !ok || value.Sign() <= 0 {
		return nil, errors.New("the amount must be positive")
	}
	if value.Cmp(maxContractAmount) > 0 {
		return nil, errors.New("the amount is larger than the contract can hold")
	}
	commitment, err := hex.DecodeString(bundle)
	if err != nil {
		return nil, err
	}
	calldata := make([]byte, 0, poolBundleBytes+16)
	calldata = append(calldata, commitment...)
	calldata = append(calldata, value.FillBytes(make([]byte, 16))...)
	return &preparedAction{
		Format:     preparedFundFormat,
		Version:    actionRequestVersion,
		Operation:  operationAddReserve,
		Network:    network,
		Contract:   contract,
		Amount:     amount,
		Calldata:   hex.EncodeToString(calldata),
		Commitment: bundle,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func runPrepareFund(contractHex, bundleHex, amount, network, preparedPath string) {
	if contractHex == "" || bundleHex == "" || amount == "" || preparedPath == "" {
		fmt.Fprintln(os.Stderr, "prepare-fund needs -contract ADDRESS -pool-bundle COMMITMENT -amount BASE_UNITS -prepared FILE")
		os.Exit(2)
	}
	prepared, err := preparePoolFund(contractHex, bundleHex, amount, network)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("operation   %s on %s\n", prepared.Operation, prepared.Network)
	fmt.Printf("contract    %s\n", prepared.Contract)
	fmt.Printf("bundle      %s\n", prepared.Commitment)
	fmt.Printf("amount      %s base units (the transaction sends exactly this)\n", prepared.Amount)
	fmt.Printf("calldata    %d bytes\n", len(prepared.Calldata)/2)
	if err := writeJSON(preparedPath, prepared); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
		os.Exit(1)
	}
	fmt.Printf("prepared    %s\n", preparedPath)
	fmt.Println("not submitted: pass this file to `fod submit` to sign it; the funder's nodekey sends it")
}
