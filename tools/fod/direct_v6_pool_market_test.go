package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func poolV6TestAmount(value uint64) []byte {
	return new(big.Int).SetUint64(value).FillBytes(make([]byte, 16))
}

func poolV6TestCounter(value uint64) []byte {
	raw := make([]byte, 8)
	binary.BigEndian.PutUint64(raw, value)
	return raw
}

func poolV6TestCIDs(author byte) []string {
	cids := []string{cidRawSha256([]byte{author, 0}), cidRawSha256([]byte{author, 1})}
	sort.Strings(cids)
	return cids
}

func poolV6TestInventory(author byte) []byte {
	cids := poolV6TestCIDs(author)
	raw := []byte{byte(len(cids))}
	for _, cid := range cids {
		raw = append(raw, byte(len(cid)))
		raw = append(raw, cid...)
	}
	return raw
}

func poolV6TestOffer(author byte, status byte, previous, next [32]byte, payout, openOrder, nextNonce uint64) []byte {
	raw := make([]byte, poolV6OfferBytes)
	authorAddress := [20]byte{author}
	raw[0] = author
	binary.BigEndian.PutUint16(raw[20:22], 230)
	binary.BigEndian.PutUint32(raw[22:26], 2)
	commitment, _ := hex.DecodeString(inventoryCommitment(hex.EncodeToString(authorAddress[:]), 230, poolV6TestCIDs(author)))
	copy(raw[26:58], commitment)
	copy(raw[58:90], bytes.Repeat([]byte{0x42}, 32))
	copy(raw[90:106], poolV6TestAmount(payout))
	copy(raw[106:122], poolV6TestAmount(20))
	binary.BigEndian.PutUint64(raw[122:130], nextNonce)
	binary.BigEndian.PutUint64(raw[130:138], openOrder)
	raw[138] = status
	if status == 2 {
		raw[139] = 1
	}
	copy(raw[140:172], previous[:])
	copy(raw[172:204], next[:])
	binary.BigEndian.PutUint64(raw[204:212], uint64(author))
	binary.BigEndian.PutUint64(raw[212:220], 90)
	raw[220] = 1
	return raw
}

func poolV6TestOrder(offer [32]byte) []byte {
	raw := make([]byte, poolV6OrderBytes)
	copy(raw[:32], offer[:])
	raw[32] = 0x51
	copy(raw[52:117], bytes.Repeat([]byte{0xe1}, 65))
	copy(raw[117:133], poolV6TestAmount(40))
	copy(raw[133:149], poolV6TestAmount(5))
	copy(raw[149:165], poolV6TestAmount(30))
	binary.BigEndian.PutUint64(raw[165:173], 100)
	binary.BigEndian.PutUint64(raw[173:181], 1)
	raw[181] = 1
	copy(raw[182:214], bytes.Repeat([]byte{0xe2}, 32))
	raw[214] = 1
	return raw
}

type poolV6TestNode struct {
	fields    map[string][]byte
	items     map[string][]*mapItem
	heads     []rpcBlock
	headReads int
}

