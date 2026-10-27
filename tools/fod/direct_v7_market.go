package main

// A bounded, read-only v7 market snapshot. Storage is read from one loopback
// node and accepted only while its reported chain head remains unchanged.

import (
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

	"github.com/idena-network/idena-go/crypto"
)

type poolV7MarketOffer struct {
	OfferID      string   `json:"offerId"`
	Author       string   `json:"author"`
	Epoch        uint16   `json:"epoch"`
	Covered      uint32   `json:"covered"`
	OriginalCIDs []string `json:"originalCids"`
	Status       string   `json:"status"`
	PublicSeed   bool     `json:"publicSeed"`
	EverSold     bool     `json:"everSold"`
}

type poolV7MarketOrder struct {
	OrderID            string `json:"orderId"`
	OfferID            string `json:"offerId"`
	Author             string `json:"author"`
	Buyer              string `json:"buyer"`
	State              string `json:"state"`
	FundedAt           uint64 `json:"fundedAt"`
	ProofDeadline      uint64 `json:"proofDeadline"`
	Challenged         bool   `json:"challenged"`
	InvalidDeliveryKey bool   `json:"invalidDeliveryKey"`
}

type poolV7MarketSnapshot struct {
	Format         string              `json:"format"`
	Version        int                 `json:"version"`
	Network        string              `json:"network"`
	Contract       string              `json:"contract"`
	CodeHash       string              `json:"codeHash"`
	ObservedAt     string              `json:"observedAt"`
	BlockHeight    uint64              `json:"blockHeight"`
	ActiveIdentity string              `json:"activeIdentity"`
	SeedID         string              `json:"seedId"`
	PrimaryHead    string              `json:"primaryHead"`
	SecondaryHead  string              `json:"secondaryHead"`
	PrimaryUnits   string              `json:"primaryUnits"`
	SecondaryUnits string              `json:"secondaryUnits"`
	Reserve        string              `json:"reserve"`
	Insurance      string              `json:"insurance"`
	Quote          *string             `json:"quote"`
	NextOffer      *poolV7MarketOffer  `json:"nextOffer"`
	OpenOrders     []poolV7MarketOrder `json:"openOrders"`
}

type poolV7SyncStatus struct {
	Syncing      *bool   `json:"syncing"`
	CurrentBlock *uint64 `json:"currentBlock"`
	HighestBlock *uint64 `json:"highestBlock"`
	WrongTime    *bool   `json:"wrongTime"`
}

var errNodeNotSynced = errors.New("v7 node is not confirmed synced")

func readPoolV7SyncedHead(c *client) (rpcBlock, error) {
	var sync poolV7SyncStatus
	if err := c.call("bcn_syncing", []any{}, &sync); err != nil {
		return rpcBlock{}, fmt.Errorf("read v7 node sync: %w", err)
	}
	if sync.Syncing == nil || sync.CurrentBlock == nil || sync.HighestBlock == nil || sync.WrongTime == nil ||
		*sync.Syncing || *sync.WrongTime || *sync.CurrentBlock != *sync.HighestBlock {
		return rpcBlock{}, errNodeNotSynced
	}
	var head rpcBlock
	if err := c.call("bcn_lastBlock", []any{}, &head); err != nil {
		return rpcBlock{}, fmt.Errorf("read v7 market head: %w", err)
	}
	if head.Height == 0 || head.Height > directMarketMaxJSONHeight ||
		!isHex(strings.ToLower(strings.TrimPrefix(head.Hash, "0x")), 32) || head.Height != *sync.CurrentBlock {
		return rpcBlock{}, errors.New("invalid or unsynced v7 market head")
	}
	return head, nil
}

