package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	tiebreakerQuestion      = "Total goals"
	tiebreakerUnexpectedErr = "unexpected error: %v"
	tiebreakerValidationFmt = "expected validation error, got %v"
)

// stubTiebreakerConfigRepo implements repository.TiebreakerConfigRepository.
type stubTiebreakerConfigRepo struct {
	cfg         *domain.TiebreakerConfig
	quinielaCfg *domain.TiebreakerConfig // non-nil → GetByQuiniela returns this
	upsertErr   error
	setResErr   error
	getErr      error
}

func (r *stubTiebreakerConfigRepo) Get(_ context.Context) (*domain.TiebreakerConfig, error) {
	return r.cfg, r.getErr
}
func (r *stubTiebreakerConfigRepo) GetByPhase(_ context.Context, _ domain.MatchPhase) (*domain.TiebreakerConfig, error) {
	return nil, r.getErr
}
func (r *stubTiebreakerConfigRepo) GetByQuiniela(_ context.Context, _ int) (*domain.TiebreakerConfig, error) {
	if r.quinielaCfg != nil {
		return r.quinielaCfg, nil
	}
	return nil, r.getErr // nil = no group-specific config; service falls back to global
}
func (r *stubTiebreakerConfigRepo) Upsert(_ context.Context, question string) (*domain.TiebreakerConfig, error) {
	if r.upsertErr != nil {
		return nil, r.upsertErr
	}
	cfg := &domain.TiebreakerConfig{ID: 1, Question: question}
	return cfg, nil
}
func (r *stubTiebreakerConfigRepo) UpsertForPhase(_ context.Context, phase domain.MatchPhase, question string) (*domain.TiebreakerConfig, error) {
	if r.upsertErr != nil {
		return nil, r.upsertErr
	}
	return &domain.TiebreakerConfig{ID: 2, Question: question, Phase: &phase}, nil
}
func (r *stubTiebreakerConfigRepo) UpsertForQuiniela(_ context.Context, quinielaID int, question string) (*domain.TiebreakerConfig, error) {
	if r.upsertErr != nil {
		return nil, r.upsertErr
	}
	return &domain.TiebreakerConfig{ID: 3, Question: question, QuinielaID: &quinielaID}, nil
}
func (r *stubTiebreakerConfigRepo) SetResult(_ context.Context, _ int) error {
	return r.setResErr
}
func (r *stubTiebreakerConfigRepo) SetResultByID(_ context.Context, _, _ int) error {
	return r.setResErr
}

// stubTiebreakerRepoSvc implements repository.TiebreakerRepository.
type stubTiebreakerRepoSvc struct {
	existing *domain.Tiebreaker
	err      error
	upsertID int // ID stamped onto the tiebreaker by Upsert; 0 → uses 99
}

func (r *stubTiebreakerRepoSvc) Upsert(_ context.Context, tb *domain.Tiebreaker) error {
	if r.err != nil {
		return r.err
	}
	id := r.upsertID
	if id == 0 {
		id = 99
	}
	tb.ID = id
	return nil
}
func (r *stubTiebreakerRepoSvc) GetByUser(_ context.Context, _, _ int) (*domain.Tiebreaker, error) {
	return r.existing, r.err
}
func (r *stubTiebreakerRepoSvc) Update(_ context.Context, _ *domain.Tiebreaker) error {
	return r.err
}
func (r *stubTiebreakerRepoSvc) ListByUserIDs(_ context.Context, _ []int) ([]*domain.Tiebreaker, error) {
	return nil, r.err
}
func (r *stubTiebreakerRepoSvc) ListByUserIDsForConfig(_ context.Context, _ []int, _ int) ([]*domain.Tiebreaker, error) {
	return nil, r.err
}
func (r *stubTiebreakerRepoSvc) ListAll(_ context.Context, _ repository.Pagination) ([]*domain.Tiebreaker, error) {
	return nil, r.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

// newTiebreakerSvc builds a TiebreakerService for unit tests. m drives the
// active-member authz check: non-nil active membership → allowed; otherwise
// RequireActiveMember returns Forbidden.
func newTiebreakerSvc(cfg *domain.TiebreakerConfig, m *domain.GroupMembership, tbRepo *stubTiebreakerRepoSvc) TiebreakerService {
	authz := newGroupAuthz(&stubMemberRepo{membership: m})
	return NewTiebreakerService(
		&stubTiebreakerConfigRepo{cfg: cfg},
		authz,
		tbRepo,
		&noopAuditLogger{},
		zap.NewNop(),
	)
}

func configWithQuestion(question string) *domain.TiebreakerConfig {
	return &domain.TiebreakerConfig{ID: 1, Question: question}
}

func activeM(userID int) *domain.GroupMembership {
	return &domain.GroupMembership{
		ID:         10,
		QuinielaID: 1,
		UserID:     userID,
		Status:     domain.MembershipActive,
	}
}

// ── SetQuestion ───────────────────────────────────────────────────────────────

func TestTiebreakerService_SetQuestion_StoresQuestion(t *testing.T) {
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{},
		newGroupAuthz(&stubMemberRepo{}),
		&stubTiebreakerRepoSvc{},
		&noopAuditLogger{},
		zap.NewNop(),
	)

	got, err := svc.SetQuestion(context.Background(), tiebreakerQuestion)
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if got == nil || got.Question != tiebreakerQuestion {
		t.Errorf("expected question to be set, got %v", got)
	}
}

