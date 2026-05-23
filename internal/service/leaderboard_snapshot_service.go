package service

import (
	"context"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// Snapshotter persists point-in-time leaderboard copies for a
// quiniela. It is called by the scoring worker immediately after ScoreMatch
// completes so the latest rankings are available without re-computing them.
type Snapshotter interface {
	// Snapshot takes an admin/manual snapshot for quinielaID. No idempotency
	// key is used; each call creates a new row. Used by RecalculateLeaderboard.
	Snapshot(ctx context.Context, quinielaID int) (*domain.LeaderboardSnapshot, error)
	// SnapshotForMatch takes a worker-triggered snapshot associating the result
	// with matchID. Replaying the same (quinielaID, matchID) pair is idempotent:
	// the existing snapshot row is returned without creating a duplicate.
	SnapshotForMatch(ctx context.Context, quinielaID, matchID int) (*domain.LeaderboardSnapshot, error)
}

// leaderboardSnapshotService is the concrete implementation of Snapshotter.
type leaderboardSnapshotService struct {
	ranker   Ranker
	snapRepo repository.LeaderboardSnapshotRepository
}

// NewLeaderboardSnapshotService constructs a leaderboardSnapshotService.
func NewLeaderboardSnapshotService(
	ranker Ranker,
	snapRepo repository.LeaderboardSnapshotRepository,
) Snapshotter {
	return &leaderboardSnapshotService{ranker: ranker, snapRepo: snapRepo}
}

// Snapshot computes the current leaderboard via Ranker and stores it as a
// point-in-time snapshot. Used for admin-triggered (manual) recalculations;
// each call creates a new row. For worker-triggered snapshots (after scoring)
// use SnapshotForMatch, which is idempotent on replay.
func (s *leaderboardSnapshotService) Snapshot(ctx context.Context, quinielaID int) (*domain.LeaderboardSnapshot, error) {
	return s.snapshot(ctx, quinielaID, nil)
}

// SnapshotForMatch is the idempotent variant of Snapshot used by the scoring
// worker after a MatchFinished event. Replaying the same (quinielaID, matchID)
// pair returns the existing snapshot row without inserting a duplicate.
func (s *leaderboardSnapshotService) SnapshotForMatch(ctx context.Context, quinielaID, matchID int) (*domain.LeaderboardSnapshot, error) {
	return s.snapshot(ctx, quinielaID, &matchID)
}

func (s *leaderboardSnapshotService) snapshot(ctx context.Context, quinielaID int, triggerMatchID *int) (*domain.LeaderboardSnapshot, error) {
	result, err := s.ranker.GetLeaderboard(ctx, quinielaID)
	if err != nil {
		return nil, err
	}

	var entries []*domain.LeaderboardEntry
	if result != nil {
		entries = result.Entries
	}

	snapshotEntries := make([]domain.LeaderboardSnapshotEntry, 0, len(entries))
	for _, e := range entries {
		snapshotEntries = append(snapshotEntries, domain.LeaderboardSnapshotEntry{
			UserID:      e.User.ID,
			Rank:        e.Rank,
			TotalPoints: e.TotalPoints,
			PrizeWinner: e.PrizeWinner,
		})
	}

	snap := &domain.LeaderboardSnapshot{
		QuinielaID:         quinielaID,
		TakenAt:            time.Now().UTC(),
		Entries:            snapshotEntries,
		TriggeredByMatchID: triggerMatchID,
	}
	if err := s.snapRepo.Create(ctx, snap); err != nil {
		return nil, err
	}
	return snap, nil
}

var _ Snapshotter = (*leaderboardSnapshotService)(nil)
