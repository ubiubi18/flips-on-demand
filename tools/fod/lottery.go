package main

// The epoch lottery: which flips each participant had to answer.
//
// Package 2's eligibility half needs this, because answers are positional. A
// short-answer payload is a byte per flip in the order the lottery gave that
// participant (`SOURCES.md` F-79), so mapping an answer back to a flip needs the
// assignment. The assignment is not committed anywhere and no node keeps it past
// the epoch: it is derived from the lottery seed and the epoch's candidate list,
// which is what this file implements.
//
// The rules are the ones recorded in `SOURCES.md` F-79, read from the pinned node
// and written here as this repository's own implementation. No line is
// translated from the pinned files, which are LGPL-3.0 (`SOURCES.md` F-11);
// the thresholds, the order of the passes, and the two seeds derived from the
// lottery seed are protocol facts, and `lottery_vector_test.go` checks this
// implementation against vectors captured by running the pinned code unchanged.
//
// The two randomness sources are derived from the seed with the node's own
// arithmetic, `seed64*21-77` for the author queue and `seed64*12+3` for the
// candidate permutation, so the same seed has to produce the same assignment for
// every participant, or a proof built on one node's view would not check out
// anywhere else.

import (
	"encoding/binary"
	"fmt"
	"math/rand"
)

// authorTarget is the floor the reference node brings every author up to when a
// shard has more than seven authors (`SOURCES.md` F-79).
const authorTarget = 13

// sevenAuthors is the size above which the reference hands out extra candidate
// slots. It is a literal in the pinned code, not a configurable.
const sevenAuthors = 7

// LotteryCandidate is one participant of an epoch, as the candidate list holds
// it: whether the identity submitted flips this epoch, and those flips in the
// order the list carries them.
type LotteryCandidate struct {
	IsAuthor bool
	Flips    [][]byte
}

// LotteryAssignment is the result for one shard.
type LotteryAssignment struct {
	// AuthorsPerCandidate lists, per candidate index, the author indexes whose
	// flips that candidate was given.
	AuthorsPerCandidate map[int][]int
	// CandidatesPerAuthor lists, per author index, the candidate indexes that
	// received that author's flips. This is the committee a flip is judged by.
	CandidatesPerAuthor map[int][]int
	// ShortFlipsPerCandidate lists, per candidate index, the flip indexes the
	// candidate answered in the short session, in the order the answers are
	// submitted.
	ShortFlipsPerCandidate [][]int
	// LongFlipsPerCandidate lists the same for the long session.
	LongFlipsPerCandidate [][]int
}

// AssignFlips reproduces the lottery for one shard. Flips are addressed by their
// index in the flattened candidate list, which is what the reference uses and
// what makes an assignment checkable against a submission.
func AssignFlips(candidates []LotteryCandidate, seed []byte, shortFlipsCount int) (LotteryAssignment, error) {
	if len(seed) < 8 {
		return LotteryAssignment{}, fmt.Errorf("the lottery seed is %d bytes, need at least 8", len(seed))
	}
	if shortFlipsCount <= 0 {
		return LotteryAssignment{}, fmt.Errorf("the short session holds %d flips", shortFlipsCount)
	}
	out := LotteryAssignment{
		AuthorsPerCandidate: map[int][]int{},
		CandidatesPerAuthor: map[int][]int{},
	}
	if len(candidates) == 0 {
		return out, nil
	}
	authors := authorIndexes(candidates)
	if len(authors) == 0 {
		// Nobody submitted a flip: the reference still fills the long list so a
		// caller always has one index to point at.
		out.ShortFlipsPerCandidate = make([][]int, len(candidates))
		out.LongFlipsPerCandidate = oneFlipEach(make([][]int, len(candidates)))
		return out, nil
	}

	flips, flipsPerAuthor := flatten(candidates)
	out.AuthorsPerCandidate, out.CandidatesPerAuthor = firstDistribution(authors, candidates, seed, shortFlipsCount)
	if len(authors) > sevenAuthors {
		out.AuthorsPerCandidate, out.CandidatesPerAuthor =
			extraCandidates(seed, len(candidates), out.AuthorsPerCandidate, out.CandidatesPerAuthor)
	}
	out.ShortFlipsPerCandidate, out.LongFlipsPerCandidate =
		flipDistribution(len(candidates), out.AuthorsPerCandidate, flipsPerAuthor, flips, seed, shortFlipsCount)
	out.LongFlipsPerCandidate = oneFlipEach(out.LongFlipsPerCandidate)
	return out, nil
}

