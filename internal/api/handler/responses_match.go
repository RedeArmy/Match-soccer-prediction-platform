package handler

import "github.com/rede/world-cup-quiniela/internal/domain"

// MatchResponse is the JSON representation of a Match returned by the API.
// Domain entities are intentionally free of json tags, so this struct acts
// as an explicit HTTP contract with snake_case field names.
type MatchResponse struct {
	ID         int              `json:"id"`
	HomeTeam   string           `json:"home_team"`
	AwayTeam   string           `json:"away_team"`
	HomeScore  *int             `json:"home_score"`
	AwayScore  *int             `json:"away_score"`
	Status     string           `json:"status"`
	Phase      string           `json:"phase"`
	GroupLabel *string          `json:"group_label,omitempty"`
	WinMethod  *string          `json:"win_method,omitempty"`
	StadiumID  *int             `json:"stadium_id"`
	Stadium    *StadiumResponse `json:"stadium,omitempty"`
	KickoffAt  string           `json:"kickoff_at"`
	CreatedAt  string           `json:"created_at"`
	UpdatedAt  string           `json:"updated_at"`
}

func matchToResponse(m *domain.Match) MatchResponse {
	resp := MatchResponse{
		ID:         m.ID,
		HomeTeam:   m.HomeTeam,
		AwayTeam:   m.AwayTeam,
		HomeScore:  m.HomeScore,
		AwayScore:  m.AwayScore,
		Status:     string(m.Status),
		Phase:      string(m.Phase),
		GroupLabel: m.GroupLabel,
		StadiumID:  m.StadiumID,
		KickoffAt:  m.KickoffAt.Format(timeFormat),
		CreatedAt:  m.CreatedAt.Format(timeFormat),
		UpdatedAt:  m.UpdatedAt.Format(timeFormat),
	}
	if m.WinMethod != nil {
		wm := string(*m.WinMethod)
		resp.WinMethod = &wm
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
