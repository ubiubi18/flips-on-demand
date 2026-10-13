package main

// The delivery desk of `SPEC.md` 6.4 step 9: watch the pool for funded orders
// on the bundles whose epoch keys this machine holds, publish the buyer-bound
// ciphertext so the buyer can use the keys immediately, and keep the delivery
// beacon fresh so the contract keeps accepting buys.
//
// The fast path needs no proof. The ciphertext is the pinned ECIES of the
// canonical secrets to the delivery key the order recorded, and the buyer
// checks it against the bundle's key commitment before confirming. When the
// buyer never confirms, the desk settles the order inside the window: with a
// prover it can prepare the delivery proof (the measured v2 proof takes about
// sixteen minutes and has not met this deadline), and within `revealMargin` it publishes the
// committed secrets (`reveal`), which settles the order and retires the
// bundle's remaining units (`AH-19`). A delivery is only published while a
// later pass can still include that reveal before the refund boundary;
// otherwise the desk reveals directly or leaves the order to the buyer's
// repayment. Signing and broadcasting recheck that the next block is still
// before the deadline. This does not hide pending data or guarantee inclusion;
// pre-settlement disclosure remains a protocol gap.

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
)

const (
	operationDeliver = "deliver"
	operationBeacon  = "beacon"

	// beaconFreshnessBlocks is the contract's window; the desk refreshes well
	// before it closes, so a pass that fails still leaves time for the next.
	beaconFreshnessBlocks = 288
	serveDefaultRefresh   = uint64(144)
	serveDefaultProof     = uint64(280)
	serveDefaultReveal    = uint64(2)

	preparedDeliverFormat = "fod.prepared-deliver"
	preparedBeaconFormat  = "fod.prepared-beacon"
	preparedFulfillFormat = "fod.prepared-fulfill-order"
)

// The nonce poll is a variable so a test can take it down to a millisecond;
// the timeout stays longer than a block so a slow node is not a refusal.
var (
	serveNoncePoll    = 3 * time.Second
	serveNonceTimeout = 90 * time.Second
)

// servedBundle is one bundle this desk can deliver, indexed by the commitment
// the pool stores. The commitment is a function of the epoch keys alone, so the
// vaults match the chain without a separate index.
type servedBundle struct {
	Commitment string
	Author     string
	Epoch      uint16
	Payload    VaultPayload
}

// servedBundlesFromPayloads indexes the vaults this desk holds by bundle
// commitment. Every payload is checked before it can serve anything.
func servedBundlesFromPayloads(payloads []VaultPayload) (map[string]servedBundle, error) {
	out := make(map[string]servedBundle)
	for _, payload := range payloads {
		if payload.Kind != "fod.epoch-keys" || payload.Version != 1 {
			return nil, fmt.Errorf("unexpected vault payload: kind=%q version=%d", payload.Kind, payload.Version)
		}
		author, err := hex.DecodeString(strings.TrimPrefix(payload.Address, "0x"))
		if err != nil || len(author) != addressBytes {
			return nil, fmt.Errorf("the vault names no valid author address")
		}
		keys, err := EpochKeysFromVaultPayload(payload)
		if err != nil {
			return nil, err
		}
		publicPart, privatePart := keys.SecretBytes()
		commitment, err := SecretsCommitmentHex(payload.Epoch, author, publicPart, privatePart)
		if err != nil {
			return nil, err
		}
		out[commitment] = servedBundle{
			Commitment: commitment,
			Author:     strings.ToLower(hex.EncodeToString(author)),
			Epoch:      payload.Epoch,
			Payload:    payload,
		}
	}
	return out, nil
}

