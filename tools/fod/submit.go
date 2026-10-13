package main

// The last step of the file-based workflow in `SPEC.md` 4.3: turn a prepared
// action into the transaction a node accepts.
//
// The transaction is built with the pinned node's own packages -
// `attachments.CallContractAttachment`, `types.SignTx`, `Transaction.ToBytes` -
// so the encoding is the node's rather than a second implementation of it. What
// this command cannot check without a node is whether the nonce, epoch, and fee
// it was given are the ones the chain will accept; it says so, and a stale
// transaction is rejected by the node rather than applied.
//
// Nothing is sent unless `-broadcast` is passed. The signed transaction is
// verified here first: it is decoded again with the node's own decoder, the
// sender is recovered, and the attachment is parsed back, so a mistake in how
// this command fills the transaction is caught before a fee can be spent.

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/crypto"
)

// preparedFormats are the prepared actions this command can sign. Each carries
// the same three things it needs: the contract, the operation, and the calldata
// that operation takes.
var preparedFormats = map[string]string{
	preparedDirectListFormat:            "list",
	preparedDirectBuyFormat:             operationBuy,
	preparedDirectConfirmFormat:         operationConfirm,
	preparedDirectReserveFallbackFormat: directReserveFallbackOperation,
	preparedDirectStartFallbackFormat:   directStartFallbackOperation,
	preparedDirectStartFallbackV4Format: directStartFallbackOperation,
	preparedDirectStartFallbackV5Format: directStartFallbackOperation,
	preparedDirectFallbackFormat:        directFallbackOperation,
	preparedDirectRefundFormat:          operationRefund,
	"fod.prepared-withdrawal":           operationWithdrawal,
	"fod.prepared-withdraw-reserve":     operationReserveWithdrawal,
	"fod.prepared-buy":                  operationBuy,
	"fod.prepared-deliver":              operationDeliver,
	"fod.prepared-beacon":               operationBeacon,
	"fod.prepared-fulfill-order":        operationFulfill,
	"fod.prepared-fulfill-fast":         operationFulfillFast,
	"fod.prepared-confirm":              operationConfirm,
	"fod.prepared-register-bundle":      operationRegisterBundle,
	"fod.prepared-register-fast":        operationRegisterFast,
	"fod.prepared-sell-unit":            operationSellUnit,
	"fod.prepared-donate-unit":          operationDonateUnit,
	"fod.prepared-refund-order":         operationRefund,
	"fod.prepared-reveal":               operationReveal,
	"fod.prepared-fund":                 operationAddReserve,
}

// preparedAction is the union of the three prepared files. They share what a
// transaction needs - the contract, the operation, the amount, and the calldata
// - and differ in the terms they carry for the record, so every field any of
// them can name is listed and strict decoding still rejects anything else.
type preparedAction struct {
	buyValidated       bool
	buyTargetValidated string
	confirmValidated   bool
	directValidated    bool
	VerifiedDelivery   *OpenedPoolDelivery `json:"verifiedDelivery,omitempty"`
	Format             string              `json:"format"`
	Version            int                 `json:"version"`
	Operation          string              `json:"operation"`
	Network            string              `json:"network"`
	Contract           string              `json:"contract"`
	Amount             string              `json:"amount"`
	Calldata           string              `json:"calldata"`
	PreparedAt         string              `json:"preparedAt"`
	Submitted          bool                `json:"submitted"`

	// Listing terms.
	Commitment string `json:"commitment,omitempty"`
	Price      string `json:"price,omitempty"`
	Ceiling    string `json:"ceiling,omitempty"`

	// Buy terms: the bundle a purchase names, the pool nonce whose possession
	// proof the calldata carries, and the producer's note for the reader. The
	// decoder is strict, so a field a producer writes and this union does not
	// name rejects the CLI's own file (`REVIEW.md` A07).
	Bundle string `json:"bundle,omitempty"`
	Nonce  uint64 `json:"nonce,omitempty"`
	Note   string `json:"note,omitempty"`
	// Availability is the buyer's own content check, recorded by prepare-buy
	// before the call was built (`SPEC.md` 5.8). It is evidence for the buyer,
	// not a term of the transaction.
	Availability *AvailabilityCheck `json:"availability,omitempty"`
	Fast         *fastBuyEvidence   `json:"fast,omitempty"`
	// CiphertextSha256 is what a prepared confirmation binds: the digest of the
	// delivered bytes the buyer checked (`SPEC.md` 7.1). It travels in the
	// calldata too, and it is recorded here so a reader sees which payload the
	// confirmation settles.
	CiphertextSha256 string `json:"ciphertextSha256,omitempty"`

	// Order terms, shared by the purchase and the delivery.
	OfferID     uint64 `json:"offerId,omitempty"`
	OrderNonce  uint64 `json:"orderNonce,omitempty"`
	Seller      string `json:"seller,omitempty"`
	Buyer       string `json:"buyer,omitempty"`
	DeliveryKey string `json:"deliveryKey,omitempty"`
	Proof       string `json:"proof,omitempty"`

	// Registration terms.
	Author             string `json:"author,omitempty"`
	Epoch              uint16 `json:"epoch,omitempty"`
	CoveredFlips       uint32 `json:"coveredFlips,omitempty"`
	ManifestCommitment string `json:"manifestCommitment,omitempty"`
	KeyCommitment      string `json:"keyCommitment,omitempty"`
	// The version 3 statement publishes these two so a buyer can recompute the
	// inventory commitment and the contract can refuse another network; both
	// travel in the file `prepare-registration` writes, so the strict decoder
	// has to name them (`REVIEW.md` A07).
	InventoryCommitment string `json:"inventoryCommitment,omitempty"`
	PolicyDigest        string `json:"policyDigest,omitempty"`
	ConsensusDigest     string `json:"consensusDigest,omitempty"`
	NetworkDigest       string `json:"networkDigest,omitempty"`
	ImageID             string `json:"imageId,omitempty"`
	ClaimDigest         string `json:"claimDigest,omitempty"`

	// Direct-sale context is kept distinct from the pool's numeric offer ID.
	DirectOfferID string                `json:"directOfferId,omitempty"`
	DirectOrderID uint64                `json:"directOrderId,omitempty"`
	DirectVersion int                   `json:"directVersion,omitempty"`
	DirectSession *SessionBundle        `json:"directSession,omitempty"`
	DirectOpened  *DirectOpenedDelivery `json:"directOpened,omitempty"`
}

