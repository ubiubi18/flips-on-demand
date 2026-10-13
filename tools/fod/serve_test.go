package main

// The delivery desk's fast path: the buyer-bound ciphertext has to open with
// the order's delivery key and hash to the bundle's commitment, the pass has to
// pick funded orders and stale beacons, and the prepared files the desk writes
// have to be the ones `fod submit` can sign.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/crypto"
	"github.com/idena-network/idena-go/crypto/ecies"
)

// testVaultPayload builds an epoch-keys payload for one synthetic author and
// epoch. The scalars are real curve scalars, so the commitment and the ECIES
// path are the production ones.
func testVaultPayload(t *testing.T, epoch uint16) (VaultPayload, []byte) {
	t.Helper()
	publicKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the public-part key: %v", err)
	}
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the private-part key: %v", err)
	}
	author := make([]byte, addressBytes)
	for i := range author {
		author[i] = byte(0x40 + i)
	}
	return VaultPayload{
		Kind:           "fod.epoch-keys",
		Version:        1,
		Epoch:          epoch,
		Address:        hex.EncodeToString(author),
		PublicPartSec:  hex.EncodeToString(crypto.FromECDSA(publicKey)),
		PrivatePartSec: hex.EncodeToString(crypto.FromECDSA(privateKey)),
	}, author
}

// The desk reads the deadline budget from the node's own blocks: the interval
// when the node reports one, and blocks alone when it cannot
// (`SPEC.md` 7.2, `REVIEW.md` A06/D3).
func TestSettlementBudgetUsesTheNodesOwnBlockInterval(t *testing.T) {
	cases := []struct{ name, want string }{
		{"720 blocks at 15s", "720 blocks left, about 3h00 at this node's 15s per block"},
		{"120 blocks at 30s", "120 blocks left, about 1h00 at this node's 30s per block"},
		{"90 blocks at 15s", "90 blocks left, about 22 min at this node's 15s per block"},
		{"40 blocks at 15s", "40 blocks left, about 10 min at this node's 15s per block"},
		{"3 blocks at 10s", "3 blocks left, about 30s at this node's 10s per block"},
	}
	head := uint64(1_000)
	for index, testCase := range cases {
		deadline := head + uint64([]uint64{720, 120, 90, 40, 3}[index])
		blockSeconds := int64([]int{15, 30, 15, 15, 10}[index])
		if got := settlementBudget(head, deadline, blockSeconds); got != testCase.want {
			t.Errorf("%s: %q, want %q", testCase.name, got, testCase.want)
		}
	}
	if got := settlementBudget(head, head+720, 0); !strings.Contains(got, "no block interval") {
		t.Errorf("a node without an interval reads %q", got)
	}
	if got := settlementBudget(head, head-1, 15); !strings.Contains(got, "passed") {
		t.Errorf("a passed deadline reads %q", got)
	}
}

func TestServedBundlesMatchThePoolCommitment(t *testing.T) {
	payload, author := testVaultPayload(t, 228)
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatalf("index the vault: %v", err)
	}
	publicSecret, err := hex.DecodeString(payload.PublicPartSec)
	if err != nil {
		t.Fatalf("decode the public secret: %v", err)
	}
	privateSecret, err := hex.DecodeString(payload.PrivatePartSec)
	if err != nil {
		t.Fatalf("decode the private secret: %v", err)
	}
	commitment, err := SecretsCommitmentHex(228, author, publicSecret, privateSecret)
	if err != nil {
		t.Fatalf("compute the commitment: %v", err)
	}
	bundle, ok := served[commitment]
	if !ok {
		t.Fatalf("the vault is not indexed under its own commitment %s", commitment)
	}
	if bundle.Author != hex.EncodeToString(author) || bundle.Epoch != 228 {
		t.Fatalf("the served bundle reads back as %s epoch %d", bundle.Author, bundle.Epoch)
	}

	if _, err := servedBundlesFromPayloads([]VaultPayload{{Kind: "other", Version: 1}}); err == nil {
		t.Fatal("a payload of another kind was accepted")
	}
	if _, err := servedBundlesFromPayloads([]VaultPayload{{Kind: "fod.epoch-keys", Version: 1, Address: "00"}}); err == nil {
		t.Fatal("a payload without an author address was accepted")
	}
}

