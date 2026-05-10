# Jepsen Incident Survey

A small audit of real linearizability violations from Jepsen-style
analyses, classified against whynot's pattern catalogue. The aim is
NOT a comprehensive review of distributed-system bugs; the aim is to
verify that the five patterns cover what real reports actually
document, and to surface anything that falls outside the catalogue.

This survey is the empirical claim that turns the catalogue from
"patterns I happened to think of" into "patterns derived from real
incidents." See DESIGN.md § 8.

## Methodology

For each report we record: the system, a one-line description of the
bug at the protocol level, the anomaly type the report names (or our
inference if it does not), and the pattern in our catalogue that best
fits — or `(none)` / `(out of scope)` when the violation is
structurally outside single-key linearizability.

Sources: Jepsen analyses on jepsen.io and Aphyr's call-me-maybe series.
Incidents are paraphrased; cite the original report for precise wording.

## Incidents

| # | System / Source | Anomaly summary | Pattern | Notes |
|---|---|---|---|---|
| 1 | MongoDB 4.2.6 (jepsen.io) | Acknowledged writes disappear after partition heal; later reads see only earlier writes. | `lost_update` | Multiple acked writes vanish across the partition. |
| 2 | MongoDB 4.2.6 | Single append applied twice (transaction retry double-application). | (none) | Inverse of lost update; no current pattern fits. See Findings. |
| 3 | MongoDB 4.2.6 | Acked write reported as `TransactionCoordinatorSteppingDown`, yet visible. | (out of scope) | Reporting/protocol issue, not a value anomaly. |
| 4 | etcd 3.4.3 (jepsen.io) | Watch from `rev=0` misses historical updates; observers skip ack’d writes to the watched key. | `stale_read` (weak) | Best-fit. Could also be argued as `phantom_value` if pre-history is treated as the entire missing prefix. |
| 5 | Redis-Raft 1b3fbf6 (jepsen.io) | Post-election leaders skip the no-op log entry; T₂'s read 3.25s after T₁ saw older state. | `stale_read` | Textbook fit. |
| 6 | Redis-Raft 1b3fbf6 | Fresh node startup returns `[]` for keys that other clients see as `[…86 87 89]`. | `non_monotonic_read` | Same client sees state move backwards relative to its own session. |
| 7 | MongoDB 2014 (Aphyr) | `MAJORITY` write acked then rolled back when old primary rejoins. ~2 of 6000 acked writes lost. | `lost_update` | The canonical Jepsen-MongoDB violation. |
| 8 | MongoDB 2014 | Rollback discards unreplicated acked writes. | `lost_update` | Special case of #7. |
| 9 | YugabyteDB 1.3.1 (jepsen.io) | Counter read returns a value lower than the sum of acked increments under clock skew exceeding `--max_clock_skew_usec`. | `stale_read` | Counter model — fits squarely. |
| 10 | MongoDB Galera (Aphyr) | Minority primary serves stale local reads during partition. | `stale_read` | Read returns older value than majority-primary write. |
| 11 | MongoDB Galera | Uncommitted intermediate states observed before rollback. | (out of scope) | Transactional dirty read, not single-key linearizability. |

## Pattern coverage

| Pattern             | Incidents fitting | Real-world signal |
|---------------------|-------------------|-------------------|
| `stale_read`        | 4 (4, 5, 9, 10)   | Strong — the most common observable shape |
| `lost_update`       | 3 (1, 7, 8)       | Strong — recurrent under partition healing |
| `non_monotonic_read`| 1 (6)             | Present — typically post-restart/failover |
| `phantom_value`     | 0                 | Weak in this sample; closest is #4 |
| `realtime_inversion`| 0                 | Absent in this sample |

In-scope: 9 incidents. Out of scope or unclassifiable: 2 incidents
(#3 reporting issue, #11 transactional). Catalogue covers 8 of 9
in-scope incidents (89%).

## Findings

1. **`stale_read` and `lost_update` are load-bearing.** Seven of nine
   in-scope incidents fit one of these two patterns. The catalogue's
   value depends primarily on these two matchers being correct. The
   matcher implementations should be evaluated most strictly here.

2. **`realtime_inversion` has no real-world match in this sample.**
   The pattern is theoretically valid but represents an apparent
   time-travel from a future write — a class of bug that may simply
   not arise in well-behaved Raft/Paxos implementations. Two
   interpretations:
     - Drop it as a theoretical curiosity.
     - Keep it as a coverage check; if it ever fires on a real
       incident, that is itself a noteworthy finding.

   **Decision (recorded here):** keep `realtime_inversion` in the
   catalogue, document its empirical absence, and flag any future
   match against it as a paper-worthy observation. The matcher is
   cheap to maintain and synthetic tests cover the predicate.

3. **`phantom_value` is the weakest empirical match.** etcd's watch
   bug is the closest fit but is also describable as a degenerate
   `stale_read` (read returns the empty pre-history). Keep
   `phantom_value` because it fires unambiguously on contrived
   inputs (e.g., a read returning a value never written) and serves
   as a sanity-check for the matcher pipeline; do not present it as
   the headline pattern.

4. **A `duplicate_write` pattern is missing.** Incident #2 is real
   and recurrent in failover-with-retry systems. Our register and
   counter models cannot express it without tagging writes or moving
   to a multiset/list semantics. Out of scope for this week; logged
   as future work.

5. **Dirty reads are correctly out of scope.** They are a
   transactional isolation concern and fall outside single-key
   linearizability by definition. Confirms § 1's stated non-goal.

## Catalogue revisions for this week

None. The five-pattern catalogue covers 89% of in-scope incidents in
this sample. The two notable gaps (`duplicate_write` and the absence
of `realtime_inversion` evidence) are recorded for future work and
do not require changes to the matcher implementations.
