package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// BulkOperationResult is the outcome of a bulk administrative operation.
// Succeeded holds the IDs of entities that were processed; Failed holds IDs
// that could not be processed (not found or already in a terminal state).
type BulkOperationResult struct {
	Succeeded []int
	Failed    []int
}

// AdminGroupService exposes administrative operations on Quiniela groups that
// are not available to regular members.
//
// All methods require an adminID that is stored in the audit trail. The admin
// role gate is enforced at the HTTP layer via RequireRole - this service does
// not re-check it internally.
type AdminGroupService interface {
	// DeleteGroup soft-deletes the quiniela. Returns NotFound when it does not
	// exist or is already deleted.
	DeleteGroup(ctx context.Context, quinielaID, adminID int) error
	// RemoveMember sets the membership status to 'left'. Returns NotFound for
	// inactive or non-existent memberships.
	RemoveMember(ctx context.Context, membershipID, adminID int) error
	// UpdateGroupSettings changes the entry_fee for a group. Returns the updated Quiniela.
	UpdateGroupSettings(ctx context.Context, quinielaID int, entryFee, adminID int) (*domain.Quiniela, error)
	// TransferOwnership assigns MembershipRoleCreateOwner to newOwnerUserID and
	// demotes the current owner to MembershipRoleMember. Returns NotFound when
	// quinielaID does not exist or newOwnerUserID is not an active member.
	TransferOwnership(ctx context.Context, quinielaID, newOwnerUserID, adminID int) error
	// BulkDeleteGroups soft-deletes multiple quinielas. Succeeded contains IDs
	// that were deleted; Failed contains IDs already deleted or not found.
	BulkDeleteGroups(ctx context.Context, ids []int, adminID int) (BulkOperationResult, error)
	// BulkRemoveMembers sets multiple memberships to 'left'. Only memberships
	// that belong to quinielaID are affected; IDs from other groups are silently
	// ignored. Succeeded contains removed IDs; Failed contains IDs already
	// inactive, not found, or belonging to a different group.
	BulkRemoveMembers(ctx context.Context, quinielaID int, ids []int, adminID int) (BulkOperationResult, error)
	// RecalculateLeaderboard triggers an immediate leaderboard snapshot for the
	// given quiniela. Returns the newly created snapshot.
	RecalculateLeaderboard(ctx context.Context, quinielaID, adminID int) (*domain.LeaderboardSnapshot, error)
	// DistributePrizes credits the prize pool to each ranked winner in quinielaID.
	// The prize pool is EntryFee * ActivePaidMembers divided equally among
	// WinnerCount winners. Users below KYCTierTwo have their share frozen in
	// escrow via KYCService.FreezeBalance instead of being credited immediately.
	DistributePrizes(ctx context.Context, quinielaID, adminID int) error
}

// adminGroupService is the concrete implementation of AdminGroupService.
type adminGroupService struct {
	quinielaRepo  repository.QuinielaRepository
	memberRepo    repository.GroupMembershipRepository
	snapshotter   Snapshotter
	ranker        Ranker
	prizeCrediter PrizeCrediter // wired via SetPrizeCrediter after construction
	audit         AuditLogger
	log           *zap.Logger
}

// NewAdminGroupService constructs an adminGroupService.
func NewAdminGroupService(
	quinielaRepo repository.QuinielaRepository,
	memberRepo repository.GroupMembershipRepository,
	snapshotter Snapshotter,
	ranker Ranker,
	audit AuditLogger,
	log *zap.Logger,
) AdminGroupService {
	return &adminGroupService{
		quinielaRepo: quinielaRepo,
		memberRepo:   memberRepo,
		snapshotter:  snapshotter,
		ranker:       ranker,
		audit:        audit,
		log:          log,
	}
}

// SetPrizeCrediter wires the PrizeCrediter into the service. Called once at
// startup after the KYC module is initialised; nil is safe (DistributePrizes
// returns apperrors.Internal when prizeCrediter has not been set).
func (s *adminGroupService) SetPrizeCrediter(pc PrizeCrediter) { s.prizeCrediter = pc }

func (s *adminGroupService) DeleteGroup(ctx context.Context, quinielaID, adminID int) error {
	if err := s.quinielaRepo.DeleteByAdmin(ctx, quinielaID, adminID); err != nil {
		return err
	}

	resType := "quiniela"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionGroupDeleted, &resType, &quinielaID, nil)
	return nil
}

