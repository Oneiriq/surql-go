package migration

import (
	"context"
	stdErrors "errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
	"github.com/Oneiriq/surql-go/pkg/surql/schema"
)

// resetAutoSnapshot makes the global flag test-local so tests don't bleed
// into one another.
func resetAutoSnapshot(t *testing.T) {
	t.Helper()
	previous := IsAutoSnapshotEnabled()
	t.Cleanup(func() {
		if previous {
			EnableAutoSnapshots()
		} else {
			DisableAutoSnapshots()
		}
	})
}

func TestAutoSnapshotToggle(t *testing.T) {
	resetAutoSnapshot(t)

	DisableAutoSnapshots()
	if IsAutoSnapshotEnabled() {
		t.Error("disabled should read false")
	}

	EnableAutoSnapshots()
	if !IsAutoSnapshotEnabled() {
		t.Error("enabled should read true")
	}

	DisableAutoSnapshots()
	if IsAutoSnapshotEnabled() {
		t.Error("re-disabled should read false")
	}
}

func TestCreateSnapshotOnMigration_NoopWhenDisabled(t *testing.T) {
	resetAutoSnapshot(t)
	DisableAutoSnapshots()

	dir := t.TempDir()
	reg := schema.NewSchemaRegistry()
	path, err := CreateSnapshotOnMigration(
		context.Background(), dir,
		Migration{Version: "20260101_000000", Description: "noop"},
		SnapshotHook{Registry: reg},
	)
	if err != nil {
		t.Fatalf("disabled CreateSnapshotOnMigration: %v", err)
	}
	if path != "" {
		t.Errorf("disabled path = %q, want empty", path)
	}
	// No files should have been written.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("unexpected files in snapshot dir: %+v", entries)
	}
}

func TestCreateSnapshotOnMigration_WritesSnapshot(t *testing.T) {
	resetAutoSnapshot(t)
	EnableAutoSnapshots()

	// Fixed clock so the snapshot version is predictable.
	fixed := time.Date(2026, 4, 18, 12, 30, 45, 0, time.UTC)
	orig := snapshotClock
	snapshotClock = func() time.Time { return fixed }
	t.Cleanup(func() { snapshotClock = orig })

	dir := t.TempDir()
	reg := schema.NewSchemaRegistry()
	if err := reg.RegisterTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email")),
	)); err != nil {
		t.Fatalf("RegisterTable: %v", err)
	}

	path, err := CreateSnapshotOnMigration(
		context.Background(), dir,
		Migration{Version: "20260101_000000", Description: "initial schema"},
		SnapshotHook{Registry: reg},
	)
	if err != nil {
		t.Fatalf("CreateSnapshotOnMigration: %v", err)
	}
	if path == "" {
		t.Fatal("path must not be empty on success")
	}
	if !strings.HasSuffix(path, "20260418_123045.snapshot.json") {
		t.Errorf("unexpected snapshot filename: %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("snapshot file missing: %v", err)
	}

	// The snapshot should round-trip cleanly.
	snap, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if snap.Version != "20260418_123045" {
		t.Errorf("snapshot version = %q", snap.Version)
	}
	if snap.Description != "initial schema" {
		t.Errorf("snapshot description = %q", snap.Description)
	}
	if len(snap.Tables) != 1 {
		t.Errorf("tables = %v, want 1", snap.Tables)
	}
}

func TestCreateSnapshotOnMigration_RunsBeforeAndAfterHooks(t *testing.T) {
	resetAutoSnapshot(t)
	EnableAutoSnapshots()

	dir := t.TempDir()
	reg := schema.NewSchemaRegistry()
	if err := reg.RegisterTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email")),
	)); err != nil {
		t.Fatalf("RegisterTable: %v", err)
	}

	var (
		beforeCalls, afterCalls int
		observedPath            string
		observedVersion         string
		mu                      sync.Mutex
	)
	hook := SnapshotHook{
		Registry: reg,
		Before: func(ctx context.Context, m Migration) error {
			mu.Lock()
			defer mu.Unlock()
			beforeCalls++
			observedVersion = m.Version
			return nil
		},
		After: func(ctx context.Context, m Migration, snap SchemaSnapshot, p string) error {
			mu.Lock()
			defer mu.Unlock()
			afterCalls++
			observedPath = p
			return nil
		},
	}

	path, err := CreateSnapshotOnMigration(
		context.Background(), dir,
		Migration{Version: "20260101_000000", Description: "x"},
		hook,
	)
	if err != nil {
		t.Fatalf("CreateSnapshotOnMigration: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if beforeCalls != 1 {
		t.Errorf("beforeCalls = %d, want 1", beforeCalls)
	}
	if afterCalls != 1 {
		t.Errorf("afterCalls = %d, want 1", afterCalls)
	}
	if observedVersion != "20260101_000000" {
		t.Errorf("observedVersion = %q", observedVersion)
	}
	if observedPath != path {
		t.Errorf("observedPath = %q, want %q", observedPath, path)
	}
}

