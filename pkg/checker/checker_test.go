package checker

import (
	"testing"
	"time"

	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/model"
)

// linearizable history: write 1, then read returns 1.
func TestCheck_Ok(t *testing.T) {
	h := &history.History{
		Model: "register",
		Init:  0,
		Ops: []history.Op{
			{ID: 0, Client: 0, Type: history.OpWrite, Value: 1, Call: 0, Return: 10},
			{ID: 1, Client: 1, Type: history.OpRead, Value: 1, Call: 20, Return: 30},
		},
	}
	rt := Check(model.Register(h.Init), model.RegisterOps(h), h, 5*time.Second)
	if rt.Result != Ok {
		t.Fatalf("expected Ok, got %s", rt.Result)
	}
	if len(rt.Partitions) != 0 {
		t.Errorf("expected no partitions on Ok, got %d", len(rt.Partitions))
	}
}

// stale read: write 1 ack at t=10, read returns 0 at t=20.
// Expected: blocked op = read, value-mismatch, prefix = [write],
// ConflictWith = [write] (the establishing write).
func TestCheck_StaleRead_ValueMismatch(t *testing.T) {
	h := &history.History{
		Model: "register",
		Init:  0,
		Ops: []history.Op{
			{ID: 0, Client: 0, Type: history.OpWrite, Value: 1, Call: 0, Return: 10},
			{ID: 1, Client: 1, Type: history.OpRead, Value: 0, Call: 20, Return: 30},
		},
	}
	rt := Check(model.Register(h.Init), model.RegisterOps(h), h, 5*time.Second)
	if rt.Result != Illegal {
		t.Fatalf("expected Illegal, got %s", rt.Result)
	}
	if len(rt.Partitions) != 1 {
		t.Fatalf("expected 1 partition, got %d", len(rt.Partitions))
	}
	p := rt.Partitions[0]
	if len(p.BlockedOps) != 1 || p.BlockedOps[0] != 1 {
		t.Fatalf("expected BlockedOps=[1], got %v", p.BlockedOps)
	}
	br := p.Blocks[0]
	if br.Constraint != ConstraintValueMismatch {
		t.Errorf("expected constraint %q, got %q", ConstraintValueMismatch, br.Constraint)
	}
	if len(br.PrefixIDs) != 1 || br.PrefixIDs[0] != 0 {
		t.Errorf("expected PrefixIDs=[0] (the write), got %v", br.PrefixIDs)
	}
	if len(br.ConflictWith) != 1 || br.ConflictWith[0] != 0 {
		t.Errorf("expected ConflictWith=[0] (the establishing write), got %v", br.ConflictWith)
	}
}

// phantom value: read returns 1 from init=0 with no prior write.
// Porcupine returns 0 partials (the algorithm walks entries in time
// order; the bad read is the first call, fails Step, backtrack hits
// nothing). Both ops end up in BlockedOps with empty bestPrefix.
// Expected: read → value-mismatch with empty PrefixIDs and no
// ConflictWith (no write to blame); write → concurrent-conflict.
func TestCheck_PhantomValue_NoPartials(t *testing.T) {
	h := &history.History{
		Model: "register",
		Init:  0,
		Ops: []history.Op{
			{ID: 0, Client: 0, Type: history.OpRead, Value: 1, Call: 0, Return: 10},
			{ID: 1, Client: 1, Type: history.OpWrite, Value: 1, Call: 20, Return: 30},
		},
	}
	rt := Check(model.Register(h.Init), model.RegisterOps(h), h, 5*time.Second)
	if rt.Result != Illegal {
		t.Fatalf("expected Illegal, got %s", rt.Result)
	}
	p := rt.Partitions[0]
	if len(p.BlockedOps) != 2 {
		t.Fatalf("expected both ops blocked, got %v", p.BlockedOps)
	}
	if len(p.LongestPrefix) != 0 {
		t.Errorf("expected empty LongestPrefix, got %v", p.LongestPrefix)
	}

	byID := map[int]BlockReason{}
	for _, br := range p.Blocks {
		byID[br.OpID] = br
	}

	read := byID[0]
	if read.Constraint != ConstraintValueMismatch {
		t.Errorf("read: expected value-mismatch, got %q", read.Constraint)
	}
	if len(read.PrefixIDs) != 0 {
		t.Errorf("read: expected empty PrefixIDs, got %v", read.PrefixIDs)
	}
	if len(read.ConflictWith) != 0 {
		t.Errorf("read: expected empty ConflictWith (no write to blame), got %v", read.ConflictWith)
	}

	write := byID[1]
	if write.Constraint != ConstraintConcurrentConflict {
		t.Errorf("write: expected concurrent-conflict, got %q", write.Constraint)
	}
}

// downstream blockage: a legal write, an illegal read in the middle,
// and a downstream write. Porcupine reaches a partial of length 1
// containing the first write but cannot extend past the bad read,
// leaving the bad read AND the downstream write blocked.
// The downstream write should classify as concurrent-conflict
// (it would extend the prefix legally on its own).
func TestCheck_DownstreamConcurrentConflict(t *testing.T) {
	h := &history.History{
		Model: "register",
		Init:  0,
		Ops: []history.Op{
			{ID: 0, Client: 0, Type: history.OpWrite, Value: 5, Call: 0, Return: 10},
			{ID: 1, Client: 1, Type: history.OpRead, Value: 9, Call: 15, Return: 20},
			{ID: 2, Client: 2, Type: history.OpWrite, Value: 9, Call: 25, Return: 30},
		},
	}
	rt := Check(model.Register(h.Init), model.RegisterOps(h), h, 5*time.Second)
	if rt.Result != Illegal {
		t.Fatalf("expected Illegal, got %s", rt.Result)
	}
	p := rt.Partitions[0]
	if len(p.LongestPrefix) != 1 || p.LongestPrefix[0] != 0 {
		t.Fatalf("expected LongestPrefix=[0], got %v", p.LongestPrefix)
	}
	byID := map[int]BlockReason{}
	for _, br := range p.Blocks {
		byID[br.OpID] = br
	}
	if byID[1].Constraint != ConstraintValueMismatch {
		t.Errorf("op 1 (bad read): expected value-mismatch, got %q", byID[1].Constraint)
	}
	if byID[2].Constraint != ConstraintConcurrentConflict {
		t.Errorf("op 2 (downstream write): expected concurrent-conflict, got %q", byID[2].Constraint)
	}
}
