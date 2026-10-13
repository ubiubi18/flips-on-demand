//! The qualification rules of a closed epoch, as `SOURCES.md` F-61, F-64, and
//! F-66 record them: the flip verdict, and the eligibility policy this project
//! proves.
//!
//! This is the guest's copy of rules the CLI also implements in Go. The two are
//! kept together by `proofs/registration/vectors/qualification-v1.json`, which
//! both check against, rather than by one importing the other.
//!
//! The shares are compared as `f32` because that is the comparison the node
//! makes, and the boundaries are the point of the rule.

use alloc::{vec, vec::Vec};

/// The node's answer enum.
pub const ANSWER_NONE: u8 = 0;
pub const ANSWER_LEFT: u8 = 1;
pub const ANSWER_RIGHT: u8 = 2;

/// The node's grade enum.
pub const GRADE_NONE: u8 = 0;
pub const GRADE_REPORTED: u8 = 1;
pub const GRADE_D: u8 = 2;
pub const GRADE_C: u8 = 3;
pub const GRADE_B: u8 = 4;
pub const GRADE_A: u8 = 5;

/// The score of a grade, as the node's `Grade::Score` maps it.
pub const fn grade_score(grade: u8) -> i64 {
    match grade {
        GRADE_REPORTED => 0,
        GRADE_NONE => 1,
        GRADE_D => 2,
        GRADE_C => 4,
        GRADE_B => 6,
        GRADE_A => 8,
        _ => 0,
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FlipStatus {
    Qualified,
    WeaklyQualified,
    QualifiedByNone,
    NotQualified,
}

/// The switches that change how a flip is judged.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Rules {
    pub upgrade10: bool,
    pub upgrade11: bool,
}

/// What an epoch's answers and grades say about one flip.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct Committee {
    pub left: u32,
    pub right: u32,
    pub none: u32,
    pub reports: u32,
    pub total_grade_score: i64,
    pub approves: u32,
    pub report_committee_size: u32,
    pub grade_score_committee_size: u32,
}

/// One flip's verdict. The score is a rational because the upgrade-11 path
/// divides two integers exactly.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Verdict {
    pub status: FlipStatus,
    pub answer: u8,
    pub grade: u8,
    pub grade_score: (i64, i64),
}

impl Verdict {
    /// Compares a grade score with another rational, as the vector does.
    pub fn score_equals(&self, numerator: i64, denominator: i64) -> bool {
        self.grade_score.0 * denominator == numerator * self.grade_score.1
    }
}

/// The reported gate: a committee of zero or one never counts, two and three
/// need every vote, four needs three, five needs four, and anything larger needs
/// more than half.
pub fn reported_enough(reports: u32, committee: u32) -> bool {
    match committee {
        0 | 1 => false,
        2 | 3 => reports >= committee,
        4 => reports >= 3,
        5 => reports >= 4,
        _ => reports as f32 / committee as f32 > 0.5,
    }
}

/// The graded gate. Upgrade 10 turned the flat "more than a third" rule into
/// exact small-committee counts; upgrade 11 changed which committee the count is
/// measured against, not the counts themselves.
pub fn graded_enough(approves: u32, graded_committee_size: u32, upgrade10: bool) -> bool {
    if !upgrade10 {
        if graded_committee_size == 0 {
            return false;
        }
        return approves as f32 / graded_committee_size as f32 > 0.33;
    }
    match graded_committee_size {
        0 => false,
        1 => approves == 1,
        2 | 3 | 4 => approves >= 2,
        5 => approves >= 3,
        _ => approves as f32 / graded_committee_size as f32 > 0.33,
    }
}

/// A participant that reported on 34% or more of its own flips loses its reports
/// from every committee.
pub fn dropped_reports(reported: u32, assigned_flips: u32) -> bool {
    if assigned_flips == 0 {
        return false;
    }
    reported as f32 / assigned_flips as f32 >= 0.34
}

/// Rounds half away from zero, which is what the node's `math.Round` does. The
/// value is never negative here, and `no_std` has no `f64::round`.
fn round_half_away(value: f64) -> i64 {
    let truncated = value as i64;
    if value - truncated as f64 >= 0.5 {
        truncated + 1
    } else {
        truncated
    }
}

