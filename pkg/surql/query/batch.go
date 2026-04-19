package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

// Relation describes one edge pending creation by RelateMany.
//
// From and To should be full record IDs (e.g. "user:alice"). Data is
// optional; when populated, each key becomes a `SET` clause on the
// generated RELATE statement.
type Relation struct {
	From string
	To   string
	Data map[string]any
}

// UpsertMany batches multiple UPSERT-INTO records into a single
// statement, matching surql-py's upsert_many.
//
// An empty items slice is a no-op and returns (nil, nil). When
// conflictFields is non-empty a WHERE clause of the form
// `field = $item.field AND ...` is appended, allowing the server to
// match existing rows by an alternate key rather than id.
func UpsertMany(
	ctx context.Context,
	client *connection.DatabaseClient,
	table string,
	items []map[string]any,
	conflictFields []string,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if len(items) == 0 {
		return nil, nil
	}
	surql, err := BuildUpsertQuery(table, items, conflictFields)
	if err != nil {
		return nil, err
	}
	raw, err := client.Query(ctx, surql)
	if err != nil {
		return nil, err
	}
	return ExtractResult(raw), nil
}

// InsertMany inserts every element of items into table with a single
// INSERT statement. Unlike UpsertMany a duplicate id causes SurrealDB
// to fail the whole call. Mirrors surql-py's insert_many.
func InsertMany(
	ctx context.Context,
	client *connection.DatabaseClient,
	table string,
	items []map[string]any,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if len(items) == 0 {
		return nil, nil
	}
	if err := validateIdentifier(table, "table name"); err != nil {
		return nil, err
	}
	arr, err := formatItemsArray(items)
	if err != nil {
		return nil, err
	}
	surql := fmt.Sprintf("INSERT INTO %s %s;", table, arr)
	raw, err := client.Query(ctx, surql)
	if err != nil {
		return nil, err
	}
	return ExtractResult(raw), nil
}

// RelateMany creates an edge per entry in relations. Statements are
// concatenated into one request so the server round-trip cost is paid
// once. Mirrors surql-py's relate_many.
//
// fromTable and toTable are validated for identifier shape and logged
// for parity with surql-py even though they do not appear in the
// generated SurrealQL (the record ids carry their own table prefix).
func RelateMany(
	ctx context.Context,
	client *connection.DatabaseClient,
	fromTable, edge, toTable string,
	relations []Relation,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if len(relations) == 0 {
		return nil, nil
	}
	if err := validateIdentifier(edge, "edge table name"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(fromTable, "from table name"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(toTable, "to table name"); err != nil {
		return nil, err
	}
	stmts := make([]string, 0, len(relations))
	for i, rel := range relations {
		if rel.From == "" || rel.To == "" {
			return nil, surqlerrors.Newf(
				surqlerrors.ErrValidation,
				"relations[%d]: from and to record ids are required", i,
			)
		}
		if err := validateIdentifier(splitTablePart(rel.From), "from record table"); err != nil {
			return nil, err
		}
		if err := validateIdentifier(splitTablePart(rel.To), "to record table"); err != nil {
			return nil, err
		}
		stmt, err := BuildRelateQuery(rel.From, edge, rel.To, rel.Data)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
	}
	raw, err := client.Query(ctx, strings.Join(stmts, "\n"))
	if err != nil {
		return nil, err
	}
	return ExtractResult(raw), nil
}

// DeleteMany removes one record per id from table and returns any
// rows the server reports under `RETURN BEFORE`. Mirrors surql-py's
// delete_many: ids may be bare ("alice") or fully qualified
// ("user:alice").
//
// A "table does not exist" error from v3+ is treated as "nothing to
// delete" so behaviour matches v2 semantics and the Python port.
func DeleteMany(
	ctx context.Context,
	client *connection.DatabaseClient,
	table string,
	ids []string,
) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if err := validateIdentifier(table, "table name"); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, surqlerrors.New(surqlerrors.ErrValidation, "id cannot be empty")
		}
		target := id
		if !strings.Contains(id, ":") {
			target = table + ":" + id
		} else {
			if err := validateIdentifier(splitTablePart(id), "record ID table"); err != nil {
				return nil, err
			}
		}
		raw, err := client.Query(ctx, "DELETE "+target+" RETURN BEFORE;")
		if err != nil {
			if isTableMissingError(err) {
				continue
			}
			return nil, err
		}
		if rows := ExtractResult(raw); len(rows) > 0 {
			out = append(out, rows...)
		}
	}
	return out, nil
}

