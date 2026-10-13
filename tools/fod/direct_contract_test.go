package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"testing"
)

func TestDirectOfferIDAndStorageKeys(t *testing.T) {
	var author [20]byte
	for i := range author {
		author[i] = byte(i)
	}
	id := DirectOfferID(author, 0x1234)
	const expected = "defef78b70820d900c36a0844c67122f44e6b457f860500ae243e9fba8f55ab2"
	if hex.EncodeToString(id[:]) != expected {
		t.Fatalf("offer ID = %x, want %s", id, expected)
	}
	key := DirectOfferStorageKey(id)
	if !bytes.Equal(key[:len("direct/offer/")], []byte("direct/offer/")) || !bytes.Equal(key[len("direct/offer/"):], id[:]) || len(key) != len("direct/offer/")+32 {
		t.Fatalf("offer key does not contain the raw 32-byte ID: %x", key)
	}
	for _, orderID := range []uint64{0, 9, math.MaxUint64} {
		digits := strconv.FormatUint(orderID, 10)
		if got := string(DirectOrderStorageKey(orderID)); got != "direct/order/"+digits {
			t.Fatalf("order key %q", got)
		}
		if got := string(DirectRevealStorageKey(orderID)); got != "direct/reveal/"+digits {
			t.Fatalf("reveal key %q", got)
		}
		if got := string(DirectIntentStorageKey(orderID)); got != "direct/intent/"+digits {
			t.Fatalf("intent key %q", got)
		}
		if got := string(DirectStakeStorageKey(orderID)); got != "direct/stake/"+digits {
			t.Fatalf("stake key %q", got)
		}
		if got := string(DirectProofRateStorageKey(orderID)); got != "direct/proof-rate/"+digits {
			t.Fatalf("proof rate key %q", got)
		}
	}
}

func TestDirectOfferRecordExactLayout(t *testing.T) {
	maxAmount := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	offer := DirectOffer{
		Author: [20]byte{0: 0xa1, 19: 0xb2}, Epoch: 0x1234, Covered: 32,
		Inventory: [32]byte{0: 0xc3, 31: 0xd4}, Keys: [32]byte{0: 0xe5, 31: 0xf6},
		Price: maxAmount, NextNonce: 0x0102030405060708, OpenOrderID: 0x1112131415161718, Retired: true,
	}
	raw, err := EncodeDirectOffer(offer)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 123 || raw[0] != 0xa1 || raw[19] != 0xb2 || !bytes.Equal(raw[20:22], []byte{0x12, 0x34}) ||
		!bytes.Equal(raw[22:26], []byte{0, 0, 0, 32}) || raw[26] != 0xc3 || raw[57] != 0xd4 ||
		raw[58] != 0xe5 || raw[89] != 0xf6 || !bytes.Equal(raw[90:106], bytes.Repeat([]byte{0xff}, 16)) ||
		binary.BigEndian.Uint64(raw[106:114]) != offer.NextNonce || binary.BigEndian.Uint64(raw[114:122]) != offer.OpenOrderID || raw[122] != 1 {
		t.Fatalf("wrong direct offer offsets: %x", raw)
	}
	decoded, err := DecodeDirectOffer(raw)
	if err != nil || !reflect.DeepEqual(*decoded, offer) {
		t.Fatalf("offer round trip = %+v, %v", decoded, err)
	}
	for _, invalid := range [][]byte{raw[:122], append(bytes.Clone(raw), 0), func() []byte {
		copy := bytes.Clone(raw)
		copy[122] = 2
		return copy
	}()} {
		if _, err := DecodeDirectOffer(invalid); err == nil {
			t.Fatalf("accepted invalid offer record of length %d", len(invalid))
		}
	}
	for _, invalid := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 128)} {
		offer.Price = invalid
		if _, err := EncodeDirectOffer(offer); err == nil {
			t.Fatalf("accepted invalid offer price %v", invalid)
		}
	}
}

