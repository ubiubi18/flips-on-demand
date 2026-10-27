package main

// A local browser client for the same v8 protocol used by the native client.
// The HTTP boundary never accepts private file paths or caller identities.
import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

type standaloneV8Grant struct {
	Identity                  string
	Until                     time.Time
	Passphrase                string
	Fees, PerTrade, Purchases *big.Int
}
type standaloneV8Runtime struct {
	Grant       *standaloneV8Grant
	Message     string
	PauseReason string
	Capture     string
	PoolSales   map[string]bool
	RetryAfter  time.Time
	CaptureDone chan struct{}
}
type standaloneV8Status struct {
	Market         *bondedV8Snapshot   `json:"market"`
	Epoch          uint16              `json:"epoch"`
	Wallet         string              `json:"wallet"`
	Packages       []standalonePackage `json:"packages"`
	CanSign        bool                `json:"canSign"`
	Automatic      bool                `json:"automatic"`
	Message        string              `json:"message"`
	PauseReason    string              `json:"pauseReason,omitempty"`
	Capture        string              `json:"capture"`
	FeeBudget      string              `json:"feeBudget,omitempty"`
	PurchaseBudget string              `json:"purchaseBudget,omitempty"`
	BuyerOnly      bool                `json:"buyerOnly"`
	SellerClient   bool                `json:"sellerClient,omitempty"`
}

