package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

// CreateRecord inserts a new record into table and returns the server
// response normalised to a single record map.
func CreateRecord(ctx context.Context, client *connection.DatabaseClient, table string, data map[string]any) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := validateIdentifier(table, "table name"); err != nil {
		return nil, err
	}
	if data == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "data cannot be nil")
	}
	res, err := client.Create(ctx, table, data)
	if err != nil {
		return nil, err
	}
	return normaliseSingle(res), nil
}

// CreateRecords inserts each element of rows as its own record. The loop
// matches the surql-py behaviour: a single failure short-circuits the
// batch.
func CreateRecords(ctx context.Context, client *connection.DatabaseClient, table string, rows []map[string]any) ([]map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := validateIdentifier(table, "table name"); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for i, row := range rows {
		if row == nil {
			return nil, surqlerrors.Newf(surqlerrors.ErrValidation, "rows[%d] cannot be nil", i)
		}
		res, err := client.Create(ctx, table, row)
		if err != nil {
			return nil, err
		}
		out = append(out, normaliseSingle(res))
	}
	return out, nil
}

// GetRecord fetches a single record by id. recordID may be a bare string
// (combined with table to form `table:id`) or a types.RecordID (used
// verbatim). Returns (nil, nil) when the record does not exist.
//
// Uses a raw `SELECT * FROM <table:id>` query rather than the SDK's
// Select shortcut because SurrealDB v3+ treats the SDK's string-target
// as a table name (rejecting "table:id" as a missing table).
func GetRecord(ctx context.Context, client *connection.DatabaseClient, table string, recordID any) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	target, err := buildRecordTarget(table, recordID)
	if err != nil {
		return nil, err
	}
	res, err := client.Query(ctx, "SELECT * FROM "+target+";")
	if err != nil {
		if isTableMissingError(err) {
			return nil, nil
		}
		return nil, err
	}
	rows := ExtractResult(res)
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// UpdateRecord replaces the record identified by recordID with data
// (PUT semantics).
func UpdateRecord(ctx context.Context, client *connection.DatabaseClient, table string, recordID any, data map[string]any) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	target, err := buildRecordTarget(table, recordID)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "data cannot be nil")
	}
	res, err := client.Update(ctx, target, data)
	if err != nil {
		return nil, err
	}
	return normaliseSingle(res), nil
}

// MergeRecord performs a partial update on the record identified by
// recordID, leaving unspecified fields untouched.
func MergeRecord(ctx context.Context, client *connection.DatabaseClient, table string, recordID any, data map[string]any) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	target, err := buildRecordTarget(table, recordID)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "data cannot be nil")
	}
	res, err := client.Merge(ctx, target, data)
	if err != nil {
		return nil, err
	}
	return normaliseSingle(res), nil
}

// UpsertRecord inserts the record when absent or replaces it when
// present. Uses the UPSERT builder so the resulting surql statement is
// validated before submission.
func UpsertRecord(ctx context.Context, client *connection.DatabaseClient, table string, recordID any, data map[string]any) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	target, err := buildRecordTarget(table, recordID)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "data cannot be nil")
	}
	q, err := Query{}.Upsert(target, data)
	if err != nil {
		return nil, err
	}
	raw, err := ExecuteQuery(ctx, client, q)
	if err != nil {
		return nil, err
	}
	if m := ExtractOne(raw); m != nil {
		return m, nil
	}
	return normaliseSingle(raw), nil
}

