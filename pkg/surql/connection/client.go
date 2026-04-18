package connection

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	surrealdb "github.com/surrealdb/surrealdb.go"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
)

// DatabaseClient is a high-level wrapper around the official SurrealDB Go SDK
// (*surrealdb.DB). It centralises connection, retry, authentication, CRUD, raw
// query, transaction, and live-query operations so callers interact with a
// stable surface driven by ConnectionConfig and the typed Credentials set.
//
// The zero value is not usable; construct one with NewDatabaseClient.
type DatabaseClient struct {
	cfg ConnectionConfig

	mu        sync.RWMutex
	db        *surrealdb.DB
	connected bool
	token     string
	authType  AuthType
}

// NewDatabaseClient constructs a DatabaseClient from the provided configuration.
// The config is validated but no network I/O is performed; call Connect to
// establish the connection.
func NewDatabaseClient(cfg ConnectionConfig) (*DatabaseClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &DatabaseClient{cfg: cfg}, nil
}

// Config returns a copy of the client's connection configuration.
func (c *DatabaseClient) Config() ConnectionConfig {
	return c.cfg
}

// IsConnected reports whether the underlying SDK connection has been
// established (and not yet closed).
func (c *DatabaseClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.db != nil
}

// Connect opens the SurrealDB connection, selects the configured namespace and
// database, and applies exponential backoff (bounded by the config's retry
// fields). Reconnects transparently if the client is already connected.
//
// The provided context cancels the entire retry window; individual attempts
// are not further subdivided.
func (c *DatabaseClient) Connect(ctx context.Context) error {
	if c.IsConnected() {
		if err := c.Disconnect(); err != nil {
			return err
		}
	}

	var lastErr error
	attempts := c.cfg.DBRetryMaxAttempts
	if attempts == 0 {
		attempts = 1
	}

	for attempt := uint32(1); attempt <= attempts; attempt++ {
		if ctx.Err() != nil {
			return surqlerrors.Wrap(surqlerrors.ErrConnection, "context cancelled while connecting", ctx.Err())
		}

		db, err := surrealdb.FromEndpointURLString(ctx, c.cfg.DBURL)
		if err == nil {
			if useErr := db.Use(ctx, c.cfg.DBNS, c.cfg.DB); useErr != nil {
				_ = db.Close(ctx)
				err = useErr
			} else {
				c.mu.Lock()
				c.db = db
				c.connected = true
				c.mu.Unlock()
				return nil
			}
		}

		lastErr = err
		if attempt == attempts {
			break
		}

		wait := backoffDelay(
			attempt,
			c.cfg.DBRetryMinWait,
			c.cfg.DBRetryMaxWait,
			c.cfg.DBRetryMultiplier,
		)
		select {
		case <-ctx.Done():
			return surqlerrors.Wrap(surqlerrors.ErrConnection, "context cancelled during retry backoff", ctx.Err())
		case <-time.After(wait):
		}
	}

	return surqlerrors.Wrapf(
		surqlerrors.ErrConnection,
		lastErr,
		"failed to connect to %s after %d attempts",
		c.cfg.DBURL, attempts,
	)
}

// Disconnect closes the SDK connection. Calling Disconnect on an already
// disconnected client is a no-op.
func (c *DatabaseClient) Disconnect() error {
	c.mu.Lock()
	db := c.db
	c.db = nil
	c.connected = false
	c.token = ""
	c.authType = ""
	c.mu.Unlock()

	if db == nil {
		return nil
	}
	if err := db.Close(context.Background()); err != nil {
		return surqlerrors.Wrap(surqlerrors.ErrConnection, "failed to close connection", err)
	}
	return nil
}