// deliveryCiphertextFor re-encrypts the bundle's canonical secrets to the
// buyer's delivery key. This is the fast path of `SPEC.md` 7.2: the buyer's own
// tool decrypts and hashes the plaintext against the bundle's commitment.
func deliveryCiphertextFor(bundle servedBundle, buyerKeyHex string) ([]byte, error) {
	buyerKey, err := hex.DecodeString(strings.TrimPrefix(buyerKeyHex, "0x"))
	if err != nil || len(buyerKey) != deliveryKeyBytes {
		return nil, fmt.Errorf("the order's delivery key is not %d bytes of hex", deliveryKeyBytes)
	}
	publicKey, err := crypto.UnmarshalPubkey(buyerKey)
	if err != nil {
		return nil, fmt.Errorf("the order's delivery key is not a curve point: %w", err)
	}
	author, err := hex.DecodeString(strings.TrimPrefix(bundle.Author, "0x"))
	if err != nil || len(author) != addressBytes {
		return nil, fmt.Errorf("the bundle names no valid author address")
	}
	keys, err := EpochKeysFromVaultPayload(bundle.Payload)
	if err != nil {
		return nil, err
	}
	publicPart, privatePart := keys.SecretBytes()
	canonical, err := SecretsCanonicalBytes(bundle.Epoch, author, publicPart, privatePart)
	if err != nil {
		return nil, err
	}
	ciphertext, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(publicKey), canonical, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("encrypt the delivery: %w", err)
	}
	return ciphertext, nil
}

// poolDeliverCalldata is the escrow's `deliver` argument: order id (8,
// big-endian) || ciphertext.
func poolDeliverCalldata(orderID uint64, ciphertext []byte) []byte {
	out := make([]byte, 8+len(ciphertext))
	binary.BigEndian.PutUint64(out[:8], orderID)
	copy(out[8:], ciphertext)
	return out
}

// deliveryRecord is the desk's own note of the ciphertext it published for an
// order. The escrow keeps whichever ciphertext was published last, so the desk
// compares its record with the chain: a mismatch means somebody else filled the
// slot, and the desk publishes its own again instead of leaving the buyer with
// bytes no honest desk vouches for (`REVIEW.md` A06).
type deliveryRecord struct {
	OrderID          uint64 `json:"orderId"`
	CiphertextSha256 string `json:"ciphertextSha256"`
}

func deliveryRecordPath(outDir string, orderID uint64) string {
	return filepath.Join(outDir, fmt.Sprintf("delivered-%d.json", orderID))
}

// readDeliveryRecord returns the digest this desk last published for an order,
// when it has a record of one.
func readDeliveryRecord(outDir string, orderID uint64) (string, bool) {
	raw, err := os.ReadFile(deliveryRecordPath(outDir, orderID))
	if err != nil {
		return "", false
	}
	var record deliveryRecord
	if err := json.Unmarshal(raw, &record); err != nil || record.OrderID != orderID || record.CiphertextSha256 == "" {
		return "", false
	}
	return record.CiphertextSha256, true
}

func writeDeliveryRecord(outDir string, orderID uint64, digest string) error {
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", outDir, err)
	}
	return writeJSON(deliveryRecordPath(outDir, orderID), &deliveryRecord{OrderID: orderID, CiphertextSha256: digest})
}

// poolBeaconCalldata is the escrow's `beacon` argument: the bundle commitment.
func poolBeaconCalldata(commitmentHex string) ([]byte, error) {
	commitment, err := hex.DecodeString(strings.TrimPrefix(commitmentHex, "0x"))
	if err != nil || len(commitment) != 32 {
		return nil, fmt.Errorf("the bundle commitment is not 32 bytes of hex")
	}
	return commitment, nil
}

// serveAction is one piece of work a pass found. Deliveries come first so an
// existing buyer is served before the desk advertises the bundle again.
type serveAction struct {
	Kind    string
	Order   *poolOrderRecord
	Bundle  *servedBundle
	Listing *marketPoolListing
}

