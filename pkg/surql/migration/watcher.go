package migration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// DefaultWatcherDebounce is the debounce window applied to a burst of
// filesystem events before the watcher invokes its DriftChecker.
const DefaultWatcherDebounce = 500 * time.Millisecond

// SchemaChangeType categorises an observed filesystem event.
type SchemaChangeType string

// SchemaChangeType values.
const (
	SchemaChangeCreated  SchemaChangeType = "created"
	SchemaChangeModified SchemaChangeType = "modified"
	SchemaChangeDeleted  SchemaChangeType = "deleted"
	SchemaChangeRenamed  SchemaChangeType = "renamed"
)

// SchemaChangeEvent is a single filesystem change captured by the watcher.
// Multiple events can collapse into a single DriftReport invocation after
// debouncing.
type SchemaChangeEvent struct {
	Path       string           `json:"path"`
	ChangeType SchemaChangeType `json:"change_type"`
	Timestamp  time.Time        `json:"timestamp"`
}

// DriftChecker produces a DriftReport for the given batch of change
// events. It is called from the watcher's goroutine after the debounce
// window elapses; implementations should be fast or push the slow work
// into their own worker pool.
//
// A nil return with a nil error indicates the checker decided no report
// is warranted; the watcher will not emit anything to its output channel
// in that case.
type DriftChecker func(ctx context.Context, events []SchemaChangeEvent) (*DriftReport, error)

// SchemaFileFilter reports whether path should be considered a schema
// file for watch purposes. By default the watcher uses
// DefaultSchemaFileFilter which accepts any `.surql` file or any `.go`
// file that is not a test file; callers can supply a custom filter to
// narrow or broaden the set.
type SchemaFileFilter func(path string) bool

// WatcherOptions configures a SchemaWatcher. The zero value is valid:
// Debounce defaults to DefaultWatcherDebounce, Filter defaults to
// DefaultSchemaFileFilter, and Recursive defaults to true.
type WatcherOptions struct {
	// Debounce controls how long the watcher waits for a burst of events
	// to settle before invoking the DriftChecker. Zero or negative values
	// are replaced with DefaultWatcherDebounce.
	Debounce time.Duration
	// Recursive, when true, watches subdirectories of Dir. Defaults to true.
	// Callers that want to opt out must explicitly set Recursive=false.
	Recursive bool
	// RecursiveSet tracks whether the caller provided an explicit Recursive
	// value; when false the zero default (Recursive=true) is used.
	RecursiveSet bool
	// Filter narrows the set of paths the watcher reacts to. Nil uses
	// DefaultSchemaFileFilter.
	Filter SchemaFileFilter
	// ReportBufferSize is the size of the output DriftReport channel. A
	// value of 0 applies a sensible default of 8. Callers that consume
	// reports synchronously should leave this at the default.
	ReportBufferSize int
	// OnError, when non-nil, is invoked for each non-fatal watcher error
	// (checker failure, observer failure, filesystem glitch). The
	// callback must not block; the watcher otherwise pauses event
	// dispatch.
	OnError func(error)
}

// SchemaWatcher monitors a schema directory for filesystem changes and
// runs a DriftChecker against the collapsed batch of events after each
// debounce window. It is the Go equivalent of surql-py's SchemaWatcher
// and mirrors its lifecycle (Start / Stop) while surfacing drift
// results over a channel rather than an async callback.
//
// Instances are not safe for concurrent Start/Stop calls; the expected
// use is to create a watcher, Start it once, drain Reports(), then Stop.
type SchemaWatcher struct {
	dir      string
	checker  DriftChecker
	opts     WatcherOptions
	fsw      *fsnotify.Watcher
	reports  chan DriftReport
	done     chan struct{}
	events   chan SchemaChangeEvent
	mu       sync.Mutex
	running  bool
	stopOnce sync.Once
}

// NewSchemaWatcher constructs a SchemaWatcher watching dir using checker
// to compute a DriftReport for each debounced batch. The returned
// watcher is dormant until Start is called.
//
// dir must exist and be a directory. checker must be non-nil.
func NewSchemaWatcher(dir string, checker DriftChecker, opts WatcherOptions) (*SchemaWatcher, error) {
	if checker == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation,
			"DriftChecker must not be nil")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrValidation, err,
			"failed to stat schema directory %q", dir)
	}
	if !info.IsDir() {
		return nil, surqlerrors.Newf(surqlerrors.ErrValidation,
			"schema path %q is not a directory", dir)
	}

	// Normalise options with defaults.
	if opts.Debounce <= 0 {
		opts.Debounce = DefaultWatcherDebounce
	}
	if !opts.RecursiveSet {
		opts.Recursive = true
	}
	if opts.Filter == nil {
		opts.Filter = DefaultSchemaFileFilter
	}
	if opts.ReportBufferSize <= 0 {
		opts.ReportBufferSize = 8
	}

	return &SchemaWatcher{
		dir:     dir,
		checker: checker,
		opts:    opts,
		reports: make(chan DriftReport, opts.ReportBufferSize),
		done:    make(chan struct{}),
		events:  make(chan SchemaChangeEvent, 64),
	}, nil
}

