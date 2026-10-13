package harness

// Checks that the RISC Zero Groth16 verifier code path builds for the contract
// target and executes inside the real runtime.
//
// This does NOT verify a receipt: the probe feeds an empty seal, so the expected
// result is a rejection. Verifying a real receipt needs a prover, whose
// toolchain is not installed on this machine yet.

import (
	"os"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

const r0WasmPath = "../r0-verifier-probe/dist/fod_r0_verifier_probe.mvp.wasm"

func TestR0VerifierProbeInRuntime(t *testing.T) {
	code, err := os.ReadFile(r0WasmPath)
	if err != nil {
		t.Fatalf("read probe module (build it first, see spikes/README.md): %v", err)
	}
	t.Logf("module: %d bytes", len(code))

	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})

	deployGas, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("deploy rejected: %v", err)
	}
	t.Logf("deploy accepted: gas=%d", deployGas)

	gas, _, err := lib.Execute(api, code, "probe", [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("probe call failed in runtime: %v", err)
	}
	stored := env.GetStorage(&lib.GasMeter{}, []byte("probe.result"))
	t.Logf("probe: gas=%d (%.2fM) stored=%s", gas, float64(gas)/1e6, string(stored))
}
