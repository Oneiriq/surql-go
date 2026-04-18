# Migrations

## File format

Migration files are plain `.surql` files with three section markers:

```text
-- @metadata
-- version: 20260418_193300
-- description: Add user table
-- depends_on: [20260418_180000]
-- @up
DEFINE TABLE user SCHEMAFULL;
DEFINE FIELD email ON TABLE user TYPE string;
-- @down
REMOVE TABLE IF EXISTS user;
```

Version pattern: `YYYYMMDD_HHMMSS`. Descriptions are slug-cased by the
generator. Every file includes a SHA-256 checksum.

## Generating migrations

```go
import "github.com/Oneiriq/surql-go/pkg/surql/migration"

m, _ := migration.CreateBlankMigration("add_log_table", "Add log table", "migrations")
m, _  = migration.GenerateInitialMigration(reg, "migrations")
m, _  = migration.GenerateMigrationFromDiffs("rename_email", diffs, "migrations")
```

## Diffing

```go
code := migration.SchemaSnapshot{Tables: codeReg.Tables(), Edges: codeReg.Edges()}
db   := migration.SchemaSnapshot{Tables: dbTables, Edges: dbEdges}
diffs, _ := migration.DiffSchemas(code, db)
```

## Discovery

```go
migrations, _ := migration.DiscoverMigrations("migrations")
for _, m := range migrations {
    fmt.Println(m.Version, m.Description)
}
one, _ := migration.LoadMigration("migrations/20260418_193300_add_user_table.surql")
```

## Versioning + snapshots

```go
snap, _  := migration.CreateSnapshot(reg, "after user table")
migration.StoreSnapshot(snap, "snapshots")
all, _   := migration.ListSnapshots("snapshots")

diff, _  := migration.CompareSnapshots(all[0], all[1])

g := migration.NewVersionGraph()
for _, s := range all { g.Add(s) }
```

## What's next

- **[Query Builder](queries.md)** -- immutable fluent queries for your
  migrated schema.
