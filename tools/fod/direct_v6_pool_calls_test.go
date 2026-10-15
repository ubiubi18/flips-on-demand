package main

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
)

func directV6PoolTestHex(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func directV6PoolTestBytes(t *testing.T, method string, got []byte, err error, wantHex string, wantLen int) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	want := directV6PoolTestHex(t, wantHex)
	if len(got) != wantLen || !bytes.Equal(got, want) {
		t.Fatalf("%s: got %d bytes %x, want %d bytes %x", method, len(got), got, wantLen, want)
	}
}

// The lengths and field order below come from the v6 Rust ABI: OFFER_ARG_BYTES
// (86), BUY_ARG_BYTES (97), CONFIRM_ARG_BYTES (40), REVEAL_ARG_BYTES (114),
// and the [u8; 32] / u64 arguments. These expected bytes are independent of
// the Go encoder constants and helpers.
func TestDirectV6PoolCallExactRustLayouts(t *testing.T) {
	minimum, ok := new(big.Int).SetString("0102030405060708090a0b0c0d0e0f10", 16)
	if !ok {
		t.Fatal("invalid test minimum")
	}
	args := DirectV6PoolListArgs{Epoch: 0x0123, Covered: 2, MinimumPayout: minimum}
	copy(args.Inventory[:], bytes.Repeat([]byte{0x11}, 32))
	copy(args.Keys[:], bytes.Repeat([]byte{0x22}, 32))
	got, err := EncodeDirectV6PoolList(args)
	directV6PoolTestBytes(t, "list", got, err,
		"012300000002"+strings.Repeat("11", 32)+strings.Repeat("22", 32)+"0102030405060708090a0b0c0d0e0f10", 86)

	var offerID [32]byte
	offerID[0], offerID[31] = 0xa1, 0xb2
	offerHex := "a1" + strings.Repeat("00", 30) + "b2"
	var seedAuthor [20]byte
	copy(seedAuthor[:], bytes.Repeat([]byte{0x33}, 20))
	seedID := poolV6OfferID(seedAuthor, 230)
	seedHex := hex.EncodeToString(seedID[:])
	got, err = EncodeDirectV6PoolDeploySeedIDs(offerID, seedID)
	directV6PoolTestBytes(t, "deploy", got, err, offerHex+seedHex, 64)
	got, err = EncodeDirectV6PoolActivateSeedIDs(offerID, seedID)
	directV6PoolTestBytes(t, "activate", got, err, offerHex+seedHex, 64)
	got, err = EncodeDirectV6PoolBuyPublicSeed(seedID)
	directV6PoolTestBytes(t, "buyPublicSeed", got, err, seedHex, 32)
	got, err = EncodeDirectV6PoolTopUpBond(offerID)
	directV6PoolTestBytes(t, "topUpBond", got, err, offerHex, 32)

	// secp256k1 generator, encoded as the canonical uncompressed point.
	keyHex := "04" +
		"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798" +
		"483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8"
	key := directV6PoolTestHex(t, keyHex)
	got, err = EncodeDirectV6PoolBuy(offerID, key)
	directV6PoolTestBytes(t, "buy", got, err, offerHex+keyHex, 97)
	key[1] ^= 0xff
	if !bytes.Equal(got[32:], directV6PoolTestHex(t, keyHex)) {
		t.Fatal("buy calldata aliases the caller's public key")
	}

	const orderHex = "0102030405060708"
	const orderID = uint64(0x0102030405060708)
	got, err = EncodeDirectV6PoolChallenge(orderID)
	directV6PoolTestBytes(t, "challenge", got, err, orderHex, 8)
	got, err = EncodeDirectV6PoolConfirm(orderID, []byte{0, 1, 2})
	directV6PoolTestBytes(t, "confirm", got, err,
		orderHex+"ae4b3280e56e2faf83f414a6e3dabe9d5fbe18976544c05fed121accb85b53fc", 40)
	got, err = EncodeDirectV6PoolStartFallback(orderID)
	directV6PoolTestBytes(t, "startFallback pre-proved", got, err, orderHex, 8)

	secretsHex := "666f643a65706f63682d736563726574733a7631" + "00e6" +
		strings.Repeat("33", 20) + strings.Repeat("44", 32) + strings.Repeat("55", 32)
	secrets := directV6PoolTestHex(t, secretsHex)
	got, err = EncodeDirectV6PoolSeedBundle(seedID, secrets)
	directV6PoolTestBytes(t, "seedBundle", got, err, seedHex+secretsHex, 138)
	got, err = EncodeDirectV6PoolProveAndReveal(orderID, secrets)
	directV6PoolTestBytes(t, "proveAndReveal", got, err, orderHex+secretsHex, 114)
	secrets[22] ^= 0xff
	if !bytes.Equal(got[8:], directV6PoolTestHex(t, secretsHex)) {
		t.Fatal("reveal calldata aliases the caller's secrets")
	}

	got, err = EncodeDirectV6PoolRefund(orderID)
	directV6PoolTestBytes(t, "refund", got, err, orderHex, 8)
	got, err = EncodeDirectV6PoolRefund(^uint64(0))
	directV6PoolTestBytes(t, "max u64 order ID", got, err, "ffffffffffffffff", 8)
}

