package main

// Small, method-specific v7 reads. The v6 snapshot has different record
// offsets, seed keys, offer IDs, and quote economics and cannot validate v7.

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
)

const (
	poolV7ProofWindow = uint64(12_960)
	poolV7BondGas     = int64(5_000_000)
	poolV7ValuePolicy = "pool/value-policy"
)

// The original deployed v7 has no policy key and accepts zero-value calls.
// The revised candidate stores 1 and refunds that one base unit in each
// otherwise nonpayable method. Reject unknown policies before preparing value.
func poolV7NonpayableValue(c *client, contract string) (*big.Int, error) {
	policy, err := readPoolV7U64(c, contract, poolV7ValuePolicy)
	if errors.Is(err, errNotInRegistry) {
		return big.NewInt(0), nil
	}
	if err != nil {
		return nil, err
	}
	if policy > 1 {
		return nil, fmt.Errorf("unsupported v7 attached-value policy %d", policy)
	}
	return new(big.Int).SetUint64(policy), nil
}

func poolV7IsNonpayable(method string) bool {
	switch method {
	case "qualify", "proveBundle", "slashUnproved", "seedBundle", "cancel", "expireHead",
		"challenge", "confirm", "startFallback", "proveAndReveal", "refund":
		return true
	default:
		return false
	}
}

type poolV7Offer struct {
	author          [20]byte
	epoch           uint16
	covered         uint32
	inventory       [32]byte
	keys            [32]byte
	sellerBond      *big.Int
	nextNonce       uint64
	openOrder       uint64
	status          byte
	everSold        bool
	previous        [32]byte
	next            [32]byte
	sequence        uint64
	listedAt        uint64
	contentProven   bool
	fraudCollateral *big.Int
	publicSeed      bool
}

type poolV7Order struct {
	offer        [32]byte
	buyer        [20]byte
	deliveryKey  [65]byte
	paid         *big.Int
	bond         *big.Int
	sellerPayout *big.Int
	fundedAt     uint64
	nonce        uint64
	state        byte
	challenged   bool
}

func decodePoolV7Offer(raw []byte) (*poolV7Offer, error) {
	if len(raw) != 222 || raw[122] > 6 || raw[123] > 1 || raw[204] > 1 || raw[221] > 1 {
		return nil, errors.New("invalid v7 offer record")
	}
	o := &poolV7Offer{
		epoch: binary.BigEndian.Uint16(raw[20:22]), covered: binary.BigEndian.Uint32(raw[22:26]),
		sellerBond: new(big.Int).SetBytes(raw[90:106]), nextNonce: binary.BigEndian.Uint64(raw[106:114]),
		openOrder: binary.BigEndian.Uint64(raw[114:122]), status: raw[122], everSold: raw[123] == 1,
		sequence: binary.BigEndian.Uint64(raw[188:196]), listedAt: binary.BigEndian.Uint64(raw[196:204]),
		contentProven: raw[204] == 1, fraudCollateral: new(big.Int).SetBytes(raw[205:221]), publicSeed: raw[221] == 1,
	}
	copy(o.author[:], raw[:20])
	copy(o.inventory[:], raw[26:58])
	copy(o.keys[:], raw[58:90])
	copy(o.previous[:], raw[124:156])
	copy(o.next[:], raw[156:188])
	if o.covered == 0 || o.covered > 32 || o.inventory == ([32]byte{}) || o.keys == ([32]byte{}) || o.nextNonce == 0 {
		return nil, errors.New("invalid v7 offer identity or commitments")
	}
	return o, nil
}

func decodePoolV7Order(raw []byte) (*poolV7Order, error) {
	if len(raw) != 215 || raw[181] < 1 || raw[181] > 4 || raw[214] > 1 {
		return nil, errors.New("invalid v7 order record")
	}
	o := &poolV7Order{
		paid: new(big.Int).SetBytes(raw[117:133]), bond: new(big.Int).SetBytes(raw[133:149]),
		sellerPayout: new(big.Int).SetBytes(raw[149:165]), fundedAt: binary.BigEndian.Uint64(raw[165:173]),
		nonce: binary.BigEndian.Uint64(raw[173:181]), state: raw[181], challenged: raw[214] == 1,
	}
	copy(o.offer[:], raw[:32])
	copy(o.buyer[:], raw[32:52])
	copy(o.deliveryKey[:], raw[52:117])
	return o, nil
}

