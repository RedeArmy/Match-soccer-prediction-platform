package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	errMsgMaxMembersReached     = "this group has reached its maximum number of members"
	errMsgFreeMaxMembersReached = "this free group has reached its member limit; upgrade to premium to add more members"
)

// querier is satisfied by both pgxpool.Pool and pgx.Tx, allowing
// enforceMaxMembers to be called with either a pool connection or a live
// transaction without duplicating the COUNT query.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PostgresGroupMembershipRepository is the PostgreSQL-backed implementation of
// GroupMembershipRepository.
type PostgresGroupMembershipRepository struct {
	db *pgxpool.Pool
}

// NewPostgresGroupMembershipRepository constructs a PostgresGroupMembershipRepository.
func NewPostgresGroupMembershipRepository(db *pgxpool.Pool) *PostgresGroupMembershipRepository {
	return &PostgresGroupMembershipRepository{db: db}
}

const (
	membershipColumns     = "id, quiniela_id, user_id, status, role, paid, joined_at, created_at, updated_at, removed_at, removed_by"
	errMembershipNotFound = "membership not found"
)

func scanMembership(row pgx.Row) (*domain.GroupMembership, error) {
	m := &domain.GroupMembership{}
	var joinedAt *time.Time
	if err := row.Scan(&m.ID, &m.QuinielaID, &m.UserID, &m.Status, &m.Role, &m.Paid, &joinedAt, &m.CreatedAt, &m.UpdatedAt, &m.RemovedAt, &m.RemovedBy); err != nil {
		return nil, singleScanErr(err)
	}
	m.JoinedAt = joinedAt
	return m, nil
}

// RequestJoinByInviteCode serialises invite-code resolution, existing-membership
// inspection, capacity enforcement, and membership mutation in one transaction.
func (r *PostgresGroupMembershipRepository) RequestJoinByInviteCode(ctx context.Context, inviteCode string, userID, maxMembers, freeMaxMembers int) (*domain.Quiniela, *domain.GroupMembership, error) {
	var q *domain.Quiniela
	var m *domain.GroupMembership
	err := withTx(ctx, r.db, "GroupMembershipRepository.RequestJoinByInviteCode", func(tx pgx.Tx) error {
		var err error
		q, err = findQuinielaByInviteCode(ctx, tx, inviteCode)
		if err != nil {
			return err
		}
		existing, err := findMembershipForLocking(ctx, tx, q.ID, userID)
		if err != nil {
			return err
		}
		if err := validateMembershipStatus(existing); err != nil {
			return err
		}
		if err := enforceJoinCapacity(ctx, tx, q, freeMaxMembers, maxMembers); err != nil {
			return err
		}
		autoPaid := q.EntryFee == 0
		if existing != nil {
			m, err = reactivateMembership(ctx, tx, existing.ID, autoPaid)
		} else {
			m, err = createPendingMembership(ctx, tx, q.ID, userID, autoPaid)
		}
		return err
	})
	return q, m, err
}

func findQuinielaByInviteCode(ctx context.Context, tx pgx.Tx, inviteCode string) (*domain.Quiniela, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+quinielaColumns+` FROM quinielas
		  WHERE invite_code = $1
		    AND deleted_at IS NULL
		    AND (invite_code_expires_at IS NULL OR invite_code_expires_at > NOW())
		  FOR UPDATE`,
		inviteCode,
	)
	q, err := scanQuiniela(row)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound("group not found for the given invite code")
	}
	return q, nil
}

func findMembershipForLocking(ctx context.Context, tx pgx.Tx, quinielaID, userID int) (*domain.GroupMembership, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+membershipColumns+`
		   FROM group_memberships
		  WHERE quiniela_id = $1
		    AND user_id     = $2
		  FOR UPDATE`,
		quinielaID, userID,
	)
	return scanMembership(row)
}

func validateMembershipStatus(existing *domain.GroupMembership) error {
	if existing == nil {
		return nil
	}
	switch existing.Status {
	case domain.MembershipActive:
		return apperrors.Conflict("you are already a member of this group")
	case domain.MembershipPending:
		return apperrors.Conflict("you already have a pending join request for this group")
	default: // MembershipLeft: user previously left and is allowed to rejoin
		return nil
	}
}

func enforceJoinCapacity(ctx context.Context, tx pgx.Tx, q *domain.Quiniela, freeMaxMembers, maxMembers int) error {
	if !q.IsPremium && freeMaxMembers > 0 {
		if err := enforceFreeMax(ctx, tx, q.ID, freeMaxMembers); err != nil {
			return err
		}
	}
	return enforceMaxMembers(ctx, tx, q.ID, maxMembers)
}

