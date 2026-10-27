package main

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/core/state"
)

func TestBondedV9SessionVerdictUsesAnchoredFlagsAndExactFlipCount(t *testing.T) {
	addr := sessionAddress(0x71)
	author := hex.EncodeToString(addr[:])
	for _, tc := range []struct {
		name      string
		flags     state.ValidationStatusFlag
		wantClean bool
		wantError bool
	}{
		{"clean", 0, true, false},
		{"reported", state.AtLeastOneFlipReported, false, false},
		{"not qualified", state.AtLeastOneFlipNotQualified, false, false},
		{"all not qualified", state.AllFlipsNotQualified, false, false},
		{"unsupported flag", state.ValidationStatusFlag(8), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeSessionDatadir(t, addr, sessionCIDs(3), tc.flags, true, fixtureEpoch+1)
			session, err := readSessionBundle(dir, sessionInventoryHeight, sessionVerdictHeight, author)
			if err != nil {
				t.Fatalf("read session: %v", err)
			}
			got, err := readBondedV9SessionVerdict(session)
			if (err != nil) != tc.wantError {
				t.Fatalf("verdict error = %v, wantError = %t", err, tc.wantError)
			}
			if tc.wantError {
				return
			}
			if got.Clean != tc.wantClean || got.Flags != uint32(tc.flags) || got.Flips != 3 {
				t.Fatalf("verdict = %+v, want clean=%t flags=%d flips=3", got, tc.wantClean, tc.flags)
			}
		})
	}
}

func TestBondedV9SessionVerdictRejectsInvalidEvidenceAndCount(t *testing.T) {
	addr := sessionAddress(0x81)
	author := hex.EncodeToString(addr[:])
	uniqueCIDs := func(count int) [][]byte {
		cids := make([][]byte, count)
		for i := range cids {
			cids[i] = append([]byte{0x01, 0x55, 0x12, 0x20}, bytes.Repeat([]byte{byte(i + 1)}, 32)...)
		}
		return cids
	}
	read := func(count int) *SessionBundle {
		t.Helper()
		dir := writeSessionDatadir(t, addr, uniqueCIDs(count), 0, true, fixtureEpoch+1)
		bundle, err := readSessionBundle(dir, sessionInventoryHeight, sessionVerdictHeight, author)
		if err != nil {
			t.Fatalf("read %d-flip session: %v", count, err)
		}
		return bundle
	}
	if got, err := readBondedV9SessionVerdict(read(32)); err != nil || !got.Clean || got.Flips != 32 {
		t.Fatalf("32-flip verdict = %+v, %v", got, err)
	}
	if _, err := readBondedV9SessionVerdict(read(33)); err == nil {
		t.Fatal("33-flip session was accepted")
	}
	changed := read(2)
	changed.Manifest[0] = changed.Manifest[1]
	if _, err := readBondedV9SessionVerdict(changed); err == nil {
		t.Fatal("a manifest differing from the state leaf was accepted")
	}
	changed = read(2)
	changed.Verdict.RootHex = strings.Repeat("0", 64)
	if _, err := readBondedV9SessionVerdict(changed); err == nil {
		t.Fatal("a verdict leaf with a false root was accepted")
	}
}
