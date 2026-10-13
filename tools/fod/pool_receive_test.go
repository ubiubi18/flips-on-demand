package main

// The buyer's pool receive path and the confirm call: the ciphertext the desk
// publishes has to open with the order's delivery key and hash to the bundle
// commitment, and the confirm call carries exactly the bytes the buyer checked,
// so a replaced pending payload cannot strand the payment (`AH-10`).

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

func TestSettledRevealRecoversWithoutDeskOrCiphertext(t *testing.T) {
	dir := t.TempDir()
	_, sellerVault, blobDir, public, private, testCID := buildSyntheticBundle(t, dir, "seller-pass")
	raw, err := ReadVault(sellerVault, "seller-pass")
	if err != nil {
		t.Fatal(err)
	}
	var payload VaultPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatal(err)
	}
	var bundle servedBundle
	for _, b := range served {
		bundle = b
	}
	secrets, err := revealSecrets(payload, bundle.Commitment)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(filepath.Join(blobDir, testCID))
	if err != nil {
		t.Fatal(err)
	}
	check := retainedTestCheck(blob)
	check.Author, check.Epoch = bundle.Author, bundle.Epoch
	check.InventoryCommitment = inventoryCommitment(check.Author, check.Epoch, []string{cidRawSha256(blob)})
	if err := os.RemoveAll(blobDir); err != nil {
		t.Fatal(err)
	}
	buyer, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	buyerAddr, _ := nodekeyAddressOf(buyer)
	keyHex := hex.EncodeToString(crypto.FromECDSAPub(&buyer.PublicKey))
	keyPath := filepath.Join(dir, "buyer.key")
	if err := crypto.SaveECDSA(keyPath, buyer); err != nil {
		t.Fatal(err)
	}
	order := poolOrderRecordFor(t, mustDecode(bundle.Commitment), mustDecode(buyerAddr), crypto.FromECDSAPub(&buyer.PublicKey), 7, 3)
	for _, junk := range []bool{false, true} {
		t.Run(fmt.Sprintf("junk-candidate-%t", junk), func(t *testing.T) {
			values := map[string][]byte{"porder/7": order, "preveal/7": secrets}
			if junk {
				values["pdeliv/7"] = []byte("unusable candidate")
			}
			vaultPath := filepath.Join(t.TempDir(), "buyer.age")
			c := registryStub(t, values) // Order/reveal state only; no desk or blob endpoint.
			opened, err := openPoolDeliveryFromRegistry(c, testContract, 7, keyPath, vaultPath, "buyer-pass")
			if err != nil {
				t.Fatal(err)
			}
			if opened.KeySource != "public-reveal" || opened.State != "settled" || opened.CiphertextHex != "" {
				t.Fatalf("incorrect recovery source: %s", opened.KeySource)
			}
			before, err := os.ReadFile(vaultPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := openPoolDeliveryFromRegistry(c, testContract, 7, keyPath, vaultPath, "buyer-pass"); err != nil {
				t.Fatal("restart recovery:", err)
			}
			after, _ := os.ReadFile(vaultPath)
			if !bytes.Equal(before, after) {
				t.Fatal("restart replaced existing keys")
			}
			p := &preparedAction{Bundle: bundle.Commitment, Contract: testContract, Nonce: 7, Buyer: buyerAddr, DeliveryKey: keyHex, Availability: check}
			listing := poolRecordBytes(mustDecode(check.Author), filled(0x44, 32), mustDecode(check.InventoryCommitment), 0, 1000, 0, 8, 1)
			prefix := filepath.Join(t.TempDir(), "opened.json")
			if err := verifyPurchasedContent(poolBuyStub(t, bundle.Commitment, listing, nil, values), p, opened, "buyer-pass", prefix); err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string][]byte{"public": public, "private": private} {
				got, err := os.ReadFile(prefix + "." + cidRawSha256(blob) + "." + name)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("unusable %s after serving loss: %v", name, err)
				}
			}
			if err := signOpenedDelivery(opened, buyer); err != nil {
				t.Fatal(err)
			}
			if err := writePrivateJSON(prefix, opened); err != nil {
				t.Fatal(err)
			}
			if _, err := readUsableDeliveryRecord(prefix, testContract, 7); err == nil {
				t.Fatal("public recovery offered another confirmation")
			}
		})
	}
	for _, tc := range []struct {
		name  string
		state byte
		data  []byte
	}{
		{"delivered-only", 2, secrets}, {"refunded", 4, secrets}, {"substituted-keys", 3, []byte("wrong keys")},
		{"empty-reveal", 3, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string][]byte{"porder/7": poolOrderRecordFor(t, mustDecode(bundle.Commitment), mustDecode(buyerAddr), crypto.FromECDSAPub(&buyer.PublicKey), 7, tc.state), "preveal/7": tc.data}
			path := filepath.Join(t.TempDir(), "refused.age")
			if _, err := openPoolDeliveryFromRegistry(registryStub(t, values), testContract, 7, keyPath, path, "buyer-pass"); err == nil {
				t.Fatal("invalid recovery accepted")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("refused recovery wrote a vault")
			}
		})
	}
}

