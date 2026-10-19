package main

// The standalone app is a local interface to the same guarded v7 CLI. It has
// no remote backend and never serves the node API key or signing key to a page.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

//go:embed standalone_app.html standalone_app.js standalone_app.css
var standaloneAssets embed.FS

const standaloneUploadLimit = 24 << 20

var standaloneID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var standaloneEstimateGas = regexp.MustCompile(`read-only estimate ([0-9]+) gas; no transaction signed or sent`)
var standaloneOperations = map[string]bool{
	"qualify": true, "proveBundle": true, "seedBundle": true,
	"list": true, "buy": true, "buyPublicSeed": true,
	"confirm": true, "challenge": true, "startFallback": true,
	"proveAndReveal": true, "refund": true, "relist": true,
	"cancel": true, "expireHead": true, "topUpBond": true,
	"slashUnproved": true,
}

type standaloneConfig struct {
	RPC, KeyFile, Contract, CodeHash, NodeKey, DataDir string
}

type standaloneServer struct {
	cfg        standaloneConfig
	client     *client
	origin     string
	token      string
	mu         sync.Mutex
	runCommand func(context.Context, []string, string) ([]byte, error)
	market     func() (any, error)
}

type standaloneAction struct {
	ID            string                `json:"id"`
	ReviewHash    string                `json:"reviewHash"`
	Operation     string                `json:"operation"`
	Contract      string                `json:"contract"`
	CodeHash      string                `json:"codeHash"`
	OfferID       string                `json:"offerId,omitempty"`
	OrderID       uint64                `json:"orderId,omitempty"`
	Epoch         uint16                `json:"epoch,omitempty"`
	CoveredFlips  uint32                `json:"coveredFlips,omitempty"`
	Amount        string                `json:"amount"`
	Price         string                `json:"price,omitempty"`
	MinimumPayout string                `json:"minimumPayout,omitempty"`
	MaximumFee    string                `json:"maximumFee"`
	Author        string                `json:"author,omitempty"`
	Buyer         string                `json:"buyer,omitempty"`
	Signer        string                `json:"signer"`
	PreparedAt    string                `json:"preparedAt"`
	Attempted     bool                  `json:"attempted"`
	Submission    *standaloneSubmission `json:"submission,omitempty"`
}

type standaloneSubmission struct {
	TransactionHash string `json:"transactionHash,omitempty"`
	Included        bool   `json:"included"`
	Success         bool   `json:"success"`
	Note            string `json:"note,omitempty"`
}

type standalonePrivateResult struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

func randomStandaloneID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func standalonePrivateDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("app data directory must be a private, ordinary directory")
	}
	return nil
}

func newStandaloneServer(cfg standaloneConfig) (*standaloneServer, error) {
	if !directLocalRPC(cfg.RPC) {
		return nil, errors.New("standalone app requires loopback node RPC")
	}
	contract, err := directAddress(cfg.Contract)
	if err != nil {
		return nil, err
	}
	if !isHex(strings.ToLower(strings.TrimPrefix(cfg.CodeHash, "0x")), 32) {
		return nil, errors.New("independently verified deployed code hash is required")
	}
	cfg.Contract = contract
	cfg.CodeHash = strings.ToLower(strings.TrimPrefix(cfg.CodeHash, "0x"))
	if cfg.KeyFile == "" {
		return nil, errors.New("node RPC key file is required")
	}
	key, err := resolveRPCKey(cfg.KeyFile, true)
	if err != nil {
		return nil, err
	}
	if cfg.NodeKey != "" {
		info, err := os.Lstat(cfg.NodeKey)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, errors.New("nodekey must be an owner-only regular file")
		}
	}
	if cfg.DataDir == "" {
		return nil, errors.New("private app data directory is required")
	}
	if err := standalonePrivateDir(cfg.DataDir); err != nil {
		return nil, err
	}
	if err := standalonePrivateDir(filepath.Join(cfg.DataDir, "actions")); err != nil {
		return nil, err
	}
	if err := standalonePrivateDir(filepath.Join(cfg.DataDir, "private")); err != nil {
		return nil, err
	}
	c := &client{url: cfg.RPC, key: key, http: newHTTPClient()}
	if err := checkBuyTarget(c, contract, contract, cfg.CodeHash); err != nil {
		return nil, err
	}
	token, err := randomStandaloneID()
	if err != nil {
		return nil, err
	}
	s := &standaloneServer{cfg: cfg, client: c, token: token}
	s.runCommand = s.command
	s.market = func() (any, error) { return readPoolV7MarketSnapshot(s.client, s.cfg.Contract, s.cfg.CodeHash) }
	return s, nil
}