/// Applies the rules in the order F-61 records them: the reported gate first,
/// then the graded gate and the grade, then the answer shares.
pub fn qualify_flip(committee: &Committee, rules: Rules) -> Verdict {
    let graded_committee_size = if rules.upgrade11 {
        committee.grade_score_committee_size
    } else {
        committee.left + committee.right + committee.none
    };
    let reported = reported_enough(committee.reports, committee.report_committee_size);
    let graded = graded_enough(committee.approves, graded_committee_size, rules.upgrade10);

    let mut grade = GRADE_NONE;
    let mut score = (0i64, 1i64);
    if reported {
        grade = GRADE_REPORTED;
    } else if graded {
        if rules.upgrade11 {
            if committee.grade_score_committee_size != 0 {
                score = (
                    committee.total_grade_score,
                    committee.grade_score_committee_size as i64,
                );
            }
        } else {
            let mean = committee.total_grade_score as f64 / committee.approves as f64;
            grade = round_half_away(mean) as u8;
        }
    } else if rules.upgrade11 {
        score = (grade_score(GRADE_D), 1);
    } else {
        grade = GRADE_D;
    }

    let total = (committee.left + committee.right + committee.none) as f32;
    let (status, answer) = if total == 0.0 {
        (FlipStatus::NotQualified, ANSWER_NONE)
    } else if committee.left as f32 / total >= 0.75 {
        (FlipStatus::Qualified, ANSWER_LEFT)
    } else if committee.right as f32 / total >= 0.75 {
        (FlipStatus::Qualified, ANSWER_RIGHT)
    } else if committee.left as f32 / total >= 0.66 {
        (FlipStatus::WeaklyQualified, ANSWER_LEFT)
    } else if committee.right as f32 / total >= 0.66 {
        (FlipStatus::WeaklyQualified, ANSWER_RIGHT)
    } else if committee.none as f32 / total >= 0.66 {
        (FlipStatus::QualifiedByNone, ANSWER_NONE)
    } else {
        (FlipStatus::NotQualified, ANSWER_NONE)
    };

    Verdict {
        status,
        answer,
        grade,
        grade_score: score,
    }
}

/// One participant's long answers for an epoch: the flips it was assigned, in
/// assignment order, and the answer and grade it gave each. A participant whose
/// answers are absent or empty contributes to no committee, which is how the
/// node treats one whose long answers were void.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CandidateGrades {
    pub flips: Vec<u32>,
    pub answers: Vec<u8>,
    pub grades: Vec<u8>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AssemblyError {
    FlipOutOfRange(u32),
    LengthMismatch { candidate: usize },
}

/// The upgrade-11 tracker: every grade counts towards the committee size and the
/// score, including `GradeNone`, whose score is 1.
#[derive(Debug, Clone, Copy, Default)]
struct FlipTracker {
    total_score: i64,
    count: u32,
    approves: u32,
    reports: u32,
}

