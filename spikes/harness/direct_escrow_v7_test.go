package harness

// Synthetic v7 queue, accounting, and sealed recovery checks in the pinned
// Idena WASM runtime. The bootstrap seed is injected as pre-qualified public
// inventory; the focused gas test verifies the existing sealed receipt.

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

const directV7WasmPath = "../../contracts/direct-escrow-v7/target/fod_direct_escrow_v7.lowered.wasm"
const directV7FastContentImageID = "4fd124c8cb91370dab60d3e1d7c4e666e79485e210b5f20ed36cc2104883726c"

type directV7Fixture struct {
	env         *MinimalEnv
	api         *lib.GoAPI
	code        []byte
	deployGas   uint64
	seedGas     uint64
	activateGas uint64
	seedID      [32]byte
	seedAuthor  lib.Address
	reserve     *big.Int
}

func v7OfferID(author lib.Address, epoch uint16) [32]byte {
	var encoded [2]byte
	binary.BigEndian.PutUint16(encoded[:], epoch)
	return sha256.Sum256(join([]byte("fod:pool-offer:v7"), author[:], encoded[:]))
}

func (f *directV7Fixture) put(key, value []byte) {
	f.env.SetStorage(&lib.GasMeter{}, key, value)
}

func (f *directV7Fixture) get(key string) []byte {
	return f.env.GetStorage(&lib.GasMeter{}, []byte(key))
}

func (f *directV7Fixture) call(t *testing.T, method string, arg []byte) uint64 {
	t.Helper()
	defer f.env.ResetPayAmount()
	if f.env.payAmount != nil && f.env.payAmount.Sign() == 0 {
		f.env.payAmount = nil
	}
	gas, _, err := lib.Execute(f.api, f.code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("v7 %s rejected by pinned runtime: %v", method, err)
	}
	return gas
}

