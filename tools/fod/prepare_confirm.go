package main

// The buyer's confirmation (`SPEC.md` 7.1): one call that settles a pool order
// on the ciphertext the buyer's own tool checked. The calldata is the order id
// followed by those exact bytes, so a payload an outsider replaced after the
// check cannot strand the payment (`AH-10`).

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	operationConfirm      = "confirm"
	preparedConfirmFormat = "fod.prepared-confirm"
)

// confirmCalldata is the escrow's argument: the order id, then the exact
// ciphertext bytes the buyer checked (`contracts/escrow/src/lib.rs`, `confirm`).
func confirmCalldata(orderID uint64, ciphertextHex string) (string, error) {
	ciphertext, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(ciphertextHex), "0x"))
	if err != nil || len(ciphertext) == 0 {
		return "", fmt.Errorf("the ciphertext has to be the checked bytes, hex encoded")
	}
	arg := make([]byte, 0, 8+len(ciphertext))
	arg = binary.BigEndian.AppendUint64(arg, orderID)
	arg = append(arg, ciphertext...)
	return hex.EncodeToString(arg), nil
}

// prepareConfirmAction builds the `confirm` call for one pool order from the
// bytes the buyer's own run checked and recorded.
func prepareConfirmAction(contract, network string, orderID uint64, ciphertextHex string) (*preparedAction, error) {
	calldata, err := confirmCalldata(orderID, ciphertextHex)
	if err != nil {
		return nil, err
	}
	ciphertext, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(ciphertextHex), "0x"))
	if err != nil {
		return nil, fmt.Errorf("the ciphertext has to be hex encoded")
	}
	digest := sha256.Sum256(ciphertext)
	return &preparedAction{
		Format:    preparedConfirmFormat,
		Version:   actionRequestVersion,
		Operation: operationConfirm,
		Network:   network,
		Contract:  strings.ToLower(strings.TrimPrefix(contract, "0x")),
		Amount:    "0",
		Calldata:  calldata,
		// Recorded so a reader sees which payload the confirmation settles.
		CiphertextSha256: hex.EncodeToString(digest[:]),
		OfferID:          orderID,
		PreparedAt:       time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func runPrepareConfirm(contract, network string, orderID uint64, deliveryPath, preparedPath string) {
	if contract == "" || orderID == 0 || preparedPath == "" {
		fmt.Fprintln(os.Stderr, "prepare-confirm needs -contract ADDRESS -pool-order ID -prepared FILE")
		os.Exit(2)
	}
	if !isHex(strings.ToLower(strings.TrimPrefix(contract, "0x")), addressBytes) {
		fmt.Fprintln(os.Stderr, "prepare-confirm needs a 20-byte contract address")
		os.Exit(2)
	}
	// The bytes are not read from the chain: they are the ones the buyer's own
	// run recorded after checking them. Naming the chain's current payload
	// instead would confirm whatever an outsider had just published.
	opened, err := readUsableDeliveryRecord(deliveryPath, contract, orderID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "confirm refused: %v\n", err)
		os.Exit(1)
	}
	ciphertextHex, digest := opened.CiphertextHex, opened.CiphertextSha256
	prepared, err := prepareConfirmAction(contract, network, orderID, ciphertextHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "confirm refused: %v\n", err)
		os.Exit(1)
	}
	prepared.VerifiedDelivery = opened
	if err := writePrivateJSON(preparedPath, prepared); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
		os.Exit(1)
	}
	fmt.Printf("order       %d\n", orderID)
	fmt.Printf("ciphertext  sha256 %s (%d bytes this buyer checked)\n", digest, len(ciphertextHex)/2)
	fmt.Printf("calldata    %s (%d bytes: the order id, then those bytes)\n", prepared.Calldata, len(prepared.Calldata)/2)
	fmt.Printf("prepared    %s\n", preparedPath)
	fmt.Println("not submitted: pass this file to `fod submit` to sign it; the buyer's nodekey sends it")
}

// confirmPayloadFromRecord reads the record `fod receive -out` wrote and
// returns the checked ciphertext it carries with its digest. The order and the
// contract have to be the ones the record names, so a record for another order
// cannot be used to settle this one.
func confirmPayloadFromRecord(path, contract string, orderID uint64) (string, string, error) {
	opened, err := readUsableDeliveryRecord(path, contract, orderID)
	if err != nil {
		return "", "", err
	}
	return opened.CiphertextHex, opened.CiphertextSha256, nil
}

