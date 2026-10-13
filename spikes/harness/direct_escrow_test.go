package harness

// Direct-sale paths against the pinned Idena WASM runtime. Cooperative paths
// use an older receipt only as public journal data; the fallback test requires
// a genuine seal for the direct-v2 guest and placeholder contract.

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

const directWindowBlocks = 8640
const directConfirmGraceBlocks = 30
const directProofMarginBlocks = 120

func directWasmPath() string {
	if staged := os.Getenv("FOD_DIRECT_PRODUCTION_WASM"); staged != "" {
		return staged
	}
	return "../../release/artifacts/fod_direct_escrow_v4.wasm"
}

type directFixture struct {
	env     *MinimalEnv
	api     *lib.GoAPI
	code    []byte
	receipt sealedRegistrationV2
	author  lib.Address
	buyer   lib.Address
	offerID [32]byte
	price   *big.Int
	bond    *big.Int
}

func directCall(t *testing.T, f *directFixture, method string, arg []byte) uint64 {
	t.Helper()
	defer f.env.ResetPayAmount()
	gas, _, err := lib.Execute(f.api, f.code, method, [][]byte{arg}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatalf("%s rejected by runtime: %v", method, err)
	}
	return gas
}

func newDirectFixture(t *testing.T) *directFixture {
	return newDirectFixtureFrom(t, registrationV2Artefact)
}

func newDirectFallbackFixture(t *testing.T) *directFixture {
	return newDirectFixtureFrom(t, "../../proofs/registration-v2/dist/direct-v2-synthetic.json")
}

func newDirectFixtureFrom(t *testing.T, receiptPath string) *directFixture {
	t.Helper()
	code, err := os.ReadFile(directWasmPath())
	if err != nil {
		t.Fatalf("read direct contract: %v", err)
	}
	receipt := loadRegistrationV2File(t, receiptPath)
	env := NewMinimalEnv()
	env.SetContractAddress(addressFromHex(t, hexOf(receipt.journal[4:24])))
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	deployEscrow(t, env, api, code)
	env.SetBlockNumber(1000)
	receipt.bindSessionAnchors(t, env)
	author := addressFromHex(t, hexOf(receipt.author()))
	buyer := addressFromHex(t, "b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2")
	epoch := receipt.journal[44:46]
	h := sha256.New()
	h.Write([]byte("fod:direct-offer:v2"))
	h.Write(author[:])
	h.Write(epoch)
	var id [32]byte
	copy(id[:], h.Sum(nil))
	price := new(big.Int).Set(testPriceValue)
	bond := new(big.Int).Mul(env.feePerGas, big.NewInt(2_500_000*2))
	f := &directFixture{env: env, api: api, code: code, receipt: receipt, author: author, buyer: buyer, offerID: id, price: price, bond: bond}
	f.list(t)
	return f
}

func (f *directFixture) list(t *testing.T) {
	t.Helper()
	f.env.SetCaller(f.author)
	arg := join(f.receipt.journal[44:46], f.receipt.journal[46:50], f.receipt.inventoryCommitment(), f.receipt.keyCommitment(), u128ArgValue(f.price))
	directCall(t, f, "list", arg)
	if got := f.env.GetStorage(&lib.GasMeter{}, join([]byte("direct/offer/"), f.offerID[:])); len(got) != 123 {
		t.Fatalf("stored offer has %d bytes", len(got))
	}
}

func (f *directFixture) buy(t *testing.T) {
	t.Helper()
	f.env.SetCaller(f.buyer)
	total := new(big.Int).Add(f.price, f.bond)
	f.env.SetPayAmount(total)
	f.env.CreditContractBalance(total)
	key := DeliveryKeyBytes(NewDeliveryKey(t))
	if len(key) != 65 {
		t.Fatalf("delivery key has %d bytes", len(key))
	}
	directCall(t, f, "buy", join(f.offerID[:], key))
	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1")); len(got) != 198 {
		t.Fatalf("stored order has %d bytes", len(got))
	}
}

