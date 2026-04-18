// Package errors defines the unified error hierarchy for the surql library.
//
// It follows idiomatic Go error handling: sentinel errors for the high-level
// categories (used with errors.Is) plus typed error structs carrying
// structured context (used with errors.As). Every fallible operation in
// surql returns an error built on one of these roots.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors. Use errors.Is to test membership.
var (
	// ErrDatabase is the root for all database-level failures.
	ErrDatabase = errors.New("database error")
	// ErrConnection indicates a connection failed, timed out, or closed.
	ErrConnection = errors.New("connection error")
	// ErrQuery indicates a query failed at the database or during decoding.
	ErrQuery = errors.New("query error")
	// ErrTransaction indicates a transaction lifecycle failure.
	ErrTransaction = errors.New("transaction error")
	// ErrContext indicates a missing or misconfigured ambient connection context.
	ErrContext = errors.New("context error")
	// ErrRegistry indicates a named-connection registry failure.
	ErrRegistry = errors.New("registry error")
	// ErrStreaming indicates a live/streaming query failure.
	ErrStreaming = errors.New("streaming error")
	// ErrValidation indicates invalid input or malformed identifier.
	ErrValidation = errors.New("validation error")
	// ErrSchemaParse indicates the schema parser could not interpret input.
	ErrSchemaParse = errors.New("schema parse error")
	// ErrMigrationDiscovery indicates a migration file discovery failure.
	ErrMigrationDiscovery = errors.New("migration discovery error")
	// ErrMigrationLoad indicates loading an individual migration failed.
	ErrMigrationLoad = errors.New("migration load error")
	// ErrMigrationGeneration indicates migration generation failed.
	ErrMigrationGeneration = errors.New("migration generation error")
	// ErrMigrationExecution indicates executing a migration failed.
	ErrMigrationExecution = errors.New("migration execution error")
	// ErrMigrationHistory indicates a migration history failure.
	ErrMigrationHistory = errors.New("migration history error")
	// ErrMigrationSquash indicates migration squashing failed.
	ErrMigrationSquash = errors.New("migration squash error")
	// ErrOrchestration indicates a multi-environment orchestration failure.
	ErrOrchestration = errors.New("orchestration error")
	// ErrSerialization indicates JSON encode/decode failure.
	ErrSerialization = errors.New("serialization error")
)

// SurqlError is the typed error returned by surql operations. It wraps
// one of the sentinels (available via Unwrap / errors.Is) with a
// human-readable reason and an optional inner error.
type SurqlError struct {
	Kind    error
	Reason  string
	Wrapped error
}

// Error returns a formatted error string.
func (e *SurqlError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Reason == "" && e.Wrapped == nil {
		if e.Kind != nil {
			return e.Kind.Error()
		}
		return "surql: unknown error"
	}
	kind := "surql error"
	if e.Kind != nil {
		kind = e.Kind.Error()
	}
	switch {
	case e.Reason != "" && e.Wrapped != nil:
		return fmt.Sprintf("%s: %s: %v", kind, e.Reason, e.Wrapped)
	case e.Reason != "":
		return fmt.Sprintf("%s: %s", kind, e.Reason)
	default:
		return fmt.Sprintf("%s: %v", kind, e.Wrapped)
	}
}

// Unwrap returns the next error in the chain. If a wrapped error exists
// it is preferred (preserving cause chains); otherwise the sentinel kind
// is returned so that errors.Is works.
func (e *SurqlError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Wrapped != nil {
		return e.Wrapped
	}
	return e.Kind
}

// Is reports whether target matches this error's kind or wrapped chain.
func (e *SurqlError) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	if e.Kind != nil && errors.Is(e.Kind, target) {
		return true
	}
	if e.Wrapped != nil && errors.Is(e.Wrapped, target) {
		return true
	}
	return false
}

// New constructs a SurqlError with the given sentinel kind and reason.
func New(kind error, reason string) *SurqlError {
	return &SurqlError{Kind: kind, Reason: reason}
}

// Wrap constructs a SurqlError that wraps an existing error with an added
// sentinel kind and contextual reason.
func Wrap(kind error, reason string, err error) *SurqlError {
	return &SurqlError{Kind: kind, Reason: reason, Wrapped: err}
}

// Newf constructs a SurqlError with fmt.Sprintf-style formatting.
func Newf(kind error, format string, args ...any) *SurqlError {
	return &SurqlError{Kind: kind, Reason: fmt.Sprintf(format, args...)}
}

// Wrapf constructs a SurqlError that wraps err with fmt.Sprintf-style context.
func Wrapf(kind error, err error, format string, args ...any) *SurqlError {
	return &SurqlError{Kind: kind, Reason: fmt.Sprintf(format, args...), Wrapped: err}
}
