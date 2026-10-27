package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
)

func buyerRelaySignedTx(t *testing.T, to string, kind types.TxType, signed bool, method string) string {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	address, err := hex.DecodeString(strings.TrimPrefix(to, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	var target common.Address
	copy(target[:], address)
	call, err := attachments.CreateCallContractAttachment(method, []byte{1, 2, 3}).ToBytes()
	if err != nil {
		t.Fatal(err)
	}
	tx := &types.Transaction{AccountNonce: 1, Epoch: 1, Type: kind, To: &target,
		Amount: big.NewInt(0), MaxFee: big.NewInt(1000), Tips: big.NewInt(0), Payload: call}
	if signed {
		tx, err = types.SignTx(tx, key)
		if err != nil {
			t.Fatal(err)
		}
	}
	raw, err := tx.ToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return "0x" + hex.EncodeToString(raw)
}

func TestBuyerRelayPinsNodeAndContract(t *testing.T) {
	const key = "server-only-test-key"
	contract := "0x" + strings.Repeat("ab", addressBytes)
	other := "0x" + strings.Repeat("cd", addressBytes)
	forwarded := 0
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		var request struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Key    string          `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Key != key {
			t.Errorf("node request did not keep the server key: %v, %q", err, request.Key)
		}
		switch request.Method {
		case "contract_readData":
			io.WriteString(w, `{"result":null,"error":{"code":-32000,"message":"data is nil"}}`)
		case "ipfs_get":
			io.WriteString(w, `{"result":"0x0102"}`)
		default:
			io.WriteString(w, `{"result":{"height":123}}`)
		}
	}))
	defer node.Close()
	handler, err := newBuyerRelayHandler(&client{url: node.URL, key: key, http: newHTTPClient()}, contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newBuyerRelayHandler(&client{url: node.URL, http: newHTTPClient()}, contract); err == nil {
		t.Fatal("relay accepted a node without a server-only API key")
	}
	if _, err := newBuyerRelayHandler(&client{url: "https://node.example/rpc", key: key, http: newHTTPClient()}, contract); err == nil {
		t.Fatal("relay accepted a non-loopback node")
	}
	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	for _, request := range []string{
		`{"method":"bcn_lastBlock","params":[],"id":7}`,
		fmt.Sprintf(`{"method":"contract_getStake","params":[%q],"id":8}`, contract),
		fmt.Sprintf(`{"method":"bcn_sendRawTx","params":[%q],"id":9}`, buyerRelaySignedTx(t, contract, types.CallContractTx, true, "queueBuy")),
		fmt.Sprintf(`{"method":"ipfs_get","params":[%q],"id":10}`, cidRawSha256([]byte{1, 2})),
	} {
		w := post(request)
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), key) || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("allowed request: status %d, headers %v, body %s", w.Code, w.Header(), w.Body.String())
		}
	}
	if got := post(fmt.Sprintf(`{"method":"contract_readData","params":[%q,"v9/order/1","hex"],"id":11}`, contract)); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"message":"data is nil"`) {
		t.Fatalf("missing storage error was not preserved: %d %s", got.Code, got.Body.String())
	}
	before := forwarded
	for _, request := range []string{
		`{"method":"dna_sendTransaction","params":[],"id":1}`,
		`{"method":"dna_getCoinbaseAddr","params":[],"id":1}`,
		fmt.Sprintf(`{"method":"contract_getStake","params":[%q],"id":1}`, other),
		fmt.Sprintf(`{"method":"bcn_sendRawTx","params":[%q],"id":1}`, buyerRelaySignedTx(t, other, types.CallContractTx, true, "queueBuy")),
		fmt.Sprintf(`{"method":"bcn_sendRawTx","params":[%q],"id":1}`, buyerRelaySignedTx(t, contract, types.SendTx, true, "queueBuy")),
		fmt.Sprintf(`{"method":"bcn_sendRawTx","params":[%q],"id":1}`, buyerRelaySignedTx(t, contract, types.CallContractTx, false, "queueBuy")),
		fmt.Sprintf(`{"method":"bcn_sendRawTx","params":[%q],"id":1}`, buyerRelaySignedTx(t, contract, types.CallContractTx, true, "queueSell")),
		`{"method":"bcn_lastBlock","params":[],"id":1,"key":"client-key"}`,
		`[{"method":"bcn_lastBlock","params":[],"id":1}]`,
		fmt.Sprintf(`{"method":"ipfs_get","params":[%q],"id":1}`, "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"),
	} {
		w := post(request)
		if w.Code == http.StatusOK {
			t.Fatalf("unsafe request accepted: %s", request)
		}
	}
	if forwarded != before {
		t.Fatalf("unsafe requests reached the node: %d new calls", forwarded-before)
	}
	get := httptest.NewRequest(http.MethodGet, "/rpc", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, get)
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET should be refused: %d %v", w.Code, w.Header())
	}
}

func TestBuyerRelayLimitsIPAndContent(t *testing.T) {
	contract := "0x" + strings.Repeat("ab", addressBytes)
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"result":"0x`+strings.Repeat("00", maxRetainedFlipBytes+1)+`"}`)
	}))
	defer node.Close()
	interfaceHandler, err := newBuyerRelayHandler(&client{url: node.URL, key: "test-node-key", http: newHTTPClient()}, contract)
	if err != nil {
		t.Fatal(err)
	}
	handler := interfaceHandler.(*buyerRelay)
	for i := 0; i < buyerRelayRatePerMinute; i++ {
		if !handler.allowIP("127.0.0.1:1234") {
			t.Fatalf("rate limit reached early at %d", i)
		}
	}
	if handler.allowIP("127.0.0.1:5678") || !handler.allowIP("127.0.0.2:1234") {
		t.Fatal("rate limit must use socket IP, not source port")
	}
	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(
		fmt.Sprintf(`{"method":"ipfs_get","params":[%q],"id":1}`, cidRawSha256([]byte{1, 2}))))
	req.RemoteAddr = "127.0.0.2:1234"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "node content exceeds the buyer limit") {
		t.Fatalf("oversized IPFS result was passed through: %d %s", w.Code, w.Body.String())
	}
}
