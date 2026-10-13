package main

// The join Package 2's eligibility half is missing: an epoch's long answers,
// placed on the flips the lottery assigned, turned into one verdict per flip.
//
// Long answers are a bit string with no flip identifiers in them, and
// `answers.go` had to infer the flip count from the payload's length because the
// assignment did not exist. It does now, so the count comes from the assignment
// instead and a payload that is too short for it is refused rather than
// misread. The rules are the ones F-61 and F-64 record, read from the pinned
// node.

import (
	"fmt"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
)

// EpochParticipant is one candidate of a shard as the epoch store carries them:
// the address the answers are keyed by, and the flips they submitted this epoch,
// in list order, which is also the order the assignment indexes into.
type EpochParticipant struct {
	Address  common.Address
	IsAuthor bool
	Flips    [][]byte
}

// DecodeLongAnswers reads one candidate's long answers against the number of
// flips the assignment gave them.
//
// The payload layout is F-61's: two left/right flag bits and three grade bits
// per flip, so a candidate with n assigned flips fills ceil(5n/8) bytes at most.
// A shorter payload is normal rather than broken: the encoder writes a
// big-endian integer, so leading zero bytes drop off, and a candidate whose last
// flips carry no answer and no grade sends fewer bytes than the bound. Reading
// those missing bits as zero is what the node does too.
//
// More bytes than the bound can hold is the case worth refusing: the node
// ignores the extra bits, but for a reconstruction they mean the payload
// belongs to a candidate with a longer assignment, and silently ignoring them
// would attach another candidate's answers to this one.
func DecodeLongAnswers(payload []byte, flipsCount int) (CandidateGrades, error) {
	if flipsCount <= 0 {
		return CandidateGrades{}, fmt.Errorf("the assignment gives this candidate %d flips", flipsCount)
	}
	attachment := attachments.ParseLongAnswerBytesAttachment(payload)
	if attachment == nil || len(attachment.Answers) == 0 {
		return CandidateGrades{}, fmt.Errorf("the payload carries no answers")
	}
	needed := (5*flipsCount + 7) / 8
	if len(attachment.Answers) > needed {
		return CandidateGrades{}, fmt.Errorf(
			"the payload holds %d answer bytes, %d flips can hold %d",
			len(attachment.Answers), flipsCount, needed)
	}
	decoded := types.NewAnswersFromBits(uint(flipsCount), attachment.Answers)
	out := CandidateGrades{
		Answers: make([]types.Answer, 0, flipsCount),
		Grades:  make([]types.Grade, 0, flipsCount),
	}
	for idx := uint(0); idx < uint(flipsCount); idx++ {
		answer, grade := decoded.Answer(idx)
		out.Answers = append(out.Answers, answer)
		out.Grades = append(out.Grades, grade)
	}
	return out, nil
}

// ReconstructFlipVerdicts applies one shard's assignment to that shard's long
// answers and returns a verdict for every flip in the shard's flattened flip
// list, which is the index space the assignment and the node both use.
//
// A candidate with no stored long answers contributes nothing, which is how the
// node treats one whose answers were absent or void. Short answers are not read:
// they score a candidate, while a flip's verdict comes from the long session.
func ReconstructFlipVerdicts(participants []EpochParticipant, longAnswers map[common.Address][]byte, assignment LotteryAssignment, rules ConsensusRules) (map[int]FlipVerdict, error) {
	totalFlips := 0
	for _, participant := range participants {
		totalFlips += len(participant.Flips)
	}
	if len(assignment.LongFlipsPerCandidate) != len(participants) {
		return nil, fmt.Errorf("the assignment covers %d candidates, the shard has %d",
			len(assignment.LongFlipsPerCandidate), len(participants))
	}

	grades := make([]CandidateGrades, len(participants))
	for idx, participant := range participants {
		payload, ok := longAnswers[participant.Address]
		if !ok || len(payload) == 0 {
			continue
		}
		decoded, err := DecodeLongAnswers(payload, len(assignment.LongFlipsPerCandidate[idx]))
		if err != nil {
			return nil, fmt.Errorf("candidate %d (%s): %w", idx, participant.Address.String(), err)
		}
		grades[idx] = decoded
	}

	committees, err := BuildFlipCommittees(totalFlips, assignment.LongFlipsPerCandidate, grades, rules)
	if err != nil {
		return nil, err
	}
	verdicts := make(map[int]FlipVerdict, len(committees))
	for flipIndex, committee := range committees {
		verdicts[flipIndex] = QualifyFlip(committee, rules)
	}
	return verdicts, nil
}

// shardWitness is one shard's committee evidence in the shape the registration
// prover reads: every candidate with the flips the assignment gave it, the
// answers it gave, and the grades it reported, one entry per assigned flip.
//
// A candidate whose long answers are absent or empty is emitted empty, which is
// how both the node and the proof program treat it - it joins no committee - and
// the proof program skips an entry with no answers and no grades. A candidate
// that did answer keeps its assignment in the same index space the verdicts and
// the inventory use, which is the shard's flattened flip list.
func shardWitness(participants []EpochParticipant, longAnswers map[common.Address][]byte, assignment LotteryAssignment) ([]registrationCandidate, error) {
	witness := make([]registrationCandidate, len(participants))
	for index, participant := range participants {
		payload, ok := longAnswers[participant.Address]
		if !ok || len(payload) == 0 {
			continue
		}
		flips := assignment.LongFlipsPerCandidate[index]
		decoded, err := DecodeLongAnswers(payload, len(flips))
		if err != nil {
			return nil, fmt.Errorf("candidate %d (%s): %w", index, participant.Address.String(), err)
		}
		entry := registrationCandidate{
			Assignment: make([]uint32, 0, len(flips)),
			Answers:    make([]int, 0, len(flips)),
			Grades:     make([]int, 0, len(flips)),
		}
		for _, flip := range flips {
			entry.Assignment = append(entry.Assignment, uint32(flip))
		}
		for position := range decoded.Answers {
			entry.Answers = append(entry.Answers, int(decoded.Answers[position]))
			entry.Grades = append(entry.Grades, int(decoded.Grades[position]))
		}
		witness[index] = entry
	}
	return witness, nil
}
