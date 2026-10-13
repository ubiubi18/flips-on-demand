package main

// The buyer's side of a pool purchase: the calldata `buy` takes, with the
// delivery-key possession proof over the pool's own context - the bundle and the
// buy nonce rather than an offer id (`contracts/escrow/src/lib.rs`,
// `pool_delivery_key_message`).

import (
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/idena-network/idena-go/crypto"
)

// poolBuyDomain is the escrow's `DELIVERY_KEY_DOMAIN`.
const poolBuyDomain = "fod:delivery-key:v1"

// operationBuy is the escrow export the prepared file signs.
const operationBuy = "buy"

// / The bundle a pool buy names is the 32-byte key commitment.
const poolBundleBytes = 32

// feePerItem is the escrow's `FEE_PER_ITEM`: the retrieval-fee cap is the
// authenticated covered count times this value, and never bytes.
const feePerItem = "1000000000000000000"

// buyFeeArgument is one retrieval host the buyer names, with the amount the
// escrow credits it on settlement.
type buyFeeArgument struct {
	Host   string `json:"host"`
	Amount string `json:"amount"`
}

// PoolBuy is the artefact `prepare-buy` writes.
type PoolBuy struct {
	Format      string `json:"format"`
	Version     int    `json:"version"`
	Operation   string `json:"operation"`
	Network     string `json:"network"`
	Contract    string `json:"contract"`
	Bundle      string `json:"bundle"`
	Nonce       uint64 `json:"nonce"`
	Buyer       string `json:"buyer"`
	DeliveryKey string `json:"deliveryKey"`
	Amount      string `json:"amount"`
	Calldata    string `json:"calldata"`
	Note        string `json:"note"`
	// Availability is what this run checked about the bundle's content before
	// anything was signed (`SPEC.md` 5.8): the seller's advisory repeated by the
	// buyer's own node, or an explicit `unchecked` record when no advisory was
	// named. It is the buyer's observation, not the seller's claim, and its
	// binding says which of the two it is.
	Availability *AvailabilityCheck `json:"availability,omitempty"`
	Fast         *fastBuyEvidence   `json:"fast,omitempty"`
}

// poolBuyMessage mirrors the escrow's hashed context: the domain, this contract,
// the bundle, the buy nonce, the paying caller, and the delivery key.
func poolBuyMessage(contract, bundle, buyer, deliveryKey []byte, nonce uint64) []byte {
	message := make([]byte, 0, len(poolBuyDomain)+addressBytes+poolBundleBytes+8+addressBytes+deliveryKeyBytes)
	message = append(message, []byte(poolBuyDomain)...)
	message = append(message, contract...)
	message = append(message, bundle...)
	message = append(message, u64be(nonce)...)
	message = append(message, buyer...)
	message = append(message, deliveryKey...)
	return message
}

