package harness

// What deploying the *release* module costs, against the runtime the contract is
// verified in.
//
// The node charges a transaction ten gas per byte and refuses a max fee whose
// whole value implies more than the block's gas limit (`SOURCES.md` F-76). For
// the release module that leaves a narrow window: the signed deploy transaction
// is 357,858 bytes, so its size fee is 3,578,580 gas of the 5,120,000 allowed,
// and the deploy's own execution has to fit in the remaining 1,541,420 gas
// (before the x100 wasm scaling). A module that needs more can never be
// deployed, on any chain, at any fee rate.

import (
	"os"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

// deployTxBytes is the signed deploy transaction the CLI produced for the
// release artefact (`docs/PROGRESS.md`, the deploy fare and the submission
// guard entries). RLP encodes the values it carries, so the CLI checks the
// signed size at submission rather than trusting this constant.
const deployTxBytes = 357_860

// maxUserGasUpgrade11 is the block's gas limit with upgrade 11 (`SOURCES.md`
// F-15).
const maxUserGasUpgrade11 = 5_120_000

// wasmGasPerUserGas is the runtime's scaling factor (`SOURCES.md` F-8).
const wasmGasPerUserGas = 100

func TestReleaseModuleFitsTheDeployGasHeadroom(t *testing.T) {
	code, err := os.ReadFile("../../release/artifacts/fod_escrow.wasm")
	if err != nil {
		t.Fatalf("read the committed release module: %v", err)
	}
	headroomUserGas := uint64(maxUserGasUpgrade11 - deployTxBytes*10)
	headroomWasmGas := headroomUserGas * wasmGasPerUserGas

	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	gas, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("the release module was rejected by the runtime: %v", err)
	}
	t.Logf("deploy used %d wasm gas (%d user gas) of %d user gas of headroom",
		gas, gas/wasmGasPerUserGas, headroomUserGas)
	if gas > headroomWasmGas {
		t.Fatalf("the deploy needs %d wasm gas, more than the %d the node allows for a %d-byte transaction",
			gas, headroomWasmGas, deployTxBytes)
	}
}
