package main

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDirectFlipStructureMatchesSealedVector(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/registration/vectors/blob-flip-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		PublicPartPlaintext  string `json:"publicPartPlaintext"`
		PrivatePartPlaintext string `json:"privatePartPlaintext"`
	}
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatal(err)
	}
	public, err := hex.DecodeString(vector.PublicPartPlaintext)
	if err != nil {
		t.Fatal(err)
	}
	private, err := hex.DecodeString(vector.PrivatePartPlaintext)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectFlipParts(public, private); err != nil {
		t.Fatalf("the sealed flip structure was rejected: %v", err)
	}
	rawPublic, err := hex.DecodeString(strings.TrimPrefix(string(public), "0x"))
	if err != nil {
		t.Fatal(err)
	}
	rawPrivate, err := hex.DecodeString(strings.TrimPrefix(string(private), "0x"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectFlipParts(rawPublic, rawPrivate); err != nil {
		t.Fatalf("the raw RLP flip structure was rejected: %v", err)
	}
	for name, parts := range map[string][2][]byte{
		"empty":       {[]byte("0xc0"), private},
		"malformed":   {[]byte("0xzz"), private},
		"same orders": {public, []byte("0xc7c24142c3c0c0c0")},
		// The proof guest treats a leading private list as the image list and
		// requires a second list for orders. A buyer must not accept it alone.
		"orders without private images": {
			[]byte{0xc4, 'A', 'B', 'C', 'D'},
			[]byte{0xcb, 0xca, 0xc4, 0x80, 0x01, 0x02, 0x03, 0xc4, 0x03, 0x02, 0x01, 0x80},
		},
		"noncanonical image": {
			[]byte{0xc2, 'A', 'B'},
			[]byte{0xce, 0x81, 'C', 'D', 0xca, 0xc4, 0x80, 0x01, 0x02, 0x03, 0xc4, 0x03, 0x02, 0x01, 0x80},
		},
	} {
		if err := validateDirectFlipParts(parts[0], parts[1]); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if err := validateDirectFlipParts([]byte(strings.Repeat("x", 2*maxRetainedFlipBytes+3)), private); err == nil {
		t.Fatal("oversized decrypted part was accepted")
	}
}
