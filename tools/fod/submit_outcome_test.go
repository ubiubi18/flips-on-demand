package main

// `SPEC.md` 4.3 step 7: the record a submission leaves behind. The node's three
// answers are all exercised - a receipt, `null` while the transaction is not
// mined, and the error its receipt converter raises on a missing receipt.

import (
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stubNode answers named RPC methods with a fixed result, so a check can be
// exercised against the node's own shapes without a node.
func stubNode(t *testing.T, results map[string]string) *client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		result, ok := results[request.Method]
		if !ok {
			io.WriteString(w, `{"result":null,"error":{"code":-32601,"message":"method not found"}}`)
			return
		}
		io.WriteString(w, `{"result":`+result+`,"error":null}`)
	}))
	t.Cleanup(server.Close)
	return &client{url: server.URL, http: newHTTPClient()}
}

func TestSubmitRefusesWhatTheNodeCannotAccept(t *testing.T) {
	const sender = "19e7e376e7c213b7e7e7e46cc70a5dd086daff2a"
	cases := []struct {
		name    string
		nonce   uint32
		epoch   uint16
		account string
		chain   string
		refusal string
	}{
		{name: "a nonce equal to the account's", nonce: 4, epoch: 0, account: `{"nonce":4}`, chain: `{"epoch":0}`, refusal: "has to be higher: use 5"},
		{name: "a nonce below the account's", nonce: 2, epoch: 0, account: `{"nonce":4}`, chain: `{"epoch":0}`, refusal: "use 5"},
		{name: "a nonce above the account's", nonce: 5, epoch: 0, account: `{"nonce":4}`, chain: `{"epoch":0}`},
		{name: "an epoch behind the chain's", nonce: 5, epoch: 3, account: `{"nonce":4}`, chain: `{"epoch":4}`, refusal: "cannot name epoch 3"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			node := stubNode(t, map[string]string{
				"dna_getBalance": test.account,
				"dna_epoch":      test.chain,
			})
			err := checkAccountAgainstChain(node, sender, test.nonce, test.epoch)
			switch {
			case test.refusal == "" && err != nil:
				t.Fatalf("the transaction was refused: %v", err)
			case test.refusal != "" && err == nil:
				t.Fatalf("the transaction was accepted; expected a refusal naming %q", test.refusal)
			case test.refusal != "" && !strings.Contains(err.Error(), test.refusal):
				t.Fatalf("the refusal is %q, expected it to name %q", err, test.refusal)
			}
		})
	}

	t.Run("the account cannot be read", func(t *testing.T) {
		err := checkAccountAgainstChain(stubNode(t, map[string]string{"dna_epoch": `{"epoch":0}`}), sender, 5, 0)
		if err == nil || !strings.Contains(err.Error(), "could not be read") {
			t.Fatalf("an unreadable account was accepted, or the refusal is unclear: %v", err)
		}
	})
}

func TestDirectSpendBalanceRequiresFullMaximumCostAndClearNonce(t *testing.T) {
	const sender = "19e7e376e7c213b7e7e7e46cc70a5dd086daff2a"
	amount := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	maxFee := new(big.Int).Set(amount)
	tips := big.NewInt(1)
	for _, test := range []struct {
		name    string
		account string
		nonce   uint32
		wantErr string
	}{
		{name: "sufficient", account: `{"balance":"2.000000000000000001","nonce":3,"mempoolNonce":3}`, nonce: 4},
		{name: "one atom short", account: `{"balance":"2","nonce":3,"mempoolNonce":3}`, nonce: 4, wantErr: "cannot cover"},
		{name: "pending nonce", account: `{"balance":"3","nonce":3,"mempoolNonce":4}`, nonce: 4, wantErr: "pending transaction"},
		{name: "nonce gap", account: `{"balance":"3","nonce":3,"mempoolNonce":3}`, nonce: 5, wantErr: "next executable nonce"},
		{name: "invalid balance", account: `{"balance":"unknown","nonce":3,"mempoolNonce":3}`, nonce: 4, wantErr: "balance is invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			node := stubNode(t, map[string]string{"dna_getBalance": test.account})
			err := checkDirectSpendBalance(node, sender, test.nonce, amount, tips, maxFee)
			if test.wantErr == "" && err != nil {
				t.Fatalf("sufficient account refused: %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("refusal = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestDirectBroadcastKeepsSignedHashWhenSendResponseIsLost(t *testing.T) {
	previous := outcomeRetryDelay
	outcomeRetryDelay = 0
	t.Cleanup(func() { outcomeRetryDelay = previous })
	out := filepath.Join(t.TempDir(), "signed.json")
	savedAtSend := make(chan []byte, 1)
	var sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode RPC: %v", err)
			return
		}
		switch request.Method {
		case "bcn_sendRawTx":
			sends.Add(1)
			data, _ := os.ReadFile(out)
			savedAtSend <- data
			// An empty response can follow acceptance by a node. It is ambiguous.
		case "bcn_txReceipt":
			io.WriteString(w, `{"result":null,"error":null}`)
		}
	}))
	t.Cleanup(server.Close)
	raw := []byte{0xf8, 0x01, 0x02}
	submitted := &SubmittedTransaction{Format: "fod.submitted-transaction", Broadcast: true}
	err := broadcastDirect(&client{url: server.URL, http: newHTTPClient()}, raw, out, submitted)
	if err == nil || !strings.Contains(err.Error(), "requires reconciliation") {
		t.Fatalf("ambiguous send was treated as final: %v", err)
	}
	if sends.Load() != 1 {
		t.Fatalf("raw transaction was sent %d times", sends.Load())
	}
	var before SubmittedTransaction
	if err := json.Unmarshal(<-savedAtSend, &before); err != nil || before.TxHash == "" || before.Outcome == nil {
		t.Fatalf("signed transaction was not saved before send: %v, %+v", err, before)
	}
	var after SubmittedTransaction
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &after); err != nil || after.TxHash != before.TxHash || after.NodeHash != "" || after.Outcome == nil || !strings.Contains(after.Outcome.Note, "unknown") {
		t.Fatalf("ambiguous outcome was not preserved: %v, %+v", err, after)
	}
	info, err := os.Stat(out)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("signed transaction is not owner-only: %v, %v", err, info)
	}
}

