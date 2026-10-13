package harness

// Cross-check for the deployment command: the address the CLI prints must be
// the address the node derives, and the signed bytes must carry the module.
//
// The derivation is written here the way the node computes it - keccak256 over
// the code hash, the packed argument list, and the deployment nonce - using the
// binding's own argument packing and go-ethereum's keccak, so neither the CLI's
// helper nor idena-go is the only thing that says what the address is.
//
// The artefact is freshly produced offline with a disposable key. Historical
// prepared transactions are never reused or replaced by this test.

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/idena-network/idena-wasm-binding/lib"
)

type preparedDeployArtifact struct {
	Format       string `json:"format"`
	Network      string `json:"network"`
	Code         string `json:"code"`
	CodeBytes    int    `json:"codeBytes"`
	CodeHash     string `json:"codeHash"`
	Address      string `json:"address"`
	Sender       string `json:"sender"`
	DeployNonce  string `json:"deployNonce"`
	AccountNonce uint32 `json:"accountNonce"`
	Epoch        uint16 `json:"epoch"`
	RawTx        string `json:"rawTx"`
	Broadcast    bool   `json:"broadcast"`
}

func TestCliPreparedDeployDerivesTheNodesAddress(t *testing.T) {
	directory := t.TempDir()
	key, err := gethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(directory, "disposable.key")
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(gethcrypto.FromECDSA(key))), 0o600); err != nil {
		t.Fatal(err)
	}
	modulePath, err := filepath.Abs("../../release/artifacts/fod_escrow.wasm")
	if err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(directory, "prepared-deploy.json")
	command := exec.Command("go", "run", ".", "prepare-deploy", "-code", modulePath,
		"-nodekey", keyPath, "-nonce", "1", "-tx-epoch", "1", "-max-fee", "9000000000000000000",
		"-network", "isolated-security-test", "-out", artifactPath)
	command.Dir = "../../tools/fod"
	// The CLI declares a newer Go than the harness. Permit its already cached,
	// checksum-verified toolchain, but refuse module/toolchain downloads.
	command.Env = append(os.Environ(), "GOFLAGS=-modfile=go.local.mod", "GOTOOLCHAIN=auto", "GOPROXY=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("offline CLI preparation: %v\n%s", err, output)
	}
	raw, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	var deploy preparedDeployArtifact
	if err := json.Unmarshal(raw, &deploy); err != nil {
		t.Fatalf("parse the prepared deploy: %v", err)
	}
	if deploy.Format != "fod.prepared-deploy" {
		t.Fatalf("unexpected artefact: %s", deploy.Format)
	}
	if deploy.Broadcast {
		t.Fatal("the artefact says it was broadcast; this check is for a transaction that stopped at the network")
	}

	// The artefact has to be about the module this repository ships.
	code, err := os.ReadFile("../../release/artifacts/fod_escrow.wasm")
	if err != nil {
		t.Fatalf("read the release module: %v", err)
	}
	if deploy.CodeBytes != len(code) {
		t.Fatalf("the artefact names %d module bytes, the release module is %d", deploy.CodeBytes, len(code))
	}
	codeHash := gethcrypto.Keccak256(code)
	if hex.EncodeToString(codeHash) != deploy.CodeHash {
		t.Fatalf("the artefact's code hash is not the release module's")
	}

	nonce, err := hex.DecodeString(deploy.DeployNonce)
	if err != nil {
		t.Fatalf("the deployment nonce is not hex: %v", err)
	}
	if len(nonce) == 0 {
		t.Fatal("the artefact carries no deployment nonce")
	}

	// `vm/wasm/util.go`: keccak256(codeHash || packedArgs || nonce), last 20 bytes.
	packed := lib.PackArguments(nil)
	buf := append(append(append([]byte{}, codeHash...), packed...), nonce...)
	derived := gethcrypto.Keccak256(buf)[12:]
	if hex.EncodeToString(derived) != strings.ToLower(strings.TrimPrefix(deploy.Address, "0x")) {
		t.Fatalf("the CLI printed address %s, the node's derivation gives %x", deploy.Address, derived)
	}

	// The signed bytes are the module's, not a summary of it: the protobuf
	// attachment carries the code verbatim.
	rawTx, err := hex.DecodeString(deploy.RawTx)
	if err != nil {
		t.Fatalf("the raw transaction is not hex: %v", err)
	}
	if len(rawTx) <= len(code) {
		t.Fatalf("the raw transaction is %d bytes, which cannot carry a %d-byte module", len(rawTx), len(code))
	}
	if !strings.Contains(strings.ToLower(deploy.RawTx), hex.EncodeToString(code)) {
		t.Fatal("the signed transaction does not carry the module bytes")
	}
	if !strings.Contains(strings.ToLower(deploy.RawTx), deploy.DeployNonce) {
		t.Fatal("the signed transaction does not carry the deployment nonce")
	}
}