func (f *directV7Fixture) reject(t *testing.T, method string, arg []byte) {
	t.Helper()
	restore := f.env.Snapshot()
	before := f.env.StateDigest()
	f.env.payAmount = nil
	_, _, err := lib.Execute(f.api, f.code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	restore()
	f.env.ResetPayAmount()
	if err == nil || f.env.StateDigest() != before {
		t.Fatalf("v7 %s should reject without committed state change: %v", method, err)
	}
}

func (f *directV7Fixture) rejectPayable(t *testing.T, method string, arg []byte, payment *big.Int) {
	t.Helper()
	restore := f.env.Snapshot()
	before := f.env.StateDigest()
	f.env.SetPayAmount(payment)
	f.env.CreditContractBalance(payment)
	_, _, err := lib.Execute(f.api, f.code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	restore()
	f.env.ResetPayAmount()
	if err == nil || f.env.StateDigest() != before {
		t.Fatalf("v7 %s should reject payment %s without committed state change: %v", method, payment, err)
	}
}

func (f *directV7Fixture) amount(t *testing.T, key string, want *big.Int) {
	t.Helper()
	got := new(big.Int).SetBytes(f.get(key))
	if got.Cmp(want) != 0 {
		t.Fatalf("v7 %s = %s, want %s", key, got, want)
	}
}

func (f *directV7Fixture) units(t *testing.T, key string, want uint64) {
	t.Helper()
	raw := f.get(key)
	if len(raw) != 8 || binary.BigEndian.Uint64(raw) != want {
		t.Fatalf("v7 %s = %x, want %d", key, raw, want)
	}
}

func newDirectV7DeployedFixture(t *testing.T) *directV7Fixture {
	t.Helper()
	wasmPath := directV7WasmPath
	if candidate := os.Getenv("FOD_DIRECT_V7_TEST_WASM"); candidate != "" {
		wasmPath = candidate
	}
	code, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatalf("read lowered v7 candidate: %v", err)
	}
	env := NewMinimalEnv()
	env.SetEpoch(230)
	env.SetContractAddress(addressFromHex(t, "1111111111111111111111111111111111111111"))
	env.SetBlockNumber(1000)
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	f := &directV7Fixture{env: env, api: api, code: code, reserve: mustAmount("10000000000000000000000")}
	f.seedAuthor = addressFromHex(t, hexOf(bytes.Repeat([]byte{0xc1}, 20)))
	f.seedID = v7OfferID(f.seedAuthor, 228)
	env.payAmount = nil
	deployGas, _, err := lib.Deploy(api, code, [][]byte{f.seedID[:]}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("v7 deploy rejected by pinned runtime: %v", err)
	}
	f.deployGas = deployGas
	env.ResetPayAmount()
	if !bytes.Equal(f.get("pool/seed-id"), f.seedID[:]) {
		t.Fatal("deploy did not pin the one public seed offer")
	}
	return f
}

func (f *directV7Fixture) syntheticSeed(t *testing.T, author lib.Address, epoch uint16, contentProven bool) ([32]byte, []byte) {
	t.Helper()
	id := v7OfferID(author, epoch)
	var epochBytes [2]byte
	binary.BigEndian.PutUint16(epochBytes[:], epoch)
	secrets := join([]byte("fod:epoch-secrets:v1"), epochBytes[:], author[:], bytes.Repeat([]byte{0x31}, 64))
	keys := sha256.Sum256(secrets)
	offer := make([]byte, 222)
	copy(offer[0:20], author[:])
	copy(offer[20:22], epochBytes[:])
	copy(offer[22:26], []byte{0, 0, 0, 1})
	copy(offer[26:58], bytes.Repeat([]byte{0x41}, 32))
	copy(offer[58:90], keys[:])
	copy(offer[106:114], u64Arg(1))
	if contentProven {
		offer[204] = 1 // synthetic pre-proved content
	}
	f.put(join([]byte("pool/offer/"), id[:]), offer)
	return id, secrets
}

func newDirectV7Fixture(t *testing.T) *directV7Fixture {
	t.Helper()
	f := newDirectV7DeployedFixture(t)
	id, secrets := f.syntheticSeed(t, f.seedAuthor, 228, true)
	f.env.SetCaller(f.seedAuthor)
	f.seedGas = f.call(t, "seedBundle", join(id[:], secrets))
	if !bytes.Equal(f.get("pool/public-seed/"+string(id[:])), secrets) {
		t.Fatal("seedBundle did not publicly disclose canonical keys")
	}
	if got := f.get("pool/seller/" + string(f.seedAuthor[:])); len(got) != 0 {
		t.Fatal("public seed occupies its author's private listing slot")
	}
	f.env.SetCaller(addressFromHex(t, hexOf(bytes.Repeat([]byte{0xa7}, 20))))
	f.env.SetPayAmount(f.reserve)
	f.env.CreditContractBalance(f.reserve)
	f.activateGas = f.call(t, "activate", f.seedID[:])
	f.units(t, "pool/units", 1)
	f.units(t, "pool/secondary-units", 1)
	f.units(t, "pool/head-since", 1000)
	return f
}

func TestDirectV7ActivationRequiresOnePinnedProvedSeed(t *testing.T) {
	f := newDirectV7DeployedFixture(t)
	f.rejectPayable(t, "activate", f.seedID[:], f.reserve)
	id, secrets := f.syntheticSeed(t, f.seedAuthor, 228, false)
	f.env.SetCaller(f.seedAuthor)
	f.reject(t, "seedBundle", join(id[:], secrets))
	_, secrets = f.syntheticSeed(t, f.seedAuthor, 228, true)
	other := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xc2}, 20)))
	f.env.SetCaller(other)
	f.reject(t, "seedBundle", join(id[:], secrets))
	otherID, otherSecrets := f.syntheticSeed(t, other, 229, true)
	f.reject(t, "seedBundle", join(otherID[:], otherSecrets))
	f.env.SetCaller(f.seedAuthor)
	f.call(t, "seedBundle", join(id[:], secrets))
	f.units(t, "pool/seed-count", 1)
	f.units(t, "pool/units", 1)
	if !bytes.Equal(f.get("pool/public-seed/"+string(id[:])), secrets) {
		t.Fatal("the pinned seed's canonical keys were not disclosed")
	}
	f.reject(t, "seedBundle", join(id[:], secrets))
	f.rejectPayable(t, "activate", f.seedID[:], new(big.Int).Sub(f.reserve, big.NewInt(1)))
	f.rejectPayable(t, "activate", f.seedID[:], new(big.Int).Add(f.reserve, big.NewInt(1)))
	f.rejectPayable(t, "activate", otherID[:], f.reserve)
	f.env.SetCaller(other)
	f.env.SetPayAmount(f.reserve)
	f.env.CreditContractBalance(f.reserve)
	f.call(t, "activate", f.seedID[:])
	f.amount(t, "pool/reserve", f.reserve)
	f.units(t, "pool/secondary-units", 1)
	f.rejectPayable(t, "activate", f.seedID[:], f.reserve)
}

func (f *directV7Fixture) listQualified(t *testing.T, author lib.Address, epoch uint16, proved bool) [32]byte {
	t.Helper()
	id := v7OfferID(author, epoch)
	var epochBytes [2]byte
	binary.BigEndian.PutUint16(epochBytes[:], epoch)
	metadata := join(epochBytes[:], []byte{0, 0, 0, 1}, bytes.Repeat([]byte{0x55}, 32), bytes.Repeat([]byte{0x66}, 32))
	offer := make([]byte, 222)
	copy(offer[0:20], author[:])
	copy(offer[20:90], metadata)
	copy(offer[106:114], u64Arg(1))
	if proved {
		offer[204] = 1 // synthetic pre-proved content
	}
	f.put(join([]byte("pool/offer/"), id[:]), offer)
	f.env.SetCaller(author)
	posted := big.NewInt(5_000_000)
	if !proved {
		posted.Add(posted, mustAmount("1000000000000000000000"))
	}
	f.env.SetPayAmount(posted)
	f.env.CreditContractBalance(posted)
	f.call(t, "list", join(metadata, u128ArgValue(big.NewInt(0))))
	return id
}

func (f *directV7Fixture) listProved(t *testing.T, author lib.Address, epoch uint16) [32]byte {
	return f.listQualified(t, author, epoch, true)
}

