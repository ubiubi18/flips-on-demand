package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func directV4ProofFixture(t *testing.T, offer *DirectOffer, contract string) (string, string, []byte, []byte) {
	t.Helper()
	secrets := make([]byte, directSecretsBytes)
	copy(secrets, directSecretsDomain)
	binary.BigEndian.PutUint16(secrets[len(directSecretsDomain):], offer.Epoch)
	copy(secrets[len(directSecretsDomain)+2:], offer.Author[:])
	offer.Keys = sha256.Sum256(secrets)
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
	artefact := registrationArtefact{
		Statement: registrationStatement, Journal: hex.EncodeToString(journal),
		Seal: hex.EncodeToString(make([]byte, directSealBytes)), SealBytes: directSealBytes,
		ImageID: directV2ImageIDHex, ClaimDigest: hex.EncodeToString(claim[:]),
	}
	raw, err := json.Marshal(artefact)
	if err != nil {
		t.Fatal(err)
	}
	proofPath := filepath.Join(t.TempDir(), "proof.json")
	if err := os.WriteFile(proofPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	hostPath := filepath.Join(t.TempDir(), "verify-host")
	if err := os.WriteFile(hostPath, []byte("#!/bin/sh\n[ \"$1\" = '--verify-direct-v2' ] && [ \"$3\" = '--contract' ] && [ \"$4\" = '"+contract+"' ]\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return proofPath, hostPath, secrets, journal
}

func TestDirectV4StatusShowsSafePreparationHeights(t *testing.T) {
	order := &DirectOrder{FundedAt: 100, State: DirectFunded}
	for _, check := range []struct {
		head uint64
		open bool
	}{
		{129, false}, {130, true}, {8618, true}, {8619, false},
	} {
		status := directV4WindowStatus(order, check.head)
		if !strings.Contains(status, "blocks 130 through 8618") ||
			!strings.Contains(status, "height window open="+strconv.FormatBool(check.open)) {
			t.Fatalf("proof-start status at head %d: %s", check.head, status)
		}
	}
	order.State = DirectFallbackPending
	for _, check := range []struct {
		head uint64
		open bool
	}{
		{8738, true}, {8739, false},
	} {
		status := directV4WindowStatus(order, check.head)
		if !strings.Contains(status, "through block 8738") ||
			!strings.Contains(status, "height window open="+strconv.FormatBool(check.open)) {
			t.Fatalf("key-reveal status at head %d: %s", check.head, status)
		}
	}
}

func TestDirectV4StartIsProofBearingPayableFirstSellerAction(t *testing.T) {
	c, offer, order, _, _, head, seller, other, contract, _, closeNode := directTransitionFixture(t)
	defer closeNode()
	proofPath, hostPath, secrets, journal := directV4ProofFixture(t, offer, contract)
	sellerAddress, err := nodekeyAddressOf(seller)
	if err != nil {
		t.Fatal(err)
	}
	otherAddress, err := nodekeyAddressOf(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := directStartFallbackV4Ready(c, contract, 7, otherAddress); err == nil {
		t.Fatal("another signer prepared the seller's fallback")
	}
	*head = order.FundedAt + directConfirmGrace - 1
	if _, _, err := directStartFallbackV4Ready(c, contract, 7, sellerAddress); err == nil {
		t.Fatal("v4 start bypassed buyer confirmation grace")
	}
	*head = 200
	prepared, err := prepareDirectStartFallbackV4(c, contract, 7, sellerAddress, proofPath, hostPath, secrets)
	if err != nil {
		t.Fatal(err)
	}
	calldata := mustDecode(prepared.Calldata)
	if prepared.Format != preparedDirectStartFallbackV4Format || prepared.DirectVersion != 4 ||
		prepared.Operation != directStartFallbackOperation || prepared.Amount != "125000" ||
		prepared.Price != offer.Price.String() || prepared.OrderNonce != order.Nonce ||
		len(calldata) != directStartFallbackArgBytes || binary.BigEndian.Uint64(calldata[:8]) != 7 ||
		!equalBytes(calldata[8:8+directJournalBytes], journal) {
		t.Fatalf("v4 start lost stake or proof bytes: %+v", prepared)
	}
	// Inject the fixture's code hash as the trusted pin; release integration
	// uses the independently measured hash of the v4 module instead.
	if err := validatePreparedDirectWithV4Pin(c, prepared, seller, directV3CodeHashHex); err != nil {
		t.Fatalf("v4 start refused the matching fixture pin: %v", err)
	}
	if err := validatePreparedDirectWithV4Pin(c, prepared, seller, strings.Repeat("00", 32)); err == nil {
		t.Fatal("v4 start accepted another contract code hash")
	}
	if err := validatePreparedDirectWithV4Pin(c, prepared, other, directV3CodeHashHex); err == nil {
		t.Fatal("another signer validated the v4 start")
	}
	changed := *prepared
	changed.Amount = "99999"
	if err := validatePreparedDirectWithV4Pin(c, &changed, seller, directV3CodeHashHex); err == nil {
		t.Fatal("seller stake below the fee floor validated")
	}
	changed = *prepared
	changed.ClaimDigest = strings.Repeat("00", 32)
	if err := validatePreparedDirectWithV4Pin(c, &changed, seller, directV3CodeHashHex); err == nil {
		t.Fatal("changed proof metadata validated")
	}
	changed = *prepared
	changed.Contract = strings.Repeat("bb", 20)
	if err := validatePreparedDirectWithV4Pin(c, &changed, seller, directV3CodeHashHex); err == nil {
		t.Fatal("proof bound to another contract validated")
	}
	changed = *prepared
	changed.DirectVersion = 3
	if err := validatePreparedDirectWithV4Pin(c, &changed, seller, directV3CodeHashHex); err == nil {
		t.Fatal("v4 format accepted a v3 marker")
	}
	*head = order.FundedAt + directProofWindow - directReserveMargin - 1
	if err := validatePreparedDirectWithV4Pin(c, prepared, seller, directV3CodeHashHex); err == nil {
		t.Fatal("v4 start validated without an inclusion margin")
	}
	*head = 200
	order.State = DirectFallbackPending
	if err := validatePreparedDirectWithV4Pin(c, prepared, seller, directV3CodeHashHex); err == nil {
		t.Fatal("v4 start validated after another start was included")
	}
}

func TestDirectV4PendingStakeReadAndVersionGate(t *testing.T) {
	c, _, order, stored, _, _, _, _, contract, _, closeNode := directTransitionFixture(t)
	defer closeNode()
	order.State = DirectFallbackPending
	stake, err := directPendingStakeV4(c, contract, 7, order)
	if err != nil || stake.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("included v4 stake = %v, %v", stake, err)
	}
	stored.Stake.SetUint64(999)
	if _, err := directPendingStakeV4(c, contract, 7, order); err == nil {
		t.Fatal("stake below funded price accepted")
	}
	stored.Stake.SetUint64(0)
	if _, err := directPendingStakeV4(c, contract, 7, order); err == nil {
		t.Fatal("zero stake accepted")
	}
	if _, err := DecodeDirectOrderV4(func() []byte {
		order.State = DirectFallbackIntent
		raw, encodeErr := EncodeDirectOrder(*order)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		return raw
	}()); err == nil {
		t.Fatal("v4 decoder accepted v3-only intent state")
	}
	if version, err := directFallbackVersion(directCommandOptions{CodeHash: directV3CodeHashHex}, contract); err != nil || version != 3 {
		t.Fatalf("pinned v3 fallback rejected: %d, %v", version, err)
	}
	if _, err := directFallbackVersion(directCommandOptions{CodeHash: directV3CodeHashHex}, strings.Repeat("bb", 20)); err == nil {
		t.Fatal("v3 fallback accepted another contract")
	}
	if _, err := directFallbackVersion(directCommandOptions{DirectVersion: 4, CodeHash: directV3CodeHashHex}, contract); err == nil {
		t.Fatal("v4 fallback accepted the deployed v3 hash")
	}
}
