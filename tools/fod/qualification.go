package main

// The per-flip half of Package 2: whether a flip qualified in a closed epoch,
// reconstructed from authenticated inputs.
//
// The rules are the ones recorded in `SOURCES.md` F-61, read from the pinned
// node and written here as this repository's own implementation. No line is
// translated from the pinned files, which are LGPL-3.0 (F-11); the comparison
// points, the gates, and the order they run in are protocol facts.
//
// The shares are compared as float32 because that is the comparison the node
// makes, and the boundaries are the whole point of the rule: 0.66 and 0.75 are
// reached by ratios that do not land on an exact binary value, and a float64
// implementation would disagree with the chain at some of them.

import (
	"fmt"
	"math"
	"math/big"

	"github.com/idena-network/idena-go/blockchain/types"
	"github.com/idena-network/idena-go/common"
)

// FlipStatus is the reconstruction's verdict for one flip, spelled the way the
// node spells it so a reconstruction can be compared with chain data directly.
type FlipStatus string

const (
	FlipQualified       FlipStatus = "qualified"
	FlipWeaklyQualified FlipStatus = "weaklyQualified"
	FlipQualifiedByNone FlipStatus = "qualifiedByNone"
	FlipNotQualified    FlipStatus = "notQualified"
)

// ConsensusRules are the switches that change how a flip is judged. Both were
// already active in the epochs this release supports, and both are kept explicit
// because the thresholds differ across them.
type ConsensusRules struct {
	Upgrade10 bool
	Upgrade11 bool
}

// FlipVerdict is one flip's reconstructed outcome. GradeScore carries the exact
// grade score the upgrade-11 path computes (the node keeps it as a decimal with
// a division precision of 16 digits; this keeps the rational and leaves any
// rounding to whoever records it), and is zero when the integer grade carries
// the outcome, which is how the node leaves that field too. Grade is
// GradeReported when the committee reported the flip, and is otherwise unset on
// the score path.
type FlipVerdict struct {
	Status     FlipStatus
	Answer     types.Answer
	Grade      types.Grade
	GradeScore *big.Rat
}

// FlipCommittee is what an epoch's answers and grades say about one flip.
type FlipCommittee struct {
	// Answers is one answer per participant the flip was assigned to and who
	// answered, in the order the epoch recorded them.
	Answers []types.Answer
	// Reports counts the participants that answered GradeReported on the flip.
	Reports int
	// TotalGradeScore sums the grade points, and Approves counts the grades
	// that carried points: GradeNone and GradeReported add nothing to either.
	TotalGradeScore int
	Approves        int
	// ReportCommitteeSize is the size of the committee that reported or graded
	// the flip. From upgrade 11 the node recomputes it as Approves + Reports;
	// before that it is the running count of graded and reported answers.
	ReportCommitteeSize int
	// GradeScoreCommitteeSize is the divisor of the upgrade-11 score.
	GradeScoreCommitteeSize int
}

// AnswerCounts returns how many answers of the flip's vector were left, right,
// and none.
func AnswerCounts(answers []types.Answer) (left, right, none int) {
	for _, answer := range answers {
		switch answer {
		case types.Left:
			left++
		case types.Right:
			right++
		case types.None:
			none++
		}
	}
	return left, right, none
}

// ReportedEnough is the reported gate: a committee of zero or one never counts
// as reported, two and three need every vote, four needs three, five needs four,
// and anything larger needs more than half.
func ReportedEnough(reports, committee int) bool {
	switch committee {
	case 0, 1:
		return false
	case 2, 3:
		return reports >= committee
	case 4:
		return reports >= 3
	case 5:
		return reports >= 4
	default:
		return float32(reports)/float32(committee) > 0.5
	}
}

// GradedEnough is the graded gate. Upgrade 10 turned the flat "more than a
// third" rule into exact small-committee counts; upgrade 11 changed which
// committee the count is measured against, not the counts themselves.
func GradedEnough(approves, gradedCommitteeSize int, upgrade10 bool) bool {
	if !upgrade10 {
		if gradedCommitteeSize == 0 {
			return false
		}
		return float32(approves)/float32(gradedCommitteeSize) > 0.33
	}
	switch gradedCommitteeSize {
	case 0:
		return false
	case 1:
		return approves == 1
	case 2, 3, 4:
		return approves >= 2
	case 5:
		return approves >= 3
	default:
		return float32(approves)/float32(gradedCommitteeSize) > 0.33
	}
}