func (s *standaloneServer) command(ctx context.Context, args []string, passphrase string) ([]byte, error) {
	bin, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, PassphraseEnvVar+"=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	if passphrase != "" {
		cmd.Env = append(cmd.Env, PassphraseEnvVar+"="+passphrase)
	}
	return cmd.CombinedOutput()
}

func (s *standaloneServer) actionDir(id string) (string, error) {
	if !standaloneID.MatchString(id) {
		return "", errors.New("invalid action ID")
	}
	return filepath.Join(s.cfg.DataDir, "actions", id), nil
}

func (s *standaloneServer) privateDir(id string) (string, error) {
	if !standaloneID.MatchString(id) {
		return "", errors.New("invalid private file ID")
	}
	return filepath.Join(s.cfg.DataDir, "private", id), nil
}

func (s *standaloneServer) commonArgs(command string) []string {
	return []string{command, "-rpc", s.cfg.RPC, "-key-file", s.cfg.KeyFile,
		"-network", NetworkNamePinned, "-contract", s.cfg.Contract,
		"-code-hash", s.cfg.CodeHash, "-direct-version", "7"}
}

func standaloneSaveUpload(r *http.Request, field, dir string) (string, error) {
	file, _, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	path := filepath.Join(dir, field)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer out.Close()
	n, err := io.Copy(out, io.LimitReader(file, standaloneUploadLimit+1))
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if n > standaloneUploadLimit {
		_ = os.Remove(path)
		return "", errors.New("selected file is too large")
	}
	if err := out.Sync(); err != nil {
		return "", err
	}
	return path, nil
}

// The CLI's successful simulation includes gas headroom. Add signing-size and
// short-term rate headroom, then keep the result below the node's hard cap.
func standaloneSuggestedFee(output []byte, rate *big.Int) (string, error) {
	match := standaloneEstimateGas.FindSubmatch(output)
	if len(match) != 2 || rate == nil || rate.Sign() <= 0 {
		return "", errors.New("the node did not return a usable v7 fee estimate")
	}
	gas, err := strconv.ParseUint(string(match[1]), 10, 64)
	if err != nil || gas == 0 {
		return "", errors.New("the node returned an invalid v7 fee estimate")
	}
	fee := new(big.Int).Mul(new(big.Int).SetUint64(gas), rate)
	fee.Mul(fee, big.NewInt(5))
	fee.Add(fee, big.NewInt(3))
	fee.Div(fee, big.NewInt(4))
	if fee.Cmp(maxMaxFeeFor(rate)) > 0 {
		return "", errors.New("the estimated network fee exceeds the node cap; review this action manually")
	}
	return fee.String(), nil
}

