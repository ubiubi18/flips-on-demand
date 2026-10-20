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

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/crypto"
)

func TestDirectV7PoolOfferIDAndRustCallLayouts(t *testing.T) {
	var author [20]byte
	copy(author[:], bytes.Repeat([]byte{0x11}, 20))
	id := poolV7OfferID(author, 230)
	if got := hex.EncodeToString(id[:]); got != "5dc83998bccd9b06287506b259946959f0d6e666498347292995bda4dcca9bf6" || id == poolV6OfferID(author, 230) {
		t.Fatalf("wrong v7 offer domain: %s", got)
	}
	var inventory, keys [32]byte
	copy(inventory[:], bytes.Repeat([]byte{0x22}, 32))
	copy(keys[:], bytes.Repeat([]byte{0x33}, 32))
	wire := []byte{0xa1, 0xb2, 0xc3}
	qualify, err := encodePoolV7Qualify(230, 2, inventory, keys, wire)
	if err != nil || hex.EncodeToString(qualify) != "00e600000002"+strings.Repeat("22", 32)+strings.Repeat("33", 32)+"0003a1b2c3" {
		t.Fatalf("wrong v7 qualification layout: %v", err)
	}
	journal, seal := bytes.Repeat([]byte{0x44}, 273), bytes.Repeat([]byte{0x55}, 256)
	prove, err := encodePoolV7ProveBundle(id, journal, seal, wire)
	if err != nil || len(prove) != 566 || !bytes.Equal(prove[:32], id[:]) ||
		!bytes.Equal(prove[32:305], journal) || !bytes.Equal(prove[305:561], seal) ||
		!bytes.Equal(prove[561:], []byte{0, 3, 0xa1, 0xb2, 0xc3}) {
		t.Fatalf("wrong v7 proveBundle layout: %v", err)
	}
	seed, err := encodePoolV7SeedID(id)
	if err != nil || len(seed) != 32 || !bytes.Equal(seed, id[:]) {
		t.Fatalf("wrong v7 single-seed constructor: %v", err)
	}
	minimum := big.NewInt(7)
	list, err := encodePoolV7List(230, 2, inventory, keys, minimum)
	if err != nil || len(list) != 86 || !bytes.Equal(list[:70], qualify[:70]) || list[85] != 7 {
		t.Fatalf("wrong v7 listing layout: %v", err)
	}
	key, _ := hex.DecodeString("0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8")
	buy, err := encodePoolV7Buy(id, key)
	if err != nil || len(buy) != 97 || !bytes.Equal(buy[:32], id[:]) || !bytes.Equal(buy[32:], key) {
		t.Fatalf("wrong v7 buy layout: %v", err)
	}
	order, err := encodePoolV7OrderID(0x0102030405060708)
	if err != nil || hex.EncodeToString(order) != "0102030405060708" {
		t.Fatalf("wrong v7 order-only layout: %v", err)
	}
	secrets := make([]byte, 106)
	copy(secrets, directSecretsDomain)
	binary.BigEndian.PutUint16(secrets[len(directSecretsDomain):], 230)
	copy(secrets[len(directSecretsDomain)+2:], author[:])
	seedBundle, err := encodePoolV7SeedBundle(id, secrets)
	if err != nil || len(seedBundle) != 138 || !bytes.Equal(seedBundle[:32], id[:]) || !bytes.Equal(seedBundle[32:], secrets) {
		t.Fatalf("wrong v7 public seed layout: %v", err)
	}
	reveal, err := encodePoolV7ProveAndReveal(0x0102030405060708, secrets)
	if err != nil || len(reveal) != 114 || !bytes.Equal(reveal[:8], order) || !bytes.Equal(reveal[8:], secrets) {
		t.Fatalf("wrong v7 reveal layout: %v", err)
	}
	if _, err := encodePoolV7SeedID([32]byte{}); err == nil {
		t.Fatal("accepted zero seed ID")
	}
	if _, err := encodePoolV7OrderID(0); err == nil {
		t.Fatal("accepted zero order ID")
	}
	if _, err := encodePoolV7SeedBundle(poolV6OfferID(author, 230), secrets); err == nil {
		t.Fatal("accepted v6 offer ID for v7 seed")
	}
}

