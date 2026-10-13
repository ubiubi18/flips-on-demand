package main

// The buyer's own repeat of the availability check (`SPEC.md` 5.8): the
// advisory is the seller's claim, and before a purchase is prepared the buyer's
// tool fetches every CID the claim names from its own node and rebuilds each
// CID from the bytes. A purchase whose content cannot be fetched is not
// prepared, and the check that was made is recorded in the prepared file with
// its time - so what the buyer saw before signing is auditable afterwards.
//
// The record says exactly what was established and no more. The pool record
// republishes the registration statement's inventory commitment, and
// prepare-buy recomputes it from the advisory's own CIDs before anything is
// fetched, so `committed-inventory` means the set is the bundle's committed
// one and not merely the seller's claim. What no check can establish is that a
// peer keeps serving the bytes; that is what the fetch in this run observes,
// and `binding` names which of the two a record carries.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// AvailabilityCheck is the buyer's own observation of one bundle's content.
type AvailabilityCheck struct {
	CheckedAt string `json:"checkedAt"`
	// File names the advisory this check repeated, so a reader can compare the
	// claim with the observation.
	File string `json:"file,omitempty"`
	// Claimed author/epoch must reproduce the canonical inventory commitment;
	// signing also checks the author against the registered pool listing.
	Author string `json:"author"`
	Epoch  uint16 `json:"epoch"`
	// Binding is `advisory` when the CIDs below came from the seller's advisory
	// alone, and `committed-inventory` when the set was also compared with the
	// commitment the registration statement published. Only the latter is a
	// statement about the bundle's committed inventory.
	Binding             string   `json:"binding"`
	CoveredFlips        int      `json:"coveredFlips"`
	VerifiedFlips       int      `json:"verifiedFlips"`
	Failures            []string `json:"failures,omitempty"`
	InventoryCommitment string   `json:"inventoryCommitment,omitempty"`
	// These encrypted bytes survive preparation, signing and restart in the
	// buyer's private prepared file. No serving peer is needed after payment.
	Blobs []RetainedFlip `json:"blobs,omitempty"`
}

type RetainedFlip struct {
	CID string `json:"cid"`
	Hex string `json:"hex"`
}

const (
	// bindingAdvisory marks a check that repeated the seller's own claim.
	bindingAdvisory = "advisory"
	// bindingCommitted marks a check whose CID set was compared with the
	// commitment the registration statement published, so it is a statement
	// about the bundle's committed inventory.
	bindingCommitted = "committed-inventory"
)

// Complete reports whether every flip the advisory named was fetched and
// rebuilt its CID in this run, and whether the set was bound to the bundle's
// committed inventory rather than left as the seller's claim. An unchecked
// purchase is never complete: nothing was fetched.
func (c *AvailabilityCheck) Complete() bool {
	return c != nil && c.Binding == bindingCommitted && c.CoveredFlips > 0 && c.VerifiedFlips == c.CoveredFlips && len(c.Failures) == 0
}

// availabilityMaxAge bounds how old the buyer's own content check may be when
// the purchase it belongs to is signed (`AH-17`). The check is an observation,
// not a proof about the future; the bound keeps a stale one from being signed
// as if it had been made for this purchase, without pretending the protocol can
// guarantee that a peer keeps serving afterwards.
const availabilityMaxAge = 15 * time.Minute

// requireCheckedAvailability is the signing-time gate on the ordinary buy
// path: the prepared file has to carry a complete, chain-bound check made
// recently, or the CLI refuses to sign it.
func requireCheckedAvailability(check *AvailabilityCheck, now time.Time) error {
	if err := requireRetainedInventory(check); err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339, check.CheckedAt)
	if err != nil {
		return fmt.Errorf("the availability check carries no usable timestamp: %q", check.CheckedAt)
	}
	if at.After(now.Add(time.Minute)) || now.Sub(at) > availabilityMaxAge {
		return fmt.Errorf("the availability check is stale: it was made at %s, and a purchase may be signed for %s after that", check.CheckedAt, availabilityMaxAge)
	}
	return nil
}

