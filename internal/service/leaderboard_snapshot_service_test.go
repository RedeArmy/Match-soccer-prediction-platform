package service

import (
	"context"
	"errors"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// stubSnapshotRepo implements repository.LeaderboardSnapshotRepository.
type stubSnapshotRepo struct {
	snapshot  *domain.LeaderboardSnapshot
	snapshots []*domain.LeaderboardSnapshot
	err       error
}

func (r *stubSnapshotRepo) Create(_ context.Context, snap *domain.LeaderboardSnapshot) error {
	if r.err != nil {
		return r.err
	}
	snap.ID = 1
	return nil
}
func (r *stubSnapshotRepo) ListByQuiniela(_ context.Context, _ int, _ int) ([]*domain.LeaderboardSnapshot, error) {
	return r.snapshots, r.err
}
func (r *stubSnapshotRepo) GetLatest(_ context.Context, _ int) (*domain.LeaderboardSnapshot, error) {
	return r.snapshot, r.err
}

// stubRankerSnap implements Ranker for snapshot tests without importing cached_ranking_service_test.go's stubRanker.
type stubRankerSnap struct {
	entries []*domain.LeaderboardEntry
	err     error
}

func (r *stubRankerSnap) GetLeaderboard(_ context.Context, _ int) (*LeaderboardResult, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &LeaderboardResult{Entries: r.entries}, nil
}
func (r *stubRankerSnap) GetPhaseLeaderboard(_ context.Context, _ int, _ domain.MatchPhase) (*LeaderboardResult, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &LeaderboardResult{Entries: r.entries}, nil
}

// ── Snapshot ──────────────────────────────────────────────────────────────────

func TestLeaderboardSnapshotService_Snapshot_HappyPath_PersistsSnapshot(t *testing.T) {
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1}, Rank: 1, TotalPoints: 10},
		{User: &domain.User{ID: 2}, Rank: 2, TotalPoints: 5},
	}
	svc := NewLeaderboardSnapshotService(&stubRankerSnap{entries: entries}, &stubSnapshotRepo{})

	snap, err := svc.Snapshot(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if len(snap.Entries) != 2 {
		t.Errorf("expected 2 entries in snapshot, got %d", len(snap.Entries))
	}
	if snap.QuinielaID != 1 {
		t.Errorf("expected quinielaID 1, got %d", snap.QuinielaID)
	}
}

func TestLeaderboardSnapshotService_Snapshot_EmptyLeaderboard_StoresEmptyEntries(t *testing.T) {
	svc := NewLeaderboardSnapshotService(&stubRankerSnap{entries: nil}, &stubSnapshotRepo{})

	snap, err := svc.Snapshot(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if len(snap.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(snap.Entries))
	}
}

func TestLeaderboardSnapshotService_Snapshot_RankerError_Propagates(t *testing.T) {
	svc := NewLeaderboardSnapshotService(
		&stubRankerSnap{err: errors.New("ranker error")},
		&stubSnapshotRepo{},
	)

	_, err := svc.Snapshot(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from ranker, got nil")
	}
}

func TestLeaderboardSnapshotService_Snapshot_RepoError_Propagates(t *testing.T) {
	entries := []*domain.LeaderboardEntry{{User: &domain.User{ID: 1}, Rank: 1, TotalPoints: 3}}
	svc := NewLeaderboardSnapshotService(
		&stubRankerSnap{entries: entries},
		&stubSnapshotRepo{err: errors.New("db error")},
	)

	_, err := svc.Snapshot(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── SnapshotForMatch ──────────────────────────────────────────────────────────

func TestLeaderboardSnapshotService_SnapshotForMatch_SetsTriggeredByMatchID(t *testing.T) {
	entries := []*domain.LeaderboardEntry{
		{User: &domain.User{ID: 1}, Rank: 1, TotalPoints: 7},
	}
	svc := NewLeaderboardSnapshotService(&stubRankerSnap{entries: entries}, &stubSnapshotRepo{})

	snap, err := svc.SnapshotForMatch(context.Background(), 42, 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.QuinielaID != 42 {
		t.Errorf("expected quinielaID 42, got %d", snap.QuinielaID)
	}
	if snap.TriggeredByMatchID == nil || *snap.TriggeredByMatchID != 99 {
		t.Errorf("expected TriggeredByMatchID=99, got %v", snap.TriggeredByMatchID)
	}
}

func TestLeaderboardSnapshotService_SnapshotForMatch_RankerError_Propagates(t *testing.T) {
	svc := NewLeaderboardSnapshotService(
		&stubRankerSnap{err: errors.New("ranker error")},
		&stubSnapshotRepo{},
	)

	_, err := svc.SnapshotForMatch(context.Background(), 1, 5)
	if err == nil {
		t.Fatal("expected error from ranker, got nil")
	}
}

func TestLeaderboardSnapshotService_SnapshotForMatch_RepoError_Propagates(t *testing.T) {
	entries := []*domain.LeaderboardEntry{{User: &domain.User{ID: 1}, Rank: 1, TotalPoints: 3}}
	svc := NewLeaderboardSnapshotService(
		&stubRankerSnap{entries: entries},
		&stubSnapshotRepo{err: errors.New("db error")},
	)

	_, err := svc.SnapshotForMatch(context.Background(), 1, 5)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

func TestLeaderboardSnapshotService_Snapshot_TriggeredByMatchID_IsNil(t *testing.T) {
	svc := NewLeaderboardSnapshotService(&stubRankerSnap{}, &stubSnapshotRepo{})

	snap, err := svc.Snapshot(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.TriggeredByMatchID != nil {
		t.Errorf("expected TriggeredByMatchID nil for manual Snapshot, got %v", snap.TriggeredByMatchID)
	}
}
