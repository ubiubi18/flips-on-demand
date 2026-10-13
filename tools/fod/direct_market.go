package main

// Public direct-sale state for a local interface. Private delivery keys,
// ciphertext, receipts, and revealed flip keys are never part of this format.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const directMarketMaxItems = 5000
const directMarketMaxJSONHeight = 1<<53 - 1

type directMarketOffer struct {
	OfferID             string `json:"offerId"`
	Author              string `json:"author"`
	Epoch               uint16 `json:"epoch"`
	CoveredFlips        uint32 `json:"coveredFlips"`
	InventoryCommitment string `json:"inventoryCommitment"`
	Price               string `json:"price"`
	OpenOrderID         string `json:"openOrderId"`
	Retired             bool   `json:"retired"`
}

type directMarketOrder struct {
	OrderID       string `json:"orderId"`
	OfferID       string `json:"offerId"`
	Buyer         string `json:"buyer"`
	Paid          string `json:"paid"`
	Bond          string `json:"bond"`
	SellerStake   string `json:"sellerStake,omitempty"`
	FundedAtBlock string `json:"fundedAtBlock"`
	RefundAtBlock string `json:"refundAtBlock"`
	State         string `json:"state"`
	KeysPublic    bool   `json:"keysPublic"`
}

type directMarketState struct {
	Format      string              `json:"format"`
	Version     int                 `json:"version"`
	Network     string              `json:"network"`
	Contract    string              `json:"contract"`
	CodeHash    string              `json:"codeHash"`
	BlockHeight uint64              `json:"blockHeight"`
	ObservedAt  string              `json:"observedAt"`
	Source      string              `json:"source"`
	Offers      []directMarketOffer `json:"offers"`
	Orders      []directMarketOrder `json:"orders"`
}

// The map RPC returns suffix keys as hex. Keep this reader local to the export:
// the shared readPrefix has no item limit and silently overwrites duplicate keys.
func readDirectMarketPrefix(c *client, contract, prefix string, count *int) (map[string][]byte, error) {
	records := make(map[string][]byte)
	seenTokens := make(map[string]bool)
	var token any
	for {
		var page mapPage
		if err := c.call("contract_iterateMap", []any{contract, prefix, token, "hex", "hex", marketPageLimit}, &page); err != nil {
			return nil, fmt.Errorf("read %s: %w", prefix, err)
		}
		if len(page.Items) > marketPageLimit || *count+len(page.Items) > directMarketMaxItems {
			return nil, fmt.Errorf("direct market exceeds the %d-item limit", directMarketMaxItems)
		}
		for _, item := range page.Items {
			if item == nil {
				return nil, fmt.Errorf("read %s: empty map item", prefix)
			}
			key, err := decodeHexField(item.Key)
			if err != nil {
				return nil, fmt.Errorf("read %s: invalid key: %w", prefix, err)
			}
			value, err := decodeHexField(item.Value)
			if err != nil {
				return nil, fmt.Errorf("read %s: invalid value: %w", prefix, err)
			}
			canonical := hex.EncodeToString(key)
			if _, exists := records[canonical]; exists {
				return nil, fmt.Errorf("read %s: duplicate key", prefix)
			}
			records[canonical] = value
			*count += 1
		}
		if page.ContinuationToken == nil || strings.TrimPrefix(*page.ContinuationToken, "0x") == "" {
			return records, nil
		}
		if len(page.Items) == 0 || seenTokens[*page.ContinuationToken] {
			return nil, fmt.Errorf("read %s: invalid continuation", prefix)
		}
		seenTokens[*page.ContinuationToken] = true
		token = *page.ContinuationToken
	}
}

func buildDirectMarketState(c *client, contract, codeHash string) (*directMarketState, error) {
	return buildDirectMarketStateWithVersion(c, contract, codeHash, 3)
}

