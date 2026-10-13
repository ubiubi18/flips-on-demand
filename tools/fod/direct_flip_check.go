package main

// The buyer checks the same supported four-image, two-order plaintext shape as
// the registration guest before an automatic direct-sale confirmation.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/idena-network/idena-go/rlp"
)

type directRLPItem struct {
	kind rlp.Kind
	data []byte
}

func directRLPList(encoded []byte) ([]directRLPItem, error) {
	content, rest, err := rlp.SplitList(encoded)
	if err != nil || len(rest) != 0 {
		return nil, errors.New("part is not one RLP list")
	}
	items := make([]directRLPItem, 0, 4)
	for len(content) > 0 {
		kind, data, next, err := rlp.Split(content)
		if err != nil {
			return nil, err
		}
		items = append(items, directRLPItem{kind: kind, data: data})
		content = next
	}
	return items, nil
}

func directNestedRLP(item directRLPItem) ([]directRLPItem, error) {
	if item.kind != rlp.List {
		return nil, errors.New("expected RLP list")
	}
	items := make([]directRLPItem, 0, 4)
	for data := item.data; len(data) > 0; {
		kind, body, rest, err := rlp.Split(data)
		if err != nil {
			return nil, err
		}
		items = append(items, directRLPItem{kind: kind, data: body})
		data = rest
	}
	return items, nil
}

func directRLPIndex(item directRLPItem) (byte, error) {
	if item.kind == rlp.List {
		nested, err := directNestedRLP(item)
		if err != nil || len(nested) == 0 {
			return 0, errors.New("order index list is empty")
		}
		item = nested[0]
	}
	if item.kind == rlp.List || len(item.data) > 1 {
		return 0, errors.New("order index is not one byte")
	}
	if len(item.data) == 0 {
		return 0, nil
	}
	if item.data[0] > 3 {
		return 0, errors.New("order index exceeds the four images")
	}
	return item.data[0], nil
}

func directPlaintextRLP(part []byte) ([]byte, error) {
	text := string(part)
	text = strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")
	if len(text)%2 != 0 {
		return nil, errors.New("decrypted flip part has odd hex length")
	}
	decoded, err := hex.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("decrypted flip part is not hex: %w", err)
	}
	return decoded, nil
}

func validateDirectFlipParts(publicPlain, privatePlain []byte) error {
	if len(publicPlain) == 0 || len(privatePlain) == 0 ||
		len(publicPlain)+len(privatePlain) > maxRetainedFlipBytes {
		return errors.New("decrypted flip parts have invalid length")
	}
	if err := validateDirectFlipRLP(publicPlain, privatePlain); err == nil {
		return nil
	}
	publicRLP, err := directPlaintextRLP(publicPlain)
	if err != nil {
		return err
	}
	privateRLP, err := directPlaintextRLP(privatePlain)
	if err != nil {
		return err
	}
	return validateDirectFlipRLP(publicRLP, privateRLP)
}

func validateDirectFlipRLP(publicRLP, privateRLP []byte) error {
	public, err := directRLPList(publicRLP)
	if err != nil {
		return err
	}
	if len(public) == 1 && public[0].kind == rlp.List {
		public, err = directNestedRLP(public[0])
		if err != nil {
			return err
		}
	}
	private, err := directRLPList(privateRLP)
	if err != nil {
		return err
	}
	var images, orders []directRLPItem
	images = append(images, public...)
	if len(private) > 0 && private[0].kind == rlp.List {
		if len(private) != 2 || private[1].kind != rlp.List {
			return errors.New("private flip part has an invalid nested layout")
		}
		more, err := directNestedRLP(private[0])
		if err != nil {
			return err
		}
		images = append(images, more...)
		orders, err = directNestedRLP(private[1])
		if err != nil {
			return err
		}
	} else {
		for _, item := range private {
			if item.kind == rlp.List {
				if orders != nil {
					return errors.New("more than one order list")
				}
				orders, err = directNestedRLP(item)
				if err != nil {
					return err
				}
			} else {
				if orders != nil {
					return errors.New("image after order list")
				}
				images = append(images, item)
			}
		}
	}
	if len(images) != 4 {
		return fmt.Errorf("flip has %d images, expected four", len(images))
	}
	for _, image := range images {
		if image.kind == rlp.List || len(image.data) == 0 {
			return errors.New("flip has an empty or nested image")
		}
	}
	if len(orders) != 2 {
		return fmt.Errorf("flip has %d orders, expected two", len(orders))
	}
	var sequences [2][4]byte
	for i, order := range orders {
		entries, err := directNestedRLP(order)
		if err != nil || len(entries) != 4 {
			return errors.New("flip order must have four entries")
		}
		for j, item := range entries {
			index, err := directRLPIndex(item)
			if err != nil {
				return err
			}
			sequences[i][j] = index
		}
	}
	if sequences[0] == sequences[1] {
		return errors.New("flip orders are identical")
	}
	return nil
}
