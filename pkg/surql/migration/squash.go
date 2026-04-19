package migration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// SquashError is the sentinel error returned when a squash operation cannot
// be completed safely. It wraps surqlerrors.ErrMigrationSquash so callers
// that already filter on the package-level sentinel continue to work; new
// callers can match on SquashError directly via errors.Is.
var SquashError = surqlerrors.ErrMigrationSquash

// squashedFromMarker is emitted in the squashed migration's description
// header so that discovery tooling can recognise the provenance of the
// squashed file without re-parsing the full body.
const squashedFromMarker = "-- @squashed-from:"

// SquashWarning captures a potential issue with a squash operation. The
// severity field mirrors surql-py's validate_squash_safety so callers can
// decide whether to abort or proceed with a warning log.
type SquashWarning struct {
	// Migration is the source migration version that produced the warning.
	Migration string `json:"migration"`
	// Message is a human-readable description of the concern.
	Message string `json:"message"`
	// Severity is one of "low", "medium", "high".
	Severity string `json:"severity"`
}

// SquashResult is returned from a successful SquashMigrations call.
type SquashResult struct {
	// SquashedPath is the absolute path to the newly written migration file.
	SquashedPath string `json:"squashed_path"`
	// Version is the YYYYMMDD_HHMMSS version of the new migration.
	Version string `json:"version"`
	// Checksum is the SHA-256 digest of the generated file's contents.
	Checksum string `json:"checksum"`
	// OriginalCount is the number of migrations combined.
	OriginalCount int `json:"original_count"`
	// StatementCount is the number of up statements in the squashed output
	// after any optimisation pass.
	StatementCount int `json:"statement_count"`
	// OptimizationsApplied is the number of redundant statements removed.
	OptimizationsApplied int `json:"optimizations_applied"`
	// OriginalMigrations lists the versions of the source migrations, in
	// discovery (ascending) order.
	OriginalMigrations []string `json:"original_migrations"`
	// Warnings lists non-fatal issues detected during safety validation.
	Warnings []SquashWarning `json:"warnings,omitempty"`
}

// SquashOptions configures the SquashMigrations operation. The zero value
// is valid and applies the default behaviour (optimise on, write to disk).
type SquashOptions struct {
	// Description is the human-readable summary to emit for the squashed
	// migration. When empty a synthetic "squashed_<from>_to_<to>" slug is used.
	Description string
	// OutputPath is an explicit destination file path. When empty the
	// squashed migration is written to the source directory under the
	// canonical <version>_<slug>.surql name.
	OutputPath string
	// Optimize controls whether redundant DEFINE / REMOVE pairs and
	// duplicate DEFINEs are removed from the merged statement list.
	// Defaults to true; pass &false to disable.
	Optimize *bool
	// DryRun skips the on-disk write and returns a result with an empty
	// Checksum and the would-be SquashedPath.
	DryRun bool
}

// SquashMigrations combines every migration in directory whose version is
// within [fromVersion, toVersion] (inclusive, empty strings treated as
// open-ended) into a single migration file.
//
// The new file carries a merged up section (each source migration's
// UpStatements concatenated in ascending version order, optionally
// optimised) and a merged down section (each migration's DownStatements
// concatenated in reverse order). Its description is annotated with the
// "-- @squashed-from: vA..vZ" marker so downstream tooling can recognise
// the provenance. A fresh SHA-256 checksum is computed from the generated
// body and returned in the SquashResult.
//
// The operation is atomic on the filesystem (via GenerateMigration's
// temp-file + rename) and never mutates the source migrations. Duplicate
// INSERT / UPDATE / DELETE statements that surql-py flags as high-severity
// abort the call with a wrapped SquashError so the caller can review and
// retry with an explicit safety override.
func SquashMigrations(
	ctx context.Context,
	directory, fromVersion, toVersion string,
) (*SquashResult, error) {
	return SquashMigrationsWithOptions(ctx, directory, fromVersion, toVersion, SquashOptions{})
}