// Signin authenticates against SurrealDB using the supplied Credentials. The
// returned TokenAuth wraps the JWT issued by the server and is also cached on
// the client for subsequent calls.
func (c *DatabaseClient) Signin(ctx context.Context, creds Credentials) (TokenAuth, error) {
	if creds == nil {
		return TokenAuth{}, surqlerrors.New(surqlerrors.ErrValidation, "credentials must not be nil")
	}
	db, err := c.requireDB()
	if err != nil {
		return TokenAuth{}, err
	}

	payload := toSdkAuthPayload(creds)
	token, err := db.SignIn(ctx, payload)
	if err != nil {
		return TokenAuth{}, surqlerrors.Wrap(surqlerrors.ErrConnection, "signin failed", err)
	}

	c.mu.Lock()
	c.token = token
	c.authType = creds.AuthType()
	c.mu.Unlock()

	return TokenAuth{Token: token}, nil
}

// Signup signs up a new record user with the supplied scope credentials.
func (c *DatabaseClient) Signup(ctx context.Context, creds ScopeCredentials) (TokenAuth, error) {
	db, err := c.requireDB()
	if err != nil {
		return TokenAuth{}, err
	}

	payload := toSdkAuthPayload(creds)
	token, err := db.SignUp(ctx, payload)
	if err != nil {
		return TokenAuth{}, surqlerrors.Wrap(surqlerrors.ErrConnection, "signup failed", err)
	}

	c.mu.Lock()
	c.token = token
	c.authType = AuthScope
	c.mu.Unlock()

	return TokenAuth{Token: token}, nil
}

// Authenticate authenticates the session with an existing JWT.
func (c *DatabaseClient) Authenticate(ctx context.Context, token string) error {
	db, err := c.requireDB()
	if err != nil {
		return err
	}
	if token == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "token must not be empty")
	}
	if err := db.Authenticate(ctx, token); err != nil {
		return surqlerrors.Wrap(surqlerrors.ErrConnection, "authenticate failed", err)
	}
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
	return nil
}

// Invalidate drops the current session's authentication state on both client
// and server.
func (c *DatabaseClient) Invalidate(ctx context.Context) error {
	db, err := c.requireDB()
	if err != nil {
		return err
	}
	if err := db.Invalidate(ctx); err != nil {
		return surqlerrors.Wrap(surqlerrors.ErrConnection, "invalidate failed", err)
	}
	c.mu.Lock()
	c.token = ""
	c.authType = ""
	c.mu.Unlock()
	return nil
}

// CurrentToken returns the most recent JWT issued to this client (or the empty
// string if unauthenticated).
func (c *DatabaseClient) CurrentToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// CurrentAuthType returns the AuthType of the most recent successful signin
// (or empty if never authenticated).
func (c *DatabaseClient) CurrentAuthType() AuthType {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authType
}

// Query executes a raw SurrealQL query without any parameters and returns the
// response result slice produced by the SDK.
func (c *DatabaseClient) Query(ctx context.Context, surql string) (any, error) {
	return c.QueryWithVars(ctx, surql, nil)
}

// QueryWithVars executes a parameterised SurrealQL query. Results are returned
// as []any, preserving the per-statement response envelope from the server.
func (c *DatabaseClient) QueryWithVars(ctx context.Context, surql string, vars map[string]any) (any, error) {
	db, err := c.requireDB()
	if err != nil {
		return nil, err
	}
	results, err := surrealdb.Query[any](ctx, db, surql, vars)
	if err != nil {
		return nil, mapQueryError(err)
	}
	if results == nil {
		return []any{}, nil
	}
	out := make([]any, 0, len(*results))
	for _, r := range *results {
		out = append(out, map[string]any{
			"status": r.Status,
			"time":   r.Time,
			"result": r.Result,
		})
	}
	return out, nil
}

