package main

import (
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"

	"github.com/shopspring/decimal"
)

const directBlockGasCap = 5_120_000

type directEstimate struct {
	Success     bool   `json:"success"`
	GasUsed     uint64 `json:"gasUsed"`
	Error       string `json:"error"`
	TxFee       string `json:"txFee"`
	MinGasLimit uint64 `json:"minGasLimit"`
}

// The node runs the exact contract method against current state without
// broadcasting. Fallback calldata contains the keys, so its simulation must
// stay on a loopback RPC controlled by the seller.
func estimateDirectCall(c *client, p *preparedAction, sender string, maxFee, feePerGas *big.Int) (uint64, *big.Int, error) {
	if !isDirectFormat(p.Format) {
		return 0, nil, errors.New("not a direct call")
	}
	if p.Format == preparedDirectFallbackFormat && !directLocalRPC(c.url) {
		return 0, nil, errors.New("direct fallback requires a loopback RPC to keep unreleased keys private")
	}
	return estimatePreparedDirectCall(c, p, sender, maxFee, feePerGas)
}

func estimateDirectV6PoolCall(c *client, p *preparedAction, sender string, maxFee, feePerGas *big.Int) (uint64, *big.Int, error) {
	if p == nil || p.DirectVersion != 6 || p.Network != NetworkNamePinned || p.Amount != "0" ||
		p.Author != sender || !isHex(p.Contract, addressBytes) || !isHex(p.DirectOfferID, 32) ||
		!(p.Format == preparedDirectV6PoolQualifyFormat && p.Operation == "qualify" ||
			p.Format == preparedDirectV6PoolProveFormat && p.Operation == "proveBundle") {
		return 0, nil, errors.New("invalid v6 pool estimate context")
	}
	if !directLocalRPC(c.url) {
		return 0, nil, errors.New("v6 pool estimate needs a loopback node RPC")
	}
	return estimatePreparedDirectCall(c, p, sender, maxFee, feePerGas)
}

func estimatePreparedDirectCall(c *client, p *preparedAction, sender string, maxFee, feePerGas *big.Int) (uint64, *big.Int, error) {
	amount, ok := new(big.Int).SetString(p.Amount, 10)
	if !ok || amount.Sign() < 0 {
		return 0, nil, errors.New("invalid direct call amount")
	}
	var receipt directEstimate
	err := c.call("contract_estimateCall", []any{map[string]any{
		"from": "0x" + sender, "contract": "0x" + p.Contract,
		"method": p.Operation, "amount": decimal.NewFromBigInt(amount, -18).String(),
		"maxFee": decimal.NewFromBigInt(maxFee, -18).String(),
		"args":   []any{map[string]any{"index": 0, "format": "hex", "value": "0x" + p.Calldata}},
	}}, &receipt)
	if err != nil {
		return 0, nil, fmt.Errorf("direct call simulation failed: %w", err)
	}
	if !receipt.Success || receipt.GasUsed == 0 {
		return 0, nil, fmt.Errorf("direct call simulation refused the current contract state: %s", receipt.Error)
	}
	// Keep room for signed-transaction framing and small state/fee changes.
	gas := receipt.GasUsed
	if isBondedV8Format(p.Format) && receipt.MinGasLimit > gas {
		gas = receipt.MinGasLimit
	}
	budget := gas + gas/10 + 1_000
	if budget < receipt.GasUsed || budget > directBlockGasCap {
		return 0, nil, errors.New("direct call exceeds the chain block gas cap with margin")
	}
	rate := feePerGas
	if rate == nil {
		var chainFee big.Int
		if err := c.call("bcn_feePerGas", []any{}, &chainFee); err != nil {
			return 0, nil, fmt.Errorf("read direct-call chain fee: %w", err)
		}
		rate = &chainFee
	}
	if rate.Sign() <= 0 {
		return 0, nil, errors.New("direct-call fee rate is unavailable")
	}
	required := new(big.Int).Mul(new(big.Int).SetUint64(budget), rate)
	if isBondedV8Format(p.Format) {
		// The VM gas does not include transaction-size fees. Preserve a current
		// per-action estimate instead of charging every action a proof-sized cap.
		txFee, err := v8IDNAAtoms(receipt.TxFee)
		if err != nil {
			return 0, nil, errors.New("v8 simulation omitted a valid transaction fee")
		}
		margin := new(big.Int).Add(txFee, big.NewInt(9))
		margin.Quo(margin, big.NewInt(10))
		required.Add(required, txFee).Add(required, margin)
		p.EstimatedMaxFee = required.String()
	}
	if maxFee == nil || maxFee.Cmp(required) < 0 {
		return budget, rate, fmt.Errorf("direct call needs max fee at least %s base units for the current simulation", required)
	}
	if cap := maxMaxFeeFor(rate); maxFee.Cmp(cap) > 0 {
		return budget, rate, fmt.Errorf("direct max fee exceeds node cap %s", cap)
	}
	return budget, rate, nil
}

func directLocalRPC(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Port() == "" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}
