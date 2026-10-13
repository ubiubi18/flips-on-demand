package main

// The hosted shared node's onboarding for one author.
//
// `SPEC.md` 10.4 G3: an author connects a self-hosted or provider-hosted shared
// node, which performs the Idena node work, the export/session-pool step and the
// delivery desk, while the custody boundary stays explicit - a hosted node holds
// the author's epoch vault and can deliver, a self-hosted one keeps it with the
// author. This is the node half of that journey: from the author's vault and the
// captured inventory it performs the export and builds the pool a gateway
// loads, then writes the `fod.shared-node` connection the author imports, which
// names the node, the author and which side holds the vault. The listing stays
// the author's own action; the delivered content is paid to the registered
// author by the escrow, whatever the custody mode.

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const sharedNodeFormat = "fod.shared-node"
const sharedNodeVersion = 1

// SharedNodeOptions is what a node operator handles for one author.
type SharedNodeOptions struct {
	VaultPath     string
	InventoryPath string
	BlobDir       string
	OutDir        string
	AnswersPath   string
	AnswersSource string
	SessionID     string
	UtilityID     string
	Threshold     int
	OpensAt       string
	ClosesAt      string
	NodeID        string
	NodeURL       string
	Custody       string
	Passphrase    string
	AnswerSalt    string
	Now           time.Time
}

// SharedNodeConnection is the file an author imports to point at this node.
type SharedNodeConnection struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	NodeID  string `json:"nodeId"`
	URL     string `json:"url"`
	Author  string `json:"author"`
	Custody string `json:"custody"`
	// The pool the node built, relative to the directory the gateway is started
	// from, so the operator can point `poolPath` at it without guessing.
	Pool string `json:"pool"`
	Note string `json:"note"`
}

type SharedNodeResult struct {
	Export         *SessionContentExport
	Pool           *sessionPoolFile
	ExportDir      string
	PoolPath       string
	ConnectionPath string
	Connection     SharedNodeConnection
}

// BuildSharedNode performs the node's part of the hosted journey: export the
// author's flips, build the session pool the gateway serves, and write the
// connection file that states who holds the vault.
func BuildSharedNode(options SharedNodeOptions) (*SharedNodeResult, error) {
	if options.VaultPath == "" || options.InventoryPath == "" || options.BlobDir == "" || options.OutDir == "" {
		return nil, errors.New("the shared node needs -vault FILE -inventory FILE -blob-dir DIR -out DIR")
	}
	if options.SessionID == "" || options.UtilityID == "" || options.Threshold < 1 || options.ClosesAt == "" {
		return nil, errors.New("the shared node needs -session-id ID -utility-id ID -threshold N -closes-at RFC3339")
	}
	if options.Custody != "hosted" && options.Custody != "self" {
		return nil, errors.New("the custody boundary must be hosted or self")
	}
	if options.NodeID == "" || options.NodeURL == "" {
		return nil, errors.New("the shared node needs -node-id NAME -node-url URL")
	}
	url := strings.TrimSpace(options.NodeURL)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, errors.New("the node URL must be http or https")
	}

	exportDir := filepath.Join(options.OutDir, "export")
	exported, err := ExportSessionContent(ExportOptions{
		InventoryPath: options.InventoryPath,
		VaultPath:     options.VaultPath,
		Passphrase:    options.Passphrase,
		BlobDir:       options.BlobDir,
		OutDir:        exportDir,
		AnswersPath:   options.AnswersPath,
		AnswersSource: options.AnswersSource,
	})
	if err != nil {
		return nil, fmt.Errorf("export the author's flips: %w", err)
	}

	poolPath := filepath.Join(options.OutDir, "pool.json")
	pool, err := BuildSessionPool(SessionPoolOptions{
		ExportDir:  exportDir,
		OutPath:    poolPath,
		SessionID:  options.SessionID,
		UtilityID:  options.UtilityID,
		Threshold:  options.Threshold,
		OpensAt:    options.OpensAt,
		ClosesAt:   options.ClosesAt,
		AnswerSalt: options.AnswerSalt,
		Now:        options.Now,
	})
	if err != nil {
		return nil, fmt.Errorf("build the session pool: %w", err)
	}

	author := strings.ToLower(exported.Bundle.Author)
	if !strings.HasPrefix(author, "0x") {
		author = "0x" + author
	}
	if !isHex(strings.TrimPrefix(author, "0x"), addressBytes) {
		return nil, fmt.Errorf("the export names no usable author address: %q", exported.Bundle.Author)
	}

	connection := SharedNodeConnection{
		Format:  sharedNodeFormat,
		Version: sharedNodeVersion,
		NodeID:  options.NodeID,
		URL:     url,
		Author:  author,
		Custody: options.Custody,
		Pool:    filepath.Base(poolPath),
		Note: fmt.Sprintf(
			"export and session pool built by this node for epoch %d. custody %s: %s. The escrow pays "+
				"the registered author address either way, and the customer's escape stays one permissionless refund.",
			exported.Bundle.Epoch,
			options.Custody,
			map[string]string{
				"hosted": "the node holds the epoch vault and can deliver the flips",
				"self":   "the author's own machine keeps the epoch vault",
			}[options.Custody],
		),
	}
	connectionPath := filepath.Join(options.OutDir, "fod-shared-node.json")
	if err := writeJSON(connectionPath, connection); err != nil {
		return nil, fmt.Errorf("write the connection file: %w", err)
	}

	return &SharedNodeResult{
		Export:         exported,
		Pool:           pool,
		ExportDir:      exportDir,
		PoolPath:       poolPath,
		ConnectionPath: connectionPath,
		Connection:     connection,
	}, nil
}