func TestDeliveryCiphertextOpensToTheBundleCommitment(t *testing.T) {
	payload, _ := testVaultPayload(t, 228)
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatalf("index the vault: %v", err)
	}
	var bundle servedBundle
	for _, item := range served {
		bundle = item
	}
	buyer, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the buyer key: %v", err)
	}
	buyerKey := hex.EncodeToString(crypto.FromECDSAPub(&buyer.PublicKey))
	ciphertext, err := deliveryCiphertextFor(bundle, buyerKey)
	if err != nil {
		t.Fatalf("build the delivery ciphertext: %v", err)
	}
	plain, err := ecies.ImportECDSA(buyer).Decrypt(ciphertext, nil, nil)
	if err != nil {
		t.Fatalf("the buyer cannot open the delivery: %v", err)
	}
	digest := sha256.Sum256(plain)
	if got := hex.EncodeToString(digest[:]); got != bundle.Commitment {
		t.Fatalf("the delivered secrets hash to %s, the bundle commits to %s", got, bundle.Commitment)
	}

	unrelated, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate an unrelated key: %v", err)
	}
	if _, err := ecies.ImportECDSA(unrelated).Decrypt(ciphertext, nil, nil); err == nil {
		t.Fatal("an unrelated key opened the delivery")
	}
	if _, err := deliveryCiphertextFor(bundle, "00"); err == nil {
		t.Fatal("a malformed delivery key was accepted")
	}
}

func TestServePlanDeliversFundedOrdersAndRefreshesStaleBeacons(t *testing.T) {
	payload, author := testVaultPayload(t, 228)
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatalf("index the vault: %v", err)
	}
	var commitment string
	for key := range served {
		commitment = key
	}
	const head = uint64(10_000)
	listings := map[string]marketPoolListing{
		commitment: {
			Bundle:      commitment,
			Author:      hex.EncodeToString(author),
			BeaconBlock: head - serveDefaultRefresh,
			Units:       3,
		},
	}
	unserved := strings.Repeat("bb", 32)
	orders := []poolOrderRecord{
		{ID: 1, Bundle: commitment, State: "funded", Deadline: head + 100},
		{ID: 2, Bundle: commitment, State: "funded", Deadline: head - 1},
		{ID: 3, Bundle: commitment, State: "delivered", Deadline: head + 200},
		{ID: 4, Bundle: unserved, State: "funded", Deadline: head + 100},
		{ID: 5, Bundle: commitment, State: "settled", Deadline: head + 100},
	}
	actions := planServePass(served, listings, orders, head, serveDefaultRefresh, 300, serveDefaultReveal, true, "", nil)
	kinds := make([]string, 0, len(actions))
	for _, action := range actions {
		switch action.Kind {
		case "deliver":
			kinds = append(kinds, action.Kind+":"+itoa(action.Order.ID))
		case "fulfill":
			kinds = append(kinds, action.Kind+":"+itoa(action.Order.ID))
		case "beacon":
			kinds = append(kinds, action.Kind+":"+action.Bundle.Commitment[:8])
		}
	}
	want := []string{"deliver:1", "fulfill:3", "beacon:" + commitment[:8]}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("the plan is %v, expected %v", kinds, want)
	}

	// Without a prover the delivered order waits for its buyer; the funded order
	// and the stale beacon are still served.
	actions = planServePass(served, listings, orders, head, serveDefaultRefresh, 300, serveDefaultReveal, false, "", nil)
	if len(actions) != 2 || actions[0].Kind != "deliver" || actions[1].Kind != "beacon" {
		t.Fatalf("without a prover the plan is %v", actions)
	}

	// A fresh beacon is left alone.
	listings[commitment] = marketPoolListing{Bundle: commitment, BeaconBlock: head - 1}
	actions = planServePass(served, listings, orders, head, serveDefaultRefresh, 300, serveDefaultReveal, false, "", nil)
	if len(actions) != 1 || actions[0].Kind != "deliver" {
		t.Fatalf("with a fresh beacon the plan is %v", actions)
	}

	// A desk that signs as someone other than the bundle's author leaves the
	// beacon alone: the escrow would refuse the call.
	listings[commitment] = marketPoolListing{Bundle: commitment, BeaconBlock: 0, Author: strings.Repeat("71", 20)}
	actions = planServePass(served, listings, orders, head, serveDefaultRefresh, 300, serveDefaultReveal, false, strings.Repeat("72", 20), nil)
	if len(actions) != 1 || actions[0].Kind != "deliver" {
		t.Fatalf("a desk that is not the author planned %v", actions)
	}

	// A delivery slot that somebody else filled first is replaced: the desk
	// publishes its own ciphertext again instead of leaving the order to the
	// stranger's bytes until the refund deadline.
	listings[commitment] = marketPoolListing{Bundle: commitment, Author: hex.EncodeToString(author), BeaconBlock: head - 1}
	actions = planServePass(served, listings, orders, head, serveDefaultRefresh, 300, serveDefaultReveal, true, "", map[uint64]bool{3: true})
	replaced := false
	for _, action := range actions {
		if action.Kind == "fulfill" {
			t.Fatalf("an occupied slot planned a fulfillment: %v", action.Order.ID)
		}
		if action.Kind == "deliver" && action.Order.ID == 3 {
			replaced = true
		}
	}
	if !replaced {
		t.Fatalf("an occupied slot was not replaced: %v", actions)
	}
}

