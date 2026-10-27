package main

// Bonded market adapter. All monetary values are decimal base units. Public
// state comes from the pinned loopback node, never from a hosted order book.
import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"io"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const bondedV8Format = "fod.prepared-bonded-v8-"

var bondedV8Methods = []string{"depositStake", "withdrawStake", "addReserve", "registerBundle", "queueSell", "queueBuy", "cancelSell", "cancelBuy", "acquire", "matchNext", "ready", "deliver", "accept", "challenge", "proveDelivery", "expireTrade"}

func init() {
	for _, op := range bondedV8Methods {
		preparedFormats[bondedV8Format+op] = op
		if op != "registerBundle" {
			preparedFormats[bondedFormat(9)+op] = op
		}
	}
}
func isBondedV8Format(s string) bool {
	return strings.HasPrefix(s, bondedV8Format) || strings.HasPrefix(s, bondedFormat(9))
}
func bondedFormat(version int) string { return "fod.prepared-bonded-v" + strconv.Itoa(version) + "-" }

func buyerBondedOperation(op string) bool {
	switch op {
	case "depositStake", "withdrawStake", "queueBuy", "cancelBuy", "matchNext", "ready", "accept", "challenge", "expireTrade":
		return true
	}
	return false
}

func sellerClientBondedOperation(op string) bool {
	if buyerBondedOperation(op) {
		return true
	}
	switch op {
	case "queueSell", "cancelSell", "acquire", "deliver", "proveDelivery":
		return true
	}
	return false
}

func verifySellerClientBondedAction(c *client, contract string, req *bondedV8Request) error {
	if !c.sellerClient {
		return nil
	}
	if req.version() != 9 || !sellerClientBondedOperation(req.Operation) || req.Caller != c.author {
		return errors.New("seller-client action differs from the local identity key or supported v9 operations")
	}
	switch req.Operation {
	case "cancelSell":
		raw, err := v8record(c, contract, bondedPrefix(c)+"sell/", req.ID, 117)
		if err != nil {
			return err
		}
		if hex.EncodeToString(raw[32:52]) != c.author {
			return errors.New("sell order belongs to another identity")
		}
	case "cancelBuy":
		raw, err := v8record(c, contract, bondedPrefix(c)+"buy/", req.ID, 110)
		if err != nil {
			return err
		}
		if hex.EncodeToString(raw[:20]) != c.author {
			return errors.New("buy order belongs to another identity")
		}
	case "deliver", "proveDelivery", "challenge", "expireTrade":
		trade, err := readBondedV8Trade(c, contract, req.ID)
		if err != nil {
			return err
		}
		if req.Operation == "deliver" || req.Operation == "proveDelivery" {
			if trade.Seller != c.author {
				return errors.New("trade belongs to another seller")
			}
		} else if req.Operation == "challenge" && trade.Buyer != c.author ||
			req.Operation == "expireTrade" && trade.Seller != c.author && trade.Buyer != c.author {
			return errors.New("trade belongs to another identity")
		}
	}
	return nil
}

func verifyBuyerBondedAction(c *client, contract string, req *bondedV8Request) error {
	if !c.buyerOnly {
		return nil
	}
	if !buyerBondedOperation(req.Operation) || req.version() != 9 || req.Caller != c.wallet {
		return errors.New("buyer-only action differs from the local wallet")
	}
	switch req.Operation {
	case "cancelBuy":
		raw, err := v8record(c, contract, bondedPrefix(c)+"buy/", req.ID, 110)
		if err != nil {
			return err
		}
		if hex.EncodeToString(raw[:20]) != c.wallet {
			return errors.New("buy order belongs to another wallet")
		}
	case "challenge", "expireTrade":
		trade, err := readBondedV8Trade(c, contract, req.ID)
		if err != nil {
			return err
		}
		if trade.Buyer != c.wallet {
			return errors.New("trade belongs to another buyer")
		}
	}
	return nil
}
func bondedVersion(c *client) int {
	if c != nil && c.bondedVersion == 9 {
		return 9
	}
	return 8
}
func bondedPrefix(c *client) string { return "v" + strconv.Itoa(bondedVersion(c)) + "/" }
func (r *bondedV8Request) version() int {
	if r.ProtocolVersion == 0 {
		return 8
	}
	return r.ProtocolVersion
}

