package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/idena-network/idena-go/crypto"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBondedV9StartsAt500AndPreservesV8Quotes(t *testing.T) {
	s, _, _ := standaloneV8Fixture(t)
	s.client.bondedVersion = 9
	snapshot, e := readBondedV8Snapshot(s.client, s.cfg.Contract, s.cfg.CodeHash)
	if e != nil || snapshot.Version != 9 || snapshot.BuyQuote != "500000000000000000000" {
		t.Fatal("v9 snapshot must use the new starting price", e)
	}
	buy, sell, e := bondedQuotes(9, big.NewInt(0), big.NewInt(0))
	if e != nil || buy.String() != "500000000000000000000" || sell.String() != "409090909090909090909" {
		t.Fatal("v9 initial package price must be exactly 500 iDNA", buy, sell, e)
	}
	legacyBuy, legacySell, e := bondedQuotes(8, big.NewInt(0), big.NewInt(0))
	if e != nil || legacyBuy.String() != "11111111111111111112" || legacySell.String() != "9090909090909090909" {
		t.Fatal("v8 quotes changed", e)
	}
	reserve, _ := new(big.Int).SetString("1000000000000000000000", 10)
	before, proceeds, e := bondedQuotes(9, reserve, big.NewInt(0))
	if e != nil {
		t.Fatal(e)
	}
	after, _, e := bondedQuotes(9, new(big.Int).Sub(reserve, proceeds), big.NewInt(100))
	if e != nil || after.Cmp(before) >= 0 {
		t.Fatal("acquired supply did not lower the quote", e)
	}
	restored, _, e := bondedQuotes(9, reserve, big.NewInt(0))
	if e != nil || restored.Cmp(after) <= 0 {
		t.Fatal("settled demand did not raise the quote", e)
	}
}

