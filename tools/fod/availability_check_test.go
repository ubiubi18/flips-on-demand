package main

// The buyer's repeat of the availability check: every CID the advisory names
// gets a verdict, and a purchase is refused unless all of them rebuild their
// CIDs from bytes this buyer's own node served (`SPEC.md` 5.8).

import (
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

func retainedTestCheck(blob []byte) *AvailabilityCheck {
	author := strings.Repeat("33", addressBytes)
	cid := cidRawSha256(blob)
	return &AvailabilityCheck{CheckedAt: checkedAtNow(), Author: author, Epoch: 228, Binding: bindingCommitted,
		CoveredFlips: 1, VerifiedFlips: 1, InventoryCommitment: inventoryCommitment(author, 228, []string{cid}),
		Blobs: []RetainedFlip{{CID: cid, Hex: hex.EncodeToString(blob)}}}
}

func TestCIDVerificationNeverTrustsTheServingPeersAttestation(t *testing.T) {
	calls := 0
	for _, cid := range []string{planBDagPubCID(), "unsupported-cid"} {
		ok, _ := verifyBlobCID(cid, []byte("substituted bytes"), func([]byte) (string, error) { calls++; return cid, nil })
		if ok {
			t.Fatal("a peer's false CID assertion authenticated substituted bytes")
		}
	}
	if calls != 0 {
		t.Fatal("the serving peer was asked to authorize its bytes")
	}
}

func TestRetainedPurchaseSurvivesServerRemovalButRefusesEditedEvidence(t *testing.T) {
	node, bundle, path := availableBuyFixture(t)
	identity, _ := crypto.GenerateKey()
	delivery, _ := crypto.GenerateKey()
	buyer, _ := nodekeyAddressOf(identity)
	p, err := preparePoolBuyFromRequest(node, &actionRequest{Network: "idena-local", Contract: testContract},
		&buyRequestArguments{BundleID: bundle, ExpectedQuote: "25000", Buyer: buyer}, identity, delivery, path)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "purchase.json")
	if err := writePrivateJSON(file, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadPreparedAction(file)
	if err != nil {
		t.Fatal(err)
	}
	// This replacement endpoint knows canonical storage but serves no blobs.
	listing, err := registryPoolBundle(node, testContract, bundle)
	if err != nil {
		t.Fatal(err)
	}
	record := poolRecordBytes(mustDecode(listing.Author), mustDecode(listing.ManifestCommitment), mustDecode(listing.InventoryCommitment), 3, 1000, 0, 7, 1)
	storageOnly := poolBuyStub(t, bundle, record, nil)
	if _, _, err := BuildTransaction(loaded, identity, 1, 228, big.NewInt(1_000_000), big.NewInt(0)); err == nil {
		t.Fatal("a buy skipped canonical validation")
	}
	if err := validatePreparedBuy(storageOnly, loaded); err != nil {
		t.Fatal(err)
	}
	// The content check is independent of the CLI's approved target check.
	loaded.buyTargetValidated = loaded.Contract
	if _, _, err := BuildTransaction(loaded, identity, 1, 228, big.NewInt(1_000_000), big.NewInt(0)); err != nil {
		t.Fatal(err)
	}
	oldContract := loaded.Contract
	loaded.Contract = strings.Repeat("cd", addressBytes)
	if _, _, err := BuildTransaction(loaded, identity, 1, 228, big.NewInt(1_000_000), big.NewInt(0)); err == nil {
		t.Fatal("a validated buy remained signable after its target changed")
	}
	loaded.Contract = oldContract
	open := make([]byte, 8*maxOpenPoolOrders)
	for index := 0; index < maxOpenPoolOrders; index++ {
		binary.BigEndian.PutUint64(open[index*8:(index+1)*8], uint64(index+1))
	}
	full := poolBuyStub(t, bundle, record, nil, map[string][]byte{"popen/" + bundle: open})
	if err := validatePreparedBuy(full, loaded); err == nil || !strings.Contains(err.Error(), "eight open purchases") {
		t.Fatalf("signing accepted a now-full bundle: %v", err)
	}
	if _, _, err := BuildTransaction(loaded, identity, 1, 228, big.NewInt(1_000_000), big.NewInt(0)); err == nil {
		t.Fatal("a stale buy remained signable after the open-order recheck failed")
	}
	for _, mutate := range []func(*preparedAction){
		func(p *preparedAction) { p.Availability.Blobs = nil },
		func(p *preparedAction) { p.Availability.Blobs[0].Hex = "00" },
		func(p *preparedAction) { p.Availability.InventoryCommitment = strings.Repeat("00", 32) },
		func(p *preparedAction) { p.Availability.CheckedAt = time.Now().Add(-time.Hour).Format(time.RFC3339) },
		func(p *preparedAction) { p.Bundle = strings.Repeat("cd", 32) },
		func(p *preparedAction) { p.Nonce++ },
		func(p *preparedAction) { p.Amount = "1" },
	} {
		candidate, err := readPreparedAction(file)
		if err != nil {
			t.Fatal(err)
		}
		mutate(candidate)
		if err := validatePreparedBuy(storageOnly, candidate); err == nil {
			t.Fatal("edited purchase evidence was accepted")
		}
	}
	// Recovery does not depend on a short prepayment freshness window.
	loaded.Availability.CheckedAt = time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	if err := writePrivateJSON(file, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := readRetainedPurchase(file); err != nil {
		t.Fatal(err)
	}
}

func fixedFetch(blobs map[string][]byte) bodyFetcher {
	return func(cid string) ([]byte, string, error) {
		blob, ok := blobs[cid]
		if !ok {
			return nil, "the buyer's node", errNotInRegistry
		}
		return blob, "the buyer's node", nil
	}
}

func TestRecheckAvailabilityVerdictsEveryFlip(t *testing.T) {
	good := []byte("the ciphertext this CID commits to")
	other := []byte("bytes that commit to another CID")
	goodCID := cidRawSha256(good)
	otherCID := cidRawSha256(other)
	advisory := &AvailabilityAdvisory{
		Author: strings.Repeat("ab", addressBytes),
		Epoch:  228,
		Flips: []AvailabilityFlip{
			{CID: goodCID},
			{CID: otherCID},
			{CID: cidRawSha256([]byte("nothing serves these bytes"))},
			{CID: goodCID},
		},
	}
	// The node serves the right bytes for one CID and the wrong bytes for
	// another; the third CID is not served at all.
	check := recheckAvailability(advisory, fixedFetch(map[string][]byte{goodCID: good, otherCID: good}), nil)
	if check.CoveredFlips != 4 || check.VerifiedFlips != 1 {
		t.Fatalf("the check reads %d/%d verified", check.VerifiedFlips, check.CoveredFlips)
	}
	if check.Complete() {
		t.Fatal("a check with failures reports complete")
	}
	if len(check.Failures) != 3 {
		t.Fatalf("the failures read %v", check.Failures)
	}
	if check.CheckedAt == "" {
		t.Fatal("the check carries no time")
	}
	// The record names what the CIDs are: the seller's claim, repeated.
	if check.Binding != bindingAdvisory {
		t.Fatalf("the check binds the content as %q", check.Binding)
	}
	// The bytes served for the second CID rebuild a different CID, which is a
	// failure with the reason, not a silent pass.
	joined := strings.Join(check.Failures, " | ")
	if !strings.Contains(joined, "another CID") && !strings.Contains(joined, "not the cid") {
		t.Fatalf("the wrong-bytes failure does not say what happened: %s", joined)
	}
	if !strings.Contains(joined, "twice") {
		t.Fatalf("a repeated CID is not reported: %s", joined)
	}
}

func TestRecheckAvailabilityRefusesAnEmptyAdvisory(t *testing.T) {
	check := recheckAvailability(&AvailabilityAdvisory{Author: strings.Repeat("ab", addressBytes), Epoch: 228}, fixedFetch(nil), nil)
	if check.Complete() || len(check.Failures) == 0 {
		t.Fatalf("an advisory that names no flips reads %+v", check)
	}
}
