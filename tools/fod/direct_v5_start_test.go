package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDirectV5VersionAndPreparedCodePinFailClosed(t *testing.T) {
	if !isDirectFormat(preparedDirectStartFallbackV5Format) ||
		preparedFormats[preparedDirectStartFallbackV5Format] != directStartFallbackOperation {
		t.Fatal("v5 fallback start is absent from prepared action dispatch")
	}
	pin := strings.Repeat("ab", 32)
	contract := strings.Repeat("cd", 20)
	if _, err := directFallbackVersion(directCommandOptions{DirectVersion: 5, CodeHash: pin}, contract); err == nil {
		t.Fatal("v5 command accepted an unbuilt release pin")
	}
	if version, err := directFallbackVersion(directCommandOptions{DirectVersion: 5, CodeHash: directV5CodeHashHex}, contract); err != nil || version != 5 {
		t.Fatalf("v5 command refused the pinned candidate code: %d, %v", version, err)
	}
	if _, err := directFallbackVersion(directCommandOptions{CodeHash: directV5CodeHashHex}, contract); err == nil {
		t.Fatal("pinned v5 code accepted without an explicit version")
	}
	if version, err := directFallbackVersionWithV5Pin(directCommandOptions{DirectVersion: 5, CodeHash: pin}, contract, pin); err != nil || version != 5 {
		t.Fatalf("v5 command refused the injected release pin: %d, %v", version, err)
	}
	if _, err := directFallbackVersionWithV5Pin(directCommandOptions{CodeHash: pin}, contract, pin); err == nil {
		t.Fatal("v5 code accepted without an explicit version")
	}
	if _, err := directFallbackVersionWithV5Pin(directCommandOptions{DirectVersion: 5, CodeHash: strings.Repeat("ef", 32)}, contract, pin); err == nil {
		t.Fatal("v5 command accepted another code hash")
	}

	c, _, _, _, seller, _, _, _, closeNode := directClientFixture(t)
	defer closeNode()
	caller, err := nodekeyAddressOf(seller)
	if err != nil {
		t.Fatal(err)
	}
	p := &preparedAction{Format: preparedDirectListFormat, DirectVersion: 5, Network: NetworkNamePinned,
		Contract: contract, Amount: "0", Author: caller}
	if err := validatePreparedDirectWithPins(c, p, seller, directV4CodeHashHex, ""); err == nil {
		t.Fatal("v5 prepared action accepted without a released code pin")
	}
	if err := validatePreparedDirectWithPins(c, p, seller, directV4CodeHashHex, strings.Repeat("ee", 32)); err == nil {
		t.Fatal("v5 prepared action accepted another deployed code hash")
	}
	// This test node reports a fixed code hash. The distinct v5 pin in a
	// real release is checked before the action-specific state can be signed.
	p.DirectVersion = 0
	if err := validatePreparedDirectWithPins(c, p, seller, directV4CodeHashHex, strings.Repeat("cd", 32)); err == nil || !strings.Contains(err.Error(), "v5 direct contract requires") {
		t.Fatalf("v5 code accepted a versionless prepared action: %v", err)
	}
}

func TestDirectV5StartStatementBindsSealedContentAndCanonicalSession(t *testing.T) {
	c, session, offer, _, _, _, _, _, closeNode := directClientFixture(t)
	defer closeNode()
	raw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-session.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Author   string `json:"author"`
		Sessions []struct {
			Inventory SessionAnchor `json:"inventory"`
			Verdict   SessionAnchor `json:"verdict"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil || len(fixture.Sessions) != 1 {
		t.Fatal("invalid public synthetic session fixture")
	}
	verified, err := sessionBundleFromAnchors(fixture.Author, &fixture.Sessions[0].Inventory, &fixture.Sessions[0].Verdict)
	if err != nil {
		t.Fatal(err)
	}
	*session = *verified // The test node's canonical headers use this pointer.
	raw, err = os.ReadFile("../../proofs/fast-content/dist/synthetic-v5-one-flip.json")
	if err != nil {
		t.Fatal(err)
	}
	var proof fastContentArtefact
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	journal, err := hex.DecodeString(proof.Journal)
	if err != nil || len(journal) != fastContentJournalBytes {
		t.Fatal("invalid synthetic content journal")
	}
	seal, err := hex.DecodeString(proof.Seal)
	if err != nil || len(seal) != fastContentSealBytes {
		t.Fatal("invalid synthetic content seal")
	}
	copy(offer.Inventory[:], journal[144:176])
	copy(offer.Keys[:], journal[241:273])
	contract := hex.EncodeToString(journal[36:56])
	caller := hex.EncodeToString(journal[56:76])
	wire, err := encodeFastRegistrationWire(session)
	if err != nil {
		t.Fatal(err)
	}
	arg, err := EncodeDirectStartFallbackV5(1, journal, seal, wire)
	if err != nil {
		t.Fatal(err)
	}
	p := &preparedAction{
		Format: preparedDirectStartFallbackV5Format, DirectVersion: 5,
		Operation: directStartFallbackOperation, DirectOrderID: 1, Contract: contract,
		Author: caller, Epoch: binary.BigEndian.Uint16(journal[76:78]),
		CoveredFlips:        uint32(binary.BigEndian.Uint16(journal[78:80])),
		InventoryCommitment: hex.EncodeToString(journal[144:176]),
		KeyCommitment:       hex.EncodeToString(journal[241:273]),
		ImageID:             proof.ImageID, ClaimDigest: proof.ClaimDigest, DirectSession: session,
	}
	if err := validateDirectStartFallbackV5Statement(c, p, offer, arg); err != nil {
		t.Fatalf("valid synthetic content and canonical session refused: %v", err)
	}
	wrongWire := bytes.Clone(arg)
	wrongWire[len(wrongWire)-1] ^= 1
	if err := validateDirectStartFallbackV5Statement(c, p, offer, wrongWire); err == nil {
		t.Fatal("changed canonical session wire was accepted")
	}
	wrongJournal := bytes.Clone(arg)
	wrongJournal[8+144] ^= 1
	if err := validateDirectStartFallbackV5Statement(c, p, offer, wrongJournal); err == nil {
		t.Fatal("changed sealed inventory was accepted")
	}
	wrongImage := *p
	wrongImage.ImageID = strings.Repeat("00", 32)
	if err := validateDirectStartFallbackV5Statement(c, &wrongImage, offer, arg); err == nil {
		t.Fatal("another guest image was accepted")
	}
	wrongOffer := *offer
	wrongOffer.Keys[0] ^= 1
	if err := validateDirectStartFallbackV5Statement(c, p, &wrongOffer, arg); err == nil {
		t.Fatal("another funded key commitment was accepted")
	}
	wrongVersion := *p
	wrongVersion.DirectVersion = 4
	if err := validateDirectStartFallbackV5Statement(c, &wrongVersion, offer, arg); err == nil {
		t.Fatal("v4 prepared action accepted v5 fallback bytes")
	}
}