// SubmittedTransaction is what the CLI hands back after signing.
type SubmittedTransaction struct {
	Format       string             `json:"format"`
	Version      int                `json:"version"`
	Network      string             `json:"network"`
	Contract     string             `json:"contract"`
	Operation    string             `json:"operation"`
	Amount       string             `json:"amount"`
	Sender       string             `json:"sender"`
	AccountNonce uint32             `json:"accountNonce"`
	Epoch        uint16             `json:"epoch"`
	MaxFee       string             `json:"maxFee"`
	Tips         string             `json:"tips"`
	RawTx        string             `json:"rawTx"`
	TxHash       string             `json:"txHash"`
	NodeHash     string             `json:"nodeHash,omitempty"`
	Broadcast    bool               `json:"broadcast"`
	SignedAt     string             `json:"signedAt"`
	Outcome      *SubmissionOutcome `json:"outcome,omitempty"`
}

// SubmissionOutcome is what the node reported about a broadcast transaction.
//
// `SPEC.md` 4.3 step 7 asks the CLI to export receipts, and the honest record is
// whatever the node said: a transaction that is not mined yet has no receipt, and
// saying so is not a failure of the submission. `included` is therefore separate
// from `success`, which is the contract call's own result once there is one.
type SubmissionOutcome struct {
	Included  bool   `json:"included"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	GasUsed   uint64 `json:"gasUsed,omitempty"`
	TxFee     string `json:"txFee,omitempty"`
	CheckedAt string `json:"checkedAt"`
	Note      string `json:"note,omitempty"`
}

func loadPreparedAction(path string) (*preparedAction, error) {
	prepared, err := readPreparedAction(path)
	if err != nil {
		return nil, err
	}
	if prepared.Operation == operationBuy {
		if err := requireCheckedAvailability(prepared.Availability, time.Now()); err != nil {
			return nil, err
		}
	}
	return prepared, nil
}

func readRetainedPurchase(path string) (*preparedAction, error) {
	p, err := readPreparedAction(path)
	if err != nil {
		return nil, err
	}
	if p.Operation != operationBuy {
		return nil, fmt.Errorf("not a retained purchase")
	}
	if err := requireRetainedInventory(p.Availability); err != nil {
		return nil, err
	}
	return p, nil
}

func readPreparedAction(path string) (*preparedAction, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var prepared preparedAction
	if err := decodeStrict(raw, &prepared); err != nil {
		return nil, fmt.Errorf("read prepared action: %w", err)
	}
	operation, ok := preparedFormats[prepared.Format]
	if !ok {
		return nil, fmt.Errorf("not a prepared action this command signs: %q", prepared.Format)
	}
	if prepared.Version != actionRequestVersion {
		return nil, fmt.Errorf("unsupported prepared action version %d", prepared.Version)
	}
	if prepared.Operation != operation {
		return nil, fmt.Errorf("the %s file names operation %q", prepared.Format, prepared.Operation)
	}
	if !isHex(prepared.Contract, addressBytes) {
		return nil, fmt.Errorf("the prepared action names no valid contract address")
	}
	if !isDecimal(prepared.Amount) {
		return nil, fmt.Errorf("the prepared amount is not an integer in base units")
	}
	if !isHex(prepared.Calldata, len(prepared.Calldata)/2) || len(prepared.Calldata)%2 != 0 {
		return nil, fmt.Errorf("the prepared calldata is not hex")
	}
	return &prepared, nil
}

// BuildTransaction assembles and signs the contract call, then checks its own
// work with the node's decoder before returning the bytes.
func BuildTransaction(prepared *preparedAction, identity *ecdsa.PrivateKey, nonce uint32, epoch uint16, maxFee, tips *big.Int) ([]byte, common.Address, error) {
	if prepared.Operation == operationConfirm && !prepared.confirmValidated {
		return nil, common.Address{}, fmt.Errorf("confirm requires signing-time validation of the buyer's usable-content receipt")
	}
	if prepared.Operation == operationBuy {
		if !prepared.buyValidated || !strings.EqualFold(prepared.buyTargetValidated, prepared.Contract) {
			return nil, common.Address{}, fmt.Errorf("buy requires signing-time canonical inventory validation")
		}
		if err := requireCheckedAvailability(prepared.Availability, time.Now()); err != nil {
			return nil, common.Address{}, err
		}
		buyer, err := nodekeyAddressOf(identity)
		if err != nil || buyer != prepared.Buyer {
			return nil, common.Address{}, fmt.Errorf("the signing key does not control the prepared buyer")
		}
	}
	if isDirectFormat(prepared.Format) && !prepared.directValidated {
		return nil, common.Address{}, errors.New("direct sale requires signing-time canonical-state validation")
	}
	if prepared.Operation == operationRegisterFast {
		if fastContentImageIDHex == strings.Repeat("0", 64) || !strings.EqualFold(prepared.ImageID, fastContentImageIDHex) {
			return nil, common.Address{}, errors.New("fast content guest image is not pinned")
		}
		author, err := nodekeyAddressOf(identity)
		if err != nil || !strings.EqualFold(author, prepared.Author) {
			return nil, common.Address{}, errors.New("the signing key does not control the fast registration author")
		}
		if prepared.Amount != "0" || !isHex(prepared.Bundle, 32) {
			return nil, common.Address{}, errors.New("invalid fast registration terms")
		}
	}
	if prepared.Operation == operationFulfillFast {
		seller, err := nodekeyAddressOf(identity)
		if err != nil || seller != prepared.Seller {
			return nil, common.Address{}, errors.New("the signing key does not control the fast delivery author")
		}
	}
	contractBytes, err := hex.DecodeString(strings.ToLower(prepared.Contract))
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("contract address: %w", err)
	}
	var contract common.Address
	copy(contract[:], contractBytes)

	amount, ok := new(big.Int).SetString(prepared.Amount, 10)
	if !ok {
		return nil, common.Address{}, fmt.Errorf("the prepared amount could not be read")
	}
	calldata, err := hex.DecodeString(strings.ToLower(prepared.Calldata))
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("prepared calldata: %w", err)
	}

	// The attachment is the node's own type; the contract runtime receives its
	// args as the bytes this carries.
	attachment := attachments.CreateCallContractAttachment(prepared.Operation, calldata)
	payload, err := attachment.ToBytes()
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("encode the contract call: %w", err)
	}

	signed, err := types.SignTx(&types.Transaction{
		AccountNonce: nonce,
		Epoch:        epoch,
		Type:         types.CallContractTx,
		To:           &contract,
		Amount:       amount,
		MaxFee:       maxFee,
		Tips:         tips,
		Payload:      payload,
	}, identity)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("sign the transaction: %w", err)
	}
	raw, err := signed.ToBytes()
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("encode the transaction: %w", err)
	}

	// Check the signed bytes the way a node will: decode, recover the sender,
	// and parse the attachment back out.
	var decoded types.Transaction
	if err := decoded.FromBytes(raw); err != nil {
		return nil, common.Address{}, fmt.Errorf("the node's own decoder refused the transaction: %w", err)
	}
	if decoded.Type != types.CallContractTx || decoded.To == nil || *decoded.To != contract {
		return nil, common.Address{}, fmt.Errorf("the decoded transaction does not call the contract")
	}
	if decoded.AccountNonce != nonce || decoded.Epoch != epoch {
		return nil, common.Address{}, fmt.Errorf("the decoded transaction lost its nonce or epoch")
	}
	// The node's own encoding writes a zero amount as nothing and reads it back
	// as nil, which is how it spells zero everywhere. Compare through that.
	if amountOrZero(decoded.Amount).Cmp(amount) != 0 {
		return nil, common.Address{}, fmt.Errorf("the decoded transaction lost its amount")
	}
	if amountOrZero(decoded.MaxFee).Cmp(maxFee) != 0 {
		return nil, common.Address{}, fmt.Errorf("the decoded transaction lost its max fee")
	}
	parsed := attachments.ParseCallContractAttachment(&decoded)
	if parsed == nil || parsed.Method != prepared.Operation {
		return nil, common.Address{}, fmt.Errorf("the decoded transaction lost its contract call")
	}
	// An empty argument survives the encoding as one empty argument, which is
	// what a call without arguments looks like; accepting none as well costs
	// nothing and means the same thing.
	argsSurvived := len(parsed.Args) == 1 && bytesEqual(parsed.Args[0], calldata)
	if len(calldata) == 0 && len(parsed.Args) == 0 {
		argsSurvived = true
	}
	if !argsSurvived {
		return nil, common.Address{}, fmt.Errorf("the decoded transaction lost its call arguments")
	}
	if len(decoded.Signature) != signatureBytes {
		return nil, common.Address{}, fmt.Errorf("the signature is %d bytes, expected %d", len(decoded.Signature), signatureBytes)
	}
	sender, err := types.Sender(&decoded)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("the node's own recovery refused the signature: %w", err)
	}
	expected, err := nodekeyAddressOf(identity)
	if err != nil {
		return nil, common.Address{}, err
	}
	if !strings.EqualFold(sender.Hex(), "0x"+expected) {
		return nil, common.Address{}, fmt.Errorf("the transaction recovers to %s, not %s", sender.Hex(), expected)
	}
	return raw, sender, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// amountOrZero reads an amount the way the node does: a missing one is zero.
func amountOrZero(value *big.Int) *big.Int {
	if value == nil {
		return new(big.Int)
	}
	return value
}

// nodeReceipt is the part of the node's `bcn_txReceipt` answer this CLI records.
// `txFee` is kept raw because the node writes a decimal, which is a JSON string,
// and the record should stay honest even if that changes.
type nodeReceipt struct {
	Method  string          `json:"method"`
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	GasUsed uint64          `json:"gasUsed"`
	TxFee   json.RawMessage `json:"txFee"`
}

// outcomeRetryDelay is how long recordOutcome waits before asking a second time.
// It is a variable so a test does not have to wait for a block.
var outcomeRetryDelay = 3 * time.Second

// checkAccountAgainstChain refuses a transaction the node is certain to reject on
// its own nonce or epoch rules, and names the value that would work.
//
// Idena does not take a nonce equal to the account's: a transaction is invalid
// unless its nonce is higher (`blockchain/validation/validation.go`,
// `nonce >= tx.AccountNonce`), which is not what a reader expects and is what the
// node's own error says only obliquely. The epoch rule is one-sided in the same
// way: a transaction may not name an epoch below the chain's.
func checkAccountAgainstChain(c *client, sender string, accountNonce uint32, epoch uint16) error {
	var account struct {
		Nonce uint32 `json:"nonce"`
	}
	if err := c.call("dna_getBalance", []any{"0x" + sender}, &account); err != nil {
		return fmt.Errorf("the account could not be read: %w", err)
	}
	if account.Nonce >= accountNonce {
		return fmt.Errorf("the account nonce on the chain is %d, and a new transaction has to be higher: use %d",
			account.Nonce, account.Nonce+1)
	}
	var chainEpoch struct {
		Epoch uint16 `json:"epoch"`
	}
	if err := c.call("dna_epoch", []any{}, &chainEpoch); err != nil {
		return fmt.Errorf("the chain epoch could not be read: %w", err)
	}
	if chainEpoch.Epoch > epoch {
		return fmt.Errorf("the chain is at epoch %d, so a transaction cannot name epoch %d", chainEpoch.Epoch, epoch)
	}
	return nil
}

// checkDirectSpendBalance uses the same maximum cost as the node's transaction
// validation: call amount, tips, and max fee. A pending nonce also makes a new
// signed direct call unsafe to broadcast from this account.
func checkDirectSpendBalance(c *client, sender string, signedNonce uint32, amount, tips, maxFee *big.Int) error {
	var account struct {
		Balance      string `json:"balance"`
		Nonce        uint32 `json:"nonce"`
		MempoolNonce uint32 `json:"mempoolNonce"`
	}
	if err := c.call("dna_getBalance", []any{"0x" + sender}, &account); err != nil {
		return fmt.Errorf("the direct account balance could not be read: %w", err)
	}
	if account.Nonce != account.MempoolNonce {
		return errors.New("the direct account has a pending transaction; reconcile its nonce before broadcasting")
	}
	if account.Nonce == ^uint32(0) || signedNonce != account.Nonce+1 {
		return fmt.Errorf("the direct transaction needs the next executable nonce after %d", account.Nonce)
	}
	available, ok := new(big.Rat).SetString(account.Balance)
	if !ok || available.Sign() < 0 {
		return errors.New("the direct account balance is invalid")
	}
	available.Mul(available, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)))
	if !available.IsInt() {
		return errors.New("the direct account balance has sub-atom precision")
	}
	needed := new(big.Int).Add(amount, tips)
	needed.Add(needed, maxFee)
	if available.Num().Cmp(needed) < 0 {
		return fmt.Errorf("the direct account balance cannot cover amount, tips, and max fee (%s base units required)", needed)
	}
	return nil
}

// recordOutcome asks the node what became of a broadcast transaction.
//
// A transaction that was just broadcast is usually still in the mempool, where
// the node has no receipt for it: it answers `null` for something it knows only
// there, and its receipt converter fails on a missing receipt and recovers per
// request. Both are recorded as "not included yet" rather than treated as a
// failure of the submission, because the transaction is already on its way.
// One retry after a block's worth of time catches the ordinary case without
// holding the command open.
func recordOutcome(node *client, hash string) *SubmissionOutcome {
	outcome := &SubmissionOutcome{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(outcomeRetryDelay)
		}
		var receipt *nodeReceipt
		if err := node.call("bcn_txReceipt", []any{hash}, &receipt); err != nil {
			outcome.Included = false
			if errors.Is(err, errEmptyResponse) {
				// The node's own converter panicked on the missing receipt and
				// the server recovered per request (H-12): the transaction is
				// simply not in a block yet, which is not a decode failure.
				outcome.Note = "the node has no receipt yet: its converter panicked on a missing receipt and the request came back empty"
			} else {
				outcome.Note = fmt.Sprintf("the node reported no receipt: %v", err)
			}
			continue
		}
		if receipt == nil {
			outcome.Included = false
			outcome.Note = "the node has no receipt yet: not mined, or the hash is unknown to it"
			continue
		}
		outcome.Included = true
		outcome.Success = receipt.Success
		outcome.Error = receipt.Error
		outcome.GasUsed = receipt.GasUsed
		outcome.Note = ""
		if len(receipt.TxFee) > 0 {
			outcome.TxFee = strings.Trim(string(receipt.TxFee), "\"")
		}
		return outcome
	}
	return outcome
}

// broadcastDirect saves the signed call and its locally derived hash before
// asking the node to send it. An interrupted or unanswered send remains
// recoverable by that hash; it must not be repeated until chain state is read.
func broadcastDirect(node *client, raw []byte, outPath string, submitted *SubmittedTransaction) error {
	if outPath == "" {
		return errors.New("a private signed output is required for a direct broadcast")
	}
	expectedHash := common.Hash(crypto.Hash(raw)).Hex()
	submitted.TxHash = expectedHash
	submitted.Outcome = &SubmissionOutcome{
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Note:      "send outcome unknown; check the transaction hash and account nonce before any retry",
	}
	if err := writePrivateJSON(outPath, submitted); err != nil {
		return fmt.Errorf("save signed direct transaction before sending: %w", err)
	}
	var nodeHash string
	if err := node.call("bcn_sendRawTx", []any{"0x" + hex.EncodeToString(raw)}, &nodeHash); err != nil {
		submitted.Outcome = recordOutcome(node, expectedHash)
		if !submitted.Outcome.Included {
			submitted.Outcome.Note = "node did not acknowledge the send; inclusion is unknown; check the saved hash and account nonce before any retry"
		}
		if saveErr := writePrivateJSON(outPath, submitted); saveErr != nil {
			return fmt.Errorf("node did not acknowledge the send (%v); signed transaction was saved but its outcome update failed: %w", err, saveErr)
		}
		if submitted.Outcome.Included && submitted.Outcome.Success {
			return nil
		}
		return fmt.Errorf("node did not acknowledge the send; transaction outcome requires reconciliation: %w", err)
	}
	submitted.NodeHash = nodeHash
	if !strings.EqualFold(nodeHash, expectedHash) {
		submitted.Outcome = &SubmissionOutcome{
			CheckedAt: time.Now().UTC().Format(time.RFC3339),
			Note:      "node hash differs from the signed transaction; reconcile both hashes and account nonce before any retry",
		}
		if err := writePrivateJSON(outPath, submitted); err != nil {
			return fmt.Errorf("node returned a different transaction hash and outcome update failed: %w", err)
		}
		return errors.New("node returned a different transaction hash; outcome requires reconciliation")
	}
	submitted.Outcome = recordOutcome(node, expectedHash)
	if err := writePrivateJSON(outPath, submitted); err != nil {
		return fmt.Errorf("node acknowledged the send but outcome update failed; signed transaction was saved: %w", err)
	}
	return nil
}

// maxFeeShortfall returns the max fee a signed transaction needs when the one
// it carries cannot give the call the gas it needs, and nil when it can.
//
// The requirement is the size fee of what was actually signed plus the gas
// budget the measured wasm gas implies, because the node derives the call's
// allowance by dividing what is left of the max fee by the chain's rate
// (`tools/fod/fees.go`). The signed size is the one that counts: RLP encodes
// the nonce, epoch, amount, and the max fee itself, so the transaction a caller
// signs can be a byte or two longer than the one the table measured, and a max
// fee computed from the measured size then leaves the call short of gas, which
// the chain still charges for.
func maxFeeShortfall(operation string, signedBytes uint64, feePerGas, maxFee *big.Int) *big.Int {
	gas, ok := measuredGasFor(operation)
	if !ok || feePerGas == nil || feePerGas.Sign() <= 0 || maxFee == nil {
		return nil
	}
	required := requiredMaxFeeFor(gas.WasmGas, signedBytes, feePerGas)
	if maxFee.Cmp(required) < 0 {
		return required
	}
	return nil
}

func runSubmit(preparedPath, nodeKeyPath string, nonce uint32, epoch uint16, maxFee, tips *big.Int, outPath string, broadcast bool, rpcURL, apiKey string, feePerGas *big.Int, approvedContract, approvedCodeHash string) {
	explicitFeePerGas := feePerGas
	prepared, err := loadPreparedAction(preparedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepared action refused: %v\n", err)
		os.Exit(1)
	}
	if prepared.Operation == operationReveal && outPath == "" {
		fmt.Fprintln(os.Stderr, "reveal refused: -out FILE is required to keep unrevealed keys out of terminal output")
		os.Exit(1)
	}
	if prepared.Operation == operationFulfillFast && outPath == "" {
		fmt.Fprintln(os.Stderr, "fast delivery refused: -out FILE is required to keep the buyer-bound proof out of terminal output")
		os.Exit(1)
	}
	if isDirectFormat(prepared.Format) && outPath == "" {
		fmt.Fprintln(os.Stderr, "direct sale refused: -signed FILE is required to keep private prepared data out of terminal output")
		os.Exit(1)
	}
	if prepared.Operation == operationBuy || isDirectFormat(prepared.Format) {
		c := &client{url: rpcURL, key: apiKey, http: newHTTPClient()}
		if err := checkBuyTarget(c, prepared.Contract, approvedContract, approvedCodeHash); err != nil {
			fmt.Fprintf(os.Stderr, "purchase refused before signing: %v\n", err)
			os.Exit(1)
		}
	}
	identity, err := crypto.LoadECDSA(nodeKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load nodekey: %v\n", err)
		os.Exit(1)
	}
	if prepared.Operation == operationBuy && !isDirectFormat(prepared.Format) {
		c := &client{url: rpcURL, key: apiKey, http: newHTTPClient()}
		if err := validatePreparedBuy(c, prepared); err != nil {
			fmt.Fprintf(os.Stderr, "purchase refused before signing: %v\n", err)
			os.Exit(1)
		}
		prepared.buyTargetValidated = prepared.Contract
	}
	if prepared.Operation == operationConfirm && !isDirectFormat(prepared.Format) {
		c := &client{url: rpcURL, key: apiKey, http: newHTTPClient()}
		if err := validatePreparedConfirm(c, prepared, identity); err != nil {
			fmt.Fprintf(os.Stderr, "confirmation refused before signing: %v\n", err)
			os.Exit(1)
		}
	}
	if err := validatePreparedDisclosure(&client{url: rpcURL, key: apiKey, http: newHTTPClient()}, prepared); err != nil {
		fmt.Fprintf(os.Stderr, "disclosure refused before signing: %v\n", err)
		os.Exit(1)
	}
	if isDirectFormat(prepared.Format) {
		c := &client{url: rpcURL, key: apiKey, http: newHTTPClient()}
		if err := validatePreparedDirect(c, prepared, identity); err != nil {
			fmt.Fprintf(os.Stderr, "direct sale refused before signing: %v\n", err)
			os.Exit(1)
		}
		sender, err := nodekeyAddressOf(identity)
		if err != nil {
			fmt.Fprintf(os.Stderr, "direct sender refused: %v\n", err)
			os.Exit(1)
		}
		if _, rate, err := estimateDirectCall(c, prepared, sender, maxFee, feePerGas); err != nil {
			fmt.Fprintf(os.Stderr, "direct simulation refused before signing: %v\n", err)
			os.Exit(1)
		} else {
			feePerGas = rate
		}
	}
	raw, sender, err := BuildTransaction(prepared, identity, nonce, epoch, maxFee, tips)
	if err != nil {
		fmt.Fprintf(os.Stderr, "refused: %v\n", err)
		os.Exit(1)
	}

	submitted := &SubmittedTransaction{
		Format:       "fod.submitted-transaction",
		Version:      actionRequestVersion,
		Network:      prepared.Network,
		Contract:     strings.ToLower(prepared.Contract),
		Operation:    prepared.Operation,
		Amount:       prepared.Amount,
		Sender:       strings.ToLower(strings.TrimPrefix(sender.Hex(), "0x")),
		AccountNonce: nonce,
		Epoch:        epoch,
		MaxFee:       maxFee.String(),
		Tips:         tips.String(),
		RawTx:        hex.EncodeToString(raw),
		Broadcast:    broadcast,
		SignedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	// `SPEC.md` 4.3 step 5 asks for the estimated fee before a transaction is
	// submitted. The rate the chain is charging is the one that matters, and a
	// broadcast is already talking to the node, so it is read there rather than
	// left to the caller to look up. An explicit -fee-per-gas still wins.
	var node *client
	if broadcast {
		node = &client{url: rpcURL, key: apiKey, http: newHTTPClient()}
		if feePerGas == nil {
			var chainFee big.Int
			if err := node.call("bcn_feePerGas", []any{}, &chainFee); err == nil && chainFee.Sign() > 0 {
				feePerGas = &chainFee
			}
		}
	}

	fmt.Printf("operation   %s on %s\n", submitted.Operation, submitted.Network)
	fmt.Printf("contract    %s\n", submitted.Contract)
	fmt.Printf("amount      %s iDNA base units\n", submitted.Amount)
	fmt.Printf("sender      %s\n", submitted.Sender)
	fmt.Printf("nonce       %d, epoch %d\n", submitted.AccountNonce, submitted.Epoch)
	fmt.Printf("max fee     %s, tips %s\n", submitted.MaxFee, submitted.Tips)
	var shortfall *big.Int
	if gas, ok := measuredGasFor(prepared.Operation); ok && feePerGas != nil && feePerGas.Sign() > 0 {
		// The requirement is computed from the transaction that was actually
		// signed: RLP encodes the nonce, epoch, amount, and the max fee itself,
		// so a signed transaction can be a byte or two longer than the one the
		// table measured, and the node charges the size it is given.
		required := requiredMaxFeeFor(gas.WasmGas, uint64(len(raw)), feePerGas)
		fmt.Printf("estimated   %s base units at %s per gas for the %d bytes signed (%d user gas for %d measured wasm gas, %d runs)\n",
			required, feePerGas, len(raw), userGasForWasmGas(gas.WasmGas), gas.WasmGas, gas.Runs)
		if uint64(len(raw)) > gas.TxBytes {
			fmt.Printf("note        the signed transaction is %d bytes, %d more than the %d the fee table measured; the requirement above uses the signed size\n",
				len(raw), uint64(len(raw))-gas.TxBytes, gas.TxBytes)
		}
		if maxFee.Cmp(required) < 0 {
			shortfall = required
			fmt.Println("warning     the max fee is below that requirement; the call runs out of gas and the fee is still charged")
		}
		if cap := maxMaxFeeFor(feePerGas); maxFee.Cmp(cap) > 0 {
			fmt.Printf("warning     the max fee is above the node's cap (%s); it is refused as too high\n", cap)
		}
	} else if isDirectFormat(prepared.Format) {
		fmt.Println("estimated   direct call simulated on current contract state; max fee includes gas margin")
	} else if feePerGas != nil {
		fmt.Printf("estimated   no measured gas for %s; nothing to compare\n", prepared.Operation)
		if cap := maxMaxFeeFor(feePerGas); maxFee.Cmp(cap) > 0 {
			fmt.Printf("warning     the max fee is above the node's cap (%s); it is refused as too high\n", cap)
		}
	} else if broadcast {
		fmt.Println("estimated   the node did not report a fee per gas, so there is no estimate to show")
	}
	fmt.Printf("raw tx      %d bytes\n", len(raw))
	fmt.Println("note        the nonce, epoch, and fee are what you passed; a broadcast checks the nonce and epoch against the account before it sends")

	if broadcast {
		if shortfall != nil {
			fmt.Fprintf(os.Stderr, "refused before broadcasting: this signed transaction needs a max fee of %s, and it carries %s; sign it again with the higher value\n",
				shortfall, maxFee)
			os.Exit(1)
		}
		if err := checkAccountAgainstChain(node, submitted.Sender, nonce, epoch); err != nil {
			fmt.Fprintf(os.Stderr, "refused before broadcasting: %v\n", err)
			os.Exit(1)
		}
		if err := validatePreparedDisclosure(node, prepared); err != nil {
			fmt.Fprintf(os.Stderr, "disclosure refused before broadcasting: %v\n", err)
			os.Exit(1)
		}
		if isDirectFormat(prepared.Format) {
			if err := validatePreparedDirect(node, prepared, identity); err != nil {
				fmt.Fprintf(os.Stderr, "direct sale refused before broadcasting: %v\n", err)
				os.Exit(1)
			}
			if _, _, err := estimateDirectCall(node, prepared, submitted.Sender, maxFee, explicitFeePerGas); err != nil {
				fmt.Fprintf(os.Stderr, "direct simulation refused before broadcasting: %v\n", err)
				os.Exit(1)
			}
			amount, ok := new(big.Int).SetString(prepared.Amount, 10)
			if !ok {
				fmt.Fprintln(os.Stderr, "direct sale refused before broadcasting: invalid amount")
				os.Exit(1)
			}
			if err := checkDirectSpendBalance(node, submitted.Sender, nonce, amount, tips, maxFee); err != nil {
				fmt.Fprintf(os.Stderr, "direct sale refused before broadcasting: %v\n", err)
				os.Exit(1)
			}
		}
		if prepared.Operation == operationBuy || isDirectFormat(prepared.Format) {
			if err := checkBuyTarget(node, prepared.Contract, approvedContract, approvedCodeHash); err != nil {
				fmt.Fprintf(os.Stderr, "purchase refused before broadcasting: %v\n", err)
				os.Exit(1)
			}
		}
		var hash string
		if isDirectFormat(prepared.Format) {
			if err := broadcastDirect(node, raw, outPath, submitted); err != nil {
				fmt.Fprintf(os.Stderr, "direct broadcast needs reconciliation: %v\n", err)
				os.Exit(1)
			}
			hash = submitted.TxHash
		} else {
			if err := node.call("bcn_sendRawTx", []any{"0x" + hex.EncodeToString(raw)}, &hash); err != nil {
				fmt.Fprintf(os.Stderr, "the node refused the transaction: %v\n", err)
				os.Exit(1)
			}
			submitted.NodeHash = hash
			submitted.TxHash = hash
			submitted.Outcome = recordOutcome(node, hash)
		}
		fmt.Printf("broadcast   %s\n", hash)
		if submitted.Outcome.Included {
			fmt.Printf("outcome     mined, contract call success=%t, gas %d, fee %s\n",
				submitted.Outcome.Success, submitted.Outcome.GasUsed, submitted.Outcome.TxFee)
			if submitted.Outcome.Error != "" {
				fmt.Printf("outcome     the call reported %s\n", submitted.Outcome.Error)
			}
		} else {
			fmt.Printf("outcome     %s\n", submitted.Outcome.Note)
		}
	} else {
		fmt.Println("not broadcast: pass -broadcast to send it to the node")
	}

	if outPath != "" {
		if err := writePrivateJSON(outPath, submitted); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", outPath, err)
			os.Exit(1)
		}
		fmt.Printf("signed      %s\n", outPath)
	} else {
		fmt.Printf("raw         %s\n", submitted.RawTx)
	}
}

func parseNonce(value string) (uint32, error) {
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("nonce %q is not an unsigned 32-bit integer", value)
	}
	return uint32(parsed), nil
}

func parseEpoch(value string) (uint16, error) {
	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("epoch %q is not an unsigned 16-bit integer", value)
	}
	return uint16(parsed), nil
}
