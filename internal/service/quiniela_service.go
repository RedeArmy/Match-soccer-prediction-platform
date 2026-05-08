package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/codegen"
)

// quinielaService is the concrete implementation of QuinielaService.
type quinielaService struct {
	repo       repository.QuinielaRepository
	memberRepo repository.GroupMembershipRepository
	params     SystemParamService
	audit      AuditLogger
	codeGen    codegen.Generator
}

// NewQuinielaService constructs a quinielaService with the given dependencies.
// memberRepo is required to verify group ownership in RenameGroup.
// params is used to read group.invite_code_length at runtime.
// audit records rename operations in the audit trail.
func NewQuinielaService(repo repository.QuinielaRepository, memberRepo repository.GroupMembershipRepository, params SystemParamService, audit AuditLogger, codeGen codegen.Generator) QuinielaService {
	return &quinielaService{repo: repo, memberRepo: memberRepo, params: params, audit: audit, codeGen: codeGen}
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
	m, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, callerUserID)
	if err != nil {
		return nil, err
	}
	if m == nil || m.Role != domain.MembershipRoleCreateOwner {
		return nil, apperrors.Forbidden("only the group owner can rename the group")
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

var _ QuinielaService = (*quinielaService)(nil)
