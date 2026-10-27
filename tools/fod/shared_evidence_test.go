package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/core/state"
	"github.com/idena-network/idena-go/crypto"
)

func sharedEvidenceFixture(t *testing.T) (string, string, string, []byte) {
	t.Helper()
	blob := []byte("original encrypted flip fixture")
	cid := cidRawSha256(blob)
	cidBytes, err := cidBytesFromString(cid)
	if err != nil {
		t.Fatal(err)
	}
	addr := sessionAddress(0x73)
	author := hex.EncodeToString(addr[:])
	dataDir := writeSessionDatadir(t, addr, [][]byte{cidBytes}, state.AtLeastOneFlipReported, true, fixtureEpoch+1)
	bundle, err := readSessionBundle(dataDir, sessionInventoryHeight, sessionVerdictHeight, author)
	if err != nil {
		t.Fatal(err)
	}
	bundleBytes, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	authorDir := filepath.Join(root, "public-authors", "epoch-228", "authors", "pilot")
	if err := os.MkdirAll(filepath.Join(authorDir, "encrypted-blobs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authorDir, "session-bundle.json"), bundleBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authorDir, "encrypted-blobs", cid), blob, 0o600); err != nil {
		t.Fatal(err)
	}
	marker := sharedEvidenceReady{
		Format: sharedEvidenceFormat, Epoch: fixtureEpoch, Author: author,
		BundleSHA256: sharedSHA256(bundleBytes), Manifest: bundle.Manifest,
		Blobs:                 map[string]sharedEvidenceBlob{cid: {Bytes: len(blob), SHA256: sharedSHA256(blob)}},
		HeadersCheckedAgainst: []string{"node-a", "node-b"},
	}
	markerBytes, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authorDir, "ready.json"), markerBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, authorDir, cid, blob
}

func TestSharedEvidenceServesOnlyCompleteVerifiedArchive(t *testing.T) {
	root, dir, cid, blob := sharedEvidenceFixture(t)
	handler, err := newSharedEvidenceHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		return response
	}
	prefix := "/evidence/v1/epochs/228/authors/pilot/"
	if got := request(http.MethodGet, prefix+"session"); got.Code != http.StatusOK ||
		!bytes.Contains(got.Body.Bytes(), []byte(`"format":"fod.session-bundle"`)) {
		t.Fatalf("verified session response: code=%d body=%q", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, prefix+"blobs/"+cid); got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), blob) {
		t.Fatalf("verified blob response: code=%d body=%q", got.Code, got.Body.String())
	}
	markerRaw, err := os.ReadFile(filepath.Join(dir, "ready.json"))
	if err != nil {
		t.Fatal(err)
	}
	var marker sharedEvidenceReady
	if err := json.Unmarshal(markerRaw, &marker); err != nil {
		t.Fatal(err)
	}
	byAddress := "/evidence/v1/epochs/228/authors/" + marker.Author + "/"
	discovery := "/evidence/v1/authors/" + marker.Author + "/epochs"
	if got := request(http.MethodGet, discovery); got.Code != http.StatusOK ||
		strings.TrimSpace(got.Body.String()) != `{"epochs":[228]}` {
		t.Fatalf("author discovery returned %d: %q", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, byAddress+"session"); got.Code != http.StatusOK {
		t.Fatalf("author-address lookup returned %d", got.Code)
	}
	if got := request(http.MethodGet, byAddress+"blobs/"+cid); got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), blob) {
		t.Fatalf("author-address blob returned %d", got.Code)
	}
	marker.HeadersCheckedAgainst = []string{"node-a"}
	uncorroborated, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ready.json"), uncorroborated, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, prefix+"session"); got.Code != http.StatusNotFound {
		t.Fatalf("single-node marker returned %d", got.Code)
	}
	if got := request(http.MethodGet, discovery); got.Code != http.StatusOK ||
		strings.TrimSpace(got.Body.String()) != `{"epochs":[]}` {
		t.Fatalf("single-node record was advertised: %d %q", got.Code, got.Body.String())
	}
	if err := os.WriteFile(filepath.Join(dir, "ready.json"), markerRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/evidence/v1/epochs/0228/authors/pilot/session",
		"/evidence/v1/epochs/228/authors/../session",
		"/evidence/v1/epochs/228/authors/pilot/blobs/not-a-cid",
		"/evidence/v1/epochs/228/authors/other/session",
		"/evidence/v1/epochs/228/authors/pilot/ready.json",
		"/evidence/v1/authors/" + strings.ToUpper(marker.Author) + "/epochs",
		"/evidence/v1/authors/pilot/epochs",
	} {
		if got := request(http.MethodGet, path); got.Code != http.StatusNotFound {
			t.Fatalf("%s returned %d", path, got.Code)
		}
	}
	if got := request(http.MethodPost, prefix+"session"); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST returned %d", got.Code)
	}
	if err := os.WriteFile(filepath.Join(dir, "encrypted-blobs", cid), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, prefix+"session"); got.Code != http.StatusNotFound {
		t.Fatalf("incomplete archive returned %d", got.Code)
	}
	if got := request(http.MethodGet, discovery); got.Code != http.StatusOK ||
		strings.TrimSpace(got.Body.String()) != `{"epochs":[]}` {
		t.Fatalf("missing ciphertext was advertised: %d %q", got.Code, got.Body.String())
	}
}

