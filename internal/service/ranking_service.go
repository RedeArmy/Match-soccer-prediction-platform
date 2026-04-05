package service

import (
	"context"
	"sort"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// rankingService is the concrete implementation of Ranker.
type rankingService struct {
	quinielaRepo  repository.QuinielaRepository
	predRepo      repository.PredictionRepository
	userRepo      repository.UserRepository
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
// Only predictions with non-nil Points (i.e. the match has been scored) are
// included in the totals. Users with equal points are ordered by their user ID
// as a stable secondary sort until the tiebreaker rule is implemented.
func (s *rankingService) GetLeaderboard(ctx context.Context, quinielaID int) ([]*domain.User, error) {
	quiniela, err := s.quinielaRepo.GetByID(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if quiniela == nil {
		return nil, nil
	}

	// Aggregate points per user from all predictions in this quiniela.
	pointsByUser := make(map[int]int)
	for _, pred := range quiniela.Predictions {
		if pred.Points != nil {
			pointsByUser[pred.UserID] += *pred.Points
		}
	}

	users, err := s.userRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	// Filter to participants (users that have at least one prediction) and sort.
	var ranked []*domain.User
	for _, u := range users {
		if _, ok := pointsByUser[u.ID]; ok {
			ranked = append(ranked, u)
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		pi := pointsByUser[ranked[i].ID]
		pj := pointsByUser[ranked[j].ID]
		if pi != pj {
			return pi > pj // descending points
		}
		return ranked[i].ID < ranked[j].ID // stable secondary sort
	})
	return ranked, nil
}