// preparePoolBuy builds the buy calldata: the bundle, the delivery key, and the
// possession proof the pool recovers.
func preparePoolBuy(
	contractHex, bundleHex, buyerHex string,
	nonce uint64,
	quote, network string,
	fees []buyFeeArgument,
	capItems uint64,
	deliveryKey *ecdsa.PrivateKey,
) (*PoolBuy, error) {
	contract, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(contractHex), "0x"))
	if err != nil || len(contract) != addressBytes {
		return nil, fmt.Errorf("the contract address is not 20 bytes of hex")
	}
	bundle, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(bundleHex), "0x"))
	if err != nil || len(bundle) != poolBundleBytes {
		return nil, fmt.Errorf("the bundle commitment is not 32 bytes of hex")
	}
	buyer, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(buyerHex), "0x"))
	if err != nil || len(buyer) != addressBytes {
		return nil, fmt.Errorf("the buyer address is not 20 bytes of hex")
	}
	quoteValue, ok := new(big.Int).SetString(quote, 10)
	if !ok || quoteValue.Sign() <= 0 {
		return nil, fmt.Errorf("the amount %q is not a positive integer", quote)
	}
	// The retrieval-fee list: the total is capped by the authenticated covered
	// count, never by bytes, and the buyer's signed amount is quote plus fees.
	perItem, ok := new(big.Int).SetString(feePerItem, 10)
	if !ok {
		return nil, fmt.Errorf("the fee cap is not an integer")
	}
	cap := new(big.Int).Mul(perItem, new(big.Int).SetUint64(capItems))
	feeTotal := new(big.Int)
	feeBytes := make([]byte, 0, len(fees)*(20+16))
	for _, fee := range fees {
		host, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(fee.Host), "0x"))
		if err != nil || len(host) != addressBytes {
			return nil, fmt.Errorf("the retrieval host %q is not 20 bytes of hex", fee.Host)
		}
		amount, ok := new(big.Int).SetString(fee.Amount, 10)
		if !ok || amount.Sign() <= 0 || amount.BitLen() > 128 {
			return nil, fmt.Errorf("the retrieval fee %q is not a positive integer", fee.Amount)
		}
		feeTotal.Add(feeTotal, amount)
		feeBytes = append(feeBytes, host...)
		amountBytes := make([]byte, 16)
		amount.FillBytes(amountBytes)
		feeBytes = append(feeBytes, amountBytes...)
	}
	if feeTotal.Cmp(cap) > 0 {
		return nil, fmt.Errorf("the retrieval fee %s is above the cap %s for %d covered items", feeTotal, cap, capItems)
	}
	total := new(big.Int).Add(quoteValue, feeTotal)
	if len(fees) > 16 {
		return nil, fmt.Errorf("%d retrieval hosts, at most 16 are accepted", len(fees))
	}
	if deliveryKey == nil {
		return nil, fmt.Errorf("no delivery key")
	}
	publicKey := crypto.FromECDSAPub(&deliveryKey.PublicKey)
	if len(publicKey) != deliveryKeyBytes {
		return nil, fmt.Errorf("the delivery public key is %d bytes, expected %d", len(publicKey), deliveryKeyBytes)
	}
	signature, err := crypto.Sign(
		crypto.Keccak256(poolBuyMessage(contract, bundle, buyer, publicKey, nonce)),
		deliveryKey,
	)
	if err != nil {
		return nil, fmt.Errorf("sign the possession proof: %w", err)
	}
	if len(signature) != signatureBytes {
		return nil, fmt.Errorf("the signature is %d bytes, expected %d", len(signature), signatureBytes)
	}
	calldata := make([]byte, 0, poolBundleBytes+deliveryKeyBytes+signatureBytes+2+len(feeBytes))
	calldata = append(calldata, bundle...)
	calldata = append(calldata, publicKey...)
	calldata = append(calldata, signature...)
	calldata = append(calldata, byte(len(fees)>>8), byte(len(fees)))
	calldata = append(calldata, feeBytes...)

	return &PoolBuy{
		Format:      "fod.prepared-buy",
		Version:     1,
		Operation:   operationBuy,
		Network:     network,
		Contract:    strings.ToLower(strings.TrimPrefix(contractHex, "0x")),
		Bundle:      hex.EncodeToString(bundle),
		Nonce:       nonce,
		Buyer:       hex.EncodeToString(buyer),
		DeliveryKey: hex.EncodeToString(publicKey),
		Amount:      total.String(),
		Calldata:    hex.EncodeToString(calldata),
		Note:        "the call sends exactly Amount; the pool refuses any other figure, and the possession proof is bound to this bundle and nonce",
	}, nil
}

