package pattern

import (
	"fmt"

	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
)

// PhantomValue matches a violation where a read returns a value that
// is neither the initial value nor produced by any write in the
// history.
//
// Predicate: a blocked read R with R.Value != Init and no write op in
// the history has Value == R.Value.
type PhantomValue struct{}

func (PhantomValue) Name() string { return "phantom_value" }

func (PhantomValue) Match(rt *checker.RejectionTrace) (*explainer.Explanation, bool) {
	if rt.Result != checker.Illegal {
		return nil, false
	}
	h := rt.History
	for _, p := range rt.Partitions {
		for _, b := range p.Blocks {
			read := findOp(h, b.OpID)
			if read == nil || read.Type != history.OpRead {
				continue
			}
			if read.Value == h.Init {
				continue
			}
			anyWriteOfValue := false
			for _, op := range h.Ops {
				if op.Type == history.OpWrite && op.Value == read.Value {
					anyWriteOfValue = true
					break
				}
			}
			if anyWriteOfValue {
				continue
			}
			witness := &history.History{
				Model: h.Model,
				Init:  h.Init,
				Ops:   []history.Op{*read},
			}
			return &explainer.Explanation{
				Verdict: explainer.VerdictNonLinearizable,
				Pattern: "phantom_value",
				Summary: fmt.Sprintf(
					"Read (op %d) returned %d, but no write of %d appears anywhere in the history and the initial value was %d.",
					read.ID, read.Value, read.Value, h.Init,
				),
				Witness: witness,
				Conflicts: []explainer.Conflict{{
					OpIDs: []int{read.ID},
					Why: fmt.Sprintf(
						"No write produced the value %d; reading it implies a value materialized from outside the recorded history.",
						read.Value,
					),
				}},
				Suggestions: []string{
					"Check for uninitialized memory or buffer-reuse bugs in the client serialization path",
					fmt.Sprintf("Check for response routing — could op %d have received another op's response?", read.ID),
				},
			}, true
		}
	}
	return nil, false
}