// readPoolV7MarketSnapshot is reusable by a local UI. The caller supplies its
// own deployment pins; neither the contract nor its hash comes from an offer.
func readPoolV7MarketSnapshot(c *client, contract, codeHash string) (*poolV7MarketSnapshot, error) {
	if c == nil || c.http == nil || !directLocalRPC(c.url) || c.key == "" {
		return nil, errors.New("v7 market needs a keyed loopback node RPC")
	}
	var err error
	contract, err = directAddress(contract)
	if err != nil {
		return nil, err
	}
	codeHash = strings.ToLower(strings.TrimPrefix(codeHash, "0x"))
	if !isHex(codeHash, 32) {
		return nil, errors.New("v7 market needs an independent 32-byte code hash")
	}
	before, err := readPoolV7SyncedHead(c)
	if err != nil {
		return nil, err
	}
	if err := checkBuyTarget(c, contract, contract, codeHash); err != nil {
		return nil, err
	}
	var identity string
	if err := c.call("dna_getCoinbaseAddr", []any{}, &identity); err != nil {
		return nil, fmt.Errorf("read active v7 identity: %w", err)
	}
	identity, err = directAddress(identity)
	if err != nil {
		return nil, fmt.Errorf("invalid active v7 identity: %w", err)
	}
	if identity == strings.Repeat("0", addressBytes*2) {
		return nil, errors.New("v7 node has no active identity")
	}

	ids := make(map[string][32]byte)
	for _, name := range []string{"seed-id", "primary-head", "primary-tail", "secondary-head", "secondary-tail"} {
		ids[name], err = readPoolV7ID(c, contract, "pool/"+name)
		if err != nil {
			return nil, err
		}
	}
	units := make(map[string]uint64)
	for _, name := range []string{"primary-units", "secondary-units", "units", "reserved-units", "seed-count", "next-order", "next-sequence", "activated-at"} {
		units[name], err = readPoolV7U64(c, contract, "pool/"+name)
		if err != nil {
			return nil, err
		}
	}
	reserve, err := readPoolV7U128(c, contract, "pool/reserve")
	if err != nil {
		return nil, err
	}
	insurance, err := readPoolV7U128(c, contract, "pool/insurance")
	if err != nil {
		return nil, err
	}
	if ids["seed-id"] == ([32]byte{}) || units["next-order"] == 0 || units["next-sequence"] == 0 ||
		units["primary-units"] > ^uint64(0)-units["secondary-units"] ||
		units["activated-at"] > before.Height {
		return nil, errors.New("invalid v7 market counters or seed")
	}

	count := 0
	offerRecords, err := readDirectMarketPrefix(c, registryAddress(contract), poolV6OfferPrefix, &count)
	if err != nil {
		return nil, err
	}
	orderRecords, err := readDirectMarketPrefix(c, registryAddress(contract), poolV6OrderPrefix, &count)
	if err != nil {
		return nil, err
	}
	offers := make(map[string]*poolV7Offer, len(offerRecords))
	var maxSequence uint64
	for key, raw := range offerRecords {
		if len(key) != 64 {
			return nil, errors.New("v7 offer storage key is not a 32-byte ID")
		}
		offer, decodeErr := decodePoolV7Offer(raw)
		if decodeErr != nil {
			return nil, fmt.Errorf("invalid v7 offer record %s", key)
		}
		derived := poolV7OfferID(offer.author, offer.epoch)
		if offer.author == ([20]byte{}) || hex.EncodeToString(derived[:]) != key ||
			offer.listedAt > before.Height || offer.sequence >= units["next-sequence"] {
			return nil, fmt.Errorf("v7 offer %s has invalid identity or sequence", key)
		}
		if offer.sequence > maxSequence {
			maxSequence = offer.sequence
		}
		offers[key] = offer
	}
	if maxSequence >= units["next-sequence"] {
		return nil, errors.New("v7 next sequence is stale")
	}

	orders := make(map[uint64]*poolV7Order, len(orderRecords))
	var maxOrderID uint64
	v7PriceCap, _ := new(big.Int).SetString(poolV7PriceCap, 10)
	for key, raw := range orderRecords {
		keyBytes, decodeErr := hex.DecodeString(key)
		if decodeErr != nil {
			return nil, errors.New("invalid v7 order storage key")
		}
		id, parseErr := strconv.ParseUint(string(keyBytes), 10, 64)
		if parseErr != nil || id == 0 || strconv.FormatUint(id, 10) != string(keyBytes) {
			return nil, errors.New("v7 order storage key is not canonical")
		}
		order, decodeErr := decodePoolV7Order(raw)
		if decodeErr != nil {
			return nil, fmt.Errorf("invalid v7 order %d: %w", id, decodeErr)
		}
		offer := offers[hex.EncodeToString(order.offer[:])]
		if offer == nil || offer.publicSeed || order.buyer == ([20]byte{}) || order.nonce == 0 || order.nonce >= offer.nextNonce ||
			order.fundedAt == 0 || order.fundedAt > before.Height || order.paid.Sign() == 0 || order.bond.Sign() == 0 ||
			order.paid.Cmp(v7PriceCap) > 0 || order.sellerPayout.Cmp(order.paid) > 0 {
			return nil, fmt.Errorf("v7 order %d has invalid offer reference or amount", id)
		}
		if order.state == 1 || order.state == 4 {
			expected := order.paid
			if offer.everSold {
				expected = new(big.Int).Rsh(new(big.Int).Set(order.paid), 1)
			}
			if offer.status != 3 || offer.openOrder != id || order.sellerPayout.Cmp(expected) != 0 {
				return nil, fmt.Errorf("v7 order %d disagrees with its active offer", id)
			}
		}
		orders[id] = order
		if id > maxOrderID {
			maxOrderID = id
		}
	}
	if maxOrderID >= units["next-order"] {
		return nil, errors.New("v7 next order is stale")
	}

	positions := make(map[string]bool)
	for _, queue := range []struct {
		name   string
		status byte
	}{{"primary", 1}, {"secondary", 2}} {
		var previous [32]byte
		visited := uint64(0)
		for current := ids[queue.name+"-head"]; current != ([32]byte{}); {
			key := hex.EncodeToString(current[:])
			offer := offers[key]
			if offer == nil || offer.status != queue.status || offer.openOrder != 0 ||
				offer.previous != previous || positions[key] || offer.sequence == 0 || offer.listedAt == 0 ||
				(queue.status == 1 && (offer.publicSeed || offer.everSold || offer.sellerBond.Sign() == 0)) ||
				(queue.status == 2 && ((!offer.publicSeed && (!offer.everSold || offer.sellerBond.Sign() == 0)) ||
					!offer.contentProven || (offer.publicSeed && (offer.sellerBond.Sign() != 0 || offer.fraudCollateral.Sign() != 0)))) {
				return nil, fmt.Errorf("v7 %s queue has invalid offer link", queue.name)
			}
			visited++
			if visited > units[queue.name+"-units"] || len(positions) >= directMarketMaxItems {
				return nil, fmt.Errorf("v7 %s queue exceeds its unit count", queue.name)
			}
			positions[key] = true
			previous, current = current, offer.next
		}
		if previous != ids[queue.name+"-tail"] || visited != units[queue.name+"-units"] {
			return nil, fmt.Errorf("v7 %s queue differs from its head, tail, or unit count", queue.name)
		}
	}
	active := uint64(0)
	for key, offer := range offers {
		if offer.status == 1 || offer.status == 2 {
			if !positions[key] {
				return nil, errors.New("v7 queued offer is not linked")
			}
		} else if offer.previous != ([32]byte{}) || offer.next != ([32]byte{}) {
			return nil, errors.New("v7 unqueued offer has queue links")
		}
		if offer.status == 3 {
			active++
			order := orders[offer.openOrder]
			if offer.publicSeed || offer.openOrder == 0 || order == nil ||
				(order.state != 1 && order.state != 4) || order.offer != poolV7OfferID(offer.author, offer.epoch) {
				return nil, errors.New("v7 active offer has no live order")
			}
		}
	}
	queued := units["primary-units"] + units["secondary-units"]
	if queued > ^uint64(0)-active || units["reserved-units"] != active || units["units"] != queued+active {
		return nil, errors.New("v7 unit counters disagree with queue and orders")
	}
	seedID, primaryHead, secondaryHead := ids["seed-id"], ids["primary-head"], ids["secondary-head"]
	seedKey := hex.EncodeToString(seedID[:])
	seed := offers[seedKey]
	if reserve.Sign() == 0 {
		if units["activated-at"] != 0 || units["seed-count"] > 1 || insurance.Sign() != 0 ||
			(primaryHead != ([32]byte{}) || secondaryHead != ([32]byte{})) ||
			units["units"] != 0 || len(orders) != 0 {
			return nil, errors.New("v7 market is not activated")
		}
	} else if units["seed-count"] != 1 || units["activated-at"] == 0 || reserve.Cmp(insurance) <= 0 ||
		seed == nil || !seed.publicSeed || seed.status != 2 || !seed.contentProven ||
		!positions[seedKey] {
		return nil, errors.New("v7 seed or reserve is inconsistent")
	}

	selected := primaryHead
	if selected == ([32]byte{}) {
		selected = secondaryHead
	}
	state := &poolV7MarketSnapshot{
		Format: "fod.direct-v7-market", Version: 7, Network: NetworkNamePinned,
		Contract: contract, CodeHash: codeHash, BlockHeight: before.Height,
		ActiveIdentity: identity, SeedID: seedKey,
		PrimaryHead: hex.EncodeToString(primaryHead[:]), SecondaryHead: hex.EncodeToString(secondaryHead[:]),
		PrimaryUnits:   strconv.FormatUint(units["primary-units"], 10),
		SecondaryUnits: strconv.FormatUint(units["secondary-units"], 10),
		Reserve:        reserve.String(), Insurance: insurance.String(),
		OpenOrders: []poolV7MarketOrder{},
	}
	orderIDs := make([]uint64, 0, len(orders))
	for id, order := range orders {
		if order.state == 1 || order.state == 4 {
			orderIDs = append(orderIDs, id)
		}
	}
	sort.Slice(orderIDs, func(i, j int) bool { return orderIDs[i] < orderIDs[j] })
	for _, id := range orderIDs {
		order := orders[id]
		offer := offers[hex.EncodeToString(order.offer[:])]
		if order.fundedAt > ^uint64(0)-poolV7ProofWindow {
			return nil, fmt.Errorf("v7 order %d has an invalid proof deadline", id)
		}
		label := "funded"
		if order.state == 4 {
			label = "fallback pending"
		}
		_, keyErr := crypto.UnmarshalPubkey(order.deliveryKey[:])
		state.OpenOrders = append(state.OpenOrders, poolV7MarketOrder{
			OrderID: strconv.FormatUint(id, 10), OfferID: hex.EncodeToString(order.offer[:]),
			Author: hex.EncodeToString(offer.author[:]), Buyer: hex.EncodeToString(order.buyer[:]), State: label,
			FundedAt: order.fundedAt, ProofDeadline: order.fundedAt + poolV7ProofWindow, Challenged: order.challenged,
			InvalidDeliveryKey: order.deliveryKey[0] != 4 || keyErr != nil,
		})
	}
	if selected != ([32]byte{}) {
		offer := offers[hex.EncodeToString(selected[:])]
		if offer == nil {
			return nil, errors.New("v7 selected head has no offer")
		}
		quote, quoteErr := poolV7Quote(queued, reserve, insurance)
		if quoteErr != nil {
			return nil, quoteErr
		}
		quoteText := quote.String()
		state.Quote = &quoteText
		inventoryCount := 0
		inventoryRecords, inventoryErr := readDirectMarketPrefix(c, registryAddress(contract), poolV6InventoryPrefix, &inventoryCount)
		if inventoryErr != nil {
			return nil, inventoryErr
		}
		inventory, ok := inventoryRecords[hex.EncodeToString(selected[:])]
		if !ok {
			return nil, errors.New("v7 selected offer has no public CID inventory")
		}
		cids, inventoryErr := decodePoolV6Inventory(inventory, poolV6OfferMeta{
			author: offer.author, epoch: offer.epoch, covered: offer.covered, inventory: offer.inventory,
		})
		if inventoryErr != nil {
			return nil, fmt.Errorf("v7 selected offer has invalid public CID inventory: %w", inventoryErr)
		}
		labels := [...]string{"qualified", "queued-primary", "queued-secondary", "active", "sold-private", "cancelled", "retired"}
		state.NextOffer = &poolV7MarketOffer{
			OfferID: hex.EncodeToString(selected[:]), Author: hex.EncodeToString(offer.author[:]),
			Epoch: offer.epoch, Covered: offer.covered, OriginalCIDs: cids, Status: labels[offer.status],
			PublicSeed: offer.publicSeed, EverSold: offer.everSold,
		}
	}
	after, err := readPoolV7SyncedHead(c)
	if err != nil {
		return nil, err
	}
	if before.Height != after.Height || before.Hash != after.Hash {
		return nil, errors.New("v7 market head changed during snapshot; retry")
	}
	state.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	return state, nil
}

func writePoolV7MarketSnapshot(path string, state *poolV7MarketSnapshot) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if path == "-" {
		_, err = os.Stdout.Write(raw)
		return err
	}
	return writeNewPoolV7PrivateFile(path, raw)
}
