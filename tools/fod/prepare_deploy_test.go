package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
)

func TestFastCandidateDeployRequiresMatchingGasManifest(t *testing.T) {
	root := t.TempDir()
	codePath := filepath.Join(root, "release", "artifacts", "fod_fast_escrow.wasm")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o700); err != nil {
		t.Fatal(err)
	}
	code := deployModule()
	if _, err := deployGasForCode(codePath, code); err == nil {
		t.Fatal("a missing fast candidate measurement was accepted")
	}
	hash := sha256.Sum256(code)
	manifest := fmt.Sprintf(`{"format":"fod.fast-candidate-manifest","version":1,"artifact":{"path":"release/artifacts/fod_fast_escrow.wasm","bytes":%d,"sha256":"%s"},"checks":{"deploy":{"wasmGas":123456}}}`,
		len(code), hex.EncodeToString(hash[:]))
	if err := os.WriteFile(filepath.Join(root, "release", "FAST_MANIFEST.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	gas, err := deployGasForCode(codePath, code)
	if err != nil || gas.WasmGas != 123456 {
		t.Fatalf("matching fast candidate gas was refused: gas=%d, error=%v", gas.WasmGas, err)
	}
	alias := filepath.Join(filepath.Dir(codePath), "renamed.wasm")
	if gas, err := deployGasForCode(alias, code); err != nil || gas.WasmGas != 123456 {
		t.Fatalf("renamed measured bytes lost their gas measurement: gas=%d, error=%v", gas.WasmGas, err)
	}
	changed := append([]byte(nil), code...)
	changed[len(changed)-1] ^= 1
	if _, err := deployGasForCode(codePath, changed); err == nil {
		t.Fatal("a code change reused the old deploy gas measurement")
	}
	if _, err := deployGasForCode(alias, changed); err == nil {
		t.Fatal("renaming arbitrary bytes reused the measured gas")
	}
}

func TestDirectCandidateDeployRequiresMatchingGasManifest(t *testing.T) {
	root := t.TempDir()
	codePath := filepath.Join(root, "release", "artifacts", "fod_direct_escrow.wasm")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o700); err != nil {
		t.Fatal(err)
	}
	code := deployModule()
	if _, err := deployGasForCode(codePath, code); err == nil {
		t.Fatal("missing direct candidate measurement was accepted")
	}
	hash := sha256.Sum256(code)
	manifest := fmt.Sprintf(`{"format":"fod.direct-candidate-manifest","version":2,"artifact":{"path":"release/artifacts/fod_direct_escrow.wasm","bytes":%d,"sha256":"%s"},"checks":{"deploy":{"wasmGas":3000058},"registrationImageId":"%s","buyerBond":{"gasBudget":2500000,"feeMargin":2},"conservativeFallbackGas":1966726}}`,
		len(code), hex.EncodeToString(hash[:]), directV2ImageIDHex)
	if err := os.WriteFile(filepath.Join(root, "release", "DIRECT_MANIFEST.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if gas, err := deployGasForCode(codePath, code); err != nil || gas.WasmGas != 3000058 {
		t.Fatalf("matching direct candidate gas was refused: gas=%d, error=%v", gas.WasmGas, err)
	}
	code[len(code)-1] ^= 1
	if _, err := deployGasForCode(codePath, code); err == nil {
		t.Fatal("changed direct candidate reused its deploy measurement")
	}
}

func TestDirectReleaseArtifactMatchesMeasuredManifest(t *testing.T) {
	codePath := "../../release/artifacts/fod_direct_escrow.wasm"
	code, err := os.ReadFile(codePath)
	if err != nil {
		t.Fatal(err)
	}
	gas, err := deployGasForCode(codePath, code)
	if err != nil || gas.WasmGas != 3000058 {
		t.Fatalf("direct release artifact lost its runtime measurement: gas=%d, error=%v", gas.WasmGas, err)
	}
}

func TestDirectV3DeployRequiresMeasuredProofAndReveal(t *testing.T) {
	root := t.TempDir()
	codePath := filepath.Join(root, "release", "artifacts", "fod_direct_escrow_v3.wasm")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o700); err != nil {
		t.Fatal(err)
	}
	code := deployModule()
	hash := sha256.Sum256(code)
	manifestPath := filepath.Join(root, "release", "DIRECT_V3_MANIFEST.json")
	manifest := fmt.Sprintf(`{"format":"fod.direct-candidate-manifest","version":3,"artifact":{"path":"release/artifacts/fod_direct_escrow_v3.wasm","bytes":%d,"sha256":"%s"},"checks":{"deploy":{"wasmGas":3000058},"registrationImageId":"%s","buyerBond":{"gasBudget":2500000,"feeMargin":2,"reimbursementGasBudget":2000000,"sellerIntentGasBudget":100000},"bondedSellerIntent":{"wasmGas":100000,"conservativeTransactionGas":6200},"syntheticSealedFallbackStart":{"wasmGas":180000000,"conservativeTransactionGas":1850000,"genuineRegistrationV2":true},"publicKeyReveal":{"wasmGas":10000000,"conservativeTransactionGas":110000},"conservativeFallbackGas":1960000}}`,
		len(code), hex.EncodeToString(hash[:]), directV2ImageIDHex)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if gas, err := deployGasForCode(codePath, code); err != nil || gas.WasmGas != 3000058 {
		t.Fatalf("matching direct v3 candidate was refused: gas=%d, error=%v", gas.WasmGas, err)
	}
	for name, changed := range map[string]string{
		"unsealed start":            strings.Replace(manifest, `"genuineRegistrationV2":true`, `"genuineRegistrationV2":false`, 1),
		"underfunded reimbursement": strings.Replace(manifest, `"reimbursementGasBudget":2000000`, `"reimbursementGasBudget":1900000`, 1),
		"missing seller intent":     strings.Replace(manifest, `"sellerIntentGasBudget":100000`, `"sellerIntentGasBudget":0`, 1),
		"missing reveal gas":        strings.Replace(manifest, `"wasmGas":10000000`, `"wasmGas":0`, 1),
	} {
		if err := os.WriteFile(manifestPath, []byte(changed), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := deployGasForCode(codePath, code); err == nil {
			t.Errorf("%s reused the deploy measurement", name)
		}
	}
}

func TestDirectV4DeployRequiresMeasuredPayableProofAndReveal(t *testing.T) {
	root := t.TempDir()
	codePath := filepath.Join(root, "release", "artifacts", "fod_direct_escrow_v4.wasm")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o700); err != nil {
		t.Fatal(err)
	}
	code := deployModule()
	hash := sha256.Sum256(code)
	manifestPath := filepath.Join(root, "release", "DIRECT_V4_MANIFEST.json")
	manifest := fmt.Sprintf(`{"format":"fod.direct-candidate-manifest","version":4,"artifact":{"path":"release/artifacts/fod_direct_escrow_v4.wasm","bytes":%d,"sha256":"%s"},"checks":{"deploy":{"wasmGas":3000058},"registrationImageId":"%s","buyerBond":{"gasBudget":2500000,"feeMargin":2,"reimbursementGasBudget":2000000,"sellerStakeGasBudget":100000},"syntheticSealedFallbackStart":{"wasmGas":180000000,"conservativeTransactionGas":1850000,"genuineRegistrationV2":true},"publicKeyReveal":{"wasmGas":10000000,"conservativeTransactionGas":110000},"conservativeFallbackGas":1960000}}`,
		len(code), hex.EncodeToString(hash[:]), directV2ImageIDHex)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if gas, err := deployGasForCode(codePath, code); err != nil || gas.WasmGas != 3000058 {
		t.Fatalf("matching v4 candidate refused: gas=%d, error=%v", gas.WasmGas, err)
	}
	for name, changed := range map[string]string{
		"missing seller stake floor": strings.Replace(manifest, `"sellerStakeGasBudget":100000`, `"sellerStakeGasBudget":0`, 1),
		"unsealed start":             strings.Replace(manifest, `"genuineRegistrationV2":true`, `"genuineRegistrationV2":false`, 1),
		"missing reveal gas":         strings.Replace(manifest, `"wasmGas":10000000`, `"wasmGas":0`, 1),
		"legacy intent":              strings.Replace(manifest, `"syntheticSealedFallbackStart"`, `"bondedSellerIntent":{"wasmGas":1},"syntheticSealedFallbackStart"`, 1),
	} {
		if err := os.WriteFile(manifestPath, []byte(changed), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := deployGasForCode(codePath, code); err == nil {
			t.Errorf("%s reused the v4 measurement", name)
		}
	}
}

func TestDirectV5DeployRequiresMeasuredFastContentFallback(t *testing.T) {
	root := t.TempDir()
	codePath := filepath.Join(root, "release", "artifacts", "fod_direct_escrow_v5.wasm")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o700); err != nil {
		t.Fatal(err)
	}
	code, err := os.ReadFile("../../release/artifacts/fod_direct_escrow_v5.wasm")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile("../../release/DIRECT_V5_MANIFEST.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deployGasForCode(codePath, code); err == nil {
		t.Fatal("v5 deploy accepted without a release manifest")
	}
	manifestPath := filepath.Join(root, "release", "DIRECT_V5_MANIFEST.json")
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	gas, err := deployGasForCode(codePath, code)
	if err != nil || gas.WasmGas != 3_000_058 {
		t.Fatalf("matching v5 manifest refused: gas=%d, error=%v", gas.WasmGas, err)
	}
	tests := []struct {
		name string
		edit func(map[string]any)
	}{
		{"wrong guest image", func(m map[string]any) { m["checks"].(map[string]any)["fastContentImageId"] = "wrong" }},
		{"underfunded proof", func(m map[string]any) {
			m["checks"].(map[string]any)["buyerBond"].(map[string]any)["reimbursementGasBudget"] = 1_900_000
		}},
		{"unmeasured fallback", func(m map[string]any) { m["checks"].(map[string]any)["startFallback"].(map[string]any)["wasmGas"] = 0 }},
		{"unmeasured confirm", func(m map[string]any) { m["checks"].(map[string]any)["confirm"].(map[string]any)["wasmGas"] = 0 }},
		{"unmeasured reveal", func(m map[string]any) { m["checks"].(map[string]any)["proveAndReveal"].(map[string]any)["wasmGas"] = 0 }},
		{"unmeasured refund", func(m map[string]any) { m["checks"].(map[string]any)["refund"].(map[string]any)["wasmGas"] = 0 }},
		{"changed fallback payload", func(m map[string]any) {
			m["checks"].(map[string]any)["startFallback"].(map[string]any)["payloadBytes"] = 1358
		}},
		{"changed combined fallback gas", func(m map[string]any) { m["checks"].(map[string]any)["conservativeFallbackGas"] = 0 }},
		{"wrong refund boundary", func(m map[string]any) { m["checks"].(map[string]any)["refundBoundaryBlocks"] = 3 }},
		{"wrong code hash", func(m map[string]any) { m["artifact"].(map[string]any)["codeHash"] = strings.Repeat("0", 64) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(manifest, &candidate); err != nil {
				t.Fatal(err)
			}
			tc.edit(candidate)
			changed, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, changed, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := deployGasForCode(codePath, code); err == nil {
				t.Fatal("modified v5 manifest reused the deploy measurement")
			}
		})
	}
	changedCode := append([]byte(nil), code...)
	changedCode[len(changedCode)-1] ^= 1
	var changedManifest map[string]any
	if err := json.Unmarshal(manifest, &changedManifest); err != nil {
		t.Fatal(err)
	}
	changedHash := sha256.Sum256(changedCode)
	changedCodeHash := crypto.Hash(changedCode)
	artifact := changedManifest["artifact"].(map[string]any)
	artifact["sha256"] = hex.EncodeToString(changedHash[:])
	artifact["codeHash"] = hex.EncodeToString(changedCodeHash[:])
	changed, err := json.Marshal(changedManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := deployGasForCode(codePath, changedCode); err == nil {
		t.Fatal("changed v5 module with matching manifest hashes bypassed the CLI code pin")
	}
}

// deployModule is a stand-in for a built contract: the wasm magic and version
// followed by bytes that make it unique. The builder does not read wasm, so the
// node's gatekeeper is the only thing that would reject a real module.
func deployModule() []byte {
	code := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	for i := 0; i < 64; i++ {
		code = append(code, byte(i))
	}
	return code
}

func deployNonceFixture() []byte {
	nonce := make([]byte, deployNonceBytes)
	for i := range nonce {
		nonce[i] = byte(0x40 + i)
	}
	return nonce
}

// expectedDeployAddress writes the node's derivation out in the test, so the
// builder's own helper is not the only thing that says what the address is.
func expectedDeployAddress(code, packedArgs, nonce []byte) common.Address {
	codeHash := crypto.Hash(code)
	buf := append([]byte{}, codeHash[:]...)
	buf = append(buf, packedArgs...)
	buf = append(buf, nonce...)
	digest := crypto.Hash(buf)
	var address common.Address
	address.SetBytes(digest[:])
	return address
}

// TestBuildDeployTransactionIsTheNodesOwnFormat is the point of this command:
// the bytes it signs decode with the node's own decoder, recover the signing
// address, carry the module and its code hash back out, and name an address
// derived the way `vm/wasm/util.go` derives it.
func TestBuildDeployTransactionIsTheNodesOwnFormat(t *testing.T) {
	dir := t.TempDir()
	keyPath, _ := writeTestKey(t, dir, "nodekey")
	identity, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		t.Fatalf("load nodekey: %v", err)
	}
	signer, err := nodekeyAddressOf(identity)
	if err != nil {
		t.Fatalf("signer address: %v", err)
	}

	code := deployModule()
	deployNonce := deployNonceFixture()
	amount := new(big.Int)
	maxFee := big.NewInt(1_000_000)
	tips := big.NewInt(0)

	raw, sender, address, err := BuildDeployTransaction(code, deployNonce, identity, 7, 204, amount, maxFee, tips)
	if err != nil {
		t.Fatalf("build the deploy: %v", err)
	}
	if !strings.EqualFold(sender.Hex(), "0x"+signer) {
		t.Fatalf("the recovered sender %s is not the nodekey's %s", sender.Hex(), signer)
	}

	var decoded types.Transaction
	if err := decoded.FromBytes(raw); err != nil {
		t.Fatalf("the node's decoder refused the deploy: %v", err)
	}
	if decoded.Type != types.DeployContractTx {
		t.Fatalf("the deploy is type %#x, expected %#x", decoded.Type, types.DeployContractTx)
	}
	if decoded.To != nil {
		t.Fatalf("the deploy names a recipient: %s", decoded.To.Hex())
	}
	if decoded.AccountNonce != 7 || decoded.Epoch != 204 {
		t.Fatalf("the deploy lost its nonce or epoch: %d, %d", decoded.AccountNonce, decoded.Epoch)
	}
	if amountOrZero(decoded.MaxFee).Cmp(maxFee) != 0 {
		t.Fatalf("the deploy lost its max fee")
	}

	parsed := attachments.ParseDeployContractAttachment(&decoded)
	if parsed == nil {
		t.Fatal("the deploy lost its attachment")
	}
	if parsed.CodeHash != common.Hash(crypto.Hash(code)) {
		t.Fatalf("the code hash is %s, expected %s", parsed.CodeHash.Hex(), common.Hash(crypto.Hash(code)).Hex())
	}
	if !bytesEqual(parsed.Code, code) {
		t.Fatal("the deploy lost the module bytes")
	}
	if len(parsed.Args) != 0 {
		t.Fatalf("the deploy carries %d constructor arguments, expected none", len(parsed.Args))
	}
	if !bytesEqual(parsed.Nonce, deployNonce) {
		t.Fatal("the deploy lost its deployment nonce")
	}

	// The argument format byte is part of the address, so the check pins it.
	expected := expectedDeployAddress(code, []byte{packedArgsFormatByte}, deployNonce)
	if address != expected {
		t.Fatalf("the deploy lands on %s, expected %s", address.Hex(), expected.Hex())
	}
}

