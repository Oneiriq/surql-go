package query

import (
	"errors"
	"strings"
	"testing"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

func TestHelper_Select(t *testing.T) {
	q := Select(nil)
	if q.Operation != OpSelect {
		t.Errorf("got %v", q.Operation)
	}
	if len(q.Fields) != 1 || q.Fields[0] != "*" {
		t.Errorf("fields: %v", q.Fields)
	}
}

func TestHelper_SelectWithFields(t *testing.T) {
	q := Select([]string{"a", "b"})
	if len(q.Fields) != 2 || q.Fields[0] != "a" || q.Fields[1] != "b" {
		t.Errorf("fields: %v", q.Fields)
	}
}

func TestHelper_FromTable(t *testing.T) {
	q, err := FromTable("user")
	if err != nil {
		t.Fatal(err)
	}
	if q.TableName != "user" {
		t.Errorf("got %q", q.TableName)
	}
}

func TestHelper_FromTable_Invalid(t *testing.T) {
	if _, err := FromTable(""); err == nil {
		t.Fatal("expected empty-table error")
	} else if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
	if _, err := FromTable("1bad"); err == nil {
		t.Fatal("expected syntax error")
	}
}

func TestHelper_Where(t *testing.T) {
	q, err := FromTable("user")
	if err != nil {
		t.Fatal(err)
	}
	q = q.Select(nil)
	q, err = Where(q, "age > 18")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "SELECT * FROM user WHERE (age > 18)"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestHelper_OrderBy(t *testing.T) {
	q, _ := Select(nil).FromTable("user")
	q, err := OrderBy(q, "name", "DESC")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "SELECT * FROM user ORDER BY name DESC"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestHelper_Limit(t *testing.T) {
	q, _ := Select(nil).FromTable("user")
	q, err := Limit(q, 5)
	if err != nil {
		t.Fatal(err)
	}
	if q.LimitValue == nil || *q.LimitValue != 5 {
		t.Errorf("limit: %v", q.LimitValue)
	}
}

func TestHelper_Offset(t *testing.T) {
	q, _ := Select(nil).FromTable("user")
	q, err := Offset(q, 3)
	if err != nil {
		t.Fatal(err)
	}
	if q.OffsetValue == nil || *q.OffsetValue != 3 {
		t.Errorf("offset: %v", q.OffsetValue)
	}
}

func TestHelper_Insert(t *testing.T) {
	q, err := Insert("user", map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	if q.Operation != OpInsert {
		t.Errorf("op: %v", q.Operation)
	}
	if q.TableName != "user" {
		t.Errorf("table: %q", q.TableName)
	}
}

func TestHelper_Update(t *testing.T) {
	q, err := Update("user:alice", map[string]any{"age": 30})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "UPDATE user:alice SET age = 30"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestHelper_Upsert(t *testing.T) {
	q, err := Upsert("user:alice", map[string]any{"status": "active"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "UPSERT user:alice CONTENT {status: 'active'}"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestHelper_Delete(t *testing.T) {
	q, err := Delete("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "DELETE user:alice"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestHelper_Relate(t *testing.T) {
	q, err := Relate("likes", "user:a", "post:1", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if want := "RELATE user:a->likes->post:1"; got != want {
		t.Errorf("got %q", got)
	}
}

func TestHelper_VectorSearchQuery(t *testing.T) {
	q, err := VectorSearchQuery(
		"documents", "embedding", []float64{0.1, 0.2, 0.3}, 10,
		DistanceCosine, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if !strings.Contains(got, "<|10,COSINE|>") {
		t.Errorf("missing vector op: %q", got)
	}
}

func TestHelper_VectorSearchQuery_WithThreshold(t *testing.T) {
	thr := 0.5
	q, err := VectorSearchQuery(
		"documents", "embedding", []float64{0.1, 0.2, 0.3}, 10,
		DistanceCosine, []string{"id", "text"}, &thr,
	)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if !strings.Contains(got, "<|10,COSINE,0.5|>") || !strings.Contains(got, "SELECT id, text") {
		t.Errorf("got %q", got)
	}
}

func TestHelper_SimilaritySearchQuery(t *testing.T) {
	q, err := SimilaritySearchQuery(
		"chunk", "embedding", []float64{0.1, 0.2}, 5,
		DistanceCosine, nil, []string{"id"}, "score",
	)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := q.ToSurql()
	if !strings.Contains(got, "vector::similarity::cosine(embedding, [0.1, 0.2]) AS score") {
		t.Errorf("missing similarity projection: %q", got)
	}
	if !strings.Contains(got, "<|5,COSINE|>") {
		t.Errorf("missing vector operator: %q", got)
	}
}

func TestReturnFormat_Values(t *testing.T) {
	if ReturnNone.String() != "NONE" {
		t.Error("none")
	}
	if ReturnAfter.ToSurql() != "AFTER" {
		t.Error("after")
	}
}

func TestVectorDistance_Constants(t *testing.T) {
	cases := []VectorDistanceType{
		DistanceCosine, DistanceEuclidean, DistanceManhattan, DistanceChebyshev,
		DistanceMinkowski, DistanceHamming, DistanceJaccard, DistancePearson,
		DistanceMahalanobis,
	}
	for _, c := range cases {
		if string(c) == "" {
			t.Errorf("empty distance constant")
		}
	}
}

func TestValidateIdentifier_Cases(t *testing.T) {
	valid := []string{"user", "_hidden", "a1", "user_table"}
	for _, v := range valid {
		if err := validateIdentifier(v, "test"); err != nil {
			t.Errorf("valid %q rejected: %v", v, err)
		}
	}
	invalid := []string{"", "1user", "user-table", "user.name", "user table"}
	for _, v := range invalid {
		if err := validateIdentifier(v, "test"); err == nil {
			t.Errorf("invalid %q accepted", v)
		}
	}
}

func TestSplitTablePart(t *testing.T) {
	if splitTablePart("user") != "user" {
		t.Error("plain")
	}
	if splitTablePart("user:alice") != "user" {
		t.Error("record id")
	}
}
