package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

var (
	testCID  = cidRawSha256([]byte("synthetic export fixture filename"))
	otherCID = cidRawSha256([]byte("deleted export fixture filename"))
)

// buildSyntheticBundle writes an inventory, a vault, and the saved blobs the
// export reads, using the same primitives the node uses.
func buildSyntheticBundle(t *testing.T, dir, passphrase string) (inventoryPath, vaultPath, blobDir string, publicPlain, privatePlain []byte, testCID string) {
	t.Helper()
	authorKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate author key: %v", err)
	}
	keys, err := DeriveEpochKeys(authorKey, 204)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	publicPlain = []byte("public part of the exported flip")
	privatePlain = []byte("private part of the exported flip")
	publicCipher, err := keys.EncryptPublicPart(publicPlain)
	if err != nil {
		t.Fatalf("encrypt public part: %v", err)
	}
	privateCipher, err := keys.EncryptPrivatePart(privatePlain)
	if err != nil {
		t.Fatalf("encrypt private part: %v", err)
	}
	blob, err := BuildFlipBlob(crypto.FromECDSAPub(&authorKey.PublicKey), publicCipher, privateCipher)
	if err != nil {
		t.Fatalf("build blob: %v", err)
	}

	testCID = cidRawSha256(blob)
	blobDir = filepath.Join(dir, "blobs")
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		t.Fatalf("create blob dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blobDir, testCID), blob, 0o600); err != nil {
		t.Fatalf("write blob: %v", err)
	}

	inventory := BundleInventory{
		Author:      keys.Address,
		Epoch:       204,
		Submissions: 2,
		UniqueCIDs:  2,
		Deleted:     1,
		BlobsOk:     1,
		Complete:    true,
		Entries: []BundleEntry{
			{CID: testCID, Pair: 0, TxHash: "0xaa", BlockHeight: 100, BlobCIDOk: true},
			{CID: otherCID, Pair: 1, TxHash: "0xbb", BlockHeight: 101, Deleted: true},
		},
	}
	encoded, err := json.Marshal(inventory)
	if err != nil {
		t.Fatalf("encode inventory: %v", err)
	}
	inventoryPath = filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(inventoryPath, encoded, 0o600); err != nil {
		t.Fatalf("write inventory: %v", err)
	}

	payload, err := json.Marshal(keys.ToVaultPayload())
	if err != nil {
		t.Fatalf("encode vault payload: %v", err)
	}
	vaultPath = filepath.Join(dir, "bundle.age")
	if err := WriteVault(vaultPath, passphrase, payload); err != nil {
		t.Fatalf("write vault: %v", err)
	}
	return inventoryPath, vaultPath, blobDir, publicPlain, privatePlain, testCID
}