func TestDirectV7SinglePublicSeedRemainsAndHeadTimeoutResets(t *testing.T) {
	f := newDirectV7Fixture(t)
	buyer := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb2}, 20)))
	price := mustAmount("1000000000000000000000")
	for i := 0; i < 2; i++ {
		f.env.SetCaller(buyer)
		f.env.SetPayAmount(price)
		f.env.CreditContractBalance(price)
		f.call(t, "buyPublicSeed", f.seedID[:])
		f.units(t, "pool/units", 1)
		f.units(t, "pool/secondary-units", 1)
		f.amount(t, "pool/reserve", new(big.Int).Add(f.reserve, new(big.Int).Mul(big.NewInt(int64(i+1)), new(big.Int).Div(price, big.NewInt(2)))))
		if f.env.BalanceOf(f.seedAuthor).Cmp(new(big.Int).Mul(big.NewInt(int64(i+1)), new(big.Int).Div(price, big.NewInt(2)))) != 0 {
			t.Fatal("public seed author did not receive half the funded price")
		}
	}
	if !bytes.Equal(f.get("pool/secondary-head"), f.seedID[:]) ||
		!bytes.Equal(f.get("pool/secondary-tail"), f.seedID[:]) {
		t.Fatal("the sole proved public seed did not remain in the secondary queue")
	}
	f.env.SetBlockNumber(1000 + 30_240)
	f.call(t, "expireHead", f.seedID[:])
	if !bytes.Equal(f.get("pool/secondary-head"), f.seedID[:]) {
		t.Fatal("expired sole public seed was removed")
	}
	f.units(t, "pool/units", 1)
	f.units(t, "pool/secondary-units", 1)
	f.units(t, "pool/head-since", 1000+30_240)
	f.reject(t, "expireHead", f.seedID[:])
	privateID := f.listProved(t, f.seedAuthor, 230)
	if !bytes.Equal(f.get("pool/primary-head"), privateID[:]) {
		t.Fatal("the seed author's separate private listing did not take primary priority")
	}
	f.units(t, "pool/units", 2)
	f.units(t, "pool/primary-units", 1)
}

func TestDirectV7PrimaryPayoutTracksCurrentCurve(t *testing.T) {
	f := newDirectV7Fixture(t)
	var first [32]byte
	var firstAuthor lib.Address
	for i := 0; i < 11; i++ {
		author := addressFromHex(t, hexOf(bytes.Repeat([]byte{byte(0x81 + i)}, 20)))
		id := f.listProved(t, author, 230)
		if i == 0 {
			first = id
			firstAuthor = author
			firstQuote := new(big.Int).Div(new(big.Int).Add(f.reserve, big.NewInt(10)), big.NewInt(11))
			if firstQuote.Cmp(mustAmount("1000000000000000000000")) >= 0 {
				t.Fatal("the first private listing did not lower the initial price below the cap")
			}
			f.rejectPayable(t, "buyPublicSeed", f.seedID[:], firstQuote)
		}
	}
	f.units(t, "pool/units", 12)
	f.units(t, "pool/primary-units", 11)
	buyer := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb2}, 20)))
	quote := new(big.Int).Div(new(big.Int).Add(f.reserve, big.NewInt(20)), big.NewInt(21))
	if quote.Cmp(mustAmount("1000000000000000000000")) >= 0 {
		t.Fatal("extra primary inventory did not lower the curve price")
	}
	bond := big.NewInt(5_000_000)
	total := new(big.Int).Add(quote, bond)
	f.env.SetCaller(buyer)
	f.env.SetPayAmount(total)
	f.env.CreditContractBalance(total)
	f.call(t, "buy", join(first[:], DeliveryKeyBytes(NewDeliveryKey(t))))
	order := f.get("pool/order/1")
	if len(order) != 215 || new(big.Int).SetBytes(order[117:133]).Cmp(quote) != 0 ||
		new(big.Int).SetBytes(order[149:165]).Cmp(quote) != 0 {
		t.Fatal("first-sale payout was not fixed to the buyer's live curve payment")
	}
	f.env.SetCaller(buyer)
	f.call(t, "confirm", join(u64Arg(1), bytes.Repeat([]byte{0x42}, 32)))
	f.amount(t, "pool/reserve", f.reserve)
	if f.env.BalanceOf(firstAuthor).Cmp(new(big.Int).Add(quote, bond)) != 0 {
		t.Fatal("first-sale author did not receive the live buyer price and returned bond")
	}
	f.units(t, "pool/units", 11)
	f.units(t, "pool/primary-units", 10)
	if got := f.get("pool/order/1"); len(got) != 215 || got[181] != 2 {
		t.Fatal("buyer confirmation did not settle")
	}
}

