package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

type directCommandOptions struct {
	Mode            string
	DirectVersion   int
	Node            *client
	Contract        string
	CodeHash        string
	Operation       string
	OfferID         string
	MinimumPayout   string
	Network         string
	SessionPath     string
	VaultPath       string
	NodeKeyPath     string
	BuyerAddress    string
	DeliveryKeyPath string
	PurchasePath    string
	MessagePath     string
	DeliveryPath    string
	ProofPath       string
	ProofHostPath   string
	PreparedPath    string
	OutPath         string
	OutExplicit     bool
	Amount          string
	OrderID         uint64
	AutoConfirm     bool
	Broadcast       bool
	NonceText       string
	TxEpochText     string
	MaxFeeText      string
	TipsText        string
	FeePerGasText   string
	SignedPath      string
	RPCURL          string
	APIKey          string
}

const (
	directV3ContractHex = "6bc41502f4611d041a986cda580209d7f5a4e0d4"
	directV3CodeHashHex = "24221880179c31a21e183221e13d2456b222a71b12e874b288504748aaca38e1"
	// Keccak-256 of release/artifacts/fod_direct_escrow_v4.wasm.
	directV4CodeHashHex = "8b9fd14f9c08708db1ab25d90396563d3efe6bed6843c5aae15932d68bdf614c"
	// Keccak-256 of the lowered v5 candidate; release must reproduce these bytes.
	directV5CodeHashHex = "45d1d0ebf1140e0f9a7d0383878a6e6e00c424ec12c12de78cacb92ec92f268a"
)

func directFallbackVersion(o directCommandOptions, contract string) (int, error) {
	return directFallbackVersionWithV5Pin(o, contract, directV5CodeHashHex)
}

func directFallbackVersionWithV5Pin(o directCommandOptions, contract, v5CodeHash string) (int, error) {
	hash := strings.ToLower(strings.TrimPrefix(o.CodeHash, "0x"))
	if v5CodeHash != "" && hash == v5CodeHash && o.DirectVersion != 5 {
		return 0, errors.New("v5 direct contract requires -direct-version 5")
	}
	if o.DirectVersion == 5 {
		if !isHex(v5CodeHash, 32) || hash != v5CodeHash {
			return 0, errors.New("v5 direct fallback needs the reproducibly pinned v5 code hash")
		}
		return 5, nil
	}
	if o.DirectVersion == 4 {
		if directV4CodeHashHex == "" || hash != directV4CodeHashHex {
			return 0, errors.New("v4 direct fallback needs the reproducibly pinned v4 code hash")
		}
		return 4, nil
	}
	if o.DirectVersion != 0 && o.DirectVersion != 3 {
		return 0, errors.New("unsupported direct contract version")
	}
	if contract == directV3ContractHex && hash == directV3CodeHashHex {
		return 3, nil
	}
	return 0, errors.New("v3 fallback requires the exact deployed v3 contract and code hash; v4 needs -direct-version 4")
}

// A check result is display-only; direct-confirm accepts a signed delivery
// receipt, which is issued only after direct-receive saves the buyer's vault.
type directCheckResult struct {
	Format      string `json:"format"`
	Version     int    `json:"version"`
	Contract    string `json:"contract"`
	OrderID     uint64 `json:"orderId"`
	Author      string `json:"author"`
	Epoch       uint16 `json:"epoch"`
	UsableFlips int    `json:"usableFlips"`
}

// Direct outputs use atomic replacement, so an output path must not name any
// input or another output needed to finish or recover the trade.
func checkDirectPaths(o directCommandOptions) error {
	var paths []string
	switch o.Mode {
	case "direct-v7-prepare":
		paths = []string{o.SessionPath, o.ProofPath, o.ProofHostPath, o.VaultPath, o.DeliveryKeyPath, o.DeliveryPath, o.PreparedPath}
	case "direct-v7-dm":
		paths = []string{o.VaultPath, o.OutPath}
	case "direct-v7-receive":
		messagePath := o.MessagePath
		if messagePath == "-" {
			messagePath = ""
		}
		paths = []string{messagePath, o.PurchasePath, o.DeliveryKeyPath, o.VaultPath, o.OutPath}
	case "direct-pool-qualify", "direct-pool-prove-bundle":
		paths = []string{o.SessionPath, o.ProofPath, o.ProofHostPath, o.PreparedPath}
	case "direct-list":
		paths = []string{o.SessionPath, o.VaultPath, o.NodeKeyPath, o.PreparedPath}
	case "direct-buy":
		paths = []string{o.SessionPath, o.NodeKeyPath, o.DeliveryKeyPath, o.PreparedPath}
	case "direct-dm":
		paths = []string{o.VaultPath, o.OutPath}
	case "direct-check":
		messagePath := o.MessagePath
		if messagePath == "-" {
			messagePath = ""
		}
		paths = []string{messagePath, o.PurchasePath, o.DeliveryKeyPath, o.OutPath}
	case "direct-receive":
		messagePath := o.MessagePath
		if messagePath == "-" {
			messagePath = ""
		}
		paths = []string{messagePath, o.PurchasePath, o.DeliveryKeyPath, o.VaultPath, o.OutPath}
		if o.AutoConfirm {
			paths = append(paths, o.NodeKeyPath, o.PreparedPath, o.SignedPath)
		}
	case "direct-recover":
		paths = []string{o.PurchasePath, o.VaultPath, o.OutPath}
	case "direct-confirm":
		paths = []string{o.DeliveryPath, o.PreparedPath}
	case "direct-reserve-fallback":
		paths = []string{o.NodeKeyPath, o.PreparedPath}
	case "direct-start-fallback":
		paths = []string{o.NodeKeyPath, o.ProofPath, o.SessionPath, o.VaultPath, o.ProofHostPath, o.PreparedPath}
	case "direct-fallback":
		paths = []string{o.NodeKeyPath, o.VaultPath, o.PreparedPath}
	}
	seen := make(map[string]bool, len(paths))
	var existing []os.FileInfo
	for _, path := range paths {
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolve direct command path: %w", err)
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if errors.Is(err, os.ErrNotExist) {
			parent, parentErr := filepath.EvalSymlinks(filepath.Dir(absolute))
			if parentErr != nil {
				return fmt.Errorf("resolve direct command directory: %w", parentErr)
			}
			resolved = filepath.Join(parent, filepath.Base(absolute))
		} else if err != nil {
			return fmt.Errorf("resolve direct command path: %w", err)
		}
		info, err := os.Stat(absolute)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect direct command path: %w", err)
		}
		if seen[resolved] {
			return errors.New("direct command input and output paths must be distinct")
		}
		if info != nil {
			for _, old := range existing {
				if os.SameFile(old, info) {
					return errors.New("direct command input and output paths must be distinct")
				}
			}
			existing = append(existing, info)
		}
		seen[resolved] = true
	}
	return nil
}