// GetByTarget fetches the single record referenced by a target expression
// such as [types.TypeRecord] or [types.TypeThing]. Returns (nil, nil) when
// the underlying record does not exist. Intended to be paired with
// TypeRecord/TypeThing for callers who already have a composed target.
func GetByTarget(ctx context.Context, client *connection.DatabaseClient, target types.SurrealFn) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	expr := target.ToSurql()
	if expr == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "target cannot be empty")
	}
	res, err := client.Query(ctx, "SELECT * FROM "+expr+";")
	if err != nil {
		if isTableMissingError(err) {
			return nil, nil
		}
		return nil, err
	}
	rows := ExtractResult(res)
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// UpdateByTarget replaces the record referenced by target (PUT semantics).
// Mirrors [UpdateRecord] but takes a single target expression so callers
// can use [types.TypeRecord] / [types.TypeThing] verbatim.
func UpdateByTarget(ctx context.Context, client *connection.DatabaseClient, target types.SurrealFn, data map[string]any) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	expr := target.ToSurql()
	if expr == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "target cannot be empty")
	}
	if data == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "data cannot be nil")
	}
	if err := validateDataKeys(data); err != nil {
		return nil, err
	}
	// Render as a raw UPDATE so the target expression is splice-safe.
	surql := "UPDATE " + expr + " CONTENT " + renderDataObject(data) + ";"
	res, err := client.Query(ctx, surql)
	if err != nil {
		return nil, err
	}
	if m := ExtractOne(res); m != nil {
		return m, nil
	}
	return normaliseSingle(res), nil
}

// MergeByTarget partially updates the record referenced by target
// (PATCH semantics). Mirrors [MergeRecord] for composed targets.
func MergeByTarget(ctx context.Context, client *connection.DatabaseClient, target types.SurrealFn, data map[string]any) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	expr := target.ToSurql()
	if expr == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "target cannot be empty")
	}
	if data == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "data cannot be nil")
	}
	if err := validateDataKeys(data); err != nil {
		return nil, err
	}
	surql := "UPDATE " + expr + " MERGE " + renderDataObject(data) + ";"
	res, err := client.Query(ctx, surql)
	if err != nil {
		return nil, err
	}
	if m := ExtractOne(res); m != nil {
		return m, nil
	}
	return normaliseSingle(res), nil
}

// UpsertByTarget inserts-or-replaces the record referenced by target.
// Mirrors [UpsertRecord] for composed targets.
func UpsertByTarget(ctx context.Context, client *connection.DatabaseClient, target types.SurrealFn, data map[string]any) (map[string]any, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	expr := target.ToSurql()
	if expr == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "target cannot be empty")
	}
	if data == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "data cannot be nil")
	}
	if err := validateDataKeys(data); err != nil {
		return nil, err
	}
	surql := "UPSERT " + expr + " CONTENT " + renderDataObject(data) + ";"
	res, err := client.Query(ctx, surql)
	if err != nil {
		return nil, err
	}
	if m := ExtractOne(res); m != nil {
		return m, nil
	}
	return normaliseSingle(res), nil
}

// DeleteByTarget removes the record referenced by target. A "table does
// not exist" error is treated as a no-op for v3 parity.
func DeleteByTarget(ctx context.Context, client *connection.DatabaseClient, target types.SurrealFn) error {
	if client == nil {
		return surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	expr := target.ToSurql()
	if expr == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "target cannot be empty")
	}
	if _, err := client.Query(ctx, "DELETE "+expr+";"); err != nil {
		if isTableMissingError(err) {
			return nil
		}
		return err
	}
	return nil
}

// ExistsByTarget reports whether the record referenced by target exists.
func ExistsByTarget(ctx context.Context, client *connection.DatabaseClient, target types.SurrealFn) (bool, error) {
	rec, err := GetByTarget(ctx, client, target)
	if err != nil {
		return false, err
	}
	return rec != nil, nil
}

