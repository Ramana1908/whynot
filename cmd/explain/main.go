// Command explain is the whynot CLI. At this stage it is a baseline driver:
// it loads a JSON history, runs Porcupine's checker, and prints whatever
// Porcupine reports — i.e., the uninformative output that motivates the
// rest of the project.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/model"
	"github.com/anishathalye/porcupine"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s <history.json>\n", os.Args[0])
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

	if h.Model != "register" {
		fmt.Fprintf(os.Stderr, "model %q not yet supported by baseline driver\n", h.Model)
		os.Exit(1)
	}

	m := model.Register(h.Init)
	ops := model.RegisterOps(h)

	res, info := porcupine.CheckOperationsVerbose(m, ops, 10*time.Second)

	fmt.Printf("history: %s (%d ops)\n", flag.Arg(0), len(ops))
	fmt.Printf("porcupine result: %s\n", res)

	if res == porcupine.Illegal {
		fmt.Println()
		fmt.Println("--- baseline output (this is what we want to improve) ---")
		fmt.Println("Porcupine says: Illegal.")
		fmt.Println("That is the entire actionable output.")
		fmt.Println()
		// Show that PartialLinearizationsOperations has *some* info, but
		// is not yet an explanation.
		parts := info.PartialLinearizationsOperations()
		fmt.Printf("LinearizationInfo: %d partition(s)\n", len(parts))
		for i, partials := range parts {
			fmt.Printf("  partition %d: %d partial linearization(s) found\n", i, len(partials))
			for j, lin := range partials {
				if j >= 2 {
					fmt.Printf("    ... (%d more)\n", len(partials)-2)
					break
				}
				fmt.Printf("    [%d] length=%d\n", j, len(lin))
			}
		}
	}
}
