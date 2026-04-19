package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	toml "github.com/pelletier/go-toml/v2"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// Environment is one of the supported deployment environments,
// matching the Python Literal in surql-py.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// Valid reports whether e is one of the recognised environments.
func (e Environment) Valid() bool {
	switch e {
	case EnvDevelopment, EnvStaging, EnvProduction:
		return true
	default:
		return false
	}
}

// LogLevel mirrors surql-py's Literal for log_level.
type LogLevel string

const (
	LogDebug    LogLevel = "DEBUG"
	LogInfo     LogLevel = "INFO"
	LogWarning  LogLevel = "WARNING"
	LogError    LogLevel = "ERROR"
	LogCritical LogLevel = "CRITICAL"
)

// Valid reports whether l is a recognised log level.
func (l LogLevel) Valid() bool {
	switch l {
	case LogDebug, LogInfo, LogWarning, LogError, LogCritical:
		return true
	default:
		return false
	}
}

// Settings is the whole surql runtime configuration.
type Settings struct {
	Environment   Environment
	Debug         bool
	LogLevel      LogLevel
	AppName       string
	Version       string
	MigrationPath string
	Database      connection.ConnectionConfig
}

// Defaults returns the library default Settings (equivalent to
// surql-py's Settings() with no overrides).
func Defaults() Settings {
	return Settings{
		Environment:   EnvDevelopment,
		Debug:         true,
		LogLevel:      LogInfo,
		AppName:       "surql",
		Version:       "0.1.0-dev",
		MigrationPath: "migrations",
		Database:      connection.DefaultConfig(),
	}
}

// LoadOption is the functional-option form for explicit overrides
// passed to LoadSettings. It is applied last (highest priority).
type LoadOption func(*Settings)

// WithEnvironment overrides Settings.Environment.
func WithEnvironment(env Environment) LoadOption {
	return func(s *Settings) { s.Environment = env }
}

// WithDebug overrides Settings.Debug.
func WithDebug(debug bool) LoadOption {
	return func(s *Settings) { s.Debug = debug }
}

// WithLogLevel overrides Settings.LogLevel.
func WithLogLevel(level LogLevel) LoadOption {
	return func(s *Settings) { s.LogLevel = level }
}

// WithAppName overrides Settings.AppName.
func WithAppName(name string) LoadOption {
	return func(s *Settings) { s.AppName = name }
}

// WithVersion overrides Settings.Version.
func WithVersion(version string) LoadOption {
	return func(s *Settings) { s.Version = version }
}

// WithMigrationPath overrides Settings.MigrationPath.
func WithMigrationPath(path string) LoadOption {
	return func(s *Settings) { s.MigrationPath = path }
}

// WithDatabase overrides Settings.Database.
func WithDatabase(cfg connection.ConnectionConfig) LoadOption {
	return func(s *Settings) { s.Database = cfg }
}

// loadConfig tracks resolver inputs for LoadSettings.
type loadConfig struct {
	envLookup  func(string) (string, bool)
	dotenvPath string
	configPath string
	skipDotenv bool
	skipFile   bool
	cwd        string
}

// LoadSettingsOption configures the resolver pipeline (separate from
// LoadOption which mutates the Settings struct directly).
type LoadSettingsOption func(*loadConfig)

// WithEnvLookup replaces os.LookupEnv with an arbitrary lookup. Tests
// use this to avoid mutating process env.
func WithEnvLookup(lookup func(string) (string, bool)) LoadSettingsOption {
	return func(c *loadConfig) { c.envLookup = lookup }
}

// WithDotenvPath loads this specific .env file instead of searching
// for a default one in cwd.
func WithDotenvPath(path string) LoadSettingsOption {
	return func(c *loadConfig) { c.dotenvPath = path }
}

// WithConfigFile loads this specific surql.yaml/surql.toml instead of
// searching cwd. The extension (.yaml/.yml/.toml) drives the format.
func WithConfigFile(path string) LoadSettingsOption {
	return func(c *loadConfig) { c.configPath = path }
}

// WithoutDotenv disables the dotenv layer entirely.
func WithoutDotenv() LoadSettingsOption {
	return func(c *loadConfig) { c.skipDotenv = true }
}

// WithoutConfigFile disables the surql.yaml/toml layer entirely.
func WithoutConfigFile() LoadSettingsOption {
	return func(c *loadConfig) { c.skipFile = true }
}

// WithCwd overrides the directory used when searching for default
// .env / surql.yaml / surql.toml files.
func WithCwd(dir string) LoadSettingsOption {
	return func(c *loadConfig) { c.cwd = dir }
}

