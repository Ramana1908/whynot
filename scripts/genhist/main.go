// Command genhist runs a Jepsen-style workload against a deliberately
// buggy in-process register and emits the resulting history as JSON.
//
// The bug: writes go to a "leader" replica; reads go to a "follower"
// replica that lags by a fixed replication delay. A read can return
// the previous value of the register if it lands in the replication
// window, producing a stale read.
//
// Usage:
//   go run ./scripts/genhist -out testdata/histories/jepsen_kv.json
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

func main() {
	out := flag.String("out", "testdata/runs/jepsen_kv.json", "output history path")
	clients := flag.Int("clients", 4, "concurrent clients")
	opsPerClient := flag.Int("ops", 6, "ops per client")
	delay := flag.Duration("delay", 30*time.Millisecond, "replication delay (the bug)")
	think := flag.Duration("think", 10*time.Millisecond, "client mean think time between ops")
	seed := flag.Int64("seed", 1, "rng seed")
	flag.Parse()

	rng := newDeterministicRand(*seed)
	kv := &StaleReplicaKV{delay: *delay}
	rec := &recorder{t0: time.Now()}

	var wg sync.WaitGroup
	for c := 0; c < *clients; c++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			for i := 0; i < *opsPerClient; i++ {
				time.Sleep(time.Duration(rng.Intn(int(*think))))
				if rng.Intn(2) == 0 {
					v := clientID*100 + i + 1 // unique per client+iteration
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

	// Sort ops by call time for nicer reading. ID stays stable.
	rec.mu.Lock()
	ops := append([]history.Op(nil), rec.ops...)
	rec.mu.Unlock()
	sortByCall(ops)

	h := history.History{Model: "register", Init: 0, Ops: ops}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		fail(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(*out, b, 0644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s (%d ops, %d clients, replication delay %v)\n", *out, len(ops), *clients, *delay)
}

func sortByCall(ops []history.Op) {
	for i := 1; i < len(ops); i++ {
		for j := i; j > 0 && ops[j-1].Call > ops[j].Call; j-- {
			ops[j-1], ops[j] = ops[j], ops[j-1]
		}
	}
}

// newDeterministicRand returns an *rand.Rand from math/rand seeded
// reproducibly. Imported as a small helper so the workload is
// deterministic given the same seed/clients/ops/delay/think.
type deterministicRand struct {
	state int64
}

func newDeterministicRand(seed int64) *deterministicRand {
	return &deterministicRand{state: seed}
}

// xorshift, sufficient for sequencing think-time choices.
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
