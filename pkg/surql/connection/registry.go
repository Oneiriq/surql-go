package connection

import (
	"context"
	"sort"
	"sync"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// Registry is a thread-safe map of named DatabaseClient connections. It
// mirrors surql-py's `ConnectionRegistry` singleton but is a plain struct
// behind an RWMutex — Go does not need async primitives for this.
//
// The zero value is not usable; consumers interact with the package-level
// default via GetRegistry, or construct ad-hoc registries with
// NewRegistry for isolation in tests.
type Registry struct {
	mu          sync.RWMutex
	connections map[string]*DatabaseClient
	configs     map[string]ConnectionConfig
	defaultName string
}

// NewRegistry constructs an empty Registry. Most callers should use
// GetRegistry to share the package-level singleton; NewRegistry is handy in
// tests that want a pristine namespace.
func NewRegistry() *Registry {
	return &Registry{
		connections: map[string]*DatabaseClient{},
		configs:     map[string]ConnectionConfig{},
	}
}

// defaultRegistry is the package-level singleton returned by GetRegistry.
var defaultRegistry = NewRegistry()

// GetRegistry returns the shared Registry used by the library's global
// connection namespace. Mirrors surql-py's `get_registry()`.
func GetRegistry() *Registry {
	return defaultRegistry
}

// RegisterOptions tunes the behaviour of Register.
//
// Defaults (zero value):
//   - Connect:    true  — dial the database immediately.
//   - SetDefault: false — only the first registered connection becomes default.
//
// Pointers are used so callers can distinguish "not set" from the zero value,
// letting them override specific fields without copy-and-paste defaults.
type RegisterOptions struct {
	// Connect, when non-nil, controls whether the client is connected
	// during registration. Defaults to true.
	Connect *bool
	// SetDefault, when non-nil and true, marks the newly registered
	// connection as the default. Defaults to false, except that the first
	// registered connection always becomes the default.
	SetDefault *bool
}

// Register adds a new named connection. Returns ErrRegistry if name is
// already in use.
//
// If opts.Connect is nil or *true, the client is connected synchronously
// with ctx; the Connect error is returned verbatim on failure (the registry
// is left unchanged).
//
// The first connection ever registered becomes the default; explicit
// opts.SetDefault==true promotes a later connection ahead of it.
func (r *Registry) Register(
	ctx context.Context,
	name string,
	cfg ConnectionConfig,
	opts *RegisterOptions,
) (*DatabaseClient, error) {
	if name == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "connection name cannot be empty")
	}

	connect := true
	setDefault := false
	if opts != nil {
		if opts.Connect != nil {
			connect = *opts.Connect
		}
		if opts.SetDefault != nil {
			setDefault = *opts.SetDefault
		}
	}

	// Build the client outside the lock so its Connect (a blocking
	// network call) doesn't serialise registry access. We only grab the
	// lock once I/O has completed.
	client, err := NewDatabaseClient(cfg)
	if err != nil {
		return nil, err
	}
	if connect {
		if err := client.Connect(ctx); err != nil {
			return nil, err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.connections[name]; exists {
		if connect {
			// Undo the connection we just made so we don't leak sockets.
			_ = client.Disconnect()
		}
		return nil, surqlerrors.Newf(
			surqlerrors.ErrRegistry,
			"connection %q already registered", name,
		)
	}

	r.connections[name] = client
	r.configs[name] = cfg
	if setDefault || r.defaultName == "" {
		r.defaultName = name
	}
	return client, nil
}

// Unregister removes the named connection and, when disconnect is true,
// closes it.
//
// Returns ErrRegistry if the connection does not exist. When the removed
// connection was the default, the default falls back to the
// lexicographically first remaining entry (or empty if none remain).
func (r *Registry) Unregister(ctx context.Context, name string, disconnect bool) error {
	r.mu.Lock()
	client, exists := r.connections[name]
	if !exists {
		r.mu.Unlock()
		return surqlerrors.Newf(surqlerrors.ErrRegistry, "connection %q not found", name)
	}
	delete(r.connections, name)
	delete(r.configs, name)
	if r.defaultName == name {
		r.defaultName = firstName(r.connections)
	}
	r.mu.Unlock()

	if disconnect && client != nil {
		if err := client.Disconnect(); err != nil {
			return err
		}
	}
	_ = ctx // retained for symmetry with py; Disconnect currently ignores ctx
	return nil
}

// Get returns the named connection or, when name is empty, the default
// connection. Returns ErrRegistry when the connection does not exist.
func (r *Registry) Get(name string) (*DatabaseClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		if r.defaultName == "" {
			return nil, surqlerrors.New(surqlerrors.ErrRegistry, "no default connection set")
		}
		name = r.defaultName
	}
	client, ok := r.connections[name]
	if !ok {
		return nil, surqlerrors.Newf(surqlerrors.ErrRegistry, "connection %q not found", name)
	}
	return client, nil
}

// GetConfig returns the configuration for the named connection (or default
// when name is empty). Returns ErrRegistry when the connection does not exist.
func (r *Registry) GetConfig(name string) (ConnectionConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		if r.defaultName == "" {
			return ConnectionConfig{}, surqlerrors.New(
				surqlerrors.ErrRegistry, "no default connection set",
			)
		}
		name = r.defaultName
	}
	cfg, ok := r.configs[name]
	if !ok {
		return ConnectionConfig{}, surqlerrors.Newf(
			surqlerrors.ErrRegistry, "connection %q not found", name,
		)
	}
	return cfg, nil
}

// SetDefault promotes name to the default connection. Returns ErrRegistry
// when the connection does not exist.
func (r *Registry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.connections[name]; !ok {
		return surqlerrors.Newf(surqlerrors.ErrRegistry, "connection %q not found", name)
	}
	r.defaultName = name
	return nil
}

// DefaultName returns the name of the default connection (empty string when
// none is set).
func (r *Registry) DefaultName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultName
}

// List returns the registered connection names sorted lexicographically so
// the output is stable across calls.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.connections))
	for name := range r.connections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DisconnectAll closes every registered connection. The registry entries
// remain; call Clear to also remove them.
//
// The first non-nil Disconnect error is returned after attempting every
// connection, so callers still drain the full set.
func (r *Registry) DisconnectAll(ctx context.Context) error {
	r.mu.RLock()
	clients := make([]*DatabaseClient, 0, len(r.connections))
	for _, c := range r.connections {
		clients = append(clients, c)
	}
	r.mu.RUnlock()

	var firstErr error
	for _, c := range clients {
		if c == nil || !c.IsConnected() {
			continue
		}
		if err := c.Disconnect(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	_ = ctx
	return firstErr
}

// Clear disconnects and removes every registered connection, leaving the
// registry empty. Errors from DisconnectAll are returned verbatim.
func (r *Registry) Clear(ctx context.Context) error {
	if err := r.DisconnectAll(ctx); err != nil {
		// Drain the map even when Disconnect fails so consumers are not
		// stuck with phantom entries.
		r.mu.Lock()
		r.connections = map[string]*DatabaseClient{}
		r.configs = map[string]ConnectionConfig{}
		r.defaultName = ""
		r.mu.Unlock()
		return err
	}
	r.mu.Lock()
	r.connections = map[string]*DatabaseClient{}
	r.configs = map[string]ConnectionConfig{}
	r.defaultName = ""
	r.mu.Unlock()
	return nil
}

// firstName returns the lexicographically first key in m, or the empty
// string when m is empty. Used to pick a fallback default when the current
// default is removed.
func firstName(m map[string]*DatabaseClient) string {
	if len(m) == 0 {
		return ""
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names[0]
}
