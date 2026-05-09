// Package explainer is the user-facing layer of whynot. It assembles a
// RejectionTrace, the minimized history witness, and the matched pattern
// into an Explanation suitable for printing or further tooling.
package explainer

import "github.com/Ramana1908/whynot/pkg/history"

type Verdict string

const (
	VerdictLinearizable    Verdict = "linearizable"
	VerdictNonLinearizable Verdict = "non-linearizable"
	VerdictUnknown         Verdict = "unknown"
)

// Explanation is the user-facing output. It is JSON-serializable and
// pretty-printable. See DESIGN.md § 5.
type Explanation struct {
	Verdict     Verdict          `json:"verdict"`
	Pattern     string           `json:"pattern,omitempty"` // "" if no template fit
	Summary     string           `json:"summary"`           // one sentence, CI-log friendly
	Witness     *history.History `json:"witness,omitempty"` // minimized sub-history that still fails
	Conflicts   []Conflict       `json:"conflicts,omitempty"`
	Suggestions []string         `json:"suggestions,omitempty"`
}

// Conflict names a smoking-gun set of operations and explains why they
// cannot all be ordered.
type Conflict struct {
	OpIDs []int  `json:"op_ids"`
	Why   string `json:"why"`
}