func v8Assets() map[string]string {
	return map[string]string{"/": "standalone_v8.html", "/app.js": "standalone_v8.js", "/app.css": "standalone_v8.css", "/model.mjs": "standalone_v8_model.mjs"}
}
func runStandaloneV8(args []string) error { return runStandaloneBonded(args, 8) }
func runStandaloneBonded(args []string, version int) error {
	f := flag.NewFlagSet("app-v"+strconv.Itoa(version), flag.ContinueOnError)
	base, e := os.UserConfigDir()
	if e != nil {
		return e
	}
	buyerOnly := f.Bool("buyer-only", false, "v9 buyer wallet through an HTTPS relay")
	sellerClient := f.Bool("seller-client", false, "v9 seller identity key through an HTTPS relay")
	profile := f.String("profile", "", "owner-only saved connection")
	preview := f.Bool("read-only-preview", false, "synthetic UI without a node or transaction endpoints")
	noBrowser := f.Bool("no-browser", false, "print the local URL without launching a browser")
	rpc := f.String("rpc", "", "loopback RPC URL")
	relay := f.String("relay", "", "HTTPS market relay ending in /rpc")
	key := f.String("key-file", "", "owner-only node RPC key file")
	walletKey := f.String("wallet-key", "", "owner-only buyer wallet key file")
	contract := f.String("contract", "", "verified v8 contract")
	code := f.String("code-hash", "", "verified deployed code hash")
	nodekey := f.String("nodekey", "", "owner-only identity key")
	readOnly := f.Bool("read-only", false, "disable signing, including a key saved in the profile")
	data := f.String("app-data-dir", "", "owner-only local market folder")
	archive := f.String("archive-dir", "", "existing session archive, read only")
	nodeData := f.String("node-dir", "", "optional native node data directory for local validation capture")
	if e = f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return errors.New("use named app-v8 options")
	}
	if *buyerOnly && *sellerClient {
		return errors.New("choose one remote connection mode")
	}
	if (*buyerOnly || *sellerClient) && version != 9 || !*buyerOnly && !*sellerClient && (*relay != "" || *walletKey != "") {
		return errors.New("relay connections are only available with app-v9")
	}
	if *buyerOnly && (*rpc != "" || *key != "" || *nodekey != "" || *nodeData != "" || *archive != "" || *readOnly) {
		return errors.New("buyer-only mode accepts a relay and wallet key, not local node or seller options")
	}
	if *sellerClient && (*rpc != "" || *key != "" || *walletKey != "" || *nodeData != "" || *readOnly) {
		return errors.New("seller-client mode accepts a relay and local identity key, not node RPC options")
	}
	if *profile == "" {
		folder := "Flips on Demand v" + strconv.Itoa(version)
		if *buyerOnly {
			folder += " buyer"
		} else if *sellerClient {
			folder += " seller"
		}
		*profile = filepath.Join(base, folder, "connection.json")
	}
	if *preview {
		return runStandaloneBondedPreview(*noBrowser, version)
	}
	cfg := standaloneConfig{Version: version, BuyerOnly: *buyerOnly, SellerClient: *sellerClient}
	if _, e = os.Lstat(*profile); e == nil {
		raw, e := readBondedV8Private(*profile, 16384)
		if e != nil {
			return e
		}
		if e = decodeStrict(raw, &cfg); e != nil {
			return e
		}
		if cfg.BuyerOnly != *buyerOnly || cfg.SellerClient != *sellerClient {
			return errors.New("saved profile belongs to a different connection mode")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	for _, v := range []struct {
		dst   *string
		value string
	}{{&cfg.RPC, *rpc}, {&cfg.KeyFile, *key}, {&cfg.Contract, *contract}, {&cfg.CodeHash, *code}, {&cfg.NodeKey, *nodekey}, {&cfg.DataDir, *data}, {&cfg.ArchiveDir, *archive}, {&cfg.NodeDataDir, *nodeData}} {
		if v.value != "" {
			*v.dst = v.value
		}
	}
	if *relay != "" {
		cfg.RPC = *relay
	}
	if *walletKey != "" {
		cfg.WalletKey = *walletKey
	}
	if cfg.Version != version {
		return errors.New("profile belongs to a different market version")
	}
	cfg.Version = version
	cfg.NoBrowser = *noBrowser
	if *readOnly {
		cfg.NodeKey = ""
	}
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Dir(*profile)
	}
	if cfg.RPC == "" || (!cfg.BuyerOnly && !cfg.SellerClient && cfg.KeyFile == "") ||
		(cfg.BuyerOnly && cfg.WalletKey == "") || (cfg.SellerClient && cfg.NodeKey == "") || cfg.Contract == "" || cfg.CodeHash == "" {
		if cfg.BuyerOnly {
			return errors.New("first buyer connection needs -relay, -wallet-key, -contract and -code-hash; these are remembered in the owner-only buyer profile")
		}
		if cfg.SellerClient {
			return errors.New("first seller connection needs -relay, -nodekey, -contract and -code-hash; these are remembered in the owner-only seller profile")
		}
		return errors.New("first connection needs -rpc, -key-file, -contract and -code-hash; these are remembered in the owner-only profile. Use -read-only-preview to explore without a node")
	}
	cfg.Contract = strings.TrimPrefix(strings.ToLower(cfg.Contract), "0x")
	cfg.CodeHash = strings.TrimPrefix(strings.ToLower(cfg.CodeHash), "0x")
	if !filepath.IsAbs(*profile) || !filepath.IsAbs(cfg.DataDir) {
		return errors.New("profile and app data paths must be absolute")
	}
	// Pin the deployed code before remembering the connection. An own node may
	// still be syncing; market reads and trading remain blocked until it catches up.
	probe, e := newStandaloneServer(cfg)
	if e != nil {
		return e
	}
	if _, e = readBondedV8Snapshot(probe.client, cfg.Contract, cfg.CodeHash); e != nil &&
		!(!cfg.BuyerOnly && !cfg.SellerClient && errors.Is(e, errNodeNotSynced)) {
		return e
	}
	if errors.Is(e, errNodeNotSynced) {
		fmt.Println("Node is still syncing. The local app will open and retry market reads automatically.")
	}
	if e = standalonePrivateDir(filepath.Dir(*profile)); e != nil {
		return e
	}
	if e = writePrivateJSON(*profile, cfg); e != nil {
		return e
	}
	return runStandaloneApp(cfg)
}
func (s *standaloneServer) v8ReadMarket() (*bondedV8Snapshot, error) {
	return readBondedV8Snapshot(s.client, s.cfg.Contract, s.cfg.CodeHash)
}
func (s *standaloneServer) v8Status() (*standaloneV8Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, e := s.v8ReadMarket()
	if e != nil {
		return nil, e
	}
	var epoch struct {
		Epoch uint16 `json:"epoch"`
	}
	if e = s.client.call("dna_epoch", []any{}, &epoch); e != nil {
		return nil, e
	}
	var wallet struct {
		Balance string `json:"balance"`
	}
	if e = s.client.call("dna_getBalance", []any{"0x" + m.Identity}, &wallet); e != nil {
		return nil, e
	}
	packages := []standalonePackage{}
	if !s.cfg.BuyerOnly {
		packages, e = s.v8Packages(m, epoch.Epoch)
		if e != nil {
			return nil, e
		}
	}
	out := &standaloneV8Status{Market: m, Epoch: epoch.Epoch, Wallet: wallet.Balance, Packages: packages, CanSign: s.cfg.NodeKey != "" || s.cfg.BuyerOnly, Automatic: s.v8GrantValid(m.Identity), Message: s.v8.Message, PauseReason: s.v8.PauseReason, Capture: s.v8.Capture, BuyerOnly: s.cfg.BuyerOnly, SellerClient: s.cfg.SellerClient}
	if out.Automatic {
		out.FeeBudget = s.v8.Grant.Fees.String()
		out.PurchaseBudget = s.v8.Grant.Purchases.String()
	}
	return out, nil
}
func (s *standaloneServer) v8CheckPending() error {
	entries, e := os.ReadDir(filepath.Join(s.cfg.DataDir, "actions"))
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if !entry.IsDir() || !standaloneID.MatchString(entry.Name()) {
			continue
		}
		if _, e := os.Lstat(filepath.Join(s.cfg.DataDir, "actions", entry.Name(), "attempted")); os.IsNotExist(e) {
			continue
		}
		a, e := s.readAction(entry.Name())
		if e != nil || !a.Attempted || a.Submission == nil {
			return errors.New("reconcile the previous transaction in Activity before starting another")
		}
		if a.Submission.Included {
			if !a.Submission.Success {
				return errors.New("previous transaction failed on chain; review Activity before resuming")
			}
			continue
		}
		a, e = s.reconcile(a.ID)
		if e != nil {
			return fmt.Errorf("reconcile the previous transaction in Activity: %w", e)
		}
		if a.Submission == nil || !a.Submission.Included {
			return errV8ReceiptPending
		}
		if !a.Submission.Success {
			return errors.New("previous transaction failed on chain; review Activity before resuming")
		}
		return errV8ReceiptResolved // Read a fresh market snapshot before the next action.
	}
	return nil
}

var (
	errV8ReceiptPending  = errors.New("transaction sent; waiting for a block receipt")
	errV8ReceiptResolved = errors.New("transaction included; refreshing chain state")
)

