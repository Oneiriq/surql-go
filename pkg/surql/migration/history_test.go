package migration

import (
	"testing"
	"time"
)

// --- extractRecords ---

func TestExtractRecords_NilResult(t *testing.T) {
	if got := extractRecords(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestExtractRecords_NestedEnvelope(t *testing.T) {
	result := []any{
		map[string]any{
			"status": "OK",
			"time":   "1.2ms",
			"result": []any{
				map[string]any{"version": "a", "description": "first"},
				map[string]any{"version": "b", "description": "second"},
			},
		},
	}
	records := extractRecords(result)
	if len(records) != 2 {
		t.Fatalf("len=%d, want 2", len(records))
	}
	if records[0]["version"] != "a" {
		t.Errorf("records[0].version=%v, want 'a'", records[0]["version"])
	}
}

func TestExtractRecords_SingleObjectResult(t *testing.T) {
	result := []any{
		map[string]any{
			"result": map[string]any{"version": "x"},
		},
	}
	records := extractRecords(result)
	if len(records) != 1 {
		t.Fatalf("len=%d, want 1", len(records))
	}
	if records[0]["version"] != "x" {
		t.Errorf("unexpected record: %v", records[0])
	}
}

func TestExtractRecords_EmptyResult(t *testing.T) {
	result := []any{
		map[string]any{"status": "OK", "time": "0.1ms", "result": nil},
	}
	records := extractRecords(result)
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

// --- decodeHistoryRecord ---

func TestDecodeHistoryRecord_Full(t *testing.T) {
	ts := "2026-04-18T12:00:00Z"
	rec := map[string]any{
		"version":           "20260418_120000",
		"description":       "create users",
		"applied_at":        ts,
		"checksum":          "deadbeef",
		"execution_time_ms": float64(250),
	}
	entry, ok := decodeHistoryRecord(rec)
	if !ok {
		t.Fatal("expected decode to succeed")
	}
	if entry.Version != "20260418_120000" {
		t.Errorf("Version=%q", entry.Version)
	}
	if entry.Checksum != "deadbeef" {
		t.Errorf("Checksum=%q", entry.Checksum)
	}
	if entry.ExecutionTimeMs == nil || *entry.ExecutionTimeMs != 250 {
		t.Errorf("ExecutionTimeMs=%v, want 250", entry.ExecutionTimeMs)
	}
	if entry.AppliedAt.IsZero() {
		t.Error("AppliedAt should be parsed")
	}
}

func TestDecodeHistoryRecord_MissingVersion(t *testing.T) {
	rec := map[string]any{"description": "no version"}
	if _, ok := decodeHistoryRecord(rec); ok {
		t.Error("expected decode to fail for missing version")
	}
}

func TestDecodeHistoryRecord_EmptyVersion(t *testing.T) {
	rec := map[string]any{"version": ""}
	if _, ok := decodeHistoryRecord(rec); ok {
		t.Error("expected decode to fail for empty version")
	}
}

func TestDecodeHistoryRecord_NullExecutionTime(t *testing.T) {
	rec := map[string]any{
		"version":           "a",
		"description":       "",
		"applied_at":        "2026-04-18T12:00:00Z",
		"checksum":          "",
		"execution_time_ms": nil,
	}
	entry, ok := decodeHistoryRecord(rec)
	if !ok {
		t.Fatal("expected decode to succeed")
	}
	if entry.ExecutionTimeMs != nil {
		t.Errorf("expected nil ExecutionTimeMs, got %v", entry.ExecutionTimeMs)
	}
}

// --- parseHistoryDatetime ---

func TestParseHistoryDatetime_RFC3339(t *testing.T) {
	got := parseHistoryDatetime("2026-04-18T12:00:00Z")
	want := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parse = %v, want %v", got, want)
	}
}

func TestParseHistoryDatetime_RFC3339Nano(t *testing.T) {
	got := parseHistoryDatetime("2026-04-18T12:00:00.123456789Z")
	if got.IsZero() {
		t.Error("expected parse to succeed")
	}
}

func TestParseHistoryDatetime_DirectTime(t *testing.T) {
	in := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	got := parseHistoryDatetime(in)
	if !got.Equal(in) {
		t.Errorf("parse = %v, want %v", got, in)
	}
}

func TestParseHistoryDatetime_InvalidReturnsZero(t *testing.T) {
	got := parseHistoryDatetime("not-a-date")
	if !got.IsZero() {
		t.Errorf("expected zero time, got %v", got)
	}
}

// --- coerceInt64 ---

func TestCoerceInt64(t *testing.T) {
	cases := []struct {
		in   any
		want int64
		ok   bool
	}{
		{int(3), 3, true},
		{int32(4), 4, true},
		{int64(5), 5, true},
		{uint(6), 6, true},
		{uint32(7), 7, true},
		{uint64(8), 8, true},
		{float32(9.5), 9, true},
		{float64(10.7), 10, true},
		{"not-a-number", 0, false},
		{nil, 0, false},
	}
	for _, tc := range cases {
		got, ok := coerceInt64(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("coerceInt64(%v)=(%d,%v), want (%d,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
