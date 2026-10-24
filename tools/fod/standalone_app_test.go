package main

import (
	"bytes"
	"context"
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

func TestStandaloneAppRefusesCrossSiteRequestsAndKeepsDataPrivate(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "private")
	if err := standalonePrivateDir(dataDir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dataDir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("app data directory must be owner-only: %v, %v", info, err)
	}
	s := &standaloneServer{cfg: standaloneConfig{DataDir: dataDir}, origin: "http://127.0.0.1:39441",
		token: strings.Repeat("a", 32), pairCode: strings.Repeat("b", 32),
		market: func() (any, error) { return map[string]bool{"ready": true}, nil }}
	handler := s.handler()
	for _, tc := range []struct {
		name, host, origin, token string
	}{
		{"rebinding host", "example.test:39441", "http://127.0.0.1:39441", s.token},
		{"cross-site origin", "127.0.0.1:39441", "https://example.test", s.token},
		{"missing token", "127.0.0.1:39441", "http://127.0.0.1:39441", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:39441/api/submit", strings.NewReader(`{}`))
			req.Host = tc.host
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("X-FOD-Token", tc.token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != http.StatusForbidden {
				t.Fatalf("untrusted request got status %d", response.Code)
			}
		})
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:39441/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), s.token) ||
		strings.Contains(response.Body.String(), s.pairCode) ||
		!strings.Contains(response.Header().Get("Content-Security-Policy"), "default-src 'none'") ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("local page exposed a credential or lost its guarded shell")
	}
	assetRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:39441/app.js", nil)
	assetResponse := httptest.NewRecorder()
	handler.ServeHTTP(assetResponse, assetRequest)
	if assetResponse.Code != http.StatusOK || strings.Contains(assetResponse.Body.String(), s.token) || strings.Contains(assetResponse.Body.String(), s.pairCode) {
		t.Fatal("public script exposed a credential")
	}
	apiRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:39441/api/actions", nil)
	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, apiRequest)
	if apiResponse.Code != http.StatusForbidden {
		t.Fatalf("private API read without token got status %d", apiResponse.Code)
	}
	for _, tc := range []struct {
		name, host, origin, code string
		status                   int
	}{
		{"wrong code", "127.0.0.1:39441", s.origin, strings.Repeat("c", 32), http.StatusForbidden},
		{"cross-site pairing", "127.0.0.1:39441", "https://example.test", s.pairCode, http.StatusForbidden},
		{"rebinding pairing", "example.test:39441", s.origin, s.pairCode, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"code": tc.code})
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:39441/api/pair", bytes.NewReader(body))
			request.Host = tc.host
			request.Header.Set("Origin", tc.origin)
			result := httptest.NewRecorder()
			handler.ServeHTTP(result, request)
			if result.Code != tc.status || strings.Contains(result.Body.String(), s.token) {
				t.Fatalf("unsafe pairing response: %d %s", result.Code, result.Body.String())
			}
		})
	}
	body, _ := json.Marshal(map[string]string{"code": s.pairCode})
	pair := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:39441/api/pair", bytes.NewReader(body))
	pair.Header.Set("Origin", s.origin)
	paired := httptest.NewRecorder()
	handler.ServeHTTP(paired, pair)
	var result struct {
		Token string `json:"token"`
	}
	if paired.Code != http.StatusOK || json.Unmarshal(paired.Body.Bytes(), &result) != nil || result.Token != s.token {
		t.Fatalf("valid pairing did not return the run token: %d", paired.Code)
	}
	marketRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:39441/api/market", nil)
	marketRequest.Header.Set("X-FOD-Token", result.Token)
	marketResponse := httptest.NewRecorder()
	handler.ServeHTTP(marketResponse, marketRequest)
	if marketResponse.Code != http.StatusOK {
		t.Fatalf("paired browser could not read the market: %d", marketResponse.Code)
	}
	marketRequest.Header.Set("Origin", "https://example.test")
	marketResponse = httptest.NewRecorder()
	handler.ServeHTTP(marketResponse, marketRequest)
	if marketResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin read with token got status %d", marketResponse.Code)
	}
}

