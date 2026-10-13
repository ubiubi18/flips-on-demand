package main

// The reveal path (`SPEC.md` 7.2): the call has to be the order id followed by
// exactly the canonical secrets the registration committed, and a vault that
// cannot settle the order has to be refused before a fee is spent.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/crypto"
)

func TestRevealSigningRequiresPrivateOutputBeforeLoadingKeys(t *testing.T) {
	if os.Getenv("FOD_TEST_REVEAL_STDOUT") == "1" {
		runSubmit(os.Getenv("FOD_TEST_REVEAL_PREPARED"), "missing-disposable-key", 1, 228,
			big.NewInt(1), big.NewInt(0), "", false, "http://127.0.0.1:1", "", nil, "", "")
		return
	}
	prepared := buildRevealAction(testContract, "idena-local", 7, strings.Repeat("ab", 32), []byte("synthetic secrets"))
	path := filepath.Join(t.TempDir(), "prepared.json")
	if err := writePrivateJSON(path, prepared); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestRevealSigningRequiresPrivateOutputBeforeLoadingKeys$")
	cmd.Env = append(os.Environ(), "FOD_TEST_REVEAL_STDOUT=1", "FOD_TEST_REVEAL_PREPARED="+path)
	output, err := cmd.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("-out FILE is required")) {
		t.Fatalf("reveal did not require private output before loading keys: %v, %s", err, output)
	}
}

func TestRevealSigningKeepsSecretsOutOfStdoutAndOutsideSymlinks(t *testing.T) {
	dir := t.TempDir()
	keyPath, _ := writeTestKey(t, dir, "disposable-nodekey")
	payload, _ := testVaultPayload(t, 228)
	bundles, err := servedBundlesFromPayloads([]VaultPayload{payload})
	if err != nil {
		t.Fatal(err)
	}
	var bundle servedBundle
	for _, candidate := range bundles {
		bundle = candidate
	}
	secrets, err := revealSecrets(payload, bundle.Commitment)
	if err != nil {
		t.Fatal(err)
	}
	deliveryKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	order := poolOrderRecordFor(t, mustDecode(bundle.Commitment), bytes.Repeat([]byte{1}, 20), crypto.FromECDSAPub(&deliveryKey.PublicKey), 7, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		switch req.Method {
		case "contract_readData":
			fmt.Fprintf(w, `{"result":"0x%x"}`, order)
		case "contract_iterateMap":
			writeRevealRegistryPage(t, w, req.Params, bundle.Commitment, []uint64{7}, 0)
		case "bcn_lastBlock":
			fmt.Fprint(w, `{"result":{"height":1000}}`)
		default:
			t.Errorf("unexpected RPC %s", req.Method)
			http.Error(w, "unexpected method", 400)
		}
	}))
	defer server.Close()
	prepared := filepath.Join(dir, "prepared.json")
	if err := writePrivateJSON(prepared, buildRevealAction(testContract, "idena-local", 7, bundle.Commitment, secrets)); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "keep")
	if err := os.WriteFile(outside, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "signed.json")
	if err := os.Symlink(outside, out); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = stdout }()
	runSubmit(prepared, keyPath, 1, 228, big.NewInt(3_000_000), big.NewInt(0), out, false, server.URL, "", nil, "", "")
	writer.Close()
	os.Stdout = stdout
	printed, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(printed, []byte(hex.EncodeToString(secrets))) || bytes.Contains(printed, []byte("raw         ")) {
		t.Fatal("printed unrevealed keys or the secret-bearing transaction")
	}
	if secure, err := VaultPermissionsSecure(out); err != nil || !secure {
		t.Fatal("signed reveal is not private", err)
	}
	untouched, err := os.ReadFile(outside)
	if err != nil || string(untouched) != "unchanged" {
		t.Fatal("signed reveal overwrote an outside file", err)
	}
	stored, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var submitted SubmittedTransaction
	if err := json.Unmarshal(stored, &submitted); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(submitted.RawTx, hex.EncodeToString(secrets)) || submitted.Broadcast {
		t.Fatal("private output did not preserve the unbroadcast reveal transaction")
	}
}

