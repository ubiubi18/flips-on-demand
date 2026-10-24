// fod reconstructs historical Idena flip evidence from a node's RPC or a
// captured datadir, derives and vaults epoch keys, prepares marketplace
// actions, and signs them.
//
// The evidence half: sweep a block range, classify its transactions, verify the
// bodies those transactions came in against their CIDs, reconstruct the
// epoch's lottery assignment and its qualification arithmetic, and decide
// whether one author's bundle is eligible (`SPEC.md` 5.1-5.3). The marketplace
// half turns that decision into the calls the escrow takes.
//
// The wire-format decoding deliberately reuses the pinned idena-go parsers
// instead of re-implementing protobuf layouts: those parsers are the reference
// definition of the format.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common/hexutil"
	"github.com/idena-network/idena-go/crypto"
)

// ---------------------------------------------------------------------------
// Node RPC client (subset used here)
// ---------------------------------------------------------------------------

type rpcBlock struct {
	Height       uint64   `json:"height"`
	Hash         string   `json:"hash"`
	Root         string   `json:"root"`
	Time         int64    `json:"timestamp"`
	IpfsCid      string   `json:"ipfsCid"`
	Transactions []string `json:"transactions"`
	IsEmpty      bool     `json:"isEmpty"`
}

type rpcTx struct {
	Hash    string        `json:"hash"`
	Type    string        `json:"type"`
	From    string        `json:"from"`
	To      string        `json:"to"`
	Epoch   uint16        `json:"epoch"`
	Payload hexutil.Bytes `json:"payload"`
}

type client struct {
	url           string
	key           string
	http          *httpClient
	bondedVersion int
}

func (c *client) call(method string, params any, result any) error {
	return c.http.call(c.url, c.key, method, params, result)
}

// ---------------------------------------------------------------------------
// Sweep output
// ---------------------------------------------------------------------------

// Submission is one accepted flip submission transaction.
type Submission struct {
	TxHash      string `json:"txHash"`
	BlockHeight uint64 `json:"blockHeight"`
	Author      string `json:"author"`
	Epoch       uint16 `json:"epoch"`
	CID         string `json:"cid"`
	CIDBytes    string `json:"cidBytesHex"`
	Pair        uint8  `json:"pair"`
}

// Deletion is one flip deletion transaction, keyed by CID like the submission.
type Deletion struct {
	TxHash      string `json:"txHash"`
	BlockHeight uint64 `json:"blockHeight"`
	Author      string `json:"author"`
	Epoch       uint16 `json:"epoch"`
	CID         string `json:"cid"`
	CIDBytes    string `json:"cidBytesHex"`
}

type SweepResult struct {
	FromHeight      uint64         `json:"fromHeight"`
	ToHeight        uint64         `json:"toHeight"`
	BlocksSeen      int            `json:"blocksSeen"`
	BlocksWithBody  int            `json:"blocksWithBody"`
	TxsSeen         int            `json:"txsSeen"`
	MissingTxBodies int            `json:"missingTxBodies"`
	MissingHashes   []string       `json:"missingTxHashes"`
	BodiesVerified  int            `json:"bodiesCidVerified"`
	TxSetMismatch   int            `json:"bodyTxSetMismatches"`
	TypeCounts      map[string]int `json:"typeCounts"`
	Submissions     []Submission   `json:"submissions"`
	Deletions       []Deletion     `json:"deletions"`
	Txs             []TxSummary    `json:"txs"`
	BodyChecks      []BodyCheck    `json:"bodyChecks"`
	// Answers holds the long-answer transactions the sweep saw, with the
	// payloads themselves. A node's epoch store holds the same payloads while it
	// runs the ceremony and clears them when the epoch ends, so a sweep is how
	// the answers stay available to a reconstruction afterwards (`SPEC.md` 5.2
	// wants inputs authenticated by the transaction that carried them).
	Answers []LongAnswerTx `json:"answers,omitempty"`
}