func (s *standaloneServer) prepare(r *http.Request) (standaloneAction, error) {
	if err := r.ParseMultipartForm(standaloneUploadLimit); err != nil {
		return standaloneAction{}, err
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	op := r.FormValue("operation")
	if !standaloneOperations[op] {
		return standaloneAction{}, errors.New("unsupported action")
	}
	maxFee := r.FormValue("maxFee")
	automaticFee := maxFee == ""
	var rate *big.Int
	if automaticFee {
		var err error
		rate, err = readFeePerGas(s.client)
		if err != nil {
			return standaloneAction{}, fmt.Errorf("read current network fee: %w", err)
		}
		maxFee = maxMaxFeeFor(rate).String() // simulation only; never signed
	} else if !isDecimal(maxFee) || maxFee == "0" {
		return standaloneAction{}, errors.New("enter a positive maximum fee in base units")
	}
	id, err := randomStandaloneID()
	if err != nil {
		return standaloneAction{}, err
	}
	dir, _ := s.actionDir(id)
	if err := standalonePrivateDir(dir); err != nil {
		return standaloneAction{}, err
	}
	var before string
	if err := s.client.call("dna_getCoinbaseAddr", []any{}, &before); err != nil {
		return standaloneAction{}, err
	}
	before, err = directAddress(before)
	if err != nil {
		return standaloneAction{}, err
	}
	args := s.commonArgs("direct-v7-prepare")
	args = append(args, "-operation", op, "-max-fee", maxFee, "-prepared", filepath.Join(dir, "prepared.json"))
	if offer := r.FormValue("offerId"); offer != "" {
		args = append(args, "-v7-offer-id", offer)
	}
	if order := r.FormValue("orderId"); order != "" {
		if _, err := strconv.ParseUint(order, 10, 64); err != nil {
			return standaloneAction{}, errors.New("invalid order ID")
		}
		args = append(args, "-order", order)
	}
	if minimum := r.FormValue("minimumPayout"); minimum != "" {
		args = append(args, "-minimum-payout", minimum)
	}
	if amount := r.FormValue("amount"); amount != "" {
		args = append(args, "-amount", amount)
	}
	for _, field := range []string{"session", "proof", "registration-host", "vault", "delivery"} {
		path, err := standaloneSaveUpload(r, field, dir)
		if err != nil {
			return standaloneAction{}, err
		}
		if path != "" {
			args = append(args, "-"+field, path)
		}
	}
	if op == "confirm" && r.FormValue("receiptId") != "" {
		if r.MultipartForm.File["delivery"] != nil {
			return standaloneAction{}, errors.New("choose either a saved receipt or an uploaded receipt")
		}
		privateDir, err := s.privateDir(r.FormValue("receiptId"))
		if err != nil {
			return standaloneAction{}, err
		}
		kind, err := os.ReadFile(filepath.Join(privateDir, "kind"))
		if err != nil || string(kind) != "receive" {
			return standaloneAction{}, errors.New("select a saved verified buyer receipt")
		}
		args = append(args, "-delivery", filepath.Join(privateDir, "receipt.json"))
	}
	if op == "buy" {
		args = append(args, "-delivery-key", filepath.Join(dir, "delivery-key"))
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	output, err := s.runCommand(ctx, args, r.FormValue("passphrase"))
	if err != nil {
		return standaloneAction{}, fmt.Errorf("preparation refused: %s", strings.TrimSpace(string(output)))
	}
	if automaticFee {
		freshRate, readErr := readFeePerGas(s.client)
		if readErr != nil {
			return standaloneAction{}, fmt.Errorf("recheck current network fee: %w", readErr)
		}
		if freshRate.Cmp(rate) > 0 {
			rate = freshRate
		}
		maxFee, err = standaloneSuggestedFee(output, rate)
		if err != nil {
			return standaloneAction{}, err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "maximum-fee"), []byte(maxFee), 0600); err != nil {
		return standaloneAction{}, err
	}
	var active string
	if err := s.client.call("dna_getCoinbaseAddr", []any{}, &active); err != nil {
		return standaloneAction{}, err
	}
	signer, err := directAddress(active)
	if err != nil {
		return standaloneAction{}, err
	}
	if signer != before {
		return standaloneAction{}, errors.New("active identity changed during preparation")
	}
	if err := os.WriteFile(filepath.Join(dir, "signer"), []byte(signer), 0600); err != nil {
		return standaloneAction{}, err
	}
	return s.readAction(id)
}

func (s *standaloneServer) privateAction(r *http.Request, kind string) (standalonePrivateResult, error) {
	if kind != "dm" && kind != "receive" {
		return standalonePrivateResult{}, errors.New("unsupported private action")
	}
	if err := r.ParseMultipartForm(standaloneUploadLimit); err != nil {
		return standalonePrivateResult{}, err
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	order := r.FormValue("orderId")
	if value, err := strconv.ParseUint(order, 10, 64); err != nil || value == 0 {
		return standalonePrivateResult{}, errors.New("enter a funded order ID")
	}
	id, err := randomStandaloneID()
	if err != nil {
		return standalonePrivateResult{}, err
	}
	dir, _ := s.privateDir(id)
	if err := standalonePrivateDir(dir); err != nil {
		return standalonePrivateResult{}, err
	}
	args := s.commonArgs("direct-v7-" + kind)
	args = append(args, "-order", order)
	if kind == "dm" {
		vault, err := standaloneSaveUpload(r, "vault", dir)
		if err != nil {
			return standalonePrivateResult{}, err
		}
		if vault == "" {
			return standalonePrivateResult{}, errors.New("select the encrypted seller vault")
		}
		args = append(args, "-vault", vault, "-out", filepath.Join(dir, "ciphertext.txt"))
	} else {
		purchaseID := r.FormValue("purchaseId")
		purchaseDir, err := s.actionDir(purchaseID)
		if err != nil {
			return standalonePrivateResult{}, err
		}
		purchase, err := s.readAction(purchaseID)
		if err != nil || purchase.Operation != "buy" {
			return standalonePrivateResult{}, errors.New("select your saved private purchase")
		}
		message, err := standaloneSaveUpload(r, "message", dir)
		if err != nil {
			return standalonePrivateResult{}, err
		}
		if message == "" {
			return standalonePrivateResult{}, errors.New("select the seller ciphertext file")
		}
		args = append(args, "-message", message, "-purchase", filepath.Join(purchaseDir, "prepared.json"),
			"-delivery-key", filepath.Join(purchaseDir, "delivery-key"), "-vault", filepath.Join(dir, "vault"),
			"-out", filepath.Join(dir, "receipt.json"))
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	output, err := s.runCommand(ctx, args, r.FormValue("passphrase"))
	if err != nil {
		return standalonePrivateResult{}, fmt.Errorf("private file verification refused: %s", strings.TrimSpace(string(output)))
	}
	if err := os.WriteFile(filepath.Join(dir, "kind"), []byte(kind), 0600); err != nil {
		return standalonePrivateResult{}, err
	}
	return standalonePrivateResult{ID: id, Kind: kind}, nil
}

func (s *standaloneServer) readAction(id string) (standaloneAction, error) {
	dir, err := s.actionDir(id)
	if err != nil {
		return standaloneAction{}, err
	}
	path := filepath.Join(dir, "prepared.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return standaloneAction{}, err
	}
	prepared, err := readPreparedAction(path)
	if err != nil || !isPoolV7Format(prepared.Format) || prepared.Contract != s.cfg.Contract || prepared.CodeHash != s.cfg.CodeHash {
		return standaloneAction{}, errors.New("saved action is not for the selected v7 contract and code")
	}
	fee, err := os.ReadFile(filepath.Join(dir, "maximum-fee"))
	if err != nil {
		return standaloneAction{}, err
	}
	signer, err := os.ReadFile(filepath.Join(dir, "signer"))
	if err != nil {
		return standaloneAction{}, err
	}
	digest := sha256.New()
	_, _ = digest.Write(raw)
	_, _ = digest.Write(fee)
	_, _ = digest.Write(signer)
	a := standaloneAction{ID: id, ReviewHash: hex.EncodeToString(digest.Sum(nil)), Operation: prepared.Operation,
		Contract: prepared.Contract, CodeHash: prepared.CodeHash, OfferID: prepared.DirectOfferID,
		OrderID: prepared.DirectOrderID, Epoch: prepared.Epoch, CoveredFlips: prepared.CoveredFlips,
		Amount: prepared.Amount, Price: prepared.Price,
		MinimumPayout: prepared.MinimumPayout, MaximumFee: string(fee), Author: prepared.Author,
		Buyer: prepared.Buyer, PreparedAt: prepared.PreparedAt}
	a.Signer = string(signer)
	if _, err := os.Stat(filepath.Join(dir, "attempted")); err == nil {
		a.Attempted = true
	}
	var signed SubmittedTransaction
	if raw, err := os.ReadFile(filepath.Join(dir, "signed.json")); err == nil && json.Unmarshal(raw, &signed) == nil {
		a.Submission = &standaloneSubmission{TransactionHash: signed.TxHash}
		if signed.Outcome != nil {
			a.Submission.Included, a.Submission.Success, a.Submission.Note = signed.Outcome.Included, signed.Outcome.Success, signed.Outcome.Note
		}
	}
	return a, nil
}

func (s *standaloneServer) submit(r *http.Request) (standaloneAction, error) {
	if s.cfg.NodeKey == "" {
		return standaloneAction{}, errors.New("start the app with your nodekey file to enable signing")
	}
	var request struct {
		ID         string `json:"id"`
		ReviewHash string `json:"reviewHash"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&request); err != nil {
		return standaloneAction{}, err
	}
	a, err := s.readAction(request.ID)
	if err != nil {
		return standaloneAction{}, err
	}
	if request.ReviewHash != a.ReviewHash {
		return standaloneAction{}, errors.New("action changed since review")
	}
	if a.Attempted {
		return standaloneAction{}, errors.New("this action was already attempted; reconcile its saved hash and chain state before any new transaction")
	}
	var active string
	if err := s.client.call("dna_getCoinbaseAddr", []any{}, &active); err != nil {
		return standaloneAction{}, err
	}
	identity, err := directAddress(active)
	if err != nil {
		return standaloneAction{}, err
	}
	if identity != a.Signer {
		return standaloneAction{}, errors.New("active identity changed since preparation")
	}
	key, err := crypto.LoadECDSA(s.cfg.NodeKey)
	if err != nil {
		return standaloneAction{}, fmt.Errorf("load signing key: %w", err)
	}
	keyAddress, err := nodekeyAddressOf(key)
	if err != nil || keyAddress != identity {
		return standaloneAction{}, errors.New("selected signing key does not control the active identity")
	}
	if err := checkBuyTarget(s.client, s.cfg.Contract, s.cfg.Contract, s.cfg.CodeHash); err != nil {
		return standaloneAction{}, err
	}
	var account struct {
		Nonce        uint32 `json:"nonce"`
		MempoolNonce uint32 `json:"mempoolNonce"`
	}
	if err := s.client.call("dna_getBalance", []any{"0x" + identity}, &account); err != nil {
		return standaloneAction{}, err
	}
	if account.Nonce != account.MempoolNonce || account.Nonce == ^uint32(0) {
		return standaloneAction{}, errors.New("identity has pending or exhausted nonce; reconcile before signing")
	}
	var chainEpoch struct {
		Epoch uint16 `json:"epoch"`
	}
	if err := s.client.call("dna_epoch", []any{}, &chainEpoch); err != nil {
		return standaloneAction{}, err
	}
	dir, _ := s.actionDir(a.ID)
	marker, err := os.OpenFile(filepath.Join(dir, "attempted"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return standaloneAction{}, errors.New("signing attempt already recorded")
	}
	_, _ = fmt.Fprintf(marker, "nonce=%d epoch=%d prepared-sha256=%s\n", account.Nonce+1, chainEpoch.Epoch, a.ReviewHash)
	if err := marker.Sync(); err != nil {
		marker.Close()
		return standaloneAction{}, err
	}
	if err := marker.Close(); err != nil {
		return standaloneAction{}, err
	}
	args := s.commonArgs("submit")
	args = append(args, "-prepared", filepath.Join(dir, "prepared.json"), "-nodekey", s.cfg.NodeKey,
		"-nonce", strconv.FormatUint(uint64(account.Nonce+1), 10), "-tx-epoch", strconv.FormatUint(uint64(chainEpoch.Epoch), 10),
		"-max-fee", a.MaximumFee, "-signed", filepath.Join(dir, "signed.json"), "-broadcast")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	output, err := s.runCommand(ctx, args, "")
	if err != nil {
		return standaloneAction{}, fmt.Errorf("signing or broadcast needs reconciliation: %s", strings.TrimSpace(string(output)))
	}
	return s.readAction(a.ID)
}

func (s *standaloneServer) reconcile(id string) (standaloneAction, error) {
	dir, err := s.actionDir(id)
	if err != nil {
		return standaloneAction{}, err
	}
	path := filepath.Join(dir, "signed.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return standaloneAction{}, errors.New("no saved signed transaction is available for this action")
	}
	var signed SubmittedTransaction
	if err := json.Unmarshal(raw, &signed); err != nil {
		return standaloneAction{}, err
	}
	if signed.Contract != s.cfg.Contract || !standaloneID.MatchString(id) || !isHex(strings.TrimPrefix(signed.TxHash, "0x"), 32) {
		return standaloneAction{}, errors.New("saved transaction hash or contract is invalid")
	}
	signed.Outcome = recordOutcome(s.client, signed.TxHash)
	if err := writePrivateJSON(path, &signed); err != nil {
		return standaloneAction{}, err
	}
	return s.readAction(id)
}

func (s *standaloneServer) handler() http.Handler {
	mux := http.NewServeMux()
	assets := map[string]string{"/": "standalone_app.html", "/app.js": "standalone_app.js", "/app.css": "standalone_app.css"}
	for route, file := range assets {
		route, file := route, file
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method refused", 405)
				return
			}
			if r.URL.Path != route {
				http.NotFound(w, r)
				return
			}
			raw, err := standaloneAssets.ReadFile(file)
			if err != nil {
				http.Error(w, "asset unavailable", 500)
				return
			}
			if route == "/" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				raw = []byte(strings.ReplaceAll(string(raw), "__APP_TOKEN__", s.token))
			} else if strings.HasSuffix(route, ".js") {
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			} else {
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
			}
			_, _ = w.Write(raw)
		})
	}
	mux.HandleFunc("/api/market", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method refused", 405)
			return
		}
		var result any
		var err error
		if s.market != nil {
			result, err = s.market()
		} else {
			err = errors.New("market reader unavailable")
		}
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		writeStandaloneJSON(w, result)
	})
	mux.HandleFunc("/api/preview", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method refused", 405)
			return
		}
		preview, err := s.preview(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 503)
			return
		}
		writeStandaloneJSON(w, preview)
	})
	mux.HandleFunc("/api/actions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method refused", 405)
			return
		}
		entries, err := os.ReadDir(filepath.Join(s.cfg.DataDir, "actions"))
		if err != nil {
			http.Error(w, "saved actions unavailable", 500)
			return
		}
		list := make([]standaloneAction, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() || !standaloneID.MatchString(entry.Name()) {
				continue
			}
			if action, err := s.readAction(entry.Name()); err == nil {
				list = append(list, action)
			}
		}
		writeStandaloneJSON(w, list)
	})
	mux.HandleFunc("/api/prepare", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method refused", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, standaloneUploadLimit)
		s.mu.Lock()
		defer s.mu.Unlock()
		action, err := s.prepare(r)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeStandaloneJSON(w, action)
	})
	mux.HandleFunc("/api/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method refused", 405)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		action, err := s.submit(r)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		writeStandaloneJSON(w, action)
	})
	mux.HandleFunc("/api/private/dm", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method refused", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, standaloneUploadLimit)
		s.mu.Lock()
		defer s.mu.Unlock()
		result, err := s.privateAction(r, "dm")
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeStandaloneJSON(w, result)
	})
	mux.HandleFunc("/api/private/receive", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method refused", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, standaloneUploadLimit)
		s.mu.Lock()
		defer s.mu.Unlock()
		result, err := s.privateAction(r, "receive")
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeStandaloneJSON(w, result)
	})
	mux.HandleFunc("/api/private/download/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("X-FOD-Token") != s.token {
			http.Error(w, "download refused", 403)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/private/download/")
		dir, err := s.privateDir(id)
		if err != nil {
			http.Error(w, "unknown delivery", 404)
			return
		}
		kind, err := os.ReadFile(filepath.Join(dir, "kind"))
		if err != nil || string(kind) != "dm" {
			http.Error(w, "unknown delivery", 404)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=flips-ciphertext.txt")
		http.ServeFile(w, r, filepath.Join(dir, "ciphertext.txt"))
	})
	mux.HandleFunc("/api/reconcile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method refused", 405)
			return
		}
		var request struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&request); err != nil {
			http.Error(w, "invalid action ID", 400)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		action, err := s.reconcile(request.ID)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		writeStandaloneJSON(w, action)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src data:; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		if s.origin == "" || r.Host != strings.TrimPrefix(s.origin, "http://") {
			http.Error(w, "host refused", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("X-FOD-Token") != s.token {
			http.Error(w, "local request token refused", 403)
			return
		}
		if r.Method == http.MethodPost {
			if r.Header.Get("Origin") != s.origin || r.Header.Get("X-FOD-Token") != s.token {
				http.Error(w, "local request token refused", 403)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func writeStandaloneJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func runStandaloneApp(cfg standaloneConfig) error {
	s, err := newStandaloneServer(cfg)
	if err != nil {
		return err
	}
	lockPath := filepath.Join(cfg.DataDir, "app.lock")
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("this app data directory is already in use or needs a reviewed stale-lock cleanup")
	}
	_, _ = fmt.Fprintf(lock, "pid=%d\n", os.Getpid())
	_ = lock.Close()
	defer os.Remove(lockPath)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	s.origin = "http://" + listener.Addr().String()
	fmt.Println("Flips on Demand local app:", s.origin)
	fmt.Println("The page uses your own node. Keep this terminal open; no transaction is sent without the review and submit step.")
	if err := openStandaloneBrowser(s.origin); err != nil {
		fmt.Println("Open the local app URL above in your browser.")
	}
	server := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	return server.Serve(listener)
}

func openStandaloneBrowser(url string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{url}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		command, args = "xdg-open", []string{url}
	}
	cmd := exec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
