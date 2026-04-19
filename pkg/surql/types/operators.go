package types

import (
	"fmt"
	"strconv"
	"strings"
)

// Operator is any expression that renders to SurrealQL.
type Operator interface {
	ToSurql() string
}

// RawSurqlValue is an opt-in marker that causes [quoteValue] to emit
// `ToSurql()` verbatim instead of quoting the value. Any type from
// outside this package can participate in raw-value rendering by
// implementing the single [IsRawSurqlValue] marker method alongside
// `ToSurql() string`.
//
// The marker guard stops arbitrary [fmt.Stringer] implementations from
// accidentally opting in — a type must intentionally declare raw-render
// semantics by implementing both methods.
type RawSurqlValue interface {
	ToSurql() string
	IsRawSurqlValue()
}

// quoteValue renders v as a SurrealQL literal.
//
// Rules (matching the Python port):
//   - nil               -> NULL
//   - bool              -> true / false
//   - int/uint/float    -> base-10 formatted number
//   - string            -> 'single-quoted' with \ and ' escaped
//   - SurrealFn         -> raw expression
//   - RecordRef         -> type::record(...) raw expression
//   - RawSurqlValue     -> raw expression (opt-in via marker interface)
//   - []any             -> '[' + quoted elements + ']'
//   - fmt.Stringer      -> quoted string form
func quoteValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", x)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", x)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return quoteString(x)
	case SurrealFn:
		return x.ToSurql()
	case RecordRef:
		return x.ToSurql()
	case RawSurqlValue:
		return x.ToSurql()
	case []any:
		parts := make([]string, len(x))
		for i, el := range x {
			parts[i] = quoteValue(el)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case fmt.Stringer:
		return quoteString(x.String())
	default:
		return quoteString(fmt.Sprint(x))
	}
}

func quoteString(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}

// --- Comparison operators ---------------------------------------------------

// Eq renders as `field = value`.
type Eq struct {
	Field string
	Value any
}

// ToSurql implements Operator.
func (o Eq) ToSurql() string { return fmt.Sprintf("%s = %s", o.Field, quoteValue(o.Value)) }

// Ne renders as `field != value`.
type Ne struct {
	Field string
	Value any
}

// ToSurql implements Operator.
func (o Ne) ToSurql() string { return fmt.Sprintf("%s != %s", o.Field, quoteValue(o.Value)) }

// Gt renders as `field > value`.
type Gt struct {
	Field string
	Value any
}

// ToSurql implements Operator.
func (o Gt) ToSurql() string { return fmt.Sprintf("%s > %s", o.Field, quoteValue(o.Value)) }

// Gte renders as `field >= value`.
type Gte struct {
	Field string
	Value any
}

// ToSurql implements Operator.
func (o Gte) ToSurql() string { return fmt.Sprintf("%s >= %s", o.Field, quoteValue(o.Value)) }

// Lt renders as `field < value`.
type Lt struct {
	Field string
	Value any
}

// ToSurql implements Operator.
func (o Lt) ToSurql() string { return fmt.Sprintf("%s < %s", o.Field, quoteValue(o.Value)) }

// Lte renders as `field <= value`.
type Lte struct {
	Field string
	Value any
}

// ToSurql implements Operator.
func (o Lte) ToSurql() string { return fmt.Sprintf("%s <= %s", o.Field, quoteValue(o.Value)) }

// Contains renders as `field CONTAINS value`.
type Contains struct {
	Field string
	Value any
}

// ToSurql implements Operator.
func (o Contains) ToSurql() string {
	return fmt.Sprintf("%s CONTAINS %s", o.Field, quoteValue(o.Value))
}

// ContainsNot renders as `field CONTAINSNOT value`.
type ContainsNot struct {
	Field string
	Value any
}

// ToSurql implements Operator.
func (o ContainsNot) ToSurql() string {
	return fmt.Sprintf("%s CONTAINSNOT %s", o.Field, quoteValue(o.Value))
}

// --- Array-comparison operators ---------------------------------------------

// ContainsAll renders as `field CONTAINSALL [...]`.
type ContainsAll struct {
	Field  string
	Values []any
}

// ToSurql implements Operator.
func (o ContainsAll) ToSurql() string {
	return fmt.Sprintf("%s CONTAINSALL %s", o.Field, renderList(o.Values))
}

// ContainsAny renders as `field CONTAINSANY [...]`.
type ContainsAny struct {
	Field  string
	Values []any
}