// DroppedReports reports whether a participant's reports leave every committee:
// it reported on 34% or more of its own flips.
func DroppedReports(reported, assignedFlips int) bool {
	if assignedFlips == 0 {
		return false
	}
	return float32(reported)/float32(assignedFlips) >= 0.34
}

// gradeScore is the upgrade-11 score as an exact rational.
func gradeScore(totalGradeScore, committee int) *big.Rat {
	if committee == 0 {
		return new(big.Rat)
	}
	return new(big.Rat).SetFrac(big.NewInt(int64(totalGradeScore)), big.NewInt(int64(committee)))
}

// QualifyFlip applies the rules in the order F-61 records them: the reported
// gate first, then the graded gate and the grade, then the answer shares. The
// reported gate wins over the graded one, which is why a reported flip carries
// no grade score.
func QualifyFlip(committee FlipCommittee, rules ConsensusRules) FlipVerdict {
	gradedCommitteeSize := len(committee.Answers)
	if rules.Upgrade11 {
		gradedCommitteeSize = committee.GradeScoreCommitteeSize
	}

	reported := ReportedEnough(committee.Reports, committee.ReportCommitteeSize)
	graded := GradedEnough(committee.Approves, gradedCommitteeSize, rules.Upgrade10)

	var grade types.Grade
	// The node leaves its decimal at its zero value whenever the integer grade
	// carries the outcome, so the score is always present and zero there.
	score := new(big.Rat)
	switch {
	case reported:
		grade = types.GradeReported
	case graded:
		if rules.Upgrade11 {
			score = gradeScore(committee.TotalGradeScore, committee.GradeScoreCommitteeSize)
		} else {
			grade = types.Grade(math.Round(float64(committee.TotalGradeScore) / float64(committee.Approves)))
		}
	default:
		if rules.Upgrade11 {
			score = new(big.Rat).SetInt64(int64(types.GradeD.Score()))
		} else {
			grade = types.GradeD
		}
	}

	left, right, none := AnswerCounts(committee.Answers)
	total := float32(len(committee.Answers))
	verdict := FlipVerdict{Answer: types.None, Grade: grade, GradeScore: score}
	switch {
	case total == 0:
		// No answers at all: the node's comparisons are all false here, so the
		// flip falls through to the same verdict as an indecisive one.
		verdict.Status = FlipNotQualified
	case float32(left)/total >= 0.75:
		verdict.Status, verdict.Answer = FlipQualified, types.Left
	case float32(right)/total >= 0.75:
		verdict.Status, verdict.Answer = FlipQualified, types.Right
	case float32(left)/total >= 0.66:
		verdict.Status, verdict.Answer = FlipWeaklyQualified, types.Left
	case float32(right)/total >= 0.66:
		verdict.Status, verdict.Answer = FlipWeaklyQualified, types.Right
	case float32(none)/total >= 0.66:
		verdict.Status = FlipQualifiedByNone
	default:
		verdict.Status = FlipNotQualified
	}
	return verdict
}

// CandidateGrades is one participant's long answers for an epoch: the answer
// and the grade it gave each flip it was assigned, in assignment order.
type CandidateGrades struct {
	Answers []types.Answer
	Grades  []types.Grade
}

// flipTracker is the upgrade-11 accounting the node keeps per flip
// (`core/ceremony/reporters.go`, read as the fact F-64 records): every grade
// counts towards the committee size and the score sum, including GradeNone,
// whose score is 1.
type flipTracker struct {
	totalScore int
	count      int
	approves   int
	reports    int
}

