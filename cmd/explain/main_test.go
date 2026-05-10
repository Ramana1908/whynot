package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDir resolves to the repo's testdata/histories directory regardless
// of where `go test` is invoked from.
func fixtureDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "histories")
}

func TestCLI_TextOutput_AllFixtures(t *testing.T) {
	cases := []struct {
		fixture     string
		wantPattern string // "" for the linearizable control
	}{
		{"linearizable_control.json", ""},
		{"stale_read_simple.json", "stale_read"},
		{"lost_update.json", "lost_update"},
		{"realtime_inversion.json", "realtime_inversion"},
		{"non_monotonic.json", "non_monotonic_read"},
		{"phantom_value.json", "phantom_value"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			path := filepath.Join(fixtureDir(t), tc.fixture)
			var stdout, stderr bytes.Buffer
			code := run("explain", []string{path}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
			}
			out := stdout.String()
			if !strings.HasPrefix(out, "verdict: ") {
				t.Errorf("expected output to start with 'verdict: ', got %q", firstLine(out))
			}
			if tc.wantPattern == "" {
				if strings.Contains(out, "pattern:") {
					t.Errorf("linearizable fixture should not have a pattern line; got:\n%s", out)
				}
				if !strings.Contains(out, "linearizable") {
					t.Errorf("expected 'linearizable' in output, got:\n%s", out)
				}
				return
			}
			if !strings.Contains(out, "pattern: "+tc.wantPattern) {
				t.Errorf("expected 'pattern: %s' in output, got:\n%s", tc.wantPattern, out)
			}
			if !strings.Contains(out, "witness (") {
				t.Errorf("expected witness section in output, got:\n%s", out)
			}
			if !strings.Contains(out, "suggestions:") {
				t.Errorf("expected suggestions section in output, got:\n%s", out)
			}
		})
	}
}

func TestCLI_JSONOutput_WellFormed(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "stale_read_simple.json")
	var stdout, stderr bytes.Buffer
	code := run("explain", []string{"-format", "json", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}
	for _, k := range []string{"verdict", "pattern", "summary", "witness", "conflicts", "suggestions"} {
		if _, ok := got[k]; !ok {
			t.Errorf("JSON missing key %q; got keys %v", k, mapKeys(got))
		}
	}
	if got["verdict"] != "non-linearizable" {
		t.Errorf("verdict = %v, want non-linearizable", got["verdict"])
	}
	if got["pattern"] != "stale_read" {
		t.Errorf("pattern = %v, want stale_read", got["pattern"])
	}
}

func TestCLI_JSONOutput_LinearizableHasNoPatternKey(t *testing.T) {
	// `pattern,omitempty` should drop the key when no pattern fired.
	path := filepath.Join(fixtureDir(t), "linearizable_control.json")
	var stdout, stderr bytes.Buffer
	code := run("explain", []string{"-format", "json", path}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := got["pattern"]; ok {
		t.Errorf("linearizable JSON should omit 'pattern'; got %v", got)
	}
	if got["verdict"] != "linearizable" {
		t.Errorf("verdict = %v, want linearizable", got["verdict"])
	}
}

func TestCLI_NoArgs_ExitsWithUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run("explain", nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr should contain usage line; got %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("usage error should not write to stdout; got %q", stdout.String())
	}
}

func TestCLI_TooManyArgs_ExitsWithUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run("explain", []string{"a.json", "b.json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestCLI_MissingFile_ExitsOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run("explain", []string{filepath.Join(t.TempDir(), "does-not-exist.json")}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr should contain 'error:'; got %q", stderr.String())
	}
}

func TestCLI_MalformedJSON_ExitsOne(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run("explain", []string{bad}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestCLI_UnknownFlag_ExitsTwo(t *testing.T) {
	// flag.ContinueOnError causes Parse to return an error, which we
	// translate to exit 2.
	var stdout, stderr bytes.Buffer
	code := run("explain", []string{"-nope"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
