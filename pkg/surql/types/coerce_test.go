package types

import (
	"testing"
	"time"
)

func TestCoerceDatetime_ZSuffix(t *testing.T) {
	got, err := CoerceDatetime("2024-01-15T10:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCoerceDatetime_Offset(t *testing.T) {
	got, err := CoerceDatetime("2024-01-15T10:30:00+00:00")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hour() != 10 || got.Location() != time.UTC {
		t.Errorf("got %v (loc=%v)", got, got.Location())
	}
}

func TestCoerceDatetime_Nanoseconds(t *testing.T) {
	got, err := CoerceDatetime("2024-01-15T10:30:00.123456789Z")
	if err != nil {
		t.Fatal(err)
	}
	if got.Nanosecond() != 123456789 {
		t.Errorf("nsec=%d", got.Nanosecond())
	}
}

func TestCoerceDatetime_Naive(t *testing.T) {
	got, err := CoerceDatetime("2024-01-15T10:30:00")
	if err != nil {
		t.Fatal(err)
	}
	if got.Location() != time.UTC {
		t.Errorf("expected UTC, got %v", got.Location())
	}
}

func TestCoerceDatetime_RejectsGarbage(t *testing.T) {
	if _, err := CoerceDatetime("not-a-date"); err == nil {
		t.Error("expected error")
	}
}

func TestCoerceRecordDatetimes_ReplacesStringWithTime(t *testing.T) {
	data := map[string]any{
		"name":       "Alice",
		"created_at": "2024-01-15T10:30:00Z",
	}
	out, err := CoerceRecordDatetimes(data, []string{"created_at"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["created_at"].(time.Time); !ok {
		t.Errorf("created_at not time.Time: %T", out["created_at"])
	}
	if out["name"] != "Alice" {
		t.Errorf("name corrupted: %v", out["name"])
	}
}

func TestCoerceRecordDatetimes_SkipsMissingAndNil(t *testing.T) {
	data := map[string]any{"deleted_at": nil}
	out, err := CoerceRecordDatetimes(data, []string{"deleted_at", "not_present"})
	if err != nil {
		t.Fatal(err)
	}
	if out["deleted_at"] != nil {
		t.Errorf("expected nil, got %v", out["deleted_at"])
	}
}

func TestCoerceRecordDatetimes_WrapsFieldError(t *testing.T) {
	data := map[string]any{"created_at": "not-a-date"}
	_, err := CoerceRecordDatetimes(data, []string{"created_at"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsSubstr(err.Error(), "created_at") {
		t.Errorf("expected field name in error: %q", err.Error())
	}
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
