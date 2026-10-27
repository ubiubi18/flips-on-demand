package main

// The standalone app is a local interface to the same guarded v7 CLI. It has
// no remote backend and never serves the node API key or signing key to a page.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

//go:embed standalone_app.html standalone_app.js standalone_app.css standalone_v8.html standalone_v8.js standalone_v8.css standalone_v8_model.mjs session_indexer.py
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
	NoBrowser                                          bool `json:"-"`
	RPC, KeyFile, Contract, CodeHash, NodeKey, DataDir string
	Version                                            int    `json:"version,omitempty"`
	BuyerOnly                                          bool   `json:"buyerOnly,omitempty"`
	SellerClient                                       bool   `json:"sellerClient,omitempty"`
	WalletKey                                          string `json:"walletKey,omitempty"`
	ArchiveDir                                         string `json:"archiveDir,omitempty"`
	NodeDataDir                                        string `json:"nodeDataDir,omitempty"`
}

type standaloneServer struct {
	cfg            standaloneConfig
	client         *client
	origin         string
	token          string
	pairCode       string
	pairFailed     int
	pairRetryAfter time.Time
	nonceDir       string
	mu             sync.Mutex
	runCommand     func(context.Context, []string, string) ([]byte, error)
	market         func() (any, error)
	v8             *standaloneV8Runtime
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
	AttemptNonce  uint32                `json:"attemptNonce,omitempty"`
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

type standaloneNonceReservation struct {
	Signer     string `json:"signer"`
	Nonce      uint32 `json:"nonce"`
	ActionID   string `json:"actionId"`
	ReviewHash string `json:"reviewHash"`
	CreatedAt  string `json:"createdAt"`
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

func buyerWalletAddress(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("buyer wallet key path must be absolute")
	}
	raw, err := readBondedV8Private(path, 128)
	if err != nil {
		return "", fmt.Errorf("read owner-only buyer wallet key: %w", err)
	}
	defer clear(raw)
	key, err := crypto.HexToECDSA(strings.TrimSpace(string(raw)))
	if err != nil {
		return "", errors.New("invalid buyer wallet key")
	}
	return nodekeyAddressOf(key)
}

func sellerIdentityAddress(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("seller identity key path must be absolute")
	}
	raw, err := readBondedV8Private(path, 128)
	if err != nil {
		return "", fmt.Errorf("read owner-only seller identity key: %w", err)
	}
	defer clear(raw)
	key, err := crypto.HexToECDSA(strings.TrimSpace(string(raw)))
	if err != nil {
		return "", errors.New("invalid seller identity key")
	}
	return nodekeyAddressOf(key)
}

