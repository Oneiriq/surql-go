# Features & Package Layout

surql-go is organised as one module with subpackages under
`pkg/surql/`. The top-level `pkg/surql` package re-exports the most
commonly used helpers so consumers can import a single path for the
common case.

## Module map

```
github.com/Oneiriq/surql-go
├── cmd/surql             # cobra-based CLI binary
├── internal/cli          # CLI implementation (root, migrate, schema, db, orchestrate)
└── pkg/surql             # top-level re-exports (ExtractOne / ExtractScalar[T] / ...)
    ├── types             # operators, record ids, SurrealFn, reserved words, coerce
    ├── query             # query builder, executor, CRUD, batch, graph, hints, results
    ├── schema            # code-first DSL: fields, tables, edges, indexes, events
    ├── migration         # discovery, diff, generator, versioning, squash, watcher, hooks
    ├── connection        # DatabaseClient, Transaction, LiveQuery, AuthManager, StreamingManager
    ├── cache             # memory + redis cache backends and decorators
    ├── orchestration     # multi-environment deployment coordinator, strategies, health
    ├── settings          # yaml / toml / .env settings loader + Option API
    └── errors            # sentinel errors + typed SurqlError with errors.Is/As
```

## Per-package summary

### `pkg/surql/types`

Type-safe primitives shared by the builder and CRUD layers:

- `Operator` interface + concrete `EqOp`, `GtOp`, `ContainsOp`, `AndOp`,
  `OrOp`, `NotOp`, `InsideOp`, `IsNullOp`, and more.
- `RecordID`, `RecordIDValue`, `RecordRef` for typed record identifiers.
- `SurrealFn` + factories (`NewSurrealFn`, `SurqlFn`, `TypeRecord`,
  `TypeThing`) for raw function expressions.
- `RawSurqlValue` marker interface so `Expression` / `SurrealFn` emit
  verbatim in SET / SELECT / WHERE without caller-side casts.
- `CoerceDatetime` / `CoerceRecordDatetimes` for v3 ISO-8601 handling.
- `SurrealReservedWords` + `CheckReservedWord` for DDL safety.

### `pkg/surql/query`

The immutable fluent builder, executor, and CRUD surface:

- `Query` type with `Select`, `Insert`, `Update`, `Upsert`, `Delete`,
  `Relate`, plus `SelectExpr`, `SelectAliased`, `From`, `Execute`
  (v0.2.0 additions).
- `Expression` helpers (`Field`, `Value`, `Raw`, `Func`, `MathMean`,
  `Count`, `Concat`, …) and the new `types.SurrealFn` factories
  (`MathAbs/Ceil/Floor/Round`, `StringLen/Lower/Upper/Concat`,
  `CountAll/Field/If`).
- Hints: `IndexHint`, `ParallelHint`, `TimeoutHint`, `FetchHint`,
  `ExplainHint` with `MergeHints` / `RenderHints` / `ValidateHint`.
- Executor: `ExecuteQuery`, `ExecuteRaw`, `FetchOne`, `FetchAll`,
  `FetchMany`, plus generic `ExecuteRawTyped`, `FetchRecords[T]`,
  `QueryRecordsWrapped`.
- CRUD: `CreateRecord` / `GetRecord` / `UpdateRecord` / `UpsertRecord` /
  `DeleteRecord`, generic `CreateTyped[T]` / `GetTyped[T]` /
  `UpdateTyped[T]`, and the new `*ByTarget` helpers
  (`GetByTarget`, `UpdateByTarget`, `UpsertByTarget`, `DeleteByTarget`,
  `ExistsByTarget`).
- Aggregation: `AggregateRecords` + `AggregateOpts` wrapping the
  projection + grouping in one call.
- Batch: `UpsertMany`, `InsertMany`, `RelateMany`, `DeleteMany`.
- Graph: `Traverse`, `TraverseWithDepth`, `CreateRelation`,
  `GetOutgoingEdges`, `GetIncomingEdges`, `ShortestPath`, plus
  `GraphQuery` fluent builder.
- Results: `QueryResult[T]`, `RecordResult[T]`, `ListResult[T]`,
  `PaginatedResult[T]`, `CountResult`, `AggregateResult` + raw-envelope
  extractors `ExtractOne`, `ExtractResult`, `ExtractScalar`, `HasResults`.

### `pkg/surql/schema`

Code-first DSL for every `DEFINE` statement SurrealDB emits:

- Tables (`NewTableDefinition`, `WithMode`, `WithFields`, `WithIndexes`,
  `WithEvents`, `WithPermissions`).
- Edges (`NewEdgeDefinition`, `WithEdgeFrom`, `WithEdgeTo`,
  `WithEdgeMode`).
- Fields (`StringField`, `IntField`, `FloatField`, `BoolField`,
  `DatetimeField`, `DurationField`, `DecimalField`, `ObjectField`,
  `ArrayField`, `RecordField`, `ComputedField`) with chainable
  `WithAssertion / WithDefault / WithValue / WithReadonly /
  WithPermissions`.
