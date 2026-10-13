package main

// Fast buyers authenticate the on-chain certificate through their own node,
// then retain every original blob and encrypted sale copy before funding.

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const fastBundleRecordBytes = 105
const fastCertificateMaxBytes = fastContentJournalBytes + fastContentSealBytes + 2 + fastRegistrationMaxWireBytes
const fastCopyMaxBytes = maxRetainedFlipBytes + 45

type fastBundleRecord struct {
	Epoch        uint16
	Count        uint16
	Flags        uint32
	Copies       string
	Point        string
	OriginalKeys string
}

type retainedFastCopy struct {
	OriginalCID string `json:"originalCid"`
	CopyCID     string `json:"copyCid"`
	Hex         string `json:"hex"`
}

type fastBuyEvidence struct {
	CertificateSHA256 string             `json:"certificateSha256"`
	Copies            []retainedFastCopy `json:"copies"`
}

func registryFastBundle(c *client, contract, bundle string) (*fastBundleRecord, error) {
	records, err := readPrefix(c, registryAddress(contract), "pfast/")
	if err != nil {
		return nil, err
	}
	raw := records[strings.ToLower(strings.TrimPrefix(bundle, "0x"))]
	if len(raw) != fastBundleRecordBytes {
		return nil, errors.New("missing or invalid fast bundle record")
	}
	return &fastBundleRecord{
		Epoch:        binary.BigEndian.Uint16(raw[:2]),
		Count:        binary.BigEndian.Uint16(raw[2:4]),
		Flags:        binary.BigEndian.Uint32(raw[4:8]),
		Copies:       hex.EncodeToString(raw[8:40]),
		Point:        hex.EncodeToString(raw[40:73]),
		OriginalKeys: hex.EncodeToString(raw[73:105]),
	}, nil
}

func registryFastCertificate(c *client, contract, bundle string) ([]byte, error) {
	bundle = strings.ToLower(strings.TrimPrefix(bundle, "0x"))
	if !isHex(bundle, poolBundleBytes) {
		return nil, errors.New("invalid fast bundle ID")
	}
	certificate, err := readRegistryValue(c, strings.TrimPrefix(strings.ToLower(contract), "0x"), "pfcert/"+bundle)
	if err != nil {
		return nil, err
	}
	if len(certificate) <= fastContentJournalBytes+fastContentSealBytes+2 || len(certificate) > fastCertificateMaxBytes {
		return nil, errors.New("fast certificate is missing or outside the bounded size")
	}
	wireLen := int(binary.BigEndian.Uint16(certificate[fastContentJournalBytes+fastContentSealBytes:]))
	if wireLen == 0 || len(certificate) != fastContentJournalBytes+fastContentSealBytes+2+wireLen {
		return nil, errors.New("fast certificate witness length is inconsistent")
	}
	return certificate, nil
}

// validateFastChainCertificate compares the state value to the pool and fast
// records the contract created in the same successful registration call.
func validateFastChainCertificate(contract, bundle string, listing *marketPoolListing, fast *fastBundleRecord, certificate []byte, expectedImage string) error {
	if expectedImage == strings.Repeat("0", 64) || !isHex(expectedImage, 32) {
		return errors.New("fast content guest image is not pinned")
	}
	if listing == nil || fast == nil || len(certificate) <= fastContentJournalBytes+fastContentSealBytes+2 || len(certificate) > fastCertificateMaxBytes {
		return errors.New("missing fast registration state")
	}
	journal := certificate[:fastContentJournalBytes]
	wire := certificate[fastContentJournalBytes+fastContentSealBytes+2:]
	if binary.BigEndian.Uint32(journal[:4]) != 1 || int(binary.BigEndian.Uint16(certificate[fastContentJournalBytes+fastContentSealBytes:])) != len(wire) {
		return errors.New("invalid fast certificate encoding")
	}
	id := sha256.Sum256(journal)
	contract = strings.TrimPrefix(strings.ToLower(contract), "0x")
	bundle = strings.TrimPrefix(strings.ToLower(bundle), "0x")
	if !isHex(contract, addressBytes) || hex.EncodeToString(id[:]) != bundle || hex.EncodeToString(journal[36:56]) != contract ||
		hex.EncodeToString(journal[56:76]) != listing.Author || hex.EncodeToString(journal[144:176]) != listing.InventoryCommitment ||
		binary.BigEndian.Uint16(journal[76:78]) != fast.Epoch || binary.BigEndian.Uint16(journal[78:80]) != fast.Count ||
		uint32(fast.Count) != listing.FlipCount || hex.EncodeToString(journal[176:208]) != fast.Copies ||
		hex.EncodeToString(journal[208:241]) != fast.Point || hex.EncodeToString(journal[241:273]) != fast.OriginalKeys {
		return errors.New("fast certificate disagrees with canonical bundle state")
	}
	if hex.EncodeToString(journal[4:36]) != labelDigest(NetworkNamePinned) ||
		hex.EncodeToString(journal[80:112]) != labelDigest(ConsensusVersionPinned) ||
		hex.EncodeToString(journal[112:144]) != labelDigest(EligibilityPolicyV1) {
		return errors.New("fast certificate uses another network, consensus, or policy")
	}
	manifest := sha256.New()
	manifest.Write([]byte("fod:fast:manifest:v1"))
	manifest.Write(journal)
	manifest.Write(wire)
	if hex.EncodeToString(manifest.Sum(nil)) != listing.ManifestCommitment {
		return errors.New("fast certificate does not match the registered session witness")
	}
	return nil
}

