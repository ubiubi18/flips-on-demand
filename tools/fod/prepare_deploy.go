package main

// The deployment step of `SPEC.md` 7.5: the release module has to be a contract
// on a chain before anything can be listed, bought, or delivered. This builds
// and signs that one transaction, prints the address the deploy will produce,
// and stops at the network unless -broadcast is passed.
//
// The transaction is the pinned node's own deploy type carrying a
// `DeployContractAttachment`: the module bytes, their keccak256 code hash, and
// the deployment nonce. The address is
// `keccak256(keccak256(code) || packed-args || nonce)`, which is what the node's
// `vm/wasm/util.go` derives. Historical escrows take no constructor argument;
// the v6 pool takes one checked 64-byte pair of seed offer IDs.
//
// Nothing here reimplements consensus: the signed bytes are decoded again with
// the node's decoder, the sender is recovered with the node's own recovery, the
// attachment is parsed back, and the address is derived from the bytes that
// survived that round trip.

import (
	"crypto/ecdsa"
	"crypto/rand"
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

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-wasm-binding/lib"
)

const (
	preparedDeployFormat = "fod.prepared-deploy"
	// The binding prefixes every packed argument list with its format byte
	// (`lib.ArgsProtobufFormat`), so a deploy with no constructor arguments
	// still carries one byte.
	packedArgsFormatByte = 0x01
	deployNonceBytes     = 32
)

// PreparedDeploy records what a deployment will do before it happens: the
// module's identity, the address the chain will derive, and the signed bytes.
type PreparedDeploy struct {
	Format       string `json:"format"`
	Version      int    `json:"version"`
	Network      string `json:"network"`
	Code         string `json:"code"`
	CodeBytes    int    `json:"codeBytes"`
	CodeHash     string `json:"codeHash"`
	Address      string `json:"address"`
	Sender       string `json:"sender"`
	DeployNonce  string `json:"deployNonce"`
	AccountNonce uint32 `json:"accountNonce"`
	Epoch        uint16 `json:"epoch"`
	Amount       string `json:"amount"`
	MaxFee       string `json:"maxFee"`
	Tips         string `json:"tips"`
	RawTx        string `json:"rawTx"`
	Broadcast    bool   `json:"broadcast"`
	NodeHash     string `json:"nodeHash,omitempty"`
	SignedAt     string `json:"signedAt"`
}

// packedDeployArgs retains the historical no-argument deploy format used by
// existing tests. The v6 path packs its checked constructor argument with the
// binding itself.
func packedDeployArgs() []byte { return []byte{packedArgsFormatByte} }

// deployAddress is the node's own derivation (`vm/wasm/util.go`): the last 20
// bytes of keccak256 over the code hash, the packed arguments, and the
// deployment nonce.
func deployAddress(code, packedArgs, deployNonce []byte) common.Address {
	codeHash := crypto.Hash(code)
	buf := make([]byte, 0, 32+len(packedArgs)+len(deployNonce))
	buf = append(buf, codeHash[:]...)
	buf = append(buf, packedArgs...)
	buf = append(buf, deployNonce...)
	digest := crypto.Hash(buf)
	var address common.Address
	address.SetBytes(digest[:])
	return address
}

// BuildDeployTransaction assembles and signs one deploy, then checks its own
// work with the node's decoder. It returns the raw transaction, the sender, and
// the address the chain will derive for the module.
func BuildDeployTransaction(code, deployNonce []byte, identity *ecdsa.PrivateKey, nonce uint32, epoch uint16, amount, maxFee, tips *big.Int) ([]byte, common.Address, common.Address, error) {
	return buildDeployTransaction(code, deployNonce, nil, identity, nonce, epoch, amount, maxFee, tips)
}

