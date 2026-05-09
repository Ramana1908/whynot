// Command explain is the whynot CLI. It loads a JSON history, runs the
// full pipeline (checker → pattern matchers → explanation), and prints
// the result either as JSON (-format json) or human-readable text.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/pattern"
)

func main() {
	format := flag.String("format", "text", "output format: text | json")
	timeout := flag.Duration("timeout", 30*time.Second, "checker timeout")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-format text|json] [-timeout DUR] <history.json>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	h, err := history.Load(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	matchers := make([]explainer.PatternMatcher, 0, len(pattern.All()))
	for _, m := range pattern.All() {
		matchers = append(matchers, m)
	}

	expl, err := explainer.Explain(h, matchers, *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(expl)
		return
	}
	printText(expl)
}

func printText(e *explainer.Explanation) {
	fmt.Printf("verdict: %s\n", e.Verdict)
	if e.Pattern != "" {
		fmt.Printf("pattern: %s\n", e.Pattern)
	}
	if e.Summary != "" {
		fmt.Printf("\n%s\n", e.Summary)
	}
	if len(e.Conflicts) > 0 {
		fmt.Println("\nconflicts:")
		for _, c := range e.Conflicts {
			fmt.Printf("  ops %v:\n    %s\n", c.OpIDs, wrap(c.Why, 76, "    "))
		}
	}
	if e.Witness != nil && len(e.Witness.Ops) > 0 {
		fmt.Printf("\nwitness (%d op%s):\n", len(e.Witness.Ops), plural(len(e.Witness.Ops)))
		for _, op := range e.Witness.Ops {
			fmt.Printf("  [t=%d..%d] client %d: %s\n", op.Call, op.Return, op.Client, opStr(op))
		}
	}
	if len(e.Suggestions) > 0 {
		fmt.Println("\nsuggestions:")
		for _, s := range e.Suggestions {
			fmt.Printf("  - %s\n", s)
		}
	}
}

func opStr(op history.Op) string {
	switch op.Type {
	case history.OpRead:
		if op.Key != "" {
			return fmt.Sprintf("get(%q) -> %d", op.Key, op.Value)
		}
		return fmt.Sprintf("read() -> %d", op.Value)
	case history.OpWrite:
		if op.Key != "" {
			return fmt.Sprintf("put(%q, %d)", op.Key, op.Value)
		}
		return fmt.Sprintf("write(%d)", op.Value)
	}
	return string(op.Type)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// wrap is a tiny word-wrap for the conflict "why" body.
func wrap(s string, width int, indent string) string {
	words := strings.Fields(s)
	var out strings.Builder
	col := 0
	for i, w := range words {
		if col+len(w)+1 > width && col > 0 {
			out.WriteString("\n")
			out.WriteString(indent)
			col = 0
		} else if i > 0 {
			out.WriteString(" ")
			col++
		}
		out.WriteString(w)
		col += len(w)
	}
	return out.String()
}
