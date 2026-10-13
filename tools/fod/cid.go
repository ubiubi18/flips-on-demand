package main

import (
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"

	"github.com/idena-network/idena-go/common/hexutil"
)

// cidString renders a CIDv1 byte string in the multibase base32 form the node's
// IPFS API accepts (`b` prefix, lower case, no padding).
//
// Only CIDv1 encoding is needed here: flip submissions observed on this chain
// carry CIDv1 values (0x01 0x55 0x12 0x20 ...). If a CIDv0 value appears, this
// returns an empty string rather than a wrong CID.
func cidString(cidBytes []byte) string {
	if len(cidBytes) == 0 || cidBytes[0] != 0x01 {
		return ""
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(cidBytes)
	return "b" + strings.ToLower(encoded)
}

// cidBytesFromString parses the multibase CID text the node stores back to its
// bytes, so a commitment taken over the hex of those bytes can be recomputed
// from a CID a reader only has as text. The text has to be exactly the form the
// node writes - `b` plus lower-case base32 without padding - and it has to
// re-encode to itself, so two spellings of one CID cannot pass as two CIDs.
func cidBytesFromString(cid string) ([]byte, error) {
	if len(cid) < 2 || cid[0] != 'b' {
		return nil, fmt.Errorf("%s is not a multibase CID this build reads", cid)
	}
	encoded := cid[1:]
	if strings.ToLower(encoded) != encoded {
		return nil, fmt.Errorf("%s is not the lower-case form the node writes", cid)
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(encoded))
	if err != nil {
		return nil, fmt.Errorf("%s is not base32: %w", cid, err)
	}
	if cidString(decoded) != cid {
		return nil, fmt.Errorf("%s does not re-encode to itself", cid)
	}
	return decoded, nil
}

// cidRawSha256 builds the CIDv1/raw/sha2-256 CID for a byte string. This is the
// form the node uses for block bodies and flip blobs, so an existing CID can be
// checked by recomputing it over the fetched bytes.
func cidRawSha256(data []byte) string {
	digest := sha256.Sum256(data)
	prefix := []byte{0x01, 0x55, 0x12, 0x20}
	return cidString(append(prefix, digest[:]...))
}

// cidComputer is retained for diagnostic compatibility. Its answer is never
// evidence for accepting content: the importer must be reproduced locally.
type cidComputer func(data []byte) (string, error)

// nodeCIDComputer asks a node's own IPFS for the CID of the bytes (`ipfs_cid`).
// This is diagnostic information, not an authorization check.
func nodeCIDComputer(c *client) cidComputer {
	return func(data []byte) (string, error) {
		var cid string
		if err := c.call("ipfs_cid", []any{hexutil.Bytes(data)}, &cid); err != nil {
			return "", err
		}
		if cid == "" {
			return "", errors.New("the node's IPFS named no cid for the blob")
		}
		return cid, nil
	}
}

// isRawCID reports whether a CID is CIDv1/raw/sha2-256, the form whose bytes can
// be checked by recomputing a sha256 locally.
func isRawCID(cid string) bool {
	return len(cid) == 59 && strings.HasPrefix(cid, "bafkrei")
}

// isDagPBCID reports whether a CID is CIDv1/dag-pb/sha2-256, the form the node
// produces for a flip that does not fit in one chunk.
func isDagPBCID(cid string) bool {
	return len(cid) == 59 && strings.HasPrefix(cid, "bafybei")
}

// verifyBlobCID decides whether a blob's bytes are the ones a recorded CID
// commits to, and says who decided it.
//
// Only locally reconstructed raw and pinned UnixFS profiles are accepted. An
// RPC assertion from the peer that supplied the bytes is not hash evidence.
// The diagnostic callback is deliberately never used for authorization.
func verifyBlobCID(cid string, blob []byte, _ cidComputer) (bool, string) {
	if isRawCID(cid) {
		if cidRawSha256(blob) == cid {
			return true, "checked: a raw cid is the sha256 of these bytes"
		}
		return false, "the sha256 of these bytes is not the cid they were fetched for"
	}
	if isDagPBCID(cid) {
		if computed, err := unixfsCID(blob); err == nil && computed == cid {
			return true, "checked: these bytes build the recorded UnixFS/dag-pb cid with the importer the node uses"
		}
	}
	return false, "unverified: these bytes do not reproduce the CID locally under a supported importer profile"
}
