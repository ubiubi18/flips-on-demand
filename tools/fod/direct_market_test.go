package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func directMarketItem(key, value []byte) *mapItem {
	return &mapItem{Key: "0x" + hex.EncodeToString(key), Value: "0x" + hex.EncodeToString(value)}
}

func directMarketNode(t *testing.T, offers, orders []*mapItem, calls *[]string, intents ...*mapItem) (*client, func()) {
	t.Helper()
	return directMarketNodeWithPrefixes(t, offers, orders, calls, intents, intents)
}

func directMarketNodeWithPrefixes(t *testing.T, offers, orders []*mapItem, calls *[]string, intents, stakes []*mapItem) (*client, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		*calls = append(*calls, request.Method)
		var result any
		switch request.Method {
		case "contract_getStake":
			result = map[string]any{"Hash": "0x" + directV3CodeHashHex}
		case "bcn_lastBlock":
			result = map[string]any{"height": 120, "hash": "0xabc"}
		case "contract_iterateMap":
			if len(request.Params) != 6 || (request.Params[0] != "0x"+directV3ContractHex && request.Params[0] != "0x"+strings.Repeat("ab", 20)) ||
				request.Params[3] != "hex" || request.Params[4] != "hex" || request.Params[5] != float64(marketPageLimit) {
				http.Error(w, "unexpected map request", http.StatusBadRequest)
				return
			}
			switch request.Params[1] {
			case directOfferPrefix:
				result = mapPage{Items: offers}
			case directOrderPrefix:
				result = mapPage{Items: orders}
			case directIntentPrefix:
				result = mapPage{Items: intents}
			case directStakePrefix:
				result = mapPage{Items: stakes}
			default:
				http.Error(w, "unexpected prefix", http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	}))
	return &client{url: server.URL, http: newHTTPClient()}, server.Close
}

func directMarketRecords(t *testing.T) ([]*mapItem, []*mapItem, [32]byte) {
	t.Helper()
	offer := DirectOffer{Author: [20]byte{1}, Epoch: 230, Covered: 2, Inventory: [32]byte{3}, Keys: [32]byte{4},
		Price: big.NewInt(12345), NextNonce: 3, OpenOrderID: 10}
	id := DirectOfferID(offer.Author, offer.Epoch)
	offerRaw, err := EncodeDirectOffer(offer)
	if err != nil {
		t.Fatal(err)
	}
	order := DirectOrder{OfferID: id, Buyer: [20]byte{5}, DeliveryKey: [65]byte{4, 7},
		Paid: big.NewInt(12345), Bond: big.NewInt(5_000_000), FundedAt: 100, Nonce: 2, State: DirectFunded}
	orderRaw, err := EncodeDirectOrder(order)
	if err != nil {
		t.Fatal(err)
	}
	oldOrder := order
	oldOrder.Nonce = 1
	oldOrder.State = DirectSettled
	oldOrder.DeliveryHash = [32]byte{8}
	oldRaw, err := EncodeDirectOrder(oldOrder)
	if err != nil {
		t.Fatal(err)
	}
	return []*mapItem{directMarketItem(id[:], offerRaw)},
		[]*mapItem{directMarketItem([]byte("10"), orderRaw), directMarketItem([]byte("2"), oldRaw)}, id
}

