package handler

import "github.com/rede/world-cup-quiniela/internal/domain"

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
