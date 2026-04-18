package schema

import (
	"strings"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// AccessType enumerates the access control types supported by DEFINE ACCESS.
type AccessType string

// AccessType values.
const (
	AccessTypeJWT    AccessType = "JWT"
	AccessTypeRecord AccessType = "RECORD"
)

// IsValid reports whether the AccessType is recognised.
func (a AccessType) IsValid() bool {
	switch a {
	case AccessTypeJWT, AccessTypeRecord:
		return true
	}
	return false
}

// JwtConfig describes the JWT-specific configuration of a DEFINE ACCESS
// statement. The Algorithm field defaults to "HS256" when unset.
type JwtConfig struct {
	Algorithm string
	Key       string
	URL       string
	Issuer    string
}

// RecordAccessConfig describes the RECORD-specific configuration of a DEFINE
// ACCESS statement.
type RecordAccessConfig struct {
	Signup string
	Signin string
}

// AccessDefinition captures a DEFINE ACCESS statement.
type AccessDefinition struct {
	Name            string
	Type            AccessType
	JWT             *JwtConfig
	Record          *RecordAccessConfig
	DurationSession string
	DurationToken   string
}

// AccessOption customises an AccessDefinition created via NewAccess.
type AccessOption func(*AccessDefinition)

// WithJWT attaches a JwtConfig (and implicitly sets Type to AccessTypeJWT if
// not already set by the caller).
func WithJWT(cfg JwtConfig) AccessOption {
	return func(a *AccessDefinition) {
		cp := cfg
		a.JWT = &cp
	}
}

// WithRecord attaches a RecordAccessConfig.
func WithRecord(cfg RecordAccessConfig) AccessOption {
	return func(a *AccessDefinition) {
		cp := cfg
		a.Record = &cp
	}
}

// WithDurationSession sets the FOR SESSION clause.
func WithDurationSession(dur string) AccessOption {
	return func(a *AccessDefinition) { a.DurationSession = dur }
}

// WithDurationToken sets the FOR TOKEN clause.
func WithDurationToken(dur string) AccessOption {
	return func(a *AccessDefinition) { a.DurationToken = dur }
}

// NewAccess constructs an AccessDefinition of the given type.
func NewAccess(name string, accessType AccessType, opts ...AccessOption) AccessDefinition {
	a := AccessDefinition{Name: name, Type: accessType}
	for _, opt := range opts {
		opt(&a)
	}
	return a
}

// JwtAccess constructs a JWT-type AccessDefinition. Algorithm defaults to
// HS256 when left blank.
func JwtAccess(name string, cfg JwtConfig, opts ...AccessOption) AccessDefinition {
	if cfg.Algorithm == "" {
		cfg.Algorithm = "HS256"
	}
	merged := append([]AccessOption{WithJWT(cfg)}, opts...)
	return NewAccess(name, AccessTypeJWT, merged...)
}

// RecordAccess constructs a RECORD-type AccessDefinition.
func RecordAccess(name string, cfg RecordAccessConfig, opts ...AccessOption) AccessDefinition {
	merged := append([]AccessOption{WithRecord(cfg)}, opts...)
	return NewAccess(name, AccessTypeRecord, merged...)
}

// Validate checks structural invariants for the access definition.
func (a AccessDefinition) Validate() error {
	if a.Name == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "access name cannot be empty")
	}
	if !a.Type.IsValid() {
		return surqlerrors.Newf(surqlerrors.ErrValidation,
			"invalid access type %q for access %q", string(a.Type), a.Name)
	}
	switch a.Type {
	case AccessTypeJWT:
		if a.JWT == nil {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"JWT access %q requires a JWT config", a.Name)
		}
	case AccessTypeRecord:
		if a.Record == nil {
			return surqlerrors.Newf(surqlerrors.ErrValidation,
				"RECORD access %q requires a record config", a.Name)
		}
	}
	return nil
}

// ToSurql emits the DEFINE ACCESS statement.
func (a AccessDefinition) ToSurql() string {
	var b strings.Builder
	b.WriteString("DEFINE ACCESS ")
	b.WriteString(a.Name)
	b.WriteString(" ON DATABASE TYPE ")
	b.WriteString(string(a.Type))

	if a.Type == AccessTypeJWT && a.JWT != nil {
		algo := a.JWT.Algorithm
		if algo == "" {
			algo = "HS256"
		}
		b.WriteString(" ALGORITHM ")
		b.WriteString(algo)
		if a.JWT.Key != "" {
			b.WriteString(" KEY '")
			b.WriteString(a.JWT.Key)
			b.WriteString("'")
		}
		if a.JWT.URL != "" {
			b.WriteString(" URL '")
			b.WriteString(a.JWT.URL)
			b.WriteString("'")
		}
		if a.JWT.Issuer != "" {
			b.WriteString(" WITH ISSUER '")
			b.WriteString(a.JWT.Issuer)
			b.WriteString("'")
		}
	}

	if a.Type == AccessTypeRecord && a.Record != nil {
		if a.Record.Signup != "" {
			b.WriteString(" SIGNUP (")
			b.WriteString(a.Record.Signup)
			b.WriteString(")")
		}
		if a.Record.Signin != "" {
			b.WriteString(" SIGNIN (")
			b.WriteString(a.Record.Signin)
			b.WriteString(")")
		}
	}

	if a.DurationSession != "" || a.DurationToken != "" {
		parts := make([]string, 0, 2)
		if a.DurationSession != "" {
			parts = append(parts, "FOR SESSION "+a.DurationSession)
		}
		if a.DurationToken != "" {
			parts = append(parts, "FOR TOKEN "+a.DurationToken)
		}
		b.WriteString(" DURATION ")
		b.WriteString(strings.Join(parts, ", "))
	}

	b.WriteString(";")
	return b.String()
}
