package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/randcode"
)

// QuinielaService defines operations on the Quiniela entity.
//
// Create generates a unique invite code and records the owner as the first
// active member (MembershipRoleCreateOwner). GetByInviteCode enables the join flow:
// the caller obtains the quiniela from a short code before creating the membership.
//
// The invite code is permanent - it is generated once at creation and never
// rotated. Groups are identified by a stable code for the tournament's duration.
type QuinielaService interface {
	Create(ctx context.Context, quiniela *domain.Quiniela) error
	GetByID(ctx context.Context, id int) (*domain.Quiniela, error)
	GetByInviteCode(ctx context.Context, code string) (*domain.Quiniela, error)
	GetByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error)
	// RenameGroup changes the name of the given group. Only the CreateOwner
	// (MembershipRoleCreateOwner) of the group may call this; any other caller receives
	// Forbidden. Returns the updated Quiniela on success.
	RenameGroup(ctx context.Context, quinielaID, callerUserID int, name string) (*domain.Quiniela, error)
	// SetTournamentMode configures the hybrid payment mode for a group. Only the
	// group owner (MembershipRoleCreateOwner) may call this. The group must have
	// at least MinMembersForActive active members before premium modes can be
	// enabled (is_premium=true). Disabling premium (is_premium=false) is always
	// allowed. Returns the updated Quiniela on success.
	SetTournamentMode(ctx context.Context, quinielaID, callerUserID int, modeGeneral, modeRound bool) (*domain.Quiniela, error)
}

// quinielaService is the concrete implementation of QuinielaService.
type quinielaService struct {
	repo       repository.QuinielaRepository
	memberRepo repository.GroupMembershipRepository
	authz      GroupAuthz
	params     SystemParamService
	audit      AuditLogger
	codeGen    randcode.Generator
}

// NewQuinielaService constructs a quinielaService with the given dependencies.
// authz enforces group ownership in RenameGroup.
// params is used to read group.invite_code_length at runtime.
// audit records rename operations in the audit trail.
func NewQuinielaService(repo repository.QuinielaRepository, authz GroupAuthz, params SystemParamService, audit AuditLogger, codeGen randcode.Generator) QuinielaService {
	return &quinielaService{repo: repo, authz: authz, params: params, audit: audit, codeGen: codeGen}
}

// WithMemberRepo wires a GroupMembershipRepository into the service so that
// SetTournamentMode can charge existing members when a free group goes premium.
func WithMemberRepo(svc QuinielaService, memberRepo repository.GroupMembershipRepository) QuinielaService {
	qs := svc.(*quinielaService)
	qs.memberRepo = memberRepo
	return qs
}

func (s *quinielaService) Create(ctx context.Context, quiniela *domain.Quiniela) error {
	if err := domain.ValidateQuiniela(quiniela); err != nil {
		return err
	}

	length := s.params.GetInt(ctx, domain.ParamKeyGroupInviteCodeLength, domain.DefaultGroupInviteCodeLength)
	code, err := s.codeGen.Generate(length)
	if err != nil {
		return apperrors.Internal(err)
	}
	quiniela.InviteCode = code
	quiniela.InviteCodeExpiresAt = nil // invite links never expire

	// A new group starts inactive: the owner alone is below MinMembersForActive.
	// Status is promoted to active automatically when enough members join and
	// are approved.
	quiniela.Status = domain.QuinielaStatusInactive

	if quiniela.Currency == "" {
		quiniela.Currency = "MXN"
	}

	// The owner is the CreateOwner (MembershipRoleCreateOwner) and becomes an active
	// member immediately - no approval required. Marked as paid for free groups;
	// for paid groups the payment system will flip paid=true after confirmation.
	// Both writes are wrapped in a single transaction via CreateWithMembership.
	now := time.Now().UTC()
	ownerMembership := &domain.GroupMembership{
		UserID:   quiniela.OwnerID,
		Role:     domain.MembershipRoleCreateOwner,
		Status:   domain.MembershipActive,
		Paid:     quiniela.EntryFee == 0,
		JoinedAt: &now,
	}
	return s.repo.CreateWithMembership(ctx, quiniela, ownerMembership)
}