func (f *directFixture) startArg() []byte {
	return join(u64Arg(1), f.receipt.journal, f.receipt.seal)
}

func (f *directFixture) start(t *testing.T, stake *big.Int) uint64 {
	t.Helper()
	f.env.SetBlockNumber(1000 + directConfirmGraceBlocks)
	f.env.SetCaller(f.author)
	f.env.SetPayAmount(stake)
	f.env.CreditContractBalance(stake)
	return directCall(t, f, "startFallback", f.startArg())
}

func (f *directFixture) pending() *big.Int {
	return new(big.Int).SetBytes(f.env.GetStorage(&lib.GasMeter{}, []byte("direct/pending")))
}

func (f *directFixture) secrets(t *testing.T) []byte {
	t.Helper()
	var vector struct {
		PublicPartSecret  string `json:"publicPartSecret"`
		PrivatePartSecret string `json:"privatePartSecret"`
	}
	raw, err := os.ReadFile("../../proofs/registration/vectors/blob-flip-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatal(err)
	}
	// The blob vector supplies the two synthetic scalars; the registration
	// fixture commits to them under its own author and epoch.
	secrets := join([]byte("fod:epoch-secrets:v1"), f.receipt.journal[44:46], f.receipt.author(), mustHex(t, vector.PublicPartSecret), mustHex(t, vector.PrivatePartSecret))
	if len(secrets) != 106 || sha256.Sum256(secrets) != [32]byte(f.receipt.keyCommitment()) {
		t.Fatal("synthetic fixture commitment differs from receipt")
	}
	return secrets
}

func TestDirectBuyerConfirmPaysImmediatelyWithoutProof(t *testing.T) {
	f := newDirectFixture(t)
	f.buy(t)
	f.env.SetCaller(f.buyer)
	checksum := sha256.Sum256([]byte("private-envelope"))
	gas := directCall(t, f, "confirm", join(u64Arg(1), checksum[:]))
	if f.env.BalanceOf(f.author).Cmp(f.price) != 0 {
		t.Fatal("buyer confirmation did not pay the author")
	}
	if f.env.BalanceOf(f.buyer).Cmp(f.bond) != 0 {
		t.Fatal("buyer confirmation did not return the proof bond")
	}
	order := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1"))
	if order[165] != 2 || !bytes.Equal(order[166:198], checksum[:]) {
		t.Fatal("confirmed order lacks exact ciphertext commitment")
	}
	if pending := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/pending")); !bytes.Equal(pending, make([]byte, 16)) {
		t.Fatal("settled order remains a pending liability")
	}
	escrowReject(t, f.env, f.api, f.code, "confirm", join(u64Arg(1), checksum[:]))
	f.env.SetBlockNumber(1000 + directWindowBlocks)
	escrowReject(t, f.env, f.api, f.code, "refund", u64Arg(1))
	t.Logf("direct confirm wasm gas=%d", gas)
}

func TestDirectBuyerCanConfirmAtRefundBoundaryUntilRefundLands(t *testing.T) {
	f := newDirectFixture(t)
	f.buy(t)
	f.env.SetBlockNumber(1000 + directWindowBlocks)
	f.env.SetCaller(f.buyer)
	checksum := sha256.Sum256([]byte("verified late private delivery"))
	directCall(t, f, "confirm", join(u64Arg(1), checksum[:]))
	if f.env.BalanceOf(f.author).Cmp(f.price) != 0 || f.env.BalanceOf(f.buyer).Cmp(f.bond) != 0 {
		t.Fatal("late buyer confirmation did not settle funded order")
	}
	escrowReject(t, f.env, f.api, f.code, "refund", u64Arg(1))
}

