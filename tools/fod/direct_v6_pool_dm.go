package main

// A v6 private-delivery envelope contains only an already-encrypted payload.
// Its context must come from independently read offer and order chain records.

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/idena-network/idena-go/crypto"
)

const directV6PoolDMVersion = 6

type DirectV6PoolDMOffer struct {
	ID                  string
	Author              string
	Epoch               uint16
	Covered             uint32
	InventoryCommitment string
	KeysCommitment      string
	Payout              string
	SellerBond          string
	NextNonce           uint64
	OpenOrderID         uint64
	State               string
	PublicSeed          bool
}

type DirectV6PoolDMOrder struct {
	ID           uint64
	OfferID      string
	Buyer        string
	DeliveryKey  string
	Paid         string
	Bond         string
	SellerPayout string
	FundedAt     uint64
	Nonce        uint64
	State        string
}

type DirectV6PoolDMContext struct {
	Network  string
	Contract string
	Offer    DirectV6PoolDMOffer
	Order    DirectV6PoolDMOrder
}

// NewDirectV6PoolDMEnvelope packages ciphertext without encrypting or sending it.
func NewDirectV6PoolDMEnvelope(context DirectV6PoolDMContext, ciphertext []byte) (*DirectDMEnvelope, error) {
	envelope := &DirectDMEnvelope{
		Format:      directDMFormat,
		Version:     directV6PoolDMVersion,
		Network:     context.Network,
		Contract:    context.Contract,
		Author:      context.Offer.Author,
		OfferID:     context.Offer.ID,
		OrderID:     context.Order.ID,
		OrderNonce:  context.Order.Nonce,
		Buyer:       context.Order.Buyer,
		DeliveryKey: context.Order.DeliveryKey,
		Ciphertext:  hex.EncodeToString(ciphertext),
	}
	if err := ValidateDirectV6PoolDMEnvelope(envelope, context); err != nil {
		return nil, err
	}
	return envelope, nil
}

// EncodeDirectV6PoolDMEnvelope returns compact, bounded JSON after validation.
func EncodeDirectV6PoolDMEnvelope(envelope *DirectDMEnvelope, context DirectV6PoolDMContext) (string, error) {
	if err := ValidateDirectV6PoolDMEnvelope(envelope, context); err != nil {
		return "", err
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	if len(raw) > directDMMaxTextBytes {
		return "", errors.New("v6 pool delivery message is too large")
	}
	return string(raw), nil
}

// ParseDirectV6PoolDMEnvelope rejects duplicate, unknown, and missing fields
// before binding the message to independently supplied chain state.
func ParseDirectV6PoolDMEnvelope(message string, context DirectV6PoolDMContext) (*DirectDMEnvelope, error) {
	if len(message) == 0 || len(message) > directDMMaxTextBytes {
		return nil, errors.New("v6 pool delivery message has invalid size")
	}
	if err := directDMFields([]byte(message)); err != nil {
		return nil, err
	}
	var envelope DirectDMEnvelope
	if err := decodeStrict([]byte(message), &envelope); err != nil {
		return nil, fmt.Errorf("decode v6 pool delivery: %w", err)
	}
	if err := ValidateDirectV6PoolDMEnvelope(&envelope, context); err != nil {
		return nil, err
	}
	return &envelope, nil
}

// ValidateDirectV6PoolDMEnvelope checks the sale identity and ECIES wire shape.
// It does not authenticate a social sender or prove successful decryption.
func ValidateDirectV6PoolDMEnvelope(envelope *DirectDMEnvelope, context DirectV6PoolDMContext) error {
	if envelope == nil || envelope.Format != directDMFormat || envelope.Version != directV6PoolDMVersion {
		return errors.New("unsupported v6 pool delivery envelope")
	}
	if context.Network != NetworkNamePinned || !directDMHex(context.Contract, addressBytes) ||
		envelope.Network != context.Network || envelope.Contract != context.Contract {
		return errors.New("v6 pool delivery network or contract differs from canonical context")
	}
	offer := context.Offer
	order := context.Order
	if !directDMHex(offer.Author, addressBytes) || !directDMHex(offer.ID, 32) {
		return errors.New("invalid canonical v6 pool offer identity")
	}
	authorBytes, _ := hex.DecodeString(offer.Author)
	var author [addressBytes]byte
	copy(author[:], authorBytes)
	derivedID := poolV6OfferID(author, offer.Epoch)
	if offer.ID != hex.EncodeToString(derivedID[:]) ||
		envelope.Author != offer.Author || envelope.OfferID != offer.ID {
		return errors.New("v6 pool delivery offer identity differs from canonical context")
	}
	if offer.Covered == 0 || offer.Covered > 32 ||
		!directDMHex(offer.InventoryCommitment, 32) || !directDMHex(offer.KeysCommitment, 32) ||
		offer.PublicSeed || offer.State != "active" || offer.NextNonce == 0 ||
		order.ID == 0 || offer.OpenOrderID != order.ID || order.Nonce == 0 ||
		offer.NextNonce-1 != order.Nonce || order.OfferID != offer.ID ||
		order.FundedAt == 0 || order.State != "funded" {
		return errors.New("v6 pool offer and order are not one active funded private sale")
	}
	payout, payoutOK := directDMAmount(offer.Payout)
	_, sellerBondOK := directDMAmount(offer.SellerBond)
	_, paidOK := directDMAmount(order.Paid)
	_, bondOK := directDMAmount(order.Bond)
	orderPayout, orderPayoutOK := directDMAmount(order.SellerPayout)
	if !payoutOK || !sellerBondOK || !paidOK || !bondOK || !orderPayoutOK ||
		payout.Cmp(orderPayout) != 0 {
		return errors.New("v6 pool delivery amounts differ from canonical funded sale")
	}
	if !directDMHex(order.Buyer, addressBytes) || !directDMHex(order.DeliveryKey, deliveryKeyBytes) {
		return errors.New("invalid canonical v6 pool buyer or delivery key")
	}
	key, _ := hex.DecodeString(order.DeliveryKey)
	if key[0] != 4 {
		return errors.New("canonical v6 pool delivery key is not uncompressed")
	}
	if _, err := crypto.UnmarshalPubkey(key); err != nil {
		return errors.New("canonical v6 pool delivery key is not a secp256k1 point")
	}
	if envelope.OrderID != order.ID || envelope.OrderNonce != order.Nonce ||
		envelope.Buyer != order.Buyer || envelope.DeliveryKey != order.DeliveryKey {
		return errors.New("v6 pool delivery names a different order, buyer, or delivery key")
	}
	if len(envelope.Ciphertext) != directDMCiphertextBytes*2 ||
		envelope.Ciphertext != strings.ToLower(envelope.Ciphertext) {
		return errors.New("v6 pool delivery ciphertext has invalid ECIES size or hex form")
	}
	ciphertext, err := hex.DecodeString(envelope.Ciphertext)
	if err != nil {
		return errors.New("v6 pool delivery ciphertext is not hex")
	}
	if ciphertext[0] != 4 {
		return errors.New("v6 pool delivery ciphertext has a compressed or invalid ephemeral point")
	}
	if _, err := crypto.UnmarshalPubkey(ciphertext[:deliveryKeyBytes]); err != nil {
		return errors.New("v6 pool delivery ciphertext has an invalid ECIES ephemeral point")
	}
	return nil
}