func TestDirectV7RefundRestoresBuyerWithoutSpendingSeed(t *testing.T) {
	f := newDirectV7Fixture(t)
	author := addressFromHex(t, hexOf(bytes.Repeat([]byte{0x91}, 20)))
	offerID := f.listQualified(t, author, 230, false)
	quote := new(big.Int).Div(new(big.Int).Add(f.reserve, big.NewInt(10)), big.NewInt(11))
	bond := big.NewInt(5_000_000)
	buyer := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb2}, 20)))
	total := new(big.Int).Add(quote, bond)
	f.env.SetCaller(buyer)
	f.env.SetPayAmount(total)
	f.env.CreditContractBalance(total)
	f.call(t, "buy", join(offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
	f.amount(t, "pool/reserve", f.reserve)
	f.env.SetBlockNumber(1000 + 12_960)
	f.env.SetCaller(addressFromHex(t, hexOf(bytes.Repeat([]byte{0xa7}, 20))))
	f.call(t, "refund", u64Arg(1))
	insurance := mustAmount("1000000000000000000000")
	f.amount(t, "pool/reserve", new(big.Int).Add(f.reserve, insurance))
	f.amount(t, "pool/insurance", insurance)
	f.amount(t, "pool/pending", big.NewInt(0))
	f.units(t, "pool/units", 1)
	if f.env.BalanceOf(buyer).Cmp(new(big.Int).Add(total, bond)) != 0 {
		t.Fatal("buyer did not receive the full price, buyer bond, and seller default bond")
	}
	if got := f.get("pool/order/1"); len(got) != 215 || got[181] != 3 {
		t.Fatal("refunded order did not close")
	}
	f.env.SetCaller(buyer)
	f.reject(t, "confirm", join(u64Arg(1), bytes.Repeat([]byte{0x42}, 32)))
}

func TestDirectV7SealedContentProofAgainstCanonicalSession(t *testing.T) {
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
	if proof.Format != "fod:fast-content-sealed:v1" || proof.Statement != "fod:fast-content:v1" ||
		proof.ImageID != directV7FastContentImageID {
		t.Fatal("synthetic sealed content receipt differs from v7's pinned guest image")
	}
	journal, seal := mustHex(t, proof.Journal), mustHex(t, proof.Seal)
	if len(journal) != 273 || len(seal) != 256 {
		t.Fatal("invalid sealed content lengths")
	}
	sessionInput, session := fastSessionInputFromFile(t, "../../proofs/fast-content/dist/synthetic-session.json")
	if !bytes.Equal(journal[56:76], sessionInput[:20]) ||
		!bytes.Equal(journal[76:80], sessionInput[20:24]) ||
		!bytes.Equal(journal[144:176], sessionInput[24:56]) {
		t.Fatal("sealed journal disagrees with canonical session witness")
	}
	wire := sessionInput[56:]
	if len(wire) == 0 || len(wire) > 65535 {
		t.Fatal("invalid session wire length")
	}
	f := newDirectV7Fixture(t)
	contract := f.env.ContractAddressValue()
	if !bytes.Equal(contract[:], journal[36:56]) {
		t.Fatal("sealed journal names another contract")
	}
	var author lib.Address
	copy(author[:], journal[56:76])
	epoch := binary.BigEndian.Uint16(journal[76:78])
	offerID := v7OfferID(author, epoch)
	covered := make([]byte, 4)
	binary.BigEndian.PutUint32(covered, uint32(binary.BigEndian.Uint16(journal[78:80])))
	metadata := join(journal[76:78], covered, journal[144:176], journal[241:273])
	f.env.SetEpoch(epoch)
	f.env.SetBlockHeader(session.Inventory.Height, proposedHeaderWire(t, session.Inventory.Height, mustHex(t, session.Inventory.Root)))
	f.env.SetBlockHeader(session.Verdict.Height, proposedHeaderWire(t, session.Verdict.Height, mustHex(t, session.Verdict.Root)))
	f.env.SetBlockNumber(session.Verdict.Height + 100)
	f.env.SetCaller(author)
	wireLength := binary.BigEndian.AppendUint16(nil, uint16(len(wire)))
	qualifyArg := join(metadata, wireLength, wire)
	qualifyGas := f.call(t, "qualify", qualifyArg)
	qualified := f.get("pool/offer/" + string(offerID[:]))
	if len(qualified) != 222 || qualified[122] != 0 || qualified[204] != 0 {
		t.Fatal("v7 qualification did not retain unproved original inventory")
	}
	f.env.SetCaller(addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb2}, 20))))
	proveArg := join(offerID[:], journal, seal, wireLength, wire)
	proveGas := f.call(t, "proveBundle", proveArg)
	for method, sample := range map[string]struct {
		wasm  uint64
		bytes int
	}{
		"qualify":     {qualifyGas, len(qualifyArg)},
		"proveBundle": {proveGas, len(proveArg)},
	} {
		conservative := (sample.wasm+wasmGasPerUserGas-1)/wasmGasPerUserGas + uint64(sample.bytes+2048)*10
		if conservative > maxUserGasUpgrade11 {
			t.Fatalf("v7 %s exceeds one block's gas limit: %d", method, conservative)
		}
		t.Logf("v7 %s conservative transaction gas: %d", method, conservative)
	}
	proved := f.get("pool/offer/" + string(offerID[:]))
	if len(proved) != 222 || proved[204] != 1 || proved[122] != 0 {
		t.Fatal("v7 did not accept the valid sealed content proof")
	}
}