func TestDirectMarketCommandExportsOnlyPublicCanonicalState(t *testing.T) {
	offers, orders, id := directMarketRecords(t)
	second := DirectOffer{Author: [20]byte{9}, Epoch: 231, Covered: 1, Inventory: [32]byte{6}, Keys: [32]byte{7},
		Price: big.NewInt(678), NextNonce: 1}
	secondID := DirectOfferID(second.Author, second.Epoch)
	secondRaw, err := EncodeDirectOffer(second)
	if err != nil {
		t.Fatal(err)
	}
	offers = append(offers, directMarketItem(secondID[:], secondRaw))
	var calls []string
	node, closeNode := directMarketNode(t, offers, orders, &calls)
	defer closeNode()
	path := filepath.Join(t.TempDir(), "direct-market.json")
	err = directCommand(directCommandOptions{Mode: "direct-market", Node: node, Network: NetworkNamePinned,
		Contract: "0x" + strings.ToUpper(directV3ContractHex),
		CodeHash: "0x" + strings.ToUpper(directV3CodeHashHex), OutPath: path, OutExplicit: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Format      string              `json:"format"`
		Version     int                 `json:"version"`
		Network     string              `json:"network"`
		Contract    string              `json:"contract"`
		CodeHash    string              `json:"codeHash"`
		BlockHeight uint64              `json:"blockHeight"`
		ObservedAt  string              `json:"observedAt"`
		Source      string              `json:"source"`
		Offers      []directMarketOffer `json:"offers"`
		Orders      []directMarketOrder `json:"orders"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if state.Format != "fod.direct-market-state" || state.Version != 2 || state.Network != "idena" ||
		state.Contract != directV3ContractHex || state.CodeHash != directV3CodeHashHex ||
		state.BlockHeight != 120 || state.Source != "node" || state.ObservedAt == "" {
		t.Fatalf("wrong snapshot metadata: %+v", state)
	}
	if _, err := time.Parse(time.RFC3339, state.ObservedAt); err != nil {
		t.Fatalf("snapshot time is not RFC3339: %v", err)
	}
	if len(state.Offers) != 2 || len(state.Orders) != 2 || state.Orders[0].OrderID != "2" || state.Orders[1].OrderID != "10" ||
		state.Orders[1].OfferID != hex.EncodeToString(id[:]) || state.Orders[1].Paid != "12345" || state.Orders[1].Bond != "5000000" ||
		state.Orders[1].FundedAtBlock != "100" || state.Orders[1].RefundAtBlock != "8740" || state.Orders[1].State != "funded" {
		t.Fatalf("wrong public records: offers=%+v orders=%+v", state.Offers, state.Orders)
	}
	ids := []string{state.Offers[0].OfferID, state.Offers[1].OfferID}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("offers not sorted by ID: %v", ids)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	if len(top) != 10 {
		t.Fatalf("unexpected top-level fields: %v", reflect.ValueOf(top).MapKeys())
	}
	var decodedOffers, decodedOrders []map[string]json.RawMessage
	if err := json.Unmarshal(top["offers"], &decodedOffers); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(top["orders"], &decodedOrders); err != nil {
		t.Fatal(err)
	}
	offerFields, orderFields := decodedOffers[0], decodedOrders[0]
	if len(offerFields) != 8 || len(orderFields) != 9 ||
		strings.Contains(string(raw), "deliveryKey") || strings.Contains(string(raw), "deliveryHash") ||
		strings.Contains(string(raw), "ciphertext") || strings.Contains(string(raw), "vault") ||
		strings.Contains(string(raw), "receipt") {
		t.Fatalf("snapshot exposed non-schema or private fields: %s", raw)
	}
	for _, method := range calls {
		if method != "contract_getStake" && method != "bcn_lastBlock" && method != "contract_iterateMap" {
			t.Fatalf("direct-market called %s", method)
		}
	}
	if err := directCommand(directCommandOptions{Mode: "direct-market", Node: node, Network: NetworkNamePinned,
		Contract: directV3ContractHex, CodeHash: directV3CodeHashHex}); err == nil {
		t.Fatal("accepted an implicit output path")
	}
}

func TestDirectMarketMarksOnlyProvenPublicFallbackRecoverable(t *testing.T) {
	offer := DirectOffer{Author: [20]byte{1}, Epoch: 230, Covered: 1, Inventory: [32]byte{3}, Keys: [32]byte{4},
		Price: big.NewInt(12345), NextNonce: 2, Retired: true}
	id := DirectOfferID(offer.Author, offer.Epoch)
	offerRaw, err := EncodeDirectOffer(offer)
	if err != nil {
		t.Fatal(err)
	}
	order := DirectOrder{OfferID: id, Buyer: [20]byte{5}, DeliveryKey: [65]byte{4, 7},
		Paid: big.NewInt(12345), Bond: big.NewInt(5_000_000), FundedAt: 100, Nonce: 1, State: DirectSettled}
	orderRaw, err := EncodeDirectOrder(order)
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	node, closeNode := directMarketNode(t, []*mapItem{directMarketItem(id[:], offerRaw)},
		[]*mapItem{directMarketItem([]byte("1"), orderRaw)}, &calls)
	defer closeNode()
	state, err := buildDirectMarketState(node, strings.Repeat("ab", 20), strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Orders) != 1 || !state.Orders[0].KeysPublic {
		t.Fatal("seller proof settlement was not marked recoverable")
	}
}

func TestDirectMarketExportsFallbackPendingAsActiveAndPrivate(t *testing.T) {
	offers, orders, _ := directMarketRecords(t)
	raw, err := hex.DecodeString(strings.TrimPrefix(orders[0].Value, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	raw[165] = byte(DirectFallbackPending)
	orders[0].Value = "0x" + hex.EncodeToString(raw)
	var calls []string
	node, closeNode := directMarketNode(t, offers, orders, &calls)
	defer closeNode()
	state, err := buildDirectMarketState(node, strings.Repeat("ab", 20), strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Orders) != 2 || state.Orders[1].State != "fallback-pending" || state.Orders[1].KeysPublic {
		t.Fatalf("fallback-pending state exposed as %+v", state.Orders)
	}
}

func TestDirectMarketIntentUsesEarlyRefundHeight(t *testing.T) {
	offers, orders, _ := directMarketRecords(t)
	raw, err := hex.DecodeString(strings.TrimPrefix(orders[0].Value, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	raw[165] = byte(DirectFallbackIntent)
	binary.BigEndian.PutUint64(raw[149:157], 80)
	orders[0].Value = "0x" + hex.EncodeToString(raw)
	intent := make([]byte, directIntentRecordBytes)
	big.NewInt(20000).FillBytes(intent[:16])
	binary.BigEndian.PutUint64(intent[16:], 110)
	var calls []string
	node, closeNode := directMarketNode(t, offers, orders, &calls, directMarketItem([]byte("10"), intent))
	defer closeNode()
	state, err := buildDirectMarketState(node, strings.Repeat("ab", 20), strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Orders) != 2 || state.Orders[1].State != "fallback-intent" ||
		state.Orders[1].RefundAtBlock != "4430" || state.Orders[1].KeysPublic {
		t.Fatalf("wrong intent snapshot: %+v", state.Orders)
	}
	missing, closeMissing := directMarketNode(t, offers, orders, &calls)
	defer closeMissing()
	if _, err := buildDirectMarketState(missing, strings.Repeat("ab", 20), strings.Repeat("cd", 32)); err == nil {
		t.Fatal("intent snapshot accepted an absent intent record")
	}
}

func TestDirectV4MarketReadsPendingSellerStake(t *testing.T) {
	offers, orders, _ := directMarketRecords(t)
	raw, err := hex.DecodeString(strings.TrimPrefix(orders[0].Value, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	raw[165] = byte(DirectFallbackPending)
	orders[0].Value = "0x" + hex.EncodeToString(raw)
	stakeRaw := make([]byte, directStakeRecordBytes)
	big.NewInt(125000).FillBytes(stakeRaw)
	var calls []string
	stakeRecord := directMarketItem([]byte("10"), stakeRaw)
	node, closeNode := directMarketNode(t, offers, orders, &calls, stakeRecord)
	defer closeNode()
	state, err := buildDirectMarketStateWithVersion(node, strings.Repeat("ab", 20), strings.Repeat("cd", 32), 4)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 3 || len(state.Orders) != 2 || state.Orders[1].SellerStake != "125000" ||
		state.Orders[1].RefundAtBlock != "8740" || state.Orders[1].KeysPublic {
		t.Fatalf("wrong v4 pending snapshot: %+v", state)
	}
	missing, closeMissing := directMarketNode(t, offers, orders, &calls)
	defer closeMissing()
	if _, err := buildDirectMarketStateWithVersion(missing, strings.Repeat("ab", 20), strings.Repeat("cd", 32), 4); err == nil {
		t.Fatal("v4 pending snapshot accepted a missing stake")
	}
	bad := directMarketItem([]byte("10"), make([]byte, directStakeRecordBytes))
	zero, closeZero := directMarketNode(t, offers, orders, &calls, bad)
	defer closeZero()
	if _, err := buildDirectMarketStateWithVersion(zero, strings.Repeat("ab", 20), strings.Repeat("cd", 32), 4); err == nil {
		t.Fatal("v4 pending snapshot accepted a zero stake")
	}
}

func TestDirectV5MarketReadsStakeAndRejectsLegacyIntent(t *testing.T) {
	offers, orders, _ := directMarketRecords(t)
	offerRaw, err := hex.DecodeString(strings.TrimPrefix(offers[0].Value, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(orders[0].Value, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	if len(offerRaw) != 123 || len(raw) != 198 {
		t.Fatalf("wrong direct record lengths: offer=%d order=%d", len(offerRaw), len(raw))
	}
	raw[165] = byte(DirectFallbackPending)
	orders[0].Value = "0x" + hex.EncodeToString(raw)
	stakeRaw := make([]byte, directStakeRecordBytes)
	big.NewInt(125000).FillBytes(stakeRaw)
	stake := directMarketItem([]byte("10"), stakeRaw)
	intentRaw := make([]byte, directIntentRecordBytes)
	big.NewInt(20000).FillBytes(intentRaw[:16])
	binary.BigEndian.PutUint64(intentRaw[16:], 110)
	intent := directMarketItem([]byte("10"), intentRaw)
	var calls []string
	node, closeNode := directMarketNodeWithPrefixes(t, offers, orders, &calls, []*mapItem{intent}, []*mapItem{stake})
	defer closeNode()
	state, err := buildDirectMarketStateWithVersion(node, strings.Repeat("ab", 20), strings.Repeat("cd", 32), 5)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != 3 || len(state.Orders) != 2 || state.Orders[1].State != "fallback-pending" ||
		state.Orders[1].SellerStake != "125000" || state.Orders[1].RefundAtBlock != "8740" || state.Orders[1].KeysPublic {
		t.Fatalf("wrong v5 pending snapshot: %+v", state)
	}

	missing, closeMissing := directMarketNodeWithPrefixes(t, offers, orders, &calls, []*mapItem{intent}, nil)
	defer closeMissing()
	if _, err := buildDirectMarketStateWithVersion(missing, strings.Repeat("ab", 20), strings.Repeat("cd", 32), 5); err == nil || !strings.Contains(err.Error(), "seller stake") {
		t.Fatalf("v5 pending snapshot accepted a missing stake with a legacy intent present: %v", err)
	}

	intentOrder := append([]byte(nil), raw...)
	intentOrder[165] = byte(DirectFallbackIntent)
	binary.BigEndian.PutUint64(intentOrder[149:157], 80)
	legacyOrders := []*mapItem{directMarketItem([]byte("10"), intentOrder), orders[1]}
	legacy, closeLegacy := directMarketNodeWithPrefixes(t, offers, legacyOrders, &calls, []*mapItem{intent}, []*mapItem{stake})
	defer closeLegacy()
	if _, err := buildDirectMarketStateWithVersion(legacy, strings.Repeat("ab", 20), strings.Repeat("cd", 32), 5); err == nil || !strings.Contains(err.Error(), "fallback-intent") {
		t.Fatalf("v5 snapshot accepted a v3 fallback-intent order: %v", err)
	}
}

func TestDirectMarketRejectsMalformedRecords(t *testing.T) {
	baseOffers, baseOrders, id := directMarketRecords(t)
	for _, tc := range []struct {
		name   string
		change func([]*mapItem, []*mapItem)
	}{
		{"wrong offer ID", func(offers, orders []*mapItem) { offers[0].Key = hex.EncodeToString(make([]byte, 32)) }},
		{"short offer", func(offers, orders []*mapItem) { offers[0].Value = "0x00" }},
		{"noncanonical order ID", func(offers, orders []*mapItem) { orders[0].Key = hex.EncodeToString([]byte("010")) }},
		{"missing offer", func(offers, orders []*mapItem) {
			raw, _ := hex.DecodeString(orders[0].Value[2:])
			raw[0] ^= 0xff
			orders[0].Value = hex.EncodeToString(raw)
		}},
		{"bad open order", func(offers, orders []*mapItem) {
			raw, _ := hex.DecodeString(offers[0].Value[2:])
			raw[121] = 9
			offers[0].Value = hex.EncodeToString(raw)
		}},
		{"invalid retired flag", func(offers, orders []*mapItem) {
			raw, _ := hex.DecodeString(offers[0].Value[2:])
			raw[122] = 3
			offers[0].Value = hex.EncodeToString(raw)
		}},
		{"zero bond", func(offers, orders []*mapItem) {
			raw, _ := hex.DecodeString(orders[0].Value[2:])
			clear(raw[133:149])
			orders[0].Value = hex.EncodeToString(raw)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			offers := []*mapItem{{Key: baseOffers[0].Key, Value: baseOffers[0].Value}}
			orders := []*mapItem{{Key: baseOrders[0].Key, Value: baseOrders[0].Value}, {Key: baseOrders[1].Key, Value: baseOrders[1].Value}}
			tc.change(offers, orders)
			var calls []string
			node, closeNode := directMarketNode(t, offers, orders, &calls)
			defer closeNode()
			if _, err := buildDirectMarketState(node, strings.Repeat("ab", 20), strings.Repeat("cd", 32)); err == nil {
				t.Fatalf("accepted malformed direct records for offer %s", hex.EncodeToString(id[:]))
			}
		})
	}
	var calls []string
	node, closeNode := directMarketNode(t, baseOffers, baseOrders, &calls)
	defer closeNode()
	count := directMarketMaxItems
	if _, err := readDirectMarketPrefix(node, registryAddress(strings.Repeat("ab", 20)), directOfferPrefix, &count); err == nil {
		t.Fatalf("accepted more than %s records", strconv.Itoa(directMarketMaxItems))
	}
}

func TestDirectMarketAcceptsEmptyNodeMaps(t *testing.T) {
	// The node encodes an empty IterateMap result as items:null.
	var calls []string
	node, closeNode := directMarketNode(t, nil, nil, &calls)
	defer closeNode()
	state, err := buildDirectMarketState(node, strings.Repeat("ab", 20), strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"offers":[]`) || !strings.Contains(string(encoded), `"orders":[]`) {
		t.Fatalf("empty node maps did not produce empty arrays: %s", encoded)
	}
}
