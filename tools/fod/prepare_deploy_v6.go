package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/idena-network/idena-go/crypto"
)

const directV6CodeHashHex = "03389d9ddb389e1afd7c4e0080cbae6dc6cb8ee29c027a8fa737082a592d450d"
const directV6DeployWasmGas uint64 = 4_234_803

// A v6 deployment uses only the exact locally measured module. The contract
// remains experimental; this gate does not substitute for a real seller proof.
func deployGasForDirectV6Code(codePath string, code []byte) (operationGas, error) {
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(codePath)), "DIRECT_V6_MANIFEST.json"))
	if err != nil {
		return operationGas{}, fmt.Errorf("read direct v6 candidate manifest: %w", err)
	}
	type measurement struct {
		WasmGas                    uint64 `json:"wasmGas"`
		PayloadBytes               uint64 `json:"payloadBytes"`
		ConservativeTransactionGas uint64 `json:"conservativeTransactionGas"`
	}
	var manifest struct {
		Format   string `json:"format"`
		Version  int    `json:"version"`
		Status   string `json:"status"`
		Artifact struct {
			Path     string `json:"path"`
			Bytes    int    `json:"bytes"`
			Sha256   string `json:"sha256"`
			CodeHash string `json:"codeHash"`
		} `json:"artifact"`
		Checks struct {
			Runtime                 string      `json:"runtime"`
			SourceOnlyGuest         string      `json:"sourceOnlyGuest"`
			SealedSyntheticFallback string      `json:"sealedSyntheticFallback"`
			FastContentImageID      string      `json:"fastContentImageId"`
			Deploy                  measurement `json:"deploy"`
			Activate                measurement `json:"activate"`
			StartFallback           measurement `json:"startFallback"`
			ProveAndReveal          measurement `json:"proveAndReveal"`
			ConservativeFallbackGas uint64      `json:"conservativeFallbackGas"`
			BuyerBond               struct {
				GasBudget              uint64 `json:"gasBudget"`
				FeeMargin              uint64 `json:"feeMargin"`
				ReimbursementGasBudget uint64 `json:"reimbursementGasBudget"`
			} `json:"buyerBond"`
			SellerBond struct {
				ProofGasBudget uint64 `json:"proofGasBudget"`
				FeeMargin      uint64 `json:"feeMargin"`
			} `json:"sellerBond"`
			BlockGasLimit uint64 `json:"blockGasLimit"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return operationGas{}, fmt.Errorf("parse direct v6 candidate manifest: %w", err)
	}
	sha := sha256.Sum256(code)
	codeHash := crypto.Hash(code)
	if manifest.Format != "fod.direct-candidate-manifest" || manifest.Version != 6 ||
		manifest.Status != "candidate-only" ||
		manifest.Artifact.Path != "release/artifacts/fod_direct_escrow_v6.wasm" ||
		manifest.Artifact.Bytes != len(code) || manifest.Artifact.Sha256 != hex.EncodeToString(sha[:]) ||
		manifest.Artifact.CodeHash != hex.EncodeToString(codeHash[:]) ||
		manifest.Artifact.CodeHash != directV6CodeHashHex ||
		manifest.Checks.Runtime != "pinned idena-wasm-binding f7c71ce32d86c8ed26e74935265c95768b0230d0" ||
		manifest.Checks.SourceOnlyGuest != "two clean source builds with identical image and ELF; image matches v6 contract, v6 harness, v5 CLI and sealed fixture" ||
		manifest.Checks.SealedSyntheticFallback != "genuine fast-content receipt and four-leaf session accepted in the pinned Idena WASM runtime" ||
		manifest.Checks.FastContentImageID != fastContentV5ImageIDHex ||
		manifest.Checks.BuyerBond.GasBudget != 2_500_000 || manifest.Checks.BuyerBond.FeeMargin != 2 ||
		manifest.Checks.BuyerBond.ReimbursementGasBudget != 2_500_000 ||
		manifest.Checks.SellerBond.ProofGasBudget != 2_500_000 || manifest.Checks.SellerBond.FeeMargin != 2 ||
		manifest.Checks.Deploy.WasmGas != directV6DeployWasmGas ||
		manifest.Checks.BlockGasLimit != maxUserGasUpgrade11 {
		return operationGas{}, errors.New("direct v6 candidate differs from its pinned code, proof, runtime, or bond policy")
	}
	measured := func(m measurement) bool {
		if m.WasmGas == 0 || m.PayloadBytes == 0 || m.WasmGas > maxUserGasUpgrade11*wasmGasPerUserGas ||
			m.PayloadBytes > maxUserGasUpgrade11 || m.PayloadBytes+2048 > maxUserGasUpgrade11 {
			return false
		}
		want := userGasForWasmGas(m.WasmGas) + (m.PayloadBytes+2048)*10
		return m.ConservativeTransactionGas == want && want < maxUserGasUpgrade11
	}
	if !measured(manifest.Checks.Deploy) || !measured(manifest.Checks.Activate) ||
		!measured(manifest.Checks.StartFallback) || !measured(manifest.Checks.ProveAndReveal) ||
		manifest.Checks.Deploy.PayloadBytes != uint64(len(code)) ||
		manifest.Checks.Activate.PayloadBytes != directV6PoolDeployArgBytes ||
		manifest.Checks.ConservativeFallbackGas !=
			manifest.Checks.StartFallback.ConservativeTransactionGas+manifest.Checks.ProveAndReveal.ConservativeTransactionGas ||
		manifest.Checks.ConservativeFallbackGas > manifest.Checks.BuyerBond.ReimbursementGasBudget {
		return operationGas{}, errors.New("direct v6 candidate gas measurements do not cover deploy, activation, and fallback")
	}
	return operationGas{Operation: operationDeployModule, WasmGas: manifest.Checks.Deploy.WasmGas, TxBytes: uint64(len(code)), Runs: 1}, nil
}