type bondedV8Request struct {
	ProtocolVersion int    `json:"protocolVersion,omitempty"`
	Operation       string `json:"operation"`
	Caller          string `json:"caller"`
	Amount          string `json:"amount,omitempty"`
	Target          string `json:"target,omitempty"`
	Minimum         string `json:"minimum,omitempty"`
	Capacity        string `json:"capacity,omitempty"`
	Maximum         string `json:"maximum,omitempty"`
	Bundle          string `json:"bundle,omitempty"`
	ID              string `json:"id,omitempty"`
	Expires         uint64 `json:"expires,omitempty"`
	Exit            bool   `json:"exit,omitempty"`
	DeliveryKey     string `json:"deliveryKey,omitempty"`
	Ciphertext      string `json:"ciphertext,omitempty"`
	CiphertextHash  string `json:"ciphertextHash,omitempty"`
	Journal         string `json:"journal,omitempty"`
	Seal            string `json:"seal,omitempty"`
	ContentJournal  string `json:"contentJournal,omitempty"`
	ContentSeal     string `json:"contentSeal,omitempty"`
	SessionWire     string `json:"sessionWire,omitempty"`
	// These files remain in the owner-only client directory. The CLI rechecks
	// bytes and private-key ownership before signing ready/accept.
	SessionFile     string `json:"sessionFile,omitempty"`
	VaultFile       string `json:"vaultFile,omitempty"`
	RetainedFile    string `json:"retainedFile,omitempty"`
	DeliveryKeyFile string `json:"deliveryKeyFile,omitempty"`
}
type bondedV8Account struct {
	Balance    string `json:"balance"`
	Reserved   string `json:"reserved"`
	Target     string `json:"target"`
	NeedsTopup bool   `json:"needsTopup"`
}
type bondedV8Bundle struct {
	ID        string   `json:"id"`
	Author    string   `json:"author"`
	Epoch     uint16   `json:"epoch"`
	Count     uint16   `json:"count"`
	Inventory string   `json:"inventory"`
	Keys      string   `json:"keys"`
	Sale      string   `json:"sale"`
	Used      bool     `json:"used"`
	CIDs      []string `json:"cids"`
}
type bondedV8Sell struct {
	ID       string `json:"id"`
	Bundle   string `json:"bundle"`
	Seller   string `json:"seller"`
	Minimum  string `json:"minimum"`
	Capacity string `json:"capacity"`
	Expires  uint64 `json:"expires"`
	QueuedAt uint64 `json:"queuedAt"`
	Proceeds string `json:"proceeds"`
	State    byte   `json:"state"`
}
type bondedV8Buy struct {
	ID          string `json:"id"`
	Buyer       string `json:"buyer"`
	DeliveryKey string `json:"deliveryKey"`
	Maximum     string `json:"maximum"`
	Expires     uint64 `json:"expires"`
	State       byte   `json:"state"`
}
type bondedPricePoint struct {
	Trade  string `json:"trade"`
	Height uint64 `json:"height"`
	Price  string `json:"price"`
}
type bondedV8Trade struct {
	ID                string `json:"id"`
	Sell              string `json:"sell"`
	Buy               string `json:"buy"`
	Bundle            string `json:"bundle"`
	Seller            string `json:"seller"`
	Buyer             string `json:"buyer"`
	DeliveryKey       string `json:"deliveryKey"`
	Price             string `json:"price"`
	Proceeds          string `json:"proceeds"`
	PoolFunded        bool   `json:"poolFunded"`
	ReadyBy           uint64 `json:"readyBy"`
	FundedAt          uint64 `json:"fundedAt"`
	FundingRoot       string `json:"fundingRoot"`
	DeliverBy         uint64 `json:"deliverBy"`
	AcceptBy          uint64 `json:"acceptBy"`
	ProveBy           uint64 `json:"proveBy"`
	CiphertextHash    string `json:"ciphertextHash"`
	ProofCost         string `json:"proofCost"`
	State             byte   `json:"state"`
	ExplicitChallenge bool   `json:"explicitChallenge"`
}
type bondedV8Snapshot struct {
	Version         int                `json:"version"`
	Contract        string             `json:"contract"`
	CodeHash        string             `json:"codeHash"`
	Identity        string             `json:"identity"`
	Height          uint64             `json:"height"`
	ObservedAt      string             `json:"observedAt"`
	Reserve         string             `json:"reserve"`
	Escrow          string             `json:"escrow"`
	Stakes          string             `json:"stakes"`
	PendingEarnings string             `json:"pendingEarnings"`
	Inventory       string             `json:"inventory"`
	BuyQuote        string             `json:"buyQuote"`
	SellQuote       string             `json:"sellQuote"`
	PoolCanAcquire  bool               `json:"poolCanAcquire"`
	Account         bondedV8Account    `json:"account"`
	Sells           []bondedV8Sell     `json:"sells"`
	Buys            []bondedV8Buy      `json:"buys"`
	Trades          []bondedV8Trade    `json:"trades"`
	Bundles         []bondedV8Bundle   `json:"bundles"`
	PriceHistory    []bondedPricePoint `json:"priceHistory"`
}

