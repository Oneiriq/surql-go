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
