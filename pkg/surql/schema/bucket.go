package schema

import (
	"regexp"
	"sort"
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// bucketNameRe matches valid bucket names: an identifier of alphanumerics and
// underscores that does not start with a digit. SurrealDB bucket names follow
// the same identifier rules as table names.
var bucketNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// BucketDefinition captures a DEFINE BUCKET statement (SurrealDB v3 object
// storage). A bucket is a named handle over a storage backend (in-memory, a
// local directory, or an S3-compatible endpoint) that file-typed fields and
// the connection.Bucket runtime handle read from and write to.
//
// It mirrors AccessDefinition in construction style: a plain struct with
// functional-option constructors and a ToSurql emitter.
type BucketDefinition struct {
	// Name is the bucket identifier (table-name rules).
	Name string
	// Backend is the storage backend URL. Recognised forms are "memory",
	// "file:/absolute/path", and "s3://bucket/prefix". The value is emitted
	// verbatim inside the BACKEND "..." clause.
	Backend string
	// ReadOnly marks the bucket READONLY (writes are rejected by the server).
	ReadOnly bool
	// Permissions maps an action (select / create / update / delete) to a
	// SurrealQL WHERE expression, mirroring TableDefinition.Permissions. The
	// special single-entry maps {"*": "FULL"} and {"*": "NONE"} emit
	// PERMISSIONS FULL / PERMISSIONS NONE respectively.
	Permissions map[string]string
	// Comment is an optional COMMENT "..." annotation. The empty string emits
	// no COMMENT clause.
	Comment string
}

// BucketOption customises a BucketDefinition created via NewBucket.
type BucketOption func(*BucketDefinition)

// WithBucketReadOnly marks the bucket READONLY.
func WithBucketReadOnly(readonly bool) BucketOption {
	return func(b *BucketDefinition) { b.ReadOnly = readonly }
}

// WithBucketPermissions sets the PERMISSIONS map keyed by action. Pass
// {"*": "FULL"} or {"*": "NONE"} for the blanket forms.
func WithBucketPermissions(perms map[string]string) BucketOption {
	return func(b *BucketDefinition) {
		if perms == nil {
			b.Permissions = nil
			return
		}
		copied := make(map[string]string, len(perms))
		for k, v := range perms {
			copied[k] = v
		}
		b.Permissions = copied
	}
}

// WithBucketComment attaches a COMMENT "..." annotation.
func WithBucketComment(comment string) BucketOption {
	return func(b *BucketDefinition) { b.Comment = comment }
}

// NewBucket constructs a BucketDefinition over the given backend.
func NewBucket(name, backend string, opts ...BucketOption) BucketDefinition {
	b := BucketDefinition{Name: name, Backend: backend}
	for _, opt := range opts {
		opt(&b)
	}
	return b
}

// MemoryBucket constructs an in-memory bucket (BACKEND "memory").
func MemoryBucket(name string, opts ...BucketOption) BucketDefinition {
	return NewBucket(name, "memory", opts...)
}

// FileBucket constructs a bucket backed by a local directory (BACKEND
// "file:<path>").
func FileBucket(name, path string, opts ...BucketOption) BucketDefinition {
	return NewBucket(name, "file:"+path, opts...)
}

// S3Bucket constructs a bucket backed by an S3-compatible endpoint (BACKEND
// "s3://<location>").
func S3Bucket(name, location string, opts ...BucketOption) BucketDefinition {
	return NewBucket(name, "s3://"+location, opts...)
}

// Validate checks structural invariants for the bucket definition.
func (b BucketDefinition) Validate() error {
	if b.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "bucket name cannot be empty")
	}
	if !bucketNameRe.MatchString(b.Name) {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"invalid bucket name %q: must contain only alphanumeric characters and underscores, and cannot start with a digit",
			b.Name)
	}
	if strings.TrimSpace(b.Backend) == "" {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"bucket %q requires a non-empty backend", b.Name)
	}
	return nil
}

// ToSurql emits the DEFINE BUCKET statement.
func (b BucketDefinition) ToSurql() string {
	return b.toSurql(bucketDefinePlain)
}

// ToSurqlIfNotExists emits DEFINE BUCKET IF NOT EXISTS.
func (b BucketDefinition) ToSurqlIfNotExists() string {
	return b.toSurql(bucketDefineIfNotExists)
}

// ToSurqlOverwrite emits DEFINE BUCKET OVERWRITE.
func (b BucketDefinition) ToSurqlOverwrite() string {
	return b.toSurql(bucketDefineOverwrite)
}

// bucketDefineMode selects the optional clause after DEFINE BUCKET.
type bucketDefineMode int

const (
	bucketDefinePlain bucketDefineMode = iota
	bucketDefineIfNotExists
	bucketDefineOverwrite
)