// SquashMigrationsWithOptions is the configurable variant of
// SquashMigrations. See SquashOptions for field semantics.
func SquashMigrationsWithOptions(
	ctx context.Context,
	directory, fromVersion, toVersion string,
	opts SquashOptions,
) (*SquashResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, surqlerrors.Wrap(SquashError, "context cancelled before squash", err)
	}

	all, err := DiscoverMigrations(directory)
	if err != nil {
		return nil, surqlerrors.Wrap(SquashError, "failed to discover migrations", err)
	}
	if len(all) == 0 {
		return nil, surqlerrors.New(SquashError, "no migrations found in directory")
	}

	selected := filterMigrationsByVersion(all, fromVersion, toVersion)
	if len(selected) == 0 {
		return nil, surqlerrors.New(SquashError, "no migrations match the specified version range")
	}
	if len(selected) < 2 {
		return nil, surqlerrors.New(SquashError, "at least 2 migrations are required for squashing")
	}

	warnings := validateSquashSafety(selected)
	var highSeverity []SquashWarning
	for _, w := range warnings {
		if w.Severity == "high" {
			highSeverity = append(highSeverity, w)
		}
	}
	if len(highSeverity) > 0 {
		msgs := make([]string, 0, len(highSeverity))
		for _, w := range highSeverity {
			msgs = append(msgs, w.Message)
		}
		return nil, surqlerrors.Newf(SquashError,
			"high severity warnings prevent squashing: %s",
			strings.Join(msgs, "; "),
		)
	}

	up := make([]string, 0)
	versions := make([]string, 0, len(selected))
	for _, m := range selected {
		up = append(up, m.UpStatements...)
		versions = append(versions, m.Version)
	}

	// Down is the reverse-order concatenation of each source's down. We
	// reverse both the list of migrations and (within each) the statement
	// order so that the resulting script undoes them in the inverse
	// sequence of their up section.
	down := make([]string, 0)
	for i := len(selected) - 1; i >= 0; i-- {
		stmts := selected[i].DownStatements
		for j := len(stmts) - 1; j >= 0; j-- {
			down = append(down, stmts[j])
		}
	}

	optimize := true
	if opts.Optimize != nil {
		optimize = *opts.Optimize
	}
	optimizationsApplied := 0
	if optimize {
		up, optimizationsApplied = optimizeStatements(up)
	}

	firstVersion := selected[0].Version
	lastVersion := selected[len(selected)-1].Version
	description := opts.Description
	if strings.TrimSpace(description) == "" {
		description = fmt.Sprintf("squashed_%s_to_%s", firstVersion, lastVersion)
	}

	version := now().Format(timestampLayout)
	outputDir := directory
	if opts.OutputPath != "" {
		// When the caller supplied an explicit file we still delegate to
		// writeMigrationFile (which derives its own name), but we respect
		// the requested parent directory.
		outputDir = filepath.Dir(opts.OutputPath)
	}

	extraHeader := []string{
		fmt.Sprintf("%s %s..%s", squashedFromMarker, firstVersion, lastVersion),
	}

	if opts.DryRun {
		result := &SquashResult{
			SquashedPath:         buildSquashedPath(outputDir, version, description),
			Version:              version,
			Checksum:             "",
			OriginalCount:        len(selected),
			StatementCount:       len(up),
			OptimizationsApplied: optimizationsApplied,
			OriginalMigrations:   versions,
			Warnings:             warnings,
		}
		return result, nil
	}

	written, err := writeSquashedMigrationFile(
		version,
		description,
		description,
		extraHeader,
		up,
		down,
		outputDir,
	)
	if err != nil {
		return nil, surqlerrors.Wrap(SquashError, "failed to write squashed migration", err)
	}

	return &SquashResult{
		SquashedPath:         written.Path,
		Version:              written.Version,
		Checksum:             written.Checksum,
		OriginalCount:        len(selected),
		StatementCount:       len(up),
		OptimizationsApplied: optimizationsApplied,
		OriginalMigrations:   versions,
		Warnings:             warnings,
	}, nil
}

// filterMigrationsByVersion returns the subset of migrations whose version
// is within [from, to] inclusive. Empty from / to strings are treated as
// open-ended bounds, matching surql-py's _filter_migrations_by_version.
func filterMigrationsByVersion(migrations []Migration, from, to string) []Migration {
	out := make([]Migration, 0, len(migrations))
	for _, m := range migrations {
		if from != "" && m.Version < from {
			continue
		}
		if to != "" && m.Version > to {
			continue
		}
		out = append(out, m)
	}
	// DiscoverMigrations already returns ascending order, but callers of
	// this helper may pass pre-filtered slices; re-sort to be safe.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Version < out[j].Version
	})
	return out
}

