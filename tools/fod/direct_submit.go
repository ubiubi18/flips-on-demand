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

func isDirectFormat(format string) bool {
	switch format {
	case preparedDirectListFormat, preparedDirectBuyFormat, preparedDirectConfirmFormat,
		preparedDirectReserveFallbackFormat, preparedDirectStartFallbackFormat, preparedDirectStartFallbackV4Format,
		preparedDirectStartFallbackV5Format,
		preparedDirectFallbackFormat, preparedDirectRefundFormat:
		return true
	default:
		return false
	}
}

func directPreparedOfferID(p *preparedAction) ([32]byte, error) {
	var id [32]byte
	raw, err := hex.DecodeString(p.DirectOfferID)
	if err != nil || len(raw) != len(id) {
		return id, errors.New("invalid direct offer ID")
	}
	copy(id[:], raw)
	return id, nil
}

func validateDirectV4Code(c *client, contract, codeHash string) error {
	if !isHex(codeHash, 32) || checkBuyTarget(c, contract, contract, codeHash) != nil {
		return errors.New("v4 direct action needs the pinned v4 contract code hash")
	}
	return nil
}

func validateDirectV5Code(c *client, contract, codeHash string) error {
	if !isHex(codeHash, 32) || checkBuyTarget(c, contract, contract, codeHash) != nil {
		return errors.New("v5 direct action needs the reproducibly pinned v5 contract code hash")
	}
	return nil
}

func validateDirectV3Contract(c *client, contract string) error {
	if contract != directV3ContractHex || checkBuyTarget(c, contract, contract, directV3CodeHashHex) != nil {
		return errors.New("v3 direct fallback needs the exact deployed v3 contract and code hash")
	}
	return nil
}

// Each direct action is rechecked against canonical state immediately before
// signing and again immediately before broadcast. The prepared JSON is a claim.
func validatePreparedDirect(c *client, p *preparedAction, identity *ecdsa.PrivateKey) error {
	return validatePreparedDirectWithPins(c, p, identity, directV4CodeHashHex, directV5CodeHashHex)
}

func validatePreparedDirectWithV4Pin(c *client, p *preparedAction, identity *ecdsa.PrivateKey, v4CodeHash string) error {
	return validatePreparedDirectWithPins(c, p, identity, v4CodeHash, "")
}

