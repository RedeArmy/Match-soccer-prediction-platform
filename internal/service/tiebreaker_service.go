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

// resolveConfigForGroup returns the active TiebreakerConfig for quinielaID.
// It checks for a group-specific config first; if none exists it falls back
// to the platform-wide global config. Returns nil, nil when neither is set.
func (s *tiebreakerService) resolveConfigForGroup(ctx context.Context, quinielaID int) (*domain.TiebreakerConfig, error) {
	cfg, err := s.configRepo.GetByQuiniela(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		return cfg, nil
	}
	return s.configRepo.Get(ctx)
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

// SetQuestionForPhase stores or replaces the phase-scoped question.
func (s *tiebreakerService) SetQuestionForPhase(ctx context.Context, phase domain.MatchPhase, question string) (*domain.TiebreakerConfig, error) {
	if question == "" {
		return nil, apperrors.Validation("tiebreaker question cannot be empty")
	}
	if err := domain.ValidateMatchPhase(phase); err != nil {
		return nil, err
	}
	cfg, err := s.configRepo.UpsertForPhase(ctx, phase, question)
	if err != nil {
		return nil, err
	}

	resType := "tiebreaker_config"
	s.audit.Log(ctx, nil, nil, domain.AuditActionTiebreakerQuestion, &resType, nil, map[string]any{
		"phase":    string(phase),
		"question": question,
	})
	return cfg, nil
}

// SetQuestionForQuiniela stores or replaces the group-specific question.
func (s *tiebreakerService) SetQuestionForQuiniela(ctx context.Context, quinielaID int, question string) (*domain.TiebreakerConfig, error) {
	if question == "" {
		return nil, apperrors.Validation("tiebreaker question cannot be empty")
	}
	cfg, err := s.configRepo.UpsertForQuiniela(ctx, quinielaID, question)
	if err != nil {
		return nil, err
	}

	resType := "tiebreaker_config"
	s.audit.Log(ctx, nil, nil, domain.AuditActionTiebreakerQuestion, &resType, nil, map[string]any{
		"quiniela_id": quinielaID,
		"question":    question,
	})
	return cfg, nil
}

// Submit upserts the caller's prediction for the active config of quinielaID.
// quinielaID is used to verify active membership and resolve the config.
func (s *tiebreakerService) Submit(ctx context.Context, quinielaID, callerID, prediction int) (*domain.Tiebreaker, error) {
	cfg, err := s.resolveConfigForGroup(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, apperrors.Validation("no tiebreaker question has been configured yet")
	}

	if err := s.requireActiveMember(ctx, quinielaID, callerID); err != nil {
		return nil, err
	}

	existing, err := s.tiebreakerRepo.GetByUser(ctx, callerID, cfg.ID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if cfg.Result != nil {
			return nil, apperrors.Conflict("tiebreaker result has already been confirmed - predictions are closed")
		}
		existing.Prediction = prediction
		if err := s.tiebreakerRepo.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	if cfg.Result != nil {
		return nil, apperrors.Conflict("tiebreaker result has already been confirmed - predictions are closed")
	}

	tb := &domain.Tiebreaker{
		UserID:             callerID,
		TiebreakerConfigID: cfg.ID,
		Prediction:         prediction,
	}
	if err := s.tiebreakerRepo.Create(ctx, tb); err != nil {
		return nil, err
	}
	return tb, nil
}

// GetMine returns the active question and the caller's own prediction for quinielaID.
func (s *tiebreakerService) GetMine(ctx context.Context, quinielaID, callerID int) (*domain.TiebreakerView, error) {
	if err := s.requireActiveMember(ctx, quinielaID, callerID); err != nil {
		return nil, err
	}

	cfg, err := s.resolveConfigForGroup(ctx, quinielaID)
	if err != nil {
		return nil, err
	}

	view := &domain.TiebreakerView{}
	if cfg != nil {
		view.Question = &cfg.Question
		tb, err := s.tiebreakerRepo.GetByUser(ctx, callerID, cfg.ID)
		if err != nil {
			return nil, err
		}
		view.Entry = tb
	}
	return view, nil
}

// ConfirmResult records the official numeric result for the global config.
// Returns Validation when no global question has been configured yet.
func (s *tiebreakerService) ConfirmResult(ctx context.Context, result int) error {
	cfg, err := s.configRepo.Get(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		return apperrors.Validation("tiebreaker question has not been configured - cannot confirm result")
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

// ConfirmResultByID records the official numeric result for any config by ID.
// Returns NotFound when configID does not exist.
func (s *tiebreakerService) ConfirmResultByID(ctx context.Context, configID, result int) error {
	if err := s.configRepo.SetResultByID(ctx, configID, result); err != nil {
		return err
	}

	resType := "tiebreaker_config"
	s.audit.Log(ctx, nil, nil, domain.AuditActionTiebreakerResult, &resType, nil, map[string]any{
		"config_id": configID,
		"result":    result,
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
