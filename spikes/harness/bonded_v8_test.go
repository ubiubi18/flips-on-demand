package harness

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"github.com/idena-network/idena-wasm-binding/lib"
	"math/big"
	"os"
	"testing"
)

type bondedV8Fixture struct {
	env  *MinimalEnv
	api  *lib.GoAPI
	code []byte
}

func newBondedV8(t *testing.T) *bondedV8Fixture {
	t.Helper()
	code, err := os.ReadFile("../../contracts/bonded-market-v8/target/bonded-v8.wasm")
	if err != nil {
		t.Fatal(err)
	}
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, "1111111111111111111111111111111111111111"))
	env.SetBlockNumber(1001)
	f := &bondedV8Fixture{env, lib.NewGoAPI(env, &lib.GasMeter{}), code}
	f.call(t, "deploy", nil, big.NewInt(1))
	return f
}
func (f *bondedV8Fixture) call(t *testing.T, method string, arg []byte, amount *big.Int) uint64 {
	t.Helper()
	if arg == nil {
		arg = []byte{8}
	}
	f.env.SetPayAmount(amount)
	f.env.CreditContractBalance(amount)
	var gas uint64
	var err error
	if method == "deploy" {
		gas, _, err = lib.Deploy(f.api, f.code, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	} else {
		gas, _, err = lib.Execute(f.api, f.code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	}
	f.env.ResetPayAmount()
	if err != nil {
		t.Fatalf("v8 %s: %v", method, err)
	}
	return gas
}
func (f *bondedV8Fixture) reject(t *testing.T, method string, arg []byte) {
	t.Helper()
	restore := f.env.Snapshot()
	f.env.SetPayAmount(big.NewInt(1))
	f.env.CreditContractBalance(big.NewInt(1))
	_, _, err := lib.Execute(f.api, f.code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	restore()
	f.env.ResetPayAmount()
	if err == nil {
		t.Fatalf("v8 %s accepted invalid call", method)
	}
}
func (f *bondedV8Fixture) get(key []byte) []byte { return f.env.GetStorage(&lib.GasMeter{}, key) }
func (f *bondedV8Fixture) put(key, value []byte) { f.env.SetStorage(&lib.GasMeter{}, key, value) }
func v8key(prefix string, id uint64) []byte      { return join([]byte(prefix), u64Arg(id)) }
func v8DeliveryFixture(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	var p struct {
		Journal    string `json:"journal"`
		Seal       string `json:"seal"`
		Ciphertext string `json:"ciphertext"`
	}
	raw, e := os.ReadFile("../../proofs/delivery/dist/delivery-v2.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &p); e != nil {
		t.Fatal(e)
	}
	return mustHex(t, p.Journal), mustHex(t, p.Seal), mustHex(t, p.Ciphertext)
}
func (f *bondedV8Fixture) match(t *testing.T, id [32]byte, seller, buyer lib.Address, buyerKey []byte, height uint64, pool bool) {
	t.Helper()
	cap := mustAmount("500000000000000000000")
	one := big.NewInt(1)
	f.env.SetBlockNumber(height)
	f.env.SetCaller(seller)
	f.call(t, "depositStake", u128ArgValue(cap), cap)
	f.call(t, "queueSell", join(id[:], u128ArgValue(one), u128ArgValue(cap), u64Arg(height+1000)), one)
	if pool {
		f.call(t, "addReserve", nil, mustAmount("1000000000000000000000"))
		f.call(t, "acquire", nil, one)
	}
	f.env.SetCaller(buyer)
	f.call(t, "depositStake", u128ArgValue(cap), cap)
	f.call(t, "queueBuy", join(buyerKey, u128ArgValue(cap), u64Arg(height+1000)), cap)
	f.call(t, "matchNext", nil, one)
}
func TestBondedV8CanonicalRegistrationAndCooperativeSettlement(t *testing.T) {
	f := newBondedV8(t)
	var proof struct {
		Journal string `json:"journal"`
		Seal    string `json:"seal"`
	}
	raw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-v5-one-flip.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	j, seal := mustHex(t, proof.Journal), mustHex(t, proof.Seal)
	sessionInput, session := fastSessionInputFromFile(t, "../../proofs/fast-content/dist/synthetic-session.json")
	f.env.SetBlockHeader(session.Inventory.Height, proposedHeaderWire(t, session.Inventory.Height, mustHex(t, session.Inventory.Root)))
	f.env.SetBlockHeader(session.Verdict.Height, proposedHeaderWire(t, session.Verdict.Height, mustHex(t, session.Verdict.Root)))
	height := session.Verdict.Height + 100
	f.env.SetBlockNumber(height)
	seller := addressFromHex(t, hexOf(j[56:76]))
	buyer := addressFromHex(t, "3333333333333333333333333333333333333333")
	f.env.SetCaller(seller)
	wire := sessionInput[56:]
	reg := join(j, seal, binary.BigEndian.AppendUint16(nil, uint16(len(wire))), wire)
	altered := append([]byte(nil), reg...)
	altered[36] ^= 1
	f.reject(t, "registerBundle", altered)
	gas := f.call(t, "registerBundle", reg, big.NewInt(1))
	t.Logf("registerBundle wasm gas %d; user gas %d", gas, ((gas + 99) / 100))
	f.reject(t, "registerBundle", reg)
	id := sha256.Sum256(join([]byte("fod:bundle:v8"), seller[:], j[76:78]))
	one := big.NewInt(1)
	dj, _, cipher := v8DeliveryFixture(t)
	f.match(t, id, seller, buyer, dj[80:145], height, true)
	f.env.SetBlockHeader(height-1, fastSyntheticHeader(bytes.Repeat([]byte{8}, 32)))
	f.call(t, "ready", u64Arg(1), one)
	f.env.SetCaller(seller)
	f.call(t, "deliver", join(u64Arg(1), cipher), one)
	f.reject(t, "deliver", join(u64Arg(1), cipher))
	f.env.SetCaller(buyer)
	hash := sha256.Sum256(cipher)
	f.call(t, "accept", join(u64Arg(1), hash[:]), one)
	trade := f.get(v8key("v8/trade/", 1))
	if len(trade) != 308 || trade[306] != 5 {
		t.Fatal("not settled")
	}
	f.reject(t, "accept", join(u64Arg(1), hash[:]))
	f.reject(t, "expireTrade", u64Arg(1))
	totals := f.get([]byte("v8/totals"))
	if len(totals) != 104 || new(big.Int).SetBytes(totals[16:32]).Sign() != 0 || new(big.Int).SetBytes(totals[48:64]).Sign() != 0 {
		t.Fatal("escrow or entitlement remained")
	}
	if ((gas + 99) / 100) >= maxUserGasUpgrade11 {
		t.Fatal("registration exceeds block")
	}
}
func TestBondedV8PrivateProofNativeBindingAndSettlement(t *testing.T) {
	f := newBondedV8(t)
	j, seal, cipher := v8DeliveryFixture(t)
	// Synthetic pre-proved content only; production registration is tested above.
	seller, buyer := addressFromHex(t, hexOf(j[40:60])), addressFromHex(t, hexOf(j[60:80]))
	id := sha256.Sum256(join([]byte("fod:bundle:v8"), seller[:], []byte{0, 232}))
	bundle := join(seller[:], []byte{0, 232, 0, 5}, bytes.Repeat([]byte{4}, 32), j[145:177], u64Arg(0), []byte{0})
	f.put(join([]byte("v8/bundle/"), id[:]), bundle)
	totals := f.get([]byte("v8/totals"))
	binary.BigEndian.PutUint64(totals[96:104], 7)
	f.put([]byte("v8/totals"), totals)
	f.match(t, id, seller, buyer, j[80:145], 1001, false)
	one := big.NewInt(1)
	f.env.SetBlockHeader(1000, fastSyntheticHeader(j[249:281]))
	f.call(t, "ready", u64Arg(7), one)
	f.env.SetCaller(seller)
	f.call(t, "deliver", join(u64Arg(7), cipher), one)
	f.env.SetCaller(buyer)
	f.call(t, "challenge", u64Arg(7), one)
	f.env.SetCaller(seller)
	altered := append([]byte(nil), j...)
	altered[177] ^= 1
	f.reject(t, "proveDelivery", join(u64Arg(7), altered, seal))
	gas := f.call(t, "proveDelivery", join(u64Arg(7), j, seal), one)
	if trade := f.get(v8key("v8/trade/", 7)); trade[306] != 5 {
		t.Fatal("private proof did not settle")
	}
	if account := f.get(join([]byte("v8/account/"), buyer[:])); account[48] != 1 {
		t.Fatal("buyer top-up was not required")
	}
	t.Logf("proveDelivery wasm gas %d; user gas %d", gas, ((gas + 99) / 100))
	if ((gas + 99) / 100) >= maxUserGasUpgrade11 {
		t.Fatal("private proof exceeds block")
	}
	f.reject(t, "proveDelivery", join(u64Arg(7), j, seal))
	f.reject(t, "allocate", nil)
}
