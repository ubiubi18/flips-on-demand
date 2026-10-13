package main

// Read-only public state for contracts/direct-escrow-v6. In particular, order
// delivery keys, private messages, reveals, and secret storage are not read.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	poolV6OfferBytes      = 238
	poolV6OrderBytes      = 215
	poolV6OfferDomain     = "fod:pool-offer:v6"
	poolV6OfferPrefix     = "pool/offer/"
	poolV6InventoryPrefix = "pool/inventory/"
	poolV6OrderPrefix     = "pool/order/"
	poolV6ProofWindow     = 12_960
	poolV6BuyerProofGas   = 2_500_000
	poolV6BuyerFeeMargin  = 2
	poolV6SellerProofGas  = 2_500_000
	poolV6SellerFeeMargin = 2
	poolV6MaxHistory      = 100_000
)

var poolV6MaxAmount = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
var poolV6PriceCap = new(big.Int).Mul(big.NewInt(1_000), big.NewInt(1_000_000_000_000_000_000))

func capPoolV6Quote(quote *big.Int) *big.Int {
	if quote.Cmp(poolV6PriceCap) > 0 {
		quote.Set(poolV6PriceCap)
	}
	return quote
}

type poolV6Call func(method string, params any, result any) error

type poolV6OfferRecord struct {
	author          [20]byte
	epoch           uint16
	covered         uint32
	inventory       [32]byte
	keys            [32]byte
	payout          *big.Int
	sellerBond      *big.Int
	nextNonce       uint64
	openOrder       uint64
	status          byte
	everSold        bool
	previous        [32]byte
	next            [32]byte
	sequence        uint64
	listedAt        uint64
	contentProven   bool
	fraudCollateral *big.Int
	publicSeed      bool
}

type poolV6OrderRecord struct {
	offer        [32]byte
	buyer        [20]byte
	paid         *big.Int
	bond         *big.Int
	sellerPayout *big.Int
	fundedAt     uint64
	nonce        uint64
	state        byte
	challenged   bool
}

type poolV6OfferMeta struct {
	author     [20]byte
	epoch      uint16
	covered    uint32
	inventory  [32]byte
	nextNonce  uint64
	publicSeed bool
}

// Decode only fields needed for the public view. The 65-byte order delivery
// key and 32-byte delivery hash are deliberately skipped.
func decodePoolV6Offer(raw []byte) (*poolV6OfferRecord, error) {
	if len(raw) != poolV6OfferBytes || raw[138] > 6 || raw[139] > 1 || raw[220] > 1 || raw[237] > 1 {
		return nil, errors.New("invalid v6 pool offer record")
	}
	o := &poolV6OfferRecord{
		epoch: binary.BigEndian.Uint16(raw[20:22]), covered: binary.BigEndian.Uint32(raw[22:26]),
		payout: new(big.Int).SetBytes(raw[90:106]), sellerBond: new(big.Int).SetBytes(raw[106:122]),
		nextNonce: binary.BigEndian.Uint64(raw[122:130]), openOrder: binary.BigEndian.Uint64(raw[130:138]),
		status: raw[138], everSold: raw[139] == 1,
		sequence: binary.BigEndian.Uint64(raw[204:212]), listedAt: binary.BigEndian.Uint64(raw[212:220]),
		contentProven: raw[220] == 1, fraudCollateral: new(big.Int).SetBytes(raw[221:237]),
		publicSeed: raw[237] == 1,
	}
	copy(o.author[:], raw[0:20])
	copy(o.inventory[:], raw[26:58])
	copy(o.keys[:], raw[58:90])
	copy(o.previous[:], raw[140:172])
	copy(o.next[:], raw[172:204])
	return o, nil
}

