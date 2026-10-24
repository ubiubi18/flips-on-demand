package main

// The author's non-cooperative settlement (`SPEC.md` 7.2 and 10.3): publish the
// exact secrets the registration committed, so an order that was delivered
// settles without the buyer's confirmation and a withholding buyer cannot take
// the refund instead.
//
// The bundle record is keyed by `sha256(secrets)`, so the escrow needs one hash
// check - no proof and no trusted signer - and every outcome is fixed by the
// registration: the purchase amount goes to the bundle's registered author, the
// retrieval fee to the hosts the buyer named, and the caller receives nothing.
// Only the vault's epoch keys can build the call, and publishing them is the
// author's own choice; the command says so before it writes anything.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	operationReveal      = "reveal"
	preparedRevealFormat = "fod.prepared-reveal"
)

// Refuse disclosure-bearing calls when even the next inclusion block is too
// late. This is a local containment check, not a guarantee of inclusion or
// mempool confidentiality. Call it again immediately before broadcasting.
func validatePreparedDisclosure(c *client, prepared *preparedAction) error {
	if isBondedV8Format(prepared.Format) {
		return nil
	} // Validated by the v8 adapter; ciphertext only.
	switch prepared.Operation {
	case operationDeliver, operationReveal, operationFulfill, operationFulfillFast:
	default:
		return nil
	}
	calldata, err := hex.DecodeString(prepared.Calldata)
	if err != nil || len(calldata) <= 8 || !isHex(prepared.Contract, addressBytes) {
		return fmt.Errorf("malformed disclosure call")
	}
	id := binary.BigEndian.Uint64(calldata[:8])
	if id == 0 || prepared.OfferID != id || prepared.Amount != "0" {
		return fmt.Errorf("disclosure context does not name this order and zero payment")
	}
	raw, err := readRegistryValue(c, prepared.Contract, fmt.Sprintf("%s%d", marketPoolOrderPrefix, id))
	if err != nil {
		return err
	}
	order, err := decodePoolOrderFull(id, raw)
	if err != nil {
		return err
	}
	if order.State != "funded" && order.State != "delivered" {
		return fmt.Errorf("order %d is %s: refusing a disclosure call", id, order.State)
	}
	if prepared.Operation == operationFulfillFast {
		if len(calldata) != 8+196 || order.State != "funded" || prepared.Bundle != order.Bundle ||
			prepared.OrderNonce != order.Nonce || prepared.Buyer != order.Buyer || prepared.DeliveryKey != order.DeliveryKey ||
			prepared.Proof != hex.EncodeToString(calldata[8:]) {
			return errors.New("fast delivery differs from the funded order")
		}
		listing, err := registryPoolBundle(c, prepared.Contract, order.Bundle)
		if err != nil {
			return err
		}
		if listing.Author != prepared.Seller {
			return errors.New("fast delivery names another author")
		}
		if _, err := registryFastBundle(c, prepared.Contract, order.Bundle); err != nil {
			return err
		}
	}
	orders := []*poolOrderRecord{order}
	if prepared.Operation == operationReveal {
		digest := sha256.Sum256(calldata[8:])
		if hex.EncodeToString(digest[:]) != order.Bundle || prepared.Bundle != order.Bundle {
			return fmt.Errorf("the revealed keys do not belong to this order")
		}
		strikes, err := registryPoolBundleStrikes(c, prepared.Contract, order.Bundle)
		if err != nil {
			return err
		}
		if strikes > 0 {
			return fmt.Errorf("bundle has a prior refund: refusing public key disclosure")
		}
		openIDs, err := registryOpenPoolOrderIDs(c, prepared.Contract, order.Bundle)
		if err != nil {
			return err
		}
		orders = orders[:0]
		for _, openID := range openIDs {
			candidate := order
			if openID != id {
				raw, err := readRegistryValue(c, prepared.Contract, fmt.Sprintf("%s%d", marketPoolOrderPrefix, openID))
				if err != nil {
					return err
				}
				candidate, err = decodePoolOrderFull(openID, raw)
				if err != nil {
					return err
				}
			}
			if candidate.Bundle != order.Bundle || (candidate.State != "funded" && candidate.State != "delivered") {
				return fmt.Errorf("order %d is not open on this bundle: refusing public key disclosure", openID)
			}
			orders = append(orders, candidate)
		}
		found := false
		for _, candidate := range orders {
			if candidate.ID == id {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("order %d is absent from the open set: refusing public key disclosure", id)
		}
	}
	var head struct {
		Height uint64 `json:"height"`
	}
	if err := c.call("bcn_lastBlock", []any{}, &head); err != nil {
		return err
	}
	// The reported head has already executed; a newly submitted call can enter
	// no earlier than head+1, and the escrow refuses a unilateral settlement
	// included at the refund boundary. The plan and this guard share the same
	// last usable broadcast head (`lastDisclosureHead` in `serve.go`), so a
	// planned reveal is never refused here for the head alone.
	for _, candidate := range orders {
		if candidate.Deadline <= candidate.FundedAt || head.Height < candidate.FundedAt || head.Height > lastDisclosureHead(candidate.Deadline) {
			return fmt.Errorf("no remaining inclusion block before order %d's refund boundary %d; refusing key disclosure", candidate.ID, candidate.Deadline)
		}
	}
	return nil
}

// revealCalldata is the escrow's argument: the order id, then the canonical
// secrets (`contracts/escrow/src/lib.rs`, `reveal`).
func revealCalldata(orderID uint64, secrets []byte) string {
	arg := make([]byte, 0, 8+len(secrets))
	arg = binary.BigEndian.AppendUint64(arg, orderID)
	arg = append(arg, secrets...)
	return hex.EncodeToString(arg)
}

// revealSecrets re-encodes the vault's epoch keys and refuses unless the bytes
// hash to the bundle the order names - the same check the escrow makes, so a
// vault that cannot settle the order is refused before a fee is spent.
func revealSecrets(payload VaultPayload, bundleHex string) ([]byte, error) {
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
	secrets, err := SecretsCanonicalBytes(payload.Epoch, author, publicPart, privatePart)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(secrets)
	want := strings.ToLower(strings.TrimPrefix(bundleHex, "0x"))
	if hex.EncodeToString(digest[:]) != want {
		return nil, fmt.Errorf("this vault's secrets do not hash to the order's bundle %s", want)
	}
	return secrets, nil
}

// buildRevealAction writes the call `fod submit` signs.
func buildRevealAction(contract, network string, orderID uint64, bundle string, secrets []byte) *preparedAction {
	return &preparedAction{
		Format:    preparedRevealFormat,
		Version:   actionRequestVersion,
		Operation: operationReveal,
		Network:   network,
		Contract:  strings.ToLower(strings.TrimPrefix(contract, "0x")),
		Amount:    "0",
		Calldata:  revealCalldata(orderID, secrets),
		OfferID:   orderID,
		Bundle:    strings.ToLower(strings.TrimPrefix(bundle, "0x")),
		Note: "publishes the committed secrets: settlement pays the registered author and named hosts " +
			"for every open order on the bundle and refuses their refunds afterwards",
		PreparedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// runReveal reads the order, finds the committed secrets in the author's vault
// and writes the prepared call.
func runReveal(c *client, contract, network string, orderID uint64, vaultPath, preparedPath string) {
	contractHex := strings.ToLower(strings.TrimPrefix(contract, "0x"))
	if !isHex(contractHex, addressBytes) {
		fmt.Fprintln(os.Stderr, "reveal needs a 20-byte contract address")
		os.Exit(2)
	}
	if orderID == 0 || vaultPath == "" || preparedPath == "" {
		fmt.Fprintln(os.Stderr, "reveal needs -contract ADDRESS -pool-order ID -vault FILE -prepared FILE")
		os.Exit(2)
	}
	raw, err := readRegistryValue(c, contractHex, fmt.Sprintf("%s%d", marketPoolOrderPrefix, orderID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "reveal refused: %v\n", err)
		os.Exit(1)
	}
	order, err := decodePoolOrderFull(orderID, raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reveal refused: %v\n", err)
		os.Exit(1)
	}
	if order.State != "funded" && order.State != "delivered" {
		fmt.Fprintf(os.Stderr, "reveal refused: order %d is %s\n", orderID, order.State)
		os.Exit(1)
	}
	passphrase, err := readPassphrase("Vault passphrase: ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "reveal refused: %v\n", err)
		os.Exit(1)
	}
	vaultRaw, err := ReadVault(vaultPath, passphrase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reveal refused: read vault %s: %v\n", vaultPath, err)
		os.Exit(1)
	}
	var payload VaultPayload
	if err := json.Unmarshal(vaultRaw, &payload); err != nil {
		fmt.Fprintf(os.Stderr, "reveal refused: parse vault %s: %v\n", vaultPath, err)
		os.Exit(1)
	}
	secrets, err := revealSecrets(payload, order.Bundle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reveal refused: %v\n", err)
		os.Exit(1)
	}
	prepared := buildRevealAction(contract, network, orderID, order.Bundle, secrets)
	if err := writeRevealAction(preparedPath, prepared); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", preparedPath, err)
		os.Exit(1)
	}
	fmt.Printf("order       %d %s, bundle %s\n", order.ID, order.State, order.Bundle)
	fmt.Printf("publishes   %d bytes of committed secrets; sha256 matches the bundle\n", len(secrets))
	fmt.Printf("prepared    %s\n", preparedPath)
	fmt.Println("not submitted: pass this file to `fod submit` to sign it; the author's nodekey sends it")
}

// writeRevealAction writes the prepared call the way key material has to be
// written: the calldata carries the committed secrets, so the file is owner-only
// even when it already existed with wider permissions.
func writeRevealAction(path string, prepared *preparedAction) error {
	return writePrivateJSON(path, prepared)
}
