package schema

import (
	stdErrors "errors"
	"reflect"
	"sort"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// ---------------------------------------------------------------------------
// parseTableMode / ParseTableInfo
// ---------------------------------------------------------------------------

func TestParseTableMode_Schemafull(t *testing.T) {
	if got := parseTableMode("DEFINE TABLE user SCHEMAFULL;"); got != TableModeSchemafull {
		t.Errorf("parseTableMode(SCHEMAFULL) = %q, want %q", got, TableModeSchemafull)
	}
}

func TestParseTableMode_Schemaless(t *testing.T) {
	if got := parseTableMode("DEFINE TABLE user SCHEMALESS;"); got != TableModeSchemaless {
		t.Errorf("parseTableMode(SCHEMALESS) = %q, want %q", got, TableModeSchemaless)
	}
}

func TestParseTableMode_Drop(t *testing.T) {
	if got := parseTableMode("DEFINE TABLE tmp DROP;"); got != TableModeDrop {
		t.Errorf("parseTableMode(DROP) = %q, want %q", got, TableModeDrop)
	}
}

func TestParseTableMode_EmptyDefaultsSchemaless(t *testing.T) {
	if got := parseTableMode(""); got != TableModeSchemaless {
		t.Errorf("parseTableMode(\"\") = %q, want %q", got, TableModeSchemaless)
	}
}

func TestParseTableInfo_EmptyName(t *testing.T) {
	_, err := ParseTableInfo("", nil)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !stdErrors.Is(err, surqlerrors.ErrSchemaParse) {
		t.Errorf("expected ErrSchemaParse, got %v", err)
	}
}

func TestParseTableInfo_MinimalSchemafull(t *testing.T) {
	info := map[string]any{
		"tb": "DEFINE TABLE user SCHEMAFULL;",
	}
	got, err := ParseTableInfo("user", info)
	if err != nil {
		t.Fatalf("ParseTableInfo: %v", err)
	}
	if got.Name != "user" {
		t.Errorf("Name = %q, want user", got.Name)
	}
	if got.Mode != TableModeSchemafull {
		t.Errorf("Mode = %q, want SCHEMAFULL", got.Mode)
	}
	if len(got.Fields) != 0 || len(got.Indexes) != 0 || len(got.Events) != 0 {
		t.Errorf("expected empty children, got %+v", got)
	}
}

func TestParseTableInfo_FullRoundTrip(t *testing.T) {
	table := TableDefinition{
		Name: "user",
		Mode: TableModeSchemafull,
		Fields: []FieldDefinition{
			StringField("email", WithAssertion("string::is::email($value)")),
			IntField("age", WithDefault("0")),
			BoolField("admin", WithReadOnly(true)),
		},
		Indexes: []IndexDefinition{
			UniqueIndex("email_idx", []string{"email"}),
		},
		Events: []EventDefinition{
			NewEvent("on_create", "$event = 'CREATE'", "(CREATE log SET t = time::now())"),
		},
	}

	// Build synthetic INFO response.
	fd := map[string]any{}
	for _, f := range table.Fields {
		fd[f.Name] = f.ToSurql(table.Name)
	}
	ix := map[string]any{}
	for _, i := range table.Indexes {
		ix[i.Name] = i.ToSurql(table.Name)
	}
	ev := map[string]any{}
	for _, e := range table.Events {
		ev[e.Name] = e.ToSurql(table.Name)
	}

	info := map[string]any{
		"tb":      table.ToSurql(),
		"fields":  fd,
		"indexes": ix,
		"events":  ev,
	}

	parsed, err := ParseTableInfo("user", info)
	if err != nil {
		t.Fatalf("ParseTableInfo: %v", err)
	}
	if parsed.Mode != table.Mode {
		t.Errorf("Mode = %q, want %q", parsed.Mode, table.Mode)
	}
	if len(parsed.Fields) != len(table.Fields) {
		t.Errorf("expected %d fields, got %d", len(table.Fields), len(parsed.Fields))
	}
	if len(parsed.Indexes) != len(table.Indexes) {
		t.Errorf("expected %d indexes, got %d", len(table.Indexes), len(parsed.Indexes))
	}
	if len(parsed.Events) != len(table.Events) {
		t.Errorf("expected %d events, got %d", len(table.Events), len(parsed.Events))
	}
}

func TestParseTableInfo_AliasKeys(t *testing.T) {
	info := map[string]any{
		"tb": "DEFINE TABLE user SCHEMAFULL;",
		"fd": map[string]any{
			"email": "DEFINE FIELD email ON TABLE user TYPE string;",
		},
		"ix": map[string]any{
			"email_idx": "DEFINE INDEX email_idx ON TABLE user COLUMNS email UNIQUE;",
		},
		"ev": map[string]any{
			"on_x": "DEFINE EVENT on_x ON TABLE user WHEN $event = 'CREATE' THEN (SELECT 1);",
		},
	}
	got, err := ParseTableInfo("user", info)
	if err != nil {
		t.Fatalf("ParseTableInfo: %v", err)
	}
	if len(got.Fields) != 1 || got.Fields[0].Name != "email" {
		t.Errorf("fields = %+v", got.Fields)
	}
	if len(got.Indexes) != 1 || got.Indexes[0].Name != "email_idx" {
		t.Errorf("indexes = %+v", got.Indexes)
	}
	if len(got.Events) != 1 || got.Events[0].Name != "on_x" {
		t.Errorf("events = %+v", got.Events)
	}
}

func TestParseTableInfo_MapStringStringAccepted(t *testing.T) {
	info := map[string]any{
		"tb": "DEFINE TABLE user SCHEMAFULL;",
		"fields": map[string]string{
			"email": "DEFINE FIELD email ON TABLE user TYPE string;",
		},
	}
	got, err := ParseTableInfo("user", info)
	if err != nil {
		t.Fatalf("ParseTableInfo: %v", err)
	}
	if len(got.Fields) != 1 {
		t.Errorf("expected 1 field, got %d", len(got.Fields))
	}
}

// ---------------------------------------------------------------------------
// ParseField
// ---------------------------------------------------------------------------

func TestParseField_String(t *testing.T) {
	fd, err := ParseField("email", "DEFINE FIELD email ON TABLE user TYPE string;")
	if err != nil {
		t.Fatal(err)
	}
	if fd.Type != FieldTypeString {
		t.Errorf("Type = %q, want string", fd.Type)
	}
}

func TestParseField_AllTypes(t *testing.T) {
	cases := map[string]FieldType{
		"string":   FieldTypeString,
		"int":      FieldTypeInt,
		"float":    FieldTypeFloat,
		"bool":     FieldTypeBool,
		"datetime": FieldTypeDatetime,
		"duration": FieldTypeDuration,
		"decimal":  FieldTypeDecimal,
		"number":   FieldTypeNumber,
		"object":   FieldTypeObject,
		"array":    FieldTypeArray,
		"record":   FieldTypeRecord,
		"geometry": FieldTypeGeometry,
		"any":      FieldTypeAny,
	}
	for typeStr, want := range cases {
		def := "DEFINE FIELD x ON TABLE user TYPE " + typeStr + ";"
		fd, err := ParseField("x", def)
		if err != nil {
			t.Fatalf("ParseField(%s): %v", typeStr, err)
		}
		if fd.Type != want {
			t.Errorf("Type for %q = %q, want %q", typeStr, fd.Type, want)
		}
	}
}

func TestParseField_UnknownTypeFallsBackToAny(t *testing.T) {
	fd, err := ParseField("x", "DEFINE FIELD x ON TABLE user TYPE weirdthing;")
	if err != nil {
		t.Fatal(err)
	}
	if fd.Type != FieldTypeAny {
		t.Errorf("Type = %q, want any", fd.Type)
	}
}

func TestParseField_WithAssertion(t *testing.T) {
	fd, err := ParseField("email",
		"DEFINE FIELD email ON TABLE user TYPE string ASSERT string::is::email($value);")
	if err != nil {
		t.Fatal(err)
	}
	if fd.Assertion != "string::is::email($value)" {
		t.Errorf("Assertion = %q", fd.Assertion)
	}
}

func TestParseField_WithDefault(t *testing.T) {
	fd, err := ParseField("age",
		"DEFINE FIELD age ON TABLE user TYPE int DEFAULT 18;")
	if err != nil {
		t.Fatal(err)
	}
	if fd.Default != "18" {
		t.Errorf("Default = %q, want 18", fd.Default)
	}
}

func TestParseField_WithValue(t *testing.T) {
	fd, err := ParseField("full",
		"DEFINE FIELD full ON TABLE user TYPE string VALUE string::concat(first, last);")
	if err != nil {
		t.Fatal(err)
	}
	if fd.Value != "string::concat(first, last)" {
		t.Errorf("Value = %q", fd.Value)
	}
}

func TestParseField_ReadOnlyFlag(t *testing.T) {
	fd, err := ParseField("id",
		"DEFINE FIELD id ON TABLE user TYPE string READONLY;")
	if err != nil {
		t.Fatal(err)
	}
	if !fd.ReadOnly {
		t.Error("ReadOnly should be true")
	}
}

func TestParseField_FlexibleFlag(t *testing.T) {
	fd, err := ParseField("meta",
		"DEFINE FIELD meta ON TABLE user TYPE object FLEXIBLE;")
	if err != nil {
		t.Fatal(err)
	}
	if !fd.Flexible {
		t.Error("Flexible should be true")
	}
}

func TestParseField_EmptyDefinitionErrors(t *testing.T) {
	_, err := ParseField("x", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrSchemaParse) {
		t.Errorf("expected ErrSchemaParse, got %v", err)
	}
}

func TestParseField_EmptyNameErrors(t *testing.T) {
	_, err := ParseField("", "DEFINE FIELD x ON TABLE user TYPE string;")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseField_RoundTrip(t *testing.T) {
	orig := NewField("email", FieldTypeString,
		WithAssertion("$value != NONE"),
		WithDefault("'a@b.com'"),
	)
	def := orig.ToSurql("user")
	parsed, err := ParseField("email", def)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Type != orig.Type {
		t.Errorf("Type: got %q, want %q", parsed.Type, orig.Type)
	}
	if parsed.Assertion != orig.Assertion {
		t.Errorf("Assertion: got %q, want %q", parsed.Assertion, orig.Assertion)
	}
	if parsed.Default != orig.Default {
		t.Errorf("Default: got %q, want %q", parsed.Default, orig.Default)
	}
}

// ---------------------------------------------------------------------------
// ParseIndex
// ---------------------------------------------------------------------------

func TestParseIndex_Standard(t *testing.T) {
	idx, err := ParseIndex("ix1",
		"DEFINE INDEX ix1 ON TABLE user COLUMNS email;")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Type != IndexTypeStandard {
		t.Errorf("Type = %q, want INDEX", idx.Type)
	}
	if !reflect.DeepEqual(idx.Columns, []string{"email"}) {
		t.Errorf("Columns = %v", idx.Columns)
	}
}

func TestParseIndex_Unique(t *testing.T) {
	idx, err := ParseIndex("email_idx",
		"DEFINE INDEX email_idx ON TABLE user COLUMNS email UNIQUE;")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Type != IndexTypeUnique {
		t.Errorf("Type = %q, want UNIQUE", idx.Type)
	}
}

func TestParseIndex_MultipleColumns(t *testing.T) {
	idx, err := ParseIndex("composite",
		"DEFINE INDEX composite ON TABLE user COLUMNS a, b, c UNIQUE;")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(idx.Columns, want) {
		t.Errorf("Columns = %v, want %v", idx.Columns, want)
	}
}

func TestParseIndex_FieldsAlias(t *testing.T) {
	idx, err := ParseIndex("ix",
		"DEFINE INDEX ix ON TABLE user FIELDS a, b;")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(idx.Columns, []string{"a", "b"}) {
		t.Errorf("Columns = %v", idx.Columns)
	}
}

func TestParseIndex_MTree(t *testing.T) {
	idx, err := ParseIndex("emb_idx",
		"DEFINE INDEX emb_idx ON TABLE doc COLUMNS embedding MTREE DIMENSION 128 DIST COSINE TYPE F32;")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Type != IndexTypeMTree {
		t.Errorf("Type = %q, want MTREE", idx.Type)
	}
	if idx.Dimension != 128 {
		t.Errorf("Dimension = %d", idx.Dimension)
	}
	if idx.Distance != MTreeDistanceCosine {
		t.Errorf("Distance = %q", idx.Distance)
	}
	if idx.VectorType != MTreeVectorF32 {
		t.Errorf("VectorType = %q", idx.VectorType)
	}
}

func TestParseIndex_HNSW(t *testing.T) {
	idx, err := ParseIndex("hnsw_idx",
		"DEFINE INDEX hnsw_idx ON TABLE doc COLUMNS embedding HNSW DIMENSION 256 DIST EUCLIDEAN TYPE F64 EFC 200 M 16;")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Type != IndexTypeHNSW {
		t.Errorf("Type = %q, want HNSW", idx.Type)
	}
	if idx.Dimension != 256 {
		t.Errorf("Dimension = %d", idx.Dimension)
	}
	if idx.HnswDistance != HnswDistanceEuclidean {
		t.Errorf("HnswDistance = %q", idx.HnswDistance)
	}
	if idx.VectorType != MTreeVectorF64 {
		t.Errorf("VectorType = %q", idx.VectorType)
	}
	if idx.EFC != 200 {
		t.Errorf("EFC = %d", idx.EFC)
	}
	if idx.M != 16 {
		t.Errorf("M = %d", idx.M)
	}
}

func TestParseIndex_HNSWChebyshev(t *testing.T) {
	idx, err := ParseIndex("h",
		"DEFINE INDEX h ON TABLE x COLUMNS v HNSW DIMENSION 4 DIST CHEBYSHEV;")
	if err != nil {
		t.Fatal(err)
	}
	if idx.HnswDistance != HnswDistanceChebyshev {
		t.Errorf("got %q", idx.HnswDistance)
	}
}

func TestParseIndex_EmptyDefinition(t *testing.T) {
	_, err := ParseIndex("x", "   ")
	if err == nil {
		t.Fatal("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrSchemaParse) {
		t.Errorf("expected ErrSchemaParse")
	}
}

func TestParseIndex_RoundTripUnique(t *testing.T) {
	orig := UniqueIndex("email_idx", []string{"email"})
	parsed, err := ParseIndex(orig.Name, orig.ToSurql("user"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Type != orig.Type {
		t.Errorf("Type: got %q, want %q", parsed.Type, orig.Type)
	}
	if !reflect.DeepEqual(parsed.Columns, orig.Columns) {
		t.Errorf("Columns: got %v, want %v", parsed.Columns, orig.Columns)
	}
}

func TestParseIndex_RoundTripMTree(t *testing.T) {
	orig := MTreeIndex("emb_idx", "embedding", 64, MTreeIndexOptions{
		Distance:   MTreeDistanceEuclidean,
		VectorType: MTreeVectorF32,
	})
	parsed, err := ParseIndex(orig.Name, orig.ToSurql("doc"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Type != IndexTypeMTree {
		t.Errorf("Type = %q", parsed.Type)
	}
	if parsed.Dimension != 64 {
		t.Errorf("Dimension = %d", parsed.Dimension)
	}
	if parsed.Distance != MTreeDistanceEuclidean {
		t.Errorf("Distance = %q", parsed.Distance)
	}
	if parsed.VectorType != MTreeVectorF32 {
		t.Errorf("VectorType = %q", parsed.VectorType)
	}
}

func TestParseIndex_RoundTripHnsw(t *testing.T) {
	orig := HnswIndex("h", "v", 128, HnswIndexOptions{
		Distance:   HnswDistanceCosine,
		VectorType: MTreeVectorF32,
		EFC:        150,
		M:          12,
	})
	parsed, err := ParseIndex(orig.Name, orig.ToSurql("doc"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.HnswDistance != HnswDistanceCosine {
		t.Errorf("HnswDistance = %q", parsed.HnswDistance)
	}
	if parsed.EFC != 150 || parsed.M != 12 {
		t.Errorf("EFC=%d M=%d", parsed.EFC, parsed.M)
	}
}

func TestParseIndex_FulltextKeyword(t *testing.T) {
	idx, err := ParseIndex("s",
		"DEFINE INDEX s ON TABLE doc COLUMNS content FULLTEXT ANALYZER text_en BM25 HIGHLIGHTS;")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Type != IndexTypeSearch {
		t.Errorf("Type = %q, want SEARCH", idx.Type)
	}
	if !reflect.DeepEqual(idx.Columns, []string{"content"}) {
		t.Errorf("Columns = %v", idx.Columns)
	}
	if idx.Analyzer != "text_en" {
		t.Errorf("Analyzer = %q, want text_en", idx.Analyzer)
	}
	if !idx.BM25 {
		t.Error("expected BM25 = true")
	}
	if !idx.Highlights {
		t.Error("expected Highlights = true")
	}
}

func TestParseIndex_LegacySearchKeyword(t *testing.T) {
	// The v1/v2 SEARCH spelling is still recognised so historical INFO output
	// round-trips into the same IndexTypeSearch.
	idx, err := ParseIndex("s",
		"DEFINE INDEX s ON TABLE doc COLUMNS content SEARCH ANALYZER text_en BM25;")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Type != IndexTypeSearch {
		t.Errorf("Type = %q, want SEARCH", idx.Type)
	}
	if idx.Analyzer != "text_en" {
		t.Errorf("Analyzer = %q, want text_en", idx.Analyzer)
	}
	if !idx.BM25 {
		t.Error("expected BM25 = true")
	}
	if idx.Highlights {
		t.Error("expected Highlights = false")
	}
}

func TestParseIndex_FulltextAsciiNormalizesToUnset(t *testing.T) {
	// The historical `ascii` default normalises back to "" so the default
	// SearchIndex form round-trips as an identity.
	idx, err := ParseIndex("content_search",
		"DEFINE INDEX content_search ON TABLE post COLUMNS title, content FULLTEXT ANALYZER ascii;")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Type != IndexTypeSearch {
		t.Errorf("Type = %q, want SEARCH", idx.Type)
	}
	if idx.Analyzer != "" {
		t.Errorf("Analyzer = %q, want \"\" (ascii normalises to unset)", idx.Analyzer)
	}
	if idx.BM25 || idx.Highlights {
		t.Errorf("expected no BM25/HIGHLIGHTS, got bm25=%v highlights=%v", idx.BM25, idx.Highlights)
	}
}

func TestParseIndex_RoundTripSearchDefault(t *testing.T) {
	orig := SearchIndex("content_search", []string{"title", "content"})
	parsed, err := ParseIndex(orig.Name, orig.ToSurql("post"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Type != orig.Type {
		t.Errorf("Type: got %q, want %q", parsed.Type, orig.Type)
	}
	if !reflect.DeepEqual(parsed.Columns, orig.Columns) {
		t.Errorf("Columns: got %v, want %v", parsed.Columns, orig.Columns)
	}
	if parsed.Analyzer != orig.Analyzer {
		t.Errorf("Analyzer: got %q, want %q", parsed.Analyzer, orig.Analyzer)
	}
	if parsed.BM25 != orig.BM25 || parsed.Highlights != orig.Highlights {
		t.Errorf("flags differ: parsed bm25=%v hl=%v, orig bm25=%v hl=%v",
			parsed.BM25, parsed.Highlights, orig.BM25, orig.Highlights)
	}
}

func TestParseIndex_RoundTripBM25(t *testing.T) {
	orig := BM25Index("content_bm25", []string{"content"}, "text_en").WithHighlights()
	parsed, err := ParseIndex(orig.Name, orig.ToSurql("memory"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Type != IndexTypeSearch {
		t.Errorf("Type = %q, want SEARCH", parsed.Type)
	}
	if !reflect.DeepEqual(parsed.Columns, orig.Columns) {
		t.Errorf("Columns: got %v, want %v", parsed.Columns, orig.Columns)
	}
	if parsed.Analyzer != "text_en" {
		t.Errorf("Analyzer = %q, want text_en", parsed.Analyzer)
	}
	if !parsed.BM25 {
		t.Error("expected BM25 = true")
	}
	if !parsed.Highlights {
		t.Error("expected Highlights = true")
	}
}

// ---------------------------------------------------------------------------
// ParseEvent
// ---------------------------------------------------------------------------

func TestParseEvent_Simple(t *testing.T) {
	ev, err := ParseEvent("on_create",
		"DEFINE EVENT on_create ON TABLE user WHEN $event = 'CREATE' THEN (SELECT 1);")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Condition != "$event = 'CREATE'" {
		t.Errorf("Condition = %q", ev.Condition)
	}
	if ev.Action != "(SELECT 1)" {
		t.Errorf("Action = %q", ev.Action)
	}
}

func TestParseEvent_WithBraces(t *testing.T) {
	ev, err := ParseEvent("on_create",
		"DEFINE EVENT on_create ON TABLE user WHEN $event = 'CREATE' THEN { CREATE log SET type = 'create' };")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Action != "CREATE log SET type = 'create'" {
		t.Errorf("Action = %q", ev.Action)
	}
}

func TestParseEvent_MissingWhen(t *testing.T) {
	_, err := ParseEvent("bad",
		"DEFINE EVENT bad ON TABLE user THEN (SELECT 1);")
	if err == nil {
		t.Fatal("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrSchemaParse) {
		t.Error("expected ErrSchemaParse")
	}
}

func TestParseEvent_EmptyDefinition(t *testing.T) {
	_, err := ParseEvent("x", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseEvent_RoundTrip(t *testing.T) {
	orig := NewEvent("trigger", "$before.count < $after.count", "(UPDATE stats SET incremented += 1)")
	parsed, err := ParseEvent(orig.Name, orig.ToSurql("t"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Condition != orig.Condition {
		t.Errorf("Condition: got %q, want %q", parsed.Condition, orig.Condition)
	}
	if parsed.Action != "(UPDATE stats SET incremented += 1)" {
		t.Errorf("Action: got %q", parsed.Action)
	}
}

// ---------------------------------------------------------------------------
// ParseAccess
// ---------------------------------------------------------------------------

func TestParseAccess_JWTKey(t *testing.T) {
	def := "DEFINE ACCESS api ON DATABASE TYPE JWT ALGORITHM HS256 KEY 'secret';"
	ad, err := ParseAccess("api", def)
	if err != nil {
		t.Fatal(err)
	}
	if ad.Type != AccessTypeJWT {
		t.Errorf("Type = %q", ad.Type)
	}
	if ad.JWT == nil {
		t.Fatal("JWT config nil")
	}
	if ad.JWT.Algorithm != "HS256" {
		t.Errorf("Algorithm = %q", ad.JWT.Algorithm)
	}
	if ad.JWT.Key != "secret" {
		t.Errorf("Key = %q", ad.JWT.Key)
	}
}

func TestParseAccess_JWTURLIssuer(t *testing.T) {
	def := "DEFINE ACCESS api ON DATABASE TYPE JWT ALGORITHM RS256 URL 'https://jwks.example/.well-known/jwks.json' WITH ISSUER 'issuer-x';"
	ad, err := ParseAccess("api", def)
	if err != nil {
		t.Fatal(err)
	}
	if ad.JWT.URL != "https://jwks.example/.well-known/jwks.json" {
		t.Errorf("URL = %q", ad.JWT.URL)
	}
	if ad.JWT.Issuer != "issuer-x" {
		t.Errorf("Issuer = %q", ad.JWT.Issuer)
	}
	if ad.JWT.Algorithm != "RS256" {
		t.Errorf("Algorithm = %q", ad.JWT.Algorithm)
	}
}

func TestParseAccess_Record(t *testing.T) {
	def := "DEFINE ACCESS user ON DATABASE TYPE RECORD SIGNUP (CREATE user SET email=$email) SIGNIN (SELECT * FROM user WHERE email=$email);"
	ad, err := ParseAccess("user", def)
	if err != nil {
		t.Fatal(err)
	}
	if ad.Type != AccessTypeRecord {
		t.Errorf("Type = %q", ad.Type)
	}
	if ad.Record == nil {
		t.Fatal("Record config nil")
	}
	if ad.Record.Signup != "CREATE user SET email=$email" {
		t.Errorf("Signup = %q", ad.Record.Signup)
	}
	if ad.Record.Signin != "SELECT * FROM user WHERE email=$email" {
		t.Errorf("Signin = %q", ad.Record.Signin)
	}
}

func TestParseAccess_Duration(t *testing.T) {
	def := "DEFINE ACCESS api ON DATABASE TYPE JWT ALGORITHM HS256 KEY 'x' DURATION FOR SESSION 12h, FOR TOKEN 1h;"
	ad, err := ParseAccess("api", def)
	if err != nil {
		t.Fatal(err)
	}
	if ad.DurationSession != "12h" {
		t.Errorf("DurationSession = %q", ad.DurationSession)
	}
	if ad.DurationToken != "1h" {
		t.Errorf("DurationToken = %q", ad.DurationToken)
	}
}

func TestParseAccess_UnknownTypeError(t *testing.T) {
	def := "DEFINE ACCESS api ON DATABASE TYPE FOO;"
	_, err := ParseAccess("api", def)
	if err == nil {
		t.Fatal("expected error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrSchemaParse) {
		t.Errorf("expected ErrSchemaParse")
	}
}

func TestParseAccess_EmptyDefError(t *testing.T) {
	_, err := ParseAccess("api", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseAccess_EmptyNameError(t *testing.T) {
	_, err := ParseAccess("", "DEFINE ACCESS api ON DATABASE TYPE JWT ALGORITHM HS256 KEY 'x';")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseAccess_JWTRoundTrip(t *testing.T) {
	orig := JwtAccess("api", JwtConfig{Algorithm: "HS256", Key: "topsecret", Issuer: "me"})
	parsed, err := ParseAccess(orig.Name, orig.ToSurql())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.JWT.Key != "topsecret" || parsed.JWT.Issuer != "me" || parsed.JWT.Algorithm != "HS256" {
		t.Errorf("JWT round trip mismatch: %+v", parsed.JWT)
	}
}

// ---------------------------------------------------------------------------
// ParseDBInfo
// ---------------------------------------------------------------------------

func TestParseDBInfo_Nil(t *testing.T) {
	got, err := ParseDBInfo(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tables == nil || got.Edges == nil || got.Accesses == nil {
		t.Error("expected non-nil maps")
	}
}

func TestParseDBInfo_TablesAndEdges(t *testing.T) {
	info := map[string]any{
		"tb": map[string]any{
			"user":    "DEFINE TABLE user SCHEMAFULL;",
			"product": "DEFINE TABLE product SCHEMALESS;",
			"likes":   "DEFINE TABLE likes TYPE RELATION FROM user TO product;",
		},
	}
	got, err := ParseDBInfo(info)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables) != 2 {
		t.Errorf("tables = %d, want 2", len(got.Tables))
	}
	if _, ok := got.Tables["user"]; !ok {
		t.Error("missing user table")
	}
	if got.Tables["user"].Mode != TableModeSchemafull {
		t.Errorf("user mode = %q", got.Tables["user"].Mode)
	}
	if got.Tables["product"].Mode != TableModeSchemaless {
		t.Errorf("product mode = %q", got.Tables["product"].Mode)
	}
	if len(got.Edges) != 1 {
		t.Errorf("edges = %d, want 1", len(got.Edges))
	}
	likes := got.Edges["likes"]
	if likes.FromTable != "user" || likes.ToTable != "product" {
		t.Errorf("likes from/to = %q/%q", likes.FromTable, likes.ToTable)
	}
	if likes.Mode != EdgeModeRelation {
		t.Errorf("likes mode = %q", likes.Mode)
	}
}

func TestParseDBInfo_Accesses(t *testing.T) {
	info := map[string]any{
		"ac": map[string]any{
			"api":  "DEFINE ACCESS api ON DATABASE TYPE JWT ALGORITHM HS256 KEY 'k';",
			"user": "DEFINE ACCESS user ON DATABASE TYPE RECORD SIGNUP (CREATE user) SIGNIN (SELECT * FROM user);",
		},
	}
	got, err := ParseDBInfo(info)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Accesses) != 2 {
		t.Errorf("accesses = %d, want 2", len(got.Accesses))
	}
	names := []string{}
	for n := range got.Accesses {
		names = append(names, n)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{"api", "user"}) {
		t.Errorf("names = %v", names)
	}
}

func TestParseDBInfo_SkipsMalformedAccess(t *testing.T) {
	info := map[string]any{
		"ac": map[string]any{
			"good": "DEFINE ACCESS good ON DATABASE TYPE JWT ALGORITHM HS256 KEY 'k';",
			"bad":  "DEFINE ACCESS bad ON DATABASE TYPE UNKNOWN;",
		},
	}
	got, err := ParseDBInfo(info)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Accesses["good"]; !ok {
		t.Error("good access missing")
	}
	if _, ok := got.Accesses["bad"]; ok {
		t.Error("bad access should have been skipped")
	}
}

// ---------------------------------------------------------------------------
// ParseEdgeInfo
// ---------------------------------------------------------------------------

func TestParseEdgeInfo_Relation(t *testing.T) {
	info := map[string]any{
		"tb": "DEFINE TABLE likes TYPE RELATION FROM user TO product;",
		"fields": map[string]any{
			"weight": "DEFINE FIELD weight ON TABLE likes TYPE float DEFAULT 1.0;",
		},
	}
	edge, err := ParseEdgeInfo("likes", info)
	if err != nil {
		t.Fatal(err)
	}
	if edge.Mode != EdgeModeRelation {
		t.Errorf("Mode = %q", edge.Mode)
	}
	if edge.FromTable != "user" || edge.ToTable != "product" {
		t.Errorf("from/to = %q/%q", edge.FromTable, edge.ToTable)
	}
	if len(edge.Fields) != 1 || edge.Fields[0].Name != "weight" {
		t.Errorf("fields = %+v", edge.Fields)
	}
}

func TestParseEdgeInfo_EmptyName(t *testing.T) {
	_, err := ParseEdgeInfo("", map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// extractParenClause / helpers
// ---------------------------------------------------------------------------

func TestExtractParenClause_Nested(t *testing.T) {
	def := "DEFINE ACCESS u ON DATABASE TYPE RECORD SIGNUP (CREATE user SET x = (1 + 2));"
	got := extractParenClause(def, "SIGNUP")
	want := "CREATE user SET x = (1 + 2)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractParenClause_Missing(t *testing.T) {
	def := "DEFINE ACCESS u ON DATABASE TYPE RECORD;"
	if got := extractParenClause(def, "SIGNUP"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestSplitColumns_TrimsEmpties(t *testing.T) {
	got := splitColumns("a, ,b , c,")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtractStringMap_ReadsAlias(t *testing.T) {
	in := map[string]any{
		"fd": map[string]any{"x": "def"},
	}
	got := extractStringMap(in, "fields", "fd")
	if got["x"] != "def" {
		t.Errorf("got %v", got)
	}
}

func TestExtractStringMap_IgnoresNonString(t *testing.T) {
	in := map[string]any{
		"fd": map[string]any{"x": 123, "y": "ok"},
	}
	got := extractStringMap(in, "fd")
	if _, ok := got["x"]; ok {
		t.Error("numeric entry should be dropped")
	}
	if got["y"] != "ok" {
		t.Errorf("y = %q", got["y"])
	}
}

func TestStringAt_WrongTypeReturnsEmpty(t *testing.T) {
	in := map[string]any{"tb": 5}
	if got := stringAt(in, "tb"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestIsEdgeDefinitionSource(t *testing.T) {
	if !isEdgeDefinitionSource("DEFINE TABLE likes TYPE RELATION FROM a TO b;") {
		t.Error("should detect RELATION")
	}
	if isEdgeDefinitionSource("DEFINE TABLE user SCHEMAFULL;") {
		t.Error("should not flag plain table")
	}
}
