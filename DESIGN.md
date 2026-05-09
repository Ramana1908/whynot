# whynot — Design

A linearizability *violation explainer*. Given a history that a checker has
rejected, produce a human-readable account of which operations conflict, why
no valid ordering exists for them, a minimized sub-history that still fails,
and (when one fits) a named pattern.

This document is a living design log. When a decision is made, the rejected
alternative and the reason are recorded inline.

---

## 1. Goals and non-goals

### Goals

1. Take a non-linearizable history of operations on a single integer
   register, counter, or key-value store, and produce a structured
   explanation: conflicting operations, minimized witness sub-history,
   and (when applicable) a named pattern.
2. Be useful as a debugging tool — an engineer should reach an
   actionable diagnosis faster with the explainer than without.
3. Be paper-shaped: the architecture, taxonomy, and evaluation should
   support a workshop-paper write-up (target: PaPoC) without rework.

### Non-goals

- A new linearizability checker. We use Porcupine.
- Strict serializability or transactional anomaly classification (write
  skew, dirty read). Future work; tracked here so we don't accidentally
  scope-creep into it.
- Production performance. The target is debug-time tooling on
  histories of up to a few thousand operations.
- Arbitrary user-defined data types. Register, counter, KV only.

---

## 2. Pipeline at a glance

```
                    +------------------+
  history.json -->  | pkg/history      |
                    +--------+---------+
                             |
                             v
                    +------------------+
                    | pkg/model        |  (register | counter | kv)
                    +--------+---------+
                             |
                             v
                    +------------------+
                    | pkg/checker      |  wraps Porcupine
                    | -> RejectionTrace|
                    +--------+---------+
                             |
                             v
                    +------------------+
                    | pkg/minimize     |  ddmin on history; re-runs checker
                    +--------+---------+
                             |
                             v
                    +------------------+
                    | pkg/pattern      |  matches templates against trace
                    +--------+---------+
                             |
                             v
                    +------------------+
                    | pkg/explainer    |  assembles final Explanation
                    +--------+---------+
                             |
                             v
                    Explanation (JSON / text)
```

The pipeline is one-directional. Each stage's output is the next stage's
sole input. This is what makes the system testable: every stage can be
unit-tested with a hand-written input and expected output.

---

## 3. History data model

`pkg/history.History` is the canonical representation. It is JSON-encoded
in `testdata/` and on the CLI.

```go
type History struct {
    Model string // "register" | "counter" | "kv"
    Init  int    // initial state (model-dependent)
    Ops   []Op
}

type Op struct {
    ID       int    // stable across the run
    Client   int
    Type     OpKind // "read" | "write" | "cas"
    Key      string // KV only
    Value    int    // read: returned value; write: written value
    Expected int    // cas: expected value
    Call     int64  // logical timestamp, closed interval
    Return   int64
}
```

Decisions:

- **Logical timestamps, not wall-clock.** The interval semantics
  (closed) match Porcupine. Logical lets us author fixtures by hand.
- **Stable IDs.** The whole pipeline refers to operations by `Op.ID`,
  never by position. Minimization removes ops; positions shift; IDs do
  not.
- **Single `Value` field for reads and writes.** Slight overload but
  keeps JSON terse and avoids a sum-type. Documented at the field.

**Rejected alternative:** Porcupine's event-pair format directly. Rejected
because authoring fixtures becomes much noisier (two entries per op) and
the operation-with-interval form is what Porcupine itself recommends for
testing.

---

## 4. RejectionTrace — the checker/explainer contract

This is the central data structure. It is what `pkg/checker` produces and
what every downstream stage consumes. Getting this right is the most
important decision in the project, because it defines what kinds of
explanation become possible.

