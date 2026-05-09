// Package model contains data-type specifications (register, counter, KV)
// used by the linearizability checker. Each model exposes a porcupine.Model
// plus a translator from history.Op to porcupine.Operation.
package model

import (
	"fmt"

	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/anishathalye/porcupine"
)

// RegisterInput is the input value passed to porcupine for register ops.
// op=false → write, op=true → read. Mirrors Porcupine's example register.
type RegisterInput struct {
	Op    bool
	Value int
}

// Register returns a porcupine.Model for a single integer register.
// Initial value is supplied by the caller (matches History.Init).
func Register(init int) porcupine.Model {
	return porcupine.Model{
		Init: func() interface{} { return init },
		Step: func(state, input, output interface{}) (bool, interface{}) {
			in := input.(RegisterInput)
			if !in.Op { // write
				return true, in.Value
			}
			// read: legal iff returned value matches current state
			return output == state, state
		},
		DescribeOperation: func(input, output interface{}) string {
			in := input.(RegisterInput)
			if in.Op {
				return fmt.Sprintf("read() -> %d", output.(int))
			}
			return fmt.Sprintf("write(%d)", in.Value)
		},
	}
}

// RegisterOps translates a history.History into porcupine.Operation slice
// for the register model. Ops keep their original ID via ClientId... no,
// keep ID as Operation.Metadata so we can map back from porcupine results.
func RegisterOps(h *history.History) []porcupine.Operation {
	out := make([]porcupine.Operation, len(h.Ops))
	for i, op := range h.Ops {
		var input RegisterInput
		var output int
		switch op.Type {
		case history.OpWrite:
			input = RegisterInput{Op: false, Value: op.Value}
			output = 0 // unused for writes; register model ignores write outputs
		case history.OpRead:
			input = RegisterInput{Op: true}
			output = op.Value
		default:
			// CAS and others not supported by the plain register model.
			// Caller should pick a different model.
			input = RegisterInput{Op: true}
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