func validatePreparedDirectWithPins(c *client, p *preparedAction, identity *ecdsa.PrivateKey, v4CodeHash, v5CodeHash string) error {
	p.directValidated = false
	p.buyValidated = false
	p.confirmValidated = false
	if !isDirectFormat(p.Format) || p.Network != NetworkNamePinned || p.Amount == "" ||
		p.Contract != strings.ToLower(p.Contract) || !isHex(p.Contract, addressBytes) {
		return errors.New("invalid direct action context")
	}
	if p.DirectVersion == 5 {
		if err := validateDirectV5Code(c, p.Contract, v5CodeHash); err != nil {
			return err
		}
	} else if v5CodeHash != "" && checkBuyTarget(c, p.Contract, p.Contract, v5CodeHash) == nil {
		return errors.New("v5 direct contract requires a v5 prepared action")
	}
	caller, err := nodekeyAddressOf(identity)
	if err != nil {
		return err
	}
	id, err := directPreparedOfferID(p)
	if err != nil {
		return err
	}
	calldata, err := hex.DecodeString(p.Calldata)
	if err != nil {
		return err
	}
	switch p.Format {
	case preparedDirectListFormat:
		if p.Amount != "0" || caller != p.Author || p.DirectSession == nil ||
			p.Epoch != p.DirectSession.Epoch || p.CoveredFlips != uint32(len(p.DirectSession.Manifest)) {
			return errors.New("direct listing author or session changed")
		}
		if err := directVerifiedSession(c, p.DirectSession); err != nil {
			return err
		}
		author, _ := hex.DecodeString(caller)
		var author20 [20]byte
		copy(author20[:], author)
		if DirectOfferID(author20, p.Epoch) != id {
			return errors.New("direct listing ID differs from author and epoch")
		}
		if _, err := directOfferFromChain(c, p.Contract, id); err == nil {
			return errors.New("direct listing already exists")
		} else if !errors.Is(err, errNotInRegistry) {
			return err
		}
		if p.InventoryCommitment != inventoryCommitment(caller, p.Epoch, p.DirectSession.Manifest) {
			return errors.New("direct listing inventory changed")
		}
		if !isHex(p.KeyCommitment, 32) {
			return errors.New("direct listing has no key commitment")
		}
		if len(calldata) != directListArgBytes || binary.BigEndian.Uint16(calldata[:2]) != p.Epoch ||
			binary.BigEndian.Uint32(calldata[2:6]) != p.CoveredFlips ||
			hex.EncodeToString(calldata[6:38]) != p.InventoryCommitment ||
			hex.EncodeToString(calldata[38:70]) != p.KeyCommitment ||
			!isDecimal(p.Price) || p.Price != newBigFromBytes(calldata[70:86]) {
			return errors.New("direct listing calldata differs from checked terms")
		}
	case preparedDirectBuyFormat:
		if p.Buyer != caller || p.DirectSession == nil || p.DirectOfferID == "" {
			return errors.New("direct purchase buyer or session changed")
		}
		if err := directVerifiedSession(c, p.DirectSession); err != nil {
			return err
		}
		if err := requireCheckedAvailability(p.Availability, time.Now()); err != nil {
			return err
		}
		offer, err := directOfferFromChain(c, p.Contract, id)
		if err != nil {
			return err
		}
		if !offer.CanBuy() || p.OrderNonce != offer.NextNonce || p.Price != offer.Price.String() ||
			p.Epoch != offer.Epoch || p.CoveredFlips != offer.Covered || p.Author != hex.EncodeToString(offer.Author[:]) ||
			p.DirectSession.Epoch != offer.Epoch || !strings.EqualFold(p.DirectSession.Author, p.Author) ||
			p.Availability.CoveredFlips != int(offer.Covered) || p.Availability.Epoch != offer.Epoch ||
			strings.TrimPrefix(strings.ToLower(p.Availability.Author), "0x") != p.Author ||
			p.InventoryCommitment != hex.EncodeToString(offer.Inventory[:]) || p.KeyCommitment != hex.EncodeToString(offer.Keys[:]) ||
			p.Availability.InventoryCommitment != p.InventoryCommitment ||
			inventoryCommitment(p.Author, p.Epoch, p.DirectSession.Manifest) != p.InventoryCommitment {
			return errors.New("direct purchase differs from current offer or canonical inventory")
		}
		rate, err := readFeePerGas(c)
		if err != nil {
			return fmt.Errorf("read current direct bond fee: %w", err)
		}
		minimumBond, err := directBondForFee(rate)
		if err != nil {
			return err
		}
		maximumBond, err := directQuotedBondForFee(rate)
		if err != nil {
			return err
		}
		funded, ok := new(big.Int).SetString(p.Amount, 10)
		if !ok || !isDecimal(p.Amount) || funded.String() != p.Amount {
			return errors.New("invalid direct purchase funding amount")
		}
		bond := new(big.Int).Sub(funded, offer.Price)
		if bond.Cmp(minimumBond) < 0 || bond.Cmp(maximumBond) > 0 {
			return errors.New("direct purchase bond is outside the current fee quote")
		}
		total, err := directFundedAmount(offer.Price, bond)
		if err != nil || total.Cmp(funded) != 0 {
			return errors.New("invalid direct purchase funding amount")
		}
		if len(calldata) != directBuyArgBytes || !equalBytes(calldata[:32], id[:]) || hex.EncodeToString(calldata[32:]) != p.DeliveryKey {
			return errors.New("direct purchase calldata differs from checked delivery key")
		}
		deliveryPublicKey, err := cryptoUnmarshalDirectKey(calldata[32:])
		if err != nil {
			return err
		}
		if hex.EncodeToString(crypto.PubkeyToAddress(*deliveryPublicKey).Bytes()) == caller {
			return errors.New("direct delivery key is the buyer's signing identity")
		}
		p.buyValidated = true
		p.buyTargetValidated = p.Contract
	case preparedDirectConfirmFormat:
		if p.Amount != "0" || p.DirectOpened == nil || p.DirectOrderID != p.DirectOpened.OrderID ||
			p.DirectOfferID != p.DirectOpened.OfferID || p.Buyer != caller {
			return errors.New("direct confirmation context changed")
		}
		if err := verifyDirectOpened(p.DirectOpened); err != nil {
			return err
		}
		if p.DirectOpened.Buyer != caller || p.DirectOpened.Contract != p.Contract {
			return errors.New("direct receipt belongs to another buyer or contract")
		}
		prepared, err := prepareDirectConfirm(c, p.Contract, p.DirectOpened)
		if err != nil {
			return err
		}
		if prepared.Calldata != p.Calldata || prepared.CiphertextSha256 != p.CiphertextSha256 || prepared.DirectOfferID != p.DirectOfferID {
			return errors.New("direct confirmation bytes differ from checked receipt")
		}
		p.confirmValidated = true
	case preparedDirectReserveFallbackFormat:
		if p.DirectVersion != 0 && p.DirectVersion != 3 {
			return errors.New("v4 direct fallback has no reservation")
		}
		if err := validateDirectV3Contract(c, p.Contract); err != nil {
			return err
		}
		if p.Operation != directReserveFallbackOperation || len(calldata) != directReserveArgBytes ||
			p.DirectOrderID == 0 || binary.BigEndian.Uint64(calldata) != p.DirectOrderID {
			return errors.New("invalid direct fallback reservation bytes")
		}
		offer, order, _, err := directReserveFallbackReady(c, p.Contract, p.DirectOrderID, caller)
		if err != nil {
			return err
		}
		if p.Author != caller || order.OfferID != id || p.OrderNonce != order.Nonce || p.Price != offer.Price.String() {
			return errors.New("direct fallback reservation differs from current seller, offer, or nonce")
		}
		rate, err := readFeePerGas(c)
		if err != nil {
			return fmt.Errorf("read current direct intent fee: %w", err)
		}
		minimum, err := directIntentStakeForFee(offer.Price, rate)
		if err != nil {
			return err
		}
		maximum, err := directQuotedIntentStakeForFee(offer.Price, rate)
		if err != nil {
			return err
		}
		stake, ok := new(big.Int).SetString(p.Amount, 10)
		if !ok || !isDecimal(p.Amount) || stake.String() != p.Amount ||
			stake.Cmp(minimum) < 0 || stake.Cmp(maximum) > 0 {
			return errors.New("direct fallback reservation stake is outside the current fee quote")
		}
	case preparedDirectStartFallbackFormat:
		if p.DirectVersion != 0 && p.DirectVersion != 3 {
			return errors.New("v3 direct fallback format cannot authorize v4 start")
		}
		if err := validateDirectV3Contract(c, p.Contract); err != nil {
			return err
		}
		if p.Operation != directStartFallbackOperation || p.Amount != "0" || len(calldata) != directStartFallbackArgBytes ||
			p.DirectOrderID == 0 || binary.BigEndian.Uint64(calldata[:8]) != p.DirectOrderID {
			return errors.New("invalid direct fallback start bytes")
		}
		offer, order, _, err := directActiveSale(c, p.Contract, p.DirectOrderID)
		if err != nil {
			return err
		}
		if caller != hex.EncodeToString(offer.Author[:]) || p.Author != caller || order.OfferID != id {
			return errors.New("direct fallback start signer is not this offer's author")
		}
		head, _, err := registryHead(c)
		if err != nil {
			return err
		}
		if order.State != DirectFallbackIntent {
			return errors.New("direct fallback proof start requires an included fallback-intent order")
		}
		intent, err := directIncludedIntent(c, p.Contract, p.DirectOrderID, offer, order, head)
		if err != nil {
			return err
		}
		intentEnd, intentOK := directIntentDeadline(intent.At)
		proofEnd, proofOK := directReserveDeadline(order.FundedAt)
		if !intentOK || !proofOK || !order.CanStartFallback(head, intent) || head >= min(intentEnd, proofEnd)-1 {
			return errors.New("direct fallback proof start has no safe inclusion margin")
		}
		if err := validateDirectStartFallbackStatement(c, p, offer, calldata); err != nil {
			return err
		}
	case preparedDirectStartFallbackV4Format:
		if p.DirectVersion != 4 || p.Operation != directStartFallbackOperation || len(calldata) != directStartFallbackArgBytes ||
			p.DirectOrderID == 0 || binary.BigEndian.Uint64(calldata[:8]) != p.DirectOrderID {
			return errors.New("invalid v4 direct fallback start bytes")
		}
		if err := validateDirectV4Code(c, p.Contract, v4CodeHash); err != nil {
			return err
		}
		offer, order, err := directStartFallbackV4Ready(c, p.Contract, p.DirectOrderID, caller)
		if err != nil {
			return err
		}
		if order.OfferID != id || p.Author != caller || p.OrderNonce != order.Nonce || p.Price != offer.Price.String() {
			return errors.New("v4 direct fallback start differs from funded seller, offer, or nonce")
		}
		rate, err := readFeePerGas(c)
		if err != nil {
			return fmt.Errorf("read current direct seller stake fee: %w", err)
		}
		minimum, err := directIntentStakeForFee(offer.Price, rate)
		if err != nil {
			return err
		}
		maximum, err := directQuotedStartStakeForFee(offer.Price, rate)
		if err != nil {
			return err
		}
		stake, ok := new(big.Int).SetString(p.Amount, 10)
		if !ok || !isDecimal(p.Amount) || stake.String() != p.Amount ||
			stake.Cmp(minimum) < 0 || stake.Cmp(maximum) > 0 {
			return errors.New("v4 direct fallback stake is outside the current fee quote")
		}
		if err := validateDirectStartFallbackStatement(c, p, offer, calldata); err != nil {
			return err
		}
	case preparedDirectStartFallbackV5Format:
		if p.DirectVersion != 5 || p.Operation != directStartFallbackOperation || p.DirectOrderID == 0 {
			return errors.New("invalid v5 direct fallback start bytes")
		}
		offer, order, err := directStartFallbackV4Ready(c, p.Contract, p.DirectOrderID, caller)
		if err != nil {
			return err
		}
		if order.OfferID != id || p.Author != caller || p.OrderNonce != order.Nonce || p.Price != offer.Price.String() {
			return errors.New("v5 direct fallback start differs from funded seller, offer, or nonce")
		}
		rate, err := readFeePerGas(c)
		if err != nil {
			return fmt.Errorf("read current direct seller stake fee: %w", err)
		}
		minimum, err := directIntentStakeForFee(offer.Price, rate)
		if err != nil {
			return err
		}
		maximum, err := directQuotedStartStakeForFee(offer.Price, rate)
		if err != nil {
			return err
		}
		stake, ok := new(big.Int).SetString(p.Amount, 10)
		if !ok || !isDecimal(p.Amount) || stake.String() != p.Amount ||
			stake.Cmp(minimum) < 0 || stake.Cmp(maximum) > 0 {
			return errors.New("v5 direct fallback stake is outside the current fee quote")
		}
		if err := validateDirectStartFallbackV5Statement(c, p, offer, calldata); err != nil {
			return err
		}
	case preparedDirectFallbackFormat:
		if p.DirectVersion == 4 {
			if err := validateDirectV4Code(c, p.Contract, v4CodeHash); err != nil {
				return err
			}
		} else if p.DirectVersion == 5 {
			if err := validateDirectV5Code(c, p.Contract, v5CodeHash); err != nil {
				return err
			}
		} else {
			if p.DirectVersion != 0 && p.DirectVersion != 3 {
				return errors.New("unsupported direct fallback version")
			}
			if err := validateDirectV3Contract(c, p.Contract); err != nil {
				return err
			}
		}
		if p.Operation != directFallbackOperation || p.Amount != "0" || len(calldata) != directProveArgBytes ||
			p.DirectOrderID == 0 || binary.BigEndian.Uint64(calldata[:8]) != p.DirectOrderID {
			return errors.New("invalid direct fallback bytes")
		}
		offer, order, _, err := directFallbackReady(c, p.Contract, p.DirectOrderID)
		if err != nil {
			return err
		}
		if caller != hex.EncodeToString(offer.Author[:]) || p.Author != caller || order.OfferID != id {
			return errors.New("direct fallback signer is not this offer's author")
		}
		if p.DirectVersion == 4 || p.DirectVersion == 5 {
			if _, err := directPendingStakeV4(c, p.Contract, p.DirectOrderID, order); err != nil {
				return err
			}
		}
		if err := validateDirectFallbackEvidence(c, p, offer, order, calldata); err != nil {
			return err
		}
	case preparedDirectRefundFormat:
		if p.DirectVersion == 4 {
			if err := validateDirectV4Code(c, p.Contract, v4CodeHash); err != nil {
				return err
			}
		} else if p.DirectVersion == 5 {
			if err := validateDirectV5Code(c, p.Contract, v5CodeHash); err != nil {
				return err
			}
		} else {
			if p.DirectVersion != 0 && p.DirectVersion != 3 {
				return errors.New("unsupported direct refund version")
			}
			if err := validateDirectV3Contract(c, p.Contract); err != nil {
				return err
			}
		}
		if p.Amount != "0" || len(calldata) != directRefundArgBytes || p.DirectOrderID == 0 ||
			binary.BigEndian.Uint64(calldata) != p.DirectOrderID {
			return errors.New("invalid direct refund bytes")
		}
		offer, order, _, err := directActiveSale(c, p.Contract, p.DirectOrderID)
		if err != nil {
			return err
		}
		if order.OfferID != id {
			return errors.New("direct refund names another offer")
		}
		if p.DirectVersion == 4 || p.DirectVersion == 5 {
			if order.State == DirectFallbackIntent {
				return errors.New("v4/v5 direct order has removed fallback-intent state")
			}
			if order.State == DirectFallbackPending {
				if _, err := directPendingStakeV4(c, p.Contract, p.DirectOrderID, order); err != nil {
					return err
				}
			}
		}
		head, _, err := registryHead(c)
		if err != nil {
			return err
		}
		var intent *DirectIntent
		if order.State == DirectFallbackIntent {
			intent, err = directIncludedIntent(c, p.Contract, p.DirectOrderID, offer, order, head)
			if err != nil {
				return err
			}
		}
		refundAt, ok := order.RefundAt(intent)
		if !ok || !order.CanRefund(head, intent) {
			return fmt.Errorf("direct order is refundable only from block %d", refundAt)
		}
	default:
		return errors.New("unsupported direct operation")
	}
	p.directValidated = true
	return nil
}