func TestPurchasedBytesRemainUsableWithoutAnyContentServer(t *testing.T) {
	dir := t.TempDir()
	_, sellerVault, blobs, public, private, testCID := buildSyntheticBundle(t, dir, "seller-pass")
	raw, err := ReadVault(sellerVault, "seller-pass")
	if err != nil {
		t.Fatal(err)
	}
	var payload VaultPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatal(err)
	}
	var bundle servedBundle
	for _, b := range served {
		bundle = b
	}
	blob, err := os.ReadFile(filepath.Join(blobs, testCID))
	if err != nil {
		t.Fatal(err)
	}
	cid := cidRawSha256(blob)
	check := retainedTestCheck(blob)
	check.Author = strings.TrimPrefix(payload.Address, "0x")
	check.Epoch = uint16(payload.Epoch)
	check.InventoryCommitment = inventoryCommitment(check.Author, check.Epoch, []string{cid})
	check.CheckedAt = time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	buyer, _ := crypto.GenerateKey()
	buyerAddr, _ := nodekeyAddressOf(buyer)
	keyHex := hex.EncodeToString(crypto.FromECDSAPub(&buyer.PublicKey))
	cipher, err := deliveryCiphertextFor(bundle, keyHex)
	if err != nil {
		t.Fatal(err)
	}
	contract := testContract
	order := poolOrderRecordFor(t, mustDecode(bundle.Commitment), mustDecode(buyerAddr), crypto.FromECDSAPub(&buyer.PublicKey), 7, 2)
	keyPath := filepath.Join(dir, "delivery.key")
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(crypto.FromECDSA(buyer))), 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err := openPoolDeliveryFromRegistry(registryStub(t, map[string][]byte{"porder/7": order, "pdeliv/7": cipher}), contract, 7, keyPath, filepath.Join(dir, "buyer.age"), "buyer-pass")
	if err != nil {
		t.Fatal(err)
	}
	p := &preparedAction{Bundle: bundle.Commitment, Contract: contract, Nonce: 7, Buyer: buyerAddr, DeliveryKey: keyHex, Availability: check}
	before, _ := os.ReadFile(opened.VaultPath)
	if _, err := openPoolDeliveryFromRegistry(registryStub(t, map[string][]byte{"porder/7": order, "pdeliv/7": cipher}), contract, 7, keyPath, opened.VaultPath, "buyer-pass"); err != nil {
		t.Fatal("restart recovery could not reuse the existing keys:", err)
	}
	otherPayload, _ := testVaultPayload(t, 229)
	otherBundles, err := servedBundlesFromPayloads([]VaultPayload{otherPayload})
	if err != nil {
		t.Fatal(err)
	}
	var other servedBundle
	for _, b := range otherBundles {
		other = b
	}
	otherCipher, err := deliveryCiphertextFor(other, keyHex)
	if err != nil {
		t.Fatal(err)
	}
	otherOrder := poolOrderRecordFor(t, mustDecode(other.Commitment), mustDecode(buyerAddr), crypto.FromECDSAPub(&buyer.PublicKey), 8, 2)
	if _, err := openPoolDeliveryFromRegistry(registryStub(t, map[string][]byte{"porder/8": otherOrder, "pdeliv/8": otherCipher}), contract, 8, keyPath, opened.VaultPath, "buyer-pass"); err == nil {
		t.Fatal("second purchase overwrote the first purchase's vault")
	}
	after, _ := os.ReadFile(opened.VaultPath)
	if !bytes.Equal(before, after) {
		t.Fatal("restart or second purchase changed stored keys")
	}
	record := poolRecordBytes(mustDecode(check.Author), filled(0x44, 32), mustDecode(check.InventoryCommitment), 0, 1000, 0, 8, 1)
	storage := poolBuyStub(t, bundle.Commitment, record, nil, map[string][]byte{"porder/7": order})
	prefix := filepath.Join(dir, "opened.json")
	if err := verifyPurchasedContent(storage, p, opened, "buyer-pass", prefix); err != nil {
		t.Fatal(err)
	}
	if opened.UsableFlips != 1 {
		t.Fatal("the cached flip was not opened")
	}
	for name, want := range map[string][]byte{"public": public, "private": private} {
		got, err := os.ReadFile(prefix + "." + cid + "." + name)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("cached %s was not usable: %v", name, err)
		}
	}
	if err := signOpenedDelivery(opened, buyer); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateJSON(prefix, opened); err != nil {
		t.Fatal(err)
	}
	if _, _, err := confirmPayloadFromRecord(prefix, contract, 7); err != nil {
		t.Fatal(err)
	}

	confirm, err := prepareConfirmAction(contract, "lab", 7, opened.CiphertextHex)
	if err != nil {
		t.Fatal(err)
	}
	confirm.VerifiedDelivery = opened
	if _, _, err := BuildTransaction(confirm, buyer, 1, 228, big.NewInt(1), big.NewInt(0)); err == nil {
		t.Fatal("signed confirmation without canonical receipt validation")
	}
	if err := validatePreparedConfirm(storage, confirm, buyer); err != nil {
		t.Fatal("normal confirmation:", err)
	}
	if _, _, err := BuildTransaction(confirm, buyer, 1, 228, big.NewInt(1), big.NewInt(0)); err != nil {
		t.Fatal("verified confirmation:", err)
	}
	for _, attack := range []string{"ciphertext", "count", "order", "key"} {
		forged := *opened
		prepared := *confirm
		prepared.VerifiedDelivery = &forged
		switch attack {
		case "ciphertext":
			prepared.Calldata += "00"
		case "count":
			forged.UsableFlips++
		case "order":
			prepared.OfferID++
		case "key":
			attacker, _ := crypto.GenerateKey()
			forged.DeliveryKey = hex.EncodeToString(crypto.FromECDSAPub(&attacker.PublicKey))
			if err := signOpenedDelivery(&forged, attacker); err != nil {
				t.Fatal(err)
			}
		}
		if err := validatePreparedConfirm(storage, &prepared, buyer); err == nil {
			t.Fatal("confirmation accepted", attack)
		}
		if prepared.confirmValidated {
			t.Fatal("failed revalidation retained signing authority")
		}
	}
	opened.UsableFlips = 0
	if err := writePrivateJSON(prefix, opened); err != nil {
		t.Fatal(err)
	}
	if _, _, err := confirmPayloadFromRecord(prefix, contract, 7); err == nil {
		t.Fatal("keys-only receipt allowed normal confirmation")
	}
	p.Availability.Blobs[0].Hex = "00"
	if err := verifyPurchasedContent(storage, p, opened, "buyer-pass", ""); err == nil {
		t.Fatal("invalid cached bytes were usable")
	}
}