// ToSurql implements Operator.
func (o ContainsAny) ToSurql() string {
	return fmt.Sprintf("%s CONTAINSANY %s", o.Field, renderList(o.Values))
}

// Inside renders as `field INSIDE [...]`.
type Inside struct {
	Field  string
	Values []any
}

// ToSurql implements Operator.
func (o Inside) ToSurql() string {
	return fmt.Sprintf("%s INSIDE %s", o.Field, renderList(o.Values))
}

// NotInside renders as `field NOTINSIDE [...]`.
type NotInside struct {
	Field  string
	Values []any
}

// ToSurql implements Operator.
func (o NotInside) ToSurql() string {
	return fmt.Sprintf("%s NOTINSIDE %s", o.Field, renderList(o.Values))
}

func renderList(values []any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = quoteValue(v)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// --- Null checks ------------------------------------------------------------

// IsNull renders as `field IS NULL`.
type IsNull struct {
	Field string
}

// ToSurql implements Operator.
func (o IsNull) ToSurql() string { return o.Field + " IS NULL" }

// IsNotNull renders as `field IS NOT NULL`.
type IsNotNull struct {
	Field string
}

// ToSurql implements Operator.
func (o IsNotNull) ToSurql() string { return o.Field + " IS NOT NULL" }

// --- Logical operators ------------------------------------------------------

// And renders as `(left) AND (right)`.
type And struct {
	Left  Operator
	Right Operator
}

// ToSurql implements Operator.
func (o And) ToSurql() string {
	return fmt.Sprintf("(%s) AND (%s)", o.Left.ToSurql(), o.Right.ToSurql())
}

// Or renders as `(left) OR (right)`.
type Or struct {
	Left  Operator
	Right Operator
}

// ToSurql implements Operator.
func (o Or) ToSurql() string {
	return fmt.Sprintf("(%s) OR (%s)", o.Left.ToSurql(), o.Right.ToSurql())
}

// Not renders as `NOT (operand)`.
type Not struct {
	Operand Operator
}

// ToSurql implements Operator.
func (o Not) ToSurql() string { return "NOT (" + o.Operand.ToSurql() + ")" }

// --- Functional helpers (mirror the Python API) -----------------------------

// EqOp builds an Eq operator.
func EqOp(field string, value any) Eq { return Eq{Field: field, Value: value} }

// NeOp builds a Ne operator.
func NeOp(field string, value any) Ne { return Ne{Field: field, Value: value} }

// GtOp builds a Gt operator.
func GtOp(field string, value any) Gt { return Gt{Field: field, Value: value} }

// GteOp builds a Gte operator.
func GteOp(field string, value any) Gte { return Gte{Field: field, Value: value} }

// LtOp builds an Lt operator.
func LtOp(field string, value any) Lt { return Lt{Field: field, Value: value} }

// LteOp builds an Lte operator.
func LteOp(field string, value any) Lte { return Lte{Field: field, Value: value} }

// ContainsOp builds a Contains operator.
func ContainsOp(field string, value any) Contains {
	return Contains{Field: field, Value: value}
}

// ContainsNotOp builds a ContainsNot operator.
func ContainsNotOp(field string, value any) ContainsNot {
	return ContainsNot{Field: field, Value: value}
}

// ContainsAllOp builds a ContainsAll operator.
func ContainsAllOp(field string, values []any) ContainsAll {
	return ContainsAll{Field: field, Values: values}
}

// ContainsAnyOp builds a ContainsAny operator.
func ContainsAnyOp(field string, values []any) ContainsAny {
	return ContainsAny{Field: field, Values: values}
}

// InsideOp builds an Inside operator.
func InsideOp(field string, values []any) Inside {
	return Inside{Field: field, Values: values}
}

// NotInsideOp builds a NotInside operator.
func NotInsideOp(field string, values []any) NotInside {
	return NotInside{Field: field, Values: values}
}

// IsNullOp builds an IsNull operator.
func IsNullOp(field string) IsNull { return IsNull{Field: field} }

// IsNotNullOp builds an IsNotNull operator.
func IsNotNullOp(field string) IsNotNull { return IsNotNull{Field: field} }

// AndOp combines two operators with logical AND.
func AndOp(left, right Operator) And { return And{Left: left, Right: right} }

// OrOp combines two operators with logical OR.
func OrOp(left, right Operator) Or { return Or{Left: left, Right: right} }

// NotOp negates an operator.
func NotOp(o Operator) Not { return Not{Operand: o} }