func TestExportSessionContentOmitsKeysAndDecodesFlips(t *testing.T) {
	dir := t.TempDir()
	passphrase := "export-passphrase"
	inventoryPath, vaultPath, blobDir, publicPlain, privatePlain, testCID := buildSyntheticBundle(t, dir, passphrase)
	outDir := filepath.Join(dir, "export")

	exported, err := ExportSessionContent(ExportOptions{
		InventoryPath: inventoryPath,
		VaultPath:     vaultPath,
		Passphrase:    passphrase,
		BlobDir:       blobDir,
		OutDir:        outDir,
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if exported.Format != exportFormat || exported.Version != exportVersion {
		t.Fatalf("unexpected export identity: %s v%d", exported.Format, exported.Version)
	}
	if exported.Bundle.Epoch != 204 || exported.Bundle.Flips != 1 {
		t.Fatalf("export should carry the one live flip: %+v", exported.Bundle)
	}
	if exported.Bundle.ManifestCommitment != inventoryCommitment(exported.Bundle.Author, 204, []string{testCID}) {
		t.Fatalf("manifest commitment does not match its own inputs")
	}

	// The decoded parts are on disk and byte-identical to what was encrypted.
	publicFile, err := os.ReadFile(filepath.Join(outDir, exported.Flips[0].PublicPart.File))
	if err != nil {
		t.Fatalf("read public part: %v", err)
	}
	privateFile, err := os.ReadFile(filepath.Join(outDir, exported.Flips[0].PrivatePart.File))
	if err != nil {
		t.Fatalf("read private part: %v", err)
	}
	if string(publicFile) != string(publicPlain) || string(privateFile) != string(privatePlain) {
		t.Fatalf("exported parts do not match the originals")
	}

	// Nothing in the export may carry epoch key material.
	manifest, err := os.ReadFile(filepath.Join(outDir, "content-export.json"))
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	payload, err := ReadVault(vaultPath, passphrase)
	if err != nil {
		t.Fatalf("reopen vault: %v", err)
	}
	var stored VaultPayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("parse vault payload: %v", err)
	}
	for name, secret := range map[string]string{
		"public-part private key":  stored.PublicPartSec,
		"private-part private key": stored.PrivatePartSec,
	} {
		if secret == "" {
			t.Fatalf("vault payload has no %s to check against", name)
		}
		if strings.Contains(string(manifest), secret) {
			t.Fatalf("the export leaked the %s", name)
		}
	}
	if !strings.Contains(exported.Keys, "excluded") {
		t.Fatalf("the export should say keys are excluded: %s", exported.Keys)
	}

	// Without an answers file the export says so rather than inventing answers.
	if len(exported.Answers) != 0 || !strings.Contains(exported.AnswersBy, "none") {
		t.Fatalf("answers should be empty and labelled: %+v / %s", exported.Answers, exported.AnswersBy)
	}
}

// TestExportTightensAPreExistingOutput pins `AH-20`: the export writes the
// flip's private part and the metadata through os.WriteFile, which keeps the
// mode of a file that already exists, so a rewrite has to re-apply 0600.
func TestExportTightensAPreExistingOutput(t *testing.T) {
	dir := t.TempDir()
	passphrase := "export-passphrase"
	inventoryPath, vaultPath, blobDir, _, _, testCID := buildSyntheticBundle(t, dir, passphrase)
	outDir := filepath.Join(dir, "export")
	paths := []string{
		filepath.Join(outDir, "content-export.json"),
		filepath.Join(outDir, "flips", testCID+".public"),
		filepath.Join(outDir, "flips", testCID+".private"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
			t.Fatalf("pre-create %s: %v", path, err)
		}
	}
	if _, err := ExportSessionContent(ExportOptions{
		InventoryPath: inventoryPath,
		VaultPath:     vaultPath,
		Passphrase:    passphrase,
		BlobDir:       blobDir,
		OutDir:        outDir,
	}); err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s is mode %o after export, want 0600", path, info.Mode().Perm())
		}
	}
}

