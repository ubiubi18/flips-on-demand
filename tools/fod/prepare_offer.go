package main

// The seller's first command of `SPEC.md` 6.1: derive the keys, validate the
// content, and produce the registration proof for one bundle.
//
// Everything it needs now exists as a checked piece: the capture holds the seed,
// the candidate list in the ceremony's order, and the answers; the assignment and
// the answer join turn those into the per-flip committees; `fod inspect`'s
// inventory says which CIDs the chain actually records for that author and epoch,
// with the blobs fetched and hashed back; the nodekey derives the epoch's two
// secrets; and `proofs/registration/host` proves the input this command writes.
// What was missing was the command that puts them in order, which is this one.

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/idena-network/idena-go/crypto"
)

// The input file `proofs/registration/host` reads, in its own shape. Answers and
// grades are numbers rather than bytes, because Go writes a byte slice as base64
// and the host reads a list of integers.
type registrationInput struct {
	Statement         string                      `json:"statement"`
	Note              string                      `json:"note,omitempty"`
	Contract          string                      `json:"contract"`
	Network           string                      `json:"network"`
	Author            string                      `json:"author"`
	Epoch             uint16                      `json:"epoch"`
	ValidationAnchor  string                      `json:"validationAnchor"`
	ConsensusVersion  string                      `json:"consensusVersion"`
	EligibilityPolicy string                      `json:"eligibilityPolicy"`
	Cids              []string                    `json:"cids"`
	Inventory         []registrationInventoryItem `json:"inventory"`
	PublicPartSecret  string                      `json:"publicPartSecret"`
	PrivatePartSecret string                      `json:"privatePartSecret"`
	Candidates        []registrationCandidate     `json:"candidates"`
	Covered           []uint32                    `json:"covered"`
	// ProofAnchor is the identity-proof bundle the guest folds to the anchor
	// root it binds in the claim. It is the session binding of SPEC.md 5.5.
	ProofAnchor *ProofBundle `json:"proofAnchor,omitempty"`
}

type registrationInventoryItem struct {
	Flip uint32 `json:"flip"`
	Cid  string `json:"cid"`
	Blob string `json:"blob"`
}

type registrationCandidate struct {
	Assignment []uint32 `json:"assignment"`
	Answers    []int    `json:"answers"`
	Grades     []int    `json:"grades"`
}

// registrationInputStatement is the marker the host's dump writes and this
// command writes; a mismatch means the file is not one of these.
const registrationInputStatement = "fod:registration-input:v1"

// validateRegistrationInput refuses what the host would refuse, before a proof is
// started. It mirrors `proofs/registration/host/src/main.rs` check by check.
func validateRegistrationInput(input *registrationInput) error {
	if input.Statement != registrationInputStatement {
		return fmt.Errorf("the input carries statement %q, expected %q", input.Statement, registrationInputStatement)
	}
	for name, value := range map[string]string{
		"contract": input.Contract,
		"author":   input.Author,
	} {
		raw, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(value), "0x"))
		if err != nil || len(raw) != 20 {
			return fmt.Errorf("the input's %s is not a 20-byte address", name)
		}
	}
	if anchor, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(input.ValidationAnchor), "0x")); err != nil || len(anchor) != 32 {
		return errors.New("the input's validation anchor is not a 32-byte digest")
	}
	for name, secret := range map[string]string{
		"public part secret":  input.PublicPartSecret,
		"private part secret": input.PrivatePartSecret,
	} {
		if raw, err := hex.DecodeString(secret); err != nil || len(raw) != 32 {
			return fmt.Errorf("the input's %s is not 32 bytes", name)
		}
	}
	if input.Network == "" || input.ConsensusVersion == "" || input.EligibilityPolicy == "" {
		return errors.New("the input needs a network, a consensus version, and an eligibility policy")
	}
	if len(input.Cids) == 0 || len(input.Inventory) == 0 || len(input.Covered) == 0 {
		return errors.New("the input needs a manifest, an inventory, and covered flips")
	}
	manifest := append([]string{}, input.Cids...)
	inventory := make([]string, 0, len(input.Inventory))
	flips := map[uint32]bool{}
	for _, entry := range input.Inventory {
		inventory = append(inventory, entry.Cid)
		flips[entry.Flip] = true
		if len(entry.Blob) == 0 {
			return fmt.Errorf("the inventory entry for %s carries no blob", entry.Cid)
		}
	}
	sort.Strings(manifest)
	sort.Strings(inventory)
	if strings.Join(manifest, "\x00") != strings.Join(inventory, "\x00") {
		return errors.New("the manifest and the inventory name different CIDs")
	}
	covered := map[uint32]bool{}
	for _, flip := range input.Covered {
		if !flips[flip] {
			return fmt.Errorf("covered flip %d is not in the inventory", flip)
		}
		if covered[flip] {
			return fmt.Errorf("covered flip %d appears twice", flip)
		}
		covered[flip] = true
	}
	for index, candidate := range input.Candidates {
		if len(candidate.Answers) != len(candidate.Assignment) || len(candidate.Grades) != len(candidate.Assignment) {
			return fmt.Errorf("candidate %d has %d assignments, %d answers, and %d grades",
				index, len(candidate.Assignment), len(candidate.Answers), len(candidate.Grades))
		}
	}
	if err := validateProofAnchor(input.ProofAnchor); err != nil {
		return err
	}
	if err := proofAnchorMatchesAnchor(input.ProofAnchor, input.ValidationAnchor); err != nil {
		return err
	}
	return nil
}

