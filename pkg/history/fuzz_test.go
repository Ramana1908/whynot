package history

import (
	"bytes"
	"testing"
)

// FuzzRead drives history.Read with arbitrary bytes. The contract is:
// Read either returns (non-nil History, nil error) or (nil-or-non-nil
// History, non-nil error). It must never panic, and on success the
// returned history must satisfy validate(). Run with:
//
//	go test ./pkg/history -run none -fuzz FuzzRead -fuzztime 30s
func FuzzRead(f *testing.F) {
	// Seed with shapes that exercise different code paths: minimal valid,
	// validation-rejected, malformed JSON, and unknown-fields rejection.
	seeds := []string{
		`{"model":"register","init":0,"ops":[]}`,
		`{"model":"register","init":0,"ops":[{"id":0,"client":0,"type":"read","value":0,"call":0,"return":1}]}`,
		`{"model":"kv","init":0,"ops":[{"id":0,"client":0,"type":"write","key":"a","value":1,"call":0,"return":1}]}`,
		`{"model":"register","init":0,"ops":[{"id":0,"client":0,"type":"read","value":0,"call":5,"return":1}]}`, // call > return
		`{"model":"register","init":0,"ops":[{"id":0,"client":0,"type":"read","value":0,"call":0,"return":1},{"id":0,"client":1,"type":"read","value":0,"call":2,"return":3}]}`, // dup id
		`{}`,
		``,
		`{"model":"x","extra":1,"ops":[]}`, // unknown field
		`null`,
		`[]`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		h, err := Read(bytes.NewReader(data))
		if err != nil {
			// Error path: do not assert anything about h. Just must not panic.
			return
		}
		if h == nil {
			t.Fatalf("Read returned nil history with nil error; input=%q", data)
		}
		if h.Model == "" {
			t.Errorf("Read accepted empty model; input=%q", data)
		}
		seen := map[int]bool{}
		for i, op := range h.Ops {
			if op.Call > op.Return {
				t.Errorf("Read accepted op %d with call %d > return %d; input=%q", i, op.Call, op.Return, data)
			}
			if seen[op.ID] {
				t.Errorf("Read accepted duplicate op id %d; input=%q", op.ID, data)
			}
			seen[op.ID] = true
		}
	})
}
