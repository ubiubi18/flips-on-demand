package main

// The marketplace state the interface imports (`SPEC.md` 4.5), read from the
// registry's own storage.
//
// A read-only *call* is not available for this: the pinned node's
// `contract_readonlyCall` only instantiates embedded contracts, so a custom
// registry answers `unknown contract` (`docs/PROGRESS.md`, the local-chain
// entry). The storage itself is readable: `contract_iterateMap` walks a key
// prefix and returns keys and values as hex, and the withdrawal total lives
// under the ASCII key `credits0`.
//
// The escrow stores the pool's money and commitments, not provenance. Author,
// epoch, flip count, and manifest commitment come from the registration
// journals, which the caller supplies as the artefacts a registration produced;
// the registry is keyed by the same key commitment those journals carry.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	marketStateFormat  = "fod.marketplace-state"
	marketStateVersion = 1

	// Storage prefixes and ASCII counter keys of the registry
	// (`contracts/escrow/src/lib.rs`).
	marketCreditPrefix    = "credit/"
	marketPoolPrefix      = "pool/"
	marketPoolOrderPrefix = "porder/"
	marketCreditsKey      = "credits0"
	// The one pool's running counters (`running_total` writes prefix + "0"):
	// the reserve, the pending buyer escrow and the market-wide sale units.
	marketReserveKey   = "reserve0"
	marketPendingKey   = "pending0"
	marketPoolUnitsKey = "units0"

	// Fixed record sizes from the same file.
	marketCreditValueBytes = 16
	// author (20) + manifest (32) + inventory (32) + units (8) + beacon (8)
	// + strikes (4) + nonce (8) + covered (4): the `PoolBundle` layout after
	// the owner's "one global pool" answer of 2026-09-29 moved the reserve, the
	// sale units and the buyer escrow into the counters above.
	marketPoolValueBytes = 116
	// The same record's fields, in the order the contract writes them
	// (`contracts/escrow/src/lib.rs`).
	marketPoolAuthorOffset    = 0
	marketPoolManifestOffset  = 20
	marketPoolInventoryOffset = marketPoolManifestOffset + 32
	marketPoolUnitsOffset     = marketPoolInventoryOffset + 32
	marketPoolBeaconOffset    = marketPoolUnitsOffset + 8
	marketPoolStrikesOffset   = marketPoolBeaconOffset + 8
	marketPoolNonceOffset     = marketPoolStrikesOffset + 4
	marketPoolCoveredOffset   = marketPoolNonceOffset + 8
	marketPoolOrderBytes      = 150

	// The hard boundary: unilateral delivery is strictly before funded+3,
	// and the first refund block is funded+3 (no grace period).
	marketDeadlineBlocks = 3
	marketRefundBlocks   = 0

	marketPageLimit = 200
)

type marketCredit struct {
	Address string `json:"address"`
	Amount  string `json:"amount"`
}

// marketPoolListing is one bundle's pool state as `SPEC.md` 5.1 defines it: the
// reserve against the sale units, the beacon, the strikes, and both quotes.
type marketPoolListing struct {
	Bundle             string `json:"bundle"`
	Fast               bool   `json:"fast,omitempty"`
	Author             string `json:"author"`
	ManifestCommitment string `json:"manifestCommitment"`
	// InventoryCommitment is the statement's own commitment over the bundle's
	// CIDs, published by the pool record. A buyer who holds a candidate CID set
	// computes the same value and compares it, which is what makes the content
	// check a proof about the committed inventory rather than an observation of
	// the seller's advisory.
	InventoryCommitment string `json:"inventoryCommitment"`
	// Pending is the buyer escrow the bundle holds for orders that have neither
	// settled nor refunded (`SPEC.md` 5.1). It is not part of the reserve.
	Pending string `json:"pending"`
	// Epoch and FlipCount come from a registration artefact when the caller
	// names one; the pool record itself carries only the author and manifest.
	Epoch     uint16 `json:"epoch,omitempty"`
	FlipCount uint32 `json:"flipCount,omitempty"`
	Reserve   string `json:"reserve"`
	Units     uint64 `json:"units"`
	// PoolUnits is the one pool's market-wide sale-unit count; Units above is
	// this bundle's own remaining inventory.
	PoolUnits   uint64 `json:"poolUnits,omitempty"`
	BeaconBlock uint64 `json:"beaconBlock"`
	Strikes     uint32 `json:"strikes"`
	// Nonce is what the buyer's delivery-key possession proof has to name.
	// It is public chain state, and the client needs it to prepare a buy.
	Nonce     uint64 `json:"nonce"`
	BuyQuote  string `json:"buyQuote,omitempty"`
	SellQuote string `json:"sellQuote,omitempty"`
	// Availability is the last off-chain blob check the caller supplied. It is
	// advisory and never part of the registry's own state.
	Availability *marketAvailability `json:"availability,omitempty"`
}