// Select executes a SurrealDB SELECT operation on the target table or record.
func (c *DatabaseClient) Select(ctx context.Context, target string) (any, error) {
	db, err := c.requireDB()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Select[any](ctx, db, target)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "select %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Create inserts a new record and returns the server response.
func (c *DatabaseClient) Create(ctx context.Context, target string, data any) (any, error) {
	db, err := c.requireDB()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Create[any](ctx, db, target, data)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "create %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Update replaces a record (PUT semantics).
func (c *DatabaseClient) Update(ctx context.Context, target string, data any) (any, error) {
	db, err := c.requireDB()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Update[any](ctx, db, target, data)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "update %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Merge performs a PATCH-style merge on a target record or table.
func (c *DatabaseClient) Merge(ctx context.Context, target string, data any) (any, error) {
	db, err := c.requireDB()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Merge[any](ctx, db, target, data)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "merge %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Delete removes a record or a whole table.
func (c *DatabaseClient) Delete(ctx context.Context, target string) (any, error) {
	db, err := c.requireDB()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Delete[any](ctx, db, target)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "delete %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Health performs a lightweight RPC (SDK version probe) to check the
// connection is responsive. Returns (true, nil) on success.
func (c *DatabaseClient) Health(ctx context.Context) (bool, error) {
	db, err := c.requireDB()
	if err != nil {
		return false, err
	}
	if _, err := db.Version(ctx); err != nil {
		return false, surqlerrors.Wrap(surqlerrors.ErrConnection, "health check failed", err)
	}
	return true, nil
}

// DB exposes the underlying *surrealdb.DB for advanced callers that need
// direct SDK access (e.g. live-query primitives). The pointer is only valid
// while the client remains connected.
func (c *DatabaseClient) DB() *surrealdb.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.db
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (c *DatabaseClient) requireDB() (*surrealdb.DB, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.connected || c.db == nil {
		return nil, surqlerrors.New(surqlerrors.ErrConnection, "client is not connected")
	}
	return c.db, nil
}

// toSdkAuthPayload converts our Credentials interface into the key schema the
// surrealdb-go SDK expects (NS/DB/AC/user/pass). ScopeCredentials flattens the
// Variables map at the top level (SurrealDB accepts arbitrary scope vars there).
func toSdkAuthPayload(creds Credentials) map[string]any {
	switch v := creds.(type) {
	case RootCredentials:
		return map[string]any{
			"user": v.Username,
			"pass": v.Password,
		}
	case NamespaceCredentials:
		return map[string]any{
			"NS":   v.Namespace,
			"user": v.Username,
			"pass": v.Password,
		}
	case DatabaseCredentials:
		return map[string]any{
			"NS":   v.Namespace,
			"DB":   v.Database,
			"user": v.Username,
			"pass": v.Password,
		}
	case ScopeCredentials:
		out := map[string]any{
			"NS": v.Namespace,
			"DB": v.Database,
			"AC": v.Access,
		}
		for k, val := range v.Variables {
			out[k] = val
		}
		return out
	default:
		// Fall back to the generic payload if a consumer adds a new kind.
		return creds.ToSigninPayload()
	}
}

// backoffDelay returns the waiting duration before the next retry attempt,
// using exponential growth (base * multiplier^(attempt-1)) clamped between
// minWait and maxWait. All units are seconds (matching ConnectionConfig).
func backoffDelay(attempt uint32, minWait, maxWait, multiplier float64) time.Duration {
	if multiplier < 1 {
		multiplier = 1
	}
	if minWait < 0 {
		minWait = 0
	}
	if maxWait < minWait {
		maxWait = minWait
	}

	exp := math.Pow(multiplier, float64(attempt-1))
	delay := minWait * exp
	if math.IsInf(delay, 0) || math.IsNaN(delay) || delay > maxWait {
		delay = maxWait
	}
	if delay < minWait {
		delay = minWait
	}
	return time.Duration(delay * float64(time.Second))
}

// mapQueryError classifies SDK errors into the correct surql error kind. The
// SDK surfaces QueryError as a struct whose Is() method recognises itself;
// everything else is mapped to ErrQuery by default, except obvious context
// cancellations.
func mapQueryError(err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := context.Cause(context.Background()); ctxErr != nil && errors.Is(err, ctxErr) {
		return surqlerrors.Wrap(surqlerrors.ErrConnection, "query cancelled", err)
	}
	var qe *surrealdb.QueryError
	if errors.As(err, &qe) {
		return surqlerrors.Wrap(surqlerrors.ErrQuery, qe.Message, err)
	}
	return surqlerrors.Wrap(surqlerrors.ErrQuery, "query failed", err)
}