func validateFastPairs(proof *fastContentArtefact, certificate []byte, listing *marketPoolListing, fast *fastBundleRecord, expectedImage string) error {
	if proof == nil || proof.Format != "fod:fast-content-sealed:v1" || proof.Statement != fastContentStatement ||
		!strings.EqualFold(proof.ImageID, expectedImage) || proof.SealBytes != fastContentSealBytes {
		return errors.New("invalid public fast content artefact")
	}
	journal := certificate[:fastContentJournalBytes]
	seal := certificate[fastContentJournalBytes : fastContentJournalBytes+fastContentSealBytes]
	if !strings.EqualFold(proof.Journal, hex.EncodeToString(journal)) || !strings.EqualFold(proof.Seal, hex.EncodeToString(seal)) {
		return errors.New("published content proof differs from canonical certificate")
	}
	claim, err := claimDigestFromHex(expectedImage, journal)
	if err != nil || !strings.EqualFold(proof.ClaimDigest, hex.EncodeToString(claim[:])) {
		return errors.New("published content claim differs from canonical certificate")
	}
	if len(proof.Items) != int(fast.Count) {
		return errors.New("public copy list has wrong count")
	}
	originals := make([]string, len(proof.Items))
	for i, item := range proof.Items {
		originals[i] = item.OriginalCID
	}
	if inventoryCommitment(listing.Author, fast.Epoch, originals) != listing.InventoryCommitment {
		return errors.New("public copy list differs from original flip inventory")
	}
	copies, err := fastCopiesCommitment(proof.Items)
	if err != nil || copies != fast.Copies {
		return errors.New("public copy list differs from sealed copies")
	}
	return nil
}

func fetchFastCopies(c *client, items []fastCopyPair) ([]retainedFastCopy, error) {
	fetch := ipfsFetch(c)
	retained := make([]retainedFastCopy, 0, len(items))
	for _, item := range items {
		body, err := fetch(item.CopyCID)
		if err != nil || len(body) == 0 || len(body) > fastCopyMaxBytes {
			return nil, fmt.Errorf("cannot fetch bounded encrypted copy for %s", item.CopyCID)
		}
		if ok, _ := verifyBlobCID(item.CopyCID, body, nil); !ok {
			return nil, fmt.Errorf("encrypted copy %s does not rebuild its CID", item.CopyCID)
		}
		retained = append(retained, retainedFastCopy{
			OriginalCID: item.OriginalCID,
			CopyCID:     item.CopyCID,
			Hex:         hex.EncodeToString(body),
		})
	}
	return retained, nil
}

