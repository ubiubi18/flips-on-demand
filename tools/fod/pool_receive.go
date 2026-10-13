package main

// The buyer's half of a pool purchase (`SPEC.md` 6.5 steps 7-8): read the
// order the pool recorded, open the ciphertext the desk published, check it
// against the bundle commitment the order carries, and write the epoch keys as
// a vault the blob path opens. The buyer's own check is what makes the
// content usable; `prepare-confirm` is the separate call that settles it.

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
)

const poolDeliveryStoragePrefix = "pdeliv/"
const poolRevealStoragePrefix = "preveal/"

// OpenedPoolDelivery records what the buyer checked and where the keys went.
type OpenedPoolDelivery struct {
	Format        string `json:"format"`
	Version       int    `json:"version"`
	Contract      string `json:"contract"`
	OrderID       uint64 `json:"orderId"`
	OrderNonce    uint64 `json:"orderNonce"`
	State         string `json:"state"`
	Buyer         string `json:"buyer"`
	DeliveryKey   string `json:"deliveryKey"`
	Epoch         uint16 `json:"epoch"`
	Author        string `json:"author"`
	KeyCommitment string `json:"keyCommitment"`
	SecretsSha256 string `json:"secretsSha256"`
	// Public-reveal recovery is already settled and needs no confirmation.
	KeySource string `json:"keySource,omitempty"`
	// CiphertextSha256 is the digest of the exact bytes this run read from the
	// registry, opened and checked. `prepare-confirm` carries it with the bytes
	// below, so the escrow settles on the payload this buyer checked whatever
	// the order's pending slot holds (`SPEC.md` 7.1, `AH-10`).
	CiphertextSha256 string `json:"ciphertextSha256"`
	// CiphertextHex is those exact bytes, hex encoded: the confirmation brings
	// them with the call, so nothing can replace the checked payload afterwards.
	CiphertextHex         string   `json:"ciphertextHex"`
	VaultPath             string   `json:"vaultPath"`
	Checks                []string `json:"checks"`
	NotChecked            []string `json:"notChecked"`
	UsableFlips           int      `json:"usableFlips"`
	ContentPaths          []string `json:"contentPaths,omitempty"`
	ContentCheckSignature string   `json:"contentCheckSignature,omitempty"`
}

func openedDeliveryDigest(opened *OpenedPoolDelivery) []byte {
	copy := *opened
	copy.ContentCheckSignature = ""
	raw, _ := json.Marshal(copy)
	digest := sha256.Sum256(append([]byte("fod.usable-delivery:v1:"), raw...))
	return digest[:]
}

func signOpenedDelivery(opened *OpenedPoolDelivery, key *ecdsa.PrivateKey) error {
	if opened.UsableFlips < 1 || hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey)) != opened.DeliveryKey {
		return fmt.Errorf("no usable content bound to this delivery key")
	}
	signature, err := crypto.Sign(openedDeliveryDigest(opened), key)
	if err != nil {
		return err
	}
	opened.ContentCheckSignature = hex.EncodeToString(signature)
	return nil
}

func verifyOpenedDelivery(opened *OpenedPoolDelivery) error {
	if opened == nil || opened.Version != 1 || opened.Format != "fod.opened-pool-delivery" || opened.UsableFlips < 1 {
		return fmt.Errorf("no usable-content delivery receipt")
	}
	signature, err := hex.DecodeString(opened.ContentCheckSignature)
	if err != nil || len(signature) != signatureBytes {
		return fmt.Errorf("unsigned usable-content receipt")
	}
	pub, err := crypto.SigToPub(openedDeliveryDigest(opened), signature)
	if err != nil || hex.EncodeToString(crypto.FromECDSAPub(pub)) != opened.DeliveryKey {
		return fmt.Errorf("the usable-content receipt was altered or signed by another key")
	}
	return nil
}

