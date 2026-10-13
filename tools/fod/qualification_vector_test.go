package main

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/idena-network/idena-go/blockchain/types"
)

type committeeJSON struct {
	Left                    int `json:"left"`
	Right                   int `json:"right"`
	None                    int `json:"none"`
	Reports                 int `json:"reports"`
	TotalGradeScore         int `json:"totalGradeScore"`
	Approves                int `json:"approves"`
	ReportCommitteeSize     int `json:"reportCommitteeSize"`
	GradeScoreCommitteeSize int `json:"gradeScoreCommitteeSize"`
}

type candidateJSON struct {
	Flips   []int `json:"flips"`
	Answers []int `json:"answers"`
	Grades  []int `json:"grades"`
}

type epochJSON struct {
	Name         string          `json:"name"`
	TotalFlips   int             `json:"totalFlips"`
	Candidates   []candidateJSON `json:"candidates"`
	PreUpgrade11 []committeeJSON `json:"preUpgrade11"`
	Upgrade11    []committeeJSON `json:"upgrade11"`
}

type qualificationVector struct {
	Note         string `json:"note"`
	FlipVerdicts []struct {
		Name      string        `json:"name"`
		Committee committeeJSON `json:"committee"`
		Expected  struct {
			Status     string `json:"status"`
			Answer     int    `json:"answer"`
			Grade      int    `json:"grade"`
			GradeScore string `json:"gradeScore"`
		} `json:"expected"`
	} `json:"flipVerdicts"`
	Eligibility []struct {
		Name     string `json:"name"`
		Verdicts []struct {
			Flip   int    `json:"flip"`
			Status string `json:"status"`
			Answer int    `json:"answer"`
			Grade  int    `json:"grade"`
		} `json:"verdicts"`
		Covered  []int `json:"covered"`
		Expected struct {
			Eligible   bool `json:"eligible"`
			Exclusions []struct {
				Flip   int    `json:"flip"`
				Reason string `json:"reason"`
			} `json:"exclusions"`
		} `json:"expected"`
	} `json:"eligibility"`
	Epochs []epochJSON `json:"epochs"`
}

func readQualificationVector(t *testing.T) qualificationVector {
	t.Helper()
	raw, err := os.ReadFile("../../proofs/registration/vectors/qualification-v1.json")
	if err != nil {
		t.Fatalf("read the shared qualification vector: %v", err)
	}
	var vector qualificationVector
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatalf("parse the shared qualification vector: %v", err)
	}
	return vector
}

func answerList(values []int) []types.Answer {
	out := make([]types.Answer, 0, len(values))
	for _, value := range values {
		out = append(out, types.Answer(value))
	}
	return out
}

func gradeList(values []int) []types.Grade {
	out := make([]types.Grade, 0, len(values))
	for _, value := range values {
		out = append(out, types.Grade(value))
	}
	return out
}

func (c committeeJSON) matches(committee FlipCommittee) bool {
	left, right, none := AnswerCounts(committee.Answers)
	return c.Left == left && c.Right == right && c.None == none &&
		c.Reports == committee.Reports &&
		c.TotalGradeScore == committee.TotalGradeScore &&
		c.Approves == committee.Approves &&
		c.ReportCommitteeSize == committee.ReportCommitteeSize &&
		c.GradeScoreCommitteeSize == committee.GradeScoreCommitteeSize
}

// TestTheCliMatchesTheQualificationVector is the CLI's side of the shared
// vector. The verdict cases are the pinned node's own, so this checks the
// reconstruction against the node rather than against itself.
func TestTheCliMatchesTheQualificationVector(t *testing.T) {
	vector := readQualificationVector(t)
	if len(vector.FlipVerdicts) != 15 {
		t.Fatalf("the vector carries %d flip verdict cases, expected 15", len(vector.FlipVerdicts))
	}
	rules := ConsensusRules{Upgrade10: true, Upgrade11: true}
	for _, c := range vector.FlipVerdicts {
		committee := FlipCommittee{
			Answers:                 answersFixture(c.Committee.Left, c.Committee.Right, c.Committee.None),
			Reports:                 c.Committee.Reports,
			TotalGradeScore:         c.Committee.TotalGradeScore,
			Approves:                c.Committee.Approves,
			ReportCommitteeSize:     c.Committee.ReportCommitteeSize,
			GradeScoreCommitteeSize: c.Committee.GradeScoreCommitteeSize,
		}
		verdict := QualifyFlip(committee, rules)
		if string(verdict.Status) != c.Expected.Status {
			t.Errorf("%s: status is %s, expected %s", c.Name, verdict.Status, c.Expected.Status)
		}
		if int(verdict.Answer) != c.Expected.Answer {
			t.Errorf("%s: answer is %d, expected %d", c.Name, verdict.Answer, c.Expected.Answer)
		}
		if int(verdict.Grade) != c.Expected.Grade {
			t.Errorf("%s: grade is %d, expected %d", c.Name, verdict.Grade, c.Expected.Grade)
		}
		expected, ok := new(big.Rat).SetString(c.Expected.GradeScore)
		if !ok {
			t.Fatalf("%s: the vector's score %q is not a rational", c.Name, c.Expected.GradeScore)
		}
		if verdict.GradeScore.Cmp(expected) != 0 {
			t.Errorf("%s: grade score is %s, expected %s", c.Name, verdict.GradeScore.RatString(), expected.RatString())
		}
	}
}