func TestStandalonePairingCooldownAndOwnerRecovery(t *testing.T) {
	s := &standaloneServer{origin: "http://127.0.0.1:39441", token: strings.Repeat("a", 32), pairCode: strings.Repeat("b", 32)}
	call := func(body, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, s.origin+"/api/pair", strings.NewReader(body))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		return w
	}
	valid := `{"code":"` + s.pairCode + `"}`
	for i := 0; i < 5; i++ {
		if w := call(`{"code":"wrong"}`, s.origin); w.Code != 403 {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	deadline := s.pairRetryAfter
	for _, body := range []string{`{"code":"wrong"}`, `{`, strings.Repeat("x", 257)} {
		w := call(body, s.origin)
		if w.Code != 429 || w.Header().Get("Retry-After") == "" || strings.Contains(w.Body.String(), s.token) || !deadline.Equal(s.pairRetryAfter) {
			t.Fatal("invalid attempts must be throttled without extending cooldown")
		}
	}
	if w := call(valid, "https://untrusted.example"); w.Code != 403 {
		t.Fatal("cross-origin recovery accepted")
	}
	if w := call(valid, s.origin); w.Code != 200 || !strings.Contains(w.Body.String(), s.token) || s.pairFailed != 0 || !s.pairRetryAfter.IsZero() {
		t.Fatal("owner could not recover during cooldown")
	}
	s.pairFailed = 5
	s.pairRetryAfter = time.Now().Add(-time.Second)
	if w := call(`{"code":"wrong"}`, s.origin); w.Code != 403 || s.pairFailed != 1 {
		t.Fatal("expired cooldown did not reset attempt window")
	}
	if w := call(valid, s.origin); w.Code != 200 {
		t.Fatal("normal pairing failed after cooldown")
	}
}

func TestStandaloneConfirmNeedsLocallyVerifiedReceipt(t *testing.T) {
	s := &standaloneServer{}
	for _, tc := range []struct {
		name, receipt string
		upload        bool
		want          string
	}{
		{name: "missing receipt", want: "verify the seller ciphertext locally"},
		{name: "uploaded receipt", receipt: strings.Repeat("a", 32), upload: true, want: "only a buyer receipt verified by this app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			_ = form.WriteField("operation", "confirm")
			if tc.receipt != "" {
				_ = form.WriteField("receiptId", tc.receipt)
			}
			if tc.upload {
				part, err := form.CreateFormFile("delivery", "receipt.json")
				if err != nil {
					t.Fatal(err)
				}
				_, _ = part.Write([]byte(`{}`))
			}
			_ = form.Close()
			request := httptest.NewRequest(http.MethodPost, "/api/prepare", &body)
			request.Header.Set("Content-Type", form.FormDataContentType())
			if _, err := s.prepare(request); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unsafe confirmation was not refused: %v", err)
			}
		})
	}
}

func TestStandaloneDemoHasNoPrivateOrWriteEndpoints(t *testing.T) {
	origin := "http://127.0.0.1:39441"
	handler := standaloneDemoHandler(origin)
	for _, path := range []string{"/", "/app.js", "/app.css", "/api/mode"} {
		request := httptest.NewRequest(http.MethodGet, origin+path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("demo %s returned %d", path, response.Code)
		}
		if path == "/api/mode" && !strings.Contains(response.Body.String(), `"preview":true`) {
			t.Fatal("demo did not identify itself as synthetic")
		}
	}
	for _, path := range []string{"/api/submit", "/api/prepare", "/api/private/receive", "/api/private/download/test"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			request := httptest.NewRequest(method, origin+path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code == http.StatusOK {
				t.Fatalf("demo accepted %s %s", method, path)
			}
		}
	}
}

func TestStandaloneActionIDsRejectTraversal(t *testing.T) {
	s := &standaloneServer{cfg: standaloneConfig{DataDir: t.TempDir()}}
	for _, id := range []string{"../private", "", strings.Repeat("f", 31), strings.Repeat("g", 32)} {
		if _, err := s.actionDir(id); err == nil {
			t.Fatalf("accepted action ID %q", id)
		}
		if _, err := s.privateDir(id); err == nil {
			t.Fatalf("accepted private file ID %q", id)
		}
	}
}

func TestStandaloneSuggestedFeeRequiresMeasuredSimulationAndNodeCap(t *testing.T) {
	rate := big.NewInt(1000)
	fee, err := standaloneSuggestedFee([]byte("v7 buy prepared for offer; read-only estimate 10000 gas; no transaction signed or sent\n"), rate)
	if err != nil || fee != "12500000" {
		t.Fatalf("wrong suggested fee: %q, %v", fee, err)
	}
	for _, output := range []string{"simulation failed", "read-only estimate 99999999 gas; no transaction signed or sent"} {
		if _, err := standaloneSuggestedFee([]byte(output), rate); err == nil {
			t.Fatalf("accepted unsafe estimate %q", output)
		}
	}
}