// BuildDirectV6PoolDeployTransaction pins both original seed offers in the
// constructor. A v6 deployment never carries the liquidity deposit.
func BuildDirectV6PoolDeployTransaction(code, deployNonce, seedIDs []byte, identity *ecdsa.PrivateKey, nonce uint32, epoch uint16, amount, maxFee, tips *big.Int) ([]byte, common.Address, common.Address, error) {
	if len(seedIDs) != directV6PoolDeployArgBytes || amount == nil || amount.Sign() != 0 {
		return nil, common.Address{}, common.Address{}, errors.New("v6 deploy needs two seed IDs and zero value")
	}
	var first, second [32]byte
	copy(first[:], seedIDs[:32])
	copy(second[:], seedIDs[32:])
	if _, err := EncodeDirectV6PoolDeploySeedIDs(first, second); err != nil {
		return nil, common.Address{}, common.Address{}, err
	}
	return buildDeployTransaction(code, deployNonce, [][]byte{seedIDs}, identity, nonce, epoch, amount, maxFee, tips)
}

func buildDeployTransaction(code, deployNonce []byte, args [][]byte, identity *ecdsa.PrivateKey, nonce uint32, epoch uint16, amount, maxFee, tips *big.Int) ([]byte, common.Address, common.Address, error) {
	if len(code) == 0 {
		return nil, common.Address{}, common.Address{}, errors.New("the module is empty")
	}
	if len(deployNonce) != deployNonceBytes {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("the deployment nonce is %d bytes, expected %d", len(deployNonce), deployNonceBytes)
	}

	codeHash := common.Hash(crypto.Hash(code))
	attachment := attachments.CreateDeployContractAttachment(codeHash, code, deployNonce, args...)
	payload, err := attachment.ToBytes()
	if err != nil {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("encode the deploy attachment: %w", err)
	}

	// A deploy has no recipient: the address is derived from the attachment.
	signed, err := types.SignTx(&types.Transaction{
		AccountNonce: nonce,
		Epoch:        epoch,
		Type:         types.DeployContractTx,
		Amount:       amount,
		MaxFee:       maxFee,
		Tips:         tips,
		Payload:      payload,
	}, identity)
	if err != nil {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("sign the transaction: %w", err)
	}
	raw, err := signed.ToBytes()
	if err != nil {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("encode the transaction: %w", err)
	}

	// Check the signed bytes the way a node will: decode, recover the sender,
	// and parse the attachment back out.
	var decoded types.Transaction
	if err := decoded.FromBytes(raw); err != nil {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("the node's own decoder refused the transaction: %w", err)
	}
	if decoded.Type != types.DeployContractTx {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded transaction is not a contract deploy")
	}
	if decoded.To != nil {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded deploy names a recipient")
	}
	if decoded.AccountNonce != nonce || decoded.Epoch != epoch {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded transaction lost its nonce or epoch")
	}
	if amountOrZero(decoded.Amount).Cmp(amount) != 0 {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded transaction lost its amount")
	}
	if amountOrZero(decoded.MaxFee).Cmp(maxFee) != 0 {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded transaction lost its max fee")
	}
	parsed := attachments.ParseDeployContractAttachment(&decoded)
	if parsed == nil {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded transaction lost its deploy attachment")
	}
	if parsed.CodeHash != codeHash {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded deploy lost its code hash")
	}
	if !bytesEqual(parsed.Code, code) {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded deploy lost the module bytes")
	}
	if len(parsed.Args) != len(args) {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded deploy changed its constructor argument count")
	}
	for i := range args {
		if !bytesEqual(parsed.Args[i], args[i]) {
			return nil, common.Address{}, common.Address{}, errors.New("the decoded deploy changed a constructor argument")
		}
	}
	if !bytesEqual(parsed.Nonce, deployNonce) {
		return nil, common.Address{}, common.Address{}, errors.New("the decoded deploy lost its deployment nonce")
	}
	if len(decoded.Signature) != signatureBytes {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("the signature is %d bytes, expected %d", len(decoded.Signature), signatureBytes)
	}
	sender, err := types.Sender(&decoded)
	if err != nil {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("the node's own recovery refused the signature: %w", err)
	}
	expected, err := nodekeyAddressOf(identity)
	if err != nil {
		return nil, common.Address{}, common.Address{}, err
	}
	if !strings.EqualFold(sender.Hex(), "0x"+expected) {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("the recovered sender %s is not the nodekey's %s", sender.Hex(), expected)
	}

	// The address comes from the bytes that survived the round trip, so a
	// decoding that changed them is caught here rather than on chain.
	address := deployAddress(parsed.Code, lib.PackArguments(parsed.Args), parsed.Nonce)
	return raw, sender, address, nil
}

