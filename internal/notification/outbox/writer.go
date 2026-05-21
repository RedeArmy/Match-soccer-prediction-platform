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
	"errors"
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

// insertOutboxDedupSQL inserts an outbox row with a dedup_key.  If a row with
// the same non-null dedup_key is already in the table the insert is a no-op
// (ON CONFLICT DO NOTHING) and no id is returned.  The partial unique index
// idx_outbox_dedup (WHERE dedup_key IS NOT NULL) satisfies the conflict target.
const insertOutboxDedupSQL = `
INSERT INTO domain_outbox
    (event_type, aggregate_type, aggregate_id, payload, scheduled_at, dedup_key)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (dedup_key) WHERE dedup_key IS NOT NULL DO NOTHING
RETURNING id
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

// BatchEvent is a single notification intent passed to WriteBatch.
type BatchEvent struct {
	EventType     notification.EventType
	AggregateType string
	AggregateID   string
	Payload       any
}

// WriteBatch atomically inserts all events in a single transaction acquired
// from the pool.  If any insert fails the whole batch is rolled back and no
// rows are written.
//
// Use WriteBatch when two correlated notifications — e.g. an admin alert and a
// user-facing event for the same business action — must either both appear in
// the outbox or neither does.  This eliminates the crash window that exists
// between two successive Write calls.
//
// For full end-to-end atomicity (domain write + outbox writes in the same
// commit) use WriteInTx with the caller's existing transaction instead.
func (w *Writer) WriteBatch(ctx context.Context, events []BatchEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return apperrors.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, e := range events {
		if err := w.WriteInTx(ctx, tx, e.EventType, e.AggregateType, e.AggregateID, e.Payload); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// WriteDedup inserts an outbox row with an explicit dedup_key.  If a row with
// the same key already exists in the table (regardless of its status) the
// insert is skipped and written=false is returned with a nil error.
//
// Use this method from scheduler jobs that may fire more than once within a
// single reminder window.  The dedupKey should encode the event's full
// uniqueness scope — e.g. "prediction.missing_reminder:match:42:user:99:b60"
// for the 60-minute bucket of a specific (match, user) pair — so that each
// logical send fires exactly once per window even if the scheduler restarts.
//
// The dedup protection covers the lifetime of the outbox row: once the row is
// purged from the table the same key can be re-used on a future send.
func (w *Writer) WriteDedup(
	ctx context.Context,
	dedupKey string,
	eventType notification.EventType,
	aggregateType, aggregateID string,
	payload any,
) (written bool, err error) {
	entry, err := notification.NewOutboxEntry(eventType, aggregateType, aggregateID, payload)
	if err != nil {
		return false, err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var id int64
	scanErr := w.pool.QueryRow(writeCtx, insertOutboxDedupSQL,
		string(entry.EventType),
		entry.AggregateType,
		entry.AggregateID,
		entry.Payload,
		entry.ScheduledAt,
		dedupKey,
	).Scan(&id)
	if errors.Is(scanErr, pgx.ErrNoRows) {
		return false, nil // conflict: existing row with same dedup_key
	}
	if scanErr != nil {
		return false, apperrors.Internal(scanErr)
	}
	return true, nil
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