// poolOrderRecordFor builds the escrow's 150-byte order record for the tests.
func poolOrderRecordFor(t *testing.T, bundle, buyer, deliveryKey []byte, nonce uint64, state byte) []byte {
	t.Helper()
	if len(bundle) != 32 || len(buyer) != 20 || len(deliveryKey) != 65 {
		t.Fatalf("order parts are %d/%d/%d bytes", len(bundle), len(buyer), len(deliveryKey))
	}
	record := make([]byte, marketPoolOrderBytes)
	copy(record[0:32], bundle)
	copy(record[32:52], buyer)
	copy(record[52:117], deliveryKey)
	binary.BigEndian.PutUint64(record[125:133], 25_000)
	binary.BigEndian.PutUint64(record[133:141], 1000)
	binary.BigEndian.PutUint64(record[141:149], nonce)
	record[149] = state
	return record
}

func TestPoolReceiveOpensTheDeskDelivery(t *testing.T) {
	payload, _ := testVaultPayload(t, 228)
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatalf("index the vault: %v", err)
	}
	var bundle servedBundle
	for _, item := range served {
		bundle = item
	}
	buyer, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the buyer key: %v", err)
	}
	buyerAddress := crypto.PubkeyToAddress(buyer.PublicKey)
	deliveryKey := hex.EncodeToString(crypto.FromECDSAPub(&buyer.PublicKey))
	ciphertext, err := deliveryCiphertextFor(bundle, deliveryKey)
	if err != nil {
		t.Fatalf("build the delivery ciphertext: %v", err)
	}
	commitment, err := hex.DecodeString(bundle.Commitment)
	if err != nil {
		t.Fatalf("decode the commitment: %v", err)
	}
	order := poolOrderRecordFor(t, commitment, buyerAddress[:], crypto.FromECDSAPub(&buyer.PublicKey), 7, 2)
	contract := strings.Repeat("ab", 20)
	node := registryStub(t, map[string][]byte{
		"porder/7": order,
		"pdeliv/7": ciphertext,
	})

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "delivery.key")
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(crypto.FromECDSA(buyer))), 0o600); err != nil {
		t.Fatalf("write the delivery key: %v", err)
	}
	vaultPath := filepath.Join(dir, "bundle.age")
	opened, err := openPoolDeliveryFromRegistry(node, contract, 7, keyPath, vaultPath, "passphrase")
	if err != nil {
		t.Fatalf("open the pool delivery: %v", err)
	}
	if opened.OrderID != 7 || opened.OrderNonce != 7 || opened.State != "delivered" {
		t.Fatalf("the opened order reads as %+v", opened)
	}
	if opened.Epoch != 228 || !strings.EqualFold(opened.Author, "0x"+hex.EncodeToString(bundleBytes(t, bundle.Author))) {
		t.Fatalf("the opened bundle names epoch %d author %s", opened.Epoch, opened.Author)
	}
	// Older settled private deliveries have no public-reveal state. Only an
	// explicit absent-record answer may select this compatible recovery path.
	settledOrder := poolOrderRecordFor(t, commitment, buyerAddress[:], crypto.FromECDSAPub(&buyer.PublicKey), 7, 3)
	legacyNode := registryStub(t, map[string][]byte{"porder/7": settledOrder, "pdeliv/7": ciphertext})
	legacy, err := openPoolDeliveryFromRegistry(legacyNode, contract, 7, keyPath, vaultPath, "passphrase")
	if err != nil || legacy.KeySource != "" || legacy.State != "settled" {
		t.Fatalf("legacy settled ciphertext recovery failed: %v", err)
	}
	if !strings.EqualFold(opened.SecretsSha256, bundle.Commitment) {
		t.Fatalf("the opened secrets hash to %s, the bundle commits to %s", opened.SecretsSha256, bundle.Commitment)
	}
	if len(opened.Checks) < 4 {
		t.Fatalf("the opened delivery lists %d checks", len(opened.Checks))
	}
	storedRaw, err := ReadVault(vaultPath, "passphrase")
	if err != nil {
		t.Fatalf("read the written vault: %v", err)
	}
	var stored VaultPayload
	if err := json.Unmarshal(storedRaw, &stored); err != nil {
		t.Fatalf("parse the written vault: %v", err)
	}
	if stored.Kind != "fod.epoch-keys" || stored.Version != 1 || stored.Epoch != 228 {
		t.Fatalf("the written vault reads as %+v", stored)
	}

	// Another key is refused before anything is decrypted.
	unrelated, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate an unrelated key: %v", err)
	}
	otherKeyPath := filepath.Join(dir, "other.key")
	if err := os.WriteFile(otherKeyPath, []byte(hex.EncodeToString(crypto.FromECDSA(unrelated))), 0o600); err != nil {
		t.Fatalf("write the unrelated key: %v", err)
	}
	if _, err := openPoolDeliveryFromRegistry(node, contract, 7, otherKeyPath, filepath.Join(dir, "other.age"), "passphrase"); err == nil || !strings.Contains(err.Error(), "not for this key") {
		t.Fatalf("an unrelated delivery key was accepted: %v", err)
	}

	// A ciphertext for another bundle opens but does not hash to this bundle's
	// commitment, so it is refused.
	otherPayload, _ := testVaultPayload(t, 229)
	otherServed, err := servedBundlesFromPayloads([]VaultPayload{otherPayload})
	if err != nil {
		t.Fatalf("index the other vault: %v", err)
	}
	var otherBundle servedBundle
	for _, item := range otherServed {
		otherBundle = item
	}
	otherCiphertext, err := deliveryCiphertextFor(otherBundle, deliveryKey)
	if err != nil {
		t.Fatalf("build the other ciphertext: %v", err)
	}
	otherNode := registryStub(t, map[string][]byte{
		"porder/7": order,
		"pdeliv/7": otherCiphertext,
	})
	if _, err := openPoolDeliveryFromRegistry(otherNode, contract, 7, keyPath, filepath.Join(dir, "other-bundle.age"), "passphrase"); err == nil || !strings.Contains(err.Error(), "refusing to keep them") {
		t.Fatalf("a delivery for another bundle was accepted: %v", err)
	}

	// An order that is only funded has no delivery to open yet.
	funded := poolOrderRecordFor(t, commitment, buyerAddress[:], crypto.FromECDSAPub(&buyer.PublicKey), 7, 1)
	fundedNode := registryStub(t, map[string][]byte{"porder/7": funded})
	if _, err := openPoolDeliveryFromRegistry(fundedNode, contract, 7, keyPath, filepath.Join(dir, "funded.age"), "passphrase"); err == nil || !strings.Contains(err.Error(), "no delivery has been published yet") {
		t.Fatalf("a funded order without a delivery was accepted: %v", err)
	}
}

