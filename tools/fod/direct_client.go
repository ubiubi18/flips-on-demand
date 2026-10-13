package main

// The direct-sale client treats a message as transport and the node's canonical
// state, session anchors, retained CIDs, and decrypted flip content as evidence.

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
)

const (
	preparedDirectListFormat            = "fod.prepared-direct-list"
	preparedDirectBuyFormat             = "fod.prepared-direct-buy"
	preparedDirectConfirmFormat         = "fod.prepared-direct-confirm"
	preparedDirectReserveFallbackFormat = "fod.prepared-direct-reserve-fallback"
	preparedDirectStartFallbackFormat   = "fod.prepared-direct-start-fallback"
	preparedDirectStartFallbackV4Format = "fod.prepared-direct-start-fallback-v4"
	preparedDirectFallbackFormat        = "fod.prepared-direct-fallback"
	preparedDirectRefundFormat          = "fod.prepared-direct-refund"
	directOpenedFormat                  = "fod.opened-direct-delivery"
	directFallbackOperation             = "proveAndReveal"
	directStartFallbackOperation        = "startFallback"
	directReserveFallbackOperation      = "reserveFallback"
)

func directAddress(value string) (string, error) {
	value = strings.ToLower(strings.TrimPrefix(value, "0x"))
	if !isHex(value, addressBytes) {
		return "", errors.New("invalid direct-sale address")
	}
	return value, nil
}

func directOfferFromChain(c *client, contract string, id [32]byte) (*DirectOffer, error) {
	records, err := readPrefix(c, registryAddress(contract), directOfferPrefix)
	if err != nil {
		return nil, err
	}
	raw, ok := records[hex.EncodeToString(id[:])]
	if !ok {
		return nil, errNotInRegistry
	}
	offer, err := DecodeDirectOffer(raw)
	if err != nil {
		return nil, err
	}
	if DirectOfferID(offer.Author, offer.Epoch) != id {
		return nil, errors.New("direct offer ID differs from its author and epoch")
	}
	return offer, nil
}

func directOrderFromChain(c *client, contract string, id uint64) (*DirectOrder, error) {
	if id == 0 {
		return nil, errors.New("direct order ID is zero")
	}
	raw, err := readRegistryValue(c, contract, string(DirectOrderStorageKey(id)))
	if err != nil {
		return nil, err
	}
	return DecodeDirectOrder(raw)
}

func directIntentFromChain(c *client, contract string, id uint64) (*DirectIntent, error) {
	raw, err := readRegistryValue(c, contract, string(DirectIntentStorageKey(id)))
	if err != nil {
		return nil, err
	}
	return DecodeDirectIntent(raw)
}

func directStakeFromChain(c *client, contract string, id uint64) (*big.Int, error) {
	raw, err := readRegistryValue(c, contract, string(DirectStakeStorageKey(id)))
	if err != nil {
		return nil, err
	}
	return DecodeDirectStake(raw)
}

func directPendingStakeV4(c *client, contract string, id uint64, order *DirectOrder) (*big.Int, error) {
	if order == nil || order.State != DirectFallbackPending || order.Paid == nil {
		return nil, errors.New("v4 direct order has no pending fallback")
	}
	stake, err := directStakeFromChain(c, contract, id)
	if err != nil {
		return nil, fmt.Errorf("v4 direct fallback has no included seller stake: %w", err)
	}
	if stake.Cmp(order.Paid) < 0 {
		return nil, errors.New("v4 direct fallback stake is below the offer price")
	}
	return stake, nil
}

func directProofRateFromChain(c *client, contract string, id uint64) (*big.Int, error) {
	raw, err := readRegistryValue(c, contract, string(DirectProofRateStorageKey(id)))
	if err != nil {
		return nil, err
	}
	return DecodeDirectProofRate(raw)
}

func validateDirectIntent(intent *DirectIntent, offer *DirectOffer, order *DirectOrder, head uint64) error {
	if intent == nil || intent.Stake == nil || offer == nil || offer.Price == nil || order == nil {
		return errors.New("direct fallback intent differs from the funded order or canonical head")
	}
	end, ok := directReserveDeadline(order.FundedAt)
	if !ok || order.FundedAt > ^uint64(0)-directConfirmGrace ||
		intent.Stake.Cmp(offer.Price) < 0 || intent.At > head ||
		intent.At < order.FundedAt+directConfirmGrace || intent.At >= end {
		return errors.New("direct fallback intent differs from the funded order or canonical head")
	}
	return nil
}

