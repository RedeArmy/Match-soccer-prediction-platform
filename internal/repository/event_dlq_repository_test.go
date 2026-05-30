package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

// TestPostgresEventDLQRepository_RecordDeadLettered verifies that
// RecordDeadLettered inserts one event_dlq row and that the constructor
// returns a non-nil repository. (ATD-007)
func TestPostgresEventDLQRepository_RecordDeadLettered_PersistsRow(t *testing.T) {
	ctx := context.Background()
	if _, err := testDB.Exec(ctx, `TRUNCATE event_dlq RESTART IDENTITY`); err != nil {
		t.Fatalf("truncate event_dlq: %v", err)
	}

	repo := repository.NewPostgresEventDLQRepository(testDB)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}

	err := repo.RecordDeadLettered(
		ctx,
		"match.finished",
		`{"event_type":"match.finished","envelope":{},"error":"handler failed","attempts":3}`,
		"scorer unavailable",
		3,
	)
	if err != nil {
		t.Fatalf("RecordDeadLettered: %v", err)
	}

	var count int
	if err := testDB.QueryRow(ctx,
		`SELECT COUNT(*) FROM event_dlq WHERE event_type = 'match.finished' AND resolved_at IS NULL`,
	).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 unresolved row, got %d", count)
	}
}

// TestPostgresEventDLQRepository_RecordDeadLettered_InvalidJSON verifies that
// passing a non-JSON payload returns an error (Postgres rejects invalid JSONB).
func TestPostgresEventDLQRepository_RecordDeadLettered_InvalidJSON_ReturnsError(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewPostgresEventDLQRepository(testDB)

	err := repo.RecordDeadLettered(ctx, "match.finished", `not-valid-json`, "err", 1)
	if err == nil {
		t.Error("expected error for invalid JSONB payload, got nil")
	}
}
