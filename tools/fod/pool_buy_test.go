package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func decodeHex(t *testing.T, text string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(text)
	if err != nil {
		t.Fatalf("decode %q: %v", text, err)
	}
	return raw
}

// TestPreparePoolBuySignsThePossessionProof checks the call the pool's `buy`
// takes: the bundle, the delivery key, and a signature the contract can
// recover, bound to the bundle, the buy nonce, and the caller.
func TestPreparePoolBuySignsThePossessionProof(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate delivery key: %v", err)
	}
	contract := strings.Repeat("11", addressBytes)
	bundle := strings.Repeat("ab", poolBundleBytes)
	buyer := strings.Repeat("22", addressBytes)

	artefact, err := preparePoolBuy(contract, bundle, buyer, 7, "25000", "idena-local", nil, 0, key)
	if err != nil {
		t.Fatalf("prepare the pool buy: %v", err)
	}
	if artefact.Format != "fod.prepared-buy" || artefact.Operation != "buy" {
		t.Fatalf("unexpected artefact: %s / %s", artefact.Format, artefact.Operation)
	}
	calldata := decodeHex(t, artefact.Calldata)
	// The calldata ends with the two-byte retrieval-fee count (no hosts here).
	if len(calldata) != poolBundleBytes+deliveryKeyBytes+signatureBytes+2 {
		t.Fatalf("the calldata is %d bytes", len(calldata))
	}
	if calldata[poolBundleBytes+deliveryKeyBytes+signatureBytes] != 0 ||
		calldata[poolBundleBytes+deliveryKeyBytes+signatureBytes+1] != 0 {
		t.Fatal("the calldata's retrieval-fee count is not zero")
	}
	publicKey := crypto.FromECDSAPub(&key.PublicKey)
	if !bytes.Equal(calldata[:poolBundleBytes], decodeHex(t, bundle)) {
		t.Fatal("the calldata does not start with the bundle")
	}
	if !bytes.Equal(calldata[poolBundleBytes:poolBundleBytes+deliveryKeyBytes], publicKey) {
		t.Fatal("the calldata does not carry the delivery key")
	}

	message := poolBuyMessage(
		decodeHex(t, contract),
		decodeHex(t, bundle),
		decodeHex(t, buyer),
		publicKey,
		7,
	)
	signature := calldata[poolBundleBytes+deliveryKeyBytes : poolBundleBytes+deliveryKeyBytes+signatureBytes]
	recovered, err := crypto.SigToPub(crypto.Keccak256(message), signature)
	if err != nil {
		t.Fatalf("recover the signer: %v", err)
	}
	if !bytes.Equal(crypto.FromECDSAPub(recovered), publicKey) {
		t.Fatal("the signature does not recover to the delivery key")
	}

	// Another nonce, another caller, and another bundle must each break it.
	for name, other := range map[string][]byte{
		"nonce":  poolBuyMessage(decodeHex(t, contract), decodeHex(t, bundle), decodeHex(t, buyer), publicKey, 8),
		"caller": poolBuyMessage(decodeHex(t, contract), decodeHex(t, bundle), decodeHex(t, strings.Repeat("33", addressBytes)), publicKey, 7),
		"bundle": poolBuyMessage(decodeHex(t, contract), decodeHex(t, strings.Repeat("cd", poolBundleBytes)), decodeHex(t, buyer), publicKey, 7),
	} {
		recovered, err := crypto.SigToPub(crypto.Keccak256(other), signature)
		if err == nil && bytes.Equal(crypto.FromECDSAPub(recovered), publicKey) {
			t.Fatalf("the proof is not bound to the %s", name)
		}
	}
}