// verifyPurchasedContent opens the buyer's retained ciphertext, never a peer's
// new response. Keys and retained bytes must name the same order and author.
func verifyPurchasedContent(c *client, p *preparedAction, opened *OpenedPoolDelivery, passphrase, outputPrefix string) error {
	if err := requireRetainedInventory(p.Availability); err != nil {
		return err
	}
	listing, err := registryPoolBundle(c, opened.Contract, opened.KeyCommitment)
	if err != nil {
		return err
	}
	if listing.InventoryCommitment != p.Availability.InventoryCommitment || int(listing.FlipCount) != len(p.Availability.Blobs) {
		return fmt.Errorf("retained content is not the canonical purchased inventory")
	}
	if p.Bundle != opened.KeyCommitment || p.Contract != opened.Contract || p.Nonce != opened.OrderNonce || p.Buyer != opened.Buyer || p.DeliveryKey != opened.DeliveryKey ||
		p.Availability.Epoch != opened.Epoch || !strings.EqualFold(strings.TrimPrefix(p.Availability.Author, "0x"), strings.TrimPrefix(opened.Author, "0x")) {
		return fmt.Errorf("the retained purchase does not belong to this delivery")
	}
	payload, err := ReadVault(opened.VaultPath, passphrase)
	if err != nil {
		return err
	}
	var stored VaultPayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return err
	}
	keys, err := EpochKeysFromVaultPayload(stored)
	if err != nil {
		return err
	}
	for _, retained := range p.Availability.Blobs {
		blob, _ := hex.DecodeString(retained.Hex)
		pub, publicCipher, privateCipher, err := ParseFlipBlob(blob)
		if err != nil {
			return err
		}
		authorKey, err := crypto.UnmarshalPubkey(pub)
		if err != nil {
			return fmt.Errorf("cached flip has an invalid author key")
		}
		if !strings.EqualFold(crypto.PubkeyToAddress(*authorKey).Hex(), "0x"+strings.TrimPrefix(p.Availability.Author, "0x")) {
			return fmt.Errorf("cached flip has another author")
		}
		public, err := keys.DecryptPublicPart(publicCipher)
		if err != nil {
			return fmt.Errorf("cached public part is unusable: %w", err)
		}
		private, err := keys.DecryptPrivatePart(privateCipher)
		if err != nil {
			return fmt.Errorf("cached private part is unusable: %w", err)
		}
		if len(public) == 0 || len(private) == 0 {
			return fmt.Errorf("cached flip decrypts to empty content")
		}
		if outputPrefix != "" {
			for _, part := range []struct {
				name string
				data []byte
			}{{"public", public}, {"private", private}} {
				path := outputPrefix + "." + retained.CID + "." + part.name
				if err := writePrivateBytes(path, part.data); err != nil {
					return err
				}
				opened.ContentPaths = append(opened.ContentPaths, path)
			}
		}
		opened.UsableFlips++
	}
	opened.Checks = append(opened.Checks,
		"the retained CID inventory matches the canonical bundle commitment and count",
		"all retained encrypted bytes reproduced their CIDs and decrypted with the purchased keys")
	opened.NotChecked = []string{"which desk published the ciphertext: content and key commitments determine validity"}
	return nil
}

