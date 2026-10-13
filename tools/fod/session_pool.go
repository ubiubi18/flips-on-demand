package main

// The provider's session pool, the file a gateway scores a session against.
//
// A provider already has everything it needs in `fod.session-content`: the
// decrypted public and private parts of each flip, and the consensus answers it
// can evidence for. This turns that export into the `fod.flip-pool` file the
// reference gateway loads (`SPEC.md` 6.7, 8.2), so a provider never has to
// hand-write content.
//
// Two rules are kept here rather than in the gateway: every challenge needs the
// answer the provider vouches for (answers are never inferred), and each part
// file has to be the bytes the export recorded (its SHA-256), so an edited
// export is refused instead of published. Whether the content is a flip the
// client can show is the gateway's own load check (`SOURCES.md` F-98).

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	sessionPoolFormat  = "fod.flip-pool"
	sessionPoolVersion = 1
)

// SessionPoolOptions is what the CLI passes in.
type SessionPoolOptions struct {
	ExportDir string
	OutPath   string
	SessionID string
	UtilityID string
	Threshold int
	OpensAt   string
	ClosesAt  string
	// AnswerSalt is the per-session secret the content digest is salted with
	// (`gateway/src/pool.ts`, AH-2). Empty means a fresh random salt; a fixed
	// value keeps deterministic fixtures byte-identical.
	AnswerSalt string
	Now        time.Time
}

type sessionPoolProvenance struct {
	Epoch      uint16 `json:"epoch"`
	Author     string `json:"author"`
	Source     string `json:"source"`
	BlockRange string `json:"blockRange"`
	// Answers is what the provider's answers rest on, carried over from the
	// export. It is the provider's own statement, which is what the session says
	// when it shows it (`REVIEW.md` A09: the session should connect the content
	// and the answers' provenance).
	Answers string `json:"answers,omitempty"`
}

type sessionPoolRules struct {
	PassThreshold int    `json:"passThreshold"`
	OpensAt       string `json:"opensAt"`
	ClosesAt      string `json:"closesAt"`
}

type sessionPoolContent struct {
	PublicPart  string `json:"publicPart"`
	PrivatePart string `json:"privatePart"`
}

type sessionPoolChallenge struct {
	ChallengeID  string              `json:"challengeId"`
	CID          string              `json:"cid"`
	AnswerDigest string              `json:"answerDigest"`
	Content      *sessionPoolContent `json:"content,omitempty"`
}

type sessionPoolFile struct {
	Format     string                 `json:"format"`
	Version    int                    `json:"version"`
	SessionID  string                 `json:"sessionId"`
	UtilityID  string                 `json:"utilityId"`
	Provenance sessionPoolProvenance  `json:"provenance"`
	Rules      sessionPoolRules       `json:"rules"`
	AnswerSalt string                 `json:"answerSalt"`
	Challenges []sessionPoolChallenge `json:"challenges"`
}

