package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
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

// GetLeaderboard returns the ranked standings for the given quiniela.
//
// Only active, paid members are included. Predictions with nil points (match
// not yet scored) are excluded from the aggregation. Members with no scored
// predictions appear with TotalPoints = 0.
//
// Ranking algorithm — standard competition ranking (1224…):
// Two members with equal points receive the same rank. The rank after a tie
// of N members at position P is P+N (not P+1). This is the most common
// expectation in tournament contexts.
//
// The implementation is two database round-trips:
//  1. TotalPointsByQuiniela — one SQL query with a LEFT JOIN; O(members).
//  2. ListByIDs — one query using ANY($1); O(members).
//
// No N+1 queries regardless of group size.
func (s *rankingService) GetLeaderboard(ctx context.Context, quinielaID int) ([]*domain.LeaderboardEntry, error) {
	q, err := s.quinielaRepo.GetByID(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("quiniela %d not found", quinielaID))
	}

	// Step 1: total scored points per active+paid member — single SQL join.
	pointsByUser, err := s.predRepo.TotalPointsByQuiniela(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if len(pointsByUser) == 0 {
		return nil, nil
	}

	// Step 2: hydrate user objects — single batch query.
	userIDs := make([]int, 0, len(pointsByUser))
	for id := range pointsByUser {
		userIDs = append(userIDs, id)
	}
	users, err := s.userRepo.ListByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	// Build index for O(1) lookup when assembling entries.
	userByID := make(map[int]*domain.User, len(users))
	for _, u := range users {
		userByID[u.ID] = u
	}

	entries := make([]*domain.LeaderboardEntry, 0, len(pointsByUser))
	for userID, pts := range pointsByUser {
		u, ok := userByID[userID]
		if !ok {
			// User was deleted after membership was created; skip silently.
			continue
		}
		entries = append(entries, &domain.LeaderboardEntry{
			User:        u,
			TotalPoints: pts,
		})
	}

	// Sort descending by total points; break ties by user ID (stable,
	// deterministic ordering so the leaderboard does not shuffle between pages).
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].TotalPoints != entries[j].TotalPoints {
			return entries[i].TotalPoints > entries[j].TotalPoints
		}
		return entries[i].User.ID < entries[j].User.ID
	})

	// Assign standard competition ranks (1224…).
	assignRanks(entries)

	return entries, nil
}

// assignRanks applies standard competition ranking (1224…) to a pre-sorted
// slice of leaderboard entries. Two entries with equal TotalPoints receive the
// same rank; the next rank is skipped accordingly.
func assignRanks(entries []*domain.LeaderboardEntry) {
	for i := 0; i < len(entries); i++ {
		if i == 0 || entries[i].TotalPoints != entries[i-1].TotalPoints {
			entries[i].Rank = i + 1
		} else {
			entries[i].Rank = entries[i-1].Rank
		}
	}
}
