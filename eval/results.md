# Evaluation Results

This file is the empirical output of the whynot pipeline. Two parts:

1. **Synthetic fixtures.** A pattern-by-pattern table of curated
   histories with their expected and produced explanations. These are
   the same fixtures backing the golden tests in
   `pkg/explainer/explainer_test.go`.
2. **End-to-end Jepsen-style run.** A buggy in-process register
   driven concurrently by multiple clients, history fed through the
   full pipeline. See `scripts/genhist/main.go`.

## Synthetic-fixture results

| Fixture | Ops | Verdict | Pattern matched | Witness size |
|---|---:|---|---|---:|
| `linearizable_control.json` | 4 | linearizable | — | n/a |
| `stale_read_simple.json` | 2 | non-linearizable | `stale_read` | 2 |
| `lost_update.json` | 3 | non-linearizable | `lost_update` | 3 |
| `realtime_inversion.json` | 3 | non-linearizable | `realtime_inversion` | 2 |
| `non_monotonic.json` | 4 | non-linearizable | `non_monotonic_read` | 4 |
| `phantom_value.json` | 2 | non-linearizable | `phantom_value` | 1 |

All six results match the goldens in `testdata/golden/`. Coverage is
intentional — every pattern in the catalogue has a positive fixture,
and the `linearizable_control` fixture verifies the explainer does
not falsely reject correct histories.

## End-to-end: stale-replica register under concurrent load

### Setup

`scripts/genhist` runs an in-process register implementation with the
following deliberate bug:

- Writes update a `leader` atomic int64 immediately.
- A goroutine asynchronously copies the new value to a `follower`
  atomic int64 after a fixed replication delay (default 30 ms).
- All reads go to the `follower` replica.

Default workload: 4 concurrent clients × 6 operations each, mixed
reads and writes, 24 ops total. Replication delay 30 ms, mean client
think time 10 ms. Timestamps are wall-clock microseconds since
workload start.

### Pipeline output

A representative run (the bug fires whenever a read lands inside a
30 ms post-write replication window):

```
$ go run ./scripts/genhist
wrote testdata/runs/jepsen_kv.json (24 ops, 4 clients, replication delay 30ms)

$ go run ./cmd/explain testdata/runs/jepsen_kv.json
verdict: non-linearizable
pattern: lost_update

Read (op 11) returned 0, but two writes (ops 9, 10) had completed;
the second write's effect (value 303) is not visible.

conflicts:
  ops [9 10 11]:
    Both writes returned before the read started (op 9 at t=12916,
    op 10 at t=13199); the read at t=14088 returned 0, ignoring
    op 10's update of 303.

witness (3 ops):
  [t=12914..12916] client 0: write(5)
  [t=13198..13199] client 3: write(303)
  [t=14088..14090] client 2: read() -> 0

suggestions:
  - Check for last-write-wins races without proper synchronization
  - Check for read-from-stale-replica that hasn't seen op 10
```

### Observations

- The **pipeline correctly classified** the violation as
  `lost_update`. The actual bug (read-from-stale-replica) directly
  produced the `lost_update` shape: two writes returned before the
  read, but the read sees the pre-write state.
- The **second suggestion** literally names the bug:
  *"Check for read-from-stale-replica that hasn't seen op 10."*
  This is the kind of actionable output the project aimed for.
- The **witness** was minimized from 24 ops to 3 — the two writes
  immediately preceding the stale read, plus the read itself. An
  engineer reading the CI log can focus on those three ops without
  scanning the full history.
- The **op IDs in the conflicts and the witness agree** because the
  matcher re-runs on the minimized history (see
  `pkg/explainer/explainer.go`). Earlier prototypes did not, and
  produced a confusing mismatch.

### Limitations of this run

- N=1. A single demonstration. The result is reproducible on this
  machine but the timing-driven bug means the *specific* op IDs
  vary across runs.
- The bug is constructed to fire `lost_update`. A real evaluation
  would ideally include workloads constructed to fire each pattern
  and a workload that should be linearizable (negative control on
  the runtime side, complementing the synthetic positive control
  in `linearizable_control.json`).
- No measurement of debugging-time savings. The paper-shaped version
  (DESIGN.md § 8) calls for at least informal user-study data:
  "with the explainer vs. without, time to first hypothesis." That
  is future work and not part of this 7-day deliverable.

## Catalogue empirical signal (combined)

Combining the synthetic positive fixtures with `eval/incidents.md`:

| Pattern              | Synthetic | Jepsen incidents (eval/incidents.md) | End-to-end demo |
|----------------------|:---------:|:------------------------------------:|:---------------:|
| `stale_read`         | ✓         | 4                                    |                 |
| `lost_update`        | ✓         | 3                                    | ✓               |
| `non_monotonic_read` | ✓         | 1                                    |                 |
| `phantom_value`      | ✓         | 0 (etcd watch is a weak fit)         |                 |
| `realtime_inversion` | ✓         | 0                                    |                 |

The catalogue is well-supported empirically for the first three
patterns and has synthetic-only coverage for the last two. See
`eval/incidents.md` § Findings for the implications.