func newPoolV6TestNode() (*poolV6TestNode, [4][32]byte) {
	var ids [4][32]byte
	for i := range ids {
		ids[i] = poolV6OfferID([20]byte{byte(i + 1)}, 230)
	}
	items := map[string][]*mapItem{
		poolV6OfferPrefix: {
			directMarketItem(ids[0][:], poolV6TestOffer(1, 1, [32]byte{}, ids[1], 10, 0, 1)),
			directMarketItem(ids[1][:], poolV6TestOffer(2, 1, ids[0], [32]byte{}, 10, 0, 1)),
			directMarketItem(ids[2][:], poolV6TestOffer(3, 2, [32]byte{}, [32]byte{}, 0, 0, 1)),
			directMarketItem(ids[3][:], poolV6TestOffer(4, 3, [32]byte{}, [32]byte{}, 30, 1, 2)),
		},
		poolV6OrderPrefix: {directMarketItem([]byte("1"), poolV6TestOrder(ids[3]))},
		poolV6InventoryPrefix: {
			directMarketItem(ids[0][:], poolV6TestInventory(1)),
			directMarketItem(ids[1][:], poolV6TestInventory(2)),
			directMarketItem(ids[2][:], poolV6TestInventory(3)),
			directMarketItem(ids[3][:], poolV6TestInventory(4)),
		},
	}
	fields := map[string][]byte{
		"pool/reserve": poolV6TestAmount(120), "pool/committed": poolV6TestAmount(50),
		"pool/buyer-payments": poolV6TestAmount(40), "pool/insurance": poolV6TestAmount(6),
		"pool/pending": poolV6TestAmount(125), "pool/primary-units": poolV6TestCounter(2),
		"pool/secondary-units": poolV6TestCounter(1), "pool/units": poolV6TestCounter(4),
		"pool/reserved-units": poolV6TestCounter(1), "pool/next-order": poolV6TestCounter(2),
		"pool/seed-count":    poolV6TestCounter(2),
		"pool/next-sequence": poolV6TestCounter(5),
		"pool/primary-head":  ids[0][:], "pool/primary-tail": ids[1][:],
		"pool/secondary-head": ids[2][:], "pool/secondary-tail": ids[2][:],
		"pool/seed-first": ids[2][:], "pool/seed-second": ids[3][:],
	}
	return &poolV6TestNode{fields: fields, items: items, heads: []rpcBlock{{Height: 120, Hash: "0xabc"}, {Height: 120, Hash: "0xabc"}}}, ids
}

func (n *poolV6TestNode) call(method string, params any, result any) error {
	switch method {
	case "bcn_lastBlock":
		if n.headReads >= len(n.heads) {
			return errors.New("unexpected head read")
		}
		*result.(*rpcBlock) = n.heads[n.headReads]
		n.headReads++
	case "bcn_feePerGas":
		*result.(*json.RawMessage) = json.RawMessage("1")
	case "contract_iterateMap":
		p := params.([]any)
		if len(p) != 6 || p[0] != "0x"+testContract ||
			p[3] != "hex" || p[4] != "hex" || p[5] != marketPageLimit {
			return errors.New("unexpected map parameters")
		}
		items, ok := n.items[p[1].(string)]
		if !ok {
			return errors.New("unexpected storage prefix")
		}
		start := 0
		if p[2] != nil {
			var err error
			token, tokenOK := p[2].(string)
			if !tokenOK {
				return errors.New("unexpected continuation token")
			}
			start, err = strconv.Atoi(token)
			if err != nil || start < 1 || start >= len(items) {
				return errors.New("invalid continuation token")
			}
		}
		end := start + marketPageLimit
		if end > len(items) {
			end = len(items)
		}
		page := mapPage{Items: items[start:end]}
		if end < len(items) {
			continuation := strconv.Itoa(end)
			page.ContinuationToken = &continuation
		}
		*result.(*mapPage) = page
	case "contract_readData":
		p := params.([]any)
		if len(p) != 3 || p[0] != "0x"+testContract || p[2] != "hex" {
			return errors.New("unexpected field parameters")
		}
		raw, ok := n.fields[p[1].(string)]
		if !ok {
			return errors.New("unexpected storage key")
		}
		*result.(*string) = "0x" + hex.EncodeToString(raw)
	default:
		return errors.New("unexpected RPC method")
	}
	return nil
}

