package query

import (
	"regexp"
	"strings"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// ReturnFormat selects the data returned by a mutation.
//
// Mirrors the Python `ReturnFormat` enum: NONE returns nothing, DIFF
// returns only changed fields, FULL returns the entire record, and
// BEFORE / AFTER capture state around the write.
type ReturnFormat string

const (
	// ReturnNone omits the record body from the response.
	ReturnNone ReturnFormat = "NONE"
	// ReturnDiff emits just the fields that changed.
	ReturnDiff ReturnFormat = "DIFF"
	// ReturnFull emits the full record.
	ReturnFull ReturnFormat = "FULL"
	// ReturnBefore emits the record state before the write.
	ReturnBefore ReturnFormat = "BEFORE"
	// ReturnAfter emits the record state after the write (SurrealDB default).
	ReturnAfter ReturnFormat = "AFTER"
)

// ToSurql renders the format as its SurrealQL keyword.
func (f ReturnFormat) ToSurql() string { return string(f) }

// String implements fmt.Stringer.
func (f ReturnFormat) String() string { return string(f) }

// VectorDistanceType names a vector-similarity metric.
//
// Mirrors the Python `VectorDistanceType` literal. MAHALANOBIS is
// included for parity even though the surql-py public surface only
// lists 8 metrics — the generator and SurrealDB both accept it.
type VectorDistanceType string

const (
	// DistanceCosine is cosine similarity.
	DistanceCosine VectorDistanceType = "COSINE"
	// DistanceEuclidean is L2 distance.
	DistanceEuclidean VectorDistanceType = "EUCLIDEAN"
	// DistanceManhattan is L1 distance.
	DistanceManhattan VectorDistanceType = "MANHATTAN"
	// DistanceChebyshev is L-infinity distance.
	DistanceChebyshev VectorDistanceType = "CHEBYSHEV"
	// DistanceMinkowski is generalized Lp distance.
	DistanceMinkowski VectorDistanceType = "MINKOWSKI"
	// DistanceHamming is hamming distance.
	DistanceHamming VectorDistanceType = "HAMMING"
	// DistanceJaccard is jaccard distance.
	DistanceJaccard VectorDistanceType = "JACCARD"
	// DistancePearson is pearson correlation distance.
	DistancePearson VectorDistanceType = "PEARSON"
	// DistanceMahalanobis is mahalanobis distance (parity extension).
	DistanceMahalanobis VectorDistanceType = "MAHALANOBIS"
)

// identifierPattern matches a safe SurrealDB identifier.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validateIdentifier checks that a table / field / edge name is safe to
// splice into a generated SurrealQL string.
func validateIdentifier(name, context string) error {
	if name == "" {
		return surqlerrors.Newf(surqlerrors.ErrValidation, "%s cannot be empty", capitalize(context))
	}
	if !identifierPattern.MatchString(name) {
		return surqlerrors.Newf(
			surqlerrors.ErrValidation,
			"Invalid %s: %q. Must contain only alphanumeric characters and underscores, and cannot start with a digit",
			context, name,
		)
	}
	return nil
}

// splitTablePart peels off the `table:id` prefix so we can validate just
// the table portion of a target string.
func splitTablePart(target string) string {
	if idx := strings.Index(target, ":"); idx >= 0 {
		return target[:idx]
	}
	return target
}

// capitalize upper-cases the first rune of s.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ---------------------------------------------------------------------------
// Standalone helper constructors — mirror the free functions in helpers.py.
// Each produces a fresh Query with the relevant state set.
// ---------------------------------------------------------------------------

// Select starts a SELECT query with the given fields. Pass nil for `SELECT *`.
func Select(fields []string) Query {
	return Query{}.Select(fields)
}

// FromTable builds a Query whose target is the given table.
func FromTable(table string) (Query, error) {
	return Query{}.FromTable(table)
}

// Where adds a WHERE condition to an existing query. The condition can
// be a raw string or any value implementing `types.Operator`.
func Where(q Query, condition any) (Query, error) {
	return q.Where(condition)
}

// OrderBy adds an ORDER BY clause to an existing query.
func OrderBy(q Query, field, direction string) (Query, error) {
	return q.OrderBy(field, direction)
}

// Limit adds a LIMIT clause to an existing query.
func Limit(q Query, n int) (Query, error) {
	return q.Limit(n)
}

// Offset adds an OFFSET clause to an existing query.
func Offset(q Query, n int) (Query, error) {
	return q.Offset(n)
}

// Insert creates an INSERT query.
func Insert(table string, data map[string]any) (Query, error) {
	return Query{}.Insert(table, data)
}

// Update creates an UPDATE query.
func Update(target string, data map[string]any) (Query, error) {
	return Query{}.Update(target, data)
}

// Upsert creates an UPSERT query.
func Upsert(target string, data map[string]any) (Query, error) {
	return Query{}.Upsert(target, data)
}

// Delete creates a DELETE query.
func Delete(target string) (Query, error) {
	return Query{}.Delete(target)
}

// Relate creates a RELATE query for the given edge + endpoints.
//
// `from` and `to` may be `string` or `types.RecordID`.
func Relate(edgeTable string, from, to any, data map[string]any) (Query, error) {
	return Query{}.Relate(edgeTable, from, to, data)
}

// VectorSearchQuery is the convenience builder: `SELECT ... FROM table
// WHERE field <|k,distance|> vector`.
func VectorSearchQuery(
	table, field string,
	vector []float64,
	k int,
	distance VectorDistanceType,
	fields []string,
	threshold *float64,
) (Query, error) {
	q := Select(fields)
	q, err := q.FromTable(table)
	if err != nil {
		return Query{}, err
	}
	return q.VectorSearch(field, vector, k, distance, threshold)
}

// SimilaritySearchQuery mirrors `similarity_search_query`: combines a
// vector search clause with a similarity score field in the projection.
func SimilaritySearchQuery(
	table, field string,
	vector []float64,
	k int,
	distance VectorDistanceType,
	threshold *float64,
	fields []string,
	alias string,
) (Query, error) {
	q := Select(fields)
	q, err := q.FromTable(table)
	if err != nil {
		return Query{}, err
	}
	if alias == "" {
		alias = "similarity"
	}
	q = q.SimilarityScore(field, vector, distance, alias)
	return q.VectorSearch(field, vector, k, distance, threshold)
}
