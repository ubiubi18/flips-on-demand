package main

// The pool a provider publishes: every challenge carries the flip's own parts
// and the answer the provider vouches for, an edited export is refused, and a
// flip without an answer never becomes a challenge (`SPEC.md` 6.7, 8.2).

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeExportFixture(t *testing.T, flips int, answers int) string {
	t.Helper()
	dir := t.TempDir()
	export := SessionContentExport{
		Format:  exportFormat,
		Version: exportVersion,
		Answers: []ExportedAnswer{},
	}
	export.Bundle.Author = "0x" + strings.Repeat("ab", addressBytes)
	export.Bundle.Epoch = 228
	export.Bundle.Source = "the lab's sweep"
	export.AnswersBy = "the epoch's own answers file"
	for index := 0; index < flips; index++ {
		cid := "bafkreitestflip" + string(rune('a'+index))
		public := "0x0a01" // the shape does not matter here; the gateway checks it
		private := "0x0b02"
		publicFile := filepath.Join("flips", cid+".public")
		privateFile := filepath.Join("flips", cid+".private")
		if err := os.MkdirAll(filepath.Join(dir, "flips"), 0o700); err != nil {
			t.Fatalf("create the flip directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, publicFile), []byte(public), 0o600); err != nil {
			t.Fatalf("write the public part: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, privateFile), []byte(private), 0o600); err != nil {
			t.Fatalf("write the private part: %v", err)
		}
		export.Flips = append(export.Flips, ExportedFlip{
			CID:         cid,
			Pair:        1,
			BlockHeight: uint64(10_870_000 + index),
			PublicPart:  describePart(publicFile, []byte(public)),
			PrivatePart: describePart(privateFile, []byte(private)),
		})
		if index < answers {
			digest := sha256.Sum256([]byte("right"))
			export.Answers = append(export.Answers, ExportedAnswer{
				CID: cid, Digest: hex.EncodeToString(digest[:]), Evidence: "the epoch's own answers file",
			})
		}
	}
	raw, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		t.Fatalf("render the export: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "content-export.json"), raw, 0o600); err != nil {
		t.Fatalf("write the export: %v", err)
	}
	return dir
}

func poolOptions(dir, out string) SessionPoolOptions {
	return SessionPoolOptions{
		ExportDir: dir,
		OutPath:   out,
		SessionID: "session-1",
		UtilityID: "utility-1",
		Threshold: 1,
		ClosesAt:  "2026-12-31T23:59:59Z",
		Now:       time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}
}

func TestSessionPoolCarriesContentAndAnswers(t *testing.T) {
	dir := writeExportFixture(t, 2, 2)
	out := filepath.Join(t.TempDir(), "pool.json")
	// An earlier run may have left the pool wider; the write has to tighten it.
	if err := os.WriteFile(out, []byte("{}"), 0o644); err != nil {
		t.Fatalf("pre-create the pool: %v", err)
	}
	pool, err := BuildSessionPool(poolOptions(dir, out))
	if err != nil {
		t.Fatalf("build the pool: %v", err)
	}
	if pool.Format != sessionPoolFormat || pool.Version != sessionPoolVersion {
		t.Fatalf("the pool is %s v%d", pool.Format, pool.Version)
	}
	if pool.Provenance.Epoch != 228 || !strings.EqualFold(pool.Provenance.Author, strings.Repeat("ab", addressBytes)) {
		t.Fatalf("the provenance reads %+v", pool.Provenance)
	}
	if pool.Provenance.BlockRange != "10870000-10870001" {
		t.Fatalf("the block range reads %q", pool.Provenance.BlockRange)
	}
	// What the answers rest on travels with the pool, so a session can say it
	// (`REVIEW.md` A09).
	if pool.Provenance.Answers != "the epoch's own answers file" {
		t.Fatalf("the answers' provenance reads %q", pool.Provenance.Answers)
	}
	if pool.Rules.OpensAt != "2026-09-27T12:00:00Z" || pool.Rules.PassThreshold != 1 {
		t.Fatalf("the rules read %+v", pool.Rules)
	}
	if len(pool.Challenges) != 2 {
		t.Fatalf("the pool carries %d challenges", len(pool.Challenges))
	}
	first := pool.Challenges[0]
	if first.ChallengeID != first.CID || first.Content == nil {
		t.Fatalf("the challenge reads %+v", first)
	}
	if first.Content.PublicPart != "0x0a01" || first.Content.PrivatePart != "0x0b02" {
		t.Fatalf("the content reads %+v", first.Content)
	}
	// 0600: the pool carries the answer digests and the flips the provider sells.
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat the pool: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the pool is mode %o", info.Mode().Perm())
	}
}

// The committed pool is the cross-language fixture: this command produced
// `gateway/fixtures/cli-pool.json`, and the gateway's own test loads that file,
// so a change to either side fails here or there.
func TestSessionPoolMatchesTheCommittedFixture(t *testing.T) {
	out := filepath.Join(t.TempDir(), "pool.json")
	committed, err := os.ReadFile("../../gateway/fixtures/cli-pool.json")
	if err != nil {
		t.Fatalf("read the committed pool: %v", err)
	}
	// The salt is the one secret a committed fixture cannot leave random, so
	// the fixture's own value is fed back in and the bytes are compared.
	var pinned sessionPoolFile
	if err := json.Unmarshal(committed, &pinned); err != nil {
		t.Fatalf("parse the committed pool: %v", err)
	}
	if !isHex(pinned.AnswerSalt, 32) {
		t.Fatalf("the committed pool carries no 32-byte answer salt: %q", pinned.AnswerSalt)
	}
	_, err = BuildSessionPool(SessionPoolOptions{
		ExportDir:  "../../gateway/fixtures/session-export",
		OutPath:    out,
		SessionID:  "session-cli-1",
		UtilityID:  "utility-cli",
		Threshold:  1,
		OpensAt:    "2026-09-27T12:00:00Z",
		ClosesAt:   "2026-12-31T23:59:59Z",
		AnswerSalt: pinned.AnswerSalt,
	})
	if err != nil {
		t.Fatalf("build the pool: %v", err)
	}
	produced, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read the produced pool: %v", err)
	}
	if !bytes.Equal(produced, committed) {
		t.Fatalf("the committed pool differs from what this command writes:\n got %s\nwant %s", produced, committed)
	}
}