func runPrepareBuy(contractHex, bundleHex string, nonce uint64, amount, network, nodeKeyPath, deliveryKeyPath, preparedPath string) {
	if contractHex == "" || bundleHex == "" || amount == "0" || network == "" || nodeKeyPath == "" || deliveryKeyPath == "" {
		fmt.Fprintln(os.Stderr, "prepare-buy needs -contract ADDRESS -pool-bundle COMMITMENT -amount BASE_UNITS -network LABEL -nodekey FILE -delivery-key FILE")
		os.Exit(2)
	}
	identity, err := crypto.LoadECDSA(nodeKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load nodekey: %v\n", err)
		os.Exit(1)
	}
	buyer, err := nodekeyAddressOf(identity)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	deliveryKey, err := loadDeliveryKey(deliveryKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load delivery key: %v\n", err)
		os.Exit(1)
	}
	artefact, err := preparePoolBuy(contractHex, bundleHex, buyer, nonce, amount, network, nil, 0, deliveryKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "action refused: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSON(preparedPath, artefact); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
		os.Exit(1)
	}
	fmt.Printf("calldata    %d bytes\n", len(artefact.Calldata)/2)
	fmt.Printf("buyer       %s\n", artefact.Buyer)
	fmt.Printf("prepared    %s\n", preparedPath)
	fmt.Println("not submitted: pass this file to `fod submit` to sign it; the buyer's nodekey sends it")
}

// buyRequestArguments is the interface's pool buy request (`SPEC.md` 4.5).
type buyRequestArguments struct {
	BundleID      string `json:"bundleId"`
	ExpectedQuote string `json:"expectedQuote"`
	Buyer         string `json:"buyer"`
	// Fees names the retrieval hosts and the amounts the buyer pays them; the
	// escrow holds the money with the purchase until settlement and returns it
	// on timeout.
	Fees []buyFeeArgument `json:"fees,omitempty"`
}

// loadBuyRequest reads the interface's buy request.
func loadBuyRequest(path string) (*actionRequest, *buyRequestArguments, error) {
	request, err := readActionRequest(path)
	if err != nil {
		return nil, nil, err
	}
	if request.Operation != operationBuy {
		return nil, nil, fmt.Errorf("unsupported operation %q", request.Operation)
	}
	var arguments buyRequestArguments
	if err := decodeStrict(request.Arguments, &arguments); err != nil {
		return nil, nil, fmt.Errorf("read request arguments: %w", err)
	}
	if !isHex(strings.ToLower(strings.TrimPrefix(arguments.BundleID, "0x")), poolBundleBytes) {
		return nil, nil, errors.New("the request names no valid bundle")
	}
	if !isDecimal(arguments.ExpectedQuote) {
		return nil, nil, errors.New("the request quote is not an integer in base units")
	}
	if !isHex(strings.ToLower(strings.TrimPrefix(arguments.Buyer, "0x")), addressBytes) {
		return nil, nil, errors.New("the request names no valid buyer address")
	}
	// The signed amount is the quote plus the capped retrieval fees, so a
	// request that names hosts has to carry the larger figure.
	feeTotal := new(big.Int)
	for _, fee := range arguments.Fees {
		value, ok := new(big.Int).SetString(fee.Amount, 10)
		if !ok || value.Sign() <= 0 {
			return nil, nil, fmt.Errorf("the retrieval fee %q is not a positive integer", fee.Amount)
		}
		feeTotal.Add(feeTotal, value)
	}
	quote, _ := new(big.Int).SetString(arguments.ExpectedQuote, 10)
	due := new(big.Int).Add(quote, feeTotal)
	if request.Amount != due.String() {
		return nil, nil, fmt.Errorf("the request amount %s and its quote plus fees %s disagree", request.Amount, due)
	}
	return request, &arguments, nil
}

