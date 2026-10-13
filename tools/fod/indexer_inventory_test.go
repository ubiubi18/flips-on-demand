package main

// The Plan B route: an indexer supplies the listing, and a node decides whether
// any of it is true. The tests keep the two roles apart on purpose - the indexer
// server answers only the explorer's routes, and the node server answers only the
// node's RPC - so a listing that passes has been checked, not believed.

import (
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/common/hexutil"
)

const (
	planBAuthor          = "0x1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d"
	planBAuthorChecksum  = "0x1a2B3c4d5E6f708192a3B4c5D6E7F8091A2b3C4d"
	planBEpoch           = 228
	planBTxHash          = "0x54a914e555df1734bdeb56ec79a43e7727984cc5a8d4151a17a40c31730d9099"
	planBBlockHash       = "0xf3b2b60bf2bf28662aae3f9c64427f37ecc5aaa2d3d556c746f00e90741907a8"
	planBSecondTxHash    = "0x54a914e555df1734bdeb56ec79a43e7727984cc5a8d4151a17a40c31730d9098"
	planBSecondBlockHash = "0xf3b2b60bf2bf28662aae3f9c64427f37ecc5aaa2d3d556c746f00e90741907a7"
	planBBlockHeight     = 11351745
	planBRoot            = "5f7c1e4f2a6a1e0b0ef5f3a34f0f4b6f4f0d7c2a8e9b1d3c5a7f9e1b3d5f7a9c"
	planBEpochStart      = 11350781
)

// planBDagPubCID is a CIDv1/dag-pb/sha2-256 value, the form the pinned node
// produces for a flip larger than one chunk (`SOURCES.md` F-86).
func planBDagPubCID() string {
	raw := make([]byte, 4+32)
	copy(raw, []byte{0x01, 0x70, 0x12, 0x20})
	for i := 4; i < len(raw); i++ {
		raw[i] = 0x3f
	}
	return cidString(raw)
}

// decodeCID turns the multibase form back into the bytes a flip submission
// carries, so the tests can build the payload the node would serve.
func decodeCID(t *testing.T, cid string) []byte {
	t.Helper()
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(cid[1:]))
	if err != nil {
		t.Fatalf("decode cid %s: %v", cid, err)
	}
	return raw
}

// planBAnchor is one flip's chain anchor as the explorer publishes it.
type planBAnchor struct {
	cid       string
	txHash    string
	blockHash string
	height    uint64
}

// planBIndexer serves the explorer's four routes for one author-epoch.
func planBIndexer(t *testing.T, anchors []planBAnchor) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/Flips"):
			records := make([]string, 0, len(anchors))
			for index, anchor := range anchors {
				answer := "Left"
				if index%2 == 1 {
					answer = "Right"
				}
				records = append(records, fmt.Sprintf(`{"cid":%q,"author":%q,"epoch":%d,"status":"Qualified","answer":%q}`,
					anchor.cid, planBAuthorChecksum, planBEpoch, answer))
			}
			fmt.Fprintf(w, `{"result":[%s]}`, strings.Join(records, ","))
		case strings.Contains(r.URL.Path, "/Flip/"):
			wanted := strings.TrimPrefix(r.URL.Path, "/api/Flip/")
			for _, anchor := range anchors {
				if anchor.cid != wanted {
					continue
				}
				fmt.Fprintf(w, `{"result":{"author":%q,"epoch":%d,"size":307014,"status":"Qualified","answer":"Left","txHash":%q,"blockHash":%q,"blockHeight":%d}}`,
					planBAuthorChecksum, planBEpoch, anchor.txHash, anchor.blockHash, anchor.height)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"message":"no such flip"}}`)
		case r.URL.Path == fmt.Sprintf("/api/Epoch/%d", planBEpoch):
			fmt.Fprintf(w, `{"result":{"stateRoot":"0x%s"}}`, planBRoot)
		case strings.HasSuffix(r.URL.Path, "/Blocks"):
			fmt.Fprintf(w, `{"result":[{"height":%d}]}`, planBEpochStart)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"message":"unknown route"}}`)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// planBNode answers what a node holds for the same flips: the header at the