// planServePass turns the pool's records into the actions a desk holding the
// named bundles should take. Orders that are already settled, refunded, or past
// their deadline are left alone; keys are disclosed only while a later pass can
// still settle the order before the refund boundary, and a delivered order near
// its deadline is a candidate for the proof fallback when a prover is
// configured.
func planServePass(
	served map[string]servedBundle,
	listings map[string]marketPoolListing,
	orders []poolOrderRecord,
	head, refresh, proofMargin, revealMargin uint64,
	proverReady bool,
	// deskSender is the address the desk signs with, when it knows one. The
	// escrow accepts a beacon only from the bundle's registered author, so a
	// desk that is not the author leaves that bundle's beacon alone instead of
	// writing a call the node will refuse.
	deskSender string,
	// replacements names the delivered orders whose recorded ciphertext is not
	// this desk's. Those orders get a fresh delivery rather than waiting for a
	// buyer who cannot use what a stranger published.
	replacements map[uint64]bool,
) []serveAction {
	actions := make([]serveAction, 0)
	sorted := append([]poolOrderRecord(nil), orders...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Deadline != sorted[j].Deadline {
			return sorted[i].Deadline < sorted[j].Deadline
		}
		return sorted[i].ID < sorted[j].ID
	})
	for i := range sorted {
		order := &sorted[i]
		bundle, ok := served[order.Bundle]
		if !ok {
			continue
		}
		listing, hasListing := listings[order.Bundle]
		action := serveAction{Order: order, Bundle: &bundle}
		if hasListing {
			action.Listing = &listing
		}
		// A delivery hands the buyer the keys, so the desk may only disclose them
		// while it can still settle the order before the refund boundary: the
		// committed secrets must remain broadcastable and includable in a later
		// pass. A delivery that leaves no such pass is the documented
		// counterexample - the buyer decrypts and then refunds - so it is
		// withheld and the order falls to the desk's own settlement or to the
		// buyer's repayment.
		last := lastDisclosureHead(order.Deadline)
		switch order.State {
		case "funded":
			if head < last {
				action.Kind = "deliver"
				actions = append(actions, action)
				break
			}
			if revealMargin > 0 && head == last {
				action.Kind = "reveal"
				actions = append(actions, action)
			}
		case "delivered":
			// A delivery a stranger published is replaced while a later reveal
			// still fits; afterwards the committed secrets are the buyer's only
			// usable content, so it falls through to the settlement checks below.
			if replacements[order.ID] && head < last {
				action.Kind = "deliver"
				actions = append(actions, action)
				break
			}
			// A buyer who never confirms would otherwise take the refund and keep
			// the decrypted content. Inside the last blocks of the window the desk
			// publishes the committed secrets (`reveal`), which settles the order
			// without the buyer; the proof path below is preferred while it still
			// fits, because a reveal retires the bundle's remaining units (`AH-19`).
			if revealMargin > 0 && head <= last && order.Deadline-head <= revealMargin {
				action.Kind = "reveal"
				actions = append(actions, action)
				break
			}
			if proverReady && head <= last && order.Deadline-head <= proofMargin {
				action.Kind = "fulfill"
				actions = append(actions, action)
			}
		}
	}

	commitments := make([]string, 0, len(served))
	for commitment := range served {
		commitments = append(commitments, commitment)
	}
	sort.Strings(commitments)
	for _, commitment := range commitments {
		listing, ok := listings[commitment]
		if !ok {
			continue
		}
		if listing.BeaconBlock != 0 && head < listing.BeaconBlock+refresh {
			continue
		}
		bundle := served[commitment]
		if deskSender != "" && !strings.EqualFold(strings.TrimPrefix(listing.Author, "0x"), strings.TrimPrefix(deskSender, "0x")) {
			continue
		}
		actions = append(actions, serveAction{
			Kind:    "beacon",
			Bundle:  &bundle,
			Listing: &listing,
		})
	}
	return actions
}

// lastDisclosureHead is the last block at which the desk may broadcast the
// committed secrets for an order: an included call lands no earlier than
// head+1, and the escrow refuses a unilateral settlement included at the
// refund boundary, so the last usable broadcast head is deadline-2. The
// disclosure guard in `reveal.go` applies the same bound before signing.
func lastDisclosureHead(deadline uint64) uint64 {
	if deadline < 2 {
		return 0
	}
	return deadline - 2
}

