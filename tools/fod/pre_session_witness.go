package main

// A private content witness needs the author's submitted CIDs and epoch keys,
// but it does not need a validation verdict. The later session witness still
// has to match this exact inventory before the contract can register it.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const preSessionInventoryFormat = "fod.pre-session-inventory:v1"

type preSessionInventory struct {
	Format   string   `json:"format"`
	Author   string   `json:"author"`
	Epoch    uint16   `json:"epoch"`
	Manifest []string `json:"manifest"`
}

func readPreSessionInventory(path string) (*preSessionInventory, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect pre-session inventory: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 16*1024 {
		return nil, fmt.Errorf("pre-session inventory must be an owner-only regular file of at most 16 KiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read pre-session inventory: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 16*1024+1))
	decoder.DisallowUnknownFields()
	var inventory preSessionInventory
	if err := decoder.Decode(&inventory); err != nil {
		return nil, fmt.Errorf("decode pre-session inventory: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("pre-session inventory has trailing data")
	}
	if inventory.Format != preSessionInventoryFormat || inventory.Epoch == 0 ||
		len(inventory.Manifest) == 0 || len(inventory.Manifest) > 32 {
		return nil, fmt.Errorf("invalid pre-session inventory")
	}
	if _, err := parseIdentityAddress(inventory.Author); err != nil {
		return nil, fmt.Errorf("invalid pre-session author: %w", err)
	}
	return &inventory, nil
}

func writePreSessionWitness(out, inventoryPath, nodekeyPath, blobDir string) error {
	inventory, err := readPreSessionInventory(inventoryPath)
	if err != nil {
		return err
	}
	bundle := &SessionBundle{
		Author: inventory.Author, Epoch: inventory.Epoch, Manifest: inventory.Manifest,
	}
	return writeRegistrationBlobInput(out, "", nodekeyPath, "", blobDir, bundle, nil)
}
