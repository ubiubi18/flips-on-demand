package main

import (
	"math/big"
	"testing"

	"github.com/idena-network/idena-go/blockchain/types"
)

// answersFixture is a flip's answer vector with the lefts first, then the
// rights, then the nones, which is the shape the node's own tests build.
func answersFixture(left, right, none int) []types.Answer {
	answers := make([]types.Answer, 0, left+right+none)
	for i := 0; i < left; i++ {
		answers = append(answers, types.Left)
	}
	for i := 0; i < right; i++ {
		answers = append(answers, types.Right)
	}
	for i := 0; i < none; i++ {
		answers = append(answers, types.None)
	}
	return answers
}

func committee(left, right, none, reports, totalGradeScore, approves, reportCommitteeSize, gradeScoreCommitteeSize int) FlipCommittee {
	return FlipCommittee{
		Answers:                 answersFixture(left, right, none),
		Reports:                 reports,
		TotalGradeScore:         totalGradeScore,
		Approves:                approves,
		ReportCommitteeSize:     reportCommitteeSize,
		GradeScoreCommitteeSize: gradeScoreCommitteeSize,
	}
}

func rat(numerator, denominator int64) *big.Rat {
	return new(big.Rat).SetFrac(big.NewInt(numerator), big.NewInt(denominator))
}

// TestQualifyFlipMatchesTheNodesVectors runs the cases recorded from the node's
// own test file (`core/ceremony/qualification_test.go`, `Test_qualifyOneFlip`)
// and listed in `SOURCES.md` F-61. They are the check on this implementation:
// the thresholds, the gate order, and the score arithmetic all have to agree
// with the node for each of them.
func TestQualifyFlipMatchesTheNodesVectors(t *testing.T) {
	rules := ConsensusRules{Upgrade10: true, Upgrade11: true}
	zero := new(big.Rat)
	// The threshold-only cases below leave every committee input at zero, which
	// grades nothing at all: the node's default branch then writes grade D's
	// score, not zero. Only a reported flip keeps the zero value.
	gradeDScore := new(big.Rat).SetInt64(int64(types.GradeD.Score()))
	cases := []struct {
		name   string
		flip   FlipCommittee
		status FlipStatus
		answer types.Answer
		grade  types.Grade
		score  *big.Rat
	}{
		{name: "6 left 1 right 1 none, 4 reports of 8, graded 18 of 8", flip: committee(6, 1, 1, 4, 18, 4, 8, 8), status: FlipQualified, answer: types.Left, score: rat(18, 8)},
		{name: "6 left of 6, 2 reports of 4, graded 6 of 4", flip: committee(6, 0, 0, 2, 6, 2, 4, 4), status: FlipQualified, answer: types.Left, score: rat(6, 4)},
		{name: "7 left of 7, 2 reports of 4, graded 8 of 4", flip: committee(7, 0, 0, 2, 8, 2, 4, 4), status: FlipQualified, answer: types.Left, score: rat(8, 4)},
		{name: "7 left of 7, 3 reports of 6, graded 12 of 7", flip: committee(7, 0, 0, 3, 12, 3, 6, 7), status: FlipQualified, answer: types.Left, score: rat(12, 7)},
		{name: "6 left 1 right 1 none, 3 reports of 6, graded 13 of 6", flip: committee(6, 1, 1, 3, 13, 3, 6, 6), status: FlipQualified, answer: types.Left, score: rat(13, 6)},
		{name: "6 left 1 right 1 none, 4 reports of 8, graded 12 of 8", flip: committee(6, 1, 1, 4, 12, 4, 8, 8), status: FlipQualified, answer: types.Left, score: rat(12, 8)},
		{name: "6 left 1 right 1 none, 4 reports of 8, graded 8 of 8", flip: committee(6, 1, 1, 4, 8, 4, 8, 8), status: FlipQualified, answer: types.Left, score: rat(8, 8)},
		{name: "6 left 1 right 1 none, 5 reports of 8", flip: committee(6, 1, 1, 5, 6, 3, 8, 8), status: FlipQualified, answer: types.Left, grade: types.GradeReported, score: zero},

		// The answer thresholds, with the committee inputs at zero: 75% is
		// qualified, 66% is weakly qualified, and neither is not qualified.
		{name: "25 left 75 right, 100 answers", flip: committee(25, 75, 0, 0, 0, 0, 0, 0), status: FlipQualified, answer: types.Right, score: gradeDScore},
		{name: "10 right of 10", flip: committee(0, 10, 0, 0, 0, 0, 0, 0), status: FlipQualified, answer: types.Right, score: gradeDScore},
		{name: "15 left 3 right 4 none, 22 answers", flip: committee(15, 3, 4, 0, 0, 0, 0, 0), status: FlipWeaklyQualified, answer: types.Left, score: gradeDScore},
		{name: "30 left 66 right 4 none, 100 answers", flip: committee(30, 66, 4, 0, 0, 0, 0, 0), status: FlipWeaklyQualified, answer: types.Right, score: gradeDScore},
		{name: "4 left 4 right 4 none, 12 answers", flip: committee(4, 4, 4, 0, 0, 0, 0, 0), status: FlipNotQualified, answer: types.None, score: gradeDScore},
		{name: "1 left 2 right 10 none, 13 answers", flip: committee(1, 2, 10, 0, 0, 0, 0, 0), status: FlipQualifiedByNone, answer: types.None, score: gradeDScore},
		{name: "15 right of 15", flip: committee(0, 15, 0, 0, 0, 0, 0, 0), status: FlipQualified, answer: types.Right, score: gradeDScore},
	}

	for _, c := range cases {
		verdict := QualifyFlip(c.flip, rules)
		if verdict.Status != c.status {
			t.Errorf("%s: status is %s, expected %s", c.name, verdict.Status, c.status)
		}
		if verdict.Answer != c.answer {
			t.Errorf("%s: answer is %d, expected %d", c.name, verdict.Answer, c.answer)
		}
		if verdict.Grade != c.grade {
			t.Errorf("%s: grade is %d, expected %d", c.name, verdict.Grade, c.grade)
		}
		if verdict.GradeScore.Cmp(c.score) != 0 {
			t.Errorf("%s: grade score is %s, expected %s", c.name, verdict.GradeScore.RatString(), c.score.RatString())
		}
	}
}

