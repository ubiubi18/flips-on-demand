package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
)

func TestBondedV8CipherMatchesGenuineProofVector(t *testing.T) {
	raw, e := os.ReadFile("../../proofs/delivery/dist/delivery-v2.json")
	if e != nil {
		t.Fatal(e)
	}
	var p struct {
		Ciphertext string `json:"ciphertext"`
		Order      struct {
			BuyerKey string `json:"buyer_key"`
		} `json:"order"`
	}
	if e = json.Unmarshal(raw, &p); e != nil {
		t.Fatal(e)
	}
	scalar := func(v byte) []byte { b := bytes.Repeat([]byte{v}, 32); b[0] = 0; b[1] = 0x11; return b }
	plain, e := SecretsCanonicalBytes(204, bytes.Repeat([]byte{0x44}, 20), scalar(0x61), scalar(0x71))
	if e != nil {
		t.Fatal(e)
	}
	pub, _ := hex.DecodeString(p.Order.BuyerKey)
	ct, e := encryptBondedV8(pub, plain, scalar(0x81))
	if e != nil {
		t.Fatal(e)
	}
	if hex.EncodeToString(ct) != p.Ciphertext {
		t.Fatal("encryption differs from the genuine Groth16 ciphertext")
	}
	key, _ := crypto.ToECDSA(scalar(0x51))
	opened, e := ecies.ImportECDSA(key).Decrypt(ct, nil, nil)
	if e != nil || !bytes.Equal(opened, plain) {
		t.Fatal("node ECIES cannot open proofable ciphertext", e)
	}
	ct[120] ^= 1
	if _, e = ecies.ImportECDSA(key).Decrypt(ct, nil, nil); e == nil {
		t.Fatal("modified private delivery accepted")
	}
	if _, e = encryptBondedV8(bytes.Repeat([]byte{4}, 65), plain, scalar(0x81)); e == nil {
		t.Fatal("invalid recipient accepted")
	}
}
func TestBondedV8CurveAndTypedSigningBoundary(t *testing.T) {
	atom := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	r := new(big.Int).Mul(big.NewInt(1000), atom)
	buy, sell, e := bondedV8Quotes(r, big.NewInt(0))
	if e != nil {
		t.Fatal(e)
	}
	if sell.Cmp(new(big.Int).Mul(big.NewInt(100), atom)) != 0 || buy.Cmp(sell) <= 0 {
		t.Fatal("incorrect curve quote")
	}
	_, lower, e := bondedV8Quotes(new(big.Int).Sub(r, sell), big.NewInt(100))
	if e != nil || lower.Cmp(sell) >= 0 {
		t.Fatal("acquisition failed to lower price")
	}
	key, _ := crypto.GenerateKey()
	who, _ := nodekeyAddressOf(key)
	req := &bondedV8Request{Operation: "queueBuy", Caller: who, DeliveryKey: hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey)), Maximum: "12345678901234567890", Expires: 200}
	data, value, e := encodeBondedV8(req)
	if e != nil || len(data) != 89 || value != req.Maximum || binary.BigEndian.Uint64(data[81:]) != 200 {
		t.Fatal("typed buy encoding", e)
	}
	p := &preparedAction{Format: bondedV8Format + req.Operation, Version: 1, Operation: req.Operation, Network: "idena", Contract: strings.Repeat("11", 20), Amount: value, Calldata: hex.EncodeToString(data), BondedV8: req}
	if _, _, e = BuildTransaction(p, key, 1, 233, big.NewInt(100), big.NewInt(0)); e == nil {
		t.Fatal("unverified bonded action was signable")
	}
	req.Maximum = new(big.Int).Lsh(big.NewInt(1), 128).String()
	if _, _, e = encodeBondedV8(req); e == nil {
		t.Fatal("u128 overflow accepted")
	}
	req = &bondedV8Request{Operation: "matchNext", Caller: who}
	data, value, e = encodeBondedV8(req)
	if e != nil || !bytes.Equal(data, []byte{8}) || value != "1" {
		t.Fatal("empty ABI marker or strict value incorrect")
	}
}
func TestBondedV8RetainedInventoryTamperAndCrossTrade(t *testing.T) {
	blob := []byte("retained encrypted fixture")
	a := retainedTestCheck(blob)
	b := &bondedV8Bundle{ID: strings.Repeat("11", 32), Author: a.Author, Epoch: a.Epoch, Count: 1, Inventory: a.InventoryCommitment}
	tr := &bondedV8Trade{ID: "8", Buyer: strings.Repeat("22", 20), DeliveryKey: strings.Repeat("33", 65)}
	retained := &bondedV8Retained{testContract, tr.ID, tr.Buyer, tr.DeliveryKey, b.ID, a}
	file := filepath.Join(t.TempDir(), "retained.json")
	if e := writePrivateJSON(file, retained); e != nil {
		t.Fatal(e)
	}
	if _, e := readBondedV8Retained(testContract, tr, b, file); e != nil {
		t.Fatal(e)
	}
	tr.ID = "9"
	if _, e := readBondedV8Retained(testContract, tr, b, file); e == nil {
		t.Fatal("cross-trade retained content accepted")
	}
	tr.ID = "8"
	retained.Availability.Blobs[0].Hex = hex.EncodeToString([]byte("substituted"))
	if e := writePrivateJSON(file, retained); e != nil {
		t.Fatal(e)
	}
	if _, e := readBondedV8Retained(testContract, tr, b, file); e == nil {
		t.Fatal("CID mismatch accepted")
	}
	if e := os.Chmod(file, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := readBondedV8Private(file, 1024*1024); e == nil {
		t.Fatal("public file accepted")
	}
}
func TestBondedV8StoredKeyCannotBeOverwritten(t *testing.T) {
	file := filepath.Join(t.TempDir(), "key")
	if e := writeNewPoolV7PrivateFile(file, []byte("original")); e != nil {
		t.Fatal(e)
	}
	if e := writeNewPoolV7PrivateFile(file, []byte("replacement")); e == nil {
		t.Fatal("existing key overwritten")
	}
	raw, _ := os.ReadFile(file)
	a := sha256.Sum256(raw)
	b := sha256.Sum256([]byte("original"))
	if a != b {
		t.Fatal("existing file changed")
	}
}