type marketAvailability struct {
	CheckedAt     string `json:"checkedAt"`
	Source        string `json:"source,omitempty"`
	CoveredFlips  int    `json:"coveredFlips"`
	VerifiedFlips int    `json:"verifiedFlips"`
	Complete      bool   `json:"complete"`
}

// loadAvailability reads the advisory files and indexes them by author/epoch,
// the identity a pool listing's provenance carries.
func loadAvailability(paths []string) (map[string]marketAvailability, error) {
	out := make(map[string]marketAvailability)
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		advisory, err := readAvailability(path)
		if err != nil {
			return nil, err
		}
		out[availabilityKey(advisory.Author, advisory.Epoch)] = marketAvailability{
			CheckedAt:     advisory.CheckedAt,
			Source:        advisory.Source,
			CoveredFlips:  advisory.CoveredFlips,
			VerifiedFlips: advisory.VerifiedFlips,
			Complete:      advisory.Complete,
		}
	}
	return out, nil
}

// attachAvailability joins a listing to its last check when the provenance
// supplies the epoch the advisory is keyed by.
func attachAvailability(listing *marketPoolListing, advisories map[string]marketAvailability) {
	if listing.Epoch == 0 {
		return
	}
	if item, ok := advisories[availabilityKey(listing.Author, listing.Epoch)]; ok {
		copy := item
		listing.Availability = &copy
	}
}

// withPoolProvenance adds the epoch and flip count a registration artefact
// supplies, so the interface can show what a pool listing contains.
func withPoolProvenance(listing *marketPoolListing, provenanceByCommitment map[string]provenance) {
	item, ok := provenanceByCommitment[listing.Bundle]
	if !ok {
		return
	}
	listing.Epoch = item.Epoch
	// The record's count comes from the registration statement, so it wins; an
	// artefact only fills in what the record cannot carry (`SPEC.md` 5.5).
	if listing.FlipCount == 0 {
		listing.FlipCount = item.FlipCount
	}
	if listing.Author == "" {
		listing.Author = item.Author
	}
}

type marketViewer struct {
	Address string `json:"address"`
	Credits string `json:"credits"`
}

type marketState struct {
	Format      string `json:"format"`
	Version     int    `json:"version"`
	Network     string `json:"network"`
	Contract    string `json:"contract"`
	CodeHash    string `json:"codeHash,omitempty"`
	BlockHeight uint64 `json:"blockHeight"`
	// BlockSeconds is the node's observed interval between the last two blocks,
	// when it reports one. The interface turns a block deadline into a
	// wall-clock estimate with it; a chain that does not report an interval
	// leaves it zero and the interface says the time depends on the chain.
	BlockSeconds int64  `json:"blockSeconds,omitempty"`
	ObservedAt   string `json:"observedAt"`
	// StateDigest is the comparison two operators make: the same chain view has
	// to produce the same digest, and the observation timestamp is the one field
	// that legitimately differs between two reads. The canonical form is the
	// state as this tool writes it - the schema's own key order, no whitespace -
	// with `observedAt` and `stateDigest` removed, so a reader that reserialises
	// the file it was handed without those two fields recomputes this value.
	StateDigest string         `json:"stateDigest,omitempty"`
	Source      string         `json:"source"`
	Viewer      *marketViewer  `json:"viewer,omitempty"`
	Credits     []marketCredit `json:"credits"`
	// Pool carries the pool listings, and PoolOrders the client's trade list
	// and the delivery desk's work list.
	Pool       []marketPoolListing `json:"pool,omitempty"`
	PoolOrders []marketPoolOrder   `json:"poolOrders,omitempty"`
}

