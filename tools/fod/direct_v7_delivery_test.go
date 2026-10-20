package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

func TestDirectV7PrivateDeliveryAndCooperativeConfirm(t *testing.T) {
	_, session, _, _, deliveryKey, buyerKey, vector, _, closeOld := directClientFixture(t)
	closeOld()
	contract := strings.Repeat("cc", 20)
	codeHash := strings.Repeat("cd", 32)
	authorRaw, _ := hex.DecodeString(vector.Author)
	var author [20]byte
	copy(author[:], authorRaw)
	id := poolV7OfferID(author, vector.Epoch)
	var inventory, keys [32]byte
	copy(inventory[:], mustDecode(inventoryCommitment(vector.Author, vector.Epoch, session.Manifest)))
	copy(keys[:], mustDecode(vector.SecretsCommitment))
	var buyer [20]byte
	copy(buyer[:], crypto.PubkeyToAddress(buyerKey.PublicKey).Bytes())
	active := buyer
	deliveryPub := crypto.FromECDSAPub(&deliveryKey.PublicKey)
	price, bond := big.NewInt(1000), big.NewInt(5_000_000)
	offerRaw := make([]byte, 222)
	copy(offerRaw[:20], author[:])
	binary.BigEndian.PutUint16(offerRaw[20:22], vector.Epoch)
	binary.BigEndian.PutUint32(offerRaw[22:26], 1)
	copy(offerRaw[26:58], inventory[:])
	copy(offerRaw[58:90], keys[:])
	offerRaw[105] = 1
	binary.BigEndian.PutUint64(offerRaw[106:114], 2)
	binary.BigEndian.PutUint64(offerRaw[114:122], 1)
	offerRaw[122], offerRaw[204] = 3, 1
	orderRaw := make([]byte, 215)
	copy(orderRaw[:32], id[:])
	copy(orderRaw[32:52], buyer[:])
	copy(orderRaw[52:117], deliveryPub)
	copy(orderRaw[133-len(price.Bytes()):133], price.Bytes())
	copy(orderRaw[149-len(bond.Bytes()):149], bond.Bytes())
	copy(orderRaw[165-len(price.Bytes()):165], price.Bytes())
	binary.BigEndian.PutUint64(orderRaw[165:173], 100)
	binary.BigEndian.PutUint64(orderRaw[173:181], 1)
	orderRaw[181] = 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "dna_getCoinbaseAddr":
			result = "0x" + hex.EncodeToString(active[:])
		case "contract_getStake":
			result = map[string]any{"Hash": "0x" + codeHash}
		case "contract_iterateMap":
			result = map[string]any{"items": []any{map[string]any{"key": hex.EncodeToString(id[:]), "value": hex.EncodeToString(offerRaw)}}, "continuationToken": nil}
		case "contract_readData":
			if request.Params[1] == "pool/value-policy" {
				result = "0x0000000000000000"
				break
			}
			if request.Params[1] != "pool/order/1" {
				http.Error(w, "unexpected state key", http.StatusBadRequest)
				return
			}
			result = "0x" + hex.EncodeToString(orderRaw)
		case "bcn_blockAt":
			height := uint64(request.Params[0].(float64))
			root := ""
			switch height {
			case session.Inventory.Height:
				root = session.Inventory.RootHex
			case session.Verdict.Height:
				root = session.Verdict.RootHex
			default:
				http.Error(w, "unexpected header", http.StatusBadRequest)
				return
			}
			result = map[string]any{"height": height, "root": "0x" + root}
		case "bcn_feePerGas":
			result = "1"
		case "contract_estimateCall":
			call := request.Params[0].(map[string]any)
			if call["method"] != "confirm" || call["amount"] != "0" || call["from"] != "0x"+hex.EncodeToString(buyer[:]) {
				http.Error(w, "unexpected confirm estimate", http.StatusBadRequest)
				return
			}
			result = map[string]any{"success": true, "gasUsed": 1000}
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient()}
	message, err := preparePoolV7PrivateMessage(c, contract, 1, mustDecode(vector.SecretsCanonical), vector.Author)
	if err != nil {
		t.Fatalf("funded seller could not prepare private ciphertext: %v", err)
	}
	purchase := &preparedAction{
		Format: preparedPoolV7Buy, Version: 1, Operation: "buy", Network: NetworkNamePinned,
		DirectVersion: 7, Contract: contract, DirectOfferID: hex.EncodeToString(id[:]),
		Author: vector.Author, Buyer: hex.EncodeToString(buyer[:]), Epoch: vector.Epoch,
		CoveredFlips: 1, OrderNonce: 1, DeliveryKey: hex.EncodeToString(deliveryPub),
		InventoryCommitment: hex.EncodeToString(inventory[:]), KeyCommitment: hex.EncodeToString(keys[:]),
		Price: price.String(), Amount: new(big.Int).Add(price, bond).String(), DirectSession: session,
		Availability: &AvailabilityCheck{
			CheckedAt: time.Now().UTC().Format(time.RFC3339), Author: vector.Author, Epoch: vector.Epoch,
			Binding: bindingCommitted, CoveredFlips: 1, VerifiedFlips: 1,
			InventoryCommitment: hex.EncodeToString(inventory[:]),
			Blobs:               []RetainedFlip{{CID: vector.BlobCID, Hex: vector.Blob}},
		},
	}
	receipt, vault, err := verifyPoolV7PrivateDelivery(c, contract, 1, message, purchase, deliveryKey)
	if err != nil || receipt.UsableFlips != 1 || vault.Address != "0x"+vector.Author {
		t.Fatalf("buyer did not verify the original flip and committed keys: %v", err)
	}
	if err := signDirectOpened(receipt, deliveryKey); err != nil {
		t.Fatal(err)
	}
	// Exercise the file-based seller and buyer commands as a local private
	// handoff. Their outputs must preserve the signed receipt used by confirm.
	commandDir := t.TempDir()
	t.Setenv(PassphraseEnvVar, "local-v7-test-passphrase")
	sellerVaultPath := filepath.Join(commandDir, "seller-vault.age")
	buyerVaultPath := filepath.Join(commandDir, "buyer-vault.age")
	purchasePath := filepath.Join(commandDir, "purchase.json")
	deliveryKeyPath := filepath.Join(commandDir, "delivery.key")
	messagePath := filepath.Join(commandDir, "ciphertext.json")
	receivedPath := filepath.Join(commandDir, "received.json")
	vaultJSON, err := json.Marshal(vault)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteVault(sellerVaultPath, "local-v7-test-passphrase", vaultJSON); err != nil {
		t.Fatal(err)
	}
	buyArg, err := encodePoolV7Buy(id, deliveryPub)
	if err != nil {
		t.Fatal(err)
	}
	purchase.Calldata = hex.EncodeToString(buyArg)
	if err := writePrivateJSON(purchasePath, purchase); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateBytes(deliveryKeyPath, []byte(hex.EncodeToString(crypto.FromECDSA(deliveryKey)))); err != nil {
		t.Fatal(err)
	}
	active = author
	if err := directCommand(directCommandOptions{
		Mode: "direct-v7-dm", DirectVersion: 7, Network: NetworkNamePinned,
		Node: c, Contract: contract, CodeHash: codeHash, OrderID: 1,
		VaultPath: sellerVaultPath, OutPath: messagePath, OutExplicit: true,
	}); err != nil {
		t.Fatalf("seller could not write local v7 ciphertext: %v", err)
	}
	active = buyer
	if err := directCommand(directCommandOptions{
		Mode: "direct-v7-receive", DirectVersion: 7, Network: NetworkNamePinned,
		Node: c, Contract: contract, CodeHash: codeHash, OrderID: 1,
		MessagePath: messagePath, PurchasePath: purchasePath,
		DeliveryKeyPath: deliveryKeyPath, VaultPath: buyerVaultPath,
		OutPath: receivedPath, OutExplicit: true,
	}); err != nil {
		t.Fatalf("buyer could not verify local v7 ciphertext: %v", err)
	}
	var received DirectOpenedDelivery
	receivedJSON, err := os.ReadFile(receivedPath)
	if err != nil || decodeStrict(receivedJSON, &received) != nil || verifyDirectOpened(&received) != nil ||
		received.Contract != contract || received.OrderID != 1 || received.UsableFlips != 1 {
		t.Fatalf("buyer did not save a signed, exact v7 receipt: %v", err)
	}
	if _, err := preparePoolV7Confirm(c, contract, 1, &received); err != nil {
		t.Fatalf("file-based buyer receipt cannot authorize cooperative confirm: %v", err)
	}
	prepared, err := preparePoolV7Confirm(c, contract, 1, receipt)
	if err != nil {
		t.Fatal(err)
	}
	prepared.CodeHash = codeHash
	var envelope DirectDMEnvelope
	if err := json.Unmarshal([]byte(message), &envelope); err != nil {
		t.Fatal(err)
	}
	ciphertext, _ := hex.DecodeString(envelope.Ciphertext)
	digest := sha256.Sum256(ciphertext)
	want := binary.BigEndian.AppendUint64(nil, 1)
	want = append(want, digest[:]...)
	got, _ := hex.DecodeString(prepared.Calldata)
	if !bytes.Equal(got, want) || prepared.CiphertextSha256 != hex.EncodeToString(digest[:]) {
		t.Fatal("confirm did not bind the exact verified ciphertext")
	}
	if _, _, err := BuildTransaction(prepared, buyerKey, 1, vector.Epoch, big.NewInt(1_000_000), big.NewInt(0)); err == nil {
		t.Fatal("v7 confirmation signed without a fresh state validation")
	}
	if err := validatePreparedPoolV7Caller(c, prepared, hex.EncodeToString(buyer[:])); err != nil {
		t.Fatalf("verified cooperative confirmation refused: %v", err)
	}
	if _, _, err := BuildTransaction(prepared, buyerKey, 1, vector.Epoch, big.NewInt(1_000_000), big.NewInt(0)); err != nil {
		t.Fatalf("validated confirmation did not sign: %v", err)
	}
	directory := t.TempDir()
	receiptPath, preparedPath := filepath.Join(directory, "receipt.json"), filepath.Join(directory, "confirm.json")
	if err := writePrivateJSON(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	if err := preparePoolV7FromFiles(directCommandOptions{
		DirectVersion: 7, Node: c, Operation: "confirm", CodeHash: codeHash,
		OrderID: 1, DeliveryPath: receiptPath, PreparedPath: preparedPath, MaxFeeText: "1000000",
	}, contract); err != nil {
		t.Fatalf("typed v7 confirm preparation refused: %v", err)
	}
	rawPrepared, err := os.ReadFile(preparedPath)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded preparedAction
	if err := decodeStrict(rawPrepared, &reloaded); err != nil || reloaded.Calldata != prepared.Calldata || reloaded.Amount != "0" {
		t.Fatalf("typed v7 confirm did not preserve verified calldata: %v", err)
	}

	wrong := *purchase
	wrong.Availability = &AvailabilityCheck{}
	*wrong.Availability = *purchase.Availability
	wrong.Availability.Blobs = []RetainedFlip{{CID: vector.BlobCID, Hex: "00"}}
	if _, _, err := verifyPoolV7PrivateDelivery(c, contract, 1, message, &wrong, deliveryKey); err == nil {
		t.Fatal("receipt issued without the original retained flip")
	}
	wrong = *purchase
	wrong.DirectSession = nil
	if _, _, err := verifyPoolV7PrivateDelivery(c, contract, 1, message, &wrong, deliveryKey); err == nil {
		t.Fatal("receipt issued without a canonical session")
	}
	if _, _, err := verifyPoolV7PrivateDelivery(c, contract, 1, strings.Replace(message, purchase.Buyer, strings.Repeat("42", 20), 1), purchase, deliveryKey); err == nil {
		t.Fatal("private ciphertext accepted for a different funded buyer")
	}
	changed := *receipt
	changed.Ciphertext = strings.Repeat("00", directDMCiphertextBytes)
	if _, err := preparePoolV7Confirm(c, contract, 1, &changed); err == nil {
		t.Fatal("modified ciphertext reused the signed receipt")
	}
	orderRaw[181] = 3
	if _, err := preparePoolV7Confirm(c, contract, 1, receipt); err == nil {
		t.Fatal("refunded order accepted a cooperative confirmation")
	}
}

func TestDirectV7DeliveryOutputPreservesExistingMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.json")
	first := []byte("original ciphertext")
	if err := writeNewPoolV7PrivateFile(path, first); err != nil {
		t.Fatal(err)
	}
	if err := writeNewPoolV7PrivateFile(path, []byte("different ciphertext")); err == nil {
		t.Fatal("v7 delivery replaced an existing private output")
	}
	retained, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(retained, first) {
		t.Fatalf("the original private output was not preserved: %v", err)
	}
}
