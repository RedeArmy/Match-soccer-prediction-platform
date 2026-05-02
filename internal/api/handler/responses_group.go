package handler

import "github.com/rede/world-cup-quiniela/internal/domain"

// GroupResponse is the JSON representation of a Quiniela (group).
type GroupResponse struct {
	ID                  int     `json:"id"`
	Name                string  `json:"name"`
	OwnerID             int     `json:"owner_user_id"`
	InviteCode          string  `json:"invite_code"`
	InviteCodeExpiresAt *string `json:"invite_code_expires_at"`
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
