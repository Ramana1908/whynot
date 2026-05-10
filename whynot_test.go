package whynot_test

import (
	"strings"
	"testing"

	"github.com/Ramana1908/whynot"
)

// TestExplain_BuilderToVerdict is the README's "30-second integration"
// example as a test: build a stale-read history with no JSON, ask whynot
// to explain it, get a named-pattern verdict.
func TestExplain_BuilderToVerdict(t *testing.T) {
	h := whynot.NewBuilder("register").Init(0).
		Write(0, 1, 0, 10).
		Read(1, 0, 20, 30).
		Build()

	expl, err := whynot.Explain(h)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if expl.Verdict != whynot.VerdictNonLinearizable {
		t.Errorf("verdict = %s, want non-linearizable", expl.Verdict)
	}
	if expl.Pattern != "stale_read" {
		t.Errorf("pattern = %q, want stale_read", expl.Pattern)
	}
	if expl.Witness == nil || len(expl.Witness.Ops) == 0 {
		t.Error("expected non-empty witness")
	}
}

func TestExplainJSON_Roundtrip(t *testing.T) {
	src := `{
		"model": "register",
		"init": 0,
		"ops": [
			{"id": 0, "client": 0, "type": "write", "value": 1, "call": 0, "return": 10},
			{"id": 1, "client": 1, "type": "read",  "value": 0, "call": 20, "return": 30}
		]
	}`
	expl, err := whynot.ExplainJSON(strings.NewReader(src))
	if err != nil {
		t.Fatalf("explain JSON: %v", err)
	}
	if expl.Pattern != "stale_read" {
		t.Errorf("pattern = %q, want stale_read", expl.Pattern)
	}
}

func TestExplain_Linearizable(t *testing.T) {
	h := whynot.NewBuilder("register").Init(0).
		Write(0, 1, 0, 10).
		Read(0, 1, 20, 30).
		Build()
	expl, err := whynot.Explain(h)
	if err != nil {
		t.Fatal(err)
	}
	if expl.Verdict != whynot.VerdictLinearizable {
		t.Errorf("verdict = %s, want linearizable", expl.Verdict)
	}
}