// servePreparedAction is the in-memory prepared action the desk signs, or
// writes for `fod submit` when the caller did not ask for a broadcast.
func servePreparedAction(operation, format, contract, network string, calldata []byte, orderID uint64, bundle, buyer string) *preparedAction {
	return &preparedAction{
		Format:     format,
		Version:    actionRequestVersion,
		Operation:  operation,
		Network:    network,
		Contract:   strings.ToLower(strings.TrimPrefix(contract, "0x")),
		Amount:     "0",
		Calldata:   hex.EncodeToString(calldata),
		OfferID:    orderID,
		Commitment: bundle,
		Buyer:      buyer,
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// serveDesk is the running state: the vaults it serves, the node it reads, and
// (with -broadcast) the key it signs with.
type serveDesk struct {
	client       *client
	contract     string
	network      string
	served       map[string]servedBundle
	prover       string
	outDir       string
	broadcast    bool
	refresh      uint64
	proofMargin  uint64
	revealMargin uint64
	maxFee       *big.Int
	tips         *big.Int
	feePerGas    *big.Int
	identity     *ecdsa.PrivateKey
	sender       string
}

func (d *serveDesk) pass() error {
	// The head height decides what is due; the node's own block interval, when it
	// reports one, turns the block deadline into a budget a desk can act on.
	height, blockSeconds, err := registryHead(d.client)
	if err != nil {
		return err
	}
	poolRecords, err := readPrefix(d.client, "0x"+d.contract, marketPoolPrefix)
	if err != nil {
		return err
	}
	poolReserve, err := readCounter(d.client, "0x"+d.contract, marketReserveKey)
	if err != nil {
		return err
	}
	poolPending, err := readCounter(d.client, "0x"+d.contract, marketPendingKey)
	if err != nil {
		return err
	}
	poolUnits, err := readCounter(d.client, "0x"+d.contract, marketPoolUnitsKey)
	if err != nil {
		return err
	}
	listings := make(map[string]marketPoolListing, len(poolRecords))
	for key, raw := range poolRecords {
		listing, err := decodePoolRecord(key, raw, poolReserve, poolPending, poolUnits.Uint64())
		if err != nil {
			return err
		}
		listings[key] = *listing
	}
	orderRecords, err := readPrefix(d.client, "0x"+d.contract, marketPoolOrderPrefix)
	if err != nil {
		return err
	}
	orders := make([]poolOrderRecord, 0, len(orderRecords))
	for key, raw := range orderRecords {
		rawKey, err := hex.DecodeString(key)
		if err != nil {
			return fmt.Errorf("pool order key %q: %w", key, err)
		}
		id, err := strconv.ParseUint(string(rawKey), 10, 64)
		if err != nil {
			return fmt.Errorf("pool order key %q is not an id", key)
		}
		order, err := decodePoolOrderFull(id, raw)
		if err != nil {
			return err
		}
		orders = append(orders, *order)
	}

	replacements, err := d.foreignDeliveries(orders)
	if err != nil {
		return err
	}

	actions := planServePass(d.served, listings, orders, height, d.refresh, d.proofMargin, d.revealMargin, d.prover != "", d.sender, replacements)
	if len(actions) == 0 {
		fmt.Printf("head %d, nothing to do (%d served bundles, %d orders)\n", height, len(d.served), len(orders))
		return nil
	}
	for i := range actions {
		action := &actions[i]
		switch action.Kind {
		case "fulfill":
			fmt.Printf("budget      order %d: %s\n", action.Order.ID, settlementBudget(height, action.Order.Deadline, blockSeconds))
		case "reveal":
			fmt.Printf("boundary    order %d: no confirmation, publishing the committed secrets (%s)\n",
				action.Order.ID, settlementBudget(height, action.Order.Deadline, blockSeconds))
		}
		if err := d.runAction(action); err != nil {
			return err
		}
	}
	return nil
}

// foreignDeliveries names the delivered orders whose recorded ciphertext is not
// the one this desk published. The escrow lets any caller fill the slot, so a
// stranger can land bytes first; the desk notices the mismatch, publishes its
// own ciphertext again and records it. An order this desk has no record of - for
// instance one delivered before the record was introduced, or while the desk was
// offline - is treated the same way, which repairs it on the next pass.
func (d *serveDesk) foreignDeliveries(orders []poolOrderRecord) (map[uint64]bool, error) {
	out := make(map[uint64]bool)
	for _, order := range orders {
		if order.State != "delivered" {
			continue
		}
		raw, err := readRegistryValue(d.client, d.contract, fmt.Sprintf("%s%d", poolDeliveryStoragePrefix, order.ID))
		if err != nil {
			return nil, fmt.Errorf("read the delivery recorded for pool order %d: %w", order.ID, err)
		}
		digest := sha256.Sum256(raw)
		recorded, ok := readDeliveryRecord(d.outDir, order.ID)
		if !ok || !strings.EqualFold(recorded, hex.EncodeToString(digest[:])) {
			out[order.ID] = true
		}
	}
	return out, nil
}

// settlementBudget says how much time the chain's own blocks leave before an
// order's deadline. The delivery proof takes minutes to make, so a desk deciding
// whether the fallback still fits needs the budget in wall-clock terms, and it is
// read from the node rather than assumed: a node on a quiet or automined chain
// reports no interval, and then the budget is blocks alone.
func settlementBudget(head, deadline uint64, blockSeconds int64) string {
	if deadline <= head {
		return "the deadline has passed"
	}
	blocks := deadline - head
	if blockSeconds <= 0 {
		return fmt.Sprintf("%d blocks left; this node reports no block interval, so the wall-clock budget is unknown", blocks)
	}
	seconds := int64(blocks) * blockSeconds
	return fmt.Sprintf("%d blocks left, about %s at this node's %ds per block", blocks, budgetDuration(seconds), blockSeconds)
}

// budgetDuration renders a block budget the way a desk reads it: minutes under
// an hour, hours and minutes above.
func budgetDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	return fmt.Sprintf("%dh%02d", minutes/60, minutes%60)
}

func (d *serveDesk) runAction(action *serveAction) error {
	switch action.Kind {
	case "deliver":
		ciphertext, err := deliveryCiphertextFor(*action.Bundle, action.Order.DeliveryKey)
		if err != nil {
			return fmt.Errorf("order %d: %w", action.Order.ID, err)
		}
		digest := sha256.Sum256(ciphertext)
		if err := writeDeliveryRecord(d.outDir, action.Order.ID, hex.EncodeToString(digest[:])); err != nil {
			return fmt.Errorf("order %d: record the delivery: %w", action.Order.ID, err)
		}
		prepared := servePreparedAction(
			operationDeliver, preparedDeliverFormat, d.contract, d.network,
			poolDeliverCalldata(action.Order.ID, ciphertext),
			action.Order.ID, action.Bundle.Commitment, action.Order.Buyer,
		)
		return d.submit(prepared, fmt.Sprintf("deliver-%d", action.Order.ID))
	case "beacon":
		calldata, err := poolBeaconCalldata(action.Bundle.Commitment)
		if err != nil {
			return err
		}
		prepared := servePreparedAction(
			operationBeacon, preparedBeaconFormat, d.contract, d.network,
			calldata, 0, action.Bundle.Commitment, "",
		)
		return d.submit(prepared, "beacon-"+action.Bundle.Commitment[:8])
	case "fulfill":
		return d.runProof(action)
	case "reveal":
		// The buyer did not confirm and the deadline is close: publish the
		// committed secrets so the order settles instead of refunding. The
		// prepared action carries those secrets, so it is written owner-only
		// (`AH-19`, `AH-20`).
		secrets, err := revealSecrets(action.Bundle.Payload, action.Bundle.Commitment)
		if err != nil {
			return fmt.Errorf("order %d: %w", action.Order.ID, err)
		}
		prepared := buildRevealAction(d.contract, d.network, action.Order.ID, action.Bundle.Commitment, secrets)
		return d.submit(prepared, fmt.Sprintf("reveal-%d", action.Order.ID))
	}
	return fmt.Errorf("unknown serve action %q", action.Kind)
}

// runProof is the fallback for a delivered order whose buyer has not confirmed:
// prove the buyer-bound delivery with the prover, then submit `fulfill`.
// Order-specific proving takes minutes and cannot promise a three-block
// completion. Prefer the cheap in-window reveal; proofs need advance planning.
func (d *serveDesk) runProof(action *serveAction) error {
	if d.prover == "" {
		return errors.New("no prover is configured for the fallback")
	}
	if err := os.MkdirAll(d.outDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", d.outDir, err)
	}
	order, err := poolDeliveryOrderInput(d.contract, *action.Bundle, action.Order)
	if err != nil {
		return err
	}
	var anchor struct {
		StateRoot string `json:"stateRoot"`
	}
	if err := d.client.call("bcn_blockAt", []any{action.Order.FundedAt}, &anchor); err != nil {
		return err
	}
	order.FundingRoot = strings.TrimPrefix(strings.ToLower(anchor.StateRoot), "0x")
	if !isHex(order.FundingRoot, 32) {
		return errors.New("funding header has no state root")
	}
	orderJSON, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("encode the delivery order: %w", err)
	}
	proofPath := filepath.Join(d.outDir, fmt.Sprintf("proof-%d.json", action.Order.ID))
	if _, err := runDeliveryProver(d.prover, proofPath, orderJSON); err != nil {
		return err
	}
	artefact, err := readProverArtefact(proofPath)
	if err != nil {
		return err
	}
	request := &actionRequest{Version: actionRequestVersion, Network: d.network, Contract: d.contract}
	arguments := &fulfillArguments{
		FundingHeight: action.Order.FundedAt,
		FundingRoot:   order.FundingRoot,
		OfferID:       action.Order.ID,
		OrderNonce:    action.Order.Nonce,
		Seller:        action.Bundle.Author,
		Buyer:         action.Order.Buyer,
		DeliveryKey:   action.Order.DeliveryKey,
		Epoch:         action.Bundle.Epoch,
		Author:        action.Bundle.Author,
	}
	prepared, err := AssembleDelivery(request, arguments, artefact)
	if err != nil {
		return fmt.Errorf("the prover's artefact was refused: %w", err)
	}
	fulfil := &preparedAction{
		Format:      preparedFulfillFormat,
		Version:     prepared.Version,
		Operation:   operationFulfill,
		Network:     prepared.Network,
		Contract:    prepared.Contract,
		Amount:      prepared.Amount,
		Calldata:    prepared.Calldata,
		OfferID:     prepared.OfferID,
		OrderNonce:  prepared.OrderNonce,
		Seller:      prepared.Seller,
		Buyer:       prepared.Buyer,
		DeliveryKey: prepared.DeliveryKey,
		Proof:       prepared.Proof,
		PreparedAt:  prepared.PreparedAt,
	}
	return d.submit(fulfil, fmt.Sprintf("fulfill-%d", action.Order.ID))
}