func enforceMaxMembers(ctx context.Context, q querier, quinielaID, maxMembers int) error {
	var count int
	err := q.QueryRow(ctx,
		`SELECT COUNT(*)
		   FROM group_memberships
		  WHERE quiniela_id = $1
		    AND status      = 'active'`,
		quinielaID,
	).Scan(&count)
	if err != nil {
		return apperrors.Internal(err)
	}
	if count >= maxMembers {
		return apperrors.Conflict(errMsgMaxMembersReached)
	}
	return nil
}

func enforceFreeMax(ctx context.Context, q querier, quinielaID, freeMax int) error {
	var count int
	err := q.QueryRow(ctx,
		`SELECT COUNT(*)
		   FROM group_memberships
		  WHERE quiniela_id = $1
		    AND status      = 'active'`,
		quinielaID,
	).Scan(&count)
	if err != nil {
		return apperrors.Internal(err)
	}
	if count >= freeMax {
		return apperrors.Conflict(errMsgFreeMaxMembersReached)
	}
	return nil
}

func reactivateMembership(ctx context.Context, tx pgx.Tx, membershipID int, autoPaid bool) (*domain.GroupMembership, error) {
	row := tx.QueryRow(ctx,
		`UPDATE group_memberships
		    SET status     = 'pending',
		        paid       = $1,
		        joined_at  = NULL,
		        removed_at = NULL,
		        removed_by = NULL,
		        updated_at = NOW()
		  WHERE id     = $2
		    AND status = 'left'
		  RETURNING `+membershipColumns,
		autoPaid, membershipID,
	)
	m, err := scanMembership(row)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, apperrors.Conflict("this membership request changed; please retry")
	}
	return m, nil
}

func createPendingMembership(ctx context.Context, tx pgx.Tx, quinielaID, userID int, autoPaid bool) (*domain.GroupMembership, error) {
	row := tx.QueryRow(ctx,
		`INSERT INTO group_memberships (quiniela_id, user_id, status, role, paid, joined_at)
		 VALUES ($1, $2, 'pending', 'member', $3, NULL)
		 RETURNING `+membershipColumns,
		quinielaID, userID, autoPaid,
	)
	m, err := scanMembership(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, apperrors.Conflict("you already have a membership record for this group")
		}
		return nil, err
	}
	return m, nil
}

func (r *PostgresGroupMembershipRepository) Create(ctx context.Context, m *domain.GroupMembership) error {
	if m.Status == domain.MembershipActive {
		if err := enforceMaxMembers(ctx, r.db, m.QuinielaID, domain.MaxMembersPerGroup); err != nil {
			return err
		}
	}
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

// CountActivePaid returns the number of members with status='active' AND
// paid=true in the given quiniela. This is the authoritative input to
// domain.WinnerCount and domain.EligibleForPayments: only members who are
// both active and have settled their entry fee are counted for prize purposes.
func (r *PostgresGroupMembershipRepository) CountActivePaid(ctx context.Context, quinielaID int) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*)
		   FROM group_memberships
		  WHERE quiniela_id = $1
		    AND status      = 'active'
		    AND paid        = TRUE`,
		quinielaID,
	).Scan(&count)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return count, nil
}

func (r *PostgresGroupMembershipRepository) Update(ctx context.Context, m *domain.GroupMembership) error {
	if m.Status == domain.MembershipActive {
		if err := enforceMaxMembers(ctx, r.db, m.QuinielaID, domain.MaxMembersPerGroup); err != nil {
			return err
		}
	}
	row := r.db.QueryRow(ctx,
		`UPDATE group_memberships
		    SET status=$1, paid=$2, joined_at=$3, removed_at=$4, removed_by=$5, updated_at=NOW()
		  WHERE id=$6 RETURNING `+membershipColumns,
		m.Status, m.Paid, m.JoinedAt, m.RemovedAt, m.RemovedBy, m.ID,
	)
	result, err := scanMembership(row)
	if err != nil {
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
	// u.name is selected to populate DisplayName without an extra round-trip.
	rows, err := r.db.Query(ctx,
		`SELECT gm.id, gm.quiniela_id, gm.user_id, gm.status, gm.role, gm.paid,
		        gm.joined_at, gm.created_at, gm.updated_at, gm.removed_at, gm.removed_by,
		        COALESCE(NULLIF(u.username, ''), NULLIF(TRIM(u.name), ''), u.email) AS display_name
		 FROM group_memberships gm
		 JOIN users u ON u.id = gm.user_id AND u.deleted_at IS NULL
		 WHERE gm.quiniela_id = $1
		   AND gm.status      != 'left'
		 ORDER BY gm.created_at ASC`,
		quinielaID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectRows(rows, func(r pgx.Rows) (*domain.GroupMembership, error) {
		m := &domain.GroupMembership{}
		var joinedAt *time.Time
		if err := r.Scan(&m.ID, &m.QuinielaID, &m.UserID, &m.Status, &m.Role, &m.Paid,
			&joinedAt, &m.CreatedAt, &m.UpdatedAt, &m.RemovedAt, &m.RemovedBy, &m.DisplayName); err != nil {
			return nil, err
		}
		m.JoinedAt = joinedAt
		return m, nil
	})
}

func (r *PostgresGroupMembershipRepository) ListActiveMemberIDsByGroup(ctx context.Context, quinielaID int) ([]int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT gm.user_id
		 FROM group_memberships gm
		 JOIN users u ON u.id = gm.user_id AND u.deleted_at IS NULL
		 WHERE gm.quiniela_id = $1 AND gm.status = 'active'`,
		quinielaID,
	)
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
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return ids, nil
}

