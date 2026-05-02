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
	Team              *string `json:"team"`
	ConfirmedAt       *string `json:"confirmed_at,omitempty"`
	ConfirmedByUserID *int    `json:"confirmed_by_user_id,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
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
		Team:              s.Team,
		ConfirmedByUserID: s.ConfirmedByUserID,
		CreatedAt:         s.CreatedAt.Format(timeFormat),
		UpdatedAt:         s.UpdatedAt.Format(timeFormat),
	}
	if s.ConfirmedAt != nil {
		t := s.ConfirmedAt.Format(timeFormat)
		resp.ConfirmedAt = &t
	}
	return resp
}