// authorIndexes is the candidate list filtered down to the ones that submitted
// flips, in list order.
func authorIndexes(candidates []LotteryCandidate) []int {
	var out []int
	for idx, candidate := range candidates {
		if candidate.IsAuthor {
			out = append(out, idx)
		}
	}
	return out
}

// flatten builds the two views of the flip pool the reference works with: every
// flip in candidate order, and the flips of each author, keyed by the author's
// index in that same list.
func flatten(candidates []LotteryCandidate) ([][]byte, map[int][][]byte) {
	var flips [][]byte
	perAuthor := make(map[int][][]byte)
	for idx, candidate := range candidates {
		for _, flip := range candidate.Flips {
			flips = append(flips, flip)
			perAuthor[idx] = append(perAuthor[idx], flip)
		}
	}
	return flips, perAuthor
}

// firstDistribution hands every candidate exactly shortFlipsCount authors, drawn
// from a queue that is refilled from a fresh permutation whenever it runs dry.
func firstDistribution(authors []int, candidates []LotteryCandidate, seed []byte, shortFlipsCount int) (map[int][]int, map[int][]int) {
	queue := authorQueue(seed, authors, len(candidates)*shortFlipsCount)
	authorsPerCandidate := make(map[int][]int)
	candidatesPerAuthor := make(map[int][]int)
	candidateIndex := 0
	for queue.len() > 0 {
		if candidateIndex == len(candidates) {
			candidateIndex = 0
		}
		author := nextUnused(queue, candidateIndex, authorsPerCandidate[candidateIndex])
		authorsPerCandidate[candidateIndex] = append(authorsPerCandidate[candidateIndex], author)
		candidatesPerAuthor[author] = append(candidatesPerAuthor[author], candidateIndex)
		candidateIndex++
	}
	return authorsPerCandidate, candidatesPerAuthor
}

// extraCandidates is the second pass: a shard with more than seven authors leaves
// authors below the floor, and the reference tops them up to candidatesPerAuthor
// from a shuffled candidate list.
func extraCandidates(seed []byte, candidatesCount int, authorsPerCandidate, candidatesPerAuthor map[int][]int) (map[int][]int, map[int][]int) {
	random := rand.New(rand.NewSource(seed64(seed)*77 + 55))
	shuffled := func() *intQueue {
		queue := &intQueue{}
		for _, idx := range random.Perm(candidatesCount) {
			queue.push(idx)
		}
		return queue
	}

	queue := shuffled()
	for author := 0; author < candidatesCount; author++ {
		assigned, ok := candidatesPerAuthor[author]
		if !ok || len(assigned) >= authorTarget {
			continue
		}
		if queue.len() == 0 {
			queue = shuffled()
		}
		for len(candidatesPerAuthor[author]) < authorTarget && queue.len() > 0 {
			candidate := nextUnused(queue, author, candidatesPerAuthor[author])
			candidatesPerAuthor[author] = append(candidatesPerAuthor[author], candidate)
			authorsPerCandidate[candidate] = append(authorsPerCandidate[candidate], author)
		}
	}
	return authorsPerCandidate, candidatesPerAuthor
}

