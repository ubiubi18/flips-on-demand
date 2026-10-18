package main

// These v7 methods are typed before simulation and signing. The node still
// executes each exact call against current state before a transaction is sent.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/big"
)

const poolV7HeadTimeout = uint64(30_240)

func poolV7OldestOffer(c *client, contract string) ([32]byte, error) {
	primary, err := readPoolV7ID(c, contract, "pool/primary-head")
	if err != nil || primary != ([32]byte{}) {
		return primary, err
	}
	return readPoolV7ID(c, contract, "pool/secondary-head")
}

func poolV7Queued(offer *poolV7Offer) bool {
	if offer == nil || offer.openOrder != 0 {
		return false
	}
	if offer.publicSeed {
		return offer.status == 2 && offer.contentProven
	}
	return offer.sellerBond.Sign() > 0 &&
		((offer.status == 1 && !offer.everSold) ||
			(offer.status == 2 && offer.everSold && offer.contentProven))
}

func validatePoolV7LifecycleCall(c *client, p *preparedAction, caller string, offer *poolV7Offer, id [32]byte, calldata []byte, amount *big.Int) error {
	if c == nil || p == nil || offer == nil || amount == nil {
		return errors.New("missing v7 lifecycle context")
	}
	orderID := p.DirectOrderID
	switch p.Operation {
	case "buyPublicSeed":
		if !bytes.Equal(calldata, id[:]) || !poolV7Queued(offer) || !offer.publicSeed ||
			!offer.contentProven || offer.sellerBond.Sign() != 0 || offer.fraudCollateral.Sign() != 0 {
			return errors.New("v7 public buy needs the queued proved seed")
		}
		seed, err := readPoolV7ID(c, p.Contract, "pool/seed-id")
		if err != nil || seed != id {
			return errors.New("v7 public buy differs from the pinned seed")
		}
		oldest, err := poolV7OldestOffer(c, p.Contract)
		if err != nil || oldest != id {
			return errors.New("v7 public seed is not the selectable head")
		}
		quote, err := poolV7CurrentQuote(c, p.Contract, false)
		if err != nil || amount.Cmp(quote) != 0 || p.Price != quote.String() {
			return errors.New("v7 public buy differs from the exact current quote")
		}
	case "relist":
		if caller != p.Author || offer.publicSeed || offer.openOrder != 0 ||
			(offer.status != 4 && offer.status != 5) || offer.sellerBond.Sign() != 0 ||
			offer.fraudCollateral.Sign() != 0 || (offer.everSold && !offer.contentProven) {
			return errors.New("v7 relist needs a privately sold or cancelled offer")
		}
		minimum, err := poolV7ValidAmount(p.MinimumPayout)
		if err != nil {
			return err
		}
		want, err := encodePoolV7Relist(id, minimum)
		if err != nil || !bytes.Equal(calldata, want) {
			return errors.New("v7 relist argument differs from the offer and minimum")
		}
		quote, err := poolV7CurrentQuote(c, p.Contract, true)
		if err != nil {
			return err
		}
		if offer.everSold {
			quote = newBigHalf(quote)
		}
		if minimum.Cmp(quote) > 0 {
			return errors.New("v7 relist minimum exceeds current seller quote")
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
			return errors.New("v7 relist value differs from current required bonds")
		}
	case "cancel":
		if amount.Sign() != 0 || !bytes.Equal(calldata, id[:]) || caller != p.Author ||
			offer.publicSeed || !poolV7Queued(offer) {
			return errors.New("v7 cancellation needs the author's queued private offer and zero value")
		}
	case "expireHead":
		if amount.Sign() != 0 || !bytes.Equal(calldata, id[:]) || !poolV7Queued(offer) {
			return errors.New("v7 head expiry needs a queued offer and zero value")
		}
		oldest, err := poolV7OldestOffer(c, p.Contract)
		if err != nil || oldest != id {
			return errors.New("v7 expiry target is not the selectable head")
		}
		reserve, err := readPoolV7U128(c, p.Contract, "pool/reserve")
		if err != nil || reserve.Sign() == 0 {
			return errors.New("v7 pool is not activated")
		}
		since, err := readPoolV7U64(c, p.Contract, "pool/head-since")
		if err != nil {
			return err
		}
		activated, err := readPoolV7U64(c, p.Contract, "pool/activated-at")
		if err != nil {
			return err
		}
		if activated > since {
			since = activated
		}
		head, _, err := registryHead(c)
		if err != nil || since > ^uint64(0)-poolV7HeadTimeout || head < since+poolV7HeadTimeout {
			return errors.New("v7 selectable head has not reached expiry")
		}
	case "topUpBond":
		if !bytes.Equal(calldata, id[:]) || caller != p.Author || offer.publicSeed ||
			!poolV7Queued(offer) || amount.Sign() <= 0 ||
			new(big.Int).Add(offer.sellerBond, amount).Cmp(maxContractAmount) > 0 {
			return errors.New("v7 bond top-up needs a queued private author and positive value")
		}
	case "refund", "slashUnproved":
		if amount.Sign() != 0 || orderID == 0 || len(calldata) != 8 ||
			binary.BigEndian.Uint64(calldata) != orderID {
			return errors.New("v7 lifecycle order call needs zero value and exact order ID")
		}
		order, err := readPoolV7Order(c, p.Contract, orderID)
		if err != nil || order.offer != id || order.nonce == 0 ||
			offer.nextNonce != order.nonce+1 || offer.openOrder != orderID ||
			offer.publicSeed || order.fundedAt == 0 {
			return errors.New("v7 lifecycle order differs from the active offer")
		}
		head, _, err := registryHead(c)
		if err != nil || order.fundedAt > ^uint64(0)-poolV7ProofWindow ||
			head < order.fundedAt+poolV7ProofWindow {
			return errors.New("v7 lifecycle order has not reached its proof deadline")
		}
		payout := order.paid
		if offer.everSold {
			payout = newBigHalf(order.paid)
		}
		if p.Operation == "refund" {
			if (order.state != 1 && order.state != 4) || offer.status != 3 ||
				offer.sellerBond.Sign() <= 0 || order.sellerPayout.Cmp(payout) != 0 {
				return errors.New("v7 refund needs a funded defaulted private order")
			}
		} else if order.state != 2 || offer.status != 4 || offer.contentProven ||
			offer.fraudCollateral.Sign() <= 0 {
			return errors.New("v7 slash needs settled unproved private content")
		}
	default:
		return errors.New("unsupported v7 lifecycle operation")
	}
	return nil
}