// provenance is what a registration journal adds to a stored bundle.
type provenance struct {
	Author             string
	Epoch              uint16
	FlipCount          uint32
	ManifestCommitment string
}

type mapItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type mapPage struct {
	Items             []*mapItem `json:"items"`
	ContinuationToken *string    `json:"continuationToken"`
}

func decodeHexField(value string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(value, "0x"))
}

// readPrefix walks one storage prefix, following the continuation token.
func readPrefix(c *client, contract, prefix string) (map[string][]byte, error) {
	out := make(map[string][]byte)
	var token any
	for {
		var page mapPage
		params := []any{contract, prefix, token, "hex", "hex", marketPageLimit}
		if err := c.call("contract_iterateMap", params, &page); err != nil {
			return nil, fmt.Errorf("read %s: %w", prefix, err)
		}
		for _, item := range page.Items {
			key, err := decodeHexField(item.Key)
			if err != nil {
				return nil, fmt.Errorf("read %s: key %q: %w", prefix, item.Key, err)
			}
			value, err := decodeHexField(item.Value)
			if err != nil {
				return nil, fmt.Errorf("read %s: value for %q: %w", prefix, item.Key, err)
			}
			out[hex.EncodeToString(key)] = value
		}
		// The node answers `0x` when the prefix is exhausted, which is an empty
		// token rather than a key to continue from.
		if page.ContinuationToken == nil || strings.TrimPrefix(*page.ContinuationToken, "0x") == "" {
			return out, nil
		}
		token = *page.ContinuationToken
	}
}

