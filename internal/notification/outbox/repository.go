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

// Repository defines the read/claim-side of the domain_outbox table.
// The Worker depends on this interface; the PostgreSQL implementation is in
// the same package.  Callers that need a testable stub can supply a mock.
type Repository interface {
	// ClaimBatch atomically transitions up to limit pending rows to
	// 'processing', sets locked_until = NOW() + lockDuration, increments
	// attempts, and returns the claimed entries.
	// Uses SELECT FOR UPDATE SKIP LOCKED so multiple worker replicas process
	// disjoint batches without blocking each other.
	ClaimBatch(ctx context.Context, limit int, lockDuration time.Duration) ([]*notification.OutboxEntry, error)

	// MarkDone transitions the row to 'done' and records processed_at.
	MarkDone(ctx context.Context, id int64) error

	// MarkFailed transitions the row to 'failed', records error_detail, and
	// sets processed_at.  Called when attempts == max_attempts.
	MarkFailed(ctx context.Context, id int64, errorDetail string) error

	// Schedule transitions the row back to 'pending' with a new scheduled_at.
	// Used by the worker to implement exponential-backoff retry before the
	// row reaches max_attempts.
	Schedule(ctx context.Context, id int64, scheduledAt time.Time) error

	// UnlockStale reclaims rows that have been in 'processing' status past
	// their locked_until timestamp.  This recovers entries left behind by a
	// crashed worker instance.  Returns the number of rows unlocked.
	UnlockStale(ctx context.Context) (int64, error)
}

// -- PostgreSQL implementation --

const staleThreshold = 10 * time.Minute

// postgresRepository is the PostgreSQL-backed implementation of Repository.
type postgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository constructs a PostgreSQL-backed Repository.
func NewPostgresRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) ClaimBatch(ctx context.Context, limit int, lockDuration time.Duration) ([]*notification.OutboxEntry, error) {
	// The CTE selects candidate IDs with SKIP LOCKED to avoid contention
	// between concurrent worker instances, then the UPDATE claims them
	// atomically.
	const q = `
WITH candidates AS (
    SELECT id
    FROM   domain_outbox
    WHERE  status = 'pending'
      AND  scheduled_at <= NOW()
    ORDER  BY scheduled_at
    LIMIT  $1
    FOR UPDATE SKIP LOCKED
)
UPDATE domain_outbox o
SET    status       = 'processing',
       locked_until = NOW() + $2::interval,
       attempts     = attempts + 1
FROM   candidates
WHERE  o.id = candidates.id
RETURNING
    o.id, o.event_type, o.aggregate_id, o.aggregate_type,
    o.payload, o.status, o.attempts, o.max_attempts,
    o.scheduled_at, o.locked_until, o.processed_at,
    o.error_detail, o.created_at
`
	lockInterval := lockDuration.String()
	rows, err := r.pool.Query(ctx, q, limit, lockInterval)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (r *postgresRepository) MarkDone(ctx context.Context, id int64) error {
	const q = `
UPDATE domain_outbox
SET    status       = 'done',
       processed_at = NOW(),
       locked_until = NULL
WHERE  id = $1
`
	return r.execOne(ctx, q, id)
}

func (r *postgresRepository) MarkFailed(ctx context.Context, id int64, errorDetail string) error {
	const q = `
UPDATE domain_outbox
SET    status        = 'failed',
       error_detail  = $2,
       processed_at  = NOW(),
       locked_until  = NULL
WHERE  id = $1
`
	return r.execOne(ctx, q, id, errorDetail)
}

func (r *postgresRepository) Schedule(ctx context.Context, id int64, scheduledAt time.Time) error {
	const q = `
UPDATE domain_outbox
SET    status       = 'pending',
       scheduled_at = $2,
       locked_until = NULL
WHERE  id = $1
`
	return r.execOne(ctx, q, id, scheduledAt)
}

func (r *postgresRepository) UnlockStale(ctx context.Context) (int64, error) {
	const q = `
UPDATE domain_outbox
SET    status       = 'pending',
       locked_until = NULL
WHERE  status       = 'processing'
  AND  locked_until < NOW() - $1::interval
`
	tag, err := r.pool.Exec(ctx, q, staleThreshold.String())
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

// execOne runs a single-row UPDATE and returns Internal when the query fails.
// It does NOT check for zero rows affected — the outbox worker never retries
// a not-found row; the row either exists or the worker has already processed it.
func (r *postgresRepository) execOne(ctx context.Context, q string, args ...any) error {
	if _, err := r.pool.Exec(ctx, q, args...); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

// scanEntries reads all rows returned by a ClaimBatch query into OutboxEntry
// slice.
func scanEntries(rows pgx.Rows) ([]*notification.OutboxEntry, error) {
	var out []*notification.OutboxEntry
	for rows.Next() {
		e := &notification.OutboxEntry{}
		var eventType string
		var status string
		if err := rows.Scan(
			&e.ID, &eventType, &e.AggregateID, &e.AggregateType,
			&e.Payload, &status, &e.Attempts, &e.MaxAttempts,
			&e.ScheduledAt, &e.LockedUntil, &e.ProcessedAt,
			&e.ErrorDetail, &e.CreatedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		e.EventType = notification.EventType(eventType)
		e.Status = notification.OutboxStatus(status)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.Internal(err)
	}
	return out, nil
}
