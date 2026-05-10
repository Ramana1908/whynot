package explainer_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Ramana1908/whynot/pkg/checker"
	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/model"
	"github.com/Ramana1908/whynot/pkg/pattern"
)

const propTimeout = 5 * time.Second

// genLinearizableRegister produces a register history that is trivially
// linearizable by construction: ops are non-overlapping in real time, and
// each read returns the value of the most recent preceding write (or init).
// Any correct checker MUST classify this as linearizable.
func genLinearizableRegister(rng *rand.Rand, init, nOps, nClients int) *history.History {
	if nOps < 1 {
		nOps = 1
	}
	if nClients < 1 {
		nClients = 1
	}
	ops := make([]history.Op, 0, nOps)
	cur := init
	var t int64 = 0
	for i := 0; i < nOps; i++ {
		// First op is forced to be a write so reads have something to see.
		isWrite := i == 0 || rng.Intn(2) == 0
		op := history.Op{
			ID:     i,
			Client: rng.Intn(nClients),
			Call:   t,
			Return: t + 1,
		}
		if isWrite {
			op.Type = history.OpWrite
			op.Value = rng.Intn(100)
			cur = op.Value
		} else {
			op.Type = history.OpRead
			op.Value = cur
		}
		ops = append(ops, op)
		t += 2
	}
	return &history.History{Model: "register", Init: init, Ops: ops}
}

// TestProperty_LinearizableHistoriesAccepted asserts that randomly-generated
// histories which are linearizable by construction are never rejected.
// This is the most important property: false positives would make the tool
// dangerous to use in CI.
func TestProperty_LinearizableHistoriesAccepted(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const trials = 100
	for trial := 0; trial < trials; trial++ {
		init := rng.Intn(5)
		nOps := 1 + rng.Intn(20)
		nClients := 1 + rng.Intn(4)
		h := genLinearizableRegister(rng, init, nOps, nClients)

		expl, err := explainer.Explain(h, matchers(), propTimeout)
		if err != nil {
			t.Fatalf("trial %d: explain: %v", trial, err)
		}
		if expl.Verdict != explainer.VerdictLinearizable {
			t.Errorf("trial %d (seed=1): linearizable history (n=%d, init=%d) classified %s; first op: %+v",
				trial, nOps, init, expl.Verdict, h.Ops[0])
		}
	}
}

// TestProperty_StaleReadAlwaysCaught asserts that for every random pair
// (write of v at [0,10], read of init at [20,30]) where v != init, the
// pipeline returns non-linearizable AND fires some matcher.
func TestProperty_StaleReadAlwaysCaught(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	const trials = 50
	for trial := 0; trial < trials; trial++ {
		init := rng.Intn(50)
		v := init + 1 + rng.Intn(50) // ensure v != init
		h := &history.History{
			Model: "register",
			Init:  init,
			Ops: []history.Op{
				{ID: 0, Client: 0, Type: history.OpWrite, Value: v, Call: 0, Return: 10},
				{ID: 1, Client: 1, Type: history.OpRead, Value: init, Call: 20, Return: 30},
			},
		}
		expl, err := explainer.Explain(h, matchers(), propTimeout)
		if err != nil {
			t.Fatalf("trial %d: %v", trial, err)
		}
		if expl.Verdict != explainer.VerdictNonLinearizable {
			t.Errorf("trial %d: stale read (init=%d v=%d) verdict=%s", trial, init, v, expl.Verdict)
		}
		if expl.Pattern == "" {
			t.Errorf("trial %d: stale read (init=%d v=%d) matched no pattern", trial, init, v)
		}
	}
}

// TestProperty_LostUpdateAlwaysCaught: two preceding writes, read returns
// the older write's value. Should always trip lost_update.
func TestProperty_LostUpdateAlwaysCaught(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	const trials = 50
	for trial := 0; trial < trials; trial++ {
		v1 := 1 + rng.Intn(50)
		v2 := v1 + 1 + rng.Intn(50) // distinct from v1
		h := &history.History{
			Model: "register",
			Init:  0,
			Ops: []history.Op{
				{ID: 0, Client: 0, Type: history.OpWrite, Value: v1, Call: 0, Return: 10},
				{ID: 1, Client: 1, Type: history.OpWrite, Value: v2, Call: 20, Return: 30},
				{ID: 2, Client: 2, Type: history.OpRead, Value: v1, Call: 40, Return: 50},
			},
		}
		expl, err := explainer.Explain(h, matchers(), propTimeout)
		if err != nil {
			t.Fatalf("trial %d: %v", trial, err)
		}
		if expl.Pattern != "lost_update" {
			t.Errorf("trial %d: v1=%d v2=%d: expected pattern lost_update, got %q", trial, v1, v2, expl.Pattern)
		}
	}
}

