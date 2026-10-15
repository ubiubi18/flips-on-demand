package main

import (
	"bytes"
	"crypto/sha256"
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

func TestDirectV6PoolPreparationBindsTwoAuthorIdentitiesAndSimulation(t *testing.T) {
	var fixture struct {
		Author   string `json:"author"`
		Sessions []struct {
			Inventory SessionAnchor `json:"inventory"`
			Verdict   SessionAnchor `json:"verdict"`
		} `json:"sessions"`
	}
	raw, err := os.ReadFile("../../proofs/fast-content/dist/synthetic-session.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil || len(fixture.Sessions) != 1 {
		t.Fatal("invalid canonical synthetic session fixture")
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
	if err != nil || len(journal) != 273 {
		t.Fatal("invalid sealed fixture journal")
	}
	contract := hex.EncodeToString(journal[36:56])
	otherAuthor := strings.Repeat("ab", 20)
	activeAuthor := session.Author
	estimateSuccess := true
	var simulated []string
	var rpcCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rpcCalls++
		var result any
		switch request.Method {
		case "contract_getStake":
			result = map[string]any{"Hash": "0x" + directV6CodeHashHex}
		case "dna_getCoinbaseAddr":
			result = "0x" + activeAuthor
		case "bcn_blockAt":
			height := uint64(request.Params[0].(float64))
			root := ""
			switch height {
			case session.Inventory.Height:
				root = session.Inventory.RootHex
			case session.Verdict.Height:
				root = session.Verdict.RootHex
			default:
				http.Error(w, "unexpected session height", http.StatusBadRequest)
				return
			}
			result = map[string]any{"height": height, "root": "0x" + root}
		case "bcn_feePerGas":
			result = "1000"
		case "contract_estimateCall":
			params := request.Params[0].(map[string]any)
			if params["from"] != "0x"+session.Author || params["contract"] != "0x"+contract || params["amount"] != "0" || params["maxFee"] != "0.00000000002" {
				http.Error(w, "wrong simulation identity, contract, or amount", http.StatusBadRequest)
				return
			}
			args := params["args"].([]any)[0].(map[string]any)
			if args["index"] != float64(0) || args["format"] != "hex" {
				http.Error(w, "wrong calldata attachment", http.StatusBadRequest)
				return
			}
			simulated = append(simulated, params["method"].(string)+":"+args["value"].(string))
			result = map[string]any{"success": estimateSuccess, "gasUsed": 10_000, "error": "refused"}
		default:
			http.Error(w, "unexpected RPC method", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "error": nil})
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
	host := "#!/bin/sh\nif [ \"$#\" -eq 1 ] && [ \"$1\" = --image-id ]; then\n" +
		"  printf '%s\\n' " + fastContentV5ImageIDHex + "\n" +
		"elif [ \"$#\" -eq 4 ] && [ \"$1\" = --verify ] && [ \"$3\" = --contract ]; then\n" +
		"  exit 0\nelse\n  exit 1\nfi\n"
	if err := os.WriteFile(hostPath, []byte(host), 0o700); err != nil {
		t.Fatal(err)
	}
	options := directCommandOptions{
		Mode: "direct-pool-qualify", DirectVersion: 6, Node: c, Network: NetworkNamePinned,
		Contract: contract, CodeHash: directV6CodeHashHex, SessionPath: sessionPath,
		ProofPath: proofPath, ProofHostPath: hostPath, PreparedPath: filepath.Join(dir, "qualify.json"),
		MaxFeeText: "20000000",
	}
	if err := directCommand(options); err != nil {
		t.Fatalf("canonical v6 qualification refused: %v", err)
	}
	var qualify preparedAction
	preparedRaw, err := os.ReadFile(options.PreparedPath)
	if err != nil || decodeStrict(preparedRaw, &qualify) != nil {
		t.Fatalf("read v6 qualification: %v", err)
	}
	wire, err := encodeFastRegistrationWire(session)
	if err != nil {
		t.Fatal(err)
	}
	expectedQualify := make([]byte, 72+len(wire))
	binary.BigEndian.PutUint16(expectedQualify[:2], session.Epoch)
	binary.BigEndian.PutUint32(expectedQualify[2:6], uint32(len(session.Manifest)))
	copy(expectedQualify[6:38], journal[144:176])
	copy(expectedQualify[38:70], journal[241:273])
	binary.BigEndian.PutUint16(expectedQualify[70:72], uint16(len(wire)))
	copy(expectedQualify[72:], wire)
	if qualify.Operation != "qualify" || qualify.DirectVersion != 6 || qualify.Author != session.Author ||
		qualify.Contract != contract || qualify.Amount != "0" || qualify.Calldata != hex.EncodeToString(expectedQualify) ||
		len(simulated) != 1 || simulated[0] != "qualify:0x"+qualify.Calldata {
		t.Fatal("v6 qualification changed the author, contract, or exact ABI calldata")
	}
	if _, err := readPreparedAction(options.PreparedPath); err == nil {
		t.Fatal("v6 preparation unexpectedly became signable by submit")
	}
	options.Mode, options.PreparedPath = "direct-pool-prove-bundle", filepath.Join(dir, "prove.json")
	if err := directCommand(options); err != nil {
		t.Fatalf("canonical v6 proof refused: %v", err)
	}
	var prove preparedAction
	preparedRaw, err = os.ReadFile(options.PreparedPath)
	if err != nil || decodeStrict(preparedRaw, &prove) != nil {
		t.Fatalf("read v6 proof: %v", err)
	}
	seal, _ := hex.DecodeString(proof.Seal)
	preimage := append([]byte("fod:pool-offer:v6"), mustDecode(session.Author)...)
	preimage = binary.BigEndian.AppendUint16(preimage, session.Epoch)
	id := sha256.Sum256(preimage)
	expectedProve := append([]byte{}, id[:]...)
	expectedProve = append(expectedProve, journal...)
	expectedProve = append(expectedProve, seal...)
	expectedProve = binary.BigEndian.AppendUint16(expectedProve, uint16(len(wire)))
	expectedProve = append(expectedProve, wire...)
	if prove.Operation != "proveBundle" || prove.DirectOfferID != hex.EncodeToString(id[:]) ||
		prove.Calldata != hex.EncodeToString(expectedProve) || len(simulated) != 2 || simulated[1] != "proveBundle:0x"+prove.Calldata {
		t.Fatal("v6 proof changed the offer ID or exact ABI calldata")
	}
	if info, err := os.Stat(options.PreparedPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("v6 prepared proof is not owner-only")
	}
	activeAuthor = otherAuthor
	options.PreparedPath = filepath.Join(dir, "wrong-author.json")
	if err := directCommand(options); err == nil || len(simulated) != 2 {
		t.Fatal("second active author was allowed to prepare the first author's proof")
	}
	activeAuthor = session.Author
	options.CodeHash = strings.Repeat("00", 32)
	before := rpcCalls
	if err := directCommand(options); err == nil || rpcCalls != before {
		t.Fatal("wrong v6 code pin reached the node")
	}
	options.CodeHash, options.DirectVersion = directV6CodeHashHex, 5
	if err := directCommand(options); err == nil || rpcCalls != before {
		t.Fatal("wrong direct version reached the node")
	}
	options.DirectVersion = 6
	options.Amount = "1"
	if err := directCommand(options); err == nil || rpcCalls != before {
		t.Fatal("nonzero amount reached the node")
	}
	options.Amount = "0"
	options.CodeHash, options.Contract = directV6CodeHashHex, otherAuthor
	if err := directCommand(options); err == nil || len(simulated) != 2 {
		t.Fatal("sealed proof prepared for another contract")
	}
	options.Contract = contract
	estimateSuccess = false
	options.PreparedPath = filepath.Join(dir, "failed-simulation.json")
	if err := directCommand(options); err == nil || len(simulated) != 3 {
		t.Fatal("refused contract simulation was accepted")
	}
	if _, err := os.Stat(options.PreparedPath); !os.IsNotExist(err) {
		t.Fatal("failed simulation wrote a prepared action")
	}
	var otherAuthorBytes [20]byte
	copy(otherAuthorBytes[:], mustDecode(otherAuthor))
	otherID := poolV6OfferID(otherAuthorBytes, session.Epoch)
	if bytes.Equal(id[:], otherID[:]) {
		t.Fatal("distinct authors share a v6 offer ID")
	}
}
