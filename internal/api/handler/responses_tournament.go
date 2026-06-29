package handler

import "github.com/rede/world-cup-quiniela/internal/domain"

// GroupStandingResponse is the JSON representation of one team's position in a
// World Cup group.
type GroupStandingResponse struct {
	Group  string `json:"group"`
	Team   string `json:"team"`
	Played int    `json:"played"`
	Won    int    `json:"won"`
	Drawn  int    `json:"drawn"`
	Lost   int    `json:"lost"`
	GF     int    `json:"gf"`
	GC     int    `json:"gc"`
	GD     int    `json:"gd"`
	Points int    `json:"points"`
}

// TournamentStandingsResponse wraps all-group standings for
// GET /api/v1/tournament/standings.
type TournamentStandingsResponse struct {
	Groups map[string][]GroupStandingResponse `json:"groups"`
}

// TournamentSlotResponse is the JSON representation of a bracket position slot.
type TournamentSlotResponse struct {
	ID                int     `json:"id"`
	Label             string  `json:"label"`
	Description       string  `json:"description"`
	Team              *string `json:"team"`
	AutoSource        *string `json:"auto_source,omitempty"`
	ConfirmedAt       *string `json:"confirmed_at,omitempty"`
	ConfirmedByUserID *int    `json:"confirmed_by_user_id,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	MatchKickoffAt    *string `json:"match_kickoff_at,omitempty"`
	MatchHomeScore    *int    `json:"match_home_score,omitempty"`
	MatchAwayScore    *int    `json:"match_away_score,omitempty"`
	MatchStatus       *string `json:"match_status,omitempty"`
	MatchWinMethod    *string `json:"match_win_method,omitempty"`
	PenaltyHomeScore  *int    `json:"penalty_home_score,omitempty"`
	PenaltyAwayScore  *int    `json:"penalty_away_score,omitempty"`
}

func standingToResponse(st *domain.GroupStanding) GroupStandingResponse {
	return GroupStandingResponse{
		Group:  st.Group,
		Team:   st.Team,
		Played: st.Played,
		Won:    st.Won,
		Drawn:  st.Drawn,
		Lost:   st.Lost,
		GF:     st.GF,
		GC:     st.GC,
		GD:     st.GD,
		Points: st.Points,
	}
}

func allStandingsToResponse(grouped map[string][]*domain.GroupStanding) TournamentStandingsResponse {
	out := make(map[string][]GroupStandingResponse, len(grouped))
	for group, entries := range grouped {
		rows := make([]GroupStandingResponse, len(entries))
		for i, e := range entries {
			rows[i] = standingToResponse(e)
		}
		out[group] = rows
	}
	return TournamentStandingsResponse{Groups: out}
}

func slotToResponse(s *domain.TournamentSlot) TournamentSlotResponse {
	resp := TournamentSlotResponse{
		ID:                s.ID,
		Label:             s.Label,
		Description:       s.Description,
		Team:              s.Team,
		AutoSource:        s.AutoSource,
		ConfirmedByUserID: s.ConfirmedByUserID,
		CreatedAt:         s.CreatedAt.Format(timeFormat),
		UpdatedAt:         s.UpdatedAt.Format(timeFormat),
	}
	if s.ConfirmedAt != nil {
		t := s.ConfirmedAt.Format(timeFormat)
		resp.ConfirmedAt = &t
	}
	if s.MatchKickoffAt != nil {
		k := s.MatchKickoffAt.Format(timeFormat)
		resp.MatchKickoffAt = &k
	}
	resp.MatchHomeScore = s.MatchHomeScore
	resp.MatchAwayScore = s.MatchAwayScore
	resp.MatchStatus = s.MatchStatus
	resp.MatchWinMethod = s.MatchWinMethod
	resp.PenaltyHomeScore = s.PenaltyHomeScore
	resp.PenaltyAwayScore = s.PenaltyAwayScore
	return resp
}