func TestDirectProofFallbackRequiresGenuineContentReceipt(t *testing.T) {
	f := newDirectFallbackFixture(t)
	f.buy(t)
	secrets := f.secrets(t)
	startArg := f.startArg()
	revealArg := join(u64Arg(1), secrets)
	if len(startArg) != 8+326+256 || len(revealArg) != 8+106 {
		t.Fatal("fallback calls have unexpected argument lengths")
	}
	f.env.SetCaller(f.author)
	escrowReject(t, f.env, f.api, f.code, "proveAndReveal", revealArg)
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", startArg, f.price)
	f.env.feePerGas = big.NewInt(2)
	startGas := f.start(t, f.price)
	if stake := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/stake/1")); len(stake) != 16 || new(big.Int).SetBytes(stake).Cmp(f.price) != 0 {
		t.Fatal("proof-backed start did not retain the exact seller stake")
	}
	if want := new(big.Int).Add(new(big.Int).Add(f.price, f.bond), f.price); f.pending().Cmp(want) != 0 {
		t.Fatal("seller stake is not included in pending liabilities")
	}
	if rate := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/proof-rate/1")); len(rate) != 16 || new(big.Int).SetBytes(rate).Cmp(big.NewInt(2)) != 0 {
		t.Fatal("proof-start fee rate was not recorded")
	}
	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/reveal/1")); len(got) != 0 {
		t.Fatal("proof-backed start exposed public keys")
	}
	if f.env.BalanceOf(f.author).Sign() != 0 || f.env.BalanceOf(f.buyer).Sign() != 0 {
		t.Fatal("proof-backed start settled before key reveal")
	}
	f.env.SetCaller(f.buyer)
	checksum := sha256.Sum256([]byte("late private-envelope"))
	escrowReject(t, f.env, f.api, f.code, "confirm", join(u64Arg(1), checksum[:]))
	f.env.SetCaller(f.author)
	bad := append([]byte(nil), revealArg...)
	bad[8+10] ^= 1
	escrowReject(t, f.env, f.api, f.code, "proveAndReveal", bad)
	f.env.SetCaller(f.buyer)
	escrowReject(t, f.env, f.api, f.code, "proveAndReveal", revealArg)
	f.env.SetCaller(f.author)
	f.env.feePerGas = big.NewInt(9)
	revealGas := directCall(t, f, "proveAndReveal", revealArg)
	reimbursement := big.NewInt(4_000_000)
	if reimbursement.Cmp(f.bond) > 0 {
		reimbursement.Set(f.bond)
	}
	if f.env.BalanceOf(f.author).Cmp(new(big.Int).Add(new(big.Int).Add(f.price, f.price), reimbursement)) != 0 {
		t.Fatal("sealed fallback did not return seller stake, price, and proof-start reimbursement")
	}
	if f.env.BalanceOf(f.buyer).Cmp(new(big.Int).Sub(f.bond, reimbursement)) != 0 {
		t.Fatal("sealed fallback did not return the unused buyer bond")
	}
	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/reveal/1")); !bytes.Equal(got, secrets) {
		t.Fatal("public recovery keys missing after settlement")
	}
	if offer := f.env.GetStorage(&lib.GasMeter{}, join([]byte("direct/offer/"), f.offerID[:])); offer[122] != 1 {
		t.Fatal("revealed offer can be bought again")
	}
	if f.pending().Sign() != 0 || f.env.ContractBalance().Sign() != 0 {
		t.Fatal("reveal left pending liability or contract balance")
	}
	escrowReject(t, f.env, f.api, f.code, "refund", u64Arg(1))
	t.Logf("direct sealed fallback start wasm gas=%d", startGas)
	t.Logf("direct key reveal wasm gas=%d", revealGas)
}

