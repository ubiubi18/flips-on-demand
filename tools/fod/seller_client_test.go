package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
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

func sellerClientTestKey(t *testing.T) (string, string) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "identity.key")
	if err := os.WriteFile(path, []byte(hex.EncodeToString(crypto.FromECDSA(key))), 0600); err != nil {
		t.Fatal(err)
	}
	author, err := sellerIdentityAddress(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, author
}

func TestSellerClientRequiresLocalOwnerKeyAndV9HTTPSRelay(t *testing.T) {
	keyPath, author := sellerClientTestKey(t)
	c := &client{url: "https://relay.example/rpc", http: newHTTPClient(), bondedVersion: 9,
		sellerClient: true, author: author, authorKeyPath: keyPath}
	if !c.bondedRPCAllowed() {
		t.Fatal("seller-client rejected an HTTPS /rpc endpoint and local identity")
	}
	if who, err := c.bondedIdentity(); err != nil || who != author {
		t.Fatalf("local identity unavailable: %q %v", who, err)
	}
	for _, endpoint := range []string{"http://relay.example/rpc", "https://relay.example/other", "https://user:pass@relay.example/rpc", "https://relay.example/rpc?key=secret"} {
		copy := *c
		copy.url = endpoint
		if copy.bondedRPCAllowed() {
			t.Errorf("unsafe seller endpoint accepted: %s", endpoint)
		}
	}
	for _, mutate := range []func(*client){
		func(c *client) { c.bondedVersion = 8 },
		func(c *client) { c.key = "node-api-key" },
		func(c *client) { c.buyerOnly = true },
		func(c *client) { c.author = strings.Repeat("ab", 20) },
	} {
		copy := *c
		mutate(&copy)
		if copy.bondedRPCAllowed() {
			t.Error("invalid seller-client configuration accepted")
		}
	}
	if err := os.Chmod(keyPath, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.bondedIdentity(); err == nil {
		t.Fatal("seller identity accepted a key that is no longer owner-only")
	}
	if err := os.Chmod(keyPath, 0600); err != nil {
		t.Fatal(err)
	}
	if err := runBonded([]string{"-seller-client", "-nodekey", keyPath, "-rpc", "http://relay.example/rpc", "-out", filepath.Join(t.TempDir(), "status.json")}, 9); err == nil {
		t.Fatal("v9 seller command accepted an HTTP relay")
	}
}

func TestSellerClientStatusAndCollectUseLocalIdentity(t *testing.T) {
	keyPath, author := sellerClientTestKey(t)
	contract := strings.Repeat("ab", 20)
	pin := strings.Repeat("cd", 32)
	relay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/evidence/v1/") {
			if r.URL.Path == "/evidence/v1/authors/"+author+"/epochs" {
				_, _ = w.Write([]byte(`{"epochs":[]}`))
			} else {
				http.NotFound(w, r)
			}
			return
		}
		if r.URL.Path != "/rpc" || r.Method != http.MethodPost {
			t.Errorf("unexpected relay route %s %s", r.Method, r.URL.Path)
			http.Error(w, "bad route", 400)
			return
		}
		var call struct {
			Method string            `json:"method"`
			Key    string            `json:"key"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil || call.Key != "" {
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
				result = "0x" + strings.Repeat("00", 104)
			}
		case "contract_iterateMap":
			result = map[string]any{"items": []any{}, "continuationToken": nil}
		case "dna_epoch":
			result = map[string]any{"epoch": 233}
		case "dna_identity":
			if len(call.Params) != 1 || string(call.Params[0]) != `"0x`+author+`"` {
				t.Errorf("identity lookup used another address: %s", call.Params)
			}
			result = map[string]any{"flips": []string{}}
		case "dna_getBalance":
			result = map[string]any{"balance": "10", "nonce": 0, "mempoolNonce": 0}
		default:
			t.Errorf("seller called an unexpected RPC method %q", call.Method)
			http.Error(w, "forbidden", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	defer relay.Close()
	c := &client{url: relay.URL + "/rpc", http: &httpClient{client: relay.Client()}, bondedVersion: 9,
		sellerClient: true, author: author, authorKeyPath: keyPath}
	s := &standaloneServer{cfg: standaloneConfig{Version: 9, SellerClient: true, RPC: c.url, Contract: contract, CodeHash: pin,
		DataDir: t.TempDir(), NodeKey: keyPath}, client: c, v8: &standaloneV8Runtime{}}
	if err := standalonePrivateDir(filepath.Join(s.cfg.DataDir, "packages")); err != nil {
		t.Fatal(err)
	}
	status, err := s.v8Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Market.Identity != author || !status.SellerClient || !status.CanSign {
		t.Fatal("seller status did not use the local identity key")
	}
	if _, err := s.v8Collect(status.Market); err != nil {
		t.Fatal(err)
	}
}

func TestSellerClientRejectsOtherIdentityOrdersAndTrades(t *testing.T) {
	keyPath, author := sellerClientTestKey(t)
	other := strings.Repeat("ef", 20)
	contract := strings.Repeat("ab", 20)
	paths := []string{}
	relay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil || call.Method != "contract_readMap" {
			t.Error("unexpected seller ownership lookup")
			http.Error(w, "bad request", 400)
			return
		}
		var prefix string
		_ = json.Unmarshal(call.Params[1], &prefix)
		paths = append(paths, prefix)
		var raw []byte
		switch prefix {
		case "v9/sell/":
			raw = make([]byte, 117)
			copy(raw[32:52], mustDecode(other))
		case "v9/buy/":
			raw = make([]byte, 110)
			copy(raw[:20], mustDecode(other))
		case "v9/trade/":
			raw = make([]byte, 308)
			copy(raw[48:68], mustDecode(other))
			copy(raw[68:88], mustDecode(other))
			raw[306] = 2
		default:
			t.Errorf("unexpected map prefix %q", prefix)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "0x" + hex.EncodeToString(raw)})
	}))
	defer relay.Close()
	c := &client{url: relay.URL + "/rpc", http: &httpClient{client: relay.Client()}, bondedVersion: 9,
		sellerClient: true, author: author, authorKeyPath: keyPath}
	for _, op := range []string{"cancelSell", "cancelBuy", "deliver", "proveDelivery", "challenge", "expireTrade"} {
		if err := verifySellerClientBondedAction(c, contract, &bondedV8Request{ProtocolVersion: 9, Operation: op, Caller: author, ID: "1"}); err == nil {
			t.Errorf("%s accepted another identity's record", op)
		}
	}
	if len(paths) != 6 {
		t.Fatalf("expected ownership checks for all six operations, got %v", paths)
	}
	for _, req := range []bondedV8Request{
		{ProtocolVersion: 9, Operation: "queueSell", Caller: other},
		{ProtocolVersion: 8, Operation: "queueSell", Caller: author},
		{ProtocolVersion: 9, Operation: "registerBundle", Caller: author},
		{ProtocolVersion: 9, Operation: "addReserve", Caller: author},
	} {
		if err := verifySellerClientBondedAction(c, contract, &req); err == nil {
			t.Errorf("unsupported or cross-identity request accepted: %+v", req)
		}
	}
}

func TestSellerClientCollectsVerifiedPriorEpochEvidence(t *testing.T) {
	keyPath, author := sellerClientTestKey(t)
	var address common.Address
	copy(address[:], mustDecode(author))
	blob := []byte("seller shared encrypted flip fixture")
	cid := cidRawSha256(blob)
	cidBytes, err := cidBytesFromString(cid)
	if err != nil {
		t.Fatal(err)
	}
	source := writeSessionDatadir(t, address, [][]byte{cidBytes}, state.AtLeastOneFlipReported, true, fixtureEpoch+1)
	session, err := readSessionBundle(source, sessionInventoryHeight, sessionVerdictHeight, author)
	if err != nil {
		t.Fatal(err)
	}
	sessionBytes, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		blob []byte
		good bool
	}{
		{"complete evidence", blob, true},
		{"wrong CID body", []byte("wrong encrypted flip"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := "/evidence/v1/epochs/228/authors/" + author + "/"
			discovery := "/evidence/v1/authors/" + author + "/epochs"
			sessionGets := 0
			relay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					switch r.URL.Path {
					case discovery:
						_, _ = w.Write([]byte(`{"epochs":[228]}`))
					case prefix + "session":
						sessionGets++
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write(sessionBytes)
					case prefix + "blobs/" + cid:
						_, _ = w.Write(tc.blob)
					default:
						t.Errorf("unexpected shared evidence path %q", r.URL.Path)
						http.NotFound(w, r)
					}
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/rpc" {
					t.Errorf("unexpected relay method or path %s %s", r.Method, r.URL.Path)
					http.Error(w, "bad route", 400)
					return
				}
				var call struct {
					Method string            `json:"method"`
					Key    string            `json:"key"`
					Params []json.RawMessage `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&call); err != nil || call.Key != "" {
					t.Errorf("invalid shared relay RPC request: %v", err)
					http.Error(w, "bad request", 400)
					return
				}
				var result any
				switch call.Method {
				case "dna_epoch":
					result = map[string]any{"epoch": fixtureEpoch + 5}
				case "dna_identity":
					result = map[string]any{"flips": []string{}}
				case "bcn_blockAt":
					var height uint64
					_ = json.Unmarshal(call.Params[0], &height)
					var root string
					switch height {
					case session.Inventory.Height:
						root = session.Inventory.RootHex
					case session.Verdict.Height:
						root = session.Verdict.RootHex
					default:
						t.Errorf("unexpected session header %d", height)
					}
					result = map[string]any{"height": height, "root": "0x" + root}
				default:
					t.Errorf("unexpected shared relay RPC method %q", call.Method)
					http.Error(w, "forbidden", 400)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
			}))
			defer relay.Close()
			folder := t.TempDir()
			if err := standalonePrivateDir(filepath.Join(folder, "packages")); err != nil {
				t.Fatal(err)
			}
			c := &client{url: relay.URL + "/rpc", http: &httpClient{client: relay.Client()}, bondedVersion: 9,
				sellerClient: true, author: author, authorKeyPath: keyPath}
			s := &standaloneServer{cfg: standaloneConfig{Version: 9, SellerClient: true, RPC: c.url, DataDir: folder},
				client: c, v8: &standaloneV8Runtime{}}
			_, err := s.v8Collect(&bondedV8Snapshot{Identity: author})
			if (err == nil) != tc.good {
				t.Fatalf("collect result differs from verified evidence: %v", err)
			}
			packageDir, _ := s.packageDir(bondedBundleID(9, author, fixtureEpoch))
			if tc.good {
				if sessionGets != 1 {
					t.Fatalf("historical session fetches = %d, want 1", sessionGets)
				}
				saved, err := readBondedV8Private(filepath.Join(packageDir, "session.json"), 8<<20)
				if err != nil || !bytes.Equal(saved, sessionBytes) {
					t.Fatal("canonical session was not retained unchanged")
				}
				retained, err := readBondedV8Private(filepath.Join(packageDir, "blobs", cid), maxRetainedFlipBytes)
				if err != nil || !bytes.Equal(retained, blob) {
					t.Fatal("original encrypted flip was not retained")
				}
				if _, err := s.v8Collect(&bondedV8Snapshot{Identity: author}); err != nil || sessionGets != 1 {
					t.Fatalf("completed historical session was fetched again: %d %v", sessionGets, err)
				}
				packages, err := s.v8Packages(&bondedV8Snapshot{Identity: author}, fixtureEpoch+5)
				if err != nil || len(packages) == 0 || packages[0].Status != "unqualified" {
					t.Fatalf("reported validation was presented as sellable: %+v %v", packages, err)
				}
			} else if _, err := os.Lstat(filepath.Join(packageDir, "session.json")); !os.IsNotExist(err) {
				t.Fatal("invalid shared content created a local session")
			}
		})
	}
}
