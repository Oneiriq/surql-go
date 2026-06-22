package connection

import (
	"context"
	stdErrors "errors"
	"testing"

	surrealdb "github.com/surrealdb/surrealdb.go"

	surqlerrors "github.com/Oneiriq/surql-go/pkg/surql/errors"
)

func TestQueryResultsToEnvelopes(t *testing.T) {
	// nil pointer -> non-nil empty slice.
	if got := queryResultsToEnvelopes(nil); got == nil || len(got) != 0 {
		t.Errorf("nil results = %v, want empty non-nil slice", got)
	}

	results := []surrealdb.QueryResult[any]{
		{Status: "OK", Time: "1ms", Result: 42},
		{Status: "ERR", Time: "2ms", Result: nil},
	}
	got := queryResultsToEnvelopes(&results)
	if len(got) != 2 {
		t.Fatalf("got %d envelopes, want 2", len(got))
	}
	first, ok := got[0].(map[string]any)
	if !ok {
		t.Fatalf("envelope type = %T", got[0])
	}
	if first["status"] != "OK" || first["time"] != "1ms" || first["result"] != 42 {
		t.Errorf("first envelope = %v", first)
	}
	second := got[1].(map[string]any)
	if second["status"] != "ERR" || second["result"] != nil {
		t.Errorf("second envelope = %v", second)
	}
}

func TestAttach_NotConnected(t *testing.T) {
	c := &DatabaseClient{}
	_, err := c.Attach(context.Background())
	if err == nil {
		t.Fatal("Attach on disconnected client should error")
	}
	if !stdErrors.Is(err, surqlerrors.ErrConnection) {
		t.Errorf("error kind = %v, want ErrConnection", err)
	}
}

// TestSession_DetachedGuards verifies every operation on a detached session
// returns ErrConnection rather than dereferencing a nil SDK session. A
// zero-value Session has session == nil, which requireSession treats as
// detached.
func TestSession_DetachedGuards(t *testing.T) {
	ctx := context.Background()
	s := &Session{detached: true}

	checks := []struct {
		name string
		err  error
	}{
		{"Use", s.Use(ctx, "ns", "db")},
		{"Authenticate", s.Authenticate(ctx, "tok")},
		{"Invalidate", s.Invalidate(ctx)},
	}
	for _, c := range checks {
		if c.err == nil {
			t.Errorf("%s on detached session should error", c.name)
			continue
		}
		if !stdErrors.Is(c.err, surqlerrors.ErrConnection) {
			t.Errorf("%s error kind = %v, want ErrConnection", c.name, c.err)
		}
	}

	if _, err := s.Signin(ctx, NewRootCredentials("root", "root")); !stdErrors.Is(err, surqlerrors.ErrConnection) {
		t.Errorf("Signin error = %v, want ErrConnection", err)
	}
	if _, err := s.Query(ctx, "RETURN 1;"); !stdErrors.Is(err, surqlerrors.ErrConnection) {
		t.Errorf("Query error = %v, want ErrConnection", err)
	}
	if _, err := s.Select(ctx, "t"); !stdErrors.Is(err, surqlerrors.ErrConnection) {
		t.Errorf("Select error = %v, want ErrConnection", err)
	}
	if _, err := s.Create(ctx, "t", nil); !stdErrors.Is(err, surqlerrors.ErrConnection) {
		t.Errorf("Create error = %v, want ErrConnection", err)
	}
	if _, err := s.Update(ctx, "t", nil); !stdErrors.Is(err, surqlerrors.ErrConnection) {
		t.Errorf("Update error = %v, want ErrConnection", err)
	}
	if _, err := s.Merge(ctx, "t", nil); !stdErrors.Is(err, surqlerrors.ErrConnection) {
		t.Errorf("Merge error = %v, want ErrConnection", err)
	}
	if _, err := s.Delete(ctx, "t"); !stdErrors.Is(err, surqlerrors.ErrConnection) {
		t.Errorf("Delete error = %v, want ErrConnection", err)
	}
}

func TestSession_Signin_NilCreds(t *testing.T) {
	s := &Session{}
	if _, err := s.Signin(context.Background(), nil); !stdErrors.Is(err, surqlerrors.ErrValidation) {
		t.Errorf("Signin(nil) error = %v, want ErrValidation", err)
	}
}

func TestSession_Detach_Idempotent(t *testing.T) {
	s := &Session{detached: true}
	if err := s.Detach(context.Background()); err != nil {
		t.Errorf("Detach on already-detached session = %v, want nil", err)
	}
	if !s.IsDetached() {
		t.Error("IsDetached() = false after Detach")
	}
}

func TestSession_BucketAccessor(t *testing.T) {
	s := &Session{}
	h := s.Bucket("assets")
	if h.Name() != "assets" {
		t.Errorf("Name() = %q", h.Name())
	}
}

func TestSession_TokenAndAuthType_ZeroValue(t *testing.T) {
	s := &Session{}
	if s.CurrentToken() != "" {
		t.Errorf("CurrentToken() = %q, want empty", s.CurrentToken())
	}
	if s.CurrentAuthType() != "" {
		t.Errorf("CurrentAuthType() = %q, want empty", s.CurrentAuthType())
	}
}
