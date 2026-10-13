package main

// Recovery from a settled direct sale reads only canonical public state and
// the buyer's retained original flips. No delivery message or signing key is
// needed after proveAndReveal.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/idena-network/idena-go/crypto"
)

const directRecoveredFormat = "fod.recovered-direct-delivery"

// DirectRecoveredDelivery records a local content check after public fallback.
// It does not carry the revealed secrets or authorize a confirmation call.
type DirectRecoveredDelivery struct {
	Format              string `json:"format"`
	Version             int    `json:"version"`
	Network             string `json:"network"`
	Contract            string `json:"contract"`
	CodeHash            string `json:"codeHash"`
	OfferID             string `json:"offerId"`
	OrderID             uint64 `json:"orderId"`
	OrderNonce          uint64 `json:"orderNonce"`
	Buyer               string `json:"buyer"`
	DeliveryKey         string `json:"deliveryKey"`
	Author              string `json:"author"`
	Epoch               uint16 `json:"epoch"`
	InventoryCommitment string `json:"inventoryCommitment"`
	KeyCommitment       string `json:"keyCommitment"`
	KeySource           string `json:"keySource"`
	UsableFlips         int    `json:"usableFlips"`
}

func recoverDirectReveal(c *client, contract string, orderID uint64, purchase *preparedAction, codeHash string) (*DirectRecoveredDelivery, VaultPayload, error) {
	contract, err := directAddress(contract)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	order, err := directOrderFromChain(c, contract, orderID)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	offer, err := directOfferFromChain(c, contract, order.OfferID)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	if order.State != DirectSettled || order.DeliveryHash != ([32]byte{}) ||
		!offer.Retired || offer.OpenOrderID != 0 || offer.NextNonce == 0 || order.Nonce == 0 ||
		order.Nonce+1 != offer.NextNonce || order.Paid.Cmp(offer.Price) != 0 ||
		offer.Covered == 0 || offer.Covered > 32 {
		return nil, VaultPayload{}, errors.New("direct order is not a settled public fallback for this offer")
	}
	total, err := directFundedAmount(order.Paid, order.Bond)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	offerID := hex.EncodeToString(order.OfferID[:])
	author := hex.EncodeToString(offer.Author[:])
	buyer := hex.EncodeToString(order.Buyer[:])
	deliveryKey := hex.EncodeToString(order.DeliveryKey[:])
	inventory := hex.EncodeToString(offer.Inventory[:])
	keyCommitment := hex.EncodeToString(offer.Keys[:])
	calldata, err := EncodeDirectBuy(order.OfferID, order.DeliveryKey)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	if purchase == nil || purchase.Format != preparedDirectBuyFormat || purchase.Version != actionRequestVersion ||
		purchase.Operation != operationBuy || purchase.Network != NetworkNamePinned ||
		purchase.Contract != contract || purchase.DirectOfferID != offerID ||
		purchase.Buyer != buyer || purchase.DeliveryKey != deliveryKey ||
		purchase.OrderNonce != order.Nonce || purchase.Price != order.Paid.String() ||
		purchase.Amount != total.String() || purchase.Calldata != hex.EncodeToString(calldata) ||
		purchase.Author != author || purchase.Epoch != offer.Epoch || purchase.CoveredFlips != offer.Covered ||
		purchase.InventoryCommitment != inventory || purchase.KeyCommitment != keyCommitment {
		return nil, VaultPayload{}, errors.New("retained purchase differs from the settled direct order")
	}
	if err := requireRetainedInventory(purchase.Availability); err != nil {
		return nil, VaultPayload{}, err
	}
	if err := directVerifiedSession(c, purchase.DirectSession); err != nil {
		return nil, VaultPayload{}, err
	}
	if purchase.DirectSession.Epoch != offer.Epoch ||
		strings.TrimPrefix(strings.ToLower(purchase.DirectSession.Author), "0x") != author ||
		purchase.Availability.Epoch != offer.Epoch ||
		strings.TrimPrefix(strings.ToLower(purchase.Availability.Author), "0x") != author ||
		purchase.Availability.CoveredFlips != int(offer.Covered) ||
		purchase.Availability.InventoryCommitment != inventory ||
		inventoryCommitment(author, offer.Epoch, purchase.DirectSession.Manifest) != inventory ||
		len(purchase.DirectSession.Manifest) != len(purchase.Availability.Blobs) {
		return nil, VaultPayload{}, errors.New("retained flips differ from the canonical direct session")
	}
	for i, retained := range purchase.Availability.Blobs {
		if retained.CID != purchase.DirectSession.Manifest[i] {
			return nil, VaultPayload{}, errors.New("retained CID order differs from the canonical direct session")
		}
	}
	secrets, err := readRegistryValue(c, contract, string(DirectRevealStorageKey(orderID)))
	if err != nil {
		return nil, VaultPayload{}, fmt.Errorf("read direct public reveal: %w", err)
	}
	if !directSecretsMatch(secrets, offer) {
		return nil, VaultPayload{}, errors.New("public reveal differs from the settled offer's epoch secrets")
	}
	keys, payload, err := epochKeysFromDeliveredSecrets(secrets)
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
		if err != nil || hex.EncodeToString(crypto.PubkeyToAddress(*authorKey).Bytes()) != author {
			return nil, VaultPayload{}, errors.New("retained original flip names another author")
		}
		public, err := keys.DecryptPublicPart(publicCipher)
		if err != nil {
			return nil, VaultPayload{}, errors.New("original flip public part does not open")
		}
		private, err := keys.DecryptPrivatePart(privateCipher)
		if err != nil {
			return nil, VaultPayload{}, errors.New("original flip private part does not open")
		}
		if err := validateDirectFlipParts(public, private); err != nil {
			return nil, VaultPayload{}, fmt.Errorf("original flip %s is not usable: %w", retained.CID, err)
		}
	}
	return &DirectRecoveredDelivery{
		Format: directRecoveredFormat, Version: 1, Network: NetworkNamePinned,
		Contract: contract, CodeHash: strings.ToLower(strings.TrimPrefix(codeHash, "0x")),
		OfferID: offerID, OrderID: orderID, OrderNonce: order.Nonce,
		Buyer: buyer, DeliveryKey: deliveryKey, Author: author, Epoch: offer.Epoch,
		InventoryCommitment: inventory, KeyCommitment: keyCommitment,
		KeySource: "public-reveal", UsableFlips: len(purchase.Availability.Blobs),
	}, payload, nil
}