// TestPreparePoolBuyRoundTripsThroughSubmit is the regression for REVIEW.md
// A07: the producer wrote `bundle`, `nonce` and `note`, and the submit reader's
// strict decoder did not declare them, so the CLI rejected its own file with
// `unknown field "bundle"` before any key was read or any transaction signed.
func TestPreparePoolBuyRoundTripsThroughSubmit(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate delivery key: %v", err)
	}
	contract := strings.Repeat("11", addressBytes)
	bundle := strings.Repeat("ab", poolBundleBytes)
	buyer := strings.Repeat("22", addressBytes)

	artefact, err := preparePoolBuy(contract, bundle, buyer, 7, "25000", "idena-local", nil, 0, key)
	if err != nil {
		t.Fatalf("prepare the pool buy: %v", err)
	}
	// The ordinary buy path carries the buyer's own recent content check; the
	// signing side refuses a file without one (`AH-17`).
	artefact.Availability = retainedTestCheck([]byte("cached encrypted flip"))
	out := filepath.Join(t.TempDir(), "prepared-buy.json")
	if err := writeJSON(out, artefact); err != nil {
		t.Fatalf("write the prepared buy: %v", err)
	}
	loaded, err := loadPreparedAction(out)
	if err != nil {
		t.Fatalf("load the prepared buy through the submit reader: %v", err)
	}
	if loaded.Format != "fod.prepared-buy" || loaded.Operation != operationBuy {
		t.Fatalf("the loaded buy is %s / %s", loaded.Format, loaded.Operation)
	}
	if loaded.Bundle != bundle || loaded.Nonce != 7 || loaded.Calldata != artefact.Calldata {
		t.Fatalf("the loaded buy lost its terms: bundle %s, nonce %d, calldata %s",
			loaded.Bundle, loaded.Nonce, loaded.Calldata)
	}
}