func directListingRiskWarning(c *client, price *big.Int) string {
	const consequence = "a buyer who does not confirm can force a public proof within the 48-hour block window or a refund that permanently retires this offer"
	rate, err := readFeePerGas(c)
	if err != nil {
		return "warning: " + consequence + "; current buyer bond is unavailable"
	}
	bond, err := directQuotedBondForFee(rate)
	if err != nil {
		return "warning: " + consequence + "; current buyer bond cannot be represented"
	}
	return fmt.Sprintf("warning: buyers must lock about %s iDNA in refundable proof bond on top of the %s iDNA price at the current fee rate; %s",
		iDNAText(bond), iDNAText(price), consequence)
}

func directCommand(o directCommandOptions) error {
	if o.Mode == "direct-v7-prepare" && (o.Node == nil || !directLocalRPC(o.Node.url) || o.DirectVersion != 7 ||
		o.Broadcast || o.AutoConfirm || o.NodeKeyPath != "" || o.SignedPath != "") {
		return errors.New("v7 preparation requires -direct-version 7 and a loopback RPC, without signing or broadcast flags")
	}
	if (o.Mode == "direct-v7-dm" || o.Mode == "direct-v7-receive") &&
		(o.Node == nil || !directLocalRPC(o.Node.url) || o.DirectVersion != 7 || o.Broadcast || o.AutoConfirm ||
			o.NodeKeyPath != "" || o.SignedPath != "") {
		return errors.New("v7 private delivery needs -direct-version 7 and a loopback RPC, without signing or broadcast")
	}
	if isDirectV6PoolPrepareMode(o.Mode) {
		if err := validateDirectV6PoolPrepareOptions(o); err != nil {
			return err
		}
	}
	if (o.Mode == "direct-market" || o.Mode == "direct-pool-market") && (!o.OutExplicit || strings.TrimSpace(o.OutPath) == "") {
		return fmt.Errorf("%s needs -network idena -contract ADDRESS -code-hash KECCAK256 -out FILE", o.Mode)
	}
	if o.Mode == "direct-pool-market" && o.DirectVersion != 6 {
		return errors.New("direct-pool-market requires -direct-version 6")
	}
	if o.Mode == "direct-buy" && (o.NodeKeyPath == "") == (o.BuyerAddress == "") {
		return errors.New("direct-buy needs exactly one of -buyer ADDRESS or -nodekey FILE")
	}
	if err := checkDirectPaths(o); err != nil {
		return err
	}
	contract, err := directAddress(o.Contract)
	if err != nil {
		return err
	}
	if o.Network != NetworkNamePinned {
		return errors.New("direct sale requires the pinned network")
	}
	if err := checkBuyTarget(o.Node, contract, contract, o.CodeHash); err != nil {
		return err
	}
	if directV5CodeHashHex != "" && strings.EqualFold(strings.TrimPrefix(o.CodeHash, "0x"), directV5CodeHashHex) && o.DirectVersion != 5 {
		return errors.New("v5 direct contract requires -direct-version 5")
	}
	var fallbackVersion int
	if o.Mode != "direct-pool-market" && o.Mode != "direct-v7-prepare" && o.Mode != "direct-v7-dm" && o.Mode != "direct-v7-receive" && !isDirectV6PoolPrepareMode(o.Mode) && (o.Mode == "direct-reserve-fallback" || o.Mode == "direct-start-fallback" ||
		o.Mode == "direct-fallback" || o.Mode == "direct-refund" ||
		o.Mode == "direct-status" || o.Mode == "direct-market" || o.DirectVersion != 0) {
		fallbackVersion, err = directFallbackVersion(o, contract)
		if err != nil {
			return err
		}
	}
	switch o.Mode {
	case "direct-v7-prepare":
		return preparePoolV7FromFiles(o, contract)
	case "direct-v7-dm":
		if o.DirectVersion != 7 || o.OrderID == 0 || o.VaultPath == "" || !o.OutExplicit || o.OutPath == "" || o.Broadcast || o.AutoConfirm {
			return errors.New("direct-v7-dm needs -direct-version 7 -order -vault -out; no broadcast")
		}
		passphrase, err := readPassphrase("Vault passphrase: ")
		if err != nil {
			return err
		}
		secrets, _, err := directVaultSecrets(o.VaultPath, passphrase)
		if err != nil {
			return err
		}
		var active string
		if err := o.Node.call("dna_getCoinbaseAddr", []any{}, &active); err != nil {
			return err
		}
		caller, err := directAddress(active)
		if err != nil {
			return err
		}
		message, err := preparePoolV7PrivateMessage(o.Node, contract, o.OrderID, secrets, caller)
		if err != nil {
			return err
		}
		if err := writeNewPoolV7PrivateFile(o.OutPath, []byte(message)); err != nil {
			return err
		}
		fmt.Println("v7 encrypted delivery written locally; no message sent")
		return nil
	case "direct-v7-receive":
		if o.DirectVersion != 7 || o.OrderID == 0 || o.MessagePath == "" || o.PurchasePath == "" ||
			o.DeliveryKeyPath == "" || o.VaultPath == "" || !o.OutExplicit || o.OutPath == "" || o.Broadcast || o.AutoConfirm {
			return errors.New("direct-v7-receive needs -direct-version 7 -order -message -purchase -delivery-key -vault -out; no broadcast")
		}
		message, err := readDirectMessageFile(o.MessagePath)
		if err != nil {
			return err
		}
		purchase, err := readPreparedAction(o.PurchasePath)
		if err != nil {
			return err
		}
		deliveryKey, err := loadDeliveryKey(o.DeliveryKeyPath)
		if err != nil {
			return err
		}
		record, payload, err := verifyPoolV7PrivateDelivery(o.Node, contract, o.OrderID, message, purchase, deliveryKey)
		if err != nil {
			return err
		}
		if err := signDirectOpened(record, deliveryKey); err != nil {
			return err
		}
		passphrase, err := confirmPassphrase()
		if err != nil {
			return err
		}
		vaultJSON, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if err := WriteVault(o.VaultPath, passphrase, vaultJSON); err != nil {
			return err
		}
		receiptJSON, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		if err := writeNewPoolV7PrivateFile(o.OutPath, receiptJSON); err != nil {
			return err
		}
		fmt.Printf("verified %d original v7 flips; receipt and private vault saved; no confirmation sent\n", record.UsableFlips)
		return nil
	case "direct-pool-qualify", "direct-pool-prove-bundle":
		return prepareDirectV6PoolFromFiles(o, contract)
	case "direct-pool-market":
		state, err := buildDirectV6PoolState(o.Node, contract, strings.ToLower(strings.TrimPrefix(o.CodeHash, "0x")))
		if err != nil {
			return err
		}
		if err := writeDirectMarketJSON(o.OutPath, state); err != nil {
			return err
		}
		fmt.Printf("direct pool market: %d offers, %d orders at block %d\n", len(state.Offers), len(state.Orders), state.BlockHeight)
	case "direct-market":
		state, err := buildDirectMarketStateWithVersion(o.Node, contract, strings.ToLower(strings.TrimPrefix(o.CodeHash, "0x")), fallbackVersion)
		if err != nil {
			return err
		}
		if err := writeDirectMarketState(o.OutPath, state); err != nil {
			return err
		}
		fmt.Printf("direct market: %d offers, %d orders at block %d\n", len(state.Offers), len(state.Orders), state.BlockHeight)
	case "direct-status":
		orderID := o.OrderID
		if o.SessionPath != "" {
			session, err := readVerifiedFastSession(o.SessionPath)
			if err != nil {
				return err
			}
			if err := directVerifiedSession(o.Node, session); err != nil {
				return err
			}
			var author [20]byte
			authorRaw, err := hex.DecodeString(strings.TrimPrefix(session.Author, "0x"))
			if err != nil || len(authorRaw) != len(author) {
				return errors.New("session author is invalid")
			}
			copy(author[:], authorRaw)
			id := DirectOfferID(author, session.Epoch)
			offer, err := directOfferFromChain(o.Node, contract, id)
			if err != nil {
				return err
			}
			fmt.Printf("direct offer: %d canonical flips, %s base units, retired=%t\n", offer.Covered, offer.Price, offer.Retired)
			if offer.OpenOrderID != 0 {
				orderID = offer.OpenOrderID
			}
		}
		if orderID == 0 {
			if o.SessionPath != "" {
				fmt.Println("no open direct order")
				return nil
			}
			return errors.New("direct-status needs -order ID or -session FILE")
		}
		order, err := directOrderFromChain(o.Node, contract, orderID)
		if err != nil {
			return err
		}
		if _, err := directFundedAmount(order.Paid, order.Bond); err != nil {
			return err
		}
		var sellerStake *big.Int
		if fallbackVersion == 4 || fallbackVersion == 5 {
			if order.State == DirectFallbackIntent {
				return errors.New("v4/v5 direct order has removed fallback-intent state")
			}
			if order.State == DirectFallbackPending {
				sellerStake, err = directPendingStakeV4(o.Node, contract, orderID, order)
				if err != nil {
					return err
				}
			}
		}
		refundAt := DirectProofDeadline(order.FundedAt)
		if order.State == DirectFallbackIntent {
			offer, err := directOfferFromChain(o.Node, contract, order.OfferID)
			if err != nil {
				return err
			}
			head, _, err := registryHead(o.Node)
			if err != nil {
				return err
			}
			intent, err := directIncludedIntent(o.Node, contract, orderID, offer, order, head)
			if err != nil {
				return err
			}
			refundAt, _ = order.RefundAt(intent)
		}
		state := directOrderStateLabel(order.State)
		fmt.Printf("direct order %d: %s, price %s base units, bond %s base units, refund from block %d if still active\n",
			orderID, state, order.Paid, order.Bond, refundAt)
		if sellerStake != nil {
			fmt.Printf("seller stake %s base units is refundable on valid reveal or paid to the buyer at refund\n", sellerStake)
		}
		if (fallbackVersion == 4 || fallbackVersion == 5) && (order.State == DirectFunded || order.State == DirectFallbackPending) {
			head, _, err := registryHead(o.Node)
			if err != nil {
				return err
			}
			fmt.Println(directWindowStatus(order, head, fallbackVersion))
		}
	case "direct-list":
		if o.SessionPath == "" || o.VaultPath == "" || o.NodeKeyPath == "" || o.PreparedPath == "" {
			return errors.New("direct-list needs -session -vault -nodekey -contract -code-hash -amount -prepared")
		}
		session, err := readVerifiedFastSession(o.SessionPath)
		if err != nil {
			return err
		}
		identity, err := crypto.LoadECDSA(o.NodeKeyPath)
		if err != nil {
			return err
		}
		caller, err := nodekeyAddressOf(identity)
		if err != nil || !strings.EqualFold(caller, strings.TrimPrefix(session.Author, "0x")) {
			return errors.New("listing nodekey is not the session author")
		}
		passphrase, err := readPassphrase("Vault passphrase: ")
		if err != nil {
			return err
		}
		secrets, _, err := directVaultSecrets(o.VaultPath, passphrase)
		if err != nil {
			return err
		}
		price, ok := new(big.Int).SetString(o.Amount, 10)
		if !ok {
			return errors.New("direct price is not an integer")
		}
		prepared, err := prepareDirectList(o.Node, contract, session, secrets, price)
		if err != nil {
			return err
		}
		if fallbackVersion == 5 {
			prepared.DirectVersion = 5
		}
		if err := writePrivateJSON(o.PreparedPath, prepared); err != nil {
			return err
		}
		fmt.Printf("direct listing prepared: %d original flips; price %s base units; submit with this author's nodekey\n", prepared.CoveredFlips, prepared.Price)
		fmt.Fprintln(os.Stderr, directListingRiskWarning(o.Node, price))
	case "direct-buy":
		if o.SessionPath == "" || o.DeliveryKeyPath == "" || o.PreparedPath == "" {
			return errors.New("direct-buy needs -session (-buyer ADDRESS | -nodekey FILE) -delivery-key -contract -code-hash -prepared")
		}
		session, err := readVerifiedFastSession(o.SessionPath)
		if err != nil {
			return err
		}
		var buyer string
		if o.BuyerAddress != "" {
			buyer, err = directAddress(o.BuyerAddress)
			if err != nil {
				return fmt.Errorf("invalid -buyer address: %w", err)
			}
			var activeAddress string
			if err := o.Node.call("dna_getCoinbaseAddr", []any{}, &activeAddress); err != nil {
				return fmt.Errorf("read active buyer address: %w", err)
			}
			activeBuyer, err := directAddress(activeAddress)
			if err != nil {
				return fmt.Errorf("invalid active buyer address from node: %w", err)
			}
			if buyer != activeBuyer {
				return fmt.Errorf("-buyer %s does not match the active node buyer %s", buyer, activeBuyer)
			}
		} else {
			identity, err := crypto.LoadECDSA(o.NodeKeyPath)
			if err != nil {
				return err
			}
			buyer, err = nodekeyAddressOf(identity)
			if err != nil {
				return err
			}
		}
		if _, err := os.Lstat(o.DeliveryKeyPath); errors.Is(err, os.ErrNotExist) {
			if _, err := CreateDeliveryKey(o.DeliveryKeyPath); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		deliveryKey, err := loadDeliveryKey(o.DeliveryKeyPath)
		if err != nil {
			return err
		}
		prepared, err := prepareDirectBuy(o.Node, contract, session, buyer, deliveryKey)
		if err != nil {
			return err
		}
		if fallbackVersion == 5 {
			prepared.DirectVersion = 5
		}
		if err := writePrivateJSON(o.PreparedPath, prepared); err != nil {
			return err
		}
		submitNote := ""
		if o.NodeKeyPath != "" {
			submitNote = "; submit with this buyer's nodekey"
		}
		fmt.Printf("direct purchase prepared: %d original encrypted flips retained; price %s base units; total escrow %s base units including bond%s\n",
			prepared.CoveredFlips, prepared.Price, prepared.Amount, submitNote)
		fmt.Println("a valid public seller fallback can charge capped proof costs to this bond even if no private message arrives; any unused bond returns to the buyer")
	case "direct-dm":
		if o.OrderID == 0 || o.VaultPath == "" || !o.OutExplicit {
			return errors.New("direct-dm needs -order -vault -contract -code-hash -out MESSAGE_FILE")
		}
		passphrase, err := readPassphrase("Vault passphrase: ")
		if err != nil {
			return err
		}
		secrets, _, err := directVaultSecrets(o.VaultPath, passphrase)
		if err != nil {
			return err
		}
		message, err := prepareDirectDM(o.Node, contract, o.OrderID, secrets)
		if err != nil {
			return err
		}
		if err := writePrivateBytes(o.OutPath, []byte(message)); err != nil {
			return err
		}
		fmt.Println("private direct-delivery text prepared; send it only to this order's intended buyer through the chosen private channel")
	case "direct-check":
		if o.OrderID == 0 || o.MessagePath == "" || o.PurchasePath == "" || o.DeliveryKeyPath == "" || !o.OutExplicit || o.OutPath == "" {
			return errors.New("direct-check needs -order -message -purchase -delivery-key -contract -code-hash -out RESULT_FILE")
		}
		if o.AutoConfirm || o.Broadcast {
			return errors.New("direct-check only verifies delivery and cannot broadcast or confirm")
		}
		message, err := readDirectMessageFile(o.MessagePath)
		if err != nil {
			return err
		}
		purchase, err := readPreparedAction(o.PurchasePath)
		if err != nil {
			return err
		}
		deliveryKey, err := loadDeliveryKey(o.DeliveryKeyPath)
		if err != nil {
			return err
		}
		record, _, err := verifyDirectDM(o.Node, contract, o.OrderID, message, purchase, deliveryKey, true)
		if err != nil {
			return err
		}
		result := directCheckResult{
			Format: "fod.direct-check", Version: 1, Contract: record.Contract,
			OrderID: record.OrderID, Author: record.Author, Epoch: record.Epoch,
			UsableFlips: record.UsableFlips,
		}
		if err := writePrivateJSON(o.OutPath, result); err != nil {
			return err
		}
		fmt.Printf("verified %d original flips; display-only result written; no transaction submitted\n", record.UsableFlips)
	case "direct-receive":
		if o.OrderID == 0 || o.MessagePath == "" || o.PurchasePath == "" || o.DeliveryKeyPath == "" || o.VaultPath == "" || !o.OutExplicit {
			return errors.New("direct-receive needs -order -message -purchase -delivery-key -vault OUTPUT -out RECEIPT_FILE")
		}
		if o.AutoConfirm && (!o.Broadcast || o.PreparedPath == "" || o.NodeKeyPath == "" || o.SignedPath == "" ||
			o.NonceText == "" || o.TxEpochText == "" || o.MaxFeeText == "") {
			return errors.New("automatic confirmation needs -broadcast -prepared -nodekey -signed -nonce -tx-epoch -max-fee")
		}
		message, err := readDirectMessageFile(o.MessagePath)
		if err != nil {
			return err
		}
		purchase, err := readPreparedAction(o.PurchasePath)
		if err != nil {
			return err
		}
		deliveryKey, err := loadDeliveryKey(o.DeliveryKeyPath)
		if err != nil {
			return err
		}
		passphrase, err := confirmPassphrase()
		if err != nil {
			return err
		}
		record, payload, err := importDirectDM(o.Node, contract, o.OrderID, message, purchase, deliveryKey, !o.AutoConfirm && !o.Broadcast)
		if err != nil {
			return err
		}
		vaultJSON, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if err := WriteVault(o.VaultPath, passphrase, vaultJSON); err != nil {
			return err
		}
		if err := writePrivateJSON(o.OutPath, record); err != nil {
			return err
		}
		fmt.Printf("verified %d original flips; private vault and signed usable-delivery receipt written; buyer may prepare confirmation while the order is funded\n", record.UsableFlips)
		if o.AutoConfirm {
			confirm, err := prepareDirectConfirm(o.Node, contract, record)
			if err != nil {
				return err
			}
			if fallbackVersion == 5 {
				confirm.DirectVersion = 5
			}
			if err := writePrivateJSON(o.PreparedPath, confirm); err != nil {
				return err
			}
			nonce, err := parseNonce(o.NonceText)
			if err != nil {
				return err
			}
			epoch, err := parseEpoch(o.TxEpochText)
			if err != nil {
				return err
			}
			maxFee, ok := new(big.Int).SetString(o.MaxFeeText, 10)
			if !ok || maxFee.Sign() <= 0 {
				return errors.New("automatic confirmation needs a positive max fee")
			}
			tips := big.NewInt(0)
			if o.TipsText != "" {
				tips, ok = new(big.Int).SetString(o.TipsText, 10)
				if !ok || tips.Sign() < 0 {
					return errors.New("invalid automatic confirmation tips")
				}
			}
			var feePerGas *big.Int
			if o.FeePerGasText != "" {
				feePerGas, ok = new(big.Int).SetString(o.FeePerGasText, 10)
				if !ok || feePerGas.Sign() <= 0 {
					return errors.New("invalid automatic confirmation fee rate")
				}
			}
			runSubmit(o.PreparedPath, o.NodeKeyPath, nonce, epoch, maxFee, tips,
				o.SignedPath, true, o.RPCURL, o.APIKey, feePerGas, contract, o.CodeHash)
		}
	case "direct-recover":
		if o.OrderID == 0 || o.PurchasePath == "" || o.VaultPath == "" || !o.OutExplicit || o.OutPath == "" {
			return errors.New("direct-recover needs -network idena -contract -code-hash -order -purchase -vault OUTPUT -out RECEIPT")
		}
		if o.Broadcast || o.AutoConfirm {
			return errors.New("direct-recover only reads public state and cannot broadcast or confirm")
		}
		purchase, err := readPreparedAction(o.PurchasePath)
		if err != nil {
			return err
		}
		record, payload, err := recoverDirectReveal(o.Node, contract, o.OrderID, purchase, o.CodeHash)
		if err != nil {
			return err
		}
		passphrase, err := confirmPassphrase()
		if err != nil {
			return err
		}
		vaultJSON, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if err := WriteVault(o.VaultPath, passphrase, vaultJSON); err != nil {
			return err
		}
		if err := writePrivateJSON(o.OutPath, record); err != nil {
			return err
		}
		fmt.Printf("verified %d original flips from the settled public reveal; owner-only vault and recovery receipt written\n", record.UsableFlips)
	case "direct-confirm":
		if o.DeliveryPath == "" || o.PreparedPath == "" {
			return errors.New("direct-confirm needs -delivery RECEIPT_FILE -contract -code-hash -prepared")
		}
		raw, err := os.ReadFile(o.DeliveryPath)
		if err != nil {
			return err
		}
		var record DirectOpenedDelivery
		if err := decodeStrict(raw, &record); err != nil {
			return err
		}
		prepared, err := prepareDirectConfirm(o.Node, contract, &record)
		if err != nil {
			return err
		}
		if fallbackVersion == 5 {
			prepared.DirectVersion = 5
		}
		if err := writePrivateJSON(o.PreparedPath, prepared); err != nil {
			return err
		}
		fmt.Println("buyer confirmation prepared from signed local verification; submit with the funded buyer's nodekey")
	case "direct-reserve-fallback":
		if fallbackVersion != 3 {
			return errors.New("v4/v5 has no separate fallback reservation")
		}
		if o.OrderID == 0 || o.NodeKeyPath == "" || o.PreparedPath == "" {
			return errors.New("direct-reserve-fallback needs -order -nodekey -contract -code-hash -prepared")
		}
		identity, err := crypto.LoadECDSA(o.NodeKeyPath)
		if err != nil {
			return err
		}
		caller, err := nodekeyAddressOf(identity)
		if err != nil {
			return err
		}
		prepared, err := prepareDirectReserveFallback(o.Node, contract, o.OrderID, caller)
		if err != nil {
			return err
		}
		if err := writePrivateJSON(o.PreparedPath, prepared); err != nil {
			return err
		}
		fmt.Println("seller fallback intent prepared; submit with the seller's nodekey, then wait for state fallback-intent before preparing a proof")
	case "direct-start-fallback":
		if o.OrderID == 0 || o.NodeKeyPath == "" || o.ProofPath == "" || o.VaultPath == "" || o.ProofHostPath == "" || o.PreparedPath == "" || (fallbackVersion == 5 && o.SessionPath == "") {
			return errors.New("direct-start-fallback needs -order -nodekey -proof -vault -registration-host -contract -code-hash -prepared; v5 also needs -session")
		}
		identity, err := crypto.LoadECDSA(o.NodeKeyPath)
		if err != nil {
			return err
		}
		caller, err := nodekeyAddressOf(identity)
		if err != nil {
			return err
		}
		if fallbackVersion == 4 || fallbackVersion == 5 {
			if _, _, err := directStartFallbackV4Ready(o.Node, contract, o.OrderID, caller); err != nil {
				return err
			}
		} else if _, _, err := directStartFallbackReady(o.Node, contract, o.OrderID, caller); err != nil {
			return err
		}
		passphrase, err := readPassphrase("Vault passphrase: ")
		if err != nil {
			return err
		}
		secrets, _, err := directVaultSecrets(o.VaultPath, passphrase)
		if err != nil {
			return err
		}
		var prepared *preparedAction
		if fallbackVersion == 5 {
			prepared, err = prepareDirectStartFallbackV5(o.Node, contract, o.OrderID, caller, o.ProofPath, o.SessionPath, o.ProofHostPath, secrets)
		} else if fallbackVersion == 4 {
			prepared, err = prepareDirectStartFallbackV4(o.Node, contract, o.OrderID, caller, o.ProofPath, o.ProofHostPath, secrets)
		} else {
			prepared, err = prepareDirectStartFallback(o.Node, contract, o.OrderID, caller, o.ProofPath, o.ProofHostPath, secrets)
		}
		if err != nil {
			return err
		}
		if err := writePrivateJSON(o.PreparedPath, prepared); err != nil {
			return err
		}
		if fallbackVersion == 4 || fallbackVersion == 5 {
			fmt.Printf("proof-backed fallback start prepared with seller stake %s base units; submit with the seller's nodekey and wait for state fallback-pending before public disclosure\n", prepared.Amount)
		} else {
			fmt.Println("fallback start prepared after sealed proof preflight; submit with the seller's nodekey and wait for state fallback-pending before preparing public disclosure")
		}
	case "direct-fallback":
		if o.OrderID == 0 || o.NodeKeyPath == "" || o.VaultPath == "" || o.PreparedPath == "" {
			return errors.New("direct-fallback needs -order -nodekey -vault -contract -code-hash -prepared")
		}
		_, pendingOrder, sale, err := directFallbackReady(o.Node, contract, o.OrderID)
		if err != nil {
			return err
		}
		if fallbackVersion == 4 || fallbackVersion == 5 {
			if _, err := directPendingStakeV4(o.Node, contract, o.OrderID, pendingOrder); err != nil {
				return err
			}
		}
		identity, err := crypto.LoadECDSA(o.NodeKeyPath)
		if err != nil {
			return err
		}
		caller, err := nodekeyAddressOf(identity)
		if err != nil {
			return err
		}
		if caller != sale.Offer.Author {
			return errors.New("direct fallback reveal requires this offer's author")
		}
		passphrase, err := readPassphrase("Vault passphrase: ")
		if err != nil {
			return err
		}
		secrets, _, err := directVaultSecrets(o.VaultPath, passphrase)
		if err != nil {
			return err
		}
		prepared, err := prepareDirectFallback(o.Node, contract, o.OrderID, caller, secrets)
		if err != nil {
			return err
		}
		if fallbackVersion == 4 || fallbackVersion == 5 {
			prepared.DirectVersion = fallbackVersion
		}
		if err := writePrivateJSON(o.PreparedPath, prepared); err != nil {
			return err
		}
		fmt.Println("public fallback reveal prepared; submitting it publishes the epoch flip keys after the sealed proof was accepted at fallback start")
	case "direct-refund":
		if o.OrderID == 0 || o.PreparedPath == "" {
			return errors.New("direct-refund needs -order -contract -code-hash -prepared")
		}
		prepared, err := prepareDirectRefund(o.Node, contract, o.OrderID)
		if err != nil {
			return err
		}
		if fallbackVersion == 4 || fallbackVersion == 5 {
			order, err := directOrderFromChain(o.Node, contract, o.OrderID)
			if err != nil {
				return err
			}
			if order.State == DirectFallbackIntent {
				return errors.New("v4/v5 direct order has removed fallback-intent state")
			}
			if order.State == DirectFallbackPending {
				if _, err := directPendingStakeV4(o.Node, contract, o.OrderID, order); err != nil {
					return err
				}
			}
			prepared.DirectVersion = fallbackVersion
		}
		if err := writePrivateJSON(o.PreparedPath, prepared); err != nil {
			return err
		}
		fmt.Println("refund prepared; the contract sends escrow only to the original buyer")
	default:
		return errors.New("unsupported direct-sale command")
	}
	return nil
}

// Report the CLI's conservative preparation window; an open height window is
// still subject to the fresh proof, signer, fee, and chain checks at submit.
func directV4WindowStatus(order *DirectOrder, head uint64) string {
	return directWindowStatus(order, head, 4)
}

func directWindowStatus(order *DirectOrder, head uint64, version int) string {
	if order == nil {
		return fmt.Sprintf("v%d fallback height window is unavailable", version)
	}
	switch order.State {
	case DirectFunded:
		end, ok := directReserveDeadline(order.FundedAt)
		if !ok {
			return fmt.Sprintf("v%d proof-start height window is unavailable", version)
		}
		return fmt.Sprintf("v%d proof-start preparation: blocks %d through %d; observed head %d; height window open=%t",
			version, order.FundedAt+directConfirmGrace, end-2, head, order.CanStartFallbackV4(head) && head < end-1)
	case DirectFallbackPending:
		deadline := DirectProofDeadline(order.FundedAt)
		if deadline == ^uint64(0) && order.FundedAt > ^uint64(0)-directProofWindow {
			return fmt.Sprintf("v%d key-reveal height window is unavailable", version)
		}
		return fmt.Sprintf("v%d key-reveal preparation: through block %d; observed head %d; height window open=%t",
			version, deadline-2, head, order.CanProve(head) && head < deadline-1)
	default:
		return fmt.Sprintf("v%d fallback height window is unavailable", version)
	}
}

func directReserveFallbackReady(c *client, contract string, orderID uint64, caller string) (*DirectOffer, *DirectOrder, DirectDMContext, error) {
	offer, order, context, err := directOpenSale(c, contract, orderID)
	if err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	if caller != context.Offer.Author {
		return nil, nil, DirectDMContext{}, errors.New("direct fallback reservation requires this offer's author")
	}
	head, _, err := registryHead(c)
	if err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	end, ok := directReserveDeadline(order.FundedAt)
	if !order.CanReserveFallback(head) || !ok || head >= end-1 {
		return nil, nil, DirectDMContext{}, fmt.Errorf("direct fallback reservation is outside the grace and proof window ending at block %d", end)
	}
	return offer, order, context, nil
}

func prepareDirectReserveFallback(c *client, contract string, orderID uint64, caller string) (*preparedAction, error) {
	offer, order, sale, err := directReserveFallbackReady(c, contract, orderID, caller)
	if err != nil {
		return nil, err
	}
	rate, err := readFeePerGas(c)
	if err != nil {
		return nil, fmt.Errorf("read current direct intent fee: %w", err)
	}
	stake, err := directQuotedIntentStakeForFee(offer.Price, rate)
	if err != nil {
		return nil, err
	}
	arg, err := EncodeDirectReserveFallback(orderID)
	if err != nil {
		return nil, err
	}
	return &preparedAction{
		Format: preparedDirectReserveFallbackFormat, Version: actionRequestVersion, Operation: directReserveFallbackOperation,
		Network: NetworkNamePinned, Contract: contract, Amount: stake.String(), Calldata: hex.EncodeToString(arg),
		DirectOfferID: sale.Offer.ID, DirectOrderID: orderID, Author: sale.Offer.Author, Price: offer.Price.String(),
		OrderNonce: order.Nonce, PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func directStartFallbackReady(c *client, contract string, orderID uint64, caller string) (*DirectOffer, DirectDMContext, error) {
	offer, order, context, err := directActiveSale(c, contract, orderID)
	if err != nil {
		return nil, DirectDMContext{}, err
	}
	if caller != context.Offer.Author {
		return nil, DirectDMContext{}, errors.New("direct fallback start requires this offer's author")
	}
	if order.State != DirectFallbackIntent {
		return nil, DirectDMContext{}, errors.New("direct fallback proof start requires an included fallback-intent order")
	}
	head, _, err := registryHead(c)
	if err != nil {
		return nil, DirectDMContext{}, err
	}
	intent, err := directIncludedIntent(c, contract, orderID, offer, order, head)
	if err != nil {
		return nil, DirectDMContext{}, err
	}
	end, ok := directIntentDeadline(intent.At)
	proofEnd, proofOK := directReserveDeadline(order.FundedAt)
	if ok && proofOK {
		end = min(end, proofEnd)
	}
	if !order.CanStartFallback(head, intent) || !ok || !proofOK || head >= end-1 {
		return nil, DirectDMContext{}, fmt.Errorf("direct fallback proof start closes at block %d", end)
	}
	return offer, context, nil
}

func directStartFallbackV4Ready(c *client, contract string, orderID uint64, caller string) (*DirectOffer, *DirectOrder, error) {
	offer, order, context, err := directOpenSale(c, contract, orderID)
	if err != nil {
		return nil, nil, err
	}
	if caller != context.Offer.Author {
		return nil, nil, errors.New("direct fallback start requires this offer's author")
	}
	head, _, err := registryHead(c)
	if err != nil {
		return nil, nil, err
	}
	end, ok := directReserveDeadline(order.FundedAt)
	if !ok || !order.CanStartFallbackV4(head) || head >= end-1 {
		return nil, nil, fmt.Errorf("v4 direct fallback start is outside the grace and proof window ending at block %d", end)
	}
	return offer, order, nil
}

func prepareDirectStartFallback(c *client, contract string, orderID uint64, caller, proofPath, hostPath string, secrets []byte) (*preparedAction, error) {
	offer, context, err := directStartFallbackReady(c, contract, orderID, caller)
	if err != nil {
		return nil, err
	}
	registration, err := verifyDirectStartProof(c, contract, orderID, proofPath, hostPath, secrets)
	if err != nil {
		return nil, err
	}
	proof, err := hex.DecodeString(registration.Calldata)
	if err != nil {
		return nil, err
	}
	arg, err := EncodeDirectStartFallback(orderID, proof[:directJournalBytes], proof[directJournalBytes:])
	if err != nil {
		return nil, err
	}
	p := &preparedAction{
		Format: preparedDirectStartFallbackFormat, Version: actionRequestVersion, Operation: directStartFallbackOperation,
		Network: NetworkNamePinned, Contract: contract, Amount: "0", Calldata: hex.EncodeToString(arg),
		DirectOfferID: context.Offer.ID, DirectOrderID: orderID, Author: hex.EncodeToString(offer.Author[:]),
		Epoch: registration.Epoch, CoveredFlips: registration.CoveredFlips,
		ManifestCommitment: registration.ManifestCommitment, KeyCommitment: registration.KeyCommitment,
		InventoryCommitment: registration.InventoryCommitment, PolicyDigest: registration.PolicyDigest,
		ConsensusDigest: registration.ConsensusDigest, NetworkDigest: registration.NetworkDigest,
		ImageID: registration.ImageID, ClaimDigest: registration.ClaimDigest,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := validateDirectStartFallbackStatement(c, p, offer, arg); err != nil {
		return nil, err
	}
	return p, nil
}

func prepareDirectStartFallbackV4(c *client, contract string, orderID uint64, caller, proofPath, hostPath string, secrets []byte) (*preparedAction, error) {
	offer, order, err := directStartFallbackV4Ready(c, contract, orderID, caller)
	if err != nil {
		return nil, err
	}
	registration, err := verifyDirectStartProofForVersion(c, contract, orderID, proofPath, hostPath, secrets, 4)
	if err != nil {
		return nil, err
	}
	rate, err := readFeePerGas(c)
	if err != nil {
		return nil, fmt.Errorf("read current direct seller stake fee: %w", err)
	}
	stake, err := directQuotedStartStakeForFee(offer.Price, rate)
	if err != nil {
		return nil, err
	}
	proof, err := hex.DecodeString(registration.Calldata)
	if err != nil {
		return nil, err
	}
	arg, err := EncodeDirectStartFallback(orderID, proof[:directJournalBytes], proof[directJournalBytes:])
	if err != nil {
		return nil, err
	}
	p := &preparedAction{
		Format: preparedDirectStartFallbackV4Format, Version: actionRequestVersion, DirectVersion: 4,
		Operation: directStartFallbackOperation, Network: NetworkNamePinned, Contract: contract,
		Amount: stake.String(), Calldata: hex.EncodeToString(arg), DirectOfferID: hex.EncodeToString(order.OfferID[:]),
		DirectOrderID: orderID, OrderNonce: order.Nonce, Price: offer.Price.String(), Author: caller,
		Epoch: registration.Epoch, CoveredFlips: registration.CoveredFlips,
		ManifestCommitment: registration.ManifestCommitment, KeyCommitment: registration.KeyCommitment,
		InventoryCommitment: registration.InventoryCommitment, PolicyDigest: registration.PolicyDigest,
		ConsensusDigest: registration.ConsensusDigest, NetworkDigest: registration.NetworkDigest,
		ImageID: registration.ImageID, ClaimDigest: registration.ClaimDigest,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := validateDirectStartFallbackStatement(c, p, offer, arg); err != nil {
		return nil, err
	}
	return p, nil
}

// The seller produces the proof after the bonded intent is included. The
// independent host check verifies the Groth16 seal without publishing keys.
func verifyDirectStartProof(c *client, contract string, orderID uint64, proofPath, hostPath string, secrets []byte) (*PreparedRegistration, error) {
	return verifyDirectStartProofForVersion(c, contract, orderID, proofPath, hostPath, secrets, 3)
}

func verifyDirectStartProofForVersion(c *client, contract string, orderID uint64, proofPath, hostPath string, secrets []byte, version int) (*PreparedRegistration, error) {
	offer, order, sale, err := directActiveSale(c, contract, orderID)
	if err != nil {
		return nil, err
	}
	if version == 4 {
		if _, _, err := directStartFallbackV4Ready(c, contract, orderID, sale.Offer.Author); err != nil {
			return nil, err
		}
	} else if version == 3 {
		head, _, err := registryHead(c)
		if err != nil {
			return nil, err
		}
		intent, err := directIncludedIntent(c, contract, orderID, offer, order, head)
		if err != nil {
			return nil, err
		}
		intentEnd, intentOK := directIntentDeadline(intent.At)
		proofEnd, proofOK := directReserveDeadline(order.FundedAt)
		if order.State != DirectFallbackIntent || !intentOK || !proofOK || !order.CanStartFallback(head, intent) || head >= min(intentEnd, proofEnd)-1 {
			return nil, errors.New("direct fallback proof start has no safe inclusion margin")
		}
	} else {
		return nil, errors.New("unsupported direct fallback proof version")
	}
	if !directSecretsMatch(secrets, offer) {
		return nil, errors.New("fallback vault differs from funded offer")
	}
	proofInfo, err := os.Lstat(proofPath)
	if err != nil {
		return nil, err
	}
	if !proofInfo.Mode().IsRegular() || proofInfo.Mode().Perm()&0o077 != 0 || proofInfo.Size() > 64*1024 {
		return nil, errors.New("fallback proof must be an owner-only regular file")
	}
	hostInfo, err := os.Stat(hostPath)
	if err != nil {
		return nil, err
	}
	if !hostInfo.Mode().IsRegular() || hostInfo.Mode().Perm()&0o111 == 0 {
		return nil, errors.New("registration host must be an executable regular file")
	}
	artefact, err := readRegistrationArtefact(proofPath)
	if err != nil {
		return nil, err
	}
	registration, err := assembleRegistration(NetworkNamePinned, contract, proofPath, artefact, directStartFallbackOperation, preparedDirectStartFallbackFormat, directV2ImageIDHex)
	if err != nil {
		return nil, err
	}
	if registration.Author != sale.Offer.Author || registration.Epoch != offer.Epoch || registration.CoveredFlips != offer.Covered ||
		registration.KeyCommitment != hex.EncodeToString(offer.Keys[:]) || registration.InventoryCommitment != hex.EncodeToString(offer.Inventory[:]) {
		return nil, errors.New("sealed fallback statement differs from funded direct offer")
	}
	journal, _ := hex.DecodeString(strings.TrimPrefix(artefact.Journal, "0x"))
	seal, _ := hex.DecodeString(strings.TrimPrefix(artefact.Seal, "0x"))
	arg, err := EncodeDirectStartFallback(orderID, journal, seal)
	if err != nil {
		return nil, err
	}
	prepared := &preparedAction{
		Contract: contract, Author: sale.Offer.Author, Epoch: offer.Epoch, CoveredFlips: offer.Covered,
		ManifestCommitment: registration.ManifestCommitment, PolicyDigest: registration.PolicyDigest,
		ConsensusDigest: registration.ConsensusDigest, NetworkDigest: registration.NetworkDigest,
		KeyCommitment: registration.KeyCommitment, InventoryCommitment: registration.InventoryCommitment,
		ImageID: registration.ImageID, ClaimDigest: registration.ClaimDigest,
	}
	if err := validateDirectStartFallbackStatement(c, prepared, offer, arg); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostPath, "--verify-direct-v2", proofPath, "--contract", contract)
	// The verifier needs neither the vault passphrase nor RPC credentials from
	// the parent process. Keep its environment minimal even for a user-supplied
	// executable path.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "RISC0_DEV_MODE=0"}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("sealed fallback proof verification timed out")
		}
		return nil, errors.New("registration host refused the sealed fallback proof")
	}
	return registration, nil
}

func directFallbackReady(c *client, contract string, orderID uint64) (*DirectOffer, *DirectOrder, DirectDMContext, error) {
	offer, order, context, err := directActiveSale(c, contract, orderID)
	if err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	if order.State != DirectFallbackPending {
		return nil, nil, DirectDMContext{}, errors.New("direct fallback requires an included fallback-pending order")
	}
	if _, err := directProofRateFromChain(c, contract, orderID); err != nil {
		return nil, nil, DirectDMContext{}, fmt.Errorf("direct fallback requires an included proof-rate record: %w", err)
	}
	head, _, err := registryHead(c)
	if err != nil {
		return nil, nil, DirectDMContext{}, err
	}
	if head >= DirectProofDeadline(order.FundedAt)-1 {
		return nil, nil, DirectDMContext{}, errors.New("direct fallback has no safe inclusion margin")
	}
	return offer, order, context, nil
}

func prepareDirectFallback(c *client, contract string, orderID uint64, caller string, secrets []byte) (*preparedAction, error) {
	offer, order, context, err := directFallbackReady(c, contract, orderID)
	if err != nil {
		return nil, err
	}
	if caller != context.Offer.Author {
		return nil, errors.New("direct fallback reveal requires this offer's author")
	}
	if !directSecretsMatch(secrets, offer) {
		return nil, errors.New("fallback vault differs from funded offer")
	}
	arg, err := EncodeDirectProveAndReveal(orderID, secrets)
	if err != nil {
		return nil, err
	}
	p := &preparedAction{
		Format: preparedDirectFallbackFormat, Version: actionRequestVersion, Operation: directFallbackOperation,
		Network: NetworkNamePinned, Contract: contract, Amount: "0", Calldata: hex.EncodeToString(arg),
		DirectOfferID: context.Offer.ID, DirectOrderID: orderID, Author: context.Offer.Author,
		Epoch: offer.Epoch, CoveredFlips: offer.Covered, KeyCommitment: hex.EncodeToString(offer.Keys[:]),
		InventoryCommitment: hex.EncodeToString(offer.Inventory[:]), PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := validateDirectFallbackEvidence(c, p, offer, order, arg); err != nil {
		return nil, err
	}
	return p, nil
}

func prepareDirectRefund(c *client, contract string, orderID uint64) (*preparedAction, error) {
	offer, order, context, err := directActiveSale(c, contract, orderID)
	if err != nil {
		return nil, err
	}
	head, _, err := registryHead(c)
	if err != nil {
		return nil, err
	}
	var intent *DirectIntent
	if order.State == DirectFallbackIntent {
		intent, err = directIncludedIntent(c, contract, orderID, offer, order, head)
		if err != nil {
			return nil, err
		}
	}
	refundAt, ok := order.RefundAt(intent)
	if !ok || !order.CanRefund(head, intent) {
		return nil, fmt.Errorf("refund opens at block %d", refundAt)
	}
	arg, err := EncodeDirectRefund(orderID)
	if err != nil {
		return nil, err
	}
	return &preparedAction{
		Format: preparedDirectRefundFormat, Version: actionRequestVersion, Operation: operationRefund,
		Network: NetworkNamePinned, Contract: contract, Amount: "0", Calldata: hex.EncodeToString(arg),
		DirectOfferID: context.Offer.ID, DirectOrderID: orderID,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func directReceiptCipherHash(record *DirectOpenedDelivery) string {
	ciphertext, _ := hex.DecodeString(record.Ciphertext)
	digest := sha256.Sum256(ciphertext)
	return hex.EncodeToString(digest[:])
}
