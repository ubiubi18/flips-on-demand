package main

// Long answers have two sources - the capture's epoch store and the epoch's own
// transactions - and the reconstruction takes both, refusing a disagreement.
// These tests drive that with payloads the reference attachment type produced,
// so what travels through the merge is the wire form, not a stand-in.

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idena-network/idena-go/blockchain/attachments"
	"github.com/idena-network/idena-go/common"
)

// answerAsAddress is the parse the sweep's author strings go through.
func answerAsAddress(t *testing.T, value string) common.Address {
	t.Helper()
	address, err := answerAddress(value)
	if err != nil {
		t.Fatalf("parse %s: %v", value, err)
	}
	return address
}

// longAnswerPayloadFor builds one candidate's long-answer payload with the
// reference attachment type.
func longAnswerPayloadFor(t *testing.T, bits []byte) []byte {
	t.Helper()
	payload, err := (&attachments.LongAnswerAttachment{
		Answers: bits,
		Proof:   []byte{0x01, 0x02},
		Key:     []byte{0x03},
		Salt:    []byte{0x04, 0x05},
	}).ToBytes()
	if err != nil {
		t.Fatalf("build a long-answer payload: %v", err)
	}
	return payload
}

func TestAnswersFromSweepReadsOnlyTheEpochsOwnAnswers(t *testing.T) {
	dir := t.TempDir()
	payload := longAnswerPayloadFor(t, []byte{0xa5, 0x11})
	sweep := SweepResult{
		Answers: []LongAnswerTx{
			{TxHash: "0x1", BlockHeight: 100, Author: "0x" + strings.Repeat("aa", 20), Epoch: 204, PayloadHex: hex.EncodeToString(payload)},
			// Another epoch's answer is not this reconstruction's business.
			{TxHash: "0x2", BlockHeight: 200, Author: "0x" + strings.Repeat("bb", 20), Epoch: 205, PayloadHex: hex.EncodeToString(payload)},
			// The same author twice: the first submission is the one that counts.
			{TxHash: "0x3", BlockHeight: 101, Author: "0x" + strings.Repeat("aa", 20), Epoch: 204, PayloadHex: hex.EncodeToString(longAnswerPayloadFor(t, []byte{0xff}))},
			// A payload the reference parser does not accept never reaches a sweep.
			{TxHash: "0x4", BlockHeight: 102, Author: "0x" + strings.Repeat("cc", 20), Epoch: 204, PayloadHex: ""},
		},
	}
	path := filepath.Join(dir, "sweep.json")
	if err := writeJSON(path, &sweep); err != nil {
		t.Fatalf("write the sweep: %v", err)
	}

	answers, evidence, err := answersFromSweep(path, 204)
	if err != nil {
		t.Fatalf("read the sweep's answers: %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("the sweep supplied %d answers for epoch 204, expected 1: %v", len(answers), answers)
	}
	if got := hex.EncodeToString(answers[answerAsAddress(t, "0x"+strings.Repeat("aa", 20))]); got != hex.EncodeToString(payload) {
		t.Fatalf("the first submission did not win: %s", got)
	}
	if evidence.FromChain != 1 || evidence.Duplicates != 1 {
		t.Fatalf("the evidence does not record what the sweep held: %+v", evidence)
	}
}

func TestMergeAnswersTakesBothSourcesAndRefusesADisagreement(t *testing.T) {
	one := answerAsAddress(t, "0x"+strings.Repeat("aa", 20))
	two := answerAsAddress(t, "0x"+strings.Repeat("bb", 20))
	first := longAnswerPayloadFor(t, []byte{0x01})
	second := longAnswerPayloadFor(t, []byte{0x02})

	merged, evidence, err := mergeAnswers(
		map[common.Address][]byte{one: first},
		map[common.Address][]byte{one: first, two: second},
	)
	if err != nil {
		t.Fatalf("two sources that agree were refused: %v", err)
	}
	if len(merged) != 2 || evidence.Agreed != 1 || evidence.StoreOnly != 0 || evidence.ChainOnly != 1 {
		t.Fatalf("the merge did not take both sources: %d answers, %+v", len(merged), evidence)
	}
	if !strings.Contains(evidence.Note, "agree") {
		t.Fatalf("the note does not say the sources agreed: %s", evidence.Note)
	}

	if _, _, err := mergeAnswers(
		map[common.Address][]byte{one: first},
		map[common.Address][]byte{one: longAnswerPayloadFor(t, []byte{0x03})},
	); err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("a disagreement between the capture and the chain was accepted: %v", err)
	}
}

// TestMergeAnswersTakesTheChainsAnswersWhenTheCaptureHasNone is the case the
// capture window creates: a store read after a node crossed an epoch holds no
// answers, while the epoch's transactions still carry them.
func TestMergeAnswersTakesTheChainsAnswersWhenTheCaptureHasNone(t *testing.T) {
	one := answerAsAddress(t, "0x"+strings.Repeat("aa", 20))
	payload := longAnswerPayloadFor(t, []byte{0x01})

	merged, evidence, err := mergeAnswers(nil, map[common.Address][]byte{one: payload})
	if err != nil {
		t.Fatalf("the chain's answers alone were refused: %v", err)
	}
	if len(merged) != 1 || evidence.FromChain != 1 || evidence.FromStore != 0 {
		t.Fatalf("the merge did not take the chain's answers: %d answers, %+v", len(merged), evidence)
	}
	if !strings.Contains(evidence.Note, "transactions alone") {
		t.Fatalf("the note does not name the source: %s", evidence.Note)
	}
}

func TestLongAnswerPayloadKeepsTheWireFormTheStoreHolds(t *testing.T) {
	payload := longAnswerPayloadFor(t, []byte{0xa5, 0x11})
	if got := longAnswerPayload(payload); got != hex.EncodeToString(payload) {
		t.Fatalf("the sweep would record %q, expected the whole payload", got)
	}
	if got := longAnswerPayload([]byte{0x00, 0x01, 0x02}); got != "" {
		t.Fatalf("a payload the reference parser rejects was recorded as %q", got)
	}
}
