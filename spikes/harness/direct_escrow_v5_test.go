package harness

// Exercise the v5 candidate with the sealed synthetic content receipt and its
// matching four-leaf session in the pinned Idena WASM runtime.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

const directV5WasmPath = "../../contracts/direct-escrow-v5/target/fod_direct_escrow_v5.lowered.wasm"
const directV5FastContentImageID = "4fd124c8cb91370dab60d3e1d7c4e666e79485e210b5f20ed36cc2104883726c"

func TestDirectV5SyntheticFallbackRuntimeAndGas(t *testing.T) {
	code, err := os.ReadFile(directV5WasmPath)
	if err != nil {
		t.Fatalf("read lowered v5 candidate: %v", err)
	}
	raw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-v5-one-flip.json")
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Format    string `json:"format"`
		Statement string `json:"statement"`
		ImageID   string `json:"image_id"`
		Journal   string `json:"journal"`
		Seal      string `json:"seal"`
	}
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	if proof.Format != "fod:fast-content-sealed:v1" || proof.Statement != "fod:fast-content:v1" || proof.ImageID != directV5FastContentImageID {
		t.Fatal("synthetic content receipt differs from the pinned v5 claim")
	}
	journal, seal := mustHex(t, proof.Journal), mustHex(t, proof.Seal)
	if len(journal) != 273 || len(seal) != 256 {
		t.Fatalf("invalid content receipt lengths: journal=%d seal=%d", len(journal), len(seal))
	}
	sessionInput, session := fastSessionInputFromFile(t, "../../proofs/fast-content/dist/synthetic-session.json")
	if len(sessionInput) < 56 || !bytes.Equal(journal[56:76], sessionInput[:20]) ||
		!bytes.Equal(journal[76:80], sessionInput[20:24]) ||
		!bytes.Equal(journal[144:176], sessionInput[24:56]) {
		t.Fatal("sealed content receipt and four-leaf session disagree")
	}
	wire := sessionInput[56:]
	if len(wire) == 0 || len(wire) > 65535 {
		t.Fatalf("invalid bounded session witness length: %d", len(wire))
	}
	// This published toy vector is hash-pinned by the source snapshot. Its
	// canonical keys must open the commitment in the sealed v5 journal.
	vectorRaw, err := os.ReadFile("../../proofs/registration/vectors/blob-flip-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		SecretsCanonical string `json:"secretsCanonical"`
	}
	if err := json.Unmarshal(vectorRaw, &vector); err != nil {
		t.Fatal(err)
	}
	secrets := mustHex(t, vector.SecretsCanonical)
	secretsHash := sha256.Sum256(secrets)
	if len(secrets) != 106 || !bytes.Equal(secretsHash[:], journal[241:273]) {
		t.Fatal("public synthetic keys do not open the sealed v5 commitment")
	}

	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, hexOf(journal[36:56])))
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployGas, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("v5 deploy rejected by pinned runtime: %v", err)
	}
	// Ten gas per signed transaction byte; reserve 2 KiB for the method,
	// argument framing, signature, and envelope beyond each exact payload.
	txGas := func(wasmGas uint64, payloadBytes int) uint64 {
		return (wasmGas+wasmGasPerUserGas-1)/wasmGasPerUserGas + uint64(payloadBytes+2048)*10
	}
	deployTxGas := txGas(deployGas, len(code))
	if deployTxGas > maxUserGasUpgrade11 {
		t.Fatalf("v5 deploy exceeds block gas: wasm=%d bytes=%d conservative tx=%d", deployGas, len(code), deployTxGas)
	}
	call := func(method string, arg []byte) uint64 {
		t.Helper()
		defer env.ResetPayAmount()
		gas, _, err := lib.Execute(api, code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
		if err != nil {
			t.Fatalf("v5 %s rejected by pinned runtime: %v", method, err)
		}
		return gas
	}

	author := addressFromHex(t, hexOf(journal[56:76]))
	buyer := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb2}, 20)))
	buyHeight := session.Verdict.Height + 100
	env.SetBlockNumber(buyHeight)
	env.SetCaller(author)
	price := new(big.Int).Set(testPriceValue)
	covered := make([]byte, 4)
	binary.BigEndian.PutUint32(covered, uint32(binary.BigEndian.Uint16(journal[78:80])))
	listArg := join(journal[76:78], covered, journal[144:176], journal[241:273], u128ArgValue(price))
	listGas := call("list", listArg)
	h := sha256.New()
	h.Write([]byte("fod:direct-offer:v2"))
	h.Write(author[:])
	h.Write(journal[76:78])
	offerID := h.Sum(nil)
	if offer := env.GetStorage(&lib.GasMeter{}, join([]byte("direct/offer/"), offerID)); len(offer) != 123 {
		t.Fatal("v5 listing did not store the synthetic offer")
	}

	env.SetCaller(buyer)
	bond := new(big.Int).Mul(env.feePerGas, big.NewInt(2_500_000*2))
	total := new(big.Int).Add(price, bond)
	env.SetPayAmount(total)
	env.CreditContractBalance(total)
	buyArg := join(offerID, DeliveryKeyBytes(NewDeliveryKey(t)))
	buyGas := call("buy", buyArg)
	if order := env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1")); len(order) != 198 || order[165] != 1 {
		t.Fatal("v5 buy did not leave one funded order")
	}
	restoreFunded := env.Snapshot()
	checksum := sha256.Sum256([]byte("synthetic private delivery envelope"))
	confirmArg := join(u64Arg(1), checksum[:])
	confirmGas := call("confirm", confirmArg)
	if order := env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1")); len(order) != 198 || order[165] != 2 || !bytes.Equal(order[166:198], checksum[:]) {
		t.Fatal("v5 confirmation did not settle with the delivery checksum")
	}
	if env.BalanceOf(author).Cmp(price) != 0 || env.BalanceOf(buyer).Cmp(bond) != 0 ||
		new(big.Int).SetBytes(env.GetStorage(&lib.GasMeter{}, []byte("direct/pending"))).Sign() != 0 || env.ContractBalance().Sign() != 0 {
		t.Fatal("v5 confirmation did not pay the seller and return the buyer bond")
	}
	restoreFunded()

	env.SetBlockHeader(session.Inventory.Height, proposedHeaderWire(t, session.Inventory.Height, mustHex(t, session.Inventory.Root)))
	env.SetBlockHeader(session.Verdict.Height, proposedHeaderWire(t, session.Verdict.Height, mustHex(t, session.Verdict.Root)))
	env.SetBlockNumber(buyHeight + directConfirmGraceBlocks)
	env.SetCaller(author)
	startArg := join(u64Arg(1), journal, seal, []byte{byte(len(wire) >> 8), byte(len(wire))}, wire)
	env.SetPayAmount(price)
	env.CreditContractBalance(price)
	startGas := call("startFallback", startArg)
	if order := env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1")); len(order) != 198 || order[165] != 4 {
		t.Fatal("v5 fallback did not enter proof-backed pending state")
	}
	if stake := env.GetStorage(&lib.GasMeter{}, []byte("direct/stake/1")); len(stake) != 16 || new(big.Int).SetBytes(stake).Cmp(price) != 0 {
		t.Fatal("v5 fallback did not retain the payable seller stake")
	}
	if want := new(big.Int).Add(total, price); new(big.Int).SetBytes(env.GetStorage(&lib.GasMeter{}, []byte("direct/pending"))).Cmp(want) != 0 {
		t.Fatal("v5 fallback did not include the stake in pending liabilities")
	}
	restorePending := env.Snapshot()
	revealArg := join(u64Arg(1), secrets)
	revealGas := call("proveAndReveal", revealArg)
	if order := env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1")); len(order) != 198 || order[165] != 2 {
		t.Fatal("v5 proof-backed reveal did not settle the order")
	}
	if got := env.GetStorage(&lib.GasMeter{}, []byte("direct/reveal/1")); !bytes.Equal(got, secrets) {
		t.Fatal("v5 proof-backed reveal did not retain the public recovery keys")
	}
	reimbursement := new(big.Int).Mul(env.feePerGas, big.NewInt(2_500_000))
	if reimbursement.Cmp(bond) > 0 {
		reimbursement.Set(bond)
	}
	sellerPayout := new(big.Int).Add(new(big.Int).Add(price, price), reimbursement)
	if env.BalanceOf(author).Cmp(sellerPayout) != 0 ||
		env.BalanceOf(buyer).Cmp(new(big.Int).Sub(bond, reimbursement)) != 0 ||
		new(big.Int).SetBytes(env.GetStorage(&lib.GasMeter{}, []byte("direct/pending"))).Sign() != 0 || env.ContractBalance().Sign() != 0 {
		t.Fatal("v5 proof-backed reveal did not settle the stake, price, and bond")
	}
	restorePending()
	env.SetBlockNumber(buyHeight + directWindowBlocks)
	env.SetCaller(buyer)
	refundArg := u64Arg(1)
	refundGas := call("refund", refundArg)
	if order := env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1")); len(order) != 198 || order[165] != 3 {
		t.Fatal("v5 pending-order refund did not retire the order")
	}
	if env.BalanceOf(buyer).Cmp(new(big.Int).Add(total, price)) != 0 || env.BalanceOf(author).Sign() != 0 ||
		new(big.Int).SetBytes(env.GetStorage(&lib.GasMeter{}, []byte("direct/pending"))).Sign() != 0 || env.ContractBalance().Sign() != 0 {
		t.Fatal("v5 pending-order refund did not return price, bond, and seller stake")
	}
	startTxGas := txGas(startGas, len(startArg))
	t.Logf("v5 deploy: wasm gas=%d wasm bytes=%d conservative tx gas=%d", deployGas, len(code), deployTxGas)
	t.Logf("v5 list: wasm gas=%d payload bytes=%d conservative tx gas=%d", listGas, len(listArg), txGas(listGas, len(listArg)))
	t.Logf("v5 buy: wasm gas=%d payload bytes=%d conservative tx gas=%d", buyGas, len(buyArg), txGas(buyGas, len(buyArg)))
	t.Logf("v5 confirm: wasm gas=%d payload bytes=%d conservative tx gas=%d", confirmGas, len(confirmArg), txGas(confirmGas, len(confirmArg)))
	t.Logf("v5 startFallback: wasm gas=%d exact payload bytes=%d conservative tx gas=%d", startGas, len(startArg), startTxGas)
	revealTxGas := txGas(revealGas, len(revealArg))
	t.Logf("v5 proveAndReveal: wasm gas=%d payload bytes=%d conservative tx gas=%d", revealGas, len(revealArg), revealTxGas)
	t.Logf("v5 refund: wasm gas=%d payload bytes=%d conservative tx gas=%d", refundGas, len(refundArg), txGas(refundGas, len(refundArg)))
	if startTxGas+revealTxGas > 2_500_000 {
		t.Fatalf("v5 fallback start and reveal exceed the contract's 2,500,000-gas reimbursement allowance")
	}
}