// TestTheGatesHaveTheirOwnBoundaries covers what the vectors above do not: the
// exact small committees, the report share that drops a reporter, and the
// threshold the epoch used before upgrade 10.
func TestTheGatesHaveTheirOwnBoundaries(t *testing.T) {
	if ReportedEnough(1, 1) {
		t.Error("a committee of one reported a flip")
	}
	if ReportedEnough(2, 3) {
		t.Error("two reports of three counted as reported")
	}
	if !ReportedEnough(3, 3) {
		t.Error("three reports of three did not count as reported")
	}
	if ReportedEnough(2, 4) {
		t.Error("two reports of four counted as reported")
	}
	if !ReportedEnough(3, 4) {
		t.Error("three reports of four did not count as reported")
	}
	if ReportedEnough(3, 5) {
		t.Error("three reports of five counted as reported")
	}
	if !ReportedEnough(4, 5) {
		t.Error("four reports of five did not count as reported")
	}
	if ReportedEnough(5, 10) {
		t.Error("exactly half counted as reported")
	}
	if !ReportedEnough(6, 10) {
		t.Error("more than half did not count as reported")
	}

	if GradedEnough(0, 0, true) {
		t.Error("an empty committee graded a flip")
	}
	if !GradedEnough(1, 1, true) {
		t.Error("a single approve did not grade a flip")
	}
	if GradedEnough(1, 4, true) {
		t.Error("one approve of four graded a flip")
	}
	if !GradedEnough(2, 4, true) {
		t.Error("two approves of four did not grade a flip")
	}
	if GradedEnough(2, 5, true) {
		t.Error("two approves of five graded a flip")
	}
	if !GradedEnough(3, 5, true) {
		t.Error("three approves of five did not grade a flip")
	}
	if GradedEnough(3, 10, true) {
		t.Error("three approves of ten graded a flip")
	}
	if !GradedEnough(4, 10, true) {
		t.Error("four approves of ten did not grade a flip")
	}
	// Before upgrade 10 the small-committee counts do not apply: the rule is
	// the flat third, so one approve of one still grades a flip.
	if !GradedEnough(1, 1, false) {
		t.Error("one approve of one did not grade a flip before upgrade ten")
	}
	if GradedEnough(0, 0, false) {
		t.Error("an empty committee graded a flip before upgrade ten")
	}

	if DroppedReports(0, 0) {
		t.Error("a participant with no flips dropped its reports")
	}
	if DroppedReports(33, 100) {
		t.Error("33% of flips reported dropped the reports")
	}
	if !DroppedReports(34, 100) {
		t.Error("34% of flips reported did not drop the reports")
	}
	if !DroppedReports(1, 2) {
		t.Error("half of a small set did not drop the reports")
	}
}