func TestSharedEvidenceMissingAuthorSkipsBundleVerification(t *testing.T) {
	root, dir, _, _ := sharedEvidenceFixture(t)
	handler, err := newSharedEvidenceHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	markerRaw, err := os.ReadFile(filepath.Join(dir, "ready.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ready sharedEvidenceReady
	if err := json.Unmarshal(markerRaw, &ready); err != nil {
		t.Fatal(err)
	}
	verified := 0
	verify := func(label string) (*sharedEvidenceReady, error) {
		verified++
		marker, _, _, _, err := sharedEvidenceRecord(root, fixtureEpoch, label)
		return marker, err
	}
	missing := strings.Repeat("ab", addressBytes)
	if label, err := handler.labelForAuthorWithRecord(fixtureEpoch, missing, verify); err == nil || label != "" || verified != 0 {
		t.Fatalf("missing author triggered %d full bundle verifications: label=%q err=%v", verified, label, err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/evidence/v1/authors/"+missing+"/epochs", nil))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"epochs":[]}` {
		t.Fatalf("missing author discovery returned %d: %q", response.Code, response.Body.String())
	}
	if label, err := handler.labelForAuthorWithRecord(fixtureEpoch, ready.Author, verify); err != nil || label != "pilot" || verified != 1 {
		t.Fatalf("matching author requires one full verification: label=%q err=%v count=%d", label, err, verified)
	}
}

func TestSharedEvidenceRefusesUnsafeArchive(t *testing.T) {
	root, dir, cid, _ := sharedEvidenceFixture(t)
	if err := os.Remove(filepath.Join(dir, "encrypted-blobs", cid)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "session-bundle.json"), filepath.Join(dir, "encrypted-blobs", cid)); err != nil {
		t.Fatal(err)
	}
	h, err := newSharedEvidenceHandler(root)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/evidence/v1/epochs/228/authors/pilot/session", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("symlink archive returned %d", response.Code)
	}
	if _, err := newSharedEvidenceHandler(strings.TrimPrefix(root, "/")); err == nil {
		t.Fatal("relative archive accepted")
	}
}

func TestSharedMarketRelayPinsContractAndRefusesNodeSigning(t *testing.T) {
	root, _, cid, blob := sharedEvidenceFixture(t)
	otherBytes := []byte("not an Idena encrypted flip")
	otherCID := cidRawSha256(otherBytes)
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	flipBytes, err := BuildFlipBlob(crypto.FromECDSAPub(&key.PublicKey), []byte{1, 2, 3}, []byte{4, 5, 6})
	if err != nil {
		t.Fatal(err)
	}
	flipCID := cidRawSha256(flipBytes)
	const epoch = uint16(234)
	author := strings.Repeat("73", addressBytes)
	bundleID := bondedBundleID(9, author, epoch)
	decode := func(value string) []byte {
		t.Helper()
		raw, err := hex.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	bundleRecord := make([]byte, 97)
	copy(bundleRecord[:20], decode(author))
	binary.BigEndian.PutUint16(bundleRecord[20:22], epoch)
	binary.BigEndian.PutUint16(bundleRecord[22:24], 1)
	copy(bundleRecord[24:56], decode(inventoryCommitment(author, epoch, []string{flipCID})))
	cidsRecord := append([]byte{1, 59}, []byte(flipCID)...)
	tradeRecord := make([]byte, 308)
	copy(tradeRecord[16:48], decode(bundleID))
	tradeRecord[306] = 1
	contract := "0x" + strings.Repeat("ab", addressBytes)
	other := "0x" + strings.Repeat("cd", addressBytes)
	codeHash := "0x" + strings.Repeat("ef", 32)
	forwarded := 0
	ipfsForwarded := 0
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		var req struct {
			Method string   `json:"method"`
			Key    string   `json:"key"`
			Params []string `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key != "server-only-test-key" {
			t.Errorf("bad upstream request: %v", err)
		}
		if req.Method == "contract_getStake" {
			_, _ = io.WriteString(w, fmt.Sprintf(`{"result":{"Hash":%q}}`, codeHash))
			return
		}
		if req.Method == "contract_readData" && len(req.Params) == 3 && req.Params[1] == "v9/active" {
			_, _ = io.WriteString(w, `{"result":"0x0000000000000001"}`)
			return
		}
		if req.Method == "contract_readMap" && len(req.Params) == 4 {
			var raw []byte
			switch req.Params[1] {
			case "v9/trade/":
				raw = tradeRecord
			case "v9/bundle/":
				raw = bundleRecord
			case "v9/cids/":
				raw = cidsRecord
			}
			_, _ = io.WriteString(w, fmt.Sprintf(`{"result":%q}`, "0x"+hex.EncodeToString(raw)))
			return
		}
		if req.Method == "ipfs_get" && len(req.Params) == 1 {
			ipfsForwarded++
			if req.Params[0] == otherCID {
				_, _ = io.WriteString(w, fmt.Sprintf(`{"result":%q}`, "0x"+hex.EncodeToString(otherBytes)))
				return
			}
			if req.Params[0] == flipCID {
				_, _ = io.WriteString(w, fmt.Sprintf(`{"result":%q}`, "0x"+hex.EncodeToString(flipBytes)))
				return
			}
		}
		_, _ = io.WriteString(w, `{"result":"0x1234"}`)
	}))
	defer node.Close()
	h, err := newSharedMarketHandler(&client{url: node.URL, key: "server-only-test-key", http: newHTTPClient()}, contract, codeHash, root)
	if err != nil {
		t.Fatal(err)
	}
	post := func(body string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, req)
		return response.Code
	}
	if got := post(fmt.Sprintf(`{"method":"bcn_sendRawTx","params":[%q],"id":1}`,
		buyerRelaySignedTx(t, contract, types.CallContractTx, true, "queueSell"))); got != http.StatusOK {
		t.Fatalf("locally signed seller call returned %d", got)
	}
	beforeArchive := forwarded
	request := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(
		fmt.Sprintf(`{"method":"ipfs_get","params":[%q],"id":2}`, cid)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	var fetched struct {
		Result string `json:"result"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &fetched) != nil ||
		fetched.Result != "0x"+hex.EncodeToString(blob) || forwarded != beforeArchive {
		t.Fatalf("verified archive did not serve the original CID without a node fetch: %d", response.Code)
	}
	for _, sample := range []struct {
		cid        string
		wantResult bool
	}{{otherCID, false}, {flipCID, true}} {
		request := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(
			fmt.Sprintf(`{"method":"ipfs_get","params":[%q],"id":2}`, sample.cid)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		var answer struct {
			Result string                   `json:"result"`
			Error  *buyerRelayUpstreamError `json:"error"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &answer) != nil ||
			(sample.wantResult && (answer.Result == "" || answer.Error != nil)) ||
			(!sample.wantResult && (answer.Result != "" || answer.Error == nil)) {
			t.Fatalf("shared relay returned wrong CID content boundary: %d", response.Code)
		}
	}
	if ipfsForwarded != 1 {
		t.Fatalf("shared relay fetched %d node CIDs; only the active-trade flip was authorized", ipfsForwarded)
	}
	before := forwarded
	for _, body := range []string{
		`{"method":"dna_sendTransaction","params":[],"id":1}`,
		fmt.Sprintf(`{"method":"bcn_sendRawTx","params":[%q],"id":1}`,
			buyerRelaySignedTx(t, other, types.CallContractTx, true, "queueSell")),
		fmt.Sprintf(`{"method":"bcn_sendRawTx","params":[%q],"id":1}`,
			buyerRelaySignedTx(t, contract, types.CallContractTx, true, "registerBundle")),
	} {
		if got := post(body); got != http.StatusForbidden {
			t.Fatalf("unsafe call returned %d", got)
		}
	}
	if forwarded != before {
		t.Fatalf("unsafe requests reached node: %d", forwarded-before)
	}
	if _, err := newSharedMarketHandler(&client{url: node.URL, key: "server-only-test-key", http: newHTTPClient()},
		contract, "0x"+strings.Repeat("01", 32), root); err == nil {
		t.Fatal("wrong deployed code hash accepted")
	}
}
