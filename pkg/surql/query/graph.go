package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

// TraverseDirection picks the arrow used by graph helpers.
//
// It mirrors surql-py's `direction: "out" | "in" | "both"` literal.
type TraverseDirection string

const (
	// TraverseOut follows outgoing edges (->).
	TraverseOut TraverseDirection = "out"
	// TraverseIn follows incoming edges (<-).
	TraverseIn TraverseDirection = "in"
	// TraverseBoth follows edges in both directions (<->).
	TraverseBoth TraverseDirection = "both"
)

// arrow returns the SurrealQL arrow symbol for a direction or an
// ErrValidation-wrapped error for an unknown value.
func (d TraverseDirection) arrow() (string, error) {
	switch d {
	case TraverseOut, "":
		return "->", nil
	case TraverseIn:
		return "<-", nil
	case TraverseBoth:
		return "<->", nil
	default:
		return "", surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"invalid direction %q: must be \"out\", \"in\" or \"both\"", string(d),
		)
	}
}

// recordToString coerces a record reference (string or types.RecordID)
// into its SurrealQL target string.
func recordToString(record any, role string) (string, error) {
	switch v := record.(type) {
	case nil:
		return "", surqlerrors.Newf(surqlerrors.ErrValidation, "%s record cannot be nil", role)
	case string:
		if v == "" {
			return "", surqlerrors.Newf(surqlerrors.ErrValidation, "%s record cannot be empty", role)
		}
		return v, nil
	case types.RecordID:
		return v.String(), nil
	case fmt.Stringer:
		s := v.String()
		if s == "" {
			return "", surqlerrors.Newf(surqlerrors.ErrValidation, "%s record cannot be empty", role)
		}
		return s, nil
	default:
		return "", surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"%s record must be string or types.RecordID, got %T", role, record,
		)
	}
}

// applyConditions appends each entry of conditions to q as a WHERE
// clause. Entries combine with AND and each is rendered by
// [Query.Where], so raw SurrealQL strings and types.Operator values are
// both accepted and may be mixed in one slice. A nil or empty slice is
// a no-op, which is what keeps the emitted SurrealQL unchanged for
// callers that do not filter.
func applyConditions(q Query, conditions []any) (Query, error) {
	for _, condition := range conditions {
		next, err := q.Where(condition)
		if err != nil {
			return Query{}, err
		}
		q = next
	}
	return q, nil
}

// selectTraversalSurql renders `SELECT * FROM <start><path> [WHERE ...];`.
//
// Split out from the helpers so statement construction is testable
// without a live client.
func selectTraversalSurql(start, path string, conditions []any) (string, error) {
	q, err := NewQuery().Select(nil).FromTable(start)
	if err != nil {
		return "", err
	}
	q, err = applyConditions(q.Traverse(path), conditions)
	if err != nil {
		return "", err
	}
	stmt, err := q.ToSurql()
	if err != nil {
		return "", err
	}
	return stmt + ";", nil
}

// Traverse navigates the graph from a starting record along a raw
// SurrealQL path (e.g. `->likes->post`, `<-follows<-user`) and returns
// the destination rows. Mirrors surql-py's traverse.
//
// Path is injected verbatim — callers compose paths from validated
// identifiers via TraverseWithDepth or by building the path
// themselves.
//
// conditions are appended as WHERE clauses combined with AND; each
// entry may be a raw SurrealQL string or a types.Operator. Pass nil to
// filter nothing. This is the hook for row-level isolation — a
// traversal that must stay inside a tenant boundary carries its guard
// here rather than abandoning the helper.
func Traverse(
	ctx context.Context,
	client *connection.DatabaseClient,
	start any,
	path string,
	conditions []any,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	startStr, err := recordToString(start, "start")
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "path cannot be empty")
	}
	stmt, err := selectTraversalSurql(startStr, path, conditions)
	if err != nil {
		return nil, err
	}
	raw, err := client.Query(ctx, stmt)
	if err != nil {
		return nil, err
	}
	return ExtractResult(raw), nil
}