// TestAnswerCountsIgnoresWhatItDoesNotKnow keeps an unknown answer out of the
// shares instead of counting it as none: the node counts the three values it
// names and nothing else.
func TestAnswerCountsIgnoresWhatItDoesNotKnow(t *testing.T) {
	answers := append(answersFixture(3, 2, 1), types.Answer(9))
	left, right, none := AnswerCounts(answers)
	if left != 3 || right != 2 || none != 1 {
		t.Fatalf("counts are %d/%d/%d, expected 3/2/1", left, right, none)
	}

	// A flip nobody answered is not qualified.
	verdict := QualifyFlip(FlipCommittee{}, ConsensusRules{Upgrade10: true, Upgrade11: true})
	if verdict.Status != FlipNotQualified || verdict.Answer != types.None {
		t.Fatalf("an unanswered flip is %s/%d, expected notQualified/none", verdict.Status, verdict.Answer)
	}
}

// epochFixture is three participants over three flips, with one report between
// them so no participant crosses the 34% report share, and no participant
// approving more than one grade above D so none is dropped for that either.
func epochFixture() ([][]int, []CandidateGrades) {
	flipsPerCandidate := [][]int{{0, 1, 2}, {0, 2}, {1}}
	candidates := []CandidateGrades{
		{
			Answers: []types.Answer{types.Left, types.Left, types.Left},
			Grades:  []types.Grade{types.GradeD, types.GradeReported, types.GradeD},
		},
		{
			// A GradeNone beside one grade D: from upgrade 11 the None counts
			// towards the committee and scores 1, and a participant with at
			// least one approve keeps its grades.
			Answers: []types.Answer{types.Right, types.Right},
			Grades:  []types.Grade{types.GradeNone, types.GradeD},
		},
		{
			Answers: []types.Answer{types.Left},
			Grades:  []types.Grade{types.GradeC},
		},
	}
	return flipsPerCandidate, candidates
}

