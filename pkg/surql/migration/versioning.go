package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/schema"
)

// snapshotFileSuffix is the filename suffix used for persisted snapshots.
// Snapshots are stored as "<version>.snapshot.json".
const snapshotFileSuffix = ".snapshot.json"

// snapshotClock is the clock source used by CreateSnapshot. Tests override it
// to make Version / Timestamp deterministic; production code always uses
// time.Now in UTC.
var snapshotClock = func() time.Time { return time.Now().UTC() }

// CreateSnapshot builds a SchemaSnapshot from the given SchemaRegistry.
//
// The Version is a UTC timestamp in YYYYMMDD_HHMMSS form matching the
// migration file convention. Tables, Edges, and Accesses are copied from the
// registry in deterministic (alphabetical) order. The snapshot captures the
// registry as-of the call; subsequent registry mutations do not affect the
// returned value.
//
// A nil registry is rejected with ErrValidation; an empty description is
// allowed (matches surql-py where description is optional metadata).
func CreateSnapshot(registry *schema.SchemaRegistry, description string) (SchemaSnapshot, error) {
	if registry == nil {
		return SchemaSnapshot{}, surqlerrors.New(surqlerrors.ErrValidation,
			"cannot create snapshot from nil registry")
	}

	ts := snapshotClock()
	version := ts.Format(timestampLayout)

	return SchemaSnapshot{
		Version:     version,
		Timestamp:   ts,
		Description: description,
		Tables:      registry.Tables(),
		Edges:       registry.Edges(),
		Accesses:    []schema.AccessDefinition{},
	}, nil
}

// StoreSnapshot writes a snapshot to a JSON file at path.
//
// If path is a directory, the snapshot is written to
// "<path>/<version>.snapshot.json". If path already ends with the expected
// suffix it is used verbatim. The snapshot must have a non-empty Version.
// Parent directories are created as needed with 0o755 permissions; the file
// itself is written with 0o644.
func StoreSnapshot(snapshot SchemaSnapshot, path string) error {
	if snapshot.Version == "" {
		return surqlerrors.New(surqlerrors.ErrValidation,
			"cannot store snapshot with empty version")
	}
	if path == "" {
		return surqlerrors.New(surqlerrors.ErrValidation,
			"snapshot path cannot be empty")
	}

	target, err := resolveSnapshotPath(path, snapshot.Version)
	if err != nil {
		return err
	}

	if dir := filepath.Dir(target); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrSerialization, err,
				"failed to create snapshot directory %q", dir)
		}
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return surqlerrors.Wrapf(surqlerrors.ErrSerialization, err,
			"failed to marshal snapshot %q", snapshot.Version)
	}

	// Snapshots are intended to be world-readable (e.g. committed to the
	// repository or shared between developers); 0644 is deliberate here.
	if err := os.WriteFile(target, data, 0o644); err != nil { //nolint:gosec // G306: snapshots are intentionally world-readable
		return surqlerrors.Wrapf(surqlerrors.ErrSerialization, err,
			"failed to write snapshot file %q", target)
	}
	return nil
}

// LoadSnapshot reads a snapshot JSON file from path and returns the decoded
// SchemaSnapshot.
//
// A missing file returns ErrMigrationLoad; a malformed file returns
// ErrSerialization. The returned snapshot is validated for a non-empty
// Version field — legacy snapshots without versioning metadata are rejected
// so that callers can rely on Version being present.
func LoadSnapshot(path string) (SchemaSnapshot, error) {
	if path == "" {
		return SchemaSnapshot{}, surqlerrors.New(surqlerrors.ErrValidation,
			"snapshot path cannot be empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return SchemaSnapshot{}, surqlerrors.Wrapf(surqlerrors.ErrMigrationLoad, err,
			"failed to read snapshot file %q", path)
	}

	var snapshot SchemaSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return SchemaSnapshot{}, surqlerrors.Wrapf(surqlerrors.ErrSerialization, err,
			"failed to decode snapshot file %q", path)
	}

	if snapshot.Version == "" {
		return SchemaSnapshot{}, surqlerrors.Newf(surqlerrors.ErrSerialization,
			"snapshot file %q has empty version", path)
	}
	return snapshot, nil
}

// ListSnapshots scans dir for snapshot files and returns the decoded
// snapshots in ascending Version order.
//
// Files that do not match the snapshot suffix are skipped. Individual decode
// failures are surfaced as errors (rather than silently dropped) so callers
// can detect corruption. A non-existent directory returns
// ErrMigrationDiscovery.
func ListSnapshots(dir string) ([]SchemaSnapshot, error) {
	if dir == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation,
			"snapshot directory cannot be empty")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrMigrationDiscovery, err,
			"failed to read snapshot directory %q", dir)
	}

	snapshots := make([]SchemaSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, snapshotFileSuffix) {
			continue
		}
		snap, err := LoadSnapshot(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Version < snapshots[j].Version
	})
	return snapshots, nil
}

