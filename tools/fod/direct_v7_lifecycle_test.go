package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectV7LifecycleCallsRequireCurrentStateAndExactValue(t *testing.T) {
	contract, codeHash := strings.Repeat("cc", 20), strings.Repeat("cd", 32)
	var author, buyer [20]byte
	author[0], buyer[0] = 0x11, 0x22
	id := poolV7OfferID(author, 230)
	encodeU64 := func(n uint64) []byte { return binary.BigEndian.AppendUint64(nil, n) }
	encodeU128 := func(n *big.Int) []byte { return n.FillBytes(make([]byte, 16)) }
	seedAmount, _ := new(big.Int).SetString(poolV7SeedAmount, 10)
	feeBond := big.NewInt(5_000_000)
	var offer = make([]byte, 222)
	copy(offer[:20], author[:])
	binary.BigEndian.PutUint16(offer[20:22], 230)
	binary.BigEndian.PutUint32(offer[22:26], 1)
	offer[26], offer[58] = 2, 3
	copy(offer[90:106], encodeU128(feeBond))
	copy(offer[106:114], encodeU64(2))
	copy(offer[114:122], encodeU64(1))
	offer[122], offer[204] = 3, 1
	var order = make([]byte, 215)
	copy(order[:32], id[:])
	copy(order[32:52], buyer[:])
	copy(order[117:133], encodeU128(big.NewInt(1000)))
	copy(order[133:149], encodeU128(feeBond))
	copy(order[149:165], encodeU128(big.NewInt(1000)))
	copy(order[165:173], encodeU64(100))
	copy(order[173:181], encodeU64(1))
	order[181] = 1
	head := uint64(100 + poolV7ProofWindow - 1)
	active := hex.EncodeToString(buyer[:])
	values := map[string][]byte{
		"pool/seed-id":         id[:],
		"pool/primary-head":    id[:],
		"pool/secondary-head":  make([]byte, 32),
		"pool/primary-units":   encodeU64(0),
		"pool/secondary-units": encodeU64(1),
		"pool/reserve":         encodeU128(seedAmount),
		"pool/insurance":       make([]byte, 16),
		"pool/head-since":      encodeU64(100),
		"pool/activated-at":    encodeU64(100),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			http.Error(w, "bad RPC", http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "dna_getCoinbaseAddr":
			result = "0x" + active
		case "contract_getStake":
			result = map[string]any{"Hash": "0x" + codeHash}
		case "contract_iterateMap":
			result = map[string]any{"items": []any{map[string]any{"key": hex.EncodeToString(id[:]), "value": hex.EncodeToString(offer)}}, "continuationToken": nil}
		case "contract_readData":
			key := request.Params[1].(string)
			value := values[key]
			if key == "pool/order/1" {
				value = order
			}
			if value == nil {
				http.Error(w, "unknown state", http.StatusBadRequest)
				return
			}
			result = "0x" + hex.EncodeToString(value)
		case "bcn_lastBlock":
			result = map[string]any{"height": head}
		case "bcn_feePerGas":
			result = "1"
		case "contract_estimateCall":
			result = map[string]any{"success": true, "gasUsed": 1000}
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient()}
	base := func(format, operation, amount, calldata string) *preparedAction {
		return &preparedAction{
			Format: format, Version: 1, Operation: operation, Network: NetworkNamePinned,
			DirectVersion: 7, Contract: contract, CodeHash: codeHash, Amount: amount,
			Calldata: calldata, DirectOfferID: hex.EncodeToString(id[:]), DirectOrderID: 1,
			Author: hex.EncodeToString(author[:]), Epoch: 230, CoveredFlips: 1,
			InventoryCommitment: hex.EncodeToString(offer[26:58]), KeyCommitment: hex.EncodeToString(offer[58:90]),
		}
	}
	preparedDir := t.TempDir()
	checkPreparation := func(operation, actor, amount, minimum string, orderID uint64) {
		t.Helper()
		active = actor
		output := filepath.Join(preparedDir, operation+".json")
		if err := preparePoolV7FromFiles(directCommandOptions{
			DirectVersion: 7, Network: NetworkNamePinned, Node: c, Operation: operation,
			CodeHash: codeHash, OfferID: hex.EncodeToString(id[:]), OrderID: orderID,
			Amount: amount, MinimumPayout: minimum, MaxFeeText: "1000000", PreparedPath: output,
		}, contract); err != nil {
			t.Fatalf("typed %s preparation failed: %v", operation, err)
		}
		raw, err := os.ReadFile(output)
		var prepared preparedAction
		if err != nil || decodeStrict(raw, &prepared) != nil || prepared.Operation != operation {
			t.Fatalf("typed %s action was not saved correctly: %v", operation, err)
		}
	}
	orderArg := hex.EncodeToString(encodeU64(1))
	idArg := hex.EncodeToString(id[:])
	refund := base(preparedPoolV7Refund, "refund", "0", orderArg)
	if err := validatePreparedPoolV7Caller(c, refund, hex.EncodeToString(buyer[:])); err == nil {
		t.Fatal("early refund was accepted")
	}
	head++
	if err := validatePreparedPoolV7Caller(c, refund, hex.EncodeToString(buyer[:])); err != nil {
		t.Fatalf("refund at exact proof deadline was refused: %v", err)
	}
	checkPreparation("refund", hex.EncodeToString(buyer[:]), "", "", 1)
	order[181], offer[122], offer[204], offer[220] = 2, 4, 0, 1
	slash := base(preparedPoolV7Slash, "slashUnproved", "0", orderArg)
	if err := validatePreparedPoolV7Caller(c, slash, hex.EncodeToString(buyer[:])); err != nil {
		t.Fatalf("public slash after unproved settlement was refused: %v", err)
	}
	checkPreparation("slashUnproved", hex.EncodeToString(buyer[:]), "", "", 1)
	order[181], offer[122], offer[204], offer[220] = 1, 1, 1, 0
	copy(offer[114:122], encodeU64(0))
	cancel := base(preparedPoolV7Cancel, "cancel", "0", idArg)
	if err := validatePreparedPoolV7Caller(c, cancel, hex.EncodeToString(author[:])); err != nil {
		t.Fatalf("seller cancellation was refused: %v", err)
	}
	checkPreparation("cancel", hex.EncodeToString(author[:]), "", "", 0)
	cancel.Amount = "1"
	if err := validatePreparedPoolV7Caller(c, cancel, hex.EncodeToString(author[:])); err == nil {
		t.Fatal("nonpayable cancellation accepted attached value")
	}
	topUp := base(preparedPoolV7TopUp, "topUpBond", "1", idArg)
	if err := validatePreparedPoolV7Caller(c, topUp, hex.EncodeToString(author[:])); err != nil {
		t.Fatalf("seller bond top-up was refused: %v", err)
	}
	checkPreparation("topUpBond", hex.EncodeToString(author[:]), "1", "", 0)
	topUp.Amount = "0"
	if err := validatePreparedPoolV7Caller(c, topUp, hex.EncodeToString(author[:])); err == nil {
		t.Fatal("zero bond top-up was accepted")
	}
	offer[122] = 5
	copy(offer[90:106], make([]byte, 16))
	relistArg, _ := encodePoolV7Relist(id, big.NewInt(0))
	relist := base(preparedPoolV7Relist, "relist", feeBond.String(), hex.EncodeToString(relistArg))
	relist.MinimumPayout = "0"
	if err := validatePreparedPoolV7Caller(c, relist, hex.EncodeToString(author[:])); err != nil {
		t.Fatalf("cancelled first sale could not be relisted: %v", err)
	}
	checkPreparation("relist", hex.EncodeToString(author[:]), "", "0", 0)
	offer[122] = 1
	copy(offer[90:106], encodeU128(feeBond))
	expire := base(preparedPoolV7Expire, "expireHead", "0", idArg)
	head = 100 + poolV7HeadTimeout - 1
	if err := validatePreparedPoolV7Caller(c, expire, hex.EncodeToString(buyer[:])); err == nil {
		t.Fatal("early private-head expiry was accepted")
	}
	head++
	if err := validatePreparedPoolV7Caller(c, expire, hex.EncodeToString(buyer[:])); err != nil {
		t.Fatalf("private-head expiry at boundary was refused: %v", err)
	}
	checkPreparation("expireHead", hex.EncodeToString(buyer[:]), "", "", 0)
	offer[122], offer[221] = 2, 1
	copy(offer[90:106], make([]byte, 16))
	values["pool/primary-head"] = make([]byte, 32)
	values["pool/secondary-head"] = id[:]
	publicBuy := base(preparedPoolV7PublicBuy, "buyPublicSeed", poolV7PriceCap, idArg)
	publicBuy.Price = poolV7PriceCap
	if err := validatePreparedPoolV7Caller(c, publicBuy, hex.EncodeToString(buyer[:])); err != nil {
		t.Fatalf("exact public seed purchase was refused: %v", err)
	}
	checkPreparation("buyPublicSeed", hex.EncodeToString(buyer[:]), "", "", 0)
	publicBuy.Amount = new(big.Int).Add(seedAmount, big.NewInt(1)).String()
	if err := validatePreparedPoolV7Caller(c, publicBuy, hex.EncodeToString(buyer[:])); err == nil {
		t.Fatal("incorrect public-seed payment was accepted")
	}
}
