package types

import (
	"fmt"
	"strings"
)

// RecordRef references a SurrealDB record via `type::record(table, id)`.
// When emitted in a query body the expression renders verbatim rather
// than being quoted as a string literal.
type RecordRef struct {
	Table    string        `json:"table"`
	RecordID RecordIDValue `json:"record_id"`
}

// ToSurql renders as a `type::record()` SurrealQL call.
func (r RecordRef) ToSurql() string {
	if r.RecordID.IsInt() {
		return fmt.Sprintf("type::record('%s', %d)", r.Table, r.RecordID.Int())
	}
	escaped := strings.ReplaceAll(r.RecordID.String(), `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return fmt.Sprintf("type::record('%s', '%s')", r.Table, escaped)
}

// String implements fmt.Stringer.
func (r RecordRef) String() string { return r.ToSurql() }

// NewRecordRef constructs a RecordRef.
func NewRecordRef(table string, id RecordIDValue) RecordRef {
	return RecordRef{Table: table, RecordID: id}
}

// StringRecordRef is a convenience for string-valued record refs.
func StringRecordRef(table, id string) RecordRef {
	return RecordRef{Table: table, RecordID: StringID(id)}
}

// IntRecordRef is a convenience for integer-valued record refs.
func IntRecordRef(table string, id int64) RecordRef {
	return RecordRef{Table: table, RecordID: IntID(id)}
}
