package main

import (
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type constructionVector struct {
	Note string `json:"note"`
	CID  struct {
		Data           string `json:"data"`
		CIDV1RawSha256 string `json:"cidV1RawSha256"`
	} `json:"cid"`
}

// TestTheCliComputesTheVectorsCID is the CLI's side of the shared construction
// vector: the CID strings a manifest carries have to be the same value the
// registration guest computes over the blob bytes, so the vector pins both.
func TestTheCliComputesTheVectorsCID(t *testing.T) {
	raw, err := os.ReadFile("../../proofs/vectors/ecies-v1.json")
	if err != nil {
		t.Fatalf("read the shared vector: %v", err)
	}
	var vector constructionVector
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatalf("parse the shared vector: %v", err)
	}
	if vector.CID.Data == "" || vector.CID.CIDV1RawSha256 == "" {
		t.Fatal("the vector carries no CID section")
	}

	// cidRawSha256 renders the base32 multibase form; the vector carries the raw
	// bytes, so decode one and compare with the other.
	rendered := cidRawSha256([]byte(vector.CID.Data))
	if !strings.HasPrefix(rendered, "b") {
		t.Fatalf("the CLI rendered %q, which is not a CIDv1 value", rendered)
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).
		DecodeString(strings.ToUpper(rendered[1:]))
	if err != nil {
		t.Fatalf("the CLI's CID is not base32: %v", err)
	}
	if hex.EncodeToString(decoded) != vector.CID.CIDV1RawSha256 {
		t.Fatalf("the CLI computes %s, the vector says %s", hex.EncodeToString(decoded), vector.CID.CIDV1RawSha256)
	}
}
