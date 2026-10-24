package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"

	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
)

type bondedV8Retained struct {
	Contract     string             `json:"contract"`
	Trade        string             `json:"trade"`
	Buyer        string             `json:"buyer"`
	DeliveryKey  string             `json:"deliveryKey"`
	Bundle       string             `json:"bundle"`
	Availability *AvailabilityCheck `json:"availability"`
}

func retainBondedV8(c *client, contract string, t *bondedV8Trade, b *bondedV8Bundle) (*bondedV8Retained, error) {
	a := &AvailabilityCheck{CheckedAt: checkedAtNow(), Author: b.Author, Epoch: b.Epoch, Binding: bindingCommitted, CoveredFlips: int(b.Count), VerifiedFlips: int(b.Count), InventoryCommitment: b.Inventory}
	for _, cid := range b.CIDs {
		blob, e := ipfsFetch(c)(cid)
		if e != nil {
			return nil, errors.New("encrypted flip unavailable from the local node; readiness was not sent")
		}
		if len(blob) > maxRetainedFlipBytes {
			return nil, errors.New("oversized flip")
		}
		if ok, _ := verifyBlobCID(cid, blob, nil); !ok {
			return nil, errors.New("encrypted flip CID mismatch")
		}
		a.Blobs = append(a.Blobs, RetainedFlip{cid, hex.EncodeToString(blob)})
	}
	if e := requireRetainedInventory(a); e != nil {
		return nil, e
	}
	return &bondedV8Retained{contract, t.ID, t.Buyer, t.DeliveryKey, b.ID, a}, nil
}
func readBondedV8Retained(contract string, t *bondedV8Trade, b *bondedV8Bundle, file string) (*bondedV8Retained, error) {
	raw, e := readBondedV8Private(file, 48*1024*1024)
	if e != nil {
		return nil, e
	}
	var a bondedV8Retained
	if e = decodeStrict(raw, &a); e != nil {
		return nil, e
	}
	if a.Contract != contract || a.Trade != t.ID || a.Buyer != t.Buyer || a.DeliveryKey != t.DeliveryKey || a.Bundle != b.ID || a.Availability == nil {
		return nil, errors.New("retained flips do not belong to this trade")
	}
	if a.Availability.Author != b.Author || a.Availability.Epoch != b.Epoch || a.Availability.InventoryCommitment != b.Inventory || a.Availability.CoveredFlips != int(b.Count) {
		return nil, errors.New("retained flips differ from registered bundle")
	}
	if e = requireRetainedInventory(a.Availability); e != nil {
		return nil, e
	}
	return &a, nil
}
func openBondedV8(c *client, contract string, t *bondedV8Trade, b *bondedV8Bundle, r *bondedV8Request) (*VaultPayload, error) {
	a, e := readBondedV8Retained(contract, t, b, r.RetainedFile)
	if e != nil {
		return nil, e
	}
	rawKey, e := readBondedV8Private(r.DeliveryKeyFile, 128)
	if e != nil {
		return nil, e
	}
	defer clear(rawKey)
	priv, e := crypto.HexToECDSA(string(bytes.TrimSpace(rawKey)))
	if e != nil {
		return nil, errors.New("invalid private delivery key")
	}
	if hex.EncodeToString(crypto.FromECDSAPub(&priv.PublicKey)) != t.DeliveryKey {
		return nil, errors.New("private delivery key differs from the buyer order")
	}
	suffix, _ := v8number(t.ID, 8)
	ct, e := v8map(c, contract, bondedPrefix(c)+"ciphertext/", suffix)
	if e != nil {
		return nil, e
	}
	digest := sha256.Sum256(ct)
	if len(ct) != 219 || hex.EncodeToString(digest[:]) != t.CiphertextHash {
		return nil, errors.New("on-chain ciphertext mismatch")
	}
	plain, e := ecies.ImportECDSA(priv).Decrypt(ct, nil, nil)
	if e != nil {
		return nil, errors.New("private delivery cannot be decrypted")
	}
	defer clear(plain)
	commit := sha256.Sum256(plain)
	if hex.EncodeToString(commit[:]) != b.Keys {
		return nil, errors.New("delivered keys differ from the listed commitment")
	}
	keys, vault, e := epochKeysFromDeliveredSecrets(plain)
	if e != nil {
		return nil, e
	}
	author, e := directAddress(vault.Address)
	if e != nil || author != b.Author || vault.Epoch != b.Epoch {
		return nil, errors.New("delivered epoch or author mismatch")
	}
	for _, retained := range a.Availability.Blobs {
		blob, _ := hex.DecodeString(retained.Hex)
		pub, publicCipher, privateCipher, e := ParseFlipBlob(blob)
		if e != nil {
			return nil, e
		}
		authorKey, e := crypto.UnmarshalPubkey(pub)
		if e != nil || hex.EncodeToString(crypto.PubkeyToAddress(*authorKey).Bytes()) != b.Author {
			return nil, errors.New("flip author mismatch")
		}
		public, e := keys.DecryptPublicPart(publicCipher)
		if e != nil {
			return nil, errors.New("public flip part cannot be opened")
		}
		private, e := keys.DecryptPrivatePart(privateCipher)
		if e != nil {
			return nil, errors.New("private flip part cannot be opened")
		}
		if e = validateDirectFlipParts(public, private); e != nil {
			return nil, e
		}
	}
	return &vault, nil
}
func verifyBondedV8LocalAction(c *client, contract string, r *bondedV8Request) error {
	if r.version() == 9 && r.Operation == "queueSell" {
		expected := *r
		if e := fillBondedV9Listing(c, contract, &expected, os.Getenv("FOD_PASSPHRASE")); e != nil {
			return e
		}
		if expected.Journal != r.Journal || expected.SessionWire != r.SessionWire || expected.Bundle != r.Bundle {
			return errors.New("listing differs from canonical local package")
		}
		return nil
	}
	if r.Operation == "queueBuy" {
		raw, e := readBondedV8Private(r.DeliveryKeyFile, 128)
		if e != nil {
			return e
		}
		defer clear(raw)
		key, e := crypto.HexToECDSA(string(bytes.TrimSpace(raw)))
		if e != nil || hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey)) != r.DeliveryKey {
			return errors.New("buyer must retain the matching private delivery key")
		}
		return nil
	}
	if r.Operation != "ready" && r.Operation != "accept" && r.Operation != "deliver" {
		return nil
	}
	t, e := readBondedV8Trade(c, contract, r.ID)
	if e != nil {
		return e
	}
	b, e := readBondedV8Bundle(c, contract, t.Bundle)
	if e != nil {
		return e
	}
	if r.Operation == "deliver" {
		if r.Caller != t.Seller {
			return errors.New("only assigned seller can deliver")
		}
		return nil
	}
	if r.Caller != t.Buyer {
		return errors.New("only the assigned buyer can verify delivery")
	}
	if r.Operation == "ready" {
		_, e = readBondedV8Retained(contract, t, b, r.RetainedFile)
		return e
	}
	if r.CiphertextHash != t.CiphertextHash {
		return errors.New("acceptance differs from immutable delivery")
	}
	_, e = openBondedV8(c, contract, t, b, r)
	return e
}

