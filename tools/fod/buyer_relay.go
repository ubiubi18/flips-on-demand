package main

// The buyer relay exposes only the node reads needed by a wallet-only buyer and
// forwarding of an already signed call to one configured contract. It has no
// signing key and never accepts a caller-supplied node API key.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
)

const (
	buyerRelayRequestLimit  = 256 << 10
	buyerRelayResultLimit   = 4 << 20
	buyerRelayIPFSResultMax = 2*maxRetainedFlipBytes + 1024
	buyerRelayTimeout       = 20 * time.Second
	buyerRelayRatePerMinute = 600
	buyerRelayMaxIPs        = 4096
)

type buyerRelayRequest struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      *int64          `json:"id"`
}

type buyerRelayBucket struct {
	window time.Time
	count  int
}

type buyerRelay struct {
	node      *client
	contract  common.Address
	http      *http.Client
	seller    bool
	archive   *sharedEvidenceHandler
	mu        sync.Mutex
	byIP      map[string]buyerRelayBucket
	cidMu     sync.Mutex
	cidAt     time.Time
	tradeCIDs map[string]bool
}

// newBuyerRelayHandler is a reusable /rpc handler. Its caller must terminate
// HTTPS and configure the node URL, API key, and contract independently of
// incoming buyer requests. The transport does not authenticate chain state.
func newBuyerRelayHandler(node *client, contract string) (http.Handler, error) {
	if node == nil || node.http == nil || node.http.client == nil || !directLocalRPC(node.url) || node.key == "" {
		return nil, errors.New("buyer relay needs a keyed loopback node RPC client")
	}
	if err := checkKeyedRPCURL(node.url, node.key); err != nil {
		return nil, err
	}
	address, err := directAddress(contract)
	if err != nil || address == strings.Repeat("0", addressBytes*2) {
		return nil, errors.New("buyer relay needs a nonzero pinned contract address")
	}
	contractBytes, _ := hex.DecodeString(address)
	var pinned common.Address
	copy(pinned[:], contractBytes)

	// Preserve the configured transport (including test transports) while never
	// following a credential-bearing redirect or waiting indefinitely.
	upstream := *node.http.client
	upstream.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	upstream.Timeout = buyerRelayTimeout
	return &buyerRelay{node: node, contract: pinned, http: &upstream, byIP: make(map[string]buyerRelayBucket)}, nil
}