func TestDirectV6PoolQualificationAndProofExactRustLayouts(t *testing.T) {
	var inventory, keys, offerID [32]byte
	copy(inventory[:], bytes.Repeat([]byte{0x11}, 32))
	copy(keys[:], bytes.Repeat([]byte{0x22}, 32))
	copy(offerID[:], bytes.Repeat([]byte{0x33}, 32))
	wire := []byte{0x01, 0xa0, 0xb0}
	qualify, err := EncodeDirectV6PoolQualify(DirectV6PoolQualifyArgs{
		Epoch: 0x0123, Covered: 2, Inventory: inventory, Keys: keys,
	}, wire)
	directV6PoolTestBytes(t, "qualify", qualify, err,
		"012300000002"+strings.Repeat("11", 32)+strings.Repeat("22", 32)+"000301a0b0", 75)
	journal := bytes.Repeat([]byte{0x44}, 273)
	seal := bytes.Repeat([]byte{0x55}, 256)
	prove, err := EncodeDirectV6PoolProveBundle(offerID, journal, seal, wire)
	directV6PoolTestBytes(t, "proveBundle", prove, err,
		strings.Repeat("33", 32)+strings.Repeat("44", 273)+strings.Repeat("55", 256)+"000301a0b0", 566)
	journal[0], seal[0], wire[0] = 0, 0, 0
	if prove[32] != 0x44 || prove[32+273] != 0x55 || prove[len(prove)-3] != 1 {
		t.Fatal("proveBundle calldata aliases its input")
	}
	if _, err := EncodeDirectV6PoolQualify(DirectV6PoolQualifyArgs{Epoch: 1, Covered: 2, Inventory: inventory, Keys: keys}, nil); err == nil {
		t.Fatal("qualify accepted an empty session witness")
	}
	if _, err := EncodeDirectV6PoolProveBundle([32]byte{}, prove[32:32+273], prove[32+273:32+273+256], prove[len(prove)-3:]); err == nil {
		t.Fatal("proveBundle accepted a zero offer ID")
	}
}

