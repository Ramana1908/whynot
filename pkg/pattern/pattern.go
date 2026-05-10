// Package pattern is whynot's library of named violation templates.
// Each pattern matches against a checker.RejectionTrace and produces a
// partial Explanation when it fits.
//
// The matcher operates on the *original* (un-minimized) trace. The
// minimizer runs after pattern matching with the constraint that the
// matched pattern still applies to the minimized history.
package pattern

import (
	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/explainer"
)

// Matcher is one named violation pattern. Match returns a partial
// Explanation (Pattern, Summary, Conflicts, Suggestions, optional
// Witness shape) if the pattern fits the trace.
type Matcher interface {
	Name() string
	Match(*checker.RejectionTrace) (*explainer.Explanation, bool)
}

// All returns the pattern matchers in catalogue order. The order
// matters: the first matcher to fit wins.
//
// Catalogue order is most-specific to least-specific:
//   phantom_value (1 op)
//   realtime_inversion (2 ops, real-time forced)
//   non_monotonic_read (4 ops, single-client)
//   lost_update (3 ops, 2 writes + read)
//   stale_read (2 ops, fallback)
func All() []Matcher {
	return []Matcher{
		PhantomValue{},
		RealtimeInversion{},
		NonMonotonicRead{},
		LostUpdate{},
		StaleRead{},
	}
}

// AllMatchers is All() returned as a slice of explainer.PatternMatcher so
// it can be passed directly to explainer.Explain without a manual copy.
func AllMatchers() []explainer.PatternMatcher {
	src := All()
	out := make([]explainer.PatternMatcher, len(src))
	for i, m := range src {
		out[i] = m
	}
	return out
}
