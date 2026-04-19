package types

import (
	"fmt"
	"strings"
)

// SurrealFn wraps a raw SurrealQL function expression so that, when used as
// a value in CREATE/UPDATE/UPSERT, it is emitted verbatim (not quoted as a
// string literal).
type SurrealFn struct {
	Expression string `json:"expression"`
}

// ToSurql renders the function call as raw SurrealQL.
func (f SurrealFn) ToSurql() string {
	return f.Expression
}

// String implements fmt.Stringer.
func (f SurrealFn) String() string {
	return f.Expression
}

// NewSurrealFn constructs a SurrealFn from a rendered expression.
func NewSurrealFn(expression string) SurrealFn {
	return SurrealFn{Expression: expression}
}

// SurqlFn builds a SurrealDB function call value. args are rendered verbatim
// (via fmt.Sprint) and joined with ", ". When args is empty the call uses
// empty parentheses.
func SurqlFn(name string, args ...any) SurrealFn {
	if len(args) == 0 {
		return SurrealFn{Expression: name + "()"}
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprint(a)
	}
	return SurrealFn{Expression: fmt.Sprintf("%s(%s)", name, strings.Join(parts, ", "))}
}

// TypeRecord returns a SurrealFn wrapping `type::record(table, id)`. When
// used as the target of a CRUD helper or as a value in SET / WHERE it
// renders as raw SurrealQL and evaluates to the record id at query time.
//
// id may be a string, any integer kind, or a RecordIDValue. Strings are
// single-quoted (with backslashes and single quotes escaped) to match the
// surql-py renderer.
func TypeRecord(table string, id any) SurrealFn {
	return SurrealFn{Expression: renderTypeRecord("record", table, id)}
}

// TypeThing is an alias for TypeRecord provided for API parity with
// downstream code that prefers the `thing` nomenclature.
//
// SurrealDB v3+ removed `type::thing` in favour of `type::record`; both
// helpers therefore emit `type::record(...)` under the hood. The pre-v3
// spelling would fail parsing with "Invalid function/constant path, did
// you maybe mean `type::record`".
func TypeThing(table string, id any) SurrealFn {
	return SurrealFn{Expression: renderTypeRecord("record", table, id)}
}

// renderTypeRecord shares the rendering path between TypeRecord and
// TypeThing; the only difference is the function name.
func renderTypeRecord(kind, table string, id any) string {
	switch v := id.(type) {
	case RecordIDValue:
		if v.IsInt() {
			return fmt.Sprintf("type::%s('%s', %d)", kind, table, v.Int())
		}
		return fmt.Sprintf("type::%s('%s', %s)", kind, table, quoteTypeRecordString(v.String()))
	case string:
		return fmt.Sprintf("type::%s('%s', %s)", kind, table, quoteTypeRecordString(v))
	case int:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case int8:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case int16:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case int32:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case int64:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case uint:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case uint8:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case uint16:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case uint32:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case uint64:
		return fmt.Sprintf("type::%s('%s', %d)", kind, table, v)
	case fmt.Stringer:
		return fmt.Sprintf("type::%s('%s', %s)", kind, table, quoteTypeRecordString(v.String()))
	default:
		return fmt.Sprintf("type::%s('%s', %s)", kind, table, quoteTypeRecordString(fmt.Sprint(v)))
	}
}

func quoteTypeRecordString(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}
