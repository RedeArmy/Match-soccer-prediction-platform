package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const errMsgMaxMembersReached = "this group has reached its maximum number of members"

// PostgresGroupMembershipRepository is the PostgreSQL-backed implementation of
// GroupMembershipRepository.
type PostgresGroupMembershipRepository struct {
	db *pgxpool.Pool
}

// NewPostgresGroupMembershipRepository constructs a PostgresGroupMembershipRepository.
func NewPostgresGroupMembershipRepository(db *pgxpool.Pool) *PostgresGroupMembershipRepository {
	return &PostgresGroupMembershipRepository{db: db}
}

// isMaxMembersViolation reports whether err originates from the
// enforce_max_members trigger, which raises EXCEPTION 'max_members_exceeded'
// (PostgreSQL error code P0001 = raise_exception).
func isMaxMembersViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "P0001" &&
		strings.Contains(pgErr.Message, "max_members_exceeded")
}

const (
	membershipColumns     = "id, quiniela_id, user_id, status, role, paid, joined_at, created_at, updated_at, removed_at, removed_by"
	errMembershipNotFound = "membership not found"
)

func scanMembership(row pgx.Row) (*domain.GroupMembership, error) {
	m := &domain.GroupMembership{}
	var joinedAt *time.Time
	err := row.Scan(&m.ID, &m.QuinielaID, &m.UserID, &m.Status, &m.Role, &m.Paid, &joinedAt, &m.CreatedAt, &m.UpdatedAt, &m.RemovedAt, &m.RemovedBy)
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
	role := m.Role
	if role == "" {
		role = domain.MembershipRoleMember
	}
	row := r.db.QueryRow(ctx,
		`INSERT INTO group_memberships (quiniela_id, user_id, status, role, paid, joined_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+membershipColumns,
		m.QuinielaID, m.UserID, m.Status, role, m.Paid, m.JoinedAt,
	)
	result, err := scanMembership(row)
	if err != nil {
		if isMaxMembersViolation(err) {
			return apperrors.Conflict(errMsgMaxMembersReached)
		}
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
		`UPDATE group_memberships
		    SET status=$1, paid=$2, joined_at=$3, removed_at=$4, removed_by=$5, updated_at=NOW()
		  WHERE id=$6 RETURNING `+membershipColumns,
		m.Status, m.Paid, m.JoinedAt, m.RemovedAt, m.RemovedBy, m.ID,
	)
	result, err := scanMembership(row)
	if err != nil {
		if isMaxMembersViolation(err) {
			return apperrors.Conflict(errMsgMaxMembersReached)
		}
		return err
	}
	if result == nil {
		return apperrors.NotFound(errMembershipNotFound)
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
		return nil, apperrors.NotFound(errMembershipNotFound)
	}
	return result, nil
}

func (r *PostgresGroupMembershipRepository) ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.GroupMembership, error) {
	// JOIN with users excludes memberships belonging to soft-deleted users so
	// that the group roster shown to administrators never contains ghost entries.
	rows, err := r.db.Query(ctx,
		`SELECT gm.id, gm.quiniela_id, gm.user_id, gm.status, gm.role, gm.paid,
		        gm.joined_at, gm.created_at, gm.updated_at, gm.removed_at, gm.removed_by
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
		`SELECT gm.id, gm.quiniela_id, gm.user_id, gm.status, gm.role, gm.paid,
		        gm.joined_at, gm.created_at, gm.updated_at, gm.removed_at, gm.removed_by
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
		if err := rows.Scan(&m.ID, &m.QuinielaID, &m.UserID, &m.Status, &m.Role, &m.Paid, &joinedAt, &m.CreatedAt, &m.UpdatedAt, &m.RemovedAt, &m.RemovedBy); err != nil {
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

// OldestActiveMember returns the active membership with the earliest JoinedAt
// in quinielaID, excluding excludeUserID. Returns nil, nil when no eligible
// successor exists (empty group after the owner leaves).
func (r *PostgresGroupMembershipRepository) OldestActiveMember(ctx context.Context, quinielaID, excludeUserID int) (*domain.GroupMembership, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+membershipColumns+`
		   FROM group_memberships
		  WHERE quiniela_id = $1
		    AND status      = 'active'
		    AND user_id    != $2
		  ORDER BY joined_at ASC
		  LIMIT 1`,
		quinielaID, excludeUserID,
	)
	return scanMembership(row)
}

// SetRole updates the role column for the given membership. It is the only
// sanctioned path for privilege changes; the general Update method does not
// touch role to prevent accidental escalation.
func (r *PostgresGroupMembershipRepository) SetRole(ctx context.Context, membershipID int, role domain.MembershipRole) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE group_memberships SET role=$1, updated_at=NOW() WHERE id=$2`,
		role, membershipID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound(errMembershipNotFound)
	}
	return nil
}