// poolDeliveryOrderInput is the prover's view of one pool order. The journal it
// will produce names the bundle's author as the seller, because that is what
// `fulfill` compares the proof against; the desk's own address is not part
// of the statement.
func poolDeliveryOrderInput(contract string, bundle servedBundle, order *poolOrderRecord) (*deliveryOrderInput, error) {
	keys, err := EpochKeysFromVaultPayload(bundle.Payload)
	if err != nil {
		return nil, err
	}
	publicPart, privatePart := keys.SecretBytes()
	ephemeral, err := newEphemeralScalar()
	if err != nil {
		return nil, err
	}
	return &deliveryOrderInput{
		Network:           deliveryNetworkName,
		FundedAt:          order.FundedAt,
		Note:              fmt.Sprintf("pool delivery for order %d", order.ID),
		Contract:          contract,
		OfferID:           order.ID,
		OrderNonce:        order.Nonce,
		Seller:            bundle.Author,
		Buyer:             order.Buyer,
		BuyerKey:          order.DeliveryKey,
		Epoch:             bundle.Epoch,
		Author:            bundle.Author,
		PublicPartSecret:  hex.EncodeToString(publicPart),
		PrivatePartSecret: hex.EncodeToString(privatePart),
		Ephemeral:         hex.EncodeToString(ephemeral),
	}, nil
}