// parseDeployNonce draws a fresh deployment nonce, or reads the one the caller
// pinned so that a deployment can be reproduced address for address.
func parseDeployNonce(value string) ([]byte, error) {
	if value == "" {
		out := make([]byte, deployNonceBytes)
		if _, err := rand.Read(out); err != nil {
			return nil, fmt.Errorf("draw the deployment nonce: %w", err)
		}
		return out, nil
	}
	out, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(value), "0x"))
	if err != nil {
		return nil, fmt.Errorf("the deployment nonce is not hex: %w", err)
	}
	if len(out) != deployNonceBytes {
		return nil, fmt.Errorf("the deployment nonce is %d bytes, expected %d", len(out), deployNonceBytes)
	}
	return out, nil
}

// Broadcast deployment needs a manifest bound to the exact measured bytes.
// An arbitrary module cannot reuse the historical escrow's fee measurement.
func deployGasForCode(codePath string, code []byte) (operationGas, error) {
	if filepath.Base(codePath) == "fod_direct_escrow_v6.wasm" {
		return deployGasForDirectV6Code(codePath, code)
	}
	if filepath.Base(codePath) == "fod_direct_escrow_v5.wasm" {
		return deployGasForDirectV5Code(codePath, code)
	}
	manifestName, expectedFormat, expectedPath := "FAST_MANIFEST.json", "fod.fast-candidate-manifest", "release/artifacts/fod_fast_escrow.wasm"
	isDirectV2 := filepath.Base(codePath) == "fod_direct_escrow.wasm"
	isDirectV3 := filepath.Base(codePath) == "fod_direct_escrow_v3.wasm"
	isDirectV4 := filepath.Base(codePath) == "fod_direct_escrow_v4.wasm"
	isDirect := isDirectV2 || isDirectV3 || isDirectV4
	if isDirectV2 {
		manifestName, expectedFormat, expectedPath = "DIRECT_MANIFEST.json", "fod.direct-candidate-manifest", "release/artifacts/fod_direct_escrow.wasm"
	} else if isDirectV3 {
		manifestName, expectedFormat, expectedPath = "DIRECT_V3_MANIFEST.json", "fod.direct-candidate-manifest", "release/artifacts/fod_direct_escrow_v3.wasm"
	} else if isDirectV4 {
		manifestName, expectedFormat, expectedPath = "DIRECT_V4_MANIFEST.json", "fod.direct-candidate-manifest", "release/artifacts/fod_direct_escrow_v4.wasm"
	}
	manifestPath := filepath.Join(filepath.Dir(filepath.Dir(codePath)), manifestName)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return operationGas{}, fmt.Errorf("read fast candidate manifest: %w", err)
	}
	var manifest struct {
		Format   string `json:"format"`
		Version  int    `json:"version"`
		Artifact struct {
			Path   string `json:"path"`
			Bytes  int    `json:"bytes"`
			Sha256 string `json:"sha256"`
		} `json:"artifact"`
		Checks struct {
			Deploy struct {
				WasmGas uint64 `json:"wasmGas"`
			} `json:"deploy"`
			BuyerBond struct {
				GasBudget              uint64 `json:"gasBudget"`
				FeeMargin              uint64 `json:"feeMargin"`
				ReimbursementGasBudget uint64 `json:"reimbursementGasBudget"`
				SellerIntentGasBudget  uint64 `json:"sellerIntentGasBudget"`
				SellerStakeGasBudget   uint64 `json:"sellerStakeGasBudget"`
			} `json:"buyerBond"`
			BondedSellerIntent struct {
				WasmGas                    uint64 `json:"wasmGas"`
				ConservativeTransactionGas uint64 `json:"conservativeTransactionGas"`
			} `json:"bondedSellerIntent"`
			SyntheticSealedFallbackStart struct {
				WasmGas                    uint64 `json:"wasmGas"`
				ConservativeTransactionGas uint64 `json:"conservativeTransactionGas"`
				GenuineRegistrationV2      bool   `json:"genuineRegistrationV2"`
			} `json:"syntheticSealedFallbackStart"`
			PublicKeyReveal struct {
				WasmGas                    uint64 `json:"wasmGas"`
				ConservativeTransactionGas uint64 `json:"conservativeTransactionGas"`
			} `json:"publicKeyReveal"`
			RegistrationImageID     string `json:"registrationImageId"`
			ConservativeFallbackGas uint64 `json:"conservativeFallbackGas"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return operationGas{}, fmt.Errorf("parse fast candidate manifest: %w", err)
	}
	hash := sha256.Sum256(code)
	expectedVersion := 1
	if isDirectV2 {
		expectedVersion = 2
	} else if isDirectV3 {
		expectedVersion = 3
	} else if isDirectV4 {
		expectedVersion = 4
	}
	if manifest.Format != expectedFormat || manifest.Version != expectedVersion ||
		manifest.Artifact.Path != expectedPath ||
		manifest.Artifact.Bytes != len(code) || manifest.Artifact.Sha256 != hex.EncodeToString(hash[:]) ||
		manifest.Checks.Deploy.WasmGas == 0 || manifest.Checks.Deploy.WasmGas > maxUserGasUpgrade11*wasmGasPerUserGas {
		return operationGas{}, errors.New("candidate manifest does not match the measured module")
	}
	if isDirect && (manifest.Checks.BuyerBond.GasBudget != directBondGasBudget ||
		manifest.Checks.BuyerBond.FeeMargin != directBondFeeMargin ||
		manifest.Checks.RegistrationImageID != directV2ImageIDHex ||
		manifest.Checks.ConservativeFallbackGas == 0 ||
		manifest.Checks.ConservativeFallbackGas >= manifest.Checks.BuyerBond.GasBudget) {
		return operationGas{}, errors.New("direct candidate manifest does not bind a sufficient buyer bond to measured fallback gas")
	}
	if isDirectV3 && (manifest.Checks.BuyerBond.ReimbursementGasBudget != 2_000_000 ||
		manifest.Checks.BuyerBond.SellerIntentGasBudget != 100_000 ||
		manifest.Checks.BondedSellerIntent.WasmGas == 0 ||
		manifest.Checks.BondedSellerIntent.ConservativeTransactionGas <
			(manifest.Checks.BondedSellerIntent.WasmGas+wasmGasPerUserGas-1)/wasmGasPerUserGas ||
		manifest.Checks.BondedSellerIntent.ConservativeTransactionGas >= maxUserGasUpgrade11 ||
		manifest.Checks.SyntheticSealedFallbackStart.WasmGas == 0 ||
		!manifest.Checks.SyntheticSealedFallbackStart.GenuineRegistrationV2 ||
		manifest.Checks.PublicKeyReveal.WasmGas == 0 ||
		manifest.Checks.SyntheticSealedFallbackStart.ConservativeTransactionGas <
			(manifest.Checks.SyntheticSealedFallbackStart.WasmGas+wasmGasPerUserGas-1)/wasmGasPerUserGas ||
		manifest.Checks.PublicKeyReveal.ConservativeTransactionGas <
			(manifest.Checks.PublicKeyReveal.WasmGas+wasmGasPerUserGas-1)/wasmGasPerUserGas ||
		manifest.Checks.SyntheticSealedFallbackStart.ConservativeTransactionGas >= maxUserGasUpgrade11 ||
		manifest.Checks.PublicKeyReveal.ConservativeTransactionGas >= maxUserGasUpgrade11 ||
		manifest.Checks.ConservativeFallbackGas !=
			manifest.Checks.SyntheticSealedFallbackStart.ConservativeTransactionGas+manifest.Checks.PublicKeyReveal.ConservativeTransactionGas ||
		manifest.Checks.ConservativeFallbackGas > manifest.Checks.BuyerBond.ReimbursementGasBudget) {
		return operationGas{}, errors.New("direct v3 manifest does not bind a genuine sealed start and key reveal to the buyer reimbursement budget")
	}
	if isDirectV4 && (manifest.Checks.BuyerBond.ReimbursementGasBudget != 2_000_000 ||
		manifest.Checks.BuyerBond.SellerStakeGasBudget != directIntentGasBudget ||
		manifest.Checks.BondedSellerIntent.WasmGas != 0 ||
		manifest.Checks.SyntheticSealedFallbackStart.WasmGas == 0 ||
		!manifest.Checks.SyntheticSealedFallbackStart.GenuineRegistrationV2 ||
		manifest.Checks.PublicKeyReveal.WasmGas == 0 ||
		manifest.Checks.SyntheticSealedFallbackStart.ConservativeTransactionGas <
			(manifest.Checks.SyntheticSealedFallbackStart.WasmGas+wasmGasPerUserGas-1)/wasmGasPerUserGas ||
		manifest.Checks.PublicKeyReveal.ConservativeTransactionGas <
			(manifest.Checks.PublicKeyReveal.WasmGas+wasmGasPerUserGas-1)/wasmGasPerUserGas ||
		manifest.Checks.SyntheticSealedFallbackStart.ConservativeTransactionGas >= maxUserGasUpgrade11 ||
		manifest.Checks.PublicKeyReveal.ConservativeTransactionGas >= maxUserGasUpgrade11 ||
		manifest.Checks.ConservativeFallbackGas !=
			manifest.Checks.SyntheticSealedFallbackStart.ConservativeTransactionGas+manifest.Checks.PublicKeyReveal.ConservativeTransactionGas ||
		manifest.Checks.ConservativeFallbackGas > manifest.Checks.BuyerBond.ReimbursementGasBudget) {
		return operationGas{}, errors.New("direct v4 manifest does not bind a genuine sealed payable start and key reveal to the buyer reimbursement budget")
	}
	return operationGas{Operation: operationDeployModule, WasmGas: manifest.Checks.Deploy.WasmGas, TxBytes: uint64(len(code)), Runs: 1}, nil
}

// The v5 candidate uses the fast-content credential and has a separate
// manifest shape. Keep this gate separate from the deployed v2-v4 manifests.
func deployGasForDirectV5Code(codePath string, code []byte) (operationGas, error) {
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(codePath)), "DIRECT_V5_MANIFEST.json"))
	if err != nil {
		return operationGas{}, fmt.Errorf("read direct v5 candidate manifest: %w", err)
	}
	type measurement struct {
		WasmGas                    uint64 `json:"wasmGas"`
		PayloadBytes               uint64 `json:"payloadBytes"`
		ConservativeTransactionGas uint64 `json:"conservativeTransactionGas"`
	}
	var manifest struct {
		Format   string `json:"format"`
		Version  int    `json:"version"`
		Artifact struct {
			Path     string `json:"path"`
			Bytes    int    `json:"bytes"`
			Sha256   string `json:"sha256"`
			CodeHash string `json:"codeHash"`
		} `json:"artifact"`
		Checks struct {
			Runtime                 string      `json:"runtime"`
			SourceOnlyGuest         string      `json:"sourceOnlyGuest"`
			SyntheticSealedFallback string      `json:"syntheticSealedFallback"`
			FastContentImageID      string      `json:"fastContentImageId"`
			Deploy                  measurement `json:"deploy"`
			List                    measurement `json:"list"`
			Buy                     measurement `json:"buy"`
			Confirm                 measurement `json:"confirm"`
			StartFallback           measurement `json:"startFallback"`
			ProveAndReveal          measurement `json:"proveAndReveal"`
			Refund                  measurement `json:"refund"`
			ConservativeFallbackGas uint64      `json:"conservativeFallbackGas"`
			BuyerBond               struct {
				GasBudget              uint64 `json:"gasBudget"`
				FeeMargin              uint64 `json:"feeMargin"`
				ReimbursementGasBudget uint64 `json:"reimbursementGasBudget"`
				SellerStakeGasBudget   uint64 `json:"sellerStakeGasBudget"`
			} `json:"buyerBond"`
			ConfirmGraceBlocks   uint64 `json:"confirmGraceBlocks"`
			DeadlineMarginBlocks uint64 `json:"deadlineMarginBlocks"`
			RefundBoundaryBlocks uint64 `json:"refundBoundaryBlocks"`
			BlockGasLimit        uint64 `json:"blockGasLimit"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return operationGas{}, fmt.Errorf("parse direct v5 candidate manifest: %w", err)
	}
	hash := sha256.Sum256(code)
	codeHash := crypto.Hash(code)
	if manifest.Format != "fod.direct-candidate-manifest" || manifest.Version != 5 ||
		manifest.Artifact.Path != "release/artifacts/fod_direct_escrow_v5.wasm" ||
		manifest.Artifact.Bytes != len(code) || manifest.Artifact.Sha256 != hex.EncodeToString(hash[:]) ||
		manifest.Artifact.CodeHash != hex.EncodeToString(codeHash[:]) ||
		manifest.Artifact.CodeHash != directV5CodeHashHex ||
		manifest.Checks.Runtime != "pinned idena-wasm-binding f7c71ce32d86c8ed26e74935265c95768b0230d0" ||
		manifest.Checks.SourceOnlyGuest != "two clean builds with identical image and ELF; image matches verifier, CLI, and sealed fixture" ||
		manifest.Checks.SyntheticSealedFallback != "genuine fast-content receipt and four-leaf session accepted in the pinned Idena WASM runtime" ||
		manifest.Checks.FastContentImageID != fastContentV5ImageIDHex ||
		manifest.Checks.BuyerBond.GasBudget != directBondGasBudget ||
		manifest.Checks.BuyerBond.FeeMargin != directBondFeeMargin ||
		manifest.Checks.BuyerBond.ReimbursementGasBudget != directBondGasBudget ||
		manifest.Checks.BuyerBond.SellerStakeGasBudget != directIntentGasBudget ||
		manifest.Checks.ConfirmGraceBlocks != 30 || manifest.Checks.DeadlineMarginBlocks != 120 ||
		manifest.Checks.RefundBoundaryBlocks != directProofWindow ||
		manifest.Checks.BlockGasLimit != maxUserGasUpgrade11 {
		return operationGas{}, errors.New("direct v5 candidate manifest differs from the pinned proof, runtime, or bond policy")
	}
	measured := func(m measurement) bool {
		if m.WasmGas == 0 || m.PayloadBytes == 0 || m.WasmGas > maxUserGasUpgrade11*wasmGasPerUserGas ||
			m.PayloadBytes > maxUserGasUpgrade11 || m.PayloadBytes+2048 > maxUserGasUpgrade11 {
			return false
		}
		want := userGasForWasmGas(m.WasmGas) + (m.PayloadBytes+2048)*10
		return m.ConservativeTransactionGas == want && want < maxUserGasUpgrade11
	}
	if !measured(manifest.Checks.Deploy) || !measured(manifest.Checks.List) ||
		!measured(manifest.Checks.Buy) || !measured(manifest.Checks.Confirm) ||
		!measured(manifest.Checks.StartFallback) || !measured(manifest.Checks.ProveAndReveal) ||
		!measured(manifest.Checks.Refund) ||
		manifest.Checks.Deploy.PayloadBytes != uint64(len(code)) ||
		manifest.Checks.List.PayloadBytes != directListArgBytes ||
		manifest.Checks.Buy.PayloadBytes != directBuyArgBytes ||
		manifest.Checks.Confirm.PayloadBytes != 8+32 ||
		manifest.Checks.StartFallback.PayloadBytes < 8+273+256+2+1 ||
		manifest.Checks.StartFallback.PayloadBytes > 8+273+256+2+65_535 ||
		manifest.Checks.ProveAndReveal.PayloadBytes != 8+106 ||
		manifest.Checks.Refund.PayloadBytes != 8 ||
		manifest.Checks.ConservativeFallbackGas != manifest.Checks.StartFallback.ConservativeTransactionGas+manifest.Checks.ProveAndReveal.ConservativeTransactionGas ||
		manifest.Checks.ConservativeFallbackGas > manifest.Checks.BuyerBond.ReimbursementGasBudget {
		return operationGas{}, errors.New("direct v5 candidate gas measurements do not cover confirmation, the sealed fallback, and refund")
	}
	return operationGas{Operation: operationDeployModule, WasmGas: manifest.Checks.Deploy.WasmGas, TxBytes: uint64(len(code)), Runs: 1}, nil
}

