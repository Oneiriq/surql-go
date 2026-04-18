package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// Section markers used inside migration files.
const (
	markerUp          = "-- @up"
	markerDown        = "-- @down"
	markerDescription = "-- @description:"
	markerDependsOn   = "-- @depends_on:"
)

// filenameRe matches the YYYYMMDD_HHMMSS_description.surql convention.
// The description may contain additional underscores and alphanumerics.
var filenameRe = regexp.MustCompile(`^(\d{8})_(\d{6})_([A-Za-z0-9][A-Za-z0-9_-]*)\.surql$`)

// DiscoverMigrations scans directory for `.surql` migration files and returns
// them sorted by version (timestamp).
//
// Files whose names do not conform to the YYYYMMDD_HHMMSS_*.surql convention
// are skipped silently (mirroring the Python port's glob-plus-validate
// behaviour). A non-existent directory is not an error and yields an empty
// slice. A path that exists but is not a directory returns a wrapped
// surqlerrors.ErrMigrationDiscovery.
func DiscoverMigrations(directory string) ([]Migration, error) {
	info, err := os.Stat(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return []Migration{}, nil
		}
		return nil, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationDiscovery, err,
			"cannot stat migration directory %q", directory,
		)
	}
	if !info.IsDir() {
		return nil, surqlerrors.Newf(
			surqlerrors.ErrMigrationDiscovery,
			"path is not a directory: %q", directory,
		)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationDiscovery, err,
			"cannot read migration directory %q", directory,
		)
	}

	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !ValidateMigrationName(name) {
			continue
		}
		full := filepath.Join(directory, name)
		m, err := LoadMigration(full)
		if err != nil {
			return nil, surqlerrors.Wrapf(
				surqlerrors.ErrMigrationDiscovery, err,
				"failed to load migration %q", full,
			)
		}
		migrations = append(migrations, m)
	}

	sort.SliceStable(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

// LoadMigration loads a single `.surql` migration file.
//
// The file must contain `-- @up` and `-- @down` section markers. Optional
// `-- @description:` and `-- @depends_on:` header lines override the values
// derived from the file name.
func LoadMigration(path string) (Migration, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Migration{}, surqlerrors.Newf(
				surqlerrors.ErrMigrationLoad,
				"migration file not found: %q", path,
			)
		}
		return Migration{}, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationLoad, err,
			"cannot stat migration file %q", path,
		)
	}
	if info.IsDir() {
		return Migration{}, surqlerrors.Newf(
			surqlerrors.ErrMigrationLoad,
			"path is not a file: %q", path,
		)
	}

	filename := filepath.Base(path)
	if !ValidateMigrationName(filename) {
		return Migration{}, surqlerrors.Newf(
			surqlerrors.ErrMigrationLoad,
			"invalid migration filename: %q", filename,
		)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return Migration{}, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationLoad, err,
			"cannot read migration file %q", path,
		)
	}

	parsed, err := parseMigrationContent(string(content))
	if err != nil {
		return Migration{}, surqlerrors.Wrapf(
			surqlerrors.ErrMigrationLoad, err,
			"failed to parse migration %q", path,
		)
	}

	version, _ := GetVersionFromFilename(filename)
	description := parsed.description
	if description == "" {
		desc, _ := GetDescriptionFromFilename(filename)
		description = humanizeDescription(desc)
	}

	return Migration{
		Version:        version,
		Description:    description,
		Path:           path,
		UpStatements:   parsed.up,
		DownStatements: parsed.down,
		Checksum:       sha256Hex(content),
		DependsOn:      parsed.dependsOn,
	}, nil
}

// ValidateMigrationName reports whether filename matches the
// YYYYMMDD_HHMMSS_description.surql convention.
func ValidateMigrationName(filename string) bool {
	return filenameRe.MatchString(filename)
}

