# whynot

A linearizability *violation explainer*. Linearizability checkers like
Porcupine answer "is this history linearizable?" with yes or no. When
the answer is no, an engineer typically spends hours figuring out
*which* operations conflict and *why*. `whynot` turns that no into a
structured, human-readable explanation.

Status: Day 1 of a 7-day project. Baseline driver works. Real
explainer logic is not yet implemented. See `DESIGN.md`.

## Try the baseline

```
go run ./cmd/explain testdata/histories/stale_read_simple.json
```

This loads a known-bad register history, hands it to Porcupine, and
prints the uninformative output that motivates the rest of the project.
