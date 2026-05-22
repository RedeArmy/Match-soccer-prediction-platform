package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

const (
	histTestKey     = "test.history_key"
	histOldValue    = "old"
	histNewValue    = "new"
	histActionSet   = "set"
	histActionReset = "reset"
)

// seedHistoryParam ensures the parent system_params row exists so that
// system_params_history FK constraints are satisfied.
func seedHistoryParam(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(context.Background(),
		`INSERT INTO system_params (key, value, default_value, category)
		 VALUES ($1, $2, $2, 'system')
		 ON CONFLICT (key) DO NOTHING`,
		histTestKey, histOldValue,
	)
	if err != nil {
		t.Fatalf("seed history param: %v", err)
	}
}

// ── Record ────────────────────────────────────────────────────────────────────

func TestSystemParamHistoryRepository_Record_InsertsRow(t *testing.T) {
	cleanTables(t)
	seedHistoryParam(t)
	u := seedUser(t)

	repo := repository.NewPostgresSystemParamHistoryRepository(testDB)
	entry := &domain.SystemParamHistory{
		Key:      histTestKey,
		OldValue: histOldValue,
		NewValue: histNewValue,
		ActorID:  u.ID,
		Action:   histActionSet,
	}

	if err := repo.Record(context.Background(), entry); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if entry.ID == 0 {
		t.Error("expected ID to be back-filled by database, got 0")
	}
	if entry.ChangedAt.IsZero() {
		t.Error("expected ChangedAt to be back-filled by database, got zero")
	}
}

func TestSystemParamHistoryRepository_Record_RejectsInvalidAction(t *testing.T) {
	cleanTables(t)
	seedHistoryParam(t)
	u := seedUser(t)

	repo := repository.NewPostgresSystemParamHistoryRepository(testDB)
	entry := &domain.SystemParamHistory{
		Key:      histTestKey,
		OldValue: histOldValue,
		NewValue: histNewValue,
		ActorID:  u.ID,
		Action:   "invalid_action",
	}

	if err := repo.Record(context.Background(), entry); err == nil {
		t.Error("expected error for invalid action, got nil")
	}
}

// ── ListByKey ─────────────────────────────────────────────────────────────────

func TestSystemParamHistoryRepository_ListByKey_ReturnsNewestFirst(t *testing.T) {
	cleanTables(t)
	seedHistoryParam(t)
	u := seedUser(t)
	ctx := context.Background()

	repo := repository.NewPostgresSystemParamHistoryRepository(testDB)
	for _, action := range []string{histActionSet, histActionReset} {
		_ = repo.Record(ctx, &domain.SystemParamHistory{
			Key: histTestKey, OldValue: histOldValue, NewValue: histNewValue,
			ActorID: u.ID, Action: action,
		})
	}

	entries, next, err := repo.ListByKey(ctx, histTestKey, repository.CursorPage{Limit: 10})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if next != "" {
		t.Errorf("expected empty cursor on last page, got %q", next)
	}
	// Newest-first: second inserted (reset) should be entries[0].
	if entries[0].Action != histActionReset {
		t.Errorf("expected newest entry first (reset), got %q", entries[0].Action)
	}
}

func TestSystemParamHistoryRepository_ListByKey_UnknownKey_ReturnsEmpty(t *testing.T) {
	cleanTables(t)

	repo := repository.NewPostgresSystemParamHistoryRepository(testDB)
	entries, next, err := repo.ListByKey(context.Background(), "does.not.exist", repository.CursorPage{Limit: 10})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for unknown key, got %d", len(entries))
	}
	if next != "" {
		t.Errorf("expected empty cursor, got %q", next)
	}
}

func TestSystemParamHistoryRepository_ListByKey_PaginationCursor(t *testing.T) {
	cleanTables(t)
	seedHistoryParam(t)
	u := seedUser(t)
	ctx := context.Background()

	repo := repository.NewPostgresSystemParamHistoryRepository(testDB)
	for i := 0; i < 3; i++ {
		_ = repo.Record(ctx, &domain.SystemParamHistory{
			Key: histTestKey, OldValue: histOldValue, NewValue: histNewValue,
			ActorID: u.ID, Action: histActionSet,
		})
	}

	// First page: limit 2 — should return cursor.
	page1, cursor, err := repo.ListByKey(ctx, histTestKey, repository.CursorPage{Limit: 2})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 on first page, got %d", len(page1))
	}
	if cursor == "" {
		t.Fatal("expected non-empty cursor after first page")
	}

	// Second page via cursor — should return the remaining 1 row.
	page2, cursor2, err := repo.ListByKey(ctx, histTestKey, repository.CursorPage{Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 on second page, got %d", len(page2))
	}
	if cursor2 != "" {
		t.Errorf("expected empty cursor on final page, got %q", cursor2)
	}
}

// ── PurgeOldParamHistory ──────────────────────────────────────────────────────

func TestPurger_PurgeOldParamHistory_DeletesOldRows(t *testing.T) {
	cleanTables(t)
	seedHistoryParam(t)
	u := seedUser(t)
	ctx := context.Background()

	_, err := testDB.Exec(ctx,
		`INSERT INTO system_params_history (key, old_value, new_value, actor_id, action, changed_at)
		 VALUES ($1, $2, $3, $4, $5, NOW() - INTERVAL '91 days')`,
		histTestKey, histOldValue, histNewValue, u.ID, histActionSet,
	)
	if err != nil {
		t.Fatalf("insert old history row: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	n, err := purger.PurgeOldParamHistory(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 1 {
		t.Errorf("expected 1 purged row, got %d", n)
	}
}

func TestPurger_PurgeOldParamHistory_PreservesRecentRows(t *testing.T) {
	cleanTables(t)
	seedHistoryParam(t)
	u := seedUser(t)
	ctx := context.Background()

	repo := repository.NewPostgresSystemParamHistoryRepository(testDB)
	_ = repo.Record(ctx, &domain.SystemParamHistory{
		Key: histTestKey, OldValue: histOldValue, NewValue: histNewValue,
		ActorID: u.ID, Action: histActionSet,
	})

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	n, err := purger.PurgeOldParamHistory(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows purged (row is recent), got %d", n)
	}
}