func TestPrepareDeployWritesOwnerOnlySignedArtifact(t *testing.T) {
	dir := t.TempDir()
	codePath := filepath.Join(dir, "synthetic.wasm")
	if err := os.WriteFile(codePath, deployModule(), 0o600); err != nil {
		t.Fatal(err)
	}
	keyPath, _ := writeTestKey(t, dir, "nodekey")
	for _, existing := range []bool{false, true} {
		name := "new"
		if existing {
			name = "replace"
		}
		t.Run(name, func(t *testing.T) {
			outPath := filepath.Join(dir, name+".json")
			if existing {
				if err := os.WriteFile(outPath, []byte("old"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(outPath, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			runPrepareDeploy(codePath, keyPath, hex.EncodeToString(deployNonceFixture()), "test", outPath,
				7, 204, big.NewInt(0), big.NewInt(1_000_000), big.NewInt(0), false, "", "", nil)
			info, err := os.Lstat(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("signed deploy artifact mode %o, want 0600", info.Mode().Perm())
			}
			raw, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			var artifact PreparedDeploy
			if err := json.Unmarshal(raw, &artifact); err != nil || artifact.RawTx == "" {
				t.Fatalf("signed deploy artifact missing raw transaction: %v", err)
			}
		})
	}
}

// TestDeployAddressPinsTheFormatByte keeps the one byte the binding adds to an
// empty argument list from being dropped: the same module and nonce with and
// without it must not collide.
func TestDeployAddressPinsTheFormatByte(t *testing.T) {
	code := deployModule()
	deployNonce := deployNonceFixture()
	withByte := deployAddress(code, packedDeployArgs(), deployNonce)
	withoutByte := deployAddress(code, nil, deployNonce)
	if withByte == withoutByte {
		t.Fatal("the format byte does not change the address")
	}
	if withByte != expectedDeployAddress(code, []byte{packedArgsFormatByte}, deployNonce) {
		t.Fatal("the builder's address is not the derivation the node uses")
	}
}

func TestBuildDeployTransactionRefusals(t *testing.T) {
	dir := t.TempDir()
	keyPath, _ := writeTestKey(t, dir, "nodekey")
	identity, err := crypto.LoadECDSA(keyPath)
	if err != nil {
		t.Fatalf("load nodekey: %v", err)
	}
	one := big.NewInt(1)

	if _, _, _, err := BuildDeployTransaction(nil, deployNonceFixture(), identity, 1, 204, big.NewInt(0), one, big.NewInt(0)); err == nil {
		t.Fatal("an empty module was accepted")
	}
	if _, _, _, err := BuildDeployTransaction(deployModule(), deployNonceFixture()[:16], identity, 1, 204, big.NewInt(0), one, big.NewInt(0)); err == nil {
		t.Fatal("a short deployment nonce was accepted")
	}
}

func TestParseDeployNonce(t *testing.T) {
	fresh, err := parseDeployNonce("")
	if err != nil {
		t.Fatalf("draw a nonce: %v", err)
	}
	if len(fresh) != deployNonceBytes {
		t.Fatalf("the drawn nonce is %d bytes, expected %d", len(fresh), deployNonceBytes)
	}
	again, err := parseDeployNonce("")
	if err != nil {
		t.Fatalf("draw a second nonce: %v", err)
	}
	if bytesEqual(fresh, again) {
		t.Fatal("two drawn nonces are identical")
	}

	pinned := strings.Repeat("ab", deployNonceBytes)
	out, err := parseDeployNonce("0x" + pinned)
	if err != nil {
		t.Fatalf("read a pinned nonce: %v", err)
	}
	if len(out) != deployNonceBytes || out[0] != 0xab {
		t.Fatalf("the pinned nonce did not survive: %x", out)
	}

	if _, err := parseDeployNonce("not hex"); err == nil {
		t.Fatal("a non-hex nonce was accepted")
	}
	if _, err := parseDeployNonce("ab"); err == nil {
		t.Fatal("a short nonce was accepted")
	}
}
