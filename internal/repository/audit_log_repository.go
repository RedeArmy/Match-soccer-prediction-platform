package repository

import (
	"context"
	"encoding/json"

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

// ListByEntity returns audit entries for a specific resource, ordered newest first.
func (r *PostgresAuditLogRepository) ListByEntity(ctx context.Context, resourceType string, resourceID int, p CursorPage) ([]*domain.AuditLog, string, error) {
	return r.List(ctx, AuditLogFilters{ResourceType: &resourceType, ResourceID: &resourceID}, p)
}

// ListByActor returns all actions performed by actorID, ordered newest first.
func (r *PostgresAuditLogRepository) ListByActor(ctx context.Context, actorID int, p CursorPage) ([]*domain.AuditLog, string, error) {
	return r.List(ctx, AuditLogFilters{ActorID: &actorID}, p)
}

// ListByAction returns all entries whose action field equals action, ordered newest first.
func (r *PostgresAuditLogRepository) ListByAction(ctx context.Context, action string, p CursorPage) ([]*domain.AuditLog, string, error) {
	return r.List(ctx, AuditLogFilters{Action: &action}, p)
}

// List is the general-purpose query method. Non-nil filter fields are AND-ed.
// Ordering is by id DESC (primary key), which is insertion-time order with no
// ties. Fetches p.Limit+1 rows to detect whether a next page exists; the extra
// row is stripped before returning. The returned cursor is empty on the last page.
func (r *PostgresAuditLogRepository) List(ctx context.Context, f AuditLogFilters, p CursorPage) ([]*domain.AuditLog, string, error) {
	if p.Limit <= 0 {
		return nil, "", apperrors.BadRequest("page size must be a positive integer")
	}

	wb := newWhereBuilder()

	if p.Cursor != "" {
		afterID, err := decodeCursor(p.Cursor)
		if err != nil {
			return nil, "", err
		}
		wb.add("id < $%d", afterID)
	}
	if f.ActorID != nil {
		wb.add("actor_id = $%d", *f.ActorID)
	}
	if f.Action != nil {
		wb.add("action = $%d", *f.Action)
	}
	if f.ResourceType != nil {
		wb.add("resource_type = $%d", *f.ResourceType)
	}
	if f.ResourceID != nil {
		wb.add("resource_id = $%d", *f.ResourceID)
	}
	if f.CreatedAfter != nil {
		wb.add("created_at >= $%d", *f.CreatedAfter)
	}
	if f.CreatedBefore != nil {
		wb.add("created_at <= $%d", *f.CreatedBefore)
	}

	args := wb.args
	n := wb.next()
	args = append(args, p.Limit+1)

	q := `SELECT ` + auditLogColumns + ` FROM audit_log` + wb.clause() +
		` ORDER BY id DESC LIMIT $` + itoa(n)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, "", apperrors.Internal(err)
	}
	defer rows.Close()

	entries, err := collectAuditLogs(rows)
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(entries) > p.Limit {
		entries = entries[:p.Limit]
		nextCursor = encodeCursor(entries[len(entries)-1].ID)
	}

	return entries, nextCursor, nil
}

var _ AuditLogRepository = (*PostgresAuditLogRepository)(nil)
