package main

// The eligibility policy of `SPEC.md` 5.1, 5.3, and 5.5: which covered flips a
// bundle may include before a registration proof will claim them. This is this
// project's own rule rather than a mirror of a node rule, so it carries its own
// version string for the manifest's `eligibility-policy version` field.
//
// The input is the epoch's reconstructed verdicts for every flip the epoch
// assigned. The policy never takes a participant-supplied subset of votes:
// `SPEC.md` 5.3 forbids that, and this function has no parameter that could
// carry one.

import (
	"fmt"
	"sort"

	"github.com/idena-network/idena-go/blockchain/types"
)

// EligibilityPolicyV1 is what this release proves: every covered flip has
// strong consensus, which the reconstructed statuses spell as `qualified` rather
// than `weaklyQualified` or `qualifiedByNone`, and carries no reported grade.
const EligibilityPolicyV1 = "fod:eligibility:v1"

// Exclusion reasons, one per way a covered flip fails the policy.
const (
	// ReasonNoVerdict: the epoch has no verdict for the flip at all.
	ReasonNoVerdict = "no_verdict"
	// ReasonNotStrong: the flip was not qualified, so its consensus was not
	// strong enough for this policy.
	ReasonNotStrong = "not_strong_consensus"
	// ReasonReported: the flip carries a reported grade.
	ReasonReported = "reported"
)

// FlipExclusion is one covered flip that fails the policy, with the first
// reason it failed.
type FlipExclusion struct {
	FlipIndex int    `json:"flip"`
	Reason    string `json:"reason"`
}

// EligibilityVerdict is the policy's decision for one bundle. Covered is sorted
// so the decision is the same however the caller listed the flips, which is what
// lets a manifest commit to it.
type EligibilityVerdict struct {
	Policy     string          `json:"policy"`
	Covered    []int           `json:"covered"`
	Exclusions []FlipExclusion `json:"exclusions"`
	Eligible   bool            `json:"eligible"`
}

// CheckBundleEligibility applies the policy to a bundle's covered flips. A flip
// can fail for both reasons and is recorded twice, because an author reading the
// decision should see every way the bundle misses the policy, not the first one.
func CheckBundleEligibility(policy string, verdicts map[int]FlipVerdict, coveredFlips []int) (EligibilityVerdict, error) {
	if policy != EligibilityPolicyV1 {
		return EligibilityVerdict{}, fmt.Errorf("unsupported eligibility policy %q; this release proves %q", policy, EligibilityPolicyV1)
	}
	if len(coveredFlips) == 0 {
		return EligibilityVerdict{}, fmt.Errorf("the bundle covers no flips")
	}

	covered := make([]int, 0, len(coveredFlips))
	seen := make(map[int]bool, len(coveredFlips))
	for _, flipIndex := range coveredFlips {
		if seen[flipIndex] {
			return EligibilityVerdict{}, fmt.Errorf("flip %d is covered twice", flipIndex)
		}
		seen[flipIndex] = true
		covered = append(covered, flipIndex)
	}
	sort.Ints(covered)

	decision := EligibilityVerdict{Policy: policy, Covered: covered}
	for _, flipIndex := range covered {
		flip, ok := verdicts[flipIndex]
		if !ok {
			decision.Exclusions = append(decision.Exclusions, FlipExclusion{FlipIndex: flipIndex, Reason: ReasonNoVerdict})
			continue
		}
		if flip.Status != FlipQualified {
			decision.Exclusions = append(decision.Exclusions, FlipExclusion{FlipIndex: flipIndex, Reason: ReasonNotStrong})
		}
		if flip.Grade == types.GradeReported {
			decision.Exclusions = append(decision.Exclusions, FlipExclusion{FlipIndex: flipIndex, Reason: ReasonReported})
		}
	}
	decision.Eligible = len(decision.Exclusions) == 0
	return decision, nil
}
