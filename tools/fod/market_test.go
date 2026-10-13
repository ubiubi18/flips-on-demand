package main

// The marketplace state reader's decoding, against records built to the layout
// `contracts/escrow/src/lib.rs` writes. The live read path is exercised against
// the lab chain in `docs/PROGRESS.md`.

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

// poolRecordBytes builds one `PoolBundle` record to the version 3 layout the
// contract writes (`contracts/escrow/src/lib.rs`), so the fixtures follow one
// definition instead of repeating offsets.
func poolRecordBytes(
	author, manifest, inventory []byte,
	units, beacon uint64,
	strikes uint32,
	nonce uint64,
	covered uint32,
) []byte {
	out := make([]byte, marketPoolValueBytes)
	copy(out[marketPoolAuthorOffset:marketPoolManifestOffset], author)
	copy(out[marketPoolManifestOffset:marketPoolInventoryOffset], manifest)
	copy(out[marketPoolInventoryOffset:marketPoolUnitsOffset], inventory)
	binary.BigEndian.PutUint64(out[marketPoolUnitsOffset:marketPoolBeaconOffset], units)
	binary.BigEndian.PutUint64(out[marketPoolBeaconOffset:marketPoolStrikesOffset], beacon)
	binary.BigEndian.PutUint32(out[marketPoolStrikesOffset:marketPoolNonceOffset], strikes)
	binary.BigEndian.PutUint64(out[marketPoolNonceOffset:marketPoolCoveredOffset], nonce)
	binary.BigEndian.PutUint32(out[marketPoolCoveredOffset:marketPoolValueBytes], covered)
	return out
}

func filled(value byte, count int) []byte {
	out := make([]byte, count)
	for index := range out {
		out[index] = value
	}
	return out
}

// poolStubCounters answers `contract_readData` for the one pool's running
// counters, so a stubbed registry can serve the reserve, the pending escrow and
// the market-wide unit count beside a pool record.
func poolStubCounters(key string) (string, bool) {
	switch key {
	case marketReserveKey:
		return "0x" + hex.EncodeToString(new(big.Int).SetUint64(50_000).Bytes()), true
	case marketPendingKey:
		return "0x00", true
	case marketPoolUnitsKey:
		return "0x" + hex.EncodeToString(new(big.Int).SetUint64(3).Bytes()), true
	}
	return "", false
}

// TestDecodePoolRecordReadsTheReserveAndQuotes mirrors the escrow's PoolBundle
// encoding and the contract's rounding: a big-endian record, a buy quote that
// rounds up, and a sell quote that rounds down.
func TestDecodePoolRecordReadsTheReserveAndQuotes(t *testing.T) {
	bundle := strings.Repeat("ab", 32)
	record := func(units uint64) []byte {
		return poolRecordBytes(filled(0x33, 20), filled(0x44, 32), filled(0x55, 32), units, 10_860_138, 2, 11, 5)
	}

	// The one pool's counters are read beside the record (`buildMarketState`).
	reserve := big.NewInt(50_000)
	pending := big.NewInt(12_500)

	listing, err := decodePoolRecord(bundle, record(6), reserve, pending, 6)
	if err != nil {
		t.Fatalf("decode the pool record: %v", err)
	}
	if listing.Author != strings.Repeat("33", 20) || listing.ManifestCommitment != strings.Repeat("44", 32) {
		t.Fatalf("the author and manifest read back as %s and %s", listing.Author, listing.ManifestCommitment)
	}
	if listing.InventoryCommitment != strings.Repeat("55", 32) {
		t.Fatalf("the inventory commitment reads %s", listing.InventoryCommitment)
	}
	// The buyer escrow is published beside the reserve: it is the money a
	// settlement moves and a refund releases (`SPEC.md` 5.1).
	if listing.Pending != "12500" {
		t.Fatalf("the pending escrow reads %s", listing.Pending)
	}
	if listing.Reserve != "50000" || listing.Units != 6 || listing.PoolUnits != 6 {
		t.Fatalf("the pool reads %s against %d units of %d", listing.Reserve, listing.Units, listing.PoolUnits)
	}
	// The covered flip count is the registration statement's own number, read
	// straight out of the record (`SPEC.md` 5.5).
	if listing.FlipCount != 5 {
		t.Fatalf("the record's covered flip count reads %d", listing.FlipCount)
	}
	if listing.BuyQuote != "10000" || listing.SellQuote != "7142" {
		t.Fatalf("the quotes read %s and %s", listing.BuyQuote, listing.SellQuote)
	}
	if listing.BeaconBlock != 10_860_138 || listing.Strikes != 2 {
		t.Fatalf("the beacon reads %d with %d strikes", listing.BeaconBlock, listing.Strikes)
	}
	if listing.Nonce != 11 {
		t.Fatalf("the buy nonce reads %d, expected 11", listing.Nonce)
	}

	rounded, err := decodePoolRecord(bundle, record(3), big.NewInt(10), big.NewInt(0), 3)
	if err != nil {
		t.Fatalf("decode the rounding record: %v", err)
	}
	if rounded.BuyQuote != "5" || rounded.SellQuote != "2" {
		t.Fatalf("rounding reads %s and %s", rounded.BuyQuote, rounded.SellQuote)
	}

	single, err := decodePoolRecord(bundle, record(1), big.NewInt(50_000), big.NewInt(0), 1)
	if err != nil {
		t.Fatalf("decode the single-unit record: %v", err)
	}
	if single.BuyQuote != "" || single.SellQuote != "25000" {
		t.Fatalf("a one-unit bundle quotes %q and %q", single.BuyQuote, single.SellQuote)
	}

	if _, err := decodePoolRecord(bundle, record(6)[:marketPoolValueBytes-1], reserve, pending, 6); err == nil {
		t.Fatal("a short pool record was accepted")
	}
}

