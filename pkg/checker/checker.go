// Package checker wraps Porcupine and produces a RejectionTrace — a
// structured account of why a history was rejected, suitable for downstream
// pattern matching and explanation.
//
// The RejectionTrace is the firewall between Porcupine and the rest of
// whynot: only this package imports porcupine, so a future fork or
// replacement does not ripple into pkg/explainer or pkg/pattern.
package checker

import (
	"fmt"
	"sort"
	"time"

	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/anishathalye/porcupine"
)

type Result string

const (
	Ok      Result = "Ok"
	Illegal Result = "Illegal"
	Unknown Result = "Unknown"
)

type Constraint string

const (
	// ConstraintValueMismatch: prefix is real-time-consistent with the
	// blocked op, but the model's Step function rejects the op given the
	// state produced by the prefix.
	ConstraintValueMismatch Constraint = "value-mismatch"

	// ConstraintRealTimePrecedes: the prefix contains operations that
	// completed (in real time) only after the blocked op started, meaning
	// the blocked op should have come before them in any valid
	// linearization.
	ConstraintRealTimePrecedes Constraint = "real-time-precedes"

	// ConstraintConcurrentConflict: the blocked op fits this prefix in
	// isolation but no extension reaches the rest of the history. Catch-all
	// for cases the simpler classifications miss.
	ConstraintConcurrentConflict Constraint = "concurrent-conflict"
)

// RejectionTrace is the contract between checker and explainer. See
// DESIGN.md § 4.
type RejectionTrace struct {
	History    *history.History
	Result     Result
	Partitions []PartitionTrace
}

// PartitionTrace holds per-partition rejection information. For
// non-partitioned models (register, counter), there is exactly one
// partition with empty PartitionKey.
type PartitionTrace struct {
	PartitionKey string

	// LongestPrefix is the longest partial linearization the search found
	// in this partition (op IDs).
	LongestPrefix []int

	// PartialLinearizations: each is a sequence of op IDs that the
	// search confirmed legal up to some point. Drawn from
	// porcupine.LinearizationInfo.PartialLinearizationsOperations.
	PartialLinearizations [][]int

	// BlockedOps are operations in this partition that did not appear in
	// any partial linearization — i.e., the search could not legally
	// incorporate them anywhere.
	BlockedOps []int

	// Blocks is one entry per BlockedOp explaining why.
	Blocks []BlockReason
}

// BlockReason captures, for a single blocked op, the longest legal prefix
// the checker reached and the constraint that prevented the extension.
type BlockReason struct {
	OpID         int
	PrefixIDs    []int
	StateDesc    string     // model state after replaying PrefixIDs (rendered)
	Constraint   Constraint
	ConflictWith []int      // prefix op IDs most directly responsible
	Detail       string     // human-readable, e.g. "read returned 0; state was 1"
}

// Check runs Porcupine on (model, ops) and returns a RejectionTrace.
// Each op's Metadata field MUST hold its history.Op.ID (int) — pkg/model
// guarantees this for its translators.
func Check(model porcupine.Model, ops []porcupine.Operation, h *history.History, timeout time.Duration) *RejectionTrace {
	res, info := porcupine.CheckOperationsVerbose(model, ops, timeout)
	rt := &RejectionTrace{History: h}

	switch res {
	case porcupine.Ok:
		rt.Result = Ok
		return rt
	case porcupine.Unknown:
		rt.Result = Unknown
		return rt
	case porcupine.Illegal:
		rt.Result = Illegal
	}

	histByID := indexHistOps(h)
	pOpByID := indexPorcupineOps(ops)

	partitionedOps := partitionOps(model, ops)
	partials := info.PartialLinearizationsOperations()

	// Defensive: porcupine returns one slice per partition; partitionedOps
	// must match. If not, fall back to single partition.
	if len(partials) != len(partitionedOps) {
		partitionedOps = [][]porcupine.Operation{ops}
	}

	for i := range partials {
		pt := buildPartitionTrace(model, h, histByID, pOpByID, partitionedOps[i], partials[i])
		rt.Partitions = append(rt.Partitions, pt)
	}

	return rt
}

func buildPartitionTrace(
	model porcupine.Model,
	h *history.History,
	histByID map[int]history.Op,
	pOpByID map[int]porcupine.Operation,
	partOps []porcupine.Operation,
	partials [][]porcupine.Operation,
) PartitionTrace {
	pt := PartitionTrace{}

	// Translate partials to op-ID slices and accumulate seen IDs.
	seen := map[int]bool{}
	pt.PartialLinearizations = make([][]int, 0, len(partials))
	for _, lin := range partials {
		ids := make([]int, len(lin))
		for j, op := range lin {
			id := op.Metadata.(int)
			ids[j] = id
			seen[id] = true
		}
		pt.PartialLinearizations = append(pt.PartialLinearizations, ids)
		if len(ids) > len(pt.LongestPrefix) {
			pt.LongestPrefix = ids
		}
	}

	// BlockedOps = ops in this partition not in any partial.
	for _, pop := range partOps {
		id := pop.Metadata.(int)
		if !seen[id] {
			pt.BlockedOps = append(pt.BlockedOps, id)
		}
	}
	sort.Ints(pt.BlockedOps)

	for _, blockedID := range pt.BlockedOps {
		pt.Blocks = append(pt.Blocks, classifyBlock(model, histByID, pOpByID, blockedID, pt.PartialLinearizations))
	}
	return pt
}

