package schema

import (
	"sort"
	"sync"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// SchemaRegistry is a thread-safe registry for code-defined table and edge
// schemas. It mirrors surql-py's SchemaRegistry but uses a sync.RWMutex for
// concurrent access and emits definitions in deterministic (alphabetical)
// order for schema comparison and SQL generation.
type SchemaRegistry struct {
	mu     sync.RWMutex
	tables map[string]TableDefinition
	edges  map[string]EdgeDefinition
}

// NewSchemaRegistry constructs an empty SchemaRegistry. Prefer GetRegistry for
// the process-wide singleton used by code-as-schema consumers; NewSchemaRegistry
// is primarily useful for tests and scoped schema registration.
func NewSchemaRegistry() *SchemaRegistry {
	return &SchemaRegistry{
		tables: make(map[string]TableDefinition),
		edges:  make(map[string]EdgeDefinition),
	}
}

var (
	globalRegistry     *SchemaRegistry
	globalRegistryOnce sync.Once
)

// GetRegistry returns the process-wide SchemaRegistry singleton, creating it
// on first call. It is safe for concurrent use.
func GetRegistry() *SchemaRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewSchemaRegistry()
	})
	return globalRegistry
}

// RegisterTable stores a table definition keyed by its name. An empty-name
// table returns ErrValidation. Re-registering the same name silently replaces
// the previous definition, matching surql-py semantics.
func (r *SchemaRegistry) RegisterTable(table TableDefinition) error {
	if table.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation,
			"cannot register table with empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tables[table.Name] = table
	return nil
}

// RegisterEdge stores an edge definition keyed by its name.
func (r *SchemaRegistry) RegisterEdge(edge EdgeDefinition) error {
	if edge.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation,
			"cannot register edge with empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.edges[edge.Name] = edge
	return nil
}

// GetTable returns a registered table definition and a boolean indicating
// whether it was found.
func (r *SchemaRegistry) GetTable(name string) (TableDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tables[name]
	return t, ok
}

// GetEdge returns a registered edge definition and a boolean indicating
// whether it was found.
func (r *SchemaRegistry) GetEdge(name string) (EdgeDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.edges[name]
	return e, ok
}

// Tables returns every registered TableDefinition sorted alphabetically by
// name. The returned slice is a fresh copy; callers may mutate it freely.
func (r *SchemaRegistry) Tables() []TableDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tables))
	for name := range r.tables {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]TableDefinition, 0, len(names))
	for _, name := range names {
		out = append(out, r.tables[name])
	}
	return out
}

// Edges returns every registered EdgeDefinition sorted alphabetically by name.
// The returned slice is a fresh copy.
func (r *SchemaRegistry) Edges() []EdgeDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.edges))
	for name := range r.edges {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]EdgeDefinition, 0, len(names))
	for _, name := range names {
		out = append(out, r.edges[name])
	}
	return out
}

// TableNames returns the names of every registered table, sorted.
func (r *SchemaRegistry) TableNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tables))
	for name := range r.tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// EdgeNames returns the names of every registered edge, sorted.
func (r *SchemaRegistry) EdgeNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.edges))
	for name := range r.edges {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TableCount returns the number of registered tables.
func (r *SchemaRegistry) TableCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tables)
}

// EdgeCount returns the number of registered edges.
func (r *SchemaRegistry) EdgeCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.edges)
}

// Clear removes every registered table and edge. Useful for test isolation.
func (r *SchemaRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tables = make(map[string]TableDefinition)
	r.edges = make(map[string]EdgeDefinition)
}