// TestProperty_WitnessIsSubsetOfInput: for every fixture, the explanation's
// witness contains only op IDs from the original history, in original order.
// This guards against minimization synthesizing or reordering ops.
func TestProperty_WitnessIsSubsetOfInput(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "histories")
	for _, name := range listFixtures(t, dir) {
		t.Run(name, func(t *testing.T) {
			h, err := history.Load(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			expl, err := explainer.Explain(h, matchers(), propTimeout)
			if err != nil {
				t.Fatal(err)
			}
			if expl.Witness == nil {
				return
			}
			origOrder := map[int]int{}
			for i, op := range h.Ops {
				origOrder[op.ID] = i
			}
			lastIdx := -1
			for _, op := range expl.Witness.Ops {
				idx, ok := origOrder[op.ID]
				if !ok {
					t.Errorf("witness op id %d not present in original history", op.ID)
					continue
				}
				if idx <= lastIdx {
					t.Errorf("witness ops out of original order: id %d at orig pos %d after pos %d", op.ID, idx, lastIdx)
				}
				lastIdx = idx
				orig := h.Ops[idx]
				if orig.Call != op.Call || orig.Return != op.Return || orig.Value != op.Value || orig.Type != op.Type {
					t.Errorf("witness op id %d differs from original: orig=%+v witness=%+v", op.ID, orig, op)
				}
			}
		})
	}
}

// TestProperty_WitnessAloneIsNonLinearizable: re-running the checker on the
// minimized witness must still produce Illegal. If minimization shrinks too
// far, this catches it.
func TestProperty_WitnessAloneIsNonLinearizable(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "histories")
	for _, name := range listFixtures(t, dir) {
		t.Run(name, func(t *testing.T) {
			h, err := history.Load(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			expl, err := explainer.Explain(h, matchers(), propTimeout)
			if err != nil {
				t.Fatal(err)
			}
			if expl.Verdict != explainer.VerdictNonLinearizable {
				return
			}
			if expl.Witness == nil {
				t.Fatalf("non-linearizable explanation must include a witness")
			}
			m, ops, err := model.SelectModel(expl.Witness)
			if err != nil {
				t.Fatalf("select model on witness: %v", err)
			}
			rt := checker.Check(m, ops, expl.Witness, propTimeout)
			if rt.Result != checker.Illegal {
				t.Errorf("witness alone classified %s, expected Illegal", rt.Result)
			}
		})
	}
}

// TestProperty_PatternFirstFitConsistent: the matcher named in the
// Explanation must itself fire when re-run on the original trace's
// rejection. Guards against the explainer reporting one pattern while
// internally another won.
func TestProperty_PatternFirstFitConsistent(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "histories")
	for _, name := range listFixtures(t, dir) {
		t.Run(name, func(t *testing.T) {
			h, err := history.Load(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			expl, err := explainer.Explain(h, matchers(), propTimeout)
			if err != nil {
				t.Fatal(err)
			}
			if expl.Pattern == "" {
				return
			}
			m, ops, err := model.SelectModel(h)
			if err != nil {
				t.Fatal(err)
			}
			rt := checker.Check(m, ops, h, propTimeout)
			if rt.Result != checker.Illegal {
				t.Fatalf("expected Illegal verdict from checker, got %s", rt.Result)
			}
			// Walk the matcher list in catalogue order; the first one that
			// fires should match the reported pattern (first-fit semantics).
			var firstFit string
			for _, mt := range pattern.All() {
				if _, ok := mt.Match(rt); ok {
					firstFit = mt.Name()
					break
				}
			}
			if firstFit != expl.Pattern {
				t.Errorf("first-fit matcher = %q, but explanation reported %q", firstFit, expl.Pattern)
			}
		})
	}
}

func listFixtures(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("list fixtures: %v", err)
	}
	out := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}