func TestDirectOrderRecordExactLayout(t *testing.T) {
	order := DirectOrder{
		OfferID: [32]byte{0: 0x11, 31: 0x22}, Buyer: [20]byte{0: 0x33, 19: 0x44},
		DeliveryKey: [65]byte{0: 4, 1: 0x55, 64: 0x66}, Paid: big.NewInt(123456789), Bond: big.NewInt(5_000_000),
		FundedAt: 0x0102030405060708, Nonce: 0x1112131415161718, State: DirectFunded,
		DeliveryHash: [32]byte{0: 0x77, 31: 0x88},
	}
	raw, err := EncodeDirectOrder(order)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 198 || raw[0] != 0x11 || raw[31] != 0x22 || raw[32] != 0x33 || raw[51] != 0x44 ||
		raw[52] != 4 || raw[53] != 0x55 || raw[116] != 0x66 || new(big.Int).SetBytes(raw[117:133]).Cmp(order.Paid) != 0 ||
		new(big.Int).SetBytes(raw[133:149]).Cmp(order.Bond) != 0 ||
		binary.BigEndian.Uint64(raw[149:157]) != order.FundedAt || binary.BigEndian.Uint64(raw[157:165]) != order.Nonce ||
		raw[165] != 1 || raw[166] != 0x77 || raw[197] != 0x88 {
		t.Fatalf("wrong direct order offsets: %x", raw)
	}
	decoded, err := DecodeDirectOrder(raw)
	if err != nil || !reflect.DeepEqual(*decoded, order) {
		t.Fatalf("order round trip = %+v, %v", decoded, err)
	}
	for _, invalid := range [][]byte{raw[:197], append(bytes.Clone(raw), 0), func() []byte {
		copy := bytes.Clone(raw)
		copy[165] = 6
		return copy
	}()} {
		if _, err := DecodeDirectOrder(invalid); err == nil {
			t.Fatalf("accepted invalid order record of length %d", len(invalid))
		}
	}
	order.State = DirectFallbackPending
	raw, err = EncodeDirectOrder(order)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = DecodeDirectOrder(raw)
	if err != nil || decoded.State != DirectFallbackPending {
		t.Fatalf("fallback-pending order round trip = %+v, %v", decoded, err)
	}
	order.State = DirectFallbackIntent
	raw, err = EncodeDirectOrder(order)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = DecodeDirectOrder(raw)
	if err != nil || decoded.State != DirectFallbackIntent {
		t.Fatalf("fallback-intent order round trip = %+v, %v", decoded, err)
	}
	order.State = 0
	if _, err := EncodeDirectOrder(order); err == nil {
		t.Fatal("encoded an invalid order state")
	}
	order.State = DirectFunded
	order.Bond = nil
	if _, err := EncodeDirectOrder(order); err == nil {
		t.Fatal("encoded an order without a bond field")
	}
}

func TestDirectBondAndFundedAmountChecked(t *testing.T) {
	bond, err := directBondForFee(big.NewInt(3))
	if err != nil || bond.Cmp(big.NewInt(15_000_000)) != 0 {
		t.Fatalf("bond = %v, %v", bond, err)
	}
	quoted, err := directQuotedBondForFee(big.NewInt(3))
	if err != nil || quoted.Cmp(big.NewInt(18_750_000)) != 0 {
		t.Fatalf("quoted bond = %v, %v", quoted, err)
	}
	total, err := directFundedAmount(big.NewInt(12_345), bond)
	if err != nil || total.Cmp(big.NewInt(15_012_345)) != 0 {
		t.Fatalf("funded amount = %v, %v", total, err)
	}
	for _, rate := range []*big.Int{nil, big.NewInt(0), new(big.Int).Lsh(big.NewInt(1), 128)} {
		if _, err := directBondForFee(rate); err == nil {
			t.Fatalf("accepted invalid fee rate %v", rate)
		}
		if _, err := directQuotedBondForFee(rate); err == nil {
			t.Fatalf("accepted invalid quoted fee rate %v", rate)
		}
	}
	maxAmount := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	if _, err := directFundedAmount(maxAmount, bond); err == nil {
		t.Fatal("accepted overflowing price plus bond")
	}
}