// Opt-in check of the real epoch-230 v6 receipt against v7's pre-sale legacy
// path. The three owner-only inputs are supplied outside the checkout.
func TestDirectV7RealLegacyContentProof(t *testing.T) {
	paths := []struct {
		name string
		env  string
		out  any
	}{
		{name: "receipt", env: "FOD_DIRECT_V7_REAL_RECEIPT"},
		{name: "session", env: "FOD_DIRECT_V7_REAL_SESSION"},
		{name: "headers", env: "FOD_DIRECT_V7_REAL_HEADERS"},
	}
	var proof struct {
		Format    string `json:"format"`
		Statement string `json:"statement"`
		ImageID   string `json:"image_id"`
		Journal   string `json:"journal"`
		Seal      string `json:"seal"`
		SealBytes int    `json:"sealBytes"`
	}
	var session struct {
		Format    string     `json:"format"`
		Version   int        `json:"version"`
		Author    string     `json:"author"`
		Epoch     uint16     `json:"epoch"`
		Manifest  []string   `json:"manifest"`
		Inventory fastAnchor `json:"inventory"`
		Verdict   fastAnchor `json:"verdict"`
	}
	var headers struct {
		Inventory struct {
			Height    uint64 `json:"height"`
			StateRoot string `json:"stateRoot"`
		} `json:"inventory"`
		Verdict struct {
			Height    uint64 `json:"height"`
			StateRoot string `json:"stateRoot"`
		} `json:"verdict"`
	}
	paths[0].out, paths[1].out, paths[2].out = &proof, &session, &headers
	provided := 0
	for _, input := range paths {
		if os.Getenv(input.env) != "" {
			provided++
		}
	}
	if provided == 0 {
		t.Skip("real receipt, session, and independent header inputs were not supplied")
	}
	if provided != len(paths) {
		t.Fatal("real receipt test needs all three owner-only input paths")
	}
	for _, input := range paths {
		path := os.Getenv(input.env)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 2<<20 {
			t.Fatalf("%s must be a bounded owner-only regular file", input.name)
		}
		raw, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(raw, input.out) != nil {
			t.Fatalf("cannot read %s input", input.name)
		}
	}
	journal, seal := fastHex(t, proof.Journal), fastHex(t, proof.Seal)
	if proof.Format != "fod:fast-content-sealed:v1" || proof.Statement != "fod:fast-content:v1" ||
		proof.ImageID != directV7FastContentImageID || len(journal) != 273 ||
		len(seal) != 256 || proof.SealBytes != len(seal) {
		t.Fatal("real receipt does not match the pinned v7 content proof format")
	}
	author := fastHex(t, session.Author)
	epoch := binary.BigEndian.Uint16(journal[76:78])
	count := binary.BigEndian.Uint16(journal[78:80])
	if session.Format != "fod.session-bundle" || session.Version != 1 ||
		session.Epoch != 230 || session.Epoch != epoch || count != uint16(len(session.Manifest)) ||
		count == 0 || count > 32 || len(author) != 20 || !bytes.Equal(author, journal[56:76]) {
		t.Fatal("real receipt and captured session disagree")
	}
	if session.Inventory.Height == 0 || session.Verdict.Height <= session.Inventory.Height ||
		session.Verdict.Height > ^uint64(0)-100 ||
		headers.Inventory.Height != session.Inventory.Height ||
		headers.Verdict.Height != session.Verdict.Height {
		t.Fatal("real session anchor heights disagree with independent headers")
	}
	invRoot, verdictRoot := fastHex(t, headers.Inventory.StateRoot), fastHex(t, headers.Verdict.StateRoot)
	if len(invRoot) != 32 || len(verdictRoot) != 32 ||
		!bytes.Equal(invRoot, fastHex(t, session.Inventory.Root)) ||
		!bytes.Equal(verdictRoot, fastHex(t, session.Verdict.Root)) {
		t.Fatal("real session roots disagree with independent headers")
	}
	wire := append([]byte{1}, author...)
	fastU16(&wire, session.Epoch)
	wire = append(wire, byte(len(session.Manifest)))
	for _, cid := range session.Manifest {
		if len(cid) == 0 || len(cid) > 128 {
			t.Fatal("invalid real session manifest CID")
		}
		wire = append(wire, byte(len(cid)))
		wire = append(wire, cid...)
	}
	fastAnchorWire(t, &wire, session.Inventory)
	fastAnchorWire(t, &wire, session.Verdict)
	if len(wire) == 0 || len(wire) > 65535 {
		t.Fatal("real session wire exceeds the contract bound")
	}

	f := newDirectV7Fixture(t)
	contract := f.env.ContractAddressValue()
	if bytes.Equal(contract[:], journal[36:56]) {
		t.Fatal("real receipt would take the native v7 proof path")
	}
	f.env.SetEpoch(session.Epoch)
	f.env.SetBlockHeader(session.Inventory.Height, proposedHeaderWire(t, session.Inventory.Height, invRoot))
	f.env.SetBlockHeader(session.Verdict.Height, proposedHeaderWire(t, session.Verdict.Height, verdictRoot))
	f.env.SetBlockNumber(session.Verdict.Height + 100)
	var seller lib.Address
	copy(seller[:], author)
	offerID := v7OfferID(seller, session.Epoch)
	f.env.SetCaller(seller)
	metadata := join(journal[76:78], []byte{0, 0, 0, byte(count)}, journal[144:176], journal[241:273])
	wireLength := binary.BigEndian.AppendUint16(nil, uint16(len(wire)))
	f.call(t, "qualify", join(metadata, wireLength, wire))
	qualified := f.get("pool/offer/" + string(offerID[:]))
	if len(qualified) != 222 || qualified[122] != 0 || qualified[204] != 0 {
		t.Fatal("real offer is not qualified and unproved")
	}
	f.env.SetCaller(addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb2}, 20))))
	proveArg := join(offerID[:], journal, seal, wireLength, wire)
	tampered := append([]byte(nil), proveArg...)
	tampered[32+len(journal)] ^= 1
	f.reject(t, "proveBundle", tampered)
	gas := f.call(t, "proveBundle", proveArg)
	proved := f.get("pool/offer/" + string(offerID[:]))
	if len(proved) != 222 || proved[122] != 0 || proved[204] != 1 {
		t.Fatal("v7 did not accept the real legacy content receipt")
	}
	conservative := (gas+wasmGasPerUserGas-1)/wasmGasPerUserGas + uint64(len(proveArg)+2048)*10
	if conservative >= maxUserGasUpgrade11 {
		t.Fatalf("real legacy proveBundle exceeds one block: %d gas", conservative)
	}
	t.Logf("real legacy proveBundle conservative transaction gas: %d", conservative)
}

