package main

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
)

func directClientFixture(t *testing.T) (*client, *SessionBundle, *DirectOffer, *DirectOrder, *ecdsa.PrivateKey, *ecdsa.PrivateKey, blobVector, *big.Int, func()) {
	t.Helper()
	raw, err := os.ReadFile("../../proofs/registration/vectors/blob-flip-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector blobVector
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatal(err)
	}
	cid, err := cidBytesFromString(vector.BlobCID)
	if err != nil {
		t.Fatal(err)
	}
	var author common.Address
	copy(author[:], mustDecode(vector.Author))
	dir := writeSessionDatadirAtEpoch(t, author, [][]byte{cid}, state.AtLeastOneFlipReported, true,
		vector.Epoch, vector.Epoch+1, state.FlipLotteryPeriod)
	session, err := readSessionBundle(dir, sessionInventoryHeight, sessionVerdictHeight, vector.Author)
	if err != nil {
		t.Fatal(err)
	}
	var keyDigest [32]byte
	copy(keyDigest[:], mustDecode(vector.SecretsCommitment))
	var inventory [32]byte
	copy(inventory[:], mustDecode(inventoryCommitment(vector.Author, vector.Epoch, session.Manifest)))
	offer := &DirectOffer{
		Author: author, Epoch: vector.Epoch, Covered: 1, Inventory: inventory, Keys: keyDigest,
		Price: big.NewInt(1000), NextNonce: 2, OpenOrderID: 1,
	}
	delivery, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	var deliveryPub [65]byte
	copy(deliveryPub[:], crypto.FromECDSAPub(&delivery.PublicKey))
	buyerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	var buyer [20]byte
	copy(buyer[:], crypto.PubkeyToAddress(buyerKey.PublicKey).Bytes())
	id := DirectOfferID(author, vector.Epoch)
	order := &DirectOrder{OfferID: id, Buyer: buyer, DeliveryKey: deliveryPub, Paid: big.NewInt(1000), Bond: big.NewInt(5_000_000), FundedAt: 100, Nonce: 1, State: DirectFunded}
	feeRate := big.NewInt(1)
	if _, err := EncodeDirectOffer(*offer); err != nil {
		t.Fatal(err)
	}
	if _, err := EncodeDirectOrder(*order); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		var result any
		switch request.Method {
		case "dna_getCoinbaseAddr":
			result = "0x" + hex.EncodeToString(buyer[:])
		case "contract_getStake":
			result = map[string]any{"Hash": "0x" + strings.Repeat("cd", 32)}
		case "bcn_feePerGas":
			result = feeRate.String()
		case "contract_iterateMap":
			if len(request.Params) < 2 || request.Params[1] != directOfferPrefix {
				http.Error(w, "unexpected prefix", 400)
				return
			}
			offerRaw, _ := EncodeDirectOffer(*offer)
			result = map[string]any{"items": []any{map[string]any{"key": hex.EncodeToString(id[:]), "value": hex.EncodeToString(offerRaw)}}, "continuationToken": nil}
		case "contract_readData":
			if len(request.Params) < 2 {
				http.Error(w, "unexpected state key", 400)
				return
			}
			switch request.Params[1] {
			case "direct/order/1":
				orderRaw, _ := EncodeDirectOrder(*order)
				result = "0x" + hex.EncodeToString(orderRaw)
			case "direct/reveal/1":
				result = "0x" + vector.SecretsCanonical
			default:
				http.Error(w, "unexpected state key", 400)
				return
			}
		case "ipfs_get":
			if len(request.Params) != 1 || request.Params[0] != vector.BlobCID {
				http.Error(w, "unexpected CID", 400)
				return
			}
			result = "0x" + vector.Blob
		case "bcn_blockAt":
			if len(request.Params) != 1 {
				http.Error(w, "missing height", 400)
				return
			}
			height := uint64(request.Params[0].(float64))
			root := ""
			switch height {
			case session.Inventory.Height:
				root = session.Inventory.RootHex
			case session.Verdict.Height:
				root = session.Verdict.RootHex
			default:
				http.Error(w, "unexpected height", 400)
				return
			}
			result = map[string]any{"height": height, "root": "0x" + root}
		default:
			http.Error(w, "unexpected method "+request.Method, 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "error": nil})
	}))
	return &client{url: server.URL, http: newHTTPClient()}, session, offer, order, delivery, buyerKey, vector, feeRate, server.Close
}

