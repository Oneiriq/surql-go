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
- `SearchIndex(name, cols)`
- `MTreeIndex(name, col, opts)` with `MTreeIndexOptions{Dimension, Distance, Capacity, Cache}`
- `HnswIndex(name, col, opts)` with `HnswIndexOptions{Dimension, Distance, EFC, M}`

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
