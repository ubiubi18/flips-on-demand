package harness

// Synthetic point-delivery verification in the pinned Idena WASM runtime.
// Build the spike with the same wasm-opt lowering used for the escrow before
// running this focused test; it carries no private or live order data.

import (
	"encoding/hex"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-wasm-binding/lib"
	models "github.com/idena-network/idena-wasm-binding/lib/protobuf"
)

const fastVerifierWasmPath = "../fast-delivery-contract/dist/fod_fast_delivery_verifier.wasm"

// Generated from the fixed, synthetic scalars in the Rust spike's first test.
const fastDeliveryWire = "03023a30e0eb90ce6f6e7e05de6602738f17c4e1460dc0dcd3e1afc360a5c4aa80" +
	"03defdea4cdb677750a420fee807eacf21eb9898ae79b9768766e4faa04a2d4a34" +
	"3373796e746865746963206e6574776f726b2f636f6e74726163742f62756e646c652f6f726465722f6e6f6e63652f6275796572" +
	"022b4ea0a797a443d293ef5cff444f4979f06acfebd7e86d277475656138385b6c" +
	"030429053001eec810b1d22b59a2b999628cb29372e5799f7688236c81fdbc33b0" +
	"02c44d12c7065d812e8acf28d7cbb19f9011ecd9e9fdf281b0e6a3b5e87d22e7db" +
	"020a9578f53e53148ee78bc755cb021642227f953d96fc82221a663fe2709f3f4" +
	"d8d20ba0589171c222446568db559a60569f86c377b0d75cd5d41e5d53a9b4dd" +
	"916a0e12f1f36ec29545525682c5f0c5821ac89d88dcb4cc888929c1c629c96c2"

func TestFastDeliveryProofInIdenaRuntime(t *testing.T) {
	code, err := os.ReadFile(fastVerifierWasmPath)
	if err != nil {
		t.Fatalf("read the lowered verifier spike: %v", err)
	}
	wire, err := hex.DecodeString(fastDeliveryWire)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 314 {
		t.Fatalf("synthetic wire is %d bytes, expected 314", len(wire))
	}
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	if _, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false); err != nil {
		t.Fatalf("deploy the verifier spike: %v", err)
	}
	check := func(input []byte, want string) uint64 {
		t.Helper()
		gas, raw, err := lib.Execute(api, code, "verifyDelivery", [][]byte{input}, lib.Address{}, maxCallGasWasm, false)
		if err != nil {
			t.Fatalf("verify in Idena runtime: %v", err)
		}
		var result models.ActionResult
		if err := proto.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if string(result.OutputData) != want {
			t.Fatalf("verification result %q, expected %q", result.OutputData, want)
		}
		return gas
	}
	validGas := check(wire, "true")
	t.Logf("valid point delivery: wasm gas=%d", validGas)
	wrongOrder := append([]byte(nil), wire...)
	wrongOrder[67] ^= 1
	check(wrongOrder, "false")
	badPoint := append([]byte(nil), wire...)
	for index := 0; index < 33; index++ {
		badPoint[index] = 0
	}
	check(badPoint, "false")
}
