//! The registration side of the shared qualification vector: the verdicts the
//! pinned node's own tests pin, and this project's eligibility policy.

use fod_registration_core::qualification as q;
use serde_json::Value;

fn vector() -> Value {
    let path =
        std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../vectors/qualification-v1.json");
    let raw = std::fs::read_to_string(&path).expect("read the shared vector");
    serde_json::from_str(&raw).expect("parse the shared vector")
}

fn number(value: &Value, name: &str) -> u64 {
    value[name]
        .as_u64()
        .unwrap_or_else(|| panic!("{name} is not a number"))
}

fn status(name: &str) -> q::FlipStatus {
    match name {
        "qualified" => q::FlipStatus::Qualified,
        "weaklyQualified" => q::FlipStatus::WeaklyQualified,
        "qualifiedByNone" => q::FlipStatus::QualifiedByNone,
        "notQualified" => q::FlipStatus::NotQualified,
        other => panic!("unknown status {other}"),
    }
}

fn reason(name: &str) -> q::ExclusionReason {
    match name {
        "no_verdict" => q::ExclusionReason::NoVerdict,
        "not_strong_consensus" => q::ExclusionReason::NotStrongConsensus,
        "reported" => q::ExclusionReason::Reported,
        other => panic!("unknown exclusion reason {other}"),
    }
}

fn rational(value: &str) -> (i64, i64) {
    let (numerator, denominator) = value.split_once('/').expect("a rational");
    (
        numerator.parse().expect("numerator"),
        denominator.parse().expect("denominator"),
    )
}

#[test]
fn the_registration_verdict_matches_the_nodes_vectors() {
    let vector = vector();
    let cases = vector["flipVerdicts"].as_array().expect("flipVerdicts");
    assert_eq!(cases.len(), 15, "the vector lost its flip verdict cases");

    for case in cases {
        let name = case["name"].as_str().expect("name");
        let committee = &case["committee"];
        let built = q::Committee {
            left: number(committee, "left") as u32,
            right: number(committee, "right") as u32,
            none: number(committee, "none") as u32,
            reports: number(committee, "reports") as u32,
            total_grade_score: number(committee, "totalGradeScore") as i64,
            approves: number(committee, "approves") as u32,
            report_committee_size: number(committee, "reportCommitteeSize") as u32,
            grade_score_committee_size: number(committee, "gradeScoreCommitteeSize") as u32,
        };
        let expected = &case["expected"];
        let verdict = q::qualify_flip(
            &built,
            q::Rules {
                upgrade10: true,
                upgrade11: true,
            },
        );

        assert_eq!(
            verdict.status,
            status(expected["status"].as_str().expect("status")),
            "{name}: status"
        );
        assert_eq!(
            verdict.answer,
            number(expected, "answer") as u8,
            "{name}: answer"
        );
        assert_eq!(
            verdict.grade,
            number(expected, "grade") as u8,
            "{name}: grade"
        );
        let (numerator, denominator) = rational(expected["gradeScore"].as_str().expect("score"));
        assert!(
            verdict.score_equals(numerator, denominator),
            "{name}: grade score is {}/{}, expected {numerator}/{denominator}",
            verdict.grade_score.0,
            verdict.grade_score.1
        );
    }
}

#[test]
fn the_registration_gates_have_the_nodes_boundaries() {
    assert!(!q::reported_enough(1, 1));
    assert!(!q::reported_enough(2, 3));
    assert!(q::reported_enough(3, 3));
    assert!(!q::reported_enough(2, 4));
    assert!(q::reported_enough(3, 4));
    assert!(!q::reported_enough(3, 5));
    assert!(q::reported_enough(4, 5));
    assert!(!q::reported_enough(5, 10));
    assert!(q::reported_enough(6, 10));

    assert!(!q::graded_enough(0, 0, true));
    assert!(q::graded_enough(1, 1, true));
    assert!(!q::graded_enough(1, 4, true));
    assert!(q::graded_enough(2, 4, true));
    assert!(!q::graded_enough(2, 5, true));
    assert!(q::graded_enough(3, 5, true));
    assert!(!q::graded_enough(3, 10, true));
    assert!(q::graded_enough(4, 10, true));
    assert!(q::graded_enough(1, 1, false));
    assert!(!q::graded_enough(0, 0, false));

    assert!(!q::dropped_reports(0, 0));
    assert!(!q::dropped_reports(33, 100));
    assert!(q::dropped_reports(34, 100));
    assert!(q::dropped_reports(1, 2));
}

#[test]
fn the_registration_eligibility_matches_the_vector() {
    let vector = vector();
    let cases = vector["eligibility"].as_array().expect("eligibility");
    assert_eq!(cases.len(), 6, "the vector lost its eligibility cases");

    for case in cases {
        let name = case["name"].as_str().expect("name");
        let verdicts: Vec<q::FlipVerdict> = case["verdicts"]
            .as_array()
            .expect("verdicts")
            .iter()
            .map(|verdict| q::FlipVerdict {
                flip: number(verdict, "flip") as u32,
                status: status(verdict["status"].as_str().expect("status")),
                answer: number(verdict, "answer") as u8,
                grade: number(verdict, "grade") as u8,
            })
            .collect();
        let covered: Vec<u32> = case["covered"]
            .as_array()
            .expect("covered")
            .iter()
            .map(|flip| flip.as_u64().expect("flip") as u32)
            .collect();

        let decision = q::check_bundle_eligibility(q::ELIGIBILITY_POLICY_V1, &verdicts, &covered)
            .expect("the vector's bundle is decidable");
        let expected = &case["expected"];
        assert_eq!(
            decision.eligible,
            expected["eligible"].as_bool().expect("eligible"),
            "{name}: eligible"
        );

        let expected_exclusions: Vec<q::Exclusion> = expected["exclusions"]
            .as_array()
            .expect("exclusions")
            .iter()
            .map(|exclusion| q::Exclusion {
                flip: number(exclusion, "flip") as u32,
                reason: reason(exclusion["reason"].as_str().expect("reason")),
            })
            .collect();
        assert_eq!(
            decision.exclusions, expected_exclusions,
            "{name}: exclusions"
        );
    }
}

