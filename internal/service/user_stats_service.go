package service

import (
	"context"
	"math"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// userStatsService is the concrete implementation of UserStatsService.
type userStatsService struct {
	predRepo repository.PredictionRepository
}

// NewUserStatsService constructs a userStatsService.
func NewUserStatsService(predRepo repository.PredictionRepository) UserStatsService {
	return &userStatsService{predRepo: predRepo}
}

// GetMyStats builds the complete performance profile for the given user.
//
// Three repository calls are made in sequence:
//  1. GetUserPredictionCounts — aggregate counts + total points in one SQL pass.
//  2. GetUserPointsByPhase — phase breakdown for PointsByPhase.
//  3. ListUserScoredPointsChronological — ordered points for streak computation.
//
// AccuracyPct and AvgPointsPerPred are both 0.0 when no predictions have been
// scored yet, avoiding division-by-zero without special-casing in the handler.
func (s *userStatsService) GetMyStats(ctx context.Context, userID int) (*domain.UserStats, error) {
	counts, err := s.predRepo.GetUserPredictionCounts(ctx, userID)
	if err != nil {
		return nil, err
	}

	byPhase, err := s.predRepo.GetUserPointsByPhase(ctx, userID)
	if err != nil {
		return nil, err
	}

	chronoPoints, err := s.predRepo.ListUserScoredPointsChronological(ctx, userID)
	if err != nil {
		return nil, err
	}

	currentStreak, longestStreak := computeStreaks(chronoPoints)

	stats := &domain.UserStats{
		TotalPredictions:   counts.TotalPredictions,
		ScoredPredictions:  counts.ScoredPredictions,
		CorrectPredictions: counts.CorrectPredictions,
		ExactPredictions:   counts.ExactPredictions,
		TotalPoints:        counts.TotalPoints,
		PointsByPhase:      byPhase,
		CurrentStreak:      currentStreak,
		LongestStreak:      longestStreak,
		LastPredictionAt:   counts.LastPredictionAt,
	}

	if counts.ScoredPredictions > 0 {
		stats.AccuracyPct = round2dp(
			float64(counts.CorrectPredictions) / float64(counts.ScoredPredictions) * 100,
		)
		stats.AvgPointsPerPred = round2dp(
			float64(counts.TotalPoints) / float64(counts.ScoredPredictions),
		)
	}

	return stats, nil
}

// computeStreaks derives current and longest correct-prediction streaks from
// an ascending-kickoff-ordered slice of scored point values.
//
// A streak is a run of consecutive scored predictions where points > 0.
// Zero-point predictions break the streak. Unscored predictions are excluded
// from the input by the repository layer so they never interrupt a streak.
//
// current is the unbroken run ending at the most recent scored match.
// longest is the longest unbroken run across the full history.
func computeStreaks(points []int) (current, longest int) {
	// Forward scan for longest.
	run := 0
	for _, p := range points {
		if p > 0 {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	// Backward scan for current (most recent unbroken run).
	for i := len(points) - 1; i >= 0; i-- {
		if points[i] > 0 {
			current++
		} else {
			break
		}
	}
	return
}

// round2dp rounds a float64 to two decimal places.
func round2dp(v float64) float64 {
	return math.Round(v*100) / 100
}