// CompareSnapshots computes the set of SchemaDiff operations needed to evolve
// the "from" snapshot into the "to" snapshot.
//
// Internally this is DiffSchemas(to, from) — DiffSchemas treats the first
// argument as the desired (code) state and the second as the current (db)
// state, so passing to/from in that order yields ADD diffs for entries
// present in "to" but missing in "from", DROP diffs for the reverse.
//
// Accesses are not yet part of DiffSchemas; CompareSnapshots reports only
// table and edge differences. Future work can extend the diff engine to
// cover DEFINE ACCESS statements.
func CompareSnapshots(from, to SchemaSnapshot) ([]SchemaDiff, error) {
	return DiffSchemas(to, from)
}

// VersionNode is a node in the VersionGraph, tying a snapshot to its parent
// (previous schema version) and child (subsequent) versions.
//
// Parent is the empty string for root nodes. Children is maintained by the
// owning VersionGraph and is always kept in sorted order for deterministic
// traversal.
type VersionNode struct {
	Version  string         `json:"version"`
	Parent   string         `json:"parent,omitempty"`
	Snapshot SchemaSnapshot `json:"snapshot"`
	Children []string       `json:"children,omitempty"`
}

// VersionGraph is a thread-safe DAG of schema snapshots keyed by Version.
//
// It mirrors surql-py's VersionGraph: snapshots are inserted with an optional
// parent, and the graph exposes path-finding (BFS), ancestor, descendant, and
// bulk version listing helpers used by rollback and version comparison
// tooling.
type VersionGraph struct {
	mu    sync.RWMutex
	nodes map[string]*VersionNode
	root  string
}

// NewVersionGraph returns an empty VersionGraph.
func NewVersionGraph() *VersionGraph {
	return &VersionGraph{nodes: make(map[string]*VersionNode)}
}

// Add inserts a snapshot into the graph under the given parent.
//
// Parent may be the empty string to register a root node; the first
// parentless node becomes the graph's root. A duplicate Version returns
// ErrValidation. A non-empty parent that does not yet exist in the graph
// also returns ErrValidation. The snapshot must have a non-empty Version.
func (g *VersionGraph) Add(snapshot SchemaSnapshot, parent string) error {
	if snapshot.Version == "" {
		return surqlerrors.New(surqlerrors.ErrValidation,
			"cannot add snapshot with empty version to graph")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.nodes[snapshot.Version]; exists {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"version %q already exists in graph", snapshot.Version)
	}
	if parent != "" {
		if _, ok := g.nodes[parent]; !ok {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"parent version %q not found in graph", parent)
		}
	}

	node := &VersionNode{
		Version:  snapshot.Version,
		Parent:   parent,
		Snapshot: snapshot,
	}
	g.nodes[snapshot.Version] = node

	if parent != "" {
		parentNode := g.nodes[parent]
		parentNode.Children = insertSorted(parentNode.Children, snapshot.Version)
	} else if g.root == "" {
		g.root = snapshot.Version
	}
	return nil
}

// Remove deletes a node from the graph.
//
// Removal rewires the graph: each child of the removed node is re-parented
// to the removed node's parent (or becomes a root if no parent exists). A
// missing version returns ErrValidation.
func (g *VersionGraph) Remove(version string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	node, ok := g.nodes[version]
	if !ok {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"version %q not found in graph", version)
	}

	// Detach from parent's children list.
	if node.Parent != "" {
		if parentNode, ok := g.nodes[node.Parent]; ok {
			parentNode.Children = removeFromSlice(parentNode.Children, version)
		}
	}

	// Re-parent children onto the removed node's parent (or promote to root).
	for _, childVersion := range node.Children {
		child, ok := g.nodes[childVersion]
		if !ok {
			continue
		}
		child.Parent = node.Parent
		if node.Parent != "" {
			if parentNode, ok := g.nodes[node.Parent]; ok {
				parentNode.Children = insertSorted(parentNode.Children, childVersion)
			}
		}
	}

	delete(g.nodes, version)

	// Reset root if we just removed it; promote an arbitrary remaining root.
	if g.root == version {
		g.root = ""
		for v, n := range g.nodes {
			if n.Parent == "" {
				g.root = v
				break
			}
		}
	}
	return nil
}

// Get returns the VersionNode for the given version (and whether it exists).
// The returned node is a shallow copy; mutating its Children slice is safe
// and will not affect graph state.
func (g *VersionGraph) Get(version string) (VersionNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	node, ok := g.nodes[version]
	if !ok {
		return VersionNode{}, false
	}
	cp := *node
	cp.Children = append([]string(nil), node.Children...)
	return cp, true
}

// Has reports whether the given version exists in the graph.
func (g *VersionGraph) Has(version string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.nodes[version]
	return ok
}

