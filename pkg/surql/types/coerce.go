package types

import (
	"strings"
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// CoerceDatetime converts a SurrealDB ISO-8601 datetime string into a
// time.Time in UTC.
//
// Supported inputs:
//   - 2024-01-15T10:30:00Z
//   - 2024-01-15T10:30:00+00:00
//   - 2024-01-15T10:30:00.123456789Z  (nanoseconds)
//   - 2024-01-15T10:30:00             (naive; treated as UTC)
func CoerceDatetime(value string) (time.Time, error) {
	// Try RFC 3339 first (handles Z, offsets, and fractional seconds).
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC(), nil
	}
	// Naive formats (no timezone).
	layouts := []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05.999",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, surqlerrors.Newf(
		surqlerrors.ErrValidation,
		"could not parse datetime string %q", value,
	)
}

// CoerceRecordDatetimes returns a copy of `data` with the listed string
// fields parsed as UTC datetimes. Missing, null, or non-string fields are
// left untouched. A parse failure returns the error for the offending
// field and leaves callers free to recover.
func CoerceRecordDatetimes(
	data map[string]any,
	datetimeFields []string,
) (map[string]any, error) {
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = v
	}
	for _, field := range datetimeFields {
		raw, ok := out[field]
		if !ok || raw == nil {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			continue
		}
		t, err := CoerceDatetime(s)
		if err != nil {
			return nil, surqlerrors.Wrapf(
				surqlerrors.ErrValidation,
				err,
				"field %q", field,
			)
		}
		out[field] = t
	}
	return out, nil
}

// normaliseNanos truncates trailing nanos past 9 digits so the RFC3339
// parser always succeeds (kept internal for symmetry with surql-rs).
//
//nolint:unused // retained as a reference helper
func normaliseNanos(value string) string {
	idx := strings.Index(value, ".")
	if idx < 0 {
		return value
	}
	end := idx + 1
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	frac := value[idx+1 : end]
	if len(frac) <= 9 {
		return value
	}
	return value[:idx+1] + frac[:9] + value[end:]
}
