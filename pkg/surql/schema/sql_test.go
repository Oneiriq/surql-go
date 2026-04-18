package schema

import (
	stdErrors "errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// ---------- GenerateTableSQL ----------

func TestGenerateTableSQL_SchemafullMinimal(t *testing.T) {
	tbl := NewTable("user", WithMode(TableModeSchemafull))
	stmts := GenerateTableSQL(tbl, false)
	if len(stmts) != 1 {
		t.Fatalf("len(stmts) = %d, want 1", len(stmts))
	}
	if stmts[0] != "DEFINE TABLE user SCHEMAFULL;" {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
}

func TestGenerateTableSQL_Schemaless(t *testing.T) {
	tbl := NewTable("log", WithMode(TableModeSchemaless))
	stmts := GenerateTableSQL(tbl, false)
	if stmts[0] != "DEFINE TABLE log SCHEMALESS;" {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
}

func TestGenerateTableSQL_WithFields(t *testing.T) {
	tbl := NewTable("user",
		WithMode(TableModeSchemafull),
		WithFields(StringField("name"), IntField("age")),
	)
	stmts := GenerateTableSQL(tbl, false)
	if !containsSubstring(stmts, "DEFINE FIELD name ON TABLE user TYPE string") {
		t.Errorf("missing name field; stmts = %v", stmts)
	}
	if !containsSubstring(stmts, "DEFINE FIELD age ON TABLE user TYPE int") {
		t.Errorf("missing age field; stmts = %v", stmts)
	}
}

func TestGenerateTableSQL_WithFieldAssertion(t *testing.T) {
	tbl := NewTable("user",
		WithFields(StringField("email",
			WithAssertion("string::is::email($value)"))),
	)
	stmts := GenerateTableSQL(tbl, false)
	if !containsSubstring(stmts, "ASSERT string::is::email($value)") {
		t.Errorf("missing assertion; stmts = %v", stmts)
	}
}

func TestGenerateTableSQL_WithFieldDefault(t *testing.T) {
	tbl := NewTable("event",
		WithFields(DatetimeField("created_at", WithDefault("time::now()"))),
	)
	stmts := GenerateTableSQL(tbl, false)
	if !containsSubstring(stmts, "DEFAULT time::now()") {
		t.Errorf("missing default; stmts = %v", stmts)
	}
}

func TestGenerateTableSQL_WithReadOnlyField(t *testing.T) {
	tbl := NewTable("event",
		WithFields(DatetimeField("created_at", WithReadOnly(true))),
	)
	stmts := GenerateTableSQL(tbl, false)
	if !containsSubstring(stmts, "READONLY") {
		t.Errorf("missing READONLY; stmts = %v", stmts)
	}
}

func TestGenerateTableSQL_WithUniqueIndex(t *testing.T) {
	tbl := NewTable("user",
		WithIndexes(UniqueIndex("email_idx", []string{"email"})),
	)
	stmts := GenerateTableSQL(tbl, false)
	want := "DEFINE INDEX email_idx ON TABLE user COLUMNS email UNIQUE;"
	if !hasExact(stmts, want) {
		t.Errorf("expected %q in stmts = %v", want, stmts)
	}
}

func TestGenerateTableSQL_WithStandardIndex(t *testing.T) {
	tbl := NewTable("post",
		WithIndexes(NewIndex("title_idx", []string{"title"}, IndexTypeStandard)),
	)
	stmts := GenerateTableSQL(tbl, false)
	want := "DEFINE INDEX title_idx ON TABLE post COLUMNS title;"
	if !hasExact(stmts, want) {
		t.Errorf("expected %q in stmts = %v", want, stmts)
	}
}

func TestGenerateTableSQL_WithEvent(t *testing.T) {
	tbl := NewTable("user",
		WithEvents(NewEvent("email_changed",
			"$before.email != $after.email",
			"CREATE audit_log SET user = $value.id")),
	)
	stmts := GenerateTableSQL(tbl, false)
	if !containsSubstring(stmts, "DEFINE EVENT email_changed ON TABLE user") {
		t.Errorf("missing event header; stmts = %v", stmts)
	}
	if !containsSubstring(stmts, "WHEN $before.email != $after.email") {
		t.Errorf("missing event WHEN; stmts = %v", stmts)
	}
}

func TestGenerateTableSQL_WithPermissions(t *testing.T) {
	tbl := NewTable("user",
		WithTablePermissions(map[string]string{"select": "$auth.id = id"}),
	)
	stmts := GenerateTableSQL(tbl, false)
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "FOR SELECT") && strings.Contains(s, "$auth.id = id") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected FOR SELECT ... $auth.id = id; stmts = %v", stmts)
	}
}

func TestGenerateTableSQL_MinimalSingleStatement(t *testing.T) {
	tbl := NewTable("empty")
	stmts := GenerateTableSQL(tbl, false)
	if len(stmts) != 1 {
		t.Errorf("len(stmts) = %d, want 1", len(stmts))
	}
}

func TestGenerateTableSQL_FirstStatementIsTable(t *testing.T) {
	tbl := NewTable("user",
		WithFields(StringField("name")),
		WithIndexes(UniqueIndex("name_idx", []string{"name"})),
	)
	stmts := GenerateTableSQL(tbl, false)
	if !strings.HasPrefix(stmts[0], "DEFINE TABLE") {
		t.Errorf("first stmt = %q, want prefix DEFINE TABLE", stmts[0])
	}
}

// ---------- GenerateEdgeSQL ----------

func TestGenerateEdgeSQL_RelationWithFromTo(t *testing.T) {
	e := TypedEdge("likes", "user", "post")
	stmts, err := GenerateEdgeSQL(e, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "DEFINE TABLE likes TYPE RELATION FROM user TO post;"
	if stmts[0] != want {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], want)
	}
}