func TestDirectV6PoolCallRejectsMalformedFields(t *testing.T) {
	valid := DirectV6PoolListArgs{
		Epoch: 230, Covered: 1, Inventory: [32]byte{1}, Keys: [32]byte{2}, MinimumPayout: big.NewInt(0),
	}
	if _, err := EncodeDirectV6PoolList(valid); err != nil {
		t.Fatalf("v6 ABI permits a zero minimum payout: %v", err)
	}
	for _, test := range []struct {
		name string
		edit func(*DirectV6PoolListArgs)
	}{
		{"zero covered", func(a *DirectV6PoolListArgs) { a.Covered = 0 }},
		{"too many covered", func(a *DirectV6PoolListArgs) { a.Covered = 33 }},
		{"empty inventory", func(a *DirectV6PoolListArgs) { a.Inventory = [32]byte{} }},
		{"empty keys", func(a *DirectV6PoolListArgs) { a.Keys = [32]byte{} }},
		{"nil payout", func(a *DirectV6PoolListArgs) { a.MinimumPayout = nil }},
		{"negative payout", func(a *DirectV6PoolListArgs) { a.MinimumPayout = big.NewInt(-1) }},
		{"overflow payout", func(a *DirectV6PoolListArgs) { a.MinimumPayout = new(big.Int).Lsh(big.NewInt(1), 128) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := valid
			test.edit(&args)
			if _, err := EncodeDirectV6PoolList(args); err == nil {
				t.Fatal("accepted malformed listing")
			}
		})
	}

	if _, err := EncodeDirectV6PoolTopUpBond([32]byte{}); err == nil {
		t.Fatal("accepted zero offer ID for bond top-up")
	}
	if _, err := EncodeDirectV6PoolDeploySeedIDs([32]byte{}, [32]byte{1}); err == nil {
		t.Fatal("accepted zero first seed ID")
	}
	if _, err := EncodeDirectV6PoolDeploySeedIDs([32]byte{1}, [32]byte{1}); err == nil {
		t.Fatal("accepted duplicate seed IDs")
	}
	if _, err := EncodeDirectV6PoolBuyPublicSeed([32]byte{}); err == nil {
		t.Fatal("accepted zero public seed ID")
	}
	seedSecrets := append([]byte("fod:epoch-secrets:v1"), make([]byte, 86)...)
	if _, err := EncodeDirectV6PoolSeedBundle([32]byte{1}, seedSecrets); err == nil {
		t.Fatal("accepted secrets bound to another seed ID")
	}
	key := directV6PoolTestHex(t, "04"+
		"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"+
		"483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8")
	if _, err := EncodeDirectV6PoolBuy([32]byte{}, key); err == nil {
		t.Fatal("accepted zero expected offer ID")
	}
	offCurve := make([]byte, 65)
	offCurve[0], offCurve[32], offCurve[64] = 4, 1, 1
	for _, test := range []struct {
		name string
		key  []byte
	}{
		{"empty", nil},
		{"short", key[:64]},
		{"long", append(bytes.Clone(key), 0)},
		{"compressed prefix", append([]byte{2}, key[1:]...)},
		{"zero point", append([]byte{4}, make([]byte, 64)...)},
		{"off curve", offCurve},
	} {
		t.Run("key "+test.name, func(t *testing.T) {
			if _, err := EncodeDirectV6PoolBuy([32]byte{1}, test.key); err == nil {
				t.Fatal("accepted malformed buyer delivery key")
			}
		})
	}

	for _, test := range []struct {
		name   string
		encode func() ([]byte, error)
	}{
		{"challenge", func() ([]byte, error) { return EncodeDirectV6PoolChallenge(0) }},
		{"confirm", func() ([]byte, error) { return EncodeDirectV6PoolConfirm(0, []byte{1}) }},
		{"startFallback", func() ([]byte, error) { return EncodeDirectV6PoolStartFallback(0) }},
		{"proveAndReveal", func() ([]byte, error) { return EncodeDirectV6PoolProveAndReveal(0, make([]byte, 106)) }},
		{"refund", func() ([]byte, error) { return EncodeDirectV6PoolRefund(0) }},
	} {
		t.Run("zero order ID "+test.name, func(t *testing.T) {
			if _, err := test.encode(); err == nil {
				t.Fatal("accepted zero order ID")
			}
		})
	}
	if _, err := EncodeDirectV6PoolConfirm(1, nil); err == nil {
		t.Fatal("accepted empty delivery ciphertext")
	}
	secrets := append([]byte("fod:epoch-secrets:v1"), make([]byte, 86)...)
	for _, malformed := range [][]byte{secrets[:105], append(bytes.Clone(secrets), 0), append([]byte{0}, secrets[1:]...)} {
		if _, err := EncodeDirectV6PoolProveAndReveal(1, malformed); err == nil {
			t.Fatal("accepted malformed epoch secrets")
		}
	}
}
