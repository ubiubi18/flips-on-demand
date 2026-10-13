package main

// Byte layouts and local state checks for contracts/direct-escrow. These
// helpers do not read chain state, sign transactions, or submit calldata.

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
)

const (
	directOfferRecordBytes      = 123
	directOrderRecordBytes      = 198
	directListArgBytes          = 86
	directBuyArgBytes           = 97
	directConfirmArgBytes       = 40
	directReserveArgBytes       = 8
	directStartFallbackArgBytes = 8 + 326 + 256
	directProveArgBytes         = 8 + 106
	directRefundArgBytes        = 8
	directJournalBytes          = 326
	directSealBytes             = 256
	directSecretsBytes          = 106
	directProofWindow           = 8_640 // About 48 hours at the observed 20-second block cadence.
	directIntentWindow          = 4_320
	directConfirmGrace          = 30
	directReserveMargin         = 120
	directIntentGasBudget       = 100_000
	directIntentRecordBytes     = 24
	directStakeRecordBytes      = 16
	directProofRateRecordBytes  = 16
	directOfferDomain           = "fod:direct-offer:v2"
	directBondGasBudget         = 2_500_000
	directBondFeeMargin         = 2
	// A signed quote can survive a fee increase of up to 25% before inclusion.
	directQuoteMarginNumerator   = 5
	directQuoteMarginDenominator = 4
	directSecretsDomain          = "fod:epoch-secrets:v1"
	directOfferPrefix            = "direct/offer/"
	directOrderPrefix            = "direct/order/"
	directRevealPrefix           = "direct/reveal/"
	directIntentPrefix           = "direct/intent/"
	directStakePrefix            = "direct/stake/"
	directProofRatePrefix        = "direct/proof-rate/"
)

type DirectOrderState byte

const (
	DirectFunded          DirectOrderState = 1
	DirectSettled         DirectOrderState = 2
	DirectRefunded        DirectOrderState = 3
	DirectFallbackPending DirectOrderState = 4
	DirectFallbackIntent  DirectOrderState = 5
)

// DirectOffer and DirectOrder mirror the fixed-width records in state.rs.
// Amounts are unsigned 128-bit base units, represented as big.Int values.
type DirectOffer struct {
	Author      [20]byte
	Epoch       uint16
	Covered     uint32
	Inventory   [32]byte
	Keys        [32]byte
	Price       *big.Int
	NextNonce   uint64
	OpenOrderID uint64
	Retired     bool
}

type DirectOrder struct {
	OfferID      [32]byte
	Buyer        [20]byte
	DeliveryKey  [65]byte
	Paid         *big.Int
	Bond         *big.Int
	FundedAt     uint64
	Nonce        uint64
	State        DirectOrderState
	DeliveryHash [32]byte
}

type DirectIntent struct {
	Stake *big.Int
	At    uint64
}

// DirectListArgs contains precisely the fields accepted by the list method.
// The author is the transaction caller, not a calldata field.
type DirectListArgs struct {
	Epoch     uint16
	Covered   uint32
	Inventory [32]byte
	Keys      [32]byte
	Price     *big.Int
}

func DirectOfferID(author [20]byte, epoch uint16) [32]byte {
	var epochBytes [2]byte
	binary.BigEndian.PutUint16(epochBytes[:], epoch)
	preimage := make([]byte, 0, len(directOfferDomain)+len(author)+len(epochBytes))
	preimage = append(preimage, directOfferDomain...)
	preimage = append(preimage, author[:]...)
	preimage = append(preimage, epochBytes[:]...)
	return sha256.Sum256(preimage)
}

// Storage keys use a raw offer ID, or an unpadded decimal order ID.
func DirectOfferStorageKey(id [32]byte) []byte {
	key := make([]byte, len(directOfferPrefix)+len(id))
	copy(key, directOfferPrefix)
	copy(key[len(directOfferPrefix):], id[:])
	return key
}

func DirectOrderStorageKey(id uint64) []byte {
	return []byte(directOrderPrefix + strconv.FormatUint(id, 10))
}

func DirectRevealStorageKey(id uint64) []byte {
	return []byte(directRevealPrefix + strconv.FormatUint(id, 10))
}

func DirectIntentStorageKey(id uint64) []byte {
	return []byte(directIntentPrefix + strconv.FormatUint(id, 10))
}

