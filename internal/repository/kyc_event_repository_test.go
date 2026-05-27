package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func seedKYCEvent(t *testing.T, profileID int) *domain.KYCEvent {
	t.Helper()
	repo := repository.NewPostgresKYCEventRepository(testDB)
	e := &domain.KYCEvent{
		ProfileID:   profileID,
		ProfileType: domain.KYCProfileTypeUser,
		EventType:   domain.KYCEventSubmitted,
		NewStatus:   domain.KYCStatusPending,
		Reason:      "initial submission",
	}
	if err := repo.Create(context.Background(), e); err != nil {
		t.Fatalf("seedKYCEvent: %v", err)
	}
	return e
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestKYCEventRepository_Create_PopulatesIDAndCreatedAt(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	e := seedKYCEvent(t, p.ID)

	if e.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if e.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestKYCEventRepository_Create_WithOldStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCEventRepository(testDB)

	old := domain.KYCStatusPending
	e := &domain.KYCEvent{
		ProfileID:   p.ID,
		ProfileType: domain.KYCProfileTypeUser,
		EventType:   domain.KYCEventApproved,
		OldStatus:   &old,
		NewStatus:   domain.KYCStatusApproved,
		ActorID:     &admin.ID,
		Reason:      "docs verified",
	}
	if err := repo.Create(context.Background(), e); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if e.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestKYCEventRepository_Create_WithMetadata(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCEventRepository(testDB)

	e := &domain.KYCEvent{
		ProfileID:   p.ID,
		ProfileType: domain.KYCProfileTypeUser,
		EventType:   domain.KYCEventSubmitted,
		NewStatus:   domain.KYCStatusPending,
		Metadata:    map[string]any{"source": "web", "tier": 1},
	}
	if err := repo.Create(context.Background(), e); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if e.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

// ── ListByProfile ─────────────────────────────────────────────────────────────

func TestKYCEventRepository_ListByProfile_ReturnsEvents(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	seedKYCEvent(t, p.ID)
	seedKYCEvent(t, p.ID)
	repo := repository.NewPostgresKYCEventRepository(testDB)

	events, next, err := repo.ListByProfile(context.Background(), p.ID, domain.KYCProfileTypeUser,
		repository.CursorPage{Limit: 10})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
	if next != "" {
		t.Errorf("expected empty next cursor, got %q", next)
	}
}

func TestKYCEventRepository_ListByProfile_Empty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCEventRepository(testDB)

	events, _, err := repo.ListByProfile(context.Background(), 99999, domain.KYCProfileTypeUser,
		repository.CursorPage{Limit: 10})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty slice, got %d", len(events))
	}
}

func TestKYCEventRepository_ListByProfile_Pagination(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	for range 3 {
		seedKYCEvent(t, p.ID)
	}
	repo := repository.NewPostgresKYCEventRepository(testDB)

	// First page — limit=2, expect a next cursor.
	page1, cursor, err := repo.ListByProfile(context.Background(), p.ID, domain.KYCProfileTypeUser,
		repository.CursorPage{Limit: 2})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1: expected 2 events, got %d", len(page1))
	}
	if cursor == "" {
		t.Fatal("expected non-empty next cursor after page 1")
	}

	// Second page — should return 1 remaining event.
	page2, nextCursor, err := repo.ListByProfile(context.Background(), p.ID, domain.KYCProfileTypeUser,
		repository.CursorPage{Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("page2: expected 1 event, got %d", len(page2))
	}
	if nextCursor != "" {
		t.Errorf("expected empty cursor on last page, got %q", nextCursor)
	}
}

func TestKYCEventRepository_ListByProfile_InvalidLimit_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCEventRepository(testDB)

	_, _, err := repo.ListByProfile(context.Background(), 1, domain.KYCProfileTypeUser,
		repository.CursorPage{Limit: 0})
	if err == nil {
		t.Fatal("expected error for zero limit, got nil")
	}
}
