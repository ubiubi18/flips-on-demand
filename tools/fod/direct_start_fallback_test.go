package main

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func directTransitionFixture(t *testing.T) (*client, *DirectOffer, *DirectOrder, *DirectIntent, *big.Int, *uint64, *ecdsa.PrivateKey, *ecdsa.PrivateKey, string, [32]byte, func()) {
	t.Helper()
	seller, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	other, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	var author [20]byte
	copy(author[:], crypto.PubkeyToAddress(seller.PublicKey).Bytes())
	offer := DirectOffer{Author: author, Epoch: 230, Covered: 1, Inventory: [32]byte{1}, Keys: [32]byte{2},
		Price: big.NewInt(1000), NextNonce: 2, OpenOrderID: 7}
	id := DirectOfferID(author, offer.Epoch)
	order := &DirectOrder{OfferID: id, Buyer: [20]byte{3}, DeliveryKey: [65]byte{4, 5},
		Paid: big.NewInt(1000), Bond: big.NewInt(5_000_000), FundedAt: 100, Nonce: 1, State: DirectFunded}
	intent := &DirectIntent{Stake: big.NewInt(1000), At: 150}
	proofRate := big.NewInt(1)
	head := uint64(200)
	contract := directV3ContractHex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "contract_getStake":
			result = map[string]any{"Hash": "0x" + directV3CodeHashHex}
		case "contract_iterateMap":
			if len(request.Params) < 2 || request.Params[1] != directOfferPrefix {
				http.Error(w, "unexpected map prefix", http.StatusBadRequest)
				return
			}
			raw, err := EncodeDirectOffer(offer)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			result = mapPage{Items: []*mapItem{directMarketItem(id[:], raw)}}
		case "contract_readData":
			if len(request.Params) < 2 {
				http.Error(w, "unexpected state key", http.StatusBadRequest)
				return
			}
			var raw []byte
			switch request.Params[1] {
			case "direct/order/7":
				var err error
				raw, err = EncodeDirectOrder(*order)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			case "direct/intent/7":
				raw = make([]byte, directIntentRecordBytes)
				intent.Stake.FillBytes(raw[:16])
				binary.BigEndian.PutUint64(raw[16:], intent.At)
			case "direct/stake/7":
				raw = make([]byte, directStakeRecordBytes)
				intent.Stake.FillBytes(raw)
			case "direct/proof-rate/7":
				if proofRate.Sign() == 0 {
					_, _ = w.Write([]byte(`{"result":null,"error":{"code":-32000,"message":"data is nil"}}`))
					return
				}
				raw = make([]byte, directProofRateRecordBytes)
				proofRate.FillBytes(raw)
			default:
				http.Error(w, "unexpected state key", http.StatusBadRequest)
				return
			}
			result = "0x" + hex.EncodeToString(raw)
		case "bcn_lastBlock":
			result = map[string]any{"height": head}
		case "bcn_feePerGas":
			result = "0x1"
		case "bcn_blockAt":
			height, ok := request.Params[0].(float64)
			if !ok || (height != 10 && height != 11) {
				http.Error(w, "unexpected block height", http.StatusBadRequest)
				return
			}
			root := strings.Repeat("45", 32)
			if height == 11 {
				root = strings.Repeat("67", 32)
			}
			result = map[string]any{"height": uint64(height), "root": "0x" + root}
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	}))
	return &client{url: server.URL, http: newHTTPClient()}, &offer, order, intent, proofRate, &head, seller, other, contract, id, server.Close
}

