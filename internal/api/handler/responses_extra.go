package handler

import "github.com/rede/world-cup-quiniela/internal/domain"

// ExtraPredictionResponse is the JSON representation of an ExtraPrediction.
type ExtraPredictionResponse struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	MatchID   int    `json:"match_id"`
	ExtraType string `json:"extra_type"`
	Answer    string `json:"answer"`
	Points    *int   `json:"points"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func extraPredictionToResponse(p *domain.ExtraPrediction) ExtraPredictionResponse {
	return ExtraPredictionResponse{
		ID:        p.ID,
		UserID:    p.UserID,
		MatchID:   p.MatchID,
		ExtraType: string(p.ExtraType),
		Answer:    p.Answer,
		Points:    p.Points,
		CreatedAt: p.CreatedAt.Format(timeFormat),
		UpdatedAt: p.UpdatedAt.Format(timeFormat),
	}
}
