package porcupine_test

import (
	"strings"
	"testing"

	"github.com/Ramana1908/whynot"
	"github.com/Ramana1908/whynot/pkg/adapter/porcupine"
	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/model"
	pcp "github.com/anishathalye/porcupine"
)

// TestRegisterRoundTrip: build a history with whynot's Builder, project
// it through model.RegisterOps to []porcupine.Operation, and bring it
// back through the adapter. The reconstructed history must match the
// original op-for-op.
func TestRegisterRoundTrip(t *testing.T) {
	orig := whynot.NewBuilder("register").Init(0).
		Write(0, 1, 0, 10).
		Read(1, 1, 20, 30).
		Write(0, 2, 40, 50).
		Read(1, 2, 60, 70).
		Build()

	pops := model.RegisterOps(orig)

	got, err := porcupine.FromOperations(pops, "register", orig.Init, porcupine.RegisterTranslator)
	if err != nil {
		t.Fatalf("FromOperations: %v", err)
	}
	assertHistoriesEqual(t, orig, got)
}

func TestKVRoundTrip(t *testing.T) {
	orig := whynot.NewBuilder("kv").
		WriteKey(0, "a", 1, 0, 10).
		ReadKey(1, "a", 1, 20, 30).
		WriteKey(0, "b", 7, 40, 50).
		ReadKey(1, "b", 7, 60, 70).
		Build()

	pops := model.KVOps(orig)

	got, err := porcupine.FromOperations(pops, "kv", 0, porcupine.KVTranslator)
	if err != nil {
		t.Fatalf("FromOperations: %v", err)
	}
	assertHistoriesEqual(t, orig, got)
}

// TestExplainAfterAdapter: end-to-end — adapter output flows straight
// into whynot.Explain and yields the expected verdict.
func TestExplainAfterAdapter(t *testing.T) {
	orig := whynot.NewBuilder("register").Init(0).
		Write(0, 1, 0, 10).
		Read(1, 0, 20, 30). // stale
		Build()

	pops := model.RegisterOps(orig)
	h, err := porcupine.FromOperations(pops, "register", 0, porcupine.RegisterTranslator)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	expl, err := whynot.Explain(h)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if expl.Pattern != "stale_read" {
		t.Errorf("pattern = %q, want stale_read", expl.Pattern)
	}
}

// TestCustomTranslator: a project using a custom Porcupine model can
// supply its own translator. Here we mimic an "Op" string convention:
// Input is "r" or "w<value>", Output is the read result.
func TestCustomTranslator(t *testing.T) {
	pops := []pcp.Operation{
		{ClientId: 0, Input: "w1", Output: nil, Call: 0, Return: 10, Metadata: 100},
		{ClientId: 1, Input: "r", Output: 0, Call: 20, Return: 30, Metadata: 101},
	}
	tr := func(op pcp.Operation) (history.OpKind, int, string, error) {
		s := op.Input.(string)
		if s == "r" {
			return history.OpRead, op.Output.(int), "", nil
		}
		// "w<digit>"
		v := int(s[1] - '0')
		return history.OpWrite, v, "", nil
	}

	h, err := porcupine.FromOperations(pops, "register", 0, tr)
	if err != nil {
		t.Fatalf("FromOperations: %v", err)
	}
	if len(h.Ops) != 2 {
		t.Fatalf("len(ops) = %d", len(h.Ops))
	}
	if h.Ops[0].ID != 100 || h.Ops[1].ID != 101 {
		t.Errorf("Metadata IDs not preserved: %d %d", h.Ops[0].ID, h.Ops[1].ID)
	}
	expl, err := whynot.Explain(h)
	if err != nil {
		t.Fatal(err)
	}
	if expl.Verdict != whynot.VerdictNonLinearizable {
		t.Errorf("verdict = %s, want non-linearizable", expl.Verdict)
	}
}

func TestTranslatorErrorPropagated(t *testing.T) {
	pops := []pcp.Operation{
		{ClientId: 0, Input: "this is not a RegisterInput", Output: 0, Call: 0, Return: 1},
	}
	_, err := porcupine.FromOperations(pops, "register", 0, porcupine.RegisterTranslator)
	if err == nil {
		t.Fatal("expected error for wrong input type")
	}
	if !strings.Contains(err.Error(), "RegisterInput") {
		t.Errorf("error should mention RegisterInput; got %v", err)
	}
}

func TestNilTranslator(t *testing.T) {
	_, err := porcupine.FromOperations(nil, "register", 0, nil)
	if err == nil {
		t.Fatal("expected error for nil translator")
	}
}

func TestDuplicateMetadataRejected(t *testing.T) {
	pops := []pcp.Operation{
		{ClientId: 0, Input: model.RegisterInput{Op: false, Value: 1}, Output: 0, Call: 0, Return: 10, Metadata: 7},
		{ClientId: 1, Input: model.RegisterInput{Op: true}, Output: 1, Call: 20, Return: 30, Metadata: 7},
	}
	_, err := porcupine.FromOperations(pops, "register", 0, porcupine.RegisterTranslator)
	if err == nil {
		t.Fatal("expected error on duplicate Metadata IDs")
	}
}

func assertHistoriesEqual(t *testing.T, want, got *history.History) {
	t.Helper()
	if want.Model != got.Model || want.Init != got.Init {
		t.Errorf("model/init mismatch: want %s/%d got %s/%d", want.Model, want.Init, got.Model, got.Init)
	}
	if len(want.Ops) != len(got.Ops) {
		t.Fatalf("op count: want %d got %d", len(want.Ops), len(got.Ops))
	}
	for i := range want.Ops {
		w, g := want.Ops[i], got.Ops[i]
		if w != g {
			t.Errorf("op %d differ:\n  want %+v\n  got  %+v", i, w, g)
		}
	}
}
