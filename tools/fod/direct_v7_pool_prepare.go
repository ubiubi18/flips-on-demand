package main

// v7 preparation reads only the loopback node and local evidence. It writes a
// typed unsigned action; submit rechecks the same terms before signing.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const poolV7LegacyContentContract = "92f664db85805d707016febbfa55bd2beaaf808b"

func poolV7ProofBindingAllowed(proofContract, contract string, epoch uint16, operation string) bool {
	return proofContract == contract ||
		(proofContract == poolV7LegacyContentContract && epoch == 230 &&
			(operation == "qualify" || operation == "proveBundle"))
}

func preparePoolV7FromFiles(o directCommandOptions, contract string) error {
	if o.DirectVersion != 7 || o.Node == nil || !directLocalRPC(o.Node.url) ||
		o.PreparedPath == "" || o.MaxFeeText == "" || !isHex(strings.ToLower(strings.TrimPrefix(o.CodeHash, "0x")), 32) ||
		o.Broadcast || o.AutoConfirm || o.NodeKeyPath != "" || o.SignedPath != "" {
		return errors.New("v7 preparation needs -direct-version 7, loopback RPC, -contract, -code-hash, -prepared, and -max-fee; no signing or broadcast flags")
	}
	codeHash := strings.ToLower(strings.TrimPrefix(o.CodeHash, "0x"))
	maxFee, ok := new(big.Int).SetString(o.MaxFeeText, 10)
	if !ok || !isDecimal(o.MaxFeeText) || maxFee.Sign() <= 0 {
		return errors.New("v7 estimation needs a positive decimal -max-fee")
	}
	if err := checkBuyTarget(o.Node, contract, contract, codeHash); err != nil {
		return err
	}
	var active string
	if err := o.Node.call("dna_getCoinbaseAddr", []any{}, &active); err != nil {
		return fmt.Errorf("read active v7 identity: %w", err)
	}
	caller, err := directAddress(active)
	if err != nil {
		return err
	}
	format := ""
	for candidate, method := range poolV7Formats {
		if method == o.Operation {
			format = candidate
			break
		}
	}
	if format == "" {
		return errors.New("unsupported v7 operation")
	}
	p := &preparedAction{
		Format: format, Version: actionRequestVersion, Operation: o.Operation,
		Network: NetworkNamePinned, Contract: contract, CodeHash: codeHash, DirectVersion: 7,
		Amount: "0", PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}
	var id [32]byte
	if o.OfferID != "" {
		if !isHex(o.OfferID, 32) || o.OfferID != strings.ToLower(o.OfferID) {
			return errors.New("v7 -v7-offer-id must be canonical 32-byte hex")
		}
		decoded, _ := hex.DecodeString(o.OfferID)
		copy(id[:], decoded)
	}
	var calldata, listingSecrets []byte
	switch o.Operation {
	case "qualify", "proveBundle":
		if o.SessionPath == "" || o.ProofPath == "" || o.ProofHostPath == "" {
			return errors.New("v7 qualify/proveBundle needs -session -proof -registration-host")
		}
		session, err := readVerifiedFastSession(o.SessionPath)
		if err != nil {
			return err
		}
		if err := directVerifiedSession(o.Node, session); err != nil {
			return err
		}
		var proof fastContentArtefact
		if err := readFastPublicJSON(o.ProofPath, &proof); err != nil {
			return err
		}
		journal, err := hex.DecodeString(proof.Journal)
		if err != nil || len(journal) != fastContentJournalBytes {
			return errors.New("invalid v7 content journal")
		}
		proofContract := hex.EncodeToString(journal[36:56])
		if !poolV7ProofBindingAllowed(proofContract, contract, session.Epoch, o.Operation) {
			return errors.New("v7 proof is bound to another contract")
		}
		if err := verifyDirectV5FastHost(o.ProofPath, o.ProofHostPath, proofContract, fastContentV5ImageIDHex); err != nil {
			return err
		}
		registration, err := assembleFastRegistration(NetworkNamePinned, proofContract, &proof, session, fastContentV5ImageIDHex)
		if err != nil {
			return err
		}
		if o.Operation == "qualify" && caller != session.Author {
			return errors.New("v7 qualification needs the active session author")
		}
		var author [20]byte
		copy(author[:], journal[56:76])
		derived := poolV7OfferID(author, session.Epoch)
		if id != ([32]byte{}) && id != derived {
			return errors.New("v7 supplied offer ID differs from proof")
		}
		id = derived
		var inventory, keys [32]byte
		copy(inventory[:], journal[144:176])
		copy(keys[:], journal[241:273])
		wire, err := encodeFastRegistrationWire(session)
		if err != nil {
			return err
		}
		if o.Operation == "qualify" {
			calldata, err = encodePoolV7Qualify(session.Epoch, uint32(len(session.Manifest)), inventory, keys, wire)
		} else {
			seal, sealErr := hex.DecodeString(proof.Seal)
			if sealErr != nil {
				return sealErr
			}
			calldata, err = encodePoolV7ProveBundle(id, journal, seal, wire)
		}
		if err != nil {
			return err
		}
		p.Author, p.Epoch, p.CoveredFlips = session.Author, session.Epoch, uint32(len(session.Manifest))
		p.InventoryCommitment, p.KeyCommitment = hex.EncodeToString(inventory[:]), hex.EncodeToString(keys[:])
		p.DirectSession, p.ImageID, p.ClaimDigest = session, registration.ImageID, registration.ClaimDigest
	case "seedBundle", "list", "proveAndReveal":
		if o.VaultPath == "" {
			return errors.New("v7 seed/list/reveal needs -vault")
		}
		passphrase, err := readPassphrase("Vault passphrase: ")
		if err != nil {
			return err
		}
		secrets, _, err := directVaultSecrets(o.VaultPath, passphrase)
		if err != nil {
			return err
		}
		derived, _, err := poolV7SecretsOfferID(secrets)
		if err != nil {
			return err
		}
		if id != ([32]byte{}) && id != derived {
			return errors.New("v7 vault differs from selected offer")
		}
		id = derived
		if o.Operation == "list" {
			listingSecrets = secrets
		}
		if o.Operation == "seedBundle" {
			calldata, err = encodePoolV7SeedBundle(id, secrets)
		}
		if o.Operation == "proveAndReveal" {
			if o.OrderID == 0 {
				return errors.New("v7 proveAndReveal needs -order")
			}
			p.DirectOrderID = o.OrderID
			calldata, err = encodePoolV7ProveAndReveal(o.OrderID, secrets)
		}
		if err != nil {
			return err
		}
	case "activate", "buy":
		if id == ([32]byte{}) {
			return errors.New("v7 activate/buy needs -v7-offer-id")
		}
		if o.Operation == "activate" {
			calldata, err = encodePoolV7SeedID(id)
			p.Amount = poolV7SeedAmount
		} else {
			if o.DeliveryKeyPath == "" {
				return errors.New("v7 buy needs -delivery-key output path")
			}
			// Check the queue before creating the buyer's dedicated key file.
			primary, err := readPoolV7ID(o.Node, contract, "pool/primary-head")
			if err != nil {
				return err
			}
			oldest := primary
			if oldest == ([32]byte{}) {
				oldest, err = readPoolV7ID(o.Node, contract, "pool/secondary-head")
				if err != nil {
					return err
				}
			}
			if oldest != id {
				return errors.New("v7 buy target is not the oldest queued offer")
			}
			public, err := CreateDeliveryKey(o.DeliveryKeyPath)
			if err != nil {
				return err
			}
			key, _ := hex.DecodeString(public)
			calldata, err = encodePoolV7Buy(id, key)
			p.Buyer, p.DeliveryKey = caller, public
		}
		if err != nil {
			return err
		}
	case "challenge", "startFallback":
		if o.OrderID == 0 {
			return errors.New("v7 challenge/startFallback needs -order")
		}
		p.DirectOrderID = o.OrderID
		calldata, err = encodePoolV7OrderID(o.OrderID)
		if err != nil {
			return err
		}
		order, err := readPoolV7Order(o.Node, contract, o.OrderID)
		if err != nil {
			return err
		}
		if id != ([32]byte{}) && id != order.offer {
			return errors.New("v7 order differs from selected offer")
		}
		id = order.offer
		if o.Operation == "challenge" {
			p.Buyer = caller
		}
	default:
		return errors.New("unsupported v7 operation")
	}
	if id == ([32]byte{}) {
		return errors.New("v7 offer ID is zero")
	}
	p.DirectOfferID = hex.EncodeToString(id[:])
	if o.Operation != "qualify" {
		offer, err := readPoolV7Offer(o.Node, contract, id)
		if err != nil {
			return err
		}
		p.Author, p.Epoch, p.CoveredFlips = hex.EncodeToString(offer.author[:]), offer.epoch, offer.covered
		p.InventoryCommitment, p.KeyCommitment = hex.EncodeToString(offer.inventory[:]), hex.EncodeToString(offer.keys[:])
		if o.Operation == "list" {
			if o.SessionPath == "" {
				return errors.New("v7 list needs -session")
			}
			session, err := readVerifiedFastSession(o.SessionPath)
			if err != nil {
				return err
			}
			if err := directVerifiedSession(o.Node, session); err != nil {
				return err
			}
			if session.Author != p.Author || session.Epoch != p.Epoch ||
				inventoryCommitment(p.Author, p.Epoch, session.Manifest) != p.InventoryCommitment ||
				!poolV7SecretsMatch(listingSecrets, offer) {
				return errors.New("v7 listing session or vault differs from qualified offer")
			}
			minimumText := o.MinimumPayout
			if minimumText == "" {
				minimumText = "0"
			}
			minimum, err := poolV7ValidAmount(minimumText)
			if err != nil {
				return err
			}
			p.MinimumPayout = minimum.String()
			calldata, err = encodePoolV7List(offer.epoch, offer.covered, offer.inventory, offer.keys, minimum)
			if err != nil {
				return err
			}
			rate, err := readFeePerGas(o.Node)
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
			p.Amount = bond.String()
		}
		if o.Operation == "buy" {
			rate, err := readFeePerGas(o.Node)
			if err != nil {
				return err
			}
			bond, err := poolV7Bond(rate)
			if err != nil {
				return err
			}
			quote, err := poolV7CurrentQuote(o.Node, contract, false)
			if err != nil {
				return err
			}
			p.Price = quote.String()
			p.Amount = new(big.Int).Add(quote, bond).String()
		}
	}
	if o.Amount != "" && o.Amount != "0" && o.Amount != p.Amount {
		return errors.New("v7 -amount differs from the exact current method value")
	}
	p.Calldata = hex.EncodeToString(calldata)
	if err := validatePreparedPoolV7Caller(o.Node, p, caller); err != nil {
		return err
	}
	rate, err := readFeePerGas(o.Node)
	if err != nil {
		return err
	}
	budget, _, err := estimatePoolV7Call(o.Node, p, caller, maxFee, rate)
	if err != nil {
		return err
	}
	if err := writePrivateJSON(o.PreparedPath, p); err != nil {
		return err
	}
	fmt.Printf("v7 %s prepared for offer %s; read-only estimate %d gas; no transaction signed or sent\n", p.Operation, p.DirectOfferID, budget)
	return nil
}
