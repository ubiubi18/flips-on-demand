package main

// A v5 fallback uses a pre-session content seal and the later four public
// state leaves. The long proof is verified locally before a payable call is
// prepared; the contract independently verifies both parts again.

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const preparedDirectStartFallbackV5Format = "fod.prepared-direct-start-fallback-v5"
const fastContentV5ImageIDHex = "4fd124c8cb91370dab60d3e1d7c4e666e79485e210b5f20ed36cc2104883726c"

func prepareDirectStartFallbackV5(c *client, contract string, orderID uint64, caller, proofPath, sessionPath, hostPath string, secrets []byte) (*preparedAction, error) {
	offer, order, err := directStartFallbackV4Ready(c, contract, orderID, caller)
	if err != nil {
		return nil, err
	}
	if !directSecretsMatch(secrets, offer) {
		return nil, errors.New("fallback vault differs from funded offer")
	}
	var proof fastContentArtefact
	if err := readFastPublicJSON(proofPath, &proof); err != nil {
		return nil, err
	}
	session, err := readVerifiedFastSession(sessionPath)
	if err != nil {
		return nil, err
	}
	if err := directVerifiedSession(c, session); err != nil {
		return nil, err
	}
	registration, err := assembleFastRegistration(NetworkNamePinned, contract, &proof, session, fastContentV5ImageIDHex)
	if err != nil {
		return nil, err
	}
	if registration.Author != caller || registration.Epoch != offer.Epoch || registration.CoveredFlips != offer.Covered ||
		registration.InventoryCommitment != hex.EncodeToString(offer.Inventory[:]) {
		return nil, errors.New("sealed content and canonical session differ from the funded offer")
	}
	proofBytes, err := hex.DecodeString(registration.Calldata)
	if err != nil || len(proofBytes) < fastContentJournalBytes+fastContentSealBytes+2 {
		return nil, errors.New("invalid sealed content and session payload")
	}
	journal := proofBytes[:fastContentJournalBytes]
	seal := proofBytes[fastContentJournalBytes : fastContentJournalBytes+fastContentSealBytes]
	wire := proofBytes[fastContentJournalBytes+fastContentSealBytes+2:]
	if binary.BigEndian.Uint16(proofBytes[fastContentJournalBytes+fastContentSealBytes:]) != uint16(len(wire)) ||
		hex.EncodeToString(journal[241:273]) != hex.EncodeToString(offer.Keys[:]) {
		return nil, errors.New("sealed content keys or session wire differ from the funded offer")
	}
	if err := verifyDirectV5FastHost(proofPath, hostPath, contract, fastContentV5ImageIDHex); err != nil {
		return nil, err
	}
	arg, err := EncodeDirectStartFallbackV5(orderID, journal, seal, wire)
	if err != nil {
		return nil, err
	}
	rate, err := readFeePerGas(c)
	if err != nil {
		return nil, fmt.Errorf("read current direct seller stake fee: %w", err)
	}
	stake, err := directQuotedStartStakeForFee(offer.Price, rate)
	if err != nil {
		return nil, err
	}
	p := &preparedAction{
		Format: preparedDirectStartFallbackV5Format, Version: actionRequestVersion, DirectVersion: 5,
		Operation: directStartFallbackOperation, Network: NetworkNamePinned, Contract: contract,
		Amount: stake.String(), Calldata: hex.EncodeToString(arg), DirectOfferID: hex.EncodeToString(order.OfferID[:]),
		DirectOrderID: orderID, OrderNonce: order.Nonce, Price: offer.Price.String(), Author: caller,
		Epoch: offer.Epoch, CoveredFlips: offer.Covered, KeyCommitment: hex.EncodeToString(offer.Keys[:]),
		InventoryCommitment: registration.InventoryCommitment, ImageID: registration.ImageID,
		ClaimDigest: registration.ClaimDigest, DirectSession: session,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := validateDirectStartFallbackV5Statement(c, p, offer, arg); err != nil {
		return nil, err
	}
	return p, nil
}

func validateDirectStartFallbackV5Statement(c *client, p *preparedAction, offer *DirectOffer, calldata []byte) error {
	const headerBytes = 8 + fastContentJournalBytes + fastContentSealBytes + 2
	if p == nil || offer == nil || p.DirectSession == nil || p.DirectVersion != 5 ||
		p.Format != preparedDirectStartFallbackV5Format || p.Operation != directStartFallbackOperation ||
		p.DirectOrderID == 0 || len(calldata) <= headerBytes || binary.BigEndian.Uint64(calldata[:8]) != p.DirectOrderID {
		return errors.New("invalid v5 fallback start context")
	}
	wire := calldata[headerBytes:]
	if len(wire) > fastRegistrationMaxWireBytes ||
		binary.BigEndian.Uint16(calldata[headerBytes-2:headerBytes]) != uint16(len(wire)) {
		return errors.New("v5 fallback session wire has the wrong length")
	}
	if err := directVerifiedSession(c, p.DirectSession); err != nil {
		return err
	}
	expectedWire, err := encodeFastRegistrationWire(p.DirectSession)
	if err != nil || !bytes.Equal(wire, expectedWire) {
		return errors.New("v5 fallback session wire differs from canonical state leaves")
	}
	journal := calldata[8 : 8+fastContentJournalBytes]
	claim, err := claimDigestFromHex(fastContentV5ImageIDHex, journal)
	if err != nil || !strings.EqualFold(p.ImageID, fastContentV5ImageIDHex) ||
		!strings.EqualFold(p.ClaimDigest, hex.EncodeToString(claim[:])) ||
		binary.BigEndian.Uint32(journal[:4]) != 1 ||
		hex.EncodeToString(journal[4:36]) != labelDigest(NetworkNamePinned) ||
		hex.EncodeToString(journal[36:56]) != p.Contract ||
		hex.EncodeToString(journal[56:76]) != p.Author ||
		binary.BigEndian.Uint16(journal[76:78]) != p.Epoch ||
		uint32(binary.BigEndian.Uint16(journal[78:80])) != p.CoveredFlips ||
		hex.EncodeToString(journal[80:112]) != labelDigest(ConsensusVersionPinned) ||
		hex.EncodeToString(journal[112:144]) != labelDigest(EligibilityPolicyV1) ||
		hex.EncodeToString(journal[144:176]) != p.InventoryCommitment ||
		hex.EncodeToString(journal[241:273]) != p.KeyCommitment ||
		p.Author != hex.EncodeToString(offer.Author[:]) || p.Epoch != offer.Epoch ||
		p.CoveredFlips != offer.Covered || p.InventoryCommitment != hex.EncodeToString(offer.Inventory[:]) ||
		p.KeyCommitment != hex.EncodeToString(offer.Keys[:]) ||
		p.InventoryCommitment != inventoryCommitment(p.Author, p.Epoch, p.DirectSession.Manifest) {
		return errors.New("v5 fallback journal differs from the pinned image, funded offer, or original flips")
	}
	return nil
}
