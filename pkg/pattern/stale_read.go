package pattern

import (
	"fmt"

	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
)

// StaleRead matches a history where a read op is blocked because the
// model state — established by an earlier real-time-preceding write —
// disagrees with the read's returned value.
//
// Predicate: there is a blocked read R and a write W in the history
// with W.Return < R.Call and the value mismatch is the established
// state. This is the most permissive matcher and acts as a fallback;
// more specific patterns are tried first by All().
type StaleRead struct{}

func (StaleRead) Name() string { return "stale_read" }

func (StaleRead) Match(rt *checker.RejectionTrace) (*explainer.Explanation, bool) {
	if rt.Result != checker.Illegal {
		return nil, false
	}
	for _, p := range rt.Partitions {
		for _, b := range p.Blocks {
			if b.Constraint != checker.ConstraintValueMismatch {
				continue
			}
			read := findOp(rt.History, b.OpID)
			if read.Type != history.OpRead {
				continue
			}
			// Need a real-time-preceding write whose value differs
			// from the read's.
			var establishingWrite *history.Op
			for i := range rt.History.Ops {
				w := &rt.History.Ops[i]
				if w.Type != history.OpWrite {
					continue
				}
				if w.Return < read.Call && w.Value != read.Value {
					establishingWrite = w
					break
				}
			}
			if establishingWrite == nil {
				continue
			}
			witness := &history.History{
				Model: rt.History.Model,
				Init:  rt.History.Init,
				Ops:   []history.Op{*establishingWrite, *read},
			}
			return &explainer.Explanation{
				Verdict: explainer.VerdictNonLinearizable,
				Pattern: "stale_read",
				Summary: fmt.Sprintf(
					"Read (op %d) returned %d, but write of %d (op %d) had already completed before the read started.",
					read.ID, read.Value, establishingWrite.Value, establishingWrite.ID,
				),
				Witness: witness,
				Conflicts: []explainer.Conflict{{
					OpIDs: []int{establishingWrite.ID, read.ID},
					Why: fmt.Sprintf(
						"Write of %d returned at t=%d; read started at t=%d and must see a value at least as new as %d, but returned %d.",
						establishingWrite.Value, establishingWrite.Return, read.Call, establishingWrite.Value, read.Value,
					),
				}},
				Suggestions: []string{
					"Check for stale replica reads (read-from-follower without quorum)",
					"Check for missed cache invalidation between the writer and reader",
				},
			}, true
		}
	}
	return nil, false
}

func findOp(h *history.History, id int) *history.Op {
	for i := range h.Ops {
		if h.Ops[i].ID == id {
			return &h.Ops[i]
		}
	}
	return nil
}
