package schema

import (
	stdErrors "errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestEdgeMode_IsValid(t *testing.T) {
	for _, m := range []EdgeMode{EdgeModeRelation, EdgeModeSchemafull, EdgeModeSchemaless} {
		if !m.IsValid() {
			t.Errorf("%q should be valid", string(m))
		}
	}
	if EdgeMode("bogus").IsValid() {
		t.Error("bogus edge mode")
	}
}

func TestNewEdge_DefaultMode(t *testing.T) {
	e := NewEdge("likes")
	if e.Mode != EdgeModeRelation {
		t.Errorf("default mode = %q", string(e.Mode))
	}
}

func TestEdgeToSurql_Relation(t *testing.T) {
	e := NewEdge("likes", WithFromTable("user"), WithToTable("post"))
	got := e.ToSurql()
	want := "DEFINE TABLE likes TYPE RELATION FROM user TO post;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestEdgeToSurql_Schemafull(t *testing.T) {
	e := NewEdge("entity_rel", WithEdgeMode(EdgeModeSchemafull))
	got := e.ToSurql()
	want := "DEFINE TABLE entity_rel SCHEMAFULL;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestEdgeToSurql_Schemaless(t *testing.T) {
	e := NewEdge("loose_rel", WithEdgeMode(EdgeModeSchemaless))
	got := e.ToSurql()
	want := "DEFINE TABLE loose_rel SCHEMALESS;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestEdgeToSurqlIfNotExists(t *testing.T) {
	e := NewEdge("likes", WithFromTable("user"), WithToTable("post"))
	got := e.ToSurqlIfNotExists()
	want := "DEFINE TABLE IF NOT EXISTS likes TYPE RELATION FROM user TO post;"
	if got != want {
		t.Errorf("ToSurqlIfNotExists() = %q, want %q", got, want)
	}
}

func TestEdgeStatements_MissingFromTable(t *testing.T) {
	e := NewEdge("likes", WithToTable("post"))
	_, err := e.ToSurqlStatements()
	if err == nil {
		t.Fatal("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "requires both from_table and to_table") {
		t.Errorf("err message = %q", err.Error())
	}
}

func TestEdgeStatements_MissingToTable(t *testing.T) {
	e := NewEdge("likes", WithFromTable("user"))
	_, err := e.ToSurqlStatements()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEdgeStatements_MissingBoth(t *testing.T) {
	e := NewEdge("likes")
	_, err := e.ToSurqlStatements()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEdgeStatements_SchemafullNoTablesRequired(t *testing.T) {
	e := NewEdge("entity_rel", WithEdgeMode(EdgeModeSchemafull))
	stmts, err := e.ToSurqlStatements()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 1 {
		t.Errorf("len = %d, want 1", len(stmts))
	}
}

func TestEdgeStatements_WithFields(t *testing.T) {
	e := NewEdge("likes",
		WithFromTable("user"),
		WithToTable("post"),
		WithEdgeFields(DatetimeField("created_at", WithDefault("time::now()"))),
	)
	stmts, err := e.ToSurqlStatements()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "DEFINE FIELD created_at ON TABLE likes TYPE datetime") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected field stmt: %v", stmts)
	}
}

func TestEdgeStatements_StartsWithDefineTable(t *testing.T) {
	e := NewEdge("follows", WithFromTable("user"), WithToTable("user"))
	stmts, err := e.ToSurqlStatements()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(stmts[0], "DEFINE TABLE") {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
}

func TestEdgeStatements_IfNotExists(t *testing.T) {
	e := NewEdge("likes",
		WithFromTable("user"),
		WithToTable("post"),
		WithEdgeFields(DatetimeField("created_at", WithDefault("time::now()"))),
	)
	stmts, err := e.ToSurqlStatementsIfNotExists()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if stmts[0] != "DEFINE TABLE IF NOT EXISTS likes TYPE RELATION FROM user TO post;" {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "DEFINE FIELD IF NOT EXISTS created_at ON TABLE likes") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected IF NOT EXISTS field stmt: %v", stmts)
	}
}

func TestEdgeStatements_SchemafullIfNotExists(t *testing.T) {
	e := NewEdge("entity_rel", WithEdgeMode(EdgeModeSchemafull))
	stmts, err := e.ToSurqlStatementsIfNotExists()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if stmts[0] != "DEFINE TABLE IF NOT EXISTS entity_rel SCHEMAFULL;" {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
}

func TestEdgeStatements_SchemalessIfNotExists(t *testing.T) {
	e := NewEdge("loose_rel", WithEdgeMode(EdgeModeSchemaless))
	stmts, err := e.ToSurqlStatementsIfNotExists()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if stmts[0] != "DEFINE TABLE IF NOT EXISTS loose_rel SCHEMALESS;" {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
}

func TestBidirectionalEdge(t *testing.T) {
	e := BidirectionalEdge("follows", "user")
	if e.FromTable != "user" || e.ToTable != "user" {
		t.Errorf("from=%q to=%q", e.FromTable, e.ToTable)
	}
	if e.Mode != EdgeModeRelation {
		t.Errorf("mode = %q", string(e.Mode))
	}
}

func TestTypedEdge(t *testing.T) {
	e := TypedEdge("authored", "user", "post")
	if e.FromTable != "user" || e.ToTable != "post" {
		t.Errorf("from=%q to=%q", e.FromTable, e.ToTable)
	}
}

func TestEdgeValidate_EmptyName(t *testing.T) {
	e := EdgeDefinition{Mode: EdgeModeSchemafull}
	if err := e.Validate(); err == nil {
		t.Error("empty name should fail")
	}
}

func TestEdgeValidate_InvalidMode(t *testing.T) {
	e := EdgeDefinition{Name: "x", Mode: EdgeMode("bogus")}
	if err := e.Validate(); err == nil {
		t.Error("bogus mode should fail")
	}
}

func TestEdgeValidate_RelationRequiresTables(t *testing.T) {
	e := NewEdge("likes")
	if err := e.Validate(); err == nil {
		t.Error("RELATION without tables should fail validation")
	}
}

func TestEdgeValidate_PropagatesField(t *testing.T) {
	e := NewEdge("likes",
		WithFromTable("user"), WithToTable("post"),
		WithEdgeFields(NewField("1bad", FieldTypeString)),
	)
	if err := e.Validate(); err == nil {
		t.Error("expected error from invalid field")
	}
}

func TestEdgePermissions_MapCopyIsolation(t *testing.T) {
	perms := map[string]string{"create": "$auth.id = in"}
	e := NewEdge("likes",
		WithFromTable("u"), WithToTable("p"),
		WithEdgePermissions(perms),
	)
	perms["create"] = "MUTATED"
	if e.Permissions["create"] != "$auth.id = in" {
		t.Errorf("expected copied map; got %q", e.Permissions["create"])
	}
}

func TestEdgeStatements_WithPermissions(t *testing.T) {
	e := NewEdge("follows",
		WithFromTable("user"), WithToTable("user"),
		WithEdgePermissions(map[string]string{"create": "$auth.id = in"}),
	)
	stmts, err := e.ToSurqlStatements()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "FOR CREATE") && strings.Contains(s, "$auth.id = in") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected permissions stmt: %v", stmts)
	}
}
