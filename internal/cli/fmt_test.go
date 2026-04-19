package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestPrinter_NoColor ensures coloring is off when the color argument is
// false (covers NO_COLOR and non-tty writer cases without touching env).
func TestPrinter_NoColor(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	p := NewPrinter(out, errOut, false)
	p.Successf("it works")
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("expected no ANSI escapes, got %q", out.String())
	}
	if !strings.Contains(out.String(), "[ok]") {
		t.Fatalf("expected marker in output, got %q", out.String())
	}
}

// TestPrinter_Color ensures ANSI escapes are emitted when the color flag
// is on.
func TestPrinter_Color(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	p := NewPrinter(out, errOut, true)
	p.Errorf("broken")
	if !strings.Contains(errOut.String(), "\x1b[31m") {
		t.Fatalf("expected red ANSI escape, got %q", errOut.String())
	}
}

// TestPrinter_Quiet suppresses info/success messages while preserving
// warnings and errors.
func TestPrinter_Quiet(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	p := NewPrinter(out, errOut, false)
	p.SetQuiet(true)
	p.Infof("hidden")
	p.Successf("hidden")
	p.Warnf("shown")
	p.Errorf("shown")
	if out.Len() != 0 {
		t.Fatalf("quiet mode should suppress stdout, got %q", out.String())
	}
	if !strings.Contains(errOut.String(), "shown") {
		t.Fatalf("warn/error should pass through quiet mode, got %q", errOut.String())
	}
}

// TestPrinter_Silent suppresses every channel, including warn/error.
func TestPrinter_Silent(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	p := NewPrinter(out, errOut, false)
	p.SetSilent(true)
	p.Infof("nope")
	p.Successf("nope")
	p.Warnf("nope")
	p.Errorf("nope")
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("silent mode should suppress all output, stdout=%q stderr=%q",
			out.String(), errOut.String())
	}
}

// TestPrinter_JSON verifies pretty-printed JSON is emitted to stdout.
func TestPrinter_JSON(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	p := NewPrinter(out, errOut, false)
	payload := map[string]any{"alpha": 1, "beta": "two"}
	if err := p.JSON(payload); err != nil {
		t.Fatalf("unexpected JSON error: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(out.Bytes(), &back); err != nil {
		t.Fatalf("output was not valid JSON: %v", err)
	}
	if back["alpha"] != float64(1) || back["beta"] != "two" {
		t.Fatalf("round-trip mismatch: %v", back)
	}
}

// TestPrinter_Table_RendersColumns validates that a header and rows
// produce the expected columns. Column alignment is position-stable.
func TestPrinter_Table_RendersColumns(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	p := NewPrinter(out, errOut, false)
	p.Table([]string{"A", "B"}, [][]string{
		{"1", "two"},
		{"three", "4"},
	})
	got := out.String()
	if !strings.Contains(got, "A") || !strings.Contains(got, "B") {
		t.Fatalf("headers missing, got %q", got)
	}
	for _, w := range []string{"1", "two", "three", "4"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing cell %q in table output", w)
		}
	}
}

// TestTruncateString covers boundary conditions.
func TestTruncateString(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short", "abc", 10, "abc"},
		{"exact", "abcdefghij", 10, "abcdefghij"},
		{"truncated", "abcdefghijkl", 8, "abcde..."},
		{"tiny-limit", "abcdefghij", 2, "abcdefghij"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateString(tc.in, tc.max)
			if got != tc.want {
				t.Errorf("truncateString(%q,%d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}

// TestPadRight verifies padding semantics.
func TestPadRight(t *testing.T) {
	if got := padRight("hi", 5); got != "hi   " {
		t.Errorf("padRight(\"hi\",5)=%q", got)
	}
	if got := padRight("hello", 3); got != "hello" {
		t.Errorf("padRight should not truncate, got %q", got)
	}
}

// TestSortedStringKeys returns stable alphabetic ordering.
func TestSortedStringKeys(t *testing.T) {
	m := map[string]any{"c": 1, "a": 2, "b": 3}
	keys := sortedStringKeys(m)
	want := []string{"a", "b", "c"}
	if len(keys) != len(want) {
		t.Fatalf("length mismatch: %v", keys)
	}
	for i, k := range want {
		if keys[i] != k {
			t.Errorf("key[%d]=%q, want %q", i, keys[i], k)
		}
	}
}
