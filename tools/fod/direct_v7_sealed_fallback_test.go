package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectV7SealedFallbackPreparesAndRechecksNativeProof(t *testing.T) {
	var fixture struct {
		Author   string `json:"author"`
		Sessions []struct {
			Inventory SessionAnchor `json:"inventory"`
			Verdict   SessionAnchor `json:"verdict"`
		} `json:"sessions"`
	}
	sessionRaw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-session.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sessionRaw, &fixture); err != nil || len(fixture.Sessions) != 1 {
		t.Fatal("invalid synthetic session")
	}
	session, err := sessionBundleFromAnchors(fixture.Author, &fixture.Sessions[0].Inventory, &fixture.Sessions[0].Verdict)
	if err != nil {
		t.Fatal(err)
	}
	proofRaw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-v5-one-flip.json")
	if err != nil {
		t.Fatal(err)
	}
	var proof fastContentArtefact
	if err := json.Unmarshal(proofRaw, &proof); err != nil {
		t.Fatal(err)
	}
	journal, err := hex.DecodeString(proof.Journal)
	if err != nil || len(journal) != fastContentJournalBytes {
		t.Fatal("invalid sealed fixture")
	}
	contract := hex.EncodeToString(journal[36:56])
	var offer poolV7Offer
	copy(offer.author[:], journal[56:76])
	offer.epoch = binary.BigEndian.Uint16(journal[76:78])
	offer.covered = uint32(binary.BigEndian.Uint16(journal[78:80]))
	copy(offer.inventory[:], journal[144:176])
	copy(offer.keys[:], journal[241:273])
	offer.status, offer.openOrder, offer.nextNonce = 3, 1, 2
	id := poolV7OfferID(offer.author, offer.epoch)
	offerRaw := make([]byte, 222)
	copy(offerRaw[:20], offer.author[:])
	binary.BigEndian.PutUint16(offerRaw[20:22], offer.epoch)
	binary.BigEndian.PutUint32(offerRaw[22:26], offer.covered)
	copy(offerRaw[26:58], offer.inventory[:])
	copy(offerRaw[58:90], offer.keys[:])
	offerRaw[105], offerRaw[122] = 1, 3 // Seller bond and active state.
	binary.BigEndian.PutUint64(offerRaw[106:114], offer.nextNonce)
	binary.BigEndian.PutUint64(offerRaw[114:122], offer.openOrder)
	orderRaw := make([]byte, 215)
	copy(orderRaw[:32], id[:])
	orderRaw[132], orderRaw[148], orderRaw[164] = 1, 1, 1 // Paid, buyer bond, seller payout.
	binary.BigEndian.PutUint64(orderRaw[165:173], 1_000)
	binary.BigEndian.PutUint64(orderRaw[173:181], 1)
	orderRaw[181] = 1 // Funded.
	head := uint64(1_030)
	var simulated string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "unexpected RPC", http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "bcn_blockAt":
			height := uint64(request.Params[0].(float64))
			var root string
			switch height {
			case session.Inventory.Height:
				root = session.Inventory.RootHex
			case session.Verdict.Height:
				root = session.Verdict.RootHex
			default:
				http.Error(w, "unexpected header", http.StatusBadRequest)
				return
			}
			result = map[string]any{"height": height, "root": "0x" + root}
		case "bcn_lastBlock":
			result = map[string]any{"height": head}
		case "dna_getCoinbaseAddr":
			result = "0x" + session.Author
		case "contract_getStake":
			result = map[string]any{"Hash": "0x" + strings.Repeat("cd", 32)}
		case "contract_iterateMap":
			result = map[string]any{"items": []any{map[string]any{"key": hex.EncodeToString(id[:]), "value": hex.EncodeToString(offerRaw)}}, "continuationToken": nil}
		case "contract_readData":
			if request.Params[1] != "pool/order/1" {
				http.Error(w, "unexpected state key", http.StatusBadRequest)
				return
			}
			result = "0x" + hex.EncodeToString(orderRaw)
		case "bcn_feePerGas":
			result = "1"
		case "contract_estimateCall":
			params := request.Params[0].(map[string]any)
			if params["method"] != "startFallback" || params["amount"] != "0" || params["from"] != "0x"+session.Author {
				http.Error(w, "wrong simulation", http.StatusBadRequest)
				return
			}
			args := params["args"].([]any)[0].(map[string]any)
			simulated = args["value"].(string)
			result = map[string]any{"success": true, "gasUsed": 1_000}
		default:
			http.Error(w, "unexpected RPC", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result})
	}))
	defer server.Close()
	c := &client{url: server.URL, http: newHTTPClient()}
	dir := t.TempDir()
	sessionPath, proofPath, hostPath := filepath.Join(dir, "session.json"), filepath.Join(dir, "proof.json"), filepath.Join(dir, "host")
	if err := writePrivateJSON(sessionPath, session); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proofPath, proofRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	// This stub checks host invocation; the WASM harness verifies the fixture's seal.
	host := "#!/bin/sh\nif [ \"$#\" -eq 1 ] && [ \"$1\" = --image-id ]; then\n" +
		"  printf '%s\\n' " + fastContentV5ImageIDHex + "\n" +
		"elif [ \"$#\" -eq 4 ] && [ \"$1\" = --verify ] && [ \"$3\" = --contract ] && [ \"$4\" = 0x" + contract + " ]; then\n" +
		"  exit 0\nelse\n  exit 1\nfi\n"
	if err := os.WriteFile(hostPath, []byte(host), 0o700); err != nil {
		t.Fatal(err)
	}
	p := &preparedAction{
		Contract: contract, Author: session.Author, Epoch: offer.epoch, CoveredFlips: offer.covered,
		InventoryCommitment: hex.EncodeToString(offer.inventory[:]), KeyCommitment: hex.EncodeToString(offer.keys[:]),
		DirectOrderID: 1,
	}
	o := directCommandOptions{Node: c, OrderID: 1, SessionPath: sessionPath, ProofPath: proofPath, ProofHostPath: hostPath}
	calldata, err := preparePoolV7SealedFallback(o, contract, &offer, p)
	if err != nil {
		t.Fatalf("native v7 sealed fallback refused: %v", err)
	}
	seal, _ := hex.DecodeString(proof.Seal)
	wire, _ := encodeFastRegistrationWire(session)
	want := binary.BigEndian.AppendUint64(nil, 1)
	want = append(want, journal...)
	want = append(want, seal...)
	want = binary.BigEndian.AppendUint16(want, uint16(len(wire)))
	want = append(want, wire...)
	if !bytes.Equal(calldata, want) || p.DirectSession == nil || p.ImageID != fastContentV5ImageIDHex || p.ClaimDigest != proof.ClaimDigest {
		t.Fatal("v7 fallback lost the exact sealed receipt or canonical session")
	}
	if err := validatePoolV7SealedFallbackStatement(c, p, &offer, calldata); err != nil {
		t.Fatalf("native v7 fallback failed signing recheck: %v", err)
	}
	preparedPath := filepath.Join(dir, "prepared.json")
	full := o
	full.DirectVersion, full.Operation, full.CodeHash = 7, "startFallback", strings.Repeat("cd", 32)
	full.PreparedPath, full.MaxFeeText = preparedPath, "1000000"
	if err := preparePoolV7FromFiles(full, contract); err != nil {
		t.Fatalf("typed v7 fallback preparation refused: %v", err)
	}
	var prepared preparedAction
	preparedRaw, err := os.ReadFile(preparedPath)
	if err != nil || decodeStrict(preparedRaw, &prepared) != nil ||
		prepared.Calldata != hex.EncodeToString(calldata) || simulated != "0x"+prepared.Calldata {
		t.Fatalf("typed v7 fallback did not simulate the exact sealed call: %v", err)
	}
	if err := validatePreparedPoolV7Caller(c, &prepared, session.Author); err != nil {
		t.Fatalf("typed v7 fallback failed signing recheck: %v", err)
	}
	orderRaw[181] = 4 // Another start landed before signing.
	if err := validatePreparedPoolV7Caller(c, &prepared, session.Author); err == nil {
		t.Fatal("signed a fallback after the funded order changed state")
	}
	orderRaw[181] = 1
	offerRaw[204] = 1 // Another content proof landed before signing.
	if err := validatePreparedPoolV7Caller(c, &prepared, session.Author); err == nil {
		t.Fatal("signed a sealed fallback after the offer became proved")
	}
	offerRaw[204] = 0
	proofCall, err := encodePoolV7ProveBundle(id, journal, seal, wire)
	if err != nil {
		t.Fatal(err)
	}
	preparedProof := *p
	preparedProof.Format, preparedProof.Operation = preparedPoolV7Prove, "proveBundle"
	preparedProof.Network, preparedProof.DirectVersion = NetworkNamePinned, 7
	preparedProof.CodeHash, preparedProof.Amount = strings.Repeat("cd", 32), "0"
	preparedProof.DirectOfferID, preparedProof.Calldata = hex.EncodeToString(id[:]), hex.EncodeToString(proofCall)
	checkProof := func(allowed bool, label string) {
		t.Helper()
		err := validatePreparedPoolV7Caller(c, &preparedProof, session.Author)
		if allowed && err != nil || !allowed && err == nil {
			t.Fatalf("%s: allowed=%t, validation error=%v", label, allowed, err)
		}
	}
	checkProof(true, "active funded proof")
	orderRaw[181] = 4 // Fallback pending still permits independent proof.
	checkProof(true, "active fallback proof")
	orderRaw[181] = 2 // Settled proof requires a sold offer.
	checkProof(false, "active settled order")
	offerRaw[122], offerRaw[123] = 4, 1
	checkProof(true, "privately settled proof")
	orderRaw[181] = 1
	checkProof(false, "sold funded order")
	orderRaw[181] = 2
	head = 1_000 + poolV7ProofWindow
	checkProof(false, "proof deadline")
	head = 1_030
	orderRaw[0] ^= 1
	checkProof(false, "unrelated funded order")
	orderRaw[0] ^= 1
	offerRaw[122], offerRaw[123], orderRaw[181] = 3, 0, 1
	wrongContract := strings.Repeat("aa", 20)
	if _, err := preparePoolV7SealedFallback(o, wrongContract, &offer, p); err == nil {
		t.Fatal("accepted a receipt bound to another contract")
	}
	legacyJournal := append([]byte(nil), journal...)
	legacyAddress, _ := hex.DecodeString(poolV7LegacyContentContract)
	copy(legacyJournal[36:56], legacyAddress)
	legacyClaim, err := claimDigestFromHex(fastContentV5ImageIDHex, legacyJournal)
	if err != nil {
		t.Fatal(err)
	}
	legacyProof := proof
	legacyProof.Journal, legacyProof.ClaimDigest = hex.EncodeToString(legacyJournal), hex.EncodeToString(legacyClaim[:])
	legacyPath := filepath.Join(dir, "legacy-proof.json")
	if err := writePrivateJSON(legacyPath, &legacyProof); err != nil {
		t.Fatal(err)
	}
	legacyOptions := o
	legacyOptions.ProofPath = legacyPath
	if _, err := preparePoolV7SealedFallback(legacyOptions, contract, &offer, p); err == nil {
		t.Fatal("prepared a legacy-bound receipt for a funded v7 fallback")
	}
	changed := *p
	changed.ClaimDigest = strings.Repeat("00", 32)
	if err := validatePoolV7SealedFallbackStatement(c, &changed, &offer, calldata); err == nil {
		t.Fatal("accepted a changed proof claim")
	}
	changed = *p
	changed.Contract = poolV7LegacyContentContract
	if err := validatePoolV7SealedFallbackStatement(c, &changed, &offer, calldata); err == nil {
		t.Fatal("accepted a legacy contract binding after funding")
	}
	changedWire := append([]byte(nil), calldata...)
	changedWire[len(changedWire)-1] ^= 1
	if err := validatePoolV7SealedFallbackStatement(c, p, &offer, changedWire); err == nil {
		t.Fatal("accepted changed canonical session wire")
	}
	changedOrder := append([]byte(nil), calldata...)
	changedOrder[7] ^= 1
	if err := validatePoolV7SealedFallbackStatement(c, p, &offer, changedOrder); err == nil {
		t.Fatal("accepted another order ID")
	}
}
