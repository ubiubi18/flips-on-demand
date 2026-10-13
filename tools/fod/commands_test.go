package main

// The command line rewrites a few friendly flag names before the flag package
// sees them. One of them used to swallow the sweep's own epoch filter.

import (
	"strings"
	"testing"
)

func TestTheEpochAliasLeavesTheSweepsOwnFilterAlone(t *testing.T) {
	swept := translateFlags("sweep", []string{"-from", "100", "-epoch", "204", "-out", "x.json"})
	if strings.Join(swept, " ") != "-from 100 -epoch 204 -out x.json" {
		t.Fatalf("the sweep's epoch filter was rewritten: %v", swept)
	}
	// The modes that name an epoch to work on keep the friendly alias.
	read := translateFlags("inventory", []string{"-epoch", "204", "-author", "0x1"})
	if strings.Join(read, " ") != "-inventory-epoch 204 -author 0x1" {
		t.Fatalf("the inventory epoch alias was dropped: %v", read)
	}
	// The same holds for the `-flag=value` form.
	equal := translateFlags("sweep", []string{"-epoch=204"})
	if strings.Join(equal, " ") != "-epoch=204" {
		t.Fatalf("the sweep's `=` form was rewritten: %v", equal)
	}
}

func TestNormalizeArgsSelectsTheModeAndKeepsTheSweepFilter(t *testing.T) {
	args := normalizeArgs([]string{"sync", "-from", "1", "-to", "2", "-epoch", "204"})
	if args[0] != "-mode=sweep" || strings.Join(args[1:], " ") != "-from 1 -to 2 -epoch 204" {
		t.Fatalf("sync did not normalise to the sweep mode with its filter: %v", args)
	}
}

func TestNormalizeArgsSelectsTheSessionBundleMode(t *testing.T) {
	args := normalizeArgs([]string{"session-bundle", "-datadir", "/tmp/node", "-inventory-height", "10", "-verdict-height", "20", "-author", "0x1"})
	if args[0] != "-mode=session-bundle" {
		t.Fatalf("session-bundle did not normalise to its mode: %v", args)
	}
	if strings.Join(args[1:], " ") != "-datadir /tmp/node -inventory-height 10 -verdict-height 20 -author 0x1" {
		t.Fatalf("session-bundle lost a flag: %v", args)
	}
}

func TestNormalizeArgsSelectsPreSessionWitness(t *testing.T) {
	args := normalizeArgs([]string{"pre-session-witness", "-pre-session-inventory", "private.json", "-out", "witness.json"})
	if strings.Join(args, " ") != "-mode=pre-session-witness -pre-session-inventory private.json -out witness.json" {
		t.Fatalf("pre-session witness command did not preserve its inputs: %v", args)
	}
}
