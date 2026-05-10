package explainer

import (
	"fmt"
	"time"

	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/minimize"
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
			expl.Witness = minimize.DDMin(h, patternPredicate(matcher, timeout))
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

// patternPredicate yields a minimize.Predicate that holds on a
// sub-history iff the sub-history is non-linearizable AND the given
// matcher still fires on it. This is the pipeline-default predicate:
// it preserves the matched classification through minimization.
func patternPredicate(matcher PatternMatcher, timeout time.Duration) minimize.Predicate {
	return func(sub *history.History) bool {
		m, ops, err := model.SelectModel(sub)
		if err != nil {
			return false
		}
		rt := checker.Check(m, ops, sub, timeout)
		if rt.Result != checker.Illegal {
			return false
		}
		_, ok := matcher.Match(rt)
		return ok
	}
}

func countBlocked(rt *checker.RejectionTrace) int {
	n := 0
	for _, p := range rt.Partitions {
		n += len(p.BlockedOps)
	}
	return n
}