func (r *PostgresGroupMembershipRepository) ListByUser(ctx context.Context, userID int) ([]*domain.GroupMembership, error) {
	// JOIN with quinielas excludes memberships in soft-deleted groups so that
	// GET /api/v1/groups/me never surfaces a group the owner has deleted.
	// q.name and q.status are included so callers can enrich responses without
	// extra round-trips.
	rows, err := r.db.Query(ctx,
		`SELECT gm.id, gm.quiniela_id, gm.user_id, gm.status, gm.role, gm.paid,
		        gm.joined_at, gm.created_at, gm.updated_at, gm.removed_at, gm.removed_by,
		        q.name, q.status AS group_status, q.invite_code, q.entry_fee, q.currency,
		        q.is_premium, q.mode_general, q.mode_round,
		        (SELECT COUNT(*) FROM group_memberships a
		           WHERE a.quiniela_id = gm.quiniela_id AND a.status = 'active')::int AS active_member_count
		 FROM group_memberships gm
		 JOIN quinielas q ON q.id = gm.quiniela_id AND q.deleted_at IS NULL
		 WHERE gm.user_id = $1
		   AND gm.status  != 'left'
		 ORDER BY gm.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectRows(rows, func(r pgx.Rows) (*domain.GroupMembership, error) {
		m := &domain.GroupMembership{}
		var joinedAt *time.Time
		if err := r.Scan(
			&m.ID, &m.QuinielaID, &m.UserID, &m.Status, &m.Role, &m.Paid,
			&joinedAt, &m.CreatedAt, &m.UpdatedAt, &m.RemovedAt, &m.RemovedBy,
			&m.GroupName, &m.GroupStatus, &m.InviteCode, &m.EntryFee, &m.Currency,
			&m.IsPremium, &m.ModeGeneral, &m.ModeRound, &m.ActiveMemberCount,
		); err != nil {
			return nil, err
		}
		m.JoinedAt = joinedAt
		return m, nil
	})
}

func collectMemberships(rows pgx.Rows) ([]*domain.GroupMembership, error) {
	return collectRows(rows, func(r pgx.Rows) (*domain.GroupMembership, error) {
		m := &domain.GroupMembership{}
		var joinedAt *time.Time
		if err := r.Scan(&m.ID, &m.QuinielaID, &m.UserID, &m.Status, &m.Role, &m.Paid, &joinedAt, &m.CreatedAt, &m.UpdatedAt, &m.RemovedAt, &m.RemovedBy); err != nil {
			return nil, err
		}
		m.JoinedAt = joinedAt
		return m, nil
	})
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
	return withTx(ctx, r.db, "GroupMembershipRepository.TransferOwnershipRoles", func(tx pgx.Tx) error {
		// Demote all current owners. Using quinielaID scope instead of a specific
		// membership ID handles the edge case where corrupted data left multiple
		// owners; both are demoted atomically.
		if _, err := tx.Exec(ctx,
			`UPDATE group_memberships
			    SET role = 'member', updated_at = NOW()
			  WHERE quiniela_id = $1
			    AND role        = 'owner'
			    AND status      = 'active'`,
			quinielaID,
		); err != nil {
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
		return nil
	})
}

// syncStatusInTx recomputes the quiniela's active/inactive status inside an
// open transaction. The COUNT subquery runs within the same snapshot as the
// preceding membership write, so the status update is always consistent with
// the member count. A soft-deleted quiniela matches 0 rows; the UPDATE is a
// silent no-op - the group is already effectively removed.
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
// recalculates the quiniela's status in a single transaction.
// The quiniela row is locked with FOR UPDATE for the full duration so that
// concurrent approvals cannot both pass the capacity check and both commit.
func (r *PostgresGroupMembershipRepository) ApproveMembership(
	ctx context.Context,
	membershipID, quinielaID int,
	now time.Time,
	minMembers, maxMembers int,
) (*domain.GroupMembership, error) {
	var m *domain.GroupMembership
	err := withTx(ctx, r.db, "GroupMembershipRepository.ApproveMembership", func(tx pgx.Tx) error {
		// Lock the quiniela row for the duration of the transaction. This serialises
		// all concurrent ApproveMembership calls for the same group and ensures the
		// capacity check below is race-safe.
		var lockedID int
		if err := tx.QueryRow(ctx,
			`SELECT id FROM quinielas WHERE id = $1 FOR UPDATE`,
			quinielaID,
		).Scan(&lockedID); err != nil {
			return apperrors.Internal(err)
		}

		if err := enforceMaxMembers(ctx, tx, quinielaID, maxMembers); err != nil {
			return err
		}

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
		var err error
		m, err = scanMembership(row)
		if err != nil {
			return err
		}
		if m == nil {
			// The service pre-flight confirmed the request was pending; 0 rows means
			// a concurrent approval committed between that check and this call.
			return apperrors.Conflict("this join request is no longer pending")
		}
		return syncStatusInTx(ctx, tx, quinielaID, minMembers)
	})
	return m, err
}

// LeaveMembership atomically transitions a membership to left and recalculates
// the quiniela's status in a single transaction.
func (r *PostgresGroupMembershipRepository) LeaveMembership(
	ctx context.Context,
	quinielaID, userID int,
	now time.Time,
	minMembers int,
) error {
	return withTx(ctx, r.db, "GroupMembershipRepository.LeaveMembership", func(tx pgx.Tx) error {
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
		return syncStatusInTx(ctx, tx, quinielaID, minMembers)
	})
}

// LeaveMembershipAndTransferOwnership atomically hands ownership to an active
// successor, marks the current owner as left, and refreshes group status.
func (r *PostgresGroupMembershipRepository) LeaveMembershipAndTransferOwnership(
	ctx context.Context,
	quinielaID, leavingUserID, successorMembershipID int,
	now time.Time,
	minMembers int,
) error {
	return withTx(ctx, r.db, "GroupMembershipRepository.LeaveMembershipAndTransferOwnership", func(tx pgx.Tx) error {
		// Demote any currently-active owners first so the promotion below is the
		// only surviving owner role when the transaction commits.
		if _, err := tx.Exec(ctx,
			`UPDATE group_memberships
			    SET role = 'member', updated_at = NOW()
			  WHERE quiniela_id = $1
			    AND role        = 'owner'
			    AND status      = 'active'`,
			quinielaID,
		); err != nil {
			return apperrors.Internal(err)
		}

		tag, err := tx.Exec(ctx,
			`UPDATE group_memberships
			    SET role = 'owner', updated_at = NOW()
			  WHERE id          = $1
			    AND quiniela_id = $2
			    AND status      = 'active'
			    AND user_id    != $3`,
			successorMembershipID, quinielaID, leavingUserID,
		)
		if err != nil {
			return apperrors.Internal(err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.NotFound("new owner membership not found")
		}

		tag, err = tx.Exec(ctx,
			`UPDATE group_memberships
			    SET status     = 'left',
			        role       = 'member',
			        joined_at  = NULL,
			        removed_at = $1,
			        removed_by = NULL,
			        updated_at = NOW()
			  WHERE quiniela_id = $2
			    AND user_id     = $3
			    AND status      = 'active'`,
			now, quinielaID, leavingUserID,
		)
		if err != nil {
			return apperrors.Internal(err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.Conflict("you are no longer an active member of this group")
		}
		return syncStatusInTx(ctx, tx, quinielaID, minMembers)
	})
}

// DebitBalanceAndMarkPaid atomically deducts amountCents from the user's
// available balance, marks the membership as paid, and writes a balance_ledger
// row — all in a single transaction.
func (r *PostgresGroupMembershipRepository) DebitBalanceAndMarkPaid(ctx context.Context, quinielaID, userID, amountCents int) (*domain.GroupMembership, error) {
	var membership *domain.GroupMembership
	err := withTx(ctx, r.db, "GroupMembershipRepository.DebitBalanceAndMarkPaid", func(tx pgx.Tx) error {
		var balanceAfter int
		err := tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents = balance_cents - $2,
			       updated_at    = NOW()
			 WHERE id = $1
			   AND deleted_at IS NULL
			   AND (balance_cents - reserved_cents) >= $2
			 RETURNING balance_cents
		`, userID, amountCents).Scan(&balanceAfter)
		if err == pgx.ErrNoRows {
			return insufficientOrNotFound(ctx, tx, userID)
		}
		if err != nil {
			return apperrors.Internal(err)
		}

		if err := insertLedgerTx(ctx, tx, ledgerRow{UserID: userID, DeltaCents: -amountCents, Kind: domain.LedgerKindEntryFee, BalanceAfter: balanceAfter, RefID: int64(quinielaID), RefType: "group_membership"}); err != nil {
			return err
		}

		row := tx.QueryRow(ctx,
			`UPDATE group_memberships SET paid=TRUE, updated_at=NOW()
			 WHERE quiniela_id=$1 AND user_id=$2 RETURNING `+membershipColumns,
			quinielaID, userID,
		)
		m, scanErr := scanMembership(row)
		if scanErr != nil {
			return scanErr
		}
		if m == nil {
			return apperrors.NotFound(errMembershipNotFound)
		}
		membership = m
		return nil
	})
	if err != nil {
		return nil, err
	}
	return membership, nil
}