// TraverseWithDepth is the structured companion to Traverse: it
// assembles `<arrow><edge><depth?><arrow><target>` after validating
// every identifier. A nil depth emits no depth suffix.
//
// Mirrors surql-py's traverse_with_depth.
func TraverseWithDepth(
	ctx context.Context,
	client *connection.DatabaseClient,
	start any,
	edgeTable, targetTable string,
	direction TraverseDirection,
	depth *int,
	conditions []any,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := validateIdentifier(edgeTable, "edge table name"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(targetTable, "target table name"); err != nil {
		return nil, err
	}
	arrow, err := direction.arrow()
	if err != nil {
		return nil, err
	}
	depthStr := ""
	if depth != nil {
		if *depth < 0 {
			return nil, surqlerrors.Newf(surqlerrors.ErrValidation, "depth must be non-negative, got %d", *depth)
		}
		depthStr = fmt.Sprintf("%d", *depth)
	}
	path := fmt.Sprintf("%s%s%s%s%s", arrow, edgeTable, depthStr, arrow, targetTable)
	return Traverse(ctx, client, start, path, conditions)
}

// CreateRelation opens a single RELATE statement between two records,
// returning the created edge row. Mirrors surql-py's relate.
//
// data keys are validated for identifier shape and rendered with the
// standard SurrealQL literal quoter.
func CreateRelation(
	ctx context.Context,
	client *connection.DatabaseClient,
	edgeTable string,
	fromRecord, toRecord any,
	data map[string]any,
) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	fromStr, err := recordToString(fromRecord, "from")
	if err != nil {
		return nil, err
	}
	toStr, err := recordToString(toRecord, "to")
	if err != nil {
		return nil, err
	}
	stmt, err := BuildRelateQuery(fromStr, edgeTable, toStr, data)
	if err != nil {
		return nil, err
	}
	raw, err := client.Query(ctx, stmt)
	if err != nil {
		return nil, err
	}
	return ExtractOne(raw), nil
}

