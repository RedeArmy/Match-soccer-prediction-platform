package service

import (
	"context"
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// rankingService is the concrete implementation of Ranker.
type rankingService struct {
	quinielaRepo repository.QuinielaRepository
	predRepo     repository.PredictionRepository
	userRepo     repository.UserRepository
	log          *zap.Logger
}

// NewRankingService constructs a rankingService with the given dependencies.
func NewRankingService(
	quinielaRepo repository.QuinielaRepository,
	predRepo repository.PredictionRepository,
	userRepo repository.UserRepository,
	log *zap.Logger,
) Ranker {
	return &rankingService{
		quinielaRepo: quinielaRepo,
		predRepo:     predRepo,
		userRepo:     userRepo,
		log:          log,
	}
}

// GetLeaderboard returns the overall ranked standings for the given quiniela.
//
// Only active, paid members are included. Predictions with nil points (match
// not yet scored) are excluded from the aggregation. Members with no scored
// predictions appear with TotalPoints = 0. PrizeWinner is set to true on
// entries within the prize positions derived from the quiniela's PrizeThreshold.
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

	entries, err := s.buildEntries(ctx, quinielaID, pointsByUser)
	if err != nil {
		return nil, err
	}

	sortAndRank(entries)
	assignPrizes(entries, q.PrizeThreshold)

	return entries, nil
}

// GetPhaseLeaderboard returns standings restricted to predictions on matches in
// the specified tournament phase. The algorithm is identical to GetLeaderboard
// but delegates point aggregation to TotalPointsByQuinielaAndPhase so only
// phase-relevant predictions are counted.
func (s *rankingService) GetPhaseLeaderboard(ctx context.Context, quinielaID int, phase domain.MatchPhase) ([]*domain.LeaderboardEntry, error) {
	q, err := s.quinielaRepo.GetByID(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("quiniela %d not found", quinielaID))
	}

	pointsByUser, err := s.predRepo.TotalPointsByQuinielaAndPhase(ctx, quinielaID, phase)
	if err != nil {
		return nil, err
	}
	if len(pointsByUser) == 0 {
		return nil, nil
	}

	entries, err := s.buildEntries(ctx, quinielaID, pointsByUser)
	if err != nil {
		return nil, err
	}

	sortAndRank(entries)
	assignPrizes(entries, q.PrizeThreshold)

	return entries, nil
}

// buildEntries hydrates LeaderboardEntry values from a userID→points map.
// It fetches all user objects in a single batch query and logs a warning for
// any user ID that is absent from the users table (soft-deleted users).
func (s *rankingService) buildEntries(ctx context.Context, quinielaID int, pointsByUser map[int]int) ([]*domain.LeaderboardEntry, error) {
	userIDs := make([]int, 0, len(pointsByUser))
	for id := range pointsByUser {
		userIDs = append(userIDs, id)
	}
	users, err := s.userRepo.ListByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	userByID := make(map[int]*domain.User, len(users))
	for _, u := range users {
		userByID[u.ID] = u
	}

	entries := make([]*domain.LeaderboardEntry, 0, len(pointsByUser))
	for userID, pts := range pointsByUser {
		u, ok := userByID[userID]
		if !ok {
			s.log.Warn("leaderboard: skipping member absent from users table — likely soft-deleted",
				zap.Int("user_id", userID),
				zap.Int("quiniela_id", quinielaID),
			)
			continue
		}
		entries = append(entries, &domain.LeaderboardEntry{
			User:        u,
			TotalPoints: pts,
		})
	}
	return entries, nil
}

// sortAndRank sorts entries descending by TotalPoints (ties broken by user ID
// for a deterministic ordering) and applies standard competition ranks (1224…).
func sortAndRank(entries []*domain.LeaderboardEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].TotalPoints != entries[j].TotalPoints {
			return entries[i].TotalPoints > entries[j].TotalPoints
		}
		return entries[i].User.ID < entries[j].User.ID
	})
	assignRanks(entries)
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

// assignPrizes marks entries as PrizeWinner = true based on the quiniela's
// prize distribution formula:
//
//	winnerCount = max(1, floor(len(entries) / prizeThreshold))
//
// All entries whose Rank is ≤ winnerCount are prize winners, including tied
// entries that share a rank at the boundary. This means the actual number of
// prize winners may exceed winnerCount when a tie falls on the cut-off rank.
// The entries slice must already be sorted and ranked before this is called.
//
// If prizeThreshold is zero or negative (invalid data, should never reach this
// point after validation), DefaultPrizeThreshold is used as a safe fallback to
// prevent a division-by-zero panic.
func assignPrizes(entries []*domain.LeaderboardEntry, prizeThreshold int) {
	if len(entries) == 0 {
		return
	}
	if prizeThreshold <= 0 {
		prizeThreshold = domain.DefaultPrizeThreshold
	}
	winnerCount := len(entries) / prizeThreshold
	if winnerCount < 1 {
		winnerCount = 1
	}
	// Determine the rank threshold. Because entries are sorted and multiple
	// entries can share a rank, we read the rank of the winnerCount-th entry
	// (0-indexed: winnerCount-1) to include all tied entries at that rank.
	cutoffRank := entries[winnerCount-1].Rank
	for _, e := range entries {
		e.PrizeWinner = e.Rank <= cutoffRank
	}
}
