package model

import (
	"fmt"

	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/anishathalye/porcupine"
)

// CounterInput specifies the operation: read, or increment-by-delta.
// Delta is ignored for read.
type CounterInput struct {
	Op    bool // false = increment, true = read
	Delta int
}

// Counter returns a porcupine.Model for an integer counter that
// supports increment(delta) and read(). Increment is always legal;
// read is legal iff the returned value matches the current count.
func Counter(init int) porcupine.Model {
	return porcupine.Model{
		Init: func() interface{} { return init },
		Step: func(state, input, output interface{}) (bool, interface{}) {
			in := input.(CounterInput)
			s := state.(int)
			if !in.Op {
				return true, s + in.Delta
			}
			return output == s, s
		},
		DescribeOperation: func(input, output interface{}) string {
			in := input.(CounterInput)
			if in.Op {
				return fmt.Sprintf("read() -> %d", output.(int))
			}
			return fmt.Sprintf("incr(%d)", in.Delta)
		},
	}
}

// CounterOps translates a counter history into porcupine operations.
// Writes are interpreted as increments (Value = delta).
func CounterOps(h *history.History) []porcupine.Operation {
	out := make([]porcupine.Operation, len(h.Ops))
	for i, op := range h.Ops {
		var input CounterInput
		var output int
		switch op.Type {
		case history.OpWrite:
			input = CounterInput{Op: false, Delta: op.Value}
			output = 0
		case history.OpRead:
			input = CounterInput{Op: true}
			output = op.Value
		default:
			input = CounterInput{Op: true}
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
