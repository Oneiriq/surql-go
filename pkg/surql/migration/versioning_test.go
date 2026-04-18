package migration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/schema"
)

// withSnapshotClock overrides snapshotClock for the duration of the test.
func withSnapshotClock(t *testing.T, fixed time.Time) {
	t.Helper()
	previous := snapshotClock
	snapshotClock = func() time.Time { return fixed }
	t.Cleanup(func() { snapshotClock = previous })
}

// fixedTime is a deterministic timestamp used across tests.
func fixedTime() time.Time {
	return time.Date(2026, 4, 18, 12, 30, 45, 0, time.UTC)
}

// populatedRegistry returns a registry containing one table + one edge.
func populatedRegistry(t *testing.T) *schema.SchemaRegistry {
	t.Helper()
	reg := schema.NewSchemaRegistry()
	if err := reg.RegisterTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email"), schema.StringField("name")),
	)); err != nil {
		t.Fatalf("failed to register table: %v", err)
	}
	if err := reg.RegisterEdge(schema.TypedEdge("likes", "user", "post")); err != nil {
		t.Fatalf("failed to register edge: %v", err)
	}
	return reg
}

func TestCreateSnapshotPopulatesVersionAndTimestamp(t *testing.T) {
	withSnapshotClock(t, fixedTime())
	reg := populatedRegistry(t)

	snap, err := CreateSnapshot(reg, "initial")
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}
	if snap.Version != "20260418_123045" {
		t.Errorf("Version = %q, want 20260418_123045", snap.Version)
	}
	if !snap.Timestamp.Equal(fixedTime()) {
		t.Errorf("Timestamp = %v, want %v", snap.Timestamp, fixedTime())
	}
	if snap.Description != "initial" {
		t.Errorf("Description = %q, want %q", snap.Description, "initial")
	}
}

func TestCreateSnapshotCopiesRegistryContents(t *testing.T) {
	withSnapshotClock(t, fixedTime())
	reg := populatedRegistry(t)

	snap, err := CreateSnapshot(reg, "")
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}
	if len(snap.Tables) != 1 || snap.Tables[0].Name != "user" {
		t.Errorf("Tables = %v, want [user]", snap.Tables)
	}
	if len(snap.Edges) != 1 || snap.Edges[0].Name != "likes" {
		t.Errorf("Edges = %v, want [likes]", snap.Edges)
	}
}

func TestCreateSnapshotAllowsEmptyDescription(t *testing.T) {
	withSnapshotClock(t, fixedTime())
	snap, err := CreateSnapshot(schema.NewSchemaRegistry(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Description != "" {
		t.Errorf("Description = %q, want empty", snap.Description)
	}
}

func TestCreateSnapshotRejectsNilRegistry(t *testing.T) {
	_, err := CreateSnapshot(nil, "nope")
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestCreateSnapshotIsIndependentOfLaterRegistryMutation(t *testing.T) {
	withSnapshotClock(t, fixedTime())
	reg := populatedRegistry(t)
	snap, err := CreateSnapshot(reg, "")
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}

	// Mutating the registry after snapshot must not alter the snapshot.
	if err := reg.RegisterTable(schema.NewTable("added_after")); err != nil {
		t.Fatalf("failed to register later table: %v", err)
	}
	if len(snap.Tables) != 1 {
		t.Errorf("snapshot Tables len = %d, want 1 after registry mutation",
			len(snap.Tables))
	}
}

func TestStoreAndLoadSnapshotRoundTrip(t *testing.T) {
	withSnapshotClock(t, fixedTime())
	reg := populatedRegistry(t)
	snap, err := CreateSnapshot(reg, "initial schema")
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}

	dir := t.TempDir()
	if err := StoreSnapshot(snap, dir); err != nil {
		t.Fatalf("StoreSnapshot returned error: %v", err)
	}

	expectedPath := filepath.Join(dir, snap.Version+snapshotFileSuffix)
	loaded, err := LoadSnapshot(expectedPath)
	if err != nil {
		t.Fatalf("LoadSnapshot returned error: %v", err)
	}

	if loaded.Version != snap.Version {
		t.Errorf("Version mismatch: got %q, want %q", loaded.Version, snap.Version)
	}
	if loaded.Description != snap.Description {
		t.Errorf("Description mismatch: got %q, want %q",
			loaded.Description, snap.Description)
	}
	if len(loaded.Tables) != len(snap.Tables) {
		t.Errorf("Tables len: got %d, want %d",
			len(loaded.Tables), len(snap.Tables))
	}
	if len(loaded.Edges) != len(snap.Edges) {
		t.Errorf("Edges len: got %d, want %d",
			len(loaded.Edges), len(snap.Edges))
	}
	if !loaded.Timestamp.Equal(snap.Timestamp) {
		t.Errorf("Timestamp mismatch: got %v, want %v",
			loaded.Timestamp, snap.Timestamp)
	}
}

func TestStoreSnapshotCreatesParentDirectory(t *testing.T) {
	withSnapshotClock(t, fixedTime())
	snap, err := CreateSnapshot(schema.NewSchemaRegistry(), "x")
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}

	base := t.TempDir()
	nested := filepath.Join(base, "sub", "dir")
	if err := StoreSnapshot(snap, nested); err != nil {
		t.Fatalf("StoreSnapshot returned error: %v", err)
	}
	// Directory should exist now.
	info, err := os.Stat(nested)
	if err != nil {
		t.Fatalf("expected nested dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected nested path to be a directory")
	}
}

