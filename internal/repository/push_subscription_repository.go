package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

type postgresPushSubscriptionRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresPushSubscriptionRepository constructs a PostgreSQL-backed
// PushSubscriptionRepository.
func NewPostgresPushSubscriptionRepository(pool *pgxpool.Pool) PushSubscriptionRepository {
	return &postgresPushSubscriptionRepository{pool: pool}
}

func (r *postgresPushSubscriptionRepository) Create(ctx context.Context, sub *domain.PushSubscription) error {
	const q = `
INSERT INTO push_subscriptions (user_id, endpoint, p256dh_key, auth_key, user_agent, active)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), TRUE)
ON CONFLICT (endpoint) DO UPDATE
    SET user_id    = EXCLUDED.user_id,
        p256dh_key = EXCLUDED.p256dh_key,
        auth_key   = EXCLUDED.auth_key,
        user_agent = EXCLUDED.user_agent,
        active     = TRUE
RETURNING id, created_at
`
	return apperrors.Internal(r.pool.QueryRow(ctx, q,
		sub.UserID, sub.Endpoint, sub.P256dhKey, sub.AuthKey, sub.UserAgent,
	).Scan(&sub.ID, &sub.CreatedAt))
}

func (r *postgresPushSubscriptionRepository) ListActiveByUser(ctx context.Context, userID int) ([]*domain.PushSubscription, error) {
	const q = `
SELECT id, user_id, endpoint, p256dh_key, auth_key,
       COALESCE(user_agent, ''), active, created_at, last_used_at
FROM push_subscriptions
WHERE user_id = $1 AND active = TRUE
`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var out []*domain.PushSubscription
	for rows.Next() {
		s := &domain.PushSubscription{}
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.Endpoint, &s.P256dhKey, &s.AuthKey,
			&s.UserAgent, &s.Active, &s.CreatedAt, &s.LastUsedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return out, nil
}

func (r *postgresPushSubscriptionRepository) DeleteByEndpoint(ctx context.Context, endpoint string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM push_subscriptions WHERE endpoint = $1`,
		endpoint,
	)
	return apperrors.Internal(err)
}

func (r *postgresPushSubscriptionRepository) MarkInactive(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE push_subscriptions SET active = FALSE WHERE id = $1`,
		id,
	)
	return apperrors.Internal(err)
}

var _ PushSubscriptionRepository = (*postgresPushSubscriptionRepository)(nil)