func TestBondedV9ListingVerifiesOriginalContentWithoutProof(t *testing.T) {
	c, session, _, _, _, _, vector, _, closeNode := directClientFixture(t)
	defer closeNode()
	c.bondedVersion = 9
	// Use the public clean-validation fixture over the same original encrypted flip.
	var f struct {
		Author   string `json:"author"`
		Sessions []struct {
			Inventory SessionAnchor `json:"inventory"`
			Verdict   SessionAnchor `json:"verdict"`
		} `json:"sessions"`
	}
	raw, e := os.ReadFile("../../proofs/fast-content/dist/synthetic-session.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	verified, e := sessionBundleFromAnchors(f.Author, &f.Sessions[0].Inventory, &f.Sessions[0].Verdict)
	if e != nil {
		t.Fatal(e)
	}
	*session = *verified
	dir := t.TempDir()
	sessionFile := filepath.Join(dir, "session.json")
	if e = writePrivateJSON(sessionFile, session); e != nil {
		t.Fatal(e)
	}
	_, vault, e := epochKeysFromDeliveredSecrets(mustDecode(vector.SecretsCanonical))
	if e != nil {
		t.Fatal(e)
	}
	vaultFile := filepath.Join(dir, "keys.vault")
	raw, _ = json.Marshal(vault)
	pass := "local synthetic package passphrase"
	t.Setenv("FOD_PASSPHRASE", pass)
	if e = WriteVault(vaultFile, pass, raw); e != nil {
		t.Fatal(e)
	}
	contract := strings.Repeat("11", 20)
	r := &bondedV8Request{ProtocolVersion: 9, Operation: "queueSell", Caller: session.Author, SessionFile: sessionFile, VaultFile: vaultFile, Minimum: "1", Capacity: "1000", Expires: 10890000}
	if e = fillBondedV9Listing(c, contract, r, pass); e != nil {
		t.Fatal(e)
	}
	if r.Seal != "" || r.Journal == "" || r.Bundle == standaloneBundleID(r.Caller, session.Epoch) {
		t.Fatal("listing used a proof or v8 bundle domain")
	}
	journal, _ := hex.DecodeString(r.Journal)
	if hex.EncodeToString(journal[241:]) != vector.SecretsCommitment || !bytes.Equal(journal[176:241], make([]byte, 65)) {
		t.Fatal("incorrect public commitment")
	}
	wire, value, e := encodeBondedV8(r)
	if e != nil || value != "1" || len(wire) != 40+273+2+len(r.SessionWire)/2 || binary.BigEndian.Uint64(wire[32:40]) != r.Expires {
		t.Fatal("incorrect atomic listing ABI", e)
	}
	if e = verifyBondedV8LocalAction(c, contract, r); e != nil {
		t.Fatal(e)
	}
	changed := *r
	changed.Journal = strings.Repeat("00", 273)
	if e = verifyBondedV8LocalAction(c, contract, &changed); e == nil {
		t.Fatal("changed commitment accepted by signing preflight")
	}
	if e = fillBondedV9Listing(c, contract, r, "wrong passphrase"); e == nil {
		t.Fatal("unopened vault accepted")
	}
	changed = *r
	changed.Caller = strings.Repeat("44", 20)
	if e = fillBondedV9Listing(c, contract, &changed, pass); e == nil {
		t.Fatal("another identity can list the original flips")
	}
	changed = *r
	changed.Seal = strings.Repeat("00", 256)
	if _, _, e = encodeBondedV8(&changed); e == nil {
		t.Fatal("upfront proof silently accepted")
	}
}

func TestBondedV9DisputeAndVersionBoundaries(t *testing.T) {
	r := &bondedV8Request{ProtocolVersion: 9, Operation: "registerBundle", Caller: strings.Repeat("11", 20)}
	if _, _, e := encodeBondedV8(r); e == nil {
		t.Fatal("v9 registration accepted")
	}
	r.Operation = "proveDelivery"
	r.ID = "7"
	r.Journal = strings.Repeat("22", 281)
	r.Seal = strings.Repeat("33", 256)
	if _, _, e := encodeBondedV8(r); e == nil {
		t.Fatal("delivery-only dispute accepted")
	}
	r.ContentJournal = strings.Repeat("44", 273)
	r.ContentSeal = strings.Repeat("55", 256)
	wire, _, e := encodeBondedV8(r)
	if e != nil || len(wire) != 8+273+256+281+256 || wire[8] != 0x44 || wire[537] != 0x22 {
		t.Fatal("incorrect combined dispute ABI", e)
	}
	r.ProtocolVersion = 10
	if _, _, e = encodeBondedV8(r); e == nil {
		t.Fatal("unknown market version accepted")
	}
	s := &standaloneServer{cfg: standaloneConfig{Version: 9}}
	pkg, e := s.canonicalPackage(strings.Repeat("33", 20), 233, []string{cidRawSha256([]byte("fixture"))})
	if e != nil {
		t.Fatal(e)
	}
	s.cfg.DataDir = t.TempDir()
	if e = s.savePackage(pkg); e != nil {
		t.Fatal(e)
	}
	dir, _ := s.packageDir(pkg.ID)
	if _, e = s.readPackage(dir); e != nil {
		t.Fatal(e)
	}
	s.cfg.Version = 8
	if _, e = s.readPackage(dir); e == nil {
		t.Fatal("v8 loaded a v9 package")
	}
}

func TestBondedV9BuyerRejectsWrongKeysEvenWhenCommitmentMatches(t *testing.T) {
	var v blobVector
	raw, e := os.ReadFile("../../proofs/registration/vectors/blob-flip-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	key, e := crypto.GenerateKey()
	if e != nil {
		t.Fatal(e)
	}
	public := hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey))
	contract := strings.Repeat("11", 20)
	bundle := &bondedV8Bundle{ID: bondedBundleID(9, v.Author, v.Epoch), Author: v.Author, Epoch: v.Epoch, Count: 1, CIDs: []string{v.BlobCID}, Inventory: inventoryCommitment(v.Author, v.Epoch, []string{v.BlobCID})}
	trade := &bondedV8Trade{ID: "1", Buyer: strings.Repeat("33", 20), Seller: v.Author, DeliveryKey: public, Bundle: bundle.ID}
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "delivery.key")
	if e = os.WriteFile(keyFile, []byte(hex.EncodeToString(crypto.FromECDSA(key))), 0600); e != nil {
		t.Fatal(e)
	}
	retained := &bondedV8Retained{Contract: contract, Trade: trade.ID, Buyer: trade.Buyer, DeliveryKey: public, Bundle: bundle.ID, Availability: &AvailabilityCheck{Author: v.Author, Epoch: v.Epoch, Binding: bindingCommitted, CoveredFlips: 1, VerifiedFlips: 1, InventoryCommitment: bundle.Inventory, Blobs: []RetainedFlip{{CID: v.BlobCID, Hex: v.Blob}}}}
	retainedFile := filepath.Join(dir, "retained.json")
	if e = writePrivateJSON(retainedFile, retained); e != nil {
		t.Fatal(e)
	}
	var ciphertext []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var prefix string
		_ = json.Unmarshal(req.Params[1], &prefix)
		if req.Method != "contract_readMap" || prefix != "v9/ciphertext/" {
			t.Error("wrong ciphertext source")
			http.Error(w, "wrong source", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "0x" + hex.EncodeToString(ciphertext)})
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient(), bondedVersion: 9}
	r := &bondedV8Request{ProtocolVersion: 9, RetainedFile: retainedFile, DeliveryKeyFile: keyFile}
	for _, wrong := range []bool{false, true} {
		plain := mustDecode(v.SecretsCanonical)
		if wrong {
			plain[len(plain)-1] ^= 1
		}
		digest := sha256.Sum256(plain)
		bundle.Keys = hex.EncodeToString(digest[:])
		scalar := make([]byte, 32)
		scalar[31] = 3
		ciphertext, e = encryptBondedV8(crypto.FromECDSAPub(&key.PublicKey), plain, scalar)
		if e != nil {
			t.Fatal(e)
		}
		digest = sha256.Sum256(ciphertext)
		trade.CiphertextHash = hex.EncodeToString(digest[:])
		_, e = openBondedV8(c, contract, trade, bundle, r)
		if !wrong && e != nil {
			t.Fatal("valid original content refused", e)
		}
		if wrong && e == nil {
			t.Fatal("matching commitment hid incorrect original keys")
		}
	}
}