// readCounter reads one ASCII counter; a missing value is zero.
func readCounter(c *client, contract, key string) (*big.Int, error) {
	var raw string
	if err := c.call("contract_readData", []any{contract, key, "hex"}, &raw); err != nil {
		if strings.Contains(err.Error(), "data is nil") {
			return big.NewInt(0), nil
		}
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	value, err := decodeHexField(raw)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	return new(big.Int).SetBytes(value), nil
}

// decodePoolRecord reads the escrow's `PoolBundle` encoding: author (20),
// manifest commitment (32), inventory commitment (32, the version 3 field a
// buyer checks a CID set against), reserve (16, big-endian), units (8), beacon
// (8), strikes (4), the buy nonce (8), which the views do not publish, the
// covered flip count (4) the registration statement bound, and the buyer escrow
// (16) the reserve must not be spent from.
func decodePoolRecord(
	key string,
	raw []byte,
	reserve, pending *big.Int,
	poolUnits uint64,
) (*marketPoolListing, error) {
	if len(raw) < marketPoolValueBytes {
		return nil, fmt.Errorf("pool listing %s is %d bytes, expected %d", key, len(raw), marketPoolValueBytes)
	}
	units := binary.BigEndian.Uint64(raw[marketPoolUnitsOffset:marketPoolBeaconOffset])
	listing := &marketPoolListing{
		Bundle:              key,
		Author:              hex.EncodeToString(raw[marketPoolAuthorOffset:marketPoolManifestOffset]),
		ManifestCommitment:  hex.EncodeToString(raw[marketPoolManifestOffset:marketPoolInventoryOffset]),
		InventoryCommitment: hex.EncodeToString(raw[marketPoolInventoryOffset:marketPoolUnitsOffset]),
		Reserve:             reserve.String(),
		Pending:             pending.String(),
		Units:               units,
		PoolUnits:           poolUnits,
		BeaconBlock:         binary.BigEndian.Uint64(raw[marketPoolBeaconOffset:marketPoolStrikesOffset]),
		Strikes:             binary.BigEndian.Uint32(raw[marketPoolStrikesOffset:marketPoolNonceOffset]),
		Nonce:               binary.BigEndian.Uint64(raw[marketPoolNonceOffset:marketPoolCoveredOffset]),
		FlipCount:           binary.BigEndian.Uint32(raw[marketPoolCoveredOffset:marketPoolValueBytes]),
	}
	// The price is the one pool's: `ceil(reserve / (poolUnits - 1))`, and
	// nothing while the market holds fewer than two units.
	if quote, ok := poolBuyQuote(reserve, poolUnits); ok {
		listing.BuyQuote = quote.String()
	}
	listing.SellQuote = poolSellQuote(reserve, poolUnits).String()
	return listing, nil
}

// poolBuyQuote is the contract's `ceil(reserve / (units - 1))`, and nothing when
// the bundle has fewer than two units (`contracts/escrow/src/pool.rs`).
func poolBuyQuote(reserve *big.Int, units uint64) (*big.Int, bool) {
	if units < 2 {
		return nil, false
	}
	divisor := new(big.Int).SetUint64(units - 1)
	quotient, remainder := new(big.Int).QuoRem(reserve, divisor, new(big.Int))
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient, true
}

// poolSellQuote is the contract's `floor(reserve / (units + 1))`.
func poolSellQuote(reserve *big.Int, units uint64) *big.Int {
	return new(big.Int).Quo(reserve, new(big.Int).SetUint64(units+1))
}

// marketPoolOrder is one pool order as `SPEC.md` 7.2 defines it: what a buyer
// paid, where the order stands, and when it refunds. The delivery key stays out
// of the state file: the buyer keeps their own copy, and the desk reads the
// order from the chain.
type marketPoolOrder struct {
	OrderID   uint64 `json:"orderId"`
	Bundle    string `json:"bundle"`
	Buyer     string `json:"buyer"`
	Paid      string `json:"paid"`
	FundedAt  uint64 `json:"fundedAtBlock"`
	Deadline  uint64 `json:"deliveryDeadlineBlock"`
	RefundAt  uint64 `json:"refundAfterBlock"`
	State     string `json:"state"`
	Delivered bool   `json:"delivered,omitempty"`
}

// poolOrderRecord is the escrow's full order record. The marketplace state
// keeps only the public half (marketPoolOrder below); the delivery desk needs
// the buyer's delivery key and the buy nonce as well.
type poolOrderRecord struct {
	ID          uint64
	Bundle      string
	Buyer       string
	DeliveryKey string
	Paid        string
	FundedAt    uint64
	Nonce       uint64
	State       string
	Deadline    uint64
	RefundAt    uint64
}

// decodePoolOrderFull reads the escrow's `PoolOrder` encoding: bundle (32),
// buyer (20), delivery key (65), paid (16, big-endian), funded at (8), nonce
// (8), and the state byte.
func decodePoolOrderFull(id uint64, raw []byte) (*poolOrderRecord, error) {
	if len(raw) < marketPoolOrderBytes {
		return nil, fmt.Errorf("order %d is %d bytes, expected %d", id, len(raw), marketPoolOrderBytes)
	}
	fundedAt := binary.BigEndian.Uint64(raw[133:141])
	states := map[byte]string{1: "funded", 2: "delivered", 3: "settled", 4: "refunded"}
	state, ok := states[raw[149]]
	if !ok {
		return nil, fmt.Errorf("order %d carries state %d", id, raw[149])
	}
	return &poolOrderRecord{
		ID:          id,
		Bundle:      hex.EncodeToString(raw[:32]),
		Buyer:       hex.EncodeToString(raw[32:52]),
		DeliveryKey: hex.EncodeToString(raw[52:117]),
		Paid:        new(big.Int).SetBytes(raw[117:133]).String(),
		FundedAt:    fundedAt,
		Nonce:       binary.BigEndian.Uint64(raw[141:149]),
		State:       state,
		Deadline:    fundedAt + marketDeadlineBlocks,
		RefundAt:    fundedAt + marketDeadlineBlocks + marketRefundBlocks,
	}, nil
}

// decodePoolOrderRecord is the interface's view: the same record without the
// delivery key and buy nonce, which stay out of the marketplace state file.
func decodePoolOrderRecord(id uint64, raw []byte) (*marketPoolOrder, error) {
	order, err := decodePoolOrderFull(id, raw)
	if err != nil {
		return nil, err
	}
	return &marketPoolOrder{
		OrderID:   order.ID,
		Bundle:    order.Bundle,
		Buyer:     order.Buyer,
		Paid:      order.Paid,
		FundedAt:  order.FundedAt,
		Deadline:  order.Deadline,
		RefundAt:  order.RefundAt,
		State:     order.State,
		Delivered: order.State == "delivered" || order.State == "settled",
	}, nil
}

// loadProvenance reads registration artefacts and indexes them by the key
// commitment their journal carries.
func loadProvenance(paths []string) (map[string]provenance, error) {
	out := make(map[string]provenance)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var artefact registrationArtefact
		if err := json.Unmarshal(raw, &artefact); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if artefact.Statement != registrationStatement {
			return nil, fmt.Errorf("%s is not a registration artefact", path)
		}
		journal, err := decodeHexField(artefact.Journal)
		if err != nil || len(journal) != registrationJournalBytes {
			return nil, fmt.Errorf("%s has no usable journal", path)
		}
		commitment := hex.EncodeToString(journal[registrationCommitmentOffset : registrationCommitmentOffset+32])
		out[commitment] = provenance{
			Author:             hex.EncodeToString(journal[registrationAuthorOffset : registrationAuthorOffset+addressBytes]),
			Epoch:              binary.BigEndian.Uint16(journal[registrationEpochOffset : registrationEpochOffset+2]),
			FlipCount:          binary.BigEndian.Uint32(journal[registrationCoveredOffset : registrationCoveredOffset+4]),
			ManifestCommitment: hex.EncodeToString(journal[registrationManifestOffset : registrationManifestOffset+32]),
		}
	}
	return out, nil
}