// TestBuildFlipCommitteesAccountsForBothPaths pins what changed at upgrade 11:
// before it the score path is off, so the sum is over the grade enum values and
// only graded or reported answers count towards the committee; from it every
// grade counts and the sum is over the grade's score, so a GradeNone adds one.
func TestBuildFlipCommitteesAccountsForBothPaths(t *testing.T) {
	flipsPerCandidate, candidates := epochFixture()

	before, err := BuildFlipCommittees(3, flipsPerCandidate, candidates, ConsensusRules{Upgrade10: true})
	if err != nil {
		t.Fatalf("assemble the pre-upgrade-11 committees: %v", err)
	}
	if len(before) != 3 {
		t.Fatalf("the epoch has %d flips, expected 3", len(before))
	}
	// Flip 0: one grade D (enum 2) and one GradeNone, which does not count here.
	if before[0].TotalGradeScore != 2 || before[0].Approves != 1 || before[0].ReportCommitteeSize != 1 || before[0].Reports != 0 {
		t.Errorf("flip 0 is %+v, expected score 2, 1 approve, committee 1, no reports", before[0])
	}
	if len(before[0].Answers) != 2 || before[0].Answers[0] != types.Left || before[0].Answers[1] != types.Right {
		t.Errorf("flip 0 carries the wrong answers: %v", before[0].Answers)
	}
	// Flip 1: one report and one grade C (enum 3).
	if before[1].TotalGradeScore != 3 || before[1].Approves != 1 || before[1].ReportCommitteeSize != 2 || before[1].Reports != 1 {
		t.Errorf("flip 1 is %+v, expected score 3, 1 approve, committee 2, 1 report", before[1])
	}
	// Flip 2: two grades D, the GradeNone counting for nothing here.
	if before[2].TotalGradeScore != 4 || before[2].Approves != 2 || before[2].ReportCommitteeSize != 2 || before[2].Reports != 0 {
		t.Errorf("flip 2 is %+v, expected score 4, 2 approves, committee 2, no reports", before[2])
	}

	after, err := BuildFlipCommittees(3, flipsPerCandidate, candidates, ConsensusRules{Upgrade10: true, Upgrade11: true})
	if err != nil {
		t.Fatalf("assemble the upgrade-11 committees: %v", err)
	}
	// Flip 0: grade D scores 2 and approves, GradeNone scores 1 and does not.
	if after[0].TotalGradeScore != 3 || after[0].Approves != 1 || after[0].GradeScoreCommitteeSize != 2 {
		t.Errorf("flip 0 is %+v, expected score 3, 1 approve, committee size 2", after[0])
	}
	if after[0].Reports != 0 || after[0].ReportCommitteeSize != 1 {
		t.Errorf("flip 0 reports are %d of committee %d, expected 0 of 1", after[0].Reports, after[0].ReportCommitteeSize)
	}
	// Flip 1: a report scores nothing, grade C scores 4.
	if after[1].TotalGradeScore != 4 || after[1].Approves != 1 || after[1].GradeScoreCommitteeSize != 2 {
		t.Errorf("flip 1 is %+v, expected score 4, 1 approve, committee size 2", after[1])
	}
	if after[1].Reports != 1 || after[1].ReportCommitteeSize != 2 {
		t.Errorf("flip 1 reports are %d of committee %d, expected 1 of 2", after[1].Reports, after[1].ReportCommitteeSize)
	}
	// Flip 2: two grades D.
	if after[2].TotalGradeScore != 4 || after[2].Approves != 2 || after[2].GradeScoreCommitteeSize != 2 || after[2].Reports != 0 {
		t.Errorf("flip 2 is %+v, expected score 4, 2 approves, committee size 2, no reports", after[2])
	}
}

// TestBuildFlipCommitteesDropsAParticipantWithNoApprove is the third upgrade-11
// drop, and the one that is easy to miss: a participant that never approved
// anything loses its grades too, so a GradeNone answer leaves the committee with
// them.
func TestBuildFlipCommitteesDropsAParticipantWithNoApprove(t *testing.T) {
	flipsPerCandidate := [][]int{{0}}
	candidates := []CandidateGrades{{
		Answers: []types.Answer{types.Left},
		Grades:  []types.Grade{types.GradeNone},
	}}

	after, err := BuildFlipCommittees(1, flipsPerCandidate, candidates, ConsensusRules{Upgrade10: true, Upgrade11: true})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if after[0].GradeScoreCommitteeSize != 0 || after[0].Approves != 0 || after[0].TotalGradeScore != 0 {
		t.Errorf("flip 0 is %+v, expected the GradeNone to be dropped with the rest", after[0])
	}
	if len(after[0].Answers) != 1 {
		t.Errorf("the answer left the flip's vector, but only the grades should have")
	}
}

