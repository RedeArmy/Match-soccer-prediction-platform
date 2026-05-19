package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

type postgresUserNotificationRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresUserNotificationRepository constructs a PostgreSQL-backed
// UserNotificationRepository.
func NewPostgresUserNotificationRepository(pool *pgxpool.Pool) UserNotificationRepository {
	return &postgresUserNotificationRepository{pool: pool}
}

func (r *postgresUserNotificationRepository) Create(ctx context.Context, n *domain.UserNotification) (bool, error) {
	const q = `
INSERT INTO notifications (user_id, event_type, title, body, action_url, metadata, idempotency_key)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''))
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING id, created_at
`
	meta, err := json.Marshal(n.Metadata)
	if err != nil {
		return false, apperrors.Internal(err)
	}

	var id int64
	var createdAt time.Time
	err = r.pool.QueryRow(ctx, q,
		n.UserID, n.EventType, n.Title, n.Body,
		n.ActionURL, meta, n.IdempotencyKey,
	).Scan(&id, &createdAt)
	if err != nil {
		// pgx returns ErrNoRows when ON CONFLICT DO NOTHING suppresses the INSERT.
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, apperrors.Internal(err)
	}
	n.ID = id
	n.CreatedAt = createdAt
	return true, nil
}

func (r *postgresUserNotificationRepository) List(ctx context.Context, userID, limit, offset int, unreadOnly bool) ([]*domain.UserNotification, error) {
	q := `
SELECT id, user_id, event_type, title, body,
       COALESCE(action_url, ''), metadata, COALESCE(idempotency_key, ''),
       read_at, created_at
FROM notifications
WHERE user_id = $1`
	if unreadOnly {
		q += ` AND read_at IS NULL`
	}
	q += ` ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var out []*domain.UserNotification
	for rows.Next() {
		n := &domain.UserNotification{}
		var rawMeta []byte
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.EventType, &n.Title, &n.Body,
			&n.ActionURL, &rawMeta, &n.IdempotencyKey,
			&n.ReadAt, &n.CreatedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		if len(rawMeta) > 0 {
			_ = json.Unmarshal(rawMeta, &n.Metadata)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return out, nil
}

func (r *postgresUserNotificationRepository) CountUnread(ctx context.Context, userID int) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return count, nil
}

func (r *postgresUserNotificationRepository) MarkRead(ctx context.Context, notificationID int64, userID int) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		notificationID, userID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("notification not found or already read")
	}
	return nil
}

func (r *postgresUserNotificationRepository) MarkAllRead(ctx context.Context, userID int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

var _ UserNotificationRepository = (*postgresUserNotificationRepository)(nil)
