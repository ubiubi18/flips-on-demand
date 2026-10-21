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
	"strings"
	"testing"
)

func TestDirectV7MarketSnapshotAndRefusals(t *testing.T) {
	contract := strings.Repeat("ab", 20)
	codeHash := strings.Repeat("cd", 32)
	identity := strings.Repeat("ef", 20)
	var primary, seed, active [32]byte
	primary = poolV7OfferID([20]byte{1}, 230)
	seed = poolV7OfferID([20]byte{2}, 230)
	active = poolV7OfferID([20]byte{3}, 230)
	uint64Bytes := func(value uint64) []byte {
		raw := make([]byte, 8)
		binary.BigEndian.PutUint64(raw, value)
		return raw
	}
	amountBytes := func(value *big.Int) []byte { return value.FillBytes(make([]byte, 16)) }
	cidFor := func(author byte) string { return "b" + strings.Repeat(string(rune('a'+author)), 58) }
	offerBytes := func(author, status byte, bond, nextNonce, openOrder, sequence uint64, publicSeed bool) []byte {
		raw := make([]byte, 222)
		raw[0] = author
		binary.BigEndian.PutUint16(raw[20:22], 230)
		binary.BigEndian.PutUint32(raw[22:26], 1)
		var address [20]byte
		address[0] = author
		commitment, _ := hex.DecodeString(inventoryCommitment(hex.EncodeToString(address[:]), 230, []string{cidFor(author)}))
		copy(raw[26:58], commitment)
		raw[58] = 2
		copy(raw[90:106], amountBytes(new(big.Int).SetUint64(bond)))
		binary.BigEndian.PutUint64(raw[106:114], nextNonce)
		binary.BigEndian.PutUint64(raw[114:122], openOrder)
		raw[122] = status
		binary.BigEndian.PutUint64(raw[188:196], sequence)
		binary.BigEndian.PutUint64(raw[196:204], 90)
		raw[204] = 1
		if publicSeed {
			raw[221] = 1
		}
		return raw
	}
	order := make([]byte, 215)
	copy(order[:32], active[:])
	order[32] = 4
	deliveryKey, err := hex.DecodeString("0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8")
	if err != nil {
		t.Fatal(err)
	}
	copy(order[52:117], deliveryKey)
	copy(order[117:133], amountBytes(big.NewInt(500)))
	copy(order[133:149], amountBytes(big.NewInt(50)))
	copy(order[149:165], amountBytes(big.NewInt(500)))
	binary.BigEndian.PutUint64(order[165:173], 100)
	binary.BigEndian.PutUint64(order[173:181], 1)
	order[181] = 1
	seedAmount, _ := new(big.Int).SetString(poolV7SeedAmount, 10)
	fields := map[string][]byte{
		"pool/seed-id": seed[:], "pool/primary-head": primary[:], "pool/primary-tail": primary[:],
		"pool/secondary-head": seed[:], "pool/secondary-tail": seed[:],
		"pool/primary-units": uint64Bytes(1), "pool/secondary-units": uint64Bytes(1),
		"pool/units": uint64Bytes(3), "pool/reserved-units": uint64Bytes(1),
		"pool/seed-count": uint64Bytes(1), "pool/next-order": uint64Bytes(2),
		"pool/next-sequence": uint64Bytes(4), "pool/activated-at": uint64Bytes(80),
		"pool/reserve": amountBytes(seedAmount), "pool/insurance": amountBytes(big.NewInt(0)),
	}
	newNode := func(syncing, drift, corruptOrder, publicHead, corruptInventory, invalidDeliveryKey bool) *client {
		t.Helper()
		headReads := 0
		localFields := make(map[string][]byte, len(fields))
		for key, value := range fields {
			localFields[key] = value
		}
		localOrder := append([]byte(nil), order...)
		if corruptOrder {
			localOrder[0] ^= 0xff
		}
		if invalidDeliveryKey {
			for i := 53; i < 117; i++ {
				localOrder[i] = 0
			}
		}
		localFields["pool/order/1"] = localOrder
		if publicHead {
			localFields["pool/primary-head"] = make([]byte, 32)
			localFields["pool/primary-tail"] = make([]byte, 32)
			localFields["pool/primary-units"] = uint64Bytes(0)
			localFields["pool/units"] = uint64Bytes(2)
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				Method string `json:"method"`
				Key    string `json:"key"`
				Params []any  `json:"params"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.Key != "test-rpc-key" {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			var result any
			switch request.Method {
			case "bcn_syncing":
				result = map[string]any{"syncing": syncing, "currentBlock": 120, "highestBlock": 120, "wrongTime": false}
			case "bcn_lastBlock":
				headReads++
				hash := "0x" + strings.Repeat("aa", 32)
				if drift && headReads == 2 {
					hash = "0x" + strings.Repeat("bb", 32)
				}
				result = map[string]any{"height": 120, "hash": hash}
			case "contract_getStake":
				result = map[string]any{"Hash": "0x" + codeHash}
			case "dna_getCoinbaseAddr":
				result = "0x" + identity
			case "contract_readData":
				if len(request.Params) != 3 || request.Params[0] != "0x"+contract {
					http.Error(w, "invalid storage read", http.StatusBadRequest)
					return
				}
				raw, ok := localFields[request.Params[1].(string)]
				if !ok {
					http.Error(w, "unknown field", http.StatusBadRequest)
					return
				}
				result = "0x" + hex.EncodeToString(raw)
			case "contract_iterateMap":
				if len(request.Params) != 6 || request.Params[0] != "0x"+contract {
					http.Error(w, "invalid map read", http.StatusBadRequest)
					return
				}
				switch request.Params[1] {
				case poolV6OfferPrefix:
					first := offerBytes(1, 1, 10, 1, 0, 2, false)
					if publicHead {
						first = offerBytes(1, 5, 0, 1, 0, 2, false)
					}
					result = mapPage{Items: []*mapItem{
						directMarketItem(primary[:], first),
						directMarketItem(seed[:], offerBytes(2, 2, 0, 1, 0, 1, true)),
						directMarketItem(active[:], offerBytes(3, 3, 10, 2, 1, 3, false)),
					}}
				case poolV6OrderPrefix:
					result = mapPage{Items: []*mapItem{directMarketItem([]byte("1"), localOrder)}}
				case poolV6InventoryPrefix:
					entry := func(author byte) []byte {
						cid := cidFor(author)
						return append([]byte{1, byte(len(cid))}, []byte(cid)...)
					}
					first := entry(1)
					if corruptInventory {
						first[len(first)-1] ^= 1
					}
					result = mapPage{Items: []*mapItem{
						directMarketItem(primary[:], first),
						directMarketItem(seed[:], entry(2)),
						directMarketItem(active[:], entry(3)),
					}}
				default:
					http.Error(w, "unknown prefix", http.StatusBadRequest)
					return
				}
			default:
				http.Error(w, "unknown method", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
		}))
		t.Cleanup(server.Close)
		return &client{url: server.URL, key: "test-rpc-key", http: newHTTPClient()}
	}

	node := newNode(false, false, false, false, false, false)
	snapshot, err := readPoolV7MarketSnapshot(node, contract, codeHash)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Quote == nil || *snapshot.Quote != "909090909090909090910" ||
		snapshot.NextOffer == nil || snapshot.NextOffer.OfferID != hex.EncodeToString(primary[:]) ||
		snapshot.NextOffer.Status != "queued-primary" || snapshot.NextOffer.PublicSeed || snapshot.BlockHeight != 120 ||
		len(snapshot.NextOffer.OriginalCIDs) != 1 || snapshot.NextOffer.OriginalCIDs[0] != cidFor(1) ||
		len(snapshot.OpenOrders) != 1 || snapshot.OpenOrders[0].OrderID != "1" ||
		snapshot.OpenOrders[0].State != "funded" || snapshot.OpenOrders[0].FundedAt != 100 ||
		snapshot.OpenOrders[0].ProofDeadline != 100+poolV7ProofWindow || snapshot.OpenOrders[0].Challenged ||
		snapshot.OpenOrders[0].InvalidDeliveryKey {
		t.Fatalf("wrong selected v7 offer or quote: %+v", snapshot)
	}
	keyFile := filepath.Join(t.TempDir(), "rpc.key")
	if err := os.WriteFile(keyFile, []byte("test-rpc-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "market.json")
	if err := directCommand(directCommandOptions{
		Mode: "direct-v7-market", DirectVersion: 7, Node: newNode(false, false, false, false, false, false),
		Contract: contract, CodeHash: codeHash, Network: NetworkNamePinned,
		OutPath: out, OutExplicit: true, RPCExplicit: true, KeyFileExplicit: true,
		KeyFilePath: keyFile, APIKey: "test-rpc-key",
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("v7 snapshot was not written owner-only: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil || !json.Valid(raw) || strings.Contains(string(raw), "deliveryKey") ||
		strings.Contains(string(raw), hex.EncodeToString(deliveryKey)) {
		t.Fatalf("v7 snapshot contains unexpected data or invalid JSON: %v", err)
	}
	publicSnapshot, err := readPoolV7MarketSnapshot(newNode(false, false, false, true, false, false), contract, codeHash)
	if err != nil || publicSnapshot.NextOffer == nil || !publicSnapshot.NextOffer.PublicSeed ||
		publicSnapshot.NextOffer.OfferID != hex.EncodeToString(seed[:]) || publicSnapshot.Quote == nil ||
		*publicSnapshot.Quote != poolV7PriceCap {
		t.Fatalf("public seed did not receive its v7 quote: %v, %+v", err, publicSnapshot)
	}
	t.Run("invalid delivery key remains visible", func(t *testing.T) {
		badNode := newNode(false, false, false, false, false, true)
		server := &standaloneServer{
			origin: "http://127.0.0.1:39441", token: strings.Repeat("a", 32),
			market: func() (any, error) { return readPoolV7MarketSnapshot(badNode, contract, codeHash) },
		}
		request := httptest.NewRequest(http.MethodGet, server.origin+"/api/market", nil)
		request.Header.Set("X-FOD-Token", server.token)
		response := httptest.NewRecorder()
		server.handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("funded order made market unavailable: HTTP %d", response.Code)
		}
		var got poolV7MarketSnapshot
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.NextOffer == nil || got.Quote == nil || len(got.OpenOrders) != 1 ||
			got.OpenOrders[0].OrderID != "1" || got.OpenOrders[0].State != "funded" ||
			!got.OpenOrders[0].InvalidDeliveryKey {
			t.Fatalf("market omitted the unsafe funded order or warning: %+v", got)
		}
		if strings.Contains(response.Body.String(), "deliveryKey\":") ||
			strings.Contains(response.Body.String(), "04"+strings.Repeat("00", 64)) {
			t.Fatal("market exposed the buyer delivery key")
		}
		if _, err := readPoolV7FundedDelivery(badNode, contract, 1); err == nil ||
			!strings.Contains(err.Error(), "invalid buyer delivery key") {
			t.Fatalf("private delivery accepted an invalid buyer key: %v", err)
		}
	})
	for _, tc := range []struct {
		name, want                        string
		syncing, drift, bad, badInventory bool
	}{
		{"unsynced node", "not confirmed synced", true, false, false, false},
		{"changed head", "head changed", false, true, false, false},
		{"invalid order", "invalid offer reference", false, false, true, false},
		{"invalid inventory", "invalid public CID inventory", false, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readPoolV7MarketSnapshot(newNode(tc.syncing, tc.drift, tc.bad, false, tc.badInventory, false), contract, codeHash); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q refusal, got %v", tc.want, err)
			}
		})
	}
}