func writeRevealRegistryPage(t *testing.T, w http.ResponseWriter, params []json.RawMessage, bundle string, open []uint64, strikes uint32) {
	t.Helper()
	if len(params) < 2 {
		t.Fatal("registry request lacks a prefix")
	}
	var prefix string
	if err := json.Unmarshal(params[1], &prefix); err != nil {
		t.Fatal(err)
	}
	var value []byte
	switch prefix {
	case marketPoolPrefix:
		value = make([]byte, marketPoolValueBytes)
		binary.BigEndian.PutUint32(value[marketPoolStrikesOffset:marketPoolNonceOffset], strikes)
	case "popen/":
		value = make([]byte, 8*len(open))
		for i, id := range open {
			binary.BigEndian.PutUint64(value[i*8:(i+1)*8], id)
		}
	default:
		t.Errorf("unexpected registry prefix %q", prefix)
		http.Error(w, "unexpected prefix", 400)
		return
	}
	fmt.Fprintf(w, `{"result":{"items":[{"key":"0x%s","value":"0x%x"}],"continuationToken":"0x"}}`, bundle, value)
}

func TestDisclosureRefusesClosedOrdersAndMissedInclusionWindow(t *testing.T) {
	secrets := []byte("synthetic committed keys")
	// This fixture's bundle commits the actual reveal payload below.
	digest := sha256.Sum256(secrets)
	bundle := hex.EncodeToString(digest[:])
	for _, operation := range []string{operationDeliver, operationReveal, operationFulfill} {
		for _, tc := range []struct {
			name  string
			head  uint64
			state byte
			want  bool
		}{
			{"funding-head", 1000, 1, true}, {"last-safe-head", 1001, 2, true},
			{"next-block-is-deadline", 1002, 1, false}, {"deadline", 1003, 2, false},
			{"expired", 1004, 1, false}, {"settled", 1001, 3, false}, {"refunded", 1001, 4, false},
			{"unavailable-head", 0, 1, false}, {"before-funding", 999, 1, false},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				key, err := crypto.GenerateKey()
				if err != nil {
					t.Fatal(err)
				}
				order := poolOrderRecordFor(t, mustDecode(bundle), bytes.Repeat([]byte{1}, 20), crypto.FromECDSAPub(&key.PublicKey), 7, tc.state)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var req struct {
						Method string            `json:"method"`
						Params []json.RawMessage `json:"params"`
					}
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						http.Error(w, "bad request", 400)
						return
					}
					switch req.Method {
					case "contract_readData":
						fmt.Fprintf(w, `{"result":"0x%x"}`, order)
					case "contract_iterateMap":
						writeRevealRegistryPage(t, w, req.Params, bundle, []uint64{7}, 0)
					case "bcn_lastBlock":
						fmt.Fprintf(w, `{"result":{"height":%d}}`, tc.head)
					default:
						t.Errorf("unexpected RPC %s", req.Method)
						http.Error(w, "unexpected method", 400)
					}
				}))
				defer server.Close()
				c := &client{url: server.URL, http: newHTTPClient()}
				prepared := buildRevealAction(testContract, "idena", 7, bundle, secrets)
				prepared.Operation = operation
				if got := validatePreparedDisclosure(c, prepared); (got == nil) != tc.want {
					t.Fatalf("allowed=%t, wanted=%t: %v", got == nil, tc.want, got)
				}
				if !tc.want {
					desk := &serveDesk{client: c, broadcast: true, identity: key}
					if err := desk.submit(prepared, "refused-disclosure"); err == nil {
						t.Fatal("delivery desk bypassed the disclosure guard")
					}
				}
				if tc.want {
					prepared.OfferID = 8
					if err := validatePreparedDisclosure(c, prepared); err == nil {
						t.Fatal("editable order metadata overrode calldata")
					}
					prepared.OfferID = 7
					if operation == operationReveal {
						prepared.Calldata = revealCalldata(7, []byte("different keys"))
						if err := validatePreparedDisclosure(c, prepared); err == nil {
							t.Fatal("unbound keys accepted for disclosure")
						}
					}
				}
			})
		}
	}
}