func readPoolV7Offer(c *client, contract string, id [32]byte) (*poolV7Offer, error) {
	var raw []byte
	scanned := 0
	_, err := walkPoolV6Prefix(c.call, registryAddress(contract), poolV6OfferPrefix, &scanned, func(key string, value []byte) error {
		if len(key) != 64 {
			return errors.New("invalid v7 offer storage key")
		}
		if key == hex.EncodeToString(id[:]) {
			raw = append([]byte(nil), value...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, errNotInRegistry
	}
	o, err := decodePoolV7Offer(raw)
	if err != nil {
		return nil, err
	}
	if poolV7OfferID(o.author, o.epoch) != id {
		return nil, errors.New("v7 offer ID differs from its author and epoch")
	}
	return o, nil
}

func readPoolV7Order(c *client, contract string, id uint64) (*poolV7Order, error) {
	raw, err := readRegistryValue(c, contract, "pool/order/"+strconv.FormatUint(id, 10))
	if err != nil {
		return nil, err
	}
	return decodePoolV7Order(raw)
}

func readPoolV7U64(c *client, contract, key string) (uint64, error) {
	raw, err := readRegistryValue(c, contract, key)
	if err != nil {
		return 0, err
	}
	if len(raw) != 8 {
		return 0, fmt.Errorf("invalid v7 %s length", key)
	}
	return binary.BigEndian.Uint64(raw), nil
}

func readPoolV7U128(c *client, contract, key string) (*big.Int, error) {
	raw, err := readRegistryValue(c, contract, key)
	if err != nil {
		return nil, err
	}
	if len(raw) != 16 {
		return nil, fmt.Errorf("invalid v7 %s length", key)
	}
	return new(big.Int).SetBytes(raw), nil
}

func readPoolV7ID(c *client, contract, key string) ([32]byte, error) {
	var id [32]byte
	raw, err := readRegistryValue(c, contract, key)
	if err != nil {
		return id, err
	}
	if len(raw) != 32 {
		return id, fmt.Errorf("invalid v7 %s length", key)
	}
	copy(id[:], raw)
	return id, nil
}

func poolV7Quote(units uint64, reserve, insurance *big.Int) (*big.Int, error) {
	if units == 0 || units > ^uint64(0)-10 || reserve == nil || insurance == nil ||
		reserve.Cmp(insurance) <= 0 {
		return nil, errors.New("v7 pool has no valid current quote")
	}
	virtual := new(big.Int).Sub(reserve, insurance)
	denominator := new(big.Int).SetUint64(units + 9)
	quote, remainder := new(big.Int).QuoRem(virtual, denominator, new(big.Int))
	if remainder.Sign() != 0 {
		quote.Add(quote, big.NewInt(1))
	}
	cap, _ := new(big.Int).SetString(poolV7PriceCap, 10)
	if quote.Cmp(cap) > 0 {
		quote.Set(cap)
	}
	if quote.Sign() == 0 {
		return nil, errors.New("v7 pool quote is zero")
	}
	return quote, nil
}

func poolV7CurrentQuote(c *client, contract string, extraUnit bool) (*big.Int, error) {
	primary, err := readPoolV7U64(c, contract, "pool/primary-units")
	if err != nil {
		return nil, err
	}
	secondary, err := readPoolV7U64(c, contract, "pool/secondary-units")
	if err != nil {
		return nil, err
	}
	if primary > ^uint64(0)-secondary {
		return nil, errors.New("v7 pool unit count overflow")
	}
	units := primary + secondary
	if extraUnit {
		if units == ^uint64(0) {
			return nil, errors.New("v7 pool unit count overflow")
		}
		units++
	}
	reserve, err := readPoolV7U128(c, contract, "pool/reserve")
	if err != nil {
		return nil, err
	}
	insurance, err := readPoolV7U128(c, contract, "pool/insurance")
	if err != nil {
		return nil, err
	}
	return poolV7Quote(units, reserve, insurance)
}

func poolV7SecretsMatch(secrets []byte, offer *poolV7Offer) bool {
	if offer == nil {
		return false
	}
	id, digest, err := poolV7SecretsOfferID(secrets)
	return err == nil && id == poolV7OfferID(offer.author, offer.epoch) && digest == offer.keys
}

func poolV7JournalMatches(journal []byte, offer *poolV7Offer, contract string) bool {
	if len(journal) != fastContentJournalBytes || offer == nil ||
		binary.BigEndian.Uint32(journal[:4]) != 1 || binary.BigEndian.Uint16(journal[76:78]) != offer.epoch ||
		binary.BigEndian.Uint16(journal[78:80]) != uint16(offer.covered) ||
		!equalBytes(journal[56:76], offer.author[:]) || !equalBytes(journal[144:176], offer.inventory[:]) ||
		!equalBytes(journal[241:273], offer.keys[:]) ||
		hex.EncodeToString(journal[4:36]) != labelDigest(NetworkNamePinned) ||
		hex.EncodeToString(journal[80:112]) != labelDigest(ConsensusVersionPinned) ||
		hex.EncodeToString(journal[112:144]) != labelDigest(EligibilityPolicyV1) {
		return false
	}
	bound := hex.EncodeToString(journal[36:56])
	return bound == contract || (bound == poolV7LegacyContentContract &&
		offer.epoch == 230 && offer.status == 0 && offer.openOrder == 0 && !offer.everSold)
}

func poolV7Bond(rate *big.Int) (*big.Int, error) {
	if rate == nil || rate.Sign() <= 0 {
		return nil, errors.New("v7 fee rate is unavailable")
	}
	bond := new(big.Int).Mul(rate, big.NewInt(poolV7BondGas))
	if bond.Cmp(maxContractAmount) > 0 {
		return nil, errors.New("v7 fee bond exceeds u128")
	}
	return bond, nil
}

func poolV7ValidAmount(text string) (*big.Int, error) {
	if !isDecimal(text) {
		return nil, errors.New("invalid v7 amount")
	}
	amount, ok := new(big.Int).SetString(text, 10)
	if !ok || amount.String() != text || amount.Cmp(maxContractAmount) > 0 {
		return nil, errors.New("invalid or noncanonical v7 amount")
	}
	return amount, nil
}
