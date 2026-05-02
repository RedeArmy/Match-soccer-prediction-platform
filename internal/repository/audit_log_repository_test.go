package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── AuditLogRepository ────────────────────────────────────────────────────────

func TestAuditLogRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	actorID := u.ID
	resType := "user"
	resID := u.ID
	entry := &domain.AuditLog{
		ActorID:      &actorID,
		Action:       "ban_user",
		ResourceType: &resType,
		ResourceID:   &resID,
		Metadata:     map[string]any{"reason": "spam"},
	}
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if entry.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestAuditLogRepository_Create_NilMetadataAndActor(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	entry := &domain.AuditLog{Action: "system_boot"}
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if entry.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestAuditLogRepository_ListByActor(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	actorID := u.ID
	for range 3 {
		e := &domain.AuditLog{ActorID: &actorID, Action: "some_action"}
		_ = repo.Create(context.Background(), e)
	}
	other := seedUser(t)
	otherID := other.ID
	_ = repo.Create(context.Background(), &domain.AuditLog{ActorID: &otherID, Action: "other_action"})

	results, err := repo.ListByActor(context.Background(), u.ID, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 entries for actor %d, got %d", u.ID, len(results))
	}
}

func TestAuditLogRepository_ListByEntity(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	resType := repoResourceQuiniela
	resID := 42
	for range 2 {
		e := &domain.AuditLog{Action: "update", ResourceType: &resType, ResourceID: &resID}
		_ = repo.Create(context.Background(), e)
	}
	otherType := "user"
	_ = repo.Create(context.Background(), &domain.AuditLog{Action: "ban", ResourceType: &otherType, ResourceID: &resID})

	results, err := repo.ListByEntity(context.Background(), repoResourceQuiniela, 42, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 quiniela entries, got %d", len(results))
	}
}

func TestAuditLogRepository_ListByAction(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	for range 4 {
		_ = repo.Create(context.Background(), &domain.AuditLog{Action: "payment_validate"})
	}
	_ = repo.Create(context.Background(), &domain.AuditLog{Action: "payment_reject"})

	results, err := repo.ListByAction(context.Background(), "payment_validate", repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 4 {
		t.Errorf("expected 4 validate entries, got %d", len(results))
	}
}

func TestAuditLogRepository_List_Pagination(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	for range 5 {
		_ = repo.Create(context.Background(), &domain.AuditLog{Action: "ping"})
	}

	page, err := repo.List(context.Background(), repository.AuditLogFilters{}, repository.Pagination{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(page) != 2 {
		t.Errorf("expected 2 entries with limit=2, got %d", len(page))
	}
}

func TestAuditLogRepository_Create_WithActorRole(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	actorID := u.ID
	role := domain.RoleAdmin
	entry := &domain.AuditLog{
		ActorID:   &actorID,
		ActorRole: &role,
		Action:    "ban_user",
	}
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if entry.ActorRole == nil || *entry.ActorRole != domain.RoleAdmin {
		t.Errorf("expected ActorRole %q, got %v", domain.RoleAdmin, entry.ActorRole)
	}
}

func TestAuditLogRepository_List_WithRoleAndMetadata(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresAuditLogRepository(testDB)

	actorID := u.ID
	role := domain.RoleAdmin
	resType := repoResourceQuiniela
	resID := 1
	entry := &domain.AuditLog{
		ActorID:      &actorID,
		ActorRole:    &role,
		Action:       "delete_group",
		ResourceType: &resType,
		ResourceID:   &resID,
		Metadata:     map[string]any{"reason": "inactivity"},
	}
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	results, err := repo.List(context.Background(), repository.AuditLogFilters{}, repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(results))
	}
	got := results[0]
	if got.ActorRole == nil || *got.ActorRole != domain.RoleAdmin {
		t.Errorf("expected ActorRole %q, got %v", domain.RoleAdmin, got.ActorRole)
	}
	if got.Metadata["reason"] != "inactivity" {
		t.Errorf("expected metadata reason 'inactivity', got %v", got.Metadata["reason"])
	}
}