// Retained content remains usable after signing-time freshness has elapsed.
func requireRetainedInventory(check *AvailabilityCheck) error {
	if check == nil {
		return errors.New(
			"this purchase carries no availability check: prepare-buy fetches and checks the bundle's content before it builds the call, so an unchecked file is not signable")
	}
	if !check.Complete() {
		return fmt.Errorf(
			"the availability check is not a complete, chain-bound check: %d of %d flips verified with binding %q",
			check.VerifiedFlips, check.CoveredFlips, check.Binding)
	}
	if !isHex(strings.TrimPrefix(check.Author, "0x"), addressBytes) || check.Epoch == 0 || len(check.Blobs) != check.CoveredFlips {
		return errors.New("the purchase must retain every authenticated encrypted flip, its author and epoch")
	}
	cids := make([]string, 0, len(check.Blobs))
	seen := make(map[string]bool, len(check.Blobs))
	for _, retained := range check.Blobs {
		if _, err := cidBytesFromString(retained.CID); err != nil || seen[retained.CID] {
			return errors.New("the retained inventory contains an invalid or duplicate CID")
		}
		seen[retained.CID] = true
		blob, err := hex.DecodeString(retained.Hex)
		if err != nil || len(blob) == 0 || len(blob) > maxRetainedFlipBytes {
			return errors.New("the retained flip bytes are missing, invalid or oversized")
		}
		if ok, note := verifyBlobCID(retained.CID, blob, nil); !ok {
			return fmt.Errorf("retained flip %s: %s", retained.CID, note)
		}
		cids = append(cids, retained.CID)
	}
	if inventoryCommitment(check.Author, check.Epoch, cids) != check.InventoryCommitment {
		return errors.New("the retained CID set does not match its inventory commitment")
	}
	return nil
}

// The pinned node rejects larger flips (common.MaxFlipSize).
const maxRetainedFlipBytes = 614400

// recheckAvailability fetches every flip an advisory names and rebuilds its
// CID. Every entry gets a verdict: a CID that cannot be fetched, that comes
// back empty, or whose bytes rebuild another CID is a failure with its reason,
// never a silent skip.
func recheckAvailability(advisory *AvailabilityAdvisory, fetch bodyFetcher, compute cidComputer) *AvailabilityCheck {
	check := &AvailabilityCheck{
		CheckedAt:    checkedAtNow(),
		Author:       advisory.Author,
		Epoch:        advisory.Epoch,
		Binding:      bindingAdvisory,
		CoveredFlips: len(advisory.Flips),
	}
	if len(advisory.Flips) == 0 {
		check.Failures = append(check.Failures, "the advisory names no flips")
		return check
	}
	seen := make(map[string]bool, len(advisory.Flips))
	for _, flip := range advisory.Flips {
		if flip.CID == "" {
			check.Failures = append(check.Failures, "an entry names no CID")
			continue
		}
		if seen[flip.CID] {
			check.Failures = append(check.Failures, fmt.Sprintf("%s: the advisory names this CID twice", flip.CID))
			continue
		}
		seen[flip.CID] = true
		blob, _, err := fetch(flip.CID)
		if err != nil {
			check.Failures = append(check.Failures, fmt.Sprintf("%s: fetch: %v", flip.CID, err))
			continue
		}
		if len(blob) == 0 || len(blob) > maxRetainedFlipBytes {
			check.Failures = append(check.Failures, fmt.Sprintf("%s: the source returned no bytes", flip.CID))
			continue
		}
		ok, note := verifyBlobCID(flip.CID, blob, compute)
		if !ok {
			check.Failures = append(check.Failures, fmt.Sprintf("%s: %s", flip.CID, note))
			continue
		}
		check.VerifiedFlips++
		check.Blobs = append(check.Blobs, RetainedFlip{CID: flip.CID, Hex: hex.EncodeToString(blob)})
	}
	sort.Strings(check.Failures)
	return check
}