// preparePoolBuyFromRequest rechecks the interface's quote against the pool and
// builds the buy call. The nonce and the amount come from the chain, never from
// the snapshot: a stale snapshot must not price or nonce a payment.
//
// availabilityPath names the seller's availability advisory and is required:
// the buyer's tool checks the advisory against the chain - the commitment
// recomputed over its CIDs has to equal the inventory commitment the
// registration statement published, and every CID has to fetch from the buyer's
// own node and rebuild itself - before it builds the call. Either check failing
// refuses the purchase, and the signing side refuses a buy file without a
// recent, complete, chain-bound check (`AH-17`).
func preparePoolBuyFromRequest(c *client, request *actionRequest, arguments *buyRequestArguments, identity *ecdsa.PrivateKey, deliveryKey *ecdsa.PrivateKey, availabilityPath string) (*PoolBuy, error) {
	buyer, err := nodekeyAddressOf(identity)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(buyer, strings.TrimPrefix(arguments.Buyer, "0x")) {
		return nil, fmt.Errorf("the request names buyer %s, and the nodekey controls %s", arguments.Buyer, buyer)
	}
	listing, err := registryPoolBundle(c, request.Contract, arguments.BundleID)
	if err != nil {
		return nil, err
	}
	fast, err := registryHasFastBundle(c, request.Contract, arguments.BundleID)
	if err != nil {
		return nil, err
	}
	if listing.BuyQuote == "" {
		return nil, fmt.Errorf("the pool has no buy quote for bundle %s: it needs at least two units", arguments.BundleID)
	}
	if listing.BuyQuote != arguments.ExpectedQuote {
		return nil, fmt.Errorf("the pool now quotes %s for bundle %s, and the request expected %s: refresh the snapshot and request again",
			listing.BuyQuote, arguments.BundleID, arguments.ExpectedQuote)
	}
	openOrders, err := registryOpenPoolOrderCount(c, request.Contract, arguments.BundleID)
	if err != nil {
		return nil, err
	}
	if openOrders == maxOpenPoolOrders {
		return nil, fmt.Errorf("bundle %s has %d open purchases; wait for settlement or refund before buying", arguments.BundleID, openOrders)
	}
	if availabilityPath == "" {
		return nil, fmt.Errorf(
			"no content evidence was given: pass an availability advisory or a sealed fast proof as -availability")
	}
	if fast {
		return prepareFastPoolBuyFromRequest(c, request, arguments, listing, identity, deliveryKey, availabilityPath)
	}
	var check *AvailabilityCheck
	{
		advisory, err := readAvailability(availabilityPath)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(strings.TrimPrefix(advisory.Author, "0x"), strings.TrimPrefix(listing.Author, "0x")) {
			return nil, fmt.Errorf("the advisory %s names author %s, and bundle %s belongs to %s",
				availabilityPath, advisory.Author, arguments.BundleID, listing.Author)
		}
		// The statement publishes the commitment over the bundle's CIDs, so the
		// advisory is checked against chain state rather than believed: the same
		// value recomputed from the advisory's own CIDs has to match it. A
		// substituted, omitted, duplicated or wrong-epoch entry changes the
		// value, and a bundle registered before the statement published the
		// field carries zeros and cannot be checked at all. The registration
		// guest hashes each CID as the multibase text the node writes
		// (`manifest_cids` in `proofs/registration-v2/host`), so this commits
		// the canonical text after rejecting a non-canonical spelling - a
		// commitment over the decoded bytes is a different value that no
		// registered bundle carries.
		inventory := make([]string, 0, len(advisory.Flips))
		for _, flip := range advisory.Flips {
			decoded, err := cidBytesFromString(flip.CID)
			if err != nil {
				return nil, fmt.Errorf("the advisory %s names an entry this build cannot commit: %w", availabilityPath, err)
			}
			inventory = append(inventory, cidString(decoded))
		}
		if strings.Trim(strings.TrimPrefix(strings.ToLower(listing.InventoryCommitment), "0x"), "0") == "" {
			return nil, fmt.Errorf(
				"bundle %s carries no inventory commitment: it was registered before the statement published one, so its content cannot be checked against the chain",
				arguments.BundleID)
		}
		commitment := inventoryCommitment(advisory.Author, advisory.Epoch, inventory)
		if !strings.EqualFold(commitment, listing.InventoryCommitment) {
			return nil, fmt.Errorf(
				"the advisory %s does not describe bundle %s: its %d CIDs commit to %s and the chain holds %s",
				availabilityPath, arguments.BundleID, len(inventory), commitment, listing.InventoryCommitment)
		}
		check = recheckAvailability(advisory, oneSource(ipfsFetch(c), "the buyer's node"), nodeCIDComputer(c))
		check.File = availabilityPath
		check.Binding = bindingCommitted
		check.InventoryCommitment = listing.InventoryCommitment
		if len(advisory.Flips) != int(listing.FlipCount) {
			return nil, errors.New("the retained inventory count does not match the authenticated covered count")
		}
		if !check.Complete() {
			return nil, fmt.Errorf("the content of bundle %s did not check out at %s: %d of %d flips verified, %s",
				arguments.BundleID, check.CheckedAt, check.VerifiedFlips, check.CoveredFlips, strings.Join(check.Failures, "; "))
		}
	}
	artefact, err := preparePoolBuy(request.Contract, arguments.BundleID, buyer, listing.Nonce, listing.BuyQuote, request.Network, arguments.Fees, uint64(listing.FlipCount), deliveryKey)
	if err != nil {
		return nil, err
	}
	artefact.Availability = check
	return artefact, nil
}