```go
// pkg/checker.RejectionTrace
type RejectionTrace struct {
    History *history.History       // the input
    Result  Result                  // Ok | Illegal | Unknown

    // Per partition (single partition for register/counter; per-key for KV).
    Partitions []PartitionTrace
}

type PartitionTrace struct {
    PartitionKey string             // "" for non-partitioned models

    // Longest legal prefix found by the search. IDs index into History.Ops.
    LongestPrefix []int

    // Partial linearizations: each is a sequence of op IDs that the search
    // confirmed was legal up to some point. From porcupine's
    // PartialLinearizationsOperations, mapped back to our op IDs.
    PartialLinearizations [][]int

    // Operations the search could *never* incorporate into any legal
    // linearization explored. Derived: ops that appear in History but in
    // no PartialLinearization.
    BlockedOps []int

    // For each blocked op, the closest near-miss: the longest legal
    // prefix the checker had reached when it failed to extend with this
    // op, along with the constraint that was violated.
    Blocks []BlockReason
}

type BlockReason struct {
    OpID         int       // the op that wouldn't fit
    PrefixIDs    []int     // the legal prefix at the moment of conflict
    State        any       // model state after the prefix (rendered via DescribeState)
    Constraint   string    // "real-time-precedes" | "value-mismatch" | "concurrent-conflict"
    ConflictWith []int     // op IDs of the prefix elements directly involved
    Detail       string    // human-readable, e.g. "read returned 0; state was 1"
}
```

### Why this shape

The whole project rests on the claim that `RejectionTrace` is enough to
produce a meaningful explanation. Two questions test that claim:

1. **Can each downstream module work from `RejectionTrace` alone?**
   - Minimizer: yes — it only needs `Result == Illegal` plus the
     ability to re-run the checker on a sub-history.
   - Pattern matcher: yes, for the patterns we plan to support.
     Each pattern's predicate (§ 6) reduces to claims about
     `BlockedOps`, `Blocks[i].Constraint`, and the relationship
     between `ConflictWith` ops and the blocked op in real time.
   - Renderer: yes — `Detail` carries the human-readable bit.

2. **Can `RejectionTrace` actually be filled in from Porcupine?**
   - `LongestPrefix`, `PartialLinearizations`, `BlockedOps`: yes, from
     `LinearizationInfo.PartialLinearizationsOperations()`.
   - `BlockReason.Constraint` and `ConflictWith`: **partially**. The
     public Porcupine API doesn't expose the model-step failure that
     killed extension of a particular prefix. We have two options:

     **(a) Public API only.** For each `BlockedOp`, take the longest
     prefix that didn't include it; replay it through the model;
     attempt the step; classify the failure. This is a reconstruction,
     not the checker's actual reason, but for the patterns we care
     about it is sufficient and keeps Porcupine unmodified.

     **(b) Fork Porcupine.** Instrument the search to record, per
     blocked op, the actual prefix and constraint at the moment of
     failure. More accurate, more code.

     **Decision: start with (a). Fork only if (a) cannot distinguish
     two patterns we need to distinguish.** We will know by Day 4.
     The decision is recorded here and revisited in DESIGN.md if it
     changes.

**Rejected alternative:** expose Porcupine's raw `LinearizationInfo`
through `pkg/checker`. Rejected because it leaks the dependency — every
downstream module would import porcupine, and a fork or replacement
would ripple through. `RejectionTrace` is the firewall.

---

## 5. The Explanation data structure

This is the user-facing output. JSON-serializable, also pretty-printable.

```go
// pkg/explainer.Explanation
type Explanation struct {
    Verdict       Verdict       // "linearizable" | "non-linearizable" | "unknown"
    Pattern       string        // "" if no pattern matched; e.g. "stale_read"
    Summary       string        // one sentence, suitable for a CI log
    Witness       *history.History // minimized sub-history that still fails
    Conflicts     []Conflict
    Suggestions   []string      // optional remediation hints, pattern-specific
}

type Conflict struct {
    OpIDs []int                 // ops in the smoking gun
    Why   string                // one-paragraph natural language
}
```

**Why a flat struct rather than a tree of "evidence" nodes.** A tree is
tempting (mirrors the structure of a verification proof). Rejected
because the *audience* is an engineer reading a CI log, not a verifier
consuming a proof. Flat reads better. If we later want a richer
representation for tooling, we can add a `Proof` field; we do not need
it on Day 1.

---

## 6. Pattern catalogue

Five patterns, derived from a survey of real linearizability incidents
(this list will be backed by citations to specific Jepsen analyses by
Day 6 — see § 8 methodology).

### 6.1 Stale read

**Informal:** a read returned a value older than a write that already
completed in real time.

**Predicate over `RejectionTrace`:**
- there exists a blocked read `r` with `Constraint == "value-mismatch"`,
- there exists a write `w` with `w.Return < r.Call` and `w.Value` is
  newer than `r.Value` (newer = appears later in the longest prefix).

**Witness shape after minimization:** {w, r}, two ops.

### 6.2 Lost update

**Informal:** two writes, both acknowledged, but a later read sees
neither (or sees only the earlier one), implying one write was lost.

