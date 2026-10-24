package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/rlp"
)

func TestBondedPurchasedPreviewImagesAndOrders(t *testing.T) {
	var pictures [][]byte
	for i := 0; i < 4; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 4, 3))
		img.Set(1, 1, color.RGBA{uint8(i * 60), 100, 200, 255})
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, nil); err != nil {
			t.Fatal(err)
		}
		pictures = append(pictures, buf.Bytes())
	}
	orders := [][]uint64{{2, 0, 1, 3}, {3, 1, 0, 2}}
	pub, _ := rlp.EncodeToBytes([]any{pictures[:2]})
	priv, _ := rlp.EncodeToBytes([]any{pictures[2:], orders})
	for _, hexPlaintext := range []bool{false, true} {
		public, private := pub, priv
		if hexPlaintext {
			public = []byte("0x" + hex.EncodeToString(pub))
			private = []byte("0x" + hex.EncodeToString(priv))
		}
		result, err := previewFlipParts(public, private)
		if err != nil {
			t.Fatal(err)
		}
		if result.Orders != [2][4]byte{{2, 0, 1, 3}, {3, 1, 0, 2}} || len(result.Images) != 4 {
			t.Fatal("original sequences were changed")
		}
		for i, url := range result.Images {
			raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/jpeg;base64,"))
			if err != nil || !bytes.Equal(raw, pictures[i]) {
				t.Fatal("image changed")
			}
		}
		encoded, _ := json.Marshal(result)
		if !bytes.Contains(encoded, []byte(`"orders":[[2,0,1,3],[3,1,0,2]]`)) {
			t.Fatal("orders not JSON arrays")
		}
	}
	if _, err := previewRaster([]byte(`<svg onload="alert(1)"></svg>`)); err == nil {
		t.Fatal("active content accepted")
	}
	if _, err := previewRaster(make([]byte, 2*1024*1024+1)); err == nil {
		t.Fatal("oversized image accepted")
	}
	bad, _ := rlp.EncodeToBytes([]any{pictures[2:], [][]uint64{{0, 0, 1, 2}, {3, 2, 1, 0}}})
	if _, err := previewFlipParts(pub, bad); err == nil {
		t.Fatal("invalid permutation accepted")
	}
}

func TestBondedPurchasedPreviewRequiresCompletedBuyer(t *testing.T) {
	buyer := strings.Repeat("aa", 20)
	for _, tt := range []struct {
		operation, caller string
		state             byte
	}{{"preview", buyer, 3}, {"preview", strings.Repeat("bb", 20), 5}, {"accept", buyer, 5}} {
		_, err := readBondedPreview(nil, "", &bondedV8Trade{Buyer: buyer, State: tt.state}, nil, &bondedV8Request{Operation: tt.operation, Caller: tt.caller})
		if err == nil {
			t.Fatal("preview permitted without completed buyer ownership")
		}
	}
}

func TestBondedMarketHistoryIncludesOtherIdentitiesWithoutTheirPrivateTrades(t *testing.T) {
	s, _, _ := standaloneV8Fixture(t)
	s.client.bondedVersion = 9
	original := s.client.url
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var request struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(raw, &request)
		var field string
		if len(request.Params) > 1 {
			_ = json.Unmarshal(request.Params[1], &field)
		}
		var result any
		handled := false
		if request.Method == "contract_readMap" && field == "v9/trade/" {
			trade := make([]byte, 308)
			copy(trade[48:68], bytes.Repeat([]byte{0x44}, 20))
			copy(trade[68:88], bytes.Repeat([]byte{0x55}, 20))
			binary.BigEndian.PutUint64(trade[153+8:169], 500)
			trade[306] = 5
			result = "0x" + hex.EncodeToString(trade)
			handled = true
		}
		if handled {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
			return
		}
		resp, err := http.Post(original, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Error(err)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if request.Method == "contract_readData" && field == "v9/totals" {
			var response map[string]any
			_ = json.Unmarshal(body, &response)
			totals := make([]byte, 104)
			for _, off := range []int{80, 88, 96} {
				binary.BigEndian.PutUint64(totals[off:off+8], 1)
			}
			binary.BigEndian.PutUint64(totals[96:], 2)
			response["result"] = "0x" + hex.EncodeToString(totals)
			body, _ = json.Marshal(response)
		}
		_, _ = w.Write(body)
	}))
	defer proxy.Close()
	s.client.url = proxy.URL
	snapshot, err := readBondedV8Snapshot(s.client, s.cfg.Contract, s.cfg.CodeHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Trades) != 0 || len(snapshot.PriceHistory) != 1 || snapshot.PriceHistory[0].Trade != "1" || snapshot.PriceHistory[0].Price != "500" {
		t.Fatalf("incorrect public history: %+v", snapshot.PriceHistory)
	}
}
