package main

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func TestBuyTargetRequiresIndependentMatchingPins(t *testing.T) {
	contract := strings.Repeat("11", addressBytes)
	codeHash := strings.Repeat("ab", 32)
	var calls atomic.Int32
	var fail atomic.Bool
	var answerMu sync.Mutex
	deployed := "0x" + codeHash
	answer := &deployed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Method string   `json:"method"`
			Params []string `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Method != "contract_getStake" ||
			len(request.Params) != 1 || request.Params[0] != "0x"+contract {
			http.Error(w, "unexpected contract hash request", http.StatusBadRequest)
			return
		}
		if fail.Load() {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": -1, "message": "unavailable"}})
			return
		}
		answerMu.Lock()
		hash := answer
		answerMu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"Hash": hash}})
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient()}
	for _, tc := range []struct {
		name, target, approved, hash string
		wantRPC                      bool
	}{
		{"missing address", contract, "", codeHash, false},
		{"missing hash", contract, contract, "", false},
		{"malformed hash pin", contract, contract, "abc", false},
		{"substituted target", strings.Repeat("22", addressBytes), contract, codeHash, false},
		{"approved target", contract, "0x" + contract, "0x" + strings.ToUpper(codeHash), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls.Load()
			err := checkBuyTarget(c, tc.target, tc.approved, tc.hash)
			if (err == nil) != tc.wantRPC {
				t.Fatalf("got error %v; expected success=%t", err, tc.wantRPC)
			}
			if (calls.Load() > before) != tc.wantRPC {
				t.Fatalf("unexpected RPC use for %s", tc.name)
			}
		})
	}
	wrong := strings.Repeat("cd", 32)
	answerMu.Lock()
	answer = &wrong
	answerMu.Unlock()
	if err := checkBuyTarget(c, contract, contract, codeHash); err == nil {
		t.Fatal("wrong deployed code hash accepted")
	}
	malformed := "malformed"
	answerMu.Lock()
	answer = &malformed
	answerMu.Unlock()
	if err := checkBuyTarget(c, contract, contract, codeHash); err == nil {
		t.Fatal("malformed deployed code hash accepted")
	}
	answerMu.Lock()
	answer = nil
	answerMu.Unlock()
	if err := checkBuyTarget(c, contract, contract, codeHash); err == nil {
		t.Fatal("missing deployed code hash accepted")
	}
	fail.Store(true)
	if err := checkBuyTarget(c, contract, contract, codeHash); err == nil {
		t.Fatal("node RPC failure accepted")
	}
}

func TestBuyTargetRefusesSubmitBeforeKeyOrBroadcast(t *testing.T) {
	if os.Getenv("FOD_TEST_BUY_TARGET_CHILD") == "1" {
		runSubmit(os.Getenv("FOD_TEST_BUY_PREPARED"), "missing-disposable-nodekey", 1, 230,
			big.NewInt(1), big.NewInt(0), "", true, os.Getenv("FOD_TEST_BUY_RPC"), "", nil,
			os.Getenv("FOD_TEST_BUY_APPROVED"), os.Getenv("FOD_TEST_BUY_CODE_HASH"))
		return
	}
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	contract := strings.Repeat("11", addressBytes)
	codeHash := strings.Repeat("ab", 32)
	buy, err := preparePoolBuy(contract, strings.Repeat("cd", poolBundleBytes), strings.Repeat("22", addressBytes),
		1, "1", "idena-local", nil, 1, identity)
	if err != nil {
		t.Fatal(err)
	}
	buy.Availability = retainedTestCheck([]byte("synthetic encrypted flip"))
	path := filepath.Join(t.TempDir(), "prepared-buy.json")
	if err := writeJSON(path, buy); err != nil {
		t.Fatal(err)
	}
	var reads, sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.Method == "bcn_sendRawTx" {
			sends.Add(1)
		}
		if request.Method == "contract_getStake" {
			reads.Add(1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"Hash": "0x" + codeHash}})
	}))
	defer server.Close()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, approved, hash, want string
		wantReads                  int32
	}{
		{"missing pins", "", "", "independent -contract", 0},
		{"wrong address", strings.Repeat("33", addressBytes), codeHash, "differs from the approved", 0},
		{"wrong code", contract, strings.Repeat("ef", 32), "code hash does not match", 1},
		{"correct pins pass target gate", contract, codeHash, "load nodekey", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := reads.Load()
			cmd := exec.Command(binary, "-test.run=^TestBuyTargetRefusesSubmitBeforeKeyOrBroadcast$")
			cmd.Env = append(os.Environ(), "FOD_TEST_BUY_TARGET_CHILD=1", "FOD_TEST_BUY_PREPARED="+path,
				"FOD_TEST_BUY_RPC="+server.URL, "FOD_TEST_BUY_APPROVED="+tc.approved,
				"FOD_TEST_BUY_CODE_HASH="+tc.hash)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.want) {
				t.Fatalf("submit result %v did not report %q: %s", err, tc.want, output)
			}
			if reads.Load()-before != tc.wantReads || sends.Load() != 0 {
				t.Fatalf("unexpected RPC calls: code reads=%d, broadcasts=%d", reads.Load()-before, sends.Load())
			}
		})
	}
}