func TestPoolV6SnapshotQuotesFIFOAndExcludesDeliveryMaterial(t *testing.T) {
	node, ids := newPoolV6TestNode()
	state, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash")
	if err != nil {
		t.Fatal(err)
	}
	if node.headReads != 2 || state.Version != 6 || state.Scope != "live" || state.BlockHeight != 120 || state.Pool.VirtualReserve != "104" ||
		state.Pool.BuyQuote != "52" || state.Pool.NextOfferID != hex.EncodeToString(ids[0][:]) ||
		state.Pool.SeedOfferIDs != [2]string{hex.EncodeToString(ids[2][:]), hex.EncodeToString(ids[3][:])} ||
		state.Pool.MinimumBuyerBond != "5000000" || state.Pool.MinimumBuyerTotal != "5000052" ||
		state.Pool.NextSellerPayout != "26" || state.Pool.MinimumProvedSellerBond != "5000000" ||
		state.Pool.MinimumUnprovedSellerFunds != "5000026" ||
		!state.Pool.BuyPausedForBond || state.Pool.BondTopUpRequired != "5000022" {
		t.Fatalf("unexpected stable pool snapshot: %+v", state.Pool)
	}
	if len(state.Offers) != 4 || len(state.Offers[0].OriginalCIDs) != 2 {
		t.Fatal("snapshot omitted the verified public flip IDs")
	}
	for i, id := range ids[:3] {
		var found *directV6PoolOffer
		for index := range state.Offers {
			if state.Offers[index].OfferID == hex.EncodeToString(id[:]) {
				found = &state.Offers[index]
			}
		}
		if found == nil || found.QueuePosition != i+1 {
			t.Fatalf("offer %d has wrong global FIFO position: %+v", i, found)
		}
		if i < 2 && found.Queue != "primary" || i == 2 && found.Queue != "secondary" {
			t.Fatalf("offer %d has wrong queue: %+v", i, found)
		}
	}
	if len(state.Orders) != 1 || state.Orders[0].State != "funded" || !state.Orders[0].Challenged ||
		state.Orders[0].RefundAtBlock != "13060" || state.Orders[0].RefundAvailable {
		t.Fatalf("unexpected public order state: %+v", state.Orders)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"deliveryKey", "deliveryHash", "ciphertext", "revealedSecrets", strings.Repeat("e1", 65), strings.Repeat("e2", 32)} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("snapshot exposed %s", forbidden)
		}
	}
}

func TestPoolV6SnapshotVerifiesRepeatSaleDividendAndPendingPoolCredit(t *testing.T) {
	node, ids := newPoolV6TestNode()
	repeat := poolV6TestOffer(4, 3, [32]byte{}, [32]byte{}, 0, 1, 2)
	repeat[139] = 1 // the original bundle was previously sold on-chain
	node.items[poolV6OfferPrefix][3] = directMarketItem(ids[3][:], repeat)
	order := poolV6TestOrder(ids[3])
	copy(order[149:165], poolV6TestAmount(20)) // half of the 40-unit buyer price
	node.items[poolV6OrderPrefix][0] = directMarketItem([]byte("1"), order)
	node.fields["pool/committed"] = poolV6TestAmount(20)
	node.fields["pool/buyer-payments"] = poolV6TestAmount(20)
	state, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash")
	if err != nil || state.Orders[0].SellerPayout != "20" || state.Pool.BuyerPayments != "20" {
		t.Fatalf("repeat sale did not preserve the 50/50 split: state=%+v err=%v", state, err)
	}
	node.headReads = 0
	copy(order[149:165], poolV6TestAmount(21))
	node.items[poolV6OrderPrefix][0] = directMarketItem([]byte("1"), order)
	if _, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash"); err == nil {
		t.Fatal("reader accepted a repeat-sale payout other than half the buyer price")
	}
}

func TestPoolV6SnapshotCapsQuotesAtOneThousandIDNA(t *testing.T) {
	node, _ := newPoolV6TestNode()
	reserve := new(big.Int).Mul(big.NewInt(10_000), big.NewInt(1_000_000_000_000_000_000))
	node.fields["pool/reserve"] = reserve.FillBytes(make([]byte, 16))
	state, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash")
	if err != nil {
		t.Fatal(err)
	}
	if state.Pool.BuyQuote != poolV6PriceCap.String() || state.Pool.NextSellerPayout != poolV6PriceCap.String() {
		t.Fatalf("pool quotes exceed the configured ceiling: %+v", state.Pool)
	}
}

