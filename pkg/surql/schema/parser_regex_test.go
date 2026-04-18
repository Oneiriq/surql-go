package schema

import "testing"

// Regression coverage for issue #32: the parser's extractAssertion,
// extractDefault, and extractValue regexes over-matched when an
// expression contained `$value` / `$default`, because the case-
// insensitive VALUE / DEFAULT keywords matched inside the tail of the
// $-prefixed identifier. Fix anchors the leading keyword with
// `(?:^|\s)`.

func TestExtractAssertion_DollarValueIsPreserved(t *testing.T) {
	cases := map[string]string{
		"ASSERT $value >= 0":                                "$value >= 0",
		"ASSERT $value >= 0 AND $value <= 150":              "$value >= 0 AND $value <= 150",
		"ASSERT string::is::email($value)":                  "string::is::email($value)",
		"ASSERT $value != NONE AND string::len($value) > 0": "$value != NONE AND string::len($value) > 0",
		"ASSERT array::len($value) > 0":                     "array::len($value) > 0",
		"DEFINE FIELD x ON y TYPE int ASSERT $value > 0":    "$value > 0",
	}
	for input, want := range cases {
		if got := extractAssertion(input); got != want {
			t.Errorf("extractAssertion(%q): got %q, want %q", input, got, want)
		}
	}
}

func TestExtractAssertion_StopsAtTerminator(t *testing.T) {
	cases := map[string]string{
		`ASSERT string::is::email($value) DEFAULT "a@b.example"`: "string::is::email($value)",
		"ASSERT $value >= 0 VALUE $value":                        "$value >= 0",
		"ASSERT $value != NONE READONLY":                         "$value != NONE",
		"ASSERT $value >= 0;":                                    "$value >= 0",
	}
	for input, want := range cases {
		if got := extractAssertion(input); got != want {
			t.Errorf("extractAssertion(%q): got %q, want %q", input, got, want)
		}
	}
}

func TestExtractDefault_DollarValueIsPreserved(t *testing.T) {
	cases := map[string]string{
		"DEFAULT $value + 1":                              "$value + 1",
		"DEFAULT $value + 1 VALUE 42":                     "$value + 1",
		"DEFAULT $value + 1 ASSERT $value > 0":            "$value + 1",
		"DEFAULT time::now()":                             "time::now()",
		"DEFINE FIELD x ON y TYPE int DEFAULT $value + 1": "$value + 1",
	}
	for input, want := range cases {
		if got := extractDefault(input); got != want {
			t.Errorf("extractDefault(%q): got %q, want %q", input, got, want)
		}
	}
}

func TestExtractValue_DollarValueIsPreserved(t *testing.T) {
	cases := map[string]string{
		"VALUE $value * 2":                              "$value * 2",
		"VALUE $value * 2 READONLY":                     "$value * 2",
		"VALUE $value + 1 DEFAULT 0":                    "$value + 1",
		"DEFINE FIELD x ON y TYPE int VALUE $value * 2": "$value * 2",
	}
	for input, want := range cases {
		if got := extractValue(input); got != want {
			t.Errorf("extractValue(%q): got %q, want %q", input, got, want)
		}
	}
}

// Previously, these produced non-empty false matches because the leading
// VALUE / DEFAULT keyword matched inside `$value` / `$default`.
func TestExtractValue_DoesNotMatchInsideDollarValue(t *testing.T) {
	cases := []string{
		"ASSERT $value >= 0",
		"ASSERT $value >= 0 AND $value <= 150",
		"DEFINE FIELD x ON y TYPE int ASSERT $value > 0",
	}
	for _, input := range cases {
		if got := extractValue(input); got != "" {
			t.Errorf("extractValue(%q): got %q, want empty", input, got)
		}
	}
}

func TestExtractDefault_DoesNotMatchInsideDollarValue(t *testing.T) {
	cases := []string{
		"ASSERT $value >= 0",
		"VALUE $value * 2 READONLY",
	}
	for _, input := range cases {
		if got := extractDefault(input); got != "" {
			t.Errorf("extractDefault(%q): got %q, want empty", input, got)
		}
	}
}