func (s *standaloneServer) v8GrantValid(who string) bool {
	g := s.v8.Grant
	if g != nil && !time.Now().Before(g.Until) {
		s.v8.Grant = nil
		s.v8.PauseReason = "The eight-hour trading permission expired."
		return false
	}
	return g != nil && g.Identity == who && time.Now().Before(g.Until) && g.Fees.Sign() > 0
}
func v8Amount(s string) (*big.Int, error) {
	if _, e := v8number(s, 16); e != nil {
		return nil, e
	}
	n, _ := new(big.Int).SetString(s, 10)
	return n, nil
}
func (s *standaloneServer) v8PrepareHTTP(r *http.Request) (standaloneAction, error) {
	if e := r.ParseMultipartForm(standaloneUploadLimit); e != nil {
		return standaloneAction{}, e
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	m, e := s.v8ReadMarket()
	if e != nil {
		return standaloneAction{}, e
	}
	req := bondedV8Request{Operation: r.FormValue("operation"), Caller: m.Identity, Amount: r.FormValue("amount"), Target: r.FormValue("target"), Minimum: r.FormValue("minimum"), Maximum: r.FormValue("maximum"), Capacity: r.FormValue("capacity"), Bundle: r.FormValue("bundle"), ID: r.FormValue("id"), Exit: r.FormValue("exit") == "true"}
	return s.v8Prepare(r.Context(), m, req, r.FormValue("passphrase"), r)
}
func (s *standaloneServer) v8CLI(ctx context.Context, command string, req *bondedV8Request, out, pass, fee string) error {
	input := out + ".request.json"
	if e := writePrivateJSON(input, req); e != nil {
		return e
	}
	req.ProtocolVersion = s.cfg.Version
	args := []string{"v" + strconv.Itoa(s.cfg.Version), "-command", command, "-rpc", s.cfg.RPC, "-contract", s.cfg.Contract, "-code-hash", s.cfg.CodeHash, "-request", input, "-out", out}
	if s.cfg.BuyerOnly {
		args = append(args, "-buyer-only", "-wallet-key", s.cfg.WalletKey)
	} else if s.cfg.SellerClient {
		args = append(args, "-seller-client", "-nodekey", s.cfg.NodeKey)
	} else {
		args = append(args, "-key-file", s.cfg.KeyFile)
	}
	if fee != "" {
		args = append(args, "-max-fee", fee)
	}
	_, e := s.runCommand(ctx, args, pass)
	if e != nil {
		return fmt.Errorf("local %s check failed; no new action was authorized", command)
	}
	return nil
}
func (s *standaloneServer) v8Prepare(ctx context.Context, m *bondedV8Snapshot, req bondedV8Request, pass string, upload *http.Request) (standaloneAction, error) {
	zero := standaloneAction{}
	req.ProtocolVersion = s.cfg.Version
	if s.cfg.BuyerOnly && !buyerBondedOperation(req.Operation) {
		return zero, errors.New("buyer-only mode refuses seller or unsupported operations")
	}
	if s.cfg.SellerClient && !sellerClientBondedOperation(req.Operation) {
		return zero, errors.New("seller-client mode refuses unsupported operations")
	}
	if s.cfg.Version == 9 && req.Operation == "registerBundle" {
		return zero, errors.New("list the package directly; no proof registration is needed")
	}
	if e := s.v8CheckPending(); e != nil {
		return zero, e
	}
	if req.Caller != m.Identity {
		return zero, errors.New("identity changed")
	}
	allowed := false
	for _, op := range bondedV8Methods {
		if req.Operation == op {
			allowed = true
		}
	}
	if !allowed {
		return zero, errors.New("unsupported v8 operation")
	}
	if req.Operation == "withdrawStake" && (!req.Exit || req.Amount != m.Account.Balance || m.Account.Reserved != "0") {
		return zero, errors.New("close all orders and trades before withdrawing the full deposit")
	}
	id, e := randomStandaloneID()
	if e != nil {
		return zero, e
	}
	dir, _ := s.actionDir(id)
	if e = standalonePrivateDir(dir); e != nil {
		return zero, e
	}
	if req.Operation == "queueBuy" || req.Operation == "queueSell" {
		req.Expires = m.Height + 4320
	}
	if req.Operation == "queueBuy" {
		key, e := crypto.GenerateKey()
		if e != nil {
			return zero, e
		}
		req.DeliveryKey = hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey))
		req.DeliveryKeyFile = filepath.Join(dir, "delivery.key")
		if e = writeNewPoolV7PrivateFile(req.DeliveryKeyFile, []byte(hex.EncodeToString(crypto.FromECDSA(key)))); e != nil {
			return zero, e
		}
	}
	if req.Operation == "queueSell" {
		found := false
		for _, b := range m.Bundles {
			if b.ID == req.Bundle && b.Author == m.Identity && !b.Used && b.Sale == "0" {
				found = true
			}
		}
		if !found && s.cfg.Version != 9 {
			return zero, errors.New("select your registered, unused package")
		}
		if e = s.v8CheckSellerPackage(m, req.Bundle, pass); e != nil {
			return zero, e
		}
	}
	if req.Operation == "queueSell" && s.cfg.Version == 9 {
		pkgDir, e := s.packageDir(req.Bundle)
		if e != nil {
			return zero, e
		}
		req.SessionFile = filepath.Join(pkgDir, "session.json")
		req.VaultFile = filepath.Join(pkgDir, "keys.vault")
		if e = fillBondedV9Listing(s.client, s.cfg.Contract, &req, pass); e != nil {
			return zero, e
		}
	}
	if req.Operation == "registerBundle" || req.Operation == "proveDelivery" {
		if upload == nil {
			return zero, errors.New("select the completed proof first")
		}
		proof, e := standaloneSaveUpload(upload, "proof", dir)
		if e != nil || proof == "" {
			return zero, errors.New("a completed proof file is required")
		}
		raw, e := readBondedV8Private(proof, 8<<20)
		if e != nil {
			return zero, e
		}
		var p struct {
			Journal string `json:"journal"`
			Seal    string `json:"seal"`
		}
		if e = json.Unmarshal(raw, &p); e != nil {
			return zero, e
		}
		req.Journal = p.Journal
		req.Seal = p.Seal
		if s.cfg.Version == 9 {
			contentFile, e := standaloneSaveUpload(upload, "contentProof", dir)
			if e != nil || contentFile == "" {
				return zero, errors.New("select the original-content proof for this dispute")
			}
			raw, e := readBondedV8Private(contentFile, 8<<20)
			if e != nil {
				return zero, e
			}
			if e = json.Unmarshal(raw, &p); e != nil {
				return zero, e
			}
			req.ContentJournal = p.Journal
			req.ContentSeal = p.Seal
		}
		if req.Operation == "registerBundle" {
			session, e := standaloneSaveUpload(upload, "session", dir)
			if e != nil || session == "" {
				return zero, errors.New("select the canonical session bundle")
			}
			req.SessionFile = session
			converted := filepath.Join(dir, "registration.json")
			if e = s.v8CLI(ctx, "registration", &req, converted, "", ""); e != nil {
				return zero, e
			}
			raw, e := readBondedV8Private(converted, 2<<20)
			if e != nil {
				return zero, e
			}
			if e = decodeStrict(raw, &req); e != nil {
				return zero, e
			}
		}
	}
	if req.Operation == "ready" || req.Operation == "accept" || req.Operation == "deliver" {
		if e = s.v8TradeTerms(ctx, m, &req, pass, upload); e != nil {
			return zero, e
		}
	}
	if _, _, e = encodeBondedV8(&req); e != nil {
		return zero, e
	}
	rate, e := readFeePerGas(s.client)
	if e != nil {
		return zero, e
	}
	fee := maxMaxFeeFor(rate)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	prepared := filepath.Join(dir, "prepared.json")
	if e = s.v8CLI(ctx, "prepare", &req, prepared, pass, fee.String()); e != nil {
		return zero, e
	}
	p, e := readPreparedAction(prepared)
	if e != nil {
		return zero, e
	}
	cap, e := v8Amount(p.EstimatedMaxFee)
	if e != nil || cap.Sign() <= 0 || cap.Cmp(fee) > 0 {
		return zero, errors.New("valid simulated fee unavailable")
	}
	if e = writeNewPoolV7PrivateFile(filepath.Join(dir, "maximum-fee"), []byte(cap.String())); e != nil {
		return zero, e
	}
	if e = writeNewPoolV7PrivateFile(filepath.Join(dir, "signer"), []byte(m.Identity)); e != nil {
		return zero, e
	}
	if e = standaloneSyncDir(dir); e != nil {
		return zero, e
	}
	return s.readAction(id)
}
func (s *standaloneServer) v8BuyerKey(m *bondedV8Snapshot, t *bondedV8Trade) (string, error) {
	entries, e := os.ReadDir(filepath.Join(s.cfg.DataDir, "actions"))
	if e != nil {
		return "", e
	}
	for _, ent := range entries {
		if !ent.IsDir() || !standaloneID.MatchString(ent.Name()) {
			continue
		}
		dir, _ := s.actionDir(ent.Name())
		p, e := readPreparedAction(filepath.Join(dir, "prepared.json"))
		if e == nil && p.Contract == s.cfg.Contract && p.CodeHash == s.cfg.CodeHash && p.BondedV8 != nil && p.BondedV8.Caller == m.Identity && p.Operation == "queueBuy" && p.BondedV8.DeliveryKey == t.DeliveryKey {
			return p.BondedV8.DeliveryKeyFile, nil
		}
	}
	return "", errors.New("the private delivery key for this purchase is missing")
}
func (s *standaloneServer) v8TradeTerms(ctx context.Context, m *bondedV8Snapshot, req *bondedV8Request, pass string, upload *http.Request) error {
	var trade *bondedV8Trade
	for i := range m.Trades {
		if m.Trades[i].ID == req.ID {
			trade = &m.Trades[i]
		}
	}
	if trade == nil {
		return errors.New("trade unavailable")
	}
	dir := filepath.Join(s.cfg.DataDir, "private", "trade-"+trade.ID)
	if e := standalonePrivateDir(dir); e != nil {
		return e
	}
	if req.Operation == "ready" || req.Operation == "accept" {
		if trade.Buyer != m.Identity {
			return errors.New("not your purchase")
		}
		req.RetainedFile = filepath.Join(dir, "retained.json")
		if _, e := os.Lstat(req.RetainedFile); os.IsNotExist(e) {
			if e = s.v8CLI(ctx, "retain", req, req.RetainedFile, "", ""); e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
		key, e := s.v8BuyerKey(m, trade)
		if e != nil {
			return e
		}
		req.DeliveryKeyFile = key
		if req.Operation == "accept" {
			if len(pass) < 12 {
				return errors.New("unlock with a vault passphrase of at least 12 characters")
			}
			req.CiphertextHash = trade.CiphertextHash
			if e = s.v8CLI(ctx, "receive", req, filepath.Join(dir, "received.vault"), pass, ""); e != nil {
				return e
			}
		}
		return nil
	}
	if trade.Seller != m.Identity {
		return errors.New("not your sale")
	}
	envelope := filepath.Join(dir, "delivery.json")
	if _, e := os.Lstat(envelope); os.IsNotExist(e) {
		vault := filepath.Join(s.cfg.DataDir, "packages", trade.Bundle, "keys.vault")
		if _, e = os.Lstat(vault); os.IsNotExist(e) && upload != nil {
			vault, e = standaloneSaveUpload(upload, "vault", dir)
			if e != nil {
				return e
			}
		}
		req.VaultFile = vault
		if len(pass) < 12 {
			return errors.New("unlock the seller vault first")
		}
		if e = s.v8CLI(ctx, "delivery", req, envelope, pass, ""); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	raw, e := readBondedV8Private(envelope, 1<<20)
	if e != nil {
		return e
	}
	var delivery bondedV8Request
	if e = decodeStrict(raw, &delivery); e != nil {
		return e
	}
	if delivery.Caller != m.Identity || delivery.ID != req.ID || delivery.Operation != "deliver" {
		return errors.New("saved delivery context differs")
	}
	req.Ciphertext = delivery.Ciphertext
	return nil
}
func (s *standaloneServer) v8Execute(ctx context.Context, m *bondedV8Snapshot, req bondedV8Request) (standaloneAction, error) {
	if !s.v8GrantValid(m.Identity) {
		return standaloneAction{}, errors.New("enable local automatic trading first")
	}
	if s.cfg.BuyerOnly && !map[string]bool{"queueBuy": true, "matchNext": true, "ready": true, "accept": true}[req.Operation] {
		return standaloneAction{}, errors.New("buyer-only automatic trading refuses this operation")
	}
	if s.cfg.SellerClient && !sellerClientBondedOperation(req.Operation) {
		return standaloneAction{}, errors.New("seller-client automatic trading refuses this operation")
	}
	g := s.v8.Grant
	if !map[string]bool{"queueBuy": true, "queueSell": true, "matchNext": true, "acquire": true, "ready": true, "deliver": true, "accept": true}[req.Operation] {
		return standaloneAction{}, errors.New("operation requires a separate transaction review")
	}
	if m.Account.NeedsTopup && (req.Operation == "queueBuy" || req.Operation == "queueSell") {
		return standaloneAction{}, errors.New("refill security deposit before opening new trades")
	}
	if e := s.v8CheckPending(); e != nil {
		if !errors.Is(e, errV8ReceiptPending) && !errors.Is(e, errV8ReceiptResolved) {
			s.v8.Grant = nil
			s.v8.PauseReason = "An earlier transaction needs review in Activity."
		}
		return standaloneAction{}, e
	}
	if req.Operation == "queueBuy" {
		n, e := v8Amount(req.Maximum)
		if e != nil || n.Cmp(g.PerTrade) > 0 || n.Cmp(g.Purchases) > 0 {
			return standaloneAction{}, errors.New("purchase exceeds your approved budget")
		}
	}
	if req.Operation == "queueSell" {
		n, e := v8Amount(req.Capacity)
		if e != nil || n.Cmp(g.PerTrade) > 0 {
			return standaloneAction{}, errors.New("sale exceeds your approved trade limit")
		}
	}
	a, e := s.v8Prepare(ctx, m, req, g.Passphrase, nil)
	if e != nil {
		return a, e
	}
	fee, e := v8Amount(a.MaximumFee)
	if e != nil || fee.Cmp(g.Fees) > 0 {
		return a, errors.New("remaining fee budget is too small; review the prepared action")
	}
	// Consume limits before any ambiguous attempt, never restore on a timeout.
	g.Fees.Sub(g.Fees, fee)
	if req.Operation == "queueBuy" {
		n, _ := v8Amount(req.Maximum)
		g.Purchases.Sub(g.Purchases, n)
	}
	body, _ := json.Marshal(map[string]string{"id": a.ID, "reviewHash": a.ReviewHash, "passphrase": g.Passphrase})
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.origin+"/api/submit", bytes.NewReader(body))
	a, e = s.submit(r)
	if e != nil {
		s.v8.Grant = nil
		s.v8.PauseReason = "The signing or broadcast result is uncertain. Review Activity."
		return a, e
	}
	a, e = s.reconcile(a.ID)
	if e != nil || a.Submission == nil {
		s.v8.Grant = nil
		s.v8.PauseReason = "Receipt or nonce reconciliation failed. Review Activity."
		return a, errors.New("transaction needs reconciliation; automatic actions paused")
	}
	if !a.Submission.Included {
		return a, nil // The send was acknowledged; background checks inclusion later.
	}
	if !a.Submission.Success {
		s.v8.Grant = nil
		s.v8.PauseReason = "A transaction failed on chain. Review Activity."
		return a, errors.New("transaction failed on chain; automatic actions paused")
	}
	return a, nil
}
func v8JSON(r *http.Request, v any) error {
	raw, e := io.ReadAll(io.LimitReader(r.Body, 16385))
	if e != nil || len(raw) > 16384 {
		return errors.New("request too large")
	}
	return decodeStrict(raw, v)
}
func (s *standaloneServer) v8Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v8/download", s.v8Download)
	route := func(path string, fn func(*http.Request) (any, error)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method refused", 405)
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			v, e := fn(r)
			if e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
			writeStandaloneJSON(w, v)
		})
	}
	route("/api/v8/collect", func(r *http.Request) (any, error) {
		if s.cfg.BuyerOnly {
			return nil, errors.New("package collection is unavailable in buyer-only mode")
		}
		m, e := s.v8ReadMarket()
		if e != nil {
			return nil, e
		}
		return s.v8Collect(m)
	})
	route("/api/v8/protect", func(r *http.Request) (any, error) {
		if s.cfg.BuyerOnly {
			return nil, errors.New("seller protection is unavailable in buyer-only mode")
		}
		return s.v8Protect(r)
	})
	route("/api/v8/purchased-preview", s.v8PurchasedPreview)
	route("/api/v8/import", func(r *http.Request) (any, error) {
		if s.cfg.BuyerOnly {
			return nil, errors.New("seller import is unavailable in buyer-only mode")
		}
		return s.v8Import(r)
	})
	route("/api/v8/unlock", func(r *http.Request) (any, error) {
		var in struct {
			Passphrase     string `json:"passphrase"`
			PerTrade       string `json:"perTrade"`
			PurchaseBudget string `json:"purchaseBudget"`
			FeeBudget      string `json:"feeBudget"`
		}
		if e := v8JSON(r, &in); e != nil {
			return nil, e
		}
		if s.cfg.NodeKey == "" && !s.cfg.BuyerOnly || len(in.Passphrase) < 12 || len(in.Passphrase) > 1024 {
			return nil, errors.New("signing configuration and a vault passphrase of at least 12 characters are required")
		}
		m, e := s.v8ReadMarket()
		if e != nil {
			return nil, e
		}
		keyPath := s.cfg.NodeKey
		if s.cfg.BuyerOnly {
			if address, err := buyerWalletAddress(s.cfg.WalletKey); err != nil || address != m.Identity {
				return nil, errors.New("buyer wallet key changed or is no longer owner-only")
			}
			keyPath = s.cfg.WalletKey
		} else if s.cfg.SellerClient {
			if address, err := sellerIdentityAddress(s.cfg.NodeKey); err != nil || address != m.Identity {
				return nil, errors.New("seller identity key changed or is no longer owner-only")
			}
		}
		key, e := crypto.LoadECDSA(keyPath)
		if e != nil {
			return nil, errors.New("signing key unavailable")
		}
		who, e := nodekeyAddressOf(key)
		if e != nil || who != m.Identity {
			return nil, errors.New("signing key does not control this identity")
		}
		per, e := v8Amount(in.PerTrade)
		if e != nil || per.Sign() <= 0 {
			return nil, errors.New("invalid trade limit")
		}
		purchases, e := v8Amount(in.PurchaseBudget)
		if e != nil {
			return nil, e
		}
		fees, e := v8Amount(in.FeeBudget)
		if e != nil || fees.Sign() <= 0 || fees.Cmp(new(big.Int).Mul(big.NewInt(25), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))) > 0 {
			return nil, errors.New("fee budget must be greater than zero and no more than 25 iDNA")
		}
		if m.Account.Balance == "0" {
			return nil, errors.New("set up your deposit first")
		}
		s.v8.Grant = &standaloneV8Grant{m.Identity, time.Now().Add(8 * time.Hour), in.Passphrase, fees, per, purchases}
		s.v8.PauseReason = ""
		return map[string]bool{"automatic": true}, nil
	})
	route("/api/v8/lock", func(r *http.Request) (any, error) {
		s.v8.Grant = nil
		s.v8.PauseReason = "You paused one-click trading."
		return map[string]bool{"automatic": false}, nil
	})
	route("/api/v8/order", func(r *http.Request) (any, error) {
		var in struct {
			Side     string `json:"side"`
			Bundle   string `json:"bundle"`
			Maximum  string `json:"maximum"`
			Minimum  string `json:"minimum"`
			Capacity string `json:"capacity"`
		}
		if e := v8JSON(r, &in); e != nil {
			return nil, e
		}
		if s.cfg.BuyerOnly && in.Side != "buy" {
			return nil, errors.New("buyer-only mode accepts only buy orders")
		}
		m, e := s.v8ReadMarket()
		if e != nil {
			return nil, e
		}
		req := bondedV8Request{Caller: m.Identity}
		if in.Side == "buy" {
			req.Operation = "queueBuy"
			req.Maximum = in.Maximum
		} else if in.Side == "sell" || in.Side == "pool" {
			req.Operation = "queueSell"
			req.Bundle = in.Bundle
			req.Minimum = in.Minimum
			req.Capacity = in.Capacity
		} else {
			return nil, errors.New("choose buy or sell")
		}
		a, e := s.v8Execute(r.Context(), m, req)
		if e == nil && in.Side == "pool" {
			if s.v8.PoolSales == nil {
				s.v8.PoolSales = map[string]bool{}
			}
			s.v8.PoolSales[req.Bundle] = true
		}
		return a, e
	})
}

