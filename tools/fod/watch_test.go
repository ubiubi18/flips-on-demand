package main

// The refund watch signs whatever prepared action it writes, so that file has to
// live somewhere no other local user can pre-create or replace: a private
// directory with an owner-only action inside it.

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWatchActionLivesInAPrivateDirectory(t *testing.T) {
	action := PreparePoolRefund(&actionRequest{Network: "idena", Contract: strings.Repeat("ab", 20)},
		&refundArguments{OrderID: 9})
	path, cleanup, err := writeWatchAction(9, action)
	if err != nil {
		t.Fatalf("write the watch action: %v", err)
	}
	defer cleanup()

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat the work directory: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("the watch work directory is %v, expected 0700", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the action: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("the watch action is %v, expected 0600", fileInfo.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the action: %v", err)
	}
	var back preparedAction
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("the action does not parse: %v", err)
	}
	if back.OfferID != 9 || back.Operation != operationRefund {
		t.Fatalf("the action names %s/%d", back.Operation, back.OfferID)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the work directory survived cleanup: %v", err)
	}
}

// stubWatchNode answers the refund watch's RPC calls from one order record and
// counts the raw transactions the watch broadcasts.
func stubWatchNode(t *testing.T, record []byte, head uint64) (*client, string, func() int) {
	t.Helper()
	var mu sync.Mutex
	var sent [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result := any(nil)
		switch request.Method {
		case "bcn_lastBlock":
			result = map[string]any{"height": head}
		case "contract_readData":
			result = "0x" + hex.EncodeToString(record)
		case "dna_getBalance":
			result = map[string]any{"nonce": 0}
		case "dna_epoch":
			result = map[string]any{"epoch": 0}
		case "bcn_feePerGas":
			result = "10000000000000000"
		case "bcn_sendRawTx":
			rawHex, _ := request.Params[0].(string)
			raw, err := hex.DecodeString(strings.TrimPrefix(rawHex, "0x"))
			if err != nil {
				http.Error(w, "bad raw transaction", http.StatusBadRequest)
				return
			}
			mu.Lock()
			sent = append(sent, raw)
			mu.Unlock()
			result = "0x" + strings.Repeat("11", 32)
		case "bcn_txReceipt":
			result = map[string]any{"success": true, "gasUsed": 13_579, "txFee": "12.1"}
		default:
			http.Error(w, "unexpected method "+request.Method, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	}))
	t.Cleanup(server.Close)
	return &client{url: server.URL, http: newHTTPClient()}, server.URL, func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(sent)
	}
}

// watchOrderFixture mirrors the escrow's 150-byte PoolOrder record: bundle,
// buyer, delivery key, paid, funded-at, buy nonce and the state byte.
func watchOrderFixture(state byte, fundedAt uint64) []byte {
	record := make([]byte, marketPoolOrderBytes)
	for i := 0; i < 32; i++ {
		record[i] = 0xab
	}
	for i := 32; i < 52; i++ {
		record[i] = 0xcd
	}
	binary.BigEndian.PutUint64(record[133:141], fundedAt)
	binary.BigEndian.PutUint64(record[141:149], 7)
	record[149] = state
	return record
}

func TestWatchSubmitsTheRefundOnceTheWindowOpens(t *testing.T) {
	keyPath, _ := writeTestKey(t, t.TempDir(), "nodekey")
	// A delivered order funded at 1000 has its first refundable block at 1003.
	node, rpcURL, sent := stubWatchNode(t, watchOrderFixture(2, 1000), 1003)
	if err := runWatch(node, 7, testContract, "testnet", keyPath, 1, rpcURL, ""); err != nil {
		t.Fatalf("watch the order: %v", err)
	}
	if got := sent(); got != 1 {
		t.Fatalf("the watch sent %d transactions, expected exactly one refund", got)
	}
}

func TestWatchLeavesASettledOrderAlone(t *testing.T) {
	keyPath, _ := writeTestKey(t, t.TempDir(), "nodekey")
	node, rpcURL, sent := stubWatchNode(t, watchOrderFixture(3, 1000), 1004)
	if err := runWatch(node, 7, testContract, "testnet", keyPath, 1, rpcURL, ""); err != nil {
		t.Fatalf("watch the order: %v", err)
	}
	if got := sent(); got != 0 {
		t.Fatalf("a settled order still sent %d transactions", got)
	}
}
