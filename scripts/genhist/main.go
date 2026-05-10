// Command genhist runs a Jepsen-style workload against a deliberately
// buggy in-process register and emits the resulting history as JSON.
//
// Five bugs are available via -bug:
//
//	stale     — single stale-replica read           → stale_read
//	lost      — two writes, second's effect lost    → lost_update
//	inversion — clock-skew on the reading client    → realtime_inversion
//	nmr       — per-client follower flip            → non_monotonic_read
//	phantom   — buffer-reuse: read returns sentinel → phantom_value
//	concurrent — original concurrent stale-replica register (timing-dependent)
//
// Usage:
//
//	go run ./scripts/genhist -bug stale
//	go run ./scripts/genhist -bug concurrent -out testdata/runs/jepsen_kv.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Ramana1908/whynot/pkg/history"
)

// generate dispatches to the bug-specific workload.
func generate(bug string, seed int64) (*history.History, error) {
	switch bug {
	case "stale":
		return runStaleRead(), nil
	case "lost":
		return runLostUpdate(), nil
	case "inversion":
		return runRealtimeInversion(), nil
	case "nmr":
		return runNonMonotonic(), nil
	case "phantom":
		return runPhantomValue(), nil
	case "concurrent":
		return runConcurrentStaleReplica(seed, 4, 6), nil
	default:
		return nil, fmt.Errorf("unknown -bug %q (want: stale|lost|inversion|nmr|phantom|concurrent)", bug)
	}
}

func main() {
	bug := flag.String("bug", "stale", "which bug to simulate: stale|lost|inversion|nmr|phantom|concurrent")
	out := flag.String("out", "", "output history path (default: testdata/runs/jepsen_<bug>.json)")
	seed := flag.Int64("seed", 1, "rng seed for the concurrent driver")
	flag.Parse()

	h, err := generate(*bug, *seed)
	if err != nil {
		fail(err)
	}

	path := *out
	if path == "" {
		path = fmt.Sprintf("testdata/runs/jepsen_%s.json", *bug)
	}

	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		fail(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s (%d ops, bug=%s)\n", path, len(h.Ops), *bug)
}

// --- legacy concurrent driver (kept verbatim for parity) -------------------

type StaleReplicaKV struct {
	leader   atomic.Int64
	follower atomic.Int64
	delay    time.Duration
}

func (kv *StaleReplicaKV) Write(v int) {
	kv.leader.Store(int64(v))
	go func() {
		time.Sleep(kv.delay)
		kv.follower.Store(int64(v))
	}()
}

func (kv *StaleReplicaKV) Read() int {
	return int(kv.follower.Load())
}

type recorder struct {
	mu  sync.Mutex
	ops []history.Op
	id  atomic.Int32
	t0  time.Time
}

func (r *recorder) record(client int, kind history.OpKind, value int, call, ret time.Time) {
	id := int(r.id.Add(1)) - 1
	op := history.Op{
		ID:     id,
		Client: client,
		Type:   kind,
		Value:  value,
		Call:   call.Sub(r.t0).Microseconds(),
		Return: ret.Sub(r.t0).Microseconds(),
	}
	r.mu.Lock()
	r.ops = append(r.ops, op)
	r.mu.Unlock()
}

func runConcurrentStaleReplicaImpl(seed int64, clients, opsPerClient int) *history.History {
	rng := newDeterministicRand(seed)
	kv := &StaleReplicaKV{delay: 30 * time.Millisecond}
	rec := &recorder{t0: time.Now()}

	var wg sync.WaitGroup
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			for i := 0; i < opsPerClient; i++ {
				time.Sleep(time.Duration(rng.Intn(int(10 * time.Millisecond))))
				if rng.Intn(2) == 0 {
					v := clientID*100 + i + 1
					call := time.Now()
					kv.Write(v)
					ret := time.Now()
					rec.record(clientID, history.OpWrite, v, call, ret)
				} else {
					call := time.Now()
					v := kv.Read()
					ret := time.Now()
					rec.record(clientID, history.OpRead, v, call, ret)
				}
			}
		}(c)
	}
	wg.Wait()

	rec.mu.Lock()
	ops := append([]history.Op(nil), rec.ops...)
	rec.mu.Unlock()
	sortByCall(ops)
	return &history.History{Model: "register", Init: 0, Ops: ops}
}

func sortByCall(ops []history.Op) {
	for i := 1; i < len(ops); i++ {
		for j := i; j > 0 && ops[j-1].Call > ops[j].Call; j-- {
			ops[j-1], ops[j] = ops[j], ops[j-1]
		}
	}
}

// xorshift, sufficient for sequencing think-time choices in the
// concurrent driver.
type deterministicRand struct{ state int64 }

func newDeterministicRand(seed int64) *deterministicRand {
	return &deterministicRand{state: seed}
}
func (r *deterministicRand) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	r.state ^= r.state << 13
	r.state ^= int64(uint64(r.state) >> 7)
	r.state ^= r.state << 17
	v := int(r.state)
	if v < 0 {
		v = -v
	}
	return v % n
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