func newStandaloneServer(cfg standaloneConfig) (*standaloneServer, error) {
	if cfg.BuyerOnly && cfg.SellerClient {
		return nil, errors.New("choose one remote connection mode")
	}
	if cfg.SellerClient {
		if cfg.Version != 9 || !buyerRelayURL(cfg.RPC) || cfg.KeyFile != "" || cfg.WalletKey != "" ||
			cfg.NodeDataDir != "" || cfg.NodeKey == "" {
			return nil, errors.New("seller-client v9 needs an HTTPS /rpc relay and a local identity key, without node RPC options")
		}
	} else if cfg.BuyerOnly {
		if cfg.Version != 9 || !buyerRelayURL(cfg.RPC) || cfg.KeyFile != "" || cfg.NodeKey != "" || cfg.NodeDataDir != "" || cfg.ArchiveDir != "" {
			return nil, errors.New("buyer-only v9 needs an HTTPS /rpc relay, wallet key, and no node or seller data")
		}
	} else if !directLocalRPC(cfg.RPC) || cfg.WalletKey != "" {
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
	key := ""
	wallet := ""
	if cfg.SellerClient {
		wallet, err = sellerIdentityAddress(cfg.NodeKey)
	} else if cfg.BuyerOnly {
		wallet, err = buyerWalletAddress(cfg.WalletKey)
	} else {
		if cfg.KeyFile == "" {
			return nil, errors.New("node RPC key file is required")
		}
		key, err = resolveRPCKey(cfg.KeyFile, true)
	}
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
	c := &client{url: cfg.RPC, key: key, http: newHTTPClient(), bondedVersion: cfg.Version, buyerOnly: cfg.BuyerOnly, wallet: wallet}
	if cfg.SellerClient {
		c.wallet = ""
		c.sellerClient, c.author, c.authorKeyPath = true, wallet, cfg.NodeKey
	}
	if err := checkBuyTarget(c, contract, contract, cfg.CodeHash); err != nil {
		return nil, err
	}
	if cfg.SellerClient {
		var identity struct {
			Flips []string `json:"flips"`
		}
		if err := c.call("dna_identity", []any{"0x" + c.author}, &identity); err != nil {
			return nil, fmt.Errorf("seller relay identity read unavailable: %w", err)
		}
	}
	token, err := randomStandaloneID()
	if err != nil {
		return nil, err
	}
	pairCode, err := randomStandaloneID()
	if err != nil {
		return nil, err
	}
	s := &standaloneServer{cfg: cfg, client: c, token: token, pairCode: pairCode}
	if cfg.NodeKey != "" || cfg.BuyerOnly {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("nonce journal directory unavailable: %w", err)
		}
		s.nonceDir = filepath.Join(base, "Flips on Demand nonce journal")
		if err := standalonePrivateDir(s.nonceDir); err != nil {
			return nil, err
		}
	}
	s.runCommand = s.command
	s.market = func() (any, error) { return readPoolV7MarketSnapshot(s.client, s.cfg.Contract, s.cfg.CodeHash) }
	if cfg.Version == 8 || cfg.Version == 9 {
		s.v8 = &standaloneV8Runtime{}
		s.market = func() (any, error) { return s.v8Status() }
		if err := standalonePrivateDir(filepath.Join(cfg.DataDir, "packages")); err != nil {
			return nil, err
		}
	}
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

func (s *standaloneServer) nonceReservationPath(signer string, nonce uint32) (string, error) {
	address, err := directAddress(signer)
	if err != nil || nonce == 0 || s.nonceDir == "" {
		return "", errors.New("nonce journal unavailable")
	}
	if err := standalonePrivateDir(s.nonceDir); err != nil {
		return "", err
	}
	return filepath.Join(s.nonceDir, fmt.Sprintf("%s-%d.json", address, nonce)), nil
}

func standaloneSyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// Reserve before creating an action marker or invoking the signing CLI. The
// fixed per-user directory keeps this guard when the app data path changes.
func (s *standaloneServer) reserveNonce(a standaloneAction, nonce uint32) error {
	path, err := s.nonceReservationPath(a.Signer, nonce)
	if err != nil {
		return err
	}
	if !standaloneID.MatchString(a.ID) || !isHex(a.ReviewHash, 32) {
		return errors.New("invalid action for nonce reservation")
	}
	record := standaloneNonceReservation{Signer: a.Signer, Nonce: nonce, ActionID: a.ID,
		ReviewHash: a.ReviewHash, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	err = writePrivateFile(path, false, func(file *os.File) error {
		_, err := file.Write(raw)
		return err
	})
	if errors.Is(err, os.ErrExist) {
		return errors.New("this identity and nonce have an unresolved saved attempt; reconcile it before signing another action")
	}
	if err != nil {
		return fmt.Errorf("nonce journal needs reconciliation: %w", err)
	}
	return nil
}

// A missing receipt leaves the reservation in place. Only an included receipt
// or a chain nonce that has consumed the reserved value permits its removal.
func (s *standaloneServer) reconcileNonceReservation(a standaloneAction, signed SubmittedTransaction) error {
	path, err := s.nonceReservationPath(signed.Sender, signed.AccountNonce)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil // transactions signed before this journal was introduced
	}
	if err != nil {
		return err
	}
	var record standaloneNonceReservation
	if err := json.Unmarshal(raw, &record); err != nil || record.Signer != a.Signer ||
		record.Nonce != signed.AccountNonce || record.ActionID != a.ID || record.ReviewHash != a.ReviewHash {
		return errors.New("nonce journal differs from the saved action; review it manually")
	}
	if signed.Outcome == nil || !signed.Outcome.Included {
		var account struct {
			Nonce uint32 `json:"nonce"`
		}
		if err := s.client.call("dna_getBalance", []any{"0x" + a.Signer}, &account); err != nil {
			return nil // no trustworthy consumed-nonce evidence yet
		}
		if account.Nonce < signed.AccountNonce {
			return nil
		}
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return standaloneSyncDir(s.nonceDir)
}

func (s *standaloneServer) privateDir(id string) (string, error) {
	if !standaloneID.MatchString(id) {
		return "", errors.New("invalid private file ID")
	}
	return filepath.Join(s.cfg.DataDir, "private", id), nil
}

func (s *standaloneServer) commonArgs(command string) []string {
	if s.cfg.BuyerOnly {
		return []string{command, "-rpc", s.cfg.RPC, "-buyer-only",
			"-network", NetworkNamePinned, "-contract", s.cfg.Contract,
			"-code-hash", s.cfg.CodeHash}
	}
	if s.cfg.SellerClient {
		return []string{command, "-rpc", s.cfg.RPC, "-seller-client",
			"-network", NetworkNamePinned, "-contract", s.cfg.Contract,
			"-code-hash", s.cfg.CodeHash}
	}
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
	if s.cfg.Version == 8 || s.cfg.Version == 9 {
		return s.v8PrepareHTTP(r)
	}
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
	if op == "confirm" {
		if r.FormValue("receiptId") == "" {
			return standaloneAction{}, errors.New("verify the seller ciphertext locally and select its saved buyer receipt before confirming")
		}
		if r.MultipartForm.File["delivery"] != nil {
			return standaloneAction{}, errors.New("confirm accepts only a buyer receipt verified by this app")
		}
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
	for _, field := range []string{"session", "proof", "registration-host", "vault"} {
		path, err := standaloneSaveUpload(r, field, dir)
		if err != nil {
			return standaloneAction{}, err
		}
		if path != "" {
			args = append(args, "-"+field, path)
		}
	}
	if op == "confirm" {
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
	if err != nil || !((s.cfg.Version == 8 || s.cfg.Version == 9) && prepared.BondedV8 != nil && prepared.BondedV8.version() == s.cfg.Version && prepared.Format == bondedFormat(s.cfg.Version)+prepared.Operation || (s.cfg.Version != 8 && s.cfg.Version != 9) && isPoolV7Format(prepared.Format)) || prepared.Contract != s.cfg.Contract || prepared.CodeHash != s.cfg.CodeHash {
		return standaloneAction{}, errors.New("saved action is not for the selected market version, contract and code")
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
	if marker, err := os.ReadFile(filepath.Join(dir, "attempted")); err == nil {
		a.Attempted = true
		_, _ = fmt.Sscanf(string(marker), "nonce=%d", &a.AttemptNonce)
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
	keyPath := s.cfg.NodeKey
	if s.cfg.BuyerOnly {
		keyPath = s.cfg.WalletKey
	}
	if keyPath == "" {
		return standaloneAction{}, errors.New("start the app with your nodekey file to enable signing")
	}
	var request struct {
		ID         string `json:"id"`
		ReviewHash string `json:"reviewHash"`
		Passphrase string `json:"passphrase,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&request); err != nil {
		return standaloneAction{}, err
	}
	a, err := s.readAction(request.ID)
	if err != nil {
		return standaloneAction{}, err
	}
	if s.cfg.BuyerOnly && !buyerBondedOperation(a.Operation) {
		return standaloneAction{}, errors.New("buyer-only mode refuses this operation")
	}
	if s.cfg.SellerClient && !sellerClientBondedOperation(a.Operation) {
		return standaloneAction{}, errors.New("seller-client mode refuses this operation")
	}
	signingPassphrase := ""
	if s.cfg.Version == 9 && a.Operation == "queueSell" {
		signingPassphrase = request.Passphrase
		if signingPassphrase == "" && s.v8GrantValid(a.Signer) {
			signingPassphrase = s.v8.Grant.Passphrase
		}
		if signingPassphrase == "" || len(signingPassphrase) > 1024 {
			return standaloneAction{}, errors.New("unlock the package vault before signing this sale")
		}
	}
	if request.ReviewHash != a.ReviewHash {
		return standaloneAction{}, errors.New("action changed since review")
	}
	if a.Attempted {
		return standaloneAction{}, errors.New("this action was already attempted; reconcile its saved hash and chain state before any new transaction")
	}
	identity, err := s.client.bondedIdentity()
	if err != nil {
		return standaloneAction{}, err
	}
	if identity != a.Signer {
		return standaloneAction{}, errors.New("active identity changed since preparation")
	}
	if s.cfg.BuyerOnly {
		if address, err := buyerWalletAddress(keyPath); err != nil || address != identity {
			return standaloneAction{}, errors.New("buyer wallet key changed or is no longer owner-only")
		}
	}
	if s.cfg.SellerClient {
		if address, err := sellerIdentityAddress(keyPath); err != nil || address != identity {
			return standaloneAction{}, errors.New("seller identity key changed or is no longer owner-only")
		}
	}
	key, err := crypto.LoadECDSA(keyPath)
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
	if err := s.reserveNonce(a, account.Nonce+1); err != nil {
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
	args = append(args, "-prepared", filepath.Join(dir, "prepared.json"), "-nodekey", keyPath,
		"-nonce", strconv.FormatUint(uint64(account.Nonce+1), 10), "-tx-epoch", strconv.FormatUint(uint64(chainEpoch.Epoch), 10),
		"-max-fee", a.MaximumFee, "-signed", filepath.Join(dir, "signed.json"), "-broadcast")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	output, err := s.runCommand(ctx, args, signingPassphrase)
	if err != nil {
		return standaloneAction{}, fmt.Errorf("signing or broadcast needs reconciliation: %s", strings.TrimSpace(string(output)))
	}
	return s.readAction(a.ID)
}

func (s *standaloneServer) reconcile(id string) (standaloneAction, error) {
	a, err := s.readAction(id)
	if err != nil {
		return standaloneAction{}, err
	}
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
	signer, err := directAddress(signed.Sender)
	if err != nil || signer != a.Signer || signed.AccountNonce == 0 {
		return standaloneAction{}, errors.New("saved transaction signer or nonce is invalid")
	}
	signed.Outcome = recordOutcome(s.client, signed.TxHash)
	if err := writePrivateJSON(path, &signed); err != nil {
		return standaloneAction{}, err
	}
	if err := s.reconcileNonceReservation(a, signed); err != nil {
		return standaloneAction{}, err
	}
	return s.readAction(id)
}

func (s *standaloneServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/mode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method refused", 405)
			return
		}
		writeStandaloneJSON(w, struct {
			Preview   bool `json:"preview"`
			BuyerOnly bool `json:"buyerOnly"`
			Version   int  `json:"version"`
		}{Preview: false, BuyerOnly: s.cfg.BuyerOnly, Version: s.cfg.Version})
	})
	assets := map[string]string{"/": "standalone_app.html", "/app.js": "standalone_app.js", "/app.css": "standalone_app.css"}
	if s.cfg.Version == 8 || s.cfg.Version == 9 {
		assets = v8Assets()
		s.v8Routes(mux)
	}
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
			} else if strings.HasSuffix(route, ".js") || strings.HasSuffix(route, ".mjs") {
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			} else {
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
			}
			_, _ = w.Write(raw)
		})
	}
	mux.HandleFunc("/api/pair", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method refused", 405)
			return
		}
		var request struct {
			Code string `json:"code"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 256)
		decodeErr := json.NewDecoder(r.Body).Decode(&request)
		s.mu.Lock()
		defer s.mu.Unlock()
		valid := decodeErr == nil && s.pairCode != "" && subtle.ConstantTimeCompare([]byte(request.Code), []byte(s.pairCode)) == 1
		if !valid {
			now := time.Now()
			if now.Before(s.pairRetryAfter) {
				seconds := int(time.Until(s.pairRetryAfter).Seconds()) + 1
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				http.Error(w, "too many incorrect codes; try again shortly", http.StatusTooManyRequests)
				return
			}
			if !s.pairRetryAfter.IsZero() {
				s.pairFailed = 0
				s.pairRetryAfter = time.Time{}
			}
			s.pairFailed++
			if s.pairFailed >= 5 {
				s.pairRetryAfter = now.Add(30 * time.Second)
			}
			http.Error(w, "pairing refused", http.StatusForbidden)
			return
		}
		// Possession of the random owner code can recover pairing during cooldown.
		s.pairFailed = 0
		s.pairRetryAfter = time.Time{}
		writeStandaloneJSON(w, struct {
			Token string `json:"token"`
		}{Token: s.token})
	})
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
		if s.cfg.Version == 8 || s.cfg.Version == 9 {
			http.NotFound(w, r)
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
		if s.cfg.Version == 8 || s.cfg.Version == 9 {
			http.NotFound(w, r)
			return
		}
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
		if s.cfg.Version == 8 || s.cfg.Version == 9 {
			http.NotFound(w, r)
			return
		}
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
		policy := "default-src 'none'; script-src 'self'; style-src 'self'; img-src data:; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
		if s.cfg.Version == 8 || s.cfg.Version == 9 {
			policy += "; frame-src https://idena.social"
		}
		w.Header().Set("Content-Security-Policy", policy)
		if s.origin == "" || r.Host != strings.TrimPrefix(s.origin, "http://") {
			http.Error(w, "host refused", 403)
			return
		}
		if origin := r.Header.Get("Origin"); (origin != "" && origin != s.origin) || (r.Method == http.MethodPost && origin != s.origin) {
			http.Error(w, "origin refused", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/pair" && r.URL.Path != "/api/mode" &&
			(s.token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-FOD-Token")), []byte(s.token)) != 1) {
			http.Error(w, "local request token refused", 403)
			return
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
	fmt.Println("Enter this pairing code in the browser:", s.pairCode)
	if cfg.Version == 8 || cfg.Version == 9 {
		if cfg.BuyerOnly {
			fmt.Println("The page uses your local buyer wallet and an HTTPS relay. Keep this app open for delivery.")
		} else if cfg.SellerClient {
			fmt.Println("The page uses your local seller identity key and an HTTPS relay. Keep this app open for delivery.")
		} else {
			fmt.Println("The page uses your own node. Trading requires your explicit review or a bounded automatic-trading permission. Keep this app open for delivery.")
		}
	} else {
		fmt.Println("The page uses your own node. Keep this terminal open; no transaction is sent without the review and submit step.")
	}
	if !cfg.NoBrowser {
		if err := openStandaloneBrowser(s.origin); err != nil {
			fmt.Println("Open the local app URL above in your browser.")
		}
	}
	server := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	if cfg.Version == 8 || cfg.Version == 9 {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		if !cfg.BuyerOnly && !cfg.SellerClient {
			s.startV8Capture(ctx)
		}
		defer func() {
			cancel()
			if s.v8.CaptureDone != nil {
				select {
				case <-s.v8.CaptureDone:
				case <-time.After(10 * time.Second):
				}
			}
		}()
		go s.v8Background(ctx)
		go func() { <-ctx.Done(); _ = server.Close() }()
	}
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
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
