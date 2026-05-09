package pattern

import (
	"fmt"

	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
)

// NonMonotonicRead matches a violation where a single client observes
// state going backwards: it reads a "newer" value, then a "later" read
// returns an "older" value.
//
// Predicate: same Client, two reads R1, R2 with R1.Return < R2.Call,
// where there exists a write W with W.Value == R1.Value and a write W'
// with W'.Value == R2.Value such that W' completed before W started
// (W' is the older write). I.e., R1 sees a post-W state, R2 sees a
// pre-W state.
type NonMonotonicRead struct{}

func (NonMonotonicRead) Name() string { return "non_monotonic_read" }

func (NonMonotonicRead) Match(rt *checker.RejectionTrace) (*explainer.Explanation, bool) {
	if rt.Result != checker.Illegal {
		return nil, false
	}
	h := rt.History

	// Index reads by client.
	readsByClient := map[int][]history.Op{}
	for _, op := range h.Ops {
		if op.Type == history.OpRead {
			readsByClient[op.Client] = append(readsByClient[op.Client], op)
		}
	}
	// Writes by value, ordered.
	writesByValue := map[int][]history.Op{}
	for _, op := range h.Ops {
		if op.Type == history.OpWrite {
			writesByValue[op.Value] = append(writesByValue[op.Value], op)
		}
	}

	for _, reads := range readsByClient {
		for i := 0; i < len(reads); i++ {
			for j := 0; j < len(reads); j++ {
				if i == j {
					continue
				}
				r1, r2 := reads[i], reads[j]
				if r1.Return >= r2.Call {
					continue // not strictly ordered in real time
				}
				if r1.Value == r2.Value {
					continue // monotonic
				}
				// Look for writes producing each value.
				ws1 := writesByValue[r1.Value]
				ws2 := writesByValue[r2.Value]
				if len(ws1) == 0 && r1.Value != h.Init {
					continue
				}
				if len(ws2) == 0 && r2.Value != h.Init {
					continue
				}
				// r2's value comes from an earlier write (or init) than r1's value.
				if isOlderState(ws1, ws2, r1.Value, r2.Value, h.Init) {
					witness := buildMonotonicWitness(h, r1, r2)
					return &explainer.Explanation{
						Verdict: explainer.VerdictNonLinearizable,
						Pattern: "non_monotonic_read",
						Summary: fmt.Sprintf(
							"Client %d read %d (op %d) and then read %d (op %d); the second read sees a value that was overwritten before the first read.",
							r1.Client, r1.Value, r1.ID, r2.Value, r2.ID,
						),
						Witness: witness,
						Conflicts: []explainer.Conflict{{
							OpIDs: []int{r1.ID, r2.ID},
							Why: fmt.Sprintf(
								"Within a single client (client %d), op %d returned %d (the post-op-%d state) but op %d, started later, returned %d (the pre-op-%d state). State cannot move backwards from a client's perspective.",
								r1.Client, r1.ID, r1.Value, latestWriteID(ws1), r2.ID, r2.Value, latestWriteID(ws1),
							),
						}},
						Suggestions: []string{
							"Check for client routing that fails over to a stale replica between requests",
							"Check for read-your-writes session guarantees being violated",
						},
					}, true
				}
			}
		}
	}
	return nil, false
}

// isOlderState returns true if v2 is a strictly-earlier state than v1
// in real time (some write of v2 returned before any write of v1
// returned, or v2 == Init and at least one write of v1 exists).
func isOlderState(ws1, ws2 []history.Op, v1, v2, init int) bool {
	if v2 == init && len(ws1) > 0 {
		return true
	}
	if len(ws1) == 0 || len(ws2) == 0 {
		return false
	}
	// Some w2 returned before some w1 started.
	for _, w2 := range ws2 {
		for _, w1 := range ws1 {
			if w2.Return < w1.Call {
				return true
			}
		}
	}
	return false
}

func latestWriteID(ws []history.Op) int {
	if len(ws) == 0 {
		return -1
	}
	latest := ws[0]
	for _, w := range ws[1:] {
		if w.Return > latest.Return {
			latest = w
		}
	}
	return latest.ID
}

// buildMonotonicWitness includes both reads plus the writes that
// establish the ordering relationship between their values.
func buildMonotonicWitness(h *history.History, r1, r2 history.Op) *history.History {
	keep := map[int]bool{r1.ID: true, r2.ID: true}
	for _, op := range h.Ops {
		if op.Type == history.OpWrite && (op.Value == r1.Value || op.Value == r2.Value) {
			keep[op.ID] = true
		}
	}
	var ops []history.Op
	for _, op := range h.Ops {
		if keep[op.ID] {
			ops = append(ops, op)
		}
	}
	return &history.History{Model: h.Model, Init: h.Init, Ops: ops}
}
