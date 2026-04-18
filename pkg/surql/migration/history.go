package migration

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/albedosehen/surql-go/pkg/surql/connection"
	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// MigrationTableName is the SurrealDB table that tracks applied migrations.
// Migration tooling defines this table schemafully on first use and never
// drops it.
const MigrationTableName = "_migration_history"

// CreateMigrationTable defines the migration history table and its schema.
//
// The operation is idempotent: every DEFINE statement uses SurrealDB's
// IF NOT EXISTS form, so repeat invocations are safe. The table is
// schemafull with a UNIQUE index on the version column; execution time is
// stored in milliseconds (nullable).
func CreateMigrationTable(ctx context.Context, client *connection.DatabaseClient) error {
	if client == nil {
		return surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}

	statements := []string{
		fmt.Sprintf("DEFINE TABLE IF NOT EXISTS %s SCHEMAFULL;", MigrationTableName),
		fmt.Sprintf("DEFINE FIELD IF NOT EXISTS version ON TABLE %s TYPE string;", MigrationTableName),
		fmt.Sprintf("DEFINE FIELD IF NOT EXISTS description ON TABLE %s TYPE string;", MigrationTableName),
		fmt.Sprintf("DEFINE FIELD IF NOT EXISTS applied_at ON TABLE %s TYPE datetime;", MigrationTableName),
		fmt.Sprintf("DEFINE FIELD IF NOT EXISTS checksum ON TABLE %s TYPE string;", MigrationTableName),
		fmt.Sprintf("DEFINE FIELD IF NOT EXISTS execution_time_ms ON TABLE %s TYPE option<int>;", MigrationTableName),
		fmt.Sprintf("DEFINE INDEX IF NOT EXISTS version_idx ON TABLE %s COLUMNS version UNIQUE;", MigrationTableName),
	}

	for _, stmt := range statements {
		if _, err := client.Query(ctx, stmt); err != nil {
			return surqlerrors.Wrapf(
				surqlerrors.ErrMigrationHistory, err,
				"failed to create migration history table",
			)
		}
	}
	return nil
}

// EnsureMigrationTable creates the migration history table when it does not
// already exist. Callers of RecordMigration / GetAppliedMigrations do not
// need to call this directly; the CRUD helpers do so implicitly.
func EnsureMigrationTable(ctx context.Context, client *connection.DatabaseClient) error {
	return CreateMigrationTable(ctx, client)
}

// RecordMigration inserts a new history entry.
//
// entry.Version must be non-empty; AppliedAt is stored verbatim (UTC is
// strongly recommended). Duplicate versions are rejected by the UNIQUE
// index defined in CreateMigrationTable.
func RecordMigration(ctx context.Context, client *connection.DatabaseClient, entry MigrationHistory) error {
	if client == nil {
		return surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if entry.Version == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "history entry version cannot be empty")
	}

	if err := EnsureMigrationTable(ctx, client); err != nil {
		return err
	}

	appliedAt := entry.AppliedAt
	if appliedAt.IsZero() {
		appliedAt = time.Now().UTC()
	}

	// SurrealDB v3 rejects bare ISO strings for datetime-typed fields
	// ("Expected `datetime` but found '...'"), so we cast explicitly via
	// the SurrealQL `<datetime>` prefix inside a raw CREATE statement.
	vars := map[string]any{
		"version":     entry.Version,
		"description": entry.Description,
		"applied_at":  appliedAt.UTC().Format(time.RFC3339Nano),
		"checksum":    entry.Checksum,
	}
	sql := "CREATE " + MigrationTableName + " SET " +
		"version = $version, " +
		"description = $description, " +
		"applied_at = <datetime> $applied_at, " +
		"checksum = $checksum"
	if entry.ExecutionTimeMs != nil {
		vars["execution_time_ms"] = *entry.ExecutionTimeMs
		sql += ", execution_time_ms = $execution_time_ms"
	}
	sql += ";"

	if _, err := client.QueryWithVars(ctx, sql, vars); err != nil {
		return surqlerrors.Wrapf(
			surqlerrors.ErrMigrationHistory, err,
			"failed to record migration %q", entry.Version,
		)
	}
	return nil
}

// RemoveMigrationRecord deletes the history entry for the given version.
//
// A missing record is not an error (matches the Python port's silent
// warning); the call short-circuits after the SELECT lookup.
func RemoveMigrationRecord(ctx context.Context, client *connection.DatabaseClient, version string) error {
	if client == nil {
		return surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if version == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "version cannot be empty")
	}
	if err := EnsureMigrationTable(ctx, client); err != nil {
		return err
	}

	query := fmt.Sprintf(
		"DELETE %s WHERE version = $version;",
		MigrationTableName,
	)
	if _, err := client.QueryWithVars(ctx, query, map[string]any{"version": version}); err != nil {
		return surqlerrors.Wrapf(
			surqlerrors.ErrMigrationHistory, err,
			"failed to remove migration record %q", version,
		)
	}
	return nil
}