func (s *standaloneServer) v8PurchasedPreview(r *http.Request) (any, error) {
	var in struct {
		ID string `json:"id"`
	}
	if e := v8JSON(r, &in); e != nil {
		return nil, e
	}
	if _, e := v8number(in.ID, 8); e != nil {
		return nil, errors.New("invalid trade")
	}
	m, e := s.v8ReadMarket()
	if e != nil {
		return nil, e
	}
	var trade *bondedV8Trade
	for i := range m.Trades {
		if m.Trades[i].ID == in.ID && m.Trades[i].Buyer == m.Identity && m.Trades[i].State == 5 {
			trade = &m.Trades[i]
		}
	}
	if trade == nil {
		return nil, errors.New("only your completed purchases can be previewed")
	}
	bundle, e := readBondedV8Bundle(s.client, s.cfg.Contract, trade.Bundle)
	if e != nil {
		return nil, e
	}
	key, e := s.v8BuyerKey(m, trade)
	if e != nil {
		return nil, e
	}
	req := &bondedV8Request{Operation: "preview", Caller: m.Identity,
		RetainedFile: filepath.Join(s.cfg.DataDir, "private", "trade-"+in.ID, "retained.json"), DeliveryKeyFile: key}
	preview, e := readBondedPreview(s.client, s.cfg.Contract, trade, bundle, req)
	if e != nil {
		return nil, e
	}
	owner, e := s.client.bondedIdentity()
	if e != nil || owner != m.Identity {
		return nil, errors.New("active identity changed; refresh before opening a purchase")
	}
	return preview, nil
}

