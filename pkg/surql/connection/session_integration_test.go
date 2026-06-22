//go:build integration
// +build integration

package connection

import (
	"context"
	"testing"
	"time"
)

// requireWebSocket skips when the integration URL is not a WebSocket endpoint,
// since sessions are WebSocket-only.
func requireWebSocket(t *testing.T, client *DatabaseClient) {
	t.Helper()
	proto, err := client.Config().Protocol()
	if err != nil {
		t.Fatalf("Protocol: %v", err)
	}
	if proto != ProtocolWebSocket && proto != ProtocolWebSocketSecure {
		t.Skipf("sessions require ws:// / wss://; SURREAL_URL is %q", proto)
	}
}

func TestIntegration_SessionAttachQueryDetach(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	requireWebSocket(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := client.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer func() { _ = sess.Detach(ctx) }()

	// A fresh session is unauthenticated; sign in and select ns/db.
	user := envOr("SURREAL_USER", "root")
	pass := envOr("SURREAL_PASS", "root")
	if _, err := sess.Signin(ctx, NewRootCredentials(user, pass)); err != nil {
		t.Fatalf("session Signin: %v", err)
	}
	if err := sess.Use(ctx, "surqlgo_test", "session"); err != nil {
		t.Fatalf("session Use: %v", err)
	}

	res, err := sess.Query(ctx, "RETURN 40 + 2;")
	if err != nil {
		t.Fatalf("session Query: %v", err)
	}
	list, ok := res.([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("unexpected query response: %+v", res)
	}

	// CRUD via the session.
	table := "surqlgo_session_crud"
	_, _ = sess.Delete(ctx, table)
	if _, err := sess.Create(ctx, table, map[string]any{"name": "dave"}); err != nil {
		t.Fatalf("session Create: %v", err)
	}
	got, err := sess.Select(ctx, table)
	if err != nil {
		t.Fatalf("session Select: %v", err)
	}
	if got == nil {
		t.Error("session Select returned nil")
	}
	_, _ = sess.Delete(ctx, table)

	if err := sess.Detach(ctx); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	// Operations after Detach must fail with ErrConnection.
	if _, err := sess.Query(ctx, "RETURN 1;"); err == nil {
		t.Error("Query after Detach should fail")
	}
}

func TestIntegration_MultipleSessionsIsolated(t *testing.T) {
	client, cleanup := newIntegrationClient(t)
	defer cleanup()
	requireWebSocket(t, client)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	user := envOr("SURREAL_USER", "root")
	pass := envOr("SURREAL_PASS", "root")

	s1, err := client.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach s1: %v", err)
	}
	defer func() { _ = s1.Detach(ctx) }()
	s2, err := client.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach s2: %v", err)
	}
	defer func() { _ = s2.Detach(ctx) }()

	for _, s := range []*Session{s1, s2} {
		if _, err := s.Signin(ctx, NewRootCredentials(user, pass)); err != nil {
			t.Fatalf("Signin: %v", err)
		}
	}

	// Point each session at a different namespace/database; a SET in one must
	// not leak into the other.
	if err := s1.Use(ctx, "surqlgo_test", "sess_a"); err != nil {
		t.Fatalf("s1 Use: %v", err)
	}
	if err := s2.Use(ctx, "surqlgo_test", "sess_b"); err != nil {
		t.Fatalf("s2 Use: %v", err)
	}

	if _, err := s1.QueryWithVars(ctx, "LET $marker = 'a';", nil); err != nil {
		t.Fatalf("s1 LET: %v", err)
	}
	// s2 should not see s1's session variable.
	res, err := s2.Query(ctx, "RETURN $marker ?? 'unset';")
	if err != nil {
		t.Fatalf("s2 Query: %v", err)
	}
	if list, ok := res.([]any); ok && len(list) > 0 {
		if env, ok := list[0].(map[string]any); ok {
			if env["result"] == "a" {
				t.Error("session variable leaked across sessions")
			}
		}
	}
}
