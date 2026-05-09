package model

import (
	"fmt"

	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/anishathalye/porcupine"
)

// KVInput is the input to a per-key register operation. Op false=write,
// true=read. Key is preserved as Op metadata so that the partition
// function can group operations by key.
type KVInput struct {
	Op    bool
	Key   string
	Value int
}

// KV returns a porcupine.Model for a key-value store of integer values,
// where each key is treated as an independent register (per-key
// linearizability). The model partitions the history by key, so each
// partition can be checked separately.
func KV() porcupine.Model {
	return porcupine.Model{
		Partition: func(ops []porcupine.Operation) [][]porcupine.Operation {
			byKey := map[string][]porcupine.Operation{}
			order := []string{}
			for _, op := range ops {
				k := op.Input.(KVInput).Key
				if _, ok := byKey[k]; !ok {
					order = append(order, k)
				}
				byKey[k] = append(byKey[k], op)
			}
			out := make([][]porcupine.Operation, 0, len(order))
			for _, k := range order {
				out = append(out, byKey[k])
			}
			return out
		},
		Init: func() interface{} { return 0 },
		Step: func(state, input, output interface{}) (bool, interface{}) {
			in := input.(KVInput)
			if !in.Op {
				return true, in.Value
			}
			return output == state, state
		},
		DescribeOperation: func(input, output interface{}) string {
			in := input.(KVInput)
			if in.Op {
				return fmt.Sprintf("get(%q) -> %d", in.Key, output.(int))
			}
			return fmt.Sprintf("put(%q, %d)", in.Key, in.Value)
		},
	}
}

// KVOps translates a KV history into porcupine operations.
func KVOps(h *history.History) []porcupine.Operation {
	out := make([]porcupine.Operation, len(h.Ops))
	for i, op := range h.Ops {
		var input KVInput
		var output int
		switch op.Type {
		case history.OpWrite:
			input = KVInput{Op: false, Key: op.Key, Value: op.Value}
			output = 0
		case history.OpRead:
			input = KVInput{Op: true, Key: op.Key}
			output = op.Value
		default:
			input = KVInput{Op: true, Key: op.Key}
			output = op.Value
		}
		out[i] = porcupine.Operation{
			ClientId: op.Client,
			Input:    input,
			Output:   output,
			Call:     op.Call,
			Return:   op.Return,
			Metadata: op.ID,
		}
	}
	return out
}

// SelectModel returns the porcupine.Model + ops translation for a
// history, dispatching on h.Model. This is the canonical entry point
// callers should use.
func SelectModel(h *history.History) (porcupine.Model, []porcupine.Operation, error) {
	switch h.Model {
	case "register":
		return Register(h.Init), RegisterOps(h), nil
	case "counter":
		return Counter(h.Init), CounterOps(h), nil
	case "kv":
		return KV(), KVOps(h), nil
	default:
		return porcupine.Model{}, nil, fmt.Errorf("unknown model %q", h.Model)
	}
}