// flipDistribution turns the author-to-candidate mapping into the flip lists each
// candidate answers, spending candidates on authors round-robin so a shard with
// few authors does not hand the same flips to everyone in the short session.
func flipDistribution(candidatesCount int, authorsPerCandidate map[int][]int, flipsPerAuthor map[int][][]byte, flips [][]byte, seed []byte, shortFlipsCount int) ([][]int, [][]int) {
	flipIndex := make(map[string]int, len(flips))
	for idx, flip := range flips {
		flipIndex[string(flip)] = idx
	}
	random := rand.New(rand.NewSource(seed64(seed)*12 + 3))
	permutation := random.Perm(candidatesCount)

	timesUsed := make([]int, candidatesCount)
	nextFlipOf := make([]int, candidatesCount)
	short := make([][]int, candidatesCount)
	long := make([][]int, candidatesCount)

	for _, candidate := range permutation {
		authors, ok := authorsPerCandidate[candidate]
		if !ok {
			continue
		}
		authors = distinct(authors)
		used := make(map[int]bool)
		usedFlips := make(map[int]map[int]bool)

		for j := 0; j < shortFlipsCount; j++ {
			if len(authors) < shortFlipsCount && len(used) == len(authors) {
				used = make(map[int]bool)
			}
			author, flip, at := nextShortFlip(authors, used, flipsPerAuthor, nextFlipOf, timesUsed)
			if len(authors) >= shortFlipsCount {
				if usedFlips[author] == nil {
					usedFlips[author] = make(map[int]bool)
				}
				usedFlips[author][at] = true
			}
			short[candidate] = append(short[candidate], flipIndex[string(flip)])
			timesUsed[author]++
			used[author] = true
		}

		for _, author := range authors {
			for idx, flip := range flipsPerAuthor[author] {
				if len(authors) < shortFlipsCount {
					long[candidate] = append(long[candidate], flipIndex[string(flip)])
					continue
				}
				if taken := usedFlips[author]; taken == nil || !taken[idx] {
					long[candidate] = append(long[candidate], flipIndex[string(flip)])
				}
			}
		}
		short[candidate] = distinct(short[candidate])
		long[candidate] = distinct(long[candidate])
	}
	return short, long
}

// nextShortFlip spends the author that has been used least, and takes that
// author's next flip, wrapping around when the author runs out. It reports the
// position it took, which the long session uses to avoid repeating a flip.
func nextShortFlip(authors []int, used map[int]bool, flipsPerAuthor map[int][][]byte, nextFlipOf, timesUsed []int) (int, []byte, int) {
	author := 0
	fewest := int(^uint(0) >> 1)
	for _, candidate := range authors {
		if used[candidate] {
			continue
		}
		if timesUsed[candidate] < fewest {
			author = candidate
			fewest = timesUsed[candidate]
		}
	}
	authorFlips := flipsPerAuthor[author]
	at := nextFlipOf[author]
	if at >= len(authorFlips) {
		at = 0
	}
	nextFlipOf[author] = at + 1
	return author, authorFlips[at], at
}

// authorQueue is a repeating shuffle of the author indexes, long enough to give
// every candidate its short session.
func authorQueue(seed []byte, authors []int, length int) *intQueue {
	random := rand.New(rand.NewSource(seed64(seed)*21 - 77))
	order := random.Perm(len(authors))
	queue := &intQueue{}
	at := 0
	for queue.len() < length {
		if at == len(order) {
			at = 0
			order = random.Perm(len(authors))
		}
		queue.push(authors[order[at]])
		at++
	}
	return queue
}

// nextUnused takes the first author from the queue that is neither the candidate
// itself nor one this candidate already has.
func nextUnused(queue *intQueue, candidate int, alreadyUsed []int) int {
	for i := 0; i < queue.len(); i++ {
		next := queue.pop()
		if next != candidate && !contains(alreadyUsed, next) {
			return next
		}
		queue.push(next)
	}
	return queue.pop()
}

func distinct(values []int) []int {
	seen := make(map[int]bool, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func contains(values []int, wanted int) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// oneFlipEach applies the reference's last pass: a candidate with no long-session
// flip at all is pointed at flip 0 rather than left empty.
func oneFlipEach(lists [][]int) [][]int {
	for idx := range lists {
		if len(lists[idx]) == 0 {
			lists[idx] = []int{0}
		}
	}
	return lists
}

func seed64(seed []byte) int64 {
	return int64(binary.LittleEndian.Uint64(seed))
}

// intQueue is the FIFO the reference builds on a linked list; order is all the
// algorithm depends on.
type intQueue struct {
	values []int
	head   int
}

func (q *intQueue) push(value int) {
	q.values = append(q.values, value)
}

func (q *intQueue) pop() int {
	value := q.values[q.head]
	q.head++
	if q.head == len(q.values) {
		q.values = q.values[:0]
		q.head = 0
	}
	return value
}

func (q *intQueue) len() int {
	return len(q.values) - q.head
}
