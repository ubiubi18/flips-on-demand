package harness

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/idena-network/idena-wasm-binding/lib"
	models "github.com/idena-network/idena-wasm-binding/lib/protobuf"
)

const fastSessionWasmPath = "../fast-session-contract/dist/fod_fast_session_verifier.wasm"

type fastPathItem struct {
	Height  int64  `json:"height"`
	Size    int64  `json:"size"`
	Version int64  `json:"version"`
	Left    string `json:"leftHex"`
	Right   string `json:"rightHex"`
}
type fastLeaf struct {
	Key     string         `json:"keyHex"`
	Value   string         `json:"valueHex"`
	Version int64          `json:"version"`
	Path    []fastPathItem `json:"path"`
}
type fastAnchor struct {
	Height   uint64   `json:"height"`
	Root     string   `json:"rootHex"`
	Global   fastLeaf `json:"global"`
	Identity fastLeaf `json:"identity"`
}
type fastSession struct {
	Epoch     uint16     `json:"epoch"`
	Inventory fastAnchor `json:"inventory"`
	Verdict   fastAnchor `json:"verdict"`
}
type fastFixture struct {
	Author   string        `json:"author"`
	Manifest []string      `json:"manifest"`
	Sessions []fastSession `json:"sessions"`
}

func fastHex(t *testing.T, value string) []byte {
	t.Helper()
	out, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func fastU16(out *[]byte, value uint16) { *out = binary.BigEndian.AppendUint16(*out, value) }
func fastU64(out *[]byte, value uint64) { *out = binary.BigEndian.AppendUint64(*out, value) }

func fastLeafWire(t *testing.T, out *[]byte, leaf fastLeaf) {
	t.Helper()
	key, value := fastHex(t, leaf.Key), fastHex(t, leaf.Value)
	*out = append(*out, byte(len(key)))
	*out = append(*out, key...)
	fastU16(out, uint16(len(value)))
	*out = append(*out, value...)
	fastU64(out, uint64(leaf.Version))
	*out = append(*out, byte(len(leaf.Path)))
	for _, item := range leaf.Path {
		fastU64(out, uint64(item.Height))
		fastU64(out, uint64(item.Size))
		fastU64(out, uint64(item.Version))
		for _, side := range []string{item.Left, item.Right} {
			bytes := fastHex(t, side)
			*out = append(*out, byte(len(bytes)))
			*out = append(*out, bytes...)
		}
	}
}

func fastAnchorWire(t *testing.T, out *[]byte, anchor fastAnchor) {
	t.Helper()
	fastU64(out, anchor.Height)
	*out = append(*out, fastHex(t, anchor.Root)...)
	fastLeafWire(t, out, anchor.Global)
	fastLeafWire(t, out, anchor.Identity)
}

func fastSyntheticHeader(root []byte) []byte {
	sub := append([]byte{0x32, 0x20}, root...) // Proposed header, state root field 6.
	return append([]byte{0x0a, byte(len(sub))}, sub...)
}

func fastSessionInput(t *testing.T) ([]byte, fastSession) {
	return fastSessionInputFromFile(t, "../../proofs/session-core/fixtures/session-bundle-vector.json")
}

func fastSessionInputFromFile(t *testing.T, path string) ([]byte, fastSession) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture fastFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	session := fixture.Sessions[0]
	author := fastHex(t, fixture.Author)
	cids := append([]string(nil), fixture.Manifest...)
	sort.Strings(cids)
	hash := sha256.New()
	hash.Write([]byte("fod.bundle-manifest:v1"))
	hash.Write([]byte(fixture.Author))
	var epoch [2]byte
	binary.BigEndian.PutUint16(epoch[:], session.Epoch)
	hash.Write(epoch[:])
	for _, cid := range cids {
		hash.Write([]byte{0})
		hash.Write([]byte(cid))
	}
	inventory := hash.Sum(nil)

	wire := []byte{1}
	wire = append(wire, author...)
	fastU16(&wire, session.Epoch)
	wire = append(wire, byte(len(fixture.Manifest)))
	for _, cid := range fixture.Manifest {
		wire = append(wire, byte(len(cid)))
		wire = append(wire, []byte(cid)...)
	}
	fastAnchorWire(t, &wire, session.Inventory)
	fastAnchorWire(t, &wire, session.Verdict)
	input := append([]byte(nil), author...)
	fastU16(&input, session.Epoch)
	fastU16(&input, uint16(len(fixture.Manifest)))
	input = append(input, inventory...)
	input = append(input, wire...)
	return input, session
}

func TestFastSessionCheckInIdenaRuntime(t *testing.T) {
	code, err := os.ReadFile(fastSessionWasmPath)
	if err != nil {
		t.Fatalf("read lowered public-session verifier: %v", err)
	}
	input, session := fastSessionInput(t)
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	if _, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false); err != nil {
		t.Fatalf("deploy verifier spike: %v", err)
	}
	invRoot, verdictRoot := fastHex(t, session.Inventory.Root), fastHex(t, session.Verdict.Root)
	env.SetBlockHeader(session.Inventory.Height, fastSyntheticHeader(invRoot))
	env.SetBlockHeader(session.Verdict.Height, fastSyntheticHeader(verdictRoot))
	check := func(arg []byte, want string) uint64 {
		t.Helper()
		gas, raw, err := lib.Execute(api, code, "verifySession", [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
		if err != nil {
			t.Fatalf("public-session verification: %v", err)
		}
		var result models.ActionResult
		if err := proto.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if string(result.OutputData) != want {
			t.Fatalf("result %q, expected %q", result.OutputData, want)
		}
		return gas
	}
	t.Logf("valid four-leaf session: wasm gas=%d", check(input, "true"))
	tampered := append([]byte(nil), input...)
	tampered[56+25] ^= 1 // First original CID byte in the packed witness.
	check(tampered, "false")
	env.SetBlockHeader(session.Inventory.Height, fastSyntheticHeader(make([]byte, 32)))
	check(input, "false")
}
