package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

type postgresNotificationDLQRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresNotificationDLQRepository constructs a PostgreSQL-backed
// NotificationDLQRepository.
func NewPostgresNotificationDLQRepository(pool *pgxpool.Pool) NotificationDLQRepository {
	return &postgresNotificationDLQRepository{pool: pool}
}

func (r *postgresNotificationDLQRepository) CreateEntry(ctx context.Context, entry *domain.NotificationDLQEntry) error {
	const q = `
INSERT INTO notification_dlq
    (outbox_id, channel, user_id, event_type, payload, error_detail)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at
`
	if err := r.pool.QueryRow(ctx, q,
		entry.OutboxID,
		entry.Channel,
		entry.UserID,
		entry.EventType,
		entry.Payload,
		entry.ErrorDetail,
	).Scan(&entry.ID, &entry.CreatedAt); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

func (r *postgresNotificationDLQRepository) ClaimBatch(ctx context.Context, limit, maxAttempts int) ([]*domain.NotificationDLQEntry, error) {
	const q = `
SELECT id, outbox_id, channel, user_id, event_type, payload, error_detail,
       attempts, created_at, last_retry_at, resolved_at
FROM notification_dlq
WHERE resolved_at IS NULL
  AND attempts < $2
  AND (last_retry_at IS NULL
       OR last_retry_at < NOW() - (INTERVAL '1 second' * POWER(2, attempts)))
ORDER BY created_at ASC
LIMIT $1
`
	rows, err := r.pool.Query(ctx, q, limit, maxAttempts)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var entries []*domain.NotificationDLQEntry
	for rows.Next() {
		e := &domain.NotificationDLQEntry{}
		if err := rows.Scan(
			&e.ID, &e.OutboxID, &e.Channel, &e.UserID, &e.EventType,
			&e.Payload, &e.ErrorDetail, &e.Attempts, &e.CreatedAt,
			&e.LastRetryAt, &e.ResolvedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		entries = append(entries, e)
	}
	return entries, apperrors.Internal(rows.Err())
}

func (r *postgresNotificationDLQRepository) MarkResolved(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notification_dlq SET resolved_at = NOW() WHERE id = $1`,
		id,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

func (r *postgresNotificationDLQRepository) RecordFailure(ctx context.Context, id int64, errDetail string) error {
	_, err := r.pool.Exec(ctx, `
UPDATE notification_dlq
SET attempts      = attempts + 1,
    last_retry_at = NOW(),
    error_detail  = $2
WHERE id = $1`,
		id, errDetail,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

func (r *postgresNotificationDLQRepository) CountUnresolved(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_dlq WHERE resolved_at IS NULL`,
	).Scan(&n)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return n, nil
}

var _ NotificationDLQRepository = (*postgresNotificationDLQRepository)(nil)
