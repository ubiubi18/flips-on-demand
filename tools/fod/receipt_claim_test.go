package main

// The claim digest the CLI refuses receipts by, pinned from both ends: the
// escrow's own frozen delivery fixture, and the registration artefact this
// release ships.

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestClaimDigestMatchesTheActiveDeliveryArtefact(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/delivery/dist/delivery-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var artefact struct {
		ImageID     string `json:"image_id"`
		Journal     string `json:"journal"`
		ClaimDigest string `json:"claim_digest"`
	}
	if err := json.Unmarshal(raw, &artefact); err != nil {
		t.Fatal(err)
	}
	if artefact.ImageID != deliveryImageIDHex {
		t.Fatalf("active delivery image %s does not match CLI pin %s", artefact.ImageID, deliveryImageIDHex)
	}
	journal, err := hex.DecodeString(artefact.Journal)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := claimDigestFromHex(deliveryImageIDHex, journal)
	if err != nil || hex.EncodeToString(digest[:]) != artefact.ClaimDigest {
		t.Fatalf("active delivery claim mismatch: %x, %v", digest, err)
	}
}

func TestClaimDigestMatchesTheEscrowsOwnFixture(t *testing.T) {
	// `contracts/escrow/src/verifier.rs`, the proved delivery run.
	const (
		deliveryImageID = "2a4f2f7b21fe44fd418f158608083ffa70ddf2144ddec6fc6c2f683aaaa524e4"
		deliveryJournal = "00000001111111111111111111111111111111111111111100000000000000070000000000000001" +
			"22222222222222222222222222222222222222223333333333333333333333333333333333333333" +
			"04c7e8ce2d833482ce249b86b7b9f739ac441e33e3b674d28a7de2d5e96d3bb2c4c3a1ea30571f" +
			"583598451031e4566924435c6de709adcca76b6fdbe787c65e3193c72e262384abc773825769d4b1" +
			"a0a9b749d4c4921367105c023c4eaf21329e28925793e6950fee60fe106790547a2fa96d145dd31f24" +
			"215be872341af22f8b"
		deliveryClaimDigest = "7e509bfef97049579c5d5008ecf25b6a4311a7f291fd8aa8c173010a05fb7688"
	)
	journal, err := hex.DecodeString(deliveryJournal)
	if err != nil {
		t.Fatalf("decode the fixture: %v", err)
	}
	digest, err := claimDigestFromHex(deliveryImageID, journal)
	if err != nil {
		t.Fatalf("compute the claim: %v", err)
	}
	if hex.EncodeToString(digest[:]) != deliveryClaimDigest {
		t.Fatalf("the claim digest is %x, the escrow's fixture says %s", digest, deliveryClaimDigest)
	}
	// A tampered journal must move the claim, which is what makes the check a
	// check rather than a restatement.
	journal[10] ^= 0x01
	moved, err := claimDigestFromHex(deliveryImageID, journal)
	if err != nil {
		t.Fatalf("compute the tampered claim: %v", err)
	}
	if hex.EncodeToString(moved[:]) == deliveryClaimDigest {
		t.Fatal("a tampered journal produced the same claim digest")
	}
}

func TestClaimDigestMatchesTheProvedRegistrationArtefact(t *testing.T) {
	artefact := loadSealedRegistration(t)
	journal, err := hex.DecodeString(strings.TrimPrefix(artefact.Journal, "0x"))
	if err != nil {
		t.Fatalf("decode the journal: %v", err)
	}
	if !strings.EqualFold(strings.TrimPrefix(artefact.ImageID, "0x"), registrationImageIDHex) {
		t.Fatalf("the artefact was proved by %s, this release pins %s", artefact.ImageID, registrationImageIDHex)
	}
	digest, err := claimDigestFromHex(registrationImageIDHex, journal)
	if err != nil {
		t.Fatalf("compute the claim: %v", err)
	}
	if !strings.EqualFold(hex.EncodeToString(digest[:]), strings.TrimPrefix(artefact.ClaimDigest, "0x")) {
		t.Fatalf("the computed claim is %x, the artefact records %s", digest, artefact.ClaimDigest)
	}
}
