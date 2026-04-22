package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// tiebreakerService is the concrete implementation of TiebreakerService.
type tiebreakerService struct {
	configRepo     repository.TiebreakerConfigRepository
	memberRepo     repository.GroupMembershipRepository
	tiebreakerRepo repository.TiebreakerRepository
	audit          AuditLogger
	log            *zap.Logger
}

// NewTiebreakerService constructs a tiebreakerService.
func NewTiebreakerService(
	configRepo repository.TiebreakerConfigRepository,
	memberRepo repository.GroupMembershipRepository,
	tiebreakerRepo repository.TiebreakerRepository,
	audit AuditLogger,
	log *zap.Logger,
) TiebreakerService {
	return &tiebreakerService{
		configRepo:     configRepo,
		memberRepo:     memberRepo,
		tiebreakerRepo: tiebreakerRepo,
		audit:          audit,
		log:            log,
	}
}

// SetQuestion stores or replaces the global tiebreaker question.
// Returns Validation when question is empty.
func (s *tiebreakerService) SetQuestion(ctx context.Context, question string) (*domain.TiebreakerConfig, error) {
	if question == "" {
		return nil, apperrors.Validation("tiebreaker question cannot be empty")
	}
	cfg, err := s.configRepo.Upsert(ctx, question)
	if err != nil {
		return nil, err
	}

	resType := "tiebreaker_config"
	s.audit.Log(ctx, nil, nil, domain.AuditActionTiebreakerQuestion, &resType, nil, map[string]any{
		"question": question,
	})
	return cfg, nil
}

// Submit upserts the caller's global numeric prediction.
// quinielaID is used only to verify active membership.
func (s *tiebreakerService) Submit(ctx context.Context, quinielaID, callerID, prediction int) (*domain.Tiebreaker, error) {
	cfg, err := s.configRepo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, apperrors.Validation("no tiebreaker question has been configured yet")
	}

	if err := s.requireActiveMember(ctx, quinielaID, callerID); err != nil {
		return nil, err
	}

	existing, err := s.tiebreakerRepo.GetByUser(ctx, callerID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if cfg.Result != nil {
			return nil, apperrors.Conflict("tiebreaker result has already been confirmed — predictions are closed")
		}
		existing.Prediction = prediction
		if err := s.tiebreakerRepo.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	if cfg.Result != nil {
		return nil, apperrors.Conflict("tiebreaker result has already been confirmed — predictions are closed")
	}

	tb := &domain.Tiebreaker{
		UserID:     callerID,
		Prediction: prediction,
	}
	if err := s.tiebreakerRepo.Create(ctx, tb); err != nil {
		return nil, err
	}
	return tb, nil
}

// GetMine returns the global question and the caller's own prediction.
// quinielaID is used only to verify active membership.
func (s *tiebreakerService) GetMine(ctx context.Context, quinielaID, callerID int) (*domain.TiebreakerView, error) {
	if err := s.requireActiveMember(ctx, quinielaID, callerID); err != nil {
		return nil, err
	}

	cfg, err := s.configRepo.Get(ctx)
	if err != nil {
		return nil, err
	}

	tb, err := s.tiebreakerRepo.GetByUser(ctx, callerID)
	if err != nil {
		return nil, err
	}

	view := &domain.TiebreakerView{Entry: tb}
	if cfg != nil {
		view.Question = &cfg.Question
	}
	return view, nil
}

// ConfirmResult records the official numeric result globally.
// Returns Validation when no question has been configured yet.
func (s *tiebreakerService) ConfirmResult(ctx context.Context, result int) error {
	cfg, err := s.configRepo.Get(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		return apperrors.Validation("tiebreaker question has not been configured — cannot confirm result")
	}
	if err := s.configRepo.SetResult(ctx, result); err != nil {
		return err
	}

	resType := "tiebreaker_config"
	s.audit.Log(ctx, nil, nil, domain.AuditActionTiebreakerResult, &resType, nil, map[string]any{
		"result": result,
	})
	return nil
}

// requireActiveMember returns Forbidden when userID is not an active member of
// the quiniela.
func (s *tiebreakerService) requireActiveMember(ctx context.Context, quinielaID, userID int) error {
	m, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, userID)
	if err != nil {
		return err
	}
	if m == nil || m.Status != domain.MembershipActive {
		return apperrors.Forbidden("caller is not an active member of this group")
	}
	return nil
}

var _ TiebreakerService = (*tiebreakerService)(nil)
