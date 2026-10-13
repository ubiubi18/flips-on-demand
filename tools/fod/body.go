package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/idena-network/idena-go/blockchain/types"
)

// BodyCheck records what was done with one block body.
//
// Trust chain: the block header (as served by the node) commits to the body by
// CID; the fetched body bytes are verified against that CID; the decoded
// transaction set is then compared with the transaction hashes the node lists
// for the same block. Two independent data paths have to agree, and the body is
// bound to the CID by hashing.
//
// Not yet covered: authenticating the header fields themselves. The node's RPC
// does not expose the header's TxHash, so the strongest statement this tool can
// make today is "this body matches the CID the node reported, and its
// transactions match the node's listing for that block".
type BodyCheck struct {
	Height        uint64 `json:"height"`
	BlockHash     string `json:"blockHash"`
	BodyCID       string `json:"bodyCid"`
	BodyBytes     int    `json:"bodyBytes"`
	CIDMatches    bool   `json:"cidMatches"`
	Decoded       bool   `json:"decoded"`
	TxCountBody   int    `json:"txCountBody"`
	TxCountRPC    int    `json:"txCountRpc"`
	TxSetMatches  bool   `json:"txSetMatches"`
	DerivedTxHash string `json:"derivedTxHash"`
	Error         string `json:"error,omitempty"`
}

type bodyFetchFunc func(cid string) ([]byte, error)

// bodyFetcher is a fetch function that also names the source that answered, so
// an artifact records where ciphertext came from instead of assuming the node's
// IPFS supplied it.
type bodyFetcher func(cid string) (data []byte, source string, err error)

// oneSource turns a single fetch function into a bodyFetcher with the name the
// artifact should carry for it.
func oneSource(fetch bodyFetchFunc, name string) bodyFetcher {
	return func(cid string) ([]byte, string, error) {
		data, err := fetch(cid)
		return data, name, err
	}
}

// nodeThenGateway tries the node's own IPFS first and falls back to an IPFS
// gateway the caller named. A node-less user needs the fallback to obtain
// ciphertext at all; whoever answers is recorded, and the CID check that follows
// is what decides whether the bytes are the ones the chain committed to.
func nodeThenGateway(node bodyFetchFunc, gateway bodyFetchFunc) bodyFetcher {
	return func(cid string) ([]byte, string, error) {
		var nodeErr error
		if node != nil {
			data, err := node(cid)
			if err == nil {
				return data, "node ipfs", nil
			}
			nodeErr = err
		}
		if gateway != nil {
			data, err := gateway(cid)
			if err == nil {
				return data, "gateway", nil
			}
			if nodeErr != nil {
				return nil, "", fmt.Errorf("the node's IPFS failed (%v) and the gateway failed (%w)", nodeErr, err)
			}
			return nil, "", err
		}
		if nodeErr != nil {
			return nil, "", nodeErr
		}
		return nil, "", fmt.Errorf("no IPFS source was named")
	}
}

// gatewayFetch fetches ciphertext from an IPFS gateway by CID, for a caller who
// has no node of their own. The gateway is only a delivery route: the CID check
// that follows decides whether the bytes are the ones the chain committed to, and
// a gateway that serves different bytes fails that check.
func gatewayFetch(prefix string) bodyFetchFunc {
	client := &http.Client{Timeout: 120 * time.Second}
	return func(cid string) ([]byte, error) {
		url := strings.TrimRight(prefix, "/") + "/" + cid
		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s answered %d", url, resp.StatusCode)
		}
		return readBoundedResponse(resp, maxNetworkResponseBytes)
	}
}

// checkBlockBody fetches and verifies one block body.
func checkBlockBody(height uint64, blockHash, bodyCID string, rpcTxHashes []string, fetch bodyFetchFunc) BodyCheck {
	check := BodyCheck{
		Height:     height,
		BlockHash:  blockHash,
		BodyCID:    bodyCID,
		TxCountRPC: len(rpcTxHashes),
	}
	if bodyCID == "" {
		check.Error = "no body cid in header"
		return check
	}

	raw, err := fetch(bodyCID)
	if err != nil {
		check.Error = fmt.Sprintf("fetch: %v", err)
		return check
	}
	check.BodyBytes = len(raw)
	if len(raw) == 0 {
		check.Error = "body is empty"
		return check
	}
	check.CIDMatches = cidRawSha256(raw) == bodyCID

	var body types.Body
	if err := body.DecodeBytes(raw); err != nil {
		check.Error = fmt.Sprintf("decode: %v", err)
		return check
	}
	check.Decoded = true
	check.TxCountBody = len(body.Transactions)

	// Cross-check against the transaction hashes the node lists for this block.
	if len(body.Transactions) == len(rpcTxHashes) {
		match := true
		for i, tx := range body.Transactions {
			if tx.Hash().String() != rpcTxHashes[i] {
				match = false
				break
			}
		}
		check.TxSetMatches = match
	}

	// Record the commitment this body implies for the header, so it can be
	// compared against an authenticated header later.
	// Types.Transactions is the reference list wrapper that DeriveSha expects.
	derived := types.DeriveSha(types.Transactions(body.Transactions))
	check.DerivedTxHash = derived.String()
	return check
}