// BulkDebitAndMarkPaid atomically charges all active unpaid members of quinielaID
// the given amountCents and marks each membership paid. All debits happen in one
// transaction; if any member has insufficient available balance the whole
// transaction is rolled back and a Conflict error is returned.
func (r *PostgresGroupMembershipRepository) BulkDebitAndMarkPaid(ctx context.Context, quinielaID, amountCents int) ([]int, error) {
	var charged []int
	err := withTx(ctx, r.db, "GroupMembershipRepository.BulkDebitAndMarkPaid", func(tx pgx.Tx) error {
		userIDs, err := fetchUnpaidMemberIDs(ctx, tx, quinielaID)
		if err != nil {
			return err
		}
		for _, userID := range userIDs {
			if err := debitMemberAndMarkPaid(ctx, tx, quinielaID, userID, amountCents); err != nil {
				return err
			}
		}
		charged = userIDs
		return nil
	})
	if err != nil {
		return nil, err
	}
	return charged, nil
}

// fetchUnpaidMemberIDs returns the user IDs of all active unpaid members of
// quinielaID, locking the rows for update so concurrent calls serialise.
func fetchUnpaidMemberIDs(ctx context.Context, tx pgx.Tx, quinielaID int) ([]int, error) {
	rows, err := tx.Query(ctx, `
		SELECT gm.user_id
		  FROM group_memberships gm
		 WHERE gm.quiniela_id = $1
		   AND gm.status      = 'active'
		   AND gm.paid        = false
		   AND gm.removed_at  IS NULL
		 ORDER BY gm.joined_at
		   FOR UPDATE OF gm`, quinielaID)
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
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return ids, nil
}

