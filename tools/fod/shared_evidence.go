package main

// A shared node can publish captured public evidence and original encrypted
// flips. The identity key, epoch secrets, local vault and signing remain with
// the author. Only complete, immutable-looking archive records are served.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sharedEvidenceFormat = "fod.shared-evidence-ready:v1"

var sharedAuthorLabel = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

type sharedEvidenceBlob struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type sharedEvidenceReady struct {
	Format                string                        `json:"format"`
	Epoch                 uint16                        `json:"epoch"`
	Author                string                        `json:"author"`
	BundleSHA256          string                        `json:"bundleSha256"`
	Manifest              []string                      `json:"manifest"`
	Blobs                 map[string]sharedEvidenceBlob `json:"blobs"`
	HeadersCheckedAgainst []string                      `json:"headersCheckedAgainst"`
}

type sharedEvidenceLocation struct {
	epoch uint16
	label string
}

type sharedEvidenceHandler struct {
	archive string
	indexMu sync.Mutex
	indexAt time.Time
	byCID   map[string][]sharedEvidenceLocation
}

// The author-address route lets a seller collect their own opted-in evidence
// without knowing the operator's local label. Ambiguous records are refused.
func (h *sharedEvidenceHandler) labelForAuthor(epoch uint16, author string) (string, error) {
	return h.labelForAuthorWithRecord(epoch, author, func(label string) (*sharedEvidenceReady, error) {
		marker, _, _, _, err := sharedEvidenceRecord(h.archive, epoch, label)
		return marker, err
	})
}

func (h *sharedEvidenceHandler) labelForAuthorWithRecord(epoch uint16, author string, verify func(string) (*sharedEvidenceReady, error)) (string, error) {
	root := h.archive
	for _, part := range []string{"public-authors", fmt.Sprintf("epoch-%d", epoch), "authors"} {
		root = filepath.Join(root, part)
		if !sharedArchiveDir(root) {
			return "", errors.New("author index unavailable")
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) > 256 {
		return "", errors.New("author index unavailable")
	}
	found := ""
	for _, entry := range entries {
		if !entry.IsDir() || !sharedAuthorLabel.MatchString(entry.Name()) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if !sharedArchiveDir(dir) {
			continue
		}
		// The marker only filters candidates; matching records still need full verification.
		raw, err := sharedArchiveFile(filepath.Join(dir, "ready.json"), 64<<10)
		if err != nil {
			continue
		}
		var candidate struct {
			Author string `json:"author"`
		}
		if json.NewDecoder(bytes.NewReader(raw)).Decode(&candidate) != nil || candidate.Author != author {
			continue
		}
		marker, err := verify(entry.Name())
		if err != nil || marker.Author != author {
			continue
		}
		if found != "" {
			return "", errors.New("ambiguous author archive")
		}
		found = entry.Name()
	}
	if found == "" {
		return "", errors.New("author archive unavailable")
	}
	return found, nil
}

// Discovery lists only complete opted-in records for a public author address.
// Session and blob requests independently recheck the archive before serving it.
func (h *sharedEvidenceHandler) epochsForAuthor(author string) ([]uint16, error) {
	root := filepath.Join(h.archive, "public-authors")
	if !sharedArchiveDir(root) {
		return []uint16{}, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) > 256 {
		return nil, errors.New("shared archive epoch index is unavailable")
	}
	epochs := make([]uint16, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "epoch-") {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimPrefix(entry.Name(), "epoch-"), 10, 16)
		if err != nil || n == 0 || fmt.Sprintf("epoch-%d", n) != entry.Name() {
			continue
		}
		label, err := h.labelForAuthor(uint16(n), author)
		if err != nil {
			continue
		}
		marker, _, _, dir, err := sharedEvidenceRecord(h.archive, uint16(n), label)
		if err != nil {
			continue
		}
		complete := true
		for _, cid := range marker.Manifest {
			if _, err := sharedEvidenceBlobAt(dir, cid, marker.Blobs[cid]); err != nil {
				complete = false
				break
			}
		}
		if complete {
			epochs = append(epochs, uint16(n))
		}
	}
	sort.Slice(epochs, func(i, j int) bool { return epochs[i] > epochs[j] })
	return epochs, nil
}

func newSharedEvidenceHandler(archive string) (*sharedEvidenceHandler, error) {
	if !filepath.IsAbs(archive) {
		return nil, errors.New("shared archive path must be absolute")
	}
	info, err := os.Lstat(archive)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("shared archive must be an owner-only directory")
	}
	return &sharedEvidenceHandler{archive: archive}, nil
}

