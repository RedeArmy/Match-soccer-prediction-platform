package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── LeaderboardSnapshotRepository ────────────────────────────────────────────

func TestLeaderboardSnapshotRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	snapshot := &domain.LeaderboardSnapshot{
		QuinielaID: q.ID,
		TakenAt:    time.Now().UTC().Truncate(time.Microsecond),
		Entries: []domain.LeaderboardSnapshotEntry{
			{UserID: u.ID, Rank: 1, TotalPoints: 15, PrizeWinner: true},
		},
	}
	if err := repo.Create(context.Background(), snapshot); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if snapshot.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if len(snapshot.Entries) != 1 || snapshot.Entries[0].UserID != u.ID {
		t.Errorf("entries not round-tripped correctly: %+v", snapshot.Entries)
	}
}

func TestLeaderboardSnapshotRepository_Create_EmptyEntries(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	snapshot := &domain.LeaderboardSnapshot{
		QuinielaID: q.ID,
		TakenAt:    time.Now().UTC(),
		Entries:    []domain.LeaderboardSnapshotEntry{},
	}
	if err := repo.Create(context.Background(), snapshot); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if snapshot.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestLeaderboardSnapshotRepository_GetLatest(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	base := time.Now().UTC().Truncate(time.Microsecond)
	older := &domain.LeaderboardSnapshot{QuinielaID: q.ID, TakenAt: base.Add(-time.Hour), Entries: nil}
	newer := &domain.LeaderboardSnapshot{QuinielaID: q.ID, TakenAt: base, Entries: nil}
	_ = repo.Create(context.Background(), older)
	_ = repo.Create(context.Background(), newer)

	latest, err := repo.GetLatest(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if latest == nil || latest.ID != newer.ID {
		t.Errorf("expected latest ID %d, got %v", newer.ID, latest)
	}
}

func TestLeaderboardSnapshotRepository_GetLatest_NoneExists(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	snap, err := repo.GetLatest(context.Background(), 999999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if snap != nil {
		t.Errorf(fmtExpectNilGot, snap)
	}
}

func TestLeaderboardSnapshotRepository_ListByQuiniela_NoLimit_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	base := time.Now().UTC().Truncate(time.Microsecond)
	for i := range 3 {
		s := &domain.LeaderboardSnapshot{QuinielaID: q.ID, TakenAt: base.Add(time.Duration(i) * time.Minute)}
		_ = repo.Create(context.Background(), s)
	}

	results, err := repo.ListByQuiniela(context.Background(), q.ID, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 3 {
		t.Errorf("expected all 3 snapshots with limit=0, got %d", len(results))
	}
}

func TestLeaderboardSnapshotRepository_ListByQuiniela_WithLimit(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresLeaderboardSnapshotRepository(testDB)

	base := time.Now().UTC().Truncate(time.Microsecond)
	for i := range 5 {
		s := &domain.LeaderboardSnapshot{QuinielaID: q.ID, TakenAt: base.Add(time.Duration(i) * time.Minute)}
		_ = repo.Create(context.Background(), s)
	}

	results, err := repo.ListByQuiniela(context.Background(), q.ID, 3)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 snapshots with limit=3, got %d", len(results))
	}
}
