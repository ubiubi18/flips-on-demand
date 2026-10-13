package main

// The middle of Package 2's eligibility half, as one command: a capture plus the
// epoch's stored answers become a verdict for every flip of one author, and then
// the policy's decision for the bundle that author would sell.
//
// The pieces exist and are checked - `assignmentFor` derives the lottery,
// `ReconstructFlipVerdicts` places the answers, `CheckBundleEligibility` applies
// `SPEC.md` 5.1 - and this is what runs them in order on a captured epoch. The
// seller reads the decision, and the covered flip list it carries is the same
// list the registration input needs.

import (
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/idena-network/idena-go/common"
)

// CoveredFlip is one flip of the author's own, with what the reconstruction
// decided about it.
type CoveredFlip struct {
	Index  int    `json:"index"`
	Cid    string `json:"cid"`
	Status string `json:"status"`
	Grade  int    `json:"grade"`
	Answer string `json:"answer"`
}

// EligibilityReport is the seller's evidence for one bundle.
type EligibilityReport struct {
	Statement  string             `json:"statement"`
	Epoch      uint16             `json:"epoch"`
	Author     string             `json:"author"`
	Shard      uint8              `json:"shard"`
	Policy     string             `json:"policy"`
	Rules      map[string]bool    `json:"rules"`
	Candidates int                `json:"candidates"`
	Answered   int                `json:"answered"`
	Covered    []CoveredFlip      `json:"covered"`
	Decision   EligibilityVerdict `json:"decision"`
	Indexer    *IndexerCheck      `json:"indexer,omitempty"`
	Answers    *AnswersEvidence   `json:"answers,omitempty"`

	// EpochEnded is whether the chain had finished the epoch when the report was
	// made. `SPEC.md` 5.2 requires a completed validation for every covered
	// flip, so a buyer has to be able to see that this was checked, and that the
	// report was refused when it was not.
	EpochEnded *EpochStateCheck `json:"epochEnded,omitempty"`

	// witness is the shard's committee evidence for the registration prover. It
	// is derived from the same capture the decision was, and it is deliberately
	// not part of the report's JSON: the report is the seller's decision
	// document, and the witness is an input to one proof.
	witness []registrationCandidate
}

// buildEligibilityReport reconstructs one author's flips for one captured epoch.
//
// The author is found in the captured candidate list, and the flips that author
// submitted are the ones the assignment indexes: the shard keeps its flips in
// candidate order, so the author's own indices follow from the list the capture
// holds.
func buildEligibilityReport(dataDir string, epoch uint16, author string, policy string, sweepPath string) (*EligibilityReport, error) {
	if err := requireSupportedEpoch(epoch); err != nil {
		return nil, err
	}
	if policy != EligibilityPolicyV1 {
		return nil, fmt.Errorf("unsupported eligibility policy %q; this release proves %q", policy, EligibilityPolicyV1)
	}
	seed, identities, err := readLotteryRecords(dataDir, epoch)
	if err != nil {
		return nil, err
	}
	if len(identities) == 0 {
		return nil, fmt.Errorf(
			"epoch %d has no lottery identities in %s: a node records them only when it is live at that epoch's FlipLotteryStarted block",
			epoch, dataDir)
	}
	stored, err := readEpochAnswers(dataDir, epoch)
	if err != nil {
		return nil, err
	}
	var chain map[common.Address][]byte
	if sweepPath != "" {
		chain, _, err = answersFromSweep(sweepPath, epoch)
		if err != nil {
			return nil, err
		}
	}
	answers, evidence, err := mergeAnswers(stored, chain)
	if err != nil {
		return nil, err
	}
	if len(answers) == 0 {
		return nil, fmt.Errorf(
			"epoch %d has no long answers in the capture, and no sweep supplied any: pass -sweep with the epoch's own transactions to reconstruct without the capture's copy",
			epoch)
	}

	captured := make([]CapturedIdentity, 0, len(identities))
	for _, identity := range identities {
		cids := make([]string, 0, len(identity.FlipCids))
		for _, cid := range identity.FlipCids {
			cids = append(cids, cidString(cid))
		}
		captured = append(captured, CapturedIdentity{
			Address:  identity.Address.String(),
			Shard:    uint8(identity.ShiftedShardId),
			FlipCids: cids,
		})
	}
	report, err := eligibilityFor(epoch, seed, captured, answers, author, policy)
	if err != nil {
		return nil, err
	}
	report.Answers = evidence
	return report, nil
}