// Root returns the root version (empty string when the graph is empty).
func (g *VersionGraph) Root() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.root
}

// Len returns the number of versions in the graph.
func (g *VersionGraph) Len() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// Versions returns every version in the graph sorted alphabetically.
func (g *VersionGraph) Versions() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]string, 0, len(g.nodes))
	for v := range g.nodes {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// Path returns the shortest path from one version to another, traversing
// both parent and child edges (BFS). An empty slice is returned when either
// endpoint is missing; a nil slice is returned when no path exists between
// existing nodes.
func (g *VersionGraph) Path(from, to string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.nodes[from]; !ok {
		return nil
	}
	if _, ok := g.nodes[to]; !ok {
		return nil
	}
	if from == to {
		return []string{from}
	}

	type state struct {
		version string
		path    []string
	}
	queue := []state{{version: from, path: []string{from}}}
	visited := map[string]struct{}{from: {}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		node := g.nodes[cur.version]
		neighbours := make([]string, 0, len(node.Children)+1)
		neighbours = append(neighbours, node.Children...)
		if node.Parent != "" {
			neighbours = append(neighbours, node.Parent)
		}

		for _, n := range neighbours {
			if _, seen := visited[n]; seen {
				continue
			}
			visited[n] = struct{}{}
			newPath := make([]string, len(cur.path)+1)
			copy(newPath, cur.path)
			newPath[len(cur.path)] = n
			if n == to {
				return newPath
			}
			queue = append(queue, state{version: n, path: newPath})
		}
	}
	return nil
}

// Ancestors returns every ancestor of the given version, ordered from root
// down to the immediate parent. An unknown version returns nil.
func (g *VersionGraph) Ancestors(version string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.nodes[version]; !ok {
		return nil
	}

	var ancestors []string
	current := version
	for {
		node := g.nodes[current]
		if node == nil || node.Parent == "" {
			break
		}
		ancestors = append([]string{node.Parent}, ancestors...)
		current = node.Parent
	}
	return ancestors
}

// Descendants returns every descendant of the given version in BFS order.
// An unknown version returns nil.
func (g *VersionGraph) Descendants(version string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	node, ok := g.nodes[version]
	if !ok {
		return nil
	}

	descendants := make([]string, 0)
	queue := append([]string(nil), node.Children...)
	visited := make(map[string]struct{}, len(node.Children))
	for _, c := range node.Children {
		visited[c] = struct{}{}
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		descendants = append(descendants, cur)

		childNode, ok := g.nodes[cur]
		if !ok {
			continue
		}
		for _, c := range childNode.Children {
			if _, seen := visited[c]; seen {
				continue
			}
			visited[c] = struct{}{}
			queue = append(queue, c)
		}
	}
	return descendants
}

// resolveSnapshotPath normalises a caller-supplied path + version into the
// absolute snapshot filename. When path names an existing directory the
// snapshot is placed under "<path>/<version>.snapshot.json"; when path ends
// with the snapshot suffix it is used verbatim; otherwise the path is
// treated as the destination file name (directory created on write).
func resolveSnapshotPath(path, version string) (string, error) {
	if strings.HasSuffix(path, snapshotFileSuffix) {
		return path, nil
	}

	info, err := os.Stat(path)
	switch {
	case err == nil && info.IsDir():
		return filepath.Join(path, fmt.Sprintf("%s%s", version, snapshotFileSuffix)), nil
	case err == nil:
		return "", surqlerrors.Newf(surqlerrors.ErrValidation,
			"snapshot path %q exists and is neither a directory nor a %s file",
			path, snapshotFileSuffix)
	case os.IsNotExist(err):
		// Non-existent paths without the snapshot suffix are treated as a
		// directory that should be created lazily on write.
		return filepath.Join(path, fmt.Sprintf("%s%s", version, snapshotFileSuffix)), nil
	default:
		return "", surqlerrors.Wrapf(surqlerrors.ErrValidation, err,
			"failed to stat snapshot path %q", path)
	}
}

// insertSorted returns a copy of slice with value inserted in sorted order,
// preserving uniqueness.
func insertSorted(slice []string, value string) []string {
	idx := sort.SearchStrings(slice, value)
	if idx < len(slice) && slice[idx] == value {
		return slice
	}
	out := make([]string, len(slice)+1)
	copy(out, slice[:idx])
	out[idx] = value
	copy(out[idx+1:], slice[idx:])
	return out
}

// removeFromSlice returns a copy of slice with the first occurrence of value
// removed.
func removeFromSlice(slice []string, value string) []string {
	for i, v := range slice {
		if v == value {
			out := make([]string, 0, len(slice)-1)
			out = append(out, slice[:i]...)
			out = append(out, slice[i+1:]...)
			return out
		}
	}
	return append([]string(nil), slice...)
}
