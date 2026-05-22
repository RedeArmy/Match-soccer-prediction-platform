package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresSystemParamHistoryRepository is the PostgreSQL-backed implementation
// of SystemParamHistoryRepository.
type PostgresSystemParamHistoryRepository struct {
	db *pgxpool.Pool
}

// NewPostgresSystemParamHistoryRepository constructs the repository.
func NewPostgresSystemParamHistoryRepository(db *pgxpool.Pool) *PostgresSystemParamHistoryRepository {
	return &PostgresSystemParamHistoryRepository{db: db}
}

const systemParamHistoryColumns = "id, key, old_value, new_value, actor_id, action, changed_at"

func scanSystemParamHistory(row pgx.Row) (*domain.SystemParamHistory, error) {
	h := &domain.SystemParamHistory{}
	err := row.Scan(&h.ID, &h.Key, &h.OldValue, &h.NewValue, &h.ActorID, &h.Action, &h.ChangedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return h, nil
}

func collectSystemParamHistories(rows pgx.Rows) ([]*domain.SystemParamHistory, error) {
	var entries []*domain.SystemParamHistory
	for rows.Next() {
		h := &domain.SystemParamHistory{}
		if err := rows.Scan(&h.ID, &h.Key, &h.OldValue, &h.NewValue, &h.ActorID, &h.Action, &h.ChangedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		entries = append(entries, h)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return entries, nil
}

// Record inserts one history entry. entry.ID and entry.ChangedAt are set by
// the database; any caller-supplied values for those fields are ignored.
func (r *PostgresSystemParamHistoryRepository) Record(ctx context.Context, entry *domain.SystemParamHistory) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO system_params_history (key, old_value, new_value, actor_id, action)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+systemParamHistoryColumns,
		entry.Key, entry.OldValue, entry.NewValue, entry.ActorID, entry.Action,
	)
	recorded, err := scanSystemParamHistory(row)
	if err != nil {
		return err
	}
	// Back-fill generated fields so callers can observe the persisted state.
	if recorded != nil {
		entry.ID = recorded.ID
		entry.ChangedAt = recorded.ChangedAt
	}
	return nil
}

// ListByKey returns history rows for the given param key, newest-first, with
// cursor-based pagination. Fetches p.Limit+1 rows to detect whether a next
// page exists; the extra row is stripped before returning.
func (r *PostgresSystemParamHistoryRepository) ListByKey(ctx context.Context, key string, p CursorPage) ([]*domain.SystemParamHistory, string, error) {
	if p.Limit <= 0 {
		return nil, "", apperrors.BadRequest("page size must be a positive integer")
	}

	wb := newWhereBuilder()
	wb.add("key = $%d", key)

	if p.Cursor != "" {
		afterID, err := decodeCursor(p.Cursor)
		if err != nil {
			return nil, "", err
		}
		wb.add("id < $%d", afterID)
	}

	args := wb.args
	n := wb.next()
	args = append(args, p.Limit+1)

	q := `SELECT ` + systemParamHistoryColumns + ` FROM system_params_history` +
		wb.clause() + ` ORDER BY id DESC LIMIT $` + itoa(n)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, "", apperrors.Internal(err)
	}
	defer rows.Close()

	entries, err := collectSystemParamHistories(rows)
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(entries) > p.Limit {
		entries = entries[:p.Limit]
		nextCursor = encodeCursor(int(entries[len(entries)-1].ID))
	}

	return entries, nextCursor, nil
}

var _ SystemParamHistoryRepository = (*PostgresSystemParamHistoryRepository)(nil)
