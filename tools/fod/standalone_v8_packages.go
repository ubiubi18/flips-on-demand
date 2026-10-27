package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

type standalonePackage struct {
	ProtocolVersion int      `json:"protocolVersion,omitempty"`
	ID              string   `json:"id"`
	Author          string   `json:"author"`
	Epoch           uint16   `json:"epoch"`
	CIDs            []string `json:"cids"`
	Retained        int      `json:"retained"`
	Status          string   `json:"status"`
	Reason          string   `json:"reason"`
	Witness         bool     `json:"witness"`
	Vault           bool     `json:"vault"`
	Session         bool     `json:"session"`
	Registered      bool     `json:"registered"`
}

func standaloneBundleID(author string, epoch uint16) string { return bondedBundleID(8, author, epoch) }
func (s *standaloneServer) canonicalPackage(author string, epoch uint16, cids []string) (standalonePackage, error) {
	p, e := canonicalPackage(author, epoch, cids)
	if s.cfg.Version == 9 {
		p.ProtocolVersion = 9
		p.ID = bondedBundleID(9, author, epoch)
	}
	return p, e
}
func (s *standaloneServer) readPackage(dir string) (standalonePackage, error) {
	p, e := readStandalonePackage(dir)
	if e == nil && ((p.ProtocolVersion == 9) != (s.cfg.Version == 9)) {
		return p, errors.New("package belongs to another market version")
	}
	return p, e
}
func (s *standaloneServer) packageDir(id string) (string, error) {
	if _, e := v8hex(id, 32); e != nil {
		return "", e
	}
	return filepath.Join(s.cfg.DataDir, "packages", id), nil
}
func canonicalPackage(author string, epoch uint16, cids []string) (standalonePackage, error) {
	p := standalonePackage{Author: author, Epoch: epoch, CIDs: append([]string{}, cids...)}
	if _, e := v8hex(author, 20); e != nil || epoch == 0 || len(cids) > 32 {
		return p, errors.New("invalid package identity or inventory")
	}
	sort.Strings(p.CIDs)
	for i, cid := range p.CIDs {
		if !isFlipCID(cid) || (i > 0 && p.CIDs[i-1] == cid) {
			return p, errors.New("invalid or duplicate flip CID")
		}
	}
	p.ID = standaloneBundleID(author, epoch)
	return p, nil
}

func sameCIDInventory(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	left := append([]string(nil), a...)
	right := append([]string(nil), b...)
	sort.Strings(left)
	sort.Strings(right)
	return sameStrings(left, right)
}

