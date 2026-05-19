package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

type postgresAdminNotificationLogRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresAdminNotificationLogRepository constructs a PostgreSQL-backed
// AdminNotificationLogRepository.
func NewPostgresAdminNotificationLogRepository(pool *pgxpool.Pool) AdminNotificationLogRepository {
	return &postgresAdminNotificationLogRepository{pool: pool}
}

func (r *postgresAdminNotificationLogRepository) Create(ctx context.Context, entry *domain.AdminNotificationLog) error {
	const q = `
INSERT INTO admin_notification_log
    (event_type, recipients, subject, status, resend_msg_id, error_detail)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at
`
	var resendMsgID *string
	if entry.ResendMsgID != "" {
		resendMsgID = &entry.ResendMsgID
	}
	var errorDetail *string
	if entry.ErrorDetail != "" {
		errorDetail = &entry.ErrorDetail
	}

	return r.pool.QueryRow(ctx, q,
		entry.EventType,
		entry.Recipients,
		entry.Subject,
		string(entry.Status),
		resendMsgID,
		errorDetail,
	).Scan(&entry.ID, &entry.CreatedAt)
}

var _ AdminNotificationLogRepository = (*postgresAdminNotificationLogRepository)(nil)
