package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

// The fixture speaks the actual node RPC wire format. No transaction method is
// implemented: preparation and collection must succeed without one.
func standaloneV8Fixture(t *testing.T) (*standaloneServer, *[]byte, *uint16) {
	t.Helper()
	author := strings.Repeat("33", 20)
	blob := []byte("encrypted package fixture")
	cid := cidRawSha256(blob)
	epoch := uint16(233)
	totals := make([]byte, 104)
	for _, off := range []int{80, 88, 96} {
		binary.BigEndian.PutUint64(totals[off:off+8], 1)
	}
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if e := json.NewDecoder(r.Body).Decode(&request); e != nil {
			t.Error(e)
			return
		}
		var result any
		switch request.Method {
		case "contract_getStake":
			result = map[string]any{"Hash": "0x" + strings.Repeat("cd", 32)}
		case "bcn_syncing":
			result = map[string]any{"syncing": false, "currentBlock": 1000, "highestBlock": 1000, "wrongTime": false}
		case "bcn_lastBlock":
			result = map[string]any{"height": 1000, "hash": "0x" + strings.Repeat("aa", 32)}
		case "dna_getCoinbaseAddr":
			result = "0x" + author
		case "dna_epoch":
			result = map[string]any{"epoch": epoch}
		case "dna_identity":
			result = map[string]any{"flips": []string{cid}}
		case "ipfs_get":
			result = "0x" + hex.EncodeToString(blob)
		case "dna_getBalance":
			result = map[string]any{"balance": "100", "nonce": 0, "mempoolNonce": 0}
		case "bcn_feePerGas":
			result = 1000
		case "contract_readData":
			var key string
			_ = json.Unmarshal(request.Params[1], &key)
			if key == "v8/totals" || key == "v9/totals" {
				result = "0x" + hex.EncodeToString(totals)
			} else {
				result = "0x"
			}
		case "contract_readMap":
			result = "0x"
		case "contract_iterateMap":
			result = map[string]any{"items": []any{}, "continuationToken": nil}
		case "contract_estimateCall":
			result = map[string]any{"success": true, "gasUsed": 10000, "minGasLimit": 12000, "txFee": "0.000000000001"}
		default:
			t.Errorf("unexpected node operation %s", request.Method)
			http.Error(w, "unexpected operation", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	t.Cleanup(node.Close)
	root := t.TempDir()
	_ = os.Chmod(root, 0700)
	keyFile := filepath.Join(root, "api.key")
	if e := os.WriteFile(keyFile, []byte("fixture-api-key"), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := newStandaloneServer(standaloneConfig{Version: 8, RPC: node.URL, KeyFile: keyFile, Contract: strings.Repeat("ab", 20), CodeHash: strings.Repeat("cd", 32), DataDir: filepath.Join(root, "market")})
	if e != nil {
		t.Fatal(e)
	}
	s.origin = "http://127.0.0.1:39441"
	s.runCommand = func(ctx context.Context, args []string, pass string) ([]byte, error) {
		if len(args) == 0 || args[0] != "v8" {
			t.Fatalf("preparation attempted a non-v8 command: %v", args)
		}
		for _, arg := range args {
			if arg == "-nodekey" || arg == "-broadcast" {
				t.Fatal("preparation attempted to sign")
			}
		}
		return nil, runBondedV8(args[1:])
	}
	return s, &blob, &epoch
}

func TestStandaloneV8PairingBoundaryAndPreviewIsolation(t *testing.T) {
	s, _, _ := standaloneV8Fixture(t)
	for _, path := range []string{"/api/v8/order", "/api/v8/purchased-preview", "/api/v8/protect", "/api/v8/collect", "/api/v8/unlock", "/api/v8/import", "/api/v8/download?trade=1"} {
		for _, origin := range []string{"https://untrusted.example", s.origin} {
			r := httptest.NewRequest(http.MethodPost, s.origin+path, strings.NewReader(`{}`))
			r.Header.Set("Origin", origin)
			if origin != s.origin {
				r.Header.Set("X-FOD-Token", s.token)
			}
			w := httptest.NewRecorder()
			s.handler().ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("unauthorized %s got %d", path, w.Code)
			}
		}
	}
	for _, path := range []string{"/api/preview", "/api/private/dm", "/api/private/receive"} {
		method := http.MethodPost
		if path == "/api/preview" {
			method = http.MethodGet
		}
		r := httptest.NewRequest(method, s.origin+path, strings.NewReader(`{}`))
		r.Header.Set("Origin", s.origin)
		r.Header.Set("X-FOD-Token", s.token)
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("legacy external-content path %s is still available: %d", path, w.Code)
		}
	}
	h := standaloneV8PreviewHandler(s.origin)
	for _, path := range []string{"/api/submit", "/api/v8/order", "/api/v8/collect", "/api/v8/unlock"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, s.origin+path, strings.NewReader(`{}`)))
		if w.Code != 405 {
			t.Fatalf("preview accepted a mutation at %s", path)
		}
	}
	for _, path := range []string{"/", "/app.js", "/app.css", "/model.mjs"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, s.origin+path, nil))
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
			t.Fatalf("local asset unavailable or unguarded: %s", path)
		}
		if strings.Contains(strings.ReplaceAll(w.Body.String(), "https://idena.social/", ""), "https://") || strings.Contains(w.Body.String(), s.token) {
			t.Fatalf("external dependency or secret in %s", path)
		}
	}
}