func DirectStakeStorageKey(id uint64) []byte {
	return []byte(directStakePrefix + strconv.FormatUint(id, 10))
}

func DirectProofRateStorageKey(id uint64) []byte {
	return []byte(directProofRatePrefix + strconv.FormatUint(id, 10))
}

func DecodeDirectIntent(raw []byte) (*DirectIntent, error) {
	if len(raw) != directIntentRecordBytes {
		return nil, fmt.Errorf("direct intent is %d bytes, expected %d", len(raw), directIntentRecordBytes)
	}
	intent := &DirectIntent{Stake: new(big.Int).SetBytes(raw[:16]), At: binary.BigEndian.Uint64(raw[16:])}
	if intent.Stake.Sign() <= 0 || intent.At == 0 {
		return nil, errors.New("direct intent has invalid stake or height")
	}
	return intent, nil
}

func DecodeDirectStake(raw []byte) (*big.Int, error) {
	if len(raw) != directStakeRecordBytes {
		return nil, fmt.Errorf("direct stake is %d bytes, expected %d", len(raw), directStakeRecordBytes)
	}
	stake := new(big.Int).SetBytes(raw)
	if stake.Sign() <= 0 {
		return nil, errors.New("direct stake is zero")
	}
	return stake, nil
}

func DecodeDirectProofRate(raw []byte) (*big.Int, error) {
	if len(raw) != directProofRateRecordBytes {
		return nil, fmt.Errorf("direct proof rate is %d bytes, expected %d", len(raw), directProofRateRecordBytes)
	}
	rate := new(big.Int).SetBytes(raw)
	if rate.Sign() <= 0 {
		return nil, errors.New("direct proof rate is zero")
	}
	return rate, nil
}

func directAmountBytes(amount *big.Int) ([16]byte, error) {
	var out [16]byte
	if amount == nil || amount.Sign() < 0 || amount.BitLen() > 128 {
		return out, errors.New("direct amount must be an unsigned 128-bit integer")
	}
	amount.FillBytes(out[:])
	return out, nil
}

func directBondForFee(rate *big.Int) (*big.Int, error) {
	if rate == nil || rate.Sign() <= 0 {
		return nil, errors.New("direct bond needs a positive current fee per gas")
	}
	bond := new(big.Int).Mul(rate, big.NewInt(directBondGasBudget*directBondFeeMargin))
	if _, err := directAmountBytes(bond); err != nil {
		return nil, fmt.Errorf("direct bond exceeds the contract amount range: %w", err)
	}
	return bond, nil
}

func directQuotedBondForFee(rate *big.Int) (*big.Int, error) {
	minimum, err := directBondForFee(rate)
	if err != nil {
		return nil, err
	}
	quoted := new(big.Int).Mul(minimum, big.NewInt(directQuoteMarginNumerator))
	quoted.Div(quoted, big.NewInt(directQuoteMarginDenominator))
	if _, err := directAmountBytes(quoted); err != nil {
		return nil, fmt.Errorf("direct quoted bond exceeds the contract amount range: %w", err)
	}
	return quoted, nil
}

func directIntentStakeForFee(price, rate *big.Int) (*big.Int, error) {
	if price == nil || price.Sign() <= 0 || rate == nil || rate.Sign() <= 0 {
		return nil, errors.New("direct intent needs a positive price and current fee per gas")
	}
	minimum := new(big.Int).Mul(rate, big.NewInt(directIntentGasBudget))
	if price.Cmp(minimum) > 0 {
		minimum.Set(price)
	}
	if _, err := directAmountBytes(minimum); err != nil {
		return nil, fmt.Errorf("direct intent stake exceeds the contract amount range: %w", err)
	}
	return minimum, nil
}

func directQuotedIntentStakeForFee(price, rate *big.Int) (*big.Int, error) {
	minimum, err := directIntentStakeForFee(price, rate)
	if err != nil {
		return nil, err
	}
	quoted := new(big.Int).Mul(minimum, big.NewInt(directQuoteMarginNumerator))
	quoted.Div(quoted, big.NewInt(directQuoteMarginDenominator))
	if _, err := directAmountBytes(quoted); err != nil {
		return nil, fmt.Errorf("direct quoted intent stake exceeds the contract amount range: %w", err)
	}
	return quoted, nil
}

