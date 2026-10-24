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

type bondedV9Fixture struct {
	env  *MinimalEnv
	api  *lib.GoAPI
	code []byte
}

func newBondedV9(t *testing.T) *bondedV9Fixture {
	t.Helper()
	code, err := os.ReadFile("../../contracts/bonded-market-v9/target/bonded-v9.wasm")
	if err != nil {
		t.Fatal(err)
	}
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, "1111111111111111111111111111111111111111"))
	env.SetBlockNumber(1001)
	f := &bondedV9Fixture{env, lib.NewGoAPI(env, &lib.GasMeter{}), code}
	f.call(t, "deploy", nil, big.NewInt(1))
	return f
}
func (f *bondedV9Fixture) call(t *testing.T, method string, arg []byte, amount *big.Int) uint64 {
	t.Helper()
	if arg == nil {
		arg = []byte{9}
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
		t.Fatalf("v9 %s: %v", method, err)
	}
	return gas
}
func (f *bondedV9Fixture) reject(t *testing.T, method string, arg []byte) {
	t.Helper()
	restore := f.env.Snapshot()
	f.env.SetPayAmount(big.NewInt(1))
	f.env.CreditContractBalance(big.NewInt(1))
	_, _, err := lib.Execute(f.api, f.code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	restore()
	f.env.ResetPayAmount()
	if err == nil {
		t.Fatalf("v9 %s accepted invalid call", method)
	}
}
func (f *bondedV9Fixture) get(key []byte) []byte { return f.env.GetStorage(&lib.GasMeter{}, key) }
func (f *bondedV9Fixture) put(key, value []byte) { f.env.SetStorage(&lib.GasMeter{}, key, value) }
func v9key(prefix string, id uint64) []byte      { return join([]byte(prefix), u64Arg(id)) }
func v9DeliveryFixture(t *testing.T) ([]byte, []byte, []byte) {
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
func TestBondedV9CanonicalAtomicListingAndCooperativeSettlement(t *testing.T) {
	f := newBondedV9(t)
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
	j := mustHex(t, proof.Journal)
	sessionInput, session := fastSessionInputFromFile(t, "../../proofs/fast-content/dist/synthetic-session.json")
	f.env.SetBlockHeader(session.Inventory.Height, proposedHeaderWire(t, session.Inventory.Height, mustHex(t, session.Inventory.Root)))
	f.env.SetBlockHeader(session.Verdict.Height, proposedHeaderWire(t, session.Verdict.Height, mustHex(t, session.Verdict.Root)))
	height := session.Verdict.Height + 100
	f.env.SetBlockNumber(height)
	seller := addressFromHex(t, hexOf(j[56:76]))
	buyer := addressFromHex(t, "3333333333333333333333333333333333333333")
	f.env.SetCaller(seller)
	wire := sessionInput[56:]
	cap := mustAmount("500000000000000000000")
	one := big.NewInt(1)
	reg := join(u128ArgValue(one), u128ArgValue(cap), u64Arg(height+1000), j, binary.BigEndian.AppendUint16(nil, uint16(len(wire))), wire)
	f.reject(t, "queueSell", reg)
	if len(f.get([]byte("v9/sells"))) != 0 {
		t.Fatal("unfunded listing changed queue")
	}
	f.call(t, "depositStake", u128ArgValue(cap), cap)
	altered := append([]byte(nil), reg...)
	altered[40+36] ^= 1
	f.reject(t, "queueSell", altered)
	gas := f.call(t, "queueSell", reg, big.NewInt(1))
	t.Logf("queueSell wasm gas %d; user gas %d", gas, ((gas + 99) / 100))
	f.reject(t, "queueSell", reg)
	id := sha256.Sum256(join([]byte("fod:bundle:v9"), seller[:], j[76:78]))
	dj, _, cipher := v9DeliveryFixture(t)
	if len(f.get(join([]byte("v9/bundle/"), id[:]))) != 97 {
		t.Fatal("missing atomic listing")
	}
	f.call(t, "addReserve", nil, mustAmount("1000000000000000000000"))
	f.call(t, "acquire", nil, one)
	f.env.SetCaller(buyer)
	f.call(t, "depositStake", u128ArgValue(cap), cap)
	f.call(t, "queueBuy", join(dj[80:145], u128ArgValue(cap), u64Arg(height+1000)), cap)
	f.call(t, "matchNext", nil, one)
	f.env.SetBlockHeader(height-1, fastSyntheticHeader(bytes.Repeat([]byte{9}, 32)))
	f.call(t, "ready", u64Arg(1), one)
	f.env.SetCaller(seller)
	f.call(t, "deliver", join(u64Arg(1), cipher), one)
	f.reject(t, "deliver", join(u64Arg(1), cipher))
	f.env.SetCaller(buyer)
	hash := sha256.Sum256(cipher)
	f.call(t, "accept", join(u64Arg(1), hash[:]), one)
	trade := f.get(v9key("v9/trade/", 1))
	if len(trade) != 308 || trade[306] != 5 {
		t.Fatal("not settled")
	}
	f.reject(t, "accept", join(u64Arg(1), hash[:]))
	f.reject(t, "expireTrade", u64Arg(1))
	totals := f.get([]byte("v9/totals"))
	if len(totals) != 104 || new(big.Int).SetBytes(totals[16:32]).Sign() != 0 || new(big.Int).SetBytes(totals[48:64]).Sign() != 0 {
		t.Fatal("escrow or entitlement remained")
	}
	if ((gas + 99) / 100) >= maxUserGasUpgrade11 {
		t.Fatal("registration exceeds block")
	}
}

// Both receipts refer to the same synthetic original content and exact buyer
// ciphertext. Every record is created through the deployed ABI, not seeded.
func TestBondedV9GenuineCombinedDispute(t *testing.T) {
	var content struct {
		Journal string `json:"journal"`
		Seal    string `json:"seal"`
	}
	raw, e := os.ReadFile("../../proofs/fast-content/dist/synthetic-v5-one-flip.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &content); e != nil {
		t.Fatal(e)
	}
	var delivery struct {
		Journal    string `json:"journal"`
		Seal       string `json:"seal"`
		Ciphertext string `json:"ciphertext"`
	}
	raw, e = os.ReadFile("../../proofs/delivery/dist/bonded-v9-synthetic.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &delivery); e != nil {
		t.Fatal(e)
	}
	cj, cs := mustHex(t, content.Journal), mustHex(t, content.Seal)
	dj, ds, ct := mustHex(t, delivery.Journal), mustHex(t, delivery.Seal), mustHex(t, delivery.Ciphertext)
	for _, mode := range []string{"challenge", "silence"} {
		t.Run(mode, func(t *testing.T) {
			f := newBondedV9(t)
			input, session := fastSessionInputFromFile(t, "../../proofs/fast-content/dist/synthetic-session.json")
			f.env.SetBlockHeader(session.Inventory.Height, proposedHeaderWire(t, session.Inventory.Height, mustHex(t, session.Inventory.Root)))
			f.env.SetBlockHeader(session.Verdict.Height, proposedHeaderWire(t, session.Verdict.Height, mustHex(t, session.Verdict.Root)))
			height := binary.BigEndian.Uint64(dj[241:249]) + 1
			if height <= session.Verdict.Height {
				t.Fatal("synthetic funding predates validation")
			}
			f.env.SetBlockNumber(height)
			seller, buyer := addressFromHex(t, hexOf(cj[56:76])), addressFromHex(t, hexOf(dj[60:80]))
			one, cap := big.NewInt(1), mustAmount("500000000000000000000")
			f.env.SetCaller(seller)
			f.call(t, "depositStake", u128ArgValue(cap), cap)
			wire := input[56:]
			claim := append([]byte(nil), cj...)
			clear(claim[176:241])
			listing := join(u128ArgValue(one), u128ArgValue(cap), u64Arg(height+1000), claim, binary.BigEndian.AppendUint16(nil, uint16(len(wire))), wire)
			f.call(t, "queueSell", listing, one)
			f.reject(t, "registerBundle", join(cj, cs, wire))
			f.env.SetCaller(buyer)
			f.call(t, "depositStake", u128ArgValue(cap), cap)
			f.call(t, "queueBuy", join(dj[80:145], u128ArgValue(cap), u64Arg(height+1000)), cap)
			f.call(t, "matchNext", nil, one)
			if price := new(big.Int).SetBytes(f.get(v9key("v9/trade/", 1))[153:169]); price.Cmp(mustAmount("500000000000000000000")) != 0 {
				t.Fatal("native v9 initial purchase price is not 500 iDNA")
			}
			f.env.SetBlockHeader(height-1, fastSyntheticHeader(dj[249:281]))
			f.call(t, "ready", u64Arg(1), one)
			f.env.SetCaller(seller)
			f.call(t, "deliver", join(u64Arg(1), ct), one)
			if mode == "challenge" {
				f.env.SetCaller(buyer)
				f.call(t, "challenge", u64Arg(1), one)
			} else {
				tr := f.get(v9key("v9/trade/", 1))
				f.env.SetBlockNumber(binary.BigEndian.Uint64(tr[242:250]))
			}
			f.env.SetCaller(seller)
			arg := join(u64Arg(1), cj, cs, dj, ds)
			wrong := append([]byte(nil), arg...)
			wrong[8+241] ^= 1
			f.reject(t, "proveDelivery", wrong)
			wrong = append([]byte(nil), arg...)
			wrong[8+273+256+177] ^= 1
			f.reject(t, "proveDelivery", wrong)
			f.reject(t, "proveDelivery", join(u64Arg(1), dj, ds))
			gas := f.call(t, "proveDelivery", arg, one)
			if tr := f.get(v9key("v9/trade/", 1)); tr[306] != 5 {
				t.Fatal("combined proof did not settle")
			}
			if total := f.get([]byte("v9/totals")); new(big.Int).SetBytes(total[16:32]).Sign() != 0 {
				t.Fatal("buyer escrow remained")
			}
			if account := f.get(join([]byte("v9/account/"), buyer[:])); account[48] != 1 {
				t.Fatal("buyer charge did not require refill")
			}
			t.Logf("combined dispute user gas %d", (gas+99)/100)
			if (gas+99)/100 >= maxUserGasUpgrade11 {
				t.Fatal("combined proof exceeds native block gas cap")
			}
			f.reject(t, "proveDelivery", arg)
		})
	}
}
