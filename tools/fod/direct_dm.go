package main

// An idena.social message carries only a candidate private delivery. Its sender
// is not an authority: the caller must supply independently read chain state,
// and the buyer must decrypt and check the committed secrets before confirming.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/idena-network/idena-go/crypto"
)

const (
	directDMFormat  = "fod.direct-delivery"
	directDMVersion = 2
	// Uncompressed ephemeral point || AES-CTR IV || encrypted canonical
	// 106-byte epoch secrets || HMAC-SHA256 tag, under delivery ECIES v1.
	directDMCiphertextBytes = 65 + 16 + 106 + 32
	directDMMaxTextBytes    = 2048
)

// DirectDMEnvelope is JSON text suitable for copying into one private message.
// Hex fields use lowercase without a 0x prefix. The ciphertext is still
// untrusted until the buyer decrypts it and checks the offer's key commitment.
type DirectDMEnvelope struct {
	Format      string `json:"format"`
	Version     int    `json:"version"`
	Network     string `json:"network"`
	Contract    string `json:"contract"`
	Author      string `json:"author,omitempty"`
	OfferID     string `json:"offerId"`
	OrderID     uint64 `json:"orderId"`
	OrderNonce  uint64 `json:"orderNonce"`
	Buyer       string `json:"buyer"`
	DeliveryKey string `json:"deliveryKey"`
	Ciphertext  string `json:"ciphertext"`
}

// DirectDMOffer and DirectDMOrder are the canonical state the caller checked
// independently of the message and its purported sender. Amounts are decimal
// base units so they retain the contract's full unsigned 128-bit range.
type DirectDMOffer struct {
	ID          string
	Author      string
	Epoch       uint16
	Price       string
	NextNonce   uint64
	OpenOrderID uint64
	Retired     bool
}

type DirectDMOrder struct {
	ID          uint64
	OfferID     string
	Buyer       string
	DeliveryKey string
	Paid        string
	FundedAt    uint64
	Nonce       uint64
	State       string
}

type DirectDMContext struct {
	Network  string
	Contract string
	Offer    DirectDMOffer
	Order    DirectDMOrder
}

// NewDirectDMEnvelope builds an envelope from canonical order terms and an
// already-encrypted payload. It neither encrypts secrets nor sends a message.
func NewDirectDMEnvelope(context DirectDMContext, ciphertext []byte) (*DirectDMEnvelope, error) {
	envelope := &DirectDMEnvelope{
		Format:      directDMFormat,
		Version:     directDMVersion,
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
	if err := ValidateDirectDMEnvelope(envelope, context); err != nil {
		return nil, err
	}
	return envelope, nil
}

// EncodeDirectDMEnvelope returns compact JSON only after checking every field
// against the supplied chain context. Nothing is signed, sent, or settled.
func EncodeDirectDMEnvelope(envelope *DirectDMEnvelope, context DirectDMContext) (string, error) {
	if err := ValidateDirectDMEnvelope(envelope, context); err != nil {
		return "", err
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	if len(raw) > directDMMaxTextBytes {
		return "", errors.New("direct delivery message is too large")
	}
	return string(raw), nil
}

// ParseDirectDMEnvelope accepts one JSON object and rejects unknown, missing,
// or repeated fields before validating it against independently supplied state.
func ParseDirectDMEnvelope(message string, context DirectDMContext) (*DirectDMEnvelope, error) {
	if len(message) == 0 || len(message) > directDMMaxTextBytes {
		return nil, errors.New("direct delivery message has invalid size")
	}
	if err := directDMFields([]byte(message)); err != nil {
		return nil, err
	}
	var envelope DirectDMEnvelope
	if err := decodeStrict([]byte(message), &envelope); err != nil {
		return nil, fmt.Errorf("decode direct delivery: %w", err)
	}
	if err := ValidateDirectDMEnvelope(&envelope, context); err != nil {
		return nil, err
	}
	return &envelope, nil
}

// directDMFields checks the JSON object shape separately because the standard
// decoder otherwise silently accepts duplicate keys and trailing JSON values.
func directDMFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return errors.New("direct delivery must be one JSON object")
	}
	allowed := map[string]bool{
		"format": true, "version": true, "network": true, "contract": true, "author": true,
		"offerId": true, "orderId": true, "orderNonce": true, "buyer": true,
		"deliveryKey": true, "ciphertext": true,
	}
	seen := make(map[string]bool, len(allowed))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("direct delivery field: %w", err)
		}
		key, ok := token.(string)
		if !ok || !allowed[key] || seen[key] {
			return fmt.Errorf("unknown or repeated direct delivery field %q", key)
		}
		seen[key] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("direct delivery field %q: %w", key, err)
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return errors.New("direct delivery is missing fields or has invalid JSON")
	}
	for field := range allowed {
		if field != "author" && !seen[field] {
			return errors.New("direct delivery is missing fields or has invalid JSON")
		}
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("direct delivery has trailing content")
	}
	return nil
}

