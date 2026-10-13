package main

// The pool's supply side of `SPEC.md` 6.4 steps 7-8: the holder sells one unit
// at the pool's current quote (`sellUnit`) or gives one (`donateUnit`). A sale
// presents the same sealed registration receipt that registered the bundle, so
// this reads the receipt, checks it against the bundle the chain already holds,
// and builds the call. A gift carries only the commitment.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	operationSellUnit        = "sellUnit"
	operationDonateUnit      = "donateUnit"
	preparedSellUnitFormat   = "fod.prepared-sell-unit"
	preparedDonateUnitFormat = "fod.prepared-donate-unit"
)

// poolListArguments is the argument set of a pool sell or give request. There
// is no price: the pool's quote is the price, and no person sets it.
type poolListArguments struct {
	Commitment string `json:"commitment"`
}

// loadPoolListRequest reads a `sellUnit` or `donateUnit` request.
func loadPoolListRequest(path string) (*actionRequest, *poolListArguments, error) {
	request, err := readActionRequest(path)
	if err != nil {
		return nil, nil, err
	}
	switch request.Operation {
	case operationSellUnit, operationDonateUnit:
	default:
		return nil, nil, fmt.Errorf("unsupported pool operation %q", request.Operation)
	}
	var arguments poolListArguments
	if err := decodeStrict(request.Arguments, &arguments); err != nil {
		return nil, nil, fmt.Errorf("read request arguments: %w", err)
	}
	if !isHex(strings.ToLower(strings.TrimPrefix(arguments.Commitment, "0x")), poolBundleBytes) {
		return nil, nil, errors.New("the request names no valid key commitment")
	}
	return request, &arguments, nil
}

// PreparePoolList builds the call. A sale is commitment (32) || journal (178)
// || seal (256), with the receipt validated exactly as registration validates
// it; a gift is only the commitment. bundleAuthor is the author the pool record
// names, which the receipt has to match.
func PreparePoolList(request *actionRequest, arguments *poolListArguments, proofPath string, artefact *registrationArtefact, bundleAuthor string) (*preparedAction, error) {
	commitment := strings.ToLower(strings.TrimPrefix(arguments.Commitment, "0x"))
	commitmentBytes, err := hex.DecodeString(commitment)
	if err != nil || len(commitmentBytes) != poolBundleBytes {
		return nil, errors.New("the request names no valid key commitment")
	}

	operation := operationDonateUnit
	format := preparedDonateUnitFormat
	calldata := commitmentBytes
	if request.Operation == operationSellUnit {
		if artefact == nil {
			return nil, errors.New("a sale needs the sealed registration receipt: pass -proof FILE")
		}
		receipt, err := AssemblePoolRegistration(request.Network, request.Contract, proofPath, artefact)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(receipt.KeyCommitment, commitment) {
			return nil, fmt.Errorf("the receipt commits to bundle %s, and the request names %s", receipt.KeyCommitment, commitment)
		}
		if !strings.EqualFold(receipt.Author, bundleAuthor) {
			return nil, fmt.Errorf("the receipt names author %s, and the pool bundle belongs to %s", receipt.Author, bundleAuthor)
		}
		receiptBytes, err := hex.DecodeString(receipt.Calldata)
		if err != nil {
			return nil, err
		}
		calldata = append(calldata, receiptBytes...)
		operation = operationSellUnit
		format = preparedSellUnitFormat
	} else if artefact != nil {
		return nil, errors.New("a gift carries no registration receipt")
	}

	return &preparedAction{
		Format:     format,
		Version:    actionRequestVersion,
		Operation:  operation,
		Network:    request.Network,
		Contract:   strings.ToLower(strings.TrimPrefix(request.Contract, "0x")),
		Amount:     "0",
		Calldata:   hex.EncodeToString(calldata),
		Commitment: commitment,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// runPreparePoolList reads the pool record, validates a sale's receipt against
// it, and writes the prepared call.
func runPreparePoolList(c *client, requestPath, proofPath, preparedPath string) {
	request, arguments, err := loadPoolListRequest(requestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "action request refused: %v\n", err)
		os.Exit(1)
	}
	contract := strings.ToLower(strings.TrimPrefix(request.Contract, "0x"))
	commitment := strings.ToLower(strings.TrimPrefix(arguments.Commitment, "0x"))
	listing, err := registryPoolBundle(c, contract, commitment)
	if err != nil {
		fmt.Fprintf(os.Stderr, "the pool does not hold bundle %s: %v\n", commitment, err)
		os.Exit(1)
	}
	var artefact *registrationArtefact
	if request.Operation == operationSellUnit {
		if proofPath == "" {
			fmt.Fprintln(os.Stderr, "a sellUnit request needs -proof FILE (the sealed registration receipt)")
			os.Exit(2)
		}
		artefact, err = readRegistrationArtefact(proofPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}
	prepared, err := PreparePoolList(request, arguments, proofPath, artefact, listing.Author)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("operation   %s on %s\n", prepared.Operation, prepared.Network)
	fmt.Printf("contract    %s\n", prepared.Contract)
	fmt.Printf("bundle      %s\n", prepared.Commitment)
	fmt.Printf("pool        %s, reserve %s, %d units\n", listing.Author, listing.Reserve, listing.Units)
	if prepared.Operation == operationSellUnit {
		fmt.Printf("receipt     %s\n", proofPath)
	} else {
		fmt.Println("gift        no payment; when a buyer's order settles, the escrow pays the author")
	}
	fmt.Printf("calldata    %d bytes\n", len(prepared.Calldata)/2)
	if preparedPath != "" {
		if err := writeJSON(preparedPath, prepared); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
			os.Exit(1)
		}
		fmt.Printf("prepared    %s\n", preparedPath)
	}
	notSubmitted(preparedPath, "")
}