func TestExportAnswersRequireEvidence(t *testing.T) {
	dir := t.TempDir()
	passphrase := "export-passphrase"
	inventoryPath, vaultPath, blobDir, _, _, testCID := buildSyntheticBundle(t, dir, passphrase)

	answerFile := filepath.Join(dir, "answers.json")
	answers := []map[string]string{{
		"cid":          testCID,
		"answerDigest": strings.Repeat("ab", 32),
		"evidence":     "long-answer tx 0xcc in block 1234 of epoch 204",
	}}
	encoded, err := json.Marshal(answers)
	if err != nil {
		t.Fatalf("encode answers: %v", err)
	}
	if err := os.WriteFile(answerFile, encoded, 0o600); err != nil {
		t.Fatalf("write answers: %v", err)
	}

	exported, err := ExportSessionContent(ExportOptions{
		InventoryPath: inventoryPath,
		VaultPath:     vaultPath,
		Passphrase:    passphrase,
		BlobDir:       blobDir,
		OutDir:        filepath.Join(dir, "with-answers"),
		AnswersPath:   answerFile,
		AnswersSource: "epoch 204 long-answer sweep",
	})
	if err != nil {
		t.Fatalf("export with answers: %v", err)
	}
	if len(exported.Answers) != 1 || exported.Answers[0].Digest != strings.Repeat("ab", 32) {
		t.Fatalf("answers were not carried: %+v", exported.Answers)
	}
	if exported.AnswersBy != "epoch 204 long-answer sweep" {
		t.Fatalf("answers evidence missing: %s", exported.AnswersBy)
	}

	// Answers without a named source, or without evidence, are refused.
	if _, err := ExportSessionContent(ExportOptions{
		InventoryPath: inventoryPath,
		VaultPath:     vaultPath,
		Passphrase:    passphrase,
		BlobDir:       blobDir,
		OutDir:        filepath.Join(dir, "no-source"),
		AnswersPath:   answerFile,
	}); err == nil {
		t.Fatalf("answers without a source were accepted")
	}

	badAnswers := filepath.Join(dir, "bad-answers.json")
	if err := os.WriteFile(badAnswers, []byte(`[{"cid":"`+testCID+`","answerDigest":"nope","evidence":"x"}]`), 0o600); err != nil {
		t.Fatalf("write bad answers: %v", err)
	}
	if _, err := ExportSessionContent(ExportOptions{
		InventoryPath: inventoryPath,
		VaultPath:     vaultPath,
		Passphrase:    passphrase,
		BlobDir:       blobDir,
		OutDir:        filepath.Join(dir, "bad"),
		AnswersPath:   badAnswers,
		AnswersSource: "sweep",
	}); err == nil {
		t.Fatalf("a malformed answer digest was accepted")
	}
}

func TestExportRefusesAMismatchedVault(t *testing.T) {
	dir := t.TempDir()
	passphrase := "export-passphrase"
	inventoryPath, vaultPath, blobDir, _, _, _ := buildSyntheticBundle(t, dir, passphrase)

	if _, err := ExportSessionContent(ExportOptions{
		InventoryPath: inventoryPath,
		VaultPath:     vaultPath,
		Passphrase:    "wrong-passphrase",
		BlobDir:       blobDir,
		OutDir:        filepath.Join(dir, "wrong-pass"),
	}); err == nil {
		t.Fatalf("a wrong vault passphrase was accepted")
	}

	if _, err := ExportSessionContent(ExportOptions{
		InventoryPath: inventoryPath,
		VaultPath:     vaultPath,
		Passphrase:    passphrase,
		BlobDir:       filepath.Join(dir, "missing-blobs"),
		OutDir:        filepath.Join(dir, "missing"),
	}); err == nil {
		t.Fatalf("a missing blob was accepted")
	}
}

