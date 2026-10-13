package harness

// Measures what arkworks BN254 work costs inside the real contract runtime.
// The runtime, gatekeeper, and gas metering are real; the host environment is
// stubbed. Nothing here verifies a proof - see spikes/groth16-verify/src/lib.rs.

import (
	"os"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

const groth16WasmPath = "../groth16-verify/dist/fod_groth16_spike.mvp.wasm"

// maxCallGasWasm is the per-transaction ceiling derived from the block gas
// constant, expressed in wasm gas (100x user gas). See docs/SOURCES.md F-15.
const maxCallGasWasm = 512_000_000

func TestGroth16SpikeInRuntime(t *testing.T) {
	code, err := os.ReadFile(groth16WasmPath)
	if err != nil {
		t.Fatalf("read spike module (build it first, see spikes/README.md): %v", err)
	}
	t.Logf("module: %d bytes", len(code))

	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})

	deployGas, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("deploy rejected: %v", err)
	}
	t.Logf("deploy accepted: gas=%d", deployGas)

	cases := []struct {
		method string
		arg    string
	}{
		{"field", "1000"},
		{"field", "10000"},
		{"field", "100000"},
		{"mul", "1"},
		{"mul", "100"},
		{"pairing", "1"},
		{"pairing", "2"},
		{"verify", "0"},
		{"verify", "1"},
	}

	for _, c := range cases {
		gas, _, err := lib.Execute(api, code, c.method, [][]byte{[]byte(c.arg)}, lib.Address{}, maxCallGasWasm, false)
		if err != nil {
			t.Logf("%s(%s): FAILED in runtime: %v", c.method, c.arg, err)
			continue
		}
		stored := env.GetStorage(&lib.GasMeter{}, []byte("spike.result"))
		t.Logf("%s(%s): gas=%d (%.2fM) stored=%s", c.method, c.arg, gas, float64(gas)/1e6, string(stored))
	}
}
