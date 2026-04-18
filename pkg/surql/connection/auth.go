package connection

// AuthType is the SurrealDB authentication level.
type AuthType string

const (
	// AuthRoot == server root user.
	AuthRoot AuthType = "root"
	// AuthNamespace == namespace user.
	AuthNamespace AuthType = "namespace"
	// AuthDatabase == database user.
	AuthDatabase AuthType = "database"
	// AuthScope == record/scope-level user.
	AuthScope AuthType = "scope"
)

// Credentials is implemented by every credential type so the runtime
// client can serialise them to the SurrealDB SDK signin/signup payload.
type Credentials interface {
	AuthType() AuthType
	ToSigninPayload() map[string]any
}

// RootCredentials carry server root-level username + password.
type RootCredentials struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

// AuthType implements Credentials.
func (RootCredentials) AuthType() AuthType { return AuthRoot }

// ToSigninPayload implements Credentials.
func (c RootCredentials) ToSigninPayload() map[string]any {
	out := map[string]any{"username": c.Username}
	if c.Password != "" {
		out["password"] = c.Password
	}
	return out
}

// NewRootCredentials constructs a RootCredentials.
func NewRootCredentials(username, password string) RootCredentials {
	return RootCredentials{Username: username, Password: password}
}

// NamespaceCredentials carry namespace-scoped signin material.
type NamespaceCredentials struct {
	Namespace string `json:"namespace"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
}

// AuthType implements Credentials.
func (NamespaceCredentials) AuthType() AuthType { return AuthNamespace }

// ToSigninPayload implements Credentials.
func (c NamespaceCredentials) ToSigninPayload() map[string]any {
	out := map[string]any{
		"namespace": c.Namespace,
		"username":  c.Username,
	}
	if c.Password != "" {
		out["password"] = c.Password
	}
	return out
}

// NewNamespaceCredentials constructs a NamespaceCredentials.
func NewNamespaceCredentials(namespace, username, password string) NamespaceCredentials {
	return NamespaceCredentials{Namespace: namespace, Username: username, Password: password}
}

// DatabaseCredentials carry database-scoped signin material.
type DatabaseCredentials struct {
	Namespace string `json:"namespace"`
	Database  string `json:"database"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
}

// AuthType implements Credentials.
func (DatabaseCredentials) AuthType() AuthType { return AuthDatabase }

// ToSigninPayload implements Credentials.
func (c DatabaseCredentials) ToSigninPayload() map[string]any {
	out := map[string]any{
		"namespace": c.Namespace,
		"database":  c.Database,
		"username":  c.Username,
	}
	if c.Password != "" {
		out["password"] = c.Password
	}
	return out
}

// NewDatabaseCredentials constructs a DatabaseCredentials.
func NewDatabaseCredentials(namespace, database, username, password string) DatabaseCredentials {
	return DatabaseCredentials{
		Namespace: namespace, Database: database, Username: username, Password: password,
	}
}

// ScopeCredentials carry record/scope-level signin material. Variables are
// flattened into the top-level signin payload (e.g. email, password).
type ScopeCredentials struct {
	Namespace string         `json:"namespace"`
	Database  string         `json:"database"`
	Access    string         `json:"access"`
	Variables map[string]any `json:"variables,omitempty"`
}

// AuthType implements Credentials.
func (ScopeCredentials) AuthType() AuthType { return AuthScope }

// ToSigninPayload implements Credentials.
func (c ScopeCredentials) ToSigninPayload() map[string]any {
	out := map[string]any{
		"namespace": c.Namespace,
		"database":  c.Database,
		"access":    c.Access,
	}
	for k, v := range c.Variables {
		out[k] = v
	}
	return out
}

// NewScopeCredentials constructs ScopeCredentials with an empty variable set.
func NewScopeCredentials(namespace, database, access string) ScopeCredentials {
	return ScopeCredentials{
		Namespace: namespace, Database: database, Access: access,
		Variables: map[string]any{},
	}
}

// With attaches a scope variable (e.g. "email", "password") and returns a
// copy.
func (c ScopeCredentials) With(key string, value any) ScopeCredentials {
	vars := make(map[string]any, len(c.Variables)+1)
	for k, v := range c.Variables {
		vars[k] = v
	}
	vars[key] = value
	c.Variables = vars
	return c
}

// TokenAuth carries an existing JWT.
type TokenAuth struct {
	Token string `json:"token"`
}

// NewTokenAuth constructs a TokenAuth.
func NewTokenAuth(token string) TokenAuth {
	return TokenAuth{Token: token}
}

// Compile-time interface checks.
var (
	_ Credentials = RootCredentials{}
	_ Credentials = NamespaceCredentials{}
	_ Credentials = DatabaseCredentials{}
	_ Credentials = ScopeCredentials{}
)
