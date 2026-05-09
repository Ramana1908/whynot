package explainer

import (
	"fmt"
	"time"

	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/model"
)

// PatternMatcher decouples explainer from pkg/pattern (which would
// otherwise create a cyclic import: pkg/pattern needs explainer.Explanation).
type PatternMatcher interface {
	Name() string
	Match(*checker.RejectionTrace) (*Explanation, bool)
}

// Explain runs the full pipeline: model selection → checker →
// pattern matching → assembly. matchers are tried in order; first
// fit wins. Pass nil to use no matchers (raw verdict only).
func Explain(h *history.History, matchers []PatternMatcher, timeout time.Duration) (*Explanation, error) {
	m, ops, err := model.SelectModel(h)
	if err != nil {
		return nil, err
	}
	rt := checker.Check(m, ops, h, timeout)

	switch rt.Result {
	case checker.Ok:
		return &Explanation{
			Verdict: VerdictLinearizable,
			Summary: "History is linearizable.",
		}, nil
	case checker.Unknown:
		return &Explanation{
			Verdict: VerdictUnknown,
			Summary: "Checker timed out before reaching a verdict.",
		}, nil
	}

	for _, matcher := range matchers {
		if expl, ok := matcher.Match(rt); ok {
			return expl, nil
		}
	}

	// Fallback: non-linearizable, no pattern fit.
	return &Explanation{
		Verdict: VerdictNonLinearizable,
		Witness: h,
		Summary: fmt.Sprintf("History is non-linearizable; no pattern in the catalogue fit (%d blocked op(s)).", countBlocked(rt)),
	}, nil
}

func countBlocked(rt *checker.RejectionTrace) int {
	n := 0
	for _, p := range rt.Partitions {
		n += len(p.BlockedOps)
	}
	return n
}
