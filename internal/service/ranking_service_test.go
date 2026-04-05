package service

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// stubUserRepo implements repository.UserRepository with configurable returns.
type stubUserRepo struct {
	user  *domain.User
	users []*domain.User
	err   error
}

func (r *stubUserRepo) Create(_ context.Context, _ *domain.User) error            { return r.err }
func (r *stubUserRepo) GetByID(_ context.Context, _ int) (*domain.User, error)    { return r.user, r.err }
func (r *stubUserRepo) Update(_ context.Context, _ *domain.User) error            { return r.err }
func (r *stubUserRepo) Delete(_ context.Context, _ int) error                     { return r.err }
func (r *stubUserRepo) List(_ context.Context) ([]*domain.User, error)            { return r.users, r.err }

// ── RankingService tests ──────────────────────────────────────────────────────

func TestGetLeaderboard_SortedByPoints(t *testing.T) {
	pts5, pts2 := 5, 2
	quiniela := &domain.Quiniela{
		ID:   1,
		Name: "Test Pool",
		Predictions: []domain.Prediction{
			{UserID: 1, Points: &pts5}, // Alice: 5
			{UserID: 2, Points: &pts2}, // Bob:   2
			{UserID: 1, Points: &pts5}, // Alice: +5 = 10
		},
	}
	users := []*domain.User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: quiniela},
		&stubPredRepo{},
		&stubUserRepo{users: users},
	)

	ranked, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(ranked) != 2 {
		t.Fatalf("expected 2 users, got %d", len(ranked))
	}
	if ranked[0].Name != "Alice" {
		t.Errorf("expected Alice first (10 pts), got %s", ranked[0].Name)
	}
	if ranked[1].Name != "Bob" {
		t.Errorf("expected Bob second (2 pts), got %s", ranked[1].Name)
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
		t.Errorf("expected nil for unknown quiniela, got %v", got)
	}
}

func TestGetLeaderboard_NoPredictions_ReturnsEmpty(t *testing.T) {
	quiniela := &domain.Quiniela{ID: 1, Name: "Empty Pool", Predictions: nil}
	users := []*domain.User{{ID: 1, Name: "Alice"}}
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: quiniela},
		&stubPredRepo{},
		&stubUserRepo{users: users},
	)

	ranked, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(ranked) != 0 {
		t.Errorf("expected empty leaderboard, got %d users", len(ranked))
	}
}

func TestGetLeaderboard_UnscoredPredictions_ExcludedFromRanking(t *testing.T) {
	// Points is nil → match not yet scored → user should not appear in ranking.
	quiniela := &domain.Quiniela{
		ID:          1,
		Predictions: []domain.Prediction{{UserID: 1, Points: nil}},
	}
	users := []*domain.User{{ID: 1, Name: "Alice"}}
	svc := NewRankingService(
		&stubQuinielaRepo{quiniela: quiniela},
		&stubPredRepo{},
		&stubUserRepo{users: users},
	)

	ranked, err := svc.GetLeaderboard(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(ranked) != 0 {
		t.Errorf("expected empty leaderboard for unscored predictions, got %d", len(ranked))
	}
}
