package harness

// Run against the separately built fast-candidate plus test-hooks escrow.
// This is synthetic settlement evidence; registerFast remains proof gated.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/idena-network/idena-wasm-binding/lib"
)

const fastEscrowWasmPath = "../fast-escrow-candidate/dist/fod_fast_escrow.testhooks.wasm"
const fastPointCommitment = "03023a30e0eb90ce6f6e7e05de6602738f17c4e1460dc0dcd3e1afc360a5c4aa80"
const fastPointDelivery = "03b477c00186c19c54c9eb46f75f005dc4d349cfdcdeaeabc07960813c44bf224503ca0a7b5b3ba9be95b4e19b2e6e2364b77d85380f7cdef363969146d94eaa017202de5cebb99f652cd375fcecd5e4b36e6cd0ee3240fcaf8803954bb25f128eedfd022133be65a56d3a5d63837f4eff641b18630a79500c98a96f8da7788ae7e5c978a033873f866f2a2088444707c3444a3de9b5f54d3fc3335ddc69e64e63eaa44eb03323fdc080374a652228b90f191b1d82d7858031f41359a032e87de68faeb2"
const fastContentImageID = "b8586d40d7d5b912a1ce3863009c9ccb88eb836d2b441a3a08ce32ecc85bd7d2"

func TestFastContentSealVerifiesInPinnedRuntime(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-one-flip.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ImageID string `json:"image_id"`
		Journal string `json:"journal"`
		Seal    string `json:"seal"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil || fixture.ImageID != fastContentImageID {
		t.Fatal("fixture does not name the pinned fast content image")
	}
	journal, err := hex.DecodeString(fixture.Journal)
	if err != nil || len(journal) != 273 {
		t.Fatal("invalid fixture journal")
	}
	seal, err := hex.DecodeString(fixture.Seal)
	if err != nil || len(seal) != 256 {
		t.Fatal("invalid fixture seal")
	}
	code, err := os.ReadFile(fastEscrowWasmPath)
	if err != nil {
		t.Fatal(err)
	}
	env, api := d0Env(t, code)
	gas, out := escrowCallBytes(t, env, api, code, "verifyFastContentFixture", join(journal, seal))
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("genuine fast content seal refused: %s", out)
	}
	changed := append([]byte(nil), journal...)
	changed[len(changed)-1] ^= 1
	escrowReject(t, env, api, code, "verifyFastContentFixture", join(changed, seal))
	changed = append([]byte(nil), seal...)
	changed[len(changed)-1] ^= 1
	escrowReject(t, env, api, code, "verifyFastContentFixture", join(journal, changed))
	t.Logf("genuine fast content seal in pinned Idena WASM harness: gas=%d", gas)
}

func TestFastSyntheticRegistrationMeasuresCombinedGas(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-one-flip.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ImageID string `json:"image_id"`
		Journal string `json:"journal"`
		Seal    string `json:"seal"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil || fixture.ImageID != fastContentImageID {
		t.Fatal("synthetic proof does not name the pinned image")
	}
	journal, seal := mustHex(t, fixture.Journal), mustHex(t, fixture.Seal)
	if len(journal) != 273 || len(seal) != 256 {
		t.Fatal("invalid synthetic content proof")
	}
	sessionInput, session := fastSessionInputFromFile(t, "../../proofs/fast-content/dist/synthetic-session.json")
	if !bytes.Equal(journal[56:76], sessionInput[:20]) ||
		!bytes.Equal(journal[76:80], sessionInput[20:24]) ||
		!bytes.Equal(journal[144:176], sessionInput[24:56]) {
		t.Fatal("content proof and public session disagree")
	}
	wire := sessionInput[56:]
	if len(wire) > 65535 {
		t.Fatal("session wire exceeds call limit")
	}
	call := append(append([]byte(nil), journal...), seal...)
	call = binary.BigEndian.AppendUint16(call, uint16(len(wire)))
	call = append(call, wire...)
	code, err := os.ReadFile(fastEscrowWasmPath)
	if err != nil {
		t.Fatal(err)
	}
	env, api := d0Env(t, code)
	contract := env.ContractAddressValue()
	if !bytes.Equal(journal[36:56], contract[:]) {
		t.Fatal("synthetic proof names another contract")
	}
	env.SetCaller(addressFromHex(t, hex.EncodeToString(journal[56:76])))
	env.SetBlockHeader(session.Inventory.Height, fastSyntheticHeader(mustHex(t, session.Inventory.Root)))
	env.SetBlockHeader(session.Verdict.Height, fastSyntheticHeader(mustHex(t, session.Verdict.Root)))
	bad := append([]byte(nil), call...)
	bad[273+256+2] ^= 1
	escrowReject(t, env, api, code, "registerFast", bad)
	gas, out := escrowCallBytes(t, env, api, code, "registerFast", call)
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("synthetic registration refused: %s", out)
	}
	bundle := sha256.Sum256(journal)
	key := []byte("pfcert/" + hex.EncodeToString(bundle[:]))
	if got := env.GetStorage(&lib.GasMeter{}, key); !bytes.Equal(got, call) {
		t.Fatal("registration did not retain the exact certificate")
	}
	if _, view := escrowCallBytes(t, env, api, code, "fastBundleView", bundle[:]); !strings.Contains(view, `"ok":true`) {
		t.Fatalf("registered fast bundle is unavailable: %s", view)
	}
	escrowReject(t, env, api, code, "registerFast", call)
	t.Logf("successful synthetic registerFast: wasm gas=%d, call bytes=%d", gas, len(call))
}

