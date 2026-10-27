package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/idena-network/idena-go/crypto"
)

func bondedBundleID(version int, author string, epoch uint16) string {
	b, _ := hex.DecodeString(author)
	raw := append([]byte("fod:bundle:v"+strconv.Itoa(version)), b...)
	raw = binary.BigEndian.AppendUint16(raw, epoch)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// A listing asserts a key commitment, not a proven one. Validate every original
// CID and its decrypted format locally before risking the seller's collateral.
// No keys or plaintext are included in the resulting public journal.
func fillBondedV9Listing(c *client, contract string, r *bondedV8Request, pass string) error {
	if r.version() != 9 || r.Operation != "queueSell" || r.Seal != "" {
		return errors.New("invalid optimistic listing")
	}
	session, e := readVerifiedFastSession(r.SessionFile)
	if e != nil {
		return e
	}
	if e = directVerifiedSession(c, session); e != nil {
		return e
	}
	verdict, e := readBondedV9SessionVerdict(session)
	if e != nil {
		return e
	}
	if !verdict.Clean || int(verdict.Flips) != len(session.Manifest) {
		return errors.New("validation result does not qualify this flip set for v9 listing")
	}
	if session.Author != r.Caller {
		return errors.New("only the canonical author can list this package")
	}
	secrets, vault, e := directVaultSecrets(r.VaultFile, pass)
	if e != nil {
		return e
	}
	defer clear(secrets)
	author, e := directAddress(vault.Address)
	if e != nil || author != session.Author || vault.Epoch != session.Epoch {
		return errors.New("vault differs from canonical package")
	}
	keys, _, e := epochKeysFromDeliveredSecrets(secrets)
	if e != nil {
		return e
	}
	for _, cid := range session.Manifest {
		blob, e := ipfsFetch(c)(cid)
		if e != nil {
			return errors.New("original encrypted flip unavailable; collect it before listing")
		}
		if len(blob) > maxRetainedFlipBytes {
			return errors.New("oversized original flip")
		}
		if ok, _ := verifyBlobCID(cid, blob, nil); !ok {
			return errors.New("original flip CID mismatch")
		}
		pub, publicCipher, privateCipher, e := ParseFlipBlob(blob)
		if e != nil {
			return e
		}
		publicKey, e := crypto.UnmarshalPubkey(pub)
		if e != nil || hex.EncodeToString(crypto.PubkeyToAddress(*publicKey).Bytes()) != session.Author {
			return errors.New("original flip author mismatch")
		}
		public, e := keys.DecryptPublicPart(publicCipher)
		if e != nil {
			return errors.New("vault cannot open original public flip")
		}
		private, e := keys.DecryptPrivatePart(privateCipher)
		if e != nil {
			clear(public)
			return errors.New("vault cannot open original private flip")
		}
		e = validateDirectFlipParts(public, private)
		clear(public)
		clear(private)
		if e != nil {
			return e
		}
	}
	wire, e := encodeFastRegistrationWire(session)
	if e != nil {
		return e
	}
	j := make([]byte, 273)
	binary.BigEndian.PutUint32(j[:4], 1)
	put := func(start int, value string) { b, _ := hex.DecodeString(value); copy(j[start:], b) }
	if _, e = v8hex(contract, 20); e != nil {
		return e
	}
	put(4, labelDigest(NetworkNamePinned))
	put(36, contract)
	put(56, session.Author)
	binary.BigEndian.PutUint16(j[76:78], session.Epoch)
	binary.BigEndian.PutUint16(j[78:80], verdict.Flips)
	put(80, labelDigest(ConsensusVersionPinned))
	put(112, labelDigest(EligibilityPolicyV1))
	put(144, inventoryCommitment(session.Author, session.Epoch, session.Manifest))
	digest := sha256.Sum256(secrets)
	copy(j[241:], digest[:])
	r.Bundle = bondedBundleID(9, session.Author, session.Epoch)
	r.Journal = hex.EncodeToString(j)
	r.SessionWire = hex.EncodeToString(wire)
	return nil
}