// eligibilityFor is the reconstruction itself: a seed, a captured candidate list
// in the ceremony's order, and the epoch's long answers, in and out.
func eligibilityFor(epoch uint16, seed []byte, captured []CapturedIdentity, answers map[common.Address][]byte, author, policy string) (*EligibilityReport, error) {
	if err := requireSupportedEpoch(epoch); err != nil {
		return nil, err
	}
	if policy != EligibilityPolicyV1 {
		return nil, fmt.Errorf("unsupported eligibility policy %q; this release proves %q", policy, EligibilityPolicyV1)
	}
	wanted := strings.ToLower(strings.TrimPrefix(author, "0x"))
	report := &EligibilityReport{
		Statement:  "fod:eligibility-report:v1",
		Epoch:      epoch,
		Author:     wanted,
		Policy:     policy,
		Rules:      map[string]bool{"upgrade10": true, "upgrade11": true},
		Candidates: len(captured),
	}
	// The reconstruction is per shard, because the lottery and the flip indices
	// are (F-79, F-80). Only the author's shard carries the bundle.
	order := []uint8{}
	byShard := map[uint8][]CapturedIdentity{}
	for _, identity := range captured {
		if _, seen := byShard[identity.Shard]; !seen {
			order = append(order, identity.Shard)
		}
		byShard[identity.Shard] = append(byShard[identity.Shard], identity)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	rules := ConsensusRules{Upgrade10: true, Upgrade11: true}
	for _, shard := range order {
		group := byShard[shard]
		assignment, err := assignmentFor(epoch, seed, group, defaultShortFlipsCount())
		if err != nil {
			return nil, err
		}
		derived := assignment.Shards[0]
		participants, covered, found, err := shardParticipants(group, wanted)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if len(covered) == 0 {
			return nil, fmt.Errorf("the capture holds %s in epoch %d, but records no flips for it", wanted, epoch)
		}
		report.Shard = shard
		verdicts, err := ReconstructFlipVerdicts(participants, answers, lotteryAssignment(derived), rules)
		if err != nil {
			return nil, err
		}
		for _, participant := range participants {
			if _, ok := answers[participant.Address]; ok {
				report.Answered++
			}
		}
		flips := make([]CoveredFlip, 0, len(covered))
		indices := make([]int, 0, len(covered))
		for _, flip := range covered {
			verdict, ok := verdicts[flip.Index]
			if !ok {
				return nil, fmt.Errorf("flip %d has no verdict in shard %d", flip.Index, shard)
			}
			indices = append(indices, flip.Index)
			flips = append(flips, CoveredFlip{
				Index:  flip.Index,
				Cid:    flip.Cid,
				Status: string(verdict.Status),
				Grade:  int(verdict.Grade),
				Answer: answerName(verdict.Answer),
			})
		}
		report.Covered = flips
		decision, err := CheckBundleEligibility(policy, verdicts, indices)
		if err != nil {
			return nil, err
		}
		report.Decision = decision
		witness, err := shardWitness(participants, answers, lotteryAssignment(derived))
		if err != nil {
			return nil, err
		}
		report.witness = witness
		break
	}
	if report.Covered == nil {
		return nil, fmt.Errorf("the capture for epoch %d holds no identity %s", epoch, wanted)
	}
	return report, nil
}

// coveredFlip is one of the author's own flips: the index the assignment uses and
// the CID that goes into the bundle's manifest.
type coveredFlip struct {
	Index int
	Cid   string
}

// shardParticipants turns one shard's captured identities into the participants
// the join takes, and reports which flips are the wanted author's own.
//
// The shard's flip list is the candidates' flips concatenated in the captured
// order, so the author's flips are the indices their own submissions occupy.
func shardParticipants(group []CapturedIdentity, wanted string) ([]EpochParticipant, []coveredFlip, bool, error) {
	participants := make([]EpochParticipant, 0, len(group))
	covered := []coveredFlip{}
	found := false
	index := 0
	for _, identity := range group {
		address, err := addressOf(identity.Address)
		if err != nil {
			return nil, nil, false, err
		}
		flips := make([][]byte, 0, len(identity.FlipCids))
		for _, cid := range identity.FlipCids {
			flips = append(flips, []byte(cid))
		}
		participants = append(participants, EpochParticipant{
			Address:  address,
			IsAuthor: len(flips) > 0,
			Flips:    flips,
		})
		if normalizeAddress(identity.Address) == wanted {
			found = true
			for offset, cid := range identity.FlipCids {
				covered = append(covered, coveredFlip{Index: index + offset, Cid: cid})
			}
		}
		index += len(identity.FlipCids)
	}
	return participants, covered, found, nil
}

// addressOf reads the captured address text into the form the join keys answers
// by. A capture writes what the node writes, so an address this cannot read means
// the capture is not the shape it claims to be, and the reconstruction stops
// rather than answering under a zero address.
func addressOf(value string) (common.Address, error) {
	address := common.Address{}
	raw, err := hex.DecodeString(normalizeAddress(value))
	if err != nil {
		return address, fmt.Errorf("the capture holds %q, which is not an address: %w", value, err)
	}
	if len(raw) != 20 {
		return address, fmt.Errorf("the capture holds %q, which is %d bytes rather than 20", value, len(raw))
	}
	copy(address[:], raw)
	return address, nil
}

func normalizeAddress(value string) string {
	lower := strings.ToLower(value)
	return strings.TrimPrefix(lower, "0x")
}

func lotteryAssignment(derived ShardAssignment) LotteryAssignment {
	return LotteryAssignment{
		AuthorsPerCandidate:    derived.AuthorsPerCandidate,
		CandidatesPerAuthor:    derived.CandidatesPerAuthor,
		ShortFlipsPerCandidate: derived.ShortFlipsPerCandidate,
		LongFlipsPerCandidate:  derived.LongFlipsPerCandidate,
	}
}

// eligibilityRefusal is the one place that decides whether an indexer cross-check
// leaves a bundle registrable. It returns the reason to refuse, or "" when the
// check found nothing to refuse.
//
// `SPEC.md` 5.2 wants a successfully completed validation for every covered flip,
// so a flip the explorer does not list, lists differently, or publishes no
// qualification result for is not evidence of one: all three refuse, and the last
// of them used to be counted and then ignored. The per-identity counts are held
// to the same standard: a candidate who answered must match the assignment
// exactly, and an address the explorer does not list refuses rather than
// preferring either side.
func eligibilityRefusal(check *IndexerCheck, flips *FlipCheck) string {
	if check == nil {
		return ""
	}
	if check.Mismatched > 0 || check.Missing > 0 {
		return check.Note
	}
	if flips == nil {
		return ""
	}
	if len(flips.Mismatched) > 0 || len(flips.Missing) > 0 {
		return flips.Note
	}
	if len(flips.NoResult) > 0 {
		return fmt.Sprintf("%s: an explorer with no qualification result for a covered flip cannot show that it completed validation", flips.Note)
	}
	return ""
}

func runEligibility(dataDir string, epoch uint16, author, policy, outPath, indexerURL, sweepPath string, node *client, requireNode bool, openingBlock uint64) {
	if author == "" {
		fmt.Fprintln(os.Stderr, "eligibility mode needs -datadir PATH -inventory-epoch N -author ADDRESS, and -out FILE")
		os.Exit(2)
	}
	// The epoch has to be over before anything else, because an unfinished
	// validation cannot have produced the completed one `SPEC.md` 5.2 requires.
	ended, err := epochCompletionCheck(node, epoch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "eligibility refused: %v\n", err)
		os.Exit(1)
	}
	if requireNode && !ended.Verified {
		fmt.Fprintf(os.Stderr, "eligibility refused: %s\n", ended.Note)
		os.Exit(1)
	}
	// Said before the work rather than after it: a reader has to know whether the
	// epoch was confirmed finished even when a later step refuses the bundle.
	fmt.Printf("epoch   %s\n", ended.Note)
	report, err := buildEligibilityReport(dataDir, epoch, author, policy, sweepPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "eligibility failed: %v\n", err)
		os.Exit(1)
	}
	report.EpochEnded = &ended
	if report.Answers != nil {
		fmt.Printf("answers %s\n", report.Answers.Note)
	}
	if indexerURL != "" {
		captured, err := buildEpochAssignment(dataDir, epoch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "the indexer cross-check needs the epoch's assignment: %v\n", err)
			os.Exit(1)
		}
		client := newHTTPClient()
		check, err := crossCheckAssignment(client, indexerURL, captured, node, openingBlock)
		if err != nil {
			fmt.Fprintf(os.Stderr, "the indexer cross-check failed: %v\n", err)
			os.Exit(1)
		}
		flips, err := crossCheckCoveredFlips(client, indexerURL, epoch, author, report.Covered)
		if err != nil {
			fmt.Fprintf(os.Stderr, "the covered-flip cross-check failed: %v\n", err)
			os.Exit(1)
		}
		check.Flips = flips
		report.Indexer = check
		if reason := eligibilityRefusal(check, flips); reason != "" {
			fmt.Fprintf(os.Stderr, "eligibility refused: %s\n", reason)
			for _, disagreement := range append(append(append([]string{}, flips.Mismatched...), flips.Missing...), flips.NoResult...) {
				fmt.Fprintf(os.Stderr, "  %s\n", disagreement)
			}
			os.Exit(1)
		}
		// A Plan B source is only as good as what a node can confirm about it,
		// so the epoch's state root is compared with the node's own header
		// whenever a node answers; -require-node turns "no node answered" from a
		// recorded note into a refusal for callers who will not take the
		// explorer's word for the epoch at all.
		if requireNode && !check.NodeVerified {
			fmt.Fprintf(os.Stderr, "eligibility refused: the epoch's state root was not confirmed by a node: %s\n", check.NodeNote)
			os.Exit(1)
		}
		fmt.Printf("indexer %s: %s\n", check.Source, check.Note)
		fmt.Printf("flips   %s\n", flips.Note)
		fmt.Printf("node    %s\n", check.NodeNote)
	}
	if err := writeJSON(outPath, report); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", outPath, err)
		os.Exit(1)
	}
	fmt.Printf("epoch=%d author=%s shard=%d flips=%d candidates=%d answered=%d eligible=%t -> %s\n",
		report.Epoch, report.Author, report.Shard, len(report.Covered),
		report.Candidates, report.Answered, report.Decision.Eligible, outPath)
	for _, flip := range report.Covered {
		fmt.Printf("  flip %d %s: %s, answer %s, grade %d\n", flip.Index, flip.Cid[:16], flip.Status, flip.Answer, flip.Grade)
	}
	for _, exclusion := range report.Decision.Exclusions {
		fmt.Printf("  excluded flip %d: %s\n", exclusion.FlipIndex, exclusion.Reason)
	}
}
