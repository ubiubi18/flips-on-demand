package harness

// D1 feasibility: can the contract authenticate a session header itself, inside
// the real runtime, with no oracle and no consensus change?
//
// `spikes/header-anchor` takes `height || block_hash || state_root ||
// identity_root || seed`, calls the runtime's own `block_header(height)`, parses
// the node's protobuf header and refuses unless every value matches, the block
// is proposed and it carries the FlipLotteryStarted flag.
//
// The fixture is encoded by the pinned node's own protobuf package
// (`spikes/header-anchor/fixtures/headers.json`); the field values are
// synthetic but the wire format and sizes are the node's. A real captured
// header replaces the synthetic one when a capture lands.

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

const headerAnchorWasmPath = "../header-anchor/dist/fod_header_anchor.mvp.wasm"

type headerVectors struct {
	Source         string `json:"source"`
	Proposed       headerVector
	IdentityUpdate headerVector
	Empty          headerVector
}

type headerVector struct {
	Wire         string `json:"wire"`
	Bytes        int    `json:"bytes"`
	Submessage   string `json:"submessage"`
	BlockHash    string `json:"blockHash"`
	Height       uint64 `json:"height"`
	StateRoot    string `json:"stateRoot"`
	IdentityRoot string `json:"identityRoot"`
	Seed         string `json:"seed"`
	Flags        uint32 `json:"flags"`
	IpfsHash     string `json:"ipfsHash"`
}

