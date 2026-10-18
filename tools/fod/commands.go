package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// flagWasSet reports whether the caller passed the named flag, which is how a
// mode tells its own default apart from one it inherited from the shared set.
func flagWasSet(name string) bool {
	set := false
	flag.CommandLine.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// notSubmitted says what to do with a prepared call, and names the flag that
// writes it when the caller did not ask for a file: the message must not point
// at a file that was never written.
func notSubmitted(preparedPath, note string) {
	if preparedPath == "" {
		fmt.Println("not submitted: pass -prepared FILE to write this call, then pass that file to `fod submit`")
		return
	}
	if note == "" {
		fmt.Println("not submitted: pass this file to `fod submit` to sign it, and check the values there")
		return
	}
	fmt.Printf("not submitted: pass this file to `fod submit` to sign it; %s\n", note)
}

// Command modes exposes the tool as the `fod` CLI from the spec: the first
// argument selects a command, and flags follow it.
//
// `sync` and `inspect` are the spec's names for the two commands that fetch and
// summarise public data; the technical names remain as aliases.
var commandModes = map[string]string{
	"sync":                      "sweep",
	"sweep":                     "sweep",
	"inspect":                   "inventory",
	"inventory":                 "inventory",
	"decode-answers":            "decode-answers",
	"epoch-db":                  "epoch-db",
	"epoch-keys":                "epoch-keys",
	"state-at":                  "state-at",
	"state-proof":               "state-proof",
	"session-anchor":            "session-anchor",
	"session-bundle":            "session-bundle",
	"session-pool":              "session-pool",
	"seed-at":                   "seed-at",
	"derive-keys":               "derive-keys",
	"export-keys":               "export-keys",
	"import-keys":               "import-keys",
	"receive":                   "receive",
	"market":                    "market",
	"fees":                      "fees",
	"prepare-offer":             "prepare-offer",
	"prepare-registration":      "prepare-registration",
	"prepare-fast-registration": "prepare-fast-registration",
	"pre-session-witness":       "pre-session-witness",
	"export-fast-certificate":   "export-fast-certificate",
	"export-fast-order":         "export-fast-order",
	"prepare-list":              "prepare-list",
	"prepare-buy":               "prepare-buy",
	"prepare-confirm":           "prepare-confirm",
	"prepare-fund":              "prepare-fund",
	"serve":                     "serve",
	"shared-node":               "shared-node",
	"prepare-deploy":            "prepare-deploy",
	"direct-status":             "direct-status",
	"direct-market":             "direct-market",
	"direct-pool-market":        "direct-pool-market",
	"direct-pool-qualify":       "direct-pool-qualify",
	"direct-pool-prove-bundle":  "direct-pool-prove-bundle",
	"direct-v7-prepare":         "direct-v7-prepare",
	"direct-v7-dm":              "direct-v7-dm",
	"direct-v7-receive":         "direct-v7-receive",
	"direct-list":               "direct-list",
	"direct-buy":                "direct-buy",
	"direct-dm":                 "direct-dm",
	"direct-check":              "direct-check",
	"direct-receive":            "direct-receive",
	"direct-recover":            "direct-recover",
	"direct-confirm":            "direct-confirm",
	"direct-reserve-fallback":   "direct-reserve-fallback",
	"direct-start-fallback":     "direct-start-fallback",
	"direct-fallback":           "direct-fallback",
	"direct-refund":             "direct-refund",
	"submit":                    "submit",
	"refund":                    "refund",
	"reveal":                    "reveal",
	"withdraw":                  "withdraw",
	"withdraw-reserve":          "withdraw-reserve",
	"watch":                     "watch",
	"assignment":                "assignment",
	"eligibility":               "eligibility",
	"export-session":            "export",
	"export":                    "export",
}

// normalizeArgs turns `fod <command> [flags]` into the internal `-mode` form so
// that one code path serves both the CLI surface and the earlier flag style.
func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		printUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)
	}
	mode, ok := commandModes[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "fod: unknown command %q\n\n", args[0])
		printUsage()
		os.Exit(2)
	}
	return append([]string{"-mode=" + mode}, translateFlags(mode, args[1:])...)
}

