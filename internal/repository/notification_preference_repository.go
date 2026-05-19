package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

type postgresNotificationPreferenceRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresNotificationPreferenceRepository constructs a PostgreSQL-backed
// NotificationPreferenceRepository.
func NewPostgresNotificationPreferenceRepository(pool *pgxpool.Pool) NotificationPreferenceRepository {
	return &postgresNotificationPreferenceRepository{pool: pool}
}

func (r *postgresNotificationPreferenceRepository) Get(ctx context.Context, userID int, eventType string) (*domain.NotificationPreference, error) {
	const q = `
SELECT user_id, event_type, channel_email, channel_push, channel_inapp, updated_at
FROM notification_preferences
WHERE user_id = $1 AND event_type = $2
`
	p := &domain.NotificationPreference{}
	err := r.pool.QueryRow(ctx, q, userID, eventType).Scan(
		&p.UserID, &p.EventType,
		&p.ChannelEmail, &p.ChannelPush, &p.ChannelInApp,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("notification preference not found")
		}
		return nil, apperrors.Internal(err)
	}
	return p, nil
}

func (r *postgresNotificationPreferenceRepository) ListByUser(ctx context.Context, userID int) ([]*domain.NotificationPreference, error) {
	const q = `
SELECT user_id, event_type, channel_email, channel_push, channel_inapp, updated_at
FROM notification_preferences
WHERE user_id = $1
ORDER BY event_type
`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var out []*domain.NotificationPreference
	for rows.Next() {
		p := &domain.NotificationPreference{}
		if err := rows.Scan(
			&p.UserID, &p.EventType,
			&p.ChannelEmail, &p.ChannelPush, &p.ChannelInApp,
			&p.UpdatedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return out, nil
}

func (r *postgresNotificationPreferenceRepository) Upsert(ctx context.Context, pref *domain.NotificationPreference) error {
	const q = `
INSERT INTO notification_preferences (user_id, event_type, channel_email, channel_push, channel_inapp, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT (user_id, event_type) DO UPDATE
    SET channel_email = EXCLUDED.channel_email,
        channel_push  = EXCLUDED.channel_push,
        channel_inapp = EXCLUDED.channel_inapp,
        updated_at    = NOW()
`
	_, err := r.pool.Exec(ctx, q,
		pref.UserID, pref.EventType,
		pref.ChannelEmail, pref.ChannelPush, pref.ChannelInApp,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

var _ NotificationPreferenceRepository = (*postgresNotificationPreferenceRepository)(nil)