// submit signs and (when asked) broadcasts one prepared call. Without
// -broadcast it writes the action for `fod submit`, which is the same rule the
// rest of the CLI follows: nothing is sent unless the caller says so.
func (d *serveDesk) submit(prepared *preparedAction, label string) error {
	if !d.broadcast {
		if err := os.MkdirAll(d.outDir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", d.outDir, err)
		}
		path := filepath.Join(d.outDir, label+".json")
		// The reveal action carries the committed secrets; every other desk
		// action is public data that goes on chain anyway.
		writer := writeJSON
		if prepared.Format == preparedRevealFormat {
			writer = writePrivateJSON
		}
		if err := writer(path, prepared); err != nil {
			return err
		}
		fmt.Printf("prepared %s\n", path)
		return nil
	}
	if d.identity == nil {
		return errors.New("no signing key")
	}
	if err := validatePreparedDisclosure(d.client, prepared); err != nil {
		return err
	}
	nonce, epoch, rate, err := d.nextTransaction()
	if err != nil {
		return err
	}
	maxFee := d.maxFee
	if maxFee == nil {
		gas, ok := measuredGasFor(prepared.Operation)
		if !ok {
			return fmt.Errorf("no measured gas for %s: pass -max-fee", prepared.Operation)
		}
		maxFee = deskMaxFee(gas, rate)
	}
	raw, sender, err := BuildTransaction(prepared, d.identity, nonce, epoch, maxFee, d.tips)
	if err != nil {
		return err
	}
	if short := maxFeeShortfall(prepared.Operation, uint64(len(raw)), rate, maxFee); short != nil {
		maxFee = short
		raw, sender, err = BuildTransaction(prepared, d.identity, nonce, epoch, maxFee, d.tips)
		if err != nil {
			return err
		}
	}
	senderHex := strings.ToLower(strings.TrimPrefix(sender.Hex(), "0x"))
	if err := checkAccountAgainstChain(d.client, senderHex, nonce, epoch); err != nil {
		return err
	}
	if err := validatePreparedDisclosure(d.client, prepared); err != nil {
		return err
	}
	var hash string
	if err := d.client.call("bcn_sendRawTx", []any{"0x" + hex.EncodeToString(raw)}, &hash); err != nil {
		return fmt.Errorf("the node refused %s: %w", prepared.Operation, err)
	}
	fmt.Printf("%s %s at nonce %d, tx %s\n", prepared.Operation, label, nonce, hash)
	return d.waitForNonce(nonce)
}

