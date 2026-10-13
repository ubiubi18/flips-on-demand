package main

// The buyer's automatic recovery: follow one pool order and, as soon as the
// chain passes its refund block without settlement, submit the permissionless
// refund with the given nodekey. The refund pays the order's buyer no matter
// who sends it, so the key can be the buyer's own or any funded relayer; no
// operator, project service or browser tab is involved, and the plain
// `refund` + `submit` pair stays as the manual fallback.

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

// runWatch blocks until the order settles, refunds, or reaches its refund
// block; in the last case it submits the refund and returns.
func runWatch(c *client, orderID uint64, contractHex, network, nodeKeyPath string, intervalSeconds int, rpcURL, apiKey string) error {
	if orderID == 0 || contractHex == "" || nodeKeyPath == "" {
		return fmt.Errorf("watch mode needs -order N -contract ADDRESS -nodekey FILE")
	}
	identity, err := crypto.LoadECDSA(nodeKeyPath)
	if err != nil {
		return fmt.Errorf("load nodekey: %w", err)
	}
	address, err := nodekeyAddressOf(identity)
	if err != nil {
		return err
	}
	interval := time.Duration(intervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	contract := strings.TrimPrefix(strings.ToLower(contractHex), "0x")
	addressHex := "0x" + strings.TrimPrefix(strings.ToLower(address), "0x")
	for {
		var head struct {
			Height uint64 `json:"height"`
		}
		if err := c.call("bcn_lastBlock", []any{}, &head); err != nil {
			return fmt.Errorf("read the head block: %w", err)
		}
		raw, err := readRegistryValue(c, contract, fmt.Sprintf("%s%d", marketPoolOrderPrefix, orderID))
		if err != nil {
			return fmt.Errorf("read order %d: %w", orderID, err)
		}
		order, err := decodePoolOrderRecord(orderID, raw)
		if err != nil {
			return err
		}
		fmt.Printf("order %d  state %-9s head %d  refund from %d  watch key %s\n",
			orderID, order.State, head.Height, order.RefundAt, addressHex)
		if order.State == "settled" || order.State == "refunded" {
			return nil
		}
		if head.Height >= order.RefundAt {
			return submitWatchRefund(c, orderID, contract, network, nodeKeyPath, address, rpcURL, apiKey)
		}
		time.Sleep(interval)
	}
}

// submitWatchRefund builds the refund action and signs it with the watch key.
func submitWatchRefund(c *client, orderID uint64, contract, network, nodeKeyPath, address, rpcURL, apiKey string) error {
	prepared := PreparePoolRefund(
		&actionRequest{Network: network, Contract: contract},
		&refundArguments{OrderID: orderID},
	)
	// The node's RPC takes the address in its 0x form; the key census writes it
	// without the prefix, and a bare string is refused before the nonce is read.
	accountAddress := "0x" + strings.TrimPrefix(strings.ToLower(address), "0x")
	path, cleanup, err := writeWatchAction(orderID, prepared)
	if err != nil {
		return err
	}
	defer cleanup()
	var account struct {
		Nonce uint32 `json:"nonce"`
	}
	if err := c.call("dna_getBalance", []any{accountAddress}, &account); err != nil {
		return fmt.Errorf("read the account nonce: %w", err)
	}
	var epoch struct {
		Epoch uint16 `json:"epoch"`
	}
	if err := c.call("dna_epoch", []any{}, &epoch); err != nil {
		return fmt.Errorf("read the epoch: %w", err)
	}
	feePerGas, err := readFeePerGas(c)
	if err != nil {
		return fmt.Errorf("read the fee rate: %w", err)
	}
	gas, ok := measuredGasFor(operationRefund)
	if !ok {
		return fmt.Errorf("refund has no measured gas")
	}
	// The fee table carries the size it measured; the signed envelope is the RLP
	// of the nonce, epoch, amount and max fee, so it can be a byte or two larger
	// and the node charges the size it is given. Ask for the calldata's own bytes
	// plus that headroom; `fod submit` still refuses a max fee the signed size
	// does not cover, so headroom cannot overpay, it only stops the client from
	// refusing itself.
	headroom := uint64(len(prepared.Calldata)/2) + 2
	maxFee := requiredMaxFeeFor(gas.WasmGas, gas.TxBytes+headroom, feePerGas)
	fmt.Printf("submitting the refund: nonce %d, epoch %d, %s per gas, max fee %s\n",
		account.Nonce+1, epoch.Epoch, feePerGas, maxFee)
	runSubmit(path, nodeKeyPath, account.Nonce+1, epoch.Epoch, maxFee, big.NewInt(0), "", true, rpcURL, apiKey, feePerGas, "", "")
	return nil
}

// writeWatchAction writes the prepared refund into a private work directory and
// returns the file path and a cleanup function. The action is signed with the
// watch key, so it must not sit at a predictable path in a world-writable
// directory: another local user could pre-create or replace it and have this
// process sign their call. MkdirTemp creates the directory 0700, and the file is
// written 0600 by the private writer.
func writeWatchAction(orderID uint64, prepared *preparedAction) (string, func(), error) {
	dir, err := os.MkdirTemp("", "fod-watch-")
	if err != nil {
		return "", nil, fmt.Errorf("create the watch work directory: %w", err)
	}
	cleanup := func() { os.RemoveAll(dir) }
	path := filepath.Join(dir, fmt.Sprintf("refund-%d.json", orderID))
	if err := writePrivateJSON(path, prepared); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write %s: %w", path, err)
	}
	return path, cleanup, nil
}