func TestTiebreakerService_SetQuestion_EmptyQuestion_ReturnsValidation(t *testing.T) {
	svc := newTiebreakerSvc(nil, nil, &stubTiebreakerRepoSvc{})

	_, err := svc.SetQuestion(context.Background(), "")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf(tiebreakerValidationFmt, err)
	}
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestTiebreakerService_Submit_NewEntry_CreatesAndReturns(t *testing.T) {
	cfg := configWithQuestion(tiebreakerQuestion)
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{existing: nil})

	tb, err := svc.Submit(context.Background(), 1, 42, 5)
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if tb.Prediction != 5 {
		t.Errorf("expected prediction 5, got %d", tb.Prediction)
	}
}

func TestTiebreakerService_Submit_ExistingEntry_UpsertUpdates(t *testing.T) {
	cfg := configWithQuestion(tiebreakerQuestion)
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{})

	tb, err := svc.Submit(context.Background(), 1, 42, 7)
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if tb.Prediction != 7 {
		t.Errorf("expected prediction 7, got %d", tb.Prediction)
	}
}

func TestTiebreakerService_Submit_ResultAlreadyConfirmed_ReturnsConflict(t *testing.T) {
	result := 10
	cfg := &domain.TiebreakerConfig{ID: 1, Question: "Total goals", Result: &result}
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{})

	_, err := svc.Submit(context.Background(), 1, 42, 9)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestTiebreakerService_Submit_NoQuestion_ReturnsValidation(t *testing.T) {
	svc := newTiebreakerSvc(nil, activeM(42), &stubTiebreakerRepoSvc{})

	_, err := svc.Submit(context.Background(), 1, 42, 5)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf(tiebreakerValidationFmt, err)
	}
}

