package harness

// The registry has no owner, and this pins the module half of that claim.
//
// The chain half is enforced elsewhere: `validateTerminateContractTx` refuses to
// terminate anything outside the embedded contract set once upgrade 11 is on
// (`blockchain/validation/validation.go`), which it has been since March 2023
// (`SOURCES.md` F-78). So even a module that exported a terminate path could not
// be terminated - and this module exports none, nor any hook that only a test
// build should carry.
//
// The export surface is compared by name against the published list, not against
// a deny-list, so an administrative hook added later under a new name fails here
// as well.

import (
	"os"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

// publishedExports is the complete surface `SPEC.md` 7.1 publishes: the entry
// points, the read views, the allocator the runtime calls to place arguments,
// and the memory it hands to the module. Kind 0 is a function, kind 2 a memory.
var publishedExports = map[string]byte{
	"memory":   2,
	"allocate": 0,
	"deploy":   0,
	// The pool of SPEC 5.1 and 7.1: the supply side, the demand side, and the
	// views. Nothing else may be exported.
	"registerBundle": 0,
	"addReserve":     0,
	// The reserve's donor-owned exit: a contribution can be withdrawn by the
	// address that made it (`AH-15`).
	"withdrawReserve": 0,
	"donateUnit":      0,
	"sellUnit":        0,
	"beacon":          0,
	"poolView":        0,
	"buy":             0,
	"deliver":         0,
	"confirm":         0,
	"fulfill":         0,
	"refund":          0,
	// The approved settlement without the buyer's confirmation (`SPEC.md` 7.2):
	// publishing the committed secrets pays the registered author and the named
	// hosts and closes the order.
	"reveal":        0,
	"orderPoolView": 0,
	"withdraw":      0,
	"accounting":    0,
}

func TestReleaseModuleExportsNoAdministrativeEntryPoint(t *testing.T) {
	code, err := os.ReadFile("../../release/artifacts/fod_escrow.wasm")
	if err != nil {
		t.Fatalf("read the committed release module: %v", err)
	}
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	if _, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false); err != nil {
		t.Fatalf("deploy rejected: %v", err)
	}

	// A real method has to run, so the refusals below are not passing by
	// accident: this one is exported and executes.
	if _, _, err := lib.Execute(api, code, "accounting", [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false); err != nil {
		t.Fatalf("accounting failed in the runtime: %v", err)
	}

	// No terminate path, and none of the test-hook entries may ship.
	for _, method := range []string{
		"terminate",
		"seedPoolBundle",
		"seedPoolOrder",
	} {
		if _, _, err := lib.Execute(api, code, method, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false); err == nil {
			t.Fatalf("the release module exports %s", method)
		}
	}

	exports := wasmExports(t, code)
	for name, kind := range publishedExports {
		actual, ok := exports[name]
		switch {
		case !ok:
			t.Errorf("the release module does not export %s", name)
		case actual != kind:
			t.Errorf("export %s has kind %d, expected %d", name, actual, kind)
		}
	}
	for name := range exports {
		if _, ok := publishedExports[name]; !ok {
			t.Errorf("the release module exports %s, which is not on the published surface", name)
		}
	}
}

// wasmExports reads the export section of a module, so the check above can fail
// on any unexpected entry point rather than only on the names it already knows.
func wasmExports(t *testing.T, code []byte) map[string]byte {
	t.Helper()
	reader := &wasmReader{t: t, code: code}
	if string(reader.take(4)) != "\x00asm" {
		t.Fatalf("the artefact is not a wasm module")
	}
	reader.take(4) // version
	exports := map[string]byte{}
	for reader.left() > 0 {
		id := reader.take(1)[0]
		section := reader.take(int(reader.uvarint()))
		if id != exportSectionID {
			continue
		}
		body := &wasmReader{t: t, code: section}
		for count := body.uvarint(); count > 0; count-- {
			name := string(body.take(int(body.uvarint())))
			kind := body.take(1)[0]
			body.uvarint() // index into the function, table, memory, or global space
			exports[name] = kind
		}
	}
	if len(exports) == 0 {
		t.Fatalf("the module has no export section")
	}
	return exports
}

const exportSectionID = 7

type wasmReader struct {
	t    *testing.T
	code []byte
	at   int
}

func (r *wasmReader) left() int {
	return len(r.code) - r.at
}

func (r *wasmReader) take(n int) []byte {
	r.t.Helper()
	if n < 0 || r.left() < n {
		r.t.Fatalf("truncated wasm module at byte %d", r.at)
	}
	out := r.code[r.at : r.at+n]
	r.at += n
	return out
}

func (r *wasmReader) uvarint() uint64 {
	r.t.Helper()
	var value uint64
	for shift := uint(0); ; shift += 7 {
		if shift > 63 {
			r.t.Fatalf("oversized varint at byte %d", r.at)
		}
		b := r.take(1)[0]
		value |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return value
		}
	}
}