// validatePreparedBuy re-reads canonical terms immediately before signing. The
// JSON's counters, commitment and quote are claims, never their own authority.
func validatePreparedBuy(c *client, p *preparedAction) error {
	p.buyValidated = false
	if err := requireCheckedAvailability(p.Availability, time.Now()); err != nil {
		return err
	}
	listing, err := registryPoolBundle(c, p.Contract, p.Bundle)
	if err != nil {
		return err
	}
	fast, err := registryHasFastBundle(c, p.Contract, p.Bundle)
	if err != nil {
		return err
	}
	if listing.Units == 0 || listing.Strikes >= 2 || listing.Nonce != p.Nonce || listing.BuyQuote == "" {
		return errors.New("the prepared purchase no longer names a saleable bundle and current nonce")
	}
	openOrders, err := registryOpenPoolOrderCount(c, p.Contract, p.Bundle)
	if err != nil {
		return err
	}
	if openOrders == maxOpenPoolOrders {
		return errors.New("the bundle has eight open purchases; wait for settlement or refund before signing")
	}
	check := p.Availability
	if !strings.EqualFold(strings.TrimPrefix(check.Author, "0x"), listing.Author) ||
		check.InventoryCommitment != listing.InventoryCommitment || check.CoveredFlips != int(listing.FlipCount) {
		return errors.New("the retained inventory is not the inventory the chain authenticates for this bundle")
	}
	if fast {
		if err := validateFastBuyEvidence(c, p, listing); err != nil {
			return err
		}
	} else if p.Fast != nil {
		return errors.New("ordinary purchase carries unexpected fast copy evidence")
	}
	calldata, err := hex.DecodeString(p.Calldata)
	const prefix = poolBundleBytes + deliveryKeyBytes + signatureBytes
	if err != nil || len(calldata) < prefix+2 || hex.EncodeToString(calldata[:poolBundleBytes]) != p.Bundle ||
		hex.EncodeToString(calldata[poolBundleBytes:poolBundleBytes+deliveryKeyBytes]) != p.DeliveryKey {
		return errors.New("the purchase calldata does not match its bundle and delivery key")
	}
	fees := int(binary.BigEndian.Uint16(calldata[prefix : prefix+2]))
	if fees > 16 || len(calldata) != prefix+2+fees*(addressBytes+16) {
		return errors.New("invalid retrieval fee encoding")
	}
	feeTotal := new(big.Int)
	for offset := prefix + 2; offset < len(calldata); offset += addressBytes + 16 {
		fee := new(big.Int).SetBytes(calldata[offset+addressBytes : offset+addressBytes+16])
		if fee.Sign() <= 0 {
			return errors.New("invalid retrieval fee")
		}
		feeTotal.Add(feeTotal, fee)
	}
	cap, _ := new(big.Int).SetString(feePerItem, 10)
	cap.Mul(cap, new(big.Int).SetUint64(uint64(listing.FlipCount)))
	quote, _ := new(big.Int).SetString(listing.BuyQuote, 10)
	if feeTotal.Cmp(cap) > 0 || new(big.Int).Add(quote, feeTotal).String() != p.Amount {
		return errors.New("the signed amount is not the current quote plus capped retrieval fees")
	}
	contract, _ := hex.DecodeString(p.Contract)
	buyer, err := hex.DecodeString(p.Buyer)
	if err != nil || len(buyer) != addressBytes {
		return errors.New("invalid purchase buyer")
	}
	key := calldata[poolBundleBytes : poolBundleBytes+deliveryKeyBytes]
	sig := calldata[poolBundleBytes+deliveryKeyBytes : prefix]
	recovered, err := crypto.SigToPub(crypto.Keccak256(poolBuyMessage(contract, calldata[:poolBundleBytes], buyer, key, p.Nonce)), sig)
	if err != nil || !bytesEqual(crypto.FromECDSAPub(recovered), key) {
		return errors.New("invalid purchase context proof")
	}
	p.buyValidated = true
	return nil
}

