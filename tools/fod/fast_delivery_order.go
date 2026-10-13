package main

// Public funded-order context for the seller's local, fast point proof. This
// export contains no seller witness or signing key.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const operationFulfillFast = "fulfillFast"

type fastDeliveryOrderInput struct {
	Format      string `json:"format"`
	Version     int    `json:"version"`
	Network     string `json:"network"`
	Contract    string `json:"contract"`
	Bundle      string `json:"bundle"`
	Journal     string `json:"journal"`
	OrderID     uint64 `json:"orderId"`
	Nonce       uint64 `json:"nonce"`
	Buyer       string `json:"buyer"`
	DeliveryKey string `json:"deliveryKey"`
	Author      string `json:"author"`
}

func prepareFastDeliveryOrder(c *client, contract string, orderID uint64) (*fastDeliveryOrderInput, error) {
	contract = strings.ToLower(strings.TrimPrefix(contract, "0x"))
	if !isHex(contract, addressBytes) || orderID == 0 {
		return nil, errors.New("invalid fast order contract or ID")
	}
	raw, err := readRegistryValue(c, contract, fmt.Sprintf("%s%d", marketPoolOrderPrefix, orderID))
	if err != nil {
		return nil, err
	}
	order, err := decodePoolOrderFull(orderID, raw)
	if err != nil {
		return nil, err
	}
	if order.State != "funded" {
		return nil, errors.New("fast order is not awaiting delivery")
	}
	listing, err := registryPoolBundle(c, contract, order.Bundle)
	if err != nil {
		return nil, err
	}
	fast, err := registryFastBundle(c, contract, order.Bundle)
	if err != nil {
		return nil, err
	}
	certificate, err := registryFastCertificate(c, contract, order.Bundle)
	if err != nil {
		return nil, err
	}
	if err := validateFastChainCertificate(contract, order.Bundle, listing, fast, certificate, fastContentImageIDHex); err != nil {
		return nil, err
	}
	var head rpcBlock
	if err := c.call("bcn_lastBlock", []any{}, &head); err != nil {
		return nil, err
	}
	if head.Height < order.FundedAt || head.Height > lastDisclosureHead(order.Deadline) {
		return nil, errors.New("no inclusion block remains before the fast delivery deadline")
	}
	return &fastDeliveryOrderInput{
		Format:      "fod.fast-delivery-order",
		Version:     1,
		Network:     NetworkNamePinned,
		Contract:    contract,
		Bundle:      order.Bundle,
		Journal:     hex.EncodeToString(certificate[:fastContentJournalBytes]),
		OrderID:     orderID,
		Nonce:       order.Nonce,
		Buyer:       order.Buyer,
		DeliveryKey: order.DeliveryKey,
		Author:      listing.Author,
	}, nil
}