func TestPrepareConfirmCarriesTheCheckedCiphertext(t *testing.T) {
	contract := strings.Repeat("ab", 20)
	ciphertext := []byte("the delivered ciphertext this buyer checked")
	digest := sha256.Sum256(ciphertext)
	for _, orderID := range []uint64{7, 9_007_199_254_740_992} {
		prepared, err := prepareConfirmAction(contract, "testnet", orderID, hex.EncodeToString(ciphertext))
		if err != nil {
			t.Fatalf("prepare the confirm action: %v", err)
		}
		if prepared.Format != preparedConfirmFormat || prepared.Operation != operationConfirm {
			t.Fatalf("the confirm action is %s / %s", prepared.Format, prepared.Operation)
		}
		calldata, err := hex.DecodeString(prepared.Calldata)
		if err != nil {
			t.Fatalf("decode the confirm calldata: %v", err)
		}
		if len(calldata) != 8+len(ciphertext) {
			t.Fatalf("the confirm calldata is %d bytes, expected %d", len(calldata), 8+len(ciphertext))
		}
		if binary.BigEndian.Uint64(calldata[:8]) != orderID {
			t.Fatalf("the calldata names order %d, expected %d", binary.BigEndian.Uint64(calldata[:8]), orderID)
		}
		if !bytes.Equal(calldata[8:], ciphertext) {
			t.Fatal("the calldata does not carry the ciphertext it was given")
		}
		path := filepath.Join(t.TempDir(), "confirm.json")
		if err := writeJSON(path, prepared); err != nil {
			t.Fatalf("write the confirm action: %v", err)
		}
		loaded, err := loadPreparedAction(path)
		if err != nil {
			t.Fatalf("load the confirm action: %v", err)
		}
		if loaded.Operation != operationConfirm || loaded.OfferID != orderID ||
			loaded.CiphertextSha256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("the loaded confirm action is %+v", loaded)
		}
	}

	// A confirmation without the checked bytes is refused: there would be
	// nothing to settle on.
	if _, err := prepareConfirmAction(contract, "testnet", 7, ""); err == nil {
		t.Fatal("a confirm call without a ciphertext was built")
	}
}