func readUsableDeliveryRecord(path, contract string, orderID uint64) (*OpenedPoolDelivery, error) {
	if path == "" {
		return nil, fmt.Errorf(
			"no -delivery record was named: run `fod receive ... -out opened.json` first, because this call has to name the ciphertext the buyer checked, not whatever the chain holds now")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the opened delivery: %w", err)
	}
	var opened OpenedPoolDelivery
	if err := decodeStrict(raw, &opened); err != nil {
		return nil, fmt.Errorf("read the opened delivery: %w", err)
	}
	if opened.Format != "fod.opened-pool-delivery" {
		return nil, fmt.Errorf("%s is not an opened pool delivery", path)
	}
	if opened.KeySource == "public-reveal" {
		return nil, fmt.Errorf("this order is already settled by public reveal; recovered content needs no confirmation")
	}
	if opened.UsableFlips < 1 {
		return nil, fmt.Errorf("the delivery record did not verify usable retained content; receive with -prepared BUY_FILE first")
	}
	if opened.OrderID != orderID {
		return nil, fmt.Errorf("the record is for pool order %d, not %d", opened.OrderID, orderID)
	}
	if !strings.EqualFold(strings.TrimPrefix(opened.Contract, "0x"), strings.TrimPrefix(contract, "0x")) {
		return nil, fmt.Errorf("the record names contract %s, not %s", opened.Contract, contract)
	}
	ciphertext, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(opened.CiphertextHex), "0x"))
	if err != nil || len(ciphertext) == 0 {
		return nil, fmt.Errorf(
			"the record carries no ciphertext bytes: re-run `fod receive` with this build, which records the bytes it checked")
	}
	digest := sha256.Sum256(ciphertext)
	if recorded := strings.ToLower(strings.TrimPrefix(opened.CiphertextSha256, "0x")); recorded != hex.EncodeToString(digest[:]) {
		return nil, fmt.Errorf("the record's ciphertext does not hash to the digest it names")
	}
	if err := verifyOpenedDelivery(&opened); err != nil {
		return nil, err
	}
	return &opened, nil
}

func validatePreparedConfirm(c *client, prepared *preparedAction, identity *ecdsa.PrivateKey) error {
	prepared.confirmValidated = false
	opened := prepared.VerifiedDelivery
	if err := verifyOpenedDelivery(opened); err != nil {
		return err
	}
	if prepared.Contract != opened.Contract || prepared.OfferID != opened.OrderID || prepared.Amount != "0" || prepared.CiphertextSha256 != opened.CiphertextSha256 {
		return fmt.Errorf("confirmation context differs from the usable-content receipt")
	}
	expected, err := confirmCalldata(opened.OrderID, opened.CiphertextHex)
	if err != nil || expected != prepared.Calldata {
		return fmt.Errorf("confirmation bytes differ from the signed receipt")
	}
	raw, err := readRegistryValue(c, prepared.Contract, fmt.Sprintf("%s%d", marketPoolOrderPrefix, opened.OrderID))
	if err != nil {
		return err
	}
	order, err := decodePoolOrderFull(opened.OrderID, raw)
	if err != nil {
		return err
	}
	buyer, err := nodekeyAddressOf(identity)
	if err != nil || buyer != order.Buyer || opened.Buyer != order.Buyer || opened.KeyCommitment != order.Bundle ||
		opened.OrderNonce != order.Nonce || opened.DeliveryKey != order.DeliveryKey || (order.State != "funded" && order.State != "delivered") {
		return fmt.Errorf("the receipt does not belong to this buyer's currently open canonical order")
	}
	listing, err := registryPoolBundle(c, prepared.Contract, order.Bundle)
	if err != nil {
		return err
	}
	if opened.UsableFlips != int(listing.FlipCount) || strings.TrimPrefix(opened.Author, "0x") != listing.Author {
		return fmt.Errorf("the receipt does not cover this registered author's full purchased inventory")
	}
	prepared.confirmValidated = true
	return nil
}

