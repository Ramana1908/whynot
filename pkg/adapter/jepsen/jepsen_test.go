package jepsen_test

import (
	"strings"
	"testing"

	"github.com/Ramana1908/whynot"
	"github.com/Ramana1908/whynot/pkg/adapter/jepsen"
)

const staleReadEDN = `
[{:type :invoke, :process 0, :time 0,    :f :write, :value 1}
 {:type :ok,     :process 0, :time 10,   :f :write, :value 1}
 {:type :invoke, :process 1, :time 20,   :f :read,  :value nil}
 {:type :ok,     :process 1, :time 30,   :f :read,  :value 0}]
`

func TestReadEDN_StaleRead(t *testing.T) {
	h, err := jepsen.ReadEDN(strings.NewReader(staleReadEDN), "register", 0)
	if err != nil {
		t.Fatalf("ReadEDN: %v", err)
	}
	if len(h.Ops) != 2 {
		t.Fatalf("op count = %d, want 2", len(h.Ops))
	}
	expl, err := whynot.Explain(h)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if expl.Pattern != "stale_read" {
		t.Errorf("pattern = %q, want stale_read", expl.Pattern)
	}
}

const lostUpdateEDN = `
[{:type :invoke, :process 0, :time 0,   :f :write, :value 5}
 {:type :ok,     :process 0, :time 10,  :f :write, :value 5}
 {:type :invoke, :process 1, :time 20,  :f :write, :value 99}
 {:type :ok,     :process 1, :time 30,  :f :write, :value 99}
 {:type :invoke, :process 2, :time 40,  :f :read,  :value nil}
 {:type :ok,     :process 2, :time 50,  :f :read,  :value 5}]
`

func TestReadEDN_LostUpdate(t *testing.T) {
	h, err := jepsen.ReadEDN(strings.NewReader(lostUpdateEDN), "register", 0)
	if err != nil {
		t.Fatalf("ReadEDN: %v", err)
	}
	expl, err := whynot.Explain(h)
	if err != nil {
		t.Fatal(err)
	}
	if expl.Pattern != "lost_update" {
		t.Errorf("pattern = %q, want lost_update", expl.Pattern)
	}
}

// :fail and :info entries should be dropped — they do not appear on the
// linearization timeline.
const withFailAndInfoEDN = `
[{:type :invoke, :process 0, :time 0,   :f :write, :value 1}
 {:type :ok,     :process 0, :time 10,  :f :write, :value 1}
 {:type :invoke, :process 1, :time 12,  :f :write, :value 2}
 {:type :fail,   :process 1, :time 15,  :f :write, :value 2}
 {:type :invoke, :process 2, :time 16,  :f :write, :value 3}
 {:type :info,   :process 2, :time 18,  :f :write, :value 3}
 {:type :invoke, :process 3, :time 20,  :f :read,  :value nil}
 {:type :ok,     :process 3, :time 30,  :f :read,  :value 0}]
`

func TestReadEDN_DropsFailAndInfo(t *testing.T) {
	h, err := jepsen.ReadEDN(strings.NewReader(withFailAndInfoEDN), "register", 0)
	if err != nil {
		t.Fatalf("ReadEDN: %v", err)
	}
	if len(h.Ops) != 2 {
		t.Fatalf("op count = %d, want 2 (write + read; fail/info dropped)", len(h.Ops))
	}
}

// Non-integer process (e.g. :nemesis) should be silently skipped.
const withNemesisEDN = `
[{:type :info,   :process :nemesis, :time 0, :f :start-partition}
 {:type :invoke, :process 0,        :time 5,  :f :write, :value 1}
 {:type :ok,     :process 0,        :time 10, :f :write, :value 1}
 {:type :info,   :process :nemesis, :time 12, :f :stop-partition}
 {:type :invoke, :process 1,        :time 20, :f :read,  :value nil}
 {:type :ok,     :process 1,        :time 30, :f :read,  :value 0}]
`

func TestReadEDN_SkipsNemesis(t *testing.T) {
	h, err := jepsen.ReadEDN(strings.NewReader(withNemesisEDN), "register", 0)
	if err != nil {
		t.Fatalf("ReadEDN: %v", err)
	}
	if len(h.Ops) != 2 {
		t.Fatalf("op count = %d, want 2 (nemesis events skipped)", len(h.Ops))
	}
	expl, err := whynot.Explain(h)
	if err != nil {
		t.Fatal(err)
	}
	if expl.Pattern != "stale_read" {
		t.Errorf("pattern = %q, want stale_read", expl.Pattern)
	}
}

// Line-delimited maps (no top-level vector) should also work.
const lineDelimitedEDN = `
{:type :invoke, :process 0, :time 0,  :f :write, :value 1}
{:type :ok,     :process 0, :time 10, :f :write, :value 1}
{:type :invoke, :process 1, :time 20, :f :read,  :value nil}
{:type :ok,     :process 1, :time 30, :f :read,  :value 0}
`

func TestReadEDN_LineDelimited(t *testing.T) {
	h, err := jepsen.ReadEDN(strings.NewReader(lineDelimitedEDN), "register", 0)
	if err != nil {
		t.Fatalf("ReadEDN: %v", err)
	}
	if len(h.Ops) != 2 {
		t.Fatalf("op count = %d", len(h.Ops))
	}
}

// Comma-as-whitespace should be tolerated.
func TestReadEDN_CommasAsWhitespace(t *testing.T) {
	src := `[{:type :invoke, :process 0, :time 0, :f :write, :value 1},
		      {:type :ok, :process 0, :time 10, :f :write, :value 1}]`
	_, err := jepsen.ReadEDN(strings.NewReader(src), "register", 0)
	if err != nil {
		t.Fatalf("ReadEDN: %v", err)
	}
}

// Comments (;) should be skipped.
func TestReadEDN_Comments(t *testing.T) {
	src := `
; this is a Jepsen run
[{:type :invoke, :process 0, :time 0, :f :write, :value 1}  ; client 0 issues
 {:type :ok,     :process 0, :time 10, :f :write, :value 1}] ; and acks
`
	_, err := jepsen.ReadEDN(strings.NewReader(src), "register", 0)
	if err != nil {
		t.Fatalf("ReadEDN: %v", err)
	}
}

func TestReadEDN_MalformedRejected(t *testing.T) {
	_, err := jepsen.ReadEDN(strings.NewReader(`[{:type :invoke, :process 0`), "register", 0)
	if err == nil {
		t.Fatal("expected parse error on truncated input")
	}
}

func TestReadEDN_OkWithoutInvokeRejected(t *testing.T) {
	src := `[{:type :ok, :process 0, :time 10, :f :write, :value 1}]`
	_, err := jepsen.ReadEDN(strings.NewReader(src), "register", 0)
	if err == nil {
		t.Fatal("expected error: :ok with no preceding :invoke")
	}
}

func TestReadEDN_NegativeAndLargeTimes(t *testing.T) {
	src := `[{:type :invoke, :process 0, :time -5,         :f :write, :value 1}
	         {:type :ok,     :process 0, :time 9999999999, :f :write, :value 1}]`
	h, err := jepsen.ReadEDN(strings.NewReader(src), "register", 0)
	if err != nil {
		t.Fatalf("ReadEDN: %v", err)
	}
	if h.Ops[0].Call != -5 || h.Ops[0].Return != 9999999999 {
		t.Errorf("times not preserved: %+v", h.Ops[0])
	}
}