func TestDirectIntentStakeAndStorageLayout(t *testing.T) {
	minimum, err := directIntentStakeForFee(big.NewInt(250_001), big.NewInt(2))
	if err != nil || minimum.Cmp(big.NewInt(250_001)) != 0 {
		t.Fatalf("price floor = %v, %v", minimum, err)
	}
	quoted, err := directQuotedIntentStakeForFee(big.NewInt(250_001), big.NewInt(2))
	if err != nil || quoted.Cmp(big.NewInt(312_501)) != 0 {
		t.Fatalf("price floor quote = %v, %v", quoted, err)
	}
	minimum, err = directIntentStakeForFee(big.NewInt(1000), big.NewInt(3))
	if err != nil || minimum.Cmp(big.NewInt(300_000)) != 0 {
		t.Fatalf("fee floor = %v, %v", minimum, err)
	}
	quoted, err = directQuotedIntentStakeForFee(big.NewInt(1000), big.NewInt(3))
	if err != nil || quoted.Cmp(big.NewInt(375_000)) != 0 {
		t.Fatalf("fee floor quote = %v, %v", quoted, err)
	}
	for _, rate := range []*big.Int{nil, big.NewInt(0), new(big.Int).Lsh(big.NewInt(1), 128)} {
		if _, err := directQuotedIntentStakeForFee(big.NewInt(1), rate); err == nil {
			t.Fatalf("accepted invalid intent rate %v", rate)
		}
	}
	raw := make([]byte, directIntentRecordBytes)
	big.NewInt(375_000).FillBytes(raw[:16])
	binary.BigEndian.PutUint64(raw[16:], 130)
	intent, err := DecodeDirectIntent(raw)
	if err != nil || intent.Stake.Cmp(big.NewInt(375_000)) != 0 || intent.At != 130 {
		t.Fatalf("intent layout = %+v, %v", intent, err)
	}
	if _, err := DecodeDirectIntent(raw[:23]); err == nil {
		t.Fatal("short intent accepted")
	}
	proofRate := make([]byte, directProofRateRecordBytes)
	big.NewInt(3).FillBytes(proofRate)
	rate, err := DecodeDirectProofRate(proofRate)
	if err != nil || rate.Cmp(big.NewInt(3)) != 0 {
		t.Fatalf("proof rate layout = %v, %v", rate, err)
	}
	if _, err := DecodeDirectProofRate(make([]byte, directProofRateRecordBytes)); err == nil {
		t.Fatal("zero proof rate accepted")
	}
}

func TestDirectFallbackPinsItsOwnSealedGuest(t *testing.T) {
	const proof = "../../proofs/registration-v2/dist/direct-v2-synthetic.json"
	artefact, err := readRegistrationArtefact(proof)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := assembleRegistration(NetworkNamePinned, "", proof, artefact,
		directFallbackOperation, preparedDirectFallbackFormat, directV2ImageIDHex)
	if err != nil || prepared.ImageID != directV2ImageIDHex {
		t.Fatalf("direct proof did not match its pinned guest: %v", err)
	}
	if _, err := AssemblePoolRegistration(NetworkNamePinned, "", proof, artefact); err == nil {
		t.Fatal("pool accepted the direct-v2 proof under its historical image pin")
	}
	wrong := *artefact
	wrong.ImageID = registrationImageIDHex
	if _, err := assembleRegistration(NetworkNamePinned, "", proof, &wrong,
		directFallbackOperation, preparedDirectFallbackFormat, directV2ImageIDHex); err == nil {
		t.Fatal("direct fallback accepted a proof that named the historical image")
	}
}

