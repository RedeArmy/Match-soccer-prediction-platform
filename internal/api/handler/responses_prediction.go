package handler

import "github.com/rede/world-cup-quiniela/internal/domain"

// PredictionResponse is the JSON representation of a Prediction.
type PredictionResponse struct {
	ID                 int     `json:"id"`
	UserID             int     `json:"user_id"`
	MatchID            int     `json:"match_id"`
	HomeScore          int     `json:"home_score"`
	AwayScore          int     `json:"away_score"`
	PredictedWinMethod *string `json:"predicted_win_method,omitempty"`
	Points             *int    `json:"points"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

func predToResponse(p *domain.Prediction) PredictionResponse {
	resp := PredictionResponse{
		ID:        p.ID,
		UserID:    p.UserID,
		MatchID:   p.MatchID,
		HomeScore: p.HomeScore,
		AwayScore: p.AwayScore,
		Points:    p.Points,
		CreatedAt: p.CreatedAt.Format(timeFormat),
		UpdatedAt: p.UpdatedAt.Format(timeFormat),
	}
	if p.PredictedWinMethod != nil {
		wm := string(*p.PredictedWinMethod)
		resp.PredictedWinMethod = &wm
	}
	return resp
}
