package pattern

import (
	"fmt"

	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
)

// RealtimeInversion matches a violation where a read returned a value
// that is only produced by a write that started after the read finished
// — i.e., the read appears to observe a future write.
//
// Predicate: a blocked read R with R.Value != Init, and there exists a
// write W in the history with W.Value == R.Value and W.Call > R.Return.
// Additionally, no write of R.Value completes before R.Call (otherwise
// stale_read would fit better).
type RealtimeInversion struct{}

func (RealtimeInversion) Name() string { return "realtime_inversion" }

func (RealtimeInversion) Match(rt *checker.RejectionTrace) (*explainer.Explanation, bool) {
	if rt.Result != checker.Illegal {
		return nil, false
	}
	for _, p := range rt.Partitions {
		for _, b := range p.Blocks {
			read := findOp(rt.History, b.OpID)
			if read == nil || read.Type != history.OpRead {
				continue
			}
			if read.Value == rt.History.Init {
				continue
			}
			// Look for a future write that produced this value, AND
			// confirm no past write produced it.
			var futureWrite *history.Op
			pastWriteOfSameValue := false
			for i := range rt.History.Ops {
				w := &rt.History.Ops[i]
				if w.Type != history.OpWrite || w.Value != read.Value {
					continue
				}
				if w.Call > read.Return {
					if futureWrite == nil {
						futureWrite = w
					}
				}
				if w.Return < read.Call {
					pastWriteOfSameValue = true
				}
			}
			if futureWrite == nil || pastWriteOfSameValue {
				continue
			}
			witness := &history.History{
				Model: rt.History.Model,
				Init:  rt.History.Init,
				Ops:   []history.Op{*read, *futureWrite},
			}
			return &explainer.Explanation{
				Verdict: explainer.VerdictNonLinearizable,
				Pattern: "realtime_inversion",
				Summary: fmt.Sprintf(
					"Read (op %d) returned %d at t=[%d,%d], but the only write of %d (op %d) did not start until t=%d — the read would have to be ordered after a write that did not yet exist.",
					read.ID, read.Value, read.Call, read.Return,
					read.Value, futureWrite.ID, futureWrite.Call,
				),
				Witness: witness,
				Conflicts: []explainer.Conflict{{
					OpIDs: []int{read.ID, futureWrite.ID},
					Why: fmt.Sprintf(
						"Op %d (read of %d) finished at t=%d, but op %d (the only write of %d) started at t=%d; real-time precedence forces op %d before op %d, so op %d cannot have observed op %d's write.",
						read.ID, read.Value, read.Return, futureWrite.ID, futureWrite.Value, futureWrite.Call,
						read.ID, futureWrite.ID, read.ID, futureWrite.ID,
					),
				}},
				Suggestions: []string{
					"Check the clock source feeding the history — apparent time travel is often a clock skew artifact",
					"Check for replay/retry logic returning the response of a future operation",
				},
			}, true
		}
	}
	return nil, false
}
