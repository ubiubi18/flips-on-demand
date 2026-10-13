package main

// The hosted shared node's onboarding: one author's vault becomes the export,
// the pool a gateway loads, and the connection file the author imports, with
// the custody boundary stated rather than implied (`SPEC.md` 10.4 G3).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeAnswerFixture records the consensus answer for the synthetic bundle's
// live flip: a pool refuses a challenge the provider cannot score, so the node
// needs the epoch's own answer record.
func writeAnswerFixture(t *testing.T, dir, testCID string) string {
	t.Helper()
	digest := sha256.Sum256([]byte("right"))
	encoded, err := json.Marshal([]ExportedAnswer{
		{CID: testCID, Digest: hex.EncodeToString(digest[:]), Evidence: "the test's consensus record"},
	})
	if err != nil {
		t.Fatalf("encode the answers: %v", err)
	}
	path := filepath.Join(dir, "answers.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write the answers: %v", err)
	}
	return path
}

func TestSharedNodeBuildsTheExportPoolAndConnection(t *testing.T) {
	dir := t.TempDir()
	passphrase := "shared-node-pass"
	inventoryPath, vaultPath, blobDir, publicPlain, _, testCID := buildSyntheticBundle(t, dir, passphrase)
	answersPath := writeAnswerFixture(t, dir, testCID)
	out := filepath.Join(dir, "node")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatalf("create the node directory: %v", err)
	}
	closesAt := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)

	result, err := BuildSharedNode(SharedNodeOptions{
		VaultPath:     vaultPath,
		InventoryPath: inventoryPath,
		BlobDir:       blobDir,
		OutDir:        out,
		AnswersPath:   answersPath,
		AnswersSource: "the test's consensus record",
		SessionID:     "session-shared-1",
		UtilityID:     "utility-shared",
		Threshold:     1,
		ClosesAt:      closesAt,
		NodeID:        "node-1",
		NodeURL:       "http://127.0.0.1:8787",
		Custody:       "hosted",
		Passphrase:    passphrase,
		AnswerSalt:    strings.Repeat("ab", 32),
	})
	if err != nil {
		t.Fatalf("build the shared node: %v", err)
	}

	// The node performed the export itself: the decoded flip is on disk, and it
	// is the flip the author's vault decrypts.
	if result.Export.Bundle.Flips != 1 {
		t.Fatalf("the export should carry the one live flip: %+v", result.Export.Bundle)
	}
	exported, err := os.ReadFile(filepath.Join(out, "export", "content-export.json"))
	if err != nil {
		t.Fatalf("read the export: %v", err)
	}
	if !strings.Contains(string(exported), "excluded: this export carries decoded content") {
		t.Fatalf("the export must state that it carries no epoch key pairs")
	}
	var decoded SessionContentExport
	if err := json.Unmarshal(exported, &decoded); err != nil {
		t.Fatalf("parse the export: %v", err)
	}
	if decoded.Bundle.Author != result.Export.Bundle.Author {
		t.Fatalf("the export and the result disagree about the author")
	}
	flipPublic, err := os.ReadFile(filepath.Join(out, "export", decoded.Flips[0].PublicPart.File))
	if err != nil {
		t.Fatalf("read the exported public part: %v", err)
	}
	if string(flipPublic) != string(publicPlain) {
		t.Fatalf("the exported public part is not the flip the vault decrypts")
	}

	// The pool is what the gateway loads, and it stays private because it holds
	// the answer digests (`SPEC.md` 8.2).
	poolInfo, err := os.Stat(result.PoolPath)
	if err != nil {
		t.Fatalf("stat the pool: %v", err)
	}
	if poolInfo.Mode().Perm() != 0o600 {
		t.Fatalf("the pool must stay 0600, got %v", poolInfo.Mode().Perm())
	}
	rawPool, err := os.ReadFile(result.PoolPath)
	if err != nil {
		t.Fatalf("read the pool: %v", err)
	}
	var pool sessionPoolFile
	if err := json.Unmarshal(rawPool, &pool); err != nil {
		t.Fatalf("parse the pool: %v", err)
	}
	if pool.SessionID != "session-shared-1" || pool.UtilityID != "utility-shared" {
		t.Fatalf("the pool names the wrong session or utility: %+v", pool)
	}
	if pool.Rules.PassThreshold != 1 || len(pool.Challenges) != 1 {
		t.Fatalf("the pool should carry one challenge with threshold 1: %+v", pool.Rules)
	}
	if pool.Challenges[0].AnswerDigest != result.Pool.Challenges[0].AnswerDigest {
		t.Fatalf("the pool on disk is not the pool the node returned")
	}

	// The connection file is what the author imports: it names the node, the
	// author the escrow pays, the custody boundary and the pool the gateway
	// loads, and it is public information.
	connectionInfo, err := os.Stat(result.ConnectionPath)
	if err != nil {
		t.Fatalf("stat the connection file: %v", err)
	}
	if connectionInfo.Mode().Perm() != 0o644 {
		t.Fatalf("the connection file carries no secrets and may be 0644, got %v", connectionInfo.Mode().Perm())
	}
	rawConnection, err := os.ReadFile(result.ConnectionPath)
	if err != nil {
		t.Fatalf("read the connection file: %v", err)
	}
	var connection SharedNodeConnection
	if err := json.Unmarshal(rawConnection, &connection); err != nil {
		t.Fatalf("parse the connection file: %v", err)
	}
	if connection.Format != sharedNodeFormat || connection.Version != sharedNodeVersion {
		t.Fatalf("the connection file is not this build's format")
	}
	if connection.NodeID != "node-1" || connection.URL != "http://127.0.0.1:8787" {
		t.Fatalf("the connection file names the wrong node: %+v", connection)
	}
	author := strings.ToLower(strings.TrimPrefix(result.Export.Bundle.Author, "0x"))
	if connection.Author != "0x"+author || !isHex(author, addressBytes) {
		t.Fatalf("the connection file must name the author the escrow pays, got %q", connection.Author)
	}
	if connection.Custody != "hosted" || connection.Pool != "pool.json" {
		t.Fatalf("the connection file must state the custody boundary and the pool: %+v", connection)
	}
	if !strings.Contains(connection.Note, "custody hosted") {
		t.Fatalf("the note has to state the custody boundary: %q", connection.Note)
	}
}