func v8amount(b []byte) string { return new(big.Int).SetBytes(b).String() }
func v8id(b []byte) string     { return strconv.FormatUint(binary.BigEndian.Uint64(b), 10) }
func v8hex(s string, size int) ([]byte, error) {
	if !isHex(s, size) || s != strings.ToLower(s) {
		return nil, errors.New("invalid v8 hex field")
	}
	return hex.DecodeString(s)
}
func v8number(s string, n int) ([]byte, error) {
	if !isDecimal(s) {
		return nil, errors.New("invalid v8 amount or ID")
	}
	a, ok := new(big.Int).SetString(s, 10)
	if !ok || a.Sign() < 0 || a.BitLen() > n*8 {
		return nil, errors.New("v8 amount or ID overflow")
	}
	return a.FillBytes(make([]byte, n)), nil
}
func v8map(c *client, contract, prefix string, suffix []byte) ([]byte, error) {
	var out string
	if err := c.call("contract_readMap", []any{"0x" + contract, prefix, "0x" + hex.EncodeToString(suffix), "hex"}, &out); err != nil {
		var answered answeredError
		if errors.As(err, &answered) && answered.Error() == "rpc contract_readMap: data is nil (-32000)" {
			return nil, nil
		}
		return nil, err
	}
	return hex.DecodeString(strings.TrimPrefix(out, "0x"))
}
func v8record(c *client, contract, prefix, id string, size int) ([]byte, error) {
	suffix, err := v8number(id, 8)
	if err != nil {
		return nil, err
	}
	raw, err := v8map(c, contract, prefix, suffix)
	if err != nil {
		return nil, err
	}
	if len(raw) != size {
		return nil, errors.New("missing or malformed v8 record")
	}
	return raw, nil
}
func readBondedV8Trade(c *client, contract, id string) (*bondedV8Trade, error) {
	r, e := v8record(c, contract, bondedPrefix(c)+"trade/", id, 308)
	if e != nil {
		return nil, e
	}
	if r[185] > 1 || r[307] > 1 || r[306] < 1 || r[306] > 6 {
		return nil, errors.New("invalid v8 trade flags")
	}
	return &bondedV8Trade{ID: id, Sell: v8id(r[:8]), Buy: v8id(r[8:16]), Bundle: hex.EncodeToString(r[16:48]), Seller: hex.EncodeToString(r[48:68]), Buyer: hex.EncodeToString(r[68:88]), DeliveryKey: hex.EncodeToString(r[88:153]), Price: v8amount(r[153:169]), Proceeds: v8amount(r[169:185]), PoolFunded: r[185] == 1, ReadyBy: binary.BigEndian.Uint64(r[186:194]), FundedAt: binary.BigEndian.Uint64(r[194:202]), FundingRoot: hex.EncodeToString(r[202:234]), DeliverBy: binary.BigEndian.Uint64(r[234:242]), AcceptBy: binary.BigEndian.Uint64(r[242:250]), ProveBy: binary.BigEndian.Uint64(r[250:258]), CiphertextHash: hex.EncodeToString(r[258:290]), ProofCost: v8amount(r[290:306]), State: r[306], ExplicitChallenge: r[307] == 1}, nil
}
func readBondedV8Bundle(c *client, contract, id string) (*bondedV8Bundle, error) {
	suffix, e := v8hex(id, 32)
	if e != nil {
		return nil, e
	}
	r, e := v8map(c, contract, bondedPrefix(c)+"bundle/", suffix)
	if e != nil {
		return nil, e
	}
	if len(r) != 97 || r[96] > 1 {
		return nil, errors.New("invalid v8 bundle")
	}
	b := &bondedV8Bundle{ID: id, Author: hex.EncodeToString(r[:20]), Epoch: binary.BigEndian.Uint16(r[20:22]), Count: binary.BigEndian.Uint16(r[22:24]), Inventory: hex.EncodeToString(r[24:56]), Keys: hex.EncodeToString(r[56:88]), Sale: v8id(r[88:96]), Used: r[96] == 1}
	d := sha256.Sum256(append(append([]byte("fod:bundle:v"+strconv.Itoa(bondedVersion(c))), r[:20]...), r[20:22]...))
	if hex.EncodeToString(d[:]) != id || b.Count == 0 || b.Count > 32 {
		return nil, errors.New("v8 bundle identity mismatch")
	}
	raw, e := v8map(c, contract, bondedPrefix(c)+"cids/", suffix)
	if e != nil {
		return nil, e
	}
	if len(raw) != 1+int(b.Count)*60 || raw[0] != byte(b.Count) {
		return nil, errors.New("invalid v8 CID inventory")
	}
	for i := 0; i < int(b.Count); i++ {
		off := 1 + i*60
		if raw[off] != 59 {
			return nil, errors.New("invalid v8 CID length")
		}
		cid := string(raw[off+1 : off+60])
		if i > 0 && cid <= b.CIDs[i-1] {
			return nil, errors.New("v8 CIDs are not canonical")
		}
		b.CIDs = append(b.CIDs, cid)
	}
	if inventoryCommitment(b.Author, b.Epoch, b.CIDs) != b.Inventory {
		return nil, errors.New("v8 inventory commitment mismatch")
	}
	return b, nil
}
func bondedV8Quotes(reserve, inventory *big.Int) (buy, sell *big.Int, error error) {
	return bondedQuotes(8, reserve, inventory)
}
func bondedQuotes(version int, reserve, inventory *big.Int) (buy, sell *big.Int, error error) {
	virtualReserve := int64(100)
	if version == 9 {
		virtualReserve = 4500
	} else if version != 8 {
		return nil, nil, errors.New("unsupported bonded protocol")
	}
	if reserve.Sign() < 0 || inventory.Sign() < 0 || reserve.BitLen() > 128 || inventory.BitLen() > 128 {
		return nil, nil, errors.New("invalid curve state")
	}
	r := new(big.Int).Add(reserve, new(big.Int).Mul(big.NewInt(virtualReserve), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)))
	x := new(big.Int).Add(inventory, big.NewInt(1000))
	numerator := new(big.Int).Mul(r, big.NewInt(100))
	if r.BitLen() > 128 || x.BitLen() > 128 || numerator.BitLen() > 128 {
		return nil, nil, errors.New("curve arithmetic overflow")
	}
	sell = new(big.Int).Div(new(big.Int).Set(numerator), new(big.Int).Add(x, big.NewInt(100)))
	denominator := new(big.Int).Sub(x, big.NewInt(100))
	rounded := new(big.Int).Add(numerator, new(big.Int).Sub(denominator, big.NewInt(1)))
	if rounded.BitLen() > 128 {
		return nil, nil, errors.New("curve arithmetic overflow")
	}
	buy = new(big.Int).Div(rounded, denominator)
	return
}
func readBondedV8Snapshot(c *client, contract, pin string) (*bondedV8Snapshot, error) {
	if !c.bondedRPCAllowed() {
		return nil, errors.New("bonded market requires an approved RPC connection")
	}
	if _, e := v8hex(contract, 20); e != nil {
		return nil, e
	}
	if _, e := v8hex(pin, 32); e != nil {
		return nil, e
	}
	before, e := readPoolV7SyncedHead(c)
	if e != nil {
		return nil, e
	}
	if e = checkBuyTarget(c, contract, contract, pin); e != nil {
		return nil, e
	}
	identity, e := c.bondedIdentity()
	if e != nil {
		return nil, e
	}
	t, e := readRegistryValue(c, contract, bondedPrefix(c)+"totals")
	if e != nil {
		return nil, e
	}
	if len(t) != 104 {
		return nil, errors.New("this deployment is not a bonded v8 market")
	}
	qbuy, qsell, e := bondedQuotes(bondedVersion(c), new(big.Int).SetBytes(t[:16]), new(big.Int).SetBytes(t[64:80]))
	if e != nil {
		return nil, e
	}
	s := &bondedV8Snapshot{Version: bondedVersion(c), Contract: contract, CodeHash: pin, Identity: identity, Height: before.Height, ObservedAt: checkedAtNow(), Reserve: v8amount(t[:16]), Escrow: v8amount(t[16:32]), Stakes: v8amount(t[32:48]), PendingEarnings: v8amount(t[48:64]), Inventory: v8amount(t[64:80]), BuyQuote: qbuy.String(), SellQuote: qsell.String(), PoolCanAcquire: new(big.Int).SetBytes(t[:16]).Cmp(qsell) >= 0, Account: bondedV8Account{"0", "0", "0", false}, Sells: []bondedV8Sell{}, Buys: []bondedV8Buy{}, Trades: []bondedV8Trade{}, Bundles: []bondedV8Bundle{}}
	who, _ := hex.DecodeString(identity)
	a, e := v8map(c, contract, bondedPrefix(c)+"account/", who)
	if e != nil {
		return nil, e
	}
	if len(a) > 0 {
		if len(a) != 49 || a[48] > 1 {
			return nil, errors.New("invalid v8 stake")
		}
		s.Account = bondedV8Account{v8amount(a[:16]), v8amount(a[16:32]), v8amount(a[32:48]), a[48] == 1}
	}
	bundles := map[string]bool{}
	for _, side := range []string{"sells", "buys"} {
		q, e := readRegistryValue(c, contract, bondedPrefix(c)+""+side)
		if errors.Is(e, errNotInRegistry) {
			q = nil
			e = nil
		}
		if e != nil {
			return nil, e
		}
		if len(q) > 512 || len(q)%8 != 0 {
			return nil, errors.New("invalid v8 queue")
		}
		for off := 0; off < len(q); off += 8 {
			id := v8id(q[off : off+8])
			if side == "sells" {
				r, e := v8record(c, contract, bondedPrefix(c)+"sell/", id, 117)
				if e != nil {
					return nil, e
				}
				b := hex.EncodeToString(r[:32])
				bundles[b] = true
				s.Sells = append(s.Sells, bondedV8Sell{id, b, hex.EncodeToString(r[32:52]), v8amount(r[52:68]), v8amount(r[68:84]), binary.BigEndian.Uint64(r[84:92]), binary.BigEndian.Uint64(r[92:100]), v8amount(r[100:116]), r[116]})
			} else {
				r, e := v8record(c, contract, bondedPrefix(c)+"buy/", id, 110)
				if e != nil {
					return nil, e
				}
				s.Buys = append(s.Buys, bondedV8Buy{id, hex.EncodeToString(r[:20]), hex.EncodeToString(r[20:85]), v8amount(r[85:101]), binary.BigEndian.Uint64(r[101:109]), r[109]})
			}
		}
	}
	// Include every unresolved trade, plus a bounded recent terminal history.
	active, e := readRegistryValue(c, contract, bondedPrefix(c)+"active")
	if errors.Is(e, errNotInRegistry) {
		active = nil
		e = nil
	}
	if e != nil {
		return nil, e
	}
	if len(active) > 512 || len(active)%8 != 0 {
		return nil, errors.New("invalid active trade index")
	}
	ids := map[uint64]bool{}
	for off := 0; off < len(active); off += 8 {
		ids[binary.BigEndian.Uint64(active[off:off+8])] = true
	}
	next := binary.BigEndian.Uint64(t[96:104])
	start := uint64(1)
	if next > 33 {
		start = next - 32
	}
	for id := start; id < next; id++ {
		ids[id] = true
	}
	for id := range ids {
		tr, e := readBondedV8Trade(c, contract, strconv.FormatUint(id, 10))
		if e != nil {
			return nil, e
		}
		if tr.Buyer == identity || tr.Seller == identity {
			s.Trades = append(s.Trades, *tr)
			bundles[tr.Bundle] = true
		}
		if tr.State == 5 {
			s.PriceHistory = append(s.PriceHistory, bondedPricePoint{tr.ID, tr.FundedAt, tr.Price})
		}
	}
	// Discover registered author bundles independently of whether they are listed.
	count := 0
	records, e := readDirectMarketPrefix(c, registryAddress(contract), bondedPrefix(c)+"bundle/", &count)
	if e != nil {
		return nil, e
	}
	for id, r := range records {
		if len(r) != 97 {
			return nil, errors.New("invalid bundle index")
		}
		if bytes.Equal(r[:20], who) {
			bundles[id] = true
		}
	}
	for id := range bundles {
		b, e := readBondedV8Bundle(c, contract, id)
		if e != nil {
			return nil, e
		}
		s.Bundles = append(s.Bundles, *b)
	}
	sort.Slice(s.Bundles, func(i, j int) bool { return s.Bundles[i].Epoch > s.Bundles[j].Epoch })
	sort.Slice(s.Trades, func(i, j int) bool {
		a, _ := strconv.ParseUint(s.Trades[i].ID, 10, 64)
		b, _ := strconv.ParseUint(s.Trades[j].ID, 10, 64)
		return a > b
	})
	sort.Slice(s.PriceHistory, func(i, j int) bool {
		a, _ := strconv.ParseUint(s.PriceHistory[i].Trade, 10, 64)
		b, _ := strconv.ParseUint(s.PriceHistory[j].Trade, 10, 64)
		return a < b
	})
	after, e := readPoolV7SyncedHead(c)
	if e != nil {
		return nil, e
	}
	if before.Height != after.Height || before.Hash != after.Hash {
		return nil, errors.New("market changed while reading; refresh")
	}
	return s, nil
}