func TestDirectCalldataExactLayouts(t *testing.T) {
	args := DirectListArgs{Epoch: 230, Covered: 32, Inventory: [32]byte{0: 0x12}, Keys: [32]byte{31: 0x34}, Price: big.NewInt(12345)}
	listed, err := EncodeDirectList(args)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 86 || binary.BigEndian.Uint16(listed[0:2]) != 230 || binary.BigEndian.Uint32(listed[2:6]) != 32 ||
		listed[6] != 0x12 || listed[69] != 0x34 || new(big.Int).SetBytes(listed[70:86]).Cmp(args.Price) != 0 {
		t.Fatalf("wrong list calldata: %x", listed)
	}
	for _, covered := range []uint32{0, 33} {
		args.Covered = covered
		if _, err := EncodeDirectList(args); err == nil {
			t.Fatalf("accepted covered count %d", covered)
		}
	}
	args.Covered = 1
	for _, price := range []*big.Int{big.NewInt(0), big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 128)} {
		args.Price = price
		if _, err := EncodeDirectList(args); err == nil {
			t.Fatalf("accepted list price %s", price)
		}
	}
	args.Price = big.NewInt(1)
	args.Inventory = [32]byte{}
	if _, err := EncodeDirectList(args); err == nil {
		t.Fatal("accepted zero inventory commitment")
	}
	args.Inventory[0] = 1
	args.Keys = [32]byte{}
	if _, err := EncodeDirectList(args); err == nil {
		t.Fatal("accepted zero keys commitment")
	}

	id := [32]byte{0: 0xa1, 31: 0xb2}
	key := [65]byte{0: 4, 1: 0xc3, 64: 0xd4}
	bought, err := EncodeDirectBuy(id, key)
	if err != nil || len(bought) != 97 || !bytes.Equal(bought[:32], id[:]) || !bytes.Equal(bought[32:], key[:]) {
		t.Fatalf("wrong buy calldata: %x, %v", bought, err)
	}
	key[0] = 3
	if _, err := EncodeDirectBuy(id, key); err == nil {
		t.Fatal("accepted compressed delivery key")
	}
	key = [65]byte{0: 4}
	if _, err := EncodeDirectBuy(id, key); err == nil {
		t.Fatal("accepted zero delivery point")
	}

	confirmed, err := EncodeDirectConfirm(0x0102030405060708, []byte{0, 1, 2})
	const expectedDigest = "ae4b3280e56e2faf83f414a6e3dabe9d5fbe18976544c05fed121accb85b53fc"
	if err != nil || len(confirmed) != 40 || binary.BigEndian.Uint64(confirmed[:8]) != 0x0102030405060708 || hex.EncodeToString(confirmed[8:]) != expectedDigest {
		t.Fatalf("wrong confirm calldata: %x, %v", confirmed, err)
	}
	if _, err := EncodeDirectConfirm(0, []byte{1}); err == nil {
		t.Fatal("accepted zero order ID")
	}
	reserved, err := EncodeDirectReserveFallback(0x0102030405060708)
	if err != nil || len(reserved) != 8 || binary.BigEndian.Uint64(reserved) != 0x0102030405060708 {
		t.Fatalf("wrong fallback reservation calldata: %x, %v", reserved, err)
	}
	if _, err := EncodeDirectReserveFallback(0); err == nil {
		t.Fatal("accepted zero reservation order ID")
	}

	journal := bytes.Repeat([]byte{0x11}, 326)
	seal := bytes.Repeat([]byte{0x22}, 256)
	secrets := append([]byte("fod:epoch-secrets:v1"), bytes.Repeat([]byte{0x33}, 86)...)
	proved, err := EncodeDirectProveAndReveal(9, secrets)
	if err != nil || len(proved) != 114 || binary.BigEndian.Uint64(proved[:8]) != 9 || !bytes.Equal(proved[8:], secrets) {
		t.Fatalf("wrong reveal calldata: %x, %v", proved, err)
	}
	for _, invalid := range [][]byte{secrets[:105], append(bytes.Clone(secrets), 0)} {
		if _, err := EncodeDirectProveAndReveal(9, invalid); err == nil {
			t.Fatal("accepted wrong reveal length")
		}
	}
	badSecrets := bytes.Clone(secrets)
	badSecrets[0] ^= 1
	if _, err := EncodeDirectProveAndReveal(9, badSecrets); err == nil {
		t.Fatal("accepted noncanonical secrets domain")
	}
	if _, err := EncodeDirectProveAndReveal(0, secrets); err == nil {
		t.Fatal("accepted zero reveal order ID")
	}
	started, err := EncodeDirectStartFallback(math.MaxUint64, journal, seal)
	if err != nil || len(started) != 590 || binary.BigEndian.Uint64(started[:8]) != math.MaxUint64 ||
		!bytes.Equal(started[8:334], journal) || !bytes.Equal(started[334:], seal) {
		t.Fatalf("wrong fallback start calldata: %x, %v", started, err)
	}
	for _, input := range []struct{ journal, seal []byte }{{journal[:325], seal}, {journal, seal[:255]}} {
		if _, err := EncodeDirectStartFallback(9, input.journal, input.seal); err == nil {
			t.Fatal("accepted wrong start proof length")
		}
	}
	if _, err := EncodeDirectStartFallback(0, journal, seal); err == nil {
		t.Fatal("accepted zero fallback start order ID")
	}

	refunded, err := EncodeDirectRefund(math.MaxUint64)
	if err != nil || len(refunded) != 8 || binary.BigEndian.Uint64(refunded) != math.MaxUint64 {
		t.Fatalf("wrong refund calldata: %x, %v", refunded, err)
	}
	if _, err := EncodeDirectRefund(0); err == nil {
		t.Fatal("accepted zero refund order ID")
	}
}

