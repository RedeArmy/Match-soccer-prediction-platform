package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresAuditLogRepository is the PostgreSQL-backed implementation of
// AuditLogRepository.
type PostgresAuditLogRepository struct {
	db *pgxpool.Pool
}

// NewPostgresAuditLogRepository constructs a PostgresAuditLogRepository.
func NewPostgresAuditLogRepository(db *pgxpool.Pool) *PostgresAuditLogRepository {
	return &PostgresAuditLogRepository{db: db}
}

const auditLogColumns = "id, actor_id, actor_role, action, resource_type, resource_id, metadata, created_at"

func scanAuditLog(row pgx.Row) (*domain.AuditLog, error) {
	entry := &domain.AuditLog{}
	var actorRole *string
	var metaBytes []byte
	err := row.Scan(
		&entry.ID, &entry.ActorID, &actorRole, &entry.Action,
		&entry.ResourceType, &entry.ResourceID, &metaBytes, &entry.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	if actorRole != nil {
		r := domain.UserRole(*actorRole)
		entry.ActorRole = &r
	}
	if metaBytes != nil {
		if err := json.Unmarshal(metaBytes, &entry.Metadata); err != nil {
			return nil, apperrors.Internal(err)
		}
	}
	return entry, nil
}

func collectAuditLogs(rows pgx.Rows) ([]*domain.AuditLog, error) {
	var entries []*domain.AuditLog
	for rows.Next() {
		entry := &domain.AuditLog{}
		var actorRole *string
		var metaBytes []byte
		if err := rows.Scan(
			&entry.ID, &entry.ActorID, &actorRole, &entry.Action,
			&entry.ResourceType, &entry.ResourceID, &metaBytes, &entry.CreatedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		if actorRole != nil {
			r := domain.UserRole(*actorRole)
			entry.ActorRole = &r
		}
		if metaBytes != nil {
			if err := json.Unmarshal(metaBytes, &entry.Metadata); err != nil {
				return nil, apperrors.Internal(err)
			}
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return entries, nil
}

// Create inserts an immutable audit log entry. The entry's ID and CreatedAt
// are populated on return. Metadata may be nil for events with no extra context.
func (r *PostgresAuditLogRepository) Create(ctx context.Context, entry *domain.AuditLog) error {
	var metaBytes []byte
	if entry.Metadata != nil {
		var err error
		metaBytes, err = json.Marshal(entry.Metadata)
		if err != nil {
			return apperrors.Internal(err)
		}
	}
	row := r.db.QueryRow(ctx,
		`INSERT INTO audit_log (actor_id, actor_role, action, resource_type, resource_id, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+auditLogColumns,
		entry.ActorID, entry.ActorRole, entry.Action,
		entry.ResourceType, entry.ResourceID, metaBytes,
	)
	result, err := scanAuditLog(row)
	if err != nil {
		return err
	}
	*entry = *result
	return nil
}

// ListByEntity returns audit entries for a specific resource, ordered newest
// first.
func (r *PostgresAuditLogRepository) ListByEntity(ctx context.Context, resourceType string, resourceID int, p Pagination) ([]*domain.AuditLog, error) {
	return r.List(ctx, AuditLogFilters{ResourceType: &resourceType, ResourceID: &resourceID}, p)
}

// ListByActor returns all actions performed by actorID, ordered newest first.
func (r *PostgresAuditLogRepository) ListByActor(ctx context.Context, actorID int, p Pagination) ([]*domain.AuditLog, error) {
	return r.List(ctx, AuditLogFilters{ActorID: &actorID}, p)
}

// ListByAction returns all entries whose action field equals action, ordered
// newest first.
func (r *PostgresAuditLogRepository) ListByAction(ctx context.Context, action string, p Pagination) ([]*domain.AuditLog, error) {
	return r.List(ctx, AuditLogFilters{Action: &action}, p)
}

// List is the general-purpose query method. Non-nil filter fields are combined
// with AND. Pagination.Limit = 0 means no upper bound.
func (r *PostgresAuditLogRepository) List(ctx context.Context, f AuditLogFilters, p Pagination) ([]*domain.AuditLog, error) {
	q := `SELECT ` + auditLogColumns + ` FROM audit_log`
	var args []any
	argIdx := 1

	var conds []string
	if f.ActorID != nil {
		conds = append(conds, fmt.Sprintf("actor_id = $%d", argIdx))
		args = append(args, *f.ActorID)
		argIdx++
	}
	if f.Action != nil {
		conds = append(conds, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, *f.Action)
		argIdx++
	}
	if f.ResourceType != nil {
		conds = append(conds, fmt.Sprintf("resource_type = $%d", argIdx))
		args = append(args, *f.ResourceType)
		argIdx++
	}
	if f.ResourceID != nil {
		conds = append(conds, fmt.Sprintf("resource_id = $%d", argIdx))
		args = append(args, *f.ResourceID)
		argIdx++
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY created_at DESC"
	if p.Limit > 0 {
		args = append(args, p.Limit)
		q += fmt.Sprintf(" LIMIT $%d", argIdx)
		argIdx++
	}
	if p.Offset > 0 {
		args = append(args, p.Offset)
		q += fmt.Sprintf(" OFFSET $%d", argIdx)
	}

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectAuditLogs(rows)
}

var _ AuditLogRepository = (*PostgresAuditLogRepository)(nil)