func TestLoadConfirmRequestNamesTheOrder(t *testing.T) {
	bundle := strings.Repeat("ab", poolBundleBytes)
	ciphertext := strings.Repeat("71", 8)
	digestBytes := sha256.Sum256(mustDecode(ciphertext))
	digest := hex.EncodeToString(digestBytes[:])
	body := `{
  "format": "fod.action-request",
  "version": 1,
  "network": "idena-local",
  "contract": "` + strings.Repeat("11", addressBytes) + `",
  "operation": "confirm",
  "arguments": {"bundleId": "` + bundle + `", "orderId": 7, "ciphertextSha256": "` + digest + `", "ciphertextHex": "` + ciphertext + `"},
  "amount": "0",
  "createdAt": "2026-09-26T12:00:00Z"
}`
	requestPath := writeRequest(t, t.TempDir(), body)
	request, arguments, err := loadConfirmRequest(requestPath)
	if err != nil {
		t.Fatalf("load the confirm request: %v", err)
	}
	if request.Operation != operationConfirm || arguments.OrderID != 7 || arguments.BundleID != bundle ||
		arguments.CiphertextSha256 != digest || arguments.CiphertextHex != ciphertext {
		t.Fatalf("the confirm request reads as %s / %+v", request.Operation, arguments)
	}
	badPath := writeRequest(t, t.TempDir(), strings.Replace(body, `"orderId": 7`, `"orderId": 0`, 1))
	if _, _, err := loadConfirmRequest(badPath); err == nil {
		t.Fatal("a confirm request without an order id was accepted")
	}
	// A request that names no digest would settle whatever the chain holds when
	// it is signed, so it is refused rather than defaulted.
	noDigest := writeRequest(t, t.TempDir(), strings.Replace(body, `, "ciphertextSha256": "`+digest+`"`, "", 1))
	if _, _, err := loadConfirmRequest(noDigest); err == nil {
		t.Fatal("a confirm request without a ciphertext digest was accepted")
	}
	// Bytes that do not hash to the named digest are refused: the pair has to
	// describe one payload.
	mismatch := writeRequest(t, t.TempDir(), strings.Replace(body, `"ciphertextHex": "`+ciphertext+`"`, `"ciphertextHex": "72"`, 1))
	if _, _, err := loadConfirmRequest(mismatch); err == nil {
		t.Fatal("a confirm request whose bytes do not match its digest was accepted")
	}
}

