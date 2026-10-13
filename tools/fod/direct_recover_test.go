package main

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectRecoverChecksSettledRevealAndRetainedOriginals(t *testing.T) {
	c, session, offer, order, _, _, vector, _, closeServer := directClientFixture(t)
	defer closeServer()
	offer.Retired = true
	offer.OpenOrderID = 0
	order.State = DirectSettled
	contract := strings.Repeat("cc", 20)
	calldata, err := EncodeDirectBuy(order.OfferID, order.DeliveryKey)
	if err != nil {
		t.Fatal(err)
	}
	purchase := &preparedAction{
		Format: preparedDirectBuyFormat, Version: actionRequestVersion, Operation: operationBuy,
		Network: NetworkNamePinned, Contract: contract, Amount: "5001000", Price: order.Paid.String(),
		Calldata: hex.EncodeToString(calldata), DirectOfferID: hex.EncodeToString(order.OfferID[:]),
		Buyer: hex.EncodeToString(order.Buyer[:]), DeliveryKey: hex.EncodeToString(order.DeliveryKey[:]),
		OrderNonce: order.Nonce, Author: vector.Author, Epoch: vector.Epoch, CoveredFlips: 1,
		InventoryCommitment: hex.EncodeToString(offer.Inventory[:]), KeyCommitment: hex.EncodeToString(offer.Keys[:]),
		DirectSession: session,
		Availability: &AvailabilityCheck{
			Author: vector.Author, Epoch: vector.Epoch, Binding: bindingCommitted,
			CoveredFlips: 1, VerifiedFlips: 1, InventoryCommitment: hex.EncodeToString(offer.Inventory[:]),
			Blobs: []RetainedFlip{{CID: vector.BlobCID, Hex: vector.Blob}},
		},
	}
	dir := t.TempDir()
	purchasePath := filepath.Join(dir, "purchase.json")
	if err := writePrivateJSON(purchasePath, purchase); err != nil {
		t.Fatal(err)
	}
	vaultPath := filepath.Join(dir, "recovered.age")
	receiptPath := filepath.Join(dir, "receipt.json")
	t.Setenv(PassphraseEnvVar, "test recovery passphrase")
	options := directCommandOptions{
		Mode: "direct-recover", Node: c, Network: NetworkNamePinned,
		Contract: contract, CodeHash: strings.Repeat("cd", 32), OrderID: 1,
		PurchasePath: purchasePath, VaultPath: vaultPath, OutPath: receiptPath, OutExplicit: true,
	}
	if err := directCommand(options); err != nil {
		t.Fatalf("settled public recovery refused: %v", err)
	}
	for _, path := range []string{vaultPath, receiptPath} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("recovery output %s is not owner-only: %v", path, err)
		}
	}
	vaultJSON, err := ReadVault(vaultPath, "test recovery passphrase")
	if err != nil {
		t.Fatal(err)
	}
	var payload VaultPayload
	if err := json.Unmarshal(vaultJSON, &payload); err != nil || payload.Address != "0x"+vector.Author || payload.Epoch != vector.Epoch {
		t.Fatalf("recovered vault names another author or epoch: %v", err)
	}
	receiptJSON, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt DirectRecoveredDelivery
	if err := json.Unmarshal(receiptJSON, &receipt); err != nil || receipt.Format != directRecoveredFormat ||
		receipt.KeySource != "public-reveal" || receipt.UsableFlips != 1 || receipt.Buyer != purchase.Buyer ||
		strings.Contains(string(receiptJSON), vector.SecretsCanonical) ||
		strings.Contains(string(receiptJSON), vaultPath) || strings.Contains(string(receiptJSON), "vaultPath") {
		t.Fatalf("recovery receipt is incomplete or contains public secrets: %v", err)
	}

	bad := *purchase
	bad.Buyer = strings.Repeat("ab", 20)
	if _, _, err := recoverDirectReveal(c, contract, 1, &bad, options.CodeHash); err == nil {
		t.Fatal("purchase for another buyer was accepted")
	}
	bad = *purchase
	check := *purchase.Availability
	check.Blobs = []RetainedFlip{{CID: vector.BlobCID, Hex: "00"}}
	bad.Availability = &check
	if _, _, err := recoverDirectReveal(c, contract, 1, &bad, options.CodeHash); err == nil {
		t.Fatal("altered original CID bytes were accepted")
	}
	bad = *purchase
	bad.DirectSession = nil
	if _, _, err := recoverDirectReveal(c, contract, 1, &bad, options.CodeHash); err == nil {
		t.Fatal("missing canonical session was accepted")
	}
	offer.Keys[0] ^= 1
	bad = *purchase
	bad.KeyCommitment = hex.EncodeToString(offer.Keys[:])
	if _, _, err := recoverDirectReveal(c, contract, 1, &bad, options.CodeHash); err == nil {
		t.Fatal("public reveal with the wrong secrets hash was accepted")
	}
	offer.Keys[0] ^= 1
	order.DeliveryHash[0] = 1
	offer.Retired = false
	if _, _, err := recoverDirectReveal(c, contract, 1, purchase, options.CodeHash); err == nil {
		t.Fatal("buyer-confirmed settlement was accepted as public fallback")
	}
	options.VaultPath = filepath.Join(dir, "should-not-exist.age")
	options.OutPath = filepath.Join(dir, "should-not-exist.json")
	options.CodeHash = strings.Repeat("ef", 32)
	if err := directCommand(options); err == nil {
		t.Fatal("unapproved contract code hash was accepted")
	}
	if _, err := os.Stat(options.VaultPath); !os.IsNotExist(err) {
		t.Fatalf("unapproved code hash wrote a vault: %v", err)
	}
}