// BuildSessionPool reads an export and writes the pool the gateway loads.
func BuildSessionPool(options SessionPoolOptions) (*sessionPoolFile, error) {
	if options.ExportDir == "" || options.OutPath == "" {
		return nil, errors.New("the export directory and the output path are required")
	}
	if options.SessionID == "" || options.UtilityID == "" {
		return nil, errors.New("the pool needs a session id and a utility id")
	}
	raw, err := os.ReadFile(filepath.Join(options.ExportDir, "content-export.json"))
	if err != nil {
		return nil, err
	}
	var export SessionContentExport
	if err := json.Unmarshal(raw, &export); err != nil {
		return nil, fmt.Errorf("parse the export: %w", err)
	}
	if export.Format != exportFormat || export.Version != exportVersion {
		return nil, fmt.Errorf("the export is %s v%d, this build reads %s v%d",
			export.Format, export.Version, exportFormat, exportVersion)
	}
	if len(export.Flips) == 0 {
		return nil, errors.New("the export carries no flips")
	}

	answers := make(map[string]string, len(export.Answers))
	for _, answer := range export.Answers {
		answers[strings.ToLower(answer.CID)] = strings.ToLower(strings.TrimPrefix(answer.Digest, "0x"))
	}

	pool := &sessionPoolFile{
		Format:     sessionPoolFormat,
		Version:    sessionPoolVersion,
		SessionID:  options.SessionID,
		UtilityID:  options.UtilityID,
		AnswerSalt: strings.ToLower(strings.TrimPrefix(options.AnswerSalt, "0x")),
		Provenance: sessionPoolProvenance{
			Epoch:   export.Bundle.Epoch,
			Author:  strings.ToLower(strings.TrimPrefix(export.Bundle.Author, "0x")),
			Source:  export.Bundle.Source,
			Answers: export.AnswersBy,
		},
		Rules: sessionPoolRules{
			PassThreshold: options.Threshold,
			OpensAt:       options.OpensAt,
			ClosesAt:      options.ClosesAt,
		},
		Challenges: make([]sessionPoolChallenge, 0, len(export.Flips)),
	}
	if pool.Provenance.Author == "" {
		return nil, errors.New("the export names no author")
	}
	if pool.AnswerSalt == "" {
		salt := make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return nil, fmt.Errorf("draw the session's answer salt: %w", err)
		}
		pool.AnswerSalt = hex.EncodeToString(salt)
	}
	if !isHex(pool.AnswerSalt, 32) {
		return nil, errors.New("the answer salt must be a 32-byte hex value")
	}
	if pool.Provenance.Source == "" {
		pool.Provenance.Source = fmt.Sprintf("epoch %d of %s", export.Bundle.Epoch, pool.Provenance.Author)
	}
	pool.Provenance.BlockRange = exportBlockRange(export.Flips)

	for _, flip := range export.Flips {
		digest, ok := answers[strings.ToLower(flip.CID)]
		if !ok || digest == "" {
			return nil, fmt.Errorf(
				"flip %s has no answered consensus record in this export: a pool cannot carry a challenge the provider cannot score, and answers are never inferred",
				flip.CID)
		}
		if !isHex(digest, 32) {
			return nil, fmt.Errorf("flip %s has an answer digest that is not a SHA-256 value", flip.CID)
		}
		public, err := readExportedPart(options.ExportDir, flip.CID, "public part", flip.PublicPart)
		if err != nil {
			return nil, err
		}
		private, err := readExportedPart(options.ExportDir, flip.CID, "private part", flip.PrivatePart)
		if err != nil {
			return nil, err
		}
		pool.Challenges = append(pool.Challenges, sessionPoolChallenge{
			ChallengeID:  flip.CID,
			CID:          flip.CID,
			AnswerDigest: digest,
			Content:      &sessionPoolContent{PublicPart: public, PrivatePart: private},
		})
	}

	if pool.Rules.PassThreshold < 1 || pool.Rules.PassThreshold > len(pool.Challenges) {
		return nil, fmt.Errorf("the pass threshold %d is not between 1 and the %d challenges this export carries",
			pool.Rules.PassThreshold, len(pool.Challenges))
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	if pool.Rules.OpensAt == "" {
		pool.Rules.OpensAt = now.UTC().Format(time.RFC3339)
	}
	for _, boundary := range []struct{ name, value string }{
		{"opensAt", pool.Rules.OpensAt},
		{"closesAt", pool.Rules.ClosesAt},
	} {
		if _, err := time.Parse(time.RFC3339, boundary.value); err != nil {
			return nil, fmt.Errorf("the pool's %s is not an RFC 3339 timestamp: %q", boundary.name, boundary.value)
		}
	}
	opens, _ := time.Parse(time.RFC3339, pool.Rules.OpensAt)
	closes, _ := time.Parse(time.RFC3339, pool.Rules.ClosesAt)
	if !closes.After(opens) {
		return nil, fmt.Errorf("the pool closes at %s, which is not after it opens at %s", pool.Rules.ClosesAt, pool.Rules.OpensAt)
	}

	encoded, err := json.MarshalIndent(pool, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writePrivateBytes(options.OutPath, append(encoded, '\n')); err != nil {
		return nil, fmt.Errorf("write %s: %w", options.OutPath, err)
	}
	return pool, nil
}

// readExportedPart returns one part file's text, refusing anything that is not
// the bytes the export recorded.
func readExportedPart(dir, cid, name string, part ExportedPart) (string, error) {
	if part.File == "" {
		return "", fmt.Errorf("flip %s: the export names no %s file", cid, name)
	}
	if !fs.ValidPath(part.File) || strings.Contains(part.File, "\\") {
		return "", fmt.Errorf("flip %s: the part path must stay inside the export directory", cid)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	raw, err := root.ReadFile(filepath.FromSlash(part.File))
	if err != nil {
		return "", fmt.Errorf("flip %s: %w", cid, err)
	}
	if part.Bytes != 0 && part.Bytes != len(raw) {
		return "", fmt.Errorf("flip %s: the %s is %d bytes, the export recorded %d", cid, name, len(raw), part.Bytes)
	}
	digest := sha256.Sum256(raw)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), part.SHA256) {
		return "", fmt.Errorf("flip %s: the %s does not hash to the digest the export recorded", cid, name)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", fmt.Errorf("flip %s: the %s is empty", cid, name)
	}
	return text, nil
}

// exportBlockRange is the span the flips were submitted in, which is what the
// pool's provenance names; an export without heights says so rather than
// inventing one.
func exportBlockRange(flips []ExportedFlip) string {
	low, high := uint64(0), uint64(0)
	for _, flip := range flips {
		if flip.BlockHeight == 0 {
			continue
		}
		if low == 0 || flip.BlockHeight < low {
			low = flip.BlockHeight
		}
		if flip.BlockHeight > high {
			high = flip.BlockHeight
		}
	}
	if low == 0 {
		return "unspecified"
	}
	return fmt.Sprintf("%d-%d", low, high)
}
