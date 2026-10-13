package main

import (
	"fmt"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/blockchain/types"
)

// AnswersSummary is the decoded shape of one participant's long answers.
//
// Encoding (reference: blockchain/types/types.go NewAnswersFromBits, Answer,
// determineGrade): for a candidate assigned n flips, the bit string holds
//   bits [0, n)      left flags
//   bits [n, 2n)     right flags
//   bits [2n, 5n)    three bits of grade per flip
// and the grades are None/Reported/D/C/B/A = 0..5.
type AnswersSummary struct {
	TxHash        string   `json:"txHash"`
	Author        string   `json:"author"`
	AnswerBytes   int      `json:"answerBytes"`
	ProofBytes    int      `json:"proofBytes"`
	KeyBytes      int      `json:"keyBytes"`
	SaltBytes     int      `json:"saltBytes"`
	InferredFlips int      `json:"inferredFlips"`
	Left          int      `json:"left"`
	Right         int      `json:"right"`
	None          int      `json:"none"`
	GradeReported int      `json:"gradeReported"`
	GradeD        int      `json:"gradeD"`
	GradeC        int      `json:"gradeC"`
	GradeB        int      `json:"gradeB"`
	GradeA        int      `json:"gradeA"`
	Sample        []string `json:"sample"`
}

// decodeLongAnswers parses a submitLongAnswers payload.
//
// The flip count is not carried in the payload; it comes from the candidate's
// lottery assignment. Until that assignment is implemented, the count is
// inferred from the payload size (5 bits per flip), which is exact only when
// 5n is a multiple of 8, so the reported counts are provisional.
func decodeLongAnswers(tx *types.Transaction) (*AnswersSummary, error) {
	attachment := attachments.ParseLongAnswerAttachment(tx)
	if attachment == nil {
		return nil, fmt.Errorf("not a long-answer payload")
	}
	summary := &AnswersSummary{
		TxHash:      tx.Hash().String(),
		AnswerBytes: len(attachment.Answers),
		ProofBytes:  len(attachment.Proof),
		KeyBytes:    len(attachment.Key),
		SaltBytes:   len(attachment.Salt),
	}
	if address, err := types.Sender(tx); err == nil {
		summary.Author = address.String()
	}
	if len(attachment.Answers) == 0 {
		return nil, fmt.Errorf("empty answers")
	}

	flips := uint(len(attachment.Answers) * 8 / 5)
	if flips == 0 {
		return nil, fmt.Errorf("answers too short")
	}
	summary.InferredFlips = int(flips)

	answers := types.NewAnswersFromBits(flips, attachment.Answers)
	for i := uint(0); i < flips; i++ {
		answer, grade := answers.Answer(i)
		switch answer {
		case types.Left:
			summary.Left++
		case types.Right:
			summary.Right++
		default:
			summary.None++
		}
		switch grade {
		case types.GradeReported:
			summary.GradeReported++
		case types.GradeD:
			summary.GradeD++
		case types.GradeC:
			summary.GradeC++
		case types.GradeB:
			summary.GradeB++
		case types.GradeA:
			summary.GradeA++
		}
		if i < 8 {
			summary.Sample = append(summary.Sample, fmt.Sprintf("%s/%s", answerName(answer), gradeName(grade)))
		}
	}
	return summary, nil
}

func answerName(a types.Answer) string {
	switch a {
	case types.Left:
		return "left"
	case types.Right:
		return "right"
	default:
		return "none"
	}
}

func gradeName(g types.Grade) string {
	switch g {
	case types.GradeReported:
		return "reported"
	case types.GradeD:
		return "D"
	case types.GradeC:
		return "C"
	case types.GradeB:
		return "B"
	case types.GradeA:
		return "A"
	default:
		return "none"
	}
}