// TestServePlanDisclosesKeysOnlyWhileSettlementStillFits pins the desk's side of
// the three-block window. The committed secrets must still be broadcastable and
// includable before the refund boundary (`reveal.go` refuses once the head
// reaches `deadline-1`), so a delivery is withheld once no later pass can
// reveal: at the last disclosure head the desk settles a funded order itself,
// and past it the buyer's repayment is the only exit. A delivery a stranger
// published is replaced while there is room and settled by the reveal
// afterwards (`AH-19`).
func TestServePlanDisclosesKeysOnlyWhileSettlementStillFits(t *testing.T) {
	payload, author := testVaultPayload(t, 228)
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatalf("index the vault: %v", err)
	}
	var commitment string
	for key := range served {
		commitment = key
	}
	const head = uint64(10_000)
	listings := map[string]marketPoolListing{
		commitment: {Bundle: commitment, Author: hex.EncodeToString(author), BeaconBlock: head - 1},
	}
	// head = 10_000, serveDefaultReveal = 2: the last disclosure head is
	// deadline-2, and the reveal margin opens at deadline-head <= 2.
	orders := []poolOrderRecord{
		// funded: deliver with a later pass to spare, reveal at the last
		// disclosure head, nothing after it.
		{ID: 1, Bundle: commitment, State: "funded", Deadline: head + 3},
		{ID: 2, Bundle: commitment, State: "funded", Deadline: head + 2},
		{ID: 3, Bundle: commitment, State: "funded", Deadline: head + 1},
		// delivered: wait while time is left, reveal inside the margin and at
		// the last disclosure head, nothing past it.
		{ID: 4, Bundle: commitment, State: "delivered", Deadline: head + 4},
		{ID: 5, Bundle: commitment, State: "delivered", Deadline: head + 2},
		{ID: 6, Bundle: commitment, State: "delivered", Deadline: head + 1},
		// a stranger's delivery: replaced while it fits, settled by the reveal
		// at the last disclosure head, left to the refund past it.
		{ID: 7, Bundle: commitment, State: "delivered", Deadline: head + 3},
		{ID: 8, Bundle: commitment, State: "delivered", Deadline: head + 2},
		{ID: 9, Bundle: commitment, State: "delivered", Deadline: head + 1},
	}
	actions := planServePass(served, listings, orders, head, serveDefaultRefresh, 300, serveDefaultReveal, false, "",
		map[uint64]bool{7: true, 8: true, 9: true})
	kinds := map[string]bool{}
	for _, action := range actions {
		kinds[action.Kind+":"+itoa(action.Order.ID)] = true
	}
	want := []string{"deliver:1", "reveal:2", "reveal:5", "deliver:7", "reveal:8"}
	if len(actions) != len(want) {
		t.Fatalf("the plan is %v, expected %v", kinds, want)
	}
	for _, kind := range want {
		if !kinds[kind] {
			t.Fatalf("the plan is %v, expected %v", kinds, want)
		}
	}
	for _, id := range []uint64{3, 4, 6, 9} {
		for kind := range kinds {
			if strings.HasSuffix(kind, ":"+itoa(id)) {
				t.Fatalf("order %d was acted on past the last usable disclosure head: %v", id, actions)
			}
		}
	}
}