// buildMarketState reads the registry and assembles the snapshot.
func buildMarketState(c *client, contract, viewer, network string, proofs []string, availabilityPaths []string) (*marketState, error) {
	contract = strings.ToLower(strings.TrimPrefix(contract, "0x"))
	if !isHex(contract, addressBytes) {
		return nil, errors.New("no valid registry address")
	}

	height, blockSeconds, err := registryHead(c)
	if err != nil {
		return nil, err
	}
	var stake struct {
		Hash *string `json:"Hash"`
	}
	if err := c.call("contract_getStake", []any{"0x" + contract}, &stake); err != nil {
		return nil, fmt.Errorf("read the registry code hash: %w", err)
	}
	credits, err := readPrefix(c, "0x"+contract, marketCreditPrefix)
	if err != nil {
		return nil, err
	}
	poolRecords, err := readPrefix(c, "0x"+contract, marketPoolPrefix)
	if err != nil {
		return nil, err
	}
	fastRecords, err := readPrefix(c, "0x"+contract, "pfast/")
	if err != nil {
		return nil, err
	}
	poolOrders, err := readPrefix(c, "0x"+contract, marketPoolOrderPrefix)
	if err != nil {
		return nil, err
	}
	poolReserve, err := readCounter(c, "0x"+contract, marketReserveKey)
	if err != nil {
		return nil, err
	}
	poolPending, err := readCounter(c, "0x"+contract, marketPendingKey)
	if err != nil {
		return nil, err
	}
	poolUnitCounter, err := readCounter(c, "0x"+contract, marketPoolUnitsKey)
	if err != nil {
		return nil, err
	}
	provenanceByCommitment, err := loadProvenance(proofs)
	if err != nil {
		return nil, err
	}
	advisories, err := loadAvailability(availabilityPaths)
	if err != nil {
		return nil, err
	}

	state := &marketState{
		Format:       marketStateFormat,
		Version:      marketStateVersion,
		Network:      network,
		Contract:     contract,
		BlockHeight:  height,
		BlockSeconds: blockSeconds,
		ObservedAt:   time.Now().UTC().Format(time.RFC3339),
		Source:       fmt.Sprintf("registry storage at block %d, read through the node", height),
		Credits:      []marketCredit{},
	}
	if stake.Hash != nil {
		state.CodeHash = strings.ToLower(strings.TrimPrefix(*stake.Hash, "0x"))
	}

	poolKeys := make([]string, 0, len(poolRecords))
	for key := range poolRecords {
		poolKeys = append(poolKeys, key)
	}
	sort.Strings(poolKeys)
	for _, key := range poolKeys {
		listing, err := decodePoolRecord(key, poolRecords[key], poolReserve, poolPending, poolUnitCounter.Uint64())
		if err != nil {
			return nil, err
		}
		withPoolProvenance(listing, provenanceByCommitment)
		if fast, exists := fastRecords[key]; exists {
			if len(fast) != fastBundleRecordBytes || uint32(binary.BigEndian.Uint16(fast[2:4])) != listing.FlipCount {
				return nil, fmt.Errorf("invalid fast bundle record for %s", key)
			}
			listing.Fast = true
			listing.Epoch = binary.BigEndian.Uint16(fast[:2])
		}
		attachAvailability(listing, advisories)
		state.Pool = append(state.Pool, *listing)
	}

	orderKeys := make([]uint64, 0, len(poolOrders))
	for key := range poolOrders {
		rawKey, err := hex.DecodeString(key)
		if err != nil {
			return nil, err
		}
		id, err := strconv.ParseUint(string(rawKey), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("pool order key %q is not an id", key)
		}
		orderKeys = append(orderKeys, id)
	}
	sort.Slice(orderKeys, func(i, j int) bool { return orderKeys[i] < orderKeys[j] })
	for _, id := range orderKeys {
		order, err := decodePoolOrderRecord(id, poolOrders[hex.EncodeToString([]byte(strconv.FormatUint(id, 10)))])
		if err != nil {
			return nil, err
		}
		state.PoolOrders = append(state.PoolOrders, *order)
	}

	creditKeys := make([]string, 0, len(credits))
	for key := range credits {
		creditKeys = append(creditKeys, key)
	}
	sort.Strings(creditKeys)
	for _, key := range creditKeys {
		raw := credits[key]
		if len(raw) < marketCreditValueBytes {
			return nil, fmt.Errorf("credit record %s is %d bytes", key, len(raw))
		}
		address := key
		amount := new(big.Int).SetBytes(raw[:marketCreditValueBytes]).String()
		state.Credits = append(state.Credits, marketCredit{Address: address, Amount: amount})
		if viewer != "" && strings.EqualFold(strings.TrimPrefix(viewer, "0x"), address) {
			state.Viewer = &marketViewer{Address: address, Credits: amount}
		}
	}
	if viewer != "" && state.Viewer == nil {
		state.Viewer = &marketViewer{
			Address: strings.ToLower(strings.TrimPrefix(viewer, "0x")),
			Credits: "0",
		}
	}
	return state, nil
}

