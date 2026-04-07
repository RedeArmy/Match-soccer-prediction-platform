package handler

import (
	"github.com/rede/world-cup-quiniela/internal/domain"
)

// StadiumResponse is the JSON representation of a Stadium.
type StadiumResponse struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	City     string `json:"city"`
	Country  string `json:"country"`
	Capacity int    `json:"capacity"`
}

// MatchResponse is the JSON representation of a Match returned by the API.
// Domain entities are intentionally free of json tags, so this struct acts
// as an explicit HTTP contract with snake_case field names.
type MatchResponse struct {
	ID        int              `json:"id"`
	HomeTeam  string           `json:"home_team"`
	AwayTeam  string           `json:"away_team"`
	HomeScore *int             `json:"home_score"`
	AwayScore *int             `json:"away_score"`
	Status    string           `json:"status"`
	StadiumID *int             `json:"stadium_id"`
	Stadium   *StadiumResponse `json:"stadium,omitempty"`
	KickoffAt string           `json:"kickoff_at"`
	CreatedAt string           `json:"created_at"`
	UpdatedAt string           `json:"updated_at"`
}

// PredictionResponse is the JSON representation of a Prediction.
type PredictionResponse struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	MatchID   int    `json:"match_id"`
	HomeScore int    `json:"home_score"`
	AwayScore int    `json:"away_score"`
	Points    *int   `json:"points"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ErrorResponse is the standard error envelope returned on all 4xx/5xx responses.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries the machine-readable code and human-readable message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

func matchToResponse(m *domain.Match) MatchResponse {
	resp := MatchResponse{
		ID:        m.ID,
		HomeTeam:  m.HomeTeam,
		AwayTeam:  m.AwayTeam,
		HomeScore: m.HomeScore,
		AwayScore: m.AwayScore,
		Status:    string(m.Status),
		StadiumID: m.StadiumID,
		KickoffAt: m.KickoffAt.Format(timeFormat),
		CreatedAt: m.CreatedAt.Format(timeFormat),
		UpdatedAt: m.UpdatedAt.Format(timeFormat),
	}
	if m.Stadium != nil {
		resp.Stadium = &StadiumResponse{
			ID:       m.Stadium.ID,
			Name:     m.Stadium.Name,
			City:     m.Stadium.City,
			Country:  m.Stadium.Country,
			Capacity: m.Stadium.Capacity,
		}
	}
	return resp
}

func predToResponse(p *domain.Prediction) PredictionResponse {
	return PredictionResponse{
		ID:        p.ID,
		UserID:    p.UserID,
		MatchID:   p.MatchID,
		HomeScore: p.HomeScore,
		AwayScore: p.AwayScore,
		Points:    p.Points,
		CreatedAt: p.CreatedAt.Format(timeFormat),
		UpdatedAt: p.UpdatedAt.Format(timeFormat),
	}
}