func directDMHex(value string, size int) bool {
	return isHex(value, size) && value == strings.ToLower(value) &&
		value != strings.Repeat("0", size*2)
}

func directDMAmount(value string) (*big.Int, bool) {
	if !isDecimal(value) {
		return nil, false
	}
	amount, ok := new(big.Int).SetString(value, 10)
	return amount, ok && amount.Sign() > 0 && amount.Cmp(maxContractAmount) <= 0
}

// ValidateDirectDMEnvelope checks role and replay boundaries, then the ECIES
// wire shape. This does not authenticate the social sender or prove decryption.
func ValidateDirectDMEnvelope(envelope *DirectDMEnvelope, context DirectDMContext) error {
	if envelope == nil || envelope.Format != directDMFormat || (envelope.Version != 1 && envelope.Version != directDMVersion) {
		return errors.New("unsupported direct delivery envelope")
	}
	if context.Network != NetworkNamePinned || !directDMHex(context.Contract, addressBytes) ||
		envelope.Network != context.Network || envelope.Contract != context.Contract {
		return errors.New("direct delivery network or contract differs from canonical context")
	}
	if !directDMHex(context.Offer.Author, addressBytes) || !directDMHex(context.Offer.ID, 32) {
		return errors.New("invalid canonical offer identity")
	}
	if (envelope.Version == 1 && envelope.Author != "") ||
		(envelope.Version == directDMVersion && envelope.Author != context.Offer.Author) {
		return errors.New("direct delivery author differs from canonical offer")
	}
	author, _ := hex.DecodeString(context.Offer.Author)
	offerPreimage := make([]byte, 0, len(directOfferDomain)+addressBytes+2)
	offerPreimage = append(offerPreimage, directOfferDomain...)
	offerPreimage = append(offerPreimage, author...)
	offerPreimage = append(offerPreimage, byte(context.Offer.Epoch>>8), byte(context.Offer.Epoch))
	derivedOfferID := sha256.Sum256(offerPreimage)
	if context.Offer.ID != hex.EncodeToString(derivedOfferID[:]) {
		return errors.New("canonical offer ID differs from its author and epoch")
	}
	if context.Order.ID == 0 || context.Order.Nonce == 0 || context.Order.FundedAt == 0 ||
		context.Order.State != "funded" || context.Offer.Retired ||
		context.Offer.NextNonce == 0 || context.Offer.NextNonce-1 != context.Order.Nonce ||
		context.Offer.OpenOrderID != context.Order.ID || context.Order.OfferID != context.Offer.ID {
		return errors.New("canonical offer and funded order do not describe one open sale")
	}
	price, priceOK := directDMAmount(context.Offer.Price)
	paid, paidOK := directDMAmount(context.Order.Paid)
	if !priceOK || !paidOK || price.Cmp(paid) != 0 {
		return errors.New("funded order price differs from canonical offer")
	}
	if !directDMHex(context.Order.Buyer, addressBytes) || !directDMHex(context.Order.DeliveryKey, deliveryKeyBytes) {
		return errors.New("invalid canonical buyer or delivery key")
	}
	key, _ := hex.DecodeString(context.Order.DeliveryKey)
	if _, err := crypto.UnmarshalPubkey(key); err != nil {
		return errors.New("canonical delivery key is not an uncompressed secp256k1 point")
	}
	if envelope.OfferID != context.Offer.ID || envelope.OrderID != context.Order.ID ||
		envelope.OrderNonce != context.Order.Nonce || envelope.Buyer != context.Order.Buyer ||
		envelope.DeliveryKey != context.Order.DeliveryKey {
		return errors.New("direct delivery names a different offer, order, buyer, or delivery key")
	}
	if len(envelope.Ciphertext) != directDMCiphertextBytes*2 || envelope.Ciphertext != strings.ToLower(envelope.Ciphertext) {
		return errors.New("direct delivery ciphertext has invalid ECIES size or hex form")
	}
	ciphertext, err := hex.DecodeString(envelope.Ciphertext)
	if err != nil {
		return errors.New("direct delivery ciphertext is not hex")
	}
	if _, err := crypto.UnmarshalPubkey(ciphertext[:deliveryKeyBytes]); err != nil {
		return errors.New("direct delivery ciphertext has an invalid ECIES ephemeral point")
	}
	return nil
}
