// Package outbox provides the write-side (Writer) and read/claim-side
// (Repository, Worker) of the transactional outbox for the notification
// subsystem.
//
// The canonical usage pattern is:
//
//  1. A service method opens (or reuses) a pgx.Tx for its domain write.
//  2. Before committing, it calls Writer.WriteInTx so that the outbox row is
//     durable in the same atomic commit.
//  3. The Worker polls domain_outbox, claims rows, dispatches to channels,
//     and marks each row done or failed.
//
// WriteInTx is the preferred path.  Writer.Write (pool-level, best-effort) is
// provided for callers whose repository layer does not expose its internal
// transaction; in that case a tiny window exists between the domain commit and
// the outbox insert where a crash would silently drop the event.
package outbox

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const insertOutboxSQL = `
INSERT INTO domain_outbox
    (event_type, aggregate_type, aggregate_id, payload, scheduled_at)
VALUES ($1, $2, $3, $4, $5)
`

// Writer is the write-side of the transactional outbox.
// Construct one with NewWriter and share it across services; it is safe for
// concurrent use.
type Writer struct {
	pool *pgxpool.Pool
}

// NewWriter constructs a Writer backed by the given connection pool.
func NewWriter(pool *pgxpool.Pool) *Writer {
	return &Writer{pool: pool}
}

// WriteInTx inserts an outbox row using the provided pgx.Tx, making the
// notification intent durable within the same atomic commit as the domain
// change.  This is the preferred path for true at-least-once delivery.
//
// eventType identifies the notification event (see notification.EventType).
// aggregateType and aggregateID describe the entity that changed (e.g.
// "withdrawal", "42").  payload must be JSON-serialisable; use one of the
// typed payload structs from the notification package.
func (w *Writer) WriteInTx(
	ctx context.Context,
	tx pgx.Tx,
	eventType notification.EventType,
	aggregateType, aggregateID string,
	payload any,
) error {
	entry, err := notification.NewOutboxEntry(eventType, aggregateType, aggregateID, payload)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, insertOutboxSQL,
		string(entry.EventType),
		entry.AggregateType,
		entry.AggregateID,
		entry.Payload,
		entry.ScheduledAt,
	); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

// Write opens a short-lived connection from the pool and inserts an outbox row
// outside of any existing transaction.
//
// Use this path only when the calling service does not expose its internal
// pgx.Tx.  If the process crashes between the domain commit and this insert the
// event is silently lost.  Prefer WriteInTx wherever possible.
func (w *Writer) Write(
	ctx context.Context,
	eventType notification.EventType,
	aggregateType, aggregateID string,
	payload any,
) error {
	entry, err := notification.NewOutboxEntry(eventType, aggregateType, aggregateID, payload)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := w.pool.Exec(writeCtx, insertOutboxSQL,
		string(entry.EventType),
		entry.AggregateType,
		entry.AggregateID,
		entry.Payload,
		entry.ScheduledAt,
	); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}