// BuildFlipCommittees assembles an epoch's per-flip committees from every
// participant's answers: who answered what, who is dropped for reporting too
// much, and what each flip's committee ends up being. `flipsPerCandidate` gives
// each candidate's assigned flip indices in the order the epoch assigned them.
//
// A participant whose answers are absent or empty contributes to no committee,
// which is how the node treats one whose long answers were void.
func BuildFlipCommittees(totalFlips int, flipsPerCandidate [][]int, candidates []CandidateGrades, rules ConsensusRules) ([]FlipCommittee, error) {
	if totalFlips < 0 {
		return nil, fmt.Errorf("flip count %d is negative", totalFlips)
	}
	committees := make([]FlipCommittee, totalFlips)
	trackers := make([]flipTracker, totalFlips)

	for candidateIdx, flips := range flipsPerCandidate {
		if candidateIdx >= len(candidates) {
			return nil, fmt.Errorf("candidate %d has %d flips but no answers", candidateIdx, len(flips))
		}
		candidate := candidates[candidateIdx]
		if len(candidate.Answers) == 0 && len(candidate.Grades) == 0 {
			continue
		}
		if len(candidate.Answers) != len(flips) || len(candidate.Grades) != len(flips) {
			return nil, fmt.Errorf("candidate %d submitted %d answers and %d grades for %d flips",
				candidateIdx, len(candidate.Answers), len(candidate.Grades), len(flips))
		}

		var reportedFlips []int
		hasApprove := false
		increasedGrades := 0
		for position, flipIdx := range flips {
			if flipIdx < 0 || flipIdx >= totalFlips {
				return nil, fmt.Errorf("candidate %d was assigned flip %d, outside 0..%d", candidateIdx, flipIdx, totalFlips-1)
			}
			answer, grade := candidate.Answers[position], candidate.Grades[position]
			committee := &committees[flipIdx]
			committee.Answers = append(committee.Answers, answer)

			switch grade {
			case types.GradeNone:
				// GradeNone adds nothing here. It counts on the upgrade-11
				// path only, through the tracker below.
			case types.GradeReported:
				committee.Reports++
				committee.ReportCommitteeSize++
				reportedFlips = append(reportedFlips, flipIdx)
			default:
				// Before upgrade 11 the score path is off, so the mean is taken
				// over the grade enum values themselves, not over the scores.
				committee.TotalGradeScore += int(grade)
				committee.Approves++
				committee.ReportCommitteeSize++
				hasApprove = true
				if grade > types.GradeD {
					increasedGrades++
				}
			}

			if rules.Upgrade11 {
				tracker := &trackers[flipIdx]
				tracker.count++
				tracker.totalScore += grade.Score()
				if grade >= types.GradeD {
					tracker.approves++
				}
				if grade == types.GradeReported {
					tracker.reports++
				}
			}
		}

		reportShareDrops := DroppedReports(len(reportedFlips), len(flips))
		if !rules.Upgrade11 {
			// Before upgrade 11 only the reports leave: the grades stay, and
			// the committee sizes lose the reported votes.
			if reportShareDrops {
				for _, flipIdx := range reportedFlips {
					committees[flipIdx].Reports--
					committees[flipIdx].ReportCommitteeSize--
				}
			}
			continue
		}

		// From upgrade 11 a participant's whole contribution leaves the
		// committee when it reported too much, approved nothing at all, or
		// approved more than one grade above D.
		if reportShareDrops || !hasApprove || increasedGrades > 1 {
			for position, flipIdx := range flips {
				grade := candidate.Grades[position]
				tracker := &trackers[flipIdx]
				tracker.count--
				tracker.totalScore -= grade.Score()
				if grade >= types.GradeD {
					tracker.approves--
				}
				if grade == types.GradeReported {
					tracker.reports--
				}
			}
		}
	}

	if rules.Upgrade11 {
		// The upgrade-11 path reads its committee straight from the tracker,
		// and its reported gate counts the tracker's reporters.
		for flipIdx := range committees {
			tracker := trackers[flipIdx]
			committees[flipIdx].Reports = tracker.reports
			committees[flipIdx].TotalGradeScore = tracker.totalScore
			committees[flipIdx].Approves = tracker.approves
			committees[flipIdx].GradeScoreCommitteeSize = tracker.count
			committees[flipIdx].ReportCommitteeSize = tracker.approves + tracker.reports
		}
	}
	return committees, nil
}

// CandidateScore is one participant's result against an epoch's flip verdicts.
// Considered marks the flips that counted towards the score, which is what the
// node reports per answer, and Points is the score itself: a matching qualified
// answer is 1, a matching weakly qualified answer is 1, a weakly qualified flip
// answered none is 0, and a weakly qualified flip answered the other way is 0.5.
type CandidateScore struct {
	Points          float32
	QualifiedFlips  uint32
	Considered      map[int]bool
	NoQualification bool
	NoAnswers       bool
}

