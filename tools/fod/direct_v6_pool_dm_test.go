package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func directV6PoolDMFixture(t *testing.T) (DirectV6PoolDMContext, []byte) {
	t.Helper()
	buyerKey, err := crypto.ToECDSA(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatalf("buyer public key: %v", err)
	}
	ephemeralKey, err := crypto.ToECDSA(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatalf("ephemeral public key: %v", err)
	}
	var author [addressBytes]byte
	copy(author[:], bytes.Repeat([]byte{0x11}, addressBytes))
	id := poolV6OfferID(author, 230)
	offerID := hex.EncodeToString(id[:])
	context := DirectV6PoolDMContext{
		Network:  NetworkNamePinned,
		Contract: strings.Repeat("cc", addressBytes),
		Offer: DirectV6PoolDMOffer{
			ID: offerID, Author: hex.EncodeToString(author[:]), Epoch: 230,
			Covered: 2, InventoryCommitment: strings.Repeat("ab", 32),
			KeysCommitment: strings.Repeat("cd", 32), Payout: "7", SellerBond: "5",
			NextNonce: 3, OpenOrderID: 7, State: "active",
		},
		Order: DirectV6PoolDMOrder{
			ID: 7, OfferID: offerID, Buyer: strings.Repeat("bb", addressBytes),
			DeliveryKey: hex.EncodeToString(crypto.FromECDSAPub(&buyerKey.PublicKey)),
			Paid:        "11", Bond: "2", SellerPayout: "7", FundedAt: 12345,
			Nonce: 2, State: "funded",
		},
	}
	// The envelope validator checks ECIES wire shape; no encryption occurs here.
	ciphertext := append(crypto.FromECDSAPub(&ephemeralKey.PublicKey),
		bytes.Repeat([]byte{0x42}, directDMCiphertextBytes-deliveryKeyBytes)...)
	return context, ciphertext
}

func TestDirectV6PoolDMEnvelopeRoundTrip(t *testing.T) {
	context, ciphertext := directV6PoolDMFixture(t)
	if context.Order.Paid == context.Offer.Payout {
		t.Fatal("fixture must exercise distinct buyer payment and seller payout")
	}
	envelope, err := NewDirectV6PoolDMEnvelope(context, ciphertext)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	message, err := EncodeDirectV6PoolDMEnvelope(envelope, context)
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	if len(message) > directDMMaxTextBytes || strings.Contains(message, "\n") ||
		envelope.Version != directV6PoolDMVersion || envelope.Author != context.Offer.Author {
		t.Fatal("envelope is not compact, bounded, and bound to the v6 seller")
	}
	parsed, err := ParseDirectV6PoolDMEnvelope(message, context)
	if err != nil {
		t.Fatalf("parse envelope: %v", err)
	}
	if !reflect.DeepEqual(parsed, envelope) {
		t.Fatal("parsed envelope differs from encoded envelope")
	}
}

func TestDirectV6PoolDMRejectsChangedEnvelope(t *testing.T) {
	context, ciphertext := directV6PoolDMFixture(t)
	valid, err := NewDirectV6PoolDMEnvelope(context, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*DirectDMEnvelope){
		"format":            func(v *DirectDMEnvelope) { v.Format = "fod.other" },
		"version":           func(v *DirectDMEnvelope) { v.Version = 2 },
		"network":           func(v *DirectDMEnvelope) { v.Network = "other" },
		"contract":          func(v *DirectDMEnvelope) { v.Contract = strings.Repeat("dd", addressBytes) },
		"author":            func(v *DirectDMEnvelope) { v.Author = strings.Repeat("aa", addressBytes) },
		"offer":             func(v *DirectDMEnvelope) { v.OfferID = strings.Repeat("aa", 32) },
		"order":             func(v *DirectDMEnvelope) { v.OrderID++ },
		"nonce":             func(v *DirectDMEnvelope) { v.OrderNonce++ },
		"buyer":             func(v *DirectDMEnvelope) { v.Buyer = strings.Repeat("aa", addressBytes) },
		"delivery key":      func(v *DirectDMEnvelope) { v.DeliveryKey = strings.Repeat("04", deliveryKeyBytes) },
		"short ciphertext":  func(v *DirectDMEnvelope) { v.Ciphertext = v.Ciphertext[:len(v.Ciphertext)-2] },
		"long ciphertext":   func(v *DirectDMEnvelope) { v.Ciphertext += "00" },
		"nonhex ciphertext": func(v *DirectDMEnvelope) { v.Ciphertext = "zz" + v.Ciphertext[2:] },
		"uppercase hex":     func(v *DirectDMEnvelope) { v.Ciphertext = "AB" + v.Ciphertext[2:] },
		"off-curve point": func(v *DirectDMEnvelope) {
			v.Ciphertext = strings.Repeat("00", deliveryKeyBytes) + v.Ciphertext[deliveryKeyBytes*2:]
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			modified := *valid
			change(&modified)
			if _, err := EncodeDirectV6PoolDMEnvelope(&modified, context); err == nil {
				t.Fatal("encoder accepted changed envelope")
			}
			raw, err := json.Marshal(&modified)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseDirectV6PoolDMEnvelope(string(raw), context); err == nil {
				t.Fatal("parser accepted changed envelope")
			}
		})
	}
}