// height the explorer names, its own copy of the transaction, and the CID its own
// IPFS imports the bytes to. `payloads` maps a transaction hash to the submission
// payload the node serves for it, and `blobCIDs` maps blob bytes to the CID the
// node's IPFS answers - which is the only way a dag-pb CID can be decided.
func planBNode(t *testing.T, anchors []planBAnchor, payloads map[string][]byte, blobCIDs map[string]string) *client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode the node request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "bcn_blockAt":
			var params []uint64
			if err := json.Unmarshal(request.Params, &params); err != nil || len(params) != 1 {
				t.Errorf("the node was asked for a block without a height: %s", request.Params)
			}
			// The epoch's opening block is where the state root belongs; a node
			// keeps headers long after it prunes state.
			if params[0] == planBEpochStart {
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"height":%d,"hash":"0x%s","root":"0x%s","transactions":[]}}`,
					planBEpochStart, strings.Repeat("cd", 32), planBRoot)
				return
			}
			for _, anchor := range anchors {
				if anchor.height != params[0] {
					continue
				}
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"height":%d,"hash":%q,"root":"0x%s","transactions":[%q]}}`,
					anchor.height, anchor.blockHash, planBRoot, anchor.txHash)
				return
			}
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"block not found"}}`)
		case "bcn_transaction":
			var params []string
			if err := json.Unmarshal(request.Params, &params); err != nil || len(params) != 1 {
				t.Errorf("the node was asked for a transaction without a hash: %s", request.Params)
			}
			payload, ok := payloads[params[0]]
			if !ok {
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"transaction not found"}}`)
				return
			}
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"hash":%q,"type":"submitFlip","from":%q,"epoch":%d,"payload":"0x%s"}}`,
				params[0], planBAuthor, planBEpoch, hex.EncodeToString(payload))
		case "ipfs_cid":
			var params []hexutil.Bytes
			if err := json.Unmarshal(request.Params, &params); err != nil || len(params) != 1 {
				t.Errorf("the node was asked for a cid without one blob: %s", request.Params)
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"bad params"}}`)
				return
			}
			if cid, ok := blobCIDs[string(params[0])]; ok {
				fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%q}`, cid)
				return
			}
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"blob not found"}}`)
		default:
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`)
		}
	}))
	t.Cleanup(server.Close)
	return &client{url: server.URL, key: "test", http: newHTTPClient()}
}

func TestIndexerInventoryIsCheckedByANodeRatherThanBelieved(t *testing.T) {
	rawBlob := []byte("a flip that fits in one chunk")
	rawCID := cidRawSha256(rawBlob)
	dagBlob := nodeCIDPattern(262145)
	dagCID := "bafybeich6cqd6yabw3cku6edwmz4rvwe275nx2l6ootvc2bpbt5bdvarua"
	rawSubmission := &attachments.FlipSubmitAttachment{Cid: decodeCID(t, rawCID), Pair: 1}
	rawPayload, err := rawSubmission.ToBytes()
	if err != nil {
		t.Fatalf("build the first flip's payload: %v", err)
	}
	dagSubmission := &attachments.FlipSubmitAttachment{Cid: decodeCID(t, dagCID), Pair: 0}
	dagPayload, err := dagSubmission.ToBytes()
	if err != nil {
		t.Fatalf("build the second flip's payload: %v", err)
	}
	anchors := []planBAnchor{
		{cid: rawCID, txHash: planBTxHash, blockHash: planBBlockHash, height: planBBlockHeight},
		{cid: dagCID, txHash: planBSecondTxHash, blockHash: planBSecondBlockHash, height: planBBlockHeight + 6},
	}
	indexer := planBIndexer(t, anchors)
	node := planBNode(t, anchors,
		map[string][]byte{planBTxHash: rawPayload, planBSecondTxHash: dagPayload},
		map[string]string{string(rawBlob): rawCID, string(dagBlob): dagCID})
	blobs := map[string][]byte{rawCID: rawBlob, dagCID: dagBlob}

	inventory, err := buildIndexerInventory(newHTTPClient(), indexer.URL+"/api", planBEpoch, planBAuthor,
		oneSource(func(cid string) ([]byte, error) { return blobs[cid], nil }, "test gateway"),
		"", nodeCIDComputer(node), node, false)
	if err != nil {
		t.Fatalf("build the Plan B inventory: %v", err)
	}
	if inventory.Submissions != 2 || inventory.UniqueCIDs != 2 {
		t.Fatalf("the listing holds %d submissions and %d unique cids", inventory.Submissions, inventory.UniqueCIDs)
	}
	if inventory.Complete {
		t.Fatal("a listing no sweep completed was recorded as complete")
	}
	if !inventory.NodeVerified {
		t.Fatalf("every anchor was checkable and the listing says it is unverified: %s", inventory.NodeNote)
	}
	if inventory.Root == nil || !inventory.Root.NodeVerified || inventory.Root.Height != planBEpochStart {
		t.Fatalf("the epoch's state root was not checked against the node: %+v", inventory.Root)
	}
	if len(inventory.Entries) != 2 {
		t.Fatalf("the listing holds %d entries", len(inventory.Entries))
	}
	for _, entry := range inventory.Entries {
		if !entry.NodeChecked {
			t.Fatalf("entry %s carries no node check: %+v", entry.CID, entry)
		}
		if entry.TxHash == "" || entry.BlockHeight == 0 || entry.BlockHash == "" {
			t.Fatalf("entry %s does not carry the explorer's anchor: %+v", entry.CID, entry)
		}
		if !entry.BlobCIDOk {
			t.Fatalf("entry %s was not verified: %s (%s)", entry.CID, entry.Error, entry.BlobCIDNote)
		}
		decidedByIPFS := strings.Contains(entry.BlobCIDNote, "UnixFS")
		decidedBySha256 := strings.Contains(entry.BlobCIDNote, "raw cid")
		if !decidedByIPFS && !decidedBySha256 {
			t.Fatalf("entry %s does not say who decided its cid: %s", entry.CID, entry.BlobCIDNote)
		}
		if isRawCID(entry.CID) != decidedBySha256 {
			t.Fatalf("entry %s was decided by the wrong check: %s", entry.CID, entry.BlobCIDNote)
		}
	}
	if !strings.Contains(strings.Join(inventory.Notes, " "), "UnixFS/dag-pb") {
		t.Fatalf("the listing does not name the dag-pb cids it carries: %v", inventory.Notes)
	}
}

func TestIndexerInventoryRefusesAnAnchorTheNodeContradicts(t *testing.T) {
	rawBlob := []byte("a flip that fits in one chunk")
	rawCID := cidRawSha256(rawBlob)
	submission := &attachments.FlipSubmitAttachment{Cid: decodeCID(t, rawCID), Pair: 0}
	payload, err := submission.ToBytes()
	if err != nil {
		t.Fatalf("build the flip's payload: %v", err)
	}
	anchors := []planBAnchor{{cid: rawCID, txHash: planBTxHash, blockHash: planBBlockHash, height: planBBlockHeight}}
	indexer := planBIndexer(t, anchors)
	// The node's header at that height hashes to something else: a contradiction,
	// not a failure to check.
	node := planBNode(t, []planBAnchor{
		{cid: rawCID, txHash: planBTxHash, blockHash: "0x" + strings.Repeat("ab", 32), height: planBBlockHeight},
	}, map[string][]byte{planBTxHash: payload}, map[string]string{string(rawBlob): rawCID})

	_, err = buildIndexerInventory(newHTTPClient(), indexer.URL+"/api", planBEpoch, planBAuthor,
		oneSource(func(string) ([]byte, error) { return rawBlob, nil }, "test gateway"),
		"", nodeCIDComputer(node), node, false)
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("an indexer's block hash was accepted against a node that disagrees: %v", err)
	}
}

func TestIndexerInventoryRefusesWithoutANodeWhenTheCallerRequiresOne(t *testing.T) {
	rawBlob := []byte("a flip that fits in one chunk")
	rawCID := cidRawSha256(rawBlob)
	indexer := planBIndexer(t, []planBAnchor{{cid: rawCID, txHash: planBTxHash, blockHash: planBBlockHash, height: planBBlockHeight}})

	_, err := buildIndexerInventory(newHTTPClient(), indexer.URL+"/api", planBEpoch, planBAuthor,
		oneSource(func(string) ([]byte, error) { return rawBlob, nil }, "test gateway"),
		"", nil, nil, true)
	if err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("an unconfirmed listing passed a caller that required a node: %v", err)
	}
}

// TestIndexerInventoryLeavesAStrangeBlobUnverifiedWithoutANode is the honest half
// of F-86: bytes that do not build the cid they were fetched for are reported as
// unverified when no node can rule on the importer that produced it, rather than
// accepted because a gateway served them.
func TestIndexerInventoryLeavesAStrangeBlobUnverifiedWithoutANode(t *testing.T) {
	dagBlob := []byte("a flip over one chunk")
	dagCID := planBDagPubCID()
	indexer := planBIndexer(t, []planBAnchor{{cid: dagCID, txHash: planBTxHash, blockHash: planBBlockHash, height: planBBlockHeight}})

	inventory, err := buildIndexerInventory(newHTTPClient(), indexer.URL+"/api", planBEpoch, planBAuthor,
		oneSource(func(string) ([]byte, error) { return dagBlob, nil }, "test gateway"),
		"", nil, nil, false)
	if err != nil {
		t.Fatalf("build the Plan B inventory: %v", err)
	}
	if inventory.BlobsOk != 0 || inventory.Complete || inventory.NodeVerified {
		t.Fatalf("an unchecked dag-pb blob came out verified: %+v", inventory)
	}
	if !strings.Contains(inventory.Entries[0].BlobCIDNote, "locally") {
		t.Fatalf("the entry does not say what is missing: %s", inventory.Entries[0].BlobCIDNote)
	}
}

// TestIndexerInventoryVerifiesTheNodesImporterWithoutOne is the other half: the
// layout the node uses is reproduced here, so a caller with no node decides a
// flip over one chunk from its bytes alone (`SOURCES.md` F-88).
func TestIndexerInventoryVerifiesTheNodesImporterWithoutOne(t *testing.T) {
	const cid = "bafybeich6cqd6yabw3cku6edwmz4rvwe275nx2l6ootvc2bpbt5bdvarua"
	blob := nodeCIDPattern(262145)
	indexer := planBIndexer(t, []planBAnchor{{cid: cid, txHash: planBTxHash, blockHash: planBBlockHash, height: planBBlockHeight}})

	inventory, err := buildIndexerInventory(newHTTPClient(), indexer.URL+"/api", planBEpoch, planBAuthor,
		oneSource(func(string) ([]byte, error) { return blob, nil }, "test gateway"),
		"", nil, nil, false)
	if err != nil {
		t.Fatalf("build the Plan B inventory: %v", err)
	}
	if inventory.BlobsOk != 1 || inventory.BlobsFailed != 0 {
		t.Fatalf("the node's importer was reproduced and the blob was still not verified: %+v", inventory.Entries)
	}
	if !strings.Contains(inventory.Entries[0].BlobCIDNote, "the importer the node uses") {
		t.Fatalf("the entry does not say what decided it: %s", inventory.Entries[0].BlobCIDNote)
	}
	// The listing itself is still not a node-confirmed one.
	if inventory.NodeVerified || inventory.Complete {
		t.Fatalf("a listing no node checked came out verified: %+v", inventory)
	}
}

func TestAListingFromAnIndexerCannotBecomeAManifest(t *testing.T) {
	cid := planBDagPubCID()
	inventory := &BundleInventory{
		Author: planBAuthor, Epoch: planBEpoch, Source: "https://api.idena.io/api",
		Submissions: 1, UniqueCIDs: 1, BlobsOk: 1,
		Entries: []BundleEntry{{CID: cid, BlobCIDOk: true}},
	}
	_, err := inventoryCidsFor(inventory, planBAuthor, planBEpoch, []string{cid})
	if err == nil || !strings.Contains(err.Error(), "no node confirmed") {
		t.Fatalf("an unconfirmed indexer listing was accepted as a manifest: %v", err)
	}
	inventory.NodeVerified = true
	_, err = inventoryCidsFor(inventory, planBAuthor, planBEpoch, []string{cid})
	if err == nil || !strings.Contains(err.Error(), "committed-body sweep") {
		t.Fatalf("an indexer listing was accepted as a manifest: %v", err)
	}
}