func TestRevealDisclosureChecksTheWholeBundleBeforeSigning(t *testing.T) {
	secrets := []byte("shared reveal keys")
	digest := sha256.Sum256(secrets)
	bundle := hex.EncodeToString(digest[:])
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		head          uint64
		namedFundedAt uint64
		open          []uint64
		strikes       uint32
		want          bool
	}{
		{"both-inside-window", 1001, 1000, []uint64{7, 8}, 0, true},
		{"older-sibling-at-boundary", 1002, 1001, []uint64{7, 8}, 0, false},
		{"prior-refund", 1001, 1000, []uint64{7}, 1, false},
		{"named-order-missing", 1001, 1000, []uint64{8}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			named := poolOrderRecordFor(t, digest[:], bytes.Repeat([]byte{1}, 20), crypto.FromECDSAPub(&key.PublicKey), 7, 1)
			binary.BigEndian.PutUint64(named[133:141], tc.namedFundedAt)
			sibling := poolOrderRecordFor(t, digest[:], bytes.Repeat([]byte{2}, 20), crypto.FromECDSAPub(&key.PublicKey), 8, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "bad request", 400)
					return
				}
				switch req.Method {
				case "contract_readData":
					var storageKey string
					if len(req.Params) < 2 || json.Unmarshal(req.Params[1], &storageKey) != nil {
						http.Error(w, "bad storage key", 400)
						return
					}
					if storageKey == "porder/8" {
						fmt.Fprintf(w, `{"result":"0x%x"}`, sibling)
					} else if storageKey == "porder/7" {
						fmt.Fprintf(w, `{"result":"0x%x"}`, named)
					} else {
						http.Error(w, "unexpected storage key", 400)
					}
				case "contract_iterateMap":
					writeRevealRegistryPage(t, w, req.Params, bundle, tc.open, tc.strikes)
				case "bcn_lastBlock":
					fmt.Fprintf(w, `{"result":{"height":%d}}`, tc.head)
				default:
					http.Error(w, "unexpected method", 400)
				}
			}))
			defer server.Close()
			client := &client{url: server.URL, http: newHTTPClient()}
			err := validatePreparedDisclosure(client, buildRevealAction(testContract, "idena", 7, bundle, secrets))
			if (err == nil) != tc.want {
				t.Fatalf("allowed=%t, wanted=%t: %v", err == nil, tc.want, err)
			}
		})
	}
}

func TestRevealCalldataIsTheOrderIdAndTheCommittedSecrets(t *testing.T) {
	payload, author := testVaultPayload(t, 228)
	keys, err := EpochKeysFromVaultPayload(payload)
	if err != nil {
		t.Fatalf("rebuild the epoch keys: %v", err)
	}
	publicPart, privatePart := keys.SecretBytes()
	want, err := SecretsCanonicalBytes(payload.Epoch, author, publicPart, privatePart)
	if err != nil {
		t.Fatalf("encode the canonical secrets: %v", err)
	}
	commitment, err := SecretsCommitmentHex(payload.Epoch, author, publicPart, privatePart)
	if err != nil {
		t.Fatalf("commit the secrets: %v", err)
	}

	secrets, err := revealSecrets(payload, commitment)
	if err != nil {
		t.Fatalf("the vault did not settle its own bundle: %v", err)
	}
	if !bytes.Equal(secrets, want) {
		t.Fatal("the reveal re-encoded different bytes than the canonical encoding")
	}
	arg, err := hex.DecodeString(revealCalldata(7, secrets))
	if err != nil {
		t.Fatalf("decode the calldata: %v", err)
	}
	if len(arg) != 8+len(want) {
		t.Fatalf("the calldata is %d bytes, expected %d", len(arg), 8+len(want))
	}
	if id := binary.BigEndian.Uint64(arg[:8]); id != 7 {
		t.Fatalf("the calldata names order %d", id)
	}
	if !bytes.Equal(arg[8:], want) {
		t.Fatal("the calldata does not carry the canonical secrets")
	}

	if _, err := revealSecrets(payload, strings.Repeat("ab", 32)); err == nil {
		t.Fatal("a vault that does not hash to the order's bundle was accepted")
	}
}

