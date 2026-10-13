package main

// The explorer's per-identity flip record, and what it is allowed to be used
// for: `/Epoch/{n}/Identity/{address}/Flips`.
//
// The epoch-wide `/Epoch/{n}/Flips` route answers the same records for everybody
// in the epoch, which is more than one author's bundle needs and has no author
// filter; the per-identity route returns one author's flips with the status and
// the answer the node's own qualification produced for each. That is the second
// half of the Plan B check `SPEC.md` 5.3 permits: the reconstruction computes
// those two fields itself, and the explorer's copy of them has to agree.
//
// What it does not do: the record is never a source for a verdict. A flip the
// explorer does not list, or lists differently, refuses the artifact rather than
// being preferred over the capture and the assignment the report was built from,
// and a record whose status is empty is counted as unpublished instead of being
// read as agreement.

import (
	"fmt"
	"net/url"
	"strings"
)

// indexerFlip is one record of the explorer's per-identity flip list. The fields
// are the ones the record carries that this repository compares; the explorer
// sends more (icon, size, timestamps, report counts), which are a rendering of
// the same chain data and are not read here.
type indexerFlip struct {
	CID    string `json:"cid"`
	Author string `json:"author"`
	Epoch  uint16 `json:"epoch"`
	Status string `json:"status"`
	Answer string `json:"answer"`
}

type indexerFlipPage struct {
	Result            []indexerFlip `json:"result"`
	ContinuationToken string        `json:"continuationToken"`
}

// fetchIndexerIdentityFlips walks one author's flips for one epoch. The route
// paginates like the other explorer lists but omits `continuationToken` on the
// last page rather than sending it empty, so both shapes end the walk.
func fetchIndexerIdentityFlips(c *httpClient, base string, epoch uint16, author string) ([]indexerFlip, error) {
	base = strings.TrimRight(base, "/")
	address := strings.TrimPrefix(strings.TrimSpace(author), "0x")
	all := []indexerFlip{}
	token := ""
	for page := 0; page < indexerMaxPages; page++ {
		endpoint := fmt.Sprintf("%s/Epoch/%d/Identity/0x%s/Flips?limit=%d", base, epoch, address, indexerPageLimit)
		if token != "" {
			endpoint += "&continuationToken=" + url.QueryEscape(token)
		}
		var answer indexerFlipPage
		if err := c.get(endpoint, &answer); err != nil {
			return nil, err
		}
		all = append(all, answer.Result...)
		if answer.ContinuationToken == "" {
			return all, nil
		}
		token = answer.ContinuationToken
	}
	return nil, fmt.Errorf("the indexer kept paginating past %d pages", indexerMaxPages)
}

// indexerStatusName is the reconstruction's status under the name the node and
// the explorer publish for it. The reconstruction spells them the way the node's
// own constants do, so the comparison is a name lookup rather than a guess.
func indexerStatusName(status FlipStatus) string {
	switch status {
	case FlipQualified:
		return "Qualified"
	case FlipWeaklyQualified:
		return "WeaklyQualified"
	case FlipQualifiedByNone:
		return "QualifiedByNone"
	case FlipNotQualified:
		return "NotQualified"
	default:
		return string(status)
	}
}

// crossCheckCoveredFlips compares the reconstruction's covered flips with the
// explorer's records for the same author and epoch.
//
// An answer the explorer publishes is compared in either vocabulary the node
// uses (`Left`/`Right`/`None`); a record whose status is empty carries no
// verdict, so it is counted as unpublished rather than as a disagreement. Every
// other field is a claim about the same qualification arithmetic this repository
// recomputed, which is why a disagreement is a refusal and not a preference.
func crossCheckCoveredFlips(c *httpClient, base string, epoch uint16, author string, covered []CoveredFlip) (*FlipCheck, error) {
	records, err := fetchIndexerIdentityFlips(c, base, epoch, author)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("the indexer lists no flips for %s in epoch %d", author, epoch)
	}
	byCid := make(map[string]indexerFlip, len(records))
	for _, record := range records {
		if !strings.EqualFold(strings.TrimPrefix(record.Author, "0x"), strings.TrimPrefix(author, "0x")) {
			continue
		}
		if record.Epoch != epoch {
			continue
		}
		byCid[record.CID] = record
	}
	check := &FlipCheck{Source: base, Read: len(byCid)}
	for _, flip := range covered {
		record, ok := byCid[flip.Cid]
		if !ok {
			check.Missing = append(check.Missing, fmt.Sprintf("flip %d %s", flip.Index, flip.Cid))
			continue
		}
		if record.Status == "" {
			check.NoResult = append(check.NoResult, fmt.Sprintf("flip %d %s", flip.Index, flip.Cid))
			continue
		}
		statusOk := strings.EqualFold(record.Status, indexerStatusName(FlipStatus(flip.Status)))
		answerOk := record.Answer == "" || strings.EqualFold(record.Answer, flip.Answer)
		if statusOk && answerOk {
			check.Matched++
			continue
		}
		check.Mismatched = append(check.Mismatched, fmt.Sprintf(
			"flip %d %s: this reconstruction says %s/%s, the explorer says %s/%s",
			flip.Index, flip.Cid, flip.Status, flip.Answer, record.Status, record.Answer))
	}
	check.Note = fmt.Sprintf(
		"of %d covered flips, %d agree with the explorer's own qualification record, %d disagree, %d the explorer does not list, %d carry no published result",
		len(covered), check.Matched, len(check.Mismatched), len(check.Missing), len(check.NoResult))
	return check, nil
}