func TestDirectV7PreSaleLegacyProofBinding(t *testing.T) {
	contract := strings.Repeat("11", 20)
	for _, operation := range []string{"qualify", "proveBundle"} {
		if !poolV7ProofBindingAllowed(poolV7LegacyContentContract, contract, 230, operation) {
			t.Fatalf("epoch-230 legacy proof rejected for %s", operation)
		}
		if !poolV7ProofBindingAllowed(contract, contract, 231, operation) {
			t.Fatalf("native proof rejected for %s", operation)
		}
		if poolV7ProofBindingAllowed(poolV7LegacyContentContract, contract, 231, operation) ||
			poolV7ProofBindingAllowed(strings.Repeat("22", 20), contract, 230, operation) {
			t.Fatalf("unbound proof accepted for %s", operation)
		}
	}
	if poolV7ProofBindingAllowed(poolV7LegacyContentContract, contract, 230, "startFallback") {
		t.Fatal("legacy content proof accepted for a funded fallback")
	}
}

func TestDirectV7RecordsAndQuoteUseV7OffsetsAndCurve(t *testing.T) {
	var raw [222]byte
	copy(raw[:20], bytes.Repeat([]byte{0x11}, 20))
	binary.BigEndian.PutUint16(raw[20:22], 230)
	binary.BigEndian.PutUint32(raw[22:26], 1)
	raw[26], raw[58], raw[105] = 1, 2, 3
	binary.BigEndian.PutUint64(raw[106:114], 1)
	raw[122], raw[204] = 1, 1
	offer, err := decodePoolV7Offer(raw[:])
	if err != nil || offer.status != 1 || offer.sellerBond.Int64() != 3 || !offer.contentProven {
		t.Fatalf("wrong v7 offer offsets: %v", err)
	}
	if _, err := decodePoolV7Offer(raw[:221]); err == nil {
		t.Fatal("accepted v6-sized offer")
	}
	reserve, _ := new(big.Int).SetString(poolV7SeedAmount, 10)
	quote, err := poolV7Quote(1, reserve, big.NewInt(0))
	if err != nil || quote.String() != poolV7PriceCap {
		t.Fatalf("wrong one-seed quote: %v, %v", quote, err)
	}
	quote, err = poolV7Quote(2, reserve, big.NewInt(0))
	if err != nil || quote.String() != "909090909090909090910" {
		t.Fatalf("wrong first private listing quote: %v, %v", quote, err)
	}
}

func TestDirectV7DeployOneArgumentAndSignGate(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	id := [32]byte{1}
	raw, _, _, err := BuildDirectV7PoolDeployTransaction(deployModule(), deployNonceFixture(), id[:], key, 7, 230, big.NewInt(1), big.NewInt(1_000_000), big.NewInt(0))
	if err != nil {
		t.Fatal(err)
	}
	var tx types.Transaction
	if err := tx.FromBytes(raw); err != nil {
		t.Fatal(err)
	}
	parsed := attachments.ParseDeployContractAttachment(&tx)
	if parsed == nil || len(parsed.Args) != 1 || !bytes.Equal(parsed.Args[0], id[:]) || tx.Amount == nil || tx.Amount.Cmp(big.NewInt(1)) != 0 {
		t.Fatal("v7 signed deploy lost the single constructor ID or refundable call value")
	}
	if _, _, _, err := BuildDirectV7PoolDeployTransaction(deployModule(), deployNonceFixture(), append(id[:], id[:]...), key, 7, 230, big.NewInt(1), big.NewInt(1_000_000), big.NewInt(0)); err == nil {
		t.Fatal("v7 accepted v6 two-seed constructor")
	}
	for _, invalid := range []int64{0, 2} {
		if _, _, _, err := BuildDirectV7PoolDeployTransaction(deployModule(), deployNonceFixture(), id[:], key, 7, 230, big.NewInt(invalid), big.NewInt(1_000_000), big.NewInt(0)); err == nil {
			t.Fatalf("v7 accepted deployment value %d", invalid)
		}
	}
	p := &preparedAction{Format: preparedPoolV7Challenge, Version: 1, Operation: "challenge", Network: NetworkNamePinned,
		Contract: strings.Repeat("11", 20), DirectVersion: 7, Amount: "0", Calldata: "0000000000000001"}
	if _, _, err := BuildTransaction(p, key, 7, 230, big.NewInt(1_000_000), big.NewInt(0)); err == nil {
		t.Fatal("typed v7 call signed without current-state validation")
	}
}