// GetAppliedMigrations returns every entry from the migration history table,
// ordered by AppliedAt ascending.
func GetAppliedMigrations(ctx context.Context, client *connection.DatabaseClient) ([]MigrationHistory, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if err := EnsureMigrationTable(ctx, client); err != nil {
		return nil, err
	}

	query := fmt.Sprintf("SELECT * FROM %s ORDER BY applied_at ASC;", MigrationTableName)
	res, err := client.Query(ctx, query)
	if err != nil {
		return nil, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationHistory, err,
			"failed to fetch applied migrations",
		)
	}

	records := extractRecords(res)
	migrations := make([]MigrationHistory, 0, len(records))
	for _, rec := range records {
		entry, ok := decodeHistoryRecord(rec)
		if !ok {
			continue
		}
		migrations = append(migrations, entry)
	}
	return migrations, nil
}

// IsMigrationApplied reports whether the given version has a history entry.
func IsMigrationApplied(ctx context.Context, client *connection.DatabaseClient, version string) (bool, error) {
	if client == nil {
		return false, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	if version == "" {
		return false, surqlerrors.New(surqlerrors.ErrValidation, "version cannot be empty")
	}

	applied, err := GetAppliedMigrations(ctx, client)
	if err != nil {
		return false, err
	}
	for _, m := range applied {
		if m.Version == version {
			return true, nil
		}
	}
	return false, nil
}

// GetMigrationHistory returns every entry ordered by AppliedAt ascending.
// This is a thin alias over GetAppliedMigrations that guarantees the
// ordering contract for callers (notably the CLI's `status` command).
func GetMigrationHistory(ctx context.Context, client *connection.DatabaseClient) ([]MigrationHistory, error) {
	entries, err := GetAppliedMigrations(ctx, client)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].AppliedAt.Before(entries[j].AppliedAt)
	})
	return entries, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// extractRecords pulls the []map[string]any payload out of the envelope
// produced by DatabaseClient.Query. The client returns []any where each
// entry is {status, time, result}; we surface only `result` arrays.
func extractRecords(result any) []map[string]any {
	if result == nil {
		return nil
	}
	arr, ok := result.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0)
	for _, entry := range arr {
		envelope, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		raw, hasResult := envelope["result"]
		if !hasResult {
			// Flat records (e.g. from db.select) are admitted verbatim.
			out = append(out, envelope)
			continue
		}
		appendResultValues(&out, raw)
	}
	return out
}

// appendResultValues normalises the `result` field, which may be a list of
// records, a single record, or nil.
func appendResultValues(out *[]map[string]any, raw any) {
	switch v := raw.(type) {
	case nil:
		return
	case []any:
		for _, item := range v {
			if rec, ok := item.(map[string]any); ok {
				*out = append(*out, rec)
			}
		}
	case map[string]any:
		*out = append(*out, v)
	}
}

// decodeHistoryRecord converts a raw SurrealDB record into a
// MigrationHistory. Records missing the mandatory fields are skipped.
func decodeHistoryRecord(rec map[string]any) (MigrationHistory, bool) {
	version, ok := rec["version"].(string)
	if !ok || version == "" {
		return MigrationHistory{}, false
	}
	description, _ := rec["description"].(string)
	checksum, _ := rec["checksum"].(string)
	appliedAt := parseHistoryDatetime(rec["applied_at"])

	entry := MigrationHistory{
		Version:     version,
		Description: description,
		AppliedAt:   appliedAt,
		Checksum:    checksum,
	}
	if raw, has := rec["execution_time_ms"]; has && raw != nil {
		if ms, ok := coerceInt64(raw); ok {
			entry.ExecutionTimeMs = &ms
		}
	}
	return entry, true
}

// parseHistoryDatetime accepts time.Time, RFC3339 strings, or unix seconds
// and returns a UTC time.Time. Unparseable values degrade to the zero time.
func parseHistoryDatetime(v any) time.Time {
	switch x := v.(type) {
	case time.Time:
		return x.UTC()
	case string:
		// SurrealDB typically emits RFC3339 with nanoseconds.
		if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
			return t.UTC()
		}
		if t, err := time.Parse(time.RFC3339, x); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// coerceInt64 extracts an int64 from the common JSON numeric encodings.
func coerceInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		return int64(n), true
	case float32:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}