// TestServeWritesTheRevealActionOwnerOnly pins the file rule for the one desk
// action that carries key material: the prepared reveal's calldata is the
// committed secrets, so a rewrite has to tighten a wider file (`AH-19`,
// `AH-20`).
func TestServeWritesTheRevealActionOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	desk := &serveDesk{contract: strings.Repeat("11", 20), network: "idena-local", outDir: dir}
	prepared := buildRevealAction(desk.contract, desk.network, 7, strings.Repeat("aa", 32), []byte("the committed secrets"))
	path := filepath.Join(dir, "reveal-7.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("pre-create the action: %v", err)
	}
	if err := desk.submit(prepared, "reveal-7"); err != nil {
		t.Fatalf("write the reveal action: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the action: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the reveal action is mode %o, want 0600", info.Mode().Perm())
	}
}

func TestDeliveryRecordRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok := readDeliveryRecord(dir, 7); ok {
		t.Fatal("an empty directory reported a record")
	}
	if err := writeDeliveryRecord(dir, 7, "abc"); err != nil {
		t.Fatalf("write the record: %v", err)
	}
	if got, ok := readDeliveryRecord(dir, 7); !ok || got != "abc" {
		t.Fatalf("the record is %q, %v", got, ok)
	}
	if err := writeDeliveryRecord(dir, 8, "def"); err != nil {
		t.Fatalf("write the second record: %v", err)
	}
	if got, _ := readDeliveryRecord(dir, 7); got != "abc" {
		t.Fatalf("order 7 reads %q once order 8 has a record", got)
	}
	// A corrupt record is treated as no record, so the desk republishes rather
	// than trusting a digest it cannot read.
	if err := os.WriteFile(deliveryRecordPath(dir, 9), []byte("{"), 0o600); err != nil {
		t.Fatalf("write the corrupt record: %v", err)
	}
	if _, ok := readDeliveryRecord(dir, 9); ok {
		t.Fatal("a corrupt record was trusted")
	}
}

func TestDeskMaxFeeLeavesHeadroom(t *testing.T) {
	rate := big.NewInt(10_000_000_000_000_000)
	for _, operation := range []string{operationDeliver, operationBeacon, operationDonateUnit, operationBuy} {
		gas, ok := measuredGasFor(operation)
		if !ok {
			t.Fatalf("%s has no measured gas", operation)
		}
		floor := requiredMaxFeeFor(gas.WasmGas, gas.TxBytes, rate)
		got := deskMaxFee(gas, rate)
		if got.Cmp(floor) <= 0 {
			t.Fatalf("%s: the desk fee %s is not above the measured floor %s", operation, got, floor)
		}
		if got.Cmp(maxMaxFeeFor(rate)) > 0 {
			t.Fatalf("%s: the desk fee %s is above the node's cap", operation, got)
		}
	}
}

func TestServePreparedActionsAreSignable(t *testing.T) {
	contract := strings.Repeat("aa", 20)
	cases := []struct {
		operation string
		format    string
		calldata  []byte
	}{
		{operationDeliver, preparedDeliverFormat, poolDeliverCalldata(7, make([]byte, 219))},
		{operationBeacon, preparedBeaconFormat, make([]byte, 32)},
		{operationFulfill, preparedFulfillFormat, make([]byte, 8+281+256+219)},
	}
	for _, c := range cases {
		prepared := servePreparedAction(c.operation, c.format, contract, "testnet", c.calldata, 7, strings.Repeat("cc", 32), strings.Repeat("dd", 20))
		path := t.TempDir() + "/action.json"
		if err := writeJSON(path, prepared); err != nil {
			t.Fatalf("%s: write: %v", c.operation, err)
		}
		loaded, err := loadPreparedAction(path)
		if err != nil {
			t.Fatalf("%s: load: %v", c.operation, err)
		}
		if loaded.Format != c.format || loaded.Operation != c.operation {
			t.Fatalf("the loaded action is %s / %s", loaded.Format, loaded.Operation)
		}
	}
}