func TestDirectFallbackStartRejectsInvalidEvidence(t *testing.T) {
	f := newDirectFallbackFixture(t)
	f.buy(t)
	arg := f.startArg()
	f.env.SetBlockNumber(1000 + directConfirmGraceBlocks)
	f.env.SetCaller(f.author)
	escrowRejectPayable(t, f.env, f.api, f.code, "reserveFallback", u64Arg(1), f.price)
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", u64Arg(1), f.price)
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", append(append([]byte(nil), arg...), make([]byte, 106)...), f.price)
	f.env.SetCaller(f.buyer)
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", arg, f.price)
	f.env.SetCaller(f.author)
	for _, offset := range []int{8 + 4, 8 + registrationV2InventoryOffset, 8 + registrationV2JournalBytes + 10} {
		bad := append([]byte(nil), arg...)
		bad[offset] ^= 1
		escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", bad, f.price)
	}
	inventoryHeight := binary.BigEndian.Uint64(f.receipt.journal[registrationV2InventoryHeight : registrationV2InventoryHeight+8])
	f.env.SetBlockHeader(inventoryHeight, proposedHeaderWire(t, inventoryHeight, bytes.Repeat([]byte{9}, 32)))
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", arg, f.price)
	f.receipt.bindSessionAnchors(t, f.env)
	if order := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1")); order[165] != 1 {
		t.Fatal("invalid evidence changed the funded order")
	}
	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/stake/1")); len(got) != 0 {
		t.Fatal("invalid evidence retained a seller stake")
	}
	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/proof-rate/1")); len(got) != 0 {
		t.Fatal("invalid evidence recorded a proof fee rate")
	}
	if want := new(big.Int).Add(f.price, f.bond); f.pending().Cmp(want) != 0 {
		t.Fatal("invalid evidence changed pending liabilities")
	}
	if got := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/reveal/1")); len(got) != 0 {
		t.Fatal("invalid evidence exposed public keys")
	}
	f.env.SetCaller(f.buyer)
	checksum := sha256.Sum256([]byte("private delivery after rejected proof"))
	directCall(t, f, "confirm", join(u64Arg(1), checksum[:]))
	oldImage := newDirectFixture(t)
	oldImage.buy(t)
	oldImage.env.SetBlockNumber(1000 + directConfirmGraceBlocks)
	oldImage.env.SetCaller(oldImage.author)
	escrowRejectPayable(t, oldImage.env, oldImage.api, oldImage.code, "startFallback", oldImage.startArg(), oldImage.price)
}

func TestDirectFallbackStartAndBuyerConfirmAreFirstIncludedWins(t *testing.T) {
	confirmed := newDirectFixture(t)
	confirmed.buy(t)
	confirmed.env.SetBlockNumber(1000 + directConfirmGraceBlocks)
	confirmed.env.SetCaller(confirmed.buyer)
	checksum := sha256.Sum256([]byte("private-envelope"))
	directCall(t, confirmed, "confirm", join(u64Arg(1), checksum[:]))
	confirmed.env.SetCaller(confirmed.author)
	escrowRejectPayable(t, confirmed.env, confirmed.api, confirmed.code, "startFallback", confirmed.startArg(), confirmed.price)
	if got := confirmed.env.GetStorage(&lib.GasMeter{}, []byte("direct/reveal/1")); len(got) != 0 {
		t.Fatal("confirmation-first race exposed public keys")
	}

	pending := newDirectFallbackFixture(t)
	pending.buy(t)
	pending.env.SetCaller(pending.buyer)
	escrowRejectPayable(t, pending.env, pending.api, pending.code, "startFallback", pending.startArg(), pending.price)
	pending.env.SetCaller(pending.author)
	pending.start(t, pending.price)
	escrowRejectPayable(t, pending.env, pending.api, pending.code, "startFallback", pending.startArg(), pending.price)
	if order := pending.env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1")); order[165] != 4 {
		t.Fatal("secret-free fallback start did not lock the funded order")
	}
	pending.env.SetCaller(pending.buyer)
	escrowReject(t, pending.env, pending.api, pending.code, "confirm", join(u64Arg(1), checksum[:]))
	pending.env.SetBlockNumber(1000 + directWindowBlocks)
	pending.env.SetCaller(pending.author)
	escrowReject(t, pending.env, pending.api, pending.code, "proveAndReveal", join(u64Arg(1), pending.secrets(t)))
	directCall(t, pending, "refund", u64Arg(1))
	if pending.env.BalanceOf(pending.buyer).Cmp(new(big.Int).Add(new(big.Int).Add(pending.price, pending.bond), pending.price)) != 0 {
		t.Fatal("verified fallback without reveal did not slash seller stake")
	}
	if pending.pending().Sign() != 0 {
		t.Fatal("verified refund left pending liability")
	}
	if pending.env.ContractBalance().Sign() != 0 {
		t.Fatal("verified refund left contract funds")
	}
}

