# CLI Reference

The `surql` binary wraps the library's migration, schema, database, and
orchestration surface behind a single cobra CLI.

```shell
go install github.com/Oneiriq/surql-go/cmd/surql@latest
```

## Global flags

| Flag            | Shorthand | Purpose                                                             |
|-----------------|-----------|---------------------------------------------------------------------|
| `--config PATH` |           | Explicit path to `surql.yaml` / `surql.toml` (overrides discovery). |
| `--verbose`     |           | Enable verbose logging.                                             |
| `--quiet`       | `-q`      | Suppress informational output.                                      |
| `--no-color`    |           | Disable ANSI color output.                                          |
| `--json`        |           | Emit JSON output where supported.                                   |
| `--version`     | `-v`      | Print the CLI version.                                              |

`version`, `help`, and `completion` run without requiring a resolvable
config file. Every other subcommand hydrates settings via
`pkg/surql/settings.LoadSettings` (searches for `surql.yaml` / `surql.toml`
up the directory tree, falling back to `SURQL_*` env vars).

## `surql migrate`

| Subcommand                              | Purpose                                                    |
|-----------------------------------------|------------------------------------------------------------|
| `migrate up [--steps N] [--target V]`   | Apply pending migrations; `--dry-run` previews the plan.   |
| `migrate down [--steps N] [--target V]` | Roll back applied migrations; `--dry-run` previews.        |
| `migrate status`                        | Applied vs pending counts (`--json` for machine output).   |
| `migrate history`                       | Table of applied migrations (version, description, when).  |
| `migrate create <description>`          | Write a blank `.surql` file with metadata / @up / @down.   |
| `migrate validate [version]`            | Structural validation over every migration file.           |
| `migrate generate <description>`        | Blank generator stub (schema-diff path not yet wired in).  |
| `migrate squash <from> <to>`            | Collapse a range into one file; `--dry-run` previews.      |

Common flags: `--directory / -d <path>` overrides the configured
migrations directory per invocation.

## `surql schema`

| Subcommand                     | Purpose                                                          |
|--------------------------------|------------------------------------------------------------------|
| `schema show [table]`          | Dump `INFO FOR DB` (or `INFO FOR TABLE <table>`) as JSON.        |
| `schema diff --from A --to B`  | Compare two stored snapshots (version or explicit path).         |
| `schema generate [-o PATH]`    | Render DEFINE statements from the code registry.                 |
| `schema sync [--dry-run]`      | Push the code registry directly to the database (destructive).   |
| `schema export [-f json/yaml]` | Export the live `INFO FOR DB` payload.                           |
| `schema tables`                | List tables in the configured database.                          |
| `schema inspect <table>`       | Dump field / index / event details for one table.                |
| `schema validate [--strict]`   | Run the schema validator over the code registry.                 |
| `schema check`                 | Compare the code registry against the live DB; fails on drift.   |
| `schema hook-config`           | Emit a pre-commit hook YAML snippet for `schema check`.          |
| `schema watch [--debounce d]`  | Watch a directory for `.surql` changes with debounced batches.   |
| `schema visualize`             | Render Mermaid / GraphViz / ASCII diagrams from the registry.    |

`schema visualize` accepts `--theme modern|dark|forest|minimal` and
`--format mermaid|graphviz|ascii`.

## `surql db`

| Subcommand                | Purpose                                                                    |
|---------------------------|----------------------------------------------------------------------------|
| `db init`                 | Create the migration history table.                                        |
| `db ping`                 | Verify connectivity against the resolved settings.                         |
| `db info`                 | Print resolved database configuration (password masked).                   |
| `db reset --yes`          | REMOVE every table in the configured DB (destructive; requires `--yes`).   |
| `db query <surql>`        | Execute a raw SurrealQL statement; JSON output by default.                 |
| `db query -f FILE`        | Read SurrealQL from `FILE`.                                                |
| `db version`              | Print the connected SurrealDB server version.                              |

## `surql orchestrate`

Multi-environment deployment commands (aliased as `surql orch`). Each
reads an `environments.json` registry via the `--plan` flag.

| Subcommand                                                         | Purpose                                               |
|--------------------------------------------------------------------|-------------------------------------------------------|
| `orchestrate deploy --plan plan.json -e dev,staging`               | Apply migrations across named environments.           |
| `orchestrate status --plan plan.json [-e a,b]`                     | Render health of each environment.                    |
| `orchestrate validate --plan plan.json`                            | Verify every environment can connect + has the table. |

Deploy strategies: `sequential` (default), `parallel` (`--max-concurrent`),
`rolling` (`--batch-size`), `canary` (`--canary-percent`). Rollback is on
by default; pass `--no-rollback` to disable. `--dry-run` traces the plan
without executing.

## `surql version`

Prints the library version plus (if built with goreleaser-style ldflags)
the commit and build date:

```shell
$ surql version
surql 0.2.1 (commit abc1234) built 2026-04-18T19:00:00Z
```

## Exit codes

| Code | Meaning                                                   |
|------|-----------------------------------------------------------|
| 0    | Success.                                                  |
| 1    | Runtime failure (validation, IO, execution).              |
| 2    | Usage error (missing flag, bad argument, unknown command).|

## What's next

- **[Migrations](migrations.md)** — the library-side of what `surql migrate` drives.
- **[Query UX](query-ux.md)** — helpers that underpin `surql db query` output.
- **[v3 Patterns](v3-patterns.md)** — v3 semantics surfaced by `schema` and `db` subcommands.
