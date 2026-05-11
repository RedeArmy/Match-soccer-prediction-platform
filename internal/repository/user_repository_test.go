package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── UserRepository ────────────────────────────────────────────────────────────

func TestUserRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	u := &domain.User{Name: "Bob", Email: "bob@example.com", Role: domain.RoleUser}

	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if u.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if u.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt after create")
	}
}

func TestUserRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	created := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.Email != created.Email {
		t.Errorf("email: got %q, want %q", got.Email, created.Email)
	}
}

func TestUserRepository_GetByID_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for missing user, got %+v", got)
	}
}

func TestUserRepository_GetByClerkSubject_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	// Set a clerk_subject via Update so GetByClerkSubject can find it.
	u.ClerkSubject = "user_clerk_abc123"
	if err := repo.Update(context.Background(), u); err != nil {
		t.Fatalf("set clerk_subject: %v", err)
	}

	got, err := repo.GetByClerkSubject(context.Background(), "user_clerk_abc123")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.ID != u.ID {
		t.Errorf(fmtIDMismatch, got.ID, u.ID)
	}
}

func TestUserRepository_GetByClerkSubject_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.GetByClerkSubject(context.Background(), "user_nonexistent")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown clerk subject, got %+v", got)
	}
}

func TestUserRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	u.Name = "Alice Updated"
	if err := repo.Update(context.Background(), u); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if u.Name != "Alice Updated" {
		t.Errorf("name not updated: got %q", u.Name)
	}
}

func TestUserRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	ghost := &domain.User{ID: 99999, Name: "Ghost", Email: "g@g.com", Role: domain.RoleUser}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUserRepository_Delete_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	if err := repo.Delete(context.Background(), u.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), u.ID)
	if got != nil {
		t.Error("expected user to be deleted")
	}
}

func TestUserRepository_Delete_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	if err := repo.Delete(context.Background(), 99999); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUserRepository_List_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	users, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}
	if users[0].ID != u.ID {
		t.Errorf("unexpected user ID: got %d, want %d", users[0].ID, u.ID)
	}
}

// ── UserRepository - ListByIDs ─────────────────────────────────────────────────

func TestUserRepository_ListByIDs_ReturnsMatchingUsers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	_ = seedUser(t) // u3 not requested
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.ListByIDs(context.Background(), []int{u1.ID, u2.ID})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 users, got %d", len(got))
	}
}

func TestUserRepository_ListByIDs_EmptySlice_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	got, err := repo.ListByIDs(context.Background(), []int{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf("expected nil for empty ids, got %v", got)
	}
}

func TestUserRepository_ListByIDs_IgnoresDeletedUsers(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	userRepo := repository.NewPostgresUserRepository(testDB)

	// Soft-delete u2 - it must not appear in ListByIDs results.
	if err := userRepo.Delete(context.Background(), u2.ID); err != nil {
		t.Fatalf("delete u2: %v", err)
	}

	got, err := userRepo.ListByIDs(context.Background(), []int{u1.ID, u2.ID})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 active user, got %d", len(got))
	}
	if got[0].ID != u1.ID {
		t.Errorf("expected user %d, got %d", u1.ID, got[0].ID)
	}
}

// ── UserRepository extensions (Ban/Unban/ListBanned) ─────────────────────────

func TestUserRepository_Ban_SetsFields(t *testing.T) {
	cleanTables(t)
	user := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	banned, err := repo.Ban(context.Background(), user.ID, admin.ID, repoPolicyViolation)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if banned.BannedAt == nil {
		t.Error("expected BannedAt to be set")
	}
	if banned.BannedBy == nil || *banned.BannedBy != admin.ID {
		t.Errorf("expected BannedBy %d, got %v", admin.ID, banned.BannedBy)
	}
	if banned.BanReason != repoPolicyViolation {
		t.Errorf("expected BanReason %q, got %q", repoPolicyViolation, banned.BanReason)
	}
}

