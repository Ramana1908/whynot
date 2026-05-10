package main

// Bug-specific workload generators. Each runX function returns a
// deterministic history that fires its named pattern when fed through
// the whynot pipeline. Determinism matters: every bug here is covered
// by main_test.go, which asserts that the produced history matches the
// intended pattern.
//
// The "buggy register" framing is shared: a small in-memory storage
// type with a deliberate flaw, exercised by a logical-time scheduler
// that records each op's call/return interval.

import (
	"github.com/Ramana1908/whynot/pkg/history"
)

// step assigns each op a Call/Return interval and advances logical time.
type clock struct {
	now      int64
	dur      int64 // op duration
	gap      int64 // inter-op gap
	nextID   int
	clientOf int // last client; not used here but kept for symmetry
}

func newClock() *clock { return &clock{dur: 10, gap: 10} }

func (c *clock) interval() (call, ret int64) {
	call = c.now
	ret = c.now + c.dur
	c.now = ret + c.gap
	return
}

// skipped advances time without recording an op (e.g., to model
// background replication lag).
func (c *clock) skip(d int64) { c.now += d }

func (c *clock) issue(client int, kind history.OpKind, value int) history.Op {
	call, ret := c.interval()
	op := history.Op{ID: c.nextID, Client: client, Type: kind, Value: value, Call: call, Return: ret}
	c.nextID++
	return op
}

// runStaleRead — single write completes, follower hasn't replicated yet,
// next read returns init value. Fires `stale_read`.
func runStaleRead() *history.History {
	c := newClock()
	w := c.issue(0, history.OpWrite, 5)
	r := c.issue(1, history.OpRead, 0) // follower still at init
	return &history.History{Model: "register", Init: 0, Ops: []history.Op{w, r}}
}

// runLostUpdate — two writes from different clients both complete, then a
// read sees the FIRST write's value (the second write's effect was lost,
// e.g. dropped during replication). Fires `lost_update`.
func runLostUpdate() *history.History {
	c := newClock()
	w1 := c.issue(0, history.OpWrite, 5)
	w2 := c.issue(1, history.OpWrite, 99) // dropped during replication
	r := c.issue(2, history.OpRead, 5)    // sees w1, not w2
	return &history.History{Model: "register", Init: 0, Ops: []history.Op{w1, w2, r}}
}

// runRealtimeInversion — read returns a value that only a *later* write
// produces. Models a clock-skew bug: the read client's clock runs slow,
// so the recorded read interval ends before the write that produced its
// value started. Storage is correct; the bug is in the timestamps.
// Fires `realtime_inversion`.
func runRealtimeInversion() *history.History {
	// Construct directly so we control the inversion: read at [5,15],
	// write of 5 at [20,30]. The read returns the value the write
	// produces, but its return precedes the write's call.
	ops := []history.Op{
		{ID: 0, Client: 1, Type: history.OpRead, Value: 5, Call: 5, Return: 15},
		{ID: 1, Client: 0, Type: history.OpWrite, Value: 5, Call: 20, Return: 30},
	}
	return &history.History{Model: "register", Init: 0, Ops: ops}
}

// runNonMonotonic — single client reads twice; first read goes to a fresh
// follower (sees 5), second read goes to a stale follower (sees init).
// Models per-client load-balancer routing flipping mid-session. Fires
// `non_monotonic_read`.
func runNonMonotonic() *history.History {
	c := newClock()
	w := c.issue(0, history.OpWrite, 5)  // client 0 writes 5
	c.skip(5)                            // wait for follower-1 to catch up
	r1 := c.issue(1, history.OpRead, 5)  // client 1 → follower-1, sees 5
	r2 := c.issue(1, history.OpRead, 0)  // client 1 → follower-2 (stale), sees 0
	return &history.History{Model: "register", Init: 0, Ops: []history.Op{w, r1, r2}}
}

// runPhantomValue — read returns a value no client ever wrote. Models
// a buffer-reuse / response-routing bug where the read response carried
// stale memory. Fires `phantom_value`.
func runPhantomValue() *history.History {
	c := newClock()
	r := c.issue(0, history.OpRead, 99) // 99 was never written; init was 0
	return &history.History{Model: "register", Init: 0, Ops: []history.Op{r}}
}

// runConcurrentStaleReplica is the original Jepsen-style concurrent
// driver kept for historical parity with eval/results.md. It runs the
// stale-replica KV from real goroutines and produces lost_update or
// stale_read depending on timing. Used by the legacy default.
func runConcurrentStaleReplica(seed int64, clients, opsPerClient int) *history.History {
	return runConcurrentStaleReplicaImpl(seed, clients, opsPerClient)
}
