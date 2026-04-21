package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// stubTiebreakerConfigRepo implements repository.TiebreakerConfigRepository.
type stubTiebreakerConfigRepo struct {
	cfg       *domain.TiebreakerConfig
	upsertErr error
	setResErr error
	getErr    error
}

func (r *stubTiebreakerConfigRepo) Get(_ context.Context) (*domain.TiebreakerConfig, error) {
	return r.cfg, r.getErr
}
func (r *stubTiebreakerConfigRepo) Upsert(_ context.Context, question string) (*domain.TiebreakerConfig, error) {
	if r.upsertErr != nil {
		return nil, r.upsertErr
	}
	cfg := &domain.TiebreakerConfig{ID: 1, Question: question}
	return cfg, nil
}
func (r *stubTiebreakerConfigRepo) SetResult(_ context.Context, _ int) error {
	return r.setResErr
}

// stubTiebreakerRepoSvc implements repository.TiebreakerRepository.
type stubTiebreakerRepoSvc struct {
	existing  *domain.Tiebreaker
	err       error
	createErr error
	updateErr error
}

func (r *stubTiebreakerRepoSvc) Create(_ context.Context, tb *domain.Tiebreaker) error {
	if r.createErr != nil {
		return r.createErr
	}
	tb.ID = 99
	return nil
}
func (r *stubTiebreakerRepoSvc) GetByUser(_ context.Context, _ int) (*domain.Tiebreaker, error) {
	return r.existing, r.err
}
func (r *stubTiebreakerRepoSvc) Update(_ context.Context, _ *domain.Tiebreaker) error {
	return r.updateErr
}
func (r *stubTiebreakerRepoSvc) ListByUserIDs(_ context.Context, _ []int) ([]*domain.Tiebreaker, error) {
	return nil, r.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newTiebreakerSvc(cfg *domain.TiebreakerConfig, m *domain.GroupMembership, tbRepo *stubTiebreakerRepoSvc) TiebreakerService {
	return NewTiebreakerService(
		&stubTiebreakerConfigRepo{cfg: cfg},
		&stubMemberRepo{membership: m},
		tbRepo,
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
		&stubMemberRepo{},
		&stubTiebreakerRepoSvc{},
		zap.NewNop(),
	)

	got, err := svc.SetQuestion(context.Background(), "Total goals in the Final")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Question != "Total goals in the Final" {
		t.Errorf("expected question to be set, got %v", got)
	}
}

func TestTiebreakerService_SetQuestion_EmptyQuestion_ReturnsValidation(t *testing.T) {
	svc := newTiebreakerSvc(nil, nil, &stubTiebreakerRepoSvc{})

	_, err := svc.SetQuestion(context.Background(), "")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestTiebreakerService_Submit_NewEntry_CreatesAndReturns(t *testing.T) {
	cfg := configWithQuestion("Total goals in the Final")
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{existing: nil})

	tb, err := svc.Submit(context.Background(), 1, 42, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tb.Prediction != 5 {
		t.Errorf("expected prediction 5, got %d", tb.Prediction)
	}
}

func TestTiebreakerService_Submit_ExistingEntry_UpdatesAndReturns(t *testing.T) {
	cfg := configWithQuestion("Total goals in the Final")
	existing := &domain.Tiebreaker{ID: 3, UserID: 42, Prediction: 3}
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{existing: existing})

	tb, err := svc.Submit(context.Background(), 1, 42, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tb.Prediction != 7 {
		t.Errorf("expected updated prediction 7, got %d", tb.Prediction)
	}
}

func TestTiebreakerService_Submit_ResultAlreadyConfirmed_ReturnsConflict(t *testing.T) {
	result := 10
	cfg := &domain.TiebreakerConfig{ID: 1, Question: "Total goals", Result: &result}
	existing := &domain.Tiebreaker{ID: 3, UserID: 42, Prediction: 8}
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{existing: existing})

	_, err := svc.Submit(context.Background(), 1, 42, 9)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestTiebreakerService_Submit_NoQuestion_ReturnsValidation(t *testing.T) {
	svc := newTiebreakerSvc(nil, activeM(42), &stubTiebreakerRepoSvc{})

	_, err := svc.Submit(context.Background(), 1, 42, 5)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestTiebreakerService_Submit_NotActiveMember_ReturnsForbidden(t *testing.T) {
	cfg := configWithQuestion("Total goals in the Final")
	svc := newTiebreakerSvc(cfg, nil, &stubTiebreakerRepoSvc{})

	_, err := svc.Submit(context.Background(), 1, 99, 5)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

// ── GetMine ───────────────────────────────────────────────────────────────────

func TestTiebreakerService_GetMine_ActiveMember_ReturnsView(t *testing.T) {
	cfg := configWithQuestion("Total goals in the Final")
	tb := &domain.Tiebreaker{ID: 3, UserID: 42, Prediction: 5}
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{existing: tb})

	view, err := svc.GetMine(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Question == nil {
		t.Fatal("expected non-nil question")
	}
	if view.Entry == nil || view.Entry.Prediction != 5 {
		t.Errorf("expected entry with prediction 5, got %v", view.Entry)
	}
}

func TestTiebreakerService_GetMine_NoEntry_EntryIsNil(t *testing.T) {
	cfg := configWithQuestion("Total goals in the Final")
	svc := newTiebreakerSvc(cfg, activeM(42), &stubTiebreakerRepoSvc{existing: nil})

	view, err := svc.GetMine(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Entry != nil {
		t.Errorf("expected nil entry when not submitted, got %v", view.Entry)
	}
}

func TestTiebreakerService_GetMine_NotActiveMember_ReturnsForbidden(t *testing.T) {
	cfg := configWithQuestion("Total goals in the Final")
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
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Question != nil {
		t.Errorf("expected nil question when not configured, got %v", view.Question)
	}
}

// ── ConfirmResult ─────────────────────────────────────────────────────────────

func TestTiebreakerService_ConfirmResult_Succeeds(t *testing.T) {
	cfg := configWithQuestion("Total goals in the Final")
	svc := newTiebreakerSvc(cfg, nil, &stubTiebreakerRepoSvc{})

	if err := svc.ConfirmResult(context.Background(), 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTiebreakerService_ConfirmResult_NoQuestion_ReturnsValidation(t *testing.T) {
	svc := newTiebreakerSvc(nil, nil, &stubTiebreakerRepoSvc{})

	err := svc.ConfirmResult(context.Background(), 10)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestTiebreakerService_ConfirmResult_RepoError_Propagates(t *testing.T) {
	cfg := configWithQuestion("Total goals in the Final")
	svc := NewTiebreakerService(
		&stubTiebreakerConfigRepo{cfg: cfg, setResErr: errors.New("db error")},
		&stubMemberRepo{},
		&stubTiebreakerRepoSvc{},
		zap.NewNop(),
	)

	if err := svc.ConfirmResult(context.Background(), 10); err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}