// The desk signs its own calls, so each desk operation needs a measured size
// for the first fee estimate. The signed size is checked again after signing
// (`maxFeeShortfall`), which is what covers the ±1-byte integer encodings a
// different nonce or fee can add.
func TestServeCanonicalSignedSizes(t *testing.T) {
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the signing key: %v", err)
	}
	contract := strings.Repeat("aa", 20)
	cases := []struct {
		operation string
		format    string
		calldata  []byte
		amount    string
	}{
		{operationDeliver, preparedDeliverFormat, poolDeliverCalldata(7, make([]byte, 219)), "0"},
		{operationBeacon, preparedBeaconFormat, make([]byte, 32), "0"},
		{operationFulfill, preparedFulfillFormat, make([]byte, 8+281+256+219), "0"},
	}
	for _, c := range cases {
		prepared := servePreparedAction(c.operation, c.format, contract, "testnet", c.calldata, 7, strings.Repeat("cc", 32), strings.Repeat("dd", 20))
		prepared.Amount = c.amount
		raw, _, err := BuildTransaction(prepared, identity, 1, 228, big.NewInt(1_000_000_000_000_000), big.NewInt(0))
		if err != nil {
			t.Fatalf("%s: sign: %v", c.operation, err)
		}
		gas, ok := measuredGasFor(c.operation)
		if !ok {
			t.Fatalf("%s has no measured gas", c.operation)
		}
		if gas.TxBytes != uint64(len(raw)) {
			t.Fatalf("%s signs %d bytes, the fee table records %d", c.operation, len(raw), gas.TxBytes)
		}
	}
}

