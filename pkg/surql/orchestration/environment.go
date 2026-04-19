package orchestration

import (
	"encoding/json"
	"os"
	"sort"
	"sync"

	"github.com/Oneiriq/surql-go/pkg/surql/connection"
	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// EnvironmentConfig describes a single database environment the coordinator
// can deploy migrations to.
//
// The struct mirrors surql-py's frozen `EnvironmentConfig` model. Instances
// are expected to be treated as immutable after construction; the registry
// returns copies so callers cannot mutate registered entries.
type EnvironmentConfig struct {
	// Name is the unique environment identifier (e.g. "production").
	Name string `json:"name"`
	// Connection carries the SurrealDB connection details. The connection
	// is validated when the environment is registered.
	Connection connection.ConnectionConfig `json:"connection"`
	// Priority controls ordering in ListEnvironments: lower numbers sort
	// first. Defaults to 100 when left at zero-value.
	Priority int `json:"priority"`
	// Tags are free-form labels used for categorisation (e.g. "prod",
	// "critical"). Stored as a set to match py semantics.
	Tags map[string]struct{} `json:"tags,omitempty"`
	// RequireApproval indicates the CLI should prompt for confirmation
	// before deploying. The coordinator does not enforce approval itself;
	// it is surfaced for operator tooling.
	RequireApproval bool `json:"require_approval,omitempty"`
	// AllowDestructive controls whether destructive migrations (DROP,
	// REMOVE, ...) are permitted against this environment. Defaults to
	// true to mirror py.
	AllowDestructive bool `json:"allow_destructive"`
}

// defaultPriority is applied when EnvironmentConfig.Priority is unset (zero).
// Matches surql-py's default of 100.
const defaultPriority = 100

// HasTag reports whether the environment carries the named tag.
func (e EnvironmentConfig) HasTag(tag string) bool {
	if e.Tags == nil {
		return false
	}
	_, ok := e.Tags[tag]
	return ok
}

// TagList returns the environment's tags as a sorted slice for stable
// iteration/logging.
func (e EnvironmentConfig) TagList() []string {
	tags := make([]string, 0, len(e.Tags))
	for t := range e.Tags {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

// validate enforces the rules from surql-py's EnvironmentConfig validator.
// name must be non-empty and alphanumeric (plus '_' / '-'); the underlying
// connection config must validate.
func (e EnvironmentConfig) validate() error {
	if e.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "environment name cannot be empty")
	}
	for _, r := range e.Name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"environment name %q must be alphanumeric with optional underscores/hyphens", e.Name)
		}
	}
	if err := e.Connection.Validate(); err != nil {
		return err
	}
	if e.Priority < 0 {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"environment priority must be >= 0, got %d", e.Priority)
	}
	return nil
}

// normalised returns a defensive copy with defaulted Priority and a non-nil
// Tags map so downstream callers never have to nil-check.
func (e EnvironmentConfig) normalised() EnvironmentConfig {
	out := e
	if out.Priority == 0 {
		out.Priority = defaultPriority
	}
	if out.Tags == nil {
		out.Tags = map[string]struct{}{}
	} else {
		tags := make(map[string]struct{}, len(out.Tags))
		for k := range out.Tags {
			tags[k] = struct{}{}
		}
		out.Tags = tags
	}
	return out
}

// EnvironmentRegistry is a thread-safe map of named EnvironmentConfig
// entries. It mirrors surql-py's `EnvironmentRegistry` class while using
// Go's sync.RWMutex — async primitives are unnecessary for an in-memory
// lookup table.
//
// Construct with NewEnvironmentRegistry or reach the package-level
// singleton via GetRegistry. The zero value is not usable.
type EnvironmentRegistry struct {
	mu           sync.RWMutex
	environments map[string]EnvironmentConfig
}

// NewEnvironmentRegistry constructs an empty registry. Prefer GetRegistry
// when you want the shared library-level registry; NewEnvironmentRegistry
// is useful for tests that want isolation.
func NewEnvironmentRegistry() *EnvironmentRegistry {
	return &EnvironmentRegistry{environments: map[string]EnvironmentConfig{}}
}

// RegisterOptions tunes RegisterEnvironment behaviour. All fields are
// optional; zero-values mirror surql-py's defaults.
type RegisterOptions struct {
	// Priority overrides the default of 100 when non-zero.
	Priority int
	// Tags is a set of free-form labels.
	Tags []string
	// RequireApproval toggles operator-confirmation gating.
	RequireApproval bool
	// AllowDestructive, when explicitly set to a non-nil pointer, replaces
	// the default of true. A nil pointer leaves the default in place.
	AllowDestructive *bool
}

// Register adds a new environment. Returns ErrRegistry when name is
// already registered and ErrValidation on invalid input (empty name,
// invalid ConnectionConfig, negative priority).
//
// The stored config is normalised (Priority defaults to 100,
// AllowDestructive defaults to true) and frozen; callers can freely mutate
// their local copy of cfg afterwards.
func (r *EnvironmentRegistry) Register(
	name string,
	cfg connection.ConnectionConfig,
	opts *RegisterOptions,
) error {
	allowDestructive := true
	priority := defaultPriority
	var tags []string
	var requireApproval bool

	if opts != nil {
		if opts.Priority > 0 {
			priority = opts.Priority
		}
		tags = opts.Tags
		requireApproval = opts.RequireApproval
		if opts.AllowDestructive != nil {
			allowDestructive = *opts.AllowDestructive
		}
	}

	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		tagSet[t] = struct{}{}
	}

	env := EnvironmentConfig{
		Name:             name,
		Connection:       cfg,
		Priority:         priority,
		Tags:             tagSet,
		RequireApproval:  requireApproval,
		AllowDestructive: allowDestructive,
	}
	if err := env.validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.environments[name]; exists {
		return surqlerrors.Newf(surqlerrors.ErrRegistry,
			"environment %q already registered", name)
	}
	r.environments[name] = env.normalised()
	return nil
}

