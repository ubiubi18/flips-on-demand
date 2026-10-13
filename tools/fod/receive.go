package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReceivedFlip is the result of decrypting a purchased flip blob.
type ReceivedFlip struct {
	AuthorPublicKeyHex string `json:"authorPublicKeyHex"`
	DecryptingAddress  string `json:"decryptingAddress"`
	Epoch              uint16 `json:"epoch"`
	PublicPartBytes    int    `json:"publicPartBytes"`
	PrivatePartBytes   int    `json:"privatePartBytes"`
	PublicPartText     string `json:"publicPartPreview,omitempty"`
	PrivatePartText    string `json:"privatePartPreview,omitempty"`
	OutputPath         string `json:"outputPath,omitempty"`
}

// receiveFlip decrypts a flip blob using the keys stored in a vault.
//
// This is the buyer path: the vault holds the purchased epoch keys, the blob is
// the ciphertext the seller promised, and both parts must open. It deliberately
// does not verify where the blob came from - that is the delivery proof's job.
func receiveFlip(vaultPath, blobPath, outPath, passphrase string) (*ReceivedFlip, error) {
	payload, err := ReadVault(vaultPath, passphrase)
	if err != nil {
		return nil, err
	}
	var stored VaultPayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, fmt.Errorf("parse vault payload: %w", err)
	}
	if stored.Kind != "fod.epoch-keys" {
		return nil, fmt.Errorf("vault does not hold epoch keys (kind=%q)", stored.Kind)
	}
	keys, err := EpochKeysFromVaultPayload(stored)
	if err != nil {
		return nil, err
	}

	blob, err := os.ReadFile(blobPath)
	if err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}
	authorPubKey, publicCipher, privateCipher, err := ParseFlipBlob(blob)
	if err != nil {
		return nil, err
	}

	publicPlain, err := keys.DecryptPublicPart(publicCipher)
	if err != nil {
		return nil, fmt.Errorf("decrypt public part: %w", err)
	}
	privatePlain, err := keys.DecryptPrivatePart(privateCipher)
	if err != nil {
		return nil, fmt.Errorf("decrypt private part: %w", err)
	}

	result := &ReceivedFlip{
		AuthorPublicKeyHex: fmt.Sprintf("%x", authorPubKey),
		DecryptingAddress:  keys.Address,
		Epoch:              keys.Epoch,
		PublicPartBytes:    len(publicPlain),
		PrivatePartBytes:   len(privatePlain),
		PublicPartText:     previewText(publicPlain),
		PrivatePartText:    previewText(privatePlain),
	}
	if outPath != "" {
		if err := writePrivateBytes(outPath, publicPlain); err != nil {
			return nil, fmt.Errorf("write public part: %w", err)
		}
		if err := writePrivateBytes(outPath+".private", privatePlain); err != nil {
			return nil, fmt.Errorf("write private part: %w", err)
		}
		result.OutputPath = outPath
	}
	return result, nil
}

// previewText returns a short printable preview when the plaintext looks like
// text, and an empty string otherwise, so binary content is not dumped.
func previewText(data []byte) string {
	const limit = 60
	if len(data) == 0 {
		return ""
	}
	for _, b := range data {
		if b < 0x20 && b != '\n' && b != '\t' && b != '\r' {
			return ""
		}
	}
	if len(data) > limit {
		return string(data[:limit]) + "..."
	}
	return string(data)
}