func runPrepareDeploy(codePath, nodeKeyPath, deployNonceHex, v6SeedIDsHex, network, outPath string, nonce uint32, epoch uint16, amount, maxFee, tips *big.Int, broadcast bool, rpcURL, apiKey string, feePerGas *big.Int) {
	if filepath.Base(codePath) == "fod_direct_escrow_v7.wasm" {
		fmt.Fprintln(os.Stderr, "refused: v7 deploy needs the one-seed -v7-seed-id path")
		os.Exit(1)
	}
	code, err := os.ReadFile(codePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read the module: %v\n", err)
		os.Exit(1)
	}
	var gas operationGas
	isV6 := filepath.Base(codePath) == "fod_direct_escrow_v6.wasm"
	codeHash := crypto.Hash(code)
	if isV6 != (hex.EncodeToString(codeHash[:]) == directV6CodeHashHex) {
		fmt.Fprintln(os.Stderr, "refused: v6 code hash and artifact name disagree")
		os.Exit(1)
	}
	if isV6 != (v6SeedIDsHex != "") {
		fmt.Fprintln(os.Stderr, "refused: v6 deployment needs -v6-seed-ids; other modules cannot use it")
		os.Exit(1)
	}
	if isV6 && (amount == nil || amount.Sign() != 0) {
		fmt.Fprintln(os.Stderr, "refused: v6 deployment must carry zero value")
		os.Exit(1)
	}
	if broadcast || isV6 || filepath.Base(codePath) == "fod_fast_escrow.wasm" || filepath.Base(codePath) == "fod_direct_escrow.wasm" || filepath.Base(codePath) == "fod_direct_escrow_v3.wasm" || filepath.Base(codePath) == "fod_direct_escrow_v4.wasm" || filepath.Base(codePath) == "fod_direct_escrow_v5.wasm" {
		gas, err = deployGasForCode(codePath, code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "refused: %v\n", err)
			os.Exit(1)
		}
	}
	identity, err := crypto.LoadECDSA(nodeKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load nodekey: %v\n", err)
		os.Exit(1)
	}
	deployNonce, err := parseDeployNonce(deployNonceHex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused: %v\n", err)
		os.Exit(1)
	}
	var raw []byte
	var sender, address common.Address
	if isV6 {
		var seedIDs []byte
		seedIDs, err = hex.DecodeString(strings.TrimPrefix(strings.ToLower(v6SeedIDsHex), "0x"))
		if err == nil {
			raw, sender, address, err = BuildDirectV6PoolDeployTransaction(code, deployNonce, seedIDs, identity, nonce, epoch, amount, maxFee, tips)
		}
	} else {
		raw, sender, address, err = BuildDeployTransaction(code, deployNonce, identity, nonce, epoch, amount, maxFee, tips)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused: %v\n", err)
		os.Exit(1)
	}

	deployed := &PreparedDeploy{
		Format:       preparedDeployFormat,
		Version:      actionRequestVersion,
		Network:      network,
		Code:         codePath,
		CodeBytes:    len(code),
		CodeHash:     strings.ToLower(strings.TrimPrefix(common.Hash(crypto.Hash(code)).Hex(), "0x")),
		Address:      strings.ToLower(strings.TrimPrefix(address.Hex(), "0x")),
		Sender:       strings.ToLower(strings.TrimPrefix(sender.Hex(), "0x")),
		DeployNonce:  hex.EncodeToString(deployNonce),
		AccountNonce: nonce,
		Epoch:        epoch,
		Amount:       amount.String(),
		MaxFee:       maxFee.String(),
		Tips:         tips.String(),
		RawTx:        hex.EncodeToString(raw),
		Broadcast:    broadcast,
		SignedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	fmt.Printf("module      %s (%d bytes)\n", deployed.Code, deployed.CodeBytes)
	fmt.Printf("code hash   %s\n", deployed.CodeHash)
	fmt.Printf("address     %s\n", deployed.Address)
	fmt.Printf("sender      %s\n", deployed.Sender)
	fmt.Printf("nonce       %d, epoch %d\n", deployed.AccountNonce, deployed.Epoch)
	fmt.Printf("amount      %s, max fee %s, tips %s\n", deployed.Amount, deployed.MaxFee, deployed.Tips)
	fmt.Printf("raw tx      %d bytes\n", len(raw))
	fmt.Println("note        a module that is not an embedded contract needs the chain's upgrade-11 flag, or validation refuses the deploy")
	fmt.Println("note        the address is derived from the module, packed constructor arguments, and deployment nonce")

	if broadcast {
		node := &client{url: rpcURL, key: apiKey, http: newHTTPClient()}
		// A deploy is one transaction, the largest one this project signs, and
		// its fee is dominated by its size. The chain's own rate decides what
		// the max fee has to cover, so a broadcast reads it here unless the
		// caller passed one, and a max fee that cannot run the deploy is
		// refused rather than sent: a call that runs out of gas is still
		// charged (`docs/PROGRESS.md`, 2026-09-26).
		rate := feePerGas
		if rate == nil {
			var chainFee big.Int
			if err := node.call("bcn_feePerGas", []any{}, &chainFee); err == nil && chainFee.Sign() > 0 {
				rate = &chainFee
			}
		}
		if rate == nil {
			fmt.Fprintln(os.Stderr, "refused before broadcasting: the node did not report a fee per gas; pass a verified rate")
			os.Exit(1)
		}
		required := requiredMaxFeeFor(gas.WasmGas, uint64(len(raw)), rate)
		fmt.Printf("estimated   %s base units at %s per gas for the %d bytes signed (%d user gas for %d measured wasm gas, %d runs)\n",
			required, rate, len(raw), userGasForWasmGas(gas.WasmGas), gas.WasmGas, gas.Runs)
		if maxFee.Cmp(required) < 0 {
			fmt.Fprintf(os.Stderr, "refused before broadcasting: this signed deploy needs a max fee of %s, and it carries %s; sign it again with the higher value\n",
				required, maxFee)
			os.Exit(1)
		}
		if cap := maxMaxFeeFor(rate); maxFee.Cmp(cap) > 0 {
			fmt.Fprintf(os.Stderr, "refused before broadcasting: the max fee exceeds the node's cap (%s)\n", cap)
			os.Exit(1)
		}
		if err := checkAccountAgainstChain(node, deployed.Sender, nonce, epoch); err != nil {
			fmt.Fprintf(os.Stderr, "refused before broadcasting: %v\n", err)
			os.Exit(1)
		}
		if isV6 {
			if err := checkDirectSpendBalance(node, deployed.Sender, nonce, amount, tips, maxFee); err != nil {
				fmt.Fprintf(os.Stderr, "refused before broadcasting: %v\n", err)
				os.Exit(1)
			}
		}
		var hash string
		if err := node.call("bcn_sendRawTx", []any{"0x" + hex.EncodeToString(raw)}, &hash); err != nil {
			fmt.Fprintf(os.Stderr, "the node refused the transaction: %v\n", err)
			os.Exit(1)
		}
		deployed.NodeHash = hash
		fmt.Printf("broadcast   %s\n", hash)
	} else {
		fmt.Println("not broadcast: pass -broadcast to send it to the node")
	}

	if outPath != "" {
		if err := writePrivateJSON(outPath, deployed); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", outPath, err)
			os.Exit(1)
		}
	}
}