func TestDirectPrivateDeliveryRequiresOriginalFlipAndCurrentOrder(t *testing.T) {
	c, session, offer, order, delivery, buyerKey, vector, _, closeServer := directClientFixture(t)
	defer closeServer()
	contract := strings.Repeat("cc", 20)
	secrets := mustDecode(vector.SecretsCanonical)
	message, err := prepareDirectDM(c, contract, 1, secrets)
	if err != nil {
		t.Fatal(err)
	}
	check := &AvailabilityCheck{
		CheckedAt: checkedAtNow(), Author: vector.Author, Epoch: vector.Epoch,
		Binding: bindingCommitted, CoveredFlips: 1, VerifiedFlips: 1,
		InventoryCommitment: hex.EncodeToString(offer.Inventory[:]),
		Blobs:               []RetainedFlip{{CID: vector.BlobCID, Hex: vector.Blob}},
	}
	context := directDMContext(contract, offer, 1, order)
	purchase := &preparedAction{
		Format: preparedDirectBuyFormat, Version: 1, Operation: operationBuy,
		Network: NetworkNamePinned, Contract: contract, Amount: "5001000", Price: order.Paid.String(),
		DirectOfferID: context.Offer.ID, Buyer: context.Order.Buyer,
		DeliveryKey: context.Order.DeliveryKey, OrderNonce: order.Nonce,
		InventoryCommitment: hex.EncodeToString(offer.Inventory[:]), KeyCommitment: hex.EncodeToString(offer.Keys[:]),
		Availability: check, DirectSession: session,
	}
	record, payload, err := importDirectDM(c, contract, 1, message, purchase, delivery, false)
	if err != nil {
		t.Fatalf("genuine delivery refused: %v", err)
	}
	if record.UsableFlips != 1 || payload.Address != "0x"+vector.Author {
		t.Fatal("genuine delivery did not open the original flip")
	}
	receiptJSON, err := json.Marshal(record)
	if err != nil || strings.Contains(string(receiptJSON), "vaultPath") {
		t.Fatal("buyer verification receipt exposes a local vault path")
	}
	prepared, err := prepareDirectConfirm(c, contract, record)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildTransaction(prepared, buyerKey, 1, vector.Epoch, big.NewInt(1_000_000), big.NewInt(0)); err == nil {
		t.Fatal("direct confirmation signed before canonical revalidation")
	}
	if err := validatePreparedDirect(c, prepared, buyerKey); err != nil {
		t.Fatalf("verified confirmation refused: %v", err)
	}
	if !prepared.confirmValidated {
		t.Fatal("verified confirmation was not marked signable")
	}
	if _, _, err := BuildTransaction(prepared, buyerKey, 1, vector.Epoch, big.NewInt(1_000_000), big.NewInt(0)); err != nil {
		t.Fatalf("validated direct confirmation could not be signed: %v", err)
	}

	bad := *purchase
	bad.Availability = &AvailabilityCheck{}
	*bad.Availability = *check
	bad.Availability.Blobs = []RetainedFlip{{CID: vector.BlobCID, Hex: "00"}}
	if _, _, err := importDirectDM(c, contract, 1, message, &bad, delivery, false); err == nil {
		t.Fatal("wrong retained original bytes were accepted")
	}
	bad = *purchase
	bad.DirectSession = nil
	if _, _, err := importDirectDM(c, contract, 1, message, &bad, delivery, false); err == nil {
		t.Fatal("missing canonical session was accepted")
	}
	bad = *purchase
	bad.Price = "999"
	if _, _, err := importDirectDM(c, contract, 1, message, &bad, delivery, false); err == nil {
		t.Fatal("retained purchase with a different price was accepted")
	}
	bad = *purchase
	bad.Amount = bad.Price
	if _, _, err := importDirectDM(c, contract, 1, message, &bad, delivery, false); err == nil {
		t.Fatal("retained purchase without the funded bond was accepted")
	}
	if _, _, err := importDirectDM(c, contract, 1, strings.Replace(message, context.Order.Buyer, strings.Repeat("42", 20), 1), purchase, delivery, false); err == nil {
		t.Fatal("private message bound to another buyer was accepted")
	}
	mutated := *record
	mutated.UsableFlips = 0
	if _, err := prepareDirectConfirm(c, contract, &mutated); err == nil {
		t.Fatal("altered usable-delivery receipt was accepted")
	}
}

