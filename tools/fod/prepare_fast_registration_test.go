package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestAssembleFastRegistrationBindsEveryFlipAndSession(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/session-core/fixtures/session-bundle-vector.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Author   string   `json:"author"`
		Manifest []string `json:"manifest"`
		Sessions []struct {
			Inventory SessionAnchor `json:"inventory"`
			Verdict   SessionAnchor `json:"verdict"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil || len(fixture.Sessions) == 0 {
		t.Fatal("invalid public session fixture")
	}
	session, err := sessionBundleFromAnchors(fixture.Author, &fixture.Sessions[0].Inventory, &fixture.Sessions[0].Verdict)
	if err != nil {
		t.Fatal(err)
	}
	originals := slices.Clone(session.Manifest)
	slices.Sort(originals)
	items := make([]fastCopyPair, len(originals))
	for i, cid := range originals {
		items[i] = fastCopyPair{OriginalCID: cid, CopyCID: cid}
	}
	copies, err := fastCopiesCommitment(items)
	if err != nil {
		t.Fatal(err)
	}
	contract := strings.Repeat("22", 20)
	image := fastContentImageIDHex
	journal := make([]byte, fastContentJournalBytes)
	binary.BigEndian.PutUint32(journal[:4], 1)
	copy(journal[4:36], mustDecodeFastHex(t, labelDigest(NetworkNamePinned)))
	copy(journal[36:56], mustDecodeFastHex(t, contract))
	copy(journal[56:76], mustDecodeFastHex(t, fixture.Author))
	binary.BigEndian.PutUint16(journal[76:78], session.Epoch)
	binary.BigEndian.PutUint16(journal[78:80], uint16(len(items)))
	copy(journal[80:112], mustDecodeFastHex(t, labelDigest(ConsensusVersionPinned)))
	copy(journal[112:144], mustDecodeFastHex(t, labelDigest(EligibilityPolicyV1)))
	copy(journal[144:176], mustDecodeFastHex(t, inventoryCommitment(fixture.Author, session.Epoch, originals)))
	copy(journal[176:208], mustDecodeFastHex(t, copies))
	claim, err := claimDigestFromHex(image, journal)
	if err != nil {
		t.Fatal(err)
	}
	proof := &fastContentArtefact{
		Format:      "fod:fast-content-sealed:v1",
		Statement:   fastContentStatement,
		Journal:     hex.EncodeToString(journal),
		Seal:        strings.Repeat("00", fastContentSealBytes),
		SealBytes:   fastContentSealBytes,
		ClaimDigest: hex.EncodeToString(claim[:]),
		ImageID:     image,
		Items:       items,
	}
	prepared, err := assembleFastRegistration(NetworkNamePinned, contract, proof, session, image)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := encodeFastRegistrationWire(session)
	if err != nil {
		t.Fatal(err)
	}
	calldata := mustDecodeFastHex(t, prepared.Calldata)
	if got := int(binary.BigEndian.Uint16(calldata[fastContentJournalBytes+fastContentSealBytes:])); got != len(wire) ||
		!slices.Equal(calldata[fastContentJournalBytes+fastContentSealBytes+2:], wire) {
		t.Fatal("prepared call does not carry the exact public session witness")
	}
	bundle := sha256.Sum256(journal)
	if prepared.Bundle != hex.EncodeToString(bundle[:]) {
		t.Fatal("prepared bundle ID differs from the sealed journal")
	}
	manifest := sha256.New()
	manifest.Write([]byte("fod:fast:manifest:v1"))
	manifest.Write(journal)
	manifest.Write(wire)
	listing := &marketPoolListing{
		Author: fixture.Author, InventoryCommitment: prepared.InventoryCommitment,
		ManifestCommitment: hex.EncodeToString(manifest.Sum(nil)), FlipCount: uint32(len(items)),
	}
	fast := &fastBundleRecord{
		Epoch: session.Epoch, Count: uint16(len(items)), Copies: copies,
		Point: hex.EncodeToString(journal[208:241]), OriginalKeys: hex.EncodeToString(journal[241:273]),
	}
	if err := validateFastChainCertificate(contract, prepared.Bundle, listing, fast, calldata, image); err != nil {
		t.Fatalf("canonical certificate binding: %v", err)
	}
	if err := validateFastPairs(proof, calldata, listing, fast, image); err != nil {
		t.Fatalf("public copy binding: %v", err)
	}
	tampered := slices.Clone(calldata)
	tampered[len(tampered)-1] ^= 1
	if err := validateFastChainCertificate(contract, prepared.Bundle, listing, fast, tampered, image); err == nil {
		t.Fatal("changed session witness was accepted")
	}
	changed := *proof
	changed.Items = slices.Clone(items)
	changed.Items[0].CopyCID = cidRawSha256([]byte("changed copy"))
	if _, err := assembleFastRegistration(NetworkNamePinned, contract, &changed, session, image); err == nil {
		t.Fatal("changed copy pair was accepted")
	}
	if err := validateFastPairs(&changed, calldata, listing, fast, image); err == nil {
		t.Fatal("changed buyer copy list was accepted")
	}
	if _, err := assembleFastRegistration(NetworkNamePinned, contract, proof, session, strings.Repeat("0", 64)); err == nil {
		t.Fatal("unpinned guest image was accepted")
	}
}

func TestFastContentImagePinMatchesSealedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-one-flip.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ImageID string `json:"image_id"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(fastContentImageIDHex, fixture.ImageID) {
		t.Fatal("fast content CLI image pin differs from the sealed fixture")
	}
}

func mustDecodeFastHex(t *testing.T, value string) []byte {
	t.Helper()
	value = strings.TrimPrefix(value, "0x")
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
