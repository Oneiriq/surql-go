// Package types provides type-safe primitives for building SurrealDB queries.
package types

import (
	"fmt"
	"strings"
)

// SurrealReservedWords is the canonical set of SurrealDB reserved words
// used for field-name collision detection.
var SurrealReservedWords = map[string]struct{}{
	"select": {}, "from": {}, "where": {}, "group": {}, "order": {},
	"limit": {}, "start": {}, "fetch": {}, "timeout": {}, "parallel": {},
	"value": {}, "content": {}, "set": {}, "create": {}, "update": {},
	"delete": {}, "relate": {}, "insert": {}, "define": {}, "remove": {},
	"begin": {}, "commit": {}, "cancel": {}, "return": {}, "let": {},
	"if": {}, "else": {}, "then": {}, "end": {}, "for": {},
	"break": {}, "continue": {}, "throw": {}, "none": {}, "null": {},
	"true": {}, "false": {}, "and": {}, "or": {}, "not": {},
	"is": {}, "contains": {}, "inside": {}, "outside": {}, "intersects": {},
	"type": {}, "table": {}, "field": {}, "index": {}, "event": {},
	"namespace": {}, "database": {}, "scope": {}, "token": {}, "info": {},
	"live": {}, "kill": {}, "sleep": {}, "use": {}, "in": {}, "out": {},
}

// EdgeAllowedReserved names reserved words that are permitted on edge schemas.
var EdgeAllowedReserved = map[string]struct{}{
	"in":  {},
	"out": {},
}

// IsReservedWord reports whether name (case-insensitive, leaf of dot-path) is reserved.
func IsReservedWord(name string) bool {
	leaf := leafSegment(name)
	_, ok := SurrealReservedWords[strings.ToLower(leaf)]
	return ok
}

// CheckReservedWord returns a warning message when name collides with a
// SurrealDB reserved word, or the empty string when it is safe. When
// allowEdgeFields is true, the edge-allowed identifiers (`in`, `out`) are
// permitted.
//
// Dot-notation names have only their leaf segment checked.
func CheckReservedWord(name string, allowEdgeFields bool) string {
	leaf := leafSegment(name)
	lower := strings.ToLower(leaf)

	if _, reserved := SurrealReservedWords[lower]; !reserved {
		return ""
	}
	if allowEdgeFields {
		if _, ok := EdgeAllowedReserved[lower]; ok {
			return ""
		}
	}
	return fmt.Sprintf(
		"Field name %q collides with SurrealDB reserved word %q. This may cause unexpected query behavior.",
		name, lower,
	)
}

func leafSegment(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx+1:]
	}
	return name
}
