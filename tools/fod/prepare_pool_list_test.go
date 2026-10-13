package main

// The pool's supply-side preparation: a gift carries the commitment, a sale
// carries the commitment followed by the sealed registration receipt, and both
// write prepared files the submit path can sign.

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func poolListRequestJSON(contract, operation, commitment string) string {
	return fmt.Sprintf(`{
  "format": "fod.action-request",
  "version": 1,
  "network": "idena-local",
  "contract": %q,
  "operation": %q,
  "arguments": {
    "commitment": %q
  },
  "amount": "0",
  "createdAt": "2026-09-26T12:00:00Z"
}`, contract, operation, commitment)
}

func TestPreparePoolDonateUnitBuildsTheCommitment(t *testing.T) {
	commitment := strings.Repeat("ab", poolBundleBytes)
	requestPath := writeRequest(t, t.TempDir(), poolListRequestJSON(testContract, operationDonateUnit, commitment))
	request, arguments, err := loadPoolListRequest(requestPath)
	if err != nil {
		t.Fatalf("load the gift request: %v", err)
	}
	prepared, err := PreparePoolList(request, arguments, "", nil, "")
	if err != nil {
		t.Fatalf("prepare the gift: %v", err)
	}
	if prepared.Format != preparedDonateUnitFormat || prepared.Operation != operationDonateUnit {
		t.Fatalf("the gift is %s / %s", prepared.Format, prepared.Operation)
	}
	calldata, err := hex.DecodeString(prepared.Calldata)
	if err != nil {
		t.Fatalf("decode the gift calldata: %v", err)
	}
	if hex.EncodeToString(calldata) != commitment {
		t.Fatalf("the gift calldata is %x", calldata)
	}
	out := filepath.Join(t.TempDir(), "give.json")
	if err := writeJSON(out, prepared); err != nil {
		t.Fatalf("write the gift: %v", err)
	}
	loaded, err := loadPreparedAction(out)
	if err != nil {
		t.Fatalf("load the gift through the submit reader: %v", err)
	}
	if loaded.Operation != operationDonateUnit {
		t.Fatalf("the loaded gift names %s", loaded.Operation)
	}
}

func TestPreparePoolSellUnitBuildsTheReceiptArgument(t *testing.T) {
	artefact := loadSealedRegistration(t)
	journal, err := hex.DecodeString(artefact.Journal)
	if err != nil || len(journal) != registrationJournalBytes {
		t.Fatalf("decode the receipt journal: %v (%d bytes)", err, len(journal))
	}
	seal, err := hex.DecodeString(artefact.Seal)
	if err != nil || len(seal) != registrationSealBytes {
		t.Fatalf("decode the receipt seal: %v (%d bytes)", err, len(seal))
	}
	contract := hex.EncodeToString(journal[registrationContractOffset : registrationContractOffset+addressBytes])
	commitment := hex.EncodeToString(journal[registrationCommitmentOffset : registrationCommitmentOffset+poolBundleBytes])
	author := hex.EncodeToString(journal[registrationAuthorOffset : registrationAuthorOffset+addressBytes])

	requestPath := writeRequest(t, t.TempDir(), poolListRequestJSON(contract, operationSellUnit, commitment))
	request, arguments, err := loadPoolListRequest(requestPath)
	if err != nil {
		t.Fatalf("load the sale request: %v", err)
	}
	prepared, err := PreparePoolList(request, arguments, sealedRegistrationArtefact, artefact, author)
	if err != nil {
		t.Fatalf("prepare the sale: %v", err)
	}
	if prepared.Format != preparedSellUnitFormat || prepared.Operation != operationSellUnit {
		t.Fatalf("the sale is %s / %s", prepared.Format, prepared.Operation)
	}
	calldata, err := hex.DecodeString(prepared.Calldata)
	if err != nil {
		t.Fatalf("decode the sale calldata: %v", err)
	}
	if len(calldata) != poolBundleBytes+registrationJournalBytes+registrationSealBytes {
		t.Fatalf("the sale calldata is %d bytes", len(calldata))
	}
	if hex.EncodeToString(calldata[:poolBundleBytes]) != commitment {
		t.Fatalf("the sale calldata starts with %x", calldata[:poolBundleBytes])
	}
	if !bytes.Equal(calldata[poolBundleBytes:poolBundleBytes+registrationJournalBytes], journal) {
		t.Fatal("the sale calldata does not carry the receipt's journal")
	}
	if !bytes.Equal(calldata[poolBundleBytes+registrationJournalBytes:], seal) {
		t.Fatal("the sale calldata does not carry the receipt's seal")
	}
	out := filepath.Join(t.TempDir(), "sell.json")
	if err := writeJSON(out, prepared); err != nil {
		t.Fatalf("write the sale: %v", err)
	}
	loaded, err := loadPreparedAction(out)
	if err != nil {
		t.Fatalf("load the sale through the submit reader: %v", err)
	}
	if loaded.Operation != operationSellUnit || loaded.Commitment != commitment {
		t.Fatalf("the loaded sale is %+v", loaded)
	}

	// The receipt has to name the bundle's author and the bundle the request
	// names; a missing receipt and a receipt on a gift are both refused.
	if _, err := PreparePoolList(request, arguments, sealedRegistrationArtefact, artefact, strings.Repeat("99", addressBytes)); err == nil {
		t.Fatal("a receipt for another author was accepted")
	}
	otherCommitment := strings.Repeat("cd", poolBundleBytes)
	otherRequestPath := writeRequest(t, t.TempDir(), poolListRequestJSON(contract, operationSellUnit, otherCommitment))
	otherRequest, otherArguments, err := loadPoolListRequest(otherRequestPath)
	if err != nil {
		t.Fatalf("load the other sale request: %v", err)
	}
	if _, err := PreparePoolList(otherRequest, otherArguments, sealedRegistrationArtefact, artefact, author); err == nil {
		t.Fatal("a receipt for another bundle was accepted")
	}
	if _, err := PreparePoolList(request, arguments, "", nil, author); err == nil {
		t.Fatal("a sale without a receipt was accepted")
	}
	giftPath := writeRequest(t, t.TempDir(), poolListRequestJSON(contract, operationDonateUnit, commitment))
	giftRequest, giftArguments, err := loadPoolListRequest(giftPath)
	if err != nil {
		t.Fatalf("load the gift request: %v", err)
	}
	if _, err := PreparePoolList(giftRequest, giftArguments, sealedRegistrationArtefact, artefact, author); err == nil {
		t.Fatal("a gift with a receipt was accepted")
	}
}