// Measure the exact one-seed bootstrap and both funded seller recovery paths.
// The short path uses an independently pre-proved offer; the long path verifies
// the published synthetic Groth16 seal when the funded seller starts fallback.
func TestDirectV7CandidateGas(t *testing.T) {
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
	if proof.Format != "fod:fast-content-sealed:v1" || proof.Statement != "fod:fast-content:v1" ||
		proof.ImageID != directV7FastContentImageID {
		t.Fatal("synthetic receipt differs from the pinned v7 guest image")
	}
	journal, seal := mustHex(t, proof.Journal), mustHex(t, proof.Seal)
	if len(journal) != 273 || len(seal) != 256 {
		t.Fatal("invalid sealed content lengths")
	}
	sessionInput, session := fastSessionInputFromFile(t, "../../proofs/fast-content/dist/synthetic-session.json")
	if len(sessionInput) < 56 || !bytes.Equal(journal[56:76], sessionInput[:20]) ||
		!bytes.Equal(journal[76:80], sessionInput[20:24]) ||
		!bytes.Equal(journal[144:176], sessionInput[24:56]) {
		t.Fatal("sealed receipt and canonical session disagree")
	}
	wire := sessionInput[56:]
	if len(wire) == 0 || len(wire) > 65535 {
		t.Fatal("invalid bounded session wire")
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
		t.Fatal("public synthetic keys do not open the sealed commitment")
	}
	author := addressFromHex(t, hexOf(journal[56:76]))
	buyer := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb2}, 20)))
	epoch := binary.BigEndian.Uint16(journal[76:78])
	offerID := v7OfferID(author, epoch)
	covered := make([]byte, 4)
	binary.BigEndian.PutUint32(covered, uint32(binary.BigEndian.Uint16(journal[78:80])))
	metadata := join(journal[76:78], covered, journal[144:176], journal[241:273])
	wireLength := binary.BigEndian.AppendUint16(nil, uint16(len(wire)))
	qualifyArg := join(metadata, wireLength, wire)
	proveArg := join(offerID[:], journal, seal, wireLength, wire)
	listArg := join(metadata, u128ArgValue(big.NewInt(0)))
	buyHeight := session.Verdict.Height + 100
	bond := big.NewInt(5_000_000)
	priceCap := mustAmount("1000000000000000000000")

	prepare := func(f *directV7Fixture) {
		t.Helper()
		if contract := f.env.ContractAddressValue(); !bytes.Equal(contract[:], journal[36:56]) {
			t.Fatal("sealed receipt names another contract")
		}
		f.env.SetEpoch(epoch)
		f.env.SetBlockHeader(session.Inventory.Height, proposedHeaderWire(t, session.Inventory.Height, mustHex(t, session.Inventory.Root)))
		f.env.SetBlockHeader(session.Verdict.Height, proposedHeaderWire(t, session.Verdict.Height, mustHex(t, session.Verdict.Root)))
		f.env.SetBlockNumber(buyHeight)
		f.env.SetCaller(author)
	}
	buy := func(f *directV7Fixture) (uint64, uint64) {
		t.Helper()
		quote := new(big.Int).Div(new(big.Int).Add(f.reserve, big.NewInt(10)), big.NewInt(11))
		payment := new(big.Int).Add(quote, bond)
		f.env.SetCaller(buyer)
		f.env.SetPayAmount(payment)
		f.env.CreditContractBalance(payment)
		gas := f.call(t, "buy", join(offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
		if order := f.get("pool/order/1"); len(order) != 215 ||
			new(big.Int).SetBytes(order[117:133]).Cmp(quote) != 0 {
			t.Fatal("v7 buy did not retain the measured funded order")
		}
		challengeGas := f.call(t, "challenge", u64Arg(1))
		f.env.SetBlockNumber(buyHeight + directConfirmGraceBlocks)
		f.env.SetCaller(author)
		return gas, challengeGas
	}

	proved := newDirectV7Fixture(t)
	prepare(proved)
	qualifyGas := proved.call(t, "qualify", qualifyArg)
	if offer := proved.get("pool/offer/" + string(offerID[:])); len(offer) != 222 || offer[204] != 0 {
		t.Fatal("canonical qualification did not retain the unproved offer")
	}
	proved.env.SetCaller(buyer)
	proveGas := proved.call(t, "proveBundle", proveArg)
	if offer := proved.get("pool/offer/" + string(offerID[:])); len(offer) != 222 || offer[204] != 1 {
		t.Fatal("v7 did not accept the sealed pre-listing content proof")
	}
	proved.env.SetCaller(author)
	proved.env.SetPayAmount(bond)
	proved.env.CreditContractBalance(bond)
	provedListGas := proved.call(t, "list", listArg)
	provedBuyGas, provedChallengeGas := buy(proved)
	shortArg := u64Arg(1)
	shortGas := proved.call(t, "startFallback", shortArg)
	if order := proved.get("pool/order/1"); len(order) != 215 || order[181] != 4 {
		t.Fatal("v7 short fallback did not enter pending state")
	}
	revealArg := join(u64Arg(1), secrets)
	shortRevealGas := proved.call(t, "proveAndReveal", revealArg)
	reimbursement := big.NewInt(2_500_000)
	quote := new(big.Int).Div(new(big.Int).Add(proved.reserve, big.NewInt(10)), big.NewInt(11))
	if order := proved.get("pool/order/1"); len(order) != 215 || order[181] != 2 ||
		!bytes.Equal(proved.get("pool/reveal/1"), secrets) ||
		new(big.Int).SetBytes(proved.get("pool/proof-rate/1")).Cmp(big.NewInt(1)) != 0 {
		t.Fatal("v7 short fallback did not settle with the pinned proof-request rate")
	}
	wantShortSeller := new(big.Int).Add(quote, new(big.Int).Add(bond, reimbursement))
	if proved.env.BalanceOf(author).Cmp(wantShortSeller) != 0 ||
		proved.env.BalanceOf(buyer).Cmp(new(big.Int).Sub(bond, reimbursement)) != 0 {
		t.Fatal("v7 short fallback did not distribute the capped proof-request reimbursement")
	}

	sealed := newDirectV7Fixture(t)
	prepare(sealed)
	sealedQualifyGas := sealed.call(t, "qualify", qualifyArg)
	if sealedQualifyGas != qualifyGas {
		t.Fatal("canonical qualification gas changed between identical bootstrap states")
	}
	if offer := sealed.get("pool/offer/" + string(offerID[:])); len(offer) != 222 || offer[204] != 0 {
		t.Fatal("v7 sealed seller was not unproved after canonical qualification")
	}
	sealed.env.SetPayAmount(new(big.Int).Add(bond, priceCap))
	sealed.env.CreditContractBalance(new(big.Int).Add(bond, priceCap))
	sealedListGas := sealed.call(t, "list", listArg)
	if offer := sealed.get("pool/offer/" + string(offerID[:])); len(offer) != 222 ||
		new(big.Int).SetBytes(offer[205:221]).Cmp(priceCap) != 0 {
		t.Fatal("v7 unproved listing did not lock the fixed fraud collateral")
	}
	sealedBuyGas, sealedChallengeGas := buy(sealed)
	sealedStartArg := join(u64Arg(1), journal, seal, wireLength, wire)
	sealedStartGas := sealed.call(t, "startFallback", sealedStartArg)
	if order := sealed.get("pool/order/1"); len(order) != 215 || order[181] != 4 {
		t.Fatal("v7 sealed fallback did not enter pending state")
	}
	if rate := new(big.Int).SetBytes(sealed.get("pool/proof-rate/1")); rate.Cmp(big.NewInt(1)) != 0 {
		t.Fatal("v7 sealed fallback did not record the pinned proof rate")
	}
	sealedRevealGas := sealed.call(t, "proveAndReveal", revealArg)
	if order := sealed.get("pool/order/1"); len(order) != 215 || order[181] != 2 ||
		!bytes.Equal(sealed.get("pool/reveal/1"), secrets) {
		t.Fatal("v7 sealed fallback did not settle and reveal canonical keys")
	}
	if offer := sealed.get("pool/offer/" + string(offerID[:])); len(offer) != 222 ||
		offer[204] != 1 || new(big.Int).SetBytes(offer[205:221]).Sign() != 0 {
		t.Fatal("v7 sealed fallback did not release the fraud collateral")
	}
	wantSealedSeller := new(big.Int).Add(wantShortSeller, priceCap)
	if sealed.env.BalanceOf(author).Cmp(wantSealedSeller) != 0 ||
		sealed.env.BalanceOf(buyer).Cmp(new(big.Int).Sub(bond, reimbursement)) != 0 {
		t.Fatal("v7 sealed fallback did not distribute collateral and capped reimbursement")
	}

	measure := func(method string, wasmGas uint64, payloadBytes int) uint64 {
		t.Helper()
		if wasmGas == 0 || payloadBytes <= 0 {
			t.Fatalf("v7 %s has no runtime measurement", method)
		}
		txGas := (wasmGas+wasmGasPerUserGas-1)/wasmGasPerUserGas + uint64(payloadBytes+2048)*10
		t.Logf("v7 %s: wasm gas=%d payload bytes=%d conservative tx gas=%d", method, wasmGas, payloadBytes, txGas)
		if txGas >= maxUserGasUpgrade11 {
			t.Fatalf("v7 %s exceeds one block's %d-gas limit", method, maxUserGasUpgrade11)
		}
		return txGas
	}
	measure("deploy", proved.deployGas, len(proved.code)+len(proved.seedID))
	measure("seedBundle", proved.seedGas, 32+106)
	measure("activate", proved.activateGas, 32)
	measure("qualify", qualifyGas, len(qualifyArg))
	measure("proveBundle", proveGas, len(proveArg))
	measure("listProved", provedListGas, len(listArg))
	measure("buyProved", provedBuyGas, 32+65)
	measure("challengeProved", provedChallengeGas, 8)
	shortTxGas := measure("startFallbackShort", shortGas, len(shortArg))
	shortRevealTxGas := measure("proveAndRevealShort", shortRevealGas, len(revealArg))
	measure("listSealed", sealedListGas, len(listArg))
	measure("buySealed", sealedBuyGas, 32+65)
	measure("challengeSealed", sealedChallengeGas, 8)
	sealedTxGas := measure("startFallbackSealed", sealedStartGas, len(sealedStartArg))
	sealedRevealTxGas := measure("proveAndRevealSealed", sealedRevealGas, len(revealArg))
	if shortTxGas+shortRevealTxGas > 2_500_000 || sealedTxGas+sealedRevealTxGas > 2_500_000 {
		t.Fatal("v7 seller fallback exceeds the buyer's 2,500,000-gas reimbursement allowance")
	}
}

func TestDirectV7FeeRiseRequiresSellerBondTopUp(t *testing.T) {
	f := newDirectV7Fixture(t)
	author := addressFromHex(t, hexOf(bytes.Repeat([]byte{0x93}, 20)))
	offerID := f.listProved(t, author, 230)
	f.env.feePerGas = big.NewInt(2)
	quote := new(big.Int).Div(new(big.Int).Add(f.reserve, big.NewInt(10)), big.NewInt(11))
	total := new(big.Int).Add(quote, big.NewInt(10_000_000))
	buyer := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb2}, 20)))
	restore := f.env.Snapshot()
	before := f.env.StateDigest()
	f.env.SetCaller(buyer)
	f.env.SetPayAmount(total)
	f.env.CreditContractBalance(total)
	_, _, err := lib.Execute(f.api, f.code, "buy", [][]byte{join(offerID[:], DeliveryKeyBytes(NewDeliveryKey(t)))}, lib.Address{}, maxCallGasWasm, false)
	restore()
	f.env.ResetPayAmount()
	if err == nil || f.env.StateDigest() != before {
		t.Fatal("v7 accepted an underbonded queue head after fees rose")
	}
	f.env.SetCaller(author)
	f.env.SetPayAmount(big.NewInt(5_000_000))
	f.env.CreditContractBalance(big.NewInt(5_000_000))
	f.call(t, "topUpBond", offerID[:])
	f.env.SetCaller(buyer)
	f.env.SetPayAmount(total)
	f.env.CreditContractBalance(total)
	f.call(t, "buy", join(offerID[:], DeliveryKeyBytes(NewDeliveryKey(t))))
	if got := f.get("pool/order/1"); len(got) != 215 || got[181] != 1 {
		t.Fatal("bond top-up did not make the funded order available")
	}
}