func TestPreparePoolBuyRefusesBadInputs(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate delivery key: %v", err)
	}
	contract := strings.Repeat("11", addressBytes)
	buyer := strings.Repeat("22", addressBytes)
	for name, call := range map[string]func() (*PoolBuy, error){
		"a short bundle": func() (*PoolBuy, error) {
			return preparePoolBuy(contract, "abcd", buyer, 0, "1", "idena-local", nil, 0, key)
		},
		"a bad amount": func() (*PoolBuy, error) {
			return preparePoolBuy(contract, strings.Repeat("ab", poolBundleBytes), buyer, 0, "12x", "idena-local", nil, 0, key)
		},
		"a short contract": func() (*PoolBuy, error) {
			return preparePoolBuy("11", strings.Repeat("ab", poolBundleBytes), buyer, 0, "1", "idena-local", nil, 0, key)
		},
		"no delivery key": func() (*PoolBuy, error) {
			return preparePoolBuy(contract, strings.Repeat("ab", poolBundleBytes), buyer, 0, "1", "idena-local", nil, 0, nil)
		},
	} {
		if _, err := call(); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

// poolRegistryStub answers the pool prefix walk with one record, so the
// buy-request path can recheck the chain without a live node.
func poolRegistryStub(t *testing.T, commitment string, record []byte) *client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch request.Method {
		case "contract_iterateMap":
			if len(request.Params) > 1 && (request.Params[1] == "popen/" || request.Params[1] == "pfast/") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"result": map[string]any{"items": []any{}, "continuationToken": "0x"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"items": []map[string]string{{
						"key":   commitment,
						"value": hex.EncodeToString(record),
					}},
					"continuationToken": "0x",
				},
			})
		case "contract_readData":
			key, _ := request.Params[1].(string)
			value, ok := poolStubCounters(key)
			if !ok {
				http.Error(w, "unexpected key "+key, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": value})
		default:
			http.Error(w, "unexpected method "+request.Method, http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	return &client{url: server.URL, http: newHTTPClient()}
}

// availableBuyFixture builds the smallest honest request-driven buy: one
// committed flip whose blob the buyer's node serves, and the advisory the
// mandatory availability check repeats (`AH-17`).
func availableBuyFixture(t *testing.T, stored ...map[string][]byte) (*client, string, string) {
	t.Helper()
	bundle := strings.Repeat("ab", poolBundleBytes)
	author := strings.Repeat("33", addressBytes)
	blob := []byte("the ciphertext this fixture serves for the committed flip")
	cid := cidRawSha256(blob)
	// The registration guest commits the CID as the multibase text the node
	// writes, so the record the buyer checks is built the same way.
	commitment := inventoryCommitment(author, 228, []string{cid})
	record := poolRecordBytes(filled(0x33, 20), filled(0x44, 32), mustDecode(commitment), 3, 1000, 0, 7, 1)
	node := poolBuyStub(t, bundle, record, map[string][]byte{cid: blob}, stored...)
	advisoryPath := filepath.Join(t.TempDir(), "advisory.json")
	if err := writeJSON(advisoryPath, &AvailabilityAdvisory{
		Format:        availabilityFormat,
		Version:       availabilityVersion,
		Author:        author,
		Epoch:         228,
		CheckedAt:     "2026-09-26T12:00:00Z",
		CoveredFlips:  1,
		VerifiedFlips: 1,
		Complete:      true,
		Flips: []AvailabilityFlip{
			{CID: cid, Verified: true, CheckedAt: "2026-09-26T12:00:00Z", Source: "seller's node"},
		},
	}); err != nil {
		t.Fatalf("write the advisory: %v", err)
	}
	return node, bundle, advisoryPath
}

// TestTheInventoryCommitmentCommitsTheCanonicalCIDText pins the convention the
// registration guest hashes: the domain, the author's lower-case hex, the
// epoch, then each CID as the multibase text the node writes. The live lab's
// first bundle held 3f56c06e… for this CID; a commitment taken over the hex of
// the decoded bytes instead gave d949a69b…, which the chain refused.
func TestTheInventoryCommitmentCommitsTheCanonicalCIDText(t *testing.T) {
	const (
		author = "19e7e376e7c213b7e7e7e46cc70a5dd086daff2a"
		cid    = "bafkreie7fhwchauzjipbivbybng3mgcpfiiqhl74pvwpa627jhkt2sajmm"
	)
	if _, err := cidBytesFromString(cid); err != nil {
		t.Fatalf("the pinned CID does not parse: %v", err)
	}
	hasher := sha256.New()
	hasher.Write([]byte("fod.bundle-manifest:v1"))
	hasher.Write([]byte(author))
	hasher.Write([]byte{0x00, 0x01}) // epoch 1, big endian
	hasher.Write([]byte{0x00})
	hasher.Write([]byte(cid))
	expected := hex.EncodeToString(hasher.Sum(nil))
	// The value the registration guest wrote into the live lab's receipt.
	if expected != "3f56c06e381d9bb9f215433fed03260a1c8b72dab9680dfdc2fd4a5798a0f237" {
		t.Fatalf("the pinned vector itself changed: %s", expected)
	}
	if got := inventoryCommitment(author, 1, []string{cid}); !strings.EqualFold(got, expected) {
		t.Fatalf("inventoryCommitment committed %s, and the CID text commits to %s", got, expected)
	}
	decoded, err := cidBytesFromString(cid)
	if err != nil {
		t.Fatalf("decode the pinned CID: %v", err)
	}
	if other := inventoryCommitment(author, 1, []string{hex.EncodeToString(decoded)}); strings.EqualFold(other, expected) {
		t.Fatal("a commitment over the decoded bytes collides with the chain's value")
	}
}

// TestPreparePoolBuyFromTheInterfaceRequest: the request supplies the buyer's
// intent, and the chain supplies the nonce and the quote. A stale quote and
// another buyer's request are refused before anything is signed.
func TestPreparePoolBuyFromTheInterfaceRequest(t *testing.T) {
	node, bundle, advisoryPath := availableBuyFixture(t)

	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the buyer key: %v", err)
	}
	buyer := crypto.PubkeyToAddress(identity.PublicKey)
	deliveryKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the delivery key: %v", err)
	}
	request := &actionRequest{Network: "idena-local", Contract: testContract}
	arguments := &buyRequestArguments{
		BundleID:      bundle,
		ExpectedQuote: "25000",
		Buyer:         hex.EncodeToString(buyer.Bytes()),
	}
	artefact, err := preparePoolBuyFromRequest(node, request, arguments, identity, deliveryKey, advisoryPath)
	if err != nil {
		t.Fatalf("prepare the request's buy: %v", err)
	}
	if artefact.Nonce != 7 || artefact.Amount != "25000" {
		t.Fatalf("the prepared buy is nonce %d for %s", artefact.Nonce, artefact.Amount)
	}
	if !strings.EqualFold(artefact.Buyer, arguments.Buyer) || artefact.Bundle != bundle {
		t.Fatalf("the prepared buy names %s / %s", artefact.Buyer, artefact.Bundle)
	}
	// The advisory is the ordinary path's precondition, not an option.
	if _, err := preparePoolBuyFromRequest(node, request, arguments, identity, deliveryKey, ""); err == nil {
		t.Fatal("a purchase without an availability advisory was prepared")
	}

	moved := *arguments
	moved.ExpectedQuote = "24000"
	if _, err := preparePoolBuyFromRequest(node, request, &moved, identity, deliveryKey, advisoryPath); err == nil {
		t.Fatal("a stale quote was accepted")
	}
	other, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate another key: %v", err)
	}
	wrong := *arguments
	wrong.Buyer = hex.EncodeToString(crypto.PubkeyToAddress(other.PublicKey).Bytes())
	if _, err := preparePoolBuyFromRequest(node, request, &wrong, identity, deliveryKey, advisoryPath); err == nil {
		t.Fatal("another buyer's request was accepted")
	}
}

