package main

// Session-content export (`SPEC.md` 6.7).
//
// What a provider hands to a session: the decoded flip material, the source
// CIDs and provenance, and any consensus answers it can vouch for. What it
// deliberately never contains: the epoch key pairs. Participants get the
// content, not the keys that open other people's bundles.
//
// The answers section is only filled from evidence the provider supplies. It is
// never derived from a guess: mapping answers to flips needs the lottery
// assignment, which is the same external data the eligibility half of Package 2
// waits on, so an export without it says so instead of inventing one.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

const exportFormat = "fod.session-content"
const exportVersion = 1
const manifestDomain = "fod.bundle-manifest:v1"

// ExportedPart is one decoded half of a flip, written beside the manifest.
type ExportedPart struct {
	File   string `json:"file"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// ExportedFlip is one challenge's material plus where it came from.
type ExportedFlip struct {
	CID         string       `json:"cid"`
	Pair        uint8        `json:"pair"`
	TxHash      string       `json:"txHash,omitempty"`
	BlockHeight uint64       `json:"blockHeight,omitempty"`
	PublicPart  ExportedPart `json:"publicPart"`
	PrivatePart ExportedPart `json:"privatePart"`
}

// ExportedAnswer is a consensus answer the provider vouches for. The digest is
// over the canonical answer text, the same rule the session scores with.
type ExportedAnswer struct {
	CID        string `json:"cid"`
	Digest     string `json:"answerDigest"`
	Evidence   string `json:"evidence"`
	RecordedAt string `json:"recordedAt,omitempty"`
}

// SessionContentExport is the exported bundle.
type SessionContentExport struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Bundle  struct {
		Author             string `json:"author"`
		Epoch              uint16 `json:"epoch"`
		ManifestCommitment string `json:"manifestCommitment"`
		Flips              int    `json:"flips"`
		Source             string `json:"source"`
		InventoryComplete  bool   `json:"inventoryComplete"`
	} `json:"bundle"`
	Flips      []ExportedFlip   `json:"flips"`
	Answers    []ExportedAnswer `json:"answers"`
	AnswersBy  string           `json:"answersEvidence"`
	Keys       string           `json:"keys"`
	ExportedAt string           `json:"exportedAt"`
	Tool       string           `json:"tool"`
}

// inventoryCommitment binds author, epoch, and the sorted, non-deleted CIDs. It
// is the part of a `BundleManifest`'s commitment that covers the inventory; the
// manifest adds the validation anchor, the consensus version, and the eligibility
// policy on top of it (`manifest.go`).
func inventoryCommitment(author string, epoch uint16, cids []string) string {
	sorted := append([]string(nil), cids...)
	sort.Strings(sorted)
	hasher := sha256.New()
	hasher.Write([]byte(manifestDomain))
	hasher.Write([]byte(strings.ToLower(strings.TrimPrefix(author, "0x"))))
	hasher.Write([]byte{byte(epoch >> 8), byte(epoch)})
	for _, cid := range sorted {
		hasher.Write([]byte{0})
		hasher.Write([]byte(cid))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// ExportOptions is what the CLI passes in.
type ExportOptions struct {
	InventoryPath string
	VaultPath     string
	Passphrase    string
	BlobDir       string
	OutDir        string
	AnswersPath   string
	AnswersSource string
}

// ExportSessionContent decrypts a bundle's blobs and writes the export.
func ExportSessionContent(options ExportOptions) (*SessionContentExport, error) {
	rawInventory, err := os.ReadFile(options.InventoryPath)
	if err != nil {
		return nil, err
	}
	var inventory BundleInventory
	if err := json.Unmarshal(rawInventory, &inventory); err != nil {
		return nil, fmt.Errorf("parse inventory: %w", err)
	}
	if inventory.Author == "" || inventory.Epoch == 0 {
		return nil, fmt.Errorf("inventory does not name an author and epoch")
	}
	seen := map[string]bool{}
	for _, entry := range inventory.Entries {
		if entry.Deleted {
			continue
		}
		if _, err := cidBytesFromString(entry.CID); err != nil || !(isRawCID(entry.CID) || isDagPBCID(entry.CID)) || seen[entry.CID] {
			return nil, fmt.Errorf("inventory has an invalid or duplicate CID filename")
		}
		seen[entry.CID] = true
	}

	payload, err := ReadVault(options.VaultPath, options.Passphrase)
	if err != nil {
		return nil, err
	}
	var stored VaultPayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, fmt.Errorf("parse vault payload: %w", err)
	}
	if stored.Kind != "fod.epoch-keys" {
		return nil, fmt.Errorf("vault does not hold epoch keys (kind=%q)", stored.Kind)
	}
	keys, err := EpochKeysFromVaultPayload(stored)
	if err != nil {
		return nil, err
	}
	if uint16(stored.Epoch) != inventory.Epoch {
		return nil, fmt.Errorf("vault holds epoch %d but the inventory is epoch %d", stored.Epoch, inventory.Epoch)
	}
	if !isHex(strings.TrimPrefix(inventory.Author, "0x"), addressBytes) ||
		!strings.EqualFold(strings.TrimPrefix(keys.Address, "0x"), strings.TrimPrefix(inventory.Author, "0x")) {
		return nil, fmt.Errorf("vault and inventory must name the same author")
	}

	if err := os.MkdirAll(options.OutDir, 0o700); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	outRoot, err := os.OpenRoot(options.OutDir)
	if err != nil {
		return nil, err
	}
	defer outRoot.Close()
	if err := outRoot.MkdirAll("flips", 0o700); err != nil {
		return nil, err
	}
	if info, err := outRoot.Lstat("flips"); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("the flips output directory must not be a symlink")
	}
	blobRoot, err := os.OpenRoot(options.BlobDir)
	if err != nil {
		return nil, err
	}
	defer blobRoot.Close()

	export := &SessionContentExport{
		Format:     exportFormat,
		Version:    exportVersion,
		Flips:      []ExportedFlip{},
		Answers:    []ExportedAnswer{},
		Keys:       "excluded: this export carries decoded content, never epoch key pairs",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Tool:       "fod",
	}
	export.Bundle.Author = strings.ToLower(strings.TrimPrefix(inventory.Author, "0x"))
	export.Bundle.Epoch = inventory.Epoch
	export.Bundle.Source = fmt.Sprintf("inventory %s (%d submissions, %d verified blobs, %d deleted)",
		filepath.Base(options.InventoryPath), inventory.Submissions, inventory.BlobsOk, inventory.Deleted)
	export.Bundle.InventoryComplete = inventory.Complete

	cids := []string{}
	for _, entry := range inventory.Entries {
		if entry.Deleted || entry.CID == "" {
			continue
		}
		cids = append(cids, entry.CID)
		file, err := blobRoot.Open(entry.CID)
		if err != nil {
			return nil, fmt.Errorf("blob for %s is not in %s: %w", entry.CID, options.BlobDir, err)
		}
		blob, err := io.ReadAll(io.LimitReader(file, maxRetainedFlipBytes+1))
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("read blob %s: %w", entry.CID, err)
		}
		if len(blob) > maxRetainedFlipBytes {
			return nil, fmt.Errorf("blob %s exceeds the supported flip size", entry.CID)
		}
		if ok, note := verifyBlobCID(entry.CID, blob, nil); !ok {
			return nil, fmt.Errorf("blob %s is not authenticated: %s", entry.CID, note)
		}
		pub, publicCipher, privateCipher, err := ParseFlipBlob(blob)
		if err != nil {
			return nil, fmt.Errorf("parse blob %s: %w", entry.CID, err)
		}
		authorKey, err := crypto.UnmarshalPubkey(pub)
		if err != nil || !strings.EqualFold(crypto.PubkeyToAddress(*authorKey).Hex(), "0x"+strings.TrimPrefix(inventory.Author, "0x")) {
			return nil, fmt.Errorf("blob %s does not belong to the inventory author", entry.CID)
		}
		publicPlain, err := keys.DecryptPublicPart(publicCipher)
		if err != nil {
			return nil, fmt.Errorf("decrypt public part of %s: %w", entry.CID, err)
		}
		privatePlain, err := keys.DecryptPrivatePart(privateCipher)
		if err != nil {
			return nil, fmt.Errorf("decrypt private part of %s: %w", entry.CID, err)
		}
		publicFile := filepath.Join("flips", entry.CID+".public")
		privateFile := filepath.Join("flips", entry.CID+".private")
		if err := writeRootPrivateBytes(outRoot, privateFile, privatePlain); err != nil {
			return nil, err
		}
		if err := writeRootPrivateBytes(outRoot, publicFile, publicPlain); err != nil {
			return nil, err
		}
		export.Flips = append(export.Flips, ExportedFlip{
			CID:         entry.CID,
			Pair:        entry.Pair,
			TxHash:      entry.TxHash,
			BlockHeight: entry.BlockHeight,
			PublicPart:  describePart(publicFile, publicPlain),
			PrivatePart: describePart(privateFile, privatePlain),
		})
	}
	export.Bundle.Flips = len(export.Flips)
	export.Bundle.ManifestCommitment = inventoryCommitment(export.Bundle.Author, export.Bundle.Epoch, cids)

	if options.AnswersPath != "" {
		answers, err := loadExportedAnswers(options.AnswersPath, options.AnswersSource)
		if err != nil {
			return nil, err
		}
		export.Answers = answers
		export.AnswersBy = options.AnswersSource
	} else {
		export.AnswersBy = "none: this export carries no consensus answers"
	}

	encoded, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeRootPrivateBytes(outRoot, "content-export.json", append(encoded, '\n')); err != nil {
		return nil, fmt.Errorf("write export: %w", err)
	}
	return export, nil
}

func writeRootPrivateBytes(root *os.Root, name string, data []byte) error {
	return writeRootPrivateFile(root, name, true, func(file *os.File) error {
		_, err := file.Write(data)
		return err
	})
}

func writeExportPart(path string, data []byte, mode os.FileMode) error {
	if mode != VaultFileMode {
		return fmt.Errorf("export parts must be private")
	}
	return writePrivateBytes(path, data)
}

func describePart(file string, data []byte) ExportedPart {
	digest := sha256.Sum256(data)
	return ExportedPart{
		File:   filepath.ToSlash(file),
		Bytes:  len(data),
		SHA256: hex.EncodeToString(digest[:]),
	}
}

// loadExportedAnswers reads answers the provider has evidence for: a JSON list
// of {cid, answerDigest, evidence}. Nothing is inferred.
func loadExportedAnswers(path, source string) ([]ExportedAnswer, error) {
	if source == "" {
		return nil, fmt.Errorf("answers were supplied without naming their evidence")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var answers []ExportedAnswer
	if err := json.Unmarshal(raw, &answers); err != nil {
		return nil, fmt.Errorf("parse answers: %w", err)
	}
	for index, answer := range answers {
		if answer.CID == "" || !isSHA256Hex(answer.Digest) {
			return nil, fmt.Errorf("answers[%d] needs a cid and a sha256 answerDigest", index)
		}
		if answer.Evidence == "" {
			return nil, fmt.Errorf("answers[%d] names no evidence", index)
		}
	}
	return answers, nil
}

func isSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