func directIncludedIntent(c *client, contract string, id uint64, offer *DirectOffer, order *DirectOrder, head uint64) (*DirectIntent, error) {
	intent, err := directIntentFromChain(c, contract, id)
	if err != nil {
		return nil, fmt.Errorf("direct fallback needs an included intent: %w", err)
	}
	if err := validateDirectIntent(intent, offer, order, head); err != nil {
		return nil, err
	}
	return intent, nil
}

func directOrderStateLabel(state DirectOrderState) string {
	return map[DirectOrderState]string{
		DirectFunded: "funded", DirectSettled: "settled", DirectRefunded: "refunded",
		DirectFallbackPending: "fallback-pending", DirectFallbackIntent: "fallback-intent",
	}[state]
}

func directDMContext(contract string, offer *DirectOffer, orderID uint64, order *DirectOrder) DirectDMContext {
	id := DirectOfferID(offer.Author, offer.Epoch)
	return DirectDMContext{
		Network:  NetworkNamePinned,
		Contract: contract,
		Offer: DirectDMOffer{
			ID: hex.EncodeToString(id[:]), Author: hex.EncodeToString(offer.Author[:]),
			Epoch: offer.Epoch, Price: offer.Price.String(), NextNonce: offer.NextNonce,
			OpenOrderID: offer.OpenOrderID, Retired: offer.Retired,
		},
		Order: DirectDMOrder{
			ID: orderID, OfferID: hex.EncodeToString(order.OfferID[:]),
			Buyer: hex.EncodeToString(order.Buyer[:]), DeliveryKey: hex.EncodeToString(order.DeliveryKey[:]),
			Paid: order.Paid.String(), FundedAt: order.FundedAt,
			Nonce: order.Nonce, State: directOrderStateLabel(order.State),
		},
	}
}

func directActiveSale(c *client, contract string, orderID uint64) (*DirectOffer, *DirectOrder, DirectDMContext, error) {
	order, err := directOrderFromChain(c, contract, orderID)
	if err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	offer, err := directOfferFromChain(c, contract, order.OfferID)
	if err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	if (order.State != DirectFunded && order.State != DirectFallbackPending && order.State != DirectFallbackIntent) || offer.Retired || offer.OpenOrderID != orderID ||
		order.Paid.Cmp(offer.Price) != 0 || offer.NextNonce == 0 || order.Nonce+1 != offer.NextNonce {
		return nil, nil, DirectDMContext{}, errors.New("direct sale is not a currently active order")
	}
	if _, err := directFundedAmount(order.Paid, order.Bond); err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	context := directDMContext(contract, offer, orderID, order)
	return offer, order, context, nil
}

func directOpenSale(c *client, contract string, orderID uint64) (*DirectOffer, *DirectOrder, DirectDMContext, error) {
	offer, order, context, err := directActiveSale(c, contract, orderID)
	if err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	if order.State != DirectFunded {
		return nil, nil, DirectDMContext{}, errors.New("direct sale is not a currently funded order")
	}
	return offer, order, context, nil
}

// A settled cooperative order retains its immutable offer and delivery hash.
// Reconstruct only the funded context needed to validate the original message;
// this path never authorizes a new confirmation.
func directCheckSale(c *client, contract string, orderID uint64) (*DirectOffer, *DirectOrder, DirectDMContext, error) {
	order, err := directOrderFromChain(c, contract, orderID)
	if err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	if order.State == DirectFunded {
		return directOpenSale(c, contract, orderID)
	}
	if order.State != DirectSettled || order.DeliveryHash == ([32]byte{}) || order.Nonce == 0 || order.Nonce == ^uint64(0) {
		return nil, nil, DirectDMContext{}, errors.New("direct order is not funded or cooperatively settled")
	}
	offer, err := directOfferFromChain(c, contract, order.OfferID)
	if err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	if offer.NextNonce <= order.Nonce || order.Paid.Cmp(offer.Price) != 0 {
		return nil, nil, DirectDMContext{}, errors.New("settled direct order differs from its offer")
	}
	if _, err := directFundedAmount(order.Paid, order.Bond); err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	context := directDMContext(contract, offer, orderID, order)
	context.Offer.NextNonce = order.Nonce + 1
	context.Offer.OpenOrderID = orderID
	context.Offer.Retired = false
	context.Order.State = "funded"
	return offer, order, context, nil
}