/// Assembles an epoch's per-flip committees from every participant's answers:
/// who answered what, who is dropped for reporting too much or for approving
/// wrongly, and what each flip's committee ends up being.
pub fn build_flip_committees(
    total_flips: usize,
    candidates: &[CandidateGrades],
    rules: Rules,
) -> Result<Vec<Committee>, AssemblyError> {
    let mut committees = vec![Committee::default(); total_flips];
    let mut trackers = vec![FlipTracker::default(); total_flips];

    for (index, candidate) in candidates.iter().enumerate() {
        if candidate.answers.is_empty() && candidate.grades.is_empty() {
            continue;
        }
        if candidate.answers.len() != candidate.flips.len()
            || candidate.grades.len() != candidate.flips.len()
        {
            return Err(AssemblyError::LengthMismatch { candidate: index });
        }

        let mut reported_flips: Vec<u32> = Vec::new();
        let mut has_approve = false;
        let mut increased_grades = 0u32;
        for (position, flip) in candidate.flips.iter().enumerate() {
            if *flip as usize >= total_flips {
                return Err(AssemblyError::FlipOutOfRange(*flip));
            }
            let answer = candidate.answers[position];
            let grade = candidate.grades[position];
            let committee = &mut committees[*flip as usize];
            match answer {
                ANSWER_LEFT => committee.left += 1,
                ANSWER_RIGHT => committee.right += 1,
                _ => committee.none += 1,
            }
            match grade {
                GRADE_NONE => {}
                GRADE_REPORTED => {
                    committee.reports += 1;
                    committee.report_committee_size += 1;
                    reported_flips.push(*flip);
                }
                _ => {
                    // Before upgrade 11 the mean is taken over the grade enum
                    // values, not over the scores.
                    committee.total_grade_score += grade as i64;
                    committee.approves += 1;
                    committee.report_committee_size += 1;
                    has_approve = true;
                    if grade > GRADE_D {
                        increased_grades += 1;
                    }
                }
            }
            if rules.upgrade11 {
                let tracker = &mut trackers[*flip as usize];
                tracker.count += 1;
                tracker.total_score += grade_score(grade);
                if grade >= GRADE_D {
                    tracker.approves += 1;
                }
                if grade == GRADE_REPORTED {
                    tracker.reports += 1;
                }
            }
        }

        let drop_reports =
            dropped_reports(reported_flips.len() as u32, candidate.flips.len() as u32);
        if !rules.upgrade11 {
            // Before upgrade 11 only the reports leave: the grades stay.
            if drop_reports {
                for flip in &reported_flips {
                    let committee = &mut committees[*flip as usize];
                    committee.reports -= 1;
                    committee.report_committee_size -= 1;
                }
            }
            continue;
        }

        // From upgrade 11 a participant's whole contribution leaves when it
        // reported too much, approved nothing, or approved more than one grade
        // above D.
        if drop_reports || !has_approve || increased_grades > 1 {
            for (position, flip) in candidate.flips.iter().enumerate() {
                let grade = candidate.grades[position];
                let tracker = &mut trackers[*flip as usize];
                tracker.count -= 1;
                tracker.total_score -= grade_score(grade);
                if grade >= GRADE_D {
                    tracker.approves -= 1;
                }
                if grade == GRADE_REPORTED {
                    tracker.reports -= 1;
                }
            }
        }
    }

    if rules.upgrade11 {
        // The upgrade-11 path reads its committee straight from the tracker, and
        // its reported gate counts the tracker's reporters.
        for (flip, committee) in committees.iter_mut().enumerate() {
            let tracker = trackers[flip];
            committee.reports = tracker.reports;
            committee.total_grade_score = tracker.total_score;
            committee.approves = tracker.approves;
            committee.grade_score_committee_size = tracker.count;
            committee.report_committee_size = tracker.approves + tracker.reports;
        }
    }
    Ok(committees)
}

/// The eligibility policy of `SPEC.md` 5.1, as `SOURCES.md` F-66 records it.
pub const ELIGIBILITY_POLICY_V1: &str = "fod:eligibility:v1";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ExclusionReason {
    NoVerdict,
    NotStrongConsensus,
    Reported,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Exclusion {
    pub flip: u32,
    pub reason: ExclusionReason,
}

/// One covered flip's reconstructed verdict.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct FlipVerdict {
    pub flip: u32,
    pub status: FlipStatus,
    pub answer: u8,
    pub grade: u8,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct EligibilityDecision {
    pub eligible: bool,
    pub exclusions: Vec<Exclusion>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EligibilityError {
    UnsupportedPolicy,
    EmptyBundle,
    DuplicateFlip(u32),
}

/// Decides whether a bundle's covered flips satisfy the policy, listing every
/// way a flip misses it, in covered order.
pub fn check_bundle_eligibility(
    policy: &str,
    verdicts: &[FlipVerdict],
    covered: &[u32],
) -> Result<EligibilityDecision, EligibilityError> {
    if policy != ELIGIBILITY_POLICY_V1 {
        return Err(EligibilityError::UnsupportedPolicy);
    }
    if covered.is_empty() {
        return Err(EligibilityError::EmptyBundle);
    }

    let mut sorted = covered.to_vec();
    sorted.sort_unstable();
    for pair in sorted.windows(2) {
        if pair[0] == pair[1] {
            return Err(EligibilityError::DuplicateFlip(pair[0]));
        }
    }

    let mut exclusions = Vec::new();
    for flip in &sorted {
        match verdicts.iter().find(|verdict| verdict.flip == *flip) {
            None => exclusions.push(Exclusion {
                flip: *flip,
                reason: ExclusionReason::NoVerdict,
            }),
            Some(verdict) => {
                if verdict.status != FlipStatus::Qualified {
                    exclusions.push(Exclusion {
                        flip: *flip,
                        reason: ExclusionReason::NotStrongConsensus,
                    });
                }
                if verdict.grade == GRADE_REPORTED {
                    exclusions.push(Exclusion {
                        flip: *flip,
                        reason: ExclusionReason::Reported,
                    });
                }
            }
        }
    }

    Ok(EligibilityDecision {
        eligible: exclusions.is_empty(),
        exclusions,
    })
}
