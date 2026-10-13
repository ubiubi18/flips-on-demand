package main

// Where an eligibility reconstruction's long answers come from.
//
// Two sources describe the same submissions: the node's epoch store, which
// holds the payloads while it runs the ceremony and clears them when the epoch
// ends (`core/ceremony/ceremony.go`, `completeEpoch`), and the epoch's own
// transactions, which a sweep reads out of CID-committed block bodies. The
// second is the one `SPEC.md` 5.2 asks for - an input authenticated by the
// transaction that carried it - and it survives the epoch, so a reconstruction
// does not have to race the store. When both are present they are compared, and
// a disagreement refuses: a reconstruction may not choose between two versions
// of what a candidate answered.

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/idena-network/idena-go/common"
)

// LongAnswerTx is one long-answer transaction a sweep saw.
type LongAnswerTx struct {
	TxHash      string `json:"txHash"`
	BlockHeight uint64 `json:"blockHeight"`
	Author      string `json:"author"`
	Epoch       uint16 `json:"epoch"`
	PayloadHex  string `json:"payloadHex"`
}

// AnswersEvidence records where a reconstruction's answers came from and what
// the two sources said about each other.
type AnswersEvidence struct {
	FromStore  int    `json:"fromStore"`
	FromChain  int    `json:"fromChain"`
	Agreed     int    `json:"agreed"`
	StoreOnly  int    `json:"storeOnly"`
	ChainOnly  int    `json:"chainOnly"`
	Duplicates int    `json:"duplicateSubmissions"`
	Note       string `json:"note,omitempty"`
}

// answersFromSweep reads the long answers a sweep recorded for one epoch,
// keyed by the address that sent them.
//
// A repeated submission keeps the first: the node ignores a second long-answer
// transaction from the same address, so the reconstruction follows the same
// rule rather than letting a later transaction overwrite an earlier one.
func answersFromSweep(path string, epoch uint16) (map[common.Address][]byte, *AnswersEvidence, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var sweep SweepResult
	if err := json.Unmarshal(raw, &sweep); err != nil {
		return nil, nil, fmt.Errorf("parse sweep %s: %w", path, err)
	}
	out := map[common.Address][]byte{}
	evidence := &AnswersEvidence{}
	for _, answer := range sweep.Answers {
		if answer.Epoch != epoch {
			continue
		}
		payload, err := hex.DecodeString(answer.PayloadHex)
		if err != nil {
			return nil, nil, fmt.Errorf("the sweep's answer from %s is not hex: %w", answer.Author, err)
		}
		if len(payload) == 0 {
			continue
		}
		address, err := answerAddress(answer.Author)
		if err != nil {
			return nil, nil, err
		}
		if _, seen := out[address]; seen {
			evidence.Duplicates++
			continue
		}
		out[address] = payload
	}
	evidence.FromChain = len(out)
	return out, evidence, nil
}

// mergeAnswers combines the answers a capture holds with the ones a sweep
// holds, and refuses a disagreement rather than preferring either.
//
// A source that is absent contributes nothing: a capture taken before the
// answers arrived, or a run that names no sweep, still produces a
// reconstruction from what there is. The counts say which case happened.
func mergeAnswers(store, chain map[common.Address][]byte) (map[common.Address][]byte, *AnswersEvidence, error) {
	evidence := &AnswersEvidence{FromStore: len(store), FromChain: len(chain)}
	merged := make(map[common.Address][]byte, len(store)+len(chain))
	for address, payload := range store {
		merged[address] = payload
	}
	for address, payload := range chain {
		if stored, ok := merged[address]; ok {
			if !equalBytes(stored, payload) {
				return nil, nil, fmt.Errorf(
					"the capture and the epoch's transactions disagree about %s's long answers: the capture holds %d bytes, the transaction carries %d",
					address.String(), len(stored), len(payload))
			}
			evidence.Agreed++
			continue
		}
		merged[address] = payload
	}
	evidence.ChainOnly = len(chain) - evidence.Agreed
	evidence.StoreOnly = len(store) - evidence.Agreed
	switch {
	case evidence.FromStore > 0 && evidence.FromChain == 0:
		evidence.Note = "the answers came from the capture alone: the sweep carried none for this epoch"
	case evidence.FromStore == 0 && evidence.FromChain > 0:
		evidence.Note = "the answers came from the epoch's transactions alone: the capture holds none for this epoch"
	case evidence.FromStore == 0 && evidence.FromChain == 0:
		evidence.Note = "neither the capture nor the sweep holds a long answer for this epoch"
	case evidence.Agreed == evidence.FromStore && evidence.Agreed == evidence.FromChain:
		evidence.Note = "the capture and the epoch's transactions agree about every answer they both hold"
	default:
		evidence.Note = "the capture and the epoch's transactions agreed where both held an answer, and each supplied what the other did not"
	}
	return merged, evidence, nil
}

// answerAddress parses the address form the sweep writes.
func answerAddress(value string) (common.Address, error) {
	address := common.Address{}
	raw, err := hex.DecodeString(trimAddress(value))
	if err != nil || len(raw) != len(address) {
		return address, fmt.Errorf("the sweep names an answer author %q that is not a 20-byte address", value)
	}
	address.SetBytes(raw)
	return address, nil
}

// trimAddress removes the optional 0x prefix, in either case.
func trimAddress(value string) string {
	if len(value) >= 2 && value[0] == '0' && (value[1] == 'x' || value[1] == 'X') {
		return value[2:]
	}
	return value
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
