package connection

import (
	"context"
	"sync"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

// AuthManager is a thin orchestrator over a DatabaseClient's
// signin/signup/authenticate/invalidate lifecycle. It tracks the most recent
// token and auth level so downstream helpers can inspect auth state without
// threading it through the call stack. Mirrors surql-py's
// `connection.auth.AuthManager`.
//
// All methods are safe for concurrent use. The underlying client drives the
// actual I/O; this type only manages cached auth bookkeeping under its own
// mutex so the client's lock isn't held across higher-level orchestration.
type AuthManager struct {
	client *DatabaseClient

	mu       sync.RWMutex
	token    string
	authType AuthType
}

// NewAuthManager wraps client so consumers can call Signin/Signup/etc. off
// the manager. The client must be non-nil and should generally be connected
// before orchestrating auth; callers remain responsible for Connect/Disconnect.
func NewAuthManager(client *DatabaseClient) (*AuthManager, error) {
	if client == nil {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "client cannot be nil")
	}
	return &AuthManager{client: client}, nil
}

// Signin authenticates the underlying client with the supplied credentials
// and caches the resulting JWT + auth level. Returns the TokenAuth yielded
// by the client for callers that want to persist it externally.
func (a *AuthManager) Signin(ctx context.Context, creds Credentials) (TokenAuth, error) {
	if creds == nil {
		return TokenAuth{}, surqlerrors.New(surqlerrors.ErrValidation, "credentials cannot be nil")
	}
	token, err := a.client.Signin(ctx, creds)
	if err != nil {
		return TokenAuth{}, err
	}
	a.mu.Lock()
	a.token = token.Token
	a.authType = creds.AuthType()
	a.mu.Unlock()
	return token, nil
}

// Signup registers a new record-level user with the supplied scope
// credentials and caches the resulting JWT. The auth level is always
// AuthScope because signup is only valid at scope/record level.
func (a *AuthManager) Signup(ctx context.Context, creds ScopeCredentials) (TokenAuth, error) {
	token, err := a.client.Signup(ctx, creds)
	if err != nil {
		return TokenAuth{}, err
	}
	a.mu.Lock()
	a.token = token.Token
	a.authType = AuthScope
	a.mu.Unlock()
	return token, nil
}

// Authenticate binds an existing JWT to the underlying session. The cached
// token is updated so subsequent CurrentToken calls reflect the new value,
// but the cached auth type is left untouched because the JWT alone does not
// tell us which level minted it.
func (a *AuthManager) Authenticate(ctx context.Context, token string) error {
	if token == "" {
		return surqlerrors.New(surqlerrors.ErrValidation, "token cannot be empty")
	}
	if err := a.client.Authenticate(ctx, token); err != nil {
		return err
	}
	a.mu.Lock()
	a.token = token
	a.mu.Unlock()
	return nil
}

// Invalidate drops the current session's authentication state on both
// client and server, and clears the cached token/auth type.
func (a *AuthManager) Invalidate(ctx context.Context) error {
	if err := a.client.Invalidate(ctx); err != nil {
		return err
	}
	a.mu.Lock()
	a.token = ""
	a.authType = ""
	a.mu.Unlock()
	return nil
}

// Refresh re-authenticates the current session using the cached token. It
// is a thin convenience over Authenticate for long-running consumers that
// periodically need to nudge the session. Returns ErrContext when no
// token has been cached yet (i.e. the manager was never signed in).
func (a *AuthManager) Refresh(ctx context.Context) error {
	a.mu.RLock()
	token := a.token
	a.mu.RUnlock()
	if token == "" {
		return surqlerrors.New(
			surqlerrors.ErrContext,
			"no cached token to refresh; sign in first",
		)
	}
	return a.client.Authenticate(ctx, token)
}

// CurrentToken returns the most recently cached JWT (empty string when the
// manager is unauthenticated).
func (a *AuthManager) CurrentToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.token
}

// AuthType returns the cached auth level of the most recent successful
// Signin/Signup call (empty string when unauthenticated).
func (a *AuthManager) AuthType() AuthType {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.authType
}

// IsAuthenticated reports whether a cached token is present.
func (a *AuthManager) IsAuthenticated() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.token != ""
}