func TestDeskRechecksDeadlineAfterSigningBeforeBroadcast(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	secrets := []byte("synthetic keys for a delayed signer")
	digest := sha256.Sum256(secrets)
	bundle := hex.EncodeToString(digest[:])
	order := poolOrderRecordFor(t, digest[:], bytes.Repeat([]byte{1}, 20), crypto.FromECDSAPub(&key.PublicKey), 7, 1)
	heads, broadcasts := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		switch req.Method {
		case "contract_readData":
			fmt.Fprintf(w, `{"result":"0x%x"}`, order)
		case "contract_iterateMap":
			writeRevealRegistryPage(t, w, req.Params, bundle, []uint64{7}, 0)
		case "bcn_lastBlock":
			heads++
			// Signing started with one valid future inclusion block. While
			// nonce/fee work completed, that last block was finalized.
			fmt.Fprintf(w, `{"result":{"height":%d}}`, 1000+heads)
		case "dna_getBalance":
			fmt.Fprint(w, `{"result":{"nonce":0}}`)
		case "dna_epoch":
			fmt.Fprint(w, `{"result":{"epoch":228}}`)
		case "bcn_sendRawTx":
			broadcasts++
			fmt.Fprint(w, `{"error":{"code":-32000,"message":"unexpected broadcast"}}`)
		default:
			t.Errorf("unexpected RPC %s", req.Method)
			http.Error(w, "unexpected method", 400)
		}
	}))
	defer server.Close()
	desk := &serveDesk{client: &client{url: server.URL, http: newHTTPClient()}, broadcast: true,
		identity: key, feePerGas: big.NewInt(1000), maxFee: big.NewInt(1_000_000_000_000_000_000), tips: big.NewInt(0)}
	if err := desk.submit(buildRevealAction(testContract, "idena", 7, bundle, secrets), "late-reveal"); err == nil {
		t.Fatal("late broadcast was accepted")
	}
	if heads != 2 || broadcasts != 0 {
		t.Fatalf("fresh head checks=%d, broadcasts=%d", heads, broadcasts)
	}
}

func TestRevealActionNamesTheOrderAndSendsNothing(t *testing.T) {
	secrets := []byte("some canonical secrets")
	action := buildRevealAction("0x2cfbbafa229f53a9c2009551a832019b3fc94244", "idena", 3, strings.Repeat("cd", 32), secrets)
	if action.Format != preparedRevealFormat || action.Operation != operationReveal {
		t.Fatalf("unexpected action: %s / %s", action.Format, action.Operation)
	}
	if action.Amount != "0" {
		t.Fatalf("a reveal sends no value, the action says %q", action.Amount)
	}
	if action.OfferID != 3 || action.Bundle != strings.Repeat("cd", 32) {
		t.Fatalf("the action names order %d and bundle %s", action.OfferID, action.Bundle)
	}
	want, err := hex.DecodeString(revealCalldata(3, secrets))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, err := hex.DecodeString(action.Calldata)
	if err != nil {
		t.Fatalf("decode the action calldata: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the action carries different calldata")
	}
}

// The prepared reveal carries the committed secrets, so it has to be written
// the way key material is written - and a file that already existed with wider
// permissions has to be tightened, not left as it was.
func TestRevealActionFileIsOwnerOnly(t *testing.T) {
	secrets := []byte("some canonical secrets")
	action := buildRevealAction("0x2cfbbafa229f53a9c2009551a832019b3fc94244", "idena", 4, strings.Repeat("cd", 32), secrets)
	path := filepath.Join(t.TempDir(), "prepared-reveal.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write the pre-existing file: %v", err)
	}
	if err := writeRevealAction(path, action); err != nil {
		t.Fatalf("write the prepared reveal: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the prepared reveal: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the prepared reveal is %v, expected 0600 - it carries the committed secrets", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	var back preparedAction
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("the written action does not parse: %v", err)
	}
	if back.Calldata != action.Calldata {
		t.Fatal("the written action carries different calldata")
	}
}
