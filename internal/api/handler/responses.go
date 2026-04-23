package handler

import (
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

// GroupResponse is the JSON representation of a Quiniela (group).
type GroupResponse struct {
	ID                  int     `json:"id"`
	Name                string  `json:"name"`
	OwnerID             int     `json:"owner_id"`
	InviteCode          string  `json:"invite_code"`
	InviteCodeExpiresAt *string `json:"invite_code_expires_at"` // always nil; invite links never expire
	// Status is system-managed: "active" when the group has ≥ 3 active members,
	// "inactive" otherwise. Only active groups are eligible for payment processing
	// and prize distribution.
	Status         string `json:"status"`
	EntryFee       int    `json:"entry_fee"`
	Currency       string `json:"currency"`
	MaxMembers     *int   `json:"max_members"`
	PrizeThreshold int    `json:"prize_threshold"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// TiebreakerConfigResponse is the JSON representation of the global tiebreaker
// configuration managed by the system administrator.
type TiebreakerConfigResponse struct {
	ID        int    `json:"id"`
	Question  string `json:"question"`
	Result    *int   `json:"result"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// TiebreakerResponse is the JSON representation of a single user tiebreaker prediction.
type TiebreakerResponse struct {
	ID         int    `json:"id"`
	UserID     int    `json:"user_id"`
	Prediction int    `json:"prediction"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// TiebreakerViewResponse is the JSON representation of GetMine: the group's
// question and the caller's own numeric prediction entry.
type TiebreakerViewResponse struct {
	Question *string             `json:"question"`
	Entry    *TiebreakerResponse `json:"entry"`
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
	Role       string  `json:"role"`
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
	ID         int              `json:"id"`
	HomeTeam   string           `json:"home_team"`
	AwayTeam   string           `json:"away_team"`
	HomeScore  *int             `json:"home_score"`
	AwayScore  *int             `json:"away_score"`
	Status     string           `json:"status"`
	Phase      string           `json:"phase"`
	GroupLabel *string          `json:"group_label,omitempty"`
	StadiumID  *int             `json:"stadium_id"`
	Stadium    *StadiumResponse `json:"stadium,omitempty"`
	KickoffAt  string           `json:"kickoff_at"`
	CreatedAt  string           `json:"created_at"`
	UpdatedAt  string           `json:"updated_at"`
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

// ErrorResponse is the standard error envelope returned on all 4xx/5xx responses.
// Defined once in middleware; aliased here so Swagger annotations can reference
// handler.ErrorResponse without import cycles.
type ErrorResponse = middleware.ErrorResponse

// ErrorDetail carries the machine-readable code and human-readable message.
type ErrorDetail = middleware.ErrorDetail

const timeFormat = "2006-01-02T15:04:05Z07:00"

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
		Status:         string(q.Status),
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

func tiebreakerConfigToResponse(cfg *domain.TiebreakerConfig) TiebreakerConfigResponse {
	return TiebreakerConfigResponse{
		ID:        cfg.ID,
		Question:  cfg.Question,
		Result:    cfg.Result,
		CreatedAt: cfg.CreatedAt.Format(timeFormat),
		UpdatedAt: cfg.UpdatedAt.Format(timeFormat),
	}
}

func tiebreakerToResponse(tb *domain.Tiebreaker) TiebreakerResponse {
	return TiebreakerResponse{
		ID:         tb.ID,
		UserID:     tb.UserID,
		Prediction: tb.Prediction,
		CreatedAt:  tb.CreatedAt.Format(timeFormat),
		UpdatedAt:  tb.UpdatedAt.Format(timeFormat),
	}
}

func tiebreakerViewToResponse(view *domain.TiebreakerView) TiebreakerViewResponse {
	resp := TiebreakerViewResponse{Question: view.Question}
	if view.Entry != nil {
		r := tiebreakerToResponse(view.Entry)
		resp.Entry = &r
	}
	return resp
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

func memberToResponse(m *domain.GroupMembership) MemberResponse {
	resp := MemberResponse{
		ID:         m.ID,
		QuinielaID: m.QuinielaID,
		UserID:     m.UserID,
		Role:       string(m.Role),
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

// ── Admin response types ──────────────────────────────────────────────────────

// Paged wraps a paginated list with page metadata.
type Paged[T any] struct {
	Data []T      `json:"data"`
	Page PageMeta `json:"page"`
}

// PageMeta describes the current page of a paginated response.
type PageMeta struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// AdminUserResponse is the admin view of a user account.
type AdminUserResponse struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	Role      string  `json:"role"`
	BannedAt  *string `json:"banned_at,omitempty"`
	BannedBy  *int    `json:"banned_by,omitempty"`
	BanReason string  `json:"ban_reason,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// AdminUserProfileResponse is the full admin view of a user.
type AdminUserProfileResponse struct {
	User        AdminUserResponse `json:"user"`
	Memberships []MemberResponse  `json:"memberships"`
	Payments    []PaymentResponse `json:"payments"`
}

// PaymentResponse is the JSON representation of a PaymentRecord.
type PaymentResponse struct {
	ID          int     `json:"id"`
	QuinielaID  int     `json:"quiniela_id"`
	UserID      int     `json:"user_id"`
	Amount      int     `json:"amount"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
	Reference   *string `json:"reference,omitempty"`
	ReviewedBy  *int    `json:"reviewed_by,omitempty"`
	Notes       string  `json:"notes,omitempty"`
	ConfirmedAt *string `json:"confirmed_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// GlobalLeaderboardEntryResponse is one row of the admin global leaderboard.
type GlobalLeaderboardEntryResponse struct {
	Rank        int    `json:"rank"`
	UserID      int    `json:"user_id"`
	UserName    string `json:"user_name"`
	TotalPoints int    `json:"total_points"`
}

// SnapshotResponse is the JSON representation of a LeaderboardSnapshot.
type SnapshotResponse struct {
	ID         int                     `json:"id"`
	QuinielaID int                     `json:"quiniela_id"`
	TakenAt    string                  `json:"taken_at"`
	Entries    []SnapshotEntryResponse `json:"entries"`
	CreatedAt  string                  `json:"created_at"`
}

// SnapshotEntryResponse is one row within a LeaderboardSnapshot.
type SnapshotEntryResponse struct {
	UserID      int  `json:"user_id"`
	Rank        int  `json:"rank"`
	TotalPoints int  `json:"total_points"`
	PrizeWinner bool `json:"prize_winner"`
}

// AuditLogResponse is the JSON representation of an AuditLog entry.
type AuditLogResponse struct {
	ID           int            `json:"id"`
	ActorID      *int           `json:"actor_id,omitempty"`
	ActorRole    *string        `json:"actor_role,omitempty"`
	Action       string         `json:"action"`
	ResourceType *string        `json:"resource_type,omitempty"`
	ResourceID   *int           `json:"resource_id,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    string         `json:"created_at"`
}

// SystemParamResponse is the JSON representation of a SystemParam.
type SystemParamResponse struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Type      string `json:"type"`
	Category  string `json:"category"`
	IsRuntime bool   `json:"is_runtime"`
	UpdatedAt string `json:"updated_at"`
}

// TiebreakerSubmissionResponse is the admin view of a tiebreaker prediction.
type TiebreakerSubmissionResponse struct {
	ID         int    `json:"id"`
	UserID     int    `json:"user_id"`
	UserName   string `json:"user_name"`
	Prediction int    `json:"prediction"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// ConflictResponse is the JSON representation of a detected operational conflict.
type ConflictResponse struct {
	Type       string         `json:"type"`
	EntityID   int            `json:"entity_id"`
	EntityType string         `json:"entity_type"`
	Details    map[string]any `json:"details,omitempty"`
	DetectedAt string         `json:"detected_at"`
}

// ── Admin converter functions ─────────────────────────────────────────────────

func adminUserToResponse(u *domain.User) AdminUserResponse {
	resp := AdminUserResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		Role:      string(u.Role),
		BanReason: u.BanReason,
		CreatedAt: u.CreatedAt.Format(timeFormat),
		UpdatedAt: u.UpdatedAt.Format(timeFormat),
	}
	if u.BannedAt != nil {
		s := u.BannedAt.Format(timeFormat)
		resp.BannedAt = &s
		resp.BannedBy = u.BannedBy
	}
	return resp
}

func paymentToResponse(p *domain.PaymentRecord) PaymentResponse {
	resp := PaymentResponse{
		ID:         p.ID,
		QuinielaID: p.QuinielaID,
		UserID:     p.UserID,
		Amount:     p.Amount,
		Currency:   p.Currency,
		Status:     string(p.Status),
		Reference:  p.Reference,
		ReviewedBy: p.ReviewedBy,
		Notes:      p.Notes,
		CreatedAt:  p.CreatedAt.Format(timeFormat),
		UpdatedAt:  p.UpdatedAt.Format(timeFormat),
	}
	if p.ConfirmedAt != nil {
		s := p.ConfirmedAt.Format(timeFormat)
		resp.ConfirmedAt = &s
	}
	return resp
}

func snapshotToResponse(s *domain.LeaderboardSnapshot) SnapshotResponse {
	entries := make([]SnapshotEntryResponse, len(s.Entries))
	for i, e := range s.Entries {
		entries[i] = SnapshotEntryResponse{
			UserID:      e.UserID,
			Rank:        e.Rank,
			TotalPoints: e.TotalPoints,
			PrizeWinner: e.PrizeWinner,
		}
	}
	return SnapshotResponse{
		ID:         s.ID,
		QuinielaID: s.QuinielaID,
		TakenAt:    s.TakenAt.Format(timeFormat),
		Entries:    entries,
		CreatedAt:  s.CreatedAt.Format(timeFormat),
	}
}

func auditLogToResponse(a *domain.AuditLog) AuditLogResponse {
	resp := AuditLogResponse{
		ID:           a.ID,
		ActorID:      a.ActorID,
		ResourceType: a.ResourceType,
		ResourceID:   a.ResourceID,
		Action:       a.Action,
		Metadata:     a.Metadata,
		CreatedAt:    a.CreatedAt.Format(timeFormat),
	}
	if a.ActorRole != nil {
		s := string(*a.ActorRole)
		resp.ActorRole = &s
	}
	return resp
}

func systemParamToResponse(p *domain.SystemParam) SystemParamResponse {
	return SystemParamResponse{
		Key:       p.Key,
		Value:     p.Value,
		Type:      string(p.Type),
		Category:  p.Category,
		IsRuntime: p.IsRuntime,
		UpdatedAt: p.UpdatedAt.Format(timeFormat),
	}
}

func conflictToResponse(c domain.Conflict) ConflictResponse {
	return ConflictResponse{
		Type:       string(c.Type),
		EntityID:   c.EntityID,
		EntityType: c.EntityType,
		Details:    c.Details,
		DetectedAt: c.DetectedAt.Format(timeFormat),
	}
}
