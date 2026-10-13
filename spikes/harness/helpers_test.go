package harness

// Shared harness helpers: the module under test, the realistic amounts the
// tests use, and the two call wrappers every escrow test runs through.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"os"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

const escrowWasmPath = "../../contracts/escrow/dist/fod_escrow.testhooks.wasm"

// sha256Arg is the digest the escrow binds a ciphertext by: the delivery
// journal carries it, and a buyer's confirmation names it
// (`contracts/escrow/src/lib.rs`, `confirm`).
func sha256Arg(payload []byte) []byte {
	sum := sha256.Sum256(payload)
	return sum[:]
}

// registrationArtifact is the sealed receipt proofs/registration/host writes;
// the pool's supply test re-parses it.
type registrationArtifact struct {
	Statement    string  `json:"statement"`
	Journal      string  `json:"journal"`
	Seal         string  `json:"seal"`
	SealBytes    int     `json:"sealBytes"`
	TotalCycles  uint64  `json:"totalCycles"`
	ProveSeconds float64 `json:"proveSeconds"`
}

// preparedDeliveryArtifact is the CLI's `fod.prepared-delivery` output; the
// pool's proof path re-parses it to check the seal against a pool order.
type preparedDeliveryArtifact struct {
	Format      string `json:"format"`
	Operation   string `json:"operation"`
	Network     string `json:"network"`
	Contract    string `json:"contract"`
	OfferID     uint64 `json:"offerId"`
	OrderNonce  uint64 `json:"orderNonce"`
	Seller      string `json:"seller"`
	Buyer       string `json:"buyer"`
	DeliveryKey string `json:"deliveryKey"`
	Amount      string `json:"amount"`
	Calldata    string `json:"calldata"`
	Proof       string `json:"proof"`
	Submitted   bool   `json:"submitted"`
}

// Prices in these tests are realistic, not toy values: 100 iDNA is 1e20 base
// units, which does not fit in 64 bits. The escrow stores amounts as 128-bit
// integers for exactly that reason (SPEC.md O-11).
const (
	testPrice       = "100000000000000000000"
	testHalfPrice   = "50000000000000000000"
	testDoublePrice = "200000000000000000000"
)

func mustAmount(value string) *big.Int {
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		panic("bad test amount: " + value)
	}
	return amount
}

var (
	testPriceValue  = mustAmount(testPrice)
	testHalfValue   = mustAmount(testHalfPrice)
	testDoubleValue = mustAmount(testDoublePrice)
	// testFeeValue is one iDNA: the retrieval-fee cap per covered item.
	testFeeValue = mustAmount("1000000000000000000")
)

func escrowCall(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte, method, arg string) (uint64, string) {
	t.Helper()
	defer env.ResetPayAmount()
	gas, _, err := lib.Execute(api, code, method, [][]byte{[]byte(arg)}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("%s(%s) failed in runtime: %v", method, arg, err)
	}
	// The test-hook build mirrors each reply into storage key "spike-last".
	stored := string(env.GetStorage(&lib.GasMeter{}, []byte("spike-last")))
	return gas, stored
}

// escrowCallBytes is escrowCall for methods whose argument is a byte encoding
// rather than a decimal id.
func escrowCallBytes(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte, method string, arg []byte) (uint64, string) {
	t.Helper()
	defer env.ResetPayAmount()
	gas, _, err := lib.Execute(api, code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("%s failed in runtime: %v", method, err)
	}
	stored := string(env.GetStorage(&lib.GasMeter{}, []byte("spike-last")))
	return gas, stored
}

// readEscrowModule reads the test-hooks build the harness exercises.
func readEscrowModule(t *testing.T) []byte {
	t.Helper()
	code, err := os.ReadFile(escrowWasmPath)
	if err != nil {
		t.Fatalf("read escrow module: %v", err)
	}
	return code
}

// escrowReject runs a call that must fail.
//
// A refused mutation has to be a failed call: the node commits a call's storage
// and value only when it succeeds, so a JSON `{"ok":false}` reply is a
// successful call that keeps whatever the caller sent (`REVIEW.md` A04). The
// helper also requires the modelled state to be exactly what the call found, so
// a write that happens before the failure cannot hide behind the node's
// rollback.
func escrowReject(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte, method string, arg []byte) uint64 {
	t.Helper()
	defer env.ResetPayAmount()
	before := env.StateDigest()
	gas, _, err := lib.Execute(api, code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	if err == nil {
		t.Fatalf("%s did not fail: a refused mutation must abort the call", method)
	}
	if after := env.StateDigest(); after != before {
		t.Fatalf("%s wrote state before it failed", method)
	}
	return gas
}

// escrowRejectCall is escrowReject for the methods whose argument is text.
func escrowRejectCall(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte, method, arg string) uint64 {
	t.Helper()
	return escrowReject(t, env, api, code, method, []byte(arg))
}

// escrowRejectPayable models a payable call that must fail: the host moves the
// carried value into the contract, the call aborts, and the node keeps nothing.
// The comparison runs with the value already credited, so a partial write is
// visible before the model rolls the transfer back.
func escrowRejectPayable(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code []byte, method string, arg []byte, amount *big.Int) uint64 {
	t.Helper()
	defer env.ResetPayAmount()
	restore := env.Snapshot()
	before := env.StateDigest()
	env.SetPayAmount(amount)
	env.CreditContractBalance(amount)
	withValue := env.StateDigest()
	gas, _, err := lib.Execute(api, code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	if err == nil {
		restore()
		t.Fatalf("%s accepted a value it must refuse", method)
	}
	if after := env.StateDigest(); after != withValue {
		restore()
		t.Fatalf("%s wrote state before it failed", method)
	}
	restore()
	if after := env.StateDigest(); after != before {
		t.Fatalf("%s left the transfer behind after the rollback", method)
	}
	return gas
}

func addressFromHex(t *testing.T, value string) lib.Address {
	t.Helper()
	raw, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("address hex: %v", err)
	}
	if len(raw) != 20 {
		t.Fatalf("address is %d bytes", len(raw))
	}
	var addr lib.Address
	copy(addr[:], raw)
	return addr
}

func mustHex(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("hex %q: %v", value, err)
	}
	return raw
}

func TestMinimalEnvPreservesLongStorageKeysAndChargesNodeCosts(t *testing.T) {
	env := NewMinimalEnv()
	first := append([]byte("pool/inventory/"), bytes.Repeat([]byte{0x42}, 32)...)
	second := append([]byte(nil), first...)
	second[len(second)-1] = 0x43
	meter := &lib.GasMeter{}
	env.SetStorage(meter, first, []byte{1})
	env.SetStorage(meter, second, []byte{2})
	if got := meter.GasConsumed(); got != uint64(2*costsWriteStatePerByteGas*(len(first)+1)*wasmGasMultiplier) {
		t.Fatal("storage writes were not charged at the pinned node rate")
	}
	if !bytes.Equal(env.GetStorage(meter, first), []byte{1}) || !bytes.Equal(env.GetStorage(meter, second), []byte{2}) {
		t.Fatal("distinct long contract keys collided")
	}
	env.RemoveStorage(meter, first)
	if len(env.GetStorage(meter, first)) != 0 || !bytes.Equal(env.GetStorage(meter, second), []byte{2}) {
		t.Fatal("removing one long key changed another")
	}
	wantGas := uint64((2*costsWriteStatePerByteGas*(len(first)+1) + 3*costsReadStatePerByteGas + costsRemoveStateGas) * wasmGasMultiplier)
	if meter.GasConsumed() != wantGas {
		t.Fatal("storage reads or removal were not charged at the pinned node rate")
	}
}