func (s *adminGroupService) RemoveMember(ctx context.Context, membershipID, adminID int) error {
	if err := s.memberRepo.RemoveByAdmin(ctx, membershipID, adminID); err != nil {
		return err
	}

	resType := "group_membership"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionMemberRemoved, &resType, &membershipID, nil)
	return nil
}

func (s *adminGroupService) UpdateGroupSettings(ctx context.Context, quinielaID int, entryFee, adminID int) (*domain.Quiniela, error) {
	q, err := s.quinielaRepo.UpdateGroupSettings(ctx, quinielaID, entryFee)
	if err != nil {
		return nil, err
	}

	resType := "quiniela"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionGroupSettingsUpdated, &resType, &quinielaID, map[string]any{"entry_fee": entryFee})
	return q, nil
}

// TransferOwnership assigns MembershipRoleCreateOwner to newOwnerUserID and
// demotes the current owner to MembershipRoleMember atomically. Both role
// changes occur in a single database transaction via TransferOwnershipRoles,
// so a partial failure cannot leave the group without an owner.
func (s *adminGroupService) TransferOwnership(ctx context.Context, quinielaID, newOwnerUserID, adminID int) error {
	newMembership, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, newOwnerUserID)
	if err != nil {
		return err
	}
	if newMembership == nil || newMembership.Status != domain.MembershipActive {
		return apperrors.NotFound("new owner must be an active member of this group")
	}

	if err := s.memberRepo.TransferOwnershipRoles(ctx, quinielaID, newMembership.ID); err != nil {
		return err
	}

	resType := "quiniela"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionOwnershipTransferred, &resType, &quinielaID, map[string]any{
		"new_owner_user_id": newOwnerUserID,
	})
	return nil
}

// BulkDeleteGroups soft-deletes multiple quinielas and records a single audit
// entry listing all succeeded and failed IDs.
func (s *adminGroupService) BulkDeleteGroups(ctx context.Context, ids []int, adminID int) (BulkOperationResult, error) {
	succeeded, err := s.quinielaRepo.BulkDeleteByAdmin(ctx, ids, adminID)
	if err != nil {
		return BulkOperationResult{}, err
	}
	result := diffBulkResult(ids, succeeded)
	if len(succeeded) > 0 {
		resType := "quiniela"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &adminID, &role, domain.AuditActionGroupBulkDeleted, &resType, nil, map[string]any{
			"succeeded": result.Succeeded,
			"failed":    result.Failed,
		})
	}
	return result, nil
}

// BulkRemoveMembers sets multiple memberships to 'left' and records a single
// audit entry listing all succeeded and failed IDs. Only memberships that
// belong to quinielaID are affected; the repo filters by quiniela_id to
// prevent cross-group scope bypass.
func (s *adminGroupService) BulkRemoveMembers(ctx context.Context, quinielaID int, ids []int, adminID int) (BulkOperationResult, error) {
	succeeded, err := s.memberRepo.BulkRemoveByAdmin(ctx, quinielaID, ids, adminID)
	if err != nil {
		return BulkOperationResult{}, err
	}
	result := diffBulkResult(ids, succeeded)
	if len(succeeded) > 0 {
		resType := "group_membership"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &adminID, &role, domain.AuditActionMemberBulkRemoved, &resType, nil, map[string]any{
			"succeeded": result.Succeeded,
			"failed":    result.Failed,
		})
	}
	return result, nil
}

// diffBulkResult computes which of the requested ids were not returned in
// succeeded and returns a BulkOperationResult with both slices populated.
func diffBulkResult(ids, succeeded []int) BulkOperationResult {
	succeededSet := make(map[int]bool, len(succeeded))
	for _, id := range succeeded {
		succeededSet[id] = true
	}
	var failed []int
	for _, id := range ids {
		if !succeededSet[id] {
			failed = append(failed, id)
		}
	}
	return BulkOperationResult{Succeeded: succeeded, Failed: failed}
}

// RecalculateLeaderboard triggers an immediate snapshot and records an audit entry.
func (s *adminGroupService) RecalculateLeaderboard(ctx context.Context, quinielaID, adminID int) (*domain.LeaderboardSnapshot, error) {
	snap, err := s.snapshotter.Snapshot(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	resType := "quiniela"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionLeaderboardRefreshed, &resType, &quinielaID, nil)
	return snap, nil
}