#[test]
fn the_registration_policy_refuses_bad_input() {
    let verdicts = [q::FlipVerdict {
        flip: 3,
        status: q::FlipStatus::Qualified,
        answer: q::ANSWER_LEFT,
        grade: q::GRADE_D,
    }];
    assert_eq!(
        q::check_bundle_eligibility("fod:eligibility:v9", &verdicts, &[3]),
        Err(q::EligibilityError::UnsupportedPolicy)
    );
    assert_eq!(
        q::check_bundle_eligibility(q::ELIGIBILITY_POLICY_V1, &verdicts, &[]),
        Err(q::EligibilityError::EmptyBundle)
    );
    assert_eq!(
        q::check_bundle_eligibility(q::ELIGIBILITY_POLICY_V1, &verdicts, &[3, 3]),
        Err(q::EligibilityError::DuplicateFlip(3))
    );
}

fn committee_from(value: &Value) -> q::Committee {
    q::Committee {
        left: number(value, "left") as u32,
        right: number(value, "right") as u32,
        none: number(value, "none") as u32,
        reports: number(value, "reports") as u32,
        total_grade_score: number(value, "totalGradeScore") as i64,
        approves: number(value, "approves") as u32,
        report_committee_size: number(value, "reportCommitteeSize") as u32,
        grade_score_committee_size: number(value, "gradeScoreCommitteeSize") as u32,
    }
}

fn candidate_from(value: &Value) -> q::CandidateGrades {
    let list = |name: &str| -> Vec<u64> {
        value[name]
            .as_array()
            .unwrap_or_else(|| panic!("{name} is not an array"))
            .iter()
            .map(|entry| entry.as_u64().expect("a number"))
            .collect()
    };
    q::CandidateGrades {
        flips: list("flips").into_iter().map(|flip| flip as u32).collect(),
        answers: list("answers")
            .into_iter()
            .map(|answer| answer as u8)
            .collect(),
        grades: list("grades")
            .into_iter()
            .map(|grade| grade as u8)
            .collect(),
    }
}

// TestTheRegistrationAssemblesTheVectorsEpochs runs the epoch fixtures through
// the assembly for both paths. The fixtures are the ones the CLI's own tests
// pin: the tracker's accounting, and the three upgrade-11 drops.
#[test]
fn the_registration_assembles_the_vectors_epochs() {
    let vector = vector();
    let epochs = vector["epochs"].as_array().expect("epochs");
    assert_eq!(epochs.len(), 6, "the vector lost its epoch fixtures");

    for epoch in epochs {
        let name = epoch["name"].as_str().expect("name");
        let total_flips = number(epoch, "totalFlips") as usize;
        let candidates: Vec<q::CandidateGrades> = epoch["candidates"]
            .as_array()
            .expect("candidates")
            .iter()
            .map(candidate_from)
            .collect();

        for (path, rules) in [
            (
                "preUpgrade11",
                q::Rules {
                    upgrade10: true,
                    upgrade11: false,
                },
            ),
            (
                "upgrade11",
                q::Rules {
                    upgrade10: true,
                    upgrade11: true,
                },
            ),
        ] {
            let built = q::build_flip_committees(total_flips, &candidates, rules)
                .unwrap_or_else(|error| panic!("{name} ({path}): {error:?}"));
            let expected = epoch[path].as_array().expect("committees");
            assert_eq!(
                built.len(),
                expected.len(),
                "{name} ({path}): committee count"
            );
            for (index, want) in expected.iter().enumerate() {
                assert_eq!(
                    built[index],
                    committee_from(want),
                    "{name} ({path}): committee {index}"
                );
            }
        }
    }
}

#[test]
fn the_registration_assembly_refuses_bad_input() {
    let candidate = |flips: Vec<u32>, answers: Vec<u8>, grades: Vec<u8>| q::CandidateGrades {
        flips,
        answers,
        grades,
    };
    let rules = q::Rules {
        upgrade10: true,
        upgrade11: true,
    };

    assert_eq!(
        q::build_flip_committees(1, &[candidate(vec![1], vec![1], vec![2])], rules),
        Err(q::AssemblyError::FlipOutOfRange(1))
    );
    assert_eq!(
        q::build_flip_committees(2, &[candidate(vec![0, 1], vec![1], vec![2, 2])], rules),
        Err(q::AssemblyError::LengthMismatch { candidate: 0 })
    );
    // A participant with no answers at all contributes nothing and is not an
    // error, in either path.
    for rules in [
        q::Rules {
            upgrade10: true,
            upgrade11: false,
        },
        q::Rules {
            upgrade10: true,
            upgrade11: true,
        },
    ] {
        let built = q::build_flip_committees(1, &[candidate(vec![0], vec![], vec![])], rules)
            .expect("a void participant");
        assert_eq!(built, vec![q::Committee::default()]);
    }
}