// TestBuildFlipCommitteesDropsTooManyIncreasedGrades covers the other upgrade-11
// drop, the one that has nothing to do with reports: a participant that approves
// more than one grade above D loses its whole contribution.
func TestBuildFlipCommitteesDropsTooManyIncreasedGrades(t *testing.T) {
	flipsPerCandidate := [][]int{{0, 1}}
	candidates := []CandidateGrades{{
		Answers: []types.Answer{types.Left, types.Left},
		Grades:  []types.Grade{types.GradeC, types.GradeC},
	}}

	after, err := BuildFlipCommittees(2, flipsPerCandidate, candidates, ConsensusRules{Upgrade10: true, Upgrade11: true})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	for flipIdx, committee := range after {
		if committee.Approves != 0 || committee.GradeScoreCommitteeSize != 0 {
			t.Errorf("flip %d kept %+v, expected the contribution to be dropped", flipIdx, committee)
		}
	}

	// Before upgrade 11 the same grades stay: two grades above D are not a drop
	// there, and this participant never reported anything.
	before, err := BuildFlipCommittees(2, flipsPerCandidate, candidates, ConsensusRules{Upgrade10: true})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	for flipIdx, committee := range before {
		if committee.Approves != 1 || committee.TotalGradeScore != 3 {
			t.Errorf("flip %d is %+v, expected one grade C and a score of 3", flipIdx, committee)
		}
	}
}

// TestBuildFlipCommitteesDropsAReportHeavyCandidate covers both drop rules: a
// participant that reported on all of its flips loses its reports, and from
// upgrade 11 it loses its whole contribution.
func TestBuildFlipCommitteesDropsAReportHeavyCandidate(t *testing.T) {
	flipsPerCandidate := [][]int{{0, 1}}
	candidates := []CandidateGrades{{
		Answers: []types.Answer{types.Left, types.Left},
		Grades:  []types.Grade{types.GradeReported, types.GradeReported},
	}}

	before, err := BuildFlipCommittees(2, flipsPerCandidate, candidates, ConsensusRules{Upgrade10: true})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	for flipIdx, committee := range before {
		if committee.Reports != 0 || committee.ReportCommitteeSize != 0 {
			t.Errorf("flip %d kept %d reports of committee %d after the drop", flipIdx, committee.Reports, committee.ReportCommitteeSize)
		}
	}

	after, err := BuildFlipCommittees(2, flipsPerCandidate, candidates, ConsensusRules{Upgrade10: true, Upgrade11: true})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	for flipIdx, committee := range after {
		if committee.Reports != 0 || committee.GradeScoreCommitteeSize != 0 || committee.ReportCommitteeSize != 0 {
			t.Errorf("flip %d kept %+v after the drop", flipIdx, committee)
		}
	}
}

// TestBuildFlipCommitteesKeepsAModerateReporter is the other side of the rule:
// one report in three flips is below the share, and grades up to D are not
// "increased" grades, so the participant stays in the committee.
func TestBuildFlipCommitteesKeepsAModerateReporter(t *testing.T) {
	flipsPerCandidate := [][]int{{0, 1, 2}}
	candidates := []CandidateGrades{{
		Answers: []types.Answer{types.Left, types.Left, types.Left},
		Grades:  []types.Grade{types.GradeReported, types.GradeD, types.GradeD},
	}}

	after, err := BuildFlipCommittees(3, flipsPerCandidate, candidates, ConsensusRules{Upgrade10: true, Upgrade11: true})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if after[0].Reports != 1 {
		t.Errorf("the report was dropped: flip 0 has %d reports", after[0].Reports)
	}
	if after[1].Approves != 1 || after[1].GradeScoreCommitteeSize != 1 {
		t.Errorf("the grade D was dropped: flip 1 is %+v", after[1])
	}
	if after[2].Approves != 1 {
		t.Errorf("the second grade D was dropped: flip 2 is %+v", after[2])
	}
}

