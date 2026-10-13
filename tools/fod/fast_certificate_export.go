package main

// Export a contract-retained certificate and the corresponding canonical
// header roots so any verifier can replay the content and session checks.

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type fastHeaderRoot struct {
	Height    uint64 `json:"height"`
	StateRoot string `json:"stateRoot"`
}

type fastCertificateHeaders struct {
	Inventory fastHeaderRoot `json:"inventory"`
	Verdict   fastHeaderRoot `json:"verdict"`
}

type fastWireReader struct {
	wire []byte
	at   int
}

func (r *fastWireReader) take(n int) ([]byte, error) {
	if n < 0 || n > len(r.wire)-r.at {
		return nil, errors.New("truncated fast session witness")
	}
	part := r.wire[r.at : r.at+n]
	r.at += n
	return part, nil
}

func (r *fastWireReader) leaf() error {
	keyLen, err := r.take(1)
	if err != nil || keyLen[0] == 0 || keyLen[0] > 21 {
		return errors.New("invalid fast state key")
	}
	if _, err = r.take(int(keyLen[0])); err != nil {
		return err
	}
	length, err := r.take(2)
	if err != nil {
		return err
	}
	valueLen := int(binary.BigEndian.Uint16(length))
	if valueLen > fastRegistrationMaxValueBytes {
		return errors.New("oversized fast state value")
	}
	if _, err = r.take(valueLen + 8); err != nil {
		return err
	}
	count, err := r.take(1)
	if err != nil || count[0] > fastRegistrationMaxPathItems {
		return errors.New("invalid fast state path")
	}
	for range int(count[0]) {
		if _, err := r.take(24); err != nil {
			return err
		}
		left, err := r.take(1)
		if err != nil || left[0] != 0 && left[0] != 32 {
			return errors.New("invalid fast state sibling")
		}
		if _, err = r.take(int(left[0])); err != nil {
			return err
		}
		right, err := r.take(1)
		if err != nil || right[0] != 0 && right[0] != 32 || (left[0] == 0) == (right[0] == 0) {
			return errors.New("invalid fast state sibling")
		}
		if _, err = r.take(int(right[0])); err != nil {
			return err
		}
	}
	return nil
}

func (r *fastWireReader) anchor() (fastHeaderRoot, error) {
	part, err := r.take(40)
	if err != nil {
		return fastHeaderRoot{}, err
	}
	header := fastHeaderRoot{Height: binary.BigEndian.Uint64(part[:8]), StateRoot: hex.EncodeToString(part[8:40])}
	for range 2 {
		if err := r.leaf(); err != nil {
			return fastHeaderRoot{}, err
		}
	}
	return header, nil
}

func extractFastCertificateHeaders(certificate []byte) (*fastCertificateHeaders, error) {
	if len(certificate) <= fastContentJournalBytes+fastContentSealBytes+2 || len(certificate) > fastCertificateMaxBytes {
		return nil, errors.New("invalid fast certificate size")
	}
	wire := certificate[fastContentJournalBytes+fastContentSealBytes+2:]
	if len(wire) != int(binary.BigEndian.Uint16(certificate[fastContentJournalBytes+fastContentSealBytes:])) {
		return nil, errors.New("fast certificate witness length disagrees with its prefix")
	}
	r := fastWireReader{wire: wire}
	header, err := r.take(24)
	if err != nil || header[0] != fastRegistrationWireVersion || header[23] == 0 || header[23] > fastRegistrationMaxFlips {
		return nil, errors.New("invalid fast session prefix")
	}
	for range int(header[23]) {
		length, err := r.take(1)
		if err != nil || length[0] == 0 || length[0] > fastRegistrationMaxCIDBytes {
			return nil, errors.New("invalid fast CID length")
		}
		if _, err = r.take(int(length[0])); err != nil {
			return nil, err
		}
	}
	inventory, err := r.anchor()
	if err != nil {
		return nil, err
	}
	verdict, err := r.anchor()
	if err != nil {
		return nil, err
	}
	if r.at != len(wire) || inventory.Height == 0 || verdict.Height <= inventory.Height {
		return nil, errors.New("invalid fast session anchor sequence")
	}
	return &fastCertificateHeaders{Inventory: inventory, Verdict: verdict}, nil
}

func exportFastCertificate(c *client, contract, bundle string) ([]byte, *fastCertificateHeaders, error) {
	listing, err := registryPoolBundle(c, contract, bundle)
	if err != nil {
		return nil, nil, err
	}
	fast, err := registryFastBundle(c, contract, bundle)
	if err != nil {
		return nil, nil, err
	}
	certificate, err := registryFastCertificate(c, contract, bundle)
	if err != nil {
		return nil, nil, err
	}
	if err := validateFastChainCertificate(contract, bundle, listing, fast, certificate, fastContentImageIDHex); err != nil {
		return nil, nil, err
	}
	headers, err := extractFastCertificateHeaders(certificate)
	if err != nil {
		return nil, nil, err
	}
	for _, expected := range []fastHeaderRoot{headers.Inventory, headers.Verdict} {
		var block rpcBlock
		if err := c.call("bcn_blockAt", []any{expected.Height}, &block); err != nil {
			return nil, nil, fmt.Errorf("read canonical fast session header: %w", err)
		}
		if block.Height != expected.Height || !strings.EqualFold(strings.TrimPrefix(block.Root, "0x"), expected.StateRoot) {
			return nil, nil, fmt.Errorf("fast session root differs from canonical header at height %d", expected.Height)
		}
	}
	return certificate, headers, nil
}
