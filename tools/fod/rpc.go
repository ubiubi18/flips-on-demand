package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

// rpcRetryDelay is how long a read waits before its one retry. It is a variable
// so a test can take it to zero.
var rpcRetryDelay = 2 * time.Second

// Client support limit, not a chain consensus limit. Applied to decompressed
// bytes as well as declared lengths, including chunked/lengthless responses.
const maxNetworkResponseBytes int64 = 16 * 1024 * 1024

func readBoundedResponse(resp *http.Response, limit int64) ([]byte, error) {
	if resp.ContentLength > limit {
		return nil, answeredError{fmt.Errorf("response exceeds the %d-byte client limit", limit)}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, answeredError{fmt.Errorf("response exceeds the %d-byte client limit", limit)}
	}
	return raw, nil
}

// answeredError marks an error the node itself reported. A retry cannot change
// an answer, so only failures that are not answers are retried.
type answeredError struct{ err error }

func (e answeredError) Error() string { return e.err.Error() }
func (e answeredError) Unwrap() error { return e.err }

// errEmptyResponse is the node's recovered-panic path: a handler panicked, the
// RPC server recovered it per request, and the client is handed a response with
// no body. `bcn_txReceipt` does this for a transaction that is not in a block
// yet (`docs/PROGRESS.md`, H-12). It is a transient, not a client-side decode
// failure, and callers that poll must say so.
var errEmptyResponse = errors.New("the node recovered from a handler panic and answered with no body")

// retryableRPC reports whether one more attempt can help. A node under load
// answers with an empty body now and then - its own handlers recover per
// request (`docs/SOURCES.md`) - and for a sweep that walks thousands of blocks
// a single blank answer used to end the run. A raw transaction send is never
// retried: the same bytes twice would at best be refused and at worst hide
// what happened.
func retryableRPC(method string, err error) bool {
	if method == "bcn_sendRawTx" || strings.HasPrefix(method, "dna_send") {
		return false
	}
	var answered answeredError
	return !errors.As(err, &answered)
}

// httpClient is a minimal JSON-RPC client for the node's HTTP endpoint.
type httpClient struct {
	client *http.Client
}

func newHTTPClient() *httpClient {
	return &httpClient{client: &http.Client{
		Timeout: 60 * time.Second,
		// RPC bodies contain the endpoint's API key and may contain signed
		// transactions. A redirect must not send them to a different service.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

type rpcRequest struct {
	Method string `json:"method"`
	Params any    `json:"params"`
	ID     int    `json:"id"`
	Key    string `json:"key,omitempty"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// get reads one indexer endpoint and decodes the whole answer into `out`.
//
// The public explorer wraps its payload in `result` like the node's RPC does, but
// it puts pagination beside that field rather than inside it, so the caller
// models the envelope it needs instead of receiving only `result`. An `error`
// field is turned into an error before the caller sees anything.
func (c *httpClient) get(url string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := readBoundedResponse(resp, maxNetworkResponseBytes)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("indexer answered HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode indexer response: %w", err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("indexer: %s", envelope.Error.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// call sends one JSON-RPC request and gives a failure that is not an answer one
// more attempt (`retryableRPC`).
func (c *httpClient) call(url, key, method string, params any, out any) error {
	if err := checkKeyedRPCURL(url, key); err != nil {
		return err
	}
	err := c.callOnce(url, key, method, params, out)
	if err == nil || !retryableRPC(method, err) {
		return err
	}
	time.Sleep(rpcRetryDelay)
	return c.callOnce(url, key, method, params, out)
}

func checkKeyedRPCURL(endpoint, key string) error {
	if key == "" {
		return nil
	}
	parsed, err := neturl.Parse(endpoint)
	if err != nil {
		return err
	}
	if !strings.EqualFold(parsed.Scheme, "http") {
		return nil
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback() {
		return nil
	}
	return fmt.Errorf("node RPC API key requires HTTPS outside loopback")
}

func (c *httpClient) callOnce(url, key, method string, params any, out any) error {
	body, err := json.Marshal(rpcRequest{Method: method, Params: params, ID: 1, Key: key})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return answeredError{fmt.Errorf("rpc %s answered HTTP %d", method, resp.StatusCode)}
	}
	raw, err := readBoundedResponse(resp, maxNetworkResponseBytes)
	if err != nil {
		return err
	}
	// A transaction that is not in a block yet makes the node's receipt
	// converter panic on the missing receipt; the RPC server recovers per
	// request and the client then sees a response with no body at all
	// (`docs/PROGRESS.md`, H-12, with the node's own stack trace). That is not
	// a decode failure and must not be reported as one.
	if len(bytes.TrimSpace(raw)) == 0 {
		return errEmptyResponse
	}
	var parsed rpcResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("decode RPC response: %w", err)
	}
	if parsed.Error != nil {
		return answeredError{fmt.Errorf("rpc %s: %s (%d)", method, parsed.Error.Message, parsed.Error.Code)}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(parsed.Result, out)
}