// TestBuildFlipCommitteesRefusalsAndVoidAnswers covers the inputs the assembly
// has to refuse and the participant the node skips entirely.
func TestBuildFlipCommitteesRefusalsAndVoidAnswers(t *testing.T) {
	if _, err := BuildFlipCommittees(1, [][]int{{0}}, nil, ConsensusRules{}); err == nil {
		t.Error("a candidate with flips and no answers was accepted")
	}
	if _, err := BuildFlipCommittees(1, [][]int{{0}}, []CandidateGrades{{Answers: []types.Answer{types.Left}}}, ConsensusRules{}); err == nil {
		t.Error("a candidate with more flips than grades was accepted")
	}
	if _, err := BuildFlipCommittees(1, [][]int{{1}}, []CandidateGrades{{Answers: []types.Answer{types.Left}, Grades: []types.Grade{types.GradeD}}}, ConsensusRules{}); err == nil {
		t.Error("a flip outside the epoch was accepted")
	}

	// A participant whose answers are void contributes to no committee, but the
	// others still do.
	flipsPerCandidate := [][]int{{0}, {0}}
	candidates := []CandidateGrades{
		{},
		{Answers: []types.Answer{types.Left}, Grades: []types.Grade{types.GradeA}},
	}
	committees, err := BuildFlipCommittees(1, flipsPerCandidate, candidates, ConsensusRules{Upgrade10: true, Upgrade11: true})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(committees[0].Answers) != 1 || committees[0].Approves != 1 {
		t.Fatalf("the void participant still counted: %+v", committees[0])
	}
}

// candidateVerdicts is the flip qualification map of the node's own
// `Test_qualifyCandidate`, plus the participant's answers for the two sessions.
func candidateVerdicts() map[int]FlipVerdict {
	return map[int]FlipVerdict{
		10: {Status: FlipQualified, Answer: types.Left, Grade: types.GradeReported},
		11: {Status: FlipNotQualified, Answer: types.None, Grade: types.GradeD},
		12: {Status: FlipNotQualified, Answer: types.None, Grade: types.GradeD},
		13: {Status: FlipNotQualified, Answer: types.None, Grade: types.GradeD},
		20: {Status: FlipWeaklyQualified, Answer: types.Right, Grade: types.GradeD},
		21: {Status: FlipWeaklyQualified, Answer: types.Right, Grade: types.GradeD},
		22: {Status: FlipWeaklyQualified, Answer: types.Left, Grade: types.GradeD},
		23: {Status: FlipQualified, Answer: types.Right, Grade: types.GradeD},
		24: {Status: FlipQualified, Answer: types.Right, Grade: types.GradeD},
	}
}

func candidateAnswers() []types.Answer {
	return []types.Answer{
		types.Left, types.None, types.None, types.Right, types.Right,
		types.Left, types.Left, types.Left, types.Right,
	}
}

// TestScoreCandidateMatchesTheNodesVectors runs the node's own
// `Test_qualifyCandidate` numbers: a short session, a long session, and a
// participant whose answers fail the short-answer binding.
func TestScoreCandidateMatchesTheNodesVectors(t *testing.T) {
	verdicts := candidateVerdicts()
	answers := candidateAnswers()
	notApproved := map[int]bool{11: true, 12: true}

	shortFlips := []int{10, 11, 12, 13, 20, 21, 22}
	short, err := ScoreCandidate(verdicts, shortFlips, answers, true, notApproved)
	if err != nil {
		t.Fatalf("score the short session: %v", err)
	}
	if short.Points != 2.5 || short.QualifiedFlips != 3 {
		t.Errorf("the short session scored %v over %d flips, expected 2.5 over 3", short.Points, short.QualifiedFlips)
	}
	// The reported flip pays nothing in a short session, and the extra flip the
	// participant bought with two none answers does.
	if short.Considered[10] {
		t.Error("the reported flip counted in the short session")
	}
	for _, flipIndex := range []int{20, 21, 22} {
		if !short.Considered[flipIndex] {
			t.Errorf("flip %d did not count in the short session", flipIndex)
		}
	}

	longFlips := []int{10, 11, 12, 13, 20, 21, 22, 23, 24}
	long, err := ScoreCandidate(verdicts, longFlips, answers, false, notApproved)
	if err != nil {
		t.Fatalf("score the long session: %v", err)
	}
	if long.Points != 4.5 || long.QualifiedFlips != 6 {
		t.Errorf("the long session scored %v over %d flips, expected 4.5 over 6", long.Points, long.QualifiedFlips)
	}
	// A qualified flip counts whatever the participant answered, but only a
	// matching answer pays: flip 23 was answered left against a right verdict.
	if !long.Considered[23] {
		t.Error("a qualified flip answered the other way did not count")
	}
	if long.Considered[11] {
		t.Error("a not qualified flip counted")
	}

	voided := VoidedAnswers(len(longFlips))
	if voided.Points != 0 || voided.QualifiedFlips != 9 || len(voided.Considered) != 0 {
		t.Errorf("voided answers are %+v, expected zero points and the assignment's flips", voided)
	}
}

