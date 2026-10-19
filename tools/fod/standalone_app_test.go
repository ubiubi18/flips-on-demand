package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	s := &standaloneServer{origin: "http://127.0.0.1:39441", token: "test-token"}
	handler := s.handler()
	for _, tc := range []struct {
		name, host, origin, token string
	}{
		{"rebinding host", "example.test:39441", "http://127.0.0.1:39441", "test-token"},
		{"cross-site origin", "127.0.0.1:39441", "https://example.test", "test-token"},
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
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "test-token") ||
		!strings.Contains(response.Header().Get("Content-Security-Policy"), "default-src 'none'") ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("local page did not receive its guarded shell")
	}
	apiRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:39441/api/actions", nil)
	apiResponse := httptest.NewRecorder()
	handler.ServeHTTP(apiResponse, apiRequest)
	if apiResponse.Code != http.StatusForbidden {
		t.Fatalf("private API read without token got status %d", apiResponse.Code)
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
