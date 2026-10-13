package main

// The canonical secrets encoding of the pinned construction: the domain, the
// epoch, the author, and the two per-epoch scalars. A registration journal
// carries the commitment to these bytes, and the delivery proof binds the same
// commitment, so this project implements the encoding once per language and
// pins all of them with `proofs/vectors/ecies-v1.json`.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// secretsDomain is the same string the two proof cores carry.
const secretsDomain = "fod:epoch-secrets:v1"

// SecretsCanonicalBytes encodes the secrets a bundle sells.
func SecretsCanonicalBytes(epoch uint16, author, publicPart, privatePart []byte) ([]byte, error) {
	if len(author) != addressBytes {
		return nil, errors.New("the secrets need a 20-byte author address")
	}
	if len(publicPart) != 32 || len(privatePart) != 32 {
		return nil, errors.New("the secrets need two 32-byte scalars")
	}
	out := make([]byte, 0, len(secretsDomain)+2+20+64)
	out = append(out, secretsDomain...)
	out = append(out, byte(epoch>>8), byte(epoch))
	out = append(out, author...)
	out = append(out, publicPart...)
	out = append(out, privatePart...)
	return out, nil
}

// SecretsCommitmentHex is the commitment a registration journal carries and the
// escrow's bundle record is keyed by.
func SecretsCommitmentHex(epoch uint16, author, publicPart, privatePart []byte) (string, error) {
	canonical, err := SecretsCanonicalBytes(epoch, author, publicPart, privatePart)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}