// LoadSettings resolves Settings from the four layered sources
// (lowest -> highest priority):
//
//  1. Defaults()
//  2. surql.yaml / surql.toml (project root)
//  3. .env via godotenv (project root)
//  4. SURQL_-prefixed environment variables
//  5. explicit LoadOption values
//
// The layering deliberately matches surql-py's priority semantics
// (init_settings > env > dotenv > pyproject-toml).
func LoadSettings(opts ...any) (*Settings, error) {
	cfg := &loadConfig{envLookup: os.LookupEnv}
	var inits []LoadOption
	for _, o := range opts {
		switch v := o.(type) {
		case LoadSettingsOption:
			v(cfg)
		case LoadOption:
			inits = append(inits, v)
		case nil:
			// skip
		default:
			return nil, surqlerrors.Newf(surqlerrors.ErrValidation,
				"LoadSettings: unsupported option type %T", o)
		}
	}
	s := Defaults()

	cwd := cfg.cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "resolve cwd")
		}
	}

	// Layer 2: optional config file (surql.yaml or surql.toml)
	if !cfg.skipFile {
		path := cfg.configPath
		if path == "" {
			path = findConfigFile(cwd)
		}
		if path != "" {
			if err := applyConfigFile(&s, path); err != nil {
				return nil, err
			}
		}
	}

	// Layer 3: .env (overrides config file via env_settings layer below)
	if !cfg.skipDotenv {
		path := cfg.dotenvPath
		if path == "" {
			candidate := filepath.Join(cwd, ".env")
			if fileExists(candidate) {
				path = candidate
			}
		}
		if path != "" {
			if err := applyDotenv(&s, path); err != nil {
				return nil, err
			}
		}
	}

	// Layer 4: SURQL_ env vars (process env by default).
	if cfg.envLookup != nil {
		if err := applyEnv(&s, cfg.envLookup); err != nil {
			return nil, err
		}
	}

	// Layer 5: explicit constructor overrides (highest priority).
	for _, o := range inits {
		o(&s)
	}

	if err := validateSettings(s); err != nil {
		return nil, err
	}
	return &s, nil
}

// validateSettings applies the non-struct-level invariants.
func validateSettings(s Settings) error {
	if !s.Environment.Valid() {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"settings: invalid environment %q", s.Environment)
	}
	if !s.LogLevel.Valid() {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"settings: invalid log_level %q", s.LogLevel)
	}
	if s.AppName == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "settings: app_name cannot be empty")
	}
	if s.MigrationPath == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "settings: migration_path cannot be empty")
	}
	return s.Database.Validate()
}

// applyEnv overlays SURQL_-prefixed env vars onto s.
func applyEnv(s *Settings, lookup func(string) (string, bool)) error {
	if v, ok := lookup("SURQL_ENVIRONMENT"); ok {
		s.Environment = Environment(strings.ToLower(v))
	}
	if v, ok := lookup("SURQL_DEBUG"); ok {
		b, err := parseBool(v)
		if err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "SURQL_DEBUG=%q", v)
		}
		s.Debug = b
	}
	if v, ok := lookup("SURQL_LOG_LEVEL"); ok {
		s.LogLevel = LogLevel(strings.ToUpper(v))
	}
	if v, ok := lookup("SURQL_APP_NAME"); ok {
		s.AppName = v
	}
	if v, ok := lookup("SURQL_VERSION"); ok {
		s.Version = v
	}
	if v, ok := lookup("SURQL_MIGRATION_PATH"); ok {
		s.MigrationPath = v
	}
	// Overlay connection fields onto the current s.Database so the
	// env layer only mutates fields the user actually set (values
	// from the config file or defaults otherwise carry through).
	return overlayDatabaseEnv(&s.Database, lookup)
}