func classifyBlock(
	model porcupine.Model,
	histByID map[int]history.Op,
	pOpByID map[int]porcupine.Operation,
	blockedID int,
	partials [][]int,
) BlockReason {
	blockedHist := histByID[blockedID]
	blockedPOp := pOpByID[blockedID]

	// Choose the longest partial as the "best" prefix we'd be extending
	// with the blocked op. (Heuristic; a more careful choice could pick
	// the partial that maximizes pre-blocked real-time consistency.)
	var bestPrefix []int
	for _, p := range partials {
		if len(p) > len(bestPrefix) {
			bestPrefix = p
		}
	}

	// 1) Real-time-precedes check: any prefix op that started AFTER the
	// blocked op finished must come after the blocked op in any
	// linearization. If the prefix contains such an op, the blocked op
	// could not be added because it would have to be inserted earlier
	// than the prefix already extends.
	var inversions []int
	for _, id := range bestPrefix {
		o := histByID[id]
		if o.Call > blockedHist.Return {
			inversions = append(inversions, id)
		}
	}
	if len(inversions) > 0 {
		return BlockReason{
			OpID:         blockedID,
			PrefixIDs:    bestPrefix,
			Constraint:   ConstraintRealTimePrecedes,
			ConflictWith: inversions,
			Detail: fmt.Sprintf(
				"op %d (returned at t=%d) must precede op(s) %v in any linearization, but the longest legal prefix already includes them",
				blockedID, blockedHist.Return, inversions,
			),
		}
	}

	// 2) Replay the prefix to compute the model state at the moment of
	// rejection, then attempt to extend with the blocked op.
	state := model.Init()
	for _, id := range bestPrefix {
		op := pOpByID[id]
		ok, ns := model.Step(state, op.Input, op.Output)
		if !ok {
			// Should not happen — porcupine said this prefix was legal.
			return BlockReason{
				OpID:       blockedID,
				PrefixIDs:  bestPrefix,
				Constraint: ConstraintConcurrentConflict,
				Detail:     fmt.Sprintf("internal: replay of partial linearization rejected at op %d", id),
			}
		}
		state = ns
	}

	stateDesc := describeState(model, state)

	ok, _ := model.Step(state, blockedPOp.Input, blockedPOp.Output)
	if !ok {
		opDesc := describeOp(model, blockedPOp)
		return BlockReason{
			OpID:         blockedID,
			PrefixIDs:    bestPrefix,
			StateDesc:    stateDesc,
			Constraint:   ConstraintValueMismatch,
			ConflictWith: lastWriteIn(bestPrefix, histByID),
			Detail: fmt.Sprintf(
				"%s rejected by model in state %s (after replaying prefix %v)",
				opDesc, stateDesc, bestPrefix,
			),
		}
	}

	return BlockReason{
		OpID:       blockedID,
		PrefixIDs:  bestPrefix,
		StateDesc:  stateDesc,
		Constraint: ConstraintConcurrentConflict,
		Detail: fmt.Sprintf(
			"op fits prefix %v in state %s individually, but no extension reaches the rest of the history",
			bestPrefix, stateDesc,
		),
	}
}

func indexHistOps(h *history.History) map[int]history.Op {
	m := make(map[int]history.Op, len(h.Ops))
	for _, op := range h.Ops {
		m[op.ID] = op
	}
	return m
}

func indexPorcupineOps(ops []porcupine.Operation) map[int]porcupine.Operation {
	m := make(map[int]porcupine.Operation, len(ops))
	for _, op := range ops {
		m[op.Metadata.(int)] = op
	}
	return m
}

func partitionOps(model porcupine.Model, ops []porcupine.Operation) [][]porcupine.Operation {
	if model.Partition == nil {
		return [][]porcupine.Operation{ops}
	}
	return model.Partition(ops)
}

func describeState(model porcupine.Model, state interface{}) string {
	if model.DescribeState != nil {
		return model.DescribeState(state)
	}
	return fmt.Sprintf("%v", state)
}

func describeOp(model porcupine.Model, op porcupine.Operation) string {
	if model.DescribeOperation != nil {
		return model.DescribeOperation(op.Input, op.Output)
	}
	return fmt.Sprintf("%v -> %v", op.Input, op.Output)
}

// lastWriteIn picks the most recent write in the prefix (by position),
// which for a register is the op that established the current state.
// Returns nil if the prefix has no writes.
func lastWriteIn(prefix []int, histByID map[int]history.Op) []int {
	for i := len(prefix) - 1; i >= 0; i-- {
		op := histByID[prefix[i]]
		if op.Type == history.OpWrite {
			return []int{op.ID}
		}
	}
	return nil
}