**Predicate:**
- two writes `w1`, `w2` with non-overlapping return points,
- a read `r` after both, with `r.Value != w2.Value && r.Value != w1.Value`,
  or `r.Value == w1.Value` despite `w2` having returned before `r.Call`.

**Witness shape:** {w1, w2, r}.

### 6.3 Real-time inversion

**Informal:** op A finished before op B started, but the only
linearizations the checker can extend require B before A.

**Predicate:**
- two ops `a`, `b` with `a.Return < b.Call`,
- in every partial linearization that includes both, `b` precedes `a`,
  and in every prefix that includes `a` first, no extension legally
  reaches `b`.

**Witness shape:** {a, b} plus any reads needed to expose the inversion.

### 6.4 Non-monotonic per-client read

**Informal:** within a single client, reads return values that go
backwards in the underlying state's evolution.

**Predicate:**
- for a single `Client`, two reads `r1`, `r2` with `r1.Return < r2.Call`,
- `r2.Value` corresponds to a state that, in every partial linearization,
  precedes the state corresponding to `r1.Value`.

**Witness shape:** {r1, r2} plus the writes establishing the ordering.

### 6.5 Phantom value

**Informal:** a read returned a value that no write ever produced.

**Predicate:**
- a blocked read `r` with `r.Value` not in `{w.Value : w is a write in History}`,
- `r.Value != Init`.

**Witness shape:** {r}, single op (the writes are absent by definition).

### Pattern coverage discipline

A history may match multiple patterns. The matcher returns all matches,
ordered by specificity (most specific first). The Explanation surfaces
the most specific match in `Pattern`; less specific matches are listed
in `Suggestions`. "Most specific" = smallest witness.

**Rejected alternative:** mutually-exclusive patterns. Rejected because
real incidents often manifest as several patterns simultaneously
(e.g., a stale read *is* a real-time inversion); forcing a choice
loses information.

---

## 7. Minimization

`pkg/minimize.DDMin` implements one-minimal delta debugging on the
history. Predicate: "checker still returns Illegal." Output: a sub-history
that is still non-linearizable but no proper subset is.

### Notes

- Removing operations from a non-linearizable history can make it
  linearizable; that is the whole point of the search. There is no
  monotonicity violation to worry about.