// CandidateFlipStatus is the short session's override of a flip's status. A flip
// that is already not qualified, that is not in the not-approved set, or that
// was qualified by nobody is left alone; otherwise a None answer turns it into
// not qualified.
func CandidateFlipStatus(flipIndex, position int, base FlipStatus, notApproved map[int]bool, answers []types.Answer, shortSession bool) FlipStatus {
	if !shortSession || base == FlipNotQualified || !notApproved[flipIndex] || base == FlipQualifiedByNone {
		return base
	}
	if position < len(answers) && answers[position] == types.None {
		return FlipNotQualified
	}
	return base
}

// ScoreCandidate reproduces the node's scoring of one participant's answers
// against the epoch's verdicts (`qualifyCandidate`, the facts of F-65).
//
// `answers` holds one answer per entry of `flipsToSolve`, in the same order,
// decoded with the node's own answer decoder. An empty slice is how the node
// sees a participant whose long answers never arrived.
func ScoreCandidate(verdicts map[int]FlipVerdict, flipsToSolve []int, answers []types.Answer, shortSession bool, notApproved map[int]bool) (CandidateScore, error) {
	score := CandidateScore{Considered: make(map[int]bool, len(flipsToSolve))}
	if len(answers) == 0 {
		score.NoAnswers = true
		return score, nil
	}
	if len(answers) < len(flipsToSolve) {
		return score, fmt.Errorf("the participant answered %d flips of %d", len(answers), len(flipsToSolve))
	}

	// A short session may solve up to two extra flips, but only for flips the
	// epoch did not approve and that the participant answered none: those free
	// answers buy the extra flips.
	availableExtraFlips := 0
	if shortSession {
		for i := 0; i < int(common.ShortSessionFlips) && i < len(flipsToSolve) && availableExtraFlips < int(common.ShortSessionExtraFlips); i++ {
			if notApproved[flipsToSolve[i]] && answers[i] == types.None {
				availableExtraFlips++
			}
		}
	}

	for position, flipIndex := range flipsToSolve {
		verdict, ok := verdicts[flipIndex]
		if !ok {
			return score, fmt.Errorf("flip %d has no verdict", flipIndex)
		}
		status := CandidateFlipStatus(flipIndex, position, verdict.Status, notApproved, answers, shortSession)
		answer := answers[position]

		if shortSession && position >= int(common.ShortSessionFlips) {
			if availableExtraFlips > 0 && answer != types.None {
				availableExtraFlips--
			} else {
				// The answer stays in the record but does not count.
				continue
			}
		}

		// A flip the committee reported scores nothing in a short session: the
		// short session cannot see the grade, so it is not paid for a flip whose
		// own answer is in dispute.
		if !shortSession || verdict.Grade != types.GradeReported {
			switch status {
			case FlipQualified:
				// Every qualified flip counts, and only a matching answer pays.
				if verdict.Answer == answer {
					score.Points++
				}
				score.QualifiedFlips++
				score.Considered[flipIndex] = true
			case FlipWeaklyQualified:
				switch {
				case verdict.Answer == answer:
					score.Points++
					score.QualifiedFlips++
					score.Considered[flipIndex] = true
				case answer == types.None:
					score.QualifiedFlips++
					score.Considered[flipIndex] = true
				default:
					score.Points += 0.5
					score.QualifiedFlips++
					score.Considered[flipIndex] = true
				}
			}
		}
	}
	score.NoQualification = score.QualifiedFlips == 0
	return score, nil
}

// VoidedAnswers is what the node returns for a participant whose long answers
// fail the short-answer binding: no points and no considered flips, but the
// assignment's flip count in the qualified count. It is modelled apart because
// it is the node's own shape, not an inference from the other rules.
func VoidedAnswers(assignedFlips int) CandidateScore {
	if assignedFlips < 0 {
		assignedFlips = 0
	}
	return CandidateScore{
		QualifiedFlips:  uint32(assignedFlips),
		Considered:      map[int]bool{},
		NoQualification: false,
	}
}