// DistributePrizes credits each ranked winner their share of the prize pool.
// Prize pool = EntryFee × ActivePaidMembers, split equally among WinnerCount winners.
//
// Idempotency: prizes_distributed_at is claimed atomically inside
// DistributePrizesAtomically. A second call returns apperrors.Conflict (HTTP 409);
// the caller should surface this to the admin UI as a terminal state.
//
// All direct credits and KYC-freeze holds are executed inside a single transaction:
// if any write fails the transaction rolls back, prizes_distributed_at remains NULL,
// and the entire distribution can be safely retried.
//
// Outbox notifications for frozen winners are fired after the transaction commits
// (best-effort) — a delivery failure does not undo the freeze.
func (s *adminGroupService) DistributePrizes(ctx context.Context, quinielaID, adminID int) error {
	if s.prizeCrediter == nil {
		return apperrors.Internal(fmt.Errorf("prize crediter not configured"))
	}
	q, err := s.quinielaRepo.GetByID(ctx, quinielaID)
	if err != nil {
		return err
	}
	if q == nil {
		return apperrors.NotFound("quiniela not found")
	}
	if q.EntryFee == 0 {
		return apperrors.Conflict("quiniela has no entry fee; no prizes to distribute")
	}
	result, err := s.ranker.GetLeaderboard(ctx, quinielaID)
	if err != nil {
		return err
	}
	if !result.EligibleForPrizes || result.WinnerCount == 0 {
		return apperrors.Conflict("quiniela is not eligible for prize distribution")
	}
	prizePool := q.EntryFee * result.ActivePaidMembers
	prizePerWinner := prizePool / result.WinnerCount

	credits, freezes, freezeUserIDs := buildPrizeAllocations(result.Entries, quinielaID, prizePerWinner)

	if err := s.quinielaRepo.DistributePrizesAtomically(ctx, quinielaID, credits, freezes); err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			s.log.Info("prizes_already_distributed",
				zap.Int("quiniela_id", quinielaID),
			)
		}
		return err
	}

	// Fire outbox notifications for frozen winners outside the transaction.
	// Each call re-freezes idempotently (safe) and writes the outbox event.
	for _, userID := range freezeUserIDs {
		if _, notifyErr := s.prizeCrediter.CreditPrize(ctx, userID, prizePerWinner, int64(quinielaID), "quiniela"); notifyErr != nil {
			s.log.Warn("prize_freeze_notify_failed",
				zap.Int("user_id", userID),
				zap.Int("quiniela_id", quinielaID),
				zap.Error(notifyErr),
			)
		}
	}

	resType := "quiniela"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionPrizesDistributed, &resType, &quinielaID, map[string]any{
		"prize_pool_cents":   prizePool,
		"prize_per_winner":   prizePerWinner,
		"winners_credited":   len(credits),
		"winners_frozen_kyc": len(freezes),
	})
	return nil
}

// buildPrizeAllocations partitions winning leaderboard entries into direct
// credits (KYCTierTwo and above) and KYC-freeze holds (below KYCTierTwo).
func buildPrizeAllocations(entries []*domain.LeaderboardEntry, quinielaID, prizePerWinner int) ([]repository.PrizeCredit, []repository.PrizeFreeze, []int) {
	var credits []repository.PrizeCredit
	var freezes []repository.PrizeFreeze
	var freezeUserIDs []int
	for _, entry := range entries {
		if !entry.PrizeWinner || entry.User == nil {
			continue
		}
		if entry.User.KYCTier >= domain.KYCTierTwo {
			credits = append(credits, repository.PrizeCredit{
				UserID:      entry.User.ID,
				AmountCents: prizePerWinner,
				RefID:       int64(quinielaID),
				RefType:     "quiniela",
			})
		} else {
			reason := fmt.Sprintf(
				"Ha ganado un premio de Q%.2f. Para recibir tus fondos debes completar la verificación "+
					"de identidad (DPI vigente para residentes en Guatemala; pasaporte u equivalente para extranjeros). "+
					"Tu saldo ha sido retenido hasta que el equipo de cumplimiento apruebe tu solicitud.",
				float64(prizePerWinner)/100,
			)
			freezes = append(freezes, repository.PrizeFreeze{
				UserID:      entry.User.ID,
				AmountCents: prizePerWinner,
				Reason:      reason,
			})
			freezeUserIDs = append(freezeUserIDs, entry.User.ID)
		}
	}
	return credits, freezes, freezeUserIDs
}

var _ AdminGroupService = (*adminGroupService)(nil)
