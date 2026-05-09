package pattern

import (
	"fmt"

	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
)

// LostUpdate matches a stale-read variant where TWO writes returned in
// real time before the read, and the read shows the earlier write's
// effect — implying the later write was lost.
//
// Predicate: a blocked read R, two writes W1 and W2 with
// W1.Return < W2.Return < R.Call, and R.Value matches an earlier
// write (or the init) rather than W2.Value.
type LostUpdate struct{}

func (LostUpdate) Name() string { return "lost_update" }

func (LostUpdate) Match(rt *checker.RejectionTrace) (*explainer.Explanation, bool) {
	if rt.Result != checker.Illegal {
		return nil, false
	}
	for _, p := range rt.Partitions {
		for _, b := range p.Blocks {
			if b.Constraint != checker.ConstraintValueMismatch {
				continue
			}
			read := findOp(rt.History, b.OpID)
			if read == nil || read.Type != history.OpRead {
				continue
			}
			// Collect writes that completed before the read started.
			var precedingWrites []history.Op
			for _, op := range rt.History.Ops {
				if op.Type == history.OpWrite && op.Return < read.Call {
					precedingWrites = append(precedingWrites, op)
				}
			}
			if len(precedingWrites) < 2 {
				continue
			}
			// Find latest preceding write by Return time.
			latest := precedingWrites[0]
			for _, w := range precedingWrites[1:] {
				if w.Return > latest.Return {
					latest = w
				}
			}
			if read.Value == latest.Value {
				// Read sees the latest preceding write — not a lost
				// update (could be linearizable, but if blocked some
				// other pattern fits).
				continue
			}
			// Build witness: two preceding writes (earliest + latest) + read.
			earliest := precedingWrites[0]
			for _, w := range precedingWrites[1:] {
				if w.Return < earliest.Return {
					earliest = w
				}
			}
			witness := &history.History{
				Model: rt.History.Model,
				Init:  rt.History.Init,
				Ops:   []history.Op{earliest, latest, *read},
			}
			return &explainer.Explanation{
				Verdict: explainer.VerdictNonLinearizable,
				Pattern: "lost_update",
				Summary: fmt.Sprintf(
					"Read (op %d) returned %d, but two writes (ops %d, %d) had completed; the second write's effect (value %d) is not visible.",
					read.ID, read.Value, earliest.ID, latest.ID, latest.Value,
				),
				Witness: witness,
				Conflicts: []explainer.Conflict{{
					OpIDs: []int{earliest.ID, latest.ID, read.ID},
					Why: fmt.Sprintf(
						"Both writes returned before the read started (op %d at t=%d, op %d at t=%d); the read at t=%d returned %d, ignoring op %d's update of %d.",
						earliest.ID, earliest.Return, latest.ID, latest.Return,
						read.Call, read.Value, latest.ID, latest.Value,
					),
				}},
				Suggestions: []string{
					"Check for last-write-wins races without proper synchronization",
					fmt.Sprintf("Check for read-from-stale-replica that hasn't seen op %d", latest.ID),
				},
			}, true
		}
	}
	return nil, false
}
