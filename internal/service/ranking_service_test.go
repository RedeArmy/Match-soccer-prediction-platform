package service

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// stubUserRepo implements repository.UserRepository for service tests.
type stubUserRepo struct {
	user  *domain.User
	users []*domain.User
	err   error
}

func (r *stubUserRepo) Create(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepo) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) GetByClerkSubject(_ context.Context, _ string) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) Update(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepo) Delete(_ context.Context, _ int) error          { return r.err }
func (r *stubUserRepo) List(_ context.Context) ([]*domain.User, error) { return r.users, r.err }

// ── RankingService tests ──────────────────────────────────────────────────────
//
// GetLeaderboard is a stub pending Phase 3 (group-scoped leaderboard).
// These tests verify the stub contract: it returns nil, nil for any input
// including non-existent quinielas.

func TestGetLeaderboard_ReturnsNil(t *testing.T) {
	svc := NewRankingService(
		&stubQuinielaRepo{},
		&stubPredRepo{},
		&stubUserRepo{},
	)

	ranked, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ranked != nil {
		t.Errorf("expected nil slice (Phase 3 stub), got %v", ranked)
	}
}

func TestGetLeaderboard_QuinielaNotFound_ReturnsNil(t *testing.T) {
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: nil},
		&stubPredRepo{},
		&stubUserRepo{},
	)

	got, err := svc.GetLeaderboard(context.Background(), 99)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown quiniela (stub), got %v", got)
	}
}
