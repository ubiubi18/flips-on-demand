package harness

// Synthetic v6 bootstrap against the pinned Idena WASM runtime. The shared
// fixture injects public seeds; the sealed fallback test qualifies its seller
// through the contract and verifies the full proof-backed recovery path.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/idena-network/idena-wasm-binding/lib"
)

const directV6WasmPath = "../../contracts/direct-escrow-v6/target/fod_direct_escrow_v6.lowered.wasm"
const directV6FastContentImageID = "4fd124c8cb91370dab60d3e1d7c4e666e79485e210b5f20ed36cc2104883726c"

type directV6Fixture struct {
	env       *MinimalEnv
	api       *lib.GoAPI
	code      []byte
	deployGas uint64
	activateGas uint64
	author    lib.Address
	buyer     lib.Address
	offerID   [32]byte
	seedIDs   [2][32]byte
	metadata  []byte
	reserve   *big.Int
	payout    *big.Int
	quote     *big.Int
	bond      *big.Int
	buyerBond *big.Int
}

func v6OfferID(author lib.Address, epoch uint16) [32]byte {
	var encoded [2]byte
	binary.BigEndian.PutUint16(encoded[:], epoch)
	return sha256.Sum256(join([]byte("fod:pool-offer:v6"), author[:], encoded[:]))
}

func newDirectV6Fixture(t *testing.T) *directV6Fixture {
	return newDirectV6FixtureWithActivation(t, true)
}

func newDirectV6FixtureWithActivation(t *testing.T, activate bool) *directV6Fixture {
	t.Helper()
	code, err := os.ReadFile(directV6WasmPath)
	if err != nil {
		t.Fatalf("read lowered v6 candidate: %v", err)
	}
	env := NewMinimalEnv()
	env.SetEpoch(230)
	env.SetContractAddress(addressFromHex(t, "1111111111111111111111111111111111111111"))
	env.SetBlockNumber(1000)
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	reserve := mustAmount("10000000000000000000000") // 10,000 iDNA
	var seedIDs [2][32]byte
	for i, epoch := range []uint16{228, 229} {
		author := addressFromHex(t, hexOf(bytes.Repeat([]byte{byte(0xc1 + i)}, 20)))
		seedIDs[i] = v6OfferID(author, epoch)
	}
	// A zero-value transaction loses its amount field in the node's signed
	// encoding. The WASM host consequently presents an absent pay_amount.
	env.payAmount = nil
	deployGas, _, err := lib.Deploy(api, code, [][]byte{join(seedIDs[0][:], seedIDs[1][:])}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("v6 deploy rejected by pinned runtime: %v", err)
	}
	env.ResetPayAmount()
	f := &directV6Fixture{
		env: env, api: api, code: code,
		deployGas: deployGas,
		author:    addressFromHex(t, hexOf(bytes.Repeat([]byte{0xa1}, 20))),
		buyer:     addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb2}, 20))),
		reserve:   reserve,
		payout:    mustAmount("1000000000000000000000"), // capped at 1,000 iDNA
		quote:     mustAmount("1000000000000000000000"), // capped at 1,000 iDNA
		bond:      big.NewInt(5_000_000),                // proof-cost penalty; no initial price shortfall
		buyerBond: big.NewInt(5_000_000),
		seedIDs:   seedIDs,
	}
	// Recreate the two 238-byte public seed records and their queue. They are
	// deliberately injected after deploy: no session or content proof runs here.
	f.queueID(t, "pool/seed-first", seedIDs[0])
	f.queueID(t, "pool/seed-second", seedIDs[1])
	for i, epoch := range []uint16{228, 229} {
		author := addressFromHex(t, hexOf(bytes.Repeat([]byte{byte(0xc1 + i)}, 20)))
		f.seedPublicOffer(t, i, author, epoch)
	}
	f.put([]byte("pool/secondary-head"), f.seedIDs[0][:])
	f.put([]byte("pool/secondary-tail"), f.seedIDs[1][:])
	f.put([]byte("pool/secondary-units"), u64Arg(2))
	f.put([]byte("pool/units"), u64Arg(2))
	f.put([]byte("pool/seed-count"), u64Arg(2))
	f.put([]byte("pool/next-sequence"), u64Arg(3))
	f.seedQueue(t)
	f.units(t, "pool/units", 2)
	f.units(t, "pool/secondary-units", 2)
	if activate {
		f.env.SetPayAmount(reserve)
		f.env.CreditContractBalance(reserve)
		f.activateGas = f.call(t, "activate", join(seedIDs[0][:], seedIDs[1][:]))
		f.amount(t, "pool/reserve", reserve)
	}

	f.offerID = v6OfferID(f.author, 230)
	f.metadata = join([]byte{0, 230, 0, 0, 0, 1}, bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32))
	qualified := make([]byte, 238)
	copy(qualified[0:20], f.author[:])
	copy(qualified[20:90], f.metadata)
	copy(qualified[122:130], u64Arg(1))
	f.put(join([]byte("pool/offer/"), f.offerID[:]), qualified)
	return f
}

func (f *directV6Fixture) seedPublicOffer(t *testing.T, i int, author lib.Address, epoch uint16) {
	t.Helper()
	var encoded [2]byte
	binary.BigEndian.PutUint16(encoded[:], epoch)
	secrets := join([]byte("fod:epoch-secrets:v1"), encoded[:], author[:], bytes.Repeat([]byte{byte(0x31 + i)}, 64))
	if len(secrets) != 106 {
		t.Fatalf("v6 seed secrets have %d bytes, want 106", len(secrets))
	}
	keys := sha256.Sum256(secrets)
	offer := make([]byte, 238)
	copy(offer[0:20], author[:])
	copy(offer[20:22], encoded[:])
	copy(offer[22:26], []byte{0, 0, 0, 1})
	copy(offer[26:58], bytes.Repeat([]byte{byte(0x41 + i)}, 32))
	copy(offer[58:90], keys[:])
	copy(offer[122:130], u64Arg(1))
	offer[138] = 2 // queued secondary
	if i == 0 {
		copy(offer[172:204], f.seedIDs[1][:])
	} else {
		copy(offer[140:172], f.seedIDs[0][:])
	}
	copy(offer[204:212], u64Arg(uint64(i+1)))
	copy(offer[212:220], u64Arg(1000))
	offer[220] = 1 // content proven
	offer[237] = 1 // public seed
	f.put(join([]byte("pool/offer/"), f.seedIDs[i][:]), offer)
	f.put(join([]byte("pool/public-seed/"), f.seedIDs[i][:]), secrets)
	f.put(join([]byte("pool/seller/"), author[:]), f.seedIDs[i][:])
}

func (f *directV6Fixture) put(key, value []byte) {
	f.env.SetStorage(&lib.GasMeter{}, key, value)
}

func (f *directV6Fixture) get(key []byte) []byte {
	return f.env.GetStorage(&lib.GasMeter{}, key)
}

func (f *directV6Fixture) call(t *testing.T, method string, arg []byte) uint64 {
	t.Helper()
	defer f.env.ResetPayAmount()
	if f.env.payAmount != nil && f.env.payAmount.Sign() == 0 {
		f.env.payAmount = nil
	}
	gas, _, err := lib.Execute(f.api, f.code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("v6 %s rejected by pinned runtime: %v", method, err)
	}
	return gas
}