func TestEncodeDirectStartFallbackV5(t *testing.T) {
	journal := bytes.Repeat([]byte{0x11}, 273)
	journal[272] = 0x12
	seal := bytes.Repeat([]byte{0x22}, 256)
	seal[255] = 0x23
	wire := bytes.Repeat([]byte{0x33}, 257)
	wire[256] = 0x34

	encoded, err := EncodeDirectStartFallbackV5(0x0102030405060708, journal, seal, wire)
	const headerBytes = 8 + 273 + 256 + 2
	if err != nil || len(encoded) != headerBytes+len(wire) {
		t.Fatalf("v5 fallback calldata length = %d, error = %v", len(encoded), err)
	}
	if binary.BigEndian.Uint64(encoded[:8]) != 0x0102030405060708 ||
		!bytes.Equal(encoded[8:281], journal) || !bytes.Equal(encoded[281:537], seal) ||
		!bytes.Equal(encoded[537:539], []byte{0x01, 0x01}) || !bytes.Equal(encoded[539:], wire) {
		t.Fatal("v5 fallback calldata has wrong field boundaries, length prefix, or trailing bytes")
	}

	maxWire := make([]byte, 65535)
	maxEncoded, err := EncodeDirectStartFallbackV5(9, journal, seal, maxWire)
	if err != nil || len(maxEncoded) != headerBytes+len(maxWire) ||
		!bytes.Equal(maxEncoded[537:539], []byte{0xff, 0xff}) {
		t.Fatalf("v5 fallback rejected or misencoded maximum wire length: %v", err)
	}

	for _, input := range []struct {
		name                string
		orderID             uint64
		journal, seal, wire []byte
	}{
		{"zero order ID", 0, journal, seal, wire},
		{"short journal", 9, journal[:272], seal, wire},
		{"trailing journal byte", 9, append(bytes.Clone(journal), 0), seal, wire},
		{"short seal", 9, journal, seal[:255], wire},
		{"trailing seal byte", 9, journal, append(bytes.Clone(seal), 0), wire},
		{"empty wire", 9, journal, seal, nil},
		{"wire beyond u16 limit", 9, journal, seal, make([]byte, 65536)},
	} {
		t.Run(input.name, func(t *testing.T) {
			if _, err := EncodeDirectStartFallbackV5(input.orderID, input.journal, input.seal, input.wire); err == nil {
				t.Fatal("accepted invalid v5 fallback calldata input")
			}
		})
	}
}

