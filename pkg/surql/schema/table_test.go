package schema

import (
	stdErrors "errors"
	"strings"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestTableMode_IsValid(t *testing.T) {
	for _, m := range []TableMode{TableModeSchemafull, TableModeSchemaless, TableModeDrop} {
		if !m.IsValid() {
			t.Errorf("%q should be valid", string(m))
		}
	}
	if TableMode("bogus").IsValid() {
		t.Error("bogus mode should not be valid")
	}
}

func TestNewTable_DefaultMode(t *testing.T) {
	t1 := NewTable("user")
	if t1.Mode != TableModeSchemafull {
		t.Errorf("default mode = %q, want SCHEMAFULL", string(t1.Mode))
	}
}

func TestTableToSurql_Schemafull(t *testing.T) {
	tbl := NewTable("user", WithMode(TableModeSchemafull))
	got := tbl.ToSurql()
	want := "DEFINE TABLE user SCHEMAFULL;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestTableToSurql_Schemaless(t *testing.T) {
	tbl := NewTable("log", WithMode(TableModeSchemaless))
	got := tbl.ToSurql()
	want := "DEFINE TABLE log SCHEMALESS;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestTableToSurql_Drop(t *testing.T) {
	tbl := NewTable("temp", WithMode(TableModeDrop))
	got := tbl.ToSurql()
	want := "DEFINE TABLE temp DROP;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestTableToSurqlIfNotExists(t *testing.T) {
	tbl := NewTable("user", WithMode(TableModeSchemafull))
	got := tbl.ToSurqlIfNotExists()
	want := "DEFINE TABLE IF NOT EXISTS user SCHEMAFULL;"
	if got != want {
		t.Errorf("ToSurqlIfNotExists() = %q, want %q", got, want)
	}
}

func TestTableStatements_Minimal(t *testing.T) {
	tbl := NewTable("empty")
	stmts := tbl.ToSurqlStatements()
	if len(stmts) != 1 {
		t.Fatalf("len(stmts) = %d, want 1", len(stmts))
	}
	if !strings.HasPrefix(stmts[0], "DEFINE TABLE") {
		t.Errorf("first stmt = %q", stmts[0])
	}
}

func TestTableStatements_WithFields(t *testing.T) {
	tbl := NewTable("user",
		WithFields(StringField("name"), IntField("age")),
	)
	stmts := tbl.ToSurqlStatements()
	if len(stmts) != 3 {
		t.Fatalf("len(stmts) = %d, want 3", len(stmts))
	}
	if !strings.HasPrefix(stmts[0], "DEFINE TABLE user") {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
	if stmts[1] != "DEFINE FIELD name ON TABLE user TYPE string;" {
		t.Errorf("stmts[1] = %q", stmts[1])
	}
	if stmts[2] != "DEFINE FIELD age ON TABLE user TYPE int;" {
		t.Errorf("stmts[2] = %q", stmts[2])
	}
}

func TestTableStatements_WithAssertion(t *testing.T) {
	tbl := NewTable("user",
		WithFields(StringField("email", WithAssertion("string::is::email($value)"))),
	)
	stmts := tbl.ToSurqlStatements()
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "ASSERT string::is::email($value)") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ASSERT clause in statements: %v", stmts)
	}
}

func TestTableStatements_WithDefault(t *testing.T) {
	tbl := NewTable("event",
		WithFields(DatetimeField("created_at", WithDefault("time::now()"))),
	)
	stmts := tbl.ToSurqlStatements()
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "DEFAULT time::now()") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected DEFAULT clause: %v", stmts)
	}
}

func TestTableStatements_WithReadonly(t *testing.T) {
	tbl := NewTable("event",
		WithFields(DatetimeField("created_at", WithReadOnly(true))),
	)
	stmts := tbl.ToSurqlStatements()
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "READONLY") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected READONLY clause: %v", stmts)
	}
}

