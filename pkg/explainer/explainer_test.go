package explainer_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Ramana1908/whynot/pkg/explainer"
	"github.com/Ramana1908/whynot/pkg/history"
	"github.com/Ramana1908/whynot/pkg/pattern"
)

var update = flag.Bool("update", false, "rewrite golden files from current explainer output")

func matchers() []explainer.PatternMatcher {
	out := make([]explainer.PatternMatcher, 0, len(pattern.All()))
	for _, m := range pattern.All() {
		out = append(out, m)
	}
	return out
}

// TestGoldenExplanations runs the full pipeline on each fixture in
// testdata/histories/ and compares the result with the corresponding
// golden file in testdata/golden/. Use `go test -update` to refresh.
func TestGoldenExplanations(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "histories")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}

	cases := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			cases = append(cases, e.Name())
		}
	}
	sort.Strings(cases)

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			h, err := history.Load(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("load history: %v", err)
			}
			got, err := explainer.Explain(h, matchers(), 5*time.Second)
			if err != nil {
				t.Fatalf("explain: %v", err)
			}

			goldenPath := filepath.Join("..", "..", "testdata", "golden", name)
			if *update {
				writeGolden(t, goldenPath, got)
				return
			}

			want := loadGolden(t, goldenPath)
			compareExplanations(t, want, got)
		})
	}
}

func compareExplanations(t *testing.T, want, got *explainer.Explanation) {
	t.Helper()
	gotJSON := mustMarshal(t, got)
	wantJSON := mustMarshal(t, want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("explanation mismatch.\n--- want ---\n%s\n--- got ---\n%s", wantJSON, gotJSON)
	}
}

func mustMarshal(t *testing.T, e *explainer.Explanation) []byte {
	t.Helper()
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func loadGolden(t *testing.T, path string) *explainer.Explanation {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open golden %s: %v", path, err)
	}
	defer f.Close()
	var e explainer.Explanation
	if err := json.NewDecoder(f).Decode(&e); err != nil {
		t.Fatalf("decode golden %s: %v", path, err)
	}
	return &e
}

func writeGolden(t *testing.T, path string, e *explainer.Explanation) {
	t.Helper()
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
}