func TestDirectCheckVerifiesWithoutIssuingConfirmableReceipt(t *testing.T) {
	c, session, offer, order, delivery, _, vector, _, closeServer := directClientFixture(t)
	defer closeServer()
	contract := strings.Repeat("cc", 20)
	message, err := prepareDirectDM(c, contract, 1, mustDecode(vector.SecretsCanonical))
	if err != nil {
		t.Fatal(err)
	}
	context := directDMContext(contract, offer, 1, order)
	purchase := &preparedAction{
		Format: preparedDirectBuyFormat, Version: 1, Operation: operationBuy,
		Network: NetworkNamePinned, Contract: contract, Amount: "5001000", Price: order.Paid.String(),
		DirectOfferID: context.Offer.ID, Buyer: context.Order.Buyer,
		DeliveryKey: context.Order.DeliveryKey, OrderNonce: order.Nonce,
		InventoryCommitment: hex.EncodeToString(offer.Inventory[:]), KeyCommitment: hex.EncodeToString(offer.Keys[:]),
		Availability: &AvailabilityCheck{
			CheckedAt: checkedAtNow(), Author: vector.Author, Epoch: vector.Epoch,
			Binding: bindingCommitted, CoveredFlips: 1, VerifiedFlips: 1,
			InventoryCommitment: hex.EncodeToString(offer.Inventory[:]),
			Blobs:               []RetainedFlip{{CID: vector.BlobCID, Hex: vector.Blob}},
		},
		DirectSession: session,
	}
	dir := t.TempDir()
	messagePath := filepath.Join(dir, "message")
	purchasePath := filepath.Join(dir, "purchase.json")
	keyPath := filepath.Join(dir, "delivery.key")
	resultPath := filepath.Join(dir, "check.json")
	if err := os.WriteFile(messagePath, []byte(message), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateJSON(purchasePath, purchase); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(crypto.FromECDSA(delivery))), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := directCommandOptions{
		Mode: "direct-check", Node: c, Network: NetworkNamePinned,
		Contract: contract, CodeHash: strings.Repeat("cd", 32), OrderID: 1,
		MessagePath: messagePath, PurchasePath: purchasePath, DeliveryKeyPath: keyPath,
		OutPath: resultPath, OutExplicit: true,
	}
	if err := directCommand(opts); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result directCheckResult
	if err := decodeStrict(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Format != "fod.direct-check" || result.OrderID != 1 || result.UsableFlips != 1 || result.Epoch != vector.Epoch {
		t.Fatalf("unexpected check result: %+v", result)
	}
	if strings.Contains(string(raw), "contentCheckSignature") || strings.Contains(string(raw), "ciphertext") || strings.Contains(string(raw), vector.SecretsCanonical) {
		t.Fatal("display-only result contains a receipt or secret")
	}
	info, err := os.Stat(resultPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("check result is not owner-only: %v", err)
	}
	var receipt DirectOpenedDelivery
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDirectConfirm(c, contract, &receipt); err == nil {
		t.Fatal("display-only result was accepted for payment confirmation")
	}
	envelope, err := ParseDirectDMEnvelope(message, context)
	if err != nil {
		t.Fatal(err)
	}
	order.State = DirectSettled
	order.DeliveryHash = sha256.Sum256(mustDecode(envelope.Ciphertext))
	offer.OpenOrderID = 0
	offer.NextNonce = 3
	settled := opts
	settled.OutPath = filepath.Join(dir, "settled-check.json")
	if err := directCommand(settled); err != nil {
		t.Fatalf("cooperatively settled delivery refused: %v", err)
	}
	t.Setenv(PassphraseEnvVar, "settled-test-passphrase")
	receive := opts
	receive.Mode = "direct-receive"
	receive.VaultPath = filepath.Join(dir, "settled-vault.age")
	receive.OutPath = filepath.Join(dir, "settled-receipt.json")
	if err := directCommand(receive); err != nil {
		t.Fatalf("cooperatively settled delivery could not be saved: %v", err)
	}
	vaultRaw, err := ReadVault(receive.VaultPath, "settled-test-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	var vault VaultPayload
	if err := decodeStrict(vaultRaw, &vault); err != nil || vault.Address != "0x"+vector.Author {
		t.Fatalf("settled vault does not hold the verified author's keys: %v", err)
	}
	receiptRaw, err := os.ReadFile(receive.OutPath)
	if err != nil {
		t.Fatal(err)
	}
	var opened DirectOpenedDelivery
	if err := decodeStrict(receiptRaw, &opened); err != nil {
		t.Fatal(err)
	}
	if err := verifyDirectOpened(&opened); err != nil || opened.Ciphertext != envelope.Ciphertext || opened.UsableFlips != 1 {
		t.Fatalf("settled receipt is not the verified delivery: %v", err)
	}
	for _, path := range []string{receive.VaultPath, receive.OutPath} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("settled output is not owner-only: %v", err)
		}
	}
	confirm := directCommandOptions{
		Mode: "direct-confirm", Node: c, Network: NetworkNamePinned,
		Contract: contract, CodeHash: opts.CodeHash, DeliveryPath: receive.OutPath,
		PreparedPath: filepath.Join(dir, "settled-confirm.json"),
	}
	if err := directCommand(confirm); err == nil || !strings.Contains(err.Error(), "not a currently active order") {
		t.Fatalf("settled receipt could prepare another confirmation: %v", err)
	}
	if _, err := os.Stat(confirm.PreparedPath); !os.IsNotExist(err) {
		t.Fatalf("settled confirmation wrote a prepared action: %v", err)
	}

	order.DeliveryHash = [32]byte{1}
	settled.OutPath = filepath.Join(dir, "wrong-hash.json")
	if err := directCommand(settled); err == nil {
		t.Fatal("settled delivery with another ciphertext hash was accepted")
	}
	wrong := receive
	wrong.VaultPath = filepath.Join(dir, "wrong-hash-vault.age")
	wrong.OutPath = filepath.Join(dir, "wrong-hash-receipt.json")
	if err := directCommand(wrong); err == nil || !strings.Contains(err.Error(), "settled chain hash") {
		t.Fatalf("receive accepted a different ciphertext hash: %v", err)
	}
	for _, path := range []string{wrong.VaultPath, wrong.OutPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("wrong-hash delivery wrote an output: %v", err)
		}
	}
	order.DeliveryHash = sha256.Sum256(mustDecode(envelope.Ciphertext))
	for _, flags := range []struct {
		name        string
		autoConfirm bool
		broadcast   bool
	}{
		{"auto-confirm", true, true},
		{"broadcast", false, true},
	} {
		blocked := receive
		blocked.AutoConfirm = flags.autoConfirm
		blocked.Broadcast = flags.broadcast
		blocked.VaultPath = filepath.Join(dir, flags.name+"-vault.age")
		blocked.OutPath = filepath.Join(dir, flags.name+"-receipt.json")
		blocked.PreparedPath = filepath.Join(dir, flags.name+"-prepared.json")
		blocked.SignedPath = filepath.Join(dir, flags.name+"-signed.json")
		blocked.NodeKeyPath = filepath.Join(dir, flags.name+"-nodekey")
		blocked.NonceText, blocked.TxEpochText, blocked.MaxFeeText = "1", "1", "1"
		if err := directCommand(blocked); err == nil || !strings.Contains(err.Error(), "not a currently active order") {
			t.Fatalf("settled receive accepted %s: %v", flags.name, err)
		}
		for _, path := range []string{blocked.VaultPath, blocked.OutPath, blocked.PreparedPath, blocked.SignedPath} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("settled %s wrote an output: %v", flags.name, err)
			}
		}
	}
	order.DeliveryHash = [32]byte{}
	if err := directCommand(settled); err == nil {
		t.Fatal("public fallback was accepted as a private cooperative delivery")
	}
	for _, flag := range []string{"auto-confirm", "broadcast"} {
		blocked := opts
		blocked.AutoConfirm = flag == "auto-confirm"
		blocked.Broadcast = flag == "broadcast"
		blocked.OutPath = filepath.Join(dir, "blocked.json")
		if err := directCommand(blocked); err == nil {
			t.Fatalf("direct-check accepted %s", flag)
		}
	}
}