// TestServeOnceWritesTheDeliverAndBeaconActions runs the real runner against a
// node-shaped JSON-RPC endpoint. The records are the contract's own encodings;
// the node itself is a test double, so this proves the read, plan, ciphertext,
// and prepared-file path, not a live chain.
func TestServeOnceWritesTheDeliverAndBeaconActions(t *testing.T) {
	payload, author := testVaultPayload(t, 228)
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatalf("index the vault: %v", err)
	}
	var commitment string
	for key := range served {
		commitment = key
	}
	commitmentBytes, err := hex.DecodeString(commitment)
	if err != nil {
		t.Fatalf("decode the commitment: %v", err)
	}

	buyer, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the buyer key: %v", err)
	}
	buyerAddress := crypto.PubkeyToAddress(buyer.PublicKey)

	// The bundle's own record: author (20), manifest (32), inventory (32),
	// units (8), beacon (8), strikes (4), nonce (8), covered (4). The one
	// pool's reserve and unit count live in the counters the fake node answers.
	// A beacon at 800 is stale at head 1000.
	poolRecord := make([]byte, marketPoolValueBytes)
	copy(poolRecord[marketPoolAuthorOffset:marketPoolManifestOffset], author)
	binary.BigEndian.PutUint64(poolRecord[marketPoolUnitsOffset:marketPoolBeaconOffset], 3)
	binary.BigEndian.PutUint64(poolRecord[marketPoolBeaconOffset:marketPoolStrikesOffset], 800)

	// The order record: bundle (32), buyer (20), delivery key (65), paid (16),
	// funded (8), nonce (8), state (1).
	orderRecord := make([]byte, marketPoolOrderBytes)
	copy(orderRecord[0:32], commitmentBytes)
	copy(orderRecord[32:52], buyerAddress[:])
	copy(orderRecord[52:117], crypto.FromECDSAPub(&buyer.PublicKey))
	binary.BigEndian.PutUint64(orderRecord[117+8:117+16], 25_000)
	binary.BigEndian.PutUint64(orderRecord[133:141], 1000)
	binary.BigEndian.PutUint64(orderRecord[141:149], 7)
	orderRecord[149] = 1

	contract := strings.Repeat("aa", 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result := any(nil)
		switch request.Method {
		case "bcn_lastBlock":
			result = map[string]any{"height": 1000}
		case "contract_iterateMap":
			prefix, _ := request.Params[1].(string)
			items := []map[string]string{}
			switch prefix {
			case marketPoolPrefix:
				items = append(items, map[string]string{
					"key":   commitment,
					"value": hex.EncodeToString(poolRecord),
				})
			case marketPoolOrderPrefix:
				items = append(items, map[string]string{
					"key":   hex.EncodeToString([]byte("1")),
					"value": hex.EncodeToString(orderRecord),
				})
			}
			result = map[string]any{"items": items, "continuationToken": "0x"}
		case "contract_readData":
			key, _ := request.Params[1].(string)
			value := "0x00"
			switch key {
			case marketReserveKey:
				value = "0x" + hex.EncodeToString(new(big.Int).SetUint64(50_000).Bytes())
			case marketPoolUnitsKey:
				value = "0x" + hex.EncodeToString(new(big.Int).SetUint64(3).Bytes())
			}
			result = value
		default:
			http.Error(w, "unexpected method "+request.Method, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	}))
	defer server.Close()

	t.Setenv(PassphraseEnvVar, "test-pass")
	vaultPath := filepath.Join(t.TempDir(), "epoch.age")
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode the vault payload: %v", err)
	}
	if err := WriteVault(vaultPath, "test-pass", payloadJSON); err != nil {
		t.Fatalf("write the vault: %v", err)
	}
	outDir := t.TempDir()
	err = runServe(serveOptions{
		RPCURL:       server.URL,
		Contract:     contract,
		Network:      "testnet",
		VaultFlag:    vaultPath,
		OutDir:       outDir,
		Once:         true,
		IntervalSec:  1,
		Refresh:      serveDefaultRefresh,
		ProofMargin:  serveDefaultProof,
		RevealMargin: serveDefaultReveal,
		Tips:         big.NewInt(0),
	})
	if err != nil {
		t.Fatalf("serve pass: %v", err)
	}

	deliverRaw, err := os.ReadFile(filepath.Join(outDir, "deliver-1.json"))
	if err != nil {
		t.Fatalf("the deliver action was not written: %v", err)
	}
	var deliver preparedAction
	if err := json.Unmarshal(deliverRaw, &deliver); err != nil {
		t.Fatalf("parse the deliver action: %v", err)
	}
	if deliver.Operation != operationDeliver {
		t.Fatalf("the deliver action names %s", deliver.Operation)
	}
	deliverCalldata, err := hex.DecodeString(deliver.Calldata)
	if err != nil {
		t.Fatalf("decode the deliver calldata: %v", err)
	}
	if binary.BigEndian.Uint64(deliverCalldata[:8]) != 1 {
		t.Fatalf("the deliver calldata names order %d", binary.BigEndian.Uint64(deliverCalldata[:8]))
	}
	plain, err := ecies.ImportECDSA(buyer).Decrypt(deliverCalldata[8:], nil, nil)
	if err != nil {
		t.Fatalf("the buyer cannot open the prepared delivery: %v", err)
	}
	digest := sha256.Sum256(plain)
	if hex.EncodeToString(digest[:]) != commitment {
		t.Fatal("the prepared delivery does not open to the bundle commitment")
	}

	beaconRaw, err := os.ReadFile(filepath.Join(outDir, "beacon-"+commitment[:8]+".json"))
	if err != nil {
		t.Fatalf("the beacon action was not written: %v", err)
	}
	var beaconAction preparedAction
	if err := json.Unmarshal(beaconRaw, &beaconAction); err != nil {
		t.Fatalf("parse the beacon action: %v", err)
	}
	if beaconAction.Operation != operationBeacon || !strings.EqualFold(beaconAction.Calldata, commitment) {
		t.Fatalf("the beacon action is %s on %s", beaconAction.Operation, beaconAction.Calldata)
	}
}

