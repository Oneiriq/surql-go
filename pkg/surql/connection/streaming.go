package connection

import (
	"context"
	"sync"

	surqlerrors "github.com/albedosehen/surql-go/pkg/surql/errors"
	surrealdb "github.com/surrealdb/surrealdb.go"
	sdkconn "github.com/surrealdb/surrealdb.go/pkg/connection"
	"github.com/surrealdb/surrealdb.go/pkg/models"
)

// LiveNotification is the event emitted by a LiveQuery subscription. It is a
// thin alias over the SDK struct so callers don't have to import the SDK
// directly for basic subscription use.
type LiveNotification = sdkconn.Notification

// LiveAction is the string identifier of a live-query action (CREATE/UPDATE/DELETE).
type LiveAction = sdkconn.Action

// LiveQuery wraps a SurrealDB live-query subscription: the server-side UUID,
// the table being watched, and a consumer-facing Go channel.
//
// Close must be called when the consumer no longer needs notifications; this
// sends KILL via the SDK and releases the notification channel. Close is
// idempotent and safe to call from any goroutine.
type LiveQuery struct {
	client  *DatabaseClient
	id      *models.UUID
	table   string
	diff    bool
	notifs  <-chan LiveNotification
	closeMu sync.Mutex
	closed  bool
}

// Live starts a LIVE SELECT subscription on the given table and returns a
// LiveQuery wrapping the SDK's notification channel.
//
// HTTP/HTTPS connections are rejected with ErrValidation because SurrealDB's
// live-query protocol is WebSocket-only. Memory / file / surrealkv engines
// technically support live queries but are not built into the SDK we consume,
// so this MVP also rejects non-WebSocket transports.
func (c *DatabaseClient) Live(ctx context.Context, table string, diff bool) (*LiveQuery, error) {
	if table == "" {
		return nil, surqlerrors.New(surqlerrors.ErrValidation, "table must not be empty")
	}

	proto, err := c.cfg.Protocol()
	if err != nil {
		return nil, err
	}
	if proto == ProtocolHTTP || proto == ProtocolHTTPS {
		return nil, surqlerrors.Newf(surqlerrors.ErrValidation,
			"live queries are not supported on %s transport", proto)
	}

	db, err := c.requireDB()
	if err != nil {
		return nil, err
	}

	id, err := surrealdb.Live(ctx, db, models.Table(table), diff)
	if err != nil {
		return nil, surqlerrors.Wrapf(surqlerrors.ErrStreaming, err, "live(%q) failed", table)
	}

	ch, err := db.LiveNotifications(id.String())
	if err != nil {
		// Best-effort cleanup if we can't grab the notification channel.
		_ = surrealdb.Kill(ctx, db, id.String())
		return nil, surqlerrors.Wrapf(surqlerrors.ErrStreaming, err,
			"unable to subscribe to notifications for %q", table)
	}

	return &LiveQuery{
		client: c,
		id:     id,
		table:  table,
		diff:   diff,
		notifs: ch,
	}, nil
}

// ID returns the SurrealDB-issued UUID for this subscription.
func (l *LiveQuery) ID() string {
	if l == nil || l.id == nil {
		return ""
	}
	return l.id.String()
}

// Table returns the table the subscription is watching.
func (l *LiveQuery) Table() string { return l.table }

// Diff reports whether diff mode was requested for this subscription.
func (l *LiveQuery) Diff() bool { return l.diff }

// Notifications returns the read-only channel of events. The channel closes
// automatically when Close is called (or when the server terminates the
// subscription).
func (l *LiveQuery) Notifications() <-chan LiveNotification { return l.notifs }

// Close sends KILL to the server and releases the notification channel. Safe
// to call multiple times: subsequent calls return nil.
func (l *LiveQuery) Close(ctx context.Context) error {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true

	db := l.client.DB()
	if db == nil {
		return nil
	}
	if err := surrealdb.Kill(ctx, db, l.id.String()); err != nil {
		return surqlerrors.Wrap(surqlerrors.ErrStreaming, "kill failed", err)
	}
	return nil
}