func newBigFromBytes(value []byte) string { return new(big.Int).SetBytes(value).String() }

func cryptoUnmarshalDirectKey(raw []byte) (*ecdsa.PublicKey, error) {
	pub, err := crypto.UnmarshalPubkey(raw)
	if err != nil {
		return nil, errors.New("direct delivery key is not a secp256k1 point")
	}
	return pub, nil
}

func validateDirectFallbackEvidence(c *client, p *preparedAction, offer *DirectOffer, order *DirectOrder, calldata []byte) error {
	if order.State != DirectFallbackPending {
		return errors.New("direct fallback requires an included fallback-pending order")
	}
	head, _, err := registryHead(c)
	if err != nil {
		return err
	}
	// At least one block of inclusion margin is required. This still cannot
	// guarantee inclusion before the exclusive deadline.
	if head >= DirectProofDeadline(order.FundedAt)-1 {
		return errors.New("direct fallback has no safe inclusion margin")
	}
	return validateDirectFallbackReveal(p, offer, calldata)
}

func validateDirectFallbackReveal(p *preparedAction, offer *DirectOffer, calldata []byte) error {
	if len(calldata) != directProveArgBytes {
		return errors.New("direct fallback reveal argument has wrong length")
	}
	secrets := calldata[8:]
	if !directSecretsMatch(secrets, offer) || p.KeyCommitment != hex.EncodeToString(offer.Keys[:]) ||
		p.InventoryCommitment != hex.EncodeToString(offer.Inventory[:]) || p.Author != hex.EncodeToString(offer.Author[:]) ||
		p.Epoch != offer.Epoch || p.CoveredFlips != offer.Covered {
		return errors.New("direct fallback reveal and public keys differ from the funded offer")
	}
	return nil
}