// proofAnchorMatchesAnchor is the rule that makes the bundle useful: it has to
// prove the root the manifest anchors, or it is evidence about some other state
// and says nothing about this epoch.
func proofAnchorMatchesAnchor(bundle *ProofBundle, anchor string) error {
	if bundle == nil {
		return nil
	}
	got := strings.TrimPrefix(strings.ToLower(bundle.RootHex), "0x")
	want := strings.TrimPrefix(strings.ToLower(anchor), "0x")
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("the proof anchor's root %s is not the manifest's validation anchor %s", got, want)
	}
	return nil
}

// inventoryCidsFor checks the inventory against the capture and returns the
// author's CIDs in the order the inventory records them.
//
// Two independent sources describe the same author-epoch: the chain's
// transactions (the inventory) and the ceremony's own candidate list (the
// capture). They have to agree, and a disagreement is refused rather than
// resolved: the manifest commits to a set, and a wrong set cannot be corrected
// after the proof exists.
func inventoryCidsFor(inventory *BundleInventory, author string, epoch uint16, captured []string) ([]string, error) {
	if !strings.EqualFold(strings.TrimPrefix(inventory.Author, "0x"), strings.TrimPrefix(author, "0x")) {
		return nil, fmt.Errorf("the inventory is for author %s, not %s", inventory.Author, author)
	}
	if inventory.Epoch != epoch {
		return nil, fmt.Errorf("the inventory is for epoch %d, not %d", inventory.Epoch, epoch)
	}
	// A listing the Plan B route wrote is allowed to be useful and is not allowed
	// to become a manifest: completeness rests on a committed-body sweep, which
	// that route does not perform (`SPEC.md` 5.3).
	if inventory.Source != "" && inventory.Source != "node sweep" {
		if !inventory.NodeVerified {
			return nil, fmt.Errorf(
				"the inventory came from %q and no node confirmed its anchors: run inspect with -sweep against a node, or with -indexer and a node that can check the anchors",
				inventory.Source)
		}
		return nil, fmt.Errorf(
			"the inventory came from %q, and a manifest's inventory has to come from a committed-body sweep (-sweep): only that route shows every submission the chain recorded for the author and epoch",
			inventory.Source)
	}
	if !inventory.Complete {
		return nil, errors.New("the inventory is not complete, and a manifest may not commit to an incomplete one")
	}
	fromInventory := []string{}
	for _, entry := range inventory.Entries {
		if entry.Deleted {
			continue
		}
		if entry.Error != "" || !entry.BlobCIDOk {
			return nil, fmt.Errorf("the inventory entry %s is not verified: %s", entry.CID, entry.Error)
		}
		fromInventory = append(fromInventory, entry.CID)
	}
	sortedInventory := append([]string{}, fromInventory...)
	sortedCapture := append([]string{}, captured...)
	sort.Strings(sortedInventory)
	sort.Strings(sortedCapture)
	if strings.Join(sortedInventory, "\x00") != strings.Join(sortedCapture, "\x00") {
		return nil, fmt.Errorf(
			"the capture and the inventory disagree about %s's flips in epoch %d: %d recorded, %d captured",
			author, epoch, len(sortedInventory), len(sortedCapture))
	}
	return fromInventory, nil
}