// debitMemberAndMarkPaid deducts amountCents from userID's balance, inserts a
// ledger entry, and flips the membership paid flag — all within the caller's
// transaction. Returns a Conflict error when the balance is insufficient.
func debitMemberAndMarkPaid(ctx context.Context, tx pgx.Tx, quinielaID, userID, amountCents int) error {
	var balanceAfter int
	err := tx.QueryRow(ctx, `
		UPDATE users
		   SET balance_cents = balance_cents - $2,
		       updated_at    = NOW()
		 WHERE id            = $1
		   AND deleted_at    IS NULL
		   AND (balance_cents - reserved_cents) >= $2
		 RETURNING balance_cents`, userID, amountCents).Scan(&balanceAfter)
	if err == pgx.ErrNoRows {
		return insufficientOrNotFound(ctx, tx, userID)
	}
	if err != nil {
		return apperrors.Internal(err)
	}

	if err := insertLedgerTx(ctx, tx, ledgerRow{
		UserID:       userID,
		DeltaCents:   -amountCents,
		Kind:         domain.LedgerKindEntryFee,
		BalanceAfter: balanceAfter,
		RefID:        int64(quinielaID),
		RefType:      "group_membership",
	}); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE group_memberships
		   SET paid       = true,
		       updated_at = NOW()
		 WHERE quiniela_id = $1
		   AND user_id     = $2
		   AND status      = 'active'`, quinielaID, userID)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

var _ GroupMembershipRepository = (*PostgresGroupMembershipRepository)(nil)