// TestRegistryPoolBundleReadsTheRawKeyLayout: the pool's records are keyed by
// the prefix plus the raw commitment, so the read has to walk the prefix rather
// than ask for the commitment's hex text.
func TestRegistryPoolBundleReadsTheRawKeyLayout(t *testing.T) {
	commitment := strings.Repeat("ab", poolBundleBytes)
	record := poolRecordBytes(filled(0x33, 20), filled(0x44, 32), filled(0x55, 32), 3, 900, 0, 7, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if request.Method == "contract_readData" {
			key, _ := request.Params[1].(string)
			value, ok := poolStubCounters(key)
			if !ok {
				http.Error(w, "unexpected key "+key, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": value})
			return
		}
		if request.Method != "contract_iterateMap" {
			http.Error(w, "unexpected method "+request.Method, http.StatusBadRequest)
			return
		}
		prefix, _ := request.Params[1].(string)
		if prefix != marketPoolPrefix {
			http.Error(w, "unexpected prefix "+prefix, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"items": []map[string]string{{
					"key":   commitment,
					"value": hex.EncodeToString(record),
				}},
				"continuationToken": "0x",
			},
		})
	}))
	defer server.Close()

	node := &client{url: server.URL, http: newHTTPClient()}
	listing, err := registryPoolBundle(node, testContract, commitment)
	if err != nil {
		t.Fatalf("read the pool bundle: %v", err)
	}
	if listing.Bundle != commitment || listing.Reserve != "50000" || listing.Units != 3 || listing.Nonce != 7 {
		t.Fatalf("the pool bundle reads as %+v", listing)
	}
}

// The supply operations are signed by the holder, so their fee entries need the
// canonical signed sizes the same way the desk's do.
func TestPoolSupplyCanonicalSignedSizes(t *testing.T) {
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the signing key: %v", err)
	}
	contract := strings.Repeat("aa", addressBytes)
	cases := []struct {
		operation string
		format    string
		calldata  []byte
	}{
		{operationRegisterBundle, preparedPoolRegistrationFormat, make([]byte, registrationJournalBytes+registrationSealBytes)},
		{operationSellUnit, preparedSellUnitFormat, make([]byte, poolBundleBytes+registrationJournalBytes+registrationSealBytes)},
		{operationDonateUnit, preparedDonateUnitFormat, make([]byte, poolBundleBytes)},
	}
	for _, c := range cases {
		prepared := servePreparedAction(c.operation, c.format, contract, "testnet", c.calldata, 0, "", "")
		raw, _, err := BuildTransaction(prepared, identity, 1, 228, big.NewInt(1_000_000_000_000_000), big.NewInt(0))
		if err != nil {
			t.Errorf("%s: sign: %v", c.operation, err)
			continue
		}
		gas, ok := measuredGasFor(c.operation)
		if !ok {
			t.Errorf("%s has no measured gas", c.operation)
			continue
		}
		if gas.TxBytes != uint64(len(raw)) {
			t.Errorf("%s signs %d bytes, the fee table records %d", c.operation, len(raw), gas.TxBytes)
		}
	}
}
