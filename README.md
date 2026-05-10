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

## Use whynot in your project

`whynot` is a Go module — drop it into an existing project as a library,
or install the CLI and pipe JSON histories through it from any language.

### As a Go library

```sh
go get github.com/Ramana1908/whynot
```

The top-level package gives you a one-call API. Construct a history with
the fluent builder, hand it to `Explain`, inspect the result:

```go
package main

import (
    "fmt"

    "github.com/Ramana1908/whynot"
)

func main() {
    h := whynot.NewBuilder("register").Init(0).
        Write(0, 1, 0, 10). // client 0: write(1) over [0,10]
        Read(1, 0, 20, 30). // client 1: read()->0 over [20,30]
        Build()

    expl, err := whynot.Explain(h)
    if err != nil {
        panic(err)
    }
    fmt.Println(expl.Verdict, expl.Pattern, expl.Summary)
    // non-linearizable stale_read Read (op 1) returned 0, but write of 1 ...
}
```

If your test harness already emits whynot's JSON schema, skip the
builder:

```go
expl, err := whynot.ExplainJSON(jsonReader)   // io.Reader
expl, err := whynot.ExplainFile("history.json")
```

The returned `*whynot.Explanation` has `Verdict`, `Pattern`, `Summary`,
`Witness` (minimized sub-history), `Conflicts`, and `Suggestions` —
everything the CLI prints, as plain Go fields you can route into your
own assertion or logging code.

### As a CLI (any language)

```sh
go install github.com/Ramana1908/whynot/cmd/explain@latest

explain history.json                # human-readable
explain -format json history.json   # machine-readable
```

Any language that can write JSON can drive `whynot` as a subprocess.
Exit codes: `0` success, `1` runtime error (load/explain), `2` usage
error.

### History JSON schema

If you'd rather emit JSON than build histories programmatically, the
schema is small. One file, one history:

```json
{
  "model": "register",
  "init": 0,
  "ops": [
    {"id": 0, "client": 0, "type": "write", "value": 1, "call": 0,  "return": 10},
    {"id": 1, "client": 1, "type": "read",  "value": 0, "call": 20, "return": 30}
  ]
}
```

- `model`: `"register"`, `"counter"`, or `"kv"` (KV ops carry an extra
  `"key"` field; the model partitions and checks per-key linearizability).
- `id`: unique per op; whynot uses these IDs in conflicts and witnesses.
- `call`/`return`: integer logical timestamps; the interval is closed.

See `testdata/histories/` for one example per pattern.

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