// Reports returns the read end of the DriftReport channel. The channel
// is closed when the watcher's goroutines have fully shut down after a
// Stop call.
func (w *SchemaWatcher) Reports() <-chan DriftReport {
	return w.reports
}

// Start begins the watch loop. The provided ctx cancels the watcher and
// its internal goroutines; a Stop call achieves the same.
//
// Start is idempotent — calling it twice returns ErrValidation rather
// than panicking. The watcher must be fully Stopped before it can be
// restarted; create a new instance for that use case.
func (w *SchemaWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return surqlerrors.New(surqlerrors.ErrValidation,
			"SchemaWatcher is already running")
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		w.mu.Unlock()
		return surqlerrors.Wrap(surqlerrors.ErrValidation,
			"failed to create fsnotify watcher", err)
	}
	w.fsw = fsw
	w.running = true
	w.mu.Unlock()

	// Register directories. When Recursive is true we walk the tree once
	// at startup so newly-created subdirectories below dir are covered.
	if err := w.addRoot(); err != nil {
		_ = fsw.Close()
		w.mu.Lock()
		w.fsw = nil
		w.running = false
		w.mu.Unlock()
		return err
	}

	go w.eventLoop(ctx)
	go w.dispatchLoop(ctx)

	return nil
}

// Stop signals the watcher to shut down and waits until its goroutines
// have exited and the Reports channel is closed. It is safe to call
// Stop from any goroutine and from multiple callers concurrently; only
// the first invocation performs the shutdown.
func (w *SchemaWatcher) Stop() error {
	w.stopOnce.Do(func() {
		close(w.done)
	})

	w.mu.Lock()
	fsw := w.fsw
	w.mu.Unlock()
	if fsw != nil {
		_ = fsw.Close()
	}
	return nil
}

// addRoot registers w.dir with fsnotify, honouring the Recursive option.
func (w *SchemaWatcher) addRoot() error {
	if !w.opts.Recursive {
		if err := w.fsw.Add(w.dir); err != nil {
			return surqlerrors.Wrapf(surqlerrors.ErrValidation, err,
				"failed to watch %q", w.dir)
		}
		return nil
	}

	return filepath.Walk(w.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Surface walk errors via OnError, but keep traversing.
			w.reportError(surqlerrors.Wrapf(
				surqlerrors.ErrValidation, err,
				"walking %q", path))
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if err := w.fsw.Add(path); err != nil {
			w.reportError(surqlerrors.Wrapf(
				surqlerrors.ErrValidation, err,
				"failed to watch directory %q", path))
		}
		return nil
	})
}

// eventLoop translates fsnotify events into SchemaChangeEvents and
// forwards them to the dispatcher. Directory creates trigger an
// incremental Add so new subtrees are covered without re-walking.
func (w *SchemaWatcher) eventLoop(ctx context.Context) {
	defer close(w.events)

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.reportError(surqlerrors.Wrap(
				surqlerrors.ErrValidation,
				"filesystem watcher error", err))
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handleEvent(ev)
		}
	}
}

// handleEvent filters and translates a single fsnotify.Event.
func (w *SchemaWatcher) handleEvent(ev fsnotify.Event) {
	// Attempt to register newly-created directories so we keep covering
	// the subtree. This is best-effort; errors are reported via OnError.
	if w.opts.Recursive && ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if addErr := w.fsw.Add(ev.Name); addErr != nil {
				w.reportError(surqlerrors.Wrapf(
					surqlerrors.ErrValidation, addErr,
					"failed to watch new directory %q", ev.Name))
			}
			return
		}
	}

	if w.opts.Filter != nil && !w.opts.Filter(ev.Name) {
		return
	}

	change := classifyEvent(ev)
	select {
	case w.events <- SchemaChangeEvent{
		Path:       ev.Name,
		ChangeType: change,
		Timestamp:  time.Now().UTC(),
	}:
	default:
		// Buffer full: drop oldest via a best-effort non-blocking receive
		// and retry once. This keeps the watcher responsive under bursty
		// loads without falling back to unbounded memory growth.
		select {
		case <-w.events:
		default:
		}
		select {
		case w.events <- SchemaChangeEvent{
			Path:       ev.Name,
			ChangeType: change,
			Timestamp:  time.Now().UTC(),
		}:
		default:
		}
	}
}