func TestDirectReserveFallbackPreparesSellerStakeWithoutVaultOrProof(t *testing.T) {
	c, offer, order, _, _, head, seller, other, contract, id, closeNode := directTransitionFixture(t)
	defer closeNode()
	sellerAddress, err := nodekeyAddressOf(seller)
	if err != nil {
		t.Fatal(err)
	}
	otherAddress, err := nodekeyAddressOf(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDirectReserveFallback(c, contract, 7, otherAddress); err == nil {
		t.Fatal("another account reserved the seller's fallback")
	}
	prepared, err := prepareDirectReserveFallback(c, contract, 7, sellerAddress)
	if err != nil {
		t.Fatal(err)
	}
	calldata := mustDecode(prepared.Calldata)
	if prepared.Format != preparedDirectReserveFallbackFormat || prepared.Operation != directReserveFallbackOperation ||
		prepared.Amount != "125000" || len(calldata) != 8 || binary.BigEndian.Uint64(calldata) != 7 ||
		prepared.DirectOfferID != hex.EncodeToString(id[:]) || prepared.OrderNonce != order.Nonce ||
		prepared.Author != sellerAddress || prepared.Price != offer.Price.String() || prepared.Proof != "" || prepared.DirectSession != nil {
		t.Fatalf("wrong secret-free reserve action: %+v", prepared)
	}
	if err := validatePreparedDirect(c, prepared, seller); err != nil {
		t.Fatalf("seller reservation submit refused: %v", err)
	}
	if err := validatePreparedDirect(c, prepared, other); err == nil {
		t.Fatal("another account validated reservation")
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "seller.key")
	if err := crypto.SaveECDSA(keyPath, seller); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "reserve.json")
	if err := directCommand(directCommandOptions{Mode: "direct-reserve-fallback", Node: c, Network: NetworkNamePinned,
		Contract: contract, CodeHash: directV3CodeHashHex, OrderID: 7, NodeKeyPath: keyPath,
		PreparedPath: path}); err != nil {
		t.Fatalf("secret-free reserve command failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("reserve action not written: %v", err)
	}
	stale := *prepared
	stale.OrderNonce++
	if err := validatePreparedDirect(c, &stale, seller); err == nil {
		t.Fatal("changed reservation nonce validated")
	}
	stale = *prepared
	stale.Amount = "99999"
	if err := validatePreparedDirect(c, &stale, seller); err == nil {
		t.Fatal("reservation below current minimum validated")
	}
	*head = order.FundedAt + directConfirmGrace - 1
	if _, err := prepareDirectReserveFallback(c, contract, 7, sellerAddress); err == nil {
		t.Fatal("reservation bypassed buyer confirmation grace")
	}
	*head = order.FundedAt + directProofWindow - directReserveMargin - 1
	if err := validatePreparedDirect(c, prepared, seller); err == nil {
		t.Fatal("reservation accepted with no inclusion margin")
	}
	*head = 200
	order.State = DirectFallbackIntent
	if err := validatePreparedDirect(c, prepared, seller); err == nil {
		t.Fatal("stale reservation validated after inclusion")
	}
}

func TestDirectStartFallbackRequiresSellerAndCurrentFundedOrder(t *testing.T) {
	c, _, order, intent, _, head, seller, other, contract, id, closeNode := directTransitionFixture(t)
	defer closeNode()
	sellerAddress, err := nodekeyAddressOf(seller)
	if err != nil {
		t.Fatal(err)
	}
	otherAddress, err := nodekeyAddressOf(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := directStartFallbackReady(c, contract, 7, otherAddress); err == nil {
		t.Fatal("another account prepared fallback start")
	}
	if _, _, err := directStartFallbackReady(c, contract, 7, sellerAddress); err == nil {
		t.Fatal("proof start accepted funded state without seller intent")
	}
	order.State = DirectFallbackIntent
	intent.At = *head + 1
	if _, _, err := directStartFallbackReady(c, contract, 7, sellerAddress); err == nil {
		t.Fatal("future intent height accepted as included")
	}
	intent.At = 150
	intent.Stake.SetUint64(1)
	if _, _, err := directStartFallbackReady(c, contract, 7, sellerAddress); err == nil {
		t.Fatal("intent stake below offer price accepted")
	}
	intent.Stake.SetUint64(1000)
	offer, context, err := directStartFallbackReady(c, contract, 7, sellerAddress)
	if err != nil {
		t.Fatal(err)
	}
	if context.Offer.ID != hex.EncodeToString(id[:]) || hex.EncodeToString(offer.Author[:]) != sellerAddress {
		t.Fatal("fallback start resolved another offer")
	}
	order.State = DirectFallbackPending
	if _, _, err := directStartFallbackReady(c, contract, 7, sellerAddress); err == nil {
		t.Fatal("stale fallback start prepared after inclusion")
	}
	order.State = DirectFallbackIntent
	*head = intent.At + directIntentWindow
	if _, _, err := directStartFallbackReady(c, contract, 7, sellerAddress); err == nil {
		t.Fatal("fallback start prepared at intent deadline")
	}
}

func TestDirectStartFallbackRequiresProofBeforeWritingAction(t *testing.T) {
	c, _, _, _, _, _, _, _, contract, _, closeNode := directTransitionFixture(t)
	defer closeNode()
	path := filepath.Join(t.TempDir(), "start.json")
	err := directCommand(directCommandOptions{Mode: "direct-start-fallback", OrderID: 7,
		Node: c, Network: NetworkNamePinned, Contract: contract, CodeHash: directV3CodeHashHex,
		NodeKeyPath: "seller.key", PreparedPath: path})
	if err == nil || !strings.Contains(err.Error(), "-proof -vault -registration-host") {
		t.Fatalf("missing proof inputs were accepted: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("fallback action was written without proof inputs: %v", err)
	}
}

func TestDirectStartProofChecksFundedStatementAndRunsVerifier(t *testing.T) {
	c, offer, order, intent, _, head, seller, other, contract, _, closeNode := directTransitionFixture(t)
	defer closeNode()
	secrets := make([]byte, directSecretsBytes)
	copy(secrets, directSecretsDomain)
	binary.BigEndian.PutUint16(secrets[len(directSecretsDomain):], offer.Epoch)
	copy(secrets[len(directSecretsDomain)+2:], offer.Author[:])
	offer.Keys = sha256.Sum256(secrets)
	order.State = DirectFallbackIntent

	journal := make([]byte, directJournalBytes)
	binary.BigEndian.PutUint32(journal[:4], registrationStatementVersion)
	contractBytes, _ := hex.DecodeString(contract)
	copy(journal[registrationContractOffset:], contractBytes)
	copy(journal[registrationAuthorOffset:], offer.Author[:])
	binary.BigEndian.PutUint16(journal[registrationEpochOffset:], offer.Epoch)
	binary.BigEndian.PutUint32(journal[registrationCoveredOffset:], offer.Covered)
	binary.BigEndian.PutUint64(journal[registrationInventoryHeightOffset:], 10)
	binary.BigEndian.PutUint64(journal[registrationVerdictHeightOffset:], 11)
	for at := registrationInventoryRootOffset; at < registrationInventoryRootOffset+32; at++ {
		journal[at] = 0x45
	}
	for at := registrationVerdictRootOffset; at < registrationVerdictRootOffset+32; at++ {
		journal[at] = 0x67
	}
	copy(journal[registrationCommitmentOffset:], offer.Keys[:])
	copy(journal[registrationInventoryOffset:], offer.Inventory[:])
	for _, item := range []struct {
		at    int
		label string
	}{
		{registrationPolicyOffset, EligibilityPolicyV1},
		{registrationConsensusOffset, ConsensusVersionPinned},
		{registrationNetworkOffset, NetworkNamePinned},
	} {
		digest, err := hex.DecodeString(labelDigest(item.label))
		if err != nil {
			t.Fatal(err)
		}
		copy(journal[item.at:], digest)
	}
	claim, err := claimDigestFromHex(directV2ImageIDHex, journal)
	if err != nil {
		t.Fatal(err)
	}
	artefact := registrationArtefact{Statement: registrationStatement, Journal: hex.EncodeToString(journal),
		Seal: hex.EncodeToString(make([]byte, directSealBytes)), SealBytes: directSealBytes,
		ImageID: directV2ImageIDHex, ClaimDigest: hex.EncodeToString(claim[:])}
	proofPath := filepath.Join(t.TempDir(), "proof.json")
	writeProof := func() {
		t.Helper()
		raw, err := json.Marshal(artefact)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(proofPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeProof()
	hostPath := filepath.Join(t.TempDir(), "verify-host")
	if err := os.WriteFile(hostPath, []byte("#!/bin/sh\n[ \"$#\" -eq 4 ] && [ \"$1\" = '--verify-direct-v2' ] && [ \"$3\" = '--contract' ] && [ \"$RISC0_DEV_MODE\" = 0 ] && [ -z \"${FOD_PASSPHRASE+x}\" ]\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FOD_PASSPHRASE", "test-canary")
	if _, err := verifyDirectStartProof(c, contract, 7, proofPath, hostPath, secrets); err != nil {
		t.Fatalf("matching proof readiness refused: %v", err)
	}
	sellerAddress, err := nodekeyAddressOf(seller)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareDirectStartFallback(c, contract, 7, sellerAddress, proofPath, hostPath, secrets)
	if err != nil {
		t.Fatalf("matching proof start refused: %v", err)
	}
	calldata := mustDecode(prepared.Calldata)
	if prepared.Format != preparedDirectStartFallbackFormat || prepared.Operation != directStartFallbackOperation ||
		prepared.Amount != "0" || prepared.Author != sellerAddress || prepared.DirectOrderID != 7 ||
		len(calldata) != directStartFallbackArgBytes || binary.BigEndian.Uint64(calldata[:8]) != 7 ||
		!equalBytes(calldata[8:8+directJournalBytes], journal) || !equalBytes(calldata[8+directJournalBytes:], make([]byte, directSealBytes)) ||
		prepared.ClaimDigest != artefact.ClaimDigest || prepared.ImageID != directV2ImageIDHex {
		t.Fatalf("fallback start lost proof bytes or metadata: %+v", prepared)
	}
	if err := validatePreparedDirect(c, prepared, seller); err != nil || !prepared.directValidated {
		t.Fatalf("seller's proof-backed start refused: %v", err)
	}
	if err := validatePreparedDirect(c, prepared, other); err == nil {
		t.Fatal("another account validated seller's fallback start")
	}
	badMetadata := *prepared
	badMetadata.ClaimDigest = strings.Repeat("00", 32)
	if err := validatePreparedDirect(c, &badMetadata, seller); err == nil {
		t.Fatal("changed start proof metadata was accepted")
	}
	badRoot := *prepared
	changed := append([]byte(nil), calldata...)
	changed[8+registrationInventoryRootOffset] ^= 1
	badRoot.Calldata = hex.EncodeToString(changed)
	newClaim, err := claimDigestFromHex(directV2ImageIDHex, changed[8:8+directJournalBytes])
	if err != nil {
		t.Fatal(err)
	}
	badRoot.ClaimDigest = hex.EncodeToString(newClaim[:])
	if err := validatePreparedDirect(c, &badRoot, seller); err == nil || !strings.Contains(err.Error(), "canonical header") {
		t.Fatalf("changed start proof root was accepted: %v", err)
	}
	badFlags := *prepared
	changed = append([]byte(nil), calldata...)
	changed[8+registrationFlagsOffset+3] = 0b1000
	badFlags.Calldata = hex.EncodeToString(changed)
	newClaim, err = claimDigestFromHex(directV2ImageIDHex, changed[8:8+directJournalBytes])
	if err != nil {
		t.Fatal(err)
	}
	badFlags.ClaimDigest = hex.EncodeToString(newClaim[:])
	if err := validatePreparedDirect(c, &badFlags, seller); err == nil {
		t.Fatal("unsupported start proof flags were accepted")
	}
	oldKeys := offer.Keys
	offer.Keys[0] ^= 1
	if err := validatePreparedDirect(c, prepared, seller); err == nil {
		t.Fatal("start proof accepted changed live offer keys")
	}
	offer.Keys = oldKeys
	order.State = DirectFallbackPending
	if err := validatePreparedDirect(c, prepared, seller); err == nil {
		t.Fatal("stale start validated after fallback inclusion")
	}
	order.State = DirectFallbackIntent
	*head = intent.At + directIntentWindow
	if err := validatePreparedDirect(c, prepared, seller); err == nil {
		t.Fatal("start validated after intent boundary")
	}
	*head = 200
	bad := artefact
	bad.Journal = strings.Repeat("00", directJournalBytes)
	artefact = bad
	writeProof()
	if _, err := verifyDirectStartProof(c, contract, 7, proofPath, hostPath, secrets); err == nil {
		t.Fatal("wrong journal passed proof readiness")
	}
	artefact = bad
	artefact.Journal = hex.EncodeToString(journal)
	writeProof()
	if err := os.WriteFile(hostPath, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyDirectStartProof(c, contract, 7, proofPath, hostPath, secrets); err == nil || !strings.Contains(err.Error(), "host refused") {
		t.Fatalf("failed seal verifier was accepted: %v", err)
	}
}

func TestDirectProofWaitsForIncludedFallbackAndRefundAcceptsPending(t *testing.T) {
	c, offer, order, intent, proofRate, head, seller, other, contract, id, closeNode := directTransitionFixture(t)
	defer closeNode()
	sellerAddress, err := nodekeyAddressOf(seller)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDirectFallback(c, contract, 7, sellerAddress, nil); err == nil || !strings.Contains(err.Error(), "included fallback-pending") {
		t.Fatalf("reveal preparation did not wait for included state: %v", err)
	}
	proof := &preparedAction{Format: preparedDirectFallbackFormat, Version: actionRequestVersion,
		Operation: directFallbackOperation, Network: NetworkNamePinned, Contract: contract, Amount: "0",
		DirectOfferID: hex.EncodeToString(id[:]), DirectOrderID: 7}
	calldata := make([]byte, directProveArgBytes)
	binary.BigEndian.PutUint64(calldata, 7)
	proof.Calldata = hex.EncodeToString(calldata)
	if err := validatePreparedDirect(c, proof, seller); err == nil || !strings.Contains(err.Error(), "included fallback-pending") {
		t.Fatalf("proof submission did not wait for included state: %v", err)
	}
	if err := directCommand(directCommandOptions{Mode: "direct-fallback", Node: c, Network: NetworkNamePinned,
		Contract: contract, CodeHash: directV3CodeHashHex, OrderID: 7,
		NodeKeyPath: filepath.Join(t.TempDir(), "seller.key"), VaultPath: filepath.Join(t.TempDir(), "vault.json"),
		PreparedPath: filepath.Join(t.TempDir(), "fallback.json")}); err == nil || !strings.Contains(err.Error(), "included fallback-pending") {
		t.Fatalf("CLI touched seller key or vault before included state: %v", err)
	}
	order.State = DirectFallbackIntent
	if _, err := prepareDirectRefund(c, contract, 7); err == nil {
		t.Fatal("intent refunded before its deadline")
	}
	*head = intent.At + directIntentWindow
	if _, err := prepareDirectRefund(c, contract, 7); err != nil {
		t.Fatalf("expired intent did not unlock refund: %v", err)
	}
	*head = 200
	order.State = DirectFallbackPending
	proofRate.SetUint64(0)
	if _, _, _, err := directFallbackReady(c, contract, 7); err == nil || !strings.Contains(err.Error(), "proof-rate") {
		t.Fatalf("legacy or stale pending state disclosed keys without verified proof rate: %v", err)
	}
	proofRate.SetUint64(1)
	if _, _, _, err := directFallbackReady(c, contract, 7); err != nil {
		t.Fatalf("included fallback state refused: %v", err)
	}
	dir := t.TempDir()
	otherKeyPath := filepath.Join(dir, "other.key")
	if err := crypto.SaveECDSA(otherKeyPath, other); err != nil {
		t.Fatal(err)
	}
	if err := directCommand(directCommandOptions{Mode: "direct-fallback", Node: c, Network: NetworkNamePinned,
		Contract: contract, CodeHash: directV3CodeHashHex, OrderID: 7,
		NodeKeyPath: otherKeyPath, VaultPath: filepath.Join(dir, "missing-vault.json"),
		PreparedPath: filepath.Join(dir, "fallback.json")}); err == nil || !strings.Contains(err.Error(), "offer's author") {
		t.Fatalf("CLI read the vault before rejecting another seller: %v", err)
	}
	if _, err := prepareDirectFallback(c, contract, 7, sellerAddress, nil); err == nil || !strings.Contains(err.Error(), "vault differs") {
		t.Fatalf("included state did not reach vault checks: %v", err)
	}
	secrets := make([]byte, directSecretsBytes)
	copy(secrets, directSecretsDomain)
	binary.BigEndian.PutUint16(secrets[len(directSecretsDomain):], offer.Epoch)
	copy(secrets[len(directSecretsDomain)+2:], offer.Author[:])
	offer.Keys = sha256.Sum256(secrets)
	if _, err := prepareDirectFallback(c, contract, 7, "other seller", secrets); err == nil {
		t.Fatal("another seller prepared public reveal")
	}
	reveal, err := prepareDirectFallback(c, contract, 7, sellerAddress, secrets)
	if err != nil {
		t.Fatalf("committed reveal refused: %v", err)
	}
	revealed := mustDecode(reveal.Calldata)
	if len(revealed) != directProveArgBytes || binary.BigEndian.Uint64(revealed[:8]) != 7 || !equalBytes(revealed[8:], secrets) ||
		reveal.ImageID != "" || reveal.ClaimDigest != "" {
		t.Fatalf("reveal contains the wrong bytes or proof metadata: %+v", reveal)
	}
	if err := validatePreparedDirect(c, reveal, seller); err != nil {
		t.Fatalf("committed reveal submission refused: %v", err)
	}
	proofRate.SetUint64(0)
	if err := validatePreparedDirect(c, reveal, seller); err == nil {
		t.Fatal("reveal submitted after proof rate disappeared")
	}
	proofRate.SetUint64(1)
	if err := validatePreparedDirect(c, reveal, other); err == nil {
		t.Fatal("another seller validated public reveal")
	}
	changed := *reveal
	changedBytes := append([]byte(nil), revealed...)
	changedBytes[len(changedBytes)-1] ^= 1
	changed.Calldata = hex.EncodeToString(changedBytes)
	if err := validatePreparedDirect(c, &changed, seller); err == nil {
		t.Fatal("uncommitted reveal keys validated")
	}
	*head = DirectProofDeadline(order.FundedAt)
	if err := validatePreparedDirect(c, reveal, seller); err == nil {
		t.Fatal("reveal validated at refund deadline")
	}
	refund, err := prepareDirectRefund(c, contract, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePreparedDirect(c, refund, other); err != nil {
		t.Fatalf("permissionless pending refund refused: %v", err)
	}
}
