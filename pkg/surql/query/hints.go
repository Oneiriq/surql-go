package query

import (
	"fmt"
	"strconv"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// HintType is the category of a query hint, used to dedupe when MergeHints
// collapses overlapping hints.
type HintType string

const (
	// HintIndex is the index-selection hint category.
	HintIndex HintType = "index"
	// HintParallel is the parallel-execution hint category.
	HintParallel HintType = "parallel"
	// HintTimeout is the timeout-override hint category.
	HintTimeout HintType = "timeout"
	// HintFetch is the fetch-strategy hint category.
	HintFetch HintType = "fetch"
	// HintExplain is the execution-plan hint category.
	HintExplain HintType = "explain"
)

// QueryHint renders a SurrealQL comment that the query planner interprets.
type QueryHint interface {
	ToSurql() string
	Type() HintType
}

// FetchStrategy controls how records are retrieved for a query.
type FetchStrategy string

const (
	// FetchEager fetches all records up front.
	FetchEager FetchStrategy = "eager"
	// FetchLazy fetches records on demand.
	FetchLazy FetchStrategy = "lazy"
	// FetchBatch fetches records in fixed-size batches.
	FetchBatch FetchStrategy = "batch"
)

// IndexHint forces or suggests use of a particular index.
type IndexHint struct {
	Table string `json:"table"`
	Index string `json:"index"`
	Force bool   `json:"force,omitempty"`
}

// NewIndexHint builds an IndexHint that suggests (does not force) the index.
func NewIndexHint(table, index string) IndexHint {
	return IndexHint{Table: table, Index: index}
}

// WithForce returns a copy with the FORCE flag set.
func (h IndexHint) WithForce(force bool) IndexHint {
	h.Force = force
	return h
}

// ToSurql implements QueryHint.
func (h IndexHint) ToSurql() string {
	prefix := "USE"
	if h.Force {
		prefix = "FORCE"
	}
	return fmt.Sprintf("/* %s INDEX %s.%s */", prefix, h.Table, h.Index)
}

// Type implements QueryHint.
func (IndexHint) Type() HintType { return HintIndex }

// ParallelHint toggles parallel query execution and bounds worker count.
type ParallelHint struct {
	Enabled    bool  `json:"enabled"`
	MaxWorkers *uint `json:"max_workers,omitempty"`
}

// ParallelEnabled enables parallel execution with the server-picked worker count.
func ParallelEnabled() ParallelHint { return ParallelHint{Enabled: true} }

// ParallelDisabled disables parallel execution.
func ParallelDisabled() ParallelHint { return ParallelHint{Enabled: false} }

// ParallelWithWorkers enables parallel execution capped at n workers.
// Returns ErrValidation when n is outside 1..=32.
func ParallelWithWorkers(n uint) (ParallelHint, error) {
	if n < 1 || n > 32 {
		return ParallelHint{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"ParallelHint max_workers must be in 1..=32, got %d", n)
	}
	return ParallelHint{Enabled: true, MaxWorkers: &n}, nil
}

// ToSurql implements QueryHint.
func (h ParallelHint) ToSurql() string {
	if !h.Enabled {
		return "/* PARALLEL OFF */"
	}
	if h.MaxWorkers != nil {
		return fmt.Sprintf("/* PARALLEL %d */", *h.MaxWorkers)
	}
	return "/* PARALLEL ON */"
}

// Type implements QueryHint.
func (ParallelHint) Type() HintType { return HintParallel }

// TimeoutHint overrides the query timeout.
type TimeoutHint struct {
	Seconds float64 `json:"seconds"`
}

// NewTimeoutHint builds a TimeoutHint. Returns ErrValidation on non-positive seconds.
func NewTimeoutHint(seconds float64) (TimeoutHint, error) {
	if seconds != seconds /* NaN */ || seconds <= 0 {
		return TimeoutHint{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"TimeoutHint seconds must be > 0, got %v", seconds)
	}
	return TimeoutHint{Seconds: seconds}, nil
}

// ToSurql implements QueryHint.
func (h TimeoutHint) ToSurql() string {
	return "/* TIMEOUT " + trimFloat(h.Seconds) + "s */"
}

// Type implements QueryHint.
func (TimeoutHint) Type() HintType { return HintTimeout }

// FetchHint controls how records are fetched.
type FetchHint struct {
	Strategy  FetchStrategy `json:"strategy"`
	BatchSize *uint32       `json:"batch_size,omitempty"`
}

// FetchEagerHint builds an eager-fetch FetchHint.
func FetchEagerHint() FetchHint { return FetchHint{Strategy: FetchEager} }

// FetchLazyHint builds a lazy-fetch FetchHint.
func FetchLazyHint() FetchHint { return FetchHint{Strategy: FetchLazy} }

// FetchBatchHint builds a batch-fetch FetchHint (size must be in 1..=10000).
func FetchBatchHint(batchSize uint32) (FetchHint, error) {
	if batchSize < 1 || batchSize > 10_000 {
		return FetchHint{}, surqlerrors.Newf(surqlerrors.ErrValidation,
			"FetchHint batch_size must be in 1..=10000, got %d", batchSize)
	}
	bs := batchSize
	return FetchHint{Strategy: FetchBatch, BatchSize: &bs}, nil
}

// Validate reports missing-batch-size errors (mirrors surql-py).
func (h FetchHint) Validate() error {
	if h.Strategy == FetchBatch && h.BatchSize == nil {
		return surqlerrors.New(surqlerrors.ErrValidation,
			"FetchHint batch_size required when strategy is batch")
	}
	return nil
}

// ToSurql implements QueryHint.
func (h FetchHint) ToSurql() string {
	if h.Strategy == FetchBatch && h.BatchSize != nil {
		return fmt.Sprintf("/* FETCH BATCH %d */", *h.BatchSize)
	}
	switch h.Strategy {
	case FetchEager:
		return "/* FETCH EAGER */"
	case FetchLazy:
		return "/* FETCH LAZY */"
	case FetchBatch:
		return "/* FETCH BATCH */"
	default:
		return "/* FETCH */"
	}
}

// Type implements QueryHint.
func (FetchHint) Type() HintType { return HintFetch }

// ExplainHint requests the query execution plan.
type ExplainHint struct {
	Full bool `json:"full,omitempty"`
}

// ExplainShort builds a short-form ExplainHint.
func ExplainShort() ExplainHint { return ExplainHint{Full: false} }

// ExplainFull builds a full-form ExplainHint.
func ExplainFull() ExplainHint { return ExplainHint{Full: true} }

// ToSurql implements QueryHint.
func (h ExplainHint) ToSurql() string {
	if h.Full {
		return "/* EXPLAIN FULL */"
	}
	return "/* EXPLAIN */"
}

// Type implements QueryHint.
func (ExplainHint) Type() HintType { return HintExplain }

// ValidateHint checks a hint against the query's target table.
//
// Returns human-readable problems; an empty slice means the hint is OK.
// `table` is the query's target table, or "" if not known.
func ValidateHint(hint QueryHint, table string) []string {
	var errors []string
	if idx, ok := hint.(IndexHint); ok && table != "" && idx.Table != table {
		errors = append(errors, fmt.Sprintf(
			"Index hint table %q does not match query table %q", idx.Table, table))
	}
	return errors
}

// MergeHints collapses duplicate hints by HintType, keeping the last one
// of each type and preserving the insertion order of unique types.
func MergeHints(hints []QueryHint) []QueryHint {
	idx := map[HintType]int{}
	var out []QueryHint
	for _, h := range hints {
		t := h.Type()
		if i, ok := idx[t]; ok {
			out[i] = h
		} else {
			idx[t] = len(out)
			out = append(out, h)
		}
	}
	return out
}

// RenderHints joins a slice of hints into a single SurrealQL comment string.
func RenderHints(hints []QueryHint) string {
	if len(hints) == 0 {
		return ""
	}
	merged := MergeHints(hints)
	parts := make([]string, 0, len(merged))
	for _, h := range merged {
		parts = append(parts, h.ToSurql())
	}
	// manual join avoids extra allocation and mirrors the Python " ".join()
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += " " + parts[i]
	}
	return out
}

// HintRenderer is kept for API symmetry with surql-py.
type HintRenderer struct{}

// RenderHints is the stateful wrapper for RenderHints.
func (HintRenderer) RenderHints(hints []QueryHint) string { return RenderHints(hints) }

// trimFloat prints f without unnecessary trailing zeros (30.0 -> "30").
func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
