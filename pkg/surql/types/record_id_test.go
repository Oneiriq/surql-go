package types

import (
	"encoding/json"
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestStringID_RendersSimple(t *testing.T) {
	id, err := NewStringRecordID("user", "alice")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id.String() != "user:alice" {
		t.Errorf("got %q", id.String())
	}
}

func TestComplexID_UsesAngleBrackets(t *testing.T) {
	id, err := NewStringRecordID("outlet", "alaskabeacon.com")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id.String() != "outlet:<alaskabeacon.com>" {
		t.Errorf("got %q", id.String())
	}
}

func TestIntID_NeverBrackets(t *testing.T) {
	id, err := NewIntRecordID("post", 123)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id.String() != "post:123" {
		t.Errorf("got %q", id.String())
	}
}

func TestParse_SimpleString(t *testing.T) {
	id, err := ParseRecordID("user:alice")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if id.Table() != "user" || id.ID().String() != "alice" {
		t.Errorf("got table=%q id=%q", id.Table(), id.ID().String())
	}
}

func TestParse_IntegerID(t *testing.T) {
	id, err := ParseRecordID("post:42")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !id.ID().IsInt() || id.ID().Int() != 42 {
		t.Errorf("expected int 42, got %+v", id.ID())
	}
}

func TestParse_AngleBrackets(t *testing.T) {
	id, err := ParseRecordID("outlet:<alaskabeacon.com>")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if id.ID().IsInt() {
		t.Error("expected string id, got int")
	}
	if id.ID().String() != "alaskabeacon.com" {
		t.Errorf("got %q", id.ID().String())
	}
}

func TestParse_Rejects(t *testing.T) {
	cases := []string{"", "user", ":id", "user:"}
	for _, c := range cases {
		if _, err := ParseRecordID(c); err == nil {
			t.Errorf("expected error for %q", c)
		} else if !errors.Is(err, surqlerrors.ErrValidation) {
			t.Errorf("expected validation error for %q, got %v", c, err)
		}
	}
}

func TestNew_RejectsInvalidTable(t *testing.T) {
	cases := []string{"", "user-name", "1user", "user name"}
	for _, c := range cases {
		if _, err := NewStringRecordID(c, "alice"); err == nil {
			t.Errorf("expected error for table %q", c)
		}
	}
}

func TestJSON_StringRoundtrip(t *testing.T) {
	id, _ := NewStringRecordID("user", "alice")
	data, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `"user:alice"` {
		t.Errorf("got %s", data)
	}
	var back RecordID
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.String() != id.String() {
		t.Errorf("roundtrip mismatch: %q vs %q", back.String(), id.String())
	}
}

func TestJSON_ComplexRoundtrip(t *testing.T) {
	id, _ := NewStringRecordID("outlet", "alaskabeacon.com")
	data, _ := json.Marshal(id)
	// json.Marshal HTML-escapes `<`/`>` by default; both the escaped and
	// literal forms must decode identically.
	want1 := `"outlet:<alaskabeacon.com>"`
	want2 := `"outlet:\u003calaskabeacon.com\u003e"`
	if s := string(data); s != want1 && s != want2 {
		t.Errorf("got %s (expected %s or %s)", s, want1, want2)
	}
	var back RecordID
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ID().String() != "alaskabeacon.com" {
		t.Errorf("got %q", back.ID().String())
	}
	// Literal form also roundtrips.
	literal, err := id.marshalJSONNoHTMLEscape()
	if err != nil {
		t.Fatalf("no-html marshal: %v", err)
	}
	if string(literal) != want1 {
		t.Errorf("literal form: got %s, want %s", literal, want1)
	}
	var back2 RecordID
	if err := json.Unmarshal(literal, &back2); err != nil {
		t.Fatalf("unmarshal literal: %v", err)
	}
	if back2.ID().String() != "alaskabeacon.com" {
		t.Errorf("literal roundtrip: got %q", back2.ID().String())
	}
}

func TestJSON_IntRoundtrip(t *testing.T) {
	id, _ := NewIntRecordID("post", 123)
	data, _ := json.Marshal(id)
	if string(data) != `"post:123"` {
		t.Errorf("got %s", data)
	}
	var back RecordID
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !back.ID().IsInt() || back.ID().Int() != 123 {
		t.Errorf("lost int type: %+v", back.ID())
	}
}
