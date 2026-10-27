package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/golang/protobuf/proto"
	models "github.com/idena-network/idena-go/protobuf"
)

// bondedV9SessionVerdict describes the public session facts that the v9
// listing checks. Clean is false for any adverse validation status flag.
type bondedV9SessionVerdict struct {
	Clean bool
	Flags uint32
	Flips uint16
}

// readBondedV9SessionVerdict verifies the saved state leaves and reads the
// post-validation identity's raw flags. The caller must separately match both
// anchor roots to canonical headers before treating Clean as listing-ready.
func readBondedV9SessionVerdict(session *SessionBundle) (bondedV9SessionVerdict, error) {
	var result bondedV9SessionVerdict
	if session == nil || session.Format != sessionBundleFormat || session.Version != sessionBundleVersion {
		return result, errors.New("invalid session bundle")
	}
	verified, err := sessionBundleFromAnchors(session.Author, &session.Inventory, &session.Verdict)
	if err != nil {
		return result, fmt.Errorf("verify session leaves: %w", err)
	}
	if session.Epoch != verified.Epoch || !slices.Equal(session.Manifest, verified.Manifest) {
		return result, errors.New("session bundle differs from its verified state leaves")
	}
	if len(verified.Manifest) == 0 || len(verified.Manifest) > fastRegistrationMaxFlips {
		return result, fmt.Errorf("v9 listing needs 1 to %d flips", fastRegistrationMaxFlips)
	}
	// This applies the same wire bounds and duplicate-CID refusal as the
	// witness passed to the on-chain session verifier.
	if _, err := encodeFastRegistrationWire(verified); err != nil {
		return result, fmt.Errorf("encode v9 session witness: %w", err)
	}
	value, err := hex.DecodeString(verified.Verdict.Identity.ValueHex)
	if err != nil {
		return result, fmt.Errorf("decode verdict identity: %w", err)
	}
	var identity models.ProtoStateIdentity
	if err := proto.Unmarshal(value, &identity); err != nil {
		return result, fmt.Errorf("read verdict identity: %w", err)
	}
	// The fast-session verifier rejects flags outside these three chain bits.
	if identity.ValidationStatus&^uint32(0b111) != 0 {
		return result, errors.New("verdict identity has unsupported validation flags")
	}
	return bondedV9SessionVerdict{
		Clean: identity.ValidationStatus == 0,
		Flags: identity.ValidationStatus,
		Flips: uint16(len(verified.Manifest)),
	}, nil
}