func TestDirectBuyPreparesForActiveRPCBuyer(t *testing.T) {
	c, session, offer, _, _, buyerKey, vector, _, closeServer := directClientFixture(t)
	defer closeServer()
	offer.OpenOrderID = 0
	offer.NextNonce = 1
	buyer, err := nodekeyAddressOf(buyerKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.json")
	if err := writePrivateJSON(sessionPath, session); err != nil {
		t.Fatal(err)
	}
	deliveryPath := filepath.Join(dir, "delivery.key")
	preparedPath := filepath.Join(dir, "purchase.json")
	if err := directCommand(directCommandOptions{
		Mode: "direct-buy", Node: c, Network: NetworkNamePinned,
		Contract: strings.Repeat("cc", 20), CodeHash: strings.Repeat("cd", 32),
		SessionPath: sessionPath, BuyerAddress: "0x" + strings.ToUpper(buyer),
		DeliveryKeyPath: deliveryPath, PreparedPath: preparedPath,
	}); err != nil {
		t.Fatalf("active buyer purchase refused: %v", err)
	}
	var prepared preparedAction
	if raw, err := os.ReadFile(preparedPath); err != nil {
		t.Fatal(err)
	} else if err := decodeStrict(raw, &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.Buyer != buyer || prepared.Format != preparedDirectBuyFormat ||
		prepared.Operation != operationBuy || prepared.DirectSession == nil ||
		prepared.Availability == nil || len(prepared.Availability.Blobs) != 1 ||
		prepared.Availability.Blobs[0].CID != vector.BlobCID || prepared.Availability.Blobs[0].Hex != vector.Blob {
		t.Fatalf("purchase did not retain the canonical offer for buyer %s", buyer)
	}
	for _, path := range []string{deliveryPath, preparedPath} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s is not owner-only: %v", path, err)
		}
	}
}