// The v4 start deposits the same price/fee floor as the v3 intent, but the
// proof and stake now travel in one transaction.
func directQuotedStartStakeForFee(price, rate *big.Int) (*big.Int, error) {
	return directQuotedIntentStakeForFee(price, rate)
}

func directFundedAmount(price, bond *big.Int) (*big.Int, error) {
	if price == nil || bond == nil || price.Sign() <= 0 || bond.Sign() <= 0 {
		return nil, errors.New("direct funding needs a positive price and bond")
	}
	if _, err := directAmountBytes(price); err != nil {
		return nil, err
	}
	if _, err := directAmountBytes(bond); err != nil {
		return nil, err
	}
	total := new(big.Int).Add(price, bond)
	if _, err := directAmountBytes(total); err != nil {
		return nil, errors.New("direct price plus bond exceeds the contract amount range")
	}
	return total, nil
}

func DecodeDirectOffer(raw []byte) (*DirectOffer, error) {
	if len(raw) != directOfferRecordBytes {
		return nil, fmt.Errorf("direct offer is %d bytes, expected %d", len(raw), directOfferRecordBytes)
	}
	if raw[122] > 1 {
		return nil, errors.New("direct offer has invalid retired flag")
	}
	offer := &DirectOffer{
		Epoch:       binary.BigEndian.Uint16(raw[20:22]),
		Covered:     binary.BigEndian.Uint32(raw[22:26]),
		Price:       new(big.Int).SetBytes(raw[90:106]),
		NextNonce:   binary.BigEndian.Uint64(raw[106:114]),
		OpenOrderID: binary.BigEndian.Uint64(raw[114:122]),
		Retired:     raw[122] == 1,
	}
	copy(offer.Author[:], raw[0:20])
	copy(offer.Inventory[:], raw[26:58])
	copy(offer.Keys[:], raw[58:90])
	return offer, nil
}

func EncodeDirectOffer(offer DirectOffer) ([]byte, error) {
	price, err := directAmountBytes(offer.Price)
	if err != nil {
		return nil, err
	}
	out := make([]byte, directOfferRecordBytes)
	copy(out[0:20], offer.Author[:])
	binary.BigEndian.PutUint16(out[20:22], offer.Epoch)
	binary.BigEndian.PutUint32(out[22:26], offer.Covered)
	copy(out[26:58], offer.Inventory[:])
	copy(out[58:90], offer.Keys[:])
	copy(out[90:106], price[:])
	binary.BigEndian.PutUint64(out[106:114], offer.NextNonce)
	binary.BigEndian.PutUint64(out[114:122], offer.OpenOrderID)
	if offer.Retired {
		out[122] = 1
	}
	return out, nil
}

func DecodeDirectOrder(raw []byte) (*DirectOrder, error) {
	if len(raw) != directOrderRecordBytes {
		return nil, fmt.Errorf("direct order is %d bytes, expected %d", len(raw), directOrderRecordBytes)
	}
	state := DirectOrderState(raw[165])
	if state < DirectFunded || state > DirectFallbackIntent {
		return nil, fmt.Errorf("direct order has invalid state %d", state)
	}
	order := &DirectOrder{
		Paid:     new(big.Int).SetBytes(raw[117:133]),
		Bond:     new(big.Int).SetBytes(raw[133:149]),
		FundedAt: binary.BigEndian.Uint64(raw[149:157]),
		Nonce:    binary.BigEndian.Uint64(raw[157:165]),
		State:    state,
	}
	copy(order.OfferID[:], raw[0:32])
	copy(order.Buyer[:], raw[32:52])
	copy(order.DeliveryKey[:], raw[52:117])
	copy(order.DeliveryHash[:], raw[166:198])
	return order, nil
}

func DecodeDirectOrderV4(raw []byte) (*DirectOrder, error) {
	order, err := DecodeDirectOrder(raw)
	if err != nil {
		return nil, err
	}
	if order.State == DirectFallbackIntent {
		return nil, errors.New("v4 direct order has removed fallback-intent state")
	}
	return order, nil
}

