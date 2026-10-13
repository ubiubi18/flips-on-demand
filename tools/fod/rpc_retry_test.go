package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A node under load answers with an empty body now and then, and one blank
// answer used to end a whole sweep. One retry turns that transient into a
// delay; an answer the node itself gives is returned as it is, and a raw
// transaction send is never repeated (`docs/PROGRESS.md`, 2026-09-26).
func TestABlankAnswerIsRetriedOnceAndASendIsNot(t *testing.T) {
	previous := rpcRetryDelay
	rpcRetryDelay = 0
	t.Cleanup(func() { rpcRetryDelay = previous })

	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &request)
		counts[request.Method]++
		switch request.Method {
		case "dna_epoch":
			if counts[request.Method] == 1 {
				return // the blank body
			}
			io.WriteString(w, `{"result":{"epoch":228},"error":null}`)
		case "bcn_sendRawTx":
			return // the blank body, which must not be retried
		default: // an answered error
			io.WriteString(w, `{"result":null,"error":{"code":-32000,"message":"the node said no"}}`)
		}
	}))
	t.Cleanup(server.Close)
	c := &client{url: server.URL, http: newHTTPClient()}

	var epoch struct {
		Epoch uint16 `json:"epoch"`
	}
	if err := c.call("dna_epoch", []any{}, &epoch); err != nil || epoch.Epoch != 228 {
		t.Fatalf("a blank answer was not retried: epoch %d, err %v", epoch.Epoch, err)
	}
	if counts["dna_epoch"] != 2 {
		t.Fatalf("the retried read was sent %d times, expected 2", counts["dna_epoch"])
	}

	var hash string
	if err := c.call("bcn_sendRawTx", []any{"0x00"}, &hash); err == nil {
		t.Fatal("a blank answer to a raw send was accepted")
	}
	if counts["bcn_sendRawTx"] != 1 {
		t.Fatalf("a raw send was repeated %d times, expected 1", counts["bcn_sendRawTx"])
	}

	var tx string
	if err := c.call("bcn_transaction", []any{"0x00"}, &tx); err == nil {
		t.Fatal("an answered error was accepted")
	}
	if counts["bcn_transaction"] != 1 {
		t.Fatalf("an answered error was retried %d times, expected 1", counts["bcn_transaction"])
	}
}
