package main

// The join against one frozen lottery case: answers built with the node's own
// encoder, placed by the vector's assignment, and read back as per-flip
// verdicts.
//
// The assignment comes from `lottery-v1.json` rather than from the port, so a
// mis-placed answer lands on a flip other than the one this test configures and
// the verdicts below stop matching.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
)

func TestEpochJoinPlacesLongAnswersOnAssignedFlips(t *testing.T) {
	vector := readLotteryCase(t, "seven-candidates-three-authors")
	assignment := lotteryVectorAssignment(vector.Shards[0])
	participants := lotteryVectorParticipants(vector.Shards[0])
	if len(participants) != 7 {
		t.Fatalf("the vector case has %d participants, expected 7", len(participants))
	}

	const (
		qualifiedFlip = 0
		noneFlip      = 2
	)
	if qualifiedFlip == noneFlip {
		t.Fatal("the three cases need three different flips")
	}

	// Which candidates were assigned each flip, straight from the vector.
	graders := map[int][]int{}
	for candidate, flips := range assignment.LongFlipsPerCandidate {
		for _, flip := range flips {
			graders[flip] = append(graders[flip], candidate)
		}
	}
	if len(graders) != 6 {
		t.Fatalf("the vector assigns %d flips, expected 6", len(graders))
	}
	reportedFlip := -1
	for flip, assigned := range graders {
		if flip == qualifiedFlip || flip == noneFlip {
			continue
		}
		if len(assigned) >= 4 && (reportedFlip == -1 || flip < reportedFlip) {
			reportedFlip = flip
		}
	}
	if reportedFlip == -1 {
		t.Fatal("no flip besides the two fixed ones has four or more graders, so the reported case cannot be built")
	}

	if qualifiedFlip == reportedFlip || noneFlip == reportedFlip {
		t.Fatalf("the three cases need three different flips, got %d, %d, %d", qualifiedFlip, noneFlip, reportedFlip)
	}

	// Every candidate answers to the same plan, per flip of their assignment.
	payloads := map[common.Address][]byte{}
	for candidate, participant := range participants {
		flips := assignment.LongFlipsPerCandidate[candidate]
		answers := types.NewAnswersFromBits(uint(len(flips)), nil)
		for position, flip := range flips {
			switch flip {
			case qualifiedFlip:
				answers.Left(uint(position))
				answers.Grade(uint(position), types.GradeA)
			case reportedFlip:
				answers.Right(uint(position))
				answers.Grade(uint(position), types.GradeReported)
			case noneFlip:
				answers.Grade(uint(position), types.GradeNone)
			}
		}
		payloads[participant.Address] = encodeLongAnswers(answers)
	}

	verdicts, err := ReconstructFlipVerdicts(participants, payloads, assignment, ConsensusRules{Upgrade10: true, Upgrade11: true})
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	if len(verdicts) != len(graders) {
		t.Fatalf("the reconstruction returned %d verdicts for %d flips", len(verdicts), len(graders))
	}

	if got := verdicts[qualifiedFlip]; got.Status != FlipQualified || got.Answer != types.Left {
		t.Errorf("flip %d is %s/%v, expected %s/left", qualifiedFlip, got.Status, got.Answer, FlipQualified)
	}
	if got := verdicts[noneFlip]; got.Status != FlipQualifiedByNone {
		t.Errorf("flip %d is %s, expected %s", noneFlip, got.Status, FlipQualifiedByNone)
	}
	if got := verdicts[reportedFlip]; got.Grade != types.GradeReported {
		t.Errorf("flip %d carries grade %v, expected the reported grade", reportedFlip, got.Grade)
	}

	// A candidate whose answers are absent contributes nothing: dropping every
	// payload for the qualified flip's graders leaves that flip with fewer
	// answers, so it can no longer reach the strong answer share.
	withoutGraders := map[common.Address][]byte{}
	for candidate, participant := range participants {
		payload, ok := payloads[participant.Address]
		if !ok || assignedTo(graders[qualifiedFlip], candidate) {
			continue
		}
		withoutGraders[participant.Address] = payload
	}
	thinned, err := ReconstructFlipVerdicts(participants, withoutGraders, assignment, ConsensusRules{Upgrade10: true, Upgrade11: true})
	if err != nil {
		t.Fatalf("reconstruct without the graders: %v", err)
	}
	if got := thinned[qualifiedFlip]; got.Status == FlipQualified {
		t.Errorf("flip %d still qualifies with every one of its answers removed", qualifiedFlip)
	}
}

