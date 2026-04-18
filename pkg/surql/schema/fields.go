package schema

import (
	"fmt"
	"regexp"
	"strings"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
	"github.com/albedosehen/surql-go/pkg/surql/types"
)

// FieldType enumerates the SurrealDB field types supported in DEFINE FIELD.
type FieldType string

// FieldType values mirrored from surql-py FieldType enum.
const (
	FieldTypeString   FieldType = "string"
	FieldTypeInt      FieldType = "int"
	FieldTypeFloat    FieldType = "float"
	FieldTypeBool     FieldType = "bool"
	FieldTypeDatetime FieldType = "datetime"
	FieldTypeDuration FieldType = "duration"
	FieldTypeDecimal  FieldType = "decimal"
	FieldTypeNumber   FieldType = "number"
	FieldTypeObject   FieldType = "object"
	FieldTypeArray    FieldType = "array"
	FieldTypeRecord   FieldType = "record"
	FieldTypeGeometry FieldType = "geometry"
	FieldTypeAny      FieldType = "any"
)

// IsValid reports whether the receiver is one of the defined FieldType constants.
func (f FieldType) IsValid() bool {
	switch f {
	case FieldTypeString, FieldTypeInt, FieldTypeFloat, FieldTypeBool,
		FieldTypeDatetime, FieldTypeDuration, FieldTypeDecimal, FieldTypeNumber,
		FieldTypeObject, FieldTypeArray, FieldTypeRecord, FieldTypeGeometry,
		FieldTypeAny:
		return true
	}
	return false
}

// String returns the SurrealQL-ready string form of the type.
func (f FieldType) String() string { return string(f) }

var fieldNamePartRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// FieldDefinition is an immutable description of a single column in a DEFINE
// FIELD statement.
type FieldDefinition struct {
	Name        string
	Type        FieldType
	Assertion   string
	Default     string
	Value       string // computed fields
	Permissions map[string]string
	ReadOnly    bool
	Flexible    bool
}

// FieldOption customises a FieldDefinition created via NewField.
type FieldOption func(*FieldDefinition)

// WithAssertion attaches a SurrealQL ASSERT clause.
func WithAssertion(expr string) FieldOption {
	return func(f *FieldDefinition) { f.Assertion = expr }
}

// WithDefault attaches a DEFAULT expression.
func WithDefault(expr string) FieldOption {
	return func(f *FieldDefinition) { f.Default = expr }
}

// WithValue attaches a VALUE expression (computed field).
func WithValue(expr string) FieldOption {
	return func(f *FieldDefinition) { f.Value = expr }
}

// WithFieldPermissions attaches a PERMISSIONS map keyed by action.
func WithFieldPermissions(perms map[string]string) FieldOption {
	return func(f *FieldDefinition) {
		if perms == nil {
			f.Permissions = nil
			return
		}
		copied := make(map[string]string, len(perms))
		for k, v := range perms {
			copied[k] = v
		}
		f.Permissions = copied
	}
}

// WithReadOnly marks the field READONLY.
func WithReadOnly(readonly bool) FieldOption {
	return func(f *FieldDefinition) { f.ReadOnly = readonly }
}

// WithFlexible marks the field FLEXIBLE (used for object fields).
func WithFlexible(flex bool) FieldOption {
	return func(f *FieldDefinition) { f.Flexible = flex }
}

// NewField constructs a FieldDefinition. It does not fail; use Validate to
// surface name/type problems before emitting SurrealQL.
func NewField(name string, ft FieldType, opts ...FieldOption) FieldDefinition {
	fd := FieldDefinition{Name: name, Type: ft}
	for _, opt := range opts {
		opt(&fd)
	}
	return fd
}

// Validate checks the field name (alphanumeric / underscore, dot-notation
// allowed) and that the field type is recognised.
func (f FieldDefinition) Validate() error {
	if err := validateFieldName(f.Name); err != nil {
		return err
	}
	if !f.Type.IsValid() {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"invalid field type %q for field %q", string(f.Type), f.Name)
	}
	return nil
}

// ReservedWarning returns a human-readable warning when the field name
// collides with a SurrealDB reserved word, or an empty string.
//
// allowEdgeFields should be true when the field belongs to an edge table
// (permitting "in" and "out" identifiers).
func (f FieldDefinition) ReservedWarning(allowEdgeFields bool) string {
	return types.CheckReservedWord(f.Name, allowEdgeFields)
}

