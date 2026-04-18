package schema

import (
	stdErrors "errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestFieldType_IsValid(t *testing.T) {
	cases := []struct {
		in   FieldType
		want bool
	}{
		{FieldTypeString, true},
		{FieldTypeInt, true},
		{FieldTypeFloat, true},
		{FieldTypeBool, true},
		{FieldTypeDatetime, true},
		{FieldTypeDuration, true},
		{FieldTypeDecimal, true},
		{FieldTypeNumber, true},
		{FieldTypeObject, true},
		{FieldTypeArray, true},
		{FieldTypeRecord, true},
		{FieldTypeGeometry, true},
		{FieldTypeAny, true},
		{FieldType("bogus"), false},
		{FieldType(""), false},
	}
	for _, tc := range cases {
		if got := tc.in.IsValid(); got != tc.want {
			t.Errorf("FieldType(%q).IsValid() = %v, want %v", string(tc.in), got, tc.want)
		}
	}
}

func TestFieldType_StringValues(t *testing.T) {
	pairs := map[FieldType]string{
		FieldTypeString:   "string",
		FieldTypeInt:      "int",
		FieldTypeFloat:    "float",
		FieldTypeBool:     "bool",
		FieldTypeDatetime: "datetime",
		FieldTypeDuration: "duration",
		FieldTypeDecimal:  "decimal",
		FieldTypeNumber:   "number",
		FieldTypeObject:   "object",
		FieldTypeArray:    "array",
		FieldTypeRecord:   "record",
		FieldTypeGeometry: "geometry",
		FieldTypeAny:      "any",
	}
	for ft, want := range pairs {
		if ft.String() != want {
			t.Errorf("FieldType(%q).String() = %q, want %q", string(ft), ft.String(), want)
		}
	}
}

func TestNewField_Minimal(t *testing.T) {
	f := NewField("name", FieldTypeString)
	if f.Name != "name" || f.Type != FieldTypeString {
		t.Fatalf("unexpected field: %+v", f)
	}
	if f.Assertion != "" || f.Default != "" || f.Value != "" || f.ReadOnly || f.Flexible {
		t.Fatalf("unexpected non-zero defaults: %+v", f)
	}
}

func TestNewField_WithAllOptions(t *testing.T) {
	f := NewField("age", FieldTypeInt,
		WithAssertion("$value >= 0"),
		WithDefault("0"),
		WithReadOnly(true),
		WithFieldPermissions(map[string]string{"select": "$auth.id = id"}),
	)
	if f.Assertion != "$value >= 0" {
		t.Errorf("assertion = %q", f.Assertion)
	}
	if f.Default != "0" {
		t.Errorf("default = %q", f.Default)
	}
	if !f.ReadOnly {
		t.Errorf("readonly = false")
	}
	if v := f.Permissions["select"]; v != "$auth.id = id" {
		t.Errorf("permissions[select] = %q", v)
	}
}

func TestFieldToSurql_Basic(t *testing.T) {
	f := StringField("name")
	got := f.ToSurql("user")
	want := "DEFINE FIELD name ON TABLE user TYPE string;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestFieldToSurql_WithAssertion(t *testing.T) {
	f := StringField("email", WithAssertion("string::is::email($value)"))
	got := f.ToSurql("user")
	want := "DEFINE FIELD email ON TABLE user TYPE string ASSERT string::is::email($value);"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestFieldToSurql_WithDefault(t *testing.T) {
	f := DatetimeField("created_at", WithDefault("time::now()"))
	got := f.ToSurql("event")
	want := "DEFINE FIELD created_at ON TABLE event TYPE datetime DEFAULT time::now();"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestFieldToSurql_WithValue(t *testing.T) {
	f := ComputedField("full_name", "string::concat(first, ' ', last)", FieldTypeString)
	got := f.ToSurql("user")
	if !strings.Contains(got, "VALUE string::concat(first, ' ', last)") {
		t.Errorf("ToSurql() missing VALUE clause: %q", got)
	}
	if !strings.Contains(got, "READONLY") {
		t.Errorf("computed field should include READONLY: %q", got)
	}
}

func TestFieldToSurql_Readonly(t *testing.T) {
	f := DatetimeField("created_at", WithDefault("time::now()"), WithReadOnly(true))
	got := f.ToSurql("event")
	want := "DEFINE FIELD created_at ON TABLE event TYPE datetime DEFAULT time::now() READONLY;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestFieldToSurql_ObjectFlexible(t *testing.T) {
	f := ObjectField("metadata")
	if !f.Flexible {
		t.Fatalf("ObjectField should default flexible=true")
	}
	got := f.ToSurql("user")
	if !strings.Contains(got, "FLEXIBLE") {
		t.Errorf("ObjectField ToSurql should include FLEXIBLE: %q", got)
	}
	if !strings.Contains(got, "TYPE object") {
		t.Errorf("ObjectField ToSurql should include TYPE object: %q", got)
	}
}

func TestFieldToSurql_ObjectFlexibleOptOut(t *testing.T) {
	f := ObjectField("metadata", WithFlexible(false))
	if f.Flexible {
		t.Fatalf("ObjectField(WithFlexible(false)) should disable flexible")
	}
	got := f.ToSurql("user")
	if strings.Contains(got, "FLEXIBLE") {
		t.Errorf("ToSurql should not include FLEXIBLE: %q", got)
	}
}

func TestFieldToSurql_AllClauses(t *testing.T) {
	f := NewField("status", FieldTypeString,
		WithAssertion("$value IN ['a','b']"),
		WithDefault("'a'"),
		WithReadOnly(true),
		WithFlexible(true),
	)
	got := f.ToSurql("t")
	want := "DEFINE FIELD status ON TABLE t TYPE string ASSERT $value IN ['a','b'] DEFAULT 'a' READONLY FLEXIBLE;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestFieldToSurqlIfNotExists(t *testing.T) {
	f := StringField("name")
	got := f.ToSurqlIfNotExists("user")
	want := "DEFINE FIELD IF NOT EXISTS name ON TABLE user TYPE string;"
	if got != want {
		t.Errorf("ToSurqlIfNotExists() = %q, want %q", got, want)
	}
}

func TestFieldValidate_EmptyName(t *testing.T) {
	f := NewField("", FieldTypeString)
	err := f.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil for empty name")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("err = %v, want ErrValidation chain", err)
	}
}

func TestFieldValidate_InvalidType(t *testing.T) {
	f := NewField("x", FieldType("bogus"))
	err := f.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil for invalid type")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("err = %v, want ErrValidation chain", err)
	}
}

