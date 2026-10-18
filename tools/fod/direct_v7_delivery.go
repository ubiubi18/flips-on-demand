package main

// V7 private delivery is an encrypted off-chain message. The buyer verifies
// retained original blobs before issuing a receipt that can authorize confirm.

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
)

const directV7DMVersion = 7

func writeNewPoolV7PrivateFile(path string, data []byte) error {
	return writePrivateFile(path, false, func(file *os.File) error {
		_, err := file.Write(data)
		return err
	})
}

type poolV7DeliveryContext struct {
	contract string
	id       [32]byte
	orderID  uint64
	offer    *poolV7Offer
	order    *poolV7Order
}

func readPoolV7FundedDelivery(c *client, contract string, orderID uint64) (*poolV7DeliveryContext, error) {
	if c == nil || orderID == 0 {
		return nil, errors.New("v7 delivery needs a funded order")
	}
	order, err := readPoolV7Order(c, contract, orderID)
	if err != nil {
		return nil, err
	}
	offer, err := readPoolV7Offer(c, contract, order.offer)
	if err != nil {
		return nil, err
	}
	if order.state != 1 || offer.status != 3 || offer.publicSeed || offer.openOrder != orderID ||
		offer.nextNonce == 0 || order.nonce == 0 || offer.nextNonce-1 != order.nonce ||
		order.fundedAt == 0 || order.paid.Sign() <= 0 || order.bond.Sign() <= 0 ||
		offer.sellerBond.Sign() <= 0 ||
		(offer.everSold && order.sellerPayout.Cmp(newBigHalf(order.paid)) != 0) ||
		(!offer.everSold && order.sellerPayout.Cmp(order.paid) != 0) {
		return nil, errors.New("v7 offer and order are not one funded private sale")
	}
	if order.deliveryKey[0] != 4 {
		return nil, errors.New("v7 funded order has no uncompressed buyer delivery key")
	}
	if _, err := crypto.UnmarshalPubkey(order.deliveryKey[:]); err != nil {
		return nil, errors.New("v7 funded order has an invalid buyer delivery key")
	}
	return &poolV7DeliveryContext{contract: contract, id: order.offer, orderID: orderID, offer: offer, order: order}, nil
}

func newBigHalf(value *big.Int) *big.Int { return new(big.Int).Rsh(new(big.Int).Set(value), 1) }

func validatePoolV7DeliveryEnvelope(envelope *DirectDMEnvelope, context *poolV7DeliveryContext) ([]byte, error) {
	if envelope == nil || context == nil || context.offer == nil || context.order == nil ||
		envelope.Format != directDMFormat || envelope.Version != directV7DMVersion ||
		envelope.Network != NetworkNamePinned || envelope.Contract != context.contract ||
		envelope.Author != hex.EncodeToString(context.offer.author[:]) ||
		envelope.OfferID != hex.EncodeToString(context.id[:]) ||
		envelope.OrderID != context.orderID || envelope.OrderNonce != context.order.nonce ||
		envelope.Buyer != hex.EncodeToString(context.order.buyer[:]) ||
		envelope.DeliveryKey != hex.EncodeToString(context.order.deliveryKey[:]) {
		return nil, errors.New("v7 private delivery differs from the funded chain order")
	}
	if len(envelope.Ciphertext) != directDMCiphertextBytes*2 ||
		envelope.Ciphertext != strings.ToLower(envelope.Ciphertext) {
		return nil, errors.New("v7 private delivery has invalid ciphertext size or form")
	}
	ciphertext, err := hex.DecodeString(envelope.Ciphertext)
	if err != nil || ciphertext[0] != 4 {
		return nil, errors.New("v7 private delivery ciphertext is not canonical ECIES")
	}
	if _, err := crypto.UnmarshalPubkey(ciphertext[:deliveryKeyBytes]); err != nil {
		return nil, errors.New("v7 private delivery has an invalid ephemeral point")
	}
	return ciphertext, nil
}