func TestStandalonePreparationCannotSignAndWrongNodekeyDoesNotRecordAttempt(t *testing.T) {
	contract := strings.Repeat("ab", 20)
	codeHash := strings.Repeat("cd", 32)
	active := strings.Repeat("ef", 20)
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode node request: %v", err)
			return
		}
		var result any
		switch request.Method {
		case "dna_getCoinbaseAddr":
			result = "0x" + active
		case "bcn_feePerGas":
			result = "1000"
		default:
			t.Errorf("unexpected node method %s", request.Method)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	defer node.Close()
	dataDir := t.TempDir()
	if err := standalonePrivateDir(filepath.Join(dataDir, "actions")); err != nil {
		t.Fatal(err)
	}
	s := &standaloneServer{cfg: standaloneConfig{DataDir: dataDir, Contract: contract, CodeHash: codeHash},
		client: &client{url: node.URL, key: "test", http: newHTTPClient()},
		origin: "http://127.0.0.1:39441", token: "test-token"}
	s.runCommand = func(_ context.Context, args []string, _ string) ([]byte, error) {
		for _, arg := range args {
			if arg == "submit" || arg == "-broadcast" || arg == "-nodekey" {
				t.Fatalf("preparation included a signing argument: %s", arg)
			}
		}
		var path string
		for i := range args {
			if args[i] == "-prepared" && i+1 < len(args) {
				path = args[i+1]
			}
		}
		prepared := preparedAction{Format: preparedPoolV7PublicBuy, Version: actionRequestVersion,
			Operation: "buyPublicSeed", Network: NetworkNamePinned, Contract: contract, CodeHash: codeHash,
			DirectVersion: 7, Amount: "1", Calldata: strings.Repeat("11", 32)}
		raw, err := json.Marshal(prepared)
		if err != nil {
			t.Fatal(err)
		}
		return []byte("v7 buyPublicSeed prepared for offer; read-only estimate 10000 gas; no transaction signed or sent\n"), os.WriteFile(path, raw, 0600)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("operation", "buyPublicSeed")
	_ = form.Close()
	prepare := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:39441/api/prepare", &body)
	prepare.Header.Set("Content-Type", form.FormDataContentType())
	prepare.Header.Set("Origin", s.origin)
	prepare.Header.Set("X-FOD-Token", s.token)
	preparedResponse := httptest.NewRecorder()
	s.handler().ServeHTTP(preparedResponse, prepare)
	if preparedResponse.Code != http.StatusOK {
		t.Fatalf("preparation failed: %s", preparedResponse.Body.String())
	}
	var action standaloneAction
	if err := json.NewDecoder(preparedResponse.Body).Decode(&action); err != nil || action.Attempted || action.Signer != active || action.MaximumFee != "12500000" {
		t.Fatalf("unexpected unsigned action: %+v, %v", action, err)
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dataDir, "other-nodekey")
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(crypto.FromECDSA(key))), 0600); err != nil {
		t.Fatal(err)
	}
	s.cfg.NodeKey = keyPath
	requestBody, _ := json.Marshal(map[string]string{"id": action.ID, "reviewHash": action.ReviewHash})
	submit := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:39441/api/submit", bytes.NewReader(requestBody))
	submit.Header.Set("Origin", s.origin)
	submit.Header.Set("X-FOD-Token", s.token)
	submittedResponse := httptest.NewRecorder()
	s.handler().ServeHTTP(submittedResponse, submit)
	if submittedResponse.Code != http.StatusConflict || !strings.Contains(submittedResponse.Body.String(), "selected signing key") {
		t.Fatalf("wrong key was not refused before signing: %d %s", submittedResponse.Code, submittedResponse.Body.String())
	}
	dir, _ := s.actionDir(action.ID)
	if _, err := os.Stat(filepath.Join(dir, "attempted")); !os.IsNotExist(err) {
		t.Fatalf("wrong signing key recorded an attempt: %v", err)
	}
}

