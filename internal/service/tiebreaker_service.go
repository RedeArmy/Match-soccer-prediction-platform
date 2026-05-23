package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const errMsgEmptyTiebreakerQuestion = "tiebreaker question cannot be empty"

// TiebreakerService manages tiebreaker question configurations and member
// predictions. A tiebreaker resolves ranking ties when all statistical
// rules still leave two or more members at the same rank.
//
// Lifecycle:
//  1. An administrator calls SetQuestion (global), SetQuestionForPhase, or
//     SetQuestionForQuiniela to define the active question for the relevant
//     scope. Until set, no member may submit a prediction.
//  2. Members call Submit with their numeric estimate.
//     Submit resolves the active config for the caller's group: it checks for
//     a group-specific config first, then falls back to the global config.
//  3. After the tournament, the administrator calls ConfirmResult (global) or
//     ConfirmResultByID (any config). After confirmation, Submit returns Conflict.
//
// Admin gates for write operations are enforced at the HTTP layer via
// RequireRole middleware, not inside this service.
type TiebreakerService interface {
	// SetQuestion stores or replaces the global tiebreaker prompt.
	// Returns Validation when question is empty.
	SetQuestion(ctx context.Context, question string) (*domain.TiebreakerConfig, error)

	// SetQuestionForPhase stores or replaces the phase-scoped question for the
	// given tournament phase. Returns Validation when question is empty or phase
	// is not a recognised FIFA 2026 tournament phase.
	SetQuestionForPhase(ctx context.Context, phase domain.MatchPhase, question string) (*domain.TiebreakerConfig, error)

	// SetQuestionForQuiniela stores or replaces a group-specific question.
	// Returns Validation when question is empty.
	SetQuestionForQuiniela(ctx context.Context, quinielaID int, question string) (*domain.TiebreakerConfig, error)

	// Submit upserts the caller's prediction for the active config of quinielaID.
	// The active config is resolved as: group-specific config → global fallback.
	// Returns Conflict when the result has already been confirmed.
	// Returns Validation when no question has been configured yet.
	// Returns Forbidden when the caller is not an active member of quinielaID.
	Submit(ctx context.Context, quinielaID, callerID, prediction int) (*domain.Tiebreaker, error)

	// GetMine returns the active question and the caller's own prediction for
	// quinielaID. The active config is resolved the same way as Submit.
	// Entry is nil when the caller has not submitted yet.
	// Returns Forbidden when the caller is not an active member of quinielaID.
	GetMine(ctx context.Context, quinielaID, callerID int) (*domain.TiebreakerView, error)

	// ConfirmResult records the official numeric result for the global config.
	// After confirmation, Submit returns Conflict for the global scope.
	// Returns Validation when no global question has been configured yet.
	ConfirmResult(ctx context.Context, result int) error

	// ConfirmResultByID records the official numeric result for any config.
	// Returns NotFound when configID does not exist.
	ConfirmResultByID(ctx context.Context, configID, result int) error
}

// tiebreakerService is the concrete implementation of TiebreakerService.
type tiebreakerService struct {
	configRepo     repository.TiebreakerConfigRepository
	authz          GroupAuthz
	tiebreakerRepo repository.TiebreakerRepository
	audit          AuditLogger
	log            *zap.Logger
}

// NewTiebreakerService constructs a tiebreakerService.
func NewTiebreakerService(
	configRepo repository.TiebreakerConfigRepository,
	authz GroupAuthz,
	tiebreakerRepo repository.TiebreakerRepository,
	audit AuditLogger,
	log *zap.Logger,
) TiebreakerService {
	return &tiebreakerService{
		configRepo:     configRepo,
		authz:          authz,
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
		return nil, apperrors.Validation(errMsgEmptyTiebreakerQuestion)
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
		return nil, apperrors.Validation(errMsgEmptyTiebreakerQuestion)
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
		return nil, apperrors.Validation(errMsgEmptyTiebreakerQuestion)
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
// Concurrent submissions for the same user converge idempotently via the
// database ON CONFLICT DO UPDATE; the closed-result check is intentionally
// done before the upsert so that late submissions after result confirmation
// are rejected.
func (s *tiebreakerService) Submit(ctx context.Context, quinielaID, callerID, prediction int) (*domain.Tiebreaker, error) {
	cfg, err := s.resolveConfigForGroup(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, apperrors.Validation("no tiebreaker question has been configured yet")
	}
	if cfg.Result != nil {
		return nil, apperrors.Conflict("tiebreaker result has already been confirmed - predictions are closed")
	}
	if err := s.authz.RequireActiveMember(ctx, quinielaID, callerID); err != nil {
		return nil, err
	}
	tb := &domain.Tiebreaker{
		UserID:             callerID,
		TiebreakerConfigID: cfg.ID,
		Prediction:         prediction,
	}
	if err := s.tiebreakerRepo.Upsert(ctx, tb); err != nil {
		return nil, err
	}
	return tb, nil
}

// GetMine returns the active question and the caller's own prediction for quinielaID.
func (s *tiebreakerService) GetMine(ctx context.Context, quinielaID, callerID int) (*domain.TiebreakerView, error) {
	if err := s.authz.RequireActiveMember(ctx, quinielaID, callerID); err != nil {
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

var _ TiebreakerService = (*tiebreakerService)(nil)
