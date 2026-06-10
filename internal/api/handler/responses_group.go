package handler

import "github.com/rede/world-cup-quiniela/internal/domain"

// GroupResponse is the JSON representation of a Quiniela (group).
type GroupResponse struct {
	ID                  int     `json:"id"`
	Name                string  `json:"name"`
	OwnerID             int     `json:"owner_user_id"`
	InviteCode          string  `json:"invite_code"`
	InviteCodeExpiresAt *string `json:"invite_code_expires_at"`
	// Status is system-managed: "active" when the group has ≥ 5 active members,
	// "inactive" otherwise. Only active groups are eligible for payment processing
	// and prize distribution.
	Status   string `json:"status"`
	EntryFee int    `json:"entry_fee"`
	Currency string `json:"currency"`
	// IsPremium is false for free groups (points-only) and true when at least one
	// paid mode is active. Frontend renders this as "Gratis" (gray) or "Premium" (gold).
	IsPremium bool `json:"is_premium"`
	// ModeGeneral and ModeRound reflect which paid sub-modes are active.
	ModeGeneral bool   `json:"mode_general"`
	ModeRound   bool   `json:"mode_round"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// MyGroupResponse is the enriched view returned by GET /api/v1/groups/me.
// It combines the group metadata with the caller's membership status so the
// dashboard can render both the group name and the membership state.
type MyGroupResponse struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	GroupStatus       string `json:"group_status"`
	MembershipStatus  string `json:"membership_status"`
	Role              string `json:"role"`
	InviteCode        string `json:"invite_code"`
	EntryFee          int    `json:"entry_fee"`
	Currency          string `json:"currency"`
	IsPremium         bool   `json:"is_premium"`
	ModeGeneral       bool   `json:"mode_general"`
	ModeRound         bool   `json:"mode_round"`
	ActiveMemberCount int    `json:"active_member_count"`
}

// MemberResponse is the JSON representation of a GroupMembership.
type MemberResponse struct {
	ID          int     `json:"id"`
	QuinielaID  int     `json:"quiniela_id"`
	UserID      int     `json:"user_id"`
	DisplayName string  `json:"display_name"`
	Role        string  `json:"role"`
	Status      string  `json:"status"`
	Paid        bool    `json:"paid"`
	JoinedAt    *string `json:"joined_at"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func groupToResponse(q *domain.Quiniela) GroupResponse {
	resp := GroupResponse{
		ID:          q.ID,
		Name:        q.Name,
		OwnerID:     q.OwnerID,
		InviteCode:  q.InviteCode,
		Status:      string(q.Status),
		EntryFee:    q.EntryFee,
		Currency:    q.Currency,
		IsPremium:   q.IsPremium,
		ModeGeneral: q.ModeGeneral,
		ModeRound:   q.ModeRound,
		CreatedAt:   q.CreatedAt.Format(timeFormat),
		UpdatedAt:   q.UpdatedAt.Format(timeFormat),
	}
	if q.InviteCodeExpiresAt != nil {
		s := q.InviteCodeExpiresAt.Format(timeFormat)
		resp.InviteCodeExpiresAt = &s
	}
	return resp
}

func memberToResponse(m *domain.GroupMembership) MemberResponse {
	resp := MemberResponse{
		ID:          m.ID,
		QuinielaID:  m.QuinielaID,
		UserID:      m.UserID,
		DisplayName: m.DisplayName,
		Role:        string(m.Role),
		Status:      string(m.Status),
		Paid:        m.Paid,
		CreatedAt:   m.CreatedAt.Format(timeFormat),
		UpdatedAt:   m.UpdatedAt.Format(timeFormat),
	}
	if m.JoinedAt != nil {
		s := m.JoinedAt.Format(timeFormat)
		resp.JoinedAt = &s
	}
	return resp
}