func TestStandaloneV8PurchasedPreviewRejectsPathsAndUnownedTrades(t *testing.T) {
	s, _, _ := standaloneV8Fixture(t)
	for _, body := range []string{`{"id":"../1"}`, `{"id":"1","deliveryKeyFile":"/tmp/key"}`, `{"id":"1","caller":"other"}`, `{"id":"1"}`} {
		r := httptest.NewRequest(http.MethodPost, s.origin+"/api/v8/purchased-preview", strings.NewReader(body))
		r.Header.Set("Origin", s.origin)
		r.Header.Set("X-FOD-Token", s.token)
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("unsafe preview accepted: %d", w.Code)
		}
	}
	for _, origin := range []string{s.origin, "https://idena.social"} {
		r := httptest.NewRequest(http.MethodPost, s.origin+"/api/v8/purchased-preview", strings.NewReader(`{"id":"1"}`))
		r.Header.Set("Origin", origin)
		if origin != s.origin {
			r.Header.Set("X-FOD-Token", s.token)
		}
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("preview lost token or origin isolation")
		}
	}
}

func TestStandaloneV8CollectionPreservesEvidenceAndSeparatesAuthors(t *testing.T) {
	s, blob, epoch := standaloneV8Fixture(t)
	m, e := s.v8ReadMarket()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.v8Collect(m); e != nil {
		t.Fatal(e)
	}
	p, e := s.v8Packages(m, *epoch)
	if e != nil || len(p) != 1 || p[0].Retained != 1 || p[0].Registered || p[0].Status == "ready" {
		t.Fatalf("discovery incorrectly claims sale readiness: %+v %v", p, e)
	}
	dir, _ := s.packageDir(p[0].ID)
	saved, e := readBondedV8Private(filepath.Join(dir, "blobs", p[0].CIDs[0]), maxRetainedFlipBytes)
	if e != nil || !bytes.Equal(saved, *blob) {
		t.Fatal("original encrypted blob was not retained", e)
	}
	witness := filepath.Join(dir, "witness-original.json")
	if e = os.WriteFile(witness, []byte("original witness"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.v8Collect(m); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(witness)
	if string(after) != "original witness" {
		t.Fatal("collection changed a witness")
	}
	foreign, e := canonicalPackage(strings.Repeat("44", 20), *epoch, p[0].CIDs)
	if e != nil {
		t.Fatal(e)
	}
	if foreign.ID == p[0].ID {
		t.Fatal("authors share a package identity")
	}
	if e = s.savePackage(foreign); e != nil {
		t.Fatal(e)
	}
	visible, e := s.v8Packages(m, *epoch)
	if e != nil || len(visible) != 1 {
		t.Fatal("another author's private package is visible", e)
	}
	// A corrupt existing blob is reported and preserved, never silently replaced.
	file := filepath.Join(dir, "blobs", p[0].CIDs[0])
	if e = os.WriteFile(file, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.v8Collect(m); e == nil {
		t.Fatal("corrupt retained CID accepted")
	}
	after, _ = os.ReadFile(file)
	if string(after) != "corrupt" {
		t.Fatal("corrupt evidence was silently overwritten")
	}
}

func TestStandaloneV8RejectsCIDMismatchAndForgedSession(t *testing.T) {
	s, blob, _ := standaloneV8Fixture(t)
	m, e := s.v8ReadMarket()
	if e != nil {
		t.Fatal(e)
	}
	*blob = []byte("different remote body")
	if _, e = s.v8Collect(m); e == nil {
		t.Fatal("node-supplied wrong CID accepted")
	}
	session := SessionBundle{Author: m.Identity, Epoch: 233, Manifest: []string{cidRawSha256([]byte("original"))}}
	raw, _ := json.Marshal(session)
	if e = s.v8SaveSession(&session, raw); e == nil {
		t.Fatal("unproved session accepted")
	}
	if _, e = canonicalPackage(m.Identity, 233, []string{session.Manifest[0], session.Manifest[0]}); e == nil {
		t.Fatal("duplicate flip inventory accepted")
	}
}

func TestStandaloneV8PoolSaleRemainsPendingUntilSettlement(t *testing.T) {
	s, blob, epoch := standaloneV8Fixture(t)
	m, e := s.v8ReadMarket()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.v8Collect(m); e != nil {
		t.Fatal(e)
	}
	p, _ := canonicalPackage(m.Identity, *epoch, []string{cidRawSha256(*blob)})
	m.Bundles = []bondedV8Bundle{{ID: p.ID, Author: p.Author, Epoch: p.Epoch, Inventory: inventoryCommitment(p.Author, p.Epoch, p.CIDs), Sale: "1", Used: true, CIDs: p.CIDs}}
	m.Sells = []bondedV8Sell{{ID: "1", Bundle: p.ID, Seller: p.Author, State: 2}}
	packages, e := s.v8Packages(m, *epoch)
	if e != nil || packages[0].Status != "queued" {
		t.Fatal("acquired package misreported as settled", e)
	}
	m.Sells = nil
	m.Trades = []bondedV8Trade{{Bundle: p.ID, State: 3}}
	packages, e = s.v8Packages(m, *epoch)
	if e != nil || packages[0].Status != "queued" {
		t.Fatal("unaccepted delivery misreported as settled", e)
	}
	m.Trades[0].State = 5
	packages, e = s.v8Packages(m, *epoch)
	if e != nil || packages[0].Status != "sold" {
		t.Fatal("settled package state missing", e)
	}
	if e = s.v8CheckSellerPackage(m, p.ID, "not-a-real-passphrase"); e == nil {
		t.Fatal("missing seller vault allowed sale")
	}
}

func TestStandaloneV8RealPreparationAndWrongSignerBoundary(t *testing.T) {
	s, _, _ := standaloneV8Fixture(t)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("operation", "depositStake")
	_ = form.WriteField("amount", "17000000000000000000")
	_ = form.WriteField("target", "17000000000000000000")
	_ = form.Close()
	r := httptest.NewRequest(http.MethodPost, s.origin+"/api/prepare", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	r.Header.Set("Origin", s.origin)
	r.Header.Set("X-FOD-Token", s.token)
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("real v8 preparation failed: %s", w.Body)
	}
	var a standaloneAction
	if e := json.Unmarshal(w.Body.Bytes(), &a); e != nil {
		t.Fatal(e)
	}
	if a.Attempted || a.MaximumFee != "15300000" || a.Amount != "17000000000000000000" {
		t.Fatalf("invalid unsigned review %+v", a)
	}
	key, _ := crypto.GenerateKey()
	s.cfg.NodeKey = filepath.Join(t.TempDir(), "wrong-key")
	_ = crypto.SaveECDSA(s.cfg.NodeKey, key)
	raw, _ := json.Marshal(map[string]string{"id": a.ID, "reviewHash": a.ReviewHash})
	if _, e := s.submit(httptest.NewRequest(http.MethodPost, s.origin+"/api/submit", bytes.NewReader(raw))); e == nil {
		t.Fatal("wrong identity key accepted")
	}
	check, _ := s.readAction(a.ID)
	if check.Attempted {
		t.Fatal("wrong signer consumed the attempt marker")
	}
	s.cfg.Version = 7
	if _, e := s.readAction(a.ID); e == nil {
		t.Fatal("v8 action replayed into v7 app")
	}
	s.cfg.Version = 8
	dir, _ := s.actionDir(a.ID)
	_ = os.WriteFile(filepath.Join(dir, "attempted"), []byte("nonce=1"), 0600)
	if e := s.v8CheckPending(); e == nil {
		t.Fatal("uncertain attempt did not block another action")
	}
	_ = os.WriteFile(filepath.Join(dir, "prepared.json"), []byte("corrupt"), 0600)
	if e := s.v8CheckPending(); e == nil {
		t.Fatal("corrupt pending action was ignored")
	}
}

func TestStandaloneV8AutomaticAuthorityIsBounded(t *testing.T) {
	s, _, _ := standaloneV8Fixture(t)
	m, e := s.v8ReadMarket()
	if e != nil {
		t.Fatal(e)
	}
	s.v8.Grant = &standaloneV8Grant{Identity: m.Identity, Until: time.Now().Add(time.Hour), Passphrase: "fixture-password", Fees: big.NewInt(100), PerTrade: big.NewInt(1000), Purchases: big.NewInt(2000)}
	for _, op := range []string{"depositStake", "withdrawStake", "addReserve", "registerBundle", "challenge", "proveDelivery"} {
		if _, e := s.v8Execute(context.Background(), m, bondedV8Request{Caller: m.Identity, Operation: op}); e == nil {
			t.Fatalf("automatic authority included %s", op)
		}
	}
	if _, e := s.v8Execute(context.Background(), m, bondedV8Request{Caller: m.Identity, Operation: "queueBuy", Maximum: "1001"}); e == nil {
		t.Fatal("trade limit exceeded")
	}
	m.Account.NeedsTopup = true
	if _, e := s.v8Execute(context.Background(), m, bondedV8Request{Caller: m.Identity, Operation: "queueBuy", Maximum: "1"}); e == nil {
		t.Fatal("penalized account opened another trade")
	}
	m.Account.NeedsTopup = false
	// A locally simulated fee above the remaining allowance never reaches submit.
	if _, e := s.v8Execute(context.Background(), m, bondedV8Request{Caller: m.Identity, Operation: "queueBuy", Maximum: "1000"}); e == nil {
		t.Fatal("fee allowance exceeded")
	}
	if s.v8.Grant.Fees.Int64() != 100 || s.v8.Grant.Purchases.Int64() != 2000 {
		t.Fatal("preparation consumed a transaction budget")
	}
	s.v8.Grant.Until = time.Now().Add(-time.Second)
	if s.v8GrantValid(m.Identity) || s.v8.Grant != nil {
		t.Fatal("expired signing permission retained")
	}
}

func TestStandaloneV8CollectionRefusesEpochRollover(t *testing.T) {
	s, _, _ := standaloneV8Fixture(t)
	m, e := s.v8ReadMarket()
	if e != nil {
		t.Fatal(e)
	}
	// Use a separate RPC that rolls over between inventory reads.
	calls := 0
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		var value any
		switch q.Method {
		case "dna_epoch":
			calls++
			value = map[string]any{"epoch": 233 + calls}
		case "dna_identity":
			value = map[string]any{"flips": []string{}}
		case "dna_getCoinbaseAddr":
			value = "0x" + m.Identity
		default:
			t.Error("unexpected method", q.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": value})
	}))
	defer node.Close()
	s.client.url = node.URL
	if _, e = s.v8Collect(m); e == nil {
		t.Fatal("mixed-epoch inventory accepted")
	}
	entries, e := os.ReadDir(filepath.Join(s.cfg.DataDir, "packages"))
	if e != nil || len(entries) != 0 {
		t.Fatal("rollover wrote an inventory")
	}
}

func TestStandaloneV8FailedAutomationConsumesBudgetAndStops(t *testing.T) {
	s, _, _ := standaloneV8Fixture(t)
	m, e := s.v8ReadMarket()
	if e != nil {
		t.Fatal(e)
	}
	grant := &standaloneV8Grant{Identity: m.Identity, Until: time.Now().Add(time.Hour), Fees: big.NewInt(100000000), PerTrade: big.NewInt(1000), Purchases: big.NewInt(2000)}
	s.v8.Grant = grant
	// Submission is refused because signing is deliberately unconfigured. Once
	// handed to submit, an allowance is consumed conservatively even on failure.
	if _, e = s.v8Execute(context.Background(), m, bondedV8Request{Caller: m.Identity, Operation: "queueBuy", Maximum: "1000"}); e == nil {
		t.Fatal("read-only app submitted")
	}
	if s.v8.Grant != nil || grant.Purchases.Int64() != 1000 || grant.Fees.Int64() != 84700000 {
		t.Fatal("failed submit did not stop and conservatively consume limits")
	}
	s.v8.Grant = &standaloneV8Grant{Identity: m.Identity, Until: time.Now().Add(time.Hour), Fees: big.NewInt(100000000), PerTrade: big.NewInt(1000), Purchases: big.NewInt(2000)}
	s.runCommand = func(context.Context, []string, string) ([]byte, error) { return nil, errors.New("fixture failure") }
	if _, e = s.v8Execute(context.Background(), m, bondedV8Request{Caller: m.Identity, Operation: "queueBuy", Maximum: "1000"}); e == nil {
		t.Fatal("failed preparation accepted")
	}
}