func decodePoolV6Inventory(raw []byte, offer poolV6OfferMeta) ([]string, error) {
	if len(raw) < 2 || int(raw[0]) != int(offer.covered) || offer.covered == 0 || offer.covered > 32 {
		return nil, errors.New("invalid public CID count")
	}
	cids := make([]string, 0, offer.covered)
	at := 1
	for range offer.covered {
		if at >= len(raw) {
			return nil, errors.New("truncated public CID inventory")
		}
		length := int(raw[at])
		at++
		if length == 0 || at+length > len(raw) {
			return nil, errors.New("invalid public CID length")
		}
		cid := string(raw[at : at+length])
		if !isFlipCID(cid) || (len(cids) != 0 && cids[len(cids)-1] >= cid) {
			return nil, errors.New("invalid or unsorted public CID")
		}
		cids = append(cids, cid)
		at += length
	}
	if at != len(raw) || inventoryCommitment(hex.EncodeToString(offer.author[:]), offer.epoch, cids) != hex.EncodeToString(offer.inventory[:]) {
		return nil, errors.New("public CIDs differ from committed inventory")
	}
	return cids, nil
}

func decodePoolV6Order(raw []byte) (*poolV6OrderRecord, error) {
	if len(raw) != poolV6OrderBytes || raw[181] < 1 || raw[181] > 4 || raw[214] > 1 {
		return nil, errors.New("invalid v6 pool order record")
	}
	o := &poolV6OrderRecord{
		paid: new(big.Int).SetBytes(raw[117:133]), bond: new(big.Int).SetBytes(raw[133:149]),
		sellerPayout: new(big.Int).SetBytes(raw[149:165]),
		fundedAt:     binary.BigEndian.Uint64(raw[165:173]), nonce: binary.BigEndian.Uint64(raw[173:181]),
		state: raw[181], challenged: raw[214] == 1,
	}
	copy(o.offer[:], raw[0:32])
	copy(o.buyer[:], raw[32:52])
	return o, nil
}

func poolV6OfferID(author [20]byte, epoch uint16) [32]byte {
	var epochBytes [2]byte
	binary.BigEndian.PutUint16(epochBytes[:], epoch)
	preimage := make([]byte, 0, len(poolV6OfferDomain)+len(author)+2)
	preimage = append(preimage, poolV6OfferDomain...)
	preimage = append(preimage, author[:]...)
	preimage = append(preimage, epochBytes[:]...)
	return sha256.Sum256(preimage)
}

type directV6PoolOffer struct {
	OfferID             string   `json:"offerId"`
	Author              string   `json:"author"`
	Epoch               uint16   `json:"epoch"`
	CoveredFlips        uint32   `json:"coveredFlips"`
	OriginalCIDs        []string `json:"originalCids"`
	InventoryCommitment string   `json:"inventoryCommitment"`
	KeysCommitment      string   `json:"keysCommitment"`
	Payout              string   `json:"payout"`
	SellerBond          string   `json:"sellerBond"`
	FraudCollateral     string   `json:"fraudCollateral"`
	NextNonce           string   `json:"nextNonce"`
	OpenOrderID         string   `json:"openOrderId"`
	State               string   `json:"state"`
	EverSold            bool     `json:"everSold"`
	ContentProven       bool     `json:"contentProven"`
	PublicSeed          bool     `json:"publicSeed"`
	Queue               string   `json:"queue,omitempty"`
	QueuePosition       int      `json:"queuePosition,omitempty"`
	Sequence            string   `json:"sequence"`
	ListedAtBlock       string   `json:"listedAtBlock"`
}

type directV6PoolOrder struct {
	OrderID         string `json:"orderId"`
	OfferID         string `json:"offerId"`
	Buyer           string `json:"buyer"`
	Paid            string `json:"paid"`
	Bond            string `json:"bond"`
	SellerPayout    string `json:"sellerPayout"`
	FundedAtBlock   string `json:"fundedAtBlock"`
	RefundAtBlock   string `json:"refundAtBlock"`
	Nonce           string `json:"nonce"`
	State           string `json:"state"`
	Challenged      bool   `json:"challenged"`
	RefundAvailable bool   `json:"refundAvailable"`
}

