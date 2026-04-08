package service

import (
	"context"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// rankingService is the concrete implementation of Ranker.
type rankingService struct {
	quinielaRepo repository.QuinielaRepository
	predRepo     repository.PredictionRepository
	userRepo     repository.UserRepository
}

// NewRankingService constructs a rankingService with the given dependencies.
func NewRankingService(
	quinielaRepo repository.QuinielaRepository,
	predRepo repository.PredictionRepository,
	userRepo repository.UserRepository,
) Ranker {
	return &rankingService{
		quinielaRepo: quinielaRepo,
		predRepo:     predRepo,
		userRepo:     userRepo,
	}
}

// GetLeaderboard returns users ordered by total points for a given quiniela.
//
// Phase 3 — not yet implemented: the leaderboard must be scoped to paid group
// members. Implementation will retrieve the active+paid membership list for
// the quiniela, load each member's scored predictions, aggregate points, and
// sort descending. Returns nil until that phase is complete.
func (s *rankingService) GetLeaderboard(_ context.Context, _ int) ([]*domain.User, error) {
	return nil, nil
}