// anchorFor resolves the manifest's validation anchor.
//
// F-67 left the anchor's source open, and this is the choice: the state root of
// the block at which the epoch's validation began. It is a chain commitment, so a
// seller can state it, a node holder can read it from the header at that height,
// and the explorer publishes the same value - which is why the two are compared
// when both are available rather than either being trusted alone.
func anchorFor(explicit string, indexer string, epoch uint16, node *client) (string, uint64, error) {
	if explicit != "" {
		if raw, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(explicit), "0x")); err != nil || len(raw) != 32 {
			return "", 0, errors.New("the anchor is not a 32-byte digest")
		}
		return strings.TrimPrefix(strings.ToLower(explicit), "0x"), 0, nil
	}
	if indexer == "" {
		return "", 0, errors.New("the manifest needs a validation anchor: pass -anchor, or -indexer to read the epoch's state root")
	}
	explorer := newHTTPClient()
	root, err := fetchIndexerStateRoot(explorer, indexer, epoch)
	if err != nil {
		return "", 0, fmt.Errorf("read the epoch's state root: %w", err)
	}
	if raw, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(root), "0x")); err != nil || len(raw) != 32 {
		return "", 0, fmt.Errorf("the indexer's state root %q is not a 32-byte digest", root)
	}
	// The root belongs to the epoch's opening block, not to the first block of
	// its validation (`SOURCES.md` F-82), so the height that travels with it is
	// that block's.
	height, err := fetchIndexerEpochOpeningBlock(explorer, indexer, epoch)
	if err != nil {
		if node == nil {
			return strings.TrimPrefix(strings.ToLower(root), "0x"), 0, nil
		}
		return "", 0, fmt.Errorf("resolve the block the epoch's state root belongs to: %w", err)
	}
	if node == nil {
		return strings.TrimPrefix(strings.ToLower(root), "0x"), height, nil
	}
	// With a node, the explorer's root is checked rather than believed.
	var header rpcBlock
	if err := node.call("bcn_blockAt", []any{height}, &header); err != nil {
		return "", 0, fmt.Errorf("read the header at block %d: %w", height, err)
	}
	if !strings.EqualFold(header.Root, root) {
		return "", 0, fmt.Errorf(
			"the indexer's state root %s is not the root of block %d (%s): the anchor is refused",
			root, height, header.Root)
	}
	return strings.TrimPrefix(strings.ToLower(root), "0x"), height, nil
}

// runRegistrationProver calls the host with the input file and waits. The host
// verifies the receipt it produces before it writes anything, so a return here
// means a receipt that verified against the guest the contract pins.
func runRegistrationProver(proverPath, inputPath, proofPath string) (string, error) {
	cmd := exec.Command(proverPath, "--input", inputPath, "--out", proofPath, "--prove")
	var log bytes.Buffer
	cmd.Stdout = &log
	cmd.Stderr = &log
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("the prover failed: %w\n%s", err, tailLines(log.String(), 8))
	}
	return log.String(), nil
}