// The salt is what keeps a published digest from being enumerable offline
// before the session closes (`AH-2`), so every pool without a pinned salt
// draws a fresh 32-byte secret.
func TestSessionPoolDrawsAFreshAnswerSalt(t *testing.T) {
	dir := writeExportFixture(t, 1, 1)
	first, err := BuildSessionPool(poolOptions(dir, filepath.Join(t.TempDir(), "first.json")))
	if err != nil {
		t.Fatalf("build the first pool: %v", err)
	}
	second, err := BuildSessionPool(poolOptions(dir, filepath.Join(t.TempDir(), "second.json")))
	if err != nil {
		t.Fatalf("build the second pool: %v", err)
	}
	if !isHex(first.AnswerSalt, 32) {
		t.Fatalf("the first pool drew no 32-byte salt: %q", first.AnswerSalt)
	}
	if first.AnswerSalt == second.AnswerSalt {
		t.Fatal("two pools drew the same answer salt")
	}
}

func TestSessionPoolRefusesWhatItCannotScore(t *testing.T) {
	dir := writeExportFixture(t, 2, 1)
	if _, err := BuildSessionPool(poolOptions(dir, filepath.Join(t.TempDir(), "pool.json"))); err == nil {
		t.Fatal("a flip without an answer became a challenge")
	}

	// A part file that no longer matches the digest the export recorded.
	dir = writeExportFixture(t, 1, 1)
	exported, err := os.ReadFile(filepath.Join(dir, "content-export.json"))
	if err != nil {
		t.Fatalf("read the export: %v", err)
	}
	var export SessionContentExport
	if err := json.Unmarshal(exported, &export); err != nil {
		t.Fatalf("parse the export: %v", err)
	}
	part := filepath.Join(dir, filepath.FromSlash(export.Flips[0].PublicPart.File))
	if err := os.WriteFile(part, []byte("0x0a99"), 0o600); err != nil {
		t.Fatalf("edit the part: %v", err)
	}
	if _, err := BuildSessionPool(poolOptions(dir, filepath.Join(t.TempDir(), "pool.json"))); err == nil {
		t.Fatal("an edited export produced a pool")
	}

	// A threshold above the challenge count is a session nobody can pass.
	dir = writeExportFixture(t, 1, 1)
	options := poolOptions(dir, filepath.Join(t.TempDir(), "pool.json"))
	options.Threshold = 2
	if _, err := BuildSessionPool(options); err == nil {
		t.Fatal("a threshold above the challenge count was accepted")
	}

	// A close before the open is a session that never runs.
	options = poolOptions(dir, filepath.Join(t.TempDir(), "pool.json"))
	options.OpensAt = "2027-01-01T00:00:00Z"
	if _, err := BuildSessionPool(options); err == nil {
		t.Fatal("a session closing before it opens was accepted")
	}
}