func TestExportRejectsUnauthenticatedBlobsAndAuthors(t *testing.T) {
	for _, attack := range []string{"substituted-bytes", "wrong-vault-author", "wrong-blob-author"} {
		t.Run(attack, func(t *testing.T) {
			dir := t.TempDir()
			inventoryPath, vaultPath, blobs, _, _, flipCID := buildSyntheticBundle(t, dir, "export-safe")
			raw, err := os.ReadFile(inventoryPath)
			if err != nil {
				t.Fatal(err)
			}
			var inventory BundleInventory
			if err := json.Unmarshal(raw, &inventory); err != nil {
				t.Fatal(err)
			}
			blob, err := os.ReadFile(filepath.Join(blobs, flipCID))
			if err != nil {
				t.Fatal(err)
			}
			switch attack {
			case "substituted-bytes":
				// Valid protobuf and decryptable ciphertext, but different bytes.
				key, err := crypto.GenerateKey()
				if err != nil {
					t.Fatal(err)
				}
				_, public, private, err := ParseFlipBlob(blob)
				if err != nil {
					t.Fatal(err)
				}
				blob, err = BuildFlipBlob(crypto.FromECDSAPub(&key.PublicKey), public, private)
				if err != nil {
					t.Fatal(err)
				}
			case "wrong-vault-author":
				inventory.Author = "0x" + strings.Repeat("11", 20)
			case "wrong-blob-author":
				key, err := crypto.GenerateKey()
				if err != nil {
					t.Fatal(err)
				}
				_, public, private, err := ParseFlipBlob(blob)
				if err != nil {
					t.Fatal(err)
				}
				blob, err = BuildFlipBlob(crypto.FromECDSAPub(&key.PublicKey), public, private)
				if err != nil {
					t.Fatal(err)
				}
				flipCID = cidRawSha256(blob)
				inventory.Entries[0].CID = flipCID
			}
			if err := os.WriteFile(filepath.Join(blobs, flipCID), blob, 0o600); err != nil {
				t.Fatal(err)
			}
			raw, err = json.Marshal(inventory)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(inventoryPath, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "export")
			if _, err := ExportSessionContent(ExportOptions{InventoryPath: inventoryPath, VaultPath: vaultPath, Passphrase: "export-safe", BlobDir: blobs, OutDir: out}); err == nil {
				t.Fatal("export accepted unauthenticated provenance", attack)
			}
			if _, err := os.Stat(filepath.Join(out, "content-export.json")); !os.IsNotExist(err) {
				t.Fatal("refused export must not publish a manifest", err)
			}
		})
	}
}

func TestExportRejectsTraversalAndEscapingSymlinks(t *testing.T) {
	for _, attack := range []string{"cid-traversal", "input-symlink", "output-symlink"} {
		t.Run(attack, func(t *testing.T) {
			dir := t.TempDir()
			inventory, vault, blobs, _, _, testCID := buildSyntheticBundle(t, dir, "export-safe")
			out := filepath.Join(dir, "export")
			outside := t.TempDir()
			sentinel := filepath.Join(outside, "keep")
			os.WriteFile(sentinel, []byte("unchanged"), 0o600)
			switch attack {
			case "cid-traversal":
				var inv BundleInventory
				raw, _ := os.ReadFile(inventory)
				json.Unmarshal(raw, &inv)
				inv.Entries[0].CID = "../../keep"
				raw, _ = json.Marshal(inv)
				os.WriteFile(inventory, raw, 0o600)
			case "input-symlink":
				os.Remove(filepath.Join(blobs, testCID))
				os.Symlink(sentinel, filepath.Join(blobs, testCID))
			case "output-symlink":
				os.MkdirAll(out, 0o700)
				os.Symlink(outside, filepath.Join(out, "flips"))
			}
			if _, err := ExportSessionContent(ExportOptions{InventoryPath: inventory, VaultPath: vault, Passphrase: "export-safe", BlobDir: blobs, OutDir: out}); err == nil {
				t.Fatal("export accepted", attack)
			}
			got, _ := os.ReadFile(sentinel)
			if string(got) != "unchanged" {
				t.Fatal("export changed outside content")
			}
			entries, _ := os.ReadDir(outside)
			if len(entries) != 1 {
				t.Fatal("export wrote outside its directory")
			}
		})
	}
}

func TestSessionPoolRejectsEscapingPartPaths(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	data := []byte("private material outside the export")
	os.WriteFile(outside, data, 0o600)
	for _, name := range []string{outside, "../secret", "flips/../../secret"} {
		if _, err := readExportedPart(dir, testCID, "private", describePart(name, data)); err == nil {
			t.Fatal("accepted outside path", name)
		}
	}
	os.Symlink(outside, filepath.Join(dir, "link"))
	if _, err := readExportedPart(dir, testCID, "private", describePart("link", data)); err == nil {
		t.Fatal("read outside symlink")
	}
	os.WriteFile(filepath.Join(dir, "valid"), data, 0o600)
	if got, err := readExportedPart(dir, testCID, "private", describePart("valid", data)); err != nil || got != string(data) {
		t.Fatal("normal contained export failed", err)
	}
}
