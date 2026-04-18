package types

import (
	"encoding/json"
	"testing"
)

func TestRecordRef_RendersString(t *testing.T) {
	got := StringRecordRef("user", "alice").ToSurql()
	if got != "type::record('user', 'alice')" {
		t.Errorf("got %q", got)
	}
}

func TestRecordRef_RendersInt(t *testing.T) {
	got := IntRecordRef("post", 123).ToSurql()
	if got != "type::record('post', 123)" {
		t.Errorf("got %q", got)
	}
}

func TestRecordRef_EscapesSingleQuote(t *testing.T) {
	got := StringRecordRef("user", "o'brien").ToSurql()
	want := `type::record('user', 'o\'brien')`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRecordRef_EscapesBackslash(t *testing.T) {
	got := StringRecordRef("path", `a\b`).ToSurql()
	want := `type::record('path', 'a\\b')`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRecordRef_StringMatchesToSurql(t *testing.T) {
	r := StringRecordRef("user", "alice")
	if r.String() != r.ToSurql() {
		t.Errorf("String=%q ToSurql=%q", r.String(), r.ToSurql())
	}
}

func TestRecordRef_JSONRoundtrip(t *testing.T) {
	r := StringRecordRef("user", "alice")
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back RecordRef
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ToSurql() != r.ToSurql() {
		t.Errorf("roundtrip: got %q vs %q", back.ToSurql(), r.ToSurql())
	}
}

func TestRecordRef_JSONRoundtripInt(t *testing.T) {
	r := IntRecordRef("post", 42)
	data, _ := json.Marshal(r)
	var back RecordRef
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !back.RecordID.IsInt() || back.RecordID.Int() != 42 {
		t.Errorf("lost int id: %+v", back.RecordID)
	}
}
