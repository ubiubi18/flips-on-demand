package main

// A fast registration carries a sealed proof of every original flip and a
// public, root-checked session witness. No seller keys enter this call.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	operationRegisterFast       = "registerFast"
	preparedFastRegisterFormat  = "fod.prepared-register-fast"
	fastContentStatement        = "fod:fast-content:v1"
	fastContentJournalBytes     = 273
	fastContentSealBytes        = 256
	fastContentImageIDHex       = "b8586d40d7d5b912a1ce3863009c9ccb88eb836d2b441a3a08ce32ecc85bd7d2"
	maxFastRegistrationFileSize = 2 << 20
)

type fastCopyPair struct {
	OriginalCID string `json:"originalCid"`
	CopyCID     string `json:"copyCid"`
}

type fastContentArtefact struct {
	Format      string         `json:"format"`
	Statement   string         `json:"statement"`
	Journal     string         `json:"journal"`
	Seal        string         `json:"seal"`
	SealBytes   int            `json:"sealBytes"`
	ClaimDigest string         `json:"claim_digest"`
	ImageID     string         `json:"image_id"`
	Items       []fastCopyPair `json:"items"`
}

type preparedFastRegistration struct {
	Format              string `json:"format"`
	Version             int    `json:"version"`
	Operation           string `json:"operation"`
	Network             string `json:"network"`
	Contract            string `json:"contract"`
	Author              string `json:"author"`
	Epoch               uint16 `json:"epoch"`
	CoveredFlips        uint32 `json:"coveredFlips"`
	Bundle              string `json:"bundle"`
	InventoryCommitment string `json:"inventoryCommitment"`
	ImageID             string `json:"imageId"`
	ClaimDigest         string `json:"claimDigest"`
	Calldata            string `json:"calldata"`
	Amount              string `json:"amount"`
	PreparedAt          string `json:"preparedAt"`
	Submitted           bool   `json:"submitted"`
}

func readFastPublicJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFastRegistrationFileSize {
		return errors.New("public evidence is not a bounded regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(raw) > maxFastRegistrationFileSize {
		return errors.New("public evidence exceeds size limit")
	}
	return decodeStrict(raw, value)
}