func TestStoreSnapshotAcceptsExplicitFilePath(t *testing.T) {
	withSnapshotClock(t, fixedTime())
	snap, err := CreateSnapshot(schema.NewSchemaRegistry(), "x")
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}
	target := filepath.Join(t.TempDir(), "explicit"+snapshotFileSuffix)
	if err := StoreSnapshot(snap, target); err != nil {
		t.Fatalf("StoreSnapshot returned error: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected file at %q: %v", target, err)
	}
}

func TestStoreSnapshotRejectsEmptyVersion(t *testing.T) {
	err := StoreSnapshot(SchemaSnapshot{}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for empty version")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestStoreSnapshotRejectsEmptyPath(t *testing.T) {
	err := StoreSnapshot(SchemaSnapshot{Version: "v1"}, "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestLoadSnapshotMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist"+snapshotFileSuffix)
	_, err := LoadSnapshot(missing)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, surqlerrors.ErrMigrationLoad) {
		t.Errorf("expected ErrMigrationLoad, got %v", err)
	}
}

func TestLoadSnapshotMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad"+snapshotFileSuffix)
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_, err := LoadSnapshot(path)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !errors.Is(err, surqlerrors.ErrSerialization) {
		t.Errorf("expected ErrSerialization, got %v", err)
	}
}

func TestLoadSnapshotRejectsEmptyVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "noversion"+snapshotFileSuffix)
	payload, _ := json.Marshal(map[string]any{"tables": []any{}})
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_, err := LoadSnapshot(path)
	if err == nil {
		t.Fatal("expected error for empty version")
	}
	if !errors.Is(err, surqlerrors.ErrSerialization) {
		t.Errorf("expected ErrSerialization, got %v", err)
	}
}