// TestServeBroadcastSignsTheDeskCalls closes the loop the prepare-only test
// leaves open: the same pass reads a funded order and a stale beacon, signs
// both calls with the desk's own key, and hands them to the node. The node is
// still a test double, so this proves the signing and submission path, not a
// live chain.
func TestServeBroadcastSignsTheDeskCalls(t *testing.T) {
	payload, author := testVaultPayload(t, 228)
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatalf("index the vault: %v", err)
	}
	var commitment string
	for key := range served {
		commitment = key
	}
	commitmentBytes, err := hex.DecodeString(commitment)
	if err != nil {
		t.Fatalf("decode the commitment: %v", err)
	}
	buyer, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the buyer key: %v", err)
	}
	buyerAddress := crypto.PubkeyToAddress(buyer.PublicKey)
	poolRecord := make([]byte, marketPoolValueBytes)
	copy(poolRecord[marketPoolAuthorOffset:marketPoolManifestOffset], author)
	binary.BigEndian.PutUint64(poolRecord[marketPoolUnitsOffset:marketPoolBeaconOffset], 3)
	binary.BigEndian.PutUint64(poolRecord[marketPoolBeaconOffset:marketPoolStrikesOffset], 800)
	orderRecord := make([]byte, marketPoolOrderBytes)
	copy(orderRecord[0:32], commitmentBytes)
	copy(orderRecord[32:52], buyerAddress[:])
	copy(orderRecord[52:117], crypto.FromECDSAPub(&buyer.PublicKey))
	binary.BigEndian.PutUint64(orderRecord[125:133], 25_000)
	binary.BigEndian.PutUint64(orderRecord[133:141], 1000)
	binary.BigEndian.PutUint64(orderRecord[141:149], 7)
	orderRecord[149] = 1

	contract := strings.Repeat("aa", 20)
	var mu sync.Mutex
	nonce := uint32(0)
	var sent [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result := any(nil)
		switch request.Method {
		case "bcn_lastBlock":
			result = map[string]any{"height": 1000}
		case "contract_iterateMap":
			prefix, _ := request.Params[1].(string)
			items := []map[string]string{}
			switch prefix {
			case marketPoolPrefix:
				items = append(items, map[string]string{"key": commitment, "value": hex.EncodeToString(poolRecord)})
			case marketPoolOrderPrefix:
				items = append(items, map[string]string{"key": hex.EncodeToString([]byte("1")), "value": hex.EncodeToString(orderRecord)})
			}
			result = map[string]any{"items": items, "continuationToken": "0x"}
		case "contract_readData":
			key, _ := request.Params[1].(string)
			value := "0x00"
			switch key {
			case "porder/1":
				value = "0x" + hex.EncodeToString(orderRecord)
			case marketReserveKey:
				value = "0x" + hex.EncodeToString(new(big.Int).SetUint64(50_000).Bytes())
			case marketPoolUnitsKey:
				value = "0x" + hex.EncodeToString(new(big.Int).SetUint64(3).Bytes())
			}
			result = value
		case "dna_getBalance":
			mu.Lock()
			current := nonce
			mu.Unlock()
			result = map[string]any{"nonce": current}
		case "dna_epoch":
			result = map[string]any{"epoch": 228}
		case "bcn_sendRawTx":
			rawHex, _ := request.Params[0].(string)
			raw, err := hex.DecodeString(strings.TrimPrefix(rawHex, "0x"))
			if err != nil {
				http.Error(w, "bad raw tx", http.StatusBadRequest)
				return
			}
			mu.Lock()
			sent = append(sent, raw)
			nonce++
			mu.Unlock()
			result = "0x" + strings.Repeat("11", 32)
		default:
			http.Error(w, "unexpected method "+request.Method, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	}))
	defer server.Close()

	t.Setenv(PassphraseEnvVar, "test-pass")
	oldPoll := serveNoncePoll
	serveNoncePoll = time.Millisecond
	t.Cleanup(func() { serveNoncePoll = oldPoll })
	vaultPath := filepath.Join(t.TempDir(), "epoch.age")
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode the vault payload: %v", err)
	}
	if err := WriteVault(vaultPath, "test-pass", payloadJSON); err != nil {
		t.Fatalf("write the vault: %v", err)
	}
	identity, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate the desk key: %v", err)
	}
	nodeKeyPath := filepath.Join(t.TempDir(), "nodekey")
	if err := crypto.SaveECDSA(nodeKeyPath, identity); err != nil {
		t.Fatalf("write the nodekey: %v", err)
	}
	// The escrow accepts a beacon only from the bundle's registered author, so
	// this desk is the author of the bundle it serves.
	deskAddress := crypto.PubkeyToAddress(identity.PublicKey)
	copy(poolRecord[marketPoolAuthorOffset:marketPoolManifestOffset], deskAddress[:])
	err = runServe(serveOptions{
		RPCURL:       server.URL,
		Contract:     contract,
		Network:      "testnet",
		VaultFlag:    vaultPath,
		NodeKey:      nodeKeyPath,
		OutDir:       t.TempDir(),
		Broadcast:    true,
		Once:         true,
		IntervalSec:  1,
		Refresh:      serveDefaultRefresh,
		ProofMargin:  serveDefaultProof,
		RevealMargin: serveDefaultReveal,
		Tips:         big.NewInt(0),
		FeePerGas:    big.NewInt(1000),
	})
	if err != nil {
		t.Fatalf("serve broadcast pass: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 2 {
		t.Fatalf("the desk sent %d transactions, expected 2", len(sent))
	}
	var deliverTx types.Transaction
	if err := deliverTx.FromBytes(sent[0]); err != nil {
		t.Fatalf("decode the deliver transaction: %v", err)
	}
	deliverCall := attachments.ParseCallContractAttachment(&deliverTx)
	if deliverCall == nil || deliverCall.Method != operationDeliver || len(deliverCall.Args) != 1 {
		t.Fatalf("the first transaction is not a deliver call: %+v", deliverCall)
	}
	if binary.BigEndian.Uint64(deliverCall.Args[0][:8]) != 1 {
		t.Fatalf("the deliver call names order %d", binary.BigEndian.Uint64(deliverCall.Args[0][:8]))
	}
	plain, err := ecies.ImportECDSA(buyer).Decrypt(deliverCall.Args[0][8:], nil, nil)
	if err != nil {
		t.Fatalf("the broadcast delivery does not open for the buyer: %v", err)
	}
	digest := sha256.Sum256(plain)
	if hex.EncodeToString(digest[:]) != commitment {
		t.Fatal("the broadcast delivery does not open to the bundle commitment")
	}
	var beaconTx types.Transaction
	if err := beaconTx.FromBytes(sent[1]); err != nil {
		t.Fatalf("decode the beacon transaction: %v", err)
	}
	beaconCall := attachments.ParseCallContractAttachment(&beaconTx)
	if beaconCall == nil || beaconCall.Method != operationBeacon || len(beaconCall.Args) != 1 {
		t.Fatalf("the second transaction is not a beacon call: %+v", beaconCall)
	}
	if !strings.EqualFold(hex.EncodeToString(beaconCall.Args[0]), commitment) {
		t.Fatalf("the beacon call names %x", beaconCall.Args[0])
	}
}

