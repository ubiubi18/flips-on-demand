package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func TestFastDisclosureRequiresTheFundedOrderAndAuthorKey(t *testing.T) {
	bundle := strings.Repeat("55", 32)
	author := strings.Repeat("22", 20)
	buyer := strings.Repeat("33", 20)
	deliveryKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	order := poolOrderRecordFor(t, mustDecode(bundle), mustDecode(buyer), crypto.FromECDSAPub(&deliveryKey.PublicKey), 7, 1)
	pool := poolRecordBytes(mustDecode(author), bytes.Repeat([]byte{4}, 32), bytes.Repeat([]byte{5}, 32), 2, 1000, 0, 0, 1)
	fast := make([]byte, fastBundleRecordBytes)
	binary.BigEndian.PutUint16(fast[2:4], 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		switch req.Method {
		case "contract_readData":
			var key string
			_ = json.Unmarshal(req.Params[1], &key)
			if key == fmt.Sprintf("%s%d", marketPoolOrderPrefix, 7) {
				fmt.Fprintf(w, `{"result":"0x%x"}`, order)
				return
			}
			if value, ok := poolStubCounters(key); ok {
				fmt.Fprintf(w, `{"result":%q}`, value)
				return
			}
		case "contract_iterateMap":
			var prefix string
			_ = json.Unmarshal(req.Params[1], &prefix)
			value := pool
			if prefix == "pfast/" {
				value = fast
			} else if prefix != marketPoolPrefix {
				break
			}
			fmt.Fprintf(w, `{"result":{"items":[{"key":%q,"value":%q}],"continuationToken":"0x"}}`, bundle, hex.EncodeToString(value))
			return
		case "bcn_lastBlock":
			fmt.Fprint(w, `{"result":{"height":1001}}`)
			return
		}
		http.Error(w, "unexpected RPC", 400)
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient()}
	proof := bytes.Repeat([]byte{7}, 196)
	calldata := binary.BigEndian.AppendUint64(nil, 7)
	calldata = append(calldata, proof...)
	prepared := &preparedAction{
		Operation: operationFulfillFast, Contract: testContract, Amount: "0",
		Calldata: hex.EncodeToString(calldata), OfferID: 7, OrderNonce: 7,
		Bundle: bundle, Seller: author, Buyer: buyer,
		DeliveryKey: hex.EncodeToString(crypto.FromECDSAPub(&deliveryKey.PublicKey)),
		Proof:       hex.EncodeToString(proof),
	}
	if err := validatePreparedDisclosure(c, prepared); err != nil {
		t.Fatalf("funded fast delivery was refused: %v", err)
	}
	changed := *prepared
	changed.OrderNonce++
	if err := validatePreparedDisclosure(c, &changed); err == nil {
		t.Fatal("changed funded-order nonce was accepted")
	}
	changed = *prepared
	changed.Seller = buyer
	if err := validatePreparedDisclosure(c, &changed); err == nil {
		t.Fatal("another seller was accepted")
	}
	order[149] = 3
	if err := validatePreparedDisclosure(c, prepared); err == nil {
		t.Fatal("already settled order was accepted")
	}
	wrongSigner, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildTransaction(prepared, wrongSigner, 1, 230, big.NewInt(1), big.NewInt(0)); err == nil {
		t.Fatal("another identity signed the seller's fast delivery")
	}
}