// confirmRequestArguments is the interface's confirmation request (`SPEC.md`
// 4.5): the bundle is recorded for the reader, and the order id is what the
// escrow parses.
type confirmRequestArguments struct {
	BundleID string `json:"bundleId"`
	OrderID  uint64 `json:"orderId"`
	// CiphertextSha256 is the digest of the delivered bytes the buyer checked,
	// which the CLI checks the bytes below against (`SPEC.md` 7.1).
	CiphertextSha256 string `json:"ciphertextSha256"`
	// CiphertextHex is those exact bytes: the confirmation brings them with the
	// call, so a replaced pending payload cannot strand the payment (`AH-10`).
	CiphertextHex string `json:"ciphertextHex"`
}

func loadConfirmRequest(path string) (*actionRequest, *confirmRequestArguments, error) {
	request, err := readActionRequest(path)
	if err != nil {
		return nil, nil, err
	}
	if request.Operation != operationConfirm {
		return nil, nil, fmt.Errorf("unsupported operation %q", request.Operation)
	}
	var arguments confirmRequestArguments
	if err := decodeStrict(request.Arguments, &arguments); err != nil {
		return nil, nil, fmt.Errorf("read request arguments: %w", err)
	}
	if !isHex(strings.ToLower(strings.TrimPrefix(arguments.BundleID, "0x")), poolBundleBytes) {
		return nil, nil, fmt.Errorf("the request names no valid bundle")
	}
	if arguments.OrderID == 0 {
		return nil, nil, fmt.Errorf("the request names no pool order")
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(arguments.CiphertextSha256), "0x")); err != nil || arguments.CiphertextSha256 == "" {
		return nil, nil, fmt.Errorf("the request names no ciphertext digest")
	}
	ciphertext, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(arguments.CiphertextHex), "0x"))
	if err != nil || len(ciphertext) == 0 {
		return nil, nil, fmt.Errorf("the request names no ciphertext bytes: re-run `fod receive` and use its record")
	}
	digest := sha256.Sum256(ciphertext)
	if recorded := strings.ToLower(strings.TrimPrefix(arguments.CiphertextSha256, "0x")); recorded != hex.EncodeToString(digest[:]) {
		return nil, nil, fmt.Errorf("the request's ciphertext does not hash to the digest it names")
	}
	return request, &arguments, nil
}

// runPrepareConfirmRequest is the interface-driven form of `prepare-confirm`.
func runPrepareConfirmRequest(requestPath, deliveryPath, preparedPath string) {
	request, arguments, err := loadConfirmRequest(requestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "confirm request refused: %v\n", err)
		os.Exit(1)
	}
	opened, err := readUsableDeliveryRecord(deliveryPath, request.Contract, arguments.OrderID)
	if err != nil || opened.CiphertextHex != strings.TrimPrefix(strings.ToLower(arguments.CiphertextHex), "0x") {
		fmt.Fprintf(os.Stderr, "confirm refused: request must match the usable delivery record (%v)\n", err)
		os.Exit(1)
	}
	prepared, err := prepareConfirmAction(request.Contract, request.Network, arguments.OrderID, arguments.CiphertextHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "confirm refused: %v\n", err)
		os.Exit(1)
	}
	prepared.Commitment = strings.ToLower(strings.TrimPrefix(arguments.BundleID, "0x"))
	prepared.VerifiedDelivery = opened
	if prepared.VerifiedDelivery.KeyCommitment != prepared.Commitment {
		fmt.Fprintln(os.Stderr, "confirm refused: receipt does not match the requested bundle")
		os.Exit(1)
	}
	if err := writePrivateJSON(preparedPath, prepared); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
		os.Exit(1)
	}
	fmt.Printf("order       %d\n", arguments.OrderID)
	fmt.Printf("bundle      %s\n", prepared.Commitment)
	fmt.Printf("ciphertext  sha256 %s (%d bytes this buyer checked)\n", arguments.CiphertextSha256, len(arguments.CiphertextHex)/2)
	fmt.Printf("calldata    %s (%d bytes: the order id, then those bytes)\n", prepared.Calldata, len(prepared.Calldata)/2)
	fmt.Printf("prepared    %s\n", preparedPath)
	fmt.Println("not submitted: pass this file to `fod submit` to sign it; the buyer's nodekey sends it")
}
