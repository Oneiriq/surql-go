# Quick Start

This walkthrough defines a small `user` table, generates an initial
migration, and inspects the resulting SurrealQL.

## 1. Define a schema

```go
package main

import (
    "fmt"
    "github.com/Oneiriq/surql-go/pkg/surql/schema"
)

func buildRegistry() (*schema.SchemaRegistry, error) {
    user, err := schema.NewTableDefinition(
        "user",
        schema.WithMode(schema.TableModeSchemafull),
        schema.WithFields(
            schema.StringField("name"),
            schema.StringField("email").WithAssertion("string::is::email($value)"),
            schema.IntField("age").WithAssertion("$value >= 0 AND $value <= 150"),
            schema.DatetimeField("created_at").WithDefault("time::now()").WithReadonly(true),
        ),
        schema.WithIndexes(schema.UniqueIndex("email_idx", []string{"email"})),
    )
    if err != nil {
        return nil, err
    }
    reg := schema.NewSchemaRegistry()
    if err := reg.RegisterTable(user); err != nil {
        return nil, err
    }
    return reg, nil
}
```

## 2. Render SurrealQL

```go
reg, _ := buildRegistry()
stmts, err := schema.GenerateSchemaSQL(reg, false)
if err != nil { panic(err) }
for _, s := range stmts { fmt.Println(s) }
```

Output:

```sql
DEFINE TABLE user SCHEMAFULL;
DEFINE FIELD name ON TABLE user TYPE string;
DEFINE FIELD email ON TABLE user TYPE string ASSERT string::is::email($value);
DEFINE FIELD age ON TABLE user TYPE int ASSERT $value >= 0 AND $value <= 150;
DEFINE FIELD created_at ON TABLE user TYPE datetime VALUE time::now() READONLY;
DEFINE INDEX email_idx ON TABLE user COLUMNS email UNIQUE;
```

## 3. Generate a migration file

```go
import "github.com/Oneiriq/surql-go/pkg/surql/migration"

m, err := migration.GenerateInitialMigration(reg, "migrations")
if err != nil { panic(err) }
fmt.Printf("wrote migration %s: %s\n", m.Version, m.Description)
```

Each file is a portable `.surql` document:

```text
-- @metadata
-- version: 20260418_193300
-- description: Initial schema
-- @up
DEFINE TABLE user SCHEMAFULL;
...
-- @down
REMOVE TABLE IF EXISTS user;
```

## 4. Diff code vs database

```go
snap := migration.SchemaSnapshot{Tables: reg.Tables(), Edges: reg.Edges()}
// db := /* fetched via INFO FOR DB once the client lands */
diffs, _ := migration.DiffSchemas(snap, db)
for _, d := range diffs {
    fmt.Printf("%s: %s\n", d.Operation, d.TargetName)
}
```

## What's next

- **[Schema Definition](schema.md)** -- fields, tables, edges, indexes,
  events, access rules.
- **[Migrations](migrations.md)** -- the full migration lifecycle.
- **[Query Builder](queries.md)** -- immutable fluent queries.
