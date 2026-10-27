package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func TestBuyerOnlyV9WalletIdentityAndSellerBoundary(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	keyPath := filepath.Join(root, "wallet.key")
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(crypto.FromECDSA(key))), 0600); err != nil {
		t.Fatal(err)
	}
	wallet, err := buyerWalletAddress(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	contract := strings.Repeat("ab", 20)
	pin := strings.Repeat("cd", 32)
	relay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc" || r.Method != http.MethodPost {
			t.Errorf("unexpected relay path or method: %s %s", r.Method, r.URL.Path)
			http.Error(w, "bad request", 400)
			return
		}
		var envelope map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Errorf("decode relay request: %v", err)
			http.Error(w, "bad request", 400)
			return
		}
		if _, supplied := envelope["key"]; supplied {
			t.Error("buyer sent a key field to the public relay")
			http.Error(w, "bad request", 400)
			return
		}
		encoded, _ := json.Marshal(envelope)
		var call struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(encoded, &call); err != nil {
			t.Errorf("invalid or keyed relay request: %v", err)
			http.Error(w, "bad request", 400)
			return
		}
		var result any
		switch call.Method {
		case "bcn_syncing":
			result = map[string]any{"syncing": false, "currentBlock": 1000, "highestBlock": 1000, "wrongTime": false}
		case "bcn_lastBlock":
			result = map[string]any{"height": 1000, "hash": "0x" + strings.Repeat("11", 32)}
		case "contract_getStake":
			result = map[string]any{"Hash": "0x" + pin}
		case "contract_readData", "contract_readMap":
			result = "0x"
			var storageKey string
			if len(call.Params) > 1 {
				_ = json.Unmarshal(call.Params[1], &storageKey)
			}
			if call.Method == "contract_readData" && storageKey == "v9/totals" {
				// The empty market has a fixed-size totals record and no orders.
				result = "0x" + strings.Repeat("00", 104)
			}
		case "contract_iterateMap":
			result = map[string]any{"items": []any{}, "continuationToken": nil}
		case "dna_epoch":
			result = map[string]any{"epoch": 233}
		case "dna_getBalance":
			result = map[string]any{"balance": "10", "nonce": 0, "mempoolNonce": 0}
		default:
			t.Errorf("buyer made forbidden or unexpected RPC call %q", call.Method)
			http.Error(w, "forbidden", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	defer relay.Close()
	c := &client{url: relay.URL + "/rpc", http: &httpClient{client: relay.Client()}, bondedVersion: 9, buyerOnly: true, wallet: wallet}
	if !c.bondedRPCAllowed() {
		t.Fatal("HTTPS buyer relay refused")
	}
	s := &standaloneServer{cfg: standaloneConfig{Version: 9, BuyerOnly: true, Contract: contract, CodeHash: pin}, client: c, v8: &standaloneV8Runtime{}, origin: "http://127.0.0.1:39441"}
	status, err := s.v8Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Market.Identity != wallet || !status.BuyerOnly || status.Packages == nil || len(status.Packages) != 0 {
		t.Fatal("buyer status did not use the local wallet identity")
	}
	if _, err := s.v8Prepare(context.Background(), status.Market, bondedV8Request{Operation: "queueSell", Caller: wallet}, "", nil); err == nil {
		t.Fatal("buyer-only mode accepted a seller operation")
	}
	mode := httptest.NewRecorder()
	s.handler().ServeHTTP(mode, httptest.NewRequest(http.MethodGet, s.origin+"/api/mode", nil))
	if mode.Code != 200 || !strings.Contains(mode.Body.String(), `"buyerOnly":true`) {
		t.Fatalf("buyer mode was not exposed: %d %s", mode.Code, mode.Body.String())
	}
	for _, endpoint := range []string{"http://relay.example/rpc", "https://user:pass@relay.example/rpc", "https://relay.example/rpc?key=secret", "https://relay.example/other"} {
		if buyerRelayURL(endpoint) {
			t.Fatalf("unsafe relay URL accepted: %s", endpoint)
		}
	}
	if err := os.Chmod(keyPath, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := buyerWalletAddress(keyPath); err == nil {
		t.Fatal("non-owner-only wallet file accepted")
	}
	created := filepath.Join(root, "created-wallet.key")
	if err := runBuyerWalletCreate([]string{"-out", created}); err != nil {
		t.Fatal(err)
	}
	if _, err := buyerWalletAddress(created); err != nil {
		t.Fatal("created wallet is not usable:", err)
	}
	if err := runBuyerWalletCreate([]string{"-out", created}); err == nil {
		t.Fatal("wallet creation overwrote an existing key")
	}
}