func TestDirectFallbackStartChecksCallerGraceStakeAndDeadline(t *testing.T) {
	f := newDirectFallbackFixture(t)
	f.buy(t)
	f.env.SetCaller(f.author)
	f.env.SetBlockNumber(1000 + directConfirmGraceBlocks - 1)
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", f.startArg(), f.price)
	f.env.SetBlockNumber(1000 + directConfirmGraceBlocks)
	f.env.SetCaller(f.buyer)
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", f.startArg(), f.price)
	f.env.SetCaller(f.author)
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", f.startArg(), new(big.Int).Sub(f.price, big.NewInt(1)))
	f.env.feePerGas = big.NewInt(2_000_000_000_000_000)
	minimum := new(big.Int).Mul(f.env.feePerGas, big.NewInt(100_000))
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", f.startArg(), new(big.Int).Sub(minimum, big.NewInt(1)))
	f.env.SetPayAmount(minimum)
	f.env.CreditContractBalance(minimum)
	directCall(t, f, "startFallback", f.startArg())
	if got := f.pending(); got.Cmp(new(big.Int).Add(new(big.Int).Add(f.price, f.bond), minimum)) != 0 {
		t.Fatal("actual seller stake was not added to pending")
	}
	if got := new(big.Int).SetBytes(f.env.GetStorage(&lib.GasMeter{}, []byte("direct/stake/1"))); got.Cmp(minimum) != 0 {
		t.Fatal("actual seller stake was not stored")
	}
	f.env.SetCaller(f.buyer)
	checksum := sha256.Sum256([]byte("buyer attempted to front-run proof"))
	escrowReject(t, f.env, f.api, f.code, "confirm", join(u64Arg(1), checksum[:]))
	f.env.SetCaller(f.author)
	escrowRejectPayable(t, f.env, f.api, f.code, "startFallback", f.startArg(), minimum)

	late := newDirectFallbackFixture(t)
	late.buy(t)
	late.env.SetBlockNumber(1000 + directWindowBlocks - directProofMarginBlocks)
	late.env.SetCaller(late.author)
	escrowRejectPayable(t, late.env, late.api, late.code, "startFallback", late.startArg(), late.price)
	late.env.SetBlockNumber(1000 + directWindowBlocks - directProofMarginBlocks - 1)
	late.env.SetPayAmount(late.price)
	late.env.CreditContractBalance(late.price)
	directCall(t, late, "startFallback", late.startArg())
}

func TestDirectMalformedStakeRecordIsRefused(t *testing.T) {
	f := newDirectFallbackFixture(t)
	f.buy(t)
	f.start(t, f.price)
	f.env.SetStorage(&lib.GasMeter{}, []byte("direct/stake/1"), []byte{1})
	escrowReject(t, f.env, f.api, f.code, "startFallback", f.startArg())
	escrowReject(t, f.env, f.api, f.code, "proveAndReveal", join(u64Arg(1), f.secrets(t)))
	f.env.SetBlockNumber(1000 + directWindowBlocks)
	escrowReject(t, f.env, f.api, f.code, "refund", u64Arg(1))
}