func TestDirectBroadcastRecordsAcknowledgedReceipt(t *testing.T) {
	previous := outcomeRetryDelay
	outcomeRetryDelay = 0
	t.Cleanup(func() { outcomeRetryDelay = previous })
	out := filepath.Join(t.TempDir(), "signed.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode RPC: %v", err)
			return
		}
		switch request.Method {
		case "bcn_sendRawTx":
			data, err := os.ReadFile(out)
			if err != nil {
				t.Errorf("signed record missing before send: %v", err)
				return
			}
			var saved SubmittedTransaction
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Errorf("decode signed record: %v", err)
				return
			}
			io.WriteString(w, `{"result":"`+saved.TxHash+`","error":null}`)
		case "bcn_txReceipt":
			io.WriteString(w, `{"result":{"method":"startFallback","success":true,"gasUsed":100,"txFee":"1"},"error":null}`)
		}
	}))
	t.Cleanup(server.Close)
	submitted := &SubmittedTransaction{Format: "fod.submitted-transaction", Broadcast: true}
	if err := broadcastDirect(&client{url: server.URL, http: newHTTPClient()}, []byte{0xf8, 0x03}, out, submitted); err != nil {
		t.Fatalf("acknowledged direct transaction refused: %v", err)
	}
	if submitted.TxHash == "" || submitted.NodeHash != submitted.TxHash || submitted.Outcome == nil || !submitted.Outcome.Included || !submitted.Outcome.Success {
		t.Fatalf("acknowledged result was not reconciled: %+v", submitted)
	}
	var saved SubmittedTransaction
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil || saved.NodeHash != saved.TxHash || saved.Outcome == nil || !saved.Outcome.Included {
		t.Fatalf("acknowledged result was not saved: %v, %+v", err, saved)
	}
}

func stubReceiptNode(t *testing.T, answer func(call int, w http.ResponseWriter)) *client {
	t.Helper()
	previous := outcomeRetryDelay
	outcomeRetryDelay = 0
	t.Cleanup(func() { outcomeRetryDelay = previous })

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		answer(calls, w)
	}))
	t.Cleanup(server.Close)
	return &client{url: server.URL, http: newHTTPClient()}
}