func validateDirectStartFallbackStatement(c *client, p *preparedAction, offer *DirectOffer, calldata []byte) error {
	if len(calldata) != directStartFallbackArgBytes {
		return errors.New("direct fallback start proof argument has wrong length")
	}
	journal := calldata[8 : 8+directJournalBytes]
	seal := calldata[8+directJournalBytes : 8+directJournalBytes+directSealBytes]
	if binary.BigEndian.Uint32(journal[:4]) != registrationStatementVersion ||
		binary.BigEndian.Uint32(journal[registrationFlagsOffset:registrationFlagsOffset+4])&^uint32(0b111) != 0 ||
		p.KeyCommitment != hex.EncodeToString(offer.Keys[:]) ||
		p.InventoryCommitment != hex.EncodeToString(offer.Inventory[:]) || p.Author != hex.EncodeToString(offer.Author[:]) ||
		p.Epoch != offer.Epoch || p.CoveredFlips != offer.Covered ||
		hex.EncodeToString(journal[registrationContractOffset:registrationContractOffset+addressBytes]) != p.Contract ||
		hex.EncodeToString(journal[registrationAuthorOffset:registrationAuthorOffset+addressBytes]) != p.Author ||
		binary.BigEndian.Uint16(journal[registrationEpochOffset:registrationEpochOffset+2]) != p.Epoch ||
		binary.BigEndian.Uint32(journal[registrationCoveredOffset:registrationCoveredOffset+4]) != p.CoveredFlips ||
		hex.EncodeToString(journal[registrationManifestOffset:registrationManifestOffset+32]) != p.ManifestCommitment ||
		hex.EncodeToString(journal[registrationCommitmentOffset:registrationCommitmentOffset+32]) != p.KeyCommitment ||
		hex.EncodeToString(journal[registrationInventoryOffset:registrationInventoryOffset+32]) != p.InventoryCommitment ||
		hex.EncodeToString(journal[registrationPolicyOffset:registrationPolicyOffset+32]) != p.PolicyDigest ||
		hex.EncodeToString(journal[registrationConsensusOffset:registrationConsensusOffset+32]) != p.ConsensusDigest ||
		hex.EncodeToString(journal[registrationNetworkOffset:registrationNetworkOffset+32]) != p.NetworkDigest ||
		p.PolicyDigest != labelDigest(EligibilityPolicyV1) ||
		p.ConsensusDigest != labelDigest(ConsensusVersionPinned) ||
		p.NetworkDigest != labelDigest(NetworkNamePinned) ||
		p.ImageID != directV2ImageIDHex {
		return errors.New("direct fallback start proof and public keys differ from the funded offer")
	}
	claim, err := claimDigestFromHex(directV2ImageIDHex, journal)
	if err != nil || p.ClaimDigest != hex.EncodeToString(claim[:]) || len(seal) != directSealBytes {
		return errors.New("direct fallback start receipt metadata mismatch")
	}
	for _, at := range []int{registrationInventoryHeightOffset, registrationVerdictHeightOffset} {
		height := binary.BigEndian.Uint64(journal[at : at+8])
		if height == 0 {
			return errors.New("direct fallback has no canonical session height")
		}
		var block rpcBlock
		if err := c.call("bcn_blockAt", []any{height}, &block); err != nil {
			return err
		}
		root := hex.EncodeToString(journal[at+8 : at+40])
		if block.Height != height || !strings.EqualFold(strings.TrimPrefix(block.Root, "0x"), root) {
			return errors.New("direct fallback receipt root differs from canonical header")
		}
	}
	return nil
}
