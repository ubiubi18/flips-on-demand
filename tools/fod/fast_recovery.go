package main

// Export the included fast delivery and the buyer's prefetched copies for
// local recovery. The buyer's secret key never enters this transcript.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/idena-network/idena-go/crypto"
)

type fastRecoveryInput struct {
	Format      string             `json:"format"`
	Version     int                `json:"version"`
	Contract    string             `json:"contract"`
	Bundle      string             `json:"bundle"`
	Journal     string             `json:"journal"`
	OrderID     uint64             `json:"orderId"`
	Nonce       uint64             `json:"nonce"`
	Buyer       string             `json:"buyer"`
	DeliveryKey string             `json:"deliveryKey"`
	Delivery    string             `json:"delivery"`
	Copies      []retainedFastCopy `json:"copies"`
}

func prepareFastRecovery(c *client, contract string, orderID uint64, prepared *preparedAction, keyPath string) (*fastRecoveryInput, error) {
	contract = strings.ToLower(strings.TrimPrefix(contract, "0x"))
	if !isHex(contract, addressBytes) || prepared == nil || prepared.Fast == nil || prepared.Contract != contract {
		return nil, errors.New("purchase does not name this fast contract")
	}
	orderRaw, err := readRegistryValue(c, contract, fmt.Sprintf("%s%d", marketPoolOrderPrefix, orderID))
	if err != nil {
		return nil, err
	}
	order, err := decodePoolOrderFull(orderID, orderRaw)
	if err != nil {
		return nil, err
	}
	if order.State != "settled" || order.Bundle != prepared.Bundle || order.Nonce != prepared.Nonce ||
		order.Buyer != prepared.Buyer || order.DeliveryKey != prepared.DeliveryKey || order.Paid != prepared.Amount {
		return nil, errors.New("the settled order does not match the retained fast purchase")
	}
	key, err := loadDeliveryKey(keyPath)
	if err != nil {
		return nil, err
	}
	if hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey)) != order.DeliveryKey {
		return nil, errors.New("this delivery key does not belong to the settled order")
	}
	listing, err := registryPoolBundle(c, contract, order.Bundle)
	if err != nil {
		return nil, err
	}
	if err := validateFastBuyEvidence(c, prepared, listing); err != nil {
		return nil, err
	}
	certificate, err := registryFastCertificate(c, contract, order.Bundle)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(certificate)
	if hex.EncodeToString(digest[:]) != prepared.Fast.CertificateSHA256 {
		return nil, errors.New("fast certificate changed during recovery preparation")
	}
	delivery, err := readRegistryValue(c, contract, fmt.Sprintf("pfdelivery/%d", orderID))
	if err != nil {
		return nil, err
	}
	if len(delivery) != 196 {
		return nil, errors.New("included fast delivery has the wrong length")
	}
	return &fastRecoveryInput{
		Format:      "fod.fast-recovery-input",
		Version:     1,
		Contract:    contract,
		Bundle:      order.Bundle,
		Journal:     hex.EncodeToString(certificate[:fastContentJournalBytes]),
		OrderID:     orderID,
		Nonce:       order.Nonce,
		Buyer:       order.Buyer,
		DeliveryKey: order.DeliveryKey,
		Delivery:    hex.EncodeToString(delivery),
		Copies:      prepared.Fast.Copies,
	}, nil
}