// ---------------------------------------------------------------------------
// Statement optimisation
// ---------------------------------------------------------------------------

// parsedStatement captures the minimal metadata needed to match DEFINE /
// REMOVE pairs across a squashed migration. The receiver semantics mirror
// surql-py's _ParsedStatement.
type parsedStatement struct {
	statement  string
	operation  string
	objectType string
	tableName  string
	fieldName  string
	indexName  string
}

var (
	reDefineTable = regexp.MustCompile(`(?i)^(DEFINE|REMOVE)\s+TABLE(?:\s+IF\s+(?:NOT\s+)?EXISTS)?\s+(\w+)`)
	reDefineField = regexp.MustCompile(`(?i)^(DEFINE|REMOVE)\s+FIELD(?:\s+IF\s+(?:NOT\s+)?EXISTS)?\s+(\w+)\s+ON\s+(?:TABLE\s+)?(\w+)`)
	reDefineIndex = regexp.MustCompile(`(?i)^(DEFINE|REMOVE)\s+INDEX(?:\s+IF\s+(?:NOT\s+)?EXISTS)?\s+(\w+)\s+ON\s+(?:TABLE\s+)?(\w+)`)
	reDefineEvent = regexp.MustCompile(`(?i)^(DEFINE|REMOVE)\s+EVENT(?:\s+IF\s+(?:NOT\s+)?EXISTS)?\s+(\w+)\s+ON\s+(?:TABLE\s+)?(\w+)`)
	reUpdateSet   = regexp.MustCompile(`(?i)\bSET\s+(\w+)\s*=`)
	reUpdateTable = regexp.MustCompile(`(?i)\bUPDATE\s+(\w+)\b`)
)

// parseStatement extracts the operation and object metadata from a single
// SurrealQL statement. Only DEFINE, REMOVE, INSERT, UPDATE, DELETE, CREATE
// are recognised — everything else is returned with operation=="UNKNOWN".
func parseStatement(statement string) parsedStatement {
	trimmed := strings.TrimSpace(statement)
	upper := strings.ToUpper(trimmed)

	operation := ""
	switch {
	case strings.HasPrefix(upper, "DEFINE"):
		operation = "DEFINE"
	case strings.HasPrefix(upper, "REMOVE"):
		operation = "REMOVE"
	case strings.HasPrefix(upper, "INSERT"):
		operation = "INSERT"
	case strings.HasPrefix(upper, "UPDATE"):
		operation = "UPDATE"
	case strings.HasPrefix(upper, "DELETE"):
		operation = "DELETE"
	case strings.HasPrefix(upper, "CREATE"):
		operation = "CREATE"
	default:
		return parsedStatement{statement: trimmed, operation: "UNKNOWN"}
	}

	out := parsedStatement{statement: trimmed, operation: operation}

	// Order matters: FIELD / INDEX / EVENT patterns are more specific than
	// TABLE because they contain an additional "ON TABLE" clause.
	if m := reDefineField.FindStringSubmatch(trimmed); m != nil {
		out.objectType = "FIELD"
		out.fieldName = strings.ToLower(m[2])
		out.tableName = strings.ToLower(m[3])
		return out
	}
	if m := reDefineIndex.FindStringSubmatch(trimmed); m != nil {
		out.objectType = "INDEX"
		out.indexName = strings.ToLower(m[2])
		out.tableName = strings.ToLower(m[3])
		return out
	}
	if m := reDefineEvent.FindStringSubmatch(trimmed); m != nil {
		out.objectType = "EVENT"
		// Reuse indexName for event identifier (matches surql-py).
		out.indexName = strings.ToLower(m[2])
		out.tableName = strings.ToLower(m[3])
		return out
	}
	if m := reDefineTable.FindStringSubmatch(trimmed); m != nil {
		out.objectType = "TABLE"
		out.tableName = strings.ToLower(m[2])
		return out
	}
	return out
}