func TestDirectRolesAndProofRefundBoundary(t *testing.T) {
	buyer := [20]byte{0: 1}
	other := [20]byte{0: 2}
	order := DirectOrder{Buyer: buyer, FundedAt: 100, State: DirectFunded}
	if got := DirectProofDeadline(100); got != 8740 {
		t.Fatalf("first refund block = %d, want 8740", got)
	}
	if !order.CanConfirm(buyer) || order.CanConfirm(other) {
		t.Fatal("confirmation role differs from the funded buyer")
	}
	if order.CanReserveFallback(129) || !order.CanReserveFallback(130) || !order.CanReserveFallback(8619) || order.CanReserveFallback(8620) ||
		order.CanStartFallback(200, nil) || order.CanProve(8739) || order.CanRefund(8739, nil) || !order.CanRefund(8740, nil) {
		t.Fatal("funded reservation and refund boundary is wrong")
	}
	if !order.CanConfirm(buyer) {
		t.Fatal("buyer confirmation should remain available after the proof deadline")
	}
	intent := &DirectIntent{Stake: big.NewInt(100), At: 130}
	order.State = DirectFallbackIntent
	if order.CanConfirm(buyer) || order.CanReserveFallback(200) || order.CanStartFallback(129, intent) ||
		!order.CanStartFallback(130, intent) || !order.CanStartFallback(4449, intent) || order.CanStartFallback(4450, intent) ||
		order.CanRefund(4449, intent) || !order.CanRefund(4450, intent) || order.CanRefund(8740, nil) {
		t.Fatal("fallback intent proof and early refund boundary is wrong")
	}
	order.State = DirectFallbackPending
	if order.CanConfirm(buyer) || order.CanStartFallback(8739, intent) || !order.CanProve(8739) || order.CanProve(8740) || order.CanRefund(8739, nil) || !order.CanRefund(8740, nil) {
		t.Fatal("fallback-pending proof and refund boundary is wrong")
	}
	for _, state := range []DirectOrderState{DirectSettled, DirectRefunded} {
		order.State = state
		if order.CanConfirm(buyer) || order.CanStartFallback(8739, intent) || order.CanProve(8739) || order.CanRefund(8740, nil) {
			t.Fatalf("closed order state %d remains actionable", state)
		}
	}
	order.State = DirectFunded
	order.FundedAt = math.MaxUint64 - 1
	if DirectProofDeadline(order.FundedAt) != math.MaxUint64 || order.CanReserveFallback(math.MaxUint64-1) || order.CanRefund(math.MaxUint64, nil) {
		t.Fatal("uint64 overflow must fail closed like the contract")
	}
	order.State = DirectFallbackPending
	if order.CanProve(math.MaxUint64-1) || order.CanProve(math.MaxUint64) || order.CanRefund(math.MaxUint64, nil) {
		t.Fatal("fallback-pending uint64 deadline must refuse overflow refund")
	}
	offer := DirectOffer{Price: big.NewInt(1)}
	if !offer.CanBuy() {
		t.Fatal("open priced offer should be buyable")
	}
	offer.OpenOrderID = 9
	if offer.CanBuy() {
		t.Fatal("offer with a funded order remains buyable")
	}
	offer.OpenOrderID = 0
	offer.Retired = true
	if offer.CanBuy() {
		t.Fatal("retired offer remains buyable")
	}
}