// deskFeeHeadroomPercent is the margin the desk adds to the measured
// requirement when it signs without an operator watching. The table's gas is
// the highest figure measured so far, but the node adds a transaction that runs
// out of gas inside a proposal to its persistent blacklist, so the desk must not
// sign the exact floor.
const deskFeeHeadroomPercent = 25

// deskMaxFee is the measured floor plus the desk's headroom.
func deskMaxFee(gas operationGas, rate *big.Int) *big.Int {
	required := requiredMaxFeeFor(gas.WasmGas, gas.TxBytes, rate)
	margin := new(big.Int).Div(new(big.Int).Mul(required, big.NewInt(deskFeeHeadroomPercent)), big.NewInt(100))
	return required.Add(required, margin)
}

// nextTransaction reads the values a new transaction needs. The nonce is the
// account's nonce plus one, which is the smallest value the node accepts.
func (d *serveDesk) nextTransaction() (uint32, uint16, *big.Int, error) {
	var account struct {
		Nonce uint32 `json:"nonce"`
	}
	if err := d.client.call("dna_getBalance", []any{"0x" + d.sender}, &account); err != nil {
		return 0, 0, nil, fmt.Errorf("read the account: %w", err)
	}
	var chainEpoch struct {
		Epoch uint16 `json:"epoch"`
	}
	if err := d.client.call("dna_epoch", []any{}, &chainEpoch); err != nil {
		return 0, 0, nil, fmt.Errorf("read the chain epoch: %w", err)
	}
	rate := d.feePerGas
	if rate == nil {
		read, err := readFeePerGas(d.client)
		if err != nil {
			return 0, 0, nil, err
		}
		rate = read
	}
	return account.Nonce + 1, chainEpoch.Epoch, rate, nil
}

// waitForNonce keeps one transaction in flight at a time, so the next call the
// pass prepares cannot reuse a nonce the mempool already holds.
func (d *serveDesk) waitForNonce(sent uint32) error {
	deadline := time.Now().Add(serveNonceTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(serveNoncePoll)
		var account struct {
			Nonce uint32 `json:"nonce"`
		}
		if err := d.client.call("dna_getBalance", []any{"0x" + d.sender}, &account); err != nil {
			continue
		}
		if account.Nonce >= sent {
			return nil
		}
	}
	return fmt.Errorf("transaction %d was broadcast but the account nonce has not moved yet", sent)
}