func TestTableStatements_WithUniqueIndex(t *testing.T) {
	tbl := NewTable("user",
		WithIndexes(UniqueIndex("email_idx", []string{"email"})),
	)
	stmts := tbl.ToSurqlStatements()
	found := false
	for _, s := range stmts {
		if s == "DEFINE INDEX email_idx ON TABLE user COLUMNS email UNIQUE;" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unique index statement: %v", stmts)
	}
}

func TestTableStatements_WithStandardIndex(t *testing.T) {
	tbl := NewTable("post",
		WithIndexes(NewIndex("title_idx", []string{"title"}, IndexTypeStandard)),
	)
	stmts := tbl.ToSurqlStatements()
	found := false
	for _, s := range stmts {
		if s == "DEFINE INDEX title_idx ON TABLE post COLUMNS title;" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected standard index statement: %v", stmts)
	}
}

func TestTableStatements_WithSearchIndex(t *testing.T) {
	tbl := NewTable("post",
		WithIndexes(SearchIndex("content_search", []string{"title", "content"})),
	)
	stmts := tbl.ToSurqlStatements()
	found := false
	for _, s := range stmts {
		if s == "DEFINE INDEX content_search ON TABLE post COLUMNS title, content SEARCH ANALYZER ascii;" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected search index statement: %v", stmts)
	}
}

func TestTableStatements_WithEvent(t *testing.T) {
	tbl := NewTable("user",
		WithEvents(NewEvent(
			"email_changed",
			"$before.email != $after.email",
			"CREATE audit_log SET user = $value.id",
		)),
	)
	stmts := tbl.ToSurqlStatements()
	want := "DEFINE EVENT email_changed ON TABLE user WHEN $before.email != $after.email THEN CREATE audit_log SET user = $value.id;"
	found := false
	for _, s := range stmts {
		if s == want {
			found = true
		}
	}
	if !found {
		t.Errorf("expected event statement %q, got %v", want, stmts)
	}
}

func TestTableStatements_WithPermissions(t *testing.T) {
	tbl := NewTable("user",
		WithTablePermissions(map[string]string{"select": "$auth.id = id"}),
	)
	stmts := tbl.ToSurqlStatements()
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "FOR SELECT") && strings.Contains(s, "$auth.id = id") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected permissions statement: %v", stmts)
	}
}