func fastCopiesCommitment(items []fastCopyPair) (string, error) {
	if len(items) == 0 || len(items) > fastRegistrationMaxFlips {
		return "", errors.New("invalid copy pair count")
	}
	h := sha256.New()
	h.Write([]byte("fod:fast-content:copies:v1"))
	var count [2]byte
	binary.BigEndian.PutUint16(count[:], uint16(len(items)))
	h.Write(count[:])
	previous := ""
	for _, item := range items {
		if len(item.OriginalCID) == 0 || len(item.OriginalCID) > 128 || len(item.CopyCID) == 0 || len(item.CopyCID) > 128 ||
			(previous != "" && previous >= item.OriginalCID) {
			return "", errors.New("copy pairs are empty, overlong, or unsorted")
		}
		for _, cid := range []string{item.OriginalCID, item.CopyCID} {
			decoded, err := cidBytesFromString(cid)
			if err != nil || cidString(decoded) != cid {
				return "", errors.New("invalid or noncanonical copy pair CID")
			}
			binary.BigEndian.PutUint16(count[:], uint16(len(cid)))
			h.Write(count[:])
			h.Write([]byte(cid))
		}
		previous = item.OriginalCID
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// assembleFastRegistration takes only public artefacts. It checks every field
// that can be checked before the contract repeats the seal and header checks.
func assembleFastRegistration(network, contract string, proof *fastContentArtefact, session *SessionBundle, expectedImage string) (*preparedFastRegistration, error) {
	if proof == nil || session == nil || proof.Format != "fod:fast-content-sealed:v1" || proof.Statement != fastContentStatement {
		return nil, errors.New("missing sealed content proof or verified session")
	}
	if expectedImage == strings.Repeat("0", 64) || !isHex(expectedImage, 32) {
		return nil, errors.New("fast content guest image is not pinned")
	}
	if !strings.EqualFold(proof.ImageID, expectedImage) {
		return nil, errors.New("content proof image differs from the contract pin")
	}
	journal, err := hex.DecodeString(proof.Journal)
	if err != nil || len(journal) != fastContentJournalBytes || binary.BigEndian.Uint32(journal[:4]) != 1 {
		return nil, errors.New("invalid fast content journal")
	}
	seal, err := hex.DecodeString(proof.Seal)
	if err != nil || len(seal) != fastContentSealBytes || proof.SealBytes != fastContentSealBytes {
		return nil, errors.New("invalid fast content seal")
	}
	expectedClaim, err := claimDigestFromHex(expectedImage, journal)
	if err != nil || !strings.EqualFold(proof.ClaimDigest, hex.EncodeToString(expectedClaim[:])) {
		return nil, errors.New("the seal claim does not bind this journal and guest image")
	}
	contract = strings.ToLower(strings.TrimPrefix(contract, "0x"))
	if !isHex(contract, addressBytes) || network != NetworkNamePinned {
		return nil, errors.New("invalid contract or network")
	}
	author := hex.EncodeToString(journal[56:76])
	epoch := binary.BigEndian.Uint16(journal[76:78])
	count := binary.BigEndian.Uint16(journal[78:80])
	if hex.EncodeToString(journal[36:56]) != contract ||
		!strings.EqualFold(session.Author, author) || session.Epoch != epoch ||
		count == 0 || int(count) != len(session.Manifest) || int(count) != len(proof.Items) {
		return nil, errors.New("content proof disagrees with contract, author, epoch, or flip count")
	}
	if hex.EncodeToString(journal[4:36]) != labelDigest(NetworkNamePinned) ||
		hex.EncodeToString(journal[80:112]) != labelDigest(ConsensusVersionPinned) ||
		hex.EncodeToString(journal[112:144]) != labelDigest(EligibilityPolicyV1) {
		return nil, errors.New("content proof uses another network, consensus, or policy")
	}
	originals := slices.Clone(session.Manifest)
	slices.Sort(originals)
	if hex.EncodeToString(journal[144:176]) != inventoryCommitment(author, epoch, originals) {
		return nil, errors.New("content proof does not cover the exact session CID inventory")
	}
	for i, item := range proof.Items {
		if item.OriginalCID != originals[i] {
			return nil, errors.New("public copy list does not cover the sealed original inventory")
		}
	}
	copies, err := fastCopiesCommitment(proof.Items)
	if err != nil || copies != hex.EncodeToString(journal[176:208]) {
		return nil, errors.New("public copy list differs from the sealed copies commitment")
	}
	wire, err := encodeFastRegistrationWire(session)
	if err != nil {
		return nil, err
	}
	calldata := make([]byte, 0, len(journal)+len(seal)+2+len(wire))
	calldata = append(calldata, journal...)
	calldata = append(calldata, seal...)
	calldata = binary.BigEndian.AppendUint16(calldata, uint16(len(wire)))
	calldata = append(calldata, wire...)
	bundleID := sha256.Sum256(journal)
	return &preparedFastRegistration{
		Format:              preparedFastRegisterFormat,
		Version:             actionRequestVersion,
		Operation:           operationRegisterFast,
		Network:             network,
		Contract:            contract,
		Author:              author,
		Epoch:               epoch,
		CoveredFlips:        uint32(count),
		Bundle:              hex.EncodeToString(bundleID[:]),
		InventoryCommitment: hex.EncodeToString(journal[144:176]),
		ImageID:             strings.ToLower(expectedImage),
		ClaimDigest:         strings.ToLower(proof.ClaimDigest),
		Calldata:            hex.EncodeToString(calldata),
		Amount:              "0",
		PreparedAt:          time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func readVerifiedFastSession(path string) (*SessionBundle, error) {
	var claimed SessionBundle
	if err := readFastPublicJSON(path, &claimed); err != nil {
		return nil, err
	}
	if claimed.Format != sessionBundleFormat || claimed.Version != sessionBundleVersion {
		return nil, errors.New("invalid session bundle format")
	}
	verified, err := sessionBundleFromAnchors(claimed.Author, &claimed.Inventory, &claimed.Verdict)
	if err != nil {
		return nil, err
	}
	if claimed.Epoch != verified.Epoch || !slices.Equal(claimed.Manifest, verified.Manifest) {
		return nil, errors.New("session bundle differs from its verified state leaves")
	}
	return verified, nil
}

func checkFastSessionHeaders(c *client, session *SessionBundle) error {
	for _, anchor := range []SessionAnchor{session.Inventory, session.Verdict} {
		var block rpcBlock
		if err := c.call("bcn_blockAt", []any{anchor.Height}, &block); err != nil {
			return fmt.Errorf("read canonical header: %w", err)
		}
		if block.Height != anchor.Height || !strings.EqualFold(strings.TrimPrefix(block.Root, "0x"), anchor.RootHex) {
			return fmt.Errorf("session root differs from canonical header at height %d", anchor.Height)
		}
	}
	return nil
}

func runPrepareFastRegistration(c *client, proofPath, sessionPath, contract, network, preparedPath string) error {
	if proofPath == "" || sessionPath == "" || contract == "" || preparedPath == "" {
		return errors.New("prepare-fast-registration needs -proof FILE -bundle SESSION_FILE -contract ADDRESS -prepared FILE")
	}
	if fastContentImageIDHex == strings.Repeat("0", 64) {
		return errors.New("fast content guest image is not pinned")
	}
	var proof fastContentArtefact
	if err := readFastPublicJSON(proofPath, &proof); err != nil {
		return err
	}
	session, err := readVerifiedFastSession(sessionPath)
	if err != nil {
		return err
	}
	if err := checkFastSessionHeaders(c, session); err != nil {
		return err
	}
	prepared, err := assembleFastRegistration(network, contract, &proof, session, fastContentImageIDHex)
	if err != nil {
		return err
	}
	if err := writeJSON(preparedPath, prepared); err != nil {
		return err
	}
	fmt.Printf("prepared fast registration for %d verified flips; no transaction sent\n", prepared.CoveredFlips)
	return nil
}