// classifyEvent maps an fsnotify.Op bitmap to a SchemaChangeType. Rename
// is reported as "renamed"; multiple bits favour the more destructive
// interpretation (Remove > Rename > Create > Write).
func classifyEvent(ev fsnotify.Event) SchemaChangeType {
	switch {
	case ev.Has(fsnotify.Remove):
		return SchemaChangeDeleted
	case ev.Has(fsnotify.Rename):
		return SchemaChangeRenamed
	case ev.Has(fsnotify.Create):
		return SchemaChangeCreated
	default:
		return SchemaChangeModified
	}
}

// dispatchLoop collects events into batches separated by the debounce
// window and invokes the DriftChecker once per batch. The resulting
// DriftReport (when non-nil) is written to the Reports channel.
func (w *SchemaWatcher) dispatchLoop(ctx context.Context) {
	defer close(w.reports)

	var (
		batch []SchemaChangeEvent
		// timer is created lazily and always stopped / drained together
		// with batch resets so stale ticks cannot fire on a subsequent
		// batch.
		timer *time.Timer
	)
	dedupe := make(map[string]SchemaChangeEvent)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ordered := make([]SchemaChangeEvent, 0, len(batch))
		seen := make(map[string]struct{}, len(batch))
		// Preserve the insertion order while removing same-path duplicates.
		for i := len(batch) - 1; i >= 0; i-- {
			ev := batch[i]
			if _, ok := seen[ev.Path]; ok {
				continue
			}
			seen[ev.Path] = struct{}{}
			ordered = append([]SchemaChangeEvent{ev}, ordered...)
		}
		report, err := w.checker(ctx, ordered)
		if err != nil {
			w.reportError(surqlerrors.Wrap(
				surqlerrors.ErrValidation,
				"drift checker failed", err))
		} else if report != nil {
			select {
			case w.reports <- *report:
			case <-ctx.Done():
			case <-w.done:
			}
		}
		batch = batch[:0]
		for k := range dedupe {
			delete(dedupe, k)
		}
	}

	for {
		var timerC <-chan time.Time
		if timer != nil {
			timerC = timer.C
		}

		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			flush()
			return
		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			flush()
			return
		case ev, ok := <-w.events:
			if !ok {
				if timer != nil {
					timer.Stop()
				}
				flush()
				return
			}
			// Track the latest event per path for deduplication; the
			// slice retains order for downstream consumers.
			if _, exists := dedupe[ev.Path]; !exists {
				batch = append(batch, ev)
			} else {
				// Overwrite the prior occurrence in place so ChangeType
				// reflects the most recent state (e.g. a create+delete
				// pair still surfaces the delete).
				for i, existing := range batch {
					if existing.Path == ev.Path {
						batch[i] = ev
						break
					}
				}
			}
			dedupe[ev.Path] = ev

			// Reset (or start) the debounce timer.
			if timer == nil {
				timer = time.NewTimer(w.opts.Debounce)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(w.opts.Debounce)
			}
		case <-timerC:
			timer = nil
			flush()
		}
	}
}

// reportError forwards a non-fatal error to the OnError callback (if
// set) without blocking the caller. Errors during shutdown are dropped.
func (w *SchemaWatcher) reportError(err error) {
	if err == nil {
		return
	}
	if w.opts.OnError == nil {
		return
	}
	// Run in a goroutine so a slow handler can't stall the watcher.
	go w.opts.OnError(err)
}

// DefaultSchemaFileFilter accepts any .surql file and any .go file that
// is not a *_test.go file, matching the Go-idiomatic set of schema
// definitions. Files outside of those extensions are ignored.
func DefaultSchemaFileFilter(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	switch ext {
	case ".surql":
		return true
	case ".go":
		if len(base) >= len("_test.go") &&
			base[len(base)-len("_test.go"):] == "_test.go" {
			return false
		}
		return true
	}
	return false
}

// IsClosed reports whether err stems from fsnotify.Watcher.Close being
// called. It is exposed so callers embedding the watcher can filter
// shutdown noise.
func IsClosed(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, fsnotify.ErrEventOverflow) ||
		err.Error() == "fsnotify: watcher already closed"
}
