package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/schema"
)

// timestampLayout is the UTC timestamp layout used for migration versions.
// Format: YYYYMMDD_HHMMSS (e.g. 20260102_120000).
const timestampLayout = "20060102_150405"

// slugFilterPattern is used to sanitize a description into a filename slug.
// Any character that is not lower-case alphanumeric or underscore is dropped.
var slugFilterPattern = regexp.MustCompile(`[^a-z0-9_]`)

// slugCollapsePattern collapses runs of underscores into a single underscore.
var slugCollapsePattern = regexp.MustCompile(`_+`)

// initialDescriptionDefault matches surql-py's default.
const initialDescriptionDefault = "Initial schema"

// now is the clock source used by the generator. Tests override this to make
// timestamps deterministic; production code always uses time.Now.
var now = func() time.Time { return time.Now().UTC() }

// GenerateMigration writes a migration file to dir containing the given up
// and down SurrealQL statements. The file name is derived from a UTC
// timestamp and a slug of name.
//
// Both upStatements and downStatements may be nil or empty, but at least one
// statement must be present across the two directions. An empty name is
// rejected with ErrMigrationGeneration. The returned Migration carries the
// absolute Path of the written file along with the parsed metadata.
//
// The write is atomic: a temporary file is created in dir and renamed to the
// final name only after it has been fully written and fsynced.
func GenerateMigration(
	name string,
	upStatements []string,
	downStatements []string,
	dir string,
) (Migration, error) {
	if strings.TrimSpace(name) == "" {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"migration name must not be empty",
		)
	}
	if len(upStatements) == 0 && len(downStatements) == 0 {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"migration must define at least one up or down statement",
		)
	}

	version := now().Format(timestampLayout)
	return writeMigrationFile(version, name, name, nil, upStatements, downStatements, dir)
}

// GenerateInitialMigration renders the full DEFINE script for every table and
// edge registered in registry as the up section of a single migration file.
// The down section is empty (initial migrations are not auto-reversible).
//
// A nil or empty registry is rejected with ErrMigrationGeneration.
func GenerateInitialMigration(registry *schema.SchemaRegistry, dir string) (Migration, error) {
	if registry == nil {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"registry must not be nil",
		)
	}
	if registry.TableCount() == 0 && registry.EdgeCount() == 0 {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"registry contains no tables or edges",
		)
	}

	script, err := schema.GenerateSchemaSQL(registry, false)
	if err != nil {
		return Migration{}, surqlerrors.Wrap(
			surqlerrors.ErrMigrationGeneration,
			"failed to render initial schema SQL",
			err,
		)
	}

	up := splitScriptStatements(script)
	if len(up) == 0 {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"registry produced no DEFINE statements",
		)
	}

	version := now().Format(timestampLayout)
	return writeMigrationFile(
		version,
		initialDescriptionDefault,
		initialDescriptionDefault,
		nil,
		up,
		nil,
		dir,
	)
}

// CreateBlankMigration writes an empty migration template to dir. The file
// contains no up or down statements; callers are expected to edit it before
// applying. name is used to form the filename slug, description (when
// non-empty) is written to the -- @description: header.
//
// An empty name is rejected with ErrMigrationGeneration.
func CreateBlankMigration(name, description, dir string) (Migration, error) {
	if strings.TrimSpace(name) == "" {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"migration name must not be empty",
		)
	}

	effectiveDescription := description
	if strings.TrimSpace(effectiveDescription) == "" {
		effectiveDescription = name
	}

	version := now().Format(timestampLayout)
	return writeMigrationFile(
		version,
		name,
		effectiveDescription,
		nil,
		nil,
		nil,
		dir,
	)
}

// GenerateMigrationFromDiffs converts a pre-computed list of SchemaDiff
// entries into a migration file. Forward SQL is emitted in diff order; down
// statements are emitted in reverse order to satisfy drop-before-define
// rollback semantics.
//
// An empty diffs slice is rejected with ErrMigrationGeneration.
func GenerateMigrationFromDiffs(
	name string,
	diffs []SchemaDiff,
	dir string,
) (Migration, error) {
	if strings.TrimSpace(name) == "" {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"migration name must not be empty",
		)
	}
	if len(diffs) == 0 {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"at least one diff is required",
		)
	}

	up := make([]string, 0, len(diffs))
	for _, d := range diffs {
		if d.ForwardSQL == "" {
			continue
		}
		up = append(up, splitScriptStatements(d.ForwardSQL)...)
	}

	down := make([]string, 0, len(diffs))
	for i := len(diffs) - 1; i >= 0; i-- {
		d := diffs[i]
		if d.BackwardSQL == "" {
			continue
		}
		down = append(down, splitScriptStatements(d.BackwardSQL)...)
	}

	if len(up) == 0 && len(down) == 0 {
		return Migration{}, surqlerrors.New(
			surqlerrors.ErrMigrationGeneration,
			"diffs produced no SQL statements",
		)
	}

	version := now().Format(timestampLayout)
	return writeMigrationFile(version, name, name, nil, up, down, dir)
}

