package main

// Prepare the public v6 qualification and independent content proof. Neither
// prepared format is accepted by submit; estimation only simulates a call.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	preparedDirectV6PoolQualifyFormat = "fod.prepared-direct-v6-pool-qualify"
	preparedDirectV6PoolProveFormat   = "fod.prepared-direct-v6-pool-prove-bundle"
)

func isDirectV6PoolPrepareMode(mode string) bool {
	return mode == "direct-pool-qualify" || mode == "direct-pool-prove-bundle"
}

func prepareDirectV6PoolCall(c *client, contract, sender, mode string, proof *fastContentArtefact, session *SessionBundle) (*preparedAction, error) {
	if !isDirectV6PoolPrepareMode(mode) || session == nil || sender != session.Author {
		return nil, errors.New("v6 pool preparation requires the active session author")
	}
	if err := directVerifiedSession(c, session); err != nil {
		return nil, err
	}
	registration, err := assembleFastRegistration(NetworkNamePinned, contract, proof, session, fastContentV5ImageIDHex)
	if err != nil {
		return nil, err
	}
	if registration.Author != sender || registration.Epoch != session.Epoch || registration.CoveredFlips != uint32(len(session.Manifest)) {
		return nil, errors.New("v6 pool proof and session belong to different authors or epochs")
	}
	proofBytes, err := hex.DecodeString(registration.Calldata)
	if err != nil || len(proofBytes) < fastContentJournalBytes+fastContentSealBytes+2 {
		return nil, errors.New("invalid v6 pool sealed proof")
	}
	journal := proofBytes[:fastContentJournalBytes]
	seal := proofBytes[fastContentJournalBytes : fastContentJournalBytes+fastContentSealBytes]
	wire := proofBytes[fastContentJournalBytes+fastContentSealBytes+2:]
	var author [20]byte
	var inventory, keys [32]byte
	copy(author[:], journal[56:76])
	copy(inventory[:], journal[144:176])
	copy(keys[:], journal[241:273])
	id := poolV6OfferID(author, session.Epoch)
	var calldata []byte
	format, operation := preparedDirectV6PoolQualifyFormat, "qualify"
	if mode == "direct-pool-qualify" {
		calldata, err = EncodeDirectV6PoolQualify(DirectV6PoolQualifyArgs{
			Epoch: session.Epoch, Covered: uint32(len(session.Manifest)), Inventory: inventory, Keys: keys,
		}, wire)
	} else {
		format, operation = preparedDirectV6PoolProveFormat, "proveBundle"
		calldata, err = EncodeDirectV6PoolProveBundle(id, journal, seal, wire)
	}
	if err != nil {
		return nil, err
	}
	return &preparedAction{
		Format: format, Version: actionRequestVersion, Operation: operation,
		Network: NetworkNamePinned, Contract: contract, DirectVersion: 6,
		Amount: "0", Calldata: hex.EncodeToString(calldata), DirectOfferID: hex.EncodeToString(id[:]),
		Author: sender, Epoch: session.Epoch, CoveredFlips: uint32(len(session.Manifest)),
		InventoryCommitment: hex.EncodeToString(inventory[:]), KeyCommitment: hex.EncodeToString(keys[:]),
		ImageID: registration.ImageID, ClaimDigest: registration.ClaimDigest,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func prepareDirectV6PoolFromFiles(o directCommandOptions, contract string) error {
	if o.SessionPath == "" || o.ProofPath == "" || o.ProofHostPath == "" || o.PreparedPath == "" || o.MaxFeeText == "" {
		return errors.New("v6 pool preparation needs -session -proof -registration-host -prepared -max-fee")
	}
	maxFee, ok := new(big.Int).SetString(o.MaxFeeText, 10)
	if !ok || maxFee.Sign() <= 0 || !isDecimal(o.MaxFeeText) {
		return errors.New("v6 pool estimation needs a positive decimal -max-fee")
	}
	session, err := readVerifiedFastSession(o.SessionPath)
	if err != nil {
		return err
	}
	var activeAddress string
	if err := o.Node.call("dna_getCoinbaseAddr", []any{}, &activeAddress); err != nil {
		return fmt.Errorf("read active v6 author: %w", err)
	}
	sender, err := directAddress(activeAddress)
	if err != nil || sender != session.Author {
		return errors.New("active node identity differs from the canonical session author")
	}
	var proof fastContentArtefact
	if err := readFastPublicJSON(o.ProofPath, &proof); err != nil {
		return err
	}
	if err := verifyDirectV5FastHost(o.ProofPath, o.ProofHostPath, contract, fastContentV5ImageIDHex); err != nil {
		return err
	}
	prepared, err := prepareDirectV6PoolCall(o.Node, contract, sender, o.Mode, &proof, session)
	if err != nil {
		return err
	}
	rate, err := readFeePerGas(o.Node)
	if err != nil {
		return fmt.Errorf("read current v6 pool fee: %w", err)
	}
	budget, _, err := estimateDirectV6PoolCall(o.Node, prepared, sender, maxFee, rate)
	if err != nil {
		return err
	}
	if err := writePrivateJSON(o.PreparedPath, prepared); err != nil {
		return err
	}
	minimum := new(big.Int).Mul(new(big.Int).SetUint64(budget), rate)
	fmt.Printf("v6 %s prepared for author %s and offer %s; read-only estimate %d gas, minimum max fee %s base units; no transaction sent\n",
		prepared.Operation, sender, prepared.DirectOfferID, budget, minimum.String())
	return nil
}

func validateDirectV6PoolPrepareOptions(o directCommandOptions) error {
	if o.DirectVersion != 6 || strings.ToLower(strings.TrimPrefix(o.CodeHash, "0x")) != directV6CodeHashHex {
		return errors.New("v6 pool preparation needs -direct-version 6 and the pinned v6 -code-hash")
	}
	if o.Amount != "" && o.Amount != "0" {
		return errors.New("v6 qualify and proveBundle must carry zero amount")
	}
	if o.Broadcast || o.AutoConfirm {
		return errors.New("v6 pool preparation only estimates; it cannot broadcast")
	}
	if o.NodeKeyPath != "" || o.SignedPath != "" {
		return errors.New("v6 pool preparation does not read a nodekey or write a signed transaction")
	}
	if o.Node == nil {
		return errors.New("v6 pool preparation needs a node RPC")
	}
	if !directLocalRPC(o.Node.url) {
		return errors.New("v6 pool preparation needs a loopback node RPC")
	}
	return nil
}