// GetVersionFromFilename returns the YYYYMMDD_HHMMSS version component of a
// migration file name. The boolean is false when the filename does not match
// the expected convention.
func GetVersionFromFilename(filename string) (string, bool) {
	m := filenameRe.FindStringSubmatch(filename)
	if m == nil {
		return "", false
	}
	return m[1] + "_" + m[2], true
}

// GetDescriptionFromFilename returns the description component of a
// migration file name. The boolean is false when the filename does not match
// the expected convention.
func GetDescriptionFromFilename(filename string) (string, bool) {
	m := filenameRe.FindStringSubmatch(filename)
	if m == nil {
		return "", false
	}
	return m[3], true
}

// parsed holds the three logical sections extracted from a migration file.
type parsed struct {
	description string
	dependsOn   []string
	up          []string
	down        []string
}

// parseMigrationContent splits a migration file body into header, up, and
// down sections and returns cleaned statement slices.
func parseMigrationContent(body string) (parsed, error) {
	var p parsed

	// Normalize line endings and split.
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")

	// Locate section markers. Markers are expected on their own line (after
	// any leading whitespace).
	upIdx, downIdx := -1, -1
	for i, raw := range lines {
		switch strings.TrimSpace(raw) {
		case markerUp:
			if upIdx != -1 {
				return parsed{}, surqlerrors.New(
					surqlerrors.ErrMigrationLoad,
					"duplicate '-- @up' marker",
				)
			}
			upIdx = i
		case markerDown:
			if downIdx != -1 {
				return parsed{}, surqlerrors.New(
					surqlerrors.ErrMigrationLoad,
					"duplicate '-- @down' marker",
				)
			}
			downIdx = i
		}
	}

	if upIdx == -1 {
		return parsed{}, surqlerrors.New(
			surqlerrors.ErrMigrationLoad,
			"missing '-- @up' section marker",
		)
	}
	if downIdx == -1 {
		return parsed{}, surqlerrors.New(
			surqlerrors.ErrMigrationLoad,
			"missing '-- @down' section marker",
		)
	}
	if downIdx < upIdx {
		return parsed{}, surqlerrors.New(
			surqlerrors.ErrMigrationLoad,
			"'-- @down' marker must follow '-- @up'",
		)
	}

	// Header lines: everything above upIdx.
	for _, raw := range lines[:upIdx] {
		trim := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(trim, markerDescription):
			p.description = strings.TrimSpace(strings.TrimPrefix(trim, markerDescription))
		case strings.HasPrefix(trim, markerDependsOn):
			p.dependsOn = splitCSV(strings.TrimPrefix(trim, markerDependsOn))
		}
	}

	// Up / down sections.
	p.up = splitStatements(lines[upIdx+1 : downIdx])
	p.down = splitStatements(lines[downIdx+1:])

	return p, nil
}

// splitStatements joins the given lines, removes pure-comment and blank
// lines, and splits the remainder on `;` boundaries, trimming whitespace.
func splitStatements(lines []string) []string {
	cleaned := make([]string, 0, len(lines))
	for _, raw := range lines {
		trim := strings.TrimSpace(raw)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "--") {
			continue
		}
		cleaned = append(cleaned, raw)
	}
	if len(cleaned) == 0 {
		return nil
	}
	joined := strings.Join(cleaned, "\n")
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

// splitCSV splits a comma-separated header value, trimming whitespace and
// dropping empty entries. Returns nil when the input has no values.
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// humanizeDescription converts an underscore/dash-separated slug into a
// space-separated label: `create_user_table` -> `create user table`.
func humanizeDescription(slug string) string {
	if slug == "" {
		return ""
	}
	slug = strings.ReplaceAll(slug, "-", " ")
	slug = strings.ReplaceAll(slug, "_", " ")
	// Collapse duplicated whitespace.
	fields := strings.Fields(slug)
	return strings.Join(fields, " ")
}

// sha256Hex returns the hex-encoded SHA-256 digest of the given bytes.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