// RenameGroup changes the name of the group. The caller must hold
// MembershipRoleCreateOwner in the group; any other caller receives Forbidden.
func (s *quinielaService) RenameGroup(ctx context.Context, quinielaID, callerUserID int, name string) (*domain.Quiniela, error) {
	if err := s.authz.RequireOwner(ctx, quinielaID, callerUserID); err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperrors.Validation("group name cannot be empty")
	}

	q, err := s.repo.GetByID(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("quiniela %d not found", quinielaID))
	}

	q.Name = name
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, err
	}
	resType := "quiniela"
	role := domain.RoleUser
	s.audit.Log(ctx, &callerUserID, &role, domain.AuditActionGroupRenamed, &resType, &quinielaID, map[string]any{"name": name})
	return q, nil
}

func (s *quinielaService) GetByID(ctx context.Context, id int) (*domain.Quiniela, error) {
	q, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("quiniela %d not found", id))
	}
	return q, nil
}

func (s *quinielaService) GetByInviteCode(ctx context.Context, code string) (*domain.Quiniela, error) {
	q, err := s.repo.GetByInviteCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound("group not found for the given invite code")
	}
	return q, nil
}

func (s *quinielaService) GetByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error) {
	return s.repo.ListByOwner(ctx, ownerID)
}

// SetTournamentMode switches a group between free and premium payment modes.
//
// Constraints:
//   - Caller must hold MembershipRoleCreateOwner.
//   - Enabling premium (modeGeneral=true or modeRound=true) requires that the
//     group already has ≥ MinMembersForActive active members; this prevents
//     activating paid modes in a ghost group with no real participants.
//   - Disabling both modes always succeeds regardless of member count.
//
// When modeGeneral is true, entry_fee is set from
// tournament.general_entry_fee_cents (or DefaultTournamentGeneralEntryFeeCents).
// When only modeRound is active, entry_fee remains 0 (round fees are tracked
// per-round in quiniela_round_entries).
func (s *quinielaService) SetTournamentMode(
	ctx context.Context, quinielaID, callerUserID int, modeGeneral, modeRound bool,
) (*domain.Quiniela, error) {
	if err := s.authz.RequireOwner(ctx, quinielaID, callerUserID); err != nil {
		return nil, err
	}

	isPremium := modeGeneral || modeRound

	if isPremium {
		// Count active members to enforce the minimum before enabling paid modes.
		minMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMinMembers, domain.MinMembersForActive)
		q, err := s.repo.GetByID(ctx, quinielaID)
		if err != nil {
			return nil, err
		}
		if q == nil {
			return nil, apperrors.NotFound(fmt.Sprintf("quiniela %d not found", quinielaID))
		}
		// The group status transitions to active when it reaches minMembers.
		// Checking status == active is an indirect proxy for the member count.
		if q.Status != domain.QuinielaStatusActive {
			return nil, apperrors.Validation(
				fmt.Sprintf("premium mode requires at least %d active members", minMembers),
			)
		}
	}

	// Resolve the general-mode entry fee from the system param.
	entryFee := 0
	if modeGeneral {
		entryFee = s.params.GetInt(ctx, domain.ParamKeyTournamentGeneralEntryFeeCents, domain.DefaultTournamentGeneralEntryFeeCents)
	}

	// When upgrading from free to premium with a non-zero entry fee, charge all
	// existing active members atomically before persisting the mode change.
	if isPremium && entryFee > 0 && s.memberRepo != nil {
		existing, err := s.repo.GetByID(ctx, quinielaID)
		if err != nil {
			return nil, err
		}
		if existing != nil && !existing.IsPremium {
			if _, err := s.memberRepo.BulkDebitAndMarkPaid(ctx, quinielaID, entryFee); err != nil {
				return nil, err
			}
		}
	}

	return s.repo.UpdateTournamentMode(ctx, quinielaID, isPremium, modeGeneral, modeRound, entryFee)
}

var _ QuinielaService = (*quinielaService)(nil)