// TestDecodePoolOrderRecordReadsTheDeadlines mirrors the escrow's 150-byte
// PoolOrder encoding and the deadlines the contract enforces on it.
func TestDecodePoolOrderRecordReadsTheDeadlines(t *testing.T) {
	raw := make([]byte, marketPoolOrderBytes)
	for i := 0; i < 32; i++ {
		raw[i] = 0xab
	}
	for i := 32; i < 52; i++ {
		raw[i] = 0xcd
	}
	for i := 52; i < 117; i++ {
		raw[i] = 0x07
	}
	new(big.Int).SetUint64(25_000).FillBytes(raw[117:133])
	binary.BigEndian.PutUint64(raw[133:141], 1_000)
	binary.BigEndian.PutUint64(raw[141:149], 3)
	raw[149] = 2

	order, err := decodePoolOrderRecord(7, raw)
	if err != nil {
		t.Fatalf("decode the pool order: %v", err)
	}
	if order.OrderID != 7 || order.Bundle != strings.Repeat("ab", 32) || order.Buyer != strings.Repeat("cd", 20) {
		t.Fatalf("the order reads back as %+v", order)
	}
	if order.Paid != "25000" || order.FundedAt != 1_000 {
		t.Fatalf("the order's money reads as %s from block %d", order.Paid, order.FundedAt)
	}
	// Refund-only starts at funded+3; all consumers share that boundary.
	if order.Deadline != 1_003 || order.RefundAt != 1_003 {
		t.Fatalf("the deadlines read as %d and %d", order.Deadline, order.RefundAt)
	}
	if order.State != "delivered" || !order.Delivered {
		t.Fatalf("the state reads as %q (delivered %v)", order.State, order.Delivered)
	}

	raw[149] = 9
	if _, err := decodePoolOrderRecord(7, raw); err == nil {
		t.Fatal("an unknown state was accepted")
	}
	if _, err := decodePoolOrderRecord(7, raw[:marketPoolOrderBytes-1]); err == nil {
		t.Fatal("a short order record was accepted")
	}
}

// The provenance the state carries comes straight out of a real registration
// journal, so the link between a stored bundle and its author is checked against
// the artefact the escrow accepts rather than against a fixture this file wrote.
func TestProvenanceFromTheSealedRegistrationArtefact(t *testing.T) {
	provenanceByCommitment, err := loadProvenance([]string{sealedRegistrationArtefact})
	if err != nil {
		t.Fatalf("read the committed sealed registration artefact: %v", err)
	}
	if len(provenanceByCommitment) != 1 {
		t.Fatalf("the artefact produced %d provenance entries", len(provenanceByCommitment))
	}
	for commitment, item := range provenanceByCommitment {
		if len(commitment) != 64 || !isHex(commitment, 32) {
			t.Fatalf("the commitment %q is not 32 bytes of hex", commitment)
		}
		if !isHex(item.Author, addressBytes) || item.Epoch == 0 || item.FlipCount == 0 {
			t.Fatalf("provenance decoded as %+v", item)
		}
		if !isHex(item.ManifestCommitment, 32) {
			t.Fatalf("the manifest commitment %q is not 32 bytes of hex", item.ManifestCommitment)
		}
	}
}

