package minimize_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/minimize"
	"github.com/Ramana1908/whynot/pkg/model"
	"github.com/Ramana1908/whynot/pkg/pattern"
)

func loadHistory(t *testing.T, name string) *history.History {
	t.Helper()
	h, err := history.Load(filepath.Join("..", "..", "testdata", "histories", name))
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return h
}

func illegalPredicate(sub *history.History) bool {
	m, ops, err := model.SelectModel(sub)
	if err != nil {
		return false
	}
	rt := checker.Check(m, ops, sub, 5*time.Second)
	return rt.Result == checker.Illegal
}

func patternPredicate(name string) minimize.Predicate {
	matchers := pattern.All()
	var matcher pattern.Matcher
	for _, m := range matchers {
		if m.Name() == name {
			matcher = m
			break
		}
	}
	return func(sub *history.History) bool {
		m, ops, err := model.SelectModel(sub)
		if err != nil {
			return false
		}
		rt := checker.Check(m, ops, sub, 5*time.Second)
		if rt.Result != checker.Illegal {
			return false
		}
		_, ok := matcher.Match(rt)
		return ok
	}
}

// On the lost_update fixture (3 ops: w1, w2, r=1), the over-permissive
// "any-illegal" predicate shrinks all the way to {r=1}, a single-op
// history that is itself non-linearizable (phantom value: read returns
// 1 from init=0 with no writes). This test is here to document the
// hazard: a predicate that ignores pattern semantics over-minimizes,
// erasing the structure that produced the original explanation. The
// pipeline uses pattern-preserving predicates instead.
func TestDDMin_AnyIllegal_OverMinimizesToPhantom(t *testing.T) {
	h := loadHistory(t, "lost_update.json")
	got := minimize.DDMin(h, illegalPredicate)
	if len(got.Ops) != 1 {
		t.Fatalf("expected 1 op (over-minimization to phantom read), got %d: %+v", len(got.Ops), got.Ops)
	}
	if got.Ops[0].ID != 2 || got.Ops[0].Type != history.OpRead {
		t.Errorf("expected the read (id 2) as the residue, got %+v", got.Ops[0])
	}
}

// With the lost_update predicate, minimization cannot shrink: all 3
// ops are required for the pattern to fit (predicate needs 2 writes
// preceding the read).
func TestDDMin_PreservesLostUpdatePattern(t *testing.T) {
	h := loadHistory(t, "lost_update.json")
	got := minimize.DDMin(h, patternPredicate("lost_update"))
	if len(got.Ops) != 3 {
		t.Errorf("expected 3 ops preserved, got %d: %+v", len(got.Ops), got.Ops)
	}
}

// On a noisy history (lost_update + a linearizable pair appended),
// minimization with the lost_update predicate should drop the noise
// and recover the 3-op witness.
func TestDDMin_DropsNoise(t *testing.T) {
	noisy := &history.History{
		Model: "register",
		Init:  0,
		Ops: []history.Op{
			{ID: 0, Client: 0, Type: history.OpWrite, Value: 1, Call: 0, Return: 10},
			{ID: 1, Client: 1, Type: history.OpWrite, Value: 2, Call: 20, Return: 30},
			{ID: 2, Client: 2, Type: history.OpRead, Value: 1, Call: 40, Return: 50},
			// Noise: a read after the bad one that is consistent with state=2.
			{ID: 3, Client: 3, Type: history.OpRead, Value: 2, Call: 60, Return: 70},
			// More noise: another consistent read.
			{ID: 4, Client: 4, Type: history.OpRead, Value: 2, Call: 80, Return: 90},
		},
	}
	got := minimize.DDMin(noisy, patternPredicate("lost_update"))
	if len(got.Ops) != 3 {
		t.Errorf("expected minimization to recover 3-op witness, got %d ops: %+v", len(got.Ops), got.Ops)
	}
	keep := map[int]bool{}
	for _, op := range got.Ops {
		keep[op.ID] = true
	}
	for _, id := range []int{0, 1, 2} {
		if !keep[id] {
			t.Errorf("expected op %d preserved", id)
		}
	}
	for _, id := range []int{3, 4} {
		if keep[id] {
			t.Errorf("expected op %d (noise) dropped", id)
		}
	}
}

// Minimizing a linearizable history with the illegal predicate is
// nonsensical (predicate fails on input); DDMin should return the
// input unchanged.
func TestDDMin_PredicateFalseOnInput_NoOp(t *testing.T) {
	h := loadHistory(t, "linearizable_control.json")
	got := minimize.DDMin(h, illegalPredicate)
	if len(got.Ops) != len(h.Ops) {
		t.Errorf("expected unchanged history, got %d ops vs original %d", len(got.Ops), len(h.Ops))
	}
}