func runPrepareOffer(dataDir string, epoch uint16, author, nodeKeyPath, inventoryPath, blobsDir, contract, network, policy, anchor, indexer, proofAnchorPath, inputPath, proverPath, proofPath, sweepPath string, node *client) {
	if dataDir == "" || epoch == 0 || author == "" || nodeKeyPath == "" || inventoryPath == "" || blobsDir == "" {
		fmt.Fprintln(os.Stderr, "prepare-offer needs -datadir PATH -inventory-epoch N -author ADDRESS -nodekey FILE -inventory FILE -blobs DIR")
		os.Exit(2)
	}
	if contract == "" || network == "" {
		fmt.Fprintln(os.Stderr, "prepare-offer needs -contract ADDRESS and -network LABEL: the manifest commits to both")
		os.Exit(2)
	}
	if proofAnchorPath == "" {
		fmt.Fprintln(os.Stderr, "prepare-offer needs -proof-anchor FILE: a bundle is sellable only with the identity proofs that fold to its anchor (SPEC.md 5.5)")
		os.Exit(2)
	}
	if inputPath == "" {
		inputPath = "registration-input.json"
	}

	// The same precondition `fod eligibility` applies, because this command is
	// what turns a decision into a proof: an epoch the chain has not finished
	// cannot have produced the completed validation `SPEC.md` 5.2 requires.
	ended, err := epochCompletionCheck(node, epoch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "action refused: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("epoch       %s\n", ended.Note)

	raw, err := os.ReadFile(inventoryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read the inventory: %v\n", err)
		os.Exit(1)
	}
	var inventory BundleInventory
	if err := decodeStrict(raw, &inventory); err != nil {
		fmt.Fprintf(os.Stderr, "parse the inventory: %v\n", err)
		os.Exit(1)
	}

	report, err := buildEligibilityReport(dataDir, epoch, author, policy, sweepPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "eligibility failed: %v\n", err)
		os.Exit(1)
	}
	if report.Answers != nil {
		fmt.Printf("answers     %s\n", report.Answers.Note)
	}
	captured := make([]string, 0, len(report.Covered))
	for _, flip := range report.Covered {
		captured = append(captured, flip.Cid)
	}
	cids, err := inventoryCidsFor(&inventory, author, epoch, captured)
	if err != nil {
		fmt.Fprintf(os.Stderr, "action refused: %v\n", err)
		os.Exit(1)
	}

	resolvedAnchor, height, err := anchorFor(anchor, indexer, epoch, node)
	if err != nil {
		fmt.Fprintf(os.Stderr, "action refused: %v\n", err)
		os.Exit(1)
	}

	var proofAnchor *ProofBundle
	if proofAnchorPath != "" {
		proofAnchor, err = readProofBundleFile(proofAnchorPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "action refused: %v\n", err)
			os.Exit(1)
		}
	}

	identity, err := crypto.LoadECDSA(nodeKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load nodekey: %v\n", err)
		os.Exit(1)
	}
	keys, err := DeriveEpochKeys(identity, epoch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "derive epoch keys: %v\n", err)
		os.Exit(1)
	}
	publicPart, privatePart := keys.SecretBytes()

	input, err := assembleRegistrationInput(assembleParts{
		Contract:      contract,
		Network:       network,
		Author:        author,
		Epoch:         epoch,
		Anchor:        resolvedAnchor,
		ProofAnchor:   proofAnchor,
		Consensus:     ConsensusVersionPinned,
		Policy:        policy,
		Cids:          cids,
		Report:        report,
		BlobsDir:      blobsDir,
		PublicSecret:  publicPart,
		PrivateSecret: privatePart,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "action refused: %v\n", err)
		os.Exit(1)
	}
	if err := validateRegistrationInput(input); err != nil {
		fmt.Fprintf(os.Stderr, "action refused: %v\n", err)
		os.Exit(1)
	}
	if err := writeRegistrationInput(inputPath, input); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", inputPath, err)
		os.Exit(1)
	}

	fmt.Printf("operation   registerBundle on %s\n", input.Network)
	fmt.Printf("contract    %s\n", input.Contract)
	fmt.Printf("author      %s, epoch %d, shard %d\n", input.Author, input.Epoch, report.Shard)
	fmt.Printf("flips       %d covered of %d in the manifest\n", len(input.Covered), len(input.Cids))
	fmt.Printf("anchor      %s", resolvedAnchor)
	if height > 0 {
		fmt.Printf(" (state root of the epoch's opening block %d)", height)
	}
	fmt.Println()
	fmt.Printf("input       %s\n", inputPath)

	if proverPath == "" {
		fmt.Println("not proved: pass -prover to run the registration prover on this input")
		return
	}
	if proofPath == "" {
		proofPath = strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".registration.json"
	}
	fmt.Printf("proving     %s (about fifteen minutes)\n", proverPath)
	log, err := runRegistrationProver(proverPath, inputPath, proofPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	for _, line := range strings.Split(strings.TrimRight(log, "\n"), "\n") {
		if strings.HasPrefix(line, "journal") || strings.HasPrefix(line, "cycles") || strings.HasPrefix(line, "proving") {
			fmt.Println(line)
		}
	}
	fmt.Printf("proof       %s\n", proofPath)
	fmt.Println("next: fod prepare-registration -proof <that file> -contract <registry> to turn it into the call")
}

