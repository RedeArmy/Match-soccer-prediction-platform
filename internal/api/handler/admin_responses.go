package handler

import (
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// ── Admin response types ──────────────────────────────────────────────────────

// AdminUserResponse is the admin view of a user account.
type AdminUserResponse struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	Role      string  `json:"role"`
	Locale    string  `json:"locale"`
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
	Key          string `json:"key"`
	Value        string `json:"value"`
	DefaultValue string `json:"default_value"`
	Type         string `json:"type"`
	Category     string `json:"category"`
	IsRuntime    bool   `json:"is_runtime"`
	Description  string `json:"description"`
	UpdatedAt    string `json:"updated_at"`
}

// SystemParamHistoryResponse is one row from system_params_history.
type SystemParamHistoryResponse struct {
	ID        int64  `json:"id"`
	Key       string `json:"key"`
	OldValue  string `json:"old_value"`
	NewValue  string `json:"new_value"`
	ActorID   int    `json:"actor_id"`
	Action    string `json:"action"`
	ChangedAt string `json:"changed_at"`
}

// systemParamHistoryToResponse maps a domain history entry to its wire type.
func systemParamHistoryToResponse(h *domain.SystemParamHistory) SystemParamHistoryResponse {
	return SystemParamHistoryResponse{
		ID:        h.ID,
		Key:       h.Key,
		OldValue:  h.OldValue,
		NewValue:  h.NewValue,
		ActorID:   h.ActorID,
		Action:    h.Action,
		ChangedAt: h.ChangedAt.UTC().Format(time.RFC3339),
	}
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
	AgeDays    *int           `json:"age_days,omitempty"`
}

// ConflictTypeSummaryResponse is one row in ConflictSummaryResponse.
type ConflictTypeSummaryResponse struct {
	Type       string   `json:"type"`
	Count      int      `json:"count"`
	AvgAgeDays *float64 `json:"avg_age_days,omitempty"`
}

// ConflictSummaryResponse is the response body for GET /admin/stats/conflicts/summary.
type ConflictSummaryResponse struct {
	TotalUnresolved int                           `json:"total_unresolved"`
	ByType          []ConflictTypeSummaryResponse `json:"by_type"`
	// LimitReached is true when the conflict backlog equals or exceeds max_scan.
	// When true, the summary is incomplete - some conflicts were not scanned.
	// Dashboard widgets should display an urgent alert when this flag is set.
	LimitReached bool `json:"limit_reached"`
	// MaxScan is the configured scan limit (conflict.max_scan) applied to this request.
	// Provides context for interpreting total_unresolved and limit_reached.
	MaxScan int `json:"max_scan"`
}

// DashboardStatsResponse is the response body for GET /admin/stats.
type DashboardStatsResponse struct {
	Groups   GroupDashboardStatsResponse   `json:"groups"`
	Users    UserDashboardStatsResponse    `json:"users"`
	Payments PaymentDashboardStatsResponse `json:"payments"`
}

// GroupDashboardStatsResponse is the group-counts section of DashboardStatsResponse.
type GroupDashboardStatsResponse struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Inactive int `json:"inactive"`
	Deleted  int `json:"deleted"`
}

// UserDashboardStatsResponse is the user-counts section of DashboardStatsResponse.
type UserDashboardStatsResponse struct {
	Total  int `json:"total"`
	Active int `json:"active"`
	Banned int `json:"banned"`
}

// PaymentDashboardStatsResponse is the payment-counts section of DashboardStatsResponse.
// TotalCollected is the sum of confirmed payment amounts in minor currency units (e.g. centavos).
type PaymentDashboardStatsResponse struct {
	Pending        int `json:"pending"`
	Confirmed      int `json:"confirmed"`
	Rejected       int `json:"rejected"`
	TotalCollected int `json:"total_collected"`
}

// BulkOperationResultResponse is the response body for bulk admin operations.
// HTTP 200 when all succeeded; HTTP 207 when some failed (partial success).
type BulkOperationResultResponse struct {
	Succeeded []int `json:"succeeded"`
	Failed    []int `json:"failed"`
}

// BulkBanErrorResponse is the per-user failure detail within BulkBanResultResponse.
type BulkBanErrorResponse struct {
	UserID  int    `json:"user_id"`
	Message string `json:"message"`
}

// BulkBanResultResponse is the response body for POST /admin/users/bulk-ban.
// HTTP 200 when all bans succeeded; HTTP 207 when some bans failed (partial success).
type BulkBanResultResponse struct {
	Banned []int                  `json:"banned"`
	Failed []BulkBanErrorResponse `json:"failed"`
}

// ── Admin converter functions ─────────────────────────────────────────────────

func adminUserToResponse(u *domain.User) AdminUserResponse {
	resp := AdminUserResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		Role:      string(u.Role),
		Locale:    u.Locale,
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
		Key:          p.Key,
		Value:        p.Value,
		DefaultValue: p.DefaultValue,
		Type:         string(p.Type),
		Category:     p.Category,
		IsRuntime:    p.IsRuntime,
		Description:  p.Description,
		UpdatedAt:    p.UpdatedAt.Format(timeFormat),
	}
}

func dashboardStatsToResponse(s *domain.DashboardStats) DashboardStatsResponse {
	return DashboardStatsResponse{
		Groups: GroupDashboardStatsResponse{
			Total:    s.Groups.Total,
			Active:   s.Groups.Active,
			Inactive: s.Groups.Inactive,
			Deleted:  s.Groups.Deleted,
		},
		Users: UserDashboardStatsResponse{
			Total:  s.Users.Total,
			Active: s.Users.Active,
			Banned: s.Users.Banned,
		},
		Payments: PaymentDashboardStatsResponse{
			Pending:        s.Payments.Pending,
			Confirmed:      s.Payments.Confirmed,
			Rejected:       s.Payments.Rejected,
			TotalCollected: s.Payments.TotalCollected,
		},
	}
}

func bulkOperationResultToResponse(r service.BulkOperationResult) BulkOperationResultResponse {
	succeeded := r.Succeeded
	if succeeded == nil {
		succeeded = []int{}
	}
	failed := r.Failed
	if failed == nil {
		failed = []int{}
	}
	return BulkOperationResultResponse{Succeeded: succeeded, Failed: failed}
}

func conflictToResponse(c domain.Conflict) ConflictResponse {
	return ConflictResponse{
		Type:       string(c.Type),
		EntityID:   c.EntityID,
		EntityType: c.EntityType,
		Details:    c.Details,
		DetectedAt: c.DetectedAt.Format(timeFormat),
		AgeDays:    c.AgeDays,
	}
}