func (h *buyerRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rpc" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.allowIP(r.RemoteAddr) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
		return
	}
	if r.ContentLength > buyerRelayRequestLimit {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	var input buyerRelayRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, buyerRelayRequestLimit))
	decoder.DisallowUnknownFields() // In particular, a client cannot supply `key`.
	if err := decoder.Decode(&input); err != nil {
		buyerRelayBadRequest(w, err)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		buyerRelayBadRequest(w, err)
		return
	}
	paramsRaw := bytes.TrimSpace(input.Params)
	if input.JSONRPC != "" && input.JSONRPC != "2.0" || input.Method == "" || input.ID == nil ||
		len(paramsRaw) == 0 || paramsRaw[0] != '[' {
		http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	var params []json.RawMessage
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		http.Error(w, "invalid JSON-RPC parameters", http.StatusBadRequest)
		return
	}
	if !marketRelayAllowed(input.Method, params, h.contract, h.seller) {
		http.Error(w, "RPC method or parameters refused", http.StatusForbidden)
		return
	}
	var archiveCID string
	if input.Method == "ipfs_get" && h.archive != nil {
		_ = json.Unmarshal(params[0], &archiveCID) // The allowlist already checked it.
		if blob, err := h.archive.blobByCID(archiveCID); err == nil {
			result, _ := json.Marshal("0x" + hex.EncodeToString(blob))
			buyerRelayAnswer(w, *input.ID, result, nil)
			return
		}
		if !h.activeTradeCID(archiveCID) {
			buyerRelayAnswer(w, *input.ID, nil, &buyerRelayUpstreamError{Code: -32000, Message: "encrypted flip is unavailable from this relay"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), buyerRelayTimeout)
	defer cancel()
	result, upstreamErr := h.call(ctx, input.Method, params)
	if archiveCID != "" && upstreamErr == nil {
		validNodeContent := false
		if buyerRelayIPFSResult(result) {
			var encoded string
			if json.Unmarshal(result, &encoded) == nil {
				if raw, err := hex.DecodeString(encoded[2:]); err == nil {
					if cidValid, _ := verifyBlobCID(archiveCID, raw, nil); cidValid {
						pub, _, _, parseErr := ParseFlipBlob(raw)
						if parseErr == nil {
							_, parseErr = crypto.UnmarshalPubkey(pub)
							validNodeContent = parseErr == nil
						}
					}
				}
			}
		}
		if !validNodeContent {
			upstreamErr = &buyerRelayUpstreamError{}
		}
	}
	if upstreamErr != nil {
		// The buyer's existing readers distinguish an absent storage key by this
		// one node error. All other upstream details stay server-side.
		message := "node RPC unavailable or refused the request"
		if (input.Method == "contract_readData" || input.Method == "contract_readMap") &&
			upstreamErr.Code == -32000 && upstreamErr.Message == "data is nil" {
			message = "data is nil"
		}
		buyerRelayAnswer(w, *input.ID, nil, &buyerRelayUpstreamError{Code: -32000, Message: message})
		return
	}
	if input.Method == "ipfs_get" && !buyerRelayIPFSResult(result) {
		buyerRelayAnswer(w, *input.ID, nil, &buyerRelayUpstreamError{Code: -32000, Message: "node content exceeds the buyer limit"})
		return
	}
	buyerRelayAnswer(w, *input.ID, result, nil)
}

func buyerRelayBadRequest(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
}

func (h *buyerRelay) allowIP(remote string) bool {
	ip, _, err := net.SplitHostPort(remote)
	if err != nil || net.ParseIP(ip) == nil {
		ip = "unknown"
	}
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	bucket, exists := h.byIP[ip]
	if !exists && len(h.byIP) >= buyerRelayMaxIPs {
		for key, value := range h.byIP {
			if now.Sub(value.window) >= time.Minute {
				delete(h.byIP, key)
			}
		}
		if len(h.byIP) >= buyerRelayMaxIPs {
			return false
		}
	}
	if !exists || now.Sub(bucket.window) >= time.Minute {
		bucket = buyerRelayBucket{window: now}
	}
	if bucket.count >= buyerRelayRatePerMinute {
		return false
	}
	bucket.count++
	h.byIP[ip] = bucket
	return true
}

// The shared relay may fetch a CID that is absent from its enrolled archive
// only when the pinned contract currently records it in an active trade.
// This preserves independent sellers' matched trades without turning the
// node's IPFS store into a public arbitrary-CID gateway.
func (h *buyerRelay) activeTradeCID(cid string) bool {
	h.cidMu.Lock()
	defer h.cidMu.Unlock()
	if h.tradeCIDs == nil || time.Since(h.cidAt) >= 10*time.Second {
		allowed, err := h.loadActiveTradeCIDs()
		if err != nil {
			allowed = make(map[string]bool) // Fail closed and briefly back off a failing node.
		}
		h.tradeCIDs, h.cidAt = allowed, time.Now()
	}
	return h.tradeCIDs[cid]
}

func (h *buyerRelay) loadActiveTradeCIDs() (map[string]bool, error) {
	node := *h.node
	node.bondedVersion = 9
	contract := strings.TrimPrefix(strings.ToLower(h.contract.Hex()), "0x")
	raw, err := readRegistryValue(&node, contract, "v9/active")
	if errors.Is(err, errNotInRegistry) {
		return map[string]bool{}, nil
	}
	if err != nil || len(raw) > 512 || len(raw)%8 != 0 {
		return nil, errors.New("active trade index unavailable")
	}
	allowed := make(map[string]bool)
	seenTrades := make(map[uint64]bool)
	seenBundles := make(map[string]bool)
	for off := 0; off < len(raw); off += 8 {
		id := binary.BigEndian.Uint64(raw[off : off+8])
		if id == 0 || seenTrades[id] {
			return nil, errors.New("invalid active trade index")
		}
		seenTrades[id] = true
		trade, err := readBondedV8Trade(&node, contract, strconv.FormatUint(id, 10))
		if err != nil {
			return nil, err
		}
		if seenBundles[trade.Bundle] {
			continue
		}
		seenBundles[trade.Bundle] = true
		bundle, err := readBondedV8Bundle(&node, contract, trade.Bundle)
		if err != nil {
			return nil, err
		}
		for _, bundledCID := range bundle.CIDs {
			allowed[bundledCID] = true
		}
	}
	return allowed, nil
}

type buyerRelayUpstreamError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (h *buyerRelay) call(ctx context.Context, method string, params []json.RawMessage) (json.RawMessage, *buyerRelayUpstreamError) {
	body, err := json.Marshal(rpcRequest{Method: method, Params: params, ID: 1, Key: h.node.key})
	if err != nil {
		return nil, &buyerRelayUpstreamError{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.node.url, bytes.NewReader(body))
	if err != nil {
		return nil, &buyerRelayUpstreamError{}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.http.Do(req)
	if err != nil {
		return nil, &buyerRelayUpstreamError{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &buyerRelayUpstreamError{}
	}
	limit := int64(buyerRelayResultLimit)
	if method == "ipfs_get" {
		limit = buyerRelayIPFSResultMax
	}
	raw, err := readBoundedResponse(resp, limit)
	if err != nil {
		return nil, &buyerRelayUpstreamError{}
	}
	var answer struct {
		Result json.RawMessage          `json:"result"`
		Error  *buyerRelayUpstreamError `json:"error"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil || answer.Result == nil && answer.Error == nil {
		return nil, &buyerRelayUpstreamError{}
	}
	if answer.Error != nil {
		return nil, answer.Error
	}
	return answer.Result, nil
}

func buyerRelayAnswer(w http.ResponseWriter, id int64, result json.RawMessage, rpcErr *buyerRelayUpstreamError) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		ID     int64                    `json:"id"`
		Result json.RawMessage          `json:"result,omitempty"`
		Error  *buyerRelayUpstreamError `json:"error,omitempty"`
	}{ID: id, Result: result, Error: rpcErr})
}

func buyerRelayIPFSResult(result json.RawMessage) bool {
	var value string
	if err := json.Unmarshal(result, &value); err != nil || !strings.HasPrefix(value, "0x") {
		return false
	}
	if len(value) < 4 || len(value) > 2+maxRetainedFlipBytes*2 {
		return false
	}
	_, err := hex.DecodeString(value[2:])
	return err == nil
}

func buyerRelayAllowed(method string, params []json.RawMessage, contract common.Address) bool {
	return marketRelayAllowed(method, params, contract, false)
}

func sellerRelayOperation(op string) bool {
	switch op {
	case "queueSell", "cancelSell", "acquire", "deliver", "proveDelivery":
		return true
	}
	return false
}

func marketRelayOperation(op string, seller bool) bool {
	return buyerBondedOperation(op) || seller && sellerRelayOperation(op)
}

func marketRelayAllowed(method string, params []json.RawMessage, contract common.Address, seller bool) bool {
	pin := strings.ToLower(contract.Hex())
	switch method {
	case "bcn_syncing", "bcn_lastBlock", "bcn_feePerGas", "dna_epoch":
		return len(params) == 0
	case "bcn_blockAt":
		var height uint64
		return len(params) == 1 && json.Unmarshal(params[0], &height) == nil
	case "bcn_transaction", "bcn_txReceipt":
		return len(params) == 1 && buyerRelayHex(params[0], 32)
	case "dna_getBalance":
		return len(params) == 1 && buyerRelayHex(params[0], addressBytes)
	case "dna_identity":
		return seller && len(params) == 1 && buyerRelayHex(params[0], addressBytes)
	case "contract_getStake":
		return len(params) == 1 && buyerRelayIsPin(params[0], pin)
	case "contract_readData":
		return len(params) == 3 && buyerRelayIsPin(params[0], pin) &&
			buyerRelayText(params[1], 256) && buyerRelayStringIs(params[2], "hex")
	case "contract_readMap":
		return len(params) == 4 && buyerRelayIsPin(params[0], pin) &&
			buyerRelayText(params[1], 128) && buyerRelayHexBounded(params[2], 256) && buyerRelayStringIs(params[3], "hex")
	case "contract_iterateMap":
		if len(params) != 6 || !buyerRelayIsPin(params[0], pin) || !buyerRelayText(params[1], 128) ||
			!buyerRelayStringIs(params[3], "hex") || !buyerRelayStringIs(params[4], "hex") {
			return false
		}
		if string(params[2]) != "null" && !buyerRelayHexBounded(params[2], 256) {
			return false
		}
		var count int
		return json.Unmarshal(params[5], &count) == nil && count > 0 && count <= marketPageLimit
	case "contract_estimateCall":
		return len(params) == 1 && marketRelayEstimate(params[0], pin, seller)
	case "ipfs_get":
		if len(params) != 1 {
			return false
		}
		var name string
		if json.Unmarshal(params[0], &name) != nil || len(name) != cidV1MultibaseLength || !isFlipCID(name) {
			return false
		}
		decoded, err := cidBytesFromString(name)
		return err == nil && len(decoded) == 36 && decoded[0] == 1 &&
			(decoded[1] == 0x55 || decoded[1] == 0x70) && decoded[2] == 0x12 && decoded[3] == 0x20
	case "bcn_sendRawTx":
		return len(params) == 1 && marketRelaySignedCall(params[0], contract, seller)
	default:
		return false
	}
}

func buyerRelayStringIs(raw json.RawMessage, expected string) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && value == expected
}

func buyerRelayText(raw json.RawMessage, max int) bool {
	var value string
	if json.Unmarshal(raw, &value) != nil || len(value) == 0 || len(value) > max {
		return false
	}
	for _, b := range []byte(value) {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

func buyerRelayHex(raw json.RawMessage, size int) bool {
	var value string
	if json.Unmarshal(raw, &value) != nil || len(value) != 2+size*2 || !strings.HasPrefix(value, "0x") {
		return false
	}
	_, err := hex.DecodeString(value[2:])
	return err == nil
}

func buyerRelayHexBounded(raw json.RawMessage, maxBytes int) bool {
	var value string
	if json.Unmarshal(raw, &value) != nil || !strings.HasPrefix(value, "0x") || len(value) > 2+maxBytes*2 {
		return false
	}
	_, err := hex.DecodeString(value[2:])
	return err == nil
}

func buyerRelayIsPin(raw json.RawMessage, pin string) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && strings.EqualFold(value, pin)
}

func buyerRelayDecimal(value string) bool {
	if len(value) == 0 || len(value) > 80 {
		return false
	}
	dots := 0
	for _, b := range []byte(value) {
		if b == '.' {
			dots++
		} else if b < '0' || b > '9' {
			return false
		}
	}
	return dots <= 1 && value[0] != '.' && value[len(value)-1] != '.'
}

func buyerRelayEstimate(raw json.RawMessage, pin string) bool {
	return marketRelayEstimate(raw, pin, false)
}

func marketRelayEstimate(raw json.RawMessage, pin string, seller bool) bool {
	var call struct {
		From     string `json:"from"`
		Contract string `json:"contract"`
		Method   string `json:"method"`
		Amount   string `json:"amount"`
		MaxFee   string `json:"maxFee"`
		Args     []struct {
			Index  int    `json:"index"`
			Format string `json:"format"`
			Value  string `json:"value"`
		} `json:"args"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&call) != nil || !strings.EqualFold(call.Contract, pin) || !marketRelayOperation(call.Method, seller) ||
		!buyerRelayDecimal(call.Amount) || !buyerRelayDecimal(call.MaxFee) || len(call.Args) != 1 ||
		call.Args[0].Index != 0 || call.Args[0].Format != "hex" {
		return false
	}
	for _, b := range []byte(call.Method) {
		if !((b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')) {
			return false
		}
	}
	if !buyerRelayHex(json.RawMessage(strconv.Quote(call.From)), addressBytes) ||
		!buyerRelayHexBounded(json.RawMessage(strconv.Quote(call.Args[0].Value)), 128<<10) {
		return false
	}
	return true
}

func buyerRelaySignedCall(raw json.RawMessage, contract common.Address) bool {
	return marketRelaySignedCall(raw, contract, false)
}

func marketRelaySignedCall(raw json.RawMessage, contract common.Address, seller bool) bool {
	var value string
	if json.Unmarshal(raw, &value) != nil || !strings.HasPrefix(value, "0x") ||
		len(value) < 4 || len(value) > 2+(128<<10)*2 {
		return false
	}
	txBytes, err := hex.DecodeString(value[2:])
	if err != nil {
		return false
	}
	var tx types.Transaction
	if tx.FromBytes(txBytes) != nil || tx.Type != types.CallContractTx || tx.To == nil || *tx.To != contract ||
		len(tx.Signature) != signatureBytes {
		return false
	}
	if _, err := types.Sender(&tx); err != nil {
		return false
	}
	call := attachments.ParseCallContractAttachment(&tx)
	return call != nil && marketRelayOperation(call.Method, seller) && len(call.Args) <= 1
}