func EncodeDirectOrder(order DirectOrder) ([]byte, error) {
	paid, err := directAmountBytes(order.Paid)
	if err != nil {
		return nil, err
	}
	bond, err := directAmountBytes(order.Bond)
	if err != nil {
		return nil, err
	}
	if order.State < DirectFunded || order.State > DirectFallbackIntent {
		return nil, fmt.Errorf("direct order has invalid state %d", order.State)
	}
	out := make([]byte, directOrderRecordBytes)
	copy(out[0:32], order.OfferID[:])
	copy(out[32:52], order.Buyer[:])
	copy(out[52:117], order.DeliveryKey[:])
	copy(out[117:133], paid[:])
	copy(out[133:149], bond[:])
	binary.BigEndian.PutUint64(out[149:157], order.FundedAt)
	binary.BigEndian.PutUint64(out[157:165], order.Nonce)
	out[165] = byte(order.State)
	copy(out[166:198], order.DeliveryHash[:])
	return out, nil
}

func EncodeDirectList(args DirectListArgs) ([]byte, error) {
	price, err := directAmountBytes(args.Price)
	if err != nil {
		return nil, err
	}
	if args.Covered == 0 || args.Covered > 32 || args.Inventory == ([32]byte{}) || args.Keys == ([32]byte{}) || args.Price.Sign() == 0 {
		return nil, errors.New("direct listing needs 1 to 32 flips, both commitments, and a positive price")
	}
	out := make([]byte, directListArgBytes)
	binary.BigEndian.PutUint16(out[0:2], args.Epoch)
	binary.BigEndian.PutUint32(out[2:6], args.Covered)
	copy(out[6:38], args.Inventory[:])
	copy(out[38:70], args.Keys[:])
	copy(out[70:86], price[:])
	return out, nil
}

func EncodeDirectBuy(offerID [32]byte, deliveryKey [65]byte) ([]byte, error) {
	var emptyKey [65]byte
	emptyKey[0] = 4
	if deliveryKey[0] != 4 || deliveryKey == emptyKey {
		return nil, errors.New("direct buy needs an uncompressed nonzero delivery key")
	}
	out := make([]byte, directBuyArgBytes)
	copy(out[0:32], offerID[:])
	copy(out[32:97], deliveryKey[:])
	return out, nil
}

// The buyer confirms the exact private ciphertext bytes they checked locally.
func EncodeDirectConfirm(orderID uint64, ciphertext []byte) ([]byte, error) {
	if orderID == 0 {
		return nil, errors.New("direct order ID must be positive")
	}
	digest := sha256.Sum256(ciphertext)
	out := make([]byte, directConfirmArgBytes)
	binary.BigEndian.PutUint64(out[0:8], orderID)
	copy(out[8:40], digest[:])
	return out, nil
}

func EncodeDirectReserveFallback(orderID uint64) ([]byte, error) {
	if orderID == 0 {
		return nil, errors.New("direct order ID must be positive")
	}
	out := make([]byte, directReserveArgBytes)
	binary.BigEndian.PutUint64(out, orderID)
	return out, nil
}

func EncodeDirectProveAndReveal(orderID uint64, secrets []byte) ([]byte, error) {
	if orderID == 0 || len(secrets) != directSecretsBytes {
		return nil, errors.New("direct reveal needs an order ID and 106-byte secrets")
	}
	if string(secrets[:len(directSecretsDomain)]) != directSecretsDomain {
		return nil, errors.New("direct reveal needs canonical epoch secrets")
	}
	out := make([]byte, directProveArgBytes)
	binary.BigEndian.PutUint64(out[0:8], orderID)
	copy(out[8:], secrets)
	return out, nil
}

// This packages a proof; callers must verify the seal and canonical roots.
func EncodeDirectStartFallback(orderID uint64, journal, seal []byte) ([]byte, error) {
	if orderID == 0 || len(journal) != directJournalBytes || len(seal) != directSealBytes {
		return nil, errors.New("direct fallback start needs an order ID, 326-byte journal, and 256-byte seal")
	}
	out := make([]byte, directStartFallbackArgBytes)
	binary.BigEndian.PutUint64(out[:8], orderID)
	copy(out[8:8+directJournalBytes], journal)
	copy(out[8+directJournalBytes:], seal)
	return out, nil
}