func encodeBondedV8(r *bondedV8Request) ([]byte, string, error) {
	if r == nil {
		return nil, "", errors.New("missing v8 terms")
	}
	if r.version() != 8 && r.version() != 9 {
		return nil, "", errors.New("unsupported bonded protocol")
	}
	if _, e := v8hex(r.Caller, 20); e != nil {
		return nil, "", e
	}
	var out []byte
	amount := "1"
	var err error
	num := func(s string, n int) {
		if err != nil {
			return
		}
		var b []byte
		b, err = v8number(s, n)
		out = append(out, b...)
	}
	hx := func(s string, n int) {
		if err != nil {
			return
		}
		var b []byte
		b, err = v8hex(s, n)
		out = append(out, b...)
	}
	id := func() {
		num(r.ID, 8)
		if r.ID == "0" {
			err = errors.New("trade/order ID must be positive")
		}
	}
	switch r.Operation {
	case "depositStake":
		num(r.Target, 16)
		amount = r.Amount
	case "withdrawStake":
		num(r.Amount, 16)
		if r.Exit {
			out = append(out, 1)
		} else {
			out = append(out, 0)
		}
	case "addReserve":
		out = []byte{byte(r.version())}
		amount = r.Amount
	case "registerBundle":
		if r.version() == 9 {
			return nil, "", errors.New("v9 lists packages directly; upfront proof registration is not supported")
		}
		hx(r.Journal, 273)
		hx(r.Seal, 256)
		wire, e := hex.DecodeString(r.SessionWire)
		if e != nil || len(wire) == 0 || len(wire) > 65535 {
			err = errors.New("invalid canonical session wire")
		} else {
			out = binary.BigEndian.AppendUint16(out, uint16(len(wire)))
			out = append(out, wire...)
		}
	case "queueSell":
		if r.version() == 8 {
			hx(r.Bundle, 32)
		}
		num(r.Minimum, 16)
		num(r.Capacity, 16)
		out = binary.BigEndian.AppendUint64(out, r.Expires)
		if r.version() == 9 {
			hx(r.Journal, 273)
			wire, e := hex.DecodeString(r.SessionWire)
			if e != nil || len(wire) == 0 || len(wire) > 65535 || r.Seal != "" {
				return nil, "", errors.New("listing needs canonical session evidence and no content seal")
			}
			out = binary.BigEndian.AppendUint16(out, uint16(len(wire)))
			out = append(out, wire...)
		}
	case "queueBuy":
		hx(r.DeliveryKey, 65)
		num(r.Maximum, 16)
		out = binary.BigEndian.AppendUint64(out, r.Expires)
		amount = r.Maximum
	case "cancelSell", "cancelBuy", "ready", "challenge", "expireTrade":
		id()
	case "acquire", "matchNext":
		out = []byte{byte(r.version())}
	case "deliver":
		id()
		hx(r.Ciphertext, 219)
	case "accept":
		id()
		hx(r.CiphertextHash, 32)
	case "proveDelivery":
		id()
		if r.version() == 9 {
			hx(r.ContentJournal, 273)
			hx(r.ContentSeal, 256)
		}
		hx(r.Journal, 281)
		hx(r.Seal, 256)
	default:
		err = errors.New("unsupported bonded operation")
	}
	if err != nil {
		return nil, "", err
	}
	a, e := v8number(amount, 16)
	if e != nil || new(big.Int).SetBytes(a).Sign() == 0 {
		return nil, "", errors.New("positive attached value required")
	}
	return out, amount, nil
}
func validatePreparedBondedV8(c *client, p *preparedAction, identity *ecdsa.PrivateKey) error {
	p.bondedV8Validated = false
	if !isBondedV8Format(p.Format) || p.Network != NetworkNamePinned || p.BondedV8 == nil || p.BondedV8.Operation != p.Operation || p.Format != bondedFormat(p.BondedV8.version())+p.Operation {
		return errors.New("invalid bonded v8 action")
	}
	copyClient := *c
	copyClient.bondedVersion = p.BondedV8.version()
	c = &copyClient
	sender, e := nodekeyAddressOf(identity)
	if e != nil {
		return e
	}
	if sender != p.BondedV8.Caller {
		return errors.New("v8 signing identity mismatch")
	}
	if c.buyerOnly && (!buyerBondedOperation(p.Operation) || p.BondedV8.version() != 9) {
		return errors.New("buyer-only signing refuses this operation")
	}
	data, amount, e := encodeBondedV8(p.BondedV8)
	if e != nil {
		return e
	}
	if p.Amount != amount || p.Calldata != hex.EncodeToString(data) {
		return errors.New("v8 terms differ from signed call")
	}
	if !c.bondedRPCAllowed() {
		return errors.New("bonded signing requires an approved RPC connection")
	}
	if e = checkBuyTarget(c, p.Contract, p.Contract, p.CodeHash); e != nil {
		return e
	}
	if e = verifySellerClientBondedAction(c, p.Contract, p.BondedV8); e != nil {
		return e
	}
	if _, e = readPoolV7SyncedHead(c); e != nil {
		return e
	}
	if e = verifyBondedV8LocalAction(c, p.Contract, p.BondedV8); e != nil {
		return e
	}
	if e = verifyBuyerBondedAction(c, p.Contract, p.BondedV8); e != nil {
		return e
	}
	p.bondedV8Validated = true
	return nil
}
func runBondedV8(args []string) error {
	return runBonded(args, 8)
}
func runBonded(args []string, version int) error {
	if version != 8 && version != 9 {
		return errors.New("unsupported bonded protocol")
	}
	f := flag.NewFlagSet("v"+strconv.Itoa(version), flag.ContinueOnError)
	command := f.String("command", "status", "status, prepare, listing, contentWitness, retain, receive, delivery, preview")
	rpc := f.String("rpc", "", "keyed loopback node RPC")
	keyFile := f.String("key-file", "", "private RPC key file")
	buyerOnly := f.Bool("buyer-only", false, "v9 buyer-only relay connection")
	sellerClient := f.Bool("seller-client", false, "v9 seller identity through an HTTPS relay")
	walletKey := f.String("wallet-key", "", "owner-only buyer wallet key file")
	nodeKey := f.String("nodekey", "", "owner-only seller identity key file")
	contract := f.String("contract", "", "independent deployment pin")
	pin := f.String("code-hash", "", "independent code pin")
	input := f.String("request", "", "owner-only request JSON")
	out := f.String("out", "", "owner-only output JSON")
	fee := f.String("max-fee", "", "fee ceiling in base units")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 || *out == "" {
		return errors.New("v8 requires output file and named options")
	}
	var e error
	c := &client{url: *rpc, http: newHTTPClient(), bondedVersion: version, buyerOnly: *buyerOnly, sellerClient: *sellerClient}
	if *buyerOnly && *sellerClient {
		return errors.New("choose one remote connection mode")
	}
	if *sellerClient {
		if version != 9 || *keyFile != "" || *walletKey != "" {
			return errors.New("seller-client requires v9, a local identity key and no node RPC key")
		}
		author, err := sellerIdentityAddress(*nodeKey)
		if err != nil {
			return err
		}
		c.author, c.authorKeyPath = author, *nodeKey
	} else if *buyerOnly {
		if version != 9 || *keyFile != "" || *nodeKey != "" {
			return errors.New("buyer-only requires v9 and no node RPC key")
		}
		wallet, err := buyerWalletAddress(*walletKey)
		if err != nil {
			return err
		}
		c.wallet = wallet
	} else {
		if *walletKey != "" || *nodeKey != "" {
			return errors.New("wallet-key is only for v9 buyer-only mode")
		}
		api, err := resolveRPCKey(*keyFile, true)
		if err != nil {
			return err
		}
		c.key = api
	}
	if !c.bondedRPCAllowed() {
		return errors.New("bonded command requires an approved RPC connection")
	}
	if *command == "status" {
		s, e := readBondedV8Snapshot(c, *contract, *pin)
		if e != nil {
			return e
		}
		return writePrivateJSON(*out, s)
	}
	if e = checkBuyTarget(c, *contract, *contract, *pin); e != nil {
		return e
	}
	raw, e := readBondedV8Private(*input, 8*1024*1024)
	if e != nil {
		return e
	}
	var req bondedV8Request
	if e = decodeStrict(raw, &req); e != nil {
		return e
	}
	if req.ProtocolVersion == 0 && version == 9 {
		req.ProtocolVersion = 9
	}
	if req.version() != version {
		return errors.New("request protocol differs from selected market")
	}
	if c.buyerOnly && (*command == "prepare" && !buyerBondedOperation(req.Operation) ||
		*command != "prepare" && !map[string]bool{"retain": true, "receive": true, "preview": true}[*command]) {
		return errors.New("buyer-only command refuses seller or unsupported work")
	}
	if c.sellerClient && (*command == "prepare" && !sellerClientBondedOperation(req.Operation) ||
		*command != "prepare" && !map[string]bool{"listing": true, "contentWitness": true, "retain": true, "receive": true, "delivery": true, "preview": true}[*command]) {
		return errors.New("seller-client command refuses unsupported work")
	}
	if c.buyerOnly && req.Caller != c.wallet {
		return errors.New("buyer request differs from wallet key")
	}
	if c.sellerClient && req.Caller != c.author {
		return errors.New("seller request differs from identity key")
	}
	if c.sellerClient {
		if _, err := c.bondedIdentity(); err != nil {
			return err
		}
	}
	if *command != "prepare" {
		return runBondedV8Content(c, *contract, *command, &req, *out)
	}
	data, amount, e := encodeBondedV8(&req)
	if e != nil {
		return e
	}
	if _, e = readPoolV7SyncedHead(c); e != nil {
		return e
	}
	who, e := c.bondedIdentity()
	if e != nil || who != req.Caller {
		return errors.New("v8 request differs from active identity")
	}
	if e = verifyBondedV8LocalAction(c, *contract, &req); e != nil {
		return e
	}
	if e = verifyBuyerBondedAction(c, *contract, &req); e != nil {
		return e
	}
	if e = verifySellerClientBondedAction(c, *contract, &req); e != nil {
		return e
	}
	p := &preparedAction{Format: bondedFormat(version) + req.Operation, Version: 1, Operation: req.Operation, Network: NetworkNamePinned, Contract: *contract, CodeHash: *pin, Amount: amount, Calldata: hex.EncodeToString(data), PreparedAt: time.Now().UTC().Format(time.RFC3339), BondedV8: &req}
	maxFee, ok := new(big.Int).SetString(*fee, 10)
	if !ok || maxFee.Sign() <= 0 {
		return errors.New("invalid fee cap")
	}
	var wallet struct {
		Balance string `json:"balance"`
	}
	if e = c.call("dna_getBalance", []any{"0x" + req.Caller}, &wallet); e != nil {
		return e
	}
	available, e := v8IDNAAtoms(wallet.Balance)
	if e != nil {
		return e
	}
	attached, _ := new(big.Int).SetString(amount, 10)
	available.Sub(available, attached)
	if available.Sign() <= 0 {
		return errors.New("wallet cannot cover the action and its network fee")
	}
	if maxFee.Cmp(available) > 0 {
		maxFee.Set(available)
	}
	if _, _, e = estimatePreparedDirectCall(c, p, req.Caller, maxFee, nil); e != nil {
		return e
	}
	return writePrivateJSON(*out, p)
}
func v8IDNAAtoms(text string) (*big.Int, error) {
	r, ok := new(big.Rat).SetString(text)
	if !ok || r.Sign() < 0 {
		return nil, errors.New("invalid iDNA amount")
	}
	r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)))
	if !r.IsInt() {
		return nil, errors.New("iDNA amount has sub-atom precision")
	}
	return new(big.Int).Set(r.Num()), nil
}
func readBondedV8Private(p string, max int64) ([]byte, error) {
	st, e := os.Lstat(p)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > max {
		return nil, errors.New("v8 input must be an owner-only regular file within the size limit")
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(st, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0077 != 0 || opened.Size() > max {
		return nil, errors.New("v8 input changed while opening")
	}
	raw, e := io.ReadAll(io.LimitReader(f, max+1))
	if int64(len(raw)) > max {
		return nil, errors.New("v8 input exceeds size limit")
	}
	return raw, e
}

func checkBondedV8Submission(c *client, p *preparedAction, key *ecdsa.PrivateKey, nonce uint32, epoch uint16, maxFee, tips, rate *big.Int) (*big.Int, error) {
	if e := validatePreparedBondedV8(c, p, key); e != nil {
		return nil, e
	}
	sender, e := nodekeyAddressOf(key)
	if e != nil {
		return nil, e
	}
	if e = checkAccountAgainstChain(c, sender, nonce, epoch); e != nil {
		return nil, e
	}
	amount, _ := new(big.Int).SetString(p.Amount, 10)
	if e = checkDirectSpendBalance(c, sender, nonce, amount, tips, maxFee); e != nil {
		return nil, e
	}
	_, rate, e = estimatePreparedDirectCall(c, p, sender, maxFee, rate)
	return rate, e
}
