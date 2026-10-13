package harness

// Verifies a real RISC Zero Groth16 receipt inside the contract runtime.
//
// The receipt was produced by spikes/r0-receipt with dev mode off, so it is a
// genuine Groth16 seal. This test proves the verification path works in this
// runtime and that the check rejects a tampered claim digest.

import (
	"os"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

const r0ReceiptWasmPath = "../r0-receipt-verify/dist/fod_r0_receipt_verify.mvp.wasm"

func TestR0ReceiptVerificationInRuntime(t *testing.T) {
	code, err := os.ReadFile(r0ReceiptWasmPath)
	if err != nil {
		t.Fatalf("read receipt verifier module (build it first, see spikes/README.md): %v", err)
	}
	t.Logf("module: %d bytes", len(code))

	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})

	deployGas, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("deploy rejected: %v", err)
	}
	t.Logf("deploy accepted: gas=%d", deployGas)

	for _, mode := range []string{"0", "1"} {
		gas, _, err := lib.Execute(api, code, "verify", [][]byte{[]byte(mode)}, lib.Address{}, maxCallGasWasm, false)
		if err != nil {
			t.Fatalf("verify(%s) failed in runtime: %v", mode, err)
		}
		stored := env.GetStorage(&lib.GasMeter{}, []byte("receipt.result"))
		t.Logf("verify(%s): gas=%d (%.2fM) stored=%s", mode, gas, float64(gas)/1e6, string(stored))

		if mode == "0" {
			if want := `"verified":true`; !contains(string(stored), want) {
				t.Fatalf("real receipt was not verified; stored=%s", string(stored))
			}
		} else {
			if want := `"verified":false`; !contains(string(stored), want) {
				t.Fatalf("tampered claim digest was accepted; stored=%s", string(stored))
			}
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