// TestPoolConfirmAndRefundSignedSizes pins the fee table's sizes for the two
// small calls the buyer's trades view drives.
func TestPoolConfirmAndRefundSignedSizes(t *testing.T) {
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the signing key: %v", err)
	}
	contract := strings.Repeat("aa", addressBytes)
	// The reference confirm payload is the 219-byte ciphertext the lab's
	// deliveries carried; the calldata is the order id plus exactly those bytes.
	confirmCiphertext := bytes.Repeat([]byte{0x71}, 219)
	confirmAction, err := prepareConfirmAction(contract, "testnet", 7, hex.EncodeToString(confirmCiphertext))
	if err != nil {
		t.Fatalf("prepare the confirm action: %v", err)
	}
	// Fee-size fixture only: this private flag cannot be supplied by a JSON file.
	confirmAction.confirmValidated = true
	cases := []struct {
		operation string
		prepared  *preparedAction
	}{
		{operationConfirm, confirmAction},
		{
			operationRefund,
			PreparePoolRefund(&actionRequest{Network: "testnet", Contract: contract}, &refundArguments{OrderID: 7}),
		},
	}
	for _, c := range cases {
		raw, _, err := BuildTransaction(c.prepared, identity, 1, 228, big.NewInt(1_000_000_000_000_000), big.NewInt(0))
		if err != nil {
			t.Errorf("%s: sign: %v", c.operation, err)
			continue
		}
		gas, ok := measuredGasFor(c.operation)
		if !ok {
			t.Errorf("%s has no measured gas", c.operation)
			continue
		}
		if gas.TxBytes != uint64(len(raw)) {
			t.Errorf("%s signs %d bytes, the fee table records %d", c.operation, len(raw), gas.TxBytes)
		}
	}
}

// bundleBytes decodes the author hex the served bundle carries.
func bundleBytes(t *testing.T, author string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimPrefix(author, "0x"))
	if err != nil {
		t.Fatalf("decode the author: %v", err)
	}
	return raw
}