func (f *directV6Fixture) offer(t *testing.T, id [32]byte) []byte {
	t.Helper()
	raw := f.get(join([]byte("pool/offer/"), id[:]))
	if len(raw) != 238 {
		t.Fatalf("v6 offer record has %d bytes, want 238", len(raw))
	}
	return raw
}

func (f *directV6Fixture) order(t *testing.T) []byte {
	t.Helper()
	raw := f.get([]byte("pool/order/1"))
	if len(raw) != 215 {
		t.Fatalf("v6 order record has %d bytes, want 215", len(raw))
	}
	return raw
}

func (f *directV6Fixture) amount(t *testing.T, key string, want *big.Int) {
	t.Helper()
	raw := f.get([]byte(key))
	if len(raw) != 16 || new(big.Int).SetBytes(raw).Cmp(want) != 0 {
		t.Fatalf("v6 %s = %x, want %s", key, raw, want)
	}
}

func (f *directV6Fixture) units(t *testing.T, key string, want uint64) {
	t.Helper()
	raw := f.get([]byte(key))
	if len(raw) != 8 || binary.BigEndian.Uint64(raw) != want {
		t.Fatalf("v6 %s = %x, want %d", key, raw, want)
	}
}

func (f *directV6Fixture) queueID(t *testing.T, key string, want [32]byte) {
	t.Helper()
	if got := f.get([]byte(key)); !bytes.Equal(got, want[:]) {
		t.Fatalf("v6 %s = %x, want %x", key, got, want)
	}
}

func (f *directV6Fixture) seedQueue(t *testing.T) {
	t.Helper()
	f.queueID(t, "pool/secondary-head", f.seedIDs[0])
	f.queueID(t, "pool/secondary-tail", f.seedIDs[1])
	first, second := f.offer(t, f.seedIDs[0]), f.offer(t, f.seedIDs[1])
	if first[138] != 2 || first[220] != 1 || first[237] != 1 ||
		second[138] != 2 || second[220] != 1 || second[237] != 1 ||
		!bytes.Equal(first[140:172], make([]byte, 32)) || !bytes.Equal(first[172:204], f.seedIDs[1][:]) ||
		!bytes.Equal(second[140:172], f.seedIDs[0][:]) || !bytes.Equal(second[172:204], make([]byte, 32)) {
		t.Fatal("v6 synthetic public seed queue is not linked in FIFO order")
	}
}

func (f *directV6Fixture) qualified(t *testing.T, author lib.Address, epoch uint16) ([32]byte, []byte) {
	t.Helper()
	id := v6OfferID(author, epoch)
	var encoded [2]byte
	binary.BigEndian.PutUint16(encoded[:], epoch)
	metadata := join(encoded[:], []byte{0, 0, 0, 1}, bytes.Repeat([]byte{0x55}, 32), bytes.Repeat([]byte{0x66}, 32))
	qualified := make([]byte, 238)
	copy(qualified[0:20], author[:])
	copy(qualified[20:90], metadata)
	copy(qualified[122:130], u64Arg(1))
	f.put(join([]byte("pool/offer/"), id[:]), qualified)
	return id, metadata
}

func (f *directV6Fixture) currentQuote(t *testing.T) *big.Int {
	t.Helper()
	read := func(key string) *big.Int { return new(big.Int).SetBytes(f.get([]byte(key))) }
	units := read("pool/primary-units").Uint64() + read("pool/secondary-units").Uint64()
	if units < 2 {
		t.Fatal("v6 fixture needs at least two queued units")
	}
	y := new(big.Int).Sub(read("pool/reserve"), read("pool/committed"))
	y.Add(y, read("pool/buyer-payments"))
	y.Sub(y, read("pool/insurance"))
	denominator := new(big.Int).SetUint64(units - 1)
	quote := y.Add(y, new(big.Int).Sub(denominator, big.NewInt(1))).Div(y, denominator)
	cap := mustAmount("1000000000000000000000")
	if quote.Cmp(cap) > 0 {
		quote.Set(cap)
	}
	head := f.get([]byte("pool/primary-head"))
	if bytes.Equal(head, make([]byte, 32)) {
		head = f.get([]byte("pool/secondary-head"))
	}
	if len(head) == 32 {
		locked := new(big.Int).SetBytes(f.get(join([]byte("pool/offer/"), head))[90:106])
		if quote.Cmp(locked) < 0 {
			quote.Set(locked)
		}
	}
	return quote
}