func directVaultSecrets(vaultPath, passphrase string) ([]byte, *VaultPayload, error) {
	raw, err := ReadVault(vaultPath, passphrase)
	if err != nil {
		return nil, nil, err
	}
	var payload VaultPayload
	if err := decodeStrict(raw, &payload); err != nil {
		return nil, nil, err
	}
	if payload.Kind != "fod.epoch-keys" || payload.Version != 1 || payload.Epoch == 0 {
		return nil, nil, errors.New("vault is not an epoch key vault")
	}
	authorHex, err := directAddress(payload.Address)
	if err != nil {
		return nil, nil, err
	}
	author, _ := hex.DecodeString(authorHex)
	keys, err := EpochKeysFromVaultPayload(payload)
	if err != nil {
		return nil, nil, err
	}
	publicPart, privatePart := keys.SecretBytes()
	secrets, err := SecretsCanonicalBytes(payload.Epoch, author, publicPart, privatePart)
	if err != nil {
		return nil, nil, err
	}
	return secrets, &payload, nil
}

func directVerifiedSession(c *client, session *SessionBundle) error {
	if session == nil || session.Epoch == 0 || len(session.Manifest) == 0 || len(session.Manifest) > 32 {
		return errors.New("direct sale needs a complete canonical session inventory")
	}
	verified, err := sessionBundleFromAnchors(session.Author, &session.Inventory, &session.Verdict)
	if err != nil {
		return err
	}
	if verified.Epoch != session.Epoch || !sameStrings(verified.Manifest, session.Manifest) {
		return errors.New("direct session differs from its state leaves")
	}
	return checkFastSessionHeaders(c, verified)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func prepareDirectList(c *client, contract string, session *SessionBundle, secrets []byte, price *big.Int) (*preparedAction, error) {
	contract, err := directAddress(contract)
	if err != nil {
		return nil, err
	}
	if err := directVerifiedSession(c, session); err != nil {
		return nil, err
	}
	author, err := directAddress(session.Author)
	if err != nil {
		return nil, err
	}
	authorBytes, _ := hex.DecodeString(author)
	if len(secrets) != directSecretsBytes ||
		!strings.EqualFold(hex.EncodeToString(secrets[len(directSecretsDomain)+2:len(directSecretsDomain)+22]), author) {
		return nil, errors.New("epoch keys name another session author")
	}
	keys, _, err := epochKeysFromDeliveredSecrets(secrets)
	if err != nil || keys.Epoch != session.Epoch {
		return nil, errors.New("epoch keys name another session epoch")
	}
	var author20 [20]byte
	copy(author20[:], authorBytes)
	id := DirectOfferID(author20, session.Epoch)
	if _, err := directOfferFromChain(c, contract, id); err == nil {
		return nil, errors.New("this author and epoch already has a direct listing")
	} else if !errors.Is(err, errNotInRegistry) {
		return nil, err
	}
	inventory, _ := hex.DecodeString(inventoryCommitment(author, session.Epoch, session.Manifest))
	keyDigest := sha256.Sum256(secrets)
	var inv [32]byte
	copy(inv[:], inventory)
	args, err := EncodeDirectList(DirectListArgs{Epoch: session.Epoch, Covered: uint32(len(session.Manifest)), Inventory: inv, Keys: keyDigest, Price: price})
	if err != nil {
		return nil, err
	}
	return &preparedAction{
		Format: preparedDirectListFormat, Version: actionRequestVersion, Operation: "list",
		Network: NetworkNamePinned, Contract: contract, Amount: "0", Calldata: hex.EncodeToString(args),
		Author: author, Epoch: session.Epoch, CoveredFlips: uint32(len(session.Manifest)),
		InventoryCommitment: hex.EncodeToString(inv[:]), KeyCommitment: hex.EncodeToString(keyDigest[:]),
		Price: price.String(), DirectOfferID: hex.EncodeToString(id[:]), DirectSession: session,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func prepareDirectBuy(c *client, contract string, session *SessionBundle, buyer string, deliveryKey *ecdsa.PrivateKey) (*preparedAction, error) {
	contract, err := directAddress(contract)
	if err != nil {
		return nil, err
	}
	buyer, err = directAddress(buyer)
	if err != nil {
		return nil, err
	}
	if deliveryKey == nil {
		return nil, errors.New("missing dedicated delivery key")
	}
	if hex.EncodeToString(crypto.PubkeyToAddress(deliveryKey.PublicKey).Bytes()) == buyer {
		return nil, errors.New("delivery key must differ from the buyer's signing identity")
	}
	if err := directVerifiedSession(c, session); err != nil {
		return nil, err
	}
	author, _ := directAddress(session.Author)
	authorBytes, _ := hex.DecodeString(author)
	var author20 [20]byte
	copy(author20[:], authorBytes)
	id := DirectOfferID(author20, session.Epoch)
	offer, err := directOfferFromChain(c, contract, id)
	if err != nil {
		return nil, err
	}
	if !offer.CanBuy() || int(offer.Covered) != len(session.Manifest) ||
		hex.EncodeToString(offer.Inventory[:]) != inventoryCommitment(author, session.Epoch, session.Manifest) {
		return nil, errors.New("direct offer is unavailable or differs from the canonical CID inventory")
	}
	advisory := &AvailabilityAdvisory{Author: author, Epoch: session.Epoch, Flips: make([]AvailabilityFlip, 0, len(session.Manifest))}
	for _, cid := range session.Manifest {
		advisory.Flips = append(advisory.Flips, AvailabilityFlip{CID: cid})
	}
	check := recheckAvailability(advisory, oneSource(ipfsFetch(c), "buyer node"), nodeCIDComputer(c))
	check.Binding = bindingCommitted
	check.InventoryCommitment = hex.EncodeToString(offer.Inventory[:])
	if err := requireCheckedAvailability(check, time.Now()); err != nil {
		return nil, err
	}
	var publicKey [65]byte
	copy(publicKey[:], crypto.FromECDSAPub(&deliveryKey.PublicKey))
	args, err := EncodeDirectBuy(id, publicKey)
	if err != nil {
		return nil, err
	}
	rate, err := readFeePerGas(c)
	if err != nil {
		return nil, fmt.Errorf("read direct bond fee: %w", err)
	}
	bond, err := directQuotedBondForFee(rate)
	if err != nil {
		return nil, err
	}
	total, err := directFundedAmount(offer.Price, bond)
	if err != nil {
		return nil, err
	}
	return &preparedAction{
		Format: preparedDirectBuyFormat, Version: actionRequestVersion, Operation: operationBuy,
		Network: NetworkNamePinned, Contract: contract, Amount: total.String(), Calldata: hex.EncodeToString(args),
		Author: author, Epoch: session.Epoch, CoveredFlips: offer.Covered, DirectOfferID: hex.EncodeToString(id[:]),
		Price: offer.Price.String(),
		Buyer: buyer, DeliveryKey: hex.EncodeToString(publicKey[:]), OrderNonce: offer.NextNonce,
		InventoryCommitment: hex.EncodeToString(offer.Inventory[:]), KeyCommitment: hex.EncodeToString(offer.Keys[:]),
		Availability: check, DirectSession: session, PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func prepareDirectDM(c *client, contract string, orderID uint64, secrets []byte) (string, error) {
	contract, err := directAddress(contract)
	if err != nil {
		return "", err
	}
	offer, order, context, err := directOpenSale(c, contract, orderID)
	if err != nil {
		return "", err
	}
	if !directSecretsMatch(secrets, offer) {
		return "", errors.New("vault keys do not match the funded offer")
	}
	pub, err := crypto.UnmarshalPubkey(order.DeliveryKey[:])
	if err != nil {
		return "", err
	}
	ciphertext, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pub), secrets, nil, nil)
	if err != nil {
		return "", err
	}
	envelope, err := NewDirectDMEnvelope(context, ciphertext)
	if err != nil {
		return "", err
	}
	return EncodeDirectDMEnvelope(envelope, context)
}

func directSecretsMatch(secrets []byte, offer *DirectOffer) bool {
	if offer == nil || len(secrets) != directSecretsBytes || string(secrets[:len(directSecretsDomain)]) != directSecretsDomain {
		return false
	}
	if uint16(secrets[len(directSecretsDomain)])<<8|uint16(secrets[len(directSecretsDomain)+1]) != offer.Epoch {
		return false
	}
	if !equalBytes(secrets[len(directSecretsDomain)+2:len(directSecretsDomain)+22], offer.Author[:]) {
		return false
	}
	digest := sha256.Sum256(secrets)
	return digest == offer.Keys
}

// DirectOpenedDelivery is signed by the dedicated delivery key only after the
// buyer's retained original blobs have all opened as supported flips.
type DirectOpenedDelivery struct {
	Format                string `json:"format"`
	Version               int    `json:"version"`
	Contract              string `json:"contract"`
	OfferID               string `json:"offerId"`
	OrderID               uint64 `json:"orderId"`
	OrderNonce            uint64 `json:"orderNonce"`
	Buyer                 string `json:"buyer"`
	DeliveryKey           string `json:"deliveryKey"`
	Author                string `json:"author"`
	Epoch                 uint16 `json:"epoch"`
	InventoryCommitment   string `json:"inventoryCommitment"`
	KeyCommitment         string `json:"keyCommitment"`
	Ciphertext            string `json:"ciphertext"`
	UsableFlips           int    `json:"usableFlips"`
	ContentCheckSignature string `json:"contentCheckSignature"`
}

func directOpenedDigest(record *DirectOpenedDelivery) []byte {
	copy := *record
	copy.ContentCheckSignature = ""
	raw, _ := json.Marshal(copy)
	h := sha256.Sum256(append([]byte("fod:direct-usable:v1:"), raw...))
	return h[:]
}

func signDirectOpened(record *DirectOpenedDelivery, key *ecdsa.PrivateKey) error {
	if record.UsableFlips < 1 || hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey)) != record.DeliveryKey {
		return errors.New("no usable content bound to this delivery key")
	}
	sig, err := crypto.Sign(directOpenedDigest(record), key)
	if err != nil {
		return err
	}
	record.ContentCheckSignature = hex.EncodeToString(sig)
	return nil
}

func verifyDirectOpened(record *DirectOpenedDelivery) error {
	if record == nil || record.Format != directOpenedFormat || record.Version != 1 || record.UsableFlips < 1 {
		return errors.New("no usable direct delivery receipt")
	}
	sig, err := hex.DecodeString(record.ContentCheckSignature)
	if err != nil || len(sig) != signatureBytes {
		return errors.New("unsigned direct delivery receipt")
	}
	pub, err := crypto.SigToPub(directOpenedDigest(record), sig)
	if err != nil || hex.EncodeToString(crypto.FromECDSAPub(pub)) != record.DeliveryKey {
		return errors.New("direct delivery receipt signature mismatch")
	}
	return nil
}

func verifyDirectDM(c *client, contract string, orderID uint64, message string, purchase *preparedAction, deliveryKey *ecdsa.PrivateKey, allowSettled bool) (*DirectOpenedDelivery, VaultPayload, error) {
	contract, err := directAddress(contract)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	var offer *DirectOffer
	var order *DirectOrder
	var context DirectDMContext
	if allowSettled {
		offer, order, context, err = directCheckSale(c, contract, orderID)
	} else {
		offer, order, context, err = directOpenSale(c, contract, orderID)
	}
	if err != nil {
		return nil, VaultPayload{}, err
	}
	total, err := directFundedAmount(order.Paid, order.Bond)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	if purchase == nil || purchase.Format != preparedDirectBuyFormat || purchase.DirectOfferID != context.Offer.ID ||
		purchase.Contract != contract || purchase.Buyer != context.Order.Buyer || purchase.DeliveryKey != context.Order.DeliveryKey ||
		purchase.OrderNonce != order.Nonce || purchase.InventoryCommitment != hex.EncodeToString(offer.Inventory[:]) ||
		purchase.KeyCommitment != hex.EncodeToString(offer.Keys[:]) || purchase.Price != order.Paid.String() ||
		purchase.Amount != total.String() {
		return nil, VaultPayload{}, errors.New("retained purchase differs from the funded direct order")
	}
	if err := requireRetainedInventory(purchase.Availability); err != nil {
		return nil, VaultPayload{}, err
	}
	if err := directVerifiedSession(c, purchase.DirectSession); err != nil {
		return nil, VaultPayload{}, err
	}
	if purchase.DirectSession.Epoch != offer.Epoch || !strings.EqualFold(purchase.DirectSession.Author, context.Offer.Author) ||
		inventoryCommitment(context.Offer.Author, offer.Epoch, purchase.DirectSession.Manifest) != purchase.InventoryCommitment {
		return nil, VaultPayload{}, errors.New("retained purchase is not this author's canonical session")
	}
	if purchase.Availability.InventoryCommitment != purchase.InventoryCommitment || purchase.Availability.CoveredFlips != int(offer.Covered) ||
		purchase.Availability.Epoch != offer.Epoch || strings.TrimPrefix(strings.ToLower(purchase.Availability.Author), "0x") != context.Offer.Author {
		return nil, VaultPayload{}, errors.New("retained blobs differ from the canonical direct offer")
	}
	if deliveryKey == nil || hex.EncodeToString(crypto.FromECDSAPub(&deliveryKey.PublicKey)) != context.Order.DeliveryKey {
		return nil, VaultPayload{}, errors.New("this delivery key does not belong to the funded order")
	}
	envelope, err := ParseDirectDMEnvelope(message, context)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	ciphertext, _ := hex.DecodeString(envelope.Ciphertext)
	if order.State == DirectSettled && sha256.Sum256(ciphertext) != order.DeliveryHash {
		return nil, VaultPayload{}, errors.New("private delivery differs from the settled chain hash")
	}
	secrets, err := ecies.ImportECDSA(deliveryKey).Decrypt(ciphertext, nil, nil)
	if err != nil {
		return nil, VaultPayload{}, errors.New("direct delivery could not be opened by this buyer key")
	}
	if !directSecretsMatch(secrets, offer) {
		return nil, VaultPayload{}, errors.New("delivered keys do not match the funded offer")
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
		if err != nil || hex.EncodeToString(crypto.PubkeyToAddress(*authorKey).Bytes()) != context.Offer.Author {
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
	record := &DirectOpenedDelivery{
		Format: directOpenedFormat, Version: 1, Contract: contract, OfferID: context.Offer.ID,
		OrderID: orderID, OrderNonce: order.Nonce, Buyer: context.Order.Buyer,
		DeliveryKey: context.Order.DeliveryKey, Author: context.Offer.Author, Epoch: offer.Epoch,
		InventoryCommitment: hex.EncodeToString(offer.Inventory[:]), KeyCommitment: hex.EncodeToString(offer.Keys[:]),
		Ciphertext: envelope.Ciphertext, UsableFlips: len(purchase.Availability.Blobs),
	}
	return record, payload, nil
}

func importDirectDM(c *client, contract string, orderID uint64, message string, purchase *preparedAction, deliveryKey *ecdsa.PrivateKey, allowSettled bool) (*DirectOpenedDelivery, VaultPayload, error) {
	record, payload, err := verifyDirectDM(c, contract, orderID, message, purchase, deliveryKey, allowSettled)
	if err != nil {
		return nil, VaultPayload{}, err
	}
	if err := signDirectOpened(record, deliveryKey); err != nil {
		return nil, VaultPayload{}, err
	}
	return record, payload, nil
}

func prepareDirectConfirm(c *client, contract string, record *DirectOpenedDelivery) (*preparedAction, error) {
	if err := verifyDirectOpened(record); err != nil {
		return nil, err
	}
	contract, err := directAddress(contract)
	if err != nil {
		return nil, err
	}
	if record.Contract != contract {
		return nil, errors.New("receipt names another direct contract")
	}
	offer, order, context, err := directOpenSale(c, contract, record.OrderID)
	if err != nil {
		return nil, err
	}
	if record.OfferID != context.Offer.ID || record.OrderNonce != order.Nonce || record.Buyer != context.Order.Buyer ||
		record.DeliveryKey != context.Order.DeliveryKey || record.Author != context.Offer.Author || record.Epoch != offer.Epoch ||
		record.KeyCommitment != hex.EncodeToString(offer.Keys[:]) || record.InventoryCommitment != hex.EncodeToString(offer.Inventory[:]) ||
		record.UsableFlips != int(offer.Covered) {
		return nil, errors.New("usable delivery receipt differs from the funded direct order")
	}
	ciphertext, err := hex.DecodeString(record.Ciphertext)
	if err != nil || len(ciphertext) != directDMCiphertextBytes {
		return nil, errors.New("receipt has no canonical ciphertext")
	}
	arg, err := EncodeDirectConfirm(record.OrderID, ciphertext)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(ciphertext)
	return &preparedAction{
		Format: preparedDirectConfirmFormat, Version: actionRequestVersion, Operation: operationConfirm,
		Network: NetworkNamePinned, Contract: contract, Amount: "0", Calldata: hex.EncodeToString(arg),
		DirectOfferID: record.OfferID, DirectOrderID: record.OrderID, Buyer: record.Buyer,
		CiphertextSha256: hex.EncodeToString(digest[:]), DirectOpened: record,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func readDirectMessageFile(path string) (string, error) {
	if path == "-" {
		return readDirectMessage(os.Stdin)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > directDMMaxTextBytes {
		return "", errors.New("direct message is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", errors.New("direct message changed while opening it")
	}
	return readDirectMessage(file)
}

func readDirectMessage(reader io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, directDMMaxTextBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 || len(raw) > directDMMaxTextBytes {
		return "", errors.New("direct message must be nonempty and at most 2048 bytes")
	}
	return string(raw), nil
}