type directV6PoolTotals struct {
	Reserve                    string    `json:"reserve"`
	Committed                  string    `json:"committed"`
	BuyerPayments              string    `json:"buyerPayments"`
	Insurance                  string    `json:"insurance"`
	Pending                    string    `json:"pending"`
	VirtualReserve             string    `json:"virtualReserve"`
	PrimaryUnits               uint64    `json:"primaryUnits"`
	SecondaryUnits             uint64    `json:"secondaryUnits"`
	PoolUnits                  uint64    `json:"poolUnits"`
	ReservedUnits              uint64    `json:"reservedUnits"`
	SeedCount                  uint64    `json:"seedCount"`
	SeedOfferIDs               [2]string `json:"seedOfferIds"`
	NextOrderID                string    `json:"nextOrderId"`
	NextSequence               string    `json:"nextSequence"`
	NextOfferID                string    `json:"nextOfferId,omitempty"`
	BuyQuote                   string    `json:"buyQuote,omitempty"`
	MinimumBuyerBond           string    `json:"minimumBuyerBond,omitempty"`
	MinimumBuyerTotal          string    `json:"minimumBuyerTotal,omitempty"`
	NextSellerPayout           string    `json:"nextSellerPayout,omitempty"`
	MinimumProvedSellerBond    string    `json:"minimumProvedSellerBond,omitempty"`
	MinimumUnprovedSellerFunds string    `json:"minimumUnprovedSellerFunds,omitempty"`
	BondTopUpRequired          string    `json:"bondTopUpRequired,omitempty"`
	BuyPausedForBond           bool      `json:"buyPausedForBond"`
}

type directV6PoolState struct {
	Format      string              `json:"format"`
	Version     int                 `json:"version"`
	Scope       string              `json:"scope"`
	Network     string              `json:"network"`
	Contract    string              `json:"contract"`
	CodeHash    string              `json:"codeHash"`
	BlockHeight uint64              `json:"blockHeight"`
	ObservedAt  string              `json:"observedAt"`
	Source      string              `json:"source"`
	Pool        directV6PoolTotals  `json:"pool"`
	Offers      []directV6PoolOffer `json:"offers"`
	Orders      []directV6PoolOrder `json:"orders"`
}

// Scan complete map prefixes so old terminal trades cannot block the live
// dashboard. Only the caller's live records are retained. The node returns
// suffix keys as hex; reject duplicate keys and stalled pagination.
func walkPoolV6Prefix(call poolV6Call, contract, prefix string, scanned *int, visit func(string, []byte) error) (map[string]struct{}, error) {
	keys := make(map[string]struct{})
	seenTokens := make(map[string]bool)
	var token any
	for {
		var page mapPage
		if err := call("contract_iterateMap", []any{contract, prefix, token, "hex", "hex", marketPageLimit}, &page); err != nil {
			return nil, fmt.Errorf("read %s: %w", prefix, err)
		}
		if len(page.Items) > marketPageLimit || *scanned+len(page.Items) > poolV6MaxHistory {
			return nil, fmt.Errorf("v6 pool exceeds the %d-record scan limit", poolV6MaxHistory)
		}
		for _, item := range page.Items {
			if item == nil {
				return nil, fmt.Errorf("read %s: empty map item", prefix)
			}
			key, keyErr := decodeHexField(item.Key)
			value, valueErr := decodeHexField(item.Value)
			if keyErr != nil || valueErr != nil {
				return nil, fmt.Errorf("read %s: invalid map hex", prefix)
			}
			canonical := hex.EncodeToString(key)
			if _, exists := keys[canonical]; exists {
				return nil, fmt.Errorf("read %s: duplicate key", prefix)
			}
			keys[canonical] = struct{}{}
			*scanned++
			if err := visit(canonical, value); err != nil {
				return nil, err
			}
		}
		if page.ContinuationToken == nil || strings.TrimPrefix(*page.ContinuationToken, "0x") == "" {
			return keys, nil
		}
		if len(page.Items) == 0 || seenTokens[*page.ContinuationToken] {
			return nil, fmt.Errorf("read %s: invalid continuation", prefix)
		}
		seenTokens[*page.ContinuationToken] = true
		token = *page.ContinuationToken
	}
}