func TestEpochJoinReadsShortPayloadsAndRefusesWideOnes(t *testing.T) {
	vector := readLotteryCase(t, "seven-candidates-three-authors")
	participants := lotteryVectorParticipants(vector.Shards[0])
	assignment := lotteryVectorAssignment(vector.Shards[0])

	// The encoder writes a big-endian integer, so a candidate whose later flips
	// carry no answer drop those bytes. Six flips bound to four bytes, two used.
	tail := types.NewAnswersFromBits(6, nil)
	tail.Left(0)
	tail.Grade(0, types.GradeA)
	if len(tail.Bytes()) >= 4 {
		t.Fatalf("the bit string is %d bytes, expected fewer than the four-byte bound", len(tail.Bytes()))
	}
	short := encodeLongAnswers(tail)
	decoded, err := DecodeLongAnswers(short, 6)
	if err != nil {
		t.Fatalf("a payload with a zero tail was refused: %v", err)
	}
	if decoded.Answers[0] != types.Left || decoded.Grades[0] != types.GradeA {
		t.Errorf("position 0 is %v/%v, expected left/GradeA", decoded.Answers[0], decoded.Grades[0])
	}
	if decoded.Answers[5] != types.None || decoded.Grades[5] != types.GradeNone {
		t.Errorf("the missing tail read as %v/%v, expected no answer and no grade", decoded.Answers[5], decoded.Grades[5])
	}

	// A payload wider than the assignment can hold belongs to a candidate with
	// more flips, so it is refused instead of read down to this one's size.
	// Twelve flips, twice the six this shard assigns, so the reconstruction has
	// to refuse it rather than read the first six positions.
	wide := types.NewAnswersFromBits(12, nil)
	for position := uint(0); position < 12; position++ {
		wide.Left(position)
		wide.Grade(position, types.GradeA)
	}
	if len(wide.Bytes()) <= 1 {
		t.Fatalf("the wide bit string is %d bytes, expected more than a one-flip assignment can hold", len(wide.Bytes()))
	}
	payload := encodeLongAnswers(wide)
	if _, err := DecodeLongAnswers(payload, 1); err == nil {
		t.Fatal("a twelve-flip payload was read as a one-flip assignment")
	} else if !strings.Contains(err.Error(), "answer bytes") {
		t.Fatalf("the refusal does not name the size mismatch: %v", err)
	}

	// Empty and unparsable payloads are refused rather than read as silence.
	if _, err := DecodeLongAnswers(nil, 6); err == nil {
		t.Fatal("an empty payload was accepted")
	}
	if _, err := ReconstructFlipVerdicts(participants, map[common.Address][]byte{
		participants[0].Address: payload,
	}, assignment, ConsensusRules{}); err == nil {
		t.Fatal("the reconstruction accepted a payload wider than its assignment")
	}
}

func readLotteryCase(t *testing.T, name string) lotteryVector {
	t.Helper()
	raw, err := os.ReadFile("../../proofs/registration/vectors/lottery-v1.json")
	if err != nil {
		t.Fatalf("read the lottery vector: %v", err)
	}
	var file lotteryVectorFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse the lottery vector: %v", err)
	}
	for _, vector := range file.Cases {
		if vector.Name == name {
			if len(vector.Shards) != 1 {
				t.Fatalf("%s has %d shards, expected 1", name, len(vector.Shards))
			}
			return vector
		}
	}
	t.Fatalf("the lottery vector has no case %q", name)
	return lotteryVector{}
}

func lotteryVectorParticipants(shard lotteryVectorShard) []EpochParticipant {
	out := make([]EpochParticipant, 0, len(shard.Candidates))
	for idx, candidate := range shard.Candidates {
		address := common.Address{}
		address[19] = byte(idx + 1)
		entry := EpochParticipant{Address: address, IsAuthor: candidate.IsAuthor}
		for _, flip := range candidate.Flips {
			entry.Flips = append(entry.Flips, []byte(flip))
		}
		out = append(out, entry)
	}
	return out
}

func lotteryVectorAssignment(shard lotteryVectorShard) LotteryAssignment {
	return LotteryAssignment{
		AuthorsPerCandidate:    shard.AuthorsPerCandidate,
		CandidatesPerAuthor:    shard.CandidatesPerAuthor,
		ShortFlipsPerCandidate: shard.ShortFlipsPerCandidate,
		LongFlipsPerCandidate:  shard.LongFlipsPerCandidate,
	}
}

// encodeLongAnswers wraps a bit string in a long-answer payload, the shape the
// node stores and `DecodeLongAnswers` parses.
func encodeLongAnswers(answers *types.Answers) []byte {
	attachment := &attachments.LongAnswerAttachment{Answers: answers.Bytes()}
	payload, err := attachment.ToBytes()
	if err != nil {
		panic(err)
	}
	return payload
}

func assignedTo(graders []int, candidate int) bool {
	for _, grader := range graders {
		if grader == candidate {
			return true
		}
	}
	return false
}