// optimizeStatements removes redundant SurrealQL statements from a squashed
// list. Three passes run in sequence:
//
//  1. DEFINE + REMOVE pairs for the same object are dropped (the net effect
//     on the schema is a no-op).
//  2. Duplicate DEFINE statements for the same object keep only the last
//     occurrence; earlier ones are removed.
//  3. UPDATE statements that target a field that was removed in pass (1)
//     are also dropped, since the field no longer exists.
//
// The returned int is the number of statements removed.
func optimizeStatements(statements []string) ([]string, int) {
	optimizations := 0
	parsed := make([]parsedStatement, len(statements))
	for i, s := range statements {
		parsed[i] = parseStatement(s)
	}

	toRemove := make(map[int]struct{})

	// Pass 1: DEFINE + REMOVE pairs.
	for i, stmtI := range parsed {
		if _, skip := toRemove[i]; skip {
			continue
		}
		if stmtI.operation != "DEFINE" {
			continue
		}
		for j := i + 1; j < len(parsed); j++ {
			if _, skip := toRemove[j]; skip {
				continue
			}
			stmtJ := parsed[j]
			if stmtJ.operation != "REMOVE" {
				continue
			}
			if stmtI.objectType != stmtJ.objectType || stmtI.tableName != stmtJ.tableName {
				continue
			}
			match := false
			switch stmtI.objectType {
			case "TABLE":
				match = true
			case "FIELD":
				match = stmtI.fieldName == stmtJ.fieldName
			case "INDEX", "EVENT":
				match = stmtI.indexName == stmtJ.indexName
			}
			if !match {
				continue
			}
			toRemove[i] = struct{}{}
			toRemove[j] = struct{}{}
			optimizations += 2
			break
		}
	}

	// Pass 2: duplicate DEFINE statements (keep the last occurrence).
	type defineKey struct {
		objectType string
		tableName  string
		fieldName  string
		indexName  string
	}
	defines := make(map[defineKey]int)
	for i, stmt := range parsed {
		if _, skip := toRemove[i]; skip {
			continue
		}
		if stmt.operation != "DEFINE" {
			continue
		}
		key := defineKey{
			objectType: stmt.objectType,
			tableName:  stmt.tableName,
			fieldName:  stmt.fieldName,
			indexName:  stmt.indexName,
		}
		if earlier, ok := defines[key]; ok {
			if _, already := toRemove[earlier]; !already {
				toRemove[earlier] = struct{}{}
				optimizations++
			}
		}
		defines[key] = i
	}

	// Pass 3: UPDATE statements against fields removed in pass (1).
	type fieldKey struct {
		tableName string
		fieldName string
	}
	removedFields := make(map[fieldKey]struct{})
	for idx := range toRemove {
		stmt := parsed[idx]
		if stmt.objectType == "FIELD" && stmt.tableName != "" && stmt.fieldName != "" {
			removedFields[fieldKey{stmt.tableName, stmt.fieldName}] = struct{}{}
		}
	}
	if len(removedFields) > 0 {
		for i, stmt := range parsed {
			if _, skip := toRemove[i]; skip {
				continue
			}
			if stmt.operation != "UPDATE" {
				continue
			}
			for key := range removedFields {
				setMatch := reUpdateSet.FindStringSubmatch(stmt.statement)
				tableMatch := reUpdateTable.FindStringSubmatch(stmt.statement)
				if setMatch == nil || tableMatch == nil {
					continue
				}
				if strings.EqualFold(setMatch[1], key.fieldName) &&
					strings.EqualFold(tableMatch[1], key.tableName) {
					toRemove[i] = struct{}{}
					optimizations++
					break
				}
			}
		}
	}

	out := make([]string, 0, len(statements)-len(toRemove))
	for i, s := range statements {
		if _, skip := toRemove[i]; skip {
			continue
		}
		out = append(out, s)
	}
	return out, optimizations
}