func TestDirectV7UnchallengedFallbackReturnsBuyerBond(t *testing.T) {
	f := newDirectV7Fixture(t)
	author := addressFromHex(t, hexOf(bytes.Repeat([]byte{0x94}, 20)))
	id, secrets := f.syntheticSeed(t, author, 230, true)
	keys := sha256.Sum256(secrets)
	metadata := join([]byte{0, 230}, []byte{0, 0, 0, 1}, bytes.Repeat([]byte{0x41}, 32), keys[:])
	f.env.SetCaller(author)
	sellerBond := big.NewInt(5_000_000)
	f.env.SetPayAmount(sellerBond)
	f.env.CreditContractBalance(sellerBond)
	f.call(t, "list", join(metadata, u128ArgValue(big.NewInt(0))))
	quote := new(big.Int).Div(new(big.Int).Add(f.reserve, big.NewInt(10)), big.NewInt(11))
	buyerBond := big.NewInt(5_000_000)
	buyer := addressFromHex(t, hexOf(bytes.Repeat([]byte{0xb3}, 20)))
	f.env.SetCaller(buyer)
	payment := new(big.Int).Add(quote, buyerBond)
	f.env.SetPayAmount(payment)
	f.env.CreditContractBalance(payment)
	f.call(t, "buy", join(id[:], DeliveryKeyBytes(NewDeliveryKey(t))))
	f.env.SetBlockNumber(1030)
	f.env.SetCaller(author)
	f.call(t, "startFallback", u64Arg(1))
	f.call(t, "proveAndReveal", join(u64Arg(1), secrets))
	if f.env.BalanceOf(author).Cmp(new(big.Int).Add(quote, sellerBond)) != 0 ||
		f.env.BalanceOf(buyer).Cmp(buyerBond) != 0 {
		t.Fatal("unchallenged public fallback charged the buyer's proof bond")
	}
}