func preparePoolV7PrivateMessage(c *client, contract string, orderID uint64, secrets []byte, caller string) (string, error) {
	context, err := readPoolV7FundedDelivery(c, contract, orderID)
	if err != nil {
		return "", err
	}
	if caller != hex.EncodeToString(context.offer.author[:]) || !poolV7SecretsMatch(secrets, context.offer) {
		return "", errors.New("v7 delivery needs the funded seller and committed epoch keys")
	}
	pub, err := crypto.UnmarshalPubkey(context.order.deliveryKey[:])
	if err != nil {
		return "", err
	}
	ciphertext, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pub), secrets, nil, nil)
	if err != nil {
		return "", err
	}
	envelope := &DirectDMEnvelope{
		Format: directDMFormat, Version: directV7DMVersion, Network: NetworkNamePinned,
		Contract: contract, Author: caller, OfferID: hex.EncodeToString(context.id[:]),
		OrderID: orderID, OrderNonce: context.order.nonce,
		Buyer:       hex.EncodeToString(context.order.buyer[:]),
		DeliveryKey: hex.EncodeToString(context.order.deliveryKey[:]),
		Ciphertext:  hex.EncodeToString(ciphertext),
	}
	if _, err := validatePoolV7DeliveryEnvelope(envelope, context); err != nil {
		return "", err
	}
	raw, err := json.Marshal(envelope)
	if err != nil || len(raw) > directDMMaxTextBytes {
		return "", errors.New("v7 private delivery exceeds the message limit")
	}
	return string(raw), nil
}

func verifyPoolV7PrivateDelivery(c *client, contract string, orderID uint64, message string, purchase *preparedAction, deliveryKey *ecdsa.PrivateKey) (*DirectOpenedDelivery, VaultPayload, error) {
	context, err := readPoolV7FundedDelivery(c, contract, orderID)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	if purchase == nil || purchase.Format != preparedPoolV7Buy || purchase.DirectVersion != 7 ||
		purchase.Contract != contract || purchase.DirectOfferID != hex.EncodeToString(context.id[:]) ||
		purchase.Buyer != hex.EncodeToString(context.order.buyer[:]) ||
		purchase.DeliveryKey != hex.EncodeToString(context.order.deliveryKey[:]) ||
		purchase.OrderNonce != context.order.nonce || purchase.Author != hex.EncodeToString(context.offer.author[:]) ||
		purchase.Epoch != context.offer.epoch || purchase.CoveredFlips != context.offer.covered ||
		purchase.InventoryCommitment != hex.EncodeToString(context.offer.inventory[:]) ||
		purchase.KeyCommitment != hex.EncodeToString(context.offer.keys[:]) ||
		purchase.Price != context.order.paid.String() ||
		purchase.Amount != new(big.Int).Add(context.order.paid, context.order.bond).String() {
		return nil, VaultPayload{}, errors.New("retained v7 purchase differs from the funded order")
	}
	if purchase.DirectSession == nil || purchase.DirectSession.Author != purchase.Author ||
		purchase.DirectSession.Epoch != purchase.Epoch ||
		uint32(len(purchase.DirectSession.Manifest)) != purchase.CoveredFlips ||
		inventoryCommitment(purchase.Author, purchase.Epoch, purchase.DirectSession.Manifest) != purchase.InventoryCommitment {
		return nil, VaultPayload{}, errors.New("v7 purchase has no canonical original inventory")
	}
	if err := directVerifiedSession(c, purchase.DirectSession); err != nil {
		return nil, VaultPayload{}, err
	}
	if err := requireRetainedInventory(purchase.Availability); err != nil {
		return nil, VaultPayload{}, err
	}
	if purchase.Availability.Author != purchase.Author || purchase.Availability.Epoch != purchase.Epoch ||
		purchase.Availability.CoveredFlips != int(purchase.CoveredFlips) ||
		purchase.Availability.InventoryCommitment != purchase.InventoryCommitment {
		return nil, VaultPayload{}, errors.New("v7 retained flips differ from the canonical purchase")
	}
	if deliveryKey == nil || hex.EncodeToString(crypto.FromECDSAPub(&deliveryKey.PublicKey)) != purchase.DeliveryKey {
		return nil, VaultPayload{}, errors.New("v7 delivery key does not belong to this funded buyer")
	}
	if len(message) == 0 || len(message) > directDMMaxTextBytes || directDMFields([]byte(message)) != nil {
		return nil, VaultPayload{}, errors.New("v7 private delivery message is malformed")
	}
	var envelope DirectDMEnvelope
	if err := decodeStrict([]byte(message), &envelope); err != nil {
		return nil, VaultPayload{}, err
	}
	ciphertext, err := validatePoolV7DeliveryEnvelope(&envelope, context)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	secrets, err := ecies.ImportECDSA(deliveryKey).Decrypt(ciphertext, nil, nil)
	if err != nil || !poolV7SecretsMatch(secrets, context.offer) {
		return nil, VaultPayload{}, errors.New("v7 delivered keys do not open the funded commitment")
	}
	keys, vault, err := epochKeysFromDeliveredSecrets(secrets)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	for _, retained := range purchase.Availability.Blobs {
		blob, _ := hex.DecodeString(retained.Hex)
		pub, publicCipher, privateCipher, err := ParseFlipBlob(blob)
		if err != nil {
			return nil, VaultPayload{}, err
		}
		authorKey, err := crypto.UnmarshalPubkey(pub)
		if err != nil || hex.EncodeToString(crypto.PubkeyToAddress(*authorKey).Bytes()) != purchase.Author {
			return nil, VaultPayload{}, errors.New("v7 original flip names another author")
		}
		public, err := keys.DecryptPublicPart(publicCipher)
		if err != nil {
			return nil, VaultPayload{}, errors.New("v7 original flip public part does not open")
		}
		private, err := keys.DecryptPrivatePart(privateCipher)
		if err != nil {
			return nil, VaultPayload{}, errors.New("v7 original flip private part does not open")
		}
		if err := validateDirectFlipParts(public, private); err != nil {
			return nil, VaultPayload{}, fmt.Errorf("v7 original flip %s is not usable: %w", retained.CID, err)
		}
	}
	record := &DirectOpenedDelivery{
		Format: directOpenedFormat, Version: 1, Contract: contract,
		OfferID: purchase.DirectOfferID, OrderID: orderID, OrderNonce: context.order.nonce,
		Buyer: purchase.Buyer, DeliveryKey: purchase.DeliveryKey,
		Author: purchase.Author, Epoch: purchase.Epoch,
		InventoryCommitment: purchase.InventoryCommitment, KeyCommitment: purchase.KeyCommitment,
		Ciphertext: envelope.Ciphertext, UsableFlips: len(purchase.Availability.Blobs),
	}
	return record, vault, nil
}