// TestPoolListingCarriesRegistrationProvenance: the pool record has the author
// and manifest, and a registration artefact adds the epoch and flip count the
// interface shows beside the quote.
func TestPoolListingCarriesRegistrationProvenance(t *testing.T) {
	bundle := strings.Repeat("ab", 32)
	listing := &marketPoolListing{Bundle: bundle, Author: strings.Repeat("33", 20)}
	withPoolProvenance(listing, map[string]provenance{
		bundle: {
			Author:    strings.Repeat("44", 20),
			Epoch:     228,
			FlipCount: 5,
		},
	})
	if listing.Epoch != 228 || listing.FlipCount != 5 {
		t.Fatalf("the provenance reads epoch %d, %d flips", listing.Epoch, listing.FlipCount)
	}
	if listing.Author != strings.Repeat("33", 20) {
		t.Fatalf("the pool record's own author was replaced by %s", listing.Author)
	}
	missing := &marketPoolListing{Bundle: strings.Repeat("cd", 32), Author: strings.Repeat("55", 20)}
	withPoolProvenance(missing, map[string]provenance{})
	if missing.Epoch != 0 || missing.FlipCount != 0 {
		t.Fatalf("an unmatched listing gained provenance %+v", missing)
	}
}

// Two reads of the same chain view have to agree on the digest, and the only
// field allowed to differ between them is the observation timestamp.
func TestMarketStateDigestIgnoresOnlyTheObservationTime(t *testing.T) {
	base := marketState{
		Format:      marketStateFormat,
		Version:     marketStateVersion,
		Network:     "idena-local",
		Contract:    "aa",
		CodeHash:    "bb",
		BlockHeight: 7234,
		ObservedAt:  "2026-09-30T15:00:00Z",
		Source:      "registry storage",
		Credits:     []marketCredit{},
	}
	first := marketStateDigest(&base)
	if len(first) != 64 || marketStateDigest(&base) != first {
		t.Fatalf("the digest of one state must be a stable sha256 value, got %q", first)
	}

	later := base
	later.ObservedAt = "2026-09-30T16:00:00Z"
	if marketStateDigest(&later) != first {
		t.Fatalf("the observation time must not change the digest")
	}
	// A digest already stored on the state does not feed back into its own input.
	sealed := base
	sealed.StateDigest = first
	if marketStateDigest(&sealed) != first {
		t.Fatalf("the stored digest must not change the recomputed one")
	}
	// And a reader that only has the written file reproduces it: marshalling the
	// state and reading it back has to yield the same digest.
	written, err := json.Marshal(sealed)
	if err != nil {
		t.Fatalf("encode the state: %v", err)
	}
	var parsed marketState
	if err := json.Unmarshal(written, &parsed); err != nil {
		t.Fatalf("decode the state: %v", err)
	}
	if marketStateDigest(&parsed) != first {
		t.Fatalf("a reader must recompute the digest from the file it was handed")
	}

	// Anything else about the chain view does change it.
	changedBlock := base
	changedBlock.BlockHeight = 7235
	changedCode := base
	changedCode.CodeHash = "cc"
	changedContract := base
	changedContract.Contract = "dd"
	// The state the digest exists for - who is owed what and what is listed -
	// has to change it too, or two operators with different markets would agree.
	changedCredit := base
	changedCredit.Credits = []marketCredit{{Address: "aa", Amount: "1"}}
	changedListing := base
	changedListing.Pool = []marketPoolListing{{Bundle: "bb", Units: 2, Pending: "1"}}
	changedOrder := base
	changedOrder.PoolOrders = []marketPoolOrder{{OrderID: 1, State: "delivered"}}
	for name, changed := range map[string]marketState{
		"block height": changedBlock,
		"code hash":    changedCode,
		"contract":     changedContract,
		"credits":      changedCredit,
		"listing":      changedListing,
		"order":        changedOrder,
	} {
		if marketStateDigest(&changed) == first {
			t.Fatalf("a different %s must change the digest", name)
		}
	}
}