func (s *standaloneServer) v8Download(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method refused", 405)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := r.URL.Query().Get("trade")
	if _, e := v8number(id, 8); e != nil {
		http.Error(w, "invalid trade", 400)
		return
	}
	m, e := s.v8ReadMarket()
	if e != nil {
		if s.cfg.BuyerOnly {
			http.Error(w, "relay unavailable", 503)
		} else {
			http.Error(w, "node unavailable", 503)
		}
		return
	}
	var trade *bondedV8Trade
	for i := range m.Trades {
		if m.Trades[i].ID == id && m.Trades[i].Buyer == m.Identity {
			trade = &m.Trades[i]
		}
	}
	if trade == nil {
		http.Error(w, "purchase does not belong to this account", 403)
		return
	}
	bundle, e := readBondedV8Bundle(s.client, s.cfg.Contract, trade.Bundle)
	if e != nil {
		http.Error(w, "bundle unavailable", 400)
		return
	}
	dir := filepath.Join(s.cfg.DataDir, "private", "trade-"+id)
	retained, e := readBondedV8Retained(s.cfg.Contract, trade, bundle, filepath.Join(dir, "retained.json"))
	if e != nil {
		http.Error(w, "verified encrypted content is missing locally", 400)
		return
	}
	vault, e := readBondedV8Private(filepath.Join(dir, "received.vault"), 1<<20)
	if e != nil {
		http.Error(w, "verify and save this delivery first", 400)
		return
	}
	// ZIP contains encrypted data only. The passphrase remains with the owner.
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="flip-package-`+id+`.zip"`)
	z := zip.NewWriter(w)
	defer z.Close()
	write := func(name string, data []byte) error {
		f, e := z.Create(name)
		if e != nil {
			return e
		}
		_, e = f.Write(data)
		return e
	}
	manifest, _ := json.MarshalIndent(map[string]any{"format": "fod:encrypted-package:v8", "author": bundle.Author, "epoch": bundle.Epoch, "cids": bundle.CIDs, "contract": s.cfg.Contract, "trade": id}, "", "  ")
	if write("manifest.json", manifest) != nil || write("keys.vault", vault) != nil {
		return
	}
	for _, blob := range retained.Availability.Blobs {
		raw, _ := hex.DecodeString(blob.Hex)
		if write("encrypted-flips/"+blob.CID, raw) != nil {
			return
		}
	}
}
func (s *standaloneServer) v8Background(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		m, e := s.v8ReadMarket()
		if e != nil {
			if s.cfg.BuyerOnly || s.cfg.SellerClient {
				s.v8.Message = "Market unavailable: check the relay."
			} else {
				s.v8.Message = "Market unavailable: check your local node."
			}
		} else if s.cfg.BuyerOnly {
			handled := false
			if s.v8GrantValid(m.Identity) {
				handled = s.v8Step(ctx, m)
			}
			if !handled {
				s.v8.Message = "Buyer wallet connected through the relay."
			}
		} else {
			// A remote evidence fetch can wait on the shared service. Handle
			// already matched trades before collection so their deadlines keep
			// priority when that service is slow.
			handled := false
			if s.v8GrantValid(m.Identity) {
				handled = s.v8Step(ctx, m)
			}
			if _, e = s.v8Collect(m); e != nil {
				if !handled {
					if s.cfg.SellerClient {
						s.v8.Message = "Collection waiting: check the relay and saved evidence."
					} else {
						s.v8.Message = "Collection waiting: check your local node and saved evidence."
					}
				}
			} else if !handled {
				s.v8.Message = "Encrypted flips checked locally."
			}
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *standaloneServer) v8Step(ctx context.Context, m *bondedV8Snapshot) bool {
	if time.Now().Before(s.v8.RetryAfter) {
		return true
	}
	if e := s.v8CheckPending(); e != nil {
		s.v8.Message = e.Error()
		if !errors.Is(e, errV8ReceiptPending) && !errors.Is(e, errV8ReceiptResolved) {
			s.v8.Grant = nil
			s.v8.PauseReason = "An earlier transaction needs review in Activity."
		}
		return true
	}
	req := bondedV8Request{Caller: m.Identity}
	for _, t := range m.Trades {
		if t.Buyer == m.Identity && t.State == 1 && m.Height < t.ReadyBy {
			req.Operation = "ready"
			req.ID = t.ID
			break
		}
		if t.Seller == m.Identity && t.State == 2 && m.Height < t.DeliverBy {
			req.Operation = "deliver"
			req.ID = t.ID
			break
		}
		if t.Buyer == m.Identity && (t.State == 3 || t.State == 4) && m.Height < t.ProveBy {
			req.Operation = "accept"
			req.ID = t.ID
			break
		}
	}
	if req.Operation == "" {
		price, _ := v8Amount(m.BuyQuote)
		quote, _ := v8Amount(m.SellQuote)
		matched := false
		for _, buy := range m.Buys {
			maximum, _ := v8Amount(buy.Maximum)
			if buy.State != 1 || buy.Expires <= m.Height || maximum.Cmp(price) < 0 {
				continue
			}
			eligible, e := s.v8AccountEligible(buy.Buyer)
			if e != nil {
				s.v8.Message = "Queue account check unavailable."
				return true
			}
			if !eligible {
				continue
			}
			for _, sell := range m.Sells {
				if (sell.State != 1 && sell.State != 2) || sell.Expires <= m.Height || sell.Seller == buy.Buyer {
					continue
				}
				minimum, _ := v8Amount(sell.Minimum)
				capacity, _ := v8Amount(sell.Capacity)
				proceeds := quote
				if sell.State == 2 {
					proceeds, _ = v8Amount(sell.Proceeds)
				}
				if minimum.Cmp(proceeds) > 0 || capacity.Cmp(price) < 0 {
					continue
				}
				eligible, e := s.v8AccountEligible(sell.Seller)
				if e != nil {
					s.v8.Message = "Queue account check unavailable."
					return true
				}
				if !eligible {
					continue
				}
				if sell.Seller == m.Identity || buy.Buyer == m.Identity {
					req.Operation = "matchNext"
				}
				matched = true
				break
			}
			if matched {
				break
			}
		}
	}
	// Only the owner's explicitly requested pool sale may acquire reserve.
	if req.Operation == "" && m.PoolCanAcquire {
		for _, sell := range m.Sells {
			if sell.State != 1 || sell.Expires <= m.Height {
				continue
			}
			minimum, _ := v8Amount(sell.Minimum)
			capacity, _ := v8Amount(sell.Capacity)
			quote, _ := v8Amount(m.SellQuote)
			if quote.Cmp(minimum) < 0 || quote.Cmp(capacity) > 0 {
				continue
			}
			eligible, e := s.v8AccountEligible(sell.Seller)
			if e != nil {
				s.v8.Message = "Queue account check unavailable."
				return true
			}
			if !eligible {
				continue
			}
			if sell.Seller == m.Identity && s.v8.PoolSales[sell.Bundle] {
				req.Operation = "acquire"
			}
			break
		}
	}
	if req.Operation != "" {
		if action, e := s.v8Execute(ctx, m, req); e != nil {
			s.v8.Message = e.Error()
			if !errors.Is(e, errV8ReceiptPending) && !errors.Is(e, errV8ReceiptResolved) {
				s.v8.RetryAfter = time.Now().Add(time.Minute)
			}
		} else if action.Submission != nil && !action.Submission.Included {
			s.v8.Message = "Transaction sent; waiting for a block receipt."
		} else {
			s.v8.Message = "Trade advanced after local verification."
		}
		return true
	}
	return false
}

func (s *standaloneServer) v8AccountEligible(who string) (bool, error) {
	address, e := v8hex(who, 20)
	if e != nil {
		return false, e
	}
	raw, e := v8map(s.client, s.cfg.Contract, bondedPrefix(s.client)+"account/", address)
	if e != nil {
		return false, e
	}
	if len(raw) != 49 || raw[48] > 1 {
		return false, errors.New("queue account unavailable")
	}
	balance := new(big.Int).SetBytes(raw[:16])
	return raw[48] == 0 && balance.Cmp(new(big.Int).SetBytes(raw[16:32])) >= 0 && balance.Cmp(new(big.Int).SetBytes(raw[32:48])) >= 0, nil
}
func (s *standaloneServer) startV8Capture(ctx context.Context) {
	if s.cfg.NodeDataDir == "" {
		s.mu.Lock()
		s.v8.Capture = "Existing archive only. Connect a node data folder to capture future validation evidence."
		s.mu.Unlock()
		return
	}
	// Own separate archive; an externally managed capture is only read.
	u, e := url.Parse(s.cfg.RPC)
	if e != nil {
		return
	}
	port := u.Port()
	if port == "" {
		return
	}
	root := filepath.Join(s.cfg.DataDir, "capture")
	if e = standalonePrivateDir(root); e != nil {
		return
	}
	script := filepath.Join(root, "session_indexer.py")
	raw, e := standaloneAssets.ReadFile("session_indexer.py")
	if e != nil {
		return
	}
	if e = os.WriteFile(script, raw, 0600); e != nil {
		return
	}
	bin, e := os.Executable()
	if e != nil {
		return
	}
	log, e := os.OpenFile(filepath.Join(root, "capture.log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if e != nil {
		return
	}
	cmd := exec.CommandContext(ctx, "python3", script, "--cli", bin, "--archive", root, "--node", "local:"+port+":"+s.cfg.NodeDataDir)
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		log.Close()
		s.mu.Lock()
		s.v8.Capture = "Capture could not start. Python 3 and access to the local node folder are required."
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	s.v8.Capture = "Capturing validation evidence locally while this app stays open."
	s.mu.Unlock()
	s.v8.CaptureDone = make(chan struct{})
	go func() {
		e := cmd.Wait()
		log.Close()
		close(s.v8.CaptureDone)
		s.mu.Lock()
		defer s.mu.Unlock()
		if e != nil {
			s.v8.Capture = "Local capture stopped. Check the capture log before validation."
		}
	}()
}
func runStandaloneV8Preview(noBrowser bool) error { return runStandaloneBondedPreview(noBrowser, 8) }
func runStandaloneBondedPreview(noBrowser bool, version int) error {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	defer listener.Close()
	origin := "http://" + listener.Addr().String()
	fmt.Printf("Standalone v%d preview: %s\n", version, origin)
	fmt.Println("Synthetic examples only; no node, wallet or transaction endpoint.")
	if !noBrowser {
		_ = openStandaloneBrowser(origin)
	}
	return (&http.Server{Handler: standaloneBondedPreviewHandler(origin, version), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}).Serve(listener)
}
func standaloneV8PreviewHandler(origin string) http.Handler {
	return standaloneBondedPreviewHandler(origin, 8)
}
func standaloneBondedPreviewHandler(origin string, version int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src data:; frame-src https://idena.social; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Host != strings.TrimPrefix(origin, "http://") || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) {
			http.Error(w, "origin refused", 403)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "preview cannot transact", 405)
			return
		}
		if r.URL.Path == "/api/mode" {
			writeStandaloneJSON(w, map[string]any{"preview": true, "version": version})
			return
		}
		file, ok := v8Assets()[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		raw, e := standaloneAssets.ReadFile(file)
		if e != nil {
			http.Error(w, "asset unavailable", 500)
			return
		}
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		} else if strings.HasSuffix(file, ".css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		}
		_, _ = w.Write(raw)
	})
}