// Unregister removes the named environment. Returns ErrRegistry when the
// environment does not exist.
func (r *EnvironmentRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.environments[name]; !ok {
		return surqlerrors.Newf(surqlerrors.ErrRegistry,
			"environment %q not found", name)
	}
	delete(r.environments, name)
	return nil
}

// Get returns the named environment configuration. Returns
// ErrRegistry when the environment is not registered.
//
// The returned value is a copy — mutations do not affect the registry.
func (r *EnvironmentRegistry) Get(name string) (EnvironmentConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	env, ok := r.environments[name]
	if !ok {
		return EnvironmentConfig{}, surqlerrors.Newf(surqlerrors.ErrRegistry,
			"environment %q not found", name)
	}
	return env, nil
}

// List returns the registered environment names sorted by priority
// ascending (ties broken alphabetically). Matches py's
// `list_environments` semantics.
func (r *EnvironmentRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	envs := make([]EnvironmentConfig, 0, len(r.environments))
	for _, e := range r.environments {
		envs = append(envs, e)
	}
	sort.Slice(envs, func(i, j int) bool {
		if envs[i].Priority != envs[j].Priority {
			return envs[i].Priority < envs[j].Priority
		}
		return envs[i].Name < envs[j].Name
	})
	names := make([]string, 0, len(envs))
	for _, e := range envs {
		names = append(names, e.Name)
	}
	return names
}

// FindByTag returns every environment tagged with tag, sorted by priority
// ascending.
func (r *EnvironmentRegistry) FindByTag(tag string) []EnvironmentConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	matches := make([]EnvironmentConfig, 0)
	for _, e := range r.environments {
		if _, ok := e.Tags[tag]; ok {
			matches = append(matches, e)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Priority != matches[j].Priority {
			return matches[i].Priority < matches[j].Priority
		}
		return matches[i].Name < matches[j].Name
	})
	return matches
}

// Len returns the number of registered environments.
func (r *EnvironmentRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.environments)
}

// Clear removes every registered environment, leaving the registry empty.
// Useful in tests.
func (r *EnvironmentRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.environments = map[string]EnvironmentConfig{}
}

// environmentsFileSchema is the JSON shape parsed by LoadRegistryFromFile.
// It mirrors surql-py's config file format.
type environmentsFileSchema struct {
	Environments []struct {
		Name             string                      `json:"name"`
		Connection       connection.ConnectionConfig `json:"connection"`
		Priority         int                         `json:"priority,omitempty"`
		Tags             []string                    `json:"tags,omitempty"`
		RequireApproval  bool                        `json:"require_approval,omitempty"`
		AllowDestructive *bool                       `json:"allow_destructive,omitempty"`
	} `json:"environments"`
}

// LoadRegistryFromFile parses a JSON file of the form
//
//	{"environments": [{"name": "...", "connection": {...}, ...}, ...]}
//
// and returns a populated registry. A missing file produces an empty
// registry (matching py's behaviour); malformed JSON or validation
// failures surface as ErrSerialization / ErrValidation.
func LoadRegistryFromFile(path string) (*EnvironmentRegistry, error) {
	registry := NewEnvironmentRegistry()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return registry, nil
		}
		return nil, surqlerrors.Wrapf(surqlerrors.ErrOrchestration, err,
			"failed to read environments config %q", path)
	}

	var schema environmentsFileSchema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrSerialization, err,
			"failed to parse environments config %q", path)
	}

	for _, entry := range schema.Environments {
		opts := &RegisterOptions{
			Priority:        entry.Priority,
			Tags:            entry.Tags,
			RequireApproval: entry.RequireApproval,
		}
		if entry.AllowDestructive != nil {
			opts.AllowDestructive = entry.AllowDestructive
		}
		if err := registry.Register(entry.Name, entry.Connection, opts); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// globalRegistry holds the package-level singleton returned by GetRegistry.
// A sync.Mutex guards replacement via SetRegistry; the registry's own
// RWMutex handles concurrent reads/writes of the environment map.
var (
	globalRegistryMu sync.Mutex
	globalRegistry   = NewEnvironmentRegistry()
)

// GetRegistry returns the shared package-level EnvironmentRegistry.
// Mirrors surql-py's `get_registry()`.
func GetRegistry() *EnvironmentRegistry {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()
	return globalRegistry
}

// SetRegistry replaces the package-level registry. Useful in tests that
// want a hermetic registry without mutating shared state.
func SetRegistry(r *EnvironmentRegistry) {
	if r == nil {
		r = NewEnvironmentRegistry()
	}
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()
	globalRegistry = r
}

// ConfigureEnvironments replaces the package-level registry with one
// hydrated from path. Mirrors py's `configure_environments`.
func ConfigureEnvironments(path string) error {
	registry, err := LoadRegistryFromFile(path)
	if err != nil {
		return err
	}
	SetRegistry(registry)
	return nil
}

// RegisterEnvironment registers cfg under name in the package-level
// registry. Convenience wrapper around GetRegistry().Register.
func RegisterEnvironment(
	name string,
	cfg connection.ConnectionConfig,
	opts *RegisterOptions,
) error {
	return GetRegistry().Register(name, cfg, opts)
}