func TestPoolV6SnapshotFloorsBuyerQuoteAtLockedPrimaryPayout(t *testing.T) {
	node, ids := newPoolV6TestNode()
	first := poolV6TestOffer(1, 1, [32]byte{}, ids[1], 60, 0, 1)
	node.items[poolV6OfferPrefix][0] = directMarketItem(ids[0][:], first)
	node.fields["pool/committed"] = poolV6TestAmount(100)
	state, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash")
	if err != nil {
		t.Fatal(err)
	}
	if state.Pool.VirtualReserve != "54" || state.Pool.BuyQuote != "60" ||
		state.Pool.MinimumBuyerTotal != "5000060" || state.Pool.NextOfferID != hex.EncodeToString(ids[0][:]) {
		t.Fatalf("snapshot quoted below the oldest seller's locked payout: %+v", state.Pool)
	}
}

func TestPoolV6SnapshotRetainsOnlyLiveStateBeyondHistoricalLimit(t *testing.T) {
	node, _ := newPoolV6TestNode()
	oldAuthor := [20]byte{6}
	oldID := poolV6OfferID(oldAuthor, 230)
	node.items[poolV6OfferPrefix] = append(node.items[poolV6OfferPrefix],
		directMarketItem(oldID[:], poolV6TestOffer(6, 5, [32]byte{}, [32]byte{}, 10, 0, 5_102)))
	node.items[poolV6InventoryPrefix] = append(node.items[poolV6InventoryPrefix],
		directMarketItem(oldID[:], poolV6TestInventory(6)))
	for id := uint64(2); id <= 5_101; id++ {
		order := poolV6TestOrder(oldID)
		order[181] = 2 // settled and no longer holding collateral
		binary.BigEndian.PutUint64(order[173:181], id)
		node.items[poolV6OrderPrefix] = append(node.items[poolV6OrderPrefix],
			directMarketItem([]byte(strconv.FormatUint(id, 10)), order))
	}
	node.fields["pool/next-order"] = poolV6TestCounter(5_102)
	state, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash")
	if err != nil {
		t.Fatal(err)
	}
	if state.Scope != "live" || len(state.Offers) != 4 || len(state.Orders) != 1 ||
		state.Orders[0].State != "funded" || state.Pool.NextOrderID != "5102" {
		t.Fatalf("historical records obscured the live market: offers=%d orders=%d next=%s", len(state.Offers), len(state.Orders), state.Pool.NextOrderID)
	}
}

func TestPoolV6SnapshotRetainsUnresolvedPrivateSaleCollateral(t *testing.T) {
	node, _ := newPoolV6TestNode()
	author := [20]byte{5}
	id := poolV6OfferID(author, 230)
	offer := poolV6TestOffer(5, 4, [32]byte{}, [32]byte{}, 10, 2, 2)
	copy(offer[221:237], poolV6TestAmount(10))
	node.items[poolV6OfferPrefix] = append(node.items[poolV6OfferPrefix], directMarketItem(id[:], offer))
	node.items[poolV6InventoryPrefix] = append(node.items[poolV6InventoryPrefix], directMarketItem(id[:], poolV6TestInventory(5)))
	order := poolV6TestOrder(id)
	order[181] = 2 // settled, with seller fraud collateral still locked
	node.items[poolV6OrderPrefix] = append(node.items[poolV6OrderPrefix], directMarketItem([]byte("2"), order))
	node.fields["pool/next-order"] = poolV6TestCounter(3)
	state, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Offers) != 5 || len(state.Orders) != 2 || state.Orders[1].State != "settled" ||
		state.Offers[4].FraudCollateral != "10" {
		t.Fatalf("unresolved collateral disappeared from live market: offers=%d orders=%d", len(state.Offers), len(state.Orders))
	}
}