func TestDirectBuyRejectsBuyerMismatchBeforeWriting(t *testing.T) {
	c, session, _, _, _, buyerKey, _, _, closeServer := directClientFixture(t)
	defer closeServer()
	buyer, err := nodekeyAddressOf(buyerKey)
	if err != nil {
		t.Fatal(err)
	}
	mismatch := "0" + buyer[1:]
	if buyer[0] == '0' {
		mismatch = "1" + buyer[1:]
	}
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session.json")
	if err := writePrivateJSON(sessionPath, session); err != nil {
		t.Fatal(err)
	}
	deliveryPath := filepath.Join(dir, "delivery.key")
	preparedPath := filepath.Join(dir, "purchase.json")
	err = directCommand(directCommandOptions{
		Mode: "direct-buy", Node: c, Network: NetworkNamePinned,
		Contract: strings.Repeat("cc", 20), CodeHash: strings.Repeat("cd", 32),
		SessionPath: sessionPath, BuyerAddress: mismatch,
		DeliveryKeyPath: deliveryPath, PreparedPath: preparedPath,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match the active node buyer") {
		t.Fatalf("expected active buyer mismatch, got %v", err)
	}
	for _, path := range []string{deliveryPath, preparedPath} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("mismatched buyer wrote %s: %v", path, err)
		}
	}
}

