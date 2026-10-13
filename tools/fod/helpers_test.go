package main

// Shared test helpers: a valid contract address, a throwaway nodekey, and a
// request-file writer for the modes that read the interface's envelope.

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

const testContract = "1111111111111111111111111111111111111111"

func writeTestKey(t *testing.T, dir, name string) (string, []byte) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := crypto.SaveECDSA(path, key); err != nil {
		t.Fatalf("save key: %v", err)
	}
	return path, crypto.FromECDSA(key)
}

func writeRequest(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "request.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}
	return path
}

// registryStub answers `contract_readData` with one value per storage key, so
// the buyer path can be exercised without a chain.
func registryStub(t *testing.T, values map[string][]byte) *client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.Method != "contract_readData" || len(request.Params) < 2 {
			io.WriteString(w, `{"result":null,"error":{"code":-32601,"message":"stub: only contract_readData"}}`)
			return
		}
		var key string
		_ = json.Unmarshal(request.Params[1], &key)
		value, ok := values[key]
		if !ok {
			io.WriteString(w, `{"result":null,"error":{"code":-32000,"message":"data is nil"}}`)
			return
		}
		io.WriteString(w, `{"result":"0x`+hex.EncodeToString(value)+`","error":null}`)
	}))
	t.Cleanup(server.Close)
	return &client{url: server.URL, http: newHTTPClient()}
}
