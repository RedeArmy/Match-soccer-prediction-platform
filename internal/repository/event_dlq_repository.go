package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresEventDLQRepository persists critical domain events that could not
// be written to the Redis dead-letter queue (e.g. during a Redis outage).
// It satisfies messaging.DLQFallback via structural typing so the messaging
// package does not import the repository package.
type PostgresEventDLQRepository struct {
	db *pgxpool.Pool
}

// NewPostgresEventDLQRepository constructs a PostgresEventDLQRepository.
func NewPostgresEventDLQRepository(db *pgxpool.Pool) *PostgresEventDLQRepository {
	return &PostgresEventDLQRepository{db: db}
}

// RecordDeadLettered inserts one event_dlq row. envelopeJSON is the full
// JSON-encoded dlqEntry produced by the messaging layer; it is stored as
// JSONB for structured operator queries.
func (r *PostgresEventDLQRepository) RecordDeadLettered(
	ctx context.Context,
	eventType, envelopeJSON, handlerErr string,
	attempts int,
) error {
	if _, err := r.db.Exec(ctx,
		`INSERT INTO event_dlq (event_type, payload, handler_err, attempts)
		 VALUES ($1, $2::jsonb, $3, $4)`,
		eventType, envelopeJSON, handlerErr, attempts,
	); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}