- Indexes (`Index`, `UniqueIndex`, `SearchIndex`, `MTreeIndex`,
  `HnswIndex`).
- Events (`Event().WithWhen().WithThen()`).
- Access (`NewAccessDefinition`, `AccessTypeJwt`, `AccessTypeRecord`).
- Registry + SQL generation (`NewSchemaRegistry`, `RegisterTable`,
  `RegisterEdge`, `GenerateSchemaSQL`).
- Parser (`ParseDBInfo`, `ParseTableInfo`, `ParseEdgeInfo`).
- Validator (`ValidateSchema`, `ValidationReport` with severity
  filtering, `CompareSchemas`).
- Visualizer (`GenerateMermaid`, `GenerateGraphViz`, `GenerateASCII`)
  with modern / dark / forest / minimal themes via `GetTheme(name)`.

### `pkg/surql/migration`

Full migration lifecycle:

- Discovery + file format (`DiscoverMigrations`, `LoadMigration`,
  SHA-256-checksummed `.surql` files with metadata / @up / @down
  markers).
- Diff engine (`DiffSchemas`, `DiffTables`, `DiffFields`, `DiffIndexes`,
  `DiffEvents`, `DiffPermissions`, `DiffEdges`).
- Generator (`GenerateMigration`, `GenerateInitialMigration`,
  `GenerateMigrationFromDiffs`, `CreateBlankMigration`).
- Executor (`MigrateUp`, `MigrateDown`, `ExecuteMigrationPlan`,
  `ExecuteRollback`, `PlanRollbackToVersion`).
- Status + history (`GetMigrationStatus`, `GetMigrationHistory`,
  `GetAppliedMigrationsOrdered`).
- Versioning (`SchemaSnapshot`, `VersionGraph`, `CreateSnapshot`,
  `StoreSnapshot`, `ListSnapshots`, `CompareSnapshots`).
- Squash + watcher + hooks (`SquashMigrationsWithOptions`,
  `NewSchemaWatcher`, atomic auto-snapshot hooks with a global toggle).

### `pkg/surql/connection`

Runtime connection surface:

- `DatabaseClient` wrapping `*surrealdb.DB` with `Connect` /
  `Disconnect` / `Query` / `QueryWithVars` / `Health` / full CRUD.
- Credentials: `RootCredentials`, `NamespaceCredentials`,
  `DatabaseCredentials`, `ScopeCredentials`, `TokenAuth`.
- Interactive transactions via `DatabaseClient.Begin` returning a
  `Transaction` with `Execute` / `ExecuteWithVars` / `Commit` /
  `Rollback` / `State` (WebSocket only; see
  [v3 Patterns](v3-patterns.md)).
- Live queries via `DatabaseClient.Live` returning a `LiveQuery` with
  notification channel + `Close`.
- `AuthManager` for signin / signup / authenticate / invalidate /
  refresh lifecycle bookkeeping.
- `StreamingManager` for spawning + tracking multiple live queries.
- Context helpers (`SetDB` / `GetDB` / `ConnectionScope` /
  `ConnectionOverride`).
- Registry (`GetRegistry`, `Registry.Register`, `Registry.Get`) for
  multi-environment clients shared across packages.

### `pkg/surql/cache`

Memory + Redis cache backends, a `Manager` that fans out to multiple
backends, a decorator helper (`Cached(...)`) for wrapping a function
in a cache, and a stats tracker.

### `pkg/surql/orchestration`

Multi-environment deployment coordinator:

- `MigrationCoordinator.DeployToEnvironments` with four strategies
  (sequential, parallel, rolling, canary).
- `DeployOptions` (strategy, batch size, canary percent, max
  concurrency, dry-run, verify-health, auto-rollback).
- `EnvironmentConfig` + `Registry` (`LoadRegistryFromFile`,
  `Get`, `List`).
- `HealthCheck.VerifyAllEnvironments` for pre-deploy probes.
- `DeploymentResult` / `DeploymentStatus` for result reporting.

### `pkg/surql/settings`

Unified settings loader:

- `LoadSettings(opts...)` walks `surql.yaml` / `surql.toml` up the tree
  and falls back to `SURQL_*` env vars.
- `Settings` struct exposes `Database`, `MigrationPath`, `Cache`,
  `Orchestration`.
- `WithConfigFile`, `WithEnvPrefix`, `WithSource` options for
  test fixtures + explicit overrides.

### `pkg/surql/errors`

Sentinel errors (`ErrValidation`, `ErrConnection`, `ErrNotFound`,
`ErrTransaction`, `ErrStreaming`, `ErrSerialization`, …) plus a typed
`SurqlError` that composes through `errors.Is` / `errors.As` and
preserves the underlying cause.

## What's next

- **[CLI Reference](cli.md)** — the `surql` subcommand surface.
- **[v3 Patterns](v3-patterns.md)** — v3 behaviour per package.
- **[Query UX](query-ux.md)** — the new first-class helpers.