func TestPoolV6DecodeRejectsMalformedRecordsAndUsesV6OfferDomain(t *testing.T) {
	author := [20]byte{1}
	id := poolV6OfferID(author, 230)
	preimage := append([]byte("fod:pool-offer:v6"), author[:]...)
	preimage = append(preimage, 0, 230)
	if id != sha256.Sum256(preimage) || id == DirectOfferID(author, 230) {
		t.Fatal("v6 offer ID does not use its own domain")
	}
	offer := poolV6TestOffer(1, 1, [32]byte{}, [32]byte{}, 30, 0, 1)
	if _, err := decodePoolV6Offer(offer); err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{138, 139, 220, 237} {
		broken := bytes.Clone(offer)
		broken[index] = 7
		if _, err := decodePoolV6Offer(broken); err == nil {
			t.Fatalf("accepted invalid offer field at %d", index)
		}
	}
	if _, err := decodePoolV6Offer(offer[:237]); err == nil {
		t.Fatal("accepted short offer")
	}
	order := poolV6TestOrder(id)
	if _, err := decodePoolV6Order(order); err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{181, 214} {
		broken := bytes.Clone(order)
		broken[index] = 7
		if _, err := decodePoolV6Order(broken); err == nil {
			t.Fatalf("accepted invalid order field at %d", index)
		}
	}
	if _, err := decodePoolV6Order(order[:214]); err == nil {
		t.Fatal("accepted short order")
	}
}

func TestPoolV6SnapshotOmitsBuyQuoteWithOneQueuedUnit(t *testing.T) {
	node, ids := newPoolV6TestNode()
	node.items[poolV6OfferPrefix] = []*mapItem{
		directMarketItem(ids[0][:], poolV6TestOffer(1, 1, [32]byte{}, [32]byte{}, 10, 0, 1)),
	}
	node.items[poolV6OrderPrefix] = nil
	node.items[poolV6InventoryPrefix] = node.items[poolV6InventoryPrefix][:1]
	node.fields["pool/primary-units"] = poolV6TestCounter(1)
	node.fields["pool/secondary-units"] = poolV6TestCounter(0)
	node.fields["pool/units"] = poolV6TestCounter(1)
	node.fields["pool/reserved-units"] = poolV6TestCounter(0)
	node.fields["pool/committed"] = poolV6TestAmount(10)
	node.fields["pool/buyer-payments"] = poolV6TestAmount(0)
	node.fields["pool/pending"] = poolV6TestAmount(20)
	node.fields["pool/primary-tail"] = ids[0][:]
	node.fields["pool/secondary-head"] = make([]byte, 32)
	node.fields["pool/secondary-tail"] = make([]byte, 32)
	node.fields["pool/next-order"] = poolV6TestCounter(1)
	state, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash")
	if err != nil {
		t.Fatal(err)
	}
	if state.Pool.BuyQuote != "" || state.Pool.NextOfferID != hex.EncodeToString(ids[0][:]) {
		t.Fatalf("one queued unit must have no buy quote: %+v", state.Pool)
	}
}

func TestPoolV6SnapshotShowsTopUpAndRefundCutoff(t *testing.T) {
	node, _ := newPoolV6TestNode()
	oldest := node.items[poolV6OfferPrefix][0]
	raw, err := decodeHexField(oldest.Value)
	if err != nil {
		t.Fatal(err)
	}
	copy(raw[106:122], poolV6TestAmount(5_000_042))
	oldest.Value = "0x" + hex.EncodeToString(raw)
	node.heads[0].Height = 13_060
	node.heads[1].Height = 13_060
	state, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash")
	if err != nil {
		t.Fatal(err)
	}
	if state.Pool.BuyPausedForBond || state.Pool.BondTopUpRequired != "" ||
		len(state.Orders) != 1 || !state.Orders[0].RefundAvailable {
		t.Fatalf("top-up or refund cutoff not reflected: pool=%+v orders=%+v", state.Pool, state.Orders)
	}
}