// writeRegistrationInput writes the input the registration host consumes. It
// carries the epoch secrets the guest commits to, so it is key material: owner
// only, and tightened even when the file already existed with wider permissions.
func writeRegistrationInput(path string, input *registrationInput) error {
	return writePrivateJSON(path, input)
}

// assembleParts is what the assembly needs, so it can be exercised without a
// datadir.
type assembleParts struct {
	Contract, Network, Author string
	Epoch                     uint16
	Anchor, Consensus, Policy string
	ProofAnchor               *ProofBundle
	Cids                      []string
	Report                    *EligibilityReport
	BlobsDir                  string
	PublicSecret              []byte
	PrivateSecret             []byte
}

func assembleRegistrationInput(parts assembleParts) (*registrationInput, error) {
	blobs := map[string]bool{}
	flipOf := map[string]uint32{}
	for _, flip := range parts.Report.Covered {
		flipOf[flip.Cid] = uint32(flip.Index)
	}
	inventory := make([]registrationInventoryItem, 0, len(parts.Cids))
	covered := make([]uint32, 0, len(parts.Report.Covered))
	cids := append([]string{}, parts.Cids...)
	sort.Strings(cids)
	for _, cid := range cids {
		blobPath := filepath.Join(parts.BlobsDir, cid)
		body, err := os.ReadFile(blobPath)
		if err != nil {
			return nil, fmt.Errorf("read the blob for %s: %w", cid, err)
		}
		if len(body) == 0 {
			return nil, fmt.Errorf("the blob for %s is empty", cid)
		}
		flip, ok := flipOf[cid]
		if !ok {
			return nil, fmt.Errorf("the manifest holds %s, which is not one of the author's captured flips", cid)
		}
		// The guest checks every blob against its CID before it proves anything,
		// and a proof run costs about fifteen minutes: refuse here instead, with
		// the same rule the guest applies (`tools/fod/cid.go`).
		if verified, note := verifyBlobCID(cid, body, nil); !verified {
			return nil, fmt.Errorf("the blob for %s is not the one its cid commits to: %s", cid, note)
		}
		blobs[cid] = true
		inventory = append(inventory, registrationInventoryItem{
			Flip: flip,
			Cid:  cid,
			Blob: hex.EncodeToString(body),
		})
		covered = append(covered, flip)
	}
	sort.Slice(covered, func(i, j int) bool { return covered[i] < covered[j] })
	candidates, err := witnessFor(parts.Report)
	if err != nil {
		return nil, err
	}
	return &registrationInput{
		Statement:         registrationInputStatement,
		Note:              "assembled by fod prepare-offer from a capture, an inventory, and the author's nodekey",
		Contract:          strings.TrimPrefix(strings.ToLower(parts.Contract), "0x"),
		Network:           parts.Network,
		Author:            strings.TrimPrefix(strings.ToLower(parts.Author), "0x"),
		Epoch:             parts.Epoch,
		ValidationAnchor:  parts.Anchor,
		ConsensusVersion:  parts.Consensus,
		EligibilityPolicy: parts.Policy,
		Cids:              cids,
		Inventory:         inventory,
		PublicPartSecret:  hex.EncodeToString(parts.PublicSecret),
		PrivatePartSecret: hex.EncodeToString(parts.PrivateSecret),
		Candidates:        candidates,
		Covered:           covered,
		ProofAnchor:       parts.ProofAnchor,
	}, nil
}

// witnessFor is the committee evidence for the covered flips: every candidate of
// the author's shard with the flips it was assigned, the answers it gave, and the
// grades it reported. It comes from the same capture the decision did, so the
// proof proves the arithmetic the seller was shown rather than a second reading
// of the epoch.
func witnessFor(report *EligibilityReport) ([]registrationCandidate, error) {
	if report == nil || report.witness == nil {
		return nil, errors.New("the report carries no witness: it was not built from a captured epoch")
	}
	return report.witness, nil
}