func (b BucketDefinition) toSurql(mode bucketDefineMode) string {
	var sb strings.Builder
	sb.WriteString("DEFINE BUCKET")
	switch mode {
	case bucketDefineIfNotExists:
		sb.WriteString(" IF NOT EXISTS")
	case bucketDefineOverwrite:
		sb.WriteString(" OVERWRITE")
	}
	sb.WriteString(" ")
	sb.WriteString(b.Name)
	sb.WriteString(` BACKEND "`)
	sb.WriteString(b.Backend)
	sb.WriteString(`"`)
	if b.ReadOnly {
		sb.WriteString(" READONLY")
	}
	if clause := permissionsClause(b.Permissions); clause != "" {
		sb.WriteString(" ")
		sb.WriteString(clause)
	}
	if b.Comment != "" {
		sb.WriteString(` COMMENT "`)
		sb.WriteString(b.Comment)
		sb.WriteString(`"`)
	}
	sb.WriteString(";")
	return sb.String()
}

// ToRemoveSurql emits the REMOVE BUCKET statement for this bucket.
func (b BucketDefinition) ToRemoveSurql() string {
	return "REMOVE BUCKET " + b.Name + ";"
}

// RemoveBucketSurql emits REMOVE BUCKET <name> for an arbitrary name.
func RemoveBucketSurql(name string) string {
	return "REMOVE BUCKET " + name + ";"
}

// permissionsClause renders the PERMISSIONS clause shared by DEFINE BUCKET and
// ALTER BUCKET. It returns "" for an empty map. The single-entry blanket forms
// {"*": "FULL"} / {"*": "NONE"} (case-insensitive) emit PERMISSIONS FULL /
// PERMISSIONS NONE. Otherwise each action is emitted as
// "FOR <action> WHERE <expr>" in sorted order.
func permissionsClause(perms map[string]string) string {
	if len(perms) == 0 {
		return ""
	}
	if len(perms) == 1 {
		if expr, ok := perms["*"]; ok {
			switch strings.ToUpper(strings.TrimSpace(expr)) {
			case "FULL":
				return "PERMISSIONS FULL"
			case "NONE":
				return "PERMISSIONS NONE"
			}
		}
	}
	keys := make([]string, 0, len(perms))
	for k := range perms {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, action := range keys {
		parts = append(parts,
			"FOR "+strings.ToLower(action)+" WHERE "+perms[action])
	}
	return "PERMISSIONS " + strings.Join(parts, " ")
}

// AlterBucketChange describes a single mutation applied by an ALTER BUCKET
// statement. A nil pointer field means "leave unchanged"; the Drop* booleans
// emit the DROP form for that clause (and take precedence over a set value).
type AlterBucketChange struct {
	// ReadOnly, when non-nil, sets (true) or clears (false) the READONLY flag.
	// SurrealDB spells the clear form as DROP READONLY.
	ReadOnly *bool
	// Backend, when non-nil, sets the BACKEND "..." clause.
	Backend *string
	// DropBackend emits DROP BACKEND (reverting to the server default).
	DropBackend bool
	// Permissions, when non-nil, replaces the PERMISSIONS clause.
	Permissions map[string]string
	// Comment, when non-nil, sets the COMMENT "..." clause.
	Comment *string
	// DropComment emits DROP COMMENT.
	DropComment bool
}

// AlterBucketSurql emits an ALTER BUCKET statement for name applying change.
// When ifExists is true the statement carries IF EXISTS. Clauses are emitted
// in the order READONLY, BACKEND, PERMISSIONS, COMMENT to match the DEFINE
// ordering. An empty change (no fields set) still emits a syntactically valid
// "ALTER BUCKET <name>;" no-op.
func AlterBucketSurql(name string, change AlterBucketChange, ifExists bool) string {
	var sb strings.Builder
	sb.WriteString("ALTER BUCKET")
	if ifExists {
		sb.WriteString(" IF EXISTS")
	}
	sb.WriteString(" ")
	sb.WriteString(name)

	if change.ReadOnly != nil {
		if *change.ReadOnly {
			sb.WriteString(" READONLY")
		} else {
			sb.WriteString(" DROP READONLY")
		}
	}
	switch {
	case change.DropBackend:
		sb.WriteString(" DROP BACKEND")
	case change.Backend != nil:
		sb.WriteString(` BACKEND "`)
		sb.WriteString(*change.Backend)
		sb.WriteString(`"`)
	}
	if change.Permissions != nil {
		if clause := permissionsClause(change.Permissions); clause != "" {
			sb.WriteString(" ")
			sb.WriteString(clause)
		}
	}
	switch {
	case change.DropComment:
		sb.WriteString(" DROP COMMENT")
	case change.Comment != nil:
		sb.WriteString(` COMMENT "`)
		sb.WriteString(*change.Comment)
		sb.WriteString(`"`)
	}
	sb.WriteString(";")
	return sb.String()
}