// Recovery reads either a settled public reveal or the buyer-bound ciphertext.
// Both must reproduce the purchased key commitment before the vault is written.
// The node must provide authenticated canonical state; RPC assertions alone
// are not a light-client proof. The buyer's private delivery key stays local.
func openPoolDeliveryFromRegistry(c *client, contract string, orderID uint64, deliveryKeyPath, vaultPath, passphrase string) (*OpenedPoolDelivery, error) {
	contract = strings.ToLower(strings.TrimPrefix(contract, "0x"))
	orderRaw, err := readRegistryValue(c, contract, fmt.Sprintf("%s%d", marketPoolOrderPrefix, orderID))
	if err != nil {
		return nil, err
	}
	order, err := decodePoolOrderFull(orderID, orderRaw)
	if err != nil {
		return nil, err
	}
	switch order.State {
	case "delivered", "settled":
	case "funded":
		return nil, fmt.Errorf("pool order %d is funded: no delivery has been published yet; wait for the desk, or use the refund path after block %d", orderID, order.RefundAt)
	case "refunded":
		return nil, fmt.Errorf("pool order %d was refunded on the chain", orderID)
	default:
		return nil, fmt.Errorf("pool order %d carries state %q", orderID, order.State)
	}

	deliveryKey, err := loadDeliveryKey(deliveryKeyPath)
	if err != nil {
		return nil, err
	}
	if recorded := fmt.Sprintf("%x", crypto.FromECDSAPub(&deliveryKey.PublicKey)); !strings.EqualFold(recorded, order.DeliveryKey) {
		return nil, fmt.Errorf("pool order %d recorded the delivery key %s and this key's public half is %s: this delivery is not for this key",
			orderID, order.DeliveryKey, recorded)
	}

	var plain, cipherRaw []byte
	keySource, ciphertextSha256 := "", ""
	if order.State == "settled" {
		plain, err = readRegistryValue(c, contract, fmt.Sprintf("%s%d", poolRevealStoragePrefix, orderID))
		if err != nil && !errors.Is(err, errNotInRegistry) {
			return nil, fmt.Errorf("read the settled reveal for pool order %d: %w", orderID, err)
		}
		if err == nil {
			keySource = "public-reveal"
		}
	}
	if keySource == "" {
		cipherRaw, err = readRegistryValue(c, contract, fmt.Sprintf("%s%d", poolDeliveryStoragePrefix, orderID))
		if err != nil {
			return nil, fmt.Errorf("read the delivery recorded for pool order %d: %w", orderID, err)
		}
		plain, err = ecies.ImportECDSA(deliveryKey).Decrypt(cipherRaw, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("the ciphertext recorded for pool order %d does not decrypt with this delivery key: %w", orderID, err)
		}
		cipherDigest := sha256.Sum256(cipherRaw)
		ciphertextSha256 = hex.EncodeToString(cipherDigest[:])
	}
	keys, payload, err := epochKeysFromDeliveredSecrets(plain)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(plain)
	secretsSha256 := hex.EncodeToString(digest[:])
	if !strings.EqualFold(secretsSha256, order.Bundle) {
		return nil, fmt.Errorf("the delivered secrets hash to %s, and pool order %d names bundle %s: refusing to keep them",
			secretsSha256, orderID, order.Bundle)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode vault payload: %w", err)
	}
	if err := WriteVault(vaultPath, passphrase, payloadJSON); err != nil {
		return nil, fmt.Errorf("write vault: %w", err)
	}

	checks := []string{
		fmt.Sprintf("the delivery key this machine holds is the one pool order %d recorded", orderID),
		fmt.Sprintf("those secrets hash to %s, the key commitment the pool bundle carries", secretsSha256),
		fmt.Sprintf("the order is %s, so the registry recorded a delivery for it", order.State),
	}
	if keySource == "public-reveal" {
		checks = append(checks, "settled public keys recovered from contract state; no confirmation or delivery desk is needed")
	} else {
		checks = append(checks,
			fmt.Sprintf("the recorded ciphertext decrypted to %d bytes of delivered secrets", len(plain)),
			fmt.Sprintf("the recorded ciphertext hashes to %s, which is what the confirmation names", ciphertextSha256))
	}
	notChecked := []string{
		"that the bundle's flip list is the one advertised: that is the registration proof's claim, checked by the registry",
		"which desk published the ciphertext: the buyer's check is the commitment, not the publisher",
	}
	if order.State == "delivered" {
		notChecked = append(notChecked, "the buyer's confirmation: the order settles only when `prepare-confirm` is submitted")
	}

	return &OpenedPoolDelivery{
		Format:           "fod.opened-pool-delivery",
		Version:          1,
		Contract:         contract,
		OrderID:          orderID,
		OrderNonce:       order.Nonce,
		State:            order.State,
		Buyer:            order.Buyer,
		DeliveryKey:      order.DeliveryKey,
		Epoch:            keys.Epoch,
		Author:           keys.Address,
		KeyCommitment:    order.Bundle,
		SecretsSha256:    secretsSha256,
		KeySource:        keySource,
		CiphertextSha256: ciphertextSha256,
		CiphertextHex:    hex.EncodeToString(cipherRaw),
		VaultPath:        vaultPath,
		Checks:           checks,
		NotChecked:       notChecked,
	}, nil
}
