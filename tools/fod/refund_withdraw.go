package main

// The two ways value comes back out of the pool, in the file-based workflow of
// `SPEC.md` 4.3: a refund for a funded order that was never delivered, and a
// withdrawal of the contract credit that refunds and sales leave behind.
//
// Both write a prepared action for `fod submit`, and both have to match the
// argument encoding the contract actually parses - which is not uniform. A
// refund reads the order id as decimal text; a withdrawal takes no argument at
// all, because the credit it pays out is the caller's own.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	operationRefund            = "refund"
	operationWithdrawal        = "withdraw"
	operationReserveWithdrawal = "withdrawReserve"

	preparedPoolRefundFormat        = "fod.prepared-refund-order"
	preparedWithdrawalFormat        = "fod.prepared-withdrawal"
	preparedReserveWithdrawalFormat = "fod.prepared-withdraw-reserve"
)

// refundArguments is the typed argument set of a `refund` request.
type refundArguments struct {
	OrderID uint64 `json:"orderId"`
}

// PreparedClaim is the prepared withdrawal action. It names no id: the credit
// is whatever the caller's address holds.
type PreparedClaim struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	Operation  string `json:"operation"`
	Network    string `json:"network"`
	Contract   string `json:"contract"`
	Amount     string `json:"amount"`
	Calldata   string `json:"calldata"`
	PreparedAt string `json:"preparedAt"`
	Submitted  bool   `json:"submitted"`
}

// loadRefundRequest reads a pool `refund` request and its typed arguments.
func loadRefundRequest(path string) (*actionRequest, *refundArguments, error) {
	request, err := readActionRequest(path)
	if err != nil {
		return nil, nil, err
	}
	if request.Operation != operationRefund {
		return nil, nil, fmt.Errorf("unsupported operation %q", request.Operation)
	}
	var arguments refundArguments
	if err := decodeStrict(request.Arguments, &arguments); err != nil {
		return nil, nil, fmt.Errorf("read request arguments: %w", err)
	}
	if arguments.OrderID == 0 {
		return nil, nil, errors.New("the request names no pool order")
	}
	return request, &arguments, nil
}

// loadWithdrawalRequest reads a `withdraw` request. It names no arguments by
// design: the credit is whatever the caller's address holds.
func loadWithdrawalRequest(path string) (*actionRequest, error) {
	request, err := readActionRequest(path)
	if err != nil {
		return nil, err
	}
	if request.Operation != operationWithdrawal {
		return nil, fmt.Errorf("unsupported operation %q", request.Operation)
	}
	var arguments struct{}
	if err := decodeStrict(request.Arguments, &arguments); err != nil {
		return nil, fmt.Errorf("read request arguments: %w", err)
	}
	return request, nil
}

// PrepareWithdrawal writes the call for `withdraw`, which takes no argument.
func PrepareWithdrawal(request *actionRequest) (*PreparedClaim, error) {
	return &PreparedClaim{
		Format:     preparedWithdrawalFormat,
		Version:    actionRequestVersion,
		Operation:  operationWithdrawal,
		Network:    request.Network,
		Contract:   strings.ToLower(request.Contract),
		Amount:     "0",
		Calldata:   "",
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
		Submitted:  false,
	}, nil
}

// loadReserveWithdrawalRequest reads a `withdrawReserve` request. It names no
// arguments: the reserve credit is whatever the caller's address donated and
// has not withdrawn yet (`AH-15`).
func loadReserveWithdrawalRequest(path string) (*actionRequest, error) {
	request, err := readActionRequest(path)
	if err != nil {
		return nil, err
	}
	if request.Operation != operationReserveWithdrawal {
		return nil, fmt.Errorf("unsupported operation %q", request.Operation)
	}
	var arguments struct{}
	if err := decodeStrict(request.Arguments, &arguments); err != nil {
		return nil, fmt.Errorf("read request arguments: %w", err)
	}
	return request, nil
}

// PrepareReserveWithdrawal writes the call for `withdrawReserve`, which takes
// no argument either: the escrow pays the caller's own reserve credit.
func PrepareReserveWithdrawal(request *actionRequest) (*PreparedClaim, error) {
	return &PreparedClaim{
		Format:     preparedReserveWithdrawalFormat,
		Version:    actionRequestVersion,
		Operation:  operationReserveWithdrawal,
		Network:    request.Network,
		Contract:   strings.ToLower(request.Contract),
		Amount:     "0",
		Calldata:   "",
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
		Submitted:  false,
	}, nil
}

// PreparePoolRefund writes the pool's `refund` call: the order id as decimal
// text, the same encoding the escrow's `parse_u64` reads.
func PreparePoolRefund(request *actionRequest, arguments *refundArguments) *preparedAction {
	order := strconv.FormatUint(arguments.OrderID, 10)
	return &preparedAction{
		Format:     preparedPoolRefundFormat,
		Version:    actionRequestVersion,
		Operation:  operationRefund,
		Network:    request.Network,
		Contract:   strings.ToLower(strings.TrimPrefix(request.Contract, "0x")),
		OfferID:    arguments.OrderID,
		Amount:     "0",
		Calldata:   hex.EncodeToString([]byte(order)),
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func runRefund(requestPath, preparedPath string) {
	request, arguments, err := loadRefundRequest(requestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "action request refused: %v\n", err)
		os.Exit(1)
	}
	prepared := PreparePoolRefund(request, arguments)
	fmt.Printf("operation   %s on %s\n", prepared.Operation, prepared.Network)
	fmt.Printf("contract    %s\n", prepared.Contract)
	fmt.Printf("pool order  %d\n", prepared.OfferID)
	fmt.Printf("calldata    %s (the order id as decimal text, which the escrow parses)\n", prepared.Calldata)
	fmt.Println("note        the escrow refunds only after the pool order's deadline plus grace, and counts a strike")
	if preparedPath != "" {
		if err := writeJSON(preparedPath, prepared); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
			os.Exit(1)
		}
		fmt.Printf("prepared    %s\n", preparedPath)
	}
	notSubmitted(preparedPath, "")
}

func runWithdraw(requestPath, preparedPath string) {
	request, err := loadWithdrawalRequest(requestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "action request refused: %v\n", err)
		os.Exit(1)
	}
	prepared, err := PrepareWithdrawal(request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("operation   %s on %s\n", prepared.Operation, prepared.Network)
	fmt.Printf("contract    %s\n", prepared.Contract)
	fmt.Println("amount      the caller's whole credit, which only the escrow knows")
	fmt.Println("note        the escrow pays the credit of the address that sends this transaction")
	if preparedPath != "" {
		if err := writeJSON(preparedPath, prepared); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
			os.Exit(1)
		}
		fmt.Printf("prepared    %s\n", preparedPath)
	}
	notSubmitted(preparedPath, "")
}

func runReserveWithdraw(requestPath, preparedPath string) {
	request, err := loadReserveWithdrawalRequest(requestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "action request refused: %v\n", err)
		os.Exit(1)
	}
	prepared, err := PrepareReserveWithdrawal(request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("operation   %s on %s\n", prepared.Operation, prepared.Network)
	fmt.Printf("contract    %s\n", prepared.Contract)
	fmt.Println("amount      the caller's own reserve contribution, which only the escrow knows")
	fmt.Println("note        the escrow pays a donor's reserve credit and leaves buyer escrow and host credits untouched")
	if preparedPath != "" {
		if err := writeJSON(preparedPath, prepared); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
			os.Exit(1)
		}
		fmt.Printf("prepared    %s\n", preparedPath)
	}
	notSubmitted(preparedPath, "")
}
