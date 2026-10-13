package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

type rpcTransportFunc func(*http.Request) (*http.Response, error)

func (f rpcTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestKeyedRPCRequiresHTTPSOutsideLoopback(t *testing.T) {
	for _, tc := range []struct {
		name, endpoint, key string
		allowed             bool
	}{
		{"IPv4 loopback", "http://127.0.0.2:9019", "fixture-key", true},
		{"IPv6 loopback", "http://[::1]:9019", "fixture-key", true},
		{"localhost", "http://localhost:9019", "fixture-key", true},
		{"remote HTTPS", "https://node.example:9019", "fixture-key", true},
		{"remote HTTP without key", "http://node.example:9019", "", true},
		{"remote HTTP with key", "http://node.example:9019", "fixture-key", false},
		{"uppercase remote HTTP with key", "HTTP://node.example:9019", "fixture-key", false},
		{"deceptive loopback name", "http://127.0.0.1.evil.invalid:9019", "fixture-key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := &httpClient{client: &http.Client{Transport: rpcTransportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"result":"ok"}`))}, nil
			})}}
			var result string
			err := c.call(tc.endpoint, tc.key, "dna_identity", nil, &result)
			if tc.allowed {
				if err != nil || result != "ok" || calls != 1 {
					t.Fatalf("allowed RPC: result %q, calls %d, err %v", result, calls, err)
				}
			} else if err == nil || err.Error() != "node RPC API key requires HTTPS outside loopback" || calls != 0 {
				t.Fatalf("insecure RPC: calls %d, err %v", calls, err)
			}
		})
	}
}

type endlessResponse struct{ read int64 }

func (r *endlessResponse) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.read += int64(len(p))
	return len(p), nil
}
func (r *endlessResponse) Close() error { return nil }

func TestNetworkLimitAlsoBoundsLengthlessBodies(t *testing.T) {
	stream := &endlessResponse{}
	_, err := readBoundedResponse(&http.Response{Body: stream, ContentLength: -1}, 1024)
	if err == nil || stream.read != 1025 {
		t.Fatalf("unbounded read: %d, %v", stream.read, err)
	}
	if retryableRPC("ipfs_get", err) {
		t.Fatal("oversized responses must not be retried")
	}
	normal := io.NopCloser(strings.NewReader(strings.Repeat("x", 1024)))
	bytes, err := readBoundedResponse(&http.Response{Body: normal, ContentLength: -1}, 1024)
	if err != nil || len(bytes) != 1024 {
		t.Fatal("refused the limit boundary", err)
	}
}

func TestRPCIndexerAndIPFSRejectOversizedResponses(t *testing.T) {
	oversized := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if oversized {
			w.Header().Set("Content-Length", strconv.FormatInt(maxNetworkResponseBytes+1, 10))
			return
		}
		if r.URL.Path == "/ipfs/test" {
			io.WriteString(w, "normal encrypted bytes")
			return
		}
		io.WriteString(w, `{"result":"normal"}`)
	}))
	defer server.Close()
	c := newHTTPClient()
	for name, fetch := range map[string]func() error{
		"rpc":     func() error { return c.call(server.URL, "", "ipfs_get", nil, nil) },
		"indexer": func() error { return c.get(server.URL, nil) },
		"ipfs":    func() error { _, err := gatewayFetch(server.URL + "/ipfs")("test"); return err },
	} {
		if err := fetch(); err == nil || !strings.Contains(err.Error(), "client limit") {
			t.Fatal(name, err)
		}
	}
	oversized = false
	var value string
	if err := c.call(server.URL, "", "ipfs_get", nil, &value); err != nil || value != "normal" {
		t.Fatal("normal RPC", err)
	}
	if err := c.get(server.URL, nil); err != nil {
		t.Fatal("normal indexer", err)
	}
	if data, err := gatewayFetch(server.URL + "/ipfs")("test"); err != nil || string(data) != "normal encrypted bytes" {
		t.Fatal("normal IPFS", err)
	}
}

func TestRPCDoesNotForwardCredentialsAcrossRedirects(t *testing.T) {
	forwarded := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = true
		io.WriteString(w, `{"result":"redirected"}`)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	var result string
	if err := newHTTPClient().call(origin.URL, "disposable-test-credential", "dna_identity", nil, &result); err == nil {
		t.Error("accepted a redirected RPC response")
	}
	if forwarded {
		t.Error("sent the credential-bearing request to another endpoint")
	}
}

func TestRPCRefusesHTTPFailureEvenWithAResult(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"result":"not authorized"}`)
	}))
	defer server.Close()
	var result string
	if err := newHTTPClient().call(server.URL, "", "dna_identity", nil, &result); err == nil {
		t.Fatal("accepted an HTTP failure as an RPC success")
	}
	if calls != 1 {
		t.Fatalf("retried an explicit HTTP refusal %d times", calls)
	}
}
