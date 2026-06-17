package query

import (
	"errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/types"
)

// mustFrom is a shorthand for `Query{}.Select(fields).FromTable(t)` that
// returns the Query directly (fatal on error).
func mustFrom(t *testing.T, fields []string, table string) Query {
	t.Helper()
	q, err := Query{}.Select(fields).FromTable(table)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return q
}

func TestQuery_ZeroValueFailsToRender(t *testing.T) {
	if _, err := (Query{}).ToSurql(); err == nil {
		t.Fatal("expected error for empty operation")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestSelect_AllFields(t *testing.T) {
	q := mustFrom(t, nil, "user")
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM user"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSelect_SpecificFields(t *testing.T) {
	q := mustFrom(t, []string{"name", "email"}, "user")
	got, err := q.ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT name, email FROM user"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestSelect_WhereStringAndOperator(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("user")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.Where("age > 18")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.Where(types.EqOp("status", "active"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "SELECT * FROM user WHERE (age > 18) AND (status = 'active')"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestSelect_OrderByAsc(t *testing.T) {
	q := mustFrom(t, nil, "user")
	q, err := q.OrderBy("name", "asc")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "SELECT * FROM user ORDER BY name ASC"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestOrderBy_InvalidDirection(t *testing.T) {
	q := Query{}.Select(nil)
	if _, err := q.OrderBy("name", "sideways"); err == nil {
		t.Fatal("expected validation error")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestSelect_GroupBy(t *testing.T) {
	q := mustFrom(t, []string{"status", "COUNT(*)"}, "user").
		GroupBy("status")
	got, _ := q.ToSurql()
	if want := "SELECT status, COUNT(*) FROM user GROUP BY status"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestSelect_GroupAll(t *testing.T) {
	q := mustFrom(t, []string{"count()"}, "user").GroupAll()
	got, _ := q.ToSurql()
	if want := "SELECT count() FROM user GROUP ALL"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestSelect_LimitOffset(t *testing.T) {
	q := mustFrom(t, nil, "user")
	q, err := q.Limit(10)
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.Offset(20)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "SELECT * FROM user LIMIT 10 START 20"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestLimit_NegativeRejected(t *testing.T) {
	if _, err := (Query{}).Limit(-1); err == nil {
		t.Fatal("expected error")
	}
}

func TestOffset_NegativeRejected(t *testing.T) {
	if _, err := (Query{}).Offset(-1); err == nil {
		t.Fatal("expected error")
	}
}

func TestInsert_Basic(t *testing.T) {
	q, err := Query{}.Insert("user", map[string]any{"name": "Alice", "age": 30})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	// Keys are sorted deterministically.
	if want := "CREATE user CONTENT {age: 30, name: 'Alice'}"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestInsert_InvalidTableRejected(t *testing.T) {
	if _, err := (Query{}).Insert("1bad", map[string]any{"a": 1}); err == nil {
		t.Fatal("expected error")
	}
}

func TestInsert_InvalidFieldRejected(t *testing.T) {
	if _, err := (Query{}).Insert("user", map[string]any{"bad field": 1}); err == nil {
		t.Fatal("expected error")
	}
}

func TestInsert_ReturnFull(t *testing.T) {
	q, err := Query{}.Insert("user", map[string]any{"name": "Bob"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ReturnFull().ToSurql()
	if want := "CREATE user CONTENT {name: 'Bob'} RETURN FULL"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestUpdate_SetAndWhere(t *testing.T) {
	q, err := Query{}.Update("user", map[string]any{"status": "active"})
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.Where("last_login > '2024-01-01'")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "UPDATE user SET status = 'active' WHERE (last_login > '2024-01-01')"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestUpdate_ReturnDiff(t *testing.T) {
	q, err := Query{}.Update("user:alice", map[string]any{"age": 30})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ReturnDiff().ToSurql()
	if want := "UPDATE user:alice SET age = 30 RETURN DIFF"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestUpsert_Basic(t *testing.T) {
	q, err := Query{}.Upsert("user:alice", map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "UPSERT user:alice CONTENT {name: 'Alice'}"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestDelete_WithWhere(t *testing.T) {
	q, err := Query{}.Delete("user")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.Where("age < 18")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "DELETE user WHERE (age < 18)"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestDelete_ReturnBefore(t *testing.T) {
	q, err := Query{}.Delete("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ReturnBefore().ToSurql()
	if want := "DELETE user:alice RETURN BEFORE"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestRelate_StringEndpoints(t *testing.T) {
	q, err := Query{}.Relate("likes", "user:alice", "post:123", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "RELATE user:alice->likes->post:123"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestRelate_WithDataAndReturn(t *testing.T) {
	q, err := Query{}.Relate("likes", "user:a", "post:1", map[string]any{"since": "2024-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ReturnAfter().ToSurql()
	if want := "RELATE user:a->likes->post:1 CONTENT {since: '2024-01-01'} RETURN AFTER"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestRelate_RecordIDEndpoints(t *testing.T) {
	from, _ := types.NewStringRecordID("user", "alice")
	to, _ := types.NewIntRecordID("post", 42)
	q, err := Query{}.Relate("likes", from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "RELATE user:alice->likes->post:42"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestRelate_InvalidEdgeTable(t *testing.T) {
	if _, err := (Query{}).Relate("1bad", "user:a", "post:1", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestImmutability_ChainingReturnsFreshQuery(t *testing.T) {
	base := Query{}.Select(nil)
	a, err := base.FromTable("user")
	if err != nil {
		t.Fatal(err)
	}
	b, err := base.FromTable("post")
	if err != nil {
		t.Fatal(err)
	}
	if a.TableName == b.TableName {
		t.Fatalf("expected independent tables, both %q", a.TableName)
	}
	if base.TableName != "" {
		t.Fatalf("base mutated to %q", base.TableName)
	}
}

func TestImmutability_FieldsSliceSafety(t *testing.T) {
	src := []string{"id"}
	q := Query{}.Select(src)
	src[0] = "MUTATED"
	if q.Fields[0] != "id" {
		t.Fatalf("caller mutation leaked, got %q", q.Fields[0])
	}
}

func TestImmutability_DataMapSafety(t *testing.T) {
	data := map[string]any{"name": "Alice"}
	q, err := Query{}.Insert("user", data)
	if err != nil {
		t.Fatal(err)
	}
	data["name"] = "Bob"
	if q.InsertData["name"] != "Alice" {
		t.Fatalf("caller mutation leaked, got %q", q.InsertData["name"])
	}
}

func TestVectorSearch_NoThreshold(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("documents")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.VectorSearch("embedding", []float64{0.1, 0.2, 0.3}, 10, DistanceCosine, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "SELECT * FROM documents WHERE embedding <|10,COSINE|> [0.1, 0.2, 0.3]"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestVectorSearch_WithThreshold(t *testing.T) {
	thr := 0.7
	q, err := Query{}.Select(nil).FromTable("docs")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.VectorSearch("embedding", []float64{1, 2}, 5, DistanceEuclidean, &thr)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if !strings.Contains(got, "<|5,EUCLIDEAN,0.7|>") {
		t.Errorf("missing threshold operator in %q", got)
	}
}

func TestVectorSearch_EmptyVectorRejected(t *testing.T) {
	q := Query{}.Select(nil)
	if _, err := q.VectorSearch("e", nil, 3, DistanceCosine, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestVectorSearch_KMustBePositive(t *testing.T) {
	q := Query{}.Select(nil)
	if _, err := q.VectorSearch("e", []float64{1}, 0, DistanceCosine, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestHint_Composition(t *testing.T) {
	q := mustFrom(t, nil, "user")
	timeout, err := NewTimeoutHint(30)
	if err != nil {
		t.Fatal(err)
	}
	out, err := q.WithHints(timeout, ExplainShort()).ToSurql()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "/* TIMEOUT 30s */ /* EXPLAIN */\nSELECT") {
		t.Errorf("hint prefix missing in %q", out)
	}
}

func TestHint_IndexConvenienceRequiresTable(t *testing.T) {
	if _, err := (Query{}.Select(nil)).ForceIndex("idx_a"); err == nil {
		t.Fatal("expected error without table")
	}
}

func TestHint_UseIndex(t *testing.T) {
	q, err := (mustFrom(t, nil, "user")).UseIndex("email_idx")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if !strings.Contains(got, "/* USE INDEX user.email_idx */") {
		t.Errorf("hint missing in %q", got)
	}
}

func TestFromTable_InvalidRejected(t *testing.T) {
	if _, err := (Query{}).FromTable("1bad"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := (Query{}).FromTable("a-b"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFromTable_WithRecordID(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "SELECT * FROM user:alice"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestTraverse_AppendsToFrom(t *testing.T) {
	q := mustFrom(t, nil, "user:alice").Traverse("->likes->post")
	got, _ := q.ToSurql()
	if want := "SELECT * FROM user:alice->likes->post"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestSimilarityScore_AddsField(t *testing.T) {
	q := mustFrom(t, []string{"id"}, "chunk").
		SimilarityScore("embedding", []float64{0.1, 0.2}, DistanceCosine, "score")
	got, _ := q.ToSurql()
	want := "SELECT id, vector::similarity::cosine(embedding, [0.1, 0.2]) AS score FROM chunk"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestUnsupportedOperation_Rendered(t *testing.T) {
	q := Query{Operation: Operation("BOGUS")}
	if _, err := q.ToSurql(); err == nil {
		t.Fatal("expected error for unsupported op")
	}
}

func TestUpdate_WithoutDataFails(t *testing.T) {
	q := Query{Operation: OpUpdate, TableName: "user"}
	if _, err := q.ToSurql(); err == nil {
		t.Fatal("expected error")
	}
}

func TestInsert_WithoutDataFails(t *testing.T) {
	q := Query{Operation: OpInsert, TableName: "user"}
	if _, err := q.ToSurql(); err == nil {
		t.Fatal("expected error")
	}
}

func TestRelate_MissingEndpointsFails(t *testing.T) {
	q := Query{Operation: OpRelate, TableName: "likes"}
	if _, err := q.ToSurql(); err == nil {
		t.Fatal("expected error")
	}
}

func TestJoin_AppendsRawClause(t *testing.T) {
	q := mustFrom(t, nil, "user").
		Join("JOIN post ON user.id = post.author")
	got, _ := q.ToSurql()
	if !strings.Contains(got, "JOIN post ON user.id = post.author") {
		t.Errorf("join missing: %q", got)
	}
}

func TestReturnFormat_ToSurql(t *testing.T) {
	cases := map[ReturnFormat]string{
		ReturnNone:   "NONE",
		ReturnDiff:   "DIFF",
		ReturnFull:   "FULL",
		ReturnBefore: "BEFORE",
		ReturnAfter:  "AFTER",
	}
	for f, want := range cases {
		if got := f.ToSurql(); got != want {
			t.Errorf("%v: got %q want %q", f, got, want)
		}
	}
}

func TestWhere_NilRejected(t *testing.T) {
	q := Query{}.Select(nil)
	if _, err := q.Where(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestWhere_UnsupportedTypeRejected(t *testing.T) {
	q := Query{}.Select(nil)
	if _, err := q.Where(123); err == nil {
		t.Fatal("expected error for int condition")
	}
}

func TestQuoteValue_EscapesSpecialChars(t *testing.T) {
	q, err := Query{}.Insert("user", map[string]any{"name": "O'Brien"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if !strings.Contains(got, `O\'Brien`) {
		t.Errorf("escape missing: %q", got)
	}
}

func TestExplain_Hint(t *testing.T) {
	q := mustFrom(t, nil, "user").Explain(true)
	got, _ := q.ToSurql()
	if !strings.Contains(got, "/* EXPLAIN FULL */") {
		t.Errorf("explain full missing: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Full-text search
// ---------------------------------------------------------------------------

func TestFullTextSearch_RendersMatchOperator(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("memory")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.FullTextSearch("content", 1, "insider buying")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "SELECT * FROM memory WHERE content @1@ 'insider buying'"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFullTextSearch_WithScoreAndOrder(t *testing.T) {
	q, err := Query{}.Select(nil).SearchScore(1, "score").FromTable("memory")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.FullTextSearch("content", 1, "form 4")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.OrderBy("score", "DESC")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.Limit(5)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	want := "SELECT *, search::score(1) AS score FROM memory " +
		"WHERE content @1@ 'form 4' ORDER BY score DESC LIMIT 5"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFullTextSearch_EscapesSingleQuotes(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("memory")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.FullTextSearch("content", 0, "o'brien")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := `SELECT * FROM memory WHERE content @0@ 'o\'brien'`; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFullTextSearch_EmptyFieldRejected(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("memory")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.FullTextSearch("", 1, "x"); err == nil {
		t.Fatal("expected error for empty field")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestFullTextSearch_EmptyQueryRejected(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("memory")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.FullTextSearch("content", 1, ""); err == nil {
		t.Fatal("expected error for empty query")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestFullTextSearch_AndVectorBothRenderInWhere(t *testing.T) {
	q, err := Query{}.Select(nil).FromTable("memory")
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.VectorSearch("embedding", []float64{0.1, 0.2}, 5, DistanceCosine, nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err = q.FullTextSearch("content", 1, "term")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if !strings.Contains(got, "embedding <|5,COSINE|> [0.1, 0.2]") {
		t.Errorf("vector predicate missing in %q", got)
	}
	if !strings.Contains(got, "content @1@ 'term'") {
		t.Errorf("fulltext predicate missing in %q", got)
	}
	if !strings.Contains(got, " AND ") {
		t.Errorf("expected AND join in %q", got)
	}
}

func TestSearchScore_ImmutabilityPreserved(t *testing.T) {
	base, err := Query{}.Select(nil).FromTable("memory")
	if err != nil {
		t.Fatal(err)
	}
	scored := base.SearchScore(2, "rel")
	baseSQL, _ := base.ToSurql()
	if baseSQL != "SELECT * FROM memory" {
		t.Errorf("base query mutated: %q", baseSQL)
	}
	scoredSQL, _ := scored.ToSurql()
	if want := "SELECT *, search::score(2) AS rel FROM memory"; scoredSQL != want {
		t.Errorf("got %q want %q", scoredSQL, want)
	}
}
