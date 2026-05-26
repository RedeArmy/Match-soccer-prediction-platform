package repository

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresKYCEventRepository is the PostgreSQL-backed implementation of KYCEventRepository.
type PostgresKYCEventRepository struct {
	db *pgxpool.Pool
}

// NewPostgresKYCEventRepository constructs a PostgresKYCEventRepository.
func NewPostgresKYCEventRepository(db *pgxpool.Pool) *PostgresKYCEventRepository {
	return &PostgresKYCEventRepository{db: db}
}

// Create inserts one immutable KYC event audit record.
// event.ID and event.CreatedAt are populated by the database on INSERT.
func (r *PostgresKYCEventRepository) Create(ctx context.Context, event *domain.KYCEvent) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	metaBytes, err := json.Marshal(event.Metadata)
	if err != nil {
		return apperrors.Internal(err)
	}
	var oldStatus *string
	if event.OldStatus != nil {
		s := string(*event.OldStatus)
		oldStatus = &s
	}
	const q = `
		INSERT INTO kyc_events
			(profile_id, profile_type, event_type, actor_id, old_status, new_status, reason, metadata, trace_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`
	return r.db.QueryRow(ctx, q,
		event.ProfileID, string(event.ProfileType), string(event.EventType),
		event.ActorID, oldStatus, string(event.NewStatus),
		event.Reason, metaBytes, event.TraceID,
	).Scan(&event.ID, &event.CreatedAt)
}

// ListByProfile returns events for a profile ordered by created_at ASC (oldest first).
// Cursor-based pagination is keyed on id ASC; the cursor points to the last seen ID.
func (r *PostgresKYCEventRepository) ListByProfile(ctx context.Context, profileID int, profileType domain.KYCProfileType, p CursorPage) ([]*domain.KYCEvent, string, error) {
	if p.Limit <= 0 {
		return nil, "", apperrors.BadRequest("page size must be a positive integer")
	}
	wb := newWhereBuilder()
	wb.add("profile_id = $%d", profileID)
	wb.add("profile_type = $%d", string(profileType))
	if p.Cursor != "" {
		afterID, err := decodeCursor(p.Cursor)
		if err != nil {
			return nil, "", err
		}
		wb.add("id > $%d", afterID)
	}
	args := wb.args
	n := wb.next()
	args = append(args, p.Limit+1)
	q := `SELECT id, profile_id, profile_type, event_type, actor_id, old_status, new_status,
	             reason, metadata, trace_id, created_at
	        FROM kyc_events` + wb.clause() + ` ORDER BY id ASC LIMIT $` + itoa(n)
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, "", apperrors.Internal(err)
	}
	defer rows.Close()
	events, err := collectKYCEvents(rows)
	if err != nil {
		return nil, "", err
	}
	var nextCursor string
	if len(events) > p.Limit {
		events = events[:p.Limit]
		nextCursor = encodeCursor(int(events[len(events)-1].ID))
	}
	return events, nextCursor, nil
}

func collectKYCEvents(rows pgx.Rows) ([]*domain.KYCEvent, error) {
	var result []*domain.KYCEvent
	for rows.Next() {
		e, err := scanKYCEvent(rows)
		if err != nil {
			return nil, apperrors.Internal(err)
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return result, nil
}

func scanKYCEvent(s rowScanner) (*domain.KYCEvent, error) {
	e := &domain.KYCEvent{}
	var profileType, eventType, newStatus string
	var oldStatus *string
	var metaBytes []byte
	err := s.Scan(
		&e.ID, &e.ProfileID, &profileType, &eventType,
		&e.ActorID, &oldStatus, &newStatus,
		&e.Reason, &metaBytes, &e.TraceID, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	e.ProfileType = domain.KYCProfileType(profileType)
	e.EventType = domain.KYCEventType(eventType)
	e.NewStatus = domain.KYCStatus(newStatus)
	if oldStatus != nil {
		s := domain.KYCStatus(*oldStatus)
		e.OldStatus = &s
	}
	if len(metaBytes) > 0 {
		if err := json.Unmarshal(metaBytes, &e.Metadata); err != nil {
			return nil, err
		}
	}
	return e, nil
}

var _ KYCEventRepository = (*PostgresKYCEventRepository)(nil)