// overlayDatabaseEnv mutates cfg in place from SURQL_-prefixed env
// vars. Only keys present in lookup are touched.
func overlayDatabaseEnv(cfg *connection.ConnectionConfig, lookup func(string) (string, bool)) error {
	get := func(keys ...string) (string, bool) {
		for _, k := range keys {
			if v, ok := lookup(connection.EnvPrefix + k); ok {
				return v, true
			}
		}
		return "", false
	}
	if v, ok := get("URL", "DB_URL"); ok {
		cfg.DBURL = v
	}
	if v, ok := get("NAMESPACE", "DB_NS"); ok {
		cfg.DBNS = v
	}
	if v, ok := get("DATABASE", "DB"); ok {
		cfg.DB = v
	}
	if v, ok := get("USERNAME", "DB_USER"); ok {
		u := v
		cfg.DBUser = &u
	}
	if v, ok := get("PASSWORD", "DB_PASS"); ok {
		p := v
		cfg.DBPass = &p
	}
	if v, ok := get("TIMEOUT", "DB_TIMEOUT"); ok {
		f, err := parseFloat(v)
		if err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid timeout=%q", v)
		}
		cfg.DBTimeout = f
	}
	if v, ok := get("MAX_CONNECTIONS", "DB_MAX_CONNECTIONS"); ok {
		n, err := parseUint(v)
		if err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid max_connections=%q", v)
		}
		cfg.DBMaxConnections = n
	}
	if v, ok := get("RETRY_MAX_ATTEMPTS", "DB_RETRY_MAX_ATTEMPTS"); ok {
		n, err := parseUint(v)
		if err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid retry_max_attempts=%q", v)
		}
		cfg.DBRetryMaxAttempts = n
	}
	if v, ok := get("RETRY_MIN_WAIT", "DB_RETRY_MIN_WAIT"); ok {
		f, err := parseFloat(v)
		if err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid retry_min_wait=%q", v)
		}
		cfg.DBRetryMinWait = f
	}
	if v, ok := get("RETRY_MAX_WAIT", "DB_RETRY_MAX_WAIT"); ok {
		f, err := parseFloat(v)
		if err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid retry_max_wait=%q", v)
		}
		cfg.DBRetryMaxWait = f
	}
	if v, ok := get("RETRY_MULTIPLIER", "DB_RETRY_MULTIPLIER"); ok {
		f, err := parseFloat(v)
		if err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid retry_multiplier=%q", v)
		}
		cfg.DBRetryMultiplier = f
	}
	if v, ok := get("ENABLE_LIVE_QUERIES"); ok {
		b, err := parseBool(v)
		if err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid enable_live_queries=%q", v)
		}
		cfg.EnableLiveQueries = b
	}
	return nil
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func parseUint(s string) (uint32, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(n), nil
}

// applyDotenv reads path and overlays SURQL_ vars onto s without
// touching the process env.
func applyDotenv(s *Settings, path string) error {
	raw, err := godotenv.Read(path)
	if err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "read dotenv %q", path)
	}
	lookup := func(k string) (string, bool) {
		v, ok := raw[k]
		return v, ok
	}
	return applyEnv(s, lookup)
}

// configFile is the on-disk representation of surql.yaml / surql.toml.
// Unknown keys are ignored. All keys are optional; the default
// Settings is mutated in place.
type configFile struct {
	Environment   *string           `toml:"environment" yaml:"environment"`
	Debug         *bool             `toml:"debug" yaml:"debug"`
	LogLevel      *string           `toml:"log_level" yaml:"log_level"`
	AppName       *string           `toml:"app_name" yaml:"app_name"`
	Version       *string           `toml:"version" yaml:"version"`
	MigrationPath *string           `toml:"migration_path" yaml:"migration_path"`
	Database      *configFileDBFile `toml:"database" yaml:"database"`
}

// configFileDBFile is a minimal subset of ConnectionConfig exposed
// via surql.yaml / surql.toml.
type configFileDBFile struct {
	URL               *string  `toml:"url" yaml:"url"`
	Namespace         *string  `toml:"namespace" yaml:"namespace"`
	Database          *string  `toml:"database" yaml:"database"`
	Username          *string  `toml:"username" yaml:"username"`
	Password          *string  `toml:"password" yaml:"password"`
	Timeout           *float64 `toml:"timeout" yaml:"timeout"`
	MaxConnections    *uint32  `toml:"max_connections" yaml:"max_connections"`
	RetryMaxAttempts  *uint32  `toml:"retry_max_attempts" yaml:"retry_max_attempts"`
	RetryMinWait      *float64 `toml:"retry_min_wait" yaml:"retry_min_wait"`
	RetryMaxWait      *float64 `toml:"retry_max_wait" yaml:"retry_max_wait"`
	RetryMultiplier   *float64 `toml:"retry_multiplier" yaml:"retry_multiplier"`
	EnableLiveQueries *bool    `toml:"enable_live_queries" yaml:"enable_live_queries"`
}

// findConfigFile looks for surql.yaml, surql.yml, then surql.toml.
func findConfigFile(dir string) string {
	for _, name := range []string{"surql.yaml", "surql.yml", "surql.toml"} {
		candidate := filepath.Join(dir, name)
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func applyConfigFile(s *Settings, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "read config %q", path)
	}
	var cf configFile
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		if err := toml.Unmarshal(data, &cf); err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "parse toml %q", path)
		}
	case ".yaml", ".yml":
		if err := unmarshalYAML(data, &cf); err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "parse yaml %q", path)
		}
	default:
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"unknown settings file format %q (want .yaml|.yml|.toml)", filepath.Ext(path))
	}
	cf.apply(s)
	return nil
}