func TestRecordOutcomeKeepsWhatTheNodeSaid(t *testing.T) {
	cases := []struct {
		name   string
		answer func(call int, w http.ResponseWriter)
		check  func(t *testing.T, outcome *SubmissionOutcome)
	}{
		{
			name: "a mined contract call",
			answer: func(_ int, w http.ResponseWriter) {
				io.WriteString(w, `{"result":{"method":"fulfill","success":true,"error":"","gasUsed":1420000,"txFee":"0.0012"},"error":null}`)
			},
			check: func(t *testing.T, outcome *SubmissionOutcome) {
				if !outcome.Included || !outcome.Success {
					t.Fatalf("a mined call read as %+v", outcome)
				}
				if outcome.GasUsed != 1420000 || outcome.TxFee != "0.0012" || outcome.Error != "" {
					t.Fatalf("the receipt details read as %+v", outcome)
				}
			},
		},
		{
			name: "a call the contract refused",
			answer: func(_ int, w http.ResponseWriter) {
				io.WriteString(w, `{"result":{"method":"fulfill","success":false,"error":"wrong_amount","gasUsed":900000,"txFee":"0.0009"},"error":null}`)
			},
			check: func(t *testing.T, outcome *SubmissionOutcome) {
				if !outcome.Included || outcome.Success {
					t.Fatalf("a refused call read as %+v", outcome)
				}
				if outcome.Error != "wrong_amount" {
					t.Fatalf("the refusal does not carry the contract's error: %+v", outcome)
				}
			},
		},
		{
			name: "not mined yet",
			answer: func(_ int, w http.ResponseWriter) {
				io.WriteString(w, `{"result":null,"error":null}`)
			},
			check: func(t *testing.T, outcome *SubmissionOutcome) {
				if outcome.Included {
					t.Fatalf("a null receipt read as included: %+v", outcome)
				}
				if !strings.Contains(outcome.Note, "no receipt yet") {
					t.Fatalf("the note is %q, expected it to say there is no receipt yet", outcome.Note)
				}
			},
		},
		{
			name: "the node's converter failed",
			answer: func(_ int, w http.ResponseWriter) {
				io.WriteString(w, `{"result":null,"error":{"code":-32000,"message":"missing receipt"}}`)
			},
			check: func(t *testing.T, outcome *SubmissionOutcome) {
				if outcome.Included {
					t.Fatalf("an errored lookup read as included: %+v", outcome)
				}
				if !strings.Contains(outcome.Note, "missing receipt") {
					t.Fatalf("the note is %q, expected the node's own message in it", outcome.Note)
				}
			},
		},
		{
			name: "mined on the second look",
			answer: func(call int, w http.ResponseWriter) {
				if call == 1 {
					io.WriteString(w, `{"result":null,"error":null}`)
					return
				}
				io.WriteString(w, `{"result":{"method":"fulfill","success":true,"gasUsed":195582418,"txFee":"0.16"},"error":null}`)
			},
			check: func(t *testing.T, outcome *SubmissionOutcome) {
				if !outcome.Included || !outcome.Success || outcome.GasUsed != 195582418 {
					t.Fatalf("the retry read as %+v", outcome)
				}
			},
		},
		{
			name: "the node's receipt converter panicked",
			answer: func(_ int, w http.ResponseWriter) {
				// The RPC server recovered the handler panic and answered with
				// nothing at all (docs/PROGRESS.md, H-12: `convertReceipt` on a
				// receipt that does not exist yet).
				w.WriteHeader(http.StatusOK)
			},
			check: func(t *testing.T, outcome *SubmissionOutcome) {
				if outcome.Included {
					t.Fatalf("an empty answer read as included: %+v", outcome)
				}
				if !strings.Contains(outcome.Note, "no receipt yet") {
					t.Fatalf("the note is %q, expected it to say there is no receipt yet", outcome.Note)
				}
				if strings.Contains(outcome.Note, "decode RPC response") {
					t.Fatalf("the note blames a decode failure for the node's own panic: %q", outcome.Note)
				}
			},
		},
		{
			name: "mined after the converter panicked",
			answer: func(call int, w http.ResponseWriter) {
				if call == 1 {
					w.WriteHeader(http.StatusOK)
					return
				}
				io.WriteString(w, `{"result":{"method":"addReserve","success":true,"gasUsed":3731,"txFee":"18.2"},"error":null}`)
			},
			check: func(t *testing.T, outcome *SubmissionOutcome) {
				if !outcome.Included || !outcome.Success || outcome.GasUsed != 3731 {
					t.Fatalf("the retry after a panic read as %+v", outcome)
				}
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			outcome := recordOutcome(stubReceiptNode(t, test.answer), "0xabc")
			if outcome.CheckedAt == "" {
				t.Fatal("the outcome carries no timestamp")
			}
			test.check(t, outcome)
		})
	}
}

func TestRecordOutcomePollsOnceAndStops(t *testing.T) {
	calls := 0
	node := stubReceiptNode(t, func(_ int, w http.ResponseWriter) {
		calls++
		io.WriteString(w, `{"result":null,"error":null}`)
	})
	started := time.Now()
	recordOutcome(node, "0xabc")
	if calls != 2 {
		t.Fatalf("the node was asked %d times, expected two looks", calls)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("the second look took %s, expected the test delay to apply", elapsed)
	}
}
