# whynot

A linearizability *violation explainer*. Linearizability checkers like
Porcupine answer "is this history linearizable?" with yes or no. When
the answer is no, an engineer typically spends hours figuring out
*which* operations conflict and *why*. `whynot` turns that no into a
structured, human-readable explanation: which operations form the
smoking gun, a minimized witness sub-history, and a named pattern
when the violation matches one of five templates derived from real
Jepsen incidents.

## Pipeline

```
history.json
   → pkg/history (loader)
   → pkg/model (register | counter | kv)
   → pkg/checker (Porcupine + RejectionTrace)
   → pkg/pattern (5 matchers in catalogue order)
   → pkg/minimize (one-minimal delta debugging, pattern-preserving)
   → pkg/explainer (Explanation: verdict, pattern, witness, conflicts, suggestions)
```

See `DESIGN.md` for the design log (with rejected alternatives
recorded) and `eval/` for the empirical results.

## Try it

```sh
# Curated fixture — produces a stale_read explanation:
go run ./cmd/explain testdata/histories/stale_read_simple.json

# All five patterns:
for f in testdata/histories/*.json; do
  echo "=== $f ==="
  go run ./cmd/explain "$f"
done

# End-to-end: generate a Jepsen-style history from a buggy in-process
# register (stale-replica reads), then explain it:
go run ./scripts/genhist
go run ./cmd/explain testdata/runs/jepsen_kv.json
```

JSON output: `-format json`.

## Pattern catalogue

Each pattern has a formal predicate over the rejection trace
(see `DESIGN.md § 6`) and a positive synthetic fixture in
`testdata/histories/`:

| Pattern              | Predicate (informal)                                                   |
|----------------------|------------------------------------------------------------------------|
| `phantom_value`      | read returned a value no write produced                                |
| `realtime_inversion` | read sees a value only a future write produces                         |
| `non_monotonic_read` | a single client sees state move backwards                              |
| `lost_update`        | read sees an earlier write's value despite a later write completing    |
| `stale_read`         | read disagrees with the established state (catch-all)                  |

Catalogue derivation: a survey of 11 violations from 5 Jepsen
analyses, classified in `eval/incidents.md`. Coverage: 89% of in-scope
incidents.

## Test

```sh
go test ./...
```

Tests are stratified:

- `pkg/checker` — RejectionTrace shape on hand-crafted histories.
- `pkg/minimize` — ddmin shrinks/preserves correctly under different predicates.
- `pkg/explainer` — golden-file end-to-end tests, one per fixture in `testdata/`.

Update goldens with `go test ./pkg/explainer -update`.