// serveOptions is the CLI's view of one desk configuration.
type serveOptions struct {
	RPCURL       string
	APIKey       string
	Contract     string
	Network      string
	VaultFlag    string
	NodeKey      string
	Prover       string
	OutDir       string
	Broadcast    bool
	Once         bool
	IntervalSec  int
	Refresh      uint64
	ProofMargin  uint64
	RevealMargin uint64
	MaxFee       *big.Int
	Tips         *big.Int
	FeePerGas    *big.Int
}

func runServe(options serveOptions) error {
	contract := strings.ToLower(strings.TrimPrefix(options.Contract, "0x"))
	if !isHex(contract, addressBytes) {
		return errors.New("no valid contract address")
	}
	vaultPaths := make([]string, 0)
	for _, path := range strings.Split(options.VaultFlag, ",") {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			vaultPaths = append(vaultPaths, trimmed)
		}
	}
	if len(vaultPaths) == 0 {
		return errors.New("no vaults named")
	}
	if options.Broadcast && options.NodeKey == "" {
		return errors.New("-broadcast needs -nodekey")
	}
	if options.Refresh == 0 || options.Refresh >= beaconFreshnessBlocks {
		return fmt.Errorf("the beacon refresh must be between 1 and %d blocks", beaconFreshnessBlocks-1)
	}
	if options.IntervalSec <= 0 {
		return errors.New("the pass interval must be at least one second")
	}
	if options.RevealMargin == 0 {
		return errors.New("the reveal margin must be at least one block")
	}

	passphrase, err := readPassphrase("Vault passphrase: ")
	if err != nil {
		return err
	}
	payloads := make([]VaultPayload, 0, len(vaultPaths))
	for _, path := range vaultPaths {
		raw, err := ReadVault(path, passphrase)
		if err != nil {
			return fmt.Errorf("read vault %s: %w", path, err)
		}
		var payload VaultPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("parse vault %s: %w", path, err)
		}
		payloads = append(payloads, payload)
	}
	served, err := servedBundlesFromPayloads(payloads)
	if err != nil {
		return err
	}
	if len(served) == 0 {
		return errors.New("the vaults carry no epoch keys")
	}

	desk := &serveDesk{
		client:       &client{url: options.RPCURL, key: options.APIKey, http: newHTTPClient()},
		contract:     contract,
		network:      options.Network,
		served:       served,
		prover:       options.Prover,
		outDir:       options.OutDir,
		broadcast:    options.Broadcast,
		refresh:      options.Refresh,
		proofMargin:  options.ProofMargin,
		revealMargin: options.RevealMargin,
		maxFee:       options.MaxFee,
		tips:         options.Tips,
		feePerGas:    options.FeePerGas,
	}
	if options.Broadcast {
		identity, err := crypto.LoadECDSA(options.NodeKey)
		if err != nil {
			return fmt.Errorf("load nodekey: %w", err)
		}
		sender, err := nodekeyAddressOf(identity)
		if err != nil {
			return err
		}
		desk.identity = identity
		desk.sender = strings.ToLower(strings.TrimPrefix(sender, "0x"))
	}
	if !options.Broadcast {
		if err := os.MkdirAll(options.OutDir, 0o700); err != nil {
			return fmt.Errorf("create %s: %w", options.OutDir, err)
		}
	}

	fmt.Printf("desk        %s\n", contract)
	fmt.Printf("bundles     %d served\n", len(served))
	fmt.Printf("mode        %s\n", map[bool]string{true: "broadcast", false: "prepare only"}[options.Broadcast])
	if options.Prover == "" {
		fmt.Printf("fallback    reveal when an unconfirmed order has %d blocks or fewer left (no prover configured)\n", options.RevealMargin)
	} else {
		fmt.Printf("fallback    proof when an order has %d blocks or fewer left; reveal when %d or fewer\n",
			options.ProofMargin, options.RevealMargin)
	}
	fmt.Printf("delivery    only while a later reveal still fits before the refund boundary\n")
	interval := time.Duration(options.IntervalSec) * time.Second
	for {
		if err := desk.pass(); err != nil {
			if options.Once {
				return err
			}
			fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		}
		if options.Once {
			return nil
		}
		time.Sleep(interval)
	}
}