- The minimizer is parametrized over the predicate so we can also
  use it to find minimal *pattern* witnesses ("smallest sub-history
  matching `stale_read`") on Day 6.
- Performance: histories ≤ 50 ops are the hard cases of interest.
  Naive ddmin is O(n²) checker calls in the worst case. Acceptable.

---

## 8. Methodology: deriving the pattern catalogue from real incidents

The five patterns above are the prior. To make the catalogue defensible
for a paper, by Day 6 we will:

1. Read 8–10 Jepsen reports (etcd, Cassandra, MongoDB, FaunaDB, Redis,
   CockroachDB are good candidates — each has multiple reports).
2. For each report's linearizability violations, classify under the
   five patterns.
3. Record three things per incident: (a) which pattern fits,
   (b) whether the explainer would have classified it correctly given
   the history, (c) what an engineer would have wanted to see.
4. If ≥ 2 incidents do not fit any of the five, add a pattern. If a
   pattern catches no incidents, drop it.

The output is `eval/incidents.md`: a table mapping incident → pattern
→ predicted explanation. This *is* the evaluation, and it is the heart
of the paper's empirical claim.

---

## 9. Test strategy

### Unit, by package

- `pkg/history`: round-trip JSON, validation of bad histories.
- `pkg/model`: each model's Step on hand-crafted state transitions.
- `pkg/checker`: assert that hand-authored histories produce expected
  `RejectionTrace` content. Especially: `BlockedOps` correctness.
- `pkg/minimize`: synthetic histories with a known minimal witness.
- `pkg/pattern`: each pattern matcher gets a positive test (history
  that matches) and a negative test (history that doesn't).

### Golden-file end-to-end

`pkg/explainer/testdata/` contains pairs:

```
testdata/
  stale_read_simple/
    history.json
    explanation.golden.json
```

The test loads the history, runs the explainer, and asserts that the
output equals the golden file modulo a normalization step (sorted op
IDs, stable map keys). A `-update` flag rewrites goldens.

### What test-first means here

The user asked for test-first specifically on the explanation logic.
For each pattern, the workflow is:

1. Author the synthetic history.
2. Author the expected `Explanation` JSON (golden).
3. Run the test. It fails (no matcher yet).
4. Implement the matcher.
5. Run the test. It passes.

This lands `testdata/` with a clear specification of what each pattern
*means* in concrete operational terms before any matcher code exists.

---

## 10. Day-by-day, revised

- **Day 1 (today):** Repo + baseline + DESIGN.md. ✅ in progress.
- **Day 2:** Read Lowe's paper end-to-end. Implement
  `pkg/checker.RejectionTrace` extraction from Porcupine's public API
  (option (a) from § 4). Decide whether to fork Porcupine; record the
  decision here.
- **Day 3:** Author golden histories + golden explanations for all
  five patterns. Tests fail; this is intentional.
- **Day 4:** Implement matchers for stale read, lost update, real-time
  inversion. First three tests should pass.
- **Day 5:** Implement `pkg/minimize`. Confirm minimization shrinks
  authored histories to their declared witness shapes.
- **Day 6:** Survey Jepsen reports → `eval/incidents.md`. Adjust pattern
  catalogue. Implement remaining matchers (non-monotonic, phantom).
- **Day 7:** End-to-end run on a real Jepsen-style history (toy KV
  store; write a small driver if no canned one is available).
  Update `eval/results.md` with the table promised in § 8.

---

## 11. Open questions (revisit by end of Day 2)

1. Public API vs Porcupine fork (§ 4). Will know after writing the
   `RejectionTrace` extractor.
2. KV partitioning. Porcupine's per-key partition makes per-key
   linearizability cheap, but cross-key explanations are out of scope
   (we're not strict-serializable). Confirm this is fine for the toy
   KV store on Day 7.
3. Whether `BlockReason.State` should be `any` (current) or
   `string` (rendered via `DescribeState`). `any` is more flexible,
   `string` is JSON-friendly. Probably switch to a struct with both.

---

## 12. Day 2 findings

**Decision: stay on the public Porcupine API.** The
`PartialLinearizationsOperations()` accessor gives us enough to
populate `RejectionTrace` for register and counter histories. No fork
needed. This closes question § 11.1.

**`BlockReason.State` is now `string` (StateDesc).** The state is only
ever consumed for rendering and pattern matching against descriptions
of state, so the loss of the typed state value is not material.
Closes § 11.3.

**Surprising finding: `ConstraintRealTimePrecedes` is essentially
unreachable through Porcupine's public API.** I designed three
constraint labels (value-mismatch, real-time-precedes,
concurrent-conflict). The real-time-precedes branch fires only if the
"best prefix" contains an op `A` such that `A.Call > blocked.Return`.
Porcupine's search walks the entry list in real-time order; whenever a
blocked op's call comes first and its `Step` fails, the algorithm
backtracks past its return and dies — it does not skip ops to reach a
later prefix that would create the inversion condition. So the longest
prefix Porcupine reports is always made of ops whose calls happened
before the blocked op's return.

**Consequence: real-time-inversion patterns must be detected at the
pattern-matcher layer, not at the constraint-classifier layer.** The
matcher will reason about pairs of ops in the history directly:
"there exist `a, b` with `a.Return < b.Call` such that the only
linearizations consistent with the values placed `b` before `a`."
The constraint label is downgraded to a low-level signal of *what kind
of step failure killed the search at this prefix*; it is not a
description of the violation pattern.

`ConstraintRealTimePrecedes` is kept as a defensive branch (it would
fire on a non-public-API trace, e.g., if we ever did fork Porcupine).
But no test currently exercises it, and that is recorded as expected.

**Three constraint labels, with their actual roles:**

- `value-mismatch`: the model's `Step` rejected the blocked op
  given the state produced by the longest prefix. The prefix may be
  empty; if so, no establishing write is named in `ConflictWith`. This
  is the primary signal for stale read, lost update, and phantom
  value patterns.
- `concurrent-conflict`: the model would *accept* the blocked op
  individually after the prefix, but the search couldn't extend
  further. In practice, this fires for ops downstream of an earlier
  blocking op — the search backtracked past the bad op and never
  reached them.
- `real-time-precedes`: defensive, see above.

**Test coverage on this finding:**
- `TestCheck_PhantomValue_NoPartials` — empty prefix case.
- `TestCheck_DownstreamConcurrentConflict` — three-op case where the
  third op classifies as concurrent-conflict because the bad middle
  op stops the search.
