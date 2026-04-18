package connection

import (
	"math"
	"os"
	"strconv"
	"strings"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// EnvPrefix is the environment variable prefix used by LoadConfigFromEnv.
const EnvPrefix = "SURQL_"

// Protocol is the connection protocol implied by the configured URL.
type Protocol int

const (
	// ProtocolWebSocket == ws://
	ProtocolWebSocket Protocol = iota
	// ProtocolWebSocketSecure == wss://
	ProtocolWebSocketSecure
	// ProtocolHTTP == http://
	ProtocolHTTP
	// ProtocolHTTPS == https://
	ProtocolHTTPS
	// ProtocolMemory == mem:// or memory://
	ProtocolMemory
	// ProtocolFile == file://
	ProtocolFile
	// ProtocolSurrealKV == surrealkv://
	ProtocolSurrealKV
)

// String returns the canonical scheme name.
func (p Protocol) String() string {
	switch p {
	case ProtocolWebSocket:
		return "ws"
	case ProtocolWebSocketSecure:
		return "wss"
	case ProtocolHTTP:
		return "http"
	case ProtocolHTTPS:
		return "https"
	case ProtocolMemory:
		return "memory"
	case ProtocolFile:
		return "file"
	case ProtocolSurrealKV:
		return "surrealkv"
	default:
		return "unknown"
	}
}

// SupportsLiveQueries reports whether the protocol supports LIVE SELECT.
func (p Protocol) SupportsLiveQueries() bool {
	return p != ProtocolHTTP && p != ProtocolHTTPS
}

// IsEmbedded reports whether the protocol runs in-process (no server).
func (p Protocol) IsEmbedded() bool {
	return p == ProtocolMemory || p == ProtocolFile || p == ProtocolSurrealKV
}

// ConnectionConfig is the SurrealDB connection configuration.
//
// Field names follow the Python port (DBURL, DBNS, DB, ...) for
// wire-compatibility with env loading; convenient accessors (URL,
// Namespace, Database, ...) are provided for the shorter aliases.
//
//nolint:revive // Allow exported field name repetition with package name
type ConnectionConfig struct {
	DBURL              string  `json:"db_url"`
	DBNS               string  `json:"db_ns"`
	DB                 string  `json:"db"`
	DBUser             *string `json:"db_user,omitempty"`
	DBPass             *string `json:"db_pass,omitempty"`
	DBTimeout          float64 `json:"db_timeout"`
	DBMaxConnections   uint32  `json:"db_max_connections"`
	DBRetryMaxAttempts uint32  `json:"db_retry_max_attempts"`
	DBRetryMinWait     float64 `json:"db_retry_min_wait"`
	DBRetryMaxWait     float64 `json:"db_retry_max_wait"`
	DBRetryMultiplier  float64 `json:"db_retry_multiplier"`
	EnableLiveQueries  bool    `json:"enable_live_queries"`
}

// DefaultConfig returns the library defaults (same as the Python port).
func DefaultConfig() ConnectionConfig {
	return ConnectionConfig{
		DBURL:              "ws://localhost:8000/rpc",
		DBNS:               "development",
		DB:                 "main",
		DBUser:             nil,
		DBPass:             nil,
		DBTimeout:          30.0,
		DBMaxConnections:   10,
		DBRetryMaxAttempts: 3,
		DBRetryMinWait:     1.0,
		DBRetryMaxWait:     10.0,
		DBRetryMultiplier:  2.0,
		EnableLiveQueries:  true,
	}
}

// URL returns DBURL (alias).
func (c ConnectionConfig) URL() string { return c.DBURL }

// Namespace returns DBNS (alias).
func (c ConnectionConfig) Namespace() string { return c.DBNS }

// Database returns DB (alias).
func (c ConnectionConfig) Database() string { return c.DB }

// Username returns DBUser (alias).
func (c ConnectionConfig) Username() *string { return c.DBUser }

// Password returns DBPass (alias).
func (c ConnectionConfig) Password() *string { return c.DBPass }

// Timeout returns DBTimeout (alias).
func (c ConnectionConfig) Timeout() float64 { return c.DBTimeout }

// MaxConnections returns DBMaxConnections (alias).
func (c ConnectionConfig) MaxConnections() uint32 { return c.DBMaxConnections }

// RetryMaxAttempts returns DBRetryMaxAttempts (alias).
func (c ConnectionConfig) RetryMaxAttempts() uint32 { return c.DBRetryMaxAttempts }

// RetryMinWait returns DBRetryMinWait (alias).
func (c ConnectionConfig) RetryMinWait() float64 { return c.DBRetryMinWait }

// RetryMaxWait returns DBRetryMaxWait (alias).
func (c ConnectionConfig) RetryMaxWait() float64 { return c.DBRetryMaxWait }

// RetryMultiplier returns DBRetryMultiplier (alias).
func (c ConnectionConfig) RetryMultiplier() float64 { return c.DBRetryMultiplier }

// Protocol infers the protocol from DBURL (validates as a side effect).
func (c ConnectionConfig) Protocol() (Protocol, error) {
	return detectProtocol(c.DBURL)
}

// Validate applies all the rules from the Python port.
func (c ConnectionConfig) Validate() error {
	if err := validateURL(c.DBURL); err != nil {
		return err
	}
	if err := validateIdentifier(c.DBNS, "namespace"); err != nil {
		return err
	}
	if err := validateIdentifier(c.DB, "database"); err != nil {
		return err
	}
	if err := validateNumericRange("timeout", c.DBTimeout, 1.0, math.Inf(1)); err != nil {
		return err
	}
	if err := validateNumericRange("max_connections", float64(c.DBMaxConnections), 1.0, 100.0); err != nil {
		return err
	}
	if err := validateNumericRange("retry_max_attempts", float64(c.DBRetryMaxAttempts), 1.0, 10.0); err != nil {
		return err
	}
	if err := validateNumericRange("retry_min_wait", c.DBRetryMinWait, 0.1, math.Inf(1)); err != nil {
		return err
	}
	if err := validateNumericRange("retry_max_wait", c.DBRetryMaxWait, 1.0, math.Inf(1)); err != nil {
		return err
	}
	if err := validateNumericRange("retry_multiplier", c.DBRetryMultiplier, 1.0, math.Inf(1)); err != nil {
		return err
	}
	if c.DBRetryMaxWait <= c.DBRetryMinWait {
		return surqlerrors.New(surqlerrors.ErrValidation,
			"db_retry_max_wait must be greater than db_retry_min_wait")
	}
	proto, err := detectProtocol(c.DBURL)
	if err != nil {
		return err
	}
	if c.EnableLiveQueries && !proto.SupportsLiveQueries() {
		return surqlerrors.New(surqlerrors.ErrValidation,
			"Live queries require WebSocket (ws://, wss://) or embedded (mem://, memory://, file://, surrealkv://) connection")
	}
	return nil
}

// LoadConfigFromEnv loads SurrealDB configuration from process env vars
// prefixed with EnvPrefix (SURQL_). Missing values fall back to defaults.
//
// Recognised names: SURQL_URL, SURQL_NAMESPACE, SURQL_DATABASE,
// SURQL_USERNAME, SURQL_PASSWORD, SURQL_TIMEOUT, SURQL_MAX_CONNECTIONS,
// SURQL_RETRY_MAX_ATTEMPTS, SURQL_RETRY_MIN_WAIT, SURQL_RETRY_MAX_WAIT,
// SURQL_RETRY_MULTIPLIER, SURQL_ENABLE_LIVE_QUERIES. Python-port legacy
// aliases (DB_URL, DB_NS, DB, DB_USER, DB_PASS, ...) are also accepted.
func LoadConfigFromEnv() (ConnectionConfig, error) {
	return LoadConfigFromEnvWithPrefix(EnvPrefix)
}

// LoadConfigFromEnvWithPrefix is LoadConfigFromEnv with a custom prefix
// (e.g. "SURQL_PRIMARY_" for named connections).
func LoadConfigFromEnvWithPrefix(prefix string) (ConnectionConfig, error) {
	return LoadConfigFromSource(prefix, func(key string) (string, bool) {
		return os.LookupEnv(key)
	})
}

// LoadConfigFromSource builds a config from an arbitrary key lookup.
// Tests pass a map-backed lookup to avoid mutating process env.
func LoadConfigFromSource(prefix string, lookup func(string) (string, bool)) (ConnectionConfig, error) {
	cfg := DefaultConfig()
	if v, ok := getWithAliases(lookup, prefix, "URL", "DB_URL"); ok {
		cfg.DBURL = v
	}
	if v, ok := getWithAliases(lookup, prefix, "NAMESPACE", "DB_NS"); ok {
		cfg.DBNS = v
	}
	if v, ok := getWithAliases(lookup, prefix, "DATABASE", "DB"); ok {
		cfg.DB = v
	}
	if v, ok := getWithAliases(lookup, prefix, "USERNAME", "DB_USER"); ok {
		u := v
		cfg.DBUser = &u
	}
	if v, ok := getWithAliases(lookup, prefix, "PASSWORD", "DB_PASS"); ok {
		p := v
		cfg.DBPass = &p
	}
	if v, ok := getWithAliases(lookup, prefix, "TIMEOUT", "DB_TIMEOUT"); ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return ConnectionConfig{}, surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid timeout=%q", v)
		}
		cfg.DBTimeout = f
	}
	if v, ok := getWithAliases(lookup, prefix, "MAX_CONNECTIONS", "DB_MAX_CONNECTIONS"); ok {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return ConnectionConfig{}, surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid max_connections=%q", v)
		}
		cfg.DBMaxConnections = uint32(n)
	}
	if v, ok := getWithAliases(lookup, prefix, "RETRY_MAX_ATTEMPTS", "DB_RETRY_MAX_ATTEMPTS"); ok {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return ConnectionConfig{}, surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid retry_max_attempts=%q", v)
		}
		cfg.DBRetryMaxAttempts = uint32(n)
	}
	if v, ok := getWithAliases(lookup, prefix, "RETRY_MIN_WAIT", "DB_RETRY_MIN_WAIT"); ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return ConnectionConfig{}, surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid retry_min_wait=%q", v)
		}
		cfg.DBRetryMinWait = f
	}
	if v, ok := getWithAliases(lookup, prefix, "RETRY_MAX_WAIT", "DB_RETRY_MAX_WAIT"); ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return ConnectionConfig{}, surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid retry_max_wait=%q", v)
		}
		cfg.DBRetryMaxWait = f
	}
	if v, ok := getWithAliases(lookup, prefix, "RETRY_MULTIPLIER", "DB_RETRY_MULTIPLIER"); ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return ConnectionConfig{}, surqlerrors.Wrapf(surqlerrors.ErrValidation, err, "invalid retry_multiplier=%q", v)
		}
		cfg.DBRetryMultiplier = f
	}
	if v, ok := getWithAliases(lookup, prefix, "ENABLE_LIVE_QUERIES"); ok {
		b, err := parseBool(v)
		if err != nil {
			return ConnectionConfig{}, err
		}
		cfg.EnableLiveQueries = b
	}
	if err := cfg.Validate(); err != nil {
		return ConnectionConfig{}, err
	}
	return cfg, nil
}

