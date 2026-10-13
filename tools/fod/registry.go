package main

// Reading the registry's own records through a node, for the checks the CLI
// makes before it prepares anything (`SPEC.md` 4.3 step 4: a request that does
// not match the contract's current state is refused, never adjusted).
//
// `contract_iterateMap` walks a string prefix, and the registry keys a record by
// its prefix followed by either the id in decimal ASCII (`key_for`) or the raw
// commitment (`key_for_address`-style keys). Reading a whole prefix to take one
// record is what that interface allows, because neither suffix is expressible as
// a string prefix.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// errNotInRegistry marks a record the contract does not hold, as opposed to one
// the node could not be asked about. The distinction matters: an absent record
// is a refusal the caller explains, while an unreachable node is a refusal it
// cannot reason about.
var errNotInRegistry = errors.New("not in the registry")

// The release contract bounds one bundle's unresolved orders so that a public
// reveal can settle every buyer who would learn its shared keys in one call.
const maxOpenPoolOrders = 8

func registryOpenPoolOrderCount(c *client, contract, commitment string) (int, error) {
	ids, err := registryOpenPoolOrderIDs(c, contract, commitment)
	return len(ids), err
}

func registryOpenPoolOrderIDs(c *client, contract, commitment string) ([]uint64, error) {
	records, err := readPrefix(c, registryAddress(contract), "popen/")
	if err != nil {
		return nil, fmt.Errorf("read open pool orders: %w", err)
	}
	raw := records[strings.ToLower(strings.TrimPrefix(commitment, "0x"))]
	if len(raw)%8 != 0 || len(raw) > maxOpenPoolOrders*8 {
		return nil, errors.New("invalid open-order registry record")
	}
	seen := make(map[uint64]bool, len(raw)/8)
	ids := make([]uint64, 0, len(raw)/8)
	for at := 0; at < len(raw); at += 8 {
		id := binary.BigEndian.Uint64(raw[at : at+8])
		if id == 0 || seen[id] {
			return nil, errors.New("invalid open-order registry record")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func registryPoolBundleStrikes(c *client, contract, commitment string) (uint32, error) {
	records, err := readPrefix(c, registryAddress(contract), marketPoolPrefix)
	if err != nil {
		return 0, fmt.Errorf("read pool bundle: %w", err)
	}
	raw := records[strings.ToLower(strings.TrimPrefix(commitment, "0x"))]
	if len(raw) != marketPoolValueBytes {
		return 0, errors.New("invalid pool bundle registry record")
	}
	return binary.BigEndian.Uint32(raw[marketPoolStrikesOffset:marketPoolNonceOffset]), nil
}

// registryPoolBundle reads one pool bundle record, keyed by the key commitment
// the registration journal carries, so the read walks the prefix.
func registryPoolBundle(c *client, contract, commitment string) (*marketPoolListing, error) {
	address := registryAddress(contract)
	records, err := readPrefix(c, address, marketPoolPrefix)
	if err != nil {
		return nil, err
	}
	key := strings.ToLower(strings.TrimPrefix(commitment, "0x"))
	raw, ok := records[key]
	if !ok {
		return nil, fmt.Errorf("pool bundle %s is %w at %s", key, errNotInRegistry, address)
	}
	poolReserve, err := readCounter(c, address, marketReserveKey)
	if err != nil {
		return nil, err
	}
	poolPending, err := readCounter(c, address, marketPendingKey)
	if err != nil {
		return nil, err
	}
	poolUnits, err := readCounter(c, address, marketPoolUnitsKey)
	if err != nil {
		return nil, err
	}
	return decodePoolRecord(key, raw, poolReserve, poolPending, poolUnits.Uint64())
}

// A fast bundle uses authenticated copies and a point delivery. The ordinary
// buyer commands cannot open that format, so they must refuse it before buying.
func registryHasFastBundle(c *client, contract, commitment string) (bool, error) {
	key := strings.ToLower(strings.TrimPrefix(commitment, "0x"))
	if !isHex(key, poolBundleBytes) {
		return false, errors.New("invalid fast bundle id")
	}
	records, err := readPrefix(c, registryAddress(contract), "pfast/")
	if err != nil {
		return false, fmt.Errorf("read fast bundles: %w", err)
	}
	_, exists := records[key]
	return exists, nil
}

// registryHead is the chain's own sense of now: the head height and, when the
// node can show it, how long a block took between the last two.
//
// The interval is used to judge what a block deadline means in wall-clock time,
// because the seller's delivery proof takes about fifteen minutes to make. It is
// read rather than assumed: a node on a quiet or automined chain reports no
// interval, and the caller then falls back to the deadline alone.
func registryHead(c *client) (height uint64, blockSeconds int64, err error) {
	var head rpcBlock
	if err := c.call("bcn_lastBlock", []any{}, &head); err != nil {
		return 0, 0, fmt.Errorf("read the head block: %w", err)
	}
	if head.Height == 0 || head.Time == 0 {
		return head.Height, 0, nil
	}
	var previous rpcBlock
	if err := c.call("bcn_blockAt", []any{head.Height - 1}, &previous); err != nil {
		return head.Height, 0, nil
	}
	if previous.Time <= 0 || head.Time <= previous.Time {
		return head.Height, 0, nil
	}
	return head.Height, head.Time - previous.Time, nil
}

func registryAddress(contract string) string {
	return "0x" + strings.ToLower(strings.TrimPrefix(contract, "0x"))
}
