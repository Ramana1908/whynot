// Command explain is the whynot CLI. It loads a JSON history, runs the
// full pipeline (checker → pattern matchers → explanation), and prints
// the result either as JSON (-format json) or human-readable text.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/pattern"
)

func main() {
	os.Exit(run(os.Args[0], os.Args[1:], os.Stdout, os.Stderr))
}

// run is main() factored to be testable. It returns the process exit code:
// 0 success, 1 runtime error (load/explain), 2 usage error.
func run(progName string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(progName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text | json")
	timeout := fs.Duration("timeout", 30*time.Second, "checker timeout")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s [-format text|json] [-timeout DUR] <history.json>\n", progName)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}

	h, err := history.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	matchers := make([]explainer.PatternMatcher, 0, len(pattern.All()))
	for _, m := range pattern.All() {
		matchers = append(matchers, m)
	}

	expl, err := explainer.Explain(h, matchers, *timeout)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	if *format == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(expl)
		return 0
	}
	printText(stdout, expl)
	return 0
}

func printText(w io.Writer, e *explainer.Explanation) {
	fmt.Fprintf(w, "verdict: %s\n", e.Verdict)
	if e.Pattern != "" {
		fmt.Fprintf(w, "pattern: %s\n", e.Pattern)
	}
	if e.Summary != "" {
		fmt.Fprintf(w, "\n%s\n", e.Summary)
	}
	if len(e.Conflicts) > 0 {
		fmt.Fprintln(w, "\nconflicts:")
		for _, c := range e.Conflicts {
			fmt.Fprintf(w, "  ops %v:\n    %s\n", c.OpIDs, wrap(c.Why, 76, "    "))
		}
	}
	if e.Witness != nil && len(e.Witness.Ops) > 0 {
		fmt.Fprintf(w, "\nwitness (%d op%s):\n", len(e.Witness.Ops), plural(len(e.Witness.Ops)))
		for _, op := range e.Witness.Ops {
			fmt.Fprintf(w, "  [t=%d..%d] client %d: %s\n", op.Call, op.Return, op.Client, opStr(op))
		}
	}
	if len(e.Suggestions) > 0 {
		fmt.Fprintln(w, "\nsuggestions:")
		for _, s := range e.Suggestions {
			fmt.Fprintf(w, "  - %s\n", s)
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
