package main

// Explorer pictures are optional browsing aids. The offer's CID inventory is
// checked against contract storage before this code runs; explorer images never
// determine a purchase or prove the private content delivered to a buyer.

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const standaloneExplorerURL = "https://api.idena.io/api/"
const standalonePreviewFlips = 5

type standalonePreviewFlip struct {
	CID    string   `json:"cid"`
	Images []string `json:"images"`
}

type standalonePreview struct {
	OfferID string                  `json:"offerId"`
	Count   int                     `json:"count"`
	Flips   []standalonePreviewFlip `json:"flips"`
}

func standaloneExplorerGET(ctx context.Context, httpClient *http.Client, base, route string, dst any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+route, nil)
	if err != nil {
		return err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("explorer answered %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("explorer returned an oversized or trailing response")
	}
	return nil
}

func standaloneJPEGDataURL(value string) string {
	if !strings.HasPrefix(value, "0x") || len(value) < 10 || len(value) > 512*1024+2 || len(value)%2 != 0 {
		return ""
	}
	raw, err := hex.DecodeString(value[2:])
	if err != nil || len(raw) < 4 || raw[0] != 0xff || raw[1] != 0xd8 || raw[len(raw)-2] != 0xff || raw[len(raw)-1] != 0xd9 {
		return ""
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
}

func readStandalonePreview(ctx context.Context, offer *poolV7MarketOffer, base string, httpClient *http.Client) (*standalonePreview, error) {
	if offer == nil || len(offer.OriginalCIDs) != int(offer.Covered) || offer.Epoch == 0 ||
		!isHex(offer.Author, addressBytes) || !isHex(offer.OfferID, 32) {
		return nil, errors.New("the selected bundle has no verified public inventory")
	}
	limit := len(offer.OriginalCIDs)
	if limit > standalonePreviewFlips {
		limit = standalonePreviewFlips
	}
	preview := &standalonePreview{OfferID: offer.OfferID, Count: len(offer.OriginalCIDs), Flips: make([]standalonePreviewFlip, limit)}
	for i := range preview.Flips {
		cid := offer.OriginalCIDs[i]
		if !isFlipCID(cid) {
			return nil, errors.New("the selected bundle has an invalid public CID")
		}
		preview.Flips[i] = standalonePreviewFlip{CID: cid, Images: []string{}}
	}
	var listing struct {
		Result []struct {
			CID    string `json:"cid"`
			Author string `json:"author"`
			Epoch  uint16 `json:"epoch"`
		} `json:"result"`
	}
	if err := standaloneExplorerGET(ctx, httpClient, base,
		"Epoch/"+strconv.Itoa(int(offer.Epoch))+"/Identity/0x"+offer.Author+"/Flips?limit=100", &listing); err != nil {
		return preview, nil
	}
	listed := make(map[string]bool, len(listing.Result))
	for _, item := range listing.Result {
		if item.Epoch == offer.Epoch && strings.EqualFold(strings.TrimPrefix(item.Author, "0x"), offer.Author) {
			listed[item.CID] = true
		}
	}
	var wait sync.WaitGroup
	for i := range preview.Flips {
		if !listed[preview.Flips[i].CID] {
			continue
		}
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			var content struct {
				Result struct {
					Pics []string `json:"pics"`
				} `json:"result"`
			}
			if err := standaloneExplorerGET(ctx, httpClient, base, "Flip/"+preview.Flips[i].CID+"/Content", &content); err != nil || len(content.Result.Pics) != 2 {
				return
			}
			first, second := standaloneJPEGDataURL(content.Result.Pics[0]), standaloneJPEGDataURL(content.Result.Pics[1])
			if first != "" && second != "" {
				preview.Flips[i].Images = []string{first, second}
			}
		}(i)
	}
	wait.Wait()
	return preview, nil
}

func (s *standaloneServer) preview(ctx context.Context) (*standalonePreview, error) {
	if s.market == nil {
		return nil, errors.New("market reader unavailable")
	}
	state, err := s.market()
	if err != nil {
		return nil, err
	}
	market, ok := state.(*poolV7MarketSnapshot)
	if !ok || market.NextOffer == nil {
		return nil, errors.New("no selected bundle to preview")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("explorer redirect refused")
	}}
	return readStandalonePreview(ctx, market.NextOffer, standaloneExplorerURL, client)
}