// TxSummary records every transaction seen in the range, so later stages can
// find the answer submissions without re-sweeping the chain.
type TxSummary struct {
	TxHash      string `json:"txHash"`
	BlockHeight uint64 `json:"blockHeight"`
	Type        string `json:"type"`
	Epoch       uint16 `json:"epoch"`
	From        string `json:"from"`
	PayloadLen  int    `json:"payloadLen"`
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "app-v8" || os.Args[1] == "app-v9") {
		version := 8
		if os.Args[1] == "app-v9" {
			version = 9
		}
		if err := runStandaloneBonded(os.Args[2:], version); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "v8" || os.Args[1] == "v9") {
		version := 8
		if os.Args[1] == "v9" {
			version = 9
		}
		if err := runBonded(os.Args[2:], version); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	var (
		rpcURL              = flag.String("rpc", "http://127.0.0.1:9019", "node RPC URL")
		apiKeyFile          = flag.String("key-file", "", "owner-only file containing the node RPC API key")
		from                = flag.Uint64("from", 0, "first block height (inclusive)")
		to                  = flag.Uint64("to", 0, "last block height (inclusive)")
		epochFilter         = flag.Int("epoch", -1, "only keep transactions of this epoch (-1 = all)")
		out                 = flag.String("out", "sweep.json", "output file")
		conc                = flag.Int("concurrency", 16, "parallel RPC requests")
		bodySource          = flag.String("body-source", "rpc", "transaction body source: rpc (bcn_transaction) or ipfs (block body by committed CID)")
		mode                = flag.String("mode", "sweep", "sweep: walk blocks; inventory: build one author bundle from a sweep file")
		sweepFile           = flag.String("sweep", "", "sweep JSON to read in inventory mode, or the sweep to take an epoch's long answers from in eligibility mode")
		author              = flag.String("author", "", "author address for inventory and session evidence modes")
		inventoryEpoch      = flag.Uint("inventory-epoch", 0, "epoch for inventory mode")
		txHash              = flag.String("tx", "", "transaction hash for decode-answers mode")
		dataDir             = flag.String("datadir", "", "node datadir for epoch-db mode (opened read-only)")
		stateHeight         = flag.Uint64("state-height", 0, "block height to read identity state at (state-at mode)")
		inventoryHeight     = flag.Uint64("inventory-height", 0, "flip-phase height for session-bundle mode")
		verdictHeight       = flag.Uint64("verdict-height", 0, "ValidationFinished height for session-bundle mode")
		inventoryAnchor     = flag.String("inventory-anchor", "", "saved inventory anchor for session-bundle mode")
		verdictAnchor       = flag.String("verdict-anchor", "", "saved verdict anchor for session-bundle mode")
		preSessionInventory = flag.String("pre-session-inventory", "", "owner-only author/epoch/CID inventory for pre-session-witness mode")
		sessionBlobOut      = flag.String("blob-out", "", "with session-bundle: write the private registration witness for every manifest flip (use -blob-dir, -rpc, or -blob for one flip)")
		identityFlag        = flag.String("identity", "", "identity address to prove in state-proof mode (default: the first identity at that version)")
		proofTree           = flag.String("tree", "identity", "tree to prove from in state-proof mode: identity (header's IdentityRoot) or state (ledger-side record)")
		bundleOut           = flag.String("proof-bundle", "", "write a portable proof bundle (leaf hashes, paths, root) to this path in state-proof mode")
		bundleLimit         = flag.Int("proof-limit", 4, "how many proofs a bundle carries")
		addressesFlag       = flag.String("addresses", "", "file of identity addresses a proof bundle has to cover (JSON array or one per line)")
		expectRootFlag      = flag.String("expect-root", "", "refuse the bundle unless the tree's root at that height is this digest (usually the manifest's validation anchor)")
		nodeKeyPath         = flag.String("nodekey", "", "path to an Idena nodekey file for key derivation")
		buyerFlag           = flag.String("buyer", "", "buyer address for direct-buy; must match the active RPC identity")
		vaultPath           = flag.String("vault", "", "path to a passphrase-encrypted age vault")
		blobPath            = flag.String("blob", "", "path to one flip blob (receive or single-flip session-bundle mode)")
		requestPath         = flag.String("request", "", "path to an action request written by the interface")
		deliveryKey         = flag.String("delivery-key", "", "path to a 32-byte hex delivery private key (mode 0600)")
		deliveryArchive     = flag.String("delivery-key-archive", "", "write an age-encrypted archive of the delivery key here (prepare-buy mode); the key flags also accept such an archive as their input")
		preparedPath        = flag.String("prepared", "", "prepared action: output for prepare commands, input for submit and pool receive")
		proverPath          = flag.String("prover", "", "path to the delivery prover binary (serve mode's proof fallback)")
		proofPath           = flag.String("proof", "", "proof artefact: prepare-registration and prepare-list read a sealed registration receipt here")
		proofHostPath       = flag.String("registration-host", "", "local proof verifier host; fast-content-host for direct v5 fallback")
		nonceFlag           = flag.String("nonce", "", "account nonce for the transaction to sign (submit mode)")
		txEpochFlag         = flag.String("tx-epoch", "", "epoch the transaction belongs to (submit mode; -epoch is the inventory epoch)")
		maxFeeFlag          = flag.String("max-fee", "", "maximum fee in base units for the transaction (submit mode)")
		tipsFlag            = flag.String("tips", "", "tips in base units (submit mode, default 0)")
		signedPath          = flag.String("signed", "", "write the signed transaction here (submit mode)")
		broadcastFlag       = flag.Bool("broadcast", false, "send the signed transaction to the node (submit mode)")
		blobsOut            = flag.String("blobs-out", "", "save verified blobs here as <cid> (inspect mode)")
		blobDir             = flag.String("blob-dir", "", "directory of saved blobs named <cid> (session-bundle reads its manifest entries)")
		gatewayFlag         = flag.String("gateway", "", "IPFS gateway prefix, e.g. https://ipfs.io/ipfs/ (inspect mode with -indexer: fetch ciphertext when no node's IPFS answers)")
		answersPath         = flag.String("answers", "", "consensus answers to include in the export")
		answersSource       = flag.String("answers-source", "", "evidence label for the answers file")
		exportDirFlag       = flag.String("export-dir", "", "the session-content export directory (session-pool mode)")
		sessionIDFlag       = flag.String("session-id", "", "the session's own identifier (session-pool mode)")
		utilityIDFlag       = flag.String("utility-id", "", "the utility this session grants access to (session-pool mode)")
		sharedNodeID        = flag.String("node-id", "shared-node", "the node's own name in the shared-node connection file (shared-node mode)")
		sharedNodeURL       = flag.String("node-url", "", "the node's public URL, http or https (shared-node mode)")
		sharedNodeCustody   = flag.String("custody", "hosted", "who keeps the epoch vault: hosted (the node) or self (the author) (shared-node mode)")
		thresholdFlag       = flag.Int("threshold", 0, "answers needed to pass (session-pool mode)")
		opensAtFlag         = flag.String("opens-at", "", "when the session opens, RFC 3339 (session-pool mode; defaults to now)")
		closesAtFlag        = flag.String("closes-at", "", "when the session closes, RFC 3339 (session-pool mode)")
		bundleFlag          = flag.String("bundle", "", "path to the author-epoch inventory `fod inspect` wrote (prepare-offer mode)")
		directSessionFlag   = flag.String("session", "", "canonical two-anchor session bundle for direct-list, direct-buy, v5 fallback, or v6 pool preparation")
		directPurchaseFlag  = flag.String("purchase", "", "retained direct-buy action for direct-check or direct-receive")
		directMessageFlag   = flag.String("message", "", "private direct-delivery message file to import, or - for standard input")
		directAutoConfirm   = flag.Bool("auto-confirm", false, "after direct-receive verifies every original flip, sign and broadcast buyer confirmation for this order")
		inputFlag           = flag.String("input", "", "write the registration input here (prepare-offer mode; default: registration-input.json)")
		anchorFlag          = flag.String("anchor", "", "validation anchor, a 32-byte state root (prepare-offer mode; default: read from -indexer)")
		proofAnchorFlag     = flag.String("proof-anchor", "", "path to a proof bundle from `state-proof -proof-bundle`, carried into the registration input (prepare-offer mode)")
		poolBundleFlag      = flag.String("pool-bundle", "", "bundle key commitment for prepare-buy mode")
		headersFlag         = flag.String("headers", "", "header roots output for export-fast-certificate mode")
		poolNonceFlag       = flag.Uint64("pool-nonce", 0, "the bundle's current buy nonce for prepare-buy mode")
		poolOrderFlag       = flag.Uint64("pool-order", 0, "pool order id for receive or prepare-confirm mode")
		deliveryRecordFlag  = flag.String("delivery", "", "path to the record `receive -out` wrote, which names the ciphertext the buyer checked")
		serveInterval       = flag.Int("interval", 20, "seconds between serve passes")
		watchOrder          = flag.Uint64("order", 0, "pool order id to follow (watch mode)")
		serveOnce           = flag.Bool("once", false, "run one serve pass and exit (serve mode)")
		serveOut            = flag.String("outdir", "serve-actions", "write prepared desk actions here when serve runs without -broadcast")
		serveRefresh        = flag.Uint64("beacon-refresh", serveDefaultRefresh, "refresh a served bundle's beacon when it is older than this many blocks")
		serveProof          = flag.Uint64("proof-margin", serveDefaultProof, "start the serve fallback proof when an order has this many blocks or fewer left")
		serveReveal         = flag.Uint64("reveal-margin", serveDefaultReveal, "publish the committed secrets (reveal) when an open order has this many blocks or fewer left; a delivery is withheld once a later reveal can no longer be included (serve mode)")
		requireNodeFlag     = flag.Bool("require-node", false, "refuse an indexer cross-check that no node confirmed (assignment and eligibility modes)")
		epochBlockFlag      = flag.Uint64("epoch-block", 0, "the block an epoch's state root belongs to, when the caller knows it (assignment and eligibility modes; default: ask the indexer)")
		codePath            = flag.String("code", "", "path to the contract module to deploy (prepare-deploy mode)")
		codeHashFlag        = flag.String("code-hash", "", "independently verified Keccak-256 code hash (v7 prepare-deploy, prepare-buy, and buy submit)")
		directVersionFlag   = flag.Int("direct-version", 0, "direct contract version; 4 and 5 select proof-backed direct sales, 6 selects v6 pool reads, 7 selects v7 preparation (default: pinned deployed v3)")
		v7OfferIDFlag       = flag.String("v7-offer-id", "", "32-byte v7 offer ID for direct-v7-prepare")
		minimumPayoutFlag   = flag.String("minimum-payout", "0", "minimum seller payout in base units for v7 list")
		deployNonceFlag     = flag.String("deploy-nonce", "", "hex deployment nonce that decides the contract address (prepare-deploy mode; default: a fresh random one)")
		v6SeedIDsFlag       = flag.String("v6-seed-ids", "", "two 32-byte v6 seed offer IDs concatenated as hex (prepare-deploy mode only)")
		v7SeedIDFlag        = flag.String("v7-seed-id", "", "one 32-byte v7 public seed offer ID (prepare-deploy mode only)")
		amountFlag          = flag.String("amount", "0", "base units for deploy or an explicit v7 payable-call value")
		networkFlag         = flag.String("network", "unspecified", "network label recorded in the artefact (prepare-deploy mode)")
		contractFlag        = flag.String("contract", "", "expected contract address; for buys, independently approved rather than copied from a snapshot")
		feePerGasFlag       = flag.String("fee-per-gas", "", "chain fee per gas in base units; fees mode prices at it, and submit and prepare-deploy check their max fee against it instead of the node's own rate")
		operationFlag       = flag.String("operation", "", "restrict fees mode to one operation, or select the typed direct-v7-prepare method")
		indexerURLFlag      = flag.String("indexer", "", "public indexer URL: the fee rate (fees mode), the epoch records to cross-check an assignment against (assignment mode), or those records plus the author's flip qualification (eligibility mode)")
		policyFlag          = flag.String("policy", EligibilityPolicyV1, "eligibility policy to apply (eligibility mode)")
		viewerFlag          = flag.String("viewer", "", "address whose credit to include in the marketplace state (market mode)")
		proofsFlag          = flag.String("proofs", "", "comma-separated registration artefacts that supply bundle provenance (market mode)")
		availabilityFlag    = flag.String("availability", "", "content evidence for prepare-buy: an ordinary availability advisory or a sealed fast proof with its copy list")
		availabilityOutFlag = flag.String("availability-out", "", "write an availability advisory from the inventory here (inventory mode)")
		appDataDirFlag      = flag.String("app-data-dir", "", "private local data directory for the standalone app")
		appPreviewFlag      = flag.Bool("read-only-preview", false, "open a synthetic standalone app preview without a node, keys, or transactions")
	)
	// Command form: `fod <command> [flags]`. normalizeArgs maps the command to
	// the internal mode and keeps the older `-mode` flag working.
	flag.CommandLine.Parse(normalizeArgs(os.Args[1:]))
	if *mode == "app" && *appPreviewFlag {
		if err := runStandaloneDemo(); err != nil {
			fmt.Fprintf(os.Stderr, "synthetic app preview refused: %v\n", err)
			os.Exit(1)
		}
		return
	}
	apiKey, err := resolveRPCKey(*apiKeyFile, flagWasSet("key-file"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "node RPC key refused: %v\n", err)
		os.Exit(2)
	}

	c := &client{url: *rpcURL, key: apiKey, http: newHTTPClient()}
	if *mode == "app" {
		appDataDir := *appDataDirFlag
		if appDataDir == "" {
			base, err := os.UserConfigDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "local app directory unavailable: %v\n", err)
				os.Exit(2)
			}
			appDataDir = filepath.Join(base, "Flips on Demand")
		}
		if err := runStandaloneApp(standaloneConfig{RPC: *rpcURL, KeyFile: *apiKeyFile, Contract: *contractFlag,
			CodeHash: *codeHashFlag, NodeKey: *nodeKeyPath, DataDir: appDataDir}); err != nil {
			fmt.Fprintf(os.Stderr, "local app refused: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if strings.HasPrefix(*mode, "direct-") {
		err := directCommand(directCommandOptions{
			Mode: *mode, DirectVersion: *directVersionFlag, Node: c, Contract: *contractFlag, CodeHash: *codeHashFlag,
			Operation: *operationFlag, OfferID: *v7OfferIDFlag, MinimumPayout: *minimumPayoutFlag,
			Network: *networkFlag, SessionPath: *directSessionFlag, VaultPath: *vaultPath,
			NodeKeyPath: *nodeKeyPath, BuyerAddress: *buyerFlag, DeliveryKeyPath: *deliveryKey,
			PurchasePath: *directPurchaseFlag, MessagePath: *directMessageFlag,
			DeliveryPath: *deliveryRecordFlag, ProofPath: *proofPath,
			ProofHostPath: *proofHostPath,
			PreparedPath:  *preparedPath, OutPath: *out, OutExplicit: flagWasSet("out"),
			Amount: *amountFlag, AmountExplicit: flagWasSet("amount"), OrderID: *watchOrder,
			AutoConfirm: *directAutoConfirm, Broadcast: *broadcastFlag,
			NonceText: *nonceFlag, TxEpochText: *txEpochFlag, MaxFeeText: *maxFeeFlag,
			TipsText: *tipsFlag, FeePerGasText: *feePerGasFlag,
			SignedPath: *signedPath, RPCURL: *rpcURL, APIKey: apiKey,
			RPCExplicit: flagWasSet("rpc"), KeyFileExplicit: flagWasSet("key-file"), KeyFilePath: *apiKeyFile,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "direct sale refused: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *mode == "prepare-list" {
		if *requestPath == "" {
			fmt.Fprintln(os.Stderr, "prepare-list needs -request FILE, and -proof FILE for a sellUnit request")
			os.Exit(2)
		}
		runPreparePoolList(c, *requestPath, *proofPath, *preparedPath)
		return
	}

	if *mode == "prepare-buy" {
		if *requestPath != "" {
			if *nodeKeyPath == "" || *deliveryKey == "" || *preparedPath == "" {
				fmt.Fprintln(os.Stderr, "prepare-buy with -request needs -nodekey FILE -delivery-key FILE -prepared FILE -availability FILE [-delivery-key-archive FILE]")
				os.Exit(2)
			}
			if *availabilityFlag == "" {
				fmt.Fprintln(os.Stderr, "prepare-buy needs -availability FILE: an ordinary availability advisory or a sealed fast content proof")
				os.Exit(2)
			}
			runPrepareBuyRequest(c, *requestPath, *nodeKeyPath, *deliveryKey, *deliveryArchive, *preparedPath, *availabilityFlag, *contractFlag, *codeHashFlag)
			return
		}
		if err := checkBuyTarget(c, *contractFlag, *contractFlag, *codeHashFlag); err != nil {
			fmt.Fprintf(os.Stderr, "buy target refused: %v\n", err)
			os.Exit(1)
		}
		runPrepareBuy(*contractFlag, *poolBundleFlag, *poolNonceFlag, *amountFlag, *networkFlag, *nodeKeyPath, *deliveryKey, *preparedPath)
		return
	}

	if *mode == "prepare-fund" {
		if *contractFlag == "" || *poolBundleFlag == "" || *amountFlag == "" || *preparedPath == "" {
			fmt.Fprintln(os.Stderr, "prepare-fund needs -contract ADDRESS -pool-bundle COMMITMENT -amount BASE_UNITS -prepared FILE")
			os.Exit(2)
		}
		runPrepareFund(*contractFlag, *poolBundleFlag, *amountFlag, *networkFlag, *preparedPath)
		return
	}

	if *mode == "prepare-confirm" {
		if *requestPath != "" {
			if *preparedPath == "" {
				fmt.Fprintln(os.Stderr, "prepare-confirm with -request needs -prepared FILE")
				os.Exit(2)
			}
			runPrepareConfirmRequest(*requestPath, *deliveryRecordFlag, *preparedPath)
			return
		}
		if *contractFlag == "" || *poolOrderFlag == 0 || *preparedPath == "" {
			fmt.Fprintln(os.Stderr, "prepare-confirm mode needs -request FILE -prepared FILE, or -contract ADDRESS -pool-order ID -prepared FILE")
			os.Exit(2)
		}
		runPrepareConfirm(*contractFlag, *networkFlag, *poolOrderFlag, *deliveryRecordFlag, *preparedPath)
		return
	}

	if *mode == "serve" {
		if *contractFlag == "" || *vaultPath == "" {
			fmt.Fprintln(os.Stderr, "serve mode needs -contract ADDRESS -vault FILE[,FILE...]")
			os.Exit(2)
		}
		if *broadcastFlag && *nodeKeyPath == "" {
			fmt.Fprintln(os.Stderr, "serve -broadcast needs -nodekey FILE: the desk signs its own calls")
			os.Exit(2)
		}
		var maxFee *big.Int
		if *maxFeeFlag != "" {
			parsed, ok := new(big.Int).SetString(*maxFeeFlag, 10)
			if !ok || parsed.Sign() < 0 {
				fmt.Fprintf(os.Stderr, "max fee %q is not an integer amount\n", *maxFeeFlag)
				os.Exit(2)
			}
			maxFee = parsed
		}
		tips := big.NewInt(0)
		if *tipsFlag != "" {
			parsed, ok := new(big.Int).SetString(*tipsFlag, 10)
			if !ok || parsed.Sign() < 0 {
				fmt.Fprintf(os.Stderr, "tips %q is not an integer amount\n", *tipsFlag)
				os.Exit(2)
			}
			tips = parsed
		}
		var feePerGas *big.Int
		if *feePerGasFlag != "" {
			parsed, ok := new(big.Int).SetString(*feePerGasFlag, 10)
			if !ok || parsed.Sign() <= 0 {
				fmt.Fprintf(os.Stderr, "fee per gas %q is not a positive integer amount\n", *feePerGasFlag)
				os.Exit(2)
			}
			feePerGas = parsed
		}
		err := runServe(serveOptions{
			RPCURL:       *rpcURL,
			APIKey:       apiKey,
			Contract:     *contractFlag,
			Network:      *networkFlag,
			VaultFlag:    *vaultPath,
			NodeKey:      *nodeKeyPath,
			Prover:       *proverPath,
			OutDir:       *serveOut,
			Broadcast:    *broadcastFlag,
			Once:         *serveOnce,
			IntervalSec:  *serveInterval,
			Refresh:      *serveRefresh,
			ProofMargin:  *serveProof,
			RevealMargin: *serveReveal,
			MaxFee:       maxFee,
			Tips:         tips,
			FeePerGas:    feePerGas,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "serve refused: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *mode == "prepare-registration" {
		if *proofPath == "" {
			fmt.Fprintln(os.Stderr, "prepare-registration mode needs -proof FILE (the sealed registration artefact)")
			os.Exit(2)
		}
		runPrepareRegistration(*proofPath, *contractFlag, *networkFlag, *preparedPath)
		return
	}
	if *mode == "prepare-fast-registration" {
		c := &client{url: *rpcURL, key: apiKey, http: newHTTPClient()}
		if err := runPrepareFastRegistration(c, *proofPath, *bundleFlag, *contractFlag, *networkFlag, *preparedPath); err != nil {
			fmt.Fprintf(os.Stderr, "fast registration refused: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *mode == "export-fast-certificate" {
		if *contractFlag == "" || *poolBundleFlag == "" || *headersFlag == "" || !flagWasSet("out") {
			fmt.Fprintln(os.Stderr, "export-fast-certificate needs -contract ADDRESS -pool-bundle COMMITMENT -out CERTIFICATE_FILE -headers HEADERS_FILE")
			os.Exit(2)
		}
		certificate, headers, err := exportFastCertificate(c, *contractFlag, *poolBundleFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fast certificate export refused: %v\n", err)
			os.Exit(1)
		}
		if err := writePrivateBytes(*out, certificate); err != nil {
			fmt.Fprintf(os.Stderr, "write certificate: %v\n", err)
			os.Exit(1)
		}
		if err := writePrivateJSON(*headersFlag, headers); err != nil {
			fmt.Fprintf(os.Stderr, "write headers: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("contract certificate and canonical header roots exported; run the independent Rust verifier on these files")
		return
	}
	if *mode == "export-fast-order" {
		if *contractFlag == "" || *poolOrderFlag == 0 || !flagWasSet("out") {
			fmt.Fprintln(os.Stderr, "export-fast-order needs -contract ADDRESS -pool-order ID -out ORDER_FILE")
			os.Exit(2)
		}
		order, err := prepareFastDeliveryOrder(c, *contractFlag, *poolOrderFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fast order export refused: %v\n", err)
			os.Exit(1)
		}
		if err := writePrivateJSON(*out, order); err != nil {
			fmt.Fprintf(os.Stderr, "write order: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("funded fast order context exported; prepare the point delivery locally before the block deadline")
		return
	}

	if *mode == "prepare-offer" {
		if *dataDir == "" || *inventoryEpoch == 0 || *author == "" || *nodeKeyPath == "" || *bundleFlag == "" || *blobDir == "" {
			fmt.Fprintln(os.Stderr, "prepare-offer mode needs -datadir PATH -inventory-epoch N -author ADDRESS -nodekey FILE -bundle FILE -blob-dir DIR")
			os.Exit(2)
		}
		if *contractFlag == "" {
			fmt.Fprintln(os.Stderr, "prepare-offer mode needs -contract ADDRESS: the manifest commits to the registry the bundle is registered in")
			os.Exit(2)
		}
		if *networkFlag == "" || *networkFlag == "unspecified" {
			fmt.Fprintln(os.Stderr, "prepare-offer mode needs -network LABEL: the manifest names the network it is for")
			os.Exit(2)
		}
		runPrepareOffer(*dataDir, uint16(*inventoryEpoch), *author, *nodeKeyPath, *bundleFlag, *blobDir,
			*contractFlag, *networkFlag, *policyFlag, *anchorFlag, *indexerURLFlag, *proofAnchorFlag, *inputFlag, *proverPath, *proofPath,
			*sweepFile, c)
		return
	}

	if *mode == "fees" {
		runFees(*operationFlag, *feePerGasFlag, *indexerURLFlag, *rpcURL, apiKey)
		return
	}

	if *mode == "market" {
		if *contractFlag == "" {
			fmt.Fprintln(os.Stderr, "market mode needs -contract ADDRESS")
			os.Exit(2)
		}
		runMarket(*contractFlag, *viewerFlag, *proofsFlag, *networkFlag, *out, *rpcURL, apiKey, *availabilityFlag)
		return
	}

	if *mode == "submit" {
		if *requestPath != "" {
			*preparedPath = *requestPath
		}
		if *preparedPath == "" || *nodeKeyPath == "" {
			fmt.Fprintln(os.Stderr, "submit mode needs -prepared FILE -nodekey FILE")
			os.Exit(2)
		}
		if *nonceFlag == "" || *txEpochFlag == "" || *maxFeeFlag == "" {
			fmt.Fprintln(os.Stderr, "submit mode needs -nonce N -tx-epoch N -max-fee WEI")
			os.Exit(2)
		}
		nonce, err := parseNonce(*nonceFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(2)
		}
		epoch, err := parseEpoch(*txEpochFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(2)
		}
		maxFee, ok := new(big.Int).SetString(*maxFeeFlag, 10)
		if !ok || maxFee.Sign() < 0 {
			fmt.Fprintf(os.Stderr, "max fee %q is not an integer amount\n", *maxFeeFlag)
			os.Exit(2)
		}
		tips := big.NewInt(0)
		if *tipsFlag != "" {
			parsed, ok := new(big.Int).SetString(*tipsFlag, 10)
			if !ok || parsed.Sign() < 0 {
				fmt.Fprintf(os.Stderr, "tips %q is not an integer amount\n", *tipsFlag)
				os.Exit(2)
			}
			tips = parsed
		}
		var feePerGas *big.Int
		if *feePerGasFlag != "" {
			parsed, ok := new(big.Int).SetString(*feePerGasFlag, 10)
			if !ok || parsed.Sign() <= 0 {
				fmt.Fprintf(os.Stderr, "fee per gas %q is not a positive integer amount\n", *feePerGasFlag)
				os.Exit(2)
			}
			feePerGas = parsed
		}
		runSubmit(*preparedPath, *nodeKeyPath, nonce, epoch, maxFee, tips, *signedPath, *broadcastFlag, *rpcURL, apiKey, feePerGas, *contractFlag, *codeHashFlag)
		return
	}

	if *mode == "prepare-deploy" {
		if *codePath == "" || *nodeKeyPath == "" {
			fmt.Fprintln(os.Stderr, "prepare-deploy mode needs -code FILE -nodekey FILE")
			os.Exit(2)
		}
		if *nonceFlag == "" || *txEpochFlag == "" || *maxFeeFlag == "" {
			fmt.Fprintln(os.Stderr, "prepare-deploy mode needs -nonce N -tx-epoch N -max-fee WEI")
			os.Exit(2)
		}
		nonce, err := parseNonce(*nonceFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(2)
		}
		epoch, err := parseEpoch(*txEpochFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(2)
		}
		amount, ok := new(big.Int).SetString(*amountFlag, 10)
		if !ok || amount.Sign() < 0 {
			fmt.Fprintf(os.Stderr, "amount %q is not an integer amount\n", *amountFlag)
			os.Exit(2)
		}
		maxFee, ok := new(big.Int).SetString(*maxFeeFlag, 10)
		if !ok || maxFee.Sign() < 0 {
			fmt.Fprintf(os.Stderr, "max fee %q is not an integer amount\n", *maxFeeFlag)
			os.Exit(2)
		}
		tips := big.NewInt(0)
		if *tipsFlag != "" {
			parsed, ok := new(big.Int).SetString(*tipsFlag, 10)
			if !ok || parsed.Sign() < 0 {
				fmt.Fprintf(os.Stderr, "tips %q is not an integer amount\n", *tipsFlag)
				os.Exit(2)
			}
			tips = parsed
		}
		var deployFeePerGas *big.Int
		if *feePerGasFlag != "" {
			parsed, ok := new(big.Int).SetString(*feePerGasFlag, 10)
			if !ok || parsed.Sign() <= 0 {
				fmt.Fprintf(os.Stderr, "fee per gas %q is not a positive integer amount\n", *feePerGasFlag)
				os.Exit(2)
			}
			deployFeePerGas = parsed
		}
		// The deploy is one signed transaction with nowhere to go if the caller
		// names no file: -signed is its own flag, and an explicit -out is
		// honoured too, because accepting a file name and writing nothing is a
		// trap (`docs/PROGRESS.md`, 2026-09-26).
		deployPath := *signedPath
		if deployPath == "" && flagWasSet("out") {
			deployPath = *out
		}
		if *v7SeedIDFlag != "" {
			if *v6SeedIDsFlag != "" {
				fmt.Fprintln(os.Stderr, "v7 deploy cannot use v6 seed IDs")
				os.Exit(1)
			}
			if !flagWasSet("amount") {
				amount = big.NewInt(1)
			}
			if err := runPrepareDeployV7(*codePath, *codeHashFlag, *nodeKeyPath, *deployNonceFlag, *v7SeedIDFlag, *networkFlag, deployPath, nonce, epoch, amount, maxFee, tips, deployFeePerGas, *broadcastFlag); err != nil {
				fmt.Fprintf(os.Stderr, "v7 deploy refused: %v\n", err)
				os.Exit(1)
			}
			return
		}
		runPrepareDeploy(*codePath, *nodeKeyPath, *deployNonceFlag, *v6SeedIDsFlag, *networkFlag, deployPath, nonce, epoch, amount, maxFee, tips, *broadcastFlag, *rpcURL, apiKey, deployFeePerGas)
		return
	}

	if *mode == "refund" {
		if *requestPath == "" {
			fmt.Fprintln(os.Stderr, "refund mode needs -request FILE")
			os.Exit(2)
		}
		runRefund(*requestPath, *preparedPath)
		return
	}

	if *mode == "reveal" {
		if *contractFlag == "" || *poolOrderFlag == 0 || *vaultPath == "" || *preparedPath == "" {
			fmt.Fprintln(os.Stderr, "reveal mode needs -contract ADDRESS -pool-order ID -vault FILE -prepared FILE")
			os.Exit(2)
		}
		runReveal(c, *contractFlag, *networkFlag, *poolOrderFlag, *vaultPath, *preparedPath)
		return
	}

	if *mode == "withdraw" {
		if *requestPath == "" {
			fmt.Fprintln(os.Stderr, "withdraw mode needs -request FILE")
			os.Exit(2)
		}
		runWithdraw(*requestPath, *preparedPath)
		return
	}

	if *mode == "withdraw-reserve" {
		if *requestPath == "" {
			fmt.Fprintln(os.Stderr, "withdraw-reserve mode needs -request FILE")
			os.Exit(2)
		}
		runReserveWithdraw(*requestPath, *preparedPath)
		return
	}

	if *mode == "watch" {
		if *watchOrder == 0 || *contractFlag == "" || *nodeKeyPath == "" {
			fmt.Fprintln(os.Stderr, "watch mode needs -order N -contract ADDRESS -nodekey FILE")
			os.Exit(2)
		}
		if err := runWatch(c, *watchOrder, *contractFlag, *networkFlag, *nodeKeyPath, *serveInterval, *rpcURL, apiKey); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if *mode == "export" {
		if *sweepFile == "" || *vaultPath == "" || *blobDir == "" || *out == "" {
			fmt.Fprintln(os.Stderr, "export mode needs -inventory FILE -vault FILE -blob-dir DIR -out DIR")
			os.Exit(2)
		}
		passphrase, err := readPassphrase("Passphrase: ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		exported, err := ExportSessionContent(ExportOptions{
			InventoryPath: *sweepFile,
			VaultPath:     *vaultPath,
			Passphrase:    passphrase,
			BlobDir:       *blobDir,
			OutDir:        *out,
			AnswersPath:   *answersPath,
			AnswersSource: *answersSource,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("exported %d flips for %s epoch %d\n", exported.Bundle.Flips, exported.Bundle.Author, exported.Bundle.Epoch)
		fmt.Printf("manifest commitment %s\n", exported.Bundle.ManifestCommitment)
		fmt.Printf("answers: %s\n", exported.AnswersBy)
		fmt.Printf("keys: %s\n", exported.Keys)
		fmt.Printf("written to %s\n", *out)
		return
	}

	if *mode == "inventory" {
		if *author == "" {
			fmt.Fprintln(os.Stderr, "inventory mode needs -author ADDR, with either -sweep FILE (the node route) or -indexer URL (the Plan B route)")
			os.Exit(2)
		}
		if (*sweepFile == "") == (*indexerURLFlag == "") {
			fmt.Fprintln(os.Stderr, "inventory mode takes exactly one source: -sweep FILE for the node route, or -indexer URL for the Plan B route")
			os.Exit(2)
		}
		var inventory *BundleInventory
		var err error
		if *sweepFile != "" {
			inventory, err = buildInventory(*sweepFile, *author, uint16(*inventoryEpoch),
				oneSource(ipfsFetch(c), "node ipfs"), *blobsOut, nodeCIDComputer(c))
		} else {
			if *inventoryEpoch == 0 {
				fmt.Fprintln(os.Stderr, "inventory mode with -indexer needs -inventory-epoch N: the explorer's flip routes are per epoch")
				os.Exit(2)
			}
			inventory, err = buildIndexerInventory(newHTTPClient(), *indexerURLFlag, uint16(*inventoryEpoch), *author,
				nodeThenGateway(ipfsFetch(c), gatewayFetch(*gatewayFlag)), *blobsOut, nodeCIDComputer(c), c, *requireNodeFlag)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "inventory failed: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out, inventory); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		if *availabilityOutFlag != "" {
			advisory := availabilityFromInventory(inventory)
			if err := writeJSON(*availabilityOutFlag, advisory); err != nil {
				fmt.Fprintf(os.Stderr, "write %s: %v\n", *availabilityOutFlag, err)
				os.Exit(1)
			}
			fmt.Printf("availability %d/%d flips verified, complete=%v -> %s\n",
				advisory.VerifiedFlips, advisory.CoveredFlips, advisory.Complete, *availabilityOutFlag)
		}
		fmt.Printf("author=%s epoch=%d source=%q submissions=%d unique=%d duplicates=%d deleted=%d blobs_ok=%d blobs_failed=%d complete=%v node_verified=%v -> %s\n",
			inventory.Author, inventory.Epoch, inventory.Source, inventory.Submissions, inventory.UniqueCIDs,
			inventory.Duplicates, inventory.Deleted, inventory.BlobsOk, inventory.BlobsFailed,
			inventory.Complete, inventory.NodeVerified, *out)
		if inventory.NodeNote != "" {
			fmt.Printf("node    %s\n", inventory.NodeNote)
		}
		for _, note := range inventory.Notes {
			fmt.Printf("note    %s\n", note)
		}
		return
	}

	if *mode == "decode-answers" {
		if *txHash == "" {
			fmt.Fprintln(os.Stderr, "decode-answers mode needs -tx HASH")
			os.Exit(2)
		}
		var raw hexutil.Bytes
		if err := c.call("bcn_transaction", []any{*txHash}, &raw); err != nil {
			// bcn_transaction returns a full transaction object, not raw bytes.
			_ = err
		}
		var rpcTx rpcTx
		if err := c.call("bcn_transaction", []any{*txHash}, &rpcTx); err != nil {
			fmt.Fprintf(os.Stderr, "fetch tx: %v\n", err)
			os.Exit(1)
		}
		tx := &types.Transaction{Type: types.SubmitLongAnswersTx, Payload: []byte(rpcTx.Payload), Epoch: rpcTx.Epoch}
		summary, err := decodeLongAnswers(tx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "decode: %v\n", err)
			os.Exit(1)
		}
		summary.TxHash = rpcTx.Hash
		summary.Author = rpcTx.From
		if err := writeJSON(*out, summary); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Printf("tx=%s author=%s answers=%dB inferred_flips=%d left=%d right=%d reported=%d A=%d B=%d C=%d D=%d -> %s\n",
			summary.TxHash, summary.Author, summary.AnswerBytes, summary.InferredFlips,
			summary.Left, summary.Right, summary.GradeReported, summary.GradeA, summary.GradeB,
			summary.GradeC, summary.GradeD, *out)
		return
	}

	if *mode == "eligibility" {
		if *dataDir == "" || *inventoryEpoch == 0 || *author == "" {
			fmt.Fprintln(os.Stderr, "eligibility mode needs -datadir PATH -inventory-epoch N -author ADDRESS, and -out FILE")
			os.Exit(2)
		}
		runEligibility(*dataDir, uint16(*inventoryEpoch), *author, *policyFlag, *out, *indexerURLFlag, *sweepFile, c, *requireNodeFlag, *epochBlockFlag)
		return
	}

	if *mode == "assignment" {
		if *dataDir == "" || *inventoryEpoch == 0 {
			fmt.Fprintln(os.Stderr, "assignment mode needs -datadir PATH -inventory-epoch N, and -out FILE for the assignment")
			os.Exit(2)
		}
		runAssignment(*dataDir, uint16(*inventoryEpoch), *out, *indexerURLFlag, c, *epochBlockFlag)
		return
	}

	if *mode == "epoch-db" {
		if *dataDir == "" {
			fmt.Fprintln(os.Stderr, "epoch-db mode needs -datadir PATH and -inventory-epoch N")
			os.Exit(2)
		}
		if *inventoryEpoch == 0 {
			total, prefixes, err := dumpDbLayout(*dataDir, 12)
			if err != nil {
				fmt.Fprintf(os.Stderr, "db layout failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("chain db keys: %d\n", total)
			for _, p := range prefixes {
				fmt.Printf("  prefix %s count=%d sample=%s\n", p.PrefixHex, p.Count, p.SampleHex)
			}
			return
		}
		summary, err := readEpochDb(*dataDir, uint16(*inventoryEpoch))
		if err != nil {
			fmt.Fprintf(os.Stderr, "epoch-db failed: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out, summary); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Printf("epoch=%d seed_bytes=%d candidates=%d assigned_flips=%d states=%v -> %s\n",
			summary.Epoch, summary.SeedBytes, summary.Candidates, summary.TotalFlips, summary.StateCounts, *out)
		return
	}

	if *mode == "state-at" {
		if *dataDir == "" || *stateHeight == 0 {
			fmt.Fprintln(os.Stderr, "state-at mode needs -datadir PATH and -state-height N")
			os.Exit(2)
		}
		summary, err := readStateAt(*dataDir, int64(*stateHeight), 5)
		if err != nil {
			fmt.Fprintf(os.Stderr, "state-at failed: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out, summary); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Printf("height=%d identities=%d candidates=%d candidate_flips=%d states=%v -> %s\n",
			summary.Height, summary.Identities, summary.Candidates, summary.TotalFlips, summary.StateCounts, *out)
		return
	}

	if *mode == "state-proof" {
		if *dataDir == "" {
			fmt.Fprintln(os.Stderr, "state-proof mode needs -datadir PATH; -state-height 0 lists the versions the datadir holds")
			os.Exit(2)
		}
		if *bundleOut != "" {
			var addresses []string
			if *addressesFlag != "" {
				loaded, err := readAddressList(*addressesFlag)
				if err != nil {
					fmt.Fprintf(os.Stderr, "state-proof bundle failed: %v\n", err)
					os.Exit(1)
				}
				addresses = loaded
			}
			bundle, err := readProofBundle(*dataDir, int64(*stateHeight), *bundleLimit, *proofTree, addresses, *expectRootFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "state-proof bundle failed: %v\n", err)
				os.Exit(1)
			}
			if err := writeJSON(*bundleOut, bundle); err != nil {
				fmt.Fprintf(os.Stderr, "write %s: %v\n", *bundleOut, err)
				os.Exit(1)
			}
			fmt.Printf("tree=%s height=%d proofs=%d root=%s -> %s\n",
				bundle.Tree, bundle.Height, len(bundle.Proofs), bundle.RootHex, *bundleOut)
			return
		}
		proof, err := readStateProof(*dataDir, int64(*stateHeight), *identityFlag, *proofTree)
		if err != nil {
			fmt.Fprintf(os.Stderr, "state-proof failed: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out, proof); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		if *stateHeight == 0 {
			newest := 0
			for _, version := range proof.Versions {
				if version > newest {
					newest = version
				}
			}
			fmt.Printf("versions=%d newest=%d -> %s\n", len(proof.Versions), newest, *out)
			return
		}
		fmt.Printf("tree=%s height=%d identity_keys=%d address=%s identity_bytes=%d proof_bytes=%d path_items=%d verified=%v -> %s\n",
			proof.Tree, proof.Height, proof.IdentityKeys, proof.Address, proof.IdentityBytes, proof.ProofBytes, proof.PathItems, proof.Verified, *out)
		return
	}

	if *mode == "session-pool" {
		if *exportDirFlag == "" || *out == "" || *sessionIDFlag == "" || *utilityIDFlag == "" || *thresholdFlag == 0 || *closesAtFlag == "" {
			fmt.Fprintln(os.Stderr, "session-pool mode needs -export-dir DIR -out FILE -session-id ID -utility-id ID -threshold N -closes-at RFC3339 [-opens-at RFC3339]")
			os.Exit(2)
		}
		pool, err := BuildSessionPool(SessionPoolOptions{
			ExportDir: *exportDirFlag,
			OutPath:   *out,
			SessionID: *sessionIDFlag,
			UtilityID: *utilityIDFlag,
			Threshold: *thresholdFlag,
			OpensAt:   *opensAtFlag,
			ClosesAt:  *closesAtFlag,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "session pool failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("pool        %s v%d, %d challenges, threshold %d -> %s\n",
			pool.Format, pool.Version, len(pool.Challenges), pool.Rules.PassThreshold, *out)
		fmt.Println("load it     point the gateway's \"poolPath\" at that file; the gateway refuses content that is not a flip")
		return
	}

	if *mode == "shared-node" {
		if *vaultPath == "" || *sweepFile == "" || *blobDir == "" || *out == "" ||
			*sessionIDFlag == "" || *utilityIDFlag == "" || *thresholdFlag == 0 || *closesAtFlag == "" || *sharedNodeURL == "" {
			fmt.Fprintln(os.Stderr,
				"shared-node mode needs -vault FILE -inventory FILE -blob-dir DIR -out DIR -node-url URL "+
					"-session-id ID -utility-id ID -threshold N -closes-at RFC3339 [-opens-at RFC3339] "+
					"[-node-id NAME] [-custody hosted|self]")
			os.Exit(2)
		}
		passphrase, err := readPassphrase("Vault passphrase: ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		result, err := BuildSharedNode(SharedNodeOptions{
			VaultPath:     *vaultPath,
			InventoryPath: *sweepFile,
			BlobDir:       *blobDir,
			OutDir:        *out,
			AnswersPath:   *answersPath,
			AnswersSource: *answersSource,
			SessionID:     *sessionIDFlag,
			UtilityID:     *utilityIDFlag,
			Threshold:     *thresholdFlag,
			OpensAt:       *opensAtFlag,
			ClosesAt:      *closesAtFlag,
			NodeID:        *sharedNodeID,
			NodeURL:       *sharedNodeURL,
			Custody:       *sharedNodeCustody,
			Passphrase:    passphrase,
			// No fixed salt: the pool draws a fresh per-session one (`AH-2`).
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "shared node failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("exported    %d flips for %s epoch %d -> %s\n",
			result.Export.Bundle.Flips, result.Connection.Author, result.Export.Bundle.Epoch, result.ExportDir)
		fmt.Printf("pool        %s v%d, %d challenges, threshold %d -> %s\n",
			result.Pool.Format, result.Pool.Version, len(result.Pool.Challenges), result.Pool.Rules.PassThreshold, result.PoolPath)
		fmt.Printf("connection  %s -> %s\n", result.Connection.NodeID, result.ConnectionPath)
		fmt.Println("author side import that file under \"Sell through a shared node\"; listing and donation stay the author's action")
		fmt.Println("gateway     point \"poolPath\" at the pool above; keep the desk running with `fod serve -contract ADDRESS -vault FILE`")
		return
	}

	if *mode == "session-anchor" {
		if *dataDir == "" || *stateHeight == 0 || *author == "" || *out == "" {
			fmt.Fprintln(os.Stderr, "session-anchor mode needs -datadir PATH -state-height N -author ADDRESS -out FILE")
			os.Exit(2)
		}
		anchor, err := readSessionAnchor(*dataDir, int64(*stateHeight), *author)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session anchor failed: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out, anchor); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Printf("height=%d root=%s -> %s\n", anchor.Height, anchor.RootHex, *out)
		return
	}

	if *mode == "pre-session-witness" {
		if *preSessionInventory == "" || *nodeKeyPath == "" || *blobDir == "" || !flagWasSet("out") {
			fmt.Fprintln(os.Stderr, "pre-session-witness needs -pre-session-inventory FILE -nodekey FILE -blob-dir DIR -out FILE")
			os.Exit(2)
		}
		if err := writePreSessionWitness(*out, *preSessionInventory, *nodeKeyPath, *blobDir); err != nil {
			fmt.Fprintf(os.Stderr, "pre-session witness failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("private pre-session content witness created; no proof or transaction was created")
		return
	}

	if *mode == "session-bundle" {
		if *author == "" || *out == "" {
			fmt.Fprintln(os.Stderr, "session-bundle mode needs -author ADDRESS -out FILE")
			os.Exit(2)
		}
		var bundle *SessionBundle
		var err error
		if *inventoryAnchor != "" || *verdictAnchor != "" {
			if *inventoryAnchor == "" || *verdictAnchor == "" || *dataDir != "" || *inventoryHeight != 0 || *verdictHeight != 0 {
				fmt.Fprintln(os.Stderr, "saved-anchor mode needs both -inventory-anchor and -verdict-anchor, without datadir or heights")
				os.Exit(2)
			}
			if *sessionBlobOut != "" && *nodeKeyPath == "" {
				fmt.Fprintln(os.Stderr, "saved-anchor -blob-out needs -nodekey FILE for the author's epoch secrets")
				os.Exit(2)
			}
			inventory, readErr := readSessionAnchorFile(*inventoryAnchor)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "inventory anchor failed: %v\n", readErr)
				os.Exit(1)
			}
			verdict, readErr := readSessionAnchorFile(*verdictAnchor)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "verdict anchor failed: %v\n", readErr)
				os.Exit(1)
			}
			bundle, err = sessionBundleFromAnchors(*author, inventory, verdict)
		} else {
			if *dataDir == "" || *inventoryHeight == 0 || *verdictHeight == 0 {
				fmt.Fprintln(os.Stderr, "session-bundle mode needs -datadir PATH, -inventory-height N, -verdict-height N and -author ADDRESS")
				os.Exit(2)
			}
			bundle, err = readSessionBundle(*dataDir, int64(*inventoryHeight), int64(*verdictHeight), *author)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "session bundle failed: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out, bundle); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Printf("author=%s epoch=%d flips=%d inventory=%d verdict=%d -> %s\n",
			bundle.Author, bundle.Epoch, len(bundle.Manifest), bundle.Inventory.Height, bundle.Verdict.Height, *out)
		if *sessionBlobOut != "" {
			if *blobPath == "" && *blobDir == "" && !flagWasSet("rpc") {
				fmt.Fprintln(os.Stderr, "-blob-out needs -blob-dir DIR (CID-named flip blobs), -rpc URL, or -blob FILE for one flip")
				os.Exit(2)
			}
			var node *client
			if flagWasSet("rpc") {
				node = &client{url: *rpcURL, key: apiKey, http: newHTTPClient()}
			}
			if err := writeRegistrationBlobInput(*sessionBlobOut, *dataDir, *nodeKeyPath, *blobPath, *blobDir, bundle, node); err != nil {
				fmt.Fprintf(os.Stderr, "blob witness failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("blob witness -> %s (private material; keep it with the seller's keys)\n", *sessionBlobOut)
		}
		return
	}

	if *mode == "seed-at" {
		if *dataDir == "" || *stateHeight == 0 {
			fmt.Fprintln(os.Stderr, "seed-at mode needs -datadir PATH and -state-height N")
			os.Exit(2)
		}
		seed, err := readBlockSeedAt(*dataDir, *stateHeight)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed-at failed: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out, seed); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Printf("height=%d hash=%s seed=%s (%d bytes) -> %s\n",
			seed.Height, seed.BlockHash, seed.SeedHex, seed.SeedBytes, *out)
		return
	}

	if *mode == "epoch-keys" {
		if *dataDir == "" {
			fmt.Fprintln(os.Stderr, "epoch-keys mode needs -datadir PATH")
			os.Exit(2)
		}
		// A key census is not a sweep, and the shared -out default names one.
		// Writing a file called sweep.json from a read-only key listing invites
		// feeding it back to -sweep later, so this mode names its own output
		// when the caller did not (`docs/PROGRESS.md`, 2026-09-26).
		if !flagWasSet("out") {
			censusPath := "epoch-keys.json"
			out = &censusPath
		}
		summary, err := readEpochKeys(*dataDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "epoch-keys failed: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out, summary); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		fmt.Printf("epoch keys total: %d\n", summary.Total)
		for epoch, bucket := range summary.Epochs {
			fmt.Printf("  epoch %s: %v\n", epoch, bucket)
		}
		return
	}

	if *mode == "derive-keys" {
		if *nodeKeyPath == "" || *inventoryEpoch == 0 {
			fmt.Fprintln(os.Stderr, "derive-keys mode needs -nodekey PATH and -inventory-epoch N")
			os.Exit(2)
		}
		identityKey, err := crypto.LoadECDSA(*nodeKeyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load nodekey: %v\n", err)
			os.Exit(1)
		}
		keys, err := DeriveEpochKeys(identityKey, uint16(*inventoryEpoch))
		if err != nil {
			fmt.Fprintf(os.Stderr, "derive keys: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out, keys); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
			os.Exit(1)
		}
		// Only public material and fingerprints are printed or written. The
		// private halves stay in memory and are exported only through the
		// encrypted-vault path, which is not built yet.
		fmt.Printf("address=%s epoch=%d\n", keys.Address, keys.Epoch)
		fmt.Printf("public-part key  %s (fp %s)\n", keys.PublicPartKey, keys.PublicPartFP)
		fmt.Printf("private-part key %s (fp %s)\n", keys.PrivatePartKey, keys.PrivatePartFP)
		fmt.Printf("public summary written to %s (no secret material)\n", *out)
		return
	}

	if *mode == "export-keys" {
		if *nodeKeyPath == "" || *inventoryEpoch == 0 || *vaultPath == "" {
			fmt.Fprintln(os.Stderr, "export-keys mode needs -nodekey PATH -inventory-epoch N -vault FILE")
			os.Exit(2)
		}
		identityKey, err := crypto.LoadECDSA(*nodeKeyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load nodekey: %v\n", err)
			os.Exit(1)
		}
		keys, err := DeriveEpochKeys(identityKey, uint16(*inventoryEpoch))
		if err != nil {
			fmt.Fprintf(os.Stderr, "derive keys: %v\n", err)
			os.Exit(1)
		}
		passphrase, err := confirmPassphrase()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		payload, err := json.Marshal(keys.ToVaultPayload())
		if err != nil {
			fmt.Fprintf(os.Stderr, "encode vault payload: %v\n", err)
			os.Exit(1)
		}
		if err := WriteVault(*vaultPath, passphrase, payload); err != nil {
			fmt.Fprintf(os.Stderr, "write vault: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (owner-only) for address %s epoch %d\n", *vaultPath, keys.Address, keys.Epoch)
		fmt.Printf("public-part key  %s (fp %s)\n", keys.PublicPartKey, keys.PublicPartFP)
		fmt.Printf("private-part key %s (fp %s)\n", keys.PrivatePartKey, keys.PrivatePartFP)
		return
	}

	if *mode == "import-keys" {
		if *vaultPath == "" {
			fmt.Fprintln(os.Stderr, "import-keys mode needs -vault FILE")
			os.Exit(2)
		}
		passphrase, err := readPassphrase("Passphrase: ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		payload, err := ReadVault(*vaultPath, passphrase)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		var vault VaultPayload
		if err := json.Unmarshal(payload, &vault); err != nil {
			fmt.Fprintf(os.Stderr, "parse vault payload: %v\n", err)
			os.Exit(1)
		}
		if vault.Kind != "fod.epoch-keys" || vault.Version != 1 {
			fmt.Fprintf(os.Stderr, "unexpected vault payload: kind=%q version=%d\n", vault.Kind, vault.Version)
			os.Exit(1)
		}
		secure, err := VaultPermissionsSecure(*vaultPath)
		if err == nil && !secure {
			fmt.Fprintln(os.Stderr, "warning: vault file permissions are wider than owner-only")
		}
		fmt.Printf("address=%s epoch=%d\n", vault.Address, vault.Epoch)
		fmt.Printf("public-part key  %s (fp %s)\n", vault.PublicPartKey, vault.PublicPartFP)
		fmt.Printf("private-part key %s (fp %s)\n", vault.PrivatePartKey, vault.PrivatePartFP)
		fmt.Printf("secrets loaded: %d and %d hex chars (not printed)\n", len(vault.PublicPartSec), len(vault.PrivatePartSec))
		return
	}

	if *mode == "receive" {
		// Three forms, one command: `-pool-order` opens the delivery the pool
		// recorded, `-offer` opens the delivery the offer slice published
		// (`SPEC.md` 6.5 steps 7-8), and `-vault` with `-blob` opens a blob
		// with the keys a vault holds.
		if *poolOrderFlag != 0 {
			if *contractFlag == "" || *deliveryKey == "" || *preparedPath == "" || !flagWasSet("out") {
				fmt.Fprintln(os.Stderr, "receive with -pool-order needs -contract ADDRESS -delivery-key FILE -prepared BUY_FILE -out RECORD_FILE")
				os.Exit(2)
			}
			purchase, err := readRetainedPurchase(*preparedPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "purchase refused: %v\n", err)
				os.Exit(1)
			}
			if purchase.Fast != nil {
				input, err := prepareFastRecovery(c, *contractFlag, *poolOrderFlag, purchase, *deliveryKey)
				if err != nil {
					fmt.Fprintf(os.Stderr, "fast recovery refused: %v\n", err)
					os.Exit(1)
				}
				if err := writePrivateJSON(*out, input); err != nil {
					fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
					os.Exit(1)
				}
				fmt.Printf("settled fast delivery and %d retained encrypted copies checked; recovery input written owner-only to %s\n", len(input.Copies), *out)
				fmt.Println("open the content locally with the fast-content recover binary and your delivery key; no confirmation transaction is needed")
				return
			}
			if *vaultPath == "" {
				fmt.Fprintln(os.Stderr, "ordinary pool receive also needs -vault FILE")
				os.Exit(2)
			}
			passphrase, err := confirmPassphrase()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			opened, err := openPoolDeliveryFromRegistry(c, *contractFlag, *poolOrderFlag, *deliveryKey, *vaultPath, passphrase)
			if err != nil {
				fmt.Fprintf(os.Stderr, "receive failed: %v\n", err)
				os.Exit(1)
			}
			if err := verifyPurchasedContent(c, purchase, opened, passphrase, *out); err != nil {
				fmt.Fprintf(os.Stderr, "content verification failed: %v\n", err)
				os.Exit(1)
			}
			receiptKey, err := loadDeliveryKey(*deliveryKey)
			if err != nil {
				fmt.Fprintln(os.Stderr, "receipt key unavailable:", err)
				os.Exit(1)
			}
			if err := signOpenedDelivery(opened, receiptKey); err != nil {
				fmt.Fprintln(os.Stderr, "receipt refused:", err)
				os.Exit(1)
			}
			for _, check := range opened.Checks {
				fmt.Printf("checked     %s\n", check)
			}
			for _, note := range opened.NotChecked {
				fmt.Printf("not checked %s\n", note)
			}
			fmt.Printf("opened      pool order %d, epoch %d, author %s\n", opened.OrderID, opened.Epoch, opened.Author)
			fmt.Printf("wrote       %s (owner-only); verified %d retained flips; content paths are in %s\n", opened.VaultPath, opened.UsableFlips, *out)
			if opened.State == "settled" {
				fmt.Println("ready       this order is already settled; use the recovered content without another confirmation")
			} else {
				fmt.Printf("next        `fod prepare-confirm -contract %s -pool-order %d -delivery <this record> -prepared confirm.json`, then sign it with `fod submit`\n",
					opened.Contract, opened.OrderID)
			}
			if flagWasSet("out") {
				if err := writePrivateJSON(*out, opened); err != nil {
					fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
					os.Exit(1)
				}
			}
			return
		}
		if *vaultPath == "" || *blobPath == "" {
			fmt.Fprintln(os.Stderr, "receive mode needs -vault FILE and -blob FILE (or -pool-order ID for the pool path)")
			os.Exit(2)
		}
		passphrase, err := readPassphrase("Passphrase: ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		result, err := receiveFlip(*vaultPath, *blobPath, *out, passphrase)
		if err != nil {
			fmt.Fprintf(os.Stderr, "receive failed: %v\n", err)
			os.Exit(1)
		}
		if err := writeJSON(*out+".json", result); err != nil {
			fmt.Fprintf(os.Stderr, "write summary: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("decrypted with %s (epoch %d): public part %d bytes, private part %d bytes\n",
			result.DecryptingAddress, result.Epoch, result.PublicPartBytes, result.PrivatePartBytes)
		fmt.Printf("summary written to %s.json\n", *out)
		return
	}

	if *from == 0 || *to == 0 || *to < *from {
		fmt.Fprintln(os.Stderr, "usage: fod sync -rpc URL -key-file FILE -from H -to H -out FILE")
		os.Exit(2)
	}

	result, err := sweep(c, *from, *to, *epochFilter, *conc, *bodySource)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sweep failed: %v\n", err)
		os.Exit(1)
	}
	if err := writeJSON(*out, result); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Printf("blocks=%d bodies=%d cid_verified=%d txset_mismatches=%d txs=%d submissions=%d deletions=%d -> %s\n",
		result.BlocksSeen, result.BlocksWithBody, result.BodiesVerified, result.TxSetMismatch,
		result.TxsSeen, len(result.Submissions), len(result.Deletions), *out)
	fmt.Printf("tx types: %v\n", result.TypeCounts)
}

// blockInfo is what the header stage collects for one block.
type blockInfo struct {
	height   uint64
	hash     string
	ipfsCid  string
	txHashes []string
}

// sweep walks the block range in parallel, then reconstructs transactions from
// either the node's transaction API or the CID-committed block bodies.
func sweep(c *client, from, to uint64, epochFilter int, concurrency int, bodySource string) (*SweepResult, error) {
	result := &SweepResult{
		FromHeight:    from,
		ToHeight:      to,
		TypeCounts:    map[string]int{},
		Submissions:   []Submission{},
		Deletions:     []Deletion{},
		Txs:           []TxSummary{},
		MissingHashes: []string{},
		BodyChecks:    []BodyCheck{},
	}

	type blockJob struct{ height uint64 }
	type blockOut struct {
		block *rpcBlock
		err   error
	}

	jobs := make(chan blockJob)
	outs := make(chan blockOut)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				var b rpcBlock
				err := c.call("bcn_blockAt", []any{job.height}, &b)
				outs <- blockOut{block: &b, err: err}
			}
		}()
	}
	go func() {
		for h := from; h <= to; h++ {
			jobs <- blockJob{height: h}
		}
		close(jobs)
		wg.Wait()
		close(outs)
	}()

	var blocks []blockInfo
	for out := range outs {
		if out.err != nil {
			return nil, fmt.Errorf("block fetch: %w", out.err)
		}
		result.BlocksSeen++
		info := blockInfo{height: out.block.Height, hash: out.block.Hash, ipfsCid: out.block.IpfsCid}
		info.txHashes = append(info.txHashes, out.block.Transactions...)
		blocks = append(blocks, info)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].height < blocks[j].height })

	if bodySource == "ipfs" {
		return sweepFromBodies(c, blocks, epochFilter, concurrency, result)
	}

	var txHashes []txRef
	for _, b := range blocks {
		for _, h := range b.txHashes {
			txHashes = append(txHashes, txRef{hash: h, height: b.height})
		}
	}
	sort.Slice(txHashes, func(i, j int) bool { return txHashes[i].height < txHashes[j].height })

	// Fetch transaction bodies in parallel; keep deterministic ordering by height.
	type txOut struct {
		ref txRef
		tx  *rpcTx
		err error
	}
	txJobs := make(chan txRef)
	txOuts := make(chan txOut)
	var wg2 sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			for ref := range txJobs {
				var tx rpcTx
				err := c.call("bcn_transaction", []any{ref.hash}, &tx)
				txOuts <- txOut{ref: ref, tx: &tx, err: err}
			}
		}()
	}
	go func() {
		for _, ref := range txHashes {
			txJobs <- ref
		}
		close(txJobs)
		wg2.Wait()
		close(txOuts)
	}()

	var txs []txWithHeight
	for out := range txOuts {
		if out.err != nil {
			return nil, fmt.Errorf("tx fetch %s: %w", out.ref.hash, out.err)
		}
		txs = append(txs, txWithHeight{tx: out.tx, height: out.ref.height, refHash: out.ref.hash})
	}
	sort.Slice(txs, func(i, j int) bool { return txs[i].height < txs[j].height })

	for _, item := range txs {
		result.TxsSeen++
		// A null RPC result leaves the transaction empty: the block header
		// references a body this node does not serve. Record it rather than
		// silently treating it as an empty transaction.
		if item.tx.Hash == "" {
			result.MissingTxBodies++
			result.MissingHashes = append(result.MissingHashes, item.refHash)
			result.Txs = append(result.Txs, TxSummary{
				TxHash:      item.refHash,
				BlockHeight: item.height,
				Type:        "<missing-body>",
				PayloadLen:  -1,
			})
			continue
		}
		result.TypeCounts[item.tx.Type]++
		result.Txs = append(result.Txs, TxSummary{
			TxHash:      item.tx.Hash,
			BlockHeight: item.height,
			Type:        item.tx.Type,
			Epoch:       item.tx.Epoch,
			From:        item.tx.From,
			PayloadLen:  len(item.tx.Payload),
		})
		if epochFilter >= 0 && int(item.tx.Epoch) != epochFilter {
			continue
		}
		switch item.tx.Type {
		case "submitFlip":
			sub, ok := decodeSubmission(item)
			if ok {
				result.Submissions = append(result.Submissions, sub)
			}
		case "deleteFlip":
			del, ok := decodeDeletion(item)
			if ok {
				result.Deletions = append(result.Deletions, del)
			}
		case "submitLongAnswers":
			if payload := longAnswerPayload(item.tx.Payload); payload != "" {
				result.Answers = append(result.Answers, LongAnswerTx{
					TxHash:      item.tx.Hash,
					BlockHeight: item.height,
					Author:      item.tx.From,
					Epoch:       item.tx.Epoch,
					PayloadHex:  payload,
				})
			}
		}
	}
	return result, nil
}

type txRef struct {
	hash   string
	height uint64
}

type txWithHeight struct {
	tx      *rpcTx
	height  uint64
	refHash string
}

// decodeSubmission parses a submitFlip payload with the reference attachment
// type, so the wire format has exactly one definition in this codebase.
func decodeSubmission(item txWithHeight) (Submission, bool) {
	var attachment attachments.FlipSubmitAttachment
	if err := attachment.FromBytes([]byte(item.tx.Payload)); err != nil {
		return Submission{}, false
	}
	return Submission{
		TxHash:      item.tx.Hash,
		BlockHeight: item.height,
		Author:      item.tx.From,
		Epoch:       item.tx.Epoch,
		CID:         cidString(attachment.Cid),
		CIDBytes:    fmt.Sprintf("%x", attachment.Cid),
		Pair:        attachment.Pair,
	}, true
}

// decodeDeletion parses a deleteFlip payload with the reference attachment type.
func decodeDeletion(item txWithHeight) (Deletion, bool) {
	var attachment attachments.DeleteFlipAttachment
	if err := attachment.FromBytes([]byte(item.tx.Payload)); err != nil {
		return Deletion{}, false
	}
	return Deletion{
		TxHash:      item.tx.Hash,
		BlockHeight: item.height,
		Author:      item.tx.From,
		Epoch:       item.tx.Epoch,
		CID:         cidString(attachment.Cid),
		CIDBytes:    fmt.Sprintf("%x", attachment.Cid),
	}, true
}

// longAnswerPayload returns the hex form of a long-answer payload the reference
// attachment type parses, or an empty string when the payload is not one.
//
// The whole payload is kept, not just its answer bits, because that is what the
// node's own epoch store holds for the same transaction
// (`qualification.addAnswers`) - which is what lets the two sources be compared.
func longAnswerPayload(payload []byte) string {
	attachment := attachments.ParseLongAnswerBytesAttachment(payload)
	if attachment == nil || len(attachment.Answers) == 0 {
		return ""
	}
	return hex.EncodeToString(payload)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// writePrivateJSON writes a file that carries key material. It is 0600 on
// create, with atomic replacement for an existing file. The staged file is
// private before any data is written, and a target symlink is replaced rather
// than followed.
func writePrivateJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateBytes(path, data)
}

// writePrivateBytes applies writePrivateJSON's rule to outputs that are not
// JSON: 0600 on create and on atomic replacement, including a file that
// previously existed with wider permissions. The receive and export outputs carry decrypted
// flip material, so they must not keep a wider mode from an earlier run
// (`AH-20`).
func writePrivateBytes(path string, data []byte) error {
	return writePrivateFile(path, true, func(file *os.File) error {
		_, err := file.Write(data)
		return err
	})
}

// sweepFromBodies reconstructs transactions from CID-committed block bodies.
//
// Every body is fetched by the CID the block header reports, its bytes are
// verified against that CID, and the decoded transaction hashes are compared
// with the hashes the node lists for the same block.
func sweepFromBodies(c *client, blocks []blockInfo, epochFilter, concurrency int, result *SweepResult) (*SweepResult, error) {
	type job struct{ idx int }
	type out struct {
		idx   int
		check BodyCheck
		txs   []*types.Transaction
	}

	jobs := make(chan job)
	outs := make(chan out)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				b := blocks[j.idx]
				if len(b.txHashes) == 0 {
					outs <- out{idx: j.idx, check: BodyCheck{Height: b.height, BlockHash: b.hash, Error: "empty block"}}
					continue
				}
				var decoded []*types.Transaction
				fetch := func(cid string) ([]byte, error) {
					var hexBody hexutil.Bytes
					if err := c.call("ipfs_get", []any{cid}, &hexBody); err != nil {
						return nil, err
					}
					return []byte(hexBody), nil
				}
				check := checkBlockBody(b.height, b.hash, b.ipfsCid, b.txHashes, fetch)
				if check.Decoded {
					var body types.Body
					raw, err := fetch(b.ipfsCid)
					if err == nil {
						_ = body.DecodeBytes(raw)
						decoded = body.Transactions
					}
				}
				outs <- out{idx: j.idx, check: check, txs: decoded}
			}
		}()
	}
	go func() {
		for i := range blocks {
			jobs <- job{idx: i}
		}
		close(jobs)
		wg.Wait()
		close(outs)
	}()

	decodedTxs := make([][]*types.Transaction, len(blocks))
	for o := range outs {
		if o.check.BodyBytes > 0 {
			result.BlocksWithBody++
		}
		if o.check.CIDMatches {
			result.BodiesVerified++
		}
		if !o.check.TxSetMatches && o.check.Decoded {
			result.TxSetMismatch++
		}
		result.BodyChecks = append(result.BodyChecks, o.check)
		decodedTxs[o.idx] = o.txs
	}
	sort.Slice(result.BodyChecks, func(i, j int) bool { return result.BodyChecks[i].Height < result.BodyChecks[j].Height })

	for i, txs := range decodedTxs {
		h := blocks[i].height
		for _, tx := range txs {
			result.TxsSeen++
			typeName := txTypeName(tx.Type)
			result.TypeCounts[typeName]++
			if epochFilter >= 0 && int(tx.Epoch) != epochFilter {
				continue
			}
			author := ""
			if addr, err := types.Sender(tx); err == nil {
				author = addr.String()
			}
			result.Txs = append(result.Txs, TxSummary{
				TxHash:      tx.Hash().String(),
				BlockHeight: h,
				Type:        typeName,
				Epoch:       tx.Epoch,
				From:        author,
				PayloadLen:  len(tx.Payload),
			})
			switch tx.Type {
			case types.SubmitFlipTx:
				if a := attachments.ParseFlipSubmitAttachment(tx); a != nil {
					result.Submissions = append(result.Submissions, Submission{
						TxHash:      tx.Hash().String(),
						BlockHeight: h,
						Author:      author,
						Epoch:       tx.Epoch,
						CID:         cidString(a.Cid),
						CIDBytes:    fmt.Sprintf("%x", a.Cid),
						Pair:        a.Pair,
					})
				}
			case types.DeleteFlipTx:
				if a := attachments.ParseDeleteFlipAttachment(tx); a != nil {
					result.Deletions = append(result.Deletions, Deletion{
						TxHash:      tx.Hash().String(),
						BlockHeight: h,
						Author:      author,
						Epoch:       tx.Epoch,
						CID:         cidString(a.Cid),
						CIDBytes:    fmt.Sprintf("%x", a.Cid),
					})
				}
			case types.SubmitLongAnswersTx:
				if payload := longAnswerPayload(tx.Payload); payload != "" {
					result.Answers = append(result.Answers, LongAnswerTx{
						TxHash:      tx.Hash().String(),
						BlockHeight: h,
						Author:      author,
						Epoch:       tx.Epoch,
						PayloadHex:  payload,
					})
				}
			}
		}
	}
	sort.Slice(result.Submissions, func(i, j int) bool { return result.Submissions[i].BlockHeight < result.Submissions[j].BlockHeight })
	sort.Slice(result.Deletions, func(i, j int) bool { return result.Deletions[i].BlockHeight < result.Deletions[j].BlockHeight })
	return result, nil
}

// txTypeName mirrors the names the node's RPC reports for transaction types.
func txTypeName(t uint16) string {
	switch t {
	case types.SendTx:
		return "send"
	case types.ActivationTx:
		return "activation"
	case types.InviteTx:
		return "invite"
	case types.KillTx:
		return "kill"
	case types.SubmitFlipTx:
		return "submitFlip"
	case types.SubmitAnswersHashTx:
		return "submitAnswersHash"
	case types.SubmitShortAnswersTx:
		return "submitShortAnswers"
	case types.SubmitLongAnswersTx:
		return "submitLongAnswers"
	case types.EvidenceTx:
		return "evidence"
	case types.OnlineStatusTx:
		return "online"
	case types.KillInviteeTx:
		return "killInvitee"
	case types.ChangeGodAddressTx:
		return "changeGodAddress"
	case types.BurnTx:
		return "burn"
	case types.ChangeProfileTx:
		return "changeProfile"
	case types.DeleteFlipTx:
		return "deleteFlip"
	case types.DeployContractTx:
		return "deployContract"
	case types.CallContractTx:
		return "callContract"
	case types.TerminateContractTx:
		return "terminateContract"
	case types.DelegateTx:
		return "delegate"
	case types.UndelegateTx:
		return "undelegate"
	case types.KillDelegatorTx:
		return "killDelegator"
	case types.StoreToIpfsTx:
		return "storeToIpfs"
	case types.ReplenishStakeTx:
		return "replenishStake"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}
