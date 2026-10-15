package main

import (
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/idena-network/idena-go/crypto"
)

const (
	preparedPoolV7Qualify   = "fod.prepared-direct-v7-qualify"
	preparedPoolV7Prove     = "fod.prepared-direct-v7-prove-bundle"
	preparedPoolV7Seed      = "fod.prepared-direct-v7-seed-bundle"
	preparedPoolV7Activate  = "fod.prepared-direct-v7-activate"
	preparedPoolV7List      = "fod.prepared-direct-v7-list"
	preparedPoolV7Buy       = "fod.prepared-direct-v7-buy"
	preparedPoolV7Challenge = "fod.prepared-direct-v7-challenge"
	preparedPoolV7Start     = "fod.prepared-direct-v7-start-fallback"
	preparedPoolV7Reveal    = "fod.prepared-direct-v7-prove-and-reveal"
)

var poolV7Formats = map[string]string{
	preparedPoolV7Qualify:   "qualify",
	preparedPoolV7Prove:     "proveBundle",
	preparedPoolV7Seed:      "seedBundle",
	preparedPoolV7Activate:  "activate",
	preparedPoolV7List:      "list",
	preparedPoolV7Buy:       "buy",
	preparedPoolV7Challenge: "challenge",
	preparedPoolV7Start:     "startFallback",
	preparedPoolV7Reveal:    "proveAndReveal",
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
	zero := amount.Sign() == 0
	switch method {
	case "qualify":
		if !zero || caller != p.Author || p.DirectSession == nil ||
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
		if !zero || offer.publicSeed || offer.contentProven || len(calldata) < 32+fastContentJournalBytes+fastContentSealBytes+2 ||
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
		if offer.status != 0 && offer.status != 1 && offer.status != 2 {
			return errors.New("v7 independent proof needs an unfunded qualified or queued offer")
		}
	case "seedBundle":
		if !zero || caller != p.Author || offer.status != 0 || !offer.contentProven || offer.publicSeed ||
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
		if caller != p.Buyer || offer.publicSeed || (offer.status != 1 && offer.status != 2) || len(calldata) != 97 ||
			!equalBytes(calldata[:32], id[:]) || p.DeliveryKey != hex.EncodeToString(calldata[32:]) {
			return errors.New("invalid v7 buyer, queue offer, or delivery key")
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
	case "challenge", "startFallback", "proveAndReveal":
		if !zero || p.DirectOrderID == 0 || len(calldata) < 8 || binary.BigEndian.Uint64(calldata[:8]) != p.DirectOrderID {
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
			if len(calldata) != 8 || caller != p.Author || order.state != 1 || !offer.contentProven ||
				head < order.fundedAt+30 || head >= order.fundedAt+poolV7ProofWindow-120 {
				return errors.New("v7 short fallback is not ready or safely before deadline")
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