// TestTheCliMatchesTheEligibilityVector checks the policy decisions on the same
// file the registration core reads.
func TestTheCliMatchesTheEligibilityVector(t *testing.T) {
	vector := readQualificationVector(t)
	if len(vector.Eligibility) != 6 {
		t.Fatalf("the vector carries %d eligibility cases, expected 6", len(vector.Eligibility))
	}
	for _, c := range vector.Eligibility {
		verdicts := make(map[int]FlipVerdict, len(c.Verdicts))
		for _, v := range c.Verdicts {
			verdicts[v.Flip] = FlipVerdict{
				Status: FlipStatus(v.Status),
				Answer: types.Answer(v.Answer),
				Grade:  types.Grade(v.Grade),
			}
		}
		decision, err := CheckBundleEligibility(EligibilityPolicyV1, verdicts, c.Covered)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if decision.Eligible != c.Expected.Eligible {
			t.Errorf("%s: eligible is %v, expected %v", c.Name, decision.Eligible, c.Expected.Eligible)
		}
		if len(decision.Exclusions) != len(c.Expected.Exclusions) {
			t.Fatalf("%s: %d exclusions, expected %d", c.Name, len(decision.Exclusions), len(c.Expected.Exclusions))
		}
		for index, exclusion := range c.Expected.Exclusions {
			if decision.Exclusions[index].FlipIndex != exclusion.Flip || decision.Exclusions[index].Reason != exclusion.Reason {
				t.Errorf("%s: exclusion %d is %+v, expected %d/%s", c.Name, index, decision.Exclusions[index], exclusion.Flip, exclusion.Reason)
			}
		}
	}
}

// TestTheCliAssemblesTheVectorsEpochs runs the same epoch fixtures the
// registration core reads through `BuildFlipCommittees`, for both paths.
func TestTheCliAssemblesTheVectorsEpochs(t *testing.T) {
	vector := readQualificationVector(t)
	if len(vector.Epochs) != 6 {
		t.Fatalf("the vector carries %d epoch fixtures, expected 6", len(vector.Epochs))
	}
	paths := []struct {
		name  string
		rules ConsensusRules
		want  func(epochJSON) []committeeJSON
	}{
		{
			name:  "preUpgrade11",
			rules: ConsensusRules{Upgrade10: true},
			want:  func(epoch epochJSON) []committeeJSON { return epoch.PreUpgrade11 },
		},
		{
			name:  "upgrade11",
			rules: ConsensusRules{Upgrade10: true, Upgrade11: true},
			want:  func(epoch epochJSON) []committeeJSON { return epoch.Upgrade11 },
		},
	}

	for _, epoch := range vector.Epochs {
		flipsPerCandidate := make([][]int, 0, len(epoch.Candidates))
		candidates := make([]CandidateGrades, 0, len(epoch.Candidates))
		for _, candidate := range epoch.Candidates {
			flipsPerCandidate = append(flipsPerCandidate, candidate.Flips)
			candidates = append(candidates, CandidateGrades{
				Answers: answerList(candidate.Answers),
				Grades:  gradeList(candidate.Grades),
			})
		}
		for _, path := range paths {
			built, err := BuildFlipCommittees(epoch.TotalFlips, flipsPerCandidate, candidates, path.rules)
			if err != nil {
				t.Fatalf("%s (%s): %v", epoch.Name, path.name, err)
			}
			expected := path.want(epoch)
			if len(built) != len(expected) {
				t.Fatalf("%s (%s): %d committees, expected %d", epoch.Name, path.name, len(built), len(expected))
			}
			for index, want := range expected {
				if !want.matches(built[index]) {
					t.Errorf("%s (%s): committee %d is %+v, expected %+v",
						epoch.Name, path.name, index, built[index], want)
				}
			}
		}
	}
}
