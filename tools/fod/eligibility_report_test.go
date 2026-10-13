package main

// The whole middle of the eligibility half on one synthetic epoch: a captured
// candidate list, the lottery derived from it, long answers encoded with the
// node's own encoder, and the policy's decision at the end.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
)

// The join keys answers by the sender's twenty bytes, so the capture's addresses
// have to be the ones a node writes.
var (
	authorA = "0x" + strings.Repeat("aa", 20)
	authorB = "0x" + strings.Repeat("bb", 20)
	authorC = "0x" + strings.Repeat("cc", 20)
	authorD = "0x" + strings.Repeat("dd", 20)
)

// captureWith builds an epoch with the given candidates, in the ceremony's order.
func captureWith(shard uint8, candidates map[string]int) []CapturedIdentity {
	order := make([]string, 0, len(candidates))
	for address := range candidates {
		order = append(order, address)
	}
	sort.Strings(order)
	captured := make([]CapturedIdentity, 0, len(order))
	for _, address := range order {
		entry := CapturedIdentity{Address: address, Shard: shard}
		for flip := 0; flip < candidates[address]; flip++ {
			entry.FlipCids = append(entry.FlipCids, fmt.Sprintf("bafy%s%d", address[2:], flip))
		}
		captured = append(captured, entry)
	}
	return captured
}

// answersFor encodes one answer per assigned flip, using the node's own encoder.
func answersFor(t *testing.T, seed []byte, captured []CapturedIdentity, decide func(address string, flip int) (types.Answer, types.Grade)) map[common.Address][]byte {
	t.Helper()
	assignment, err := assignmentFor(204, seed, captured, defaultShortFlipsCount())
	if err != nil {
		t.Fatalf("assignment: %v", err)
	}
	derived := assignment.Shards[0]
	out := map[common.Address][]byte{}
	for index, identity := range captured {
		flips := derived.LongFlipsPerCandidate[index]
		encoded := types.NewAnswersFromBits(uint(len(flips)), nil)
		for position, flip := range flips {
			answer, grade := decide(identity.Address, flip)
			if answer == types.Left {
				encoded.Left(uint(position))
			}
			if answer == types.Right {
				encoded.Right(uint(position))
			}
			encoded.Grade(uint(position), grade)
		}
		payload, err := (&attachments.LongAnswerAttachment{Answers: encoded.Bytes()}).ToBytes()
		if err != nil {
			t.Fatalf("encode answers: %v", err)
		}
		address, err := addressOf(identity.Address)
		if err != nil {
			t.Fatalf("capture address: %v", err)
		}
		out[address] = payload
	}
	return out
}