// Identical to fod-delivery-core's proofable ECIES construction. A fresh scalar
// is persisted with the exact trade before the ciphertext can be submitted.
func encryptBondedV8(pubBytes, plain, scalar []byte) ([]byte, error) {
	pub, e := crypto.UnmarshalPubkey(pubBytes)
	if e != nil {
		return nil, e
	}
	ephemeral, e := crypto.ToECDSA(scalar)
	if e != nil {
		return nil, e
	}
	x, _ := crypto.S256().ScalarMult(pub.X, pub.Y, scalar)
	z := x.FillBytes(make([]byte, 32))
	k1 := sha256.Sum256(append([]byte{0, 0, 0, 1}, z...))
	k2 := sha256.Sum256(append([]byte{0, 0, 0, 2}, z...))
	macKey := sha256.Sum256(k1[16:])
	block, e := aes.NewCipher(k1[:16])
	if e != nil {
		return nil, e
	}
	body := make([]byte, 16+len(plain))
	copy(body, k2[:16])
	cipher.NewCTR(block, k2[:16]).XORKeyStream(body[16:], plain)
	mac := hmac.New(sha256.New, macKey[:])
	mac.Write(body)
	out := append(crypto.FromECDSAPub(&ephemeral.PublicKey), body...)
	return append(out, mac.Sum(nil)...), nil
}
func runBondedV8Content(c *client, contract, command string, r *bondedV8Request, out string) error {
	if command == "deliveryKey" {
		// Exclusive create: a saved buyer key must never be replaced on retry.
		key, e := crypto.GenerateKey()
		if e != nil {
			return e
		}
		return writeNewPoolV7PrivateFile(out, []byte(hex.EncodeToString(crypto.FromECDSA(key))))
	}
	if command == "listing" {
		if e := fillBondedV9Listing(c, contract, r, os.Getenv("FOD_PASSPHRASE")); e != nil {
			return e
		}
		raw, e := json.Marshal(r)
		if e != nil {
			return e
		}
		return writeNewPoolV7PrivateFile(out, raw)
	}
	if command == "registration" {
		session, e := readVerifiedFastSession(r.SessionFile)
		if e != nil {
			return e
		}
		if e = directVerifiedSession(c, session); e != nil {
			return e
		}
		if session.Author != r.Caller {
			return errors.New("only the canonical bundle author may register")
		}
		wire, e := encodeFastRegistrationWire(session)
		if e != nil {
			return e
		}
		r.SessionWire = hex.EncodeToString(wire)
		r.SessionFile = ""
		if _, _, e = encodeBondedV8(r); e != nil {
			return e
		}
		raw, e := json.Marshal(r)
		if e != nil {
			return e
		}
		return writeNewPoolV7PrivateFile(out, raw)
	}
	t, e := readBondedV8Trade(c, contract, r.ID)
	if e != nil {
		return e
	}
	b, e := readBondedV8Bundle(c, contract, t.Bundle)
	if e != nil {
		return e
	}
	if _, e = readPoolV7SyncedHead(c); e != nil {
		return e
	}
	switch command {
	case "preview":
		preview, err := readBondedPreview(c, contract, t, b, r)
		if err != nil {
			return err
		}
		return writePrivateJSON(out, preview)
	case "contentWitness":
		if r.version() != 9 || r.Caller != t.Seller || (t.State != 3 && t.State != 4) {
			return errors.New("content dispute witness requires the assigned seller and an unsettled delivery")
		}
		secrets, vault, e := directVaultSecrets(r.VaultFile, os.Getenv("FOD_PASSPHRASE"))
		if e != nil {
			return e
		}
		defer clear(secrets)
		digest := sha256.Sum256(secrets)
		author, e := directAddress(vault.Address)
		if e != nil || author != b.Author || vault.Epoch != b.Epoch || hex.EncodeToString(digest[:]) != b.Keys {
			return errors.New("vault differs from the listed original content")
		}
		retained, e := retainBondedV8(c, contract, t, b)
		if e != nil {
			return e
		}
		blobs := make([]map[string]string, 0, len(retained.Availability.Blobs))
		for _, blob := range retained.Availability.Blobs {
			blobs = append(blobs, map[string]string{"blobCid": blob.CID, "blob": blob.Hex})
		}
		// New dispute-specific input, never a rewrite of a pre-session witness.
		raw, e := json.Marshal(map[string]any{"format": "fod.bonded-v9.content-witness", "contract": contract, "author": b.Author, "epoch": b.Epoch, "publicPartSecret": vault.PublicPartSec, "privatePartSecret": vault.PrivatePartSec, "blobs": blobs, "inventoryCommitment": b.Inventory, "keyCommitment": b.Keys, "trade": t.ID})
		if e != nil {
			return e
		}
		defer clear(raw)
		return writeNewPoolV7PrivateFile(out, raw)
	case "retain":
		if r.Caller != t.Buyer || t.State != 1 {
			return errors.New("readiness belongs to the assigned buyer")
		}
		a, e := retainBondedV8(c, contract, t, b)
		if e != nil {
			return e
		}
		raw, e := json.Marshal(a)
		if e != nil {
			return e
		}
		return writeNewPoolV7PrivateFile(out, raw)
	case "receive":
		if r.Caller != t.Buyer || !(t.State == 3 || t.State == 4 || t.State == 5) {
			return errors.New("no buyer delivery to verify")
		}
		vault, e := openBondedV8(c, contract, t, b, r)
		if e != nil {
			return e
		}
		// An encrypted local vault must be saved before automatic acceptance.
		pass := os.Getenv("FOD_PASSPHRASE")
		if pass == "" {
			return errors.New("a passphrase is required to retain received keys")
		}
		raw, e := json.Marshal(vault)
		if e != nil {
			return e
		}
		return WriteVault(out, pass, raw)
	case "delivery":
		if r.Caller != t.Seller || t.State != 2 {
			return errors.New("seller delivery is not due")
		}
		secrets, vault, e := directVaultSecrets(r.VaultFile, os.Getenv("FOD_PASSPHRASE"))
		if e != nil {
			return e
		}
		defer clear(secrets)
		h := sha256.Sum256(secrets)
		author, e := directAddress(vault.Address)
		if e != nil || author != b.Author || vault.Epoch != b.Epoch || hex.EncodeToString(h[:]) != b.Keys {
			return errors.New("seller vault differs from the listed package")
		}
		ephemeral, e := crypto.GenerateKey()
		if e != nil {
			return e
		}
		scalar := crypto.FromECDSA(ephemeral)
		defer clear(scalar)
		pub, _ := hex.DecodeString(t.DeliveryKey)
		ct, e := encryptBondedV8(pub, secrets, scalar)
		if e != nil {
			return e
		}
		id, _ := strconv.ParseUint(t.ID, 10, 64)
		keys, _, e := epochKeysFromDeliveredSecrets(secrets)
		if e != nil {
			return e
		}
		public, private := keys.SecretBytes()
		witness := map[string]any{"contract": contract, "network": NetworkNamePinned, "funded_at": t.FundedAt, "funding_root": t.FundingRoot, "offer_id": id, "order_nonce": 1, "seller": t.Seller, "buyer": t.Buyer, "buyer_key": t.DeliveryKey, "epoch": b.Epoch, "author": b.Author, "public_part_secret": hex.EncodeToString(public), "private_part_secret": hex.EncodeToString(private), "ephemeral": hex.EncodeToString(scalar), "note": "private bonded v8 dispute witness"}
		raw, e := json.Marshal(witness)
		if e != nil {
			return e
		}
		if e = writeNewPoolV7PrivateFile(out+".witness.json", raw); e != nil {
			return e
		}
		result := bondedV8Request{ProtocolVersion: r.ProtocolVersion, Operation: "deliver", Caller: r.Caller, ID: t.ID, Ciphertext: hex.EncodeToString(ct)}
		raw, e = json.Marshal(result)
		if e != nil {
			return e
		}
		return writeNewPoolV7PrivateFile(out, raw)
	default:
		return errors.New("unsupported v8 content operation")
	}
}