func (cf configFile) apply(s *Settings) {
	if cf.Environment != nil {
		s.Environment = Environment(strings.ToLower(*cf.Environment))
	}
	if cf.Debug != nil {
		s.Debug = *cf.Debug
	}
	if cf.LogLevel != nil {
		s.LogLevel = LogLevel(strings.ToUpper(*cf.LogLevel))
	}
	if cf.AppName != nil {
		s.AppName = *cf.AppName
	}
	if cf.Version != nil {
		s.Version = *cf.Version
	}
	if cf.MigrationPath != nil {
		s.MigrationPath = *cf.MigrationPath
	}
	if cf.Database != nil {
		cf.Database.apply(&s.Database)
	}
}

func (d configFileDBFile) apply(cfg *connection.ConnectionConfig) {
	if d.URL != nil {
		cfg.DBURL = *d.URL
	}
	if d.Namespace != nil {
		cfg.DBNS = *d.Namespace
	}
	if d.Database != nil {
		cfg.DB = *d.Database
	}
	if d.Username != nil {
		u := *d.Username
		cfg.DBUser = &u
	}
	if d.Password != nil {
		p := *d.Password
		cfg.DBPass = &p
	}
	if d.Timeout != nil {
		cfg.DBTimeout = *d.Timeout
	}
	if d.MaxConnections != nil {
		cfg.DBMaxConnections = *d.MaxConnections
	}
	if d.RetryMaxAttempts != nil {
		cfg.DBRetryMaxAttempts = *d.RetryMaxAttempts
	}
	if d.RetryMinWait != nil {
		cfg.DBRetryMinWait = *d.RetryMinWait
	}
	if d.RetryMaxWait != nil {
		cfg.DBRetryMaxWait = *d.RetryMaxWait
	}
	if d.RetryMultiplier != nil {
		cfg.DBRetryMultiplier = *d.RetryMultiplier
	}
	if d.EnableLiveQueries != nil {
		cfg.EnableLiveQueries = *d.EnableLiveQueries
	}
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, errors.New("invalid boolean value")
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// -----------------------------------------------------------------------------
// Global accessors (parity with surql-py get_settings / get_db_config / ...).
// -----------------------------------------------------------------------------

var (
	globalMu   sync.RWMutex
	globalSet  *Settings
	globalErr  error
	globalOnce sync.Once
)

// GetSettings returns the process-global Settings, loading it the
// first time it is called (equivalent to surql-py's lru_cache-backed
// get_settings).
//
// On load failure every subsequent call returns the same error.
func GetSettings() (*Settings, error) {
	globalOnce.Do(func() {
		s, err := LoadSettings()
		globalMu.Lock()
		globalSet, globalErr = s, err
		globalMu.Unlock()
	})
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalSet, globalErr
}

// SetGlobal installs an explicit Settings as the process-global.
// Useful for tests that need deterministic control.
func SetGlobal(s *Settings) {
	globalMu.Lock()
	globalSet = s
	globalErr = nil
	globalMu.Unlock()
	// Ensure Do doesn't replace us on first call.
	globalOnce.Do(func() {})
}

// ResetGlobal clears the cached process-global settings. The next
// GetSettings call will lazily reload.
func ResetGlobal() {
	globalMu.Lock()
	globalSet = nil
	globalErr = nil
	globalOnce = sync.Once{}
	globalMu.Unlock()
}

// GetDBConfig returns the connection.ConnectionConfig embedded in the
// global Settings (surql-py's get_db_config).
func GetDBConfig() (connection.ConnectionConfig, error) {
	s, err := GetSettings()
	if err != nil {
		return connection.ConnectionConfig{}, err
	}
	return s.Database, nil
}

// GetMigrationPath returns the MigrationPath from the global Settings
// (surql-py's get_migration_path).
func GetMigrationPath() (string, error) {
	s, err := GetSettings()
	if err != nil {
		return "", err
	}
	return s.MigrationPath, nil
}

// String renders Settings in a human-readable one-liner useful for
// startup logging.
func (s Settings) String() string {
	return fmt.Sprintf("Settings{env=%s debug=%t log=%s app=%s version=%s migrations=%s}",
		s.Environment, s.Debug, s.LogLevel, s.AppName, s.Version, s.MigrationPath)
}
