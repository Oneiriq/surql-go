package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
)

func emptyLookup(string) (string, bool) { return "", false }

func mapLookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestDefaults(t *testing.T) {
	t.Parallel()
	d := Defaults()
	if d.Environment != EnvDevelopment {
		t.Fatalf("unexpected env: %s", d.Environment)
	}
	if !d.Debug {
		t.Fatalf("expected debug true")
	}
	if d.LogLevel != LogInfo {
		t.Fatalf("unexpected log level: %s", d.LogLevel)
	}
	if d.AppName != "surql" {
		t.Fatalf("unexpected app name: %s", d.AppName)
	}
	if d.MigrationPath != "migrations" {
		t.Fatalf("unexpected migration path: %s", d.MigrationPath)
	}
	if err := d.Database.Validate(); err != nil {
		t.Fatalf("default database invalid: %v", err)
	}
}

func TestLoadSettingsNoSources(t *testing.T) {
	t.Parallel()
	s, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithoutDotenv(),
		WithoutConfigFile(),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Environment != EnvDevelopment {
		t.Fatalf("unexpected env: %s", s.Environment)
	}
}

func TestLoadSettingsEnvOverrides(t *testing.T) {
	t.Parallel()
	s, err := LoadSettings(
		WithEnvLookup(mapLookup(map[string]string{
			"SURQL_ENVIRONMENT":    "production",
			"SURQL_DEBUG":          "false",
			"SURQL_LOG_LEVEL":      "WARNING",
			"SURQL_APP_NAME":       "driftnet",
			"SURQL_VERSION":        "1.2.3",
			"SURQL_MIGRATION_PATH": "db/migrations",
			"SURQL_URL":            "wss://example:443/rpc",
			"SURQL_NAMESPACE":      "prod",
			"SURQL_DATABASE":       "core",
		})),
		WithoutDotenv(),
		WithoutConfigFile(),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Environment != EnvProduction {
		t.Fatalf("env not applied: %s", s.Environment)
	}
	if s.Debug {
		t.Fatalf("debug not flipped")
	}
	if s.LogLevel != LogWarning {
		t.Fatalf("log level not applied: %s", s.LogLevel)
	}
	if s.AppName != "driftnet" {
		t.Fatalf("app name not applied: %s", s.AppName)
	}
	if s.Version != "1.2.3" {
		t.Fatalf("version not applied: %s", s.Version)
	}
	if s.MigrationPath != "db/migrations" {
		t.Fatalf("migration path not applied: %s", s.MigrationPath)
	}
	if s.Database.URL() != "wss://example:443/rpc" {
		t.Fatalf("db url not applied: %s", s.Database.URL())
	}
	if s.Database.Namespace() != "prod" {
		t.Fatalf("db ns not applied: %s", s.Database.Namespace())
	}
}

func TestLoadSettingsInitOptionsWin(t *testing.T) {
	t.Parallel()
	s, err := LoadSettings(
		WithEnvLookup(mapLookup(map[string]string{
			"SURQL_ENVIRONMENT": "production",
			"SURQL_APP_NAME":    "from-env",
		})),
		WithoutDotenv(),
		WithoutConfigFile(),
		WithEnvironment(EnvStaging),
		WithAppName("explicit"),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Environment != EnvStaging {
		t.Fatalf("init options should override env: %s", s.Environment)
	}
	if s.AppName != "explicit" {
		t.Fatalf("init options should override env app name: %s", s.AppName)
	}
}

func TestLoadSettingsInvalidDebug(t *testing.T) {
	t.Parallel()
	_, err := LoadSettings(
		WithEnvLookup(mapLookup(map[string]string{"SURQL_DEBUG": "nope"})),
		WithoutDotenv(),
		WithoutConfigFile(),
	)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadSettingsInvalidEnvironment(t *testing.T) {
	t.Parallel()
	_, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithoutDotenv(),
		WithoutConfigFile(),
		WithEnvironment("pizza"),
	)
	if err == nil {
		t.Fatalf("expected invalid-env error")
	}
}

func TestLoadSettingsDotenv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("SURQL_APP_NAME=from-dotenv\nSURQL_DEBUG=false\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithCwd(dir),
		WithoutConfigFile(),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.AppName != "from-dotenv" {
		t.Fatalf("dotenv not applied: %s", s.AppName)
	}
	if s.Debug {
		t.Fatalf("dotenv debug=false not applied")
	}
}

func TestLoadSettingsEnvBeatsDotenv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("SURQL_APP_NAME=from-dotenv\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadSettings(
		WithEnvLookup(mapLookup(map[string]string{"SURQL_APP_NAME": "from-env"})),
		WithCwd(dir),
		WithoutConfigFile(),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.AppName != "from-env" {
		t.Fatalf("env should override dotenv: %s", s.AppName)
	}
}

func TestLoadSettingsTOML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "surql.toml")
	body := `
environment = "staging"
app_name = "from-toml"
migration_path = "db/migrations"

[database]
url = "ws://example:8000/rpc"
namespace = "t"
database = "core"
timeout = 45.0
enable_live_queries = true
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithoutDotenv(),
		WithCwd(dir),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Environment != EnvStaging {
		t.Fatalf("toml env not applied: %s", s.Environment)
	}
	if s.AppName != "from-toml" {
		t.Fatalf("toml app name not applied: %s", s.AppName)
	}
	if s.MigrationPath != "db/migrations" {
		t.Fatalf("toml migration path not applied: %s", s.MigrationPath)
	}
	if s.Database.URL() != "ws://example:8000/rpc" {
		t.Fatalf("toml db.url not applied: %s", s.Database.URL())
	}
	if s.Database.Timeout() != 45.0 {
		t.Fatalf("toml db.timeout not applied: %v", s.Database.Timeout())
	}
}

func TestLoadSettingsYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "surql.yaml")
	body := `
environment: production
app_name: from-yaml
migration_path: ops/migrations
database:
  url: wss://example:443/rpc
  namespace: prod
  database: core
  timeout: 60
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithoutDotenv(),
		WithCwd(dir),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Environment != EnvProduction {
		t.Fatalf("yaml env not applied: %s", s.Environment)
	}
	if s.Database.URL() != "wss://example:443/rpc" {
		t.Fatalf("yaml db.url not applied: %s", s.Database.URL())
	}
	if s.Database.Timeout() != 60.0 {
		t.Fatalf("yaml db.timeout not applied: %v", s.Database.Timeout())
	}
}

func TestLoadSettingsEnvBeatsFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "surql.toml")
	body := `app_name = "from-toml"` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadSettings(
		WithEnvLookup(mapLookup(map[string]string{"SURQL_APP_NAME": "from-env"})),
		WithoutDotenv(),
		WithCwd(dir),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.AppName != "from-env" {
		t.Fatalf("env should override file: %s", s.AppName)
	}
}

func TestLoadSettingsConfigFileUnknownExt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "surql.txt")
	if err := os.WriteFile(path, []byte("nothing"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithoutDotenv(),
		WithConfigFile(path),
	)
	if err == nil {
		t.Fatalf("expected unknown-extension error")
	}
}

func TestLoadSettingsInvalidTOML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "surql.toml")
	if err := os.WriteFile(path, []byte("not = valid = toml"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithoutDotenv(),
		WithCwd(dir),
	)
	if err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestLoadSettingsUnsupportedOption(t *testing.T) {
	t.Parallel()
	_, err := LoadSettings("not an option", WithoutDotenv(), WithoutConfigFile())
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestGlobalRoundTrip(t *testing.T) {
	// not Parallel: mutates globals
	ResetGlobal()
	defer ResetGlobal()
	custom := Defaults()
	custom.AppName = "custom-global"
	SetGlobal(&custom)
	s, err := GetSettings()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if s.AppName != "custom-global" {
		t.Fatalf("SetGlobal did not take effect: %s", s.AppName)
	}
	db, err := GetDBConfig()
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if db.URL() != custom.Database.URL() {
		t.Fatalf("db url mismatch")
	}
	path, err := GetMigrationPath()
	if err != nil {
		t.Fatalf("migration path: %v", err)
	}
	if path != custom.MigrationPath {
		t.Fatalf("migration path mismatch: %s", path)
	}
}

func TestSettingsString(t *testing.T) {
	t.Parallel()
	s := Defaults()
	if got := s.String(); got == "" {
		t.Fatalf("String() must not be empty")
	}
}

func TestEnvironmentAndLogLevelValid(t *testing.T) {
	t.Parallel()
	if !EnvDevelopment.Valid() || !EnvStaging.Valid() || !EnvProduction.Valid() {
		t.Fatalf("known envs should be valid")
	}
	if Environment("other").Valid() {
		t.Fatalf("unknown env should be invalid")
	}
	if !LogInfo.Valid() || !LogDebug.Valid() || !LogWarning.Valid() || !LogError.Valid() || !LogCritical.Valid() {
		t.Fatalf("known log levels should be valid")
	}
	if LogLevel("TRACE").Valid() {
		t.Fatalf("unknown log level should be invalid")
	}
}

func TestLoadSettingsDoesNotPanicWithoutCwd(t *testing.T) {
	// Only tests that passing no cwd and not skipping sources works.
	_, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithoutDotenv(),
		WithoutConfigFile(),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestLoadSettingsDatabaseOptionOverride(t *testing.T) {
	t.Parallel()
	cfg := connection.DefaultConfig()
	cfg.DBNS = "custom"
	s, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithoutDotenv(),
		WithoutConfigFile(),
		WithDatabase(cfg),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Database.Namespace() != "custom" {
		t.Fatalf("WithDatabase not applied: %s", s.Database.Namespace())
	}
}

func TestLoadSettingsSpecificDotenvPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.env")
	if err := os.WriteFile(path, []byte("SURQL_APP_NAME=custom-dotenv\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := LoadSettings(
		WithEnvLookup(emptyLookup),
		WithDotenvPath(path),
		WithoutConfigFile(),
	)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.AppName != "custom-dotenv" {
		t.Fatalf("custom dotenv not applied: %s", s.AppName)
	}
}