func TestDirectRefundAtExactBoundaryRetiresOffer(t *testing.T) {
	f := newDirectFixture(t)
	f.buy(t)
	f.env.SetBlockNumber(1000 + directWindowBlocks - 1)
	escrowReject(t, f.env, f.api, f.code, "refund", u64Arg(1))
	f.env.SetBlockNumber(1000 + directWindowBlocks)
	f.env.SetCaller(f.author)
	directCall(t, f, "refund", u64Arg(1))
	if f.env.BalanceOf(f.buyer).Cmp(new(big.Int).Add(f.price, f.bond)) != 0 {
		t.Fatal("expired escrow and bond were not repaid to original buyer")
	}
	if offer := f.env.GetStorage(&lib.GasMeter{}, join([]byte("direct/offer/"), f.offerID[:])); offer[122] != 1 {
		t.Fatal("refunded offer must retire")
	}
	if order := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1")); order[165] != 3 {
		t.Fatal("order not marked refunded")
	}
	key := DeliveryKeyBytes(NewDeliveryKey(t))
	f.env.SetCaller(f.buyer)
	f.env.SetPayAmount(new(big.Int).Add(f.price, f.bond))
	escrowReject(t, f.env, f.api, f.code, "buy", join(f.offerID[:], key))
	escrowReject(t, f.env, f.api, f.code, "confirm", join(u64Arg(1), bytes.Repeat([]byte{1}, 32)))
}

func TestDirectBuyRequiresCurrentFeeBondAndStoresIt(t *testing.T) {
	f := newDirectFixture(t)
	f.env.feePerGas = big.NewInt(7)
	minimumBond := big.NewInt(7 * 2_500_000 * 2)
	wantBond := new(big.Int).Add(minimumBond, big.NewInt(1_000_000))
	key := DeliveryKeyBytes(NewDeliveryKey(t))
	f.env.SetCaller(f.buyer)
	f.env.SetPayAmount(new(big.Int).Add(f.price, f.bond))
	escrowReject(t, f.env, f.api, f.code, "buy", join(f.offerID[:], key))
	f.env.SetPayAmount(new(big.Int).Sub(new(big.Int).Add(f.price, minimumBond), big.NewInt(1)))
	escrowReject(t, f.env, f.api, f.code, "buy", join(f.offerID[:], key))
	f.env.SetPayAmount(new(big.Int).Add(f.price, wantBond))
	f.env.CreditContractBalance(new(big.Int).Add(f.price, wantBond))
	directCall(t, f, "buy", join(f.offerID[:], key))
	order := f.env.GetStorage(&lib.GasMeter{}, []byte("direct/order/1"))
	if len(order) != 198 || new(big.Int).SetBytes(order[133:149]).Cmp(wantBond) != 0 {
		t.Fatal("order did not retain the fee-rate-derived proof bond")
	}
	f.env.SetCaller(f.buyer)
	checksum := sha256.Sum256([]byte("verified private delivery"))
	directCall(t, f, "confirm", join(u64Arg(1), checksum[:]))
	if f.env.BalanceOf(f.buyer).Cmp(wantBond) != 0 || f.env.BalanceOf(f.author).Cmp(f.price) != 0 {
		t.Fatal("confirmation did not return the actual overfunded bond")
	}
}

func TestDirectDeployFitsBlockSizeBudget(t *testing.T) {
	code, err := os.ReadFile(directWasmPath())
	if err != nil {
		t.Fatal(err)
	}
	// A signed transaction adds an RLP envelope; reserve 2 KiB of size margin.
	if (len(code)+2048)*10 >= maxUserGasUpgrade11 {
		t.Fatalf("direct module is too large for a signed deploy")
	}
	env := NewMinimalEnv()
	api := lib.NewGoAPI(env, &lib.GasMeter{})
	gas, _, err := lib.Deploy(api, code, [][]byte{[]byte("1")}, lib.Address{}, maxCallGasWasm, false)
	if err != nil {
		t.Fatal(err)
	}
	userGas := gas / wasmGasPerUserGas
	if userGas+uint64((len(code)+2048)*10) >= maxUserGasUpgrade11 {
		t.Fatalf("direct deploy needs %d gas plus size cost, exceeding block limit", userGas)
	}
	t.Logf("direct deploy wasm gas=%d bytes=%d", gas, len(code))
}
