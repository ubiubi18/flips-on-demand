package main

// The registry reader, over a record built to the escrow's own pool layout.

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// poolRecord builds a PoolBundle record the way the escrow writes one now:
// author (20), manifest commitment (32), inventory commitment (32), units (8),
// beacon (8), strikes (4), the buy nonce (8), and the covered flip count (4).
// The one pool's reserve and unit count come from the running counters.
func poolRecord(author string, units uint64) []byte {
	return poolRecordBytes(mustDecode(author), filled(0x44, 32), filled(0x55, 32), units, 0, 0, 0, 0)
}

func mustDecode(value string) []byte {
	raw, err := hex.DecodeString(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestRegistryPoolBundleReadsTheCommitmentKey(t *testing.T) {
	contract := strings.Repeat("ab", 20)
	commitment := strings.Repeat("cd", 32)
	item := fmt.Sprintf(`{"key":"%s","value":"%s"}`,
		commitment, hex.EncodeToString(poolRecord(strings.Repeat("11", 20), 6)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if request.Method == "contract_readData" {
			key, _ := request.Params[1].(string)
			value, ok := poolStubCounters(key)
			if !ok {
				http.Error(w, "unexpected key "+key, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": value})
			return
		}
		io.WriteString(w, fmt.Sprintf(`{"result":{"items":[%s],"continuationToken":null},"error":null}`, item))
	}))
	defer server.Close()

	c := &client{url: server.URL, http: newHTTPClient()}
	listing, err := registryPoolBundle(c, "0x"+contract, commitment)
	if err != nil {
		t.Fatalf("read the pool bundle: %v", err)
	}
	if listing.Reserve != "50000" || listing.Units != 6 {
		t.Fatalf("the pool bundle read as reserve %s against %d units", listing.Reserve, listing.Units)
	}

	_, err = registryPoolBundle(c, "0x"+contract, strings.Repeat("ee", 32))
	if err == nil {
		t.Fatal("a bundle that is not in the registry was read")
	}
	if !strings.Contains(err.Error(), "not in the registry") {
		t.Fatalf("the refusal is %q, expected it to say the bundle is absent", err)
	}
}

func TestRegistryOpenPoolOrderCountRefusesMalformedState(t *testing.T) {
	contract := strings.Repeat("ab", 20)
	commitment := strings.Repeat("cd", 32)
	var raw []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if request.Method != "contract_iterateMap" || request.Params[1] != "popen/" {
			http.Error(w, "unexpected RPC request", http.StatusBadRequest)
			return
		}
		items := ""
		if raw != nil {
			items = fmt.Sprintf(`{"key":"%s","value":"%s"}`, commitment, hex.EncodeToString(raw))
		}
		io.WriteString(w, fmt.Sprintf(`{"result":{"items":[%s],"continuationToken":null},"error":null}`, items))
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient()}
	count, err := registryOpenPoolOrderCount(c, contract, commitment)
	if err != nil || count != 0 {
		t.Fatalf("absent open-order record: count %d, error %v", count, err)
	}
	raw = make([]byte, 16)
	binary.BigEndian.PutUint64(raw[:8], 1)
	binary.BigEndian.PutUint64(raw[8:], 2)
	count, err = registryOpenPoolOrderCount(c, contract, commitment)
	if err != nil || count != 2 {
		t.Fatalf("two open orders: count %d, error %v", count, err)
	}
	binary.BigEndian.PutUint64(raw[8:], 1)
	if _, err := registryOpenPoolOrderCount(c, contract, commitment); err == nil {
		t.Fatal("duplicate open order was accepted")
	}
	raw = make([]byte, 8*(maxOpenPoolOrders+1))
	if _, err := registryOpenPoolOrderCount(c, contract, commitment); err == nil {
		t.Fatal("over-capacity open-order record was accepted")
	}
}