// LoadConfigFromMap is a test-friendly alternative to LoadConfigFromEnv.
func LoadConfigFromMap(prefix string, m map[string]string) (ConnectionConfig, error) {
	return LoadConfigFromSource(prefix, func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	})
}

// NamedConnectionConfig is a named ConnectionConfig for multi-DB setups.
//
//nolint:revive // Package/field name repetition is intentional for clarity
type NamedConnectionConfig struct {
	Name   string           `json:"name"`
	Config ConnectionConfig `json:"config"`
}

// LoadNamedConfigFromEnv loads a named connection using prefix SURQL_<NAME>_.
func LoadNamedConfigFromEnv(name string) (NamedConnectionConfig, error) {
	prefix := EnvPrefix + strings.ToUpper(name) + "_"
	cfg, err := LoadConfigFromEnvWithPrefix(prefix)
	if err != nil {
		return NamedConnectionConfig{}, err
	}
	return NamedConnectionConfig{Name: strings.ToLower(name), Config: cfg}, nil
}

// LoadNamedConfigFromSource is the test-friendly variant of LoadNamedConfigFromEnv.
func LoadNamedConfigFromSource(name string, lookup func(string) (string, bool)) (NamedConnectionConfig, error) {
	prefix := EnvPrefix + strings.ToUpper(name) + "_"
	cfg, err := LoadConfigFromSource(prefix, lookup)
	if err != nil {
		return NamedConnectionConfig{}, err
	}
	return NamedConnectionConfig{Name: strings.ToLower(name), Config: cfg}, nil
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

func validateURL(url string) error {
	if url == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "URL cannot be empty")
	}
	_, err := detectProtocol(url)
	return err
}