func (s *standaloneServer) savePackage(p standalonePackage) error {
	dir, e := s.packageDir(p.ID)
	if e != nil {
		return e
	}
	if e = standalonePrivateDir(dir); e != nil {
		return e
	}
	if e = standalonePrivateDir(filepath.Join(dir, "blobs")); e != nil {
		return e
	}
	// Every changed inventory has its own immutable snapshot; secret witnesses
	// and earlier inventories are never overwritten when authors add/delete flips.
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	digest := sha256.Sum256(raw)
	revision := filepath.Join(dir, "inventory-"+hex.EncodeToString(digest[:])+".json")
	if _, e = os.Lstat(revision); os.IsNotExist(e) {
		if e = writeNewPoolV7PrivateFile(revision, raw); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	return writePrivateJSON(filepath.Join(dir, "inventory.json"), p)
}
func readStandalonePackage(dir string) (standalonePackage, error) {
	raw, e := readBondedV8Private(filepath.Join(dir, "inventory.json"), 32768)
	if e != nil {
		return standalonePackage{}, e
	}
	var p standalonePackage
	if e = decodeStrict(raw, &p); e != nil {
		return p, e
	}
	expected, e := canonicalPackage(p.Author, p.Epoch, p.CIDs)
	if p.ProtocolVersion == 9 {
		expected.ProtocolVersion = 9
		expected.ID = bondedBundleID(9, p.Author, p.Epoch)
	} else if p.ProtocolVersion != 0 && p.ProtocolVersion != 8 {
		return p, errors.New("unsupported package protocol")
	}
	if e != nil || expected.ID != p.ID || !sameStrings(expected.CIDs, p.CIDs) {
		return p, errors.New("saved package inventory is invalid")
	}
	return expected, nil
}
func (s *standaloneServer) v8Collect(m *bondedV8Snapshot) (any, error) {
	var epoch struct {
		Epoch uint16 `json:"epoch"`
	}
	if e := s.client.call("dna_epoch", []any{}, &epoch); e != nil {
		return nil, e
	}
	var identity struct {
		Flips []string `json:"flips"`
	}
	if e := s.client.call("dna_identity", []any{"0x" + m.Identity}, &identity); e != nil {
		return nil, e
	}
	var after struct {
		Epoch uint16 `json:"epoch"`
	}
	if e := s.client.call("dna_epoch", []any{}, &after); e != nil {
		return nil, e
	}
	who, e := s.client.bondedIdentity()
	if e != nil || who != m.Identity || after.Epoch != epoch.Epoch {
		return nil, errors.New("epoch or identity changed during discovery; retry")
	}
	p, e := s.canonicalPackage(m.Identity, epoch.Epoch, identity.Flips)
	if e != nil {
		return nil, e
	}
	if len(p.CIDs) > 0 {
		if e = s.savePackage(p); e != nil {
			return nil, e
		}
		dir, _ := s.packageDir(p.ID)
		for _, cid := range p.CIDs {
			file := filepath.Join(dir, "blobs", cid)
			if raw, e := readBondedV8Private(file, maxRetainedFlipBytes); e == nil {
				if ok, _ := verifyBlobCID(cid, raw, nil); !ok {
					return nil, errors.New("saved encrypted flip failed its content check")
				}
				continue
			} else if !os.IsNotExist(e) {
				return nil, e
			}
			raw, e := ipfsFetch(s.client)(cid)
			if e != nil {
				continue
			}
			if len(raw) > maxRetainedFlipBytes {
				return nil, errors.New("oversized encrypted flip")
			}
			if ok, _ := verifyBlobCID(cid, raw, nil); !ok {
				return nil, errors.New("node returned a flip with the wrong CID")
			}
			if e = writeNewPoolV7PrivateFile(file, raw); e != nil {
				return nil, e
			}
		}
	}
	for _, b := range m.Bundles {
		if b.Author != m.Identity {
			continue
		}
		dir, _ := s.packageDir(b.ID)
		if _, err := os.Lstat(filepath.Join(dir, "inventory.json")); os.IsNotExist(err) {
			pkg, err := s.canonicalPackage(b.Author, b.Epoch, b.CIDs)
			if err != nil {
				return nil, err
			}
			if err = s.savePackage(pkg); err != nil {
				return nil, err
			}
		}
	}
	// Import public session evidence only; never mutate an external capture.
	for _, root := range []string{s.cfg.ArchiveDir, filepath.Join(s.cfg.DataDir, "capture")} {
		if root != "" {
			if e = s.v8ImportArchive(root, m.Identity); e != nil {
				s.v8.Capture = "Some archive evidence could not be verified against the local node."
			}
		}
	}
	if s.cfg.SellerClient && epoch.Epoch > 1 {
		available, err := s.v9SharedEpochs(m.Identity)
		if err != nil {
			return nil, err
		}
		for _, oldEpoch := range available {
			if oldEpoch >= epoch.Epoch {
				continue
			}
			attempted, err := s.v9FetchSharedEvidence(m.Identity, oldEpoch)
			if err != nil {
				return nil, err
			}
			if attempted {
				break // Bound collection to one remote session per cycle.
			}
		}
	}
	entries, e := os.ReadDir(filepath.Join(s.cfg.DataDir, "packages"))
	if e != nil {
		return nil, e
	}
	for _, entry := range entries {
		if !entry.IsDir() || !isHex(entry.Name(), 32) {
			continue
		}
		dir, _ := s.packageDir(entry.Name())
		saved, err := s.readPackage(dir)
		if err != nil || saved.Author != m.Identity {
			continue
		}
		for _, cid := range saved.CIDs {
			file := filepath.Join(dir, "blobs", cid)
			if _, err := os.Lstat(file); !os.IsNotExist(err) {
				continue
			}
			raw, err := ipfsFetch(s.client)(cid)
			if err != nil {
				continue
			}
			if len(raw) > maxRetainedFlipBytes {
				return nil, errors.New("oversized encrypted flip")
			}
			if ok, _ := verifyBlobCID(cid, raw, nil); !ok {
				return nil, errors.New("encrypted flip failed CID verification")
			}
			if err = writeNewPoolV7PrivateFile(file, raw); err != nil {
				return nil, err
			}
		}
	}
	return map[string]any{"collected": len(p.CIDs), "epoch": epoch.Epoch}, nil
}

func (s *standaloneServer) v9SharedEpochs(author string) ([]uint16, error) {
	if !s.cfg.SellerClient || !isHex(author, addressBytes) || author != strings.ToLower(author) ||
		!s.client.bondedRPCAllowed() {
		return nil, errors.New("seller shared evidence needs the local identity and v9 HTTPS relay")
	}
	if who, err := s.client.bondedIdentity(); err != nil || who != author {
		return nil, errors.New("seller identity changed before shared evidence discovery")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, status, err := s.v9SharedGET(ctx, "/evidence/v1/authors/"+author+"/epochs", 8<<10)
	if status == http.StatusNotFound && err == nil {
		return nil, errors.New("shared relay has no author evidence index; update the relay")
	}
	if err != nil {
		return nil, fmt.Errorf("shared evidence index unavailable: %w", err)
	}
	var index struct {
		Epochs []uint16 `json:"epochs"`
	}
	if err := decodeStrict(raw, &index); err != nil || len(index.Epochs) > 256 {
		return nil, errors.New("shared evidence index is invalid")
	}
	for i, n := range index.Epochs {
		if n == 0 || (i > 0 && n >= index.Epochs[i-1]) {
			return nil, errors.New("shared evidence index is not ordered")
		}
	}
	return index.Epochs, nil
}

// Fetch only public session evidence and original ciphertext from the same
// HTTPS relay as /rpc. Keys, vaults, and passphrases never cross this boundary.
// The first result reports whether a remote fetch was attempted this cycle.
func (s *standaloneServer) v9FetchSharedEvidence(author string, epoch uint16) (bool, error) {
	if !s.cfg.SellerClient || epoch == 0 || !s.client.bondedRPCAllowed() {
		return false, errors.New("seller shared evidence needs the local identity and v9 HTTPS relay")
	}
	if who, err := s.client.bondedIdentity(); err != nil || who != author {
		return false, errors.New("seller identity changed before shared evidence collection")
	}
	id := bondedBundleID(9, author, epoch)
	dir, err := s.packageDir(id)
	if err != nil {
		return false, err
	}
	var saved SessionBundle
	savedRaw, savedErr := readBondedV8Private(filepath.Join(dir, "session.json"), 8<<20)
	if savedErr == nil {
		if decodeStrict(savedRaw, &saved) != nil || saved.Format != sessionBundleFormat ||
			saved.Version != sessionBundleVersion || saved.Author != author || saved.Epoch != epoch {
			return false, errors.New("existing session evidence is invalid; original preserved")
		}
		verified, err := sessionBundleFromAnchors(author, &saved.Inventory, &saved.Verdict)
		if err != nil || verified.Epoch != epoch || !sameStrings(verified.Manifest, saved.Manifest) {
			return false, errors.New("existing session evidence is invalid; original preserved")
		}
		complete := true
		for _, cid := range saved.Manifest {
			blob, err := readBondedV8Private(filepath.Join(dir, "blobs", cid), maxRetainedFlipBytes)
			if err != nil {
				complete = false
				break
			}
			if ok, _ := verifyBlobCID(cid, blob, nil); !ok {
				complete = false
				break
			}
		}
		if complete {
			return false, nil
		}
	} else if !os.IsNotExist(savedErr) {
		return false, savedErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	base := "/evidence/v1/epochs/" + fmt.Sprint(epoch) + "/authors/" + author + "/"
	raw, status, err := s.v9SharedGET(ctx, base+"session", 8<<20)
	if status == http.StatusNotFound && err == nil {
		return true, nil // The advertised session disappeared; retry next cycle.
	}
	if err != nil {
		return true, fmt.Errorf("shared session evidence unavailable: %w", err)
	}
	var session SessionBundle
	if err := decodeStrict(raw, &session); err != nil || session.Format != sessionBundleFormat ||
		session.Version != sessionBundleVersion || session.Author != author || session.Epoch != epoch {
		return true, errors.New("shared session belongs to another author or epoch")
	}
	if _, err := s.canonicalPackage(author, epoch, session.Manifest); err != nil {
		return true, err
	}
	if err := directVerifiedSession(s.client, &session); err != nil {
		return true, fmt.Errorf("shared session failed canonical header verification: %w", err)
	}
	if savedErr == nil && (saved.Inventory.RootHex != session.Inventory.RootHex ||
		saved.Verdict.RootHex != session.Verdict.RootHex || !sameStrings(saved.Manifest, session.Manifest)) {
		return true, errors.New("shared session differs from existing evidence; original preserved")
	}
	blobs := make(map[string][]byte, len(session.Manifest))
	for _, cid := range session.Manifest {
		blob, _, err := s.v9SharedGET(ctx, base+"blobs/"+cid, maxRetainedFlipBytes)
		if err != nil {
			return true, fmt.Errorf("shared encrypted flip unavailable: %w", err)
		}
		if len(blob) == 0 {
			return true, errors.New("shared encrypted flip is empty")
		}
		if ok, _ := verifyBlobCID(cid, blob, nil); !ok {
			return true, errors.New("shared encrypted flip failed CID verification")
		}
		blobs[cid] = blob
	}
	if err := s.v8SaveSession(&session, raw); err != nil {
		return true, err
	}
	for _, cid := range session.Manifest {
		path := filepath.Join(dir, "blobs", cid)
		if existing, err := readBondedV8Private(path, maxRetainedFlipBytes); err == nil {
			if ok, _ := verifyBlobCID(cid, existing, nil); !ok {
				return true, errors.New("existing encrypted flip failed CID verification; original preserved")
			}
			continue
		} else if !os.IsNotExist(err) {
			return true, err
		}
		if err := writeNewPoolV7PrivateFile(path, blobs[cid]); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (s *standaloneServer) v9SharedGET(ctx context.Context, path string, limit int64) ([]byte, int, error) {
	u, err := url.Parse(s.cfg.RPC)
	if err != nil || !buyerRelayURL(s.cfg.RPC) {
		return nil, 0, errors.New("seller relay URL is invalid")
	}
	u.Path, u.RawPath = path, ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	httpClient := *s.client.http.client
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, resp.StatusCode, nil
		}
		return nil, resp.StatusCode, fmt.Errorf("shared evidence answered HTTP %d", resp.StatusCode)
	}
	raw, err := readBoundedResponse(resp, limit)
	return raw, resp.StatusCode, err
}
func (s *standaloneServer) v8ImportArchive(root, who string) error {
	entries, e := os.ReadDir(root)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if len(entries) > 1000 {
		return errors.New("archive too large")
	}
	for _, ent := range entries {
		if !ent.IsDir() || !strings.HasPrefix(ent.Name(), "epoch-") {
			continue
		}
		dir := filepath.Join(root, ent.Name())
		info, e := os.Lstat(dir)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		files, e := os.ReadDir(dir)
		if e != nil {
			return e
		}
		if len(files) > 1000 {
			return errors.New("epoch archive too large")
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), "-session-bundle.json") {
				continue
			}
			source := filepath.Join(dir, file.Name())
			raw, e := readBondedV8Private(source, 8<<20)
			if e != nil {
				return e
			}
			var session SessionBundle
			if e = json.Unmarshal(raw, &session); e != nil {
				return e
			}
			if session.Author != who {
				continue
			}
			if e = s.v8SaveSession(&session, raw); e != nil {
				return e
			}
			destination, _ := s.packageDir(bondedBundleID(bondedVersion(s.client), session.Author, session.Epoch))
			nodeName := strings.TrimSuffix(file.Name(), "-session-bundle.json")
			for _, cid := range session.Manifest {
				target := filepath.Join(destination, "blobs", cid)
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					continue
				}
				blob, err := readBondedV8Private(filepath.Join(dir, "encrypted-blobs", nodeName, cid), maxRetainedFlipBytes)
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					return err
				}
				if ok, _ := verifyBlobCID(cid, blob, nil); !ok {
					return errors.New("archived flip failed CID verification")
				}
				if err = writeNewPoolV7PrivateFile(target, blob); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (s *standaloneServer) v8SaveSession(session *SessionBundle, raw []byte) error {
	if e := directVerifiedSession(s.client, session); e != nil {
		return errors.New("session does not match canonical validation evidence")
	}
	p, e := s.canonicalPackage(session.Author, session.Epoch, session.Manifest)
	if e != nil {
		return e
	}
	dir, _ := s.packageDir(p.ID)
	// Session manifests are authoritative after validation. Prior versions remain.
	if e = s.savePackage(p); e != nil {
		return e
	}
	path := filepath.Join(dir, "session.json")
	if old, e := readBondedV8Private(path, 8<<20); e == nil {
		var previous SessionBundle
		if e = json.Unmarshal(old, &previous); e != nil || previous.Inventory.RootHex != session.Inventory.RootHex || previous.Verdict.RootHex != session.Verdict.RootHex || !sameStrings(previous.Manifest, session.Manifest) {
			return errors.New("different session evidence already saved; original preserved")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	return writeNewPoolV7PrivateFile(path, raw)
}
func (s *standaloneServer) v8Packages(m *bondedV8Snapshot, epoch uint16) ([]standalonePackage, error) {
	result := []standalonePackage{}
	entries, e := os.ReadDir(filepath.Join(s.cfg.DataDir, "packages"))
	if e != nil {
		return nil, e
	}
	for _, ent := range entries {
		if !ent.IsDir() || !isHex(ent.Name(), 32) {
			continue
		}
		dir := filepath.Join(s.cfg.DataDir, "packages", ent.Name())
		info, e := os.Lstat(dir)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("unsafe package folder")
		}
		p, e := s.readPackage(dir)
		if e != nil {
			return nil, e
		}
		if p.Author != m.Identity {
			continue
		}
		for _, cid := range p.CIDs {
			raw, e := readBondedV8Private(filepath.Join(dir, "blobs", cid), maxRetainedFlipBytes)
			if e == nil {
				if ok, _ := verifyBlobCID(cid, raw, nil); ok {
					p.Retained++
				}
			}
		}
		commit := inventoryCommitment(p.Author, p.Epoch, p.CIDs)
		_, e = readBondedV8Private(filepath.Join(dir, "witness-"+commit+".json"), 48<<20)
		p.Witness = e == nil
		_, e = readBondedV8Private(filepath.Join(dir, "keys.vault"), 1<<20)
		p.Vault = e == nil
		qualified := false
		if raw, e := readBondedV8Private(filepath.Join(dir, "session.json"), 8<<20); e == nil {
			var session SessionBundle
			if json.Unmarshal(raw, &session) == nil && session.Author == p.Author && session.Epoch == p.Epoch && sameCIDInventory(session.Manifest, p.CIDs) && directVerifiedSession(s.client, &session) == nil {
				p.Session = true
				if s.cfg.Version == 9 {
					verdict, err := readBondedV9SessionVerdict(&session)
					qualified = err == nil && verdict.Clean && int(verdict.Flips) == len(p.CIDs)
				}
			}
		}
		p.Status = "collecting"
		p.Reason = fmt.Sprintf("%d of %d encrypted flips saved and checked.", p.Retained, len(p.CIDs))
		if p.Retained == len(p.CIDs) {
			p.Status = "waiting"
			p.Reason = "Waiting for validation. Keep your node and this app open."
			if s.cfg.SellerClient {
				p.Reason = "Waiting for validation evidence. Keep this app open."
			}
			if p.Epoch < epoch {
				p.Status = "missing"
				p.Reason = "Canonical validation evidence is missing. Import the saved session archive."
			}
			if !p.Vault {
				p.Status = "protect"
				p.Reason = "Protect this package with an encrypted local key vault."
			}
			if p.Session && p.Vault {
				p.Status = "prepare"
				p.Reason = "Validation verified. Add the contract-bound content proof to register this package."
				if s.cfg.Version == 9 {
					if qualified {
						p.Status = "ready"
						p.Reason = "Canonical validation result and encrypted flips checked. Keys are protected locally."
					} else {
						p.Status = "unqualified"
						p.Reason = "This session does not have a clean, matching validation result for a v9 listing."
					}
				}
			} else if p.Session && s.cfg.Version == 9 && !qualified {
				p.Status = "unqualified"
				p.Reason = "This session does not have a clean, matching validation result for a v9 listing."
			}
		}
		for _, b := range m.Bundles {
			if b.ID != p.ID || b.Author != p.Author || b.Inventory != commit {
				continue
			}
			p.Registered = true
			open := false
			for _, sale := range m.Sells {
				if sale.ID == b.Sale && sale.Bundle == b.ID && (sale.State == 1 || sale.State == 2 || sale.State == 3) {
					open = true
				}
			}
			for _, trade := range m.Trades {
				if trade.Bundle == b.ID && trade.State <= 4 {
					open = true
				}
			}
			if open {
				p.Status = "queued"
				p.Reason = "Sale in progress. Proceeds remain pending until delivery is settled."
			} else if b.Used {
				p.Status = "sold"
				p.Reason = "This package has already been used."
			} else if p.Vault && p.Retained == len(p.CIDs) {
				p.Status = "ready"
				p.Reason = "Registered package. Your encrypted key vault is available."
				if s.cfg.Version == 9 {
					p.Reason = "Package available. Keys are checked locally before listing."
				}
			} else {
				p.Status = "missing"
				p.Reason = "Registered, but local keys or encrypted flips are missing."
			}
		}
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Epoch > result[j].Epoch })
	return result, nil
}

// A registered package is only usable here if this installation can deliver
// the proven keys and has preserved every original encrypted flip.
func (s *standaloneServer) v8CheckSellerPackage(m *bondedV8Snapshot, id, pass string) error {
	dir, e := s.packageDir(id)
	if e != nil {
		return e
	}
	p, e := s.readPackage(dir)
	if e != nil {
		return errors.New("collect the package before selling")
	}
	if s.cfg.Version == 9 {
		r := &bondedV8Request{ProtocolVersion: 9, Operation: "queueSell", Caller: m.Identity, SessionFile: filepath.Join(dir, "session.json"), VaultFile: filepath.Join(dir, "keys.vault")}
		if e = fillBondedV9Listing(s.client, s.cfg.Contract, r, pass); e != nil {
			return e
		}
		if r.Bundle != id {
			return errors.New("package identity mismatch")
		}
		return nil
	}
	var bundle *bondedV8Bundle
	for i := range m.Bundles {
		if m.Bundles[i].ID == id {
			bundle = &m.Bundles[i]
		}
	}
	if bundle == nil || p.Author != m.Identity || bundle.Author != p.Author || bundle.Epoch != p.Epoch || !sameStrings(bundle.CIDs, p.CIDs) {
		return errors.New("package differs from the registered inventory")
	}
	secrets, vault, e := directVaultSecrets(filepath.Join(dir, "keys.vault"), pass)
	if e != nil {
		return errors.New("unlock the package with its vault passphrase before selling")
	}
	defer clear(secrets)
	digest := sha256.Sum256(secrets)
	author, e := directAddress(vault.Address)
	if e != nil || author != p.Author || vault.Epoch != p.Epoch || hex.EncodeToString(digest[:]) != bundle.Keys {
		return errors.New("local keys differ from the proven package")
	}
	for _, cid := range p.CIDs {
		raw, e := readBondedV8Private(filepath.Join(dir, "blobs", cid), maxRetainedFlipBytes)
		if e != nil {
			return errors.New("collect every encrypted flip before selling")
		}
		if ok, _ := verifyBlobCID(cid, raw, nil); !ok {
			return errors.New("saved flip failed CID verification")
		}
	}
	return nil
}
func (s *standaloneServer) v8Protect(r *http.Request) (any, error) {
	var in struct {
		Bundle     string `json:"bundle"`
		Passphrase string `json:"passphrase"`
	}
	if e := v8JSON(r, &in); e != nil {
		return nil, e
	}
	if len(in.Passphrase) < 12 || len(in.Passphrase) > 1024 {
		return nil, errors.New("use a vault passphrase of 12 or more characters")
	}
	if s.cfg.NodeKey == "" {
		return nil, errors.New("connect the owner's local signing key to protect this package")
	}
	dir, e := s.packageDir(in.Bundle)
	if e != nil {
		return nil, e
	}
	p, e := s.readPackage(dir)
	if e != nil {
		return nil, e
	}
	m, e := s.v8ReadMarket()
	if e != nil {
		return nil, e
	}
	if p.Author != m.Identity {
		return nil, errors.New("package belongs to another identity")
	}
	key, e := crypto.LoadECDSA(s.cfg.NodeKey)
	if e != nil {
		return nil, errors.New("owner key unavailable")
	}
	who, e := nodekeyAddressOf(key)
	if e != nil || who != p.Author {
		return nil, errors.New("owner key does not match this package")
	}
	keys, e := DeriveEpochKeys(key, p.Epoch)
	if e != nil {
		return nil, e
	}
	payload, e := json.Marshal(keys.ToVaultPayload())
	if e != nil {
		return nil, e
	}
	defer clear(payload)
	for _, cid := range p.CIDs {
		raw, e := readBondedV8Private(filepath.Join(dir, "blobs", cid), maxRetainedFlipBytes)
		if e != nil {
			return nil, errors.New("collect all encrypted flips first")
		}
		if ok, _ := verifyBlobCID(cid, raw, nil); !ok {
			return nil, errors.New("encrypted flip failed CID check")
		}
		pub, a, b, e := ParseFlipBlob(raw)
		if e != nil {
			return nil, e
		}
		author, e := crypto.UnmarshalPubkey(pub)
		if e != nil || hex.EncodeToString(crypto.PubkeyToAddress(*author).Bytes()) != p.Author {
			return nil, errors.New("flip author mismatch")
		}
		a, e = keys.DecryptPublicPart(a)
		if e != nil {
			return nil, errors.New("public flip cannot decrypt")
		}
		b, e = keys.DecryptPrivatePart(b)
		if e != nil {
			return nil, errors.New("private flip cannot decrypt")
		}
		if e = validateDirectFlipParts(a, b); e != nil {
			return nil, e
		}
	}
	if e = WriteVault(filepath.Join(dir, "keys.vault"), in.Passphrase, payload); e != nil {
		return nil, e
	}
	commit := inventoryCommitment(p.Author, p.Epoch, p.CIDs)
	witness := filepath.Join(dir, "witness-"+commit+".json")
	if _, e = os.Lstat(witness); os.IsNotExist(e) {
		tmp, e := os.MkdirTemp(dir, ".witness-")
		if e != nil {
			return nil, e
		}
		defer os.RemoveAll(tmp)
		path := filepath.Join(tmp, "content.json")
		if e = writeRegistrationBlobInput(path, "", s.cfg.NodeKey, "", filepath.Join(dir, "blobs"), &SessionBundle{Author: p.Author, Epoch: p.Epoch, Manifest: p.CIDs}, nil); e != nil {
			return nil, e
		}
		raw, e := readBondedV8Private(path, 48<<20)
		if e != nil {
			return nil, e
		}
		defer clear(raw)
		if e = writeNewPoolV7PrivateFile(witness, raw); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	}
	return map[string]bool{"protected": true}, nil
}
func (s *standaloneServer) v8Import(r *http.Request) (any, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, standaloneUploadLimit)
	if e := r.ParseMultipartForm(standaloneUploadLimit); e != nil {
		return nil, e
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	tmp, e := os.MkdirTemp(s.cfg.DataDir, ".import-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(tmp)
	sessionFile, e := standaloneSaveUpload(r, "session", tmp)
	if e != nil || sessionFile == "" {
		return nil, errors.New("select a canonical session bundle")
	}
	raw, e := readBondedV8Private(sessionFile, 8<<20)
	if e != nil {
		return nil, e
	}
	var session SessionBundle
	if e = json.Unmarshal(raw, &session); e != nil {
		return nil, e
	}
	m, e := s.v8ReadMarket()
	if e != nil {
		return nil, e
	}
	if session.Author != m.Identity {
		return nil, errors.New("session belongs to another identity")
	}
	if e = s.v8SaveSession(&session, raw); e != nil {
		return nil, e
	}
	return map[string]bool{"imported": true}, nil
}