func TestStandaloneNonceReservationBlocksAnotherActionAcrossDataDirs(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := nodekeyAddressOf(key)
	if err != nil {
		t.Fatal(err)
	}
	contract, codeHash := strings.Repeat("ab", 20), strings.Repeat("cd", 32)
	keyPath := filepath.Join(t.TempDir(), "nodekey")
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(crypto.FromECDSA(key))), 0600); err != nil {
		t.Fatal(err)
	}
	nonceDir := filepath.Join(t.TempDir(), "nonce-reservations")
	if err := standalonePrivateDir(nonceDir); err != nil {
		t.Fatal(err)
	}
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode node request: %v", err)
			return
		}
		var result any
		switch request.Method {
		case "dna_getCoinbaseAddr":
			result = "0x" + signer
		case "contract_getStake":
			result = map[string]string{"Hash": "0x" + codeHash}
		case "dna_getBalance":
			result = map[string]uint32{"nonce": 4, "mempoolNonce": 4}
		case "dna_epoch":
			result = map[string]uint16{"epoch": 100}
		default:
			t.Errorf("unexpected node method %s", request.Method)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	defer node.Close()
	makeServer := func(dataDir string) *standaloneServer {
		return &standaloneServer{cfg: standaloneConfig{DataDir: dataDir, Contract: contract, CodeHash: codeHash, NodeKey: keyPath},
			client: &client{url: node.URL, key: "test", http: newHTTPClient()}, nonceDir: nonceDir}
	}
	makeAction := func(s *standaloneServer, id string) standaloneAction {
		t.Helper()
		dir, err := s.actionDir(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		prepared := preparedAction{Format: preparedPoolV7PublicBuy, Version: actionRequestVersion,
			Operation: "buyPublicSeed", Network: NetworkNamePinned, Contract: contract, CodeHash: codeHash,
			DirectVersion: 7, Amount: "1", Calldata: strings.Repeat("11", 32)}
		raw, _ := json.Marshal(prepared)
		for name, contents := range map[string][]byte{"prepared.json": raw, "maximum-fee": []byte("12500000"), "signer": []byte(signer)} {
			if err := os.WriteFile(filepath.Join(dir, name), contents, 0600); err != nil {
				t.Fatal(err)
			}
		}
		a, err := s.readAction(id)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	first := makeServer(filepath.Join(t.TempDir(), "first-data"))
	a := makeAction(first, strings.Repeat("a", 32))
	second := makeServer(filepath.Join(t.TempDir(), "second-data"))
	b := makeAction(second, strings.Repeat("b", 32))
	commands := 0
	first.runCommand = func(_ context.Context, _ []string, _ string) ([]byte, error) {
		commands++
		path, err := first.nonceReservationPath(signer, 5)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			t.Fatalf("signing began without a private durable reservation: %v, %v", info, err)
		}
		return []byte("uncertain send"), errors.New("node response lost")
	}
	second.runCommand = func(_ context.Context, _ []string, _ string) ([]byte, error) {
		commands++
		return nil, nil
	}
	submit := func(s *standaloneServer, action standaloneAction) error {
		body, _ := json.Marshal(map[string]string{"id": action.ID, "reviewHash": action.ReviewHash})
		_, err := s.submit(httptest.NewRequest(http.MethodPost, "/api/submit", bytes.NewReader(body)))
		return err
	}
	if err := submit(first, a); err == nil || !strings.Contains(err.Error(), "needs reconciliation") {
		t.Fatalf("uncertain first send was not retained: %v", err)
	}
	if err := submit(second, b); err == nil || !strings.Contains(err.Error(), "unresolved saved attempt") {
		t.Fatalf("fresh action bypassed previous nonce reservation: %v", err)
	}
	if commands != 1 {
		t.Fatalf("signing command ran %d times", commands)
	}
}

func TestStandaloneNonceReservationReleasesOnlyWhenConsumedOrIncluded(t *testing.T) {
	nonceDir := filepath.Join(t.TempDir(), "nonce-reservations")
	if err := standalonePrivateDir(nonceDir); err != nil {
		t.Fatal(err)
	}
	chainNonce := uint32(4)
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Method != "dna_getBalance" {
			t.Errorf("unexpected node request: %v, %s", err, request.Method)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1,
			"result": map[string]uint32{"nonce": chainNonce}})
	}))
	defer node.Close()
	s := &standaloneServer{nonceDir: nonceDir, client: &client{url: node.URL, key: "test", http: newHTTPClient()}}
	a := standaloneAction{ID: strings.Repeat("a", 32), ReviewHash: strings.Repeat("b", 64), Signer: strings.Repeat("c", 40)}
	if err := s.reserveNonce(a, 5); err != nil {
		t.Fatal(err)
	}
	path, _ := s.nonceReservationPath(a.Signer, 5)
	signed := SubmittedTransaction{Sender: a.Signer, AccountNonce: 5, Outcome: &SubmissionOutcome{Included: false}}
	if err := s.reconcileNonceReservation(a, signed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing receipt cleared unresolved nonce: %v", err)
	}
	chainNonce = 5
	if err := s.reconcileNonceReservation(a, signed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("consumed chain nonce was not released: %v", err)
	}
	if err := s.reserveNonce(a, 6); err != nil {
		t.Fatal(err)
	}
	signed.AccountNonce = 6
	signed.Outcome.Included = true
	if err := s.reconcileNonceReservation(a, signed); err != nil {
		t.Fatal(err)
	}
	path, _ = s.nonceReservationPath(a.Signer, 6)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("included transaction nonce was not released: %v", err)
	}
}