// RemoveByAdmin soft-deletes a membership on behalf of an administrator by
// setting status to 'left' and recording the actor and timestamp in the
// audit columns. Returns NotFound when the membership does not exist or is
// already inactive.
func (r *PostgresGroupMembershipRepository) RemoveByAdmin(ctx context.Context, membershipID, adminID int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE group_memberships
		    SET status     = 'left',
		        removed_at = NOW(),
		        removed_by = $2,
		        updated_at = NOW()
		  WHERE id = $1 AND status = 'active'`,
		membershipID, adminID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound(errMembershipNotFound)
	}
	return nil
}

// ListGroupIDsWithoutOwner returns quiniela IDs that have no active CreateOwner member.
func (r *PostgresGroupMembershipRepository) ListGroupIDsWithoutOwner(ctx context.Context) ([]int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT q.id
		FROM quinielas q
		WHERE q.deleted_at IS NULL
		  AND NOT EXISTS (
		        SELECT 1 FROM group_memberships gm
		        WHERE gm.quiniela_id = q.id
		          AND gm.role = 'owner'
		          AND gm.status = 'active'
		  )`)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, apperrors.Internal(err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListStalePending returns pending memberships older than olderThan.
func (r *PostgresGroupMembershipRepository) ListStalePending(ctx context.Context, olderThan time.Time) ([]*domain.GroupMembership, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+membershipColumns+` FROM group_memberships WHERE status = 'pending' AND created_at < $1 ORDER BY created_at ASC`,
		olderThan,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectMemberships(rows)
}

// BulkRemoveByAdmin sets multiple memberships to 'left' on behalf of an admin.
// Only rows whose quiniela_id matches quinielaID are updated, preventing an
// admin from removing memberships that belong to a different group by passing
// arbitrary IDs. Already-inactive memberships are silently skipped.
func (r *PostgresGroupMembershipRepository) BulkRemoveByAdmin(ctx context.Context, quinielaID int, ids []int, adminID int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		UPDATE group_memberships
		SET status = 'left', removed_at = NOW(), removed_by = $3, updated_at = NOW()
		WHERE id = ANY($1) AND quiniela_id = $2 AND status = 'active'
		RETURNING id`, ids, quinielaID, adminID)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	var succeeded []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, apperrors.Internal(err)
		}
		succeeded = append(succeeded, id)
	}
	return succeeded, rows.Err()
}

// TransferOwnershipRoles atomically demotes every active owner of quinielaID
// to 'member' and promotes newOwnerMembershipID to 'owner' in one transaction.
// If either UPDATE fails the transaction rolls back and neither change persists.
func (r *PostgresGroupMembershipRepository) TransferOwnershipRoles(ctx context.Context, quinielaID, newOwnerMembershipID int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return apperrors.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Demote all current owners. Using quinielaID scope instead of a specific
	// membership ID handles the edge case where corrupted data left multiple
	// owners; both are demoted atomically.
	_, err = tx.Exec(ctx,
		`UPDATE group_memberships
		    SET role = 'member', updated_at = NOW()
		  WHERE quiniela_id = $1
		    AND role        = 'owner'
		    AND status      = 'active'`,
		quinielaID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}

	// Promote the new owner.
	tag, err := tx.Exec(ctx,
		`UPDATE group_memberships
		    SET role = 'owner', updated_at = NOW()
		  WHERE id = $1`,
		newOwnerMembershipID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("new owner membership not found")
	}

	if err := tx.Commit(ctx); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

// syncStatusInTx recomputes the quiniela's active/inactive status inside an
// open transaction. The COUNT subquery runs within the same snapshot as the
// preceding membership write, so the status update is always consistent with
// the member count. A soft-deleted quiniela matches 0 rows; the UPDATE is a
// silent no-op — the group is already effectively removed.
func syncStatusInTx(ctx context.Context, tx pgx.Tx, quinielaID, minMembers int) error {
	_, err := tx.Exec(ctx, `
		UPDATE quinielas
		   SET status = CASE
		         WHEN (
		           SELECT COUNT(*)
		             FROM group_memberships
		            WHERE quiniela_id = $1
		              AND status = 'active'
		         ) >= $2 THEN 'active'
		         ELSE 'inactive'
		       END,
		       updated_at = NOW()
		 WHERE id = $1
		   AND deleted_at IS NULL`,
		quinielaID, minMembers,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

// ApproveMembership atomically promotes a pending membership to active and
// recalculates the quiniela's status in a single transaction. The enforce_max_members
// trigger fires on the UPDATE, so a capacity overflow is caught before commit.
func (r *PostgresGroupMembershipRepository) ApproveMembership(
	ctx context.Context,
	membershipID, quinielaID int,
	now time.Time,
	minMembers int,
) (*domain.GroupMembership, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx,
		`UPDATE group_memberships
		    SET status    = 'active',
		        joined_at = $1,
		        updated_at = NOW()
		  WHERE id          = $2
		    AND quiniela_id = $3
		    AND status      = 'pending'
		  RETURNING `+membershipColumns,
		now, membershipID, quinielaID,
	)
	m, err := scanMembership(row)
	if err != nil {
		if isMaxMembersViolation(err) {
			return nil, apperrors.Conflict(errMsgMaxMembersReached)
		}
		return nil, err
	}
	if m == nil {
		// The service pre-flight confirmed the request was pending; 0 rows means
		// a concurrent approval committed between that check and this call.
		return nil, apperrors.Conflict("this join request is no longer pending")
	}

	if err := syncStatusInTx(ctx, tx, quinielaID, minMembers); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperrors.Internal(err)
	}
	return m, nil
}

// LeaveMembership atomically transitions a membership to left and recalculates
// the quiniela's status in a single transaction.
func (r *PostgresGroupMembershipRepository) LeaveMembership(
	ctx context.Context,
	quinielaID, userID int,
	now time.Time,
	minMembers int,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return apperrors.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx,
		`UPDATE group_memberships
		    SET status     = 'left',
		        joined_at  = NULL,
		        removed_at = $1,
		        removed_by = NULL,
		        updated_at = NOW()
		  WHERE quiniela_id = $2
		    AND user_id     = $3
		    AND status      = 'active'`,
		now, quinielaID, userID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		// Race: the member was removed concurrently before this call committed.
		return apperrors.Conflict("you are no longer an active member of this group")
	}

	if err := syncStatusInTx(ctx, tx, quinielaID, minMembers); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

var _ GroupMembershipRepository = (*PostgresGroupMembershipRepository)(nil)