// BuildUpsertQuery renders a batch UPSERT statement without hitting
// the database. It is the pure-string half of UpsertMany and is
// useful for previews, logging, or composing larger scripts.
//
// Mirrors surql-py's build_upsert_query in signature, but emits
// SurrealDB v3-compatible SQL: `INSERT INTO table [...] ON DUPLICATE
// KEY UPDATE ...`. The Python port targets v2 syntax (`UPSERT INTO
// table [...]`) which v3 rejects with a parse error.
//
// When conflictFields is populated the ON DUPLICATE clause assigns
// each listed field from the incoming row (`f = $input.f`), so the
// caller controls which fields are merged on conflict. With no
// conflictFields, every key in the first row is replayed into the
// assignment list, matching v3's expectation that at least one
// assignment follows ON DUPLICATE KEY UPDATE.
//
// Returns "" when items is empty.
func BuildUpsertQuery(table string, items []map[string]any, conflictFields []string) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	if err := validateIdentifier(table, "table name"); err != nil {
		return "", err
	}
	for _, f := range conflictFields {
		if err := validateIdentifier(f, "conflict field name"); err != nil {
			return "", err
		}
	}
	arr, err := formatItemsArray(items)
	if err != nil {
		return "", err
	}
	assignFields := conflictFields
	if len(assignFields) == 0 {
		// fall back to every key of the first row -- this mirrors
		// py's intent of "replace the row on conflict" without
		// requiring the caller to enumerate fields.
		assignFields = collectAssignFields(items)
	}
	if len(assignFields) == 0 {
		return fmt.Sprintf("INSERT INTO %s %s;", table, arr), nil
	}
	parts := make([]string, len(assignFields))
	for i, f := range assignFields {
		if err := validateIdentifier(f, "assign field name"); err != nil {
			return "", err
		}
		parts[i] = fmt.Sprintf("%s = $input.%s", f, f)
	}
	return fmt.Sprintf("INSERT INTO %s %s ON DUPLICATE KEY UPDATE %s;", table, arr, strings.Join(parts, ", ")), nil
}

// collectAssignFields gathers the union of all keys across items,
// excluding `id` (which is the match key). Keys are returned sorted
// so rendering is stable.
func collectAssignFields(items []map[string]any) []string {
	seen := map[string]struct{}{}
	for _, item := range items {
		for k := range item {
			if k == "id" {
				continue
			}
			seen[k] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// BuildRelateQuery renders a single RELATE statement without hitting
// the database. Mirrors surql-py's build_relate_query.
//
// fromID / toID may be qualified ("user:alice") or bare; when bare
// the value is used verbatim. Data field names are validated to
// prevent injection.
func BuildRelateQuery(fromID, edge, toID string, data map[string]any) (string, error) {
	if err := validateIdentifier(edge, "edge table name"); err != nil {
		return "", err
	}
	if fromID == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "from record id cannot be empty")
	}
	if toID == "" {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "to record id cannot be empty")
	}
	if err := validateIdentifier(splitTablePart(fromID), "from record table"); err != nil {
		return "", err
	}
	if err := validateIdentifier(splitTablePart(toID), "to record table"); err != nil {
		return "", err
	}
	stmt := fmt.Sprintf("RELATE %s->%s->%s", fromID, edge, toID)
	if len(data) > 0 {
		parts, err := formatSetClauses(data)
		if err != nil {
			return "", err
		}
		stmt += " SET " + strings.Join(parts, ", ")
	}
	return stmt + ";", nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// formatItem renders a single record in SurrealQL object syntax.
// Field names are validated. Scalars use the shared quote rendering;
// nested maps and slices are JSON-encoded for compact parity with
// surql-py's _format_item_for_surql.
func formatItem(item map[string]any) (string, error) {
	if item == nil {
		return "", surqlerrors.New(surqlerrors.ErrValidation, "item cannot be nil")
	}
	keys := make([]string, 0, len(item))
	for k := range item {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if err := validateIdentifier(k, "field name"); err != nil {
			return "", err
		}
		v := item[k]
		rendered, err := renderLiteral(v)
		if err != nil {
			return "", err
		}
		parts = append(parts, k+": "+rendered)
	}
	return "{ " + strings.Join(parts, ", ") + " }", nil
}

// formatItemsArray renders items as a SurrealQL array literal.
func formatItemsArray(items []map[string]any) (string, error) {
	rendered := make([]string, 0, len(items))
	for i, item := range items {
		s, err := formatItem(item)
		if err != nil {
			return "", surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "items[%d]", i)
		}
		rendered = append(rendered, s)
	}
	return "[\n  " + strings.Join(rendered, ",\n  ") + "\n]", nil
}

// formatSetClauses renders a map as a list of `field = value` SET
// fragments, sorted by key so output is deterministic. Map / slice
// values are JSON-encoded to match surql-py.
func formatSetClauses(data map[string]any) ([]string, error) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if err := validateIdentifier(k, "field name"); err != nil {
			return nil, err
		}
		rendered, err := renderLiteral(data[k])
		if err != nil {
			return nil, err
		}
		parts = append(parts, k+" = "+rendered)
	}
	return parts, nil
}

// renderLiteral mirrors surql-py's dispatch: nested maps/slices are
// JSON-encoded, scalars / strings / RecordID values go through the
// standard quote path.
func renderLiteral(v any) (string, error) {
	switch vv := v.(type) {
	case map[string]any, []any, []map[string]any:
		buf, err := json.Marshal(vv)
		if err != nil {
			return "", surqlerrors.Wrap(surqlerrors.ErrSerialization, "encode literal", err)
		}
		return string(buf), nil
	case types.RecordID:
		return vv.String(), nil
	default:
		return quoteValueExpr(v), nil
	}
}