// This packages a v5 fast-content proof and its exact public session wire.
// Callers must verify the seal and canonical roots before submission.
func EncodeDirectStartFallbackV5(orderID uint64, journal, seal, sessionWire []byte) ([]byte, error) {
	if orderID == 0 || len(journal) != fastContentJournalBytes || len(seal) != fastContentSealBytes ||
		len(sessionWire) == 0 || len(sessionWire) > fastRegistrationMaxWireBytes {
		return nil, errors.New("direct v5 fallback start needs an order ID, 273-byte journal, 256-byte seal, and nonempty bounded session wire")
	}
	out := make([]byte, 8+fastContentJournalBytes+fastContentSealBytes+2+len(sessionWire))
	binary.BigEndian.PutUint64(out[:8], orderID)
	offset := 8
	offset += copy(out[offset:], journal)
	offset += copy(out[offset:], seal)
	binary.BigEndian.PutUint16(out[offset:offset+2], uint16(len(sessionWire)))
	copy(out[offset+2:], sessionWire)
	return out, nil
}

func EncodeDirectRefund(orderID uint64) ([]byte, error) {
	if orderID == 0 {
		return nil, errors.New("direct order ID must be positive")
	}
	out := make([]byte, directRefundArgBytes)
	binary.BigEndian.PutUint64(out, orderID)
	return out, nil
}

// DirectProofDeadline is the first refund block for representable funded heights.
// The contract rejects overflow; the saturated value is display-only then.
func DirectProofDeadline(fundedAt uint64) uint64 {
	if fundedAt > math.MaxUint64-directProofWindow {
		return math.MaxUint64
	}
	return fundedAt + directProofWindow
}

func directReserveDeadline(fundedAt uint64) (uint64, bool) {
	if fundedAt > math.MaxUint64-directProofWindow {
		return 0, false
	}
	return fundedAt + directProofWindow - directReserveMargin, true
}

func directIntentDeadline(intentAt uint64) (uint64, bool) {
	if intentAt > math.MaxUint64-directIntentWindow {
		return 0, false
	}
	return intentAt + directIntentWindow, true
}

func (order DirectOrder) CanReserveFallback(height uint64) bool {
	if order.State != DirectFunded || order.FundedAt > math.MaxUint64-directConfirmGrace {
		return false
	}
	end, ok := directReserveDeadline(order.FundedAt)
	return ok && height >= order.FundedAt+directConfirmGrace && height < end
}

func (order DirectOrder) CanStartFallbackV4(height uint64) bool {
	return order.CanReserveFallback(height)
}

func (offer DirectOffer) CanBuy() bool {
	return !offer.Retired && offer.OpenOrderID == 0 && offer.Price != nil && offer.Price.Sign() > 0
}

// Only the funded order's buyer may confirm. Confirmation has no deadline;
// an included seller intent or settlement closes it.
func (order DirectOrder) CanConfirm(caller [20]byte) bool {
	return order.State == DirectFunded && caller == order.Buyer
}

func (order DirectOrder) CanStartFallback(height uint64, intent *DirectIntent) bool {
	if order.State != DirectFallbackIntent || intent == nil || intent.Stake == nil || intent.Stake.Sign() <= 0 ||
		intent.At < order.FundedAt || height < intent.At {
		return false
	}
	intentEnd, ok := directIntentDeadline(intent.At)
	if !ok {
		return false
	}
	proofEnd, ok := directReserveDeadline(order.FundedAt)
	return ok && height < min(intentEnd, proofEnd)
}

// The reveal method accepts only the listed author as caller, and the contract
// checks the secrets against the commitment after start verified the proof.
func (order DirectOrder) CanProve(height uint64) bool {
	return order.State == DirectFallbackPending && order.FundedAt <= math.MaxUint64-directProofWindow &&
		height < order.FundedAt+directProofWindow
}

// Anyone may call refund at the boundary; payment goes to the stored buyer.
func (order DirectOrder) RefundAt(intent *DirectIntent) (uint64, bool) {
	end := DirectProofDeadline(order.FundedAt)
	if end == math.MaxUint64 && order.FundedAt > math.MaxUint64-directProofWindow {
		return 0, false
	}
	switch order.State {
	case DirectFunded, DirectFallbackPending:
		return end, true
	case DirectFallbackIntent:
		if intent == nil {
			return 0, false
		}
		intentEnd, ok := directIntentDeadline(intent.At)
		if !ok {
			return 0, false
		}
		return min(intentEnd, end), true
	default:
		return 0, false
	}
}

func (order DirectOrder) CanRefund(height uint64, intent *DirectIntent) bool {
	end, ok := order.RefundAt(intent)
	return ok && height >= end
}