func TestDirectV7DeployManifestAndIndependentCodePin(t *testing.T) {
	code := deployModule()
	root := t.TempDir()
	codePath := filepath.Join(root, "release", "artifacts", "fod_direct_escrow_v7.wasm")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o700); err != nil {
		t.Fatal(err)
	}
	sha := sha256.Sum256(code)
	hash := crypto.Hash(code)
	gas := uint64(1200)
	payloadBytes := uint64(len(code) + 32)
	manifest := map[string]any{
		"format": "fod.direct-candidate-manifest", "version": 7, "status": "candidate-only", "nonpayableCallValue": 1,
		"artifact": map[string]any{
			"path": "release/artifacts/fod_direct_escrow_v7.wasm", "bytes": len(code),
			"sha256": hex.EncodeToString(sha[:]), "codeHash": hex.EncodeToString(hash[:]),
		},
		"checks": map[string]any{
			"fastContentImageId": fastContentV5ImageIDHex, "blockGasLimit": maxUserGasUpgrade11,
			"deploy": map[string]any{
				"wasmGas": gas, "payloadBytes": payloadBytes,
				"conservativeTransactionGas": userGasForWasmGas(gas) + (payloadBytes+2048)*10,
			},
		},
	}
	manifestPath := filepath.Join(root, "release", "DIRECT_V7_MANIFEST.json")
	write := func() {
		t.Helper()
		raw, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if err := os.WriteFile(codePath, code, 0o600); err != nil {
		t.Fatal(err)
	}
	measured, err := deployGasForDirectV7Code(codePath, code)
	if err != nil || measured.TxBytes != payloadBytes {
		t.Fatalf("one-seed deployment must count all constructor bytes: gas=%+v err=%v", measured, err)
	}
	seedID := strings.Repeat("01", 32)
	prepare := func(pin, keyPath, outPath string) error {
		return runPrepareDeployV7(codePath, pin, keyPath, hex.EncodeToString(deployNonceFixture()), seedID,
			NetworkNamePinned, outPath, 7, 230, big.NewInt(1), big.NewInt(1_000_000), big.NewInt(0), big.NewInt(1), false)
	}
	for _, tc := range []struct {
		name, pin, want string
	}{
		{"missing pin", "", "requires an independent lowercase"},
		{"wrong pin", strings.Repeat("00", 32), "differs from independent"},
		{"uppercase pin", strings.ToUpper(hex.EncodeToString(hash[:])), "requires an independent lowercase"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outPath := filepath.Join(root, tc.name+".json")
			err := prepare(tc.pin, filepath.Join(root, "missing-nodekey"), outPath)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("pin gate failed before nodekey load: %v", err)
			}
			if _, err := os.Stat(outPath); !os.IsNotExist(err) {
				t.Fatalf("refused deployment wrote a signed file: %v", err)
			}
		})
	}
	keyPath, _ := writeTestKey(t, root, "nodekey")
	outPath := filepath.Join(root, "signed.json")
	if err := prepare(hex.EncodeToString(hash[:]), keyPath, outPath); err != nil {
		t.Fatalf("matching independent pin refused: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var prepared PreparedDeploy
	if err := json.Unmarshal(raw, &prepared); err != nil || prepared.CodeHash != hex.EncodeToString(hash[:]) ||
		prepared.RawTx == "" || prepared.Amount != "1" || prepared.Broadcast {
		t.Fatalf("matching pin did not produce a one-unit offline deploy: %+v, %v", prepared, err)
	}
	signed, err := hex.DecodeString(prepared.RawTx)
	if err != nil {
		t.Fatal(err)
	}
	shortFee := requiredMaxFeeFor(gas, uint64(len(signed))+directV7DeployHeadroomBytes/2, big.NewInt(1))
	if err := runPrepareDeployV7(codePath, hex.EncodeToString(hash[:]), keyPath, hex.EncodeToString(deployNonceFixture()), seedID,
		NetworkNamePinned, filepath.Join(root, "short-fee.json"), 7, 230, big.NewInt(1), shortFee, big.NewInt(0), big.NewInt(1), false); err == nil || !strings.Contains(err.Error(), "below conservative minimum") {
		t.Fatalf("v7 deploy accepted a max fee without execution headroom: %v", err)
	}
	manifest["checks"].(map[string]any)["deploy"].(map[string]any)["payloadBytes"] = len(code)
	write()
	if _, err := deployGasForDirectV7Code(codePath, code); err == nil {
		t.Fatal("deployment accepted a manifest that omitted its constructor")
	}
}