func TestFastProductionDeployFitsBlock(t *testing.T) {
	path := os.Getenv("FOD_FAST_PRODUCTION_WASM")
	if path == "" {
		t.Skip("set FOD_FAST_PRODUCTION_WASM to check a staged production module")
	}
	code, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	gas, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatal(err)
	}
	// Idena adds ten gas per signed transaction byte and divides WASM gas by
	// 100. Allow 512 bytes above the module length for transaction framing.
	maxGas := (gas+99)/100 + uint64(len(code)+512)*10
	if maxGas > 5_120_000 {
		t.Fatalf("conservative deploy gas %d exceeds one block", maxGas)
	}
	t.Logf("production fast deploy: wasm gas=%d, wasm bytes=%d, conservative transaction gas=%d", gas, len(code), maxGas)
}

func TestFastCandidateOmitsLegacySettlement(t *testing.T) {
	code, err := os.ReadFile(fastEscrowWasmPath)
	if err != nil {
		t.Fatal(err)
	}
	exports := wasmExports(t, code)
	for _, method := range []string{"registerFast", "fulfillFast", "buy", "refund"} {
		if kind, present := exports[method]; !present || kind != 0 {
			t.Errorf("fast candidate does not export %s", method)
		}
	}
	for _, method := range []string{"registerBundle", "deliver", "confirm", "fulfill", "reveal"} {
		if _, present := exports[method]; present {
			t.Errorf("fast candidate exposes legacy method %s", method)
		}
	}
}

func fastEscrowFixture(t *testing.T) (*MinimalEnv, *lib.GoAPI, []byte, lib.Address, lib.Address) {
	t.Helper()
	code, err := os.ReadFile(fastEscrowWasmPath)
	if err != nil {
		t.Fatalf("read lowered fast escrow candidate: %v", err)
	}
	env, api := d0Env(t, code)
	author := addressFromHex(t, strings.Repeat("22", 20))
	buyer := addressFromHex(t, strings.Repeat("33", 20))
	bundle := bytes.Repeat([]byte{0x55}, 32)
	env.SetBlockNumber(1000)
	env.SetCaller(author)
	env.CreditContractBalance(big.NewInt(100))
	if _, out := escrowCallBytes(t, env, api, code, "seedPoolBundle", join(bundle, u128Arg(100), u64Arg(2), u64Arg(1000))); !strings.Contains(out, `"ok":true`) {
		t.Fatalf("seed pool: %s", out)
	}
	fast := join([]byte{0, 230}, []byte{0, 1}, make([]byte, 4), bytes.Repeat([]byte{0x77}, 32), mustHex(t, fastPointCommitment), bytes.Repeat([]byte{0x88}, 32))
	if _, out := escrowCallBytes(t, env, api, code, "seedFastBundle", join(bundle, fast)); !strings.Contains(out, `"ok":true`) {
		t.Fatalf("seed fast record: %s", out)
	}
	secret := make([]byte, 32)
	secret[31] = 17
	key, err := gethcrypto.ToECDSA(secret)
	if err != nil {
		t.Fatal(err)
	}
	env.SetCaller(buyer)
	env.SetPayAmount(big.NewInt(100))
	env.CreditContractBalance(big.NewInt(100))
	sig := SignPoolBuy(t, env.ContractAddressValue(), bundle, 0, buyer, key)
	if _, out := escrowCallBytes(t, env, api, code, "buy", PoolBuyArgs(bundle, DeliveryKeyBytes(key), sig)); !strings.Contains(out, `"order":1`) {
		t.Fatalf("buy: %s", out)
	}
	return env, api, code, author, buyer
}

