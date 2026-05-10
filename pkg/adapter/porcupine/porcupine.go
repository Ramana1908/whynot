// Package porcupine adapts existing porcupine.Operation slices into
// whynot's history.History so projects already using Porcupine can plug
// whynot in without re-instrumenting their tests.
//
// Two paths:
//
//  1. If your code uses whynot's own model.Register / model.KV, pass
//     RegisterTranslator or KVTranslator to FromOperations and you're done.
//
//  2. If you use a custom Porcupine Model, supply your own Translator
//     mapping (Input, Output) → (read|write, value, key).
package porcupine

import (
	"fmt"
	"sort"

	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/model"
	pcp "github.com/anishathalye/porcupine"
)

// Translator turns the model-specific (Input, Output) pair on a
// porcupine.Operation into the high-level fields of a history.Op:
// the kind (read/write), the value, and (for KV) the key.
//
// Adapter callers supply a Translator only if their Porcupine model
// is not one of whynot's built-ins.
type Translator func(pcp.Operation) (kind history.OpKind, value int, key string, err error)

// FromOperations converts a Porcupine operation slice into a whynot
// History using the given Translator. modelName must be one of
// "register", "counter", "kv". init is the model's initial value
// (ignored for kv). Output is sorted by Call time.
//
// Op IDs come from porcupine.Operation.Metadata when it is an int;
// otherwise they default to the slice index. Duplicate IDs (whether
// from Metadata or from a degenerate translator) return an error.
func FromOperations(ops []pcp.Operation, modelName string, init int, t Translator) (*history.History, error) {
	if t == nil {
		return nil, fmt.Errorf("porcupine adapter: translator must not be nil")
	}
	out := make([]history.Op, len(ops))
	seen := map[int]bool{}
	for i, op := range ops {
		kind, val, key, err := t(op)
		if err != nil {
			return nil, fmt.Errorf("op %d: %w", i, err)
		}
		id := i
		if md, ok := op.Metadata.(int); ok {
			id = md
		}
		if seen[id] {
			return nil, fmt.Errorf("op %d: duplicate id %d — set unique Operation.Metadata or omit it to fall back to slice index", i, id)
		}
		seen[id] = true
		out[i] = history.Op{
			ID:     id,
			Client: op.ClientId,
			Type:   kind,
			Key:    key,
			Value:  val,
			Call:   op.Call,
			Return: op.Return,
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Call < out[j].Call })
	return &history.History{Model: modelName, Init: init, Ops: out}, nil
}

// RegisterTranslator translates operations produced by whynot's
// model.Register. Use this when you ran porcupine.CheckOperations on
// model.Register(init) and want the history back.
func RegisterTranslator(op pcp.Operation) (history.OpKind, int, string, error) {
	in, ok := op.Input.(model.RegisterInput)
	if !ok {
		return "", 0, "", fmt.Errorf("expected model.RegisterInput, got %T", op.Input)
	}
	if in.Op { // read
		v, ok := op.Output.(int)
		if !ok {
			return "", 0, "", fmt.Errorf("read output not int: %T", op.Output)
		}
		return history.OpRead, v, "", nil
	}
	return history.OpWrite, in.Value, "", nil
}

// KVTranslator translates operations produced by whynot's model.KV.
func KVTranslator(op pcp.Operation) (history.OpKind, int, string, error) {
	in, ok := op.Input.(model.KVInput)
	if !ok {
		return "", 0, "", fmt.Errorf("expected model.KVInput, got %T", op.Input)
	}
	if in.Op { // read
		v, ok := op.Output.(int)
		if !ok {
			return "", 0, "", fmt.Errorf("read output not int: %T", op.Output)
		}
		return history.OpRead, v, in.Key, nil
	}
	return history.OpWrite, in.Value, in.Key, nil
}