func TestUserRepository_Ban_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	_, err := repo.Ban(context.Background(), 999999, 1, "reason")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUserRepository_Unban_ClearsFields(t *testing.T) {
	cleanTables(t)
	user := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	_, _ = repo.Ban(context.Background(), user.ID, admin.ID, "test")
	if err := repo.Unban(context.Background(), user.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	got, _ := repo.GetByID(context.Background(), user.ID)
	if got.BannedAt != nil {
		t.Errorf("expected BannedAt nil after unban, got %v", got.BannedAt)
	}
}

func TestUserRepository_Unban_IsIdempotent(t *testing.T) {
	cleanTables(t)
	user := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	// user is not banned - should succeed silently
	if err := repo.Unban(context.Background(), user.ID); err != nil {
		t.Fatalf("unban on unbanned user should not error: %v", err)
	}
}

func TestUserRepository_Unban_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)

	err := repo.Unban(context.Background(), 999999)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUserRepository_ListBanned(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	_ = seedUser(t) // active, not banned
	admin := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	_, _ = repo.Ban(context.Background(), u1.ID, admin.ID, "r1")
	_, _ = repo.Ban(context.Background(), u2.ID, admin.ID, "r2")

	banned, err := repo.ListBanned(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(banned) != 2 {
		t.Errorf("expected 2 banned users, got %d", len(banned))
	}
}

// ── UserRepository admin extensions ──────────────────────────────────────────

func TestUserRepository_ListFiltered_NoFilter_ReturnsAll(t *testing.T) {
	cleanTables(t)
	seedUser(t)
	seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	results, _, err := repo.ListFiltered(context.Background(), repository.UserFilters{}, repository.CursorPage{Limit: 1000})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 users, got %d", len(results))
	}
}

func TestUserRepository_ListFiltered_FilterByBanned(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	_ = seedUser(t) // active
	admin := seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)
	_, _ = repo.Ban(context.Background(), u1.ID, admin.ID, "test")

	banned := true
	results, _, err := repo.ListFiltered(context.Background(), repository.UserFilters{Banned: &banned}, repository.CursorPage{Limit: 1000})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].ID != u1.ID {
		t.Errorf("expected 1 banned user, got %d", len(results))
	}
}

func TestUserRepository_ListFiltered_FilterByRole(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	admin := &domain.User{Name: "Admin", Email: "admin@example.com", Role: domain.RoleAdmin}
	if err := repo.Create(context.Background(), admin); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	seedUser(t) // role = player

	role := domain.RoleAdmin
	results, _, err := repo.ListFiltered(context.Background(), repository.UserFilters{Role: &role}, repository.CursorPage{Limit: 1000})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].Role != domain.RoleAdmin {
		t.Errorf("expected 1 admin user, got %d", len(results))
	}
}

func TestUserRepository_ListFiltered_FilterBySearch(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	if err := repo.Create(context.Background(), &domain.User{Name: "alice", Email: "alice@example.com", Role: domain.RoleUser}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.User{Name: "bob", Email: "bob@example.com", Role: domain.RoleUser}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	search := "alic"
	results, _, err := repo.ListFiltered(context.Background(), repository.UserFilters{Search: &search}, repository.CursorPage{Limit: 1000})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].Name != "alice" {
		t.Errorf("expected 1 user matching 'alic', got %d", len(results))
	}
}

func TestUserRepository_ListFiltered_CursorPagination(t *testing.T) {
	cleanTables(t)
	seedUser(t)
	seedUser(t)
	seedUser(t)
	repo := repository.NewPostgresUserRepository(testDB)

	// First page: limit=2, no cursor.
	page1, cursor1, err := repo.ListFiltered(context.Background(), repository.UserFilters{}, repository.CursorPage{Limit: 2})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 users on page 1, got %d", len(page1))
	}
	if cursor1 == "" {
		t.Fatal("expected non-empty next_cursor after page 1")
	}

	// Second page: use cursor from page 1.
	page2, cursor2, err := repo.ListFiltered(context.Background(), repository.UserFilters{}, repository.CursorPage{Limit: 2, Cursor: cursor1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(page2) != 1 {
		t.Errorf("expected 1 user on page 2, got %d", len(page2))
	}
	if cursor2 != "" {
		t.Errorf("expected empty next_cursor on last page, got %q", cursor2)
	}

	// Pages must not overlap.
	if page1[0].ID == page2[0].ID || page1[1].ID == page2[0].ID {
		t.Error("pages share a user: cursor pagination overlapped")
	}
}

// ── UserRepository.GetStatusCounts ───────────────────────────────────────────

func TestUserRepository_GetStatusCounts_ReturnsCorrectTotals(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresUserRepository(testDB)
	admin := seedUser(t)

	u1 := seedUser(t)
	u2 := seedUser(t)
	if _, err := repo.Ban(context.Background(), u1.ID, admin.ID, "spam"); err != nil {
		t.Fatalf("ban: %v", err)
	}

	counts, err := repo.GetStatusCounts(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if counts.Total < 3 {
		t.Errorf("expected Total ≥ 3, got %d", counts.Total)
	}
	if counts.Banned < 1 {
		t.Errorf("expected Banned ≥ 1, got %d", counts.Banned)
	}
	if counts.Active < 2 {
		t.Errorf("expected Active ≥ 2, got %d", counts.Active)
	}
	_ = u2
}
