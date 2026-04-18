package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// tablePattern matches valid SurrealDB table names: [A-Za-z_][A-Za-z0-9_]*
var tablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// simpleIDPattern matches an id that does NOT require angle brackets.
var simpleIDPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// RecordIDValue holds either a string or integer record id. Integer ids
// never require angle-bracket syntax in SurrealDB.
type RecordIDValue struct {
	str     string
	intVal  int64
	isInt   bool
	present bool
}

// StringID constructs a string-valued record id.
func StringID(s string) RecordIDValue {
	return RecordIDValue{str: s, present: true}
}

// IntID constructs an integer-valued record id.
func IntID(n int64) RecordIDValue {
	return RecordIDValue{intVal: n, isInt: true, present: true}
}

// IsInt reports whether the id is integer-valued.
func (v RecordIDValue) IsInt() bool { return v.isInt }

// String returns the id as a string for rendering.
func (v RecordIDValue) String() string {
	if v.isInt {
		return strconv.FormatInt(v.intVal, 10)
	}
	return v.str
}

// Int returns the underlying integer id (or 0 when the id is a string).
func (v RecordIDValue) Int() int64 { return v.intVal }

// MarshalJSON encodes the id as its underlying JSON primitive.
func (v RecordIDValue) MarshalJSON() ([]byte, error) {
	if v.isInt {
		return json.Marshal(v.intVal)
	}
	return json.Marshal(v.str)
}

// UnmarshalJSON accepts either a JSON string or integer.
func (v *RecordIDValue) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*v = StringID(s)
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*v = IntID(n)
	return nil
}

// RecordID is a type-safe wrapper for SurrealDB record ids (`table:id`).
type RecordID struct {
	table string
	id    RecordIDValue
}

// NewRecordID builds a RecordID after validating the table name.
func NewRecordID(table string, id RecordIDValue) (RecordID, error) {
	if err := validateTable(table); err != nil {
		return RecordID{}, err
	}
	if !id.present {
		return RecordID{}, surqlerrors.New(surqlerrors.ErrValidation, "record id cannot be empty")
	}
	return RecordID{table: table, id: id}, nil
}

// NewStringRecordID is a convenience constructor for string-valued ids.
func NewStringRecordID(table, id string) (RecordID, error) {
	return NewRecordID(table, StringID(id))
}

// NewIntRecordID is a convenience constructor for integer-valued ids.
func NewIntRecordID(table string, id int64) (RecordID, error) {
	return NewRecordID(table, IntID(id))
}

func validateTable(table string) error {
	if table == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "Table name cannot be empty")
	}
	if !tablePattern.MatchString(table) {
		return surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"Invalid table name: %q. Must contain only alphanumeric characters and underscores, and cannot start with a digit",
			table,
		)
	}
	return nil
}

// Table returns the table name.
func (r RecordID) Table() string { return r.table }

// ID returns the record id value.
func (r RecordID) ID() RecordIDValue { return r.id }

// String renders as `table:id` or `table:<id>` when the id needs angle brackets.
func (r RecordID) String() string {
	if r.needsAngleBrackets() {
		return fmt.Sprintf("%s:<%s>", r.table, r.id.String())
	}
	return fmt.Sprintf("%s:%s", r.table, r.id.String())
}

// ToSurql renders as the SurrealQL record id literal.
func (r RecordID) ToSurql() string { return r.String() }

func (r RecordID) needsAngleBrackets() bool {
	if r.id.isInt {
		return false
	}
	return !simpleIDPattern.MatchString(r.id.str)
}

// ParseRecordID parses `table:id` (or `table:<id>`). Integer-looking ids
// become IntID; everything else stays a StringID. Angle brackets, when
// present, are stripped on parse.
func ParseRecordID(input string) (RecordID, error) {
	idx := strings.Index(input, ":")
	if idx < 0 {
		return RecordID{}, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"Invalid record ID format: %s. Expected format: table:id",
			input,
		)
	}
	table := strings.TrimSpace(input[:idx])
	idStr := strings.TrimSpace(input[idx+1:])
	if table == "" {
		return RecordID{}, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"Invalid record ID: table name cannot be empty in %q", input,
		)
	}
	if idStr == "" {
		return RecordID{}, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"Invalid record ID: id cannot be empty in %q", input,
		)
	}
	if len(idStr) >= 2 && idStr[0] == '<' && idStr[len(idStr)-1] == '>' {
		idStr = idStr[1 : len(idStr)-1]
	}
	if n, err := strconv.ParseInt(idStr, 10, 64); err == nil {
		return NewIntRecordID(table, n)
	}
	return NewStringRecordID(table, idStr)
}

// MarshalJSON encodes as a plain string (`table:id` / `table:<id>`).
//
// Note: callers that pass the result through encoding/json at the top
// level will receive Go's default HTML-escaped angle brackets (`\u003c` /
// `\u003e`). To preserve literal `<>` for wire parity with surql-py and
// surql-ts, use [encoding/json.Encoder] with SetEscapeHTML(false).
// Both forms decode back into the same RecordID.
func (r RecordID) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

// marshalJSONNoHTMLEscape is kept as a reference for callers who need the
// unescaped wire form.
func (r RecordID) marshalJSONNoHTMLEscape() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r.String()); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return out, nil
}

// UnmarshalJSON accepts a string in `table:id` / `table:<id>` form.
func (r *RecordID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseRecordID(s)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}
