package migration

import (
	"context"
	stdErrors "errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// collectReports drains reports from ch until ctx is done or until count
// reports have been received.
func collectReports(ctx context.Context, ch <-chan DriftReport, count int) []DriftReport {
	out := make([]DriftReport, 0, count)
	for len(out) < count {
		select {
		case r, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, r)
		case <-ctx.Done():
			return out
		}
	}
	return out
}

func TestDefaultSchemaFileFilter(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"schema.surql", true},
		{"model.go", true},
		{"model_test.go", false},
		{"README.md", false},
		{"subdir/tables.surql", true},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := DefaultSchemaFileFilter(tc.path); got != tc.want {
				t.Errorf("DefaultSchemaFileFilter(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestNewSchemaWatcher_ValidatesInputs(t *testing.T) {
	_, err := NewSchemaWatcher("", noopChecker, WatcherOptions{})
	if err == nil {
		t.Error("expected error for empty dir")
	}

	dir := t.TempDir()
	_, err = NewSchemaWatcher(dir, nil, WatcherOptions{})
	if err == nil {
		t.Error("expected error for nil checker")
	}

	// File path, not dir.
	file := filepath.Join(dir, "a.surql")
	if err := os.WriteFile(file, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err = NewSchemaWatcher(file, noopChecker, WatcherOptions{})
	if err == nil {
		t.Error("expected error for non-directory path")
	}
}

func TestNewSchemaWatcher_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSchemaWatcher(dir, noopChecker, WatcherOptions{})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	if w.opts.Debounce != DefaultWatcherDebounce {
		t.Errorf("Debounce = %v, want %v", w.opts.Debounce, DefaultWatcherDebounce)
	}
	if !w.opts.Recursive {
		t.Error("Recursive default should be true")
	}
	if w.opts.Filter == nil {
		t.Error("Filter default should be non-nil")
	}
	if w.opts.ReportBufferSize != 8 {
		t.Errorf("ReportBufferSize default = %d, want 8", w.opts.ReportBufferSize)
	}
}

func TestNewSchemaWatcher_RespectsExplicitRecursiveFalse(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSchemaWatcher(dir, noopChecker, WatcherOptions{
		RecursiveSet: true,
		Recursive:    false,
	})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	if w.opts.Recursive {
		t.Error("explicit Recursive=false should be honoured")
	}
}

func TestSchemaWatcher_StartTwiceIsError(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSchemaWatcher(dir, noopChecker, WatcherOptions{
		Debounce: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	if err := w.Start(ctx); err == nil {
		t.Error("expected second Start to fail")
	}
}

func TestSchemaWatcher_EmitsDriftReport(t *testing.T) {
	dir := t.TempDir()

	var invocations int32
	checker := func(ctx context.Context, events []SchemaChangeEvent) (*DriftReport, error) {
		atomic.AddInt32(&invocations, 1)
		return &DriftReport{
			DriftDetected: true,
			Issues: []DriftIssue{
				{
					Severity:    DriftSeverityInfo,
					Operation:   DiffOperationAddTable,
					Table:       "user",
					Description: "user table appeared",
				},
			},
		}, nil
	}

	w, err := NewSchemaWatcher(dir, checker, WatcherOptions{
		Debounce: 80 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	// Create a schema file; this should produce one debounced event.
	time.Sleep(50 * time.Millisecond)
	path := filepath.Join(dir, "user.surql")
	if err := os.WriteFile(path, []byte("DEFINE TABLE user SCHEMAFULL;\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer waitCancel()
	reports := collectReports(waitCtx, w.Reports(), 1)
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if !reports[0].DriftDetected {
		t.Error("DriftDetected = false, want true")
	}
	if atomic.LoadInt32(&invocations) < 1 {
		t.Error("checker was not invoked")
	}
}

func TestSchemaWatcher_DebouncesBursts(t *testing.T) {
	dir := t.TempDir()

	var batchSizes []int
	var mu sync.Mutex
	checker := func(ctx context.Context, events []SchemaChangeEvent) (*DriftReport, error) {
		mu.Lock()
		batchSizes = append(batchSizes, len(events))
		mu.Unlock()
		return &DriftReport{DriftDetected: false}, nil
	}

	w, err := NewSchemaWatcher(dir, checker, WatcherOptions{
		Debounce: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	time.Sleep(30 * time.Millisecond)
	// Three rapid writes within the debounce window.
	for i, name := range []string{"a.surql", "b.surql", "c.surql"} {
		_ = i
		if err := os.WriteFile(filepath.Join(dir, name), []byte("."), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer waitCancel()
	reports := collectReports(waitCtx, w.Reports(), 1)
	if len(reports) == 0 {
		t.Fatalf("expected at least 1 report")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(batchSizes) == 0 {
		t.Fatal("checker never called")
	}
	// Expect the first flush to batch multiple events.
	if batchSizes[0] < 2 {
		t.Errorf("first batch size = %d, want >=2 (debouncing failed)", batchSizes[0])
	}
}

func TestSchemaWatcher_FiltersIgnoredFiles(t *testing.T) {
	dir := t.TempDir()
	var invocations int32
	checker := func(ctx context.Context, events []SchemaChangeEvent) (*DriftReport, error) {
		atomic.AddInt32(&invocations, 1)
		return nil, nil
	}

	// Custom filter only accepts files ending in .surql, not .md.
	w, err := NewSchemaWatcher(dir, checker, WatcherOptions{
		Debounce: 60 * time.Millisecond,
		Filter:   DefaultSchemaFileFilter,
	})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if got := atomic.LoadInt32(&invocations); got != 0 {
		t.Errorf("checker invoked %d times, want 0", got)
	}
}

func TestSchemaWatcher_StopClosesReports(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSchemaWatcher(dir, noopChecker, WatcherOptions{
		Debounce: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := w.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Reports must close within a reasonable time.
	select {
	case _, ok := <-w.Reports():
		if ok {
			// Drain a final report if present, then expect close.
			select {
			case _, ok := <-w.Reports():
				if ok {
					t.Error("Reports channel did not close")
				}
			case <-time.After(2 * time.Second):
				t.Error("Reports channel did not close after stop")
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("Reports channel did not close after stop")
	}
}

func TestSchemaWatcher_DedupesSamePath(t *testing.T) {
	dir := t.TempDir()

	var lastBatch []SchemaChangeEvent
	var mu sync.Mutex
	checker := func(ctx context.Context, events []SchemaChangeEvent) (*DriftReport, error) {
		mu.Lock()
		lastBatch = append([]SchemaChangeEvent(nil), events...)
		mu.Unlock()
		return &DriftReport{DriftDetected: false}, nil
	}

	w, err := NewSchemaWatcher(dir, checker, WatcherOptions{
		Debounce: 120 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	time.Sleep(30 * time.Millisecond)
	path := filepath.Join(dir, "user.surql")
	// Multiple writes to the same path collapse to a single entry.
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(path, []byte{byte('a' + i)}, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer waitCancel()
	_ = collectReports(waitCtx, w.Reports(), 1)

	mu.Lock()
	defer mu.Unlock()
	if len(lastBatch) != 1 {
		t.Errorf("dedupe failed: batch had %d events (want 1): %+v", len(lastBatch), lastBatch)
	}
}

func TestSchemaWatcher_NilReportNotSent(t *testing.T) {
	dir := t.TempDir()
	checker := func(ctx context.Context, events []SchemaChangeEvent) (*DriftReport, error) {
		// Returning nil, nil means "no report needed".
		return nil, nil
	}

	w, err := NewSchemaWatcher(dir, checker, WatcherOptions{
		Debounce: 60 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "user.surql"), []byte("."), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	time.Sleep(250 * time.Millisecond)

	select {
	case r, ok := <-w.Reports():
		if ok {
			t.Errorf("unexpected report received: %+v", r)
		}
	default:
		// good: no report
	}
}

func TestSchemaWatcher_CheckerErrorReportedViaCallback(t *testing.T) {
	dir := t.TempDir()
	sentinelErr := stdErrors.New("checker boom")

	var observed atomic.Value
	opts := WatcherOptions{
		Debounce: 60 * time.Millisecond,
		OnError: func(err error) {
			observed.Store(err)
		},
	}
	w, err := NewSchemaWatcher(dir, func(ctx context.Context, events []SchemaChangeEvent) (*DriftReport, error) {
		return nil, sentinelErr
	}, opts)
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "user.surql"), []byte("."), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		if v := observed.Load(); v != nil {
			if !stdErrors.Is(v.(error), sentinelErr) {
				t.Errorf("observed error does not wrap sentinel: %v", v)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("OnError never fired")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestSchemaWatcher_CtxCancelShutsDown(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSchemaWatcher(dir, noopChecker, WatcherOptions{
		Debounce: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSchemaWatcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	cancel()
	// Give the goroutines a moment to observe the cancellation.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-w.Reports():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("watcher did not shut down after ctx cancel")
		}
	}
}

func TestClassifyEvent(t *testing.T) {
	cases := []struct {
		name string
		op   fsnotify.Op
		want SchemaChangeType
	}{
		{"create", fsnotify.Create, SchemaChangeCreated},
		{"write", fsnotify.Write, SchemaChangeModified},
		{"remove", fsnotify.Remove, SchemaChangeDeleted},
		{"rename", fsnotify.Rename, SchemaChangeRenamed},
		{"remove+write", fsnotify.Remove | fsnotify.Write, SchemaChangeDeleted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyEvent(fsnotify.Event{Op: tc.op})
			if got != tc.want {
				t.Errorf("classifyEvent(%v) = %q, want %q", tc.op, got, tc.want)
			}
		})
	}
}

// noopChecker is a DriftChecker that returns no report and no error.
func noopChecker(ctx context.Context, events []SchemaChangeEvent) (*DriftReport, error) {
	return nil, nil
}