func TestTiebreakerService_Submit_NotActiveMember_ReturnsForbidden(t *testing.T) {
	cfg := configWithQuestion(tiebreakerQuestion)
	svc := newTiebreakerSvc(cfg, nil, &stubTiebreakerRepoSvc{})

	_, err := svc.Submit(context.Background(), 1, 99, 5)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestTiebreakerService_Submit_UpsertError_Propagates(t *testing.T) {
	cfg := configWithQuestion(tiebreakerQuestion)
	tbRepo := &stubTiebreakerRepoSvc{err: errors.New("db down")}
	svc := newTiebreakerSvc(cfg, activeM(42), tbRepo)

	_, err := svc.Submit(context.Background(), 1, 42, 3)
	if err == nil {
		t.Fatal("expected error from Upsert, got nil")
	}
}

// ── GetMine ───────────────────────────────────────────────────────────────────

func TestTiebreakerService_GetMine_ActiveMember_ReturnsView(t *testing.T) {
	cfg := configWithQuestion(tiebreakerQuestion)
	tb := &domain.Tiebreaker{ID: 3, UserID: 42, Prediction: 5}
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{existing: tb})

	view, err := svc.GetMine(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if view.Question == nil {
		t.Fatal("expected non-nil question")
	}
	if view.Entry == nil || view.Entry.Prediction != 5 {
		t.Errorf("expected entry with prediction 5, got %v", view.Entry)
	}
}

func TestTiebreakerService_GetMine_NoEntry_EntryIsNil(t *testing.T) {
	cfg := configWithQuestion(tiebreakerQuestion)
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{existing: nil})

	view, err := svc.GetMine(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if view.Entry != nil {
		t.Errorf("expected nil entry when not submitted, got %v", view.Entry)
	}
}

func TestTiebreakerService_GetMine_NotActiveMember_ReturnsForbidden(t *testing.T) {
	cfg := configWithQuestion(tiebreakerQuestion)
	svc := newTiebreakerSvc(cfg, nil, &stubTiebreakerRepoSvc{})

	_, err := svc.GetMine(context.Background(), 1, 99)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestTiebreakerService_GetMine_NoQuestionConfigured_QuestionIsNil(t *testing.T) {
	svc := newTiebreakerSvc(nil, activeM(42), &stubTiebreakerRepoSvc{})

	view, err := svc.GetMine(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if view.Question != nil {
		t.Errorf("expected nil question when not configured, got %v", view.Question)
	}
}

// ── ConfirmResult ─────────────────────────────────────────────────────────────

func TestTiebreakerService_ConfirmResult_Succeeds(t *testing.T) {
	cfg := configWithQuestion(tiebreakerQuestion)
	svc := newTiebreakerSvc(cfg, nil, &stubTiebreakerRepoSvc{})

	if err := svc.ConfirmResult(context.Background(), 10); err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
}

func TestTiebreakerService_ConfirmResult_NoQuestion_ReturnsValidation(t *testing.T) {
	svc := newTiebreakerSvc(nil, nil, &stubTiebreakerRepoSvc{})

	err := svc.ConfirmResult(context.Background(), 10)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf(tiebreakerValidationFmt, err)
	}
}

// ── SetQuestionForPhase ───────────────────────────────────────────────────────

func TestTiebreakerService_SetQuestionForPhase_StoresQuestion(t *testing.T) {
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{},
		newGroupAuthz(&stubMemberRepo{}),
		&stubTiebreakerRepoSvc{},
		&noopAuditLogger{},
		zap.NewNop(),
	)

	got, err := svc.SetQuestionForPhase(context.Background(), domain.PhaseGroupStage, "Goals in group stage?")
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if got == nil || got.Question != "Goals in group stage?" {
		t.Errorf("expected question to be set, got %v", got)
	}
}

func TestTiebreakerService_SetQuestionForPhase_EmptyQuestion_ReturnsValidation(t *testing.T) {
	svc := newTiebreakerSvc(nil, nil, &stubTiebreakerRepoSvc{})
	_, err := svc.SetQuestionForPhase(context.Background(), domain.PhaseGroupStage, "")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf(tiebreakerValidationFmt, err)
	}
}

func TestTiebreakerService_SetQuestionForPhase_InvalidPhase_ReturnsValidation(t *testing.T) {
	svc := newTiebreakerSvc(nil, nil, &stubTiebreakerRepoSvc{})
	_, err := svc.SetQuestionForPhase(context.Background(), "not_a_phase", "Some question?")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf(tiebreakerValidationFmt, err)
	}
}

func TestTiebreakerService_SetQuestionForPhase_RepoError_Propagates(t *testing.T) {
	repoErr := errors.New("db error")
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{upsertErr: repoErr},
		newGroupAuthz(&stubMemberRepo{}),
		&stubTiebreakerRepoSvc{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
	_, err := svc.SetQuestionForPhase(context.Background(), domain.PhaseGroupStage, "Some question?")
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── SetQuestionForQuiniela ────────────────────────────────────────────────────

func TestTiebreakerService_SetQuestionForQuiniela_StoresQuestion(t *testing.T) {
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{},
		newGroupAuthz(&stubMemberRepo{}),
		&stubTiebreakerRepoSvc{},
		&noopAuditLogger{},
		zap.NewNop(),
	)

	got, err := svc.SetQuestionForQuiniela(context.Background(), 1, "Group-specific question?")
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if got == nil || got.Question != "Group-specific question?" {
		t.Errorf("expected question to be set, got %v", got)
	}
}

func TestTiebreakerService_SetQuestionForQuiniela_EmptyQuestion_ReturnsValidation(t *testing.T) {
	svc := newTiebreakerSvc(nil, nil, &stubTiebreakerRepoSvc{})
	_, err := svc.SetQuestionForQuiniela(context.Background(), 1, "")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf(tiebreakerValidationFmt, err)
	}
}

