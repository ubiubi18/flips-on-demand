package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
)

func TestDirectMessageInputIsBounded(t *testing.T) {
	for _, input := range []string{"", strings.Repeat("x", directDMMaxTextBytes+1)} {
		if _, err := readDirectMessage(strings.NewReader(input)); err == nil {
			t.Fatal("accepted empty or oversized private message input")
		}
	}
	input := strings.Repeat("x", directDMMaxTextBytes)
	got, err := readDirectMessage(strings.NewReader(input))
	if err != nil || got != input {
		t.Fatalf("bounded private message input was not preserved: %v", err)
	}
}

func directDMFixture(t *testing.T) (DirectDMContext, []byte) {
	t.Helper()
	buyerKey, err := crypto.ToECDSA(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatalf("buyer key: %v", err)
	}
	author := strings.Repeat("11", addressBytes)
	epoch := uint16(230)
	preimage := append([]byte("fod:direct-offer:v2"), bytes.Repeat([]byte{0x11}, addressBytes)...)
	preimage = append(preimage, byte(epoch>>8), byte(epoch))
	offerID := sha256.Sum256(preimage)
	context := DirectDMContext{
		Network:  NetworkNamePinned,
		Contract: strings.Repeat("cc", addressBytes),
		Offer: DirectDMOffer{
			ID:          hex.EncodeToString(offerID[:]),
			Author:      author,
			Epoch:       epoch,
			Price:       "340282366920938463463374607431768211455",
			NextNonce:   3,
			OpenOrderID: 7,
		},
		Order: DirectDMOrder{
			ID:          7,
			OfferID:     hex.EncodeToString(offerID[:]),
			Buyer:       strings.Repeat("bb", addressBytes),
			DeliveryKey: hex.EncodeToString(crypto.FromECDSAPub(&buyerKey.PublicKey)),
			Paid:        "340282366920938463463374607431768211455",
			FundedAt:    12345,
			Nonce:       2,
			State:       "funded",
		},
	}
	// A real pinned ECIES payload exercises the wire size without contacting a
	// node or an idena.social account. The contents are harmless test bytes.
	ciphertext, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(&buyerKey.PublicKey), bytes.Repeat([]byte{0x42}, 106), nil, nil)
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	if len(ciphertext) != directDMCiphertextBytes {
		t.Fatalf("fixture ciphertext is %d bytes", len(ciphertext))
	}
	return context, ciphertext
}

func TestDirectDMEnvelopeRoundTripWithCanonicalFundedOrder(t *testing.T) {
	context, ciphertext := directDMFixture(t)
	envelope, err := NewDirectDMEnvelope(context, ciphertext)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	message, err := EncodeDirectDMEnvelope(envelope, context)
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	if strings.Contains(message, "\n") || len(message) > directDMMaxTextBytes {
		t.Fatalf("message is not compact, bounded JSON")
	}
	if envelope.Version != 2 || envelope.Author != context.Offer.Author ||
		!strings.Contains(message, `"author":"`+context.Offer.Author+`"`) {
		t.Fatal("new delivery message does not bind the seller account")
	}
	parsed, err := ParseDirectDMEnvelope(message, context)
	if err != nil {
		t.Fatalf("parse envelope: %v", err)
	}
	if !reflect.DeepEqual(parsed, envelope) {
		t.Fatalf("parsed envelope differs from encoded envelope")
	}
}