// DeleteRecord removes a single record identified by recordID.
func DeleteRecord(ctx context.Context, client *connection.DatabaseClient, table string, recordID any) error {
	if client == nil {
		return surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	target, err := buildRecordTarget(table, recordID)
	if err != nil {
		return err
	}
	// Use raw `DELETE <table:id>` -- the SDK's Delete treats its string
	// target as a table name, which v3+ rejects when given "table:id".
	// A "table does not exist" error is treated as a successful no-op to
	// match v2 behaviour and the Python port.
	if _, err := client.Query(ctx, "DELETE "+target+";"); err != nil {
		if isTableMissingError(err) {
			return nil
		}
		return err
	}
	return nil
}

// DeleteRecords removes every record in table matching condition. Pass a
// nil condition to delete the whole table.
//
// condition may be nil, a raw SurrealQL fragment (string) or any
// types.Operator implementation.
func DeleteRecords(ctx context.Context, client *connection.DatabaseClient, table string, condition any) error {
	if client == nil {
		return surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	q, err := Query{}.Delete(table)
	if err != nil {
		return err
	}
	if condition != nil {
		q, err = q.Where(condition)
		if err != nil {
			return err
		}
	}
	if _, err := ExecuteQuery(ctx, client, q); err != nil {
		return err
	}
	return nil
}

// QueryOptions carries the common filter / pagination inputs for
// QueryRecords. All fields are optional; leaving them zero yields a
// `SELECT * FROM table` statement.
type QueryOptions struct {
	// Conditions is a list of WHERE fragments or types.Operator values.
	Conditions []any
	// OrderBy sets an ORDER BY clause when non-nil (field + direction).
	OrderBy *OrderField
	// Limit bounds the number of returned rows when non-nil.
	Limit *int
	// Offset skips the first N rows when non-nil.
	Offset *int
}

// QueryRecords runs a SELECT against table with the supplied filters and
// returns the raw record maps.
func QueryRecords(ctx context.Context, client *connection.DatabaseClient, table string, opts QueryOptions) ([]map[string]any, error) {
	q, err := buildSelectFromOptions(table, opts)
	if err != nil {
		return nil, err
	}
	raw, err := ExecuteQuery(ctx, client, q)
	if err != nil {
		return nil, err
	}
	return ExtractResult(raw), nil
}

// QueryRecordsWrapped runs QueryRecords and returns the resulting rows
// wrapped in a ListResult, retaining the Limit/Offset hints from opts so
// downstream callers can inspect pagination metadata without re-threading
// them manually. Mirrors surql-py's `query_records_wrapped`.
func QueryRecordsWrapped(ctx context.Context, client *connection.DatabaseClient, table string, opts QueryOptions) (ListResult[map[string]any], error) {
	rows, err := QueryRecords(ctx, client, table, opts)
	if err != nil {
		return ListResult[map[string]any]{}, err
	}
	var limit, offset *uint64
	if opts.Limit != nil {
		v := uint64(*opts.Limit)
		limit = &v
	}
	if opts.Offset != nil {
		v := uint64(*opts.Offset)
		offset = &v
	}
	return Records(rows, nil, limit, offset), nil
}

// CountRecords returns `count()` for table, optionally scoped by a
// condition (string or types.Operator).
func CountRecords(ctx context.Context, client *connection.DatabaseClient, table string, condition any) (int64, error) {
	if client == nil {
		return 0, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	// SurrealDB: `SELECT count() FROM table` returns one row per record
	// (each with `count: 1`); `SELECT count() FROM table GROUP ALL` returns
	// a single aggregated row. Mirror the Python port's behaviour by
	// always requesting GROUP ALL.
	q, err := Query{}.Select([]string{"count()"}).FromTable(table)
	if err != nil {
		return 0, err
	}
	if condition != nil {
		q, err = q.Where(condition)
		if err != nil {
			return 0, err
		}
	}
	q = q.GroupAll()
	raw, err := ExecuteQuery(ctx, client, q)
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

// Exists reports whether the record identified by recordID is present.
// A "table does not exist" error from SurrealDB v3+ is treated as "not
// present" rather than a hard failure, matching Python's semantic where
// a missing table counts as 'record absent'.
func Exists(ctx context.Context, client *connection.DatabaseClient, table string, recordID any) (bool, error) {
	rec, err := GetRecord(ctx, client, table, recordID)
	if err != nil {
		if isTableMissingError(err) {
			return false, nil
		}
		return false, err
	}
	return rec != nil, nil
}

// tableMissingNeedles is the set of case-insensitive substrings that
// SurrealDB surfaces when a table-level operation targets a table that
// has not been materialised. v3.0.x returns "The table '...' does not
// exist"; the trailing "does not exist" fragment is kept as a
// forward-compat fallback in case future versions rephrase the message.
// "table not found" is included defensively for the same reason.
var tableMissingNeedles = []string{
	"does not exist",  // v3.0.x: "The table '...' does not exist"
	"table not found", // hypothetical future wording
}

// isTableMissingError reports whether err is SurrealDB's "table does
// not exist" response. v2.x treated missing tables as empty results;
// v3+ returns an error. Callers that want an empty result to be
// equivalent to "no records" use this check.
//
// The match is case-insensitive against the substrings listed in
// tableMissingNeedles. See pkg/surql/query/crud_integration_test.go for
// the regression test pinning us to the v3 server wording.
func isTableMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range tableMissingNeedles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// First returns the first record matching the filter options, or nil.
// A convenience wrapper that forces `LIMIT 1`.
func First(ctx context.Context, client *connection.DatabaseClient, table string, opts QueryOptions) (map[string]any, error) {
	one := 1
	opts.Limit = &one
	rows, err := QueryRecords(ctx, client, table, opts)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// Last returns the last record matching the filter options, or nil. If
// an OrderBy is supplied its direction is flipped; without one the call
// devolves to First.
func Last(ctx context.Context, client *connection.DatabaseClient, table string, opts QueryOptions) (map[string]any, error) {
	if opts.OrderBy != nil {
		flipped := OrderField{Field: opts.OrderBy.Field}
		if opts.OrderBy.Direction == "ASC" {
			flipped.Direction = "DESC"
		} else {
			flipped.Direction = "ASC"
		}
		opts.OrderBy = &flipped
	}
	return First(ctx, client, table, opts)
}

// AggregateOpts configures a single aggregation query assembled by
// [AggregateRecords]. At least one aggregation expression must be
// supplied via Select; GroupBy / GroupAll / Where / Having are all
// optional.
type AggregateOpts struct {
	// Table is the source table. Required.
	Table string
	// Select maps result alias -> aggregation expression. Accepts any
	// [types.Operator] implementation (SurrealFn, Expression, raw
	// wrappers). Keys are emitted in sorted order for deterministic
	// rendering.
	Select map[string]types.Operator
	// GroupBy lists explicit grouping fields. Mutually exclusive with
	// GroupAll; if both are supplied, GroupAll wins.
	GroupBy []string
	// GroupAll toggles SurrealQL's `GROUP ALL` whole-result aggregation.
	GroupAll bool
	// Where is an optional filter applied before grouping. Each entry is
	// a raw SurrealQL fragment or a types.Operator implementer.
	Where []any
	// OrderBy sets an ORDER BY clause when non-nil (field + direction).
	OrderBy *OrderField
	// Limit bounds the number of aggregated rows when non-nil.
	Limit *int
	// Offset skips the first N aggregated rows when non-nil.
	Offset *int
}

// AggregateRecords runs a SELECT projection of aggregation expressions
// against opts.Table and returns the raw aggregated rows. Each row's
// keys match the aliases in opts.Select.
//
// Example:
//
//	opts := query.AggregateOpts{
//	    Table:   "memory_entry",
//	    Select:  map[string]types.Operator{
//	        "count": query.CountAll(),
//	        "total": query.MathSum("strength"),
//	    },
//	    GroupBy: []string{"network"},
//	}
//	rows, err := query.AggregateRecords(ctx, client, opts)
func AggregateRecords(ctx context.Context, client *connection.DatabaseClient, opts AggregateOpts) ([]map[string]any, error) {
	if opts.Table == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "AggregateOpts.Table cannot be empty")
	}
	if len(opts.Select) == 0 {
		return nil, surqlerrors.New(surqlerrors.ErrValidation,
			"AggregateOpts.Select must contain at least one aggregation expression")
	}
	q := Query{}.SelectAliased(opts.Select)
	// SurrealDB requires every GROUP BY idiom to appear in the projection;
	// prepend the group fields verbatim so callers don't have to
	// remember.
	if !opts.GroupAll && len(opts.GroupBy) > 0 {
		grouped := append([]string(nil), opts.GroupBy...)
		grouped = append(grouped, q.Fields...)
		q.Fields = grouped
	}
	q, err := q.FromTable(opts.Table)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	for _, w := range opts.Where {
		q, err = q.Where(w)
		if err != nil {
			return nil, err
		}
	}
	if opts.GroupAll {
		q = q.GroupAll()
	} else if len(opts.GroupBy) > 0 {
		q = q.GroupBy(opts.GroupBy...)
	}
	if opts.OrderBy != nil {
		q, err = q.OrderBy(opts.OrderBy.Field, opts.OrderBy.Direction)
		if err != nil {
			return nil, err
		}
	}
	if opts.Limit != nil {
		q, err = q.Limit(*opts.Limit)
		if err != nil {
			return nil, err
		}
	}
	if opts.Offset != nil {
		q, err = q.Offset(*opts.Offset)
		if err != nil {
			return nil, err
		}
	}
	raw, err := ExecuteQuery(ctx, client, q)
	if err != nil {
		if isTableMissingError(err) {
			return nil, nil
		}
		return nil, err
	}
	return ExtractResult(raw), nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// buildRecordTarget normalises the (table, id) pair into a SurrealDB
// record target such as `user:alice` or `user:<alice>`. When recordID is
// a types.RecordID its own string form is used verbatim.
func buildRecordTarget(table string, recordID any) (string, error) {
	switch v := recordID.(type) {
	case nil:
		return "", surqlerrors.New(surqlerrors.ErrValidation, "record id cannot be nil")
	case types.RecordID:
		return v.String(), nil
	case string:
		if v == "" {
			return "", surqlerrors.New(surqlerrors.ErrValidation, "record id cannot be empty")
		}
		if err := validateIdentifier(table, "table name"); err != nil {
			return "", err
		}
		return table + ":" + v, nil
	case fmt.Stringer:
		s := v.String()
		if s == "" {
			return "", surqlerrors.New(surqlerrors.ErrValidation, "record id cannot be empty")
		}
		if err := validateIdentifier(table, "table name"); err != nil {
			return "", err
		}
		return table + ":" + s, nil
	default:
		return "", surqlerrors.Newf(surqlerrors.ErrValidation,
			"record id must be string or types.RecordID, got %T", recordID)
	}
}

// buildSelectFromOptions materialises a Query from a QueryOptions value.
func buildSelectFromOptions(table string, opts QueryOptions) (Query, error) {
	q, err := Query{}.Select(nil).FromTable(table)
	if err != nil {
		return Query{}, err
	}
	for _, c := range opts.Conditions {
		q, err = q.Where(c)
		if err != nil {
			return Query{}, err
		}
	}
	if opts.OrderBy != nil {
		q, err = q.OrderBy(opts.OrderBy.Field, opts.OrderBy.Direction)
		if err != nil {
			return Query{}, err
		}
	}
	if opts.Limit != nil {
		q, err = q.Limit(*opts.Limit)
		if err != nil {
			return Query{}, err
		}
	}
	if opts.Offset != nil {
		q, err = q.Offset(*opts.Offset)
		if err != nil {
			return Query{}, err
		}
	}
	return q, nil
}

// normaliseSingle reduces an SDK response to a single map[string]any.
// SurrealDB occasionally wraps scalars or returns a one-element slice; we
// pick whichever form is most useful to downstream callers.
func normaliseSingle(v any) map[string]any {
	switch x := v.(type) {
	case nil:
		return nil
	case map[string]any:
		if inner, ok := x["result"]; ok {
			if nested := normaliseSingle(inner); nested != nil {
				return nested
			}
		}
		return x
	case []any:
		if len(x) == 0 {
			return nil
		}
		return normaliseSingle(x[0])
	default:
		return nil
	}
}

// toInt64 coerces JSON-ish numerics (float64, json.Number, int, int64,
// uint*) into an int64; returns 0 for anything else.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		return int64(n)
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
	}
	return 0
}
