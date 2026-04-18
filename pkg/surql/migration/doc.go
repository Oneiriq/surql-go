// Package migration provides schema migration primitives for surql.
//
// This package is a Go port of surql-py/src/surql/migration/. The current
// increment covers only migration models and file-system discovery. Diff,
// executor, generator, history store, hooks, rollback, squash, versioning,
// and watcher tooling are tracked in follow-up work.
//
// # File format
//
// Python loads `.py` migration files at runtime and invokes `up()` / `down()`
// callables plus a `metadata` dict. Go has no equivalent dynamic-import
// mechanism, so the Go port switches to a flat-file `.surql` format with
// section markers:
//
//	-- @description: create user table
//	-- @depends_on: 20260101_000000_init
//	-- @up
//	DEFINE TABLE user SCHEMAFULL;
//	DEFINE FIELD email ON user TYPE string;
//	-- @down
//	REMOVE TABLE user;
//
// Rules:
//
//   - The file name follows the Python convention:
//     YYYYMMDD_HHMMSS_description.surql.
//   - `-- @up` and `-- @down` markers are required; content above `-- @up`
//     is header metadata, content between `-- @up` and `-- @down` is the
//     forward migration, and content after `-- @down` is the rollback.
//   - Statements are split on semicolons; empty statements and pure
//     comment lines inside a section are discarded.
//   - `-- @description:` and `-- @depends_on:` headers are optional.
//     When missing, description falls back to the filename-derived value
//     and depends_on defaults to the empty slice.
//
// The migration Checksum is a SHA-256 digest of the raw file contents,
// matching the Python port for cross-language compatibility.
package migration
