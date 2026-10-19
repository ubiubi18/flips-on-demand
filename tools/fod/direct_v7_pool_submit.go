package main

import (
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

const (
	preparedPoolV7Qualify   = "fod.prepared-direct-v7-qualify"
	preparedPoolV7Prove     = "fod.prepared-direct-v7-prove-bundle"
	preparedPoolV7Seed      = "fod.prepared-direct-v7-seed-bundle"
	preparedPoolV7Activate  = "fod.prepared-direct-v7-activate"
	preparedPoolV7List      = "fod.prepared-direct-v7-list"
	preparedPoolV7Buy       = "fod.prepared-direct-v7-buy"
	preparedPoolV7PublicBuy = "fod.prepared-direct-v7-buy-public-seed"
	preparedPoolV7Confirm   = "fod.prepared-direct-v7-confirm"
	preparedPoolV7Challenge = "fod.prepared-direct-v7-challenge"
	preparedPoolV7Start     = "fod.prepared-direct-v7-start-fallback"
	preparedPoolV7Reveal    = "fod.prepared-direct-v7-prove-and-reveal"
	preparedPoolV7Refund    = "fod.prepared-direct-v7-refund"
	preparedPoolV7Relist    = "fod.prepared-direct-v7-relist"
	preparedPoolV7Cancel    = "fod.prepared-direct-v7-cancel"
	preparedPoolV7Expire    = "fod.prepared-direct-v7-expire-head"
	preparedPoolV7TopUp     = "fod.prepared-direct-v7-top-up-bond"
	preparedPoolV7Slash     = "fod.prepared-direct-v7-slash-unproved"
)

var poolV7Formats = map[string]string{
	preparedPoolV7Qualify:   "qualify",
	preparedPoolV7Prove:     "proveBundle",
	preparedPoolV7Seed:      "seedBundle",
	preparedPoolV7Activate:  "activate",
	preparedPoolV7List:      "list",
	preparedPoolV7Buy:       "buy",
	preparedPoolV7PublicBuy: "buyPublicSeed",
	preparedPoolV7Confirm:   "confirm",
	preparedPoolV7Challenge: "challenge",
	preparedPoolV7Start:     "startFallback",
	preparedPoolV7Reveal:    "proveAndReveal",
	preparedPoolV7Refund:    "refund",
	preparedPoolV7Relist:    "relist",
	preparedPoolV7Cancel:    "cancel",
	preparedPoolV7Expire:    "expireHead",
	preparedPoolV7TopUp:     "topUpBond",
	preparedPoolV7Slash:     "slashUnproved",
}

func isPoolV7Format(format string) bool { _, ok := poolV7Formats[format]; return ok }

// validatePreparedPoolV7 binds a typed file to the live v7 contract. It is
// called before signing and immediately before any later broadcast attempt.
func validatePreparedPoolV7(c *client, p *preparedAction, identity *ecdsa.PrivateKey) error {
	if identity == nil {
		return errors.New("missing v7 signing context")
	}
	caller, err := nodekeyAddressOf(identity)
	if err != nil {
		return err
	}
	return validatePreparedPoolV7Caller(c, p, caller)
}

func validatePreparedPoolV7Caller(c *client, p *preparedAction, caller string) error {
	if p == nil || c == nil || !isHex(caller, addressBytes) || caller != strings.ToLower(caller) {
		return errors.New("missing v7 call context")
	}
	p.poolV7Validated = false
	method, supported := poolV7Formats[p.Format]
	if !supported || p.Operation != method || p.DirectVersion != 7 || p.Network != NetworkNamePinned ||
		p.Contract != strings.ToLower(p.Contract) || !isHex(p.Contract, addressBytes) ||
		p.CodeHash != strings.ToLower(p.CodeHash) || !isHex(p.CodeHash, 32) ||
		!directLocalRPC(c.url) {
		return errors.New("invalid v7 method, code pin, or loopback RPC")
	}
	if err := checkBuyTarget(c, p.Contract, p.Contract, p.CodeHash); err != nil {
		return err
	}
	amount, err := poolV7ValidAmount(p.Amount)
	if err != nil {
		return err
	}
	calldata, err := hex.DecodeString(p.Calldata)
	if err != nil {
		return err
	}
	if !isHex(p.DirectOfferID, 32) {
		return errors.New("v7 prepared action has no offer ID")
	}
	var id [32]byte
	idRaw, _ := hex.DecodeString(p.DirectOfferID)
	copy(id[:], idRaw)
	if id == ([32]byte{}) {
		return errors.New("v7 offer ID is zero")
	}
	var offer *poolV7Offer
	if method != "qualify" {
		offer, err = readPoolV7Offer(c, p.Contract, id)
		if err != nil {
			return err
		}
		if p.Author != hex.EncodeToString(offer.author[:]) || p.Epoch != offer.epoch ||
			p.CoveredFlips != offer.covered || p.InventoryCommitment != hex.EncodeToString(offer.inventory[:]) ||
			p.KeyCommitment != hex.EncodeToString(offer.keys[:]) {
			return errors.New("v7 prepared offer metadata differs from current state")
		}
	}
	nonpayableValue := big.NewInt(0)
	nonpayableOK := true
	if poolV7IsNonpayable(method) {
		nonpayableValue, err = poolV7NonpayableValue(c, p.Contract)
		if err != nil {
			return err
		}
		nonpayableOK = amount.Cmp(nonpayableValue) == 0
	}
	switch method {
	case "qualify":
		if !nonpayableOK || caller != p.Author || p.DirectSession == nil ||
			p.DirectSession.Author != caller || p.Epoch != p.DirectSession.Epoch ||
			p.CoveredFlips != uint32(len(p.DirectSession.Manifest)) ||
			p.InventoryCommitment != inventoryCommitment(caller, p.Epoch, p.DirectSession.Manifest) ||
			!isHex(p.KeyCommitment, 32) {
			return errors.New("v7 qualification differs from the author or session")
		}
		if err := directVerifiedSession(c, p.DirectSession); err != nil {
			return err
		}
		authorRaw, _ := hex.DecodeString(caller)
		var author [20]byte
		copy(author[:], authorRaw)
		if poolV7OfferID(author, p.Epoch) != id {
			return errors.New("v7 qualification uses another offer ID")
		}
		if _, err := readPoolV7Offer(c, p.Contract, id); err == nil || !errors.Is(err, errNotInRegistry) {
			return errors.New("v7 offer already exists or cannot be checked")
		}
		wire, err := encodeFastRegistrationWire(p.DirectSession)
		if err != nil {
			return err
		}
		inventory, keys := mustHex32(p.InventoryCommitment), mustHex32(p.KeyCommitment)
		want, err := encodePoolV7Qualify(p.Epoch, p.CoveredFlips, inventory, keys, wire)
		if err != nil || !equalBytes(calldata, want) {
			return errors.New("v7 qualification calldata differs from canonical session")
		}
	case "proveBundle":
		if !nonpayableOK || offer.publicSeed || offer.contentProven || len(calldata) < 32+fastContentJournalBytes+fastContentSealBytes+2 ||
			!equalBytes(calldata[:32], id[:]) || !poolV7JournalMatches(calldata[32:32+fastContentJournalBytes], offer, p.Contract) {
			return errors.New("invalid v7 independent proof context")
		}
		claim, err := claimDigestFromHex(fastContentV5ImageIDHex, calldata[32:32+fastContentJournalBytes])
		if err != nil || p.ImageID != fastContentV5ImageIDHex || p.ClaimDigest != hex.EncodeToString(claim[:]) {
			return errors.New("v7 proof journal differs from its pinned guest claim")
		}
		at := 32 + fastContentJournalBytes + fastContentSealBytes
		wireSize := int(binary.BigEndian.Uint16(calldata[at : at+2]))
		if wireSize == 0 || wireSize > fastRegistrationMaxWireBytes || len(calldata) != at+2+wireSize ||
			p.DirectSession == nil || p.DirectSession.Author != p.Author || p.DirectSession.Epoch != p.Epoch {
			return errors.New("invalid v7 proof session wire")
		}
		if err := directVerifiedSession(c, p.DirectSession); err != nil {
			return err
		}
		wire, err := encodeFastRegistrationWire(p.DirectSession)
		if err != nil || !equalBytes(calldata[at+2:], wire) {
			return errors.New("v7 proof wire differs from canonical session")
		}
		switch offer.status {
		case 0, 1, 2: // Qualified or queued before funding.
		case 3, 4: // A funded or privately settled offer may still recover collateral.
			if offer.openOrder == 0 {
				return errors.New("v7 funded proof has no open order")
			}
			order, err := readPoolV7Order(c, p.Contract, offer.openOrder)
			if err != nil {
				return err
			}
			validState := offer.status == 3 && (order.state == 1 || order.state == 4) ||
				offer.status == 4 && order.state == 2
			if order.offer != id || !validState {
				return errors.New("v7 funded proof differs from its order state")
			}
			head, _, err := registryHead(c)
			if err != nil {
				return err
			}
			if order.fundedAt > ^uint64(0)-poolV7ProofWindow || head >= order.fundedAt+poolV7ProofWindow {
				return errors.New("v7 funded proof window has closed")
			}
		default:
			return errors.New("v7 independent proof needs a qualified, queued, or timely funded offer")
		}
	case "seedBundle":
		if !nonpayableOK || caller != p.Author || offer.status != 0 || !offer.contentProven || offer.publicSeed ||
			len(calldata) != 138 || !equalBytes(calldata[:32], id[:]) || !poolV7SecretsMatch(calldata[32:], offer) {
			return errors.New("v7 seed must be the author's qualified and proved offer")
		}
		seedID, err := readPoolV7ID(c, p.Contract, "pool/seed-id")
		if err != nil || seedID != id {
			return errors.New("v7 seed differs from constructor pin")
		}
		count, err := readPoolV7U64(c, p.Contract, "pool/seed-count")
		if err != nil || count != 0 {
			return errors.New("v7 seed slot is already occupied")
		}
	case "activate":
		seed, _ := new(big.Int).SetString(poolV7SeedAmount, 10)
		if amount.Cmp(seed) != 0 || !equalBytes(calldata, id[:]) || offer.status != 2 ||
			!offer.publicSeed || !offer.contentProven {
			return errors.New("v7 activation needs the proved public seed and exact deposit")
		}
		seedID, err := readPoolV7ID(c, p.Contract, "pool/seed-id")
		if err != nil || seedID != id {
			return errors.New("v7 activation differs from constructor seed")
		}
		count, err := readPoolV7U64(c, p.Contract, "pool/seed-count")
		if err != nil || count != 1 {
			return errors.New("v7 public seed is not queued")
		}
		reserve, err := readPoolV7U128(c, p.Contract, "pool/reserve")
		if err != nil || reserve.Sign() != 0 {
			return errors.New("v7 pool is already activated")
		}
	case "list":
		if caller != p.Author || offer.status != 0 || offer.everSold || offer.sellerBond.Sign() != 0 ||
			!isDecimal(p.MinimumPayout) {
			return errors.New("v7 list needs the author's qualified unsold offer")
		}
		minimum, ok := new(big.Int).SetString(p.MinimumPayout, 10)
		if !ok || minimum.Sign() < 0 || minimum.BitLen() > 128 || minimum.String() != p.MinimumPayout {
			return errors.New("invalid v7 minimum payout")
		}
		want, err := encodePoolV7List(offer.epoch, offer.covered, offer.inventory, offer.keys, minimum)
		if err != nil || !equalBytes(calldata, want) {
			return errors.New("v7 list calldata differs from qualified offer")
		}
		quote, err := poolV7CurrentQuote(c, p.Contract, true)
		if err != nil || minimum.Cmp(quote) > 0 {
			return errors.New("v7 minimum payout exceeds current listing quote")
		}
		rate, err := readFeePerGas(c)
		if err != nil {
			return err
		}
		bond, err := poolV7Bond(rate)
		if err != nil {
			return err
		}
		if !offer.contentProven {
			cap, _ := new(big.Int).SetString(poolV7PriceCap, 10)
			bond.Add(bond, cap)
		}
		if amount.Cmp(bond) != 0 {
			return errors.New("v7 list value differs from current required bond")
		}
	case "buy":
		if caller != p.Buyer || offer.publicSeed || (offer.status != 1 && offer.status != 2) ||
			p.OrderNonce == 0 || p.OrderNonce != offer.nextNonce || len(calldata) != 97 ||
			!equalBytes(calldata[:32], id[:]) || p.DeliveryKey != hex.EncodeToString(calldata[32:]) {
			return errors.New("invalid v7 buyer, queue offer, or delivery key")
		}
		if p.DirectSession == nil || p.DirectSession.Author != p.Author || p.DirectSession.Epoch != p.Epoch ||
			uint32(len(p.DirectSession.Manifest)) != p.CoveredFlips ||
			inventoryCommitment(p.Author, p.Epoch, p.DirectSession.Manifest) != p.InventoryCommitment {
			return errors.New("v7 buy has no canonical retained inventory")
		}
		if err := directVerifiedSession(c, p.DirectSession); err != nil {
			return err
		}
		if err := requireCheckedAvailability(p.Availability, time.Now()); err != nil {
			return err
		}
		if p.Availability.Author != p.Author || p.Availability.Epoch != p.Epoch ||
			p.Availability.CoveredFlips != int(p.CoveredFlips) ||
			p.Availability.InventoryCommitment != p.InventoryCommitment {
			return errors.New("v7 buy retained blobs differ from the queued inventory")
		}
		pub, err := crypto.UnmarshalPubkey(calldata[32:])
		if err != nil {
			return errors.New("invalid v7 delivery key point")
		}
		if hex.EncodeToString(crypto.PubkeyToAddress(*pub).Bytes()) == caller {
			return errors.New("v7 delivery key must differ from buyer signing identity")
		}
		primary, err := readPoolV7ID(c, p.Contract, "pool/primary-head")
		if err != nil {
			return err
		}
		oldest := primary
		if oldest == ([32]byte{}) {
			oldest, err = readPoolV7ID(c, p.Contract, "pool/secondary-head")
			if err != nil {
				return err
			}
		}
		if oldest != id {
			return errors.New("v7 buy target is not the oldest queued offer")
		}
		rate, err := readFeePerGas(c)
		if err != nil {
			return err
		}
		bond, err := poolV7Bond(rate)
		if err != nil || offer.sellerBond.Cmp(bond) < 0 {
			return errors.New("v7 seller delivery bond is below current fee requirement")
		}
		quote, err := poolV7CurrentQuote(c, p.Contract, false)
		if err != nil {
			return err
		}
		total := new(big.Int).Add(quote, bond)
		if amount.Cmp(total) != 0 || p.Price != quote.String() {
			return errors.New("v7 buy amount differs from current quote and bond")
		}
	case "confirm":
		if !nonpayableOK || p.DirectOrderID == 0 || p.DirectOpened == nil || caller != p.Buyer || len(calldata) != 40 {
			return errors.New("v7 confirmation needs the funded buyer's verified receipt")
		}
		want, err := preparePoolV7Confirm(c, p.Contract, p.DirectOrderID, p.DirectOpened)
		if err != nil {
			return err
		}
		if p.DirectOfferID != want.DirectOfferID || p.Calldata != want.Calldata ||
			p.CiphertextSha256 != want.CiphertextSha256 || p.Buyer != want.Buyer ||
			p.Author != want.Author || p.Epoch != want.Epoch ||
			p.CoveredFlips != want.CoveredFlips || p.OrderNonce != want.OrderNonce ||
			p.InventoryCommitment != want.InventoryCommitment || p.KeyCommitment != want.KeyCommitment {
			return errors.New("v7 confirmation differs from the signed delivery receipt or current order")
		}
	case "buyPublicSeed", "relist", "cancel", "expireHead", "topUpBond", "refund", "slashUnproved":
		if err := validatePoolV7LifecycleCall(c, p, caller, offer, id, calldata, amount, nonpayableValue); err != nil {
			return err
		}
	case "challenge", "startFallback", "proveAndReveal":
		if !nonpayableOK || p.DirectOrderID == 0 || len(calldata) < 8 || binary.BigEndian.Uint64(calldata[:8]) != p.DirectOrderID {
			return errors.New("invalid v7 order call")
		}
		order, err := readPoolV7Order(c, p.Contract, p.DirectOrderID)
		if err != nil {
			return err
		}
		if order.offer != id || offer.status != 3 || offer.openOrder != p.DirectOrderID ||
			offer.nextNonce != order.nonce+1 || order.fundedAt == 0 {
			return errors.New("v7 order differs from active offer")
		}
		head, _, err := registryHead(c)
		if err != nil {
			return err
		}
		if order.fundedAt > ^uint64(0)-poolV7ProofWindow || head >= order.fundedAt+poolV7ProofWindow {
			return errors.New("v7 proof window has closed")
		}
		switch method {
		case "challenge":
			if len(calldata) != 8 || caller != hex.EncodeToString(order.buyer[:]) || p.Buyer != caller ||
				order.state != 1 || order.challenged {
				return errors.New("v7 challenge requires the funded buyer")
			}
		case "startFallback":
			if caller != p.Author || order.state != 1 ||
				head < order.fundedAt+30 || head >= order.fundedAt+poolV7ProofWindow-120 {
				return errors.New("v7 fallback is not ready or safely before deadline")
			}
			if offer.contentProven {
				if len(calldata) != 8 || p.DirectSession != nil || p.ImageID != "" || p.ClaimDigest != "" {
					return errors.New("v7 proved fallback must use the short order-only call")
				}
			} else if err := validatePoolV7SealedFallbackStatement(c, p, offer, calldata); err != nil {
				return err
			}
		case "proveAndReveal":
			if len(calldata) != 114 || caller != p.Author || order.state != 4 ||
				!poolV7SecretsMatch(calldata[8:], offer) {
				return errors.New("v7 reveal needs seller, pending fallback, and committed keys")
			}
		}
	default:
		return errors.New("unsupported v7 method")
	}
	p.poolV7Validated = true
	return nil
}

func validatePoolV7SealedFallbackStatement(c *client, p *preparedAction, offer *poolV7Offer, calldata []byte) error {
	const header = 8 + fastContentJournalBytes + fastContentSealBytes + 2
	if p == nil || offer == nil || offer.contentProven || p.DirectSession == nil ||
		len(calldata) <= header || len(calldata) > header+fastRegistrationMaxWireBytes ||
		p.ImageID != fastContentV5ImageIDHex || p.DirectOrderID == 0 ||
		binary.BigEndian.Uint64(calldata[:8]) != p.DirectOrderID {
		return errors.New("invalid v7 sealed fallback context")
	}
	journal := calldata[8 : 8+fastContentJournalBytes]
	if hex.EncodeToString(journal[36:56]) != p.Contract ||
		!poolV7JournalMatches(journal, offer, p.Contract) {
		return errors.New("v7 sealed fallback journal differs from this contract or funded offer")
	}
	claim, err := claimDigestFromHex(fastContentV5ImageIDHex, journal)
	if err != nil || p.ClaimDigest != hex.EncodeToString(claim[:]) ||
		p.DirectSession.Author != p.Author || p.DirectSession.Epoch != p.Epoch ||
		uint32(len(p.DirectSession.Manifest)) != p.CoveredFlips ||
		inventoryCommitment(p.Author, p.Epoch, p.DirectSession.Manifest) != p.InventoryCommitment {
		return errors.New("v7 sealed fallback claim differs from the pinned content or session")
	}
	wire := calldata[header:]
	if len(wire) == 0 || binary.BigEndian.Uint16(calldata[header-2:header]) != uint16(len(wire)) {
		return errors.New("v7 sealed fallback has an invalid session length")
	}
	if err := directVerifiedSession(c, p.DirectSession); err != nil {
		return err
	}
	want, err := encodeFastRegistrationWire(p.DirectSession)
	if err != nil || !equalBytes(wire, want) {
		return errors.New("v7 sealed fallback wire differs from canonical session")
	}
	return nil
}

func mustHex32(value string) [32]byte {
	var out [32]byte
	raw, _ := hex.DecodeString(value)
	copy(out[:], raw)
	return out
}

func estimatePoolV7Call(c *client, p *preparedAction, sender string, maxFee, rate *big.Int) (uint64, *big.Int, error) {
	if p == nil || !p.poolV7Validated || !directLocalRPC(c.url) || !isPoolV7Format(p.Format) {
		return 0, nil, errors.New("v7 estimate requires validated typed call on loopback RPC")
	}
	return estimatePreparedDirectCall(c, p, sender, maxFee, rate)
}

func validatePoolV7ApprovedHash(p *preparedAction, approved string) error {
	approved = strings.ToLower(strings.TrimPrefix(approved, "0x"))
	if p == nil || !isHex(approved, 32) || approved != p.CodeHash {
		return fmt.Errorf("v7 prepared code hash differs from independent -code-hash pin")
	}
	return nil
}