func TestDirectDMParsesHistoricalMessageWithoutAuthor(t *testing.T) {
	context, ciphertext := directDMFixture(t)
	envelope, err := NewDirectDMEnvelope(context, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Version = 1
	envelope.Author = ""
	message, err := EncodeDirectDMEnvelope(envelope, context)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(message, `"author"`) {
		t.Fatal("historical message unexpectedly contains an author")
	}
	if _, err := ParseDirectDMEnvelope(message, context); err != nil {
		t.Fatalf("historical direct delivery no longer parses: %v", err)
	}
}

func TestDirectDMRejectsUntrustedMessageTerms(t *testing.T) {
	context, ciphertext := directDMFixture(t)
	valid, err := NewDirectDMEnvelope(context, ciphertext)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	tests := map[string]func(*DirectDMEnvelope){
		"wrong format":       func(v *DirectDMEnvelope) { v.Format = "fod.other" },
		"wrong version":      func(v *DirectDMEnvelope) { v.Version++ },
		"wrong network":      func(v *DirectDMEnvelope) { v.Network = "other" },
		"wrong contract":     func(v *DirectDMEnvelope) { v.Contract = strings.Repeat("dd", addressBytes) },
		"wrong author":       func(v *DirectDMEnvelope) { v.Author = strings.Repeat("aa", addressBytes) },
		"wrong offer":        func(v *DirectDMEnvelope) { v.OfferID = strings.Repeat("aa", 32) },
		"wrong order":        func(v *DirectDMEnvelope) { v.OrderID++ },
		"replayed nonce":     func(v *DirectDMEnvelope) { v.OrderNonce-- },
		"wrong buyer":        func(v *DirectDMEnvelope) { v.Buyer = strings.Repeat("aa", addressBytes) },
		"wrong delivery key": func(v *DirectDMEnvelope) { v.DeliveryKey = strings.Repeat("04", deliveryKeyBytes) },
		"short ciphertext":   func(v *DirectDMEnvelope) { v.Ciphertext = v.Ciphertext[:len(v.Ciphertext)-2] },
		"long ciphertext":    func(v *DirectDMEnvelope) { v.Ciphertext += "00" },
		"nonhex ciphertext":  func(v *DirectDMEnvelope) { v.Ciphertext = "zz" + v.Ciphertext[2:] },
		"off-curve point": func(v *DirectDMEnvelope) {
			v.Ciphertext = strings.Repeat("00", deliveryKeyBytes) + v.Ciphertext[deliveryKeyBytes*2:]
		},
		"uppercase hex": func(v *DirectDMEnvelope) { v.Ciphertext = v.Ciphertext[:130] + "AB" + v.Ciphertext[132:] },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			modified := *valid
			change(&modified)
			if _, err := EncodeDirectDMEnvelope(&modified, context); err == nil {
				t.Fatal("encoder accepted altered message")
			}
			raw, err := json.Marshal(&modified)
			if err != nil {
				t.Fatalf("marshal altered message: %v", err)
			}
			if _, err := ParseDirectDMEnvelope(string(raw), context); err == nil {
				t.Fatal("parser accepted altered message")
			}
		})
	}
}

func TestDirectDMRejectsMalformedJSONAndStaleChainState(t *testing.T) {
	context, ciphertext := directDMFixture(t)
	envelope, err := NewDirectDMEnvelope(context, ciphertext)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	message, err := EncodeDirectDMEnvelope(envelope, context)
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	badMessages := map[string]string{
		"unknown field":   message[:len(message)-1] + `,"sender":"trusted"}`,
		"duplicate field": message[:len(message)-1] + `,"buyer":"` + context.Order.Buyer + `"}`,
		"missing field":   strings.Replace(message, `"buyer":"`+context.Order.Buyer+`",`, "", 1),
		"missing author":  strings.Replace(message, `"author":"`+context.Offer.Author+`",`, "", 1),
		"trailing JSON":   message + ` {}`,
		"wrong type":      strings.Replace(message, `"orderId":7`, `"orderId":"7"`, 1),
		"oversized":       message + strings.Repeat(" ", directDMMaxTextBytes),
	}
	for name, bad := range badMessages {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDirectDMEnvelope(bad, context); err == nil {
				t.Fatal("parser accepted malformed message")
			}
		})
	}
	badContexts := map[string]func(*DirectDMContext){
		"settled order":    func(c *DirectDMContext) { c.Order.State = "settled" },
		"refunded order":   func(c *DirectDMContext) { c.Order.State = "refunded" },
		"closed offer":     func(c *DirectDMContext) { c.Offer.OpenOrderID = 0 },
		"retired offer":    func(c *DirectDMContext) { c.Offer.Retired = true },
		"different offer":  func(c *DirectDMContext) { c.Order.OfferID = strings.Repeat("aa", 32) },
		"different author": func(c *DirectDMContext) { c.Offer.Author = strings.Repeat("aa", addressBytes) },
		"stale nonce":      func(c *DirectDMContext) { c.Offer.NextNonce++ },
		"underpayment":     func(c *DirectDMContext) { c.Order.Paid = "1" },
		"invalid key":      func(c *DirectDMContext) { c.Order.DeliveryKey = strings.Repeat("04", deliveryKeyBytes) },
		"unfunded height":  func(c *DirectDMContext) { c.Order.FundedAt = 0 },
	}
	for name, change := range badContexts {
		t.Run(name, func(t *testing.T) {
			modified := context
			change(&modified)
			if _, err := ParseDirectDMEnvelope(message, modified); err == nil {
				t.Fatal("parser accepted stale or inconsistent chain state")
			}
		})
	}
}
