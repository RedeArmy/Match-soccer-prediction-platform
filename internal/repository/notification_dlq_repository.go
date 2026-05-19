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

var _ NotificationDLQRepository = (*postgresNotificationDLQRepository)(nil)
