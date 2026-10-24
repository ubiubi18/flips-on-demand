package main

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDirectEstimateGatesSigningOnExecutionAndCurrentFee(t *testing.T) {
	success := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string           `json:"method"`
			Params []map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		switch request.Method {
		case "contract_estimateCall":
			if len(request.Params) != 1 || request.Params[0]["method"] != "confirm" || request.Params[0]["amount"] != "0" {
				http.Error(w, "wrong simulation context", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"success": success, "gasUsed": 12000, "error": "invalid proof"}})
		case "bcn_feePerGas":
			_ = json.NewEncoder(w).Encode(map[string]any{"result": 1000})
		default:
			http.Error(w, "unexpected method", 400)
		}
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient()}
	p := &preparedAction{Format: preparedDirectConfirmFormat, Operation: "confirm", Contract: strings.Repeat("aa", 20), Amount: "0", Calldata: strings.Repeat("00", 40)}
	if _, _, err := estimateDirectCall(c, p, strings.Repeat("bb", 20), big.NewInt(1), nil); err == nil {
		t.Fatal("direct call with insufficient fee was accepted")
	}
	budget, rate, err := estimateDirectCall(c, p, strings.Repeat("bb", 20), big.NewInt(100_000_000), nil)
	if err != nil || budget != 14_200 || rate.String() != "1000" {
		t.Fatalf("unexpected gas estimate: %d, %v, %v", budget, rate, err)
	}
	success = false
	if _, _, err := estimateDirectCall(c, p, strings.Repeat("bb", 20), big.NewInt(100_000_000), nil); err == nil {
		t.Fatal("failed contract simulation was accepted")
	}
}

func TestDirectFallbackNeverSimulatesOnRemoteRPC(t *testing.T) {
	p := &preparedAction{Format: preparedDirectFallbackFormat, Operation: directFallbackOperation, Amount: "0"}
	for _, endpoint := range []string{"https://example.com:443", "http://localhost:9009", "http://127.0.0.1.example.com:9009"} {
		c := &client{url: endpoint, http: newHTTPClient()}
		if _, _, err := estimateDirectCall(c, p, strings.Repeat("bb", 20), big.NewInt(1), big.NewInt(1)); err == nil {
			t.Fatalf("fallback sent private keys to %s", endpoint)
		}
	}
	if !directLocalRPC("http://127.0.0.1:9009") || !directLocalRPC("http://[::1]:9009") {
		t.Fatal("loopback node RPC was rejected")
	}
}

func TestBondedV8EstimateIncludesSizeFeeAndReturnsActionSpecificCap(t *testing.T) {
	fee := "0.1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"success": true, "gasUsed": 12000, "minGasLimit": 20000, "txFee": fee}})
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient()}
	p := &preparedAction{Format: bondedV8Format + "accept", Operation: "accept", Contract: strings.Repeat("aa", 20), Amount: "1", Calldata: "01"}
	rate := big.NewInt(98_000_000_000_000)
	cap := new(big.Int).Mul(rate, big.NewInt(5_000_000))
	budget, _, err := estimatePreparedDirectCall(c, p, strings.Repeat("bb", 20), cap, rate)
	if err != nil || budget != 23000 || p.EstimatedMaxFee != "2364000000000000000" {
		t.Fatalf("unexpected v8 fee: %d %s %v", budget, p.EstimatedMaxFee, err)
	}
	if _, _, err = estimatePreparedDirectCall(c, p, strings.Repeat("bb", 20), big.NewInt(2_000_000_000_000_000_000), rate); err == nil {
		t.Fatal("insufficient total fee cap accepted")
	}
	fee = ""
	if _, _, err = estimatePreparedDirectCall(c, p, strings.Repeat("bb", 20), cap, rate); err == nil {
		t.Fatal("missing transaction fee accepted")
	}
}

func TestDirectListingQuotesRefundableBuyerBond(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Method != "bcn_feePerGas" {
			http.Error(w, "unexpected request", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "83333333333300"})
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient()}
	warning := directListingRiskWarning(c, big.NewInt(1_000_000_000_000_000_000))
	if !strings.Contains(warning, "520.83 iDNA") || !strings.Contains(warning, "on top of the 1.00 iDNA price") ||
		!strings.Contains(warning, "permanently retires") {
		t.Fatalf("listing warning omits the buyer bond and refund risk: %s", warning)
	}
}
