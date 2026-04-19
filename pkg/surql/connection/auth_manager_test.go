package connection

import (
	"context"
	"errors"
	"testing"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestNewAuthManager_RejectsNilClient(t *testing.T) {
	t.Parallel()

	_, err := NewAuthManager(nil)
	if err == nil {
		t.Fatal("NewAuthManager(nil) should error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}

func TestAuthManager_ZeroStateIsUnauthenticated(t *testing.T) {
	t.Parallel()

	// The client is only used for real signin; we test the pure-state
	// branches by providing a stub DatabaseClient (non-nil).
	mgr, err := NewAuthManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewAuthManager: %v", err)
	}
	if mgr.IsAuthenticated() {
		t.Fatal("zero-state AuthManager should not be authenticated")
	}
	if got := mgr.CurrentToken(); got != "" {
		t.Fatalf("CurrentToken = %q; want empty", got)
	}
	if got := mgr.AuthType(); got != "" {
		t.Fatalf("AuthType = %q; want empty", got)
	}
}

func TestAuthManager_SigninRejectsNilCredentials(t *testing.T) {
	t.Parallel()

	mgr, err := NewAuthManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewAuthManager: %v", err)
	}
	_, err = mgr.Signin(context.Background(), nil)
	if err == nil {
		t.Fatal("Signin(nil) should error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}

func TestAuthManager_AuthenticateRejectsEmptyToken(t *testing.T) {
	t.Parallel()

	mgr, err := NewAuthManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewAuthManager: %v", err)
	}
	err = mgr.Authenticate(context.Background(), "")
	if err == nil {
		t.Fatal("Authenticate(empty) should error")
	}
	if !errors.Is(err, surqlerrors.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}

func TestAuthManager_RefreshRequiresCachedToken(t *testing.T) {
	t.Parallel()

	mgr, err := NewAuthManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewAuthManager: %v", err)
	}
	err = mgr.Refresh(context.Background())
	if err == nil {
		t.Fatal("Refresh without a cached token should error")
	}
	if !errors.Is(err, surqlerrors.ErrContext) {
		t.Fatalf("err = %v; want ErrContext", err)
	}
}

func TestAuthManager_ConnectionFailuresPropagate(t *testing.T) {
	t.Parallel()

	// A disconnected client makes every delegated call return
	// ErrConnection without any network I/O, which is exactly what we want
	// for unit testing the AuthManager orchestration surface.
	mgr, err := NewAuthManager(&DatabaseClient{})
	if err != nil {
		t.Fatalf("NewAuthManager: %v", err)
	}

	creds := NewRootCredentials("root", "root")
	if _, err := mgr.Signin(context.Background(), creds); !errors.Is(err, surqlerrors.ErrConnection) {
		t.Fatalf("Signin on disconnected client err = %v; want ErrConnection", err)
	}
	if _, err := mgr.Signup(
		context.Background(),
		NewScopeCredentials("ns", "db", "user"),
	); !errors.Is(err, surqlerrors.ErrConnection) {
		t.Fatalf("Signup on disconnected client err = %v; want ErrConnection", err)
	}
	if err := mgr.Authenticate(context.Background(), "token"); !errors.Is(err, surqlerrors.ErrConnection) {
		t.Fatalf("Authenticate on disconnected client err = %v; want ErrConnection", err)
	}
	if err := mgr.Invalidate(context.Background()); !errors.Is(err, surqlerrors.ErrConnection) {
		t.Fatalf("Invalidate on disconnected client err = %v; want ErrConnection", err)
	}
}
