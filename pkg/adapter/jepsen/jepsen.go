package jepsen

import (
	"fmt"
	"io"
	"os"

	"github.com/Ramana1908/whynot/pkg/history"
)

// LoadEDN reads a Jepsen EDN history file from path and converts it.
// modelName is the whynot model identifier ("register" | "counter").
// KV histories are not yet supported (Jepsen's KV/append workloads
// encode keys non-uniformly across workloads; future work).
func LoadEDN(path, modelName string, init int) (*history.History, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadEDN(f, modelName, init)
}

// ReadEDN reads a Jepsen EDN history from r and converts it.
//
// Conversion rules:
//   - The log is a sequence of maps with at least :type, :process, :f.
//     Common shapes: a top-level vector of maps, or line-delimited maps.
//   - For each :process, an :invoke is paired with the next :ok on the
//     same process. The pair becomes one whynot Op.
//   - :fail and :info ops are skipped (no return; cannot be on the
//     linearization timeline). The skipped count is reported via the
//     returned History.Ops length being smaller than the input.
//   - :nemesis processes are skipped (fault injection events, not
//     client ops).
//   - :f :read maps to OpRead with Value = :ok :value (the returned
//     value). :f :write maps to OpWrite with Value = :invoke :value
//     (the written value).
//   - :time on :invoke becomes Call; :time on :ok becomes Return.
func ReadEDN(r io.Reader, modelName string, init int) (*history.History, error) {
	values, err := parseEDN(r)
	if err != nil {
		return nil, err
	}

	pending := map[int64]ednMap{} // process → in-flight invoke
	var ops []history.Op
	nextID := 0

	for i, v := range values {
		m, ok := v.(ednMap)
		if !ok {
			return nil, fmt.Errorf("entry %d: expected map, got %T", i, v)
		}
		typ, _ := m["type"].(ednKeyword)
		proc, ok := m["process"].(int64)
		if !ok {
			// Skip entries with non-integer process (some Jepsen
			// workloads use :nemesis as the process value).
			continue
		}
		switch typ {
		case "invoke":
			pending[proc] = m
		case "ok":
			inv, found := pending[proc]
			if !found {
				return nil, fmt.Errorf("entry %d: :ok for process %d with no preceding :invoke", i, proc)
			}
			delete(pending, proc)
			op, err := buildOp(nextID, int(proc), inv, m)
			if err != nil {
				return nil, fmt.Errorf("entry %d: %w", i, err)
			}
			nextID++
			ops = append(ops, op)
		case "fail", "info":
			// Drop. :fail is "definitely didn't happen", :info is
			// "unknown" — neither belongs on the linearization timeline.
			delete(pending, proc)
		default:
			// Unknown :type. Be conservative and error.
			return nil, fmt.Errorf("entry %d: unknown :type %q", i, typ)
		}
	}

	return &history.History{Model: modelName, Init: init, Ops: ops}, nil
}

func buildOp(id, client int, invoke, ok ednMap) (history.Op, error) {
	f, _ := invoke["f"].(ednKeyword)
	callT, callOK := invoke["time"].(int64)
	retT, retOK := ok["time"].(int64)
	if !callOK || !retOK {
		return history.Op{}, fmt.Errorf("missing :time on :invoke or :ok")
	}

	switch f {
	case "read":
		val, err := intValue(ok["value"])
		if err != nil {
			return history.Op{}, fmt.Errorf("read :value: %w", err)
		}
		return history.Op{
			ID: id, Client: client, Type: history.OpRead,
			Value: val, Call: callT, Return: retT,
		}, nil
	case "write":
		val, err := intValue(invoke["value"])
		if err != nil {
			return history.Op{}, fmt.Errorf("write :value: %w", err)
		}
		return history.Op{
			ID: id, Client: client, Type: history.OpWrite,
			Value: val, Call: callT, Return: retT,
		}, nil
	default:
		return history.Op{}, fmt.Errorf("unsupported :f %q (parser handles :read and :write)", f)
	}
}

func intValue(v ednValue) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("nil value")
	}
	n, ok := v.(int64)
	if !ok {
		return 0, fmt.Errorf("expected int, got %T", v)
	}
	return int(n), nil
}
