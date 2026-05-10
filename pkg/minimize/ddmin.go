// Package minimize implements one-minimal delta debugging on histories.
// Given a history that satisfies some predicate (typically: "still
// non-linearizable and still matches pattern P"), it returns the
// smallest sub-history that still satisfies the predicate.
package minimize

import (
	"github.com/Ramana1908/whynot/pkg/history"
)

// Predicate is a property over a sub-history. The minimizer searches for
// the smallest sub-history (preserving op order) on which Predicate
// returns true.
type Predicate func(*history.History) bool

// DDMin runs Zeller's ddmin (one-minimal delta debugging) on h, using
// pred to test sub-histories. The returned history's Ops is a subset of
// h.Ops in original order.
func DDMin(h *history.History, pred Predicate) *history.History {
	if len(h.Ops) < 2 || !pred(h) {
		return h
	}
	return ddmin(h, h.Ops, 2, pred)
}

func ddmin(orig *history.History, ops []history.Op, n int, pred Predicate) *history.History {
	if len(ops) < 2 {
		return wrap(orig, ops)
	}

	chunks := splitChunks(ops, n)

	// Reduction to a subset: keep only chunk i.
	for _, c := range chunks {
		if len(c) == 0 || len(c) == len(ops) {
			continue
		}
		if pred(wrap(orig, c)) {
			return ddmin(orig, c, 2, pred)
		}
	}

	// Reduction to complement: remove chunk i.
	for i := range chunks {
		complement := concatExcept(chunks, i)
		if len(complement) == 0 || len(complement) == len(ops) {
			continue
		}
		if pred(wrap(orig, complement)) {
			next := n - 1
			if next < 2 {
				next = 2
			}
			return ddmin(orig, complement, next, pred)
		}
	}

	// Increase granularity.
	if n < len(ops) {
		next := n * 2
		if next > len(ops) {
			next = len(ops)
		}
		return ddmin(orig, ops, next, pred)
	}

	return wrap(orig, ops)
}

func splitChunks(ops []history.Op, n int) [][]history.Op {
	if n > len(ops) {
		n = len(ops)
	}
	chunks := make([][]history.Op, 0, n)
	base := len(ops) / n
	extra := len(ops) % n
	start := 0
	for i := 0; i < n; i++ {
		size := base
		if i < extra {
			size++
		}
		end := start + size
		chunks = append(chunks, ops[start:end])
		start = end
	}
	return chunks
}

func concatExcept(chunks [][]history.Op, skip int) []history.Op {
	total := 0
	for i, c := range chunks {
		if i == skip {
			continue
		}
		total += len(c)
	}
	out := make([]history.Op, 0, total)
	for i, c := range chunks {
		if i == skip {
			continue
		}
		out = append(out, c...)
	}
	return out
}

func wrap(orig *history.History, ops []history.Op) *history.History {
	return &history.History{Model: orig.Model, Init: orig.Init, Ops: ops}
}