func readPoolV6Field(call poolV6Call, contract, key string, size int) ([]byte, error) {
	var encoded string
	if err := call("contract_readData", []any{contract, key, "hex"}, &encoded); err != nil {
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	raw, err := decodeHexField(encoded)
	if err != nil || len(raw) != size {
		return nil, fmt.Errorf("read %s: expected %d bytes", key, size)
	}
	return raw, nil
}

func buildDirectV6PoolState(c *client, contract, codeHash string) (*directV6PoolState, error) {
	return buildDirectV6PoolStateWithCall(c.call, contract, codeHash)
}

// The call seam allows the full snapshot path to be tested without a node or
// even a loopback server; production passes the existing RPC client method.
func buildDirectV6PoolStateWithCall(call poolV6Call, contract, codeHash string) (*directV6PoolState, error) {
	var before rpcBlock
	if err := call("bcn_lastBlock", []any{}, &before); err != nil {
		return nil, fmt.Errorf("read v6 pool head: %w", err)
	}
	if before.Height > directMarketMaxJSONHeight || before.Hash == "" || before.Hash == "0x" {
		return nil, errors.New("invalid v6 pool head")
	}
	address := registryAddress(contract)
	scanned, retained := 0, 0
	offers := make(map[string]*poolV6OfferRecord)
	offerMeta := make(map[string]poolV6OfferMeta)
	offerIDs := make([]string, 0)
	_, err := walkPoolV6Prefix(call, address, poolV6OfferPrefix, &scanned, func(id string, raw []byte) error {
		if len(id) != 64 {
			return errors.New("v6 pool offer key is not a 32-byte ID")
		}
		offer, err := decodePoolV6Offer(raw)
		if err != nil {
			return fmt.Errorf("v6 pool offer %s: %w", id, err)
		}
		derived := poolV6OfferID(offer.author, offer.epoch)
		if hex.EncodeToString(derived[:]) != id || offer.covered == 0 || offer.covered > 32 ||
			offer.inventory == ([32]byte{}) || offer.keys == ([32]byte{}) || offer.nextNonce == 0 ||
			offer.payout.Cmp(poolV6PriceCap) > 0 {
			return fmt.Errorf("v6 pool offer %s has invalid identity or commitments", id)
		}
		if offer.publicSeed && (!offer.contentProven || offer.payout.Sign() != 0 || offer.sellerBond.Sign() != 0 ||
			offer.fraudCollateral.Sign() != 0 || (offer.status != 2 && offer.status != 6)) {
			return fmt.Errorf("v6 pool offer %s has invalid public seed state", id)
		}
		offerMeta[id] = poolV6OfferMeta{
			author: offer.author, epoch: offer.epoch, covered: offer.covered,
			inventory: offer.inventory, nextNonce: offer.nextNonce, publicSeed: offer.publicSeed,
		}
		if offer.status == 1 || offer.status == 2 || offer.status == 3 ||
			(offer.status == 4 && offer.fraudCollateral.Sign() > 0) {
			if retained >= directMarketMaxItems {
				return fmt.Errorf("v6 pool exceeds the %d-live-record limit", directMarketMaxItems)
			}
			offers[id] = offer
			offerIDs = append(offerIDs, id)
			retained++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(offerIDs)
	publicInventories := make(map[string][]string, len(offers))
	inventoryIDs, err := walkPoolV6Prefix(call, address, poolV6InventoryPrefix, &scanned, func(id string, raw []byte) error {
		if len(id) != 64 {
			return errors.New("v6 pool public inventory key is not a 32-byte ID")
		}
		if _, exists := offerMeta[id]; !exists {
			return errors.New("v6 pool public inventory has no offer")
		}
		cids, err := decodePoolV6Inventory(raw, offerMeta[id])
		if err != nil {
			return fmt.Errorf("v6 pool public inventory %s: %w", id, err)
		}
		if offers[id] != nil {
			publicInventories[id] = cids
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(inventoryIDs) != len(offerMeta) || len(publicInventories) != len(offers) {
		return nil, errors.New("v6 pool public inventory differs from its offers")
	}
	orders := make(map[uint64]*poolV6OrderRecord)
	orderIDs := make([]uint64, 0)
	var maxOrderID uint64
	_, err = walkPoolV6Prefix(call, address, poolV6OrderPrefix, &scanned, func(key string, raw []byte) error {
		keyBytes, err := hex.DecodeString(key)
		if err != nil {
			return errors.New("v6 pool order key is invalid hex")
		}
		id, err := strconv.ParseUint(string(keyBytes), 10, 64)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != string(keyBytes) {
			return errors.New("v6 pool order key is not a canonical positive ID")
		}
		order, err := decodePoolV6Order(raw)
		if err != nil {
			return fmt.Errorf("v6 pool order %d: %w", id, err)
		}
		offerID := hex.EncodeToString(order.offer[:])
		meta, exists := offerMeta[offerID]
		if !exists || meta.publicSeed || order.nonce == 0 || order.nonce >= meta.nextNonce ||
			order.paid.Sign() == 0 || order.paid.Cmp(poolV6PriceCap) > 0 || order.bond.Sign() == 0 {
			return fmt.Errorf("v6 pool order %d has invalid offer reference or amount", id)
		}
		if id > maxOrderID {
			maxOrderID = id
		}
		offer := offers[offerID]
		if order.state == 1 || order.state == 4 ||
			(offer != nil && offer.status == 4 && offer.openOrder == id && offer.fraudCollateral.Sign() > 0) {
			if offer == nil || offer.publicSeed || order.nonce >= offer.nextNonce || retained >= directMarketMaxItems {
				return fmt.Errorf("v6 pool order %d has invalid live offer or exceeds the live-record limit", id)
			}
			orders[id] = order
			orderIDs = append(orderIDs, id)
			retained++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(orderIDs, func(i, j int) bool { return orderIDs[i] < orderIDs[j] })
	amountFields := []string{"reserve", "committed", "buyer-payments", "insurance", "pending"}
	amounts := make(map[string]*big.Int, len(amountFields))
	for _, name := range amountFields {
		raw, err := readPoolV6Field(call, address, "pool/"+name, 16)
		if err != nil {
			return nil, err
		}
		amounts[name] = new(big.Int).SetBytes(raw)
	}
	unitFields := []string{"primary-units", "secondary-units", "units", "reserved-units", "seed-count", "next-order", "next-sequence"}
	units := make(map[string]uint64, len(unitFields))
	for _, name := range unitFields {
		raw, err := readPoolV6Field(call, address, "pool/"+name, 8)
		if err != nil {
			return nil, err
		}
		units[name] = binary.BigEndian.Uint64(raw)
	}
	idFields := []string{"primary-head", "primary-tail", "secondary-head", "secondary-tail", "seed-first", "seed-second"}
	ids := make(map[string][32]byte, len(idFields))
	for _, name := range idFields {
		raw, err := readPoolV6Field(call, address, "pool/"+name, 32)
		if err != nil {
			return nil, err
		}
		ids[name] = [32]byte(raw)
	}

	if units["next-order"] == 0 || units["next-sequence"] == 0 ||
		units["next-order"] <= maxOrderID {
		return nil, errors.New("v6 pool has invalid next counters")
	}

	positions := make(map[string]int)
	for _, queue := range []struct {
		name   string
		status byte
	}{{"primary", 1}, {"secondary", 2}} {
		head := ids[queue.name+"-head"]
		tail := ids[queue.name+"-tail"]
		var previous [32]byte
		visited := uint64(0)
		for current := head; current != ([32]byte{}); {
			key := hex.EncodeToString(current[:])
			offer := offers[key]
			if offer == nil || offer.status != queue.status || offer.openOrder != 0 ||
				(!offer.publicSeed && (offer.sellerBond.Sign() == 0 ||
					(queue.status == 1 && (offer.everSold || offer.payout.Sign() == 0)) ||
					(queue.status == 2 && (!offer.everSold || !offer.contentProven || offer.payout.Sign() != 0)))) ||
				offer.previous != previous || positions[key] != 0 {
				return nil, fmt.Errorf("v6 pool %s queue has invalid link at %s", queue.name, key)
			}
			visited++
			if visited > units[queue.name+"-units"] || len(positions) >= directMarketMaxItems {
				return nil, fmt.Errorf("v6 pool %s queue exceeds its unit count", queue.name)
			}
			positions[key] = len(positions) + 1
			previous, current = current, offer.next
		}
		if previous != tail || visited != units[queue.name+"-units"] {
			return nil, fmt.Errorf("v6 pool %s queue differs from its head, tail, or unit count", queue.name)
		}
	}
	for key, offer := range offers {
		if offer.status == 1 || offer.status == 2 {
			if positions[key] == 0 {
				return nil, fmt.Errorf("v6 pool queued offer %s is unlinked", key)
			}
		} else if offer.previous != ([32]byte{}) || offer.next != ([32]byte{}) {
			return nil, fmt.Errorf("v6 pool unqueued offer %s has queue links", key)
		}
	}
	for id, order := range orders {
		if order.state == 1 || order.state == 4 {
			offer := offers[hex.EncodeToString(order.offer[:])]
			expectedPayout := offer.payout
			if offer.everSold {
				expectedPayout = new(big.Int).Div(order.paid, big.NewInt(2))
			}
			if offer.status != 3 || offer.openOrder != id || expectedPayout.Cmp(order.sellerPayout) != 0 {
				return nil, errors.New("v6 pool funded order disagrees with its active offer")
			}
		}
	}
	active := uint64(0)
	pendingPoolCredit := big.NewInt(0)
	for _, offer := range offers {
		if offer.status == 3 {
			active++
			order := orders[offer.openOrder]
			expectedID := poolV6OfferID(offer.author, offer.epoch)
			if offer.openOrder == 0 || order == nil || (order.state != 1 && order.state != 4) ||
				order.offer != expectedID {
				return nil, errors.New("v6 pool active offer has no funded order")
			}
			credit := new(big.Int).Set(order.paid)
			if offer.everSold {
				credit.Sub(credit, order.sellerPayout)
			}
			pendingPoolCredit.Add(pendingPoolCredit, credit)
		} else if offer.status == 4 && offer.fraudCollateral.Sign() > 0 {
			order := orders[offer.openOrder]
			expectedID := poolV6OfferID(offer.author, offer.epoch)
			if offer.openOrder == 0 || order == nil || order.state != 2 || order.offer != expectedID {
				return nil, errors.New("v6 pool collateralized private sale has no settled order")
			}
		}
	}
	if pendingPoolCredit.Cmp(amounts["buyer-payments"]) != 0 {
		return nil, errors.New("v6 pool pending buyer credit disagrees with its active orders")
	}
	x := units["primary-units"] + units["secondary-units"]
	if x < units["primary-units"] || x != uint64(len(positions)) ||
		units["reserved-units"] != active || units["units"] < x || units["units"]-x != active {
		return nil, errors.New("v6 pool unit counters disagree with its queues and active orders")
	}
	seedCount := units["seed-count"]
	if ids["seed-first"] == ([32]byte{}) || ids["seed-second"] == ([32]byte{}) || ids["seed-first"] == ids["seed-second"] {
		return nil, errors.New("v6 pool has invalid pinned seed offers")
	}
	firstSeed, secondSeed := ids["seed-first"], ids["seed-second"]
	if seedCount > 2 || (seedCount < 2 &&
		(units["units"] != seedCount || units["secondary-units"] != seedCount ||
			units["primary-units"] != 0 || maxOrderID != 0 || units["next-order"] != 1)) {
		return nil, errors.New("v6 pool seed initialization is inconsistent")
	}
	reserve, committed := amounts["reserve"], amounts["committed"]
	if reserve.Cmp(committed) < 0 {
		return nil, errors.New("v6 pool committed amount exceeds reserve")
	}
	virtual := new(big.Int).Sub(reserve, committed)
	virtual.Add(virtual, amounts["buyer-payments"])
	if virtual.BitLen() > 128 || virtual.Cmp(amounts["insurance"]) < 0 {
		return nil, errors.New("v6 pool virtual reserve overflows or is underfunded")
	}
	virtual.Sub(virtual, amounts["insurance"])
	if virtual.Sign() == 0 {
		return nil, errors.New("v6 pool virtual reserve is zero")
	}
	pool := directV6PoolTotals{
		Reserve: reserve.String(), Committed: committed.String(), BuyerPayments: amounts["buyer-payments"].String(),
		Insurance: amounts["insurance"].String(), Pending: amounts["pending"].String(), VirtualReserve: virtual.String(),
		PrimaryUnits: units["primary-units"], SecondaryUnits: units["secondary-units"],
		PoolUnits: units["units"], ReservedUnits: active, SeedCount: seedCount,
		SeedOfferIDs: [2]string{hex.EncodeToString(firstSeed[:]), hex.EncodeToString(secondSeed[:])},
		NextOrderID:  strconv.FormatUint(units["next-order"], 10), NextSequence: strconv.FormatUint(units["next-sequence"], 10),
	}
	oldest := ids["primary-head"]
	if oldest == ([32]byte{}) {
		oldest = ids["secondary-head"]
	}
	if oldest != ([32]byte{}) {
		pool.NextOfferID = hex.EncodeToString(oldest[:])
	}
	var fee *big.Int
	if seedCount == 2 {
		var rawFee json.RawMessage
		if err := call("bcn_feePerGas", []any{}, &rawFee); err != nil {
			return nil, fmt.Errorf("read v6 pool fee: %w", err)
		}
		fee, err = parseFeePerGas(rawFee)
		if err != nil {
			return nil, err
		}
	}
	if fee != nil && x > 0 && x < ^uint64(0) {
		nextUnits := new(big.Int).SetUint64(x + 1)
		payout := capPoolV6Quote(new(big.Int).Div(virtual, nextUnits))
		nextCommitted := new(big.Int).Add(committed, payout)
		if payout.Sign() > 0 && nextCommitted.Cmp(reserve) <= 0 {
			nextVirtual := new(big.Int).Sub(virtual, payout)
			nextBuyerQuote, remainder := new(big.Int).QuoRem(nextVirtual, new(big.Int).SetUint64(x), new(big.Int))
			if remainder.Sign() != 0 {
				nextBuyerQuote.Add(nextBuyerQuote, big.NewInt(1))
			}
			capPoolV6Quote(nextBuyerQuote)
			if nextVirtual.Sign() > 0 && nextBuyerQuote.Sign() > 0 &&
				new(big.Int).Add(nextVirtual, nextBuyerQuote).Cmp(poolV6MaxAmount) <= 0 {
				bond := new(big.Int).Sub(nextBuyerQuote, payout)
				if bond.Sign() < 0 {
					bond.SetInt64(0)
				}
				bond.Add(bond, new(big.Int).Mul(fee, big.NewInt(poolV6SellerProofGas*poolV6SellerFeeMargin)))
				unprovedFunds := new(big.Int).Add(payout, bond)
				if bond.Cmp(poolV6MaxAmount) > 0 || unprovedFunds.Cmp(poolV6MaxAmount) > 0 {
					return nil, errors.New("v6 pool seller funding is outside contract amount range")
				}
				pool.NextSellerPayout = payout.String()
				pool.MinimumProvedSellerBond = bond.String()
				pool.MinimumUnprovedSellerFunds = unprovedFunds.String()
			}
		}
	}
	if x > 1 {
		denominator := new(big.Int).SetUint64(x - 1)
		quote, remainder := new(big.Int).QuoRem(virtual, denominator, new(big.Int))
		if remainder.Sign() != 0 {
			quote.Add(quote, big.NewInt(1))
		}
		capPoolV6Quote(quote)
		if oldest != ([32]byte{}) && quote.Cmp(offers[pool.NextOfferID].payout) < 0 {
			quote.Set(offers[pool.NextOfferID].payout)
		}
		if quote.Sign() > 0 && new(big.Int).Add(virtual, quote).Cmp(poolV6MaxAmount) <= 0 {
			pool.BuyQuote = quote.String()
			if oldest != ([32]byte{}) && !offers[pool.NextOfferID].publicSeed {
				if fee == nil {
					return nil, errors.New("v6 pool has no current fee quote")
				}
				buyerBond := new(big.Int).Mul(fee, big.NewInt(poolV6BuyerProofGas*poolV6BuyerFeeMargin))
				buyerTotal := new(big.Int).Add(quote, buyerBond)
				if buyerBond.Sign() == 0 || buyerBond.Cmp(poolV6MaxAmount) > 0 || buyerTotal.Cmp(poolV6MaxAmount) > 0 {
					return nil, errors.New("v6 pool buyer funding is outside contract amount range")
				}
				pool.MinimumBuyerBond = buyerBond.String()
				pool.MinimumBuyerTotal = buyerTotal.String()
				required := new(big.Int).Sub(quote, offers[pool.NextOfferID].payout)
				if required.Sign() < 0 {
					required.SetInt64(0)
				}
				required.Add(required, new(big.Int).Mul(fee, big.NewInt(poolV6SellerProofGas*poolV6SellerFeeMargin)))
				if required.Cmp(poolV6MaxAmount) > 0 {
					return nil, errors.New("v6 pool seller bond is outside contract amount range")
				}
				shortfall := new(big.Int).Sub(required, offers[pool.NextOfferID].sellerBond)
				if shortfall.Sign() > 0 {
					pool.BondTopUpRequired = shortfall.String()
					pool.BuyPausedForBond = true
				}
			}
		}
	}

	state := &directV6PoolState{
		Format: "fod.direct-pool-state", Version: 6, Scope: "live", Network: NetworkNamePinned,
		Contract: contract, CodeHash: codeHash, BlockHeight: before.Height, Source: "node",
		Pool: pool, Offers: []directV6PoolOffer{}, Orders: []directV6PoolOrder{},
	}
	labels := []string{"qualified", "queued-primary", "queued-secondary", "active", "sold-private", "cancelled", "retired"}
	for _, id := range offerIDs {
		o := offers[id]
		entry := directV6PoolOffer{
			OfferID: id, Author: hex.EncodeToString(o.author[:]), Epoch: o.epoch, CoveredFlips: o.covered,
			OriginalCIDs:        publicInventories[id],
			InventoryCommitment: hex.EncodeToString(o.inventory[:]), KeysCommitment: hex.EncodeToString(o.keys[:]),
			Payout: o.payout.String(), SellerBond: o.sellerBond.String(), FraudCollateral: o.fraudCollateral.String(),
			NextNonce: strconv.FormatUint(o.nextNonce, 10), OpenOrderID: strconv.FormatUint(o.openOrder, 10),
			State: labels[o.status], EverSold: o.everSold, ContentProven: o.contentProven, PublicSeed: o.publicSeed,
			QueuePosition: positions[id], Sequence: strconv.FormatUint(o.sequence, 10), ListedAtBlock: strconv.FormatUint(o.listedAt, 10),
		}
		if o.status == 1 {
			entry.Queue = "primary"
		} else if o.status == 2 {
			entry.Queue = "secondary"
		}
		state.Offers = append(state.Offers, entry)
	}
	for _, id := range orderIDs {
		o := orders[id]
		if o.fundedAt > ^uint64(0)-poolV6ProofWindow {
			return nil, errors.New("v6 pool order refund height overflows")
		}
		refundAt := o.fundedAt + poolV6ProofWindow
		state.Orders = append(state.Orders, directV6PoolOrder{
			OrderID: strconv.FormatUint(id, 10), OfferID: hex.EncodeToString(o.offer[:]), Buyer: hex.EncodeToString(o.buyer[:]),
			Paid: o.paid.String(), Bond: o.bond.String(), SellerPayout: o.sellerPayout.String(),
			FundedAtBlock: strconv.FormatUint(o.fundedAt, 10), RefundAtBlock: strconv.FormatUint(refundAt, 10),
			Nonce: strconv.FormatUint(o.nonce, 10), State: directOrderStateLabel(DirectOrderState(o.state)),
			Challenged: o.challenged, RefundAvailable: (o.state == 1 || o.state == 4) && before.Height >= refundAt,
		})
	}
	var after rpcBlock
	if err := call("bcn_lastBlock", []any{}, &after); err != nil {
		return nil, fmt.Errorf("recheck v6 pool head: %w", err)
	}
	if before.Height != after.Height || before.Hash != after.Hash {
		return nil, errors.New("v6 pool head changed during snapshot; retry")
	}
	state.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	return state, nil
}