// The shared relay serves captured ciphertext from the archive before asking
// the node. The bounded index reads readiness markers only; every selected
// bundle and blob is rechecked before its bytes are returned.
func (h *sharedEvidenceHandler) scanCIDIndex() (map[string][]sharedEvidenceLocation, error) {
	index := make(map[string][]sharedEvidenceLocation)
	root := filepath.Join(h.archive, "public-authors")
	if !sharedArchiveDir(root) {
		return index, nil // The relay may start before the first enrollment.
	}
	epochs, err := os.ReadDir(root)
	if err != nil || len(epochs) > 256 {
		return nil, errors.New("shared archive epoch index is unavailable")
	}
	readyCount := 0
	for _, epochDir := range epochs {
		if !epochDir.IsDir() || !strings.HasPrefix(epochDir.Name(), "epoch-") {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimPrefix(epochDir.Name(), "epoch-"), 10, 16)
		if err != nil || n == 0 || fmt.Sprintf("epoch-%d", n) != epochDir.Name() {
			continue
		}
		parents := []string{filepath.Join(root, epochDir.Name()), filepath.Join(root, epochDir.Name(), "authors")}
		if !sharedArchiveDir(parents[0]) || !sharedArchiveDir(parents[1]) {
			continue
		}
		labels, err := os.ReadDir(parents[1])
		if err != nil || len(labels) > 256 {
			return nil, errors.New("shared archive author index is unavailable")
		}
		for _, label := range labels {
			if !label.IsDir() || !sharedAuthorLabel.MatchString(label.Name()) ||
				!sharedArchiveDir(filepath.Join(parents[1], label.Name())) {
				continue
			}
			raw, err := sharedArchiveFile(filepath.Join(parents[1], label.Name(), "ready.json"), 64<<10)
			if err != nil {
				continue
			}
			var marker sharedEvidenceReady
			if decodeStrict(raw, &marker) != nil || marker.Format != sharedEvidenceFormat ||
				marker.Epoch != uint16(n) || len(marker.Manifest) == 0 || len(marker.Manifest) > 32 {
				continue
			}
			readyCount++
			if readyCount > 4096 {
				return nil, errors.New("shared archive CID index is too large")
			}
			for _, cid := range marker.Manifest {
				if isFlipCID(cid) {
					index[cid] = append(index[cid], sharedEvidenceLocation{epoch: uint16(n), label: label.Name()})
				}
			}
		}
	}
	return index, nil
}

func (h *sharedEvidenceHandler) blobByCID(cid string) ([]byte, error) {
	if !isFlipCID(cid) {
		return nil, errors.New("invalid encrypted flip CID")
	}
	h.indexMu.Lock()
	if h.byCID == nil || time.Since(h.indexAt) >= 10*time.Second {
		index, err := h.scanCIDIndex()
		if err != nil {
			h.indexMu.Unlock()
			return nil, err
		}
		h.byCID, h.indexAt = index, time.Now()
	}
	locations := append([]sharedEvidenceLocation(nil), h.byCID[cid]...)
	h.indexMu.Unlock()
	for _, location := range locations {
		marker, _, _, dir, err := sharedEvidenceRecord(h.archive, location.epoch, location.label)
		if err != nil {
			continue
		}
		expected, ok := marker.Blobs[cid]
		if !ok {
			continue
		}
		if blob, err := sharedEvidenceBlobAt(dir, cid, expected); err == nil {
			return blob, nil
		}
	}
	return nil, errors.New("verified encrypted flip is unavailable")
}

func sharedArchiveFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
		info.Size() <= 0 || info.Size() > limit {
		return nil, errors.New("archive file is absent or unsafe")
	}
	return os.ReadFile(path)
}

func sharedArchiveDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode().Perm()&0o077 == 0
}

func sharedSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func sharedEvidenceRecord(root string, epoch uint16, label string) (*sharedEvidenceReady, *SessionBundle, []byte, string, error) {
	dir := root
	for _, part := range []string{"public-authors", fmt.Sprintf("epoch-%d", epoch), "authors", label} {
		dir = filepath.Join(dir, part)
		if !sharedArchiveDir(dir) {
			return nil, nil, nil, "", errors.New("archive author directory is absent or unsafe")
		}
	}
	markerBytes, err := sharedArchiveFile(filepath.Join(dir, "ready.json"), 64<<10)
	if err != nil {
		return nil, nil, nil, "", err
	}
	var marker sharedEvidenceReady
	if err := decodeStrict(markerBytes, &marker); err != nil ||
		marker.Format != sharedEvidenceFormat || marker.Epoch != epoch ||
		!isHex(marker.Author, addressBytes) || marker.Author != strings.ToLower(marker.Author) ||
		!isHex(marker.BundleSHA256, 32) || marker.BundleSHA256 != strings.ToLower(marker.BundleSHA256) ||
		len(marker.Manifest) == 0 || len(marker.Manifest) > 32 || len(marker.Blobs) != len(marker.Manifest) ||
		len(marker.HeadersCheckedAgainst) != 2 ||
		!sharedAuthorLabel.MatchString(marker.HeadersCheckedAgainst[0]) ||
		!sharedAuthorLabel.MatchString(marker.HeadersCheckedAgainst[1]) ||
		marker.HeadersCheckedAgainst[0] == marker.HeadersCheckedAgainst[1] {
		return nil, nil, nil, "", errors.New("archive is not marked as fully verified")
	}
	bundleBytes, err := sharedArchiveFile(filepath.Join(dir, "session-bundle.json"), 8<<20)
	if err != nil || sharedSHA256(bundleBytes) != marker.BundleSHA256 {
		return nil, nil, nil, "", errors.New("archive bundle digest mismatch")
	}
	var bundle SessionBundle
	if err := decodeStrict(bundleBytes, &bundle); err != nil || bundle.Format != sessionBundleFormat ||
		bundle.Version != sessionBundleVersion || bundle.Author != marker.Author ||
		bundle.Epoch != epoch || !sameStrings(bundle.Manifest, marker.Manifest) {
		return nil, nil, nil, "", errors.New("archive bundle identity or inventory mismatch")
	}
	verified, err := sessionBundleFromAnchors(bundle.Author, &bundle.Inventory, &bundle.Verdict)
	if err != nil || verified.Epoch != epoch || !sameStrings(verified.Manifest, marker.Manifest) {
		return nil, nil, nil, "", errors.New("archive bundle proof mismatch")
	}
	return &marker, &bundle, bundleBytes, dir, nil
}

