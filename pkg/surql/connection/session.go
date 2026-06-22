package connection

import (
	"context"
	"sync"

	surrealdb "github.com/surrealdb/surrealdb.go"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// Session is an additional authenticated context multiplexed over a single
// SurrealDB WebSocket connection (SurrealDB v3+ "multiple sessions"). It wraps
// the official SDK's *surrealdb.Session.
//
// A freshly attached session starts unauthenticated and without a selected
// namespace/database: call Signin (or Authenticate) and Use before issuing
// queries. Each session has its own auth state and scopes its own live
// notifications, independent of the parent DatabaseClient and of sibling
// sessions.
//
// Sessions are only supported on WebSocket transports (ws:// / wss://).
// Attaching over HTTP returns ErrConnection. A Session must be released exactly
// once via Detach; further use after Detach returns ErrConnection.
//
// Session mirrors the DatabaseClient query / CRUD surface. All methods are safe
// for concurrent use.
type Session struct {
	client  *DatabaseClient
	session *surrealdb.Session

	mu       sync.RWMutex
	detached bool
	token    string
	authType AuthType
}

// Attach opens a new session on the client's underlying WebSocket connection.
//
// It returns ErrConnection when the client is not connected or when the
// configured transport is not WebSocket (sessions require ws:// / wss://). The
// returned session is unauthenticated with no namespace/database selected.
func (c *DatabaseClient) Attach(ctx context.Context) (*Session, error) {
	db, err := c.requireDB()
	if err != nil {
		return nil, err
	}
	if proto, perr := c.cfg.Protocol(); perr == nil &&
		proto != ProtocolWebSocket && proto != ProtocolWebSocketSecure {
		return nil, surqlerrors.Newf(surqlerrors.ErrConnection,
			"sessions require a WebSocket connection (ws:// or wss://); current transport is %q", proto)
	}
	sess, err := db.Attach(ctx)
	if err != nil {
		return nil, surqlerrors.Wrap(surqlerrors.ErrConnection, "attach session failed", err)
	}
	return &Session{client: c, session: sess}, nil
}

// requireSession returns the underlying SDK session or ErrConnection when the
// session has been detached.
func (s *Session) requireSession() (*surrealdb.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.detached || s.session == nil {
		return nil, surqlerrors.New(surqlerrors.ErrConnection, "session has been detached")
	}
	return s.session, nil
}

// Use selects the namespace and database for this session.
func (s *Session) Use(ctx context.Context, namespace, database string) error {
	sess, err := s.requireSession()
	if err != nil {
		return err
	}
	if err := sess.Use(ctx, namespace, database); err != nil {
		return surqlerrors.Wrap(surqlerrors.ErrConnection, "session use failed", err)
	}
	return nil
}

// Signin authenticates this session with the supplied credentials, returning
// the issued JWT wrapped in a TokenAuth and caching it on the session.
func (s *Session) Signin(ctx context.Context, creds Credentials) (TokenAuth, error) {
	if creds == nil {
		return TokenAuth{}, surqlerrors.New(surqlerrors.ErrValidation, "credentials must not be nil")
	}
	sess, err := s.requireSession()
	if err != nil {
		return TokenAuth{}, err
	}
	token, err := sess.SignIn(ctx, toSdkAuthPayload(creds))
	if err != nil {
		return TokenAuth{}, surqlerrors.Wrap(surqlerrors.ErrConnection, "session signin failed", err)
	}
	s.mu.Lock()
	s.token = token
	s.authType = creds.AuthType()
	s.mu.Unlock()
	return TokenAuth{Token: token}, nil
}

// Authenticate authenticates this session with an existing JWT.
func (s *Session) Authenticate(ctx context.Context, token string) error {
	sess, err := s.requireSession()
	if err != nil {
		return err
	}
	if token == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "token must not be empty")
	}
	if err := sess.Authenticate(ctx, token); err != nil {
		return surqlerrors.Wrap(surqlerrors.ErrConnection, "session authenticate failed", err)
	}
	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
	return nil
}

// Invalidate drops this session's authentication state on both client and
// server. The session remains attached and may be re-authenticated.
func (s *Session) Invalidate(ctx context.Context) error {
	sess, err := s.requireSession()
	if err != nil {
		return err
	}
	if err := sess.Invalidate(ctx); err != nil {
		return surqlerrors.Wrap(surqlerrors.ErrConnection, "session invalidate failed", err)
	}
	s.mu.Lock()
	s.token = ""
	s.authType = ""
	s.mu.Unlock()
	return nil
}

// CurrentToken returns the most recent JWT issued to this session (empty when
// unauthenticated).
func (s *Session) CurrentToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

// CurrentAuthType returns the AuthType of the most recent successful session
// signin (empty when never authenticated).
func (s *Session) CurrentAuthType() AuthType {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authType
}

// Detach closes the session on the server, releasing its resources. Detach is
// idempotent: calling it on an already-detached session is a no-op.
func (s *Session) Detach(ctx context.Context) error {
	s.mu.Lock()
	if s.detached || s.session == nil {
		s.detached = true
		s.session = nil
		s.mu.Unlock()
		return nil
	}
	sess := s.session
	s.detached = true
	s.session = nil
	s.token = ""
	s.authType = ""
	s.mu.Unlock()

	if err := sess.Detach(ctx); err != nil {
		return surqlerrors.Wrap(surqlerrors.ErrConnection, "detach session failed", err)
	}
	return nil
}

// IsDetached reports whether Detach has been called.
func (s *Session) IsDetached() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.detached
}

// Query executes a raw SurrealQL query in this session without parameters.
func (s *Session) Query(ctx context.Context, surql string) (any, error) {
	return s.QueryWithVars(ctx, surql, nil)
}

// QueryWithVars executes a parameterised SurrealQL query in this session.
// Results preserve the per-statement response envelope, matching
// DatabaseClient.QueryWithVars.
func (s *Session) QueryWithVars(ctx context.Context, surql string, vars map[string]any) (any, error) {
	sess, err := s.requireSession()
	if err != nil {
		return nil, err
	}
	results, err := surrealdb.Query[any](ctx, sess, surql, vars)
	if err != nil {
		return nil, mapQueryError(err)
	}
	return queryResultsToEnvelopes(results), nil
}

// Select executes a SELECT against the target table or record in this session.
func (s *Session) Select(ctx context.Context, target string) (any, error) {
	sess, err := s.requireSession()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Select[any](ctx, sess, target)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "select %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Create inserts a new record in this session and returns the server response.
func (s *Session) Create(ctx context.Context, target string, data any) (any, error) {
	sess, err := s.requireSession()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Create[any](ctx, sess, target, data)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "create %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Update replaces a record (PUT semantics) in this session.
func (s *Session) Update(ctx context.Context, target string, data any) (any, error) {
	sess, err := s.requireSession()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Update[any](ctx, sess, target, data)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "update %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Merge performs a PATCH-style merge in this session.
func (s *Session) Merge(ctx context.Context, target string, data any) (any, error) {
	sess, err := s.requireSession()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Merge[any](ctx, sess, target, data)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "merge %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Delete removes a record or whole table in this session.
func (s *Session) Delete(ctx context.Context, target string) (any, error) {
	sess, err := s.requireSession()
	if err != nil {
		return nil, err
	}
	res, err := surrealdb.Delete[any](ctx, sess, target)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrQuery, err, "delete %q failed", target)
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

// Bucket returns an object-storage handle bound to name that operates within
// this session's authentication context.
func (s *Session) Bucket(name string) *SessionBucket {
	return &Bucket{runner: s, name: name}
}