func (f *directV6Fixture) reject(t *testing.T, method string, arg []byte, caller lib.Address, payment *big.Int) {
	t.Helper()
	restore := f.env.Snapshot()
	before := f.env.StateDigest()
	f.env.SetCaller(caller)
	if payment != nil {
		f.env.SetPayAmount(payment)
		f.env.CreditContractBalance(payment)
	}
	_, _, err := lib.Execute(f.api, f.code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	restore()
	f.env.ResetPayAmount()
	if err == nil || f.env.StateDigest() != before {
		t.Fatalf("v6 %s should reject without changing committed state: %v", method, err)
	}
}

func (f *directV6Fixture) list(t *testing.T) uint64 {
	t.Helper()
	f.env.SetCaller(f.author)
	posted := new(big.Int).Add(f.bond, f.payout) // unproved fraud collateral equals payout
	f.env.SetPayAmount(posted)
	f.env.CreditContractBalance(posted)
	gas := f.call(t, "list", join(f.metadata, u128ArgValue(big.NewInt(0))))
	offer := f.offer(t, f.offerID)
	if offer[138] != 1 || offer[139] != 0 || offer[220] != 0 || offer[237] != 0 ||
		new(big.Int).SetBytes(offer[90:106]).Cmp(f.payout) != 0 ||
		new(big.Int).SetBytes(offer[106:122]).Cmp(f.bond) != 0 ||
		new(big.Int).SetBytes(offer[221:237]).Cmp(f.payout) != 0 {
		t.Fatal("v6 listing did not lock its payout, shortfall-plus-proof delivery bond, and fraud collateral")
	}
	f.queueID(t, "pool/primary-head", f.offerID)
	f.queueID(t, "pool/primary-tail", f.offerID)
	f.seedQueue(t)
	f.units(t, "pool/primary-units", 1)
	f.units(t, "pool/secondary-units", 2)
	f.units(t, "pool/units", 3)
	f.amount(t, "pool/reserve", f.reserve)
	f.amount(t, "pool/committed", f.payout)
	f.amount(t, "pool/pending", posted)
	f.amount(t, "pool/buyer-payments", big.NewInt(0))
	f.amount(t, "pool/insurance", big.NewInt(0))
	if f.env.ContractBalance().Cmp(new(big.Int).Add(f.reserve, posted)) != 0 {
		t.Fatal("v6 listing did not retain the seller funds in contract balance")
	}
	return gas
}

func (f *directV6Fixture) buy(t *testing.T) uint64 {
	t.Helper()
	f.env.SetCaller(f.buyer)
	total := new(big.Int).Add(f.quote, f.buyerBond)
	f.env.SetPayAmount(total)
	f.env.CreditContractBalance(total)
	gas := f.call(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
	order := f.order(t)
	if order[181] != 1 || !bytes.Equal(order[:32], f.offerID[:]) || !bytes.Equal(order[32:52], f.buyer[:]) ||
		new(big.Int).SetBytes(order[117:133]).Cmp(f.quote) != 0 ||
		new(big.Int).SetBytes(order[133:149]).Cmp(f.buyerBond) != 0 ||
		new(big.Int).SetBytes(order[149:165]).Cmp(f.payout) != 0 {
		t.Fatal("v6 buy did not retain the quote, buyer bond, and locked payout")
	}
	offer := f.offer(t, f.offerID)
	if offer[138] != 3 || binary.BigEndian.Uint64(offer[130:138]) != 1 ||
		new(big.Int).SetBytes(offer[221:237]).Cmp(f.payout) != 0 {
		t.Fatal("v6 buy did not activate order 1 with fraud collateral held")
	}
	var empty [32]byte
	f.queueID(t, "pool/primary-head", empty)
	f.queueID(t, "pool/primary-tail", empty)
	f.seedQueue(t)
	f.units(t, "pool/primary-units", 0)
	f.units(t, "pool/secondary-units", 2)
	f.units(t, "pool/units", 3)
	f.units(t, "pool/reserved-units", 1)
	f.amount(t, "pool/reserve", f.reserve)
	f.amount(t, "pool/committed", f.payout)
	f.amount(t, "pool/buyer-payments", f.quote)
	f.amount(t, "pool/insurance", big.NewInt(0))
	pending := new(big.Int).Add(new(big.Int).Add(f.bond, f.payout), total)
	f.amount(t, "pool/pending", pending)
	if f.env.ContractBalance().Cmp(new(big.Int).Add(f.reserve, pending)) != 0 {
		t.Fatal("v6 buy did not retain all funded liabilities in contract balance")
	}
	return gas
}

func (f *directV6Fixture) buyPublicSeed(t *testing.T, index int) {
	t.Helper()
	f.env.SetCaller(f.buyer)
	quote := f.currentQuote(t)
	f.env.SetPayAmount(quote)
	f.env.CreditContractBalance(quote)
	f.call(t, "buyPublicSeed", f.seedIDs[index][:])
}

func (f *directV6Fixture) relistPublicSeed(t *testing.T, index int) {
	t.Helper()
	offer := f.offer(t, f.seedIDs[index])
	var author lib.Address
	copy(author[:], offer[:20])
	f.env.SetCaller(author)
	f.call(t, "relist", join(f.seedIDs[index][:], u128ArgValue(big.NewInt(0))))
}

func TestDirectV6SealedSellerFallbackGas(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-v5-one-flip.json")
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Format    string `json:"format"`
		Statement string `json:"statement"`
		ImageID   string `json:"image_id"`
		Journal   string `json:"journal"`
		Seal      string `json:"seal"`
	}
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	if proof.Format != "fod:fast-content-sealed:v1" || proof.Statement != "fod:fast-content:v1" || proof.ImageID != directV6FastContentImageID {
		t.Fatal("synthetic content receipt differs from the pinned v6 claim")
	}
	journal, seal := mustHex(t, proof.Journal), mustHex(t, proof.Seal)
	if len(journal) != 273 || len(seal) != 256 {
		t.Fatalf("invalid content receipt lengths: journal=%d seal=%d", len(journal), len(seal))
	}
	sessionInput, session := fastSessionInputFromFile(t, "../../proofs/fast-content/dist/synthetic-session.json")
	if len(sessionInput) < 56 || !bytes.Equal(journal[56:76], sessionInput[:20]) ||
		!bytes.Equal(journal[76:80], sessionInput[20:24]) ||
		!bytes.Equal(journal[144:176], sessionInput[24:56]) {
		t.Fatal("sealed content receipt and four-leaf session disagree")
	}
	wire := sessionInput[56:]
	if len(wire) == 0 || len(wire) > 65535 {
		t.Fatalf("invalid bounded session witness length: %d", len(wire))
	}
	vectorRaw, err := os.ReadFile("../../proofs/registration/vectors/blob-flip-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		SecretsCanonical string `json:"secretsCanonical"`
	}
	if err := json.Unmarshal(vectorRaw, &vector); err != nil {
		t.Fatal(err)
	}
	secrets := mustHex(t, vector.SecretsCanonical)
	secretsHash := sha256.Sum256(secrets)
	if len(secrets) != 106 || !bytes.Equal(secretsHash[:], journal[241:273]) {
		t.Fatal("public synthetic keys do not open the sealed v6 commitment")
	}

	f := newDirectV6Fixture(t)
	contract := f.env.ContractAddressValue()
	if !bytes.Equal(contract[:], journal[36:56]) {
		t.Fatal("sealed content receipt names a different v6 contract")
	}
	f.author = addressFromHex(t, hexOf(journal[56:76]))
	epoch := binary.BigEndian.Uint16(journal[76:78])
	f.offerID = v6OfferID(f.author, epoch)
	covered := make([]byte, 4)
	binary.BigEndian.PutUint32(covered, uint32(binary.BigEndian.Uint16(journal[78:80])))
	f.metadata = join(journal[76:78], covered, journal[144:176], journal[241:273])
	f.env.SetEpoch(epoch)
	f.env.SetBlockHeader(session.Inventory.Height, proposedHeaderWire(t, session.Inventory.Height, mustHex(t, session.Inventory.Root)))
	f.env.SetBlockHeader(session.Verdict.Height, proposedHeaderWire(t, session.Verdict.Height, mustHex(t, session.Verdict.Root)))
	buyHeight := session.Verdict.Height + 100
	f.env.SetBlockNumber(buyHeight)
	f.env.SetCaller(f.author)
	wireLength := binary.BigEndian.AppendUint16(nil, uint16(len(wire)))
	qualifyArg := join(f.metadata, wireLength, wire)
	qualifyGas := f.call(t, "qualify", qualifyArg)
	qualified := f.offer(t, f.offerID)
	if qualified[138] != 0 || qualified[220] != 0 || !bytes.Equal(qualified[20:90], f.metadata) {
		t.Fatal("v6 qualification did not store the sealed seller's unproved offer")
	}
	if len(wire) < 25 || wire[23] != 1 || int(wire[24]) == 0 || 25+int(wire[24]) > len(wire) {
		t.Fatal("invalid public CID prefix in the sealed session witness")
	}
	wantInventory := append([]byte{wire[23]}, wire[24:25+int(wire[24])]...)
	if !bytes.Equal(f.get(join([]byte("pool/inventory/"), f.offerID[:])), wantInventory) {
		t.Fatal("v6 qualification did not retain the verified public CID inventory")
	}
	listGas := f.list(t)
	buyGas := f.buy(t)
	f.env.SetBlockNumber(buyHeight + directConfirmGraceBlocks)
	f.env.SetCaller(f.author)
	startArg := join(u64Arg(1), journal, seal, wireLength, wire)
	startGas := f.call(t, "startFallback", startArg)
	if f.order(t)[181] != 4 || new(big.Int).SetBytes(f.get([]byte("pool/proof-rate/1"))).Cmp(big.NewInt(1)) != 0 {
		t.Fatal("v6 sealed fallback did not enter proof-backed pending state at the pinned rate")
	}
	revealArg := join(u64Arg(1), secrets)
	revealGas := f.call(t, "proveAndReveal", revealArg)
	if f.order(t)[181] != 2 || !bytes.Equal(f.get([]byte("pool/reveal/1")), secrets) {
		t.Fatal("v6 sealed fallback did not settle and retain the canonical public keys")
	}
	settled := f.offer(t, f.offerID)
	if settled[138] != 6 || settled[220] != 1 || new(big.Int).SetBytes(settled[221:237]).Sign() != 0 {
		t.Fatal("v6 sealed fallback did not retire the now-proven offer and release fraud collateral")
	}
	f.amount(t, "pool/pending", big.NewInt(0))
	reimbursement := big.NewInt(2_500_000)
	wantSeller := new(big.Int).Add(new(big.Int).Mul(f.payout, big.NewInt(2)), f.bond)
	wantSeller.Add(wantSeller, reimbursement)
	wantBuyer := new(big.Int).Sub(f.buyerBond, reimbursement)
	if f.env.BalanceOf(f.author).Cmp(wantSeller) != 0 || f.env.BalanceOf(f.buyer).Cmp(wantBuyer) != 0 {
		t.Fatal("v6 sealed fallback did not distribute payout, bonds, collateral, and reimbursement")
	}

	measure := func(method string, wasmGas uint64, payloadBytes int) uint64 {
		t.Helper()
		txGas := (wasmGas+wasmGasPerUserGas-1)/wasmGasPerUserGas + uint64(payloadBytes+2048)*10
		t.Logf("v6 %s: wasm gas=%d payload bytes=%d conservative tx gas=%d", method, wasmGas, payloadBytes, txGas)
		if txGas > maxUserGasUpgrade11 {
			t.Fatalf("v6 %s exceeds one block's %d-gas limit", method, maxUserGasUpgrade11)
		}
		return txGas
	}
	measure("deploy", f.deployGas, len(f.code))
	measure("activate", f.activateGas, 64)
	measure("qualify", qualifyGas, len(qualifyArg))
	measure("list", listGas, len(f.metadata)+16)
	measure("buy", buyGas, 32+65)
	startTxGas := measure("startFallback", startGas, len(startArg))
	revealTxGas := measure("proveAndReveal", revealGas, len(revealArg))
	if startTxGas+revealTxGas > 2_500_000 {
		t.Fatalf("v6 sealed fallback exceeds the 2,500,000-gas reimbursement allowance: start=%d reveal=%d", startTxGas, revealTxGas)
	}
}

func TestDirectV6SyntheticRuntimeAccounting(t *testing.T) {
	t.Run("direct allocator calls cannot trap attached value", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		f.reject(t, "allocate", []byte{0x42}, f.buyer, big.NewInt(1))
		f.reject(t, "allocate", nil, f.buyer, nil)
	})
	t.Run("nonpayable business calls cannot credit attached value to pool reserve", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		f.list(t)
		f.buy(t)
		f.env.SetCaller(f.buyer)
		f.env.SetPayAmount(big.NewInt(1))
		f.env.CreditContractBalance(big.NewInt(1))
		f.call(t, "challenge", u64Arg(1))
		f.amount(t, "pool/reserve", f.reserve)
	})
	t.Run("zero-value deploy and separately funded activation fit the pinned block gas budget", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		// Reserve 2 KiB for signed transaction framing beyond exact WASM
		// bytes, as in the v5 release measurement.
		conservative := (f.deployGas+wasmGasPerUserGas-1)/wasmGasPerUserGas + uint64(len(f.code)+2048)*10
		if conservative >= maxUserGasUpgrade11 {
			t.Fatalf("v6 deploy exceeds one block: wasm gas=%d bytes=%d conservative tx gas=%d", f.deployGas, len(f.code), conservative)
		}
		activateConservative := (f.activateGas+wasmGasPerUserGas-1)/wasmGasPerUserGas + uint64(64+2048)*10
		if activateConservative >= maxUserGasUpgrade11 {
			t.Fatalf("v6 activate exceeds one block: wasm gas=%d conservative tx gas=%d", f.activateGas, activateConservative)
		}
		t.Logf("v6 deploy: wasm gas=%d wasm bytes=%d conservative tx gas=%d", f.deployGas, len(f.code), conservative)
		t.Logf("v6 activate: wasm gas=%d conservative tx gas=%d", f.activateGas, activateConservative)
	})
	t.Run("seed funding is rejected until the two proved public offers are queued", func(t *testing.T) {
		f := newDirectV6FixtureWithActivation(t, false)
		ids := join(f.seedIDs[0][:], f.seedIDs[1][:])
		f.amount(t, "pool/reserve", big.NewInt(0))
		f.reject(t, "buyPublicSeed", f.seedIDs[0][:], f.buyer, f.quote)
		f.reject(t, "activate", ids, f.author, big.NewInt(1))
		f.reject(t, "activate", join(f.seedIDs[1][:], f.seedIDs[0][:]), f.author, f.reserve)
		f.put([]byte("pool/seed-count"), u64Arg(1))
		f.reject(t, "activate", ids, f.author, f.reserve)
		f.put([]byte("pool/seed-count"), u64Arg(2))
		f.env.SetPayAmount(f.reserve)
		f.env.CreditContractBalance(f.reserve)
		f.call(t, "activate", ids)
		f.amount(t, "pool/reserve", f.reserve)
		f.reject(t, "activate", ids, f.author, f.reserve)
	})
	t.Run("only the two pinned public seed IDs can enter the bootstrap", func(t *testing.T) {
		f := newDirectV6FixtureWithActivation(t, false)
		zeroID := make([]byte, 32)
		f.put([]byte("pool/seed-count"), u64Arg(0))
		f.put([]byte("pool/units"), u64Arg(0))
		f.put([]byte("pool/secondary-units"), u64Arg(0))
		f.put([]byte("pool/secondary-head"), zeroID)
		f.put([]byte("pool/secondary-tail"), zeroID)

		otherAuthor := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xd4}, 20)))
		otherID := v6OfferID(otherAuthor, 230)
		otherSecrets := join([]byte("fod:epoch-secrets:v1"), []byte{0, 230}, otherAuthor[:], bytes.Repeat([]byte{0x64}, 64))
		otherKeys := sha256.Sum256(otherSecrets)
		other := make([]byte, 238)
		copy(other[0:20], otherAuthor[:])
		copy(other[20:22], []byte{0, 230})
		copy(other[22:26], []byte{0, 0, 0, 1})
		copy(other[26:58], bytes.Repeat([]byte{0x54}, 32))
		copy(other[58:90], otherKeys[:])
		copy(other[122:130], u64Arg(1))
		other[220] = 1 // synthetic proof flag; no actual content proof runs here
		f.put(join([]byte("pool/offer/"), otherID[:]), other)
		f.reject(t, "seedBundle", join(otherID[:], otherSecrets), otherAuthor, nil)
		second := f.offer(t, f.seedIDs[1])
		second[138] = 0 // qualified, not queued
		second[237] = 0 // not yet public
		copy(second[140:204], make([]byte, 64))
		f.put(join([]byte("pool/offer/"), f.seedIDs[1][:]), second)
		secondAuthor := addressFromHex(t, hexOf(second[:20]))
		f.put(join([]byte("pool/seller/"), secondAuthor[:]), zeroID)
		f.reject(t, "seedBundle", join(f.seedIDs[1][:], f.get(join([]byte("pool/public-seed/"), f.seedIDs[1][:]))), secondAuthor, nil)

		first := f.offer(t, f.seedIDs[0])
		first[138] = 0 // qualified, not queued
		first[237] = 0 // not yet public
		copy(first[140:204], make([]byte, 64))
		f.put(join([]byte("pool/offer/"), f.seedIDs[0][:]), first)
		firstAuthor := addressFromHex(t, hexOf(first[:20]))
		f.put(join([]byte("pool/seller/"), firstAuthor[:]), zeroID)
		f.env.SetCaller(firstAuthor)
		f.call(t, "seedBundle", join(f.seedIDs[0][:], f.get(join([]byte("pool/public-seed/"), f.seedIDs[0][:]))))
		f.units(t, "pool/seed-count", 1)
		f.queueID(t, "pool/secondary-head", f.seedIDs[0])
	})

	t.Run("ordinary listing waits for the second proven public seed", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		first := f.offer(t, f.seedIDs[0])
		copy(first[172:204], make([]byte, 32))
		f.put(join([]byte("pool/offer/"), f.seedIDs[0][:]), first)
		f.put([]byte("pool/secondary-tail"), f.seedIDs[0][:])
		f.put([]byte("pool/secondary-units"), u64Arg(1))
		f.put([]byte("pool/units"), u64Arg(1))
		f.put([]byte("pool/seed-count"), u64Arg(1))
		posted := new(big.Int).Add(f.bond, f.payout)
		f.reject(t, "list", join(f.metadata, u128ArgValue(big.NewInt(0))), f.author, posted)
	})

	t.Run("advance-proved listing needs only the shortfall and proof-cost delivery bond", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		qualified := f.offer(t, f.offerID)
		qualified[220] = 1 // Synthetic pre-proved fixture; proof entry is a separate release gate.
		f.put(join([]byte("pool/offer/"), f.offerID[:]), qualified)
		f.env.SetCaller(f.author)
		f.env.SetPayAmount(f.bond)
		f.env.CreditContractBalance(f.bond)
		f.call(t, "list", join(f.metadata, u128ArgValue(big.NewInt(0))))
		listed := f.offer(t, f.offerID)
		if new(big.Int).SetBytes(listed[221:237]).Sign() != 0 ||
			new(big.Int).SetBytes(listed[106:122]).Cmp(f.bond) != 0 {
			t.Fatal("v6 pre-proved seller was charged fraud collateral or wrong delivery bond")
		}
		f.env.SetCaller(f.buyer)
		buyerPosted := new(big.Int).Add(f.quote, f.buyerBond)
		f.env.SetPayAmount(buyerPosted)
		f.env.CreditContractBalance(buyerPosted)
		f.call(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		checksum := sha256.Sum256([]byte("verified private delivery"))
		f.call(t, "confirm", join(u64Arg(1), checksum[:]))
		f.amount(t, "pool/pending", big.NewInt(0))
		if !bytes.Equal(f.get(join([]byte("pool/seller/"), f.author[:])), make([]byte, 32)) {
			t.Fatal("v6 proved settlement did not free the seller's next listing slot")
		}
	})

	t.Run("proved bundle sells once as primary then repeatedly pays a 50/50 author dividend", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		proved := f.offer(t, f.offerID)
		proved[220] = 1 // synthetic result of an independent content proof
		f.put(join([]byte("pool/offer/"), f.offerID[:]), proved)
		f.env.SetCaller(f.author)
		f.env.SetPayAmount(f.bond)
		f.env.CreditContractBalance(f.bond)
		f.call(t, "list", join(f.metadata, u128ArgValue(big.NewInt(0))))
		f.env.SetCaller(f.buyer)
		firstTotal := new(big.Int).Add(f.quote, f.buyerBond)
		f.env.SetPayAmount(firstTotal)
		f.env.CreditContractBalance(firstTotal)
		f.call(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		firstHash := sha256.Sum256([]byte("first private delivery"))
		f.call(t, "confirm", join(u64Arg(1), firstHash[:]))
		f.amount(t, "pool/reserve", f.reserve)
		f.amount(t, "pool/committed", big.NewInt(0))
		f.buyPublicSeed(t, 0)

		for orderID := uint64(2); orderID <= 3; orderID++ {
			bond := new(big.Int).Add(f.quote, f.bond)
			f.env.SetCaller(f.author)
			f.env.SetPayAmount(bond)
			f.env.CreditContractBalance(bond)
			f.call(t, "relist", join(f.offerID[:], u128ArgValue(big.NewInt(0))))
			offer := f.offer(t, f.offerID)
			if offer[138] != 2 || offer[139] != 1 ||
				new(big.Int).SetBytes(offer[90:106]).Sign() != 0 ||
				new(big.Int).SetBytes(offer[106:122]).Cmp(bond) != 0 {
				t.Fatal("repeat listing created another primary payout or lost its delivery bond")
			}
			f.amount(t, "pool/committed", big.NewInt(0))
			if orderID == 2 {
				f.buyPublicSeed(t, 1)
				f.relistPublicSeed(t, 0)
			} else {
				f.buyPublicSeed(t, 0)
				f.relistPublicSeed(t, 1)
			}
			f.env.SetCaller(f.buyer)
			f.env.SetPayAmount(firstTotal)
			f.env.CreditContractBalance(firstTotal)
			f.call(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
			order := f.get(join([]byte("pool/order/"), []byte{'0' + byte(orderID)}))
			dividend := new(big.Int).Div(f.quote, big.NewInt(2))
			if len(order) != 215 || new(big.Int).SetBytes(order[149:165]).Cmp(dividend) != 0 {
				t.Fatal("repeat order did not lock half the buyer payment for its original author")
			}
			f.amount(t, "pool/buyer-payments", dividend)
			hash := sha256.Sum256([]byte{byte(orderID)})
			f.call(t, "confirm", join(u64Arg(orderID), hash[:]))
			wantReserve := new(big.Int).Add(f.reserve, new(big.Int).Mul(dividend, new(big.Int).SetUint64(2*orderID-1)))
			f.amount(t, "pool/reserve", wantReserve)
			f.amount(t, "pool/buyer-payments", big.NewInt(0))
			f.amount(t, "pool/pending", big.NewInt(0))
			wantAuthor := new(big.Int).Add(f.payout, f.bond)
			wantAuthor.Add(wantAuthor, new(big.Int).Mul(new(big.Int).Add(dividend, bond), new(big.Int).SetUint64(orderID-1)))
			if f.env.BalanceOf(f.author).Cmp(wantAuthor) != 0 {
				t.Fatal("repeat sale did not pay the original author exactly half the buyer price")
			}
		}
	})

	t.Run("failed repeat delivery refunds the buyer without paying an author dividend", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		proved := f.offer(t, f.offerID)
		proved[220] = 1
		f.put(join([]byte("pool/offer/"), f.offerID[:]), proved)
		f.env.SetCaller(f.author)
		f.env.SetPayAmount(f.bond)
		f.env.CreditContractBalance(f.bond)
		f.call(t, "list", join(f.metadata, u128ArgValue(big.NewInt(0))))
		f.env.SetCaller(f.buyer)
		buyerTotal := new(big.Int).Add(f.quote, f.buyerBond)
		f.env.SetPayAmount(buyerTotal)
		f.env.CreditContractBalance(buyerTotal)
		f.call(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		firstHash := sha256.Sum256([]byte("primary delivery"))
		f.call(t, "confirm", join(u64Arg(1), firstHash[:]))
		f.buyPublicSeed(t, 0)
		f.env.SetCaller(f.author)
		bond := new(big.Int).Add(f.quote, f.bond)
		f.env.SetPayAmount(bond)
		f.env.CreditContractBalance(bond)
		f.call(t, "relist", join(f.offerID[:], u128ArgValue(big.NewInt(0))))
		f.buyPublicSeed(t, 1)
		f.relistPublicSeed(t, 0)
		f.env.SetCaller(f.buyer)
		f.env.SetPayAmount(buyerTotal)
		f.env.CreditContractBalance(buyerTotal)
		f.call(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		f.env.SetBlockNumber(1000 + 12_960)
		f.call(t, "refund", u64Arg(2))
		f.amount(t, "pool/reserve", new(big.Int).Add(f.reserve, new(big.Int).Mul(f.quote, big.NewInt(2))))
		f.amount(t, "pool/committed", big.NewInt(0))
		f.amount(t, "pool/buyer-payments", big.NewInt(0))
		f.amount(t, "pool/pending", big.NewInt(0))
		if f.env.BalanceOf(f.author).Cmp(new(big.Int).Add(f.payout, f.bond)) != 0 ||
			f.env.BalanceOf(f.buyer).Cmp(new(big.Int).Add(new(big.Int).Mul(f.buyerBond, big.NewInt(2)), bond)) != 0 ||
			f.offer(t, f.offerID)[138] != 6 {
			t.Fatal("repeat delivery refund paid a dividend or left the invalid offer queued")
		}
	})

	t.Run("pre-proved public reveal returns the full buyer bond without proof reimbursement", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		secrets := join([]byte("fod:epoch-secrets:v1"), []byte{0, 230}, f.author[:], bytes.Repeat([]byte{0x77}, 64))
		if len(secrets) != 106 {
			t.Fatal("v6 fixture has wrong epoch-secret length")
		}
		keys := sha256.Sum256(secrets)
		qualified := f.offer(t, f.offerID)
		copy(qualified[58:90], keys[:])
		qualified[220] = 1 // Synthetic proof result, without invoking proof entry.
		f.put(join([]byte("pool/offer/"), f.offerID[:]), qualified)
		copy(f.metadata[38:70], keys[:])
		f.env.SetCaller(f.author)
		f.env.SetPayAmount(f.bond)
		f.env.CreditContractBalance(f.bond)
		f.call(t, "list", join(f.metadata, u128ArgValue(big.NewInt(0))))
		f.env.SetCaller(f.buyer)
		buyerPosted := new(big.Int).Add(f.quote, f.buyerBond)
		f.env.SetPayAmount(buyerPosted)
		f.env.CreditContractBalance(buyerPosted)
		f.call(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		f.env.SetBlockNumber(1030)
		f.env.SetCaller(f.author)
		f.call(t, "startFallback", u64Arg(1))
		if len(f.get([]byte("pool/proof-rate/1"))) != 0 {
			t.Fatal("v6 pre-proved fallback recorded reimbursement for a proof it did not verify")
		}
		f.call(t, "proveAndReveal", join(u64Arg(1), secrets))
		if f.order(t)[181] != 2 || !bytes.Equal(f.get([]byte("pool/reveal/1")), secrets) {
			t.Fatal("v6 valid fallback did not settle and retain recoverable public keys")
		}
		wantSeller := new(big.Int).Add(f.payout, f.bond)
		wantBuyer := f.buyerBond
		if f.env.BalanceOf(f.author).Cmp(wantSeller) != 0 || f.env.BalanceOf(f.buyer).Cmp(wantBuyer) != 0 {
			t.Fatal("v6 pre-proved fallback did not return the full buyer bond")
		}
	})

	t.Run("list buy confirm and slash unproved collateral", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		f.list(t)
		f.buy(t)
		checksum := sha256.Sum256([]byte("synthetic private delivery envelope"))
		f.call(t, "confirm", join(u64Arg(1), checksum[:]))
		order, offer := f.order(t), f.offer(t, f.offerID)
		if order[181] != 2 || !bytes.Equal(order[182:214], checksum[:]) ||
			offer[138] != 4 || offer[139] != 1 || offer[220] != 0 ||
			new(big.Int).SetBytes(offer[106:122]).Sign() != 0 ||
			new(big.Int).SetBytes(offer[221:237]).Cmp(f.payout) != 0 {
			t.Fatal("v6 private settlement did not retain unproved fraud collateral")
		}
		wantReserve := new(big.Int).Add(f.reserve, new(big.Int).Sub(f.quote, f.payout))
		f.amount(t, "pool/reserve", wantReserve)
		f.amount(t, "pool/pending", f.payout)
		f.amount(t, "pool/committed", big.NewInt(0))
		f.amount(t, "pool/buyer-payments", big.NewInt(0))
		f.amount(t, "pool/insurance", big.NewInt(0))
		f.units(t, "pool/units", 2)
		f.units(t, "pool/reserved-units", 0)
		if f.env.BalanceOf(f.author).Cmp(new(big.Int).Add(f.payout, f.bond)) != 0 ||
			f.env.BalanceOf(f.buyer).Cmp(f.buyerBond) != 0 ||
			f.env.ContractBalance().Cmp(new(big.Int).Add(wantReserve, f.payout)) != 0 {
			t.Fatal("v6 confirmation transfers or held fraud collateral differ from settlement")
		}
		f.seedQueue(t)

		f.env.SetBlockNumber(1000 + 12_960)
		f.env.SetCaller(f.buyer)
		f.call(t, "slashUnproved", u64Arg(1))
		offer = f.offer(t, f.offerID)
		wantReserve.Add(wantReserve, f.payout)
		if offer[138] != 6 || new(big.Int).SetBytes(offer[221:237]).Sign() != 0 {
			t.Fatal("v6 unproved offer did not retire after its fraud collateral was slashed")
		}
		f.amount(t, "pool/reserve", wantReserve)
		f.amount(t, "pool/pending", big.NewInt(0))
		f.amount(t, "pool/insurance", f.payout)
		if f.env.ContractBalance().Cmp(wantReserve) != 0 {
			t.Fatal("v6 slashed collateral did not become pool reserve")
		}
		if !bytes.Equal(f.get(join([]byte("pool/ban/"), f.author[:])), []byte{1, 0, 0, 0, 240}) {
			t.Fatal("v6 unproved seller was not barred from new sales after slashing")
		}
	})

	t.Run("funded refund forfeits seller bond and fraud collateral", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		f.list(t)
		f.buy(t)
		f.env.SetBlockNumber(1000 + 12_960)
		f.env.SetCaller(addressFromHex(t, hexOf(bytes.Repeat([]byte{0xd3}, 20))))
		f.call(t, "refund", u64Arg(1))
		offer := f.offer(t, f.offerID)
		if f.order(t)[181] != 3 || offer[138] != 6 ||
			binary.BigEndian.Uint64(offer[130:138]) != 0 ||
			new(big.Int).SetBytes(offer[106:122]).Sign() != 0 ||
			new(big.Int).SetBytes(offer[221:237]).Sign() != 0 {
			t.Fatal("v6 refund did not retire the order and clear seller liabilities")
		}
		// One base unit of quote exceeds payout; the rest of the unproved
		// collateral stays in insurance outside the tradable curve reserve.
		wantReserve := new(big.Int).Add(f.reserve, f.quote)
		f.amount(t, "pool/reserve", wantReserve)
		f.amount(t, "pool/pending", big.NewInt(0))
		f.amount(t, "pool/committed", big.NewInt(0))
		f.amount(t, "pool/buyer-payments", big.NewInt(0))
		f.amount(t, "pool/insurance", f.payout)
		f.units(t, "pool/units", 2)
		f.units(t, "pool/reserved-units", 0)
		wantBuyer := new(big.Int).Add(f.buyerBond, new(big.Int).Add(f.payout, f.bond))
		if f.env.BalanceOf(f.buyer).Cmp(wantBuyer) != 0 ||
			f.env.BalanceOf(f.author).Sign() != 0 || f.env.ContractBalance().Cmp(wantReserve) != 0 {
			t.Fatal("v6 refund did not return buyer funds plus seller bond less reserve credit")
		}
		f.seedQueue(t)
		ban := f.get(join([]byte("pool/ban/"), f.author[:]))
		if !bytes.Equal(ban, []byte{1, 0, 0, 0, 240}) ||
			!bytes.Equal(f.get(join([]byte("pool/seller/"), f.author[:])), make([]byte, 32)) {
			t.Fatal("v6 first default did not free seller listing and ban new sales for 10 epochs")
		}
	})

	t.Run("first ban expires after 10 epochs and second default is permanent", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		f.list(t)
		f.buy(t)
		f.env.SetBlockNumber(1000 + 12_960)
		f.call(t, "refund", u64Arg(1))
		id, metadata := f.qualified(t, f.author, 231)
		payout := new(big.Int).Div(new(big.Int).Add(f.reserve, big.NewInt(1)), big.NewInt(3))
		posted := new(big.Int).Add(payout, f.reserve)
		f.reject(t, "list", join(metadata, u128ArgValue(big.NewInt(0))), f.author, posted)
		f.env.SetEpoch(240)
		f.env.SetCaller(f.author)
		f.env.SetPayAmount(posted)
		f.env.CreditContractBalance(posted)
		f.call(t, "list", join(metadata, u128ArgValue(big.NewInt(0))))
		if f.offer(t, id)[138] != 1 {
			t.Fatal("v6 first ban did not expire")
		}
		quote := f.currentQuote(t)
		f.env.SetCaller(f.buyer)
		buyerPosted := new(big.Int).Add(quote, f.buyerBond)
		f.env.SetPayAmount(buyerPosted)
		f.env.CreditContractBalance(buyerPosted)
		f.call(t, "buy", join(id[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		f.env.SetBlockNumber(1000 + 2*12_960)
		f.call(t, "refund", u64Arg(2))
		ban := f.get(join([]byte("pool/ban/"), f.author[:]))
		if !bytes.Equal(ban, []byte{2, 0, 0, 0, 0}) {
			t.Fatal("v6 second default did not record a permanent sales ban")
		}
		f.env.SetEpoch(250)
		_, nextMetadata := f.qualified(t, f.author, 232)
		f.reject(t, "list", join(nextMetadata, u128ArgValue(big.NewInt(0))), f.author, posted)
	})

	t.Run("rising price pauses oldest sale until seller tops up the shortfall", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		// This branch models a pool after prior valid trades reduced its reserve.
		f.reserve = mustAmount("1500000000000000000000")
		f.payout = mustAmount("500000000000000000000")
		f.put([]byte("pool/reserve"), u128ArgValue(f.reserve))
		f.env.SetContractBalance(f.reserve)
		f.list(t)
		secondAuthor := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xe4}, 20)))
		secondID, secondMetadata := f.qualified(t, secondAuthor, 231)
		secondPayout := mustAmount("250000000000000000000")
		minimumBond := big.NewInt(5_000_000)
		secondPosted := new(big.Int).Add(secondPayout, minimumBond)
		f.env.SetCaller(secondAuthor)
		f.env.SetPayAmount(secondPosted)
		f.env.CreditContractBalance(secondPosted)
		f.call(t, "list", join(secondMetadata, u128ArgValue(big.NewInt(0))))
		firstQuote := f.currentQuote(t)
		f.env.SetCaller(f.buyer)
		buyerPosted := new(big.Int).Add(firstQuote, f.buyerBond)
		f.env.SetPayAmount(buyerPosted)
		f.env.CreditContractBalance(buyerPosted)
		f.call(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		checksum := sha256.Sum256([]byte("first delivery"))
		f.call(t, "confirm", join(u64Arg(1), checksum[:]))
		risenQuote := f.currentQuote(t)
		if risenQuote.Cmp(firstQuote) <= 0 {
			t.Fatal("v6 fixture did not increase the oldest price")
		}
		payment := new(big.Int).Add(risenQuote, f.buyerBond)
		f.reject(t, "buy", join(secondID[:], DeliveryKeyBytes(NewDeliveryKey(t))), f.buyer, payment)
		required := new(big.Int).Sub(risenQuote, secondPayout)
		required.Add(required, big.NewInt(5_000_000))
		topUp := new(big.Int).Sub(required, minimumBond)
		if topUp.Sign() <= 0 {
			t.Fatal("v6 fixture did not produce a seller shortfall")
		}
		f.env.SetCaller(secondAuthor)
		f.env.SetPayAmount(topUp)
		f.env.CreditContractBalance(topUp)
		f.call(t, "topUpBond", secondID[:])
		f.env.SetCaller(f.buyer)
		f.env.SetPayAmount(payment)
		f.env.CreditContractBalance(payment)
		f.call(t, "buy", join(secondID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		if f.offer(t, secondID)[138] != 3 {
			t.Fatal("v6 sale did not resume after sufficient top-up")
		}
	})

	t.Run("locked primary payout floors a later lower curve quote", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		// Model a later reserve level while preserving the contract's accounting.
		f.reserve = mustAmount("3000000000000000000000")
		f.put([]byte("pool/reserve"), u128ArgValue(f.reserve))
		f.env.SetContractBalance(f.reserve)
		f.list(t)
		secondAuthor := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xe4}, 20)))
		secondID := v6OfferID(secondAuthor, 231)
		secondMetadata := join([]byte{0, 231, 0, 0, 0, 1}, bytes.Repeat([]byte{0x33}, 32), bytes.Repeat([]byte{0x44}, 32))
		qualified := make([]byte, 238)
		copy(qualified[0:20], secondAuthor[:])
		copy(qualified[20:90], secondMetadata)
		copy(qualified[122:130], u64Arg(1))
		f.put(join([]byte("pool/offer/"), secondID[:]), qualified)
		secondPayout := mustAmount("500000000000000000000")
		secondQuote := mustAmount("500000000000000000000")
		secondBond := big.NewInt(5_000_000)
		secondPosted := new(big.Int).Add(secondPayout, secondBond)
		f.env.SetCaller(secondAuthor)
		f.env.SetPayAmount(secondPosted)
		f.env.CreditContractBalance(secondPosted)
		f.call(t, "list", join(secondMetadata, u128ArgValue(big.NewInt(0))))
		first, second := f.offer(t, f.offerID), f.offer(t, secondID)
		if first[138] != 1 || second[138] != 1 ||
			!bytes.Equal(first[172:204], secondID[:]) || !bytes.Equal(second[140:172], f.offerID[:]) ||
			new(big.Int).SetBytes(second[90:106]).Cmp(secondPayout) != 0 ||
			new(big.Int).SetBytes(second[106:122]).Cmp(secondBond) != 0 {
			t.Fatal("v6 second listing did not preserve FIFO and its independently calculated quote")
		}
		f.queueID(t, "pool/primary-head", f.offerID)
		f.queueID(t, "pool/primary-tail", secondID)
		f.units(t, "pool/primary-units", 2)
		f.units(t, "pool/units", 4)
		f.amount(t, "pool/committed", new(big.Int).Add(f.payout, secondPayout))
		f.amount(t, "pool/pending", new(big.Int).Add(new(big.Int).Add(f.payout, f.bond), secondPosted))

		// The later listing lowers the raw curve quote, but a completed
		// primary sale must never draw the difference from the reserve.
		if secondQuote.Cmp(f.payout) >= 0 {
			t.Fatal("v6 fixture did not reach a lower raw curve quote")
		}
		underpayment := new(big.Int).Add(secondQuote, f.buyerBond)
		f.reject(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))), f.buyer, underpayment)
		paid := f.payout
		f.env.SetCaller(f.buyer)
		buyerPosted := new(big.Int).Add(paid, f.buyerBond)
		f.env.SetPayAmount(buyerPosted)
		f.env.CreditContractBalance(buyerPosted)
		f.call(t, "buy", join(f.offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		order := f.order(t)
		if order[181] != 1 || new(big.Int).SetBytes(order[117:133]).Cmp(paid) != 0 ||
			new(big.Int).SetBytes(order[149:165]).Cmp(f.payout) != 0 {
			t.Fatal("v6 buy did not lock a price covering the original seller payout")
		}
		f.queueID(t, "pool/primary-head", secondID)
		f.queueID(t, "pool/primary-tail", secondID)
		f.units(t, "pool/primary-units", 1)
		f.units(t, "pool/reserved-units", 1)
		f.amount(t, "pool/buyer-payments", paid)
		restore := f.env.Snapshot()
		checksum := sha256.Sum256([]byte("verified private delivery"))
		f.call(t, "confirm", join(u64Arg(1), checksum[:]))
		f.amount(t, "pool/reserve", f.reserve)
		f.amount(t, "pool/committed", secondPayout)
		restore()

		f.env.SetBlockNumber(1000 + 12_960)
		f.env.SetCaller(secondAuthor)
		f.call(t, "refund", u64Arg(1))
		first, second = f.offer(t, f.offerID), f.offer(t, secondID)
		if f.order(t)[181] != 3 || first[138] != 6 || second[138] != 1 ||
			!bytes.Equal(second[140:172], make([]byte, 32)) {
			t.Fatal("v6 low-quote refund did not retire only the funded offer")
		}
		wantReserve := new(big.Int).Add(f.reserve, f.payout)
		wantInsurance := new(big.Int).Set(f.payout)
		f.amount(t, "pool/reserve", wantReserve)
		f.amount(t, "pool/insurance", wantInsurance)
		f.amount(t, "pool/pending", secondPosted)
		f.amount(t, "pool/committed", secondPayout)
		f.amount(t, "pool/buyer-payments", big.NewInt(0))
		f.units(t, "pool/units", 3)
		f.units(t, "pool/reserved-units", 0)
		f.units(t, "pool/primary-units", 1)
		f.queueID(t, "pool/primary-head", secondID)
		f.queueID(t, "pool/primary-tail", secondID)
		f.seedQueue(t)
		wantBuyer := new(big.Int).Add(new(big.Int).Add(paid, f.buyerBond), f.bond)
		if f.env.BalanceOf(f.buyer).Cmp(wantBuyer) != 0 ||
			f.env.BalanceOf(f.author).Sign() != 0 ||
			f.env.ContractBalance().Cmp(new(big.Int).Add(wantReserve, secondPosted)) != 0 {
			t.Fatal("v6 low-quote refund did not return the full paid price and delivery bond")
		}
	})

	t.Run("cancel forfeits fraud collateral and preserves seed FIFO", func(t *testing.T) {
		f := newDirectV6Fixture(t)
		f.list(t)
		f.env.SetCaller(f.author)
		f.env.SetPayAmount(f.quote)
		f.env.CreditContractBalance(f.quote)
		f.call(t, "cancel", f.offerID[:])
		offer := f.offer(t, f.offerID)
		if offer[138] != 5 || new(big.Int).SetBytes(offer[106:122]).Sign() != 0 ||
			new(big.Int).SetBytes(offer[221:237]).Sign() != 0 {
			t.Fatal("v6 cancellation did not clear the delivery bond and fraud collateral")
		}
		wantReserve := new(big.Int).Add(f.reserve, f.quote)
		f.amount(t, "pool/reserve", wantReserve)
		f.amount(t, "pool/pending", big.NewInt(0))
		f.amount(t, "pool/committed", big.NewInt(0))
		f.amount(t, "pool/insurance", f.payout)
		f.units(t, "pool/units", 2)
		f.units(t, "pool/primary-units", 0)
		f.units(t, "pool/secondary-units", 2)
		var empty [32]byte
		f.queueID(t, "pool/primary-head", empty)
		f.queueID(t, "pool/primary-tail", empty)
		f.seedQueue(t)
		if f.env.BalanceOf(f.author).Cmp(new(big.Int).Add(f.payout, f.bond)) != 0 ||
			f.env.ContractBalance().Cmp(wantReserve) != 0 {
			t.Fatal("v6 cancellation did not return payout plus bond and retain the curve cost")
		}

		f.env.SetCaller(f.buyer)
		seedQuote := new(big.Int).Set(f.quote) // the public seed is also capped
		f.env.SetPayAmount(seedQuote)
		f.env.CreditContractBalance(seedQuote)
		f.call(t, "buyPublicSeed", f.seedIDs[0][:])
		seedDividend := new(big.Int).Div(seedQuote, big.NewInt(2))
		wantReserve.Add(wantReserve, new(big.Int).Sub(seedQuote, seedDividend))
		if f.offer(t, f.seedIDs[0])[138] != 6 ||
			!bytes.Equal(f.offer(t, f.seedIDs[1])[140:172], empty[:]) {
			t.Fatal("v6 public seed purchase did not retire and unlink the queue head")
		}
		f.queueID(t, "pool/secondary-head", f.seedIDs[1])
		f.queueID(t, "pool/secondary-tail", f.seedIDs[1])
		f.units(t, "pool/secondary-units", 1)
		f.units(t, "pool/units", 1)
		f.amount(t, "pool/reserve", wantReserve)
		f.amount(t, "pool/insurance", f.payout)
		if f.env.ContractBalance().Cmp(wantReserve) != 0 {
			t.Fatal("v6 public seed purchase did not credit its quote to reserve")
		}
		seedAuthor := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xc1}, 20)))
		if f.env.BalanceOf(seedAuthor).Cmp(seedDividend) != 0 {
			t.Fatal("v6 public seed purchase did not pay its original author half")
		}
		f.env.SetCaller(seedAuthor)
		f.call(t, "relist", join(f.seedIDs[0][:], u128ArgValue(big.NewInt(0))))
		f.queueID(t, "pool/secondary-head", f.seedIDs[1])
		f.queueID(t, "pool/secondary-tail", f.seedIDs[0])
		f.units(t, "pool/secondary-units", 2)
		f.units(t, "pool/units", 2)
		if f.offer(t, f.seedIDs[0])[138] != 2 {
			t.Fatal("v6 first public seed could not return as a repeat-sale unit")
		}
		thirdAuthor := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xd1}, 20)))
		thirdID, _ := f.qualified(t, thirdAuthor, 231)
		secrets := join([]byte("fod:epoch-secrets:v1"), []byte{0, 231}, thirdAuthor[:], bytes.Repeat([]byte{0x71}, 64))
		third := f.offer(t, thirdID)
		keys := sha256.Sum256(secrets)
		copy(third[58:90], keys[:])
		third[220] = 1 // Synthetic independent proof result.
		f.put(join([]byte("pool/offer/"), thirdID[:]), third)
		f.reject(t, "seedBundle", join(thirdID[:], secrets), thirdAuthor, nil)
	})
}
