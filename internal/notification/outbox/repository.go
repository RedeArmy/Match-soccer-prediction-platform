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

	// CountPending returns the number of rows in 'pending' status that are due
	// for processing (scheduled_at <= NOW()).  Used to expose an outbox lag gauge.
	CountPending(ctx context.Context) (int64, error)

	// OldestPendingAgeSecs returns the age in seconds of the oldest pending-due
	// row (scheduled_at <= NOW()). Returns 0 when the queue is empty.
	// Used to expose a processing-lag gauge for alerting on worker stalls.
	OldestPendingAgeSecs(ctx context.Context) (float64, error)
}

// -- PostgreSQL implementation --

const defaultStaleThreshold = 10 * time.Minute

// RepositoryOption is a functional option for NewPostgresRepository.
type RepositoryOption func(*postgresRepository)

// WithStaleLockThreshold overrides the duration after which a 'processing' row
// is considered abandoned and reclaimed by UnlockStale. Defaults to 10 minutes.
func WithStaleLockThreshold(d time.Duration) RepositoryOption {
	return func(r *postgresRepository) { r.staleThreshold = d }
}

// postgresRepository is the PostgreSQL-backed implementation of Repository.
type postgresRepository struct {
	pool           *pgxpool.Pool
	staleThreshold time.Duration
}

// NewPostgresRepository constructs a PostgreSQL-backed Repository.
// Callers may pass RepositoryOption values to override defaults.
func NewPostgresRepository(pool *pgxpool.Pool, opts ...RepositoryOption) Repository {
	r := &postgresRepository{pool: pool, staleThreshold: defaultStaleThreshold}
	for _, o := range opts {
		o(r)
	}
	return r
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
	tag, err := r.pool.Exec(ctx, q, r.staleThreshold.String())
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

func (r *postgresRepository) CountPending(ctx context.Context) (int64, error) {
	const q = `
SELECT COUNT(*)
FROM   domain_outbox
WHERE  status       = 'pending'
  AND  scheduled_at <= NOW()
`
	var n int64
	if err := r.pool.QueryRow(ctx, q).Scan(&n); err != nil {
		return 0, apperrors.Internal(err)
	}
	return n, nil
}

func (r *postgresRepository) OldestPendingAgeSecs(ctx context.Context) (float64, error) {
	const q = `
SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(scheduled_at))), 0)
FROM   domain_outbox
WHERE  status       = 'pending'
  AND  scheduled_at <= NOW()
`
	var age float64
	if err := r.pool.QueryRow(ctx, q).Scan(&age); err != nil {
		return 0, apperrors.Internal(err)
	}
	return age, nil
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
