// Package history defines whynot's canonical history representation and
// JSON I/O. Histories produced here are translated to porcupine.Operation
// by package model.
package history

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type OpKind string

const (
	OpRead  OpKind = "read"
	OpWrite OpKind = "write"
	OpCAS   OpKind = "cas"
)

// Op is one client-visible operation. Call and Return are integer logical
// timestamps; the interval [Call, Return] is closed (matches porcupine).
type Op struct {
	ID       int    `json:"id"`
	Client   int    `json:"client"`
	Type     OpKind `json:"type"`
	Key      string `json:"key,omitempty"`   // empty for single-register histories
	Value    int    `json:"value"`           // for read: returned value; for write: written value
	Expected int    `json:"expected,omitempty"` // for cas
	Call     int64  `json:"call"`
	Return   int64  `json:"return"`
}

type History struct {
	Model string `json:"model"` // "register" | "counter" | "kv"
	Init  int    `json:"init"`
	Ops   []Op   `json:"ops"`
}

func Load(path string) (*History, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

func Read(r io.Reader) (*History, error) {
	var h History
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&h); err != nil {
		return nil, fmt.Errorf("decode history: %w", err)
	}
	if err := h.validate(); err != nil {
		return nil, err
	}
	return &h, nil
}

func (h *History) validate() error {
	if h.Model == "" {
		return fmt.Errorf("history: missing model")
	}
	seen := map[int]bool{}
	for i, op := range h.Ops {
		if op.Call > op.Return {
			return fmt.Errorf("op %d: call %d > return %d", i, op.Call, op.Return)
		}
		if seen[op.ID] {
			return fmt.Errorf("op %d: duplicate id %d", i, op.ID)
		}
		seen[op.ID] = true
	}
	return nil
}