// BuildFilename returns the canonical migration filename for the given
// version timestamp and description slug. It is exported primarily for
// tooling; callers of the generator API receive fully resolved paths back
// from the Migration.Path field.
func BuildFilename(version, description string) string {
	slug := sanitizeSlug(description)
	if slug == "" {
		slug = "migration"
	}
	return fmt.Sprintf("%s_%s.surql", version, slug)
}

// sanitizeSlug lower-cases the input, replaces spaces with underscores, drops
// any characters that are not alphanumeric or underscore, and collapses
// consecutive underscores.
func sanitizeSlug(s string) string {
	out := strings.ToLower(strings.TrimSpace(s))
	out = strings.ReplaceAll(out, "-", "_")
	out = strings.ReplaceAll(out, " ", "_")
	out = slugFilterPattern.ReplaceAllString(out, "")
	out = slugCollapsePattern.ReplaceAllString(out, "_")
	out = strings.Trim(out, "_")
	return out
}

// splitScriptStatements breaks a SurrealQL script into individual terminated
// statements, mirroring how discovery.splitStatements re-parses an on-disk
// file. It keeps generator output round-trippable through LoadMigration.
func splitScriptStatements(script string) []string {
	if strings.TrimSpace(script) == "" {
		return nil
	}
	// Normalize line endings and drop pure-comment / blank lines.
	script = strings.ReplaceAll(script, "\r\n", "\n")
	lines := strings.Split(script, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, raw := range lines {
		trim := strings.TrimSpace(raw)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "--") {
			continue
		}
		cleaned = append(cleaned, trim)
	}
	if len(cleaned) == 0 {
		return nil
	}
	joined := strings.Join(cleaned, " ")
	parts := strings.Split(joined, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt == "" {
			continue
		}
		out = append(out, stmt+";")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// renderMigrationContent produces the body of a `.surql` migration file.
// Header lines are emitted in a stable order (description, depends_on)
// followed by the `-- @up` and `-- @down` sections.
func renderMigrationContent(
	description string,
	dependsOn []string,
	up []string,
	down []string,
) string {
	var b strings.Builder
	if description != "" {
		b.WriteString("-- @description: ")
		b.WriteString(description)
		b.WriteByte('\n')
	}
	if len(dependsOn) > 0 {
		b.WriteString("-- @depends_on: ")
		b.WriteString(strings.Join(dependsOn, ", "))
		b.WriteByte('\n')
	}
	b.WriteString("-- @up\n")
	for _, stmt := range up {
		b.WriteString(stmt)
		if !strings.HasSuffix(stmt, "\n") {
			b.WriteByte('\n')
		}
	}
	b.WriteString("-- @down\n")
	for _, stmt := range down {
		b.WriteString(stmt)
		if !strings.HasSuffix(stmt, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// writeMigrationFile renders the migration content and writes it atomically
// to <dir>/<version>_<slug>.surql. On success it returns a Migration populated
// with parsed metadata (via LoadMigration) so callers get the final on-disk
// representation, including the computed checksum.
func writeMigrationFile(
	version string,
	slugSource string,
	description string,
	dependsOn []string,
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Migration{}, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationGeneration, err,
			"cannot create migration directory %q", dir,
		)
	}

	filename := BuildFilename(version, slugSource)
	if !ValidateMigrationName(filename) {
		return Migration{}, surqlerrors.Newf(
			surqlerrors.ErrMigrationGeneration,
			"generated filename %q is not a valid migration name", filename,
		)
	}
	finalPath := filepath.Join(dir, filename)

	content := renderMigrationContent(description, dependsOn, up, down)

	if err := atomicWriteFile(finalPath, []byte(content)); err != nil {
		return Migration{}, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationGeneration, err,
			"failed to write migration %q", finalPath,
		)
	}

	// Load the written file back so the returned Migration is exactly what
	// discovery would see. This also guarantees the round-trip contract
	// documented on each generator function.
	m, err := LoadMigration(finalPath)
	if err != nil {
		// Best-effort cleanup; the on-disk file is malformed which is a
		// programmer error in the generator, not a caller error.
		_ = os.Remove(finalPath)
		return Migration{}, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationGeneration, err,
			"generator produced invalid migration file %q", finalPath,
		)
	}
	return m, nil
}

// atomicWriteFile writes data to path via a temporary file in the same
// directory, fsyncs it, and renames it into place. This avoids readers ever
// seeing a partially-written migration file.
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// Clean up the temp file on any failure after CreateTemp.
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}

	// Best-effort: fsync the containing directory so the rename is durable.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