func TestFieldValidate_SimpleNameValid(t *testing.T) {
	for _, name := range []string{"name", "_field", "n1", "foo_bar"} {
		f := NewField(name, FieldTypeString)
		if err := f.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", name, err)
		}
	}
}

func TestFieldValidate_DotNotationValid(t *testing.T) {
	for _, name := range []string{"address.city", "a.b.c", "user_1.email"} {
		f := NewField(name, FieldTypeString)
		if err := f.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", name, err)
		}
	}
}

func TestFieldValidate_InvalidNames(t *testing.T) {
	cases := []string{
		"1digit",
		"name-hyphen",
		"name space",
		".leading",
		"trailing.",
		"empty..segment",
		"1digit.b",
	}
	for _, name := range cases {
		f := NewField(name, FieldTypeString)
		err := f.Validate()
		if err == nil {
			t.Errorf("Validate(%q) = nil, want error", name)
			continue
		}
		if !stdErrors.Is(err, surqlerrors.ErrValidation) {
			t.Errorf("Validate(%q) err = %v, want ErrValidation chain", name, err)
		}
	}
}

func TestRecordField_WithTargetTable(t *testing.T) {
	f := RecordField("author", "user")
	if f.Type != FieldTypeRecord {
		t.Fatalf("type = %q", string(f.Type))
	}
	if f.Assertion != `$value.table = "user"` {
		t.Errorf("assertion = %q", f.Assertion)
	}
}

func TestRecordField_WithCustomAssertion(t *testing.T) {
	f := RecordField("author", "user", WithAssertion("$value.active = true"))
	if !strings.Contains(f.Assertion, `$value.table = "user"`) {
		t.Errorf("assertion missing table clause: %q", f.Assertion)
	}
	if !strings.Contains(f.Assertion, "$value.active = true") {
		t.Errorf("assertion missing custom clause: %q", f.Assertion)
	}
}

func TestRecordField_WithoutTable(t *testing.T) {
	f := RecordField("author", "")
	if f.Assertion != "" {
		t.Errorf("assertion = %q, want empty", f.Assertion)
	}
}

func TestReservedWarning(t *testing.T) {
	f := StringField("in")
	if f.ReservedWarning(false) == "" {
		t.Errorf("expected warning for reserved word 'in'")
	}
	if f.ReservedWarning(true) != "" {
		t.Errorf("expected no warning for 'in' with allowEdgeFields=true")
	}
}

func TestConvenienceBuilders(t *testing.T) {
	if StringField("a").Type != FieldTypeString {
		t.Error("StringField type mismatch")
	}
	if IntField("a").Type != FieldTypeInt {
		t.Error("IntField type mismatch")
	}
	if FloatField("a").Type != FieldTypeFloat {
		t.Error("FloatField type mismatch")
	}
	if BoolField("a").Type != FieldTypeBool {
		t.Error("BoolField type mismatch")
	}
	if DatetimeField("a").Type != FieldTypeDatetime {
		t.Error("DatetimeField type mismatch")
	}
	if ArrayField("a").Type != FieldTypeArray {
		t.Error("ArrayField type mismatch")
	}
	if ObjectField("a").Type != FieldTypeObject {
		t.Error("ObjectField type mismatch")
	}
}

func TestFieldPermissions_MapCopyIsolation(t *testing.T) {
	perms := map[string]string{"select": "$auth.id = id"}
	f := NewField("x", FieldTypeString, WithFieldPermissions(perms))
	perms["select"] = "MUTATED"
	if f.Permissions["select"] != "$auth.id = id" {
		t.Errorf("permissions should be copied; got %q", f.Permissions["select"])
	}
}
