package main

// Offline v7 deployment signs the exact one-seed constructor. The candidate
// manifest must bind the module bytes and measured deploy gas first.

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
)

func BuildDirectV7PoolDeployTransaction(code, deployNonce, seedID []byte, identity *ecdsa.PrivateKey, nonce uint32, epoch uint16, amount, maxFee, tips *big.Int) ([]byte, common.Address, common.Address, error) {
	if len(seedID) != 32 || amount == nil || amount.Sign() != 0 {
		return nil, common.Address{}, common.Address{}, errors.New("v7 deploy needs one 32-byte seed ID and zero value")
	}
	var id [32]byte
	copy(id[:], seedID)
	arg, err := encodePoolV7SeedID(id)
	if err != nil {
		return nil, common.Address{}, common.Address{}, err
	}
	return buildDeployTransaction(code, deployNonce, [][]byte{arg}, identity, nonce, epoch, amount, maxFee, tips)
}

func deployGasForDirectV7Code(codePath string, code []byte) (operationGas, error) {
	if filepath.Base(codePath) != "fod_direct_escrow_v7.wasm" {
		return operationGas{}, errors.New("v7 deploy requires the named v7 candidate module")
	}
	manifestPath := filepath.Join(filepath.Dir(filepath.Dir(codePath)), "DIRECT_V7_MANIFEST.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return operationGas{}, fmt.Errorf("read v7 candidate manifest: %w", err)
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
			Deploy struct {
				WasmGas                    uint64 `json:"wasmGas"`
				PayloadBytes               uint64 `json:"payloadBytes"`
				ConservativeTransactionGas uint64 `json:"conservativeTransactionGas"`
			} `json:"deploy"`
			BlockGasLimit      uint64 `json:"blockGasLimit"`
			FastContentImageID string `json:"fastContentImageId"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return operationGas{}, err
	}
	sha := sha256.Sum256(code)
	hash := crypto.Hash(code)
	if manifest.Format != "fod.direct-candidate-manifest" || manifest.Version != 7 || manifest.Status != "candidate-only" ||
		manifest.Artifact.Path != "release/artifacts/fod_direct_escrow_v7.wasm" ||
		manifest.Artifact.Bytes != len(code) || manifest.Artifact.Sha256 != hex.EncodeToString(sha[:]) ||
		manifest.Artifact.CodeHash != hex.EncodeToString(hash[:]) ||
		manifest.Checks.FastContentImageID != fastContentV5ImageIDHex ||
		manifest.Checks.BlockGasLimit != maxUserGasUpgrade11 ||
		manifest.Checks.Deploy.WasmGas == 0 ||
		manifest.Checks.Deploy.WasmGas > maxUserGasUpgrade11*wasmGasPerUserGas ||
		manifest.Checks.Deploy.PayloadBytes != uint64(len(code)+32) ||
		manifest.Checks.Deploy.ConservativeTransactionGas !=
			userGasForWasmGas(manifest.Checks.Deploy.WasmGas)+(uint64(len(code)+32)+2048)*10 ||
		manifest.Checks.Deploy.ConservativeTransactionGas >= maxUserGasUpgrade11 {
		return operationGas{}, errors.New("v7 candidate manifest differs from measured module or policy")
	}
	return operationGas{Operation: operationDeployModule, WasmGas: manifest.Checks.Deploy.WasmGas, TxBytes: uint64(len(code) + 32), Runs: 1}, nil
}

func runPrepareDeployV7(codePath, expectedCodeHash, nodeKeyPath, deployNonceHex, seedIDHex, network, outPath string, nonce uint32, epoch uint16, amount, maxFee, tips, feePerGas *big.Int, broadcast bool) error {
	if !isHex(expectedCodeHash, 32) || expectedCodeHash != strings.ToLower(expectedCodeHash) {
		return errors.New("v7 deploy requires an independent lowercase 32-byte -code-hash pin")
	}
	if broadcast || network != NetworkNamePinned || outPath == "" || nodeKeyPath == "" ||
		amount == nil || amount.Sign() != 0 || maxFee == nil || maxFee.Sign() <= 0 ||
		tips == nil || tips.Sign() < 0 || feePerGas == nil || feePerGas.Sign() <= 0 ||
		!isHex(seedIDHex, 32) || seedIDHex != strings.ToLower(seedIDHex) {
		return errors.New("v7 deploy preparation needs pinned network, one canonical seed ID, zero value, nodekey, output, and -fee-per-gas; broadcast is disabled")
	}
	code, err := os.ReadFile(codePath)
	if err != nil {
		return err
	}
	hash := crypto.Hash(code)
	if hex.EncodeToString(hash[:]) != expectedCodeHash {
		return errors.New("v7 module differs from independent -code-hash pin")
	}
	gas, err := deployGasForDirectV7Code(codePath, code)
	if err != nil {
		return err
	}
	key, err := crypto.LoadECDSA(nodeKeyPath)
	if err != nil {
		return err
	}
	deployNonce, err := parseDeployNonce(deployNonceHex)
	if err != nil {
		return err
	}
	seed, _ := hex.DecodeString(seedIDHex)
	raw, sender, address, err := BuildDirectV7PoolDeployTransaction(code, deployNonce, seed, key, nonce, epoch, amount, maxFee, tips)
	if err != nil {
		return err
	}
	if required := requiredMaxFeeFor(gas.WasmGas, uint64(len(raw)), feePerGas); maxFee.Cmp(required) < 0 {
		return fmt.Errorf("v7 deploy max fee %s is below measured minimum %s at supplied fee rate", maxFee, required)
	}
	if maxFee.Cmp(maxMaxFeeFor(feePerGas)) > 0 {
		return errors.New("v7 deploy max fee exceeds the node fee cap at supplied rate")
	}
	prepared := &PreparedDeploy{
		Format: preparedDeployFormat, Version: actionRequestVersion, Network: network,
		Code: codePath, CodeBytes: len(code), CodeHash: hex.EncodeToString(hash[:]),
		Address: hex.EncodeToString(address[:]), Sender: hex.EncodeToString(sender[:]),
		DeployNonce: hex.EncodeToString(deployNonce), AccountNonce: nonce, Epoch: epoch,
		Amount: "0", MaxFee: maxFee.String(), Tips: tips.String(), RawTx: hex.EncodeToString(raw),
		Broadcast: false, SignedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := writePrivateJSON(outPath, prepared); err != nil {
		return err
	}
	fmt.Printf("v7 deploy prepared offline: contract %s, code hash %s, one seed offer %s, zero value; no transaction sent\n", prepared.Address, prepared.CodeHash, seedIDHex)
	return nil
}