// validateSquashSafety walks the merged statement list and returns
// warnings for patterns that surql-py treats as risky to squash: INSERT,
// non-backfill UPDATE, DELETE, CREATE (non-table), and record-reference
// DEFINE FIELDs. Severity follows the reference implementation.
func validateSquashSafety(migrations []Migration) []SquashWarning {
	var warnings []SquashWarning
	for _, m := range migrations {
		for _, stmt := range m.UpStatements {
			trimmed := strings.TrimSpace(stmt)
			upper := strings.ToUpper(trimmed)

			switch {
			case strings.HasPrefix(upper, "INSERT"):
				warnings = append(warnings, SquashWarning{
					Migration: m.Version,
					Message:   "Contains INSERT statement: " + truncateForWarning(trimmed),
					Severity:  "medium",
				})
			case strings.HasPrefix(upper, "UPDATE") && strings.Contains(upper, "SET"):
				// Only warn when the UPDATE is not a backfill
				// (WHERE ... IS NONE pattern).
				if !strings.Contains(upper, "IS NONE") {
					warnings = append(warnings, SquashWarning{
						Migration: m.Version,
						Message:   "Contains UPDATE statement: " + truncateForWarning(trimmed),
						Severity:  "medium",
					})
				}
			case strings.HasPrefix(upper, "DELETE"):
				warnings = append(warnings, SquashWarning{
					Migration: m.Version,
					Message:   "Contains DELETE statement: " + truncateForWarning(trimmed),
					Severity:  "high",
				})
			case strings.HasPrefix(upper, "CREATE") && !strings.Contains(upper, "CREATE TABLE"):
				warnings = append(warnings, SquashWarning{
					Migration: m.Version,
					Message:   "Contains CREATE statement: " + truncateForWarning(trimmed),
					Severity:  "low",
				})
			}

			if strings.Contains(upper, "RECORD") && strings.Contains(upper, "TYPE") {
				warnings = append(warnings, SquashWarning{
					Migration: m.Version,
					Message:   "Contains record reference - verify table order",
					Severity:  "low",
				})
			}
		}
	}
	return warnings
}

// truncateForWarning keeps warning messages bounded so logs stay scannable
// while still showing a reasonable prefix of the offending statement.
func truncateForWarning(stmt string) string {
	const limit = 50
	if len(stmt) <= limit {
		return stmt
	}
	return stmt[:limit] + "..."
}

// buildSquashedPath mirrors writeMigrationFile's naming so DryRun callers
// get an accurate preview without writing anything to disk.
func buildSquashedPath(dir, version, description string) string {
	filename := BuildFilename(version, description)
	if dir == "" {
		return filename
	}
	return filepath.Join(dir, filename)
}

// writeSquashedMigrationFile renders a migration file body that includes a
// `-- @squashed-from:` marker line in the header (in addition to the usual
// description / up / down sections) and writes it atomically.
//
// The marker is preserved verbatim when the file is re-parsed because
// parseMigrationContent ignores unknown header comment lines, keeping the
// round-trip contract intact.
func writeSquashedMigrationFile(
	version string,
	slugSource string,
	description string,
	extraHeader []string,
	up []string,
	down []string,
	dir string,
) (Migration, error) {
	if strings.TrimSpace(dir) == "" {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"migration directory must not be empty",
		)
	}

	filename := BuildFilename(version, slugSource)
	if !ValidateMigrationName(filename) {
		return Migration{}, surqlerrors.Newf(
			surqlerrors.ErrMigrationGeneration,
			"generated filename %q is not a valid migration name", filename,
		)
	}

	// Render the canonical body and splice the squashed-from header in
	// directly before the `-- @up` marker so the resulting file still
	// matches the layout produced by GenerateMigration.
	body := renderMigrationContent(description, nil, up, down)
	if len(extraHeader) > 0 {
		marker := "-- @up\n"
		idx := strings.Index(body, marker)
		if idx == -1 {
			return Migration{}, surqlerrors.New(
				surqlerrors.ErrMigrationGeneration,
				"rendered migration body is missing the -- @up marker",
			)
		}
		var b strings.Builder
		b.WriteString(body[:idx])
		for _, line := range extraHeader {
			b.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				b.WriteByte('\n')
			}
		}
		b.WriteString(body[idx:])
		body = b.String()
	}

	finalPath := filepath.Join(dir, filename)
	// Ensure the directory exists before attempting the atomic rename.
	if err := ensureDir(dir); err != nil {
		return Migration{}, err
	}
	if err := atomicWriteFile(finalPath, []byte(body)); err != nil {
		return Migration{}, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationGeneration, err,
			"failed to write migration %q", finalPath,
		)
	}

	m, err := LoadMigration(finalPath)
	if err != nil {
		return Migration{}, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationGeneration, err,
			"generator produced invalid migration file %q", finalPath,
		)
	}
	return m, nil
}

// ensureDir creates dir (and intermediate directories) with 0755 perms if
// it does not yet exist. Existing directories are a no-op.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return surqlerrors.Wrapf(
			surqlerrors.ErrMigrationGeneration, err,
			"cannot create migration directory %q", dir,
		)
	}
	return nil
}