func TestPoolV6SnapshotRejectsChangedHeadAndInconsistentPublicState(t *testing.T) {
	cases := []struct {
		name   string
		change func(*poolV6TestNode, [4][32]byte)
	}{
		{"changed head", func(n *poolV6TestNode, _ [4][32]byte) { n.heads[1].Hash = "0xdef" }},
		{"broken queue", func(n *poolV6TestNode, _ [4][32]byte) { n.fields["pool/primary-units"] = poolV6TestCounter(3) }},
		{"funded order payout mismatch", func(n *poolV6TestNode, _ [4][32]byte) {
			raw, _ := decodeHexField(n.items[poolV6OrderPrefix][0].Value)
			copy(raw[149:165], poolV6TestAmount(29))
			n.items[poolV6OrderPrefix][0].Value = "0x" + hex.EncodeToString(raw)
		}},
		{"underfunded virtual reserve", func(n *poolV6TestNode, _ [4][32]byte) { n.fields["pool/insurance"] = poolV6TestAmount(111) }},
		{"duplicate pinned seed", func(n *poolV6TestNode, _ [4][32]byte) { n.fields["pool/seed-second"] = n.fields["pool/seed-first"] }},
		{"missing counter", func(n *poolV6TestNode, _ [4][32]byte) { delete(n.fields, "pool/reserve") }},
		{"bad order ID", func(n *poolV6TestNode, _ [4][32]byte) {
			n.items[poolV6OrderPrefix][0].Key = "0x" + hex.EncodeToString([]byte("01"))
		}},
		{"duplicate offer", func(n *poolV6TestNode, _ [4][32]byte) {
			n.items[poolV6OfferPrefix] = append(n.items[poolV6OfferPrefix], n.items[poolV6OfferPrefix][0])
		}},
		{"missing public inventory", func(n *poolV6TestNode, _ [4][32]byte) {
			n.items[poolV6InventoryPrefix] = n.items[poolV6InventoryPrefix][:3]
		}},
		{"forged public CID", func(n *poolV6TestNode, _ [4][32]byte) {
			raw, _ := decodeHexField(n.items[poolV6InventoryPrefix][0].Value)
			raw[2] = 'z'
			n.items[poolV6InventoryPrefix][0].Value = "0x" + hex.EncodeToString(raw)
		}},
		{"forged historical CID", func(n *poolV6TestNode, _ [4][32]byte) {
			id := poolV6OfferID([20]byte{6}, 230)
			n.items[poolV6OfferPrefix] = append(n.items[poolV6OfferPrefix],
				directMarketItem(id[:], poolV6TestOffer(6, 5, [32]byte{}, [32]byte{}, 10, 0, 1)))
			raw := poolV6TestInventory(6)
			raw[2] = 'z'
			n.items[poolV6InventoryPrefix] = append(n.items[poolV6InventoryPrefix], directMarketItem(id[:], raw))
		}},
		{"invalid historical order nonce", func(n *poolV6TestNode, ids [4][32]byte) {
			order := poolV6TestOrder(ids[0])
			order[181] = 2
			n.items[poolV6OrderPrefix] = append(n.items[poolV6OrderPrefix], directMarketItem([]byte("2"), order))
			n.fields["pool/next-order"] = poolV6TestCounter(3)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, ids := newPoolV6TestNode()
			tc.change(node, ids)
			if _, err := buildDirectV6PoolStateWithCall(node.call, testContract, "sample-code-hash"); err == nil {
				t.Fatal("accepted inconsistent v6 snapshot")
			}
		})
	}
	count := poolV6MaxHistory
	tooMany := poolV6Call(func(method string, params any, result any) error {
		*result.(*mapPage) = mapPage{Items: []*mapItem{directMarketItem([]byte(strconv.Itoa(1)), []byte{1})}}
		return nil
	})
	if _, err := walkPoolV6Prefix(tooMany, "0x"+testContract, poolV6OfferPrefix, &count, func(string, []byte) error { return nil }); err == nil {
		t.Fatal("accepted historical scan beyond limit")
	}
}
