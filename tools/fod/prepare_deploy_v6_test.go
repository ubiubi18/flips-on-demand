package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/vm/wasm"
)

func TestDirectV6DeployPinsConstructorAndZeroValue(t *testing.T) {
	keyPath, _ := writeTestKey(t, t.TempDir(), "nodekey")
	key, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	code := deployModule()
	first, second := [32]byte{1}, [32]byte{2}
	seedIDs, err := EncodeDirectV6PoolDeploySeedIDs(first, second)
	if err != nil {
		t.Fatal(err)
	}
	build := func(arg []byte, amount *big.Int) ([]byte, common.Address, error) {
		raw, _, addr, err := BuildDirectV6PoolDeployTransaction(code, deployNonceFixture(), arg, key,
			7, 230, amount, big.NewInt(1_000_000), big.NewInt(0))
		return raw, addr, err
	}
	raw, address, err := build(seedIDs, big.NewInt(0))
	if err != nil {
		t.Fatal(err)
	}
	var tx types.Transaction
	if err := tx.FromBytes(raw); err != nil {
		t.Fatal(err)
	}
	parsed := attachments.ParseDeployContractAttachment(&tx)
	if parsed == nil || len(parsed.Args) != 1 || !bytes.Equal(parsed.Args[0], seedIDs) {
		t.Fatal("the signed v6 deploy lost its constructor seed IDs")
	}
	if address != wasm.ComputeContractAddrWithUnpackedArgs(code, parsed.Args, deployNonceFixture()) {
		t.Fatal("the v6 address disagrees with the node's argument-aware derivation")
	}
	reversed, err := EncodeDirectV6PoolDeploySeedIDs(second, first)
	if err != nil {
		t.Fatal(err)
	}
	_, otherAddress, err := build(reversed, big.NewInt(0))
	if err != nil || otherAddress == address {
		t.Fatal("changing the seed order did not change the signed deployment address")
	}
	for _, invalid := range [][]byte{nil, seedIDs[:63], bytes.Repeat([]byte{0}, 64), append(first[:], first[:]...)} {
		if _, _, err := build(invalid, big.NewInt(0)); err == nil {
			t.Fatal("invalid v6 seed IDs were accepted")
		}
	}
	if _, _, err := build(seedIDs, big.NewInt(1)); err == nil {
		t.Fatal("a funded v6 deploy was accepted")
	}
}

func TestDirectV6DeployNeedsPinnedCandidateManifest(t *testing.T) {
	code, err := os.ReadFile("../../release/artifacts/fod_direct_escrow_v6.wasm")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile("../../release/DIRECT_V6_MANIFEST.json")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	codePath := filepath.Join(root, "release", "artifacts", "fod_direct_escrow_v6.wasm")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := deployGasForCode(codePath, code); err == nil {
		t.Fatal("v6 deployment accepted without the pinned manifest")
	}
	manifestPath := filepath.Join(root, "release", "DIRECT_V6_MANIFEST.json")
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	gas, err := deployGasForCode(codePath, code)
	if err != nil || gas.WasmGas == 0 {
		t.Fatalf("pinned v6 deployment refused: gas=%d, error=%v", gas.WasmGas, err)
	}
	for _, edit := range []func(map[string]any){
		func(m map[string]any) { m["artifact"].(map[string]any)["codeHash"] = "wrong" },
		func(m map[string]any) { m["checks"].(map[string]any)["fastContentImageId"] = "wrong" },
		func(m map[string]any) {
			deploy := m["checks"].(map[string]any)["deploy"].(map[string]any)
			deploy["wasmGas"] = float64(1)
			deploy["conservativeTransactionGas"] = float64(4_517_761)
		},
		func(m map[string]any) { m["checks"].(map[string]any)["conservativeFallbackGas"] = float64(2_500_001) },
		func(m map[string]any) {
			m["checks"].(map[string]any)["activate"].(map[string]any)["payloadBytes"] = float64(0)
		},
	} {
		var changed map[string]any
		if err := json.Unmarshal(manifest, &changed); err != nil {
			t.Fatal(err)
		}
		edit(changed)
		encoded, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := deployGasForCode(codePath, code); err == nil {
			t.Fatal("modified v6 candidate manifest reused its deployment measurement")
		}
	}
}

func TestDirectV6PrepareDeploySignsPinnedSeedsWithoutBroadcast(t *testing.T) {
	code, err := os.ReadFile("../../release/artifacts/fod_direct_escrow_v6.wasm")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile("../../release/DIRECT_V6_MANIFEST.json")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	codePath := filepath.Join(root, "release", "artifacts", "fod_direct_escrow_v6.wasm")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codePath, code, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release", "DIRECT_V6_MANIFEST.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	keyPath, _ := writeTestKey(t, root, "test-nodekey")
	first, second := [32]byte{1}, [32]byte{2}
	seedIDs, err := EncodeDirectV6PoolDeploySeedIDs(first, second)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "prepared.json")
	runPrepareDeploy(codePath, keyPath, hex.EncodeToString(deployNonceFixture()), hex.EncodeToString(seedIDs),
		"test", out, 7, 230, big.NewInt(0), big.NewInt(1_000_000_000_000_000_000), big.NewInt(0),
		false, "", "", nil)
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var prepared PreparedDeploy
	if err := json.Unmarshal(raw, &prepared); err != nil || prepared.Broadcast || prepared.Amount != "0" ||
		prepared.CodeHash != directV6CodeHashHex {
		t.Fatalf("unsafe or invalid v6 prepared deploy: %v", err)
	}
	txBytes, err := hex.DecodeString(prepared.RawTx)
	if err != nil {
		t.Fatal(err)
	}
	var tx types.Transaction
	if err := tx.FromBytes(txBytes); err != nil {
		t.Fatal(err)
	}
	parsed := attachments.ParseDeployContractAttachment(&tx)
	if parsed == nil || len(parsed.Args) != 1 || !bytes.Equal(parsed.Args[0], seedIDs) ||
		prepared.Address != hex.EncodeToString(wasm.ComputeContractAddrWithUnpackedArgs(code, parsed.Args, deployNonceFixture()).Bytes()) {
		t.Fatal("prepared v6 deployment differs from its signed bytes or pinned seed IDs")
	}
}