func TestTiebreakerService_SetQuestionForQuiniela_RepoError_Propagates(t *testing.T) {
	repoErr := errors.New("db error")
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{upsertErr: repoErr},
		newGroupAuthz(&stubMemberRepo{}),
		&stubTiebreakerRepoSvc{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
	_, err := svc.SetQuestionForQuiniela(context.Background(), 1, "Some question?")
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── ConfirmResultByID ─────────────────────────────────────────────────────────

func TestTiebreakerService_ConfirmResultByID_Succeeds(t *testing.T) {
	svc := newTiebreakerSvc(nil, nil, &stubTiebreakerRepoSvc{})
	if err := svc.ConfirmResultByID(context.Background(), 2, 42); err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
}

func TestTiebreakerService_ConfirmResultByID_RepoError_Propagates(t *testing.T) {
	repoErr := errors.New("db error")
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{setResErr: repoErr},
		newGroupAuthz(&stubMemberRepo{}),
		&stubTiebreakerRepoSvc{},
		&noopAuditLogger{},
		zap.NewNop(),
	)
	if err := svc.ConfirmResultByID(context.Background(), 2, 42); err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── resolveConfigForGroup ─────────────────────────────────────────────────────

func TestTiebreakerService_Submit_GroupConfigTakesPrecedence(t *testing.T) {
	groupCfg := &domain.TiebreakerConfig{ID: 99, Question: "group question"}
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{quinielaCfg: groupCfg},
		newGroupAuthz(&stubMemberRepo{membership: activeM(42)}),
		&stubTiebreakerRepoSvc{},
		&noopAuditLogger{},
		zap.NewNop(),
	)

	tb, err := svc.Submit(context.Background(), 1, 42, 5)
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if tb.TiebreakerConfigID != groupCfg.ID {
		t.Errorf("expected config ID %d (group), got %d", groupCfg.ID, tb.TiebreakerConfigID)
	}
}

func TestTiebreakerService_Submit_ResolveConfigError_Propagates(t *testing.T) {
	repoErr := errors.New("db error")
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{getErr: repoErr},
		newGroupAuthz(&stubMemberRepo{membership: activeM(42)}),
		&stubTiebreakerRepoSvc{},
		&noopAuditLogger{},
		zap.NewNop(),
	)

	_, err := svc.Submit(context.Background(), 1, 42, 5)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

func TestTiebreakerService_GetMine_GroupConfigTakesPrecedence(t *testing.T) {
	groupCfg := &domain.TiebreakerConfig{ID: 99, Question: "group question"}
	tb := &domain.Tiebreaker{ID: 1, UserID: 42, TiebreakerConfigID: 99, Prediction: 3}
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{quinielaCfg: groupCfg},
		newGroupAuthz(&stubMemberRepo{membership: activeM(42)}),
		&stubTiebreakerRepoSvc{existing: tb},
		&noopAuditLogger{},
		zap.NewNop(),
	)

	view, err := svc.GetMine(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf(tiebreakerUnexpectedErr, err)
	}
	if view.Question == nil || *view.Question != "group question" {
		t.Errorf("expected group question, got %v", view.Question)
	}
}

func TestTiebreakerService_Submit_NewEntry_ResultConfirmed_ReturnsConflict(t *testing.T) {
	result := 10
	cfg := &domain.TiebreakerConfig{ID: 1, Question: "Total goals", Result: &result}
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{})

	_, err := svc.Submit(context.Background(), 1, 42, 9)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict when result already confirmed, got %v", err)
	}
}

func TestTiebreakerService_ConfirmResult_RepoError_Propagates(t *testing.T) {
	cfg := configWithQuestion(tiebreakerQuestion)
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{cfg: cfg, setResErr: errors.New("db error")},
		newGroupAuthz(&stubMemberRepo{}),
		&stubTiebreakerRepoSvc{},
		&noopAuditLogger{},
		zap.NewNop(),
	)

	if err := svc.ConfirmResult(context.Background(), 10); err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}
