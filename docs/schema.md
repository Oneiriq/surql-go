# Schema Definition

The schema DSL is a code-first way to describe SurrealDB tables, edges,
fields, indexes, events, and access rules. Definitions render to `DEFINE`
statements and feed the migration generator + validator + visualizer.

## Tables

```go
user, _ := schema.NewTableDefinition(
    "user",
    schema.WithMode(schema.TableModeSchemafull),
    schema.WithFields(
        schema.StringField("email").WithAssertion("string::is::email($value)"),
        schema.IntField("age"),
    ),
    schema.WithIndexes(schema.UniqueIndex("email_idx", []string{"email"})),
)
```

`TableMode` has three variants: `TableModeSchemafull`, `TableModeSchemaless`,
`TableModeDrop`.

## Field helpers

| Helper               | SurrealDB type |
|----------------------|----------------|
| `StringField`        | `string`       |
| `IntField`           | `int`          |
| `FloatField`         | `float`        |
| `BoolField`          | `bool`         |
| `DatetimeField`      | `datetime`     |
| `DurationField`      | `duration`     |
| `DecimalField`       | `decimal`      |
| `ObjectField`        | `object` (defaults `Flexible=true`) |
| `ArrayField`         | `array`        |
| `RecordField(t)`     | `record<t>`    |
| `ComputedField`      | `VALUE <expr>` |

Chainable setters: `WithAssertion`, `WithDefault`, `WithValue`,
`WithReadonly`, `WithFlexible`, `WithPermissions`.

## Indexes

- `Index(name, cols)`
- `UniqueIndex(name, cols)`
- `SearchIndex(name, cols)` -- full-text index; renders the SurrealDB 3.x
  `FULLTEXT` keyword (see [v3 Patterns](v3-patterns.md#full-text-search-bm25))
- `BM25Index(name, cols, analyzer)` -- BM25-scored full-text index
- `MTreeIndex(name, col, dimension, opts)` with `MTreeIndexOptions{Distance, VectorType}`
- `HnswIndex(name, col, dimension, opts)` with `HnswIndexOptions{Distance, VectorType, EFC, M}`

Full-text indexes are chainable: `SearchIndex("s", []string{"content"}).WithAnalyzer("text_en").WithBM25().WithHighlights()`.
`WithBM25` is required for `search::score` to return a value; `WithHighlights`
stores positional data for `search::highlight` / `search::offsets`. With no
analyzer set, the index renders the historical `ascii` default.

## Analyzers

A full-text index references an *analyzer* that turns stored and query text into
comparable tokens (a tokenizer chain + a filter chain). Define it in code rather
than hand-authoring SurrealQL:

```go
import "github.com/Oneiriq/surql-go/pkg/surql/schema"

// class tokenizer + lowercase + ascii, plus English stemming.
a := schema.StandardAnalyzer("text_en", schema.WithFilter(schema.Snowball("english")))
// DEFINE ANALYZER text_en TOKENIZERS class FILTERS lowercase,ascii,snowball(english);
ddl := a.ToSurql()
```

- Tokenizers: `TokenizerBlank` / `TokenizerCamel` / `TokenizerClass` / `TokenizerPunct`.
- Filters: `ASCII()` / `Lowercase()` / `Uppercase()` / `EdgeNgram(min,max)` / `Ngram(min,max)` / `Snowball(lang)`.
- `Analyzer(name, opts...)` builds an empty analyzer; `StandardAnalyzer(name, opts...)`
  is `class` + `lowercase` + `ascii` — a sensible default for BM25 lexical recall.
- `GenerateAnalyzerSQL(a)` / `GenerateAnalyzerSQLIfNotExists(a)` render the
  `DEFINE ANALYZER` statement, which **must be applied before** the index that
  references it. `GenerateSchemaSQLFromSlicesWithAnalyzers(analyzers, tables, edges, ifNotExists)`
  emits analyzer DDL first, then tables, then edges.

## Events

```go
ev := schema.Event("new_user").
    WithWhen("$event = 'CREATE'").
    WithThen("CREATE log SET table = 'user', id = $value.id")
```

## Edges

```go
likes, _ := schema.NewEdgeDefinition(
    "likes",
    schema.WithEdgeMode(schema.EdgeModeRelation),
    schema.WithEdgeFrom("user"),
    schema.WithEdgeTo("post"),
)
```

## Access (record + JWT)

```go
api, _ := schema.NewAccessDefinition(
    "api",
    schema.WithAccessType(schema.AccessTypeJwt),
    schema.WithJwt(schema.JwtConfig{Algorithm: "HS512", Key: "secret"}),
    schema.WithDurationToken("1h"),
)
```

## Registry + SQL generation

```go
reg := schema.NewSchemaRegistry()
reg.RegisterTable(user)
reg.RegisterEdge(likes)

stmts, _ := schema.GenerateSchemaSQL(reg, false)
```

## What's next

- **[Migrations](migrations.md)** -- turning schema changes into migration
  files.
- **[Visualization](visualization.md)** -- Mermaid / GraphViz / ASCII
  diagrams from the registry.
