package schema

import (
	"fmt"
	"sort"
	"strings"
)

// FilterBySeverity returns the subset of results with the given severity.
// The input slice is not mutated. A nil results slice returns nil.
func FilterBySeverity(results []ValidationResult, severity ValidationSeverity) []ValidationResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]ValidationResult, 0, len(results))
	for _, r := range results {
		if r.Severity == severity {
			out = append(out, r)
		}
	}
	return out
}

// FilterErrors returns only ERROR-severity results.
func FilterErrors(results []ValidationResult) []ValidationResult {
	return FilterBySeverity(results, SeverityError)
}

// FilterWarnings returns only WARNING-severity results.
func FilterWarnings(results []ValidationResult) []ValidationResult {
	return FilterBySeverity(results, SeverityWarning)
}

// FilterInfos returns only INFO-severity results.
func FilterInfos(results []ValidationResult) []ValidationResult {
	return FilterBySeverity(results, SeverityInfo)
}

// GroupByTable buckets results by their Table field. The returned map keys are
// sortable by the caller; map ordering itself is not guaranteed.
func GroupByTable(results []ValidationResult) map[string][]ValidationResult {
	grouped := make(map[string][]ValidationResult)
	for _, r := range results {
		grouped[r.Table] = append(grouped[r.Table], r)
	}
	return grouped
}

// HasErrors reports whether any ERROR-severity result exists in results.
func HasErrors(results []ValidationResult) bool {
	for _, r := range results {
		if r.Severity == SeverityError {
			return true
		}
	}
	return false
}

// FormatValidationReport renders a human-readable schema report. When
// includeInfo is false (the default in surql-py), INFO-severity findings are
// omitted from the output.
func FormatValidationReport(results []ValidationResult, includeInfo bool) string {
	if len(results) == 0 {
		return "No schema validation issues found."
	}

	filtered := results
	if !includeInfo {
		filtered = make([]ValidationResult, 0, len(results))
		for _, r := range results {
			if r.Severity != SeverityInfo {
				filtered = append(filtered, r)
			}
		}
	}

	if len(filtered) == 0 {
		return "No significant schema validation issues found."
	}

	grouped := GroupByTable(filtered)
	errorCount := len(FilterErrors(filtered))
	warningCount := len(FilterWarnings(filtered))

	var b strings.Builder
	fmt.Fprintf(&b, "Schema Validation Report: %d errors, %d warnings\n",
		errorCount, warningCount)
	b.WriteString(strings.Repeat("=", 60))

	tableNames := make([]string, 0, len(grouped))
	for k := range grouped {
		tableNames = append(tableNames, k)
	}
	sort.Strings(tableNames)

	for _, name := range tableNames {
		b.WriteString("\n\n[")
		b.WriteString(name)
		b.WriteString("]")
		for _, r := range grouped[name] {
			b.WriteString("\n  ")
			b.WriteString(severityIcon(r.Severity))
			b.WriteString(" ")
			b.WriteString(r.Message)
			if r.Field != "" {
				b.WriteString(".")
				b.WriteString(r.Field)
			}
			if r.CodeValue != "" || r.DBValue != "" {
				b.WriteString("\n      code: ")
				b.WriteString(r.CodeValue)
				b.WriteString(", db: ")
				b.WriteString(r.DBValue)
			}
		}
	}

	return b.String()
}

// ValidationSummary aggregates counts across severity levels for quick
// programmatic inspection (CLI JSON output, dashboards).
type ValidationSummary struct {
	Total          int  `json:"total"`
	Errors         int  `json:"errors"`
	Warnings       int  `json:"warnings"`
	Info           int  `json:"info"`
	TablesAffected int  `json:"tables_affected"`
	HasErrors      bool `json:"has_errors"`
}

// GetValidationSummary returns a ValidationSummary for the provided results.
func GetValidationSummary(results []ValidationResult) ValidationSummary {
	return ValidationSummary{
		Total:          len(results),
		Errors:         len(FilterErrors(results)),
		Warnings:       len(FilterWarnings(results)),
		Info:           len(FilterInfos(results)),
		TablesAffected: len(GroupByTable(results)),
		HasErrors:      HasErrors(results),
	}
}

// severityIcon returns a short ASCII badge for a severity level.
func severityIcon(s ValidationSeverity) string {
	switch s {
	case SeverityError:
		return "[!]"
	case SeverityWarning:
		return "[~]"
	case SeverityInfo:
		return "[i]"
	}
	return "[ ]"
}

// normalizeExpression collapses contiguous whitespace and trims the result for
// comparing SurrealQL expression fragments (ASSERT, DEFAULT, VALUE). Returns
// the empty string when the input is empty or only whitespace.
func normalizeExpression(expr string) string {
	return strings.Join(strings.Fields(expr), " ")
}

// extractRecordTarget pulls the target table name out of an assertion of the
// form `$value.table = "name"` produced by RecordField. Returns the empty
// string when no match is found (manual assertions are deliberately skipped to
// avoid false positives).
func extractRecordTarget(assertion string) string {
	const prefix = "$value.table ="
	idx := strings.Index(assertion, prefix)
	if idx == -1 {
		return ""
	}
	rest := strings.TrimSpace(assertion[idx+len(prefix):])
	if rest == "" {
		return ""
	}
	quote := rest[0]
	if quote != '"' && quote != '\'' {
		return ""
	}
	end := strings.IndexByte(rest[1:], quote)
	if end == -1 {
		return ""
	}
	return rest[1 : 1+end]
}

// sortedSliceEqual reports whether a and b contain the same strings, ignoring
// their original order. Used by diffIndex so index column ordering differences
// don't trigger false mismatches (matching surql-py behaviour).
func sortedSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
