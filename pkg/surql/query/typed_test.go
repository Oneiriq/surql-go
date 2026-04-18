package query

import "testing"

type sampleUser struct {
	Name  string `json:"name"`
	Age   int    `json:"age"`
	Email string `json:"email,omitempty"`
}

func TestToJSONMap_Struct(t *testing.T) {
	u := sampleUser{Name: "Alice", Age: 30, Email: "alice@example.com"}
	m, err := toJSONMap(u)
	if err != nil {
		t.Fatal(err)
	}
	if m["name"] != "Alice" || m["email"] != "alice@example.com" {
		t.Errorf("got %+v", m)
	}
	// encoding/json decodes numeric fields as float64.
	if v, ok := m["age"].(float64); !ok || v != 30 {
		t.Errorf("expected age=30, got %v (%T)", m["age"], m["age"])
	}
}

func TestToJSONMap_StructOmitEmpty(t *testing.T) {
	u := sampleUser{Name: "Alice", Age: 30}
	m, err := toJSONMap(u)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["email"]; ok {
		t.Error("omitempty should elide email")
	}
}

func TestToJSONMap_Map(t *testing.T) {
	in := map[string]any{"name": "Alice"}
	m, err := toJSONMap(in)
	if err != nil {
		t.Fatal(err)
	}
	// Same map instance returned (documented short-circuit).
	if m["name"] != "Alice" {
		t.Errorf("got %+v", m)
	}
}

func TestToJSONMap_NilMap(t *testing.T) {
	var in map[string]any
	if _, err := toJSONMap(in); err == nil {
		t.Fatal("expected error for nil map")
	}
}

func TestToJSONMap_NilPointer(t *testing.T) {
	var p *sampleUser
	if _, err := toJSONMap(p); err == nil {
		t.Fatal("expected error for nil pointer (encodes as null)")
	}
}

func TestToJSONMap_NonObject(t *testing.T) {
	// Numbers / slices do not unmarshal into map[string]any.
	if _, err := toJSONMap(42); err == nil {
		t.Fatal("expected error for non-object payload")
	}
}

func TestDecodeRecord_Typed(t *testing.T) {
	record := map[string]any{"name": "Bob", "age": float64(25)}
	got, err := decodeRecord[sampleUser](record)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Bob" || got.Age != 25 {
		t.Errorf("got %+v", got)
	}
}