func sharedEvidenceBlobAt(dir string, cid string, expected sharedEvidenceBlob) ([]byte, error) {
	if !isFlipCID(cid) || !isHex(expected.SHA256, 32) || expected.SHA256 != strings.ToLower(expected.SHA256) ||
		expected.Bytes <= 0 || expected.Bytes > maxRetainedFlipBytes || !sharedArchiveDir(filepath.Join(dir, "encrypted-blobs")) {
		return nil, errors.New("invalid archive blob record")
	}
	_, err := cidBytesFromString(cid)
	if err != nil {
		return nil, err
	}
	raw, err := sharedArchiveFile(filepath.Join(dir, "encrypted-blobs", cid), maxRetainedFlipBytes)
	if err != nil || len(raw) != expected.Bytes || sharedSHA256(raw) != expected.SHA256 {
		return nil, errors.New("archive blob digest mismatch")
	}
	if ok, _ := verifyBlobCID(cid, raw, nil); !ok {
		return nil, errors.New("archive blob CID mismatch")
	}
	return raw, nil
}

func (h *sharedEvidenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	// Require literal path segments: encoded separators and dot segments are not
	// alternate spellings for author labels or CIDs.
	if r.URL.RawPath != "" || strings.Contains(r.URL.Path, "%") {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) == 5 && parts[0] == "evidence" && parts[1] == "v1" &&
		parts[2] == "authors" && parts[4] == "epochs" {
		author := parts[3]
		if !isHex(author, addressBytes) || author != strings.ToLower(author) {
			http.NotFound(w, r)
			return
		}
		epochs, err := h.epochsForAuthor(author)
		if err != nil {
			http.Error(w, "shared archive index unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Epochs []uint16 `json:"epochs"`
		}{Epochs: epochs})
		return
	}
	if (len(parts) != 7 && len(parts) != 8) ||
		parts[0] != "evidence" || parts[1] != "v1" || parts[2] != "epochs" ||
		parts[4] != "authors" ||
		(parts[6] != "session" && parts[6] != "blobs") {
		http.NotFound(w, r)
		return
	}
	if (parts[6] == "session" && len(parts) != 7) || (parts[6] == "blobs" && len(parts) != 8) {
		http.NotFound(w, r)
		return
	}
	n, err := strconv.ParseUint(parts[3], 10, 16)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != parts[3] {
		http.NotFound(w, r)
		return
	}
	label := parts[5]
	if !sharedAuthorLabel.MatchString(label) {
		if !isHex(label, addressBytes) || label != strings.ToLower(label) {
			http.NotFound(w, r)
			return
		}
		label, err = h.labelForAuthor(uint16(n), label)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	marker, _, bundleBytes, dir, err := sharedEvidenceRecord(h.archive, uint16(n), label)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// A stale ready marker must never advertise a set with missing ciphertext.
	for _, cid := range marker.Manifest {
		if _, err := sharedEvidenceBlobAt(dir, cid, marker.Blobs[cid]); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	if parts[6] == "session" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bundleBytes)
		return
	}
	cid := parts[7]
	expected, ok := marker.Blobs[cid]
	if !ok {
		http.NotFound(w, r)
		return
	}
	raw, err := sharedEvidenceBlobAt(dir, cid, expected)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(raw)
}

func newSharedMarketHandler(node *client, contract, codeHash, archiveDir string) (http.Handler, error) {
	base, err := newBuyerRelayHandler(node, contract)
	if err != nil {
		return nil, err
	}
	if err := checkBuyTarget(node, contract, contract, codeHash); err != nil {
		return nil, fmt.Errorf("shared relay contract pin: %w", err)
	}
	archive, err := newSharedEvidenceHandler(archiveDir)
	if err != nil {
		return nil, err
	}
	relay := base.(*buyerRelay)
	relay.seller = true
	relay.archive = archive
	mux := http.NewServeMux()
	mux.Handle("/rpc", relay)
	mux.Handle("/evidence/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !relay.allowIP(r.RemoteAddr) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		archive.ServeHTTP(w, r)
	}))
	return mux, nil
}
