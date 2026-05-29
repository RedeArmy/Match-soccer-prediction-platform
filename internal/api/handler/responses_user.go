package handler

import (
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// MeResponse is the JSON representation of the authenticated user's own profile,
// returned by GET /api/v1/users/me. It exposes the fields a user is allowed to
// read and update about themselves: identity, financial state, KYC tier, and
// locale preference.
type MeResponse struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	BalanceCents  int    `json:"balance_cents"`
	ReservedCents int    `json:"reserved_cents"`
	KYCTier       int    `json:"kyc_tier"`
	Locale        string `json:"locale"`
	CreatedAt     string `json:"created_at"`
}

func meToResponse(u *domain.User) MeResponse {
	return MeResponse{
		ID:            u.ID,
		Name:          u.Name,
		Email:         u.Email,
		Role:          string(u.Role),
		BalanceCents:  u.BalanceCents,
		ReservedCents: u.ReservedCents,
		KYCTier:       int(u.KYCTier),
		Locale:        u.Locale,
		CreatedAt:     u.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// UserStatsResponse is the JSON representation of a user's performance profile
// returned by GET /api/v1/users/me/stats.
type UserStatsResponse struct {
	TotalPredictions   int            `json:"total_predictions"`
	ScoredPredictions  int            `json:"scored_predictions"`
	CorrectPredictions int            `json:"correct_predictions"`
	ExactPredictions   int            `json:"exact_predictions"`
	TotalPoints        int            `json:"total_points"`
	PointsByPhase      map[string]int `json:"points_by_phase"`
	AccuracyPct        float64        `json:"accuracy_pct"`
	AvgPointsPerPred   float64        `json:"avg_points_per_prediction"`
	CurrentStreak      int            `json:"current_streak"`
	LongestStreak      int            `json:"longest_streak"`
	LastPredictionAt   *string        `json:"last_prediction_at,omitempty"`
}

func userStatsToResponse(s *domain.UserStats) UserStatsResponse {
	byPhase := make(map[string]int, len(s.PointsByPhase))
	for phase, pts := range s.PointsByPhase {
		byPhase[string(phase)] = pts
	}
	resp := UserStatsResponse{
		TotalPredictions:   s.TotalPredictions,
		ScoredPredictions:  s.ScoredPredictions,
		CorrectPredictions: s.CorrectPredictions,
		ExactPredictions:   s.ExactPredictions,
		TotalPoints:        s.TotalPoints,
		PointsByPhase:      byPhase,
		AccuracyPct:        s.AccuracyPct,
		AvgPointsPerPred:   s.AvgPointsPerPred,
		CurrentStreak:      s.CurrentStreak,
		LongestStreak:      s.LongestStreak,
	}
	if s.LastPredictionAt != nil {
		formatted := s.LastPredictionAt.Format(timeFormat)
		resp.LastPredictionAt = &formatted
	}
	return resp
}