func validateIdentifier(value, context string) error {
	if value == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "Identifier cannot be empty")
	}
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"Identifier (%s) must be alphanumeric with optional underscores/hyphens", context)
		}
	}
	return nil
}

func validateNumericRange(name string, value, minVal, maxVal float64) error {
	if math.IsNaN(value) {
		return surqlerrors.Newf(surqlerrors.ErrValidation, "%s must be a finite number", name)
	}
	if value < minVal {
		return surqlerrors.Newf(surqlerrors.ErrValidation, "%s must be >= %v", name, minVal)
	}
	if value > maxVal {
		return surqlerrors.Newf(surqlerrors.ErrValidation, "%s must be <= %v", name, maxVal)
	}
	return nil
}

func detectProtocol(url string) (Protocol, error) {
	trimmed := strings.TrimSpace(url)
	switch {
	case strings.HasPrefix(trimmed, "ws://"):
		if trimmed == "ws://" {
			return 0, surqlerrors.New(surqlerrors.ErrValidation, "URL host must not be empty")
		}
		return ProtocolWebSocket, nil
	case strings.HasPrefix(trimmed, "wss://"):
		return ProtocolWebSocketSecure, nil
	case strings.HasPrefix(trimmed, "http://"):
		return ProtocolHTTP, nil
	case strings.HasPrefix(trimmed, "https://"):
		return ProtocolHTTPS, nil
	case strings.HasPrefix(trimmed, "mem://"), strings.HasPrefix(trimmed, "memory://"):
		return ProtocolMemory, nil
	case strings.HasPrefix(trimmed, "file://"):
		return ProtocolFile, nil
	case strings.HasPrefix(trimmed, "surrealkv://"):
		return ProtocolSurrealKV, nil
	default:
		return 0, surqlerrors.New(surqlerrors.ErrValidation,
			"URL must use one of: ws://, wss://, http://, https://, mem://, memory://, file://, surrealkv://")
	}
}

func getWithAliases(lookup func(string) (string, bool), prefix string, keys ...string) (string, bool) {
	for _, k := range keys {
		name := prefix + k
		if v, ok := lookup(name); ok {
			return v, true
		}
		if v, ok := lookup(strings.ToLower(name)); ok {
			return v, true
		}
	}
	return "", false
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, surqlerrors.Newf(surqlerrors.ErrValidation, "invalid boolean value %q", s)
	}
}