// RemoveRelation deletes the edge that matches from->edge->to.
// Mirrors surql-py's unrelate. A missing edge table is treated as a
// no-op.
func RemoveRelation(
	ctx context.Context,
	client *connection.DatabaseClient,
	edgeTable string,
	fromRecord, toRecord any,
) error {
	if client == nil {
		return surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := validateIdentifier(edgeTable, "edge table name"); err != nil {
		return err
	}
	fromStr, err := recordToString(fromRecord, "from")
	if err != nil {
		return err
	}
	toStr, err := recordToString(toRecord, "to")
	if err != nil {
		return err
	}
	stmt := fmt.Sprintf("DELETE %s->%s->%s;", fromStr, edgeTable, toStr)
	if _, err := client.Query(ctx, stmt); err != nil {
		if isTableMissingError(err) {
			return nil
		}
		return err
	}
	return nil
}

// GetOutgoingEdges returns every edge of `edgeTable` originating at
// `record`. Mirrors surql-py's get_outgoing_edges.
func GetOutgoingEdges(
	ctx context.Context,
	client *connection.DatabaseClient,
	record any,
	edgeTable string,
	conditions []any,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := validateIdentifier(edgeTable, "edge table name"); err != nil {
		return nil, err
	}
	recordStr, err := recordToString(record, "record")
	if err != nil {
		return nil, err
	}
	stmt, err := selectTraversalSurql(recordStr, "->"+edgeTable, conditions)
	if err != nil {
		return nil, err
	}
	raw, err := client.Query(ctx, stmt)
	if err != nil {
		if isTableMissingError(err) {
			return nil, nil
		}
		return nil, err
	}
	return ExtractResult(raw), nil
}

// GetIncomingEdges returns every edge of `edgeTable` terminating at
// `record`.
//
// Deviates from the Python source's `FROM <-edge<-record` ordering:
// SurrealDB v3 requires the record at the head of the `FROM`
// expression (`FROM record<-edge`). The Python shape is a parse error
// on v3 -- verified against v3.0.5, which rejects
// `SELECT * FROM <-follows<-person:bob` with "Unexpected token `;`".
// This matches the ordering the Rust port already uses.
func GetIncomingEdges(
	ctx context.Context,
	client *connection.DatabaseClient,
	record any,
	edgeTable string,
	conditions []any,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := validateIdentifier(edgeTable, "edge table name"); err != nil {
		return nil, err
	}
	recordStr, err := recordToString(record, "record")
	if err != nil {
		return nil, err
	}
	stmt, err := selectTraversalSurql(recordStr, "<-"+edgeTable, conditions)
	if err != nil {
		return nil, err
	}
	raw, err := client.Query(ctx, stmt)
	if err != nil {
		if isTableMissingError(err) {
			return nil, nil
		}
		return nil, err
	}
	return ExtractResult(raw), nil
}

// GetRelatedRecords returns the records at the far end of the edge
// traversal from `record`. Mirrors surql-py's get_related_records.
// direction must be TraverseOut or TraverseIn; TraverseBoth is
// rejected to match the Python port's enforcement.
func GetRelatedRecords(
	ctx context.Context,
	client *connection.DatabaseClient,
	record any,
	edgeTable, targetTable string,
	direction TraverseDirection,
	conditions []any,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := validateIdentifier(edgeTable, "edge table name"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(targetTable, "target table name"); err != nil {
		return nil, err
	}
	recordStr, err := recordToString(record, "record")
	if err != nil {
		return nil, err
	}
	var path string
	switch direction {
	case TraverseOut, "":
		path = fmt.Sprintf("->%s->%s", edgeTable, targetTable)
	case TraverseIn:
		path = fmt.Sprintf("<-%s<-%s", edgeTable, targetTable)
	default:
		return nil, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"invalid direction %q: must be \"out\" or \"in\"", string(direction),
		)
	}
	stmt, err := selectTraversalSurql(recordStr, path, conditions)
	if err != nil {
		return nil, err
	}
	raw, err := client.Query(ctx, stmt)
	if err != nil {
		if isTableMissingError(err) {
			return nil, nil
		}
		return nil, err
	}
	return ExtractResult(raw), nil
}

// CountRelated returns the number of records connected to `record`
// through `edgeTable` in the given direction. Mirrors surql-py's
// count_related and enforces `GROUP ALL` to match the project-wide
// aggregate discipline.
func CountRelated(
	ctx context.Context,
	client *connection.DatabaseClient,
	record any,
	edgeTable string,
	direction TraverseDirection,
) (int64, error) {
	if client == nil {
		return 0, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := validateIdentifier(edgeTable, "edge table name"); err != nil {
		return 0, err
	}
	recordStr, err := recordToString(record, "record")
	if err != nil {
		return 0, err
	}
	var path string
	switch direction {
	case TraverseOut, "":
		path = "->" + edgeTable
	case TraverseIn:
		// Record-first, as in GetIncomingEdges: `FROM <-edge<-record`
		// is a parse error on SurrealDB v3.
		path = "<-" + edgeTable
	default:
		return 0, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"invalid direction %q: must be \"out\" or \"in\"", string(direction),
		)
	}
	counted, err := NewQuery().Select([]string{"count()"}).FromTable(recordStr)
	if err != nil {
		return 0, err
	}
	stmt, err := counted.Traverse(path).GroupAll().ToSurql()
	if err != nil {
		return 0, err
	}
	raw, err := client.Query(ctx, stmt+";")
	if err != nil {
		if isTableMissingError(err) {
			return 0, nil
		}
		return 0, err
	}
	first := ExtractOne(raw)
	if first == nil {
		return 0, nil
	}
	if v, ok := first["count"]; ok {
		return toInt64(v), nil
	}
	return 0, nil
}

// ShortestPath runs an iterative-deepening search for the shortest
// path from `fromRecord` to `toRecord` through `edgeTable` and
// returns the end-of-path record on the first depth that matches.
//
// Mirrors surql-py's shortest_path in intent but emits SurrealDB v3
// syntax: `SELECT * FROM <from>(->edge->?)*depth WHERE id = <to>`.
// The Python port ships a v2 shape (`->edge<depth>->`) that v3
// rejects.
func ShortestPath(
	ctx context.Context,
	client *connection.DatabaseClient,
	fromRecord, toRecord any,
	edgeTable string,
	maxDepth int,
	conditions []any,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := validateIdentifier(edgeTable, "edge table name"); err != nil {
		return nil, err
	}
	if maxDepth <= 0 {
		maxDepth = 10
	}
	fromStr, err := recordToString(fromRecord, "from")
	if err != nil {
		return nil, err
	}
	toStr, err := recordToString(toRecord, "to")
	if err != nil {
		return nil, err
	}
	step := "->" + edgeTable + "->?"
	for depth := 1; depth <= maxDepth; depth++ {
		path := strings.Repeat(step, depth)
		// The identity predicate leads; caller guards narrow it further.
		guards := make([]any, 0, len(conditions)+1)
		guards = append(guards, fmt.Sprintf("id = %s", toStr))
		guards = append(guards, conditions...)
		stmt, err := selectTraversalSurql(fromStr, path, guards)
		if err != nil {
			return nil, err
		}
		raw, err := client.Query(ctx, stmt)
		if err != nil {
			if isTableMissingError(err) {
				return nil, nil
			}
			return nil, err
		}
		rows := ExtractResult(raw)
		if len(rows) > 0 {
			return rows, nil
		}
	}
	return nil, nil
}

// buildGraphPath is a shared helper used by GraphQuery and a couple of
// direct callers: it joins a list of path fragments with no
// separator, trimming empty entries.
func buildGraphPath(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(p)
	}
	return b.String()
}
