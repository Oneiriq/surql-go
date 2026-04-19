package types

import (
	"encoding/json"
	"testing"
)

func TestSurqlFn_NoArgs(t *testing.T) {
	got := SurqlFn("time::now").ToSurql()
	if got != "time::now()" {
		t.Errorf("got %q", got)
	}
}

func TestSurqlFn_WithArgs(t *testing.T) {
	got := SurqlFn("time::format", "created_at", "%Y-%m-%d").ToSurql()
	want := "time::format(created_at, %Y-%m-%d)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSurqlFn_SingleArg(t *testing.T) {
	got := SurqlFn("math::sum", "scores").ToSurql()
	if got != "math::sum(scores)" {
		t.Errorf("got %q", got)
	}
}

func TestSurrealFn_StringMatchesToSurql(t *testing.T) {
	f := SurqlFn("time::now")
	if f.String() != f.ToSurql() {
		t.Errorf("String=%q ToSurql=%q", f.String(), f.ToSurql())
	}
}

func TestSurrealFn_JSONRoundtrip(t *testing.T) {
	f := NewSurrealFn("time::now()")
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back SurrealFn
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back != f {
		t.Errorf("roundtrip: got %+v, want %+v", back, f)
	}
}

func TestSurqlFn_IntegerArg(t *testing.T) {
	got := SurqlFn("math::add", 1, 2).ToSurql()
	if got != "math::add(1, 2)" {
		t.Errorf("got %q", got)
	}
}

func TestTypeRecord(t *testing.T) {
	tests := []struct {
		name  string
		table string
		id    any
		want  string
	}{
		{"string id", "task", "abc", "type::record('task', 'abc')"},
		{"int id", "post", 123, "type::record('post', 123)"},
		{"int64 id", "post", int64(9001), "type::record('post', 9001)"},
		{"uint id", "post", uint(42), "type::record('post', 42)"},
		{"string with quote", "user", "o'malley", `type::record('user', 'o\'malley')`},
		{"string with backslash", "user", `a\b`, `type::record('user', 'a\\b')`},
		{"RecordIDValue string", "task", StringID("abc"), "type::record('task', 'abc')"},
		{"RecordIDValue int", "task", IntID(7), "type::record('task', 7)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TypeRecord(tc.table, tc.id).ToSurql()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTypeThing_AliasesTypeRecord(t *testing.T) {
	// type::thing was removed in SurrealDB v3+; the helper emits
	// type::record(...) so downstream callers that prefer the "thing"
	// nomenclature still produce v3-valid SurrealQL.
	tests := []struct {
		name  string
		table string
		id    any
		want  string
	}{
		{"string id", "task", "abc", "type::record('task', 'abc')"},
		{"int id", "post", 42, "type::record('post', 42)"},
		{"RecordIDValue int", "user", IntID(7), "type::record('user', 7)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TypeThing(tc.table, tc.id).ToSurql()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTypeRecord_Composes(t *testing.T) {
	// TypeRecord must compose as a raw value in operator renderings.
	fn := TypeRecord("task", "abc")
	if fn.String() != fn.ToSurql() {
		t.Errorf("String=%q ToSurql=%q", fn.String(), fn.ToSurql())
	}
	// Using TypeRecord as a value in an Eq operator should emit raw.
	eq := EqOp("task_id", fn).ToSurql()
	want := "task_id = type::record('task', 'abc')"
	if eq != want {
		t.Errorf("got %q, want %q", eq, want)
	}
}

func TestTypeRecord_Stringer(t *testing.T) {
	// Any fmt.Stringer-implementing id falls through to string quoting.
	rec, err := NewStringRecordID("task", "abc")
	if err != nil {
		t.Fatal(err)
	}
	got := TypeRecord("wrapper", rec).ToSurql()
	// RecordID.String() renders "task:abc"; the quote-escaper does not
	// special-case colons, so the full string is quoted as an id.
	want := "type::record('wrapper', 'task:abc')"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
