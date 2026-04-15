package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresGroupMembershipRepository is the PostgreSQL-backed implementation of
// GroupMembershipRepository.
type PostgresGroupMembershipRepository struct {
	db *pgxpool.Pool
}

// NewPostgresGroupMembershipRepository constructs a PostgresGroupMembershipRepository.
func NewPostgresGroupMembershipRepository(db *pgxpool.Pool) *PostgresGroupMembershipRepository {
	return &PostgresGroupMembershipRepository{db: db}
}

const membershipColumns = "id, quiniela_id, user_id, status, paid, joined_at, created_at, updated_at"

func scanMembership(row pgx.Row) (*domain.GroupMembership, error) {
	m := &domain.GroupMembership{}
	var joinedAt *time.Time
	err := row.Scan(&m.ID, &m.QuinielaID, &m.UserID, &m.Status, &m.Paid, &joinedAt, &m.CreatedAt, &m.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	m.JoinedAt = joinedAt
	return m, nil
}

func (r *PostgresGroupMembershipRepository) Create(ctx context.Context, m *domain.GroupMembership) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO group_memberships (quiniela_id, user_id, status, paid, joined_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING `+membershipColumns,
		m.QuinielaID, m.UserID, m.Status, m.Paid, m.JoinedAt,
	)
	result, err := scanMembership(row)
	if err != nil {
		return err
	}
	*m = *result
	return nil
}

func (r *PostgresGroupMembershipRepository) GetByID(ctx context.Context, membershipID int) (*domain.GroupMembership, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+membershipColumns+` FROM group_memberships WHERE id=$1`,
		membershipID,
	)
	return scanMembership(row)
}

func (r *PostgresGroupMembershipRepository) GetByQuinielaAndUser(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+membershipColumns+` FROM group_memberships WHERE quiniela_id=$1 AND user_id=$2`,
		quinielaID, userID,
	)
	return scanMembership(row)
}

// CountActive returns the number of members with status = 'active' in the
// given quiniela. It is used exclusively by syncGroupStatus to decide whether
// the group should transition to active or inactive.
func (r *PostgresGroupMembershipRepository) CountActive(ctx context.Context, quinielaID int) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM group_memberships WHERE quiniela_id=$1 AND status='active'`,
		quinielaID,
	).Scan(&count)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return count, nil
}

func (r *PostgresGroupMembershipRepository) Update(ctx context.Context, m *domain.GroupMembership) error {
	row := r.db.QueryRow(ctx,
		`UPDATE group_memberships SET status=$1, paid=$2, joined_at=$3, updated_at=NOW()
		 WHERE id=$4 RETURNING `+membershipColumns,
		m.Status, m.Paid, m.JoinedAt, m.ID,
	)
	result, err := scanMembership(row)
	if err != nil {
		return err
	}
	if result == nil {
		return apperrors.NotFound("membership not found")
	}
	*m = *result
	return nil
}

func (r *PostgresGroupMembershipRepository) MarkPaid(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error) {
	row := r.db.QueryRow(ctx,
		`UPDATE group_memberships SET paid=TRUE, updated_at=NOW()
		 WHERE quiniela_id=$1 AND user_id=$2 RETURNING `+membershipColumns,
		quinielaID, userID,
	)
	result, err := scanMembership(row)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NotFound("membership not found")
	}
	return result, nil
}

func (r *PostgresGroupMembershipRepository) ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.GroupMembership, error) {
	// JOIN with users excludes memberships belonging to soft-deleted users so
	// that the group roster shown to administrators never contains ghost entries.
	rows, err := r.db.Query(ctx,
		`SELECT gm.id, gm.quiniela_id, gm.user_id, gm.status, gm.paid,
		        gm.joined_at, gm.created_at, gm.updated_at
		 FROM group_memberships gm
		 JOIN users u ON u.id = gm.user_id AND u.deleted_at IS NULL
		 WHERE gm.quiniela_id = $1
		 ORDER BY gm.created_at ASC`,
		quinielaID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectMemberships(rows)
}

func (r *PostgresGroupMembershipRepository) ListByUser(ctx context.Context, userID int) ([]*domain.GroupMembership, error) {
	// JOIN with quinielas excludes memberships in soft-deleted groups so that
	// GET /api/v1/groups/me never surfaces a group the owner has deleted.
	rows, err := r.db.Query(ctx,
		`SELECT gm.id, gm.quiniela_id, gm.user_id, gm.status, gm.paid,
		        gm.joined_at, gm.created_at, gm.updated_at
		 FROM group_memberships gm
		 JOIN quinielas q ON q.id = gm.quiniela_id AND q.deleted_at IS NULL
		 WHERE gm.user_id = $1
		 ORDER BY gm.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectMemberships(rows)
}

func collectMemberships(rows pgx.Rows) ([]*domain.GroupMembership, error) {
	var memberships []*domain.GroupMembership
	for rows.Next() {
		m := &domain.GroupMembership{}
		var joinedAt *time.Time
		if err := rows.Scan(&m.ID, &m.QuinielaID, &m.UserID, &m.Status, &m.Paid, &joinedAt, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		m.JoinedAt = joinedAt
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return memberships, nil
}