// TestScoreCandidateWithFewFlips covers the node's second vector: a short
// session that solved fewer flips than the session length.
func TestScoreCandidateWithFewFlips(t *testing.T) {
	verdicts := map[int]FlipVerdict{
		22: {Status: FlipQualified, Answer: types.Left, Grade: types.GradeD},
		55: {Status: FlipWeaklyQualified, Answer: types.Left, Grade: types.GradeD},
	}
	answers := []types.Answer{types.Left, types.Right}

	score, err := ScoreCandidate(verdicts, []int{22, 55}, answers, true, map[int]bool{})
	if err != nil {
		t.Fatalf("score: %v", err)
	}
	if score.Points != 1.5 || score.QualifiedFlips != 2 {
		t.Errorf("the session scored %v over %d flips, expected 1.5 over 2", score.Points, score.QualifiedFlips)
	}
}

// TestCandidateFlipStatusMatchesTheNodesVectors runs the assertions of the
// node's `Test_getFlipStatusForCandidate`. Its answer vector is three flips:
// right on 0, none on 1, left on 2.
func TestCandidateFlipStatusMatchesTheNodesVectors(t *testing.T) {
	answers := []types.Answer{types.Right, types.None, types.Left}
	notApproved := map[int]bool{5: true, 7: true}

	cases := []struct {
		flip     int
		position int
		base     FlipStatus
		short    bool
		expected FlipStatus
	}{
		{flip: 6, position: 0, base: FlipNotQualified, short: true, expected: FlipNotQualified},
		{flip: 6, position: 0, base: FlipQualified, short: true, expected: FlipQualified},
		{flip: 5, position: 0, base: FlipNotQualified, short: true, expected: FlipNotQualified},
		{flip: 5, position: 1, base: FlipNotQualified, short: true, expected: FlipNotQualified},
		{flip: 5, position: 2, base: FlipNotQualified, short: true, expected: FlipNotQualified},
		{flip: 5, position: 0, base: FlipQualified, short: true, expected: FlipQualified},
		{flip: 5, position: 1, base: FlipQualified, short: true, expected: FlipNotQualified},
		{flip: 5, position: 2, base: FlipQualified, short: true, expected: FlipQualified},
		{flip: 5, position: 1, base: FlipQualified, short: false, expected: FlipQualified},
	}
	for _, c := range cases {
		status := CandidateFlipStatus(c.flip, c.position, c.base, notApproved, answers, c.short)
		if status != c.expected {
			t.Errorf("flip %d position %d base %s short %v gave %s, expected %s",
				c.flip, c.position, c.base, c.short, status, c.expected)
		}
	}
}

// TestScoreCandidateRefusesShortAnswers keeps a participant that answered fewer
// flips than it was assigned from being scored as if it had answered them all.
func TestScoreCandidateRefusesShortAnswers(t *testing.T) {
	verdicts := candidateVerdicts()
	_, err := ScoreCandidate(verdicts, []int{10, 11, 12}, []types.Answer{types.Left}, false, map[int]bool{})
	if err == nil {
		t.Fatal("a participant that answered one flip of three was scored")
	}

	// No answers at all is a different outcome: it is the missing-attachment
	// case, not a refusal.
	score, err := ScoreCandidate(verdicts, []int{10}, nil, false, map[int]bool{})
	if err != nil {
		t.Fatalf("score a participant without answers: %v", err)
	}
	if !score.NoAnswers || score.Points != 0 || score.QualifiedFlips != 0 {
		t.Fatalf("a participant without answers is %+v, expected the no-answers outcome", score)
	}
}
