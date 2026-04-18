package migration

import (
	"time"
)

// MigrationState is the state of a migration in the execution lifecycle.
type MigrationState string

// MigrationState values mirrored from surql-py MigrationState enum.
const (
	MigrationStatePending MigrationState = "pending"
	MigrationStateApplied MigrationState = "applied"
	MigrationStateFailed  MigrationState = "failed"
)

// IsValid reports whether the receiver is one of the defined MigrationState
// constants.
func (s MigrationState) IsValid() bool {
	switch s {
	case MigrationStatePending, MigrationStateApplied, MigrationStateFailed:
		return true
	}
	return false
}

// String returns the serialized form of the state.
func (s MigrationState) String() string { return string(s) }

// MigrationDirection indicates whether a migration is applied forward (up)
// or rolled back (down).
type MigrationDirection string

// MigrationDirection values mirrored from surql-py MigrationDirection enum.
const (
	MigrationDirectionUp   MigrationDirection = "up"
	MigrationDirectionDown MigrationDirection = "down"
)

// IsValid reports whether the receiver is one of the defined
// MigrationDirection constants.
func (d MigrationDirection) IsValid() bool {
	switch d {
	case MigrationDirectionUp, MigrationDirectionDown:
		return true
	}
	return false
}

// String returns the serialized form of the direction.
func (d MigrationDirection) String() string { return string(d) }

// DiffOperation enumerates the kinds of schema changes a diff can produce.
type DiffOperation string

// DiffOperation values mirrored from surql-py DiffOperation enum.
const (
	DiffOperationAddTable          DiffOperation = "add_table"
	DiffOperationDropTable         DiffOperation = "drop_table"
	DiffOperationAddField          DiffOperation = "add_field"
	DiffOperationDropField         DiffOperation = "drop_field"
	DiffOperationModifyField       DiffOperation = "modify_field"
	DiffOperationAddIndex          DiffOperation = "add_index"
	DiffOperationDropIndex         DiffOperation = "drop_index"
	DiffOperationAddEvent          DiffOperation = "add_event"
	DiffOperationDropEvent         DiffOperation = "drop_event"
	DiffOperationModifyPermissions DiffOperation = "modify_permissions"
)

// IsValid reports whether the receiver is one of the defined DiffOperation
// constants.
func (o DiffOperation) IsValid() bool {
	switch o {
	case DiffOperationAddTable, DiffOperationDropTable,
		DiffOperationAddField, DiffOperationDropField, DiffOperationModifyField,
		DiffOperationAddIndex, DiffOperationDropIndex,
		DiffOperationAddEvent, DiffOperationDropEvent,
		DiffOperationModifyPermissions:
		return true
	}
	return false
}

// String returns the serialized form of the operation.
func (o DiffOperation) String() string { return string(o) }

// Migration is a single migration definition, parsed from a `.surql` file.
//
// Unlike the Python port (which carries Python callables), the Go model
// stores pre-parsed SurrealQL statement slices for the forward and rollback
// directions. This keeps the struct trivially serializable and thread-safe.
type Migration struct {
	// Version is the timestamp-based version string (YYYYMMDD_HHMMSS).
	Version string `json:"version"`
	// Description is a short human-readable summary.
	Description string `json:"description"`
	// Path is the absolute or relative path the migration was loaded from.
	// Empty when the migration was constructed in memory.
	Path string `json:"path,omitempty"`
	// UpStatements are the SurrealQL statements executed for a forward migration.
	UpStatements []string `json:"up_statements"`
	// DownStatements are the SurrealQL statements executed on rollback.
	DownStatements []string `json:"down_statements"`
	// Checksum is a SHA-256 digest of the raw file contents, used to detect
	// drift between applied and on-disk migrations. Empty when unknown.
	Checksum string `json:"checksum,omitempty"`
	// DependsOn lists version strings this migration requires to be applied
	// first. Empty when there are no explicit dependencies.
	DependsOn []string `json:"depends_on,omitempty"`
}

// MigrationHistory is a record of a migration that has been applied to the
// database. It is intended to be persisted (JSON-serializable).
type MigrationHistory struct {
	Version         string    `json:"version"`
	Description     string    `json:"description"`
	AppliedAt       time.Time `json:"applied_at"`
	Checksum        string    `json:"checksum"`
	ExecutionTimeMs *int64    `json:"execution_time_ms,omitempty"`
}

// MigrationPlan is an ordered list of migrations with a direction.
type MigrationPlan struct {
	Migrations []Migration        `json:"migrations"`
	Direction  MigrationDirection `json:"direction"`
}

// Count returns the number of migrations in the plan.
func (p MigrationPlan) Count() int { return len(p.Migrations) }

// IsEmpty reports whether the plan contains no migrations.
func (p MigrationPlan) IsEmpty() bool { return len(p.Migrations) == 0 }

// MigrationMetadata is the parsed header metadata of a migration file,
// mirroring the Python MigrationMetadata model.
type MigrationMetadata struct {
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

// MigrationStatus combines a migration definition with its observed state,
// optionally with the application timestamp and a last-error message.
type MigrationStatus struct {
	Migration Migration      `json:"migration"`
	State     MigrationState `json:"state"`
	AppliedAt *time.Time     `json:"applied_at,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// SchemaDiff represents a single difference between two schema versions,
// carrying both the forward and backward SurrealQL to apply / revert it.
type SchemaDiff struct {
	Operation   DiffOperation  `json:"operation"`
	Table       string         `json:"table"`
	Field       string         `json:"field,omitempty"`
	Index       string         `json:"index,omitempty"`
	Event       string         `json:"event,omitempty"`
	Description string         `json:"description"`
	ForwardSQL  string         `json:"forward_sql"`
	BackwardSQL string         `json:"backward_sql"`
	Details     map[string]any `json:"details,omitempty"`
}