func TestEligibilityReportDecidesABundle(t *testing.T) {
	seed := make([]byte, 32)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	captured := captureWith(1, map[string]int{
		authorA: 1, // the author whose bundle this is
		authorB: 1,
		authorC: 0,
	})

	t.Run("everyone answers left and approves", func(t *testing.T) {
		answers := answersFor(t, seed, captured, func(string, int) (types.Answer, types.Grade) {
			return types.Left, types.GradeA
		})
		report, err := eligibilityFor(204, seed, captured, answers, authorA, EligibilityPolicyV1)
		if err != nil {
			t.Fatalf("report: %v", err)
		}
		if len(report.Covered) != 1 || report.Covered[0].Index != 0 {
			t.Fatalf("the author's flips came out as %+v", report.Covered)
		}
		if !strings.HasPrefix(report.Covered[0].Cid, "bafy") {
			t.Fatalf("the covered flip carries no CID: %+v", report.Covered[0])
		}
		if report.Covered[0].Status != string(FlipQualified) || report.Covered[0].Answer != "left" {
			t.Fatalf("the flip was decided as %+v", report.Covered[0])
		}
		if !report.Decision.Eligible || len(report.Decision.Exclusions) != 0 {
			t.Fatalf("an all-approving committee produced %+v", report.Decision)
		}
		if report.Answered != 3 {
			t.Fatalf("the report counted %d answers, expected 3", report.Answered)
		}
	})

	t.Run("one candidate answers the other way", func(t *testing.T) {
		answers := answersFor(t, seed, captured, func(address string, flip int) (types.Answer, types.Grade) {
			if address == authorB && flip == 0 {
				return types.Right, types.GradeA
			}
			return types.Left, types.GradeA
		})
		report, err := eligibilityFor(204, seed, captured, answers, authorA, EligibilityPolicyV1)
		if err != nil {
			t.Fatalf("report: %v", err)
		}
		if report.Covered[0].Status == string(FlipQualified) {
			t.Fatalf("a split committee still qualified the flip: %+v", report.Covered[0])
		}
		if report.Decision.Eligible {
			t.Fatalf("a split committee produced an eligible bundle: %+v", report.Decision)
		}
		if len(report.Decision.Exclusions) == 0 || report.Decision.Exclusions[0].Reason != ReasonNotStrong {
			t.Fatalf("the exclusion does not name the reason: %+v", report.Decision.Exclusions)
		}
	})

	t.Run("an author the capture does not hold", func(t *testing.T) {
		answers := answersFor(t, seed, captured, func(string, int) (types.Answer, types.Grade) {
			return types.Left, types.GradeA
		})
		if _, err := eligibilityFor(204, seed, captured, answers, authorD, EligibilityPolicyV1); err == nil {
			t.Fatal("an author the capture does not hold produced a report")
		} else if !strings.Contains(err.Error(), "holds no identity") {
			t.Fatalf("the refusal is %q, expected it to say the capture holds no such identity", err)
		}
	})

	t.Run("an identity that submitted no flips", func(t *testing.T) {
		answers := answersFor(t, seed, captured, func(string, int) (types.Answer, types.Grade) {
			return types.Left, types.GradeA
		})
		if _, err := eligibilityFor(204, seed, captured, answers, authorC, EligibilityPolicyV1); err == nil {
			t.Fatal("an identity with no flips produced a bundle report")
		} else if !strings.Contains(err.Error(), "records no flips") {
			t.Fatalf("the refusal is %q, expected it to say the capture records no flips", err)
		}
	})

	t.Run("a policy this release does not prove", func(t *testing.T) {
		if _, err := eligibilityFor(204, seed, captured, nil, authorA, "fod:eligibility:v9"); err == nil {
			t.Fatal("an unsupported policy was applied")
		}
	})
}

// The floor `SPEC.md` 5.2 asks for: an epoch whose qualification ran under other
// rules fails with an explanation instead of being decided with these ones.
func TestEpochsBeforeThePinnedRulesAreRefused(t *testing.T) {
	seed := make([]byte, 32)
	captured := captureWith(1, map[string]int{authorA: 1, authorB: 1})

	if err := requireSupportedEpoch(ConsensusEpochFloor); err != nil {
		t.Fatalf("the first supported epoch was refused: %v", err)
	}
	if err := requireSupportedEpoch(228); err != nil {
		t.Fatalf("a current epoch was refused: %v", err)
	}

	err := requireSupportedEpoch(ConsensusEpochFloor - 1)
	if err == nil {
		t.Fatal("the epoch before the floor was accepted")
	}
	for _, want := range []string{"predates", "consensus V11", "upgrade 10 and upgrade 11", "does not implement"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal does not say %q: %v", want, err)
		}
	}

	// Both entry points refuse, not just the helper.
	if _, err := eligibilityFor(103, seed, captured, nil, authorA, EligibilityPolicyV1); err == nil ||
		!strings.Contains(err.Error(), "predates this release's rules") {
		t.Fatalf("the report builder decided an epoch below the floor: %v", err)
	}
	if _, err := buildEpochAssignment("", 103); err == nil ||
		!strings.Contains(err.Error(), "predates this release's rules") {
		t.Fatalf("the assignment builder read an epoch below the floor: %v", err)
	}
}