// runMarket writes the snapshot the interface imports.
func runMarket(contract, viewer, proofs, network, outPath, rpcURL, apiKey, availabilityFlag string) {
	node := &client{url: rpcURL, key: apiKey, http: newHTTPClient()}
	var proofPaths []string
	for _, path := range strings.Split(proofs, ",") {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			proofPaths = append(proofPaths, trimmed)
		}
	}
	var availabilityPaths []string
	for _, path := range strings.Split(availabilityFlag, ",") {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			availabilityPaths = append(availabilityPaths, trimmed)
		}
	}
	state, err := buildMarketState(node, contract, viewer, network, proofPaths, availabilityPaths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused: %v\n", err)
		os.Exit(1)
	}
	state.StateDigest = marketStateDigest(state)
	if state.StateDigest == "" {
		// Two readers must never "agree" on an empty digest because the encoder
		// failed silently.
		fmt.Fprintln(os.Stderr, "refused: the state could not be encoded for its digest")
		os.Exit(1)
	}
	fmt.Printf("registry    %s\n", state.Contract)
	if state.CodeHash != "" {
		fmt.Printf("code hash   %s\n", state.CodeHash)
	}
	fmt.Printf("block       %d\n", state.BlockHeight)
	fmt.Printf("pool        %d listings (%d orders)\n", len(state.Pool), len(state.PoolOrders))
	fmt.Printf("credits     %d\n", len(state.Credits))
	fmt.Printf("state digest %s\n", state.StateDigest)
	if outPath != "" {
		if err := writeJSON(outPath, state); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", outPath, err)
			os.Exit(1)
		}
		fmt.Printf("state       %s\n", outPath)
		return
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}

// marketStateDigest normalises the one field two reads legitimately disagree
// on, so two operators can compare one line instead of diffing JSON.
func marketStateDigest(state *marketState) string {
	normalized := *state
	normalized.ObservedAt = ""
	normalized.StateDigest = ""
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
