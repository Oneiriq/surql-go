package types

import (
	"encoding/json"
	"testing"
)

// BenchmarkParseRecordID exercises the common parse path (simple id that
// does not need angle brackets).
func BenchmarkParseRecordID(b *testing.B) {
	const input = "user:abc123"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ParseRecordID(input); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseRecordID_Angle exercises the angle-bracket path used for
// ids that contain non-simple characters.
func BenchmarkParseRecordID_Angle(b *testing.B) {
	const input = "user:<some-uuid-like-id>"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ParseRecordID(input); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRecordID_String measures the common render path.
func BenchmarkRecordID_String(b *testing.B) {
	rid, err := NewStringRecordID("user", "abc123")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = rid.String()
	}
}

// BenchmarkRecordID_JSONRoundtrip covers MarshalJSON + UnmarshalJSON, the
// hot path for wire-format record ids.
func BenchmarkRecordID_JSONRoundtrip(b *testing.B) {
	rid, err := NewStringRecordID("user", "abc123")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(rid)
		if err != nil {
			b.Fatal(err)
		}
		var out RecordID
		if err := json.Unmarshal(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCoerceDatetime exercises the fast RFC3339Nano path used by the
// vast majority of SurrealDB datetime fields.
func BenchmarkCoerceDatetime(b *testing.B) {
	const input = "2024-01-15T10:30:00.123456789Z"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := CoerceDatetime(input); err != nil {
			b.Fatal(err)
		}
	}
}
