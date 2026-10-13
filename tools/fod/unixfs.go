package main

// The importer the pinned node uses for every flip blob, reproduced so a caller
// with no node can still decide a UnixFS/dag-pb CID (`SOURCES.md` F-86, F-88).
//
// The node imports a file through go-unixfs's defaults with `CidVersion(1)` and
// sets neither a chunker nor raw-leaves (`ipfs/ipfs.go`, `Cid` and `Add`), so:
// a file that fits in one 262,144-byte chunk is a single raw leaf; anything
// larger is a dag-pb root whose links are raw leaves, carrying the UnixFS file
// metadata (type, filesize, blocksizes). Idena refuses a flip over
// `common.MaxFlipSize` = 614,400 bytes before it is stored, so a flip's DAG is
// one root with at most three leaves and never needs a second level.
//
// Only the raw-leaves builder is reproduced. A CID built by a client that
// disabled raw leaves is refused rather than guessed at, because guessing would
// mean accepting bytes on a layout nobody can state.

import (
	"crypto/sha256"
	"errors"
	"fmt"
)

const (
	// unixfsChunkSize is go-unixfs's default chunk size, which is also where the
	// raw and dag-pb forms split (`SOURCES.md` F-86).
	unixfsChunkSize = 262144
	// unixfsMaxLinks is go-unixfs's default fan-out: a balanced root holds this
	// many links before another level appears.
	unixfsMaxLinks = 174
	// unixfsFileType is the UnixFS DataType for a file.
	unixfsFileType = 2
)

// unixfsCID builds the CID the pinned node's importer produces for these bytes.
func unixfsCID(blob []byte) (string, error) {
	if len(blob) == 0 {
		return "", errors.New("the blob is empty")
	}
	if len(blob) <= unixfsChunkSize {
		return cidRawSha256(blob), nil
	}
	if chunks := (len(blob) + unixfsChunkSize - 1) / unixfsChunkSize; chunks > unixfsMaxLinks {
		return "", fmt.Errorf("the blob needs %d leaves, more than one dag-pb root holds (%d)", chunks, unixfsMaxLinks)
	}

	// PBNode carries its links (field 2) before its data (field 1): the order the
	// generated marshaler the pinned node builds every dag-pb block with writes.
	node := make([]byte, 0, 64+unixfsMaxLinks*48)
	sizes := make([]int, 0, len(blob)/unixfsChunkSize+1)
	for offset := 0; offset < len(blob); offset += unixfsChunkSize {
		end := offset + unixfsChunkSize
		if end > len(blob) {
			end = len(blob)
		}
		chunk := blob[offset:end]
		link := pbLink(rawCIDBytes(chunk), len(chunk))
		node = append(node, 0x12) // PBNode.Links, field 2, length-delimited
		node = append(node, pbVarint(len(link))...)
		node = append(node, link...)
		sizes = append(sizes, len(chunk))
	}
	data := unixfsFileData(len(blob), sizes)
	node = append(node, 0x0a) // PBNode.Data, field 1, length-delimited
	node = append(node, pbVarint(len(data))...)
	node = append(node, data...)
	return cidDagPBSha256(node), nil
}

// pbLink is one PBLink: the leaf's CID (field 1), its name (field 2), and the
// size of the subtree it commits to (field 3).
//
// A file link's name is empty, but the field is present all the same: the
// builder that produces these blocks always sets it, and an absent field would
// serialize to different bytes and therefore a different CID.
func pbLink(cid []byte, tsize int) []byte {
	link := make([]byte, 0, len(cid)+10)
	link = append(link, 0x0a)
	link = append(link, pbVarint(len(cid))...)
	link = append(link, cid...)
	link = append(link, 0x12, 0x00)
	link = append(link, 0x18)
	link = append(link, pbVarint(tsize)...)
	return link
}

// unixfsFileData is the UnixFS Data message of a file node: the type (field 1),
// the total size (field 3), and the size of every block (field 4).
func unixfsFileData(filesize int, sizes []int) []byte {
	data := []byte{0x08, unixfsFileType}
	data = append(data, 0x18)
	data = append(data, pbVarint(filesize)...)
	for _, size := range sizes {
		data = append(data, 0x20)
		data = append(data, pbVarint(size)...)
	}
	return data
}

// rawCIDBytes is the CIDv1/raw/sha2-256 value in its binary form, which is what
// a PBLink carries rather than its text form.
func rawCIDBytes(data []byte) []byte {
	digest := sha256.Sum256(data)
	return append([]byte{0x01, 0x55, 0x12, 0x20}, digest[:]...)
}

// cidDagPBSha256 renders the CIDv1/dag-pb/sha2-256 value of a serialized dag-pb
// block.
func cidDagPBSha256(block []byte) string {
	digest := sha256.Sum256(block)
	prefix := []byte{0x01, 0x70, 0x12, 0x20}
	return cidString(append(prefix, digest[:]...))
}

// pbVarint encodes an unsigned protobuf varint.
func pbVarint(value int) []byte {
	out := make([]byte, 0, 4)
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}
