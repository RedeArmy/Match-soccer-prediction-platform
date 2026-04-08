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
// TODO(Phase 3): The leaderboard must be scoped to group members. Once
// GroupMembership is wired, retrieve the active member list for this quiniela,
// load each member's scored predictions, aggregate points, and sort. For now
// this returns an empty slice to avoid a broken implementation.
func (s *rankingService) GetLeaderboard(_ context.Context, _ int) ([]*domain.User, error) {
	return nil, nil
}
