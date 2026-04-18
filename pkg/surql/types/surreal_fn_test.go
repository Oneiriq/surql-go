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