func TestSharedNodeStatesSelfCustodyAndRefusesBadInput(t *testing.T) {
	dir := t.TempDir()
	passphrase := "self-custody-pass"
	inventoryPath, vaultPath, blobDir, _, _, testCID := buildSyntheticBundle(t, dir, passphrase)
	answersPath := writeAnswerFixture(t, dir, testCID)
	out := filepath.Join(dir, "node")

	if _, err := BuildSharedNode(SharedNodeOptions{
		VaultPath: vaultPath, InventoryPath: inventoryPath, BlobDir: blobDir, OutDir: out,
		AnswersPath: answersPath, AnswersSource: "the test's consensus record",
		SessionID: "session-self-1", UtilityID: "utility-self", Threshold: 1,
		ClosesAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		NodeID:   "node-self", NodeURL: "http://127.0.0.1:8787",
		// The author's own machine keeps the vault: the file has to say so, and
		// nothing about the build changes.
		Custody: "self", Passphrase: passphrase,
	}); err != nil {
		t.Fatalf("build with self custody: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "fod-shared-node.json"))
	if err != nil {
		t.Fatalf("read the connection file: %v", err)
	}
	var connection SharedNodeConnection
	if err := json.Unmarshal(raw, &connection); err != nil {
		t.Fatalf("parse the connection file: %v", err)
	}
	if connection.Custody != "self" || !strings.Contains(connection.Note, "keeps the epoch vault") {
		t.Fatalf("self custody must be stated, got %+v", connection)
	}

	// A custody mode nobody defined, a URL the interface cannot read, and a
	// missing vault are refused before anything is built.
	for name, options := range map[string]SharedNodeOptions{
		"unknown custody": {
			VaultPath: vaultPath, InventoryPath: inventoryPath, BlobDir: blobDir, OutDir: filepath.Join(dir, "a"),
			SessionID: "s", UtilityID: "u", Threshold: 1, ClosesAt: "2026-10-01T00:00:00Z",
			NodeID: "n", NodeURL: "http://127.0.0.1:8787", Custody: "hosted-by-nobody", Passphrase: passphrase,
		},
		"unreadable URL": {
			VaultPath: vaultPath, InventoryPath: inventoryPath, BlobDir: blobDir, OutDir: filepath.Join(dir, "b"),
			SessionID: "s", UtilityID: "u", Threshold: 1, ClosesAt: "2026-10-01T00:00:00Z",
			NodeID: "n", NodeURL: "127.0.0.1:8787", Custody: "hosted", Passphrase: passphrase,
		},
		"no vault": {
			InventoryPath: inventoryPath, BlobDir: blobDir, OutDir: filepath.Join(dir, "c"),
			SessionID: "s", UtilityID: "u", Threshold: 1, ClosesAt: "2026-10-01T00:00:00Z",
			NodeID: "n", NodeURL: "http://127.0.0.1:8787", Custody: "hosted", Passphrase: passphrase,
		},
	} {
		if _, err := BuildSharedNode(options); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
}