func TestCreateSnapshotOnMigration_AbortOnBeforeError(t *testing.T) {
	resetAutoSnapshot(t)
	EnableAutoSnapshots()

	dir := t.TempDir()
	reg := schema.NewSchemaRegistry()
	sentinel := stdErrors.New("veto")

	_, err := CreateSnapshotOnMigration(
		context.Background(), dir,
		Migration{Version: "v1"},
		SnapshotHook{
			Registry: reg,
			Before: func(ctx context.Context, m Migration) error {
				return sentinel
			},
		},
	)
	if err == nil {
		t.Fatal("expected error from before hook")
	}
	if !stdErrors.Is(err, sentinel) {
		t.Errorf("err does not wrap sentinel: %v", err)
	}
	// No files should have been written.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("snapshot dir should be empty, got %+v", entries)
	}
}

func TestCreateSnapshotOnMigration_AfterErrorStillReturnsPath(t *testing.T) {
	resetAutoSnapshot(t)
	EnableAutoSnapshots()

	dir := t.TempDir()
	reg := schema.NewSchemaRegistry()
	if err := reg.RegisterTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email")),
	)); err != nil {
		t.Fatalf("RegisterTable: %v", err)
	}

	sentinel := stdErrors.New("after-boom")
	path, err := CreateSnapshotOnMigration(
		context.Background(), dir,
		Migration{Version: "v1", Description: "d"},
		SnapshotHook{
			Registry: reg,
			After: func(ctx context.Context, m Migration, s SchemaSnapshot, p string) error {
				return sentinel
			},
		},
	)
	if err == nil {
		t.Fatal("expected error from after hook")
	}
	if !stdErrors.Is(err, sentinel) {
		t.Errorf("err does not wrap sentinel: %v", err)
	}
	if path == "" {
		t.Error("path should be returned even when after hook fails")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("snapshot file missing after after-hook error: %v", statErr)
	}
}

func TestCreateSnapshotOnMigration_ValidatesInputs(t *testing.T) {
	resetAutoSnapshot(t)
	EnableAutoSnapshots()

	reg := schema.NewSchemaRegistry()

	cases := []struct {
		name string
		dir  string
		hook SnapshotHook
	}{
		{"empty dir", "", SnapshotHook{Registry: reg}},
		{"nil registry", t.TempDir(), SnapshotHook{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateSnapshotOnMigration(
				context.Background(), tc.dir,
				Migration{Version: "v1"}, tc.hook,
			)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !stdErrors.Is(err, surqlerrors.ErrValidation) {
				t.Errorf("err not ErrValidation: %v", err)
			}
		})
	}
}

func TestCreateSnapshotOnMigration_CancelledContext(t *testing.T) {
	resetAutoSnapshot(t)
	EnableAutoSnapshots()

	reg := schema.NewSchemaRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := CreateSnapshotOnMigration(
		ctx, t.TempDir(),
		Migration{Version: "v1"},
		SnapshotHook{Registry: reg},
	)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrMigrationHistory) {
		t.Errorf("err not ErrMigrationHistory: %v", err)
	}
}

func TestCreateSnapshotOnMigration_UsesDefaultDescription(t *testing.T) {
	resetAutoSnapshot(t)
	EnableAutoSnapshots()

	dir := t.TempDir()
	reg := schema.NewSchemaRegistry()
	if err := reg.RegisterTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email")),
	)); err != nil {
		t.Fatalf("RegisterTable: %v", err)
	}

	path, err := CreateSnapshotOnMigration(
		context.Background(), dir,
		Migration{Version: "20260101_000000"}, // no Description
		SnapshotHook{Registry: reg},
	)
	if err != nil {
		t.Fatalf("CreateSnapshotOnMigration: %v", err)
	}

	snap, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if !strings.Contains(snap.Description, "auto-snapshot after 20260101_000000") {
		t.Errorf("default description missing: %q", snap.Description)
	}
}

func TestCreateSnapshotOnMigration_ZeroValueHook(t *testing.T) {
	resetAutoSnapshot(t)
	EnableAutoSnapshots()

	// Zero value hook (Registry nil) must be rejected with ErrValidation.
	_, err := CreateSnapshotOnMigration(
		context.Background(), t.TempDir(),
		Migration{Version: "v1"}, SnapshotHook{},
	)
	if err == nil {
		t.Fatal("expected error for zero-value hook")
	}
}

func TestCreateSnapshotOnMigration_CreatesParentDirectory(t *testing.T) {
	resetAutoSnapshot(t)
	EnableAutoSnapshots()

	base := t.TempDir()
	nested := filepath.Join(base, "nested", "snaps")

	reg := schema.NewSchemaRegistry()
	if err := reg.RegisterTable(schema.NewTable("user",
		schema.WithFields(schema.StringField("email")),
	)); err != nil {
		t.Fatalf("RegisterTable: %v", err)
	}

	path, err := CreateSnapshotOnMigration(
		context.Background(), nested,
		Migration{Version: "v1", Description: "d"},
		SnapshotHook{Registry: reg},
	)
	if err != nil {
		t.Fatalf("CreateSnapshotOnMigration: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("snapshot missing: %v", statErr)
	}
}
