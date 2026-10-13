package harness

import (
	"os"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

// The runtime gatekeeper rejects bulk-memory operators, so the harness loads the
// lowered artifact produced by the documented build recipe in spikes/README.md.
const wasmPath = "../contract-abi/dist/fod_abi_spike.mvp.wasm"

func loadWasm(t *testing.T) []byte {
	t.Helper()
	code, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatalf("read spike module (build it first, see spikes/README.md): %v", err)
	}
	return code
}

// TestDeployAndCallRuntime runs the spike module in the real runtime.
func TestDeployAndCallRuntime(t *testing.T) {
	code := loadWasm(t)
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})

	deployGas, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, 30_000_000, false)
	if err != nil {
		t.Fatalf("deploy rejected by runtime: %v", err)
	}
	t.Logf("deploy accepted: gas=%d", deployGas)

	pingGas, _, err := lib.Execute(api, code, "ping", [][]byte{[]byte("1")}, lib.Address{}, 30_000_000, false)
	if err != nil {
		t.Fatalf("ping call failed: %v", err)
	}
	t.Logf("ping: gas=%d", pingGas)

	var prev uint64
	for _, iterations := range []string{"0", "1000", "10000", "100000"} {
		gas, _, err := lib.Execute(api, code, "compute", [][]byte{[]byte(iterations)}, lib.Address{}, 400_000_000, false)
		if err != nil {
			t.Fatalf("compute(%s) failed: %v", iterations, err)
		}
		stored := env.GetStorage(&lib.GasMeter{}, []byte("spike.result"))
		if len(stored) == 0 {
			t.Fatalf("compute(%s): contract stored no result", iterations)
		}
		t.Logf("compute(%s): gas=%d delta_overhead=%d stored=%s", iterations, gas, gas-prev, string(stored))
		prev = gas
	}
}