func TestPoolDeliveryOrderInputNamesTheBundleAuthor(t *testing.T) {
	payload, _ := testVaultPayload(t, 228)
	served, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatalf("index the vault: %v", err)
	}
	var bundle servedBundle
	for _, item := range served {
		bundle = item
	}
	order := &poolOrderRecord{
		ID:          7,
		Nonce:       9,
		Buyer:       strings.Repeat("dd", 20),
		DeliveryKey: strings.Repeat("ee", 65),
	}
	input, err := poolDeliveryOrderInput(strings.Repeat("aa", 20), bundle, order)
	if err != nil {
		t.Fatalf("build the prover order: %v", err)
	}
	if input.OfferID != 7 || input.OrderNonce != 9 {
		t.Fatalf("the prover order names %d / %d", input.OfferID, input.OrderNonce)
	}
	if input.Seller != bundle.Author || input.Author != bundle.Author {
		t.Fatalf("the prover order names seller %s and author %s", input.Seller, input.Author)
	}
	if input.Buyer != order.Buyer || input.BuyerKey != order.DeliveryKey || input.Epoch != 228 {
		t.Fatalf("the prover order names buyer %s key %s epoch %d", input.Buyer, input.BuyerKey, input.Epoch)
	}
	if len(input.PublicPartSecret) != 64 || len(input.PrivatePartSecret) != 64 || len(input.Ephemeral) != 64 {
		t.Fatalf("the prover order carries secrets %d/%d and an ephemeral %d hex chars",
			len(input.PublicPartSecret), len(input.PrivatePartSecret), len(input.Ephemeral))
	}
}

// itoa is a tiny id renderer so the test does not need a strconv import for one
// call site.
func itoa(value uint64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