func TestTableStatements_IfNotExists(t *testing.T) {
	tbl := NewTable("user",
		WithMode(TableModeSchemafull),
		WithFields(StringField("name")),
		WithIndexes(UniqueIndex("email_idx", []string{"email"})),
		WithEvents(NewEvent("e", "$cond", "DO")),
	)
	stmts := tbl.ToSurqlStatementsIfNotExists()
	if stmts[0] != "DEFINE TABLE IF NOT EXISTS user SCHEMAFULL;" {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
	found := 0
	for _, s := range stmts {
		if strings.Contains(s, "DEFINE FIELD IF NOT EXISTS name") {
			found++
		}
		if strings.Contains(s, "DEFINE INDEX IF NOT EXISTS email_idx") {
			found++
		}
		if strings.Contains(s, "DEFINE EVENT IF NOT EXISTS e") {
			found++
		}
	}
	if found != 3 {
		t.Errorf("expected 3 IF NOT EXISTS matches, got %d: %v", found, stmts)
	}
}

func TestTableStatements_DefaultOmitsIfNotExists(t *testing.T) {
	tbl := NewTable("user",
		WithFields(StringField("name")),
		WithIndexes(UniqueIndex("email_idx", []string{"email"})),
		WithEvents(NewEvent("e", "$cond", "DO")),
	)
	stmts := tbl.ToSurqlStatements()
	for _, s := range stmts {
		if strings.Contains(s, "IF NOT EXISTS") {
			t.Errorf("unexpected IF NOT EXISTS in %q", s)
		}
	}
}

func TestIndexType_IsValid(t *testing.T) {
	for _, ty := range []IndexType{
		IndexTypeStandard, IndexTypeUnique, IndexTypeSearch,
		IndexTypeMTree, IndexTypeHNSW,
	} {
		if !ty.IsValid() {
			t.Errorf("%q should be valid", string(ty))
		}
	}
	if IndexType("X").IsValid() {
		t.Error("bogus IndexType")
	}
}

func TestMTreeDistance_Values(t *testing.T) {
	for _, v := range []MTreeDistanceType{
		MTreeDistanceCosine, MTreeDistanceEuclidean,
		MTreeDistanceManhattan, MTreeDistanceMinkowski,
	} {
		if !v.IsValid() {
			t.Errorf("MTree distance %q should be valid", string(v))
		}
	}
	if MTreeDistanceType("X").IsValid() {
		t.Error("bogus MTree distance")
	}
}

func TestHnswDistance_Values(t *testing.T) {
	for _, v := range []HnswDistanceType{
		HnswDistanceChebyshev, HnswDistanceCosine, HnswDistanceEuclidean,
		HnswDistanceHamming, HnswDistanceJaccard, HnswDistanceManhattan,
		HnswDistanceMinkowski, HnswDistancePearson,
	} {
		if !v.IsValid() {
			t.Errorf("Hnsw distance %q should be valid", string(v))
		}
	}
}

func TestVectorType_Values(t *testing.T) {
	for _, v := range []MTreeVectorType{
		MTreeVectorF64, MTreeVectorF32, MTreeVectorI64, MTreeVectorI32, MTreeVectorI16,
	} {
		if !v.IsValid() {
			t.Errorf("vector type %q should be valid", string(v))
		}
	}
}

func TestMTreeIndex_ToSurql(t *testing.T) {
	idx := MTreeIndex("embedding_idx", "embedding", 1536, MTreeIndexOptions{
		Distance:   MTreeDistanceCosine,
		VectorType: MTreeVectorF32,
	})
	got := idx.ToSurql("doc")
	want := "DEFINE INDEX embedding_idx ON TABLE doc COLUMNS embedding MTREE DIMENSION 1536 DIST COSINE TYPE F32;"
	if got != want {
		t.Errorf("ToSurql = %q, want %q", got, want)
	}
}

func TestMTreeIndex_Defaults(t *testing.T) {
	idx := MTreeIndex("e", "v", 8, MTreeIndexOptions{})
	got := idx.ToSurql("t")
	if !strings.Contains(got, "DIST EUCLIDEAN") {
		t.Errorf("default distance missing: %q", got)
	}
	if !strings.Contains(got, "TYPE F64") {
		t.Errorf("default vector type missing: %q", got)
	}
}

func TestHnswIndex_ToSurql(t *testing.T) {
	idx := HnswIndex("embedding_idx", "embedding", 1536, HnswIndexOptions{
		Distance:   HnswDistanceCosine,
		VectorType: MTreeVectorF32,
		EFC:        150,
		M:          12,
	})
	got := idx.ToSurql("doc")
	want := "DEFINE INDEX embedding_idx ON TABLE doc COLUMNS embedding HNSW DIMENSION 1536 DIST COSINE TYPE F32 EFC 150 M 12;"
	if got != want {
		t.Errorf("ToSurql = %q, want %q", got, want)
	}
}

func TestHnswIndex_OmitsEFCAndMWhenZero(t *testing.T) {
	idx := HnswIndex("e", "v", 8, HnswIndexOptions{})
	got := idx.ToSurql("t")
	if strings.Contains(got, "EFC") {
		t.Errorf("unexpected EFC clause: %q", got)
	}
	if strings.Contains(got, " M ") {
		t.Errorf("unexpected M clause: %q", got)
	}
}

func TestIndexToSurql_UniqueAndStandard(t *testing.T) {
	u := UniqueIndex("u", []string{"email"})
	if g := u.ToSurql("user"); g != "DEFINE INDEX u ON TABLE user COLUMNS email UNIQUE;" {
		t.Errorf("unique = %q", g)
	}
	s := NewIndex("t", []string{"title"}, IndexTypeStandard)
	if g := s.ToSurql("post"); g != "DEFINE INDEX t ON TABLE post COLUMNS title;" {
		t.Errorf("standard = %q", g)
	}
}

func TestIndexValidate_Empty(t *testing.T) {
	cases := []IndexDefinition{
		{Name: "", Columns: []string{"x"}, Type: IndexTypeUnique},
		{Name: "n", Columns: []string{}, Type: IndexTypeUnique},
		{Name: "n", Columns: []string{""}, Type: IndexTypeUnique},
		{Name: "n", Columns: []string{"x"}, Type: IndexType("bogus")},
	}
	for i, idx := range cases {
		err := idx.Validate()
		if err == nil {
			t.Errorf("case %d: Validate() returned nil", i)
			continue
		}
		if !stdErrors.Is(err, surqlerrors.ErrValidation) {
			t.Errorf("case %d: err = %v", i, err)
		}
	}
}

func TestIndexValidate_MTreeDimension(t *testing.T) {
	idx := IndexDefinition{Name: "n", Columns: []string{"v"}, Type: IndexTypeMTree, Dimension: 0}
	if err := idx.Validate(); err == nil {
		t.Error("MTREE with dimension 0 should fail")
	}
}

func TestIndexValidate_HnswDimension(t *testing.T) {
	idx := IndexDefinition{Name: "n", Columns: []string{"v"}, Type: IndexTypeHNSW, Dimension: 0}
	if err := idx.Validate(); err == nil {
		t.Error("HNSW with dimension 0 should fail")
	}
}

func TestEventToSurql(t *testing.T) {
	e := NewEvent("audit", "$before != $after", "CREATE log")
	got := e.ToSurql("user")
	want := "DEFINE EVENT audit ON TABLE user WHEN $before != $after THEN CREATE log;"
	if got != want {
		t.Errorf("ToSurql() = %q, want %q", got, want)
	}
}

func TestEventValidate(t *testing.T) {
	cases := []EventDefinition{
		{Name: "", Condition: "c", Action: "a"},
		{Name: "n", Condition: "", Action: "a"},
		{Name: "n", Condition: "c", Action: ""},
	}
	for _, e := range cases {
		if err := e.Validate(); err == nil {
			t.Errorf("expected error for %+v", e)
		}
	}
	ok := NewEvent("n", "c", "a")
	if err := ok.Validate(); err != nil {
		t.Errorf("valid event errored: %v", err)
	}
}

func TestTableValidate_EmptyName(t *testing.T) {
	tbl := NewTable("")
	if err := tbl.Validate(); err == nil {
		t.Error("empty name should fail")
	}
}

func TestTableValidate_InvalidMode(t *testing.T) {
	tbl := TableDefinition{Name: "t", Mode: TableMode("bogus")}
	if err := tbl.Validate(); err == nil {
		t.Error("invalid mode should fail")
	}
}

func TestTableValidate_PropagatesField(t *testing.T) {
	tbl := NewTable("t", WithFields(NewField("1bad", FieldTypeString)))
	err := tbl.Validate()
	if err == nil {
		t.Fatal("expected validation error from field")
	}
	if !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("err = %v", err)
	}
}

func TestTablePermissions_MapCopyIsolation(t *testing.T) {
	perms := map[string]string{"select": "$auth.id = id"}
	tbl := NewTable("t", WithTablePermissions(perms))
	perms["select"] = "MUTATED"
	if tbl.Permissions["select"] != "$auth.id = id" {
		t.Errorf("expected copied map; got %q", tbl.Permissions["select"])
	}
}

func TestTableStatements_StatementOrder(t *testing.T) {
	tbl := NewTable("user",
		WithFields(StringField("name")),
		WithIndexes(UniqueIndex("name_idx", []string{"name"})),
	)
	stmts := tbl.ToSurqlStatements()
	if !strings.HasPrefix(stmts[0], "DEFINE TABLE") {
		t.Errorf("stmts[0] = %q", stmts[0])
	}
	if !strings.HasPrefix(stmts[1], "DEFINE FIELD") {
		t.Errorf("stmts[1] = %q", stmts[1])
	}
	if !strings.HasPrefix(stmts[2], "DEFINE INDEX") {
		t.Errorf("stmts[2] = %q", stmts[2])
	}
}