// runPrepareBuyRequest is the interface-driven form of `prepare-buy`: the
// request names the bundle, the quote, and the buyer; the chain supplies the
// nonce and the quote the call has to send, and a missing delivery key is
// created here.
func runPrepareBuyRequest(c *client, requestPath, nodeKeyPath, deliveryKeyPath, archivePath, preparedPath, availabilityPath, approvedContract, approvedCodeHash string) {
	request, arguments, err := loadBuyRequest(requestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "buy request refused: %v\n", err)
		os.Exit(1)
	}
	if err := checkBuyTarget(c, request.Contract, approvedContract, approvedCodeHash); err != nil {
		fmt.Fprintf(os.Stderr, "buy target refused: %v\n", err)
		os.Exit(1)
	}
	identity, err := crypto.LoadECDSA(nodeKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load nodekey: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stat(deliveryKeyPath); errors.Is(err, os.ErrNotExist) {
		publicKey, err := CreateDeliveryKey(deliveryKeyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create delivery key: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("delivery key created at %s (public half %s)\n", deliveryKeyPath, publicKey)
		fmt.Println("note        keep this file: a delivery encrypted to it counts as delivered, and losing it is not a refund condition")
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "read delivery key path: %v\n", err)
		os.Exit(1)
	}
	if archivePath != "" {
		key, err := loadDeliveryKey(deliveryKeyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load delivery key: %v\n", err)
			os.Exit(1)
		}
		passphrase, err := confirmPassphrase()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if err := WriteDeliveryKeyArchive(archivePath, passphrase, key); err != nil {
			fmt.Fprintf(os.Stderr, "write the delivery-key archive: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("delivery-key archive written to %s (owner-only)\n", archivePath)
	}
	deliveryKey, err := loadDeliveryKey(deliveryKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load delivery key: %v\n", err)
		os.Exit(1)
	}
	artefact, err := preparePoolBuyFromRequest(c, request, arguments, identity, deliveryKey, availabilityPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "action refused: %v\n", err)
		os.Exit(1)
	}
	if err := writePrivateJSON(preparedPath, artefact); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
		os.Exit(1)
	}
	fmt.Printf("operation   %s on %s\n", artefact.Operation, artefact.Network)
	fmt.Printf("contract    %s\n", artefact.Contract)
	fmt.Printf("bundle      %s\n", artefact.Bundle)
	fmt.Printf("nonce       %d, amount %s base units\n", artefact.Nonce, artefact.Amount)
	fmt.Printf("buyer       %s\n", artefact.Buyer)
	fmt.Printf("calldata    %d bytes\n", len(artefact.Calldata)/2)
	if artefact.Availability.Binding == bindingCommitted {
		fmt.Printf("checked     %d/%d flips fetched and rebuilt by this node at %s, and the advisory's CIDs match the bundle's committed inventory\n",
			artefact.Availability.VerifiedFlips, artefact.Availability.CoveredFlips, artefact.Availability.CheckedAt)
	} else {
		fmt.Println("checked     nothing: no -availability advisory was named, so this call was built without fetching the bundle's content")
	}
	fmt.Printf("prepared    %s\n", preparedPath)
	fmt.Println("not submitted: pass this file to `fod submit` to sign it; the buyer's nodekey sends it")
}