func preparePoolV7Confirm(c *client, contract string, orderID uint64, record *DirectOpenedDelivery) (*preparedAction, error) {
	if err := verifyDirectOpened(record); err != nil {
		return nil, err
	}
	context, err := readPoolV7FundedDelivery(c, contract, orderID)
	if err != nil {
		return nil, err
	}
	if record.Contract != contract || record.OfferID != hex.EncodeToString(context.id[:]) ||
		record.OrderID != orderID || record.OrderNonce != context.order.nonce ||
		record.Buyer != hex.EncodeToString(context.order.buyer[:]) ||
		record.DeliveryKey != hex.EncodeToString(context.order.deliveryKey[:]) ||
		record.Author != hex.EncodeToString(context.offer.author[:]) ||
		record.Epoch != context.offer.epoch || record.UsableFlips != int(context.offer.covered) ||
		record.InventoryCommitment != hex.EncodeToString(context.offer.inventory[:]) ||
		record.KeyCommitment != hex.EncodeToString(context.offer.keys[:]) {
		return nil, errors.New("v7 usable delivery receipt differs from funded order")
	}
	ciphertext, err := hex.DecodeString(record.Ciphertext)
	if err != nil {
		return nil, errors.New("v7 delivery receipt has invalid ciphertext")
	}
	calldata, err := encodePoolV7Confirm(orderID, ciphertext)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(ciphertext)
	return &preparedAction{
		Format: preparedPoolV7Confirm, Version: actionRequestVersion, Operation: "confirm",
		Network: NetworkNamePinned, Contract: contract, Amount: "0", Calldata: hex.EncodeToString(calldata),
		DirectVersion: 7, DirectOfferID: record.OfferID, DirectOrderID: orderID,
		Author: record.Author, Buyer: record.Buyer, Epoch: record.Epoch,
		CoveredFlips: uint32(record.UsableFlips), OrderNonce: record.OrderNonce,
		InventoryCommitment: record.InventoryCommitment, KeyCommitment: record.KeyCommitment,
		CiphertextSha256: hex.EncodeToString(digest[:]), DirectOpened: record,
	}, nil
}