func TestOrdinaryBuyerRefusesFastBundleBeforePreparationAndSigning(t *testing.T) {
	ordinaryNode, bundle, advisoryPath := availableBuyFixture(t)
	fastNode, _, _ := availableBuyFixture(t, map[string][]byte{
		"pfast/" + bundle: make([]byte, 105),
	})
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	deliveryKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	buyer, err := nodekeyAddressOf(identity)
	if err != nil {
		t.Fatal(err)
	}
	request := &actionRequest{Network: "idena-local", Contract: testContract}
	arguments := &buyRequestArguments{BundleID: bundle, ExpectedQuote: "25000", Buyer: buyer}

	prepared, err := preparePoolBuyFromRequest(ordinaryNode, request, arguments, identity, deliveryKey, advisoryPath)
	if err != nil {
		t.Fatalf("ordinary bundle should remain purchasable: %v", err)
	}
	file := filepath.Join(t.TempDir(), "prepared-buy.json")
	if err := writePrivateJSON(file, prepared); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadPreparedAction(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePreparedBuy(ordinaryNode, loaded); err != nil {
		t.Fatalf("ordinary prepared buy should remain signable: %v", err)
	}

	if _, err := preparePoolBuyFromRequest(fastNode, request, arguments, identity, deliveryKey, advisoryPath); err == nil {
		t.Fatal("fast bundle was prepared from an ordinary availability advisory")
	}
	if err := validatePreparedBuy(fastNode, loaded); err == nil || !strings.Contains(err.Error(), "fast purchase has no retained copy evidence") {
		t.Fatalf("fast bundle was accepted without copy evidence: %v", err)
	}
	if loaded.buyValidated {
		t.Fatal("a previously validated buy remained signable after the fast bundle check failed")
	}
}

// poolBuyStub answers the two calls a request-driven buy makes: the registry
// read that prices the bundle, and the node's IPFS, which serves the blobs the
// buyer's own availability check fetches.
func poolBuyStub(t *testing.T, commitment string, record []byte, blobs map[string][]byte, stored ...map[string][]byte) *client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch request.Method {
		case "contract_iterateMap":
			var prefix string
			if len(request.Params) > 1 {
				_ = json.Unmarshal(request.Params[1], &prefix)
			}
			if prefix == "popen/" {
				items := []map[string]string{}
				for _, values := range stored {
					if raw, ok := values["popen/"+commitment]; ok {
						items = append(items, map[string]string{
							"key": commitment, "value": hex.EncodeToString(raw),
						})
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"result": map[string]any{"items": items, "continuationToken": "0x"},
				})
				return
			}
			if prefix == "pfast/" {
				items := []map[string]string{}
				for _, values := range stored {
					if raw, ok := values["pfast/"+commitment]; ok {
						items = append(items, map[string]string{
							"key": commitment, "value": hex.EncodeToString(raw),
						})
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"result": map[string]any{"items": items, "continuationToken": "0x"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": map[string]any{
					"items": []map[string]string{{
						"key":   commitment,
						"value": hex.EncodeToString(record),
					}},
					"continuationToken": "0x",
				},
			})
		case "contract_readData":
			var key string
			if len(request.Params) > 1 {
				_ = json.Unmarshal(request.Params[1], &key)
			}
			value, ok := poolStubCounters(key)
			for _, values := range stored {
				if bytes, exists := values[key]; exists {
					value, ok = "0x"+hex.EncodeToString(bytes), true
				}
			}
			if !ok {
				http.Error(w, "unexpected key "+key, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": value})
		case "ipfs_get":
			var cid string
			if len(request.Params) > 0 {
				_ = json.Unmarshal(request.Params[0], &cid)
			}
			blob, ok := blobs[cid]
			if !ok {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{"code": -32000, "message": "not found"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result": "0x" + hex.EncodeToString(blob),
			})
		default:
			http.Error(w, "unexpected method "+request.Method, http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	return &client{url: server.URL, http: newHTTPClient()}
}

func TestPreparePoolBuyRefusesAFullAtomicRevealSet(t *testing.T) {
	bundle := strings.Repeat("ab", poolBundleBytes)
	open := make([]byte, 8*maxOpenPoolOrders)
	for index := 0; index < maxOpenPoolOrders; index++ {
		binary.BigEndian.PutUint64(open[index*8:(index+1)*8], uint64(index+1))
	}
	node, _, advisoryPath := availableBuyFixture(t, map[string][]byte{"popen/" + bundle: open})
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	deliveryKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	request := &actionRequest{Network: "idena-local", Contract: testContract}
	arguments := &buyRequestArguments{
		BundleID: bundle, ExpectedQuote: "25000",
		Buyer: hex.EncodeToString(crypto.PubkeyToAddress(identity.PublicKey).Bytes()),
	}
	if _, err := preparePoolBuyFromRequest(node, request, arguments, identity, deliveryKey, advisoryPath); err == nil || !strings.Contains(err.Error(), "8 open purchases") {
		t.Fatalf("a full bundle prepared a purchase: %v", err)
	}
}

// TestPreparePoolBuyRepeatsTheAvailabilityCheck: the seller's advisory is a
// claim. Before the buy call is built, prepare-buy fetches every CID the
// advisory names from the buyer's own node, rebuilds each CID from the bytes,
// and refuses a purchase whose content it cannot fetch (`SPEC.md` 5.8,
// `REVIEW.md` A08). The check it did is recorded with its own time.
func TestPreparePoolBuyRepeatsTheAvailabilityCheck(t *testing.T) {
	bundle := strings.Repeat("ab", poolBundleBytes)
	author := strings.Repeat("33", addressBytes)

	// Two committed flips, each published under its own CID, and one CID the
	// bundle never committed: enough to tell an honest set from a substituted
	// one.
	firstBlob := []byte("the ciphertext the seller published for the first flip")
	secondBlob := []byte("the ciphertext the seller published for the second flip")
	firstCID := cidRawSha256(firstBlob)
	secondCID := cidRawSha256(secondBlob)
	uncommittedCID := cidRawSha256([]byte("a flip this bundle never committed"))
	// The record carries the commitment the registration statement published:
	// the registration guest hashes each CID as the multibase text the node
	// writes, so the fixture commits the CIDs themselves.
	commitment := inventoryCommitment(author, 228, []string{firstCID, secondCID})
	record := poolRecordBytes(filled(0x33, 20), filled(0x44, 32), mustDecode(commitment), 3, 1000, 0, 7, 2)
	node := poolBuyStub(t, bundle, record, map[string][]byte{
		firstCID:       firstBlob,
		secondCID:      secondBlob,
		uncommittedCID: []byte("bytes this bundle's statement never committed"),
	})

	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the buyer key: %v", err)
	}
	buyer := crypto.PubkeyToAddress(identity.PublicKey)
	deliveryKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the delivery key: %v", err)
	}
	request := &actionRequest{Network: "idena-local", Contract: testContract}
	arguments := &buyRequestArguments{
		BundleID:      bundle,
		ExpectedQuote: "25000",
		Buyer:         hex.EncodeToString(buyer.Bytes()),
	}
	advisoryPath := filepath.Join(t.TempDir(), "advisory.json")
	advisory := &AvailabilityAdvisory{
		Format:        availabilityFormat,
		Version:       availabilityVersion,
		Author:        author,
		Epoch:         228,
		CheckedAt:     "2026-09-26T12:00:00Z",
		CoveredFlips:  2,
		VerifiedFlips: 2,
		Complete:      true,
		Flips: []AvailabilityFlip{
			{CID: firstCID, Verified: true, CheckedAt: "2026-09-26T12:00:00Z", Source: "seller's node"},
			{CID: secondCID, Verified: true, CheckedAt: "2026-09-26T12:00:00Z", Source: "seller's node"},
		},
	}
	write := func(candidate *AvailabilityAdvisory) {
		t.Helper()
		if err := writeJSON(advisoryPath, candidate); err != nil {
			t.Fatalf("write the advisory: %v", err)
		}
	}
	write(advisory)

	artefact, err := preparePoolBuyFromRequest(node, request, arguments, identity, deliveryKey, advisoryPath)
	if err != nil {
		t.Fatalf("prepare with the advisory: %v", err)
	}
	if artefact.Availability == nil {
		t.Fatal("the prepared buy records no availability check")
	}
	if !artefact.Availability.Complete() || artefact.Availability.VerifiedFlips != 2 || artefact.Availability.CoveredFlips != 2 {
		t.Fatalf("the buyer's check reads %+v", artefact.Availability)
	}
	if artefact.Availability.CheckedAt == "" || artefact.Availability.CheckedAt == advisory.CheckedAt {
		t.Fatalf("the check carries the advisory's time instead of its own: %q", artefact.Availability.CheckedAt)
	}
	// The record binds the CIDs to the chain: the set recomputes to the
	// inventory commitment the registration statement published.
	if artefact.Availability.Binding != bindingCommitted {
		t.Fatalf("the check binds the content as %q", artefact.Availability.Binding)
	}

	// Every way of leaving the committed set is refused on the commitment
	// alone, before anything is fetched: an omitted flip, a substituted one, a
	// repeated one, another epoch, and another author.
	variants := []struct {
		name string
		make func() *AvailabilityAdvisory
	}{
		{"omits a committed flip", func() *AvailabilityAdvisory {
			short := *advisory
			short.Flips = advisory.Flips[:1]
			short.CoveredFlips, short.VerifiedFlips = 1, 1
			return &short
		}},
		{"adds a flip the bundle never committed", func() *AvailabilityAdvisory {
			long := *advisory
			long.Flips = append(append([]AvailabilityFlip{}, advisory.Flips...), AvailabilityFlip{
				CID: uncommittedCID, Verified: true,
			})
			long.CoveredFlips, long.VerifiedFlips = 3, 3
			return &long
		}},
		{"names one flip twice", func() *AvailabilityAdvisory {
			duplicated := *advisory
			duplicated.Flips = append(append([]AvailabilityFlip{}, advisory.Flips...), advisory.Flips[0])
			duplicated.CoveredFlips, duplicated.VerifiedFlips = 3, 3
			return &duplicated
		}},
		{"belongs to another epoch", func() *AvailabilityAdvisory {
			otherEpoch := *advisory
			otherEpoch.Epoch = 229
			return &otherEpoch
		}},
		{"belongs to another author", func() *AvailabilityAdvisory {
			otherAuthor := *advisory
			otherAuthor.Author = strings.Repeat("55", addressBytes)
			return &otherAuthor
		}},
	}
	for _, variant := range variants {
		write(variant.make())
		if _, err := preparePoolBuyFromRequest(node, request, arguments, identity, deliveryKey, advisoryPath); err == nil {
			t.Fatalf("an advisory that %s was accepted", variant.name)
		}
	}

	// A set the chain commits but the buyer's own node cannot serve is refused
	// at the fetch, after the commitment matched.
	absentCID := cidRawSha256([]byte("bytes the node does not serve"))
	absentRecord := poolRecordBytes(
		filled(0x33, 20), filled(0x44, 32),
		mustDecode(inventoryCommitment(author, 228, []string{firstCID, absentCID})),
		3, 1000, 0, 7, 0,
	)
	absentNode := poolBuyStub(t, bundle, absentRecord, map[string][]byte{firstCID: firstBlob})
	write(&AvailabilityAdvisory{
		Format: availabilityFormat, Version: availabilityVersion,
		Author: author, Epoch: 228, CheckedAt: "2026-09-26T12:00:00Z",
		CoveredFlips: 2, VerifiedFlips: 2, Complete: true,
		Flips: []AvailabilityFlip{
			{CID: firstCID, Verified: true, CheckedAt: "2026-09-26T12:00:00Z"},
			{CID: absentCID, Verified: true, CheckedAt: "2026-09-26T12:00:00Z"},
		},
	})
	if _, err := preparePoolBuyFromRequest(absentNode, request, arguments, identity, deliveryKey, advisoryPath); err == nil {
		t.Fatal("a purchase whose content could not be fetched was prepared")
	}

	// Naming no advisory is refused outright (`AH-17`): the ordinary buy path
	// always fetches and checks the content before it builds the call.
	if _, err := preparePoolBuyFromRequest(node, request, arguments, identity, deliveryKey, ""); err == nil {
		t.Fatal("a purchase without an availability advisory was prepared")
	}
}

func TestLoadBuyRequestChecksTheEnvelope(t *testing.T) {
	body := `{
  "format": "fod.action-request",
  "version": 1,
  "network": "idena-local",
  "contract": "` + testContract + `",
  "operation": "buy",
  "arguments": {
    "bundleId": "` + strings.Repeat("ab", poolBundleBytes) + `",
    "expectedQuote": "25000",
    "buyer": "` + strings.Repeat("22", addressBytes) + `"
  },
  "amount": "25000",
  "createdAt": "2026-09-26T12:00:00Z"
}`
	path := writeRequest(t, t.TempDir(), body)
	request, arguments, err := loadBuyRequest(path)
	if err != nil {
		t.Fatalf("load the buy request: %v", err)
	}
	if request.Operation != operationBuy || arguments.ExpectedQuote != "25000" {
		t.Fatalf("the request reads as %+v / %+v", request, arguments)
	}
	mismatch := strings.Replace(body, `"amount": "25000"`, `"amount": "24999"`, 1)
	mismatchPath := writeRequest(t, t.TempDir(), mismatch)
	if _, _, err := loadBuyRequest(mismatchPath); err == nil {
		t.Fatal("a request whose amount and quote disagree was accepted")
	}
}
