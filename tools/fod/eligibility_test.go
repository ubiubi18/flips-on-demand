package main

import (
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/types"
)

// eligibleVerdicts is a small epoch of qualified flips, which is what a bundle
// following the policy covers.
func eligibleVerdicts() map[int]FlipVerdict {
	return map[int]FlipVerdict{
		3: {Status: FlipQualified, Answer: types.Left, Grade: types.GradeD},
		7: {Status: FlipQualified, Answer: types.Right, Grade: types.GradeC},
		9: {Status: FlipQualified, Answer: types.Left, Grade: types.GradeB},
	}
}

// TestEligibleBundlePassesThePolicy is the case a seller wants: every covered
// flip qualified with no reported grade.
func TestEligibleBundlePassesThePolicy(t *testing.T) {
	decision, err := CheckBundleEligibility(EligibilityPolicyV1, eligibleVerdicts(), []int{9, 3, 7})
	if err != nil {
		t.Fatalf("check the bundle: %v", err)
	}
	if !decision.Eligible || len(decision.Exclusions) != 0 {
		t.Fatalf("the bundle is %+v, expected it to be eligible", decision)
	}
	if decision.Policy != EligibilityPolicyV1 {
		t.Fatalf("the decision names policy %q", decision.Policy)
	}
	// Covered comes back sorted, so a manifest can commit to it.
	if len(decision.Covered) != 3 || decision.Covered[0] != 3 || decision.Covered[1] != 7 || decision.Covered[2] != 9 {
		t.Fatalf("covered flips are %v, expected 3 7 9", decision.Covered)
	}
}

// TestIneligibleBundlesAreRefusedWithReasons covers every way the policy says
// no, including a flip that fails for both reasons at once.
func TestIneligibleBundlesAreRefusedWithReasons(t *testing.T) {
	verdicts := map[int]FlipVerdict{
		1: {Status: FlipWeaklyQualified, Answer: types.Left, Grade: types.GradeD},
		2: {Status: FlipQualifiedByNone, Answer: types.None, Grade: types.GradeD},
		3: {Status: FlipQualified, Answer: types.Left, Grade: types.GradeReported},
		4: {Status: FlipNotQualified, Answer: types.None, Grade: types.GradeReported},
	}
	cases := []struct {
		flip     int
		reasons  []string
		strongOK bool
	}{
		{flip: 1, reasons: []string{ReasonNotStrong}},
		{flip: 2, reasons: []string{ReasonNotStrong}},
		{flip: 3, reasons: []string{ReasonReported}},
		{flip: 4, reasons: []string{ReasonNotStrong, ReasonReported}},
		{flip: 8, reasons: []string{ReasonNoVerdict}},
	}
	for _, c := range cases {
		decision, err := CheckBundleEligibility(EligibilityPolicyV1, verdicts, []int{c.flip})
		if err != nil {
			t.Fatalf("check flip %d: %v", c.flip, err)
		}
		if decision.Eligible {
			t.Errorf("flip %d made the bundle eligible", c.flip)
		}
		if len(decision.Exclusions) != len(c.reasons) {
			t.Fatalf("flip %d has exclusions %+v, expected %v", c.flip, decision.Exclusions, c.reasons)
		}
		for index, reason := range c.reasons {
			if decision.Exclusions[index].Reason != reason || decision.Exclusions[index].FlipIndex != c.flip {
				t.Errorf("flip %d exclusion %d is %+v, expected %s", c.flip, index, decision.Exclusions[index], reason)
			}
		}
	}
}

// TestEligibilityRefusesBadInput keeps the policy from answering a question it
// cannot answer: an unsupported policy version, an empty bundle, and a repeated
// flip are all refusals rather than decisions.
func TestEligibilityRefusesBadInput(t *testing.T) {
	verdicts := eligibleVerdicts()

	if _, err := CheckBundleEligibility("fod:eligibility:v9", verdicts, []int{3}); err == nil {
		t.Error("an unsupported policy version was answered instead of refused")
	} else if !strings.Contains(err.Error(), EligibilityPolicyV1) {
		t.Errorf("the refusal does not name the supported policy: %v", err)
	}
	if _, err := CheckBundleEligibility(EligibilityPolicyV1, verdicts, nil); err == nil {
		t.Error("a bundle covering no flips was accepted")
	}
	if _, err := CheckBundleEligibility(EligibilityPolicyV1, verdicts, []int{3, 3}); err == nil {
		t.Error("a bundle covering one flip twice was accepted")
	}
}