func TestLoadSnapshotRejectsEmptyPath(t *testing.T) {
	_, err := LoadSnapshot("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestListSnapshotsReturnsSortedByVersion(t *testing.T) {
	dir := t.TempDir()

	versions := []string{"20260103_000000", "20260101_000000", "20260102_000000"}
	for _, v := range versions {
		snap := SchemaSnapshot{Version: v, Timestamp: time.Now().UTC()}
		if err := StoreSnapshot(snap, dir); err != nil {
			t.Fatalf("StoreSnapshot failed for %s: %v", v, err)
		}
	}

	got, err := ListSnapshots(dir)
	if err != nil {
		t.Fatalf("ListSnapshots returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(got))
	}
	sorted := sort.SliceIsSorted(got, func(i, j int) bool {
		return got[i].Version < got[j].Version
	})
	if !sorted {
		t.Errorf("expected sorted by version, got %v",
			versionsOf(got))
	}
	want := []string{"20260101_000000", "20260102_000000", "20260103_000000"}
	for i, v := range want {
		if got[i].Version != v {
			t.Errorf("index %d: got %q, want %q", i, got[i].Version, v)
		}
	}
}

func TestListSnapshotsIgnoresNonSnapshotFiles(t *testing.T) {
	dir := t.TempDir()

	// Create one real snapshot and two decoys.
	snap := SchemaSnapshot{Version: "v1", Timestamp: time.Now().UTC()}
	if err := StoreSnapshot(snap, dir); err != nil {
		t.Fatalf("StoreSnapshot failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"),
		[]byte("nope"), 0o644); err != nil {
		t.Fatalf("write decoy failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.json"),
		[]byte("{}"), 0o644); err != nil {
		t.Fatalf("write decoy failed: %v", err)
	}

	got, err := ListSnapshots(dir)
	if err != nil {
		t.Fatalf("ListSnapshots returned error: %v", err)
	}
	if len(got) != 1 || got[0].Version != "v1" {
		t.Errorf("expected only the single snapshot v1, got %v",
			versionsOf(got))
	}
}

func TestListSnapshotsEmptyDir(t *testing.T) {
	got, err := ListSnapshots(t.TempDir())
	if err != nil {
		t.Fatalf("ListSnapshots returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(got))
	}
}

func TestListSnapshotsMissingDir(t *testing.T) {
	_, err := ListSnapshots(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
	if !errors.Is(err, surqlerrors.ErrMigrationDiscovery) {
		t.Errorf("expected ErrMigrationDiscovery, got %v", err)
	}
}

func TestListSnapshotsEmptyDirPath(t *testing.T) {
	_, err := ListSnapshots("")
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestListSnapshotsPropagatesCorruption(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad"+snapshotFileSuffix),
		[]byte("totally invalid"), 0o644); err != nil {
		t.Fatalf("write corrupt snapshot: %v", err)
	}
	_, err := ListSnapshots(dir)
	if err == nil {
		t.Fatal("expected corruption to be surfaced")
	}
	if !errors.Is(err, surqlerrors.ErrSerialization) {
		t.Errorf("expected ErrSerialization, got %v", err)
	}
}

func TestCompareSnapshotsIdentity(t *testing.T) {
	reg := populatedRegistry(t)
	withSnapshotClock(t, fixedTime())
	snap, err := CreateSnapshot(reg, "")
	if err != nil {
		t.Fatalf("CreateSnapshot returned error: %v", err)
	}

	diffs, err := CompareSnapshots(snap, snap)
	if err != nil {
		t.Fatalf("CompareSnapshots returned error: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("expected no diffs for identical snapshots, got %v", diffs)
	}
}

func TestCompareSnapshotsAddTable(t *testing.T) {
	from := SchemaSnapshot{Version: "v0"}
	to := SchemaSnapshot{
		Version: "v1",
		Tables:  []schema.TableDefinition{schema.NewTable("user")},
	}

	diffs, err := CompareSnapshots(from, to)
	if err != nil {
		t.Fatalf("CompareSnapshots returned error: %v", err)
	}
	if len(diffs) == 0 {
		t.Fatal("expected at least one diff")
	}
	if diffs[0].Operation != DiffOperationAddTable ||
		diffs[0].Table != "user" {
		t.Errorf("first diff = %+v, want add_table on user", diffs[0])
	}
}

func TestCompareSnapshotsDropTable(t *testing.T) {
	from := SchemaSnapshot{
		Version: "v0",
		Tables:  []schema.TableDefinition{schema.NewTable("legacy")},
	}
	to := SchemaSnapshot{Version: "v1"}

	diffs, err := CompareSnapshots(from, to)
	if err != nil {
		t.Fatalf("CompareSnapshots returned error: %v", err)
	}
	found := false
	for _, d := range diffs {
		if d.Operation == DiffOperationDropTable && d.Table == "legacy" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected drop_table diff for legacy, got %v", diffs)
	}
}

func TestCompareSnapshotsEndToEnd(t *testing.T) {
	// Round-trip via disk, then compare.
	dir := t.TempDir()

	reg1 := schema.NewSchemaRegistry()
	_ = reg1.RegisterTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email"))))
	withSnapshotClock(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	snap1, err := CreateSnapshot(reg1, "v1")
	if err != nil {
		t.Fatalf("CreateSnapshot v1: %v", err)
	}
	if err := StoreSnapshot(snap1, dir); err != nil {
		t.Fatalf("StoreSnapshot v1: %v", err)
	}

	reg2 := schema.NewSchemaRegistry()
	_ = reg2.RegisterTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email"), schema.StringField("name"))))
	_ = reg2.RegisterTable(schema.NewTable("post"))
	withSnapshotClock(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	snap2, err := CreateSnapshot(reg2, "v2")
	if err != nil {
		t.Fatalf("CreateSnapshot v2: %v", err)
	}
	if err := StoreSnapshot(snap2, dir); err != nil {
		t.Fatalf("StoreSnapshot v2: %v", err)
	}

	snapshots, err := ListSnapshots(dir)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	diffs, err := CompareSnapshots(snapshots[0], snapshots[1])
	if err != nil {
		t.Fatalf("CompareSnapshots: %v", err)
	}

	ops := map[DiffOperation]int{}
	for _, d := range diffs {
		ops[d.Operation]++
	}
	if ops[DiffOperationAddTable] != 1 {
		t.Errorf("expected 1 add_table (post), got %d", ops[DiffOperationAddTable])
	}
	if ops[DiffOperationAddField] != 1 {
		t.Errorf("expected 1 add_field (user.name), got %d", ops[DiffOperationAddField])
	}
}

// --- VersionGraph tests ---

func TestVersionGraphAddRoot(t *testing.T) {
	g := NewVersionGraph()
	if err := g.Add(SchemaSnapshot{Version: "v1"}, ""); err != nil {
		t.Fatalf("Add root: %v", err)
	}
	if g.Root() != "v1" {
		t.Errorf("Root = %q, want v1", g.Root())
	}
	if g.Len() != 1 {
		t.Errorf("Len = %d, want 1", g.Len())
	}
}

func TestVersionGraphAddWithParent(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	if err := g.Add(SchemaSnapshot{Version: "v2"}, "v1"); err != nil {
		t.Fatalf("Add child: %v", err)
	}
	node, ok := g.Get("v1")
	if !ok {
		t.Fatal("v1 missing")
	}
	if len(node.Children) != 1 || node.Children[0] != "v2" {
		t.Errorf("v1 children = %v, want [v2]", node.Children)
	}
	child, ok := g.Get("v2")
	if !ok || child.Parent != "v1" {
		t.Errorf("v2 parent = %q, want v1", child.Parent)
	}
}

func TestVersionGraphAddDuplicateVersion(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	err := g.Add(SchemaSnapshot{Version: "v1"}, "")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestVersionGraphAddUnknownParent(t *testing.T) {
	g := NewVersionGraph()
	err := g.Add(SchemaSnapshot{Version: "v2"}, "nonexistent")
	if err == nil {
		t.Fatal("expected unknown-parent error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestVersionGraphAddEmptyVersion(t *testing.T) {
	g := NewVersionGraph()
	err := g.Add(SchemaSnapshot{}, "")
	if err == nil {
		t.Fatal("expected empty-version error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestVersionGraphLookup(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")

	if !g.Has("v1") {
		t.Error("Has(v1) = false")
	}
	if g.Has("missing") {
		t.Error("Has(missing) = true")
	}
	if _, ok := g.Get("missing"); ok {
		t.Error("Get(missing) returned ok")
	}
}

func TestVersionGraphVersionsSorted(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v3"}, "")
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "v3")
	_ = g.Add(SchemaSnapshot{Version: "v2"}, "v3")

	got := g.Versions()
	want := []string{"v1", "v2", "v3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Versions = %v, want %v", got, want)
	}
}

func TestVersionGraphPathLinear(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	_ = g.Add(SchemaSnapshot{Version: "v2"}, "v1")
	_ = g.Add(SchemaSnapshot{Version: "v3"}, "v2")

	path := g.Path("v1", "v3")
	want := []string{"v1", "v2", "v3"}
	if strings.Join(path, ",") != strings.Join(want, ",") {
		t.Errorf("Path(v1, v3) = %v, want %v", path, want)
	}
}

func TestVersionGraphPathReverse(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	_ = g.Add(SchemaSnapshot{Version: "v2"}, "v1")

	path := g.Path("v2", "v1")
	want := []string{"v2", "v1"}
	if strings.Join(path, ",") != strings.Join(want, ",") {
		t.Errorf("Path(v2, v1) = %v, want %v", path, want)
	}
}

func TestVersionGraphPathSameNode(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")

	path := g.Path("v1", "v1")
	if len(path) != 1 || path[0] != "v1" {
		t.Errorf("Path(v1, v1) = %v, want [v1]", path)
	}
}

func TestVersionGraphPathUnknown(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	if path := g.Path("v1", "unknown"); path != nil {
		t.Errorf("expected nil path for unknown endpoint, got %v", path)
	}
}

func TestVersionGraphAncestors(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	_ = g.Add(SchemaSnapshot{Version: "v2"}, "v1")
	_ = g.Add(SchemaSnapshot{Version: "v3"}, "v2")

	got := g.Ancestors("v3")
	want := []string{"v1", "v2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Ancestors(v3) = %v, want %v", got, want)
	}
}

func TestVersionGraphAncestorsRootHasNone(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	if got := g.Ancestors("v1"); len(got) != 0 {
		t.Errorf("Ancestors(root) = %v, want empty", got)
	}
}

func TestVersionGraphAncestorsUnknown(t *testing.T) {
	g := NewVersionGraph()
	if got := g.Ancestors("nope"); got != nil {
		t.Errorf("Ancestors(unknown) = %v, want nil", got)
	}
}

func TestVersionGraphDescendants(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	_ = g.Add(SchemaSnapshot{Version: "v2a"}, "v1")
	_ = g.Add(SchemaSnapshot{Version: "v2b"}, "v1")
	_ = g.Add(SchemaSnapshot{Version: "v3"}, "v2a")

	got := g.Descendants("v1")
	sort.Strings(got)
	want := []string{"v2a", "v2b", "v3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Descendants(v1) = %v, want %v", got, want)
	}
}

func TestVersionGraphDescendantsLeaf(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	if got := g.Descendants("v1"); len(got) != 0 {
		t.Errorf("Descendants(leaf) = %v, want empty", got)
	}
}

func TestVersionGraphDescendantsUnknown(t *testing.T) {
	g := NewVersionGraph()
	if got := g.Descendants("nope"); got != nil {
		t.Errorf("Descendants(unknown) = %v, want nil", got)
	}
}

func TestVersionGraphRemoveLeaf(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	_ = g.Add(SchemaSnapshot{Version: "v2"}, "v1")

	if err := g.Remove("v2"); err != nil {
		t.Fatalf("Remove leaf: %v", err)
	}
	if g.Has("v2") {
		t.Error("v2 should be removed")
	}
	parent, _ := g.Get("v1")
	if len(parent.Children) != 0 {
		t.Errorf("v1 Children = %v, want []", parent.Children)
	}
}

func TestVersionGraphRemoveMiddleRewiresChildren(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	_ = g.Add(SchemaSnapshot{Version: "v2"}, "v1")
	_ = g.Add(SchemaSnapshot{Version: "v3"}, "v2")

	if err := g.Remove("v2"); err != nil {
		t.Fatalf("Remove middle: %v", err)
	}
	child, _ := g.Get("v3")
	if child.Parent != "v1" {
		t.Errorf("v3 parent after remove = %q, want v1", child.Parent)
	}
	parent, _ := g.Get("v1")
	if len(parent.Children) != 1 || parent.Children[0] != "v3" {
		t.Errorf("v1 Children = %v, want [v3]", parent.Children)
	}
}

func TestVersionGraphRemoveRootPromotesChild(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	_ = g.Add(SchemaSnapshot{Version: "v2"}, "v1")

	if err := g.Remove("v1"); err != nil {
		t.Fatalf("Remove root: %v", err)
	}
	child, _ := g.Get("v2")
	if child.Parent != "" {
		t.Errorf("v2 parent after root removal = %q, want empty", child.Parent)
	}
	if g.Root() != "v2" {
		t.Errorf("Root after removal = %q, want v2", g.Root())
	}
}

func TestVersionGraphRemoveUnknown(t *testing.T) {
	g := NewVersionGraph()
	err := g.Remove("nope")
	if err == nil {
		t.Fatal("expected error for unknown version")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestVersionGraphRootEmpty(t *testing.T) {
	g := NewVersionGraph()
	if g.Root() != "" {
		t.Errorf("empty graph Root = %q, want empty", g.Root())
	}
	if g.Len() != 0 {
		t.Errorf("empty Len = %d, want 0", g.Len())
	}
}

func TestVersionGraphGetReturnsCopy(t *testing.T) {
	g := NewVersionGraph()
	_ = g.Add(SchemaSnapshot{Version: "v1"}, "")
	_ = g.Add(SchemaSnapshot{Version: "v2"}, "v1")

	node, _ := g.Get("v1")
	node.Children[0] = "mutated"

	original, _ := g.Get("v1")
	if original.Children[0] != "v2" {
		t.Errorf("graph was mutated via returned node: Children[0] = %q",
			original.Children[0])
	}
}

// versionsOf is a small debug helper pulling the version strings from a list
// of snapshots.
func versionsOf(snaps []SchemaSnapshot) []string {
	out := make([]string, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, s.Version)
	}
	return out
}