func TestGenerateEdgeSQL_Schemafull(t *testing.T) {
	e := NewEdge("entity_relation", WithEdgeMode(EdgeModeSchemafull))
	stmts, err := GenerateEdgeSQL(e, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "DEFINE TABLE entity_relation SCHEMAFULL;"
	if stmts[0] != want {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], want)
	}
}

func TestGenerateEdgeSQL_Schemaless(t *testing.T) {
	e := NewEdge("loose_rel", WithEdgeMode(EdgeModeSchemaless))
	stmts, err := GenerateEdgeSQL(e, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "DEFINE TABLE loose_rel SCHEMALESS;"
	if stmts[0] != want {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], want)
	}
}

func TestGenerateEdgeSQL_WithFields(t *testing.T) {
	e := TypedEdge("likes", "user", "post",
		WithEdgeFields(DatetimeField("created_at", WithDefault("time::now()"))),
	)
	stmts, err := GenerateEdgeSQL(e, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !containsSubstring(stmts, "DEFINE FIELD created_at ON TABLE likes TYPE datetime") {
		t.Errorf("missing field; stmts = %v", stmts)
	}
}

func TestGenerateEdgeSQL_RelationMissingFrom(t *testing.T) {
	e := NewEdge("likes", WithToTable("post"))
	_, err := GenerateEdgeSQL(e, false)
	if err == nil {
		t.Fatal("expected error for missing from_table")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestGenerateEdgeSQL_RelationMissingTo(t *testing.T) {
	e := NewEdge("likes", WithFromTable("user"))
	_, err := GenerateEdgeSQL(e, false)
	if err == nil {
		t.Fatal("expected error for missing to_table")
	}
}

func TestGenerateEdgeSQL_RelationMissingBoth(t *testing.T) {
	e := NewEdge("likes")
	_, err := GenerateEdgeSQL(e, false)
	if err == nil {
		t.Fatal("expected error for missing both tables")
	}
}

func TestGenerateEdgeSQL_SchemafullNoTablesRequired(t *testing.T) {
	e := NewEdge("entity_rel", WithEdgeMode(EdgeModeSchemafull))
	stmts, err := GenerateEdgeSQL(e, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(stmts) < 1 {
		t.Errorf("want >=1 stmts, got %d", len(stmts))
	}
}

func TestGenerateEdgeSQL_FirstStatementIsTable(t *testing.T) {
	e := BidirectionalEdge("follows", "user")
	stmts, err := GenerateEdgeSQL(e, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.HasPrefix(stmts[0], "DEFINE TABLE") {
		t.Errorf("first stmt = %q", stmts[0])
	}
}

// ---------- GenerateAccessSQL ----------

func TestGenerateAccessSQL_JWT(t *testing.T) {
	a := JwtAccess("api", JwtConfig{Key: "secret"})
	stmts := GenerateAccessSQL(a)
	if len(stmts) != 1 {
		t.Fatalf("len(stmts) = %d, want 1", len(stmts))
	}
	want := "DEFINE ACCESS api ON DATABASE TYPE JWT ALGORITHM HS256 KEY 'secret';"
	if stmts[0] != want {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], want)
	}
}

func TestGenerateAccessSQL_Record(t *testing.T) {
	a := RecordAccess("users", RecordAccessConfig{
		Signup: "CREATE user SET email = $email",
		Signin: "SELECT * FROM user WHERE email = $email",
	})
	stmts := GenerateAccessSQL(a)
	if !strings.Contains(stmts[0], "SIGNUP (CREATE user SET email = $email)") {
		t.Errorf("missing SIGNUP; got %q", stmts[0])
	}
	if !strings.Contains(stmts[0], "SIGNIN (SELECT * FROM user WHERE email = $email)") {
		t.Errorf("missing SIGNIN; got %q", stmts[0])
	}
}

// ---------- GenerateSchemaSQL (registry) ----------

func TestGenerateSchemaSQL_CombinesTablesAndEdges(t *testing.T) {
	r := NewSchemaRegistry()
	if err := r.RegisterTable(NewTable("user", WithMode(TableModeSchemafull))); err != nil {
		t.Fatalf("register table: %v", err)
	}
	if err := r.RegisterEdge(TypedEdge("likes", "user", "post")); err != nil {
		t.Fatalf("register edge: %v", err)
	}
	sql, err := GenerateSchemaSQL(r, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(sql, "DEFINE TABLE user SCHEMAFULL") {
		t.Errorf("missing user table; sql = %q", sql)
	}
	if !strings.Contains(sql, "DEFINE TABLE likes TYPE RELATION FROM user TO post") {
		t.Errorf("missing likes edge; sql = %q", sql)
	}
}

func TestGenerateSchemaSQL_TablesOnly(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user"))
	sql, err := GenerateSchemaSQL(r, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(sql, "DEFINE TABLE user") {
		t.Errorf("sql missing user; sql = %q", sql)
	}
}

func TestGenerateSchemaSQL_EdgesOnly(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterEdge(BidirectionalEdge("follows", "user"))
	sql, err := GenerateSchemaSQL(r, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(sql, "DEFINE TABLE follows TYPE RELATION") {
		t.Errorf("missing edge; sql = %q", sql)
	}
}

func TestGenerateSchemaSQL_EmptyRegistryReturnsEmpty(t *testing.T) {
	r := NewSchemaRegistry()
	sql, err := GenerateSchemaSQL(r, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sql != "" {
		t.Errorf("expected empty string, got %q", sql)
	}
}

func TestGenerateSchemaSQL_NilRegistryReturnsEmpty(t *testing.T) {
	sql, err := GenerateSchemaSQL(nil, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sql != "" {
		t.Errorf("expected empty string, got %q", sql)
	}
}

func TestGenerateSchemaSQL_MultipleTablesSorted(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user"))
	_ = r.RegisterTable(NewTable("post"))
	sql, err := GenerateSchemaSQL(r, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// sorted: post should appear before user
	postIdx := strings.Index(sql, "DEFINE TABLE post")
	userIdx := strings.Index(sql, "DEFINE TABLE user")
	if postIdx == -1 || userIdx == -1 {
		t.Fatalf("missing tables; sql = %q", sql)
	}
	if postIdx > userIdx {
		t.Errorf("tables not sorted: post index %d > user index %d", postIdx, userIdx)
	}
}

func TestGenerateSchemaSQL_TablesSeparatedByBlankLines(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user"))
	_ = r.RegisterTable(NewTable("post"))
	sql, err := GenerateSchemaSQL(r, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	hasBlank := false
	for _, line := range strings.Split(sql, "\n") {
		if line == "" {
			hasBlank = true
			break
		}
	}
	if !hasBlank {
		t.Errorf("expected blank line separator; sql = %q", sql)
	}
}

func TestGenerateSchemaSQL_EdgeValidationError(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterEdge(NewEdge("bad", WithFromTable("user")))
	_, err := GenerateSchemaSQL(r, false)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("want ErrValidation, got %v", err)
	}
}

func TestGenerateSchemaSQLFromSlices_PreservesOrder(t *testing.T) {
	tables := []TableDefinition{NewTable("z"), NewTable("a")}
	sql, err := GenerateSchemaSQLFromSlices(tables, nil, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	zIdx := strings.Index(sql, "DEFINE TABLE z")
	aIdx := strings.Index(sql, "DEFINE TABLE a")
	if zIdx == -1 || aIdx == -1 {
		t.Fatalf("missing tables; sql = %q", sql)
	}
	if zIdx > aIdx {
		t.Errorf("slice order not preserved: z at %d, a at %d", zIdx, aIdx)
	}
}

// ---------- IF NOT EXISTS ----------

func TestIfNotExists_TableDefinition(t *testing.T) {
	tbl := NewTable("user", WithMode(TableModeSchemafull))
	stmts := GenerateTableSQL(tbl, true)
	want := "DEFINE TABLE IF NOT EXISTS user SCHEMAFULL;"
	if stmts[0] != want {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], want)
	}
}

func TestIfNotExists_Field(t *testing.T) {
	tbl := NewTable("user",
		WithMode(TableModeSchemafull),
		WithFields(StringField("name")),
	)
	stmts := GenerateTableSQL(tbl, true)
	want := "DEFINE FIELD IF NOT EXISTS name ON TABLE user TYPE string;"
	if !hasExact(stmts, want) {
		t.Errorf("missing %q; stmts = %v", want, stmts)
	}
}

func TestIfNotExists_Index(t *testing.T) {
	tbl := NewTable("user",
		WithIndexes(UniqueIndex("email_idx", []string{"email"})),
	)
	stmts := GenerateTableSQL(tbl, true)
	want := "DEFINE INDEX IF NOT EXISTS email_idx ON TABLE user COLUMNS email UNIQUE;"
	if !hasExact(stmts, want) {
		t.Errorf("missing %q; stmts = %v", want, stmts)
	}
}

func TestIfNotExists_StandardIndex(t *testing.T) {
	tbl := NewTable("post",
		WithIndexes(NewIndex("title_idx", []string{"title"}, IndexTypeStandard)),
	)
	stmts := GenerateTableSQL(tbl, true)
	want := "DEFINE INDEX IF NOT EXISTS title_idx ON TABLE post COLUMNS title;"
	if !hasExact(stmts, want) {
		t.Errorf("missing %q; stmts = %v", want, stmts)
	}
}

func TestIfNotExists_Event(t *testing.T) {
	tbl := NewTable("user",
		WithEvents(NewEvent("email_changed",
			"$before.email != $after.email",
			"CREATE audit_log SET user = $value.id")),
	)
	stmts := GenerateTableSQL(tbl, true)
	found := false
	for _, s := range stmts {
		if strings.HasPrefix(s, "DEFINE EVENT IF NOT EXISTS email_changed ON TABLE user") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("missing DEFINE EVENT IF NOT EXISTS; stmts = %v", stmts)
	}
}

func TestIfNotExists_EdgeRelation(t *testing.T) {
	e := TypedEdge("likes", "user", "post")
	stmts, err := GenerateEdgeSQL(e, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "DEFINE TABLE IF NOT EXISTS likes TYPE RELATION FROM user TO post;"
	if stmts[0] != want {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], want)
	}
}

func TestIfNotExists_EdgeSchemafull(t *testing.T) {
	e := NewEdge("entity_relation", WithEdgeMode(EdgeModeSchemafull))
	stmts, err := GenerateEdgeSQL(e, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "DEFINE TABLE IF NOT EXISTS entity_relation SCHEMAFULL;"
	if stmts[0] != want {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], want)
	}
}

func TestIfNotExists_EdgeSchemaless(t *testing.T) {
	e := NewEdge("loose_rel", WithEdgeMode(EdgeModeSchemaless))
	stmts, err := GenerateEdgeSQL(e, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "DEFINE TABLE IF NOT EXISTS loose_rel SCHEMALESS;"
	if stmts[0] != want {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], want)
	}
}

func TestIfNotExists_EdgeFields(t *testing.T) {
	e := TypedEdge("likes", "user", "post",
		WithEdgeFields(DatetimeField("created_at", WithDefault("time::now()"))),
	)
	stmts, err := GenerateEdgeSQL(e, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !containsSubstring(stmts, "DEFINE FIELD IF NOT EXISTS created_at ON TABLE likes") {
		t.Errorf("missing IF NOT EXISTS field; stmts = %v", stmts)
	}
}

func TestIfNotExists_SchemaSQL(t *testing.T) {
	r := NewSchemaRegistry()
	_ = r.RegisterTable(NewTable("user", WithMode(TableModeSchemafull)))
	_ = r.RegisterEdge(TypedEdge("likes", "user", "post"))
	sql, err := GenerateSchemaSQL(r, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(sql, "DEFINE TABLE IF NOT EXISTS user SCHEMAFULL") {
		t.Errorf("missing IF NOT EXISTS table; sql = %q", sql)
	}
	if !strings.Contains(sql, "DEFINE TABLE IF NOT EXISTS likes TYPE RELATION FROM user TO post") {
		t.Errorf("missing IF NOT EXISTS edge; sql = %q", sql)
	}
}

func TestIfNotExistsDefault_False(t *testing.T) {
	tbl := NewTable("user",
		WithMode(TableModeSchemafull),
		WithFields(StringField("name")),
		WithIndexes(UniqueIndex("email_idx", []string{"email"})),
		WithEvents(NewEvent("email_changed",
			"$before.email != $after.email",
			"CREATE audit_log SET user = $value.id")),
	)
	stmts := GenerateTableSQL(tbl, false)
	if stmts[0] != "DEFINE TABLE user SCHEMAFULL;" {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
	for _, s := range stmts {
		if strings.Contains(s, "IF NOT EXISTS") {
			t.Errorf("did not expect IF NOT EXISTS; got %q", s)
		}
	}
}

// ---------- helpers ----------

func containsSubstring(stmts []string, needle string) bool {
	for _, s := range stmts {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func hasExact(stmts []string, needle string) bool {
	for _, s := range stmts {
		if s == needle {
			return true
		}
	}
	return false
}