func TestDirectBuyRequiresExactlyOneBuyerSource(t *testing.T) {
	for _, opts := range []directCommandOptions{
		{Mode: "direct-buy"},
		{Mode: "direct-buy", BuyerAddress: strings.Repeat("ab", 20), NodeKeyPath: "nodekey"},
	} {
		if err := directCommand(opts); err == nil || !strings.Contains(err.Error(), "exactly one of -buyer ADDRESS or -nodekey FILE") {
			t.Fatalf("expected buyer source refusal before RPC access, got %v", err)
		}
	}
}

func TestDirectBuySignsOnlyFreshCanonicalRetainedInventory(t *testing.T) {
	c, session, offer, _, delivery, buyerKey, vector, feeRate, closeServer := directClientFixture(t)
	defer closeServer()
	offer.OpenOrderID = 0
	offer.NextNonce = 1
	contract := strings.Repeat("cc", 20)
	buyer, err := nodekeyAddressOf(buyerKey)
	if err != nil {
		t.Fatal(err)
	}
	feeRate.SetInt64(100)
	p, err := prepareDirectBuy(c, contract, session, buyer, delivery)
	if err != nil {
		t.Fatalf("canonical direct purchase refused: %v", err)
	}
	if p.Price != "1000" || p.Amount != "625001000" {
		t.Fatalf("purchase funding is price %s and total %s", p.Price, p.Amount)
	}
	if _, err := prepareDirectBuy(c, contract, session, buyer, buyerKey); err == nil {
		t.Fatal("buyer signing key was accepted as a delivery key")
	}
	badKey := *p
	var sameKey [65]byte
	copy(sameKey[:], crypto.FromECDSAPub(&buyerKey.PublicKey))
	badKey.DeliveryKey = hex.EncodeToString(sameKey[:])
	id, err := directPreparedOfferID(p)
	if err != nil {
		t.Fatal(err)
	}
	badCalldata, err := EncodeDirectBuy(id, sameKey)
	if err != nil {
		t.Fatal(err)
	}
	badKey.Calldata = hex.EncodeToString(badCalldata)
	if err := validatePreparedDirect(c, &badKey, buyerKey); err == nil {
		t.Fatal("signing gate accepted the buyer signing key as delivery key")
	}
	if len(p.Availability.Blobs) != 1 || p.Availability.Blobs[0].CID != vector.BlobCID {
		t.Fatal("purchase did not retain the original flip")
	}
	if _, _, err := BuildTransaction(p, buyerKey, 1, vector.Epoch, big.NewInt(1_000_000), big.NewInt(0)); err == nil {
		t.Fatal("direct purchase signed before current-state revalidation")
	}
	if err := validatePreparedDirect(c, p, buyerKey); err != nil {
		t.Fatalf("validated buy refused: %v", err)
	}
	if _, _, err := BuildTransaction(p, buyerKey, 1, vector.Epoch, big.NewInt(1_000_000), big.NewInt(0)); err != nil {
		t.Fatalf("validated buy could not sign: %v", err)
	}
	feeRate.SetInt64(120)
	if err := validatePreparedDirect(c, p, buyerKey); err != nil {
		t.Fatalf("buy with a fee increase inside bond headroom was refused: %v", err)
	}
	feeRate.SetInt64(126)
	if err := validatePreparedDirect(c, p, buyerKey); err == nil {
		t.Fatal("buy with insufficient bond was accepted after fee rate exceeded headroom")
	}
	feeRate.SetInt64(100)
	tooLarge := *p
	tooLarge.Amount = "625001001"
	if err := validatePreparedDirect(c, &tooLarge, buyerKey); err == nil {
		t.Fatal("purchase funding above bounded quote was accepted")
	}
	tooSmall := *p
	tooSmall.Amount = "500000999"
	if err := validatePreparedDirect(c, &tooSmall, buyerKey); err == nil {
		t.Fatal("purchase funding below minimum bond was accepted")
	}
	offer.NextNonce = 2
	if err := validatePreparedDirect(c, p, buyerKey); err == nil {
		t.Fatal("stale direct-buy nonce was accepted")
	}
	if _, _, err := BuildTransaction(p, buyerKey, 1, vector.Epoch, big.NewInt(1_000_000), big.NewInt(0)); err == nil {
		t.Fatal("stale direct-buy stayed signable")
	}
}
