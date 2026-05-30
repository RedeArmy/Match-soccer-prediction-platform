package handler

import (
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// Tests for admin response converter functions that handle optional pointer
// fields.  The branches guarded by nil checks (BannedAt, ConfirmedAt, ActorRole)
// are the specific paths that were previously uncovered.

// ── adminUserToResponse ───────────────────────────────────────────────────────

func TestAdminUserToResponse_NilBannedAt(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	u := &domain.User{
		ID:        1,
		Name:      "Ana",
		Email:     "ana@example.com",
		Role:      domain.RoleUser,
		Locale:    "es",
		CreatedAt: now,
		UpdatedAt: now,
	}
	r := adminUserToResponse(u)
	if r.BannedAt != nil || r.BannedBy != nil {
		t.Errorf("expected nil BannedAt and BannedBy for non-banned user; got %+v", r)
	}
	if r.Locale != "es" {
		t.Errorf("Locale: got %q; want \"es\"", r.Locale)
	}
}

func TestAdminUserToResponse_NonNilBannedAt_SetsBothFields(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	adminID := 99
	u := &domain.User{
		ID:        2,
		Name:      "Bob",
		Email:     "bob@example.com",
		Role:      domain.RoleUser,
		Locale:    "en",
		BannedAt:  &now,
		BannedBy:  &adminID,
		BanReason: "policy violation",
		CreatedAt: now,
		UpdatedAt: now,
	}
	r := adminUserToResponse(u)
	if r.BannedAt == nil {
		t.Fatal("BannedAt: got nil; want non-nil")
	}
	if r.BannedBy == nil || *r.BannedBy != adminID {
		t.Errorf("BannedBy: got %v; want %d", r.BannedBy, adminID)
	}
	if r.BanReason != "policy violation" {
		t.Errorf("BanReason: got %q; want \"policy violation\"", r.BanReason)
	}
}

// ── paymentToResponse ─────────────────────────────────────────────────────────

func TestPaymentToResponse_NilConfirmedAt(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	p := &domain.PaymentRecord{
		ID:         1,
		QuinielaID: 5,
		UserID:     10,
		Amount:     5000,
		Currency:   "GTQ",
		Status:     domain.PaymentStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	r := paymentToResponse(p)
	if r.ConfirmedAt != nil {
		t.Errorf("ConfirmedAt: got %v; want nil for pending payment", r.ConfirmedAt)
	}
	if r.Amount != 5000 {
		t.Errorf("Amount: got %d; want 5000", r.Amount)
	}
}

func TestPaymentToResponse_NonNilConfirmedAt_IncludesTimestamp(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	p := &domain.PaymentRecord{
		ID:          2,
		QuinielaID:  6,
		UserID:      11,
		Amount:      10000,
		Currency:    "GTQ",
		Status:      domain.PaymentStatusConfirmed,
		ConfirmedAt: &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	r := paymentToResponse(p)
	if r.ConfirmedAt == nil {
		t.Fatal("ConfirmedAt: got nil; want non-nil for confirmed payment")
	}
	want := now.Format(timeFormat)
	if *r.ConfirmedAt != want {
		t.Errorf("ConfirmedAt: got %q; want %q", *r.ConfirmedAt, want)
	}
	if r.Status != string(domain.PaymentStatusConfirmed) {
		t.Errorf("Status: got %q; want %q", r.Status, domain.PaymentStatusConfirmed)
	}
}

// ── auditLogToResponse ────────────────────────────────────────────────────────

func TestAuditLogToResponse_NilActorRole(t *testing.T) {
	now := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	actorID := 7
	a := &domain.AuditLog{
		ID:        1,
		ActorID:   &actorID,
		Action:    "match.created",
		CreatedAt: now,
	}
	r := auditLogToResponse(a)
	if r.ActorRole != nil {
		t.Errorf("ActorRole: got %v; want nil", r.ActorRole)
	}
	if r.Action != "match.created" {
		t.Errorf("Action: got %q; want \"match.created\"", r.Action)
	}
}

func TestAuditLogToResponse_NonNilActorRole_IncludesRole(t *testing.T) {
	now := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	actorID := 99
	role := domain.RoleAdmin
	resType := "quiniela"
	resID := 3
	a := &domain.AuditLog{
		ID:           2,
		ActorID:      &actorID,
		ActorRole:    &role,
		Action:       "admin_group.deleted",
		ResourceType: &resType,
		ResourceID:   &resID,
		CreatedAt:    now,
	}
	r := auditLogToResponse(a)
	if r.ActorRole == nil {
		t.Fatal("ActorRole: got nil; want non-nil")
	}
	if *r.ActorRole != string(domain.RoleAdmin) {
		t.Errorf("ActorRole: got %q; want %q", *r.ActorRole, domain.RoleAdmin)
	}
	if r.ResourceType == nil || *r.ResourceType != "quiniela" {
		t.Errorf("ResourceType: got %v; want \"quiniela\"", r.ResourceType)
	}
	if r.ResourceID == nil || *r.ResourceID != 3 {
		t.Errorf("ResourceID: got %v; want 3", r.ResourceID)
	}
}