// ToSurql renders the standalone DEFINE FIELD statement for this field on
// the given table. It does not validate; call Validate separately.
func (f FieldDefinition) ToSurql(tableName string) string {
	return f.toSurql(tableName, false)
}

// ToSurqlIfNotExists renders the DEFINE FIELD statement with IF NOT EXISTS.
func (f FieldDefinition) ToSurqlIfNotExists(tableName string) string {
	return f.toSurql(tableName, true)
}

func (f FieldDefinition) toSurql(tableName string, ifNotExists bool) string {
	var b strings.Builder
	b.WriteString("DEFINE FIELD")
	if ifNotExists {
		b.WriteString(" IF NOT EXISTS")
	}
	b.WriteString(" ")
	b.WriteString(f.Name)
	b.WriteString(" ON TABLE ")
	b.WriteString(tableName)
	b.WriteString(" TYPE ")
	b.WriteString(string(f.Type))

	if f.Assertion != "" {
		b.WriteString(" ASSERT ")
		b.WriteString(f.Assertion)
	}
	if f.Default != "" {
		b.WriteString(" DEFAULT ")
		b.WriteString(f.Default)
	}
	if f.Value != "" {
		b.WriteString(" VALUE ")
		b.WriteString(f.Value)
	}
	if f.ReadOnly {
		b.WriteString(" READONLY")
	}
	if f.Flexible {
		b.WriteString(" FLEXIBLE")
	}
	b.WriteString(";")
	return b.String()
}

// validateFieldName mirrors the Python _validate_field_name helper, supporting
// dot-notation for nested fields.
func validateFieldName(name string) error {
	if name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "field name cannot be empty")
	}
	for _, part := range strings.Split(name, ".") {
		if part == "" {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"invalid field name %q: empty segment", name)
		}
		if !fieldNamePartRe.MatchString(part) {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"invalid field name %q: segment %q must contain only alphanumeric characters and underscores, and cannot start with a digit",
				name, part)
		}
	}
	return nil
}

// Convenience constructors that mirror surql-py helper functions.

// StringField builds a string-typed FieldDefinition.
func StringField(name string, opts ...FieldOption) FieldDefinition {
	return NewField(name, FieldTypeString, opts...)
}

// IntField builds an int-typed FieldDefinition.
func IntField(name string, opts ...FieldOption) FieldDefinition {
	return NewField(name, FieldTypeInt, opts...)
}

// FloatField builds a float-typed FieldDefinition.
func FloatField(name string, opts ...FieldOption) FieldDefinition {
	return NewField(name, FieldTypeFloat, opts...)
}

// BoolField builds a bool-typed FieldDefinition.
func BoolField(name string, opts ...FieldOption) FieldDefinition {
	return NewField(name, FieldTypeBool, opts...)
}

// DatetimeField builds a datetime-typed FieldDefinition.
func DatetimeField(name string, opts ...FieldOption) FieldDefinition {
	return NewField(name, FieldTypeDatetime, opts...)
}

// RecordField builds a record-typed FieldDefinition. When targetTable is
// non-empty, an assertion constraining $value.table is pre-populated and any
// user-provided assertion is combined with it.
func RecordField(name, targetTable string, opts ...FieldOption) FieldDefinition {
	fd := NewField(name, FieldTypeRecord, opts...)
	if targetTable == "" {
		return fd
	}
	tableAssert := fmt.Sprintf("$value.table = %q", targetTable)
	if fd.Assertion == "" {
		fd.Assertion = tableAssert
	} else {
		fd.Assertion = "(" + tableAssert + ") AND (" + fd.Assertion + ")"
	}
	return fd
}

// ArrayField builds an array-typed FieldDefinition.
func ArrayField(name string, opts ...FieldOption) FieldDefinition {
	return NewField(name, FieldTypeArray, opts...)
}

// ObjectField builds an object-typed FieldDefinition, defaulting the
// FLEXIBLE flag to true (matching the Python helper). Pass WithFlexible(false)
// to opt out.
func ObjectField(name string, opts ...FieldOption) FieldDefinition {
	fd := FieldDefinition{Name: name, Type: FieldTypeObject, Flexible: true}
	for _, opt := range opts {
		opt(&fd)
	}
	return fd
}

// ComputedField builds a computed field (VALUE expression, READONLY).
func ComputedField(name, valueExpr string, ft FieldType, opts ...FieldOption) FieldDefinition {
	fd := NewField(name, ft, opts...)
	fd.Value = valueExpr
	fd.ReadOnly = true
	return fd
}
