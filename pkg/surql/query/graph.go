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

// Traverse navigates the graph from a starting record along a raw
// SurrealQL path (e.g. `->likes->post`, `<-follows<-user`) and returns
// the destination rows. Mirrors surql-py's traverse.
//
// Path is injected verbatim — callers compose paths from validated
// identifiers via TraverseWithDepth or by building the path
// themselves.
func Traverse(
	ctx context.Context,
	client *connection.DatabaseClient,
	start any,
	path string,
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
	raw, err := client.Query(ctx, fmt.Sprintf("SELECT * FROM %s%s;", startStr, path))
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
	return Traverse(ctx, client, start, path)
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
	stmt := fmt.Sprintf("SELECT * FROM %s->%s;", recordStr, edgeTable)
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
// `record`. Mirrors surql-py's get_incoming_edges.
func GetIncomingEdges(
	ctx context.Context,
	client *connection.DatabaseClient,
	record any,
	edgeTable string,
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
	stmt := fmt.Sprintf("SELECT * FROM <-%s<-%s;", edgeTable, recordStr)
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
	raw, err := client.Query(ctx, fmt.Sprintf("SELECT * FROM %s%s;", recordStr, path))
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
	var target string
	switch direction {
	case TraverseOut, "":
		target = fmt.Sprintf("%s->%s", recordStr, edgeTable)
	case TraverseIn:
		target = fmt.Sprintf("<-%s<-%s", edgeTable, recordStr)
	default:
		return 0, surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"invalid direction %q: must be \"out\" or \"in\"", string(direction),
		)
	}
	stmt := fmt.Sprintf("SELECT count() FROM %s GROUP ALL;", target)
	raw, err := client.Query(ctx, stmt)
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
		stmt := fmt.Sprintf("SELECT * FROM %s%s WHERE id = %s;", fromStr, path, toStr)
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
