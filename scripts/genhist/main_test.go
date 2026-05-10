package main

import (
	"testing"

	"github.com/Ramana1908/whynot"
)

// TestEachBugFiresTargetPattern is the deterministic end-to-end coverage
// claim: every bug-specific generator produces a history that, when fed
// through the whynot pipeline, yields the intended pattern. This is what
// closes "future work #1" in the README — multi-pattern end-to-end runs.
func TestEachBugFiresTargetPattern(t *testing.T) {
	cases := []struct {
		bug     string
		pattern string
	}{
		{"stale", "stale_read"},
		{"lost", "lost_update"},
		{"inversion", "realtime_inversion"},
		{"nmr", "non_monotonic_read"},
		{"phantom", "phantom_value"},
	}
	for _, tc := range cases {
		t.Run(tc.bug, func(t *testing.T) {
			h, err := generate(tc.bug, 1)
			if err != nil {
				t.Fatalf("generate(%s): %v", tc.bug, err)
			}
			expl, err := whynot.Explain(h)
			if err != nil {
				t.Fatalf("explain: %v", err)
			}
			if expl.Verdict != whynot.VerdictNonLinearizable {
				t.Errorf("verdict = %s, want non-linearizable", expl.Verdict)
			}
			if expl.Pattern != tc.pattern {
				t.Errorf("pattern = %q, want %q", expl.Pattern, tc.pattern)
			}
		})
	}
}

// TestUnknownBugRejected guards the dispatcher.
func TestUnknownBugRejected(t *testing.T) {
	if _, err := generate("doesnotexist", 0); err == nil {
		t.Fatal("expected error for unknown bug")
	}
}

// TestConcurrentBugProducesNonLinearizable: the legacy concurrent driver
// is timing-dependent (op IDs and even pattern vary across runs), so we
// only assert the verdict, not the specific pattern.
func TestConcurrentBugProducesNonLinearizable(t *testing.T) {
	h, err := generate("concurrent", 1)
	if err != nil {
		t.Fatal(err)
	}
	expl, err := whynot.Explain(h)
	if err != nil {
		t.Fatal(err)
	}
	if expl.Verdict != whynot.VerdictNonLinearizable {
		t.Errorf("verdict = %s, want non-linearizable", expl.Verdict)
	}
	if expl.Pattern == "" {
		t.Error("expected some pattern to fire")
	}
}
