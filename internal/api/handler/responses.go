package handler

import (
	"github.com/rede/world-cup-quiniela/internal/domain"
)

// GroupResponse is the JSON representation of a Quiniela (group).
type GroupResponse struct {
	ID                  int     `json:"id"`
	Name                string  `json:"name"`
	OwnerID             int     `json:"owner_id"`
	InviteCode          string  `json:"invite_code"`
	InviteCodeExpiresAt *string `json:"invite_code_expires_at"` // nil means never expires
	EntryFee            int     `json:"entry_fee"`
	Currency            string  `json:"currency"`
	MaxMembers          *int    `json:"max_members"`
	PrizeThreshold      int     `json:"prize_threshold"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

// LeaderboardEntryResponse is the JSON representation of a single leaderboard entry.
type LeaderboardEntryResponse struct {
	Rank        int    `json:"rank"`
	UserID      int    `json:"user_id"`
	UserName    string `json:"user_name"`
	TotalPoints int    `json:"total_points"`
	PrizeWinner bool   `json:"prize_winner"`
}

// LeaderboardResponse wraps the ranked entries returned by GET …/leaderboard.
type LeaderboardResponse struct {
	QuinielaID int                        `json:"quiniela_id"`
	Phase      string                     `json:"phase,omitempty"` // empty string omitted for the overall leaderboard
	Entries    []LeaderboardEntryResponse `json:"entries"`
}

// MemberResponse is the JSON representation of a GroupMembership.
type MemberResponse struct {
	ID         int     `json:"id"`
	QuinielaID int     `json:"quiniela_id"`
	UserID     int     `json:"user_id"`
	Status     string  `json:"status"`
	Paid       bool    `json:"paid"`
	JoinedAt   *string `json:"joined_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// CountryResponse is the JSON representation of a Country.
type CountryResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// StateResponse is the JSON representation of a State or province.
type StateResponse struct {
	ID      int             `json:"id"`
	Name    string          `json:"name"`
	Code    string          `json:"code"`
	Country CountryResponse `json:"country"`
}

// CityResponse is the JSON representation of a City.
type CityResponse struct {
	ID    int           `json:"id"`
	Name  string        `json:"name"`
	State StateResponse `json:"state"`
}

// StadiumResponse is the JSON representation of a Stadium with its full
// location hierarchy (city → state → country).
type StadiumResponse struct {
	ID       int          `json:"id"`
	Name     string       `json:"name"`
	City     CityResponse `json:"city"`
	Capacity int          `json:"capacity"`
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
	Phase     string           `json:"phase"`
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
		Phase:     string(m.Phase),
		StadiumID: m.StadiumID,
		KickoffAt: m.KickoffAt.Format(timeFormat),
		CreatedAt: m.CreatedAt.Format(timeFormat),
		UpdatedAt: m.UpdatedAt.Format(timeFormat),
	}
	if m.Stadium != nil {
		sr := &StadiumResponse{
			ID:       m.Stadium.ID,
			Name:     m.Stadium.Name,
			Capacity: m.Stadium.Capacity,
		}
		if m.Stadium.City != nil {
			sr.City = CityResponse{ID: m.Stadium.City.ID, Name: m.Stadium.City.Name}
			if m.Stadium.City.State != nil {
				st := m.Stadium.City.State
				sr.City.State = StateResponse{ID: st.ID, Name: st.Name, Code: st.Code}
				if st.Country != nil {
					sr.City.State.Country = CountryResponse{
						ID:   st.Country.ID,
						Name: st.Country.Name,
						Code: st.Country.Code,
					}
				}
			}
		}
		resp.Stadium = sr
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

func groupToResponse(q *domain.Quiniela) GroupResponse {
	resp := GroupResponse{
		ID:             q.ID,
		Name:           q.Name,
		OwnerID:        q.OwnerID,
		InviteCode:     q.InviteCode,
		EntryFee:       q.EntryFee,
		Currency:       q.Currency,
		MaxMembers:     q.MaxMembers,
		PrizeThreshold: q.PrizeThreshold,
		CreatedAt:      q.CreatedAt.Format(timeFormat),
		UpdatedAt:      q.UpdatedAt.Format(timeFormat),
	}
	if q.InviteCodeExpiresAt != nil {
		s := q.InviteCodeExpiresAt.Format(timeFormat)
		resp.InviteCodeExpiresAt = &s
	}
	return resp
}

func memberToResponse(m *domain.GroupMembership) MemberResponse {
	resp := MemberResponse{
		ID:         m.ID,
		QuinielaID: m.QuinielaID,
		UserID:     m.UserID,
		Status:     string(m.Status),
		Paid:       m.Paid,
		CreatedAt:  m.CreatedAt.Format(timeFormat),
		UpdatedAt:  m.UpdatedAt.Format(timeFormat),
	}
	if m.JoinedAt != nil {
		s := m.JoinedAt.Format(timeFormat)
		resp.JoinedAt = &s
	}
	return resp
}