func prepareFastPoolBuyFromRequest(c *client, request *actionRequest, arguments *buyRequestArguments, listing *marketPoolListing, identity *ecdsa.PrivateKey, deliveryKey *ecdsa.PrivateKey, proofPath string) (*PoolBuy, error) {
	if fastContentImageIDHex == strings.Repeat("0", 64) {
		return nil, errors.New("fast content guest image is not pinned")
	}
	var proof fastContentArtefact
	if err := readFastPublicJSON(proofPath, &proof); err != nil {
		return nil, err
	}
	fast, err := registryFastBundle(c, request.Contract, arguments.BundleID)
	if err != nil {
		return nil, err
	}
	certificate, err := registryFastCertificate(c, request.Contract, arguments.BundleID)
	if err != nil {
		return nil, err
	}
	if err := validateFastChainCertificate(request.Contract, arguments.BundleID, listing, fast, certificate, fastContentImageIDHex); err != nil {
		return nil, err
	}
	if err := validateFastPairs(&proof, certificate, listing, fast, fastContentImageIDHex); err != nil {
		return nil, err
	}
	advisory := &AvailabilityAdvisory{Author: listing.Author, Epoch: fast.Epoch, Flips: make([]AvailabilityFlip, len(proof.Items))}
	for i, item := range proof.Items {
		advisory.Flips[i] = AvailabilityFlip{CID: item.OriginalCID}
	}
	check := recheckAvailability(advisory, oneSource(ipfsFetch(c), "the buyer's node"), nil)
	check.File = proofPath
	check.Binding = bindingCommitted
	check.InventoryCommitment = listing.InventoryCommitment
	if !check.Complete() {
		return nil, errors.New("the buyer could not verify every original flip from its own node")
	}
	copies, err := fetchFastCopies(c, proof.Items)
	if err != nil {
		return nil, err
	}
	buyer, err := nodekeyAddressOf(identity)
	if err != nil {
		return nil, err
	}
	artefact, err := preparePoolBuy(request.Contract, arguments.BundleID, buyer, listing.Nonce, listing.BuyQuote, request.Network, arguments.Fees, uint64(listing.FlipCount), deliveryKey)
	if err != nil {
		return nil, err
	}
	certificateDigest := sha256.Sum256(certificate)
	artefact.Availability = check
	artefact.Fast = &fastBuyEvidence{CertificateSHA256: hex.EncodeToString(certificateDigest[:]), Copies: copies}
	return artefact, nil
}

func validateFastBuyEvidence(c *client, prepared *preparedAction, listing *marketPoolListing) error {
	if prepared.Fast == nil {
		return errors.New("fast purchase has no retained copy evidence")
	}
	fast, err := registryFastBundle(c, prepared.Contract, prepared.Bundle)
	if err != nil {
		return err
	}
	certificate, err := registryFastCertificate(c, prepared.Contract, prepared.Bundle)
	if err != nil {
		return err
	}
	if err := validateFastChainCertificate(prepared.Contract, prepared.Bundle, listing, fast, certificate, fastContentImageIDHex); err != nil {
		return err
	}
	digest := sha256.Sum256(certificate)
	if hex.EncodeToString(digest[:]) != prepared.Fast.CertificateSHA256 || len(prepared.Fast.Copies) != int(fast.Count) {
		return errors.New("retained fast evidence differs from the canonical certificate")
	}
	items := make([]fastCopyPair, 0, len(prepared.Fast.Copies))
	originals := make([]string, 0, len(prepared.Fast.Copies))
	for _, retained := range prepared.Fast.Copies {
		body, err := hex.DecodeString(retained.Hex)
		if err != nil || len(body) == 0 || len(body) > fastCopyMaxBytes {
			return errors.New("missing or overlong retained encrypted copy")
		}
		if ok, _ := verifyBlobCID(retained.CopyCID, body, nil); !ok {
			return errors.New("retained encrypted copy does not rebuild its CID")
		}
		items = append(items, fastCopyPair{OriginalCID: retained.OriginalCID, CopyCID: retained.CopyCID})
		originals = append(originals, retained.OriginalCID)
	}
	if copies, err := fastCopiesCommitment(items); err != nil || copies != fast.Copies ||
		inventoryCommitment(listing.Author, fast.Epoch, originals) != listing.InventoryCommitment {
		return errors.New("retained encrypted copies differ from the registered flip inventory")
	}
	if prepared.Availability == nil || len(prepared.Availability.Blobs) != len(originals) {
		return errors.New("original flip bytes were not retained")
	}
	for i, original := range originals {
		if prepared.Availability.Blobs[i].CID != original {
			return errors.New("retained originals and sale copies name different flips")
		}
	}
	return nil
}