func TestFastEscrowSettlesOnlyTheStoredFundedOrder(t *testing.T) {
	env, api, code, author, buyer := fastEscrowFixture(t)
	proof, err := hex.DecodeString(fastPointDelivery)
	if err != nil || len(proof) != 196 {
		t.Fatal("bad fixed delivery vector")
	}
	bad := append([]byte(nil), proof...)
	bad[len(bad)-1] ^= 1
	env.SetBlockNumber(1001)
	env.SetCaller(author)
	escrowReject(t, env, api, code, "fulfillFast", join(u64Arg(1), bad))
	if got := env.GetStorage(&lib.GasMeter{}, []byte("pfdelivery/1")); len(got) != 0 {
		t.Fatal("rejected proof published a delivery")
	}
	gas, out := escrowCallBytes(t, env, api, code, "fulfillFast", join(u64Arg(1), proof))
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("fulfill: %s", out)
	}
	if got := env.BalanceOf(author); got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("author payout %s", got)
	}
	if got := env.GetStorage(&lib.GasMeter{}, []byte("pfdelivery/1")); !bytes.Equal(got, proof) {
		t.Fatal("retained delivery does not match the verified proof")
	}
	if _, view := escrowCall(t, env, api, code, "fastDeliveryView", "1"); !strings.Contains(view, `"proof":"`+hex.EncodeToString(proof)+`"`) {
		t.Fatalf("buyer cannot retrieve the verified delivery proof: %s", view)
	}
	escrowReject(t, env, api, code, "fulfillFast", join(u64Arg(1), proof))
	env.SetBlockNumber(1003)
	env.SetCaller(buyer)
	escrowRejectCall(t, env, api, code, "refund", "1")
	t.Logf("fast settlement in pinned Idena WASM harness: gas=%d", gas)
}

func TestFastEscrowDeadlineAndMalformedRegistration(t *testing.T) {
	env, api, code, author, buyer := fastEscrowFixture(t)
	escrowReject(t, env, api, code, "registerFast", []byte{1})
	proof := mustHex(t, fastPointDelivery)
	env.SetBlockNumber(1003)
	env.SetCaller(author)
	escrowReject(t, env, api, code, "fulfillFast", join(u64Arg(1), proof))
	env.SetCaller(buyer)
	if _, out := escrowCall(t, env, api, code, "refund", "1"); !strings.Contains(out, `"ok":true`) {
		t.Fatalf("refund: %s", out)
	}
	escrowReject(t, env, api, code, "fulfillFast", join(u64Arg(1), proof))
}

func TestFastCertificateKeepsTheCompleteVerifiedCall(t *testing.T) {
	code, err := os.ReadFile(fastEscrowWasmPath)
	if err != nil {
		t.Fatal(err)
	}
	env, api := d0Env(t, code)
	bundle := bytes.Repeat([]byte{0x55}, 32)
	key := []byte("pfcert/" + strings.Repeat("55", 32))
	for _, size := range []int{5 * 1024, 273 + 256 + 2 + 65535} {
		certificate := bytes.Repeat([]byte{0xa7}, size)
		gas, out := escrowCallBytes(t, env, api, code, "seedFastCertificate", join(bundle, certificate))
		if !strings.Contains(out, `"ok":true`) {
			t.Fatalf("seed %d-byte certificate: %s", size, out)
		}
		if got := env.GetStorage(&lib.GasMeter{}, key); !bytes.Equal(got, certificate) {
			t.Fatalf("stored certificate length = %d, want %d", len(got), len(certificate))
		}
		t.Logf("retained %d certificate bytes: %d WASM gas", size, gas)
	}
}
