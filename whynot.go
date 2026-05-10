// Package whynot is the top-level convenience API for the linearizability
// violation explainer. It re-exports the most commonly needed types and
// provides one-call entry points so callers don't have to import
// pkg/history, pkg/pattern, and pkg/explainer separately.
//
// Typical usage:
//
//	import "github.com/Ramana1908/whynot"
//
//	h := whynot.NewBuilder("register").Init(0).
//	    Write(0, 1, 0, 10).
//	    Read(1, 0, 20, 30).
//	    Build()
//	expl, err := whynot.Explain(h)
//
// For raw JSON input (e.g. histories produced by another tool):
//
//	expl, err := whynot.ExplainJSON(reader)
package whynot

import (
	"io"
	"time"

	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/pattern"
)

// DefaultTimeout is the checker timeout used by Explain. Override with
// ExplainWithTimeout for long histories.
const DefaultTimeout = 30 * time.Second

// Explain runs the full pipeline (checker → pattern matchers →
// minimization) on h with the default catalogue and timeout.
func Explain(h *History) (*Explanation, error) {
	return explainer.Explain(h, pattern.AllMatchers(), DefaultTimeout)
}

// ExplainWithTimeout is Explain with a caller-supplied checker timeout.
func ExplainWithTimeout(h *History, timeout time.Duration) (*Explanation, error) {
	return explainer.Explain(h, pattern.AllMatchers(), timeout)
}

// ExplainJSON loads a history from r (whynot's JSON schema) and explains it.
func ExplainJSON(r io.Reader) (*Explanation, error) {
	h, err := history.Read(r)
	if err != nil {
		return nil, err
	}
	return Explain(h)
}

// ExplainFile loads a history from a JSON file and explains it.
func ExplainFile(path string) (*Explanation, error) {
	h, err := history.Load(path)
	if err != nil {
		return nil, err
	}
	return Explain(h)
}

// NewBuilder starts a fluent History builder for the given model
// ("register" | "counter" | "kv"). See pkg/history.Builder.
func NewBuilder(model string) *Builder {
	return history.NewBuilder(model)
}

// Re-exported types so a caller importing only "whynot" has access to
// everything they need to inspect a result.
type (
	History     = history.History
	Op          = history.Op
	OpKind      = history.OpKind
	Builder     = history.Builder
	Explanation = explainer.Explanation
	Verdict     = explainer.Verdict
	Conflict    = explainer.Conflict
)

// Re-exported constants.
const (
	OpRead  = history.OpRead
	OpWrite = history.OpWrite
	OpCAS   = history.OpCAS

	VerdictLinearizable    = explainer.VerdictLinearizable
	VerdictNonLinearizable = explainer.VerdictNonLinearizable
	VerdictUnknown         = explainer.VerdictUnknown
)
