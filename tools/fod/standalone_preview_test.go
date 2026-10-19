package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStandalonePreviewMatchesExplorerListingAndFallsBackToCIDs(t *testing.T) {
	cid := "b" + strings.Repeat("a", 58)
	author := strings.Repeat("1", 40)
	jpeg := "0x" + hex.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xd9})
	listed := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/Flip/") {
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"pics": []string{jpeg, jpeg}}})
			return
		}
		if listed {
			_ = json.NewEncoder(w).Encode(map[string]any{"result": []any{map[string]any{"cid": cid, "author": "0x" + author, "epoch": 232}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": []any{map[string]any{"cid": cid, "author": "0x" + strings.Repeat("2", 40), "epoch": 232}}})
	}))
	defer server.Close()
	offer := &poolV7MarketOffer{OfferID: strings.Repeat("a", 64), Author: author, Epoch: 232, Covered: 1, OriginalCIDs: []string{cid}}
	preview, err := readStandalonePreview(context.Background(), offer, server.URL+"/", server.Client())
	if err != nil || preview.Count != 1 || len(preview.Flips[0].Images) != 2 ||
		!strings.HasPrefix(preview.Flips[0].Images[0], "data:image/jpeg;base64,") {
		t.Fatalf("matching public preview was not rendered: %+v, %v", preview, err)
	}
	listed = false
	preview, err = readStandalonePreview(context.Background(), offer, server.URL+"/", server.Client())
	if err != nil || preview.Flips[0].CID != cid || len(preview.Flips[0].Images) != 0 {
		t.Fatalf("unmatched explorer image was accepted or CID lost: %+v, %v", preview, err)
	}
	if standaloneJPEGDataURL("0x3c7363726970743e") != "" {
		t.Fatal("accepted non-image explorer content")
	}
}
