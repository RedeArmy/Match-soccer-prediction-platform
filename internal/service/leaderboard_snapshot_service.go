package service

import (
	"context"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

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

// Snapshot computes the current leaderboard via Ranker and stores it as an
// immutable point-in-time snapshot. Called by the scoring worker immediately
// after ScoreMatch so the latest rankings are available without re-computing
// them on every API request.
//
// An empty leaderboard (no active paid members yet) is stored as an explicit
// snapshot with an empty Entries slice rather than being skipped, so the
// caller can distinguish "snapshot taken but nobody has points" from "no
// snapshot has ever been taken".
func (s *leaderboardSnapshotService) Snapshot(ctx context.Context, quinielaID int) (*domain.LeaderboardSnapshot, error) {
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
		QuinielaID: quinielaID,
		TakenAt:    time.Now().UTC(),
		Entries:    snapshotEntries,
	}
	if err := s.snapRepo.Create(ctx, snap); err != nil {
		return nil, err
	}
	return snap, nil
}

var _ Snapshotter = (*leaderboardSnapshotService)(nil)