// flagAliases maps the user-facing flag names to the internal ones, so the CLI
// reads naturally without renaming fields throughout the tool.
var flagAliases = map[string]string{
	"-epoch":      "-inventory-epoch",
	"--epoch":     "-inventory-epoch",
	"-height":     "-state-height",
	"--height":    "-state-height",
	"-inventory":  "-sweep",
	"--inventory": "-sweep",
}

// translateFlags rewrites aliases, handling both `-flag value` and `-flag=value`.
//
// The `-epoch` alias belongs to the modes that work on one epoch; `sync` has an
// epoch *filter* of its own under that name, and shadowing it would leave the
// filter unreachable from the command line (`main.go`'s `-epoch` flag).
func translateFlags(mode string, args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		name, value, hasValue := strings.Cut(arg, "=")
		if canonical, ok := flagAliases[name]; ok && !(mode == "sweep" && canonical == "-inventory-epoch") {
			if hasValue {
				out = append(out, canonical+"="+value)
			} else {
				out = append(out, canonical)
			}
			continue
		}
		out = append(out, arg)
	}
	return out
}

// usageText is the CLI's own command list. A test checks it against
// commandModes, so a command cannot be added without appearing here.
func usageText() string {
	var b strings.Builder
	b.WriteString("fod - Flips on Demand local CLI (experimental, unverified)\n\n")
	b.WriteString("usage: fod <command> [flags]\n\n")
	b.WriteString("commands:\n")
	b.WriteString("  sync / sweep   walk a block range and write a sweep of public data\n")
	b.WriteString("  inspect / inventory  build one author-epoch inventory from a sweep (node route) or from an indexer with -indexer (Plan B route)\n")
	b.WriteString("  receive        verify purchased content: -pool-order ID -prepared BUY_FILE -out RECORD_FILE (or decrypt -blob with a vault)\n")
	b.WriteString("  market         write the marketplace state the interface imports, read from the registry\n")
	b.WriteString("  fees           show the measured gas per escrow operation and what it costs at the chain's rate\n")
	b.WriteString("  prepare-buy    prepare a pool purchase; requires independent -contract and -code-hash pins\n")
	b.WriteString("  prepare-confirm  prepare confirmation using -delivery RECORD_FILE from verified receive\n")
	b.WriteString("  prepare-list   prepare the pool's sellUnit or donateUnit call from a sell or give request\n")
	b.WriteString("  prepare-fund   add iDNA to a pool bundle's reserve (the funding-profile step)\n")
	b.WriteString("  serve          watch the pool, deliver the buyer-bound ciphertext, keep the beacon fresh, and settle a missing confirmation (proof, or the committed secrets near the deadline)\n")
	b.WriteString("  prepare-offer  build a bundle's registration input from a capture, an inventory, and the author's nodekey\n")
	b.WriteString("  prepare-registration  turn a sealed registration receipt into the pool's registerBundle call\n")
	b.WriteString("  prepare-fast-registration  combine a sealed flip-content proof with a canonical two-anchor session bundle\n")
	b.WriteString("  pre-session-witness  write an owner-only content witness from submitted flips before validation\n")
	b.WriteString("  export-fast-certificate  save a registered flip proof and its canonical header roots for independent verification\n")
	b.WriteString("  export-fast-order  save a funded order's canonical context for the seller's local point delivery\n")
	b.WriteString("  prepare-deploy    build and sign the deploy of the release module, and print the address it lands on\n")
	b.WriteString("  direct-status  read a direct order and its refund height\n")
	b.WriteString("  direct-market  export public direct offers and orders from the node\n")
	b.WriteString("  direct-pool-market  export the v6 liquidity pool and queue from the node (-direct-version 6)\n")
	b.WriteString("  direct-pool-qualify  prepare and estimate v6 qualify from a sealed proof and canonical session (-direct-version 6)\n")
	b.WriteString("  direct-pool-prove-bundle  prepare and estimate v6 proveBundle for the same author and offer (-direct-version 6)\n")
	b.WriteString("  direct-v7-prepare  prepare and estimate one typed v7 pool operation (-direct-version 7 -operation NAME)\n")
	b.WriteString("  direct-v7-dm       prepare private ciphertext for one funded v7 order without sending it\n")
	b.WriteString("  direct-v7-receive  verify funded v7 ciphertext and original flips, then save a buyer receipt and vault\n")
	b.WriteString("  direct-list    prepare a canonical original-flip sale\n")
	b.WriteString("  direct-buy     retain original flips and prepare a funded purchase\n")
	b.WriteString("  direct-dm      export encrypted delivery text for a private message\n")
	b.WriteString("  direct-check   verify a private delivery and write a display-only result without a vault or transaction\n")
	b.WriteString("  direct-receive verify private delivery; -auto-confirm opts in to immediate settlement\n")
	b.WriteString("  direct-recover verify a settled public reveal against a retained direct buy and recover its vault\n")
	b.WriteString("  direct-confirm prepare buyer confirmation from a verified receipt\n")
	b.WriteString("  direct-reserve-fallback prepare the deployed v3 seller intent only\n")
	b.WriteString("  direct-start-fallback verify a sealed proof and prepare the seller stake; v4/v5 need -direct-version, and v5 needs -session\n")
	b.WriteString("  direct-fallback prepare public key disclosure after a proof-backed fallback start\n")
	b.WriteString("  direct-refund  prepare a refund when the active fallback window expires\n")
	b.WriteString("  submit         sign a prepared action; buys require independent -contract and -code-hash pins; broadcast only with -broadcast\n")
	b.WriteString("  refund         turn a pool refund request into the escrow's refund call\n")
	b.WriteString("  reveal         prepare the escrow's reveal call: publish the vault's committed secrets to settle an order without the buyer\n")
	b.WriteString("  withdraw       turn a withdrawal request into the escrow's withdraw call\n")
	b.WriteString("  withdraw-reserve  turn a reserve-withdrawal request into the escrow's withdrawReserve call\n")
	b.WriteString("  watch          follow one pool order and submit the permissionless refund when its deadline passes\n")
	b.WriteString("  export / export-session  write a session-content export from an inventory, a vault, and saved blobs\n")
	b.WriteString("  shared-node    run a hosted shared node for one author: export their flips, build the session pool, and write the connection file the author imports\n")
	b.WriteString("  derive-keys    derive an author's epoch keys (public summary only)\n")
	b.WriteString("  export-keys    write derived epoch secrets into an encrypted vault\n")
	b.WriteString("  import-keys    read a vault and print its public summary\n")
	b.WriteString("  seed-at        read a block seed from a node datadir (read-only)\n")
	b.WriteString("  epoch-keys     list a datadir's epoch-prefixed keys\n")
	b.WriteString("  state-at       read historical identity state (fails when pruned)\n")
	b.WriteString("  state-proof    read one identity with its state-tree proof and verify it; -proof-bundle writes a portable bundle (read-only)\n")
	b.WriteString("  session-anchor preserve one public state anchor for an author before node pruning (read-only)\n")
	b.WriteString("  session-bundle write the two-anchor session bundle from a datadir or two saved anchors (read-only)\n")
	b.WriteString("  session-pool   turn a session-content export into the flip pool a provider's gateway loads\n")
	b.WriteString("  epoch-db       read lottery records from a datadir (read-only)\n")
	b.WriteString("  assignment     derive the epoch's lottery assignment from a captured datadir\n")
	b.WriteString("  eligibility    reconstruct one author's flips from a capture and decide the bundle\n")
	b.WriteString("  decode-answers decode a long-answer transaction\n\n")
	b.WriteString("run `fod <command> -h` for that command's flags\n")
	return b.String()
}

func printUsage() {
	fmt.Fprint(os.Stderr, usageText())
}