func buildDirectMarketStateWithVersion(c *client, contract, codeHash string, version int) (*directMarketState, error) {
	if version != 3 && version != 4 && version != 5 && version != 0 {
		return nil, errors.New("unsupported direct market contract version")
	}
	usesStake := version == 4 || version == 5
	var before rpcBlock
	if err := c.call("bcn_lastBlock", []any{}, &before); err != nil {
		return nil, fmt.Errorf("read direct market head: %w", err)
	}
	if before.Height > directMarketMaxJSONHeight {
		return nil, errors.New("direct market block height is outside safe JSON integer range")
	}
	state := &directMarketState{
		Format: "fod.direct-market-state", Version: 2, Network: NetworkNamePinned,
		Contract: contract, CodeHash: codeHash, BlockHeight: before.Height,
		Source: "node", Offers: []directMarketOffer{}, Orders: []directMarketOrder{},
	}
	if usesStake {
		state.Version = 3
	}
	count := 0
	offerRecords, err := readDirectMarketPrefix(c, registryAddress(contract), directOfferPrefix, &count)
	if err != nil {
		return nil, err
	}
	orderRecords, err := readDirectMarketPrefix(c, registryAddress(contract), directOrderPrefix, &count)
	if err != nil {
		return nil, err
	}
	prefix := directIntentPrefix
	if usesStake {
		prefix = directStakePrefix
	}
	stakeRecords, err := readDirectMarketPrefix(c, registryAddress(contract), prefix, &count)
	if err != nil {
		return nil, err
	}

	offers := make(map[string]*DirectOffer, len(offerRecords))
	offerIDs := make([]string, 0, len(offerRecords))
	for key, raw := range offerRecords {
		id, err := hex.DecodeString(key)
		if err != nil || len(id) != 32 {
			return nil, errors.New("direct offer key is not a 32-byte ID")
		}
		offer, err := DecodeDirectOffer(raw)
		if err != nil {
			return nil, fmt.Errorf("direct offer %s: %w", key, err)
		}
		derivedID := DirectOfferID(offer.Author, offer.Epoch)
		if hex.EncodeToString(derivedID[:]) != key {
			return nil, fmt.Errorf("direct offer %s differs from its author and epoch", key)
		}
		if offer.Covered == 0 || offer.Covered > 32 || offer.Inventory == ([32]byte{}) ||
			offer.Keys == ([32]byte{}) || offer.Price.Sign() <= 0 || offer.NextNonce == 0 ||
			(offer.Retired && offer.OpenOrderID != 0) {
			return nil, fmt.Errorf("direct offer %s has invalid state", key)
		}
		offers[key] = offer
		offerIDs = append(offerIDs, key)
	}
	sort.Strings(offerIDs)
	for _, id := range offerIDs {
		offer := offers[id]
		state.Offers = append(state.Offers, directMarketOffer{
			OfferID: id, Author: hex.EncodeToString(offer.Author[:]), Epoch: offer.Epoch,
			CoveredFlips: offer.Covered, InventoryCommitment: hex.EncodeToString(offer.Inventory[:]),
			Price: offer.Price.String(), OpenOrderID: strconv.FormatUint(offer.OpenOrderID, 10), Retired: offer.Retired,
		})
	}

	orders := make(map[uint64]*DirectOrder, len(orderRecords))
	orderIDs := make([]uint64, 0, len(orderRecords))
	for key, raw := range orderRecords {
		keyBytes, err := hex.DecodeString(key)
		if err != nil {
			return nil, errors.New("direct order key is invalid hex")
		}
		id, err := strconv.ParseUint(string(keyBytes), 10, 64)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != string(keyBytes) {
			return nil, errors.New("direct order key is not a canonical positive ID")
		}
		var order *DirectOrder
		if usesStake {
			order, err = DecodeDirectOrderV4(raw)
		} else {
			order, err = DecodeDirectOrder(raw)
		}
		if err != nil {
			return nil, fmt.Errorf("direct order %d: %w", id, err)
		}
		offerID := hex.EncodeToString(order.OfferID[:])
		offer, exists := offers[offerID]
		if !exists || order.Nonce == 0 || order.Paid.Cmp(offer.Price) != 0 ||
			order.Nonce >= offer.NextNonce || ((order.State == DirectFunded || order.State == DirectFallbackPending || order.State == DirectFallbackIntent) && (offer.Retired || offer.OpenOrderID != id)) ||
			(order.State != DirectSettled && order.DeliveryHash != ([32]byte{})) ||
			(order.State == DirectSettled && order.DeliveryHash == ([32]byte{}) && !offer.Retired) {
			return nil, fmt.Errorf("direct order %d has invalid offer reference or state", id)
		}
		if _, err := directFundedAmount(order.Paid, order.Bond); err != nil {
			return nil, fmt.Errorf("direct order %d has invalid funding: %w", id, err)
		}
		orders[id] = order
		orderIDs = append(orderIDs, id)
	}
	sort.Slice(orderIDs, func(i, j int) bool { return orderIDs[i] < orderIDs[j] })
	for _, id := range orderIDs {
		order := orders[id]
		refundAt := DirectProofDeadline(order.FundedAt)
		var sellerStake string
		if !usesStake && order.State == DirectFallbackIntent {
			raw, exists := stakeRecords[hex.EncodeToString([]byte(strconv.FormatUint(id, 10)))]
			if !exists {
				return nil, fmt.Errorf("direct order %d has no included fallback intent", id)
			}
			intent, err := DecodeDirectIntent(raw)
			if err != nil {
				return nil, fmt.Errorf("direct order %d: %w", id, err)
			}
			if err := validateDirectIntent(intent, offers[hex.EncodeToString(order.OfferID[:])], order, before.Height); err != nil {
				return nil, fmt.Errorf("direct order %d: %w", id, err)
			}
			var ok bool
			refundAt, ok = order.RefundAt(intent)
			if !ok {
				return nil, fmt.Errorf("direct order %d has no valid intent refund deadline", id)
			}
		} else if usesStake && order.State == DirectFallbackPending {
			raw, exists := stakeRecords[hex.EncodeToString([]byte(strconv.FormatUint(id, 10)))]
			if !exists {
				return nil, fmt.Errorf("v%d direct order %d has no included seller stake", version, id)
			}
			stake, err := DecodeDirectStake(raw)
			if err != nil || stake.Cmp(order.Paid) < 0 {
				return nil, fmt.Errorf("v%d direct order %d has invalid seller stake", version, id)
			}
			sellerStake = stake.String()
		}
		label := directOrderStateLabel(order.State)
		state.Orders = append(state.Orders, directMarketOrder{
			OrderID: strconv.FormatUint(id, 10), OfferID: hex.EncodeToString(order.OfferID[:]),
			Buyer: hex.EncodeToString(order.Buyer[:]), Paid: order.Paid.String(), Bond: order.Bond.String(),
			SellerStake:   sellerStake,
			FundedAtBlock: strconv.FormatUint(order.FundedAt, 10),
			RefundAtBlock: strconv.FormatUint(refundAt, 10), State: label,
			KeysPublic: order.State == DirectSettled && order.DeliveryHash == ([32]byte{}),
		})
	}
	for id, offer := range offers {
		if offer.OpenOrderID == 0 {
			continue
		}
		order, exists := orders[offer.OpenOrderID]
		if !exists || (order.State != DirectFunded && order.State != DirectFallbackPending && order.State != DirectFallbackIntent) || hex.EncodeToString(order.OfferID[:]) != id {
			return nil, fmt.Errorf("direct offer %s has invalid open order reference", id)
		}
	}

	var after rpcBlock
	if err := c.call("bcn_lastBlock", []any{}, &after); err != nil {
		return nil, fmt.Errorf("recheck direct market head: %w", err)
	}
	if before.Height != after.Height || before.Hash != after.Hash {
		return nil, errors.New("direct market head changed during export; retry")
	}
	state.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	return state, nil
}

func writeDirectMarketState(path string, state *directMarketState) error {
	return writeDirectMarketJSON(path, state)
}

func writeDirectMarketJSON(path string, state any) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	temporary := ".fod-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := root.Rename(temporary, filepath.Base(path)); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