func loadHeaderVectors(t *testing.T) headerVectors {
	t.Helper()
	raw, err := os.ReadFile("../header-anchor/fixtures/headers.json")
	if err != nil {
		t.Fatalf("read the header vectors: %v", err)
	}
	var wire struct {
		Source                 string       `json:"source"`
		Proposed               headerVector `json:"proposed"`
		ProposedIdentityUpdate headerVector `json:"proposedIdentityUpdate"`
		Empty                  headerVector `json:"empty"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("parse the header vectors: %v", err)
	}
	return headerVectors{
		Source:         wire.Source,
		Proposed:       wire.Proposed,
		IdentityUpdate: wire.ProposedIdentityUpdate,
		Empty:          wire.Empty,
	}
}

func readHeaderAnchorModule(t *testing.T) []byte {
	t.Helper()
	code, err := os.ReadFile(headerAnchorWasmPath)
	if err != nil {
		t.Fatalf("read the header anchor module (build it first, see spikes/README.md): %v", err)
	}
	return code
}

// anchorArg is what the spike's `anchor` method takes.
func anchorArg(t *testing.T, height uint64, blockHash, stateRoot, identityRoot, seed string) []byte {
	t.Helper()
	arg := make([]byte, 0, 136)
	for shift := 56; shift >= 0; shift -= 8 {
		arg = append(arg, byte(height>>uint(shift)))
	}
	for _, field := range []string{blockHash, stateRoot, identityRoot, seed} {
		raw, err := hex.DecodeString(field)
		if err != nil || len(raw) != 32 {
			t.Fatalf("vector %q is not 32 bytes of hex", field)
		}
		arg = append(arg, raw...)
	}
	return arg
}

func anchorCall(t *testing.T, env *MinimalEnv, api *lib.GoAPI, code, arg []byte) uint64 {
	t.Helper()
	gas, _, err := lib.Execute(api, code, "anchor", [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("the anchor call failed in runtime: %v", err)
	}
	return gas
}

func TestHeaderAnchorAcceptsTheChainsOwnHeader(t *testing.T) {
	vectors := loadHeaderVectors(t)
	code := readHeaderAnchorModule(t)
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)

	proposed := mustHex(t, vectors.Proposed.Wire)
	if len(proposed) != vectors.Proposed.Bytes {
		t.Fatalf("the proposed vector is %d bytes, expected %d", len(proposed), vectors.Proposed.Bytes)
	}
	env.SetBlockHeader(vectors.Proposed.Height, proposed)
	arg := anchorArg(t, vectors.Proposed.Height, vectors.Proposed.BlockHash,
		vectors.Proposed.StateRoot, vectors.Proposed.IdentityRoot, vectors.Proposed.Seed)

	gas := anchorCall(t, env, api, code, arg)
	t.Logf("anchor accepted a %d-byte proposed header at height %d: gas=%d",
		len(proposed), vectors.Proposed.Height, gas)
	stored := string(env.GetStorage(&lib.GasMeter{}, []byte("anchor.result")))
	if !strings.Contains(stored, `"ok":true`) || !strings.Contains(stored, `"shape":"proposed"`) {
		t.Fatalf("the anchor's stored verdict is %q", stored)
	}
	if !strings.Contains(stored, `"flags":2`) {
		t.Fatalf("the anchor did not report the lottery flag: %q", stored)
	}

	// Protobuf forward compatibility: an unknown trailing field is skipped
	// rather than refused, so a later node version adding a field still works.
	extended := append(append([]byte{}, proposed...), 0xa0, 0x01, 0x2a)
	env.SetBlockHeader(vectors.Proposed.Height, extended)
	anchorCall(t, env, api, code, arg)
}

func TestHeaderAnchorRefusesAnythingElse(t *testing.T) {
	vectors := loadHeaderVectors(t)
	code := readHeaderAnchorModule(t)
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)

	proposed := mustHex(t, vectors.Proposed.Wire)
	height := vectors.Proposed.Height
	env.SetBlockHeader(height, proposed)
	good := anchorArg(t, height, vectors.Proposed.BlockHash, vectors.Proposed.StateRoot,
		vectors.Proposed.IdentityRoot, vectors.Proposed.Seed)
	anchorCall(t, env, api, code, good)

	other := func(field string) string {
		if field[0] == '0' {
			return "1" + field[1:]
		}
		return "0" + field[1:]
	}

	// A claimed value that the chain's header does not carry is refused.
	escrowReject(t, env, api, code, "anchor", anchorArg(t, height,
		other(vectors.Proposed.BlockHash), vectors.Proposed.StateRoot,
		vectors.Proposed.IdentityRoot, vectors.Proposed.Seed))
	escrowReject(t, env, api, code, "anchor", anchorArg(t, height,
		vectors.Proposed.BlockHash, other(vectors.Proposed.StateRoot),
		vectors.Proposed.IdentityRoot, vectors.Proposed.Seed))
	escrowReject(t, env, api, code, "anchor", anchorArg(t, height,
		vectors.Proposed.BlockHash, vectors.Proposed.StateRoot,
		other(vectors.Proposed.IdentityRoot), vectors.Proposed.Seed))
	escrowReject(t, env, api, code, "anchor", anchorArg(t, height,
		vectors.Proposed.BlockHash, vectors.Proposed.StateRoot,
		vectors.Proposed.IdentityRoot, other(vectors.Proposed.Seed)))
	// A height the node has no header for is refused, not approximated.
	escrowReject(t, env, api, code, "anchor", anchorArg(t, height+1,
		vectors.Proposed.BlockHash, vectors.Proposed.StateRoot,
		vectors.Proposed.IdentityRoot, vectors.Proposed.Seed))

	// A skip block cannot anchor a session, even with matching values.
	empty := mustHex(t, vectors.Empty.Wire)
	env.SetBlockHeader(vectors.Empty.Height, empty)
	escrowReject(t, env, api, code, "anchor", anchorArg(t, vectors.Empty.Height,
		vectors.Empty.BlockHash, vectors.Empty.StateRoot,
		vectors.Empty.IdentityRoot, vectors.Empty.Seed))

	// A proposed block without the FlipLotteryStarted flag is refused.
	env.SetBlockHeader(height, mustHex(t, vectors.IdentityUpdate.Wire))
	escrowReject(t, env, api, code, "anchor", good)

	// Malformed headers are refused rather than partially read: a truncated
	// message, a field longer than the message, and both shapes at once.
	env.SetBlockHeader(height, proposed[:len(proposed)-10])
	escrowReject(t, env, api, code, "anchor", good)
	env.SetBlockHeader(height, []byte{0x0a, 0xff, 0x7f})
	escrowReject(t, env, api, code, "anchor", good)
	env.SetBlockHeader(height, append(append([]byte{}, proposed...), 0x12, 0x02, 0x0a, 0x00))
	escrowReject(t, env, api, code, "anchor", good)
}
