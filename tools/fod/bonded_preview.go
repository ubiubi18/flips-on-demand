package main

// Purchased previews use the buyer's retained ciphertext and delivery key.
// Only raster images and the two original image orders cross into the UI.
import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/idena-network/idena-go/rlp"
)

type bondedPreviewFlip struct {
	CID    string     `json:"cid"`
	Images []string   `json:"images"`
	Orders [2][4]byte `json:"orders"`
}
type bondedPreview struct {
	Trade string              `json:"trade"`
	Epoch uint16              `json:"epoch"`
	Count int                 `json:"count"`
	Flips []bondedPreviewFlip `json:"flips"`
}

func previewRaster(raw []byte) (string, error) {
	if len(raw) == 0 || len(raw) > 2*1024*1024 {
		return "", errors.New("preview image is too large or empty")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 || config.Width*config.Height > 8*1024*1024 {
		return "", errors.New("flip preview requires a valid bounded JPEG or PNG image")
	}
	if _, _, err = image.Decode(bytes.NewReader(raw)); err != nil {
		return "", errors.New("flip preview image is incomplete")
	}
	return "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

func previewFlipParts(public, private []byte) (bondedPreviewFlip, error) {
	var out bondedPreviewFlip
	if err := validateDirectFlipParts(public, private); err != nil {
		return out, err
	}
	if validateDirectFlipRLP(public, private) != nil {
		public, _ = directPlaintextRLP(public)
		private, _ = directPlaintextRLP(private)
	}
	pub, _ := directRLPList(public)
	if len(pub) == 1 && pub[0].kind == rlp.List {
		pub, _ = directNestedRLP(pub[0])
	}
	priv, _ := directRLPList(private)
	images := append([]directRLPItem{}, pub...)
	var orders []directRLPItem
	if priv[0].kind == rlp.List {
		more, _ := directNestedRLP(priv[0])
		images = append(images, more...)
		orders, _ = directNestedRLP(priv[1])
	} else {
		for _, item := range priv {
			if item.kind == rlp.List {
				orders, _ = directNestedRLP(item)
			} else {
				images = append(images, item)
			}
		}
	}
	for _, item := range images {
		url, err := previewRaster(item.data)
		if err != nil {
			return out, err
		}
		out.Images = append(out.Images, url)
	}
	for i, order := range orders {
		entries, _ := directNestedRLP(order)
		seen := [4]bool{}
		for j, item := range entries {
			out.Orders[i][j], _ = directRLPIndex(item)
			index := out.Orders[i][j]
			if seen[index] {
				return out, errors.New("preview image order is not a permutation")
			}
			seen[index] = true
		}
	}
	return out, nil
}

func readBondedPreview(c *client, contract string, t *bondedV8Trade, b *bondedV8Bundle, request *bondedV8Request) (*bondedPreview, error) {
	if request.Operation != "preview" || request.Caller != t.Buyer || t.State != 5 {
		return nil, errors.New("only the buyer can preview a completed purchase")
	}
	var coinbase string
	if err := c.call("dna_getCoinbaseAddr", []any{}, &coinbase); err != nil {
		return nil, err
	}
	owner, err := directAddress(coinbase)
	if err != nil || owner != t.Buyer {
		return nil, errors.New("purchase belongs to another active identity")
	}
	vault, err := openBondedV8(c, contract, t, b, request)
	if err != nil {
		return nil, err
	}
	keys, err := EpochKeysFromVaultPayload(*vault)
	if err != nil {
		return nil, err
	}
	retained, err := readBondedV8Retained(contract, t, b, request.RetainedFile)
	if err != nil {
		return nil, err
	}
	out := &bondedPreview{Trade: t.ID, Epoch: b.Epoch, Count: int(b.Count), Flips: []bondedPreviewFlip{}}
	for _, saved := range retained.Availability.Blobs {
		blob, _ := hex.DecodeString(saved.Hex)
		_, publicCipher, privateCipher, err := ParseFlipBlob(blob)
		if err != nil {
			return nil, err
		}
		public, err := keys.DecryptPublicPart(publicCipher)
		if err != nil {
			return nil, err
		}
		private, err := keys.DecryptPrivatePart(privateCipher)
		if err != nil {
			clear(public)
			return nil, err
		}
		flip, err := previewFlipParts(public, private)
		clear(public)
		clear(private)
		if err != nil {
			return nil, err
		}
		flip.CID = saved.CID
		out.Flips = append(out.Flips, flip)
	}
	return out, nil
}