func TestDirectV6PoolDMRejectsMalformedJSONAndChainState(t *testing.T) {
	context, ciphertext := directV6PoolDMFixture(t)
	envelope, err := NewDirectV6PoolDMEnvelope(context, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	message, err := EncodeDirectV6PoolDMEnvelope(envelope, context)
	if err != nil {
		t.Fatal(err)
	}
	badMessages := map[string]string{
		"unknown field":   message[:len(message)-1] + `,"sender":"trusted"}`,
		"duplicate field": message[:len(message)-1] + `,"buyer":"` + context.Order.Buyer + `"}`,
		"missing author":  strings.Replace(message, `"author":"`+context.Offer.Author+`",`, "", 1),
		"trailing JSON":   message + ` {}`,
		"oversized":       message + strings.Repeat(" ", directDMMaxTextBytes),
	}
	for name, bad := range badMessages {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDirectV6PoolDMEnvelope(bad, context); err == nil {
				t.Fatal("parser accepted malformed JSON")
			}
		})
	}
	badContexts := map[string]func(*DirectV6PoolDMContext){
		"different contract":        func(c *DirectV6PoolDMContext) { c.Contract = strings.Repeat("dd", addressBytes) },
		"different author":          func(c *DirectV6PoolDMContext) { c.Offer.Author = strings.Repeat("aa", addressBytes) },
		"wrong derived offer ID":    func(c *DirectV6PoolDMContext) { c.Offer.ID = strings.Repeat("aa", 32); c.Order.OfferID = c.Offer.ID },
		"different buyer":           func(c *DirectV6PoolDMContext) { c.Order.Buyer = strings.Repeat("aa", addressBytes) },
		"different order":           func(c *DirectV6PoolDMContext) { c.Order.ID++ },
		"different nonce":           func(c *DirectV6PoolDMContext) { c.Order.Nonce++ },
		"wrong order offer":         func(c *DirectV6PoolDMContext) { c.Order.OfferID = strings.Repeat("aa", 32) },
		"stale offer nonce":         func(c *DirectV6PoolDMContext) { c.Offer.NextNonce++ },
		"zero inventory commitment": func(c *DirectV6PoolDMContext) { c.Offer.InventoryCommitment = strings.Repeat("00", 32) },
		"zero keys commitment":      func(c *DirectV6PoolDMContext) { c.Offer.KeysCommitment = strings.Repeat("00", 32) },
		"inactive offer":            func(c *DirectV6PoolDMContext) { c.Offer.State = "queued-primary" },
		"public seed":               func(c *DirectV6PoolDMContext) { c.Offer.PublicSeed = true },
		"fallback order":            func(c *DirectV6PoolDMContext) { c.Order.State = "fallback-pending" },
		"settled order":             func(c *DirectV6PoolDMContext) { c.Order.State = "settled" },
		"unfunded order":            func(c *DirectV6PoolDMContext) { c.Order.FundedAt = 0 },
		"mismatched seller payout":  func(c *DirectV6PoolDMContext) { c.Order.SellerPayout = "8" },
		"zero buyer payment":        func(c *DirectV6PoolDMContext) { c.Order.Paid = "0" },
		"off-curve delivery key":    func(c *DirectV6PoolDMContext) { c.Order.DeliveryKey = strings.Repeat("04", deliveryKeyBytes) },
	}
	for name, change := range badContexts {
		t.Run(name, func(t *testing.T) {
			modified := context
			change(&modified)
			if _, err := ParseDirectV6PoolDMEnvelope(message, modified); err == nil {
				t.Fatal("parser accepted inconsistent canonical state")
			}
		})
	}
}
